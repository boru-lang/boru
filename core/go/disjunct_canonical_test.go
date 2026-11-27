package core

import "testing"

// disjunctAltsCanonical must agree with SimplifyDisjunctAlts on every case:
// it says "already canonical" exactly when simplifying changes nothing.
// Positive and negative cases for each reason an alternative list is not
// canonical — a Never alternative, a repeat, an absorbed alternative, an
// inversion — and for the lists that are.
func TestDisjunctAltsCanonicalAgreesWithSimplify(t *testing.T) {
	I, S, N := NewTypeLiteral(TInteger), NewTypeLiteral(TString), NewTypeLiteral(TNumber)
	never := NewTypeLiteral(TNever)
	five, six := NewInteger(5), NewInteger(6)
	cases := []struct {
		name      string
		alts      []Value
		canonical bool
	}{
		{"empty", nil, true},
		{"one type", []Value{I}, true},
		{"types in rank order", []Value{I, S}, true},
		{"a type before its value", []Value{I, five}, true},
		{"two values in order", []Value{five, six}, true},
		{"a Never alternative", []Value{I, never}, false},
		{"a repeated value", []Value{five, five}, false},
		{"a repeated type", []Value{I, I}, false},
		{"a type absorbed by a wider type", []Value{I, N}, false},
		{"a value absorbed by a wider type", []Value{N, five}, false},
		{"types out of order", []Value{S, I}, false},
		{"a value before its type", []Value{five, I}, false},
		{"values out of order", []Value{six, five}, false},
	}
	for _, c := range cases {
		got := disjunctAltsCanonical(c.alts)
		if got != c.canonical {
			t.Errorf("%s: disjunctAltsCanonical = %v, want %v", c.name, got, c.canonical)
		}
		simplified := SimplifyDisjunctAlts(c.alts)
		same := len(simplified) == len(c.alts)
		for i := 0; same && i < len(simplified); i++ {
			same = CanonValue(simplified[i]) == CanonValue(c.alts[i])
		}
		if same != c.canonical {
			t.Errorf("%s: SimplifyDisjunctAlts leaves it unchanged = %v, but canonical = %v", c.name, same, c.canonical)
		}
	}
}

// A plain disjunct is canonical after resolution whatever its alternatives
// are; an Enum keeps its declared order when nothing in it changed.
func TestResolveCanonicalisesPlainDisjunctNotEnum(t *testing.T) {
	five, I := NewInteger(5), NewTypeLiteral(TInteger)
	got := CanonValue(ResolveWordsDeep(NewDisjunct([]Value{five, five, I})))
	if got != "Integer tor 5" {
		t.Errorf("plain disjunct: got %q, want %q", got, "Integer tor 5")
	}
	// already canonical: returned as it is
	if got := CanonValue(ResolveWordsDeep(NewDisjunct([]Value{I, five}))); got != "Integer tor 5" {
		t.Errorf("canonical disjunct: got %q", got)
	}
	six := NewInteger(6)
	if got := CanonValue(ResolveWordsDeep(NewEnum([]Value{six, five}))); got != "6 tor 5" {
		t.Errorf("enum: declared order must survive resolution, got %q", got)
	}
}
