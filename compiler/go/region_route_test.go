package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// routeRegion is the generic lane's first routing decision (region_route.go):
// a fn-unit dispatch whose claim carries a live word slot over a drivable
// span routes, and routing retires the frozen notes of the reads it makes
// live. Each arm of the decision is pinned with the shape that takes it,
// and the negative — a span the host cannot drive — stays a bake.
func TestRouteRegionDecidesByShape(t *testing.T) {
	live := func(nfwd int, slots ...SlotDesc) *RegionDesc {
		return &RegionDesc{Lead: LeadWord, Word: "w", NFwd: nfwd, Slots: slots}
	}
	word := SlotDesc{Source: SlotWordRef, Token: core.NewWord("k")}
	one := SlotDesc{Source: SlotConst, Token: core.NewInteger(1)}
	group := SlotDesc{Source: SlotEvent, Token: core.NewParenExpr([]core.Value{core.NewWord("id"), core.NewInteger(5)})}
	event := SlotDesc{Source: SlotEvent, Token: core.NewInteger(3)}

	es := NewEmitState()
	if es.routeRegion(nil) {
		t.Error("no descriptor, no route")
	}
	if es.routeRegion(live(2, word, one)) {
		t.Error("at top level the bake IS the read: no route outside a fn unit")
	}
	openUnit(es, false)
	if es.routeRegion(live(2, one, one)) {
		t.Error("a claim with no live word slot keeps its committed call")
	}
	if es.routeRegion(live(1, word, group)) {
		t.Error("a group in the span is an evaluation the host declines: no route")
	}
	if es.routeRegion(live(1, word, event)) {
		t.Error("an event slot beyond the claim has no value on the stack when the live claim reaches it: no route")
	}
	if !es.routeRegion(live(2, word, one)) {
		t.Error("a fn-unit claim with a live word slot over consts routes")
	}
	if !es.routeRegion(live(1, word, one)) {
		t.Error("a const slot beyond the claim is materialisable: routes")
	}
}

// Routing retires exactly the reads it makes live: a name read twice in a
// unit, routed once, stays frozen (the other read is still a bake); routed
// twice, it is unfrozen — the memo's staleness key and the escaping latch
// both drop it.
func TestRouteRegionRetiresOnlyTheRoutedReads(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	es.NoteFrozenRead("j", core.FrozenBakeValue, 1)
	d := &RegionDesc{Lead: LeadWord, Word: "w", NFwd: 1, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("k")}}}
	if !es.routeRegion(d) {
		t.Fatal("routes")
	}
	if _, frozen := rec.frozen["k"]; !frozen || rec.frozenReads["k"] != 1 {
		t.Errorf("one of two reads routed: k stays frozen with one bake left, got frozen=%v reads=%d", frozen, rec.frozenReads["k"])
	}
	if !es.routeRegion(d) {
		t.Fatal("routes again")
	}
	if _, frozen := rec.frozen["k"]; frozen || rec.bakes["k"] != 0 {
		t.Errorf("both reads routed: k is unfrozen and its bake generation dropped, got frozen=%v gen=%d", frozen, rec.bakes["k"])
	}
	if _, frozen := rec.frozen["j"]; !frozen {
		t.Error("an unrouted name is untouched")
	}
	// Unfreezing a name the unit never froze, or with no unit open, is a
	// no-op rather than a fault.
	es.unfreezeRead("never")
	es.openUnitRecs = es.openUnitRecs[:0]
	es.unfreezeRead("j")
	if _, frozen := rec.frozen["j"]; !frozen {
		t.Error("no unit open: nothing retired")
	}
	var nilES *EmitState
	if nilES.routeRegion(d) {
		t.Error("a nil recorder routes nothing")
	}
}

// A routed call is lowered through its descriptor: DISPATCH_GENERIC in place
// of CALL_USER, the region and the spec appended, never tail-marked.
func TestLowerRoutedUserCall(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "w", nParams: 2, finished: true, frag: &EmitFragment{}})
	d := RegionDesc{Lead: LeadWord, Word: "w", NFwd: 2, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("k")}, {Source: SlotConst, Token: core.NewInteger(1)}}}
	lw := &lowerer{es: es, p: &Program{}}
	lw.code, lw.debug = &lw.p.Code, &lw.p.Debug
	ev := &EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, ops: []EmitOperand{ConstOperand(0), ConstOperand(1)}, nout: 1, region: &d, generic: true, tail: true}}
	if reason := lw.lowerUserCall(ev); reason != "" {
		t.Fatalf("lowering refused: %s", reason)
	}
	if len(lw.p.Code) == 0 || lw.p.Code[len(lw.p.Code)-1].Op != OpDispatchGeneric {
		t.Fatalf("a routed call lowers to DISPATCH_GENERIC, got %v", lw.p.Code)
	}
	if len(lw.p.Generics) != 1 || lw.p.Generics[0].Region != 0 || lw.p.Generics[0].Unit != 0 || lw.p.Generics[0].NOut != 1 {
		t.Errorf("the spec names the region, the unit and the result count: %+v", lw.p.Generics)
	}
	if len(lw.p.Regions) != 1 || len(lw.vm) != 1 {
		t.Errorf("the region is appended, the two pushed operands consumed and the one result produced: regions=%d sim=%d", len(lw.p.Regions), len(lw.vm))
	}
	if n := len(lw.p.Code); n < 3 || lw.p.Code[n-3].Op != OpPushConst || lw.p.Code[n-2].Op != OpPushConst {
		t.Errorf("the operands stay pushed — they are the record's claim the VM pops: %v", lw.p.Code)
	}
}

// unfreezeRead's guards: no unit open, a unit index the table does not
// hold, a unit with no notes, a name never noted — each a no-op.
func TestUnfreezeReadGuards(t *testing.T) {
	es := NewEmitState()
	es.unfreezeRead("k")
	es.openUnitRecs = append(es.openUnitRecs, 99)
	es.unfreezeRead("k")
	es.openUnitRecs = es.openUnitRecs[:0]
	rec := openUnit(es, false)
	es.unfreezeRead("k")
	if rec.frozenReads != nil {
		t.Error("a unit with no notes stays without a table")
	}
}
