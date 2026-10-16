package native

import (
	"strings"
	"testing"
)

// =============================================================================
// TestEngineCoreStepEnd — exercises stepEnd with various forward/end patterns
// =============================================================================

func TestEngineCoreStepEndMultipleEnds(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Multiple ends with no pending forwards — should be harmlessly removed
	result := runBoru(t, r, []Value{
		NewInteger(5), NewEnd(), NewEnd(),
	})
	_as0, _ := AsNumber(result[0])
	if len(result) != 1 || _as0 != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEngineCoreStepEndTerminatesForwardAdd(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// "end" should terminate a forward expression: 10 add 20 end 99
	// The add consumes 10 and 20 via end, leaving 30 and 99
	result := runBoru(t, r, []Value{
		NewInteger(10), NewWord("add"), NewInteger(20), NewEnd(), NewInteger(99),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	_as1, _ := AsNumber(result[0])
	if _as1 != 30 {
		t.Errorf("result[0] = %v, want 30", result[0])
	}
	_as2, _ := AsNumber(result[1])
	if _as2 != 99 {
		t.Errorf("result[1] = %v, want 99", result[1])
	}
}

func TestEngineCoreStepEndSemicolonSequence(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// 1 add 2 end 3 add 4:
	// First add: fwd 2→sig[0], stack 1→sig[1] → add(2,1)=3.
	// Push 3 → stack=[3, 3]. Second add prefers stack: add(3,3)=6.
	// Push 4 → [6, 4]. Semicolons don't create stack barriers.
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewEnd(),
		NewInteger(3), NewWord("add"), NewInteger(4),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	_as3, _ := AsNumber(result[0])
	if _as3 != 3 {
		t.Errorf("result[0] = %v, want 3", result[0])
	}
	_as4, _ := AsNumber(result[1])
	if _as4 != 7 {
		t.Errorf("result[1] = %v, want 7", result[1])
	}
}

// =============================================================================
// TestEngineCoreParentheses — exercises stepOpenParen, stepCloseParen, findCloseParenAfter
// =============================================================================

func TestEngineCoreParenSimple(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// ( 2 add 3 ) => 5
	result := runBoru(t, r, []Value{
		NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
	})
	_as5, _ := AsNumber(result[0])
	if len(result) != 1 || _as5 != 5 {
		t.Errorf("(2 add 3) = %v, want 5", result)
	}
}

func TestEngineCoreParenNested(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// ( ( 1 add 2 ) add 3 ) => 6
	result := runBoru(t, r, []Value{
		NewOpenParen(), NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		NewWord("add"), NewInteger(3), NewCloseParen(),
	})
	_as6, _ := AsNumber(result[0])
	if len(result) != 1 || _as6 != 6 {
		t.Errorf("((1 add 2) add 3) = %v, want 6", result)
	}
}

func TestEngineCoreParenUnmatched(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewCloseParen(),
	})
	if err == nil {
		t.Error("expected error for unmatched closing paren")
	}
}

func TestEngineCoreParenUnmatchedOpen(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewOpenParen(), NewInteger(1),
	})
	if err == nil {
		t.Error("expected error for unmatched opening paren")
	}
}

func TestEngineCoreParenAsBarrier(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Parens create a scope barrier: 10 ( 2 add 3 ) => 10 5
	result := runBoru(t, r, []Value{
		NewInteger(10), NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
	})
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	_as7, _ := AsNumber(result[0])
	if _as7 != 10 {
		t.Errorf("result[0] = %v, want 10", result[0])
	}
	_as8, _ := AsNumber(result[1])
	if _as8 != 5 {
		t.Errorf("result[1] = %v, want 5", result[1])
	}
}

// =============================================================================
// TestEngineCoreFnDef — function definitions with typed params
// =============================================================================

func TestEngineCoreFnDefNamedParam(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def double fn [[x:Number] [Number] [x add x]] end
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewWord("x")}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})
	result := runBoru(t, r, []Value{NewInteger(7), NewWord("double")})
	_as9, _ := AsNumber(result[0])
	if len(result) != 1 || _as9 != 14 {
		t.Errorf("7 double = %v, want 14", result)
	}
}

func TestEngineCoreFnDefMultipleSigs(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Function with two overloads:
	// sig1: (Integer) -> Integer: x add 10
	// sig2: (String) -> String: x
	pairXInt := NewOrderedMap()
	pairXInt.Set("x", NewTypeLiteral(TInteger))
	pairXStr := NewOrderedMap()
	pairXStr.Set("x", NewTypeLiteral(TString))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairXInt)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(10)}),
		NewList([]Value{NewImplicitMap(pairXStr)}),
		NewList([]Value{NewTypeLiteral(TString)}),
		NewList([]Value{NewWord("x")}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-overload"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})

	// Integer overload
	result := runBoru(t, r, []Value{NewInteger(5), NewWord("my-overload")})
	_as10, _ := AsNumber(result[0])
	if len(result) != 1 || _as10 != 15 {
		t.Errorf("5 my-overload = %v, want 15", result)
	}

	// String overload
	result = runBoru(t, r, []Value{NewString("hello"), NewWord("my-overload")})
	_as11, _ := AsString(result[0])
	if len(result) != 1 || _as11 != "hello" {
		t.Errorf("'hello' my-overload = %v, want 'hello'", result)
	}
}

func TestEngineCoreFnReturnTypeCheck(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Function with return type checking: returns wrong type should error
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TString)}), // expects String return
		NewList([]Value{NewWord("x")}),            // but returns Number
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("bad-return"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})
	err := runBoruError(t, r, []Value{NewInteger(5), NewWord("bad-return")})
	if err == nil {
		t.Error("expected return type check error")
	}
}

func TestEngineCoreFnNonListBody(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// fn with non-list body abbreviation and named param: fn [{x:Number} [] 42]
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{}),
		NewInteger(42), // non-list body: abbreviation for [42]
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("always42"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("always42")})
	_as12, _ := AsNumber(result[0])
	if len(result) != 1 || _as12 != 42 {
		t.Errorf("1 always42 = %v, want 42", result)
	}
}

// =============================================================================
// TestEngineCoreUndef — undefining functions
// =============================================================================

func TestEngineCoreUndefBasic(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def my-val 100 end my-val => 100
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-val-undef"), NewInteger(100), NewEnd(),
	})
	result := runBoru(t, r, []Value{NewWord("my-val-undef")})
	_as13, _ := AsNumber(result[0])
	if len(result) != 1 || _as13 != 100 {
		t.Fatalf("my-val-undef = %v, want 100", result)
	}

	// undef my-val
	runBoru(t, r, []Value{NewWord("undef"), NewWord("my-val-undef")})

	// After undef, the word should error (undefined)
	e := New(r)
	_, runErr := e.Run([]Value{NewWord("my-val-undef")})
	if runErr == nil {
		t.Errorf("after undef, my-val-undef should error, got nil")
	}
}

func TestEngineCoreUndefShadowing(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def "x-shadow" 1 end def "x-shadow" 2 end x-shadow => 2
	runBoru(t, r, []Value{
		NewWord("def"), NewString("x-shadow"), NewInteger(1), NewEnd(),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewString("x-shadow"), NewInteger(2), NewEnd(),
	})
	result := runBoru(t, r, []Value{NewWord("x-shadow")})
	_as14, _ := AsNumber(result[0])
	if len(result) != 1 || _as14 != 2 {
		t.Fatalf("x-shadow = %v, want 2", result)
	}

	// undef x => reveals 1
	runBoru(t, r, []Value{NewWord("undef"), NewWord("x-shadow")})
	result = runBoru(t, r, []Value{NewWord("x-shadow")})
	_as15, _ := AsNumber(result[0])
	if len(result) != 1 || _as15 != 1 {
		t.Errorf("after first undef, x-shadow = %v, want 1", result)
	}
}

func TestEngineCoreUndefWithStringName(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("str-undef"), NewInteger(77), NewEnd(),
	})
	runBoru(t, r, []Value{NewWord("undef"), NewString("str-undef")})
	e := New(r)
	_, err := e.Run([]Value{NewWord("str-undef")})
	if err == nil {
		t.Errorf("after undef by string, str-undef should error, got nil")
	}
}

func TestEngineCoreUndefTargetedFnSig(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Define a function with a Number signature
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(pairX)}),
		NewList([]Value{NewTypeLiteral(TNumber)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("tgt-undef"),
		NewWord("fn"), fnBody,
		NewEnd(),
	})

	// Verify it works
	result := runBoru(t, r, []Value{NewInteger(10), NewWord("tgt-undef")})
	_as16, _ := AsNumber(result[0])
	if len(result) != 1 || _as16 != 11 {
		t.Fatalf("10 tgt-undef = %v, want 11", result)
	}

	// Targeted undef with fnsig: undef tgt-undef fnsig [[Number] [Number]]
	undefSpec := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
	})
	runBoru(t, r, []Value{
		NewWord("undef"), NewWord("tgt-undef"),
		NewWord("fnsig"), undefSpec,
	})
}

// =============================================================================
// TestEngineCoreMake — type conversion and record creation
// =============================================================================

func TestEngineCoreMakeStringToInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("42"),
	})
	_as17, _ := AsInteger(result[0])
	if len(result) != 1 || _as17 != 42 {
		t.Errorf("make Integer '42' = %v, want 42", result)
	}
}

func TestEngineCoreMakeStringToFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TFloat), NewString("3.14"),
	})
	if len(result) != 1 {
		t.Fatalf("make Float '3.14' got %d results", len(result))
	}
	_as18, _ := AsFloat(result[0])
	if _as18 != 3.14 {
		t.Errorf("make Float '3.14' = %v, want 3.14", result[0])
	}
}

func TestEngineCoreMakeToBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString("true"),
	})
	_as19, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as19 {
		t.Errorf("make Boolean 'true' = %v, want true", result)
	}

	// make Boolean coerces by presence, not content — a non-empty String
	// is true, so the text "false" is true.
	result = runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString("false"),
	})
	_as20, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as20 {
		t.Errorf("make Boolean 'false' = %v, want true", result)
	}

	result = runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewString(""),
	})
	_as21, _ := AsBoolean(result[0])
	if len(result) != 1 || _as21 {
		t.Errorf("make Boolean '' = %v, want false", result)
	}
}

func TestEngineCoreMakeToAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TAtom), NewString("hello"),
	})
	_as22, _ := AsAtom(result[0])
	if len(result) != 1 || _as22 != "hello" {
		t.Errorf("make Atom 'hello' = %v, want :hello", result)
	}
}

func TestEngineCoreMakeSameType(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// make Integer on integer should pass through
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewInteger(99),
	})
	_as23, _ := AsInteger(result[0])
	if len(result) != 1 || _as23 != 99 {
		t.Errorf("make Integer 99 = %v, want 99", result)
	}
}

func TestEngineCoreMakeErrorBadConversion(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("notanumber"),
	})
	if err == nil {
		t.Error("expected error for make Integer 'notanumber'")
	}
}

func TestEngineCoreMakeFloatTruncToInt(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// make Integer on decimal string should parse as float and truncate
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TInteger), NewString("3.7"),
	})
	_as24, _ := AsInteger(result[0])
	if len(result) != 1 || _as24 != 3 {
		t.Errorf("make Integer '3.7' = %v, want 3", result)
	}
}

func TestEngineCoreMakeBooleanFromNumber(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(0),
	})
	_as25, _ := AsBoolean(result[0])
	if len(result) != 1 || _as25 {
		t.Errorf("make Boolean 0 = %v, want false", result)
	}
	result = runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TBoolean), NewInteger(1),
	})
	_as26, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as26 {
		t.Errorf("make Boolean 1 = %v, want true", result)
	}
}

func TestEngineCoreMakeToString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TString), NewInteger(42),
	})
	_as27, _ := AsString(result[0])
	if len(result) != 1 || _as27 != "42" {
		t.Errorf("make String 42 = %v, want '42'", result)
	}
}

func TestEngineCoreMakeErrorBadFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	err := runBoruError(t, r, []Value{
		NewWord("make"), NewTypeLiteral(TFloat), NewString("xyz"),
	})
	if err == nil {
		t.Error("expected error for make Float 'xyz'")
	}
}

// =============================================================================
// TestEngineCoreRecord — record creation via make
// =============================================================================

func TestEngineCoreRecordMakePositional(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// record [x:Number y:String]
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType [1 "hello"]
	result := runBoru(t, r, []Value{
		NewWord("make"), recType, NewList([]Value{NewInteger(1), NewString("hello")}),
	})
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Fatalf("make record = %v, want map", result)
	}
	m, _ := AsMap(result[0])
	xv, _ := m.Get("x")
	yv, _ := m.Get("y")
	_as29, _ := AsNumber(xv)
	_as28, _ := AsString(yv)
	if _as29 != 1 || _as28 != "hello" {
		t.Errorf("record fields: x=%v y=%v", xv, yv)
	}
}

func TestEngineCoreRecordMakeMap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType {x:1 y:"hello"}
	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	src.Set("y", NewString("hello"))
	result := runBoru(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Fatalf("make record from map = %v, want map", result)
	}
	m, _ := AsMap(result[0])
	xv, _ := m.Get("x")
	_as30, _ := AsNumber(xv)
	if _as30 != 1 {
		t.Errorf("record x = %v, want 1", xv)
	}
}

func TestEngineCoreRecordMakeMissingField(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// Missing field y should error
	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	err := runBoruError(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if err == nil {
		t.Error("expected error for missing field")
	}
}

func TestEngineCoreRecordMakeUnknownField(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	src := NewOrderedMap()
	src.Set("x", NewInteger(1))
	src.Set("z", NewString("extra"))
	err := runBoruError(t, r, []Value{
		NewWord("make"), recType, NewMap(src),
	})
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestEngineCoreRecordMakeWrongFieldCount(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// Positional with wrong count
	err := runBoruError(t, r, []Value{
		NewWord("make"), recType, NewList([]Value{NewInteger(1)}),
	})
	if err == nil {
		t.Error("expected error for wrong field count")
	}
}

// =============================================================================
// TestEngineCoreModule — module definitions with exports
// =============================================================================

func TestEngineCoreModuleSimple(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("val"), NewInteger(42), NewEnd(),
		NewWord("export"), NewAtom("myexp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	result := runBoru(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 || !result[0].Parent().Equal(TModuleInst) {
		t.Fatalf("module should return a Module, got %v", result)
	}
	desc, _ := asModuleDesc(result[0])
	if _, ok := desc.Exports["myexp"]; !ok {
		t.Error("missing 'myexp' export")
	}
}

func TestEngineCoreModuleImportAll(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("val"), NewInteger(88), NewEnd(),
		NewWord("export"), NewAtom("coreExp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("cmod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})
	runBoru(t, r, []Value{NewWord("import"), NewWord("cmod")})

	result := runBoru(t, r, []Value{NewWord("coreExp")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("coreExp should be a module namespace, got %v", result)
	}
	v, ok := moduleExportGet(result[0], "v")
	_as31, _ := AsInteger(v)
	if !ok || _as31 != 88 {
		t.Errorf("coreExp.v = %v, want 88", v)
	}
}

func TestEngineCoreModuleImportRename(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("val"), NewInteger(33), NewEnd(),
		NewWord("export"), NewAtom("origName"),
		NewMap(singleMap("v", NewString("val"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("rmod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})
	renameList := NewList([]Value{NewAtom("origName"), NewAtom("newName")})
	runBoru(t, r, []Value{NewWord("import"), renameList, NewWord("rmod")})

	result := runBoru(t, r, []Value{NewWord("newName")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("newName should be a module namespace, got %v", result)
	}
}

func TestEngineCoreModuleImportMultiRename(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("a"), NewInteger(1), NewEnd(),
		NewWord("def"), NewString("b"), NewInteger(2), NewEnd(),
		NewWord("export"), NewAtom("ea"),
		NewMap(singleMap("v", NewString("a"))),
		NewWord("export"), NewAtom("eb"),
		NewMap(singleMap("v", NewString("b"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("mmod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	renameList := NewList([]Value{
		NewList([]Value{NewAtom("ea"), NewAtom("ra")}),
		NewList([]Value{NewAtom("eb"), NewAtom("rb")}),
	})
	runBoru(t, r, []Value{NewWord("import"), renameList, NewWord("mmod")})

	result := runBoru(t, r, []Value{NewWord("ra")})
	if len(result) != 1 || ModuleNSOf(result[0]) == nil {
		t.Fatalf("ra should be a module namespace, got %v", result)
	}
}

func TestEngineCoreModuleImportEmptyRenameError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(1), NewEnd(),
		NewWord("export"), NewAtom("ex"),
		NewMap(singleMap("v", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("emod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// Empty rename list should error
	err := runBoruError(t, r, []Value{
		NewWord("import"), NewList([]Value{}), NewWord("emod"),
	})
	if err == nil {
		t.Error("expected error for empty rename list")
	}
}

func TestEngineCoreModuleImportMissingExportError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("x"), NewInteger(1), NewEnd(),
		NewWord("export"), NewAtom("ex"),
		NewMap(singleMap("v", NewString("x"))),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("memod"),
		NewOpenParen(), NewWord("module"), moduleBody, NewCloseParen(),
		NewEnd(),
	})

	// Try to rename a non-existent export
	renameList := NewList([]Value{NewAtom("nonexistent"), NewAtom("target")})
	err := runBoruError(t, r, []Value{
		NewWord("import"), renameList, NewWord("memod"),
	})
	if err == nil {
		t.Error("expected error for missing export in rename")
	}
}

func TestEngineCoreModuleExportWithAtomString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Export using atom name (strings inside module are promoted to words)
	moduleBody := NewList([]Value{
		NewWord("def"), NewString("val"), NewInteger(77), NewEnd(),
		NewWord("export"), NewAtom("atexp"),
		NewMap(singleMap("v", NewString("val"))),
	})
	result := runBoru(t, r, []Value{NewWord("module"), moduleBody})
	if len(result) != 1 {
		t.Fatalf("module should return 1 value, got %d", len(result))
	}
	desc, _ := asModuleDesc(result[0])
	if _, ok := desc.Exports["atexp"]; !ok {
		t.Error("missing 'atexp' export")
	}
}

// =============================================================================
// TestEngineCoreBaseValue — base values for different types
// =============================================================================

func TestEngineCoreBaseInteger(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(5), NewWord("base")})
	_as32, _ := AsInteger(result[0])
	if len(result) != 1 || _as32 != 0 {
		t.Errorf("base Integer = %v, want 0", result)
	}
}

func TestEngineCoreBaseFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewFloat(3.14), NewWord("base")})
	_as33, _ := AsFloat(result[0])
	if len(result) != 1 || _as33 != 0 {
		t.Errorf("base Float = %v, want 0", result)
	}
}

func TestEngineCoreBaseString(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewString("hello"), NewWord("base")})
	_as34, _ := AsString(result[0])
	if len(result) != 1 || _as34 != "" {
		t.Errorf("base String = %v, want ''", result)
	}
}

func TestEngineCoreBaseBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("base")})
	_as35, _ := AsBoolean(result[0])
	if len(result) != 1 || _as35 {
		t.Errorf("base Boolean = %v, want false", result)
	}
}

func TestEngineCoreBaseList(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewList([]Value{NewInteger(1)}), NewWord("base"),
	})
	if len(result) != 1 {
		t.Fatalf("base List got %d results", len(result))
	}
	_lst, _ := AsList(result[0])
	if len(_lst.Slice()) != 0 {
		t.Errorf("base List = %v, want empty list", result)
	}
}

func TestEngineCoreBaseMap(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	result := runBoru(t, r, []Value{NewMap(m), NewWord("base")})
	if len(result) != 1 {
		t.Fatalf("base Map got %d results", len(result))
	}
	_m, _ := AsMap(result[0])
	if _m.Len() != 0 {
		t.Errorf("base Map = %v, want empty map", result)
	}
}

func TestEngineCoreBaseAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewAtom("foo"), NewWord("base")})
	_as36, _ := AsAtom(result[0])
	if len(result) != 1 || _as36 != "" {
		t.Errorf("base Atom = %v, want empty atom", result)
	}
}

// =============================================================================
// TestEngineCoreBaseValueForConstraint — direct unit tests
// =============================================================================

func TestEngineCoreBaseValueForConstraintTypeLiteral(t *testing.T) {
	v, err := BaseValueForConstraint(NewTypeLiteral(TString))
	if err != nil {
		t.Fatalf("BaseValueForConstraint(String): %v", err)
	}
	_as37, _ := AsString(v)
	if _as37 != "" {
		t.Errorf("base String = %v, want ''", v)
	}
}

func TestEngineCoreBaseValueForConstraintDisjunct(t *testing.T) {
	d := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	v, err := BaseValueForConstraint(d)
	if err != nil {
		t.Fatalf("BaseValueForConstraint(String|None): %v", err)
	}
	_as38, _ := AsString(v)
	if _as38 != "" {
		t.Errorf("base String|None = %v, want ''", v)
	}
}

func TestEngineCoreBaseValueForConstraintAllNone(t *testing.T) {
	d := NewDisjunct([]Value{NewTypeLiteral(TNone)})
	v, err := BaseValueForConstraint(d)
	if err != nil {
		t.Fatalf("BaseValueForConstraint(None): %v", err)
	}
	if !IsNoneShape(v) {
		t.Errorf("base all-None disjunct = %v, want None", v)
	}
}

func TestEngineCoreBaseValueForConstraintConcreteError(t *testing.T) {
	_, err := BaseValueForConstraint(NewInteger(42))
	if err == nil {
		t.Error("expected error for concrete constraint")
	}
}

// =============================================================================
// TestEngineCorePeekForwardValue — peek at forward value resolution
// =============================================================================

func TestEngineCorePeekForwardBoolTrue(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// "true" as forward should resolve to boolean
	// Test indirectly: def myval true end myval => true
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("true-val"), NewWord("true"), NewEnd(),
		NewWord("true-val"),
	})
	_as39, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as39 {
		t.Errorf("def true-val true = %v, want true", result)
	}
}

func TestEngineCorePeekForwardBoolFalse(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("false-val"), NewWord("false"), NewEnd(),
		NewWord("false-val"),
	})
	_as40, _ := AsBoolean(result[0])
	if len(result) != 1 || _as40 {
		t.Errorf("def false-val false = %v, want false", result)
	}
}

func TestEngineCorePeekForwardAtom(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// An explicit Atom value can be the body of a def (def atom-val 'myatom).
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("atom-val"), NewAtom("myatom"), NewEnd(),
		NewWord("atom-val"),
	})
	if len(result) != 1 || !result[0].Parent().Equal(TAtom) {
		t.Errorf("def atom-val 'myatom = %v, want atom", result)
	}
}

// =============================================================================
// TestEngineCoreForward — forward operations, left-to-right evaluation
// =============================================================================

func TestEngineCoreForwardLeftToRight(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// left-to-right: 2 add 3 mul 4 => (2 + 3) * 4 = 20
	result := runBoru(t, r, []Value{
		NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
	})
	_as41, _ := AsNumber(result[0])
	if len(result) != 1 || _as41 != 20 {
		t.Errorf("2 add 3 mul 4 = %v, want 20", result)
	}
}

func TestEngineCoreForwardLeftToRightReverse(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// left-to-right: 2 mul 3 add 4 => (2 * 3) + 4 = 10
	result := runBoru(t, r, []Value{
		NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("add"), NewInteger(4),
	})
	_as42, _ := AsNumber(result[0])
	if len(result) != 1 || _as42 != 10 {
		t.Errorf("2 mul 3 add 4 = %v, want 10", result)
	}
}

// =============================================================================
// TestEngineCoreForceForward — force forward via WordInfo
// =============================================================================

func TestEngineCoreForceForward(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Force forward on add: ALL args must come from forward.
	// add/f 10 5 → forward-collects both → add(10, 5) = 15
	result := runBoru(t, r, []Value{
		NewWordModified("add", -1, false, true),
		NewInteger(10),
		NewInteger(5),
	})
	_as43, _ := AsNumber(result[0])
	if len(result) != 1 || _as43 != 15 {
		t.Errorf("~add 10 5 = %v, want 15", result)
	}
}

// =============================================================================
// TestEngineCoreTypeLiterals — type name resolution
// =============================================================================

func TestEngineCoreTypeNameResolution(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Type names resolve to type literals
	for _, name := range []string{"Number", "String", "Boolean", "Integer", "Float", "List", "Map", "Atom"} {
		result := runBoru(t, r, []Value{NewWord(name)})
		if len(result) != 1 || result[0].Data() != nil {
			t.Errorf("%s should resolve to type literal, got %v", name, result)
		}
	}
}

func TestEngineCoreUnknownWordErrors(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	e := New(r)
	_, err := e.Run([]Value{NewWord("unknownXyz")})
	if err == nil {
		t.Error("expected error for undefined word, got nil")
	}
}

// =============================================================================
// TestEngineCoreFnSigMatchesSpec — direct unit test
// =============================================================================

func TestEngineCoreFnSigMatchesSpecBasic(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Name: "x", Type: TNumber}},
		Returns: []*Type{TNumber}, BarrierPos: -1,
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []*Type{TNumber},
	}
	if !FnSigMatchesSpec(sig, spec) {
		t.Error("sig should match spec (types match, ignoring names)")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffParamCount(t *testing.T) {
	sig := FnSig{
		Params: []FnParam{{Type: TNumber}}, BarrierPos: -1,
	}
	spec := FnSigSpec{
		Params: []FnParam{{Type: TNumber}, {Type: TString}},
	}
	if FnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different param count")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffReturnCount(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []*Type{TNumber}, BarrierPos: -1,
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []*Type{TNumber, TString},
	}
	if FnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different return count")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffReturnType(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []*Type{TNumber}, BarrierPos: -1,
	}
	spec := FnSigSpec{
		Params:  []FnParam{{Type: TNumber}},
		Returns: []*Type{TString},
	}
	if FnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different return type")
	}
}

func TestEngineCoreFnSigMatchesSpecDiffParamType(t *testing.T) {
	sig := FnSig{
		Params: []FnParam{{Type: TNumber}}, BarrierPos: -1,
	}
	spec := FnSigSpec{
		Params: []FnParam{{Type: TString}},
	}
	if FnSigMatchesSpec(sig, spec) {
		t.Error("sig should not match spec with different param type")
	}
}

// =============================================================================
// TestEngineCoreResolveTypeName — covers resolveTypeName branches
// =============================================================================

func TestEngineCoreResolveTypeName(t *testing.T) {
	cases := []struct {
		name string
		want *Type
	}{
		{"Any", TAny},
		{"None", TNone},
		{"Number", TNumber},
		{"Integer", TInteger},
		{"Float", TFloat},
		{"String", TString},
		{"Boolean", TBoolean},
		{"List", TList},
		{"Map", TMap},
		{"Function", TFunction},
	}
	for _, tc := range cases {
		got, err := resolveTypeName(tc.name)
		if err != nil {
			t.Errorf("resolveTypeName(%q): %v", tc.name, err)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("resolveTypeName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEngineCoreResolveTypeNameCustom(t *testing.T) {
	// Unregistered names error — undefined types fail at construction.
	if _, err := resolveTypeName("MyCustom"); err == nil {
		t.Error("expected error for unregistered type 'MyCustom'")
	}
}

// =============================================================================
// TestEngineCoreParseFnParams — covers parseFnParams edge cases
// =============================================================================

func TestEngineCoreParseFnParamsMapBadKeyCount(t *testing.T) {
	// Map with 2 keys should fail
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TNumber))
	m.Set("y", NewTypeLiteral(TString))
	input := NewList([]Value{NewImplicitMap(m)})
	_, _, err := parseFnParams(nil, input)
	if err == nil {
		t.Error("expected error for map with 2 keys")
	}
}

func TestEngineCoreParseFnParamsNonList(t *testing.T) {
	_, _, err := parseFnParams(nil, NewInteger(42))
	if err == nil {
		t.Error("expected error for non-list input")
	}
}

func TestEngineCoreParseFnParamsTypeLiteral(t *testing.T) {
	// Type literal (Data==nil) should work
	input := NewList([]Value{NewTypeLiteral(TString)})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams type literal: %v", err)
	}
	if len(params) != 1 || !params[0].Type.Equal(TString) {
		t.Errorf("params = %v, want [{Type:String}]", params)
	}
}

func TestEngineCoreParseFnParamsIntegerLiteral(t *testing.T) {
	input := NewList([]Value{NewInteger(0)})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams integer: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsBooleanLiteral(t *testing.T) {
	input := NewList([]Value{NewBoolean(true)})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams boolean: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsStringLiteral(t *testing.T) {
	input := NewList([]Value{NewString("hello")})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("parseFnParams string: %v", err)
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}
}

func TestEngineCoreParseFnParamsInvalidElem(t *testing.T) {
	// A list as a param element is invalid
	input := NewList([]Value{NewList([]Value{NewInteger(1)})})
	_, _, err := parseFnParams(nil, input)
	if err == nil {
		t.Error("expected error for list element in params")
	}
}

// =============================================================================
// TestEngineCoreParseFnParams — implicit vs explicit map
// =============================================================================

func TestEngineCoreParseFnParamsImplicitMapIsNamedParam(t *testing.T) {
	// Implicit map (from pair syntax [x:Number]) → named param
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TNumber))
	input := NewList([]Value{NewImplicitMap(m)})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "x" {
		t.Errorf("expected named param 'x', got %q", params[0].Name)
	}
	if !params[0].Type.Equal(TNumber) {
		t.Errorf("expected refine Number, got %s", params[0].Type)
	}
}

func TestEngineCoreParseFnParamsExplicitMapIsUnnamedParam(t *testing.T) {
	// Explicit map ({a:1}) → unnamed param with pattern
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	input := NewList([]Value{NewMap(m)})
	params, _, err := parseFnParams(nil, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "" {
		t.Errorf("expected unnamed param, got name %q", params[0].Name)
	}
	if !params[0].Type.Equal(TMap) {
		t.Errorf("expected refine Map, got %s", params[0].Type)
	}
	if params[0].Pattern == nil {
		t.Error("expected pattern for explicit map param")
	}
}

func TestEngineCoreFnImplicitMapNamedParamE2E(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def inc fn [[x:Integer] [Integer] [x add 1]] end 5 inc
	xParam := NewOrderedMap()
	xParam.Set("x", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("inc"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("inc"),
	})
	_as46, _ := AsInteger(result[0])
	if len(result) != 1 || _as46 != 6 {
		t.Errorf("5 inc = %v, want 6", result)
	}
}

func TestEngineCoreFnExplicitMapPatternE2E(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def foo fn [[{a:1}] [] ["matched"]] end
	// {a:1} foo → should match
	// {a:2} foo → should not match
	patternMap := NewOrderedMap()
	patternMap.Set("a", NewInteger(1))
	fnBody := NewList([]Value{
		NewList([]Value{NewMap(patternMap)}),
		NewList([]Value{}),
		NewList([]Value{NewString("matched")}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewWord("fn"), fnBody, NewEnd(),
	})

	// {a:1} foo → match
	argMap := NewOrderedMap()
	argMap.Set("a", NewInteger(1))
	result := runBoru(t, r, []Value{NewMap(argMap), NewWord("foo")})
	found := false
	for _, v := range result {
		_as47, _ := AsString(v)
		if v.Parent().ConformsTo(TString) && _as47 == "matched" {
			found = true
		}
	}
	if !found {
		t.Errorf("{a:1} foo: expected 'matched' in result, got %v", result)
	}

	// {a:2} foo → no match
	noMatch := NewOrderedMap()
	noMatch.Set("a", NewInteger(2))
	err := runBoruError(t, r, []Value{NewMap(noMatch), NewWord("foo")})
	if err == nil {
		t.Error("expected signature error for {a:2} foo")
	}
}

func TestEngineCoreFnExplicitMapNotNamedParam(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// def bar fn [[{x:Integer}] [Integer] [x add 1]] end
	// 5 bar → should FAIL because {x:Integer} is a map pattern, not a
	// named param "x". So x is not bound and the body can't use it.
	typeMap := NewOrderedMap()
	typeMap.Set("x", NewTypeLiteral(TInteger))
	fnBody := NewList([]Value{
		NewList([]Value{NewMap(typeMap)}),
		NewList([]Value{NewTypeLiteral(TInteger)}),
		NewList([]Value{NewWord("x"), NewWord("add"), NewInteger(1)}),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("bar"), NewWord("fn"), fnBody, NewEnd(),
	})

	// 5 bar → fails: no map on stack, or x not bound
	err := runBoruError(t, r, []Value{NewInteger(5), NewWord("bar")})
	if err == nil {
		t.Error("expected error: explicit map should not create named param")
	}
}

// =============================================================================
// TestEngineCoreValToAtomOrString — covers all branches
// =============================================================================

func TestEngineCoreValToAtomOrStringCoverage(t *testing.T) {
	if v := valToAtomOrString(NewWord("hello")); v != "hello" {
		t.Errorf("word: got %q, want 'hello'", v)
	}
	if v := valToAtomOrString(NewAtom("foo")); v != "foo" {
		t.Errorf("atom: got %q, want 'foo'", v)
	}
	if v := valToAtomOrString(NewString("bar")); v != "bar" {
		t.Errorf("string: got %q, want 'bar'", v)
	}
	// Fallback: integer uses .String()
	s := valToAtomOrString(NewInteger(42))
	if s != "42" {
		t.Errorf("integer fallback: got %q, want '42'", s)
	}
}

// =============================================================================
// TestEngineCoreMakeRecordWithBase — make with base:true option
// =============================================================================

func TestEngineCoreMakeRecordWithBase(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := NewRecordType(fields)

	// make RecType {x:5} {base:true} — y should get base value ""
	src := NewOrderedMap()
	src.Set("x", NewInteger(5))
	opts := NewOrderedMap()
	opts.Set("base", NewBoolean(true))
	result := runBoru(t, r, []Value{
		NewWord("make"), recType, NewMap(src), NewMap(opts),
	})
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Fatalf("make with base = %v, want map", result)
	}
	m, _ := AsMap(result[0])
	yv, ok := m.Get("y")
	if !ok {
		t.Fatal("missing field y")
	}
	_as48, _ := AsString(yv)
	if _as48 != "" {
		t.Errorf("y base value = %v, want ''", yv)
	}
}

// =============================================================================
// TestEngineCoreResolveFieldType — covers ResolveFieldType branches
// =============================================================================

func TestEngineCoreResolveFieldTypePassthrough(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Non-string, non-list value should pass through. v is a type
	// literal; its denoted lattice node is &v.
	v := ResolveFieldType(r, NewTypeLiteral(TNumber))
	if !(&v).Equal(TNumber) {
		t.Errorf("ResolveFieldType passthrough = %v, want Number", v.String())
	}
}

func TestEngineCoreResolveFieldTypeStringRef(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Define a type, then ResolveFieldType should find it
	InstallDef(r, "MyType", NewTypeLiteral(TString))
	v := ResolveFieldType(r, NewString("MyType"))
	if !(&v).Equal(TString) {
		t.Errorf("ResolveFieldType string ref = %v, want String type literal", v.String())
	}
	UninstallDef(r, "MyType")
}

func TestEngineCoreResolveFieldTypeStringNoRef(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	v := ResolveFieldType(r, NewString("NoSuchType"))
	_as49, _ := AsString(v)
	if _as49 != "NoSuchType" {
		t.Errorf("ResolveFieldType unresolved string = %v, want 'NoSuchType'", v)
	}
}

func TestEngineCoreResolveFieldTypeList(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// A list like [string tor none] should be evaluated as code
	list := NewList([]Value{NewString("string"), NewString("tor"), NewString("none")})
	v := ResolveFieldType(r, list)
	// Should produce a disjunction
	if !IsDisjunct(&v) {
		// If it didn't produce a disjunction, at least it shouldn't crash
		t.Logf("ResolveFieldType list = %v (type %s)", v, v.Parent())
	}
}

// =============================================================================
// TestEngineCoreMakeConvert — direct makeConvert unit tests
// =============================================================================

func TestEngineCoreMakeConvertStringToString(t *testing.T) {
	v, err := makeConvert(NewInteger(42), TString)
	if err != nil {
		t.Fatalf("makeConvert to string: %v", err)
	}
	_as50, _ := AsString(v)
	if _as50 != "42" {
		t.Errorf("makeConvert 42 to String = %v, want '42'", v)
	}
}

func TestEngineCoreMakeConvertStringToFloat(t *testing.T) {
	v, err := makeConvert(NewString("2.5"), TFloat)
	if err != nil {
		t.Fatalf("makeConvert to decimal: %v", err)
	}
	_as51, _ := AsFloat(v)
	if _as51 != 2.5 {
		t.Errorf("makeConvert '2.5' to Float = %v, want 2.5", v)
	}
}

func TestEngineCoreMakeConvertBadFloat(t *testing.T) {
	_, err := makeConvert(NewString("abc"), TFloat)
	if err == nil {
		t.Error("expected error converting 'abc' to decimal")
	}
}

func TestEngineCoreMakeConvertBadNumber(t *testing.T) {
	_, err := makeConvert(NewString("xyz"), TNumber)
	if err == nil {
		t.Error("expected error converting 'xyz' to number")
	}
}

func TestEngineCoreMakeConvertUnsupportedType(t *testing.T) {
	customType, _ := NewType("Custom/Thing")
	_, err := makeConvert(NewString("x"), customType)
	if err == nil {
		t.Error("expected error for unsupported target type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %v, want 'unsupported'", err)
	}
}

func TestEngineCoreMakeConvertBoolBool(t *testing.T) {
	v, err := makeConvert(NewBoolean(true), TBoolean)
	if err != nil {
		t.Fatalf("makeConvert bool to bool: %v", err)
	}
	_as52, _ := AsBoolean(v)
	if !_as52 {
		t.Error("makeConvert true to Boolean should be true")
	}
}

func TestEngineCoreMakeConvertNumberToBool(t *testing.T) {
	v, err := makeConvert(NewInteger(0), TBoolean)
	if err != nil {
		t.Fatalf("makeConvert 0 to bool: %v", err)
	}
	_as53, _ := AsBoolean(v)
	if _as53 {
		t.Error("makeConvert 0 to Boolean should be false")
	}
}

func TestEngineCoreMakeConvertToAtom(t *testing.T) {
	v, err := makeConvert(NewString("foo"), TAtom)
	if err != nil {
		t.Fatalf("makeConvert to atom: %v", err)
	}
	_as54, _ := AsAtom(v)
	if _as54 != "foo" {
		t.Errorf("makeConvert 'foo' to Atom = %v, want :foo", v)
	}
}

// =============================================================================
// TestEngineCoreImportFileNoParser — loadFileModule error path
// =============================================================================

func TestEngineCoreImportFileNoParser(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	r.ParseFunc = nil
	_, err := loadFileModule(r, "some.boru")
	if err == nil {
		t.Error("expected error when ParseFunc is nil")
	}
	if !strings.Contains(err.Error(), "parser not configured") {
		t.Errorf("error = %v, want 'parser not configured'", err)
	}
}

// =============================================================================
// TestEngineCoreForceStack — force prefix via WordInfo
// =============================================================================

func TestEngineCoreForceStack(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Force prefix on add: both args must be before the word
	result := runBoru(t, r, []Value{
		NewInteger(3), NewInteger(4),
		NewWordModified("add", -1, true, false),
	})
	_as55, _ := AsNumber(result[0])
	if len(result) != 1 || _as55 != 7 {
		t.Errorf("3 4 ^add = %v, want 7", result)
	}
}

func TestEngineCoreForceStackNoMatchError(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Force prefix with no matching args
	err := runBoruError(t, r, []Value{
		NewWordModified("add", -1, true, false),
	})
	if err == nil {
		t.Error("expected signature error for prefix add with no args")
	}
}

// =============================================================================
// TestEngineCoreMakeRecordNamed — named fields in list form
// =============================================================================

func TestEngineCoreMakeRecordNamed(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("age", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	// Named fields via list of maps: [{name:"Alice"} {age:30}]
	n := NewOrderedMap()
	n.Set("name", NewString("Alice"))
	a := NewOrderedMap()
	a.Set("age", NewInteger(30))
	src := NewList([]Value{NewMap(n), NewMap(a)})

	result := runBoru(t, r, []Value{
		NewWord("make"), recType, src,
	})
	if len(result) != 1 || !result[0].Parent().Equal(TMap) {
		t.Fatalf("make record named = %v, want map", result)
	}
	m, _ := AsMap(result[0])
	nv, _ := m.Get("name")
	av, _ := m.Get("age")
	_as57, _ := AsString(nv)
	_as56, _ := AsNumber(av)
	if _as57 != "Alice" || _as56 != 30 {
		t.Errorf("record: name=%v age=%v", nv, av)
	}
}

// =============================================================================
// TestEngineCoreMakeRecordNonListNonMap — error path
// =============================================================================

func TestEngineCoreMakeRecordBadSource(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := NewRecordType(fields)

	err := runBoruError(t, r, []Value{
		NewWord("make"), recType, NewInteger(42),
	})
	if err == nil {
		t.Error("expected error for make record with integer source")
	}
}

// =============================================================================
// TestEngineCoreMakeTable — table type instance creation
// =============================================================================

func TestEngineCoreMakeTablePositional(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	// make TableType [[1 "a"] [2 "b"]]
	rows := NewList([]Value{
		NewList([]Value{NewInteger(1), NewString("a")}),
		NewList([]Value{NewInteger(2), NewString("b")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("make"), tableType, rows,
	})
	if len(result) != 1 {
		t.Fatalf("make table got %d results", len(result))
	}
	_lst, _ := AsList(result[0])
	rowList := _lst.Slice()
	if len(rowList) != 2 {
		t.Errorf("table has %d rows, want 2", len(rowList))
	}
}

func TestEngineCoreMakeTableBadRowCount(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	fields.Set("y", NewTypeLiteral(TString))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	// Row with wrong number of fields
	rows := NewList([]Value{
		NewList([]Value{NewInteger(1)}), // missing y
	})
	err := runBoruError(t, r, []Value{
		NewWord("make"), tableType, rows,
	})
	if err == nil {
		t.Error("expected error for table row with wrong field count")
	}
}

func TestEngineCoreMakeTableNonList(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TNumber))
	recType := RecordTypeInfo{Fields: fields}
	tableType := NewTableType(recType)

	err := runBoruError(t, r, []Value{
		NewWord("make"), tableType, NewInteger(42),
	})
	if err == nil {
		t.Error("expected error for make table with non-list source")
	}
}

// =============================================================================
// TestEngineCoreResolveWordValue — direct unit test
// =============================================================================

func TestEngineCoreResolveWordValue(t *testing.T) {
	v := ResolveWordValue(NewWord("true"))
	_as58, _ := AsBoolean(v)
	if !_as58 {
		t.Error("ResolveWordValue(true) should be boolean true")
	}
	v = ResolveWordValue(NewWord("false"))
	_as59, _ := AsBoolean(v)
	if _as59 {
		t.Error("ResolveWordValue(false) should be boolean false")
	}
	v = ResolveWordValue(NewWord("None"))
	if !IsNoneShape(v) && !(&v).Equal(TNone) {
		t.Errorf("ResolveWordValue(None) = %v, want None", v)
	}
	v = ResolveWordValue(NewWord("other"))
	if !v.Parent().Equal(TAtom) {
		t.Errorf("ResolveWordValue(other) = %v, want atom", v)
	}
	// Non-word passthrough
	v = ResolveWordValue(NewInteger(42))
	_as60, _ := AsInteger(v)
	if _as60 != 42 {
		t.Errorf("ResolveWordValue(42) = %v, want 42", v)
	}
}
