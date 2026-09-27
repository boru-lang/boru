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
// the landing is an effect a second run repeats — here, only member reads —
// and the compiled stack at the statement's start holds exactly what the
// interpreter's does there (no operand of the statement or a later one was
// deferred past it, NUR207's accounting). The landing then restarts it: the
// frame region beneath the statement as the resolved prefix, the program's
// tokens from the statement on, and the island's residual is the program's.

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
		if _, landed := es.landingAfter[seq]; (landed && es.landingWord[seq].Name != "") || (te.ev.kind == evCall && te.ev.call.dynMethod != nil) {
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
		if d.start.Row == 0 || es.deoptDeferred(es.units[0], rec, &d, -1) || !restartReruns(tree, at, seq, es.rootBody, tok) {
			continue
		}
		srcs, held, ok := es.rootPreStart(lw, residual, d.start, statementFirstSeq(tree, seq, d.start))
		if !ok {
			continue
		}
		if lw.landingRestarts == nil {
			lw.landingRestarts = map[int]*landingRestart{}
		}
		lw.landingRestarts[seq] = &landingRestart{token: tok, start: d.start, depth: -1, srcs: srcs, held: held}
	}
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
		if (landed && w.Name != "" && (w.Collected || !top[seq] || landingTopToken(rec.body, w.Pos) < 0)) || (te.ev.kind == evCall && te.ev.call.dynMethod != nil) {
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
		if d.start.Row == 0 || es.deoptDeferred(u, rec, &d, -1) || !restartReruns(tree, at, seq, rec.body, tok) ||
			outsProducedBefore(rec.outOps, statementFirstSeq(tree, seq, d.start)) {
			continue
		}
		rec.deopts = append(rec.deopts, d)
	}
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
	r := lw.landingRestarts[seq]
	if r == nil || r.depth < 0 || r.unseatable || (r.held >= 0 && r.depth != r.held) {
		return nil
	}
	return r
}

// noteRestartDepths seats the compiled stack's depth on every planned
// statement island whose statement begins at or before p, the position of
// the root event whose first op is about to be emitted. In a unit it seats
// the island's prefix there too: the unnamed params the interpreter's frame
// still holds on its stack bottom at the statement's start (deoptPrefix,
// read from their slots), then the frame region.
func (lw *lowerer) noteRestartDepths(p core.SrcPos) {
	for _, r := range lw.landingRestarts {
		if r.depth >= 0 || p.Row == 0 || posAfter(r.start, p) {
			continue
		}
		r.depth = len(lw.vm)
		if lw.landingRoot {
			continue
		}
		params, ok := lw.deoptPrefix()
		if !ok {
			r.unseatable = true
			continue
		}
		for _, slot := range params {
			r.srcs = append(r.srcs, RestartSrc{Kind: RestartLocal, Idx: slot})
		}
		for i := 0; len(params) > 0 && i < r.depth; i++ {
			r.srcs = append(r.srcs, RestartSrc{Kind: RestartStack, Idx: i})
		}
	}
}

// treeEvent is one event of the root's tree: the event, whether a loop
// fragment holds it, and the index slots of the counted loops that do.
type treeEvent struct {
	ev        *EmitEvent
	inLoop    bool
	loopSlots []int
}

// rootTreeEvents indexes events and every event of their nested fragments
// by seq, marking the ones a loop's fragment holds.
func rootTreeEvents(events []EmitEvent, inLoop bool) map[int]treeEvent {
	return treeEventsUnder(events, inLoop, nil)
}

// treeEventsUnder is rootTreeEvents under the enclosing loops' index slots.
func treeEventsUnder(events []EmitEvent, inLoop bool, slots []int) map[int]treeEvent {
	out := map[int]treeEvent{}
	for i := range events {
		ev := &events[i]
		out[ev.seq] = treeEvent{ev: ev, inLoop: inLoop, loopSlots: slots}
		inner := slots
		if ev.kind == evLoop {
			inner = append(append([]int(nil), slots...), ev.loop.iterSlot)
		}
		for _, frag := range childFragments(ev) {
			if frag == nil {
				continue
			}
			for seq, te := range treeEventsUnder(frag.events, inLoop || ev.kind == evLoop, inner) {
				out[seq] = te
			}
		}
	}
	return out
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
// again from its first token when event seq (at) stops it: every event the
// compiled code ran before the stop is re-runnable (restartRunsReadsOnly).
// A stop inside a loop's body ran earlier iterations WHOLE — the body's
// events after the stop too — so there every event of the statement's span
// must be re-runnable, the branches and loops that hold them aside
// (restartSpanReruns).
func restartReruns(tree map[int]treeEvent, at treeEvent, seq int, body []core.Value, tok int) bool {
	start := body[tok].Pos()
	if !at.inLoop {
		return restartRunsReadsOnly(tree, seq, start)
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
	return restartSpanReruns(tree, start, end, body, landed)
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

// restartRunsReadsOnly reports whether every event of the root's tree the
// compiled code ran from the statement's start up to event seq — the ones
// recorded in that span, in execution order — is a member read
// (restartRead), so the statement island's second run repeats no effect. A
// shaped apply's own event has not run when it restarts; a landed event
// has, and is read like the others.
func restartRunsReadsOnly(tree map[int]treeEvent, seq int, start core.SrcPos) bool {
	first := statementFirstSeq(tree, seq, start)
	for s, te := range tree {
		if s < first || s > seq || (s == seq && te.ev.kind == evCall && te.ev.call.dynMethod != nil) {
			continue
		}
		if !restartRead(te.ev) {
			return false
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
