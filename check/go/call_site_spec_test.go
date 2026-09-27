package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// specFn is a one-signature constant fn with an identity.
func specFn(name string) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Signatures: []core.Signature{{}}})
}

// A param/arg pair specialises only as a NAMED Function param handed a
// constant fn that has an identity and nothing beyond its signatures.
func TestSpecialisableFnArg(t *testing.T) {
	r, _ := core.NewRegistry()
	other, _ := core.NewRegistry()
	gParam := core.FnParam{Name: "g", Type: core.TFunction}
	if _, ok := specialisableFnArg(r, gParam, specFn("inc")); !ok {
		t.Fatal("a constant fn to a named Function param specialises")
	}
	pattern := core.NewInteger(1)
	withFn := func(edit func(*core.FnDefInfo)) core.Value {
		fd := core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{}}}
		edit(&fd)
		return core.NewFunction(fd)
	}
	carrier := specFn("inc")
	carrier.Carrier = true
	for name, c := range map[string]struct {
		p core.FnParam
		a core.Value
	}{
		"unnamed param":    {core.FnParam{Type: core.TFunction}, specFn("inc")},
		"Any param":        {core.FnParam{Name: "g", Type: core.TAny}, specFn("inc")},
		"untyped param":    {core.FnParam{Name: "g"}, specFn("inc")},
		"patterned param":  {core.FnParam{Name: "g", Type: core.TFunction, Pattern: &pattern}, specFn("inc")},
		"carrier arg":      {gParam, carrier},
		"data arg":         {gParam, core.NewInteger(1)},
		"no identity":      {gParam, core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "x"}}},
		"capturing fn":     {gParam, withFn(func(fd *core.FnDefInfo) { fd.Captured = []core.CapturedBinding{{Name: "c"}} })},
		"generic fn":       {gParam, withFn(func(fd *core.FnDefInfo) { fd.Gen = &core.GenSpecInfo{} })},
		"macro fn":         {gParam, withFn(func(fd *core.FnDefInfo) { fd.Macro = true })},
		"modifier wrapper": {gParam, withFn(func(fd *core.FnDefInfo) { fd.Wrap = core.WrapReverse })},
		"foreign home":     {gParam, withFn(func(fd *core.FnDefInfo) { fd.Registry = other })},
	} {
		if _, ok := specialisableFnArg(r, c.p, c.a); ok {
			t.Errorf("%s: must not specialise", name)
		}
	}
}

// A fn LITERAL is pushed with a fresh identity each time a fn or closure
// unit runs it (NUR288), so passed from inside a fn body's analysis or a
// closure unit it does not specialise — its guard would never hold. At the
// root it keeps one identity, and a named reference keeps one everywhere.
func TestSpecialisableFnArgFreshLiteral(t *testing.T) {
	r, _ := core.NewRegistry()
	gParam := core.FnParam{Name: "g", Type: core.TFunction}
	lit := specFn("")
	if _, ok := specialisableFnArg(r, gParam, lit); !ok {
		t.Fatal("a root fn literal keeps one identity and specialises")
	}
	r.Check.FnBodyDepth = 1
	if _, ok := specialisableFnArg(r, gParam, lit); ok {
		t.Error("a fn literal inside a fn body's analysis must not specialise")
	}
	if _, ok := specialisableFnArg(r, gParam, specFn("inc")); !ok {
		t.Error("a named fn reference specialises inside a fn body")
	}
	r.Check.FnBodyDepth = 0
	r.Check.Emit = &covCDEmit{EmitRecorder: core.TheInactiveEmit, inClosure: true}
	if _, ok := specialisableFnArg(r, gParam, lit); ok {
		t.Error("a fn literal inside a closure unit must not specialise")
	}
}

// specialisedArgs keeps each qualifying constant fn (under a fresh value ID),
// generalises the rest as the generic unit does, and names them in the key
// suffix; with none qualifying there is nothing to specialise.
func TestSpecialisedArgs(t *testing.T) {
	params := []core.FnParam{{Name: "g", Type: core.TFunction}, {Name: "n", Type: core.TInteger}}
	fn := specFn("inc")
	gen := []core.Value{core.NewCarrier(core.TFunction), core.NewCarrier(core.TInteger)}
	r, _ := core.NewRegistry()
	args, ps, fns, suffix := specialisedArgs(r, params, []core.Value{fn, core.NewInteger(3)}, gen)
	if len(args) != 2 || !core.ExactEqual(args[0], fn) || args[0].ID == fn.ID || args[1].ID != gen[1].ID {
		t.Errorf("want the fn kept under a fresh ID and the rest generalised, got %v", args)
	}
	if len(ps) != 1 || ps[0] != 0 || len(fns) != 1 || suffix == "" {
		t.Errorf("want one guard on param 0 and a suffix, got %v %v %q", ps, fns, suffix)
	}
	if args, _, _, _ := specialisedArgs(r, params, []core.Value{core.NewInteger(1), core.NewInteger(3)}, gen); args != nil {
		t.Errorf("no constant fn arg: nothing to specialise, got %v", args)
	}
}

// The residual check: nothing declared always meets; a count miss or a
// strict value outside its declared type misses; a gradual value is a bound,
// never a miss.
func TestSpecResidualMeetsReturns(t *testing.T) {
	ints := []*core.Type{core.TInteger}
	dyn := core.NewDynamicCarrier(core.TString)
	for name, c := range map[string]struct {
		residual []core.Value
		returns  []*core.Type
		want     bool
	}{
		"nothing declared": {[]core.Value{core.NewInteger(1), core.NewInteger(2)}, nil, true},
		"count miss":       {[]core.Value{core.NewInteger(1), core.NewInteger(2)}, ints, false},
		"strict miss":      {[]core.Value{core.NewCarrier(core.TString)}, ints, false},
		"gradual":          {[]core.Value{dyn}, ints, true},
		"conforming":       {[]core.Value{core.NewCarrier(core.TInteger)}, ints, true},
	} {
		if got := specResidualMeetsReturns(c.residual, c.returns); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

// specialiseCallSite's early refusals leave the call site to the generic
// unit; a qualifying call compiles its unit (here a stub) and counts it,
// once per (site, fn) however often the site recurs, up to the quota.
func TestSpecialiseCallSiteAdmission(t *testing.T) {
	r, _ := core.NewRegistry()
	es := r.Check.Recorder()
	params := []core.FnParam{{Name: "g", Type: core.TFunction}}
	gen := []core.Value{core.NewCarrier(core.TFunction)}
	compiled := 0
	stub := func([]core.Value, string) int { compiled++; return 7 }
	fd := core.FnDefInfo{Name: "h", Signatures: []core.Signature{{}}}
	body := []core.Value{core.NewWord("g")}
	call := func(fd core.FnDefInfo, args []core.Value) int {
		return specialiseCallSite(r, es, stub, fd, false, "h", body, params, args, gen)
	}
	inc := specFn("inc")
	r.Check.SpecOff = true
	if call(fd, []core.Value{inc}) != -1 {
		t.Error("SpecOff: no specialisation")
	}
	r.Check.SpecOff = false
	multi := fd
	multi.Signatures = []core.Signature{{}, {}}
	generic := fd
	generic.Gen = &core.GenSpecInfo{}
	capturing := fd
	capturing.Captured = []core.CapturedBinding{{Name: "n"}}
	for name, f := range map[string]core.FnDefInfo{"multi-signature": multi, "generic": generic, "capturing": capturing} {
		if call(f, []core.Value{inc}) != -1 {
			t.Errorf("%s callee: no specialisation", name)
		}
	}
	if specialiseCallSite(r, es, stub, fd, true, "h", body, params, []core.Value{inc}, gen) != -1 {
		t.Error("a foreign-home callee: no specialisation")
	}
	if call(fd, []core.Value{core.NewInteger(1)}) != -1 || compiled != 0 {
		t.Error("no constant fn arg: no specialisation, nothing compiled")
	}
	if call(fd, []core.Value{inc}) != 7 || !r.Check.SpecTried {
		t.Error("a qualifying call records its specialised unit")
	}
	if call(fd, []core.Value{inc}) != 7 {
		t.Error("the same (site, fn) again records its unit, uncounted")
	}
	for i := 0; i < FnSpecQuota; i++ {
		call(fd, []core.Value{specFn("f")})
	}
	if call(fd, []core.Value{specFn("over")}) != -1 {
		t.Errorf("past FnSpecQuota distinct fns the site stops specialising")
	}
}

// A plain Map arg to a NAMED, untyped Map param specialises on its shape;
// a patterned, Node-typed or unnamed param does not, nor does a map with no
// shape.
func TestSpecialisableShapeArg(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	arg := core.NewMap(m)
	mParam := core.FnParam{Name: "m", Type: core.TMap}
	if s, _, ok := specialisableShapeArg(mParam, arg); !ok || !core.IsShapeCarrier(s) {
		t.Fatal("a plain Map to m:Map specialises on its shape")
	}
	pat := core.NewInteger(1)
	for name, p := range map[string]core.FnParam{
		"unnamed": {Type: core.TMap}, "Node": {Name: "m", Type: core.TNode},
		"untyped": {Name: "m"}, "value-patterned": {Name: "m", Type: core.TMap, Pattern: &pat},
	} {
		if _, _, ok := specialisableShapeArg(p, arg); ok {
			t.Errorf("%s: must not specialise", name)
		}
	}
	if _, _, ok := specialisableShapeArg(mParam, core.NewCarrier(core.TMap)); ok {
		t.Error("a Map carrier has no shape")
	}
	recFields := core.NewOrderedMap()
	recFields.Set("a", core.NewTypeLiteral(core.TInteger))
	recType := &core.Type{Parent: core.TMap}
	recType.SetTypeBody(core.NewMap(recFields))
	if _, _, ok := specialisableShapeArg(core.FnParam{Name: "m", Type: recType}, arg); !ok {
		t.Error("a record-typed param specialises on the arg's shape")
	}
	schemaType := &core.Type{Parent: core.TMap}
	schemaType.SetTypeBody(core.NewRecordType(recFields))
	if !isRecordType(schemaType) {
		t.Error("a node whose body is a record schema is a record type")
	}
	bodyless := &core.Type{Parent: core.TMap}
	scalarBody := &core.Type{Parent: core.TMap}
	scalarBody.SetTypeBody(core.NewInteger(1))
	for name, rt := range map[string]*core.Type{"no parent": {}, "not under Map": core.TInteger, "no body": bodyless, "scalar body": scalarBody} {
		if isRecordType(rt) {
			t.Errorf("%s: not a record type", name)
		}
	}
	xsParam := core.FnParam{Name: "xs", Type: core.TList}
	if s, _, ok := specialisableShapeArg(xsParam, core.NewList([]core.Value{core.NewInteger(1)})); !ok || !core.IsListShapeGuard(s) {
		t.Error("a plain List to xs:List specialises on its shape")
	}
	if _, _, ok := specialisableShapeArg(xsParam, arg); ok {
		t.Error("a Map to a List param has no list shape")
	}
	// specialisedArgs keys the shape into the suffix.
	r, _ := core.NewRegistry()
	args, ps, guards, suffix := specialisedArgs(r, []core.FnParam{mParam}, []core.Value{arg}, []core.Value{core.NewCarrier(core.TMap)})
	if len(args) != 1 || !core.IsShapeCarrier(args[0]) || len(ps) != 1 || len(guards) != 1 || suffix == "" {
		t.Errorf("want one shape guard, got %v %v %v %q", args, ps, guards, suffix)
	}
}

// A body residual proving an exact leaf scalar under a declared `Any`
// surfaces that type; anything else keeps the declaration's rules.
func TestProvenNarrowerReturn(t *testing.T) {
	ints := core.NewCarrier(core.TInteger)
	typed := []core.FnParam{{Name: "x", Type: core.TInteger}}
	if _, ok := provenNarrowerReturn(core.TAny, []core.FnParam{{Name: "x", Type: core.TAny}}, []core.Value{ints}, 1, 0); ok {
		t.Error("an Any-param fn's result stays gradual")
	}
	if c, ok := provenNarrowerReturn(core.TAny, typed, []core.Value{ints}, 1, 0); !ok || !c.Parent.Equal(core.TInteger) || c.Dynamic {
		t.Errorf("an Integer carrier under [Any] narrows, got %v %v", c, ok)
	}
	for name, c := range map[string]struct {
		t   *core.Type
		stk []core.Value
	}{
		"count miss":  {core.TAny, []core.Value{ints, ints}},
		"Number decl": {core.TNumber, []core.Value{ints}},
		"concrete":    {core.TAny, []core.Value{core.NewInteger(1)}},
		"dynamic":     {core.TAny, []core.Value{core.NewDynamicCarrier(core.TInteger)}},
		"upper bound": {core.TAny, []core.Value{core.NewCarrier(core.TNode)}},
		"String":      {core.TAny, []core.Value{core.NewCarrier(core.TString)}},
		"no parent":   {core.TAny, []core.Value{{Carrier: true}}},
		"nil decl":    {nil, []core.Value{ints}},
	} {
		if _, ok := provenNarrowerReturn(c.t, typed, c.stk, 1, 0); ok {
			t.Errorf("%s: must not narrow", name)
		}
	}
	if allParamsTyped([]core.FnParam{{Name: "x", Type: core.TAny}}) || allParamsTyped([]core.FnParam{{Name: "x"}}) {
		t.Error("an Any or untyped param is not typed")
	}
	if !allParamsTyped([]core.FnParam{{Name: "x", Type: core.TInteger}}) {
		t.Error("an Integer param is typed")
	}
	// A body holding a `do` may answer a trapped Error in the proven slot.
	do := core.NewWord("do")
	if !bodyTrapsErrors([]core.Value{core.NewList([]core.Value{do})}) || bodyTrapsErrors([]core.Value{core.NewInteger(1)}) {
		t.Error("a body's do traps; a body without one does not")
	}
	reg, _ := core.NewRegistry()
	if _, ok := refinedDeclaredReturn(reg, core.TAny, typed, nil, 0, 1, []core.Value{ints}, true); ok {
		t.Error("a trapping body's residual narrows nothing")
	}
	if _, ok := refinedDeclaredReturn(reg, core.TAny, typed, nil, 0, 1, []core.Value{ints}, false); !ok {
		t.Error("a plain body's Integer residual narrows")
	}
}
