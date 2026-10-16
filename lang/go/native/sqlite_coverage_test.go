package native

import (
	"testing"
)

func TestBoruTypeToSQLType(t *testing.T) {
	tests := []struct {
		typ  *Type
		want string
	}{
		{TInteger, "INTEGER"},
		{TFloat, "REAL"},
		{TNumber, "REAL"},
		{TBoolean, "INTEGER"},
		{TString, "TEXT"},
		{TAny, "TEXT"},
	}
	for _, tt := range tests {
		got := boruTypeToSQLType(tt.typ)
		if got != tt.want {
			t.Errorf("boruTypeToSQLType(%s) = %s, want %s", tt.typ, got, tt.want)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		input any
		want  int64
	}{
		{int64(42), 42},
		{float64(3.7), 3},
		{"123", 123},
		{[]byte("456"), 456},
		{true, 0},  // default case
		{"abc", 0}, // unparseable string
	}
	for _, tt := range tests {
		got := toInt64(tt.input)
		if got != tt.want {
			t.Errorf("toInt64(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input any
		want  float64
	}{
		{float64(3.14), 3.14},
		{int64(42), 42.0},
		{"1.5", 1.5},
		{[]byte("2.5"), 2.5},
		{true, 0},  // default case
		{"abc", 0}, // unparseable string
	}
	for _, tt := range tests {
		got := toFloat64(tt.input)
		if got != tt.want {
			t.Errorf("toFloat64(%v) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{"hello", "hello"},
		{[]byte("world"), "world"},
		{int64(42), "42"},
		{float64(3.14), "3.14"},
		{nil, ""},
		{true, "true"}, // default case
	}
	for _, tt := range tests {
		got := toString(tt.input)
		if got != tt.want {
			t.Errorf("toString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBoruValueToSQLParam(t *testing.T) {
	// Integer column
	got := boruValueToSQLParam(NewInteger(42), TInteger)
	if got != int64(42) {
		t.Errorf("expected 42, got %v", got)
	}

	// Integer column with string value
	got = boruValueToSQLParam(NewString("123"), TInteger)
	if got != int64(123) {
		t.Errorf("expected 123, got %v", got)
	}

	// Integer column with boolean
	got = boruValueToSQLParam(NewBoolean(true), TInteger)
	if got != int64(1) {
		t.Errorf("expected 1, got %v", got)
	}
	got = boruValueToSQLParam(NewBoolean(false), TInteger)
	if got != int64(0) {
		t.Errorf("expected 0, got %v", got)
	}

	// Integer column with non-numeric string (fallback to text)
	got = boruValueToSQLParam(NewString("abc"), TInteger)
	if got != "abc" {
		t.Errorf("expected abc, got %v", got)
	}

	// Number column with decimal
	got = boruValueToSQLParam(NewFloat(3.14), TNumber)
	if got != float64(3.14) {
		t.Errorf("expected 3.14, got %v", got)
	}

	// Number column with integer
	got = boruValueToSQLParam(NewInteger(5), TNumber)
	if got != float64(5) {
		t.Errorf("expected 5.0, got %v", got)
	}

	// Number column with numeric string
	got = boruValueToSQLParam(NewString("2.5"), TNumber)
	if got != float64(2.5) {
		t.Errorf("expected 2.5, got %v", got)
	}

	// Number column with non-numeric string
	got = boruValueToSQLParam(NewString("abc"), TNumber)
	if got != "abc" {
		t.Errorf("expected abc, got %v", got)
	}

	// Boolean column with boolean
	got = boruValueToSQLParam(NewBoolean(true), TBoolean)
	if got != int64(1) {
		t.Errorf("expected 1, got %v", got)
	}
	got = boruValueToSQLParam(NewBoolean(false), TBoolean)
	if got != int64(0) {
		t.Errorf("expected 0, got %v", got)
	}

	// Boolean column with string "true"
	got = boruValueToSQLParam(NewString("true"), TBoolean)
	if got != int64(1) {
		t.Errorf("expected 1, got %v", got)
	}

	// Boolean column with string "false"
	got = boruValueToSQLParam(NewString("false"), TBoolean)
	if got != int64(0) {
		t.Errorf("expected 0, got %v", got)
	}

	// Boolean column with non-boolean value
	got = boruValueToSQLParam(NewInteger(42), TBoolean)
	if _, ok := got.(string); !ok {
		t.Errorf("expected string fallback, got %T", got)
	}

	// Text column with string
	got = boruValueToSQLParam(NewString("hello"), TString)
	if got != "hello" {
		t.Errorf("expected hello, got %v", got)
	}

	// Text column with non-string
	got = boruValueToSQLParam(NewInteger(42), TString)
	if _, ok := got.(string); !ok {
		t.Errorf("expected string fallback, got %T", got)
	}

	// None type
	got = boruValueToSQLParam(ValueOf(&Datum{Parent: TNone}), TString)
	if got != nil {
		t.Errorf("expected nil for None, got %v", got)
	}
}

func TestSQLiteStoreBasic(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a simple table
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TInteger))
	fields.Set("score", NewTypeLiteral(TFloat))
	fields.Set("active", NewTypeLiteral(TBoolean))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("alice"))
	row1.Set("age", NewInteger(30))
	row1.Set("score", NewFloat(95.5))
	row1.Set("active", NewBoolean(true))

	td := TableData{
		Record: rec,
		Rows:   []Value{NewMap(row1)},
	}

	err = store.StoreTable("test", td)
	if err != nil {
		t.Fatal(err)
	}

	if !store.HasTable("test") {
		t.Error("expected table to exist")
	}

	// Query all rows
	result, err := store.Query(`SELECT * FROM "test"`, &rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// Drop table
	store.DropTable("test")
	if store.HasTable("test") {
		t.Error("expected table to be dropped")
	}
}

func TestSQLiteStoreEmptyTable(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Table with no columns
	fields := NewOrderedMap()
	td := TableData{Record: RecordTypeInfo{Fields: fields}}
	err = store.StoreTable("empty", td)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteStoreTempTableCoverage(t *testing.T) {
	store, err := NewSQLiteStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	td := TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{},
	}

	name, err := store.StoreTempTable(td)
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Error("expected non-empty temp table name")
	}
}

func TestSQLiteCloseNilDB(t *testing.T) {
	s := &SQLiteStore{}
	err := s.Close()
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestSqlResultToBoruValue(t *testing.T) {
	// nil
	v := sqlResultToBoruValue(nil, TString)
	if !v.Parent().Equal(TNone) {
		t.Errorf("expected none, got %s", v.Parent())
	}

	// Integer
	v = sqlResultToBoruValue(int64(42), TInteger)
	_as0, _ := AsInteger(v)
	if _as0 != 42 {
		_as1, _ := AsInteger(v)
		t.Errorf("expected 42, got %d", _as1)
	}

	// Number
	v = sqlResultToBoruValue(float64(3.14), TNumber)
	_as2, _ := AsNumber(v)
	if _as2 != 3.14 {
		_as3, _ := AsNumber(v)
		t.Errorf("expected 3.14, got %f", _as3)
	}

	// Boolean
	v = sqlResultToBoruValue(int64(1), TBoolean)
	_as4, _ := AsBoolean(v)
	if !_as4 {
		t.Error("expected true")
	}

	// String
	v = sqlResultToBoruValue("hello", TString)
	_as5, _ := AsString(v)
	if _as5 != "hello" {
		_as6, _ := AsString(v)
		t.Errorf("expected hello, got %s", _as6)
	}
}
