package core

import "testing"

// zz_cover_pr521_defstack_test.go pins noteDefStack (NUR336's computed-def
// opener): after a def completes under an active analysis pass, the stack the
// engine steps the next token over is told to the recorder — and the notes go
// on over plain values up to the first token that does work; a run's first
// step only seats the count it starts from; outside a pass defsDone is 0.
func TestNoteDefStackAfterADef(t *testing.T) {
	r := covRegistry(t, nil)
	fin := r.Check.Begin()
	defer fin()
	rec := &mayBeFnRecorder{}
	r.Check.Emit = rec
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(3), NewInteger(5), NewWord("w")}, StackHeadroom)

	// No def completed: nothing is told.
	e.Pointer = 1
	e.noteDefStack(false)
	if len(rec.stacks) != 0 {
		t.Fatalf("no def, no note: %+v", rec.stacks)
	}

	// A run's first step seats a count already standing, and tells nothing.
	r.Check.RecordDef("k", SrcPos{Row: 1, Col: 1})
	e.noteDefStack(true)
	if len(rec.stacks) != 0 || e.defsSeen != 1 {
		t.Fatalf("the first step seats the count (%d) and tells nothing: %+v", e.defsSeen, rec.stacks)
	}

	// A def completing mid-run: the stack at the pointer is told, and a plain
	// value there keeps the notes going to the next token.
	r.Check.RecordDef("j", SrcPos{Row: 1, Col: 5})
	e.noteDefStack(false)
	if len(rec.stacks) != 1 || len(rec.stacks[0]) != 1 || !e.defNotePending {
		t.Fatalf("the stack beneath the pointer is told, notes pending: %+v pending=%v", rec.stacks, e.defNotePending)
	}
	e.Pointer = 2
	e.noteDefStack(false)
	if len(rec.stacks) != 2 || len(rec.stacks[1]) != 2 || e.defNotePending {
		t.Fatalf("a working token is told once and ends the notes: %+v pending=%v", rec.stacks, e.defNotePending)
	}
	e.noteDefStack(false)
	if len(rec.stacks) != 2 {
		t.Fatalf("no def since, no further note: %+v", rec.stacks)
	}

	// Outside an analysis pass the completed-def count reads 0, whatever
	// the field holds.
	if n := (&CheckState{DefsDone: 3}).defsDone(); n != 0 {
		t.Errorf("defsDone outside a pass = %d, want 0", n)
	}
	idle := &CheckState{}
	idle.RecordDef("k", SrcPos{Row: 1, Col: 1})
	if idle.DefsDone != 0 || idle.DefsInstalled != nil {
		t.Errorf("RecordDef outside a pass records nothing: %d %v", idle.DefsDone, idle.DefsInstalled)
	}
	if n := r.Check.defsDone(); n != 2 {
		t.Errorf("defsDone inside the pass = %d, want 2", n)
	}
}
