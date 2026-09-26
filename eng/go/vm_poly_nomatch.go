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
	// sigError's two-probe cascade: the value-based reorder probe over the
	// written tuple first, the stack-prefix tuple second (engine.go:sigError).
	reorder := core.ReorderHintFor(pr.Word, fn, written)
	if reorder == "" {
		reorder = core.ReorderHintFor(pr.Word, fn, stackTuple)
	}
	ae := core.NoMatchDiag(r.Source, pr.Word, fn, written, spec.Pos, reorder)
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
