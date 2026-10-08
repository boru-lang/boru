package native

import (
	"math"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// W9_nativeB — coverage for native_type.go convert* handlers and helpers.
// Drives the scalar-conversion error/edge arms directly (crafted sealed
// values), pairing positive results with the rejected shapes. See
// design/TEST-SEAMS.10.md.

func w9BigDec(t *testing.T, s string) Value {
	t.Helper()
	d, _, err := apd.NewFromString(s)
	if err != nil {
		t.Fatalf("apd parse %q: %v", s, err)
	}
	return NewBigDecimal(d)
}

// --- convertTo direct arms ---

func TestW9ConvertToFloatFromHugeBigDecimal(t *testing.T) {
	// AsFloatApprox errors: a BigDecimal beyond float64 range.
	if _, err := convertTo(w9BigDec(t, "1e100000"), TFloat, ""); err == nil {
		t.Fatal("convert huge BigDecimal to Float must error")
	}
	// Positive pair: a small BigDecimal converts fine.
	out, err := convertTo(w9BigDec(t, "1.5"), TFloat, "")
	if err != nil {
		t.Fatalf("convert 1.5 to Float: %v", err)
	}
	if f, _ := AsFloat(out); f != 1.5 {
		t.Fatalf("want 1.5, got %v", f)
	}
}

func TestW9ConvertToIntegerFromNaNBigDecimal(t *testing.T) {
	// bigDecimalToBigIntTrunc errors on a NaN BigDecimal (SetString fails).
	if _, err := convertTo(w9BigDec(t, "NaN"), TInteger, ""); err == nil {
		t.Fatal("convert NaN BigDecimal to Integer must error")
	}
}

func TestW9ConvertToIntegerFromInteger(t *testing.T) {
	// Integer passthrough (base "" fast path).
	out, err := convertTo(NewInteger(5), TInteger, "")
	if err != nil {
		t.Fatalf("convert Integer→Integer: %v", err)
	}
	if n, _ := AsInteger(out); n != 5 {
		t.Fatalf("want 5, got %d", n)
	}
}

func TestW9ConvertToIntegerFloatOverflow(t *testing.T) {
	for _, f := range []float64{math.NaN(), math.Inf(1), 1e30} {
		if _, err := convertTo(NewFloat(f), TInteger, ""); err == nil {
			t.Fatalf("convert Float %v to Integer must overflow-error", f)
		}
	}
	// Positive pair: an in-range float truncates.
	out, err := convertTo(NewFloat(3.9), TInteger, "")
	if err != nil {
		t.Fatalf("convert 3.9 to Integer: %v", err)
	}
	if n, _ := AsInteger(out); n != 3 {
		t.Fatalf("want 3, got %d", n)
	}
}

func TestW9ConvertToBooleanDefaultArm(t *testing.T) {
	// convert Boolean is pure CoerceBoolean coercion; a non-empty Atom
	// source coerces to a Boolean (true) by presence.
	out, err := convertTo(NewAtom("x"), TBoolean, "")
	if err != nil {
		t.Fatalf("convert Atom to Boolean: %v", err)
	}
	if !out.Parent.Equal(TBoolean) {
		t.Fatalf("want Boolean, got %s", out.Parent)
	}
}

func TestW9ConvertToUnsupportedTarget(t *testing.T) {
	// A target the switch does not handle → unsupported-target error.
	if _, err := convertTo(NewInteger(1), TFunction, ""); err == nil {
		t.Fatal("convert to Function must be unsupported")
	}
}

// --- convertToBigInteger ---

func TestW9ConvertToBigIntegerBigDecimalTruncError(t *testing.T) {
	if _, err := convertToBigInteger(w9BigDec(t, "NaN"), ""); err == nil {
		t.Fatal("convertToBigInteger NaN must error")
	}
}

func TestW9ConvertToBigIntegerStringBases(t *testing.T) {
	cases := []struct {
		text, base string
		want       int64
	}{
		{"ff", "hex", 255},
		{"101", "bin", 5},
		{"17", "oct", 15},
	}
	for _, c := range cases {
		out, err := convertToBigInteger(NewString(c.text), c.base)
		if err != nil {
			t.Fatalf("convertToBigInteger %q base %q: %v", c.text, c.base, err)
		}
		n, _ := AsBigInteger(out)
		if n.Int64() != c.want {
			t.Fatalf("%q base %q: want %d, got %s", c.text, c.base, c.want, n)
		}
	}
	// Negative: unknown base rejected.
	if _, err := convertToBigInteger(NewString("5"), "zonk"); err == nil {
		t.Fatal("convertToBigInteger unknown base must error")
	}
}

// --- convertToBigDecimal ---

func TestW9ConvertToBigDecimalIdentity(t *testing.T) {
	out, err := convertToBigDecimal(w9BigDec(t, "2.5"), "")
	if err != nil {
		t.Fatalf("convertToBigDecimal identity: %v", err)
	}
	if !out.Parent.ConformsTo(TBigDecimal) {
		t.Fatalf("want BigDecimal, got %s", out.Parent)
	}
}

// --- bigDecimalToBigIntTrunc ---

func TestW9BigDecimalToBigIntTruncNonBigDecimal(t *testing.T) {
	if _, err := bigDecimalToBigIntTrunc(NewInteger(5)); err == nil {
		t.Fatal("bigDecimalToBigIntTrunc of a non-BigDecimal must error")
	}
}

func TestW9BigDecimalToBigIntTruncNegHalf(t *testing.T) {
	// "-0.5" truncates its fraction to "-", normalised to "0".
	n, err := bigDecimalToBigIntTrunc(w9BigDec(t, "-0.5"))
	if err != nil {
		t.Fatalf("bigDecimalToBigIntTrunc -0.5: %v", err)
	}
	if n.Sign() != 0 {
		t.Fatalf("want 0, got %s", n)
	}
}

func TestW9BigDecimalToBigIntTruncNaN(t *testing.T) {
	// NaN text has no integer form → SetString fails.
	if _, err := bigDecimalToBigIntTrunc(w9BigDec(t, "NaN")); err == nil {
		t.Fatal("bigDecimalToBigIntTrunc NaN must error")
	}
}

// --- convertIdealHandler ---

func TestW9ConvertIdealHandlerDefaultTarget(t *testing.T) {
	r := seam5Reg(t)
	// A bare Node literal is neither Map nor List → default rejection.
	_, err := convertIdealHandler([]Value{NewTypeLiteral(TNode), NewMap(NewOrderedMap())}, nil, nil, r)
	if err == nil {
		t.Fatal("convert Node <ideal> must reject (only Map or List)")
	}
	if !strings.Contains(err.Error(), "Map or List") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- convert2Handler / convert3Handler concrete-first-arg guards ---

func TestW9Convert2HandlerConcreteFirstArg(t *testing.T) {
	r := seam5Reg(t)
	if _, err := convert2Handler([]Value{NewInteger(5), NewInteger(6)}, nil, nil, r); err == nil {
		t.Fatal("convert2: concrete first arg (not a type literal) must error")
	}
}

func TestW9Convert3HandlerConcreteFirstArg(t *testing.T) {
	r := seam5Reg(t)
	opts := NewMap(NewOrderedMap())
	if _, err := convert3Handler([]Value{NewInteger(5), opts, NewInteger(6)}, nil, nil, r); err == nil {
		t.Fatal("convert3: concrete first arg (not a type literal) must error")
	}
}

// --- convert Boolean {truthy: true} — YAML-style parse, then fallback ---

func TestW9YamlTruthyTokens(t *testing.T) {
	for _, s := range []string{"y", "yes", "true", "on", "YES", "On", "  true  "} {
		if v, ok := yamlTruthy(s); !ok || !v {
			t.Errorf("yamlTruthy(%q) = (%v,%v), want (true,true)", s, v, ok)
		}
	}
	for _, s := range []string{"n", "no", "false", "off", "NO", "Off", "  off  "} {
		if v, ok := yamlTruthy(s); !ok || v {
			t.Errorf("yamlTruthy(%q) = (%v,%v), want (false,true)", s, v, ok)
		}
	}
	// Negative: not a YAML boolean token → ok=false (caller falls back).
	for _, s := range []string{"maybe", "", "2", "yep", "0"} {
		if _, ok := yamlTruthy(s); ok {
			t.Errorf("yamlTruthy(%q) ok = true, want false (not a token)", s)
		}
	}
}

func TestW9CoerceBooleanTruthy(t *testing.T) {
	cases := []struct {
		in   Value
		want bool
		why  string
	}{
		{NewString("yes"), true, "YAML true token"},
		{NewString("no"), false, "YAML false token"},
		{NewString("OFF"), false, "case-insensitive false token"},
		{NewString("maybe"), true, "non-token String falls back to presence (non-empty)"},
		{NewString(""), false, "empty String falls back to false"},
		{NewInteger(1), true, "non-String source uses ordinary coercion"},
		{NewInteger(0), false, "zero is false via ordinary coercion"},
		{NewBoolean(false), false, "a Boolean source passes through"},
	}
	for _, c := range cases {
		if got := coerceBooleanTruthy(c.in); got != c.want {
			t.Errorf("coerceBooleanTruthy(%s) = %v, want %v (%s)", c.in.String(), got, c.want, c.why)
		}
	}
}

func TestW9Convert3HandlerTruthy(t *testing.T) {
	r := seam5Reg(t)
	optsTruthy := func(b bool) Value {
		m := NewOrderedMap()
		m.Set("truthy", NewBoolean(b))
		return NewMap(m)
	}
	// truthy:true + Boolean target: YAML token 'no' → false.
	out, err := convert3Handler([]Value{NewTypeLiteral(TBoolean), optsTruthy(true), NewString("no")}, nil, nil, r)
	if err != nil {
		t.Fatalf("convert3 truthy 'no': %v", err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Errorf("convert Boolean {truthy:true} 'no' = %v, want false", out[0])
	}
	// truthy:true + Boolean target: non-token 'maybe' → presence fallback → true.
	out, err = convert3Handler([]Value{NewTypeLiteral(TBoolean), optsTruthy(true), NewString("maybe")}, nil, nil, r)
	if err != nil {
		t.Fatalf("convert3 truthy 'maybe': %v", err)
	}
	if b, _ := AsBoolean(out[0]); !b {
		t.Errorf("convert Boolean {truthy:true} 'maybe' = %v, want true", out[0])
	}
	// truthy:true but a NON-Boolean target: the option is inert (falls to convertTo).
	out, err = convert3Handler([]Value{NewTypeLiteral(TString), optsTruthy(true), NewString("no")}, nil, nil, r)
	if err != nil {
		t.Fatalf("convert3 truthy String target: %v", err)
	}
	if s := ValToString(out[0]); s != "no" {
		t.Errorf("convert String {truthy:true} 'no' = %q, want \"no\"", s)
	}
}

// TestConvertBoolOptsHandlerArms drives convertBoolOptsHandler's guard arms
// directly, because the spec rows can only reach the validating path: at run
// time a matched `[Boolean Map Any]` call always arrives with three args and
// a CONCRETE map, so the defensive arms have no source-level spelling.
//
// The handler exists because the Boolean form's source slot is `Any`
// (NUR053), which makes a pattern on the options slot a trap — a malformed
// map would fall back to the 2-arg row and be coerced AS THE SOURCE. These
// cases pin that it validates when it can and DELEGATES otherwise, which is
// what keeps the repair independent of signature sort order.
func TestConvertBoolOptsHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// Short arg window: DECLINED, not delegated. convert3Handler indexes
	// args[2] unconditionally, so delegating here would panic — and this
	// codebase does not permit a panic on any input (ADR-005).
	if _, err := convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("a short arg window must be declined, not delegated into an indexing handler")
	}

	// A non-concrete options slot (a Map CARRIER, as check mode produces):
	// nothing to inspect, so it must delegate rather than guess.
	if _, err := convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), NewCarrier(TMap), NewString("x")}, nil, nil, r); err != nil {
		t.Fatalf("carrier options slot must delegate, got %v", err)
	}

	// A concrete NON-map in the slot (AsMap declines): same, delegate.
	if _, err := convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), NewInteger(1), NewString("x")}, nil, nil, r); err != nil {
		t.Fatalf("non-map options slot must delegate, got %v", err)
	}

	// An unknown key is declined by NAME — the whole point of the handler.
	_, err := convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), optsKV("truthyy", NewBoolean(true)), NewString("no")}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "unknown option key truthyy") {
		t.Fatalf("unknown key must be declined by name, got %v", err)
	}

	// A known key with the wrong value type is declined by TYPE.
	_, err = convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), optsKV("truthy", NewString("yes")), NewString("no")}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "option truthy expects Boolean") {
		t.Fatalf("wrong-typed option must be declined by type, got %v", err)
	}

	// A known key explicitly set to `none` means "not supplied" (the
	// pattern's `tor None` arm) — accepted, and presence coercion applies.
	out, err := convertBoolOptsHandler(
		[]Value{NewTypeLiteral(TBoolean), optsKV("truthy", NewTypeLiteral(TNone)), NewString("no")}, nil, nil, r)
	if err != nil {
		t.Fatalf("none-valued option must be accepted: %v", err)
	}
	if b, _ := AsBoolean(out[0]); !b {
		t.Errorf(`convert Boolean {truthy:none} 'no' = %v, want true (presence)`, out[0])
	}
}

// optsKV builds a one-entry options map for the arms above.
func optsKV(k string, v Value) Value {
	m := NewOrderedMap()
	m.Set(k, v)
	return NewMap(m)
}
