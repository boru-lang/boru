package core

import (
	"strings"
	"testing"
)

// whileCont builds a while-mode continuation over the given cond/body
// token slices — the shape basic's RunWhileLoop constructs.
func whileCont(r *Registry, cond, body []Value) *Loop {
	return &Loop{Registry: r, Body: body, WhileCond: cond}
}

// TestWhileMoveCondTruthySplicesBody — a truthy condition region flips
// the continuation into body phase and splices the body region (nil
// marks map exercises the lazy-init arm).
func TestWhileMoveCondTruthySplicesBody(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{NewBoolean(true)}, []Value{NewInteger(7)})
	move := NewMoveCont("m1", "while loop", cont)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("m1"), NewBoolean(true), move}, StackHeadroom)
	e.marks = nil
	if err := e.stepMoveCont(0, 2, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveWhile cond: %v", err)
	}
	if !cont.WhileInBody {
		t.Error("truthy condition must flip the continuation into body phase")
	}
	if e.marks == nil {
		t.Error("marks map should have been lazily initialised")
	}
	// The spliced region is mark + body + move: the body token must be
	// on the tape where the condition region was.
	found := false
	for i := 0; i < e.Tape.Len(); i++ {
		if v := e.Tape.At(i); v.Parent != nil && v.Parent.Equal(TInteger) {
			found = true
		}
	}
	if !found {
		t.Error("body region was not spliced in")
	}
}

// TestWhileMoveCondFalsyEndsWithResults — a falsy condition splices the
// accumulated results and ends the loop.
func TestWhileMoveCondFalsyEndsWithResults(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{NewBoolean(false)}, []Value{NewInteger(7)})
	cont.Results = []Value{NewInteger(41), NewInteger(42)}
	move := NewMoveCont("m1", "while loop", cont)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("m1"), NewBoolean(false), move}, StackHeadroom)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMoveCont(0, 2, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveWhile falsy: %v", err)
	}
	if e.Tape.Len() != 2 {
		t.Fatalf("expected the two accumulated results on the tape, got %d entries", e.Tape.Len())
	}
	if n, _ := AsInteger(e.Tape.At(1)); n != 42 {
		t.Errorf("results not spliced in order: %v", e.Tape.At(1))
	}
}

// TestWhileMoveCondNoValueErrors — an empty condition region is a loud
// runtime_error, and the loop region is removed.
func TestWhileMoveCondNoValueErrors(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{}, []Value{NewInteger(7)})
	move := NewMoveCont("m1", "while loop", cont)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("m1"), move}, StackHeadroom)
	e.marks = map[string]bool{"m1": true}
	err := e.stepMoveCont(0, 1, MoveInfo{To: "m1", Cont: cont})
	if err == nil || !strings.Contains(err.Error(), "condition produced no value") {
		t.Fatalf("want the no-value runtime_error, got %v", err)
	}
}

// TestWhileMoveBodyCollectsAndReplaysCond — a body region's values
// accumulate into Results and the condition region replays.
func TestWhileMoveBodyCollectsAndReplaysCond(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{NewBoolean(false)}, []Value{NewInteger(7)})
	cont.WhileInBody = true
	move := NewMoveCont("m1", "while loop", cont)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewMark("m1"), NewInteger(7), move}, StackHeadroom)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMoveCont(0, 2, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("stepMoveWhile body: %v", err)
	}
	if cont.WhileInBody {
		t.Error("body phase must flip back to condition phase")
	}
	if len(cont.Results) != 1 {
		t.Fatalf("body value not accumulated: %v", cont.Results)
	}
	if n, _ := AsInteger(cont.Results[0]); n != 7 {
		t.Errorf("wrong accumulated value: %v", cont.Results[0])
	}
}

// TestWhileTraceNotes — the trace arms of the while driver ("while
// cond" / "while body" / "while done" notes) run when a trace callback
// is installed.
func TestWhileTraceNotes(t *testing.T) {
	r := covRegistry(t, nil)
	cont := whileCont(r, []Value{NewBoolean(true)}, []Value{NewInteger(7)})
	move := NewMoveCont("m1", "while loop", cont)
	e := NewTop(r)
	e.SetTrace(func(int, int, []Value, string) {})
	e.Tape = NewTape([]Value{NewMark("m1"), NewBoolean(true), move}, StackHeadroom)
	e.marks = map[string]bool{"m1": true}
	if err := e.stepMoveCont(0, 2, MoveInfo{To: "m1", Cont: cont}); err != nil {
		t.Fatalf("traced cond step: %v", err)
	}
	if e.traceNote != "while body" {
		t.Errorf("trace note = %q, want %q", e.traceNote, "while body")
	}
	// Falsy finish under trace → "while done".
	cont2 := whileCont(r, []Value{NewBoolean(false)}, nil)
	move2 := NewMoveCont("m2", "while loop", cont2)
	e2 := NewTop(r)
	e2.SetTrace(func(int, int, []Value, string) {})
	e2.Tape = NewTape([]Value{NewMark("m2"), NewBoolean(false), move2}, StackHeadroom)
	e2.marks = map[string]bool{"m2": true}
	if err := e2.stepMoveCont(0, 2, MoveInfo{To: "m2", Cont: cont2}); err != nil {
		t.Fatalf("traced falsy step: %v", err)
	}
	if e2.traceNote != "while done" {
		t.Errorf("trace note = %q, want %q", e2.traceNote, "while done")
	}
}
