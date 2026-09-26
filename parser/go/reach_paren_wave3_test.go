package parser

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- `/` modifiers on dotted paths and paren groups ---

// TestReachWave3WordModifiers pins the word-modifier placement rule: /u /s
// /f desugar to ONE sugar marker emitted BEFORE the reach group (ADR-012
// rule 3 amendment) — the engine lowers it to the role-bound word, which
// forward-collects the result.
func TestReachWave3WordModifiers(t *testing.T) {
	cases := []struct {
		src  string
		kind core.SugarKind
	}{
		{"a.b/u", core.SugarUsurp},
		{"a.b/s", core.SugarStackArgs},
		{"a.b/f", core.SugarForwardArgs},
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2: %v", c.src, len(vals), vals)
		}
		if info, ok := core.AsSugar(vals[0]); !ok || info.Kind != c.kind {
			t.Errorf("%q: value 0 = %v, want sugar(%s)", c.src, vals[0], c.kind)
		}
		if !core.IsReach(vals[1]) {
			t.Errorf("%q: last value is not a Reach: %v", c.src, vals[1])
		}
	}
}

func TestReachWave3ForceArityModifier(t *testing.T) {
	// a.b/2 → one force-arity sugar marker before the reach.
	vals := mustParseWave3(t, "a.b/2")
	if len(vals) != 2 {
		t.Fatalf("a.b/2: got %d values, want 2: %v", len(vals), vals)
	}
	if info, ok := core.AsSugar(vals[0]); !ok || info.Kind != core.SugarForceArity || info.N != 2 {
		t.Errorf("a.b/2: first value %v, want sugar(force-arity 2)", vals[0])
	}
	if !core.IsReach(vals[1]) {
		t.Errorf("a.b/2: second value is not a Reach: %v", vals[1])
	}
}

// TestReachWave3MarkerModifiers pins /v and /q: the Word/__DM marker is
// emitted AFTER the group, both fused (`a.b/v`) and standalone (`m.k /v`).
func TestReachWave3MarkerModifiers(t *testing.T) {
	for _, src := range []string{"a.b/v", "a.b/q", "m.k /v"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2: %v", src, len(vals), vals)
		}
		if !core.IsReach(vals[0]) {
			t.Errorf("%q: first value is not a Reach: %v", src, vals[0])
		}
		if !core.IsDispatchMod(vals[1]) {
			t.Errorf("%q: second value is not a Word/__DM marker: %v", src, vals[1])
		}
	}
}

// TestReachWave3TypeBoundKey pins `/t` on the final key: groupModifier
// declines (/t is word-only sugar), so the key itself desugars to the
// `(Type of [b/q])` paren expression inside the reach.
func TestReachWave3TypeBoundKey(t *testing.T) {
	vals := mustParseWave3(t, "a.b/t")
	if len(vals) != 1 || !core.IsReach(vals[0]) {
		t.Fatalf("a.b/t: expected one Reach, got %v", vals)
	}
	ri, _ := core.AsReach(vals[0])
	last := ri.Segments[len(ri.Segments)-1].KeyLit
	if info, ok := core.AsSugar(last); !ok || info.Kind != core.SugarTypeBound {
		t.Errorf("a.b/t: the final key is %v, want the type-bound sugar", last)
	}
	// …and it renders as the source it came from (NUR072).
	if s := vals[0].String(); s != "a.b/t" {
		t.Errorf("a.b/t: rendering %q, want a.b/t", s)
	}
}

// TestParenWave3StandaloneModifier pins a standalone `/mod` token applying
// to the preceding paren group's result.
func TestParenWave3StandaloneModifier(t *testing.T) {
	// Word modifier prefixes the group.
	vals := mustParseWave3(t, "(1 add 2)/s")
	if len(vals) != 2 {
		t.Fatalf("(1 add 2)/s: got %d values, want 2: %v", len(vals), vals)
	}
	if info, ok := core.AsSugar(vals[0]); !ok || info.Kind != core.SugarStackArgs {
		t.Errorf("(1 add 2)/s: first value %v, want sugar(stack-args)", vals[0])
	}
	if !core.IsParenExpr(vals[1]) {
		t.Errorf("(1 add 2)/s: second value is not a ParenExpr: %v", vals[1])
	}
	// Marker modifier suffixes the group.
	vals = mustParseWave3(t, "(f)/v")
	if len(vals) != 2 {
		t.Fatalf("(f)/v: got %d values, want 2: %v", len(vals), vals)
	}
	if !core.IsParenExpr(vals[0]) || !core.IsDispatchMod(vals[1]) {
		t.Errorf("(f)/v: want [ParenExpr, Word/__DM], got %v", vals)
	}
}

// --- computed / quoted keys, paren receivers ---

func TestReachWave3ComputedKey(t *testing.T) {
	vals := mustParseWave3(t, "m.(x add 1)")
	if len(vals) != 1 || !core.IsReach(vals[0]) {
		t.Fatalf("m.(x add 1): expected one Reach, got %v", vals)
	}
	info, err := core.AsReach(vals[0])
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if len(info.Segments) != 1 || !info.Segments[0].Computed {
		t.Fatalf("m.(x add 1): expected one computed segment, got %+v", info.Segments)
	}
	if len(info.Segments[0].KeyExpr) != 3 {
		t.Errorf("m.(x add 1): key expr has %d tokens, want 3", len(info.Segments[0].KeyExpr))
	}
}

func TestReachWave3QuotedKey(t *testing.T) {
	vals := mustParseWave3(t, "m.'k k'")
	if len(vals) != 1 || !core.IsReach(vals[0]) {
		t.Fatalf("m.'k k': expected one Reach, got %v", vals)
	}
	info, _ := core.AsReach(vals[0])
	if len(info.Segments) != 1 {
		t.Fatalf("m.'k k': got %d segments, want 1", len(info.Segments))
	}
	if s, err := core.AsString(info.Segments[0].KeyLit); err != nil || s != "k k" {
		t.Errorf("m.'k k': key literal %v, want string \"k k\"", info.Segments[0].KeyLit)
	}
}

func TestReachWave3Errors(t *testing.T) {
	// Errors inside the receiver, a computed key, or a literal key all
	// surface cleanly.
	wantParseErrWave3(t, "(1__0).a", "misplaced `_`")
	wantParseErrWave3(t, "m.(1__0)", "misplaced `_`")
	wantParseErrWave3(t, "m.1e", "invalid numeric literal")
}

// --- dot-chains in map values (the dotchain grammar rule) ---

func TestReachWave3MapValueDotChains(t *testing.T) {
	// A getr segment (`!.`), a two-segment chain, and a QUOTED key in
	// map-value position all fold into the same paren group the explicit
	// form produces.
	for src, want := range map[string]string{
		"{a: m!.k}":    "(m !).k",
		"{a: m.k1.k2}": "(m.k1).k2",
		"{a: m.'k k'}": "m.'k k'",
	} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		if s := vals[0].String(); !strings.Contains(s, want) {
			t.Errorf("%q: rendering %q does not contain %q", src, s, want)
		}
	}
}

// --- unclosed parens ---

func TestParenWave3Unclosed(t *testing.T) {
	// In an item run, and in data (map-value) context.
	wantParseErrWave3(t, "1 (2", "unmatched opening parenthesis")
	wantParseErrWave3(t, "{a:(1", "unmatched opening parenthesis")
	// Errors inside a data-context paren group surface too.
	wantParseErrWave3(t, "{a:(1__0)}", "misplaced `_`")
	// …and inside a paren group qualified by a standalone modifier.
	wantParseErrWave3(t, "(1__0)/s", "misplaced `_`")
}

// --- minilang literals ---

func TestMiniLitWave3Inline(t *testing.T) {
	// In word context the literal expands INLINE to `mini name 'src'`.
	cases := []struct{ src, name, mini string }{
		{`1 +re/x/`, "re", "x"},
		{`1 +re/a\/b/`, "re", "a/b"},           // \<delim> → delim
		{`1 +re/a\\b/`, "re", `a\b`},           // \\ → backslash
		{`1 +re/a\ b/`, "re", "a b"},           // \<space> → space
		{`1 +re/a\db/`, "re", `a\db`},          // other escapes stay raw
		{`1 +my-kind2:abc`, "my-kind2", "abc"}, // open form, digit+dash name
	}
	for _, c := range cases {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2: %v", c.src, len(vals), vals)
		}
		info, ok := core.AsSugar(vals[1])
		if !ok || info.Kind != core.SugarMini {
			t.Fatalf("%q: expected a mini sugar marker, got %v", c.src, vals[1])
		}
		if info.Name != c.name || info.Src != c.mini {
			t.Errorf("%q: marker (%s, %q), want (%s, %q)", c.src, info.Name, info.Src, c.name, c.mini)
		}
	}
}

func TestMiniLitWave3SpliceForms(t *testing.T) {
	// A single top-level literal, and a data-context (map value) literal,
	// both become the `mini name 'src'` splice.
	for _, src := range []string{`+re/x/`, `{a:+re/x/}`} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1", src, len(vals))
		}
		v := vals[0]
		if m, err := core.AsMap(v); err == nil {
			got, ok := m.Get("a")
			if !ok {
				t.Fatalf("%q: map lacks key a", src)
			}
			v = got
		}
		info, ok := core.AsSugar(v)
		if !ok || info.Kind != core.SugarMini {
			t.Fatalf("%q: expected a mini sugar marker, got %s", src, v.String())
		}
		if info.Name != "re" || info.Src != "x" {
			t.Errorf("%q: marker (%s, %q), want (re, \"x\")", src, info.Name, info.Src)
		}
	}
}

func TestMiniLitWave3Declines(t *testing.T) {
	// An empty open source is NOT a literal — the token stays a plain word.
	vals := mustParseWave3(t, `+re/`)
	if len(vals) != 1 {
		t.Fatalf("+re/: got %d values, want 1", len(vals))
	}
	if w := wordNameWave3(t, vals[0], "+re/"); w.Name != "+re/" {
		t.Errorf("+re/: want the whole token as a word, got %q", w.Name)
	}
	// A missing delimiter (EOF or whitespace right after the name) also
	// declines: `+re` stays a plain word.
	for _, c := range []struct {
		src  string
		idx  int
		want string
	}{{"1 +re", 1, "+re"}, {"+re x", 0, "+re"}} {
		vals := mustParseWave3(t, c.src)
		if len(vals) != 2 {
			t.Fatalf("%q: got %d values, want 2", c.src, len(vals))
		}
		if w := wordNameWave3(t, vals[c.idx], c.src); w.Name != c.want {
			t.Errorf("%q: got word %q, want %q", c.src, w.Name, c.want)
		}
	}
}

func TestMiniLitWave3OpenFormStopsAtWhitespace(t *testing.T) {
	// The open form ends at the first unescaped whitespace; following
	// tokens are untouched.
	vals := mustParseWave3(t, "1 +email:alice@x.com 2")
	if len(vals) != 3 {
		t.Fatalf("open-form mini: got %d values, want 3: %v", len(vals), vals)
	}
	if info, ok := core.AsSugar(vals[1]); !ok || info.Kind != core.SugarMini || info.Src != "alice@x.com" {
		t.Errorf("open-form mini: marker %v, want sugar(mini email 'alice@x.com')", vals[1])
	}
	if n, err := core.AsInteger(vals[2]); err != nil || n != 2 {
		t.Errorf("open-form mini: trailing token %v, want Integer 2", vals[2])
	}
}

// --- arrow (=>) folds ---

func TestArrowWave3Folds(t *testing.T) {
	// Word-context fold after a preceding value.
	vals := mustParseWave3(t, "1 x => [x]")
	if len(vals) != 2 {
		t.Fatalf("1 x => [x]: got %d values, want 2: %v", len(vals), vals)
	}
	if !core.IsParenExpr(vals[1]) {
		t.Fatalf("1 x => [x]: expected a folded ParenExpr, got %v", vals[1])
	}
	if s := vals[1].String(); !strings.Contains(s, "sugar(lambda)") {
		t.Errorf("1 x => [x]: fold %q lacks the lambda marker", s)
	}

	// Top-level bare-pair fold (elem-level): the documented def-lambda form.
	vals = mustParseWave3(t, "def double x:Integer => [x mul 2]")
	if len(vals) != 3 {
		t.Fatalf("def-lambda: got %d values, want 3: %v", len(vals), vals)
	}
	if !core.IsParenExpr(vals[2]) {
		t.Fatalf("def-lambda: expected the pair to fold into a ParenExpr, got %v", vals[2])
	}
	if s := vals[2].String(); !strings.Contains(s, "x:word(Integer)") || !strings.Contains(s, "sugar(lambda)") {
		t.Errorf("def-lambda: fold %q lacks the sig/lambda shape", s)
	}

	// Pair fold inside an explicit list, and paren-context pair dive.
	for _, src := range []string{"[a:1 => [1]]", "[x:Integer => [x]]", "(x:Integer => [x mul 2])"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
		if s := vals[0].String(); !strings.Contains(s, "sugar(lambda)") {
			t.Errorf("%q: %q lacks the lambda-marker fold", src, s)
		}
	}

	// A dotted receiver: the arrow still parses (the reach folds first),
	// in flat, spaced, and paren-wrapped forms.
	for _, src := range []string{"m.k => [1]", "(m . k => [1])", "(1 x => [x])"} {
		vals := mustParseWave3(t, src)
		if len(vals) != 1 {
			t.Fatalf("%q: got %d values, want 1: %v", src, len(vals), vals)
		}
	}
}

// A bare `=>` in DATA context (a map value) converts to the same
// lambda marker word context gets — the arrowTag arm of
// convertDataValueInner.
func TestArrowWave3DataContext(t *testing.T) {
	vals := mustParseWave3(t, "{a: =>}")
	if len(vals) != 1 {
		t.Fatalf("{a: =>}: got %d values, want 1: %v", len(vals), vals)
	}
	m, merr := core.AsMap(vals[0])
	if merr != nil || m == nil {
		t.Fatalf("{a: =>}: want a map, got %v (%v)", vals[0], merr)
	}
	av, _ := m.Get("a")
	if info, ok := core.AsSugar(av); !ok || info.Kind != core.SugarLambda {
		t.Errorf("{a: =>}: value %v, want the lambda marker", av)
	}
}

func TestArrowWave3Degenerate(t *testing.T) {
	// A trailing arrow with no body parses without panicking: the sig and
	// the lambda marker remain as flat tokens (nothing is silently invented).
	vals := mustParseWave3(t, "x =>")
	if len(vals) != 2 {
		t.Fatalf("x =>: got %d values, want 2: %v", len(vals), vals)
	}
	if info, ok := core.AsSugar(vals[1]); !ok || info.Kind != core.SugarLambda {
		t.Errorf("x =>: second value %v, want the lambda marker", vals[1])
	}
	// A pair arrow with no body inside a list also parses (empty fold),
	// as does a plain-value arrow with no body.
	for _, src := range []string{"[x:1 =>]", "[1 x =>]"} {
		if _, err := parseWave3(t, src); err != nil {
			t.Fatalf("%q: unexpected error: %v", src, err)
		}
	}
	// Inside a paren, a bodyless arrow leaves the group unfinished — the
	// unmatched-paren diagnostic fires (no panic).
	wantParseErrWave3(t, "(1 x =>)", "unmatched opening parenthesis")
	// A top-level bare pair with arrow and body parses flat or folded —
	// never an error.
	if _, err := parseWave3(t, "x:Integer => [x]"); err != nil {
		t.Fatalf("x:Integer => [x]: unexpected error: %v", err)
	}
	// An arrow in an EXPLICIT map value stays a syntax error.
	wantParseErrWave3(t, "{a: 1 => 2}", "")
}
