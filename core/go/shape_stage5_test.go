package core

// Stage-5 coverage tests for store_shape_state.go: RecordKey / LookupKey /
// RecordVal / LookupVals / CloneShape, including every nil-receiver guard.
// RecordVal has no production caller today; it is exercised here as part of
// the shape-state model contract (join-only, poison-on-dispatch-bearing).
//
// The join these methods apply is core's own JoinCarriers (ADR-013,
// 2026-08-08 — it used to be reached through the JoinCarriersHook slot,
// installed by the check piece). So the repeat-write assertions below
// pin the REAL fold: two distinct Integer literals widen to a bare
// Integer carrier, which is exactly what "join-only, never replace"
// means for this state.

import "testing"

func TestStage5ShapeRecordKey(t *testing.T) {
	var nilShape *StoreShapeInfo
	nilShape.RecordKey("k", NewInteger(1)) // nil receiver: no-op, no panic

	s := &StoreShapeInfo{}
	s.RecordKey("", NewInteger(1)) // empty key: no-op
	if s.KeyTypes != nil {
		t.Fatal("empty-key RecordKey must not allocate KeyTypes")
	}

	s.RecordKey("k", NewInteger(1))
	if s.KeyTypes == nil {
		t.Fatal("first RecordKey must allocate KeyTypes")
	}
	got, ok := s.LookupKey("k")
	if !ok {
		t.Fatal("recorded key must be readable")
	}
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("recorded carrier = %v (%v), want 1", got, err)
	}

	// A repeat write JOINS — never replaces. Two distinct Integer
	// literals share Integer as their immediate parent, so the fold
	// widens to a bare Integer carrier rather than keeping either value.
	s.RecordKey("k", NewInteger(2))
	got, _ = s.LookupKey("k")
	if !got.Carrier() {
		t.Errorf("joined entry = %v, want a carrier", got)
	}
	if !ValueType(got).Equal(TInteger) {
		t.Errorf("joined entry type = %v, want Integer", ValueType(got))
	}
	if _, err := AsInteger(got); err == nil {
		t.Error("the join must widen away both concrete payloads")
	}
}

func TestStage5ShapeLookupKey(t *testing.T) {
	var nilShape *StoreShapeInfo
	if _, ok := nilShape.LookupKey("k"); ok {
		t.Error("nil-shape LookupKey must decline")
	}
	s := &StoreShapeInfo{}
	if _, ok := s.LookupKey("absent"); ok {
		t.Error("an unwritten key must read as absent")
	}
}

func TestStage5ShapeRecordVal(t *testing.T) {
	var nilShape *StoreShapeInfo
	nilShape.RecordVal(NewInteger(1)) // nil receiver: no-op, no panic
	if _, ok := nilShape.LookupVals(); ok {
		t.Error("nil-shape LookupVals must decline")
	}

	s := &StoreShapeInfo{}
	if _, ok := s.LookupVals(); ok {
		t.Error("nothing recorded: LookupVals must decline")
	}

	// First unkeyed write is stored directly.
	s.RecordVal(NewInteger(1))
	got, ok := s.LookupVals()
	if !ok {
		t.Fatal("recorded val must be readable")
	}
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("vals = %v (%v), want 1", got, err)
	}

	// A second write joins, widening to a bare Integer carrier.
	s.RecordVal(NewInteger(2))
	got, _ = s.LookupVals()
	if !got.Carrier() || !ValueType(got).Equal(TInteger) {
		t.Errorf("joined vals = %v, want an Integer carrier", got)
	}

	// A dispatch-bearing value poisons the join; readers decline from
	// then on, and later writes are ignored.
	s.RecordVal(NewFunction(FnDefInfo{}))
	if !s.ValsPoisoned {
		t.Fatal("a Function write must poison the unkeyed join")
	}
	if _, ok := s.LookupVals(); ok {
		t.Error("poisoned LookupVals must decline")
	}
	s.RecordVal(NewInteger(3)) // ignored: already poisoned
	if _, ok := s.LookupVals(); ok {
		t.Error("a write after poisoning must not revive the join")
	}

	// A zero Value (Parent == nil) poisons a fresh shape too.
	s2 := &StoreShapeInfo{}
	s2.RecordVal(Value{})
	if !s2.ValsPoisoned {
		t.Error("a parentless value must poison the join")
	}
}

func TestStage5ShapeCloneShape(t *testing.T) {
	var nilShape *StoreShapeInfo
	if nilShape.CloneShape() != nil {
		t.Error("CloneShape of nil must be nil")
	}

	// A shape with no KeyTypes clones with a nil map.
	bare := &StoreShapeInfo{Scope: 1}
	if cp := bare.CloneShape(); cp == nil || cp.KeyTypes != nil || cp.Scope != 1 {
		t.Errorf("bare clone = %+v, want Scope 1 and nil KeyTypes", cp)
	}

	s := &StoreShapeInfo{Scope: 3, DeclaredVal: TInteger, Vals: NewInteger(9), ValsPoisoned: false}
	s.KeyTypes = map[string]Value{
		"a": NewInteger(1),
		"b": NewString("x"),
	}
	cp := s.CloneShape()
	if cp == nil {
		t.Fatal("CloneShape returned nil for a populated shape")
	}
	if cp.Scope != 3 || cp.DeclaredVal != TInteger || cp.ValsPoisoned {
		t.Errorf("clone lost scalar state: %+v", cp)
	}
	if n, err := AsInteger(cp.Vals); err != nil || n != 9 {
		t.Errorf("clone Vals = %v (%v), want 9", cp.Vals, err)
	}
	if len(cp.KeyTypes) != 2 {
		t.Fatalf("clone KeyTypes size = %d, want 2", len(cp.KeyTypes))
	}
	// The map is COPIED: a later write through the clone must not appear
	// in the original (entry-level sharing only).
	cp.KeyTypes["c"] = NewInteger(3)
	if _, ok := s.KeyTypes["c"]; ok {
		t.Error("clone map must be independent of the original")
	}
}
