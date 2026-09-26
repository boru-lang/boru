package check

// Unit coverage for the FlexList element-join shape (store_shape.go's
// MintFlexListShapeCarrier / FlexListShapeOf and AdoptShapeValue's list arm):
// the shape flex.tsv L230 reads through (`set x 9 (f get 0)` over a pushed
// map commits the FlexMap `set`).

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A concrete plain list mints a FlexList shape whose Vals join adopts every
// element (a map element becomes a FlexMap shape); an empty list mints with
// nothing recorded; a dispatch-bearing element poisons the join.
func TestMintFlexListShapeCarrierRecordsElementJoin(t *testing.T) {
	out, ok := MintFlexListShapeCarrier(core.NewList([]core.Value{mkMap("x", core.NewInteger(1))}), 0)
	if !ok || !out.Carrier || !out.Parent.Equal(core.TFlexList) {
		t.Fatalf("a concrete plain list must mint a FlexList shape carrier, got %v", out)
	}
	ss, ok := FlexListShapeOf(out)
	if !ok {
		t.Fatal("the minted carrier must carry its shape")
	}
	el, ok := ss.LookupVals()
	if !ok {
		t.Fatal("the element join must be recorded")
	}
	if _, ok := StoreShapeOf(el); !ok || !el.Parent.Equal(core.TFlexMap) {
		t.Errorf("a map element adopts to a FlexMap shape, got %v", el)
	}
	empty, ok := MintFlexListShapeCarrier(core.NewList(nil), 0)
	if !ok {
		t.Fatal("an empty list still mints")
	}
	es, ok := FlexListShapeOf(empty)
	if !ok {
		t.Fatal("the empty list's carrier is shaped")
	}
	if _, hit := es.LookupVals(); hit {
		t.Error("an empty list records no element join")
	}
	fn := core.NewFunction(core.FnDefInfo{Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger}, Impl: core.Boru([]core.Value{core.NewInteger(1)}),
	}}})
	poisoned, ok := MintFlexListShapeCarrier(core.NewList([]core.Value{core.NewInteger(1), fn}), 0)
	if !ok {
		t.Fatal("a fn-bearing list still mints")
	}
	ps, _ := FlexListShapeOf(poisoned)
	if _, hit := ps.LookupVals(); hit {
		t.Error("a fn element must poison the element join")
	}
}

func TestMintFlexListShapeCarrierDeclines(t *testing.T) {
	cases := []struct {
		name string
		v    core.Value
	}{
		{"non-concrete", core.NewCarrier(core.TList)},
		{"non-list", core.NewInteger(3)},
		{"a map", mkMap("k", core.NewInteger(1))},
		{"a typed list (the D2 tag owns it)", core.NewTypedListWithElements(core.NewTypeLiteral(core.TInteger), []core.Value{core.NewInteger(1)})},
		{"a list-tagged non-list payload", core.Value{Parent: core.TList, Data: core.MapPayload{}}},
	}
	for _, c := range cases {
		if _, ok := MintFlexListShapeCarrier(c.v, 0); ok {
			t.Errorf("%s: mint must decline", c.name)
		}
	}
	if _, ok := MintFlexListShapeCarrier(core.NewList([]core.Value{core.NewInteger(1)}), flexShapeMaxDepth+1); ok {
		t.Error("depth past the cap must decline")
	}
}

func TestFlexListShapeOf(t *testing.T) {
	if _, ok := FlexListShapeOf(core.NewStoreShapeCarrier(core.TFlexMap, 0)); ok {
		t.Error("a FlexMap shape keys its writes: not an element-join shape")
	}
	if _, ok := FlexListShapeOf(core.NewCarrier(core.TFlexList)); ok {
		t.Error("a bare FlexList carrier has no shape")
	}
	if _, ok := FlexListShapeOf(core.NewTypeLiteral(core.TNone)); ok {
		t.Error("a parentless value is no shape")
	}
	if _, ok := FlexListShapeOf(core.NewStoreShapeCarrier(core.TFlexList, 0)); !ok {
		t.Error("a FlexList shape carrier reports its shape")
	}
}

// AdoptShapeValue: a concrete plain list adopts to a FlexList SHAPE; a list
// the shape declines (a typed list) keeps the bare FlexList carrier.
func TestAdoptShapeValueLists(t *testing.T) {
	got := AdoptShapeValue(core.NewList([]core.Value{core.NewInteger(1)}), 0)
	if _, ok := FlexListShapeOf(got); !ok {
		t.Errorf("a concrete plain list adopts to a FlexList shape, got %v", got)
	}
	typed := core.NewTypedListWithElements(core.NewTypeLiteral(core.TInteger), []core.Value{core.NewInteger(1)})
	bare := AdoptShapeValue(typed, 0)
	if _, ok := StoreShapeOf(bare); ok || !bare.Carrier || !bare.Parent.Equal(core.TFlexList) {
		t.Errorf("a typed list keeps the bare FlexList carrier, got %v", bare)
	}
}
