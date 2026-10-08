package core

import (
	"strings"
	"testing"
)

// Stage-5 coverage for core_make.go: the record None-default fill, class
// defaults and field errors, resource missing-field, the shared-mutable
// walk and FreshenDefault's FlexList arm, MakeClassFieldValue's default /
// class-field arms, MakePathon's string entry, MakeHandler's swap /
// schema / unavailable-Ideal / Options / convert-error arms, the Ideal
// Instantiate successes, MakeWithOpts and MakeObjHandler fallthroughs,
// MakeScalarHandler's Ideal dispatch and refine-conversion failure,
// MakeConvert's Float/Number/Boolean/Atom arms, MakeFieldValueR's
// convert / unify arms, and ResolveFieldType's resolution cascade.

// --- MakeRecord / class / resource ----------------------------------------

func TestWt5MakeRecordNoneAdmittingDefault(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("opt", NewTypeLiteral(TNone))
	rec := RecordTypeInfo{Fields: fields}
	out, err := MakeRecord(rec, NewMap(NewOrderedMap()), false)
	if err != nil {
		t.Fatalf("MakeRecord: %v", err)
	}
	m, _ := AsMap(out[0])
	v, ok := m.Get("opt")
	if !ok || !IsNoneShape(v) {
		t.Fatalf("a None-admitting missing field must fill with none, got %v", v)
	}
}

func TestWt5MakeClassInstanceDefaultsAndErrors(t *testing.T) {
	r := newTestRegistry(t)

	// A concrete default fills the omitted field.
	fields := NewOrderedMap()
	fields.Set("n", NewInteger(7))
	withDefault := ClassTypeInfo{Name: "Wt5CDef", Fields: fields}
	inst, err := MakeClassInstance(withDefault, NewOrderedMap(), r)
	if err != nil {
		t.Fatalf("MakeClassInstance: %v", err)
	}
	ci, err := AsClassInstance(inst)
	if err != nil {
		t.Fatalf("not a class instance: %v", err)
	}
	if v, _ := ci.Fields.Get("n"); !ValuesEqual(v, NewInteger(7)) {
		t.Fatalf("default not filled, got %v", v)
	}

	// A non-conforming provided value is a loud field error, and
	// MakeClassInstance propagates it.
	typed := NewOrderedMap()
	typed.Set("n", NewTypeLiteral(TInteger))
	strict := ClassTypeInfo{Name: "Wt5CStrict", Fields: typed}
	provided := NewOrderedMap()
	provided.Set("n", NewString("s"))
	if _, err := MakeClassInstance(strict, provided, r); err == nil ||
		!strings.Contains(err.Error(), `field "n"`) {
		t.Fatalf("non-conforming field must error, got %v", err)
	}
}

func TestWt5MakeResourceMissingField(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	res := ResourceTypeInfo{Name: "Wt5Res", Fields: fields}
	if _, err := MakeResource(res, NewOrderedMap(), r); err == nil ||
		!strings.Contains(err.Error(), `missing field "x"`) {
		t.Fatalf("undefaulted missing field must error, got %v", err)
	}
}

// --- shared-mutable walk / FreshenDefault ----------------------------------

func TestWt5ContainsSharedMutableMapWalk(t *testing.T) {
	if !containsSharedMutable(mapOf("k", NewFlexList([]Value{NewInteger(1)}))) {
		t.Fatal("a map holding a flex node is shared-mutable")
	}
	if containsSharedMutable(mapOf("k", NewInteger(1))) {
		t.Fatal("a map of immutable scalars is not shared-mutable")
	}
}

func TestWt5FreshenDefaultFlexList(t *testing.T) {
	orig := NewFlexList([]Value{NewInteger(1)})
	fresh := FreshenDefault(orig)
	if !IsFlexList(fresh) {
		t.Fatalf("freshened flex list must stay flex, got %v", fresh)
	}
	od, _ := orig.Data.(*FlexListData)
	fd, _ := fresh.Data.(*FlexListData)
	if od == nil || fd == nil || od == fd {
		t.Fatal("freshening must produce a new FlexListData identity")
	}
}

// --- MakeClassFieldValue ---------------------------------------------------

func TestWt5MakeClassFieldValueArms(t *testing.T) {
	r := newTestRegistry(t)

	// A concrete default constrains the field to the default's own type.
	if got, err := MakeClassFieldValue(NewInteger(9), NewInteger(5), r); err != nil ||
		!ValuesEqual(got, NewInteger(9)) {
		t.Fatalf("conforming value vs concrete default = %v %v", got, err)
	}
	if _, err := MakeClassFieldValue(NewString("s"), NewInteger(5), r); err == nil ||
		!strings.Contains(err.Error(), "the default's type") {
		t.Fatalf("non-conforming value vs concrete default must error, got %v", err)
	}

	// A class-typed field admits an instance of that class.
	ct := r.Types.MintType("Wt5Inner", TClass)
	cinfo := ClassTypeInfo{Name: "Class/Wt5Inner", Type: ct, Fields: NewOrderedMap()}
	constraint := NewClassType(ct, cinfo)
	inst := NewClassInstance(ct, ClassInstanceInfo{TypeRef: &cinfo, Fields: NewOrderedMap()})
	if got, err := MakeClassFieldValue(inst, constraint, r); err != nil || !got.Parent.Equal(ct) {
		t.Fatalf("class instance vs its class constraint = %v %v", got, err)
	}
}

// --- Pathon ----------------------------------------------------------------

func TestWt5NewPathonFromString(t *testing.T) {
	v := NewPathonFromString("/a/b")
	p, ok := v.Data.(PathonPayload)
	if !ok || !p.Info.Abs || len(p.Info.Parts) != 2 || p.Info.Parts[0] != "a" {
		t.Fatalf("NewPathonFromString = %+v", v.Data)
	}
}

// --- MakeHandler arms ------------------------------------------------------

func TestWt5MakeHandlerSwapAndConvertError(t *testing.T) {
	// Reversed operand order swaps target and source.
	out, err := MakeHandler([]Value{NewInteger(3), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil || len(out) != 1 || !ValuesEqual(out[0], NewInteger(3)) {
		t.Fatalf("swapped make = %v %v", out, err)
	}
	// An unconvertible source surfaces MakeConvert's error.
	if _, err := MakeHandler([]Value{NewTypeLiteral(TInteger), NewString("abc")}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "cannot convert") {
		t.Fatalf("unconvertible source must error, got %v", err)
	}
}

// wt5Schema installs `def NAME gen [T] refine Record {value:T}` by hand
// and returns the shared schema payload (the w4bSchema pattern).
func wt5Schema(t *testing.T, r *Registry, name string) *TypeSchemaInfo {
	t.Helper()
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	fields := NewOrderedMap()
	fields.Set("value", NewTypeLiteral(spec.Params[0].Node))
	body := NewRecordType(fields)
	PopGenBindings(r, spec)
	info := &TypeSchemaInfo{Params: spec.Params, Body: body, Kind: SchemaRecord}
	if err := InstallType(r, name, NewTypeSchema(TType, info)); err != nil {
		t.Fatalf("InstallType %s: %v", name, err)
	}
	return info
}

func TestWt5MakeHandlerSchemaTarget(t *testing.T) {
	r := newTestRegistry(t)
	info := wt5Schema(t, r, "Wt5MkBox")
	schema := NewTypeSchema(info.Type, info)

	// Inference from the construction body, then ordinary instantiation.
	out, err := MakeHandler([]Value{schema, mapOf("value", NewInteger(42))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("schema make = %v %v", out, err)
	}
	m, err := AsMap(out[0])
	if err != nil {
		t.Fatalf("schema make result not a map: %v", err)
	}
	if v, _ := m.Get("value"); !ValuesEqual(v, NewInteger(42)) {
		t.Fatalf("schema make value = %v", v)
	}

	// An uninferable parameter is a loud unbound_param.
	if _, err := MakeHandler([]Value{schema, NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "cannot be inferred") {
		t.Fatalf("uninferable schema make must error, got %v", err)
	}
}

func TestWt5MakeHandlerUnavailableIdeal(t *testing.T) {
	r := newTestRegistry(t)
	mark := r.Types.MintType("Wt5Mark", TScalar)
	r.Ideals.Register(&Ideal{
		Name:    "Wt5Kind",
		Enabled: false,
		Accepts: func(v Value) bool { return v.ID != "" && v.ID == mark.ID },
	})
	if _, err := MakeHandler([]Value{NewTypeLiteral(mark), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "not available in this registry") {
		t.Fatalf("a disabled claiming kind must decline, got %v", err)
	}
}

func TestWt5MakeHandlerOptionsTarget(t *testing.T) {
	r := newTestRegistry(t)
	src := mapOf("a", NewTypeLiteral(TInteger), "b", NewInteger(2))
	out, err := MakeHandler([]Value{NewTypeLiteral(TOptions), src}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsOptionsType(out[0]) {
		t.Fatalf("make Options = %v %v", out, err)
	}
	oi, _ := AsOptionsType(out[0])
	if _, ok := oi.Fields.Get("a"); !ok {
		t.Fatal("options fields must carry the resolved constraints")
	}
}

func TestWt5MakeIdealInstantiateSuccesses(t *testing.T) {
	r := newTestRegistry(t)

	// Resource kind.
	rfields := NewOrderedMap()
	rfields.Set("a", NewTypeLiteral(TInteger))
	resType := NewResourceType(TResourceEntity, ResourceTypeInfo{Name: "Wt5ResT", Fields: rfields})
	out, err := MakeHandler([]Value{resType, mapOf("a", NewInteger(1))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make Resource = %v %v", out, err)
	}
	ri, err := AsResourceInstance(out[0])
	if err != nil {
		t.Fatalf("not a resource instance: %v", err)
	}
	if v, _ := ri.Fields.Get("a"); !ValuesEqual(v, NewInteger(1)) {
		t.Fatalf("resource field = %v", v)
	}

	// Record kind.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	out, err = MakeHandler([]Value{NewRecordType(fields), mapOf("a", NewInteger(2))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make Record = %v %v", out, err)
	}
	m, _ := AsMap(out[0])
	if v, _ := m.Get("a"); !ValuesEqual(v, NewInteger(2)) {
		t.Fatalf("record field = %v", v)
	}

	// Table kind.
	tt := NewTableType(RecordTypeInfo{Fields: fields})
	rows := NewList([]Value{NewList([]Value{NewInteger(3)})})
	out, err = MakeHandler([]Value{tt, rows}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("make Table = %v %v", out, err)
	}
	lst, _ := AsList(out[0])
	if lst.Len() != 1 {
		t.Fatalf("table rows = %v", out[0])
	}
	row, _ := AsMap(lst.Get(0))
	if v, _ := row.Get("a"); !ValuesEqual(v, NewInteger(3)) {
		t.Fatalf("table row field = %v", v)
	}
}

func TestWt5MakeWithOptsFallthrough(t *testing.T) {
	r := newTestRegistry(t)
	out, err := MakeWithOpts([]Value{NewTypeLiteral(TInteger), NewInteger(5), NewMap(NewOrderedMap())}, nil, nil, r)
	if err != nil || len(out) != 1 || !ValuesEqual(out[0], NewInteger(5)) {
		t.Fatalf("3-arg scalar make must defer to the generic path, got %v %v", out, err)
	}
}

func TestWt5MakeScalarHandlerIdealAndConvertError(t *testing.T) {
	r := newTestRegistry(t)

	// A target claimed by a registered Ideal kind routes through it.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	out, err := MakeScalarHandler([]Value{NewRecordType(fields), mapOf("a", NewInteger(4))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("scalar-handler Ideal dispatch = %v %v", out, err)
	}
	m, _ := AsMap(out[0])
	if v, _ := m.Get("a"); !ValuesEqual(v, NewInteger(4)) {
		t.Fatalf("record field = %v", v)
	}

	// A user refinement whose base conversion fails surfaces the error.
	foo := r.Types.MintType("Wt5PosB", TInteger)
	if _, err := MakeScalarHandler([]Value{NewTypeLiteral(foo), NewString("abc")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "cannot convert") {
		t.Fatalf("refinement base-conversion failure must error, got %v", err)
	}
}

func TestWt5MakeObjHandlerFallthrough(t *testing.T) {
	r := newTestRegistry(t)
	src := mapOf("a", NewTypeLiteral(TString))
	out, err := MakeObjHandler([]Value{NewTypeLiteral(TOptions), src}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsOptionsType(out[0]) {
		t.Fatalf("non-object IdealType target must defer to make, got %v %v", out, err)
	}
}

// --- MakeConvert -----------------------------------------------------------

func TestWt5MakeConvertArms(t *testing.T) {
	if v, err := MakeConvert(NewString("2.5"), TFloat); err != nil || !ValuesEqual(v, NewFloat(2.5)) {
		t.Fatalf("float convert = %v %v", v, err)
	}
	if _, err := MakeConvert(NewString("xy"), TFloat); err == nil {
		t.Fatal("unparsable float must error")
	}
	// A float-shaped string converts to Integer via the float fallback.
	if v, err := MakeConvert(NewString("2.5"), TInteger); err != nil || !ValuesEqual(v, NewInteger(2)) {
		t.Fatalf("number convert via float = %v %v", v, err)
	}
	// Boolean is truthiness coercion, never content parsing.
	if v, err := MakeConvert(NewInteger(0), TBoolean); err != nil || !ValuesEqual(v, NewBoolean(false)) {
		t.Fatalf("boolean coercion of 0 = %v %v", v, err)
	}
	if v, err := MakeConvert(NewString("false"), TBoolean); err != nil || !ValuesEqual(v, NewBoolean(true)) {
		t.Fatalf("boolean coercion of a non-empty string = %v %v", v, err)
	}
	if v, err := MakeConvert(NewInteger(7), TAtom); err != nil || !ValuesEqual(v, NewAtom("7")) {
		t.Fatalf("atom convert = %v %v", v, err)
	}
}

// --- MakeFieldValueR -------------------------------------------------------

func TestWt5MakeFieldValueRArms(t *testing.T) {
	// A non-numeric constraint still coerces via MakeConvert.
	if v, err := MakeFieldValueR(NewInteger(7), NewTypeLiteral(TString), nil); err != nil ||
		!ValuesEqual(v, NewString("7")) {
		t.Fatalf("string field convert = %v %v", v, err)
	}
	// A DepScalar constraint routes through UnifyExplainR.
	cons := NewDepScalar(DepGT, NewInteger(10))
	if v, err := MakeFieldValueR(NewInteger(20), cons, nil); err != nil ||
		!ValuesEqual(v, NewInteger(20)) {
		t.Fatalf("in-range DepScalar field = %v %v", v, err)
	}
	if _, err := MakeFieldValueR(NewInteger(5), cons, nil); err == nil ||
		!strings.Contains(err.Error(), "does not match constraint") {
		t.Fatalf("out-of-range DepScalar field must error, got %v", err)
	}
}

// --- ResolveFieldType ------------------------------------------------------

func TestWt5ResolveFieldTypeArms(t *testing.T) {
	r := newTestRegistry(t)

	// A capitalised string naming a def'd type body resolves through
	// the value-binding arm.
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	recBody := NewRecordType(fields)
	r.Defs.Push("Wt5RTName", recBody)
	defer r.Defs.Pop("Wt5RTName")
	if got := ResolveFieldType(r, NewString("Wt5RTName")); !ValuesEqual(got, recBody) {
		t.Fatalf("def-bound type name = %v", got)
	}

	// A bare Word resolves through the builtin arm.
	got := ResolveFieldType(r, NewWord("Integer"))
	if !IsBareTypeNode(got) || !(&got).Equal(TInteger) {
		t.Fatalf("builtin word = %v", got)
	}

	// Typed containers resolve deep through the registry-armed resolver.
	tl := NewTypedList(NewTypeLiteral(TInteger))
	if got := ResolveFieldType(r, tl); !IsTypedList(got) {
		t.Fatalf("typed list constraint = %v", got)
	}
}
