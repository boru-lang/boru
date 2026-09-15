package compiler

import core "github.com/boru-lang/boru/core/go"

// The routing decision for the generic lane's first ROUTED shape
// (design/FULL-COMPILATION.0.md §6.2, §6.5; Stage 4): a user-fn dispatch
// inside a fn unit whose claim carries a LIVE word slot — the frozen-read
// class the binding-sensitive memo re-records and the escaping latch
// refuses — is lowered through its descriptor (OpDispatchGeneric) instead
// of the committed CALL_USER, so the word is looked up at every execution
// as the interpreter looks it up.
//
// The shape is chosen by what the VM's descriptor host can DRIVE without an
// evaluation (region_host.go declines every evaluation): every slot in the
// span must be a plain SCALAR value token or a PLAIN word, so the live walk
// meets no group, no interpolation, no sugar, no dispatch modifier on a
// slot (`k/v`, `k/s` — syntax the host does not model, and a `/v` slot is
// one the op defers on unconditionally) — and no list or map literal,
// whose contents the interpreter EVALUATES on arrival (a data list's words
// and groups run then; the COLLECT oracle's "declined at a compound stop"
// is this limit seen from the scan's side); and no slot beyond the
// record's claim may be a prior event's result, whose value is not on the
// stack when the live claim reaches it. Measured on the corpus before the
// user seat landed (the sixty-fourth increment): 2 user-seat sites inside
// units and 708 native-seat ones qualified, 24241 and 90018 carried no live
// slot. The native seat's MONO records route since the sixty-fifth
// increment: the op's native arm calls the live handler when it is the
// record's own (GenericSpec.Impl carries the recorded signature's
// implementation) and there is no committed unit to enter
// (GenericSpec.Unit is -1). A POLY native record keeps CALL_NATIVE_POLY:
// it commits to no one implementation the op could name as the record's.
//
// Two more declines are the CALL's, not the span's (both found in review of
// #460): a lead the run-time def stack does not hold (RegionDesc.LeadLocal —
// a body-local callee the committed CALL_USER reaches by index), and a
// callee with captures (RecordUserCall — they ride as trailing operands the
// routed op has no plumbing for). A third is the NAME's: a loop-carried
// name lives in a frame slot for the rest of the run, not in the registry
// the routed op reads, so a read of one keeps its bake, and a loop that
// later carries a routed name refuses (EmitState.carriedNames /
// routedNames, found on the sixty-fifth increment's tree).
//
// A routed read is no longer a BAKE the unit depends on: unfreezeRead
// retires the note NoteFrozenRead made when the operand was resolved, so the
// memo does not re-record the unit for a rebind the dispatch already
// honours, and the escaping latch does not refuse it.

// routeRegion decides whether a completed descriptor drives its dispatch,
// retiring the frozen notes of the word slots it makes live. Nil for a
// dispatch with no descriptor, false for a lead the live lookup cannot find,
// and false outside a fn unit — at top level analysis order is program order
// and the bake IS the read.
func (es *EmitState) routeRegion(d *RegionDesc) bool {
	if d == nil || d.LeadLocal || !es.Active() || len(es.openUnitRecs) == 0 || !regionDrivable(d) {
		return false
	}
	var names []string
	for i := 0; i < d.NFwd && i < len(d.Slots); i++ {
		if d.Slots[i].Source != SlotWordRef {
			continue
		}
		if wi, err := core.AsWord(d.Slots[i].Token); err == nil {
			// A name a loop carries lives in a frame slot, not in the
			// registry the routed op reads (EmitState.carriedNames): the
			// committed call and its bake stay, decided before any note is
			// retired.
			if es.carriedNames[wi.Name] {
				return false
			}
			names = append(names, wi.Name)
		}
	}
	if len(names) == 0 {
		return false
	}
	if es.routedNames == nil {
		es.routedNames = map[string]bool{}
	}
	for _, name := range names {
		es.unfreezeRead(name)
		es.routedNames[name] = true
	}
	return true
}

// regionDrivable reports whether the VM's descriptor host can walk d
// without an evaluation and materialise every slot a live claim can reach.
func regionDrivable(d *RegionDesc) bool {
	for i := range d.Slots {
		tok := d.Slots[i].Token
		if _, kind := core.StaticForwardTypeOf(tok); kind != core.FwdValue && !core.IsWord(tok) {
			return false
		}
		if wi, err := core.AsWord(tok); err == nil && !plainWord(wi) {
			return false
		}
		if core.HasContainerIdentity(tok) {
			return false
		}
		if i >= d.NFwd && d.Slots[i].Source == SlotEvent {
			return false
		}
	}
	return true
}
