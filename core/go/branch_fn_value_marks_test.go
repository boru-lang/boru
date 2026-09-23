package core

import "testing"

// branch_fn_value_marks_test.go pins the recorder seams NUR159 / NUR187 gave
// the collapse (2026-09-23): the "might be callable" gate the re-step and
// leftover marks share asks the recorder's MayBeFn beside the static tests,
// fnReturnPark asks it before the braid, and stepEnd notes every statement
// boundary's position for the residual's apply arms.

// mayBeFnRecorder answers MayBeFn for a chosen set of ids and records the
// statement boundaries it is told about; everything else is the inactive
// no-op.
type mayBeFnRecorder struct {
	inactiveEmit
	maybe map[string]bool
	ends  []SrcPos
}

func (m *mayBeFnRecorder) MayBeFn(id string) bool    { return m.maybe[id] }
func (m *mayBeFnRecorder) NoteStatementEnd(p SrcPos) { m.ends = append(m.ends, p) }

// TestMightBeCallableAsksRecorder: a value neither fn-typed nor dynamic —
// a branch result whose merge widened the fn arm's type — is marked by
// markReStepped and markForwardLeftover exactly when the recorder says it
// may be a fn; the nil guards answer false without a registry, without
// check state or without an id.
func TestMightBeCallableAsksRecorder(t *testing.T) {
	branch := NewCarrier(TAny)
	branch.ID = "br-1"
	other := NewCarrier(TAny)
	other.ID = "br-2"
	rec := &mayBeFnRecorder{maybe: map[string]bool{"br-1": true}}
	e := &Engine{Registry: &Registry{Check: &CheckState{Emit: rec}}}
	for _, v := range []Value{branch, other} {
		e.markReStepped(v)
		e.markForwardLeftover(v)
	}
	cs := e.Registry.Check
	if !cs.ParenReSteppedFnIDs["br-1"] || !cs.ForwardLeftoverFnIDs["br-1"] {
		t.Errorf("a may-be-fn branch result is marked by both routes: re-stepped %v, leftover %v", cs.ParenReSteppedFnIDs, cs.ForwardLeftoverFnIDs)
	}
	if cs.ParenReSteppedFnIDs["br-2"] || cs.ForwardLeftoverFnIDs["br-2"] {
		t.Errorf("a value the recorder does not vouch for records nothing: re-stepped %v, leftover %v", cs.ParenReSteppedFnIDs, cs.ForwardLeftoverFnIDs)
	}
	if !e.mightBeCallable(NewCarrier(TFunction)) {
		t.Error("a fn-typed carrier is callable without asking the recorder")
	}
	noID := NewCarrier(TAny)
	for _, tc := range []struct {
		name string
		e    *Engine
		v    Value
	}{
		{"no registry", &Engine{}, branch},
		{"no check state", &Engine{Registry: &Registry{}}, branch},
		{"no id", e, noID},
		{"inactive recorder", &Engine{Registry: &Registry{Check: &CheckState{}}}, branch},
	} {
		if tc.e.recorderMayBeFn(tc.v) {
			t.Errorf("%s: recorderMayBeFn must answer false", tc.name)
		}
	}
}

// TestFnReturnParkAsksRecorder: a user paren collapsing to a lone
// may-be-fn branch result asks the braid to record the placement (the arm
// a fn-typed carrier or a dynamic value takes), and parks when the braid
// records it; the same value without the recorder's word is left to the
// main loop.
func TestFnReturnParkAsksRecorder(t *testing.T) {
	branch := NewCarrier(TAny)
	branch.ID = "br-1"
	rec := &mayBeFnRecorder{maybe: map[string]bool{"br-1": true}}
	asked := 0
	prev := CheckBraid.ParenPlacedFnCarrier
	defer func() { CheckBraid.ParenPlacedFnCarrier = prev }()
	CheckBraid.ParenPlacedFnCarrier = func(e *Engine, idx int) bool { asked++; return true }

	e := &Engine{Registry: &Registry{Check: &CheckState{Emit: rec}}, Tape: NewTape([]Value{branch}, 8)}
	if got := e.fnReturnPark(0, 2, true); got != 1 || asked != 1 {
		t.Errorf("a may-be-fn survivor asks the braid (%d) and parks: park = %d", asked, got)
	}
	rec.maybe = nil
	if got := e.fnReturnPark(0, 2, true); got != 0 || asked != 1 {
		t.Errorf("a value the recorder does not vouch for never asks (%d): park = %d", asked, got)
	}
}

// TestStepEndNotesBoundary: stepping a statement boundary under an active
// analysis pass hands its position to the recorder; outside analysis, or
// with no check state, nothing is noted.
func TestStepEndNotesBoundary(t *testing.T) {
	r := covRegistry(t, nil)
	fin := r.Check.Begin()
	defer fin()
	rec := &mayBeFnRecorder{}
	r.Check.Emit = rec
	end := NewEnd()
	end.pos = &SrcPos{Row: 3, Col: 9, Src: ";"}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), end}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
	if len(rec.ends) != 1 || rec.ends[0].Row != 3 || rec.ends[0].Col != 9 {
		t.Errorf("the boundary's position is noted once: %+v", rec.ends)
	}
	// Outside an analysis pass the recorder hears nothing.
	fin()
	e.Tape = NewTape([]Value{NewInteger(1), end}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
	if len(rec.ends) != 1 {
		t.Errorf("no note outside analysis: %+v", rec.ends)
	}
	// No check state at all: the guard, no panic.
	bare := &Engine{Registry: &Registry{}, Tape: NewTape([]Value{NewInteger(1), NewEnd()}, StackHeadroom), Pointer: 1}
	if err := bare.stepEnd(); err != nil {
		t.Fatalf("stepEnd without check state: %v", err)
	}
}
