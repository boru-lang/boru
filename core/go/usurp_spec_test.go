package core

import (
	"errors"
	"strings"
	"testing"
)

// `/u` reverses a FUNCTION's argument order; a name bound to anything else
// has no signature to reverse and is refused with illegal_ref, naming what
// the name is bound to. Naming it by its Parent was wrong twice over: a
// type literal's Parent is its lattice parent, and a root literal — None,
// Any, Never — has none, so the read dereferenced nil and surfaced as
// internal_error.
func TestUsurpOfANonFunctionNamesItsType(t *testing.T) {
	for _, c := range []struct {
		name  string
		bound Value
		want  string
	}{
		{"a value", NewInteger(5), "x is bound to Integer"},
		{"a string", NewString("a"), "x is bound to ProperString"},
		{"none", NewNone(), "x is bound to None"},
		// a type literal is named as itself, not as its lattice parent
		{"a type literal", NewTypeLiteral(TInteger), "x is bound to Integer"},
		// the roots have no lattice parent at all
		{"the None literal", NewTypeLiteral(TNone), "x is bound to None"},
		{"the Any literal", NewTypeLiteral(TAny), "x is bound to Any"},
		{"the Never literal", NewTypeLiteral(TNever), "x is bound to Never"},
	} {
		for _, asValue := range []bool{false, true} {
			r := coreSpecRegistry(t)
			r.Defs.Push("x", c.bound)
			_, err := NewTop(r).Run([]Value{NewWordUsurp("x", asValue)})
			var be *BoruError
			if !errors.As(err, &be) || be.Code != "illegal_ref" {
				t.Errorf("%s (/uv=%v): got %v, want illegal_ref", c.name, asValue, err)
				continue
			}
			if !strings.Contains(be.Detail, c.want) {
				t.Errorf("%s (/uv=%v): detail %q does not say %q", c.name, asValue, be.Detail, c.want)
			}
		}
	}
}

// A function is not refused: the wrapper dispatches, or stands as a value.
func TestUsurpOfAFunctionIsNotRefused(t *testing.T) {
	r := coreSpecRegistry(t)
	out, err := NewTop(r).Run([]Value{NewWordUsurp("negq", false), NewInteger(5)})
	if err != nil || len(out) != 1 || CanonValue(out[0]) != "-5" {
		t.Fatalf("negq/u 5: got %v, %v; want -5", out, err)
	}
	out, err = NewTop(r).Run([]Value{NewWordUsurp("negq", true)})
	if err != nil || len(out) != 1 || !IsFunctionRef(out[0]) {
		t.Fatalf("negq/uv: got %v, %v; want a function value", out, err)
	}
}

// usurpProbe registers a word of n gradual parameters, forward-eligible up
// to barrier, that returns its argument vector as one list — so a result
// shows the binding itself. The Lean specification's ArgumentRule theorems
// run the same probe (spec/Boru/Core/Theorems/ArgumentRule.lean).
func usurpProbe(r *Registry, name string, n, barrier int) {
	params := make([]FnParam, n)
	for i := range params {
		params[i] = FnParam{Name: "a" + string(rune('0'+i)), Type: TAny}
	}
	r.Register(name, Signature{
		Params:  params,
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList(append([]Value(nil), a...))}, nil
		}),
		BarrierPos: barrier,
	})
}

// USURP REVERSES THE BINDING, AT EVERY BARRIER: `f/u a b c` hands the
// original `c b a` whether the original collects forward (barrier n), from
// the stack (barrier 0) or both — and the wrapper is itself an ordinary
// all-forward function, so every split of ITS operands is the same call.
func TestUsurpReversesAtEveryBarrier(t *testing.T) {
	a, b, c := NewInteger(1), NewString("a"), NewBoolean(true)
	u := func(name string) Value { return NewWordUsurp(name, false) }
	for _, tc := range []struct {
		name string
		tape []Value
		want string
	}{
		{"arity 1", []Value{u("f1"), a}, "[1]"},
		{"arity 2, all forward", []Value{u("f2"), a, b}, "['a' 1]"},
		{"arity 2, stack form", []Value{u("s2"), a, b}, "['a' 1]"},
		{"arity 3, all forward", []Value{u("f3"), a, b, c}, "[true 'a' 1]"},
		{"arity 3, barrier in the middle", []Value{u("g3"), a, b, c}, "[true 'a' 1]"},
		// the wrapper's own splits
		{"split 2+1", []Value{c, u("f3"), a, b}, "[true 'a' 1]"},
		{"split 1+2", []Value{c, b, u("f3"), a}, "[true 'a' 1]"},
		{"split 0+3", []Value{c, b, a, u("f3")}, "[true 'a' 1]"},
		{"split 1+2, barrier in the middle", []Value{c, b, u("g3"), a}, "[true 'a' 1]"},
		// the unusurped calls, for contrast
		{"plain arity 3", []Value{NewWord("f3"), a, b, c}, "[1 'a' true]"},
		{"plain stack form", []Value{a, b, NewWord("s2")}, "['a' 1]"},
	} {
		r := coreSpecRegistry(t)
		usurpProbe(r, "f1", 1, 1)
		usurpProbe(r, "f2", 2, 2)
		usurpProbe(r, "f3", 3, 3)
		usurpProbe(r, "g3", 3, 1)
		usurpProbe(r, "s2", 2, 0)
		out, err := NewTop(r).Run(tc.tape)
		if err != nil || len(out) != 1 || CanonValue(out[0]) != tc.want {
			t.Errorf("%s: got %v, %v; want %s", tc.name, out, err, tc.want)
		}
	}
}
