package native

import (
	"bytes"
	"strings"
	"testing"
)

// ========================
// Engine edge cases: stepEnd, curryOrPrefix, peekForwardValue
// ========================

func TestStepEndNoForward(t *testing.T) {
	// "end" with no pending forward should just be removed
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewEnd()})
	_as32, _ := AsInteger(result[0])
	if len(result) != 1 || _as32 != 1 {
		t.Errorf("expected [1], got %v", result)
	}
}

func TestDefEndExplicit(t *testing.T) {
	// "def foo 42 end foo" — end terminates def's forward collection
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewInteger(42), NewEnd(),
		NewWord("foo"),
	})
	_as33, _ := AsInteger(result[0])
	if len(result) != 1 || _as33 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestParenResolvesForward(t *testing.T) {
	// (1 add 2) — paren should resolve the forward
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
	})
	_as34, _ := AsInteger(result[0])
	if len(result) != 1 || _as34 != 3 {
		t.Errorf("expected 3, got %v", result)
	}
}

func TestUnmatchedCloseParen(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	err = runBoruError(t, r, []Value{NewInteger(1), NewCloseParen()})
	if err == nil {
		t.Fatal("expected error for unmatched close paren")
	}
}

// ========================
// ResolveWordValue tests
// ========================

func TestResolveWordValueTrue(t *testing.T) {
	v := ResolveWordValue(NewWord("true"))
	_as35, _ := AsBoolean(v)
	if !v.Parent().ConformsTo(TBoolean) || !_as35 {
		t.Errorf("expected boolean true, got %s", v)
	}
}

func TestResolveWordValueFalse(t *testing.T) {
	v := ResolveWordValue(NewWord("false"))
	_as36, _ := AsBoolean(v)
	if !v.Parent().ConformsTo(TBoolean) || _as36 {
		t.Errorf("expected boolean false, got %s", v)
	}
}

func TestResolveWordValueNone(t *testing.T) {
	v := ResolveWordValue(NewWord("None"))
	if !v.Equal(TNone) {
		t.Errorf("expected none, got %s", v)
	}
}

func TestResolveWordValueOther(t *testing.T) {
	v := ResolveWordValue(NewWord("foo"))
	if !v.Parent().Equal(TAtom) {
		t.Errorf("expected atom, got %s", v)
	}
}

func TestResolveWordValueNonWord(t *testing.T) {
	v := ResolveWordValue(NewInteger(42))
	if !v.Parent().ConformsTo(TInteger) {
		t.Errorf("expected integer passthrough, got %s", v)
	}
}

// ========================
// resolveSigType / resolveTypeName tests
// ========================

func TestResolveSigTypeTypeLiteral(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewTypeLiteral(TNumber))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TNumber) {
		t.Errorf("expected number, got %s", tp)
	}
}

func TestResolveSigTypeWord(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewWord("String"))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TString) {
		t.Errorf("expected string, got %s", tp)
	}
}

func TestResolveSigTypeString(t *testing.T) {
	tp, _, err := resolveSigType(nil, NewString("Boolean"))
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TBoolean) {
		t.Errorf("expected boolean, got %s", tp)
	}
}

func TestResolveSigTypeInteger(t *testing.T) {
	v := NewInteger(42)
	tp, _, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.ConformsTo(TInteger) {
		t.Errorf("expected integer type, got %s", tp)
	}
}

func TestResolveSigTypeBoolean(t *testing.T) {
	v := NewBoolean(true)
	tp, _, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.ConformsTo(TBoolean) {
		t.Errorf("expected boolean type, got %s", tp)
	}
}

func TestResolveSigTypeMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)
	tp, pattern, err := resolveSigType(nil, mapVal)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TMap) {
		t.Errorf("expected Map for map, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected non-nil pattern for map")
	}
	if !pattern.Parent().Equal(TMap) {
		t.Errorf("expected pattern to be a map, got %s", pattern.Parent())
	}
}

func TestResolveTypeName(t *testing.T) {
	cases := map[string]*Type{
		"Any": TAny, "None": TNone, "Number": TNumber,
		"Integer": TInteger, "String": TString, "Boolean": TBoolean,
		"Atom": TAtom, "List": TList, "Map": TMap, "Scalar": TScalar,
	}
	for name, want := range cases {
		got, err := resolveTypeName(name)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(want) {
			t.Errorf("resolveTypeName(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestResolveTypeNameUnknown(t *testing.T) {
	// Unknown names error out — undefined types can't be silently materialised.
	if _, err := resolveTypeName("Foobar"); err == nil {
		t.Error("expected error for unknown type 'Foobar'")
	}
}

// ========================
// IsTypeBody tests
// ========================

func TestIsTypeValueTypeLiteral(t *testing.T) {
	if !IsTypeBody(addrOf(NewTypeLiteral(TNumber))) {
		t.Error("type literal should be a type value")
	}
}

func TestIsTypeValueDisjunct(t *testing.T) {
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	if !IsTypeBody(&disj) {
		t.Error("disjunct of types should be a type value")
	}
}

func TestIsTypeValueRecordType(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	if !IsTypeBody(&rt) {
		t.Error("record type should be a type value")
	}
}

func TestIsTypeValueNotType(t *testing.T) {
	if IsTypeBody(addrOf(NewInteger(42))) {
		t.Error("integer should not be a type value")
	}
}

func TestIsTypeValueTypedList(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TString))
	if !IsTypeBody(&tl) {
		t.Error("typed list should be a type value")
	}
}

func TestIsTypeValueTypedMap(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	if !IsTypeBody(&tm) {
		t.Error("typed map should be a type value")
	}
}

// ========================
// Fn tests (multi-signature, edge cases)
// ========================

func TestFnMultiSignature(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def f fn [[x:number] [number] [x mul x] [x:string] [string] [x upper]]
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("f"), NewWord("fn"),
		NewList([]Value{
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TNumber)))}),
			NewList([]Value{NewTypeLiteral(TNumber)}),
			NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TString)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewWord("x"), NewWord("upper")}),
		}),
		NewInteger(5), NewWord("f"),
	})
	_as37, _ := AsInteger(result[0])
	if len(result) != 1 || _as37 != 25 {
		t.Errorf("expected 25, got %v", result)
	}
}

// ========================
// Value.String edge cases (additional)
// ========================

func TestValueStringTypedListCov(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TString))
	s := tl.String()
	if !strings.Contains(s, "String") {
		t.Errorf("expected typed list string, got %q", s)
	}
}

func TestValueStringTypedMapCov(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	s := tm.String()
	if !strings.Contains(s, "Number") {
		t.Errorf("expected typed map string, got %q", s)
	}
}

// ========================
// Format: encode/decode delimited
// ========================

func TestDecodeCSV(t *testing.T) {
	content := "name,age\nalice,30\nbob,25\n"
	result, err := decodeDelimited(content, ",")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !IsTableType(result[0]) {
		t.Fatalf("expected table type, got %s", result[0])
	}
	td := result[0].Data().(TableData)
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(td.Rows))
	}
	if td.Record.Fields.Len() != 2 {
		t.Errorf("expected 2 columns, got %d", td.Record.Fields.Len())
	}
}

func TestDecodeTSV(t *testing.T) {
	content := "x\ty\n1\t2\n3\t4\n"
	result, err := decodeDelimited(content, "\t")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestDecodeEmpty(t *testing.T) {
	result, err := decodeDelimited("", ",")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEncodeTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	row.Set("age", NewInteger(30))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := ValueOf(&Datum{Parent: TList, Data: td})

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "name") || !strings.Contains(encoded, "alice") {
		t.Errorf("expected encoded CSV with 'name' and 'alice', got %q", encoded)
	}
}

func TestEncodeQuotedStrings(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("val", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("val", NewString("has,comma"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	v := ValueOf(&Datum{Parent: TList, Data: td})

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "\"has,comma\"") {
		t.Errorf("expected quoted value, got %q", encoded)
	}
}

func TestEncodeListOfMaps(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	list := NewList([]Value{NewMap(m)})

	encoded, err := encodeDelimited(list, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.Contains(encoded, "x") {
		t.Errorf("expected 'x' header, got %q", encoded)
	}
}

func TestEncodeEmptyColumns(t *testing.T) {
	fields := NewOrderedMap()
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}
	v := ValueOf(&Datum{Parent: TList, Data: td})

	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if encoded != "" {
		t.Errorf("expected empty string, got %q", encoded)
	}
}

func TestEncodeNonTable(t *testing.T) {
	v := NewString("hello")
	encoded, err := encodeDelimited(v, ",")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if encoded != "'hello'" {
		t.Errorf("expected 'hello', got %q", encoded)
	}
}

// ========================
// More query tests to cover remaining branches
// ========================

// ========================
// More unify edge cases
// ========================

func TestUnifyListsDifferentLength(t *testing.T) {
	a := NewList([]Value{NewInteger(1)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})
	_, ok := Unify(a, b)
	if ok {
		t.Error("expected different-length lists to fail")
	}
}

func TestUnifyTypedListWithConcrete(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TNumber))
	concrete := NewList([]Value{NewInteger(1), NewInteger(2)})
	result, ok := Unify(tl, concrete)
	if !ok {
		t.Fatal("expected typed list to unify with concrete list")
	}
	if !result.Parent().Equal(TList) {
		t.Errorf("expected list, got %s", result.Parent())
	}
}

func TestUnifyTypedListFail(t *testing.T) {
	tl := NewTypedList(NewTypeLiteral(TNumber))
	concrete := NewList([]Value{NewString("not a number")})
	_, ok := Unify(tl, concrete)
	if ok {
		t.Error("expected typed list + string list to fail")
	}
}

func TestUnifyRecordTypesDiffOrder(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("a", NewTypeLiteral(TNumber))
	f1.Set("b", NewTypeLiteral(TString))
	f2 := NewOrderedMap()
	f2.Set("b", NewTypeLiteral(TString))
	f2.Set("a", NewTypeLiteral(TNumber))
	_, ok := Unify(NewRecordType(f1), NewRecordType(f2))
	if ok {
		t.Error("expected different field order to fail")
	}
}

func TestUnifyRecordTypesDiffFields(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("a", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("a", NewTypeLiteral(TNumber))
	f2.Set("b", NewTypeLiteral(TString))
	_, ok := Unify(NewRecordType(f1), NewRecordType(f2))
	if ok {
		t.Error("expected different field count to fail")
	}
}

func TestUnifyTableWithNonTable(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	tt := NewTableType(RecordTypeInfo{Fields: f})
	concrete := NewList([]Value{NewInteger(1)})
	_, ok := Unify(tt, concrete)
	if ok {
		t.Error("expected table + plain list to fail")
	}
}

func TestUnifyTableTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TInteger))
	tt1 := NewTableType(RecordTypeInfo{Fields: f1})
	tt2 := NewTableType(RecordTypeInfo{Fields: f2})
	_, ok := Unify(tt1, tt2)
	if !ok {
		t.Error("expected compatible table types to unify")
	}
}

func TestUnifyListLiteralWithTable(t *testing.T) {
	listType := NewTypeLiteral(TList)
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	tt := NewTableType(RecordTypeInfo{Fields: f})
	_, ok := Unify(listType, tt)
	if ok {
		t.Error("expected list literal + table to fail")
	}
}

func TestUnifyMapLiteralWithRecord(t *testing.T) {
	mapType := NewTypeLiteral(TMap)
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	_, ok := Unify(mapType, rt)
	if ok {
		t.Error("expected map literal + record to fail")
	}
}

func TestUnifyRecordWithConcrete(t *testing.T) {
	f := NewOrderedMap()
	f.Set("x", NewTypeLiteral(TNumber))
	rt := NewRecordType(f)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	_, ok := Unify(rt, NewMap(m))
	if ok {
		t.Error("expected record type + concrete map to fail")
	}
}

// ========================
// SQLite store tests
// ========================

func TestSQLiteStoreTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	row.Set("age", NewInteger(30))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	if err := store.StoreTable("test_people", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	if !store.HasTable("test_people") {
		t.Error("expected table to exist")
	}
}

func TestSQLiteQuery(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("x", NewInteger(1))
	row2 := NewOrderedMap()
	row2.Set("x", NewInteger(2))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	if err := store.StoreTable("nums", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}

	result, err := store.Query("SELECT * FROM \"nums\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestSQLiteStoreTempTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("v", NewString("hello"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	name, err := store.StoreTempTable(td)
	if err != nil {
		t.Fatalf("StoreTempTable error: %v", err)
	}
	if name == "" {
		t.Error("expected non-empty temp table name")
	}
	if !store.HasTable(name) {
		t.Error("expected temp table to exist")
	}
}

func TestSQLiteDropTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	if err := store.StoreTable("drop_me", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	if !store.HasTable("drop_me") {
		t.Fatal("expected table to exist before drop")
	}
	store.DropTable("drop_me")
	if store.HasTable("drop_me") {
		t.Error("expected table to not exist after drop")
	}
}

func TestSQLiteStoreWithBoolAndString(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("flag", NewTypeLiteral(TBoolean))
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("flag", NewBoolean(true))
	row.Set("name", NewString("test"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	if err := store.StoreTable("bool_test", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}

	result, err := store.Query("SELECT * FROM \"bool_test\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

// ========================
// More trace coverage (wrapping, notes on second line)
// ========================

func TestRunTraceError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf
	tokens := []Value{NewInteger(10), NewWord("div"), NewInteger(0)}
	_, err = RunTrace(r, tokens, &buf)
	if err == nil {
		t.Fatal("expected error for div by zero")
	}
	out := buf.String()
	if !strings.Contains(out, "error") {
		t.Errorf("expected error in trace output, got %q", out)
	}
}

func TestTraceWrapMultiLine(t *testing.T) {
	// Create enough parts to force wrapping
	var parts []string
	for i := 0; i < 20; i++ {
		parts = append(parts, TraceColorize(NewInteger(int64(i*1000000))))
	}
	lines := TraceWrap(parts, 5, 40)
	if len(lines) < 2 {
		t.Errorf("expected multiple lines for wrapping, got %d", len(lines))
	}
}

// ========================
// makeConvert / makeFieldValue
// ========================

func TestMakeConvertToString(t *testing.T) {
	v, err := makeConvert(NewInteger(42), TString)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as38, _ := AsString(v)
	if _as38 != "42" {
		t.Errorf("expected '42', got %s", v)
	}
}

func TestMakeConvertToNumber(t *testing.T) {
	v, err := makeConvert(NewString("99"), TNumber)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as39, _ := AsInteger(v)
	if _as39 != 99 {
		t.Errorf("expected 99, got %s", v)
	}
}

func TestMakeConvertToBoolFromBool(t *testing.T) {
	v, err := makeConvert(NewBoolean(true), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as40, _ := AsBoolean(v)
	if !_as40 {
		t.Error("expected true")
	}
}

func TestMakeConvertToBoolFromInt(t *testing.T) {
	v, err := makeConvert(NewInteger(1), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as41, _ := AsBoolean(v)
	if !_as41 {
		t.Error("expected true")
	}
}

func TestMakeConvertToBoolFromString(t *testing.T) {
	v, err := makeConvert(NewString("true"), TBoolean)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as42, _ := AsBoolean(v)
	if !_as42 {
		t.Error("expected true")
	}
	// A non-empty String coerces to true — content is not parsed.
	v2, _ := makeConvert(NewString("false"), TBoolean)
	_as43, _ := AsBoolean(v2)
	if !_as43 {
		t.Error("expected true for non-empty string \"false\"")
	}
	v3, _ := makeConvert(NewString(""), TBoolean)
	_as44, _ := AsBoolean(v3)
	if _as44 {
		t.Error("expected false for empty string")
	}
}

func TestMakeConvertToAtom(t *testing.T) {
	v, err := makeConvert(NewString("hello"), TAtom)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !v.Parent().Equal(TAtom) {
		t.Errorf("expected atom, got %s", v.Parent())
	}
}

func TestMakeConvertUnsupported(t *testing.T) {
	_, err := makeConvert(NewString("x"), TList)
	if err == nil {
		t.Error("expected error for unsupported target type")
	}
}

func TestMakeFieldValueAlreadyMatches(t *testing.T) {
	v, err := makeFieldValue(NewInteger(42), NewTypeLiteral(TNumber))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as45, _ := AsInteger(v)
	if _as45 != 42 {
		t.Errorf("expected 42, got %s", v)
	}
}

func TestMakeFieldValueConvert(t *testing.T) {
	// A numeric field rejects a non-numeric source rather than laundering
	// the String "99" into a number by stringify-then-reparse (WAT §V).
	if _, err := makeFieldValue(NewString("99"), NewTypeLiteral(TNumber)); err == nil {
		t.Error("expected a String into a Number field to be rejected, got no error")
	}
	// Numeric widening between Number leaves is still coerced: an Integer
	// satisfies a Float field as 99.0.
	v, err := makeFieldValue(NewInteger(99), NewTypeLiteral(TFloat))
	if err != nil {
		t.Fatalf("Integer into Float field: %v", err)
	}
	if f, _ := AsFloat(v); f != 99.0 {
		t.Errorf("expected 99.0, got %s", v)
	}
}

func TestMakeFieldValueWordTrue(t *testing.T) {
	v, err := makeFieldValue(NewWord("true"), NewTypeLiteral(TBoolean))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as47, _ := AsBoolean(v)
	if !_as47 {
		t.Error("expected true")
	}
}

func TestMakeFieldValueConstraintUnify(t *testing.T) {
	v, err := makeFieldValue(NewInteger(42), NewInteger(42))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	_as48, _ := AsInteger(v)
	if _as48 != 42 {
		t.Errorf("expected 42, got %s", v)
	}
}

// ========================
// ValToString coverage
// ========================

func TestValToStringAtom(t *testing.T) {
	s := ValToString(NewAtom("hello"))
	if s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
}

func TestValToStringNone(t *testing.T) {
	s := ValToString(NewTypeLiteral(TNone))
	if s != "None" {
		t.Errorf("expected 'None', got %q", s)
	}
}

// ========================
// Engine: stepEnd with forward before end
// ========================

func TestStepEndWithForwardBeforeEnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def myval 42 end 1 add myval
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("myval"), NewInteger(42), NewEnd(),
		NewInteger(1), NewWord("add"), NewWord("myval"),
	})
	_as49, _ := AsInteger(result[0])
	if len(result) != 1 || _as49 != 43 {
		t.Errorf("expected 43, got %v", result)
	}
}

func TestStepEndTerminatesDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def a [1 add] end 10 a
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("a"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("add")}),
		NewEnd(),
		NewInteger(10), NewWord("a"),
	})
	_as50, _ := AsInteger(result[0])
	if len(result) != 1 || _as50 != 11 {
		t.Errorf("expected 11, got %v", result)
	}
}

// ========================
// NewRegistry (Store field)
// ========================

func TestNewRegistryHasStore(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Store field removed; context store is initialized by InitRootContext.
	if HostFormats(r) == nil {
		t.Error("expected Formats capability to be installed")
	}
}

// ========================
// Direct unit tests for query.go internal functions
// ========================

func TestBuildWhereClauseEmpty(t *testing.T) {
	cond := NewList([]Value{})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("expected '1=1', got %q", clause)
	}
}

func TestBuildWhereClauseSimpleEq(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("eq"), NewString("alice")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "=") || !strings.Contains(clause, "alice") {
		t.Errorf("expected SQL with = and alice, got %q", clause)
	}
}

func TestBuildWhereClauseLtGteLteNeq(t *testing.T) {
	tests := []struct {
		op    string
		sqlOp string
	}{
		{"lt", "<"},
		{"gte", ">="},
		{"lte", "<="},
		{"neq", "!="},
		{"gt", ">"},
		{"like", "LIKE"},
	}
	for _, tt := range tests {
		cond := NewList([]Value{NewAtom("age"), NewAtom(tt.op), NewInteger(25)})
		clause, err := buildWhereClause(cond)
		if err != nil {
			t.Errorf("op %s: %v", tt.op, err)
			continue
		}
		if !strings.Contains(clause, tt.sqlOp) {
			t.Errorf("op %s: expected %q in clause %q", tt.op, tt.sqlOp, clause)
		}
	}
}

func TestBuildWhereClauseIsNull(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("is"), NewAtom("null")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IS NULL") {
		t.Errorf("expected IS NULL, got %q", clause)
	}
}

func TestBuildWhereClauseIsNotNull(t *testing.T) {
	cond := NewList([]Value{NewAtom("name"), NewAtom("is"), NewAtom("not"), NewAtom("null")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IS NOT NULL") {
		t.Errorf("expected IS NOT NULL, got %q", clause)
	}
}

func TestBuildWhereClauseBetween(t *testing.T) {
	cond := NewList([]Value{NewAtom("age"), NewAtom("between"), NewInteger(20), NewInteger(30)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "BETWEEN") {
		t.Errorf("expected BETWEEN, got %q", clause)
	}
}

func TestBuildWhereClauseIn(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	cond := NewList([]Value{NewAtom("id"), NewAtom("in"), inList})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "IN") {
		t.Errorf("expected IN, got %q", clause)
	}
}

func TestBuildWhereClauseNotIn(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2)})
	cond := NewList([]Value{NewAtom("id"), NewAtom("not"), NewAtom("in"), inList})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT IN") {
		t.Errorf("expected NOT IN, got %q", clause)
	}
}

func TestBuildWhereClauseNotBetween(t *testing.T) {
	cond := NewList([]Value{NewAtom("age"), NewAtom("not"), NewAtom("between"), NewInteger(20), NewInteger(30)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT BETWEEN") {
		t.Errorf("expected NOT BETWEEN, got %q", clause)
	}
}

func TestBuildWhereClauseAndOr(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("age"), NewAtom("gt"), NewInteger(20),
		NewAtom("and"),
		NewAtom("name"), NewAtom("eq"), NewString("alice"),
	})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "AND") {
		t.Errorf("expected AND, got %q", clause)
	}
}

func TestBuildWhereClauseCollate(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("name"), NewAtom("eq"), NewString("alice"), NewAtom("collate"), NewAtom("nocase"),
	})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "COLLATE NOCASE") {
		t.Errorf("expected COLLATE NOCASE, got %q", clause)
	}
}

func TestBuildWhereClauseNotPrefix(t *testing.T) {
	// not [sub-condition]
	sub := NewList([]Value{NewAtom("age"), NewAtom("gt"), NewInteger(20)})
	cond := NewList([]Value{NewAtom("not"), sub})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NOT (") {
		t.Errorf("expected NOT (...), got %q", clause)
	}
}

func TestBuildWhereClauseSubgroup(t *testing.T) {
	// [[age gt 20] and name eq "alice"]
	sub := NewList([]Value{NewAtom("age"), NewAtom("gt"), NewInteger(20)})
	cond := NewList([]Value{sub, NewAtom("and"), NewAtom("name"), NewAtom("eq"), NewString("alice")})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "(") {
		t.Errorf("expected parenthesized subgroup, got %q", clause)
	}
}

func TestBuildWhereClauseValueBoolNone(t *testing.T) {
	// Test boolean and none in WHERE value
	cond := NewList([]Value{NewAtom("active"), NewAtom("eq"), NewBoolean(true)})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "'true'") {
		t.Errorf("expected 'true', got %q", clause)
	}
}

// ========================
// buildJoinCondition tests
// ========================

func TestBuildJoinConditionSimple(t *testing.T) {
	cond := NewList([]Value{NewAtom("id"), NewAtom("eq"), NewAtom("dept_id")})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "=") {
		t.Errorf("expected = in join condition, got %q", clause)
	}
}

func TestBuildJoinConditionDotQualified(t *testing.T) {
	cond := NewList([]Value{NewString("a.id"), NewAtom("eq"), NewString("b.id")})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	// quoteJoinCol should split on dot
	if !strings.Contains(clause, ".") {
		t.Errorf("expected dot-qualified columns in join condition, got %q", clause)
	}
}

func TestBuildJoinConditionMultipleWithAnd(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("id"), NewAtom("eq"), NewAtom("fk"),
		NewAtom("and"),
		NewAtom("type"), NewAtom("eq"), NewAtom("kind"),
	})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "AND") {
		t.Errorf("expected AND, got %q", clause)
	}
}

func TestBuildJoinConditionEmpty(t *testing.T) {
	cond := NewList([]Value{})
	clause, err := buildJoinCondition(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("expected '1=1', got %q", clause)
	}
}

func TestQuoteJoinCol(t *testing.T) {
	// Simple name
	if got := quoteJoinCol("name"); !strings.Contains(got, "name") {
		t.Errorf("expected 'name' in result, got %q", got)
	}
	// Dot-qualified
	got := quoteJoinCol("people.id")
	if !strings.Contains(got, ".") {
		t.Errorf("expected dot in result, got %q", got)
	}
}

// ========================
// buildOrderClause tests
// ========================

func TestBuildOrderClauseSimple(t *testing.T) {
	cols := NewList([]Value{NewAtom("name")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "name") {
		t.Errorf("expected 'name', got %q", clause)
	}
}

func TestBuildOrderClauseDesc(t *testing.T) {
	cols := NewList([]Value{NewAtom("age"), NewAtom("desc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "DESC") {
		t.Errorf("expected DESC, got %q", clause)
	}
}

func TestBuildOrderClauseNullsFirst(t *testing.T) {
	cols := NewList([]Value{NewAtom("score"), NewAtom("asc"), NewAtom("nulls"), NewAtom("first")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "NULLS FIRST") {
		t.Errorf("expected NULLS FIRST, got %q", clause)
	}
}

func TestBuildOrderClauseCollateNocase(t *testing.T) {
	cols := NewList([]Value{NewAtom("name"), NewAtom("collate"), NewAtom("nocase"), NewAtom("asc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "COLLATE NOCASE") {
		t.Errorf("expected COLLATE NOCASE, got %q", clause)
	}
}

func TestBuildOrderClausePositional(t *testing.T) {
	cols := NewList([]Value{NewInteger(1), NewInteger(2)})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, "1") || !strings.Contains(clause, "2") {
		t.Errorf("expected positional 1,2, got %q", clause)
	}
}

func TestBuildOrderClauseMultiple(t *testing.T) {
	cols := NewList([]Value{NewAtom("city"), NewAtom("asc"), NewAtom("name"), NewAtom("desc")})
	clause, err := buildOrderClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, ",") {
		t.Errorf("expected comma-separated, got %q", clause)
	}
}
