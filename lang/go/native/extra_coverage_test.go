package native

import (
	"strings"
	"testing"
)

// ── 1. Value.String() ───────────────────────────────────────────────────

func TestExtraStringMap(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	om.Set("b", NewString("two"))
	v := NewMap(om)
	s := v.String()
	if !strings.Contains(s, "a:1") || !strings.Contains(s, "b:'two'") {
		t.Errorf("map String() = %q, want keys a and b", s)
	}
	if s[0] != '{' || s[len(s)-1] != '}' {
		t.Errorf("map String() should be wrapped in braces, got %q", s)
	}
}

func TestExtraStringList(t *testing.T) {
	v := NewList([]Value{NewInteger(1), NewString("x"), NewBoolean(true)})
	s := v.String()
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		t.Errorf("list String() = %q, want brackets", s)
	}
	if !strings.Contains(s, "1") || !strings.Contains(s, "'x'") || !strings.Contains(s, "true") {
		t.Errorf("list String() = %q, missing elements", s)
	}
}

func TestExtraStringTableType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	tt := TableTypeInfo{Record: RecordTypeInfo{Fields: fields}}
	v := ValueOf(&Datum{Parent: TList, Data: tt})
	s := v.String()
	if !strings.HasPrefix(s, "table{") {
		t.Errorf("table refine String() = %q, want 'table{...'", s)
	}
	if !strings.Contains(s, "name:") || !strings.Contains(s, "age:") {
		t.Errorf("table refine String() = %q, missing fields", s)
	}
}

func TestExtraStringTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	row := NewOrderedMap()
	row.Set("x", NewString("hello"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := ValueOf(&Datum{Parent: TList, Data: td})
	s := v.String()
	if !strings.Contains(s, "table{") || !strings.Contains(s, "[") {
		t.Errorf("table data String() = %q, expected table{...}[...]", s)
	}
}

func TestExtraStringTypedList(t *testing.T) {
	v := NewTypedList(NewTypeLiteral(TString))
	s := v.String()
	if !strings.Contains(s, "[:") {
		t.Errorf("typed list String() = %q, want '[:...'", s)
	}
}

func TestExtraStringTypedMap(t *testing.T) {
	v := NewTypedMap(NewTypeLiteral(TInteger))
	s := v.String()
	if !strings.Contains(s, "{:") {
		t.Errorf("typed map String() = %q, want '{:...'", s)
	}
}

func TestExtraStringRecordType(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	v := NewRecordType(fields)
	s := v.String()
	if !strings.HasPrefix(s, "record{") {
		t.Errorf("record refine String() = %q, want 'record{...'", s)
	}
}

func TestExtraStringAtom(t *testing.T) {
	v := NewAtom("myatom")
	s := v.String()
	if s != "myatom" {
		t.Errorf("atom String() = %q, want 'myatom'", s)
	}
}

func TestExtraStringBoolean(t *testing.T) {
	if s := NewBoolean(true).String(); s != "true" {
		t.Errorf("true String() = %q", s)
	}
	if s := NewBoolean(false).String(); s != "false" {
		t.Errorf("false String() = %q", s)
	}
}

func TestExtraStringDisjunct(t *testing.T) {
	v := NewDisjunct([]Value{NewInteger(1), NewString("a")})
	s := v.String()
	if !strings.Contains(s, " tor ") {
		t.Errorf("disjunct String() = %q, want %q separator", s, " tor ")
	}
}

func TestExtraStringFnDef(t *testing.T) {
	// Function definition: the default branch should handle it
	v := NewFunction(FnDefInfo{Signatures: []FnSig{{
		Params: []FnParam{{Name: "x", Type: TInteger}},
		Impl:   Boru([]Value{NewInteger(1)}), BarrierPos: -1,
	}}})
	s := v.String()
	if s == "" {
		t.Error("fndef String() should not be empty")
	}
}

func TestExtraStringWord(t *testing.T) {
	v := NewWord("myword")
	s := v.String()
	if s != "word(myword)" {
		t.Errorf("word String() = %q, want 'word(myword)'", s)
	}
}

func TestExtraStringOpenParen(t *testing.T) {
	v := NewOpenParen()
	s := v.String()
	if s != "(" {
		t.Errorf("open paren String() = %q, want '('", s)
	}
}

func TestExtraStringTypeLiteral(t *testing.T) {
	v := NewTypeLiteral(TNumber)
	s := v.String()
	if s != "Number" {
		t.Errorf("type literal String() = %q, want 'Number'", s)
	}
}

func TestExtraStringFloat(t *testing.T) {
	v := NewFloat(3.14)
	s := v.String()
	if !strings.Contains(s, "3.14") {
		t.Errorf("decimal String() = %q, want '3.14'", s)
	}
}

func TestExtraStringInteger(t *testing.T) {
	v := NewInteger(42)
	s := v.String()
	if s != "42" {
		t.Errorf("integer String() = %q, want '42'", s)
	}
}

// ── 2. Value.AsNumber() ─────────────────────────────────────────────────

func TestExtraAsNumberInteger(t *testing.T) {
	v := NewInteger(42)
	n, _ := AsNumber(v)
	if n != 42.0 {
		t.Errorf("AsNumber() on integer = %f, want 42.0", n)
	}
}

func TestExtraAsNumberFloat(t *testing.T) {
	v := NewFloat(3.14)
	n, _ := AsNumber(v)
	if n != 3.14 {
		t.Errorf("AsNumber() on decimal = %f, want 3.14", n)
	}
}

// ── 3. Value.AsList().Slice() with TableData ────────────────────────────────────

func TestExtraAsListTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("col", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	row := NewOrderedMap()
	row.Set("col", NewString("val"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := ValueOf(&Datum{Parent: TList, Data: td})
	_lst, _ := AsList(v)
	list := _lst.Slice()
	if len(list) != 1 {
		t.Fatalf("AsList() on TableData got %d rows, want 1", len(list))
	}
}

// ── 4. Value.AsTableType() ──────────────────────────────────────────────

func TestExtraAsTableTypeTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("col", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: []Value{}}
	v := ValueOf(&Datum{Parent: TList, Data: td})
	tt, _ := AsTableType(v)
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("AsTableType() on TableData: got %d fields, want 1", tt.Record.Fields.Len())
	}
}

func TestExtraAsTableTypeTableTypeInfo(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	tti := TableTypeInfo{Record: RecordTypeInfo{Fields: fields}}
	v := ValueOf(&Datum{Parent: TList, Data: tti})
	tt, _ := AsTableType(v)
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("AsTableType() on TableTypeInfo: got %d fields, want 1", tt.Record.Fields.Len())
	}
}

func TestExtraIsTableTypeNonTable(t *testing.T) {
	// A plain list is not a table type.
	v := NewList([]Value{NewInteger(1)})
	if IsTableType(v) {
		t.Error("plain list should not be a table type")
	}
	// A map is not a table type.
	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	vm := NewMap(om)
	if IsTableType(vm) {
		t.Error("map should not be a table type")
	}
}

// ── 5. format.go: CSV/TSV decode/encode via engine ──────────────────────

func TestExtraCSVDecodeEncode(t *testing.T) {
	// Test CSV decode directly
	f := &CSVFormat{}
	vals, err := f.Decode("name,score\nAlice,100\nBob,200")
	if err != nil {
		t.Fatalf("CSV decode: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("CSV decode: got %d values, want 1", len(vals))
	}
	if !IsTableType(vals[0]) {
		t.Fatal("CSV decode: result should be a table type")
	}
	_lst, _ := AsList(vals[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("CSV decode: got %d rows, want 2", len(rows))
	}

	// Test CSV encode (roundtrip)
	out, err := f.Encode(vals[0])
	if err != nil {
		t.Fatalf("CSV encode: %v", err)
	}
	if !strings.Contains(out, "name,score") || !strings.Contains(out, "Alice") {
		t.Errorf("CSV encode: output = %q, missing expected content", out)
	}
}

func TestExtraTSVDecodeEncode(t *testing.T) {
	// Test TSV decode directly
	f := &TSVFormat{}
	vals, err := f.Decode("col1\tcol2\nval1\tval2\nval3\tval4")
	if err != nil {
		t.Fatalf("TSV decode: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("TSV decode: got %d values, want 1", len(vals))
	}
	_lst, _ := AsList(vals[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("TSV decode: got %d rows, want 2", len(rows))
	}

	// Test TSV encode (roundtrip)
	out, err := f.Encode(vals[0])
	if err != nil {
		t.Fatalf("TSV encode: %v", err)
	}
	if !strings.Contains(out, "col1\tcol2") {
		t.Errorf("TSV encode: output = %q, missing header", out)
	}
}

func TestExtraCSVDecodeEmpty(t *testing.T) {
	f := &CSVFormat{}
	vals, err := f.Decode("")
	if err != nil {
		t.Fatalf("CSV decode empty: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("CSV decode empty: got %d values, want 1", len(vals))
	}
}

func TestExtraCSVEncodeListOfMaps(t *testing.T) {
	// Encode a plain list of maps (not TableData) via encodeDelimited
	f := &CSVFormat{}
	om1 := NewOrderedMap()
	om1.Set("x", NewString("a"))
	om1.Set("y", NewString("b"))
	om2 := NewOrderedMap()
	om2.Set("x", NewString("c"))
	om2.Set("y", NewString("d"))
	v := NewList([]Value{NewMap(om1), NewMap(om2)})
	out, err := f.Encode(v)
	if err != nil {
		t.Fatalf("CSV encode list of maps: %v", err)
	}
	if !strings.Contains(out, "x,y") {
		t.Errorf("CSV encode: output = %q, missing header", out)
	}
	if !strings.Contains(out, "a,b") {
		t.Errorf("CSV encode: output = %q, missing row data", out)
	}
}

func TestExtraCSVEncodeNonTable(t *testing.T) {
	// Encode a non-table value (scalar) should return String()
	f := &CSVFormat{}
	out, err := f.Encode(NewInteger(42))
	if err != nil {
		t.Fatalf("CSV encode scalar: %v", err)
	}
	if out != "42" {
		t.Errorf("CSV encode scalar: got %q, want '42'", out)
	}
}

func TestExtraCSVEncodeQuoting(t *testing.T) {
	// String values containing commas should be quoted
	f := &CSVFormat{}
	om := NewOrderedMap()
	om.Set("val", NewString("hello,world"))
	v := NewList([]Value{NewMap(om)})
	out, err := f.Encode(v)
	if err != nil {
		t.Fatalf("CSV encode quoting: %v", err)
	}
	if !strings.Contains(out, `"hello,world"`) {
		t.Errorf("CSV encode quoting: got %q, want quoted value", out)
	}
}

func TestExtraCSVEncodeEmptyColumns(t *testing.T) {
	// Empty list should produce empty string
	f := &CSVFormat{}
	v := NewList([]Value{})
	out, err := f.Encode(v)
	if err != nil {
		t.Fatalf("CSV encode empty: %v", err)
	}
	if out != "" {
		t.Errorf("CSV encode empty: got %q, want empty", out)
	}
}

func TestExtraCSVEncodeNonStringValues(t *testing.T) {
	// Non-string values in map rows should use .String()
	f := &CSVFormat{}
	om := NewOrderedMap()
	om.Set("num", NewInteger(99))
	om.Set("flag", NewBoolean(true))
	v := NewList([]Value{NewMap(om)})
	out, err := f.Encode(v)
	if err != nil {
		t.Fatalf("CSV encode non-string: %v", err)
	}
	if !strings.Contains(out, "99") || !strings.Contains(out, "true") {
		t.Errorf("CSV encode non-string: got %q", out)
	}
}

// ── 6. Unify: lists, maps, ValuesEqual, listsEqual, mapsEqual ───────────

func TestExtraUnifyListsDifferentLengths(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1)})
	_, ok := Unify(a, b)
	if ok {
		t.Error("lists of different lengths should not unify")
	}
}

func TestExtraUnifyListsSameLengthMismatch(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewString("x")})
	_, ok := Unify(a, b)
	if ok {
		t.Error("lists with incompatible elements should not unify")
	}
}

func TestExtraUnifyListsSameSuccess(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewString("x")})
	b := NewList([]Value{NewInteger(1), NewString("x")})
	result, ok := Unify(a, b)
	if !ok {
		t.Fatal("identical lists should unify")
	}
	_lst, _ := AsList(result)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Errorf("unified list has %d elems, want 2", len(elems))
	}
}

func TestExtraUnifyMapsSuccess(t *testing.T) {
	a := NewOrderedMap()
	a.Set("x", NewInteger(1))
	b := NewOrderedMap()
	b.Set("x", NewInteger(1))
	result, ok := Unify(NewMap(a), NewMap(b))
	if !ok {
		t.Fatal("identical maps should unify")
	}
	m, _ := AsMap(result)
	v, _ := m.Get("x")
	_as0, _ := AsNumber(v)
	if _as0 != 1.0 {
		t.Errorf("unified map x = %v, want 1", v)
	}
}

func TestExtraUnifyMapsDifferentKeys(t *testing.T) {
	a := NewOrderedMap()
	a.Set("x", NewInteger(1))
	b := NewOrderedMap()
	b.Set("y", NewInteger(1))
	_, ok := Unify(NewMap(a), NewMap(b))
	if ok {
		t.Error("maps with different keys should not unify")
	}
}

func TestExtraUnifyMapsDifferentSizes(t *testing.T) {
	a := NewOrderedMap()
	a.Set("x", NewInteger(1))
	a.Set("y", NewInteger(2))
	b := NewOrderedMap()
	b.Set("x", NewInteger(1))
	_, ok := Unify(NewMap(a), NewMap(b))
	if ok {
		t.Error("maps with different sizes should not unify")
	}
}

func TestExtraUnifyMapValueMismatch(t *testing.T) {
	a := NewOrderedMap()
	a.Set("x", NewInteger(1))
	b := NewOrderedMap()
	b.Set("x", NewString("nope"))
	_, ok := Unify(NewMap(a), NewMap(b))
	if ok {
		t.Error("maps with incompatible values should not unify")
	}
}

func TestExtraUnifyBooleanEquality(t *testing.T) {
	_, ok := Unify(NewBoolean(true), NewBoolean(true))
	if !ok {
		t.Error("true should unify with true")
	}
	_, ok = Unify(NewBoolean(true), NewBoolean(false))
	if ok {
		t.Error("true should not unify with false")
	}
}

func TestExtraUnifyFloatEquality(t *testing.T) {
	// Float values: same value should unify
	a := NewFloat(3.14)
	b := NewFloat(3.14)
	_, ok := Unify(a, b)
	if !ok {
		t.Error("same decimals should unify")
	}
}

func TestExtraUnifyAtomEquality(t *testing.T) {
	_, ok := Unify(NewAtom("foo"), NewAtom("foo"))
	if !ok {
		t.Error("same atoms should unify")
	}
	_, ok = Unify(NewAtom("foo"), NewAtom("bar"))
	if ok {
		t.Error("different atoms should not unify")
	}
}

func TestExtraUnifyListTypeLiteral(t *testing.T) {
	// Type literal "list" unifies with concrete list
	tl := NewTypeLiteral(TList)
	concrete := NewList([]Value{NewInteger(1)})
	result, ok := Unify(tl, concrete)
	if !ok {
		t.Fatal("list type literal should unify with concrete list")
	}
	_lst1, _ := AsList(result)
	if len(_lst1.Slice()) != 1 {
		t.Errorf("result should be the concrete list")
	}
}

func TestExtraUnifyListTypeLiteralReverse(t *testing.T) {
	// Reverse direction
	tl := NewTypeLiteral(TList)
	concrete := NewList([]Value{NewInteger(1)})
	result, ok := Unify(concrete, tl)
	if !ok {
		t.Fatal("concrete list should unify with list type literal")
	}
	_lst, _ := AsList(result)
	if len(_lst.Slice()) != 1 {
		t.Errorf("result should be the concrete list")
	}
}

func TestExtraUnifyMapTypeLiteral(t *testing.T) {
	tl := NewTypeLiteral(TMap)
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	concrete := NewMap(om)
	result, ok := Unify(tl, concrete)
	if !ok {
		t.Fatal("map type literal should unify with concrete map")
	}
	_m, _ := AsMap(result)
	if _m.Len() != 1 {
		t.Errorf("result should be the concrete map")
	}
}

func TestExtraUnifyMapTypeLiteralReverse(t *testing.T) {
	tl := NewTypeLiteral(TMap)
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	concrete := NewMap(om)
	result, ok := Unify(concrete, tl)
	if !ok {
		t.Fatal("concrete map should unify with map type literal")
	}
	_m, _ := AsMap(result)
	if _m.Len() != 1 {
		t.Errorf("result should be the concrete map")
	}
}

func TestExtraUnifyListVsNonList(t *testing.T) {
	_, ok := Unify(NewList([]Value{}), NewInteger(1))
	if ok {
		t.Error("list vs non-list should not unify")
	}
}

func TestExtraUnifyMapVsNonMap(t *testing.T) {
	om := NewOrderedMap()
	_, ok := Unify(NewMap(om), NewInteger(1))
	if ok {
		t.Error("map vs non-map should not unify")
	}
}

func TestExtraUnifyTypeLiteralsSameType(t *testing.T) {
	// Two type literals of the same type should unify (ValuesEqual nil/nil)
	a := NewTypeLiteral(TString)
	b := NewTypeLiteral(TString)
	_, ok := Unify(a, b)
	if !ok {
		t.Error("same type literals should unify")
	}
}

func TestExtraUnifyTypeLiteralVsConcrete(t *testing.T) {
	// Type literal vs concrete value: subtype relationship
	a := NewTypeLiteral(TString)
	b := NewString("hello")
	_, ok := Unify(a, b)
	if !ok {
		t.Error("string type literal should unify with concrete string via subtype")
	}
}

func TestExtraUnifyNone(t *testing.T) {
	a := NewTypeLiteral(TNone)
	b := NewTypeLiteral(TNone)
	_, ok := Unify(a, b)
	if !ok {
		t.Error("none should unify with none")
	}
	_, ok = Unify(a, NewInteger(1))
	if ok {
		t.Error("none should not unify with integer")
	}
}

func TestExtraUnifyListTypeLiteralVsTable(t *testing.T) {
	// List type literal should NOT unify with table type
	tl := NewTypeLiteral(TList)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	table := NewTableType(RecordTypeInfo{Fields: fields})
	_, ok := Unify(tl, table)
	if ok {
		t.Error("list type literal should not unify with table type")
	}
}

func TestExtraUnifyTableVsPlainList(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	table := NewTableType(RecordTypeInfo{Fields: fields})
	plain := NewList([]Value{NewInteger(1)})
	_, ok := Unify(table, plain)
	if ok {
		t.Error("table vs plain list should not unify")
	}
}

func TestExtraUnifyTableVsTable(t *testing.T) {
	fields1 := NewOrderedMap()
	fields1.Set("x", NewTypeLiteral(TString))
	t1 := NewTableType(RecordTypeInfo{Fields: fields1})
	fields2 := NewOrderedMap()
	fields2.Set("x", NewTypeLiteral(TString))
	t2 := NewTableType(RecordTypeInfo{Fields: fields2})
	_, ok := Unify(t1, t2)
	if !ok {
		t.Error("tables with same schema should unify")
	}
}

func TestExtraUnifyTypedLists(t *testing.T) {
	a := NewTypedList(NewTypeLiteral(TString))
	b := NewTypedList(NewTypeLiteral(TString))
	_, ok := Unify(a, b)
	if !ok {
		t.Error("typed lists with same child should unify")
	}
}

func TestExtraUnifyTypedListWithConcrete(t *testing.T) {
	typed := NewTypedList(NewTypeLiteral(TString))
	concrete := NewList([]Value{NewString("a"), NewString("b")})
	_, ok := Unify(typed, concrete)
	if !ok {
		t.Error("typed list should unify with matching concrete list")
	}
}

func TestExtraUnifyTypedListWithConcreteReverse(t *testing.T) {
	typed := NewTypedList(NewTypeLiteral(TString))
	concrete := NewList([]Value{NewString("a")})
	_, ok := Unify(concrete, typed)
	if !ok {
		t.Error("concrete list should unify with matching typed list (reverse)")
	}
}

func TestExtraUnifyTypedMaps(t *testing.T) {
	a := NewTypedMap(NewTypeLiteral(TString))
	b := NewTypedMap(NewTypeLiteral(TString))
	_, ok := Unify(a, b)
	if !ok {
		t.Error("typed maps with same child should unify")
	}
}

func TestExtraUnifyTypedMapWithConcrete(t *testing.T) {
	typed := NewTypedMap(NewTypeLiteral(TString))
	om := NewOrderedMap()
	om.Set("k", NewString("v"))
	concrete := NewMap(om)
	_, ok := Unify(typed, concrete)
	if !ok {
		t.Error("typed map should unify with matching concrete map")
	}
}

func TestExtraUnifyTypedMapWithConcreteReverse(t *testing.T) {
	typed := NewTypedMap(NewTypeLiteral(TString))
	om := NewOrderedMap()
	om.Set("k", NewString("v"))
	concrete := NewMap(om)
	_, ok := Unify(concrete, typed)
	if !ok {
		t.Error("concrete map should unify with matching typed map (reverse)")
	}
}

func TestExtraUnifyRecordVsNonRecord(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	rec := NewRecordType(fields)
	om := NewOrderedMap()
	om.Set("x", NewString("hi"))
	plain := NewMap(om)
	_, ok := Unify(rec, plain)
	if ok {
		t.Error("record type vs plain map should not unify")
	}
}

func TestExtraUnifyMapTypeLiteralVsRecord(t *testing.T) {
	tl := NewTypeLiteral(TMap)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	rec := NewRecordType(fields)
	_, ok := Unify(tl, rec)
	if ok {
		t.Error("map type literal should not unify with record type")
	}
}

// ── 7. engine.go: stepEnd ───────────────────────────────────────────────

func TestExtraStepEndNoForward(t *testing.T) {
	// "end" with nothing pending should be harmless
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewEnd()})
	_as1, _ := AsNumber(result[0])
	if len(result) != 1 || _as1 != 1.0 {
		t.Errorf("end with no forward: got %v, want [1]", result)
	}
}

func TestExtraStepEndAfterForward(t *testing.T) {
	// Use "end" to terminate a forward expression: 1 add 2 end
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewEnd(),
	})
	_as2, _ := AsNumber(result[0])
	if len(result) != 1 || _as2 != 3.0 {
		t.Errorf("1 add 2 end: got %v, want [3]", result)
	}
}

// ── 8. CoerceBoolean ─────────────────────────────────────────────────────────

func TestExtraIsTruthyBoolean(t *testing.T) {
	if !CoerceBoolean(NewBoolean(true)) {
		t.Error("true should be truthy")
	}
	if CoerceBoolean(NewBoolean(false)) {
		t.Error("false should not be truthy")
	}
}

func TestExtraIsTruthyInteger(t *testing.T) {
	if !CoerceBoolean(NewInteger(1)) {
		t.Error("1 should be truthy")
	}
	if CoerceBoolean(NewInteger(0)) {
		t.Error("0 should not be truthy")
	}
}

func TestExtraIsTruthyNone(t *testing.T) {
	if CoerceBoolean(NewTypeLiteral(TNone)) {
		t.Error("none should not be truthy")
	}
}

func TestExtraIsTruthyList(t *testing.T) {
	if !CoerceBoolean(NewList([]Value{NewInteger(1)})) {
		t.Error("non-empty list should be truthy")
	}
	if CoerceBoolean(NewList([]Value{})) {
		t.Error("empty list should not be truthy")
	}
}

func TestExtraIsTruthyMap(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if !CoerceBoolean(NewMap(om)) {
		t.Error("non-empty map should be truthy")
	}
	if CoerceBoolean(NewMap(NewOrderedMap())) {
		t.Error("empty map should not be truthy")
	}
}

func TestExtraIsTruthyString(t *testing.T) {
	if !CoerceBoolean(NewString("hello")) {
		t.Error("non-empty string should be truthy")
	}
	// A String is truthy whenever non-empty; the magic "false" case was
	// removed for strings (WAT-AUDIT.5.md §E). (A non-String `false` atom
	// keeps its boolean reading — see TestExtraIsTruthyAtom.)
	if !CoerceBoolean(NewString("false")) {
		t.Error("non-empty string 'false' should be truthy")
	}
	if CoerceBoolean(NewString("")) {
		t.Error("empty string should not be truthy")
	}
	if !CoerceBoolean(NewString("true")) {
		t.Error("string 'true' should be truthy")
	}
}

func TestExtraIsTruthyAtom(t *testing.T) {
	if !CoerceBoolean(NewAtom("yes")) {
		t.Error("non-empty atom should be truthy")
	}
	if CoerceBoolean(NewAtom("false")) {
		t.Error("atom 'false' should not be truthy")
	}
}

func TestExtraIsTruthyFloat(t *testing.T) {
	// Floats go through the default/string branch
	if !CoerceBoolean(NewFloat(1.5)) {
		t.Error("non-zero decimal should be truthy")
	}
}

func TestExtraIsTruthyTableType(t *testing.T) {
	// Table types (list with non-[]Value data) take the "return true" branch
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	table := NewTableType(RecordTypeInfo{Fields: fields})
	if !CoerceBoolean(table) {
		t.Error("table type should be truthy (non-[]Value list data)")
	}
}

func TestExtraIsTruthyRecordType(t *testing.T) {
	// Record types (map with non-*OrderedMap data) take the "return true" branch
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TString))
	rec := NewRecordType(fields)
	if !CoerceBoolean(rec) {
		t.Error("record type should be truthy (non-*OrderedMap map data)")
	}
}

// ── 9. types.go: mustType and NewType ───────────────────────────────────

func TestExtraNewTypeValid(t *testing.T) {
	// NewType is strict — "String/Proper" is not a builtin and must error.
	if _, err := NewType("String/Proper"); err == nil {
		t.Error("NewType('String/Proper') should error — not a builtin")
	}
}

func TestExtraNewTypeInvalidLowercase(t *testing.T) {
	_, err := NewType("string/proper")
	if err == nil {
		t.Error("NewType with lowercase should fail")
	}
}

func TestExtraNewTypeSinglePart(t *testing.T) {
	tp, err := NewType("Number")
	if err != nil {
		t.Fatalf("NewType('Number') error: %v", err)
	}
	if tp.Path() != "Scalar/Number" {
		t.Errorf("unexpected type path: %v", tp.Path())
	}
}

// TestExtraMustTypePanics moved to borueng (mustType is engine-internal).

func TestExtraTypeSpecificity(t *testing.T) {
	tp, _ := NewType("Number/Integer")
	if tp.Specificity() != 3 {
		t.Errorf("specificity = %d, want 3", tp.Specificity())
	}
}

func TestExtraTypeIsSubtypeOf(t *testing.T) {
	child := MintTestType("String/Proper")
	parent, _ := NewType("String")
	if !child.IsSubtypeOf(parent) {
		t.Error("String/Proper should be subtype of String")
	}
	if parent.IsSubtypeOf(child) {
		t.Error("String should not be subtype of String/Proper")
	}
	if parent.IsSubtypeOf(parent) {
		t.Error("type should not be subtype of itself")
	}
}

func TestExtraTypeEqual(t *testing.T) {
	a, _ := NewType("Number/Integer")
	b, _ := NewType("Number/Integer")
	c, _ := NewType("Number/Float")
	if !a.Equal(b) {
		t.Error("same types should be equal")
	}
	if a.Equal(c) {
		t.Error("different types should not be equal")
	}
}

func TestExtraTypeMatches(t *testing.T) {
	child := MintTestType("String/Proper")
	parent, _ := NewType("String")
	if !child.ConformsTo(parent) {
		t.Error("String/Proper should match String pattern")
	}
	if parent.ConformsTo(child) {
		t.Error("String should not match String/Proper pattern")
	}
}

// ── Additional coverage: ValuesEqual edge cases ─────────────────────────

func TestExtraValuesEqualTypeLiteralVsConcrete(t *testing.T) {
	// One nil Data, one concrete — should not be equal, so Unify fails
	a := NewTypeLiteral(TInteger)
	b := NewInteger(5)
	// They have different types (TInteger vs Number/Integer/5), so Unify
	// uses subtype. But let's test via Unify word directly.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{a, b, NewWord("unify")})
	if len(result) < 2 {
		t.Fatalf("unify: got %d results", len(result))
	}
	// Should succeed (subtype relationship)
	_as3, _ := AsBoolean(result[1])
	if !_as3 {
		t.Error("integer type literal should unify with concrete integer")
	}
}

func TestExtraValuesEqualFloatsDirect(t *testing.T) {
	// Test via unify word: two decimals with different values
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewFloat(1.5), NewFloat(2.5), NewWord("unify"),
	})
	if len(result) < 2 {
		t.Fatalf("unify: got %d results", len(result))
	}
	// Different decimals should fail to unify (same type, different values)
	// ValuesEqual falls through to default fmt.Sprintf comparison
	_as4, _ := AsBoolean(result[1])
	if _as4 {
		t.Error("different decimals should not unify")
	}
}

func TestExtraValuesEqualListChildTypes(t *testing.T) {
	// Two typed lists with same child type should be equal
	a := NewTypedList(NewTypeLiteral(TString))
	b := NewTypedList(NewTypeLiteral(TString))
	_, ok := Unify(a, b)
	if !ok {
		t.Error("typed lists with same child should unify")
	}

	// Two typed lists with different child types should not be equal
	c := NewTypedList(NewTypeLiteral(TInteger))
	_, ok = Unify(a, c)
	if ok {
		t.Error("typed lists with different child types should not unify")
	}
}

func TestExtraValuesEqualMapChildTypes(t *testing.T) {
	a := NewTypedMap(NewTypeLiteral(TString))
	b := NewTypedMap(NewTypeLiteral(TInteger))
	_, ok := Unify(a, b)
	if ok {
		t.Error("typed maps with different child types should not unify")
	}
}

// ── Disjunct unification ────────────────────────────────────────────────

func TestExtraUnifyDisjunct(t *testing.T) {
	// Disjunct with matching alternative
	d := NewDisjunct([]Value{NewInteger(1), NewInteger(2)})
	result, ok := Unify(d, NewInteger(2))
	if !ok {
		t.Fatal("disjunct should unify with matching alternative")
	}
	_as5, _ := AsNumber(result)
	if _as5 != 2.0 {
		t.Errorf("unified value = %v, want 2", result)
	}
}

func TestExtraUnifyDisjunctNoMatch(t *testing.T) {
	d := NewDisjunct([]Value{NewInteger(1), NewInteger(2)})
	_, ok := Unify(d, NewInteger(3))
	if ok {
		t.Error("disjunct should not unify with non-matching value")
	}
}

func TestExtraUnifyDisjunctReverse(t *testing.T) {
	d := NewDisjunct([]Value{NewString("a"), NewString("b")})
	result, ok := Unify(NewString("b"), d)
	if !ok {
		t.Fatal("value should unify with disjunct containing it")
	}
	_as6, _ := AsString(result)
	if _as6 != "b" {
		t.Errorf("unified value = %v, want 'b'", result)
	}
}

func TestExtraStringFunction(t *testing.T) {
	// Function values (TFunction type)
	v := NewFunction(FnDefInfo{Signatures: []FnSig{{
		Params: []FnParam{{Name: "x", Type: TAny}},
		Impl:   Boru([]Value{NewInteger(1)}), BarrierPos: -1,
	}}})
	s := v.String()
	if s == "" {
		t.Error("function String() should not be empty")
	}
}
