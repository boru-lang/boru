package native

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// --- Math function coverage tests ---
// Tests for basic arithmetic operations (add, sub, mul, div, mod, pow)
// that remain as built-in words.
// Extended math operations (abs, negate, sign, min, max, ceil, floor,
// round, trunc, sqrt, cbrt, exp, log, log2, log10, sin, cos, tan,
// asin, acos, atan, atan2, hypot, math-pi, math-e) are now in the
// "boru:math-util" native module and tested in internal/nativemod/.

func TestMathPow(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Integer pow
	result := runBoru(t, r, []Value{NewInteger(2), NewWord("pow"), NewInteger(3)})
	_as0, _ := AsInteger(result[0])
	if _as0 != 8 {
		t.Errorf("2 pow 3 = %v, want 8", result[0])
	}
	// Integer pow with 0 exponent
	result = runBoru(t, r, []Value{NewInteger(5), NewWord("pow"), NewInteger(0)})
	_as1, _ := AsInteger(result[0])
	if _as1 != 1 {
		t.Errorf("5 pow 0 = %v, want 1", result[0])
	}
	// Negative exponent should error, and the error carries the
	// arith_error code like every sibling arithmetic fault (NUR010).
	err := runBoruError(t, r, []Value{NewInteger(2), NewWord("pow"), NewInteger(-1)})
	if err == nil {
		t.Error("expected error for negative exponent")
	}
	var be *BoruError
	if !errors.As(err, &be) || be.Code != "arith_error" {
		t.Errorf("negative-exponent error = %v, want a BoruError coded arith_error", err)
	}
	if !strings.Contains(be.Detail, "pow: negative exponent -1") {
		t.Errorf("negative-exponent detail = %q, want the pinned runtime message", be.Detail)
	}
	// Float pow
	result = runBoru(t, r, []Value{NewFloat(2), NewWord("pow"), NewFloat(0.5)})
	_as2, _ := AsNumber(result[0])
	if math.Abs(_as2-math.Sqrt(2)) > 0.0001 {
		t.Errorf("2 pow 0.5 = %v, want sqrt(2)", result[0])
	}
}

func TestMathDiv(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Integer div
	result := runBoru(t, r, []Value{NewInteger(10), NewWord("div"), NewInteger(3)})
	_as3, _ := AsInteger(result[0])
	if _as3 != 3 {
		t.Errorf("10 div 3 = %v, want 3", result[0])
	}
	// Float div
	result = runBoru(t, r, []Value{NewFloat(10), NewWord("div"), NewFloat(4)})
	_as4, _ := AsNumber(result[0])
	if _as4 != 2.5 {
		t.Errorf("10.0 div 4.0 = %v, want 2.5", result[0])
	}
	// Float div by zero is IEEE-754, not an error: x/0 → ±inf, 0/0 → nan.
	result = runBoru(t, r, []Value{NewFloat(1), NewWord("div"), NewFloat(0)})
	if f, _ := AsNumber(result[0]); !math.IsInf(f, 1) {
		t.Errorf("1.0 div 0.0 = %v, want +inf", result[0])
	}
	result = runBoru(t, r, []Value{NewFloat(0), NewWord("div"), NewFloat(0)})
	if f, _ := AsNumber(result[0]); !math.IsNaN(f) {
		t.Errorf("0.0 div 0.0 = %v, want nan", result[0])
	}
	// Integer div by zero stays a hard error (no integer infinity).
	if err := runBoruError(t, r, []Value{NewInteger(1), NewWord("div"), NewInteger(0)}); err == nil {
		t.Error("expected error for integer division by zero")
	}
}

func TestMathMod(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	// Integer mod
	result := runBoru(t, r, []Value{NewInteger(10), NewWord("mod"), NewInteger(3)})
	_as5, _ := AsInteger(result[0])
	if _as5 != 1 {
		t.Errorf("10 mod 3 = %v, want 1", result[0])
	}
	// Float mod
	result = runBoru(t, r, []Value{NewFloat(10.5), NewWord("mod"), NewFloat(3)})
	_as6, _ := AsNumber(result[0])
	if math.Abs(_as6-1.5) > 0.0001 {
		t.Errorf("10.5 mod 3.0 = %v, want 1.5", result[0])
	}
	// Float mod by zero is IEEE-754 nan, not an error.
	result = runBoru(t, r, []Value{NewFloat(1), NewWord("mod"), NewFloat(0)})
	if f, _ := AsNumber(result[0]); !math.IsNaN(f) {
		t.Errorf("1.0 mod 0.0 = %v, want nan", result[0])
	}
	// Integer mod by zero stays a hard error.
	if err := runBoruError(t, r, []Value{NewInteger(1), NewWord("mod"), NewInteger(0)}); err == nil {
		t.Error("expected error for integer modulo by zero")
	}
}

func TestMathMulFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewFloat(2.5), NewWord("mul"), NewFloat(4)})
	_as7, _ := AsNumber(result[0])
	if _as7 != 10.0 {
		t.Errorf("2.5 mul 4.0 = %v, want 10.0", result[0])
	}
}

func TestMathSubFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewFloat(5.5), NewWord("sub"), NewFloat(2.5)})
	_as8, _ := AsNumber(result[0])
	if _as8 != 3.0 {
		t.Errorf("5.5 sub 2.5 = %v, want 3.0", result[0])
	}
}

func TestMathAddFloat(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewFloat(1.5), NewWord("add"), NewFloat(2.5)})
	_as9, _ := AsNumber(result[0])
	if _as9 != 4.0 {
		t.Errorf("1.5 add 2.5 = %v, want 4.0", result[0])
	}
}

// --- Boolean operation coverage tests ---

func TestBoolXor(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	tests := []struct {
		a, b bool
		want bool
	}{
		{true, true, false},
		{true, false, true},
		{false, true, true},
		{false, false, false},
	}
	for _, tt := range tests {
		result := runBoru(t, r, []Value{NewBoolean(tt.a), NewWord("xor"), NewBoolean(tt.b)})
		_as10, _ := AsBoolean(result[0])
		if _as10 != tt.want {
			_as11, _ := AsBoolean(result[0])
			t.Errorf("%v xor %v = %v, want %v", tt.a, tt.b, _as11, tt.want)
		}
	}
}

func TestBoolNand(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("nand"), NewBoolean(true)})
	_as12, _ := AsBoolean(result[0])
	if _as12 != false {
		t.Errorf("true nand true = %v, want false", result[0])
	}
	result = runBoru(t, r, []Value{NewBoolean(true), NewWord("nand"), NewBoolean(false)})
	_as13, _ := AsBoolean(result[0])
	if _as13 != true {
		t.Errorf("true nand false = %v, want true", result[0])
	}
}

func TestBoolImplies(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewBoolean(true), NewWord("implies"), NewBoolean(false)})
	_as14, _ := AsBoolean(result[0])
	if _as14 != false {
		t.Errorf("true implies false = %v, want false", result[0])
	}
	result = runBoru(t, r, []Value{NewBoolean(false), NewWord("implies"), NewBoolean(false)})
	_as15, _ := AsBoolean(result[0])
	if _as15 != true {
		t.Errorf("false implies false = %v, want true", result[0])
	}
}

func TestBoolTorNonBoolean(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("tor"), NewInteger(2)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !IsDisjunct(&result[0]) {
		t.Errorf("expected disjunct, got %s", result[0])
	}
}

// --- Mixed integer/decimal operations ---

func TestMathAddMixed(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewInteger(1), NewWord("add"), NewFloat(2.5)})
	_as16, _ := AsNumber(result[0])
	if _as16 != 3.5 {
		t.Errorf("1 add 2.5 = %v, want 3.5", result[0])
	}
	result = runBoru(t, r, []Value{NewFloat(1.5), NewWord("add"), NewInteger(2)})
	_as17, _ := AsNumber(result[0])
	if _as17 != 3.5 {
		t.Errorf("1.5 add 2 = %v, want 3.5", result[0])
	}
}

// --- String add (concatenation) ---

func TestStringAdd(t *testing.T) {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewString("hello"), NewWord("add"), NewString(" world")})
	_as18, _ := AsString(result[0])
	if _as18 != "hello world" {
		t.Errorf("expected 'hello world', got %s", result[0])
	}
}
