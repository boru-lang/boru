package core

import (
	"math/big"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// TestCoerceBooleanBigLeaves pins NUR055's fix: the arbitrary-precision
// leaves are truthy by VALUE, not uniformly false. CoerceBoolean's Number
// arm used to read AsNumber's compile failure-zero as a real magnitude, so every
// BigInteger and BigDecimal coerced to false while `0d1 eq 1` was true.
func TestCoerceBooleanBigLeaves(t *testing.T) {
	bigInt := func(s string) Value {
		n, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("SetString(%q)", s)
		}
		return Value{Parent: TBigInteger, Data: BigIntPayload{N: n}}
	}
	bigDec := func(s string) Value {
		d, _, err := apd.NewFromString(s)
		if err != nil {
			t.Fatalf("NewFromString(%q): %v", s, err)
		}
		return Value{Parent: TBigDecimal, Data: DecimalPayload{D: d}}
	}

	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"BigInteger zero", bigInt("0"), false},
		{"BigInteger one", bigInt("1"), true},
		{"BigInteger negative", bigInt("-5"), true},
		{"BigInteger huge", bigInt("100000000000000000000000000"), true},
		{"BigDecimal zero", bigDec("0"), false},
		{"BigDecimal zero with scale", bigDec("0.000"), false},
		{"BigDecimal fraction", bigDec("1.5"), true},
		{"BigDecimal negative", bigDec("-0.25"), true},
		// AsFloatApprox would flatten this to 0.0; the exact test must not.
		{"BigDecimal underflowing float64", bigDec("1e-400"), true},
	}
	for _, c := range cases {
		if got := CoerceBoolean(c.v); got != c.want {
			t.Errorf("%s: CoerceBoolean = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestBigNumIsZeroUnreadablePayload covers the defensive arms: a value
// tagged as a Big leaf whose payload does not match cannot be proven zero,
// so it stays truthy — the same direction the non-Big arm takes for an
// unreadable payload. Reachable only by hand-constructing a mismatched
// Value, which is exactly what a host plugin could hand the kernel.
func TestBigNumIsZeroUnreadablePayload(t *testing.T) {
	badInt := Value{Parent: TBigInteger, Data: IntPayload{N: 0}}
	if bigNumIsZero(badInt) {
		t.Error("BigInteger with a non-BigInt payload must not report zero")
	}
	if !CoerceBoolean(badInt) {
		t.Error("…and must stay truthy")
	}

	badDec := Value{Parent: TBigDecimal, Data: IntPayload{N: 0}}
	if bigNumIsZero(badDec) {
		t.Error("BigDecimal with a non-Decimal payload must not report zero")
	}

	nilInt := Value{Parent: TBigInteger, Data: BigIntPayload{N: nil}}
	if bigNumIsZero(nilInt) {
		t.Error("BigInteger with a nil *big.Int must not report zero")
	}

	nilDec := Value{Parent: TBigDecimal, Data: DecimalPayload{D: nil}}
	if bigNumIsZero(nilDec) {
		t.Error("BigDecimal with a nil *apd.Decimal must not report zero")
	}
}
