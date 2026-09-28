package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// zz_cover_merge519_restart_test.go pins two seat guards directly:
// restartSpanReruns' landed-value screen (landing_restart.go) and the NUR222
// phantom screens of the lowerer's two spill seats (lower.go spillSeat,
// seatResidualRebuild). The phantom seats are reached by whole programs too —
// `(g) do [(1 add 1) drop] (h) swap` and `(g) (h) do [(1 add 1) drop] rot
// sub` decline loudly through them — but each such program is a booked
// compile defect, so the arm is dialled here where its decision is visible.

// restartSpanReruns needs the landed value to be a CALL event of the tree:
// one outside the tree (a stale seq) or of another kind (the loop holding
// the read) restarts nothing, even when every event of the span re-runs.
func TestMerge519RestartSpanRerunsLandedScreen(t *testing.T) {
	start := core.SrcPos{Row: 1, Col: 1}
	branch := EmitEvent{kind: evBranch, call: emitCall{pos: core.SrcPos{Row: 1, Col: 15}}, br: &emitBranch{pos: core.SrcPos{Row: 1, Col: 15}}}
	tree, body := restartTree(EmitOperand{kind: opLocal, idx: 1}, branch)
	if !restartSpanReruns(tree, start, len(body), body, 2) {
		t.Fatal("the landed member read over a loop-invariant local restarts")
	}
	if restartSpanReruns(tree, start, len(body), body, 42) {
		t.Error("a landed value outside the tree restarts nothing")
	}
	if _, isLoop := tree[9]; !isLoop || tree[9].ev.kind != evLoop {
		t.Fatalf("the fixture's loop is seq 9: %+v", tree[9])
	}
	if restartSpanReruns(tree, start, len(body), body, 9) {
		t.Error("a landed event that is no call restarts nothing")
	}
}

// m519PhantomLowerer is a lowerer over single-result event slots whose
// producer seqs in phantom are value-less `do` bodies' catch-latched
// results (eventFlags.catchPhantom, NUR222).
func m519PhantomLowerer(seqs []int, phantom ...int) *lowerer {
	lw := w8lw()
	for _, s := range seqs {
		lw.vm = append(lw.vm, vmSlot{seq: s, idx: 0})
	}
	for _, s := range phantom {
		lw.es.eventInfo[s] = eventFlags{catchPhantom: true}
	}
	return lw
}

// spillSeat declines with the caller's own wording when a slot it would
// spill is a phantom: the value-less body's latched count may be none, so a
// STORE_LOCAL of it would underflow at run time. Nothing is emitted; the
// same shape over real results spills.
func TestMerge519SpillSeatDeclinesAPhantom(t *testing.T) {
	ops := []EmitOperand{EventOperand(2, 0), EventOperand(1, 0)}
	lw := m519PhantomLowerer([]int{1, 2}, 1)
	if reason := lw.spillSeat(ops, []int{0, 1}, 2, core.SrcPos{}, "SPILLFAIL"); reason != "SPILLFAIL" {
		t.Fatalf("a phantom under the spill declines, got %q", reason)
	}
	if len(*lw.code) != 0 || len(lw.vm) != 2 || lw.numLocals != 0 {
		t.Errorf("a declined spill emits nothing: code=%v vm=%v locals=%d", *lw.code, lw.vm, lw.numLocals)
	}
	lw = m519PhantomLowerer([]int{1, 2})
	if reason := lw.spillSeat(ops, []int{0, 1}, 2, core.SrcPos{}, "SPILLFAIL"); reason != "" {
		t.Fatalf("the same shape over real results spills, got %q", reason)
	}
}

// seatResidualRebuild stands aside when ANY simulated-stack slot is a
// phantom — one no residual operand names included, since the rebuild
// spills the whole stack.
func TestMerge519ResidualRebuildDeclinesAPhantom(t *testing.T) {
	ops := []EmitOperand{EventOperand(3, 0), EventOperand(2, 0)}
	lw := m519PhantomLowerer([]int{1, 2, 3}, 1)
	if lw.seatResidualRebuild(ops, core.SrcPos{}) {
		t.Fatal("a phantom on the simulated stack declines the rebuild")
	}
	if len(*lw.code) != 0 || len(lw.vm) != 3 {
		t.Errorf("a declined rebuild emits nothing: code=%v vm=%v", *lw.code, lw.vm)
	}
	lw = m519PhantomLowerer([]int{1, 2, 3})
	if !lw.seatResidualRebuild(ops, core.SrcPos{}) {
		t.Fatal("the same residual over real results rebuilds")
	}
}
