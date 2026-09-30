package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// call_result_island_test.go pins the guards of the NUR334 call-result
// island's planning (call_result_island.go) that no whole program reaches:
// a unit index the emitter holds no record or fragment for, and a call
// positioned before every top-level token of the body. The lang suite drives the
// island end to end (TestLiveDeoptSpliceReturned, TestNUR334CallResultEdges).

func TestCallResultUnitMayCoupleGuards(t *testing.T) {
	es := &EmitState{fnRecs: []*fnUnitRec{nil, {}}}
	for _, unit := range []int{-1, 0, 1, 2} {
		if es.unitMayCouple(unit, map[int]bool{}) {
			t.Errorf("unit %d: a unit with no record or fragment couples nothing", unit)
		}
	}
	// A unit already seen is not walked again (a recursive unit).
	if es.unitMayCouple(1, map[int]bool{1: true}) {
		t.Error("a seen unit couples nothing")
	}
}

func TestCallResultPointUnplaced(t *testing.T) {
	es := &EmitState{}
	call := EmitEvent{kind: evCallUser, seq: 1, uc: emitUserCall{pos: core.SrcPos{Row: 1, Col: 1}}}
	w := core.NewWord("f")
	w.SetPos(core.SrcPos{Row: 2, Col: 1})
	if _, _, ok := es.callResultPoint(map[int]treeEvent{1: {ev: &call}}, 1, []core.Value{w}); ok {
		t.Error("a call before every top-level token has no call-result island")
	}
}

// TestTypeDeferredUnknownIndex: a type operand the emitter's type table does
// not hold defers nothing (typeDeferred's index guard, beside the NUR336
// late-start accounting TestNUR336LateStartEdges drives end to end).
func TestTypeDeferredUnknownIndex(t *testing.T) {
	es := &EmitState{}
	reads := func(string) int { return 1 }
	consumed := func(func(EmitOperand) bool, func(core.Value) bool, int) int { return 0 }
	for _, idx := range []int{-1, 0} {
		if es.typeDeferred(&fnUnitRec{}, &deoptPoint{}, idx, reads, consumed) {
			t.Errorf("type index %d: an unknown type defers nothing", idx)
		}
	}
}
