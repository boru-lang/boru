package core

import "testing"

// The tape's running Forward count is what lets pendingForwardIdx skip its
// backward walk (engine.go): zero Forwards on the tape means no index can be
// returned. A count that drifts BELOW the truth is therefore a soundness bug,
// not a perf bug — the walk would be skipped while a real pending Forward sat
// on the tape, and the value at the pointer would be pushed instead of
// collected. These tests pin the count against a recount over the tape's
// actual contents after every mutating operation.

// recountForwards walks the tape's logical contents independently of the
// maintained counter — the oracle the invariant is checked against.
func recountForwards(t *Tape) int {
	n := 0
	for i := 0; i < t.Len(); i++ {
		if IsForward(t.Ref(i)) {
			n++
		}
	}
	return n
}

func assertForwardCount(t *testing.T, tp *Tape, step string) {
	t.Helper()
	if got, want := tp.forwards, recountForwards(tp); got != want {
		t.Fatalf("%s: tape.forwards = %d, recount = %d", step, got, want)
	}
	if got, want := tp.hasForward(), recountForwards(tp) > 0; got != want {
		t.Fatalf("%s: hasForward = %v, want %v", step, got, want)
	}
}

func fwdVal(name string) Value { return NewForward(ForwardInfo{FuncName: name, ExpectedArgs: 1}) }

func TestTapeForwardCountInvariant(t *testing.T) {
	// Construction seeds the count from the program.
	tp := NewTape([]Value{NewInteger(1), fwdVal("a"), NewInteger(2)}, StackHeadroom)
	if tp.forwards != 1 {
		t.Fatalf("NewTape seeded forwards = %d, want 1", tp.forwards)
	}
	assertForwardCount(t, tp, "construct")

	// Insert of a Forward and of a non-Forward.
	tp.Insert(0, fwdVal("b"))
	assertForwardCount(t, tp, "insert forward")
	tp.Insert(2, NewInteger(9))
	assertForwardCount(t, tp, "insert literal")

	// Set across all four transitions: fwd→fwd, fwd→lit, lit→fwd, lit→lit.
	tp.Set(0, fwdVal("c"))
	assertForwardCount(t, tp, "set forward over forward")
	tp.Set(0, NewInteger(7))
	assertForwardCount(t, tp, "set literal over forward")
	tp.Set(0, fwdVal("d"))
	assertForwardCount(t, tp, "set forward over literal")
	tp.Set(1, NewInteger(8))
	assertForwardCount(t, tp, "set literal over literal")

	// Remove of a Forward and of a non-Forward.
	tp.Remove(0)
	assertForwardCount(t, tp, "remove forward")
	tp.Remove(0)
	assertForwardCount(t, tp, "remove literal")

	// Splice: consuming Forwards, inserting Forwards, and both at once.
	tp = NewTape([]Value{fwdVal("e"), NewInteger(1), fwdVal("f"), NewInteger(2)}, StackHeadroom)
	assertForwardCount(t, tp, "splice fixture")
	tp.Splice(0, 2, NewInteger(5)) // consumes one Forward, inserts none
	assertForwardCount(t, tp, "splice consuming a forward")
	tp.Splice(0, 1, fwdVal("g"), fwdVal("h")) // inserts two Forwards
	assertForwardCount(t, tp, "splice inserting forwards")
	tp.Splice(0, 2, NewInteger(6), fwdVal("i")) // consumes two, inserts one
	assertForwardCount(t, tp, "splice consuming and inserting")

	// Reload re-seeds from the new program.
	if !tp.Reload([]Value{fwdVal("j"), NewInteger(1), fwdVal("k")}) {
		t.Fatal("Reload returned false on a large-enough buffer")
	}
	if tp.forwards != 2 {
		t.Fatalf("Reload re-seeded forwards = %d, want 2", tp.forwards)
	}
	assertForwardCount(t, tp, "reload")

	if !tp.Reload(nil) {
		t.Fatal("Reload(nil) returned false")
	}
	assertForwardCount(t, tp, "reload empty")
	if tp.hasForward() {
		t.Error("an empty tape must report no forward")
	}
}

// A nil tape answers hasForward without panicking: pendingForwardIdx consults
// it before any bounds check, and an engine can reach that probe with no tape
// bound yet.
func TestTapeHasForwardNilSafe(t *testing.T) {
	var tp *Tape
	if tp.hasForward() {
		t.Error("nil tape must report no forward")
	}
}

// pendingForwardIdx's fast path answers only the "no Forward anywhere" case.
// Its two remaining -1 arms need a Forward to EXIST while none is reachable
// below the pointer — the window that opens when the pointer rewinds below a
// parked Forward (engine.go's `e.Pointer = funcIdx` before
// rearrangeForForward). These drive the arms directly, since the fast path
// would otherwise hide them.
func TestPendingForwardIdxBarrierArmsWithForwardAbovePointer(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		tape []Value
		ptr  int
		want int
	}{
		// The scan reaches an OpenParen before any Forward: the paren is a
		// collection barrier, so the value at the pointer is NOT being
		// collected even though a Forward is parked further up the tape.
		{"open-paren barrier below, forward above",
			[]Value{NewOpenParen(), NewInteger(1), fwdVal("above")}, 2, -1},
		// The scan runs off the bottom of the tape without meeting either
		// sentinel — again -1, with the Forward above the pointer.
		{"no barrier below, forward above",
			[]Value{NewInteger(1), NewInteger(2), fwdVal("above")}, 2, -1},
		// The positive arm, pinned alongside so the fast path cannot start
		// answering -1 for a Forward that IS below the pointer.
		{"forward below the pointer is found",
			[]Value{NewInteger(1), fwdVal("here"), NewInteger(2)}, 2, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := New(r)
			e.Tape = NewTape(c.tape, StackHeadroom)
			e.Pointer = c.ptr
			if !e.Tape.hasForward() {
				t.Fatal("fixture must place a Forward on the tape")
			}
			if got := e.pendingForwardIdx(); got != c.want {
				t.Errorf("pendingForwardIdx() = %d, want %d", got, c.want)
			}
			if got, want := e.isInsidePendingForward(), c.want >= 0; got != want {
				t.Errorf("isInsidePendingForward() = %v, want %v", got, want)
			}
		})
	}
}

// MoveGap and grow relocate cells without changing the multiset, so neither
// may disturb the count. grow is driven by inserting past the initial size.
func TestTapeForwardCountSurvivesGapMovesAndGrowth(t *testing.T) {
	tp := NewTapeWith([]Value{fwdVal("a"), NewInteger(1)},
		TapeConfig{InitialSize: 4, MaxGrows: 4, GrowthFactor: 2}, nil)
	assertForwardCount(t, tp, "small fixture")

	for i := 0; i < 3; i++ {
		tp.MoveGap(0)
		tp.MoveGap(tp.Len())
	}
	assertForwardCount(t, tp, "after gap moves")

	// Push past the initial capacity so grow() reallocates the buffer.
	for i := 0; i < 32; i++ {
		tp.Insert(tp.Len(), NewInteger(int64(i)))
	}
	tp.Insert(tp.Len(), fwdVal("b"))
	assertForwardCount(t, tp, "after growth")
	if tp.Cap() <= 4 {
		t.Fatalf("tape did not grow: cap %d", tp.Cap())
	}
}
