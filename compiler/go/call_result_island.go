package compiler

import (
	"sort"

	core "github.com/boru-lang/boru/core/go"
)

// The CALL-RESULT island (NUR334). The interpreter splices a fn frame's
// results back on its tape where the call stood and steps them there. A
// plain value steps as itself, which is what the compiled caller keeps; a
// tape-coupled one does not — a `/v` read of a `word` value, a unit's live
// read after a computed keep-defs body (kept_live_deopt.go), is a splice
// marker, which fires where the call stood: `9 f …` is `[9 1 2]` over a
// splice of `1 2`, `[f …]` `[[1 2]]`, `4 5 f …` over a splice of `add` is
// `[9]`. The compiled caller would keep the marker as data, so the VM's
// screen deferred ("tape-coupled deopt result") everywhere but at the
// program's end (rootEndResults).
//
// A call whose callee may leave such a result (unitMayCouple) plans its
// statement's island with the call's run of tokens — its word and the
// arguments it took (callRun) — written as the results it left
// (RestartResults), as a do's count island writes its run: the interpreter
// then steps them exactly where its own tape holds them, over the frame
// beneath the statement, and runs the rest of the unit or of the program
// after them. The VM takes the island at the call's return when the results
// are tape-coupled (CompiledFn.CallResults); plain results cost that test.

// unitMayCouple reports whether the fn unit at index unit may return a
// tape-coupled result to its caller: it reads a name live after a computed
// keep-defs body — its island's residual is the interpreter's, a `/v` read
// of a splice among it — or calls a unit that may (a tail call returns that
// unit's results as its own, and a call's own result island leaves the
// interpreter's residual too).
func (es *EmitState) unitMayCouple(unit int, seen map[int]bool) bool {
	if unit < 0 || unit >= len(es.fnRecs) || seen[unit] {
		return false
	}
	rec := es.fnRecs[unit]
	if rec == nil || rec.frag == nil {
		return false
	}
	seen[unit] = true
	if len(es.keptLiveSeqs(rec.frag.events)) > 0 {
		return true
	}
	found := false
	walkEvents(rec.frag.events, func(ev *EmitEvent) {
		found = found || (ev.kind == evCallUser && es.unitMayCouple(ev.uc.unit, seen))
	})
	return found
}

// callResultWanted reports whether event ev is a committed, non-tail user
// call whose callee may return a tape-coupled result (unitMayCouple).
func (es *EmitState) callResultWanted(ev *EmitEvent) bool {
	return ev.kind == evCallUser && !ev.uc.tail && !ev.uc.generic && ev.uc.poly == nil && ev.uc.unit >= 0 &&
		es.unitMayCouple(ev.uc.unit, map[int]bool{})
}

// callResultPoint is the call-result island of the user call event seq in
// body: its statement's first token and the runs the island writes — each
// earlier event's paren or call run (restartSubsts), then the call's own run
// as the results it left. Only scalar literals, or runs the island writes,
// may stand before the call's run on its level (inertBefore): written as
// values, the results are no collection barrier.
func (es *EmitState) callResultPoint(tree map[int]treeEvent, seq int, body []core.Value) (int, []substPlan, bool) {
	ev := tree[seq].ev
	tok := statementToken(body, eventPos(*ev))
	if tok < 0 {
		return 0, nil, false
	}
	run, span, ok := es.callRun(tree, ev, body, tok)
	if !ok {
		return 0, nil, false
	}
	first := statementFirstSeq(tree, seq, statementStart(body, tok))
	var pending []int
	for s, te := range tree {
		if s < first || s >= seq || inSpan(body, eventPos(*te.ev), run, span) {
			continue
		}
		if p := eventPos(*te.ev); p.Row == 0 || posAfter(p, eventPos(*ev)) {
			// Written after the call and run before it — the read after a
			// lazy list literal the interpreter makes before the list's
			// elements (`[(g) f …] c.n`): the island would make it again
			// after the effects the compiled code ran (NUR334).
			return 0, nil, false
		}
		pending = append(pending, s)
	}
	sort.Ints(pending)
	substs, ok := es.restartSubsts(tree, body, tok, pending)
	if !ok || !inertBefore(body, tok, run, substs...) {
		return 0, nil, false
	}
	return tok, append(substs, substPlan{path: run, span: span, seq: seq, results: true}), true
}

// planCallResultRestarts plans the root's call-result islands: one per root
// user call outside any loop whose callee may return a tape-coupled result,
// whose statement an island can take over (callResultPoint), with the
// residual's entries before it seated where the root keeps them
// (rootPreStart). A program the pass ended at a terminal trap plans none.
func (es *EmitState) planCallResultRestarts(lw *lowerer, residual []core.Value) {
	if len(es.rootBody) == 0 || es.trapAt != 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	for _, seq := range sortedSeqs(tree) {
		te := tree[seq]
		if te.inLoop || !es.callResultWanted(te.ev) {
			continue
		}
		// Where the pass told the stack at the statement's start the island
		// seats exactly that; else no operand may be deferred past it.
		tok, substs, ok := es.callResultPoint(tree, seq, es.rootBody)
		d := deoptPoint{seq: seq, slot: -1, start: statementStart(es.rootBody, tok), token: tok}
		_, told := es.stackAtStart(tok)
		srcs, held, slots, seated := es.rootPreStart(lw, tree, residual, tok, d.start, statementFirstSeq(tree, seq, d.start))
		if !ok || !seated || d.start.Row == 0 || (!told && es.deoptDeferred(es.units[0], rec, &d, -1)) {
			continue
		}
		if lw.callResultRestarts == nil {
			lw.callResultRestarts = map[int]*landingRestart{}
		}
		lw.callResultRestarts[seq] = &landingRestart{token: tok, start: d.start, depth: -1, srcs: srcs, held: held, heldAt: slots, substs: substs}
	}
}

// planUnitCallResults is planCallResultRestarts in a fn unit
// (planUnitRestarts' rules). It reports how many points it added.
func (es *EmitState) planUnitCallResults(u *emitUnit, rec *fnUnitRec) int {
	if (rec.closure && (rec.lambdaUnit || rec.storedRefUnit)) || rec.frag == nil {
		return 0
	}
	tree := rootTreeEvents(rec.frag.events, false)
	n := 0
	for _, seq := range sortedSeqs(tree) {
		te := tree[seq]
		if te.inLoop || !es.callResultWanted(te.ev) {
			continue
		}
		tok, substs, ok := es.callResultPoint(tree, seq, rec.body)
		if !ok || tailRun(rec.body, substs[len(substs)-1]) {
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: rec.body[tok].Pos(), token: tok, restart: true, callResult: true, substs: substs}
		if d.start.Row > 0 && !es.deoptDeferred(u, rec, &d, -1) && !outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start), nil) {
			rec.deopts = append(rec.deopts, d)
			n++
		}
	}
	return n
}

// tailRun reports whether the call run sp stands in a fn body's tail: the
// last token of its level, and each paren holding it the last of its own,
// up to the body's top. The interpreter may eliminate such a call's frame
// (Engine.probeTailCall), its results then going back to the unit's caller
// past the unit's own return check, where an island in this unit would step
// them. A list literal holding the run is evaluated on its own tape, whose
// calls nest.
func tailRun(body []core.Value, sp substPlan) bool {
	levels := [][]core.Value{body}
	for _, at := range sp.path[:len(sp.path)-1] {
		inner, _ := nestedToks(levels[len(levels)-1][at])
		levels = append(levels, inner)
	}
	end := sp.path[len(sp.path)-1] + sp.span
	for i := len(levels) - 1; i >= 0; i-- {
		if end != len(levels[i]) {
			return false
		}
		if i > 0 {
			holder := levels[i-1][sp.path[i-1]]
			if !core.IsParenExpr(holder) {
				return false
			}
			end = sp.path[i-1] + 1
		}
	}
	return true
}

// dropCallResultPoints is points without the call-result islands.
func dropCallResultPoints(points []deoptPoint) []deoptPoint {
	out := points[:0]
	for _, d := range points {
		if !d.callResult {
			out = append(out, d)
		}
	}
	return out
}

// seatCallResult seats the call-result island of the user call event seq on
// the emission target's CallResults entry at the pc of the call about to be
// emitted, when the walk seated its planned island where it can take the
// statement over with every value it writes held where the island reads it.
func (lw *lowerer) seatCallResult(seq int) {
	r := lw.callResultRestarts[seq]
	substs, ok := lw.restartSubstSrcs(r, EmitOperand{}, -1)
	if !ok || !r.seated() || !lw.heldIntact(r) {
		return
	}
	is := &StmtIsland{Island: lw.landingBody[r.token:], Depth: r.depth, RetPC: -1, Root: lw.landingRoot, PrefixSrc: r.srcs, Substs: substs}
	if *lw.callResults == nil {
		*lw.callResults = map[int]*StmtIsland{}
	}
	(*lw.callResults)[len(*lw.code)] = is
	lw.fitIslands = append(lw.fitIslands, is)
}
