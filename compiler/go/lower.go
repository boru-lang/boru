package compiler

import (
	"maps"

	core "github.com/boru-lang/boru/core/go"
)

// The bytecode lowerer — the second half of the compile pass. EmitState
// (emit.go) RECORDS a classified event trace during the check run; the
// lowerer here LINEARISES that trace into a Program's instruction stream,
// walking a simulated stack of producing-event sequence numbers and emitting
// the pushes, SWAPs, calls, branches and loops the VM executes. Finalize
// (emit.go) is the bridge that drives it. Split out of emit.go purely for
// navigability; both halves are package eng and share no state beyond the
// EmitEvent trace and the *EmitState pools the lowerer reads.

// maxLowerDepth bounds the lowerFragment recursion over nested branch / loop
// bodies. It sits above the parser's maxParseNestingDepth so a program the
// parser accepted is never spuriously refused here; the margin only guards an
// event tree assembled outside the parser. Exceeding it returns a refusal
// reason (Finalize then falls back to the interpreter), never a crash.
const maxLowerDepth = 12000

// lowerLoop lowers a counted/range for:
//
//	…step end start…  FOR_SETUP slot   ; pops start, end, step
//	head: FOR_NEXT -> end_pc           ; bind iterator or exit
//	…body…                             ; net ≤1 value per iteration
//	JMP -> head                        ; the back-edge
//	end_pc:
//
// The loop's stack contribution is variadic; the simulated stack
// carries one marker entry flagged in lw.variadic so only the
// program residual may absorb it. break/continue inside the body
// jump to end_pc / head via the lowerer's loop-context stack.
func (lw *lowerer) lowerLoop(ev *EmitEvent) string {
	lp := ev.loop
	// Operand layout for FOR_SETUP: start on top, then end, then
	// step. start/step are consts (RecordLoop enforced); the end may
	// be a computed value already on top of the simulated stack — a
	// SWAP threads it under the step push.
	if lp.end.kind == opEvent {
		if lw.variadic[lp.end.idx] {
			return "loop results as a loop bound (Stage 2)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], lp.end) {
			return "for: count is not on top of the stack"
		}
		lw.pushOperand(lp.step, lp.pos) // [end step]
		lw.emit(OpSwap, 0, lp.pos)      // [step end]
		lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
	} else {
		lw.pushOperand(lp.step, lp.pos)
		lw.pushOperand(lp.end, lp.pos)
	}
	lw.pushOperand(lp.start, lp.pos)
	lw.emit(OpForSetup, lp.iterSlot, lp.pos)
	lw.vm = lw.vm[:len(lw.vm)-3] // start, end, step consumed
	// Seed the loop-carried def slots with their pre-loop values — once,
	// before the first FOR_NEXT, so a zero-iteration loop leaves each cell
	// at its pre-loop value (the "loop may run zero times" join). An
	// event-sourced init pops off the sim top (reverse production order —
	// orderedCarried; a promoted producer was already rewritten to a local
	// operand); any other layout refuses, a sound interpreter fallback.
	for _, c := range orderedCarried(lp.carried) {
		if c.init.kind == opEvent {
			if lw.variadic[c.init.idx] {
				return "for: carried init is a variadic result (Stage 2)"
			}
			if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], c.init) {
				return "for: carried init is not on top of the stack"
			}
			lw.emit(OpStoreLocal, c.slot, lp.pos)
			lw.vm = lw.vm[:len(lw.vm)-1]
			continue
		}
		lw.pushOperand(c.init, lp.pos)
		lw.emit(OpStoreLocal, c.slot, lp.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	head := len(*lw.code)
	fn := lw.emit(OpForNext, 0, lp.pos)
	lw.loops = append(lw.loops, loopCtx{nextPC: head})
	// A CONDITION loop (RecordWhile, `while [cond] [body]`): the condition's
	// one value is tested at the head of every iteration — falsy jumps to the
	// FLOW_BREAK placed past the back-edge, which pops the loop frame and
	// trims the round as a `break` from a callee would. The count is
	// unbounded, so FOR_NEXT's own exit is never taken.
	condExit := -1
	if lp.cond != nil {
		if reason := lw.lowerFragment(lp.cond, &lp.condOut, false, lp.pos); reason != "" {
			lw.loops = lw.loops[:len(lw.loops)-1]
			return reason
		}
		condExit = lw.emit(OpJmpIfFalse, 0, lp.pos)
	}
	var out *EmitOperand
	if lp.hasBodyOut {
		out = &lp.bodyOut
	}
	// A multi-out body (net drivers) allows the residualN>1 variadic
	// reconciliation; its per-iteration values accumulate across iterations.
	reason := lw.lowerFragment(lp.body, out, lp.multiOut, lp.pos)
	lw.loops = lw.loops[:len(lw.loops)-1]
	if reason != "" {
		return reason
	}
	lw.emit(OpJmp, head, lp.pos)
	if condExit >= 0 {
		(*lw.code)[condExit].Arg = int32(len(*lw.code))
		lw.emit(OpFlowBreak, 0, lp.pos)
	}
	// FOR_NEXT's own exit target. A `break` in the body no longer patches a
	// hole here: it emits OpFlowBreak, which reads this very Arg back off the
	// FOR_NEXT at run time (vmLoop.exitPC) — see lowerBreak.
	(*lw.code)[fn].Arg = int32(len(*lw.code))
	// A value-producing loop contributes N (variadic) values to the simulated
	// stack; a SIDE-EFFECT loop (!hasBodyOut — body nets 0 per iteration) leaves
	// NOTHING. Mirrors RecordLoop's variadicResult/zeroOut split: keep them
	// consistent or a unit's residual reconciliation and its stack simulation
	// disagree on the loop's arity.
	if lp.hasBodyOut {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.variadic[ev.seq] = true
	}
	lw.note()
	return ""
}

// orderedCarried orders a loop's carried-slot inits for lowering:
// event-sourced inits pop off the simulated stack top-first (reverse
// production order), then the freely re-pushable const/local/type inits
// in registration order. Small n — a simple insertion sort avoids an
// import in this file.
func orderedCarried(carried []carriedInit) []carriedInit {
	if len(carried) < 2 {
		return carried
	}
	evs := make([]carriedInit, 0, len(carried))
	rest := make([]carriedInit, 0, len(carried))
	for _, c := range carried {
		if c.init.kind == opEvent {
			evs = append(evs, c)
		} else {
			rest = append(rest, c)
		}
	}
	for i := 1; i < len(evs); i++ {
		for j := i; j > 0 && evs[j].init.idx > evs[j-1].init.idx; j-- {
			evs[j], evs[j-1] = evs[j-1], evs[j]
		}
	}
	return append(evs, rest...)
}

// lowerStore lowers a recorded loop-carried def REBIND: the source value is
// stored into the name's carried frame slot (OpStoreLocal pops the top), so
// the store runs exactly when its recording site runs — inside a branch arm
// only when that arm is taken, skipped by break/continue exactly as the
// interpreter skips the def. An event source must sit on the sim top (the
// def site immediately follows the producing dispatch; a promoted producer
// was rewritten to a local operand); any other layout refuses — a sound
// interpreter fallback, never a wrong store.
// lowerResidentBind emits an ARM-RESIDENT twin's install at its def site
// inside a per-invocation unit (§6.5's each-body recovery — the event was
// stamped by AdoptResidentTwins): the op executes once per element with
// the RUNTIME value, installing through the interpreter's own installer
// and riding no unwind trail. The value operand mirrors lowerDynBind's
// resolution, with the peek/pop split GlobalBindSpec pioneered: a live
// sim-top producer is PEEKED in place (its downstream readers are
// untouched — the interpreter's def consumes nothing the compiled model
// still needs), a promoted local or param re-pushes a copy the op pops,
// and an inert literal bakes unpooled. Any other provenance refuses the
// whole program — the sound interpreter fallback, never a wrong install.
func (lw *lowerer) lowerResidentBind(d *emitDynBind) string {
	if d.undef || d.typeInstall {
		// The two operand-less halves: the TEARDOWN pops the name's live
		// binding per element, and a TYPE install re-installs its captured
		// twin BODY per element (the expression was evaluated once, at
		// compile time — AdoptResidentTwins proved it element-independent
		// before stamping — while the NODE mints per element). Neither
		// touches the stack.
		idx := len(lw.p.ResidentBinds)
		lw.p.ResidentBinds = append(lw.p.ResidentBinds, ResidentBindSpec{
			Name: d.name, Twin: d.residentTwin,
			Undef: d.undef, TypeInstall: d.typeInstall,
		})
		lw.emit(OpBindResident, idx, d.pos)
		return ""
	}
	pop, pushCopy := false, false
	src := d.src
	switch {
	case d.srcSeq >= 0 && len(lw.vm) > 0 && lw.vm[len(lw.vm)-1].seq == d.srcSeq &&
		lw.vm[len(lw.vm)-1].idx == 0 && !lw.variadic[d.srcSeq]:
		// The computed value is on the sim top. A LIVE source is peeked in
		// place (its downstream readers consume it later); a DEAD one was
		// kept only for this bind (collectResidentBindConsumes suppressed
		// the producer's drop) — the install consumes it, the interpreter's
		// own def semantics.
		pop = lw.dead[d.srcSeq]
	case d.srcSeq >= 0:
		slot, ok := lw.promoted[d.srcSeq]
		if !ok {
			return "arm-resident def `" + d.name + "` of unpromoted computed value"
		}
		src, pop, pushCopy = localOperand(slot), true, true
	case src.kind == opLocal:
		pop, pushCopy = true, true
	case src.kind == opNone && core.IsInertConst(d.val):
		src, pop, pushCopy = ConstOperand(lw.es.internUnpooled(d.val)), true, true
	default:
		return "arm-resident def `" + d.name + "` of unknown provenance"
	}
	if pushCopy {
		lw.pushOperand(src, d.pos)
	}
	idx := len(lw.p.ResidentBinds)
	lw.p.ResidentBinds = append(lw.p.ResidentBinds, ResidentBindSpec{
		Name: d.name, Twin: d.residentTwin, Pop: pop,
	})
	lw.emit(OpBindResident, idx, d.pos)
	if pop {
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	return ""
}

// lowerDynBind emits the registry-visible twin of a `def` whose name some
// OpLookupDynScope reads (the DynScopeNames set): push the bound value's
// operand, then OpBindDynScope pops it into r.Defs under the name (the VM
// records the prior depth; the frame's RET truncates back — the
// interpreter's def-cleanup discipline). A def of any other name lowers to
// nothing here — its value flows by provenance exactly as before.
func (lw *lowerer) lowerDynBind(ev *EmitEvent) string {
	d := ev.dyn
	if d.residentTwin >= 0 {
		return lw.lowerResidentBind(d)
	}
	if !d.bindsValue() {
		// An unstamped operand-less event — a teardown whose var pair the
		// bridge declined, a type install outside an adopted unit, or either
		// on the default lane — lowers to nothing: neither binds a runtime
		// value, so neither the dyn-scope nor the global write-back arms may
		// fire.
		return ""
	}
	needDyn := lw.es != nil && (lw.es.dynEnv || lw.deoptNames[d.name] || (lw.es.dynScopeNames != nil && lw.es.dynScopeNames[d.name]))
	// A ROOT-unit def of a NON-concrete value additionally needs the
	// cross-request write-back (OpBindGlobal): its binding persists past the
	// run via keep-on-compile, and the kept check-pass value is a CARRIER —
	// the next request (or any interpreter read) would resolve a type
	// literal where the interpreter binds the runtime value (`def h
	// (Model.new …)` then `Model.stop h` raised model_bad_handle). A
	// concrete bound value IS the runtime value (const-fold parity), and a
	// bare type node (`def x None`) is self-representing in both engines —
	// each keeps today's faithful binding with no op emitted.
	needGlobal := lw.es != nil && d.root && !core.IsConcrete(d.val) && !core.IsBareTypeNode(d.val)
	if !needDyn && !needGlobal {
		// DynEnv mode (a dynamic code body compiled — tryRecordDynBody)
		// widens to EVERY def: the body's runtime sub-run may read any name,
		// so all bindings must be registry-visible, as under the interpreter.
		return ""
	}
	// The write-back's fast path: a `def` binds IMMEDIATELY after its value
	// event, so the value is (almost always) live on top of the sim — peek it
	// in place (OpBindGlobal never pops) and every downstream consumer, the
	// residual accounting included, is untouched. This covers producers the
	// promotion machinery cannot seat (a branch merge, `def r (if …)`), and
	// costs one instruction. A variadic top (a loop collect) has no single
	// value to peek — it falls through to the resolved-source path below.
	fastGlobal := needGlobal && d.srcSeq >= 0 && len(lw.vm) > 0 &&
		lw.vm[len(lw.vm)-1].seq == d.srcSeq && lw.vm[len(lw.vm)-1].idx == 0 &&
		!lw.variadic[d.srcSeq]
	src := d.src
	if needDyn || !fastGlobal {
		switch {
		case d.srcSeq >= 0 && lw.variadic[d.srcSeq] && d.spliceDepth >= 0 && needGlobal:
			// (needDyn is irrelevant here: an S5 read resolves via
			// OpLookupDynScope against the registry slot the splice bind
			// SetAt-writes — registry-visible without an OpBindDynScope.)
			// The S5 first-value loop bind: the bound value is the region's
			// stack-DEEPEST entry, live at a STATIC depth below the region
			// top (SplitLoopRegionBind gated on the static count). Bind it
			// in place and splice it out — the interpreter's pending forward
			// collects the first-arrived value and the rest spill. The sim's
			// variadic marker stays: the region minus one is still the
			// variadic residual only the program end absorbs.
			gi := len(lw.p.GlobalBinds)
			lw.p.GlobalBinds = append(lw.p.GlobalBinds, GlobalBindSpec{
				Name: d.name, Depth: d.depth, Splice: true, SpliceFromTop: d.spliceDepth,
			})
			lw.emit(OpBindGlobal, gi, d.pos)
			lw.note()
			return ""
		case d.srcSeq >= 0 && !lw.variadic[d.srcSeq] && d.spliceDepth >= 0 && needGlobal:
			// The S9.1 STATIC-region first-value bind (SplitEventRegionBind):
			// the bound value is idx 0 of a multi-out event — stack-deepest of
			// the region, at the STATIC depth nout-1 below its top. Same
			// splice bind as the S5 arm, plus the SIM entry for (seq, 0) is
			// removed: unlike the variadic region (one sim entry standing for
			// the whole region), the static region's results each own a slot
			// and the spliced value's slot is gone at run time.
			gi := len(lw.p.GlobalBinds)
			lw.p.GlobalBinds = append(lw.p.GlobalBinds, GlobalBindSpec{
				Name: d.name, Depth: d.depth, Splice: true, SpliceFromTop: d.spliceDepth,
			})
			lw.emit(OpBindGlobal, gi, d.pos)
			for i := len(lw.vm) - 1; i >= 0; i-- {
				if lw.vm[i].seq == d.srcSeq && lw.vm[i].idx == 0 {
					lw.vm = append(lw.vm[:i], lw.vm[i+1:]...)
					break
				}
			}
			lw.note()
			return ""
		case d.srcSeq >= 0 && lw.variadic[d.srcSeq]:
			// A def of a VARIADIC producer (a loop collect) has no single
			// value to bind — the same Stage-2 boundary every other consumer
			// of a loop result hits, under the same reason.
			return "def `" + d.name + "` consumes loop results (Stage 2 loops only feed the program residual)"
		case d.srcSeq >= 0:
			// A COMPUTED def value lives on the simulated stack at its producing
			// event, not here — it is re-pushable only through a promoted frame
			// local (planValueDefLocals / the unit's promoted map). Without the
			// promotion there is no way to duplicate it for the registry install;
			// refuse (sound interpreter fallback).
			slot, ok := lw.promoted[d.srcSeq]
			if !ok {
				return "dynamic-scope def `" + d.name + "` of unpromoted computed value"
			}
			src = localOperand(slot)
		case src.kind == opNone && d.root && core.IsModuleFamilyValue(d.val):
			// A ROOT def of a MODULE-FAMILY value with no producing event —
			// `def m (module […])`, the descriptor a compile-time word built —
			// needs no OpBindDynScope: the check pass installed the binding
			// and it survives to run time (kept, or replayed by its twin,
			// which re-installs a concrete captured entry), so the live
			// OpLookupDynScope that dynScopeNames promised resolves it as
			// the interpreter does. A frame-local def of one keeps the
			// refusal below: its binding is popped with the frame.
			return ""
		case src.kind == opNone:
			// A literal binding: bake the recorded value verbatim, UNPOOLED (it
			// may carry a reparented tag a same-canon source literal must not
			// inherit). Only inert data bakes; a non-inert value gets one
			// rescue — a STRIPPED-LITERAL carrier whose original the recorder
			// remembered (`def x <a/>`: the checker binds the Xml literal's
			// carrier; RememberOriginal holds the materialised instance, and
			// resolveOperand recovers it as a const/local — the same slot the
			// binding's reads resolve to, so the written-back value IS the
			// instance the program uses). Anything else refuses.
			if core.IsInertConst(d.val) {
				src = ConstOperand(lw.es.internUnpooled(d.val))
			} else if op, ok := lw.es.resolveOperand(d.val); ok && (op.kind == opConst || op.kind == opLocal) {
				src = op
			} else {
				return "dynamic-scope def `" + d.name + "` of unknown provenance"
			}
		}
	}
	if needDyn {
		// The bind's own re-push of the source is not a READ of the name
		// (a deopt tests at the consumer's push — deoptAtSlot).
		lw.binding = true
		lw.pushOperand(src, d.pos)
		lw.binding = false
		lw.emit(OpBindDynScope, lw.es.internUnpooled(core.NewString(d.name)), d.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	if needGlobal {
		// A DEAD source (bindConsumes suppressed its producer-site drop) is
		// consumed BY the bind: fast path in Pop mode. A live fast-path value
		// is peeked in place for its downstream consumers.
		pop := !fastGlobal || lw.dead[d.srcSeq]
		gi := len(lw.p.GlobalBinds)
		lw.p.GlobalBinds = append(lw.p.GlobalBinds, GlobalBindSpec{Name: d.name, Depth: d.depth, Pop: pop})
		if !fastGlobal {
			// Re-push a copy from its resolved home; the bind consumes it
			// (Pop mode — one op, no separate DROP in the stream).
			lw.pushOperand(src, d.pos)
		}
		lw.emit(OpBindGlobal, gi, d.pos)
		if pop {
			lw.vm = lw.vm[:len(lw.vm)-1]
		}
	}
	return ""
}

// seatStoreName seats, on the promoted STORE_LOCAL about to be emitted, the
// def name the stored slot's produced fn value was bound under
// (EmitState.defNameAt — RecordDynBind's note), so the VM's store renames
// the closure as the interpreter's installDef renames a fn value bound by
// `def`. Nothing seated for a slot no def named, or a target with no table.
func (lw *lowerer) seatStoreName(seq, idx int) {
	if lw.es == nil || lw.storeNames == nil {
		return
	}
	name, ok := lw.es.defNameAt[seqIdx{seq, idx}]
	if !ok {
		return
	}
	if *lw.storeNames == nil {
		*lw.storeNames = map[int]string{}
	}
	(*lw.storeNames)[len(*lw.code)] = name
}

func (lw *lowerer) lowerStore(ev *EmitEvent) string {
	st := ev.store
	if st.src.kind == opEvent {
		if lw.variadic[st.src.idx] {
			return "loop-carried store of a variadic result (Stage 3)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], st.src) {
			return "loop-carried store source is not on top of the stack"
		}
		lw.emit(OpStoreLocal, st.slot, st.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		lw.note()
		return ""
	}
	lw.pushOperand(st.src, st.pos)
	lw.emit(OpStoreLocal, st.slot, st.pos)
	lw.vm = lw.vm[:len(lw.vm)-1]
	lw.note()
	return ""
}

// lowerer walks an event trace emitting instructions over a simulated
// stack of producing event seqs (-1 = const).
// loopCtx is the lowering context of one open loop: its FOR_NEXT pc, which
// is the back-edge's target. break and continue name no pc at all — they
// emit the FLOW signal ops, which resolve the open loop at run time
// (lowerBreak) — so the loop needs only its depth recorded here.
type loopCtx struct {
	nextPC int
}

type lowerer struct {
	es    *EmitState
	p     *Program
	code  *[]Instr       // current emission target (main or one fn unit)
	debug *[]core.SrcPos // 1:1 with code
	// closureRet is the emission target's callback-contract table
	// (Program.ClosureRet for the main code, CompiledFn.ClosureRet for a fn
	// unit), keyed by the target's own pc — see pushOperand.
	closureRet *map[int]ClosureRetSpec
	// dynApplyName is the emission target's trailing-apply name table
	// (CompiledFn.DynApplyName), keyed by the target's own pc — see
	// seatDynApplyName. Nil for the main code, whose applies name no frame
	// binding.
	dynApplyName *map[int]DynApplyHead
	// storeNames is the emission target's def-name table for promoted
	// stores of produced fn values (Program.StoreNames / CompiledFn.StoreNames),
	// keyed by the target's own pc — see seatStoreName.
	storeNames *map[int]string
	sigIdx     map[*core.Signature]int
	vm         []vmSlot
	variadic   map[int]bool // loop seqs: N runtime values, not one
	promoted   map[int]int  // value-def locals: producing event seq → frame local slot
	dead       map[int]bool // single-result value-defs referenced zero times: drop the result
	// bindConsumes marks DEAD producers whose result a root OpBindGlobal
	// write-back consumes (Pop mode) instead of the producer-site dead-drop:
	// the value stays live through the immediately-following evDynBind, which
	// pops it into the kept binding slot — one op, nothing lingers. Computed
	// at plan time (collectRootBindConsumes); nil for fn units (no root defs).
	bindConsumes map[int]bool
	// markBefore / variadicElse drive the chained variadic-statement-if (a 2-arg
	// `if`'s 0-or-1 result claimed as the else of a following `if`). markBefore[seq]
	// emits an OpStackMark before that event opens a variadic region; variadicElse[seq]
	// lowers the claiming branch via the mark (DROP_TO_MARK / POP_MARK) instead of
	// the fixed-offset computed-else path. Both keyed by event seq, computed by
	// planVariadicClaims.
	markBefore   map[int]bool
	variadicElse map[int]bool
	// regionPrefixSeq is the event seq of a residual-final runtime-variadic
	// REGION whose residual carries an INERT PREFIX beneath it (NUR067's
	// consuming half — planRegionPrefix armed it and put an OpStackMark in
	// markBefore). 0 = not armed. Read once, by seatRegionPrefix.
	regionPrefixSeq int
	// collectAtSeq is the event seq of the LIST LITERAL that collects a
	// runtime-variadic region (NUR067's consuming half — planRegionCollect
	// armed it and put an OpStackMark before the region's own event). 0 = not
	// armed. Read once, by collectRegionTop.
	collectAtSeq int
	loops        []loopCtx
	maxDepth     int
	// depth counts live lowerFragment recursion (nested branch / loop bodies).
	// The parser already caps source nesting (maxParseNestingDepth), so a program
	// that reached the lowerer is shallow enough; this is defense-in-depth for an
	// event tree built by any path other than the parser. Exceeding maxLowerDepth
	// REFUSES compilation (a clean fallback to the interpreter), never crashes.
	depth int
	// fragMulti is set by lowerFragment when the just-lowered arm left MORE than
	// one runtime value (a multi-value branch arm). lowerArms reads it right after
	// each lowerArm to force the merge variadic — a multi-value arm makes the
	// branch result runtime-variable-count even when both arms net "a value".
	fragMulti bool
	// isFnUnit marks a lowerer driving a USER FN body unit (not the main
	// program). A break/continue with no enclosing loop in such a unit is a
	// cross-frame flow signal — it targets the caller's loop — so it lowers to
	// OpFlowBreak/OpFlowContinue rather than refusing. At the main unit the same
	// shape stays a refusal (a top-level break outside any loop).
	isFnUnit bool
	// numLocals is the current unit's frame-local count, seeded from the unit's
	// recorded locals and bumped by allocLocal for spill temps (spillSeat). The
	// caller writes it back to the unit's NumLocals after lowering so the VM
	// allocates a frame large enough for the temps.
	numLocals int
	// deoptNames are a DEOPT unit's island-read names (planDeopts): a def
	// among them binds registry-visibly and its computed source is
	// promoted, as under dynEnv.
	deoptNames map[string]bool
	// deopts are the unit's pending deopt points (emitDeoptsBefore lowers
	// each where its statement begins; deoptTable is the unit's table).
	deopts     []deoptPoint
	deoptTable *[]DeoptSpec
	// deoptAtSlot holds the points tested where the read's value is pushed
	// as an operand (deoptPoint.atPush), keyed by its frame slot.
	deoptAtSlot map[int]deoptPoint
	// deoptAfterSeq holds the RE-STEP points (deoptPoint.restep, NUR124),
	// keyed by the event whose results they test; emitReStepAfter lowers
	// each right after that event's op.
	deoptAfterSeq map[int]deoptPoint
	// unnamedParams are the unit's unnamed param slots (seatUnitDeopts):
	// the interpreter's frame holds their values on its stack bottom, so a
	// deopt island seats every one this unit has not pushed yet beneath
	// its region (deoptPrefix). localPushed / localPushedNested record the
	// slots PUSH_LOCAL has pushed so far, at the root and inside a nested
	// fragment respectively.
	unnamedParams     []int
	localPushed       map[int]bool
	localPushedNested map[int]bool
	// binding marks a dyn-bind's own source re-push (lowerDynBind), which
	// the deoptAtSlot hook must not take for the consumer's read.
	binding bool
}

// allocLocal reserves a fresh frame-local slot in the current unit (for a
// spill-and-reload temp). The unit's NumLocals is reconciled from lw.numLocals
// after lowering.
func (lw *lowerer) allocLocal() int {
	t := lw.numLocals
	lw.numLocals++
	return t
}

// spillSeat is the destination-driven (DDCG) fallback for an operand shape the
// cheap stack-only paths can't seat: a call's computed (event) operands sit on
// the contiguous top of the simulated stack (straight-line code leaves them
// there), possibly in a permutation or interleaved at sig positions with inert
// operands not yet pushed. It SPILLS each event operand to a fresh frame-local
// destination (OpStoreLocal, popping the top), then re-pushes EVERY operand in
// sig order (deepest first, so sig position 0 lands on top) — an event from its
// temp local (PUSH_LOCAL), an inert operand via pushOperand. This seats any
// shape without a 3-deep stack rotate. It declines (returning failMsg, the
// caller's original refusal wording) only when a top slot is NOT one of the
// call's event operands — a non-operand value is interleaved, which a local
// spill cannot reach (STORE_LOCAL pops the top). Promotion is sound: a local
// re-pushes the exact value in any order.
func (lw *lowerer) spillSeat(ops []EmitOperand, results []int, n int, pos core.SrcPos, failMsg string) string {
	ne := len(results)
	if len(lw.vm) < ne {
		return failMsg
	}
	temp := make(map[int]int, ne) // ops index → spill-temp slot
	for k := 0; k < ne; k++ {
		slot := lw.vm[len(lw.vm)-1]
		oi := -1
		for _, i := range results {
			if _, seated := temp[i]; !seated && slotIs(slot, ops[i]) {
				oi = i
				break
			}
		}
		if oi < 0 {
			return failMsg
		}
		t := lw.allocLocal()
		lw.emit(OpStoreLocal, t, pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		temp[oi] = t
	}
	for i := n - 1; i >= 0; i-- {
		if t, ok := temp[i]; ok {
			lw.emit(OpPushLocal, t, pos)
			lw.vm = append(lw.vm, nonEventSlot)
		} else {
			lw.pushOperand(ops[i], pos)
		}
	}
	lw.note()
	return ""
}

// vmSlot is one entry on the lowerer's simulated operand stack: the producing
// event seq and which of that event's results it is (P5 multi-result lowering).
// A const / local / type / closure push uses seq=-1 (no producing event); a
// single-result event uses idx 0; a multi-result call pushes one slot per
// result, idx 0..N-1 deepest-first.
type vmSlot struct{ seq, idx int }

// nonEventSlot is the simulated-stack entry for a freshly pushed const / local
// / type / closure value — no producing event (seq -1).
var nonEventSlot = vmSlot{seq: -1}

// slotIs reports whether a simulated-stack slot is the value an event operand
// names: same producing event seq AND same result index.
func slotIs(slot vmSlot, op EmitOperand) bool {
	return slot.seq == op.idx && slot.idx == op.resIdx
}

// pushOperand emits the push for a const, local, or type operand.
func (lw *lowerer) pushOperand(op EmitOperand, pos core.SrcPos) {
	if op.kind == opClosure {
		// Push the captures (enclosing-scope operands), then OpPushClosure
		// pops them into the closure value. Net stack effect: +1.
		for i := range op.closureCaps {
			lw.pushOperand(op.closureCaps[i], pos)
		}
		// The contract rides on the VALUE, so it is keyed by THIS push's pc —
		// the same unit pushed at another site may carry a different one, or
		// none (see core.ClosurePayload's RetTypes comment).
		// Keyed in the EMISSION TARGET's own table at the target's own pc: a
		// fn unit's code has its own pc space (CompiledFn.ClosureRet), and
		// the main code its own (Program.ClosureRet). Keying the program map
		// at len(p.Code) from inside a unit lost every in-unit contract.
		if op.closureRet != nil {
			if *lw.closureRet == nil {
				*lw.closureRet = map[int]ClosureRetSpec{}
			}
			(*lw.closureRet)[len(*lw.code)] = *op.closureRet
		}
		lw.emit(OpPushClosure, op.closureUnit, pos)
		lw.vm = lw.vm[:len(lw.vm)-len(op.closureCaps)]
		lw.vm = append(lw.vm, nonEventSlot)
		lw.note()
		return
	}
	// pushOperand only ever materialises const/local/type operands (event
	// operands are already on the stack); opNone never reaches here.
	switch op.kind {
	case opLocal:
		if d, ok := lw.deoptAtSlot[op.idx]; ok && !lw.binding && lw.depth == 0 && lw.deoptTable != nil {
			// The read's statement begins here (deoptPoint.atPush): the
			// compiled stack holds exactly what the interpreter's held at
			// the read. Tested once, at the first push.
			delete(lw.deoptAtSlot, op.idx)
			if prefix, ok := lw.deoptPrefix(); ok {
				*lw.deoptTable = append(*lw.deoptTable, DeoptSpec{Name: d.name, Pos: d.pos, Slot: op.idx, Depth: -1, Prefix: prefix, Token: d.token, RetPC: -1})
				lw.emit(OpDeoptIfFn, len(*lw.deoptTable)-1, d.start)
			}
		}
		lw.emit(OpPushLocal, op.idx, pos)
	case opType:
		lw.emit(OpPushType, op.idx, pos)
	case opDynScope:
		lw.emit(OpLookupDynScope, op.idx, pos)
	case opDataScope:
		lw.emit(OpLookupDynScopeData, op.idx, pos)
	default: // opConst
		lw.emit(OpPushConst, op.idx, pos)
	}
	lw.vm = append(lw.vm, nonEventSlot)
	lw.note()
}

func (lw *lowerer) note() {
	if len(lw.vm) > lw.maxDepth {
		lw.maxDepth = len(lw.vm)
	}
}

// emitDeoptsBefore lowers every pending deopt point (NUR123) whose
// statement begins at or before position p — the first op of that
// statement is about to be emitted at the unit's root level — as an
// OpDeoptIfFn over the read value's current home: the frame local its
// promoted producer stores to, or its entry on the simulated stack. The
// zero position flushes every point left (a residual read no event
// follows). A value with neither home keeps the slot push (best effort).
func (lw *lowerer) emitDeoptsBefore(p core.SrcPos) {
	if len(lw.deopts) == 0 || lw.deoptTable == nil || lw.depth > 0 {
		return
	}
	kept := lw.deopts[:0]
	for _, d := range lw.deopts {
		if p.Row > 0 && posAfter(d.start, p) {
			kept = append(kept, d)
			continue
		}
		prefix, ok := lw.deoptPrefix()
		if !ok {
			continue
		}
		spec := DeoptSpec{Name: d.name, Pos: d.pos, Slot: -1, Depth: -1, Prefix: prefix, Token: d.token, RetPC: -1}
		if d.slot >= 0 {
			spec.Slot = d.slot
		} else if slot, ok := lw.promoted[d.seq]; ok {
			spec.Slot = slot
		} else {
			for i := len(lw.vm) - 1; i >= 0; i-- {
				if lw.vm[i].seq == d.seq && lw.vm[i].idx == 0 {
					spec.Depth = len(lw.vm) - 1 - i
					break
				}
			}
			if spec.Depth < 0 {
				continue
			}
		}
		*lw.deoptTable = append(*lw.deoptTable, spec)
		lw.emit(OpDeoptIfFn, len(*lw.deoptTable)-1, d.start)
	}
	lw.deopts = kept
}

// emitReStepAfter lowers a RE-STEP point (NUR124) as an OpDeoptIfFn over
// the results the event just left on top of the simulated stack — the
// values the interpreter would splice back onto the tape and step. A point
// serves only at the unit's root with the event's results on top (a
// promoted result lives in a slot; a variadic one has no static count).
// A fn-TYPED note no point serves REFUSES: the pass is certain the runtime
// value is a fn the interpreter dispatches here, and the unit would keep it
// as data. A gradual note (the value is a fn only sometimes) with no point
// keeps the optimistic model it always had.
func (lw *lowerer) emitReStepAfter(ev *EmitEvent) string {
	if lw.es == nil {
		return ""
	}
	note, noted := lw.es.reStepNotes[ev.seq]
	if !noted {
		return ""
	}
	if d, planned := lw.deoptAfterSeq[ev.seq]; planned && lw.depth == 0 && lw.deoptTable != nil && !lw.es.eventInfo[ev.seq].variadicResult {
		n := 0
		for i := len(lw.vm) - 1; i >= 0 && lw.vm[i].seq == ev.seq; i-- {
			n++
		}
		if prefix, ok := lw.deoptPrefix(); n > 0 && ok {
			delete(lw.deoptAfterSeq, ev.seq)
			*lw.deoptTable = append(*lw.deoptTable, DeoptSpec{Pos: d.pos, Slot: -1, Depth: -1, Prefix: prefix, Results: n, Token: d.token, RetPC: -1})
			lw.emit(OpDeoptIfFn, len(*lw.deoptTable)-1, d.start)
			return ""
		}
	}
	if note.strict {
		word := "a call"
		if ev.kind == evCall {
			word = "`" + ev.call.word + "`"
		}
		return word + ": a fn-typed result is re-stepped into a dispatch the model cannot make (NUR124)"
	}
	return ""
}

// seatDynApplyName records a trailing fn-value apply's head binding name at
// the pc of the OpCallDynTrailTop / OpCallDynTrailKeepQ about to be emitted
// (CompiledFn.DynApplyName), so the op's no-match diagnostic can name and
// point at the read the interpreter dispatches. An apply with no bare-read
// head, and the main code (no frame to bind), record nothing.
func (lw *lowerer) seatDynApplyName(w DynApplyHead) {
	if lw.dynApplyName == nil || w.Name == "" {
		return
	}
	if *lw.dynApplyName == nil {
		*lw.dynApplyName = map[int]DynApplyHead{}
	}
	(*lw.dynApplyName)[len(*lw.code)] = w
}

func (lw *lowerer) emit(op Opcode, arg int, pos core.SrcPos) int {
	if op == OpPushLocal && len(lw.unnamedParams) > 0 {
		if lw.depth > 0 {
			if lw.localPushedNested == nil {
				lw.localPushedNested = map[int]bool{}
			}
			lw.localPushedNested[arg] = true
		} else {
			if lw.localPushed == nil {
				lw.localPushed = map[int]bool{}
			}
			lw.localPushed[arg] = true
		}
	}
	*lw.code = append(*lw.code, Instr{Op: op, Arg: int32(arg)})
	*lw.debug = append(*lw.debug, pos)
	return len(*lw.code) - 1
}

// deoptPrefix lists the unit's unnamed params the interpreter's frame still
// holds on its stack bottom at a deopt point — the ones this unit's root
// code has not pushed yet — so the island seats them beneath its region
// (DeoptSpec.Prefix). An unnamed param pushed inside a nested fragment may
// or may not have been consumed by the time the point runs, so a point
// after such a push declines.
func (lw *lowerer) deoptPrefix() ([]int, bool) {
	var prefix []int
	for _, s := range lw.unnamedParams {
		if lw.localPushedNested[s] {
			return nil, false
		}
		if !lw.localPushed[s] {
			prefix = append(prefix, s)
		}
	}
	return prefix, true
}

// lowerEvents lowers a trace. scopeFloor is the closed-fragment rule:
// an operand produced by an event with seq <= scopeFloor lives in the
// enclosing scope, which Stage 2 branch fragments must not read.
// planVariadicClaims scans a straight-line frame for the chained variadic-if:
// a computed-else branch whose else operand is the 0-or-1 result of a PRIOR
// 2-arg (no-else) `if`. The producer leaves a runtime-variable count, so the
// claiming if cannot drop it at a fixed offset — it needs a stack-mark region.
// Returns markBefore (the seq before which to open the region — the producer's
// cond event if it is an event, else the producer itself) and variadicElse (the
// claiming branch seqs). nil maps when no claim is present (the common case).
func planVariadicClaims(events []EmitEvent) (markBefore, variadicElse map[int]bool) {
	byseq := func(seq int) *EmitEvent {
		for i := range events {
			if events[i].seq == seq {
				return &events[i]
			}
		}
		return nil
	}
	for i := range events {
		ev := &events[i]
		if ev.kind != evBranch || ev.br == nil || !ev.br.elsComputed || ev.br.elsVal.kind != opEvent {
			continue
		}
		prod := byseq(ev.br.elsVal.idx)
		// The producer must be a 2-arg (no-else) value-producing `if` — exactly the
		// 0-or-1 variadic statement guard. (A 3-arg if's result is non-variadic and
		// the existing computed-else path handles it.)
		if prod == nil || prod.kind != evBranch || prod.br == nil || prod.br.hasElse || !prod.br.hasThenOut {
			continue
		}
		markSeq := prod.seq
		if prod.br.cond.kind == opEvent {
			markSeq = prod.br.cond.idx // open the region before the producer's cond eval
		}
		if markBefore == nil {
			markBefore = map[int]bool{}
			variadicElse = map[int]bool{}
		}
		markBefore[markSeq] = true
		variadicElse[ev.seq] = true
	}
	return markBefore, variadicElse
}

// verifyMarkWindow checks the post-lowering sim stack IS the mark-window
// residual — nothing seats, nothing re-pushes: slot i (deepest-first) must be
// exactly the event result the residual lists (plan Phase 5, L-DO part 2b).
func (lw *lowerer) verifyMarkWindow(ops []EmitOperand) string {
	if len(lw.vm) != len(ops) {
		return "mark-window residual does not match the lowered stack"
	}
	for i, op := range ops {
		if op.kind != opEvent || lw.vm[i].seq != op.idx || lw.vm[i].idx != op.resIdx {
			return "mark-window residual does not match the lowered stack"
		}
	}
	return ""
}

func (lw *lowerer) lowerEvents(events []EmitEvent, scopeFloor int) string {
	for i := range events {
		ev := &events[i]
		if lw.depth == 0 && len(lw.deopts) > 0 {
			lw.emitDeoptsBefore(eventPos(*ev))
		}
		if lw.markBefore[ev.seq] {
			lw.emit(OpStackMark, 0, eventPos(*ev))
		}
		if scopeFloor > 0 {
			var crossed bool
			forEachOperand(ev, func(op EmitOperand) {
				if op.kind != opEvent || op.idx > scopeFloor {
					return
				}
				// A PROMOTED enclosing value is delivered via a FRAME LOCAL, not the
				// enclosing simulated stack the Stage-2 floor guards. Its CONSUMING
				// references (cond / thenVal / elsVal / call operands) were already
				// rewritten to local pushes by RewritePromotedRefs, so an opEvent
				// reference to a promoted producer that still reaches here is an arm-OUT
				// designation (thenOut / elsOut / condOut) — the shapes RewritePromotedRefs
				// deliberately leaves for lowerFragment to re-resolve to the local. Either
				// way the value crosses via the frame, not the sim, so it does not trip
				// the floor. Without this a handler with nested `if` arms reading an
				// enclosing value-def (mini-redis LRANGE: `def start …; if (start gte …)
				// [ … slice start … ]`) refused to compile.
				if _, prom := lw.promoted[op.idx]; prom {
					return
				}
				crossed = true
			})
			if crossed {
				return "branch reads enclosing computation (Stage 3)"
			}
		}
		var reason string
		switch ev.kind {
		case evCall:
			reason = lw.lowerCall(ev)
		case evBranch:
			reason = lw.lowerBranch(ev)
		case evLoop:
			reason = lw.lowerLoop(ev)
		case evBreak:
			reason = lw.lowerBreak(ev)
		case evContinue:
			reason = lw.lowerContinue(ev)
		case evCallUser:
			if ev.uc.poly != nil {
				reason = lw.lowerUserPolyCall(ev)
			} else {
				reason = lw.lowerUserCall(ev)
			}
		case evFallback:
			reason = lw.lowerFallback(ev)
		case evTrap:
			reason = lw.lowerTrap(ev)
		case evStore:
			reason = lw.lowerStore(ev)
		case evDynBind:
			reason = lw.lowerDynBind(ev)
		case evBindTwin:
			// Zero stack effect, no operands: the op only marks the
			// transition's stream position (inert until the flip), so neither
			// the sim nor the residual accounting moves.
			lw.emit(OpBindTwin, ev.twin.idx, ev.twin.pos)
		default:
			reason = "unknown event kind"
		}
		if reason != "" {
			return reason
		}
		if reason := lw.emitReStepAfter(ev); reason != "" {
			return reason
		}
		// A PROMOTED branch value-def (planValueDefLocals marked it, a multiply-read
		// `def bi (if …)`): store the merge to its frame slot so later references
		// re-push from the slot (RewritePromotedRefs rewrote them to local operands).
		// lowerBranch left exactly one merge slot on top; STORE+pop it. If the merge is
		// not a clean single value on top (a variadic / diverged branch the plan-time
		// branchSingleValue gate could not foresee), REFUSE — sound interpreter
		// fallback, never a wrong store.
		if ev.kind == evBranch {
			if slot, ok := lw.promoted[ev.seq]; ok {
				if lw.variadic[ev.seq] || len(lw.vm) == 0 || lw.vm[len(lw.vm)-1].seq != ev.seq {
					return "if: promoted merge is not a single value on top (Stage 3)"
				}
				lw.emit(OpStoreLocal, slot, ev.br.pos)
				lw.vm = lw.vm[:len(lw.vm)-1]
			} else if lw.dead[ev.seq] && !lw.bindConsumes[ev.seq] &&
				// A DEAD branch value-def: its merge result sits on the sim unconsumed —
				// drop it (the binding is never read; a bindConsumes merge instead
				// stays live for its root OpBindGlobal write-back to pop). lowerBranch
				// left exactly one slot for this branch's merge on top; pop+DROP it.
				len(lw.vm) > 0 && lw.vm[len(lw.vm)-1].seq == ev.seq {
				lw.emit(OpDrop, 0, ev.br.pos)
				lw.vm = lw.vm[:len(lw.vm)-1]
			}
		}
	}
	return ""
}

// forEachOperand calls fn for every enclosing-scope operand an event references
// — call / user-call / fallback args, a branch condition and arm outs, a loop's
// range and body out. A callback (rather than a returned slice) keeps the hot
// planning loops — the scopeFloor guard and planValueDefLocals — allocation-free.
func forEachOperand(ev *EmitEvent, fn func(EmitOperand)) {
	// visit surfaces an operand AND, for a closure operand, its lexical
	// captures — enclosing-scope operands carried in closureCaps. Surfacing them
	// makes a def used ONLY as a closure capture REFERENCE-COUNTED (so
	// planValueDefLocals promotes a captured computed def to a frame local rather
	// than leaving it an un-re-pushable event), and lets the scopeFloor guard see
	// a cross-floor capture. Captures are flat operands (no nested closures).
	visit := func(op EmitOperand) {
		fn(op)
		if op.kind == opClosure {
			for _, c := range op.closureCaps {
				fn(c)
			}
		}
	}
	switch ev.kind {
	case evCall:
		for _, op := range ev.call.ops {
			visit(op)
		}
	case evLoop:
		visit(ev.loop.start)
		visit(ev.loop.end)
		visit(ev.loop.step)
		visit(ev.loop.condOut)
		visit(ev.loop.bodyOut)
		for _, c := range ev.loop.carried {
			visit(c.init)
		}
	case evBreak, evContinue:
		// no operands
	case evTrap:
		// A plain trap has no operands; a runtime-rematch trap's window
		// operands are reference-counted like call ops so their producers
		// stay live (and promoted refs rewrite).
		for _, op := range ev.trap.rematchOps {
			visit(op)
		}
	case evCallUser:
		for _, op := range ev.uc.ops {
			visit(op)
		}
	case evFallback:
		for _, op := range ev.fb.ins {
			visit(op)
		}
	case evStore:
		visit(ev.store.src)
	case evDynBind:
		visit(ev.dyn.src)
	case evBindTwin:
		// no operands — the twin references nothing and produces nothing
	default: // evBranch
		// thenVal / elsVal are the value-arm operands — meaningful only when
		// thenIsVal / elsIsVal, and an opEvent only for the computed-arm shapes
		// (`if c (expr) e` / `if c [t] (expr)`); for any other branch each is the
		// zero opNone, which both consumers (the scopeFloor enclosing-scope guard
		// and the value-def ref count) skip. Including them keeps the operand set
		// complete so a computed-arm event is REFERENCE-COUNTED — otherwise
		// planValueDefLocals sees it as zero-referenced, marks it dead, and the
		// lowerer drops the value the computed-branch lowering needs on the stack.
		visit(ev.br.cond)
		visit(ev.br.condOut)
		visit(ev.br.thenOut)
		visit(ev.br.elsOut)
		visit(ev.br.thenVal)
		visit(ev.br.elsVal)
	}
}

// promoteOperand rewrites one ENCLOSING-scope operand: a single-result
// event reference whose producer was promoted to a value-def local becomes
// a local push. Inner-fragment result operands (a branch arm's / loop body's
// out) are never enclosing references and are left untouched.
func promoteOperand(op *EmitOperand, promoted map[int]int) {
	if op.kind == opEvent {
		if slot, ok := promoted[op.idx]; ok {
			// slot is the producer's BASE slot; output idx op.resIdx is at
			// slot+resIdx. Single-output promotions have resIdx 0 (unchanged);
			// a non-promoted multi-output producer is absent from the map.
			*op = localOperand(slot + op.resIdx)
		}
		return
	}
	// A closure operand carries its lexical captures as enclosing-scope operands.
	// A captured computed def promoted to a frame local must have its closureCaps
	// entry rewritten to the local push too, so OpPushClosure captures the right
	// VALUE (re-pushed from the frame slot) rather than a stale/unreachable event
	// operand — the value-passing half of the each/scan/…$body computed-capture fix.
	if op.kind == opClosure {
		for i := range op.closureCaps {
			promoteOperand(&op.closureCaps[i], promoted)
		}
	}
}

// eachClosureCap calls fn for every CLOSURE-CAPTURE operand carried by ev's own
// operands (not recursing into fragments — the caller flattens those). A closure
// capture can only reference a frame local or an enclosing operand at run time,
// never a transient simulated-stack slot, so a producer captured here MUST be
// promoted to a frame local. Mirrors forEachOperand's operand-slice coverage.
func eachClosureCap(ev *EmitEvent, fn func(EmitOperand)) {
	scan := func(ops []EmitOperand) {
		for _, op := range ops {
			if op.kind == opClosure {
				for _, c := range op.closureCaps {
					fn(c)
				}
			}
		}
	}
	switch ev.kind {
	case evCall:
		scan(ev.call.ops)
	case evCallUser:
		scan(ev.uc.ops)
	case evFallback:
		scan(ev.fb.ins)
	case evLoop:
		scan([]EmitOperand{ev.loop.start, ev.loop.end, ev.loop.step, ev.loop.condOut, ev.loop.bodyOut})
	case evBranch:
		scan([]EmitOperand{ev.br.cond, ev.br.condOut, ev.br.thenOut, ev.br.elsOut, ev.br.thenVal, ev.br.elsVal})
	}
}

// branchSingleValue reports whether a 2-arm `if` merges to exactly one value — both
// arms present and neither a MULTI-value arm (residualN>1). Only such a branch may be
// promoted to a frame local as a value-def: the store seats one value. A value-arm
// (thenIsVal/elsIsVal, nil fragment) is single by construction. A no-else if (variadic
// 0-or-1) and a multi-value arm are excluded — their merge count is runtime-variable.
func branchSingleValue(br *emitBranch) bool {
	if br == nil || !br.hasElse {
		return false
	}
	if br.then != nil && br.then.residualN > 1 {
		return false
	}
	if br.els != nil && br.els.residualN > 1 {
		return false
	}
	return true
}

// childFragments returns the body fragments a branch / loop event owns — the
// list-form condition, both `if` arms, a condition loop's condition and a
// loop body. Nil entries are kept so callers iterate a fixed shape and skip
// them; a non-branch/loop event owns none.
func childFragments(ev *EmitEvent) []*EmitFragment {
	switch ev.kind {
	case evBranch:
		return []*EmitFragment{ev.br.condFrag, ev.br.then, ev.br.els}
	case evLoop:
		return []*EmitFragment{ev.loop.cond, ev.loop.body}
	}
	return nil
}

// armRepushableResidual reports whether frag is a BRANCH ARM (childFragments
// slot 1 or 2 of an evBranch) whose MULTI-value residual RecordBranch captured
// WHOLE — every entry resolved to an operand, none of them a parked Function.
// Such an arm no longer has to leave its residual seated on the fragment's
// simulated stack: planValueDefLocals force-promotes each event entry to a
// frame local and lowerFragment re-pushes the captured list in exact order,
// the same linearisation the PROGRAM residual's forceOrder already uses. That
// is what makes the arm's INTERIOR promotable — a `def` inside it can move to
// a slot (and so be read by a closure capture) without stranding the residual.
// A loop body is excluded: its residual re-pushes on every iteration, where a
// frame slot would hold only the last one.
func armRepushableResidual(ev *EmitEvent, fi int, frag *EmitFragment) bool {
	return ev.kind == evBranch && fi > 0 && frag != nil &&
		frag.residualN > 1 && len(frag.residualOps) == frag.residualN
}

// armResidualForceSeqs returns the event seqs that ARE a whole-captured
// multi-value arm's residual. planValueDefLocals promotes each to a frame slot
// (and skips its stay-on-the-sim exemption) so lowerFragment's re-push arm
// finds an EMPTY sim and reconstructs the residual from the captured list.
func armResidualForceSeqs(allEvents []*EmitEvent) map[int]bool {
	force := map[int]bool{}
	for _, ev := range allEvents {
		for fi, frag := range childFragments(ev) {
			if !armRepushableResidual(ev, fi, frag) {
				continue
			}
			for _, op := range frag.residualOps {
				if op.kind == opEvent {
					force[op.idx] = true
				}
			}
		}
	}
	return force
}

// forEachFragmentOperand calls fn for every operand recorded INSIDE ev's body
// fragments, recursing through nested branches / loops. A reference whose
// producer lives OUTSIDE the fragment (the enclosing computation) crosses the
// fragment's scope floor; the only way the fragment can read it is as a frame
// local, so planValueDefLocals uses this walk to force such a producer's
// promotion.
func forEachFragmentOperand(ev *EmitEvent, fn func(EmitOperand)) {
	// A fragment's OUT operand (an arm result / loop body result) is a
	// cross-floor reference too when it names an ENCLOSING-scope producer —
	// the `def kid (user-call …); if c [other] [kid]` arm, whose fragment has
	// NO events of its own, so the frag.events walk below never sees the
	// reference and the producer is left unpromoted (the arm then refuses
	// "branch leaves extra values", out=opEvent vm=0). Visit the outs here so
	// planValueDefLocals counts them as fragment refs; a fragment-INTERNAL out
	// stays unpromoted regardless via the fragResult && fragInternal gate.
	switch ev.kind {
	case evBranch:
		if ev.br != nil {
			if ev.br.hasThenOut {
				fn(ev.br.thenOut)
			}
			if ev.br.hasElsOut {
				fn(ev.br.elsOut)
			}
		}
	case evLoop:
		if ev.loop != nil {
			if ev.loop.cond != nil {
				fn(ev.loop.condOut)
			}
			fn(ev.loop.bodyOut)
		}
	}
	for _, frag := range childFragments(ev) {
		if frag == nil {
			continue
		}
		for i := range frag.events {
			fe := &frag.events[i]
			forEachOperand(fe, fn)
			forEachFragmentOperand(fe, fn)
		}
	}
}

// RewritePromotedRefs redirects an event's enclosing-scope operands (call /
// user-call / fallback args, a branch condition, a loop count) to value-def
// locals, then recurses into any body fragments so a cross-floor reference
// inside a branch arm / loop body is rewritten the same way. Only producers in
// the `promoted` map are touched — an intra-fragment result reference (a branch
// arm's own out, a fragment-local temp) is never promoted, so it is left as the
// event operand the closed-fragment lowering expects.
func RewritePromotedRefs(ev *EmitEvent, promoted map[int]int) {
	switch ev.kind {
	case evTrap:
		for i := range ev.trap.rematchOps {
			promoteOperand(&ev.trap.rematchOps[i], promoted)
		}
	case evCall:
		for i := range ev.call.ops {
			promoteOperand(&ev.call.ops[i], promoted)
		}
	case evCallUser:
		for i := range ev.uc.ops {
			promoteOperand(&ev.uc.ops[i], promoted)
		}
	case evFallback:
		for i := range ev.fb.ins {
			promoteOperand(&ev.fb.ins[i], promoted)
		}
	case evBranch:
		promoteOperand(&ev.br.cond, promoted)
		// A computed-arm value (`if c (expr) e` / `if c [t] (expr)`) is an
		// enclosing-scope reference too; rewrite it in lockstep with
		// forEachOperand counting it. (A promoted computed arm then refuses at
		// lowerBranch's stack-layout check and falls back — sound, never a wrong
		// result.)
		promoteOperand(&ev.br.thenVal, promoted)
		promoteOperand(&ev.br.elsVal, promoted)
	case evLoop:
		promoteOperand(&ev.loop.end, promoted)
		for i := range ev.loop.carried {
			promoteOperand(&ev.loop.carried[i].init, promoted)
		}
	case evStore:
		promoteOperand(&ev.store.src, promoted)
	}
	for _, frag := range childFragments(ev) {
		if frag == nil {
			continue
		}
		// A whole-captured arm residual names its event entries by producing
		// seq; once those are promoted the list must read the SLOTS, or
		// lowerFragment's re-push would push the seq as a const index.
		for i := range frag.residualOps {
			promoteOperand(&frag.residualOps[i], promoted)
		}
		for i := range frag.events {
			RewritePromotedRefs(&frag.events[i], promoted)
		}
	}
}

// collectPromotableEvents flattens a unit's top-level events together with every
// event nested in an inline branch arm / loop body, depth-first — so the
// value-def promotion decision can run over a def-chain inside an `if` arm or
// `for` body, not just the top level. Without this such a chain's computed
// producers sit interleaved on the closed fragment's simulated stack and a later
// binary op refuses "operands of <op> not adjacent on top". (Ref-counting via
// forEachFragmentOperand and rewriting via RewritePromotedRefs already recurse
// into fragments; only the promotion decision did not.)
func collectPromotableEvents(events []EmitEvent) ([]*EmitEvent, map[int]bool, map[int]bool) {
	var out []*EmitEvent
	fragInternal := map[int]bool{}
	// fragID assigns each event the serial id of the fragment it lives in
	// (0 = the unit's top level; each recursed fragment gets a fresh id).
	// crossFragRef marks a producer referenced from a DIFFERENT fragment than
	// the one it lives in — the cross-NESTED-fragment reference lowerFragment
	// cannot seat from the sim (each fragment's vm starts empty): `def kid
	// (find-kid …)` produced inside the OUTER arm, referenced as the INNER
	// if's arm-out. The boolean fragInternal alone conflates "same fragment"
	// with "same-or-ancestor", so the promotion decision needs this second
	// signal to store such producers in a unit-frame local (visible across
	// every nested fragment).
	fragID := map[int]int{}
	crossFragRefSet := map[int]bool{}
	nextID := 0
	var walk func(evs []EmitEvent, id int)
	walk = func(evs []EmitEvent, id int) {
		for i := range evs {
			out = append(out, &evs[i])
			if id != 0 {
				fragInternal[evs[i].seq] = true
			}
			fragID[evs[i].seq] = id
			for fi, frag := range childFragments(&evs[i]) {
				// Only recurse into a SINGLE-result fragment (a single-value branch
				// arm residualN==1, or a loop body / condition residualN==0). A
				// MULTI-value arm (residualN>1, e.g. `[n mul 2 m (n sub 1)]`) leaves
				// several residual values on the sim stack, and promotion / dead-drop
				// would wrongly store or drop one of them ("branch leaves extra
				// values") — leave those untouched. EXCEPT when RecordBranch captured
				// that residual WHOLE (armRepushableResidual): the residual then
				// re-pushes from the captured operand list, so the arm's interior is
				// as promotable as any single-value arm's.
				if frag != nil && (frag.residualN <= 1 || armRepushableResidual(&evs[i], fi, frag)) {
					nextID++
					walk(frag.events, nextID)
				}
			}
		}
	}
	walk(events, 0)
	// Second pass: record references whose consuming fragment differs from the
	// producer's. An event's plain operands consume on ITS fragment's sim; a
	// child fragment's OUT operand must be present on the CHILD's sim, so its
	// consumer is the child fragment. Fragment ids are assigned identically to
	// the first pass (same traversal order).
	nextID = 0
	ref := func(op EmitOperand, consumerFrag int) {
		if op.kind == opEvent && fragID[op.idx] != consumerFrag {
			crossFragRefSet[op.idx] = true
		}
	}
	var walk2 func(evs []EmitEvent, id int)
	walk2 = func(evs []EmitEvent, id int) {
		for i := range evs {
			ev := &evs[i]
			// Enclosing-scope operands ONLY (mirroring RewritePromotedRefs's
			// field set). forEachOperand would ALSO visit a branch's
			// thenOut/elsOut — but those are the CHILD fragments' outs, refed
			// below at the child's id; visiting them here at the parent's id
			// falsely marked every arm's own result as cross-fragment
			// (`if c (1 add 2) [9]` promoted its then-arm add, shifting the
			// emit goldens).
			switch ev.kind {
			case evCall:
				for _, op := range ev.call.ops {
					ref(op, id)
				}
			case evCallUser:
				for _, op := range ev.uc.ops {
					ref(op, id)
				}
			case evFallback:
				for _, op := range ev.fb.ins {
					ref(op, id)
				}
			case evBranch:
				ref(ev.br.cond, id)
				ref(ev.br.thenVal, id)
				ref(ev.br.elsVal, id)
			case evLoop:
				ref(ev.loop.end, id)
				// Carried-slot inits store in the loop EVENT's own scope
				// (lowerLoop, right after FOR_SETUP — the enclosing sim).
				for _, c := range ev.loop.carried {
					ref(c.init, id)
				}
			case evStore:
				ref(ev.store.src, id)
			}
			frags := childFragments(ev)
			// Child fragments recurse with fresh ids in the same order; each
			// fragment's OUT consumes on that child's sim.
			fi := 0
			outsFor := fragmentOuts(ev)
			for _, frag := range frags {
				if frag != nil && (frag.residualN <= 1 || armRepushableResidual(ev, fi, frag)) {
					nextID++
					childID := nextID
					if fi < len(outsFor) && outsFor[fi] != nil {
						ref(*outsFor[fi], childID)
					}
					walk2(frag.events, childID)
				}
				fi++
			}
		}
	}
	walk2(events, 0)
	return out, fragInternal, crossFragRefSet
}

// fragmentOuts returns, per childFragments slot, the OUT operand that must be
// present on that fragment's sim at its close (nil for a fragment with no
// out). Order matches childFragments: [condFrag, then, els] for a branch,
// [cond, body] for a loop.
func fragmentOuts(ev *EmitEvent) []*EmitOperand {
	switch ev.kind {
	case evBranch:
		if ev.br == nil {
			return nil
		}
		outs := make([]*EmitOperand, 3)
		outs[0] = &ev.br.condOut
		if ev.br.hasThenOut {
			outs[1] = &ev.br.thenOut
		}
		if ev.br.hasElsOut {
			outs[2] = &ev.br.elsOut
		}
		return outs
	case evLoop:
		// hasBodyOut gates a SIDE-EFFECT loop's stale bodyOut operand: the body
		// nets no value per iteration, so its recorded operand must not count
		// as a fragment-out reference (it falsely promoted the body's last
		// event in `for 3 [i add 10]`, shifting the for-loop golden).
		if ev.loop == nil {
			return nil
		}
		outs := make([]*EmitOperand, 2)
		if ev.loop.cond != nil {
			outs[0] = &ev.loop.condOut
		}
		if ev.loop.hasBodyOut {
			outs[1] = &ev.loop.bodyOut
		}
		return outs
	}
	return nil
}

// fragmentResultSeqs collects the event seqs that ARE a fragment's residual —
// the value an `if` arm / list-condition / loop body leaves on the simulated
// stack for its enclosing branch/loop (thenOut / elsOut / condOut / bodyOut).
// These must NOT be promoted to a frame local: the arm-result lowering
// (lowerFragment) requires the residual on the sim stack, and a tail call in an
// arm must stay terminal for OpTailCallUser detection. Only fragment-INTERNAL
// intermediates (the def-chain feeding the result) are promotable.
func fragmentResultSeqs(allEvents []*EmitEvent) map[int]bool {
	res := map[int]bool{}
	mark := func(op EmitOperand) {
		if op.kind == opEvent {
			res[op.idx] = true
		}
	}
	for _, ev := range allEvents {
		switch ev.kind {
		case evBranch:
			mark(ev.br.thenOut)
			mark(ev.br.elsOut)
			mark(ev.br.condOut)
		case evLoop:
			mark(ev.loop.condOut)
			mark(ev.loop.bodyOut)
		}
	}
	return res
}

// fragResultStaysOnSim reports whether a fragment-internal RESULT event (an arm /
// loop-body result produced inside the fragment) can be left on that fragment's
// simulated stack — so planValueDefLocals should NOT promote it. It stays iff it
// is the fragment result, is produced inside it, is not consumed across a NESTED
// fragment boundary, AND is not ALSO consumed as an operand within its own
// fragment. The last exception is the todo-api PUT shape `def t2 {…}; (todos set
// (id) t2) drop; t2`: t2 is both the `set` argument and the arm result, so the
// operand use pops its single sim slot and nothing is left to seat as the result
// (the arm refused "branch leaves extra values", out=opEvent vm=0). Such a
// value-def is promoted to a frame local instead (stored once, re-pushed per use;
// the arm re-resolves its out to the local). refs counts operand uses only, never
// the result designation, so a pure arm result (refs==0) still stays on the sim.
func (es *EmitState) fragResultStaysOnSim(seq int, refs map[int]int, fragResult, fragInternal, crossFragRef map[int]bool) bool {
	if !fragResult[seq] || !fragInternal[seq] || crossFragRef[seq] {
		return false
	}
	selfConsumed := refs[seq] > 0 && es.eventInfo[seq].valueDef
	return !selfConsumed
}

// planValueDefLocals decides which of a unit's top-level computed results are
// referenced more than once and so must be promoted to a frame local (the
// carrier-identity item's value-def locals). A single VM-stack copy of a
// COMPUTED value is consumed by its first use; a `def`-bound result used
// several times (`def a (make …) a eq a`) needs it stored once and re-pushed.
//
// References are counted across the unit's top-level event operands AND the
// extra references (the program residual, for frame 0). Only single-result
// native-call events are promotable — a multi-result word (dup) needs the
// carrier-identity DUP path, not a single local; branch/loop variadic results
// never reach here. Slots are allocated from the unit's local namespace.
// Returns seq → slot and rewrites every reference in place to a local operand.
// forceOrder names event seqs that MUST be promoted to a frame local even when
// referenced once — the program residual is not in seatable event*-literal*
// order (an event sits above a literal), so every residual event becomes a
// local and the reconciliation re-pushes the whole residual in exact order.
// computeLeaverPrefix indexes each top-level event by its position and builds a
// prefix-sum of VALUE-LEAVING events (a call / branch / loop / fallback residual
// — one that leaves a value the next op sees on top). buried marking reads
// leaverPrefix[j]-leaverPrefix[i+1] to test whether a leaver fired strictly
// between a producer at i and a reference at j.
func computeLeaverPrefix(events []EmitEvent) (map[int]int, []int) {
	producerIndex := make(map[int]int, len(events))
	leaverPrefix := make([]int, len(events)+1)
	for i := range events {
		producerIndex[events[i].seq] = i
		leaves := 0
		switch events[i].kind {
		case evCall, evBranch, evLoop, evCallUser, evFallback:
			leaves = 1
		}
		leaverPrefix[i+1] = leaverPrefix[i] + leaves
	}
	return producerIndex, leaverPrefix
}

// collectStoreSourceSeqs returns the set of event seqs whose result is the
// on-top SOURCE of a loop-carried store (evStore). Those are re-pushed natively
// by lowerStore, so buried promotion must skip them (see the call site).
func collectStoreSourceSeqs(events []EmitEvent) map[int]bool {
	storeSrc := map[int]bool{}
	all, _, _ := collectPromotableEvents(events)
	for _, ae := range all {
		if ae.kind == evStore && ae.store.src.kind == opEvent && ae.store.src.resIdx == 0 {
			storeSrc[ae.store.src.idx] = true
		}
	}
	return storeSrc
}

// countRefsAndBurials runs planValueDefLocals' operand scans over the
// top-level events, filling the caller's maps in place: refs counts each
// producer's idx-0 consumptions (top-level + fragment); fragRef marks
// producers referenced from INSIDE a branch/loop fragment (the reference
// crosses the fragment's scope floor, so the producer is reachable there
// only as a frame local — a single cross-floor read still needs the
// local); buried marks producers consumed mid-body under an intervening
// value-leaving event (the burial doc sits on the leaverPrefix
// computation at the call site). Factored out of planValueDefLocals for
// the lint complexity ceiling — one scan, no behavior change (PR #295
// merge: main's burial triggers plus this branch's promotion triggers
// crossed gocyclo's 70 in one function).
func (es *EmitState) countRefsAndBurials(events []EmitEvent, producerIndex map[int]int, leaverPrefix []int, storeSrc map[int]bool, refs map[int]int, fragRef, buried map[int]bool) {
	for i := range events {
		forEachOperand(&events[i], func(op EmitOperand) {
			if op.kind == opEvent && op.resIdx == 0 {
				refs[op.idx]++
				if es.eventInfo[op.idx].variadicResult {
					return
				}
				if pi, ok := producerIndex[op.idx]; ok && !storeSrc[op.idx] && leaverPrefix[i]-leaverPrefix[pi+1] > 0 {
					buried[op.idx] = true
				}
			}
		})
		forEachFragmentOperand(&events[i], func(op EmitOperand) {
			if op.kind == opEvent && op.resIdx == 0 {
				refs[op.idx]++
				fragRef[op.idx] = true
				if !storeSrc[op.idx] && !es.eventInfo[op.idx].variadicResult {
					buried[op.idx] = true
				}
			}
		})
	}
}

func (es *EmitState) planValueDefLocals(unit *emitUnit, events []EmitEvent, extra []int, forceOrder map[int]bool) (map[int]int, map[int]bool) {
	refs := map[int]int{}
	fragRef := map[int]bool{} // referenced from INSIDE a branch/loop fragment
	// DynEnv: every dyn-bound def's COMPUTED source event must be promoted —
	// lowerDynBind re-pushes the value from its slot for the OpBindDynScope
	// install (an unpromoted computed value has no re-pushable home and
	// refused "unpromoted computed value" — the `def xs [add 1 2]` OpMakeList
	// shape). Collected up front; joins every promote trigger below.
	// Merged into forceOrder: a dyn-bound source needs exactly the forced
	// promotion forceOrder describes (store once, re-push per use), so one
	// set drives every trigger below without extra per-site conditions.
	var deoptNames map[string]bool
	if unit != nil {
		deoptNames = unit.deoptNames
	}
	if dynBindSrc := es.collectDynBindSources(events, deoptNames); len(dynBindSrc) > 0 {
		merged := make(map[int]bool, len(forceOrder)+len(dynBindSrc))
		for k := range forceOrder {
			merged[k] = true
		}
		for k := range dynBindSrc {
			merged[k] = true
		}
		forceOrder = merged
	}
	// A producer consumed MID-BODY as an operand of a later event needs a frame
	// slot when it is BURIED — an intervening event pushed a value on top of it,
	// so the single-consume simulated stack cannot seat it as an arg. Two triggers:
	//
	//   - TOP-LEVEL (forEachOperand): buried iff a VALUE-LEAVING event (a call /
	//     branch / loop / fallback residual — one that leaves a value the next op
	//     sees on top) fired strictly between the producer and its reference,
	//     tracked by producerIndex + leaverPrefix over the linear event stream. An
	//     in-order single use (no intervening leaver — `def y (…) y mul 2`, where
	//     the BIND_GLOBAL passes y through to the immediately-following mul) is NOT
	//     buried and keeps its cheaper stack lowering.
	//   - FRAGMENT (forEachFragmentOperand): a reference reaching UP OUT of a
	//     branch / loop arm cannot see the parent stack at all, so any such use is
	//     treated as buried (the arm's own linear order is not modelled here).
	//
	// A VARIADIC-returning producer is EXCLUDED from both: at runtime it may leave
	// 0 values (an empty if-arm, `maybe = if … [x] []`), so a STORE_LOCAL would
	// UNDERFLOW the VM — it must stay on the stack and refuse at layout if
	// unseatable, a sound interpreter fallback (the mk -1 crash, boru-lang/boru#261).
	// `buried` is taken from the operand scans only, before the residual `extra`
	// refs fold in below, so a RESIDUAL-only producer (a `def r (loop …) r`
	// bind-then-return tail) is never buried and its tail call survives
	// markTailCalls (which needs the call's result to reach the RET directly).
	producerIndex, leaverPrefix := computeLeaverPrefix(events)
	buried := map[int]bool{}
	// A producer whose result is the SOURCE of a loop-carried store (evStore)
	// must NOT be buried-promoted: lowerStore re-pushes that source natively from
	// the sim top (the loop-carried-store path — an event source that sits on the
	// sim top when the store lowers), so promoting it to a frame local only
	// diverts it to the operand path. Buried promotion exists for a value consumed
	// as a CALL / BRANCH arg, not a store source (`for … [if … [def acc (fn …)]]`
	// — the store source is the `fn` call, on top, natively re-pushed).
	//
	// Buried promotion applies to a DETACHED stamp (StampDetachedFn's isolated
	// fork, storedGradualDepth > 0) exactly as to the whole-program / module-load
	// compile: a recursive stored fn whose nested `if` arms read enclosing
	// value-defs computed by its recursive calls (voxgig-boru/template's
	// compile-hb-seq binds `head` and `tail` from recursive calls and reads both
	// inside a deeper arm) seats those defs as frame locals and compiles, where it
	// previously refused "branch reads enclosing computation". Validated by the
	// langspec census and the voxgig-boru --compile==interpret differential (all 7
	// libraries compile with zero divergences).
	storeSrc := collectStoreSourceSeqs(events)
	es.countRefsAndBurials(events, producerIndex, leaverPrefix, storeSrc,
		refs, fragRef, buried)
	// A SIDE-EFFECT loop (zeroOut: body nets 0 per iteration) lowers cleanly as an
	// UNCONSUMED statement (its result is dropped, RecordLoop marked it zeroOut).
	// But if its (zero-value) RESULT is CONSUMED — bound by `def x (for …)`
	// (valueDef) or fed as an operand to another event (refs>0 here, counted BEFORE
	// the residual `extra` below) — the interpreter's `def`/word forward-collection
	// over an empty producer GRABS THE NEXT TOKEN, which the compiled 0-value loop
	// does not replicate (off-corpus divergence: `def x (for n [print 0]) n` → interp
	// errors "got 0", compiled returns n). A 0-value CALL consumed the same way DOES
	// agree (both error), so the loop is the outlier — refuse and fall back to the
	// interpreter, scoping loop-in-fn-body lowering to genuinely discarded loops.
	for i := range events {
		seq := events[i].seq
		if events[i].kind == evLoop && es.eventInfo[seq].zeroOut &&
			(refs[seq] > 0 || es.eventInfo[seq].valueDef) {
			es.MarkUncompilable("for: side-effect loop result is consumed (Stage 3)")
		}
	}
	// `buried` (computed above) is the promote trigger for a value consumed
	// mid-body under an intervening event: a producer (user call OR branch
	// merge) whose result an INTERVENING def pushed off the single-consume
	// stack top, so the operand layout cannot seat it as an arg to a later
	// call ("result operand not on top" / "fn args not adjacent"). Storing it
	// in a frame slot re-pushes freely, exactly as the native value-def and
	// refs>=2 triggers already do — the interpreter's def-evaluates-once
	// semantics make it sound. It is taken from the OPERAND scan only, before
	// the residual `extra` refs fold in below, so a RESIDUAL-only producer (a
	// `def r (loop …) r` bind-then-return tail) is never buried and its tail
	// call survives markTailCalls (which runs after this pass and requires the
	// call's result to reach the RET directly).
	for _, seq := range extra {
		refs[seq]++
	}
	var promoted map[int]int
	var dead map[int]bool
	allEvents, fragInternal, crossFragRef := collectPromotableEvents(events)
	fragResult := fragmentResultSeqs(allEvents)
	// armForce names the event entries of a whole-captured MULTI-value arm
	// residual. Each must land in a frame slot: lowerFragment then finds the
	// arm's sim EMPTY and re-pushes the whole residual from the captured list,
	// in the interpreter's exact order. They join forceOrder (the program
	// residual's own linearisation trigger) and are exempt from the
	// stay-on-the-sim rule below — the top entry IS the arm's out operand, so
	// without the exemption fragResultStaysOnSim would pin it to the sim and
	// the re-push would never fire.
	armForce := armResidualForceSeqs(allEvents)
	if len(armForce) > 0 {
		merged := make(map[int]bool, len(forceOrder)+len(armForce))
		maps.Copy(merged, forceOrder)
		maps.Copy(merged, armForce)
		forceOrder = merged
	}
	// captured marks producers referenced by a CLOSURE CAPTURE (an each/fold/scan
	// body's lexical capture of an enclosing value-def). The capture resolves to a
	// frame local re-pushed at OpPushClosure (promoteOperand rewrites closureCaps);
	// it can never read a transient sim slot. A NATIVE captured value-def already
	// promotes via the valueDef trigger below, but a USER-call value-def
	// (`def mx (lst list-max)` captured by the radix each-loop) was shadowed by the
	// "leave single-use user call on the stack" case — so its list-max result was
	// left loose below the arm result ("branch leaves extra values"). Mark it so the
	// user-call promotion fires and the capture re-pushes from the slot.
	captured := map[int]bool{}
	for _, ev := range allEvents {
		eachClosureCap(ev, func(op EmitOperand) {
			if op.kind == opEvent && op.resIdx == 0 {
				captured[op.idx] = true
			}
		})
	}
	// storeSource marks a producer whose result is the SOURCE of a loop-carried
	// def rebind (an evStore — `def acc (nodes measure)` inside a for-arm, acc
	// pre-declared and carried). lowerStore seats such a source directly OFF THE
	// SIM TOP (OpStoreLocal pops it into the carried slot): the rebind's producing
	// dispatch immediately precedes the store, so the value is already on top and
	// needs no frame local. Promoting it to a value-def local instead would strand
	// the store looking for its source on the sim (it is now in a slot), forcing
	// the load-from-slot store path and — more to the point — an unnecessary local.
	// So the single-use valueDef promotion below EXEMPTS a plain store source (the
	// `def acc (nodes measure)` loop-carried rebind). A store source that is ALSO
	// captured / cross-fragment / dyn-env still promotes via the explicit triggers
	// (the capture or cross-fragment read genuinely needs the slot; the store then
	// loads it) — only the general single-use trigger backs off.
	storeSource := map[int]bool{}
	for _, ev := range allEvents {
		if ev.kind == evStore && ev.store.src.kind == opEvent {
			storeSource[ev.store.src.idx] = true
		}
	}
	for _, ev := range allEvents {
		// A fragment's residual event (an `if` arm / loop body result) PRODUCED INSIDE
		// the fragment must stay on that fragment's simulated stack for the arm-result
		// lowering, and a terminal tail call must stay terminal — never promote one.
		// But a fragResult that is NOT fragInternal is an ENCLOSING-scope value used as
		// an arm result (`def g (…); if c [1] [g]`, or the dynApply `def c (a b comp);
		// if (c gt 0) [c] e`): it lives on the PARENT sim, unreachable from the arm's
		// own fragment sim (lowerFragment resets lw.vm per fragment), so it MUST be
		// promoted to a frame local — the arm then re-pushes the slot (lowerFragment
		// re-resolves its captured opEvent `out` to the local). Without this the arm
		// refused "branch leaves extra values" (out=opEvent, len(lw.vm)==0).
		// A cross-NESTED-fragment reference (crossFragRef) is exempt: the
		// producer lives in one fragment and a DIFFERENT fragment's arm/out
		// consumes it — lowerFragment resets the sim per fragment, so the only
		// sound delivery is a unit-frame local (the trie-insert `def kid
		// (find-kid …)` in the outer arm, referenced as the inner if's arm-out).
		if !armForce[ev.seq] && es.fragResultStaysOnSim(ev.seq, refs, fragResult, fragInternal, crossFragRef) {
			continue
		}
		// A DEAD branch value-def — `def _ (if c [t] [e])` whose merge result is never
		// read — drops that result, like a dead call value-def: the interpreter binds
		// the if value to the dead name OFF the residual stack, so leaving the merge on
		// the sim left the enclosing fn body with an extra value ("body leaves extra
		// values (Stage 3)") — the recursive sorts' `def _ (if (n gt 1) [… quick-go]
		// [arr])` pattern. Only a 2-ARM if (hasElse): a no-else if is already a variadic
		// program-residual-only value the dead-drop must not touch.
		if ev.kind == evBranch {
			promoted, dead = es.planBranchPromotion(ev, unit, refs, buried, fragRef, fragInternal, forceOrder, promoted, dead)
			continue
		}
		// A single-result native call (evCall) OR user-fn call (evCallUser) can be
		// promoted to a frame local: its result stores once and re-pushes per
		// reference. (Without evCallUser, a user-call result above a literal in the
		// residual — `1 add2 2 3` → [1, 5] — could not be seated in order and
		// refused "call result above a literal".)
		var nout int
		isUser := false
		switch ev.kind {
		case evCall:
			nout = ev.call.nout
		case evCallUser:
			nout, isUser = ev.uc.nout, true
		default:
			continue
		}
		if nout != 1 {
			// A multi-output NATIVE stack word (dup → 2) whose results sit ABOVE
			// a residual literal (`0 0 dup` → [0, 0, 0]) can't be seated by the
			// in-order reconciliation. Force it to consecutive frame slots — one
			// per output — so each result re-pushes in residual order, the same
			// linearisation single-output events use. Only the forceOrder trigger
			// applies (a multi-output result is never a named value-def nor a
			// >=2-ref operand in Stage 1); a user fn's multi-return stays Stage 3.
			if !isUser && nout > 1 && forceOrder[ev.seq] {
				if promoted == nil {
					promoted = map[int]int{}
				}
				if _, done := promoted[ev.seq]; !done {
					promoted[ev.seq] = unit.numLocals
					unit.numLocals += nout
				}
			}
			continue
		}
		// A user-fn call (evCallUser) is promoted to fix an out-of-order residual
		// (forceOrder) — a user-call result above a literal, `1 add2 2 3` → [1, 5]
		// — OR when its result is referenced MORE THAN ONCE (refs>=2). A value
		// consumed by several uses cannot be seated from the single-consume stack
		// (the stack copy is gone after the first pop); it must be stored once and
		// re-pushed per use, which exactly matches the interpreter's def-evaluates-
		// once semantics (`def mv (x g)` runs g once, reads mv N times). Without
		// this a multi-referenced user-call value-def (`def m-val (derive-m …)`
		// read by both derive-k AND make-bits in Bloom.make) was left loose on the
		// stack, and a later call could not seat its operand on top — the
		// `fn arg result is not on top` refusal across the bloom/stats unit suites.
		// The remaining triggers (valueDef alone / fragRef / dead-result drop) stay
		// NATIVE-only: a SINGLE-use user call may feed a harness/accumulation the
		// residual ref count does not capture (Test.run-spec), where storing-once
		// or dropping its result diverges — but a refs>=2 value genuinely has
		// several consumers, so re-push is required and sound.
		// A user-call value-def captured by a closure (captured) joins the
		// forceOrder / multi-ref triggers: the capture needs the result in a frame
		// slot, so it must not be left loose on the sim (the radix list-max leaf).
		// A NAMED user-call value-def read from INSIDE a branch/loop fragment
		// (fragRef && !fragInternal) also promotes: the arm's own sim cannot reach
		// the parent stack, so without a slot the arm refuses "branch leaves extra
		// values" (out=opEvent, vm=0 — the trie-insert `def kid (nd ch find-kid);
		// … if … [kid]` shape recompiled under the unit-spec cascade). Storing a
		// named def once and re-pushing per reference IS the interpreter's
		// def-evaluates-once semantics, and the fragment read is a COUNTED ref —
		// unlike the uncounted harness/accumulation feed the single-use exclusion
		// above guards — so the store never diverges. Gated on valueDef, keeping
		// anonymous single-use harness feeds on the Stage-3 stack layout.
		// A NAMED user-call value-def (`def x (f …)`, marked valueDef via
		// MarkValueDef) promotes to a frame slot even when read ONCE: storing
		// once and re-pushing per reference IS the interpreter's
		// def-evaluates-once semantics, and a single-use def left LOOSE on the
		// sim cannot be seated once another computed operand is pushed above it
		// (`def a (nd g) … a b add c add` — the chained add's operands are "not
		// adjacent"). This mirrors the native-op value-def, which already
		// promotes unconditionally on valueDef below; the ANONYMOUS single-use
		// harness/accumulation feed (NOT a valueDef) stays on the Stage-3
		// layout, unaffected. EXCEPT a plain loop-carried store source
		// (storeSource): lowerStore seats it off the sim top, so promoting it
		// would strand the store and mint a needless local — the explicit
		// dyn-env / captured / fragment triggers above still promote a store
		// source that ALSO needs the slot for a capture or cross-fragment read.
		// buried (main's mid-body burial promotion) joins the trigger list
		// unchanged — its own storeSrc/variadic exclusions were applied at
		// marking time.
		promoteUser := isUser && (forceOrder[ev.seq] || refs[ev.seq] >= 2 || buried[ev.seq] ||
			(es.dynEnv && es.eventInfo[ev.seq].valueDef) ||
			(captured[ev.seq] && es.eventInfo[ev.seq].valueDef) ||
			(fragRef[ev.seq] && !fragInternal[ev.seq] && es.eventInfo[ev.seq].valueDef) ||
			(crossFragRef[ev.seq] && es.eventInfo[ev.seq].valueDef) ||
			(es.eventInfo[ev.seq].valueDef && !es.eventInfo[ev.seq].variadicResult && !storeSource[ev.seq]))
		// A DEAD value-def — `def _ (f …)` bound to a name referenced ZERO times —
		// drops its result for a USER call too, not only a native. The interpreter
		// binds the result to that name OFF the residual stack (the binding is the
		// only "use", and it is never read), so leaving the user-call result on the
		// stack left the fn body with an extra value ("body leaves extra values
		// (Stage 3)") — the recursive sorts' `def _l (… quick-go)` ignored-recursive-
		// result pattern. Gated on valueDef (an EXPLICIT def): this is NOT the SINGLE-
		// use (refs>=1) harness/accumulation case the comment guards against, where the
		// residual ref count can miss a dynamic read. The call's side effects run; only
		// its ignored return is dropped.
		deadValueDef := es.eventInfo[ev.seq].valueDef && refs[ev.seq] == 0 && !es.dynEnv
		switch {
		case isUser && !promoteUser && !deadValueDef:
			// leave the user-call result on the simulated stack (Stage-3 layout)
		case refs[ev.seq] == 0 && !forceOrder[ev.seq] && (!isUser || deadValueDef):
			// A single-result value-def bound to a name referenced zero times (a
			// dead binding, e.g. `def b (make C {…})` with b never used) — never the
			// program residual or a fragment read, both of which count toward refs.
			// The call still runs (side effects preserved), but its result is
			// discarded: lowerCall drops it rather than leaving it unconsumed on the
			// stack.
			if dead == nil {
				dead = map[int]bool{}
			}
			dead[ev.seq] = true
		case refs[ev.seq] >= 2 || es.eventInfo[ev.seq].valueDef || (fragRef[ev.seq] && !fragInternal[ev.seq]) || crossFragRef[ev.seq] || forceOrder[ev.seq]:
			// Promote to a frame slot (store now, re-push per reference) when the
			// result is referenced more than once, OR it is a NAMED value-def
			// (`def x (expr)`, marked via MarkValueDef), OR it is a UNIT-level producer
			// read from INSIDE a branch/loop fragment (fragRef && !fragInternal): a
			// binding may be consumed in any order, not just stack order — `def a
			// (make…) def b (make…) a.x … b.x` reads a (produced first, now deeper)
			// before b — and a cross-floor fragment read cannot reach the parent stack
			// at all. The single-consume simulated stack seats neither; a frame local
			// re-pushes freely from any scope. The fragRef trigger is gated on
			// !fragInternal: a producer INSIDE the fragment (its own intermediate) is
			// reachable on the fragment's local sim stack, and force-promoting it via
			// fragRef wrongly stored a tail call's argument and broke OpTailCallUser
			// detection — so a fragment-internal value promotes only via valueDef /
			// refs>=2. The extra STORE/PUSH for an in-order single use is harmless.
			if promoted == nil {
				promoted = map[int]int{}
			}
			if _, done := promoted[ev.seq]; !done {
				promoted[ev.seq] = unit.numLocals
				unit.numLocals++
			}
		default:
			// A single anonymous (non-def) use — stays on the simulated stack for
			// its one consumer; no local needed.
		}
	}
	if promoted != nil {
		for i := range events {
			RewritePromotedRefs(&events[i], promoted)
		}
	}
	return promoted, dead
}

// layoutMsgs holds the call-site-specific diagnostic wording for
// layoutOperands, so the shared engine can stay word/fn-agnostic while
// preserving each caller's exact reason strings.
// Each field is the refusal wording for one shape the spill fallback could not
// seat (a non-operand value interleaved on top of the simulated stack); when the
// spill succeeds — the common case — no message is used.
type layoutMsgs struct {
	loopResults  string // an operand is a variadic loop result
	resultNotTop string // the lone prior-result operand is not on top
	reorder      string // a single result operand needs reordering (>2 ops)
	shapeBeyond  string // operand count/shape the spill could not seat
	notAdjacent  string // two result operands not adjacent / not on top
}

// swapTop2 emits OpSwap and mirrors it on the simulated stack.
func (lw *lowerer) swapTop2(pos core.SrcPos) {
	lw.emit(OpSwap, 0, pos)
	lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2] = lw.vm[len(lw.vm)-2], lw.vm[len(lw.vm)-1]
}

// layoutOperands arranges call operands on the simulated stack so sig
// position 0 lands on top, emitting const/local/type pushes for inert
// operands and at most one SWAP to seat prior-result operands. It is the
// shared Stage-1/3 operand-shape engine for native calls (lowerCall) and
// user-fn calls (lowerUserCall) — they differ only in the diagnostic
// wording (msg) and the terminal call instruction each emits afterward.
// On success the len(ops) operands occupy the top slots in sig order and
// the caller pops them with its CALL_*; a non-empty return is the refusal.
func (lw *lowerer) layoutOperands(ops []EmitOperand, pos core.SrcPos, msg layoutMsgs) string {
	n := len(ops)
	results := []int{}
	for i, op := range ops {
		if op.kind == opEvent {
			if lw.variadic[op.idx] {
				return msg.loopResults
			}
			results = append(results, i)
		}
	}
	// N-event already-in-layout fast path. When EVERY operand is a prior-result
	// (event) operand AND they already sit on top of the simulated stack in sig
	// order (ops[0] on top, ops[1] next-deeper, …), no reordering is needed —
	// verify the positions and accept, emitting nothing. This generalises the
	// case-1 (ri==0/n-1) and case-2 already-in-layout paths to any N: a computed
	// list `[gensym gensym gensym]` or a call over N computed args
	// (`is-between (a) (b) (c)`) leaves its results adjacent and in order, which
	// the old 3+-results default refused outright. Soundness: it only ACCEPTS a
	// layout already correct (slotIs verifies each slot), so it can never seat an
	// operand wrongly; a non-matching layout falls through to the switch (and its
	// existing refusals) unchanged.
	if n > 0 && len(results) == n && len(lw.vm) >= n {
		allInPlace := true
		for i := 0; i < n; i++ {
			if !slotIs(lw.vm[len(lw.vm)-1-i], ops[i]) {
				allInPlace = false
				break
			}
		}
		if allInPlace {
			return ""
		}
	}
	// N-event reverse fast path (N≥3). When every operand is a prior-result
	// (event) operand sitting in exact REVERSE sig order on top — ops[n-1] on
	// top, ops[0] deepest — one OpReverse seats them in sig order. This is the
	// common forward-call shape `f (a)(b)(c)`: the args evaluate left→right so
	// sig position 0 lands DEEPEST, but the call wants it on top. N=2 is the
	// case-2 SWAP below; N≥3 needed the 3-deep rotate the VM lacked. Like the
	// in-layout fast path it only ACCEPTS an exactly-recognised layout (slotIs
	// verifies every slot), so it can never seat an operand wrongly.
	if n >= 3 && len(results) == n && len(lw.vm) >= n {
		allReversed := true
		for i := 0; i < n; i++ {
			if !slotIs(lw.vm[len(lw.vm)-1-i], ops[n-1-i]) {
				allReversed = false
				break
			}
		}
		if allReversed {
			lw.emit(OpReverse, n, pos)
			for a, b := len(lw.vm)-n, len(lw.vm)-1; a < b; a, b = a+1, b-1 {
				lw.vm[a], lw.vm[b] = lw.vm[b], lw.vm[a]
			}
			return ""
		}
	}
	switch len(results) {
	case 0:
		// Push consts/locals deepest-first so sig position 0 lands on top.
		for i := n - 1; i >= 0; i-- {
			lw.pushOperand(ops[i], pos)
		}
	case 1:
		ri := results[0]
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], ops[ri]) {
			return msg.resultNotTop
		}
		switch {
		case ri == n-1:
			// The lone prior-result operand is the DEEPEST sig position: push
			// the const/local operands above it deepest-first, so sig 0 lands
			// on top. No reordering needed.
			for i := n - 1; i >= 0; i-- {
				if i == ri {
					continue
				}
				lw.pushOperand(ops[i], pos)
			}
		case ri == 0:
			// The result operand is sig position 0 — it must end on TOP, with
			// the const/local operands below it. Push each deeper operand above
			// the result then SWAP the result back to the top, settling that
			// operand into its (deeper) place; repeat sig(n-1) down to sig 1.
			// n==2 is the single push+swap; n>2 chains it (e.g. a computed
			// receiver `setpath (make…) "k" v`, sig 0 = receiver on top).
			for j := n - 1; j >= 1; j-- {
				lw.pushOperand(ops[j], pos)
				lw.swapTop2(pos)
			}
		default:
			// The result operand sits in a MIDDLE sig position (0 < ri < n-1):
			// the cheap stack paths can't seat it (a 3-deep rotate). Spill the
			// event operand to a frame-local destination and re-push in sig order.
			return lw.spillSeat(ops, results, n, pos, msg.reorder)
		}
	case 2:
		if n == 2 && len(lw.vm) >= 2 {
			top, below := lw.vm[len(lw.vm)-1], lw.vm[len(lw.vm)-2]
			switch {
			case slotIs(top, ops[0]) && slotIs(below, ops[1]): //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return "" // already in layout
			case slotIs(top, ops[1]) && slotIs(below, ops[0]):
				lw.swapTop2(pos)
				return ""
			}
		}
		// Two events with inert operands (n>2), or two events not cleanly adjacent
		// on top: spill the events to frame-local destinations and re-push all
		// operands in sig order.
		return lw.spillSeat(ops, results, n, pos, msg.notAdjacent)
	default:
		// Three or more event operands not already in (forward or reverse) sig
		// order: spill them to frame-local destinations and re-push in sig order.
		return lw.spillSeat(ops, results, n, pos, msg.shapeBeyond)
	}
	return ""
}

// seatMsgs carries a seatResults caller's exact refusal wording, so the shared
// seat primitive stays caller-agnostic while preserving each site's reasons.
type seatMsgs struct {
	variadic     string // an event operand is a variadic loop result (when rejected)
	aboveLiteral string // an event operand sits above an already-pushed inert tail
	reordered    string // an event operand is not the next simulated-stack slot
	unconsumed   string // simulated-stack results remain after seating all operands
}

// seatResults arranges a sequence of result operands (bottom→top) as the final
// stack. Each event operand must already sit on the simulated stack in order —
// left there by its own event — and inert operands (const / local / type) are
// pushed as a trailing tail above the last event result. It is the shared core
// of the program-residual reconciliation (Finalize) and the fn-unit RET
// reconciliation (reconcileResults). rejectVariadic refuses a variadic (loop)
// event result: a fn body may not return one (Stage 3), though the program
// residual may absorb it. msgs supplies the caller's refusal wording.
func (lw *lowerer) seatResults(ops []EmitOperand, rejectVariadic, allowVariadicTail bool, msgs seatMsgs, pos core.SrcPos) string {
	vi := 0
	var tail []EmitOperand
	for i, op := range ops {
		if op.kind == opEvent {
			if rejectVariadic && lw.variadic[op.idx] {
				// A no-contract (`[]`-declared) fn may return a VARIADIC tail: its
				// RET leaves whatever the body left, exactly like the program
				// residual. Permit a variadic event in the LAST position only — a
				// variadic with fixed values ABOVE it cannot seat (the count is
				// runtime-variable) — OR a CONTIGUOUS run of the SAME variadic
				// event's results reaching the end (a multi-out dyn-body `do`
				// whose whole runtime residual lands together; the fixed static
				// model above it is unaffected, and anything pushed after would
				// sit above the runtime run exactly as recorded).
				if !(allowVariadicTail && i == len(ops)-1) && !sameEventRunToEnd(ops[i:], op.idx) {
					return msgs.variadic
				}
			}
			if len(tail) > 0 {
				return msgs.aboveLiteral
			}
			if vi >= len(lw.vm) || !slotIs(lw.vm[vi], op) {
				return msgs.reordered
			}
			vi++
			continue
		}
		tail = append(tail, op)
	}
	if vi != len(lw.vm) {
		return msgs.unconsumed
	}
	for _, op := range tail {
		lw.pushOperand(op, pos)
	}
	return ""
}

// seatProgramResidual lays out the PROGRAM's residual as the final stack and
// returns the refusal, if any. Two layouts, tried in order: the region-prefix
// seating, which closes a residual shaped [inert…, REGION] through the mark
// the plan opened; then the ordinary in-order seating, which owns every other
// shape and whose wording is the honest one for anything the first declines.
//
// Split out of Finalize rather than written inline because Finalize sits on
// the gocyclo ceiling: one more branch there is one branch too many, and this
// choice belongs beside the two seatings anyway.
func (lw *lowerer) seatProgramResidual(ops []EmitOperand, vals []core.Value, pos core.SrcPos) string {
	if lw.seatRegionPrefix(ops, pos) {
		return ""
	}
	reason := lw.seatResults(ops, false, false, seatMsgs{
		aboveLiteral: "residual shape beyond Stage 1 (call result above a literal)",
		reordered:    "residual shape beyond Stage 1 (call results reordered)",
		unconsumed:   "residual shape beyond Stage 1 (unconsumed call results)",
	}, pos)
	if reason == "" {
		return ""
	}
	// seatResults declined, and it emits nothing when it does — so the
	// rebuild below starts from the same stack it saw. A residual that may
	// carry a CALLABLE does not take it: see seatResidualRebuild.
	if !regionValsMayBeCallable(vals) && lw.seatResidualRebuild(ops, pos) {
		return ""
	}
	return reason
}

// seatResidualRebuild lays out a program residual whose ORDER is not the
// order the events produced it in, and reports whether it did. It is the
// program-residual twin of spillSeat (the call-site DDCG fallback): spill
// every simulated-stack entry to a fresh frame local, then push the residual
// back exactly as recorded — an event result from its spill temp, an inert
// operand from its own push.
//
// seatResults above owns the shapes a static lowering can seat IN PLACE
// (the events already in production order, inert values above them) and
// costs nothing when it applies; this owns everything else the full-stack
// words produce. `(1 add 2) (3 add 4) 1 roll` is the frontier row: the fold
// models the permutation exactly, and the two call results then have to be
// re-pushed swapped — no static offset reaches past a value that is already
// on the stack. The same rebuild covers a residual that DUPLICATES a call
// result (`0 pick`), one that DROPS a computed value, and one that seats an
// inert value BENEATH a call result.
//
// It declines — leaving the emitted code untouched, so the caller's refusal
// stands — for a residual that may carry a CALLABLE (the caller's screen,
// regionValsMayBeCallable) and for three shapes a spill cannot honour.
//
// The CALLABLE screen is the one that is about semantics rather than
// mechanism, and it is load-bearing. A value that ARRIVES on the stack is
// re-stepped — a Function dispatches over what is beneath it (NUR124's
// rule, and NUR129's open edge) — and the interpreter's own shuffle puts a
// picked or rolled fn back on the tape where the pointer fires it. A
// re-push here is a DATA push, so a rebuilt residual holding a closure
// would answer `[5 fn fn]` where the interpreter applies it and answers
// `[45]` (`def mk … 5 (mk 3) 0 pick`), and `[fn 5]` for its `1 roll` twin's
// `[15]`. Declining keeps the pre-existing refusal and the interpreter's
// answer. The screen is deliberately WIDE — a Dynamic residual entry counts
// as possibly-callable, because the model does not bound it — which is the
// same trade NUR129 records for the region consumers.
//
// The three mechanical declines:
//
//   - a VARIADIC region operand, whose runtime run is not one spillable
//     stack entry (the count is not the static seat);
//   - an event operand that is not on the simulated stack at all, so there
//     is no value to spill for it;
//   - an armed mark plan (the mark window, the region prefix, the region
//     collect), whose OpStackMark is already emitted and indexes the very
//     stack a spill would empty.
func (lw *lowerer) seatResidualRebuild(ops []EmitOperand, pos core.SrcPos) bool {
	if len(lw.vm) == 0 || len(lw.markBefore) > 0 || lw.regionPrefixSeq != 0 || lw.collectAtSeq != 0 {
		return false
	}
	for _, op := range ops {
		if op.kind != opEvent {
			continue
		}
		if lw.variadic[op.idx] || !lw.simHolds(op) {
			return false
		}
	}
	// Spill top-down. A slot already spilled keeps its FIRST temp: two
	// residual entries naming one call result (the `pick` shape) read the
	// same local twice.
	temp := make(map[vmSlot]int, len(lw.vm))
	for len(lw.vm) > 0 {
		slot := lw.vm[len(lw.vm)-1]
		t := lw.allocLocal()
		lw.emit(OpStoreLocal, t, pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		if _, seen := temp[slot]; !seen {
			temp[slot] = t
		}
	}
	for _, op := range ops {
		if op.kind != opEvent {
			lw.pushOperand(op, pos)
			continue
		}
		lw.emit(OpPushLocal, temp[vmSlot{seq: op.idx, idx: op.resIdx}], pos)
		lw.vm = append(lw.vm, nonEventSlot)
	}
	lw.note()
	return true
}

// simHolds reports whether an event operand's value is on the simulated
// stack — the precondition for spilling it to a frame local.
func (lw *lowerer) simHolds(op EmitOperand) bool {
	for _, slot := range lw.vm {
		if slotIs(slot, op) {
			return true
		}
	}
	return false
}

// seatRegionPrefix seats a residual shaped [inert…, REGION] and reports
// whether it did (NUR067's consuming half). planRegionPrefix armed the plan
// and opened an OpStackMark before the region's producing event; the region's
// own run is now the sim's ONE remaining entry, so the prefix pushes ABOVE it
// — the only place a static lowering can put it — and OpSeatBelowMark moves
// the prefix down to the mark, lifting the run above it without ever naming
// the run's length.
//
// Returns false — leaving the emitted code untouched — whenever the plan is
// not armed (every ordinary program) or the post-lowering stack is not the
// bare region the plan expected. The caller then takes the ordinary seating,
// whose refusal ("call result above a literal") is the honest one for a shape
// this could not close; the unused OpStackMark is emitted into a program that
// never runs.
func (lw *lowerer) seatRegionPrefix(ops []EmitOperand, pos core.SrcPos) bool {
	if lw.regionPrefixSeq == 0 || len(lw.vm) != 1 || lw.vm[0].seq != lw.regionPrefixSeq {
		return false
	}
	// The region's RUN is the trailing operands the plan's producer left, and
	// there may be more than one: a do-catch records nout seats for a run
	// whose runtime length is a different number. They lowered to the ONE
	// variadic sim slot checked above, so what matters here is only where the
	// prefix ends.
	n := len(ops)
	for n > 0 && ops[n-1].kind == opEvent && ops[n-1].idx == lw.regionPrefixSeq {
		n--
	}
	if n == len(ops) || n < 1 {
		return false
	}
	// Every operand beneath the region is inert by construction —
	// regionPrefixShape admitted the residual only because none of those
	// entries had a producing event — so each is a plain push.
	for _, op := range ops[:n] {
		lw.pushOperand(op, pos)
	}
	lw.emit(OpSeatBelowMark, n, pos)
	// Model the seated layout: the prefix beneath the region's one slot.
	lw.vm = lw.vm[:0]
	for range ops[:n] {
		lw.vm = append(lw.vm, nonEventSlot)
	}
	lw.vm = append(lw.vm, vmSlot{seq: lw.regionPrefixSeq, idx: 0})
	lw.note()
	return true
}

// planBranchPromotion classifies one evBranch merge result for
// planValueDefLocals: a DEAD 2-arm value-def (`def _ (if c [t] [e])`, never
// read) drops its result — the interpreter binds the merge OFF the residual
// stack — EXCEPT under DynEnv, where no def is dead (a dynamic code body may
// read any binding, so its OpBindDynScope twin needs the value live). A
// 2-arm if-RESULT bound to a name and read more than once / across a
// fragment floor / under DynEnv promotes to a frame local: a single sim copy
// is consumed by the first use (bucket's `bcount set bi ((bcount get bi) add
// 1)`). Gated to a SINGLE-value merge so the store seats exactly one value —
// a multi-value / variadic merge is refused at the store hook.
func (es *EmitState) planBranchPromotion(ev *EmitEvent, unit *emitUnit, refs map[int]int, buried, fragRef, fragInternal, forceOrder map[int]bool, promoted map[int]int, dead map[int]bool) (map[int]int, map[int]bool) {
	// A forceOrder branch source is NEVER dead: its binding is a real
	// runtime consumer (a dyn-bound name's OpBindDynScope re-push, or an
	// arm-resident install re-pushing per element — possibly for TWO defs
	// off one merge, row 41's `def res (if …) def _2 res`), so it takes
	// the store-once / re-push-per-use promotion below instead.
	if ev.br != nil && ev.br.hasElse && es.eventInfo[ev.seq].valueDef && refs[ev.seq] == 0 &&
		!es.dynEnv && !forceOrder[ev.seq] {
		if dead == nil {
			dead = map[int]bool{}
		}
		dead[ev.seq] = true
		return promoted, dead
	}
	if branchSingleValue(ev.br) && (es.eventInfo[ev.seq].valueDef || forceOrder[ev.seq]) &&
		(refs[ev.seq] >= 2 || buried[ev.seq] || (fragRef[ev.seq] && !fragInternal[ev.seq]) ||
			es.dynEnv || forceOrder[ev.seq]) {
		if promoted == nil {
			promoted = map[int]int{}
		}
		if _, done := promoted[ev.seq]; !done {
			promoted[ev.seq] = unit.numLocals
			unit.numLocals++
		}
	}
	return promoted, dead
}

// collectRootBindConsumes returns the DEAD producer seqs whose single result
// a root OpBindGlobal write-back consumes in Pop mode — the producer's
// dead-drop is suppressed (lowerCall / lowerUserCall / lowerUserPoly / the
// dead-branch arm) so the value stays on the stack for the immediately-
// following evDynBind to pop into the kept binding slot. The gate mirrors
// lowerDynBind's needGlobal exactly.
// collectResidentBindConsumes is collectRootBindConsumes' fn-unit twin
// for ARM-RESIDENT def sites (§6.5's each-body recovery): a stamped
// resident def whose computed source is refcount-DEAD (a def whose only
// "reader" is the binding itself — `def res (if ok [1] [2])` read once
// through the registry, never by provenance) needs the producer's drop
// suppressed so the resident install can consume the value from the sim
// top, exactly the root OpBindGlobal write-back's bind-consumes
// discipline.
func collectResidentBindConsumes(events []EmitEvent, dead map[int]bool) map[int]bool {
	all, _, _ := collectPromotableEvents(events)
	out := map[int]bool{}
	for _, ev := range all {
		if ev.kind != evDynBind || ev.dyn == nil {
			continue
		}
		if d := ev.dyn; d.residentTwin >= 0 && d.srcSeq >= 0 && dead[d.srcSeq] {
			out[d.srcSeq] = true
		}
	}
	return out
}

func collectRootBindConsumes(events []EmitEvent, dead map[int]bool) map[int]bool {
	all, _, _ := collectPromotableEvents(events)
	out := map[int]bool{}
	for _, ev := range all {
		if ev.kind != evDynBind || ev.dyn == nil {
			continue
		}
		d := ev.dyn
		if d.root && d.srcSeq >= 0 && dead[d.srcSeq] &&
			!core.IsConcrete(d.val) && !core.IsBareTypeNode(d.val) {
			out[d.srcSeq] = true
		}
	}
	return out
}

// collectDynBindSources returns the COMPUTED source-event seqs of every
// def bind that will lower to a registry-visible twin — dyn-bound defs under
// DynEnv / dynScopeNames (OpBindDynScope) and root-unit non-concrete defs
// (OpBindGlobal) — the promotion set planValueDefLocals feeds into its
// triggers so lowerDynBind can re-push each value from a frame slot for its
// install.
func (es *EmitState) collectDynBindSources(events []EmitEvent, deoptNames map[string]bool) map[int]bool {
	dynBindSrc := map[int]bool{}
	for i := range events {
		if events[i].kind != evDynBind || events[i].dyn == nil || events[i].dyn.srcSeq < 0 {
			continue
		}
		// A def's computed source is promoted for its OpBindDynScope install
		// when the bind actually LOWERS: under dynEnv (every def is registry-
		// visible) OR when this specific name is dynamically read
		// (dynScopeNames — e.g. a module-scope `flex` read from a fn body). The
		// gate mirrors lowerDynBind's, so a source is promoted exactly when the
		// bind re-pushes it. (Root-unit OpBindGlobal write-backs deliberately do
		// NOT force promotion: a def binds immediately after its producing
		// event, so the peek fast path reads the live top and every lowering
		// shape stays byte-identical; a source promoted by the ordinary
		// triggers re-pushes in Pop mode instead.)
		if es.dynEnv || deoptNames[events[i].dyn.name] || (es.dynScopeNames != nil && es.dynScopeNames[events[i].dyn.name]) ||
			// An ARM-RESIDENT body compile (the each-unit bracket, regime
			// only): every def's computed source is force-promoted so the
			// resident install can re-push it from a frame slot — a body
			// like row 41's binds one def's value from another's read
			// (`def res (if …) def _2 res`), where the shared source can
			// sit under later pushes at the second install site. Same
			// store-once / re-push-per-use discipline as the dyn-bound
			// sources above.
			es.armResidentDepth > 0 {
			dynBindSrc[events[i].dyn.srcSeq] = true
		}
	}
	return dynBindSrc
}

// singleOutputCall reports whether ev is a single-result native or user call —
// the one producer shape promoteLateDynBind can seat as a lone frame local. A
// nil event (a source in a multi-value arm collectPromotableEvents does not
// recurse into) is not one.
func singleOutputCall(ev *EmitEvent) bool {
	if ev == nil {
		return false
	}
	switch ev.kind {
	case evCall:
		return ev.call.nout == 1
	case evCallUser:
		return ev.uc.nout == 1
	default:
		return false
	}
}

// promoteLateDynBind seats a finished fn unit's dyn-bound COMPUTED def sources
// into frame locals, for the case where es.dynEnv armed AFTER the unit's
// value-def promotion was planned (a later tryRecordDynBody — e.g. a `do {…}`
// map body in another fn, or a dynamically-dispatched subject — widens the
// program to DynEnv). At plan time es.dynEnv was still false, so
// planValueDefLocals' `(es.dynEnv && valueDef)` trigger did not fire and the
// source was left on the single-consume sim stack; Finalize then lowers the
// unit widened and lowerDynBind refuses "unpromoted computed value". This
// applies exactly that trigger, deferred, over rec.frag's recorded events
// (recursing arms/bodies via collectPromotableEvents). It is a no-op unless
// es.dynEnv is set, so a program that never becomes DynEnv is byte-for-byte
// unchanged; and a unit that finished AFTER the arming already promoted these
// (they are in rec.promoted) so it is idempotent. A source that is not a
// single-output call — a fragment RESULT that must stay on its sim, a
// makeMap/branch/loop value, a multi-output OR variadic-returning producer —
// is left untouched for lowerDynBind to refuse, a sound interpreter fallback
// (never a wrong store).
func (es *EmitState) promoteLateDynBind(rec *fnUnitRec) {
	if rec == nil || rec.frag == nil || !(es.dynEnv || rec.deoptEnv) {
		return
	}
	allEvents, _, _ := collectPromotableEvents(rec.frag.events)
	fragResult := fragmentResultSeqs(allEvents)
	bySeq := make(map[int]*EmitEvent, len(allEvents))
	for _, ev := range allEvents {
		bySeq[ev.seq] = ev
	}
	changed := false
	for _, ev := range allEvents {
		if ev.kind != evDynBind || ev.dyn == nil || ev.dyn.srcSeq < 0 {
			continue
		}
		seq := ev.dyn.srcSeq
		if _, done := rec.promoted[seq]; done {
			continue
		}
		if !es.dynEnv && !rec.deoptNames[ev.dyn.name] {
			// A deopt unit promotes only the defs its islands read.
			continue
		}
		// A VARIADIC-returning producer's static nout==1 is only the check-run
		// count, not a runtime guarantee: a `def v (maybe x)` over `maybe = if …
		// [x] []` leaves 0 values when the empty arm runs. Promoting it emits a
		// lone OpStoreLocal that underflows the VM stack (compile != interpret).
		// Leave it for lowerDynBind to refuse — a sound interpreter fallback.
		if fragResult[seq] || es.eventInfo[seq].variadicResult || !singleOutputCall(bySeq[seq]) {
			continue
		}
		if rec.promoted == nil {
			rec.promoted = map[int]int{}
		}
		rec.promoted[seq] = rec.numLoc
		rec.numLoc++
		changed = true
	}
	if changed {
		for i := range rec.frag.events {
			RewritePromotedRefs(&rec.frag.events[i], rec.promoted)
		}
		for i := range rec.outOps {
			promoteOperand(&rec.outOps[i], rec.promoted)
		}
	}
}

// sameEventRunToEnd reports whether ops is entirely the SAME event's results
// in ascending result order — the contiguous multi-out variadic run the
// seatResults relaxation admits (all of a dyn-body do's outputs land together
// at run time, whatever their count).
func sameEventRunToEnd(ops []EmitOperand, idx int) bool {
	for i, op := range ops {
		if op.kind != opEvent || op.idx != idx || op.resIdx != i {
			return false
		}
	}
	return true
}

// reconcileResults arranges a unit's N result operands (bottom→top) as the
// final stack, ready for a RET. who prefixes the refusal reason ("fn name").
// This is the fn-unit caller of the shared seatResults primitive — it rejects a
// variadic loop result (a fn body may not return one in Stage 3), the one way it
// differs from Finalize's program-residual reconciliation.
//
// On a decline it takes the same REBUILD fallback the program residual has
// (seatResidualRebuild): seatResults emits nothing when it refuses, so the
// rebuild starts from the stack it saw. Not having it here was the whole of
// several refusals — `do [def b true  do [1 (if b [] [9 9])]]` refuses "fn
// do$body: result above a literal (Stage 3)", and adding one more literal
// makes the do-body closure decline instead, which loses the outer body's
// def twin and refuses at the placement gate. The two screens the caller
// supplies are what makes a body unit different from the program residual:
//
//   - vals is the residual's VALUES for the CALLABLE screen, which a rebuild
//     may never cross (a re-push is a DATA push; the interpreter re-steps an
//     arriving Function — NUR124, and NUR129's open edge);
//   - allowRebuild is false for the residual shapes whose own post-processing
//     reads the seated layout (a body-tail dynamic apply, a whole-frame
//     replay), and for a residual holding a RUNTIME-VARIABLE-count event,
//     which no single spill slot can stand for. The program residual absorbs
//     such an event; a fn RET does not.
func (lw *lowerer) reconcileResults(ops []EmitOperand, who string, noContract, variadicMid, allowRebuild bool, vals []core.Value, pos core.SrcPos) string {
	extra := who + ": body leaves extra values (Stage 3 lowers in-order results)"
	reason := lw.seatResults(ops, !variadicMid, noContract, seatMsgs{
		variadic:     who + ": result is a variadic loop value (Stage 3)",
		aboveLiteral: who + ": result above a literal (Stage 3)",
		reordered:    extra,
		unconsumed:   extra,
	}, pos)
	if reason == "" {
		return ""
	}
	if allowRebuild && !regionValsMayBeCallable(vals) &&
		!lw.opsHaveVariadicResult(ops) && lw.seatResidualRebuild(ops, pos) {
		return ""
	}
	return reason
}

// opsHaveVariadicResult reports whether any operand names an event whose
// RESULT COUNT is runtime-variable (eventFlags.variadicResult — a loop, or a
// branch whose arms leave different counts). lw.variadic covers the loop
// seqs the lowering itself marked; this is the recorder's own structural
// mark, and it is the one a fn-unit rebuild must ask: spilling such a run to
// one frame local stores one value for a run of a different length.
func (lw *lowerer) opsHaveVariadicResult(ops []EmitOperand) bool {
	if lw.es == nil {
		return false
	}
	for _, op := range ops {
		if op.kind == opEvent && lw.es.eventInfo[op.idx].variadicResult {
			return true
		}
	}
	return false
}

func (lw *lowerer) lowerCall(ev *EmitEvent) string {
	c := &ev.call
	if lw.collectRegionTop(ev) {
		return ""
	}
	n := len(c.ops)
	if reason := lw.layoutOperands(c.ops, c.pos, layoutMsgs{
		loopResults:  "consumes loop results (Stage 2 loops only feed the program residual)",
		resultNotTop: "stack discipline: result operand of " + c.word + " is not on top",
		reorder:      "operand shape at " + c.word + " needs reordering beyond Stage 1",
		shapeBeyond:  "operand shape at " + c.word + " beyond Stage 1",
		notAdjacent:  "stack discipline: operands of " + c.word + " not adjacent on top",
	}); reason != "" {
		return reason
	}
	// The region descriptor lands HERE, not on the recorder, so it shares the
	// event's rollback: a discarded loop-analysis round takes its descriptors
	// with it. Appended before the opcode is chosen because it describes the
	// DISPATCH, whichever arm below emits for it — the index is the region's,
	// not any one opcode's. Nothing reads Program.Regions yet (§6.5's OpCollect
	// is the client); this is the inert table, gated over the corpus by
	// TestRegionTableWellFormed before anything executes one.
	if c.region != nil {
		lw.p.Regions = append(lw.p.Regions, *c.region)
	}
	if c.typedBind != nil {
		// A typed value-def's runtime validate/reparent step: pop the body
		// operand (laid out above), run the interpreter-mirroring RunTypedBind,
		// push the bound value. The spec rides in TypedBinds like a trap/map spec.
		ti := len(lw.p.TypedBinds)
		lw.p.TypedBinds = append(lw.p.TypedBinds, *c.typedBind)
		lw.emit(OpBindTyped, ti, c.pos)
	} else if c.dynApply > 0 {
		// Apply the TOP operand (a runtime fn VALUE) to the `dynApply` trailing args
		// laid out below it — a paren-bounded trailing fn-value apply (`(a b comp)`)
		// recorded as an event (RecordDynApply) so it seats like any computed result:
		// a def-local, an if operand, a list member, OR the body residual. The layout
		// above placed the operands [args…, fn] with the fn on top, exactly the stack
		// OpCallDynTrailTop reads (fn = top, its dynApply args below). An event that
		// came through the `apply` WORD lowers to OpCallDynApplyTop instead — the
		// applyHandler's unquote-then-apply (Stage M2a).
		op := OpCallDynTrailTop
		if c.dynApplyUnquote {
			op = OpCallDynApplyTop
		}
		if c.dynApplyOne {
			// A GRADUAL lead under the apply word: exactly one result or defer.
			op = OpCallDynApplyOne
		}
		if c.dynApplyKeepQuote {
			// An event-provenance fn: the runtime quote state survives
			// (no read substitution to mirror) — see OpCallDynTrailKeepQ.
			op = OpCallDynTrailKeepQ
		}
		// The head's binding name for the op's own no-match diagnostic. The
		// `apply` word's flavour is excluded: applyHandler re-steps the fn
		// against the whole preceding stack, a different dispatch whose
		// diagnostics this pair does not describe.
		if op != OpCallDynApplyTop && op != OpCallDynApplyOne {
			lw.seatDynApplyName(c.dynApplyName)
		}
		lw.emit(op, c.dynApply, c.pos)
	} else if c.dynMixed {
		// Forward-drift window (REFUSAL-CLOSURE §1): the layout placed
		// [leading residual(s), dynamic value, word const, forward literal];
		// the VM islands the window verbatim — the word token dispatches in
		// the island with the interpreter's own forward collection over the
		// LIVE top value, so the value-dependent binding (and its residual
		// count) is byte-identical on every runtime path.
		lw.emit(OpCallDynamicMixed, len(c.ops), c.pos)
	} else if c.dynMethod != nil {
		// Guarded shaped-instance-method apply (Stage M2c): the layout above
		// placed [fn, a1..aN] with the fn at the base (source order — the
		// leading-boundary stack shape). OpCallDynMethod pops N+1, applies the
		// runtime fn to the N args exactly as the interpreter's forward
		// auto-dispatch, and enforces the spec's claimed result count; a claim
		// failure raises internal_error → interpreter re-run (never a wrong
		// stack). The spec rides in DynMethods like a trap/map spec.
		di := len(lw.p.DynMethods)
		lw.p.DynMethods = append(lw.p.DynMethods, *c.dynMethod)
		lw.emit(OpCallDynMethod, di, c.pos)
	} else if c.makeList {
		// Assemble the n laid-out operands into a list (a computed list literal,
		// `[1 add 2]`). No sig, no dispatch — OpMakeList pops the n and pushes one.
		lw.emit(OpMakeList, n, c.pos)
	} else if c.makeMap {
		// Assemble the n laid-out VALUE operands into a map (a computed make
		// body, `make Outer {i:(make Inner …)}`); the keys ride in MakeMaps.
		mi := len(lw.p.MakeMaps)
		lw.p.MakeMaps = append(lw.p.MakeMaps, MakeMapSpec{Keys: c.mapKeys, Implicit: c.mapImpl})
		lw.emit(OpMakeMap, mi, c.pos)
	} else if c.spliceDyn {
		// Spread the laid-out payload at run time (§9.2b) — value payloads
		// spread verbatim, code-bearing ones defer to the interpreter.
		lw.emit(OpSpliceDyn, 0, c.pos)
	} else if c.xmlTmpl != nil {
		// Assemble the n laid-out hole operands into an interpolated XML
		// element (§9.2c); the template skeleton rides in XmlInterps.
		xi := len(lw.p.XmlInterps)
		lw.p.XmlInterps = append(lw.p.XmlInterps, XmlInterpSpec{Tmpl: *c.xmlTmpl, NHoles: n})
		lw.emit(OpInterpXml, xi, c.pos)
	} else if c.interp {
		// Assemble the n laid-out hole operands into a template string (a computed
		// interpolation, `` `got ${x}` ``); the literal segments ride in Interps.
		ii := len(lw.p.Interps)
		lw.p.Interps = append(lw.p.Interps, InterpSpec{Segs: c.interpSegs, NHoles: n})
		lw.emit(OpInterp, ii, c.pos)
	} else if c.poly {
		// Runtime-matched dispatch: no baked sig, the VM re-matches over the
		// word's signatures against the n stack values.
		pi := len(lw.p.PolyRefs)
		lw.p.PolyRefs = append(lw.p.PolyRefs, PolyRef{Word: c.word, Arity: n, NOut: c.nout, Reg: c.polyReg, NoMatch: c.polyNoMatch})
		lw.emit(OpCallNativePoly, pi, c.pos)
	} else {
		si, ok := lw.sigIdx[c.sig]
		if !ok {
			lw.p.Sigs = append(lw.p.Sigs, SigRef{Word: c.word, Sig: c.sig})
			si = len(lw.p.Sigs) - 1
			lw.sigIdx[c.sig] = si
		}
		lw.emit(OpCallNative, si, c.pos)
	}
	lw.vm = lw.vm[:len(lw.vm)-n]
	// A VARIADIC REGION result (NUR067's growing direction): the handler
	// leaves 0-or-MORE values where this event carries ONE recorded slot.
	// Take the LOOP region's representation — mark the slot lw.variadic, so
	// every rule a value-producing loop's region already obeys applies here
	// verbatim: layoutOperands refuses it as a call/list operand, seatResults
	// admits it only in a variadic-absorbing LAST position (the program
	// residual, a no-contract RET), and the store/bind hooks refuse it. The
	// two dispositions BELOW both need a static count — a promotion stores
	// exactly nout values, a dead-result drop pops exactly one — so neither
	// can serve a run whose size is a runtime value; refuse instead, the
	// earliest true diagnosis, and the interpreter owns the program.
	if lw.es != nil && (lw.es.eventInfo[ev.seq].variadicRegion || lw.regionPrefixSeq == ev.seq) {
		if _, prom := lw.promoted[ev.seq]; prom {
			return c.word + ": variadic region promoted to a frame slot (the runtime count is not the static seat)"
		}
		if lw.dead[ev.seq] {
			return c.word + ": variadic region result discarded (the runtime count is not the static seat)"
		}
		lw.variadic[ev.seq] = true
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: 0})
		lw.note()
		return ""
	}
	// A promoted result: store it into a frame slot now and re-push it per
	// reference / per residual position (the references were rewritten to local
	// operands). A single-result value-def stores one slot; a multi-output stack
	// word forced to slots (dup, an out-of-order residual) stores idx 0..nout-1
	// to slot..slot+nout-1 — top (highest idx) first, since OpStoreLocal pops the
	// top — so the residual re-pushes them in idx order. Nothing is left on the
	// simulated stack either way.
	if slot, ok := lw.promoted[ev.seq]; ok {
		// A MULTI-OUT VARIADIC call result has no fixed arity to seat: the
		// stores pop exactly nout values while the runtime count is
		// variable — a fallible catch region delivers ONE caught Error on
		// the raise path (probe-pinned: `def x (do [(0 div 0) "a" "b"]
		// error [dot code]) x` underflowed STORE_LOCAL at run time — PR
		// #280 review, whether latched (L-DO) or dyn-body-marked), and a
		// splice-dyn spread's count is payload-sized. lw.variadic covers
		// only LOOP regions, so the record-side mark must gate here;
		// refusing at the promotion is the earliest true diagnosis, so the
		// frontier pins that used to surface later-stage reasons re-pinned
		// to this one. A SINGLE-out variadic (`def ok (do b error [drop
		// false])` — the dyn-env stored-handler shape) keeps its promotion:
		// its one store matches the one value BOTH the success and the
		// caught path deliver.
		if lw.es != nil && c.nout >= 2 && lw.es.eventInfo[ev.seq].variadicResult {
			return c.word + ": variadic result promoted to frame slots (runtime count differs from the static seat)"
		}
		for i := c.nout - 1; i >= 0; i-- {
			lw.seatStoreName(ev.seq, i)
			lw.emit(OpStoreLocal, slot+i, c.pos)
		}
		lw.note()
		return ""
	}
	if lw.dead[ev.seq] {
		if lw.bindConsumes[ev.seq] {
			// The root OpBindGlobal write-back consumes this dead result (Pop
			// mode) — leave it live for the immediately-following evDynBind.
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: 0})
			lw.note()
			return ""
		}
		// A single-result value-def referenced zero times: the call ran for its
		// side effects, but the result is discarded — drop it so it is not left
		// unconsumed on the stack (planValueDefLocals marks only nout==1 events).
		lw.emit(OpDrop, 0, c.pos)
		lw.note()
		return ""
	}
	// Push one simulated slot per result the call leaves (P5): 0 for a
	// side-effect word, N for a multi-result word — idx 0..N-1 deepest-first,
	// matching the VM's append of the handler's results (results[N-1] on top).
	for i := 0; i < c.nout; i++ {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: i})
	}
	lw.note()
	return ""
}

// collectRegionTop lowers a list literal whose ONE operand is a
// runtime-variadic REGION, and reports whether it did (NUR067's consuming
// half). planRegionCollect armed the plan and opened an OpStackMark before the
// region's producing event, which lowerEvents emitted immediately before the
// region ran; the run is therefore exactly stack[mark:], and
// OpMakeListToMark closes it into one List without naming the run's length.
//
// Returns false — leaving the emitted code untouched — whenever the plan is
// not armed for this event (every ordinary call), the region is not the sim's
// top, or the list result is a DEAD binding the ordinary lowering drops. The
// caller then takes the ordinary lowering, whose layoutOperands refusal
// ("consumes loop results") is the honest one.
//
// A PROMOTED result is handled here rather than declined: the collect leaves
// exactly ONE List, a static single value, so the ordinary store-once /
// re-push-per-reference promotion applies to it unchanged (`def xs [(for 3
// [i])]  xs`). It is the REGION that has no static count, and the region is
// gone by the time the store runs.
func (lw *lowerer) collectRegionTop(ev *EmitEvent) bool {
	c := &ev.call
	if lw.collectAtSeq != ev.seq || len(c.ops) != 1 || len(lw.vm) == 0 ||
		lw.vm[len(lw.vm)-1].seq != c.ops[0].idx || lw.dead[ev.seq] {
		return false
	}
	lw.emit(OpMakeListToMark, 0, c.pos)
	lw.vm[len(lw.vm)-1] = vmSlot{seq: ev.seq, idx: 0}
	if slot, prom := lw.promoted[ev.seq]; prom {
		lw.seatStoreName(ev.seq, 0)
		lw.emit(OpStoreLocal, slot, c.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	lw.note()
	return true
}

// lowerFragment lowers a closed body: a fresh stack scope that must
// end as exactly [out] (out non-nil), or empty (out nil — a net-0 or
// diverging fragment; a diverging fragment's terminator already
// emitted its jump, so whatever its scope holds is unreachable and
// ignored). Restores the parent scope afterwards.
func (lw *lowerer) lowerFragment(frag *EmitFragment, out *EmitOperand, allowVariadic bool, pos core.SrcPos) string {
	lw.depth++
	defer func() { lw.depth-- }()
	if lw.depth > maxLowerDepth {
		return "fragment nesting beyond the compile depth limit"
	}
	lw.fragMulti = false
	parent := lw.vm
	lw.vm = nil
	// An apply-FIRST per-iteration dynamic apply: the fn read precedes the
	// body's other statements in source order, so the apply emits BEFORE them —
	// a continue raised inside the applied body then skips the rest of the
	// iteration exactly as the interpreter's shared tape does. The apply nets
	// one value; the trailing statements must net zero (stores), leaving the
	// applied result as the iteration's sole value.
	if frag.applyFn != nil && frag.applyFirst {
		lw.pushOperand(*frag.applyFn, pos)
		for _, a := range frag.applyArgs {
			lw.pushOperand(a, pos)
		}
		lw.emit(OpCallDynamic, len(frag.applyArgs), pos)
		lw.vm = lw.vm[:len(lw.vm)-len(frag.applyArgs)]
		if reason := lw.lowerEvents(frag.events, frag.startSeq); reason != "" { //covergate:allow apply-first loop lowering error arm: the body events re-lower after already passing the recording pass, so a failure needs a bytecode-level fault; the same reason surfaces on the non-apply path (§compiler)
			return reason
		}
		if !fragDiverges(frag) && len(lw.vm) != 1 { //covergate:allow apply-first loop lowering backstop: setLoopBodyApply's re-pushable-args screen excludes any residual whose lowered sim diverges from one applied value; the arm guards a future recording drift (§compiler)
			return "loop body apply: statements after the apply leave values"
		}
		lw.vm = parent
		return ""
	}
	if reason := lw.lowerEvents(frag.events, frag.startSeq); reason != "" {
		return reason
	}
	if len(frag.applyArgs) > 0 {
		// Per-iteration dynamic apply (`for n [(mk2 i) 10]`): the body events left
		// a single leading fn VALUE on the sim; push the trailing static args and
		// apply via OpCallDynamic, netting one applied value (RecordLoop's
		// setLoopBodyApply seated this — see EmitFragment.applyArgs). A residual
		// that is not the sole leading fn refuses (a more complex shape than the
		// leading-fn-carrier case this lowers).
		if fragDiverges(frag) {
			lw.vm = parent
			return ""
		}
		// A re-pushable fn (applyFn — a frame-local `args.N` read): the body
		// events left nothing on the sim; push the fn first, so the layout below
		// is identical to the event-produced case.
		if frag.applyFn != nil {
			if len(lw.vm) != 0 { //covergate:allow loop apply fn re-push backstop: setLoopBodyApply seats applyFn only for a body whose events leave nothing on the sim; a non-empty residual here needs a recording drift (§compiler)
				return "loop body apply: fn re-push over a non-empty residual"
			}
			lw.pushOperand(*frag.applyFn, pos)
		}
		if len(lw.vm) != 1 {
			return "loop body apply: leading fn value not the sole residual"
		}
		for _, a := range frag.applyArgs {
			lw.pushOperand(a, pos)
		}
		lw.emit(OpCallDynamic, len(frag.applyArgs), pos)
		// OpCallDynamic pops the fn + its args and pushes one result; drop the arg
		// slots, leaving the (former fn) slot to stand for the applied result.
		lw.vm = lw.vm[:len(lw.vm)-len(frag.applyArgs)]
		lw.vm = parent
		return ""
	}
	// A fragment result that is an ENCLOSING-scope value-def PROMOTED to a frame local
	// (planValueDefLocals now promotes a fragResult that is not fragInternal) lives in
	// a SLOT, not on this fragment's sim stack — lowerEvents left nothing on lw.vm for
	// it. The fragment's `out` operand was captured at RECORDING time, before
	// promotion, so it still reads as the producing event; re-resolve it to its local
	// so the arm RE-PUSHES the slot as its result (`def g (…); if c [1] [g]`, and the
	// dynApply `def c (a b comp); if (c gt 0) [c] e`). Mirrors the main stream's
	// already-rewritten references.
	if out != nil && out.kind == opEvent {
		if slot, ok := lw.promoted[out.idx]; ok {
			loc := localOperand(slot + out.resIdx)
			out = &loc
		}
	}
	switch {
	case fragDiverges(frag):
		// Control left via break/continue; the residual scope is
		// unreachable.
	case out == nil:
		if len(lw.vm) != 0 {
			return "body leaves extra values (Stage 2 lowers single-result bodies)"
		}
	case frag.residualN > 1:
		// MULTI-VALUE arm: the interpreter leaves the WHOLE arm residual (verified:
		// `if true [1 2 3] [4]` → 1 2 3; `if (n lte 0) [] [n mul 2 m (n sub 1)]`
		// leaves n*2 then the recursive result). Every value must be EVENT-produced
		// on the sim — the single-operand arm model recorded only the top operand,
		// so inert consts/locals below it (`[1 2 3]`) were not captured and cannot
		// be reconstructed; require the full residualN event slots with the top
		// matching out, else refuse (a too-short sim is an inert-tail arm; a
		// too-long one is the lowering-artifact a single-value expression leaves —
		// `[({a:(get…)} get a)]`). Only a branch arm may carry it (allowVariadic);
		// a loop body / condition needs a single value. lowerArms marks the merge
		// variadic via fragMulti so only a variadic-absorbing position consumes it.
		if !allowVariadic {
			return "branch leaves extra values (Stage 2 lowers single-result branches)"
		}
		// ALL-INERT loop residual (`for 3 [1 2]`): nothing event-produced on
		// the sim; re-push the captured operands in order per iteration. The
		// parked-fn screen at RecordLoop already excluded Function entries.
		if len(frag.residualOps) == frag.residualN && len(lw.vm) == 0 {
			for _, op := range frag.residualOps {
				lw.pushOperand(op, pos)
			}
			lw.vm = lw.vm[:0]
			lw.fragMulti = true
			lw.vm = parent
			return ""
		}
		switch {
		case len(lw.vm) == frag.residualN && slotIs(lw.vm[len(lw.vm)-1], *out):
			// Every value EVENT-produced on the sim, the top matching out.
			lw.fragMulti = true
		case out.kind != opEvent && len(lw.vm) == frag.residualN-1:
			// A trailing CONST/LOCAL result above residualN-1 EVENTS on the sim (`[ …
			// swap-at end 0 ]` where the swap's array result is counted in residualN):
			// push the const on top of the seated events. The whole-arm residual
			// ([event…, const]) matches the interpreter, and the variadic merge
			// (fragMulti) absorbs it — the heap/intro multi-value swap-then-const arm.
			lw.pushOperand(*out, pos)
			lw.fragMulti = true
		default:
			return "branch leaves extra values (Stage 2 lowers single-result branches)"
		}
	case out.kind == opEvent:
		if lw.variadic[out.idx] && !allowVariadic {
			// The fragment's result is itself a VARIADIC (0-or-1) event — a
			// nested variadic `if` (e.g. a no-default `case` chain). A BRANCH ARM
			// may carry it (allowVariadic — the parent if propagates the
			// variadic-ness up to its own merge), but a loop body / condition may
			// not: they need a definite single value per iteration.
			return "loop results as a branch/body result (Stage 2)"
		}
		if len(lw.vm) != 1 || !slotIs(lw.vm[0], *out) {
			return "branch leaves extra values (Stage 2 lowers single-result branches)"
		}
	default:
		if len(lw.vm) != 0 {
			// Leftover side-effect EVENT results below the single const/local result: the
			// fragment analysis netted this arm at residualN<=1, so the events on the sim
			// are ignored side-effect calls (`[ arr i j swap-at end 0 ]` — the in-place
			// swap returns the array, discarded by the trailing literal `0`). They already
			// RAN (lowerEvents emitted them); DROP their results so the arm nets exactly
			// its single result. Only an allowVariadic branch ARM trims this way — a loop
			// body / condition with leftovers is a genuine over-count and still refuses.
			if !allowVariadic {
				return "branch leaves extra values (Stage 2 lowers single-result branches)"
			}
			for range lw.vm {
				lw.emit(OpDrop, 0, pos)
			}
			lw.vm = nil
		}
		lw.pushOperand(*out, pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // pushOperand tracked it; the scope owns the count
	}
	lw.vm = parent
	return ""
}

// lowerBreak / lowerContinue: flow-control terminators inside a loop
// body fragment. Both lower to the FLOW signal ops — the same pair a
// break/continue in a CALLEE uses — whether or not the loop lives in
// this unit.
//
// They used to lower to a bare OpJmp when the loop was in the same unit:
// break to the loop end (a hole patched by lowerLoop), continue back to
// FOR_NEXT. Both targets were right and both jumps were wrong, because
// the two things flowSignal ALSO does are the two things the interpreter
// does and a jump cannot (NUR132, measured 2026-09-10):
//
//   - it TRIMS THE ROUND (stack[:lp.iterBase]) — the interpreter's
//     break/continue splices the round's tape back to its mark, so a value
//     this round already produced is discarded. A jump left it on the
//     stack: `for 3 [ (7 add 2) if (i eq 2) [continue] [5] end ]`
//     answered `9 5 9 5 9` where the interpreter answers `9 5 9 5`;
//   - it POPS THE LOOP (loops[:target]) on a break. A jump to the loop's
//     end lands PAST its FOR_NEXT, the only op that pops, so the entry
//     leaked: after `for 2 [ … end for 3 [ if (i eq 1) [break] [0] end ] ]`
//     the OUTER loop's FOR_NEXT read the INNER loop's stale counter and
//     never terminated — the compiled program exhausted the stack ceiling
//     where the interpreter answers `0 0 1 0`.
//
// The signal resolves the nearest OPEN loop at run time, which for a
// same-unit break/continue is this very loop: exitPC is the FOR_NEXT's own
// exit target (the hole's old value) and nextPC is the FOR_NEXT (the
// back-edge's old value), so the destinations are unchanged. Only the
// discipline the jump skipped is added. The while lowering's
// condition-false exit has emitted OpFlowBreak for exactly this reason
// since the thirty-seventh increment; this makes the body's own
// terminators agree with it.
func (lw *lowerer) lowerBreak(ev *EmitEvent) string {
	if len(lw.loops) == 0 && !lw.isFnUnit {
		return "break outside a compiled loop (Stage 2)"
	}
	lw.emit(OpFlowBreak, 0, ev.call.pos)
	return ""
}

func (lw *lowerer) lowerContinue(ev *EmitEvent) string {
	if len(lw.loops) == 0 && !lw.isFnUnit {
		return "continue outside a compiled loop (Stage 2)"
	}
	lw.emit(OpFlowContinue, 0, ev.call.pos)
	return ""
}

// lowerTrap emits the terminal OpTrap for a check-mode-suppressed runtime error,
// pooling its TrapSpec. Execution never continues past it.
func (lw *lowerer) lowerTrap(ev *EmitEvent) string {
	if ev.trap.rematchWord != "" {
		// A VARIADIC leading operand — a multi-count branch merge (`if c [99]
		// [1 2] each …`) — cannot be seated by layout: the region is already
		// live on the runtime stack with an ARM-DEPENDENT depth. The rematch
		// is TERMINAL and only READS the top NArgs values, so seat the single
		// remaining const operand UNDER the region top instead: push it and
		// swap. The window then reads [region-top, const] — exactly the
		// [merge-carrier, const] the static match examined — for either arm
		// depth (deeper region values sit below the window and the raise
		// consumes nothing). The offset-form render bound keeps the raise
		// byte-identical (the written tuple is the const, arm-independent).
		if ops := ev.trap.rematchOps; len(ops) == 2 &&
			ops[0].kind == opEvent && lw.variadic[ops[0].idx] &&
			ops[1].kind != opEvent {
			if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], ops[0]) {
				return "stack discipline: variadic rematch region is not on top"
			}
			lw.pushOperand(ops[1], ev.trap.pos)
			lw.emit(OpSwap, 0, ev.trap.pos)
			idx := len(lw.p.Dispatches)
			lw.p.Dispatches = append(lw.p.Dispatches, DispatchSpec{
				Word:       ev.trap.rematchWord,
				NArgs:      len(ops),
				NWritten:   ev.trap.rematchNWritten,
				WrittenOff: ev.trap.rematchWrittenOff,
				Pos:        ev.trap.pos,
			})
			lw.emit(OpDispatchRematch, idx, ev.trap.pos)
			return ""
		}
		// A runtime-rematch trap: seat the failed window's operands like a
		// call's args — ops[0] (examined first, sig position 0) ends on TOP
		// (layoutOperands' contract, the callPoly window layout), event
		// results consumed from where they lie, consts pushed. A layout the
		// scheduler cannot seat refuses the trap; the caller's whole-program
		// fallback stands (slow, not wrong).
		if reason := lw.layoutOperands(ev.trap.rematchOps, ev.trap.pos, layoutMsgs{
			loopResults:  "rematch operands include a variadic loop result",
			resultNotTop: "stack discipline: rematch operand is not on top (rematch of " + ev.trap.rematchWord + ")",
			reorder:      "rematch operand shape needs reordering beyond Stage 3",
			shapeBeyond:  "rematch operand shape beyond Stage 3",
			notAdjacent:  "stack discipline: rematch operands not adjacent on top",
		}); reason != "" {
			return reason
		}
		idx := len(lw.p.Dispatches)
		lw.p.Dispatches = append(lw.p.Dispatches, DispatchSpec{
			Word:       ev.trap.rematchWord,
			NArgs:      len(ev.trap.rematchOps),
			NWritten:   ev.trap.rematchNWritten,
			WrittenOff: ev.trap.rematchWrittenOff,
			Pos:        ev.trap.pos,
		})
		lw.emit(OpDispatchRematch, idx, ev.trap.pos)
		return ""
	}
	idx := len(lw.p.Traps)
	lw.p.Traps = append(lw.p.Traps, ev.trap.spec)
	lw.emit(OpTrap, idx, ev.trap.pos)
	return ""
}

// lowerUserCall pushes the args (sig position 0 on top — frame
// locals bind by pop order) and calls or tail-calls the unit. A tail
// call replaces the frame: control never returns here, so nothing is
// pushed to the simulated stack (the marking pass already cleared
// the consumer's out expectation).
func (lw *lowerer) lowerUserCall(ev *EmitEvent) string {
	uc := &ev.uc
	n := len(uc.ops)
	// Stage 3 operand shape: all args const/local (results-on-stack
	// shapes work when the single result operand is on top, mirroring
	// lowerCall's n<=2 rules — keep it simple: allow one trailing
	// result operand at position 0). Shared with lowerCall via
	// layoutOperands; only the diagnostic wording differs.
	if reason := lw.layoutOperands(uc.ops, uc.pos, layoutMsgs{
		loopResults:  "loop results as fn args (Stage 3)",
		resultNotTop: "stack discipline: fn arg result is not on top (call of " + lw.es.fnRecs[uc.unit].name + ")",
		reorder:      "fn arg shape needs reordering beyond Stage 3",
		shapeBeyond:  "fn arg shape beyond Stage 3",
		notAdjacent:  "stack discipline: fn args not adjacent on top",
	}); reason != "" {
		return reason
	}
	if uc.tail {
		lw.emit(OpTailCallUser, uc.unit, uc.pos)
		lw.vm = lw.vm[:len(lw.vm)-n]
		return ""
	}
	lw.emit(OpCallUser, uc.unit, uc.pos)
	lw.vm = lw.vm[:len(lw.vm)-n]
	// A value-def local (single result referenced more than once, or an
	// out-of-order residual forced to a slot): store now, re-push per reference
	// (references were rewritten to local operands). Mirrors lowerCall.
	if slot, ok := lw.promoted[ev.seq]; ok {
		// …but NOT for a VARIADIC-returning callee. The one store pops one
		// value where the call left a runtime-variable count, and the rest of
		// the run stays on the stack in the wrong place: `def f fn
		// [[n:Integer] [] [for n [i]]] 9 f (1 add 2)` answered `0 1 9 2` for
		// the interpreter's `9 0 1 2` — the run's last value stored, its
		// first two stranded beneath the 9. lowerCall carries the same guard
		// for a multi-out variadic ("variadic result promoted to frame
		// slots"); this is its user-call twin, and it is needed at nout 1
		// because the variadic slot IS one slot. Measured pre-existing on
		// 6bc55db, surfaced by a Codex finding on PR #448 whose own diagnosis
		// (the mark plan) was a second, separate defect (NUR133).
		if lw.es != nil && uc.unit >= 0 && uc.unit < len(lw.es.fnRecs) &&
			lw.es.fnRecs[uc.unit].variadic {
			return lw.es.fnRecs[uc.unit].name + ": variadic fn result promoted to a frame slot (runtime count differs from the one store)"
		}
		lw.seatStoreName(ev.seq, 0)
		lw.emit(OpStoreLocal, slot, uc.pos)
		lw.note()
		return ""
	}
	if lw.dead[ev.seq] {
		if lw.bindConsumes[ev.seq] {
			// Consumed by the root OpBindGlobal write-back (Pop mode).
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: 0})
			lw.note()
			return ""
		}
		// Result referenced zero times: the call ran for effects, drop the result.
		lw.emit(OpDrop, 0, uc.pos)
		lw.note()
		return ""
	}
	if lw.es.fnRecs[uc.unit].variadic {
		// A VARIADIC-RETURNING callee leaves a runtime-variable count: push ONE
		// variadic sim slot (like a loop result) instead of uc.nout fixed slots.
		// Only a variadic-absorbing position (the program residual, a no-contract
		// RET, a parent branch merge) may consume it — layoutOperands refuses it as
		// a fixed-arity operand, the soundness gate that keeps `m 3 add 1` a refusal.
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.variadic[ev.seq] = true
		lw.note()
		return ""
	}
	// Push one simulated slot per result the unit returns (P5 multi-result):
	// 0 for a 0-return fn, N for a multi-return fn — idx 0..N-1 deepest-first,
	// matching the order the VM's CALL_USER leaves the unit's residual.
	for i := 0; i < uc.nout; i++ {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: i})
	}
	lw.note()
	return ""
}

// lowerUserPolyCall lowers a runtime-dispatched MULTI-OVERLOAD user call
// (emitUserCall with poly set): the args are pushed exactly as lowerUserCall
// lays them out (sig position 0 on top), and OpCallUserPoly re-runs
// MatchSignature over the recorded arm subset at run time to pick the body
// unit. Never a tail call (markTailCalls skips poly calls); never variadic
// (the recorder gates every arm to a fixed, identical return count).
func (lw *lowerer) lowerUserPolyCall(ev *EmitEvent) string {
	uc := &ev.uc
	n := len(uc.ops)
	if reason := lw.layoutOperands(uc.ops, uc.pos, layoutMsgs{
		loopResults:  "loop results as fn args (Stage 3)",
		resultNotTop: "stack discipline: fn arg result is not on top (poly call of " + uc.poly.word + ")",
		reorder:      "fn arg shape needs reordering beyond Stage 3",
		shapeBeyond:  "fn arg shape beyond Stage 3",
		notAdjacent:  "stack discipline: fn args not adjacent on top",
	}); reason != "" {
		return reason
	}
	pi := len(lw.p.UserPolys)
	lw.p.UserPolys = append(lw.p.UserPolys, UserPolyRef{
		Word:   uc.poly.word,
		Arity:  n,
		Reg:    uc.poly.reg,
		SigIdx: uc.poly.sigIdx,
		Units:  uc.poly.units,
		Impls:  uc.poly.impls,
		Sigs:   uc.poly.sigs,
	})
	lw.emit(OpCallUserPoly, pi, uc.pos)
	lw.vm = lw.vm[:len(lw.vm)-n]
	// Promotion / dead-result / result-slot accounting mirrors lowerUserCall.
	if slot, ok := lw.promoted[ev.seq]; ok {
		lw.seatStoreName(ev.seq, 0)
		lw.emit(OpStoreLocal, slot, uc.pos)
		lw.note()
		return ""
	}
	if lw.dead[ev.seq] {
		if lw.bindConsumes[ev.seq] {
			// Consumed by the root OpBindGlobal write-back (Pop mode).
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: 0})
			lw.note()
			return ""
		}
		lw.emit(OpDrop, 0, uc.pos)
		lw.note()
		return ""
	}
	for i := 0; i < uc.nout; i++ {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq, idx: i})
	}
	lw.note()
	return ""
}

// lowerFallback emits OpFallback. A fully-baked island (no threaded
// inputs) just runs its span; a single threaded input is the computed
// data arg (a "computed receiver" like `(iota 5) each […]`): its
// runtime value must sit on top of the operand stack when OpFallback
// runs, so the VM can preload it onto the island and back-fill the
// deepest sig position. A result operand is already on top; a
// const/local operand is pushed first. The island's single residual
// lands on the simulated stack as this event's product, read by a
// downstream consumer (or the program residual) like any computed
// value. Multiple threaded inputs are a documented follow-on.
func (lw *lowerer) lowerFallback(ev *EmitEvent) string {
	fb := &ev.fb
	// A REGION island (`error` over a maybe-raising body): the one sim slot
	// this pushes below already IS the region's representation — runFallback
	// appends whatever the re-run produced, 0 values or 1 — so the mark is
	// all that is needed, and it is what keeps a consumer from seating the
	// run at a fixed count.
	if lw.es != nil && lw.es.eventInfo[ev.seq].variadicRegion {
		lw.variadic[ev.seq] = true
	}
	switch len(fb.ins) {
	case 0:
		lw.emit(OpFallback, fb.spanIdx, fb.pos)
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.note()
		return ""
	case 1:
		op := fb.ins[0]
		if op.kind == opEvent {
			if lw.variadic[op.idx] {
				return "fallback threads a loop result (Stage 5 follow-on)"
			}
			// The computed value is already on top of the simulated
			// stack; OpFallback consumes it as the threaded input.
			if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], op) {
				return "stack discipline: fallback input is not on top"
			}
		} else {
			// A const / local / type input: materialise it on top first.
			lw.pushOperand(op, fb.pos)
		}
		lw.emit(OpFallback, fb.spanIdx, fb.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.note()
		return ""
	default:
		return "fallback island with multiple threaded inputs (Stage 5 follow-on)"
	}
}

// markTailCalls rewrites tail-position user calls in a fn body
// fragment: the final event when it produces the body's result, and
// recursively the final event of branch arms whose result is that
// arm's own trailing call. Marked calls lower as TAIL_CALL_USER and
// count as divergence (control leaves via the callee's eventual RET),
// so the arm/body contributes no merge value. A statically-taken
// (const-condition) branch inlines only its taken arm, so a tail call
// there is the body's tail call too and is marked through the same
// recursion — otherwise the dynamic-condition path would honour the
// O(1)-frame guarantee while the const-condition path silently lost it.
// fragSingleResidual reports whether a STRAIGHT-LINE fragment (plain calls /
// user calls only) leaves exactly one runtime value. It tracks net stack depth:
// each event consumes its event-sourced operands (the inert const/local/type
// operands are pushed-then-consumed, net 0) and pushes its nout results. A
// fragment containing a branch / loop / fallback / trap is not straight-line —
// return false (conservatively NOT a tail position; bailing only forgoes the
// O(1)-frame optimisation, never correctness). Used by markTailCalls to refuse
// tail-marking a call that has values left BELOW it (a multi-value arm).
func fragSingleResidual(frag *EmitFragment) bool {
	depth := 0
	for i := range frag.events {
		ev := &frag.events[i]
		var ops []EmitOperand
		var nout int
		switch ev.kind {
		case evCall:
			ops, nout = ev.call.ops, ev.call.nout
		case evCallUser:
			ops, nout = ev.uc.ops, ev.uc.nout
		default:
			return false
		}
		for _, op := range ops {
			if op.kind == opEvent {
				depth--
			}
		}
		depth += nout
	}
	return depth == 1
}

// tailCompatibleReturns reports whether a tail call to calleeUnit is sound w.r.t.
// the CALLER's declared return types. A TAIL_CALL_USER REPLACES the caller's
// frame, so the callee's RET returns straight to the caller's caller, BYPASSING
// the caller's return-type check (checkReturnContract at RET). That is sound only
// if the callee's result already satisfies the caller's contract — i.e. each
// callee return type conforms to the caller's. A caller with no contract (empty)
// is trivially safe; a count mismatch falls back to a regular CALL_USER so the
// caller's RET check runs (which the interpreter runs too). Without this a
// `[Map]`-declared fn tail-calling a `[Integer]`-returning one returned the
// Integer UNCAUGHT — a compile==interpret violation (the interpreter raises
// type_error). A caller declaring `[Any]` (or self/compatible recursion) stays a
// tail call: the check it bypasses is vacuous / already implied by the callee.
func (es *EmitState) tailCompatibleReturns(calleeUnit int, callerReturns []*core.Type) bool {
	if len(callerReturns) == 0 {
		return true
	}
	if calleeUnit < 0 || calleeUnit >= len(es.fnRecs) {
		return false
	}
	callee := es.fnRecs[calleeUnit].returns
	if len(callee) != len(callerReturns) {
		return false
	}
	for i, exp := range callerReturns {
		if exp == nil || exp.Equal(core.TAny) {
			continue
		}
		if callee[i] == nil || !callee[i].ConformsTo(exp) {
			return false
		}
	}
	return true
}

func (es *EmitState) markTailCalls(frag *EmitFragment, out *EmitOperand, hasOut bool, callerReturns []*core.Type) (stillHasOut bool) {
	if frag == nil || len(frag.events) == 0 || !hasOut || out.kind != opEvent {
		return hasOut
	}
	last := &frag.events[len(frag.events)-1]
	switch last.kind {
	case evCallUser:
		// A POLY user call (uc.poly != nil, unit -1) is never tail-marked: the
		// arm is only known at run time, and OpCallUserPoly always pushes a
		// frame (the caller's RET check must still run over the arm's result).
		if last.uc.poly == nil && last.seq == out.idx && fragSingleResidual(frag) && es.tailCompatibleReturns(last.uc.unit, callerReturns) {
			// Tail position requires the call's result to be the fragment's WHOLE
			// residual — nothing left BELOW it. A multi-value arm (`[n mul 2 m (n
			// sub 1)]`, where n*2 sits below the recursive call) is NOT tail: a
			// frame-replacing TAIL_CALL_USER would discard the lower values. Only
			// mark tail when the net residual is exactly the call's single result
			// AND the callee's returns satisfy the caller's (else the tail call
			// would bypass the caller's return-type check — see tailCompatibleReturns).
			last.uc.tail = true
			return false
		}
	case evBranch:
		if last.seq != out.idx {
			return hasOut
		}
		if last.br.constCond != nil {
			// Statically-taken branch: lowerBranch inlines ONLY the taken
			// (then) arm and its result IS the branch's result, so a tail
			// call there is the body's tail call. Mark it; if the arm fully
			// tail-diverges, the whole branch does (lowerBranch's const-cond
			// path then emits no merge slot, matching hasThenOut=false).
			if last.br.hasThenOut {
				last.br.hasThenOut = es.markTailCalls(last.br.then, &last.br.thenOut, true, callerReturns)
				if !last.br.hasThenOut {
					return false
				}
			}
			return hasOut
		}
		if last.br.hasElse {
			if last.br.hasThenOut {
				last.br.hasThenOut = es.markTailCalls(last.br.then, &last.br.thenOut, true, callerReturns)
			}
			if last.br.hasElsOut {
				last.br.hasElsOut = es.markTailCalls(last.br.els, &last.br.elsOut, true, callerReturns)
			}
			// The branch still merges normally for non-tail arms; if
			// EVERY arm tail-calls, control never reaches the merge —
			// the whole body diverges.
			if !last.br.hasThenOut && !last.br.hasElsOut {
				return false
			}
		}
	}
	return hasOut
}

func (lw *lowerer) lowerBranch(ev *EmitEvent) string {
	br := ev.br
	if br.constCond != nil {
		// Statically-taken branch: inline the taken fragment (always a body in
		// const-cond form — never a value-then).
		if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, true, br.pos); reason != "" {
			return reason
		}
		thenMulti := lw.fragMulti
		if br.hasThenOut {
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
			// A MULTI-VALUE taken arm (`if true [1 2 3] [4]` → 1 2 3) leaves N
			// runtime values the single merge slot can't track, OR the arm was
			// itself variadic — either way only the program residual / a no-contract
			// RET may absorb the run, so mark the merge variadic.
			if thenMulti || lw.variadic[br.thenOut.idx] {
				lw.variadic[ev.seq] = true
			}
			lw.note()
		}
		return ""
	}
	if lw.variadicElse[ev.seq] {
		// Chained variadic-if: the else operand is a PRIOR 2-arg `if`'s 0-or-1
		// result, sitting above an OpStackMark (opened by planVariadicClaims /
		// lowerEvents); the cond (an event) is on top. The TRUE path discards the
		// 0-or-1 eager via DROP_TO_MARK and runs the then arm; the FALSE path keeps
		// the eager as the else result via POP_MARK. The merge is itself 0-or-1.
		if br.cond.kind != opEvent || len(lw.vm) < 2 ||
			!slotIs(lw.vm[len(lw.vm)-1], br.cond) || !slotIs(lw.vm[len(lw.vm)-2], br.elsVal) {
			return "if: variadic-else claim stack layout (Stage 2)"
		}
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed
		// TRUE: discard the 0-or-1 eager (truncate to the mark) and run the then arm.
		lw.emit(OpDropToMark, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // eager folded into the merge
		if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, false, br.pos); reason != "" {
			return reason
		}
		jend := lw.emit(OpJmp, 0, br.pos)
		// FALSE: keep the eager as the else result; discard the mark.
		(*lw.code)[jf].Arg = int32(len(*lw.code))
		lw.emit(OpPopMark, 0, br.pos)
		(*lw.code)[jend].Arg = int32(len(*lw.code))
		// Merge: then nets one value, else nets the 0-or-1 eager → a VARIADIC
		// (0-or-1) result the program residual absorbs.
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		lw.variadic[ev.seq] = true
		lw.note()
		return ""
	}
	if br.thenComputed && br.elsComputed {
		// `if (c) (a) (b)` — BOTH arms eagerly computed: the three events stack
		// as [cond, then, else]; select one with OpReverse + JMP_IF_FALSE.
		return lw.lowerBothComputed(ev)
	}
	if br.elsComputed || br.thenComputed {
		// One arm's value is an eagerly-computed event (`if c [t] (expr)` or
		// `if c (expr) e`). It is the last thing evaluated before the branch, so
		// it sits on TOP of the sim stack; the OTHER (non-eager) arm DROPs it and
		// produces its own value, so it must net a value here — OR NOT ARRIVE.
		//
		// Those are two different things, and this gate used to conflate them:
		// its message said "diverges" while its test was "has no out", which a
		// DIVERGING arm and a merely 0-NETTING one both fail. Only the second
		// is a problem. A 0-netting arm reaches the merge having produced
		// nothing, so the join is 1-or-0 where the slot models 1; a DIVERGING
		// arm (break / continue / raise / a tail call) leaves the construct and
		// never reaches the merge at all, so every path that ARRIVES carries the
		// eager value and the single merge slot is exact. That is the last
		// frontier-while row (the fifty-first increment): the `continue` arm of
		// `if ((c get 'n') eq 2) [continue]` under a computed prefix.
		eager, nonEagerHasOut := br.elsVal, br.hasThenOut
		nonEager := br.then
		if br.thenComputed {
			eager, nonEagerHasOut = br.thenVal, br.hasElsOut
			nonEager = br.els
		}
		if !nonEagerHasOut && !fragDivergesDeep(nonEager) {
			return "if: computed-branch non-eager arm nets no value (Stage 2)"
		}
		// Two stack layouts reach here, and they are mirror images.
		//
		// WRITTEN-ARM (`if c [t] (expr)`): the eager arm is the last thing
		// written, so it is evaluated after the condition and sits on TOP,
		// with the cond just below — lowerComputedCond SWAPs.
		//
		// STACK-SUPPLIED ARM (`(expr) … if (c) [t]`): the eager arm arrived
		// on the VALUE STACK before the `if` was reached, and the condition
		// is a forward token evaluated at the dispatch, so the two sit the
		// other way round — the cond is already on top and no swap is owed.
		// This is the second half of the last frontier-while row: the
		// argument-order rule fills the else position from the value stack,
		// which puts the arm UNDER its own condition.
		top := len(lw.vm) - 1
		eagerOnTop := top >= 0 && slotIs(lw.vm[top], eager)
		condOnTop := !eagerOnTop && br.cond.kind == opEvent && top >= 1 &&
			slotIs(lw.vm[top], br.cond) && slotIs(lw.vm[top-1], eager)
		if !eagerOnTop && !condOnTop {
			return "if: computed-branch eager value not on top (Stage 2)"
		}
		jf, reason := lw.lowerComputedCond(br, condOnTop)
		if reason != "" {
			return reason
		}
		return lw.lowerComputedBranch(ev, jf)
	}
	// Condition on top of stack: a pre-evaluated value, or an inline
	// list-form condition body lowered here (it nets one Boolean).
	switch {
	case br.condFrag != nil:
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, false, br.pos); reason != "" {
			return reason
		}
		// The Boolean is on the runtime stack but not in the parent
		// scope's sim — JMP_IF_FALSE consumes it net-zero.
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		return lw.lowerArms(ev, jf)
	case br.cond.kind == opEvent:
		if lw.variadic[br.cond.idx] {
			return "loop results as a condition (Stage 2)"
		}
		if len(lw.vm) == 0 || !slotIs(lw.vm[len(lw.vm)-1], br.cond) {
			return "if: condition is not on top of the stack"
		}
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed
		return lw.lowerArms(ev, jf)
	default:
		lw.pushOperand(br.cond, br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		return lw.lowerArms(ev, jf)
	}
}

// lowerArm emits one if-arm as a single (or zero) merge value: a plain value
// operand is pushed; a body fragment is lowered with its out (nil when the arm
// nets nothing or diverges — it still runs). allowVariadic passes through to
// lowerFragment (the merge of two body arms may be variadic; a computed-branch
// arm may not). Shared by lowerArms, lowerBranch's const-cond inline, and the
// non-eager arm of lowerComputedBranch.
func (lw *lowerer) lowerArm(kind armKind, val EmitOperand, frag *EmitFragment, out *EmitOperand, allowVariadic bool, pos core.SrcPos) string {
	lw.fragMulti = false
	switch kind {
	case armValue:
		// Push the literal/local/type operand as the arm's single result
		// (pushOperand tracked it; the merge slot owns the count).
		lw.pushOperand(val, pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		return ""
	case armBodyOut:
		return lw.lowerFragment(frag, out, allowVariadic, pos)
	default: // armBodyVoid — a 0-value / diverging body
		return lw.lowerFragment(frag, nil, allowVariadic, pos)
	}
}

// lowerArms emits the then/else arms after the JMP_IF_FALSE at jf.
// A diverging arm's terminator already jumped out of the construct,
// so it needs no jump-to-end and contributes no value; the merge
// point then carries only the surviving arm's value. The 2-arg form
// (no else) merges with 0-or-1 values — a VARIADIC result.
func (lw *lowerer) lowerArms(ev *EmitEvent, jf int) string {
	br := ev.br
	if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, true, br.pos); reason != "" {
		return reason
	}
	thenMulti := lw.fragMulti
	if !br.hasElse {
		// 2-arg if: false path jumps straight to the merge.
		(*lw.code)[jf].Arg = int32(len(*lw.code))
		// A value-producing then yields a VARIADIC result (the value on true,
		// nothing on false) — only the program residual may absorb it. A
		// 0-value / diverging then (raise/set/break) produces 0 values on both
		// paths: a statement guard with no merge slot.
		if br.hasThenOut {
			lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
			lw.variadic[ev.seq] = true
			lw.note()
		}
		return ""
	}
	jend := -1
	if br.thenIsVal || !fragDiverges(br.then) {
		jend = lw.emit(OpJmp, 0, br.pos)
	}
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	if reason := lw.lowerArm(br.elseArm(), br.elsVal, br.els, &br.elsOut, true, br.pos); reason != "" {
		return reason
	}
	elseMulti := lw.fragMulti
	if jend >= 0 {
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	if br.hasThenOut || br.hasElsOut {
		lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
		// A MULTI-VALUE arm (either side leaves >1 runtime value) makes the merge
		// runtime-variable-count even when both arms "net a value": the two arms
		// can leave different counts (`if c [1 2] [3]`) and the interpreter leaves
		// the whole residual. Force the merge variadic so only the program residual
		// or a no-contract fn RET absorbs it.
		if thenMulti || elseMulti {
			lw.variadic[ev.seq] = true
		}
		// Mismatched arm value-counts: one arm nets a value, the other nets 0
		// WITHOUT diverging (it reaches the merge with nothing) → the merge
		// carries 0-or-1 values, a VARIADIC result only the program residual may
		// absorb (`if cond [99] []`, `if cond [raise] [99]`). A DIVERGING 0-arm
		// never reaches the merge, so the surviving arm's value is
		// unconditional — non-variadic, left as-is.
		if br.hasThenOut != br.hasElsOut {
			// Use the DEEP divergence check: an arm whose result is a
			// fully-diverging nested branch (a const-condition branch whose
			// taken arm tail-calls) leaves via that callee's RET and never
			// reaches the merge, so the surviving arm's value is unconditional
			// — non-variadic. Shallow fragDiverges misses the nested-branch
			// shape and would over-mark the merge variadic.
			thenDiv := fragDivergesDeep(br.then)
			elsDiv := br.els != nil && fragDivergesDeep(br.els)
			if (!br.hasThenOut && !thenDiv) || (!br.hasElsOut && !elsDiv) {
				lw.variadic[ev.seq] = true
			}
		}
		// Variadic ARM: an arm whose own result is a nested variadic (0-or-1)
		// event — a no-default `case`'s inner `if` chain — makes this merge
		// variadic too, so the 0-or-1 propagates up to the residual.
		if (br.hasThenOut && lw.variadic[br.thenOut.idx]) ||
			(br.hasElsOut && lw.variadic[br.elsOut.idx]) {
			lw.variadic[ev.seq] = true
		}
		lw.note()
	}
	return ""
}

// lowerBothComputed lowers `if (c) (a) (b)` where BOTH arms are eagerly-computed
// events. Entry sim/runtime stack is [.., cond, thenVal, elsVal] (elsVal on top).
// Both arms ran already (paren args evaluate eagerly — faithful to the
// interpreter, which also evaluates both), so this only SELECTS one: it rotates
// the cond to the top (OpReverse 3 → [elsVal, thenVal, cond]), branches, and
// DROPs the unselected value on each path. The merge is a single (non-variadic)
// slot.
func (lw *lowerer) lowerBothComputed(ev *EmitEvent) string {
	br := ev.br
	if br.cond.kind != opEvent {
		return lw.lowerBothComputedMatCond(ev)
	}
	if len(lw.vm) < 3 ||
		!slotIs(lw.vm[len(lw.vm)-1], br.elsVal) ||
		!slotIs(lw.vm[len(lw.vm)-2], br.thenVal) ||
		!slotIs(lw.vm[len(lw.vm)-3], br.cond) {
		return "if: both-computed stack layout (Stage 2)"
	}
	if lw.variadic[br.thenVal.idx] || lw.variadic[br.elsVal.idx] {
		return "if: both-computed arm is a variadic loop value (Stage 2)"
	}
	// Reverse the top three so the cond lands on top: [elsVal, thenVal, cond].
	lw.emit(OpReverse, 3, br.pos)
	n := len(lw.vm)
	lw.vm[n-3], lw.vm[n-1] = lw.vm[n-1], lw.vm[n-3]
	jf := lw.emit(OpJmpIfFalse, 0, br.pos) // pop cond → [elsVal, thenVal]
	// TRUE/fall-through: the result is thenVal (now on top), so drop elsVal
	// beneath it: SWAP then DROP → [thenVal].
	lw.emit(OpSwap, 0, br.pos)
	lw.emit(OpDrop, 0, br.pos)
	jend := lw.emit(OpJmp, 0, br.pos)
	// FALSE: stack is [elsVal, thenVal] (thenVal on top); the result is elsVal,
	// so drop thenVal → [elsVal].
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	lw.emit(OpDrop, 0, br.pos)
	(*lw.code)[jend].Arg = int32(len(*lw.code))
	// Sim: the three input slots (cond/then/else) collapse to one merge slot.
	lw.vm = lw.vm[:len(lw.vm)-3]
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}

// lowerBothComputedMatCond lowers `if cond (a) (b)` when the cond has NO
// eager stack home (a condFrag list body, or a const / local / type value —
// computedArmCondOK's non-event shapes): the entry sim is [thenVal, elsVal]
// and the cond MATERIALISES above them exactly as the single-computed
// lowerComputedCond does, then JMP_IF_FALSE selects. TRUE/fall-through: the
// result is thenVal, so DROP the elsVal on top; FALSE: SWAP + DROP leaves
// elsVal. Both eagers evaluated in BOTH engines (paren eagerness), so the
// selection-only lowering is the identical semantics the event-cond arm
// already compiles.
func (lw *lowerer) lowerBothComputedMatCond(ev *EmitEvent) string {
	br := ev.br
	if len(lw.vm) < 2 ||
		!slotIs(lw.vm[len(lw.vm)-1], br.elsVal) ||
		!slotIs(lw.vm[len(lw.vm)-2], br.thenVal) {
		return "if: both-computed stack layout (Stage 2)"
	}
	if lw.variadic[br.thenVal.idx] || lw.variadic[br.elsVal.idx] {
		return "if: both-computed arm is a variadic loop value (Stage 2)"
	}
	var jf int
	if br.condFrag != nil {
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, false, br.pos); reason != "" { //covergate:allow the condFrag re-lowers after passing the recording pass's probe (RecordBranch), so a failure needs a bytecode-level fault; the single-computed twin (lowerComputedCond) carries the identical arm (§compiler)
			return reason
		}
		jf = lw.emit(OpJmpIfFalse, 0, br.pos)
	} else {
		lw.pushOperand(br.cond, br.pos)
		jf = lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
	}
	lw.emit(OpDrop, 0, br.pos)
	jend := lw.emit(OpJmp, 0, br.pos)
	(*lw.code)[jf].Arg = int32(len(*lw.code))
	lw.emit(OpSwap, 0, br.pos)
	lw.emit(OpDrop, 0, br.pos)
	(*lw.code)[jend].Arg = int32(len(*lw.code))
	lw.vm = lw.vm[:len(lw.vm)-2]
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}

// lowerComputedCond materialises a computed-arm branch's condition as a Boolean
// on TOP of the already-on-stack eager arm value and emits JMP_IF_FALSE
// (consuming it), returning the jf pc. The eager value is the top sim slot on
// entry and remains so on return. It handles the three condition shapes the
// recorder admits for a computed arm:
//
//   - a list-form condition body (`if [x gt 0] (expr) e`): lowered inline above
//     the eager value, netting one Boolean (not tracked in the parent sim);
//   - an event cond (`if (x eq 0) (expr) e`): the cond event sits just BELOW the
//     eager value — SWAP it to the top. Unless condOnTop: the eager arm came
//     off the VALUE STACK before the `if`, so the cond — a forward token
//     evaluated at the dispatch — is already above it and the swap is skipped
//     (lowerBranch decided which layout this is);
//   - a const / local / type cond (`if flag (expr) e`): pushed above the eager.
func (lw *lowerer) lowerComputedCond(br *emitBranch, condOnTop bool) (int, string) {
	switch {
	case br.condFrag != nil:
		if reason := lw.lowerFragment(br.condFrag, &br.condOut, false, br.pos); reason != "" {
			return 0, reason
		}
		return lw.emit(OpJmpIfFalse, 0, br.pos), ""
	case br.cond.kind == opEvent:
		if !condOnTop {
			if len(lw.vm) < 2 || !slotIs(lw.vm[len(lw.vm)-2], br.cond) {
				return 0, "if: computed-branch condition not below the eager value (Stage 2)"
			}
			lw.swapTop2(br.pos)
		}
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed; eager value stays on top
		return jf, ""
	default:
		lw.pushOperand(br.cond, br.pos)
		jf := lw.emit(OpJmpIfFalse, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1] // cond consumed; eager value stays on top
		return jf, ""
	}
}

// lowerComputedBranch lowers an `if` whose THEN or ELSE value is an eagerly-
// computed event, after the JMP_IF_FALSE at jf. Entry sim/runtime stack is
// [.., eager] (the cond was just consumed; the eager arm value remains). The
// eager value is the result of ITS path; the OTHER (non-eager) arm DROPs it and
// produces its own value (a plain value push or a body fragment). Both arms net
// exactly one value, so the merge is a single (non-variadic) slot.
//
//   - computed ELSE (`if c [t] (expr)`): the eager value is the FALSE-path
//     result; the TRUE (fall-through) path drops it and runs the then arm.
//   - computed THEN (`if c (expr) e`): the eager value is the TRUE-path result;
//     the FALSE (jump-target) path drops it and runs the else arm.
func (lw *lowerer) lowerComputedBranch(ev *EmitEvent, jf int) string {
	br := ev.br
	if br.thenComputed {
		// TRUE/fall-through keeps the eager then value; jump over the else arm.
		jend := lw.emit(OpJmp, 0, br.pos)
		(*lw.code)[jf].Arg = int32(len(*lw.code)) // FALSE lands here
		lw.emit(OpDrop, 0, br.pos)                // discard the eager then value
		lw.vm = lw.vm[:len(lw.vm)-1]
		if reason := lw.lowerArm(br.elseArm(), br.elsVal, br.els, &br.elsOut, false, br.pos); reason != "" {
			return reason
		}
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	} else {
		// TRUE/fall-through drops the eager else value and runs the then arm;
		// the FALSE path falls through with the eager value intact.
		lw.emit(OpDrop, 0, br.pos)
		lw.vm = lw.vm[:len(lw.vm)-1]
		if reason := lw.lowerArm(br.thenArm(), br.thenVal, br.then, &br.thenOut, false, br.pos); reason != "" {
			return reason
		}
		jend := lw.emit(OpJmp, 0, br.pos)
		(*lw.code)[jf].Arg = int32(len(*lw.code)) // FALSE lands here, eager value intact
		(*lw.code)[jend].Arg = int32(len(*lw.code))
	}
	lw.vm = append(lw.vm, vmSlot{seq: ev.seq})
	lw.note()
	return ""
}
