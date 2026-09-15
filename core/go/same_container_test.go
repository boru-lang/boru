package core

import "testing"

// SameContainer is the identity half of ExactEqual, reachable for a
// REFINED container (NUR142: ExactEqual's family fold does not recognise
// one, so `w eq w` is false there). One object with one tag answers true;
// a clone, a different tag, or a non-container answers false.
func TestSameContainerIsIdentityWithTag(t *testing.T) {
	m := NewFlexMap(NewOrderedMap())
	if !SameContainer(m, m) {
		t.Error("a flex map is the same container as itself")
	}
	clone := NewFlexMap(NewOrderedMap())
	if SameContainer(m, clone) {
		t.Error("a distinct store with equal contents is not the same container")
	}
	// A refined tag over the SAME store: identity holds where ExactEqual
	// falls through — the reason the helper exists.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	sub := r.Types.MintType("S", TFlexMap)
	refined := ReparentValue(m, sub)
	if !SameContainer(refined, refined) {
		t.Error("a refined flex map is the same container as itself")
	}
	if ExactEqual(refined, refined) {
		t.Error("NUR142 still stands: ExactEqual does not reach a refined container's identity — retire this assertion with the fix")
	}
	if SameContainer(m, refined) {
		t.Error("the same store under two tags is two values")
	}
	l := NewFlexList([]Value{NewInteger(1)})
	if !SameContainer(l, l) || SameContainer(l, NewFlexList([]Value{NewInteger(1)})) {
		t.Error("a flex list identifies by its store pointer")
	}
	lit := NewList([]Value{NewInteger(2)})
	if !SameContainer(lit, lit) || SameContainer(lit, NewList([]Value{NewInteger(2)})) {
		t.Error("an immutable list identifies by its backing array")
	}
	if SameContainer(NewInteger(1), NewInteger(1)) {
		t.Error("a scalar has no container identity")
	}
	nilParent := Value{Data: MapPayload{M: NewOrderedMap()}}
	if SameContainer(nilParent, nilParent) {
		t.Error("a value with no tag has no container identity to compare")
	}
}
