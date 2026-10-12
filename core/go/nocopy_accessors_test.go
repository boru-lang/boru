package core

import "testing"

// Value.Is runs the default match in place for a behavior that carries the
// matchesAsDefault marker and never calls that behavior's Match: the marker
// promises its Match IS the default match. Pin the promise for every marked
// behavior, so a Match edited to do something else fails here instead of
// being silently bypassed.
func TestMarkedBehaviorsMatchByDefault(t *testing.T) {
	marked := []TypeBehavior{
		objectConvertBehavior{}, storeConvertBehavior{}, errorConvertBehavior{},
		reachConvertBehavior{}, tableConvertBehavior{}, xmlBehavior{},
		listFormatBehavior{}, mapFormatBehavior{}, moduleTypeBehavior{},
	}
	vals := []Value{NewInteger(1), NewString("s"), NewList([]Value{NewInteger(1)}), NewMap(NewOrderedMap())}
	types := []*Type{TInteger, TString, TList, TMap, TAny}
	for _, b := range marked {
		if _, ok := b.(defaultMatcher); !ok {
			t.Errorf("%T lost its matchesAsDefault marker", b)
		}
		for i := range vals {
			for _, ty := range types {
				if got, want := b.Match(vals[i], ty), defaultMatch(&vals[i], ty); got != want {
					t.Errorf("%T.Match(%s, %s) = %v; the default match says %v", b, vals[i].String(), ty.ID, got, want)
				}
			}
		}
	}
}

// The no-copy readers mirror their copying twins: TopRef answers what Top
// answers, without the copy, and a nil table binds nothing.
func TestDefTableTopRefMirrorsTop(t *testing.T) {
	var none *DefTable
	if e, ok := none.TopRef("x"); ok || e != nil {
		t.Error("a nil DefTable binds nothing")
	}
	dt := NewDefTable()
	dt.Push("x", NewInteger(7))
	e, ok := dt.TopRef("x")
	if !ok || e == nil {
		t.Fatal("TopRef must find a bound name")
	}
	if n, _ := AsInteger(e.Body); n != 7 {
		t.Errorf("TopRef(x).Body = %v, want 7", e.Body)
	}
	if _, ok := dt.TopRef("y"); ok {
		t.Error("TopRef of an unbound name must report false")
	}
}

// Tape.Ref out of range is the tape's zero cell, as At's is the zero Value,
// and it is cleared on every such read.
func TestTapeRefOutOfRangeIsAZeroCell(t *testing.T) {
	tp := NewTape([]Value{NewInteger(1)}, 0)
	for _, i := range []int{-1, 1, 5} {
		r := tp.Ref(i)
		if r == nil || r.Parent != nil || r.Data != nil {
			t.Fatalf("Ref(%d) = %v, want the zero cell", i, r)
		}
		r.Quoted = true // a reader that wrote through it…
		if tp.Ref(i).Quoted {
			t.Fatalf("Ref(%d): the zero cell must be cleared on every read", i)
		}
	}
	if n, _ := AsInteger(*tp.Ref(0)); n != 1 {
		t.Errorf("Ref(0) = %v, want the cell", tp.Ref(0))
	}
}

// ReadList.Ref is the element where it lives.
func TestReadListRefIsTheElement(t *testing.T) {
	elems := []Value{NewInteger(1), NewInteger(2)}
	rl := NewReadList(elems)
	if rl.Ref(1) != &elems[1] {
		t.Error("ReadList.Ref must point at the list's own element")
	}
}
