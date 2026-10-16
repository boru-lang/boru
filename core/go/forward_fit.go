package core

// A FORWARD FIT is what the interpreter's forward collection asks of a value
// written after a word (CollectCandidateScan): does it fill the next slot of
// the candidate signature? A value that does not STOPS the scan, and the
// rest of that candidate's slots come off the stack beneath the word — a
// different window. The check pass asks the same question of the operand's
// carrier, and a gradual carrier (a fn's declared-Any result, a member read
// of an opaque map) is admitted at a slot its type does not prove: the pass
// collected it forward where the run's value may be one the scan stops at.
// `{b:2} keys y` over `def y (f)` and f's Any result of 0 is keys over the
// {b:2} beneath it, then the 0, interpreted — [['b'] 0] — where the compiled
// call took the 0 forward and raised keys' no-match (NUR357).
//
// So a dispatch that collected such an operand forward publishes, for its
// own record, the candidate slots whose fit the pass did not prove
// (CheckState.CurFits, FitsFor): every signature the interpreter's plan tries
// up to the one the pass took, whose forward reach covers the operand's
// position and which admitted its carrier there. A compiled test of the
// run's value against them (ForwardFitsAll) tells the collection the pass
// modelled from one the interpreter's plan makes differently.

// ForwardFit is one candidate signature slot a forward-collected gradual
// operand must fit for the interpreter's forward collection to take it
// where the pass did: signature Sig's position Idx. Chosen marks the
// signature the pass matched — the slot it collected the operand into.
type ForwardFit struct {
	Sig    *Signature
	Idx    int
	Chosen bool
}

// ForwardValueFits reports whether the interpreter's forward scan takes v —
// a concrete value, not a fn it would dispatch — at sig's position k: a form
// or quoted slot captures any token; otherwise the value must match the
// slot, or the slot be Any, and a bare type node is refused at a
// concrete-payload slot (CollectCandidateScan's literal and def-binding
// arms, which agree on every non-fn value).
func ForwardValueFits(sig *Signature, k int, v Value) bool {
	if sig == nil || k < 0 || k >= sig.TotalArgs() {
		return false
	}
	if (sig.FormArgs != nil && sig.FormArgs[k]) || (sig.QuoteArgs != nil && sig.QuoteArgs[k]) {
		return true
	}
	expected := SigArgType(sig, k)
	if !SigArgMatches(sig, k, &v) && !expected.Equal(TAny) {
		return false
	}
	isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[k]
	return isTypeArg || !rejectsTypeLiteral(v, expected)
}

// ForwardFitsAll reports whether v fits every slot of fits.
func ForwardFitsAll(fits []ForwardFit, v Value) bool {
	for _, f := range fits {
		if !ForwardValueFits(f.Sig, f.Idx, v) {
			return false
		}
	}
	return true
}

// BarrierBars reports whether an overload among sigs stops its forward
// collection short of nFwd written operands and then reads the stack: its
// barrier (the unmodified word's forward limit) is below nFwd and below its
// own arity. The interpreter's plan for such an overload takes the leading
// operands up to its barrier and the rest off the stack beneath the word —
// never the window a dispatch whose leading nFwd operands were written
// after the word holds — so a flat match of that window against it is not
// the interpreter's match (NUR362: `get (h) {k:1}`, every get overload's
// barrier 1, raises signature_error however (h) comes out).
func BarrierBars(sigs []Signature, nFwd int) bool {
	for i := range sigs {
		if s := &sigs[i]; !s.Fallback && s.BarrierPos >= 0 && s.BarrierPos < nFwd && s.BarrierPos < s.TotalArgs() {
			return true
		}
	}
	return false
}
