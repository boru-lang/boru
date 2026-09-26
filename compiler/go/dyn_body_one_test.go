package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// dyn_body_one_test.go covers the runtime-checked single value
// (dyn_body_one.go, NUR210's follow-up): which regions demote, where the
// pre-pass looks, what the lowering flags, and the kept-defs latch's
// fresh-read exemption (kept_defs.go) that lets `def ok (do b …) … ok`
// through.

func checkableRegion() eventFlags {
	return eventFlags{dynBodyResult: true, variadicRegion: true, regionMayBeFn: true, variadicResult: true}
}

func TestDynRegionCheckable(t *testing.T) {
	if !dynRegionCheckable(checkableRegion()) {
		t.Error("a computed do body's region that may leave a callable is checkable")
	}
	for name, f := range map[string]eventFlags{
		"a plain-data region (static count)":  {dynBodyResult: true, variadicRegion: true},
		"a loop or branch region":             {variadicRegion: true, regionMayBeFn: true},
		"a dyn-body result that is no region": {dynBodyResult: true, regionMayBeFn: true},
	} {
		if dynRegionCheckable(f) {
			t.Errorf("%s is not checkable", name)
		}
	}
}

func TestDemoteDynRegion(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[4] = checkableRegion()
	es.demoteDynRegion(4)
	f := es.eventInfo[4]
	if !f.dynBodyOne || f.variadicRegion || f.regionMayBeFn || !f.dynBodyResult || !f.variadicResult {
		t.Errorf("demoted: only the region marks clear, got %+v", f)
	}
	if (&lowerer{es: es}).dynRegionMayBeFn(4) {
		t.Error("a demoted run is no region the residual seat rule reads")
	}
}

func TestDemoteConsumedDynRegions(t *testing.T) {
	es := NewEmitState()
	for seq := 1; seq <= 9; seq++ {
		es.eventInfo[seq] = checkableRegion()
	}
	es.eventInfo[10] = eventFlags{dynBodyResult: true, variadicRegion: true} // plain data: static count
	arm := &EmitFragment{events: []EmitEvent{{seq: 20, kind: evCall, call: emitCall{word: "add", ops: []EmitOperand{EventOperand(2, 0)}}}}}
	es.frames[0] = []EmitEvent{
		{seq: 11, kind: evCall, call: emitCall{word: "add", ops: []EmitOperand{EventOperand(1, 0), EventOperand(10, 0)}}},
		{seq: 12, kind: evBranch, br: &emitBranch{cond: EventOperand(3, 0), then: arm, thenOut: EventOperand(4, 0), elsVal: EventOperand(5, 0),
			carried: []carriedInit{{init: EventOperand(6, 0)}}}},
		{seq: 13, kind: evLoop, loop: &emitLoop{start: EventOperand(7, 0), bodyOut: EventOperand(8, 0),
			carried: []carriedInit{{init: EventOperand(9, 0)}}}},
	}
	unitArm := &EmitFragment{events: []EmitEvent{{seq: 21, kind: evStore, store: &emitStore{src: EventOperand(8, 0)}}}}
	es.fnRecs = []*fnUnitRec{{}, {frag: &EmitFragment{events: []EmitEvent{{seq: 22, kind: evLoop, loop: &emitLoop{body: unitArm}}}}}}
	es.demoteConsumedDynRegions()
	for seq, want := range map[int]bool{
		1: true,  // a call operand
		2: true,  // a call operand inside a branch arm
		3: true,  // a branch condition
		4: false, // a branch arm's OUT keeps the region rules
		5: true,  // a value arm
		6: true,  // a carried seed
		7: true,  // a loop bound
		8: true,  // a loop body's OUT is not consuming, but a unit's store in a nested body is
		9: true,  // a loop's carried seed
	} {
		if got := es.eventInfo[seq].dynBodyOne; got != want {
			t.Errorf("seq %d demoted=%v, want %v", seq, got, want)
		}
	}
	if es.eventInfo[10].dynBodyOne {
		t.Error("a plain-data region keeps its static decline")
	}
}

func TestDynBodyOneAt(t *testing.T) {
	if (&lowerer{}).dynBodyOneAt(1) {
		t.Error("a lowerer with no recorder answers no")
	}
	es := NewEmitState()
	es.eventInfo[1] = eventFlags{dynBodyOne: true}
	es.eventInfo[2] = checkableRegion()
	es.eventInfo[3] = checkableRegion()
	es.eventInfo[4] = checkableRegion()
	es.eventInfo[5] = eventFlags{dynBodyResult: true, variadicRegion: true}
	lw := &lowerer{es: es, promoted: map[int]int{3: 0, 5: 1}, dead: map[int]bool{4: true}}
	if !lw.dynBodyOneAt(1) {
		t.Error("a pre-pass demotion lowers checked")
	}
	if lw.dynBodyOneAt(2) || es.eventInfo[2].dynBodyOne {
		t.Error("a region seated whole stays a region")
	}
	if !lw.dynBodyOneAt(3) || !es.eventInfo[3].dynBodyOne {
		t.Error("a promoted region (`def ok (do b)` read again) demotes")
	}
	if !lw.dynBodyOneAt(4) || !es.eventInfo[4].dynBodyOne {
		t.Error("a dead region demotes")
	}
	if lw.dynBodyOneAt(5) {
		t.Error("a plain-data region keeps its decline")
	}
}

func TestBodyPlainCount(t *testing.T) {
	es := NewEmitState()
	body := core.NewDynamicCarrier(core.TList)
	body.ID = "mk-out"
	if _, ok := es.bodyPlainCount(body); ok {
		t.Fatal("an unresolvable operand has no static count")
	}
	es.consts = []core.Value{
		core.NewList([]core.Value{core.NewInteger(5)}),
		core.NewList([]core.Value{core.NewInteger(5), core.NewList([]core.Value{core.NewInteger(6)})}),
		core.NewList([]core.Value{core.NewInteger(5), core.NewDispatchMod(core.DispatchModInfo{})}),
	}
	for i := range es.consts {
		es.fnRecs = append(es.fnRecs, &fnUnitRec{outOps: []EmitOperand{{kind: opConst, idx: i}}})
		es.frames[0] = append(es.frames[0], EmitEvent{seq: i + 1, kind: evCallUser, uc: emitUserCall{unit: i, nout: 1}})
	}
	for seq, want := range map[int]int{1: 1, 2: 2} {
		es.producedBy[body.ID] = producer{seq: seq}
		if n, ok := es.bodyPlainCount(body); !ok || n != want {
			t.Errorf("seq %d: count %d/%v, want %d", seq, n, ok, want)
		}
	}
	es.producedBy[body.ID] = producer{seq: 3}
	if _, ok := es.bodyPlainCount(body); ok || es.bodyPlainData(body) {
		t.Error("a dispatch modifier is no pushed value")
	}
}

func TestEngineMarkerToken(t *testing.T) {
	if engineMarkerToken(core.NewInteger(1)) {
		t.Error("a literal is one pushed value")
	}
	if !engineMarkerToken(core.NewDispatchMod(core.DispatchModInfo{})) {
		t.Error("a dispatch modifier is a marker")
	}
}

func TestKeptDefsFreshBind(t *testing.T) {
	es := NewEmitState()
	es.noteKeptDefsFreshBind("ok", "v1")
	if es.keptDefsFresh != nil {
		t.Fatal("no latch armed: nothing is fresh")
	}
	es.units = append(es.units, &emitUnit{})
	es.runKeptDefs("do")
	es.noteKeptDefsFreshBind("ok", "")
	if es.keptDefsFresh != nil {
		t.Fatal("a value with no identity is never fresh")
	}
	es.noteKeptDefsFreshBind("ok", "v1")
	if !es.keptDefsFreshRead("v1", "ok") {
		t.Error("the read of the def made after the run is fresh")
	}
	if es.keptDefsFreshRead("v2", "ok") || es.keptDefsFreshRead("v1", "other") || es.keptDefsFreshRead("", "nope") {
		t.Error("only the very binding is fresh")
	}
	// A def in a deeper unit binds that unit's frame.
	es.units = append(es.units, &emitUnit{})
	es.noteKeptDefsFreshBind("deep", "v3")
	if es.keptDefsFreshRead("v1", "ok") || es.keptDefsFresh["deep"] != "" {
		t.Error("the exemption holds only at the latch's own depth")
	}
	es.units = es.units[:2]
	// A user call may run code the model did not see.
	es.keptDefsEvent(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: -1}})
	if es.keptDefsFreshRead("v1", "ok") {
		t.Error("an observer event clears the fresh bindings")
	}
	es.noteKeptDefsFreshBind("ok", "v1")
	es.runKeptDefs("do")
	if es.keptDefsFreshRead("v1", "ok") {
		t.Error("a second run clears the fresh bindings")
	}
	es.noteKeptDefsFreshBind("ok", "v1")
	es.keptDefsHandedOn(&fnUnitRec{closure: true, runsKeptDefs: "do"}, 1)
	if es.keptDefsFresh != nil {
		t.Error("a handed-on run clears the fresh bindings")
	}
	fin := NewEmitState()
	fin.units = append(fin.units, &emitUnit{})
	fin.runKeptDefs("do")
	fin.noteKeptDefsFreshBind("ok", "v1")
	fin.finishKeptDefs(&fnUnitRec{runsKeptDefs: "do"})
	if fin.keptDefsFresh != nil || fin.keptDefsLevel != 0 {
		t.Error("the unit's finish disarms the latch and its fresh bindings")
	}
}
