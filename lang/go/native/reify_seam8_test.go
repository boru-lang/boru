package native

import (
	"testing"
)

// w8ClassReg defines a class via boru and returns its resolved type Value
// plus the registry, ready to drive reifyHandler directly (the reify word
// lives behind the boru:struct-util module, whose resolver the bare
// DefaultRegistry does not configure).
func w8ClassReg(t *testing.T, def string, name string) (*Registry, Value) {
	t.Helper()
	r := w8Reg(t)
	if _, err := w8RunOn(t, r, def); err != nil {
		t.Fatalf("define %s: %v", name, err)
	}
	tv, ok := r.ResolveTypedName(name)
	if !ok {
		t.Fatalf("%s did not resolve", name)
	}
	// The name denotes its NODE (the Stage 2 flip); the schema is the
	// node's recorded content.
	if body, bok := TypeContentOf(tv); bok {
		tv = body
	}
	if !IsClassType(&tv) {
		t.Fatalf("%s did not resolve to a class type", name)
	}
	return r, tv
}

func TestW8ReifyInputNotMap(t *testing.T) {
	r, tv := w8ClassReg(t, `def Point class {x:Integer}`, "Point")
	// A non-string, non-map input fails the RequireConcreteMap guard.
	if _, err := reifyHandler([]Value{tv, NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("reify with an Integer input must error")
	}
}

func TestW8ReifyNodeClassKeyStringValue(t *testing.T) {
	r, tv := w8ClassReg(t, `def Point class {x:Integer}`, "Point")
	// Node form: $class arrives as a String Value (not a Go string).
	m := NewOrderedMap()
	m.Set("$class", NewString("Point"))
	m.Set("x", NewInteger(1))
	if _, err := reifyHandler([]Value{tv, NewMap(m)}, nil, nil, r); err != nil {
		t.Fatalf("reify with $class String value: %v", err)
	}
}

func TestW8ReifyEscapedDollarKey(t *testing.T) {
	// A class whose field name literally starts with `$`; the serialized
	// form escapes it as `$$x`, which reify unescapes back to `$x`.
	r, tv := w8ClassReg(t, `def C2 class {$x:Integer}`, "C2")
	m := NewOrderedMap()
	m.Set("$$x", NewInteger(1))
	if _, err := reifyHandler([]Value{tv, NewMap(m)}, nil, nil, r); err != nil {
		t.Fatalf("reify with $$-escaped key: %v", err)
	}
}

func TestW8ReifyNestedFieldValueError(t *testing.T) {
	// A nested class field whose supplied sub-map has an unknown field:
	// the recursive reify inside reifyFieldValue errors and propagates.
	r, tv := w8ClassReg(t, `def Inner class {y:Integer}  def Outer class {inner:Inner}`, "Outer")
	inner := NewOrderedMap()
	inner.Set("z", NewInteger(1)) // Inner has no field "z"
	m := NewOrderedMap()
	m.Set("inner", NewMap(inner))
	if _, err := reifyHandler([]Value{tv, NewMap(m)}, nil, nil, r); err == nil {
		t.Fatal("reify with an invalid nested class value must error")
	}
}
