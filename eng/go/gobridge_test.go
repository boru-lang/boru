package eng

import (
	"reflect"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestToNativeScalars(t *testing.T) {
	cases := []struct {
		name string
		in   core.Value
		want any
	}{
		{"string", core.NewString("hi"), "hi"},
		{"integer", core.NewInteger(42), int64(42)},
		{"decimal", core.NewFloat(3.14), float64(3.14)},
		{"boolean-true", core.NewBoolean(true), true},
		{"boolean-false", core.NewBoolean(false), false},
		{"atom", core.NewAtom("book"), "book"},
		{"none", core.NewNone(), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := core.ToNative(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ToNative(%s) = %#v, want %#v", c.name, got, c.want)
			}
		})
	}
}

func TestToNativeList(t *testing.T) {
	v := core.NewList([]core.Value{core.NewInteger(1), core.NewString("a"), core.NewBoolean(true)})
	got := core.ToNative(v)
	want := []any{int64(1), "a", true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToNative(list) = %#v, want %#v", got, want)
	}
}

func TestToNativeMap(t *testing.T) {
	om := core.NewOrderedMap()
	om.Set("name", core.NewString("Alice"))
	om.Set("age", core.NewInteger(30))
	v := core.NewMap(om)
	got := core.ToNative(v)
	want := map[string]any{"name": "Alice", "age": int64(30)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToNative(map) = %#v, want %#v", got, want)
	}
}

func TestFromNativeScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any // expected ToNative result for round-trip
	}{
		{"nil", nil, nil},
		{"string", "hi", "hi"},
		{"bool", true, true},
		{"int", 7, int64(7)},
		{"int64", int64(123), int64(123)},
		{"float-integral", float64(2.0), int64(2)},
		{"float-fractional", float64(2.5), float64(2.5)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := core.FromNative(c.in)
			got := core.ToNative(v)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ToNative(FromNative(%v)) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestRoundTripNested(t *testing.T) {
	in := map[string]any{
		"id":     int64(1),
		"name":   "Alice",
		"active": true,
		"tags":   []any{"a", "b", "c"},
		"meta":   map[string]any{"k": int64(9)},
	}
	out := core.ToNative(core.FromNative(in))
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", out, in)
	}
}

func TestFromNativeFallback(t *testing.T) {
	type custom struct{ X int }
	v := core.FromNative(custom{X: 5})
	if !v.Parent().ConformsTo(core.TString) {
		t.Fatalf("expected fallback to String for unknown type, got %s", v.Parent())
	}
}
