package compiler

import "slices"

// The RUNTIME-CHECKED SINGLE VALUE (NUR210's follow-up). A COMPUTED `do`
// body — `do b` over a List param, `do (mk)` over a factory's quoted list —
// leaves 0-or-more values at run time where the check pass models one
// dynamic(Any) out, so recordDynBodyCall records its run as a variadic
// REGION, and the region rules decline every seat that consumes a fixed
// count: a call operand, a list literal's element, a promoted value-def, a
// dead result. That declined the mini-s3 handler shape
//
//	def risky fn [[b:Any store:Any] [Any] [
//	  def ok (do b error [ drop false ])
//	  if ok [ 1 ] [ 0 ]
//	]]
//
// which compiled before the region landed and answers the interpreter's
// value whenever b leaves exactly one (TestStampDynEnvLateArmDrift: its
// units stopped stamping).
//
// Such a seat now takes the run as ONE value under a runtime check instead
// of declining. demoteDynRegion clears the region marks and sets
// eventFlags.dynBodyOne; the call lowers with SigRef/PolyRef.DynBodyOne, and
// the VM seats the run only when it left exactly one value the interpreter
// would not re-step — the one case in which the fixed seat and the
// interpreter's tape agree. Any other run (0 or 2+ values, or a fn value /
// class / active token the interpreter's tape would dispatch) is a designed
// defer at the call's own position (vm:dyn-body-one): loud, and the
// compiled lane's own, never a value seated where the interpreter would
// have placed another.
//
// Only a region that MAY LEAVE A CALLABLE is demoted (dynRegionMayBeFn — a
// body whose tokens are not proven plain data). A proven plain body has a
// static count: one plain value is no region at all (recordDynBodyCall), and
// any other count keeps the region and its declines, since a runtime check
// could only ever defer there. A region seated as a RESIDUAL's entries is
// left to the region rules too: the residual seats a run whole, and where it
// cannot (a value beneath the run, entries above it) the decline is the
// NUR210 witnesses' own answer.

// dynRegionCheckable reports whether fi is a computed `do` body's region, a
// computed error handler's (NUR300), or a strip word's island over a literal
// handler (NUR301), whose run may leave a callable — the one region
// demoteDynRegion may turn into a runtime-checked single value. The island's
// check is its span's (FallbackSpan.CheckOne): `def ok (do b error [drop
// false])` seats the handler's one value as it seated before NUR301 made the
// island a region, and a handler that leaves two defers loudly.
func dynRegionCheckable(fi eventFlags) bool {
	return (fi.dynBodyResult || fi.stripIsland) && fi.variadicRegion && fi.regionMayBeFn
}

// demoteDynRegion turns the checkable region seq into a runtime-checked
// single value (eventFlags.dynBodyOne), clearing the marks every region rule
// reads.
func (es *EmitState) demoteDynRegion(seq int) {
	f := es.eventInfo[seq]
	f.variadicRegion = false
	f.regionMayBeFn = false
	f.dynBodyOne = true
	es.eventInfo[seq] = f
}

// demoteConsumedDynRegions is Finalize's pre-pass: every checkable region an
// event CONSUMES as an operand — a call's or user call's argument, a
// fallback's input, a store's or def's source, a branch condition or value
// arm, a loop bound or carried seed, a rematch window — is demoted before
// any scope is lowered, so the residual planning and the lowering both see
// one fixed value. A fragment's OUT (an arm's or a loop body's residual) is
// not a consuming seat: it keeps the region rules. The walk covers the
// program's events and every unit's, through their nested fragments.
func (es *EmitState) demoteConsumedDynRegions() {
	visit := func(op EmitOperand) {
		if op.kind == opEvent && dynRegionCheckable(es.eventInfo[op.idx]) {
			es.demoteDynRegion(op.idx)
		}
	}
	walkConsumingOperands(es.frames[0], visit)
	for _, rec := range es.fnRecs {
		if rec.frag != nil {
			walkConsumingOperands(rec.frag.events, visit)
		}
	}
	es.settleDemotedStrips()
}

// settleDemotedStrips clears the region marks a strip-input call inherited
// from a region the pre-pass demoted (eventFlags.stripFrom): the checked run
// left exactly the one value the strip consumes, so nothing of it lies
// beneath the strip's result, which is one value — `def ok (do b error
// [drop false])` binds it as it did before NUR301 made the strip a region.
// Ascending seqs settle a strip over a strip, its source recorded first.
func (es *EmitState) settleDemotedStrips() {
	seqs := make([]int, 0, len(es.eventInfo))
	for seq, f := range es.eventInfo {
		if f.stripFrom {
			seqs = append(seqs, seq)
		}
	}
	slices.Sort(seqs)
	for _, seq := range seqs {
		f := es.eventInfo[seq]
		src := es.eventInfo[f.stripSrc]
		if f.variadicRegion && !src.variadicRegion && (src.dynBodyOne || src.stripFrom) {
			f.variadicRegion, f.regionMayBeFn = false, false
			es.eventInfo[seq] = f
		}
	}
}

// walkConsumingOperands calls fn for every consuming operand of events and of
// the events of their nested fragments.
func walkConsumingOperands(events []EmitEvent, fn func(EmitOperand)) {
	for i := range events {
		ev := &events[i]
		forEachConsumingOperand(ev, fn)
		for _, frag := range childFragments(ev) {
			if frag != nil {
				walkConsumingOperands(frag.events, fn)
			}
		}
	}
}

// forEachConsumingOperand is forEachOperand without a branch's or a loop's
// fragment OUTS: the operands the event consumes as values of a fixed count.
func forEachConsumingOperand(ev *EmitEvent, fn func(EmitOperand)) {
	switch ev.kind {
	case evLoop:
		fn(ev.loop.start)
		fn(ev.loop.end)
		fn(ev.loop.step)
		for _, c := range ev.loop.carried {
			fn(c.init)
		}
	case evBranch:
		fn(ev.br.cond)
		fn(ev.br.thenVal)
		fn(ev.br.elsVal)
		for _, c := range ev.br.carried {
			fn(c.init)
		}
	default:
		forEachOperand(ev, fn)
	}
}

// dynBodyOneAt reports whether the call event seq lowers as a runtime-checked
// single value: demoted by the pre-pass, or a checkable region the lowering
// PROMOTES to a frame slot (`def ok (do b)` read again) or drops as dead — the
// two dispositions that store or pop exactly one value, which the check makes
// the run's count.
func (lw *lowerer) dynBodyOneAt(seq int) bool {
	if lw.es == nil {
		return false
	}
	fi := lw.es.eventInfo[seq]
	if fi.dynBodyOne {
		return true
	}
	if !dynRegionCheckable(fi) {
		return false
	}
	if _, prom := lw.promoted[seq]; !prom && !lw.dead[seq] {
		return false
	}
	lw.es.demoteDynRegion(seq)
	return true
}
