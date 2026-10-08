package formatter

import (
	"strings"
	"testing"
)

// checkIdempotent asserts Format(Format(x)) == Format(x) — the formatter's
// fixed-point promise for every already-formatted output.
func checkIdempotent(t *testing.T, src string) {
	t.Helper()
	first := Format(src)
	second := Format(first)
	if first != second {
		t.Errorf("not idempotent for %q:\nfirst:  %q\nsecond: %q", src, first, second)
	}
}

// TestFormatTokenizerEdges covers the tokenizer's less-travelled arms:
// CR/CRLF newlines, block comments (terminated and unterminated), line
// comments without a trailing newline, strings with escapes, unterminated
// strings, negative and decimal numbers, and the number+`.digits` join.
func TestFormatTokenizerEdges(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"crlf newline", "def x 1\r\ndef y 2\n", "def x 1\ndef y 2\n"},
		{"bare cr newline", "def x 1\rdef y 2\n", "def x 1\ndef y 2\n"},
		{"block comment", "## a block ##\ndef x 1\n", "## a block ##\ndef x 1\n"},
		{"unterminated block comment", "## open ended", "## open ended\n"},
		{"line comment no trailing newline", "# tail comment", "# tail comment\n"},
		{"double quoted escape", `"a\"b"` + "\n", `"a\"b"` + "\n"},
		{"single quoted string", "'abc'\n", "'abc'\n"},
		{"unterminated string", `"abc`, `"abc` + "\n"},
		{"negative integer", "-42\n", "-42\n"},
		{"decimal", "3.14\n", "3.14\n"},
		{"negative decimal", "-5.25\n", "-5.25\n"},
		{"number joins spaced decimal digits", "1 .5\n", "1.5\n"},
		{"number then bare dot", "1. x\n", "1.x\n"},
		{"semicolon", "a; b\n", "a ; b\n"},
		{"pipe", "a | b\n", "a | b\n"},
		{"bang", "a ! b\n", "a ! b\n"},
		{"question attaches", "x ? y\n", "x? y\n"},
		{"dot attaches both sides", "m . a\n", "m.a\n"},
		{"colon attaches both sides", "x : y\n", "x:y\n"},
		{"word ending in dot attaches paren", "input.(foo)\n", "input.(foo)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in)
			if got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
			checkIdempotent(t, tt.in)
		})
	}
}

// TestFormatUnbalancedClosers pins the tree builder against stray or
// mismatched closing brackets — they must be absorbed, never panic.
func TestFormatUnbalancedClosers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"stray rbracket", "a]\n", "a\n"},
		{"stray rbrace", "a}\n", "a\n"},
		{"stray rparen", "a)\n", "a\n"},
		{"paren closes open list", "[a)\n", "[a]\n"},
		{"bracket closes open map", "{a:1]\n", "{a:1}\n"},
		{"unclosed list", "[a b\n", "[a b]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in)
			if got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatTypeCapitalization covers each arm of capitalizeTypesInTree:
// standalone type names capitalize, names after def/refine (and one
// further slot after them) do not, keys before a colon do not, dotted
// words never do, and record/object/function are keywords, not types.
func TestFormatTypeCapitalization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"standalone type capitalizes", "integer\n", "Integer\n"},
		{"after def stays", "def string 1\n", "def string 1\n"},
		{"two after def stays", "def Foo map\n", "def Foo map\n"},
		{"key before colon stays", "{string: 1}\n", "{string:1}\n"},
		{"dotted word stays", "table.kind\n", "table.kind\n"},
		{"record keyword stays", "record\n", "record\n"},
		{"object keyword stays", "object\n", "object\n"},
		{"function keyword stays", "function\n", "function\n"},
		{"inside paren capitalizes", "(integer)\n", "(Integer)\n"},
		{"inside list capitalizes", "[boolean]\n", "[Boolean]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in)
			if got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatValueWordsNotCapitalized pins the fix that fmt must not
// capitalise the type names that are also valid lowercase VALUE / function
// words — none (the None value, distinct from the None type), and the native
// words list / any / node. Capitalising them changes program semantics
// (`size none` → `size None`, `list 1 2 3` → `List 1 2 3`), so they are left
// verbatim, while genuine lowercase type names still capitalise and existing
// capitalised forms are untouched.
func TestFormatValueWordsNotCapitalized(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"none value stays", "size none\n", "size none\n"},
		{"none in prose-ish stays", "none has a\n", "none has a\n"},
		{"list constructor stays", "list 1 2 3\n", "list 1 2 3\n"},
		{"any predicate stays", "any [1 2]\n", "any [1 2]\n"},
		{"node constructor stays", "node {a:1}\n", "node {a:1}\n"},
		// Genuine type names still capitalise.
		{"integer type capitalises", "x is integer\n", "x is Integer\n"},
		{"map type capitalises", "convert map v\n", "convert Map v\n"},
		// Already-capital forms are never altered.
		{"None literal untouched", "size None\n", "size None\n"},
		{"List literal untouched", "x is List\n", "x is List\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.in); got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatBlankLineHandling pins emitRoot's blank-line policy: runs of
// blank lines collapse to one, and leading blank lines are dropped.
func TestFormatBlankLineHandling(t *testing.T) {
	if got := Format("a\n\n\n\nb\n"); got != "a\n\nb\n" {
		t.Errorf("blank collapse: got %q, want %q", got, "a\n\nb\n")
	}
	if got := Format("\n\ndef x 1\n"); got != "def x 1\n" {
		t.Errorf("leading blanks: got %q, want %q", got, "def x 1\n")
	}
	if got := Format("def x 1"); got != "def x 1\n" {
		t.Errorf("missing trailing newline: got %q, want %q", got, "def x 1\n")
	}
	if got := Format(""); got != "" {
		t.Errorf("empty input: got %q, want empty", got)
	}
}

// TestFormatFnMultiLine covers tryFnFormat's multi-line path: a def fn
// whose signature triple does not fit on one line puts the body on its
// own indented lines between `[` and `]]`.
func TestFormatFnMultiLine(t *testing.T) {
	// A multi-parameter fn keeps the bracketed spec-list form, so this
	// exercises tryFnFormat's multi-line body path (single-param fns take
	// the bracketless triple form and bypass tryFnFormat).
	src := "def process-all fn [[input:String extra:String] [String] [input upper input lower input trim input rev]]\n"
	want := "def process-all fn [[input:String extra:String] [String] [\n" +
		"  input upper input lower input trim input rev\n" +
		"]]\n"
	got := Format(src)
	if got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	checkIdempotent(t, src)
}

// TestFormatFnBodyGroups covers splitIntoGroups inside a fn body: each
// statement-start word (def, if, for, …) begins a new line.
func TestFormatFnBodyGroups(t *testing.T) {
	src := "def worker fn [[x:Integer] [Integer] [def y 1 def z 2 x y add z add echo echo]]\n"
	got := Format(src)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected a multi-line fn body, got %q", got)
	}
	if !strings.Contains(got, "\n  def y 1\n") || !strings.Contains(got, "\n  def z 2") {
		t.Errorf("body groups should split at def: %q", got)
	}
	checkIdempotent(t, src)
}

// TestFormatFnLongHeader covers the header-wrap branch of tryFnFormat: a
// signature line that itself exceeds the width is wrapped.
func TestFormatFnLongHeader(t *testing.T) {
	src := "def extremely-long-function-name-here fn [[alpha:String beta:String gamma:String delta:String] [String] [alpha beta add gamma add delta add more-words even-more-words]]\n"
	got := Format(src)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected multi-line output, got %q", got)
	}
	// The body must still be bracketed by the header's trailing "[" and a
	// closing "]]" line.
	if !strings.HasSuffix(got, "]]\n") {
		t.Errorf("expected trailing ]] line, got %q", got)
	}
	// NOTE: wrapped output is not a formatter fixed point (continuation
	// indents re-parse as fresh statements), so no idempotence check here.
}

// TestFormatFnNotApplicable covers tryFnFormat's decline arms: fn with a
// non-list follower and a wrapper with fewer than three inner lists both
// fall through to the generic wrap.
func TestFormatFnNotApplicable(t *testing.T) {
	// fn followed by a word, statement long enough to trigger wrapping.
	src := "fn not-a-list word-a word-b word-c word-d word-e word-f word-g word-h word-i\n"
	got := Format(src)
	if !strings.HasPrefix(got, "fn not-a-list word-a") || !strings.Contains(got, "\n  ") {
		t.Errorf("expected the generic wrap with continuation indent, got %q", got)
	}

	// fn wrapper with only two inner lists.
	src2 := "def f fn [[a:String] [String]] word-a word-b word-c word-d word-e word-f word-g\n"
	got2 := Format(src2)
	if !strings.HasPrefix(got2, "def f fn [[a:String] [String]]") {
		t.Errorf("expected the generic wrap, got %q", got2)
	}
}

// TestFormatTrailingMap covers tryTrailingContainer's map arm: a long
// statement ending in a map keeps `{` on the header line and indents one
// entry per line.
func TestFormatTrailingMap(t *testing.T) {
	src := "export Something {alpha:1 beta:2 gamma:3 delta:4 epsilon:5 zeta:6 eta:7 theta:8}\n"
	want := "export Something {\n" +
		"  alpha:1\n" +
		"  beta:2\n" +
		"  gamma:3\n" +
		"  delta:4\n" +
		"  epsilon:5\n" +
		"  zeta:6\n" +
		"  eta:7\n" +
		"  theta:8\n" +
		"}\n"
	got := Format(src)
	if got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	checkIdempotent(t, src)
}

// TestFormatTrailingList covers tryTrailingContainer's list arm — `[` on
// the header line, grouped contents, closing `]` alone.
func TestFormatTrailingList(t *testing.T) {
	src := "def big-list [alpha beta gamma delta epsilon zeta eta theta iota kappa lam]\n"
	got := Format(src)
	if !strings.HasPrefix(got, "def big-list [\n") {
		t.Errorf("expected `[` on the header line, got %q", got)
	}
	if !strings.HasSuffix(got, "\n]\n") {
		t.Errorf("expected closing ] on its own line, got %q", got)
	}
	checkIdempotent(t, src)
}

// TestFormatTrailingListWrappedGroup covers the wrapStatement fallback
// inside tryTrailingContainer for a single group longer than the width.
func TestFormatTrailingListWrappedGroup(t *testing.T) {
	src := "def wide [word-one word-two word-three word-four word-five word-six word-seven word-eight word-nine]\n"
	got := Format(src)
	for i, line := range strings.Split(got, "\n") {
		if len(line) > maxLineWidth {
			t.Errorf("line %d exceeds %d chars: %q", i+1, maxLineWidth, line)
		}
	}
	checkIdempotent(t, src)
}

// TestFormatWrapStatement covers the generic wrap: a long run of plain
// words breaks with a two-space continuation indent.
func TestFormatWrapStatement(t *testing.T) {
	src := "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike\n"
	got := Format(src)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %q", got)
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("continuation line should be indented two spaces: %q", lines[1])
	}
	for i, line := range lines {
		if len(line) > maxLineWidth {
			t.Errorf("line %d exceeds %d chars: %q", i+1, maxLineWidth, line)
		}
	}
}

// TestFormatLongParen covers emitParen's multi-line path via a long
// statement ending in a paren group.
func TestFormatLongParen(t *testing.T) {
	src := "def x (word-one word-two word-three word-four word-five word-six word-seven word-eight)\n"
	got := Format(src)
	if !strings.Contains(got, "(") || !strings.Contains(got, ")") {
		t.Fatalf("paren delimiters lost: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("expected multi-line output, got %q", got)
	}
}

// TestFormatEmptyParen pins the () and [] and {} empties inside a statement.
func TestFormatEmptyContainers(t *testing.T) {
	tests := []struct{ in, want string }{
		{"f ()\n", "f ()\n"},
		{"f []\n", "f []\n"},
		{"f {}\n", "f {}\n"},
	}
	for _, tt := range tests {
		if got := Format(tt.in); got != tt.want {
			t.Errorf("Format(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestFormatStandaloneLongMap covers emitMap's multi-line path (first
// entry on the `{` line) when the map is the whole statement.
func TestFormatStandaloneLongMap(t *testing.T) {
	src := "{alpha:100 beta:200 gamma:300 delta:400 epsilon:500 zeta:600 eta:700 theta:800}\n"
	got := Format(src)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected multi-line map, got %q", got)
	}
	if !strings.Contains(lines[0], "{alpha:100") {
		t.Errorf("first entry should ride the {: %q", lines[0])
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "}" {
		t.Errorf("closing brace should be alone: %q", lines[len(lines)-1])
	}
}

// TestFormatStandaloneLongList covers emitList's multi-line grouped path
// when the list is the whole statement, including group splits at def.
func TestFormatStandaloneLongList(t *testing.T) {
	src := "[def alpha 1 def beta 2 def gamma 3 def delta 4 def epsilon 5 def zeta 6]\n"
	got := Format(src)
	if !strings.Contains(got, "def beta 2") {
		t.Fatalf("groups lost: %q", got)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected multi-line list, got %q", got)
	}
	if !strings.HasPrefix(lines[0], "[") {
		t.Errorf("expected [ to open the first line: %q", lines[0])
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "]" {
		t.Errorf("closing bracket should be alone: %q", lines[len(lines)-1])
	}
}

// TestFormatMapEntryForms covers parseMapEntries' remaining arms: the
// explicit optional pair, string keys, comments inside maps, and the
// modifier-stripping shorthand rules of wordBaseName.
func TestFormatMapEntryForms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"explicit optional pair", "{foo?: bar}\n", "{foo?:bar}\n"},
		{"string key", `{"a":1}` + "\n", `{"a":1}` + "\n"},
		// A line comment inside a map forces the map multi-line: on one line
		// the comment would swallow the following entry and the closing `}`.
		{"comment inside map", "{a:1\n# note\nb:2}\n", "{a:1\n  # note\n  b:2\n}\n"},
		{"digit modifier shorthand", "{foo/2}\n", "{foo:foo/2}\n"},
		{"digit+force shorthand", "{foo/2s}\n", "{foo:foo/2s}\n"},
		{"qs shorthand", "{foo/qs}\n", "{foo:foo/qs}\n"},
		{"double force not a modifier", "{foo/ff}\n", "{foo/ff:foo/ff}\n"},
		{"double qr not a modifier", "{foo/qv}\n", "{foo/qv:foo/qv}\n"},
		{"double q not a modifier", "{foo/qq}\n", "{foo/qq:foo/qq}\n"},
		{"split digit runs not a modifier", "{foo/1s2}\n", "{foo/1s2:foo/1s2}\n"},
		{"unknown modifier char not stripped", "{foo/x}\n", "{foo/x:foo/x}\n"},
		{"trailing slash not stripped", "{foo/}\n", "{foo/:foo/}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in)
			if got != tt.want {
				t.Errorf("Format(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
			checkIdempotent(t, tt.in)
		})
	}
}

// TestWordBaseNameDirect pins the pure helper's contract table directly.
func TestWordBaseNameDirect(t *testing.T) {
	tests := []struct{ in, want string }{
		{"foo", "foo"},
		{"foo/v", "foo"},
		{"foo/q", "foo"},
		{"foo/s", "foo"},
		{"foo/f", "foo"},
		{"foo/2", "foo"},
		{"foo/12", "foo"},
		{"foo/2q", "foo"},
		{"foo/sq", "foo"},
		{"foo/", "foo/"},
		{"foo/x", "foo/x"},
		{"foo/ff", "foo/ff"},
		{"foo/vv", "foo/vv"},
		{"foo/1s2", "foo/1s2"},
		{"a/b/q", "a/b"},
	}
	for _, tt := range tests {
		if got := wordBaseName(tt.in); got != tt.want {
			t.Errorf("wordBaseName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestEmitNodeDirect covers emitNode's leaf punctuation arms plus the
// NdNewline / unknown-kind fallbacks that Format itself strips earlier.
func TestEmitNodeDirect(t *testing.T) {
	tests := []struct {
		kind NodeKind
		text string
		want string
	}{
		{NdComma, ",", ","},
		{NdColon, ":", ":"},
		{NdSemicolon, ";", ";"},
		{NdDot, ".", "."},
		{NdQuestion, "?", "?"},
		{NdBang, "!", "!"},
		{NdPipe, "|", "|"},
		{NdNewline, "", ""},
		{NodeKind(99), "zz", ""},
	}
	r := newRenderer(DefaultRules())
	for _, tt := range tests {
		if got := r.emitNode(&Node{Kind: tt.kind, Text: tt.text}, 0); got != tt.want {
			t.Errorf("emitNode(kind=%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

// TestCapitalizeDirect pins capitalize's empty-string guard.
func TestCapitalizeDirect(t *testing.T) {
	if got := capitalize(""); got != "" {
		t.Errorf("capitalize(\"\") = %q, want empty", got)
	}
	if got := capitalize("map"); got != "Map" {
		t.Errorf("capitalize(map) = %q, want Map", got)
	}
}

// TestTokenToNodeKindDirect covers the default arm (an unmapped token
// kind falls back to NdWord).
func TestTokenToNodeKindDirect(t *testing.T) {
	if got := tokenToNodeKind(TokWord); got != NdWord {
		t.Errorf("tokenToNodeKind(TokWord) = %d, want NdWord", got)
	}
	if got := tokenToNodeKind(TokSemicolon); got != NdSemicolon {
		t.Errorf("tokenToNodeKind(TokSemicolon) = %d, want NdSemicolon", got)
	}
}

// TestScanStringDirect covers scanString's empty-input guard.
func TestScanStringDirect(t *testing.T) {
	s, n := scanString("")
	if s != "" || n != 0 {
		t.Errorf("scanString(\"\") = %q,%d, want \"\",0", s, n)
	}
}

// TestFormatWholeFileIdempotence runs a representative module-style file
// through Format twice — the strongest end-to-end fixed-point pin.
func TestFormatWholeFileIdempotence(t *testing.T) {
	src := `# module header
(import "textkit")

## block
comment ##

def Cond refine Record [field:Atom op:String]

def process fn [[s:String] [String] [(s Styler.style)]]

def long-list [alpha beta gamma delta epsilon zeta eta theta iota kappa]

export Textkit {
process:process
extras: {a:1 b:2}
}
`
	first := Format(src)
	second := Format(first)
	if first != second {
		t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
