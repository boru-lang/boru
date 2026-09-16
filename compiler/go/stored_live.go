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
// refusing the whole program at any later rebind of a name the body reads
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
// with no declaration site — a lambda, a data value — refuses, since the
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

// noteUnitLive records that the innermost open unit reads name LIVE — a
// routed slot, a seated live read, a routed lead — so the stored-ref latch
// (NotifyNameRebound) knows the unit holds no bake of it.
func (es *EmitState) noteUnitLive(name string) {
	if es == nil || name == "" || len(es.openUnitRecs) == 0 {
		return
	}
	idx := es.openUnitRecs[len(es.openUnitRecs)-1]
	if idx < 0 || idx >= len(es.fnRecs) || es.fnRecs[idx] == nil {
		return
	}
	rec := es.fnRecs[idx]
	if rec.liveNames == nil {
		rec.liveNames = map[string]bool{}
	}
	rec.liveNames[name] = true
}

// unitLiveNames returns the names unit reads live, for a stored ref made
// over it.
func (es *EmitState) unitLiveNames(unit int) map[string]bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) || es.fnRecs[unit] == nil {
		return nil
	}
	return es.fnRecs[unit].liveNames
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
	switch v.Data.(type) {
	case core.FnDefInfo, *core.ClassTypeInfo:
		return false
	}
	if core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v) {
		return false
	}
	return true
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

// liveLeadWord reports whether word's routed dispatches resolve their lead
// live: a speculative fn family's, or a stored handler's dep.
func (es *EmitState) liveLeadWord(word string) bool {
	return es != nil && (es.specFnNames[word] || es.liveLeadNames[word])
}

// compileLiveLeadUnits gives every own signature of name's CURRENT binding
// a unit, after a module-scope transition of a live-lead name: the routed
// op runs the live signature's unit by its declaration site, and a
// binding the pass never dispatched elsewhere has none. A binding with no
// declared signature — a lambda's, a data value's — is one the op cannot
// run: refused through the undef site. Nothing while suspended (a
// transition inside a body the recorder does not record has no unit to
// compile against) — the name is then refused the same way.
func (es *EmitState) compileLiveLeadUnits(name string) {
	if es == nil || !es.Compilable || !es.liveLeadNames[name] || es.reg == nil {
		return
	}
	v, ok := es.reg.Defs.Top(name)
	if !ok {
		// Unbound: the live lookup raises the interpreter's undefined_word.
		return
	}
	if !es.Active() || !fnSigsDeclared(v) {
		es.refuseUndef(name, liveLeadUndeclared)
		return
	}
	fd := v.Data.(core.FnDefInfo)
	for i := range fd.Signatures {
		check.CompileFnSigUnit(es.reg, fd, i)
	}
}
