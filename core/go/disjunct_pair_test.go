package core

import "testing"

// The meet of a disjunct with a value is the disjunct of the meets of
// its alternatives with that value (NUR386). The corpus fixture `unifyq`
// exposes only the predicate; these tests hold the VALUE.

func pairCanon(t *testing.T, a, b Value) (string, bool) {
	t.Helper()
	got, err := UnifyExplainR(a, b, nil)
	if err != nil {
		return "", false
	}
	return CanonValue(got), true
}

func pairMap(kv ...any) Value {
	om := NewOrderedMap()
	for i := 0; i < len(kv); i += 2 {
		om.Set(kv[i].(string), kv[i+1].(Value))
	}
	return NewMap(om)
}

func TestDisjunctPairIsThePairwiseMeet(t *testing.T) {
	intT, strT, numT := NewTypeLiteral(TInteger), NewTypeLiteral(TString), NewTypeLiteral(TNumber)
	i5, i6, i7 := NewInteger(5), NewInteger(6), NewInteger(7)
	d := func(alts ...Value) Value { return NewDisjunct(alts) }

	for _, c := range []struct {
		name string
		a, b Value
		want string
		ok   bool
	}{
		// a disjunct against itself is itself — not its first member
		{"types, self", d(intT, strT), d(intT, strT), "Integer tor String", true},
		{"values, self", d(i5, i6), d(i5, i6), "5 tor 6", true},
		// the order the alternatives were written in decides nothing
		{"types, reordered", d(strT, intT), d(intT, strT), "Integer tor String", true},
		// one surviving meet is that value, not a one-alternative disjunct
		{"overlap of one", d(i5, i6), d(i6, i7), "6", true},
		// a type against the values it admits narrows to the values
		{"type narrows to values", d(intT, strT), d(i5, i6), "5 tor 6", true},
		// a meet absorbed by a wider meet is dropped
		{"absorbed meet", d(numT, strT), d(intT, numT), "Number", true},
		// no pair unifying is a failure
		{"disjoint", d(i5, i6), d(i7, strT), "", false},
		// a nested disjunct's meet is flattened into the result
		{"nested", d(d(i5, i6), strT), d(i5, i6), "5 tor 6", true},
	} {
		got, ok := pairCanon(t, c.a, c.b)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
		// the predicate is symmetric
		if _, back := pairCanon(t, c.b, c.a); back != c.ok {
			t.Errorf("%s: swapped operands answer %v, want %v", c.name, back, c.ok)
		}
	}
}

func TestDisjunctPairMapAlternativesMergeAsPatterns(t *testing.T) {
	i1, i2, i3 := NewInteger(1), NewInteger(2), NewInteger(3)
	d := func(alts ...Value) Value { return NewDisjunct(alts) }

	for _, c := range []struct {
		name string
		a, b Value
		want string
		ok   bool
	}{
		// two patterns: the maps both admit carry the keys of either
		{"subset", d(pairMap("a", i1)), d(pairMap("a", i1, "b", i2)), "{a:1 b:2}", true},
		{"superset", d(pairMap("a", i1, "b", i2)), d(pairMap("a", i1)), "{a:1 b:2}", true},
		{"disjoint keys", d(pairMap("a", i1)), d(pairMap("b", i2)), "{a:1 b:2}", true},
		// a shared key holds the meet of its two values
		{"shared key narrows", d(pairMap("a", NewTypeLiteral(TInteger))), d(pairMap("a", i3)), "{a:3}", true},
		// a shared key whose values do not unify admits no map at all
		{"shared key clashes", d(pairMap("a", i1)), d(pairMap("a", i2)), "", false},
		// a map alternative and a non-map alternative never meet
		{"map against scalar", d(pairMap("a", i1)), d(i1, i2), "", false},
	} {
		got, ok := pairCanon(t, c.a, c.b)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
		if _, back := pairCanon(t, c.b, c.a); back != c.ok {
			t.Errorf("%s: swapped operands answer %v, want %v", c.name, back, c.ok)
		}
	}
}

// The empty disjunct denotes the empty set: it is `Never` in every pair.
func TestEmptyDisjunctIsNever(t *testing.T) {
	empty := NewDisjunct(nil)
	never := NewTypeLiteral(TNever)

	for _, c := range []struct {
		name string
		a, b Value
		want string
		ok   bool
	}{
		{"itself", empty, empty, "Never", true},
		{"Never", empty, never, "Never", true},
		{"Never, swapped", never, empty, "Never", true},
		// as `Never` unifies with nothing else — `Any` included
		{"Any", empty, NewTypeLiteral(TAny), "", false},
		{"Any, swapped", NewTypeLiteral(TAny), empty, "", false},
		{"a value", empty, NewInteger(5), "", false},
		{"a type", empty, NewTypeLiteral(TScalar), "", false},
		{"a disjunct", empty, NewDisjunct([]Value{NewInteger(5)}), "", false},
		{"a disjunct, swapped", NewDisjunct([]Value{NewInteger(5)}), empty, "", false},
	} {
		got, ok := pairCanon(t, c.a, c.b)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// A TYPE against a disjunct narrows every alternative, not the first; a
// VALUE against a disjunct is that value.
func TestDisjunctMeetDistributesOverItsAlternatives(t *testing.T) {
	intT, strT, numT := NewTypeLiteral(TInteger), NewTypeLiteral(TString), NewTypeLiteral(TNumber)
	i5, i6 := NewInteger(5), NewInteger(6)
	d := func(alts ...Value) Value { return NewDisjunct(alts) }
	l := func(elems ...Value) Value { return NewList(elems) }

	for _, c := range []struct {
		name string
		a, b Value
		want string
		ok   bool
	}{
		{"type over values", d(i5, i6), intT, "5 tor 6", true},
		{"type over values, swapped", intT, d(i5, i6), "5 tor 6", true},
		{"wider type keeps the disjunct", d(intT, strT), NewTypeLiteral(TScalar), "Integer tor String", true},
		{"type admitting one alternative", d(intT, strT), numT, "Integer", true},
		{"type admitting none", d(i5, i6), strT, "", false},
		// a list holding a type is a shape, not a value
		{"list shape", d(l(i5), l(i6)), l(intT), "[5] tor [6]", true},
		// a typed container is a type
		{"typed list", d(l(i5), l(NewString("a"))), NewTypedList(intT), "[5]", true},
		// a check-mode carrier is no value either: the types that admit
		// it answer, and a concrete alternative does not
		{"carrier", d(numT, strT), NewCarrier(TInteger), "Integer", true},
		{"carrier against values", d(i5, i6), NewCarrier(TInteger), "", false},
		// a value is admitted by the first alternative that takes it
		{"value by type", d(intT, strT), i5, "5", true},
		{"value by value", d(i5, i6), i6, "6", true},
		{"list value", d(l(intT), l(strT)), l(i5), "[5]", true},
		{"value refused", d(i5, i6), NewInteger(7), "", false},
	} {
		got, ok := pairCanon(t, c.a, c.b)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// A map alternative is an open pattern. It admits a map VALUE carrying
// its keys and answers the value; it narrows a map SHAPE carrying them.
func TestDisjunctMapAlternativeAgainstValueAndShape(t *testing.T) {
	intT, strT := NewTypeLiteral(TInteger), NewTypeLiteral(TString)
	i1, i2 := NewInteger(1), NewInteger(2)
	d := func(alts ...Value) Value { return NewDisjunct(alts) }

	for _, c := range []struct {
		name string
		a, b Value
		want string
		ok   bool
	}{
		// a value: the answer is the value, extra keys and all
		{"value with extra key", d(pairMap("a", i1), pairMap("a", i2)), pairMap("a", i1, "b", i2), "{a:1 b:2}", true},
		{"value lacking the key", d(pairMap("a", i1)), pairMap("b", i2), "", false},
		// a shape: every alternative that fits narrows it
		{"shape narrowed by each", d(pairMap("a", i1), pairMap("a", i2)), pairMap("a", intT), "{a:1} tor {a:2}", true},
		{"shape keeps its other keys", d(pairMap("a", i1)), pairMap("a", intT, "b", strT), "{a:1 b:String}", true},
		{"shape lacking the key", d(pairMap("a", i1)), pairMap("b", intT), "", false},
		{"shape refusing the key", d(pairMap("a", i1)), pairMap("a", strT), "", false},
		// a map holding a list that holds a type is a shape too
		{"nested shape", d(pairMap("a", NewList([]Value{i1}))), pairMap("a", NewList([]Value{intT})), "{a:[1]}", true},
	} {
		got, ok := pairCanon(t, c.a, c.b)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
		if _, back := pairCanon(t, c.b, c.a); back != c.ok {
			t.Errorf("%s: swapped operands answer %v, want %v", c.name, back, c.ok)
		}
	}
}

func TestIsPlainValue(t *testing.T) {
	intT := NewTypeLiteral(TInteger)
	for _, c := range []struct {
		name string
		v    Value
		want bool
	}{
		{"integer", NewInteger(5), true},
		{"none", NewNone(), true},
		{"atom", NewAtom("x"), true},
		{"list of values", NewList([]Value{NewInteger(1), NewString("a")}), true},
		{"map of values", pairMap("a", NewInteger(1), "b", NewList([]Value{NewInteger(2)})), true},
		{"empty list", NewList(nil), true},
		{"type literal", intT, false},
		{"carrier", NewCarrier(TInteger), false},
		{"typed list", NewTypedList(intT), false},
		{"disjunct", NewDisjunct([]Value{NewInteger(5)}), false},
		{"list holding a type", NewList([]Value{NewInteger(1), intT}), false},
		{"map holding a type", pairMap("a", intT), false},
		{"map holding a disjunct", pairMap("a", NewDisjunct([]Value{NewInteger(5)})), false},
		{"map with nothing to read", NewValueRaw(TMap, MapPayload{}), false},
	} {
		if got := isPlainValue(c.v); got != c.want {
			t.Errorf("%s: isPlainValue = %v, want %v", c.name, got, c.want)
		}
	}
}
