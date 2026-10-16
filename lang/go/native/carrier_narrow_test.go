package native

import "testing"

// TestApplyComplementNarrowing exercises the disjunction-subtraction
// path directly: a binding of Integer|String with a guard `x is
// Integer` should narrow x to String in the else branch (subtract
// the matched alternative).
func TestApplyComplementNarrowing(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.Check.Mode = true

	// Install a disjunct carrier on x: Integer|String.
	disjunct := NewDisjunct([]Value{
		NewTypeLiteral(TInteger),
		NewTypeLiteral(TString),
	})
	disjunct.SetCarrier(true)
	r.Defs.Push("x", disjunct)

	// Build a condition list simulating `x is Integer`.
	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TInteger),
	})

	restore := ApplyComplementNarrowing(r, cond)
	defer restore()

	if d := r.Defs.Depth("x"); d < 2 {
		t.Fatalf("expected pushed narrow entry, got stack depth %d", d)
	}
	top, _ := r.Defs.Top("x")
	// Narrowing produces a carrier (Parent=String, Carrier=true) —
	// check the Parent, not the value's own identity.
	if !top.Parent().Equal(TString) {
		t.Errorf("expected top to narrow to String, got %s", top.String())
	}
}

// TestApplyComplementNarrowingNoOpOnConcrete verifies that narrowing a
// plain Integer binding by `tnot String` is a no-op: Integer is
// disjoint from String, so `Integer tand (tnot String)` = Integer and
// the DefStack is left untouched. (The complement narrower no longer
// bails on non-disjuncts — it computes the real intersection and finds
// no refinement, which is the correct result here.)
func TestApplyComplementNarrowingNoOpOnConcrete(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.Check.Mode = true

	r.Defs.Push("x", NewCarrier(TInteger))

	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TString),
	})

	restore := ApplyComplementNarrowing(r, cond)
	defer restore()

	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("expected DefStack depth to remain 1, got %d", d)
	}
}

// TestApplyComplementNarrowingSupertypeGuard exercises the capability
// the old exact-alternative subtraction lacked: a guard whose type is a
// SUPERTYPE of an alternative. `x : Integer|String` with guard `x is
// Number` must narrow the else branch to String, because every Integer
// is a Number and is therefore excluded by `tnot Number`. The old code
// only removed alternatives whose type Equalled the guard exactly, so
// it would have left the disjunct unchanged.
func TestApplyComplementNarrowingSupertypeGuard(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.Check.Mode = true

	disjunct := NewDisjunct([]Value{
		NewTypeLiteral(TInteger),
		NewTypeLiteral(TString),
	})
	disjunct.SetCarrier(true)
	r.Defs.Push("x", disjunct)

	cond := NewEvalList([]Value{
		NewWord("x"),
		NewWord("is"),
		NewTypeLiteral(TNumber),
	})

	restore := ApplyComplementNarrowing(r, cond)
	defer restore()

	top, _ := r.Defs.Top("x")
	if !top.Parent().Equal(TString) {
		t.Errorf("expected x to narrow to String (Integer excluded as a Number), got %s", top.String())
	}
}
