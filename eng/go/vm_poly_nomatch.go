package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// polyNoMatchRaise executes a poly's FAITHFUL no-match raise (plan 3c): when
// the record carried a PolyNoMatchSpec — the check pass proved, at the failed-
// dispatch tape state it recovered from, that the interpreter's sigError
// diagnostic is rebuildable from the runtime operand window — the VM raises
// the byte-identical signature_error here instead of deferring the whole run
// to the interpreter. Rebuild: the spec's window indices reproduce sigError's
// two tape tuples (the WRITTEN tuple its notes render, and the SECONDARY
// stack-prefix tuple its reorder probe falls back to); the diagnostic itself
// is then the same shared builder over the same inputs (noMatchDiag +
// reorderHintFor — both pure functions of the word, its live table, and the
// tuples). Returns nil — keeping the caller's sound defer — when no spec was
// recorded or a runtime drift guard trips: a signature table whose length
// changed since the record (the record-time arity screen no longer holds), a
// NARROWER-arity overload now present (its collection could match where this
// raise claims nothing can), or an index outside the window.
func (vc *vmContext) polyNoMatchRaise(r *core.Registry, pr *compiler.PolyRef, fn *core.FnDefInfo, window []core.Value, curDebug []core.SrcPos, pc int) error {
	spec := pr.NoMatch
	if spec == nil || fn == nil || len(fn.Signatures) != spec.NSigs {
		return nil
	}
	// A named fn VALUE's no-match (the check pass's fn-value recovery): the
	// interpreter raises uncalled_function at the recorded position, the
	// raise execFnDefLiteral builds — not sigError's signature_error. Its
	// narrower-arity overloads were proved shadowed at the record
	// (core.windowArityFirstMatch, pinned by NSigs), so the word screen
	// below does not apply to it.
	if spec.Uncalled {
		return stampAt(uncalledFunctionErrorAt(r, pr.Word, spec.Pos), curDebug, pc, r)
	}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if !s.Fallback && s.TotalArgs() < pr.Arity {
			return nil
		}
	}
	written, ok := tupleAt(window, spec.Written)
	if !ok {
		return nil
	}
	stackTuple, ok := tupleAt(window, spec.StackTuple)
	if !ok {
		return nil
	}
	// A written operand that is no concrete value at run time — a type
	// literal, None — ends the interpreter's walk over the written operands,
	// where the check pass's carrier did not: its report takes the ones
	// before it, filled from the stack prefix (NUR311).
	for i, v := range written[:min(spec.NFwd, len(written))] {
		if !core.IsConcrete(v) {
			written = core.AttemptedTuple(fn, append([]core.Value(nil), written[:i]...), stackTuple)
			break
		}
	}
	// sigError's two-probe cascade: the value-based reorder probe over the
	// written tuple first, the stack-prefix tuple second (engine.go:sigError).
	reorder := core.ReorderHintFor(pr.Word, fn, written)
	if reorder == "" {
		reorder = core.ReorderHintFor(pr.Word, fn, stackTuple)
	}
	// The recorded anchor is the word's own position; a word the source
	// never wrote — the `dot` a lens expands to under `apply` (`5 $.name
	// apply`) — has none, and neither has the debug table at this pc, so
	// the raise rendered "source position unknown" where the interpreter
	// underlined the first written value (its own fallback for a
	// positionless site: the first candidate that carries one). Anchor at
	// that value too (NUR171).
	pos := spec.Pos
	if pos.Row == 0 {
		for _, w := range written {
			if w.Pos().Row > 0 {
				pos = w.Pos()
				break
			}
		}
	}
	ae := core.NoMatchDiag(r.Source, pr.Word, fn, written, pos, reorder)
	return stampAt(ae, curDebug, pc, r)
}

// tupleAt rebuilds a recorded tape tuple from operand-window indices.
func tupleAt(window []core.Value, idx []int) ([]core.Value, bool) {
	vals := make([]core.Value, len(idx))
	for i, j := range idx {
		if j < 0 || j >= len(window) {
			return nil, false
		}
		vals[i] = window[j]
	}
	return vals, true
}

// bestEffortNoMatch builds the DeferAlt raise a no-match defer carries for
// the fence-blocked arm (vmDeferAlt): the rich no-signature diagnostic over
// the live window. Returns nil — no alt, the internal error stands — unless
// the live table PROVES the interpreter's re-run would also fail this
// dispatch: every non-fallback overload takes exactly the window's arity
// (n >= 1), so no other-arity collection and no 0-arg courtesy dispatch can
// succeed where this raise claims failure. Best-effort: the rendered tuple
// is the full window, which may be wider than the tape-derived tuple the
// interpreter would show — the open-fallback arm keeps byte-identity by
// re-running instead.
func bestEffortNoMatch(r *core.Registry, fn *core.FnDefInfo, word string, window []core.Value, curDebug []core.SrcPos, pc int) *core.BoruError {
	if fn == nil || len(window) == 0 {
		return nil
	}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if !s.Fallback && s.TotalArgs() != len(window) {
			return nil
		}
	}
	ae := core.NoMatchDiag(r.Source, word, fn, window, core.SrcPos{}, core.ReorderHintFor(word, fn, window))
	if stamped, ok := stampAt(ae, curDebug, pc, r).(*core.BoruError); ok {
		return stamped
	}
	return ae //covergate:allow stampAt returns the same *BoruError it was given (§compiler)
}

// polySplitRaise is the no-match arm for a poly whose record carried its
// exact operand layout (PolyRef.Split, NUR242). The interpreter's tape at
// the failed dispatch is the operands laid out around the word — the stack
// ones beneath it, the written ones after it — and nothing else it can
// reach, so its own plan over that tape decides, whatever arities the
// word's overloads take: a plan that finds a signature returns nil (the
// interpreter dispatches it; the caller keeps its path), and one that finds
// none raises the interpreter's signature_error over the same tape, byte
// for byte (NoMatchOverWindow). window is the poly's operands in signature
// order: the written ones first, then the stack ones, top first. A walk
// this host cannot drive returns nil too.
func polySplitRaise(r *core.Registry, pr *compiler.PolyRef, fn *core.FnDefInfo, window, stack, locals []core.Value, curDebug []core.SrcPos, pc int) error {
	sp := pr.Split
	if sp == nil {
		return nil
	}
	beneath, ok := splitBeneath(sp, stack, locals)
	if !ok {
		return nil
	}
	return splitNoMatch(r, pr.Word, fn, window, sp.NFwd, beneath, sp.After, sp.Words, curDebug, pc)
}

// fitRestart takes a call's forward-fit island (PolyRef.Fit,
// CompiledFn.CallFits, NUR357): the statement runs again on the interpreter
// from its first token over the frame of the running unit (frames' top, the
// program's at the root), its operands still on stack, and the run
// continues at the island's RetPC.
func (vc *vmContext) fitRestart(reg *core.Registry, fit *compiler.PolyFit, frames []vmFrame, stack, locals []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	fb := 0
	if len(frames) > 0 {
		fb = frames[len(frames)-1].stackBase
	}
	vc.restartLocals = locals
	ns, ent, err := vc.stopRestart(reg, fit.Restart, nil, fb, stack, curDebug, pc)
	vc.restartLocals = nil
	return ns, ent, err
}

// callFitAt is the forward-fit island of the CALL_USER at pc in the running
// unit's code (CompiledFn.CallFits, the main code's Program.CallFits for a
// negative unit), or nil.
func callFitAt(p *compiler.Program, unit, pc int) *compiler.PolyFit {
	if p == nil {
		return nil
	}
	table := p.CallFits
	if unit >= 0 {
		if unit >= len(p.Fns) {
			return nil
		}
		table = p.Fns[unit].CallFits
	}
	return table[pc]
}

// splitBeneath is the split's Beneath with each live entry (PolySplit.Live,
// NUR351) read where the compiled code keeps it: the frame's local, or the
// operand stack entry that deep below its top. ok is false for a place out
// of range, and the arm then reads no layout.
func splitBeneath(sp *compiler.PolySplit, stack, locals []core.Value) ([]core.Value, bool) {
	if len(sp.Live) == 0 {
		return sp.Beneath, true
	}
	out := append([]core.Value(nil), sp.Beneath...)
	for _, l := range sp.Live {
		if l.At < 0 || l.At >= len(out) {
			return nil, false
		}
		switch {
		case l.Local && l.Idx >= 0 && l.Idx < len(locals):
			out[l.At] = locals[l.Idx]
		case !l.Local && l.Idx >= 0 && l.Idx < len(stack):
			out[l.At] = stack[len(stack)-1-l.Idx]
		default:
			return nil, false
		}
	}
	return out, true
}

// splitNoMatch lays window (signature order, the nFwd written operands
// first) out as the interpreter's tape at word — with the constants beneath
// and the source tokens after that the record read there (NUR283) — and
// raises its signature_error over that tape when its plan finds no
// signature; nil when the plan finds one over the operands or cannot be
// driven, and for a layout out of range. A plan that takes a value beneath
// or a token after is a dispatch the interpreter makes over a window the
// program never assembled: a designed defer. words (PolySplit.Words) puts
// the source word back at each written operand read from one, for the
// report alone: the interpreter's plan read the binding and left the word
// on its tape.
func splitNoMatch(r *core.Registry, word string, fn *core.FnDefInfo, window []core.Value, nFwd int, beneath, after []core.Value, words map[int]core.Value, curDebug []core.SrcPos, pc int) error {
	if fn == nil || nFwd < 0 || nFwd > len(window) {
		return nil
	}
	h, sig, positions, ok := planSplitOver(r, word, fn, beneath, window[nFwd:], window[:nFwd], after)
	if !ok {
		return nil
	}
	if sig != nil && !sig.Fallback {
		if len(beneath)+len(after) > 0 && planReaches(positions, len(beneath), len(beneath)+len(window)) {
			return vmDefer(r, curDebug, pc, "vm:split-plan-reaches",
				"`"+word+"`'s dispatch takes a value the compiled call's operands do not hold (NUR283)")
		}
		return nil
	}
	var pos core.SrcPos
	if pc >= 0 && pc < len(curDebug) {
		pos = curDebug[pc]
	}
	at := len(beneath) + len(window) - nFwd
	return stampAt(core.NoMatchOverWindow(r.Source, splitReportTape(h.win, at, words), at, word, fn, pos), curDebug, pc, r)
}

// splitReportTape is win, the plan's tape with the dispatching word at at,
// with each written operand read from a source word (words, by written
// index) put back as that word; win itself when none is.
func splitReportTape(win *core.Tape, at int, words map[int]core.Value) *core.Tape {
	if len(words) == 0 {
		return win
	}
	toks := make([]core.Value, win.Len())
	for i := range toks {
		toks[i] = win.At(i)
	}
	for i, w := range words {
		if k := at + 1 + i; i >= 0 && k < len(toks) {
			toks[k] = w
		}
	}
	return core.NewTape(toks, core.StackHeadroom)
}

// planReaches reports whether a plan's tape positions reach outside the
// operands, which the laid-out tape holds at lo..hi (the word between them).
func planReaches(positions []int, lo, hi int) bool {
	for _, at := range positions {
		if at < lo || at > hi {
			return true
		}
	}
	return false
}

// nativeSplitRaise is the no-match arm of a committed CALL_NATIVE the pass
// matched optimistically (SigRef.Split, NUR263), run when its handler
// refused. The program passed the body as a compiled closure; the
// interpreter's tape holds the token list there, so the window takes it
// back before the plan. A plan that finds a signature returns nil, and the
// handler's own error stands — the interpreter dispatched and ran it too.
func nativeSplitRaise(r *core.Registry, word string, sp *compiler.NativeSplit, args []core.Value, curDebug []core.SrcPos, pc int) error {
	if sp.BodyAt < 0 || sp.BodyAt >= len(args) {
		return nil
	}
	window := append([]core.Value(nil), args...)
	window[sp.BodyAt] = sp.Body
	return splitNoMatch(r, word, r.Lookup(word), window, sp.NFwd, sp.Beneath, sp.After, nil, curDebug, pc)
}
