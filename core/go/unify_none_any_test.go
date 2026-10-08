package core

// NUR069 (the none-vs-Any axis) — unit pins for foldNoneRoot: `Any`
// admits `none` at the unifier, aligning the pattern walks on the
// reading `make`'s field rule and `is` always had. Never and Absent
// keep their self-only folds — the `?:T` optional-key machinery
// depends on `Unify(Any, Absent)` declining.

import "testing"

func TestNoneUnifiesWithAny(t *testing.T) {
	noneV := NewTypeLiteral(TNone)
	anyLit := NewTypeLiteral(TAny)

	// Both orientations; the intersection is the none side.
	got, ok := Unify(anyLit, noneV)
	if !ok || Shape(got) != ShapeNone {
		t.Fatalf("Unify(Any, none) = (%v, %v), want the none side", got, ok)
	}
	got2, ok2 := Unify(noneV, anyLit)
	if !ok2 || Shape(got2) != ShapeNone {
		t.Fatalf("Unify(none, Any) = (%v, %v), want the none side", got2, ok2)
	}

	// An Any-bounded CARRIER (strict or dynamic) admits none the same
	// way — Shape classifies both as ShapeAny.
	dc := NewDynamicCarrier(TAny)
	if _, ok := Unify(dc, noneV); !ok {
		t.Fatal("Unify(dynamic(Any), none) must admit")
	}

	// The record-field walk this exists for: a pattern field declared
	// Any admits a stored none (the make/param/return triangle).
	pat := NewOrderedMap()
	pat.Set("x", NewTypeLiteral(TAny))
	val := NewOrderedMap()
	val.Set("x", NewTypeLiteral(TNone))
	if _, ok := Unify(NewMap(pat), NewMap(val)); !ok {
		t.Fatal("a declared-Any record field must admit a stored none")
	}

	// none still declines every non-Any, non-none side.
	if _, ok := Unify(noneV, NewInteger(1)); ok {
		t.Fatal("Unify(none, 1) must decline")
	}
	if _, ok := Unify(NewTypeLiteral(TInteger), noneV); ok {
		t.Fatal("Unify(Integer, none) must decline")
	}
	// Never and Absent keep the self-only rule against Any.
	if _, ok := Unify(anyLit, NewTypeLiteral(TNever)); ok {
		t.Fatal("Unify(Any, Never) must keep declining")
	}
	if _, ok := Unify(anyLit, NewTypeLiteral(TAbsent)); ok {
		t.Fatal("Unify(Any, Absent) must keep declining (?:T optionality)")
	}
}
