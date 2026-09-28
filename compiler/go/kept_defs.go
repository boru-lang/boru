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
	es.keptDefsFresh = nil
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

// noteKeptDefsRead is NoteDefRead's latch check. A read the recorder seats
// LIVE observes nothing stale — the lookup (OpLookupDynScope) reads the
// registry the body installed into, raises the interpreter's undefined_word
// on a miss and bails on a fn value it would dispatch — so while the latch is
// armed, a read that may be seated live (a name a computed keep-defs body
// leaked into the unit, NUR203's noteDynKeepDefsLeak; a root read after a
// root one, rootDynLeak) waits for the tag hook that follows it
// (NoteLiveRead). Every other read is an observer now (NUR282).
func (es *EmitState) noteKeptDefsRead(id, name string) {
	es.flushKeptRead()
	if es.keptDefsLevel != 0 && id != "" && (es.keepLeakNames[name] || (es.rootDynLeak && es.TopFrameOnly())) {
		es.pendingKeptRead, es.pendingKeptName = id, name
		return
	}
	es.noteKeptDefsObserver("the read of `" + name + "`")
}

// keptReadSeatedLive settles the pending read NoteLiveRead is seating live:
// it observes nothing stale, and its value becomes a CARRIER of its type, so
// no container literal or const fold downstream can bake the check model's
// pre-body value where the live lookup stands (`… each b xs drop [t]` baked
// [0] for the interpreter's [3] when the read stayed concrete). A read that
// may hold a fn value stays an observer: a fn is interned as a const, which
// the carrier would not stop.
func (es *EmitState) keptReadSeatedLive(v *core.Value) {
	if v.ID == "" || es.pendingKeptRead != v.ID {
		return
	}
	name := es.pendingKeptName
	es.pendingKeptRead, es.pendingKeptName = "", ""
	if keptReadMayBeFn(*v) {
		es.noteKeptDefsObserver("the read of `" + name + "`")
		return
	}
	if core.IsTypeLiteral(*v) {
		*v = core.WithPos(core.ValueCarrier(*v), *v) // a type VALUE (NUR323)
		return
	}
	*v = core.WithPos(core.NewCarrier(v.Parent), *v)
}

// flushKeptRead makes a pending read an observer: nothing seated it live
// before the next recorded event, dispatch or Finalize.
func (es *EmitState) flushKeptRead() {
	if es.pendingKeptRead == "" {
		return
	}
	name := es.pendingKeptName
	es.pendingKeptRead, es.pendingKeptName = "", ""
	es.noteKeptDefsObserver("the read of `" + name + "`")
}

// keptDefsEvent is appendEvent's hook: an event that observes a binding is
// checked against the latch FIRST (a call of a kept-defs unit observes what
// ran before it), then an event that may run a kept-defs unit arms it.
func (es *EmitState) keptDefsEvent(ev *EmitEvent) {
	es.flushKeptRead()
	if what := keptDefsObserverEvent(ev); what != "" {
		es.noteKeptDefsObserver(what)
		// A user call or a fn-value apply may run code the model did not
		// see (a recursive call of a unit still open, whose kept-defs run
		// is decided at its finish): no binding is fresh past it.
		es.keptDefsFresh = nil
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
	es.keptDefsFresh = nil
	es.generaliseRootValues()
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
		es.keptDefsFresh = nil
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
	es.keptDefsFresh = nil
	es.generaliseRootValues()
}

// generaliseRootValues is NUR281's: a kept-defs body that runs at the PROGRAM
// level may rebind or unbind any root value binding, and a map literal's value
// const-folds off the emit path (core AutoEvalMap's fold), where no read
// reaches the latch — `[1 2] each (mk) end {a: x}` baked the pre-body x. So
// the pass stops knowing root values there, as `do`'s check half already does
// (basic generaliseRootValues): each binding becomes a fresh carrier of its
// type (the speculative undef's transition), which the fold stands aside for
// and whose read reaches the latch. Only at the root: inside a unit the run's
// defs land in the unit's frame, and the latch re-arms where the unit runs.
func (es *EmitState) generaliseRootValues() {
	r := es.reg
	if len(es.units) != 1 || r == nil || r.Check == nil || r.Check.FnBodyDepth > 0 {
		return
	}
	for _, name := range r.Defs.Names() {
		core.GeneraliseSpecUndef(r, name)
	}
}

// noteKeptDefsFreshBind records a def of name to the value id made while the
// latch is armed, at the latch's own unit depth: the def runs AFTER the
// computed body, on both lanes, so the binding it installs is the one the
// model holds — `def ok (do b error [drop false])  if ok […]`, the mini-s3
// handler shape, reads its own fresh result, not anything the body could
// have bound. A def in a deeper unit binds that unit's frame, which a read
// back at the latch's depth does not see; it is not recorded.
func (es *EmitState) noteKeptDefsFreshBind(name, id string) {
	if es.keptDefsLevel == 0 || es.keptDefsLevel != len(es.units) || id == "" {
		return
	}
	if es.keptDefsFresh == nil {
		es.keptDefsFresh = map[string]string{}
	}
	es.keptDefsFresh[name] = id
}

// keptDefsFreshRead reports whether a def read of name, resolving the value
// id, reads a FRESH binding (noteKeptDefsFreshBind) at the latch's depth. The
// ID must match: a read the model resolves to any other value — the join of
// a conditional def, a binding from before the run — is still an observer.
func (es *EmitState) keptDefsFreshRead(id, name string) bool {
	return es.keptDefsLevel == len(es.units) && es.keptDefsFresh[name] == id && id != ""
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

// NoteValReadLive gives a `/v` read the discipline a bare read of the same
// binding takes (NUR334). A bare read reaches the latch through NoteDefRead
// and is seated live by the tag hook (NoteLiveRead); stepWordVal resolves
// the binding its own way and used to note neither, so after a computed
// keep-defs body the value spelling baked the check model's stale binding:
// `def f fn [[b:List][Any][def t 0 do b drop t/v]] end f (quote [def t 5
// 1])` answered 0 compiled for the interpreter's 5. Now the read is a def
// read to the latch (an observer, or pending), and a read of a leaked name
// seats live at its token as the bare read's does — lowered to
// OpLookupDynScopeRef, the value spelling's lookup, which pushes whatever the
// binding holds at run time (a fn or a class as data) as stepWordVal does. A
// value the model holds as a possible fn (keptReadMayBeFn) stays unseated, as
// the bare read's does, so a pending one becomes an observer and declines.
func (es *EmitState) NoteValReadLive(v *core.Value, name string, pos core.SrcPos) {
	if !es.Active() || v == nil || v.ID == "" || name == "" {
		return
	}
	if !es.keptDefsFreshRead(v.ID, name) {
		es.noteKeptDefsRead(v.ID, name)
	}
	keepLive, rootLive := es.keptLeakLive(*v, name)
	if !keepLive && !rootLive {
		return
	}
	if keptReadMayBeFn(*v) {
		es.flushKeptRead()
		return
	}
	if es.liveReadNames == nil {
		es.liveReadNames = map[string]bool{}
	}
	es.liveReadNames[name] = true
	es.keptReadSeatedLive(v)
	es.computedLeakGradual(v, name, rootLive)
	es.seatLiveRead(v, name, pos, true)
}

// computedLeakGradual makes a `/v` read seated live after a COMPUTED
// keep-defs body gradual (NoteValReadLive; rootDynLeak's root read, a name
// noteDynKeepDefsLeak leaked in the unit): the body's tokens exist only at
// run time, so it may have bound the name to a value of any type, and a
// carrier of the pre-body type let the consumer commit to that type's
// overload — `do (mk) end x/v add 1` over `quote [def x "s"]` would run
// Integer add and answer 1 for the interpreter's "s1". The dynamic modality
// keeps the bound as a best static guess and leaves the consumer's dispatch
// to the run. A literal body's leak keeps its exact type (the pass ran its
// tokens), and a value that may be a fn stays as keptReadSeatedLive left it.
// (The bare read's seat keeps the static carrier: a gradual bare read is a
// word the run may dispatch, which the residual rules decline.)
func (es *EmitState) computedLeakGradual(v *core.Value, name string, rootLive bool) {
	if !(rootLive || es.dynLeakNames[name]) || !v.Carrier || keptReadMayBeFn(*v) {
		return
	}
	v.Dynamic = true
}

// keptReadMayBeFn reports whether a read the latch would seat live holds a
// possible fn value in the model — an appliable fn, an untyped value (the
// Any node), a Function carrier — which stays an observer: a fn is interned
// as a const the carrier would not stop, and an untyped value has no type
// to carry.
func keptReadMayBeFn(v core.Value) bool {
	return core.IsAppliableFn(v) || v.Parent == nil || v.Parent.ConformsTo(core.TFunction)
}
