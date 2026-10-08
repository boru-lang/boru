package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// prefix_island_test.go pins the prefix island's (NUR210) decline arms, each
// with the verdict the arm exists to give; its compiling shapes are pinned on
// both lanes by lang's TestNUR210ComputedDoRunBeneathAndCollected.

// runChainStart: the run and the calls producing its operands, one
// contiguous block ending at the run — anything else is no chain.
func TestRunChainStartNeedsAContiguousCallBlock(t *testing.T) {
	call := func(seq int, ops ...EmitOperand) EmitEvent {
		return EmitEvent{seq: seq, kind: evCall, call: emitCall{nout: 1, ops: ops}}
	}
	if start, ok := runChainStart([]EmitEvent{call(1), call(2, EventOperand(1, 0))}, 1); !ok || start != 0 {
		t.Errorf("a producer just before the run starts the chain: %d %v", start, ok)
	}
	if _, ok := runChainStart([]EmitEvent{call(1), call(5), call(2, EventOperand(1, 0))}, 2); ok {
		t.Error("an unrelated event between the producer and the run breaks the chain")
	}
	branch := EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{}}
	if _, ok := runChainStart([]EmitEvent{branch, call(2, EventOperand(1, 0))}, 1); ok {
		t.Error("a producer that is not a call is no chain member")
	}
	if _, ok := runChainStart([]EmitEvent{call(2, EventOperand(9, 0))}, 0); ok {
		t.Error("an operand produced outside the block is no chain")
	}
}

// islandRun records a dyn-body run event (the `do` backstop's mark).
func islandRun(es *EmitState, ops ...EmitOperand) int {
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 1, ops: ops}})
	f := es.eventInfo[seq]
	f.dynBodyResult, f.variadicResult, f.dynBodyRun = true, true, true
	es.eventInfo[seq] = f
	return seq
}

// islandPrefixOperand and inertConst: only a scalar constant re-steps as
// itself, so only one rides the island's prefix.
func TestIslandPrefixIsAScalarConstant(t *testing.T) {
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{nout: 1}})
	produced := core.NewInteger(3)
	produced.ID = core.GenerateID(core.IDPrefixForType(produced.Parent))
	es.setProducedAt(produced, seq, 0)
	for _, v := range []core.Value{produced, core.NewCarrier(core.TInteger), core.NewList([]core.Value{core.NewInteger(1)})} {
		if _, ok := es.islandPrefixOperand(v); ok {
			t.Errorf("%v is no inert prefix", v)
		}
	}
	if op, ok := es.islandPrefixOperand(core.NewInteger(9)); !ok || op.kind != opConst {
		t.Errorf("a scalar constant is: %+v %v", op, ok)
	}
	es.consts = []core.Value{core.NewInteger(1), core.NewList(nil)}
	if es.inertConst(-1) || es.inertConst(2) || es.inertConst(1) || !es.inertConst(0) {
		t.Error("inertConst admits only an in-range scalar")
	}
}

// listIsland: the list's operands name the run first (top-first), then
// scalar constants; anything else is no island.
func TestListIslandShapes(t *testing.T) {
	es := NewEmitState()
	es.consts = []core.Value{core.NewInteger(9), core.NewList(nil)}
	r := islandRun(es)
	list := func(ops ...EmitOperand) int {
		return es.appendEvent(EmitEvent{kind: evCall, call: emitCall{makeList: true, nout: 1, ops: ops}})
	}
	list(EventOperand(r, 0), ConstOperand(0))
	events := es.frames[0]
	is, ok := es.listIsland(events, 1)
	if !ok || is.region != r || len(is.prefix) != 1 || is.anchor != r {
		t.Fatalf("[9 do (mk)] is an island over the run: %+v %v", is, ok)
	}
	es2 := NewEmitState()
	es2.consts = es.consts
	r2 := islandRun(es2)
	es2.appendEvent(EmitEvent{kind: evCall, call: emitCall{makeList: true, nout: 1, ops: []EmitOperand{ConstOperand(0), EventOperand(r2, 0)}}})
	if _, ok := es2.listIsland(es2.frames[0], 1); ok {
		t.Error("a list whose top element is not the run is no island")
	}
	es3 := NewEmitState()
	es3.consts = es.consts
	r3 := islandRun(es3)
	es3.appendEvent(EmitEvent{kind: evCall, call: emitCall{makeList: true, nout: 1, ops: []EmitOperand{EventOperand(r3, 0), ConstOperand(1)}}})
	if _, ok := es3.listIsland(es3.frames[0], 1); ok {
		t.Error("a container beneath the run is no inert prefix")
	}
	es4 := NewEmitState()
	r4 := islandRun(es4, EventOperand(99, 0))
	es4.appendEvent(EmitEvent{kind: evCall, call: emitCall{makeList: true, nout: 1, ops: []EmitOperand{EventOperand(r4, 0)}}})
	if _, ok := es4.listIsland(es4.frames[0], 1); ok {
		t.Error("a run whose operand comes from outside its block is no island")
	}
}

// residualIsland: the program residual [scalar constants…, run…] with the
// run the last event's; anything else is no island.
func TestResidualIslandShapes(t *testing.T) {
	es := NewEmitState()
	if _, ok := es.residualIsland(nil, nil); ok {
		t.Error("no events, no island")
	}
	other := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{nout: 1}})
	out := core.NewCarrier(core.TAny)
	es.setProducedAt(out, other, 0)
	if _, ok := es.residualIsland(es.frames[0], []core.Value{core.NewInteger(9), out}); ok {
		t.Error("a last event that is no run is no island")
	}
	es = NewEmitState()
	r := islandRun(es)
	run := core.NewCarrier(core.TAny)
	es.setProducedAt(run, r, 0)
	if _, ok := es.residualIsland(es.frames[0], []core.Value{run}); ok {
		t.Error("a run with nothing beneath needs no island")
	}
	if _, ok := es.residualIsland(es.frames[0], []core.Value{core.NewInteger(9)}); ok {
		t.Error("a residual the run does not end is no island")
	}
	if _, ok := es.residualIsland(es.frames[0], []core.Value{core.NewList(nil), run}); ok {
		t.Error("a container beneath the run is no inert prefix")
	}
	if is, ok := es.residualIsland(es.frames[0], []core.Value{core.NewInteger(9), run}); !ok || is.list != 0 || len(is.prefix) != 1 {
		t.Errorf("9 do (mk) is an island: %+v %v", is, ok)
	}
	forceOrder := map[int]bool{r: true}
	es.excuseIslandRegion([]core.Value{core.NewInteger(9), run}, forceOrder)
	if forceOrder[r] {
		t.Error("the island's run is excused from force-promotion")
	}
	es = NewEmitState()
	r = islandRun(es, EventOperand(99, 0))
	es.setProducedAt(run, r, 0)
	if _, ok := es.residualIsland(es.frames[0], []core.Value{core.NewInteger(9), run}); ok {
		t.Error("a run whose operand comes from outside its block is no island")
	}
}

// planPrefixIsland stands aside for another mark plan, and arms nothing over
// a program with no run.
func TestPlanPrefixIslandDefersToOtherPlans(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{markBefore: map[int]bool{1: true}}
	es.planPrefixIsland(lw, nil)
	if lw.island != nil {
		t.Error("another mark plan owns the frame")
	}
	lw = &lowerer{}
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{nout: 1}})
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{nout: 1}})
	es.planPrefixIsland(lw, nil)
	if lw.island != nil {
		t.Error("a program with no run arms nothing")
	}
}

// openIsland, islandTopIs and closeListIsland: the marks open only at the
// anchor; a stack that is not the window declines the list rather than
// taking the ordinary layout under open marks.
func TestIslandWindowChecks(t *testing.T) {
	(&lowerer{}).openIsland(&EmitEvent{seq: 1})
	is := &prefixIsland{anchor: 1, region: 2, list: 3, prefix: []EmitOperand{ConstOperand(0)}}
	(&lowerer{island: is, depth: 1}).openIsland(&EmitEvent{seq: 1})
	(&lowerer{island: is}).openIsland(&EmitEvent{seq: 7})
	run := []EmitOperand{EventOperand(2, 0)}
	for _, vm := range [][]vmSlot{
		{{seq: 2}},               // shorter than the window
		{{seq: 5}, {seq: 2}},     // the prefix's slot is an event's
		{nonEventSlot, {seq: 6}}, // the run's slot is another event's
	} {
		if (&lowerer{island: is, vm: vm}).islandTopIs(run) {
			t.Errorf("%+v is not the window", vm)
		}
	}
	if !(&lowerer{island: is, vm: []vmSlot{nonEventSlot, {seq: 2}}}).islandTopIs(run) {
		t.Error("the prefix's push under the run is the window")
	}
	list := &EmitEvent{seq: 3, kind: evCall, call: emitCall{makeList: true, ops: []EmitOperand{EventOperand(2, 0), ConstOperand(0)}}}
	if reason, closed := (&lowerer{island: is, vm: []vmSlot{{seq: 2}}}).closeListIsland(list); !closed || reason == "" {
		t.Errorf("a stack that is not the window declines the list: %q %v", reason, closed)
	}
	if _, closed := (&lowerer{island: is}).closeListIsland(&EmitEvent{seq: 4}); closed {
		t.Error("only the plan's list closes the island")
	}
	lw := &lowerer{island: &prefixIsland{prefix: []EmitOperand{ConstOperand(0), ConstOperand(1)}}, vm: []vmSlot{nonEventSlot}}
	if lw.verifyMarkWindow([]EmitOperand{ConstOperand(0)}) == "" {
		t.Error("a residual shorter than the prefix is not the window")
	}
	lw = &lowerer{island: is, vm: []vmSlot{{seq: 5}, {seq: 2}}}
	if lw.verifyMarkWindow([]EmitOperand{ConstOperand(0), EventOperand(2, 0)}) == "" {
		t.Error("a residual whose prefix slot is an event's is not the window")
	}
}

// opsHaveDynBodyRun and runOperand: a run reaches a consumer directly or
// through a branch arm; nothing else is one.
func TestRunOperandThroughBranches(t *testing.T) {
	if (&lowerer{}).opsHaveDynBodyRun([]EmitOperand{EventOperand(1, 0)}) {
		t.Error("with no recorder nothing is a run")
	}
	es := NewEmitState()
	r := islandRun(es)
	then := &EmitFragment{events: []EmitEvent{es.frames[0][0]}}
	br := es.appendEvent(EmitEvent{kind: evBranch, br: &emitBranch{then: then, thenOut: EventOperand(r, 0), hasThenOut: true}})
	lw := &lowerer{es: es, scopes: [][]EmitEvent{es.frames[0]}}
	if !lw.opsHaveDynBodyRun([]EmitOperand{ConstOperand(0), EventOperand(br, 0)}) {
		t.Error("a branch whose arm leaves a run is a run")
	}
	if lw.opsHaveDynBodyRun([]EmitOperand{EventOperand(77, 0)}) {
		t.Error("an operand no scope produces is no run")
	}
	if es.runOperand(EventOperand(br, 0), es.frames[0], 9) {
		t.Error("past the depth bound there is no claim")
	}
	plain := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{nout: 1}})
	if es.runOperand(EventOperand(plain, 0), es.frames[0], 0) {
		t.Error("an ordinary call is no run")
	}
}

// The island's compiling path in the compiler's own suite: the planner arms
// a list's island before the residual's, the anchor opens the marks and
// pushes the prefix, and the list closes with the re-step and the collect —
// a promoted list result stored to its slot.
func TestPrefixIslandArmsOpensAndCloses(t *testing.T) {
	es := NewEmitState()
	es.consts = []core.Value{core.NewInteger(9)}
	r := islandRun(es)
	l := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{makeList: true, nout: 1, ops: []EmitOperand{EventOperand(r, 0), ConstOperand(0)}}})
	lw := &lowerer{}
	es.planPrefixIsland(lw, nil)
	if lw.island == nil || lw.island.list != l {
		t.Fatalf("the list's island arms: %+v", lw.island)
	}
	es2 := NewEmitState()
	r2 := islandRun(es2)
	run := core.NewCarrier(core.TAny)
	es2.setProducedAt(run, r2, 0)
	lw2 := &lowerer{}
	es2.planPrefixIsland(lw2, []core.Value{core.NewInteger(9), run})
	if lw2.island == nil || lw2.island.list != 0 {
		t.Fatalf("the residual's island arms: %+v", lw2.island)
	}

	var code []Instr
	var debug []core.SrcPos
	lw = &lowerer{es: es, island: lw.island, code: &code, debug: &debug, promoted: map[int]int{l: 0}}
	lw.openIsland(&es.frames[0][0])
	if len(code) != 3 || code[0].Op != OpStackMark || code[1].Op != OpStackMark || code[2].Op != OpPushConst {
		t.Fatalf("a list's island opens two marks and pushes its prefix: %+v", code)
	}
	lw.vm = append(lw.vm, vmSlot{seq: r, idx: 0})
	if reason, closed := lw.closeListIsland(&es.frames[0][1]); !closed || reason != "" {
		t.Fatalf("the window closes the list: %q %v", reason, closed)
	}
	if n := len(code); code[n-3].Op != OpCallDynMixedFromMark || code[n-2].Op != OpMakeListToMark || code[n-1].Op != OpStoreLocal || len(lw.vm) != 0 {
		t.Errorf("the island re-steps, the outer mark collects, and the promoted list stores: %+v vm=%+v", code, lw.vm)
	}
}
