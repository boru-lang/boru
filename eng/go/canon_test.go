package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestCanonValue(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	m.Set("b", core.NewAtom("foo"))

	quotedList := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	core.SetQuoted(&quotedList, true)

	cases := []struct {
		name string
		v    core.Value
		want string
	}{
		{"none", core.NewNone(), "none"},
		{"integer", core.NewInteger(42), "42"},
		{"negative integer", core.NewInteger(-7), "-7"},
		{"decimal", core.NewFloat(3.14), "3.14"},
		{"whole decimal", core.NewFloat(7), "7.0"},
		{"string", core.NewString("hello"), "'hello'"},
		{"true", core.NewBoolean(true), "true"},
		{"false", core.NewBoolean(false), "false"},
		{"atom", core.NewAtom("foo"), "foo/q"},
		{"empty list", core.NewList(nil), "[]"},
		{"list of ints", core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)}), "[1 2 3]"},
		{"list with atom", core.NewList([]core.Value{core.NewInteger(1), core.NewAtom("foo")}), "[1 foo/q]"},
		{"nested list", core.NewList([]core.Value{core.NewList([]core.Value{core.NewInteger(1)}), core.NewList([]core.Value{core.NewInteger(2)})}), "[[1] [2]]"},
		{"quoted list", quotedList, "(quote [1 2])"},
		{"map", core.NewMap(m), "{a:1 b:foo/q}"},
		{"type literal", core.NewTypeLiteral(core.TInteger), "Integer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := core.CanonValue(tc.v)
			if got != tc.want {
				t.Errorf("CanonValue(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestCanonStack(t *testing.T) {
	stack := []core.Value{
		core.NewInteger(1),
		core.NewAtom("foo"),
		core.NewString("bar"),
		core.NewBoolean(true),
	}
	got := core.Canon(stack)
	want := "1 foo/q 'bar' true"
	if got != want {
		t.Errorf("Canon = %q, want %q", got, want)
	}
}

func TestCanonEmptyStack(t *testing.T) {
	got := core.Canon(nil)
	if got != "" {
		t.Errorf("Canon(nil) = %q, want empty string", got)
	}
}
