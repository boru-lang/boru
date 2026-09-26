package parser

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestTemplateWave3Escapes pins every escape sequence in template literal
// text. The vocabulary is the QUOTED-STRING one (NUR026): the control
// characters, \xNN, \uNNNN, and an unknown escape yielding the bare
// character. `a\zb` was `a\zb` before the fix and is `azb` now — the
// behaviour change the record asked to pin, in both directions.
func TestTemplateWave3Escapes(t *testing.T) {
	cases := []struct{ src, want string }{
		{"`a\\nb`", "a\nb"},
		{"`a\\tb`", "a\tb"},
		{"`a\\rb`", "a\rb"},
		{"`a\\\\b`", "a\\b"},
		{"`a\\`b`", "a`b"},
		{"`a\\$b`", "a$b"},
		{"`a\\zb`", "azb"},                 // unknown escape: backslash DROPPED (was `a\zb`)
		{"`a\\0b`", "a0b"},                 // \0 is the digit, exactly as in "a\0b"
		{"`a\\bb`", "a\bb"},                // newly live: backspace
		{"`a\\fb`", "a\fb"},                // newly live: formfeed
		{"`a\\vb`", "a\vb"},                // newly live: vertical tab
		{"`a\\x41b`", "aAb"},               // newly live: two-hex-digit byte
		{"`a\\u0041b`", "aAb"},             // newly live: four-hex-digit rune
		{"`a\\u{41}b`", "aAb"},             // the braced form, as in "a\u{41}b" (NUR026)
		{"`\\ud83d\\ude00`", "\U0001F600"}, // a surrogate pair is one code point (NUR026)
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", c.src, len(vals))
		}
		s, err := core.AsString(vals[0])
		if err != nil {
			t.Fatalf("%q: AsString: %v", c.src, err)
		}
		if s != c.want {
			t.Errorf("%q: got %q, want %q", c.src, s, c.want)
		}
	}
}

// TestTemplateWave3MalformedEscapes pins NUR026's close: a malformed `\x` /
// `\u` escape in a template is refused with the quoted string's report —
// the escape itself named — where it used to read literally.
func TestTemplateWave3MalformedEscapes(t *testing.T) {
	for src, want := range map[string]string{
		"`a\\xZZb`":      "invalid ascii escape: \\xZZ",
		"`a\\u00b`":      "invalid unicode escape: \\u00b",
		"`a\\u{110000}`": "invalid unicode escape: \\u{110000}",
		"\"a\\xZZb\"":    "invalid ascii escape: \\xZZ",
		"\"a\\x4\"":      "invalid ascii escape: \\x4",
	} {
		wantParseErrWave3(t, src, want)
	}
}

// TestProcessTemplateEscapesWave3 covers the escape processor directly,
// including the corner the lexer can't easily produce: a trailing backslash.
func TestProcessTemplateEscapesWave3(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		`a\nb\tc\rd`: "a\nb\tc\rd",
		`a\\b`:       `a\b`,
		"a\\`b":      "a`b",
		`a\$b`:       "a$b",
		`a\qb`:       `aqb`, // unknown escape: bare character (NUR026)
		`a\x41b`:     "aAb",
		`a\u0041b`:   "aAb",
		// Hex LETTERS in both cases — digits alone leave parseHexEscape's
		// a-f / A-F branches unexercised.
		`a\x4ab`:   "aJb",
		`a\x4Ab`:   "aJb",
		`a\u00e9b`: "a\u00e9b",
		`a\u00E9b`: "a\u00e9b",
		// The braced form, and a surrogate pair split across two escapes —
		// one code point, a lone surrogate U+FFFD (NUR026).
		`a\u{41}b`:     "aAb",
		`\u{1F600}`:    "\U0001F600",
		`\ud83d\ude00`: "\U0001F600",
		`\ud83d\u0041`: "\uFFFDA",
		// Short runs and non-hex digits fall back to the literal reading —
		// the helper's own contract; the lexer refuses such an escape
		// before it gets here (escapeFault, NUR026).
		`a\x4`:     "ax4",
		`a\u004`:   "au004",
		`a\u{zz}b`: "au{zz}b",
		`a\bb`:     "a\bb",
		`tail\`:    `tail\`, // lone trailing backslash is preserved
	}
	for in, want := range cases {
		if got := processTemplateEscapes(in); got != want {
			t.Errorf("processTemplateEscapes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplateWave3Multiline(t *testing.T) {
	vals := mustParseWave3(t, "`line1\nline2`")
	if len(vals) != 1 {
		t.Fatalf("multiline template: got %d values, want 1", len(vals))
	}
	if s, _ := core.AsString(vals[0]); s != "line1\nline2" {
		t.Errorf("multiline template: got %q", s)
	}
}

func TestTemplateWave3Boundaries(t *testing.T) {
	// Empty template.
	vals := mustParseWave3(t, "``")
	if len(vals) != 1 {
		t.Fatalf("``: got %d values, want 1", len(vals))
	}
	if s, _ := core.AsString(vals[0]); s != "" {
		t.Errorf("``: got %q, want empty string", s)
	}
	// Unterminated with text: auto-closes at EOF with the literal.
	vals = mustParseWave3(t, "`abc")
	if len(vals) != 1 {
		t.Fatalf("`abc: got %d values, want 1", len(vals))
	}
	if s, _ := core.AsString(vals[0]); s != "abc" {
		t.Errorf("`abc: got %q, want abc", s)
	}
	// A lone backtick is rejected by the grammar (no panic).
	wantParseErrWave3(t, "`", "")
	// Unterminated WITH a hole: auto-closes to an InterpString.
	for _, src := range []string{"`${x}", "`${x"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 || !core.IsInterpString(vals[0]) {
			t.Errorf("%q: expected one InterpString, got %v", src, vals)
		}
	}
}

// TestTemplateWave3InterpNested pins interpolated templates inside paren
// groups, explicit lists, and map values (each context arms the interp
// grammar with different dlist/dmap state).
func TestTemplateWave3InterpNested(t *testing.T) {
	for _, src := range []string{"(`${1}`)", "[`${1}`]", "{a:`${1}`}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
	}
	// A template with NO holes collapses to a plain string even nested.
	for _, src := range []string{"[`x`]", "{a:`x`}"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
	}
	// An interpolated map value stays an InterpString.
	vals := mustParseWave3(t, "{a:`${1}`}")
	m, err := core.AsMap(vals[0])
	if err != nil {
		t.Fatalf("{a:`${1}`}: AsMap: %v", err)
	}
	v, ok := m.Get("a")
	if !ok || !core.IsInterpString(v) {
		t.Errorf("{a:`${1}`}: value %v is not an InterpString", v)
	}
}

func TestTemplateWave3InterpErrors(t *testing.T) {
	// A malformed expression inside ${} surfaces the inner error.
	wantParseErrWave3(t, "`${1__0}`", "interpolation expression error")
	// An empty ${} hole holds no expression and contributes nothing
	// (NUR060): the template continues past it, and one whose holes are
	// all empty is the plain string it spells.
	for src, want := range map[string]string{"`${}`": "", "`${} tail`": " tail", "`x${ }y`": "xy"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
		if got, err := core.AsString(vals[0]); err != nil || got != want {
			t.Errorf("%q: got %v (%v), want the plain string %q", src, vals[0], err, want)
		}
	}
}
