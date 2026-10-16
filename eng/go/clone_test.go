package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// CloneValue must produce a deep, independent copy: mutating the clone
// (or the original) must never be observable through the other.

func TestCloneMapIndependence(t *testing.T) {
	inner := core.NewOrderedMap()
	inner.Set("x", core.NewInteger(1))
	outer := core.NewOrderedMap()
	outer.Set("inner", core.NewMap(inner))
	orig := core.NewMap(outer)

	clone := core.CloneValue(orig)

	// The clone's nested map must be a distinct *OrderedMap.
	cloneInnerV, _ := clone.Data().(core.MapPayload).M.Get("inner")
	cloneInner := cloneInnerV.Data().(core.MapPayload).M
	if cloneInner == inner {
		t.Fatal("clone shares the original's nested *OrderedMap")
	}

	// Mutating the original's nested map must not touch the clone.
	inner.Set("x", core.NewInteger(999))
	if v, _ := cloneInner.Get("x"); func() int64 { n, _ := core.AsInteger(v); return n }() != 1 {
		t.Error("original mutation leaked into the clone's nested map")
	}
}

func TestCloneStoreIndependence(t *testing.T) {
	si := &core.StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]core.Value{}}
	si.Set("k", core.NewInteger(1))
	orig := core.NewStoreValue(core.TStore, si)

	clone := core.CloneValue(orig)
	cs, _ := core.AsStore(clone)
	cs.Set("k", core.NewInteger(99))

	if v, _ := si.Get("k"); func() int64 { n, _ := core.AsInteger(v); return n }() != 1 {
		t.Error("clone store mutation leaked into the original")
	}
}

// A self-referential store must clone without infinite recursion, and
// the clone's cycle must point at the clone (not the original).
func TestCloneStoreCycle(t *testing.T) {
	si := &core.StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]core.Value{}}
	si.Set("self", core.NewStoreValue(core.TStore, si)) // cycle

	// If the cycle map were absent this would recurse forever and the
	// test would time out — the failure signal we want.
	clone := core.CloneValue(core.NewStoreValue(core.TStore, si))

	cs, _ := core.AsStore(clone)
	selfV, ok := cs.Data["self"]
	if !ok {
		t.Fatal("cycle key lost in clone")
	}
	selfStore, _ := core.AsStore(selfV)
	if selfStore != cs {
		t.Error("cloned cycle points at the original (or a second copy), not the clone")
	}
}

func TestCloneListSharedSubstructurePreserved(t *testing.T) {
	// A FlexList referenced twice in a list must clone to ONE shared
	// clone (cycle map), not two independent copies.
	shared := core.NewFlexList([]core.Value{core.NewInteger(1)})
	orig := core.NewList([]core.Value{shared, shared})

	clone := core.CloneValue(orig)
	cl, _ := core.AsList(clone)
	a := cl.Get(0)
	b := cl.Get(1)
	fa, _ := a.Data().(*core.FlexListData)
	fb, _ := b.Data().(*core.FlexListData)
	if fa == nil || fb == nil {
		t.Fatal("flexlist payloads missing after clone")
	}
	if fa != fb {
		t.Error("shared substructure was duplicated; the clone broke aliasing")
	}
}

func TestClonePreservesRefineType(t *testing.T) {
	pos := core.MintTestType("Scalar/Number/Integer/Pos")
	v := core.NewInteger(5)
	v.SetParent(pos) // a refine-subtyped integer
	clone := core.CloneValue(v)
	if !clone.Parent().Equal(pos) {
		t.Errorf("clone Parent = %v, want the Pos refine type", clone.Parent())
	}
}

func TestCloneSharesImmutableScalars(t *testing.T) {
	// Scalars carry value payloads; a clone is equal and has the same
	// payload (sharing is fine — nothing mutates a scalar in place).
	for _, v := range []core.Value{core.NewInteger(7), core.NewString("hi"), core.NewBoolean(true), core.NewFloat(1.5)} {
		c := core.CloneValue(v)
		if !core.ExactEqual(c, v) {
			t.Errorf("clone of scalar %v not equal to original", v)
		}
	}
}

func TestCloneTypeLiteralNoPanic(t *testing.T) {
	// A bare type literal (Data == nil) must clone without panicking.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("clone of type literal panicked: %v", r)
		}
	}()
	orig := core.NewTypeLiteral(core.TMap)
	c := core.CloneValue(orig)
	// A type literal IS its lattice node and has no payload; the clone is
	// the same node (shared, immutable).
	if core.IDOf(&c) != core.IDOf(&orig) || !core.ValueType(c).Equal(core.ValueType(orig)) {
		t.Error("type-literal clone is not the same lattice node")
	}
}

// fakeBody exercises the DeepCloner capability on an ExtensionPayload.
type fakeBody struct{ data []int }

func (f fakeBody) DeepClone() any {
	cp := make([]int, len(f.data))
	copy(cp, f.data)
	return fakeBody{data: cp}
}

func TestCloneExtensionDeepCloner(t *testing.T) {
	ft := core.MintTestType("Ideal/Fake")
	orig := core.NewExtension(ft, fakeBody{data: []int{1, 2, 3}})
	clone := core.CloneValue(orig)

	ob := orig.Data().(core.ExtensionPayload).Body.(fakeBody)
	cb := clone.Data().(core.ExtensionPayload).Body.(fakeBody)
	cb.data[0] = 99
	if ob.data[0] != 1 {
		t.Error("DeepCloner clone shares the original body slice")
	}
}
