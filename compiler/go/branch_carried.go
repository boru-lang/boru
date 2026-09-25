package compiler

import "github.com/boru-lang/boru/core/go"

// The BRANCH-CARRIED def — a name bound inside an `if` arm and read after
// the merge.
//
// The recorder's model of a def is a compile-time fact: an ordinary arm
// `def` lowers to NOTHING (lowerDynBind), and a later read resolves
// statically through the producing event — which, for an arm def, sits on
// one of two mutually exclusive paths, so a read PAST the merge could not be
// placed at all ("body result of unknown provenance", the fn-locals-scope §6
// cluster) or, worse, baked the arm's own value as though the arm always ran
// (NUR110: `if false [def op 1] [0] end op` answered `0 1` for the
// interpreter's undefined_word).
//
// The mechanism here is the loop-carried def's (NoteLoopCarried), which
// already answers the same question for a name a loop body rebinds: ONE
// FRAME SLOT PER NAME PER UNIT (emitUnit.nameSlots — a loop and a branch
// carrying the same name share the cell, so nesting composes), every arm def
// of the name a STORE into that slot at its own site (emitDynBind.armSlot,
// lowered by lowerDynBind — so the store runs exactly when the arm runs),
// the pre-branch binding SEEDED into the slot before the branch when one
// stands (emitBranch.carried, lowered at the top of lowerBranch — an arm that
// does not bind then leaves the incoming binding, which is what the
// one-sided rows demand), and the joined carrier's identity aliased to the
// slot (emitUnit.localByID) so a read after the merge loads whichever arm
// ran.
//
// A slot means "bound since this frame started": a frame's locals begin as
// the zero Value and a name is bound from its first def until the frame
// ends, exactly the interpreter's frame-scoped dynamic binding (measured:
// `for 2 [if (i eq 0) [def z 9] [] end z]` is `9 9` interpreted — the
// binding an arm made in one iteration is still read in the next, so a
// seed must never re-run per branch execution; the seed is the PRE binding,
// and it is skipped when that binding already lives in the slot). A name
// with NO pre binding, bound in one arm only, may therefore be UNBOUND after
// the merge on the path that skipped the arm; its reads are bound-checked
// (emitUnit.boundLocals — OpPushLocalBound raises the interpreter's
// undefined_word on the zero slot), which is the fix NUR110 was owed: the
// program compiles and raises exactly where the interpreter does.
//
// What is NOT carried, and declines as before through the read's own site
// (the joined carrier is payload-less with a fresh identity, so nothing can
// bake it): a fn value, a type node or a module value (their consumers read
// the payload — family L's territory); an arm whose binding of the name is
// not a direct def and not a nested carry (a computed-`do` arm, a `_`-named
// def RecordDynBind never records); a pre binding the recorder cannot place.
// None of those is a new failure site: the read declines exactly where an
// unplaceable residual declines today.

// carryBranchJoins seats every name the branch left bound past its merge
// (core.BranchRecord.Joins — InstallJoinedDefs' record). ev is the branch
// event just appended; its arms are closed fragments.
func (es *EmitState) carryBranchJoins(ev *EmitEvent, b core.BranchRecord) {
	if len(b.Joins) == 0 || ev == nil || ev.br == nil {
		return
	}
	u := es.units[len(es.units)-1]
	for _, j := range b.Joins {
		es.carryBranchJoin(ev, b, u, j)
	}
}

func (es *EmitState) carryBranchJoin(ev *EmitEvent, b core.BranchRecord, u *emitUnit, j core.BranchJoin) {
	if j.Name == "" || j.Joined.ID == "" || core.IsCapitalisedName(j.Name) {
		return
	}
	if core.IsFnValueResidual(j.Joined) || (j.HasPre && core.IsFnValueResidual(j.Pre)) {
		return
	}
	br := ev.br
	// The arms that rebind the name. On a constant-condition branch only the
	// TAKEN arm was analysed and it sits in `then` whichever arm it is; a
	// taken arm with no pre binding pushed its own value (condBoundCarrier
	// leaves it), whose home stands — nothing to seat.
	var arms []*EmitFragment
	switch {
	case j.Taken:
		if !j.HasPre {
			return
		}
		arms = []*EmitFragment{br.then}
	default:
		if j.ThenBinds {
			arms = append(arms, br.then)
		}
		if j.ElseBinds {
			arms = append(arms, br.els)
		}
	}
	if len(arms) == 0 {
		return
	}
	// The slot is allocated only once every binding arm is known to carry
	// the name out on every path through it: a direct def at the arm's top
	// level (marked below), or a nested branch / loop that already carries
	// it into the same cell.
	for _, a := range arms {
		if a == nil || !fragCanCarry(es, a, j.Name) {
			return
		}
	}
	// The seed: the pre-branch binding, resolved in the enclosing scope. A
	// binding the recorder cannot place is not seated at all — the read then
	// declines as it did before. A pre binding that belongs to ANOTHER
	// FRAME is never seeded: inside a fn unit the interpreter reads such a
	// name dynamically (a caller's frame-local, a module-scope def), and
	// resolving it here would route through dynScopeRescue, whose
	// commitment is program-wide BY NAME — measured on utils/sort.boru,
	// where the recursive callee's arm-local `k` met the caller's `k` as its
	// pre, and every other fn binding a `k` then had to lower a dyn-scope
	// install it could not. The name is left unseated; its read after the
	// merge declines exactly as before.
	var seed *EmitOperand
	if j.HasPre && !j.Taken {
		if len(es.units) > 1 && (u.enclosingBindIDs[j.Pre.ID] || u.enclosingIDs[j.Pre.ID]) {
			return
		}
		had := es.dynScopeNames[j.Name]
		init, ok := es.resolveOperand(j.Pre)
		if ok && (init.kind == opDynScope || init.kind == opDataScope) {
			ok = false
		}
		if !ok {
			if !had {
				delete(es.dynScopeNames, j.Name)
			}
			return
		}
		seed = &init
	}
	slot := es.unitNameSlot(u, j.Name)
	if seed != nil && !(seed.kind == opLocal && seed.idx == slot) {
		br.carried = append(br.carried, carriedInit{slot: slot, init: *seed})
	}
	for _, a := range arms {
		markArmBinds(a, j.Name, slot)
	}
	if br.carriedNames == nil {
		br.carriedNames = map[string]bool{}
	}
	br.carriedNames[j.Name] = true
	// NOT es.carriedNames: that set is the LOOP-carried name's program-wide
	// hazard flag, read by name by the undef handler (DeclineCarriedUndef)
	// and the routed-read guard, and a branch-carried name is as common as
	// `p` or `k` — measured: adding them declined kg/main.boru on an
	// unrelated fn's undef of `p`. The undef hazard a branch-carried slot
	// has (an undef exposing a binding whose home is the same slot) is
	// unit-scoped, and nameCarried reads the unit's nameSlots for it; the
	// routed-read guard is not owed here, because an arm def a routed
	// dispatch reads keeps its registry twin (routedBindsDyn), so the
	// registry is current on the path that ran the arm and empty on the
	// path that did not — the interpreter's own state on both.
	u.localByID[j.Joined.ID] = slot
	// Bound-checked when some path to the merge may leave the slot never
	// stored: no pre binding and not every arm binding (a taken arm always
	// binds), or a pre binding that is itself possibly unbound.
	bothBind := b.HasElse && j.ThenBinds && j.ElseBinds
	unbound := !j.Taken && !bothBind && (!j.HasPre || u.boundLocals[j.Pre.ID] != "")
	if j.HasPre && bothBind {
		unbound = false
	}
	if unbound {
		if u.boundLocals == nil {
			u.boundLocals = map[string]string{}
		}
		u.boundLocals[j.Joined.ID] = j.Name
		es.noteCondBound(j.Name)
	}
	// The residual-order hazard (unit_memo.go): a fragment that read the
	// name BEFORE this branch's stores, through THIS SLOT, holds a read whose
	// end-of-fragment re-push would load the stored value. A read resolves
	// to the slot only when the binding it read was already aliased there —
	// the pre binding an earlier join or loop carried — so only then is the
	// slot-keyed hazard marked, for the enclosing fragments and each
	// storing arm; a read of a pre binding with its own home re-pushes that
	// home and is untouched by the stores. RecordDynBind marked the
	// name-keyed twin at each def.
	if !j.HasPre || u.localByID[j.Pre.ID] != slot {
		return
	}
	fids := append([]int{0}, es.fragIDs...)
	for _, a := range arms {
		fids = append(fids, a.id)
	}
	for _, fid := range fids {
		if !es.fragReads[readKey{j.Name, fid}] {
			continue
		}
		if es.storeHazard == nil {
			es.storeHazard = map[slotKey]bool{}
		}
		es.storeHazard[slotKey{slot, fid}] = true
	}
}

// unitNameSlot is the frame slot unit u carries name in — a live armed loop's
// cell for the name first (carriedSlot), then the unit's own table, else a
// fresh slot registered there. One cell per name per unit is what lets a loop
// and a branch (and nested branches) carrying the same name compose.
func (es *EmitState) unitNameSlot(u *emitUnit, name string) int {
	if s, ok := es.carriedSlot(name); ok {
		u.setNameSlot(name, s)
		return s
	}
	if s, ok := u.nameSlots[name]; ok {
		return s
	}
	s := u.numLocals
	u.numLocals++
	u.setNameSlot(name, s)
	return s
}

func (u *emitUnit) setNameSlot(name string, slot int) {
	if u.nameSlots == nil {
		u.nameSlots = map[string]int{}
	}
	u.nameSlots[name] = slot
}

// fragCanCarry reports whether every path through the arm frag leaves the
// name's current binding in its slot: the arm's top-level events include a
// direct def of the name that can store (a value-binding dyn-bind event the
// recorder captured, with a value the lowering can re-push), or a nested
// branch or loop that already carries it — and NO direct def of the name
// whose value cannot be stored. Storability is decided HERE, not at the
// lowering: a name that cannot be carried is left unseated, so a program
// that never reads it past the merge lowers exactly as before (measured on
// the generated sweep, where a `word` value bound in an arm declined eight
// call-form variants that had compiled).
func fragCanCarry(es *EmitState, frag *EmitFragment, name string) bool {
	carried := false
	for i := range frag.events {
		ev := &frag.events[i]
		switch ev.kind {
		case evDynBind:
			if ev.dyn == nil || ev.dyn.name != name || !ev.dyn.bindsValue() || ev.dyn.specFn {
				continue
			}
			if ev.dyn.carried {
				// An armed loop already stores this def into the name's
				// cell (RecordDefRebind's evStore — the same cell, by
				// unitNameSlot), and validated its value there.
				carried = true
				continue
			}
			if !es.dynBindStorable(ev.dyn) {
				return false
			}
			carried = true
		case evBranch:
			if ev.br != nil && ev.br.carriedNames[name] {
				carried = true
			}
		case evLoop:
			if ev.loop != nil && ev.loop.carriedNames[name] {
				carried = true
			}
		}
	}
	return carried
}

// dynBindStorable reports whether a def's bound value can be re-pushed for
// a frame-slot store: a non-variadic computed source (promoted or on the
// stack top at the def), a local, a const, or an inert literal. Nothing
// else is probed — resolving an arbitrary value here could commit a name to
// dynamic scope program-wide (dynScopeRescue), which is the cost this whole
// mechanism exists to avoid.
func (es *EmitState) dynBindStorable(d *emitDynBind) bool {
	switch {
	case d.srcSeq >= 0:
		return !es.eventInfo[d.srcSeq].variadicResult
	case d.src.kind == opLocal || d.src.kind == opConst:
		return true
	case d.src.kind == opNone:
		return core.IsInertConst(d.val)
	}
	return false
}

// markArmBinds stamps every top-level def of name in the arm with the slot
// it stores into. A def an armed loop already stores (RecordDefRebind's
// evStore, emitDynBind.carried) is left alone: that store targets the same
// cell. A def whose value the lowering cannot re-push declines there,
// under its own reason.
func markArmBinds(frag *EmitFragment, name string, slot int) {
	for i := range frag.events {
		ev := &frag.events[i]
		if ev.kind != evDynBind || ev.dyn == nil || ev.dyn.name != name || !ev.dyn.bindsValue() || ev.dyn.specFn {
			continue
		}
		if ev.dyn.carried {
			continue
		}
		ev.dyn.armCarried, ev.dyn.armSlot = true, slot
		ev.dyn.armSrcOuter = ev.dyn.srcSeq >= 0 && ev.dyn.srcSeq <= frag.startSeq
	}
}
