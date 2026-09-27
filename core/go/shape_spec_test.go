package core

import "testing"

func shapeTestMap(kv ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1].(Value))
	}
	return NewMap(m)
}

// A plain Map of strict values has a shape; the shape holds for exactly its
// key set and value tags.
func TestShapeOfAndHolds(t *testing.T) {
	v := shapeTestMap("a", NewInteger(1), "b", NewString("s"))
	shape, ok := ShapeOf(v, 8)
	if !ok || !IsShapeCarrier(shape) || ShapeKey(shape) == "" {
		t.Fatalf("want a shape carrier, got %v %v %q", shape, ok, ShapeKey(shape))
	}
	if !ShapeHolds(shape, shapeTestMap("a", NewInteger(7), "b", NewString("x"))) {
		t.Error("same keys and tags hold")
	}
	for name, w := range map[string]Value{
		"extra key":   shapeTestMap("a", NewInteger(7), "b", NewString("x"), "c", NewInteger(1)),
		"missing key": shapeTestMap("a", NewInteger(7), "c", NewString("x")),
		"other tag":   shapeTestMap("a", NewString("7"), "b", NewString("x")),
		"carrier":     shapeTestMap("a", NewCarrier(TInteger), "b", NewString("x")),
		"not a map":   NewInteger(1),
		"map carrier": NewCarrier(TMap),
	} {
		if ShapeHolds(shape, w) {
			t.Errorf("%s: must not hold", name)
		}
	}
	// Keys are injective: a field name holding the separator is not two
	// fields, and another shape keys differently.
	odd, _ := ShapeOf(shapeTestMap("a:Integer b", NewInteger(1)), 8)
	two, _ := ShapeOf(shapeTestMap("a", NewInteger(1), "b", NewInteger(2)), 8)
	if ShapeKey(odd) == ShapeKey(two) {
		t.Error("distinct shapes share a key")
	}
	if ShapeHolds(NewInteger(1), v) || ShapeKey(NewInteger(1)) != "" {
		t.Error("a non-shape guard never holds and has no key")
	}
	fn := NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{{}}})
	asc := shapeTestMap("a", NewInteger(1))
	asc.SetAscribed(TNode)
	ascField := NewInteger(1)
	ascField.SetAscribed(TNumber)
	for name, w := range map[string]Value{
		"empty":          NewMap(NewOrderedMap()),
		"too wide":       shapeTestMap("a", NewInteger(1), "b", NewInteger(2), "c", NewInteger(3)),
		"fn field":       shapeTestMap("a", fn),
		"carrier field":  shapeTestMap("a", NewCarrier(TInteger)),
		"ideal field":    shapeTestMap("a", Value{Parent: TReach, Data: NewInteger(1).Data}),
		"dynamic field":  shapeTestMap("a", NewDynamicCarrier(TInteger)),
		"bare field":     shapeTestMap("a", NewTypeLiteral(TInteger)),
		"ascribed field": shapeTestMap("a", ascField),
		"ascribed map":   asc,
		"no parent":      shapeTestMap("a", Value{}),
		"not a map":      NewInteger(1),
		"dynamic map":    NewDynamicCarrier(TMap),
	} {
		if _, ok := ShapeOf(w, 2); ok {
			t.Errorf("%s: no shape", name)
		}
	}
	if IsShapeCarrier(NewDynamicCarrierValue(NewRecordType(NewOrderedMap()))) {
		t.Error("a gradual record carrier is not a shape carrier")
	}
}

// A plain List of concrete elements has a shape of its exact length and
// element tags.
func TestListShape(t *testing.T) {
	shape, ok := ListShapeOf(NewList([]Value{NewInteger(1), NewInteger(2)}), 4)
	if !ok || !IsListShapeGuard(shape) || ListShapeKey(shape) == "" {
		t.Fatalf("want a list shape, got %v %v", shape, ok)
	}
	if !ListShapeHolds(shape, NewList([]Value{NewInteger(5), NewInteger(6)})) {
		t.Error("same length and tags hold")
	}
	for name, v := range map[string]Value{
		"longer":       NewList([]Value{NewInteger(5), NewInteger(6), NewInteger(7)}),
		"tag":          NewList([]Value{NewInteger(5), NewString("x")}),
		"carrier":      NewList([]Value{NewInteger(5), NewCarrier(TInteger)}),
		"map":          NewMap(NewOrderedMap()),
		"list carrier": NewCarrier(TList),
	} {
		if ListShapeHolds(shape, v) {
			t.Errorf("%s: must not hold", name)
		}
	}
	if ListShapeHolds(NewInteger(1), NewList(nil)) {
		t.Error("a non-list guard never holds")
	}
	fn := NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{{}}})
	q := NewList([]Value{NewInteger(1)})
	q.Quoted = true
	for name, v := range map[string]Value{
		"empty": NewList(nil), "too long": NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4), NewInteger(5)}),
		"fn element": NewList([]Value{fn}), "carrier element": NewList([]Value{NewCarrier(TInteger)}),
		"quoted": q, "not a list": NewInteger(1),
	} {
		if _, ok := ListShapeOf(v, 4); ok {
			t.Errorf("%s: no shape", name)
		}
	}
	for name, v := range map[string]Value{
		"concrete list": NewList([]Value{NewInteger(1)}), "empty": NewList(nil),
		"dynamic element": NewList([]Value{NewDynamicCarrier(TInteger)}), "carrier list": NewCarrier(TList),
	} {
		if IsListShapeGuard(v) {
			t.Errorf("%s: not a list shape guard", name)
		}
	}
}
