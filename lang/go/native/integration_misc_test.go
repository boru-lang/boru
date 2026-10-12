package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// --- ValToString coverage ---

func TestValToString(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"string", NewString("hello"), "hello"},
		{"integer", NewInteger(42), "42"},
		{"atom", NewAtom("foo"), "foo"},
		{"boolean", NewBoolean(true), "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValToString(tt.val)
			if got != tt.want {
				t.Errorf("ValToString(%s) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestEngineReadCSVByExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.csv"] = []byte("name,age\nAlice,30\nBob,25")
	SetHostFileOps(r, mem)

	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.csv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent)
	}
	_lst, _ := AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	r0, _ := AsMap(rows[0])
	nameVal, ok := r0.Get("name")
	if !ok {
		t.Fatal("expected 'name' key")
	}
	_as98, _ := AsString(nameVal)
	if _as98 != "Alice" {
		_as99, _ := AsString(nameVal)
		t.Errorf("name = %q, want %q", _as99, "Alice")
	}
}

func TestEngineReadTSVByExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.tsv"] = []byte("name\tage\nAlice\t30\nBob\t25")
	SetHostFileOps(r, mem)

	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.tsv")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent)
	}
	_lst, _ := AsList(v)
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestEngineReadCSVExplicitFormat(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.txt"] = []byte("a,b\n1,2")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("csv"))
	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.txt"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent)
	}
}

func TestEngineReadOverrideExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.csv"] = []byte("hello,world")
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.csv"), NewMap(opts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	// With text format, we get a plain string, not a table
	if IsTableType(result[0]) {
		t.Error("expected non-table type with text format override")
	}
	_as100, _ := AsString(result[0])
	if _as100 != "hello,world" {
		_as101, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as101, "hello,world")
	}
}

func TestEngineReadJSONByExtension(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := capabilities.NewMem()
	mem.Files["data.json"] = []byte(`{"key":"value"}`)
	SetHostFileOps(r, mem)

	result := runBoru(t, r, []Value{NewWord("read"), pathV("data.json")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	if !result[0].Parent.Equal(TMap) {
		t.Errorf("expected map type, got %s", result[0].Parent)
	}
}

// --- Inspect word tests ---

func TestEngineInspectBuiltin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// inspect add => word_inspection map
	result := runBoru(t, r, []Value{NewWord("inspect"), NewWord("add")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.Parent.Equal(TInspect) {
		t.Fatalf("expected type %s, got %s", TInspect, v.Parent)
	}
	m, _ := AsMap(v)

	// Check name field.
	name, ok := m.Get("name")
	_as102, _ := AsString(name)
	if !ok || _as102 != "add" {
		t.Errorf("name = %v, want 'add'", name)
	}

	// Check kind field.
	kind, ok := m.Get("kind")
	_as103, _ := AsAtom(kind)
	if !ok || _as103 != "native" {
		t.Errorf("kind = %v, want native", kind)
	}

	// Check signatures field is a non-empty list.
	sigs, ok := m.Get("signatures")
	if !ok {
		t.Fatal("missing signatures field")
	}
	_lst, _ := AsList(sigs)
	sigList := _lst.Slice()
	if len(sigList) == 0 {
		t.Error("expected at least one signature for add")
	}

	// Check the first signature has args. `add` carries several overloads
	// (the 2-arg arithmetic/scalar/temporal forms plus patrun's 3-arg
	// [Map Any Patrun] rule form), and SortSignatures decides which lands
	// first, so assert the count is in the valid range rather than pinning a
	// single arity.
	sig0, _ := AsMap(sigList[0])
	args, _ := sig0.Get("args")
	_lst2, _ := AsList(args)
	argList := _lst2.Slice()
	if len(argList) < 2 || len(argList) > 3 {
		t.Errorf("expected 2 or 3 args for add's first signature, got %d", len(argList))
	}
}

func TestEngineInspectUserDefined(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def double [2 mul] ; inspect double
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"), NewList([]Value{NewInteger(2), NewWord("mul")}),
		NewWord("inspect"), NewWord("double"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m, _ := AsMap(result[0])

	kind, _ := m.Get("kind")
	_as104, _ := AsAtom(kind)
	if _as104 != "defined" {
		t.Errorf("kind = %v, want defined", kind)
	}

	name, _ := m.Get("name")
	_as105, _ := AsString(name)
	if _as105 != "double" {
		t.Errorf("name = %v, want 'double'", name)
	}
}

func TestEngineInspectUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewWord("inspect"), NewAtom("nonexistent")})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m, _ := AsMap(result[0])

	kind, _ := m.Get("kind")
	_as106, _ := AsAtom(kind)
	if _as106 != "unknown" {
		t.Errorf("kind = %v, want unknown", kind)
	}

	sigs, _ := m.Get("signatures")
	_lst, _ := AsList(sigs)
	if len(_lst.Slice()) != 0 {
		t.Errorf("expected empty signatures for unknown word")
	}
}

func TestEngineInspectDotAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// inspect upper .name => 'upper'
	result := runBoru(t, r, []Value{
		NewWord("inspect"), NewWord("upper"),
		NewWord("dot"), NewWord("name"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_as107, _ := AsString(result[0])
	if _as107 != "upper" {
		t.Errorf("inspect upper .name = %v, want 'upper'", result[0])
	}
}

func TestEngineInspectTypeLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def Qty number ; inspect Qty
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Qty"), NewTypeLiteral(TNumber),
		NewWord("inspect"), NewWord("Qty"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	v := result[0]
	if !v.Parent.Equal(TInspect) {
		t.Fatalf("expected type %s, got %s", TInspect, v.Parent)
	}
	m, _ := AsMap(v)

	name, _ := m.Get("name")
	_as108, _ := AsString(name)
	if _as108 != "Qty" {
		t.Errorf("name = %v, want 'Qty'", name)
	}
	kind, _ := m.Get("kind")
	_as109, _ := AsAtom(kind)
	if _as109 != "literal" {
		t.Errorf("kind = %v, want literal", kind)
	}
	// A named type's `type` is the metatype "Type"; its underlying
	// structure leaf goes to `struct`.
	typ, _ := m.Get("type")
	_as110, _ := AsString(typ)
	if _as110 != "Type" {
		t.Errorf("type = %v, want 'Type'", typ)
	}
	strct, _ := m.Get("struct")
	_asStruct, _ := AsString(strct)
	if _asStruct != "Number" {
		t.Errorf("struct = %v, want 'Number'", strct)
	}
}

func TestEngineInspectRecordType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def Pos record{x:number,y:number} ; inspect Pos
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TNumber))
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Pos"), NewRecordType(fields),
		NewWord("inspect"), NewWord("Pos"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	m, _ := AsMap(result[0])

	name, _ := m.Get("name")
	_as111, _ := AsString(name)
	if _as111 != "Pos" {
		t.Errorf("name = %v, want 'Pos'", name)
	}
	kind, _ := m.Get("kind")
	_as112, _ := AsAtom(kind)
	if _as112 != "record" {
		t.Errorf("kind = %v, want record", kind)
	}
	flds, ok := m.Get("fields")
	if !ok {
		t.Fatal("missing fields")
	}
	fm, _ := AsMap(flds)
	xType, _ := fm.Get("x")
	_as113, _ := AsString(xType)
	if _as113 != "Number" {
		t.Errorf("fields.x = %v, want 'Number'", xType)
	}
	yType, _ := fm.Get("y")
	_as114, _ := AsString(yType)
	if _as114 != "Number" {
		t.Errorf("fields.y = %v, want 'Number'", yType)
	}
}

func TestEngineInspectTypeDotAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def Qty number ; inspect Qty .kind
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Qty"), NewTypeLiteral(TNumber),
		NewWord("inspect"), NewWord("Qty"),
		NewWord("dot"), NewWord("kind"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	_as115, _ := AsAtom(result[0])
	if _as115 != "literal" {
		t.Errorf("inspect Qty .kind = %v, want literal", result[0])
	}
}

func TestFormatFromExt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	tests := []struct {
		path string
		want string
	}{
		{"file.csv", "csv"},
		{"file.tsv", "tsv"},
		{"file.json", "json"},
		{"file.jsonic", "jsonic"},
		{"file.txt", "text"},
		{"file.unknown", ""},
		{"file", ""},
		{"path/to/data.CSV", "csv"},
		// Tabnas family + many-to-one extension aliases.
		{"file.ini", "ini"},
		{"file.cfg", "ini"},
		{"file.conf", "ini"},
		{"file.yaml", "yaml"},
		{"file.yml", "yaml"},
		{"file.toml", "toml"},
		{"file.xml", "xml"},
		{"file.json5", "json5"},
		{"file.jsonc", "jsonc"},
		{"file.zon", "zon"},
		{"file.md", "markdown"},
		{"file.markdown", "markdown"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := formatFromExt(r, tt.path)
			if got != tt.want {
				t.Errorf("formatFromExt(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// --- Return type validation tests ---

func TestEngineFnReturnTypeCorrect(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def double fn [[number] [number] [dup add]] end
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("double"),
	})
	_as116, _ := AsInteger(result[0])
	if len(result) != 1 || _as116 != 10 {
		t.Errorf("5 double = %v, want 10", result)
	}
}

func TestEngineFnReturnTypeWrong(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def bad fn [[number] [string] [dup add]] end
	// Returns a number but declares string return type.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	err = runBoruError(t, r, []Value{
		NewWord("def"), NewWord("bad"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("bad"),
	})
	if err == nil {
		t.Fatal("expected return type error, got nil")
	}
	if !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "expected") {
		t.Errorf("error should mention function name and expected type, got: %v", err)
	}
}

func TestEngineFnReturnCountWrong(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def toomany fn [[number] [number] [dup]] end
	// Body produces 2 values (dup), signature declares 1 return.
	// The extra value is the unconsumed unnamed arg which is discarded,
	// leaving only the declared return value.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("toomany"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("toomany"),
	})
	_as117, _ := AsInteger(result[0])
	if len(result) != 1 || _as117 != 5 {
		t.Errorf("expected [5], got %v", result)
	}

	// Genuinely wrong: body produces more values than unnamed args + declared returns.
	// def bad fn [[number] [number] [dup dup]] end — 3 results, 1 unnamed + 1 return = 2 max.
	fnBody2 := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("dup")}),
	})
	err = runBoruError(t, r, []Value{
		NewWord("def"), NewWord("bad"), NewWord("fn"), fnBody2, NewEnd(),
		NewInteger(5), NewWord("bad"),
	})
	if err == nil {
		t.Fatal("expected return count error, got nil")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention function name, got: %v", err)
	}
}

func TestEngineFnReturnTypeAny(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def identity fn [[any] [any] []] end
	// [any] return type should accept any value.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Any")}),
		NewList([]Value{NewWord("Any")}),
		NewList([]Value{}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("identity"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(42), NewWord("identity"),
	})
	_as118, _ := AsInteger(result[0])
	if len(result) != 1 || _as118 != 42 {
		t.Errorf("42 identity = %v, want 42", result)
	}
}

func TestEngineFnReturnTypeUncheckedEmpty(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def dbl fn [[number] [] [dup add]] end
	// Empty return sig means no checking (backwards compat).
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("dbl"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("dbl"),
	})
	_as119, _ := AsInteger(result[0])
	if len(result) != 1 || _as119 != 14 {
		t.Errorf("7 dbl = %v, want 14", result)
	}
}

func TestEngineFnReturnTypeMultipleValues(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def dup2 fn [[number] [number number] [dup]] end
	// Returns 2 numbers.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number"), NewWord("Number")}),
		NewList([]Value{NewWord("dup")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("fdup2"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(3), NewWord("fdup2"),
	})
	_as121, _ := AsInteger(result[0])
	_as120, _ := AsInteger(result[1])
	if len(result) != 2 || _as121 != 3 || _as120 != 3 {
		t.Errorf("3 fdup2 = %v, want [3 3]", result)
	}
}

func TestEngineFnReturnTypeNamedParams(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def square fn [[x:number] [number] [x mul x]] end
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(6), NewWord("square"),
	})
	_as122, _ := AsInteger(result[0])
	if len(result) != 1 || _as122 != 36 {
		t.Errorf("6 square = %v, want 36", result)
	}
}

func TestEngineFnReturnTypeNamedParamsWrongReturn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def isbig fn [[x:number] [number] [x gt 10]] end
	// Declares number return but body returns boolean via gt.
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("gt"), NewInteger(10)}),
	})
	err = runBoruError(t, r, []Value{
		NewWord("def"), NewWord("isbig"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("isbig"),
	})
	if err == nil {
		t.Fatal("expected return type error for named param fn, got nil")
	}
	if !strings.Contains(err.Error(), "isbig") {
		t.Errorf("error should mention function name, got: %v", err)
	}
}

func TestEngineFnReturnTypeMultiOverload(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def add1 fn [[number] [number] [1 add] [string] [string] ["1" add]] end
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewInteger(1), NewWord("add")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewString("1"), NewWord("add")}),
	})
	// Test number overload
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("add1"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(10), NewWord("add1"),
	})
	_as123, _ := AsInteger(result[0])
	if len(result) != 1 || _as123 != 11 {
		t.Errorf("10 add1 = %v, want 11", result)
	}
	// Test string overload
	result = runBoru(t, r, []Value{
		NewString("hello"), NewWord("add1"),
	})
	_as124, _ := AsString(result[0])
	if len(result) != 1 || _as124 != "hello1" {
		t.Errorf("'hello' add1 = %v, want 'hello1'", result)
	}
}

func TestPiecemealDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Define foo with number sig, then add string sig
	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Define both sigs
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
	})

	// Test number sig
	result := runBoru(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as125, _ := AsInteger(result[0])
	if len(result) != 1 || _as125 != 9 {
		t.Errorf("3 foo = %v, want 9", result)
	}

	// Test string sig
	result = runBoru(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	_as126, _ := AsString(result[0])
	if len(result) != 1 || _as126 != "hihi" {
		t.Errorf("\"hi\" foo = %v, want \"hihi\"", result)
	}
}

func TestPiecemealUndefPopsRecent(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// def number sig, def string sig, undef (pops string sig), test number sig
	result := runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("undef"), NewWord("foo"), NewEnd(),
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as127, _ := AsInteger(result[0])
	if _as127 != 9 {
		t.Errorf("3 foo after undef = %v, want 9", result[0])
	}
}

func TestFnUndefTargeted(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Targeted removal: def foo fn [[number] [number]] (pairs = remove sig)
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})

	// def both sigs, targeted remove number sig, string sig still works
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runBoru(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as128, _ := AsString(result[0])
	if _as128 != "hihi" {
		t.Errorf("\"hi\" foo after targeted undef = %v, want \"hihi\"", result[0])
	}
}

func TestFnUndefTargetedReverse(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	strBody := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Remove string sig, keep number sig
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
	})

	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), strBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runBoru(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as129, _ := AsInteger(result[0])
	if _as129 != 9 {
		t.Errorf("3 foo after targeted undef string = %v, want 9", result[0])
	}
}

func TestFnUndefNonExistentNoOp(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})

	// Remove a string sig that was never defined — should be a no-op
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
	})

	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
	})
	result := runBoru(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	_as130, _ := AsInteger(result[0])
	if _as130 != 9 {
		t.Errorf("3 foo after no-op undef = %v, want 9", result[0])
	}
}

func TestFnUndefRemovesAll(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	numBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})

	// Remove the only sig — word should become undefined
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})

	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), numBody, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fnsig"), undefSpec, NewEnd(),
		NewWord("foo"),
	})
	// foo should error (undefined after all sigs removed)
	if err == nil {
		t.Fatal("expected error for undefined word after removing all sigs, got nil")
	}
}

func TestPiecemealStackUnwind(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// def A (number -> dup mul), def B (string -> dup add), undef B, A still works
	bodyA := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	bodyB := NewList([]Value{
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})

	// Define both
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyA, NewEnd(),
		NewWord("def"), NewString("foo"), NewWord("fn"), bodyB, NewEnd(),
	})

	// Both sigs work
	result := runBoru(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as132, _ := AsInteger(result[0])
	if len(result) != 1 || _as132 != 9 {
		t.Fatalf("3 foo = %v, want 9", result)
	}
	result = runBoru(t, r, []Value{
		NewString("hi"), NewWord("foo"),
	})
	_as133, _ := AsString(result[0])
	if len(result) != 1 || _as133 != "hihi" {
		t.Fatalf("\"hi\" foo = %v, want \"hihi\"", result)
	}

	// Undef pops B (string sig), A (number sig) remains
	_ = runBoru(t, r, []Value{
		NewWord("undef"), NewWord("foo"), NewEnd(),
	})
	result = runBoru(t, r, []Value{
		NewInteger(3), NewWord("foo"),
	})
	_as134, _ := AsInteger(result[0])
	if len(result) != 1 || _as134 != 9 {
		t.Fatalf("3 foo after undef B = %v, want 9", result)
	}
}

// --- Metatype integration tests ---

func TestTypeofMetatypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Post the typeof-collapse + Any-root work, typeof is a single
	// Parent hop. None and Never are degenerate roots that saturate
	// at themselves.
	tests := []struct {
		name     string
		typeLit  Value
		wantType string // expected typeof result (single Parent hop)
	}{
		{"String", NewTypeLiteral(TString), "Scalar"},
		{"Number", NewTypeLiteral(TNumber), "Scalar"},
		{"Integer", NewTypeLiteral(TInteger), "Number"},
		{"Float", NewTypeLiteral(TFloat), "Number"},
		{"Boolean", NewTypeLiteral(TBoolean), "Scalar"},
		{"List", NewTypeLiteral(TList), "Node"},
		{"Map", NewTypeLiteral(TMap), "Node"},
		{"Scalar", NewTypeLiteral(TScalar), "Any"},
		{"Node", NewTypeLiteral(TNode), "Any"},
		{"Any", NewTypeLiteral(TAny), "Any"},
		{"None", NewTypeLiteral(TNone), "None"},
		{"Class", NewTypeLiteral(TClass), "Ideal"},
		{"Table", NewTypeLiteral(TTable), "Ideal"},
		{"Record", NewTypeLiteral(TRecord), "Ideal"},
		{"Resource", NewTypeLiteral(TResource), "Ideal"},
		{"Atom", NewTypeLiteral(TAtom), "Scalar"},
		{"Type", NewTypeLiteral(TType), "Any"},
		{"Function", NewTypeLiteral(TFunction), "Type"},
		{"Disjunct", NewTypeLiteral(TDisjunct), "Type"},
	}

	for _, tt := range tests {
		t.Run("typeof-"+tt.name, func(t *testing.T) {
			result := runBoru(t, r, []Value{tt.typeLit, NewWord("typeof")})
			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}
			// typeof returns a Type literal — compare via String()
			// which renders the leaf.
			got := result[0].String()
			if got != tt.wantType {
				t.Errorf("typeof %s = %q, want %q", tt.name, got, tt.wantType)
			}
		})
	}

	// Concrete values: typeof returns a Type literal of the value's
	// exact Parent. Render via String() (leaf).
	t.Run("typeof-concrete-integer", func(t *testing.T) {
		result := runBoru(t, r, []Value{NewInteger(42), NewWord("typeof")})
		if len(result) != 1 || result[0].String() != "Integer" {
			t.Errorf("typeof 42 = %v, want Integer", result)
		}
	})
	t.Run("typeof-concrete-boolean", func(t *testing.T) {
		result := runBoru(t, r, []Value{NewBoolean(true), NewWord("typeof")})
		if len(result) != 1 || result[0].String() != "Boolean" {
			t.Errorf("typeof true = %v, want Boolean", result)
		}
	})
}

// TestIs_BroadTypeRoot covers `v is Type` — Type/ is still the root
// for the *Type meta-hierarchy (Function, Disjunct, Enum, etc.) even
// after the ScalarType / NodeType / IdealType nodes were retired.
func TestIs_BroadTypeRoot(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	tests := []struct {
		name string
		val  Value
		pat  Value
		want bool
	}{
		{"Boolean is Type", NewTypeLiteral(TBoolean), NewTypeLiteral(TType), true},
		{"List is Type", NewTypeLiteral(TList), NewTypeLiteral(TType), true},
		{"Class is Type", NewTypeLiteral(TClass), NewTypeLiteral(TType), true},
		{"Any is Type", NewTypeLiteral(TAny), NewTypeLiteral(TType), true},
		{"Scalar is Type", NewTypeLiteral(TScalar), NewTypeLiteral(TType), true},
		{"Node is Type", NewTypeLiteral(TNode), NewTypeLiteral(TType), true},
		// A concrete value is NOT a Type.
		{"5 is Type", NewInteger(5), NewTypeLiteral(TType), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runBoru(t, r, []Value{tt.val, NewWord("is"), tt.pat})
			if len(result) != 1 {
				t.Fatalf("expected 1 result, got %d", len(result))
			}
			got, _ := AsBoolean(result[0])
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- String interpolation integration tests ---

func TestInterpStringLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	parts := []InterpPart{
		{Lit: "hello world"},
	}
	result := runBoru(t, r, []Value{NewInterpString(parts)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestInterpStringWithExpression(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewString("world"), NewWord("def"), NewWord("name"), NewEnd(),
		NewInterpString([]InterpPart{
			{Lit: "hello "},
			{Expr: []Value{NewWord("name")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestInterpStringArithmetic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInterpString([]InterpPart{
			{Lit: "answer: "},
			{Expr: []Value{NewInteger(1), NewWord("add"), NewInteger(2)}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "answer: 3" {
		t.Errorf("expected 'answer: 3', got %q", got)
	}
}

func TestInterpStringMultipleExprs(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("def"), NewWord("a"), NewEnd(),
		NewInteger(2), NewWord("def"), NewWord("b"), NewEnd(),
		NewInterpString([]InterpPart{
			{Expr: []Value{NewWord("a")}},
			{Lit: " and "},
			{Expr: []Value{NewWord("b")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "1 and 2" {
		t.Errorf("expected '1 and 2', got %q", got)
	}
}

// TestInterpStringNone pins the regression where a None value interpolated
// into a template literal collapsed the whole string to a "String" carrier
// (voxgig-boru/bloom-filter backend report §1). None is non-concrete yet is a
// legitimate runtime value: it must stringify to "None", not flag the part as
// a dynamic (check-mode-carrier) frontier. `def x None  \`got ${x}\“.
func TestInterpStringNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewTypeLiteral(TNone), NewWord("def"), NewWord("x"), NewEnd(),
		NewInterpString([]InterpPart{
			{Lit: "got "},
			{Expr: []Value{NewWord("x")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Positive: the real string is produced.
	got, _ := AsString(result[0])
	if got != "got None" {
		t.Errorf("expected 'got None', got %q", got)
	}
	// Negative: the result must be a concrete String, NOT a non-concrete
	// carrier — the bug returned NewCarrier(TString), which rendered "String".
	if !IsConcrete(&result[0]) {
		t.Errorf("expected a concrete String value, got non-concrete %v", result[0])
	}
}

// TestInterpStringTypeLiteral pins the sibling case: a bare type literal (the
// result of `typeof`) is also non-concrete but a legitimate runtime value, so
// it must render its type name rather than collapse to a "String" carrier.
func TestInterpStringTypeLiteral(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInterpString([]InterpPart{
			{Lit: "type: "},
			{Expr: []Value{NewInteger(42), NewWord("typeof")}},
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "type: Integer" {
		t.Errorf("expected 'type: Integer', got %q", got)
	}
	if !IsConcrete(&result[0]) {
		t.Errorf("expected a concrete String value, got non-concrete %v", result[0])
	}
}

func TestInterpStringInMapValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(42), NewWord("def"), NewWord("x"), NewEnd(),
		NewEvalMap(func() *OrderedMap {
			om := NewOrderedMap()
			om.Set("msg", NewInterpString([]InterpPart{
				{Lit: "value is "},
				{Expr: []Value{NewWord("x")}},
			}))
			return om
		}()),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	if m == nil {
		t.Fatal("expected map result")
	}
	v, ok := m.Get("msg")
	if !ok {
		t.Fatal("expected 'msg' key in map")
	}
	got, _ := AsString(v)
	if got != "value is 42" {
		t.Errorf("expected 'value is 42', got %q", got)
	}
}

func TestInterpStringAsWordArg(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInterpString([]InterpPart{
			{Lit: "hello"},
		}),
		NewWord("upper"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	got, _ := AsString(result[0])
	if got != "HELLO" {
		t.Errorf("expected 'HELLO', got %q", got)
	}
}

// TestTpartialRejectsNonType verifies tpartial errors on non-Record /
// non-Object inputs.
func TestTpartialRejectsNonType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	cases := []struct {
		name string
		arg  Value
	}{
		{"Integer-literal", NewTypeLiteral(TInteger)},
		{"concrete-int", NewInteger(42)},
		{"concrete-string", NewString("x")},
		{"List-literal", NewTypeLiteral(TList)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runBoruError(t, r, []Value{tc.arg, NewWord("tpartial")})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "Record or Object") {
				t.Errorf("error should mention Record or Object, got: %v", err)
			}
		})
	}
}
