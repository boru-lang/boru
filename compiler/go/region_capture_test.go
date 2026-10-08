package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func capTape(vals []core.Value) *core.Tape {
	return core.NewTape(vals, core.StackHeadroom)
}

// Written order is preserved and the extent stops at the delimiter.
func TestCaptureRegionSlotsWrittenOrder(t *testing.T) {
	w := capTape([]core.Value{
		core.NewWord("f"), core.NewInteger(1), core.NewInteger(2),
		core.NewEnd(), core.NewInteger(9),
	})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 3 {
		t.Fatalf("captured %d slots, want 3 (stops at `end`)", len(got))
	}
	if !core.IsWord(got[0].Token) {
		t.Errorf("slot 0 should be the word token, got %v", got[0].Token)
	}
	if v, _ := core.AsInteger(got[2].Token); v != 2 {
		t.Errorf("slot 2 token = %v, want the integer 2 (written order)", got[2].Token)
	}
}

// An empty region returns nil, so "no region" and "zero slots" are
// distinguishable without a second call.
func TestCaptureRegionSlotsEmptyIsNil(t *testing.T) {
	w := capTape([]core.Value{core.NewEnd(), core.NewInteger(1)})
	if got := CaptureRegionSlots(w, 0); got != nil {
		t.Errorf("empty region = %v, want nil", got)
	}
	if got := CaptureRegionSlots(nil, 0); got != nil {
		t.Errorf("nil window = %v, want nil", got)
	}
}

// A negative start clamps rather than reading out of range.
func TestCaptureRegionSlotsClampsNegative(t *testing.T) {
	w := capTape([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if got := CaptureRegionSlots(w, -4); len(got) != 2 {
		t.Errorf("captured %d slots from a negative start, want 2", len(got))
	}
}

// A nested group is captured whole — the region contains it.
func TestCaptureRegionSlotsContainsGroup(t *testing.T) {
	w := capTape([]core.Value{
		core.NewWord("f"), core.NewOpenParen(), core.NewInteger(1),
		core.NewCloseParen(), core.NewEnd(),
	})
	if got := CaptureRegionSlots(w, 0); len(got) != 4 {
		t.Errorf("captured %d slots, want 4 (the group is contained)", len(got))
	}
}

// The static modifier is read off the token. It is the one half of a slot's
// description that is record-time-final: no binding can move it.
func TestCaptureRegionSlotsReadsQuote(t *testing.T) {
	w := capTape([]core.Value{
		core.NewInteger(1), core.NewAtom("nm"), core.NewDispatchMod(core.DispatchModInfo{Val: true}),
	})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 3 {
		t.Fatalf("captured %d slots, want 3", len(got))
	}
	if got[0].Quote != QuoteNone {
		t.Errorf("plain value quote = %v, want QuoteNone", got[0].Quote)
	}
	if got[1].Quote != QuoteAtom {
		t.Errorf("atom quote = %v, want QuoteAtom (a /q slot is already an Atom)", got[1].Quote)
	}
	if got[2].Quote != QuoteValue {
		t.Errorf("/v mod quote = %v, want QuoteValue", got[2].Quote)
	}
	// …and /q on the same marker type is a DIFFERENT fact, not the same one.
	w2 := capTape([]core.Value{core.NewDispatchMod(core.DispatchModInfo{Quote: true})})
	if g2 := CaptureRegionSlots(w2, 0); len(g2) != 1 || g2[0].Quote != QuoteData {
		t.Errorf("/q mod quote = %v, want QuoteData", g2[0].Quote)
	}
}

// A NON-WORD slot's Source is left at its zero for the lowerer to fill: it
// is not knowable from the token, and guessing it here would be the
// frozen-class mistake in a different place. (A word IS knowable — see
// TestCaptureRegionSlotsFinishesAWordSlot — because "resolve it live" is a
// complete answer rather than a guess.)
func TestCaptureRegionSlotsLeavesSourceToLowerer(t *testing.T) {
	w := capTape([]core.Value{core.NewInteger(1)})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 1 || got[0].Source != SlotNone {
		t.Errorf("Source = %v, want the INVALID SlotNone zero — a capture must not "+
			"look like a reference to Consts[0]", got[0].Source)
	}
}

// A WORD slot is FINISHED at capture, and every other kind is not. This is
// the pair that keeps the split honest: it fails if capture stops finishing
// words (the largest slot class in the corpus, 45.6% of region tokens), and
// it fails just as loudly if capture starts guessing a source for a token
// whose source the lowerer owns.
//
// The word case is not an optimisation. A word's binding is resolved LIVE on
// every execution — `k` is a value slot or a collection barrier depending on
// what it is bound to NOW — so a later pass has nothing to learn and nothing
// to freeze. See SlotWordRef.
func TestCaptureRegionSlotsFinishesAWordSlot(t *testing.T) {
	w := capTape([]core.Value{
		core.NewWord("f"), core.NewWord("k"), core.NewInteger(1),
	})
	got := CaptureRegionSlots(w, 0)
	if len(got) != 3 {
		t.Fatalf("captured %d slots, want 3", len(got))
	}
	for _, i := range []int{0, 1} {
		if got[i].Source != SlotWordRef {
			t.Errorf("slot %d source = %v, want SlotWordRef", i, got[i].Source)
		}
		if got[i].Idx != 0 {
			t.Errorf("slot %d idx = %d, want 0 — a wordRef addresses no table", i, got[i].Idx)
		}
	}
	if got[2].Source != SlotNone {
		t.Errorf("the integer slot = %v, want SlotNone: its source is the lowerer's", got[2].Source)
	}
}

// A region of nothing but words validates STRAIGHT OUT OF CAPTURE, with no
// lowering at all. That is the concrete payoff of SlotWordRef and the reason
// it is worth a member: before it, a word slot had no source of its own, so
// the only way to give one was to match the slot against the dispatch's
// already-resolved operands — the written-order to sig-order join that three
// reverted attempts were built around.
func TestAllWordRegionValidatesUnlowered(t *testing.T) {
	w := capTape([]core.Value{core.NewWord("g"), core.NewWord("x"), core.NewWord("y")})
	d := &RegionDesc{Lead: LeadWord, Word: "f", Pos: core.SrcPos{Row: 1, Col: 1},
		Slots: CaptureRegionSlots(w, 0)}
	if err := d.Validate(0, 0, 0); err != nil {
		t.Errorf("an all-word region must validate unlowered: %v", err)
	}
}

// The negative control for the rule above: a wordRef that acquired an index
// is declined. Without this the Idx==0 requirement is a comment, and whatever
// read the index would be addressing a table this source never meant.
func TestValidateRejectsIndexedWordRef(t *testing.T) {
	d := &RegionDesc{Lead: LeadWord, Word: "f", Pos: core.SrcPos{Row: 1, Col: 1},
		Slots: []SlotDesc{{Source: SlotWordRef, Idx: 3, Token: core.NewWord("x")}}}
	if err := d.Validate(0, 0, 0); err == nil {
		t.Error("a wordRef carrying an index must be declined")
	}
}
