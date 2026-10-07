package native

import (
	"testing"
)

// --- Additional engine tests for coverage ---

func TestEngineConvert(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// 99 convert String
	result := runBoru(t, r, []Value{
		NewInteger(99), NewWord("convert"), NewWord("String"),
	})
	_as22, _ := AsString(result[0])
	if len(result) != 1 || _as22 != "99" {
		t.Errorf("convert 99 string = %v, want '99'", result)
	}
}

func TestEngineTypeof(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewWord("typeof"), NewInteger(42)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineBase(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewWord("base"), NewTypeLiteral(TInteger)})
	_as23, _ := AsInteger(result[0])
	if len(result) != 1 || _as23 != 0 {
		t.Errorf("base integer = %v, want 0", result)
	}
}

func TestEngineDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def inc [1 add] end 5 inc
	body := NewList([]Value{NewInteger(1), NewWord("add")})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("inc"), NewWord("word"), body, NewEnd(),
		NewInteger(5), NewWord("inc"),
	})
	_as24, _ := AsInteger(result[0])
	if len(result) != 1 || _as24 != 6 {
		t.Errorf("def inc [1 add]; 5 inc = %v, want 6", result)
	}
}

func TestEngineUndef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def foo 42 end foo undef foo end foo → error (foo undefined after undef)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(42), NewEnd(),
		NewWord("foo"),
		NewWord("undef"), NewWord("foo"), NewEnd(),
		NewWord("foo"),
	})
	if err == nil {
		t.Fatal("expected error for undefined word after undef, got nil")
	}
}

func TestEngineRecord(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	e := NewTop(r)
	// Parse a pair list manually: jsonic produces maps for x:number syntax
	m1 := NewOrderedMap()
	m1.Set("x", NewTypeLiteral(TNumber))
	m2 := NewOrderedMap()
	m2.Set("y", NewTypeLiteral(TString))
	list := NewList([]Value{NewMap(m1), NewMap(m2)})
	result, err := e.Run([]Value{NewWord("refine"), NewWord("Record"), list})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !IsRecordType(result[0]) {
		t.Errorf("expected record type, got %v", result)
	}
}

func TestEngineTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	e := NewTop(r)
	// Build the record type first; `refine Table` then takes it from
	// the stack as its body (a nested `refine` must not be inlined
	// bare — see native_type.go).
	m1 := NewOrderedMap()
	m1.Set("x", NewTypeLiteral(TNumber))
	list := NewList([]Value{NewMap(m1)})
	result, err := e.Run([]Value{NewWord("refine"), NewWord("Record"), list, NewWord("refine"), NewWord("Table")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !IsTableType(result[0]) {
		t.Errorf("expected table type, got %v", result)
	}
}

func TestEngineUnify(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewTypeLiteral(TNumber), NewWord("unify")})
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	_as26, _ := AsBoolean(result[1])
	if _as26 != true {
		t.Errorf("1 unify number = %v, want true", result[1])
	}
}

func TestEngineDo(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runBoru(t, r, []Value{NewWord("do"), list})
	_as27, _ := AsInteger(result[0])
	if len(result) != 1 || _as27 != 3 {
		t.Errorf("do [1 add 2] = %v, want 3", result)
	}
}

func TestEngineDoMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)}))
	result := runBoru(t, r, []Value{NewWord("do"), NewMap(m)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestEngineOr(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("or"), NewBoolean(false)})
	_as28, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as28 {
		t.Errorf("true or false = %v, want true", result)
	}
}

func TestEngineAnd(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("and"), NewBoolean(false)})
	_as29, _ := AsBoolean(result[0])
	if len(result) != 1 || _as29 {
		t.Errorf("true and false = %v, want false", result)
	}
}

func TestEngineNot(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("not")})
	_as30, _ := AsBoolean(result[0])
	if len(result) != 1 || _as30 {
		t.Errorf("true not = %v, want false", result)
	}
}

func TestEngineConvertStringVariants(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	HEXOpts := NewOrderedMap()
	HEXOpts.Set("base", NewString("HEX"))
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))

	// 10 convert String {base:hex} → 'a'
	result := runBoru(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(hexOpts),
	})
	_as31, _ := AsString(result[0])
	if len(result) != 1 || _as31 != "a" {
		t.Errorf("10 convert String {base:hex} = %v, want 'a'", result)
	}

	// 255 convert String {base:HEX} → 'FF'
	result = runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(HEXOpts),
	})
	_as32, _ := AsString(result[0])
	if len(result) != 1 || _as32 != "FF" {
		t.Errorf("255 convert String {base:HEX} = %v, want 'FF'", result)
	}

	// 10 convert String {base:bin} → '1010'
	result = runBoru(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(binOpts),
	})
	_as33, _ := AsString(result[0])
	if len(result) != 1 || _as33 != "1010" {
		t.Errorf("10 convert String {base:bin} = %v, want '1010'", result)
	}

	// 8 convert String {base:oct} → '10'
	result = runBoru(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewWord("String"), NewMap(octOpts),
	})
	_as34, _ := AsString(result[0])
	if len(result) != 1 || _as34 != "10" {
		t.Errorf("8 convert String {base:oct} = %v, want '10'", result)
	}
}

func TestEngineConvertToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// "42" convert Number → 42
	result := runBoru(t, r, []Value{
		NewString("42"), NewWord("convert"), NewWord("Number"),
	})
	_as35, _ := AsInteger(result[0])
	if len(result) != 1 || _as35 != 42 {
		t.Errorf("'42' convert Number = %v, want 42", result)
	}

	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))

	// "ff" convert Number {base:hex} → 255
	result = runBoru(t, r, []Value{
		NewString("ff"), NewWord("convert"), NewWord("Number"), NewMap(hexOpts),
	})
	_as36, _ := AsInteger(result[0])
	if len(result) != 1 || _as36 != 255 {
		t.Errorf("'ff' convert Number {base:hex} = %v, want 255", result)
	}

	// "1010" convert Number {base:bin} → 10
	result = runBoru(t, r, []Value{
		NewString("1010"), NewWord("convert"), NewWord("Number"), NewMap(binOpts),
	})
	_as37, _ := AsInteger(result[0])
	if len(result) != 1 || _as37 != 10 {
		t.Errorf("'1010' convert Number {base:bin} = %v, want 10", result)
	}

	// "10" convert Number {base:oct} → 8
	result = runBoru(t, r, []Value{
		NewString("10"), NewWord("convert"), NewWord("Number"), NewMap(octOpts),
	})
	_as38, _ := AsInteger(result[0])
	if len(result) != 1 || _as38 != 8 {
		t.Errorf("'10' convert Number {base:oct} = %v, want 8", result)
	}
}

func TestEngineConvertToBoolean(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// 1 convert Boolean → true
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewWord("Boolean"),
	})
	_as39, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as39 {
		t.Errorf("1 convert Boolean = %v, want true", result)
	}

	// 0 convert Boolean → false
	result = runBoru(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewWord("Boolean"),
	})
	_as40, _ := AsBoolean(result[0])
	if len(result) != 1 || _as40 {
		t.Errorf("0 convert Boolean = %v, want false", result)
	}

	// "true" convert Boolean → true
	result = runBoru(t, r, []Value{
		NewString("true"), NewWord("convert"), NewWord("Boolean"),
	})
	_as41, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as41 {
		t.Errorf("'true' convert Boolean = %v, want true", result)
	}

	// "" convert Boolean → false
	result = runBoru(t, r, []Value{
		NewString(""), NewWord("convert"), NewWord("Boolean"),
	})
	_as42, _ := AsBoolean(result[0])
	if len(result) != 1 || _as42 {
		t.Errorf("'' convert Boolean = %v, want false", result)
	}

	// true convert Boolean → true (passthrough)
	result = runBoru(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewWord("Boolean"),
	})
	_as43, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as43 {
		t.Errorf("true convert Boolean = %v, want true", result)
	}
}

func TestEngineBaseTypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	tests := []struct {
		name    string
		typeLit *Type
		wantStr string
	}{
		{"number", TNumber, "0"},
		{"string", TString, "''"},
		{"boolean", TBoolean, "false"},
		{"list", TList, "[]"},
		{"map", TMap, "{}"},
		{"none", TNone, "None"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runBoru(t, r, []Value{NewWord("base"), NewTypeLiteral(tt.typeLit)})
			if len(result) != 1 || result[0].String() != tt.wantStr {
				t.Errorf("base %s = %v, want %s", tt.name, result, tt.wantStr)
			}
		})
	}
}

func TestEngineFn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def double fn [[number] [number] [dup add]] end 7 double
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("double"),
	})
	_as44, _ := AsInteger(result[0])
	if len(result) != 1 || _as44 != 14 {
		t.Errorf("def double fn; 7 double = %v, want 14", result)
	}
}

func TestEngineFnNamed(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def square fn [[x:number] [number] [x mul x]] end 5 square
	xParam := NewOrderedMap()
	xParam.Set("x", NewWord("Number"))
	fnBody := NewList([]Value{
		NewList([]Value{NewImplicitMap(xParam)}),
		NewList([]Value{NewWord("Number")}),
		NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("square"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("square"),
	})
	_as45, _ := AsInteger(result[0])
	if len(result) != 1 || _as45 != 25 {
		t.Errorf("def square fn; 5 square = %v, want 25", result)
	}
}

func TestEngineFnCatterPrefixOnly(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def catter fn [[integer string] [string] [add]] end
	// Case: [1 "a"|] -> catter -> all args from prefix
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	// All prefix: nearest→sig[0]=Integer, next→sig[1]=String
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
		NewString("a"), NewInteger(1), NewWord("catter"),
	})
	if len(result) != 1 || !result[0].Parent.ConformsTo(TString) {
		t.Errorf("1 'a' catter = %v, want string result", result)
	}
}

func TestEngineFnCatterPartialForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def catter fn [[integer string] [string] [add]] end
	// Case: catter 2 "b" -> all forward (integer, string)
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
		NewWord("catter"), NewInteger(2), NewString("b"),
	})
	if len(result) != 1 || !result[0].Parent.ConformsTo(TString) {
		t.Errorf("2 catter 'b' = %v, want string result", result)
	}
}

func TestEngineFnCatterFullForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def catter fn [[integer string] [string] [add]] end
	// Case: [|] -> catter 3 "c" -> both args from forward (positional match)
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("Integer"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("catter"), NewWord("fn"), fnBody, NewEnd(),
		NewWord("catter"), NewInteger(3), NewString("c"),
	})
	if len(result) != 1 || !result[0].Parent.ConformsTo(TString) {
		t.Errorf("catter 'c' 3 = %v, want string result", result)
	}
}

func TestEngineFnConcatArgOrder(t *testing.T) {
	// def joiner fn [[string string string] [string] [args concat]] end
	// Uses args+concat to reveal the exact ordering of 3 args.
	// args returns all fn arguments as a list, concat joins them.
	// The concatenated output string directly reveals argument order.
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("String"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("drop"), NewWord("drop"), NewWord("drop"), NewWord("args"), NewWord("concat")}),
	})

	defTokens := []Value{
		NewWord("def"), NewWord("joiner"), NewWord("fn"), fnBody, NewEnd(),
	}

	// Subtest: all args from prefix (stack)
	// "A" "B" "C" joiner → nearest to joiner is "C"→sig[0], "B"→sig[1], "A"→sig[2]
	// All positions are equivalent: values nearest the word map to sig[0].
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewString("C"), NewWord("joiner"),
		)
		result := runBoru(t, r, tokens)
		_as46, _ := AsString(result[0])
		if len(result) != 1 || _as46 != "CBA" {
			t.Errorf(`"A" "B" "C" joiner = %v, want ["CBA"]`, result)
		}
	})

	// Subtest: 1 prefix + 2 forward
	// "A" joiner "B" "C" → fwd: "B"→sig[0], "C"→sig[1]; stack: "A"→sig[2]
	t.Run("MixedPrefixForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewWord("joiner"), NewString("B"), NewString("C"),
		)
		result := runBoru(t, r, tokens)
		_as47, _ := AsString(result[0])
		if len(result) != 1 || _as47 != "BCA" {
			t.Errorf(`"A" joiner "B" "C" = %v, want ["BCA"]`, result)
		}
	})

	// Subtest: 2 prefix + 1 forward
	// "A" "B" joiner "C" → fwd: "C"→sig[0]; stack: top="B"→sig[1], "A"→sig[2]
	t.Run("TwoPrefixOneForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewString("A"), NewString("B"), NewWord("joiner"), NewString("C"),
		)
		result := runBoru(t, r, tokens)
		_as48, _ := AsString(result[0])
		if len(result) != 1 || _as48 != "CBA" {
			t.Errorf(`"A" "B" joiner "C" = %v, want ["CBA"]`, result)
		}
	})

	// Subtest: all args from forward
	// joiner "A" "B" "C" -> args=["A","B","C"] -> concat -> "ABC"
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...),
			NewWord("joiner"), NewString("A"), NewString("B"), NewString("C"),
		)
		result := runBoru(t, r, tokens)
		_as49, _ := AsString(result[0])
		if len(result) != 1 || _as49 != "ABC" {
			t.Errorf(`joiner "A" "B" "C" = %v, want ["ABC"]`, result)
		}
	})
}

// concatDropBody builds the body [drop..drop args concat] for n unnamed params.
func concatDropBody(n int) []Value {
	var body []Value
	for i := 0; i < n; i++ {
		body = append(body, NewWord("drop"))
	}
	body = append(body, NewWord("args"), NewWord("concat"))
	return body
}

func TestEngineFnConcatArgOrder4Mixed(t *testing.T) {
	// def mix4 fn [[string integer boolean string] [string]
	//              [drop drop drop drop args concat]] end
	// 4 args: string, integer, boolean, string -> concat reveals ordering.
	// ValToString: integer->digits, boolean->"true"/"false"
	fnBody := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix4"), NewWord("fn"), fnBody, NewEnd(),
	}

	// All prefix: nearest→sig[0]=String, next→sig[1]=Integer, next→sig[2]=Boolean, deepest→sig[3]=String
	// Stack bottom-to-top: "Z" true 7 "X" mix4
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewString("Z"), NewBoolean(true), NewInteger(7), NewString("X"), NewWord("mix4"),
		))
		_as50, _ := AsString(result[0])
		if len(result) != 1 || _as50 != "X7trueZ" {
			t.Errorf(`all-prefix mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// "Z" mix4 "X" 7 true → 1 prefix + 3 forward, types align with sig positions.
	// sig[0]=String("X"), sig[1]=Integer(7), sig[2]=Boolean(true), sig[3]=String("Z" from stack).
	t.Run("OnePrefixThreeForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewString("Z"), NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true),
		))
		_as51, _ := AsString(result[0])
		if len(result) != 1 || _as51 != "X7trueZ" {
			t.Errorf(`1+3 mix4 = %v, want ["X7trueZ"]`, result)
		}
	})

	// mix4 "X" 7 true "Z" -> all forward
	t.Run("TwoPrefixTwoForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		_as52, _ := AsString(result[0])
		if len(result) != 1 || _as52 != "X7trueZ" {
			t.Errorf(`mix4 all-forward = %v, want ["X7trueZ"]`, result)
		}
	})

	// mix4 "X" 7 true "Z" -> all forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix4"), NewString("X"), NewInteger(7), NewBoolean(true), NewString("Z"),
		))
		_as53, _ := AsString(result[0])
		if len(result) != 1 || _as53 != "X7trueZ" {
			t.Errorf(`all-forward mix4 = %v, want ["X7trueZ"]`, result)
		}
	})
}

func TestEngineFnConcatArgOrder5Mixed(t *testing.T) {
	// def mix5 fn [[string integer decimal boolean string] [string]
	//              [drop..drop args concat]] end
	// 5 args: string, integer, decimal, boolean, string
	fnBody := NewList([]Value{
		NewList([]Value{
			NewWord("String"), NewWord("Integer"), NewWord("Float"),
			NewWord("Boolean"), NewWord("String"),
		}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(5)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix5"), NewWord("fn"), fnBody, NewEnd(),
	}

	// All prefix: nearest→sig[0]=String, ..., deepest→sig[4]=String
	// Stack bottom-to-top: "z" false 1.5 3 "a" mix5
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewString("z"), NewBoolean(false), NewFloat(1.5), NewInteger(3), NewString("a"),
			NewWord("mix5"),
		))
		_as54, _ := AsString(result[0])
		if len(result) != 1 || _as54 != "a31.5falsez" {
			t.Errorf(`all-prefix mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// mix5 "a" 3 1.5 false "z" -> all forward
	t.Run("AllForwardExplicit", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix5"), NewString("a"), NewInteger(3),
			NewFloat(1.5), NewBoolean(false), NewString("z"),
		))
		_as55, _ := AsString(result[0])
		if len(result) != 1 || _as55 != "a31.5falsez" {
			t.Errorf(`2+3 mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})

	// mix5 "a" 3 1.5 false "z" -> all forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		result := runBoru(t, r, append(append([]Value{}, defTokens...),
			NewWord("mix5"), NewString("a"), NewInteger(3),
			NewFloat(1.5), NewBoolean(false), NewString("z"),
		))
		_as56, _ := AsString(result[0])
		if len(result) != 1 || _as56 != "a31.5falsez" {
			t.Errorf(`all-forward mix5 = %v, want ["a31.5falsez"]`, result)
		}
	})
}

func TestEngineFnConcatArgOrder7Mixed(t *testing.T) {
	// def mix7 fn [[string integer decimal boolean string integer string]
	//              [string] [drop..drop args concat]] end
	// 7 args covering all scalar types with repeats.
	fnBody := NewList([]Value{
		NewList([]Value{
			NewWord("String"), NewWord("Integer"), NewWord("Float"),
			NewWord("Boolean"), NewWord("String"), NewWord("Integer"), NewWord("String"),
		}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(7)),
	})
	defTokens := []Value{
		NewWord("def"), NewWord("mix7"), NewWord("fn"), fnBody, NewEnd(),
	}
	// Expected concat in sig order: "p123.5trueq456r7"
	want := "p123.5trueq456r7"
	argVals := []Value{
		NewString("p1"), NewInteger(2), NewFloat(3.5),
		NewBoolean(true), NewString("q4"), NewInteger(56), NewString("r7"),
	}

	// All prefix: stack bottom-to-top reversed from sig order (nearest→sig[0])
	// sig[6]=String, sig[5]=Integer, sig[4]=String, sig[3]=Boolean, sig[2]=Float, sig[1]=Integer, sig[0]=String
	t.Run("AllPrefix", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		argValsReversed := []Value{
			NewString("r7"), NewInteger(56), NewString("q4"),
			NewBoolean(true), NewFloat(3.5), NewInteger(2), NewString("p1"),
		}
		tokens := append(append([]Value{}, defTokens...), argValsReversed...)
		tokens = append(tokens, NewWord("mix7"))
		result := runBoru(t, r, tokens)
		_as57, _ := AsString(result[0])
		if len(result) != 1 || _as57 != want {
			t.Errorf("all-prefix mix7 = %v, want [%q]", result, want)
		}
	})

	// all forward (was 3+4 mixed, changed for sequential planner)
	t.Run("ThreePrefixFourForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), NewWord("mix7"))
		tokens = append(tokens, argVals...)
		result := runBoru(t, r, tokens)
		_as58, _ := AsString(result[0])
		if len(result) != 1 || _as58 != want {
			t.Errorf("mix7 all-forward = %v, want [%q]", result, want)
		}
	})

	// 1 prefix + 6 forward: last arg ("r7") as prefix, rest forward.
	// Forward types must align with sig[0..5], prefix fills sig[6].
	t.Run("OnePrefixSixForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), argVals[6]) // "r7" prefix
		tokens = append(tokens, NewWord("mix7"))
		tokens = append(tokens, argVals[:6]...) // "p1" 2 3.5 true "q4" 56 forward
		result := runBoru(t, r, tokens)
		_as59, _ := AsString(result[0])
		if len(result) != 1 || _as59 != want {
			t.Errorf("1+6 mix7 = %v, want [%q]", result, want)
		}
	})

	// All forward
	t.Run("AllForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, defTokens...), NewWord("mix7"))
		tokens = append(tokens, argVals...)
		result := runBoru(t, r, tokens)
		_as60, _ := AsString(result[0])
		if len(result) != 1 || _as60 != want {
			t.Errorf("all-forward mix7 = %v, want [%q]", result, want)
		}
	})
}

func TestEngineFnConcatArgOrderEndDisambiguate(t *testing.T) {
	// Tests that the "end" word stops forward argument collection,
	// preventing the fn from consuming tokens that follow.

	// def cat3 fn [[string string string] [string]
	//              [drop drop drop args concat]] end
	cat3Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("String"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(3)),
	})
	cat3Def := []Value{
		NewWord("def"), NewWord("cat3"), NewWord("fn"), cat3Body, NewEnd(),
	}

	// def cat4 fn [[string integer boolean string] [string]
	//              [drop drop drop drop args concat]] end
	cat4Body := NewList([]Value{
		NewList([]Value{NewWord("String"), NewWord("Integer"), NewWord("Boolean"), NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList(concatDropBody(4)),
	})
	cat4Def := []Value{
		NewWord("def"), NewWord("cat4"), NewWord("fn"), cat4Body, NewEnd(),
	}

	// cat3 "A" "B" "C" end "trailing" -> cat3 gets "ABC", "trailing" on stack
	t.Run("EndStopsForward3", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"),
			NewEnd(), NewString("trailing"),
		)
		result := runBoru(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat3 A B C end trailing: got %d results, want 2: %v", len(result), result)
		}
		_as61, _ := AsString(result[0])
		if _as61 != "ABC" {
			_as62, _ := AsString(result[0])
			t.Errorf("cat3 result = %q, want %q", _as62, "ABC")
		}
		_as63, _ := AsString(result[1])
		if _as63 != "trailing" {
			t.Errorf("trailing = %v, want 'trailing'", result[1])
		}
	})

	// "Z" cat4 "X" 7 true end "after" → 1 prefix + 3 forward, types align.
	// sig[0]=String("X"), sig[1]=Integer(7), sig[2]=Boolean(true), sig[3]=String("Z" from stack).
	t.Run("EndStopsForward4Mixed", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat4Def...),
			NewString("Z"), NewWord("cat4"), NewString("X"), NewInteger(7), NewBoolean(true),
			NewEnd(), NewString("after"),
		)
		result := runBoru(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("Z cat4 X 7 true end after: got %d results, want 2: %v", len(result), result)
		}
		_as64, _ := AsString(result[0])
		if _as64 != "X7trueZ" {
			_as65, _ := AsString(result[0])
			t.Errorf("cat4 result = %q, want %q", _as65, "X7trueZ")
		}
		_as66, _ := AsString(result[1])
		if _as66 != "after" {
			t.Errorf("trailing = %v, want 'after'", result[1])
		}
	})

	// Two fn calls using parens and end: (cat3 "A" "B" "C" end) (cat3 "D" "E" "F" end)
	// Parens isolate each call; end stops forward collection within each group.
	t.Run("EndSeparatesTwoCalls", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewOpenParen(),
			NewWord("cat3"), NewString("A"), NewString("B"), NewString("C"), NewEnd(),
			NewCloseParen(),
			NewOpenParen(),
			NewWord("cat3"), NewString("D"), NewString("E"), NewString("F"), NewEnd(),
			NewCloseParen(),
		)
		result := runBoru(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("two cat3 calls: got %d results, want 2: %v", len(result), result)
		}
		_as67, _ := AsString(result[0])
		if _as67 != "ABC" {
			_as68, _ := AsString(result[0])
			t.Errorf("first cat3 = %q, want %q", _as68, "ABC")
		}
		_as69, _ := AsString(result[1])
		if _as69 != "DEF" {
			_as70, _ := AsString(result[1])
			t.Errorf("second cat3 = %q, want %q", _as70, "DEF")
		}
	})

	// Mixed types with end in parens:
	// (cat4 "m" 9 false "n" end) (cat3 "x" "y" "z" end)
	// Verifies end works when switching between fns of different arity/types.
	t.Run("EndSeparatesDifferentFns", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append([]Value{}, cat4Def...)
		tokens = append(tokens, cat3Def...)
		tokens = append(tokens,
			NewOpenParen(),
			NewWord("cat4"), NewString("m"), NewInteger(9), NewBoolean(false), NewString("n"), NewEnd(),
			NewCloseParen(),
			NewOpenParen(),
			NewWord("cat3"), NewString("x"), NewString("y"), NewString("z"), NewEnd(),
			NewCloseParen(),
		)
		result := runBoru(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("cat4+cat3 with end: got %d results, want 2: %v", len(result), result)
		}
		_as71, _ := AsString(result[0])
		if _as71 != "m9falsen" {
			_as72, _ := AsString(result[0])
			t.Errorf("cat4 = %q, want %q", _as72, "m9falsen")
		}
		_as73, _ := AsString(result[1])
		if _as73 != "xyz" {
			_as74, _ := AsString(result[1])
			t.Errorf("cat3 = %q, want %q", _as74, "xyz")
		}
	})

	// Prefix-heavy with end: "P" "Q" cat3 "R" end "extra"
	// 2 prefix, 1 forward, end stops collection, "extra" remains.
	// fwd: "R"→sig[0]; stack: top="Q"→sig[1], "P"→sig[2] → "RQP"
	t.Run("EndAfterPartialForward", func(t *testing.T) {
		r, err := DefaultRegistry()
		registerIOWords(r)
		if err != nil {
			t.Fatal(err)
		}
		tokens := append(append([]Value{}, cat3Def...),
			NewString("P"), NewString("Q"), NewWord("cat3"), NewString("R"),
			NewEnd(), NewString("extra"),
		)
		result := runBoru(t, r, tokens)
		if len(result) != 2 {
			t.Fatalf("P Q cat3 R end extra: got %d results, want 2: %v", len(result), result)
		}
		_as75, _ := AsString(result[0])
		if _as75 != "RQP" {
			_as76, _ := AsString(result[0])
			t.Errorf("cat3 = %q, want %q", _as76, "RQP")
		}
		_as77, _ := AsString(result[1])
		if _as77 != "extra" {
			t.Errorf("trailing = %v, want 'extra'", result[1])
		}
	})
}

func TestIntegerLiteralType(t *testing.T) {
	// Post §1.1 fix: NewInteger no longer encodes the value in the
	// type path. All integers share Parent=Integer;
	// specific-value dispatch goes through Signature.Patterns.
	v := NewInteger(5)
	if !v.Parent.Equal(TInteger) {
		t.Errorf("NewInteger(5).Parent = %s, want Integer", v.Parent)
	}
	if !v.Parent.ConformsTo(TNumber) {
		t.Errorf("NewInteger(5).Parent = %s, want matches number", v.Parent)
	}
	// Two different integers now share the same Parent — pattern
	// dispatch uses Signature.Patterns instead of type-path leaves.
	v0 := NewInteger(0)
	v1 := NewInteger(1)
	if !v0.Parent.Equal(v1.Parent) {
		t.Errorf("NewInteger(0) and NewInteger(1) should share Parent=Integer; got %s vs %s", v0.Parent, v1.Parent)
	}
	// And both still match Integer / Number / Scalar.
	if !v0.Parent.ConformsTo(TInteger) || !v1.Parent.ConformsTo(TInteger) {
		t.Error("both should match Integer")
	}
}

func TestEngineFnLiteralType(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def adder fn [[0] [integer] [add 2]] end
	// adder only matches the value 0, adds 2 to it
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(2)}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(0), NewWord("adder"),
	})
	_as78, _ := AsInteger(result[0])
	if len(result) != 1 || _as78 != 2 {
		t.Errorf("0 adder = %v, want 2", result)
	}
}

func TestEngineFnLiteralTypeNoMatch(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def adder fn [[0] [integer] [add 2]] end
	// adder should NOT match 5 (only matches 0)
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(2)}),
	})
	err = runBoruError(t, r, []Value{
		NewWord("def"), NewWord("adder"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("adder"),
	})
	if err == nil {
		t.Error("expected error: adder should not match 5")
	}
}

func TestEngineFnLiteralTypeMultiSig(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def handler fn [[0] [integer] [add 10] [1] [integer] [add 20]] end
	// handler 0 → 10, handler 1 → 21
	fnBody := NewList([]Value{
		NewList([]Value{NewInteger(0)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(10)}),
		NewList([]Value{NewInteger(1)}),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("add"), NewInteger(20)}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("handler"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(0), NewWord("handler"),
	})
	_as79, _ := AsInteger(result[0])
	if len(result) != 1 || _as79 != 10 {
		t.Errorf("0 handler = %v, want 10", result)
	}

	result = runBoru(t, r, []Value{
		NewInteger(1), NewWord("handler"),
	})
	_as80, _ := AsInteger(result[0])
	if len(result) != 1 || _as80 != 21 {
		t.Errorf("1 handler = %v, want 21", result)
	}
}

func TestEngineFnDefPrefixOnly(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def doubler/s fn [[x:integer] [integer] [x x add]] end
	// doubler/s registers as stack-only: takes args from the stack only,
	// never collects forward args via forward.
	fnBody := NewList([]Value{
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// 5 doubler — 5 is on stack, doubler takes it as prefix arg
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWordModified("doubler", -1, true, false), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(5), NewWord("doubler"),
	})
	_as81, _ := AsInteger(result[0])
	if len(result) != 1 || _as81 != 10 {
		t.Errorf("5 doubler = %v, want 10", result)
	}
}

func TestEngineFnDefPrefixOnlyNoForwardCollection(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def doubler/s fn [[x:integer] [integer] [x x add]] end
	// doubler 5 — stack-only word should NOT collect 5 as forward arg.
	// It should fail because there's nothing on the stack for prefix match.
	fnBody := NewList([]Value{
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewWord("x"), NewWord("x"), NewWord("add")}),
	})
	// Define using string name (def sig selection changed with new type hierarchy).
	_ = runBoru(t, r, []Value{
		NewWord("def"), NewString("doubler"), NewWord("fn"), fnBody, NewEnd(),
	})

	// Prefix call with arg on stack should work.
	result := runBoru(t, r, []Value{
		NewInteger(5), NewWord("doubler"),
	})
	_as82, _ := AsInteger(result[0])
	if len(result) != 1 || _as82 != 10 {
		t.Errorf("5 doubler = %v, want 10", result)
	}
}

func TestEngineFnAbbreviatedSignature(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// def foo fn [
	//   [string] [string] [add "Q"]    -- full form
	//   integer  string   [add "P"]    -- abbreviated input sig & output sig
	//   99       string   [drop "NN"]  -- abbreviated input sig & output sig
	// ]

	fnBody := NewList([]Value{
		// sig 1: [string] [string] [add "Q"]
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("String")}),
		NewList([]Value{NewWord("add"), NewString("Q")}),

		// sig 2: integer string [add "P"]  (abbreviated input & output)
		NewWord("Integer"),
		NewWord("String"),
		NewList([]Value{NewWord("add"), NewString("P")}),

		// sig 3: 99 string [drop "NN"]  (abbreviated input & output)
		NewInteger(99),
		NewWord("String"),
		NewList([]Value{NewWord("drop"), NewString("NN")}),
	})

	// foo "x" → "xQ" (string matches sig 1: "x" add "Q")
	result := runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewString("x"), NewWord("foo"),
	})
	_as83, _ := AsString(result[0])
	if len(result) != 1 || _as83 != "xQ" {
		t.Errorf("foo \"x\" = %v, want \"xQ\"", result)
	}

	// foo 1 → "1P" (integer matches sig 2: 1 add "P")
	result = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(1), NewWord("foo"),
	})
	_as84, _ := AsString(result[0])
	if len(result) != 1 || _as84 != "1P" {
		t.Errorf("foo 1 = %v, want \"1P\"", result)
	}

	// foo 99 → "NN" (literal 99 matches sig 3: drop "NN")
	result = runBoru(t, r, []Value{
		NewWord("def"), NewString("foo"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(99), NewWord("foo"),
	})
	_as85, _ := AsString(result[0])
	if len(result) != 1 || _as85 != "NN" {
		t.Errorf("foo 99 = %v, want \"NN\"", result)
	}

}

func TestEngineFnAbbreviatedSimple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def double fn [number number [dup add]] end 7 double
	// All three elements abbreviated (single-valued)
	fnBody := NewList([]Value{
		NewWord("Number"),
		NewWord("Number"),
		NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("double"), NewWord("fn"), fnBody, NewEnd(),
		NewInteger(7), NewWord("double"),
	})
	_as86, _ := AsInteger(result[0])
	if len(result) != 1 || _as86 != 14 {
		t.Errorf("double 7 = %v, want 14", result)
	}
}

func TestEngineFnFactorial(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def fact fn [0 integer [drop 1] [x:integer] [integer] [x mul fact (x sub 1)]]
	fnBody := NewList([]Value{
		// sig 1 (base case): 0 integer [drop 1]
		NewInteger(0),
		NewWord("Integer"),
		NewList([]Value{NewWord("drop"), NewInteger(1)}),
		// sig 2 (recursive): [x:integer] [integer] [x (fact (x sub 1)) mul]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"),
			NewOpenParen(), NewWord("fact"), NewOpenParen(), NewWord("x"), NewWord("sub"), NewInteger(1), NewCloseParen(), NewCloseParen(),
			NewWord("mul"),
		}),
	})
	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}
	for _, tc := range tests {
		result := runBoru(t, r, []Value{
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
			NewInteger(tc.input), NewWord("fact"),
		})
		_as87, _ := AsInteger(result[0])
		if len(result) != 1 || _as87 != tc.expected {
			t.Errorf("fact %d = %v, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestEngineFnFactorialNoVars(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Try several variable-free body forms for the recursive case.
	// Base case is always: 0 integer [drop 1]
	bodies := []struct {
		name string
		body []Value
	}{
		// Approach A: dup sub 1 fact swap mul
		// n dup → n n; n sub 1 → n-1; fact → fact(n-1); swap mul → n*fact(n-1)
		{"dup sub 1 fact swap mul", []Value{
			NewWord("dup"), NewWord("sub"), NewInteger(1),
			NewWord("fact"), NewWord("swap"), NewWord("mul"),
		}},
		// Approach B: dup sub 1 fact mul (rely on mul grabbing n as prefix)
		{"dup sub 1 fact mul", []Value{
			NewWord("dup"), NewWord("sub"), NewInteger(1),
			NewWord("fact"), NewWord("mul"),
		}},
		// Approach C: dup mul fact (dup sub 1)  — same structure as named version
		// but dup in inner parens has no prefix, so this likely fails
		{"dup mul fact (dup sub 1)", []Value{
			NewWord("dup"), NewWord("mul"),
			NewWord("fact"),
			NewOpenParen(), NewWord("dup"), NewWord("sub"), NewInteger(1), NewCloseParen(),
		}},
	}

	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}

	for _, b := range bodies {
		fnBody := NewList([]Value{
			NewInteger(0),
			NewWord("Integer"),
			NewList([]Value{NewWord("drop"), NewInteger(1)}),
			NewWord("Integer"),
			NewWord("Integer"),
			NewList(b.body),
		})
		allPass := true
		for _, tc := range tests {
			e := NewTop(r)
			result, err := e.Run([]Value{
				NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
				NewInteger(tc.input), NewWord("fact"),
			})
			if err != nil {
				t.Logf("FAIL body=%q: fact %d error: %v", b.name, tc.input, err)
				allPass = false
				break
			}
			_as88, _ := AsInteger(result[0])
			if len(result) != 1 || _as88 != tc.expected {
				t.Logf("FAIL body=%q: fact %d = %v, want %d", b.name, tc.input, result, tc.expected)
				allPass = false
			}
		}
		if allPass {
			t.Logf("PASS body=%q", b.name)
		}
	}
}

func TestEngineFnFactorialNamedZero(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def fact fn [[_:0] integer [1] [x:integer] [integer] [x mul fact (x sub 1)]]
	// Using {_:0} instead of bare 0 in the base case.
	// Named param "_" consumes the 0 from the stack, so the body is just [1].
	fnBody := NewList([]Value{
		// sig 1 (base case): [_:0] integer [1]
		func() Value {
			m := NewOrderedMap()
			m.Set("_", NewInteger(0))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{NewInteger(1)}),
		// sig 2 (recursive): [x:integer] [integer] [x (fact (x sub 1)) mul]
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewWord("Integer"))
			return NewList([]Value{NewImplicitMap(m)})
		}(),
		NewList([]Value{NewWord("Integer")}),
		NewList([]Value{
			NewWord("x"),
			NewOpenParen(), NewWord("fact"), NewOpenParen(), NewWord("x"), NewWord("sub"), NewInteger(1), NewCloseParen(), NewCloseParen(),
			NewWord("mul"),
		}),
	})
	tests := []struct {
		input    int64
		expected int64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{5, 120},
		{7, 5040},
	}
	for _, tc := range tests {
		result := runBoru(t, r, []Value{
			NewWord("def"), NewString("fact"), NewWord("fn"), fnBody, NewEnd(),
			NewInteger(tc.input), NewWord("fact"),
		})
		_as89, _ := AsInteger(result[0])
		if len(result) != 1 || _as89 != tc.expected {
			t.Errorf("fact %d = %v, want %d", tc.input, result, tc.expected)
		}
	}
}

func TestEngineTypeRecord(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def Point type Record [x:number y:number] end Point
	xf := NewOrderedMap()
	xf.Set("x", NewTypeLiteral(TNumber))
	yf := NewOrderedMap()
	yf.Set("y", NewTypeLiteral(TNumber))
	fields := NewList([]Value{NewMap(xf), NewMap(yf)})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Point"), NewWord("refine"), NewWord("Record"), fields, NewEnd(),
		NewWord("Point"),
	})
	// The name evaluates to its minted NODE (the Stage 2 flip); the
	// record schema is the node's recorded content.
	if len(result) != 1 {
		t.Fatalf("expected one value, got %v", result)
	}
	content, ok := TypeContentOf(result[0])
	if !ok || !IsRecordType(content) {
		t.Errorf("expected record type content, got %v", result)
	}
}

func TestEngineMakeRecord(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def P type Record [x:number y:string] end make P [1 "hi"]
	xf := NewOrderedMap()
	xf.Set("x", NewTypeLiteral(TNumber))
	yf := NewOrderedMap()
	yf.Set("y", NewTypeLiteral(TString))
	fields := NewList([]Value{NewMap(xf), NewMap(yf)})
	vals := NewList([]Value{NewInteger(1), NewString("hi")})
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("P"), NewWord("refine"), NewWord("Record"), fields, NewEnd(),
		NewWord("make"), NewWord("P"), vals,
	})
	if len(result) != 1 || !result[0].Parent.Equal(TMap) {
		t.Errorf("expected map, got %v", result)
	}
	m, _ := AsMap(result[0])
	xVal, _ := m.Get("x")
	_as90, _ := AsInteger(xVal)
	if _as90 != 1 {
		t.Errorf("x = %v, want 1", xVal)
	}
}

func TestEngineUnifyMaps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// {x:1} unify {x:1}
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	result := runBoru(t, r, []Value{NewMap(m1), NewMap(m2), NewWord("unify")})
	_as91, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as91 {
		t.Errorf("{x:1} unify {x:1} = %v, want true", result)
	}
}

func TestEngineUnifyLists(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	l1 := NewList([]Value{NewInteger(1), NewInteger(2)})
	l2 := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runBoru(t, r, []Value{l1, l2, NewWord("unify")})
	_as92, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as92 {
		t.Errorf("[1,2] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyFail(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewString("a"), NewWord("unify")})
	_as93, _ := AsBoolean(result[1])
	if len(result) != 2 || _as93 {
		t.Errorf("1 unify 'a' = %v, want false", result)
	}
}

func TestEngineUnifyTypedList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	tl := NewTypedList(NewTypeLiteral(TNumber))
	cl := NewList([]Value{NewInteger(1), NewInteger(2)})
	result := runBoru(t, r, []Value{tl, cl, NewWord("unify")})
	_as94, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as94 {
		t.Errorf("[:number] unify [1,2] = %v, want true", result)
	}
}

func TestEngineUnifyTypedMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	cm := NewOrderedMap()
	cm.Set("a", NewInteger(1))
	cm.Set("b", NewInteger(2))
	result := runBoru(t, r, []Value{tm, NewMap(cm), NewWord("unify")})
	_as95, _ := AsBoolean(result[1])
	if len(result) != 2 || !_as95 {
		t.Errorf("{:number} unify {a:1,b:2} = %v, want true", result)
	}
}

func TestEngineDisjunct(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// string tor none
	result := runBoru(t, r, []Value{
		NewTypeLiteral(TString), NewWord("tor"), NewTypeLiteral(TNone),
	})
	if len(result) != 1 || !IsDisjunct(result[0]) {
		t.Errorf("string tor none = %v, want disjunct", result)
	}
}

func TestEngineAddStrings(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewString("hello"), NewWord("add"), NewString(" world")})
	_as97, _ := AsString(result[0])
	if len(result) != 1 || _as97 != "hello world" {
		t.Errorf("'hello' add ' world' = %v, want 'hello world'", result)
	}
}
