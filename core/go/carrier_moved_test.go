package core

// Tests that travelled down with the carrier lattice (ADR-013,
// 2026-08-08 amendment). These were written against check/go's carrier
// machinery; the code they cover now lives in carrier_join.go,
// carrier_body.go, guard_narrow.go and record_typed_def.go, so the
// tests moved with it — a copy would double-count under the merged
// ADR-008 gate, so this is a MOVE, not a duplication.

import "testing"

func TestS6aObjectMakeSigNoMatchingOverload(t *testing.T) {
	r := w8reg(t)
	if objectMakeSig(r) != nil {
		t.Error("no make word registered: want nil")
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "make",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	if objectMakeSig(r) != nil {
		t.Error("make without an [Ideal Map] overload: want nil")
	}
}

func TestS6aIsNoneArmZeroValue(t *testing.T) {
	if isNoneArm(Value{}) {
		t.Error("the zero Value is not a None arm")
	}
}

func TestS6aExtractGuardClausesShortGuardFact(t *testing.T) {
	r := w8reg(t)
	cond := NewValueRaw(TBoolean, GuardFactInfo{Toks: []Value{NewWord("x")}})
	if got := extractGuardClauses(r, cond); got != nil {
		t.Errorf("a 1-token guard fact has no clauses, got %v", got)
	}
}

func TestS6aExtractGuardClausesBadWordPayload(t *testing.T) {
	r := w8reg(t)
	badWord := NewValueRaw(TWord, nil) // Word-parented, no WordInfo payload
	cond := NewList([]Value{badWord, NewWord("is"), NewTypeLiteral(TInteger)})
	if got := extractGuardClauses(r, cond); got != nil {
		t.Errorf("an AsWord failure skips the triplet, got %v", got)
	}
}

// A conjunction's else branch narrows nothing per-clause, and one exhaustive
// clause must NOT mark the else dead: `(x is Integer) and (y is String)` can
// still enter the else whenever y is not a String, even though x is always
// Integer. ApplyComplementNarrowing therefore acts only on a single-clause
// (pure `x is T`) condition and leaves a multi-clause conjunction's else
// untouched — no suppression, no narrowing. (In practice a compound
// condition's inner guards are captured as opaque ParenExpr values so
// extractGuardClauses returns 0 clauses; this pins the guard directly with a
// synthetic flat two-triple condition.)
func TestApplyComplementNarrowingConjunctionUntouched(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true
	// x is a concrete-typed Integer carrier so its `x is Integer` complement is
	// Never (the arm that, alone, must NOT kill the else); y is dynamic.
	r.Defs.Push("x", NewCarrier(TInteger))
	r.Defs.Push("y", NewDynamicCarrier(TAny))
	toks := []Value{
		NewWord("x"), NewWord("is"), NewTypeLiteral(TInteger),
		NewWord("and"),
		NewWord("y"), NewWord("is"), NewTypeLiteral(TString),
	}
	cond := NewCarrier(TBoolean)
	SetData(&cond, GuardFactInfo{Toks: toks})
	if n := len(extractGuardClauses(r, cond)); n != 2 {
		t.Fatalf("synthetic conjunction should yield 2 clauses, got %d", n)
	}
	before := r.Check.SuppressBodyErrors
	restore := ApplyComplementNarrowing(r, cond)
	if r.Check.SuppressBodyErrors != before {
		t.Error("a conjunction's else was marked dead from one exhaustive clause")
	}
	if xNow, _ := r.Defs.Top("x"); !ParentOf(&xNow).Equal(TInteger) {
		t.Error("x's else binding should be untouched for a conjunction")
	}
	restore()
	if r.Check.SuppressBodyErrors != before {
		t.Error("restore left SuppressBodyErrors unbalanced")
	}

	// SINGLE-clause control: a pure `x is Integer` guard DOES mark the else dead
	// (the else is genuinely unreachable), proving the gate is single-guard only.
	single := NewCarrier(TBoolean)
	SetData(&single, GuardFactInfo{Toks: []Value{NewWord("x"), NewWord("is"), NewTypeLiteral(TInteger)}})
	restore2 := ApplyComplementNarrowing(r, single)
	if r.Check.SuppressBodyErrors != before+1 {
		t.Error("a single exhaustive guard should mark the else dead")
	}
	restore2()
	if r.Check.SuppressBodyErrors != before {
		t.Error("restore2 left SuppressBodyErrors unbalanced")
	}
}

func TestS7SigSubsumesNilType(t *testing.T) {
	s1 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	s2 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	if sigSubsumes(s1, s2) {
		t.Error("a nil arg type must make sigSubsumes decline")
	}
}
