package compiler

import (
	"slices"
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
	// beneath is the compiled stack beneath the statement when it began
	// (the root's; noteRestartDepth): the island reads the held values
	// there, so they must still be there at the stop (restartAt).
	beneath []vmSlot
	// heldAt, where the pass told the stack at the statement's start
	// (seatStack), is the held values' producers, bottom first: the stack
	// at the stop must hold exactly those beneath the statement.
	heldAt []vmSlot
	// substs are the parens the island writes the compiled code's values in
	// place of (restartSubsts).
	substs []substPlan
	// first is the loops' first-iteration check the island takes at run
	// time (firstIterGuard); empty outside loops.
	first []RestartFirst
	// leftovers are a unit island's def leftovers (deoptPoint.leftovers),
	// and defBound the values those defs bound, which the compiled frame
	// may still hold where the interpreter's def took them
	// (noteRestartDepth).
	leftovers, defBound []producer
	// onFrame are the earlier statements' values a unit's island needs held
	// beneath the statement (deoptPoint.onFrame): the walk admits the island
	// only where the compiled frame holds each there and the region stands
	// intact at the stop (heldIntact).
	onFrame []producer
	// lits are the earlier statements' literals a unit's island seats among
	// its frame region by where each was written (deoptPoint.lits), posOf
	// where the unit's events were written.
	lits  []seatLit
	posOf map[int]core.SrcPos
	// loop, on an island in a counted loop's body, is its per-iteration
	// continuation (loopContPlan): token and start are the stop's statement
	// in the loop's body, and depth the compiled stack's depth there above
	// the iteration's start.
	loop *loopCont
	// run marks a count island of a `do` over a computed body, which its
	// call takes on a run the interpreter's tape would step (a splice, or a
	// fn value where the run is seated as data); always, one it takes
	// whatever the run left (SigRef.CountAlways): such a run before the
	// program's terminal trap (planCountRestarts).
	run, always bool
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
	// reach marks a paren apply's lead that is a member read's own value
	// (parenLead): the island writes it as the reach group the read's token
	// lowers to (RestartSubst.Reach).
	reach bool
	// named marks a paren apply's lead that is a def-bound word (parenLead):
	// the island writes its fn as the name's call runs it (RestartSubst.Named).
	named bool
	// lit marks a def-bound word's read the pass folded (boundWordPlans): the
	// island writes val, the constant the compiled code pushes for it
	// (RestartConst), and seq is -1.
	lit bool
	val core.Value
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
// statement ran member reads only. Where the program ends in a terminal
// trap only a shaped apply recorded before it plans, over a stack the pass
// told: its island runs the statement and the program after it, the trap's
// region included (`3 1 (m.f 7) add add` over a data member, NUR336).
func (es *EmitState) planLandingRestarts(lw *lowerer, residual []core.Value) {
	if len(es.rootBody) == 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	seqs := make([]int, 0, len(es.landingAfter))
	for seq, te := range tree {
		_, landed := es.landingAfter[seq]
		shaped := te.ev.kind == evCall && te.ev.call.dynMethod != nil
		if es.trapAt != 0 && (!shaped || seq > es.trapAt) {
			continue
		}
		if (landed && (es.landingWord[seq].Name != "" || es.landingOwn[seq].beneath || es.landingCollects(seq))) || shaped {
			seqs = append(seqs, seq)
		}
	}
	sort.Ints(seqs)
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	for _, seq := range seqs {
		at := tree[seq]
		tok := statementToken(es.rootBody, stopPos(at.ev))
		if tok < 0 {
			continue
		}
		tok = es.toldAfter(literalDefsBefore(tree, es.rootBody, tok, stopPos(at.ev)), stopPos(at.ev))
		if at.inLoop {
			// A stop in a loop's body: the island continues the loop from
			// the stop's own iteration (loopContPlan, NUR336), where a
			// statement island from the loop's statement serves only its
			// first iteration.
			outer := func(forTok int) (*landingRestart, bool) { return es.rootLoopOuter(lw, forTok) }
			if r, ok := es.loopContPlan(tree, at, seq, es.rootBody, outer); ok {
				if lw.landingRestarts == nil {
					lw.landingRestarts = map[int]*landingRestart{}
				}
				lw.landingRestarts[seq] = r
				es.noteLeadRead(tree, at.ev)
				continue
			}
		}
		if takesBeneath(tree, es.rootBody, statementStart(es.rootBody, tok), seq) {
			// The statement's run before the stop took values from beneath
			// it, which the compiled stack no longer holds at the stop: the
			// island takes the statement over from the stop's own token,
			// over the stack the pass told there (lateRootRestart, NUR336).
			es.lateRootRestart(lw, tree, at, seq, tok)
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: statementStart(es.rootBody, tok), token: tok}
		// Where the pass told the stack at the statement's start the island
		// seats exactly that (stackAtStart), and the walk checks each value
		// the compiled stack holds is still there at the stop (heldIntact):
		// the deferred-operand accounting the residual's prefix needs is moot.
		if _, told := es.stackAtStart(tok); d.start.Row == 0 || (!told && (es.trapAt != 0 || es.deoptDeferred(es.units[0], rec, &d, -1))) {
			continue
		}
		substs, first, reruns := es.restartReruns(tree, at, seq, es.rootBody, tok)
		if !reruns {
			continue
		}
		srcs, held, slots, ok := es.rootPreStart(lw, tree, residual, tok, d.start, statementFirstSeq(tree, seq, d.start))
		if !ok {
			continue
		}
		if lw.landingRestarts == nil {
			lw.landingRestarts = map[int]*landingRestart{}
		}
		lw.landingRestarts[seq] = &landingRestart{token: tok, start: d.start, depth: -1, srcs: srcs, held: held, heldAt: slots, substs: substs, first: first}
		es.noteLeadRead(tree, at.ev)
	}
}

// defsBefore is a unit's twin of literalDefsBefore: past the defs its
// statement opens with, each of a value the def took whole from the one
// token after its name — a scalar literal, or a paren or a reach an event
// inside which produced the value (`def m (mk) def k 3 k (m.f 7)` in a fn
// body, NUR336). Such a def leaves nothing pending and ran in the compiled
// code, never again; the island seats the frame region the compiled code
// holds after it (noteRestartDepth) — what a paren left beneath the bound
// value included — and reads the names it bound through the unit's island
// environment.
func defsBefore(tree map[int]treeEvent, body []core.Value, tok int, p core.SrcPos) int {
	defs := treeDefs(tree)
	for at := bodyTokenContaining(body, p); ; {
		next := leadingDef(tree, defs, body, tok)
		if next < 0 || next > at {
			return tok
		}
		tok = next
	}
}

// treeDefs indexes a tree's def binds by their def site.
func treeDefs(tree map[int]treeEvent) map[core.SrcPos]*emitDynBind {
	defs := map[core.SrcPos]*emitDynBind{}
	for _, te := range tree {
		if d := te.ev.dyn; te.ev.kind == evDynBind && d != nil {
			defs[d.pos] = d
		}
	}
	return defs
}

// leadingDef is the body token after the def or the def-making branch a
// statement opens with at token tok, or -1 when it opens with neither. A def takes its value whole from the one token
// after its name (tookWhole); a branch is `if` over one condition token and
// its arm lists (quietBranchAt) — NUR357's `if c [def q 3] [def q 4] {b:2}
// keys m.a`, whose arms bind and leave nothing: it ran in the compiled code,
// never again, and the island starts past it.
func leadingDef(tree map[int]treeEvent, defs map[core.SrcPos]*emitDynBind, body []core.Value, tok int) int {
	if tok+3 > len(body) {
		return -1
	}
	w, err := core.AsWord(body[tok])
	if err != nil {
		return -1
	}
	switch {
	case w.Name == "def" && core.IsWord(body[tok+1]):
		if d := defs[body[tok+1].Pos()]; d != nil && tookWhole(tree, d, body, tok+2) {
			return tok + 3
		}
	case w.Name == "if":
		return quietBranchAt(tree, body, tok)
	}
	return -1
}

// quietBranchAt is the body token after an `if` at token tok whose branch
// event stands in the tree (outside any loop) over body arms that leave
// nothing — no value, no pending literal — its condition the one token after
// the word and its arms the list tokens after that; -1 otherwise.
func quietBranchAt(tree map[int]treeEvent, body []core.Value, tok int) int {
	if tok < 0 || tok+3 > len(body) {
		return -1
	}
	if w, err := core.AsWord(body[tok]); err != nil || w.Name != "if" || !unmodifiedWordInfo(w) {
		return -1
	}
	for _, te := range tree {
		br := te.ev.br
		if te.ev.kind != evBranch || br == nil || te.inLoop || br.pending.Left || br.condFrag != nil || br.thenIsVal || br.elsIsVal {
			continue
		}
		if br.pos != body[tok+1].Pos() && br.pos != body[tok].Pos() {
			continue
		}
		span := 3
		if br.hasElse {
			span = 4
		}
		if tok+span > len(body) || !armLeavesNothing(br.then, body[tok+2]) || (br.hasElse && !armLeavesNothing(br.els, body[tok+3])) {
			return -1
		}
		return tok + span
	}
	return -1
}

// armLeavesNothing reports whether a branch arm is the body list tok and its
// recorded fragment leaves no value.
func armLeavesNothing(frag *EmitFragment, tok core.Value) bool {
	return frag != nil && frag.residualN == 0 && tok.Parent.Equal(core.TList) && tok.Eval && !tok.Quoted
}

// unmodifiedWordInfo reports whether w carries no modifier the source wrote.
func unmodifiedWordInfo(w core.WordInfo) bool {
	return w.ArgCount == -1 && !w.ForceStack && !w.ForceForward && !w.ForceVal && !w.ForceUsurp
}

// defLeftovers are the results the defs from body token tok up to (not
// including) token to — the ones defsBefore stepped past — left beneath
// the values they bound, in the order the interpreter's frame holds them: a
// def binds its call's first result (RecordDefBind's srcSeq; bound lists
// those) and every later one stays (`def k (3 dup)` leaves the second 3,
// NUR336). A def's call of a result count this cannot read leaves none it
// can name, and an island over it keeps finding such a value deferred.
func defLeftovers(tree map[int]treeEvent, body []core.Value, tok, to int) (leftovers, bound []producer) {
	defs := treeDefs(tree)
	for tok+3 <= to {
		if next := quietBranchAt(tree, body, tok); next >= 0 {
			tok = next
			continue
		}
		d := defs[body[tok+1].Pos()]
		tok += 3
		if d == nil || d.srcSeq < 0 {
			continue
		}
		bound = append(bound, producer{seq: d.srcSeq})
		for i := 1; i < resultCount(tree[d.srcSeq].ev); i++ {
			leftovers = append(leftovers, producer{seq: d.srcSeq, idx: i})
		}
	}
	return leftovers, bound
}

// resultCount is how many results call event ev leaves: a native's or a
// user fn's recorded count, and one for any other event (or none).
func resultCount(ev *EmitEvent) int {
	if ev == nil {
		return 1
	}
	switch ev.kind {
	case evCall:
		return ev.call.nout
	case evCallUser:
		return ev.uc.nout
	}
	return 1
}

// tookWhole reports whether def d took its value whole from body token at: a
// scalar literal the def bound as it is, or a paren or a reach the value's
// producing event stands inside.
func tookWhole(tree map[int]treeEvent, d *emitDynBind, body []core.Value, at int) bool {
	if d.srcSeq < 0 {
		return d.src.kind == opNone && core.IsSteplessValue(body[at])
	}
	te, in := tree[d.srcSeq]
	return in && (core.IsParenExpr(body[at]) || core.IsReach(body[at])) && bodyTokenContaining(body, eventPos(*te.ev)) == at
}

// toldAfter is where a statement island may take over the statement it would
// run from body token tok when stopped at position p: the last token from tok
// up to the one holding p whose stack the pass told as a def completed right
// before it (the engine's noteDefStack) — every run before it stays the
// compiled code's, and what it left beneath is seated as told (`def k (3
// dup) k (m.f 7)`, NUR336) — or tok itself.
func (es *EmitState) toldAfter(tok int, p core.SrcPos) int {
	at := tok
	for i, stop := tok+1, bodyTokenContaining(es.rootBody, p); i <= stop; i++ {
		if _, told := es.toldAt(i); told {
			at = i
		}
	}
	return at
}

// toldAt is the stack the pass told at root body token tok
// (NoteStatementStack), by the token's position — or, for a paren the engine
// expanded to its markers before stepping it (`def k (3 dup) (m.f 7)`: the
// def's operand group expands the paren after it too), by its first inner
// token's, where the engine keys a positionless open paren's note
// (statementStackPos, NUR336). No note is ever keyed by a token inside a
// paren otherwise: a statement stack is told over values alone, and an open
// paren beneath is none.
func (es *EmitState) toldAt(tok int) ([]core.Value, bool) {
	t := es.rootBody[tok]
	if p := t.Pos(); p.Row > 0 {
		if stack, told := es.rootStmtStacks[p]; told {
			return stack, true
		}
	}
	if !core.IsParenExpr(t) {
		return nil, false
	}
	if inner, _ := core.AsParenExpr(t); len(inner) > 0 && inner[0].Pos().Row > 0 {
		stack, told := es.rootStmtStacks[inner[0].Pos()]
		return stack, told
	}
	return nil, false
}

// statementStart is the position a statement island beginning at body token
// tok orders the program's events by: the token's own, or — for a token the
// parser mints without one (a bare pair, `a:1 (m.f 7)`) — the point just
// past the `end` before it, which every later token follows (the program's
// first token, when none precedes it). Zero where neither is known.
func statementStart(body []core.Value, tok int) core.SrcPos {
	if p := body[tok].Pos(); p.Row > 0 {
		return p
	}
	if tok == 0 {
		return core.SrcPos{Row: 1}
	}
	if e := body[tok-1].Pos(); core.IsEnd(body[tok-1]) && e.Row > 0 {
		return core.SrcPos{Row: e.Row, Col: e.Col + 1}
	}
	return core.SrcPos{}
}

// planRematchRestart plans the statement island of the root's runtime
// rematch trap (DispatchSpec.Restart): the pass's match of a word failed
// over a value it holds as a carrier the run may match — a paren apply's
// result, which the pass holds as any value (`k (m.f 7) add add`, NUR336) —
// and the compiled program ends at the trap. Where the run matches, the
// interpreter runs the statement and the program after it from the
// statement's first token instead, seated on the stack the pass told there:
// every event of the statement before the trap re-runs or is written as the
// value its call left (restartSubsts), so nothing runs twice. A trap inside a
// loop, one armed with a trap of its own (NUR264), or a statement whose stack
// was not told plans none: the rematch keeps its defer. A statement whose
// island cannot run again a BRANCH the compiled code ran before the rematch
// — an arm holding an effect or a binding (`each (if c [[print "z" 1]] [3])
// [2]`, an arm `[def q 3 q]`) — declines the program instead (NUR343): where
// the run matches, the compiled code has no answer, and the interpreter's
// run of the statement would repeat the arm's effect.
func (es *EmitState) planRematchRestart(lw *lowerer, residual []core.Value) string {
	if len(es.rootBody) == 0 || es.trapAt == 0 {
		return ""
	}
	tree := rootTreeEvents(es.frames[0], false)
	te, in := tree[es.trapAt]
	if !in || te.inLoop || te.ev.kind != evTrap || te.ev.trap.rematchWord == "" || te.ev.trap.rematchOnMatch != nil {
		return ""
	}
	p := te.ev.trap.pos
	tok := statementToken(es.rootBody, p)
	if tok < 0 {
		return ""
	}
	tok = es.toldAfter(literalDefsBefore(tree, es.rootBody, tok, p), p)
	start := statementStart(es.rootBody, tok)
	if _, told := es.stackAtStart(tok); !told {
		return ""
	}
	first := statementFirstSeq(tree, es.trapAt, start)
	var pending []int
	for s := range tree {
		if s >= first && s < es.trapAt {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	substs, ok := es.restartSubsts(tree, es.rootBody, tok, pending)
	if !ok {
		return unrerunBranchBefore(tree, pending)
	}
	srcs, held, slots, ok := es.rootPreStart(lw, tree, residual, tok, start, first)
	if !ok {
		return ""
	}
	if lw.landingRestarts == nil {
		lw.landingRestarts = map[int]*landingRestart{}
	}
	lw.landingRestarts[es.trapAt] = &landingRestart{token: tok, start: start, depth: -1, srcs: srcs, held: held, heldAt: slots, substs: substs}
	return ""
}

// lowerRootEvents plans the root rematch's statement island
// (planRematchRestart, whose decline it returns) and lowers the root's
// events, seeding the lowerer's frame-local counter from the unit's planned
// locals first — spillSeat bumps it for spill temps, and Finalize writes it
// back so Program.NumLocals covers them. It returns the lowering's decline
// reason, "" when it lowered.
func (es *EmitState) lowerRootEvents(lw *lowerer, residual []core.Value) string {
	if reason := es.planRematchRestart(lw, residual); reason != "" {
		return reason
	}
	if reason := es.pendingArmRefusal(es.frames[0], es.rootBody, es.reg); reason != "" {
		return reason
	}
	lw.numLocals = es.units[0].numLocals
	return lw.lowerEvents(es.frames[0], 0)
}

// rematchBranchUnrerun is the decline of a runtime rematch whose statement
// ran a branch its island cannot run again (NUR343).
const rematchBranchUnrerun = "a runtime rematch follows a branch whose arm holds an effect or a binding: where the run matches, the statement's island would run the arm again (NUR343)"

// unrerunBranchBefore is the rematch's decline when one of the pending
// events is a branch (not nested in another) that rerunBranch refuses, and
// "" otherwise: the rematch then keeps its defer, as before.
func unrerunBranchBefore(tree map[int]treeEvent, pending []int) string {
	for _, s := range pending {
		if ev := tree[s].ev; ev.kind == evBranch && !rerunBranch(ev) {
			return rematchBranchUnrerun
		}
	}
	return ""
}

// literalDefsBefore is where a statement island may take over the statement
// at body token tok that holds position p: past the defs of a literal it
// opens with (`def k 3 k (m.f 7)`, NUR336). Such a def collects its name and
// its value and leaves nothing pending, so the tokens after it run on the
// interpreter exactly as a statement of their own would — and the island
// need not bind the name a second time (a def is no re-runnable read). Each
// is the three tokens `def`, a name and a scalar literal whose def the pass
// recorded at that name (a def's event stands at its NAME token), all before
// the token holding p.
func literalDefsBefore(tree map[int]treeEvent, body []core.Value, tok int, p core.SrcPos) int {
	defs := map[core.SrcPos]bool{}
	for _, te := range tree {
		if d := te.ev.dyn; te.ev.kind == evDynBind && d != nil && d.srcSeq < 0 && d.src.kind == opNone {
			defs[d.pos] = true
		}
	}
	for at := bodyTokenContaining(body, p); tok+3 <= at; tok += 3 {
		if w, err := core.AsWord(body[tok]); err != nil || w.Name != "def" || !core.IsWord(body[tok+1]) ||
			!core.IsSteplessValue(body[tok+2]) || !defs[body[tok+1].Pos()] {
			break
		}
	}
	return tok
}

// fitStartBefore is literalDefsBefore past the def-making branches the
// statement opens with too (quietBranchAt, NUR357): `if c [def q 3] [def q
// 4] {b:2} keys m.a` — the branch ran in the compiled code and left
// nothing, and the poly's forward-fit island takes the statement over after
// it.
func fitStartBefore(tree map[int]treeEvent, body []core.Value, tok int, p core.SrcPos) int {
	at := bodyTokenContaining(body, p)
	for {
		tok = literalDefsBefore(tree, body, tok, p)
		next := quietBranchAt(tree, body, tok)
		if next < 0 || next > at {
			return tok
		}
		tok = next
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
		srcs, held, _, ok := es.rootPreStart(lw, tree, residual, d.token, d.start, statementFirstSeq(tree, g.seq, d.start))
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
//
// A `do` over a COMPUTED body (dynBodyRun) plans its island whatever its
// seat: its run may hold what the interpreter's tape steps where it lands —
// a splice (`quote [w/v]` over `def w word [1 2]`, NUR348), a fn value where
// the run is seated as data — which no compiled seat takes, and which the
// island writes back in the do's place for the interpreter to step.
//
// Where the program ends in a terminal trap, the trap is the pass's proof
// over the bindings the model held — and a computed body run before it may
// have changed any of them (NUR348: `do (mk) x.0` over `def x 0` and a body
// binding x to [1 2] is a static no-match to the pass, `[1]` interpreted).
// So each root `do` over a computed body the pass recorded before the trap
// plans its count island whatever its seat, over a stack the pass told at
// its statement's start (the residual is dropped under a trap), and the call
// takes it whatever the run left (always, SigRef.CountAlways): the island
// runs the statement and the program after it — the trap's statement
// included — on the interpreter, the run written in the do's place. The
// values the stack holds beneath the statement are kept off the dead list,
// as the trap's live read keeps them (trapHeldBeneath).
func (es *EmitState) planCountRestarts(lw *lowerer, residual []core.Value) {
	es.notePhantomConsumers()
	if len(es.rootBody) == 0 {
		return
	}
	tree := rootTreeEvents(es.frames[0], false)
	for seq, te := range tree {
		trapped, run := es.trapAt != 0, es.dynBodyRun(te.ev)
		if te.inLoop || (trapped && (seq > es.trapAt || !run)) || (!run && !es.countSeat(seq)) {
			continue
		}
		tok, substs, ok := es.countPoint(tree, seq, es.rootBody)
		if !ok {
			continue
		}
		if _, told := es.stackAtStart(tok); trapped && !told {
			continue
		}
		start := statementStart(es.rootBody, tok)
		srcs, held, slots, ok := es.rootPreStart(lw, tree, residual, tok, start, statementFirstSeq(tree, seq, start))
		if !ok {
			continue
		}
		if lw.countRestarts == nil {
			lw.countRestarts = map[int]*landingRestart{}
		}
		r := &landingRestart{token: tok, start: start, depth: -1, srcs: srcs, held: held, substs: substs, run: run, always: trapped}
		if trapped {
			r.heldAt = slots
			for _, slot := range slots {
				delete(lw.dead, slot.seq)
			}
			// The runs the island writes as their values read them where
			// the compiled code left them (heldAt), never dropped.
			for _, sp := range substs {
				delete(lw.dead, sp.seq)
			}
		}
		lw.countRestarts[seq] = r
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
	island := ev.kind == evFallback && es.eventInfo[seq].stripIsland
	if !island && (ev.kind != evCall || !(c.word == "do" && len(c.ops) == 1 || c.word == "error" && len(c.ops) == 2)) {
		return 0, nil, false
	}
	pos := eventPos(*ev)
	tok := statementToken(body, pos)
	path := tokenPath(body, pos)
	if tok < 0 || len(path) == 0 {
		return 0, nil, false
	}
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	at := path[len(path)-1]
	if at+1 >= len(toks) || !core.IsWord(toks[at]) || toks[at].Pos() != pos {
		return 0, nil, false
	}
	// The run the island writes: the do and its body, or — for an error
	// handler, computed (NUR300) or a literal one's interpreter island
	// (NUR301) — the do before the word, its body, the word and the
	// handler, since the handler took the do's caught value.
	span := 2
	switch {
	case island && !es.islandRun(tree, seq, toks, at), !island && c.word == "error" && !es.handlerRun(tree, seq, toks, at):
		return 0, nil, false
	case island || c.word == "error":
		path, span = append(append([]int(nil), path[:len(path)-1]...), at-2), 4
	case !es.doBodyAfter(tree, seq, toks, at):
		return 0, nil, false
	}
	first := statementFirstSeq(tree, seq, statementStart(body, tok))
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
// its caught value from the do written right before it (doBefore — `do
// [raise oops 'x'] error (mk)`, NUR300): the four tokens are one run.
func (es *EmitState) handlerRun(tree map[int]treeEvent, seq int, toks []core.Value, at int) bool {
	c := &tree[seq].ev.call
	sites := es.argSites[seq]
	if len(sites) != 2 || !es.doBefore(tree, c.ops[1], toks, at) {
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

// islandRun reports whether the strip word's interpreter island at toks[at]
// threads the value of the do written right before it over a literal body,
// and runs the literal handler written right after the word (`do [raise
// oops 'x'] error [drop 5 6]`, NUR301): the four tokens are one run, as a
// computed handler's are (handlerRun).
func (es *EmitState) islandRun(tree map[int]treeEvent, seq int, toks []core.Value, at int) bool {
	fb := &tree[seq].ev.fb
	return len(fb.ins) == 1 && es.doBefore(tree, fb.ins[0], toks, at) && literalListTok(toks[at+1])
}

// doBefore reports whether op is the value of the do written two tokens
// before the word at toks[at], with its body — a literal list, or a
// computed body whose one argument site is that token (doBodyAfter) —
// right after it: `do [raise oops 'x'] error …`, `(do b error …)`. The do,
// its body, the word and the handler are then one run.
func (es *EmitState) doBefore(tree map[int]treeEvent, op EmitOperand, toks []core.Value, at int) bool {
	if at < 2 || op.kind != opEvent {
		return false
	}
	do, in := tree[op.idx]
	if !in || do.ev.kind != evCall || do.ev.call.word != "do" || len(do.ev.call.ops) != 1 {
		return false
	}
	return core.IsWord(toks[at-2]) && toks[at-2].Pos() == do.ev.call.pos && es.doBodyAfter(tree, op.idx, toks, at-2)
}

// literalListTok reports whether v is a list literal as written: evaluated
// and unquoted.
func literalListTok(v core.Value) bool {
	return v.Eval && !v.Quoted && v.Parent.Equal(core.TList)
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
// body), or a body — computed, or a closure the pass compiled from a read or
// a paren — whose one argument site (argSites) is that token (`(do b) add
// 1`, NUR282's single seat).
func (es *EmitState) doBodyAfter(tree map[int]treeEvent, seq int, toks []core.Value, at int) bool {
	ev := tree[seq].ev
	if ev.call.ops[0].kind == opClosure {
		// A literal list, or a body the pass compiled from a def-bound
		// list's read or a paren (`do b`, `do (b)` over `def b (quote
		// […])`) whose site is the token after the word.
		if literalListTok(toks[at+1]) {
			return true
		}
		site, noted := es.closureBodySites[seq]
		return noted && site.Row > 0 && bodyTokenContaining(toks, site) == at+1
	}
	// A computed body: its one argument site is the token after the word.
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
func (es *EmitState) restartSubsts(tree map[int]treeEvent, body []core.Value, tok int, pending []int, pre ...substPlan) ([]substPlan, bool) {
	cands := append([]substPlan(nil), pre...)
	for _, s := range pending {
		ev := tree[s].ev
		if restartRead(ev) || rerunBranch(ev) {
			continue
		}
		if path, ok := parenOf(ev, body, tok); ok && (!wordAt(body, path) || inertBefore(body, tok, path, cands...)) {
			cands = append(cands, substPlan{path: path, span: 1, seq: s})
		} else if sub, whole, ok := es.doBody(ev, body, tok); ok && !whole && inertBefore(body, tok, sub, cands...) {
			cands = append(cands, substPlan{path: sub, span: 2, seq: s})
		} else if run, span, ok := es.callRun(tree, ev, body, tok); ok {
			_, nout := callShape(ev)
			if paren, whole := wholeParen(body, run, span); whole && nout == 1 && inertBefore(body, tok, paren, cands...) {
				cands = append(cands, substPlan{path: paren, span: 1, seq: s})
			} else if inertBefore(body, tok, run, cands...) {
				cands = append(cands, substPlan{path: run, span: span, seq: s, none: nout == 0, run: true})
			}
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
		case !inside && !own && !restartRead(ev) && !rerunBranch(ev) && !(isDo && whole):
			return nil, false
		}
	}
	sort.Slice(kept, func(i, j int) bool { return pathLess(kept[i].path, kept[j].path) })
	return kept, true
}

// wholeParen reports whether the call run at path (span tokens) is every
// token of the paren enclosing it — `(3 add 4)`, an infix call whose run
// opens on its stack operand, not its word (NUR348) — and that paren's path.
// A paren seals the stack off, so a call run over all of it leaves the
// paren's one value exactly as parenOf's call does: the island writes it in
// the paren's place, where the paren is no collection barrier any more.
func wholeParen(body []core.Value, path []int, span int) ([]int, bool) {
	n := len(path)
	if n < 2 || path[n-1] != 0 {
		return nil, false
	}
	toks := body
	for _, at := range path[:n-2] {
		toks, _ = nestedToks(toks[at])
	}
	outer := toks[path[n-2]]
	inner, _ := nestedToks(outer)
	return path[:n-1], core.IsParenExpr(outer) && len(inner) == span
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
			// A paren's shaped apply takes its method from the paren's
			// first token too.
			method := 0
			if ev.kind == evCall && ev.call.dynMethod != nil {
				method = 1
			}
			return path[:i+1], nout == 1 && len(ops) == len(inner)-1+method
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
		at := append(append([]int(nil), level...), i)
		if !barrierFree(toks[i]) && !writtenOver(written, at) && !writtenParen(toks[i], at, written) {
			return false
		}
	}
	return true
}

// writtenParen reports whether t, the paren at path, holds nothing but
// tokens no forward collection passes through once the island has written
// its runs — each a barrier-free token, a run written over it that the VM
// never lets write a dispatching fn value (a paren, a bare word, a call run:
// RestartSubst's NUR297 defer), or such a paren itself — so the paren leaves
// values alone and collects nothing: `((3 add 4))`, `(print "a" 5)` before a
// do's count island (NUR348).
func writtenParen(t core.Value, path []int, written []substPlan) bool {
	inner, nested := nestedToks(t)
	if !nested || !core.IsParenExpr(t) {
		return false
	}
	for j, v := range inner {
		at := append(append([]int(nil), path...), j)
		if !barrierFree(v) && !writtenParen(v, at, written) && !slices.ContainsFunc(written, func(w substPlan) bool {
			return w.covers(at) && (w.span == 1 || w.run) && !w.reach && !w.named && !w.results
		}) {
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
	first := statementFirstSeq(tree, seq, statementStart(body, tok))
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
// up to the first the statement (or a later one) leaves. Each is found where
// the compiled root keeps it, in stack order: a literal it pushes only at the
// program's end (a constant), an earlier result it promoted (its frame slot),
// or one it left on the stack — held of those, the region beneath the
// statement. ok is false for a leading entry the walk cannot place.
//
// Which statement pushed an entry is read from where it was pushed, never
// from where its value came from (NUR335): an event's result from the
// event's position (its seq only when it has none — the pass may record a
// literal's assembly late, `[m] end m (m.f 7)`), a literal from its token,
// and a READ of a def-bound value from the reads (boundReadsBefore) — the
// value itself carries the def's earlier position, so `def k 3 end k (m.f
// 7)` counted the statement's own `k` beneath it, and the island, running
// the statement again, pushed it twice.
func (es *EmitState) rootPreStart(lw *lowerer, tree map[int]treeEvent, residual []core.Value, tok int, start core.SrcPos, firstSeq int) (srcs []RestartSrc, held int, slots []vmSlot, ok bool) {
	if stack, told := es.stackAtStart(tok); told {
		return es.seatStack(lw, stack)
	}
	before, reads, ok := es.boundReadsBefore(tree, residual, start)
	if !ok {
		return nil, 0, nil, false
	}
	for i, rv := range residual {
		pr, produced := es.producedBy[rv.ID]
		if reads[i] {
			if before[rv.ID] == 0 {
				break
			}
			before[rv.ID]--
		} else if own, known := es.statementPushed(tree, rv, start, firstSeq); !known {
			return nil, 0, nil, false
		} else if own {
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
		case !core.IsConcrete(rv) || rv.Carrier || rv.Dynamic:
			return nil, 0, nil, false
		default:
			srcs = append(srcs, RestartSrc{Kind: RestartConst, Val: rv})
		}
	}
	return srcs, held, nil, true
}

// stackAtStart is the stack the pass stepped the island's first token, root
// body token tok, over, when the pass told it (NoteStatementStack): the
// interpreter's own stack there, values the statement then consumes included
// — the residual no longer holds those, so `m end drop (m.f 7)` seated
// nothing and the island's `drop` found an empty stack. It was told at tok
// when a def completed right before it (toldAfter), or else by the `end`
// before tok's statement; an island taking over past the statement's opening
// literal defs (literalDefsBefore) sees that stack too: a def pushes nothing.
func (es *EmitState) stackAtStart(tok int) ([]core.Value, bool) {
	if tok < 0 || tok >= len(es.rootBody) {
		return nil, false
	}
	if stack, told := es.toldAt(tok); told {
		return stack, true
	}
	s := tok
	for s > 0 && !core.IsEnd(es.rootBody[s-1]) {
		s--
	}
	if s == 0 {
		return nil, false
	}
	stack, told := es.rootStmtStacks[es.rootBody[s-1].Pos()]
	return stack, told
}

// seatStack finds each value of the stack at a statement's start where the
// compiled root keeps it (rootPreStart's placement): an event's result in the
// slot it was promoted to, or on the compiled stack beneath the statement
// (held of those, whose producers are slots, bottom first), a literal known
// to the bottom as a constant, and a type node the run resolves by its ID
// (RestartType, as OpPushType does — `Integer end (m.f 7)`). Anything else —
// a carrier, a dynamic value — cannot be seated, and ok is false.
func (es *EmitState) seatStack(lw *lowerer, stack []core.Value) (srcs []RestartSrc, held int, slots []vmSlot, ok bool) {
	slots = []vmSlot{}
	for _, v := range stack {
		if pr, produced := es.producedBy[v.ID]; produced {
			if slot, promoted := lw.promoted[pr.seq]; promoted {
				srcs = append(srcs, RestartSrc{Kind: RestartLocal, Idx: slot + pr.idx})
				continue
			}
			srcs = append(srcs, RestartSrc{Kind: RestartStack, Idx: held})
			slots = append(slots, vmSlot(pr))
			held++
			continue
		}
		if core.IsBareTypeNode(v) && v.ID != "" && !v.Dynamic {
			srcs = append(srcs, RestartSrc{Kind: RestartType, Val: v})
			continue
		}
		if v.Carrier || v.Dynamic || !core.DeepConcrete(v) {
			return nil, 0, nil, false
		}
		srcs = append(srcs, RestartSrc{Kind: RestartConst, Val: v})
	}
	return srcs, held, slots, true
}

// statementPushed reports whether residual entry rv — no read of a def-bound
// value — was pushed by the statement beginning at start or a later one
// (own), and whether that can be told (known). An event's result is its
// event's: written at or after start, or, for an event with no position,
// recorded at or after the statement's first event firstSeq. A literal is its
// token's; a compound the fold re-minted without a position (`{a:1}`) is told
// by the top-level tokens spelling it, when they all stand on one side of
// start.
func (es *EmitState) statementPushed(tree map[int]treeEvent, rv core.Value, start core.SrcPos, firstSeq int) (own, known bool) {
	if pr, produced := es.producedBy[rv.ID]; produced {
		if te, in := tree[pr.seq]; in {
			if p := eventPos(*te.ev); p.Row > 0 {
				return !posAfter(start, p), true
			}
		}
		return pr.seq >= firstSeq, true
	}
	if q := rv.Pos(); q.Row > 0 {
		return !posAfter(start, q), true
	}
	if !isCompoundValue(rv) {
		return false, false
	}
	canon := core.CanonValue(rv)
	pre, in := 0, 0
	for _, t := range es.rootBody {
		q := t.Pos()
		if core.IsWord(t) || !isCompoundValue(t) || q.Row == 0 || core.CanonValue(t) != canon {
			continue
		}
		if posAfter(start, q) {
			pre++
		} else {
			in++
		}
	}
	return in > 0, (pre > 0) != (in > 0)
}

// boundReadsBefore finds the residual's entries that are READS of a
// def-bound value (reads[i]) — the value a root def consumed, found again on
// the residual under the def's own value (its ID and, for a literal, its
// position: a type or another shared value carries one ID wherever it is
// spelled) — and, per value, how many of its leading occurrences were read
// before start (before). The reads' positions are the root's bare reads
// (NoteLocalRead): all before start, or none, decide it; so does a residual
// holding every read, none consumed. Anything else — a `/v` read, whose
// position the pass does not keep, more copies than reads, or reads on both
// sides some of which a word consumed (`m end m (m.f 7)`: the member read
// takes one) — cannot be told, and ok is false.
func (es *EmitState) boundReadsBefore(tree map[int]treeEvent, residual []core.Value, start core.SrcPos) (before map[string]int, reads []bool, ok bool) {
	bound := map[string][]core.SrcPos{}
	for _, te := range tree {
		if d := te.ev.dyn; te.ev.kind == evDynBind && d != nil && d.val.ID != "" {
			bound[d.val.ID] = append(bound[d.val.ID], d.val.Pos())
		}
	}
	reads = make([]bool, len(residual))
	occ := map[string]int{}
	for i, rv := range residual {
		poss, isBound := bound[rv.ID]
		if !isBound {
			continue
		}
		if _, produced := es.producedBy[rv.ID]; !produced && !containsPos(poss, rv.Pos()) {
			continue
		}
		reads[i] = true
		occ[rv.ID]++
	}
	before = map[string]int{}
	receivers := map[core.SrcPos]bool{}
	reachReceivers(es.rootBody, receivers)
	for id, n := range occ {
		if es.valReadNoted[id] {
			return nil, nil, false
		}
		pre, in := 0, 0
		for _, p := range es.rootLocalReads[id] {
			if receivers[p] {
				// A reach's receiver (`m` of `m.f`): the reach takes it.
				continue
			}
			if posAfter(start, p) {
				pre++
			} else {
				in++
			}
		}
		switch {
		case n > pre+in:
			// More copies than reads: one came some other way.
			return nil, nil, false
		case in == 0:
			before[id] = n
		case pre == 0:
			before[id] = 0
		case n == pre+in:
			before[id] = pre
		default:
			return nil, nil, false
		}
	}
	return before, reads, true
}

// reachReceivers notes the positions of the words a reach reads as its
// receiver (`m` of `m.f`), through parens and list literals: a read there is
// the reach's own operand and never stays on the stack.
func reachReceivers(toks []core.Value, at map[core.SrcPos]bool) {
	for _, t := range toks {
		if ri, err := core.AsReach(t); err == nil {
			for _, r := range ri.Receiver {
				if core.IsWord(r) {
					at[r.Pos()] = true
				}
			}
			reachReceivers(ri.Receiver, at)
			continue
		}
		if inner, nested := nestedToks(t); nested {
			reachReceivers(inner, at)
		}
	}
}

// containsPos reports whether ps holds p.
func containsPos(ps []core.SrcPos, p core.SrcPos) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
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
		tok := statementToken(rec.body, stopPos(at.ev))
		if tok < 0 {
			continue
		}
		if at.inLoop {
			// A stop in a loop's body: the island continues the loop from
			// the stop's own iteration (loopContPlan, NUR336).
			outer := func(forTok int) (*landingRestart, bool) { return es.unitLoopOuter(u, rec, tree, seq, forTok) }
			if r, lok := es.loopContPlan(tree, at, seq, rec.body, outer); lok {
				rec.deopts = append(rec.deopts, deoptPoint{seq: seq, slot: -1, start: rec.body[r.loop.forTok].Pos(), token: r.loop.forTok, restart: true, loopCont: r})
				es.noteLeadRead(tree, at.ev)
				continue
			}
		}
		d, ok := es.unitRestartPoint(u, rec, tree, seq, tok)
		if ok && takesBeneath(tree, rec.body, d.start, seq) {
			// The island's frame region would be gone at the stop.
			ok = false
		}
		if !ok {
			// A statement whose run before the stop the island cannot repeat
			// — a word that took a value from beneath it (`drop (q.f 7)`,
			// `swap (q.f 7)`) — may still be taken over from the stop's own
			// token, where the interpreter holds nothing pending
			// (lateStart, NUR336).
			if late, lateOK := lateStart(tree, rec.body, tok, seq); lateOK {
				d, ok = es.unitRestartPoint(u, rec, tree, seq, late)
			}
			if !ok {
				continue
			}
		}
		rec.deopts = append(rec.deopts, d)
		es.noteLeadRead(tree, at.ev)
	}
	// The unit's branch guards (NUR292), as the root's (planGuardRestarts).
	for _, g := range branchGuards(tree) {
		if d, ok := es.guardPoint(u, rec, tree, g); ok && !outsProducedBefore(rec.outOps, statementFirstSeq(tree, g.seq, d.start), nil) {
			rec.deopts = append(rec.deopts, d)
		}
	}
	// Its do calls whose run's count the seat may miss (NUR222), as the
	// root's (planCountRestarts).
	es.notePhantomConsumersIn(rec.frag.events)
	for _, seq := range sortedSeqs(tree) {
		te := tree[seq]
		run := es.dynBodyRun(te.ev)
		if te.inLoop || (!run && !es.countSeat(seq)) {
			continue
		}
		tok, substs, ok := es.countPoint(tree, seq, rec.body)
		if !ok {
			continue
		}
		d := deoptPoint{seq: seq, slot: -1, start: rec.body[tok].Pos(), token: tok, restart: true, count: true, countRun: run, substs: substs}
		if d.start.Row > 0 && !es.deoptDeferred(u, rec, &d, -1) && !outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start), nil) {
			rec.deopts = append(rec.deopts, d)
		}
	}
}

// unitRestartPoint is the statement island of the landing or shaped apply
// event seq in a unit, taking its statement over from body token tok (past
// the defs it opens with, defsBefore): ok is false where an operand is
// deferred past the start or the run before the stop cannot be repeated.
func (es *EmitState) unitRestartPoint(u *emitUnit, rec *fnUnitRec, tree map[int]treeEvent, seq, tok int) (deoptPoint, bool) {
	at := tree[seq]
	from := tok
	tok = defsBefore(tree, rec.body, tok, stopPos(at.ev))
	d := deoptPoint{seq: seq, slot: -1, start: statementStart(rec.body, tok), token: tok, restart: true, frameHeld: true}
	d.leftovers, d.defBound = defLeftovers(tree, rec.body, from, tok)
	if d.start.Row == 0 || es.deoptDeferred(u, rec, &d, -1) {
		return deoptPoint{}, false
	}
	// The residual's earlier results, as the statement's operands
	// (deoptPoint.onFrame): the walk finds each held beneath it, or the
	// unit keeps it in a slot until its RET and the island is not seated.
	d.onFrame = outsBefore(rec.outOps, statementFirstSeq(tree, seq, d.start), d.leftovers, d.onFrame)
	substs, first, reruns := es.restartReruns(tree, at, seq, rec.body, tok)
	if !reruns {
		return deoptPoint{}, false
	}
	d.substs, d.first = substs, first
	return d, true
}

// lateStart is the body token a statement island stopped by event seq may
// take its statement over from when the statement's first token, tok, is
// too early: the top-level token holding the stop, where the interpreter
// holds nothing pending — every event the statement wrote before that token
// was recorded before every event written at or after it, so each completed
// there over what it took (an event consumes only earlier events' results),
// and the values it left are the frame the island seats. ok is false for a
// stop at the statement's first token, one outside the top level, or a
// statement whose events interleave (a word that collected the stop's token
// forward dispatches after it).
func lateStart(tree map[int]treeEvent, body []core.Value, tok, seq int) (int, bool) {
	stop := stopPos(tree[seq].ev)
	late := bodyTokenContaining(body, stop)
	if late <= tok || late >= len(body) || body[late].Pos().Row == 0 {
		return 0, false
	}
	first := statementFirstSeq(tree, seq, body[late].Pos())
	begin := statementStart(body, tok)
	stmtFirst := statementFirstSeq(tree, seq, begin)
	for s, te := range tree {
		p := eventPos(*te.ev)
		// Unplaced in the statement (nothing tells which side it ran), or
		// written before the stop's token and run after an event written at
		// or after it.
		unplaced := p.Row == 0 && s < seq && s >= stmtFirst
		interleaved := p.Row > 0 && !posAfter(begin, p) && posAfter(body[late].Pos(), p) && s >= first
		if unplaced || interleaved {
			return 0, false
		}
	}
	return late, true
}

// lateRootRestart plans a root landing's or shaped apply's statement island
// from the stop's own token (lateStart) over the stack the pass told a paren
// group there opens over (NoteParenStack), each value seated where the
// compiled root keeps it (seatStack): the interpreter holds nothing pending
// there, and every event of the statement before it ran in the compiled code
// and runs no more. None is planned where the pass told no such stack.
func (es *EmitState) lateRootRestart(lw *lowerer, tree map[int]treeEvent, at treeEvent, seq, tok int) {
	late, ok := lateStart(tree, es.rootBody, tok, seq)
	if !ok {
		return
	}
	stack, told := es.parenStackAt(late)
	substs, first, reruns := es.restartReruns(tree, at, seq, es.rootBody, late)
	srcs, held, slots, seated := es.seatStack(lw, stack)
	if !told || !reruns || !seated {
		return
	}
	if lw.landingRestarts == nil {
		lw.landingRestarts = map[int]*landingRestart{}
	}
	lw.landingRestarts[seq] = &landingRestart{token: late, start: es.rootBody[late].Pos(), depth: -1, srcs: srcs, held: held, heldAt: slots, substs: substs, first: first}
	es.noteLeadRead(tree, at.ev)
}

// parenStackAt is the stack the pass told the paren group at root body token
// tok opens over (NoteParenStack): by the token's position or, for a paren
// the engine expanded to its markers, its first inner token's (toldAt's
// rule).
func (es *EmitState) parenStackAt(tok int) ([]core.Value, bool) {
	inner, err := core.AsParenExpr(es.rootBody[tok])
	if err != nil || len(inner) == 0 {
		return nil, false
	}
	stack, told := es.rootParenStacks[inner[0].Pos()]
	return stack, told
}

// takesBeneath reports whether an event of the statement beginning at start,
// written before the top-level token holding the stop event seq, takes an
// earlier statement's value as an operand (`drop (q.f 7)`, `swap (q.f 7)`):
// the frame region the island seats from the statement's start is not there
// at the stop any more (frameIntact), where the stop's own token (lateStart)
// finds the region those events left.
func takesBeneath(tree map[int]treeEvent, body []core.Value, start core.SrcPos, seq int) bool {
	// The stop's statement token is known (statementToken), so the token
	// holding it is too.
	lateAt := body[max(bodyTokenContaining(body, stopPos(tree[seq].ev)), 0)].Pos()
	before := func(p core.SrcPos) bool { return p.Row > 0 && posAfter(start, p) }
	// A def's value is read by its name, which the island reads again.
	bound := map[int]bool{}
	for _, te := range tree {
		if d := te.ev.dyn; te.ev.kind == evDynBind && d != nil && d.srcSeq >= 0 {
			bound[d.srcSeq] = true
		}
	}
	found := false
	for _, te := range tree {
		p := eventPos(*te.ev)
		if p.Row == 0 || before(p) || !posAfter(lateAt, p) {
			continue
		}
		forEachOperand(te.ev, func(op EmitOperand) {
			if src, in := tree[op.idx]; op.kind == opEvent && in && before(eventPos(*src.ev)) && !(op.resIdx == 0 && bound[op.idx]) {
				found = true
			}
		})
	}
	return found
}

// loopCont is a statement island's per-iteration continuation (NUR336): a
// stop inside a counted `for` loop's body on ANY iteration runs the rest of
// that iteration's statement and body, the loop's remaining iterations and
// everything after the loop on the interpreter — the loop's own mark and
// continuation (core.Loop) resumed at the stop's iteration, the values
// its earlier iterations left as the continuation's results. A statement
// island taking the loop's statement over from its first token could run
// only the first iteration, which it would otherwise repeat.
type loopCont struct {
	// elems are the loop body's tokens, which every later iteration runs;
	// after the enclosing body's tokens after the body's list token; forTok
	// the enclosing body's `for` token, which begins the loop's statement.
	elems  []core.Value
	after  []core.Value
	forTok int
	// iterName is the index name the loop binds; beneath is what the island
	// seats beneath the loop (rootLoopOuter, unitLoopOuter), seated by the
	// walk at the loop's statement, and outerDepth its depth there.
	iterName   string
	beneath    *landingRestart
	outerDepth int
}

// loopContPlan plans the per-iteration continuation of the shaped apply
// event seq (at) in body, a stop inside exactly one counted
// `for` loop written `for <count> [<body>]` at the start of its statement:
// the island runs the stop's statement in the loop body from its first token
// (restartRunsReadsOnly, over the body's tokens), each earlier run of it
// written as its value. The loop's body binds nothing (a def or a carried
// store there is a binding the island's iterations could not see), and
// outer reports that nothing stands beneath the loop's statement on the
// interpreter's stack.
func (es *EmitState) loopContPlan(tree map[int]treeEvent, at treeEvent, seq int, body []core.Value, outer func(forTok int) (*landingRestart, bool)) (*landingRestart, bool) {
	// A shaped apply's stop only: the island is its spec's (DynMethodSpec.Loop).
	if len(at.loops) != 1 || at.ev.kind != evCall || at.ev.call.dynMethod == nil {
		return nil, false
	}
	lp := at.loops[0]
	if lp.cond != nil || lp.iterName == "" || len(lp.carried) > 0 || lp.body == nil {
		return nil, false
	}
	binds := false
	walkEvents(lp.body.events, func(ev *EmitEvent) {
		binds = binds || ev.kind == evDynBind || ev.kind == evStore || ev.kind == evBindTwin
	})
	stop := stopPos(at.ev)
	path := tokenPath(body, stop)
	if binds || len(path) < 2 || path[0] < 2 || !literalListTok(body[path[0]]) {
		return nil, false
	}
	forTok := path[0] - 2
	if w, err := core.AsWord(body[forTok]); err != nil || w.Name != "for" || statementToken(body, body[forTok].Pos()) != forTok {
		return nil, false
	}
	// The stop's statement is the body's first: nothing an earlier statement
	// of this iteration left — which the compiled code may push late — lies
	// beneath it.
	beneath, ok := outer(forTok)
	elems, _ := nestedToks(body[path[0]])
	start := statementStart(elems, 0)
	substs, rok := es.restartRunsReadsOnly(tree, seq, elems, 0, core.SrcPos{})
	if !ok || !rok || start.Row == 0 || statementToken(elems, stop) != 0 {
		return nil, false
	}
	return &landingRestart{token: 0, start: start, depth: -1, held: -1, substs: substs,
		loop: &loopCont{elems: elems, after: body[path[0]+1:], forTok: forTok, iterName: lp.iterName, beneath: beneath}}, true
}

// rootLoopOuter plans what a per-iteration island seats beneath the loop
// whose statement begins at the program root's body token forTok: the
// interpreter's stack there — nothing at the program's first token, else the
// stack the pass told (stackAtStart), each value where the compiled root
// keeps it (seatStack). The walk seats its depth at the loop's statement as
// any statement island's (noteRestartDepths).
func (es *EmitState) rootLoopOuter(lw *lowerer, forTok int) (*landingRestart, bool) {
	stack, told := es.stackAtStart(forTok)
	srcs, held, slots, ok := es.seatStack(lw, stack)
	r := &landingRestart{token: forTok, start: statementStart(es.rootBody, forTok), depth: -1, srcs: srcs, held: held, heldAt: slots}
	return r, (told || forTok == 0) && ok && r.start.Row > 0
}

// unitLoopOuter is rootLoopOuter in a fn unit: the frame region beneath the
// loop's statement, which the walk seats with the unit's unnamed params and
// the earlier statements' late values (noteRestartDepth, deoptPoint.lits);
// an operand from before the statement must be held there (onFrame).
func (es *EmitState) unitLoopOuter(u *emitUnit, rec *fnUnitRec, tree map[int]treeEvent, seq, forTok int) (*landingRestart, bool) {
	d := deoptPoint{seq: seq, slot: -1, start: rec.body[forTok].Pos(), token: forTok, restart: true, frameHeld: true}
	ok := d.start.Row > 0 && !es.deoptDeferred(u, rec, &d, -1)
	d.onFrame = outsBefore(rec.outOps, statementFirstSeq(tree, seq, d.start), nil, d.onFrame)
	return &landingRestart{token: forTok, start: d.start, depth: -1, held: -1, onFrame: d.onFrame, lits: d.lits, posOf: d.posOf}, ok
}

// loopOuterIntact reports whether the walk seated o, what a per-iteration
// island seats beneath its loop, exactly over the compiled stack beneath the
// loop (vm, loopCtx.outerVM): its depth measured there, every value it reads
// off that stack still in place.
func (lw *lowerer) loopOuterIntact(o *landingRestart, vm []vmSlot) bool {
	intact := o.seated() && o.depth == len(vm) && slices.Equal(o.beneath, vm)
	for i, slot := range vm {
		intact = intact && !(slot.seq >= 0 && lw.variadic[slot.seq]) && (i >= o.held || i >= len(o.heldAt) || slot == o.heldAt[i])
	}
	return intact && !slices.ContainsFunc(o.onFrame, func(p producer) bool { return !slices.Contains(vm, vmSlot(p)) })
}

// noteLoopRestartDepths seats the per-iteration islands' depth (landingRestart.loop)
// as the walk emits the loop body's event at p: the compiled stack above the
// iteration's start, where the island's statement begins, and whether the
// loop's own compiled stack held nothing beneath it.
func (lw *lowerer) noteLoopRestartDepths(p core.SrcPos) {
	if len(lw.loops) != 1 || p.Row == 0 {
		return
	}
	for _, r := range lw.landingRestarts {
		if r.loop == nil || r.depth >= 0 || posAfter(r.start, p) {
			continue
		}
		r.depth = len(lw.vm)
		if o := r.loop.beneath; o.depth < 0 {
			// No root event of the loop's statement had a position before
			// its body's (a range literal's loop): the loop's own stack
			// beneath it is the statement's.
			saved := lw.vm
			lw.vm = lw.loops[0].outerVM
			lw.noteRestartDepth(o, o.start)
			lw.vm = saved
		}
		if o := r.loop.beneath; r.depth == 0 && lw.loopOuterIntact(o, lw.loops[0].outerVM) {
			r.srcs, r.loop.outerDepth = o.srcs, o.depth
		} else {
			r.unseatable = true
		}
	}
}

// srcsOf is each literal's source, in order.
func srcsOf(lits []seatLit) []RestartSrc {
	out := make([]RestartSrc, 0, len(lits))
	for _, l := range lits {
		out = append(out, l.src)
	}
	return out
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
// slot until its RET, where the island would not find it — unless it is a
// def's leftover the island seats from that slot (deoptPoint.leftovers).
func outsProducedBefore(outs []EmitOperand, firstSeq int, seated []producer) bool {
	for _, op := range outs {
		if op.kind == opEvent && op.idx < firstSeq && !slices.Contains(seated, producer{seq: op.idx, idx: op.resIdx}) {
			return true
		}
	}
	return false
}

// outsBefore is onFrame with every result of a unit's residual that an
// event recorded before firstSeq produced, a def's leftover the island seats
// from its slot aside.
func outsBefore(outs []EmitOperand, firstSeq int, seated, onFrame []producer) []producer {
	for _, op := range outs {
		p := producer{seq: op.idx, idx: op.resIdx}
		if op.kind == opEvent && op.idx < firstSeq && !slices.Contains(seated, p) && !slices.Contains(onFrame, p) {
			onFrame = append(onFrame, p)
		}
	}
	return onFrame
}

// restartAt is the planned statement island of event seq whose depth the
// walk seated, or nil — nil too at the root when the compiled stack there is
// not the residual's results the plan counted (rootPreStart), and in a unit
// whose frame's unnamed params the walk could not place.
func (lw *lowerer) restartAt(seq int) *landingRestart {
	if r := lw.landingRestarts[seq]; r.seated() && lw.heldIntact(r) {
		return r
	}
	return nil
}

// heldIntact reports whether the compiled stack at the stop still holds, at
// the bottom of the frame, the values the island seats from it (held of
// them): the statement ran stack words before the stop, and one that took a
// value from beneath the statement (`(mk) end drop (m.f 7)`) left another in
// its slot.
func (lw *lowerer) heldIntact(r *landingRestart) bool {
	if len(r.onFrame) > 0 {
		return lw.frameIntact(r)
	}
	if r.held <= 0 {
		return true
	}
	want := r.beneath
	if r.heldAt != nil {
		want = r.heldAt
	}
	if len(lw.vm) < r.held || len(want) < r.held {
		return false
	}
	for i := 0; i < r.held; i++ {
		// A variadic region's slot holds a run of any count, which the
		// island cannot seat as the one value it counts (NUR348: a trap
		// program keeps a computed body's run beneath its live read).
		if lw.vm[i] != want[i] || lw.variadic[lw.vm[i].seq] {
			return false
		}
	}
	return true
}

// frameIntact is heldIntact for a unit's island over earlier statements'
// values (landingRestart.onFrame): the compiled frame held each beneath the
// statement at its start, and at the stop it still holds that region as it
// was — so the frame region the island seats is the interpreter's there.
func (lw *lowerer) frameIntact(r *landingRestart) bool {
	for i, slot := range r.beneath {
		if i >= len(lw.vm) || lw.vm[i] != slot || lw.variadic[slot.seq] {
			return false
		}
	}
	// A value the compiled code promoted to a slot is not on the frame.
	return !slices.ContainsFunc(r.onFrame, func(p producer) bool { return !slices.Contains(r.beneath, vmSlot(p)) })
}

// noteRestartDepths seats the compiled stack's depth on every planned
// statement island whose statement begins at or before p, the position of
// the root event whose first op is about to be emitted. In a unit it seats
// the island's prefix there too: the unnamed params the interpreter's frame
// still holds on its stack bottom at the statement's start (deoptPrefix,
// read from their slots), then the frame region.
func (lw *lowerer) noteRestartDepths(p core.SrcPos) {
	for _, m := range []map[int]*landingRestart{lw.landingRestarts, lw.guardRestarts, lw.countRestarts, lw.fitRestarts, lw.callResultRestarts} {
		for _, r := range m {
			lw.noteRestartDepth(r, p)
			if r.loop != nil {
				// What a per-iteration island seats beneath its loop, at
				// the loop's statement.
				lw.noteRestartDepth(r.loop.beneath, p)
			}
		}
	}
}

// noteRestartDepth is noteRestartDepths for one island.
func (lw *lowerer) noteRestartDepth(r *landingRestart, p core.SrcPos) {
	if r.loop != nil || r.depth >= 0 || p.Row == 0 || posAfter(r.start, p) {
		return
	}
	r.depth = len(lw.vm)
	r.beneath = append([]vmSlot(nil), lw.vm...)
	if lw.landingRoot {
		return
	}
	params, ok := lw.deoptPrefix()
	if !ok {
		r.unseatable = true
		return
	}
	frame, lefts, ok := lw.defLeftoverSrcs(r)
	if ok && len(r.lits) > 0 {
		frame, ok = lw.mergeLits(r, frame)
	}
	if !ok {
		r.unseatable = true
		return
	}
	for _, slot := range params {
		r.srcs = append(r.srcs, RestartSrc{Kind: RestartLocal, Idx: slot})
	}
	if len(params) > 0 || len(frame) != r.depth || len(lefts) > 0 {
		r.srcs = append(r.srcs, frame...)
	}
	r.srcs = append(r.srcs, lefts...)
}

// mergeLits seats a unit island's literals (landingRestart.lits) among its
// frame region's entries, frame, by where each was written: a literal stands
// above every entry an earlier token produced and beneath every later one —
// only a word that took the literal could have moved an entry across it,
// and such a literal is no deferred one. ok is false where the frame's
// entries do not split so around a literal, or one's position is unknown.
func (lw *lowerer) mergeLits(r *landingRestart, frame []RestartSrc) ([]RestartSrc, bool) {
	lits := append([]seatLit(nil), r.lits...)
	sort.Slice(lits, func(i, j int) bool { return posAfter(lits[j].pos, lits[i].pos) })
	ok := true
	for i := range lits {
		if pr := lits[i].prod; pr != nil {
			slot, promoted := lw.promoted[pr.seq]
			ok = ok && promoted
			lits[i].src = RestartSrc{Kind: RestartLocal, Idx: slot + pr.idx}
		}
	}
	out := make([]RestartSrc, 0, len(frame)+len(lits))
	k := 0
	for _, src := range frame {
		// An entry an earlier literal was seated above must be written after
		// it, and an entry's own position must be known.
		p := r.posOf[lw.vm[src.Idx].seq]
		ok = ok && p.Row > 0 && (k == 0 || !posAfter(lits[k-1].pos, p))
		for k < len(lits) && posAfter(p, lits[k].pos) {
			out = append(out, lits[k].src)
			k++
		}
		out = append(out, src)
	}
	return append(out, srcsOf(lits[k:])...), ok
}

// defLeftoverSrcs seats a unit island's frame region past the defs its
// statement opens with (landingRestart.leftovers): the compiled frame may
// still hold a value such a def bound, which the interpreter's def took, so
// frame lists every other entry; lefts are the leftovers the compiled code
// promoted to slots instead, which sit above everything the frame held
// before the statement — seated only over a frame region that holds nothing
// else, whose order they cannot break. ok is false for a leftover held
// nowhere.
func (lw *lowerer) defLeftoverSrcs(r *landingRestart) (frame, lefts []RestartSrc, ok bool) {
	onFrame := map[producer]bool{}
	for i := 0; i < r.depth; i++ {
		p := producer(lw.vm[i])
		if p.seq >= 0 && slices.Contains(r.defBound, p) {
			continue
		}
		onFrame[p] = true
		frame = append(frame, RestartSrc{Kind: RestartStack, Idx: i})
	}
	for _, lo := range r.leftovers {
		if onFrame[lo] {
			continue
		}
		slot, promoted := lw.promoted[lo.seq]
		if !promoted || len(frame) > 0 {
			return nil, nil, false
		}
		lefts = append(lefts, RestartSrc{Kind: RestartLocal, Idx: slot + lo.idx})
	}
	return frame, lefts, true
}

// restartAnchor is where root event ev stands in its statement for the
// islands' depth (noteRestartDepths): a guarded branch at its `if` word
// (BranchRecord.CondCheckPos) — its own position is its condition value's,
// which an earlier statement may have written — a paren apply at its lead's
// token (stopPos), for the same reason, and any other event at its own
// position.
func restartAnchor(ev *EmitEvent) core.SrcPos {
	if ev.kind == evBranch && ev.br.condCheckPos.Row > 0 {
		return ev.br.condCheckPos
	}
	return stopPos(ev)
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
	start := statementStart(body, tok)
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
	first := statementFirstSeq(tree, seq, statementStart(body, tok))
	var pending []int
	for s, te := range tree {
		written := eventPos(*te.ev)
		if s >= first && s <= seq && (s != seq || te.ev.kind != evCall || te.ev.call.dynMethod == nil) &&
			(after.Row == 0 || written.Row == 0 || !posAfter(written, after)) {
			pending = append(pending, s)
		}
	}
	sort.Ints(pending)
	return es.restartSubsts(tree, body, tok, pending, es.parenLead(tree, tree[seq].ev, body, tok)...)
}

// parenLead is the island's plan for a paren apply's own lead (NUR336): the
// island writes the value the apply found in the lead's place, placed —
// what the paren had in hand, past any landing that re-stepped the read
// (`(m.f y)` over a named fn of no argument holds its result) — so the
// island neither reads the member again nor lets it collect: a word after
// it the island writes as its value (`y`) meets no collection
// (inertBefore). None for another stop, or a lead no event produced.
func (es *EmitState) parenLead(tree map[int]treeEvent, ev *EmitEvent, body []core.Value, tok int) []substPlan {
	if ev.kind != evCall || ev.call.dynMethod == nil || !ev.call.dynMethod.Paren {
		return nil
	}
	named := ev.call.dynMethod.LeadName != ""
	if !named && methodSeq(ev) < 0 {
		return nil
	}
	path := tokenPath(body, stopPos(ev))
	if len(path) < 2 || path[0] < tok {
		return nil
	}
	if named {
		// A def-bound word: the island writes the value the apply holds,
		// as the name's call treats it (the stop's own, RestartGuard).
		return []substPlan{{path: path, span: 1, seq: methodSeq(ev), run: true, named: true}}
	}
	if lead, span := leadRun(body, path); span > 1 {
		// A `/v` lead (`(m.f/v y)`) is its read's token and the modifier
		// the parser mints after it at the same position: the island writes
		// the value the apply found over both, placed (NUR336).
		return []substPlan{{path: lead, span: span, seq: methodSeq(ev), run: true}}
	}
	return []substPlan{{path: path, span: 1, seq: methodSeq(ev), run: true, reach: es.rawReachLead(tree, methodSeq(ev), body, path)}}
}

// leadRun is the run of tokens a paren's lead at path stands in: tokenPath
// reaches the last token at the lead's position, and a modifier the parser
// mints after a read (`m.f/v`) stands at the read's own — so the run is
// every token at that position, the first its path.
func leadRun(body []core.Value, path []int) ([]int, int) {
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	last := path[len(path)-1]
	first := last
	for first > 0 && toks[first-1].Pos() == toks[last].Pos() {
		first--
	}
	lead := append(append([]int(nil), path[:len(path)-1]...), first)
	return lead, last - first + 1
}

// rawRead reports whether event seq is a member read whose value nothing
// changed before its consumer took it: a `dot` or `get` read with no landing
// noted on it (NoteReStepLanding), whose re-step the compiled code may fire.
func (es *EmitState) rawRead(tree map[int]treeEvent, seq int) bool {
	te, in := tree[seq]
	if !in || te.ev.kind != evCall || (te.ev.call.word != "dot" && te.ev.call.word != "get") {
		return false
	}
	_, landed := es.landingAfter[seq]
	return !landed
}

// noteLeadRead records whether the island of paren apply ev, stopping its
// statement, steps the paren's lead as the interpreter does (leadUnrun): the
// lead is a raw member read (rawRead) — written as its reach, or read again
// with the statement — or no event's value at all.
func (es *EmitState) noteLeadRead(tree map[int]treeEvent, ev *EmitEvent) {
	if ev.kind != evCall || ev.call.dynMethod == nil || !ev.call.dynMethod.Paren || len(ev.call.ops) == 0 {
		return
	}
	if lead := ev.call.ops[0]; ev.call.dynMethod.LeadName == "" && lead.kind == opEvent && !es.rawRead(tree, lead.idx) {
		return
	}
	if es.leadReads == nil {
		es.leadReads = map[int]bool{}
	}
	es.leadReads[ev.seq] = true
}

// rawReachLead reports whether a paren apply's lead, the method event seq,
// is the value its member read left as it is (rawRead), read by the
// evaluated reach token at path (`m.f` of `(m.f 7)`). The interpreter lowers
// that token to a group of its read (expandReach), so a group holding the
// read's value — the value alone, in a reach-lowered group — is the token as
// the interpreter steps it: a fn that takes no argument dispatches there, any
// other waits for the paren's values (RestartSubst.Reach, NUR336).
func (es *EmitState) rawReachLead(tree map[int]treeEvent, seq int, body []core.Value, path []int) bool {
	if !es.rawRead(tree, seq) {
		return false
	}
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	t := toks[path[len(path)-1]]
	ri, err := core.AsReach(t)
	return err == nil && ri.Eval && !t.Quoted
}

// rerunBranch reports whether ev is a branch a statement island may run
// again whole: every event its condition and its arms hold, at any depth, is
// a restartRead or such a branch itself. It binds nothing (an arm's def is
// no read) and runs no user code, so the interpreter's second run of the
// branch's tokens takes the same arm and leaves the same value — `each (if c
// [[1]] [3]) [2]`, whose runtime rematch matches (NUR343).
func rerunBranch(ev *EmitEvent) bool {
	if ev.kind != evBranch {
		return false
	}
	for _, frag := range childFragments(ev) {
		if frag == nil {
			continue
		}
		for i := range frag.events {
			if e := &frag.events[i]; !restartRead(e) && !rerunBranch(e) {
				return false
			}
		}
	}
	return true
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
