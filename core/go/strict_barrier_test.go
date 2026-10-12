package core

// PROTOTYPE coverage for the strict-barrier rule (strictForwardBarrier /
// strandedForwardError): a function word beginning its own dispatch is
// always a forward-collection barrier, and a parked forward that cannot
// commit is reported stranded instead of waiting for the fn word's
// result. Tests are white-box and in-package, mirroring the
// commitBarrierForward seams in engine_seam7_barrier_test.go.

import (
	"strings"
	"testing"
)

// setStrict flips the prototype gate for one test and restores it.
func setStrict(t *testing.T, on bool) {
	t.Helper()
	prev := strictForwardBarrier
	strictForwardBarrier = on
	t.Cleanup(func() { strictForwardBarrier = prev })
}

// --- strandedForwardError direct seams ------------------------------------

func TestStrictBarrierInternalWordExempt(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	if err := e.strandedForwardError("__pa"); err != nil {
		t.Errorf("engine-internal boundary word must be exempt, got %v", err)
	}
}

func TestStrictBarrierNoPendingForward(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1)}, 1)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("no pending forward → nil, got %v", err)
	}
}

func TestStrictBarrierParenScopeStops(t *testing.T) {
	// A forward below an OpenParen is outside the current scope.
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0), NewOpenParen()}, 2)
	if err := e.strandedForwardError("cadd"); err != nil {
		t.Errorf("open-paren scope barrier → nil, got %v", err)
	}
}

func TestStrictBarrierParkedDefStrands(t *testing.T) {
	// There is NO def exemption: `def x add 1 2` must strand under the
	// strict rule. The fn-definition idiom survives structurally instead
	// — def's keyword signature (`[name/q fn/q sigs]`) matches at plan
	// time and never parks, so it never reaches this error path.
	e := engWithTape(t, []Value{fwdMarker("def", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("add"); err == nil {
		t.Error("a parked def waiting on a value slot must strand")
	}
}

func TestStrictBarrierStrandedError(t *testing.T) {
	e := engWithTape(t, []Value{fwdMarker("size", 0, 0, 0)}, 1)
	err := e.strandedForwardError("iota")
	if err == nil {
		t.Fatal("expected a stranded-forward error")
	}
	if err.Code != "signature_error" {
		t.Errorf("code = %q, want signature_error", err.Code)
	}
	for _, want := range []string{"size", "iota", "strict rule", "size (iota …)"} {
		if !strings.Contains(err.Detail, want) {
			t.Errorf("detail %q missing %q", err.Detail, want)
		}
	}
}

// --- the stepWord seam ------------------------------------------------------

// strictTape parks an uncommittable forward (its word is unregistered,
// so commitBarrierForward declines) below a dispatching "cadd".
func strictTape() []Value {
	return []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWord("cadd"), NewInteger(2), NewInteger(3),
	}
}

func TestStrictBarrierStepWordOff(t *testing.T) {
	setStrict(t, false)
	e := engWithTape(t, strictTape(), 3)
	if err := e.stepWord(e.Tape.Ref(3)); err != nil {
		t.Errorf("gate off keeps shipped behaviour, got %v", err)
	}
}

func TestStrictBarrierStepWordOn(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, strictTape(), 3)
	err := e.stepWord(e.Tape.Ref(3))
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWord")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}

// --- the stepWordUsurp seam -------------------------------------------------

func TestStrictBarrierStepWordUsurpOn(t *testing.T) {
	setStrict(t, true)
	vals := []Value{
		NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0),
		NewWordUsurp("cadd", false), NewInteger(2), NewInteger(3),
	}
	e := engWithTape(t, vals, 3)
	w, _ := AsWord(e.Tape.Ref(3))
	err := e.stepWordUsurp(e.Tape.At(3), w)
	if err == nil {
		t.Fatal("gate on: expected stranded-forward error from stepWordUsurp")
	}
	if !strings.Contains(err.Error(), "strict rule") {
		t.Errorf("error %q missing strict-rule detail", err.Error())
	}
}

// --- dot-access navigation is EXEMPT (structural, not a barrier) -------------

// registerCollector adds a 1-arg forward-collecting native and a trivial
// `dot` so a Reach can evaluate.
func registerCollector(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "collector",
		Signatures: []Signature{{
			Args:    []*Type{TAny},
			Impl:    Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil }),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "dot",
		Signatures: []Signature{{
			Args:      []*Type{TAtom, TAny},
			QuoteArgs: map[int]bool{0: true},
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(42)}, nil
			}),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
}

func TestStrictForwardScanReachExpands(t *testing.T) {
	// A dot-access chain is implicitly-parenthesized navigation, NOT a
	// Barrier: under strict the forward scan expands it into the collecting
	// word's slot exactly like a paren group (identical to gate-off).
	for _, on := range []bool{true, false} {
		setStrict(t, on)
		r := covRegistry(t, registerCollector)
		r.Defs.Push("x", NewInteger(0)) // receiver binding (dot ignores it)
		e := NewTop(r)
		reach := NewReach(ReachInfo{
			Receiver: []Value{NewWord("x")},
			Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
			Eval:     true,
		})
		e.Tape = NewTape([]Value{NewWord("collector"), reach}, StackHeadroom)
		e.Pointer = 0
		fn := r.Lookup("collector")
		if err := e.resolveForwardArgs(fn, WordInfo{Name: "collector", ArgCount: -1}); err != nil {
			t.Fatalf("strict=%v resolveForwardArgs: %v", on, err)
		}
		if IsReach(e.Tape.Ref(1)) {
			t.Errorf("strict=%v: the Reach must expand during forward collection (dot-access is exempt)", on)
		}
	}
}

// registerGgBnd adds a 1-and-2-arg `gg` (the 1-arg overload commits at a
// boundary) and a nullary boundary word `bnd`.
func registerGgBnd(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "gg",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(777)}, nil
			}), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(999)}, nil
			}), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "bnd",
		Signatures: []Signature{{
			Args: []*Type{}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(42)}, nil
			}), Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
}

func TestStrictCommitAtFunctionWordBoundary(t *testing.T) {
	setStrict(t, true)
	r := covRegistry(t, registerGgBnd)
	e := NewTop(r)
	// `gg 1 bnd` — gg collects 1, then the function word bnd is a barrier;
	// gg's 1-arg overload commits FIRST (→ 777), then bnd fires (→ 42).
	out, err := e.Run([]Value{NewWord("gg"), NewInteger(1), NewWord("bnd")})
	if err != nil {
		t.Fatalf("commit at the function-word boundary must succeed, got %v", err)
	}
	var saw777 bool
	for _, v := range out {
		if n, _ := AsInteger(v); n == 777 {
			saw777 = true
		}
	}
	if !saw777 {
		t.Errorf("gg's 1-arg overload must commit at the bnd barrier (777), got %v", out)
	}
}

func TestStrictStrandAtFunctionWordBoundary(t *testing.T) {
	setStrict(t, true)
	r := covRegistry(t, func(r *Registry) { registerGgBnd(r); registerCollector(r) })
	e := NewTop(r)
	// `collector bnd` — collector parks with 0 args, the function word bnd is
	// a barrier and collector cannot commit, so it strands.
	_, err := e.Run([]Value{NewWord("collector"), NewWord("bnd")})
	if err == nil {
		t.Fatal("a parked forward that cannot commit at a function-word barrier must strand")
	}
	if !strings.Contains(err.Error(), "still waiting") {
		t.Errorf("expected a strand error, got %v", err)
	}
}

func TestStrictDotAccessDoesNotStrand(t *testing.T) {
	// `collector x.k` — the dot-access is navigation, not a barrier, so its
	// value feeds the parked collector and the statement succeeds even under
	// strict (mirrors `size m.a`).
	setStrict(t, true)
	r := covRegistry(t, registerCollector)
	r.Defs.Push("x", NewInteger(0)) // receiver binding (dot ignores it)
	e := NewTop(r)
	reach := NewReach(ReachInfo{
		Receiver: []Value{NewWord("x")},
		Segments: []ReachSeg{{KeyLit: NewAtom("k")}},
		Eval:     true,
	})
	out, err := e.Run([]Value{NewWord("collector"), reach})
	if err != nil {
		t.Fatalf("a dot-access feeding a parked forward must NOT strand under strict, got %v", err)
	}
	var saw42 bool
	for _, v := range out {
		if n, _ := AsInteger(v); n == 42 {
			saw42 = true
		}
	}
	if !saw42 {
		t.Errorf("collector must receive the dot-access value (42), got %v", out)
	}
}
