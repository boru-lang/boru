package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The no-copy phase 3 arms of the recorder the merged gate's corpus does not
// reach, pinned here.

// TestP3ProducerOfKeyNilState: a nil recording state resolves no key.
func TestP3ProducerOfKeyNilState(t *testing.T) {
	var es *EmitState
	if _, ok := es.producerOfKey(core.RecKey{Pass: 1, Seq: 1}); ok {
		t.Fatal("a nil state resolved a recorder key")
	}
}

// TestP3ConsumesSpliceSpread: a dispatch whose operand is the spread of a
// computed splice that may be a list consumes a spread; one over other
// values, or over a result that spreads nothing, does not.
func TestP3ConsumesSpliceSpread(t *testing.T) {
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 1}})
	f := es.eventInfo[seq]
	f.spliceMany = true
	es.eventInfo[seq] = f
	spread := core.NewCarrier(core.TAny)
	es.stampProduced(&spread, seq, 0)
	other := core.NewInteger(7)
	if !es.consumesSpliceSpread([]core.Value{other, spread}) {
		t.Error("a spread among the operands is consumed")
	}
	if es.consumesSpliceSpread([]core.Value{other}) {
		t.Error("no operand is a spread")
	}
	one := core.NewCarrier(core.TAny)
	es.stampProduced(&one, es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "size", nout: 1}}), 0)
	if es.consumesSpliceSpread([]core.Value{one}) {
		t.Error("an event result that spreads nothing is one operand")
	}
}

// TestP3OrphanFreshUnits: a declined compile's fresh units become stamp-only
// (nil entries skipped); a memo hit — a unit some other dispatch made —
// marks nothing.
func TestP3OrphanFreshUnits(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{name: "earlier"}, {name: "fresh"}, nil, {name: "nested"}}
	es.orphanFreshUnits(1, 0)
	for _, r := range es.fnRecs {
		if r != nil && r.stampOnly {
			t.Fatalf("a memo hit marked %s", r.name)
		}
	}
	es.orphanFreshUnits(1, 1)
	if es.fnRecs[0].stampOnly || !es.fnRecs[1].stampOnly || !es.fnRecs[3].stampOnly {
		t.Errorf("the fresh units, and only they: %v %v %v", es.fnRecs[0].stampOnly, es.fnRecs[1].stampOnly, es.fnRecs[3].stampOnly)
	}
}

// TestP3FinalizeStubsAStampOnlyUnitWhoseResidualDeclines: Finalize over a
// stamp-only unit whose events lower but whose RESIDUAL seating declines —
// the residual names a result no event of the unit left — keeps the
// program compiling, the unit a trap stub at its index; a unit the program
// calls declines the program instead.
func TestP3FinalizeStubsAStampOnlyUnitWhoseResidualDeclines(t *testing.T) {
	unit := func(stampOnly bool) *EmitState {
		es := rpState(t)
		one, two := es.intern(core.NewInteger(1)), es.intern(core.NewInteger(2))
		ev := EmitEvent{seq: 6, kind: evCall, call: emitCall{word: "add", nout: 1, ops: []EmitOperand{ConstOperand(one), ConstOperand(two)}}}
		es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "stamped$body", finished: true, stampOnly: stampOnly,
			frag:   &EmitFragment{events: []EmitEvent{ev}, startSeq: 5},
			outOps: []EmitOperand{EventOperand(6, 0), EventOperand(99, 0)}, returns: []*core.Type{core.TInteger, core.TInteger}})
		return es
	}
	p, reason, ok := unit(true).Finalize(nil)
	if !ok {
		t.Fatalf("a stamp-only unit's residual decline must not decline the program: %s", reason)
	}
	if len(p.Fns) != 1 || len(p.Fns[0].Code) != 1 || p.Fns[0].Code[0].Op != OpTrap {
		t.Fatalf("the unit must be a trap stub at its index: %+v", p.Fns)
	}
	if _, reason, ok := unit(false).Finalize(nil); ok || !strings.Contains(reason, "stamped$body") {
		t.Errorf("a called unit's residual decline declines the program: ok=%v %q", ok, reason)
	}
}
