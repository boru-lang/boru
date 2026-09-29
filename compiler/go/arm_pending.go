package compiler

import core "github.com/boru-lang/boru/core/go"

// arm_pending.go — an `if` arm's pending literal (NUR356).
//
// The interpreter splices an `if` arm onto its enclosing tape as a paren
// group, and a paren evaluates nothing it leaves: a list or map literal the
// arm ends with stays PENDING past the `if`, evaluated where a word takes it
// (`size`, a `def`, an enclosing literal's assembly), at the end of the run
// it is left in (the program, a fn's frame, a loop body's iteration), or
// never (a code-body slot runs it raw). The check pass runs the arm in a
// sub-engine of its own, whose end-of-run sweep evaluates the literal AT the
// arm, and the compiled code assembles it there. The two agree only while
// nothing between the arm and the interpreter's evaluation can tell:
//
//	def x 1 end def c true end if c [[x]] [0] end def x 2 end
//	  interpreted [[2]], and the arm's eager [x] read 1
//	def c true end if c [[print "q" 2]] [3] end print "a"
//	  interpreted a q, and the arm's eager print ran first
//
// The pass hands each branch the observable literals its arms' sweeps
// evaluated (emitBranch.pending, core.PendingResidue; a literal of scalar
// leaves evaluates to itself whenever it runs and is none). pendingArmRefusal
// then proves, per event sequence, that after such a branch the first event
// touching its value TAKES it the way the interpreter evaluates a pending
// literal — a committed call over a signature that evaluates its arguments,
// an enclosing literal's assembly, a def — or that the sequence ends first,
// where its run's own end evaluates what it leaves. Between the two, a
// literal that only READS bindings admits an event that binds, unbinds or
// purely reads another name (a `var` body's parameter teardown); one that
// calls admits nothing. An arm whose sequence ends with the literal still
// pending hands it to the branch holding the arm, unless that branch's arms
// end where the interpreter evaluates what they leave (emitBranch.sweptArms
// — the `case` desugar's chain). Anything else declines the program with
// pendingArmReason.

// pendingArmReason is the decline of a branch whose pending literal the
// program does not consume before running something that could tell
// (NUR356).
const pendingArmReason = "a branch arm leaves a list or map literal the interpreter keeps pending past the `if`, and the program runs code before it evaluates it (NUR356)"

// pendingArmRefusal is the NUR356 decline for one event sequence whose end
// evaluates what it leaves — the program's root stream, or a fn unit's body
// — or "" when every pending literal a branch leaves in it is consumed in
// time and no call matched at run time takes a literal whose evaluation may
// have an effect (eagerLiteralRefusal).
func (es *EmitState) pendingArmRefusal(events []EmitEvent) string {
	if _, reason := es.pendingArmWalk(events); reason != "" {
		return reason
	}
	return es.eagerLiteralRefusal(events)
}

// lowerUnitEvents lowers a fn unit's events with flw, after the unit's
// NUR356 refusal (pendingArmRefusal); the decline reason, "" when lowered.
func (es *EmitState) lowerUnitEvents(flw *lowerer, rec *fnUnitRec) string {
	if reason := es.pendingArmRefusal(rec.frag.events); reason != "" {
		return reason
	}
	return flw.lowerEvents(rec.frag.events, rec.frag.startSeq)
}

// pendingArmWalk walks one event sequence in order. open is the residue the
// sequence ENDS with still unconsumed — for the caller to discharge (its end
// evaluates it) or to carry up (an arm's).
func (es *EmitState) pendingArmWalk(events []EmitEvent) (open core.PendingResidue, reason string) {
	pending, multi, quietArms := -1, false, false
	for i := 0; i < len(events); i++ {
		ev := &events[i]
		if open.Left {
			if bindsPendingDef(events, i, pending) {
				// The def's bind twin stands at its name, ahead of the
				// value's bind: the pair is the def taking the value.
				i++
				ev = &events[i]
			}
			switch {
			case !multi && consumesPendingEvaluated(ev, pending):
				// The one value the branch leaves is taken, and the
				// interpreter evaluates its literal right here.
				open = core.PendingResidue{}
			case !eventReadsSeq(ev, pending) && es.pendingUntouched(ev, open, quietArms):
				// Nothing the literal's evaluation could observe.
				continue
			default:
				return core.PendingResidue{}, pendingArmReason
			}
		}
		left, reason := es.eventLeavesPending(ev)
		if reason != "" {
			return core.PendingResidue{}, reason
		}
		if left.Left {
			open, pending, multi = left, ev.seq, branchLeavesMany(ev.br)
			quietArms = !es.mayEffect(ev, effectScope{seen: map[int]bool{}, binds: anyBind})
		}
	}
	return open, ""
}

// eventLeavesPending is the residue ev leaves on its sequence's stack past
// itself: a branch's, from its arms — its own model sweeps' (pending) and
// what a nested branch left at an arm's end — unless its arms end where the
// interpreter evaluates what they leave. A literal left pending at a
// condition's end is read raw by the construct (its truthiness), and one at
// a loop body's end is evaluated as its iteration ends; every other kind
// nests no sequence.
func (es *EmitState) eventLeavesPending(ev *EmitEvent) (core.PendingResidue, string) {
	switch ev.kind {
	case evBranch:
		br := ev.br
		if reason := es.pendingArmCond(br.condFrag); reason != "" {
			return core.PendingResidue{}, reason
		}
		left := br.pending
		for _, arm := range []*EmitFragment{br.then, br.els} {
			if arm == nil {
				continue
			}
			open, reason := es.pendingArmWalk(arm.events)
			if reason != "" {
				return core.PendingResidue{}, reason
			}
			if !br.sweptArms {
				left = left.Merge(open)
			}
		}
		return left, ""
	case evLoop:
		if reason := es.pendingArmCond(ev.loop.cond); reason != "" {
			return core.PendingResidue{}, reason
		}
		if ev.loop.body != nil {
			_, reason := es.pendingArmWalk(ev.loop.body.events)
			return core.PendingResidue{}, reason
		}
	}
	return core.PendingResidue{}, ""
}

// pendingArmCond walks a condition fragment, whose value the construct
// reads raw: a pending literal left at its end declines.
func (es *EmitState) pendingArmCond(frag *EmitFragment) string {
	if frag == nil {
		return ""
	}
	open, reason := es.pendingArmWalk(frag.events)
	if open.Left {
		return pendingArmReason
	}
	return reason
}

// pendingUntouched reports whether ev, run between a branch and the
// evaluation of the pending literals it left (residue p), leaves what they
// evaluate to and what the program observes unchanged. For literals that
// read bindings and call nothing: an event that binds or unbinds a name
// none of them reads, or one that may have no effect (mayEffect — binding
// nothing, running no code). For literals that call: such an event only
// where the branch's arms themselves may have none (quietArms) — then
// neither side can observe the other, whichever runs first. Anything else
// declines.
func (es *EmitState) pendingUntouched(ev *EmitEvent, p core.PendingResidue, quietArms bool) bool {
	if !p.Calls {
		switch ev.kind {
		case evDynBind:
			return !p.ReadsName(ev.dyn.name)
		case evBindTwin:
			return ev.twin.idx >= 0 && ev.twin.idx < len(es.bindTwins) && !p.ReadsName(es.bindTwins[ev.twin.idx].Name)
		}
	} else if !quietArms {
		return false
	}
	return !es.mayEffect(ev, effectScope{seen: map[int]bool{}, binds: func(name string) bool {
		return p.ReadsName(name) || es.dynReadName(name)
	}})
}

// dynReadName reports whether compiled code reads name from the registry at
// run time — a dynamic-scope lookup, a routed or a live read — where a
// binding made in between reaches it.
func (es *EmitState) dynReadName(name string) bool {
	return es.dynScopeNames[name] || es.routedNames[name] || es.liveReadNames[name]
}

// bindsPendingDef reports whether events[i] is the bind twin of a def whose
// value is event seq's — the def's own value bind (evDynBind, the same def
// site) follows it at once.
func bindsPendingDef(events []EmitEvent, i, seq int) bool {
	if events[i].kind != evBindTwin || i+1 >= len(events) {
		return false
	}
	next := &events[i+1]
	return next.kind == evDynBind && next.dyn.srcSeq == seq && next.dyn.pos == events[i].twin.pos
}

// branchLeavesMany reports whether a branch's arms leave more than one
// value (RecordBranch's residualN): a pending literal beneath another value
// is taken by no single consumer the walk can name.
func branchLeavesMany(br *emitBranch) bool {
	return (br.then != nil && br.then.residualN > 1) || (br.els != nil && br.els.residualN > 1)
}

// consumesPendingEvaluated reports whether ev takes event seq's value in a
// way the interpreter evaluates a pending literal at: a literal's assembly
// (its element run ended there), a def of it, a user fn's call, or a native
// call committed at compile time over a signature that evaluates every
// argument. A call that re-matches at run time (a poly, a dynamic apply, an
// optimistic bake's no-match arm) may take no signature — the interpreter
// then reports the literal as written, never evaluated — or one whose slot
// takes it raw; those decline.
func consumesPendingEvaluated(ev *EmitEvent, seq int) bool {
	switch ev.kind {
	case evCall:
		c := &ev.call
		if !eventReadsSeq(ev, seq) {
			return false
		}
		if c.makeList || c.makeMap {
			return true
		}
		return c.sig != nil && !c.poly && c.dynApply == 0 && !c.dynMixed && c.dynMethod == nil &&
			!c.live && !c.interp && c.xmlTmpl == nil && !c.spliceDyn && !c.hostSplice &&
			c.listReStep == nil && c.nativeSplit == nil && c.typedBind == nil && c.typeRun == nil &&
			len(c.sig.NoEvalArgs) == 0 && len(c.sig.NoEvalMapArgs) == 0
	case evCallUser:
		return eventReadsSeq(ev, seq) && !ev.uc.mayMiss && ev.uc.poly == nil
	case evDynBind:
		return ev.dyn.srcSeq == seq
	case evStore:
		return ev.store.src.kind == opEvent && ev.store.src.idx == seq
	}
	return false
}

// eventReadsSeq reports whether one of ev's operands is event seq's value.
func eventReadsSeq(ev *EmitEvent, seq int) bool {
	reads := false
	forEachOperand(ev, func(op EmitOperand) {
		reads = reads || (op.kind == opEvent && op.idx == seq)
	})
	return reads
}
