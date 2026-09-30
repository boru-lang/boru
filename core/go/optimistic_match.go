package core

// An OPTIMISTIC match is one a compile pass made over a carrier operand
// whose type does not conform to its slot's — `each (mk) [dup]` over a fn's
// declared-Any result, which the run may find to be 5. The pass models the
// dispatch as if it matched; the run re-matches the live value, and when
// nothing matches the interpreter raises the word's signature_error before
// it evaluates a single argument. Two records need to know that, and both
// need the dispatch's operand layout, which a matched dispatch no longer
// shows on its tape: rearrangeForForward has laid every operand out beneath
// the word, the ones written after it on top in signature order.
//
//   - optimisticOuter publishes the OUTERMOST such dispatch around its
//     argument evaluation, so a trap met there becomes that word's runtime
//     rematch (CheckState.OptimisticOuter, NUR264).
//   - optimisticLayout publishes the dispatch's exact layout for its own
//     record (CheckState.CurLayout, NUR242's channel), so a committed call
//     can raise the interpreter's report when the live value matches no
//     overload (NUR263). A dispatch over a GRADUAL operand publishes it too
//     (gradualMatch), even where the gradual bound conforms: the record is a
//     poly re-match, and the live value may still match nothing — a read
//     after a computed body that rebound the name to a List (`x add 1`).

// gradualMatch reports whether some operand of match is gradual (Dynamic):
// its bound is the pass's best guess, so the run re-matches the live value.
func gradualMatch(match *MatchResult) bool {
	for _, a := range match.Args {
		if a.Dynamic {
			return true
		}
	}
	return false
}

// optimisticMatch reports whether some operand of match is a carrier (or a
// dynamic) whose type does not conform to its slot's.
func optimisticMatch(match *MatchResult) bool {
	for i, a := range match.Args {
		if slot := SigArgType(match.Sig, i); slot != nil && (a.Carrier || a.Dynamic) && a.Parent != nil && !a.Parent.ConformsTo(slot) {
			return true
		}
	}
	return false
}

// forwardSplit is how many operands of the dispatch at the pointer were
// written after its word — the leading signature positions — as the last
// rearrangeForForward recorded them, when that record is this word's (its
// pointer and position); none otherwise, which is also what a dispatch that
// collected nothing forward has.
func (e *Engine) forwardSplit() int {
	if e.fwdSplitAt == e.Pointer && e.Pointer < e.Tape.Len() && e.Tape.At(e.Pointer).Pos() == e.fwdSplitPos {
		return e.fwdSplitN
	}
	return 0
}

// ForwardSplit is forwardSplit for the compile seams outside core: how many
// operands of the dispatch at the pointer its own forward collection took
// from after the word. A word re-stepped by that collection's completion
// reads every operand beneath it, but the leading ones were written after
// it, so a stack-only model of the dispatch is not the interpreter's
// (NUR362).
func (e *Engine) ForwardSplit() int { return e.forwardSplit() }

// regionMatch reports whether some operand of match is the modelled seat of
// a runtime-counted region (EmitRecorder.RegionResult): its type is the
// pass's approximation of the region — a loop's `[:T]` — so the match over
// it is no proof the run's values match (NUR340).
func (e *Engine) regionMatch(match *MatchResult) bool {
	rec := e.Registry.analysisRecorder()
	for _, a := range match.Args {
		if a.ID != "" && rec.RegionResult(a.ID) {
			return true
		}
	}
	return false
}

// optimisticOuter is the OuterMatch execMatch publishes (NUR264) when a
// compile pass matched this dispatch OPTIMISTICALLY — over an operand whose
// type does not conform to its slot's, or over a region's modelled seat
// (regionMatch, NUR340) — and no outer one is already published (the
// outermost is the first the run re-matches). indices are the match's tape
// positions, in signature order. Nil otherwise, and whenever a position is
// unknown.
func (e *Engine) optimisticOuter(match *MatchResult, indices []int) *OuterMatch {
	r := e.Registry
	if !r.analysisActive() || !r.Check.Compiling || r.Check.OptimisticOuter != nil || match.Sig == nil ||
		match.Name == "" || len(indices) != len(match.Args) || len(match.Args) == 0 ||
		(!optimisticMatch(match) && !e.regionMatch(match)) {
		return nil
	}
	n, nFwd := len(match.Args), e.forwardSplit()
	if nFwd > n {
		return nil
	}
	// The window as the rematch reads it: the stack run beneath the word
	// top down (positions nFwd..n-1), then the written operands in written
	// order (positions 0..nFwd-1). The render tuple lists the window in
	// source order: the stack run bottom up, then the written operands.
	nStack := n - nFwd
	vals := append(append(make([]Value, 0, n), match.Args[nFwd:]...), match.Args[:nFwd]...)
	written := make([]int, 0, n)
	for i := nStack - 1; i >= 0; i-- {
		written = append(written, i)
	}
	for i := nStack; i < n; i++ {
		written = append(written, i)
	}
	pos := SrcPos{}
	if e.Pointer < e.Tape.Len() {
		pos = e.Tape.At(e.Pointer).Pos()
	}
	return &OuterMatch{Word: match.Name, Vals: vals, NFwd: nFwd, Written: written, Pos: pos}
}

// optimisticLayout is the layout (DispatchLayout) of an optimistic dispatch
// at the pointer, or nil. The interpreter's tape at the run's failed
// dispatch is [Beneath, stack operands, word, written operands, After];
// here, after rearrangeForForward, signature position i sits i+1 beneath
// the word, and forwardSplit says how many were written. The rules are
// exactLayout's: the operands are the tape's own values, what the plan
// could reach around them rides with them up to a statement or group
// boundary (or the tape's end) on both sides (layoutSurround), the word
// carries no modifier the source wrote — a forced stack read is the
// forward collection's own when it recorded a split — and neither
// tape-only report layer applies.
func (e *Engine) optimisticLayout(match *MatchResult, indices []int) *DispatchLayout {
	n := len(match.Args)
	if n == 0 || len(indices) != n || match.Sig == nil || !e.Registry.analysisRecorder().Active() ||
		!(optimisticMatch(match) || gradualMatch(match)) || e.Pointer >= e.Tape.Len() {
		return nil
	}
	w, err := AsWord(e.Tape.At(e.Pointer))
	nFwd := e.forwardSplit()
	if err != nil || w.ArgCount != -1 || (w.ForceStack && nFwd == 0) || w.ForceForward || w.ForceVal || w.ForceUsurp || nFwd > n {
		return nil
	}
	if e.voidArgErrorFor(w.Name, e.Tape.At(e.Pointer).Pos()) != nil || e.IsFnShapeTypedBindingContext() {
		return nil
	}
	for i := 0; i < n; i++ {
		at := e.Pointer - 1 - i
		if indices[i] != at || at < 0 || match.Args[i].ID == "" || e.Tape.At(at).ID != match.Args[i].ID {
			return nil
		}
	}
	beneath, after, live, ok := e.layoutSurround(e.Pointer-n-1, e.Pointer+1, nFwd, true)
	if !ok {
		return nil
	}
	return &DispatchLayout{args: match.Args, NFwd: nFwd, Beneath: beneath, After: after, Live: live}
}

// publishOptimisticLayout publishes optimisticLayout for the dispatch's
// record and returns the restore the caller runs once it is recorded.
func (e *Engine) publishOptimisticLayout(match *MatchResult, indices []int) func() {
	l := e.optimisticLayout(match, indices)
	if l == nil {
		return func() {}
	}
	prev := e.Registry.Check.CurLayout
	e.Registry.Check.CurLayout = l
	return func() { e.Registry.Check.CurLayout = prev }
}
