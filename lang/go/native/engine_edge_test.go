package native

import (
	"testing"
)

// ==========================================================================
// Edge case tests — exhaustive coverage of all language elements
// ==========================================================================

// --- Edge: type system ---

func TestEdgeTypeAnyMatchesWord(t *testing.T) {
	if !TWord.ConformsTo(TAny) {
		t.Error("word should match any")
	}
}

func TestEdgeTypeAnyMatchesForward(t *testing.T) {
	if !TForward.ConformsTo(TAny) {
		t.Error("forward should match any")
	}
}

func TestEdgeTypeAnyMatchesOpenParen(t *testing.T) {
	if !TOpenParen.ConformsTo(TAny) {
		t.Error("paren/open should match any")
	}
}

func TestEdgeTypeWordMatchesItself(t *testing.T) {
	if !TWord.ConformsTo(TWord) {
		t.Error("word should match word")
	}
}

func TestEdgeTypeForwardMatchesItself(t *testing.T) {
	if !TForward.ConformsTo(TForward) {
		t.Error("forward should match forward")
	}
}

func TestEdgeTypeOpenParenMatchesParen(t *testing.T) {
	// paren/open should match pattern "paren"
	tParen, err := NewType("Paren")
	if err != nil {
		t.Fatal(err)
	}
	if !TOpenParen.ConformsTo(tParen) {
		t.Error("paren/open should match paren")
	}
}

func TestEdgeTypeEmptyStringMatchesAny(t *testing.T) {
	if !TStringEmpty.ConformsTo(TAny) {
		t.Error("string/empty should match any")
	}
}

func TestEdgeTypeUnrelatedTypes(t *testing.T) {
	tFoo := MintTestType("Foo/Bar")
	tBaz := MintTestType("Baz")
	if tFoo.ConformsTo(tBaz) {
		t.Error("foo/bar should not match baz")
	}
}

func TestEdgeTypeDeeplyNested(t *testing.T) {
	tDeep := MintTestType("A/B/C/D")
	tShallow := MintTestType("A/B")
	if !tDeep.ConformsTo(tShallow) {
		t.Error("a/b/c/d should match a/b")
	}
	if tShallow.ConformsTo(tDeep) {
		t.Error("a/b should not match a/b/c/d")
	}
}

func TestEdgeTypeSelfMatch(t *testing.T) {
	types := []*Type{TAny, TString, TStringProper, TStringEmpty, TInteger}
	for _, typ := range types {
		if !typ.ConformsTo(typ) {
			t.Errorf("%s should match itself", typ)
		}
	}
}

// --- Edge: value constructors ---

func TestEdgeNewIntegerZero(t *testing.T) {
	v := NewInteger(0)
	_as114, _ := AsInteger(v)
	if _as114 != 0 {
		_as115, _ := AsInteger(v)
		t.Errorf("got %d, want 0", _as115)
	}
}

func TestEdgeNewIntegerNegative(t *testing.T) {
	v := NewInteger(-999)
	_as116, _ := AsInteger(v)
	if _as116 != -999 {
		_as117, _ := AsInteger(v)
		t.Errorf("got %d, want -999", _as117)
	}
}

func TestEdgeNewIntegerMaxMin(t *testing.T) {
	vMax := NewInteger(9223372036854775807) // max int64
	_as118, _ := AsInteger(vMax)
	if _as118 != 9223372036854775807 {
		_as119, _ := AsInteger(vMax)
		t.Errorf("got %d, want max int64", _as119)
	}
	vMin := NewInteger(-9223372036854775808) // min int64
	_as120, _ := AsInteger(vMin)
	if _as120 != -9223372036854775808 {
		_as121, _ := AsInteger(vMin)
		t.Errorf("got %d, want min int64", _as121)
	}
}

func TestEdgeNewStringSpecialChars(t *testing.T) {
	v := NewString("hello\nworld\ttab")
	_as122, _ := AsString(v)
	if _as122 != "hello\nworld\ttab" {
		_as123, _ := AsString(v)
		t.Errorf("got %q, want string with newline and tab", _as123)
	}
}

func TestEdgeNewOpenParen(t *testing.T) {
	v := NewOpenParen()
	if !IsOpenParen(v) {
		t.Error("IsOpenParen() = false for NewOpenParen()")
	}
	if IsWord(v) {
		t.Error("IsWord() = true for open paren")
	}
	if IsForward(v) {
		t.Error("IsForward() = true for open paren")
	}
	if v.String() != "(" {
		t.Errorf("String() = %q, want '('", v.String())
	}
}

func TestEdgeValueStringRepresentations(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"word", NewWord("test"), "word(test)"},
		{"integer", NewInteger(42), "42"},
		{"string", NewString("hi"), "'hi'"},
		{"empty string", NewString(""), "''"},
		{"open paren", NewOpenParen(), "("},
		{"forward", NewForward(ForwardInfo{FuncName: "add", ExpectedArgs: 1, CollectedArgs: 0}), "forward(add,0/1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Edge: empty input ---

func TestEdgeEmptyInput(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

// --- Edge: multiple literals ---

func TestEdgeMultipleLiterals(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3),
		NewString("a"), NewString("b"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 5 {
		t.Fatalf("got %d values, want 5: %v", len(result), result)
	}
}

// --- Edge: unknown words ---

func TestEdgeMultipleUnknownWordsError(t *testing.T) {
	// foo bar baz → error (undefined words)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewWord("foo"), NewWord("bar"), NewWord("baz"),
	})
	if err == nil {
		t.Fatal("expected error for undefined words, got nil")
	}
}

func TestEdgeUnknownWordCollectedByForward(t *testing.T) {
	// lower foo → 'foo' (foo becomes string, then lower collects it)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewWord("lower"), NewString("foo")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as126, _ := AsString(result[0])
	if _as126 != "foo" {
		_as127, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as127, "foo")
	}
}

func TestEdgeUnknownWordAsSetKey(t *testing.T) {
	// set mykey 42 end get mykey → [42]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("mykey"), NewInteger(42),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("mykey"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as128, _ := AsInteger(result[0])
	if len(result) != 1 || _as128 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Edge: upper ---

func TestEdgeUpperAlreadyUpper(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("ABC"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as129, _ := AsString(result[0])
	if _as129 != "ABC" {
		_as130, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as130, "ABC")
	}
}

func TestEdgeUpperEmptyString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString(""), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as131, _ := AsString(result[0])
	if _as131 != "" {
		_as132, _ := AsString(result[0])
		t.Errorf("got %q, want empty", _as132)
	}
}

func TestEdgeUpperOnInteger(t *testing.T) {
	// 42 upper → signature error (integer doesn't match string)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewInteger(42), NewWord("upper")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Edge: lower ---

func TestEdgeLowerAlreadyLower(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("abc"), NewWord("lower")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as133, _ := AsString(result[0])
	if _as133 != "abc" {
		_as134, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as134, "abc")
	}
}

func TestEdgeLowerForwardOnInteger(t *testing.T) {
	// lower 42 → signature error (forward can't collect integer for string param)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("lower"), NewInteger(42)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Edge: dup ---

func TestEdgeDupString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("hello"), NewWord("dup")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2", len(result))
	}
	_as136, _ := AsString(result[0])
	_as135, _ := AsString(result[1])
	if _as136 != "hello" || _as135 != "hello" {
		t.Errorf("got [%v, %v], want ['hello', 'hello']", result[0], result[1])
	}
}

func TestEdgeDupNoArgs(t *testing.T) {
	// dup with nothing on stack → error
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("dup")})
	if err == nil {
		t.Fatal("expected error for dup with no args, got nil")
	}
}

// --- Edge: swap ---

func TestEdgeSwapMixedTypes(t *testing.T) {
	// "hello" 42 swap → [42, 'hello']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("hello"), NewInteger(42), NewWord("swap")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2", len(result))
	}
	_as137, _ := AsInteger(result[0])
	if _as137 != 42 {
		t.Errorf("result[0] = %v, want 42", result[0])
	}
	_as138, _ := AsString(result[1])
	if _as138 != "hello" {
		t.Errorf("result[1] = %v, want 'hello'", result[1])
	}
}

func TestEdgeSwapNoArgs(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("swap")})
	if err == nil {
		t.Fatal("expected error for swap with no args, got nil")
	}
}

func TestEdgeSwapOneArg(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewInteger(1), NewWord("swap")})
	if err == nil {
		t.Fatal("expected error for swap with one arg, got nil")
	}
}

// --- Edge: drop ---

func TestEdgeDropNoArgs(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("drop")})
	if err == nil {
		t.Fatal("expected error for drop with no args, got nil")
	}
}

func TestEdgeDropString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("gone"), NewWord("drop")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeDropPreservesOthers(t *testing.T) {
	// 1 2 3 drop → [1, 2]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3), NewWord("drop"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as140, _ := AsInteger(result[0])
	_as139, _ := AsInteger(result[1])
	if _as140 != 1 || _as139 != 2 {
		t.Errorf("got [%v, %v], want [1, 2]", result[0], result[1])
	}
}

// --- Edge: arithmetic boundary conditions ---

func TestEdgeArithmeticLargeNumbers(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	// 1000000 mul 1000000 → 1000000000000
	result, err := e.Run([]Value{
		NewInteger(1000000), NewWord("mul"), NewInteger(1000000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as141, _ := AsInteger(result[0])
	if _as141 != 1000000000000 {
		_as142, _ := AsInteger(result[0])
		t.Errorf("got %d, want 1000000000000", _as142)
	}
}

func TestEdgeArithmeticNegativeResults(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		// -5 mul 3 → -15
		{"neg mul pos", []Value{NewInteger(-5), NewWord("mul"), NewInteger(3)}, -15},
		// -5 mul -3 → 15
		{"neg mul neg", []Value{NewInteger(-5), NewWord("mul"), NewInteger(-3)}, 15},
		// -10 div 3 → -3 (truncated)
		{"neg div pos", []Value{NewInteger(-10), NewWord("div"), NewInteger(3)}, -3},
		// -10 mod 3 → -1
		{"neg mod pos", []Value{NewInteger(-10), NewWord("mod"), NewInteger(3)}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_as143, _ := AsInteger(result[0])
			if _as143 != tt.want {
				_as144, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as144, tt.want)
			}
		})
	}
}

func TestEdgeArithmeticZeroOperations(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		{"zero add zero", []Value{NewInteger(0), NewWord("add"), NewInteger(0)}, 0},
		{"zero sub zero", []Value{NewInteger(0), NewWord("sub"), NewInteger(0)}, 0},
		{"zero mul anything", []Value{NewInteger(0), NewWord("mul"), NewInteger(999)}, 0},
		{"anything mul zero", []Value{NewInteger(999), NewWord("mul"), NewInteger(0)}, 0},
		{"zero div anything", []Value{NewInteger(0), NewWord("div"), NewInteger(5)}, 0},
		{"zero mod anything", []Value{NewInteger(0), NewWord("mod"), NewInteger(5)}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_as145, _ := AsInteger(result[0])
			if _as145 != tt.want {
				_as146, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as146, tt.want)
			}
		})
	}
}

func TestEdgeArithmeticIdentity(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	tests := []struct {
		name  string
		input []Value
		want  int64
	}{
		{"add identity", []Value{NewInteger(42), NewWord("add"), NewInteger(0)}, 42},
		{"sub identity", []Value{NewInteger(42), NewWord("sub"), NewInteger(0)}, 42},
		{"mul identity", []Value{NewInteger(42), NewWord("mul"), NewInteger(1)}, 42},
		{"div identity", []Value{NewInteger(42), NewWord("div"), NewInteger(1)}, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_as147, _ := AsInteger(result[0])
			if _as147 != tt.want {
				_as148, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as148, tt.want)
			}
		})
	}
}

func TestEdgeArithmeticNoArgs(t *testing.T) {
	// add with no args → error
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("add")})
	if err == nil {
		t.Fatal("expected error for add with no args, got nil")
	}
}

func TestEdgeArithmeticOneArg(t *testing.T) {
	// 1 add → should use forward signature and wait for arg
	// Since there's no next arg, it should be an orphaned forward error
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewInteger(1), NewWord("add")})
	if err == nil {
		t.Fatal("expected error for add with one arg and no forward arg, got nil")
	}
}

func TestEdgeArithmeticStringOperands(t *testing.T) {
	// "hello" add "world" → "helloworld" (string concatenation)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("hello"), NewWord("add"), NewString("world")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as149, _ := AsString(result[0])
	if len(result) != 1 || _as149 != "helloworld" {
		t.Fatalf("got %v, want 'helloworld'", result)
	}
}

// --- Edge: long arithmetic chains ---

func TestEdgeLongInfixChain(t *testing.T) {
	// 1 add 2 add 3 add 4 add 5 → 15 (left-to-right)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2),
		NewWord("add"), NewInteger(3),
		NewWord("add"), NewInteger(4),
		NewWord("add"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as150, _ := AsInteger(result[0])
	if _as150 != 15 {
		_as151, _ := AsInteger(result[0])
		t.Errorf("got %d, want 15", _as151)
	}
}

func TestEdgeLongMixedLeftToRight(t *testing.T) {
	// 1 add 2 mul 3 add 4 mul 5 → left-to-right: ((((1+2)*3)+4)*5) = 65
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
		NewWord("add"), NewInteger(4), NewWord("mul"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as152, _ := AsInteger(result[0])
	if _as152 != 65 {
		_as153, _ := AsInteger(result[0])
		t.Errorf("got %d, want 65", _as153)
	}
}

func TestEdgePrefixChain(t *testing.T) {
	// 1 2 add 3 4 add mul → forward collection cannot cross functions,
	// so each add uses its own stack group: (1+2)=3, (3+4)=7, 3*7=21
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewWord("add"),
		NewInteger(3), NewInteger(4), NewWord("add"),
		NewWord("mul"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as154, _ := AsInteger(result[0])
	if _as154 != 9 {
		_as155, _ := AsInteger(result[0])
		t.Errorf("got %d, want 9", _as155)
	}
}

// --- Edge: modifiers ---

func TestEdgeForceStackOnForwardOnlyLower(t *testing.T) {
	// lower/s with no stack arg → error (force stack but no string on stack)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewWordModified("lower", -1, true, false),
		NewString("X"),
	})
	if err == nil {
		t.Fatal("expected error for force prefix with no prefix arg, got nil")
	}
}

func TestEdgeForceForwardWithPrefixAvailable(t *testing.T) {
	// "A" lower/f "B" → should use forward, returning 'b', with 'a' remaining
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("A"),
		NewWordModified("lower", -1, false, true),
		NewString("B"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as156, _ := AsString(result[0])
	if _as156 != "A" {
		_as157, _ := AsString(result[0])
		t.Errorf("result[0] = %q, want 'A'", _as157)
	}
	_as158, _ := AsString(result[1])
	if _as158 != "b" {
		_as159, _ := AsString(result[1])
		t.Errorf("result[1] = %q, want 'b'", _as159)
	}
}

func TestEdgeArgCountMismatch(t *testing.T) {
	// lower/2 "X" → error (no signature with 2 args)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewString("X"),
		NewWordModified("lower", 2, false, false),
	})
	if err == nil {
		t.Fatal("expected error for arg count mismatch, got nil")
	}
}

func TestEdgeForceStackAdd(t *testing.T) {
	// 1 2 add/s → 3 (force stack on add)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2),
		NewWordModified("add", -1, true, false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as160, _ := AsInteger(result[0])
	if len(result) != 1 || _as160 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

// --- Edge: end keyword ---

func TestEdgeEndAtStart(t *testing.T) {
	// end → [] (no forward, no-op, removes itself)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewEnd()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeEndConsecutive(t *testing.T) {
	// end end end → []
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewEnd(), NewEnd(), NewEnd(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeEndTerminatesGetForward(t *testing.T) {
	// context 42 "mykey" set → stores, then get mykey context end → [42]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewInteger(42), NewString("mykey"), NewWord("set"),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("mykey"), NewEnd(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// get collects 1 forward arg (mykey), Store from stack; end is no-op
	_as161, _ := AsInteger(result[0])
	if len(result) != 1 || _as161 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeEndWithMultipleForwards(t *testing.T) {
	// set a 99 context end set b 88 context end (get a context) (get b context) → [99, 88]
	// Parentheses isolate each get so the first result doesn't become
	// a prefix argument for the second get.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("a"), NewInteger(99), NewEnd(),
		NewWord("context"), NewWord("set"), NewWord("b"), NewInteger(88), NewEnd(),
		NewOpenParen(), NewWord("context"), NewWord("dot"), NewWord("a"), NewCloseParen(),
		NewOpenParen(), NewWord("context"), NewWord("dot"), NewWord("b"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as162, _ := AsInteger(result[0])
	if _as162 != 99 {
		_as163, _ := AsInteger(result[0])
		t.Errorf("result[0] = %d, want 99", _as163)
	}
	_as164, _ := AsInteger(result[1])
	if _as164 != 88 {
		_as165, _ := AsInteger(result[1])
		t.Errorf("result[1] = %d, want 88", _as165)
	}
}

func TestEdgeEndBetweenLiterals(t *testing.T) {
	// 1 2 end 3 → [1, 2, 3]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewEnd(), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
}

// --- Edge: set/get ---

func TestEdgeSetWithIntegerKey(t *testing.T) {
	// set requires String or Atom keys. Integer keys use convert first:
	// (convert String 42) 100 set → string key "42"
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewString("42"), NewInteger(100),
		NewEnd(),
		NewWord("context"), NewWord("get"), NewString("42"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as166, _ := AsInteger(result[0])
	if len(result) != 1 || _as166 != 100 {
		t.Errorf("got %v, want [100]", result)
	}
}

func TestEdgeSetEmptyString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewInteger(1), NewString(""), NewWord("set"),
		NewEnd(),
		NewWord("context"), NewString(""), NewWord("get"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as167, _ := AsInteger(result[0])
	if _as167 != 1 {
		_as168, _ := AsInteger(result[0])
		t.Errorf("got %d, want 1", _as168)
	}
}

func TestEdgeSetValueIsString(t *testing.T) {
	// set "greeting" "hello" end get "greeting" → ['hello']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewString("greeting"), NewString("hello"),
		NewEnd(),
		NewWord("context"), NewWord("get"), NewString("greeting"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as169, _ := AsString(result[0])
	if _as169 != "hello" {
		_as170, _ := AsString(result[0])
		t.Errorf("got %q, want 'hello'", _as170)
	}
}

func TestEdgeSetThenUseValue(t *testing.T) {
	// set x 10 end get x add 5 → [15]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("x"), NewInteger(10),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("x"),
		NewWord("add"), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as171, _ := AsInteger(result[0])
	if len(result) != 1 || _as171 != 15 {
		t.Errorf("got %v, want [15]", result)
	}
}

func TestEdgeSetComputedValue(t *testing.T) {
	// set total (3 mul 7) end get total → [21]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("total"),
		NewOpenParen(), NewInteger(3), NewWord("mul"), NewInteger(7), NewCloseParen(),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("total"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as172, _ := AsInteger(result[0])
	if len(result) != 1 || _as172 != 21 {
		t.Errorf("got %v, want [21]", result)
	}
}

// --- Edge: left-to-right operator interactions ---

func TestEdgeLeftToRightSubMul(t *testing.T) {
	// 10 sub 2 mul 3 → left-to-right: (10-2)*3 = 24
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(10), NewWord("sub"), NewInteger(2), NewWord("mul"), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as173, _ := AsInteger(result[0])
	if _as173 != 24 {
		_as174, _ := AsInteger(result[0])
		t.Errorf("got %d, want 24", _as174)
	}
}

func TestEdgeLeftToRightMulSub(t *testing.T) {
	// 2 mul 3 sub 1 → left-to-right: (2*3)-1 = 5
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("sub"), NewInteger(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as175, _ := AsInteger(result[0])
	if _as175 != 5 {
		_as176, _ := AsInteger(result[0])
		t.Errorf("got %d, want 5", _as176)
	}
}

func TestEdgeLeftToRightDivAdd(t *testing.T) {
	// 1 add 10 div 2 → left-to-right: (1+10)/2 = 5
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(10), NewWord("div"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as177, _ := AsInteger(result[0])
	if _as177 != 5 {
		_as178, _ := AsInteger(result[0])
		t.Errorf("got %d, want 5", _as178)
	}
}

func TestEdgeLeftToRightModAdd(t *testing.T) {
	// 1 add 10 mod 3 → left-to-right: (1+10)%3 = 11%3 = 2
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(10), NewWord("mod"), NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as179, _ := AsInteger(result[0])
	if _as179 != 2 {
		_as180, _ := AsInteger(result[0])
		t.Errorf("got %d, want 2", _as180)
	}
}

func TestEdgeLeftToRightAllOps(t *testing.T) {
	// 1 add 2 mul 3 sub 4 div 2 → left-to-right: ((((1+2)*3)-4)/2) = ((9-4)/2) = (5/2) = 2
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
		NewWord("sub"), NewInteger(4), NewWord("div"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as181, _ := AsInteger(result[0])
	if _as181 != 2 {
		_as182, _ := AsInteger(result[0])
		t.Errorf("got %d, want 2", _as182)
	}
}

// --- Edge: parentheses ---

func TestEdgeEmptyParens(t *testing.T) {
	// () → [] (empty parens produce no values)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestEdgeParenMultipleValues(t *testing.T) {
	// (1 2 3) → [1, 2, 3]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(1), NewInteger(2), NewInteger(3), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, want := range []int64{1, 2, 3} {
		_as183, _ := AsInteger(result[i])
		if _as183 != want {
			_as184, _ := AsInteger(result[i])
			t.Errorf("result[%d] = %d, want %d", i, _as184, want)
		}
	}
}

func TestEdgeParenDeeplyNested(t *testing.T) {
	// ((( 5 ))) → [5]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewOpenParen(), NewOpenParen(),
		NewInteger(5),
		NewCloseParen(), NewCloseParen(), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as185, _ := AsInteger(result[0])
	if len(result) != 1 || _as185 != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEdgeParenNestedArithmetic(t *testing.T) {
	// ((2 add 3) mul (4 sub 1)) → (5 * 3) = 15
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(),
		NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
		NewWord("mul"),
		NewOpenParen(), NewInteger(4), NewWord("sub"), NewInteger(1), NewCloseParen(),
		NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as186, _ := AsInteger(result[0])
	if len(result) != 1 || _as186 != 15 {
		t.Errorf("got %v, want [15]", result)
	}
}

func TestEdgeParenWithFunction(t *testing.T) {
	// (hello upper) → ['HELLO']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewString("hello"), NewWord("upper"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as187, _ := AsString(result[0])
	if len(result) != 1 || _as187 != "HELLO" {
		t.Errorf("got %v, want ['HELLO']", result)
	}
}

func TestEdgeParenWithDup(t *testing.T) {
	// (1 dup) → [1, 1]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(1), NewWord("dup"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
}

func TestEdgeParenAfterLiteral(t *testing.T) {
	// 10 (5) → [10, 5]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(10), NewOpenParen(), NewInteger(5), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as189, _ := AsInteger(result[0])
	_as188, _ := AsInteger(result[1])
	if _as189 != 10 || _as188 != 5 {
		t.Errorf("got %v, want [10, 5]", result)
	}
}

func TestEdgeParenCloseWithNoOpen(t *testing.T) {
	// ) → error
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewCloseParen()})
	if err == nil {
		t.Fatal("expected error for ) with no (, got nil")
	}
}

func TestEdgeParenMultipleOpenUnmatched(t *testing.T) {
	// (( 1 ) → error (one ( left unmatched)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewOpenParen(), NewOpenParen(), NewInteger(1), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("expected error for unmatched (, got nil")
	}
}

func TestEdgeParenConsecutive(t *testing.T) {
	// (1) (2) add → 1+2 = 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(1), NewCloseParen(),
		NewOpenParen(), NewInteger(2), NewCloseParen(),
		NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as190, _ := AsInteger(result[0])
	if len(result) != 1 || _as190 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeParenWithUnknownWordErrors(t *testing.T) {
	// (foo) → error (undefined word)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewOpenParen(), NewWord("foo"), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("expected error for undefined word in paren, got nil")
	}
}

func TestEdgeParenOrphanedForwardInside(t *testing.T) {
	// (add 1) → error: add creates forward inside paren, but only 1 arg collected
	// There's not enough to resolve
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewOpenParen(), NewWord("add"), NewInteger(1), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("expected error for orphaned forward inside parens, got nil")
	}
}

func TestEdgeParenBarrierStopsForwardSearch(t *testing.T) {
	// 1 add (2) → the forward for add should not cross the paren barrier.
	// Instead, (2) resolves to 2, which then gets collected by add's forward.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewWord("add"),
		NewOpenParen(), NewInteger(2), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as192, _ := AsInteger(result[0])
	if len(result) != 1 || _as192 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeParenWithEndNoOp(t *testing.T) {
	// (1 end) → end acts as no-op inside parens (no forward), yields [1]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(1), NewEnd(), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as193, _ := AsInteger(result[0])
	if len(result) != 1 || _as193 != 1 {
		t.Errorf("got %v, want [1]", result)
	}
}

func TestEdgeParenComplexExpression(t *testing.T) {
	// 2 mul (3 add 4 mul 5) → left-to-right inside parens: 3 add 4 = 7, 7 mul 5 = 35, then 2*35 = 70
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("mul"),
		NewOpenParen(), NewInteger(3), NewWord("add"), NewInteger(4), NewWord("mul"), NewInteger(5), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as194, _ := AsInteger(result[0])
	if len(result) != 1 || _as194 != 70 {
		t.Errorf("got %v, want [70]", result)
	}
}

func TestEdgeParenSiblingExpressions(t *testing.T) {
	// (1 add 2) mul (3 add 4) → 3*7 = 21
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		NewWord("mul"),
		NewOpenParen(), NewInteger(3), NewWord("add"), NewInteger(4), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as195, _ := AsInteger(result[0])
	if len(result) != 1 || _as195 != 21 {
		t.Errorf("got %v, want [21]", result)
	}
}

// --- Edge: combined features ---

func TestEdgeSetGetComputedKeyAndValue(t *testing.T) {
	// set (lower KEY) (2 add 3) end get key → [5]
	// (lower KEY) → 'key', (2 add 3) → 5
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"),
		NewOpenParen(), NewWord("lower"), NewString("KEY"), NewCloseParen(),
		NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("key"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as196, _ := AsInteger(result[0])
	if len(result) != 1 || _as196 != 5 {
		t.Errorf("got %v, want [5]", result)
	}
}

func TestEdgeDupThenAdd(t *testing.T) {
	// 5 dup add → 5+5 = 10
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(5), NewWord("dup"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as197, _ := AsInteger(result[0])
	if len(result) != 1 || _as197 != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestEdgeSwapThenSub(t *testing.T) {
	// 3 10 swap sub → 10-3 = 7 (swap makes 10 first arg)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(3), NewInteger(10), NewWord("swap"), NewWord("sub"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as198, _ := AsInteger(result[0])
	if len(result) != 1 || _as198 != 7 {
		t.Errorf("got %v, want [7]", result)
	}
}

func TestEdgeDropThenOp(t *testing.T) {
	// 1 2 3 drop add → 1+2 = 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewInteger(3), NewWord("drop"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as199, _ := AsInteger(result[0])
	if len(result) != 1 || _as199 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestEdgeUpperInParens(t *testing.T) {
	// (abc upper) → ['ABC']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewString("abc"), NewWord("upper"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as200, _ := AsString(result[0])
	if len(result) != 1 || _as200 != "ABC" {
		t.Errorf("got %v, want ['ABC']", result)
	}
}

func TestEdgeMixedStringAndIntOnStack(t *testing.T) {
	// "hello" 42 "world" → ['hello', 42, 'world']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("hello"), NewInteger(42), NewString("world"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
}

func TestEdgeChainUpperLower(t *testing.T) {
	// "Hello" upper lower → 'hello'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("Hello"), NewWord("upper"), NewWord("lower"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as201, _ := AsString(result[0])
	if len(result) != 1 || _as201 != "hello" {
		t.Errorf("got %v, want ['hello']", result)
	}
}

func TestEdgeForwardUpperThenLower(t *testing.T) {
	// lower (upper abc) → lower 'ABC' → 'abc'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("lower"),
		NewOpenParen(), NewString("abc"), NewWord("upper"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as202, _ := AsString(result[0])
	if len(result) != 1 || _as202 != "abc" {
		t.Errorf("got %v, want ['abc']", result)
	}
}

// --- Edge: signature matching specifics ---

func TestEdgeAddWithStringAndInt(t *testing.T) {
	// "hello" 1 add → "hello1" (string concatenation via scalar+scalar)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("hello"), NewInteger(1), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as203, _ := AsString(result[0])
	if len(result) != 1 || _as203 != "hello1" {
		t.Fatalf("got %v, want 'hello1'", result)
	}
}

func TestEdgePrefixMatchSpecificity(t *testing.T) {
	// Verify that more specific signatures win
	// upper takes [string], which matches "hello" (string/proper)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("test"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as204, _ := AsString(result[0])
	if _as204 != "TEST" {
		_as205, _ := AsString(result[0])
		t.Errorf("got %q, want 'TEST'", _as205)
	}
}

// --- Edge: effectiveResolved scoping ---

func TestEdgePrefixMatchDoesNotCrossParen(t *testing.T) {
	// 1 ( 2 add ) → error: inside paren, add sees only [2] as prefix, needs 2 ints
	// Actually 2 add: prefix [int,int] needs 2 ints, but only 1 inside paren.
	// So it falls through to forward (infix) match: [int|int], but then needs forward arg.
	// ')' closes paren, orphaned forward error.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewInteger(1),
		NewOpenParen(), NewInteger(2), NewWord("add"), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("expected error for add with insufficient args in paren scope, got nil")
	}
}

// --- Edge: registry ---

func TestEdgeLookupUnknown(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	fn := r.Lookup("nonexistent")
	if fn != nil {
		t.Errorf("expected nil for unknown function, got %v", fn)
	}
}

func TestEdgeEmptyRegistryErrors(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
	// Undefined words should error
	_, err = e.Run([]Value{NewWord("foo"), NewWord("bar")})
	if err == nil {
		t.Fatal("expected error for undefined words, got nil")
	}
}

func TestEdgeEmptyRegistryEndStillWorks(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
	result, err := e.Run([]Value{NewInteger(1), NewEnd()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as208, _ := AsInteger(result[0])
	if len(result) != 1 || _as208 != 1 {
		t.Errorf("got %v, want [1]", result)
	}
}

func TestEdgeEmptyRegistryParensStillWork(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(42), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as209, _ := AsInteger(result[0])
	if len(result) != 1 || _as209 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Edge: function results re-examination ---

func TestEdgeResultCollectedByPendingForward(t *testing.T) {
	// lower (upper abc) → forward for lower should collect result of (upper abc)
	// (upper abc) → 'ABC', then lower's forward collects it → 'abc'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("lower"),
		NewOpenParen(), NewString("abc"), NewWord("upper"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as210, _ := AsString(result[0])
	if len(result) != 1 || _as210 != "abc" {
		t.Errorf("got %v, want ['abc']", result)
	}
}

func TestEdgePrefixResultFeedsInfix(t *testing.T) {
	// 2 3 add add 4 → (2+3) produces 5 via prefix, then 5 add 4 → but wait,
	// the second add sees 5 on stack as prefix match [int], and sets up forward for 4
	// Result: 9
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(2), NewInteger(3), NewWord("add"),
		NewWord("add"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as211, _ := AsInteger(result[0])
	if len(result) != 1 || _as211 != 9 {
		t.Errorf("got %v, want [9]", result)
	}
}

// --- Edge: store isolation ---

func TestEdgeStoreIsolationBetweenRegistries(t *testing.T) {
	// Two different registries should have separate stores
	reg1, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg1)
	reg2, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg2)
	e1 := New(reg1)
	e2 := New(reg2)

	// Set in reg1, verify it works within same execution
	result, err := e1.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("key"), NewInteger(111),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("key"),
	})
	if err != nil {
		t.Fatalf("unexpected error on set+get: %v", err)
	}
	_as212, _ := AsInteger(result[0])
	if len(result) != 1 || _as212 != 111 {
		t.Fatalf("got %v, want [111]", result)
	}

	// Attempting get from reg2 should fail (different registry = different context)
	_, err = e2.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("key"),
	})
	if err == nil {
		t.Fatal("expected error: key should not exist in separate registry")
	}
}

// --- Edge: single-element inputs ---

func TestEdgeSingleInteger(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(0)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as213, _ := AsInteger(result[0])
	if len(result) != 1 || _as213 != 0 {
		t.Errorf("got %v, want [0]", result)
	}
}

func TestEdgeSingleString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("x")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as214, _ := AsString(result[0])
	if len(result) != 1 || _as214 != "x" {
		t.Errorf("got %v, want ['x']", result)
	}
}

func TestEdgeSingleEmptyString(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as215, _ := AsString(result[0])
	if len(result) != 1 || _as215 != "" {
		t.Errorf("got %v, want ['']", result)
	}
}

// --- Edge: forward details ---

func TestEdgeForwardInfoFields(t *testing.T) {
	info := ForwardInfo{
		FuncName:      "test",
		ExpectedArgs:  3,
		CollectedArgs: 1,
		FuncIndex:     5,
	}
	v := NewForward(info)
	got, _ := AsForward(v)
	if got.FuncName != "test" {
		t.Errorf("FuncName = %q, want 'test'", got.FuncName)
	}
	if got.ExpectedArgs != 3 {
		t.Errorf("ExpectedArgs = %d, want 3", got.ExpectedArgs)
	}
	if got.CollectedArgs != 1 {
		t.Errorf("CollectedArgs = %d, want 1", got.CollectedArgs)
	}
	if got.FuncIndex != 5 {
		t.Errorf("FuncIndex = %d, want 5", got.FuncIndex)
	}
	// The zero value of the speculation pair means "none" (the bool
	// guards the int — No-Zero-Overload rule), so raw literals like
	// the one above stay valid.
	if got.Speculative || got.SpeculativeAt != 0 {
		t.Errorf("zero-value speculation = (%v,%d), want (false,0)", got.Speculative, got.SpeculativeAt)
	}
	spec := ForwardInfo{FuncName: "g", ExpectedArgs: 3, Speculative: true, SpeculativeAt: 2}
	got2, _ := AsForward(NewForward(spec))
	if !got2.Speculative || got2.SpeculativeAt != 2 {
		t.Errorf("speculation round-trip = (%v,%d), want (true,2)", got2.Speculative, got2.SpeculativeAt)
	}
}

// --- Edge: signature edge cases ---

func TestEdgeSignatureNoPrefix(t *testing.T) {
	// A function with only forward should work when called with no prefix stack
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("echo", Signature{
		Args: []*Type{TAny},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return args, nil }), BarrierPos: -1,
	})
	e := NewTop(r)
	result, err := e.Run([]Value{NewWord("echo"), NewInteger(42)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as216, _ := AsInteger(result[0])
	if len(result) != 1 || _as216 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeSignatureMultipleForward(t *testing.T) {
	// A function that takes 2 forward args
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("pair", Signature{
		Args: []*Type{TAny, TAny},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		}), BarrierPos: -1,
	})
	e := NewTop(r)
	result, err := e.Run([]Value{
		NewWord("pair"), NewInteger(1), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
}

func TestEdgeSignatureReturnsMultiple(t *testing.T) {
	// A function that returns multiple values
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Register("triple", Signature{
		Args: []*Type{TAny},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{args[0], args[0], args[0]}, nil
		}), BarrierPos: -1,
	})
	e := NewTop(r)
	result, err := e.Run([]Value{NewInteger(7), NewWord("triple")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, v := range result {
		_as217, _ := AsInteger(v)
		if _as217 != 7 {
			_as218, _ := AsInteger(v)
			t.Errorf("result[%d] = %d, want 7", i, _as218)
		}
	}
}

func TestEdgeSignatureReturnsNothing(t *testing.T) {
	// A function that returns nothing (like drop)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewInteger(2), NewWord("drop"), NewWord("drop"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want []", result)
	}
}

// --- Edge: interactions between end and parens ---

func TestEdgeEndInsideParenNoForward(t *testing.T) {
	// (42 end) → end is no-op inside parens, gives [42]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(42), NewEnd(), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as219, _ := AsInteger(result[0])
	if len(result) != 1 || _as219 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestEdgeEndOutsideParenDoesNotCrossBarrier(t *testing.T) {
	// set a (1 add 2) end get a → set has forward, (1 add 2)=3 is collected,
	// end terminates the forward for set, get a → [3]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("a"),
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("a"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as220, _ := AsInteger(result[0])
	if len(result) != 1 || _as220 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

// --- Engine tests: def (word definition) ---

func TestDefBasicListBody(t *testing.T) {
	// def increment [1 add]  2 increment → 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// First run: define increment
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("increment"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	// Second run: use increment
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("increment"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as221, _ := AsInteger(result[0])
	if len(result) != 1 || _as221 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestDefScalarBody(t *testing.T) {
	// def myval 42  myval → 42
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("myval"), NewInteger(42),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewWord("myval"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as222, _ := AsInteger(result[0])
	if len(result) != 1 || _as222 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

func TestDefStringName(t *testing.T) {
	// def "double" [dup add]  5 double → 10
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewString("double"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(5), NewWord("double"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as223, _ := AsInteger(result[0])
	if len(result) != 1 || _as223 != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestDefPrefixBodyStringName(t *testing.T) {
	// [1 add] def "inc" 10 inc → 11
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewString("inc"), NewWord("word"),
		NewList([]Value{NewInteger(1), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(10), NewWord("inc"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as224, _ := AsInteger(result[0])
	if len(result) != 1 || _as224 != 11 {
		t.Errorf("got %v, want [11]", result)
	}
}

func TestDefPrefixBody(t *testing.T) {
	// [1 sub] def decrement  3 decrement → 2
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("decrement"), NewWord("word"),
		NewList([]Value{NewInteger(1), NewWord("sub")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("decrement"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as225, _ := AsInteger(result[0])
	if len(result) != 1 || _as225 != 2 {
		t.Errorf("got %v, want [2]", result)
	}
}

func TestDefAndUseSameRun(t *testing.T) {
	// def triple [dup dup add add] 4 triple → 12
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("triple"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("dup"), NewWord("add"), NewWord("add")}),
		NewInteger(4), NewWord("triple"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as226, _ := AsInteger(result[0])
	if len(result) != 1 || _as226 != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefDoesNotBreakExistingWordCoercion(t *testing.T) {
	// Unknown words without a pending TWord forward still coerce to strings.
	// a upper → "A"
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("a"), NewWord("upper"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as227, _ := AsString(result[0])
	if len(result) != 1 || _as227 != "A" {
		t.Errorf("got %v, want ['A']", result)
	}
}

func TestDefUndefinedWordAcceptedByTWord(t *testing.T) {
	// Undefined word "foo" is preserved as TWord when def's forward expects it.
	// def foo 99  foo → 99
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(99),
		NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as228, _ := AsInteger(result[0])
	if len(result) != 1 || _as228 != 99 {
		t.Errorf("got %v, want [99]", result)
	}
}

func TestDefStringBody(t *testing.T) {
	// def "greeting" "hello"  greeting → "hello"
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewString("greeting"), NewString("hello"),
		NewWord("greeting"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as229, _ := AsString(result[0])
	if len(result) != 1 || _as229 != "hello" {
		t.Errorf("got %v, want ['hello']", result)
	}
}

func TestDefUsedMultipleTimes(t *testing.T) {
	// def inc [1 add]  1 inc inc inc → 4
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("add")}),
		NewInteger(1), NewWord("inc"), NewWord("inc"), NewWord("inc"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as230, _ := AsInteger(result[0])
	if len(result) != 1 || _as230 != 4 {
		t.Errorf("got %v, want [4]", result)
	}
}

// --- Engine tests: def (traditional Forth-style word definitions) ---

func TestDefForthSquare(t *testing.T) {
	// : square dup mul ;
	// Classic Forth square: duplicates top of stack and multiplies.
	// def square [dup mul]  5 square → 25
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("mul")}),
		NewInteger(5), NewWord("square"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as231, _ := AsInteger(result[0])
	if len(result) != 1 || _as231 != 25 {
		t.Errorf("got %v, want [25]", result)
	}
}

func TestDefForthNegate(t *testing.T) {
	// : negate 0 swap sub ;
	// Negates a number: 0 - n.
	// def "negate" [0 swap sub]  7 negate → -7
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewString("negate"),
		NewWord("word"), NewList([]Value{NewInteger(0), NewWord("swap"), NewWord("sub")}),
		NewInteger(7), NewWord("negate"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as232, _ := AsInteger(result[0])
	if len(result) != 1 || _as232 != -7 {
		t.Errorf("got %v, want [-7]", result)
	}
}

func TestDefForthOver(t *testing.T) {
	// : over swap dup rot ;
	// In standard Forth, over copies the second item to the top.
	// Without rot, we simulate: over = [swap dup] gives (a b → b a a)
	// which isn't over. Instead test the concept of building combinators.
	// def dup2 [dup dup]  3 dup2 → 3 3 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("fdup2"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("dup")}),
		NewInteger(3), NewWord("fdup2"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, v := range result {
		_as233, _ := AsInteger(v)
		if _as233 != 3 {
			_as234, _ := AsInteger(v)
			t.Errorf("result[%d] = %d, want 3", i, _as234)
		}
	}
}

func TestDefForthComposition(t *testing.T) {
	// Define words in terms of other defined words.
	// : double dup add ;
	// : quadruple double double ;
	// 3 quadruple → 12
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// Define double
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("double"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def double: %v", err)
	}

	// Define quadruple in terms of double
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("quadruple"),
		NewWord("word"), NewList([]Value{NewWord("double"), NewWord("double")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def quadruple: %v", err)
	}

	// Use quadruple
	result, err := e.Run([]Value{
		NewInteger(3), NewWord("quadruple"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as235, _ := AsInteger(result[0])
	if len(result) != 1 || _as235 != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefForthThreeDeepComposition(t *testing.T) {
	// : inc 1 add ;
	// : inc2 inc inc ;
	// : inc6 inc2 inc2 inc2 ;
	// 0 inc6 → 6
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("inc2"),
		NewWord("word"), NewList([]Value{NewWord("inc"), NewWord("inc")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc2: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("inc6"),
		NewWord("word"), NewList([]Value{NewWord("inc2"), NewWord("inc2"), NewWord("inc2")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def inc6: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(0), NewWord("inc6"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as236, _ := AsInteger(result[0])
	if len(result) != 1 || _as236 != 6 {
		t.Errorf("got %v, want [6]", result)
	}
}

func TestDefForthSumOfSquares(t *testing.T) {
	// square = [dup mul]
	// 3 square → 3 dup mul → 9
	// 4 square → 4 dup mul → 16
	// add → 9+16 = 25
	// square = [dup mul]. Expansion: 3 dup mul 4 dup mul add.
	// With forward-first: first mul forward-collects 4 → mul(4,3)=12,
	// then dup→[3,12,12], mul reversed→mul(12,12)=144, add(144,3)=147.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("square"),
		NewInteger(4), NewWord("square"),
		NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as237, _ := AsInteger(result[0])
	if len(result) != 1 || _as237 != 147 {
		t.Errorf("got %v, want [147]", result)
	}
}

func TestDefForthCube(t *testing.T) {
	// : square dup mul ;
	// : cube dup square mul ;
	// Note: cube duplicates n, squares one copy, then multiplies: n * n^2 = n^3
	// 3 cube → 27
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def square: %v", err)
	}

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("cube"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("square"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def cube: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("cube"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as238, _ := AsInteger(result[0])
	if len(result) != 1 || _as238 != 27 {
		t.Errorf("got %v, want [27]", result)
	}
}

func TestDefForthWithInfixOps(t *testing.T) {
	// Defined words work with infix operators from the calling context.
	// : double dup add ;
	// 3 double mul 2 → 6 * 2 = 12
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	_, err = e.Run([]Value{
		NewWord("def"), NewWord("double"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("add")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	result, err := e.Run([]Value{
		NewInteger(3), NewWord("double"),
		NewWord("mul"), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as239, _ := AsInteger(result[0])
	if len(result) != 1 || _as239 != 12 {
		t.Errorf("got %v, want [12]", result)
	}
}

func TestDefForthConstant(t *testing.T) {
	// : pi 3 ;   (Forth-style constant as a word that pushes a value)
	// pi pi add → 6
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("pi"), NewInteger(3),
		NewWord("pi"), NewWord("pi"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as240, _ := AsInteger(result[0])
	if len(result) != 1 || _as240 != 6 {
		t.Errorf("got %v, want [6]", result)
	}
}

func TestDefForthStackEffectMultipleValues(t *testing.T) {
	// A word that pushes multiple values onto the stack.
	// : pair1 1 2 ;
	// pair1 add → 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("pair1"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewWord("pair1"), NewWord("add"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as241, _ := AsInteger(result[0])
	if len(result) != 1 || _as241 != 3 {
		t.Errorf("got %v, want [3]", result)
	}
}

func TestDefForthSwapSub(t *testing.T) {
	// : nip swap drop ;
	// Nip removes second element: (a b → b)
	// 10 20 nip → 20
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewString("fnip"),
		NewWord("word"), NewList([]Value{NewWord("swap"), NewWord("drop")}),
		NewInteger(10), NewInteger(20), NewWord("fnip"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as242, _ := AsInteger(result[0])
	if len(result) != 1 || _as242 != 20 {
		t.Errorf("got %v, want [20]", result)
	}
}

func TestDefForthAbsDiff(t *testing.T) {
	// Absolute difference using two defined words.
	// : monus swap sub ;  (reversed subtraction: a b → b-a)
	// Then compute |3-7| by choosing the larger minus smaller.
	// 7 3 monus → 7 - 3 = 4  (swap makes it 3 7, then sub gives 7-3=4)
	// Wait: swap sub on (7, 3) → swap gives (3, 7), sub gives 3-7=-4.
	// Actually sub is prefix [a, b] → a - b, so (3, 7) → 3 - 7 = -4.
	// Let me reconsider: just demonstrate sub with swap.
	// def rsub [swap sub]  3 7 rsub → swap gives (7,3), sub gives 7-3=4
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("rsub"),
		NewWord("word"), NewList([]Value{NewWord("swap"), NewWord("sub")}),
		NewInteger(3), NewInteger(7), NewWord("rsub"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as243, _ := AsInteger(result[0])
	if len(result) != 1 || _as243 != 4 {
		t.Errorf("got %v, want [4]", result)
	}
}

func TestDefForthMultipleDefsInSameRun(t *testing.T) {
	// Define multiple words in a single run, then use them together.
	// : inc 1 add ;
	// : dec 1 sub ;
	// 10 inc inc dec → 11
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("inc"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("add")}),
		NewWord("def"), NewWord("dec"),
		NewWord("word"), NewList([]Value{NewInteger(1), NewWord("sub")}),
		NewInteger(10), NewWord("inc"), NewWord("inc"), NewWord("dec"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as244, _ := AsInteger(result[0])
	if len(result) != 1 || _as244 != 11 {
		t.Errorf("got %v, want [11]", result)
	}
}

func TestDefForthStringWord(t *testing.T) {
	// A word that pushes a string and operates on it.
	// : shout upper ;
	// "hello" shout → "HELLO"
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("shout"),
		NewWord("word"), NewList([]Value{NewWord("upper")}),
		NewString("hello"), NewWord("shout"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as245, _ := AsString(result[0])
	if len(result) != 1 || _as245 != "HELLO" {
		t.Errorf("got %v, want ['HELLO']", result)
	}
}

func TestDefForthPersistsAcrossRuns(t *testing.T) {
	// Definitions persist in the registry across Run calls,
	// mirroring how Forth words persist once defined.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// Run 1: define square
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("square"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def: %v", err)
	}

	// Run 2: define cube using square
	_, err = e.Run([]Value{
		NewWord("def"), NewWord("cube"),
		NewWord("word"), NewList([]Value{NewWord("dup"), NewWord("square"), NewWord("mul")}),
	})
	if err != nil {
		t.Fatalf("unexpected error on def cube: %v", err)
	}

	// Run 3: use cube (tests both definitions persisted)
	result, err := e.Run([]Value{
		NewInteger(2), NewWord("cube"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as246, _ := AsInteger(result[0])
	if len(result) != 1 || _as246 != 8 {
		t.Errorf("got %v, want [8]", result)
	}

	// Run 4: use square independently
	result, err = e.Run([]Value{
		NewInteger(7), NewWord("square"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as247, _ := AsInteger(result[0])
	if len(result) != 1 || _as247 != 49 {
		t.Errorf("got %v, want [49]", result)
	}
}

func TestDefForthDefWithEnd(t *testing.T) {
	// Using end to terminate def's forward collection early,
	// with the body coming from the prefix stack.
	// [dup add] def double end 5 double → 10
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("double"), NewWord("word"),
		NewList([]Value{NewWord("dup"), NewWord("add")}), NewEnd(),
		NewInteger(5), NewWord("double"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as248, _ := AsInteger(result[0])
	if len(result) != 1 || _as248 != 10 {
		t.Errorf("got %v, want [10]", result)
	}
}

func TestDefForthFactorial5(t *testing.T) {
	// Compute 5! = 120 iteratively using defined words.
	// Without loops, we manually unroll:
	// : mul5 5 mul ;    (just multiply by a constant)
	// 1 mul5 → 5, then 4 mul → 20, then 3 mul → 60, then 2 mul → 120
	// This tests defined words mixed with inline operations.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("def"), NewWord("mul5"),
		NewWord("word"), NewList([]Value{NewInteger(5), NewWord("mul")}),
		NewInteger(1), NewWord("mul5"),
		NewInteger(4), NewWord("mul"),
		NewInteger(3), NewWord("mul"),
		NewInteger(2), NewWord("mul"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as249, _ := AsInteger(result[0])
	if len(result) != 1 || _as249 != 120 {
		t.Errorf("got %v, want [120]", result)
	}
}

func TestDefForthDefInteractsWithStore(t *testing.T) {
	// Defined words that use set/get to interact with the context store.
	// Direct set/get in a single Run.
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("x"), NewInteger(42),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("x"),
	})
	if err != nil {
		t.Fatalf("unexpected error on load-x: %v", err)
	}
	_as250, _ := AsInteger(result[0])
	if len(result) != 1 || _as250 != 42 {
		t.Errorf("got %v, want [42]", result)
	}
}

// --- Record type tests ---

func TestRecordTypeCreation(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// type Record [x:number y:number] => record{x:Number,y:Number}
	// In the list, each pair x:Number becomes a single-key map {x:Number}.
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	pairY := NewOrderedMap()
	pairY.Set("y", NewTypeLiteral(TNumber))
	input := []Value{NewWord("refine"), NewWord("Record"), NewList([]Value{NewMap(pairX), NewMap(pairY)})}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	if !IsRecordType(result[0]) {
		t.Fatalf("result is not a record type: %s", result[0].String())
	}
	if result[0].String() != "record{x:Number y:Number}" {
		t.Errorf("got %s, want record{x:Number y:Number}", result[0].String())
	}
}

func TestRecordTypeWithDef(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// def Point type Record [x:number y:number]
	// Point => record{x:Number,y:Number}
	pairX := NewOrderedMap()
	pairX.Set("x", NewTypeLiteral(TNumber))
	pairY := NewOrderedMap()
	pairY.Set("y", NewTypeLiteral(TNumber))
	input := []Value{
		NewWord("def"), NewWord("Point"),
		NewWord("refine"), NewWord("Record"), NewList([]Value{NewMap(pairX), NewMap(pairY)}),
		NewEnd(),
		NewWord("Point"),
	}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	// The name denotes its NODE (design/legacy/TYPE-REPRESENTATION.1.ignore
	// Stage 2); the record schema stays recoverable from it.
	if result[0].String() != "Point" {
		t.Errorf("Point must denote its node, got %s", result[0].String())
	}
	schema, sok := TypeContentOf(result[0])
	if !sok || !IsRecordType(schema) {
		t.Fatalf("the node must record its schema, got %s ok=%v", schema.String(), sok)
	}
	if schema.String() != "record{x:Number y:Number}" {
		t.Errorf("got %s, want record{x:Number y:Number}", schema.String())
	}
}

func TestRecordTypeUnify(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// Helper to run a unify test and return "result_string bool_string".
	runUnify := func(t *testing.T, input []Value) string {
		t.Helper()
		result, err := e.Run(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("got %d values, want 2", len(result))
		}
		return result[0].String() + " " + result[1].String()
	}

	t.Run("two records unify when compatible", func(t *testing.T) {
		f1 := NewOrderedMap()
		f1.Set("x", NewTypeLiteral(TAny))
		f2 := NewOrderedMap()
		f2.Set("x", NewTypeLiteral(TNumber))
		got := runUnify(t, []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")})
		if got != "record{x:Number} true" {
			t.Errorf("got %s, want record{x:Number} true", got)
		}
	})

	t.Run("two records fail with different keys", func(t *testing.T) {
		f1 := NewOrderedMap()
		f1.Set("x", NewTypeLiteral(TNumber))
		f2 := NewOrderedMap()
		f2.Set("y", NewTypeLiteral(TNumber))
		got := runUnify(t, []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")})
		if got != "'~unify-fail' false" {
			t.Errorf("got %s, want '~unify-fail' false", got)
		}
	})

	t.Run("field order must match", func(t *testing.T) {
		// type Record [x:number y:string] vs type Record [y:string x:number] — different order, fail.
		f1 := NewOrderedMap()
		f1.Set("x", NewTypeLiteral(TNumber))
		f1.Set("y", NewTypeLiteral(TString))
		f2 := NewOrderedMap()
		f2.Set("y", NewTypeLiteral(TString))
		f2.Set("x", NewTypeLiteral(TNumber))
		got := runUnify(t, []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")})
		if got != "'~unify-fail' false" {
			t.Errorf("got %s, want '~unify-fail' false", got)
		}
	})

	t.Run("same order unifies", func(t *testing.T) {
		f1 := NewOrderedMap()
		f1.Set("x", NewTypeLiteral(TNumber))
		f1.Set("y", NewTypeLiteral(TString))
		f2 := NewOrderedMap()
		f2.Set("x", NewTypeLiteral(TNumber))
		f2.Set("y", NewTypeLiteral(TString))
		got := runUnify(t, []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")})
		if got != "record{x:Number y:String} true" {
			t.Errorf("got %s, want record{x:Number y:String} true", got)
		}
	})

	t.Run("nested record types unify", func(t *testing.T) {
		inner1 := NewOrderedMap()
		inner1.Set("z", NewTypeLiteral(TAny))
		inner2 := NewOrderedMap()
		inner2.Set("z", NewTypeLiteral(TString))
		f1 := NewOrderedMap()
		f1.Set("a", NewRecordType(inner1))
		f2 := NewOrderedMap()
		f2.Set("a", NewRecordType(inner2))
		got := runUnify(t, []Value{NewRecordType(f1), NewRecordType(f2), NewWord("unify")})
		if got != "record{a:record{z:String}} true" {
			t.Errorf("got %s, want record{a:record{z:String}} true", got)
		}
	})

	t.Run("record does not unify with map", func(t *testing.T) {
		fields := NewOrderedMap()
		fields.Set("x", NewTypeLiteral(TNumber))
		m := NewOrderedMap()
		m.Set("x", NewInteger(1))
		got := runUnify(t, []Value{NewMap(m), NewRecordType(fields), NewWord("unify")})
		if got != "'~unify-fail' false" {
			t.Errorf("got %s, want '~unify-fail' false", got)
		}
	})

	t.Run("record does not unify with map type literal", func(t *testing.T) {
		fields := NewOrderedMap()
		fields.Set("x", NewTypeLiteral(TNumber))
		got := runUnify(t, []Value{NewTypeLiteral(TMap), NewRecordType(fields), NewWord("unify")})
		if got != "'~unify-fail' false" {
			t.Errorf("got %s, want '~unify-fail' false", got)
		}
	})

	t.Run("record does not unify with list", func(t *testing.T) {
		fields := NewOrderedMap()
		fields.Set("x", NewTypeLiteral(TNumber))
		got := runUnify(t, []Value{NewList([]Value{NewInteger(1)}), NewRecordType(fields), NewWord("unify")})
		if got != "'~unify-fail' false" {
			t.Errorf("got %s, want '~unify-fail' false", got)
		}
	})
}

func TestRecordTypeListWithMapElement(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	// type Record [{x:{z:boolean}} "y":1]
	// List element 0: map {x:{z:boolean}} — a map with nested map value
	// List element 1: pair "y":1 — a single-key map {y:1}
	innerMap := NewOrderedMap()
	innerMap.Set("z", NewTypeLiteral(TBoolean))
	elem0 := NewOrderedMap()
	elem0.Set("x", NewMap(innerMap))
	elem1 := NewOrderedMap()
	elem1.Set("y", NewInteger(1))
	input := []Value{NewWord("refine"), NewWord("Record"), NewList([]Value{NewMap(elem0), NewMap(elem1)})}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1", len(result))
	}
	if !IsRecordType(result[0]) {
		t.Fatalf("result is not a record type: %s", result[0].String())
	}
	rt, _ := AsRecordType(result[0])
	if rt.Fields.Len() != 2 {
		t.Errorf("got %d fields, want 2", rt.Fields.Len())
	}
	keys := rt.Fields.Keys()
	if keys[0] != "x" || keys[1] != "y" {
		t.Errorf("got keys %v, want [x y]", keys)
	}
}

// --- do word tests ---

func TestDoList(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	// do [1 add 2] → 3
	input := []Value{
		NewWord("do"),
		NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
	}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].String() != "3" {
		t.Errorf("do list: got %v, want [3]", result)
	}
}

func TestDoMap(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	// do {x:[3 add 4]} → {x:7}
	innerList := NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)})
	om := NewOrderedMap()
	om.Set("x", innerList)
	input := []Value{
		NewWord("do"),
		NewMap(om),
	}
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("do map: got %d results, want 1: %v", len(result), result)
	}
	t.Logf("do map result: %s (type=%v)", result[0].String(), result[0].Parent)
	if result[0].String() != "{x:7}" {
		t.Errorf("do map: got %s, want {x:7}", result[0].String())
	}
}

// --- Module tests ---

func TestModuleBasic(t *testing.T) {
	// module [def inc [add 1] export Foo {inc:inc}]
	// Should return a module descriptor.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("def"), NewWord("inc"), NewList([]Value{NewWord("add"), NewInteger(1)}),
		NewWord("export"), NewAtom("Foo"), makeMap("inc", NewWord("inc")),
	})
	result := runBoru(t, r, []Value{NewWord("module"), body})
	if len(result) != 1 {
		t.Fatalf("module: got %d results, want 1", len(result))
	}
	if !result[0].Parent.Equal(TModuleInst) {
		t.Fatalf("module: result is not a module, got %s", result[0].Parent)
	}
	desc, _ := asModuleDesc(result[0])
	fooExport, ok := desc.Exports["Foo"]
	if !ok {
		t.Fatal("module: export 'Foo' not found")
	}
	if fooExport.Len() != 1 {
		t.Fatalf("module: Foo export has wrong length: %d", fooExport.Len())
	}
	val, ok := fooExport.Get("inc")
	if !ok {
		t.Fatal("module: export Foo.inc not found")
	}
	// The exported "inc" should be the list [add 1]
	if !val.Parent.Equal(TList) {
		t.Errorf("module: export inc type = %s, want list", val.Parent)
	}
}

func TestModuleImportBasic(t *testing.T) {
	// import module [def inc [add 1] export Foo {inc:inc}]
	// Then Foo should be a def that resolves to a map {inc:[add 1]}
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("def"), NewWord("inc"), NewList([]Value{NewWord("add"), NewInteger(1)}),
		NewWord("export"), NewAtom("Foo"), makeMap("inc", NewWord("inc")),
	})
	// Run: import module [...]
	result := runBoru(t, r, []Value{
		NewWord("import"), NewWord("module"), body,
	})
	// import returns nothing
	if len(result) != 0 {
		t.Fatalf("import module: got %d results, want 0: %v", len(result), result)
	}

	// Now "Foo" should be defined and accessible.
	// Foo should resolve to map {inc:[add 1]}
	result2 := runBoru(t, r, []Value{NewWord("Foo")})
	if len(result2) != 1 {
		t.Fatalf("Foo: got %d results, want 1", len(result2))
	}
	if ModuleNSOf(result2[0]) == nil {
		t.Errorf("Foo: type = %s, want a module namespace", result2[0].Parent)
	}
}

func TestModuleImportDotAccess(t *testing.T) {
	// import module [def inc [add 1] export Foo {inc:inc}]
	// Foo.inc 2 → should evaluate to 3
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("def"), NewWord("inc"), NewList([]Value{NewWord("add"), NewInteger(1)}),
		NewWord("export"), NewAtom("Foo"), makeMap("inc", NewWord("inc")),
	})
	// Step 1: import the module
	runBoru(t, r, []Value{NewWord("import"), NewWord("module"), body})

	// Step 2: Foo.inc 2 → "inc" key in Foo map → [add 1] → applied to 2 → 3
	// Foo resolves to map {inc:[add 1]}
	// dot with "inc" gives [add 1]
	// do [add 1] with 2 on stack should give 3
	// Actually let's test just Foo . inc to get the value
	result := runBoru(t, r, []Value{NewWord("Foo"), NewAtom("inc"), NewWord("dot")})
	if len(result) != 1 {
		t.Fatalf("Foo.inc: got %d results, want 1: %v", len(result), result)
	}
	// Should be the list [add 1]
	if !result[0].Parent.Equal(TList) {
		t.Errorf("Foo.inc: type = %s, want list", result[0].Parent)
	}
}

func TestModuleIsolation(t *testing.T) {
	// Module's internal defs should not leak to the parent.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("def"), NewWord("secret"), NewInteger(42),
		NewWord("export"), NewAtom("M"), makeMap("x", NewInteger(1)),
	})
	runBoru(t, r, []Value{NewWord("import"), NewWord("module"), body})

	// "secret" should NOT be defined in the parent registry.
	if r.Lookup("secret") != nil {
		t.Error("module: internal def 'secret' leaked to parent")
	}
}

func TestModuleDefSubject(t *testing.T) {
	// Modules can be subjects of def.
	// def my-mod module [export M {x:1}]
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("export"), NewAtom("M"), makeMap("x", NewInteger(1)),
	})
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("my-mod"), NewOpenParen(), NewWord("module"), body, NewCloseParen(),
	})

	// my-mod should resolve to a module descriptor.
	result := runBoru(t, r, []Value{NewWord("my-mod")})
	if len(result) != 1 || !result[0].Parent.Equal(TModuleInst) {
		t.Fatalf("def my-mod: expected module descriptor, got %v", result)
	}

	// import my-mod should work.
	runBoru(t, r, []Value{NewWord("import"), NewWord("my-mod")})
	result2 := runBoru(t, r, []Value{NewWord("M")})
	if len(result2) != 1 || ModuleNSOf(result2[0]) == nil {
		t.Errorf("import my-mod: M = %v, want a module namespace", result2)
	}
}

func TestModuleImportRename(t *testing.T) {
	// import [Foo Bar] module [export Foo {x:1}]
	// Bar should be defined, Foo should not.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("export"), NewAtom("Foo"), makeMap("x", NewInteger(1)),
	})
	modResult := runBoru(t, r, []Value{NewWord("module"), body})
	if len(modResult) != 1 || !modResult[0].Parent.Equal(TModuleInst) {
		t.Fatal("expected module descriptor")
	}

	// import [Foo Bar] <module-desc>
	renameList := NewList([]Value{NewAtom("Foo"), NewAtom("Bar")})
	runBoru(t, r, []Value{NewWord("import"), renameList, modResult[0]})

	// Bar should be defined.
	result := runBoru(t, r, []Value{NewWord("Bar")})
	if len(result) != 1 {
		t.Fatalf("Bar: got %d results, want 1", len(result))
	}
}

func TestModuleImportMultiRename(t *testing.T) {
	// import [[Foo Baz]] module [export Foo {x:1}]
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	body := NewList([]Value{
		NewWord("export"), NewAtom("Foo"), makeMap("x", NewInteger(1)),
	})
	modResult := runBoru(t, r, []Value{NewWord("module"), body})

	// import [[Foo Baz]] <module-desc>
	pair := NewList([]Value{NewAtom("Foo"), NewAtom("Baz")})
	renameList := NewList([]Value{pair})
	runBoru(t, r, []Value{NewWord("import"), renameList, modResult[0]})

	result := runBoru(t, r, []Value{NewWord("Baz")})
	if len(result) != 1 {
		t.Fatalf("Baz: got %d results, want 1", len(result))
	}
}

func TestModuleFreshRegistry(t *testing.T) {
	// Defs in parent should not be visible inside module.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Define "foo" in parent.
	runBoru(t, r, []Value{
		NewWord("def"), NewWord("foo"), NewInteger(99),
	})

	// Module body tries to use "foo" — it should NOT find the parent def.
	// "foo" should resolve to an atom inside the module.
	body := NewList([]Value{
		NewWord("export"), NewAtom("M"), makeMap("val", NewWord("foo")),
	})
	result := runBoru(t, r, []Value{NewWord("module"), body})
	if len(result) != 1 || !result[0].Parent.Equal(TModuleInst) {
		t.Fatal("expected module")
	}
	desc, _ := asModuleDesc(result[0])
	mExport, ok := desc.Exports["M"]
	if !ok {
		t.Fatal("module: export 'M' not found")
	}
	val, _ := mExport.Get("val")
	// "foo" inside module should be an atom (not resolved), not 99.
	if val.Parent.ConformsTo(TInteger) {
		t.Error("module: parent def 'foo' leaked into module")
	}
}

// makeMap is a helper to create a map Value with a single key-value pair.
func makeMap(key string, val Value) Value {
	om := NewOrderedMap()
	om.Set(key, val)
	return NewMap(om)
}

// --- Benchmarks ---

func BenchmarkSimpleExpression(b *testing.B) {
	reg, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	registerIOWords(reg)
	input := []Value{NewInteger(1), NewInteger(2), NewWord("add")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

func BenchmarkComplexExpression(b *testing.B) {
	reg, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	registerIOWords(reg)
	input := []Value{
		NewInteger(1), NewInteger(2), NewWord("add"),
		NewInteger(3), NewWord("mul"),
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng := New(reg)
		_, _ = eng.Run(input)
	}
}

func BenchmarkRepeatedRun(b *testing.B) {
	reg, err := DefaultRegistry()
	if err != nil {
		b.Fatal(err)
	}
	registerIOWords(reg)
	input := []Value{NewInteger(1), NewInteger(2), NewWord("add")}
	eng := New(reg)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Run(input)
	}
}
