package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in unify_map.go — the bare-Map-literal-vs-Options
// arms and the concrete-map Absent-omission continue. Per
// design/TEST-SEAMS.10.md.

import "testing"

func TestW8UnifyMapLiteralVsOptions(t *testing.T) {
	// Bare Map type literal on the left, Options on the right → the
	// Options schema is projected through (Map lit accepts the Options).
	lit := NewTypeLiteral(TMap)
	of := NewOrderedMap()
	of.Set("x", NewTypeLiteral(TInteger))
	opts := NewOptionsType(of)
	got, err := unifyMapFamily(lit, Shape(lit), opts, Shape(opts), nil)
	if err != nil {
		t.Fatalf("Map literal vs Options: %v", err)
	}
	if !IsOptionsType(got) {
		t.Fatalf("expected Options result, got %v", got)
	}
	// Swapped: Options on the left, Map literal on the right.
	got2, err2 := unifyMapFamily(opts, Shape(opts), lit, Shape(lit), nil)
	if err2 != nil {
		t.Fatalf("Options vs Map literal: %v", err2)
	}
	if !IsOptionsType(got2) {
		t.Fatalf("expected Options result (swapped), got %v", got2)
	}
}

func TestW8UnifyConcreteMapsAbsentOmitted(t *testing.T) {
	// A key present on both sides whose values unify to Absent is omitted
	// from the result (the present-both Absent-continue arm).
	am := NewOrderedMap()
	am.Set("x", NewTypeLiteral(TAbsent))
	am.Set("keep", NewInteger(1))
	bm := NewOrderedMap()
	bm.Set("x", NewTypeLiteral(TAbsent))
	bm.Set("keep", NewInteger(1))
	aMap, _ := AsMap(NewMap(am))
	bMap, _ := AsMap(NewMap(bm))
	got, err := unifyConcreteMaps(aMap, bMap, nil)
	if err != nil {
		t.Fatalf("unifyConcreteMaps: %v", err)
	}
	m, _ := AsMap(got)
	if _, ok := m.Get("x"); ok {
		t.Fatal("Absent-valued key must be omitted from the result")
	}
	if _, ok := m.Get("keep"); !ok {
		t.Fatal("non-Absent key must be retained")
	}
}
