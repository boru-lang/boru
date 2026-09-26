package compiler

import core "github.com/boru-lang/boru/core/go"

// prefix_island.go — NUR210 (main's record, 2026-09-26): a code-body word's
// DYN-BODY run, `do (mk)` over a body a fn returned, with inert values beneath
// it or collected into a list literal.
//
// The interpreter's `do` runs a computed body to its residual and splices the
// results back, where they are RE-STEPPED: every value lands above what lies
// beneath, in order, and a trailing fn value applies over it. `9 do (mk)` over
// `[1 2]` is [9 1 2]; over `[5 ([x:Integer] => [x add 1])]` it is [9 6]. The
// recorder models the run as the dyn-body CALL_NATIVE's recorded seat (the
// backstop's variadic mark), and every layout that seated something beneath
// it read the run as one value: the trailing apply's rotation answered
// [1 9 2], `[9 do (mk)]` answered [1 [9 2]] and `[do (mk)]` [1 [2]].
//
// The island re-steps exactly what the interpreter re-steps. lowerEvents opens
// a mark before the run's CHAIN (the dyn-body event and the events producing
// its operands, contiguous) and pushes the inert prefix above it; the run
// lands above the prefix; then OpCallDynMixedFromMark islands [prefix…,
// run…] through the interpreter's own re-step. A list literal over the run
// opens a second, outer mark first and collects the island's results into one
// List (OpMakeListToMark), so the list holds the run's count.

// prefixIsland is the armed plan: one per program, and only when no other
// mark plan owns the frame.
type prefixIsland struct {
	anchor int           // the chain's first event: the marks open before it
	region int           // the dyn-body event whose run is re-stepped
	list   int           // the list literal collecting the island; 0: the program residual
	prefix []EmitOperand // the inert operands beneath the run, pushed after the marks
}

// dynBodyRun reports whether ev is a dyn-body CALL whose results are its
// body's residual (eventFlags.dynBodyRun). A hosted splice is main's #512's,
// admitted only over an empty stack.
func (es *EmitState) dynBodyRun(ev *EmitEvent) bool {
	if ev.kind != evCall || ev.call.hostSplice {
		return false
	}
	f := es.eventInfo[ev.seq]
	return f.dynBodyRun && !f.zeroOut
}

// runChainStart returns the index in events of the first event of the run's
// chain: the dyn-body event at events[at] and, transitively, the events that
// produce its operands. The chain must be one CONTIGUOUS block ending at
// events[at]: anything between the mark and the run would land above the
// mark. Its members are calls, and none reads an operand produced outside the
// block (the mark would index past it). ok is false for any other shape.
func runChainStart(events []EmitEvent, at int) (int, bool) {
	pending := map[int]bool{}
	take := func(ev *EmitEvent) {
		forEachOperand(ev, func(op EmitOperand) {
			if op.kind == opEvent {
				pending[op.idx] = true
			}
		})
	}
	take(&events[at])
	start := at
	for i := at - 1; i >= 0 && len(pending) > 0; i-- {
		ev := &events[i]
		if !pending[ev.seq] || (ev.kind != evCall && ev.kind != evCallUser) {
			return 0, false
		}
		delete(pending, ev.seq)
		take(ev)
		start = i
	}
	return start, len(pending) == 0
}

// islandPrefixOperand resolves one value beneath the run to the inert operand
// the island pushes at its mark: a scalar constant, which re-steps as itself.
// Anything else (an event result, a container, a fn value) declines.
func (es *EmitState) islandPrefixOperand(v core.Value) (EmitOperand, bool) {
	if _, produced := es.producedBy[v.ID]; produced || !core.IsConcrete(v) || v.Parent == nil ||
		!v.Parent.ConformsTo(core.TScalar) || es.fnLikeResidual(v) {
		return EmitOperand{}, false
	}
	op, ok := es.resolveOperand(v)
	return op, ok && op.kind == opConst
}

// planPrefixIsland arms the island (see the file doc) before the lowering
// walks the program: over a list literal whose operands end in a dyn-body
// run, or over the program residual when it ends in one with inert values
// beneath it. Declines whenever another mark plan owns the frame.
func (es *EmitState) planPrefixIsland(lw *lowerer, residual []core.Value) {
	if len(lw.markBefore) > 0 || lw.regionPrefixSeq != 0 || lw.collectAtSeq != 0 || es.markWindowSeq != 0 {
		return
	}
	events := es.frames[0]
	for li := 1; li < len(events); li++ {
		if is, ok := es.listIsland(events, li); ok {
			lw.island = is
			return
		}
	}
	if is, ok := es.residualIsland(events, residual); ok {
		lw.island = is
	}
}

// listIsland is the plan for the list literal at events[li]: its elements are
// inert constants, then the run of the dyn-body event just before it. The
// list's operands name its elements top-first, so the run leads them and the
// prefix trails them, reversed.
func (es *EmitState) listIsland(events []EmitEvent, li int) (*prefixIsland, bool) {
	l, r := &events[li], &events[li-1]
	if l.kind != evCall || !l.call.makeList || !es.dynBodyRun(r) {
		return nil, false
	}
	ops := l.call.ops
	k := 0
	for k < len(ops) && ops[k].kind == opEvent && ops[k].idx == r.seq {
		k++
	}
	if k == 0 {
		return nil, false
	}
	prefix := make([]EmitOperand, 0, len(ops)-k)
	for i := len(ops) - 1; i >= k; i-- {
		if ops[i].kind != opConst || !es.inertConst(ops[i].idx) {
			return nil, false
		}
		prefix = append(prefix, ops[i])
	}
	start, ok := runChainStart(events, li-1)
	if !ok {
		return nil, false
	}
	return &prefixIsland{anchor: events[start].seq, region: r.seq, list: l.seq, prefix: prefix}, true
}

// inertConst reports whether the constant at idx is a scalar, which the
// island re-steps as itself.
func (es *EmitState) inertConst(idx int) bool {
	if idx < 0 || idx >= len(es.consts) {
		return false
	}
	c := es.consts[idx]
	return core.IsConcrete(c) && c.Parent != nil && c.Parent.ConformsTo(core.TScalar)
}

// excuseIslandRegion drops the run the residual island will re-step from the
// force-promotion set, in place, as excusePrefixRegion does for its region:
// a promoted run stores one value where the run's length is the runtime's
// (`9 8 do (mk)` stored one of the run's two values).
func (es *EmitState) excuseIslandRegion(residual []core.Value, forceOrder map[int]bool) {
	if is, ok := es.residualIsland(es.frames[0], residual); ok {
		delete(forceOrder, is.region)
	}
}

// residualIsland is the plan for the program residual [prefix…, run…]: the
// run is the last event's, and at least one inert value lies beneath it.
func (es *EmitState) residualIsland(events []EmitEvent, residual []core.Value) (*prefixIsland, bool) {
	if len(events) == 0 {
		return nil, false
	}
	r := &events[len(events)-1]
	if !es.dynBodyRun(r) {
		return nil, false
	}
	i := len(residual)
	for i > 0 {
		pr, ok := es.producedBy[residual[i-1].ID]
		if !ok || pr.seq != r.seq {
			break
		}
		i--
	}
	if i == 0 || i == len(residual) {
		return nil, false
	}
	prefix := make([]EmitOperand, 0, i)
	for _, v := range residual[:i] {
		op, ok := es.islandPrefixOperand(v)
		if !ok {
			return nil, false
		}
		prefix = append(prefix, op)
	}
	start, ok := runChainStart(events, len(events)-1)
	if !ok {
		return nil, false
	}
	return &prefixIsland{anchor: events[start].seq, region: r.seq, prefix: prefix}, true
}

// openIsland opens the island's marks before its anchor event and pushes the
// inert prefix above them.
func (lw *lowerer) openIsland(ev *EmitEvent) {
	is := lw.island
	if is == nil || lw.depth != 0 || ev.seq != is.anchor {
		return
	}
	pos := eventPos(*ev)
	if is.list != 0 {
		lw.emit(OpStackMark, 0, pos)
	}
	lw.emit(OpStackMark, 0, pos)
	for _, op := range is.prefix {
		lw.pushOperand(op, pos)
	}
}

// islandTopIs reports whether the simulated stack's top is the island's
// window: the prefix's pushes, then the run's seats in order.
func (lw *lowerer) islandTopIs(run []EmitOperand) bool {
	is := lw.island
	n := len(is.prefix) + len(run)
	if len(lw.vm) < n {
		return false
	}
	top := lw.vm[len(lw.vm)-n:]
	for i := range is.prefix {
		if top[i] != nonEventSlot {
			return false
		}
	}
	for j, op := range run {
		if !slotIs(top[len(is.prefix)+j], op) {
			return false
		}
	}
	return true
}

// closeListIsland lowers the island's list literal: the island re-steps the
// window, and the outer mark collects its results into one List. ok is false
// for every event but the plan's list; once the marks are open the list
// cannot take the ordinary layout, so a stack that is not the window declines.
func (lw *lowerer) closeListIsland(ev *EmitEvent) (string, bool) {
	is := lw.island
	if is == nil || is.list == 0 || ev.seq != is.list {
		return "", false
	}
	c := &ev.call
	k := len(c.ops) - len(is.prefix)
	run := make([]EmitOperand, 0, k)
	for i := k - 1; i >= 0; i-- {
		run = append(run, c.ops[i])
	}
	if !lw.islandTopIs(run) {
		return "prefix island: the lowered stack is not the list's run (NUR210)", true
	}
	lw.emit(OpCallDynMixedFromMark, 0, c.pos)
	lw.emit(OpMakeListToMark, 0, c.pos)
	lw.vm = append(lw.vm[:len(lw.vm)-len(is.prefix)-len(run)], vmSlot{seq: ev.seq, idx: 0})
	if slot, prom := lw.promoted[ev.seq]; prom {
		lw.seatStoreName(ev.seq, 0)
		lw.emit(OpStoreLocal, slot, c.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	lw.note()
	return "", true
}

// opsHaveDynBodyRun reports whether any operand is a dyn-body run
// (EmitState.dynBodyRun's event), whose count is the run's — directly, or as
// the result of a branch an arm of which leaves one (`[9 if c [do (mk)]
// [3]]` assembled [1 [9 2]] for [[9 1 2]]).
func (lw *lowerer) opsHaveDynBodyRun(ops []EmitOperand) bool {
	if lw.es == nil {
		return false
	}
	for _, op := range ops {
		if lw.es.runOperand(op, lw.scopeEvents(op), 0) {
			return true
		}
	}
	return false
}

// scopeEvents returns the event list, among the scopes being lowered, that
// holds op's producer (nil when none does).
func (lw *lowerer) scopeEvents(op EmitOperand) []EmitEvent {
	for i := len(lw.scopes) - 1; i >= 0; i-- {
		if eventBySeq(lw.scopes[i], op.idx) != nil {
			return lw.scopes[i]
		}
	}
	return nil
}

// runOperand reports whether op, produced in events, is a dyn-body run or a
// branch result an arm of which is one, through nested branches.
func (es *EmitState) runOperand(op EmitOperand, events []EmitEvent, depth int) bool {
	if op.kind != opEvent || depth > 8 {
		return false
	}
	if f := es.eventInfo[op.idx]; f.dynBodyRun && !f.zeroOut {
		return true
	}
	ev := eventBySeq(events, op.idx)
	if ev == nil || ev.kind != evBranch {
		return false
	}
	b := ev.br
	return (b.hasThenOut && b.then != nil && es.runOperand(b.thenOut, b.then.events, depth+1)) ||
		(b.hasElsOut && b.els != nil && es.runOperand(b.elsOut, b.els.events, depth+1))
}
