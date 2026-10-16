package native

import (
	"testing"
)

// Wave-7 coverage for native_sort.go: the AsList/AsMap accessor-error
// arms of the sort handlers, the nil-map short-circuit, and the
// string-form fallback in compareForSort for value pairs the kernel
// cannot order (design/TEST-SEAMS.10.md).

func TestSortListHandlerRejectsTypeLiteral(t *testing.T) {
	r := b2Reg(t)
	// A bare List type literal (Data==nil) is not a concrete list.
	if _, err := sortListHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatal("sort of List type literal: expected error")
	}
	// Positive pair: a real list sorts ascending.
	out, err := sortListHandler([]Value{NewList([]Value{
		NewInteger(3), NewInteger(1), NewInteger(2),
	})}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("sort list = %v / %v", out, err)
	}
	rl, _ := AsList(out[0])
	if rl.Len() != 3 || mustI(rl.Get(0)) != 1 || mustI(rl.Get(2)) != 3 {
		t.Fatalf("sort list not ascending: %v", out[0])
	}
}

func TestSortMapHandlerArms(t *testing.T) {
	r := b2Reg(t)
	// Bare Map type literal → AsMap error arm.
	if _, err := sortMapHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r); err == nil {
		t.Fatal("sort of Map type literal: expected error")
	}
	// NOTE: the `if m == nil` guard (native_sort.go:69) is unreachable —
	// core.AsMap returns either (m, nil) or (nil, err); it never returns a
	// nil ReadMap interface with a nil error. (A MapPayload{M:nil} yields a
	// typed-nil *OrderedMap wrapped in a non-nil interface, which the
	// `m == nil` check does not catch.) Documented dead guard.

	// Positive pair: a real map sorts by value.
	om := NewOrderedMap()
	om.Set("a", NewInteger(3))
	om.Set("b", NewInteger(1))
	out, err := sortMapHandler([]Value{NewMap(om)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("sort map = %v / %v", out, err)
	}
	rm, _ := AsMap(out[0])
	if rm == nil || rm.Keys()[0] != "b" {
		t.Fatalf("sort map by value should put b first, got %v", out[0])
	}
}

func TestCompareForSortStringFallback(t *testing.T) {
	// Values with a nil Parent make CompareValues error ("nil type"), so
	// compareForSort takes its lexical String()-form fallback.
	a := NewString("aaa")
	a.SetParent(nil)
	b := NewString("bbb")
	b.SetParent(nil)
	if got := compareForSort(a, b); got != -1 {
		t.Fatalf("compareForSort(a,b) = %d, want -1 (as<bs)", got)
	}
	if got := compareForSort(b, a); got != 1 {
		t.Fatalf("compareForSort(b,a) = %d, want 1 (as>bs)", got)
	}
	if got := compareForSort(a, a); got != 0 {
		t.Fatalf("compareForSort(a,a) = %d, want 0 (equal fallback)", got)
	}
	// Positive pair: a normal orderable pair uses the kernel comparer.
	if got := compareForSort(NewInteger(1), NewInteger(2)); got != -1 {
		t.Fatalf("compareForSort(1,2) = %d, want -1", got)
	}
}

func mustI(v Value) int64 {
	n, _ := AsInteger(v)
	return n
}
