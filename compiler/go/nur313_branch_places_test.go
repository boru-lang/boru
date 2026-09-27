package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestBranchPlaces pins NUR313's placement rule over the branch shapes: a
// branch is placed when every arm that yields its value is a body netting
// one (the arm's paren parks it), and split when one such body meets a value
// arm (the value arm's path alone lands).
func TestBranchPlaces(t *testing.T) {
	one, two := &EmitFragment{residualN: 1}, &EmitFragment{residualN: 2}
	taken := true
	for _, c := range []struct {
		why           string
		br            *emitBranch
		places, split bool
	}{
		{"no branch", nil, false, false},
		{"two one-value bodies", &emitBranch{then: one, els: one, hasThenOut: true, hasElsOut: true}, true, false},
		{"a body over a diverging arm", &emitBranch{then: one, els: one, hasThenOut: true}, true, false},
		{"a decided branch's taken body", &emitBranch{constCond: &taken, then: one, hasThenOut: true}, true, false},
		{"a two-value body re-steps", &emitBranch{then: two, els: one, hasThenOut: true, hasElsOut: true}, false, false},
		{"a value arm re-steps", &emitBranch{thenIsVal: true, els: one, hasThenOut: true, hasElsOut: true}, false, true},
		{"a value else", &emitBranch{then: one, elsIsVal: true, hasThenOut: true, hasElsOut: true}, false, true},
		{"two value arms", &emitBranch{thenIsVal: true, elsIsVal: true, hasThenOut: true, hasElsOut: true}, false, false},
		{"an uncaptured arm", &emitBranch{elsIsVal: true, hasThenOut: true, hasElsOut: true}, false, false},
		{"no value on either path", &emitBranch{then: one, els: one}, false, false},
	} {
		if got := branchPlaces(c.br); got != c.places {
			t.Errorf("%s: branchPlaces = %v, want %v", c.why, got, c.places)
		}
		if got := splitLanding(c.br); got != c.split {
			t.Errorf("%s: splitLanding = %v, want %v", c.why, got, c.split)
		}
	}
}

// TestSplitArmMayBeFn pins the split branch's lead decline (NUR313): a
// placed body arm whose value is a const that is no fn cannot be applied,
// anything else may be.
func TestSplitArmMayBeFn(t *testing.T) {
	es := NewEmitState()
	if es.splitArmMayBeFn(7) {
		t.Error("no event: nothing to decline")
	}
	body := &EmitFragment{residualN: 1}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, hasThenOut: true, elsIsVal: true, hasElsOut: true, thenOut: ConstOperand(es.intern(core.NewInteger(3))),
	}})
	if es.splitArmMayBeFn(7) {
		t.Error("a data const in the placed arm is no fn")
	}
	es.frames[0][0].br.thenOut = EventOperand(2, 0)
	if !es.splitArmMayBeFn(7) {
		t.Error("an event's value may be a fn")
	}
	es.frames[0][0].br = &emitBranch{thenIsVal: true, hasThenOut: true, els: body, hasElsOut: true,
		elsOut: ConstOperand(es.intern(core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}))}
	if !es.splitArmMayBeFn(7) {
		t.Error("a fn const in the placed else arm is a fn")
	}
	es.frames[0][0].br = &emitBranch{then: body, els: body, hasThenOut: true, hasElsOut: true}
	if es.splitArmMayBeFn(7) {
		t.Error("an unsplit branch declines nothing")
	}
	if es.branchPlacesAt(99) {
		t.Error("no event at the seq places nothing")
	}
}

// TestBranchLeadDecline pins the apply arms' NUR313 declines: a lead its
// placing branch delivered once, and a split branch whose body arm may leave
// a fn, decline; a lead delivered again (an enclosing re-step) or with no
// branch behind it applies.
func TestBranchLeadDecline(t *testing.T) {
	es := NewEmitState()
	lead := core.Value{Parent: core.TAny, ID: "lead"}
	if why := es.branchLeadDecline(lead); why != "" {
		t.Errorf("no producer: %q", why)
	}
	body := &EmitFragment{residualN: 1}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, els: body, hasThenOut: true, hasElsOut: true,
	}})
	es.producedBy[lead.ID] = producer{seq: 7}
	es.NoteDelivery(lead)
	if why := es.branchLeadDecline(lead); !strings.Contains(why, "a branch's placed fn value leads") {
		t.Errorf("a placed lead delivered once: %q", why)
	}
	es.NoteDelivery(lead)
	if why := es.branchLeadDecline(lead); why != "" || es.deliveries[lead.ID] != 2 {
		t.Errorf("a lead delivered again is re-stepped: %q (%d)", why, es.deliveries[lead.ID])
	}
	es.frames[0][0].br = &emitBranch{then: body, hasThenOut: true, elsIsVal: true, hasElsOut: true, thenOut: EventOperand(2, 0)}
	if why := es.branchLeadDecline(lead); !strings.Contains(why, "whose value arm is re-stepped") {
		t.Errorf("a split branch's body arm: %q", why)
	}
	es.NoteDelivery(core.Value{})
	if _, ok := es.deliveries[""]; ok {
		t.Error("a value with no ID is not counted")
	}
}

// TestNUR317PlacementUndone pins NUR317's undoing re-steps: an enclosing
// paren's re-step or a `do` body's caller undoes a branch's placement, and a
// union lead a paren re-stepped is a conditional apply.
func TestNUR317PlacementUndone(t *testing.T) {
	es := NewEmitState()
	body := &EmitFragment{residualN: 1}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, els: body, hasThenOut: true, hasElsOut: true,
	}})
	u := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TFunction)})
	u.Carrier, u.ID = true, "u"
	if !es.branchPlacedHere(u, 7) || es.unionLeadReStepped(u, 7) {
		t.Error("no re-step: the branch places its join")
	}
	es.reg = &core.Registry{Check: &core.CheckState{ParenReSteppedFnIDs: map[string]bool{"u": true}}}
	if es.branchPlacedHere(u, 7) || !es.unionLeadReStepped(u, 7) {
		t.Error("a paren's re-step undoes the placement and applies the union")
	}
	if es.unionLeadReStepped(core.NewCarrier(core.TInteger), 7) {
		t.Error("a lead with no fn alternative is no apply")
	}
	es.reg = nil
	if es.inResidualToCallerUnit() {
		t.Error("no open unit")
	}
	es.fnRecs = append(es.fnRecs, &fnUnitRec{residualToCaller: true})
	es.openUnitRecs = []int{len(es.fnRecs) - 1}
	if !es.inResidualToCallerUnit() || es.branchPlacedHere(u, 7) {
		t.Error("a do body's caller re-steps its residual: nothing is placed")
	}
	es.openUnitRecs = []int{99}
	if es.inResidualToCallerUnit() {
		t.Error("an unknown record is no do body")
	}
}

// TestPlacedArmsMayBeFn pins the lead decline's fn test (NUR313): a placed
// branch whose arms leave data consts is no fn lead, one with an event's
// value or a fn const may be.
func TestPlacedArmsMayBeFn(t *testing.T) {
	es := NewEmitState()
	if es.placedArmsMayBeFn(7) {
		t.Error("no event: no fn")
	}
	body := &EmitFragment{residualN: 1}
	zero, nine := ConstOperand(es.intern(core.NewInteger(0))), ConstOperand(es.intern(core.NewInteger(9)))
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, els: body, hasThenOut: true, hasElsOut: true, thenOut: zero, elsOut: nine,
	}})
	if es.placedArmsMayBeFn(7) {
		t.Error("two data consts leave no fn")
	}
	es.frames[0][0].br.elsOut = EventOperand(2, 0)
	if !es.placedArmsMayBeFn(7) {
		t.Error("an event's value may be a fn")
	}
	es.frames[0][0].br.elsOut = ConstOperand(es.intern(core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}))
	if !es.placedArmsMayBeFn(7) {
		t.Error("a fn const is a fn")
	}
}

// TestDynBodySettledOp pins NUR317's settled-lead op: a dyn body's lead
// under its own sibling re-steps from the mark where the window is armed,
// and seats as it stands where it is not; any other residual is not settled.
func TestDynBodySettledOp(t *testing.T) {
	es := NewEmitState()
	lead, sib := core.NewCarrier(core.TAny), core.NewInteger(5)
	lead.Dynamic, lead.ID, sib.ID = true, "lead", "sib"
	res := []core.Value{lead, sib}
	if _, settled := es.dynBodySettledOp(res); settled {
		t.Error("no producer: not settled")
	}
	es.producedBy[lead.ID], es.producedBy[sib.ID] = producer{seq: 3}, producer{seq: 3, idx: 1}
	es.eventInfo[3] = eventFlags{dynBodyResult: true}
	if op, settled := es.dynBodySettledOp(res); !settled || op != 0 {
		t.Errorf("no mark window: seats as it stands, got %v %v", op, settled)
	}
	es.markWindowSeq = 3
	if op, settled := es.dynBodySettledOp(res); !settled || op != OpCallDynMixedFromMark {
		t.Errorf("an armed window re-steps from the mark, got %v %v", op, settled)
	}
}
