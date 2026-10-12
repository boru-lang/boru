package native

import (
	"testing"
)

// --- *Type system tests ---

func TestTypeConformsTo(t *testing.T) {
	tests := []struct {
		name    string
		typ     *Type
		pattern *Type
		want    bool
	}{
		{"exact match", TStringProper, TStringProper, true},
		{"child matches parent", TStringProper, TString, true},
		{"parent does not match child", TString, TStringProper, false},
		{"any matches string", TStringProper, TAny, true},
		{"any matches integer", TInteger, TAny, true},
		{"integer does not match string", TInteger, TString, false},
		{"string/empty matches string", TStringEmpty, TString, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.typ.ConformsTo(tt.pattern)
			if got != tt.want {
				t.Errorf("%s.ConformsTo(%s) = %v, want %v", tt.typ, tt.pattern, got, tt.want)
			}
		})
	}
}

// --- Value constructor tests ---

func TestNewString(t *testing.T) {
	// Strings carry the String subtype: ProperString for non-empty,
	// EmptyString for "". Both still match TString via the type
	// lattice, so Equal(TString) is false but ConformsTo(TString) is
	// true; specific-value dispatch still routes through
	// Signature.Patterns where finer granularity is needed.
	v := NewString("hello")
	if !v.Parent.Equal(TStringProper) {
		t.Errorf("type = %s, want ProperString", v.Parent)
	}
	if !v.Parent.ConformsTo(TString) {
		t.Errorf("ProperString should match TString")
	}
	_as0, _ := AsString(v)
	if _as0 != "hello" {
		t.Errorf("data = %q, want %q", _as0, "hello")
	}

	empty := NewString("")
	if !empty.Parent.Equal(TStringEmpty) {
		t.Errorf("empty type = %s, want EmptyString", empty.Parent)
	}
	if !empty.Parent.ConformsTo(TString) {
		t.Errorf("EmptyString should match TString")
	}
}

func TestNewInteger(t *testing.T) {
	v := NewInteger(42)
	if !v.Parent.ConformsTo(TInteger) {
		t.Errorf("type = %s, want matches number/integer", v.Parent)
	}
	_as2, _ := AsInteger(v)
	if _as2 != 42 {
		_as3, _ := AsInteger(v)
		t.Errorf("data = %d, want 42", _as3)
	}
}

func TestNewWord(t *testing.T) {
	v := NewWord("upper")
	if !IsWord(&v) {
		t.Errorf("IsWord() = false")
	}
	_as4, _ := AsWord(&v)
	if _as4.Name != "upper" {
		_as5, _ := AsWord(&v)
		t.Errorf("name = %q, want %q", _as5.Name, "upper")
	}
}

// --- Engine tests: literals ---

func TestLiteralSelfInsert(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	tests := []struct {
		name  string
		input []Value
		want  string // expected string representation of the single result
	}{
		{"integer", []Value{NewInteger(42)}, "42"},
		{"string", []Value{NewString("hello")}, "'hello'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1", len(result))
			}
			if result[0].String() != tt.want {
				t.Errorf("got %s, want %s", result[0].String(), tt.want)
			}
		})
	}
}

// --- Engine tests: prefix functions ---

func TestPrefixUpper(t *testing.T) {
	// a upper -> 'A'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("a"), NewWord("upper")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as6, _ := AsString(result[0])
	if _as6 != "A" {
		_as7, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as7, "A")
	}
}

func TestPrefixLower(t *testing.T) {
	// C lower -> 'c'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewString("C"), NewWord("lower")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as8, _ := AsString(result[0])
	if _as8 != "c" {
		_as9, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as9, "c")
	}
}

// --- Engine tests: forward functions ---

func TestForwardLower(t *testing.T) {
	// lower B -> 'b'
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewWord("lower"), NewString("B")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as10, _ := AsString(result[0])
	if _as10 != "b" {
		_as11, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as11, "b")
	}
}

// --- Engine tests: signature error ---

func TestSignatureError(t *testing.T) {
	// 99 lower -> signature error (integer doesn't match string)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewInteger(99), NewWord("lower")})
	if err == nil {
		t.Fatal("expected signature error, got nil")
	}
}

// --- Engine tests: forth primitives ---

func TestDup(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewWord("dup")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as13, _ := AsInteger(result[0])
	_as12, _ := AsInteger(result[1])
	if _as13 != 1 || _as12 != 1 {
		t.Errorf("got [%v, %v], want [1, 1]", result[0], result[1])
	}
}

func TestSwap(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("swap")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as15, _ := AsInteger(result[0])
	_as14, _ := AsInteger(result[1])
	if _as15 != 2 || _as14 != 1 {
		t.Errorf("got [%v, %v], want [2, 1]", result[0], result[1])
	}
}

func TestDrop(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewWord("drop")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

// --- Engine tests: forward Forth primitives ---

func TestDupForward(t *testing.T) {
	// dup/f 1 → [1, 1]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWordModified("dup", -1, false, true),
		NewInteger(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as17, _ := AsInteger(result[0])
	_as16, _ := AsInteger(result[1])
	if _as17 != 1 || _as16 != 1 {
		t.Errorf("got [%v, %v], want [1, 1]", result[0], result[1])
	}
}

func TestSwapForward(t *testing.T) {
	// Post §1.4 (unified dispatch): under /f the matcher fills sig
	// args from forward in source order, so `swap/f 1 2` binds
	// sig[0]=1 (first forward), sig[1]=2. The handler emits its
	// output in splice (left-to-right) order. With the unified-rule
	// handler `[args[0], args[1]]`, the result is [1, 2] — i.e.
	// /f no longer "swaps" the two values, because the forced-forward
	// reading lays them out in their source order. Stack-mode
	// swap (`1 2 swap`) still produces [2, 1] (see TestSwap).
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWordModified("swap", -1, false, true),
		NewInteger(1), NewInteger(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as19, _ := AsInteger(result[0])
	_as18, _ := AsInteger(result[1])
	if _as19 != 1 || _as18 != 2 {
		t.Errorf("got [%v, %v], want [1, 2]", result[0], result[1])
	}
}

func TestSwapInfix(t *testing.T) {
	// 1 swap 2 → error (swap is stack-only in the new model)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewInteger(1), NewWord("swap"), NewInteger(2),
	})
	if err == nil {
		t.Fatal("expected error for swap infix (swap is stack-only), got nil")
	}
}

func TestDropForward(t *testing.T) {
	// drop/f 1 → []
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWordModified("drop", -1, false, true),
		NewInteger(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d values, want 0: %v", len(result), result)
	}
}

func TestOver(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("over")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as22, _ := AsInteger(result[0])
	_as21, _ := AsInteger(result[1])
	_as20, _ := AsInteger(result[2])
	if len(result) != 3 || _as22 != 1 || _as21 != 2 || _as20 != 1 {
		t.Errorf("got %v, want [1, 2, 1]", result)
	}
}

func TestRot(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewWord("rot")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as25, _ := AsInteger(result[0])
	_as24, _ := AsInteger(result[1])
	_as23, _ := AsInteger(result[2])
	if len(result) != 3 || _as25 != 2 || _as24 != 3 || _as23 != 1 {
		t.Errorf("got %v, want [2, 3, 1]", result)
	}
}

func TestNip(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("nip")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as26, _ := AsInteger(result[0])
	if len(result) != 1 || _as26 != 2 {
		t.Errorf("got %v, want [2]", result)
	}
}

func TestTuck(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("tuck")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as29, _ := AsInteger(result[0])
	_as28, _ := AsInteger(result[1])
	_as27, _ := AsInteger(result[2])
	if len(result) != 3 || _as29 != 2 || _as28 != 1 || _as27 != 2 {
		t.Errorf("got %v, want [2, 1, 2]", result)
	}
}

func Test2Dup(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("dup2")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as33, _ := AsInteger(result[0])
	_as32, _ := AsInteger(result[1])
	_as31, _ := AsInteger(result[2])
	_as30, _ := AsInteger(result[3])
	if len(result) != 4 || _as33 != 1 || _as32 != 2 || _as31 != 1 || _as30 != 2 {
		t.Errorf("got %v, want [1, 2, 1, 2]", result)
	}
}

func Test2Swap(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4), NewWord("swap2")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as37, _ := AsInteger(result[0])
	_as36, _ := AsInteger(result[1])
	_as35, _ := AsInteger(result[2])
	_as34, _ := AsInteger(result[3])
	if len(result) != 4 || _as37 != 3 || _as36 != 4 || _as35 != 1 || _as34 != 2 {
		t.Errorf("got %v, want [3, 4, 1, 2]", result)
	}
}

func Test2Drop(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewWord("drop2")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want []", result)
	}
}

func Test2Over(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4), NewWord("over2")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as43, _ := AsInteger(result[0])
	_as42, _ := AsInteger(result[1])
	_as41, _ := AsInteger(result[2])
	_as40, _ := AsInteger(result[3])
	_as39, _ := AsInteger(result[4])
	_as38, _ := AsInteger(result[5])
	if len(result) != 6 || _as43 != 1 || _as42 != 2 || _as41 != 3 || _as40 != 4 || _as39 != 1 || _as38 != 2 {
		t.Errorf("got %v, want [1, 2, 3, 4, 1, 2]", result)
	}
}

func TestDepthEmpty(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewWord("depth")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as44, _ := AsInteger(result[0])
	if len(result) != 1 || _as44 != 0 {
		t.Errorf("got %v, want [0]", result)
	}
}

func TestDepth(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewWord("depth")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as45, _ := AsInteger(result[3])
	if len(result) != 4 || _as45 != 3 {
		t.Errorf("got %v, want [1, 2, 3, 3]", result)
	}
}

func TestPick0(t *testing.T) {
	// pick 0 = dup: 1 2 3 0 pick → 1 2 3 3
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(0), NewWord("pick")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as49, _ := AsInteger(result[0])
	_as48, _ := AsInteger(result[1])
	_as47, _ := AsInteger(result[2])
	_as46, _ := AsInteger(result[3])
	if len(result) != 4 || _as49 != 1 || _as48 != 2 || _as47 != 3 || _as46 != 3 {
		t.Errorf("got %v, want [1, 2, 3, 3]", result)
	}
}

func TestPick2(t *testing.T) {
	// 1 2 3 2 pick → 1 2 3 1
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(2), NewWord("pick")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as53, _ := AsInteger(result[0])
	_as52, _ := AsInteger(result[1])
	_as51, _ := AsInteger(result[2])
	_as50, _ := AsInteger(result[3])
	if len(result) != 4 || _as53 != 1 || _as52 != 2 || _as51 != 3 || _as50 != 1 {
		t.Errorf("got %v, want [1, 2, 3, 1]", result)
	}
}

func TestPickOutOfRange(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewInteger(1), NewInteger(5), NewWord("pick")})
	if err == nil {
		t.Fatal("expected error for out-of-range pick")
	}
}

func TestRoll2(t *testing.T) {
	// 1 2 3 2 roll → 2 3 1 (same as rot)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(2), NewWord("roll")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as56, _ := AsInteger(result[0])
	_as55, _ := AsInteger(result[1])
	_as54, _ := AsInteger(result[2])
	if len(result) != 3 || _as56 != 2 || _as55 != 3 || _as54 != 1 {
		t.Errorf("got %v, want [2, 3, 1]", result)
	}
}

func TestRoll1(t *testing.T) {
	// 1 2 3 1 roll → 1 3 2 (same as swap)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(1), NewWord("roll")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as59, _ := AsInteger(result[0])
	_as58, _ := AsInteger(result[1])
	_as57, _ := AsInteger(result[2])
	if len(result) != 3 || _as59 != 1 || _as58 != 3 || _as57 != 2 {
		t.Errorf("got %v, want [1, 3, 2]", result)
	}
}

func TestRoll0(t *testing.T) {
	// 1 2 3 0 roll → 1 2 3 (no-op)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(0), NewWord("roll")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as62, _ := AsInteger(result[0])
	_as61, _ := AsInteger(result[1])
	_as60, _ := AsInteger(result[2])
	if len(result) != 3 || _as62 != 1 || _as61 != 2 || _as60 != 3 {
		t.Errorf("got %v, want [1, 2, 3]", result)
	}
}

// TestAbs, TestNegate, TestMin, TestMax moved to internal/nativemod/ (boru:math module).

// --- Engine tests: modifier forcing ---

func TestForceForward(t *testing.T) {
	// lower/f E -> 'e' (force forward even though prefix exists)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWordModified("lower", -1, false, true),
		NewString("E"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as63, _ := AsString(result[0])
	if _as63 != "e" {
		_as64, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as64, "e")
	}
}

func TestForceStack(t *testing.T) {
	// F lower/s -> 'f' (force stack, no forward considered)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("F"),
		NewWordModified("lower", -1, true, false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as65, _ := AsString(result[0])
	if _as65 != "f" {
		_as66, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as66, "f")
	}
}

func TestArgCountForward(t *testing.T) {
	// lower/1 D -> 'd' (arg count 1 picks the forward signature)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWordModified("lower", 1, false, false),
		NewString("D"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as67, _ := AsString(result[0])
	if _as67 != "d" {
		_as68, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as68, "d")
	}
}

// --- Engine tests: unknown word ---

func TestUnknownWordErrors(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("foo")})
	if err == nil {
		t.Fatal("expected error for undefined word, got nil")
	}
}

// --- Engine tests: arithmetic (prefix / Forth-style) ---

func TestArithmeticPrefix(t *testing.T) {
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
		// 1 2 add → 3
		{"add", []Value{NewInteger(1), NewInteger(2), NewWord("add")}, 3},
		// 10 3 sub → 7
		{"sub", []Value{NewInteger(10), NewInteger(3), NewWord("sub")}, 7},
		// 4 5 mul → 20
		{"mul", []Value{NewInteger(4), NewInteger(5), NewWord("mul")}, 20},
		// 10 3 div → 3
		{"div", []Value{NewInteger(10), NewInteger(3), NewWord("div")}, 3},
		// 10 3 mod → 1
		{"mod", []Value{NewInteger(10), NewInteger(3), NewWord("mod")}, 1},
		// negative: 3 10 sub → -7
		{"sub negative", []Value{NewInteger(3), NewInteger(10), NewWord("sub")}, -7},
		// zero: 0 5 add → 5
		{"add zero", []Value{NewInteger(0), NewInteger(5), NewWord("add")}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as71, _ := AsInteger(result[0])
			if _as71 != tt.want {
				_as72, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as72, tt.want)
			}
		})
	}
}

// --- Engine tests: arithmetic (infix via forward mechanism) ---

func TestArithmeticInfix(t *testing.T) {
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
		// 1 add 2 → 3
		{"add", []Value{NewInteger(1), NewWord("add"), NewInteger(2)}, 3},
		// 10 sub 3 → 7
		{"sub", []Value{NewInteger(10), NewWord("sub"), NewInteger(3)}, 7},
		// 4 mul 5 → 20
		{"mul", []Value{NewInteger(4), NewWord("mul"), NewInteger(5)}, 20},
		// 10 div 3 → 3
		{"div", []Value{NewInteger(10), NewWord("div"), NewInteger(3)}, 3},
		// 10 mod 3 → 1
		{"mod", []Value{NewInteger(10), NewWord("mod"), NewInteger(3)}, 1},
		// 1 sub 2 → -1
		{"sub negative", []Value{NewInteger(1), NewWord("sub"), NewInteger(2)}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as73, _ := AsInteger(result[0])
			if _as73 != tt.want {
				_as74, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as74, tt.want)
			}
		})
	}
}

// --- Engine tests: arithmetic errors ---

func TestArithmeticErrors(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)

	tests := []struct {
		name  string
		input []Value
	}{
		// division by zero
		{"div by zero prefix", []Value{NewInteger(10), NewInteger(0), NewWord("div")}},
		{"div by zero infix", []Value{NewInteger(10), NewWord("div"), NewInteger(0)}},
		// modulo by zero
		{"mod by zero prefix", []Value{NewInteger(10), NewInteger(0), NewWord("mod")}},
		{"mod by zero infix", []Value{NewInteger(10), NewWord("mod"), NewInteger(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.Run(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// --- Engine tests: arithmetic chaining ---

func TestArithmeticChaining(t *testing.T) {
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
		// 1 2 add 3 add → 6 (prefix then infix)
		{"prefix then infix", []Value{
			NewInteger(1), NewInteger(2), NewWord("add"),
			NewWord("add"), NewInteger(3),
		}, 6},
		// 2 3 mul 4 add → 10 (prefix mul, then infix add)
		{"mul then add", []Value{
			NewInteger(2), NewInteger(3), NewWord("mul"),
			NewWord("add"), NewInteger(4),
		}, 10},
		// 10 sub 3 → 7, then dup → [7, 7], then mul → 49
		{"infix sub, dup, prefix mul", []Value{
			NewInteger(10), NewWord("sub"), NewInteger(3),
			NewWord("dup"),
			NewWord("mul"),
		}, 49},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as75, _ := AsInteger(result[0])
			if _as75 != tt.want {
				_as76, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as76, tt.want)
			}
		})
	}
}

// --- Engine tests: left-to-right operator evaluation ---

func TestLeftToRightMulAndAdd(t *testing.T) {
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
		// 2 add 3 mul 4 → left-to-right: (2+3)*4 = 20
		{"add then mul", []Value{
			NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
		}, 20},
		// 2 mul 3 add 4 → left-to-right: (2*3)+4 = 10
		{"mul then add", []Value{
			NewInteger(2), NewWord("mul"), NewInteger(3), NewWord("add"), NewInteger(4),
		}, 10},
		// 1 add 2 mul 3 add 4 → left-to-right: ((1+2)*3)+4 = 13
		{"add mul add", []Value{
			NewInteger(1), NewWord("add"), NewInteger(2), NewWord("mul"), NewInteger(3),
			NewWord("add"), NewInteger(4),
		}, 13},
		// 2 add 3 mul 4 mul 5 → left-to-right: ((2+3)*4)*5 = 100
		{"add mul mul", []Value{
			NewInteger(2), NewWord("add"), NewInteger(3), NewWord("mul"), NewInteger(4),
			NewWord("mul"), NewInteger(5),
		}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as77, _ := AsInteger(result[0])
			if _as77 != tt.want {
				_as78, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as78, tt.want)
			}
		})
	}
}

func TestLeftToRightSameLevel(t *testing.T) {
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
		// 10 sub 3 sub 1 → (10-3)-1 = 6 (left-to-right)
		{"sub sub", []Value{
			NewInteger(10), NewWord("sub"), NewInteger(3), NewWord("sub"), NewInteger(1),
		}, 6},
		// 2 mul 6 div 3 → (2*6)/3 = 4 (left-to-right)
		{"mul div", []Value{
			NewInteger(2), NewWord("mul"), NewInteger(6), NewWord("div"), NewInteger(3),
		}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as79, _ := AsInteger(result[0])
			if _as79 != tt.want {
				_as80, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as80, tt.want)
			}
		})
	}
}

func TestLeftToRightPrefixUnaffected(t *testing.T) {
	// Prefix (Forth-style) should still work: 2 3 mul 4 add → 10
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(2), NewInteger(3), NewWord("mul"),
		NewWord("add"), NewInteger(4),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as81, _ := AsInteger(result[0])
	if _as81 != 10 {
		_as82, _ := AsInteger(result[0])
		t.Errorf("got %d, want 10", _as82)
	}
}

// --- Engine tests: storage (set/get) ---

func TestSetGetForward(t *testing.T) {
	// set foo 99 context end get foo context → [99]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as83, _ := AsInteger(result[0])
	if _as83 != 99 {
		_as84, _ := AsInteger(result[0])
		t.Errorf("got %d, want 99", _as84)
	}
}

func TestSetGetWithoutEnd(t *testing.T) {
	// set foo 99 context get foo context → [99] (end is optional)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("context"), NewWord("dot"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as85, _ := AsInteger(result[0])
	if _as85 != 99 {
		_as86, _ := AsInteger(result[0])
		t.Errorf("got %d, want 99", _as86)
	}
}

func TestSetGetPrefix(t *testing.T) {
	// context 42 "bar" set context "bar" get → [42]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewInteger(42), NewString("bar"), NewWord("set"),
		NewWord("context"), NewString("bar"), NewWord("get"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as87, _ := AsInteger(result[0])
	if _as87 != 42 {
		_as88, _ := AsInteger(result[0])
		t.Errorf("got %d, want 42", _as88)
	}
}

func TestSetGetString(t *testing.T) {
	// set "name" "hello" context end get "name" context → ['hello']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewString("name"), NewString("hello"),
		NewEnd(),
		NewWord("context"), NewWord("get"), NewString("name"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as89, _ := AsString(result[0])
	if _as89 != "hello" {
		_as90, _ := AsString(result[0])
		t.Errorf("got %q, want %q", _as90, "hello")
	}
}

func TestSetOverwrite(t *testing.T) {
	// set x 1 context end set x 2 context end get x context → [2]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("x"), NewInteger(1),
		NewEnd(),
		NewWord("context"), NewWord("set"), NewWord("x"), NewInteger(2),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("x"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as91, _ := AsInteger(result[0])
	if _as91 != 2 {
		_as92, _ := AsInteger(result[0])
		t.Errorf("got %d, want 2", _as92)
	}
}

func TestGetUnknownKey(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{NewWord("context"), NewWord("dot"), NewWord("missing")})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestEndNoOp(t *testing.T) {
	// 42 end → [42] (end just removes itself)
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{NewInteger(42), NewEnd()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as93, _ := AsInteger(result[0])
	if _as93 != 42 {
		_as94, _ := AsInteger(result[0])
		t.Errorf("got %d, want 42", _as94)
	}
}

func TestEndMultiple(t *testing.T) {
	// 1 end 2 end 3 → [1, 2, 3]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewInteger(1), NewEnd(),
		NewInteger(2), NewEnd(),
		NewInteger(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d values, want 3: %v", len(result), result)
	}
	for i, want := range []int64{1, 2, 3} {
		_as95, _ := AsInteger(result[i])
		if _as95 != want {
			_as96, _ := AsInteger(result[i])
			t.Errorf("result[%d] = %d, want %d", i, _as96, want)
		}
	}
}

func TestEndTerminatesForward(t *testing.T) {
	// context set foo 99 end 88 → stores foo=99 in context, result=[88]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99), NewEnd(), NewInteger(88),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as97, _ := AsInteger(result[0])
	if _as97 != 88 {
		_as98, _ := AsInteger(result[0])
		t.Errorf("got %d, want 88", _as98)
	}
}

func TestEndTerminatesForwardNoRemainder(t *testing.T) {
	// context set foo 99 end context get foo → stores foo=99 then reads it back
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as99, _ := AsInteger(result[0])
	if _as99 != 99 {
		_as100, _ := AsInteger(result[0])
		t.Errorf("got %d, want 99", _as100)
	}
}

func TestEndInsufficientArgs(t *testing.T) {
	// set foo end → forward expects 3, collected 1, no prefix → error
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewWord("set"), NewWord("foo"), NewEnd(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSetGetStorePersistsWithinRun(t *testing.T) {
	// Store set/get within a single Run on the same context
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("key"), NewInteger(100),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("key"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as101, _ := AsInteger(result[0])
	if len(result) != 1 || _as101 != 100 {
		t.Errorf("got %v, want [100]", result)
	}
}

// --- Engine tests: multiple operations ---

func TestChainedOps(t *testing.T) {
	// a upper dup -> ['A', 'A']
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewString("a"),
		NewWord("upper"),
		NewWord("dup"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d values, want 2: %v", len(result), result)
	}
	_as103, _ := AsString(result[0])
	_as102, _ := AsString(result[1])
	if _as103 != "A" || _as102 != "A" {
		t.Errorf("got [%v, %v], want ['A', 'A']", result[0], result[1])
	}
}

// --- Engine tests: parentheses ---

func TestParenSimpleArithmetic(t *testing.T) {
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
		// 1 mul (2 add 3) → 1*(2+3) = 5
		{"mul paren add", []Value{
			NewInteger(1), NewWord("mul"),
			NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
		}, 5},
		// (2 add 3) → 5
		{"just paren", []Value{
			NewOpenParen(), NewInteger(2), NewWord("add"), NewInteger(3), NewCloseParen(),
		}, 5},
		// (2 mul 3) add 4 → 6+4 = 10
		{"paren mul then add", []Value{
			NewOpenParen(), NewInteger(2), NewWord("mul"), NewInteger(3), NewCloseParen(),
			NewWord("add"), NewInteger(4),
		}, 10},
		// 2 mul (3 add 4) → 2*7 = 14
		{"mul paren add 2", []Value{
			NewInteger(2), NewWord("mul"),
			NewOpenParen(), NewInteger(3), NewWord("add"), NewInteger(4), NewCloseParen(),
		}, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as104, _ := AsInteger(result[0])
			if _as104 != tt.want {
				_as105, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as105, tt.want)
			}
		})
	}
}

func TestParenWithSet(t *testing.T) {
	// set foo (1 add 2) end get foo → [3]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("foo"),
		NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as106, _ := AsInteger(result[0])
	if _as106 != 3 {
		_as107, _ := AsInteger(result[0])
		t.Errorf("got %d, want 3", _as107)
	}
}

func TestParenNested(t *testing.T) {
	// (1 add (2 mul 3)) → 1+6 = 7
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(),
		NewInteger(1), NewWord("add"),
		NewOpenParen(), NewInteger(2), NewWord("mul"), NewInteger(3), NewCloseParen(),
		NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as108, _ := AsInteger(result[0])
	if _as108 != 7 {
		_as109, _ := AsInteger(result[0])
		t.Errorf("got %d, want 7", _as109)
	}
}

func TestParenLiteral(t *testing.T) {
	// (42) → [42]
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	result, err := e.Run([]Value{
		NewOpenParen(), NewInteger(42), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d values, want 1: %v", len(result), result)
	}
	_as110, _ := AsInteger(result[0])
	if _as110 != 42 {
		_as111, _ := AsInteger(result[0])
		t.Errorf("got %d, want 42", _as111)
	}
}

func TestParenUnmatchedOpen(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewOpenParen(), NewInteger(1),
	})
	if err == nil {
		t.Fatal("expected error for unmatched open paren, got nil")
	}
}

func TestParenUnmatchedClose(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	e := New(reg)
	_, err = e.Run([]Value{
		NewInteger(1), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("expected error for unmatched close paren, got nil")
	}
}

func TestParenWithLeftToRight(t *testing.T) {
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
		// (1 add 2) mul 3 → left-to-right with parens: 3*3 = 9
		{"paren groups evaluate first", []Value{
			NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
			NewWord("mul"), NewInteger(3),
		}, 9},
		// 3 mul (1 add 2) → left-to-right: 3*3 = 9
		{"mul paren group", []Value{
			NewInteger(3), NewWord("mul"),
			NewOpenParen(), NewInteger(1), NewWord("add"), NewInteger(2), NewCloseParen(),
		}, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Run(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d values, want 1: %v", len(result), result)
			}
			_as112, _ := AsInteger(result[0])
			if _as112 != tt.want {
				_as113, _ := AsInteger(result[0])
				t.Errorf("got %d, want %d", _as113, tt.want)
			}
		})
	}
}
