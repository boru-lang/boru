package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// region_operand_screen_test.go pins the three predicates NUR133's fix turns
// on, at the seam. Each is a plain function over one event, and each has an
// arm no whole-program row can reach today — which is the point: the arms
// exist so that a LATER narrowing somewhere else cannot silently reopen the
// hole they close.
//
//   - regionReadsTheStack's evFallback arm is currently unreachable from a
//     program, because RecordFallback marks every island region
//     possibly-callable and variadicRegionEvent / singleSlotRegion both test
//     that first. The arm is kept, and tested here, because those two fixes
//     are independent: narrow regionMayBeFn and the operand screen is the
//     only thing left standing between a threaded fallback input and a mark
//     opened above it.
//   - variadicRegionEvent's nil arm is what lets regionPrefixShape check it
//     BEFORE regionReadsTheStack, so the latter is never handed a nil event.
//   - seatRegionPrefix's "nothing to seat" guard closes the two shapes the
//     trailing-run walk can land on: a residual that is ALL region, and one
//     with no region operand at the end at all.

// rosEvent builds a bare event of one kind with the operands that kind reads.
func rosEvent(kind int, ops []EmitOperand) *EmitEvent {
	ev := &EmitEvent{kind: kind}
	switch kind {
	case evCallUser:
		ev.uc = emitUserCall{ops: ops}
	case evFallback:
		ev.fb = emitFallback{ins: ops}
	case evLoop:
		// A loop reads start/end/step plus every CARRIED slot's init; the
		// fragments' own outs (condOut / bodyOut) are deliberately NOT read
		// here, which is why this cannot reuse forEachOperand.
		lp := emitLoop{}
		if len(ops) > 0 {
			lp.end = ops[0]
		}
		for _, op := range ops[1:] {
			lp.carried = append(lp.carried, carriedInit{init: op})
		}
		ev.loop = &lp
	default:
		ev.call = emitCall{ops: ops}
	}
	return ev
}

func TestRegionReadsTheStackByEventKind(t *testing.T) {
	eventOp := EventOperand(7, 0)
	constOp := ConstOperand(3)
	for _, tc := range []struct {
		name string
		kind int
		ops  []EmitOperand
		want bool
	}{
		// The two kinds the predicate was written for.
		{"a native call over a const", evCall, []EmitOperand{constOp}, false},
		{"a native call over an event", evCall, []EmitOperand{eventOp}, true},
		{"a loop bound that is a const", evLoop, []EmitOperand{constOp}, false},
		{"a loop bound that is an event", evLoop, []EmitOperand{eventOp}, true},
		{"a loop carried init that is an event", evLoop, []EmitOperand{constOp, eventOp}, true},
		{"a loop carried init that is a const", evLoop, []EmitOperand{constOp, constOp}, false},
		// The two admitted since, which went unscreened (NUR133).
		{"a user call over consts", evCallUser, []EmitOperand{constOp, constOp}, false},
		{"a user call over an event", evCallUser, []EmitOperand{constOp, eventOp}, true},
		{"a fallback with no threaded input", evFallback, nil, false},
		{"a fallback whose input is an event", evFallback, []EmitOperand{eventOp}, true},
		// A CLOSURE operand counts too: its captures may themselves be
		// enclosing-stack results.
		{"a closure operand", evCall, []EmitOperand{{kind: opClosure}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := regionReadsTheStack(rosEvent(tc.kind, tc.ops)); got != tc.want {
				t.Fatalf("regionReadsTheStack = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVariadicRegionEventNil(t *testing.T) {
	es := rpState(t)
	// regionPrefixShape checks this FIRST precisely so regionReadsTheStack is
	// never handed a nil — topLevelEventBySeq returns one for a seq that is
	// not a top-level event.
	if es.variadicRegionEvent(nil) {
		t.Fatal("a nil event is not a region")
	}
}

func TestSeatRegionPrefixNothingToSeat(t *testing.T) {
	region := 5
	for _, tc := range []struct {
		name string
		ops  []EmitOperand
	}{
		// The residual is ALL region: the trailing-run walk consumes every
		// operand, so there is no prefix under it.
		{"every operand is the region", []EmitOperand{EventOperand(region, 0), EventOperand(region, 0)}},
		// Nothing at the end belongs to the region: the walk consumes none,
		// so this is not the shape the plan armed for.
		{"no region operand at the tail", []EmitOperand{ConstOperand(1), ConstOperand(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lw := w8lw()
			lw.regionPrefixSeq = region
			lw.vm = []vmSlot{{seq: region}}
			before := len(*lw.code)
			if lw.seatRegionPrefix(tc.ops, core.SrcPos{}) {
				t.Fatal("must decline: there is no inert prefix to seat")
			}
			if len(*lw.code) != before {
				t.Errorf("a declined seating must emit nothing, got %v", *lw.code)
			}
		})
	}
}
