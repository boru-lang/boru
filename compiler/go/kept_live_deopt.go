package compiler

import (
	"slices"

	core "github.com/boru-lang/boru/core/go"
)

// The LIVE-READ DEOPT (the NUR333/NUR334 remainders). A read seated live
// after a computed keep-defs body (NoteLiveRead's kept arms, NoteValReadLive)
// reads the registry the body installed into, and the statement it sits in
// was compiled for the value the model guessed: data of the pre-body type,
// the one result the lookup pushes. The body's tokens exist only at run time,
// so the binding may turn out to be anything:
//
//   - a fn or a class the bare read DISPATCHES (`quote [def x g/v]` — `x` is
//     the interpreter's call of g), or an active token it SPLICES (`quote
//     [def y word [1 2]]` — `y` is 1 2);
//   - an active token the `/v` read delivers as data, which a later step
//     re-steps where the compiled code keeps it (a paren's result, a def of
//     it, a fn's result);
//   - a value of another type, whose dispatch the compiled statement lays out
//     where the interpreter's tape holds more (`do (mk) end x add 1` over
//     `def x [1 2]`: the run's no-match has no layout to raise from at the
//     root, whose tape beneath holds the body's gradual result).
//
// None of these is a shape the compiled statement can take, and all of them
// are what the interpreter's step does with the binding. So the read's
// statement is a deopt point (OpDeoptIfFn, NUR123's machinery) whose value
// is the registry binding itself (DeoptSpec.Live): tested where the
// statement begins, before any of its effects, it hands the statement — and
// the rest of the unit, or of the program at the root — to the interpreter
// over the frame region when the binding is one of the above
// (DeoptSpec.LiveHot), and costs the test otherwise. A read whose statement
// no island can take over (its start is deferred, the unit's names cannot be
// bound registry-visibly) keeps the lookup's own loud defer.

// keptLiveRead is one read seated live after a computed keep-defs body: the
// read value's identity, the name, the read's position, whether it is the
// `/v` spelling, and the type the statement was compiled for (nil: the
// model is a type value, which no dispatch past the read specialises).
type keptLiveRead struct {
	id    string
	name  string
	pos   core.SrcPos
	ref   bool
	model *core.Type
}

// noteKeptLiveRead records the kept live read seated at event seq, whose
// value v the seating left as the model's carrier.
func (es *EmitState) noteKeptLiveRead(seq int, v core.Value, name string, pos core.SrcPos, ref bool) {
	if es.keptLiveReads == nil {
		es.keptLiveReads = map[int]keptLiveRead{}
	}
	var model *core.Type
	if v.Parent != nil && !v.Parent.ConformsTo(core.TType) {
		model = v.Parent
	}
	es.keptLiveReads[seq] = keptLiveRead{id: v.ID, name: name, pos: pos, ref: ref, model: model}
}

// keptLiveSeqs lists the kept live reads among events and the branch arms
// and loop bodies they hold, in event order.
func (es *EmitState) keptLiveSeqs(events []EmitEvent) []int {
	var out []int
	for i := range events {
		ev := &events[i]
		if _, kept := es.keptLiveReads[ev.seq]; kept && ev.kind == evCall && ev.call.live {
			out = append(out, ev.seq)
		}
		for _, f := range childFragments(ev) {
			if f != nil {
				out = append(out, es.keptLiveSeqs(f.events)...)
			}
		}
	}
	return out
}

// livePoint is the deopt point of the kept live read at seq, carrying it.
func (es *EmitState) livePoint(d deoptPoint, seq int) deoptPoint {
	r := es.keptLiveReads[seq]
	d.live = &r
	d.slot = -1
	return d
}

// planKeptLiveDeopts plans a fn unit's live-read points: one per kept live
// read among the unit's own events whose statement an island can take over
// (livePointAt). It reports how many it added.
func (es *EmitState) planKeptLiveDeopts(u *emitUnit, rec *fnUnitRec) int {
	n := 0
	for _, seq := range es.keptLiveSeqs(rec.frag.events) {
		ci, direct := es.deoptConsumer(rec, es.keptLiveReads[seq].id, seq, -1)
		if d, ok := es.livePointAt(u, rec, seq, ci, direct); ok {
			rec.deopts = append(rec.deopts, d)
			n++
		}
	}
	return n
}

// livePointAt places the live-read point of the read at seq, whose value
// event ci consumes first (-1: none; direct: as an operand). The point runs
// where the lowering flushes it (emitDeoptsBefore): before the first event
// of the frame at or after its start, in event order (testIndex). The start
// is where the fn units' rules place the read's statement
// (deoptStatementStart; the read's own top-level token when they place
// none), moved back over what the interpreter holds pending there
// (pendingStart). It is served only when the test runs before the read
// (the read's own event is at or after the test) and after every event
// before it the island's tokens cannot be told to hold (unplacedBeforeRead),
// nothing written at or after the start ran before the test (the island
// would run it again), the
// token before the start is no word the pass ran without an event of its
// own there (silentWordBefore), and the compiled stack at the test is the
// interpreter's at the start (deoptDeferred). ok is false otherwise. An
// event between the test and the read that rebinds the name leaves the test
// a stale binding, which misses an island (the lookup's own loud defer) or
// takes a spurious one — the interpreter's answer either way, never a wrong
// one.
func (es *EmitState) livePointAt(u *emitUnit, rec *fnUnitRec, seq, ci int, direct bool) (deoptPoint, bool) {
	r := es.keptLiveReads[seq]
	tok := statementToken(rec.body, r.pos)
	if tok < 0 {
		return deoptPoint{}, false
	}
	d, ok := es.deoptStatementStart(rec, seq, r.name, r.pos, ci, direct)
	if !ok {
		t := bodyTokenContaining(rec.body, r.pos)
		d = deoptPoint{seq: seq, slot: -1, name: r.name, pos: r.pos, start: rec.body[t].Pos()}
	}
	events := rec.frag.events
	if start := es.pendingStart(events, seq, rec.body, rec.body[tok].Pos(), d.start); start != d.start {
		d.start, d.atPush = start, false
	}
	k := testIndex(events, d.start)
	if d.token = bodyTokenAt(rec.body, d.start); d.token < 0 || (d.token > tok && silentWordBefore(rec.body, events[:k], d.token)) ||
		!liveReadAfter(events, k, seq) || unplacedBeforeRead(events, k, seq) || writtenBeforeTest(events, k, d.start) || es.deoptDeferred(u, rec, &d, ci) {
		return deoptPoint{}, false
	}
	return es.livePoint(d, seq), true
}

// testIndex is the index of the frame's event a point starting at start is
// tested before — emitDeoptsBefore's rule: the first at or after start, or
// with no position; len(events) when none is (the walk's end flushes it).
func testIndex(events []EmitEvent, start core.SrcPos) int {
	for i := range events {
		if p := eventPos(events[i]); p.Row == 0 || !posAfter(start, p) {
			return i
		}
	}
	return len(events)
}

// walkEvents visits each event of events and, depth first, of the branch
// arms, conditions and loop bodies they hold.
func walkEvents(events []EmitEvent, fn func(*EmitEvent)) {
	for i := range events {
		fn(&events[i])
		for _, f := range childFragments(&events[i]) {
			if f != nil {
				walkEvents(f.events, fn)
			}
		}
	}
}

// pendingStart moves start back, within the statement beginning at stmt,
// to the frame's event holding the read at seq when that is lowered before
// the test (a branch whose arm reads it, standing at its condition), and
// over what the frame's events lowered after the test hold pending: an
// event of theirs written before start — a word collecting forward over the
// read, an infix word whose stack operand is written before it, a branch
// or a loop whose word is — from the token its own tokens begin at
// (eventToken), and a literal one of them takes, written before start. The
// compiled code pushes such a literal when its consumer runs, so it is on
// neither lane's stack at the test, and the island's tokens push it again.
// The test moves with the start, so the walk runs to a fixed point.
func (es *EmitState) pendingStart(events []EmitEvent, seq int, body []core.Value, stmt, start core.SrcPos) core.SrcPos {
	first := bodyTokenContaining(body, stmt)
	back := func(p core.SrcPos, t int) bool {
		if p.Row == 0 || posAfter(stmt, p) || !posAfter(start, body[max(t, first)].Pos()) {
			return false
		}
		start = body[max(t, first)].Pos()
		return true
	}
	for moved := true; moved; {
		k := testIndex(events, start)
		moved = false
		for i := range events[:k] {
			if liveReadAfter(events[i:i+1], 0, seq) && back(eventPos(events[i]), eventToken(body, &events[i])) {
				moved = true
			}
		}
		walkEvents(events[k:], func(ev *EmitEvent) {
			if p := eventPos(*ev); back(p, eventToken(body, ev)) {
				moved = true
			}
			forEachOperand(ev, func(op EmitOperand) {
				if op.kind == opConst && op.idx >= 0 && op.idx < len(es.consts) {
					if p := es.consts[op.idx].Pos(); back(p, bodyTokenContaining(body, p)) {
						moved = true
					}
				}
			})
		})
	}
	return start
}

// eventToken is the top-level body token event ev's own tokens begin at: the
// one holding it, for an event nested inside a list or a paren; else a
// call's word; a def's binding word, the token before the NAME it stands
// at; a branch's `if` (its condition check position — it stands at its
// condition), or the nearest `if` or `case` within four tokens before it (a
// `case` is a chain of branches); a loop's word, the nearest `for`, `while`
// or `loop` within four tokens before the count it stands at. A branch or a
// loop whose word is not found, and any other kind (an island, a trap, a
// store — none known to stand at its first token), begins at the body's
// first token, which the caller clamps to the statement's start: the whole
// statement the island's, never a token too late.
func eventToken(body []core.Value, ev *EmitEvent) int {
	t := bodyTokenContaining(body, eventPos(*ev))
	if t >= 0 && bodyTokenAt(body, eventPos(*ev)) != t {
		// Nested in a top-level token — a list, a paren — which holds the
		// word it began at as well.
		return t
	}
	switch ev.kind {
	case evDynBind, evBindTwin:
		return max(t-1, 0)
	case evBranch:
		if ev.br != nil && ev.br.condCheckPos.Row > 0 {
			return bodyTokenContaining(body, ev.br.condCheckPos)
		}
		return wordBefore(body, t, "if", "case")
	case evLoop:
		return wordBefore(body, t, "for", "while", "loop")
	case evCall, evCallUser:
		return t
	}
	return 0
}

// wordBefore is the nearest body token at or within four before t that is
// one of the words, or 0 when none is.
func wordBefore(body []core.Value, t int, words ...string) int {
	for k := t; k >= 0 && k >= t-4; k-- {
		if w, err := core.AsWord(body[k]); err == nil && slices.Contains(words, w.Name) {
			return k
		}
	}
	return 0
}

// liveReadAfter reports whether the read at seq is lowered at or after the
// frame's event k — itself, or inside one of the events from k on — so the
// test runs before it.
func liveReadAfter(events []EmitEvent, k, seq int) bool {
	found := false
	walkEvents(events[k:], func(ev *EmitEvent) {
		found = found || ev.seq == seq
	})
	return found
}

// unplacedBeforeRead reports whether a frame event from k on, before the one
// holding the read at seq, has no position: the test runs before it, and
// nothing tells whether the island's tokens from the start run it again.
func unplacedBeforeRead(events []EmitEvent, k, seq int) bool {
	for i := k; i < len(events) && !liveReadAfter(events[i:i+1], 0, seq); i++ {
		if eventPos(events[i]).Row == 0 {
			return true
		}
	}
	return false
}

// silentWordBefore reports whether the body token before token t is a word
// no event of ran (before the test) stands at: a word the pass folded or
// consumed whole, whose operand t may be — `word` collecting a splice's
// body. A read the pass met in a spliced word's tokens carries the
// position of the word's definition, not of its use, so its start lands on
// the definition's list; an island from there ran the program's tail twice
// (the sweep's for-each splices, `def w word [… for-each (mk) … end size acc]
// w`: [3 3] for [3]). A word with an event there ran as a call of its own
// (`drop t`), and one that runs after the test is pendingStart's.
func silentWordBefore(body []core.Value, ran []EmitEvent, t int) bool {
	if !core.IsWord(body[t-1]) {
		return false
	}
	for i := range ran {
		if bodyTokenContaining(body, eventPos(ran[i])) == t-1 {
			return false
		}
	}
	return true
}

// writtenBeforeTest reports whether an event lowered before the frame's
// event k — one of those events, or inside one — is written at or after
// start: the island from start would run it again.
func writtenBeforeTest(events []EmitEvent, k int, start core.SrcPos) bool {
	ran := false
	walkEvents(events[:k], func(ev *EmitEvent) {
		p := eventPos(*ev)
		ran = ran || (p.Row > 0 && !posAfter(start, p))
	})
	return ran
}

// dropLivePoints is points without the live-read ones: the unit's islands
// could not be served with them (planDeopts retries without).
func dropLivePoints(points []deoptPoint) []deoptPoint {
	out := points[:0]
	for _, d := range points {
		if d.live == nil {
			out = append(out, d)
		}
	}
	return out
}

// planRootLiveReads plans the program root's live-read points: where the fn
// units' rules place the read's statement start as a program token, no
// operand of the statement is deferred past it, and every residual entry
// before it is held on the compiled stack beneath it (rootHeldBeneath) —
// the compiled stack at the test is the interpreter's there — the island
// runs the program from that token on (a root point's residual is the
// program's).
func (es *EmitState) planRootLiveReads(lw *lowerer, residual []core.Value) {
	if len(es.keptLiveReads) == 0 || es.trapAt != 0 || len(es.rootBody) == 0 {
		return
	}
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}, body: es.rootBody, localReads: es.rootLocalReads}
	tree := rootTreeEvents(es.frames[0], false)
	for _, seq := range es.keptLiveSeqs(es.frames[0]) {
		ci, direct := rootReadConsumer(es.frames[0], es.keptLiveReads[seq].name, seq, 0, lw.promoted)
		d, ok := es.livePointAt(es.units[0], rec, seq, ci, direct)
		if !ok || !es.rootHeldBeneath(lw, tree, residual, d.start, seq) {
			continue
		}
		lw.deopts = append(lw.deopts, d)
		lw.deoptTable = &lw.p.Deopts
	}
}

// rootHeldBeneath reports whether every program residual entry the root
// holds before the statement beginning at start (rootPreStart, the
// statement's first event no later than seq) is on the compiled stack
// beneath it: a deopt island's prefix is that stack, where a statement
// island's may also seat a slot or a constant.
func (es *EmitState) rootHeldBeneath(lw *lowerer, tree map[int]treeEvent, residual []core.Value, start core.SrcPos, seq int) bool {
	srcs, _, _, ok := es.rootPreStart(lw, tree, residual, start, statementFirstSeq(tree, seq, start))
	if !ok {
		return false
	}
	for _, src := range srcs {
		if src.Kind != RestartStack {
			return false
		}
	}
	return true
}
