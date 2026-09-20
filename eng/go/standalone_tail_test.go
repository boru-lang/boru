package eng_test

// Standalone-suite tail probes, external half (design/
// ENG-COVERAGE-PARITY.0.md stage 4): the fn-spec parameter matrix
// (ParseFnParams / ResolveSigType) and the predicate-type runner.
// Rows run through the specfix registry; the pattern shapes the
// surface syntax can't spell (pre-evaluated literal elements) call
// ParseFnDef with constructed value lists — the same shapes lang's
// richer spec assembly hands it.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

func TestFnSpecParamMatrix(t *testing.T) {
	rows := []struct {
		input   string
		want    string
		wantErr string
	}{
		// Optional param via a None-bearing disjunct: both arities.
		{input: "def f fn [[x:(Integer tor None)] [Integer] [1]] f 5", want: "1"},
		{input: "def f fn [[x:(Integer tor None)] [Integer] [1]] f", want: "1"},

		// Named literal patterns; the annotation value fixes the kind.
		{input: "def f fn [[x:5] [Integer] [3]] f 5", want: "3"},
		{input: "def f fn [[x:2.5] [Integer] [4]] f 2.5", want: "4"},
		{input: "def bt true def f fn [[x:bt] [Integer] [1]] f true",
			wantErr: "must start with an uppercase letter"},

		// Structural annotations: map pattern, typed-list child, empty list.
		{input: "def f fn [[x:{a:Integer}] [Integer] [5]] f {a:9}", want: "5"},
		{input: "def f fn [[x:[:Integer]] [Integer] [6]] f [1 2]", want: "6"},
		{input: "def f fn [[x:[]] [Integer] [7]] f []", want: "7"},

		// Named types: a def-installed alias ADOPTS the canonical
		// aliased node (core.InstallType's alias arm — NUR093), so it
		// is TRANSPARENT: the raw base value dispatches into it exactly
		// as `42 is Foo` always accepted it. The old "alias is nominal"
		// reading minted an unbridged child no value could ever inhabit
		// (typed-defs did not tag either) — a nominal category with no
		// members. Nominal identity is `refine`'s job, not `def`'s.
		{input: "def Big Integer def f fn [[x:Big] [Integer] [8]] f 3",
			want: "8"},
		{input: "def Rec (refine Record [{a:Integer}]) def f fn [[x:Rec] [Integer] [9]] f {a:1}",
			want: "9"},
		{input: "def f fn [[x:P] [Integer] [10]] f p", want: "10"},

		// Builtin names in the returns slot, including the None mismatch.
		{input: "def f fn [[x:Integer] [None] [11]] f 1",
			wantErr: "expected None, got Integer"},
		{input: "def f fn [[x:Integer] [Boolean] [true]] f 1", want: "true"},
		{input: "def f fn [[x:Integer] [Map] [{a:1}]] f 1", want: "{a:1}"},
		{input: "def f fn [[x:Integer] [List] [[1]]] f 1", want: "[1]"},
		{input: "def f fn [[x:Integer] [Function] [(fn [[y:Integer] [Integer] [1]])]] f 1",
			want: "fn [[y:Integer][Integer][1]]"},

		// A dotted annotation with no bound module export is an error.
		{input: "def f fn [[x:a.b] [Integer] [1]]",
			wantErr: "dotted annotation must reach a type literal"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			values, err := parser.Parse(row.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, runErr := core.NewTop(specfixProbeRegistry(t)).Run(values)
			if row.wantErr != "" {
				if runErr == nil || !strings.Contains(runErr.Error(), row.wantErr) {
					t.Errorf("want error containing %q, got %v (out %s)", row.wantErr, runErr, core.Canon(out))
				}
				return
			}
			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			if got := core.Canon(out); got != row.want {
				t.Errorf("got %s, want %s", got, row.want)
			}
		})
	}
}

// TestParseFnDefConstructedSpecs drives the parameter arms only a
// pre-evaluated spec list reaches: unnamed positional literal
// patterns of every scalar kind.
func TestParseFnDefConstructedSpecs(t *testing.T) {
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := core.NewList([]Value{core.NewInteger(1)})
	rets := core.NewList([]Value{core.NewTypeLiteral(core.TInteger)})
	mk := func(params ...Value) []Value {
		return []Value{core.NewList(params), rets, body}
	}
	for _, c := range []struct {
		name  string
		param Value
	}{
		{"integer", core.NewInteger(7)},
		{"boolean", core.NewBoolean(true)},
		{"string", core.NewString("pat")},
		{"bare-type-node", core.NewTypeLiteral(core.TInteger)},
	} {
		if _, perr := core.ParseFnDef(r, mk(c.param)); perr != nil {
			t.Errorf("%s positional pattern: %v", c.name, perr)
		}
	}
	// Named pairs with pre-evaluated annotation values (the shapes the
	// surface syntax resolves as type names instead).
	bt := core.NewOrderedMap()
	bt.Set("x", core.NewBoolean(true))
	if _, perr := core.ParseFnDef(r, mk(core.NewMap(bt))); perr != nil {
		t.Errorf("named boolean pattern: %v", perr)
	}
	st := core.NewOrderedMap()
	st.Set("x", core.NewAtom("tag"))
	if _, perr := core.ParseFnDef(r, mk(core.NewMap(st))); perr != nil {
		t.Errorf("named atom pattern: %v", perr)
	}
	// An unresolvable element shape is the invalid-parameter arm.
	if _, perr := core.ParseFnDef(r, mk(core.NewError(errProbe{}))); perr == nil {
		t.Error("error-value param must be rejected")
	}
}

type Value = core.Value

type errProbe struct{}

func (errProbe) Error() string { return "probe" }

func TestRunPredicateArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	build := func(src string) Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := core.NewTop(r).Run(vals)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v %v", src, out, err)
		}
		return out[0]
	}
	pred := build("fn [[x:Integer] [Boolean] [x is Integer]]")

	if _, ok, err := r.RunPredicate(pred, core.NewInteger(5)); !ok || err != nil {
		t.Errorf("integer candidate must match: ok=%v err=%v", ok, err)
	}
	// Candidate outside the declared input type: no match, no run.
	if _, ok, err := r.RunPredicate(pred, core.NewString("s")); ok || err != nil {
		t.Errorf("string candidate must not match: ok=%v err=%v", ok, err)
	}
	// Non-fn constraint.
	if _, _, err := r.RunPredicate(core.NewInteger(1), core.NewInteger(5)); err == nil {
		t.Error("non-fn constraint must error")
	}
	// A two-param fn is not a predicate shape.
	two := build("fn [[a:Integer b:Integer] [Boolean] [true]]")
	if _, _, err := r.RunPredicate(two, core.NewInteger(5)); err == nil {
		t.Error("two-param constraint must error")
	}
	// A predicate returning false rejects the candidate.
	no := build("fn [[x:Integer] [Boolean] [false]]")
	if _, ok, err := r.RunPredicate(no, core.NewInteger(5)); ok || err != nil {
		t.Errorf("false predicate must reject: ok=%v err=%v", ok, err)
	}
	// A non-Boolean result is a VALUE-mode predicate: the result is
	// the transformed binding.
	xform := build("fn [[x:Integer] [Integer] [x addq 1]]")
	out, ok, err := r.RunPredicate(xform, core.NewInteger(5))
	if err != nil || !ok {
		t.Fatalf("value-mode predicate: ok=%v err=%v", ok, err)
	}
	if got := core.Canon([]Value{out}); got != "6" {
		t.Errorf("value-mode predicate result = %s, want 6", got)
	}
	// A predicate producing None rejects.
	noneOut := build("fn [[x:Integer] [Any] [{a:1} get b]]")
	if _, ok, err := r.RunPredicate(noneOut, core.NewInteger(5)); ok || err != nil {
		t.Errorf("None-producing predicate must reject: ok=%v err=%v", ok, err)
	}
	// A raising predicate surfaces the error.
	boom := build("fn [[x:Integer] [Any] [refine 5]]")
	if _, _, err := r.RunPredicate(boom, core.NewInteger(5)); err == nil {
		t.Error("raising predicate must surface its error")
	}
	// Check-mode short-circuit: matched without running the body.
	done := r.Check.Begin()
	if _, ok, err := r.RunPredicate(no, core.NewInteger(5)); !ok || err != nil {
		t.Errorf("check-mode must short-circuit to match: ok=%v err=%v", ok, err)
	}
	done()
}

// TestCallBoruArms drives the direct fn-call API: list-argument
// handling, body def-leak recovery, and unnamed-arg result trimming.
func TestCallBoruArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	build := func(src string) Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := core.NewTop(r).Run(vals)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v %v", src, out, err)
		}
		return out[0]
	}
	sigOf := func(v Value) *core.FnSig {
		t.Helper()
		info, ok := v.Data.(core.FnDefInfo)
		if !ok || len(info.Signatures) == 0 {
			t.Fatalf("not a fn value: %s", v.String())
		}
		return &info.Signatures[0]
	}

	// A list argument passes through unquoted.
	lst := build("fn [[x:[:Integer]] [Integer] [x lengthq]]")
	out, err := r.CallBoru(sigOf(lst), []Value{core.NewList([]Value{core.NewInteger(1), core.NewInteger(2)})}, nil)
	if err != nil || core.Canon(out) != "2" {
		t.Errorf("list arg call = %s / %v, want 2", core.Canon(out), err)
	}

	// A body that defs without undef: the leak-recovery arm cleans up.
	leak := build("fn [[x:Integer] [Integer] [def zz 9 x]]")
	out, err = r.CallBoru(sigOf(leak), []Value{core.NewInteger(4)}, nil)
	if err != nil || core.Canon(out) != "4" {
		t.Errorf("def-leaking call = %s / %v, want 4", core.Canon(out), err)
	}
	if _, bound := r.Defs.Top("zz"); bound {
		t.Error("leaked def must be cleaned up after the call")
	}

	// An unnamed positional param with a body producing an extra value:
	// the trim arm drops the unconsumed input from the residual bottom.
	extra := build("fn [Integer [Integer] [7]]")
	out, err = r.CallBoru(sigOf(extra), []Value{core.NewInteger(4)}, nil)
	if err != nil || core.Canon(out) != "7" {
		t.Errorf("unnamed-extra call = %s / %v, want 7", core.Canon(out), err)
	}
}

func TestRunPredicateShapeArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	// A Function-parented value with no FnDefInfo payload.
	if _, _, err := r.RunPredicate(core.NewValueRaw(core.TFunction, core.IntPayload{N: 1}), core.NewInteger(5)); err == nil {
		t.Error("non-FnDefInfo constraint must error")
	}
	// A predicate whose body yields two values.
	vals, err := parser.Parse("fn [[x:Integer] [Any] [1 2]]")
	if err != nil {
		t.Fatal(err)
	}
	out, err := core.NewTop(r).Run(vals)
	if err != nil || len(out) != 1 {
		t.Fatalf("build: %v %v", out, err)
	}
	if _, _, err := r.RunPredicate(out[0], core.NewInteger(5)); err == nil {
		t.Error("two-value predicate body must error")
	}
}

// TestParseFnDefReturnSlots drives the return-list resolution arms
// with pre-evaluated values: literal patterns in the returns slot,
// typed-map children, and the empty returns list.
func TestParseFnDefReturnSlots(t *testing.T) {
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := core.NewList([]Value{core.NewInteger(1)})
	params := core.NewList([]Value{core.NewTypeLiteral(core.TInteger)})
	mkRets := func(rets ...Value) []Value {
		return []Value{params, core.NewList(rets), body}
	}
	for _, c := range []struct {
		name string
		ret  Value
	}{
		{"boolean-literal", core.NewBoolean(true)},
		{"string-literal", core.NewString("s")},
		{"integer-literal", core.NewInteger(5)},
		{"atom-literal", core.NewAtom("tag")},
		{"typed-map", core.NewTypedMap(core.NewTypeLiteral(core.TInteger))},
		{"typed-list", core.NewTypedList(core.NewTypeLiteral(core.TInteger))},
		{"record", func() Value {
			om := core.NewOrderedMap()
			om.Set("a", core.NewTypeLiteral(core.TInteger))
			return core.NewRecordType(om)
		}()},
	} {
		if _, perr := core.ParseFnDef(r, mkRets(c.ret)); perr != nil {
			t.Errorf("%s return slot: %v", c.name, perr)
		}
	}
	// Empty returns list.
	if _, perr := core.ParseFnDef(r, mkRets()); perr != nil {
		t.Errorf("empty returns: %v", perr)
	}
}

// TestRunTraceBattery drives the trace renderer over representative
// programs: scalar flows, containers, branch words, and an erroring
// row — the colorize/wrap/step machinery renders every shape.
func TestRunTraceBattery(t *testing.T) {
	rows := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "1 addq 2", want: "3"},
		{input: "[1 2] lengthq", want: "2"},
		{input: "{a:1} get a", want: "1"},
		{input: "'x' concatq 'y'", want: "'xy'"},
		{input: "dup", wantErr: true},
		{input: "1 2 dup", want: "1 2 2"},
		{input: "quote zz typeof", want: "Atom"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			values, err := parser.Parse(row.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			r := specfixProbeRegistry(t)
			var buf strings.Builder
			out, runErr := core.RunTrace(r, values, &buf)
			if row.wantErr {
				if runErr == nil {
					t.Fatalf("want error, got %s", core.Canon(out))
				}
				return
			}
			if runErr != nil {
				t.Fatalf("run: %v", runErr)
			}
			if got := core.Canon(out); got != row.want {
				t.Errorf("got %s, want %s", got, row.want)
			}
			if buf.Len() == 0 {
				t.Error("trace output must not be empty")
			}
		})
	}
}

// TestInstallTypeArms drives the type-installation policy across body
// kinds: alias literals, record shapes, refine prefabs, options and
// disjunct bodies, and the invalid-body / invalid-name errors.
func TestInstallTypeArms(t *testing.T) {
	r := specfixProbeRegistry(t)

	if err := core.InstallType(r, "Alias1", core.NewTypeLiteral(core.TInteger)); err != nil {
		t.Errorf("alias body: %v", err)
	}
	rec := core.NewOrderedMap()
	rec.Set("a", core.NewTypeLiteral(core.TInteger))
	if err := core.InstallType(r, "Rec1", core.NewMap(rec)); err != nil {
		t.Errorf("record body: %v", err)
	}
	if err := core.InstallType(r, "Opt1", core.NewOptionsType(rec)); err != nil {
		t.Errorf("options body: %v", err)
	}
	dis := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString)})
	if err := core.InstallType(r, "Dis1", dis); err != nil {
		t.Errorf("disjunct body: %v", err)
	}
	prefab := core.NewTypeLiteral(r.Types.MintRefinePrefab(core.TInteger))
	if err := core.InstallType(r, "Fresh1", prefab); err != nil {
		t.Errorf("refine prefab: %v", err)
	}
	// A literal scalar IS a valid body (value-literal types, def Five 5).
	if err := core.InstallType(r, "Five1", core.NewInteger(5)); err != nil {
		t.Errorf("literal body: %v", err)
	}
	if err := core.InstallType(r, "Bad1", core.NewWord("w")); err == nil {
		t.Error("word body must be rejected")
	}
	if err := core.InstallType(r, "lower", core.NewTypeLiteral(core.TInteger)); err == nil {
		t.Error("lowercase type name must be rejected")
	}
}

// TestInstallDefArms drives the def-installation helper across value
// kinds and the redefinition path.
func TestInstallDefArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	core.InstallDef(r, "d1", core.NewInteger(1))
	core.InstallDef(r, "d1", core.NewInteger(2))
	if v, ok := r.Defs.Top("d1"); !ok || core.Canon([]core.Value{v}) != "2" {
		t.Errorf("redefinition must replace: %v %v", v, ok)
	}
	core.InstallDef(r, "d2", core.NewList([]core.Value{core.NewInteger(1)}))
	core.InstallDef(r, "d3", core.NewTypeLiteral(core.TString))
	if _, ok := r.Defs.Top("d3"); !ok {
		t.Error("type-literal def must bind")
	}
}

// TestInstallDefFnArms drives installDef's Function-body arms: sig
// installation via a fn value, the shadow (frame-binding) variant, the
// overlap-removal on redefinition, the conditional-body compile failure, and
// the non-FnDefInfo carrier no-op.
func TestInstallDefFnArms(t *testing.T) {
	r := specfixProbeRegistry(t)
	build := func(src string) core.Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, err := core.NewTop(r).Run(vals)
		if err != nil || len(out) != 1 {
			t.Fatalf("build %q: %v %v", src, out, err)
		}
		return out[0]
	}
	fn1 := build("fn [[x:Integer] [Integer] [x addq 1]]")
	fn2 := build("fn [[x:Integer] [Integer] [x addq 2]]")

	core.InstallDef(r, "ff", fn1)
	if r.Lookup("ff") == nil {
		t.Fatal("fn def must install signatures")
	}
	// Overlapping redefinition drops the colliding overload.
	core.InstallDef(r, "ff", fn2)
	vals, _ := parser.Parse("ff 1")
	out, err := core.NewTop(r).Run(vals)
	if err != nil || core.Canon(out) != "3" {
		t.Errorf("redefined ff 1 = %s / %v, want 3", core.Canon(out), err)
	}
	// The frame-binding variant SHADOWS instead.
	core.InstallFrameBinding(r, "ff", fn1)
	vals, _ = parser.Parse("ff 1")
	out, err = core.NewTop(r).Run(vals)
	if err != nil || core.Canon(out) != "2" {
		t.Errorf("shadowed ff 1 = %s / %v, want 2", core.Canon(out), err)
	}
	// A conditional-body redefinition marks the program uncompilable
	// (a no-op off the compile pass — the binding still replaces).
	r.Check.CondBodyDepth++
	core.InstallDef(r, "ff", fn2)
	r.Check.CondBodyDepth--
	// A Function-parented value with no FnDefInfo installs nothing.
	core.InstallDef(r, "gg", core.NewValueRaw(core.TFunction, core.IntPayload{N: 1}))
	if r.Lookup("gg") != nil {
		t.Error("non-FnDefInfo function body must not install signatures")
	}
}
