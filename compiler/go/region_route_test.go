package compiler

import (
	"strings"
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
	// The review of #460's declines. A modified word in the claim (`k/v`,
	// `k/s`) is dispatch control the descriptor host does not model — and
	// the op defers on a `/v` slot unconditionally, so routing it would
	// route a guaranteed defer.
	if es.routeRegion(live(2, SlotDesc{Source: SlotWordRef, Token: core.NewWordRef("k")}, one)) {
		t.Error("a /v word in the claim: no route")
	}
	if es.routeRegion(live(2, SlotDesc{Source: SlotWordRef, Token: core.NewWordModified("k", -1, true, false)}, one)) {
		t.Error("a /s word in the claim: no route")
	}
	// A lead the run-time def stack does not hold — a body-local callee —
	// keeps the CALL_USER that reaches its unit by index.
	local := live(2, word, one)
	local.LeadLocal = true
	if es.routeRegion(local) {
		t.Error("a body-local lead: no route")
	}
}

// The two declines that are the CALL's rather than the span's, driven
// through RecordUserCall over a real capture: a callee with captures keeps
// its committed call (the captures ride as trailing operands the routed op
// has no plumbing for), and a lead bound inside the enclosing fn completes
// with LeadLocal set and keeps it too. The control routes.
func TestRecordUserCallDeclinesCapturesAndLocalLeads(t *testing.T) {
	record := func(t *testing.T, scoped, captures bool) emitUserCall {
		t.Helper()
		es, reg, done := beginRegionPass(t)
		defer done()
		// A capture is minted inside the pass, like every value the unit
		// compile registers by identity.
		var caps []core.CapturedBinding
		if captures {
			caps = []core.CapturedBinding{{Name: "c", Value: core.NewInteger(3)}}
		}
		// k is a module-scope value the call's first operand resolves to (a
		// LIVE slot); the unit's own params are other values, so neither
		// operand is a frame slot.
		kv, b := core.NewInteger(5), core.NewInteger(1)
		reg.Defs.Push("k", kv)
		if scoped {
			// The lead is bound AFTER the enclosing fn's baseline — a
			// body-local `def f …`.
			reg.FnBaselines = append(reg.FnBaselines, map[string]int{"k": 1})
			reg.Defs.Push("f", core.NewInteger(0))
		}
		pos := capture(t, es, reg, "f", core.NewWord("k"), b)
		unit, _, ok := es.StartFnCompile("f", "f", nil, []core.Value{core.NewInteger(0), core.NewInteger(0)},
			[]*core.Type{core.TAny, core.TAny}, []string{"x", "y"}, caps, false, core.SrcPos{})
		if !ok || unit < 0 {
			t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
		}
		es.RecordUserCall(unit, "f", []core.Value{kv, b}, nil, core.SrcPos{Row: 1, Col: 3}, pos)
		frame := es.frames[len(es.frames)-1]
		if len(frame) == 0 || frame[len(frame)-1].kind != evCallUser {
			t.Fatalf("RecordUserCall recorded no user-call event (reason %q)", es.Reason)
		}
		return frame[len(frame)-1].uc
	}
	if uc := record(t, false, false); !uc.generic || uc.region == nil || uc.region.LeadLocal {
		t.Errorf("the control — a module-scope lead over a live slot, no captures — routes: generic=%v region=%v", uc.generic, uc.region)
	}
	if uc := record(t, false, true); uc.generic {
		t.Error("a callee with captures keeps its committed call")
	}
	if uc := record(t, true, false); uc.generic || uc.region == nil || !uc.region.LeadLocal {
		t.Errorf("a lead bound inside the enclosing fn completes LeadLocal and keeps its committed call: generic=%v region=%+v", uc.generic, uc.region)
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
	if len(lw.p.Generics) != 1 || lw.p.Generics[0].Region != 0 || lw.p.Generics[0].Unit != 0 || lw.p.Generics[0].NOut != 1 || lw.p.Generics[0].NArgs != 2 {
		t.Errorf("the spec names the region, the unit, the result count and the record's arity: %+v", lw.p.Generics)
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

// The native seat's routed lowering (the sixty-fifth increment): a routed
// evCall lowers to DISPATCH_GENERIC with no committed unit, whether the
// record was mono or poly; a list literal in the span is not drivable.
func TestLowerRoutedNativeCall(t *testing.T) {
	es := NewEmitState()
	d := RegionDesc{Lead: LeadWord, Word: "add", NFwd: 2, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("k")}, {Source: SlotConst, Token: core.NewInteger(1)}}}
	for _, poly := range []bool{false, true} {
		lw := &lowerer{es: es, p: &Program{Consts: []core.Value{core.NewInteger(5), core.NewInteger(1)}}, sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}}
		lw.code, lw.debug = &lw.p.Code, &lw.p.Debug
		ev := &EmitEvent{kind: evCall, call: emitCall{word: "add", ops: []EmitOperand{ConstOperand(0), ConstOperand(1)}, nout: 1, region: &d, generic: true, poly: poly}}
		if !poly {
			ev.call.sig = &core.Signature{Args: []*core.Type{core.TAny, core.TAny}, Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
				return nil, nil
			})}
		}
		if reason := lw.lowerCall(ev); reason != "" {
			t.Fatalf("poly=%v: lowering refused: %s", poly, reason)
		}
		if n := len(lw.p.Code); n == 0 || lw.p.Code[n-1].Op != OpDispatchGeneric {
			t.Fatalf("poly=%v: a routed native call lowers to DISPATCH_GENERIC, got %v", poly, lw.p.Code)
		}
		if len(lw.p.Generics) != 1 || lw.p.Generics[0].Unit != -1 || lw.p.Generics[0].NOut != 1 || lw.p.Generics[0].NArgs != 2 || len(lw.p.Sigs) != 0 || len(lw.p.PolyRefs) != 0 {
			t.Errorf("poly=%v: the spec is unit-less, carries the record's arity, and no sig or poly ref is baked: %+v sigs=%d polys=%d", poly, lw.p.Generics, len(lw.p.Sigs), len(lw.p.PolyRefs))
		}
		// The record's own set: the mono record's one implementation, the
		// poly record's live table.
		if gs := lw.p.Generics[0]; gs.LiveSet != poly || (gs.Impl != nil) == poly || (!poly && gs.Impl != ev.call.sig.Impl) {
			t.Errorf("poly=%v: want Impl for a mono record and LiveSet for a poly one, got %+v", poly, gs)
		}
		if !strings.Contains(lw.p.Disassemble(), "(native)") {
			t.Errorf("poly=%v: the disassembly names the unit-less route:\n%s", poly, lw.p.Disassemble())
		}
	}
	es2 := NewEmitState()
	openUnit(es2, false)
	list := &RegionDesc{Lead: LeadWord, Word: "size", NFwd: 1, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("k")}, {Source: SlotConst, Token: core.NewList([]core.Value{core.NewInteger(1)})}}}
	if es2.routeRegion(list) {
		t.Error("a list literal in the span is evaluated on arrival — not drivable, no route")
	}
}

// A loop-carried name and a routed read never meet (EmitState.carriedNames /
// routedNames): a region whose live slot names a carried name keeps its
// committed call and retires no note, and a loop that comes to carry a name
// a routed dispatch already reads refuses the program — the two ways the
// compiled program models one name, kept apart in both orders.
func TestRouteRegionAndLoopCarriedNamesExclude(t *testing.T) {
	d := &RegionDesc{Lead: LeadWord, Word: "w", NFwd: 1, Slots: []SlotDesc{{Source: SlotWordRef, Token: core.NewWord("k")}}}
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	es.carriedNames = map[string]bool{"k": true}
	if es.routeRegion(d) {
		t.Fatal("a carried name keeps its committed call")
	}
	if _, frozen := rec.frozen["k"]; !frozen {
		t.Error("a declined route retires no note")
	}
	// The other order: routed first, then carried.
	es2 := NewEmitState()
	openUnit(es2, false)
	if !es2.routeRegion(d) || !es2.routedNames["k"] {
		t.Fatal("routes, and remembers the name it made live")
	}
	es2.BeginLoopCarried()
	es2.NoteLoopCarried("k", core.NewInteger(9), core.NewInteger(5))
	if es2.Compilable || !strings.Contains(es2.Reason, "loop-carried def `k` rebinds a name a routed dispatch reads live") {
		t.Errorf("a loop carrying a routed name refuses: compilable=%v reason=%q", es2.Compilable, es2.Reason)
	}
	// A carried name no dispatch routes registers as before, and is
	// remembered for the routes that follow.
	es3 := NewEmitState()
	es3.BeginLoopCarried()
	es3.NoteLoopCarried("j", core.NewInteger(9), core.NewInteger(5))
	if !es3.carriedNames["j"] || !es3.Compilable {
		t.Errorf("an unrouted name is carried: %v %q", es3.carriedNames, es3.Reason)
	}
}
