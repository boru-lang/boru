package compiler

import (
	"sort"

	core "github.com/boru-lang/boru/core/go"
)

// The STATEMENT ISLAND of a root landing (NUR242, NUR219). A landed fn value
// whose re-step may CAPTURE the word after it through a `/q` slot has a
// compiled answer only where the lowering proved one: the sealed claim
// target, the island from the word on (landingIsland — the word at the
// landing's own depth, nothing beneath), the skip past the word's call and
// the apply over it. A landing nested in a list literal or a branch arm,
// over values the compiled stack pushes later, or before a wider residual
// has none, and its claim deferred: `[(m.f y)]` is `[[y y]]` interpreted
// and was the skip's count failure compiled; `if true [(m.f add 1 2)] [0]`
// and `7 m.f true` deferred outright.
//
// The interpreter can run the whole STATEMENT holding the landing again, from
// its first token, whenever nothing the compiled code did inside it before
// the landing is an effect a second run repeats — member reads only, or a
// call that opens a paren whose value the compiled code still holds: the
// island writes the value in the paren's place (restartSubsts), so the call
// is not repeated — and the compiled stack at the statement's start holds
// exactly what the interpreter's does there (no operand of the statement or
// a later one was deferred past it, NUR207's accounting). The landing then
// restarts it: the frame region beneath the statement as the resolved
// prefix, the program's tokens from the statement on, and the island's
// residual is the program's.

// landingRestart is one planned statement island: the program token the
// statement begins at, its position, and the compiled stack's depth there
// (seated by the walk; -1 until then).
type landingRestart struct {
	token int
	start core.SrcPos
	depth int
	// srcs are the program residual's entries before the statement, where
	// the compiled root keeps each (rootPreStart): the island seats them.
	srcs []RestartSrc
	// held is how many of them the compiled stack holds beneath the
	// statement (-1 in a unit, which plans none): the walk's depth must be
	// exactly that.
	held int
	// unseatable marks a unit's island whose frame, at the statement's
	// start, holds unnamed params the walk cannot place (deoptPrefix).
	unseatable bool
	// substs are the parens the island writes the compiled code's values in
	// place of (restartSubsts).
	substs []substPlan
	// first is the loops' first-iteration check the island takes at run
	// time (firstIterGuard); empty outside loops.
	first []RestartFirst
}

// substPlan is one run of tokens a statement island writes a value in place
// of (RestartSubst): its first token's path (tokenPath), how many tokens it
// holds there — a paren or a bare word is one, a `do` and its body list two
// (doBody) — and the call event whose one result it is.
type substPlan struct {
	path []int
	span int
	seq  int
	// results marks the stop's own call, written as the run it left
	// (RestartResults) rather than a value it holds.
	results bool
	// none marks a call run that left nothing (callRun over an effect,
	// `print "a"`): the island writes no token in its place (RestartNone).
	none bool
	// run marks a call run (callRun): its value, placed by the interpreter,
	// must not dispatch where the island writes it (RestartSubst.Placed).
	run bool
}

// covers reports whether path lies in the run of tokens p replaces: it
// passes through one of the run's tokens, or ends at one.
func (p substPlan) covers(path []int) bool {
	n := len(p.path)
	if n == 0 || len(path) < n {
		return false
	}
	for i := 0; i < n-1; i++ {
		if path[i] != p.path[i] {
			return false
		}
	}
	at := path[n-1]
	return at >= p.path[n-1] && at < p.path[n-1]+p.span
}

// holds reports whether c's whole run lies inside p's, on the same level,
// p's the longer: a call run over a paren it took off the stack (`(mk)
// print/s`). Writing p writes c's tokens too.
func (p substPlan) holds(c substPlan) bool {
	n := len(p.path)
	if n == 0 || len(c.path) != n || c.span >= p.span {
		return false
	}
	for i := 0; i < n-1; i++ {
		if c.path[i] != p.path[i] {
			return false
		}
	}
	return c.path[n-1] >= p.path[n-1] && c.path[n-1]+c.span <= p.path[n-1]+p.span
}

// seated reports whether the walk seated r where the island can take the
// statement over: its depth measured, its unit frame placeable and, at the
// root, the compiled stack there exactly the residual's results the plan
// counted (rootPreStart).
func (r *landingRestart) seated() bool {
	return r != nil && r.depth >= 0 && !r.unseatable && (r.held < 0 || r.depth == r.held)
}

// planLandingRestarts plans the program root's landings and shaped applies a
// statement island can take over (see the file comment): a landing with a
// noted word after its value, or a paren's shaped method apply (whose
// method may be data at run time — the paren then places the values), its
// event in the root's tree outside any loop, whose statement begins at a
// program token with nothing deferred past it, and before which the
// statement ran member reads only.
func (es *EmitState) planLandingRestarts(lw *lowerer, residual []core.Value) {
	if len(es.rootBody) == 0 || es.trapAt != 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	seqs := make([]int, 0, len(es.landingAfter))
	for seq, te := range tree {
		_, landed := es.landingAfter[seq]
		if (landed && (es.landingWord[seq].Name != "" || es.landingOwn[seq].beneath || es.landingCollects(seq))) || (te.ev.kind == evCall && te.ev.call.dynMethod != nil) {
			seqs = append(seqs, seq)
		}
	}
	sort.Ints(seqs)
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	for _, seq := range seqs {
		at := tree[seq]
		tok := statementToken(es.rootBody, eventPos(*at.ev))
		if tok < 0 {
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: es.rootBody[tok].Pos(), token: tok}
		if d.start.Row == 0 || es.deoptDeferred(es.units[0], rec, &d, -1) {
			continue
		}
		substs, first, reruns := es.restartReruns(tree, at, seq, es.rootBody, tok)
		if !reruns {
			continue
		}
		srcs, held, ok := es.rootPreStart(lw, residual, d.start, statementFirstSeq(tree, seq, d.start))
		if !ok {
			continue
		}
		if lw.landingRestarts == nil {
			lw.landingRestarts = map[int]*landingRestart{}
		}
		lw.landingRestarts[seq] = &landingRestart{token: tok, start: d.start, depth: -1, srcs: srcs, held: held, substs: substs, first: first}
	}
}

// The guards of a branch (NUR292) a statement island may take over: the
// condition's (__condguard, before the jump) and each value arm's
// (__codeguard, on the path that takes it). guardKey keys one with its
// branch event's seq.
const (
	guardCond = iota + 1
	guardThen
	guardElse
)

// guardKey keys the planned island of the guard of kind on branch event seq.
func guardKey(seq, kind int) int { return seq*4 + kind }

// guardCand is one guarded operand of a branch: the branch event's seq, the
// guard's kind and the operand it guards.
type guardCand struct {
	seq, kind int
	op        EmitOperand
}

// branchGuards lists the guarded operands of the tree's branches, in seq
// order.
func branchGuards(tree map[int]treeEvent) []guardCand {
	var out []guardCand
	for seq, te := range tree {
		if te.ev.kind != evBranch {
			continue
		}
		br := te.ev.br
		if br.condGuard {
			out = append(out, guardCand{seq: seq, kind: guardCond, op: br.cond})
		}
		if br.thenGuard {
			out = append(out, guardCand{seq: seq, kind: guardThen, op: br.thenVal})
		}
		if br.elsGuard {
			out = append(out, guardCand{seq: seq, kind: guardElse, op: br.elsVal})
		}
	}
	sort.Slice(out, func(i, j int) bool { return guardKey(out[i].seq, out[i].kind) < guardKey(out[j].seq, out[j].kind) })
	return out
}

// planGuardRestarts plans the root's branch guards a statement island can
// take over (NUR292). A guard defers on a computed condition or value arm
// that is a list at run time, which the interpreter runs as code — a
// condition inline, over the values beneath the `if`, an arm spliced in
// parens. The statement then runs again on the interpreter from its first
// token, the guarded value written in place of the paren that computed it,
// under the landing's conditions (planLandingRestarts): the branch outside
// any loop, nothing deferred past the statement's start, and every event
// the compiled code ran in the statement before the guard re-runnable or
// inside a paren the island substitutes (guardReruns).
func (es *EmitState) planGuardRestarts(lw *lowerer, residual []core.Value) {
	if len(es.rootBody) == 0 || es.trapAt != 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	for _, g := range branchGuards(tree) {
		d, ok := es.guardPoint(es.units[0], rec, tree, g)
		if !ok {
			continue
		}
		srcs, held, ok := es.rootPreStart(lw, residual, d.start, statementFirstSeq(tree, g.seq, d.start))
		if !ok {
			continue
		}
		if lw.guardRestarts == nil {
			lw.guardRestarts = map[int]*landingRestart{}
		}
		lw.guardRestarts[guardKey(g.seq, g.kind)] = &landingRestart{token: d.token, start: d.start, depth: -1, srcs: srcs, held: held, substs: d.substs, first: d.first}
	}
}

// guardPoint is guard g's statement island in rec's body, when one can take
// the statement over (planGuardRestarts' conditions but the prefix). The
// statement is the `if` word's (BranchRecord.CondCheckPos): the branch's
// own position is its condition value's, which an earlier statement may
// have written.
func (es *EmitState) guardPoint(u *emitUnit, rec *fnUnitRec, tree map[int]treeEvent, g guardCand) (deoptPoint, bool) {
	tok := statementToken(rec.body, tree[g.seq].ev.br.condCheckPos)
	if tok < 0 {
		return deoptPoint{}, false
	}
	d := deoptPoint{seq: g.seq, slot: -1, start: rec.body[tok].Pos(), token: tok, restart: true, guard: g.kind}
	if d.start.Row == 0 || es.deoptDeferred(u, rec, &d, -1) {
		return deoptPoint{}, false
	}
	substs, ok := es.guardReruns(tree, g.seq, rec.body, tok)
	if !ok {
		return deoptPoint{}, false
	}
	// A guard inside loops restarts only over counted ones, on their first
	// iteration, and substitutes no paren inside one (restartReruns' rule).
	if te := tree[g.seq]; te.inLoop {
		first, counted := es.firstIterGuard(te.loops)
		if !counted || !loopFree(tree, substs) {
			return deoptPoint{}, false
		}
		d.first = first
	}
	d.substs = substs
	return d, true
}

// planCountRestarts plans the statement island of each root `do` whose run's
// count the program's seat may miss (SigRef.Count, NUR222): a catch-latched
// value-less body seats the one Error a caught raise leaves, a later word
// takes that as its operand, and a clean run leaves nothing — `1 do [(1 add
// 1) drop] drop` is `[]` interpreted, the drop taking the 1. The stop is the
// do's own call. The events before it in its statement run again or are
// written as their values (restartSubsts), and the do word and its literal
// body list are written as the run the call left (RestartResults): the
// interpreter splices a do's results back in its place and steps them. The
// body is never run twice. A do inside a loop plans none.
func (es *EmitState) planCountRestarts(lw *lowerer, residual []core.Value) {
	es.notePhantomConsumers()
	if len(es.rootBody) == 0 || es.trapAt != 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	for seq, te := range tree {
		if te.inLoop || !es.countSeat(seq) {
			continue
		}
		tok, substs, ok := es.countPoint(tree, seq, es.rootBody)
		if !ok {
			continue
		}
		start := es.rootBody[tok].Pos()
		srcs, held, ok := es.rootPreStart(lw, residual, start, statementFirstSeq(tree, seq, start))
		if !ok {
			continue
		}
		if lw.countRestarts == nil {
			lw.countRestarts = map[int]*landingRestart{}
		}
		lw.countRestarts[seq] = &landingRestart{token: tok, start: start, depth: -1, srcs: srcs, held: held, substs: substs}
	}
}

// notePhantomConsumers marks each catch-latched value-less `do` whose
// phantom Error a CALL anywhere in the program takes as an operand
// (EmitState.phantomConsumed, NUR222): its call checks the run's count
// (SigRef.CountCheck) — the root's, the fragments' and every unit's. A
// call's operands are a fixed count; a loop's, a branch's or a residual's
// seat absorbs the region (`for 3 [do [def t 5]]`).
func (es *EmitState) notePhantomConsumers() {
	es.notePhantomConsumersIn(es.frames[0])
	for _, rec := range es.fnRecs {
		if rec != nil && rec.frag != nil {
			es.notePhantomConsumersIn(rec.frag.events)
		}
	}
}

// notePhantomConsumersIn is notePhantomConsumers over one event list and
// the fragments beneath it: a unit's own, which it plans as it closes
// (planUnitRestarts), long before the root's.
func (es *EmitState) notePhantomConsumersIn(events []EmitEvent) {
	for i := range events {
		if k := events[i].kind; k == evCall || k == evCallUser {
			forEachOperand(&events[i], func(op EmitOperand) {
				if op.kind == opEvent && es.eventInfo[op.idx].catchPhantom {
					if es.phantomConsumed == nil {
						es.phantomConsumed = map[int]bool{}
					}
					es.phantomConsumed[op.idx] = true
				}
			})
		}
		for _, f := range childFragments(&events[i]) {
			if f != nil {
				es.notePhantomConsumersIn(f.events)
			}
		}
	}
}

// countPoint is the count island of the do event seq in body: its
// statement's first token and the runs the island writes — each earlier
// event's paren (restartSubsts), then the do word and its literal body list
// as the do's own run. The body must be the list written right after the
// word (its closure), and only scalar literals may stand before the word on
// its level: written as values, the two tokens are no collection barrier.
// An error call over a computed handler (NUR300) writes its run over four
// tokens — the do before it, the do's body, the word and the handler
// (handlerRun) — and the events inside them plan nothing of their own.
func (es *EmitState) countPoint(tree map[int]treeEvent, seq int, body []core.Value) (int, []substPlan, bool) {
	ev := tree[seq].ev
	c := &ev.call
	if ev.kind != evCall || !(c.word == "do" && len(c.ops) == 1 || c.word == "error" && len(c.ops) == 2) {
		return 0, nil, false
	}
	tok := statementToken(body, c.pos)
	path := tokenPath(body, c.pos)
	if tok < 0 || len(path) == 0 {
		return 0, nil, false
	}
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	at := path[len(path)-1]
	if at+1 >= len(toks) || !core.IsWord(toks[at]) || toks[at].Pos() != c.pos {
		return 0, nil, false
	}
	// The run the island writes: the do and its body, or — for a computed
	// error handler (NUR300) — the do before the word, its body, the word
	// and the handler, since the handler took the do's caught value.
	span := 2
	if c.word == "error" {
		if !es.handlerRun(tree, seq, toks, at) {
			return 0, nil, false
		}
		path, span = append(append([]int(nil), path[:len(path)-1]...), at-2), 4
	} else if !es.doBodyAfter(tree, seq, toks, at) {
		return 0, nil, false
	}
	first := statementFirstSeq(tree, seq, body[tok].Pos())
	var pending []int
	for s, te := range tree {
		if s >= first && s < seq && !inSpan(body, eventPos(*te.ev), path, span) {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	substs, ok := es.restartSubsts(tree, body, tok, pending)
	if !ok || !inertBefore(body, tok, path, substs...) {
		return 0, nil, false
	}
	return tok, append(substs, substPlan{path: path, span: span, seq: seq, results: true}), true
}

// handlerRun reports whether the error call at toks[at] took its computed
// handler from the token right after the word (its one argument site) and
// its caught value from the do written right before it over a literal body
// (`do [raise oops 'x'] error (mk)`, NUR300): the four tokens are one run.
func (es *EmitState) handlerRun(tree map[int]treeEvent, seq int, toks []core.Value, at int) bool {
	c := &tree[seq].ev.call
	sites := es.argSites[seq]
	if at < 2 || len(sites) != 2 || c.ops[1].kind != opEvent {
		return false
	}
	do, in := tree[c.ops[1].idx]
	if !in || do.ev.kind != evCall || do.ev.call.word != "do" || len(do.ev.call.ops) != 1 || do.ev.call.ops[0].kind != opClosure {
		return false
	}
	lit := toks[at-1]
	if !core.IsWord(toks[at-2]) || toks[at-2].Pos() != do.ev.call.pos || !lit.Eval || lit.Quoted || !lit.Parent.Equal(core.TList) {
		return false
	}
	q := sites[0].pos
	if te, in := tree[sites[0].seq]; sites[0].seq >= 0 && in {
		q = eventPos(*te.ev)
	} else if sites[0].seq >= 0 {
		return false
	}
	return bodyTokenContaining(toks, q) == at+1
}

// inSpan reports whether position p stands inside the span tokens at path's
// level, from path's last index on: an event there ran inside the run the
// island writes, so no plan of its own writes it.
func inSpan(body []core.Value, p core.SrcPos, path []int, span int) bool {
	q := tokenPath(body, p)
	n := len(path)
	if len(q) < n {
		return false
	}
	for i := 0; i < n-1; i++ {
		if q[i] != path[i] {
			return false
		}
	}
	return q[n-1] >= path[n-1] && q[n-1] < path[n-1]+span
}

// doBodyAfter reports whether the do at toks[at] took its body from the
// token written right after it: a literal list, its closure (NUR222's caught
// body), or a computed body — a read, a paren — whose one argument site
// (argSites) is that token (`(do b) add 1`, NUR282's single seat).
func (es *EmitState) doBodyAfter(tree map[int]treeEvent, seq int, toks []core.Value, at int) bool {
	ev := tree[seq].ev
	if ev.call.ops[0].kind == opClosure {
		return toks[at+1].Eval && !toks[at+1].Quoted && toks[at+1].Parent.Equal(core.TList)
	}
	sites := es.argSites[seq]
	if len(sites) != 1 {
		return false
	}
	q := sites[0].pos
	if te, in := tree[sites[0].seq]; sites[0].seq >= 0 && in {
		q = eventPos(*te.ev)
	} else if sites[0].seq >= 0 {
		return false
	}
	return bodyTokenContaining(toks, q) == at+1
}

// countSeat reports whether event seq's call checks its run's count, so a
// miss may take the do's count island: a caught body's phantom a call
// consumes (NUR222), or a computed body's run a single-value seat takes
// (eventFlags.dynBodyOne, or a region Finalize may still demote to one —
// dynRegionCheckable; NUR282).
func (es *EmitState) countSeat(seq int) bool {
	fi := es.eventInfo[seq]
	return es.phantomConsumed[seq] || fi.dynBodyOne || dynRegionCheckable(fi)
}

// restartSubsts plans the runs of tokens a statement island writes values in
// place of, so that it may run the statement (from body token tok on) again:
// every event of pending — the compiled code's run in the statement before
// the stop — that is no re-runnable read (restartRead) must have run inside
// a paren another such event's call opens (parenOf), or a `do` over one call
// (doBody), binding nothing there, and the island writes each outermost one
// as the value its call left: the run is the compiled code's, never
// repeated. The plans are in token order (pathLess). ok is false otherwise.
func (es *EmitState) restartSubsts(tree map[int]treeEvent, body []core.Value, tok int, pending []int) ([]substPlan, bool) {
	var cands []substPlan
	for _, s := range pending {
		ev := tree[s].ev
		if restartRead(ev) {
			continue
		}
		if path, ok := parenOf(ev, body, tok); ok && (!wordAt(body, path) || inertBefore(body, tok, path, cands...)) {
			cands = append(cands, substPlan{path: path, span: 1, seq: s})
		} else if sub, whole, ok := es.doBody(ev, body, tok); ok && !whole && inertBefore(body, tok, sub, cands...) {
			cands = append(cands, substPlan{path: sub, span: 2, seq: s})
		} else if run, span, ok := es.callRun(tree, ev, body, tok); ok && inertBefore(body, tok, run, cands...) {
			_, nout := callShape(ev)
			cands = append(cands, substPlan{path: run, span: span, seq: s, none: nout == 0, run: true})
		}
	}
	var kept []substPlan
	for i, c := range cands {
		outer := false
		for j, o := range cands {
			outer = outer || (i != j && (len(o.path) < len(c.path) && o.covers(c.path) || o.holds(c)))
		}
		if !outer {
			kept = append(kept, c)
		}
	}
	for _, s := range pending {
		ev := tree[s].ev
		inside, own := false, false
		for _, k := range kept {
			inside = inside || k.covers(tokenPath(body, eventPos(*ev)))
			own = own || k.seq == s
		}
		_, whole, isDo := es.doBody(ev, body, tok)
		switch {
		case inside && (ev.kind == evDynBind || ev.kind == evStore || ev.kind == evBindTwin):
			return nil, false
		case !inside && !own && !restartRead(ev) && !(isDo && whole):
			return nil, false
		}
	}
	sort.Slice(kept, func(i, j int) bool { return pathLess(kept[i].path, kept[j].path) })
	return kept, true
}

// pathLess orders two token paths as the tokens they reach stand in the
// program: by the first index they differ at, an enclosing token first.
func pathLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// parenOf is the path (tokenPath) to the paren of the statement, from body
// token tok on, that call event ev opens — ev at its first token — when the
// paren leaves exactly ev's one value: a paren seals the stack off, so the
// call takes nothing from outside it, and it leaves its value alone when it
// takes every later token as one operand each. A bare word whose call took
// no operand at all is its one value the same way. ok is false for any
// other event.
func parenOf(ev *EmitEvent, body []core.Value, tok int) ([]int, bool) {
	p := eventPos(*ev)
	path := tokenPath(body, p)
	if len(path) == 0 || path[0] < tok {
		return nil, false
	}
	ops, nout := callShape(ev)
	toks := body
	for i, at := range path {
		inner, nested := nestedToks(toks[at])
		if core.IsParenExpr(toks[at]) && len(inner) > 0 && inner[0].Pos() == p {
			return path[:i+1], nout == 1 && len(ops) == len(inner)-1
		}
		if !nested {
			return path, i == len(path)-1 && core.IsWord(toks[at]) && toks[at].Pos() == p && nout == 1 && len(ops) == 0
		}
		toks = inner
	}
	return nil, false
}

// doBody is how a statement island treats a `do` over a literal body (NUR286):
// a body whose unit ran only re-runnable reads runs again whole; a body of
// ONE call — its paren, or its bare word — ran in the compiled code, so the
// island writes the do's result in place of the do and its body list (sub,
// the do word's path; a run of two tokens), read where the compiled code
// holds it. The interpreter puts a do's result back on the tape in the do's
// place and steps it there, as the island steps the token. ok is false for
// any other event, and for a body of any other shape.
func (es *EmitState) doBody(ev *EmitEvent, body []core.Value, tok int) (sub []int, whole, ok bool) {
	c := &ev.call
	if ev.kind != evCall || c.word != "do" || c.nout != 1 || len(c.ops) != 1 || c.ops[0].kind != opClosure {
		return nil, false, false
	}
	u := c.ops[0].closureUnit
	if u < 0 || u >= len(es.fnRecs) || es.fnRecs[u] == nil || es.fnRecs[u].frag == nil || es.fnRecs[u].lambdaUnit || es.fnRecs[u].storedRefUnit {
		return nil, false, false
	}
	var calls []*EmitEvent
	for _, te := range rootTreeEvents(es.fnRecs[u].frag.events, false) {
		if !restartRead(te.ev) {
			calls = append(calls, te.ev)
		}
	}
	if len(calls) == 0 {
		return nil, true, true
	}
	if len(calls) > 1 {
		return nil, false, false
	}
	path, ok := parenOf(calls[0], body, tok)
	if !ok || len(path) < 2 {
		return nil, false, false
	}
	// The call's token is the body list's only one, and the list is the
	// do's own operand: the token before it is the do word.
	toks := body
	for _, at := range path[:len(path)-2] {
		toks, _ = nestedToks(toks[at])
	}
	list := path[len(path)-2]
	elems, nested := nestedToks(toks[list])
	if !nested || core.IsParenExpr(toks[list]) || len(elems) != 1 || list == 0 || !core.IsWord(toks[list-1]) || toks[list-1].Pos() != c.pos {
		return nil, false, false
	}
	return append(append([]int(nil), path[:len(path)-2]...), list-1), false, true
}

// wordAt reports whether the token at path is a word (parenOf's bare call).
func wordAt(body []core.Value, path []int) bool {
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	return core.IsWord(toks[path[len(path)-1]])
}

// inertBefore reports whether every token before the one at path, on its own
// level of the statement — from token tok at the top, from the group's start
// inside a paren or a list literal — is a scalar literal. A word-led run the
// island writes as a value (a bare call, a do and its body) is a collection
// barrier no longer: a function word stops a forward phase and a value does
// not, so nothing before it may be collecting — `[(m.f y)]` over a 0-arg y
// calls m.f with nothing, where `[(m.f 42)]` would take the 42.
func inertBefore(body []core.Value, tok int, path []int, written ...substPlan) bool {
	toks, from := body, tok
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
		from = 0
	}
	level := path[:len(path)-1]
	for i := from; i < path[len(path)-1]; i++ {
		if !barrierFree(toks[i]) && !writtenOver(written, append(append([]int(nil), level...), i)) {
			return false
		}
	}
	return true
}

// barrierFree reports a token no forward collection passes through: a
// scalar literal, or a list literal, which places a List whatever its
// elements do (`[(l.0 true)] print "z"`). A paren's value may be a fn that
// dispatches at the pointer, and a word may be one.
func barrierFree(t core.Value) bool {
	return core.IsSteplessValue(t) || (!core.IsParenExpr(t) && t.Parent != nil && t.Parent.Equal(core.TList))
}

// writtenOver reports whether one of the runs an island writes covers the
// token at path: gone from the island, it is no barrier (NUR296 — `print
// "a" print "b"` before the stop).
func writtenOver(written []substPlan, path []int) bool {
	for _, w := range written {
		if w.covers(path) {
			return true
		}
	}
	return false
}

// callRun is the run of tokens a bare call took (NUR296): its word, the
// tokens right after it on its level that hold the arguments it took
// forward, in written order, and the tokens right before it that hold the
// ones it took off the stack (argSites), the top nearest the word — `"x"
// print/s` — the run holding nothing else. A stack argument an event left
// must come from a paren, whose own plan the run then holds (restartSubsts):
// a word's run there (`5 inc print/s`) would overlap this one. path is the
// run's first token. An island writes the call's one result in the run's
// place, or nothing for an effect (`print "a"`), so the call never runs
// twice. ok is false for any other call.
func (es *EmitState) callRun(tree map[int]treeEvent, ev *EmitEvent, body []core.Value, tok int) (path []int, span int, ok bool) {
	ops, nout := callShape(ev)
	sites, noted := es.argSites[ev.seq]
	p := eventPos(*ev)
	path = tokenPath(body, p)
	// A call whose run's count the program cannot hold (a variadic unit, a
	// caught body's latch, a region) has no fixed run to write.
	if f := es.eventInfo[ev.seq]; f.variadicResult || f.catchPhantom {
		return nil, 0, false
	}
	if nout > 1 || !noted || len(sites) != len(ops) || len(path) == 0 || path[0] < tok {
		return nil, 0, false
	}
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	at := path[len(path)-1]
	if !core.IsWord(toks[at]) || toks[at].Pos() != p {
		return nil, 0, false
	}
	lo, hi := at, at
	for _, site := range sites {
		q, made := site.pos, site.seq >= 0
		te, in := tree[site.seq]
		if made && in {
			q = eventPos(*te.ev)
		} else if made {
			return nil, 0, false
		}
		j := bodyTokenContaining(toks, q)
		switch {
		case j > at && (j == hi && hi > at || j == hi+1):
			hi = j
		case j >= 0 && j < at && (j == lo && lo < at || j == lo-1) && (!made || core.IsParenExpr(toks[j])):
			lo = j
		case j >= 0 && j < at && made:
			// A word's result (`3 4 add print/s`): the producer's own run,
			// ending right before this one on its level, joins it.
			from, ok := es.runBefore(tree, te.ev, body, tok, len(path), lo)
			if !ok {
				return nil, 0, false
			}
			lo = from
		default:
			return nil, 0, false
		}
	}
	if len(path) == 1 && lo < tok {
		return nil, 0, false
	}
	path[len(path)-1] = lo
	return path, hi - lo + 1, true
}

// runBefore is the first token of producer ev's own call run when that run
// stands at depth n, the consuming call's level, and ends right before
// token lo there: the run a call that took ev's result off the stack
// extends over. The producer stands between that level's first token and
// the call's word (callRun found it there), so a run at the same depth is
// on the same level.
func (es *EmitState) runBefore(tree map[int]treeEvent, ev *EmitEvent, body []core.Value, tok, n, lo int) (int, bool) {
	run, span, ok := es.callRun(tree, ev, body, tok)
	if !ok || len(run) != n || run[n-1]+span != lo {
		return 0, false
	}
	return run[n-1], true
}

// tokenPath is the path of token indexes from body down to the token that
// holds position p: at each level the one holding it (bodyTokenContaining),
// entered while it is a paren or a list literal.
func tokenPath(body []core.Value, p core.SrcPos) []int {
	var path []int
	for toks := body; ; {
		at := bodyTokenContaining(toks, p)
		if at < 0 {
			return path
		}
		path = append(path, at)
		inner, nested := nestedToks(toks[at])
		if !nested {
			return path
		}
		toks = inner
	}
}

// nestedToks is a paren's or a plain list literal's tokens.
func nestedToks(v core.Value) ([]core.Value, bool) {
	if core.IsParenExpr(v) {
		toks, _ := core.AsParenExpr(v)
		return toks, true
	}
	if v.Parent.Equal(core.TList) && v.Eval && !v.Quoted {
		l, err := core.AsList(v)
		return l.Slice(), err == nil
	}
	return nil, false
}

// hasPrefix reports whether path begins with prefix (non-empty).
func hasPrefix(path, prefix []int) bool {
	if len(prefix) == 0 || len(path) < len(prefix) {
		return false
	}
	for i, x := range prefix {
		if path[i] != x {
			return false
		}
	}
	return true
}

// callShape is a call event's operands and result count; none for another
// kind.
func callShape(ev *EmitEvent) ([]EmitOperand, int) {
	if ev.kind == evCall {
		return ev.call.ops, ev.call.nout
	}
	if ev.kind == evCallUser {
		return ev.uc.ops, ev.uc.nout
	}
	return nil, 0
}

// guardReruns plans the parens a guard's statement island substitutes
// (restartSubsts) when the guard on branch event seq stops it, the statement
// beginning at body token tok: the compiled code's run in it before the
// guard is every event recorded from the statement's first event up to the
// branch, its arms aside, which had not run. The guarded value's own paren
// is one of them, its value the one the guard checks.
func (es *EmitState) guardReruns(tree map[int]treeEvent, seq int, body []core.Value, tok int) ([]substPlan, bool) {
	br := tree[seq].ev.br
	arms := map[int]bool{}
	for _, frag := range []*EmitFragment{br.then, br.els} {
		if frag == nil {
			continue
		}
		for s := range rootTreeEvents(frag.events, false) {
			arms[s] = true
		}
	}
	first := statementFirstSeq(tree, seq, body[tok].Pos())
	var pending []int
	for s := range tree {
		if s >= first && s < seq && !arms[s] {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	return es.restartSubsts(tree, body, tok, pending)
}

// rootPreStart reads the program residual's entries the interpreter's stack
// holds when the statement begins at start — the residual's leading entries,
// up to the first the statement (or a later one) leaves: an event's result
// recorded at or after its first event firstSeq, or a literal written at or
// after start. Each is found where the compiled root keeps it, in stack
// order: a literal it pushes only at the program's end (a constant), an
// earlier result it promoted (its frame slot), or one it left on the stack
// — held of those, the region beneath the statement. ok is false for a
// leading literal with no position, which the walk cannot place.
func (es *EmitState) rootPreStart(lw *lowerer, residual []core.Value, start core.SrcPos, firstSeq int) (srcs []RestartSrc, held int, ok bool) {
	for _, rv := range residual {
		pr, produced := es.producedBy[rv.ID]
		q := rv.Pos()
		if (produced && pr.seq >= firstSeq) || (!produced && q.Row > 0 && !posAfter(start, q)) {
			break
		}
		switch {
		case produced:
			if slot, promoted := lw.promoted[pr.seq]; promoted {
				srcs = append(srcs, RestartSrc{Kind: RestartLocal, Idx: slot + pr.idx})
				continue
			}
			srcs = append(srcs, RestartSrc{Kind: RestartStack, Idx: held})
			held++
		case q.Row == 0 || !core.IsConcrete(rv) || rv.Carrier || rv.Dynamic:
			return nil, 0, false
		default:
			srcs = append(srcs, RestartSrc{Kind: RestartConst, Val: rv})
		}
	}
	return srcs, held, true
}

// statementFirstSeq is the first event the statement beginning at start
// records: the least seq, up to seq, of an event written at or after start.
func statementFirstSeq(tree map[int]treeEvent, seq int, start core.SrcPos) int {
	first := seq
	for s, te := range tree {
		if s < first && !posAfter(start, eventPos(*te.ev)) {
			first = s
		}
	}
	return first
}

// planUnitRestarts plans a unit's statement islands, the root's twin
// (planLandingRestarts) over the unit's body: the island reads the unit's
// names through the island environment every deopt point shares (planDeopts
// binds them — a code body's through the frame it runs in), so a lambda's or
// a stored body's unit, which escapes that frame, plans none. A landing the
// walk's own island may take (at the unit's root level, a function word
// after it) plans none either, nor does one over the unit's own values
// beneath, whose frame an island cannot seat (restartAt).
func (es *EmitState) planUnitRestarts(u *emitUnit, rec *fnUnitRec) {
	if (rec.closure && (rec.lambdaUnit || rec.storedRefUnit)) || rec.frag == nil {
		return
	}
	tree := rootTreeEvents(rec.frag.events, false)
	top := map[int]bool{}
	for i := range rec.frag.events {
		top[rec.frag.events[i].seq] = true
	}
	seqs := make([]int, 0, len(tree))
	for seq, te := range tree {
		_, landed := es.landingAfter[seq]
		w := es.landingWord[seq]
		if (landed && w.Name != "" && (w.Collected || !top[seq] || landingTopToken(rec.body, w.Pos) < 0)) || (landed && top[seq] && (es.landingOwn[seq].beneath || es.landingCollects(seq))) ||
			(te.ev.kind == evCall && te.ev.call.dynMethod != nil) {
			seqs = append(seqs, seq)
		}
	}
	sort.Ints(seqs)
	for _, seq := range seqs {
		at := tree[seq]
		tok := statementToken(rec.body, eventPos(*at.ev))
		if tok < 0 {
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: rec.body[tok].Pos(), token: tok, restart: true}
		if d.start.Row == 0 || es.deoptDeferred(u, rec, &d, -1) || outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start)) {
			continue
		}
		substs, first, reruns := es.restartReruns(tree, at, seq, rec.body, tok)
		if !reruns {
			continue
		}
		d.substs, d.first = substs, first
		rec.deopts = append(rec.deopts, d)
	}
	// The unit's branch guards (NUR292), as the root's (planGuardRestarts).
	for _, g := range branchGuards(tree) {
		if d, ok := es.guardPoint(u, rec, tree, g); ok && !outsProducedBefore(rec.outOps, statementFirstSeq(tree, g.seq, d.start)) {
			rec.deopts = append(rec.deopts, d)
		}
	}
	// Its do calls whose run's count the seat may miss (NUR222), as the
	// root's (planCountRestarts).
	es.notePhantomConsumersIn(rec.frag.events)
	for _, seq := range sortedSeqs(tree) {
		if te := tree[seq]; te.inLoop || !es.countSeat(seq) {
			continue
		}
		tok, substs, ok := es.countPoint(tree, seq, rec.body)
		if !ok {
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: rec.body[tok].Pos(), token: tok, restart: true, count: true, substs: substs}
		if d.start.Row > 0 && !es.deoptDeferred(u, rec, &d, -1) && !outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start)) {
			rec.deopts = append(rec.deopts, d)
		}
	}
}

// sortedSeqs is tree's event seqs in order.
func sortedSeqs(tree map[int]treeEvent) []int {
	seqs := make([]int, 0, len(tree))
	for seq := range tree {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	return seqs
}

// outsProducedBefore reports whether a unit's residual holds a result an
// event recorded before firstSeq produced: the unit may keep it in a frame
// slot until its RET, where the island would not find it.
func outsProducedBefore(outs []EmitOperand, firstSeq int) bool {
	for _, op := range outs {
		if op.kind == opEvent && op.idx < firstSeq {
			return true
		}
	}
	return false
}

// restartAt is the planned statement island of event seq whose depth the
// walk seated, or nil — nil too at the root when the compiled stack there is
// not the residual's results the plan counted (rootPreStart), and in a unit
// whose frame's unnamed params the walk could not place.
func (lw *lowerer) restartAt(seq int) *landingRestart {
	if r := lw.landingRestarts[seq]; r.seated() {
		return r
	}
	return nil
}

// noteRestartDepths seats the compiled stack's depth on every planned
// statement island whose statement begins at or before p, the position of
// the root event whose first op is about to be emitted. In a unit it seats
// the island's prefix there too: the unnamed params the interpreter's frame
// still holds on its stack bottom at the statement's start (deoptPrefix,
// read from their slots), then the frame region.
func (lw *lowerer) noteRestartDepths(p core.SrcPos) {
	for _, m := range []map[int]*landingRestart{lw.landingRestarts, lw.guardRestarts, lw.countRestarts} {
		for _, r := range m {
			lw.noteRestartDepth(r, p)
		}
	}
}

// noteRestartDepth is noteRestartDepths for one island.
func (lw *lowerer) noteRestartDepth(r *landingRestart, p core.SrcPos) {
	if r.depth >= 0 || p.Row == 0 || posAfter(r.start, p) {
		return
	}
	r.depth = len(lw.vm)
	if lw.landingRoot {
		return
	}
	params, ok := lw.deoptPrefix()
	if !ok {
		r.unseatable = true
		return
	}
	for _, slot := range params {
		r.srcs = append(r.srcs, RestartSrc{Kind: RestartLocal, Idx: slot})
	}
	for i := 0; len(params) > 0 && i < r.depth; i++ {
		r.srcs = append(r.srcs, RestartSrc{Kind: RestartStack, Idx: i})
	}
}

// restartAnchor is where root event ev stands in its statement for the
// islands' depth (noteRestartDepths): a guarded branch at its `if` word
// (BranchRecord.CondCheckPos) — its own position is its condition value's,
// which an earlier statement may have written — and any other event at its
// own position.
func restartAnchor(ev *EmitEvent) core.SrcPos {
	if ev.kind == evBranch && ev.br.condCheckPos.Row > 0 {
		return ev.br.condCheckPos
	}
	return eventPos(*ev)
}

// treeEvent is one event of the root's tree: the event, whether a loop
// fragment holds it, the index slots of the loops that do, and those loops.
type treeEvent struct {
	ev        *EmitEvent
	inLoop    bool
	loopSlots []int
	loops     []*emitLoop
}

// rootTreeEvents indexes events and every event of their nested fragments
// by seq, marking the ones a loop's fragment holds.
func rootTreeEvents(events []EmitEvent, inLoop bool) map[int]treeEvent {
	return treeEventsUnder(events, inLoop, nil, nil)
}

// treeEventsUnder is rootTreeEvents under the enclosing loops and their
// index slots.
func treeEventsUnder(events []EmitEvent, inLoop bool, slots []int, loops []*emitLoop) map[int]treeEvent {
	out := map[int]treeEvent{}
	for i := range events {
		ev := &events[i]
		out[ev.seq] = treeEvent{ev: ev, inLoop: inLoop, loopSlots: slots, loops: loops}
		inner, innerLoops := slots, loops
		if ev.kind == evLoop {
			inner = append(append([]int(nil), slots...), ev.loop.iterSlot)
			innerLoops = append(append([]*emitLoop(nil), loops...), ev.loop)
		}
		for _, frag := range childFragments(ev) {
			if frag == nil {
				continue
			}
			for seq, te := range treeEventsUnder(frag.events, inLoop || ev.kind == evLoop, inner, innerLoops) {
				out[seq] = te
			}
		}
	}
	return out
}

// firstIterGuard is the check a statement island inside loops takes at run
// time (RestartFirst): every enclosing loop on its FIRST iteration — its
// index slot holding its start — so no earlier iteration ran what the
// island runs again. ok is false for a loop the check cannot read: a
// condition loop, or a start that is no constant integer.
func (es *EmitState) firstIterGuard(loops []*emitLoop) ([]RestartFirst, bool) {
	out := make([]RestartFirst, 0, len(loops))
	for _, lp := range loops {
		if lp.cond != nil || lp.start.kind != opConst || lp.start.idx < 0 || lp.start.idx >= len(es.consts) {
			return nil, false
		}
		n, ok := es.consts[lp.start.idx].Data.(core.IntPayload)
		if !ok {
			return nil, false
		}
		out = append(out, RestartFirst{Slot: lp.iterSlot, Val: n.N})
	}
	return out, true
}

// loopFree reports whether no paren the island substitutes runs inside a
// loop: its value is one iteration's, where the island runs every iteration
// from its tokens.
func loopFree(tree map[int]treeEvent, substs []substPlan) bool {
	for _, sp := range substs {
		if tree[sp.seq].inLoop {
			return false
		}
	}
	return true
}

// statementToken is the program token the statement holding position p
// begins at: the top-level token holding p, walked back to just after the
// `end` before it (or the program's first token); -1 when no top-level
// token holds p.
func statementToken(body []core.Value, p core.SrcPos) int {
	at := bodyTokenContaining(body, p)
	if at < 0 {
		return -1
	}
	for at > 0 && !core.IsEnd(body[at-1]) {
		at--
	}
	return at
}

// restartReruns reports whether the statement at body token tok may run
// again from its first token when event seq (at) stops it, the parens its
// island substitutes, and the loops' first-iteration check it takes: every
// event the compiled code ran before the stop is re-runnable or ran inside
// a substituted paren (restartRunsReadsOnly). A stop inside a loop's body
// may have run earlier iterations WHOLE — the body's events after the stop
// too. Inside counted loops the island checks at run time that each is on
// its first iteration (firstIterGuard), and no paren inside one is
// substituted; otherwise every event of the statement's span must be
// re-runnable, the branches and loops that hold them aside, over a stop
// that fires on the first iteration or never (restartSpanReruns).
func (es *EmitState) restartReruns(tree map[int]treeEvent, at treeEvent, seq int, body []core.Value, tok int) ([]substPlan, []RestartFirst, bool) {
	start := body[tok].Pos()
	if !at.inLoop {
		substs, ok := es.restartRunsReadsOnly(tree, seq, body, tok, core.SrcPos{})
		return substs, nil, ok
	}
	if first, counted := es.firstIterGuard(at.loops); counted {
		if substs, ok := es.restartRunsReadsOnly(tree, seq, body, tok, eventPos(*at.ev)); ok && loopFree(tree, substs) {
			return substs, first, true
		}
	}
	end := len(body)
	for i := tok + 1; i < len(body); i++ {
		if core.IsEnd(body[i]) {
			end = i
			break
		}
	}
	// The landed value: the event itself, or a shaped apply's method.
	landed := seq
	if at.ev.kind == evCall && at.ev.call.dynMethod != nil {
		landed = methodSeq(at.ev)
	}
	return nil, nil, restartSpanReruns(tree, start, end, body, landed)
}

// restartSpanReruns reports whether a stop inside a loop may restart the
// statement spanning from start up to body token end. The stop's inputs must
// be LOOP-INVARIANT, so the stop fires on the first iteration if it fires on
// any, before the rest of the body has run once: the landed value's read
// (event landed) over operands that are constants or slots no enclosing
// loop steps, and no event of the span binding a name or a slot (a def, a
// loop-carried store, a bind transition) that the read or the landing's
// word could see. Then only the events the stopping iteration ran BEFORE
// the stop — recorded before the landed value — must be re-runnable: a
// branch or a loop, or a restartRead.
func restartSpanReruns(tree map[int]treeEvent, start core.SrcPos, end int, body []core.Value, landed int) bool {
	for _, te := range tree {
		p := eventPos(*te.ev)
		if p.Row == 0 || posAfter(start, p) || (end < len(body) && !posAfter(body[end].Pos(), p)) {
			continue
		}
		switch k := te.ev.kind; {
		case k == evDynBind || k == evStore || k == evBindTwin:
			return false
		case te.ev.seq < landed && k != evBranch && k != evLoop && !restartRead(te.ev):
			return false
		}
	}
	lt, ok := tree[landed]
	if !ok || lt.ev.kind != evCall {
		return false
	}
	for _, op := range lt.ev.call.ops {
		if op.kind == opEvent || (op.kind == opLocal && containsInt(lt.loopSlots, op.idx)) {
			return false
		}
	}
	return true
}

// methodSeq is the event a shaped apply takes as its method (its first
// operand), or -1.
func methodSeq(ev *EmitEvent) int {
	if len(ev.call.ops) == 0 || ev.call.ops[0].kind != opEvent {
		return -1
	}
	return ev.call.ops[0].idx
}

// containsInt reports whether xs holds x.
func containsInt(xs []int, x int) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// restartRunsReadsOnly plans the parens a statement island substitutes
// (restartSubsts) so that its second run repeats no effect: the compiled
// code's run in the statement, from its start (body token tok) up to event
// seq, is every event recorded in that span. A shaped apply's own event has
// not run when it restarts; a landed event has, and is planned like the
// others. On a loop's first iteration (after set), an event written after
// the stop has not run either, whatever order the pass recorded it in — the
// tape runs left to right — so it is not the run's.
func (es *EmitState) restartRunsReadsOnly(tree map[int]treeEvent, seq int, body []core.Value, tok int, after core.SrcPos) ([]substPlan, bool) {
	first := statementFirstSeq(tree, seq, body[tok].Pos())
	var pending []int
	for s, te := range tree {
		written := eventPos(*te.ev)
		if s >= first && s <= seq && (s != seq || te.ev.kind != evCall || te.ev.call.dynMethod == nil) &&
			(after.Row == 0 || written.Row == 0 || !posAfter(written, after)) {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	return es.restartSubsts(tree, body, tok, pending)
}

// restartRead reports whether ev runs no user code, binds nothing and has
// no effect a second run repeats: a native call — a member read (`dot`,
// `get`), a stack shuffle, a list or map literal's assembly, or a word its
// signature declares island-pure (core.CompileIslandPure). Its operands are
// the statement's own values, which the island computes again.
func restartRead(ev *EmitEvent) bool {
	if ev.kind != evCall {
		return false
	}
	c := &ev.call
	if c.dynMethod != nil || c.dynApply > 0 || c.dynMixed || c.live || c.interp || c.xmlTmpl != nil || c.listReStep != nil {
		return false
	}
	return c.word == "dot" || c.word == "get" || core.DynStackShuffleWords[c.word] || c.makeList || c.makeMap ||
		(!c.poly && c.sig != nil && c.sig.CompileEffect&core.CompileIslandPure != 0)
}
