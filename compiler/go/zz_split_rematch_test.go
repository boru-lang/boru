package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestW8DispatchRematchDeclines(t *testing.T) {
	// Inactive recorder + interface no-op.
	var nilES *EmitState
	if nilES.RecordDispatchRematchValues("w", []core.Value{core.NewInteger(1)}, 0, []int{0}, core.SrcPos{}) {
		t.Error("inactive EmitState must decline")
	}
	if core.TheInactiveEmit.RecordDispatchRematchValues("w", []core.Value{core.NewInteger(1)}, 0, []int{0}, core.SrcPos{}) {
		t.Error("the inactive recorder must decline")
	}

	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es, _ := r.Check.Recorder().(*EmitState)
	es.BindRegistry(r)
	if es.RecordDispatchRematchValues("w", nil, 0, []int{0}, core.SrcPos{}) {
		t.Error("an empty window must decline")
	}
	// A dynamic value with no provenance fails resolveOperand.
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	if es.RecordDispatchRematchValues("w", []core.Value{dyn}, 0, []int{0}, core.SrcPos{}) {
		t.Error("an unresolvable operand must decline")
	}
	if es.RecordDispatchRematch("", []EmitOperand{ConstOperand(0)}, 0, []int{0}, core.SrcPos{}) {
		t.Error("an empty word must decline")
	}
	if es.RecordDispatchRematch("w", nil, 0, []int{0}, core.SrcPos{}) {
		t.Error("no operands must decline")
	}
	// The render tuple must be well-formed over the window: an empty tuple,
	// a negative index, an index past the window and a repeated index all
	// decline.
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, nil, core.SrcPos{}) {
		t.Error("an empty render tuple must decline")
	}
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, []int{-1}, core.SrcPos{}) {
		t.Error("a negative render index must decline")
	}
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, []int{1}, core.SrcPos{}) {
		t.Error("a render index past the window must decline")
	}
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0), ConstOperand(1)}, 0, []int{0, 0}, core.SrcPos{}) {
		t.Error("a repeated render index must decline")
	}
	// First trap wins: with a trap latched, a second record reports owned.
	// The forward count is 0..len(ops) (DispatchSpec.NFwd, NUR211).
	if es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, -1, []int{0}, core.SrcPos{}) ||
		es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 2, []int{0}, core.SrcPos{}) {
		t.Error("a forward count outside the window must decline")
	}
	if !es.RecordDispatchRematch("w", []EmitOperand{ConstOperand(0)}, 0, []int{0}, core.SrcPos{}) {
		t.Fatal("the first rematch record must land")
	}
	if !es.RecordDispatchRematch("w2", []EmitOperand{ConstOperand(1)}, 0, []int{0}, core.SrcPos{}) {
		t.Error("a second record after the latch must report owned (true), not re-record")
	}

	// The promoted-operand rewrite reaches a rematch trap's window.
	ev := EmitEvent{kind: evTrap, trap: EmitTrap{
		rematchWord: "w",
		rematchOps:  []EmitOperand{EventOperand(0, 0)},
	}}
	RewritePromotedRefs(&ev, map[int]int{0: 3})
	if op := ev.trap.rematchOps[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("rematch operand not rewritten to the promoted local: %+v", op)
	}
}
