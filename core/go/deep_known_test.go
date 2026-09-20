package core

import "testing"

// DeepKnown is DeepConcrete widened to admit a bare type node — the one
// payload-less operand whose runtime value analysis provably holds. These
// tests pin both directions, since the widening is what lets a mirror
// decide `{value: None}` and the compile failures are what keep it honest.

func TestDeepKnownAdmitsBareTypeNodes(t *testing.T) {
	// A bare type node is inert and self-evaluating: the checked node IS
	// the runtime operand. DeepConcrete declines it (no payload); DeepKnown
	// admits it, at the top level and nested.
	none := NewTypeLiteral(TNone)
	if DeepConcrete(none) {
		t.Fatal("fixture: a bare type node must not be DeepConcrete")
	}
	if !DeepKnown(none) {
		t.Fatal("a bare type node must be DeepKnown")
	}

	m := NewOrderedMap()
	m.Set("alias", NewString("a"))
	m.Set("value", none)
	nested := NewMap(m)
	if DeepConcrete(nested) {
		t.Fatal("fixture: a map holding a type node must not be DeepConcrete")
	}
	if !DeepKnown(nested) {
		t.Fatal("a map holding a type node must be DeepKnown")
	}

	if !DeepKnown(NewList([]Value{NewInteger(1), none})) {
		t.Fatal("a list holding a type node must be DeepKnown")
	}
}

func TestDeepKnownDoesNotLowerUnknownOperands(t *testing.T) {
	// A carrier is the shape whose runtime value analysis does NOT hold,
	// at the top level and at every nested position.
	if DeepKnown(NewCarrier(TString)) {
		t.Fatal("a carrier must not be DeepKnown")
	}
	m := NewOrderedMap()
	m.Set("value", NewCarrier(TString))
	if DeepKnown(NewMap(m)) {
		t.Fatal("a map holding a carrier must not be DeepKnown")
	}
	if DeepKnown(NewList([]Value{NewCarrier(TInteger)})) {
		t.Fatal("a list holding a carrier must not be DeepKnown")
	}

	// A DYNAMIC operand was matched optimistically — declined even though
	// it carries a payload, and declined nested too.
	dyn := NewString("x")
	dyn.Dynamic = true
	if DeepKnown(dyn) {
		t.Fatal("a dynamic value must not be DeepKnown")
	}
	dm := NewOrderedMap()
	dm.Set("value", dyn)
	if DeepKnown(NewMap(dm)) {
		t.Fatal("a map holding a dynamic value must not be DeepKnown")
	}

	// A nil-backed map payload has no interior to walk.
	if DeepKnown(Value{Parent: TMap, Data: MapPayload{}}) {
		t.Fatal("a nil-backed map payload must not be DeepKnown")
	}
}

func TestDeepKnownDepthCap(t *testing.T) {
	// Past the cap the honest answer is "cannot prove it" — the same safe
	// direction DeepConcrete takes, and what keeps a self-referential flex
	// container from walking forever.
	v := NewInteger(1)
	for i := 0; i <= deepConcreteMaxDepth+1; i++ {
		v = NewList([]Value{v})
	}
	if DeepKnown(v) {
		t.Fatal("a structure past the depth cap must not be DeepKnown")
	}
}
