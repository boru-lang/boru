package compiler

import core "github.com/boru-lang/boru/core/go"

// The KEPT-DEFS LATCH (NUR210). A keep-defs word — `do` (CallableSpec
// BodyOnceKeepsDefs) and `each` and its kin (BodyMultiRunKeepsDefs) — keeps
// the defs and undefs its body makes in the ENCLOSING scope. For a literal
// body the check pass runs the tokens and models every one of those changes.
// For a COMPUTED body (a List param, a factory's quoted list) it cannot: the
// tokens exist only at run time, so the dyn-body backstop (tryRecordDynBody)
// lowers the dispatch to a CALL_NATIVE whose handler runs them, and the model
// keeps the bindings from BEFORE the run. The run may have changed any name
// at all, so a compiled observer of a binding after it would answer the
// stale value:
//
//	def x 99 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x
//	  interpreted  5          compiled  99 (an internal_error before the
//	                          dyn-body result was recorded as a region)
//
// Nothing compiled can read the run's changes back, so the latch declines:
// once armed, the FIRST observer recorded after it — a def read (NoteDefRead),
// a user fn call (its unit reads the model's bindings), a fn-value apply —
// poisons armReadCompileFailure, the arm-read seam's Finalize decline. A
// program that reads nothing after the run (`do (mk)` last, or followed only
// by literals and native calls over them) still compiles.
//
// The latch is armed where the body RUNS. At the program's top level that is
// the dispatch itself, for good. Inside a unit it is armed at the unit's
// depth and, when the unit finishes, handed to the unit (runsKeptDefs) and
// disarmed — the unit's analysis is not its run. It re-arms after whatever
// invokes the unit: a CALL_USER of it, the native a code-body closure is
// handed to (the closure is invoked right after its recording, so its finish
// re-arms at once), or — when any unit runs a kept-defs body — any event that
// may invoke a fn value indirectly (keptDefsInvoker). A unit's own defs are
// scoped to its frame on the interpreter, but an undef reaches past the frame
// to the program's bindings (`def x 99 end def f fn [[b:List][][do b]] end f
// (quote [undef x]) end x` raises undefined_word), so the hand-off is not
// optional.

// latchKeptDefs arms the latch at the current unit depth for word, unless a
// latch armed at the same or a shallower depth already covers it.
func (es *EmitState) latchKeptDefs(word string) {
	level := len(es.units)
	if es.keptDefsLevel != 0 && es.keptDefsLevel <= level {
		return
	}
	es.keptDefsLevel = level
	es.keptDefsWord = word
}

// noteKeptDefsObserver poisons the placement gate for an observer of a
// binding recorded while the latch is armed: the body run before it may have
// defined or undefined the very name it reads. It also notes the observer
// against every unit a recursive call reached while open (calledOpen), whose
// finish decides whether that call ran a kept-defs body. After a terminal
// top-level trap the observer is unreachable, and the first poison wins.
func (es *EmitState) noteKeptDefsObserver(what string) {
	if es.trapAt != 0 || es.armReadCompileFailure != "" {
		return
	}
	for _, u := range es.openUnitRecs {
		if rec := es.fnRecs[u]; rec.calledOpen && rec.openCallObserver == "" {
			rec.openCallObserver = what
		}
	}
	if es.keptDefsLevel == 0 {
		return
	}
	es.poisonKeptDefs(es.keptDefsWord, what)
}

// poisonKeptDefs latches the NUR210 decline through the arm-read seam.
func (es *EmitState) poisonKeptDefs(word, what string) {
	es.armReadCompileFailure = word + ": a computed body keeps its defs and undefs in the enclosing scope, and " +
		what + " after it would read the check model's binding, which never saw the body (NUR210)"
}

// keptDefsEvent is appendEvent's hook: an event that observes a binding is
// checked against the latch FIRST (a call of a kept-defs unit observes what
// ran before it), then an event that may run a kept-defs unit arms it.
func (es *EmitState) keptDefsEvent(ev *EmitEvent) {
	if what := keptDefsObserverEvent(ev); what != "" {
		es.noteKeptDefsObserver(what)
	}
	if ev.kind == evCallUser && ev.uc.unit >= 0 && ev.uc.unit < len(es.fnRecs) {
		rec := es.fnRecs[ev.uc.unit]
		if !rec.finished {
			rec.calledOpen = true
		}
	}
	if word := es.keptDefsInvoker(ev); word != "" {
		es.runKeptDefs(word)
	}
}

// keptDefsObserverEvent names an event that observes a binding the model may
// hold stale: a user fn call (its unit reads the model's bindings) or a
// fn-value apply (the value may be a compiled unit doing the same). "" for
// every other event — a native call, a store, a twin, an island (the
// interpreter resolving live) observes nothing stale.
func keptDefsObserverEvent(ev *EmitEvent) string {
	switch {
	case ev.kind == evCallUser:
		return "a user fn call"
	case ev.kind == evCall && (ev.call.dynApply > 0 || ev.call.dynMixed || ev.call.dynMethod != nil):
		return "the fn-value apply at `" + ev.call.word + "`"
	}
	return ""
}

// keptDefsInvoker reports the kept-defs word an event may RUN: a CALL_USER of
// a unit that runs a kept-defs body, directly. Once any unit does
// (keptDefsUnitWord), an event that may invoke a fn value indirectly counts
// too — a fn-value apply, an island, a native call handed a closure, a fn
// constant or any run-time value (which may be a fn value of such a unit).
func (es *EmitState) keptDefsInvoker(ev *EmitEvent) string {
	if ev.kind == evCallUser && ev.uc.unit >= 0 && ev.uc.unit < len(es.fnRecs) {
		if w := es.fnRecs[ev.uc.unit].runsKeptDefs; w != "" {
			return w
		}
	}
	if es.keptDefsUnitWord == "" {
		return ""
	}
	switch ev.kind {
	case evFallback:
		return es.keptDefsUnitWord
	case evCall:
		if ev.call.dynApply > 0 || ev.call.dynMixed || ev.call.dynMethod != nil {
			return es.keptDefsUnitWord
		}
		for _, op := range ev.call.ops {
			if es.operandMayInvoke(op) {
				return es.keptDefsUnitWord
			}
		}
	}
	return ""
}

// operandMayInvoke reports whether a native call's operand may be a fn value
// the native runs: a closure, a fn constant, or a value known only at run time.
func (es *EmitState) operandMayInvoke(op EmitOperand) bool {
	switch op.kind {
	case opType:
		return false
	case opConst:
		if op.idx < 0 || op.idx >= len(es.consts) {
			return false
		}
		v := es.consts[op.idx]
		_, isFn := v.Data.(core.FnDefInfo)
		return isFn || (v.Parent != nil && v.Parent.ConformsTo(core.TFunction))
	}
	return true
}

// runKeptDefs records that a kept-defs body runs HERE: every unit open at
// this point runs it (directly, or through the call being recorded), and the
// latch arms at the current depth.
func (es *EmitState) runKeptDefs(word string) {
	for _, u := range es.openUnitRecs {
		if es.fnRecs[u].runsKeptDefs == "" {
			es.fnRecs[u].runsKeptDefs = word
		}
	}
	if es.keptDefsUnitWord == "" && len(es.openUnitRecs) > 0 {
		es.keptDefsUnitWord = word
	}
	es.latchKeptDefs(word)
}

// finishKeptDefs runs at a unit's finish, before its depth is popped: a latch
// armed inside the unit (at its depth or deeper) belongs to the unit's RUN,
// not to the enclosing analysis, so it is disarmed — the unit carries the
// fact (runsKeptDefs) to wherever it runs. A code-body closure (rec.closure)
// is invoked by the native it is handed to, right after this, so it re-arms
// at the enclosing depth at once. A recursive call that reached the unit
// while it was open and was followed by an observer is decided here, now
// that the answer is known.
func (es *EmitState) finishKeptDefs(rec *fnUnitRec) {
	if rec.runsKeptDefs == "" {
		return
	}
	if es.keptDefsLevel >= len(es.units) {
		es.keptDefsLevel = 0
		es.keptDefsWord = ""
	}
	if rec.calledOpen && rec.openCallObserver != "" && es.trapAt == 0 && es.armReadCompileFailure == "" {
		es.poisonKeptDefs(rec.runsKeptDefs, rec.openCallObserver)
	}
	if rec.closure {
		es.keptDefsHandedOn(rec, len(es.units)-1)
	}
}

// keptDefsHandedOn re-arms the latch at depth level for a closure unit that
// runs a kept-defs body and is about to be invoked there (its finish, or a
// memo hit of it).
func (es *EmitState) keptDefsHandedOn(rec *fnUnitRec, level int) {
	if !rec.closure || rec.runsKeptDefs == "" {
		return
	}
	if es.keptDefsLevel == 0 || es.keptDefsLevel > level {
		es.keptDefsLevel = level
		es.keptDefsWord = rec.runsKeptDefs
	}
}

// keepsComputedDefs reports whether a dyn-body dispatch runs a COMPUTED code
// body whose defs and undefs the interpreter keeps in the enclosing scope: a
// keep-defs word over a body that is not concrete — its tokens exist only at
// run time, so the check pass modelled none of its binding changes — and that
// may be a code LIST (a Function callback runs in a frame of its own).
func keepsComputedDefs(spec *core.CallableSpec, body core.Value) bool {
	if !(spec.BodyOnceKeepsDefs || spec.BodyMultiRunKeepsDefs) || core.IsConcrete(body) {
		return false
	}
	return body.Dynamic || body.Parent == nil || body.Parent.ConformsTo(core.TList) || core.TList.ConformsTo(body.Parent)
}

// bodyProvenFn reports whether a computed body operand provably arrives as an
// interpreter fn value (strictFnOperandProven — a const fn, a member read over
// a const container holding one): a Function callback runs in a frame of its
// own and keeps nothing, however gradual its carrier (`for-each m.f xs` over
// `m {f: ([e:Integer] => …)}`).
func (es *EmitState) bodyProvenFn(body core.Value) bool {
	op, ok := es.resolveOperand(body)
	return ok && es.strictFnOperandProven(body, op)
}
