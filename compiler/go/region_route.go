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
// span must be a plain value token or a PLAIN word, so the live walk meets
// no group, no interpolation, no sugar, and no dispatch modifier on a slot
// (`k/v`, `k/s` — syntax the host does not model, and a `/v` slot is one
// the op defers on unconditionally); and no slot beyond the record's claim
// may be a prior event's result, whose value is not on the stack when the
// live claim reaches it. Measured on the corpus before this landed: at the
// user seat inside units, 2 sites qualify and 24241 carry no live slot;
// the native seat's 708 are the next slice's.
//
// Two more declines are the CALL's, not the span's (both found in review of
// #460): a lead the run-time def stack does not hold (RegionDesc.LeadLocal —
// a body-local callee the committed CALL_USER reaches by index), and a
// callee with captures (RecordUserCall — they ride as trailing operands the
// routed op has no plumbing for).
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
	live := false
	for i := 0; i < d.NFwd && i < len(d.Slots); i++ {
		if d.Slots[i].Source != SlotWordRef {
			continue
		}
		if wi, err := core.AsWord(d.Slots[i].Token); err == nil {
			es.unfreezeRead(wi.Name)
			live = true
		}
	}
	return live
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
		if i >= d.NFwd && d.Slots[i].Source == SlotEvent {
			return false
		}
	}
	return true
}
