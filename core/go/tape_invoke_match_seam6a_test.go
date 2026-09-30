package core

// Seam-6a wave A: direct in-package unit tests for the previously
// unreached guard arms in tape.go, invoke.go, and match.go (see
// design/TEST-SEAMS.10.md). All calls are direct — no seams needed;
// these are exported never-panic guards (ADR-005) plus two unexported
// matching-primitive skip arms.

import "testing"

// --- tape.go ---------------------------------------------------------------

func TestS6aNewTapeNegativeHeadroom(t *testing.T) {
	tp := NewTape(nil, -5)
	if tp == nil {
		t.Fatal("NewTape(nil, -5) returned nil")
	}
	if tp.Len() != 0 {
		t.Errorf("Len() = %d, want 0", tp.Len())
	}
	// The negative headroom must have been clamped, not subtracted.
	if tp.Cap() < DefaultTapeInitialFloor {
		t.Errorf("Cap() = %d, want >= %d", tp.Cap(), DefaultTapeInitialFloor)
	}
}

func TestS6aGrowthCeilingNeverBelowInitial(t *testing.T) {
	// An initial size beyond maxIntCap: the float ceiling exceeds
	// maxIntCap, capped clamps to maxIntCap, and the final guard raises
	// it back to the starting capacity. Reachable in production through
	// vmStackCeiling with a registry TapeConfig.InitialSize > maxIntCap
	// (no allocation on that path).
	initial := maxIntCap * 2
	if got := GrowthCeiling(initial, DefaultTapeMaxGrows, DefaultTapeGrowthFactor); got != initial {
		t.Errorf("growthCeiling(%d, ...) = %d, want %d", initial, got, initial)
	}
}

func TestS6aSpliceClampsIndexAndCount(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2)}, 0)

	// i beyond Len clamps to Len: the replacement appends at the end.
	tp.Splice(99, 1, NewInteger(7))
	if tp.Len() != 3 {
		t.Fatalf("Len() after Splice(99,1,v) = %d, want 3", tp.Len())
	}
	if n, err := AsInteger(tp.At(2)); err != nil || n != 7 {
		t.Errorf("At(2) = %v (err %v), want 7", tp.At(2), err)
	}

	// Negative count clamps to 0: nothing is deleted.
	tp.Splice(0, -5)
	if tp.Len() != 3 {
		t.Errorf("Len() after Splice(0,-5) = %d, want 3", tp.Len())
	}
	if n, err := AsInteger(tp.At(0)); err != nil || n != 1 {
		t.Errorf("At(0) = %v (err %v), want 1", tp.At(0), err)
	}
}

func TestS6aPrefixClampsEnd(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2)}, 0)
	got := tp.Prefix(99)
	if len(got) != 2 {
		t.Fatalf("Prefix(99) len = %d, want 2", len(got))
	}
	if n, err := AsInteger(got[1]); err != nil || n != 2 {
		t.Errorf("Prefix(99)[1] = %v (err %v), want 2", got[1], err)
	}
}

func TestS6aCopyRangeClampsNegativeStart(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2)}, 0)
	got := tp.CopyRange(-3, 1)
	if len(got) != 1 {
		t.Fatalf("CopyRange(-3,1) len = %d, want 1", len(got))
	}
	if n, err := AsInteger(got[0]); err != nil || n != 1 {
		t.Errorf("CopyRange(-3,1)[0] = %v (err %v), want 1", got[0], err)
	}
}

// --- invoke.go -------------------------------------------------------------

func TestS6aBodyTokensNonListBody(t *testing.T) {
	body := NewInteger(42)
	toks := BodyTokens(body)
	if len(toks) != 1 {
		t.Fatalf("bodyTokens(non-list) len = %d, want 1", len(toks))
	}
	if n, err := AsInteger(toks[0]); err != nil || n != 42 {
		t.Errorf("bodyTokens(non-list)[0] = %v (err %v), want 42", toks[0], err)
	}
	// Positive twin: a concrete list body returns its elements.
	lst := NewList([]Value{NewInteger(1), NewInteger(2)})
	if got := BodyTokens(lst); len(got) != 2 {
		t.Errorf("bodyTokens(list) len = %d, want 2", len(got))
	}
}

// --- match.go --------------------------------------------------------------

func TestS6aPatternsOkSkipsPositionBeyondMatched(t *testing.T) {
	// A pattern declared at position 1 while only one position matched:
	// patternsOk must skip it (idx >= len(positions)) rather than read
	// out of range.
	sig := &Signature{
		Args:     []*Type{TInteger, TInteger},
		Patterns: map[int]Value{1: NewInteger(0)},
	}
	tape := NewTape([]Value{NewInteger(5)}, 0)
	if !patternsOk(sig, []int{0}, tape, 0, nil, nil) {
		t.Error("patternsOk should skip a pattern position beyond len(positions)")
	}
}

func TestS6aPatternsOkSkipsNonConcreteForwardPattern(t *testing.T) {
	// A non-concrete (type-literal) pattern on a FORWARD position is
	// skipped — handlers may further constrain inside the body. The
	// value deliberately would NOT unify with the pattern, proving the
	// skip arm (not a successful unify) produced the true result.
	sig := &Signature{
		Args:     []*Type{TInteger},
		Patterns: map[int]Value{0: NewTypeLiteral(TInteger)},
	}
	tape := NewTape([]Value{NewString("not-an-integer")}, 0)
	if !patternsOk(sig, []int{0}, tape, 1, nil, nil) {
		t.Error("patternsOk should skip a non-concrete pattern on a forward position")
	}
	// Negative twin: the same pattern on a STACK position is enforced.
	if patternsOk(sig, []int{0}, tape, 0, nil, nil) {
		t.Error("patternsOk must enforce the pattern on a stack position")
	}
}
