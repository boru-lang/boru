package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// arm_tail_apply_test.go pins ArmTailApply (the thirty-fourth increment):
// an inactive state, a residual shorter than two, a top that is no pending
// apply, and a window the recorder declines all pass the residual through;
// a pending fn over a resolvable window records the apply event, consumes
// the pending entry and nets one gradual value.
func TestArmTailApplyArms(t *testing.T) {
	one := core.NewInteger(1)
	fn := core.NewDynamicCarrier(core.TFunction)
	stk := []core.Value{one, fn}
	if got := inactiveEmitState().ArmTailApply(stk); len(got) != 2 {
		t.Fatal("an inactive state passes the residual through")
	}
	es := NewEmitState()
	if got := es.ArmTailApply([]core.Value{fn}); len(got) != 1 {
		t.Fatal("a residual of one passes through")
	}
	if got := es.ArmTailApply(stk); len(got) != 2 {
		t.Fatal("a top that is no pending apply passes through")
	}
	u := es.units[len(es.units)-1]
	u.pendingApply = []pendingApply{{id: fn.ID, pos: core.SrcPos{Row: 1, Col: 9}}}
	// The fn resolves to nothing: the recorder declines, the residual and
	// the pending entry stand.
	if got := es.ArmTailApply(stk); len(got) != 2 || len(u.pendingApply) != 1 {
		t.Fatal("a window the recorder declines passes through")
	}
	seedProduced(es, fn, 1)
	before := len(es.frames[0])
	got := es.ArmTailApply(stk)
	if len(got) != 1 || !got[0].Dynamic || got[0].Parent != core.TAny || !got[0].Carrier {
		t.Fatalf("a pending fn over the arm's window nets one gradual value: %v", got)
	}
	if len(u.pendingApply) != 0 || len(es.frames[0]) != before+1 {
		t.Fatal("the apply event records and consumes the pending entry")
	}
	ev := es.frames[0][len(es.frames[0])-1]
	if ev.kind != evCall || ev.call.word != wordDynApply || ev.call.dynApply != 1 || !ev.call.dynApplyUnquote || ev.call.pos.Col != 9 {
		t.Fatalf("the event is the apply word's, one value wide, at the apply's position: %+v", ev.call)
	}
}
