package parser

import (
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- shared wave3 helpers ---

// parseWave3 parses src, failing the test if Parse panics: every input —
// well-formed or malformed — must produce values or a clean error.
func parseWave3(t *testing.T, src string) (vals []core.Value, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Parse(%q) panicked: %v", src, r)
		}
	}()
	return Parse(src)
}

// mustParseWave3 parses src and fails on error, returning the values.
func mustParseWave3(t *testing.T, src string) []core.Value {
	t.Helper()
	vals, err := parseWave3(t, src)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", src, err)
	}
	return vals
}

// wantParseErrWave3 asserts src FAILS to parse (never panics) and that the
// error message contains sub (empty sub = any error).
func wantParseErrWave3(t *testing.T, src, sub string) {
	t.Helper()
	_, err := parseWave3(t, src)
	if err == nil {
		t.Fatalf("Parse(%q): expected an error, got none", src)
	}
	if sub != "" && !strings.Contains(err.Error(), sub) {
		t.Errorf("Parse(%q): error %q does not contain %q", src, err.Error(), sub)
	}
}

// wordNameWave3 asserts v is a Word and returns its info.
func wordNameWave3(t *testing.T, v core.Value, ctx string) core.WordInfo {
	t.Helper()
	if !core.IsWord(&v) {
		t.Fatalf("%s: expected a Word, got %s", ctx, v.String())
	}
	w, err := core.AsWord(&v)
	if err != nil {
		t.Fatalf("%s: AsWord: %v", ctx, err)
	}
	return w
}

// --- reserved literals ---

func TestParseWave3NoneLiteral(t *testing.T) {
	vals := mustParseWave3(t, "none")
	if len(vals) != 1 || !core.IsNone(&vals[0]) {
		t.Fatalf("none: expected the unique None value, got %v", vals)
	}
}

func TestParseWave3FloatSpecialLiterals(t *testing.T) {
	cases := []struct {
		src   string
		check func(f float64) bool
	}{
		{"inf", func(f float64) bool { return math.IsInf(f, 1) }},
		{"-inf", func(f float64) bool { return math.IsInf(f, -1) }},
		{"nan", math.IsNaN},
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", c.src, len(vals))
		}
		f, err := core.AsFloat(vals[0])
		if err != nil {
			t.Fatalf("%q: AsFloat: %v", c.src, err)
		}
		if !c.check(f) {
			t.Errorf("%q: got %v, wrong special float", c.src, f)
		}
	}
}

// --- marker tokens ---

func TestParseWave3MarkerTokens(t *testing.T) {
	// `?` and `|` become plain marker words; a bare `)` becomes the
	// close-paren marker so the engine can report the mismatch at runtime.
	for _, c := range []struct{ src, word string }{{"?", "?"}, {"|", "|"}} {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", c.src, len(vals))
		}
		if w := wordNameWave3(t, vals[0], c.src); w.Name != c.word {
			t.Errorf("%q: word %q, want %q", c.src, w.Name, c.word)
		}
	}
	vals := mustParseWave3(t, ")")
	if len(vals) != 1 || !core.IsCloseParen(&vals[0]) {
		t.Fatalf("bare `)`: expected a close-paren marker, got %v", vals)
	}
	// A trailing `!` with no following `.` stays a bare `!` word.
	vals = mustParseWave3(t, "5 !")
	if len(vals) != 2 {
		t.Fatalf("5 !: got %d values, want 2", len(vals))
	}
	if w := wordNameWave3(t, vals[1], "5 !"); w.Name != "!" {
		t.Errorf("5 !: second value word %q, want !", w.Name)
	}
}

// --- word modifiers ---

func TestParseWave3TypeBoundSugar(t *testing.T) {
	// `Map/t` parses to one type-bound sugar marker; the engine lowers
	// it to the bounded-Type application (ADR-012 rule 3 amendment).
	vals := mustParseWave3(t, "Map/t")
	if len(vals) != 1 {
		t.Fatalf("Map/t: expected one value, got %v", vals)
	}
	info, ok := core.AsSugar(vals[0])
	if !ok || info.Kind != core.SugarTypeBound {
		t.Fatalf("Map/t: expected a type-bound sugar marker, got %v", vals[0])
	}
}

func TestParseWave3UsurpAndRefWords(t *testing.T) {
	cases := []struct {
		src        string
		usurp, ref bool
	}{
		{"foo/u", true, false},
		{"foo/uv", true, true},
		{"foo/v", false, true},
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", c.src, len(vals))
		}
		w := wordNameWave3(t, vals[0], c.src)
		if w.Name != "foo" || w.ForceUsurp != c.usurp || w.ForceVal != c.ref {
			t.Errorf("%q: got name=%q usurp=%v ref=%v, want foo/%v/%v",
				c.src, w.Name, w.ForceUsurp, w.ForceVal, c.usurp, c.ref)
		}
	}
}

// TestParseWave3InvalidModifierCombos pins the scanWordModifier rejection
// rules post-NUR027: an invalid combination spelled entirely from the
// modifier alphabet is a loud parse error (never a partial parse, never
// a panic); a suffix containing a non-modifier character keeps the whole
// token as a plain word (the slash-name / type-path reading).
func TestParseWave3InvalidModifierCombos(t *testing.T) {
	for _, src := range []string{
		"a/1f2", // second digit run
		"a/vv",  // repeated r
		"a/qv",  // q + r are mutually exclusive
		"a/qu",  // q + u are mutually exclusive
		"a/uu",  // repeated u
		"a/uq",  // u + q (other order)
		"a/tt",  // repeated t
		"a/tq",  // t combines with nothing
		"a/qt",  // t after q
		"a/tf",  // t + f
		"a/2t",  // t + argCount
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("%q: an all-alphabet invalid modifier combo must be a parse error", src)
		}
	}
	// Non-alphabet letters keep the historical plain-word demotion.
	for _, src := range []string{"a/zz", "a/qz"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		w := wordNameWave3(t, vals[0], src)
		if w.Name != src {
			t.Errorf("%q: a non-alphabet suffix must keep the whole token as the word name, got %q", src, w.Name)
		}
		if w.ForceUsurp || w.ForceVal || w.ForceStack || w.ForceForward {
			t.Errorf("%q: a non-alphabet suffix must not set any flag: %+v", src, w)
		}
	}
}

// TestParseWave3WordContextTypePath pins that a full builtin type path in
// WORD context stays an opaque Word carrying the whole path (ADR-012
// rule 4 — the `/` suffix is not a valid modifier, so the token stays
// intact; the engine's cascade resolves the path at consumption).
func TestParseWave3WordContextTypePath(t *testing.T) {
	vals := mustParseWave3(t, "Scalar/Number/Integer")
	if len(vals) != 1 {
		t.Fatalf("Scalar/Number/Integer: expected one value, got %v", vals)
	}
	if w, err := core.AsWord(&vals[0]); err != nil || w.Name != "Scalar/Number/Integer" {
		t.Fatalf("Scalar/Number/Integer: expected word(Scalar/Number/Integer), got %v", vals)
	}
}

// TestParseWave3HugeArityModifier pins that an argCount too large for int
// is a loud parse error (NUR027 — an all-digit suffix is a modifier
// attempt, and overflowing arity previously demoted silently to the
// word "a/99999…" and an obscure undefined_word).
func TestParseWave3HugeArityModifier(t *testing.T) {
	if _, err := Parse("a/99999999999999999999"); err == nil {
		t.Error("an int-overflowing argCount modifier must be a parse error")
	}
}

// --- numeric literal edges ---

func TestParseWave3NumberEdges(t *testing.T) {
	// 0e1 is scientific notation, not base-prefixed: value 0 (Integer).
	vals := mustParseWave3(t, "0e1")
	if len(vals) != 1 {
		t.Fatalf("0e1: got %d values, want 1", len(vals))
	}
	if n, err := core.AsInteger(vals[0]); err != nil || n != 0 {
		t.Errorf("0e1: got %v (err %v), want Integer 0", vals[0], err)
	}
	// int64 min in hex is exactly representable; jsonic cannot lex it as a
	// number so it reaches parseWord as text and parses exactly.
	vals = mustParseWave3(t, "-0x8000000000000000")
	if len(vals) != 1 {
		t.Fatalf("-0x8000000000000000: got %d values, want 1", len(vals))
	}
	if n, err := core.AsInteger(vals[0]); err != nil || n != math.MinInt64 {
		t.Errorf("-0x8000000000000000: got %v (err %v), want int64 min", vals[0], err)
	}
}

func TestParseWave3NumberEdgeErrors(t *testing.T) {
	cases := []struct{ src, code string }{
		{"0x8000000000000000", "integer_overflow"},       // int64 max + 1
		{"0xFFFFFFFFFFFFFFFF", "integer_overflow"},       // 2^64-1, hex text path
		{"0o2000000000000000000000", "integer_overflow"}, // 2^63 in octal
		{"0xZZ", "syntax_error"},                         // invalid hex digits
		{"0x1__2", "syntax_error"},                       // misplaced separator
		{"0xFFFFFFFFFFFFFFF__F", "syntax_error"},         // misplaced separator, text path
		{"1e309", "float_overflow"},                      // overflows binary64
	}
	for _, c := range cases {
		_, err := parseWave3(t, c.src)
		if err == nil {
			t.Errorf("Parse(%q): expected [boru/%s], got nil", c.src, c.code)
			continue
		}
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != c.code {
			t.Errorf("Parse(%q): expected [boru/%s], got %v", c.src, c.code, err)
		}
	}
}

func TestParseWave3BigNumEdges(t *testing.T) {
	// Signed exponents lex as one 0d run and build exact BigDecimals.
	cases := map[string]string{
		"0d1e+3": "0d1000",
		"0d1e-3": "0d0.001",
	}
	for src, want := range cases {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !vals[0].Parent().ConformsTo(core.TBigDecimal) {
			t.Fatalf("%q: expected one BigDecimal, got %v", src, vals)
		}
		if got := vals[0].String(); got != want {
			t.Errorf("%q: got %s, want %s", src, got, want)
		}
	}
	// `+0d5`: a digit after `+` is not a minilang name start, so the
	// big-number matcher claims the signed literal.
	vals := mustParseWave3(t, "+0d5")
	if len(vals) != 1 || !vals[0].Parent().ConformsTo(core.TBigInteger) {
		t.Fatalf("+0d5: expected one BigInteger, got %v", vals)
	}
	n, _ := core.AsBigInteger(vals[0])
	if n.Int64() != 5 {
		t.Errorf("+0d5: got %s, want 5", n)
	}
}

func TestParseWave3BigNumMalformed(t *testing.T) {
	// A `0d` prefix with a non-digit body is a clean 0d syntax error.
	for _, src := range []string{"0dx", "0de", "0d1_"} {
		_, err := parseWave3(t, src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "syntax_error" {
			t.Errorf("Parse(%q): expected [boru/syntax_error], got %v", src, err)
		}
	}
	// `0d.` — the matcher declines (no digit), the residue is a malformed
	// digit-led token.
	wantParseErrWave3(t, "0d.", "invalid numeric literal")
}

// TestNumberClassifiersWave3 pins the numeric-shape classifier edges that
// no lexed token can produce (jsonic never emits an empty or sign-only
// number token) but that the predicates must still reject.
func TestNumberClassifiersWave3(t *testing.T) {
	for _, src := range []string{"", "-", "+", "1x"} {
		if isPlainDecimalInteger(src) {
			t.Errorf("isPlainDecimalInteger(%q) = true, want false", src)
		}
	}
	for _, src := range []string{"1_0", "-42", "+7"} {
		if !isPlainDecimalInteger(src) {
			t.Errorf("isPlainDecimalInteger(%q) = false, want true", src)
		}
	}
}

// --- resolveTextValue (data-context bare text classifier) ---

func TestResolveTextValueWave3Specials(t *testing.T) {
	for src, check := range map[string]func(f float64) bool{
		"inf":  func(f float64) bool { return math.IsInf(f, 1) },
		"-inf": func(f float64) bool { return math.IsInf(f, -1) },
		"nan":  math.IsNaN,
	} {
		v := resolveTextValue(src)
		f, err := core.AsFloat(v)
		if err != nil || !check(f) {
			t.Errorf("resolveTextValue(%q) = %v (err %v), wrong special float", src, v, err)
		}
	}
	// A full builtin type PATH is data here, an Atom — the parser is
	// type-name-opaque (ADR-012 rule 4).
	v := resolveTextValue("Scalar/Number/Integer")
	if s, err := core.AsAtom(v); err != nil || s != "Scalar/Number/Integer" {
		t.Errorf("resolveTextValue(Scalar/Number/Integer) = %v, want Atom(Scalar/Number/Integer)", v)
	}
}

// --- top-level implicit map ---

func TestParseWave3TopLevelImplicitMapAutoEval(t *testing.T) {
	// A whole-input implicit map (`a:1`) must be marked Eval so value
	// expressions resolve at run time.
	vals := mustParseWave3(t, "a:1")
	if len(vals) != 1 {
		t.Fatalf("a:1: got %d values, want 1", len(vals))
	}
	if !vals[0].Parent().ConformsTo(core.TMap) {
		t.Fatalf("a:1: expected a Map, got %s", vals[0].String())
	}
	if !core.IsEval(&vals[0]) {
		t.Errorf("a:1: top-level implicit map must have Eval=true")
	}
}
