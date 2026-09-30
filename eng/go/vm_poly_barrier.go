package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// A BARRED poly (NUR362) is one some candidate of which stops its forward
// collection short of the operands the dispatch took from after its word:
// the candidate's barrier (`get`'s `[Key | Node]`) is below the written
// count and below its own arity (core.BarrierBars). The interpreter's plan
// for such a candidate takes the leading operands up to its barrier and the
// rest off the stack beneath the word — another window — so the flat
// re-match callPoly makes, which reads the window as if any candidate could
// take it, is not the interpreter's match: `get (h) {k:1}` took the
// `[String Node]` overload over both written operands and answered 1 where
// the interpreter's plan finds the stack beneath `get` empty and raises.
//
// The record carries the dispatch's exact layout for such a poly
// (PolyRef.Split; tryRecordPoly declines one without it), so the run lays
// the operands out as the interpreter's tape and asks its own plan: no plan
// raises the interpreter's signature_error over that tape; a plan over the
// window's own operands in the window's own positions dispatches that
// overload; any other plan — a window the program never assembled — defers.

// polyBarred reports whether pr's window is barred for sigs.
func polyBarred(pr *compiler.PolyRef, sigs []core.Signature) bool {
	return pr.Split != nil && core.BarrierBars(sigs, pr.Split.NFwd)
}

// callPolyPlanned dispatches a barred poly by the interpreter's own plan
// over the recorded layout (see above). window is the poly's operands in
// signature order.
func (vc *vmContext) callPolyPlanned(dispReg *core.Registry, pr *compiler.PolyRef, fn *core.FnDefInfo, window, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	r := dispReg
	n := len(window)
	sp := pr.Split
	if beneath, ok := splitBeneath(sp, stack, vc.restartLocals); ok && sp.NFwd <= n {
		_, sig, positions, planned := planSplitOver(r, pr.Word, fn, beneath, window[sp.NFwd:], window[:sp.NFwd], sp.After)
		switch {
		case planned && (sig == nil || sig.Fallback):
			// No plan: the interpreter's signature_error, over its tape.
			if err := polySplitRaise(r, pr, fn, pr.RenderWindow(window), stack, vc.restartLocals, curDebug, pc); err != nil {
				vc.polyUnmatched = true
				return nil, err
			}
		case planned && planTakesWindow(sig, positions, len(beneath), sp.NFwd, n) &&
			core.MatchSignature([]core.Signature{*sig}, window, core.WordInfo{ArgCount: n}) != nil:
			return vc.polyDispatch(dispReg, pr, sig, window, n, stack, curDebug, pc)
		}
	}
	return nil, vmDefer(r, curDebug, pc, "vm:poly-barrier",
		"`"+pr.Word+"`'s dispatch plans a window the compiled call's operands do not hold (NUR362)")
}

// matchUserPolyPlanned resolves a barred OpCallUserPoly (UserPolyRef.Split)
// by the interpreter's own plan over the call's layout and the live binding
// fd: no plan raises the interpreter's signature_error; a plan over the
// window that picks one of the recorded arms (fd's signature pr.SigIdx[k],
// whose unit is units[k] — the live mode's parallel slices) enters that
// arm's unit; anything else defers.
func (vc *vmContext) matchUserPolyPlanned(pr *compiler.UserPolyRef, fd *core.FnDefInfo, units []int, window, stack []core.Value, curDebug []core.SrcPos, pc int) (int, []core.Value, error) {
	r := vc.r
	n := len(window)
	sp := pr.Split
	if beneath, ok := splitBeneath(sp, stack, vc.restartLocals); ok && sp.NFwd <= n {
		_, sig, positions, planned := planSplitOver(r, pr.Word, fd, beneath, window[sp.NFwd:], window[:sp.NFwd], sp.After)
		if planned && (sig == nil || sig.Fallback) {
			if err := splitNoMatchAt(r, pr.Word, fd, window, sp.NFwd, beneath, sp.After, sp.Words, pr.WordPos, curDebug, pc); err != nil {
				return 0, nil, err
			}
		}
		if planned && planTakesWindow(sig, positions, len(beneath), sp.NFwd, n) &&
			core.MatchSignature([]core.Signature{*sig}, window, core.WordInfo{ArgCount: n}) != nil {
			for k, si := range pr.SigIdx {
				if k < len(units) && si >= 0 && si < len(fd.Signatures) && &fd.Signatures[si] == sig {
					return units[k], window, nil
				}
			}
		}
	}
	return 0, nil, vmDefer(r, curDebug, pc, "vm:user-poly-barrier",
		"`"+pr.Word+"`'s dispatch plans a window the compiled call's operands do not hold (NUR362)")
}

// planTakesWindow reports whether a plan over the laid-out tape — beneath
// values, then the stack operands bottom-up, the word, the written operands
// — took exactly the window: nFwd written operands in signature positions
// 0..nFwd-1, the stack operands top first after them, over an overload of
// the window's arity that dispatches.
func planTakesWindow(sig *core.Signature, positions []int, nBeneath, nFwd, n int) bool {
	if sig == nil || sig.Fallback || sig.TotalArgs() != n || len(positions) != n || sig.DispatchHandler() == nil {
		return false
	}
	at := nBeneath + n - nFwd
	for i, p := range positions {
		want := at + 1 + i
		if i >= nFwd {
			want = at - 1 - (i - nFwd)
		}
		if p != want {
			return false
		}
	}
	return true
}
