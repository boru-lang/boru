package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// programPendingApply lowers the program unit's ONE pending `apply`-word
// application when it is the residual's top — over any number of values
// beneath, none included: the op models applyHandler over an empty window
// too (a 0-arg closure fires, a wider one stays data; NUR160's neighbours,
// 2026-09-23) — and consumes it; every other shape is left for Finalize's
// decline (the dynamic-lead group, 2026-09-22).
func TestProgramPendingApply(t *testing.T) {
	var nilES *EmitState
	if nilES.programPendingApplyTop(nil) {
		t.Error("a nil state holds no pending apply")
	}
	es := NewEmitState()
	fn := core.NewCarrier(core.TFunction)
	fn.ID = "fn-p"
	residual := []core.Value{core.NewInteger(5), fn}
	if es.programPendingApplyTop(residual) {
		t.Error("no pending entry: nothing to lower")
	}
	pos := core.SrcPos{Row: 3, Col: 7}
	es.units[0].pendingApply = []pendingApply{{id: "fn-p", pos: pos}}
	if !es.programPendingApplyTop([]core.Value{fn}) {
		t.Error("nothing beneath the fn: the apply word's op over an empty window")
	}
	if es.programPendingApplyTop([]core.Value{fn, core.NewInteger(5)}) {
		t.Error("the pending fn must be the residual's top")
	}
	if !es.programPendingApplyTop(residual) {
		t.Fatal("the pending fn on top over one value is the whole-residual apply")
	}
	if len(es.units[0].pendingApply) != 1 {
		t.Error("the test must not consume the entry")
	}
	got, ok := es.programPendingApply(residual)
	if !ok || got != pos {
		t.Errorf("programPendingApply = (%v, %v), want the apply word's position", got, ok)
	}
	if len(es.units[0].pendingApply) != 0 {
		t.Error("the lowering consumes the entry")
	}
	if _, ok := es.programPendingApply(residual); ok {
		t.Error("a consumed entry lowers nothing")
	}
	es.units[0].pendingApply = []pendingApply{{id: "fn-p"}, {id: "fn-q"}}
	if es.programPendingApplyTop(residual) {
		t.Error("two pending applies are not the one shape this lowers")
	}
}
