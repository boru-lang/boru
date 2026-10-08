package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The sixty-ninth increment: a forward word slot that reads a generalised
// name ROUTES. A live read is the slot's operand (slotIsOperand), the
// routing admits the region at top level for such a slot, the read's
// event is marked a placeholder the routed op pops unread
// (placeRoutedLiveSlots), and the lowering pushes the placeholder in place
// of the lookup.
func TestForwardSlotOfGeneralisedNameRoutes(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es := NewEmitState()
	es.reg = reg
	es.specUndefNames = map[string]bool{"k": true}
	pos := core.SrcPos{Row: 1, Col: 9}
	// A live read of k: its own identity, its own event.
	v := core.NewCarrier(core.TInteger)
	reg.Defs.Push("k", v)
	es.NoteLiveRead(&v, "k", pos)
	slot := SlotDesc{Source: SlotWordRef, Token: core.NewWord("k")}
	if !es.slotIsOperand(slot, reg, v) {
		t.Fatal("a live read of the word is the word slot's operand")
	}
	other := core.NewCarrier(core.TInteger)
	es.NoteLiveRead(&other, "k", pos)
	es.defReads[other.ID] = "j"
	if es.slotIsOperand(slot, reg, other) {
		t.Fatal("a live read of another name is not")
	}
	// Routing at top level admits the region for the generalised slot only.
	d := &RegionDesc{Lead: LeadWord, Word: "add", NFwd: 2, Slots: []SlotDesc{slot, {Source: SlotConst, Token: core.NewInteger(1)}}}
	if !es.routeRegion(d) {
		t.Fatal("a root dispatch with a generalised forward slot routes")
	}
	plain := &RegionDesc{Lead: LeadWord, Word: "add", NFwd: 2, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("j")}, {Source: SlotConst, Token: core.NewInteger(1)}}}
	if es.routeRegion(plain) {
		t.Fatal("a root dispatch without one keeps the bake")
	}
	// The read's event becomes a placeholder for the routed dispatch.
	es.placeRoutedLiveSlots(d, []core.Value{v, core.NewInteger(1)})
	pr := es.producedBy[v.ID]
	if !es.livePlaceholders[pr.seq] {
		t.Fatalf("the routed slot's read is a placeholder: %v", es.livePlaceholders)
	}
	es.placeRoutedLiveSlots(nil, nil)
	es.placeRoutedLiveSlots(&RegionDesc{NFwd: 1, Slots: []SlotDesc{{Source: SlotConst, Token: core.NewInteger(1)}}}, []core.Value{core.NewInteger(1)})
	if len(es.livePlaceholders) != 1 {
		t.Fatalf("a nil descriptor or a value slot marks nothing: %v", es.livePlaceholders)
	}
	// The lowering: a placeholder push, not a lookup.
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	ev := es.frames[0][0]
	if ev.seq != pr.seq || !ev.call.live {
		t.Fatalf("the first event is the live read: %+v", ev)
	}
	if reason := lw.lowerCall(&ev); reason != "" {
		t.Fatalf("the placeholder lowers: %s", reason)
	}
	if len(cf.Code) != 1 || cf.Code[0].Op != OpPushConst || int(cf.Code[0].Arg) != ev.call.liveName || len(lw.vm) != 1 {
		t.Fatalf("a routed slot's read pushes its placeholder: code=%v vm=%v", cf.Code, lw.vm)
	}
}
