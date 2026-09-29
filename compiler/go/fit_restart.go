package compiler

import (
	"sort"

	core "github.com/boru-lang/boru/core/go"
)

// The FORWARD-FIT island of a poly (NUR357). The pass collects a gradual
// operand written after a word forward wherever its carrier is admitted —
// an Any result, a member read of an opaque map — but the interpreter's
// forward collection takes the run's value only where it fits the slot
// (core.ForwardFit): `{b:2} keys m.f` over an f of 0 is keys over the {b:2}
// beneath it, then the 0, interpreted; the compiled poly took the 0 and
// raised keys' no-match. A read with a deopt point of its own tests its
// value where its statement begins (DeoptSpec.Fits); every other such
// operand is tested at the poly itself, and a miss runs the statement again
// on the interpreter from its first token — a landing's statement island
// (landing_restart.go), under the same conditions, the stop being the poly.

// planFitRestarts plans the root's poly statement islands: one per root
// poly event, outside any loop, that collected a gradual operand forward
// (EmitState.fwdFitsAt), whose statement an island can take over — nothing
// deferred past its start, every event the compiled code ran in it before
// the poly re-runnable or written as its value (fitReruns), and the
// residual's entries before it seated where the root keeps them
// (rootPreStart). In a program the pass ended at a terminal trap — whose
// proof the pass's collection may not be (`{b:2} keys y add 1`) — a poly
// before the trap plans one over the stack the pass told at its statement's
// start, the residual being dropped, and the island runs the statement and
// the program after it, the trap's included.
func (es *EmitState) planFitRestarts(lw *lowerer, residual []core.Value) {
	if len(es.rootBody) == 0 || len(es.fwdFitsAt) == 0 {
		return
	}
	trapped := es.trapAt != 0
	tree := rootTreeEvents(es.frames[0], false)
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	for _, seq := range sortedSeqs(tree) {
		at := tree[seq]
		if len(es.fwdFitsAt[seq]) == 0 || at.inLoop || !fitStop(at.ev) || (trapped && seq > es.trapAt) {
			continue
		}
		tok := statementToken(es.rootBody, stopPos(at.ev))
		if tok < 0 {
			continue
		}
		// Past the literal defs the statement opens with, as a landing's
		// island takes it over (literalDefsBefore, toldAfter).
		tok = es.toldAfter(literalDefsBefore(tree, es.rootBody, tok, stopPos(at.ev)), stopPos(at.ev))
		d := deoptPoint{seq: seq, slot: -1, start: statementStart(es.rootBody, tok), token: tok}
		if _, told := es.stackAtStart(tok); d.start.Row == 0 || (!told && (trapped || es.deoptDeferred(es.units[0], rec, &d, -1))) {
			continue
		}
		substs, reruns := es.fitReruns(tree, seq, es.rootBody, tok)
		if !reruns {
			continue
		}
		srcs, held, slots, ok := es.rootPreStart(lw, tree, residual, tok, d.start, statementFirstSeq(tree, seq, d.start))
		if !ok {
			continue
		}
		if trapped {
			// The stack beneath and the runs the island writes are read
			// where the compiled code left them, never dropped
			// (planCountRestarts' trap arm).
			for _, slot := range slots {
				delete(lw.dead, slot.seq)
			}
			for _, sp := range substs {
				delete(lw.dead, sp.seq)
			}
		}
		if lw.fitRestarts == nil {
			lw.fitRestarts = map[int]*landingRestart{}
		}
		lw.fitRestarts[seq] = &landingRestart{token: tok, start: d.start, depth: -1, srcs: srcs, held: held, heldAt: slots, substs: substs}
	}
}

// planUnitFitRestarts is planFitRestarts in a fn unit (planUnitRestarts'
// rules): one restart point per poly of the unit's root frame, outside any
// loop, that collected a gradual operand forward, where the unit's island
// can take its statement over. It reports how many it added.
func (es *EmitState) planUnitFitRestarts(u *emitUnit, rec *fnUnitRec) int {
	if (rec.closure && (rec.lambdaUnit || rec.storedRefUnit)) || rec.frag == nil || len(es.fwdFitsAt) == 0 {
		return 0
	}
	tree := rootTreeEvents(rec.frag.events, false)
	n := 0
	for _, seq := range sortedSeqs(tree) {
		te := tree[seq]
		if len(es.fwdFitsAt[seq]) == 0 || te.inLoop || !fitStop(te.ev) {
			continue
		}
		tok := statementToken(rec.body, stopPos(te.ev))
		if tok < 0 {
			continue
		}
		// Past the defs the statement opens with, as a landing's island
		// takes it over (defsBefore): `def y m.a {b:2} keys y`.
		from := tok
		tok = defsBefore(tree, rec.body, tok, stopPos(te.ev))
		d := deoptPoint{seq: seq, slot: -1, start: statementStart(rec.body, tok), token: tok, restart: true, fit: true}
		d.leftovers, d.defBound = defLeftovers(tree, rec.body, from, tok)
		if d.start.Row == 0 || es.deoptDeferred(u, rec, &d, -1) || outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start), d.leftovers) {
			continue
		}
		substs, ok := es.fitReruns(tree, seq, rec.body, tok)
		if !ok {
			continue
		}
		d.substs = substs
		rec.deopts = append(rec.deopts, d)
		n++
	}
	return n
}

// dropFitPoints is points without the polys' forward-fit islands.
func dropFitPoints(points []deoptPoint) []deoptPoint {
	out := points[:0]
	for _, d := range points {
		if !d.fit {
			out = append(out, d)
		}
	}
	return out
}

// fitReruns plans the runs a poly's statement island writes as their values
// (restartSubsts) when the poly event seq stops it, the statement beginning
// at body token tok: every event recorded from the statement's first event
// up to the poly, which has not run — a member read the island reads again.
func (es *EmitState) fitReruns(tree map[int]treeEvent, seq int, body []core.Value, tok int) ([]substPlan, bool) {
	first := statementFirstSeq(tree, seq, statementStart(body, tok))
	var pending []int
	for s := range tree {
		if s >= first && s < seq {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	return es.restartSubsts(tree, body, tok, pending)
}

// polyFit is the forward-fit island of the poly event seq (PolyRef.Fit),
// when the walk seated its planned statement island where it can take the
// statement over, with every value it writes held where the island reads
// it; nil otherwise, and the poly keeps the match it had.
func (lw *lowerer) polyFit(seq int) *PolyFit {
	r := lw.fitRestarts[seq]
	if !r.seated() || !lw.heldIntact(r) {
		return nil
	}
	substs, ok := lw.restartSubstSrcs(r, EmitOperand{}, -1)
	if !ok {
		return nil
	}
	fits := lw.es.fwdFitsAt[seq]
	at := make([]int, 0, len(fits))
	for k := range fits {
		at = append(at, k)
	}
	sort.Ints(at)
	f := &PolyFit{At: at, Restart: &StmtIsland{
		Island: lw.landingBody[r.token:], Depth: r.depth, RetPC: -1, Root: lw.landingRoot, PrefixSrc: r.srcs, Substs: substs,
	}}
	for _, k := range at {
		f.Fits = append(f.Fits, fits[k])
	}
	lw.fitIslands = append(lw.fitIslands, f.Restart)
	return f
}

// fitStop reports whether ev is a call a forward-fit island can stop at: a
// poly re-match (PolyRef.Fit), or a committed user call that is not routed
// through a region descriptor nor a user poly (CallFits).
func fitStop(ev *EmitEvent) bool {
	switch ev.kind {
	case evCall:
		return ev.call.poly
	case evCallUser:
		return ev.uc.poly == nil && !ev.uc.generic && ev.uc.unit >= 0
	}
	return false
}

// seatCallFit seats the forward-fit island of the user call event seq
// (fitIsland) on the emission target's CallFits entry at the pc of the
// call about to be emitted — a tail call takes none — or returns the
// decline of a call that needs one and has none.
func (lw *lowerer) seatCallFit(seq int, tail bool) string {
	if tail {
		return lw.fitUnserved(seq)
	}
	f, why := lw.fitIsland(seq)
	if f == nil {
		return why
	}
	if *lw.callFits == nil {
		*lw.callFits = map[int]*PolyFit{}
	}
	(*lw.callFits)[len(*lw.code)] = f
	return ""
}

// fitIsland is the call event seq's forward-fit island (polyFit), or, when
// none is seated, the decline of a call that needs one (fitUnserved).
func (lw *lowerer) fitIsland(seq int) (*PolyFit, string) {
	if f := lw.polyFit(seq); f != nil {
		return f, ""
	}
	return nil, lw.fitUnserved(seq)
}

// fitUnserved is the decline of the call event seq when a gradual operand
// it collected forward may miss the very slot the pass collected it into
// while the stack beneath could fill that candidate instead (a Chosen
// forward fit), and no island serves it — neither the call's own nor its
// read's point (fitServed): the compiled call would answer over a window
// the interpreter's plan never assembles (NUR357). A viable alternative
// only among the other candidates is no decline: the island takes it where
// one is seated. Empty otherwise.
func (lw *lowerer) fitUnserved(seq int) string {
	if lw.fitServed[seq] {
		return ""
	}
	for _, fits := range lw.es.fwdFitsAt[seq] {
		for _, f := range fits {
			if f.Chosen {
				return "a gradual operand collected forward where the interpreter's collection may stop and draw from the stack, with no statement island to take it over (NUR357)"
			}
		}
	}
	return ""
}

// noteFitServed marks the call event a read's point tests the fits of.
func (lw *lowerer) noteFitServed(d deoptPoint) {
	if len(d.fits) == 0 {
		return
	}
	if lw.fitServed == nil {
		lw.fitServed = map[int]bool{}
	}
	lw.fitServed[d.fitConsumer] = true
}
