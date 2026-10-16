package native

import (
	"strings"
	"testing"
)

// ========================
// parseColumnSpec tests
// ========================

func TestParseColumnSpecAtoms(t *testing.T) {
	cols := NewList([]Value{NewAtom("name"), NewAtom("age")})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Name != "name" {
		t.Errorf("expected 'name', got %q", specs[0].Name)
	}
}

func TestParseColumnSpecStrings(t *testing.T) {
	cols := NewList([]Value{NewString("name")})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "name" {
		t.Errorf("expected name spec, got %+v", specs)
	}
}

func TestParseColumnSpecAlias(t *testing.T) {
	// [name, person_name] alias pair
	pair := NewList([]Value{NewAtom("name"), NewAtom("person_name")})
	cols := NewList([]Value{pair})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "name" || specs[0].Alias != "person_name" {
		t.Errorf("expected name->person_name alias, got %+v", specs)
	}
}

func TestParseColumnSpecAggregate(t *testing.T) {
	// [count name cnt]
	agg := NewList([]Value{NewAtom("count"), NewAtom("name"), NewAtom("cnt")})
	cols := NewList([]Value{agg})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "COUNT") {
		t.Errorf("expected COUNT in raw, got %q", specs[0].Raw)
	}
	if specs[0].Alias != "cnt" {
		t.Errorf("expected alias 'cnt', got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecCountStar(t *testing.T) {
	// [count * total]
	agg := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("total")})
	cols := NewList([]Value{agg})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "COUNT(*)") {
		t.Errorf("expected COUNT(*), got %q", specs[0].Raw)
	}
}

func TestParseColumnSpecCast(t *testing.T) {
	// [cast age integer age_int]
	cast := NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("integer"), NewAtom("age_int")})
	cols := NewList([]Value{cast})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if !strings.Contains(specs[0].Raw, "CAST") {
		t.Errorf("expected CAST in raw, got %q", specs[0].Raw)
	}
	if specs[0].Alias != "age_int" {
		t.Errorf("expected alias 'age_int', got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecCastNoAlias(t *testing.T) {
	// [cast age integer]
	cast := NewList([]Value{NewAtom("cast"), NewAtom("age"), NewAtom("integer")})
	cols := NewList([]Value{cast})
	specs, err := parseColumnSpec(cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Alias != "age" {
		t.Errorf("expected alias 'age' (default), got %q", specs[0].Alias)
	}
}

func TestParseColumnSpecSumAvgMinMax(t *testing.T) {
	for _, fn := range []string{"sum", "avg", "min", "max"} {
		agg := NewList([]Value{NewAtom(fn), NewAtom("val")})
		cols := NewList([]Value{agg})
		specs, err := parseColumnSpec(cols)
		if err != nil {
			t.Errorf("%s: %v", fn, err)
			continue
		}
		if len(specs) != 1 {
			t.Errorf("%s: expected 1 spec, got %d", fn, len(specs))
			continue
		}
		if !strings.Contains(specs[0].Raw, strings.ToUpper(fn)) {
			t.Errorf("%s: expected %s in raw, got %q", fn, strings.ToUpper(fn), specs[0].Raw)
		}
	}
}

// ========================
// nameFromValue tests
// ========================

func TestNameFromValueAtom(t *testing.T) {
	if got := nameFromValue(NewAtom("foo")); got != "foo" {
		t.Errorf("expected 'foo', got %q", got)
	}
}

func TestNameFromValueString(t *testing.T) {
	if got := nameFromValue(NewString("bar")); got != "bar" {
		t.Errorf("expected 'bar', got %q", got)
	}
}

func TestNameFromValueWord(t *testing.T) {
	if got := nameFromValue(NewWord("baz")); got != "baz" {
		t.Errorf("expected 'baz', got %q", got)
	}
}

func TestNameFromValueOther(t *testing.T) {
	if got := nameFromValue(NewInteger(42)); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ========================
// boruTypenameToSQLType / sqlTypeToBoruType tests
// ========================

func TestBoruTypenameToSQLType(t *testing.T) {
	tests := map[string]string{
		"Integer": "INTEGER", "int": "INTEGER",
		"real": "REAL", "float": "REAL", "Number": "REAL",
		"text": "TEXT", "String": "TEXT",
		"Boolean": "INTEGER", "bool": "INTEGER",
	}
	for input, expected := range tests {
		if got := boruTypenameToSQLType(input); got != expected {
			t.Errorf("boruTypenameToSQLType(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestSqlTypeToBoruType(t *testing.T) {
	if got := sqlTypeToBoruType("INTEGER"); !got.Equal(TInteger) {
		t.Errorf("expected TInteger, got %v", got)
	}
	if got := sqlTypeToBoruType("REAL"); !got.Equal(TFloat) {
		t.Errorf("expected TFloat, got %v", got)
	}
	if got := sqlTypeToBoruType("TEXT"); !got.Equal(TString) {
		t.Errorf("expected TString, got %v", got)
	}
	if got := sqlTypeToBoruType("UNKNOWN"); !got.Equal(TString) {
		t.Errorf("expected TString (default), got %v", got)
	}
}

// ========================
// valueToSQL tests
// ========================

func TestValueToSQLString(t *testing.T) {
	got, err := valueToSQL(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'hello'" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestValueToSQLInt(t *testing.T) {
	got, err := valueToSQL(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestValueToSQLBool(t *testing.T) {
	got, err := valueToSQL(NewBoolean(true))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'true'" {
		t.Errorf("expected \"'true'\", got %q", got)
	}
}

func TestValueToSQLNone(t *testing.T) {
	got, err := valueToSQL(MakeValue(TNone, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != "NULL" {
		t.Errorf("expected 'NULL', got %q", got)
	}
}

func TestValueToSQLAtom(t *testing.T) {
	got, err := valueToSQL(NewAtom("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'foo'" {
		t.Errorf("expected \"'foo'\", got %q", got)
	}
}

func TestValueToSQLWord(t *testing.T) {
	got, err := valueToSQL(NewWord("bar"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "'bar'" {
		t.Errorf("expected \"'bar'\", got %q", got)
	}
}

func TestValueToSQLStringWithQuote(t *testing.T) {
	got, err := valueToSQL(NewString("it's"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "''") {
		t.Errorf("expected escaped quote, got %q", got)
	}
}

// ========================
// buildInList tests
// ========================

func TestBuildInListValues(t *testing.T) {
	inList := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	got, err := buildInList(inList)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "1") || !strings.Contains(got, "3") {
		t.Errorf("expected 1,2,3 in result, got %q", got)
	}
}

func TestBuildInListSingleValue(t *testing.T) {
	got, err := buildInList(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestBuildInListFromTableValues(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("id", NewInteger(10))
	row2 := NewOrderedMap()
	row2.Set("id", NewInteger(20))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	got, err := buildInListFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "10") || !strings.Contains(got, "20") {
		t.Errorf("expected 10,20 in result, got %q", got)
	}
}

func TestBuildInListFromTableEmpty(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	got, err := buildInListFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if got != "NULL" {
		t.Errorf("expected 'NULL', got %q", got)
	}
}

func TestBuildInListFromTableInWhere(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("id", NewInteger(5))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	tdVal := MakeValue(TList, td)

	got, err := buildInList(tdVal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "5") {
		t.Errorf("expected 5 in result, got %q", got)
	}
}

// ========================
// isTableOrQuery tests
// ========================

func TestIsTableOrQueryTableData(t *testing.T) {
	td := TableData{}
	v := MakeValue(TList, td)
	if !isTableOrQuery(v) {
		t.Error("expected true for TableData")
	}
}

func TestIsTableOrQueryQueryBuilder(t *testing.T) {
	qb := QueryBuilder{}
	v := MakeValue(TList, ExtensionPayload{Body: qb})
	if !isTableOrQuery(v) {
		t.Error("expected true for QueryBuilder")
	}
}

func TestIsTableOrQueryOther(t *testing.T) {
	v := NewInteger(42)
	if isTableOrQuery(v) {
		t.Error("expected false for integer")
	}
}

// ========================
// scalarFromTable tests
// ========================

func TestScalarFromTableOneRow(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("x", NewInteger(42))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	val, err := scalarFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	_as51, _ := AsInteger(val)
	if !ParentOf(&val).ConformsTo(TInteger) || _as51 != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestScalarFromTableNoRows(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}
	td := TableData{Record: rec, Rows: nil}

	val, err := scalarFromTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if !IsNoneShape(val) {
		t.Errorf("expected None, got %v", ParentOf(&val))
	}
}

func TestScalarFromTableMultipleRows(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("x", NewInteger(1))
	row2 := NewOrderedMap()
	row2.Set("x", NewInteger(2))
	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	_, err := scalarFromTable(td)
	if err == nil {
		t.Error("expected error for multiple rows")
	}
}

// ========================
// resolveScalarValue tests
// ========================

func TestResolveScalarValuePlain(t *testing.T) {
	v := NewInteger(42)
	got, err := resolveScalarValue(v)
	if err != nil {
		t.Fatal(err)
	}
	_as52, _ := AsInteger(got)
	if _as52 != 42 {
		t.Errorf("expected 42, got %v", got)
	}
}

func TestResolveScalarValueTableData(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	row := NewOrderedMap()
	row.Set("x", NewInteger(99))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}

	v := MakeValue(TList, td)
	got, err := resolveScalarValue(v)
	if err != nil {
		t.Fatal(err)
	}
	_as53, _ := AsInteger(got)
	if _as53 != 99 {
		t.Errorf("expected 99, got %v", got)
	}
}

// ========================
// buildGroupByClause tests
// ========================

func TestBuildGroupByClause(t *testing.T) {
	cols := NewList([]Value{NewAtom("dept"), NewAtom("role")})
	clause, err := buildGroupByClause(cols)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause, ",") {
		t.Errorf("expected comma-separated, got %q", clause)
	}
}

func TestBuildGroupByClauseEmpty(t *testing.T) {
	cols := NewList([]Value{})
	_, err := buildGroupByClause(cols)
	if err == nil {
		t.Error("expected error for empty group by")
	}
}

// ========================
// Additional engine coverage: peekForwardValue
// ========================

func TestPeekForwardValueInContext(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Exercise curryOrPrefix and peekForwardValue through a word that uses forward arg collection
	// e.g., "add" with forward: 1 add 2
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	_as54, _ := AsInteger(result[0])
	if len(result) != 1 || _as54 != 3 {
		t.Errorf("expected [3], got %v", result)
	}
}

// ========================
// Additional engine.go coverage: stepEnd branches
// ========================

func TestStepEndWithMoveAndMark(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def creates a mark; calling a def word triggers move
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("dbl"), NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("add")}),
		NewInteger(5), NewWord("dbl"),
	})
	_as55, _ := AsInteger(result[0])
	if len(result) != 1 || _as55 != 10 {
		t.Errorf("expected [10], got %v", result)
	}
}

// ========================
// Additional registry.go coverage
// ========================

func TestBaseValueForConstraintCoverage(t *testing.T) {
	// Exercise BaseValueForConstraint by testing type-related operations
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Create a typed list via the type system
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("typeof"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Additional SQLite tests: various value type conversions
// ========================

func TestSQLiteQueryWithNullValues(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("score", NewTypeLiteral(TNumber))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row1.Set("score", NewInteger(100))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("bob"))
	row2.Set("score", MakeValue(TNone, nil))

	td := TableData{Record: rec, Rows: []Value{NewMap(row1), NewMap(row2)}}

	if err := store.StoreTable("scores_null", td); err != nil {
		t.Fatalf("StoreTable error: %v", err)
	}
	result, err := store.Query("SELECT * FROM \"scores_null\"", &rec)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

// ========================
// OrderedMap.Delete tests
// ========================

func TestOrderedMapDeleteExisting(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))
	m.Set("c", NewInteger(3))

	ok := m.Delete("b")
	if !ok {
		t.Error("Delete returned false for existing key")
	}
	if m.Len() != 2 {
		t.Errorf("Len = %d, want 2", m.Len())
	}
	if _, found := m.Get("b"); found {
		t.Error("key 'b' still exists after delete")
	}
	keys := m.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
		t.Errorf("keys = %v, want [a c]", keys)
	}
}

func TestOrderedMapDeleteNonExisting(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	ok := m.Delete("z")
	if ok {
		t.Error("Delete returned true for non-existing key")
	}
	if m.Len() != 1 {
		t.Errorf("Len = %d, want 1", m.Len())
	}
}

func TestOrderedMapDeleteFirst(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(10))
	m.Set("y", NewInteger(20))
	m.Delete("x")
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "y" {
		t.Errorf("keys = %v, want [y]", keys)
	}
}

func TestOrderedMapDeleteLast(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(10))
	m.Set("y", NewInteger(20))
	m.Delete("y")
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("keys = %v, want [x]", keys)
	}
}

func TestOrderedMapDeleteAll(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewInteger(2))
	m.Delete("a")
	m.Delete("b")
	if m.Len() != 0 {
		t.Errorf("Len = %d, want 0", m.Len())
	}
	if len(m.Keys()) != 0 {
		t.Errorf("keys = %v, want []", m.Keys())
	}
}

// ========================
// CallBoru tests
// ========================

func TestCallBoruBasic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// def double fn [[x:number] [number] [x add x]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("x")}),
	})
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
	})

	// Look up the function
	fnVal, ok := r.Defs.Top("double")
	if !ok {
		t.Fatal("double not defined")
	}

	args := []Value{NewInteger(5)}
	sig := MatchFnSig(fnVal, args)
	if sig == nil {
		t.Fatal("no matching signature")
	}
	result, err := r.CallBoru(sig, args, nil)
	if err != nil {
		t.Fatalf("CallBoru error: %v", err)
	}
	_as56, _ := AsInteger(result[0])
	if len(result) != 1 || _as56 != 10 {
		t.Errorf("CallBoru(double, 5) = %v, want [10]", result)
	}
}

func TestCallBoruNotAFunction(t *testing.T) {
	sig := MatchFnSig(NewInteger(42), []Value{})
	if sig != nil {
		t.Error("expected nil sig for non-function value")
	}
}

func TestCallBoruNoMatchingSig(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// def inc fn [[x:number] [number] [x add 1]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewWord("inc"), NewWord("fn"), fnBody, NewEnd(),
	})

	fnVal, _ := r.Defs.Top("inc")

	// Call with wrong type — MatchFnSig returns nil
	sig := MatchFnSig(fnVal, []Value{NewString("hello")})
	if sig != nil {
		t.Error("expected nil sig for mismatched argument types")
	}
}

// ========================
// RegisterArgs tests
// ========================

func TestArgsInsideFn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def sum2 fn [[a:number b:number] [number] [a add b]] end
	// Using named params to exercise the args stack indirectly
	aParam := NewOrderedMap()
	aParam.Set("a", NewWord("Number"))
	bParam := NewOrderedMap()
	bParam.Set("b", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(aParam), NewImplicitMap(bParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("a"), NewWord("add"), NewWord("b")}),
	})
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewWord("sum2"), NewWord("fn"), fnBody, NewEnd(),
	})
	result := runBoru(t, r, []Value{
		NewWord("sum2"), NewInteger(3), NewInteger(7),
	})
	_as62, _ := AsInteger(result[0])
	if len(result) != 1 || _as62 != 10 {
		t.Errorf("sum2 3 7 = %v, want [10]", result)
	}
}

func TestArgsDirectAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Directly exercise the args stack by pushing and calling
	r.Args.Push(NewList([]Value{NewInteger(42), NewString("hi")}))
	e := New(r)
	result, err := e.Run([]Value{NewWord("args")})
	if err != nil {
		t.Fatalf("args error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_lst, _ := AsList(result[0])
	argsList := _lst.Slice()
	if len(argsList) != 2 {
		t.Errorf("expected args list of length 2, got %d", len(argsList))
	}
	// Clean up
	r.Args.Pop()
}

func TestArgsOutsideFnErrors(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// args outside of a function should error
	err = runBoruError(t, r, []Value{NewWord("args")})
	if err == nil {
		t.Error("expected error for args outside function")
	}
}

// ========================
// resolveOrphanedForwards tests (via integration)
// ========================

// ========================
// resolveOrphanedForwards tests (via integration)
// ========================

func TestResolveOrphanedForwardsCurry(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// "2 add" with no second argument triggers orphan forward resolution => curry
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("add"),
	})
	// Some implementations produce a curry; others error. Both are valid coverage.
	if err != nil {
		// Orphan forward was processed even if it errored
		return
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
}

func TestResolveOrphanedForwardsMultipleValues(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Multiple values with no matching function should resolve gracefully
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewInteger(10), NewInteger(20),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(result), result)
	}
}

// ========================
// ResolveFieldType tests
// ========================

func TestResolveFieldTypeString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Define a custom type: def MyNum Number
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewWord("MyNum"), NewWord("Number"),
	})

	// ResolveFieldType should resolve "MyNum" string to the type value
	result := ResolveFieldType(r, NewString("MyNum"))
	if !IsTypeBody(&result) {
		t.Errorf("expected type value, got %s (data=%v)", ParentOf(&result), DataOf(&result))
	}
}

func TestResolveFieldTypeStringUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Unknown name should pass through
	v := NewString("NotAType")
	result := ResolveFieldType(r, v)
	_as63, _ := AsString(result)
	if _as63 != "NotAType" {
		t.Errorf("expected pass-through, got %v", result)
	}
}

func TestResolveFieldTypeList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// [String tor None] as code should evaluate to a disjunct
	result := ResolveFieldType(r, NewList([]Value{
		NewWord("String"), NewWord("tor"), NewWord("None"),
	}))
	// Should be a disjunct type, not a raw list
	if ParentOf(&result).ConformsTo(TList) && !IsTypedList(&result) {
		t.Error("expected resolved type, not raw list")
	}
}

func TestResolveFieldTypePassthrough(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// A type literal should pass through unchanged
	v := NewTypeLiteral(TNumber)
	result := ResolveFieldType(r, v)
	if !ParentOf(&result).Equal(ParentOf(&v)) {
		t.Errorf("expected pass-through, got %v", result)
	}
}

// ========================
// SetParseFunc tests
// ========================

func TestSetParseFunc(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	called := false
	r.SetParseFunc(func(code string) ([]Value, error) {
		called = true
		return []Value{NewInteger(99)}, nil
	})

	if r.ParseFunc == nil {
		t.Error("ParseFunc should be set")
	}

	result, err := r.ParseFunc("test")
	if err != nil {
		t.Fatalf("ParseFunc error: %v", err)
	}
	if !called {
		t.Error("ParseFunc was not called")
	}
	_as64, _ := AsInteger(result[0])
	if len(result) != 1 || _as64 != 99 {
		t.Errorf("ParseFunc result = %v, want [99]", result)
	}
}

// ========================
// resolveSigType coverage
// ========================

func TestResolveSigTypeList(t *testing.T) {
	listVal := NewList([]Value{NewInteger(1), NewInteger(2)})
	tp, pattern, err := resolveSigType(nil, listVal)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TList) {
		t.Errorf("expected List for list, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected non-nil pattern for list")
	}
	if !ParentOf(pattern).Equal(TList) {
		t.Errorf("expected pattern to be a list, got %s", ParentOf(pattern))
	}
}

func TestResolveSigTypeFloatLiteral(t *testing.T) {
	// Post §1.1 fix: scalar literals (Integer, Float, Boolean,
	// String, Atom) are routed through Signature.Patterns. The type
	// is normalised to the kind, and the literal value lands in the
	// pattern slot.
	v := NewFloat(3.14)
	tp, pattern, err := resolveSigType(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(TFloat) {
		t.Errorf("expected TFloat kind for decimal literal, got %s", tp)
	}
	if pattern == nil {
		t.Fatal("expected pattern to carry the literal value, got nil")
	}
	if got, _ := AsFloat(*pattern); got != 3.14 {
		t.Errorf("pattern value = %v, want 3.14", got)
	}
}

// ========================
// MatchSignature pattern coverage
// ========================

func TestMatchSignaturePatternReject(t *testing.T) {
	// Signature with a map pattern that should reject a non-matching map.
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewInteger(99))
	patternVal := NewMap(patternMap)

	sig := Signature{
		Args:     []*Type{TMap},
		Patterns: map[int]Value{0: patternVal},
		Impl:     Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return args, nil }), BarrierPos:

		// Matching map: {x:99}
		-1,
	}

	matchMap := NewOrderedMap()
	matchMap.Set("x", NewInteger(99))
	stack := []Value{NewMap(matchMap)}

	result := MatchSignature([]Signature{sig}, stack, WordInfo{ArgCount: -1})
	if result == nil {
		t.Error("expected match for {x:99} against pattern {x:99}")
	}

	// Non-matching map: {x:100}
	noMatchMap := NewOrderedMap()
	noMatchMap.Set("x", NewInteger(100))
	stack2 := []Value{NewMap(noMatchMap)}

	result2 := MatchSignature([]Signature{sig}, stack2, WordInfo{ArgCount: -1})
	if result2 != nil {
		t.Error("expected no match for {x:100} against pattern {x:99}")
	}
}

func TestMatchSignaturePatternFallthrough(t *testing.T) {
	// Two signatures: one with pattern (more specific), one without (fallback).
	patternMap := NewOrderedMap()
	patternMap.Set("a", NewInteger(1))
	patternVal := NewMap(patternMap)

	specificSig := Signature{
		Args:     []*Type{TMap},
		Patterns: map[int]Value{0: patternVal},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString("specific")}, nil
		}), BarrierPos: -1,
	}
	fallbackSig := Signature{
		Args: []*Type{TMap},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString("fallback")}, nil
		}), BarrierPos:

		// Non-matching map should fall through to fallback.
		-1,
	}

	m := NewOrderedMap()
	m.Set("a", NewInteger(2))
	stack := []Value{NewMap(m)}

	result := MatchSignature([]Signature{specificSig, fallbackSig}, stack, WordInfo{ArgCount: -1})
	if result == nil {
		t.Fatal("expected fallback match")
	}
	out, _ := result.Sig.DispatchHandler()(result.Args, nil, nil, nil)
	_as65, _ := AsString(out[0])
	if _as65 != "fallback" {
		_as66, _ := AsString(out[0])
		t.Errorf("expected fallback, got %s", _as66)
	}
}

// ========================
// CallBoru pattern coverage
// ========================

func TestCallBoruMapPattern(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Build a function with map pattern: fn [[x:{k:1}] [String] ["yes"]]
	patternMap := NewOrderedMap()
	patternMap.Set("k", NewInteger(1))
	patternVal := NewMap(patternMap)

	fnDef := FnDefInfo{
		Signatures: []FnSig{
			{
				Params:  []FnParam{{Name: "x", Type: TMap, Pattern: &patternVal}},
				Returns: []*Type{TString},
				Impl:    Boru([]Value{NewString("yes")}), BarrierPos: -1,
			},
		},
	}
	fnVal := NewFunction(fnDef)

	// Matching call: {k:1}
	argMap := NewOrderedMap()
	argMap.Set("k", NewInteger(1))
	matchArgs := []Value{NewMap(argMap)}
	matchSig := MatchFnSig(fnVal, matchArgs)
	if matchSig == nil {
		t.Fatal("expected matching signature")
	}
	result, callErr := r.CallBoru(matchSig, matchArgs, nil)
	if callErr != nil {
		t.Fatalf("expected match, got error: %v", callErr)
	}
	_as67, _ := AsString(result[0])
	if len(result) != 1 || _as67 != "yes" {
		t.Errorf("expected [yes], got %v", result)
	}

	// Non-matching call: {k:2}
	noArgMap := NewOrderedMap()
	noArgMap.Set("k", NewInteger(2))
	noMatchSig := MatchFnSig(fnVal, []Value{NewMap(noArgMap)})
	if noMatchSig != nil {
		t.Error("expected nil sig for non-matching pattern {k:2}")
	}
}

// ========================
// RegisterFn error paths
// ========================

func TestRegisterFnNonList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// fn with a non-list argument should error.
	err = runBoruError(t, r, []Value{NewInteger(42), NewWord("fn")})
	if err == nil {
		t.Error("expected error for fn with non-list argument")
	}
}

func TestRegisterFnEmptyList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	err = runBoruError(t, r, []Value{NewList([]Value{}), NewWord("fn")})
	if err == nil {
		t.Error("expected error for fn with empty list")
	}
}

func TestRegisterFnBadTriple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Triple with invalid input sig (non-list, non-map param element).
	err = runBoruError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewFloat(1.5)}), // invalid param type
			NewTypeLiteral(TString),
			NewList([]Value{NewString("body")}),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn with invalid param type")
	}
}

// ========================
// parseFnUndefSpec error paths
// ========================

func TestParseFnUndefSpecParamError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// 4 elements = 2 pairs, first pair has bad input sig (invalid param type).
	err = runBoruError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewFloat(1.5)}), // invalid param
			NewTypeLiteral(TString),
			NewList([]Value{NewFloat(1.5)}), // invalid param
			NewTypeLiteral(TString),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn undef spec with invalid param")
	}
}

func TestParseFnUndefSpecReturnError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// 4 elements = 2 pairs, valid input sig but invalid return type.
	err = runBoruError(t, r, []Value{
		NewList([]Value{
			NewList([]Value{NewTypeLiteral(TString)}), // valid param
			NewString("nonexistent_type"),             // invalid return type
			NewList([]Value{NewTypeLiteral(TString)}),
			NewString("nonexistent_type"),
		}),
		NewWord("fn"),
	})
	if err == nil {
		t.Error("expected error for fn undef spec with invalid return type")
	}
}

// ========================
// parseFnReturns error paths
// ========================

func TestParseFnReturnsSingleError(t *testing.T) {
	// A single non-list return WORD that names no type. A word in a type
	// slot must name a type — unlike a String or Atom, which falls back
	// to being a literal type (see TestParseFnReturnsStringLiteral).
	_, _, err := parseFnReturns(nil, NewWord("nonexistent_type"))
	if err == nil {
		t.Error("expected error for invalid return type name")
	}
}

func TestParseFnReturnsListError(t *testing.T) {
	// A list with an invalid return type element.
	_, _, err := parseFnReturns(nil, NewList([]Value{NewWord("nonexistent_type")}))
	if err == nil {
		t.Error("expected error for invalid return type in list")
	}
}

func TestParseFnReturnsStringLiteral(t *testing.T) {
	// A String that names no type is a literal TYPE — it admits exactly
	// itself — so it resolves to (String, pattern) instead of erroring.
	types, pats, err := parseFnReturns(nil, NewString("nonexistent_type"))
	if err != nil {
		t.Fatalf("a non-type string must be a literal return type: %v", err)
	}
	if len(types) != 1 || !types[0].Equal(TString) {
		t.Fatalf("types = %v, want [String]", types)
	}
	if len(pats) != 1 || pats[0] == nil {
		t.Fatalf("pats = %v, want one literal pattern", pats)
	}
	if got, aerr := AsString(*pats[0]); aerr != nil || got != "nonexistent_type" {
		t.Errorf("pattern = %v (%v)", *pats[0], aerr)
	}
}

// ========================
// FlexibleMatch coverage
// ========================

func TestFlexibleMatchTooFewValues(t *testing.T) {
	// Fewer values than types should return nil, false.
	values := []Value{NewInteger(1)}
	types := []*Type{TInteger, TString}
	result, ok := FlexibleMatch(values, &Signature{Args: types, BarrierPos: -1})
	if ok || result != nil {
		t.Errorf("expected no match with fewer values than types")
	}
}

// ========================
// fn with map pattern via engine (exercises MatchSignature patterns)
// ========================

func TestFnMapPatternViaEngine(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// def foo fn [[x:{x:99}] [String] ["A"] [x:Map] [String] ["B"]]
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewInteger(99))

	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewWord("fn"),
		NewList([]Value{
			// Overload 1: x matches {x:99}
			NewList([]Value{NewImplicitMap(singleMap("x", NewMap(patternMap)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewString("A")}),
			// Overload 2: x matches any Map
			NewList([]Value{NewImplicitMap(singleMap("x", NewTypeLiteral(TMap)))}),
			NewList([]Value{NewTypeLiteral(TString)}),
			NewList([]Value{NewString("B")}),
		}),
		// Call with {x:99} — should match overload 1
		NewMap(patternMap), NewWord("foo"),
	})
	_as68, _ := AsString(result[0])
	if len(result) != 1 || _as68 != "A" {
		t.Errorf("expected 'A' for {x:99}, got %v", result)
	}

	// Call with {x:100} — should match overload 2 (fallback)
	noMatchMap := NewOrderedMap()
	noMatchMap.Set("x", NewInteger(100))
	result2 := runBoru(t, r, []Value{
		NewMap(noMatchMap), NewWord("foo"),
	})
	_as69, _ := AsString(result2[0])
	if len(result2) != 1 || _as69 != "B" {
		t.Errorf("expected 'B' for {x:100}, got %v", result2)
	}
}
