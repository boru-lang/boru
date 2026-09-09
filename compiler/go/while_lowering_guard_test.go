package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// while_lowering_guard_test.go pins the two DEFENSIVE guards the thirty-seventh
// increment (`while` lowering) added to lower.go, both of which sit off the path
// a well-formed recorded `while` trace takes:
//
//   - lowerLoop's CONDITION-fragment refusal arm: a condition loop whose
//     condition fragment fails to lower must POP its own loop context before
//     returning the reason, so the refusal is clean — an enclosing loop's
//     context survives untouched and no half-open frame is left for a later
//     break/continue to patch. RecordWhile only ever records a net-one
//     condition, so the arm is reached here through a hand-built EmitEvent on
//     the direct-lowerer seam lower_seam8_test.go already uses for lowerLoop's
//     sibling refusals (variadic bound, carried init).
//   - fragmentOuts's nil-loop guard for an evLoop event: the planner walks
//     every event's fragment-outs, and a loop event without its payload must
//     contribute no out rather than dereference nil.
//
// Each guard is paired with its non-guard twin (a condition that DOES lower, a
// populated loop event that DOES yield outs), so a regression that
// short-circuits the whole arm cannot pass unnoticed.

// wlgLowerer builds a bare lowerer over an empty program, ready for direct
// method calls (mirrors lower_seam8_test.go's w8lw).
func wlgLowerer() *lowerer {
	p := &Program{}
	code := []Instr{}
	debug := []core.SrcPos{}
	return &lowerer{
		es: NewEmitState(), p: p, code: &code, debug: &debug,
		sigIdx:   map[*core.Signature]int{},
		variadic: map[int]bool{},
		promoted: map[int]int{},
		dead:     map[int]bool{},
	}
}

// wlgWhileEvent is a const-bounded CONDITION loop — the shape RecordWhile
// records, an unbounded counted frame whose real exit is the condition test.
// Its condition fragment leaves the result of event seq 5 on the simulated
// stack; condOut is the operand the loop claims that residual with.
func wlgWhileEvent(condOut EmitOperand) *EmitEvent {
	// A make-list call over no operands: one pushed result slot, no operands
	// consumed — the smallest event that gives the condition fragment a residual.
	push := EmitEvent{seq: 5, kind: evCall, call: emitCall{makeList: true, nout: 1}}
	return &EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{
		start: ConstOperand(0), end: ConstOperand(3), step: ConstOperand(1),
		iterSlot: 0,
		cond:     &EmitFragment{events: []EmitEvent{push}},
		condOut:  condOut,
		body:     &EmitFragment{},
	}}
}

// wlgHasOp reports whether the lowered stream contains op.
func wlgHasOp(code []Instr, op Opcode) bool {
	for _, in := range code {
		if in.Op == op {
			return true
		}
	}
	return false
}

func TestWhileConditionFragmentRefusalPopsTheLoopContext(t *testing.T) {
	lw := wlgLowerer()
	// An ENCLOSING loop context, so the pop can be shown to take exactly one
	// frame — the condition loop's own — and not the outer loop's.
	outerHoles := []int{}
	lw.loops = []loopCtx{{nextPC: 42, endHoles: &outerHoles}}

	// The condition fragment's events leave seq 5 on the sim, but the recorded
	// condOut names seq 9: the fragment cannot seat that result and refuses.
	reason := lw.lowerLoop(wlgWhileEvent(EventOperand(9, 0)))
	if !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("condition-fragment refusal reason = %q, want the fragment's refusal", reason)
	}
	if len(lw.loops) != 1 || lw.loops[0].nextPC != 42 {
		t.Fatalf("loops after the refusal = %+v, want only the outer frame (nextPC 42)", lw.loops)
	}
	// The refusal happened at the condition test, before the loop could branch
	// on it: no OpJmpIfFalse (and so no OpFlowBreak exit) was emitted.
	if wlgHasOp(*lw.code, OpJmpIfFalse) {
		t.Error("a refused condition must not emit the condition's branch")
	}

	// The twin: the same loop, whose condOut DOES name the condition's
	// residual, lowers cleanly — the condition branch and its FLOW_BREAK exit
	// are emitted — and pops its own frame on the way out too.
	lw2 := wlgLowerer()
	lw2.loops = []loopCtx{{nextPC: 42, endHoles: &outerHoles}}
	if r := lw2.lowerLoop(wlgWhileEvent(EventOperand(5, 0))); r != "" {
		t.Fatalf("a well-formed condition loop should lower cleanly, got %q", r)
	}
	if len(lw2.loops) != 1 || lw2.loops[0].nextPC != 42 {
		t.Fatalf("loops after a clean lowering = %+v, want only the outer frame", lw2.loops)
	}
	if !wlgHasOp(*lw2.code, OpJmpIfFalse) || !wlgHasOp(*lw2.code, OpFlowBreak) {
		t.Errorf("a lowered condition loop must emit its branch and FLOW_BREAK exit, got %v", *lw2.code)
	}
}

func TestFragmentOutsNilLoop(t *testing.T) {
	// A loop event without its payload contributes no fragment-outs.
	if outs := fragmentOuts(&EmitEvent{kind: evLoop, loop: nil}); outs != nil {
		t.Fatalf("nil-loop fragmentOuts = %v, want nil", outs)
	}
	// The twin: a populated condition loop yields the [cond, body] pair, both
	// pointing at the loop's own recorded operands.
	ev := &EmitEvent{kind: evLoop, loop: &emitLoop{
		cond: &EmitFragment{}, condOut: EventOperand(5, 0),
		bodyOut: EventOperand(6, 0), hasBodyOut: true,
	}}
	outs := fragmentOuts(ev)
	if len(outs) != 2 || outs[0] != &ev.loop.condOut || outs[1] != &ev.loop.bodyOut {
		t.Fatalf("populated loop fragmentOuts = %v, want the condOut/bodyOut pair", outs)
	}
}
