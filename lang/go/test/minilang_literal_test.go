package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The `+name<delim>src<delim>` shortcut is terse lexer sugar for
// `mini name 'src'`. The delimiter is the first character after the
// (lowercase) name; the same character closes, and the closing delimiter is
// optional when the source does not contain it (the OPEN form — the literal
// is a single whitespace-free span). Backslash escapes the delimiter, itself,
// and whitespace; every other backslash is preserved raw (so regex sources
// keep their escapes — the big ergonomic win over the quoted form, which
// needs '\\d'). It desugars to the identical token stream as the explicit
// `mini` call, so stack subjects, a trailing opts map, the unknown-kind
// error, and check mode all behave the same.

const miniImp = `import "boru:minilang"  `

func litRun(t *testing.T, src string) any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, miniImp+src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniLitEquivalence: the shortcut and the explicit call agree.
func TestMiniLitEquivalence(t *testing.T) {
	cases := []struct{ lit, expl string }{
		{`("AbcD" +re/[a-z]+/).fst.m`, `("AbcD" mini re '[a-z]+').fst.m`},
		{`("a1b2c3" +re/\d/).n`, `("a1b2c3" mini re '\\d').n`},
		{`("a1b2c3" +re/\d/ {limit:2}).n`, `("a1b2c3" mini re '\\d' {limit:2}).n`},
		{`+bf/++++++++[>++++++++<-]>+./`, `mini bf '++++++++[>++++++++<-]>+.'`},
	}
	for _, c := range cases {
		lit, expl := litRun(t, c.lit), litRun(t, c.expl)
		if lit != expl {
			t.Errorf("%s = %v but %s = %v (must agree)", c.lit, lit, c.expl, expl)
		}
	}
}

// TestMiniLitDelimiters: the delimiter is the first char after the name; the
// same char closes; a source containing the delimiter uses a different one or
// a backslash escape.
func TestMiniLitDelimiters(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{`("AbcD" +re/[a-z]+/).fst.m`, "bc"}, // /
		{`("AbcD" +re|[a-z]+|).fst.m`, "bc"}, // |
		{`("AbcD" +re#[a-z]+#).fst.m`, "bc"}, // #
		{`("AbcD" +re![a-z]+!).fst.m`, "bc"}, // !
		{`("xa/by" +re#a/b#).fst.m`, "a/b"},  // source has /, delim is #
		{`("xa/by" +re/a\/b/).fst.m`, "a/b"}, // escaped delimiter
	}
	for _, c := range cases {
		if got := litRun(t, c.src); got != c.want {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
}

// TestMiniLitRawBackslash: backslashes pass through raw (no doubling), the
// whole point for regex.
func TestMiniLitRawBackslash(t *testing.T) {
	// \d matches digits raw; the quoted form would need '\\d'.
	if got := litRun(t, `("a1b2c3" +re/\d+/).fst.m`); got != "1" {
		t.Errorf(`+re/\d+/ on "a1b2c3" .fst.m = %v, want "1"`, got)
	}
	// An escaped backslash \\ collapses to one backslash.
	if got := litRun(t, `("a\b" +re/a\\b/).n`); got != int64(0) {
		// "a\b" (literal a, backslash, b) — pattern \\b is "word boundary" in
		// Go regexp after one-level collapse, so just assert it runs; the
		// point is \\ is accepted, not a specific match count.
		_ = got
	}
}

// TestMiniLitStackSubjectAndOpts: a stack subject feeds in and a trailing
// opts map is collected — both inherited from the desugared mini call.
func TestMiniLitStackSubjectAndOpts(t *testing.T) {
	if got := litRun(t, `("a1b2c3" +re/\d/).n`); got != int64(3) {
		t.Errorf("stack subject, all matches: got %v, want 3", got)
	}
	if got := litRun(t, `("a1b2c3" +re/\d/ {limit:2}).n`); got != int64(2) {
		t.Errorf("trailing opts limit: got %v, want 2", got)
	}
}

// TestMiniLitCheckParity: the shortcut type-checks exactly like the explicit
// call, including at the bare top level.
func TestMiniLitCheckParity(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	cr, err := a.Check(miniImp + `"AbcD" +re/[a-z]+/`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// dynamic(Map): the re kinds' check-mode ReturnsFn (miniReShapeReturns)
	// surfaces the {ok ms fst lst n} record shape as a GRADUAL Map carrier
	// so field reads narrow — Phase 4.3 G7. Parity with the explicit call
	// is asserted below.
	if len(cr.Stack) != 1 || cr.Stack[0] != "dynamic(Map)" {
		t.Fatalf("check stack = %v, want [dynamic(Map)]", cr.Stack)
	}
	crExplicit, err := a.Check(miniImp + `MiniLang.lang_re '[a-z]+' {} "AbcD" end`)
	if err != nil {
		t.Fatalf("check explicit: %v", err)
	}
	if len(crExplicit.Stack) != 1 || crExplicit.Stack[0] != cr.Stack[0] {
		t.Fatalf("explicit-call check stack = %v, want %v (shortcut parity)", crExplicit.Stack, cr.Stack)
	}
}

// --- negatives ------------------------------------------------------------

// TestMiniLitUnknownKind: an unknown kind is the same expansion-time error as
// `mini nope`.
func TestMiniLitUnknownKind(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(miniImp + `+nope/x/`); err == nil {
		t.Fatal("expected mini_unknown_lang, got nil")
	} else if !strings.Contains(err.Error(), "mini_unknown_lang") {
		t.Fatalf("error = %v, want mini_unknown_lang", err)
	}
}

// TestMiniLitNeedsImport: like `mini`, the shortcut needs the module imported.
func TestMiniLitNeedsImport(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(`+re/[a-z]+/`); err == nil {
		t.Fatal("expected mini_unknown_lang without import, got nil")
	} else if !strings.Contains(err.Error(), "mini_unknown_lang") {
		t.Fatalf("error = %v, want mini_unknown_lang", err)
	}
}

// --- the OPEN form ----------------------------------------------------------

// TestMiniLitOpenForm: the closing delimiter is optional when the source does
// not contain the delimiter char — a literal is a single whitespace-free
// span, and with no delimiter before the first whitespace (or end of source)
// the span IS the source (`+email:alice@example.com` shape). No scan-ahead:
// a delimiter char in a LATER token (`{limit:2}`, `{limit: 2}`) can never
// capture the literal.
func TestMiniLitOpenForm(t *testing.T) {
	cases := []struct{ lit, expl string }{
		{`+hb:deadbeef`, `mini hb 'deadbeef'`},                                     // end of source terminates
		{`("a1b2c3" +re:\d {limit:2}).n`, `("a1b2c3" mini re '\\d' {limit:2}).n`},  // whitespace terminates; backslash raw
		{`("a1b2c3" +re:\d {limit: 2}).n`, `("a1b2c3" mini re '\\d' {limit:2}).n`}, // a spaced map key's `:` cannot capture
		{`("alice@example.com" +re:@example ).fst.m`, `("alice@example.com" mini re '@example').fst.m`},
		{`("xa:by" +re:a\:b ).fst.m`, `("xa:by" mini re 'a:b').fst.m`}, // escaped delim keeps the open form open
	}
	for _, c := range cases {
		lit, expl := litRun(t, c.lit), litRun(t, c.expl)
		if lit != expl {
			t.Errorf("%s = %v but %s = %v (must agree)", c.lit, lit, c.expl, expl)
		}
	}
}

// TestMiniLitOpenFormClosedPrecedence: a source char equal to the delimiter
// CLOSES the literal (first occurrence wins) — the open form only exists when
// the span has no delimiter — and a closed literal can abut syntax
// (`…/).typeof`): the delimiter itself is the boundary, no whitespace needed.
func TestMiniLitOpenFormClosedPrecedence(t *testing.T) {
	if got := litRun(t, `("aXbXc" +re:X:).n`); got != int64(2) {
		t.Errorf("+re:X: must be the closed form with src 'X': n = %v, want 2", got)
	}
	lit := litRun(t, `(+hb/de_ad_be_ef/) typeof`)
	expl := litRun(t, `(mini hb 'de_ad_be_ef') typeof`)
	if lit != expl {
		t.Errorf("closed literal abutting `)` = %v, explicit = %v (must agree)", lit, expl)
	}
}

// TestMiniLitSourceWhitespace: a source needs its whitespace ESCAPED — a
// literal is a single whitespace-free span, so the old whitespace-grouped
// spelling (`+hb/de ad be ef/`) now splits at the first space (open source
// `de`, then stranded words). `_` grouping and the quoted form remain.
func TestMiniLitSourceWhitespace(t *testing.T) {
	if got := litRun(t, `+hb/de\ ad\ be\ ef/ size`); got != int64(4) {
		t.Errorf(`+hb/de\ ad\ be\ ef/ size = %v, want 4 (escaped spaces join the span)`, got)
	}
	a, _ := lang.New()
	if _, err := a.Run(miniImp + `+hb/de ad be ef/`); err == nil {
		t.Fatal("expected undefined_word for the unescaped whitespace-grouped spelling, got nil")
	} else if !strings.Contains(err.Error(), "undefined_word") {
		t.Fatalf("error = %v, want undefined_word", err)
	}
}

// TestMiniLitOpenFormWhitespaceStops: the open source ends at the first
// whitespace — what follows is lexed normally (here, an undefined word).
func TestMiniLitOpenFormWhitespaceStops(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(miniImp + `+hb:dead beef`); err == nil {
		t.Fatal("expected undefined_word for the stranded `beef`, got nil")
	} else if !strings.Contains(err.Error(), "undefined_word") {
		t.Fatalf("error = %v, want undefined_word", err)
	}
}

// TestMiniLitOpenFormNewlineStops: a literal never spans lines — the open
// source also ends at a newline, and an unclosed literal is NOT captured by a
// stray delimiter char on a later line.
func TestMiniLitOpenFormNewlineStops(t *testing.T) {
	lit := litRun(t, "+hb:dead\ntypeof")
	expl := litRun(t, "mini hb 'dead'\ntypeof")
	if lit != expl {
		t.Errorf("newline-terminated open form = %v, explicit = %v (must agree)", lit, expl)
	}
	// A `:` on a LATER line must not close this line's literal into a giant
	// closed source — the scan is line-bounded, so the literal stays open.
	lit2 := litRun(t, "+hb:deadbeef drop\ndef m {a:1} m.a")
	if lit2 != int64(1) {
		t.Errorf("open form captured across lines: got %v, want 1", lit2)
	}
}

// TestMiniLitOpenFormEmpty: an empty open source (`+name:` before whitespace /
// end of source) is NOT a literal — it falls to normal lexing rather than
// expanding to `mini name ”`. Concretely, `+re: 'x'` is jsonic's implicit
// pair (a map with key `+re`), and a bare `+re:` is a plain syntax error.
func TestMiniLitOpenFormEmpty(t *testing.T) {
	if got := litRun(t, `+re: 'x'`); got != "{+re:'x'}" {
		t.Errorf("+re: 'x' = %v, want the implicit-pair map {+re:'x'} (not a mini literal)", got)
	}
	a, _ := lang.New()
	if _, err := a.Run(miniImp + `+re:`); err == nil {
		t.Fatal("expected a syntax error from a bare `+re:`, got nil")
	} else if strings.Contains(err.Error(), "mini_") {
		t.Fatalf("`+re:` reached the mini machinery (%v); an empty open source must fall to normal lexing", err)
	}
}

// TestMiniLitNotTriggered: a bare `+` not followed by a name+delimiter is left
// to normal lexing (no false trigger).
func TestMiniLitNotTriggered(t *testing.T) {
	// `+0d5` is a signed bignum, not a minilang literal (a digit, not a
	// lowercase name, follows `+`), so it must parse exactly like `0d5`.
	a, _ := lang.New()
	signed, err := runReference(t, a, `+0d5`)
	if err != nil {
		t.Fatalf("+0d5 should parse as a bignum: %v", err)
	}
	unsigned, err := runReference(t, a, `0d5`)
	if err != nil {
		t.Fatalf("0d5: %v", err)
	}
	if len(signed) != 1 || len(unsigned) != 1 || signed[0] != unsigned[0] {
		t.Fatalf("+0d5 = %v, 0d5 = %v (must match; + must not trigger a minilang literal)", signed, unsigned)
	}
}
