package core

// Standalone-suite tail probes (design/ENG-COVERAGE-PARITY.0.md stage
// 4): arms of pure and near-pure kernel functions that the corpus
// lanes never drive — value sizing, table/JSON rendering, program
// disassembly, and the unify families over the full shape matrix.
// Each probe exercises the arm through the function's own contract.

import (
	"strings"
	"testing"
)

func TestSizeOfArms(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))

	pathon, err := MakePathon(NewString("a/b/c"), false)
	if err != nil || len(pathon) != 1 {
		t.Fatalf("MakePathon: %v", err)
	}

	inst := NewOrderedMap()
	inst.Set("x", NewInteger(1))
	inst.Set("y", NewInteger(2))

	cases := []struct {
		name string
		v    Value
		want int
	}{
		{"string", NewString("abcd"), 4},
		{"boolean-true", NewBoolean(true), 1},
		{"boolean-false", NewBoolean(false), 0},
		{"atom", NewAtom("hello"), 5},
		{"pathon", pathon[0], 3},
		{"number", NewFloat(2.5), 2},
		{"list", NewList([]Value{NewInteger(1), NewInteger(2)}), 2},
		{"map", NewMap(om), 1},
		{"class-instance", NewClassInstance(TClass, ClassInstanceInfo{Fields: inst}), 2},
		{"list-literal", NewTypeLiteral(TList), 0},
	}
	for _, c := range cases {
		if got := SizeOf(c.v); got != c.want {
			t.Errorf("SizeOf(%s) = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestFormatTableArms(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	fields.Set("name", NewTypeLiteral(TString))

	row1 := NewOrderedMap()
	row1.Set("id", NewInteger(1))
	row1.Set("name", NewString("alpha"))
	row2 := NewOrderedMap()
	row2.Set("id", NewInteger(22))
	// row2 has no "name" — the missing-cell arm.

	td := TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewMap(row1), NewMap(row2)},
	}
	got := formatTable(td)
	for _, want := range []string{"id", "name", "alpha", "22", "--"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatTable output %q missing %q", got, want)
		}
	}

	tv := NewValueRaw(TList, td)
	if s := FormatForPrint(tv); !strings.Contains(s, "alpha") {
		t.Errorf("FormatForPrint(table) = %q", s)
	}
}

func TestFormatForPrintArms(t *testing.T) {
	dep := NewDepScalar(DepGTE, NewInteger(3))
	if s := FormatForPrint(dep); s == "" {
		t.Error("dep scalar print must render the constraint")
	}
	if s := FormatValueJSON(NewValueRaw(TNone, NonePayload{})); s != "null" {
		t.Errorf("none JSON = %q, want null", s)
	}
	if s := FormatValueJSON(NewBoolean(false)); s != "false" {
		t.Errorf("false JSON = %q", s)
	}
	if s := FormatValueJSON(dep); !strings.HasPrefix(s, "\"") {
		t.Errorf("dep scalar JSON must be quoted, got %q", s)
	}
}

func TestUnifyMapFamilyArms(t *testing.T) {
	recFields := NewOrderedMap()
	recFields.Set("a", NewTypeLiteral(TInteger))
	rec := NewRecordType(recFields)

	recFields2 := NewOrderedMap()
	recFields2.Set("a", NewTypeLiteral(TInteger))
	rec2 := NewRecordType(recFields2)

	typedInt := NewTypedMap(NewTypeLiteral(TInteger))
	typedStr := NewTypedMap(NewTypeLiteral(TString))

	mapLit := NewTypeLiteral(TMap)

	plain := NewOrderedMap()
	plain.Set("k", NewInteger(1))
	plainMap := NewMap(plain)

	flexOM := NewOrderedMap()
	flexOM.Set("k", NewInteger(9))
	flex := NewFlexMap(flexOM)

	// Map literal vs the family: records are nominal (fail), options
	// re-wrap, plain/typed maps pass, non-maps fail — both directions.
	if _, ok := Unify(mapLit, rec); ok {
		t.Error("Map literal must not unify with a record")
	}
	if _, ok := Unify(rec, mapLit); ok {
		t.Error("record vs Map literal must fail")
	}
	optFields := NewOrderedMap()
	optFields.Set("o", NewTypeLiteral(TInteger))
	opts := NewOptionsType(optFields)
	if out, ok := Unify(mapLit, opts); !ok || !IsOptionsType(out) {
		t.Error("Map literal vs options must re-wrap the options type")
	}
	if out, ok := Unify(opts, mapLit); !ok || !IsOptionsType(out) {
		t.Error("options vs Map literal must re-wrap the options type")
	}
	if _, ok := Unify(mapLit, plainMap); !ok {
		t.Error("Map literal must accept a plain map")
	}
	if _, ok := Unify(plainMap, mapLit); !ok {
		t.Error("plain map vs Map literal must unify")
	}
	if _, ok := Unify(mapLit, NewInteger(1)); ok {
		t.Error("Map literal must reject a non-map")
	}
	if _, ok := Unify(NewInteger(1), mapLit); ok {
		t.Error("non-map vs Map literal must fail")
	}

	// Record vs record, and the mixed-shape fail.
	if _, ok := Unify(rec, rec2); !ok {
		t.Error("identical records must unify")
	}
	if _, ok := Unify(rec, typedInt); ok {
		t.Error("record vs typed map must fail")
	}

	// Typed vs typed: child unify, and its failure.
	if _, ok := Unify(typedInt, NewTypedMap(NewTypeLiteral(TInteger))); !ok {
		t.Error("typed maps with equal children must unify")
	}
	if _, ok := Unify(typedInt, typedStr); ok {
		t.Error("typed maps with clashing children must fail")
	}

	// Typed vs concrete: per-value conformance, both directions.
	if _, ok := Unify(typedInt, plainMap); !ok {
		t.Error("typed Integer map must accept {k:1}")
	}
	if _, ok := Unify(plainMap, typedInt); !ok {
		t.Error("{k:1} must satisfy a typed Integer map")
	}
	if _, ok := Unify(typedStr, plainMap); ok {
		t.Error("typed String map must reject {k:1}")
	}

	// Typed vs concrete FLEX map (the flex re-wrap arm).
	if _, ok := Unify(typedInt, flex); !ok {
		t.Error("typed map must accept an empty flex map")
	}

	// Carrier vs typed (the carrier arm).
	if _, ok := Unify(NewCarrier(TMap), typedInt); !ok {
		t.Error("Map carrier must unify with a typed map")
	}

	// Concrete plain maps: key/value walk, mismatch arms.
	other := NewOrderedMap()
	other.Set("k", NewInteger(1))
	if _, ok := Unify(plainMap, NewMap(other)); !ok {
		t.Error("identical concrete maps must unify")
	}
	miss := NewOrderedMap()
	miss.Set("z", NewInteger(1))
	if _, ok := Unify(plainMap, NewMap(miss)); ok {
		t.Error("maps with different keys must fail")
	}
	clash := NewOrderedMap()
	clash.Set("k", NewString("s"))
	if _, ok := Unify(plainMap, NewMap(clash)); ok {
		t.Error("maps with clashing values must fail")
	}
	big := NewOrderedMap()
	big.Set("k", NewInteger(1))
	big.Set("l", NewInteger(2))
	if _, ok := Unify(plainMap, NewMap(big)); ok {
		t.Error("maps with different lengths must fail")
	}
}

func TestUnifyListFamilyArms(t *testing.T) {
	recFields := NewOrderedMap()
	recFields.Set("a", NewTypeLiteral(TInteger))

	tbl := NewValueRaw(TList, TableTypeInfo{Record: RecordTypeInfo{Fields: recFields}})
	tbl2 := NewValueRaw(TList, TableTypeInfo{Record: RecordTypeInfo{Fields: recFields}})

	typedInt := NewTypedList(NewTypeLiteral(TInteger))
	typedStr := NewTypedList(NewTypeLiteral(TString))
	listLit := NewTypeLiteral(TList)

	// List literal vs the family: tables are exclusive (fail), typed
	// and plain lists pass, non-lists fail — both directions.
	if _, ok := Unify(listLit, tbl); ok {
		t.Error("List literal must not unify with a table")
	}
	if _, ok := Unify(tbl, listLit); ok {
		t.Error("table vs List literal must fail")
	}
	if _, ok := Unify(listLit, typedInt); !ok {
		t.Error("List literal must accept a typed list")
	}
	if _, ok := Unify(typedInt, listLit); !ok {
		t.Error("typed list vs List literal must unify")
	}
	if _, ok := Unify(listLit, NewInteger(1)); ok {
		t.Error("List literal must reject a non-list")
	}
	if _, ok := Unify(NewInteger(1), listLit); ok {
		t.Error("non-list vs List literal must fail")
	}

	// Table vs table, and the mixed-shape fail.
	if _, ok := Unify(tbl, tbl2); !ok {
		t.Error("identical table types must unify")
	}
	if _, ok := Unify(tbl, typedInt); ok {
		t.Error("table vs typed list must fail")
	}

	// Typed vs typed.
	if _, ok := Unify(typedInt, NewTypedList(NewTypeLiteral(TInteger))); !ok {
		t.Error("typed lists with equal children must unify")
	}
	if _, ok := Unify(typedInt, typedStr); ok {
		t.Error("typed lists with clashing children must fail")
	}

	// Typed vs concrete, including flex.
	ints := NewList([]Value{NewInteger(1), NewInteger(2)})
	if _, ok := Unify(typedInt, ints); !ok {
		t.Error("typed Integer list must accept [1 2]")
	}
	if _, ok := Unify(typedStr, ints); ok {
		t.Error("typed String list must reject [1 2]")
	}
	flex := NewFlexList([]Value{NewInteger(3)})
	if _, ok := Unify(typedInt, flex); !ok {
		t.Error("typed Integer list must accept a flex list of ints")
	}

	// Carrier vs typed list.
	if _, ok := Unify(NewCarrier(TList), typedInt); !ok {
		t.Error("List carrier must unify with a typed list")
	}
}

func TestSizeOfTailArms(t *testing.T) {
	// A non-Pathon scalar sizes to 0 through the scalar behavior.
	if got := SizeOf(NewValueRaw(TScalar, IntPayload{N: 1})); got != 0 {
		t.Errorf("non-pathon scalar size = %d, want 0", got)
	}
	// List/Map-parented values whose payload isn't readable size to 0.
	if got := SizeOf(NewValueRaw(TList, IntPayload{N: 1})); got != 0 {
		t.Errorf("unreadable list size = %d, want 0", got)
	}
	if got := SizeOf(NewValueRaw(TMap, IntPayload{N: 1})); got != 0 {
		t.Errorf("unreadable map size = %d, want 0", got)
	}
	// A resource instance sizes by its field count.
	rf := NewOrderedMap()
	rf.Set("a", NewInteger(1))
	if got := SizeOf(NewValueRaw(TResource, ResourceInstanceInfo{Fields: rf})); got != 1 {
		t.Errorf("resource instance size = %d, want 1", got)
	}
}

func TestUnifyFamilyTailArms(t *testing.T) {
	typedM := NewTypedMap(NewTypeLiteral(TInteger))
	typedL := NewTypedList(NewTypeLiteral(TInteger))

	// Flex-family carriers against typed containers.
	if _, ok := Unify(NewCarrier(TFlexMap), typedM); !ok {
		t.Error("FlexMap carrier must unify with a typed map")
	}
	if _, ok := Unify(NewCarrier(TFlexList), typedL); !ok {
		t.Error("FlexList carrier must unify with a typed list")
	}

	// Records: length, order, and per-field clashes.
	mk := func(pairs ...string) Value {
		om := NewOrderedMap()
		for i := 0; i+1 < len(pairs); i += 2 {
			tt := TInteger
			if pairs[i+1] == "String" {
				tt = TString
			}
			om.Set(pairs[i], NewTypeLiteral(tt))
		}
		return NewRecordType(om)
	}
	if _, ok := Unify(mk("a", "Integer"), mk("a", "Integer", "b", "Integer")); ok {
		t.Error("records of different lengths must fail")
	}
	if _, ok := Unify(mk("a", "Integer", "b", "Integer"), mk("b", "Integer", "a", "Integer")); ok {
		t.Error("records with different field order must fail")
	}
	if _, ok := Unify(mk("a", "Integer"), mk("a", "String")); ok {
		t.Error("records with clashing field types must fail")
	}

	// Tables with clashing record schemas.
	tblOf := func(rec Value) Value {
		rt, _ := AsRecordType(rec)
		return NewValueRaw(TList, TableTypeInfo{Record: rt})
	}
	if _, ok := Unify(tblOf(mk("a", "Integer")), tblOf(mk("a", "String"))); ok {
		t.Error("tables with clashing schemas must fail")
	}

	// Both sides outside the family: the family guard.
	if _, err := unifyMapFamily(NewInteger(1), ShapeScalar, NewInteger(2), ShapeScalar, nil); err == nil {
		t.Error("map family must reject two non-map shapes")
	}
	if _, err := unifyListFamily(NewInteger(1), ShapeScalar, NewInteger(2), ShapeScalar, nil); err == nil {
		t.Error("list family must reject two non-list shapes")
	}
}

func TestRegistryTailArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	r.SetParseFunc(func(string) ([]Value, error) { return nil, nil })

	// ValToString over a word renders its name.
	if got := ValToString(NewWord("hello")); got != "hello" {
		t.Errorf("ValToString(word) = %q", got)
	}
	// StoreKey of a type literal renders the type path.
	if got := StoreKey(NewTypeLiteral(TInteger)); got == "" {
		t.Error("StoreKey(literal) must render the type")
	}

	// Match: hit and unknown-name miss.
	r.RegisterNativeFunc(NativeFunc{Name: "mword", Signatures: []Signature{{
		Args: []*Type{TInteger}, BarrierPos: -1,
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		Returns: []*Type{TInteger},
	}}})
	if m := r.Match("mword", []Value{NewInteger(1)}, WordInfo{Name: "mword", ArgCount: -1}); m == nil {
		t.Error("Match must resolve a registered word")
	}
	if m := r.Match("no_such_word", []Value{NewInteger(1)}, WordInfo{Name: "no_such_word", ArgCount: -1}); m != nil {
		t.Error("Match on an unknown word must be nil")
	}

	// RegisterCoreDefault on a name marked builtin.
	r.builtinWords["coredef"] = true
	r.RegisterCoreDefault("coredef", Signature{
		Args: []*Type{TInteger}, BarrierPos: -1,
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		Returns: []*Type{TInteger},
	})
	if r.Lookup("coredef") == nil {
		t.Error("RegisterCoreDefault must install the word")
	}

	// The register hook fires for post-ready registrations.
	fired := ""
	r.ready = true
	r.OnRegisterHook = func(name string) { fired = name }
	r.RegisterNativeFunc(NativeFunc{Name: "hooked", Signatures: []Signature{{
		BarrierPos: -1,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
		Returns: []*Type{},
	}}})
	if fired != "hooked" {
		t.Errorf("register hook fired for %q, want hooked", fired)
	}

	// ResolveTypeLiteralDef recovers a hidden builtin Resource schema
	// bound under the ResourceDefKey bridge.
	res := NewValueRaw(TResource, ResourceTypeInfo{Name: "Resource"})
	r.Defs.Push(ResourceDefKey("Resource"), res)
	got := ResolveTypeLiteralDef(NewTypeLiteral(TResource), r)
	if !IsResourceType(got) {
		t.Errorf("ResolveTypeLiteralDef must recover the schema, got %s", got.String())
	}
}
