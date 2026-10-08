package compiler

import (
	core "github.com/boru-lang/boru/core/go"
)

// The READ-SITE GUARD (NUR361). A gradual read the interpreter dispatches as
// a word when its binding holds a fn at run time (NUR123, NUR207) is planned
// as a deopt point whose test runs where the read's STATEMENT begins, before
// any of its effects. Two shapes had no such point and lowered the read as a
// plain push, a silent wrong answer when the value was a fn:
//
//   - a read nested in a body the unit or the root runs inline — a `var`
//     body, a branch arm, a loop body — whose statement is no body token of
//     the unit (`each [var [[q] def v (mk) v]] xs`: `[[fn v]]` compiled, the
//     interpreter's `[[5]]`);
//   - a read whose value is produced inside the SAME statement as the read
//     (`var [[] def v (mk) [v]]`, `if c [def v (mk) [v]] [0]`): the point's
//     test ran at the statement's start, before the producer, over a value
//     home not yet written, and never fired.
//
// No island can take such a read over: the interpreter would resume in the
// middle of a nested body. So the read is GUARDED where it happens instead —
// a designed defer, loud (DeoptSpec.Bail), tested right after the read in
// the event stream of the frame that holds it: before the next event that
// frame records after the read, or, when the read is the frame's last act,
// after the event before it. The guard runs exactly on the path the read
// runs on, after its producer, and costs one test when the value is data.
//
// A read whose guard the lowering could not seat (its anchor event was never
// lowered, or its value has no home there) declines the unit: never a slot
// push where the interpreter dispatches.

// readSite is one bare gradual read and where it stands in its frame's event
// stream: before is the seq of the first event the frame recorded after the
// read (-1: none), after the seq of the last one before it (-1: none), fragID
// the frame's fragment (0: the root frame). owner is the unit record the read
// belongs to (nil: the program root), prod the event that produced its value.
type readSite struct {
	id     string
	name   string
	pos    core.SrcPos
	before int
	after  int
	owner  *fnUnitRec
	fragID int
	prod   producer
}

// siteKey names one unit's reads of one value: the guards are armed per
// unit, since a value one unit produces may be read in another's frame (a
// closure's capture), where it has no home of the producer's.
type siteKey struct {
	owner *fnUnitRec
	id    string
}

// noteReadSite records the site of a gradual read of a value an event
// produced, in the innermost frame (a fn-typed read has its own strict
// accounting; a value no event produced — a param — has no producer home).
func (es *EmitState) noteReadSite(v core.Value, name string, pos core.SrcPos, owner *fnUnitRec) {
	pr, produced := es.producedBy[v.ID]
	if core.IsFnTypedCarrier(v) || !produced {
		return
	}
	n := len(es.frames) - 1
	s := &readSite{id: v.ID, name: name, pos: pos, before: -1, after: -1, owner: owner, fragID: es.frameID(n), prod: pr}
	if k := len(es.frames[n]); k > 0 {
		s.after = es.frames[n][k-1].seq
	}
	if es.readSites == nil {
		es.readSites = map[string][]*readSite{}
	}
	es.readSites[v.ID] = append(es.readSites[v.ID], s)
	es.pendingSites = append(es.pendingSites, s)
}

// frameID is the identity of the frame at depth n: its fragment's id, or 0
// for the root frame.
func (es *EmitState) frameID(n int) int {
	if n <= 0 {
		return 0
	}
	return es.fragIDs[n-1]
}

// resolveReadSites seats the event seq, just appended to the frame at depth
// n, as the `before` anchor of every read site that frame recorded since its
// last event.
func (es *EmitState) resolveReadSites(n, seq int) {
	if len(es.pendingSites) == 0 {
		return
	}
	id := es.frameID(n)
	kept := es.pendingSites[:0]
	for _, s := range es.pendingSites {
		if s.fragID == id {
			s.before = seq
			continue
		}
		kept = append(kept, s)
	}
	es.pendingSites = kept
}

// guardReadSites arms owner's read-site guards of the value id (lowered by
// the owner's lowering at each of its sites).
func (es *EmitState) guardReadSites(owner *fnUnitRec, id string) {
	if es.siteGuarded == nil {
		es.siteGuarded = map[siteKey]bool{}
	}
	es.siteGuarded[siteKey{owner, id}] = true
}

// firstReadSite is owner's first recorded read site of the value id, or nil.
func (es *EmitState) firstReadSite(id string, owner *fnUnitRec) *readSite {
	for _, s := range es.readSites[id] {
		if s.owner == owner {
			return s
		}
	}
	return nil
}

// indexReadSites seats the lowering's anchors for every armed read of the
// unit it lowers (lw.rec; nil at the program root) and the emission
// target's deopt table.
func (lw *lowerer) indexReadSites(table *[]DeoptSpec) {
	lw.siteTable = table
	for k := range lw.es.siteGuarded {
		if k.owner == lw.rec {
			lw.indexSitesOf(k.id)
		}
	}
}

// armReadSites arms the value id's guards mid-lowering (a point whose test
// would precede its producer, emitDeoptsBefore) and seats their anchors. It
// reports whether the read is guarded.
func (lw *lowerer) armReadSites(id string) bool {
	if lw.es.siteGuarded[siteKey{lw.rec, id}] {
		return true
	}
	if !lw.es.siteGuardable(lw.rec, id, lw.topEvents, lw.promoted) {
		return false
	}
	lw.es.guardReadSites(lw.rec, id)
	lw.indexSitesOf(id)
	return true
}

// indexSitesOf seats the anchors of the value id's sites in this lowering's
// unit: before the event the frame recorded after the read, else after the
// one before it, else — a nested frame that records no event, an arm holding
// the read alone — once that frame's events are lowered.
func (lw *lowerer) indexSitesOf(id string) {
	for _, s := range lw.es.readSites[id] {
		if s.owner != lw.rec {
			continue
		}
		switch {
		case s.before >= 0:
			if lw.sitesBefore == nil {
				lw.sitesBefore = map[int][]*readSite{}
			}
			lw.sitesBefore[s.before] = append(lw.sitesBefore[s.before], s)
		case s.after >= 0:
			if lw.sitesAfter == nil {
				lw.sitesAfter = map[int][]*readSite{}
			}
			lw.sitesAfter[s.after] = append(lw.sitesAfter[s.after], s)
		default:
			if lw.sitesAtEnd == nil {
				lw.sitesAtEnd = map[int][]*readSite{}
			}
			lw.sitesAtEnd[s.fragID] = append(lw.sitesAtEnd[s.fragID], s)
		}
	}
}

// emitReadSiteGuards lowers the guards seated at one anchor: each tests the
// read's value where it lives — the producer's promoted frame slot, else its
// entry on the simulated stack — and raises the designed defer when it holds
// a fn (DeoptSpec.Bail). A value with no home there declines the unit: never
// a slot push where the interpreter dispatches.
func (lw *lowerer) emitReadSiteGuards(sites []*readSite) string {
	for _, s := range sites {
		spec := DeoptSpec{Name: s.name, Pos: s.pos, Slot: -1, Depth: -1, Token: -1, RetPC: -1, Bail: true}
		if slot, ok := lw.promoted[s.prod.seq]; ok {
			spec.Slot = slot + s.prod.idx
		} else {
			for i := len(lw.vm) - 1; i >= 0 && spec.Depth < 0; i-- {
				if lw.vm[i].seq == s.prod.seq && lw.vm[i].idx == s.prod.idx {
					spec.Depth = len(lw.vm) - 1 - i
				}
			}
			if spec.Depth < 0 {
				return readSiteUnguarded
			}
		}
		*lw.siteTable = append(*lw.siteTable, spec)
		lw.emit(OpDeoptIfFn, len(*lw.siteTable)-1, s.pos)
	}
	return ""
}

// readSiteUnguarded is the decline of a gradual read whose guard could not be
// seated (NUR361).
const readSiteUnguarded = "a gradual read in a nested body has no seated guard: the interpreter dispatches it as a word when it holds a fn (NUR361)"

// producerAhead reports whether the value a read point tests is produced at
// or after the frame's top-level event i — the point's test would run before
// its value exists (NUR361). A value no event of the frame produces is not.
func producerAhead(events []EmitEvent, i, seq int) bool {
	found := false
	for k := i; k < len(events) && !found; k++ {
		walkEvents(events[k:k+1], func(ev *EmitEvent) {
			if ev.seq == seq {
				found = true
			}
		})
	}
	return found
}

// nestedReadUnplaced reports whether the unit read of value id (produced by
// event seq) that no deopt point could take has NO statement of the unit to
// deopt from — it sits in a body the unit runs inline, a var body, an arm, a
// loop body (deoptStatementStart places none) — and its guard can be seated
// (siteGuardable). A read whose statement was placed but whose stack there
// is not the interpreter's (deoptDeferred) keeps its slot push, as before.
func (es *EmitState) nestedReadUnplaced(rec *fnUnitRec, id, name string, seq int) bool {
	ci, direct := es.deoptConsumer(rec, id, seq, -1)
	if _, placed := es.deoptStatementStart(rec, seq, name, rec.wordReadFirst[id], ci, direct); placed {
		return false
	}
	return es.siteGuardable(rec, id, rec.frag.events, nil)
}

// siteGuardable reports whether the gradual read of value id in owner (nil:
// the program root) is one a site guard serves: it has a recorded site, and
// no event of the frame that consumes it applies a fn value the way the
// interpreter's word dispatch does — a dynamic apply, a shaped method call,
// the forward-drift window, a list literal that re-steps its elements, a
// fallback island. A read nothing consumes flows to the unit's residual,
// whose frame replay or body-tail apply dispatches it where one is armed.
// promoted maps a producer to its frame slot once the lowering promoted it.
func (es *EmitState) siteGuardable(owner *fnUnitRec, id string, events []EmitEvent, promoted map[int]int) bool {
	site := es.firstReadSite(id, owner)
	if site == nil {
		return false
	}
	pr := site.prod
	slot, hasSlot := promoted[pr.seq]
	is := func(op EmitOperand) bool {
		return (op.kind == opEvent && op.idx == pr.seq && op.resIdx == pr.idx) || (hasSlot && op.kind == opLocal && op.idx == slot+pr.idx)
	}
	consumed, applied := false, false
	walkEvents(events, func(ev *EmitEvent) {
		uses := false
		forEachOperand(ev, func(op EmitOperand) { uses = uses || is(op) })
		if !uses {
			return
		}
		consumed = true
		applied = applied || appliesFnOperand(ev)
	})
	if applied {
		return false
	}
	if !consumed && owner != nil && (owner.dynFrameW > 0 || len(owner.applyChain) > 0 || owner.dynTrailArity > 0) {
		return false
	}
	return true
}

// appliesFnOperand reports whether ev dispatches a fn operand as the
// interpreter's word dispatch would: a dynamic apply, a shaped method call,
// the forward-drift window, a re-stepping list literal, a hosted splice or a
// fallback island.
func appliesFnOperand(ev *EmitEvent) bool {
	switch ev.kind {
	case evFallback:
		return true
	case evCall:
		c := &ev.call
		return c.dynApply > 0 || c.dynMixed || c.dynMethod != nil || c.listReStep != nil || c.hostSplice || c.generic
	}
	return false
}

// guardDroppedReads arms the site guards of the gradual reads whose deopt
// points owner drops (its islands' names cannot be served): each such read
// needed a point, and without one its slot push would answer data where the
// interpreter dispatches (NUR361).
func (es *EmitState) guardDroppedReads(owner *fnUnitRec, points []deoptPoint) {
	for _, d := range points {
		if d.id != "" && d.slot < 0 && es.siteGuardable(owner, d.id, owner.frag.events, nil) {
			es.guardReadSites(owner, d.id)
		}
	}
}

// collectingWordBefore reports the position of the word before body token
// tok when that word collects the token forward: the word's own event takes
// the value the token makes (`print [j]`, `size [j]`), or no event makes the
// token at all — the word runs it inline as a body (`var [[] [j]]`), where a
// list literal is made at its own token. A word whose event takes something
// else (`do b drop [j]` — drop takes the run's value) is its own statement.
// A deopt point on a read inside the token then begins its statement at the
// word (deoptStatementStart, NUR361).
func collectingWordBefore(body []core.Value, events []EmitEvent, tok int) (core.SrcPos, bool) {
	if tok <= 0 || !core.IsWord(body[tok-1]) {
		return core.SrcPos{}, false
	}
	w, t := body[tok-1].Pos(), body[tok].Pos()
	made := map[int]bool{}
	for i := range events {
		if eventPos(events[i]) == t {
			made[events[i].seq] = true
		}
	}
	if len(made) == 0 {
		return w, w.Row > 0
	}
	for i := range events {
		if eventPos(events[i]) != w {
			continue
		}
		takes := false
		forEachOperand(&events[i], func(op EmitOperand) { takes = takes || (op.kind == opEvent && made[op.idx]) })
		if takes {
			return w, true
		}
	}
	return w, false
}
