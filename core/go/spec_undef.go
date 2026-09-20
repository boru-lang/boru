package core

// A SPECULATIVE UNDEF — an `undef` of an ENCLOSING binding from inside a
// region the runtime may never execute (a branch arm, a loop body, a fn
// body; Registry.SpecUndefBlocked) — is the one transition the check pass
// deliberately keeps OUT of its model: popping the binding would flag
// `undefined_word` on every clean program whose handler never fires or
// whose loop runs zero times (the wrapped-undef FP class). The compiled
// lane cannot inherit that leniency as a bake: after the region runs the
// binding is gone, and a read the pass folded to its value answers where
// the interpreter raises (`def k 5  if true [undef k] []  k` compiled to 5;
// `while [k eq 5] [undef k]` never terminated — the sixty-seventh
// increment's measurement). The sixty-eighth increment places the
// transition and makes the reads live, and this file is the MODEL's half
// of it.

// GeneraliseSpecUndef is the model's answer to a speculative undef of
// name: the binding STAYS (the region may never run, so the pass reports
// nothing) but its value is no longer known — a fresh carrier of its type
// replaces the body IN PLACE. In place is the whole point: the depth is
// untouched, so the region's rollback (which truncates depth growth), the
// loop join (which collects depth growth) and the bind ledger (which
// notes pushes and pops) see no transition; the generation moves, so a
// unit that baked the binding re-records at its next call site
// (unit_memo's staleness key) and the rebind notification runs, so an
// escaping unit's bake declines exactly as it does for a `def` of the
// name; and every later read of the name is NON-CONCRETE — no fold, no
// const bake, a live lookup (the recorder's dynScopeRescue) that finds the
// binding when the region did not run and misses when it did, deferring
// to the interpreter that then raises the undefined_word the program
// earns. The loop analysis re-rounds when this moves (SpecUndefGen), so
// a read recorded BEFORE the undef in the same body is re-recorded live.
//
// False, with the model untouched, for a binding the compiled lane cannot
// pop live and read live: a type binding, a fn-family value (its reads
// are DISPATCHES, not lookups — the routed dispatch's half, not this
// one), the fn-carrier side table's, an active token, and a FRAME binding
// of an enclosing fn (a param or body-local, whose reads are slots). The
// carrier is minted once per binding: a second undef of the same
// generalised binding (a loop's later round, a second arm) neither
// re-mints nor moves the generation, which is what lets the loop rounds
// stabilise.
func GeneraliseSpecUndef(r *Registry, name string) bool {
	if r == nil || r.Check == nil || name == "" {
		return false
	}
	e, ok := r.Defs.TopEntry(name)
	if !ok || e.TypeDef != nil {
		return false
	}
	if len(r.FnBaselines) > 0 && r.Defs.Depth(name) > r.TopFnBaseline()[name] {
		return false
	}
	v := e.Body
	if IsAppliableFn(v) || IsSplice(v) || IsReach(v) || IsWord(v) || IsMark(v) || IsMove(v) {
		return false
	}
	if _, hit := CheckFnCarrierBind(r, name); hit {
		return false
	}
	if id := r.Check.SpecUndefCarriers[name]; id != "" && v.ID == id {
		return true
	}
	noteRebind(r, name)
	c := NewCarrier(v.Parent)
	r.Defs.Replace(name, c)
	if r.Check.SpecUndefCarriers == nil {
		r.Check.SpecUndefCarriers = map[string]string{}
	}
	r.Check.SpecUndefCarriers[name] = c.ID
	r.Check.SpecUndefGen++
	return true
}

// PopLiveBinding pops name's live top binding at RUN time — an `undef`
// outside the ledger: the placed twin of a speculative undef
// (OpUndefDynScope), a resident teardown's per-element pop
// (ApplyResidentBind) and a replayed undef twin (ApplyBindTwin) all take
// it. Retirement mirrors basic's undef: only a node THIS binding minted;
// an adopted alias node stays in the lattice. A missing binding is a
// no-op, as the interpreter's `undef` of an unbound name is (a loop's
// second iteration over an already-popped name).
func PopLiveBinding(r *Registry, name string) {
	if r == nil {
		return
	}
	if e, ok := r.Defs.PopEntry(name); ok && e.TypeDef != nil && e.Minted {
		r.Types.Retire(e.TypeDef)
	}
}
