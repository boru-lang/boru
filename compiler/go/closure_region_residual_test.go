package compiler

import (
	"testing"
)

// closure_region_residual_test.go pins closureResidualRegion — the screen the
// fifty-seventh increment widened, and the one that decides whether a
// whole-residual dispatch (`do`) may compile a body whose residual count is
// not the check run's.
//
// The predicate is read off a PROBE unit, so no whole-program row shows its
// declines directly: a program whose probe declines simply falls to the
// dyn-body strategy, which compiles, so the difference is invisible in the
// answer and visible only in TestInterpEntryCensus. That is why the arms are
// tested here at the seam, the way region_operand_screen_test.go tests the
// screens NUR133 turned on.

// crrUnit appends a fn unit whose fragment holds ONE region event and whose
// residual is `prefix` inert operands beneath that event's run.
func crrUnit(es *EmitState, prefix, run int) int {
	seq := es.appendEvent(EmitEvent{kind: evLoop, loop: &emitLoop{hasBodyOut: true, bodyOut: EmitOperand{kind: opLocal}}})
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f

	ops := make([]EmitOperand, 0, prefix+run)
	for i := 0; i < prefix; i++ {
		ops = append(ops, ConstOperand(i))
	}
	for i := 0; i < run; i++ {
		ops = append(ops, EventOperand(seq, i))
	}
	rec := &fnUnitRec{name: "do$body", frag: &EmitFragment{events: es.frames[0]}, outOps: ops}
	es.fnRecs = append(es.fnRecs, rec)
	return len(es.fnRecs) - 1
}

func TestClosureResidualRegionAdmitsThePrefixShape(t *testing.T) {
	es := rpState(t)
	unit := crrUnit(es, 2, 1)
	if !closureResidualRegion(es, unit) {
		t.Fatal("[inert inert REGION] is the shape OpSeatBelowMark seats; it must be admitted")
	}
	// The prefix is a run, not a slot, and so is the region's own recorded
	// seat count (a do-catch records nout seats for a different runtime
	// length).
	if !closureResidualRegion(es, crrUnit(es, 1, 2)) {
		t.Error("a multi-seat region under a one-value prefix must be admitted")
	}
	// The WHOLE-RUN shape: the residual IS the region, no prefix beneath it.
	// regionPrefixShapeOps declines this one ("the run IS the whole residual;
	// no prefix to seat") and the second arm takes it — seatResults leaves
	// such a run where it lands, whatever its length (sameEventRunToEnd).
	//
	// It is not an academic arm. An ENCLOSING whole-residual body whose only
	// content is an inner one of the prefix shape has exactly this residual,
	// and without the arm the relaxation stops one level short: `do [def b
	// true  do [1 2 (if b [] [9 9])]]` compiles its inner body, declines its
	// outer one, and refuses at the twin-placement gate the outer body's def
	// needs.
	if !closureResidualRegion(es, crrUnit(es, 0, 1)) {
		t.Error("a residual that IS the region must be admitted")
	}
	if !closureResidualRegion(es, crrUnit(es, 0, 3)) {
		t.Error("a multi-seat region alone must be admitted")
	}
}

func TestClosureResidualRegionDeclines(t *testing.T) {
	es := rpState(t)
	for _, tc := range []struct {
		name string
		unit func() int
	}{
		// A run that is not contiguous to the END of the residual is neither
		// shape: an inert value ABOVE the run would have to be seated on top
		// of a length nothing knows.
		{"an inert value above the run", func() int {
			u := crrUnit(es, 0, 2)
			es.fnRecs[u].outOps = append(es.fnRecs[u].outOps, ConstOperand(7))
			return u
		}},
		// A run whose event is NOT a region: its recorded count IS its
		// runtime count, so the exactness screen owns it and a miscount here
		// would be a real one.
		{"a run of a non-region event", func() int {
			u := crrUnit(es, 0, 1)
			seq := u // any seq with no variadic mark
			es.fnRecs[u].outOps = []EmitOperand{EventOperand(seq+1000, 0)}
			es.fnRecs[u].frag = &EmitFragment{events: []EmitEvent{{kind: evCall, seq: seq + 1000}}}
			return u
		}},
		// A dynamic-apply tail and a dyn-frame window both post-process the
		// SEATED layout, so a run of unknown length is not a layout they can
		// work from — they decline before the shape is even read.
		{"a dynamic-apply tail", func() int {
			u := crrUnit(es, 2, 1)
			es.fnRecs[u].dynTrailArity = 1
			return u
		}},
		{"a dyn-frame window", func() int {
			u := crrUnit(es, 2, 1)
			es.fnRecs[u].dynFrameW = 1
			return u
		}},
		// A unit whose fragment never opened has no events to read the shape
		// off; the exactness screen owns it.
		{"no fragment", func() int {
			u := crrUnit(es, 2, 1)
			es.fnRecs[u].frag = nil
			return u
		}},
		// An entirely inert residual is EXACT, which closureResidualExact
		// admits first; this predicate is only ever asked after that declined.
		{"an inert residual", func() int {
			u := crrUnit(es, 2, 1)
			es.fnRecs[u].outOps = []EmitOperand{ConstOperand(1), ConstOperand(2)}
			return u
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if closureResidualRegion(es, tc.unit()) {
				t.Fatal("closureResidualRegion admitted a shape the unit's seating cannot lay out")
			}
		})
	}
}

// The three out-of-band arms: a pass with no concrete recorder, and a unit
// index either side of the table. Each is the same "there is nothing to read"
// answer the exactness screen beside it gives.
func TestClosureResidualRegionOutOfBand(t *testing.T) {
	es := rpState(t)
	unit := crrUnit(es, 2, 1)
	if closureResidualRegion(nil, unit) {
		t.Error("a pass with no recorder has no unit table")
	}
	if closureResidualRegion(es, -1) {
		t.Error("a declined StartFnCompile returns -1, which names no unit")
	}
	if closureResidualRegion(es, len(es.fnRecs)) {
		t.Error("a unit index past the table names no unit")
	}
}
