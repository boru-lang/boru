package compiler

import core "github.com/boru-lang/boru/core/go"

// The no-match window of a compiled user call (NUR320).
//
// A compiled CALL_USER guards its param contract at run time
// (checkParamContract in eng): a gradual operand the pass admitted may not
// fit. The interpreter raises that same signature_error from its failed
// dispatch, and its notes describe the window the dispatch ATTEMPTED
// (core's attemptedWindowOver) — the concrete tokens written after the
// word, up to the first bare word read, filled from the stack beneath to
// the smallest overload's arity — not the arguments the compiled call
// holds. `f e` with e a param holding 5 reports "takes 1 argument, but
// none were supplied" there, because the read is looked up at the dispatch
// and never lands in the window; `7 f e` reports the 7 beneath.
//
// The pass derives that window at the dispatch's first step, over its own
// tape (core's noteCallWindow), and offers it here under the dispatching
// word token — the region offer's key, held at the user-fn ReturnsFn's
// entry beside the region (HoldRegion) for the same reason. The call's
// record maps each window value to where it lives at run time: an argument
// by identity, a definite scalar by value, an event result of the unit by
// its seat (resolved at lowering, seatCallWindow). A value with no such
// home — a carrier read from a local, a compound, an enclosing unit's
// result — leaves the call with no window, and the contract reports the
// arguments as before.

// pendingWindow is one NoteCallWindow offer.
type pendingWindow struct {
	win []core.Value
	// nFwd counts win's leading entries written after the word, and prefix
	// is the stack prefix beneath it (NoteCallWindow, NUR311).
	nFwd   int
	prefix []core.Value
	// known marks a derived window; a speculative plan's dispatch offers
	// none (its runtime twin can fail at the re-step instead).
	known bool
	// deferred marks a dispatch that goes on to collect forward: its
	// force-stack re-step keeps this offer.
	deferred bool
	ok       bool
	// stable marks, per window value, a def read whose binding had not
	// moved when the dispatch offered the window (residualReadStable). It is
	// judged THEN: the record runs after the callee's body is analysed, and
	// that analysis pushes and pops the callee's own params — a same-named
	// binding's generation moves though the caller's never did.
	stable []bool
	// prefixStable is stable for prefix.
	prefixStable []bool
}

// callWinOp is one window value as the record resolved it: kind and idx as
// the lowered CallWindowOperand's, except that a WinStack entry is still
// the event op names — the lowering seats it (seatCallWindow). fwd and
// prefixOnly are the operand's Fwd and PrefixOnly marks.
type callWinOp struct {
	kind       CallWindowKind
	idx        int
	value      core.Value
	op         EmitOperand
	fwd        bool
	prefixOnly bool
}

// NoteCallWindow pools a dispatch's no-match window under its word token
// (core.EmitRecorder). A re-step (restep) over a deferred offer keeps that
// offer: the window is the first step's. A suspended recorder (not Active)
// pools nothing and drops any offer under the key — the record it may
// still fire must claim no window rather than an earlier execution's.
func (es *EmitState) NoteCallWindow(word string, pos core.SrcPos, window []core.Value, nFwd int, prefix []core.Value, deferred, restep bool) {
	if es == nil {
		return
	}
	if !es.Active() {
		delete(es.pendingWindows, keyOf(word, pos))
		return
	}
	k := keyOf(word, pos)
	if prev, ok := es.pendingWindows[k]; ok && restep && prev.deferred {
		prev.deferred = false
		es.pendingWindows[k] = prev
		return
	}
	if es.pendingWindows == nil {
		es.pendingWindows = map[regionKey]pendingWindow{}
	}
	stable := make([]bool, len(window))
	for i, v := range window {
		stable[i] = es.residualReadStable(v)
	}
	prefixStable := make([]bool, len(prefix))
	for i, v := range prefix {
		prefixStable[i] = es.residualReadStable(v)
	}
	es.pendingWindows[k] = pendingWindow{win: window, nFwd: nFwd, prefix: prefix, known: window != nil, deferred: deferred, ok: true, stable: stable, prefixStable: prefixStable}
}

// takePendingWindow removes and returns the offer for (word, pos).
func (es *EmitState) takePendingWindow(word string, pos core.SrcPos) pendingWindow {
	k := keyOf(word, pos)
	w, ok := es.pendingWindows[k]
	if ok {
		delete(es.pendingWindows, k)
	}
	return w
}

// claimHeldWindow is the record's window: the innermost hold's when the
// record runs under a hold for its key, else the pool's.
func (es *EmitState) claimHeldWindow(word string, pos core.SrcPos) pendingWindow {
	if n := len(es.heldRegions); n > 0 && es.heldRegions[n-1].key == keyOf(word, pos) {
		return es.heldRegions[n-1].win
	}
	return es.takePendingWindow(word, pos)
}

// callWindowOps resolves the claimed window for a user call over args (in
// signature order). Nil when there is no window or a value has no home the
// run can read; an empty window is a non-nil empty slice.
func (es *EmitState) callWindowOps(word string, pos core.SrcPos, args []core.Value) []callWinOp {
	pw := es.claimHeldWindow(word, pos)
	if !pw.ok || !pw.known {
		return nil
	}
	out := make([]callWinOp, 0, len(pw.win))
	for i, v := range pw.win {
		op, ok := es.callWindowOp(v, pw.stable[i], args)
		if !ok {
			return nil
		}
		op.fwd = i < pw.nFwd
		out = append(out, op)
	}
	return windowStopPrefix(es, out, pw, args)
}

// windowStopPrefix appends the stack prefix the window's stack values do
// not already cover (PrefixOnly), so the run can rebuild the interpreter's
// report where a written value stops it (NUR311). A prefix value with no
// home the run can read leaves the window unmarked: rendered as recorded.
func windowStopPrefix(es *EmitState, out []callWinOp, pw pendingWindow, args []core.Value) []callWinOp {
	fill := len(out) - pw.nFwd
	if pw.nFwd == 0 || fill > len(pw.prefix) {
		return out
	}
	for i := fill; i < len(pw.prefix); i++ {
		op, ok := es.callWindowOp(pw.prefix[i], pw.prefixStable[i], args)
		if !ok {
			for j := range out {
				out[j].fwd = false
			}
			return out[:len(pw.win)]
		}
		op.prefixOnly = true
		out = append(out, op)
	}
	return out
}

// callWindowOp is one window value's home: an argument by identity, a
// definite scalar by value, an event result of the unit being recorded, or
// a frame local read whose binding had not moved when the window was
// offered (stable — a rebind between the read and the call leaves the slot
// holding the new value, where the interpreter's window holds the one
// read).
func (es *EmitState) callWindowOp(v core.Value, stable bool, args []core.Value) (callWinOp, bool) {
	if v.ID != "" {
		for i := range args {
			if args[i].ID == v.ID {
				return callWinOp{kind: WinArg, idx: i}, true
			}
		}
	}
	if windowScalar(v) || unexpandedToken(v) {
		return callWinOp{kind: WinValue, value: v}, true
	}
	if v.ID == "" {
		return callWinOp{}, false
	}
	if pr, ok := es.producedBy[v.ID]; ok {
		if len(es.units) > 1 && !es.producedInCurrentUnit(v.ID) {
			return callWinOp{}, false
		}
		return callWinOp{kind: WinStack, op: EventOperand(pr.seq, pr.idx)}, true
	}
	if slot, ok := es.units[len(es.units)-1].localByID[v.ID]; ok && stable {
		return callWinOp{kind: WinLocal, idx: slot}, true
	}
	return callWinOp{}, false
}

// windowScalar reports whether v is a definite scalar or atom — a value the
// pass knows is the run's, which the window may carry as itself.
func windowScalar(v core.Value) bool {
	if v.Carrier || v.Dynamic || !core.IsConcrete(v) || v.Parent == nil {
		return false
	}
	return v.Parent.ConformsTo(core.TScalar) || core.IsAtom(v)
}

// unexpandedToken reports whether v is a reach or template token a failed
// dispatch's window holds unexpanded — a written operand past the one the
// forward phase evaluated (`f m.a m.b` over `x:Type`). The interpreter's
// report names the token itself (`m.b (a Reach)`), which is the run's own
// source, so a window or a rematch carries it as itself (NUR329).
func unexpandedToken(v core.Value) bool {
	return (core.IsReach(v) || core.IsInterpString(v)) && !v.Carrier && !v.Dynamic
}
