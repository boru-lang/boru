package formatter

import (
	"strings"
	"unicode"
)

// maxLineWidth mirrors the width the canonical stylesheet (fmt-rules.boru)
// declares, for tests that assert line lengths. TestDefaultRulesFromBoru
// pins the two equal — the stylesheet is the definition, this is the echo.
const maxLineWidth = 72

// knownTypes lists type names auto-capitalised to their canonical form when
// they appear as a lowercase word (e.g. `x:integer` → `x:Integer`).
//
// It deliberately EXCLUDES none / list / any / node: each is also a valid
// lowercase VALUE or function word — `none` is the None value (distinct from
// the `None` type: `canon none` → `none`, and `none tcmp None → 1`), and
// `list` / `any` / `node` are native words (a list constructor, a predicate,
// a node constructor). Capitalising those would change program semantics
// (`size none` → `size None`, `list 1 2 3` → `List 1 2 3`), and a formatter
// must never do that. The remaining names are undefined as values, so they
// only ever occur as type annotations and capitalising them is always safe.
// (Capital forms like `None` / `List` are never touched — capitalize only
// ever uppercases.)
var knownTypes = map[string]bool{
	"scalar": true, "number": true,
	"integer": true, "float": true, "string": true, "boolean": true,
	"atom": true, "map": true,
	"table": true, "record": true, "object": true, "function": true,
}

// TokenKind classifies a token in the boru source.
type TokenKind int

const (
	TokWord TokenKind = iota
	TokString
	TokNumber
	TokComment
	TokLBracket
	TokRBracket
	TokLBrace
	TokRBrace
	TokLParen
	TokRParen
	TokComma
	TokColon
	TokSemicolon
	TokDot
	TokQuestion
	TokBang
	TokPipe
	TokNewline
)

// Token is a lexical token from boru source.
type Token struct {
	Kind TokenKind
	Text string
}

// NodeKind classifies a node in the format tree.
type NodeKind int

const (
	NdRoot NodeKind = iota
	NdWord
	NdString
	NdNumber
	NdComment
	NdList
	NdMap
	NdParen
	NdComma
	NdColon
	NdSemicolon
	NdDot
	NdQuestion
	NdBang
	NdPipe
	NdNewline
)

// Node is a node in the format tree.
type Node struct {
	Kind     NodeKind
	Text     string
	Children []*Node
}

// Parse turns boru source into the format tree the emitter walks (an
// NdRoot node). It is a PARAMETER of the formatter (see FormatWith) so the
// front end is pluggable: the default is the built-in lossless lexer, but
// a tabnas grammar that preserves comments, backticks, and layout can be
// dropped in without touching the layout rules or the emitter. The
// contract is a lossless tree — comments (NdComment),
// newlines (NdNewline), and source order must be preserved for the
// emitter to reproduce them.
type Parse func(src string) *Node

// DefaultParse is the formatter's default front end: the trivia-preserving
// tabnas CST parser (TabnasParse). It realises the Q2 requirement that "Fmt
// should parse, using a tabnas parser" — the production `boru fmt`, the fmt
// module, the LSP, and the markdown/HTML embedders all reach it through
// Format. HandParse remains as the independent reference that
// TestTabnasParseCorpus pins DefaultParse against, byte-for-byte.
func DefaultParse(src string) *Node {
	return TabnasParse(src)
}

// HandParse is the original hand-written lexer/tree-builder front end
// (tokenize → buildTree). It is retained as the INDEPENDENT reference the
// tabnas front end is differentially verified against — two front ends that
// must agree on every input keep each other honest. Injectable via
// FormatWith like any other Parse.
func HandParse(src string) *Node {
	return buildTree(tokenize(src))
}

// Format formats boru source code using the default (tabnas) parser.
func Format(src string) string {
	return FormatWith(src, DefaultParse)
}

// ParseTree returns the layout tree the emitter walks for src: the default
// (tabnas) front end's parse with the semantics-preserving transforms already
// applied (type-name capitalisation, fn-bracket elision). It is the seam a
// DECLARATIVE formatter consumes — bridge this tree to boru values (boru:fmt's
// Fmt.tree) and a rule set keyed by node kind can lay it out, the Phase-3
// "layout rules as boru" direction. Emitting ParseTree(src) with the built-in
// emitRoot reproduces Format(src) exactly.
func ParseTree(src string) *Node {
	tree := DefaultParse(src)
	canonicaliseTree(tree)
	return tree
}

// canonicaliseTree applies every semantics-preserving spelling transform, in
// the order the emitter expects. It exists so ParseTree and FormatRulesWith
// cannot disagree about what "the tree" is — the invariant ParseTree's doc
// states (emitting its result with emitRoot reproduces Format exactly) holds
// only while both run the SAME set, and it silently stopped holding when
// normaliseStatementEnd was added to the format path alone: `Fmt.tree` went
// on handing a declarative rule set `end` word nodes for a terminator the
// built-in emitter had already respelled `;`. Add a transform HERE, never to
// one caller.
func canonicaliseTree(n *Node) {
	capitalizeTypesInTree(n)
	elideFnBrackets(n)
	normaliseStatementEnd(n)
}

// NodeKindName is the stable dispatch key for a node kind — the atom a
// declarative rule table matches on (`word`, `list`, `map`, `paren`,
// `comment`, `newline`, …). It is the source-CST analogue of Fmt.kind's
// tag: a formatter written as boru rules dispatches on these names.
func NodeKindName(k NodeKind) string {
	switch k {
	case NdRoot:
		return "root"
	case NdWord:
		return "word"
	case NdString:
		return "string"
	case NdNumber:
		return "number"
	case NdComment:
		return "comment"
	case NdList:
		return "list"
	case NdMap:
		return "map"
	case NdParen:
		return "paren"
	case NdComma:
		return "comma"
	case NdColon:
		return "colon"
	case NdSemicolon:
		return "semicolon"
	case NdDot:
		return "dot"
	case NdQuestion:
		return "question"
	case NdBang:
		return "bang"
	case NdPipe:
		return "pipe"
	case NdNewline:
		return "newline"
	}
	// NodeKind is a closed enum built only by this package's tokeniser and
	// tree builder; every constructed kind is named above. The fallback
	// names an out-of-range value defensively (never reached from a real
	// parse; pinned by TestNodeKindNameUnknown).
	return "unknown"
}

// FormatWith formats boru source using the supplied parser and the default
// rule table. A nil parse falls back to DefaultParse. This is the seam
// through which a tabnas-based, trivia-preserving parser is injected — the
// layout rules and the emitter operate on the tree regardless of which
// front end produced it.
func FormatWith(src string, parse Parse) string {
	return FormatRulesWith(src, DefaultRules(), parse)
}

// FormatRules formats boru source under the supplied declarative rule table
// (see Rules) with the default parser. `Fmt.format-with` is its boru twin.
func FormatRules(src string, ru Rules) string {
	return FormatRulesWith(src, ru, nil)
}

// FormatRulesWith is the fully-parameterised formatter: BOTH the parser
// (the tree front end) and the rule table (the layout stylesheet) are
// parameters. The semantics-preserving canonicalisations
// (capitalizeTypesInTree, elideFnBrackets) run on the tree, then the
// processor interprets the rules over it.
func FormatRulesWith(src string, ru Rules, parse Parse) string {
	if parse == nil {
		parse = DefaultParse
	}
	tree := parse(src)
	canonicaliseTree(tree)
	return newRenderer(ru).emitRoot(tree, 0)
}

// --- Tokenizer ---

func tokenize(src string) []Token {
	var tokens []Token
	i := 0
	for i < len(src) {
		ch := src[i]

		if ch == ' ' || ch == '\t' {
			i++
			continue
		}

		if ch == '\n' {
			tokens = append(tokens, Token{TokNewline, "\n"})
			i++
			continue
		}
		if ch == '\r' {
			i++
			if i < len(src) && src[i] == '\n' {
				i++
			}
			tokens = append(tokens, Token{TokNewline, "\n"})
			continue
		}

		// Comment. Both `#` and `##` run to end of line — boru has NO bounded
		// block comment (`## a ## b` is one comment; the real parser lexes
		// `#` to EOL). Modelling `## … ##` as bounded made fmt treat what
		// follows on the line as CODE and reformat it — corrupting comment
		// text (`## t ## x is integer` → `## t ## x is Integer`).
		if ch == '#' {
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				tokens = append(tokens, Token{TokComment, src[i:]})
				i = len(src)
			} else {
				tokens = append(tokens, Token{TokComment, src[i : i+end]})
				i = i + end
			}
			continue
		}

		// Template literals: `...${...}...` are scanned verbatim as one
		// atomic token so their content is emitted exactly as written and
		// never rewrapped — the "backtick content is left verbatim" rule.
		if ch == '`' {
			s, n := scanBacktick(src[i:])
			tokens = append(tokens, Token{TokString, s})
			i += n
			continue
		}

		// Strings
		if ch == '"' || ch == '\'' {
			s, n := scanString(src[i:])
			tokens = append(tokens, Token{TokString, s})
			i += n
			continue
		}

		// Minilang literals (+re/…/, +gex|…|, +hb/…/, +m:…) are scanned
		// verbatim as one atomic token so their delimiters and inner
		// characters are never re-tokenised and re-spaced.
		if ch == '+' {
			if lit, n := scanMinilang(src[i:]); n > 0 {
				tokens = append(tokens, Token{TokWord, lit})
				i += n
				continue
			}
		}

		// Single-char tokens
		if kind, ok := singleCharToken(ch); ok {
			tokens = append(tokens, Token{kind, string(ch)})
			i++
			continue
		}

		// Dot
		if ch == '.' {
			if len(tokens) > 0 && tokens[len(tokens)-1].Kind == TokNumber &&
				i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' {
				j := i + 1
				for j < len(src) && src[j] >= '0' && src[j] <= '9' {
					j++
				}
				tokens[len(tokens)-1].Text += src[i:j]
				i = j
				continue
			}
			tokens = append(tokens, Token{TokDot, "."})
			i++
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			tokens = append(tokens, Token{TokNumber, scanNumber(src, i)})
			i += len(tokens[len(tokens)-1].Text)
			continue
		}
		if ch == '-' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' {
			tokens = append(tokens, Token{TokNumber, scanNumber(src, i)})
			i += len(tokens[len(tokens)-1].Text)
			continue
		}

		// Words (including dotted words like foo.bar)
		j := i
		for j < len(src) && !isDelimiter(src[j]) {
			j++
		}
		if j > i {
			tokens = append(tokens, Token{TokWord, src[i:j]})
			i = j
			continue
		}

		i++ //covergate:allow formatter inline/tokenizer defensive guard (§formatter)
	}
	return tokens
}

func singleCharToken(ch byte) (TokenKind, bool) {
	switch ch {
	case '[':
		return TokLBracket, true
	case ']':
		return TokRBracket, true
	case '{':
		return TokLBrace, true
	case '}':
		return TokRBrace, true
	case '(':
		return TokLParen, true
	case ')':
		return TokRParen, true
	case ',':
		return TokComma, true
	case ':':
		return TokColon, true
	case ';':
		return TokSemicolon, true
	case '?':
		return TokQuestion, true
	case '!':
		return TokBang, true
	case '|':
		return TokPipe, true
	}
	return 0, false
}

// scanNumber consumes a complete boru numeric literal starting at start,
// so `boru fmt` never splits one apart. It covers plain decimals, the `0d`
// BigInteger/BigDecimal form, `0x`/`0o`/`0b` base literals, `_` digit
// separators, and `e` exponents. A trailing `.` is taken only when a digit
// follows, leaving `0d5.foo` as `0d5` + dot-access
// (eng/go/parser/grammar.go).
func scanNumber(src string, start int) string {
	j := start
	if j < len(src) && src[j] == '-' {
		j++
	}
	// Base-prefixed literals: 0d/0x/0o/0b, each requiring a matching digit
	// after the two-char prefix (else `0` is a plain number and the letter
	// starts a word, e.g. `0dfoo` → `0` `dfoo`).
	if j+2 < len(src) && src[j] == '0' {
		switch src[j+1] {
		case 'd', 'D':
			if isDecDigit(src[j+2]) {
				return src[start:scanRealTail(src, scanDigitRun(src, j+2, isDecDigit), isDecDigit)]
			}
		case 'x', 'X':
			if isHexDigit(src[j+2]) {
				return src[start:scanDigitRun(src, j+2, isHexDigit)]
			}
		case 'o', 'O':
			if isOctDigit(src[j+2]) {
				return src[start:scanDigitRun(src, j+2, isOctDigit)]
			}
		case 'b', 'B':
			if isBinDigit(src[j+2]) {
				return src[start:scanDigitRun(src, j+2, isBinDigit)]
			}
		}
	}
	// Plain decimal: digit run, optional fraction, optional exponent.
	return src[start:scanRealTail(src, scanDigitRun(src, j, isDecDigit), isDecDigit)]
}

// scanRealTail consumes an optional `.<digits>` fraction and an optional
// `e<sign?><digits>` exponent starting at j, using digit-class pred for
// both the mantissa fraction and the exponent digits.
func scanRealTail(src string, j int, pred func(byte) bool) int {
	if j+1 < len(src) && src[j] == '.' && pred(src[j+1]) {
		j = scanDigitRun(src, j+1, pred)
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < len(src) && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < len(src) && isDecDigit(src[k]) {
			j = scanDigitRun(src, k, isDecDigit)
		}
	}
	return j
}

// scanDigitRun consumes a run of pred-digits and `_` separators from j.
func scanDigitRun(src string, j int, pred func(byte) bool) int {
	for j < len(src) && (pred(src[j]) || src[j] == '_') {
		j++
	}
	return j
}

func isDecDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool {
	return isDecDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isOctDigit(c byte) bool { return c >= '0' && c <= '7' }
func isBinDigit(c byte) bool { return c == '0' || c == '1' }

func isDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r',
		'[', ']', '{', '}', '(', ')',
		',', ';', ':', '?', '!', '|', '#':
		return true
	}
	return false
}

func scanString(s string) (string, int) {
	if len(s) < 1 {
		return "", 0
	}
	quote := s[0]
	i := 1
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == quote {
			return s[:i+1], i + 1
		}
		i++
	}
	return s, len(s)
}

// scanBacktick scans a `...` template literal (s[0] == '`'), returning the
// full literal text — both backticks included — and the byte count
// consumed. Interpolation holes ${...} are scanned with brace-depth
// tracking, skipping nested string and template literals so their
// delimiters do not close the outer template early; nesting composes to
// any depth. An unterminated literal consumes the rest of the input.
func scanBacktick(s string) (string, int) {
	i := 1
	for i < len(s) {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i += 2
		case s[i] == '`':
			return s[:i+1], i + 1
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			i = scanInterpHole(s, i+2)
		default:
			i++
		}
	}
	return s, len(s)
}

// backtickClosed reports whether a `...` literal (s[0] == '`') has a real
// closing backtick — the same scan scanBacktick runs, returning only whether
// it reached the close branch (vs consuming to the end unterminated). Used by
// the tabnas gap recovery to tell a whole literal from one the flat lexer's
// quote-swallow split across a re-scan boundary.
func backtickClosed(s string) bool {
	i := 1
	for i < len(s) {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i += 2
		case s[i] == '`':
			return true
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			i = scanInterpHole(s, i+2)
		default:
			i++
		}
	}
	return false
}

// scanInterpHole scans from just past a "${" to just past its matching
// "}", tracking brace depth and skipping nested string / template
// literals (whose braces must not affect the count). Returns the index
// after the closing brace, or len(s) when the hole is unterminated.
func scanInterpHole(s string, i int) int {
	depth := 1
	for i < len(s) {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i += 2
		case s[i] == '{':
			depth++
			i++
		case s[i] == '}':
			depth--
			if depth == 0 {
				return i + 1
			}
			i++
		case s[i] == '`':
			_, n := scanBacktick(s[i:])
			i += n
		case s[i] == '"' || s[i] == '\'':
			_, n := scanString(s[i:])
			i += n
		default:
			i++
		}
	}
	return len(s)
}

// scanMinilang detects and scans a boru minilang literal starting at
// s[0] == '+' (e.g. +re/[a-z]+/, +gex|a*b|, +hb/de_ad/, +m:a@b.com),
// returning the full raw literal text and the byte count, or ("", 0) when s
// does not start one. The body is scanned VERBATIM and emitted as a single
// atomic token — the whole point of the scan: a literal's delimiters and
// inner characters ([, ], +, *, |, /) are ordinary word/bracket tokens to
// the rest of the lexer, so without atomic scanning `+re/[a-z]+/` fragments
// and re-spaces to `+re/ [a-z] +/`, corrupting valid source.
//
// The recogniser mirrors the parser's setupMiniLitMatcher
// (eng/go/parser/grammar.go): `+<name><delim><body>` where name is a
// lowercase kind atom ([a-z][a-z0-9-]*), delim is the first non-space char
// after the name, and the body runs to the first UNESCAPED delim (closed
// form) or to unescaped whitespace (open form), honouring the \<delim>, \\,
// \<space>, \<tab> escapes. A leading `+` followed by a digit (`+0d5`) is a
// signed number, not a name start, so it is declined here and falls through.
func scanMinilang(s string) (string, int) {
	if len(s) == 0 || s[0] != '+' {
		return "", 0
	}
	i := 1
	if i >= len(s) || !(s[i] >= 'a' && s[i] <= 'z') {
		return "", 0 // the kind name must start with a lowercase letter
	}
	for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= '0' && s[i] <= '9') || s[i] == '-') {
		i++
	}
	if i >= len(s) || isSpaceByte(s[i]) {
		return "", 0 // no delimiter (whitespace or end of input) → not a literal
	}
	delim := s[i]
	i++
	closed := false
	body := 0
	for i < len(s) {
		c := s[i]
		if isSpaceByte(c) {
			break // open form: the literal ends at unescaped whitespace
		}
		if c == '\\' && i+1 < len(s) {
			if nxt := s[i+1]; nxt == delim || nxt == '\\' || nxt == ' ' || nxt == '\t' {
				i += 2 // \<delim>, \\, \<space>, \<tab> — an escaped body char
				body++
				continue
			}
			i++ // any other backslash (e.g. \d) is preserved raw
			body++
			continue
		}
		if c == delim {
			closed = true
			i++
			break
		}
		i++
		body++
	}
	if !closed && body == 0 {
		return "", 0 // an empty open source (`+name:` before whitespace) is not one
	}
	return s[:i], i
}

// isSpaceByte reports whether b is one of the boru source whitespace bytes.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// --- Tree builder ---

func buildTree(tokens []Token) *Node {
	root := &Node{Kind: NdRoot}
	stack := []*Node{root}
	cur := func() *Node { return stack[len(stack)-1] }
	push := func(n *Node) {
		cur().Children = append(cur().Children, n)
		stack = append(stack, n)
	}
	pop := func() {
		if len(stack) > 1 {
			stack = stack[:len(stack)-1]
		}
	}
	add := func(n *Node) {
		cur().Children = append(cur().Children, n)
	}

	for _, tok := range tokens {
		switch tok.Kind {
		case TokLBracket:
			push(&Node{Kind: NdList})
		case TokRBracket:
			for len(stack) > 1 && cur().Kind != NdList {
				pop()
			}
			pop()
		case TokLBrace:
			push(&Node{Kind: NdMap})
		case TokRBrace:
			for len(stack) > 1 && cur().Kind != NdMap {
				pop()
			}
			pop()
		case TokLParen:
			push(&Node{Kind: NdParen})
		case TokRParen:
			for len(stack) > 1 && cur().Kind != NdParen {
				pop()
			}
			pop()
		case TokWord:
			add(&Node{Kind: NdWord, Text: tok.Text})
		case TokString:
			add(&Node{Kind: NdString, Text: tok.Text})
		case TokNumber:
			add(&Node{Kind: NdNumber, Text: tok.Text})
		case TokComment:
			add(&Node{Kind: NdComment, Text: tok.Text})
		case TokNewline:
			add(&Node{Kind: NdNewline})
		default:
			kind := tokenToNodeKind(tok.Kind)
			add(&Node{Kind: kind, Text: tok.Text})
		}
	}
	return root
}

func tokenToNodeKind(tk TokenKind) NodeKind {
	switch tk {
	case TokComma:
		return NdComma
	case TokColon:
		return NdColon
	case TokSemicolon:
		return NdSemicolon
	case TokDot:
		return NdDot
	case TokQuestion:
		return NdQuestion
	case TokBang:
		return NdBang
	case TokPipe:
		return NdPipe
	}
	return NdWord
}

// --- *Type capitalization ---

// capitalizeTypesInTree walks the tree and capitalizes known type
// names. A word is capitalized if it's a known type name AND:
//   - it appears after : (type annotation), OR
//   - it appears as a standalone word (not after def, not before :)
//
// Words are NOT capitalized when:
//   - immediately after "def" (variable name being defined)
//   - immediately before ":" (parameter/key name)
//   - they contain a "." (dotted word like table.kind)
//
// normaliseStatementEnd rewrites the statement terminator `end` to `;`
// (STYLE-GUIDE §S6). The two are the same token — the parser aliases `;` to
// `end` — so this is a spelling canonicalisation, like capitalizeTypesInTree
// above, and it runs on the TREE so both front ends (hand and tabnas) agree
// byte-for-byte.
//
// It is positional, because `end` is a perfectly good NAME in two places a
// spelling test alone cannot tell apart. Both are valid boru:
//
//	{end: 1}       a map KEY      — `end` immediately before a colon
//	m dot end      a field READ   — the accessor quotes the following word
//
// Rewriting either yields `{;: 1}` / `m dot ;`, which is a different program
// (or none). Everything else is the terminator.
func normaliseStatementEnd(n *Node) {
	for i, ch := range n.Children {
		if ch.Kind == NdWord && ch.Text == "end" && !isNameSlot(n.Children, i) {
			ch.Kind = NdSemicolon
			ch.Text = ";"
		}
		normaliseStatementEnd(ch)
	}
}

// isNameSlot reports whether kids[i] sits where a bare word is a NAME rather
// than a word to execute — a map key, or the field of an accessor. Both
// spelling rewriters (normaliseStatementEnd, capitalizeTypesInTree) ask this
// one question, because both are rewriting a word's SPELLING and both are
// wrong in exactly the same places. Keeping it in one function is what stops
// them drifting: the two shipped with the guard duplicated, and each was
// missing the same two positions.
//
// A word is in a name slot when it is:
//
//	{end: 1}    a map KEY        — immediately before `:`
//	{end?: 1}   an OPTIONAL key  — before `?` then `:`
//	m dot end   a field READ     — after the accessor word `dot` / `dotr`
//	m !.end     a strict-dot read — after a bare `.` token
//
// The last two are one rule with two spellings. The plain `.` sugar is safe
// on its own (`m.end` lexes as ONE dotted word, never a bare `end` node), but
// `!` breaks that merge: `m!.end` lexes as `m` `!` `.` `end`, leaving `end` a
// bare word after an NdDot. Guarding only the accessor WORDS missed it, and
// fmt turned `m!.end` into `m !.;` — output the parser rejects.
func isNameSlot(kids []*Node, i int) bool {
	// Map key: `end:` or `end?:`.
	if i+1 < len(kids) {
		if kids[i+1].Kind == NdColon {
			return true
		}
		if kids[i+1].Kind == NdQuestion &&
			i+2 < len(kids) && kids[i+2].Kind == NdColon {
			return true
		}
	}
	// Accessor field: after `dot` / `dotr`, or after a bare `.` token.
	if i > 0 {
		prev := kids[i-1]
		if prev.Kind == NdDot {
			return true
		}
		if prev.Kind == NdWord && (prev.Text == "dot" || prev.Text == "dotr") {
			return true
		}
	}
	return false
}

func capitalizeTypesInTree(n *Node) {
	for i, ch := range n.Children {
		if ch.Kind == NdWord && !strings.Contains(ch.Text, ".") {
			lower := strings.ToLower(ch.Text)
			if knownTypes[lower] && !isKeyword(lower) {
				// Skip if after "def" or "refine" (it's a name being defined).
				afterDef := i > 0 && n.Children[i-1].Kind == NdWord &&
					(n.Children[i-1].Text == "def" || n.Children[i-1].Text == "refine")
				// Skip if after a word that itself follows "refine" (e.g. refine Cond record).
				afterTypeName := i > 1 && n.Children[i-2].Kind == NdWord &&
					(n.Children[i-2].Text == "refine" || n.Children[i-2].Text == "def")
				// Skip a NAME slot — a map key (`{integer: 1}`, `{integer?: 1}`)
				// or an accessor field (`opts dot table`, `opts!.table`), which
				// QUOTE the bare word, so capitalising it would name a different
				// key. Shared with normaliseStatementEnd: same slots, same reason.
				if !afterDef && !afterTypeName && !isNameSlot(n.Children, i) {
					ch.Text = capitalize(lower)
				}
			}
		}
		// Recurse into containers.
		if ch.Kind == NdList || ch.Kind == NdMap || ch.Kind == NdParen || ch.Kind == NdRoot {
			capitalizeTypesInTree(ch)
		}
	}
}

// isKeyword returns true for type names that are also used as
// boru keywords and should not be auto-capitalized.
func isKeyword(lower string) bool {
	switch lower {
	case "record", "object", "function":
		return true
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// --- fn bracket elision ---

// elideFnBrackets rewrites single-parameter function definitions from the
// bracketed spec-list form `fn [[p] [r] [body]]` to the lighter triple
// form `fn p r [body]` — "single params (and returns) do not need
// brackets". Only a single-overload fn whose sole parameter can be
// rendered without brackets is rewritten; multi-parameter, multi-overload,
// empty-parameter, and sole-`List`-parameter fns keep the spec-list form
// (a bare `List` in the input slot is rejected by fn's `tnot List` guard,
// lang/spec/fn-triple.tsv). Runs after capitalizeTypesInTree; it only
// moves nodes, never rewrites their text, so capitalisation is preserved
// and the rewrite is idempotent (the triple form has no wrapper to match).
func elideFnBrackets(n *Node) {
	for _, ch := range n.Children {
		if ch.Kind == NdList || ch.Kind == NdMap || ch.Kind == NdParen || ch.Kind == NdRoot {
			elideFnBrackets(ch)
		}
	}
	var out []*Node
	i := 0
	for i < len(n.Children) {
		ch := n.Children[i]
		if ch.Kind == NdWord && ch.Text == "fn" && i+1 < len(n.Children) &&
			n.Children[i+1].Kind == NdList {
			if triple := elideFnTriple(n.Children[i+1]); triple != nil {
				out = append(out, ch)
				out = append(out, triple...)
				i += 2
				continue
			}
		}
		if ch.Kind == NdWord && ch.Text == "fn" {
			elideBareTripleRet(n.Children, i+1)
		}
		out = append(out, ch)
		i++
	}
	n.Children = out
}

// elideBareTripleRet is the wrapperless spelling's half of the same rule
// (NUR088): `fn x:Integer [Integer] [mul 2 x]` — a bare single-param run,
// a bracketed single-type return, the body — drops the return's brackets
// in place. The run must reach the return list on one line with no
// comment in it, and the body must follow the return directly; anything
// else is left as written.
func elideBareTripleRet(kids []*Node, from int) {
	j := from
	for j < len(kids) && kids[j].Kind != NdList {
		switch kids[j].Kind {
		case NdNewline, NdComment, NdParen, NdMap:
			return
		}
		j++
	}
	if j == from || j+1 >= len(kids) || kids[j+1].Kind != NdList {
		return
	}
	run := kids[from:j]
	if paramCount(run) != 1 || !paramUnbracketSafe(run) {
		return
	}
	kids[j] = unbracketRet(kids[j])
}

// elideFnTriple returns the bracketless triple-form node sequence for a
// single-overload fn wrapper whose sole parameter is safe to unbracket,
// or nil when the wrapper must be left as the spec-list form.
//
// One signature has several valid spellings — the params bracketed or a
// bare run, the return bracketed or a bare word — and they build one
// canon-identical value, so the formatter reads the wrapper as
// `params… ret body` (the body is the last list, the return the token
// before it, the params everything else) and collapses each of them to
// the one-pair form (NUR088). The irreducible shapes stay: two or more
// params, none, a value-pattern list, a bare `List` param, and a
// multi-overload spec list (whose "params" are lists, which count as no
// param head at all).
func elideFnTriple(wrapper *Node) []*Node {
	parts := nonTrivial(wrapper.Children)
	if len(parts) < 3 {
		return nil
	}
	body, ret := parts[len(parts)-1], parts[len(parts)-2]
	if body.Kind != NdList || (ret.Kind != NdList && ret.Kind != NdWord) {
		return nil
	}
	params := parts[:len(parts)-2]
	var pkids []*Node
	if len(params) == 1 && params[0].Kind == NdList {
		pkids = nonTrivial(params[0].Children)
	} else {
		for _, p := range params {
			if p.Kind == NdList || p.Kind == NdParen {
				return nil
			}
		}
		pkids = params
	}
	if paramCount(pkids) != 1 || !paramUnbracketSafe(pkids) {
		return nil
	}
	out := append([]*Node{}, pkids...)
	out = append(out, unbracketRet(ret))
	out = append(out, body)
	return out
}

// paramCount counts parameter heads in a params-list's children: a name
// or bare-type word (not the type sitting after a `:`), a `{…}` map
// param, or a value-pattern literal each count once.
func paramCount(kids []*Node) int {
	n := 0
	for i, k := range kids {
		switch k.Kind {
		case NdMap:
			n++
		case NdWord, NdNumber, NdString:
			if !(i > 0 && kids[i-1].Kind == NdColon) {
				n++
			}
		}
	}
	return n
}

// paramUnbracketSafe reports whether a sole parameter can render without
// its list brackets. A nested list/paren (a value-pattern list) or a bare
// unnamed `List` type must stay bracketed.
func paramUnbracketSafe(kids []*Node) bool {
	for _, k := range kids {
		if k.Kind == NdList || k.Kind == NdParen {
			return false
		}
	}
	if len(kids) == 1 && kids[0].Kind == NdWord && kids[0].Text == "List" {
		return false
	}
	return true
}

// unbracketRet drops the brackets around a single-type return list,
// returning the bare type word; a multi-token or non-word return keeps
// its list.
func unbracketRet(ret *Node) *Node {
	kids := nonTrivial(ret.Children)
	if len(kids) == 1 && kids[0].Kind == NdWord {
		return kids[0]
	}
	return ret
}

// --- Emitter ---

func (r *renderer) emitRoot(n *Node, indent int) string {
	stmts := splitStatements(n.Children)
	var lines []string
	prevBlank := false
	for _, stmt := range stmts {
		if len(stmt) == 0 {
			if !prevBlank && len(lines) > 0 {
				lines = append(lines, "")
				prevBlank = true
			}
			continue
		}
		prevBlank = false
		lines = append(lines, r.emitStatement(stmt, indent))
	}
	result := strings.Join(lines, "\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

func splitStatements(children []*Node) [][]*Node {
	var stmts [][]*Node
	var cur []*Node
	for _, ch := range children {
		if ch.Kind == NdNewline {
			stmts = append(stmts, cur)
			cur = nil
			continue
		}
		cur = append(cur, ch)
	}
	if len(cur) > 0 {
		stmts = append(stmts, cur)
	}
	return stmts
}

// emitStatement lays out one statement by trying the rule table's layout
// STRATEGIES in their declared order — the XSLT-style template dispatch.
// The first strategy that claims the statement produces it; an unknown
// strategy name is inert; if none claims it, the statement stays inline.
func (r *renderer) emitStatement(nodes []*Node, indent int) string {
	if len(nodes) == 0 {
		return ""
	}
	inline := pad(indent) + r.renderInline(nodes, indent)
	for _, name := range r.ru.Strategies {
		if s, ok := r.strategy(name, nodes, indent, inline); ok {
			return s
		}
	}
	return inline
}

// strategy applies one named statement template. The bool reports whether
// the template claimed the statement.
func (r *renderer) strategy(name string, nodes []*Node, indent int, inline string) (string, bool) {
	switch name {
	case "comment-only":
		// A comment-only statement renders as just that comment. This must
		// NOT fire when tokens follow the comment: a statement whose first
		// node is a comment but which carries more nodes is a real statement
		// whose tail would be dropped. (A LINE comment runs to end of line,
		// so a statement starting with one is always comment-only anyway —
		// len(nodes) == 1.)
		if len(nodes) == 1 && nodes[0].Kind == NdComment {
			return pad(indent) + nodes[0].Text, true
		}
	case "inline":
		if len(inline) <= r.ru.Width {
			return inline, true
		}
	case "trailing-comment":
		// A statement ending in a trailing comment stays on one line even
		// when it overflows the width: the comment annotates the whole
		// statement (as in `expr # returns X` doc examples) and must never
		// be wrapped onto its own line, detached from what it annotates.
		if nodes[len(nodes)-1].Kind == NdComment {
			return inline, true
		}
	case "fn":
		if s := r.stratFn(nodes, indent); s != "" {
			return s, true
		}
	case "trailing-container":
		if s := r.stratTrailingContainer(nodes, indent); s != "" {
			return s, true
		}
	case "wrap":
		return r.wrapStatement(nodes, indent), true
	}
	return "", false
}

// stratFn handles def name fn [[args] [returns] [body]] formatting.
// The wrapper list contains three inner lists (one signature triple).
// Returns "" if not applicable. The trigger word and the wrapper brackets
// come from the rule table (FnWord, ListOpen/ListClose).
func (r *renderer) stratFn(nodes []*Node, indent int) string {
	// Look for pattern: ... fn [wrapper]
	fnIdx := -1
	for i, n := range nodes {
		if n.Kind == NdWord && n.Text == r.ru.FnWord {
			fnIdx = i
			break
		}
	}
	if fnIdx < 0 || fnIdx+1 >= len(nodes) {
		return ""
	}

	wrapper := nodes[fnIdx+1]
	if wrapper.Kind != NdList {
		return ""
	}

	// Only the pristine case is handled here: the fn wrapper must be the LAST
	// node of the statement. stratFn renders just `header [args][ret][body]`,
	// so any node after the wrapper (a trailing `end`, a following statement on
	// the same line, another call) would be SILENTLY DROPPED. Decline instead
	// and let wrapStatement — which emits every node — format the whole thing.
	if fnIdx+2 != len(nodes) {
		return ""
	}

	// Extract the inner lists from the wrapper.
	var inner []*Node
	for _, ch := range wrapper.Children {
		if ch.Kind == NdList {
			inner = append(inner, ch)
		}
	}
	// Only a single signature triple ([args] [returns] [body]) is handled;
	// this renderer emits only inner[0..2], so a multi-overload wrapper (6, 9,
	// … inner lists) would drop every overload past the first. Decline and let
	// the general path preserve them all.
	if len(inner) != 3 {
		return ""
	}

	prefix := pad(indent)

	// Header: everything before fn + fn + [args] [returns]
	headerParts := nodes[:fnIdx+1]
	headerStr := r.renderInline(headerParts, indent)
	argsStr := r.emitNode(inner[0], indent)
	retStr := r.emitNode(inner[1], indent)
	header := prefix + headerStr + " " + r.ru.ListOpen + argsStr + " " + retStr

	body := inner[2]
	bodyChildren := nonTrivial(body.Children)

	// No single-line return here: stratFn is reached only after
	// emitStatement's renderInline of the whole statement already overflowed
	// the width, and for this pristine single-overload shape that render
	// equals the reconstructed `header [args][ret][body]` one-liner — so it
	// never fits. The fn always lays out multi-line from here.

	// The body opens with " [". When the header line has room, keep the
	// bracket on it; otherwise wrap the header, carrying the body-open
	// bracket on the last wrapped piece so no line overflows the width.
	openLine := header + " " + r.ru.ListOpen
	if len(header)+1+len(r.ru.ListOpen) > r.ru.Width {
		openLine = r.wrapStatement(append(headerParts,
			&Node{Kind: NdWord, Text: r.ru.ListOpen + argsStr},
			&Node{Kind: NdWord, Text: retStr + " " + r.ru.ListOpen},
		), indent)
	}

	// Multi-line: header [
	//   body
	// ]]
	bodyIndent := indent + r.ru.Indent
	groups := r.splitIntoGroups(bodyChildren)
	var lines []string
	lines = append(lines, openLine)
	for _, grp := range groups {
		grpLine := pad(bodyIndent) + r.renderInline(grp, bodyIndent)
		if len(grpLine) <= r.ru.Width {
			lines = append(lines, grpLine)
		} else {
			lines = append(lines, r.wrapStatement(grp, bodyIndent))
		}
	}
	lines = append(lines, prefix+r.ru.ListClose+r.ru.ListClose)

	return strings.Join(lines, "\n")
}

// renderInline renders nodes on a single line with proper attachment.
func (r *renderer) renderInline(nodes []*Node, indent int) string {
	var parts []string
	for i, n := range nodes {
		s := r.emitNode(n, indent)
		if r.attach(nodes, i) && len(parts) > 0 {
			parts[len(parts)-1] += s
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// attach reports whether the node at index i glues to the previous token
// (no space before it). The classes are DATA — the rule table's AttachPrev
// / AttachNext kind sets plus the AttachDotSuffix word rule — so the
// whitespace policy is part of the stylesheet, not the processor.
func (r *renderer) attach(nodes []*Node, i int) bool {
	if r.attachPrev[nodes[i].Kind] {
		return true
	}
	// After an attach-next kind (colon, dot), glue to it.
	if i > 0 && r.attachNext[nodes[i-1].Kind] {
		return true
	}
	// If previous word ends with '.', attach (e.g., input.( → no space).
	if r.ru.AttachDotSuffix && i > 0 && nodes[i-1].Kind == NdWord &&
		strings.HasSuffix(nodes[i-1].Text, ".") {
		return true
	}
	return false
}

// emitNode renders one node at one indent, memoised.
//
// The memo is load-bearing, not an optimisation. Layout here is
// try-then-fall-back: emitStatement renders the whole statement inline
// (`inline`, :912) before any strategy can reject it, and when the width
// strategy does reject it wrapStatement re-renders every child at the
// continuation indent (:1206); emitList and emitParen do the same
// single-line-then-broken dance for their children. So without a cache a
// subtree nested D deep is rendered on the order of 2^D times — measured at
// 53 s for kg/validate.boru (749 lines, nesting ~10), against 48 ms to PARSE
// the same file. Memoising collapses that to one render per (node, indent).
//
// Soundness: the renderer is immutable after newRenderer, and the node tree
// is only mutated during parsing/normalisation (format.go :624-:793), never
// during rendering — so (n, indent) fully determines the result.
func (r *renderer) emitNode(n *Node, indent int) string {
	key := emitKey{n: n, indent: indent}
	if s, ok := r.memo[key]; ok {
		return s
	}
	s := r.emitNodeUncached(n, indent)
	r.memo[key] = s
	return s
}

func (r *renderer) emitNodeUncached(n *Node, indent int) string {
	switch n.Kind {
	case NdRoot:
		return r.emitRoot(n, indent)
	case NdList:
		return r.emitList(n, indent)
	case NdMap:
		return r.emitMap(n, indent)
	case NdParen:
		return r.emitParen(n, indent)
	case NdWord, NdNumber, NdString:
		return n.Text
	case NdComment:
		return n.Text
	case NdComma, NdColon, NdSemicolon, NdDot, NdQuestion, NdBang, NdPipe, NdNewline:
		// Punctuation emits its TEMPLATE's literal (`comma:[',']` in the
		// stylesheet); newline's template is empty.
		return r.glyph(n.Kind)
	}
	return ""
}

// stratTrailingContainer handles statements ending with a map or list.
// Keeps the opening bracket on the same line as preceding tokens.
func (r *renderer) stratTrailingContainer(nodes []*Node, indent int) string {
	if len(nodes) < 2 {
		return ""
	}
	last := nodes[len(nodes)-1]
	if last.Kind != NdMap && last.Kind != NdList {
		return ""
	}

	prefix := pad(indent)
	head := nodes[:len(nodes)-1]
	headStr := r.renderInline(head, indent)
	container := last

	if container.Kind == NdMap {
		children := nonTrivial(container.Children)
		entries := parseMapEntries(children)

		// Try single line — but never with a line comment inside (it would
		// swallow the closing brace). Under the canonical strategy order the
		// `inline` template has already claimed anything that fits, so this
		// fires only for rule tables that reorder or drop `inline`.
		single := prefix + headStr + " " + r.ru.MapOpen + r.renderMapEntries(entries, indent) + r.ru.MapClose
		if len(single) <= r.ru.Width && !hasLineComment(children) {
			return single
		}

		// Multi-line with { on header line.
		childIndent := indent + r.ru.Indent
		childPfx := pad(childIndent)
		var lines []string
		lines = append(lines, prefix+headStr+" "+r.ru.MapOpen)
		for _, e := range entries {
			lines = append(lines, childPfx+r.renderMapEntry(e, childIndent))
		}
		lines = append(lines, prefix+r.ru.MapClose)
		return strings.Join(lines, "\n")
	}

	if container.Kind == NdList {
		children := nonTrivial(container.Children)
		inner := r.renderInline(children, indent)
		single := prefix + headStr + " " + r.ru.ListOpen + inner + r.ru.ListClose
		// Same comment guard and reachability as the map branch above.
		if len(single) <= r.ru.Width && !hasLineComment(children) {
			return single
		}

		// Multi-line with [ on header line.
		groups := r.splitIntoGroups(children)
		bodyIndent := indent + r.ru.Indent
		var lines []string
		lines = append(lines, prefix+headStr+" "+r.ru.ListOpen)
		for _, grp := range groups {
			grpLine := pad(bodyIndent) + r.renderInline(grp, bodyIndent)
			if len(grpLine) <= r.ru.Width {
				lines = append(lines, grpLine)
			} else {
				lines = append(lines, r.wrapStatement(grp, bodyIndent))
			}
		}
		lines = append(lines, prefix+r.ru.ListClose)
		return strings.Join(lines, "\n")
	}

	return "" //covergate:allow formatter inline/tokenizer defensive guard (§formatter)
}

// wrapStatement breaks a long statement across multiple lines with a
// greedy fill: tokens flow onto the current line until the next one would
// exceed the width, then continue on a hanging-indented line. This is the
// `wrap` layout template — the fill-style whitespace the rules own (an
// all-or-nothing group algebra cannot express it; a text-emitting template
// trivially does).
func (r *renderer) wrapStatement(nodes []*Node, indent int) string {
	prefix := pad(indent)
	contPrefix := pad(indent + r.ru.Indent)

	var lines []string
	var cur []string
	curLen := indent

	flush := func(nextPrefix string) {
		if len(cur) > 0 {
			lines = append(lines, prefix+strings.Join(cur, " "))
			cur = nil
			curLen = len(nextPrefix)
			prefix = nextPrefix
		}
	}

	for i, n := range nodes {
		s := r.emitNode(n, indent+r.ru.Indent)
		if r.attach(nodes, i) && len(cur) > 0 {
			cur[len(cur)-1] += s
			curLen += len(s)
			continue
		}

		tokenLen := len(s) + 1
		if curLen+tokenLen > r.ru.Width && len(cur) > 0 {
			flush(contPrefix)
		}
		cur = append(cur, s)
		curLen += tokenLen
		// A line comment runs to end of line, so nothing may follow it on
		// this line — flush so the next token starts fresh.
		if n.Kind == NdComment {
			flush(contPrefix)
		}
	}
	flush(contPrefix)
	return strings.Join(lines, "\n")
}

// emitList formats [...].
func (r *renderer) emitList(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return r.ru.ListOpen + r.ru.ListClose
	}

	// Try single line — but not when a line comment would swallow the `]`.
	if !hasLineComment(children) {
		inner := r.renderInline(children, indent)
		single := r.ru.ListOpen + inner + r.ru.ListClose
		if len(single)+indent <= r.ru.Width {
			return single
		}
	}

	// Multi-line. Split children into logical groups for wrapping.
	groups := r.splitIntoGroups(children)
	childIndent := indent + r.ru.Indent
	childPfx := pad(childIndent)

	var lines []string
	for gi, grp := range groups {
		line := r.renderInline(grp, childIndent)
		full := childPfx + line
		if len(full) <= r.ru.Width {
			if gi == 0 {
				lines = append(lines, r.ru.ListOpen+line)
			} else {
				lines = append(lines, full)
			}
		} else {
			wrapped := r.wrapStatement(grp, childIndent)
			if gi == 0 {
				// Put [ on first line of wrapped.
				wlines := strings.Split(wrapped, "\n")
				wlines[0] = r.ru.ListOpen + strings.TrimLeft(wlines[0], " ")
				lines = append(lines, strings.Join(wlines, "\n"))
			} else {
				lines = append(lines, wrapped)
			}
		}
	}
	lines = append(lines, pad(indent)+r.ru.ListClose)
	return strings.Join(lines, "\n")
}

// splitIntoGroups splits a list's children into logical statement groups.
// A new group starts at the rule table's statement-start words.
func (r *renderer) splitIntoGroups(children []*Node) [][]*Node {
	var groups [][]*Node
	var cur []*Node

	for _, ch := range children {
		if ch.Kind == NdWord && r.stmtStart[ch.Text] && len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, ch)
		// A line comment ends its line, so it closes the current group — the
		// next token must begin a new group (and thus a new line).
		if ch.Kind == NdComment {
			groups = append(groups, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	if len(groups) == 0 {
		return [][]*Node{children}
	}
	return groups
}

// emitMap formats {...}.
func (r *renderer) emitMap(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return r.ru.MapOpen + r.ru.MapClose
	}

	// Parse key:value entries.
	entries := parseMapEntries(children)

	// Try single line — but not when a line comment would swallow the `}`.
	single := r.ru.MapOpen + r.renderMapEntries(entries, indent) + r.ru.MapClose
	if len(single)+indent <= r.ru.Width && !hasLineComment(children) {
		return single
	}
	// A comma-only map has children but no entries; there is nothing to lay
	// out multi-line, so keep the single-line render (reachable only through
	// a hostile rule table — e.g. width 0 — never at the canonical width).
	if len(entries) == 0 {
		return single
	}

	// Multi-line: first entry on { line, rest indented.
	childIndent := indent + r.ru.Indent
	childPfx := pad(childIndent)
	var lines []string
	first := r.renderMapEntry(entries[0], childIndent)
	lines = append(lines, r.ru.MapOpen+first)
	for _, e := range entries[1:] {
		lines = append(lines, childPfx+r.renderMapEntry(e, childIndent))
	}
	lines = append(lines, pad(indent)+r.ru.MapClose)
	return strings.Join(lines, "\n")
}

type mapEntry struct {
	key      string
	optional bool
	value    *Node
	comment  *Node // standalone comment
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/v → foo, foo → foo). Mirrors the
// parser's wordBaseName so shorthand keys render the same on both sides.
func wordBaseName(text string) string {
	idx := strings.LastIndex(text, "/")
	if idx < 0 || idx >= len(text)-1 {
		return text
	}
	mod := text[idx+1:]
	seenDigits, forceSet, qvSet := false, false, false
	i := 0
	for i < len(mod) {
		c := mod[i]
		switch {
		case c >= '0' && c <= '9':
			if seenDigits {
				return text
			}
			for i < len(mod) && mod[i] >= '0' && mod[i] <= '9' {
				i++
			}
			seenDigits = true
			continue
		case c == 'f' || c == 's':
			if forceSet {
				return text
			}
			forceSet = true
		case c == 'q' || c == 'v':
			if qvSet {
				return text
			}
			qvSet = true
		default:
			return text
		}
		i++
	}
	return text[:idx]
}

func parseMapEntries(children []*Node) []mapEntry {
	var entries []mapEntry
	i := 0
	for i < len(children) {
		ch := children[i]
		if ch.Kind == NdComma {
			i++
			continue
		}
		if ch.Kind == NdComment {
			entries = append(entries, mapEntry{comment: ch})
			i++
			continue
		}
		// key ? : value
		if i+3 < len(children) &&
			children[i+1].Kind == NdQuestion &&
			children[i+2].Kind == NdColon {
			entries = append(entries, mapEntry{
				key:      ch.Text,
				optional: true,
				value:    children[i+3],
			})
			i += 4
			continue
		}
		// key : value
		if i+2 < len(children) && children[i+1].Kind == NdColon {
			entries = append(entries, mapEntry{
				key:   ch.Text,
				value: children[i+2],
			})
			i += 3
			continue
		}
		// Shorthand optional: `{foo?}` ≡ `{foo?: foo}` (a key followed by
		// `?` with no colon). The `key ? : value` case above already
		// handled the explicit form.
		if ch.Kind == NdWord && i+1 < len(children) && children[i+1].Kind == NdQuestion {
			entries = append(entries, mapEntry{
				key:      wordBaseName(ch.Text),
				optional: true,
				value:    ch,
			})
			i += 2
			continue
		}
		// Shorthand: `{foo}` ≡ `{foo: foo}`, `{foo/v}` ≡ `{foo: foo/v}`.
		// A lone identifier becomes a key:value pair whose key is the base
		// name and value is the full token. Skip a word that is the type
		// of a typed-map child (`{:Type}`) — it is preceded by a colon and
		// must stay key-less.
		if ch.Kind == NdWord && !(i > 0 && children[i-1].Kind == NdColon) {
			entries = append(entries, mapEntry{
				key:   wordBaseName(ch.Text),
				value: ch,
			})
			i++
			continue
		}
		// Bare value (typed map child syntax).
		entries = append(entries, mapEntry{
			value: ch,
		})
		i++
	}
	return entries
}

func (r *renderer) renderMapEntries(entries []mapEntry, indent int) string {
	var parts []string
	for _, e := range entries {
		parts = append(parts, r.renderMapEntry(e, indent))
	}
	return strings.Join(parts, " ")
}

func (r *renderer) renderMapEntry(e mapEntry, indent int) string {
	if e.comment != nil {
		return e.comment.Text
	}
	if e.key == "" {
		return r.emitNode(e.value, indent)
	}
	opt := ""
	if e.optional {
		opt = r.glyph(NdQuestion)
	}
	return e.key + opt + r.glyph(NdColon) + r.emitNode(e.value, indent)
}

// emitParen formats (...).
func (r *renderer) emitParen(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return r.ru.ParenOpen + r.ru.ParenClose
	}

	// A line comment inside the group forces multi-line: on one line the
	// closing `)` would land after `# …` and be commented out.
	if !hasLineComment(children) {
		inner := r.renderInline(children, indent)
		single := r.ru.ParenOpen + inner + r.ru.ParenClose
		if len(single)+indent <= r.ru.Width {
			return single
		}
	}

	childIndent := indent + r.ru.Indent
	var lines []string
	wrapped := r.wrapStatement(children, childIndent)
	wlines := strings.Split(wrapped, "\n")
	wlines[0] = r.ru.ParenOpen + strings.TrimLeft(wlines[0], " ")
	lines = append(lines, wlines...)
	lines = append(lines, pad(indent)+r.ru.ParenClose)
	return strings.Join(lines, "\n")
}

// hasLineComment reports whether nodes contain a LINE comment (`# …`, which
// runs to end of line). Such a comment poisons any single-line rendering: a
// closing delimiter or a following token placed after it on the same line is
// swallowed into the comment (e.g. `(a # c)` comments out the `)`). Block
// comments (`## … ##`) are bounded and do not trigger this. Callers that
// would join tokens onto one line must break instead when this is true.
func hasLineComment(nodes []*Node) bool {
	for _, n := range nodes {
		if n.Kind == NdComment {
			return true
		}
	}
	return false
}

// nonTrivial filters out newlines. Commas are KEPT — they are real layout
// tokens (renderInline glues them to the previous token; parseMapEntries
// skips them per-entry), which is why a comma-only map still has children
// yet zero entries (see emitMap's empty-entries guard).
func nonTrivial(nodes []*Node) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Kind != NdNewline {
			out = append(out, n)
		}
	}
	return out
}
