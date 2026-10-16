package core

import (
	"strings"
	"testing"
)

// TestFailingTupleStopsAtEngineMarkers pins the no-match diagnostic's
// failing-tuple walks (sigError's written tuple and its check-time twin
// rematchWritten): an engine marker after the pointer — a fn frame's tail
// DefCleanup `__dc`, a Mark, a Move — ends the tuple, so a dispatch that
// no-matches at a body's end never lists `__dc (a __DC)` as the argument
// the caller supplied; a plain value before the marker is still collected.
func TestFailingTupleStopsAtEngineMarkers(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	dc := NewDefCleanup(DefCleanupInfo{})
	if !isEngineMarker(dc) || !isEngineMarker(NewMark("m")) || !isEngineMarker(NewMove("t", "r")) || isEngineMarker(NewInteger(5)) {
		t.Fatal("isEngineMarker: markers are markers, a value is not")
	}
	e.Tape = NewTape([]Value{NewWord("g"), NewInteger(3), dc, NewInteger(9)}, StackHeadroom)
	e.Pointer = 0
	if got := ReorderForwardCandidates(e.Tape, e.Pointer); len(got) != 1 {
		t.Errorf("the written tuple stops at the frame marker: %v", got)
	}
	if got := e.rematchWritten(nil); len(got) != 1 {
		t.Errorf("the check-time twin stops there too: %v", got)
	}
	e.Tape = NewTape([]Value{NewWord("g"), dc}, StackHeadroom)
	if got := ReorderForwardCandidates(e.Tape, 0); len(got) != 0 {
		t.Errorf("a marker right after the word leaves an empty tuple: %v", got)
	}
}

// TestWordPathNilHandlerIsAnError pins the ADR-005 guard in execMatch: a
// binding whose fn value was pushed as a plain def carries no runner on the
// WORD path (an un-installed body), and dispatching it by name is a clean
// internal_error rather than a nil-function panic (NUR125: the const-fold
// sub-run over `{a: k}` with k an Any param holding a 0-arg lambda).
func TestWordPathNilHandlerIsAnError(t *testing.T) {
	r := covRegistry(t, nil)
	raw := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{
		Impl: Boru([]Value{NewInteger(42)}),
	}}})
	r.Defs.Push("k", raw)
	e := NewTop(r)
	_, err := e.Run([]Value{NewWord("k")})
	if err == nil || !strings.Contains(err.Error(), "internal_error") || !strings.Contains(err.Error(), "no runnable implementation for `k`") {
		t.Fatalf("a nil handler on the word path must error cleanly, got %v", err)
	}
}
