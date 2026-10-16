package core

import "testing"

// valuesEqualDefault over check-mode store-shaped carriers: ONE shape is
// minted per container and every carrier copy aliases it (store_shape.go),
// so equality is shape-pointer identity. Before the fix the Map branch fell
// through to AsMap (which cannot read a *StoreShapeInfo payload), ignored
// the error, and walked a nil ReadMap — the boru-check nil-deref panic over
// any module using `flex {}` locals in narrowed branches.
func TestValuesEqualStoreShapedCarriers(t *testing.T) {
	shapedA := NewStoreShapeCarrier(TFlexMap, 0)
	aliasA := shapedA // a copy of the Value aliases the same *StoreShapeInfo
	shapedB := NewStoreShapeCarrier(TFlexMap, 0)

	if !ValuesEqual(shapedA, aliasA) {
		t.Error("two carrier copies of one shape must be equal (same pointer)")
	}
	if ValuesEqual(shapedA, shapedB) {
		t.Error("distinct shapes must not be equal (distinct containers)")
	}

	// NEGATIVE: shaped vs a CONCRETE flex map — payload kinds differ.
	om := NewOrderedMap()
	concrete := NewValueRaw(TFlexMap, MapPayload{M: om})
	if ValuesEqual(shapedA, concrete) || ValuesEqual(concrete, shapedA) {
		t.Error("a shaped carrier must not equal a concrete flex map")
	}

	// List-family twin (the same guard sits in the List branch).
	shapedL1 := NewStoreShapeCarrier(TFlexList, 0)
	aliasL1 := shapedL1
	shapedL2 := NewStoreShapeCarrier(TFlexList, 0)
	if !ValuesEqual(shapedL1, aliasL1) {
		t.Error("list-family: carrier copies of one shape must be equal")
	}
	if ValuesEqual(shapedL1, shapedL2) {
		t.Error("list-family: distinct shapes must not be equal")
	}
}

// The AsMap/AsList-failure fallback: a node-family value whose payload is
// not container-readable (an abstract or foreign marker payload) must fall
// to the rendering compare — never walk a nil ReadMap. Positive (same
// rendering) and negative (different rendering) pinned for both families.
func TestValuesEqualUnreadableNodePayloads(t *testing.T) {
	mapA := Value{d: &Datum{Parent: TMap, Data: WordInfo{Name: "w1"}}}
	mapA2 := Value{d: &Datum{Parent: TMap, Data: WordInfo{Name: "w1"}}}
	mapB := Value{d: &Datum{Parent: TMap, Data: WordInfo{Name: "w2"}}}
	if !valuesEqualDefault(mapA, mapA2) {
		t.Error("map-family unreadable payloads with equal rendering must compare equal")
	}
	if valuesEqualDefault(mapA, mapB) {
		t.Error("map-family unreadable payloads with different rendering must compare unequal")
	}

	lstA := Value{d: &Datum{Parent: TList, Data: WordInfo{Name: "w1"}}}
	lstA2 := Value{d: &Datum{Parent: TList, Data: WordInfo{Name: "w1"}}}
	lstB := Value{d: &Datum{Parent: TList, Data: WordInfo{Name: "w2"}}}
	if !valuesEqualDefault(lstA, lstA2) {
		t.Error("list-family unreadable payloads with equal rendering must compare equal")
	}
	if valuesEqualDefault(lstA, lstB) {
		t.Error("list-family unreadable payloads with different rendering must compare unequal")
	}
}
