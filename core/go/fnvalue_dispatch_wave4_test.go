package core

import (
	"strings"
	"testing"
)

// Coverage for the Function-value dispatch paths in engine.go:
// execFnDefLiteral (fn values at the pointer: forward collection, /v
// dispatch mods, anonymous vs named, foreign sub-registry wrappers),
// ExecFnDefSigStackMatch, execFnDefSig (body splice + CallBoru arms),
// compileFnDef, sigParamsCorrespond, trivialDelegationTarget,
// upcomingArgs, and the /u (stepWordUsurp) and /v (stepWordVal)
// modifier paths. Reuses covRegistry/runDifferential from
// compile_pipeline_cov_test.go.

// anonFnVal builds an anonymous Function value the way `afn`/`=>` does:
// Anonymous=true, authored sigs with a boru body. compileFnDef attaches
// the body-runner handler at dispatch time.
func anonFnVal(params []FnParam, returns []*Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	})
}

// namedFnVal builds a NON-anonymous FnDef value carrying its own authored
// sigs — the shape a fn literal stored in a map / module export has.
func namedFnVal(name string, params []FnParam, returns []*Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name: name,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	})
}

func parenBody(tokens ...Value) []Value {
	out := []Value{NewOpenParen()}
	out = append(out, tokens...)
	out = append(out, NewCloseParen())
	return out
}

// --- anonymous fn values ----------------------------------------------------

func TestAnonFnValueForwardDispatch(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("b")),
	)
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(2), NewInteger(3)})
	if err != nil {
		t.Fatalf("anon forward dispatch: %v", err)
	}
	if got := renderAll(out); got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestAnonFnValueStackDispatch(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cmul"), NewWord("a"), NewWord("b")),
	)
	out, err := NewTop(r).Run([]Value{NewInteger(6), NewInteger(7), fnv})
	if err != nil {
		t.Fatalf("anon stack dispatch: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestAnonFnValueMixedSplitDispatch(t *testing.T) {
	// One stack arg, one forward arg — the k=1 split of the unified rule.
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("a"), NewWord("b")),
	)
	out, err := NewTop(r).Run([]Value{NewInteger(10), fnv, NewInteger(32)})
	if err != nil {
		t.Fatalf("anon mixed dispatch: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestAnonFnValueZeroArgIsData(t *testing.T) {
	// A 0-arg lambda alone on the stack is DATA — it must not auto-fire.
	r := covRegistry(t, nil)
	fnv := anonFnVal(nil, nil, parenBody(NewInteger(99)))
	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("0-arg anon: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Errorf("0-arg lambda did not stay data: %s", renderAll(out))
	}
}

// applied returns a copy of a fn VALUE carrying `apply`'s one-shot
// application mark — what lang's applyHandler stamps to say a call was
// asked for at this site.
func applied(v Value) Value {
	fd := v.Data.(FnDefInfo)
	fd.Applied = true
	v.Data = fd
	return v
}

// TestAnonFnValueZeroArgAppliedDispatches is the positive twin of
// TestAnonFnValueZeroArgIsData: the SAME value, marked Applied, dispatches.
//
// The two together are the whole point of the mark (ADR-016, NUR077 §5
// Hole 1). The data gate above must stay — it is what makes
// `def f ([] => [body])` bind the FUNCTION and what holds a lambda in a
// container — but it keyed on Anonymous alone, so it also decided whether an
// EXPLICIT apply fired, and a named 0-arg twin behaved differently. The mark
// separates "is this data?" from "did someone ask to call it?" without
// weakening either answer.
func TestAnonFnValueZeroArgAppliedDispatches(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := applied(anonFnVal(nil, nil, parenBody(NewInteger(99))))
	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("applied 0-arg anon: %v", err)
	}
	if got := renderAll(out); got != "99" {
		t.Errorf("got %q, want 99 — the Applied mark must make the gate yield", got)
	}
}

// TestAppliedMarkIsOneShot pins that the mark is SPENT by the re-step that
// reads it, whether or not that re-step dispatches. A macro is the case that
// makes this observable: applying a macro is never a stack-value dispatch
// (design/legacy/MACROS-PHASE1.10.ignore §5, D4), so the value parks — and it must park
// UNMARKED, or the stale flag rides into whatever binding or container takes
// it and fires a call at some later, unrelated step.
func TestAppliedMarkIsOneShot(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := anonFnVal(nil, nil, parenBody(NewInteger(99)))
	fd := fnv.Data.(FnDefInfo)
	fd.Macro = true
	fd.Applied = true
	fnv.Data = fd

	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("applied macro value: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Fatalf("a macro value must park as data, got %s", renderAll(out))
	}
	if got, ok := out[0].Data.(FnDefInfo); !ok || got.Applied {
		t.Errorf("the Applied mark must not survive the re-step, got %+v", out[0])
	}
}

func TestQuotedFnValueIsData(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}},
		nil,
		parenBody(NewWord("a")),
	)
	fnv.Quoted = true
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(5)})
	if err != nil {
		t.Fatalf("quoted fn: %v", err)
	}
	// The quoted value never dispatched; both it and 5 remain.
	if len(out) != 2 || !out[0].Parent.Equal(TFunction) {
		t.Errorf("quoted fn dispatched: %s", renderAll(out))
	}
}

func TestFnValueDispatchModLeavesInert(t *testing.T) {
	// A Word/__DM marker right after the fn value marks it inert data.
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "a", Type: TInteger}},
		nil,
		parenBody(NewWord("a")),
	)
	out, err := NewTop(r).Run([]Value{
		fnv, NewDispatchMod(DispatchModInfo{Val: true}), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("dispatch mod: %v", err)
	}
	if len(out) != 2 || !out[0].Parent.Equal(TFunction) || !out[0].Quoted {
		t.Errorf("dispatch mod did not leave fn inert: %s", renderAll(out))
	}
}

func TestDanglingDispatchModDropped(t *testing.T) {
	// A __DM marker after a NON-function value is a no-op: dropped.
	r := covRegistry(t, nil)
	out, err := NewTop(r).Run([]Value{
		NewInteger(7), NewDispatchMod(DispatchModInfo{Val: true}),
	})
	if err != nil {
		t.Fatalf("dangling mod: %v", err)
	}
	if got := renderAll(out); got != "7" {
		t.Errorf("got %q, want 7", got)
	}
}

// --- named (non-anonymous) fn values -----------------------------------------

func TestNamedFnValueBodyDispatchForward(t *testing.T) {
	// A named handler-less fn value collects forward args, then takes the
	// legacy stack-match path: ExecFnDefSigStackMatch → execFnDefSig body
	// splice (FrameOpen + named-param bindings + AppendFrameTail).
	r := covRegistry(t, nil)
	fnv := namedFnVal("dbl",
		[]FnParam{{Name: "x", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("x"), NewWord("x")),
	)
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(4)})
	if err != nil {
		t.Fatalf("named fn forward: %v", err)
	}
	if got := renderAll(out); got != "8" {
		t.Errorf("got %q, want 8", got)
	}
}

func TestNamedFnValueBodyDispatchStack(t *testing.T) {
	r := covRegistry(t, nil)
	fnv := namedFnVal("dbl",
		[]FnParam{{Name: "x", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("cadd"), NewWord("x"), NewWord("x")),
	)
	out, err := NewTop(r).Run([]Value{NewInteger(21), fnv})
	if err != nil {
		t.Fatalf("named fn stack: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestNamedFnValueUnnamedParams(t *testing.T) {
	// Unnamed params push into the body frame in i-order (args[0] at the
	// bottom); the body word consumes them from the frame stack.
	r := covRegistry(t, nil)
	fnv := namedFnVal("sum2",
		[]FnParam{{Type: TInteger}, {Type: TInteger}},
		[]*Type{TInteger},
		[]Value{NewWord("cadd")},
	)
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(5), NewInteger(7)})
	if err != nil {
		t.Fatalf("unnamed params: %v", err)
	}
	if got := renderAll(out); got != "12" {
		t.Errorf("got %q, want 12", got)
	}
}

func TestNamedFnValueReturnTypeMismatch(t *testing.T) {
	// Declared String return, Integer body result: the spliced
	// ReturnCheck raises the fn return contract error.
	r := covRegistry(t, nil)
	fnv := namedFnVal("badret",
		[]FnParam{{Name: "x", Type: TInteger}},
		[]*Type{TString},
		parenBody(NewWord("x")),
	)
	_, err := NewTop(r).Run([]Value{fnv, NewInteger(1)})
	if err == nil {
		t.Fatal("return type mismatch not raised")
	}
	if !strings.Contains(err.Error(), "return") {
		t.Errorf("error does not mention return: %v", err)
	}
}

func TestNamedFnValueZeroParamBody(t *testing.T) {
	// nArgs == 0 branch of ExecFnDefSigStackMatch → execFnDefSig with
	// no args: the body splices and runs immediately.
	r := covRegistry(t, nil)
	fnv := namedFnVal("konst", nil, []*Type{TInteger}, parenBody(NewInteger(11)))
	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("0-param named fn: %v", err)
	}
	if got := renderAll(out); got != "11" {
		t.Errorf("got %q, want 11", got)
	}
}

func TestNamedFnValueCapturedBindings(t *testing.T) {
	// Lexical captures install as frame bindings before params.
	r := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{
		Name: "plusk",
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru(parenBody(NewWord("cadd"), NewWord("x"), NewWord("k"))),
			BarrierPos: BarrierAllForward,
		}},
		Captured: []CapturedBinding{{Name: "k", Value: NewInteger(100)}},
	})
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(1)})
	if err != nil {
		t.Fatalf("captured binding: %v", err)
	}
	if got := renderAll(out); got != "101" {
		t.Errorf("got %q, want 101", got)
	}
}

func TestMacroFnValueIsData(t *testing.T) {
	// A macro FnDef at the pointer is a VALUE — applied only by name.
	r := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{
		Name:  "mmac",
		Macro: true,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TAny}},
			Impl:       Boru([]Value{NewWord("x")}),
			BarrierPos: BarrierAllForward,
		}},
	})
	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("macro value: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Errorf("macro value dispatched: %s", renderAll(out))
	}
}

func TestNamedFnValueByNameLookup(t *testing.T) {
	// A named fn value with NO own sigs resolves via registry lookup:
	// covRegistry installs ctwice, so a bare handle to it dispatches.
	r := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{Name: "ctwice"})
	out, err := NewTop(r).Run([]Value{fnv, NewInteger(9)})
	if err != nil {
		t.Fatalf("name lookup dispatch: %v", err)
	}
	if got := renderAll(out); got != "18" {
		t.Errorf("got %q, want 18", got)
	}
}

func TestUnresolvableFnValueIsData(t *testing.T) {
	// Neither own sigs nor a resolvable name: the value stays data.
	r := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{Name: "no_such_fn_xyz"})
	out, err := NewTop(r).Run([]Value{fnv})
	if err != nil {
		t.Fatalf("unresolvable fn value: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Errorf("unresolvable fn value not left as data: %s", renderAll(out))
	}
}

// --- failed dispatch ----------------------------------------------------------

func TestNamedFnValueFailedDispatchRaises(t *testing.T) {
	// A named fn value whose candidate args match no sig raises
	// uncalled_function at the dispatch site
	// (design/legacy/FN-VALUE-DISPATCH.0.ignore).
	r := covRegistry(t, nil)
	fnv := namedFnVal("intonly",
		[]FnParam{{Name: "x", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("x")),
	)
	_, err := NewTop(r).Run([]Value{NewString("nope"), fnv})
	if err == nil {
		t.Fatal("failed dispatch not raised")
	}
	if !strings.Contains(err.Error(), "uncalled_function") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNamedFnValueFailedDispatchForwardArgs(t *testing.T) {
	// Same detection for FORWARD candidate args (`Pkg.fn "bad"`).
	r := covRegistry(t, nil)
	fnv := namedFnVal("intonly",
		[]FnParam{{Name: "x", Type: TInteger}},
		[]*Type{TInteger},
		parenBody(NewWord("x")),
	)
	_, err := NewTop(r).Run([]Value{fnv, NewString("nope")})
	if err == nil {
		t.Fatal("failed forward dispatch not raised")
	}
	if !strings.Contains(err.Error(), "uncalled_function") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestAnonFnValueNoMatchStaysData(t *testing.T) {
	// An anonymous lambda with unmatchable args is NOT flagged — it may
	// be a legitimate higher-order operand.
	r := covRegistry(t, nil)
	fnv := anonFnVal(
		[]FnParam{{Name: "x", Type: TInteger}},
		nil,
		parenBody(NewWord("x")),
	)
	out, err := NewTop(r).Run([]Value{NewString("s"), fnv})
	if err != nil {
		t.Fatalf("anon no-match: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("anon no-match altered stack: %s", renderAll(out))
	}
}

// --- foreign sub-registry dispatch ---------------------------------------------

func TestForeignRegistryFnValueCallBoru(t *testing.T) {
	// An UNNAMED body-bearing fn value carrying a foreign sub-registry:
	// the body must run via CallBoru in THAT registry, where its
	// module-private word resolves.
	sub := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "modonly",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					return []Value{NewInteger(n * 1000)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
	main := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{
		Registry: sub,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru(parenBody(NewWord("modonly"), NewWord("x"))),
			BarrierPos: BarrierAllForward,
		}},
	})
	out, err := NewTop(main).Run([]Value{fnv, NewInteger(3)})
	if err != nil {
		t.Fatalf("foreign CallBoru: %v", err)
	}
	if got := renderAll(out); got != "3000" {
		t.Errorf("got %q, want 3000", got)
	}
}

func TestForeignRegistryWrapperDispatch(t *testing.T) {
	// A NAMED fn value whose Registry points at a sub-registry where the
	// fn is INSTALLED: matchSignature resolves the inner definition, the
	// wrapper branch pairs the matched sig with the corresponding own sig
	// and dispatches via execMatch in the sub-registry.
	sub := covRegistry(t, nil)
	sig := Signature{
		Params:     []FnParam{{Name: "n", Type: TInteger}},
		Returns:    []*Type{TInteger},
		Impl:       Boru(parenBody(NewWord("cmul"), NewWord("n"), NewInteger(7))),
		BarrierPos: BarrierAllForward,
	}
	InstallFnDef(sub, "times7", FnDefInfo{Signatures: []Signature{sig}})

	main := covRegistry(t, nil)
	fnv := NewFunction(FnDefInfo{
		Name:       "times7",
		Registry:   sub,
		Module:     "test:mod",
		Signatures: []Signature{sig},
	})
	out, err := NewTop(main).Run([]Value{fnv, NewInteger(6)})
	if err != nil {
		t.Fatalf("foreign wrapper: %v", err)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestForeignRegistryNativeRefDispatch(t *testing.T) {
	// A named fn value with NO own sigs referencing a NATIVE in the
	// sub-registry: dispatches straight through execMatch with match.Reg
	// carrying the sub-registry.
	sub := covRegistry(t, nil)
	main, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	main.InitRootContext()
	fnv := NewFunction(FnDefInfo{Name: "cadd", Registry: sub})
	out, rErr := NewTop(main).Run([]Value{fnv, NewInteger(20), NewInteger(22)})
	if rErr != nil {
		t.Fatalf("foreign native ref: %v", rErr)
	}
	if got := renderAll(out); got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

// --- helper probes -------------------------------------------------------------

func TestSigParamsCorrespond(t *testing.T) {
	a := []FnParam{{Name: "x", Type: TInteger}, {Name: "y", Type: TString}}
	b := []FnParam{{Name: "x", Type: TInteger}, {Name: "y", Type: TString}}
	if !sigParamsCorrespond(a, b) {
		t.Error("identical params do not correspond")
	}
	if sigParamsCorrespond(a, b[:1]) {
		t.Error("different arity corresponds")
	}
	if sigParamsCorrespond(a, []FnParam{{Name: "x", Type: TInteger}, {Name: "z", Type: TString}}) {
		t.Error("different names correspond")
	}
	if sigParamsCorrespond(a, []FnParam{{Name: "x", Type: TInteger}, {Name: "y", Type: TInteger}}) {
		t.Error("different types correspond")
	}
	if sigParamsCorrespond(a, []FnParam{{Name: "x", Type: TInteger}, {Name: "y"}}) {
		t.Error("nil vs non-nil type corresponds")
	}
	if !sigParamsCorrespond([]FnParam{{Name: "x"}}, []FnParam{{Name: "x"}}) {
		t.Error("both-nil types do not correspond")
	}
}

func TestTrivialDelegationTarget(t *testing.T) {
	sig := &FnSig{Impl: Boru([]Value{NewWord("inner")})}
	name, ok := trivialDelegationTarget(sig)
	if !ok || name != "inner" {
		t.Errorf("delegation target = %q ok=%v", name, ok)
	}
	// Named param disqualifies.
	sig2 := &FnSig{Params: []FnParam{{Name: "x"}}, Impl: Boru([]Value{NewWord("inner")})}
	if _, ok := trivialDelegationTarget(sig2); ok {
		t.Error("named-param sig recognised as trivial delegation")
	}
	// Non-word single-token body disqualifies.
	sig3 := &FnSig{Impl: Boru([]Value{NewInteger(1)})}
	if _, ok := trivialDelegationTarget(sig3); ok {
		t.Error("non-word body recognised as trivial delegation")
	}
	// Multi-token body disqualifies.
	sig4 := &FnSig{Impl: Boru([]Value{NewWord("a"), NewWord("b")})}
	if _, ok := trivialDelegationTarget(sig4); ok {
		t.Error("multi-token body recognised as trivial delegation")
	}
}

func TestCompileFnDefIdempotent(t *testing.T) {
	r := covRegistry(t, nil)
	info := FnDefInfo{
		Name:      "anon",
		Anonymous: true,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "x", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru(parenBody(NewWord("x"))),
			BarrierPos: BarrierAllForward,
		}},
	}
	c1 := compileFnDef(r, info)
	if c1 == nil || len(c1.Signatures) != 1 {
		t.Fatalf("compileFnDef: %+v", c1)
	}
	if c1.Signatures[0].DispatchHandler() == nil {
		t.Error("anonymous compiled sig has no handler")
	}
	if c1.Signatures[0].BarrierPos != 1 {
		t.Errorf("barrier not resolved: %d", c1.Signatures[0].BarrierPos)
	}
	// Recompiling the compiled sigs is idempotent.
	c2 := compileFnDef(r, *c1)
	if c2.MaxForwardArgs != c1.MaxForwardArgs || len(c2.Signatures) != 1 {
		t.Errorf("recompile diverged: %+v vs %+v", c2, c1)
	}
	// Non-anonymous: no handler attached.
	info.Anonymous = false
	c3 := compileFnDef(r, info)
	if c3.Signatures[0].DispatchHandler() != nil {
		t.Error("non-anonymous compiled sig grew a handler")
	}
}

// --- /u usurp and /v ref modifier paths -----------------------------------------

func registerCsub(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "csub",
		Signatures: []Signature{{
			Args: []*Type{TInteger, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewInteger(b - a)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
}

func TestUsurpReversesArgOrder(t *testing.T) {
	r := covRegistry(t, registerCsub)
	// Plain forward: csub 3 10 → sig[0]=3, sig[1]=10 → 10-3 = 7.
	out, err := NewTop(r).Run([]Value{NewWord("csub"), NewInteger(3), NewInteger(10)})
	if err != nil {
		t.Fatalf("plain csub: %v", err)
	}
	plain := renderAll(out)
	if plain != "7" {
		t.Fatalf("plain csub = %q, want 7", plain)
	}
	// Usurped: same written order, reversed sig assignment → -7.
	out, err = NewTop(r).Run([]Value{NewWordUsurp("csub", false), NewInteger(3), NewInteger(10)})
	if err != nil {
		t.Fatalf("usurped csub: %v", err)
	}
	if got := renderAll(out); got != "-7" {
		t.Errorf("usurped csub = %q, want -7", got)
	}
}

func TestUsurpRefLeavesData(t *testing.T) {
	// /uv: the usurped wrapper stays inert data.
	r := covRegistry(t, registerCsub)
	out, err := NewTop(r).Run([]Value{NewWordUsurp("csub", true)})
	if err != nil {
		t.Fatalf("/uv: %v", err)
	}
	if len(out) != 1 || !IsFunctionRef(out[0]) {
		t.Errorf("/uv did not leave a function value: %s", renderAll(out))
	}
}

func TestUsurpUndefinedWordErrors(t *testing.T) {
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{NewWordUsurp("no_such_word", false)})
	if err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("usurp undefined: err = %v", err)
	}
}

func TestUsurpNonFunctionErrors(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "plainv", NewInteger(1))
	_, err := NewTop(r).Run([]Value{NewWordUsurp("plainv", false)})
	if err == nil || !strings.Contains(err.Error(), "illegal_ref") {
		t.Errorf("usurp non-fn: err = %v", err)
	}
}

func TestWordRefResolvesFunction(t *testing.T) {
	r := covRegistry(t, nil)
	out, err := NewTop(r).Run([]Value{NewWordRef("cadd")})
	if err != nil {
		t.Fatalf("/v: %v", err)
	}
	if len(out) != 1 || !IsFunctionRef(out[0]) {
		t.Errorf("/v did not push a function value: %s", renderAll(out))
	}
}

func TestWordRefUndefinedErrors(t *testing.T) {
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{NewWordRef("no_such_word")})
	if err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Errorf("/v undefined: err = %v", err)
	}
}

// TestWordValNonFunctionIsTheValue: `/v` is TOTAL over binding kinds —
// for a non-fn binding it is the identity, not an error. The negative
// twin is TestWordRefUndefinedErrors above: an UNBOUND name still
// declines, because there is no value to take.
func TestWordValNonFunctionIsTheValue(t *testing.T) {
	r := covRegistry(t, nil)
	InstallDef(r, "plainv", NewInteger(2))
	out, err := NewTop(r).Run([]Value{NewWordRef("plainv")})
	if err != nil {
		t.Fatalf("/v non-fn: unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("/v non-fn: got %v, want exactly one value", out)
	}
	if got, gerr := AsInteger(out[0]); gerr != nil || got != 2 {
		t.Errorf("/v non-fn: got %v (err %v), want 2", out[0], gerr)
	}
}

// --- check-mode fn-value dispatch -----------------------------------------------

// --- reorder hint on swapped forward args ----------------------------------------

func TestSwappedForwardArgsReorderHint(t *testing.T) {
	// A user fn with (Integer, String) called with (String, Integer): the
	// aggregate's Fallback sig matches, and the reorder probe raises the
	// did-you-swap hint built from reorderForwardCandidates +
	// assignsPositionally instead of the generic fallback error.
	setup := func(r *Registry) {
		InstallFnDef(r, "cpair", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}, {Name: "s", Type: TString}},
				Returns:    []*Type{TInteger},
				Impl:       Boru(parenBody(NewWord("n"))),
				BarrierPos: BarrierAllForward,
			}},
		})
	}
	r := covRegistry(t, setup)
	_, err := NewTop(r).Run([]Value{NewWord("cpair"), NewString("s"), NewInteger(1)})
	if err == nil {
		t.Fatal("swapped forward args did not error")
	}
	if !strings.Contains(err.Error(), "swap") {
		t.Errorf("no reorder hint in: %v", err)
	}

	// Stack-form swap: the misordered values sit below the word.
	r2 := covRegistry(t, setup)
	_, err = NewTop(r2).Run([]Value{NewInteger(1), NewString("s"), NewWord("cpair")})
	if err == nil {
		t.Fatal("swapped stack args did not error")
	}
	if !strings.Contains(err.Error(), "swap") {
		t.Errorf("no stack-form reorder hint in: %v", err)
	}
}
