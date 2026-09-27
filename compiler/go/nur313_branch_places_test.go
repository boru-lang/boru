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

// TestBranchLeadDecline pins the apply arms' NUR313 decline: a split branch
// whose body arm may leave a fn declines, and a lead with no producer or a
// branch whose every arm places (placed data, not an apply) does not.
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
	if why := es.branchLeadDecline(lead); why != "" {
		t.Errorf("a placed branch is data, no apply to decline: %q", why)
	}
	es.frames[0][0].br = &emitBranch{then: body, hasThenOut: true, elsIsVal: true, hasElsOut: true, thenOut: EventOperand(2, 0)}
	if why := es.branchLeadDecline(lead); !strings.Contains(why, "whose value arm is re-stepped") {
		t.Errorf("a split branch's body arm: %q", why)
	}
	es.NoteDelivery(lead)
	es.NoteDelivery(lead)
	es.NoteDelivery(core.Value{})
	if es.deliveries[lead.ID] != 2 {
		t.Errorf("deliveries: %d", es.deliveries[lead.ID])
	}
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

// TestNUR318QuotedFnCarrier pins NUR318's operand test: a carrier a `/v`
// marker quoted that may hold a fn is a fn value at a re-stepping word, and
// an unquoted or data carrier is not.
func TestNUR318QuotedFnCarrier(t *testing.T) {
	q := core.NewDynamicCarrier(core.TAny)
	q.Quoted = true
	fq := core.NewCarrier(core.TFunction)
	fq.Quoted = true
	iq := core.NewCarrier(core.TInteger)
	iq.Quoted = true
	for _, c := range []struct {
		why  string
		v    core.Value
		want bool
	}{
		{"a quoted gradual carrier", q, true},
		{"a quoted fn carrier", fq, true},
		{"an unquoted gradual carrier", core.NewDynamicCarrier(core.TAny), false},
		{"a quoted data carrier", iq, false},
	} {
		if got := quotedFnCarrier(c.v); got != c.want {
			t.Errorf("%s: quotedFnCarrier = %v, want %v", c.why, got, c.want)
		}
	}
}

// TestNUR319UnionReadReStepped pins NUR319: a bare read of a def bound to a
// placed branch's union is its word's dispatch, a re-step that reaches it —
// the lead's conditional apply and the interior gate's unsettled value — and
// a `/v` read of it is data.
func TestNUR319UnionReadReStepped(t *testing.T) {
	es := NewEmitState()
	body := &EmitFragment{residualN: 1}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, els: body, hasThenOut: true, hasElsOut: true,
	}})
	u := core.NewDisjunct([]core.Value{core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TFunction)})
	u.Carrier, u.ID = true, "u"
	es.producedBy[u.ID] = producer{seq: 7}
	if es.unionLeadReStepped(u, 7) || es.mayBeFnUnsettled(u) {
		t.Error("an unread placed union is no re-step")
	}
	es.NoteDefRead(u.ID, "r")
	if !es.unionLeadReStepped(u, 7) || !es.mayBeFnUnsettled(u) {
		t.Error("a bare read of the def dispatches it")
	}
	if es.unionLeadReStepped(u, 8) {
		t.Error("no placing branch at the seq: no re-step")
	}
}

// TestNUR317PlacedBranchNotReStepped pins the re-step note's branch skip: a
// branch whose body arms place its value never re-steps it where the `if`
// stood, and a call's result keeps its note.
func TestNUR317PlacedBranchNotReStepped(t *testing.T) {
	es := NewEmitState()
	body := &EmitFragment{residualN: 1}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 7, kind: evBranch, br: &emitBranch{
		then: body, els: body, hasThenOut: true, hasElsOut: true,
	}})
	v := core.NewDynamicCarrier(core.TAny)
	v.ID = "v"
	es.producedBy[v.ID] = producer{seq: 7}
	es.NoteFnResultReStep(v, core.SrcPos{})
	if len(es.reStepNotes) != 0 {
		t.Errorf("a placed branch's value takes no re-step note: %v", es.reStepNotes)
	}
	w := core.NewDynamicCarrier(core.TAny)
	w.ID = "w"
	es.producedBy[w.ID] = producer{seq: 9}
	es.NoteFnResultReStep(w, core.SrcPos{})
	if _, ok := es.reStepNotes[9]; !ok {
		t.Error("a call's result keeps its re-step note")
	}
}
