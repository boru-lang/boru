package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// callDynMixedFromMark executes OpCallDynMixedFromMark — the variadic-region
// verbatim window (plan Phase 5, L-DO part 2b): island stack[mark:] through
// the SAME re-step machinery as CALL_DYNAMIC_MIXED, with the window width
// decided by the RUNTIME region (a fallible do-catch residual: 1 caught Error
// vs N values) plus the fixed values the preceding events pushed above it.
// The topmost mark is consumed; an empty window is a no-op (the mark still
// pops). Extracted from the run loop's dispatch for the cognitive-complexity
// cap (the dispatchRematch precedent).
// vmMarkOp routes the mark-family opcodes: the three bookkeeping ops go to
// vmMark; the mark-window island needs the vmContext (its island runner), so
// it dispatches here — keeping the run loop's mark case a single line under
// the cognitive-complexity cap.
func (vc *vmContext) vmMarkOp(reg *core.Registry, op compiler.Opcode, arg int, marks []int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]int, []core.Value, error) {
	if op == compiler.OpCallDynMixedFromMark {
		return vc.callDynMixedFromMark(reg, marks, stack, curDebug, pc)
	}
	if op == compiler.OpSeatBelowMark {
		return vmSeatBelowMark(arg, marks, stack, curDebug, pc)
	}
	if op == compiler.OpMakeListToMark {
		return vmMakeListToMark(marks, stack, curDebug, pc)
	}
	return vmMark(op, marks, stack, curDebug, pc)
}

// vmMakeListToMark collects a region into one List (OpMakeListToMark): pop the
// innermost mark m and replace stack[m:] with a single List holding those
// values in order. It is OpMakeList with the element count taken from the mark
// instead of from Arg, and it strips ascriptions for the same reason
// OpMakeList does — list elements are stored data.
func vmMakeListToMark(marks []int, stack []core.Value, debug []core.SrcPos, pc int) ([]int, []core.Value, error) {
	if len(marks) == 0 {
		return marks, stack, vmErrAt(debug, pc, "MAKE_LIST_TO_MARK with no open mark")
	}
	m := marks[len(marks)-1]
	marks = marks[:len(marks)-1]
	if m > len(stack) {
		return marks, stack, vmErrAt(debug, pc, "MAKE_LIST_TO_MARK above current depth")
	}
	elems := make([]core.Value, len(stack)-m)
	for i, v := range stack[m:] {
		elems[i] = core.StripAscribed(v)
	}
	return marks, append(stack[:m], core.NewList(elems)), nil
}

// vmSeatBelowMark closes a region whose residual carries an INERT PREFIX
// underneath it (NUR067's consuming half, OpSeatBelowMark). The prefix was
// pushed ABOVE the finished region — the only place a static lowering can put
// it, since the region's count is a runtime value — so this op moves the top
// n values down to the mark and lifts the run above them:
//
//	before  [ … | region₀ … regionₖ₋₁ , prefix₀ … prefixₙ₋₁ ]
//	after   [ … | prefix₀ … prefixₙ₋₁ , region₀ … regionₖ₋₁ ]
//
// with the mark at the │ and k — the region's length — never named. Both runs
// keep their order, and the mark is consumed.
func vmSeatBelowMark(n int, marks []int, stack []core.Value, debug []core.SrcPos, pc int) ([]int, []core.Value, error) {
	if len(marks) == 0 {
		return marks, stack, vmErrAt(debug, pc, "SEAT_BELOW_MARK with no open mark")
	}
	m := marks[len(marks)-1]
	marks = marks[:len(marks)-1]
	if n < 0 || len(stack)-n < m {
		return marks, stack, vmErrAt(debug, pc, "SEAT_BELOW_MARK prefix reaches past the mark")
	}
	prefix := append([]core.Value(nil), stack[len(stack)-n:]...)
	region := append([]core.Value(nil), stack[m:len(stack)-n]...)
	stack = append(stack[:m], prefix...)
	return marks, append(stack, region...), nil
}

func (vc *vmContext) callDynMixedFromMark(reg *core.Registry, marks []int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]int, []core.Value, error) {
	if len(marks) == 0 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_MIXED_FROM_MARK with no open mark")
	}
	base := marks[len(marks)-1]
	marks = marks[:len(marks)-1]
	if base > len(stack) {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_MIXED_FROM_MARK mark above stack top")
	}
	if w := len(stack) - base; w > 0 {
		var err error
		if stack, err = vc.callDynamicMixed(reg, w, stack, curDebug, pc); err != nil {
			return nil, nil, err
		}
	}
	return marks, stack, nil
}
