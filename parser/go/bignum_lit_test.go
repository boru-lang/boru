package parser

import (
	"math/big"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestParseBigIntegerLiteral pins that `0d…` (no dot/exponent) parses to a
// single BigInteger value with the exact magnitude (incl. above int64).
func TestParseBigIntegerLiteral(t *testing.T) {
	cases := map[string]string{
		"0d0":                                "0",
		"0d123":                              "123",
		"-0d5":                               "-5",
		"+0d7":                               "7",
		"0d1_000_000":                        "1000000",
		"0d99999999999999999999999999999999": "99999999999999999999999999999999",
	}
	for src, want := range cases {
		got, err := Parse(src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
			continue
		}
		if len(got) != 1 || !core.ParentOf(&got[0]).ConformsTo(core.TBigInteger) {
			t.Errorf("Parse(%q): expected one BigInteger, got %v", src, got)
			continue
		}
		n, _ := core.AsBigInteger(got[0])
		if n.Cmp(mustBig(want)) != 0 {
			t.Errorf("Parse(%q): got %s, want %s", src, n, want)
		}
	}
}

// TestParseBigDecimalLiteralDotSplit is the key regression: jsonic splits
// `0d12.5` into `0d12` · `.` · `5` unless the lexer matcher claims the
// whole run. Assert the dotted (and exponent) forms parse to ONE
// BigDecimal — and that `0d5.foo` still leaves the dot for member access.
func TestParseBigDecimalLiteralDotSplit(t *testing.T) {
	for _, src := range []string{"0d12.5", "0d0.10", "0d1.5e3", "0d1e3", "-0d0.5"} {
		got, err := Parse(src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
			continue
		}
		if len(got) != 1 || !core.ParentOf(&got[0]).ConformsTo(core.TBigDecimal) {
			t.Errorf("Parse(%q): expected one BigDecimal, got %v", src, got)
		}
	}
	// `0d5.foo`: the matcher consumes only `0d5` (the `.` is not followed
	// by a digit), leaving `.foo` as member access. The chain folds into a
	// single Reach value (NOT a malformed BigDecimal, which would error) —
	// so it parses cleanly and is NOT a number.
	got, err := Parse("0d5.foo")
	if err != nil {
		t.Fatalf("Parse(0d5.foo) error: %v", err)
	}
	if len(got) != 1 || core.ParentOf(&got[0]).ConformsTo(core.TNumber) {
		t.Errorf("0d5.foo: expected one non-number (reach) value, got %v", got)
	}
	if got[0].String() != "0d5.foo" {
		t.Errorf("0d5.foo: expected the dotted member-access form, got %q", got[0].String())
	}
}

// TestParseBigNumberMalformed pins clear syntax errors for malformed 0d.
func TestParseBigNumberMalformed(t *testing.T) {
	for _, src := range []string{"0d", "0d1__0"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "syntax_error" {
			t.Errorf("Parse(%q): expected [boru/syntax_error], got %v", src, err)
		}
	}
}

func mustBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad test bigint: " + s)
	}
	return n
}
