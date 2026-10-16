package core

import (
	"fmt"
	"testing"
)

// tapeString renders a tape's logical cells.
func tapeString(tp *Tape) string { return fmt.Sprint(tp.Snapshot()) }

// TestTapeRuns pins the run-wise tape edits against their one-slice twins:
// a program or a replacement given as runs laid end to end loads, reloads
// and splices exactly as the runs concatenated would, each run copied once
// straight onto the tape, with the Forward count kept.
func TestTapeRuns(t *testing.T) {
	a := []Value{NewInteger(1), fwdVal("f")}
	b := []Value{NewInteger(2), NewInteger(3)}
	ab := append(append([]Value(nil), a...), b...)

	// Load.
	got := NewTapeWithRuns(TapeConfig{}, nil, a, nil, b)
	want := NewTapeWith(ab, TapeConfig{}, nil)
	if tapeString(got) != tapeString(want) {
		t.Fatalf("NewTapeWithRuns = %s, want %s", tapeString(got), tapeString(want))
	}
	assertForwardCount(t, got, "load")

	// Reload over a longer previous program clears what the new one leaves.
	got.Splice(4, 0, NewInteger(4), NewInteger(5), fwdVal("g"))
	if !got.ReloadRuns(b, a) {
		t.Fatal("a reload that fits declined")
	}
	if s := tapeString(got); s != fmt.Sprint(append(append([]Value(nil), b...), a...)) {
		t.Fatalf("ReloadRuns = %s", s)
	}
	assertForwardCount(t, got, "reload")
	small := &Tape{}
	if small.ReloadRuns(ab) {
		t.Error("a reload past the buffer's capacity was accepted")
	}

	// Splice: replace one cell with runs, as Splice with their concatenation.
	sp := NewTape([]Value{NewInteger(9), NewInteger(8), NewInteger(7)}, StackHeadroom)
	ref := NewTape([]Value{NewInteger(9), NewInteger(8), NewInteger(7)}, StackHeadroom)
	sp.SpliceRuns(1, 1, a, b)
	ref.Splice(1, 1, ab...)
	if tapeString(sp) != tapeString(ref) {
		t.Fatalf("SpliceRuns = %s, want %s", tapeString(sp), tapeString(ref))
	}
	assertForwardCount(t, sp, "splice")
	// Out-of-range arguments clamp as Splice's do.
	sp.SpliceRuns(-1, -1, b)
	ref.Splice(-1, -1, b...)
	sp.SpliceRuns(99, 5, a)
	ref.Splice(99, 5, a...)
	if tapeString(sp) != tapeString(ref) {
		t.Fatalf("clamped SpliceRuns = %s, want %s", tapeString(sp), tapeString(ref))
	}
	assertForwardCount(t, sp, "clamped splice")
	// A splice past the growth ceiling drops the edit and latches.
	full := NewTapeWith([]Value{NewInteger(1)}, TapeConfig{InitialSize: 1, MaxGrows: 1, GrowthFactor: 1}, nil)
	full.maxGrows = 0
	full.SpliceRuns(1, 0, a)
	if !full.Exhausted() || full.Len() != 1 {
		t.Errorf("an exhausted SpliceRuns wrote: len %d exhausted %v", full.Len(), full.Exhausted())
	}
}
