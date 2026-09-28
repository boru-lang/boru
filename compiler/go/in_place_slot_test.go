package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// withID stamps an identity on a value built outside a check pass, where
// core.NewValueRaw leaves it empty.
func withID(v core.Value, id string) core.Value {
	v.ID = id
	return v
}

// A token the collection kernel evaluated in place (an interpolation) reaches
// the dispatch as a fresh value; the recorder's link lets the region
// completion recognise it as that slot's operand — and only that slot's.
func TestNoteInPlaceSlotLinksTheSlotOperand(t *testing.T) {
	es := NewEmitState()
	tok := withID(core.NewInterpString([]core.InterpPart{{Lit: "a"}}), "tok")
	res := withID(core.NewString("a"), "res")
	other := withID(core.NewString("a"), "other")
	slot := SlotDesc{Token: tok}

	if es.slotIsOperand(slot, nil, res) {
		t.Fatal("unlinked: a fresh value is not the slot's operand")
	}
	es.NoteInPlaceSlot(tok, res)
	if !es.slotIsOperand(slot, nil, res) {
		t.Error("linked: the in-place value is the slot's operand")
	}
	if es.slotIsOperand(slot, nil, other) {
		t.Error("another value with the same payload is not the slot's operand")
	}
	if es.slotIsOperand(SlotDesc{Token: withID(core.NewString("b"), "tok2")}, nil, res) {
		t.Error("the link names ONE token: another slot does not claim the value")
	}
}

// Identity-less values never link (an empty id never corresponds), and an
// inactive recorder records nothing.
func TestNoteInPlaceSlotDeclines(t *testing.T) {
	es := NewEmitState()
	es.NoteInPlaceSlot(core.Value{}, withID(core.NewString("x"), "r"))
	es.NoteInPlaceSlot(withID(core.NewString("t"), "t"), core.Value{})
	if len(es.inPlaceFrom) != 0 {
		t.Errorf("an empty identity must link nothing, got %v", es.inPlaceFrom)
	}
	es.Compilable = false
	es.NoteInPlaceSlot(withID(core.NewString("t"), "t"), withID(core.NewString("x"), "r"))
	if len(es.inPlaceFrom) != 0 {
		t.Errorf("an inactive recorder must link nothing, got %v", es.inPlaceFrom)
	}
}

// An interpolation or XML literal beyond the recorded claim is one only an
// evaluation could reach, which the descriptor host cannot perform; inside
// the claim it is already a value on the stack.
func TestRegionDrivableInPlaceTokens(t *testing.T) {
	interp := core.NewInterpString([]core.InterpPart{{Lit: "a"}})
	xml := core.NewXmlInterp(core.XmlTmpl{})
	for name, tok := range map[string]core.Value{"interp": interp, "xml": xml} {
		beyond := &RegionDesc{Word: "w", Slots: []SlotDesc{{Token: tok}}}
		if regionDrivable(beyond) {
			t.Errorf("%s beyond the claim must not be drivable", name)
		}
		claimed := &RegionDesc{Word: "w", NFwd: 1, Slots: []SlotDesc{{Token: tok, Source: SlotEvent}}}
		if !regionDrivable(claimed) {
			t.Errorf("%s inside the claim is a stack value: drivable", name)
		}
	}
	if !regionDrivable(&RegionDesc{Word: "w", Slots: []SlotDesc{{Token: core.NewInteger(1)}}}) {
		t.Error("a plain constant beyond the claim stays drivable")
	}
}

// Two results of one runtime-variable call are settled data; a lead from a
// different event, a single entry, an unproduced value or a fixed-count call
// are not this rule's.
func TestVariadicSiblingLead(t *testing.T) {
	es := NewEmitState()
	a, b, c := withID(core.NewInteger(1), "a"), withID(core.NewInteger(2), "b"), withID(core.NewInteger(3), "c")
	es.producedBy[a.ID] = producer{seq: 1, idx: 0}
	es.producedBy[b.ID] = producer{seq: 1, idx: 1}
	es.producedBy[c.ID] = producer{seq: 2, idx: 0}

	if es.variadicSiblingLead([]core.Value{a, b}) {
		t.Error("a fixed-count call's results are not settled by this rule")
	}
	es.eventInfo[1] = eventFlags{callVariadic: true}
	if !es.variadicSiblingLead([]core.Value{a, b}) {
		t.Error("a fallible multi-value do body's two results are settled")
	}
	es.eventInfo[1] = eventFlags{dynBodyResult: true, variadicResult: true}
	if !es.variadicSiblingLead([]core.Value{a, b}) {
		t.Error("a dyn-body code body's two results are settled")
	}
	es.eventInfo[1] = eventFlags{dynBodyResult: true}
	if es.variadicSiblingLead([]core.Value{a, b}) {
		t.Error("a fixed-count dyn-body value eval is not this rule's")
	}
	es.eventInfo[1] = eventFlags{callVariadic: true}
	if es.variadicSiblingLead([]core.Value{a, c}) {
		t.Error("an entry from another event is not a sibling")
	}
	if es.variadicSiblingLead([]core.Value{a}) {
		t.Error("a lone entry has no sibling")
	}
	if es.variadicSiblingLead([]core.Value{withID(core.NewInteger(4), "d"), b}) {
		t.Error("an unproduced lead is not a sibling")
	}
	if es.variadicSiblingLead([]core.Value{a, withID(core.NewInteger(5), "e")}) {
		t.Error("an unproduced entry above is not a sibling")
	}
}
