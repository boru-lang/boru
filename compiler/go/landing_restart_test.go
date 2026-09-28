package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// landing_restart_test.go pins the statement island's planning helpers
// (landing_restart.go, NUR242 / NUR219): where a statement begins, what a
// second run may repeat, how the island's prefix is found, and when the walk
// seats it. The lang suite drives the planners end to end
// (TestNUR242StatementIslands, TestNUR219QuoteCapturesCollectedWord).

func restartTok(row, col int) core.Value {
	w := core.NewWord("t")
	w.SetPos(core.SrcPos{Row: row, Col: col})
	return w
}

func TestStatementToken(t *testing.T) {
	end := core.NewEnd()
	end.SetPos(core.SrcPos{Row: 1, Col: 3})
	body := []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7)}
	if got := statementToken(body, core.SrcPos{Row: 1, Col: 7}); got != 2 {
		t.Errorf("a statement begins after the `end` before it: got %d", got)
	}
	if got := statementToken(body, core.SrcPos{Row: 1, Col: 1}); got != 0 {
		t.Errorf("the first statement begins at the body's first token: got %d", got)
	}
	if got := statementToken(body, core.SrcPos{Row: 0, Col: 0}); got != -1 {
		t.Errorf("a position no token holds has no statement: got %d", got)
	}
}

func TestRestartRead(t *testing.T) {
	pure := &core.Signature{CompileEffect: core.CompileIslandPure}
	for _, c := range []struct {
		name string
		ev   EmitEvent
		want bool
	}{
		{"a member read", EmitEvent{kind: evCall, call: emitCall{word: "dot", poly: true}}, true},
		{"a get", EmitEvent{kind: evCall, call: emitCall{word: "get"}}, true},
		{"a stack shuffle", EmitEvent{kind: evCall, call: emitCall{word: "drop"}}, true},
		{"a list's assembly", EmitEvent{kind: evCall, call: emitCall{makeList: true}}, true},
		{"an island-pure word", EmitEvent{kind: evCall, call: emitCall{word: "size", sig: pure}}, true},
		{"an island-pure word dispatched poly", EmitEvent{kind: evCall, call: emitCall{word: "size", sig: pure, poly: true}}, false},
		{"another native", EmitEvent{kind: evCall, call: emitCall{word: "print"}}, false},
		{"a shaped apply", EmitEvent{kind: evCall, call: emitCall{word: "dot", dynMethod: &DynMethodSpec{}}}, false},
		{"a list re-step", EmitEvent{kind: evCall, call: emitCall{makeList: true, listReStep: &ListReStepSpec{}}}, false},
		{"a user call", EmitEvent{kind: evCallUser}, false},
	} {
		if got := restartRead(&c.ev); got != c.want {
			t.Errorf("%s: restartRead = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMethodSeqAndContains(t *testing.T) {
	if got := methodSeq(&EmitEvent{kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opEvent, idx: 4}}}}); got != 4 {
		t.Errorf("the apply's first operand names its method: got %d", got)
	}
	if methodSeq(&EmitEvent{kind: evCall}) != -1 || methodSeq(&EmitEvent{kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opLocal}}}}) != -1 {
		t.Error("an apply with no event method names none")
	}
	if !containsInt([]int{1, 2}, 2) || containsInt([]int{1, 2}, 3) {
		t.Error("containsInt")
	}
	if !outsProducedBefore([]EmitOperand{{kind: opEvent, idx: 2}}, 3) || outsProducedBefore([]EmitOperand{{kind: opEvent, idx: 3}, {kind: opConst}}, 3) {
		t.Error("outsProducedBefore reads the residual's event results recorded before the statement")
	}
}

// restartTree is a root tree: a loop (seq 9, index slot 5) whose body holds
// the landed member read (seq 2) over op, then a later event after.
func restartTree(op EmitOperand, after EmitEvent) (map[int]treeEvent, []core.Value) {
	read := EmitEvent{kind: evCall, seq: 2, call: emitCall{word: "dot", ops: []EmitOperand{op}, pos: core.SrcPos{Row: 1, Col: 10}}}
	after.seq = 3
	loop := EmitEvent{kind: evLoop, seq: 9, loop: &emitLoop{iterSlot: 5, pos: core.SrcPos{Row: 1, Col: 1}, body: &EmitFragment{events: []EmitEvent{read, after}}}}
	body := []core.Value{restartTok(1, 1), restartTok(1, 10), restartTok(1, 20)}
	return rootTreeEvents([]EmitEvent{loop}, false), body
}

func TestRestartSpanReruns(t *testing.T) {
	start := core.SrcPos{Row: 1, Col: 1}
	after := func(kind int) EmitEvent {
		return EmitEvent{kind: kind, call: emitCall{word: "print", pos: core.SrcPos{Row: 1, Col: 15}}, dyn: &emitDynBind{pos: core.SrcPos{Row: 1, Col: 15}}}
	}
	tree, body := restartTree(EmitOperand{kind: opLocal, idx: 1}, after(evCall))
	if !tree[2].inLoop || len(tree[2].loopSlots) != 1 || tree[2].loopSlots[0] != 5 {
		t.Fatalf("the loop's body events are marked: %+v", tree[2])
	}
	if !restartSpanReruns(tree, start, len(body), body, 2) {
		t.Error("an effect after the stop, over a loop-invariant read, restarts on the first iteration")
	}
	before := EmitEvent{kind: evCall, seq: 1, call: emitCall{word: "print", pos: core.SrcPos{Row: 1, Col: 5}}}
	withBefore := map[int]treeEvent{1: {ev: &before, inLoop: true, loopSlots: []int{5}}}
	for seq, te := range tree {
		withBefore[seq] = te
	}
	if restartSpanReruns(withBefore, start, len(body), body, 2) {
		t.Error("an effect before the stop ran on the stopping iteration")
	}
	bind, bbody := restartTree(EmitOperand{kind: opLocal, idx: 1}, after(evDynBind))
	if restartSpanReruns(bind, start, len(bbody), bbody, 2) {
		t.Error("a bind in the span may change what the stop reads on a later iteration")
	}
	loopVar, lbody := restartTree(EmitOperand{kind: opLocal, idx: 5}, after(evCall))
	if restartSpanReruns(loopVar, start, len(lbody), lbody, 2) {
		t.Error("a read of the loop's index is no loop-invariant value")
	}
	computed, cbody := restartTree(EmitOperand{kind: opEvent, idx: 7}, after(evCall))
	if restartSpanReruns(computed, start, len(cbody), cbody, 2) {
		t.Error("a computed receiver is no proven loop-invariant value")
	}
	if restartSpanReruns(tree, start, len(body), body, 42) {
		t.Error("a landed value outside the tree restarts nothing")
	}
	// A span that ends before the later event leaves it out.
	if !restartSpanReruns(tree, start, 2, body, 3) {
		t.Error("an event past the statement's end is not the statement's")
	}
}

func TestRestartReruns(t *testing.T) {
	tree, body := restartTree(EmitOperand{kind: opLocal, idx: 1}, EmitEvent{kind: evCall, call: emitCall{word: "print", pos: core.SrcPos{Row: 1, Col: 15}}})
	end := core.NewEnd()
	end.SetPos(core.SrcPos{Row: 1, Col: 30})
	body = append(body, end, restartTok(1, 40))
	es := NewEmitState()
	if _, _, ok := es.restartReruns(tree, tree[2], 2, body, 0); !ok {
		t.Error("a loop's landing over an invariant read restarts")
	}
	apply := EmitEvent{kind: evCall, seq: 3, call: emitCall{word: "(paren apply)", dynMethod: &DynMethodSpec{}, ops: []EmitOperand{{kind: opEvent, idx: 2}}, pos: core.SrcPos{Row: 1, Col: 15}}}
	tree[3] = treeEvent{ev: &apply, inLoop: true, loopSlots: []int{5}}
	if _, _, ok := es.restartReruns(tree, tree[3], 3, body, 0); !ok {
		t.Error("a loop's shaped apply restarts over its method's invariant read")
	}
	flat := rootTreeEvents([]EmitEvent{{kind: evCall, seq: 1, call: emitCall{word: "dot", pos: core.SrcPos{Row: 1, Col: 10}}}}, false)
	if _, _, ok := es.restartReruns(flat, flat[1], 1, body, 0); !ok {
		t.Error("outside a loop, the reads before the stop are checked")
	}
}

func TestRootPreStart(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, promoted: map[int]int{4: 7}}
	val := func(id string, row, col int) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		v.SetPos(core.SrcPos{Row: row, Col: col})
		return v
	}
	es.producedBy["held"] = producer{seq: 3}
	es.producedBy["slot"] = producer{seq: 4, idx: 1}
	es.producedBy["mine"] = producer{seq: 8}
	start := core.SrcPos{Row: 2, Col: 1}
	residual := []core.Value{val("k", 1, 1), val("held", 0, 0), val("slot", 0, 0), val("mine", 0, 0), val("after", 3, 1)}
	srcs, held, ok := es.rootPreStart(lw, nil, residual, start, 8)
	if !ok || held != 1 || len(srcs) != 3 ||
		srcs[0].Kind != RestartConst || srcs[1].Kind != RestartStack || srcs[1].Idx != 0 ||
		srcs[2].Kind != RestartLocal || srcs[2].Idx != 8 {
		t.Errorf("the leading entries, each where the root keeps it: %+v held=%d ok=%v", srcs, held, ok)
	}
	if _, _, ok := es.rootPreStart(lw, nil, []core.Value{val("np", 0, 0)}, start, 8); ok {
		t.Error("a leading literal with no position cannot be placed")
	}
	if got := statementFirstSeq(map[int]treeEvent{5: {ev: &EmitEvent{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 2, Col: 4}}}}, 1: {ev: &EmitEvent{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 1}}}}}, 9, start); got != 5 {
		t.Errorf("the statement's first event is the earliest written at or after its start: got %d", got)
	}
}

func TestRestartAtAndDepths(t *testing.T) {
	lw := &lowerer{es: NewEmitState(), landingRoot: true}
	if lw.restartAt(1) != nil {
		t.Error("no plan, no island")
	}
	start := core.SrcPos{Row: 1, Col: 5}
	lw.landingRestarts = map[int]*landingRestart{1: {start: start, depth: -1, held: 0}, 2: {start: start, depth: -1, held: 1}}
	lw.noteRestartDepths(core.SrcPos{Row: 1, Col: 1})
	if lw.restartAt(1) != nil {
		t.Error("a statement not yet begun has no depth")
	}
	lw.vm = []vmSlot{}
	lw.noteRestartDepths(core.SrcPos{Row: 1, Col: 6})
	if lw.restartAt(1) == nil || lw.restartAt(2) != nil {
		t.Error("the root's island holds when the stack there is the residual's results it counted, and only then")
	}
	// A unit seats its unnamed params from their slots, then the region.
	u := &lowerer{es: NewEmitState(), unnamedParams: []int{0}, vm: []vmSlot{{seq: -1}}}
	u.landingRestarts = map[int]*landingRestart{3: {start: start, depth: -1, held: -1}}
	u.noteRestartDepths(core.SrcPos{Row: 1, Col: 6})
	r := u.restartAt(3)
	if r == nil || len(r.srcs) != 2 || r.srcs[0].Kind != RestartLocal || r.srcs[1].Kind != RestartStack {
		t.Errorf("a unit's island: its unnamed param from its slot, then the frame region: %+v", r)
	}
	// A param pushed inside a nested fragment is no placeable prefix.
	bad := &lowerer{es: NewEmitState(), unnamedParams: []int{0}, localPushedNested: map[int]bool{0: true}}
	bad.landingRestarts = map[int]*landingRestart{4: {start: start, depth: -1, held: -1}}
	bad.noteRestartDepths(core.SrcPos{Row: 1, Col: 6})
	if bad.restartAt(4) != nil {
		t.Error("a frame whose unnamed params the walk cannot place seats no island")
	}
}
