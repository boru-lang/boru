package compiler

import core "github.com/boru-lang/boru/core/go"

// eager_literal.go — a literal a call matched at run time takes (NUR356).
//
// A list or map literal written as a call's operand is PENDING until a
// signature takes it: the interpreter evaluates it only once the match
// succeeds, and never where none does. The check pass models the match it
// can see and evaluates the literal there — right before the call, after
// the call's other operands — so the compiled code assembles it just before
// the call. For a call committed at compile time that is the interpreter's
// order exactly. For one matched at RUN time (a poly dispatch, a dynamic
// apply, a rematch, a user call whose param contract may refuse) the
// assembly runs even where the run then matches nothing:
//
//	def h fn [[] [Any] [3]] end each (h) [print "p" 1]
//	  interpreted: each's no-match, and nothing printed
//	  compiled:    p printed, then each's no-match
//
// The assembly is the block of events the sequence recorded right before
// the call inside the literal's element runs (EmitEvent.litDepth), and the
// assemblies they end in. A block any of whose events may have an effect
// the program observes (mayEffect) declines, with eagerLiteralReason; a
// quiet one — reads, value words, literal assemblies, branches of them,
// calls of fns whose bodies are quiet — builds the same value whenever it
// runs, with nothing to observe, and a poly's no-match report renders it as
// written (PolyRef.Raw, NUR352).

// eagerLiteralReason is the decline of a call matched at run time over a
// list or map literal whose compiled assembly may have an effect (NUR356).
const eagerLiteralReason = "a call matched at run time takes a list or map literal whose evaluation may have an effect: the interpreter evaluates it only once a signature takes it, and never where none does (NUR356)"

// eagerLiteralRefusal walks an event sequence recorded from body (the
// program's tokens, or the fn unit's), and every sequence nested in it, for
// a call matched at run time over a literal whose assembly may have an
// effect; "" when there is none.
func (es *EmitState) eagerLiteralRefusal(events []EmitEvent, body []core.Value, reg *core.Registry) string {
	for i := range events {
		if es.eagerLiteralEffect(events, i, body, reg) {
			return eagerLiteralReason
		}
		for _, frag := range childFragments(&events[i]) {
			if frag == nil {
				continue
			}
			if reason := es.eagerLiteralRefusal(frag.events, body, reg); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// eagerLiteralEffect reports whether events[i] is a call matched at run
// time (runtimeMatched) whose literal operands' assembly block — the events
// recorded right before it, deeper in its unit's inline literal runs than
// the call (EmitEvent.litDepth), and the assemblies they end in (recorded
// once the run closes) — holds an event that may have an effect, where the
// match may miss (literalMatchSure).
func (es *EmitState) eagerLiteralEffect(events []EmitEvent, i int, body []core.Value, reg *core.Registry) bool {
	if !es.runtimeMatched(&events[i]) || es.literalMatchSure(events, i, body, reg) {
		return false
	}
	for j := i - 1; j >= 0 && (events[j].litDepth > events[i].litDepth || rawRenderedAssembly(&events[j])); j-- {
		if es.mayEffect(&events[j], effectScope{seen: map[int]bool{}, binds: anyBind}) {
			return true
		}
	}
	return false
}

// literalMatchSure reports whether the poly events[i] matches at run time
// whatever its literals evaluate to: every operand is a constant or the
// assembly of a literal written in body, and the word's first match over the
// constants and the literals AS WRITTEN — the window the interpreter matches
// before it evaluates them — takes a signature that evaluates each literal's
// slot. The interpreter then evaluates the literals exactly where the
// compiled code assembles them, right before the call: a literal's effect
// runs on both lanes, in the same order (`make Entity {… a:(m set k v)}`
// over a gradual m, whose poly `set` may pick a flex write).
func (es *EmitState) literalMatchSure(events []EmitEvent, i int, body []core.Value, unitReg *core.Registry) bool {
	c := &events[i].call
	if events[i].kind != evCall || !c.poly || c.dynApply > 0 || c.dynMixed || c.dynMethod != nil || c.nativeSplit != nil {
		return false
	}
	fn := es.polyWord(c)
	if fn == nil {
		return false
	}
	window := make([]core.Value, len(c.ops))
	var lits []int
	for k, op := range c.ops {
		switch {
		case op.kind == opConst && op.idx >= 0 && op.idx < len(es.consts):
			window[k] = es.consts[op.idx]
		case op.kind == opType:
			t, ok := es.typeOperand(op.idx, unitReg)
			if !ok {
				return false
			}
			window[k] = core.NewTypeLiteral(t)
		case op.kind == opEvent && op.resIdx == 0:
			tok, ok := assembledLiteral(events[:i], op.idx, body)
			if !ok {
				return false
			}
			window[k] = tok
			lits = append(lits, k)
		default:
			return false
		}
	}
	mr := core.MatchSignature(fn.Signatures, window, core.WordInfo{ArgCount: len(window)})
	if len(lits) == 0 || mr == nil || mr.Sig == nil || mr.Sig.Fallback {
		return false
	}
	for _, k := range lits {
		if rawSlot(mr.Sig, k) {
			return false
		}
	}
	return true
}

// typeOperand is the type an opType operand names (Program.Types idx), as
// the run pushes it (OpPushType): looked up in the unit's own registry (a
// module fn's minted types live there), the program's, then the kernel's —
// none where the run installs a node of its own in the pass's place (a
// type-run install, NUR308), whose match the pass cannot tell.
func (es *EmitState) typeOperand(idx int, unitReg *core.Registry) (*core.Type, bool) {
	if idx < 0 || idx >= len(es.types) || es.runTypes[es.types[idx].ID] {
		return nil, false
	}
	id := es.types[idx].ID
	for _, r := range []*core.Registry{unitReg, es.reg} {
		if r == nil {
			continue
		}
		if t := r.Types.LookupByID(id); t != nil {
			return t, true
		}
	}
	t := core.Builtin.LookupByID(id)
	return t, t != nil
}

// assembledLiteral is the literal written in body that the assembly event
// seq among events built (OpMakeList / OpMakeMap), or false.
func assembledLiteral(events []EmitEvent, seq int, body []core.Value) (core.Value, bool) {
	for j := range events {
		if events[j].seq == seq {
			if !rawRenderedAssembly(&events[j]) {
				return core.Value{}, false
			}
			return literalTokenAt(body, events[j].call.pos)
		}
	}
	return core.Value{}, false
}

// rawSlot reports whether sig takes its position k as written — a code
// body, a quoted or a form slot, a map whose values it does not evaluate —
// where the interpreter never evaluates a literal it binds there.
func rawSlot(sig *core.Signature, k int) bool {
	return sig.NoEvalArgs[k] || sig.NoEvalMapArgs[k] || sig.QuoteArgs[k] || sig.FormArgs[k]
}

// effectScope is where mayEffect judges an event: the units already on
// the walk (seen), and which bindings the event's scope makes observable —
// binds, nil inside a called fn's frame (it takes its defs down), else the
// test of a bound name.
type effectScope struct {
	seen  map[int]bool
	binds func(name string) bool
}

// anyBind makes every binding observable: a literal's own element run, or
// an arm whose literal the interpreter evaluates later.
func anyBind(string) bool { return true }

// mayEffect reports whether running ev may have an effect the program
// observes: a native word called for its effect alone (no result — print)
// or declaring one (core.CompileSideEffect — an IO read, the clock, a
// random draw, a flex container's in-place write; a poly over a word any of
// whose overloads does), one that runs code it is not handed as a compiled
// body (a dynamic apply, a splice, a code-body word over an interpreted
// body), a raise, an interpreter island, an observable binding
// (effectScope.binds), or a call of a fn — a code body's included — whose
// body may. Reads, value words, literal and template assemblies and
// branches of them do not.
func (es *EmitState) mayEffect(ev *EmitEvent, sc effectScope) bool {
	switch ev.kind {
	case evCall:
		c := &ev.call
		if restartRead(ev) || c.interp || c.xmlTmpl != nil || c.live {
			return false
		}
		if c.nout == 0 || c.hostSplice || c.spliceDyn || c.dynMixed || c.dynMethod != nil || c.dynApply > 0 || c.typeRun != nil {
			return true
		}
		if c.sig != nil && sideEffect(c.sig) || c.sig == nil && c.poly && es.polyEffectful(c) {
			return true
		}
		if c.sig != nil && !runsCode(c.sig) || c.sig == nil && c.poly && !es.polyOverloads(c, runsCode) {
			return false
		}
		return c.sig == nil && !c.poly || es.bodiesMayEffect(c.ops, sc)
	case evCallUser:
		return es.unitMayEffect(&ev.uc, sc)
	case evBranch, evLoop:
		for _, frag := range childFragments(ev) {
			if frag != nil && es.eventsMayEffect(frag.events, sc) {
				return true
			}
		}
		return false
	case evDynBind:
		return sc.binds != nil && sc.binds(ev.dyn.name)
	case evBindTwin:
		return sc.binds != nil && (ev.twin.idx < 0 || ev.twin.idx >= len(es.bindTwins) || sc.binds(es.bindTwins[ev.twin.idx].Name))
	case evStore:
		return sc.binds != nil
	}
	return true
}

// runsCode reports whether a native signature runs code it is handed — a
// code body, a spliced or re-stepped run — whose effects are the code's.
func runsCode(sig *core.Signature) bool {
	return sig.CompileEffect&(core.CompileDynBody|core.CompileFallbackBody|core.CompileRunsBodyOnRegistry|core.CompileResteps) != 0
}

// sideEffect reports whether a native signature declares an effect of its
// own (core.CompileSideEffect) or a raise its operands' values decide
// (core.CompileValueDiverges — `div` / `mod` by a zero the run computes): run
// early, either is observed before the interpreter's own answer.
func sideEffect(sig *core.Signature) bool {
	return sig.CompileEffect.Has(core.CompileSideEffect | core.CompileValueDiverges)
}

// valueDiverges reports whether a native signature may raise by its
// operands' values (core.CompileValueDiverges).
func valueDiverges(sig *core.Signature) bool {
	return sig.CompileEffect.Has(core.CompileValueDiverges)
}

// polyEffectful reports whether a poly dispatch of a native word may pick an
// overload that is not quiet (sideEffect): every overload has an effect of
// its own, some overload may raise by its operands' values, or the word is
// none its registry holds. Where only some overloads have an effect — `push`
// over a gradual list, whose FlexList overload writes in place — the walks
// judge the poly quiet and the compiled code guards that judgement: the
// poly is marked (emitCall.quietGuard, PolyRef.QuietGuard) and the run's
// pick of an effectful overload is a designed defer, never the early effect.
// A raise is not guarded so: its overload also answers where it does not
// raise, which a defer would refuse.
func (es *EmitState) polyEffectful(c *emitCall) bool {
	if !es.polyOverloads(c, sideEffect) {
		return false
	}
	if !es.polyOverloads(c, valueDiverges) && es.polyOverloads(c, func(s *core.Signature) bool { return !s.Fallback && !sideEffect(s) }) {
		c.quietGuard = true
		return false
	}
	return true
}

// polyOverloads reports whether a poly dispatch of a native word may pick an
// overload of which has holds, or names a word its registry does not hold.
func (es *EmitState) polyOverloads(c *emitCall, has func(*core.Signature) bool) bool {
	fn := es.polyWord(c)
	if fn == nil {
		return true
	}
	for i := range fn.Signatures {
		if has(&fn.Signatures[i]) {
			return true
		}
	}
	return false
}

// polyWord is the word a poly dispatch re-matches over — in its module's
// registry (polyReg), else the program's — or nil when none holds it.
func (es *EmitState) polyWord(c *emitCall) *core.FnDefInfo {
	reg := c.polyReg
	if reg == nil {
		reg = es.reg
	}
	if reg == nil {
		return nil
	}
	return reg.Lookup(c.word)
}

// bodiesMayEffect reports whether the code a code-body word runs may have
// an effect: each body it is handed compiled as a closure unit whose events
// may, or no compiled body at all (the code is the run's to know).
func (es *EmitState) bodiesMayEffect(ops []EmitOperand, sc effectScope) bool {
	bodies := 0
	for _, op := range ops {
		if op.kind != opClosure {
			continue
		}
		bodies++
		if es.unitMayEffect(&emitUserCall{unit: op.closureUnit}, sc) {
			return true
		}
	}
	return bodies == 0
}

// eventsMayEffect is mayEffect over a sequence.
func (es *EmitState) eventsMayEffect(events []EmitEvent, sc effectScope) bool {
	for i := range events {
		if es.mayEffect(&events[i], sc) {
			return true
		}
	}
	return false
}

// unitMayEffect reports whether a user call's fn may have an effect: a
// runtime pick among overloads, a unit not compiled, or a body with an event
// that may — its binds observable only where the unit keeps its defs past
// its frame, into its caller's scope. A unit already on the walk adds
// nothing of its own.
func (es *EmitState) unitMayEffect(uc *emitUserCall, sc effectScope) bool {
	if uc.poly != nil || uc.unit < 0 || uc.unit >= len(es.fnRecs) {
		return true
	}
	if sc.seen[uc.unit] {
		return false
	}
	sc.seen[uc.unit] = true
	rec := es.fnRecs[uc.unit]
	if rec.frag == nil {
		return true
	}
	inner := effectScope{seen: sc.seen}
	if rec.keepsDefs {
		inner.binds = sc.binds
	}
	return es.eventsMayEffect(rec.frag.events, inner)
}

// runtimeMatched reports whether ev's signature is matched at run time,
// over the values the run holds — a poly dispatch, a dynamic or shaped
// apply of a fn the program did not write, an optimistic bake's no-match
// arm, a rematch trap, a user call over an argument its contract may refuse
// or over several overloads.
func (es *EmitState) runtimeMatched(ev *EmitEvent) bool {
	switch ev.kind {
	case evCall:
		c := &ev.call
		return c.poly || (c.dynApply > 0 && !es.knownFnLead(c.ops)) || c.dynMixed || c.dynMethod != nil || c.nativeSplit != nil
	case evCallUser:
		return ev.uc.mayMiss || ev.uc.poly != nil
	case evTrap:
		return len(ev.trap.rematchOps) > 0
	}
	return false
}

// knownFnLead reports whether a dynamic apply's fn operand (its first) is
// a fn the program wrote — a constant fn value or a compiled closure — whose
// signature the pass matched the apply's arguments against, as the run
// will.
func (es *EmitState) knownFnLead(ops []EmitOperand) bool {
	if len(ops) == 0 {
		return false
	}
	switch lead := ops[0]; lead.kind {
	case opClosure:
		return true
	case opConst:
		return lead.idx >= 0 && lead.idx < len(es.consts) && core.IsAppliableFn(es.consts[lead.idx])
	}
	return false
}

// contractMayMiss reports whether a compiled call of rec over args may fail
// its param contract at run time: an argument not proved to conform to its
// param's declared type — gradual, or of a type wider than it — or a param
// with a pattern (checked against the value).
func contractMayMiss(rec *fnUnitRec, args []core.Value) bool {
	for i, a := range args {
		if i < len(rec.paramPatterns) && rec.paramPatterns[i] != nil {
			return true
		}
		if i >= len(rec.paramTypes) || rec.paramTypes[i] == nil || rec.paramTypes[i].Equal(core.TAny) {
			continue
		}
		if a.Dynamic || a.Parent == nil || !a.Parent.ConformsTo(rec.paramTypes[i]) {
			return true
		}
	}
	return false
}
