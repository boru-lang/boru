package native

import (
	"testing"
)

func TestTransformHandlerPassthrough(t *testing.T) {
	// Transform with literal spec values passes them through.
	data := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("a", NewInteger(1))
		return m
	}())
	spec := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("x", NewInteger(99))
		return m
	}())

	result, err := transformHandler([]Value{spec, data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' in result")
	}
	vi, _ := AsInteger(v)
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
	}
}

func TestTransformHandlerInject(t *testing.T) {
	// Transform with backtick path injects value from data.
	data := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("name", NewString("Alice"))
		return m
	}())
	spec := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("greeting", NewString("`name`"))
		return m
	}())

	result, err := transformHandler([]Value{spec, data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("greeting")
	if !ok {
		t.Fatal("expected key 'greeting' in result")
	}
	vs, _ := AsString(v)
	if vs != "Alice" {
		t.Errorf("expected Alice, got %s", vs)
	}
}

func TestTransformHandlerNestedPath(t *testing.T) {
	// Transform with nested backtick path.
	inner := NewOrderedMap()
	inner.Set("b", NewInteger(42))
	data := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("a", NewMap(inner))
		return m
	}())
	spec := NewMap(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("val", NewString("`a.b`"))
		return m
	}())

	result, err := transformHandler([]Value{spec, data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, _ := m.Get("val")
	vi, _ := AsInteger(v)
	if vi != 42 {
		t.Errorf("expected 42, got %d", vi)
	}
}

func TestValueToAnyRoundtrip(t *testing.T) {
	// Test that valueToAny -> anyToValue preserves structure.
	om := NewOrderedMap()
	om.Set("name", NewString("Bob"))
	om.Set("age", NewInteger(25))
	om.Set("active", NewBoolean(true))

	orig := NewMap(om)
	native := valueToAny(orig)
	back, err := anyToValue(native)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(back)
	name, _ := m.Get("name")
	ns, _ := AsString(name)
	if ns != "Bob" {
		t.Errorf("expected Bob, got %s", ns)
	}
	age, _ := m.Get("age")
	ai, _ := AsInteger(age)
	if ai != 25 {
		t.Errorf("expected 25, got %d", ai)
	}
	active, _ := m.Get("active")
	ab, _ := AsBoolean(active)
	if !ab {
		t.Error("expected true")
	}
}

func TestValueToAnyAtom(t *testing.T) {
	v := NewAtom("hello")
	result := valueToAny(v)
	s, ok := result.(string)
	if !ok || s != "hello" {
		t.Errorf("expected string hello, got %v", result)
	}
}

func TestValueToAnyNone(t *testing.T) {
	v := NewTypeLiteral(TNone)
	result := valueToAny(v)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestValueToAnyFloat(t *testing.T) {
	// A Float maps to a JSON number — this test previously PINNED the
	// bug that floats fell through to the default branch and were
	// stringified ({"x": "1.5"} from jsonify).
	v := NewFloat(3.14)
	result := valueToAny(v)
	f, ok := result.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", result)
	}
	if f != 3.14 {
		t.Errorf("expected 3.14, got %v", f)
	}
}

func TestValueToAnyDefault(t *testing.T) {
	// A value with no JSON projection (an unevaluated word) falls
	// through to the default branch and renders via String().
	v := NewWord("mystery")
	result := valueToAny(v)
	s, ok := result.(string)
	if !ok {
		t.Errorf("expected string from default, got %T", result)
	}
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestAnyToValueFractionalFloat(t *testing.T) {
	// 1.5 must come back as a Float — int64(1.5) used to truncate it
	// to Integer 1. Whole-valued JSON numbers keep the historical
	// Integer mapping (TestAnyToValue covers that side).
	v, err := anyToValue(1.5)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().Equal(TFloat) {
		t.Fatalf("expected Float, got %s", v.Parent().Name())
	}
	if f, _ := AsFloat(v); f != 1.5 {
		t.Errorf("expected 1.5, got %v", f)
	}
}

func TestAnyToValueNil(t *testing.T) {
	v, err := anyToValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !IsNoneShape(v) {
		t.Errorf("expected none, got %s", v.String())
	}
}

func TestAnyToValueBool(t *testing.T) {
	v, err := anyToValue(true)
	if err != nil {
		t.Fatal(err)
	}
	vb, _ := AsBoolean(v)
	if !vb {
		t.Error("expected true")
	}
}

func TestAnyToValueInt(t *testing.T) {
	v, err := anyToValue(int(42))
	if err != nil {
		t.Fatal(err)
	}
	vi, _ := AsInteger(v)
	if vi != 42 {
		t.Errorf("expected 42, got %d", vi)
	}
}

func TestAnyToValueInt64(t *testing.T) {
	v, err := anyToValue(int64(99))
	if err != nil {
		t.Fatal(err)
	}
	vi, _ := AsInteger(v)
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
	}
}

func TestAnyToValueUnsupported(t *testing.T) {
	_, err := anyToValue(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestValueToAnyList(t *testing.T) {
	elems := []Value{
		NewInteger(1),
		NewString("two"),
		NewBoolean(false),
	}
	orig := NewList(elems)
	native := valueToAny(orig)
	back, err := anyToValue(native)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(back)
	list := _lst.Slice()
	if len(list) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(list))
	}
	li0, _ := AsInteger(list[0])
	if li0 != 1 {
		t.Errorf("expected 1, got %d", li0)
	}
	ls1, _ := AsString(list[1])
	if ls1 != "two" {
		t.Errorf("expected two, got %s", ls1)
	}
	lb2, _ := AsBoolean(list[2])
	if lb2 {
		t.Error("expected false")
	}
}
