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

// eagerLiteralRefusal walks an event sequence, and every sequence nested in
// it, for a call matched at run time over a literal whose assembly may have
// an effect; "" when there is none.
func (es *EmitState) eagerLiteralRefusal(events []EmitEvent) string {
	for i := range events {
		if es.eagerLiteralEffect(events, i) {
			return eagerLiteralReason
		}
		for _, frag := range childFragments(&events[i]) {
			if frag == nil {
				continue
			}
			if reason := es.eagerLiteralRefusal(frag.events); reason != "" {
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
// once the run closes) — holds an event that may have an effect.
func (es *EmitState) eagerLiteralEffect(events []EmitEvent, i int) bool {
	if !es.runtimeMatched(&events[i]) {
		return false
	}
	for j := i - 1; j >= 0 && (events[j].litDepth > events[i].litDepth || rawRenderedAssembly(&events[j])); j-- {
		if es.mayEffect(&events[j], effectScope{seen: map[int]bool{}, binds: anyBind}) {
			return true
		}
	}
	return false
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
// observes: a native word called for its effect alone (no result — print),
// one that runs code it is not handed as a compiled body (a dynamic apply,
// a splice, a code-body word over an interpreted body), a raise, an
// interpreter island, an observable binding (effectScope.binds), or a call
// of a fn — a code body's included — whose body may. Reads, value words,
// literal and template assemblies and branches of them do not.
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
		if c.sig != nil && !runsCode(c.sig) || c.sig == nil && c.poly && !es.polyRunsCode(c) {
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

// polyRunsCode reports whether a poly dispatch of a native word may pick an
// overload that runs code, or names a word its registry does not hold.
func (es *EmitState) polyRunsCode(c *emitCall) bool {
	reg := c.polyReg
	if reg == nil {
		reg = es.reg
	}
	fn := reg.Lookup(c.word)
	if fn == nil {
		return true
	}
	for i := range fn.Signatures {
		if runsCode(&fn.Signatures[i]) {
			return true
		}
	}
	return false
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
