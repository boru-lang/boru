package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// region_prefix_test.go pins the PLAN side of the inert-prefix seating
// (NUR067's consuming half): which residual shapes regionPrefixShape admits,
// and the two facts it has to get right that no whole-program row can show
// directly —
//
//   - which events count as a SINGLE-SLOT region (singleSlotRegion), because
//     a do-catch is variadicResult too and seats nout static slots, and
//   - which operands count as reading the enclosing stack
//     (regionReadsTheStack), because a loop's condOut / bodyOut are its
//     FRAGMENTS' results and must NOT count — treating them as stack reads
//     would silently decline every computed-body loop.
//
// lang/go/region_prefix_test.go is the whole-program half; eng/go's
// vm_seat_below_mark_test.go is the VM half.

// rpState builds a recorder with the registry bound, as a real compile pass
// has it.
func rpState(t *testing.T) *EmitState {
	t.Helper()
	es := NewEmitState()
	es.BindRegistry(seam7Reg(t))
	return es
}

// rpLoop appends a top-level value-producing loop event and returns its seq
// and the carrier the loop's result rides on — RecordLoop's shape, reduced to
// what the plan reads.
func rpLoop(es *EmitState, lp emitLoop) (int, core.Value) {
	seq := es.appendEvent(EmitEvent{kind: evLoop, loop: &lp})
	out := core.NewCarrier(core.TList)
	out.ID = core.GenerateID("zzrp")
	es.setProduced(out, seq)
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f
	return seq, out
}

func TestRegionPrefixShapeAdmitsTheInertPrefix(t *testing.T) {
	es := rpState(t)
	seq, out := rpLoop(es, emitLoop{hasBodyOut: true, bodyOut: EmitOperand{kind: opLocal}})

	// The frontier shape: one inert value beneath a value-producing loop.
	got, ok := es.regionPrefixShape([]core.Value{core.NewInteger(99), out}, es.frames[0])
	if !ok || got != seq {
		t.Errorf("regionPrefixShape = (%d,%v), want (%d,true)", got, ok, seq)
	}
	// TWO inert values still qualify — the prefix is a run, not a slot.
	if _, ok := es.regionPrefixShape([]core.Value{core.NewInteger(1), core.NewInteger(2), out}, es.frames[0]); !ok {
		t.Error("a two-value inert prefix must qualify")
	}
	// The region ALONE needs no seating: the ordinary residual absorbs it.
	if _, ok := es.regionPrefixShape([]core.Value{out}, es.frames[0]); ok {
		t.Error("a lone region does not need the prefix plan")
	}
	// The region must be LAST. A value ABOVE it is not a prefix at all, and
	// the ordinary seating already handles that shape (the tail pushes on
	// top of the run, exactly as recorded).
	if _, ok := es.regionPrefixShape([]core.Value{out, core.NewInteger(99)}, es.frames[0]); ok {
		t.Error("a region that is not last is not this plan's shape")
	}
	// A prefix entry with a PRODUCING EVENT is live on the simulated stack
	// where its event left it — beneath the region already, so the ordinary
	// seating seats both in order and this plan must stand aside.
	prior := core.NewInteger(7)
	prior.ID = core.GenerateID("zzrp")
	es.setProduced(prior, es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzprior", nout: 1}}))
	if _, ok := es.regionPrefixShape([]core.Value{prior, out}, es.frames[0]); ok {
		t.Error("an event-produced prefix entry is not inert")
	}
	// A residual whose last entry no event produced (a bare literal program).
	if _, ok := es.regionPrefixShape([]core.Value{core.NewInteger(1), core.NewInteger(2)}, es.frames[0]); ok {
		t.Error("a residual with no region has nothing to seat")
	}
}

func TestRegionPrefixShapeRejectsAStackReadingRegion(t *testing.T) {
	es := rpState(t)
	// The loop's BOUND is a prior event's result — live BELOW the mark the
	// plan would open, so the loop would pop from beneath its own mark.
	_, out := rpLoop(es, emitLoop{hasBodyOut: true, end: EmitOperand{kind: opEvent, idx: 1}})
	if _, ok := es.regionPrefixShape([]core.Value{core.NewInteger(99), out}, es.frames[0]); ok {
		t.Error("a region reading the enclosing stack must decline the plan")
	}

	// …but its FRAGMENT outs must not count. A computed loop body's residual
	// is an event operand INSIDE the body fragment; it never touches this
	// scope, and counting it would decline `99 for 2 [(1 add 2)]`.
	es2 := rpState(t)
	_, out2 := rpLoop(es2, emitLoop{
		hasBodyOut: true,
		bodyOut:    EmitOperand{kind: opEvent, idx: 1},
		condOut:    EmitOperand{kind: opEvent, idx: 2},
	})
	if _, ok := es2.regionPrefixShape([]core.Value{core.NewInteger(99), out2}, es2.frames[0]); !ok {
		t.Error("a loop's fragment outs are not enclosing-stack reads")
	}
}

func TestSingleSlotRegionCountsOnlyWholeRunSlots(t *testing.T) {
	es := rpState(t)

	// A value-producing loop: one recorded slot, a runtime count.
	seq, _ := rpLoop(es, emitLoop{hasBodyOut: true})
	if !es.singleSlotRegion(es.topLevelEventBySeq(seq)) {
		t.Error("a value-producing loop is a single-slot region")
	}

	// A SIDE-EFFECT loop leaves zero values deterministically (zeroOut) —
	// there is no run to seat anything under.
	zseq, _ := rpLoop(es, emitLoop{})
	zf := es.eventInfo[zseq]
	zf.zeroOut = true
	es.eventInfo[zseq] = zf
	if es.singleSlotRegion(es.topLevelEventBySeq(zseq)) {
		t.Error("a zero-output loop is not a region")
	}

	// A do-catch CALL is variadicResult but seats nout STATIC slots and
	// shrinks at run time — the mark plan has no single slot to work with,
	// so it must not be admitted on the variadicResult flag alone.
	cseq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzdo", nout: 2}})
	cf := es.eventInfo[cseq]
	cf.variadicResult = true
	es.eventInfo[cseq] = cf
	if es.singleSlotRegion(es.topLevelEventBySeq(cseq)) {
		t.Error("a shrinking do-catch is not a single-slot region")
	}

	// The GROWING call region (await's winner-takes-all) is one.
	cf.variadicRegion = true
	es.eventInfo[cseq] = cf
	if !es.singleSlotRegion(es.topLevelEventBySeq(cseq)) {
		t.Error("a variadicRegion call is a single-slot region")
	}

	// An ORDINARY call is not, and neither is a seq with no top-level event
	// (a fragment-resident producer, whose OpStackMark lowerEvents would
	// never emit).
	plain := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzadd", nout: 1}})
	if es.singleSlotRegion(es.topLevelEventBySeq(plain)) {
		t.Error("an ordinary call is not a region")
	}
	if es.singleSlotRegion(es.topLevelEventBySeq(plain + 1000)) {
		t.Error("a seq with no top-level event is not a region")
	}
}

// TestVariadicRegionEventAdmitsTheDoCatch pins the forty-seventh increment's
// widening at the seam, where the two predicates sit side by side and the
// difference between them is legible.
//
// singleSlotRegion asks "is the result ONE recorded slot standing for a
// runtime count?" — the COLLECT needs that, because its shape is a list
// literal over exactly one recorded operand. variadicRegionEvent asks only
// "is the run's length not the recorded count?", which is all the PREFIX
// needs: OpSeatBelowMark lifts the top n values down to the mark whatever
// lies between, so the run's length was never its business.
func TestVariadicRegionEventAdmitsTheDoCatch(t *testing.T) {
	es := rpState(t)

	// A do-catch region: a CALL event with nout recorded seats whose runtime
	// count is a different number.
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 2}})
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f
	ev := es.topLevelEventBySeq(seq)

	if es.singleSlotRegion(ev) {
		t.Error("a do-catch records nout slots — it is not a SINGLE-slot region")
	}
	if !es.variadicRegionEvent(ev) {
		t.Error("its run's length is not its recorded count, so the prefix seating must admit it")
	}

	// A value-producing LOOP is both: one slot, runtime count.
	lseq, _ := rpLoop(es, emitLoop{hasBodyOut: true, bodyOut: EmitOperand{kind: opLocal}})
	lev := es.topLevelEventBySeq(lseq)
	if !es.singleSlotRegion(lev) || !es.variadicRegionEvent(lev) {
		t.Error("a value-producing loop is a region on both questions")
	}

	// An ORDINARY call is neither — the widening must not reach it.
	oseq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzplain", nout: 2}})
	oev := es.topLevelEventBySeq(oseq)
	if es.singleSlotRegion(oev) || es.variadicRegionEvent(oev) {
		t.Error("a fixed-arity call is not a region on either question")
	}

	// A region that MAY CARRY A CALLABLE is excluded from both, unchanged:
	// only the interpreter re-steps one (NUR129).
	f = es.eventInfo[seq]
	f.regionMayBeFn = true
	es.eventInfo[seq] = f
	if es.variadicRegionEvent(ev) {
		t.Error("a possibly-callable region stays out of the prefix seating too")
	}
}
