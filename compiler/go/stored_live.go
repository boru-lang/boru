package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// The stored-handler latch's lookup half (the seventy-first increment).
//
// A stored service handler (`add {} (r state => […]) svc`, spawn's body) is
// compiled to a unit at its STORE site and invoked by the host at CALL
// time, so the memo that re-records every other unit at its next call site
// never sees it again: whatever the unit baked of a module-scope binding —
// a data def's value as a PUSH_CONST, a helper fn as a committed CALL_USER
// — stayed frozen while the interpreter resolves the same names when the
// handler runs. NotifyNameRebound's stored-ref latch answered that by
// declining the whole program at any later rebind of a name the body reads
// (design/RELOAD-INVALIDATION.0.md §3 F1: a mid-program rebind must give
// each call its point-in-program binding, which the twin regime's replay
// now makes real at VM time).
//
// The lookup half removes the bake instead: inside a stored-ref unit a
// module-scope name is read LIVE — a bare read is seated at its token as
// OpLookupDynScope (NoteLiveRead, the sixty-eighth increment's seat), a
// word slot routes with its dispatch (routeRegion, the sixty-fifth's), and
// a fn dispatched by name routes with a live lead through the descriptor
// the speculative fn family takes (RecordUserCall, the seventieth's): the
// op resolves the lead in the registry at the call, runs the live
// signature's own unit by its declaration site, and raises the
// interpreter's undefined_word on a miss. Every rebind of such a name
// compiles the new binding's own signatures to units (RecordBindTwin), so
// the routed op always has the live signature's unit; a rebind to a value
// with no declaration site — a lambda, a data value — declines, since the
// op could not run it. The latch keeps firing for a name a unit BAKED
// (fnUnitRec.liveNames says which it did not).

// storedUnitOpen returns the innermost open unit's record when that unit
// is a stored-ref unit (a stored handler's or spawn body's), nil otherwise:
// a closure body nested inside the handler is its own unit, and its reads
// keep their bake and the latch.
func (es *EmitState) storedUnitOpen() *fnUnitRec {
	if es == nil || len(es.openUnitRecs) == 0 {
		return nil
	}
	idx := es.openUnitRecs[len(es.openUnitRecs)-1]
	if idx < 0 || idx >= len(es.fnRecs) || es.fnRecs[idx] == nil || !es.fnRecs[idx].storedRefUnit {
		return nil
	}
	return es.fnRecs[idx]
}

// openUnitRecSafe is openUnitRec with a nil recorder and an index outside
// the table answered nil rather than a fault (the seats below are reached
// from hooks that run on an inactive recorder too).
func (es *EmitState) openUnitRecSafe() *fnUnitRec {
	if es == nil || len(es.openUnitRecs) == 0 {
		return nil
	}
	idx := es.openUnitRecs[len(es.openUnitRecs)-1]
	if idx < 0 || idx >= len(es.fnRecs) {
		return nil
	}
	return es.fnRecs[idx]
}

// noteUnitLive records that the innermost open unit reads name LIVE — a
// routed slot, a seated live read, a routed lead — so the stored-ref latch
// (NotifyNameRebound) knows the unit holds no bake of it. Each seat counts,
// against the bake notes the same reads made (noteUnitBaked).
func (es *EmitState) noteUnitLive(name string) {
	rec := es.openUnitRecSafe()
	if rec == nil || name == "" {
		return
	}
	if rec.liveNames == nil {
		rec.liveNames = map[string]bool{}
		rec.storedSeats = map[string]int{}
	}
	rec.liveNames[name] = true
	rec.storedSeats[name]++
}

// noteUnitBaked counts a frozen note (NoteFrozenRead) a stored-ref unit's
// read made — the engine notes a concrete module-scope value read, a type
// read and a committed call target BEFORE the read is tagged, so a read the
// seat then takes is one seat against one bake, and a read no seat takes —
// a `/v` read, a call the op could not route — is a bake the latch keeps.
func (es *EmitState) noteUnitBaked(name string) {
	rec := es.openUnitRecSafe()
	if rec == nil || name == "" {
		return
	}
	if rec.storedBakes == nil {
		rec.storedBakes = map[string]int{}
	}
	rec.storedBakes[name]++
}

// unitLiveNames returns the names unit reads live AND nowhere baked — the
// names a rebind leaves nothing stale in — for a stored ref made over it. A
// name whose bakes outnumber its seats (review of #467: `helper 5` routed
// beside a baked `helper/v`) is left to the latch.
func (es *EmitState) unitLiveNames(unit int) map[string]bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) || es.fnRecs[unit] == nil {
		return nil
	}
	rec := es.fnRecs[unit]
	var live map[string]bool
	for name := range rec.liveNames {
		if rec.storedBakes[name] > rec.storedSeats[name] {
			continue
		}
		if live == nil {
			live = map[string]bool{}
		}
		live[name] = true
	}
	return live
}

// storedDepRead reports whether a bare read of name, inside the open
// stored-ref unit, is a module-scope binding the unit should read live: the
// value is one OpLookupDynScope pushes (not a dispatching binding — a fn, a
// class — nor an active token). A read the check pass already tagged
// dynamic (a flex map, a store, a module namespace) is live by that
// machinery and is noted so, seated nowhere new.
func (es *EmitState) storedDepRead(name string, v core.Value) bool {
	if es == nil || es.reg == nil || es.storedUnitOpen() == nil || !core.ModuleScopeBinding(es.reg, name) {
		return false
	}
	if _, ok := es.reg.Defs.Top(name); !ok {
		return false
	}
	if v.Dynamic {
		es.noteUnitLive(name)
		return false
	}
	return !dispatchingBinding(v)
}

// markLiveLead marks word a live lead when a stored-ref unit dispatches it
// as a module-scope fn with declared signatures (fnSigsDeclared — the
// identity the routed op locates a unit by). Reports whether it did.
func (es *EmitState) markLiveLead(word string) bool {
	if es == nil || es.reg == nil || word == "" || es.storedUnitOpen() == nil || !core.ModuleScopeBinding(es.reg, word) {
		return false
	}
	v, ok := es.reg.Defs.Top(word)
	if !ok || !fnSigsDeclared(v) {
		return false
	}
	if es.liveLeadNames == nil {
		es.liveLeadNames = map[string]bool{}
	}
	es.liveLeadNames[word] = true
	return true
}

// noteLiveNameTransition follows a module-scope transition of a name a
// stored handler reads live, after the install (RecordBindTwin). A live
// LEAD's new binding gets every own signature a unit (compileLiveLeadUnits):
// the routed op runs the live signature's unit by its declaration site, and
// a binding the pass never dispatched elsewhere has none. A live READ's new
// binding must be one OpLookupDynScope pushes: a fn, a class or an active
// token it would DISPATCH — deferring past the handler's effects where the
// interpreter runs it — is declined through the undef site (review of
// #467). An unbound name is the miss the ops raise as undefined_word.
func (es *EmitState) noteLiveNameTransition(name string) {
	if es == nil || !es.Compilable || es.reg == nil || (!es.liveLeadNames[name] && !es.liveReadNames[name]) {
		return
	}
	v, ok := es.reg.Defs.Top(name)
	if !ok {
		return
	}
	if es.liveReadNames[name] && dispatchingBinding(v) {
		es.refuseUndef(name, liveReadDispatching)
		return
	}
	if es.liveLeadNames[name] {
		es.compileLiveLeadUnits(name, v)
	}
}

// dispatchingBinding reports whether a binding is one the lookup op cannot
// push — the arms OpLookupDynScope defers on.
func dispatchingBinding(v core.Value) bool {
	switch v.Data.(type) {
	case core.FnDefInfo, *core.ClassTypeInfo:
		return true
	}
	return core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v)
}

// compileLiveLeadUnits gives every own signature of v, name's current
// binding, a unit. A binding with no declared signature — a lambda's, a
// data value's — is one the op cannot run: declined through the undef site.
// Nothing while suspended (a transition inside a body the recorder does
// not record has no unit to compile against) — declined the same way.
func (es *EmitState) compileLiveLeadUnits(name string, v core.Value) {
	if !es.Active() || !fnSigsDeclared(v) {
		es.refuseUndef(name, liveLeadUndeclared)
		return
	}
	fd := v.Data.(core.FnDefInfo)
	for i := range fd.Signatures {
		check.CompileFnSigUnit(es.reg, fd, i)
	}
}
