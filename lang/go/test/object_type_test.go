package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// objSchema resolves an evaluated class NAME — its NODE, post the
// Stage 2 flip of design/legacy/TYPE-REPRESENTATION.1.ignore — to the class
// schema the node records.
func objSchema(v native.Value) native.Value {
	if body, ok := native.TypeContentOf(v); ok {
		return body
	}
	return v
}

// TestObjectTypeDefine defines a named object type and verifies its structure.
// def Foo type Object {a:String,b:Boolean} → Class/Foo with fields a and b
func TestObjectTypeDefine(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String,b:Boolean}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// The name DENOTES its node (design/legacy/TYPE-REPRESENTATION.1.ignore
	// Stage 2): evaluating `Foo` yields the Foo node, and the schema
	// stays recoverable from it.
	if s := result[0].String(); s != "Foo" {
		t.Errorf("expected the Foo node, got %s", s)
	}
	body, ok := native.TypeContentOf(result[0])
	if !ok {
		t.Fatal("the node must record its class schema")
	}
	s := body.String()
	if !strings.Contains(s, "Class/Foo") {
		t.Errorf("expected type name to contain 'Class/Foo', got %s", s)
	}
	if !strings.Contains(s, "a:String") {
		t.Errorf("expected field a:String, got %s", s)
	}
	if !strings.Contains(s, "b:Boolean") {
		t.Errorf("expected field b:Boolean, got %s", s)
	}
}

// TestObjectTypeAnonymous creates an anonymous class type.
// class {c:99} → anonymous class type named Ideal/Class/<internal-id>
func TestObjectTypeAnonymous(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`class {c:99}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	s := result[0].String()
	if !strings.Contains(s, "Class/T_") {
		t.Errorf("expected anonymous class type with Class/T_ prefix, got %s", s)
	}
	if !strings.Contains(s, "c:") {
		t.Errorf("expected field c, got %s", s)
	}
}

// TestObjectTypeInheritance defines a child object type that inherits fields.
// def Bar type Foo {d:Integer} → Class/Foo/Bar with fields a,b,d
func TestObjectTypeInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String,b:Boolean}`,
		`def Bar refine Foo {d:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// The name denotes its node; the schema is the node's content.
	if s := result[0].String(); s != "Bar" {
		t.Errorf("expected the Bar node, got %s", s)
	}
	body, ok := native.TypeContentOf(result[0])
	if !ok {
		t.Fatal("the node must record its class schema")
	}
	s := body.String()
	if !strings.Contains(s, "Class/Foo/Bar") {
		t.Errorf("expected type name to contain 'Class/Foo/Bar', got %s", s)
	}
	// Should have inherited fields a,b from Foo plus own field d
	if !strings.Contains(s, "a:String") {
		t.Errorf("expected inherited field a:String, got %s", s)
	}
	if !strings.Contains(s, "b:Boolean") {
		t.Errorf("expected inherited field b:Boolean, got %s", s)
	}
	if !strings.Contains(s, "d:Integer") {
		t.Errorf("expected own field d:Integer, got %s", s)
	}
}

// TestObjectTypeParentFields verifies that parent fields are accessible
// through AllFields on the child type.
func TestObjectTypeParentFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String,b:Boolean}`,
		`def Bar refine Foo {d:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := native.TypeContentOf(result[0])
	ot, _ := native.AsClassType(schema)
	all := ot.AllFields()
	if all.Len() != 3 {
		t.Fatalf("expected 3 total fields (a,b,d), got %d", all.Len())
	}
	keys := all.Keys()
	// Parent fields come first (a,b), then own (d)
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "d" {
		t.Errorf("expected field order [a,b,d], got %v", keys)
	}
}

// TestObjectTypeOwnFieldsOnly verifies that own fields do not include inherited.
func TestObjectTypeOwnFieldsOnly(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String,b:Boolean}`,
		`def Bar refine Foo {d:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := native.TypeContentOf(result[0])
	ot, _ := native.AsClassType(schema)
	if ot.Fields.Len() != 1 {
		t.Fatalf("expected 1 own field (d), got %d", ot.Fields.Len())
	}
	keys := ot.Fields.Keys()
	if keys[0] != "d" {
		t.Errorf("expected own field 'd', got %s", keys[0])
	}
}

// TestObjectTypeDeepInheritance tests three-level inheritance: Foo → Bar → Baz.
func TestObjectTypeDeepInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar refine Foo {b:Integer}`,
		`def Baz refine Bar {c:Boolean}`,
		`Baz`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// The name denotes its node; the schema is the node's content.
	schema, ok := native.TypeContentOf(result[0])
	if !ok {
		t.Fatal("the node must record its class schema")
	}
	s := schema.String()
	if !strings.Contains(s, "Class/Foo/Bar/Baz") {
		t.Errorf("expected type name 'Class/Foo/Bar/Baz', got %s", s)
	}
	ot, _ := native.AsClassType(schema)
	all := ot.AllFields()
	if all.Len() != 3 {
		t.Fatalf("expected 3 fields (a,b,c), got %d", all.Len())
	}
	keys := all.Keys()
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected field order [a,b,c], got %v", keys)
	}
}

// TestObjectTypeUniqueID verifies that each object type gets a unique ID.
func TestObjectTypeUniqueID(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar class {b:String}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp1, _ := native.AsClassType(objSchema(result[0]))
	fooID := _tmp1.ID

	result2, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar class {b:String}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp2, _ := native.AsClassType(objSchema(result2[0]))
	barID := _tmp2.ID

	if fooID == barID {
		t.Errorf("expected different IDs for Foo and Bar, both got %s", fooID)
	}
	if !strings.HasPrefix(fooID, "T_") {
		t.Errorf("expected ID to start with 'T_', got %s", fooID)
	}
	if len(fooID) != 14 { // "T_" + 12 hex chars
		t.Errorf("expected ID length 14, got %d for %s", len(fooID), fooID)
	}
}

// TestObjectTypeParentIsNilForRoot verifies that a root object type has no parent.
func TestObjectTypeParentIsNilForRoot(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`Foo`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot, _ := native.AsClassType(objSchema(result[0]))
	if ot.Parent != nil {
		t.Errorf("expected nil parent for root object type, got %+v", ot.Parent)
	}
	if ot.Name != "Class/Foo" {
		t.Errorf("expected name 'Class/Foo', got %s", ot.Name)
	}
}

// TestObjectTypeParentReference verifies the parent reference in a child type.
func TestObjectTypeParentReference(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar refine Foo {b:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot, _ := native.AsClassType(objSchema(result[0]))
	if ot.Parent == nil {
		t.Fatal("expected non-nil parent for child object type")
	}
	if ot.Parent.Name != "Class/Foo" {
		t.Errorf("expected parent name 'Class/Foo', got %s", ot.Parent.Name)
	}
}

// TestObjectTypeFieldOverride verifies that a child can narrow parent fields.
func TestObjectTypeFieldOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:Number,b:Boolean}`,
		`def Bar refine Foo {a:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ot, _ := native.AsClassType(objSchema(result[0]))
	all := ot.AllFields()
	// a should be narrowed to Integer, b inherited as Boolean
	if all.Len() != 2 {
		t.Fatalf("expected 2 fields (a,b), got %d", all.Len())
	}
	aVal, _ := all.Get("a")
	if !strings.Contains(aVal.String(), "Integer") {
		t.Errorf("expected narrowed field a to be Integer, got %s", aVal.String())
	}
}

// TestObjectTypeVTypeMatches verifies Parent hierarchy matching.
func TestObjectTypeVTypeMatches(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar refine Foo {b:Integer}`,
		`Bar`,
	})
	if err != nil {
		t.Fatal(err)
	}
	barType := result[0].Parent
	// Bar (Class/Foo/Bar) roots under Class.
	tCls, _ := native.NewType("Class")
	if !barType.ConformsTo(tCls) {
		t.Error("Class/Foo/Bar should match Class")
	}
	// Bar (Class/Foo/Bar) should match Class/Foo
	tObjFoo, _ := native.NewType("Class/Foo")
	if !barType.ConformsTo(tObjFoo) {
		t.Error("Class/Foo/Bar should match Class/Foo")
	}
	// Bar (Class/Foo/Bar) should match Class/Foo/Bar
	tObjFooBar, _ := native.NewType("Class/Foo/Bar")
	if !barType.ConformsTo(tObjFooBar) {
		t.Error("Class/Foo/Bar should match Class/Foo/Bar")
	}
}

// TestBuiltinTypeFixedIDs verifies that builtin types have stable, fixed IDs.
func TestBuiltinTypeFixedIDs(t *testing.T) {
	// Builtin types must have non-empty fixed IDs
	if native.TAny.ID == "" {
		t.Error("TAny should have a fixed ID")
	}
	if native.TString.ID == "" {
		t.Error("TString should have a fixed ID")
	}
	if native.TList.ID == "" {
		t.Error("TList should have a fixed ID")
	}
	if native.TWord.ID == "" {
		t.Error("TWord should have a fixed ID")
	}
	if native.TClass.ID == "" {
		t.Error("TClass should have a fixed ID")
	}

	// Fixed IDs must be 14 chars (prefix + 12 hex)
	if len(native.TAny.ID) != 14 {
		t.Errorf("TAny ID should be 14 chars, got %d: %s", len(native.TAny.ID), native.TAny.ID)
	}

	// Correct prefixes
	if !strings.HasPrefix(native.TAny.ID, "T_") {
		t.Errorf("TAny ID should start with T_, got %s", native.TAny.ID)
	}
	if !strings.HasPrefix(native.TString.ID, "S_") {
		t.Errorf("TString ID should start with S_, got %s", native.TString.ID)
	}
	if !strings.HasPrefix(native.TList.ID, "N_") {
		t.Errorf("TList ID should start with N_, got %s", native.TList.ID)
	}
	if !strings.HasPrefix(native.TWord.ID, "W_") {
		t.Errorf("TWord ID should start with W_, got %s", native.TWord.ID)
	}
	if !strings.HasPrefix(native.TClass.ID, "T_") {
		t.Errorf("TClass ID should start with T_, got %s", native.TClass.ID)
	}

	// Specific known values: TAny=1, TNone=2, TScalar=3, TString=4
	expectedAny := "T_000000000001"
	if native.TAny.ID != expectedAny {
		t.Errorf("TAny ID should be %s, got %s", expectedAny, native.TAny.ID)
	}
	expectedNone := "T_000000000002"
	if native.TNone.ID != expectedNone {
		t.Errorf("TNone ID should be %s, got %s", expectedNone, native.TNone.ID)
	}
	expectedString := "S_000000000004"
	if native.TString.ID != expectedString {
		t.Errorf("TString ID should be %s, got %s", expectedString, native.TString.ID)
	}

	// IDs are stable across multiple accesses (no regeneration)
	id1 := native.TAny.ID
	id2 := native.TAny.ID
	if id1 != id2 {
		t.Errorf("TAny ID should be stable, got %s then %s", id1, id2)
	}

	// All builtin IDs are unique
	ids := map[string]string{}
	builtins := map[string]*native.Type{
		"TAny": native.TAny, "TNone": native.TNone, "TScalar": native.TScalar,
		"TString": native.TString, "TStringProper": native.TStringProper,
		"TStringEmpty": native.TStringEmpty, "TNumber": native.TNumber,
		"TInteger": native.TInteger, "TFloat": native.TFloat,
		"TBoolean": native.TBoolean, "TNode": native.TNode,
		"TList": native.TList, "TListArgs": native.TListArgs,
		"TMap": native.TMap, "TTable": native.TTable, "TRecord": native.TRecord,
		"TAtom": native.TAtom, "TWord": native.TWord, "TFunction": native.TFunction,
		"TClass": native.TClass,
	}
	for name, typ := range builtins {
		if prev, exists := ids[typ.ID]; exists {
			t.Errorf("duplicate ID: %s and %s both have %s", prev, name, typ.ID)
		}
		ids[typ.ID] = name
	}

	// NewType is strict — unregistered paths error.
	if _, err := native.NewType("String/Custom"); err == nil {
		t.Error("NewType('String/Custom') should error — unknown type")
	}
}

// TestValueIDPrefixes verifies that all value categories get the correct ID
// prefix. IDs are minted only inside a check/compile pass or an explicit mint
// scope (runtime mints elide them — eng mode-gated ID elision), so the test
// arms a scope; the prefix machinery it pins is unchanged.
func TestValueIDPrefixes(t *testing.T) {
	defer native.BeginIDMintScope()()
	// Scalar values get S_ prefix
	str := native.NewString("hello")
	if !strings.HasPrefix(str.ID, "S_") {
		t.Errorf("string ID should start with S_, got %s", str.ID)
	}
	if len(str.ID) != 14 { // "S_" + 12 hex chars
		t.Errorf("string ID should be 14 chars, got %d: %s", len(str.ID), str.ID)
	}

	num := native.NewInteger(42)
	if !strings.HasPrefix(num.ID, "S_") {
		t.Errorf("integer ID should start with S_, got %s", num.ID)
	}

	dec := native.NewFloat(3.14)
	if !strings.HasPrefix(dec.ID, "S_") {
		t.Errorf("decimal ID should start with S_, got %s", dec.ID)
	}

	boolv := native.NewBoolean(true)
	if !strings.HasPrefix(boolv.ID, "S_") {
		t.Errorf("boolean ID should start with S_, got %s", boolv.ID)
	}

	// Node values get N_ prefix
	list := native.NewList([]native.Value{})
	if !strings.HasPrefix(list.ID, "N_") {
		t.Errorf("list ID should start with N_, got %s", list.ID)
	}

	m := native.NewMap(native.NewOrderedMap())
	if !strings.HasPrefix(m.ID, "N_") {
		t.Errorf("map ID should start with N_, got %s", m.ID)
	}

	// Word values get W_ prefix
	word := native.NewWord("Test")
	if !strings.HasPrefix(word.ID, "W_") {
		t.Errorf("word ID should start with W_, got %s", word.ID)
	}

	atom := native.NewAtom("foo")
	if !strings.HasPrefix(atom.ID, "S_") {
		t.Errorf("atom ID should start with S_, got %s", atom.ID)
	}

	// Type/Object values get T_ prefix
	typeLit := native.NewTypeLiteral(native.TString)
	if !strings.HasPrefix(typeLit.ID, "S_") {
		t.Errorf("string type literal ID should start with S_ (type's own category), got %s", typeLit.ID)
	}

	noneLit := native.NewTypeLiteral(native.TNone)
	if !strings.HasPrefix(noneLit.ID, "T_") {
		t.Errorf("none type literal ID should start with T_, got %s", noneLit.ID)
	}

	// All IDs should be unique
	ids := map[string]bool{str.ID: true, num.ID: true, dec.ID: true, boolv.ID: true,
		list.ID: true, m.ID: true, word.ID: true, atom.ID: true}
	if len(ids) != 8 {
		t.Errorf("expected 8 unique IDs, got %d (some duplicates)", len(ids))
	}
}

// --- make object tests ---

// objFields is a test helper that extracts fields from an object instance result.
func objFields(t *testing.T, result []native.Value) *native.OrderedMap {
	t.Helper()
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	v := result[0]
	if !native.IsClassInstance(v) {
		t.Fatalf("expected object instance, got %s", v.String())
	}
	oi, _ := native.AsClassInstance(v)
	return oi.AllFields()
}

// TestMakeObjectBasic creates an object instance with type-literal fields.
func TestMakeObjectBasic(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo {x:"hello"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := result[0]
	if !native.IsClassInstance(inst) {
		t.Fatalf("expected object instance, got %s", inst.String())
	}
	oi, _ := native.AsClassInstance(inst)
	if oi.TypeRef.Name != "Class/Foo" {
		t.Errorf("expected type ref Class/Foo, got %s", oi.TypeRef.Name)
	}
	v, ok := oi.Fields.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	_v3, _ := native.AsString(v)
	if _v3 != "hello" {
		_v4, _ := native.AsString(v)
		t.Errorf("expected x='hello', got %s", _v4)
	}
}

// TestMakeObjectTypeConversion converts field values to match type constraints.
func TestMakeObjectTypeConversion(t *testing.T) {
	// Class fields are STRICT: no silent conversion — providing an
	// Integer to a String field is a loud type error (the legacy
	// object path converted 42 to "42").
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo {x:42}`,
	})
	if err == nil {
		t.Fatal("expected a type error for Integer into a String field")
	}
	if !strings.Contains(err.Error(), "expected String") {
		t.Errorf("expected a field type error, got: %v", err)
	}
}

// TestMakeObjectDefaultValues uses concrete defaults when fields are omitted.
func TestMakeObjectDefaultValues(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, ok := om.Get("x")
	if !ok {
		t.Fatal("missing field x")
	}
	_v7, _ := native.AsInteger(v)
	if _v7 != 1 {
		_v8, _ := native.AsInteger(v)
		t.Errorf("expected x=1 (default), got %d", _v8)
	}
}

// TestMakeObjectOverrideDefault overrides a concrete default with a new value.
func TestMakeObjectOverrideDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {x:2}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	_v9, _ := native.AsInteger(v)
	if _v9 != 2 {
		_v10, _ := native.AsInteger(v)
		t.Errorf("expected x=2, got %d", _v10)
	}
}

// TestMakeObjectMultipleFields handles multiple fields with mixed types.
func TestMakeObjectMultipleFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String,y:Integer}`,
		`make Foo {x:"hi",y:7}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	_v11, _ := native.AsString(x)
	if _v11 != "hi" {
		_v12, _ := native.AsString(x)
		t.Errorf("expected x='hi', got %s", _v12)
	}
	_v13, _ := native.AsInteger(y)
	if _v13 != 7 {
		_v14, _ := native.AsInteger(y)
		t.Errorf("expected y=7, got %d", _v14)
	}
}

// TestMakeObjectMixedDefaultsAndTypes mixes type-literal and concrete-default fields.
func TestMakeObjectMixedDefaultsAndTypes(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String,y:10}`,
		`make Foo {x:"hi"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	_v15, _ := native.AsString(x)
	if _v15 != "hi" {
		_v16, _ := native.AsString(x)
		t.Errorf("expected x='hi', got %s", _v16)
	}
	_v17, _ := native.AsInteger(y)
	if _v17 != 10 {
		_v18, _ := native.AsInteger(y)
		t.Errorf("expected y=10 (default), got %d", _v18)
	}
}

// TestMakeObjectUnknownFieldError rejects unknown fields.
func TestMakeObjectUnknownFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo {x:"hi",z:1}`,
	})
	if err == nil {
		t.Fatal("expected error for unknown field z")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectMissingRequiredFieldError rejects missing type-literal fields.
func TestMakeObjectMissingRequiredFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo {}`,
	})
	if err == nil {
		t.Fatal("expected error for missing required field x")
	}
	if !strings.Contains(err.Error(), "missing field") {
		t.Errorf("expected 'missing field' error, got: %s", err)
	}
}

// TestMakeObjectNonMapSourceError rejects non-map source values.
func TestMakeObjectNonMapSourceError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo [1 2 3]`,
	})
	if err == nil {
		t.Fatal("expected error for non-map source")
	}
	if !strings.Contains(err.Error(), "must be a map") {
		t.Errorf("expected 'must be a map' error, got: %s", err)
	}
}

// TestMakeObjectEmptyMapAllDefaults creates instance with all-default fields.
func TestMakeObjectEmptyMapAllDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1,y:"default"}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	_v19, _ := native.AsInteger(x)
	if _v19 != 1 {
		_v20, _ := native.AsInteger(x)
		t.Errorf("expected x=1, got %d", _v20)
	}
	_v21, _ := native.AsString(y)
	if _v21 != "default" {
		_v22, _ := native.AsString(y)
		t.Errorf("expected y='default', got %s", _v22)
	}
}

// TestMakeObjectInheritedFields creates instance of child type with parent fields.
func TestMakeObjectInheritedFields(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String,b:Integer}`,
		`def Bar refine Foo {c:Boolean}`,
		`make Bar {a:"hi",b:3,c:true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	b, _ := om.Get("b")
	c, _ := om.Get("c")
	_v23, _ := native.AsString(a)
	if _v23 != "hi" {
		_v24, _ := native.AsString(a)
		t.Errorf("expected a='hi', got %s", _v24)
	}
	_v25, _ := native.AsInteger(b)
	if _v25 != 3 {
		_v26, _ := native.AsInteger(b)
		t.Errorf("expected b=3, got %d", _v26)
	}
	if cb, _ := native.AsBoolean(c); !cb {
		t.Error("expected c=true")
	}
}

// TestMakeObjectInheritedDefaults uses parent defaults in child type.
func TestMakeObjectInheritedDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:1,b:2}`,
		`def Bar refine Foo {c:3}`,
		`make Bar {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	b, _ := om.Get("b")
	c, _ := om.Get("c")
	_v27, _ := native.AsInteger(a)
	if _v27 != 1 {
		_v28, _ := native.AsInteger(a)
		t.Errorf("expected a=1, got %d", _v28)
	}
	_v29, _ := native.AsInteger(b)
	if _v29 != 2 {
		_v30, _ := native.AsInteger(b)
		t.Errorf("expected b=2, got %d", _v30)
	}
	_v31, _ := native.AsInteger(c)
	if _v31 != 3 {
		_v32, _ := native.AsInteger(c)
		t.Errorf("expected c=3, got %d", _v32)
	}
}

// TestMakeObjectInheritedUnknownFieldError rejects fields not in parent or child.
func TestMakeObjectInheritedUnknownFieldError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:String}`,
		`def Bar refine Foo {b:Integer}`,
		`make Bar {a:"hi",b:1,z:99}`,
	})
	if err == nil {
		t.Fatal("expected error for unknown field z")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectOverrideInheritedDefault overrides a parent's default in child instance.
func TestMakeObjectOverrideInheritedDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:1}`,
		`def Bar refine Foo {b:String}`,
		`make Bar {a:99,b:"x"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	a, _ := om.Get("a")
	_v33, _ := native.AsInteger(a)
	if _v33 != 99 {
		_v34, _ := native.AsInteger(a)
		t.Errorf("expected a=99, got %d", _v34)
	}
}

// TestMakeObjectStringDefault uses string default value.
func TestMakeObjectStringDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:"hello"}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	_v35, _ := native.AsString(v)
	if _v35 != "hello" {
		_v36, _ := native.AsString(v)
		t.Errorf("expected x='hello', got %s", _v36)
	}
}

// TestMakeObjectStringDefaultOverride overrides string default with different string.
func TestMakeObjectStringDefaultOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:"hello"}`,
		`make Foo {x:"world"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	_v37, _ := native.AsString(v)
	if _v37 != "world" {
		_v38, _ := native.AsString(v)
		t.Errorf("expected x='world', got %s", _v38)
	}
}

// TestMakeObjectBooleanDefault uses boolean default value.
func TestMakeObjectBooleanDefault(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:true}`,
		`make Foo {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if vb, _ := native.AsBoolean(v); !vb {
		t.Error("expected x=true (default)")
	}
}

// TestMakeObjectBooleanDefaultOverride overrides boolean default.
func TestMakeObjectBooleanDefaultOverride(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:true}`,
		`make Foo {x:false}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	v, _ := om.Get("x")
	if vb, _ := native.AsBoolean(v); vb {
		t.Error("expected x=false (overridden)")
	}
}

// TestMakeObjectMultipleInstances creates multiple independent instances.
func TestMakeObjectMultipleInstances(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {x:10}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om1 := objFields(t, result)

	result2, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {x:20}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om2 := objFields(t, result2)

	v1, _ := om1.Get("x")
	v2, _ := om2.Get("x")
	_v39, _ := native.AsInteger(v1)
	_v40, _ := native.AsInteger(v2)
	if _v39 == _v40 {
		t.Error("expected independent instances with different values")
	}
}

// TestMakeObjectOnlyUnknownFieldsError rejects when only unknown fields given.
func TestMakeObjectOnlyUnknownFieldsError(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:String}`,
		`make Foo {z:"hi"}`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected 'unknown field' error, got: %s", err)
	}
}

// TestMakeObjectFieldOrderPreserved verifies field order matches type definition.
func TestMakeObjectFieldOrderPreserved(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:1,b:2,c:3}`,
		`make Foo {c:30,a:10,b:20}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	keys := om.Keys()
	// Fields should be in definition order, not input order.
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected field order [a,b,c], got %v", keys)
	}
}

// TestMakeObjectDeepInheritance tests 3-level inheritance chain.
func TestMakeObjectDeepInheritance(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def A class {x:1}`,
		`def B refine A {y:2}`,
		`def C refine B {z:3}`,
		`make C {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	om := objFields(t, result)
	x, _ := om.Get("x")
	y, _ := om.Get("y")
	z, _ := om.Get("z")
	_v41, _ := native.AsInteger(x)
	_v42, _ := native.AsInteger(y)
	_v43, _ := native.AsInteger(z)
	if _v41 != 1 || _v42 != 2 || _v43 != 3 {
		_v44, _ := native.AsInteger(x)
		_v45, _ := native.AsInteger(y)
		_v46, _ := native.AsInteger(z)
		t.Errorf("expected x=1,y=2,z=3, got x=%d,y=%d,z=%d", _v44, _v45, _v46)
	}
}

// TestMakeObjectChildOverridesParentConcreteRejected tests that a child cannot
// replace one concrete value with a different concrete value (99 vs 1).
func TestMakeObjectChildOverridesParentConcreteRejected(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`def Bar refine Foo {x:99}`,
	})
	if err == nil {
		t.Fatal("expected error: child concrete 99 cannot replace parent concrete 1")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// TestMakeObjectInstanceTypeMatchesObjectType verifies instance type path matches its type.
func TestMakeObjectInstanceTypeMatchesObjectType(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {x:5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := result[0]
	if !inst.Parent.ConformsTo(native.TClass) {
		t.Errorf("expected instance type to match TClass, got %s", inst.Parent)
	}
	oi, _ := native.AsClassInstance(inst)
	if oi.TypeRef.Name != "Class/Foo" {
		t.Errorf("expected TypeRef.Name='Class/Foo', got %s", oi.TypeRef.Name)
	}
}

// TestMakeObjectInstanceChildTypeRef verifies child instance references child type.
func TestMakeObjectInstanceChildTypeRef(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {a:1}`,
		`def Bar refine Foo {b:2}`,
		`make Bar {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	if oi.TypeRef.Name != "Class/Foo/Bar" {
		t.Errorf("expected TypeRef.Name='Class/Foo/Bar', got %s", oi.TypeRef.Name)
	}
	if oi.TypeRef.Parent == nil {
		t.Fatal("expected child TypeRef to have a parent")
	}
	if oi.TypeRef.Parent.Name != "Class/Foo" {
		t.Errorf("expected parent name='Class/Foo', got %s", oi.TypeRef.Parent.Name)
	}
}

// TestMakeObjectInstanceStringFormat verifies the String() representation.
func TestMakeObjectInstanceStringFormat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:1}`,
		`make Foo {x:5}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if !strings.Contains(s, "Class/Foo") {
		t.Errorf("expected String() to contain 'Class/Foo', got %s", s)
	}
	if !strings.Contains(s, "x:5") {
		t.Errorf("expected String() to contain 'x:5', got %s", s)
	}
}

// --- prototype tests ---

// TestMakeObjectPrototypeBasic creates a child instance with an explicit prototype.
func TestMakeObjectPrototypeBasic(t *testing.T) {
	// Class instances are FLAT: inherited fields are provided at make
	// (or defaulted) and resolve into one field map — no prototype.
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {y:String}`,
		`make Bar {x:1, y:"A"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	// Class instances are flat — the ClassInstanceInfo has no Prototype
	// field at all (open objects, which had one, were removed).
	y, _ := oi.Fields.Get("y")
	x, _ := oi.Fields.Get("x")
	if _v, _ := native.AsString(y); _v != "A" {
		t.Errorf("expected y='A', got %s", _v)
	}
	if _v, _ := native.AsInteger(x); _v != 1 {
		t.Errorf("expected x=1 (flat own field), got %d", _v)
	}
}

// TestMakeObjectPrototypeChainRef verifies the prototype pointer is set correctly.
func TestMakeObjectPrototypeChainRef(t *testing.T) {
	// The 3-arg make-with-prototype form is REJECTED for classes:
	// instances are flat, there is no chain to seed.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def foo1 (make Foo {x:42})`,
		`def Bar refine Foo {y:String}`,
		`make Bar {y:"hi"} foo1`,
	})
	if err == nil {
		t.Fatal("expected make-with-prototype on a class to error")
	}
	// Either failure proves the contract: the prototype no longer
	// seeds inherited fields (missing-field), and the explicit
	// 3-arg prototype path is rejected outright (no-prototypes).
	if !strings.Contains(err.Error(), "no prototypes") &&
		!strings.Contains(err.Error(), `missing field "x"`) {
		t.Errorf("expected the prototype path to fail loudly, got: %v", err)
	}
}

// TestMakeObjectAutoPrototypeBaseValues verifies that a child without explicit
// prototype auto-creates a parent instance with base values.
func TestMakeObjectAutoPrototypeBaseValues(t *testing.T) {
	// A TYPED inherited field is REQUIRED — there is no auto-created
	// base-value prototype; omitting it is a loud missing-field error.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {y:String}`,
		`make Bar {y:"test"}`,
	})
	if err == nil {
		t.Fatal("expected missing required inherited field to error")
	}
	if !strings.Contains(err.Error(), `missing field "x"`) {
		t.Errorf("expected missing-field error for x, got: %v", err)
	}
}

// TestMakeObjectAutoPrototypeWithDefaults verifies auto-prototype uses
// concrete defaults from the parent type definition.
func TestMakeObjectAutoPrototypeWithDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:10}`,
		`def Bar refine Foo {y:String}`,
		`make Bar {y:"test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp55, _ := native.AsClassInstance(result[0])
	allF := _tmp55.AllFields()
	x, _ := allF.Get("x")
	_v56, _ := native.AsInteger(x)
	if _v56 != 10 {
		_v57, _ := native.AsInteger(x)
		t.Errorf("expected auto-prototype x=10 (default), got %d", _v57)
	}
}

// TestMakeObjectPrototypeOverrideInherited overrides an inherited field via make source.
func TestMakeObjectPrototypeOverrideInherited(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def foo1 (make Foo {x:1})`,
		`def Bar refine Foo {y:String}`,
		`make Bar {y:"A",x:99} foo1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp58, _ := native.AsClassInstance(result[0])
	allF := _tmp58.AllFields()
	x, _ := allF.Get("x")
	_v59, _ := native.AsInteger(x)
	if _v59 != 99 {
		_v60, _ := native.AsInteger(x)
		t.Errorf("expected x=99 (overridden), got %d", _v60)
	}
}

// TestMakeObjectPrototypeGetField tests GetField on the prototype chain.
func TestMakeObjectPrototypeGetField(t *testing.T) {
	// GetField on a flat class instance is a plain map hit — inherited
	// fields were resolved at make, no chain walk involved.
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {y:String}`,
		`make Bar {x:7, y:"hi"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	x, ok := oi.GetField("x")
	if !ok {
		t.Fatal("expected GetField to find the inherited field flat")
	}
	if _v, _ := native.AsInteger(x); _v != 7 {
		t.Errorf("expected x=7, got %d", _v)
	}
	y, ok := oi.GetField("y")
	if !ok {
		t.Fatal("expected GetField to find y directly")
	}
	if _v, _ := native.AsString(y); _v != "hi" {
		t.Errorf("expected y='hi', got %s", _v)
	}
}

// --- field narrowing tests ---

// TestObjectTypeFieldNarrowingAllowed verifies a child can narrow a parent field.
func TestObjectTypeFieldNarrowingAllowed(t *testing.T) {
	// Integer is narrower than Number — should be allowed.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Number}`,
		`def Bar refine Foo {x:Integer}`,
	})
	if err != nil {
		t.Fatalf("narrowing Number→Integer should be allowed: %s", err)
	}
}

// TestObjectTypeFieldNarrowingConcreteAllowed verifies concrete narrows type literal.
func TestObjectTypeFieldNarrowingConcreteAllowed(t *testing.T) {
	// Concrete 42 narrows Integer — should be allowed.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {x:42}`,
	})
	if err != nil {
		t.Fatalf("narrowing Integer→42 should be allowed: %s", err)
	}
}

// TestObjectTypeFieldExpandingRejected verifies a child cannot expand a parent field type.
func TestObjectTypeFieldExpandingRejected(t *testing.T) {
	// String does not unify with Integer — should be rejected.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {x:String}`,
	})
	if err == nil {
		t.Fatal("expected error for expanding Integer→String")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// TestObjectTypeFieldExpandingConcreteRejected rejects incompatible concrete override.
func TestObjectTypeFieldExpandingConcreteRejected(t *testing.T) {
	// "hello" (string) does not unify with Integer.
	_, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {x:"hello"}`,
	})
	if err == nil {
		t.Fatal("expected error for incompatible concrete override")
	}
	if !strings.Contains(err.Error(), "cannot expand") {
		t.Errorf("expected 'cannot expand' error, got: %s", err)
	}
}

// --- deep inheritance tests (7 levels) ---

// TestObjectTypeDeep7Levels tests 7-level type hierarchy definition.
func TestObjectTypeDeep7Levels(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:Integer}`,
		`def L2 refine L1 {b:String}`,
		`def L3 refine L2 {c:Boolean}`,
		`def L4 refine L3 {d:Integer}`,
		`def L5 refine L4 {e:String}`,
		`def L6 refine L5 {f:Boolean}`,
		`def L7 refine L6 {g:Integer}`,
	})
	if err != nil {
		t.Fatalf("7-level type hierarchy should succeed: %s", err)
	}
}

// TestMakeObjectDeep7LevelsAllDefaults tests 7-level instance with all defaults.
func TestMakeObjectDeep7LevelsAllDefaults(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:1}`,
		`def L2 refine L1 {b:"two"}`,
		`def L3 refine L2 {c:true}`,
		`def L4 refine L3 {d:4}`,
		`def L5 refine L4 {e:"five"}`,
		`def L6 refine L5 {f:false}`,
		`def L7 refine L6 {g:7}`,
		`make L7 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp65, _ := native.AsClassInstance(result[0])
	allF := _tmp65.AllFields()
	checks := map[string]interface{}{
		"a": int64(1), "b": "two", "c": true, "d": int64(4),
		"e": "five", "f": false, "g": int64(7),
	}
	for k, expected := range checks {
		v, ok := allF.Get(k)
		if !ok {
			t.Errorf("missing field %s", k)
			continue
		}
		switch exp := expected.(type) {
		case int64:
			_v66, _ := native.AsInteger(v)
			if _v66 != exp {
				_v67, _ := native.AsInteger(v)
				t.Errorf("field %s: expected %d, got %d", k, exp, _v67)
			}
		case string:
			_v68, _ := native.AsString(v)
			if _v68 != exp {
				_v69, _ := native.AsString(v)
				t.Errorf("field %s: expected %q, got %q", k, exp, _v69)
			}
		case bool:
			if vb, _ := native.AsBoolean(v); vb != exp {
				t.Errorf("field %s: expected %v, got %v", k, exp, v.Data)
			}
		}
	}
}

// TestMakeObjectDeep7LevelsPrototypeChain tests 7-level prototype chain.
func TestMakeObjectDeep7LevelsPrototypeChain(t *testing.T) {
	// A 7-level class chain resolves FLAT: every inherited field lands
	// in the instance's own field map at make.
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:Integer}`,
		`def L2 refine L1 {b:String}`,
		`def L3 refine L2 {c:Boolean}`,
		`def L4 refine L3 {d:Integer}`,
		`def L5 refine L4 {e:String}`,
		`def L6 refine L5 {f:Boolean}`,
		`def L7 refine L6 {g:Integer}`,
		`make L7 {a:10, b:"twenty", c:true, d:40, e:"fifty", f:false, g:70}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	checks := map[string]interface{}{
		"a": int64(10), "b": "twenty", "c": true, "d": int64(40),
		"e": "fifty", "f": false, "g": int64(70),
	}
	for k, expected := range checks {
		v, ok := oi.Fields.Get(k)
		if !ok {
			t.Errorf("missing flat field %s", k)
			continue
		}
		switch exp := expected.(type) {
		case int64:
			if _v, _ := native.AsInteger(v); _v != exp {
				t.Errorf("field %s: expected %d, got %d", k, exp, _v)
			}
		case string:
			if _v, _ := native.AsString(v); _v != exp {
				t.Errorf("field %s: expected %q, got %q", k, exp, _v)
			}
		case bool:
			if vb, _ := native.AsBoolean(v); vb != exp {
				t.Errorf("field %s: expected %v, got %v", k, exp, vb)
			}
		}
	}
}

// TestMakeObjectDeep7LevelsPrototypeDepth verifies prototype chain has correct depth.
func TestMakeObjectDeep7LevelsPrototypeDepth(t *testing.T) {
	// No prototype chain exists at any depth — a 7-level class chain
	// still yields a flat instance.
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:Integer}`,
		`def L2 refine L1 {b:String}`,
		`def L3 refine L2 {c:Boolean}`,
		`def L4 refine L3 {d:Integer}`,
		`def L5 refine L4 {e:String}`,
		`def L6 refine L5 {f:Boolean}`,
		`def L7 refine L6 {g:Integer}`,
		`make L7 {a:10, b:"twenty", c:true, d:40, e:"fifty", f:false, g:70}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	_ = oi
	if oi.Fields.Len() != 7 {
		t.Errorf("expected 7 flat fields, got %d", oi.Fields.Len())
	}
}

// TestMakeObjectDeep7GrandparentFieldAccess verifies field access from grandparent+.
func TestMakeObjectDeep7GrandparentFieldAccess(t *testing.T) {
	// A grandparent field is an ordinary flat field on the instance.
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:Integer}`,
		`def L2 refine L1 {b:String}`,
		`def L3 refine L2 {c:Boolean}`,
		`make L3 {a:1, b:"hi", c:true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	b, ok := oi.GetField("b")
	if !ok {
		t.Fatal("expected GetField to find 'b' flat on the instance")
	}
	if _v, _ := native.AsString(b); _v != "hi" {
		t.Errorf("expected b='hi', got %s", _v)
	}
}

// TestMakeObjectDeep7OverrideGrandparentField overrides grandparent field at make time.
func TestMakeObjectDeep7OverrideGrandparentField(t *testing.T) {
	// Providing a grandparent field at make sets it like any other —
	// all fields are peers in the flat map.
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:Integer}`,
		`def L2 refine L1 {b:String}`,
		`def L3 refine L2 {c:Boolean}`,
		`make L3 {c:true, a:999, b:"x"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	oi, _ := native.AsClassInstance(result[0])
	a, _ := oi.Fields.Get("a")
	if _v, _ := native.AsInteger(a); _v != 999 {
		t.Errorf("expected a=999, got %d", _v)
	}
}

// TestMakeObjectDeep7NarrowingChain tests narrowing through multiple levels.
func TestMakeObjectDeep7NarrowingChain(t *testing.T) {
	// L1: x:Number, L2: x:Integer (narrows Number), L3: x:42 (narrows Integer)
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {x:Number}`,
		`def L2 refine L1 {x:Integer}`,
		`def L3 refine L2 {x:42}`,
		`make L3 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_tmp81, _ := native.AsClassInstance(result[0])
	allF := _tmp81.AllFields()
	x, _ := allF.Get("x")
	_v82, _ := native.AsInteger(x)
	if _v82 != 42 {
		_v83, _ := native.AsInteger(x)
		t.Errorf("expected x=42 (narrowed default), got %d", _v83)
	}
}

// TestMakeObjectDeep7AutoPrototypeStringFormat tests String output with deep auto-prototype.
func TestMakeObjectDeep7AutoPrototypeStringFormat(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def L1 class {a:1}`,
		`def L2 refine L1 {b:2}`,
		`def L3 refine L2 {c:3}`,
		`make L3 {}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := result[0].String()
	if !strings.Contains(s, "a:1") {
		t.Errorf("expected String to contain 'a:1', got %s", s)
	}
	if !strings.Contains(s, "b:2") {
		t.Errorf("expected String to contain 'b:2', got %s", s)
	}
	if !strings.Contains(s, "c:3") {
		t.Errorf("expected String to contain 'c:3', got %s", s)
	}
}

// TestMakeObjectPrototypeDotAccess verifies the full prototype example:
// define Foo, create foo1, define Bar extending Foo, create bar-a with foo1
// as prototype, then access fields via dot notation.
func TestMakeObjectPrototypeDotAccess(t *testing.T) {
	// Dot access reads flat instance fields — own and inherited alike.
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def foo1 (make Foo {x:1})`,
		`foo1 dot x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _v, _ := native.AsInteger(result[0]); _v != 1 {
		t.Errorf("expected foo1.x=1, got %d", _v)
	}

	result, err = runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {y:String}`,
		`def bar-a (make Bar {x:1, y:"A"})`,
		`bar-a dot y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _v, _ := native.AsString(result[0]); _v != "A" {
		t.Errorf("expected bar-a.y='A', got %s", _v)
	}

	result, err = runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def Bar refine Foo {y:String}`,
		`def bar-a (make Bar {x:1, y:"A"})`,
		`bar-a dot x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _v, _ := native.AsInteger(result[0]); _v != 1 {
		t.Errorf("expected bar-a.x=1 (flat inherited field), got %d", _v)
	}
}

// TestMakeObjectPrototypeDotAccessEndToEnd runs the full class example
// as a single program: define Foo, create foo1, define Bar extending
// Foo, create bar-a (flat — inherited fields provided at make), then
// dot-access an inherited field.
func TestMakeObjectPrototypeDotAccessEndToEnd(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def Foo class {x:Integer}`,
		`def foo1 (make Foo {x:1})`,
		`foo1.x`,
		`def Bar refine Foo {y:String}`,
		`def bar-a (make Bar {x:1, y:"A"})`,
		`bar-a.y`,
		`bar-a.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _v, _ := native.AsInteger(result[0]); _v != 1 {
		t.Errorf("expected bar-a.x=1 (flat inherited field), got %d", _v)
	}
}
