package core

// NUR068 — in-package unit tests for the record-SCHEMA-CARRIER admission
// arms in unify_map.go: a check-mode carrier bearing RecordTypeInfo
// unifies with a concrete field-bag map by the runtime's open rule
// (unifyRecordSchemaCarrierVsMap), and with a bare Map type literal as
// the map instance it abstracts. Non-carrier record values keep the
// nominal refusals on both arms.

import (
	"strings"
	"testing"
)

// recSchemaCarrier builds the check-mode carrier shape under test:
// dynamic(Map) carrying a RecordTypeInfo schema.
func recSchemaCarrier(fields *OrderedMap) Value {
	return NewDynamicCarrierValue(NewRecordType(fields))
}

func nur068Schema() *OrderedMap {
	f := NewOrderedMap()
	f.Set("name", NewTypeLiteral(TString))
	f.Set("n", NewTypeLiteral(TInteger))
	return f
}

// nur068Pattern is the dispatch pattern ResolveSigType builds for a
// record param: a CONCRETE map of field→type constraints.
func nur068Pattern() Value {
	p := NewOrderedMap()
	p.Set("name", NewTypeLiteral(TString))
	p.Set("n", NewTypeLiteral(TInteger))
	return NewMap(p)
}

func TestNur068RecordCarrierUnifiesWithFieldBag(t *testing.T) {
	rec := recSchemaCarrier(nur068Schema())
	pat := nur068Pattern()
	// Carrier on the left (patternsOk's Unify(val, pattern) orientation).
	got, err := unifyMapFamily(rec, Shape(rec), pat, Shape(pat), nil)
	if err != nil {
		t.Fatalf("record carrier vs field bag: %v", err)
	}
	if !got.Carrier || !IsRecordType(got) {
		t.Fatalf("expected the record carrier back, got %v", got)
	}
	// Carrier on the right (the swapped orientation other unify callers use).
	got2, err2 := unifyMapFamily(pat, Shape(pat), rec, Shape(rec), nil)
	if err2 != nil {
		t.Fatalf("field bag vs record carrier: %v", err2)
	}
	if !got2.Carrier || !IsRecordType(got2) {
		t.Fatalf("expected the record carrier back (swapped), got %v", got2)
	}
}

func TestNur068RecordCarrierRejectsDisjointFieldBag(t *testing.T) {
	rec := recSchemaCarrier(nur068Schema())
	// A field the schema declares with a DIFFERENT type: provably disjoint.
	p := NewOrderedMap()
	p.Set("name", NewTypeLiteral(TInteger))
	pat := NewMap(p)
	if _, err := unifyMapFamily(rec, Shape(rec), pat, Shape(pat), nil); err == nil {
		t.Fatal("String-schema field must not unify with an Integer constraint")
	}
	// A field the schema LACKS whose constraint does not accept Absent:
	// the runtime instance has exactly the schema's keys, so the pair is
	// provably disjoint.
	q := NewOrderedMap()
	q.Set("missing", NewTypeLiteral(TList))
	qat := NewMap(q)
	if _, err := unifyMapFamily(rec, Shape(rec), qat, Shape(qat), nil); err == nil {
		t.Fatal("a required key outside the schema must refuse")
	}
}

func TestNur068RecordCarrierAbsentOptionalKey(t *testing.T) {
	rec := recSchemaCarrier(nur068Schema())
	// A key outside the schema whose constraint ACCEPTS Absent (the ?:T
	// desugaring — a disjunct with an Absent alternative) stays admitted.
	p := NewOrderedMap()
	p.Set("opt", NewDisjunct([]Value{NewTypeLiteral(TAbsent), NewTypeLiteral(TInteger)}))
	pat := NewMap(p)
	if _, err := unifyMapFamily(rec, Shape(rec), pat, Shape(pat), nil); err != nil {
		t.Fatalf("Absent-accepting extra key must admit: %v", err)
	}
}

func TestNur068RecordCarrierVsTypedMap(t *testing.T) {
	// A homogeneous schema meets a compatible child constraint: admitted
	// (every runtime instance's values inhabit the field types, which meet
	// the child), in either orientation.
	hom := NewOrderedMap()
	hom.Set("a", NewTypeLiteral(TInteger))
	hom.Set("b", NewTypeLiteral(TInteger))
	rec := recSchemaCarrier(hom)
	tm := NewTypedMap(NewTypeLiteral(TInteger))
	got, err := unifyMapFamily(rec, Shape(rec), tm, Shape(tm), nil)
	if err != nil {
		t.Fatalf("homogeneous schema vs {:Integer}: %v", err)
	}
	if !got.Carrier || !IsRecordType(got) {
		t.Fatalf("expected the record carrier back, got %v", got)
	}
	if _, err := unifyMapFamily(tm, Shape(tm), rec, Shape(rec), nil); err != nil {
		t.Fatalf("{:Integer} vs homogeneous schema (swapped): %v", err)
	}
	// A field type that cannot meet the child is provably disjoint —
	// exactly as every runtime instance of the schema would be.
	het := recSchemaCarrier(nur068Schema()) // name:String n:Integer
	if _, err := unifyMapFamily(het, Shape(het), tm, Shape(tm), nil); err == nil {
		t.Fatal("a String field must refuse a {:Integer} child constraint")
	}
}

func TestNur068NonCarrierRecordKeepsNominalRefusal(t *testing.T) {
	// A record type BODY as a value (not a carrier) keeps the exclusive
	// Record-only-unifies-with-Record rule against a concrete map.
	body := NewRecordType(nur068Schema())
	pat := nur068Pattern()
	_, err := unifyMapFamily(body, Shape(body), pat, Shape(pat), nil)
	if err == nil || !strings.Contains(err.Reason, "Record only unifies with Record") {
		t.Fatalf("non-carrier record vs map must keep the nominal refusal, got %v", err)
	}
}

func TestNur068EmptySchemaCarrierFallsThrough(t *testing.T) {
	// A record carrier with no readable schema (nil Fields) declines the
	// admission and falls to the nominal refusal.
	rec := Value{Parent: TMap, Carrier: true, Dynamic: true, Data: RecordTypeInfo{}}
	pat := nur068Pattern()
	if _, err := unifyMapFamily(rec, Shape(rec), pat, Shape(pat), nil); err == nil {
		t.Fatal("nil-Fields record carrier must fall through to the refusal")
	}
}

func TestNur068UnreadableMapSideFallsThrough(t *testing.T) {
	// A ShapeMap value with no readable field bag declines the admission
	// (falls to the nominal refusal) instead of dereferencing. Two shapes:
	// a TMap-parent ExtensionPayload (classified ShapeMap by the default
	// map-family arm; not a MapPayload at all), and a Go-API-built
	// MapPayload{M: nil} (AsMap boxes the nil *OrderedMap in a non-nil
	// ReadMap, so only the payload probe catches it — the no-panics rule).
	rec := recSchemaCarrier(nur068Schema())
	ext := Value{Parent: TMap, Data: ExtensionPayload{Body: 42}}
	if _, err := unifyMapFamily(rec, Shape(rec), ext, Shape(ext), nil); err == nil {
		t.Fatal("an extension-payload map must fall through to the refusal")
	}
	nilMap := Value{Parent: TMap, Data: MapPayload{}}
	if _, err := unifyMapFamily(rec, Shape(rec), nilMap, Shape(nilMap), nil); err == nil {
		t.Fatal("a nil-OrderedMap MapPayload must fall through to the refusal")
	}
	if _, err := unifyMapFamily(nilMap, Shape(nilMap), rec, Shape(rec), nil); err == nil {
		t.Fatal("a nil-OrderedMap MapPayload must fall through (swapped)")
	}
}

func TestNur068RecordCarrierVsMapLiteral(t *testing.T) {
	rec := recSchemaCarrier(nur068Schema())
	lit := NewTypeLiteral(TMap)
	// Literal on the left.
	got, err := unifyMapFamily(lit, Shape(lit), rec, Shape(rec), nil)
	if err != nil {
		t.Fatalf("Map literal vs record carrier: %v", err)
	}
	if !got.Carrier || !IsRecordType(got) {
		t.Fatalf("expected the record carrier back, got %v", got)
	}
	// Literal on the right.
	got2, err2 := unifyMapFamily(rec, Shape(rec), lit, Shape(lit), nil)
	if err2 != nil {
		t.Fatalf("record carrier vs Map literal: %v", err2)
	}
	if !got2.Carrier || !IsRecordType(got2) {
		t.Fatalf("expected the record carrier back (swapped), got %v", got2)
	}
	// A non-carrier record BODY still refuses against the bare literal.
	body := NewRecordType(nur068Schema())
	if _, err := unifyMapFamily(lit, Shape(lit), body, Shape(body), nil); err == nil {
		t.Fatal("Map literal vs record body must keep the nominal refusal")
	}
	if _, err := unifyMapFamily(body, Shape(body), lit, Shape(lit), nil); err == nil {
		t.Fatal("record body vs Map literal must keep the nominal refusal (swapped)")
	}
}
