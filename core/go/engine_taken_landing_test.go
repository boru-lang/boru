package core

import "testing"

// takenEmit is the inactive recorder plus the taken-landing note (NUR349):
// it records the values noteCollectedLandings hands it.
type takenEmit struct {
	EmitRecorder
	active bool
	taken  map[string]bool
}

func (h *takenEmit) Active() bool             { return h.active }
func (h *takenEmit) NoteTakenLanding(v Value) { h.taken[v.ID] = true }
func newTakenEmit(active bool) *takenEmit {
	return &takenEmit{EmitRecorder: TheInactiveEmit, active: active, taken: map[string]bool{}}
}
func (h *takenEmit) was(v Value) bool { return h.taken[v.ID] }
func (h *takenEmit) count() int       { return len(h.taken) }
func stoodAside(e *Engine, vs ...Value) {
	e.Registry.Check.StoodAsideLandingIDs = map[string]bool{}
	for _, v := range vs {
		e.Registry.Check.StoodAsideLandingIDs[v.ID] = true
	}
}

// TestNoteCollectedLandings pins the dispatch-time note (NUR349): a value
// the step loop re-stepped with a collectable value after it and no landing
// (CheckState.StoodAsideLandingIDs) is TAKEN when the dispatch at the
// pointer takes it off the stack beneath another stack operand — `1 m.f 7
// add` takes the read and the 7. The topmost stack operand has nothing
// written after it to collect; a forward operand laid out beneath the word
// was written after the word; a quoted value is data; a value the step loop
// never stood aside is someone else's; an inactive recorder notes nothing.
func TestNoteCollectedLandings(t *testing.T) {
	read := NewDynamicCarrier(TAny)
	seven := NewInteger(7)
	add := WithPosAt(NewWord("add"), SrcPos{Row: 1, Col: 9})
	setup := func(active bool, tape ...Value) (*takenEmit, *Engine) {
		es := newTakenEmit(active)
		e := hazardEngine(t, es)
		e.Tape = NewTape(tape, StackHeadroom)
		e.Pointer = len(tape) - 1
		stoodAside(e, read)
		return es, e
	}

	es, e := setup(true, read, seven, add)
	e.noteCollectionHazards(nil, []int{0, 1})
	if !es.was(read) || es.count() != 1 {
		t.Errorf("the read beneath the 7 is taken: %v", es.taken)
	}

	// NEGATIVE: the read is the topmost stack operand.
	es, e = setup(true, seven, read, add)
	e.noteCollectionHazards(nil, []int{0, 1})
	if es.count() != 0 {
		t.Errorf("nothing written after the topmost operand: %v", es.taken)
	}

	// NEGATIVE: the 7 was collected forward and laid out beneath the word.
	es, e = setup(true, read, seven, add)
	e.fwdSplitAt, e.fwdSplitN, e.fwdSplitPos = 2, 1, add.Pos()
	e.noteCollectionHazards(nil, []int{0, 1})
	if es.count() != 0 {
		t.Errorf("a forward operand was written after the word: %v", es.taken)
	}

	// NEGATIVE: never stood aside, and quoted.
	es, e = setup(true, read, seven, add)
	stoodAside(e)
	e.noteCollectionHazards(nil, []int{0, 1})
	quoted := NewDynamicCarrier(TAny)
	quoted.Quoted = true
	stoodAside(e, quoted)
	e.Tape = NewTape([]Value{quoted, seven, add}, StackHeadroom)
	e.noteCollectionHazards(nil, []int{0, 1})
	if es.count() != 0 {
		t.Errorf("an unnoted or a quoted value is not taken: %v", es.taken)
	}

	// NEGATIVE: an inactive recorder.
	es, e = setup(false, read, seven, add)
	e.noteCollectedLandings([]int{0, 1})
	if es.count() != 0 {
		t.Errorf("an inactive recorder notes nothing: %v", es.taken)
	}
}
