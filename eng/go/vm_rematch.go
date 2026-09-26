package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// dispatchRematch executes OpDispatchRematch (see the opcode doc): a
// statically-failed dispatch whose window held carriers re-runs the match
// over the LIVE values (window[i] = sig position i, the callPoly layout)
// against the word's live registry binding. NO MATCH — the expected case,
// this compiled an ERROR row — raises the shared rich diagnostic built over
// the concrete values, byte-identical to the interpreter's sigError at the
// same point. A MATCH means the static model was wrong (a refined runtime
// tag, a satisfied predicate); the tail was truncated at this terminal op,
// so the run defers to the interpreter. Always returns a non-nil error.
func (vc *vmContext) dispatchRematch(ds *compiler.DispatchSpec, stack []core.Value, curDebug []core.SrcPos, pc int) error {
	r := vc.r
	if len(stack) < ds.NArgs {
		return vmErrAt(curDebug, pc, "DISPATCH_REMATCH underflow at "+ds.Word)
	}
	window := make([]core.Value, ds.NArgs)
	for i := 0; i < ds.NArgs; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	var sigs []core.Signature
	fn := r.Lookup(ds.Word)
	if fn != nil {
		sigs = fn.Signatures
	}
	matched := false
	if ds.NFwd > 0 && fn != nil {
		// Operands written after the word: the interpreter's forward phase
		// fills the leading positions from them, so the flat match below
		// is not its match (NUR211) — plan the window as it does.
		m, planned := vc.rematchSplitMatches(ds, fn, window)
		matched = m || !planned
	} else if mr := core.MatchSignature(sigs, window, core.WordInfo{ArgCount: -1}); mr != nil && mr.Sig != nil && !mr.Sig.Fallback {
		matched = true
	}
	if matched {
		return vmDefer(r, curDebug, pc, "vm:rematch-matched",
			"DISPATCH_REMATCH at "+ds.Word+" matched at run time where the static model failed; the compiled runtime cannot execute it")
	}
	// The diagnostic renders over the RENDER TUPLE (window[Written[i]]) —
	// the attempted window sigError renders, proven exactly window values by
	// ID at record time. The match above ran over the FULL window (the view
	// the failed static match examined).
	written, ok := tupleAt(window, ds.Written)
	if !ok || len(written) == 0 {
		return vmErrAt(curDebug, pc, "DISPATCH_REMATCH written tuple out of range at "+ds.Word)
	}
	ae := core.RuntimeNoMatch(r, ds.Word, written)
	ae.Row, ae.Col = ds.Pos.Row, ds.Pos.Col
	return stampAt(ae, curDebug, pc, r)
}

// rematchSplitMatches plans a failed window NFwd of whose operands were
// written after the word, as the interpreter's dispatch plans it (NUR211):
// the stack run beneath the word, the word, then the written operands, over
// the region host DISPATCH_GENERIC plans with. The window lists the stack
// run top down (window[0] is the top) and then the written operands in
// order, so `3 for (mk)` is the tape `3 for [i]`: for's forward phase stops
// at the List where a count goes, the stack's 3 fills the count, and the
// body finds nothing beneath — no match, the interpreter's raise. The flat
// match read it as `for 3 [i]` and deferred. The planned signature is then
// matched strictly over the values at its positions, as the interpreter's
// dispatch matches what arrives. planned is false when the walk needs an
// evaluation this host cannot perform; the caller then defers, as it does
// on a match.
func (vc *vmContext) rematchSplitMatches(ds *compiler.DispatchSpec, fn *core.FnDefInfo, window []core.Value) (matched, planned bool) {
	nStack := ds.NArgs - ds.NFwd
	toks := make([]core.Value, 0, ds.NArgs+1)
	for i := nStack - 1; i >= 0; i-- {
		toks = append(toks, window[i])
	}
	toks = append(toks, core.NewWord(ds.Word))
	toks = append(toks, window[nStack:]...)
	h := newRegionHostOver(vc.r, toks)
	w := core.WordInfo{Name: ds.Word, ArgCount: -1}
	if err := h.Collected(core.CollectForward(h, fn, w, nStack+1)); err != nil {
		return false, false
	}
	sig, positions, _ := core.PlanMatch(h, h.win, vc.r, fn, w, toks[:nStack], nStack, false, false, false)
	if sig == nil || sig.Fallback {
		return false, true
	}
	// The plan claims positions; the interpreter's dispatch then matches the
	// values that arrive there STRICTLY, and only that is a match. The plan
	// takes a written record by its base, so `use (mk)` over a refined
	// record another refinement's param refuses planned a match that the
	// interpreter's dispatch raises on (record.tsv L155).
	args := make([]core.Value, len(positions))
	for i, at := range positions {
		args[i] = h.win.At(at)
	}
	mr := core.MatchSignature([]core.Signature{*sig}, args, core.WordInfo{ArgCount: -1})
	return mr != nil, true
}
