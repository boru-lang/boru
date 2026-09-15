package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The hold: a user-fn ReturnsFn takes its Phase-A offer out of the pool at
// ENTRY, so the callee's body analysis cannot re-offer over it from another
// source and consume it. These drive HoldRegion/claimRegion at the seam; the
// two-source collision from a real program is pinned in
// lang/go/region_capture_e2e_test.go.

// The collision, modelled: an outer call holds (f,1:1); the body it analyses
// dispatches f at 1:1 of another source, whose capture lands in the pool
// under the SAME key and whose own hold takes it; the inner record completes
// the inner offer, the inner release pops it, and the outer record still
// finds its own offer — the token it captured, not the inner's.
func TestHoldRegionSurvivesANestedOfferAtTheSameKey(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	one, zero := core.NewInteger(1), core.NewInteger(0)
	pos := capture(t, es, reg, "f", one)
	releaseOuter := es.HoldRegion("f", pos)
	if es.PendingRegionCount() != 0 || es.HeldRegionCount() != 1 {
		t.Fatalf("the hold must move the offer out of the pool: pending %d held %d", es.PendingRegionCount(), es.HeldRegionCount())
	}
	// The inner dispatch: same word, same row and column, a different token.
	capture(t, es, reg, "f", zero)
	releaseInner := es.HoldRegion("f", pos)
	if es.HeldRegionCount() != 2 {
		t.Fatalf("holds stack: held %d, want 2", es.HeldRegionCount())
	}
	inner := es.completeRegion("f", pos, []core.Value{zero}, []EmitOperand{ConstOperand(3)})
	if inner == nil || len(inner.Slots) != 1 || inner.Slots[0].Token.ID != zero.ID {
		t.Fatalf("the inner record must complete the INNER offer: %+v", inner)
	}
	releaseInner()
	if es.HeldRegionCount() != 1 {
		t.Fatalf("the inner release must pop only its own hold: held %d", es.HeldRegionCount())
	}
	outer := es.completeRegion("f", pos, []core.Value{one}, []EmitOperand{ConstOperand(4)})
	if outer == nil || len(outer.Slots) != 1 || outer.Slots[0].Token.ID != one.ID {
		t.Fatalf("the outer record must complete the OUTER offer it held: %+v", outer)
	}
	if outer.NFwd != 1 || outer.Slots[0].Source != SlotConst || outer.Slots[0].Idx != 4 {
		t.Errorf("outer slot = %+v, want SlotConst/4 claimed", outer.Slots[0])
	}
	releaseOuter()
	if es.HeldRegionCount() != 0 {
		t.Errorf("every hold released, %d left", es.HeldRegionCount())
	}
}

// A hold that found no offer (a stack-fed dispatch never forward-collects)
// blocks the record from taking a pool entry that arrived under the same key
// during the body analysis — that capture is the inner call's, not this
// one's. Once released, the pool entry is reachable again.
func TestHoldRegionWithoutAnOfferBlocksThePool(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	pos := core.SrcPos{Row: 1, Col: 1}
	release := es.HoldRegion("f", pos)
	if es.HeldRegionCount() != 1 {
		t.Fatalf("an empty hold is still pushed: held %d", es.HeldRegionCount())
	}
	capture(t, es, reg, "f", a) // the inner re-offer under the same key
	if d := es.completeRegion("f", pos, []core.Value{a}, []EmitOperand{ConstOperand(0)}); d != nil {
		t.Fatalf("a record under an empty hold must not claim the pool's entry: %+v", d)
	}
	release()
	if d := es.completeRegion("f", pos, []core.Value{a}, []EmitOperand{ConstOperand(0)}); d == nil {
		t.Fatal("with the hold released the pool entry is the ordinary claim again")
	}
}

// A held offer completes once; a second record under the same hold (the
// zero-return record and the apply route never both fire, but the contract
// is stated) gets nothing. A record for a DIFFERENT key under a hold falls
// through to the pool as it always did.
func TestHoldRegionCompletesOnceAndOtherKeysUseThePool(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, b := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "f", a)
	release := es.HoldRegion("f", pos)
	defer release()
	if d := es.completeRegion("f", pos, []core.Value{a}, []EmitOperand{ConstOperand(0)}); d == nil {
		t.Fatal("the first record completes the held offer")
	}
	if d := es.completeRegion("f", pos, []core.Value{a}, []EmitOperand{ConstOperand(0)}); d != nil {
		t.Fatalf("a held offer completes once: %+v", d)
	}
	// A native dispatch inside the held call's body: its own key, the pool.
	gpos := core.SrcPos{Row: 1, Col: 9}
	w := core.WithPosAt(core.NewWord("g"), gpos)
	tryRecordRegion(core.NewTape([]core.Value{w, b}, 0), reg, core.WordInfo{Name: "g", ArgCount: -1}, 0)
	if d := es.completeRegion("g", gpos, []core.Value{b}, []EmitOperand{ConstOperand(1)}); d == nil || d.Word != "g" {
		t.Fatalf("a record for another key under a hold claims from the pool: %+v", d)
	}
}

// Inactive and nil states hold nothing and release nothing — the ReturnsFn
// calls HoldRegion unconditionally, on the plain check pass too.
func TestHoldRegionInactiveIsANoop(t *testing.T) {
	es := NewEmitState()
	es.HoldRegion("f", core.SrcPos{Row: 1, Col: 1})()
	if es.HeldRegionCount() != 0 {
		t.Errorf("an inactive state must not push a hold: %d", es.HeldRegionCount())
	}
	(*EmitState)(nil).HoldRegion("f", core.SrcPos{})()
	if (*EmitState)(nil).HeldRegionCount() != 0 {
		t.Error("a nil state reports no holds")
	}
}
