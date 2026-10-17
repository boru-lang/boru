package core

import "sync/atomic"

// Dispatch plans: PlanMatch, remembered.
//
// PlanMatch answers one question per dispatch — which signature, how many of
// its arguments the forward tokens fill, and where the first speculative
// slot sits — by scanning every candidate signature over the forward window
// and the stack. For a word whose every signature matches its arguments by
// tag alone (planSetFor's admission: no pattern, no type-literal slot, every
// argument type a builtin whose membership is its tag), that answer is a pure
// function of what the scans read:
//
//   - the call's modifiers (/s, /f, the arity digit), whether a forward is
//     pending, and whether the next token is a word (the /q preference);
//   - the top of the resolved stack, each value by its tag (the stack phase
//     reads no deeper than the word's widest signature);
//   - the forward window up to the widest barrier, each token by what
//     CollectCandidateScan reads of it: a literal by its tag; a word bound
//     to a value by the value's tag; a word bound to a function by its
//     fn-ness and its `/v`; an unbound word by the literal it reads as
//     (true/false, a builtin type name, an atom); a boundary as a stop.
//
// A type-literal slot (TypeArgs — the bound of `Integer lte 10`) reads two
// facts of a concrete value besides its tag: whether it is a structural type
// body and whether it is a dependent-scalar constraint (sigTypeMatchesAsType),
// so every value a key describes carries both.
//
// planKey is that description. A key is built only when every value it
// describes is decided by its tag (TagKeyable — not a bare type node, a
// carrier, a dynamic or ascribed value), so two dispatches with one key run
// the identical scans and reach the identical answer. A token whose scan
// treatment reads more — a sugar marker (which the scan expands in place),
// a reach-collapsed fn value (whose claim reads the tokens after it) — makes
// the dispatch plan afresh. Analysis passes never plan from memory: their
// matcher reads carriers and records what it chose.
//
// The plans live on the dispatch aggregate the word was looked up as, so a
// redefinition (a new aggregate) starts over and a loop body's dispatch skips
// the per-candidate scans from its second pass on.

const (
	// planArgsMax bounds a plannable word's widest signature, and with it
	// the stack a key describes and the window (no barrier exceeds a
	// signature's arity).
	planArgsMax = 4
	// planSetMax bounds the plans one aggregate keeps: a word dispatched in
	// more contexts than this keeps its first ones and plans the rest afresh,
	// so a word that keeps missing never pays for a new set per dispatch.
	planSetMax = 8
)

// A window token's kind, by what the per-candidate scan reads of it.
const (
	pdEnd      uint8 = iota + 1 // the window ends here
	pdStop                      // a boundary or a raw open paren: every scan stops
	pdValue                     // a literal, by its tag
	pdBound                     // a word bound to a value, by the value's tag
	pdFn                        // a word bound to a function (forceVal: written `/v`)
	pdBool                      // an unbound true / false
	pdTypeName                  // an unbound builtin type name, by its node
	pdAtom                      // any other unbound word
)

// Bits of planVal.facts: what a type-literal slot reads of a concrete value.
const (
	pfTypeBody uint8 = 1 << iota
	pfDepScalar
)

// planVal describes a concrete value: its tag, and the facts a type-literal
// slot reads.
type planVal struct {
	tag   *Type
	facts uint8
}

// planValOf describes v, or reports a value whose match reads more than a
// planVal holds (TagKeyable).
func planValOf(v *Value) (planVal, bool) {
	if !TagKeyable(*v) {
		return planVal{}, false
	}
	pv := planVal{tag: v.Parent()}
	if IsTypeBody(v) {
		pv.facts |= pfTypeBody
	}
	if v.IsDepScalar() {
		pv.facts |= pfDepScalar
	}
	return pv, true
}

// Bits of planKey.mods.
const (
	planForceStack uint8 = 1 << iota
	planForceForward
	planInsideForward
	planNextIsWord
)

// planDesc is one forward token of a plan key: its kind, its `/v` for a fn
// binding, and the value it presents — a literal, a binding, the builtin
// node a type name reads as.
type planDesc struct {
	kind     uint8
	forceVal bool
	val      planVal
}

// planKey is everything PlanMatch reads for one dispatch of a plannable word.
type planKey struct {
	argCount int
	mods     uint8
	nStack   uint8
	nWin     uint8
	stack    [planArgsMax]planVal
	win      [planArgsMax]planDesc
}

// dispatchPlan is PlanMatch's answer under key: the selected signature's
// index (-1: none matched), whether its positions are nil (a 0-arg or
// fallback selection), how many leading positions the forward tokens fill,
// and the speculative slot.
type dispatchPlan struct {
	key    planKey
	sig    int
	nilPos bool
	fwd    int
	specAt int
}

// planSet is one aggregate's plans, for the signature list they were made
// against (by identity — a copy of the aggregate that carries another list
// plans afresh), with the bounds its keys are built to. A set is never
// written once published: a new plan publishes a new set.
type planSet struct {
	sigs       []Signature
	maxBarrier int
	maxArgs    int
	plans      []dispatchPlan
}

// planHolder is an aggregate's plan store, shared by its copies: the
// aggregate may be dispatched from engines on several goroutines.
type planHolder struct {
	set atomic.Pointer[planSet]
}

// planSetFor returns the plan set fn dispatches from, or nil when fn is not
// a plannable aggregate: no holder, or a signature whose match reads more
// than an argument's tag.
func planSetFor(fn *FnDefInfo) *planSet {
	h := fn.plans
	if h == nil {
		return nil
	}
	if s := h.set.Load(); s != nil && sameList(s.sigs, fn.Signatures) {
		if s.maxArgs < 0 {
			return nil
		}
		return s
	}
	s := newPlanSet(fn.Signatures)
	h.set.Store(s)
	if s.maxArgs < 0 {
		return nil
	}
	return s
}

// newPlanSet is the empty plan set for sigs, with maxArgs -1 when the list
// is not plannable.
func newPlanSet(sigs []Signature) *planSet {
	s := &planSet{sigs: sigs}
	for i := range sigs {
		sig := &sigs[i]
		if sig.Fallback {
			continue
		}
		n := sig.TotalArgs()
		if n > planArgsMax || !tagDeterminedSig(sig) {
			s.maxArgs = -1
			return s
		}
		s.maxArgs = max(s.maxArgs, n)
		s.maxBarrier = max(s.maxBarrier, sig.BarrierPos)
	}
	return s
}

// tagDeterminedSig reports whether every argument of s is decided by what a
// planVal holds: TagDeterminedSigs' rule for one signature, with a
// type-literal slot admitted — sigTypeMatchesAsType reads a concrete value's
// tag and the two facts planVal carries, and the slot's own type by its
// lattice position, never its Behavior.
func tagDeterminedSig(s *Signature) bool {
	for idx := 0; idx < s.TotalArgs(); idx++ {
		if _, ok := SigPattern(s, idx); ok {
			return false
		}
		t := SigArgType(s, idx)
		if t == nil || t.Origin() != OriginBuiltin {
			return false
		}
		if s.TypeArgs != nil && s.TypeArgs[idx] {
			continue
		}
		if t.Equal(TOptions) || behaviorIsContent(t.Behavior()) {
			return false
		}
	}
	return true
}

// planKeyFor describes this dispatch for set, or reports that a value or
// token it would read is not decided by what a key holds.
func planKeyFor(set *planSet, h CollectHost, win *Tape, w WordInfo, resolved []Value, pointer int, insideForward bool) (planKey, bool) {
	k := planKey{argCount: w.ArgCount}
	// The forward limit as effectiveForwardLimit sets it per signature, at
	// its widest: /s first, then /f. PlanMatch reads /f on its own too.
	limit := set.maxBarrier
	if w.ForceForward {
		k.mods |= planForceForward
		limit = set.maxArgs
	}
	if w.ForceStack {
		k.mods |= planForceStack
		limit = 0
	}
	if insideForward {
		k.mods |= planInsideForward
	}
	if pointer+1 < win.Len() && IsWord(win.Ref(pointer+1)) {
		k.mods |= planNextIsWord
	}
	n := min(len(resolved), set.maxArgs)
	for j := 0; j < n; j++ {
		pv, ok := planValOf(&resolved[len(resolved)-1-j])
		if !ok {
			return k, false
		}
		k.stack[j] = pv
	}
	k.nStack = uint8(n)
	for i := 0; i < limit; i++ {
		at := pointer + 1 + i
		k.nWin = uint8(i + 1)
		if at >= win.Len() {
			k.win[i] = planDesc{kind: pdEnd}
			break
		}
		d, ok := planWindowDesc(h, win.Ref(at))
		if !ok {
			return k, false
		}
		k.win[i] = d
		if d.kind == pdStop {
			break
		}
	}
	return k, true
}

// planWindowDesc describes a forward token by what CollectCandidateScan reads
// of it, or reports a token whose treatment reads more.
func planWindowDesc(h CollectHost, tok *Value) (planDesc, bool) {
	if scanBoundaryToken(tok) || IsOpenParen(tok) {
		return planDesc{kind: pdStop}, true
	}
	if IsWord(tok) {
		ww, err := AsWord(tok)
		if err != nil { //covergate:allow IsWord guarantees a WordInfo payload
			return planDesc{}, false
		}
		if top, ok := h.DefTop(ww.Name); ok {
			pv, ok := planValOf(top)
			if !ok {
				return planDesc{}, false
			}
			if _, isFn := top.Data().(*FnDefInfo); isFn {
				return planDesc{kind: pdFn, forceVal: ww.ForceVal, val: pv}, true
			}
			return planDesc{kind: pdBound, val: pv}, true
		}
		if ww.Name == "true" || ww.Name == "false" {
			return planDesc{kind: pdBool}, true
		}
		if tn, isType := ResolveBuiltinTypeName(ww.Name); isType {
			return planDesc{kind: pdTypeName, val: planVal{tag: tn}}, true
		}
		return planDesc{kind: pdAtom}, true
	}
	if IsReachGroup(tok) || IsSugar(tok) {
		return planDesc{}, false
	}
	pv, ok := planValOf(tok)
	if !ok {
		return planDesc{}, false
	}
	return planDesc{kind: pdValue, val: pv}, true
}

// find returns the plan made under key, if any.
func (s *planSet) find(key *planKey) (*dispatchPlan, bool) {
	for i := range s.plans {
		if s.plans[i].key == *key {
			return &s.plans[i], true
		}
	}
	return nil, false
}

// apply lays plan p out at this dispatch: the forward positions are the
// tokens right after the word, the stack positions the top of the resolved
// stack, nearest first — the shape every PlanMatch answer has (recordPlan
// admits no other).
func (p *dispatchPlan) apply(fn *FnDefInfo, win *Tape, pointer int) (*Signature, []int, int, bool) {
	if p.sig < 0 {
		return nil, nil, p.specAt, true
	}
	sig := &fn.Signatures[p.sig]
	if p.nilPos {
		return sig, nil, p.specAt, true
	}
	positions := make([]int, sig.TotalArgs())
	for i := 0; i < p.fwd; i++ {
		positions[i] = pointer + 1 + i
	}
	if remaining := len(positions) - p.fwd; remaining > 0 {
		var buf [planArgsMax]int
		idx := resolvedIndicesBeforeInto(win, pointer, buf[:0], remaining)
		if len(idx) < remaining { //covergate:allow a hit's key is the recorded one: len(resolved) >= its stack count >= the plan's stack positions, and the index walk counts every resolved value
			return nil, nil, 0, false
		}
		for j := 0; j < remaining; j++ {
			positions[p.fwd+j] = idx[len(idx)-1-j]
		}
	}
	return sig, positions, p.specAt, true
}

// recordPlan publishes PlanMatch's answer under key — when apply reproduces
// it exactly. An answer of any other shape is left to be planned afresh
// every time.
func recordPlan(fn *FnDefInfo, set *planSet, key planKey, sig *Signature, positions []int, specAt int, win *Tape, pointer int) {
	if len(set.plans) >= planSetMax {
		return
	}
	p := dispatchPlan{key: key, sig: -1, specAt: specAt}
	if sig != nil {
		for i := range fn.Signatures {
			if &fn.Signatures[i] == sig {
				p.sig = i
				break
			}
		}
		if p.sig < 0 { //covergate:allow PlanMatch selects from fn's own signature list
			return
		}
		p.nilPos = positions == nil
		for _, pos := range positions {
			if pos > pointer {
				p.fwd++
			}
		}
	}
	gotSig, got, gotSpec, ok := p.apply(fn, win, pointer)
	if !ok || gotSig != sig || gotSpec != specAt || (got == nil) != (positions == nil) || !equalInts(got, positions) {
		return
	}
	plans := make([]dispatchPlan, len(set.plans), len(set.plans)+1)
	copy(plans, set.plans)
	next := &planSet{sigs: set.sigs, maxBarrier: set.maxBarrier, maxArgs: set.maxArgs, plans: append(plans, p)}
	fn.plans.set.CompareAndSwap(set, next)
}

// equalInts reports whether a and b hold the same ints.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// planDispatch is PlanMatch for the interpreter's run-time dispatch, through
// the aggregate's plans: a key it has planned answers from memory, any other
// is planned and remembered.
func planDispatch(h CollectHost, win *Tape, reg *Registry, fn *FnDefInfo, w WordInfo, resolved []Value, pointer int, insideForward bool) (*Signature, []int, int) {
	set := planSetFor(fn)
	if set == nil {
		return PlanMatch(h, win, reg, fn, w, resolved, pointer, insideForward, false, false)
	}
	key, ok := planKeyFor(set, h, win, w, resolved, pointer, insideForward)
	if !ok {
		return PlanMatch(h, win, reg, fn, w, resolved, pointer, insideForward, false, false)
	}
	if p, hit := set.find(&key); hit {
		if sig, positions, specAt, ok := p.apply(fn, win, pointer); ok {
			return sig, positions, specAt
		}
	}
	sig, positions, specAt := PlanMatch(h, win, reg, fn, w, resolved, pointer, insideForward, false, false)
	recordPlan(fn, set, key, sig, positions, specAt, win, pointer)
	return sig, positions, specAt
}
