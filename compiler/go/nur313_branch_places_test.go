package compiler

import (
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
