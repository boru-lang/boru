package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNoteLoopFreshNeedsALiveLoop pins NoteLoopFresh's guard (NUR214): no
// armed loop, an inactive recorder or a joined binding with no identity
// carries nothing.
func TestNoteLoopFreshNeedsALiveLoop(t *testing.T) {
	es := NewEmitState()
	j := core.NewCarrier(core.TAny)
	es.NoteLoopFresh("x", j)
	if es.carriedNames["x"] {
		t.Fatal("no armed loop: nothing carried")
	}
}

// TestRecordRuntimeDispatchUnplaceableSigForward pins the decline of an
// inline signature type's forward the compile cannot place (NUR231): a
// refinement with no compiled home makes the dispatch run-dependent, and
// the dispatch goes to RecordCall as the compile-time word it is (no
// install event); the latch is consumed.
func TestRecordRuntimeDispatchUnplaceableSigForward(t *testing.T) {
	es := NewEmitState()
	es.NoteRuntimeSigForward(&core.Type{}, core.NewCarrier(core.TInteger))
	es.RecordRuntimeDispatch("fn", &core.Signature{}, nil, nil, core.SrcPos{})
	if es.pendingSigForwards != nil {
		t.Error("the latch is consumed")
	}
	for _, ev := range es.frames[0] {
		if ev.call.typeRun != nil {
			t.Errorf("an unplaceable forward records no install event: %+v", ev)
		}
	}
}

// TestRunsBodyOnRegistryAtModuleScopeArms pins the registry-run body check
// (main's #509): a non-body argument is skipped, a body that rebinds a
// bound name (a def, a trailing def with no name, a computed name, a nested
// rebind, a paren body) refuses, and a fresh def passes.
func TestRunsBodyOnRegistryAtModuleScopeArms(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Defs.Push("x", core.NewInteger(1))
	es := NewEmitState()
	es.reg = reg
	sig := &core.Signature{NoEvalArgs: map[int]bool{1: true}}
	body := func(elems ...core.Value) core.Value { return core.NewList(elems) }
	def := core.NewWord("def")
	if !es.runsBodyOnRegistryAtModuleScope(sig, []core.Value{core.NewInteger(5), body(def, core.NewWord("y"), core.NewInteger(2))}) {
		t.Error("a fresh def over a non-body first argument bakes")
	}
	for name, b := range map[string]core.Value{
		"a bound name":    body(def, core.NewWord("x"), core.NewInteger(2)),
		"a trailing def":  body(core.NewInteger(1), def),
		"a computed name": body(def, core.NewList(nil), core.NewInteger(2)),
		"a nested rebind": body(body(def, core.NewWord("x"), core.NewInteger(2))),
		"a paren body":    core.NewParenExpr([]core.Value{def, core.NewWord("x"), core.NewInteger(2)}),
	} {
		if !es.bodyRebindsBoundName(b) {
			t.Errorf("%s: rebinds", name)
		}
	}
	if es.bodyRebindsBoundName(core.NewInteger(3)) {
		t.Error("a non-body value rebinds nothing")
	}
}

// TestValueRefsNameInAParen pins the paren arm of valueRefsName: a paren
// expression naming a word refs a name; one of literals does not.
func TestValueRefsNameInAParen(t *testing.T) {
	if !valueRefsName(core.NewParenExpr([]core.Value{core.NewInteger(1), core.NewWord("n")})) {
		t.Error("a paren naming a word refs a name")
	}
	if valueRefsName(core.NewParenExpr([]core.Value{core.NewInteger(1)})) {
		t.Error("a paren of literals refs none")
	}
}

// TestRecordLoopRefusesATypeRangeStart pins RecordLoop's range-operand
// check: a start or step that is neither a const, a local nor an event (a
// type operand) declines the loop.
func TestRecordLoopRefusesATypeRangeStart(t *testing.T) {
	es := NewEmitState()
	start := core.NewTypeLiteral(core.TInteger)
	start.ID = "T1"
	es.RecordLoop(start, core.NewInteger(3), core.NewInteger(1), &EmitFragment{}, nil, "", "i", core.Value{}, 0, core.SrcPos{})
	if es.Compilable {
		t.Fatal("a type range start declines")
	}
}

// TestLowerLoopEventStart pins lowerLoop's refusal of an event-sourced start
// or step the planner did not promote: its value is at its producer, not
// re-pushable at FOR_SETUP.
func TestLowerLoopEventStart(t *testing.T) {
	var code []Instr
	var debug []core.SrcPos
	lw := &lowerer{es: NewEmitState(), p: &Program{}, code: &code, debug: &debug}
	ev := &EmitEvent{kind: evLoop, loop: &emitLoop{start: EventOperand(3, 0), end: localOperand(0), step: localOperand(1), iterSlot: -1}}
	if reason := lw.lowerLoop(ev); reason != "for: computed range start/step (Stage 2 follow-on)" {
		t.Fatalf("got %q", reason)
	}
}
