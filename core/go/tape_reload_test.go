package core

import "testing"

// Reload clears only the previous program's logical cells — the prefix
// below the old gap and the tail above it — on the strength of the tape's
// storage invariant that every gap cell is zero (MoveGap, Remove and
// Splice zero what they vacate). These pins read the PHYSICAL buffer: after
// a reload nothing may survive outside the new program, whichever side of
// the old gap it sat on and whether or not the new program overwrites it.

func tapeStaleCells(tp *Tape, from int) []int {
	var stale []int
	for i := from; i < len(tp.buf); i++ {
		if tp.buf[i].Parent != nil { // a zero Value has no Parent; every live one has
			stale = append(stale, i)
		}
	}
	return stale
}

func TestTapeReloadClearsBothSidesOfTheOldGap(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4)}, 12)
	// Park the gap in the middle: logical cells end up on both sides.
	tp.Insert(2, NewInteger(9))
	tp.Remove(1)
	if tp.gapStart == 0 || tp.gapEnd == len(tp.buf) {
		t.Fatalf("fixture should leave cells on both sides of the gap: gap [%d,%d) of %d", tp.gapStart, tp.gapEnd, len(tp.buf))
	}
	if stale := tapeStaleCells(tp, 0); len(stale) != 4 {
		t.Fatalf("fixture holds %d live cells, want 4", len(stale))
	}
	// A SHORTER program: the old prefix beyond it and the old tail must go.
	if !tp.Reload([]Value{NewInteger(7)}) {
		t.Fatal("Reload declined a program that fits")
	}
	if stale := tapeStaleCells(tp, 1); stale != nil {
		t.Fatalf("stale cells survive the reload at physical %v", stale)
	}
	if n, _ := AsInteger(tp.At(0)); tp.Len() != 1 || n != 7 {
		t.Fatalf("reloaded tape = %v (len %d), want [7]", tp.Snapshot(), tp.Len())
	}
}

func TestTapeReloadLongerProgramAfterTakeAll(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1), NewInteger(2)}, 6)
	tp.TakeAll() // the end-of-Run handoff: gap at the end, the content a prefix
	// A LONGER program overwrites the whole old prefix: nothing to clear on
	// either side, and the tape reads exactly the new program.
	prog := []Value{NewInteger(5), NewInteger(6), NewInteger(7)}
	want := []int64{5, 6, 7}
	if !tp.Reload(prog) {
		t.Fatal("Reload declined a program that fits")
	}
	if stale := tapeStaleCells(tp, len(prog)); stale != nil {
		t.Fatalf("stale cells survive the reload at physical %v", stale)
	}
	for i := range prog {
		if n, _ := AsInteger(tp.At(i)); n != want[i] {
			t.Fatalf("reloaded tape[%d] = %v, want %d", i, tp.At(i), want[i])
		}
	}
}

// A reused tape's ceiling was derived from its FIRST program; a later,
// longer program that fits the buffer is granted the ceiling a fresh tape
// would have had (Codex P2 on #532), and a shorter one leaves a larger
// ceiling alone.
func TestTapeEnsureBoundsForNeverBelowFresh(t *testing.T) {
	short := []Value{NewInteger(1)}
	tp := NewTapeWith(short, TapeConfig{InitialSize: 4000, MaxGrows: 1, GrowthFactor: 2}, nil)
	if tp.MaxCap() != 8000 {
		t.Fatalf("fixture ceiling = %d, want 8000", tp.MaxCap())
	}
	long := make([]Value, 1500)
	for i := range long {
		long[i] = NewInteger(int64(i))
	}
	if !tp.Reload(long) {
		t.Fatal("Reload declined a program that fits")
	}
	tp.EnsureBoundsFor(len(long), TapeConfig{})
	fresh := NewTapeWith(long, TapeConfig{}, nil)
	if tp.MaxCap() != fresh.MaxCap() {
		t.Fatalf("reused ceiling = %d, want the fresh tape's %d", tp.MaxCap(), fresh.MaxCap())
	}
	// A shorter program on the same tape keeps the larger ceiling.
	if !tp.Reload(short) {
		t.Fatal("Reload declined the short program")
	}
	tp.EnsureBoundsFor(len(short), TapeConfig{})
	if tp.MaxCap() != fresh.MaxCap() {
		t.Fatalf("a shorter program lowered the ceiling to %d", tp.MaxCap())
	}
}
