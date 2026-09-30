package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The read-site guard's lowering arms (read_site_guard.go, NUR361), driven
// directly: the lang-level parity rows (lang/go nur359_361_test.go) seat a
// guard wherever the recorder armed one, so the decline a value with no
// home takes, and the bookkeeping edges, are pinned here.

func siteLowerer() *lowerer {
	lw := w8lw()
	lw.siteTable = &lw.p.Deopts
	return lw
}

// A guard whose value has no home — not promoted, not on the simulated
// stack — declines the unit wherever its anchor stands: before an event,
// after one, or at an eventless fragment's end. Never a slot push the
// interpreter would dispatch.
func TestReadSiteGuardNoHomeDeclines(t *testing.T) {
	site := &readSite{id: "v", name: "v", before: -1, after: -1, prod: producer{seq: 40}}
	twin := func(seq int) EmitEvent {
		return EmitEvent{seq: seq, kind: evBindTwin, twin: &emitBindTwin{idx: 0}}
	}

	lw := siteLowerer()
	lw.sitesBefore = map[int][]*readSite{1: {site}}
	if reason := lw.lowerEvents([]EmitEvent{twin(1)}, -1); reason != readSiteUnguarded {
		t.Errorf("before-anchored guard with no home: reason %q", reason)
	}

	lw = siteLowerer()
	lw.sitesAfter = map[int][]*readSite{1: {site}}
	if reason := lw.lowerEvents([]EmitEvent{twin(1)}, -1); reason != readSiteUnguarded {
		t.Errorf("after-anchored guard with no home: reason %q", reason)
	}

	lw = siteLowerer()
	lw.sitesAtEnd = map[int][]*readSite{7: {site}}
	if reason := lw.lowerFragment(&EmitFragment{id: 7}, nil, false, core.SrcPos{}); reason != readSiteUnguarded {
		t.Errorf("fragment-end guard with no home: reason %q", reason)
	}
}

// A guard over a promoted producer tests its slot; one over a value still
// on the simulated stack tests it at its depth. Both are designed defers.
func TestReadSiteGuardHomes(t *testing.T) {
	lw := siteLowerer()
	lw.promoted[3] = 2
	lw.vm = []vmSlot{{seq: 9}, nonEventSlot}
	sites := []*readSite{
		{id: "a", name: "a", prod: producer{seq: 3, idx: 1}},
		{id: "b", name: "b", prod: producer{seq: 9}},
	}
	if reason := lw.emitReadSiteGuards(sites); reason != "" {
		t.Fatalf("reason %q", reason)
	}
	d := lw.p.Deopts
	if len(d) != 2 || d[0].Slot != 3 || d[0].Depth != -1 || !d[0].Bail || d[1].Slot != -1 || d[1].Depth != 1 || !d[1].Bail {
		t.Errorf("deopt table = %+v", d)
	}
}

// The bookkeeping edges: a value no site records is not guardable; an armed
// read is armed once; another unit's sites are not this lowering's; a
// fallback island applies its fn operand itself.
func TestReadSiteGuardBookkeeping(t *testing.T) {
	es := NewEmitState()
	if es.siteGuardable(nil, "none", nil, nil) {
		t.Error("a value with no recorded site is not guardable")
	}
	other := &fnUnitRec{}
	es.readSites = map[string][]*readSite{"v": {
		{id: "v", owner: other, before: 5, after: -1},
		{id: "v", owner: nil, before: -1, after: 6},
		{id: "v", owner: nil, before: -1, after: -1, fragID: 2},
	}}
	lw := siteLowerer()
	lw.es = es
	es.guardReadSites(nil, "v")
	if !lw.armReadSites("v") {
		t.Error("an armed read reports guarded")
	}
	lw.indexReadSites(&lw.p.Deopts)
	if len(lw.sitesBefore) != 0 || len(lw.sitesAfter[6]) != 1 || len(lw.sitesAtEnd[2]) != 1 {
		t.Errorf("anchors before=%v after=%v end=%v", lw.sitesBefore, lw.sitesAfter, lw.sitesAtEnd)
	}
	if !appliesFnOperand(&EmitEvent{kind: evFallback}) || appliesFnOperand(&EmitEvent{kind: evStore}) {
		t.Error("appliesFnOperand: a fallback island applies, a store does not")
	}
}
