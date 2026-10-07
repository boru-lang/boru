package native

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// === 1. var word ===

func TestIntegVarWordDeclareAssign(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// var x 10 var x 11 x => 11 — a declaration, an in-place assignment, a read
	result := runBoru(t, r, []Value{
		NewWord("var"), NewWord("x"), NewInteger(10),
		NewWord("var"), NewWord("x"), NewInteger(11),
		NewWord("x"),
	})
	v, _ := AsInteger(result[0])
	if len(result) != 1 || v != 11 {
		t.Errorf("var x 10 var x 11 x = %v, want 11", result)
	}
	// var f fn […] — a var holds a value, never a function constructor
	// (the keyword form is registered only to refuse, var_error).
	err := runBoruError(t, r, []Value{
		NewWord("var"), NewWord("f"), NewWord("fn"),
		NewList([]Value{NewList([]Value{}), NewList([]Value{}), NewList([]Value{NewInteger(1)})}),
	})
	if err == nil || !strings.Contains(err.Error(), "var_error") {
		t.Errorf("var f fn […]: err = %v, want var_error", err)
	}
}

// === 2. undef word ===

func TestIntegUndefRemovesDef(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def my-val 99 end my-val undef my-val end
	// After undef, my-val should not be found (error or just word)
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-val"), NewInteger(99), NewEnd(),
		NewWord("my-val"),
	})
	_as4, _ := AsInteger(result[0])
	if len(result) != 1 || _as4 != 99 {
		t.Fatalf("def my-val 99 end my-val = %v, want 99", result)
	}

	// Now undef it and verify it's gone
	result = runBoru(t, r, []Value{
		NewWord("undef"), NewWord("my-val"),
	})
	// Should return nothing (undef returns nil)
	if len(result) != 0 {
		t.Errorf("undef my-val should return nothing, got %v", result)
	}
}

func TestIntegUndefWithString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def my-val 42 end undef "my-val"
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-val"), NewInteger(42), NewEnd(),
		NewWord("undef"), NewString("my-val"),
	})
	if len(result) != 0 {
		t.Errorf("undef by string should return nothing, got %v", result)
	}
}

func TestIntegUndefFnTargeted(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def my-fn fn [[x:Number] [Number] [x add 1]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-fn"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})

	// Verify my-fn works: 5 my-fn => 6
	result := runBoru(t, r, []Value{NewInteger(5), NewWord("my-fn")})
	_as5, _ := AsNumber(result[0])
	if len(result) != 1 || _as5 != 6 {
		t.Fatalf("5 my-fn = %v, want 6", result)
	}

	// undef my-fn (complete removal)
	runBoru(t, r, []Value{NewWord("undef"), NewWord("my-fn")})
}

// === 3. fn word ===

func TestIntegFnMultipleParams(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def add-two fn [[x:Number y:Number] [Number] [x add y]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	pairY := NewOrderedMap()
	pairY.Set("y", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX), NewImplicitMap(pairY)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("y")}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("add-two"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})

	// 3 5 add-two => 8
	result := runBoru(t, r, []Value{NewInteger(3), NewInteger(5), NewWord("add-two")})
	_as6, _ := AsNumber(result[0])
	if len(result) != 1 || _as6 != 8 {
		t.Errorf("3 5 add-two = %v, want 8", result)
	}
}

func TestIntegFnUnnamedParams(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// fn [[Number] [Number] [add 1]]  — unnamed Number param
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("add"), NewInteger(1)}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("inc"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})
	if len(result) != 0 {
		t.Fatalf("def should return nothing, got %v", result)
	}

	result = runBoru(t, r, []Value{NewInteger(10), NewWord("inc")})
	_as7, _ := AsNumber(result[0])
	if len(result) != 1 || _as7 != 11 {
		t.Errorf("10 inc = %v, want 11", result)
	}
}

func TestIntegFnUndefSpecPairs(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// fnsig with 2 elements (one input/output pair) => FnUndefInfo
	// fnsig [[Number] [Number]]
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})
	result := runBoru(t, r, []Value{NewWord("fnsig"), fnBody})
	if len(result) != 1 {
		t.Fatalf("fnsig should return 1 value, got %d", len(result))
	}
	if !result[0].Parent.Equal(TFnUndef) {
		t.Errorf("expected TFnUndef, got %s", result[0].Parent)
	}
}

func TestIntegFnEmptyListError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{NewWord("fn"), NewList([]Value{})})
	if err == nil {
		t.Error("expected error for fn with empty list")
	}
}

func TestIntegFnBadLengthError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// fn with 4 elements (not a multiple of 3) — fn now requires
	// strict triples; the 2-pair form moved to `fnsig`.
	err := runBoruError(t, r, []Value{
		NewWord("fn"), NewList([]Value{
			NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4),
		}),
	})
	if err == nil {
		t.Error("expected error for fn with 4 elements")
	}
}

func TestIntegFnSingleValueAbbreviation(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// fn with non-list input/output/body (abbreviation: treated as single-element lists)
	// fn [Number Number [add 1]]
	fnBody := NewList([]Value{
		NewWord("Number"),
		NewWord("Number"),
		NewList([]Value{NewWord("add"), NewInteger(1)}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("inc2"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})

	result := runBoru(t, r, []Value{NewInteger(7), NewWord("inc2")})
	_as8, _ := AsNumber(result[0])
	if len(result) != 1 || _as8 != 8 {
		t.Errorf("7 inc2 = %v, want 8", result)
	}
}

// === 4. module word ===

func TestIntegModuleWithExport(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// module [
	//   def x 42 end
	//   export :myExport {val: x}
	// ] end
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(42), NewEnd(),
		NewWord("export"), NewAtom("myExport"),
		NewMap(singleMap("val", NewString("x"))),
	})
	result := runBoru(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("module should return 1 value, got %d", len(result))
	}
	if !result[0].Parent.Equal(TModuleInst) {
		t.Fatalf("expected a Module, got %s", result[0].Parent)
	}

	desc, _ := asModuleDesc(result[0])
	if _, ok := desc.Exports["myExport"]; !ok {
		t.Error("expected 'myExport' in module exports")
	}
}

func TestIntegModuleImportAll(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Build module, then import it
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(99), NewEnd(),
		NewWord("export"), NewAtom("stuff"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import mymod
	runBoru(t, r, []Value{NewWord("import"), NewWord("mymod")})

	// Now "stuff" should be defined as a map with val: 99
	result := runBoru(t, r, []Value{NewWord("stuff")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("stuff should be a module namespace, got %v", result)
	}
	v, ok := moduleExportGet(result[0], "val")
	_as9, _ := AsInteger(v)
	if !ok || _as9 != 99 {
		t.Errorf("stuff.val = %v, want 99", v)
	}
}

func TestIntegModuleImportRename(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(55), NewEnd(),
		NewWord("export"), NewAtom("orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mymod2"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import [orig renamed] mymod2
	renameList := NewList([]Value{NewAtom("orig"), NewAtom("renamed")})
	runBoru(t, r, []Value{NewWord("import"), renameList, NewWord("mymod2")})

	result := runBoru(t, r, []Value{NewWord("renamed")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("renamed should be a module namespace, got %v", result)
	}
}

func TestIntegModuleImportMultiRename(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("a"), NewInteger(1), NewEnd(),
		NewWord("def"), NewString("b"), NewInteger(2), NewEnd(),
		NewWord("export"), NewAtom("expA"),
		NewMap(singleMap("val", NewString("a"))),
		NewWord("export"), NewAtom("expB"),
		NewMap(singleMap("val", NewString("b"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mm"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import [[expA newA] [expB newB]] mm
	renameList := NewList([]Value{
		NewList([]Value{NewAtom("expA"), NewAtom("newA")}),
		NewList([]Value{NewAtom("expB"), NewAtom("newB")}),
	})
	runBoru(t, r, []Value{NewWord("import"), renameList, NewWord("mm")})

	result := runBoru(t, r, []Value{NewWord("newA")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("newA should be a module namespace, got %v", result)
	}
}

func TestIntegModuleExportWithAtomName(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// export with atom name (word signature removed; unknown words become atoms)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(7), NewEnd(),
		NewWord("export"), NewAtom("wrdexp"),
		NewMap(singleMap("val", NewString("x"))),
	})
	result := runBoru(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	desc, _ := asModuleDesc(result[0])
	if _, ok := desc.Exports["wrdexp"]; !ok {
		t.Error("expected 'wrdexp' in module exports")
	}
}

func TestIntegValToAtomOrStringWord(t *testing.T) {
	// Test valToAtomOrString with word values (used in import rename)
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(10), NewEnd(),
		NewWord("export"), NewAtom("orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("wmod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import with word names (instead of atoms/strings)
	renameList := NewList([]Value{NewWord("orig"), NewWord("wordRenamed")})
	runBoru(t, r, []Value{NewWord("import"), renameList, NewWord("wmod")})

	result := runBoru(t, r, []Value{NewWord("wordRenamed")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("wordRenamed should be a module namespace, got %v", result)
	}
}

func TestIntegImportSingleRenameWord(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(42), NewEnd(),
		NewWord("export"), NewAtom("Orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import NewName mymod — renames single export Orig to NewName
	// (unknown word NewName becomes atom at runtime)
	runBoru(t, r, []Value{NewWord("import"), NewAtom("NewName"), NewWord("mymod")})

	result := runBoru(t, r, []Value{NewWord("NewName")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("NewName should be a module namespace, got %v", result)
	}
}

func TestIntegImportSingleRenameAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(42), NewEnd(),
		NewWord("export"), NewAtom("Orig"),
		NewMap(singleMap("val", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mymod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// import Renamed mymod — renames single export Orig to Renamed (atom)
	runBoru(t, r, []Value{NewWord("import"), NewAtom("Renamed"), NewWord("mymod")})

	result := runBoru(t, r, []Value{NewWord("Renamed")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("Renamed should be a module namespace, got %v", result)
	}
}

func TestIntegImportSingleRenameMultiExportError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	moduleBody := NewList([]Value{
		NewWord("export"), NewAtom("X"), NewMap(singleMap("v", NewInteger(1))),
		NewWord("export"), NewAtom("Y"), NewMap(singleMap("v", NewInteger(2))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mm"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	_, err := New(r).Run([]Value{NewWord("import"), NewWord("Only"), NewWord("mm")})
	if err == nil {
		t.Fatal("expected error when renaming module with multiple exports")
	}
}

// === 5. convert word ===

func TestIntegConvertIntegerToString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 42 convert String
	result := runBoru(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewTypeLiteral(TString),
	})
	_as10, _ := AsString(result[0])
	if len(result) != 1 || _as10 != "42" {
		t.Errorf("42 convert String = %v, want '42'", result)
	}
}

func TestIntegConvertStringToInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewString("123"), NewWord("convert"), NewTypeLiteral(TInteger),
	})
	_as11, _ := AsInteger(result[0])
	if len(result) != 1 || _as11 != 123 {
		t.Errorf("'123' convert Integer = %v, want 123", result)
	}
}

func TestIntegConvertStringToFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewString("3.14"), NewWord("convert"), NewTypeLiteral(TFloat),
	})
	_as12, _ := AsFloat(result[0])
	if len(result) != 1 || _as12 != 3.14 {
		t.Errorf("'3.14' convert Float = %v, want 3.14", result)
	}
}

func TestIntegConvertToBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// integer to boolean
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as13, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as13 {
		t.Errorf("1 convert Boolean = %v, want true", result)
	}

	// 0 to boolean
	result = runBoru(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as14, _ := AsBoolean(result[0])
	if len(result) != 1 || _as14 {
		t.Errorf("0 convert Boolean = %v, want false", result)
	}

	// string "true" to boolean
	result = runBoru(t, r, []Value{
		NewString("true"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as15, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as15 {
		t.Errorf("'true' convert Boolean = %v, want true", result)
	}

	// string "false" to boolean: coercion by presence, not content —
	// a non-empty String is true, so the text "false" is true.
	result = runBoru(t, r, []Value{
		NewString("false"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as16, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as16 {
		t.Errorf("'false' convert Boolean = %v, want true", result)
	}

	// non-empty string to boolean (truthy)
	result = runBoru(t, r, []Value{
		NewString("hello"), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as17, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as17 {
		t.Errorf("'hello' convert Boolean = %v, want true", result)
	}

	// empty string to boolean (falsy)
	result = runBoru(t, r, []Value{
		NewString(""), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as18, _ := AsBoolean(result[0])
	if len(result) != 1 || _as18 {
		t.Errorf("'' convert Boolean = %v, want false", result)
	}
}

func TestIntegConvertIntToHexString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 255 convert String {base: "hex"}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	_as19, _ := AsString(result[0])
	if len(result) != 1 || _as19 != "ff" {
		t.Errorf("255 convert String {base:hex} = %v, want 'ff'", result)
	}
}

func TestIntegConvertIntToHEXString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("HEX"))
	result := runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	_as20, _ := AsString(result[0])
	if len(result) != 1 || _as20 != "FF" {
		t.Errorf("255 convert String {base:HEX} = %v, want 'FF'", result)
	}
}

func TestIntegConvertIntToBinString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runBoru(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewTypeLiteral(TString), NewMap(binOpts),
	})
	_as21, _ := AsString(result[0])
	if len(result) != 1 || _as21 != "1010" {
		t.Errorf("10 convert String {base:bin} = %v, want '1010'", result)
	}
}

func TestIntegConvertIntToOctString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runBoru(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewTypeLiteral(TString), NewMap(octOpts),
	})
	_as22, _ := AsString(result[0])
	if len(result) != 1 || _as22 != "10" {
		t.Errorf("8 convert String {base:oct} = %v, want '10'", result)
	}
}

func TestIntegConvertHexStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// "ff" convert Number {base: "hex"}
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runBoru(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(hexOpts),
	})
	_as23, _ := AsInteger(result[0])
	if len(result) != 1 || _as23 != 255 {
		t.Errorf("'ff' convert Number {base:hex} = %v, want 255", result)
	}
}

func TestIntegConvertBinStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runBoru(t, r, []Value{
		NewString("1010"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(binOpts),
	})
	_as24, _ := AsInteger(result[0])
	if len(result) != 1 || _as24 != 10 {
		t.Errorf("'1010' convert Number {base:bin} = %v, want 10", result)
	}
}

func TestIntegConvertOctStringToNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runBoru(t, r, []Value{
		NewString("10"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(octOpts),
	})
	_as25, _ := AsInteger(result[0])
	if len(result) != 1 || _as25 != 8 {
		t.Errorf("'10' convert Number {base:oct} = %v, want 8", result)
	}
}

func TestIntegConvertWithSettingsMap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 255 convert String {base: "hex"}
	opts := NewOrderedMap()
	opts.Set("base", NewString("hex"))
	result := runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewTypeLiteral(TString), NewMap(opts),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	_as26, _ := AsString(result[0])
	if _as26 != "ff" {
		t.Errorf("255 convert String {base:hex} = %v, want 'ff'", result)
	}
}

func TestIntegConvertBooleanPassthrough(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewTypeLiteral(TBoolean),
	})
	_as27, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as27 {
		t.Errorf("true convert Boolean = %v, want true", result)
	}
}

func TestIntegConvertErrorBadFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewString("notanumber"), NewWord("convert"), NewTypeLiteral(TFloat),
	})
	if err == nil {
		t.Error("expected error converting non-numeric string to decimal")
	}
}

func TestIntegConvertErrorBadNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewString("notanumber"), NewWord("convert"), NewTypeLiteral(TNumber),
	})
	if err == nil {
		t.Error("expected error converting non-numeric string to number")
	}
}

func TestIntegConvertErrorBadVariant(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	badOpts := NewOrderedMap()
	badOpts.Set("base", NewString("badvariant"))
	err := runBoruError(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewTypeLiteral(TString), NewMap(badOpts),
	})
	if err == nil {
		t.Error("expected error for unknown string variant")
	}
}

func TestIntegConvertErrorBadNumberVariant(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	badOpts := NewOrderedMap()
	badOpts.Set("base", NewString("badvariant"))
	err := runBoruError(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewTypeLiteral(TNumber), NewMap(badOpts),
	})
	if err == nil {
		t.Error("expected error for unknown number variant")
	}
}

func TestIntegConvertErrorVariantNotInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Variant conversion only supported for integer to string
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	err := runBoruError(t, r, []Value{
		NewString("hello"), NewWord("convert"), NewTypeLiteral(TString), NewMap(hexOpts),
	})
	if err == nil {
		t.Error("expected error for variant with non-integer source")
	}
}

// === 6. do word ===

func TestIntegDoList(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("do"), NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
	})
	_as28, _ := AsNumber(result[0])
	if len(result) != 1 || _as28 != 3 {
		t.Errorf("do [1 add 2] = %v, want 3", result)
	}
}

func TestIntegDoMap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// do {x: [3 add 4]} — `add` is an unquoted Word, exactly as the
	// parser produces it from source; a `do`-map list runs words as code.
	// (A quoted "add" String is data and is left untouched — DX report T4.)
	innerList := NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)})
	m := NewOrderedMap()
	m.Set("x", innerList)
	result := runBoru(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("do map should return a map, got %v", result)
	}
	rm, _ := AsMap(result[0])
	xVal, ok := rm.Get("x")
	_as29, _ := AsNumber(xVal)
	if !ok || _as29 != 7 {
		t.Errorf("do {x:[3 add 4]}.x = %v, want 7", xVal)
	}
}

func TestIntegDoNestedMap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// do {outer: {inner: [2 add 3]}} — `add` is an unquoted Word (as the
	// parser yields), which a `do`-map list evaluates as code.
	innerList := NewList([]Value{NewInteger(2), NewWord("add"), NewInteger(3)})
	innerMap := NewOrderedMap()
	innerMap.Set("inner", innerList)
	outerMap := NewOrderedMap()
	outerMap.Set("outer", NewMap(innerMap))
	result := runBoru(t, r, []Value{NewWord("do"), NewMap(outerMap)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	outer, _ := AsMap(result[0])
	innerVal, _ := outer.Get("outer")
	innerResult, _ := AsMap(innerVal)
	v, _ := innerResult.Get("inner")
	_as30, _ := AsNumber(v)
	if _as30 != 5 {
		t.Errorf("do nested map inner = %v, want 5", v)
	}
}

func TestIntegDoListMultipleResults(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// do [1 2 3] => returns list of all results
	result := runBoru(t, r, []Value{
		NewWord("do"), NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
	})
	// The sub-engine should return all 3 values
	if len(result) != 3 {
		t.Errorf("do [1 2 3] = %v, want 3 results", result)
	}
}

// === 7. fileio with MemFileOps ===

func TestIntegFileIOWriteAndRead(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// write "test.txt" "hello world"
	result := runBoru(t, r, []Value{
		NewWord("write"), pathV("test.txt"), NewString("hello world"),
	})
	if len(result) != 1 || !IsPathon(result[0]) || result[0].String() != "test.txt" {
		t.Errorf("write should return path, got %v", result)
	}

	// read "test.txt"
	result = runBoru(t, r, []Value{
		NewWord("read"), pathV("test.txt"),
	})
	_as32, _ := AsString(result[0])
	if len(result) != 1 || _as32 != "hello world" {
		t.Errorf("read test.txt = %v, want 'hello world'", result)
	}
}

func TestIntegFileIOWriteAppend(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// write "test.txt" "hello"
	runBoru(t, r, []Value{
		NewWord("write"), pathV("test.txt"), NewString("hello"),
	})

	// write "test.txt" " world" {mode:"append"}
	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runBoru(t, r, []Value{
		NewWord("write"), pathV("test.txt"), NewString(" world"), NewMap(opts),
	})

	// read back
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("test.txt"),
	})
	_as33, _ := AsString(result[0])
	if len(result) != 1 || _as33 != "hello world" {
		t.Errorf("read after append = %v, want 'hello world'", result)
	}
}

func TestIntegFileIOWriteJSON(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// Write a map as JSON
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	runBoru(t, r, []Value{
		NewWord("write"), pathV("data.json"), NewMap(m), NewMap(opts),
	})

	// Read it back
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.json"),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("read data.json should return map, got %v", result)
	}
	rm, _ := AsMap(result[0])
	v, ok := rm.Get("x")
	_as34, _ := AsInteger(v)
	if !ok || _as34 != 1 {
		t.Errorf("read data.json x = %v, want 1", v)
	}
}

func TestIntegFileIOWriteStdout(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf

	// write to stdout
	result := runBoru(t, r, []Value{
		NewWord("write"), NewAtom("stdout"), NewString("printed text"),
	})
	a35, _ := result[0].AsConcreteAtom()
	if len(result) != 1 || a35 != "stdout" {
		t.Errorf("write stdout should return the stdout handle, got %v", result)
	}
	if buf.String() != "printed text" {
		t.Errorf("stdout output = %q, want 'printed text'", buf.String())
	}
}

func TestIntegFileIOWriteStderr(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	var buf bytes.Buffer
	r.ErrOutput = &buf

	result := runBoru(t, r, []Value{
		NewWord("write"), NewAtom("stderr"), NewString("error text"),
	})
	a36, _ := result[0].AsConcreteAtom()
	if len(result) != 1 || a36 != "stderr" {
		t.Errorf("write stderr should return the stderr handle, got %v", result)
	}
	if buf.String() != "error text" {
		t.Errorf("stderr output = %q, want 'error text'", buf.String())
	}
}

func TestIntegFileIOReadWithFmtOption(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// Write JSON content to a txt file, read with explicit fmt
	_ = mem.WriteFile("data.txt", []byte(`{"a":1}`), 0644)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("read data.txt with fmt:json should return map, got %v", result)
	}
}

func TestIntegFileIOReadJsonExtension(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	_ = mem.WriteFile("data.json", []byte(`{"b":2}`), 0644)
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.json"),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("read data.json should auto-detect json, got %v", result)
	}
}

func TestIntegFileIOWriteStringWithOpts(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("text"))
	opts.Set("nl", NewString("lf"))
	runBoru(t, r, []Value{
		NewWord("write"), pathV("output.txt"), NewString("line1\nline2"), NewMap(opts),
	})

	data, _ := mem.ReadFile("output.txt")
	if string(data) != "line1\nline2" {
		t.Errorf("write with opts = %q, want 'line1\\nline2'", data)
	}
}

// === 8. CSV/TSV format ===

func TestIntegCSVReadWrite(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	csvContent := "name,age\nAlice,30\nBob,25\n"
	_ = mem.WriteFile("people.csv", []byte(csvContent), 0644)

	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("people.csv"),
	})
	if len(result) != 1 {
		t.Fatalf("read csv should return 1 value, got %d", len(result))
	}
	// Should be a list (table data)
	if !result[0].Parent.Equal(TList) {
		t.Fatalf("csv result should be TList, got %s", result[0].Parent)
	}
}

func TestIntegTSVRead(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	tsvContent := "col1\tcol2\nval1\tval2\n"
	_ = mem.WriteFile("data.tsv", []byte(tsvContent), 0644)

	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.tsv"),
	})
	if len(result) != 1 {
		t.Fatalf("read tsv should return 1 value, got %d", len(result))
	}
}

func TestIntegCSVReadWithFmtOption(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	csvContent := "x,y\n1,2\n"
	_ = mem.WriteFile("data.txt", []byte(csvContent), 0644)

	opts := NewOrderedMap()
	opts.Set("fmt", NewString("csv"))
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TList) {
		t.Fatalf("read with csv fmt should return table, got %v", result)
	}
}

// === 9. tor for disjuncts ===

func TestIntegTorDisjunctValues(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 1 tor "hello" tor true
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("tor"), NewString("hello"), NewWord("tor"), NewBoolean(true),
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Fatalf("1 tor 'hello' tor true should be disjunct, got %v", result)
	}
	_as37, _ := AsDisjunct(result[0])
	alts := _as37.Alternatives
	if len(alts) != 3 {
		t.Errorf("disjunct should have 3 alternatives, got %d", len(alts))
	}
}

func TestIntegTorDisjunctTwoValues(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(42), NewWord("tor"), NewString("hello"),
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Fatalf("42 tor 'hello' should be disjunct, got %v", result)
	}
	_as38, _ := AsDisjunct(result[0])
	alts := _as38.Alternatives
	if len(alts) != 2 {
		t.Errorf("disjunct should have 2 alternatives, got %d", len(alts))
	}
}

func TestIntegTorDisjunctFlattensLeft(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Build a disjunct then tor with another value
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("tor"), NewInteger(2), NewWord("tor"), NewInteger(3),
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Fatalf("chained tor should produce disjunct, got %v", result)
	}
	_as39, _ := AsDisjunct(result[0])
	alts := _as39.Alternatives
	if len(alts) != 3 {
		t.Errorf("should flatten to 3 alternatives, got %d", len(alts))
	}
}

func TestIntegTorDisjunctFlattensRight(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Pre-build a disjunct on the right side
	rightDisjunct := NewDisjunct([]Value{NewInteger(2), NewInteger(3)})
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("tor"), rightDisjunct,
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Fatalf("tor with right disjunct should produce disjunct, got %v", result)
	}
	_as40, _ := AsDisjunct(result[0])
	alts := _as40.Alternatives
	if len(alts) != 3 {
		t.Errorf("should flatten right disjunct to 3 alternatives, got %d", len(alts))
	}
}

func TestIntegOrBooleanStillWorks(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Boolean or should still work as logical or
	result := runBoru(t, r, []Value{
		NewBoolean(false), NewWord("or"), NewBoolean(true),
	})
	_as41, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as41 {
		t.Errorf("false or true = %v, want true", result)
	}
}

func TestIntegOrShortCircuitReturnsValue(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 1 or 0 → 1 (first truthy wins)
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("or"), NewInteger(0),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if v, _ := AsInteger(result[0]); v != 1 {
		t.Errorf("1 or 0 = %v, want 1", result[0])
	}
	// 0 or 5 → 5 (second wins because first is falsy)
	result = runBoru(t, r, []Value{
		NewInteger(0), NewWord("or"), NewInteger(5),
	})
	if v, _ := AsInteger(result[0]); v != 5 {
		t.Errorf("0 or 5 = %v, want 5", result[0])
	}
	// 0 or 0 → 0 (last falsy)
	result = runBoru(t, r, []Value{
		NewInteger(0), NewWord("or"), NewInteger(0),
	})
	if v, _ := AsInteger(result[0]); v != 0 {
		t.Errorf("0 or 0 = %v, want 0", result[0])
	}
	// "" or "x" → "x"
	result = runBoru(t, r, []Value{
		NewString(""), NewWord("or"), NewString("x"),
	})
	if s, _ := AsString(result[0]); s != "x" {
		t.Errorf("\"\" or \"x\" = %v, want \"x\"", result[0])
	}
}

func TestIntegAndShortCircuitReturnsValue(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 1 and 2 → 2 (both truthy, last wins)
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("and"), NewInteger(2),
	})
	if v, _ := AsInteger(result[0]); v != 2 {
		t.Errorf("1 and 2 = %v, want 2", result[0])
	}
	// 0 and 5 → 0 (first falsy short-circuits)
	result = runBoru(t, r, []Value{
		NewInteger(0), NewWord("and"), NewInteger(5),
	})
	if v, _ := AsInteger(result[0]); v != 0 {
		t.Errorf("0 and 5 = %v, want 0", result[0])
	}
	// 1 and 0 → 0 (second is falsy)
	result = runBoru(t, r, []Value{
		NewInteger(1), NewWord("and"), NewInteger(0),
	})
	if v, _ := AsInteger(result[0]); v != 0 {
		t.Errorf("1 and 0 = %v, want 0", result[0])
	}
	// "x" and "y" → "y"
	result = runBoru(t, r, []Value{
		NewString("x"), NewWord("and"), NewString("y"),
	})
	if s, _ := AsString(result[0]); s != "y" {
		t.Errorf("\"x\" and \"y\" = %v, want \"y\"", result[0])
	}
}

// === 9b. tand for conjunction ===

func TestIntegTandMergeMaps(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// {x:1} tand {y:Integer} -> {x:1,y:Integer}
	left := NewOrderedMap()
	left.Set("x", NewInteger(1))
	right := NewOrderedMap()
	right.Set("y", NewTypeLiteral(TInteger))
	result := runBoru(t, r, []Value{
		NewMap(left), NewWord("tand"), NewMap(right),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	merged, _ := AsMap(result[0])
	if merged == nil {
		t.Fatalf("expected merged map, got %v", result[0])
	}
	if merged.Len() != 2 {
		t.Errorf("merged map should have 2 keys, got %d", merged.Len())
	}
	x, okX := merged.Get("x")
	if !okX {
		t.Fatalf("missing key x")
	}
	if xi, _ := AsInteger(x); xi != 1 {
		t.Errorf("x = %v, want 1", x)
	}
	y, okY := merged.Get("y")
	if !okY {
		t.Fatalf("missing key y")
	}
	if !(&y).Equal(TInteger) || y.Data != nil {
		t.Errorf("y = %v, want Integer type literal", y)
	}
}

func TestIntegTandMergeOverlap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// {x:1} tand {x:Integer} -> {x:1} (1 unifies with Integer to 1)
	left := NewOrderedMap()
	left.Set("x", NewInteger(1))
	right := NewOrderedMap()
	right.Set("x", NewTypeLiteral(TInteger))
	result := runBoru(t, r, []Value{
		NewMap(left), NewWord("tand"), NewMap(right),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	merged, _ := AsMap(result[0])
	if merged == nil || merged.Len() != 1 {
		t.Fatalf("expected single-key merged map, got %v", result[0])
	}
	x, _ := merged.Get("x")
	if xi, _ := AsInteger(x); xi != 1 {
		t.Errorf("x = %v, want 1", x)
	}
}

func TestIntegTandUnifyScalars(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 1 tand Integer -> 1
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("tand"), NewTypeLiteral(TInteger),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if v, _ := AsInteger(result[0]); v != 1 {
		t.Errorf("1 tand Integer = %v, want 1", result[0])
	}
}

// === 10. context word ===

func TestIntegContextSetGet(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// set "key" 42 context get "key" context
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("mykey"), NewInteger(42),
		NewWord("context"), NewWord("get"), NewString("mykey"),
	})
	_as42, _ := AsInteger(result[0])
	if len(result) != 1 || _as42 != 42 {
		t.Errorf("context set/get = %v, want 42", result)
	}
}

func TestIntegContextGetMissing(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// get on non-existent key => error (unknown key)
	err := runBoruError(t, r, []Value{
		NewWord("context"), NewWord("get"), NewString("nonexistent"),
	})
	if err == nil {
		t.Error("expected error for dot on non-existent key")
	}
}

func TestIntegContextSetWithWord(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// set wkey 99 context get wkey context
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("wkey"), NewInteger(99),
		NewWord("context"), NewWord("dot"), NewWord("wkey"),
	})
	_as43, _ := AsInteger(result[0])
	if len(result) != 1 || _as43 != 99 {
		t.Errorf("context set/dot with word key = %v, want 99", result)
	}
}

func TestIntegContextOverwrite(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// set, then overwrite, then get
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(2),
		NewWord("context"), NewWord("get"), NewString("k"),
	})
	_as44, _ := AsInteger(result[0])
	if len(result) != 1 || _as44 != 2 {
		t.Errorf("context overwrite = %v, want 2", result)
	}
}

func TestIntegContextPushesStore(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// context is a 0-arg word that pushes the context store onto the stack
	result := runBoru(t, r, []Value{
		NewWord("context"),
	})
	if len(result) != 1 {
		t.Errorf("context should push 1 value, got %d", len(result))
	}
}

// === Additional edge cases ===

func TestIntegFileIOWriteListAsJSON(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// Write a list value with fmt:json
	list := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("json"))
	runBoru(t, r, []Value{
		NewWord("write"), pathV("list.json"), list, NewMap(opts),
	})

	data, err := mem.ReadFile("list.json")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "[1,2,3]") {
		t.Errorf("written JSON = %q, want to contain [1,2,3]", data)
	}
}

func TestIntegFileIOReadLines(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	_ = mem.WriteFile("lines.txt", []byte("a\nb\nc"), 0644)
	opts := NewOrderedMap()
	opts.Set("fmt", NewString("lines"))
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("lines.txt"), NewMap(opts),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TList) {
		t.Fatalf("read lines should return list, got %v", result)
	}
	_lst, _ := AsList(result[0])
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("read lines should have 3 elements, got %d", len(elems))
	}
}

func TestIntegDoMapWithNonListValues(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// do {x: 42, y: "hello"} — non-list values should pass through
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	m.Set("y", NewString("hello"))
	result := runBoru(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("do map should return map, got %v", result)
	}
	rm, _ := AsMap(result[0])
	xVal, _ := rm.Get("x")
	_as45, _ := AsInteger(xVal)
	if _as45 != 42 {
		t.Errorf("do {x:42}.x = %v, want 42", xVal)
	}
}

func TestIntegFileIOReadJsonicFormat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	_ = mem.WriteFile("data.jsonic", []byte(`{x: 1, y: 2}`), 0644)
	result := runBoru(t, r, []Value{
		NewWord("read"), pathV("data.jsonic"),
	})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Fatalf("read jsonic should return map, got %v", result)
	}
}

func TestIntegFileIOWriteAppendNewFile(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// append to non-existent file should just write
	opts := NewOrderedMap()
	opts.Set("mode", NewString("append"))
	runBoru(t, r, []Value{
		NewWord("write"), pathV("new.txt"), NewString("fresh"), NewMap(opts),
	})

	data, _ := mem.ReadFile("new.txt")
	if string(data) != "fresh" {
		t.Errorf("append to new file = %q, want 'fresh'", data)
	}
}
