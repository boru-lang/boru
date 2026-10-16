package native

import (
	"strings"
	"testing"

	multisource "github.com/tabnas/multisource/go"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

func TestTextFormatDecode(t *testing.T) {
	f := &TextFormat{}
	result, err := f.Decode("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as0, _ := AsString(result[0])
	if len(result) != 1 || _as0 != "hello world" {
		t.Errorf("got %v, want ['hello world']", result)
	}
}

func TestTextFormatEncode(t *testing.T) {
	f := &TextFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}

	// Non-string uses String()
	s, err = f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJSONFormatDecode(t *testing.T) {
	f := &JSONFormat{}
	result, err := f.Decode(`{"x":1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].Parent().Equal(TMap) {
		t.Errorf("expected map, got %s", result[0].Parent())
	}
}

func TestJSONFormatDecodeError(t *testing.T) {
	f := &JSONFormat{}
	_, err := f.Decode(`{invalid`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestJSONFormatEncode(t *testing.T) {
	f := &JSONFormat{}
	s, err := f.Encode(NewInteger(42))
	if err != nil {
		t.Fatal(err)
	}
	if s != "42" {
		t.Errorf("got %q, want %q", s, "42")
	}
}

func TestJsonicFormatDecode(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`{x:1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatDecodeNull(t *testing.T) {
	f := &JsonicFormat{}
	result, err := f.Decode(`null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestJsonicFormatDecodeError(t *testing.T) {
	f := &JsonicFormat{}
	_, err := f.Decode(`{{{`)
	if err == nil {
		t.Error("expected error for invalid jsonic")
	}
}

func TestJsonicFormatEncode(t *testing.T) {
	f := &JsonicFormat{}
	s, err := f.Encode(NewString("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if s != `"hi"` {
		t.Errorf("got %q, want %q", s, `"hi"`)
	}
}

func TestLinesFormatDecode(t *testing.T) {
	f := &LinesFormat{}
	result, err := f.Decode("a\nb\nc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_lst, _ := AsList(result[0])
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(elems))
	}
	_as3, _ := AsString(elems[0])
	_as2, _ := AsString(elems[1])
	_as1, _ := AsString(elems[2])
	if _as3 != "a" || _as2 != "b" || _as1 != "c" {
		t.Errorf("got %v", elems)
	}
}

func TestLinesFormatEncode(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewString("x"), NewString("y")})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "x\ny" {
		t.Errorf("got %q, want %q", s, "x\ny")
	}
}

func TestLinesFormatEncodeNonString(t *testing.T) {
	f := &LinesFormat{}
	list := NewList([]Value{NewInteger(1), NewInteger(2)})
	s, err := f.Encode(list)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1\n2" {
		t.Errorf("got %q, want %q", s, "1\n2")
	}
}

func TestLinesFormatEncodeNonList(t *testing.T) {
	f := &LinesFormat{}
	s, err := f.Encode(NewString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "'hello'" {
		t.Errorf("got %q, want %q", s, "'hello'")
	}
}

func TestDefaultFormats(t *testing.T) {
	fmts := DefaultFormats()
	for _, name := range []string{"text", "json", "jsonic", "lines", "csv", "tsv"} {
		if _, ok := fmts[name]; !ok {
			t.Errorf("missing format: %s", name)
		}
	}
}

// --- CSV format tests ---

func TestCSVFormatDecode(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("name,age\nAlice,30\nBob,25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v)
	}

	_lst, _ := AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Check first row
	r0, _ := AsMap(rows[0])
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	_as4, _ := AsString(nameVal)
	if _as4 != "Alice" {
		_as5, _ := AsString(nameVal)
		t.Errorf("name = %q, want %q", _as5, "Alice")
	}
	ageVal, ok := r0.Get("age")
	if !ok {
		t.Fatal("expected 'age' key")
	}
	_as6, _ := AsString(ageVal)
	if _as6 != "30" {
		_as7, _ := AsString(ageVal)
		t.Errorf("age = %q, want %q", _as7, "30")
	}

	// Check second row
	r1, _ := AsMap(rows[1])
	nameVal, _ = r1.Get("name")
	_as8, _ := AsString(nameVal)
	if _as8 != "Bob" {
		_as9, _ := AsString(nameVal)
		t.Errorf("name = %q, want %q", _as9, "Bob")
	}
}

func TestCSVFormatDecodeEmpty(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("name,age\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_lst, _ := AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestCSVFormatDecodeQuoted(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("a,b\n\"hello, world\",2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_lst, _ := AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	m, _ := AsMap(rows[0])
	aVal, _ := m.Get("a")
	_as10, _ := AsString(aVal)
	if _as10 != "hello, world" {
		_as11, _ := AsString(aVal)
		t.Errorf("a = %q, want %q", _as11, "hello, world")
	}
}

func TestCSVFormatDecodeTableSchema(t *testing.T) {
	f := &CSVFormat{}
	result, err := f.Decode("x,y\n1,2")
	if err != nil {
		t.Fatal(err)
	}
	v := result[0]
	tt, _ := AsTableType(v)
	fields := tt.Record.Fields
	if fields.Len() != 2 {
		t.Errorf("expected 2 fields, got %d", fields.Len())
	}
	xType, ok := fields.Get("x")
	if !ok {
		t.Fatal("expected field 'x'")
	}
	if !(&xType).Equal(TString) {
		t.Errorf("expected string type for x, got %s", xType.String())
	}
}

func TestCSVFormatEncode(t *testing.T) {
	f := &CSVFormat{}
	// Create a table data value
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("age", NewString("30"))
	r0.Set("name", NewString("Alice"))
	r1 := NewOrderedMap()
	r1.Set("age", NewString("25"))
	r1.Set("name", NewString("Bob"))

	table := ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0), NewMap(r1)},
	}})

	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "name") || !strings.Contains(s, "age") {
		t.Errorf("encoded CSV missing headers: %q", s)
	}
	if !strings.Contains(s, "Alice") || !strings.Contains(s, "Bob") {
		t.Errorf("encoded CSV missing data: %q", s)
	}
}

func TestCSVFormatEncodeQuoted(t *testing.T) {
	f := &CSVFormat{}
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("a", NewString("hello, world"))
	table := ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0)},
	}})
	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"hello, world"`) {
		t.Errorf("expected quoted field in: %q", s)
	}
}

// --- TSV format tests ---

func TestTSVFormatDecode(t *testing.T) {
	f := &TSVFormat{}
	result, err := f.Decode("name\tage\nAlice\t30\nBob\t25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v)
	}

	_lst, _ := AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0, _ := AsMap(rows[0])
	nameVal, _ := r0.Get("name")
	_as12, _ := AsString(nameVal)
	if _as12 != "Alice" {
		_as13, _ := AsString(nameVal)
		t.Errorf("name = %q, want %q", _as13, "Alice")
	}
}

func TestTSVFormatEncode(t *testing.T) {
	f := &TSVFormat{}
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TString))
	fields.Set("b", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	r0 := NewOrderedMap()
	r0.Set("a", NewString("x"))
	r0.Set("b", NewString("y"))
	table := ValueOf(&Datum{Parent: TList, Data: TableData{
		Record: rec,
		Rows:   []Value{NewMap(r0)},
	}})
	s, err := f.Encode(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "a\tb") {
		t.Errorf("expected tab-separated headers in: %q", s)
	}
	if !strings.Contains(s, "x\ty") {
		t.Errorf("expected tab-separated data in: %q", s)
	}
}

// --- Multisource integration tests ---

func TestJsonicFormatMultisourceResolves(t *testing.T) {
	// Set up in-memory files for the resolver.
	mem := capabilities.NewMem()
	mem.Files["part.jsonic"] = []byte(`{x:1}`)

	f := &JsonicFormat{
		Resolver: MakeFileOpsResolver(mem),
	}

	// The @"part.jsonic" reference should be resolved and merged.
	result, err := f.Decode(`{@"part.jsonic", y:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x' from resolved file")
	}
	_as14, _ := AsInteger(xVal)
	if _as14 != 1 {
		t.Errorf("x = %v, want 1", xVal)
	}
	yVal, ok := m.Get("y")
	if !ok {
		t.Fatal("expected key 'y'")
	}
	_as15, _ := AsInteger(yVal)
	if _as15 != 2 {
		t.Errorf("y = %v, want 2", yVal)
	}
}

func TestJsonicFormatMultisourceNested(t *testing.T) {
	// Test nested multisource: a.jsonic references b.jsonic.
	mem := capabilities.NewMem()
	mem.Files["b.jsonic"] = []byte(`{nested: true}`)
	mem.Files["a.jsonic"] = []byte(`{@"b.jsonic", top: 1}`)

	f := &JsonicFormat{
		Resolver: MakeFileOpsResolver(mem),
	}

	result, err := f.Decode(`{@"a.jsonic", outer: 99}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, _ := AsMap(result[0])

	outerVal, ok := m.Get("outer")
	if !ok {
		t.Fatal("expected key 'outer'")
	}
	_as16, _ := AsInteger(outerVal)
	if _as16 != 99 {
		t.Errorf("outer = %v, want 99", outerVal)
	}

	topVal, ok := m.Get("top")
	if !ok {
		t.Fatal("expected key 'top' from a.jsonic")
	}
	_as17, _ := AsInteger(topVal)
	if _as17 != 1 {
		t.Errorf("top = %v, want 1", topVal)
	}

	nestedVal, ok := m.Get("nested")
	if !ok {
		t.Fatal("expected key 'nested' from b.jsonic")
	}
	_as18, _ := AsBoolean(nestedVal)
	if !_as18 {
		t.Errorf("nested = %v, want true", nestedVal)
	}
}

func TestJsonicFormatWithoutResolverNoMultisource(t *testing.T) {
	// Without a resolver, the jsonic format should work as before
	// (no multisource, just plain jsonic parsing).
	f := &JsonicFormat{}
	result, err := f.Decode(`{a:1, b:2}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
}

func TestJsonicFormatMultisourceNotUsedForJSON(t *testing.T) {
	// JSONFormat must NOT use multisource — it's strict JSON only.
	f := &JSONFormat{}
	// This is valid JSON with an @ in a key — should parse as-is.
	result, err := f.Decode(`{"@ref": "value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("@ref")
	if !ok {
		t.Fatal("expected key '@ref' in JSON output")
	}
	_as19, _ := AsString(v)
	if _as19 != "value" {
		_as20, _ := AsString(v)
		t.Errorf("got %q, want %q", _as20, "value")
	}
}

func TestJsonicFormatMultisourceNotUsedForText(t *testing.T) {
	// TextFormat must NOT use multisource.
	f := &TextFormat{}
	result, err := f.Decode(`@"somefile.jsonic"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Text format returns raw string, no resolution.
	_as21, _ := AsString(result[0])
	if _as21 != `@"somefile.jsonic"` {
		_as22, _ := AsString(result[0])
		t.Errorf("text format should return raw content, got %q", _as22)
	}
}

func TestMakeFileOpsResolverFindsFile(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["data.jsonic"] = []byte(`{found:true}`)

	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "data.jsonic",
		Kind: "jsonic",
	}
	res := resolver(spec, nil, nil)
	if !res.Found {
		t.Fatal("expected resolver to find data.jsonic")
	}
	if res.Src != `{found:true}` {
		t.Errorf("got src %q, want {found:true}", res.Src)
	}
}

func TestMakeFileOpsResolverNotFound(t *testing.T) {
	mem := capabilities.NewMem()
	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "missing.jsonic",
		Kind: "jsonic",
	}
	res := resolver(spec, nil, nil)
	if res.Found {
		t.Fatal("expected resolver to NOT find missing.jsonic")
	}
}

func TestMakeFileOpsResolverImplicitExt(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["config.jsonic"] = []byte(`{ok:true}`)

	resolver := MakeFileOpsResolver(mem)
	spec := multisource.PathSpec{
		Full: "config",
		Kind: "", // no extension → try implicit
	}
	opts := &multisource.MultiSourceOptions{
		ImplicitExt: []string{"jsonic", "json"},
	}
	res := resolver(spec, opts, nil)
	if !res.Found {
		t.Fatal("expected resolver to find config via implicit .jsonic ext")
	}
	if res.Kind != "jsonic" {
		t.Errorf("got kind %q, want %q", res.Kind, "jsonic")
	}
}

func TestRegistryJsonicFormatHasResolver(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	jf, ok := HostFormats(r)["jsonic"].(*JsonicFormat)
	if !ok {
		t.Fatal("jsonic format should be *JsonicFormat")
	}
	if jf.Resolver == nil {
		t.Error("jsonic format in registry should have a resolver set")
	}
}

func TestRegistryJSONFormatHasNoResolver(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// JSON format should remain unchanged — no multisource.
	_, ok := HostFormats(r)["json"].(*JSONFormat)
	if !ok {
		t.Fatal("json format should be *JSONFormat, not modified")
	}
}

func TestSetFileOpsUpdatesJsonicResolver(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	mem := capabilities.NewMem()
	mem.Files["test.jsonic"] = []byte(`{val:42}`)
	SetHostFileOps(r, mem)

	jf := HostFormats(r)["jsonic"].(*JsonicFormat)
	if jf.Resolver == nil {
		t.Fatal("expected resolver to be updated after SetFileOps")
	}

	// Verify the resolver uses the new FileOps.
	result, err := jf.Decode(`{@"test.jsonic"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("val")
	if !ok {
		t.Fatal("expected key 'val' from resolved test.jsonic")
	}
	_as23, _ := AsInteger(v)
	if _as23 != 42 {
		t.Errorf("val = %v, want 42", v)
	}
}
