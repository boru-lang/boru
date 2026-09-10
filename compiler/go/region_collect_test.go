package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// region_collect_test.go pins the PLAN side of the region COLLECT (NUR067's
// consuming half): which event PAIRS regionCollectShape admits.
//
// The safety argument is ADJACENCY, and it is the one thing a passing
// whole-program row cannot show. The mark opens before the region's event, so
// everything above it at run time has to be the region and nothing else; an
// event between the region and the list literal could leave a value there or
// take one from beneath. The shape therefore requires the two events to be
// neighbours in the top-level frame, and this file pins that they must be.
//
// lang/go/region_collect_test.go is the whole-program half; eng/go's
// vm_make_list_to_mark_test.go is the VM half.

// rcList appends a list-literal event over the operands given.
func rcList(es *EmitState, ops ...EmitOperand) int {
	return es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "__makelist", makeList: true, ops: ops, nout: 1}})
}

func rcRegionOp(seq int) EmitOperand { return EmitOperand{kind: opEvent, idx: seq} }

func TestRegionCollectShapeWantsAnAdjacentPair(t *testing.T) {
	es := rpState(t)
	region, _ := rpLoop(es, emitLoop{hasBodyOut: true, bodyOut: EmitOperand{kind: opLocal}})
	list := rcList(es, rcRegionOp(region))

	gotR, gotL, ok := es.regionCollectShape(es.frames[0])
	if !ok || gotR != region || gotL != list {
		t.Fatalf("regionCollectShape = (%d,%d,%v), want (%d,%d,true)", gotR, gotL, ok, region, list)
	}
}

func TestRegionCollectShapeDeclinesTheNearMisses(t *testing.T) {
	// An event BETWEEN the region and the list: it could leave a value above
	// the mark or take one from beneath, so the pair is no longer safe.
	es := rpState(t)
	region, _ := rpLoop(es, emitLoop{hasBodyOut: true})
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzbetween", nout: 1}})
	rcList(es, rcRegionOp(region))
	if _, _, ok := es.regionCollectShape(es.frames[0]); ok {
		t.Error("a non-adjacent pair must decline")
	}

	// A list literal with a SECOND element (`[9 (for 3 [i])]`): its other
	// element would have to seat either side of the run.
	es2 := rpState(t)
	region2, _ := rpLoop(es2, emitLoop{hasBodyOut: true})
	rcList(es2, EmitOperand{kind: opConst}, rcRegionOp(region2))
	if _, _, ok := es2.regionCollectShape(es2.frames[0]); ok {
		t.Error("a mixed list literal must decline")
	}

	// A list literal over SOMETHING ELSE that happens to follow a region.
	es3 := rpState(t)
	rpLoop(es3, emitLoop{hasBodyOut: true})
	rcList(es3, EmitOperand{kind: opConst})
	if _, _, ok := es3.regionCollectShape(es3.frames[0]); ok {
		t.Error("a list literal not over the region must decline")
	}

	// An ordinary CALL following the region is not a list literal at all —
	// its operand would have to be popped at a static depth.
	es4 := rpState(t)
	region4, _ := rpLoop(es4, emitLoop{hasBodyOut: true})
	es4.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzsize", ops: []EmitOperand{rcRegionOp(region4)}, nout: 1}})
	if _, _, ok := es4.regionCollectShape(es4.frames[0]); ok {
		t.Error("a plain call over the region must decline")
	}

	// A region whose own operand is live on the enclosing stack: the mark
	// would open above a value the region then pops (regionReadsTheStack).
	es5 := rpState(t)
	region5, _ := rpLoop(es5, emitLoop{hasBodyOut: true, end: EmitOperand{kind: opEvent, idx: 1}})
	rcList(es5, rcRegionOp(region5))
	if _, _, ok := es5.regionCollectShape(es5.frames[0]); ok {
		t.Error("a region reading the enclosing stack must decline")
	}

	// A list literal over an ORDINARY call's result: nothing to collect.
	es6 := rpState(t)
	plain := es6.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzadd", nout: 1}})
	out := core.NewInteger(3)
	out.ID = core.GenerateID("zzrc")
	es6.setProduced(out, plain)
	rcList(es6, rcRegionOp(plain))
	if _, _, ok := es6.regionCollectShape(es6.frames[0]); ok {
		t.Error("a list literal over a fixed result is the ordinary MAKE_LIST")
	}

	// A frame with a single event has no pair at all.
	es7 := rpState(t)
	rpLoop(es7, emitLoop{hasBodyOut: true})
	if _, _, ok := es7.regionCollectShape(es7.frames[0]); ok {
		t.Error("a lone region has nothing to collect it")
	}
}
