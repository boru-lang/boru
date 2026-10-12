package core

// Seam-6b wave B0: unit tests exercising previously-unreached blocks in
// core_make.go — guard arms of the make family (records, tables, classes,
// resources, options, user scalar refinements) and the FreshenDefault /
// containsSharedMutable helpers. All calls are in-package and direct,
// per design/TEST-SEAMS.10.md. Fresh registries throughout; no globals.

import (
	"strings"
	"testing"
)

// s6b0Rec builds a RecordTypeInfo with the given field constraints.
func s6b0Rec(pairs ...any) RecordTypeInfo {
	fields := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		fields.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return RecordTypeInfo{Fields: fields}
}

func s6b0Map(pairs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(m)
}

// --- MakeRecordR guard arms -------------------------------------------------

func TestS6b0MakeRecordBaseValueError(t *testing.T) {
	// useBase with a concrete-default constraint: BaseValueForConstraint
	// cannot derive a base value from a non-type constraint.
	rec := s6b0Rec("a", NewInteger(5))
	_, err := MakeRecordR(rec, s6b0Map(), true, nil)
	if err == nil || !strings.Contains(err.Error(), `field "a"`) {
		t.Fatalf("expected base-value error for field a, got %v", err)
	}
}

func TestS6b0MakeRecordMapCarrierSource(t *testing.T) {
	rec := s6b0Rec("a", NewTypeLiteral(TInteger))
	_, err := MakeRecordR(rec, NewCarrier(TMap), false, nil)
	if err == nil || !strings.Contains(err.Error(), "expected concrete map") {
		t.Fatalf("expected concrete-map error, got %v", err)
	}
}

func TestS6b0MakeRecordListCarrierSource(t *testing.T) {
	rec := s6b0Rec("a", NewTypeLiteral(TInteger))
	_, err := MakeRecordR(rec, NewCarrier(TList), false, nil)
	if err == nil || !strings.Contains(err.Error(), "concrete list") {
		t.Fatalf("expected concrete-list error, got %v", err)
	}
}

func TestS6b0MakeRecordFirstElemMapCarrierFallsToPositional(t *testing.T) {
	// First element conforms to Map but is a carrier: the named-form
	// probe must reset isNamed and take the positional path.
	rec := s6b0Rec("a", NewTypeLiteral(TInteger))
	_, err := MakeRecordR(rec, NewList([]Value{NewCarrier(TMap)}), false, nil)
	if err == nil {
		t.Fatal("expected a field-conversion error for the carrier element")
	}
	if strings.Contains(err.Error(), "mixed named") {
		t.Fatalf("carrier first element must not be treated as named form: %v", err)
	}
}

func TestS6b0MakeRecordNamedPairCarrier(t *testing.T) {
	rec := s6b0Rec("a", NewTypeLiteral(TInteger))
	src := NewList([]Value{s6b0Map("a", NewInteger(1)), NewCarrier(TMap)})
	_, err := MakeRecordR(rec, src, false, nil)
	if err == nil || !strings.Contains(err.Error(), "concrete map pair") {
		t.Fatalf("expected concrete-map-pair error, got %v", err)
	}
}

func TestS6b0MakeRecordNamedUnknownField(t *testing.T) {
	rec := s6b0Rec("a", NewTypeLiteral(TInteger))
	src := NewList([]Value{s6b0Map("zz", NewInteger(1))})
	_, err := MakeRecordR(rec, src, false, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown field "zz"`) {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

// --- parseMakeOptions / makeObject -------------------------------------------

func TestS6b0ParseMakeOptionsCarrier(t *testing.T) {
	if _, err := parseMakeOptions(NewCarrier(TMap)); err == nil {
		t.Fatal("expected an error for a map-carrier options value")
	}
}

func TestS6b0MakeObjectMapCarrierSource(t *testing.T) {
	r := newTestRegistry(t)
	objType := ClassTypeInfo{Name: "Ideal/S6b0O", Fields: NewOrderedMap()}
	_, err := MakeObject(objType, NewCarrier(TMap), r)
	if err == nil || !strings.Contains(err.Error(), "expected concrete map") {
		t.Fatalf("expected concrete-map error, got %v", err)
	}
}

// --- makeResource default fill ------------------------------------------------

func TestS6b0MakeResourceDefaultFill(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(7))
	res, err := MakeResource(ResourceTypeInfo{Name: "S6b0R", Fields: fields}, NewOrderedMap(), r)
	if err != nil {
		t.Fatalf("MakeResource: %v", err)
	}
	info, err := AsResourceInstance(res[0])
	if err != nil {
		t.Fatalf("AsResourceInstance: %v", err)
	}
	got, ok := info.Fields.Get("a")
	if !ok {
		t.Fatal("defaulted field a missing")
	}
	if n, err := AsInteger(got); err != nil || n != 7 {
		t.Errorf("default fill = %v (%v), want 7", got, err)
	}
}

// --- containsSharedMutable / FreshenDefault -----------------------------------

func TestS6b0ContainsSharedMutableCarrier(t *testing.T) {
	if containsSharedMutable(NewCarrier(TList)) {
		t.Error("a non-concrete carrier must not report shared-mutable state")
	}
}

func TestS6b0FreshenDefaultNilStorePayload(t *testing.T) {
	// A typed-nil *StoreInstanceInfo payload: IsStore matches, AsStore
	// yields a nil pointer — FreshenDefault must return v unchanged.
	v := Value{Parent: TStore, Data: (*StoreInstanceInfo)(nil)}
	out := FreshenDefault(v)
	if out.Data != v.Data {
		t.Error("nil store payload must pass through unchanged")
	}
}

func TestS6b0FreshenDefaultNilBackedFlexList(t *testing.T) {
	// NewFlexList(nil) has a nil element store: the list arm's IsNil
	// guard returns the value unchanged.
	v := NewFlexList(nil)
	out := FreshenDefault(v)
	if out.Data != v.Data {
		t.Error("nil-backed flex list must pass through unchanged")
	}
}

func TestS6b0FreshenDefaultFlexMapCopies(t *testing.T) {
	m := NewOrderedMap()
	m.Set("k", NewInteger(3))
	v := NewFlexMap(m)
	out := FreshenDefault(v)
	if out.Data == v.Data {
		t.Fatal("flex map default must be freshened to a new payload")
	}
	om, err := AsMutableMap(out)
	if err != nil {
		t.Fatalf("freshened copy unreadable: %v", err)
	}
	if om == m {
		t.Error("freshened copy must not alias the schema map")
	}
	got, _ := om.Get("k")
	if n, err := AsInteger(got); err != nil || n != 3 {
		t.Errorf("freshened entry = %v (%v), want 3", got, err)
	}
}

// --- MakeClassFieldValue ------------------------------------------------------

func TestS6b0MakeClassFieldValueClassConstraintRejects(t *testing.T) {
	r := newTestRegistry(t)
	ct := NewClassType(TClass, ClassTypeInfo{Name: "Ideal/S6b0K", Fields: NewOrderedMap()})
	_, err := MakeClassFieldValue(NewInteger(1), ct, r)
	if err == nil || !strings.Contains(err.Error(), "instance") {
		t.Fatalf("expected an instance-required error, got %v", err)
	}
}

// --- MakeHandler --------------------------------------------------------------

func TestS6b0MakeHandlerOptionsCarrierSource(t *testing.T) {
	_, err := MakeHandler([]Value{NewTypeLiteral(TOptions), NewCarrier(TMap)}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Options requires a concrete map") {
		t.Fatalf("expected Options concrete-map error, got %v", err)
	}
}

func TestS6b0MakeHandlerOptionsRecordPayloadSource(t *testing.T) {
	// Parent is exactly Map and concrete, but the payload is a record
	// type body: AsMutableMap declines.
	src := NewRecordType(NewOrderedMap())
	_, err := MakeHandler([]Value{NewTypeLiteral(TOptions), src}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Options requires a concrete map") {
		t.Fatalf("expected Options concrete-map error, got %v", err)
	}
}

func TestS6b0MakeHandlerConformingSourcePassesThrough(t *testing.T) {
	out, err := MakeHandler([]Value{NewTypeLiteral(TInteger), NewInteger(5)}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("MakeHandler: %v %v", out, err)
	}
	if n, err := AsInteger(out[0]); err != nil || n != 5 {
		t.Errorf("conforming source must pass through, got %v", out[0])
	}
}

func TestS6b0MakeHandlerUserRefinementConstructs(t *testing.T) {
	r := newTestRegistry(t)
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "S6b0Pos", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	entry, ok := r.Defs.TopEntry("S6b0Pos")
	if !ok || entry.TypeDef == nil {
		t.Fatal("S6b0Pos not installed with a minted type")
	}
	target := NewTypeLiteral(entry.TypeDef)

	// Non-conforming convertible source: cast to the base then reparent.
	out, err := MakeHandler([]Value{target, NewString("7")}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make S6b0Pos \"7\": %v %v", out, err)
	}
	if n, err := AsInteger(out[0]); err != nil || n != 7 {
		t.Errorf("converted payload = %v (%v), want 7", out[0], err)
	}
	if !out[0].Is(entry.TypeDef) {
		t.Error("result must carry the refinement tag")
	}

	// Unconvertible source: MakeConvert's error surfaces.
	if _, err := MakeHandler([]Value{target, NewString("zz")}, nil, nil, r); err == nil {
		t.Error("expected a conversion error for a non-numeric source")
	}
}

// --- MakeTableR guard arms ------------------------------------------------------

func s6b0TT() TableTypeInfo {
	return TableTypeInfo{Record: s6b0Rec("a", NewTypeLiteral(TInteger))}
}

func TestS6b0MakeTableGuards(t *testing.T) {
	cases := []struct {
		name string
		src  Value
		want string
	}{
		{"carrier source", NewCarrier(TList), "concrete list"},
		{"non-list row", NewList([]Value{NewInteger(1)}), "row 0 must be a list"},
		{"carrier row", NewList([]Value{NewCarrier(TList)}), "row 0 must be a concrete list"},
		{"mixed named row", NewList([]Value{NewList([]Value{s6b0Map("a", NewInteger(1)), NewInteger(2)})}), "mixed named and positional"},
		{"carrier pair row", NewList([]Value{NewList([]Value{s6b0Map("a", NewInteger(1)), NewCarrier(TMap)})}), "concrete map pair"},
		{"named field conversion", NewList([]Value{NewList([]Value{s6b0Map("a", NewString("x"))})}), `field "a"`},
		{"first elem map carrier resets named", NewList([]Value{NewList([]Value{NewCarrier(TMap)})}), `field "a"`},
	}
	for _, tc := range cases {
		_, err := MakeTable(s6b0TT(), tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestS6b0MakeTableNamedMissingField(t *testing.T) {
	tt := TableTypeInfo{Record: s6b0Rec("a", NewTypeLiteral(TInteger), "b", NewTypeLiteral(TInteger))}
	src := NewList([]Value{NewList([]Value{s6b0Map("b", NewInteger(2))})})
	_, err := MakeTable(tt, src)
	if err == nil || !strings.Contains(err.Error(), `missing field "a"`) {
		t.Fatalf("expected missing-field error, got %v", err)
	}
}

func TestS6b0MakeTableNamedUnknownField(t *testing.T) {
	src := NewList([]Value{NewList([]Value{s6b0Map("a", NewInteger(1), "zz", NewInteger(2))})})
	_, err := MakeTable(s6b0TT(), src)
	if err == nil || !strings.Contains(err.Error(), `unknown field "zz"`) {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

// --- kernel Ideal Instantiate guard arms ------------------------------------------

func TestS6b0IdealInstantiateGuards(t *testing.T) {
	r := newTestRegistry(t)

	obj := r.Ideals.Get("Object")
	if obj == nil || obj.Instantiate == nil {
		t.Fatal("Object ideal missing")
	}
	if _, err := obj.Instantiate(NewInteger(1), s6b0Map(), r); err == nil ||
		!strings.Contains(err.Error(), "expected a class type") {
		t.Errorf("Object.Instantiate wrong-typ guard: %v", err)
	}

	res := r.Ideals.Get("Resource")
	if res == nil || res.Instantiate == nil {
		t.Fatal("Resource ideal missing")
	}
	if _, err := res.Instantiate(NewInteger(1), s6b0Map(), r); err == nil ||
		!strings.Contains(err.Error(), "expected a Resource/Entity type") {
		t.Errorf("Resource.Instantiate wrong-typ guard: %v", err)
	}

	resType := NewResourceType(TMap, ResourceTypeInfo{Name: "S6b0RT", Fields: NewOrderedMap()})
	if _, err := res.Instantiate(resType, NewInteger(1), r); err == nil ||
		!strings.Contains(err.Error(), "resource values must be a map") {
		t.Errorf("Resource.Instantiate non-map data guard: %v", err)
	}
	if _, err := res.Instantiate(resType, NewCarrier(TMap), r); err == nil ||
		!strings.Contains(err.Error(), "expected concrete map") {
		t.Errorf("Resource.Instantiate carrier data guard: %v", err)
	}
}

func TestS6b0MakeBareRecordAndTableTargets(t *testing.T) {
	r := newTestRegistry(t)
	if _, err := MakeHandler([]Value{NewTypeLiteral(TRecord), s6b0Map()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "constructed record type") {
		t.Errorf("bare Record target: %v", err)
	}
	if _, err := MakeHandler([]Value{NewTypeLiteral(TTable), NewList(nil)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "constructed table type") {
		t.Errorf("bare Table target: %v", err)
	}
}

// --- MakeWithOpts operand swap ------------------------------------------------------

func TestS6b0MakeWithOptsSwapsListSourceAndMapOpts(t *testing.T) {
	r := newTestRegistry(t)
	recType := NewRecordType(func() *OrderedMap {
		m := NewOrderedMap()
		m.Set("a", NewTypeLiteral(TInteger))
		return m
	}())
	// Positional arrival: [target, map, list] — the map lands in the src
	// slot first and must swap with the list.
	out, err := MakeWithOpts([]Value{recType, s6b0Map(), NewList([]Value{NewInteger(1)})}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("MakeWithOpts: %v %v", out, err)
	}
	m, err := AsMap(out[0])
	if err != nil {
		t.Fatalf("result not a map: %v", err)
	}
	got, _ := m.Get("a")
	if n, err := AsInteger(got); err != nil || n != 1 {
		t.Errorf("record field a = %v (%v), want 1", got, err)
	}
}

// --- MakeObjHandler ---------------------------------------------------------------------

func TestS6b0MakeObjHandlerUnavailableKind(t *testing.T) {
	r := newTestRegistry(t)
	r.Ideals.Register(&Ideal{
		Name:    "S6b0Kind",
		Enabled: false,
		Accepts: func(v Value) bool { return IsAtom(v) },
	})
	_, err := MakeObjHandler([]Value{NewAtom("s6b0kind"), s6b0Map()}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "S6b0Kind type-kind is not available") {
		t.Fatalf("expected unavailable-kind error, got %v", err)
	}
}

func TestS6b0MakeObjHandlerNilRegistryClassPath(t *testing.T) {
	ct := NewClassType(TClass, ClassTypeInfo{Name: "Ideal/S6b0NR", Fields: NewOrderedMap()})
	out, err := MakeObjHandler([]Value{ct, s6b0Map()}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("MakeObjHandler nil-reg: %v %v", out, err)
	}
	if !IsClassInstance(out[0]) {
		t.Errorf("expected a class instance, got %v", out[0])
	}
}

// --- ResolveFieldType -----------------------------------------------------------------------

func TestS6b0ResolveFieldTypeWordUnbound(t *testing.T) {
	r := newTestRegistry(t)
	w := NewWord("S6b0Und")
	out := ResolveFieldType(r, w)
	if !IsWord(&out) {
		t.Errorf("unbound capitalised word must pass through, got %v", out)
	}
}
