package compiler

import (
	"slices"
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
	if _, _, ok := es.callResultPoint(map[int]treeEvent{1: {ev: &call}}, 1, []core.Value{w}, nil); ok {
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

// TestHeldReadPlanGuards pins the reads heldReadPlan declines to write as
// held values (NUR334, round 6): an event that is no restart read, a read at
// no token of the body, a read before the statement's token, a read whose
// token is a word rather than a reach, a read written after the call that a
// paren holds with another token, a read before the call that is not on the
// call's level, and a read whose token another event of the statement also
// stands at. The reads it writes — a top-level reach after the call, a paren
// holding the reach alone — are the positive half.
func TestHeldReadPlanGuards(t *testing.T) {
	at := func(col int) core.SrcPos { return core.SrcPos{Row: 1, Col: col} }
	tok := func(v core.Value, col int) core.Value { v.SetPos(at(col)); return v }
	reach := func(col int) core.Value {
		return tok(core.NewReachFromKeys(core.NewWord("c"), []core.Value{core.NewAtom("n")}), col)
	}
	// `k (c.n 5) f (c.n) c.n`, the call `f` at index 2.
	body := []core.Value{
		tok(core.NewWord("k"), 1),
		tok(core.NewParenExpr([]core.Value{reach(4), tok(core.NewInteger(5), 8)}), 3),
		tok(core.NewWord("f"), 11),
		tok(core.NewParenExpr([]core.Value{reach(14)}), 13),
		reach(19),
	}
	read := func(seq int, p core.SrcPos) *EmitEvent {
		return &EmitEvent{seq: seq, kind: evCall, call: emitCall{word: "dot", nout: 1, pos: p}}
	}
	run := []int{2}
	for _, c := range []struct {
		name  string
		ev    *EmitEvent
		tok   int
		after bool
		want  []int // nil: the read is declined
	}{
		{"a user call is no read", &EmitEvent{seq: 1, kind: evCallUser, uc: emitUserCall{pos: at(1)}}, 0, false, nil},
		{"a read at no token", read(1, core.SrcPos{}), 0, false, nil},
		{"a read before the statement", read(1, at(4)), 2, false, nil},
		{"a word, not a reach", read(1, at(1)), 0, false, nil},
		{"after the call, a paren holding more than it", read(1, at(4)), 0, true, nil},
		{"before the call, not on its level", read(1, at(4)), 0, false, nil},
		{"after the call, at the top level", read(1, at(19)), 0, true, []int{4}},
		{"after the call, a paren holding it alone", read(1, at(14)), 0, true, []int{3}},
	} {
		tree := map[int]treeEvent{c.ev.seq: {ev: c.ev}}
		sp, ok := heldReadPlan(tree, []int{c.ev.seq}, c.ev, body, c.tok, run, c.after)
		if ok != (c.want != nil) || (ok && !slices.Equal(sp.path, c.want)) {
			t.Errorf("%s: want %v, got %v %v", c.name, c.want, sp.path, ok)
		}
	}
	// Another event of the statement at the read's token declines the read.
	r, other := read(1, at(19)), read(2, at(19))
	if _, ok := heldReadPlan(map[int]treeEvent{1: {ev: r}, 2: {ev: other}}, []int{1, 2}, r, body, 0, run, true); ok {
		t.Error("a token two events stand at is not written as one read's value")
	}
}

// TestBoundReadValueGuards pins the reads boundReadValue declines (NUR334,
// round 6): a position no read stands at, one that two bindings' reads stand
// at, and a read whose binding no dyn-bind event of the tree records. A def
// of a scalar read once is the positive half; a def of a list is found but
// not written.
func TestBoundReadValueGuards(t *testing.T) {
	p := core.SrcPos{Row: 1, Col: 1}
	three := core.NewInteger(3)
	three.ID = "r1"
	bind := func(v core.Value) map[int]treeEvent {
		return map[int]treeEvent{1: {ev: &EmitEvent{seq: 1, kind: evDynBind, dyn: &emitDynBind{val: v, srcSeq: -1, src: EmitOperand{kind: opEvent}}}}}
	}
	if _, ok := boundReadValue(bind(three), map[string][]core.SrcPos{}, p); ok {
		t.Error("a position no read stands at is written as nothing")
	}
	if _, ok := boundReadValue(bind(three), map[string][]core.SrcPos{"r1": {p}, "r2": {p}}, p); ok {
		t.Error("a position two bindings' reads stand at is written as nothing")
	}
	if _, ok := boundReadValue(map[int]treeEvent{}, map[string][]core.SrcPos{"r1": {p}}, p); ok {
		t.Error("a read whose binding no event records is written as nothing")
	}
	if v, ok := boundReadValue(bind(three), map[string][]core.SrcPos{"r1": {p}}, p); !ok || v.String() != "3" {
		t.Errorf("a def-bound scalar read once is written as its value: %v %v", v, ok)
	}
	list := core.NewList(nil)
	list.ID = "r3"
	if _, ok := boundReadValue(bind(list), map[string][]core.SrcPos{"r3": {p}}, p); ok {
		t.Error("a compound binding is not written as a constant")
	}
}
