package compiler

import "testing"

// opsHaveVariadicResult is the screen a fn RET needs and the program
// residual does not: an event whose RESULT COUNT is runtime-variable cannot
// be spilled to one frame local, because the slot would hold one value for a
// run of a different length. The program residual absorbs such an event; a
// RET does not, so reconcileResults asks this before taking the rebuild.
func TestOpsHaveVariadicResult(t *testing.T) {
	bare := &lowerer{}
	if bare.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0)}) {
		t.Fatal("a lowerer with no EmitState can screen nothing; it must not claim a variadic")
	}
	es := NewEmitState()
	es.eventInfo = map[int]eventFlags{
		1: {},
		2: {variadicResult: true},
	}
	lw := &lowerer{es: es}
	if lw.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0), ConstOperand(0)}) {
		t.Fatal("a fixed-count event and a const are not variadic")
	}
	if !lw.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0), EventOperand(2, 0)}) {
		t.Fatal("an event marked variadicResult must be found wherever it sits")
	}
}

// regionPrefixShapeOps is the OPERAND form of the region-prefix shape, which
// a fn unit needs because the residual VALUES a body leaves carry identities
// producedBy does not index (a branch merge's carrier is minted at the join).
// Its three conditions, each in both directions.
func TestRegionPrefixShapeOps(t *testing.T) {
	es := NewEmitState()
	es.eventInfo = map[int]eventFlags{
		4: {variadicResult: true}, // a region
		5: {},                     // a fixed-count event
		6: {variadicResult: true, regionMayBeFn: true},
	}
	evs := []EmitEvent{
		{kind: evCall, seq: 4, call: emitCall{}},
		{kind: evCall, seq: 5, call: emitCall{}},
		{kind: evCall, seq: 6, call: emitCall{}},
	}
	region := EventOperand(4, 0)

	seq, ok := es.regionPrefixShapeOps([]EmitOperand{ConstOperand(0), region}, evs)
	if !ok || seq != 4 {
		t.Fatalf("[const, REGION] = (%d, %v), want the region's seq", seq, ok)
	}
	// The run may be more than one operand of the SAME event; the prefix is
	// whatever sits beneath it.
	if seq, ok := es.regionPrefixShapeOps(
		[]EmitOperand{ConstOperand(0), ConstOperand(1), region, EventOperand(4, 1)}, evs); !ok || seq != 4 {
		t.Fatalf("[const, const, RUN…] = (%d, %v), want the region's seq", seq, ok)
	}
	// Too short, or not ending in an event at all.
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{region}, evs); ok {
		t.Fatal("a residual that IS the run has no prefix to seat")
	}
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{region, ConstOperand(0)}, evs); ok {
		t.Fatal("a residual not ending in the region is not this shape")
	}
	// The last event is not a region, or is one whose run may leave a
	// callable (nothing downstream can re-step such a value).
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{ConstOperand(0), EventOperand(5, 0)}, evs); ok {
		t.Fatal("a fixed-count event is not a region")
	}
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{ConstOperand(0), EventOperand(6, 0)}, evs); ok {
		t.Fatal("a region whose run may leave a callable must decline")
	}
	// An EVENT beneath the run is not an inert prefix: it cannot be re-pushed
	// after the run, which is the whole premise of the seat.
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{EventOperand(5, 0), region}, evs); ok {
		t.Fatal("an event beneath the run must decline")
	}
	// An anchor resident in a NESTED fragment is not in this scope's event
	// list, so the plan would arm with no OpStackMark ever emitted.
	if _, ok := es.regionPrefixShapeOps([]EmitOperand{ConstOperand(0), EventOperand(9, 0)}, evs); ok {
		t.Fatal("an anchor outside this scope's events must decline")
	}
}
