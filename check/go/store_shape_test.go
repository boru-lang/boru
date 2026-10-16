package check

// Unit coverage for store_shape.go — the store-identity shape helpers.
// These are pure functions over core values, which is exactly the class
// of helper the .tsv corpus cannot address: no row can hand
// MintFlexShapeCarrier a map at a chosen recursion depth or observe the
// ok=false declines directly. The white-box idiom of the surrounding
// tests applies.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// mkMap builds a concrete plain map from alternating key/value pairs.
func mkMap(pairs ...any) core.Value {
	m := core.NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(core.Value))
	}
	return core.NewMap(m)
}

func TestStoreShapeOf(t *testing.T) {
	if _, ok := StoreShapeOf(core.NewInteger(1)); ok {
		t.Error("a concrete integer is not store-shaped")
	}
	if _, ok := StoreShapeOf(core.NewCarrier(core.TMap)); ok {
		t.Error("a bare carrier has no shape payload")
	}
	shaped := core.NewStoreShapeCarrier(core.TFlexMap, 0)
	ss, ok := StoreShapeOf(shaped)
	if !ok || ss == nil {
		t.Fatal("a minted store-shape carrier must report its shape")
	}
}

func TestMintFlexShapeCarrierDeclines(t *testing.T) {
	cases := []struct {
		name string
		v    core.Value
	}{
		{"non-concrete", core.NewCarrier(core.TMap)},
		{"non-map", core.NewInteger(3)},
		{"string", core.NewString("x")},
	}
	for _, c := range cases {
		if _, ok := MintFlexShapeCarrier(c.v, 0); ok {
			t.Errorf("%s: mint must decline", c.name)
		}
	}
	// Depth past the recursion cap declines regardless of the value.
	if _, ok := MintFlexShapeCarrier(mkMap("k", core.NewInteger(1)), flexShapeMaxDepth+1); ok {
		t.Error("depth past the cap must decline")
	}
}

func TestMintFlexShapeCarrierRecordsKeys(t *testing.T) {
	src := mkMap(
		"n", core.NewInteger(7),
		"s", core.NewString("hi"),
	)
	out, ok := MintFlexShapeCarrier(src, 0)
	if !ok {
		t.Fatal("a concrete plain map must mint")
	}
	ss, ok := StoreShapeOf(out)
	if !ok {
		t.Fatal("the minted carrier must carry its shape")
	}
	if _, ok := ss.KeyTypes["n"]; !ok {
		t.Error("key n not recorded")
	}
	if _, ok := ss.KeyTypes["s"]; !ok {
		t.Error("key s not recorded")
	}
}

func TestMintFlexShapeCarrierOmitsDispatchBearing(t *testing.T) {
	// A function-valued field is omitted so the read keeps the
	// dynamic(Any) hatch; a scalar sibling is recorded.
	fnDef := core.FnDefInfo{Signatures: []core.FnSig{{
		Params: nil, Returns: []*core.Type{core.TInteger},
		Impl: core.Boru([]core.Value{core.NewInteger(1)}),
	}}}
	src := mkMap(
		"go", core.NewFunction(fnDef),
		"n", core.NewInteger(1),
	)
	out, ok := MintFlexShapeCarrier(src, 0)
	if !ok {
		t.Fatal("mint must accept the map")
	}
	ss, _ := StoreShapeOf(out)
	if _, ok := ss.KeyTypes["go"]; ok {
		t.Error("a dispatch-bearing field must be omitted from the shape")
	}
	if _, ok := ss.KeyTypes["n"]; !ok {
		t.Error("the scalar sibling must still be recorded")
	}
}

func TestMintFlexShapeCarrierNestsAndCaps(t *testing.T) {
	inner := mkMap("deep", core.NewInteger(1))
	src := mkMap("nest", inner)
	out, ok := MintFlexShapeCarrier(src, 0)
	if !ok {
		t.Fatal("mint must accept the nested map")
	}
	ss, _ := StoreShapeOf(out)
	nested, ok := ss.KeyTypes["nest"]
	if !ok {
		t.Fatal("nested key not recorded")
	}
	if _, ok := StoreShapeOf(nested); !ok {
		t.Error("a nested concrete plain map must mint a nested shape")
	}

	// At the cap, the nested field cannot mint and records as-is
	// (still the concrete map, not a shape).
	capped, ok := MintFlexShapeCarrier(src, flexShapeMaxDepth)
	if !ok {
		t.Fatal("the OUTER map at the cap itself still mints")
	}
	cs, _ := StoreShapeOf(capped)
	nested = cs.KeyTypes["nest"]
	if _, ok := StoreShapeOf(nested); ok {
		t.Error("past the cap the nested field must decay, not mint")
	}
}

func TestAdoptShapeValue(t *testing.T) {
	// An already-shaped carrier shares its pointer.
	shaped := core.NewStoreShapeCarrier(core.TFlexMap, 0)
	if got := AdoptShapeValue(shaped, 0); core.DataOf(&got) != core.DataOf(&shaped) {
		t.Error("a shaped carrier must share, not re-mint")
	}
	// A concrete plain map mints a nested shape.
	if _, ok := StoreShapeOf(AdoptShapeValue(mkMap("k", core.NewInteger(1)), 0)); !ok {
		t.Error("a concrete plain map must adopt to a shape")
	}
	// A concrete list becomes a bare FlexList carrier.
	lst := core.NewList([]core.Value{core.NewInteger(1)})
	got := AdoptShapeValue(lst, 0)
	if !core.IsCarrier(&got) || !core.ParentOf(&got).ConformsTo(core.TFlexList) {
		t.Errorf("a concrete list must adopt to a FlexList carrier, got %s", core.ParentOf(&got).Leaf())
	}
	// A scalar records as-is.
	n := core.NewInteger(9)
	if got := AdoptShapeValue(n, 0); !core.ParentOf(&got).Equal(core.TInteger) || core.IsCarrier(&got) {
		t.Error("a scalar must record as-is")
	}
}

func TestShapeFieldRead(t *testing.T) {
	// A shape reads back as a dynamic carrier that KEEPS the shape, so
	// chained reads narrow too.
	shaped := core.NewStoreShapeCarrier(core.TFlexMap, 0)
	got := ShapeFieldRead(shaped)
	if !core.IsDynamic(&got) {
		t.Error("a shaped field read must stay dynamic")
	}
	// A disjunct keeps its payload, dynamically.
	dis := core.NewDisjunct([]core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)})
	if got := ShapeFieldRead(dis); !core.IsDynamic(&got) {
		t.Error("a disjunct field read must stay dynamic")
	}
	// A function-typed value keeps the dynamic(Any) hatch.
	fnDef := core.FnDefInfo{Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}}}
	got = ShapeFieldRead(core.NewFunction(fnDef))
	if !core.IsDynamic(&got) || !core.ParentOf(&got).Equal(core.TAny) {
		t.Errorf("a dispatch-bearing field must read dynamic(Any), got %s", core.ParentOf(&got).Leaf())
	}
	// An ordinary value reads back as a dynamic carrier of its type — a
	// bound a guard discharges, never a strict commitment.
	got = ShapeFieldRead(core.NewInteger(5))
	if !core.IsDynamic(&got) || !core.ParentOf(&got).ConformsTo(core.TInteger) {
		t.Errorf("an integer field must read dynamic(Integer), got %s", core.ParentOf(&got).Leaf())
	}
}
