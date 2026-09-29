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
// so the run defers to the interpreter — or, where the trap's statement has
// an island (DispatchSpec.Restart, NUR336), the interpreter runs the
// statement and the program after it, and the run continues at its end.
func (vc *vmContext) dispatchRematch(reg *core.Registry, ds *compiler.DispatchSpec, frameBase int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	matched, err := vc.rematchOutcome(ds, stack, curDebug, pc)
	if matched && ds.Restart != nil {
		return vc.stopRestart(reg, ds.Restart, nil, frameBase, stack, curDebug, pc)
	}
	return nil, nil, err
}

// rematchOutcome is the rematch's terminal error over the live window: the
// interpreter's raise, a trap recorded under the word's optimistic match, or
// the defer where the run matches the word (matched).
func (vc *vmContext) rematchOutcome(ds *compiler.DispatchSpec, stack []core.Value, curDebug []core.SrcPos, pc int) (bool, error) {
	r := vc.r
	if len(stack) < ds.NArgs {
		return false, vmErrAt(curDebug, pc, "DISPATCH_REMATCH underflow at "+ds.Word)
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
	matched, planned := false, true
	if ds.NFwd > 0 && fn != nil {
		// Operands written after the word: the interpreter's forward phase
		// fills the leading positions from them, so the flat match below
		// is not its match (NUR211) — plan the window as it does.
		var m bool
		m, planned = vc.rematchSplitMatches(ds, fn, window)
		matched = m || !planned
	} else if mr := core.MatchSignature(sigs, window, core.WordInfo{ArgCount: -1}); mr != nil && mr.Sig != nil && !mr.Sig.Fallback {
		matched = true
	}
	if matched {
		if ds.OnMatch != nil && planned {
			// A trap recorded under the word's optimistic match (NUR264):
			// the word matches, so the run meets the trap's own error, as
			// the interpreter's argument evaluation does. A split this host
			// could not plan is no proof of a match, and defers.
			ae := core.MakeBoruError(ds.OnMatch.Code, ds.OnMatch.Detail, ds.OnMatch.Word, r.Source, ds.OnMatch.Hint)
			ae.Spans, ae.Notes, ae.Suggestions = ds.OnMatch.Spans, ds.OnMatch.Notes, ds.OnMatch.Suggestions
			ae.Row, ae.Col = ds.OnMatchPos.Row, ds.OnMatchPos.Col
			return false, stampAt(ae, curDebug, pc, r)
		}
		return true, vmDefer(r, curDebug, pc, "vm:rematch-matched",
			"DISPATCH_REMATCH at "+ds.Word+" matched at run time where the static model failed; the compiled runtime cannot execute it")
	}
	// The diagnostic renders over the RENDER TUPLE (window[Written[i]]) —
	// the attempted window sigError renders, proven exactly window values by
	// ID at record time. The match above ran over the FULL window (the view
	// the failed static match examined).
	written, ok := tupleAt(window, ds.Written)
	if !ok || len(written) == 0 {
		return false, vmErrAt(curDebug, pc, "DISPATCH_REMATCH written tuple out of range at "+ds.Word)
	}
	written = rematchStoppedTuple(ds, fn, window, written)
	ae := core.RuntimeNoMatch(r, ds.Word, written)
	ae.Row, ae.Col = ds.Pos.Row, ds.Pos.Col
	return false, stampAt(ae, curDebug, pc, r)
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
	h, sig, positions, ok := planSplit(vc.r, ds.Word, fn, window[:nStack], window[nStack:])
	if !ok {
		return false, false
	}
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

// planSplit lays a dispatch's operands out as the interpreter's tape at the
// word — the stack run bottom-up (stackTop lists it top first), the word,
// the written operands in order — over the region host DISPATCH_GENERIC
// plans with, and runs the interpreter's plan there: the word's forward
// collection, then PlanMatch. ok is false when the walk needs an evaluation
// this host cannot perform; the callers then keep their defer. The pointer
// of the laid-out tape is len(stackTop).
func planSplit(reg *core.Registry, word string, fn *core.FnDefInfo, stackTop, written []core.Value) (*regionHost, *core.Signature, []int, bool) {
	return planSplitOver(reg, word, fn, nil, stackTop, written, nil)
}

// planSplitOver is planSplit over the whole tape the interpreter's plan
// could reach (NUR283): the constants beneath the stack run (tape order)
// and the source tokens after the written operands. The pointer of the
// laid-out tape is len(beneath)+len(stackTop).
func planSplitOver(reg *core.Registry, word string, fn *core.FnDefInfo, beneath, stackTop, written, after []core.Value) (*regionHost, *core.Signature, []int, bool) {
	nStack := len(stackTop)
	toks := make([]core.Value, 0, len(beneath)+nStack+1+len(written)+len(after))
	toks = append(toks, beneath...)
	for i := nStack - 1; i >= 0; i-- {
		toks = append(toks, stackTop[i])
	}
	at := len(toks)
	toks = append(toks, core.NewWord(word))
	toks = append(toks, written...)
	toks = append(toks, after...)
	h := newRegionHostOver(reg, toks)
	w := core.WordInfo{Name: word, ArgCount: -1}
	if err := h.Collected(core.CollectForward(h, fn, w, at+1)); err != nil {
		return nil, nil, nil, false
	}
	sig, positions, _ := core.PlanMatch(h, h.win, reg, fn, w, toks[:at], at, false, false, false)
	return h, sig, positions, true
}

// rematchStoppedTuple is the tuple the interpreter's no-match report renders
// over the live window. The render tuple leads with the operands written
// after the word, which the check pass held as carriers and rendered; the
// interpreter's walk over them stops at the first that is no concrete value
// at run time — a type literal, None — so its report takes the written
// operands before it, filled from the stack prefix beneath the word
// (core.AttemptedTuple): `mini m.e 'ab'` over `{e: Function}` supplied none
// where the compiled report listed both (NUR311). Without a recorded prefix
// the recorded tuple stands.
func rematchStoppedTuple(ds *compiler.DispatchSpec, fn *core.FnDefInfo, window, written []core.Value) []core.Value {
	nStack := ds.NArgs - ds.NFwd
	for i, at := range ds.Written {
		if at < nStack || core.IsConcrete(written[i]) {
			continue
		}
		prefix, ok := tupleAt(window, ds.Prefix)
		if !ds.PrefixKnown || !ok {
			return written
		}
		return core.AttemptedTuple(fn, append([]core.Value(nil), written[:i]...), prefix)
	}
	return written
}
