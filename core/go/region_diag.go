package core

import "fmt"

// The diagnostics a failed dispatch raises, built from what a WINDOW holds
// rather than from the engine's tape state — the seam the routed dispatch
// (eng's OpDispatchGeneric) needs. Its window is laid out as the tape at
// the word (the frame's resolved values, the word, the forward tokens), so
// the same derivation over it yields the interpreter's own error, byte for
// byte, where a defer would hand the program to the interpreter — and
// where an effect already performed fences that fallback, an internal
// error the user never wrote for. Engine.sigError,
// Engine.strandedForwardError and Engine.undefinedWordError are seats over
// these (the sixty-sixth increment). The interpreter's TAPE-ONLY layers
// stay on the engine: a void argument group (voidGroups), the fn-shape
// typed-binding hint, the pending-`def` hint — and each is a layer a
// drivable span never reaches (no group in the span; no pending outer
// forward, which the strict barrier forbids around a routed dispatch).

// ReorderCandidates collects up to 4 plain values from the top of the
// stack (walking down, stopping at engine markers / words) — the tuple a
// failed STACK dispatch saw, in the assignment order matchSignature uses
// (top-first: sig[i] ↔ vals[i]).
func ReorderCandidates(stack []Value) []Value {
	var vals []Value
	for i := len(stack) - 1; i >= 0 && len(vals) < 4; i-- {
		v := stack[i]
		if IsOpenParen(v) || IsForward(v) || IsWord(v) || IsEnd(v) ||
			v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) {
			break
		}
		vals = append(vals, v)
	}
	return vals
}

// ReorderForwardCandidates collects up to 4 UNCLAIMED forward value
// tokens after the word — concrete literals only; words, parens, and
// markers stop the scan. The result is in SOURCE order, which is the
// assignment order the forward plan would have used (sig[i] ↔
// token[i]) — exactly the failing-tuple view ReorderHintFor wants.
func ReorderForwardCandidates(tape *Tape, pointer int) []Value {
	var written []Value
	for i := pointer + 1; i < tape.Len() && len(written) < 4; i++ {
		v := tape.At(i)
		// An engine marker ends the written tuple exactly as it ends the
		// stack one below: a fn frame's tail markers (the DefCleanup `__dc`,
		// the pop-args `__pa`) sit right after the body's last token, and a
		// no-match there used to list `__dc (a __DC)` as the argument the
		// caller supplied — a marker no one wrote, in a user-facing note.
		if !IsConcrete(v) || IsWord(v) || IsParenExpr(v) || IsForward(v) ||
			IsOpenParen(v) || IsEnd(v) || isEngineMarker(v) {
			break
		}
		written = append(written, v)
	}
	return written
}

// NoMatchOverWindow is sigError's derivation over a window: the failing
// tuple in assignment order — the unclaimed forward tokens after the word
// (source order) when there are any, else the stack prefix top-first — and
// the reorder probe over both views, then the shared no-match builder.
// Everything sigError adds beyond this is a tape-only layer (see the file
// doc).
func NoMatchOverWindow(src string, win *Tape, pointer int, name string, fn *FnDefInfo, pos SrcPos) *BoruError {
	written := ReorderForwardCandidates(win, pointer)
	if len(written) == 0 {
		written = ReorderCandidates(win.Prefix(pointer))
	}
	reorder := ReorderHintFor(name, fn, written)
	if reorder == "" {
		reorder = ReorderHintFor(name, fn, ReorderCandidates(win.Prefix(pointer)))
	}
	return NoMatchDiag(src, name, fn, written, pos, reorder)
}

// StrandedForwardDiag is the strict-barrier signature_error: funcName is
// still waiting for `missing` argument(s) when `boundary` begins its own
// dispatch (design/STRICT-FORWARD-BARRIER.0.md). barrierReceiver adds the
// sequential-spelling note for a boundary word that reads a slot from the
// enclosing stack, which a paren group would seal off (NUR049).
func StrandedForwardDiag(src, funcName string, missing int, boundary string, barrierReceiver bool, pos SrcPos) *BoruError {
	detail := fmt.Sprintf(
		"%s is still waiting for %d argument(s) when `%s` begins its own dispatch — "+
			"a function word is a barrier and never feeds forward collection (strict rule); "+
			"group the call in parens so its RESULT becomes the argument: %s (%s …)",
		funcName, missing, boundary, funcName, boundary)
	if barrierReceiver {
		detail += fmt.Sprintf(
			"; note `%s` reads its receiver from the enclosing stack, which a paren "+
				"group seals off — if the grouped form cannot match, run it first and "+
				"bind its result in sequence instead: %s … %s",
			boundary, boundary, funcName)
	}
	return makeBoruErrorAt("signature_error", detail, funcName, src, "", pos)
}

// BarrierReceiverWord reports whether any registered signature of name
// reads a slot from the enclosing stack (BarrierPos < TotalArgs). For such
// a word the "group the call in parens" fix can starve the barrier slot —
// the paren seals the enclosing stack (NUR049) — so suggestions offer the
// sequential spelling too.
func BarrierReceiverWord(r *Registry, name string) bool {
	fd := r.Lookup(name)
	if fd == nil {
		return false
	}
	for i := range fd.Signatures {
		s := &fd.Signatures[i]
		if s.BarrierPos >= 0 && s.BarrierPos < s.TotalArgs() {
			return true
		}
	}
	return false
}

// DidYouMeanOver builds the near-miss suggestion(s) for an unbound name over
// r: the did-you-mean line, plus the describe pointer when the nearest
// miss is a builtin word (so the fix and its documentation arrive
// together). Failure-path only — the candidate enumeration is a walk of
// everything nameable.
func DidYouMeanOver(r *Registry, name string) []DiagSuggestion {
	matches := SuggestNames(name, r.SuggestionCandidates())
	if len(matches) == 0 {
		return nil
	}
	out := []DiagSuggestion{{Message: didYouMeanMessage(matches)}}
	if r.IsBuiltinWord(matches[0]) {
		out = append(out, DiagSuggestion{Message: describeSuggestion(matches[0])})
	}
	return out
}

// UndefinedWordDiag is the runtime undefined_word diagnostic over a
// registry: the stable grep-friendly Detail (UndefinedWordDetail) and the
// did-you-mean near-miss. The engine's undefinedWordError puts its two
// tape-only hints (a pending `def`, a void group) before these.
func UndefinedWordDiag(r *Registry, src, name string, pos SrcPos) *BoruError {
	ae := &BoruError{
		Code:       "undefined_word",
		Detail:     UndefinedWordDetail(name),
		Src:        name,
		Row:        pos.Row,
		Col:        pos.Col,
		FullSource: src,
	}
	ae.Suggestions = append(ae.Suggestions, DidYouMeanOver(r, name)...)
	return ae
}
