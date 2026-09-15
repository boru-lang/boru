package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// okDesc is a well-formed one-slot descriptor whose slot is INSIDE the
// recorded claim (NFwd 1). The claim bound matters to almost every case
// below: outside it a slot is not this dispatch's operand and carries no
// source by design, so a fixture that left NFwd at zero would be asserting
// the relaxation rather than the rule (see TestValidateBeyondTheClaim).
func okDesc() *RegionDesc {
	return &RegionDesc{
		Lead:  LeadWord,
		Word:  "f",
		NFwd:  1,
		Slots: []SlotDesc{{Source: SlotConst, Idx: 0}},
	}
}

func TestRegionDescValidateAccepts(t *testing.T) {
	if err := okDesc().Validate(1, 0, 0); err != nil {
		t.Errorf("a well-formed descriptor must validate, got %v", err)
	}
}

// The whole point of the sentinel: an unset source is REJECTED rather than
// silently read as a reference to Consts[0].
func TestRegionDescValidateRejectsUnsetSource(t *testing.T) {
	d := okDesc()
	d.Slots[0].Source = SlotNone
	err := d.Validate(1, 0, 0)
	if err == nil {
		t.Fatal("SlotNone must be rejected, not treated as Consts[0]")
	}
	if !strings.Contains(err.Error(), "never given a source") {
		t.Errorf("error should name the missed initialisation, got %v", err)
	}
}

func TestRegionDescValidateRejectsNil(t *testing.T) {
	var d *RegionDesc
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("a nil descriptor must be rejected")
	}
}

// A lead and its name must agree in both directions.
func TestRegionDescValidateRejectsLeadWordMismatch(t *testing.T) {
	d := okDesc()
	d.Word = ""
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("LeadWord with no name must be rejected")
	}
	d2 := okDesc()
	d2.Lead = LeadApply
	if err := d2.Validate(1, 0, 0); err == nil {
		t.Error("a non-word lead carrying a word name must be rejected")
	}
}

// The lead's modifiers, when carried, name the lead.
func TestRegionDescValidateRejectsModsOfAnotherWord(t *testing.T) {
	d := okDesc()
	d.Mods = &core.WordInfo{Name: "g", ArgCount: -1, ForceForward: true}
	if err := d.Validate(1, 0, 0); err == nil || !strings.Contains(err.Error(), "lead modifiers name") {
		t.Errorf("modifiers naming another word must be rejected, got %v", err)
	}
	d.Mods.Name = "f"
	if err := d.Validate(1, 0, 0); err != nil {
		t.Errorf("modifiers naming the lead validate, got %v", err)
	}
}

// Bounds are checked against the table the index actually addresses — an
// in-range-but-wrong index is what a sentinel alone cannot catch.
func TestRegionDescValidateRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name                  string
		src                   SlotSource
		idx                   int
		nConsts, nFns, nTypes int
	}{
		{"const past end", SlotConst, 5, 1, 0, 0},
		{"const negative", SlotConst, -1, 1, 0, 0},
		{"fragment past end", SlotGroup, 3, 1, 1, 0},
		{"fragment negative", SlotGroup, -2, 1, 1, 0},
		{"local negative", SlotLocal, -1, 1, 0, 0},
		{"event negative", SlotEvent, -1, 1, 0, 0},
		// SlotType, added on the corpus census (opType is 3840 of 51419
		// forward-collected operands), gets the same bounds treatment.
		{"type past end", SlotType, 2, 1, 0, 1},
		{"type negative", SlotType, -1, 1, 0, 1},
	}
	for _, c := range cases {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: c.src, Idx: c.idx}
		if err := d.Validate(c.nConsts, c.nFns, c.nTypes); err == nil {
			t.Errorf("%s: must be rejected", c.name)
		}
	}
}

// A local or event index is only sign-checked here: the lowerer owns the
// upper bound, since it knows the unit's own local count.
func TestRegionDescValidateAcceptsInRangeLocalAndEvent(t *testing.T) {
	for _, src := range []SlotSource{SlotLocal, SlotEvent} {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: src, Idx: 99}
		if err := d.Validate(0, 0, 0); err != nil {
			t.Errorf("source %v index 99 should pass the sign check, got %v", src, err)
		}
	}
}

func TestRegionDescValidateRejectsUnknownSource(t *testing.T) {
	d := okDesc()
	d.Slots[0] = SlotDesc{Source: SlotSource(200)}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("an unknown source must be rejected")
	}
}

// A captured descriptor is malformed until the lowerer fills Source in, for
// every slot whose source the lowerer actually owns — which is every kind
// but a WORD (finished at capture as SlotWordRef, since a word is resolved
// live and there is nothing to add). The subject here is an integer, so the
// two halves of the contract are still pinned against each other; the word
// case is pinned by TestCaptureRegionSlotsFinishesAWordSlot.
func TestCapturedRegionIsMalformedUntilLowered(t *testing.T) {
	w := core.NewTape([]core.Value{core.NewInteger(1)}, core.StackHeadroom)
	slots := CaptureRegionSlots(w, 0)
	// The claim covers the slot, which is what makes the missing Source a
	// defect: Phase B said this slot WAS the dispatch's operand.
	d := &RegionDesc{Lead: LeadWord, Word: "f", NFwd: len(slots), Slots: slots}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("a freshly captured descriptor must not validate: the lowerer has not set Source")
	}
}

// The one place the invalid zero is not a defect. A slot beyond the recorded
// claim is inside the region's syntactic span but was not this dispatch's
// operand, so no source was ever owed — the runtime defers if a live
// collection reaches it. The negative half is in the same test, because a
// relaxation with no boundary is just a hole: an INDEX out there is still a
// lowerer writing past its own claim, and the claim bound itself is checked.
func TestValidateBeyondTheClaim(t *testing.T) {
	d := okDesc()
	d.Slots = append(d.Slots, SlotDesc{Source: SlotNone})
	if err := d.Validate(1, 0, 0); err != nil {
		t.Errorf("SlotNone beyond the claim must validate, got %v", err)
	}

	d2 := okDesc()
	d2.Slots = append(d2.Slots, SlotDesc{Source: SlotNone, Idx: 3})
	err := d2.Validate(1, 0, 0)
	if err == nil {
		t.Fatal("an unsourced slot carrying an index must be rejected even beyond the claim")
	}
	if !strings.Contains(err.Error(), "beyond the claim") {
		t.Errorf("error should name the claim, got %v", err)
	}

	for _, n := range []int{-1, 2} {
		d3 := okDesc()
		d3.NFwd = n
		if err := d3.Validate(1, 0, 0); err == nil {
			t.Errorf("claim bound %d outside the descriptor's 1 slot must be rejected", n)
		}
	}
}

// TestValidateResultIndex pins the ResIdx rule added with the multi-result
// slot. ResIdx names a result WITHIN a producing event, so a non-zero value
// on any other source is a lowerer defect — and it is exactly the class
// SlotNone exists to stop, one field over: whatever consumes the slot would
// read it as "result N" and silently take the wrong value.
//
// The corpus measurement is why the field exists at all: 33 of 51419
// forward-collected operands are multi-result, so declining them was a real
// option and the field is the alternative that survives Stage 9's totality
// goal.
func TestValidateResultIndex(t *testing.T) {
	t.Run("a multi-result event slot is well-formed", func(t *testing.T) {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: SlotEvent, Idx: 0, ResIdx: 1}
		if err := d.Validate(1, 0, 0); err != nil {
			t.Fatalf("result index 1 on an event must be accepted: %v", err)
		}
	})
	t.Run("a negative result index is rejected", func(t *testing.T) {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: SlotEvent, Idx: 0, ResIdx: -1}
		if err := d.Validate(1, 0, 0); err == nil {
			t.Fatal("a negative result index must be rejected")
		}
	})
	t.Run("a result index on a non-event source is rejected", func(t *testing.T) {
		for _, src := range []SlotSource{SlotConst, SlotLocal, SlotType, SlotGroup} {
			d := okDesc()
			d.Slots[0] = SlotDesc{Source: src, Idx: 0, ResIdx: 2}
			if err := d.Validate(1, 1, 1); err == nil {
				t.Errorf("source %d must not carry a result index", src)
			}
		}
	})
	t.Run("zero is correct for a single-result event", func(t *testing.T) {
		d := okDesc()
		d.Slots[0] = SlotDesc{Source: SlotEvent, Idx: 3, ResIdx: 0}
		if err := d.Validate(1, 0, 0); err != nil {
			t.Fatalf("ResIdx 0 is the ordinary single-result case: %v", err)
		}
	})
}
