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
	if !SigArgMatches(sig, k, v) && !expected.Equal(TAny) {
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

// forwardFitsPub is CheckState.CurFits: the published fits of one
// dispatch's operands, answered only for its own operand slice.
type forwardFitsPub struct {
	args []Value
	fits map[int][]ForwardFit
}

// FitsFor returns the published forward fits when they describe args — the
// very slice the dispatch published them for — keyed by signature position;
// nil for any other record.
func (c *CheckState) FitsFor(args []Value) map[int][]ForwardFit {
	p := c.CurFits
	if p == nil || len(args) == 0 || len(p.args) != len(args) || &p.args[0] != &args[0] {
		return nil
	}
	return p.fits
}

// forwardFits is the forward fits of the dispatch match at the pointer, by
// signature position, for each operand written after the word
// (forwardSplit) that is a carrier or a dynamic whose own type does not
// prove it fits its slot in the matched signature (fitProven): the slots,
// over fn's candidates, where the run's value may make the interpreter's
// plan differ from the pass's. A candidate's collection that stops at the
// operand takes the rest of its slots from the stack beneath the word, so
// a slot counts only where that collection could match (altViable): the
// pass's scan admitted the carrier there without proof, and the stack
// holds enough values none of which is proven to miss its slot — `z add y`
// over add's Bytes overload and one value beneath fails whatever y holds.
// An operand with no such slot is still listed, with none: its poly's
// island serves its no-match (PolyRef.Fit), whose report names the stack
// the interpreter's collection read. Nil when there is no such operand, or
// the dispatch is no word's the registry names.
func (e *Engine) forwardFits(match *MatchResult) map[int][]ForwardFit {
	nFwd := e.forwardSplit()
	if nFwd == 0 || match.Sig == nil || nFwd > len(match.Args) {
		return nil
	}
	w, err := AsWord(e.Tape.At(e.Pointer))
	if err != nil {
		return nil
	}
	// The word re-steps as a forced stack read once its forward collection
	// laid the operands out (optimisticLayout's rule): the forward phase
	// the plan ran was the unmodified word's.
	w.ForceStack = false
	fn := match.DispatchRegistry(e.Registry).Lookup(match.Name)
	if fn == nil {
		return nil
	}
	stack, unbounded := e.stackBeneath(match, nFwd)
	// The matched signature, where the dispatch matched one of fn's own; a
	// copy (a user fn's frame sig) names none, and every candidate may be
	// the one the pass took.
	matched := false
	for i := range fn.Signatures {
		matched = matched || &fn.Signatures[i] == match.Sig
	}
	var out map[int][]ForwardFit
	for k := 0; k < nFwd; k++ {
		a := match.Args[k]
		if (!a.Carrier && !a.Dynamic) || fitProven(match.Sig, k, a) {
			continue
		}
		fits := []ForwardFit{}
		for i := range fn.Signatures {
			s := &fn.Signatures[i]
			if s.Fallback || k >= s.TotalArgs() || !forwardReaches(s, w, k) || !SigArgMatches(s, k, a) || fitProven(s, k, a) ||
				!altViable(s, k, stack, unbounded) {
				continue
			}
			fits = append(fits, ForwardFit{Sig: s, Idx: k, Chosen: s == match.Sig || !matched})
		}
		if out == nil {
			out = map[int][]ForwardFit{}
		}
		out[k] = fits
	}
	return out
}

// stackBeneath is the interpreter's stack at the dispatch at the pointer,
// top first: the operands the pass took from it (match.Args[nFwd:]), then
// the resolved values beneath them up to the group's open paren. Every
// operand now sits beneath the word, the nFwd written ones laid out on top
// by the forward collection (rearrangeForForward). unbounded marks a stack
// whose count is no proof — a runtime-counted region's modelled seat
// (EmitRecorder.RegionResult) may hold any count — or whose operands are
// not where the layout puts them.
func (e *Engine) stackBeneath(match *MatchResult, nFwd int) (stack []Value, unbounded bool) {
	rec := e.Registry.analysisRecorder()
	below := e.ResolvedIndicesBefore(e.Pointer + 1)
	n := len(match.Args)
	if len(below) < n {
		return nil, true
	}
	for i := 0; i < n; i++ {
		if below[len(below)-1-i] != e.Pointer-1-i {
			return nil, true
		}
	}
	stack = append(stack, match.Args[nFwd:]...)
	for i := len(below) - n - 1; i >= 0; i-- {
		v := e.Tape.At(below[i])
		if v.ID != "" && rec.RegionResult(v.ID) {
			return nil, true
		}
		stack = append(stack, v)
	}
	return stack, false
}

// altViable reports whether sig's collection, stopped at position k, may
// match over the stack beneath the word (top first): its slots k on come
// from there, so the stack must hold that many values — unbounded, any
// count — none of which the pass proves misses its slot (a gradual value
// is admitted, as the pass admitted the operand itself).
func altViable(sig *Signature, k int, stack []Value, unbounded bool) bool {
	need := sig.TotalArgs() - k
	if !unbounded && len(stack) < need {
		return false
	}
	for j := 0; j < need && j < len(stack); j++ {
		if !SigArgMatches(sig, k+j, stack[j]) {
			return false
		}
	}
	return true
}

// forwardReaches reports whether sig's forward phase at word w may reach
// its position k (effectiveForwardLimit; an all-forward barrier reaches
// every position).
func forwardReaches(sig *Signature, w WordInfo, k int) bool {
	limit := effectiveForwardLimit(sig, w)
	return limit < 0 || k < limit
}

// fitProven reports whether operand a's own type proves the forward scan
// takes its run value at sig's position k: a slot that captures any token,
// an Any slot, or a strict carrier whose type conforms to the slot's.
func fitProven(sig *Signature, k int, a Value) bool {
	if (sig.FormArgs != nil && sig.FormArgs[k]) || (sig.QuoteArgs != nil && sig.QuoteArgs[k]) {
		return true
	}
	expected := SigArgType(sig, k)
	if expected.Equal(TAny) {
		return true
	}
	return !a.Dynamic && a.Parent != nil && !a.Parent.Equal(TAny) && a.Parent.ConformsTo(expected)
}

// publishForwardFits publishes forwardFits for the dispatch's record and
// returns the restore the caller runs once it is recorded.
func (e *Engine) publishForwardFits(match *MatchResult) func() {
	if !e.Registry.analysisRecorder().Active() {
		return func() {}
	}
	fits := e.forwardFits(match)
	if fits == nil {
		return func() {}
	}
	prev := e.Registry.Check.CurFits
	e.Registry.Check.CurFits = &forwardFitsPub{args: match.Args, fits: fits}
	return func() { e.Registry.Check.CurFits = prev }
}
