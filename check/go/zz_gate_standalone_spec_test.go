package check

// Standalone-gate tests for the call-site specialisation surface
// (call_site_spec.go, the specialised halves of check_fnbody.go and
// carrier.go's runFnBodyOnce, and spec_fn_unit.go's unit compile).
//
// Every arm here is covered by the merged ADR-008 gate through the
// compiler's armed recording pass, which check cannot import. The tests
// drive the same arms compiler-less: an EmitRecorder stub embedding the
// inactive recorder (fmRec, zz_cover_fnmodel_test.go) stands in for the
// armed pass, and the specialisation state the compiler's recording would
// publish (CheckState.SpecParamNames / SpecKeySuffix) is set directly.

import (
	"maps"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gsIncFn is a one-signature constant fn `n:Integer -> Integer` — the
// constant fn a specialised call site passes to a Function param.
func gsIncFn() core.Value {
	return core.NewFunction(core.FnDefInfo{Name: "zgsinc", Signatures: []core.Signature{{
		Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns:    []*core.Type{core.TInteger},
		Impl:       core.Boru([]core.Value{core.NewWord("n")}),
		BarrierPos: core.BarrierAllForward,
	}}})
}

// specParamCallMayRefuse: a call dispatched THROUGH a specialised constant-fn
// param could fail the callee's param contract at run time when an argument
// has no type, its type (or a gradual value's bound) lies outside the
// param's declared type, or the param is patterned. A value inside its
// param's type — strict or gradual — is the passing form, as is an untyped
// param; arguments past the params are not the callee's to refuse.
func TestGateStandaloneSpecParamCallMayRefuse(t *testing.T) {
	intP := core.FnParam{Name: "n", Type: core.TInteger}
	pat := core.NewInteger(1)
	for name, c := range map[string]struct {
		params []core.FnParam
		args   []core.Value
		want   bool
	}{
		"strict value inside the type":  {[]core.FnParam{intP}, []core.Value{core.NewCarrier(core.TInteger)}, false},
		"gradual value inside the type": {[]core.FnParam{intP}, []core.Value{core.NewDynamicCarrier(core.TInteger)}, false},
		"untyped param":                 {[]core.FnParam{{Name: "x"}}, []core.Value{core.NewCarrier(core.TString)}, false},
		"surplus argument":              {[]core.FnParam{intP}, []core.Value{core.NewInteger(1), core.NewString("s")}, false},
		"no params":                     {nil, []core.Value{core.NewString("s")}, false},
		"gradual Any bound":             {[]core.FnParam{intP}, []core.Value{core.NewDynamicCarrier(core.TAny)}, true},
		"strict value outside the type": {[]core.FnParam{intP}, []core.Value{core.NewCarrier(core.TString)}, true},
		"argument with no type":         {[]core.FnParam{intP}, []core.Value{core.MakeCarrierValue(nil, nil)}, true},
		"patterned param":               {[]core.FnParam{{Name: "n", Type: core.TInteger, Pattern: &pat}}, []core.Value{core.NewInteger(1)}, true},
		"second argument refuses":       {[]core.FnParam{intP, intP}, []core.Value{core.NewInteger(1), core.NewCarrier(core.TString)}, true},
	} {
		if got := specParamCallMayRefuse(c.params, c.args); got != c.want {
			t.Errorf("%s: specParamCallMayRefuse = %v, want %v", name, got, c.want)
		}
	}
}

// The ReturnsFn arm that consults it (check_fnbody.go): a call dispatched
// under a name the specialised analysis bound to a constant fn
// (CheckState.SpecParamNames, published by runFnBodyOnce) whose arguments
// could refuse the callee's contract latches CheckState.SpecDeclined, so
// the compile pass retries the call site unspecialised. The same refusing
// arguments under any other dispatching name, and conforming arguments
// under the specialised name, decline nothing.
func TestGateStandaloneSpecParamCallLatchesDecline(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	fd, _ := core.DataOf(addrOf(gsIncFn())).(*core.FnDefInfo)
	rf := BuildFnBodyReturnsFn(r, "zgsinc", fd.Signatures[0], *fd)
	r.Check.SpecParamNames = map[string]bool{"g": true}

	// Passing form: `g` is the specialised param, its argument conforms.
	r.Check.CurCallWord = "g"
	rf([]core.Value{core.NewCarrier(core.TInteger)}, r)
	if r.Check.SpecDeclined {
		t.Fatal("a conforming argument through the specialised param must not decline")
	}
	// A gradual Any argument could miss `n:Integer` at run time: decline.
	rf([]core.Value{core.NewDynamicCarrier(core.TAny)}, r)
	if !r.Check.SpecDeclined {
		t.Fatal("a refusable argument through the specialised param must latch SpecDeclined")
	}
	// The same argument dispatched under a name that is no specialised
	// param is the generic unit's own run-time apply: nothing to decline.
	r.Check.SpecDeclined = false
	r.Check.CurCallWord = "zgsinc"
	rf([]core.Value{core.NewDynamicCarrier(core.TAny)}, r)
	if r.Check.SpecDeclined {
		t.Error("a call under a non-specialised name must not decline the specialisation")
	}
}

// fnUnitCompile.compile's specialised-residual check (check_fnbody.go): a
// SPECIALISED unit (a key suffix) whose analysed residual cannot meet the
// unit's RET contract — here two values against one declared return —
// latches SpecDeclined, because the specialised unit would raise at its own
// RET where the generic unit's run-time apply raises at the call. The
// generic unit (no suffix) with the same body declines nothing, and a
// specialised unit whose residual meets the contract declines nothing.
func TestGateStandaloneSpecUnitResidualContract(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.startUnit = 4
	r.Check.Emit = rec

	ints := []*core.Type{core.TInteger}
	two := &fnUnitCompile{r: r, es: rec, name: "zgstwo", body: []core.Value{core.NewInteger(1), core.NewInteger(2)},
		compileReturns: ints, returns: ints}
	if u := two.compile(nil, ""); u != 4 || r.Check.SpecDeclined {
		t.Fatalf("generic unit: unit %d, SpecDeclined %v — want unit 4 and no decline", u, r.Check.SpecDeclined)
	}
	if u := two.compile(nil, "!spec0=zgsinc"); u != 4 || !r.Check.SpecDeclined {
		t.Fatalf("specialised unit over a two-value residual: unit %d, SpecDeclined %v — want unit 4 and a decline", u, r.Check.SpecDeclined)
	}
	if len(rec.finished) != 2 || len(rec.finished[1]) != 2 {
		t.Errorf("each compile finishes its unit with the analysed residual, got %v", rec.finished)
	}

	r.Check.SpecDeclined = false
	one := &fnUnitCompile{r: r, es: rec, name: "zgsone", body: []core.Value{core.NewInteger(1)},
		compileReturns: ints, returns: ints}
	if u := one.compile(nil, "!spec0=zgsinc"); u != 4 || r.Check.SpecDeclined {
		t.Errorf("specialised unit meeting its contract: unit %d, SpecDeclined %v — want unit 4 and no decline", u, r.Check.SpecDeclined)
	}
	if r.Check.SpecKeySuffix != "" {
		t.Errorf("the one-shot key suffix must be consumed by the analysis, left %q", r.Check.SpecKeySuffix)
	}
}

// runFnBodyOnce's frame-bind mode (carrier.go): only a call-site
// specialised analysis (AnalyseFnBody under a SpecKeySuffix) binds a named
// param holding a CONCRETE fn the interpreter's way and publishes it in
// CheckState.SpecParamNames for the body's duration — the set
// specParamCallMayRefuse is consulted against. A non-fn param is not
// published, the generic analysis publishes nothing, and the published set
// is withdrawn when the analysis returns.
func TestGateStandaloneSpecAnalysisPublishesConstantFnParams(t *testing.T) {
	r := newTestRegistry(t)
	var seen map[string]bool
	var gBound bool
	probes := 0
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zgsprobe",
		Signatures: []core.Signature{{
			Returns:    []*core.Type{},
			BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				probes++
				seen = maps.Clone(reg.Check.SpecParamNames)
				top, ok := reg.Defs.Top("g")
				_, isFn := core.DataOf(&top).(*core.FnDefInfo)
				gBound = ok && isFn
				return nil, nil
			}, core.RunInCheck()),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	done := r.Check.Begin()
	defer done()

	body := []core.Value{core.NewWord("zgsprobe")}
	params := []string{"g", "k"}
	args := []core.Value{gsIncFn(), core.NewCarrier(core.TInteger)}

	// The generic analysis: the fn binds as a frame value, nothing is
	// published.
	AnalyseFnBody(r, "zgsh", params, body, args, nil, nil, false)
	if probes != 1 || !gBound {
		t.Fatalf("generic analysis: probes %d, g bound as a fn %v", probes, gBound)
	}
	if len(seen) != 0 {
		t.Errorf("the generic analysis must publish no specialised params, saw %v", seen)
	}

	// The specialised analysis: g (a concrete fn) is published, k is not.
	r.Check.SpecKeySuffix = "!spec0=zgsinc"
	AnalyseFnBody(r, "zgsh", params, body, args, nil, nil, false)
	if probes != 2 || !gBound {
		t.Fatalf("specialised analysis: probes %d, g bound as a fn %v", probes, gBound)
	}
	if !seen["g"] || seen["k"] {
		t.Errorf("the specialised analysis publishes exactly its constant-fn params, saw %v", seen)
	}
	if r.Check.SpecParamNames != nil {
		t.Errorf("the published set must be withdrawn on return, left %v", r.Check.SpecParamNames)
	}
	if _, ok := r.Defs.Top("g"); ok {
		t.Error("the frame binding of g must unwind with the analysis")
	}
}

// CompileFnSigUnit's compile (spec_fn_unit.go): with a recorder that opens
// the unit, the signature's declared param types and patterns, its return
// patterns and its declaration site are stamped on the unit, and the body —
// analysed over the params' own carriers — finishes it. The recorder
// declining the unit is the decline arm it pairs with.
func TestGateStandaloneCompileFnSigUnitCompiles(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	rec := newFmRec()
	rec.armed = true
	rec.startUnit = 9
	r.Check.Emit = rec

	fd, _ := core.DataOf(addrOf(gsIncFn())).(*core.FnDefInfo)
	if u := CompileFnSigUnit(r, *fd, 0); u != 9 {
		t.Fatalf("CompileFnSigUnit = %d, want the recorder's unit 9", u)
	}
	if len(rec.startArgs) != 1 || !core.IsCarrier(&rec.startArgs[0]) || !core.ParentOf(&rec.startArgs[0]).Equal(core.TInteger) {
		t.Errorf("the unit's inputs are the params' own carriers, got %v", rec.startArgs)
	}
	if len(rec.pts) != 1 || !rec.pts[0].Equal(core.TInteger) {
		t.Errorf("the unit carries the declared param types, got %v", rec.pts)
	}
	if !rec.retPats || !rec.declSet {
		t.Error("the unit carries the return patterns and the declaration site")
	}
	if len(rec.finished) != 1 || len(rec.finished[0]) != 1 || !core.ParentOf(&rec.finished[0][0]).Equal(core.TInteger) {
		t.Errorf("the body's analysed residual finishes the unit, got %v", rec.finished)
	}

	rec.startUnit = -1
	if u := CompileFnSigUnit(r, *fd, 0); u != -1 {
		t.Errorf("a recorder declining the unit compiles nothing, got %d", u)
	}
}
