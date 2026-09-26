package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cond_keep_test.go pins the recorder half of the KEPT `if` condition
// (NUR212's follow-up): a condition runs unconditionally, once, before its
// branch decides, so the binding it makes is kept. The recorder marks its
// fragment unconditional (CondBodyGuard), which the do-body adoption's root
// fence admits (rootLikeStream); a branch whose condition places bind twins
// moves its join's twins after the branch event so the stream replays them
// in the pass's order (takeJoinTwinsAfterCond); and an arm's nested
// condition defs store into the arm's carried cell (fragCanCarry /
// markArmBinds). lang's TestConditionBindingCompilesWithParity pins the
// whole path against the interpreter.

func TestCondBodyGuardMarksUnconditional(t *testing.T) {
	// Nil receiver: a no-op, like every EmitState method.
	var nilES *EmitState
	nilES.CondBodyGuard()()

	// Unarmed: the plain guard — no fragment opens.
	es := NewEmitState()
	end := es.CondBodyGuard()
	if len(es.frames) != 1 || len(es.fragUncond) != 0 {
		t.Fatalf("an unarmed condition guard must not open a fragment (frames=%d)", len(es.frames))
	}
	end()

	// Armed: the condition fragment opens, marked unconditional; the root
	// fence treats a position inside it as the root stream.
	es.ArmBranchCapture()
	end = es.CondBodyGuard()
	if len(es.frames) != 2 || len(es.fragUncond) != 1 || !es.fragUncond[0] {
		t.Fatalf("an armed condition guard must open one unconditional fragment (frames=%d uncond=%v)", len(es.frames), es.fragUncond)
	}
	if !es.rootLikeStream() {
		t.Fatal("a kept condition fragment at the root is root-like")
	}
	// An ARM fragment inside it is conditional: not root-like.
	es.ArmBranchCapture()
	arm := es.BodyAnalysisGuard()
	if es.rootLikeStream() {
		t.Fatal("an arm fragment is conditional, never root-like")
	}
	arm()
	es.TakeFragment()
	if !es.rootLikeStream() {
		t.Fatal("closing the arm returns to the condition fragment")
	}
	end()
	if es.TakeFragment() == nil || len(es.fragUncond) != 0 {
		t.Fatal("closing the condition guard captures its fragment and pops the mark")
	}
	// An open unit is never root-like.
	es.openUnitRecs = []int{0}
	if es.rootLikeStream() {
		t.Fatal("an open unit is a per-invocation body, never root-like")
	}
}

// TestAdoptBodyTwinsInsideKeptCondition: a `do` inside a kept condition
// adopts its body's twins where it stands — the condition runs once,
// unconditionally — but a `do` inside an ARM fragment still does not.
func TestAdoptBodyTwinsInsideKeptCondition(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := core.NewList([]core.Value{tokAt(1, 6)})
	keepRunTwin := func(es *EmitState) {
		end := es.KeepDefsBodyGuard(reg, "")
		es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "x", Depth: 1,
			Pos: core.SrcPos{Row: 1, Col: 6}}, core.DefEntry{Body: core.NewInteger(5)})
		end()
	}

	es := NewEmitState()
	es.ArmBranchCapture()
	end := es.CondBodyGuard()
	keepRunTwin(es)
	es.AdoptBodyTwins(body)
	end()
	frag := asFragment(es.TakeFragment())
	if !es.twinPlaced[0] || frag == nil || !fragPlacesTwin(frag) {
		t.Fatal("a do inside a kept condition must adopt its twin into the condition fragment")
	}

	es = NewEmitState()
	es.ArmBranchCapture()
	end = es.BodyAnalysisGuard()
	keepRunTwin(es)
	es.AdoptBodyTwins(body)
	end()
	es.TakeFragment()
	if es.twinPlaced[0] {
		t.Fatal("a do inside an arm fragment must not adopt (the root fence stands)")
	}
}

func TestTakeJoinTwinsAfterCond(t *testing.T) {
	twinEv := func(seq, idx int) EmitEvent {
		return EmitEvent{kind: evBindTwin, seq: seq, twin: &emitBindTwin{idx: idx}}
	}
	withTwin := &EmitFragment{startSeq: 5, events: []EmitEvent{twinEv(6, 1)}}
	noTwin := &EmitFragment{startSeq: 5, events: []EmitEvent{{kind: evDynBind, seq: 6, dyn: &emitDynBind{name: "x"}}}}

	frame := func() []EmitEvent {
		return []EmitEvent{{kind: evDynBind, seq: 3, dyn: &emitDynBind{name: "a"}}, twinEv(4, 0), twinEv(7, 2), twinEv(8, 3)}
	}
	es := NewEmitState()
	es.frames[0] = frame()
	if got := es.takeJoinTwinsAfterCond(nil); got != nil {
		t.Errorf("no condition fragment: nothing moves, got %v", got)
	}
	if got := es.takeJoinTwinsAfterCond(noTwin); got != nil {
		t.Errorf("a condition placing no twin: nothing moves, got %v", got)
	}
	got := es.takeJoinTwinsAfterCond(withTwin)
	if len(got) != 2 || got[0].seq != 7 || got[1].seq != 8 {
		t.Fatalf("the trailing join twins past the condition's opening move, got %v", got)
	}
	if len(es.frames[0]) != 2 || es.frames[0][1].seq != 4 {
		t.Fatalf("the twin noted before the condition stays, frame now %v", es.frames[0])
	}
	// No trailing twin past the opening: nothing moves.
	if got := es.takeJoinTwinsAfterCond(withTwin); got != nil {
		t.Errorf("no join twin trails the frame: nothing moves, got %v", got)
	}

	// A twin reached only through a nested branch's arm still counts.
	nested := &EmitFragment{startSeq: 5, events: []EmitEvent{{kind: evBranch, seq: 6,
		br: &emitBranch{then: &EmitFragment{events: []EmitEvent{twinEv(7, 1)}}}}}}
	if !fragPlacesTwin(nested) || fragPlacesTwin(nil) || fragPlacesTwin(noTwin) {
		t.Error("fragPlacesTwin: a twin in a nested fragment counts; none, or no fragment, does not")
	}
}

// TestArmCarriesNestedConditionDefs: an arm's nested condition def of the
// name is the arm's own — it counts toward carrying (fragBindsName /
// fragCanCarry) and is marked to store into the cell (markArmBinds); an
// unstorable one blocks the carry, so the read after the merge declines.
func TestArmCarriesNestedConditionDefs(t *testing.T) {
	es := NewEmitState()
	def := func(name string, v core.Value) EmitEvent {
		return EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1, val: v}}
	}
	condDef := def("x", core.NewInteger(2))
	arm := &EmitFragment{events: []EmitEvent{{kind: evBranch, br: &emitBranch{
		condFrag: &EmitFragment{events: []EmitEvent{condDef, def("y", core.NewInteger(1))}},
	}}}}
	if !fragBindsName(arm, "x") || fragBindsName(arm, "z") || fragBindsName(nil, "x") {
		t.Fatal("fragBindsName: a nested condition's def of the name counts; another name, or no fragment, does not")
	}
	if !fragCanCarry(es, arm, "x") {
		t.Fatal("an inert nested condition def carries the name")
	}
	if fragCanCarry(es, arm, "z") {
		t.Fatal("a name nothing binds is not carried")
	}
	markArmBinds(arm, "x", 3)
	got := arm.events[0].br.condFrag.events[0].dyn
	if !got.armCarried || got.armSlot != 3 {
		t.Fatal("the nested condition def must store into the carried cell")
	}
	if arm.events[0].br.condFrag.events[1].dyn.armCarried {
		t.Fatal("another name's def is left alone")
	}

	// An unstorable nested condition def (a carrier with no source to re-push)
	// blocks the carry.
	blocked := &EmitFragment{events: []EmitEvent{def("x", core.NewInteger(1)), {kind: evBranch, br: &emitBranch{
		condFrag: &EmitFragment{events: []EmitEvent{def("x", core.NewCarrier(core.TInteger))}},
	}}}}
	if fragCanCarry(es, blocked, "x") {
		t.Fatal("an unstorable nested condition def must block the carry")
	}
}
