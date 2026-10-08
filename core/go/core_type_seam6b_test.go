package core

// Seam-6b wave B0: guard arms in core_type.go (IsValueOfType edges,
// validateTypeName clashes, InstallType refine/DepScalar edges),
// core_boundedtype.go (bounded-Type unification and behavior
// delegation), and core_boolean.go (union edge cases, Never shapes).

import (
	"strings"
	"testing"
)

// --- IsValueOfType -------------------------------------------------------------

func TestS6b0IsValueOfTypeTypedListEdges(t *testing.T) {
	typedList := NewTypedListWithElements(NewTypeLiteral(TInteger), []Value{NewInteger(1)})
	if IsValueOfType(NewInteger(1), typedList) {
		t.Error("a non-list value must not match a typed list")
	}
	if IsValueOfType(NewCarrier(TList), typedList) {
		t.Error("a list carrier must not match a typed list")
	}
	if IsValueOfType(NewList(nil), typedList) {
		t.Error("a nil-backed list must not match a typed list")
	}
	if !IsValueOfType(NewList([]Value{NewInteger(2)}), typedList) {
		t.Error("a conforming concrete list must match")
	}
}

func TestS6b0IsValueOfTypeTypedMapEdges(t *testing.T) {
	typedMap := NewTypedMap(NewTypeLiteral(TInteger))
	if IsValueOfType(NewInteger(1), typedMap) {
		t.Error("a non-map value must not match a typed map")
	}
	// Parent is exactly Map and concrete, but the payload is a record
	// type body: AsMap declines, so the value cannot match.
	if IsValueOfType(NewRecordType(NewOrderedMap()), typedMap) {
		t.Error("a record type body must not match a typed map")
	}
}

func TestS6b0IsValueOfTypeMapAsTypeUnreadableValue(t *testing.T) {
	tm := NewOrderedMap()
	tm.Set("a", NewTypeLiteral(TInteger))
	shape := NewMap(tm)
	if IsValueOfType(NewRecordType(NewOrderedMap()), shape) {
		t.Error("a record type body must not match a map shape")
	}
}

func TestS6b0IsValueOfTypeCarrierIsNotAType(t *testing.T) {
	if IsValueOfType(NewCarrier(TInteger), NewTypeLiteral(TType)) {
		t.Error("a carrier is an abstract value, not a type")
	}
}

// --- validateTypeName clashes ----------------------------------------------------

func TestS6b0InstallTypeNameClashes(t *testing.T) {
	r := newTestRegistry(t)
	// Bind an FnDefInfo function under a capitalised name directly (native
	// registration would reject the capital) so validateTypeName's
	// r.Lookup clash arm — "already a registered function" — is reachable.
	r.Defs.Push("S6b0Fn", NewFunction(FnDefInfo{Name: "S6b0Fn"}))
	err := InstallType(r, "S6b0Fn", NewTypeLiteral(TInteger))
	if err == nil || !strings.Contains(err.Error(), "already a registered function") {
		t.Errorf("function clash: %v", err)
	}

	r.Defs.Push("S6b0Val", NewInteger(1))
	err = InstallType(r, "S6b0Val", NewTypeLiteral(TInteger))
	if err == nil || !strings.Contains(err.Error(), "already a def'd value") {
		t.Errorf("value clash: %v", err)
	}
}

// --- InstallType refine / DepScalar edges -----------------------------------------

func TestS6b0InstallTypeForeignRefinePrefab(t *testing.T) {
	r := newTestRegistry(t)
	other := newTestRegistry(t)
	prefab := other.Types.MintRefinePrefab(TInteger)
	err := InstallType(r, "S6b0Lost", NewTypeLiteral(prefab))
	if err == nil || !strings.Contains(err.Error(), "refine prefab missing from lattice") {
		t.Fatalf("expected missing-prefab error, got %v", err)
	}
}

// --- core_boundedtype.go -----------------------------------------------------------

func TestS6b0AsBoundedTypeUndenotingChild(t *testing.T) {
	// A child that neither is a bare literal nor carries a Parent:
	// the bound denotes no lattice node.
	v := NewValueRaw(TType, ChildTypeInfo{Child: Value{Parent: nil, Data: IntPayload{N: 1}}})
	if _, err := AsBoundedType(v); err == nil ||
		!strings.Contains(err.Error(), "does not denote a lattice node") {
		t.Fatalf("expected no-node error, got %v", err)
	}
}

func TestS6b0BoundedTypeSatisfiedViaIs(t *testing.T) {
	// A disjunct body denotes no single node, but Is(TDisjunct) holds.
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger)})
	if !boundedTypeSatisfied(d, NewTypeLiteral(TDisjunct), nil) {
		t.Error("a disjunct body must satisfy Type of [Disjunct] via Is")
	}
}

func TestS6b0UnifyBoundedTypePairs(t *testing.T) {
	// Bounded vs bounded: the narrower bound wins.
	a := NewBoundedType(TInteger)
	b := NewBoundedType(TNumber)
	got, err := unifyBoundedType(a, b, nil)
	if err != nil {
		t.Fatalf("bounded-vs-bounded: %v", err)
	}
	if n, e := AsBoundedType(got); e != nil || !n.Equal(TInteger) {
		t.Errorf("narrower bound must win, got %v", got)
	}
	// Value vs bounded (bok case): the conforming literal wins.
	got, err = unifyBoundedType(NewTypeLiteral(TInteger), b, nil)
	if err != nil {
		t.Fatalf("literal-vs-bounded: %v", err)
	}
	if !IsBareTypeNode(got) || !(&got).Equal(TInteger) {
		t.Errorf("the conforming literal must survive, got %v", got)
	}
}

func TestS6b0TypeMembershipBehaviorDelegates(t *testing.T) {
	b := typeMembershipBehavior{}
	if !b.Match(NewInteger(1), TInteger) {
		t.Error("non-Type target must delegate to the default lattice walk")
	}
	if got := b.Format(NewInteger(3)); got != "3" {
		t.Errorf("Format = %q, want 3", got)
	}
	b.formatDelegate() // rendering-neutral marker
}

// --- core_boolean.go -----------------------------------------------------------------

func TestS6b0UnionTypeEmptyIsNever(t *testing.T) {
	got := UnionType(NewDisjunct(nil), NewDisjunct(nil))
	if !IsBareTypeNode(got) || !(&got).Equal(TNever) {
		t.Errorf("empty union = %v, want Never", got)
	}
}

func TestS6b0IsNeverShapeSentinel(t *testing.T) {
	sentinel := Value{Parent: TNever, Data: NonePayload{}}
	if !IsNeverShape(sentinel) {
		t.Error("a Parent=Never sentinel is a Never shape")
	}
	if !IsNeverShape(NewTypeLiteral(TNever)) {
		t.Error("the bare Never literal is a Never shape")
	}
	if IsNeverShape(NewInteger(1)) {
		t.Error("an integer is not a Never shape")
	}
}
