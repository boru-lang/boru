package parser

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	core "github.com/boru-lang/boru/core/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// The tabnas jsonic engine exposes RuleSpec alternates and actions through
// accessor methods (AddOpen/ClearOpen/OpenAlts, AddBO/ClearActions, …) rather
// than the exported Open/Close/BO/BC slice fields the legacy jsonicjs parser
// used. These helpers re-create the field-assignment idioms the boru grammar is
// written against, so the rule definitions read the same:
//
//	setOpen(rs, alts)      — replace the open alternates       (was rs.Open = alts)
//	prependOpen(rs, alts)  — prepend before existing opens     (was rs.Open = append(alts, rs.Open...))
//	rs.AddOpen(alt)        — append one open alternate         (was rs.Open = append(rs.Open, alt))
//	setBO(rs, actions)     — replace the before-open actions   (was rs.BO = actions)
//
// (…and the Close / BC / AC / AO equivalents.)
func setOpen(rs *jsonic.RuleSpec, alts []*jsonic.AltSpec) { rs.ClearOpen(); rs.AddOpen(alts...) }
func setClose(rs *jsonic.RuleSpec, alts []*jsonic.AltSpec) {
	rs.ClearClose()
	rs.AddClose(alts...)
}

func prependOpen(rs *jsonic.RuleSpec, alts []*jsonic.AltSpec) {
	existing := rs.OpenAlts()
	rs.ClearOpen()
	rs.AddOpen(append(alts, existing...)...)
}

func prependClose(rs *jsonic.RuleSpec, alts []*jsonic.AltSpec) {
	existing := rs.CloseAlts()
	rs.ClearClose()
	rs.AddClose(append(alts, existing...)...)
}

func setBO(rs *jsonic.RuleSpec, actions []jsonic.StateAction) {
	rs.ClearActions("bo")
	for _, a := range actions {
		rs.AddBO(a)
	}
}

func setBC(rs *jsonic.RuleSpec, actions []jsonic.StateAction) {
	rs.ClearActions("bc")
	for _, a := range actions {
		rs.AddBC(a)
	}
}

func setAC(rs *jsonic.RuleSpec, actions []jsonic.StateAction) {
	rs.ClearActions("ac")
	for _, a := range actions {
		rs.AddAC(a)
	}
}

// addMatcher re-creates jsonicjs's AddMatcher(name, priority, fn): it appends a
// priority-ordered custom lex matcher. The tabnas lexer interleaves
// Config.CustomMatchers by priority against the built-in matchers using the
// SAME bands (match=1e6, fixed=2e6, space=3e6, …), so the boru matchers keep
// their exact relative ordering.
func addMatcher(j *jsonic.Jsonic, name string, priority int, fn jsonic.LexMatcher) {
	cfg := j.Config()
	cfg.CustomMatchers = append(cfg.CustomMatchers,
		&jsonic.MatcherEntry{Name: name, Priority: priority, Match: fn})
	sort.Slice(cfg.CustomMatchers, func(i, k int) bool {
		return cfg.CustomMatchers[i].Priority < cfg.CustomMatchers[k].Priority
	})
}

// parserTokens holds the custom jsonic token IDs registered for boru grammar.
// Passed between grammar setup stages so token IDs are defined once.
type parserTokens struct {
	OP  jsonic.Tin // (
	CP  jsonic.Tin // )
	DT  jsonic.Tin // .
	SC  jsonic.Tin // ;
	QM  jsonic.Tin // ?
	BG  jsonic.Tin // !
	PI  jsonic.Tin // |
	AR  jsonic.Tin // => (lambda arrow → aliases the word `afn`)
	BT  jsonic.Tin // ` (backtick)
	IS  jsonic.Tin // ${ (interp start)
	TL  jsonic.Tin // template literal segment
	LA  jsonic.Tin // < (angle open — general-purpose token, D14)
	RA  jsonic.Tin // > (angle close)
	ML  jsonic.Tin // minilang literal: +name<delim>src<delim> (matcher-produced)
	XML jsonic.Tin // embedded XML literal <tag>…</tag> (matcher-produced)
}

// setupBaseTokens registers the boru token table from the declarative
// grammar artifact (grammar.json — fixed tokens with their source
// text, matcher-produced tokens by name; `<` / `>` are general-purpose
// tokens per design/legacy/GENERICS.10.ignore D14, and #XML carries a whole
// embedded literal filled by the xml_literal matcher) and removes
// backtick from jsonic's string/multi chars so template strings are
// handled by custom rules. Returns the struct view alongside the
// name → Tin map the declarative edits resolve against.
func setupBaseTokens(j *jsonic.Jsonic, g declGrammar) (parserTokens, map[string]jsonic.Tin) {
	// Remove backtick from string chars so jsonic doesn't consume backtick
	// strings with the built-in string matcher. Template strings are handled
	// by custom tokens and rules below.
	delete(j.Config().StringChars, '`')
	delete(j.Config().MultiChars, '`')

	tins := registerDeclTokens(j, g)
	return parserTokens{
		OP: tins["OP"], CP: tins["CP"], DT: tins["DT"], SC: tins["SC"],
		QM: tins["QM"], BG: tins["BG"], PI: tins["PI"], AR: tins["AR"],
		BT: tins["BT"], IS: tins["IS"], TL: tins["TL"], LA: tins["LA"],
		RA: tins["RA"], ML: tins["ML"], XML: tins["XML"],
	}, tins
}

// valDeclHooks binds the named behavior the declarative val edits
// reference: the minilang node carry, the lambda-arrow tag, and the
// map-pair-value gate for the dot-chain fold.
func valDeclHooks() declHooks {
	return declHooks{
		Actions: map[string]func(*jsonic.Rule, *jsonic.Context){
			"minilangNode": func(r *jsonic.Rule, _ *jsonic.Context) {
				r.Node = r.O0.Val
			},
			"arrowTag": func(r *jsonic.Rule, _ *jsonic.Context) {
				r.Node = arrowTag{}
			},
		},
		Conds: map[string]func(*jsonic.Rule, *jsonic.Context) bool{
			"inPairValue": func(r *jsonic.Rule, _ *jsonic.Context) bool {
				return r.Parent != nil && r.Parent != jsonic.NoRule && r.Parent.Name == "pair"
			},
		},
	}
}

// setupBigNumberMatcher registers a high-priority lex matcher that claims a
// whole `0d…` literal — `[+-]?0[dD][0-9_]+(\.[0-9_]+)?([eE][+-]?[0-9_]+)?` —
// as a single #BD token BEFORE jsonic's number matcher or the `.` (#DT)
// fixed token can split it (jsonic does not know the `0d` prefix, and would
// otherwise break `0d12.5` into `0d12` · `.` · `5`). The token carries a
// bigNumberVal; the val rule (setupValRule) turns it into r.Node, and
// convertTopLevelValueInner / convertDataValue route it to bigNumberToValue.
// A trailing `.` is consumed ONLY when followed by a digit, so `0d5.foo`
// stays `0d5` + dot-access. Runs only outside template strings.
func setupBigNumberMatcher(j *jsonic.Jsonic, t parserTokens) {
	isDigit := func(c byte) bool { return (c >= '0' && c <= '9') || c == '_' }
	addMatcher(j, "big_number", 1000001, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule != nil {
			if _, ok := rule.K["boru_tpl"]; ok {
				return nil // inside a template string: leave to the literal matcher
			}
		}
		cursor := lex.Cursor()
		s := lex.Src
		si := cursor.SI
		start := si
		if si < len(s) && (s[si] == '+' || s[si] == '-') {
			si++ // optional sign, glued to the literal
		}
		// Require the 0d / 0D prefix.
		if si+1 >= len(s) || s[si] != '0' || (s[si+1] != 'd' && s[si+1] != 'D') {
			return nil
		}
		si += 2
		// Require at least one integer digit.
		if si >= len(s) || !isDigit(s[si]) {
			return nil
		}
		for si < len(s) && isDigit(s[si]) {
			si++
		}
		// Optional fraction — consume `.` only when a digit follows, so a
		// dot-access (`0d5.foo`) is left for the #DT token.
		if si+1 < len(s) && s[si] == '.' && isDigit(s[si+1]) {
			si++
			for si < len(s) && isDigit(s[si]) {
				si++
			}
		}
		// Optional exponent.
		if si < len(s) && (s[si] == 'e' || s[si] == 'E') {
			j2 := si + 1
			if j2 < len(s) && (s[j2] == '+' || s[j2] == '-') {
				j2++
			}
			if j2 < len(s) && isDigit(s[j2]) {
				si = j2
				for si < len(s) && isDigit(s[si]) {
					si++
				}
			}
		}
		src := s[start:si]
		// Emit the whole run as ONE text token. jsonic's val rule already
		// opens on text, so it flows to parseWord, which recognises the
		// `0d` prefix and builds the BigInteger/BigDecimal. The matcher's
		// only job is to claim the run so the embedded `.` isn't split off.
		tkn := lex.Token("#TX", jsonic.TinTX, src, src)
		cursor.SI = si
		cursor.CI += si - start // a 0d literal never spans newlines
		return tkn
	})
}

// setupDecimalUnderscoreMatcher is the decimal-number boundary shim shared
// conceptually with the TS port. The historical name comes from its original
// narrow job (keeping underscore-bearing runs whole), but it also closes the
// three places where the Go and TS jsonic number matchers otherwise disagree:
//
//   - a dot followed by a word starts member access (`1.x` is #NR `1` + `.`),
//     so conversion can reject a numeric receiver with boru's own diagnostic;
//   - a trailing-dot exponent is one Float (`1.e2`), as required by the
//     trailing-dot and scientific-notation rules;
//   - binary64 overflow remains a #NR token (`1e400`, `1.0e400`) instead of
//     falling back to text merely because strconv reports ErrRange.
//
// It also claims 0x/0o/0b runs that Go's stock lexer drops to text on
// overflow, preserving the same raw #NR boundary TS exposes. Boru's 0d
// literals remain on their dedicated matcher. Separator placement is
// deliberately loose here: numberValToValue owns the language rule and
// reports a misplaced `_` using the complete literal source.
func setupDecimalUnderscoreMatcher(j *jsonic.Jsonic, _ parserTokens) {
	setupDecimalBoundaryMatcher(j, false)
}

// setupDataDecimalMatcher is the DATA-mode variant of the decimal boundary
// shim, for plain-jsonic decode seams (SafeParseData). Data grammars have no
// `.` member-access token and promise jsonic's lenient superset, so this
// mode claims a run only when it is a complete numeric token at a data
// boundary and otherwise DECLINES — it never splits. Digit-led prose such as
// `1.x`, `1.2.3` or `1.2-beta` falls back whole to the stock lenient text
// scanner instead of erroring or shedding a stray second value.
func setupDataDecimalMatcher(j *jsonic.Jsonic) {
	setupDecimalBoundaryMatcher(j, true)
}

// setupDecimalBoundaryMatcher registers the shared matcher body. dataMode
// selects the boundary alphabet and the no-split rule described on the two
// wrappers above; the language mode is bit-for-bit the historical behavior.
func setupDecimalBoundaryMatcher(j *jsonic.Jsonic, dataMode bool) {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	isDigitOrSep := func(c byte) bool { return isDigit(c) || c == '_' }
	isFollowingText := func(s string, i int) bool {
		if i >= len(s) {
			return false
		}
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\'', '"', '#', ',', ':', '[', ']', '{', '}', '`':
			return false
		case ';', '(', ')', '?', '.', '!', '|', '<', '>':
			// Language operators end a token only in language mode; a plain
			// data grammar lexes them as ordinary text continuation.
			return dataMode
		case '=':
			if dataMode {
				return true
			}
			return i+1 >= len(s) || s[i+1] != '>'
		default:
			return true
		}
	}

	addMatcher(j, "decimal_underscore", 1000004, func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
		cursor := lex.Cursor()
		s := lex.Src
		start := cursor.SI
		si := start
		if si >= len(s) {
			return nil
		}

		hasSign := s[si] == '+' || s[si] == '-'
		if hasSign {
			si++
			if si >= len(s) {
				return nil
			}
		}

		leadingDot := s[si] == '.'
		integerEnd := -1
		if leadingDot {
			// Keep the bare `.5` path on the fixed-dot matcher so it retains
			// the receiverless-dot diagnostic. Signed forms are one numeric
			// token and are rejected later with their complete source.
			if !hasSign || si+1 >= len(s) || !isDigit(s[si+1]) {
				return nil
			}
			si++
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
		} else {
			if !isDigit(s[si]) {
				return nil
			}
			// Big numbers keep their dedicated matcher (which has higher
			// priority); decline here as well so this matcher is safe alone.
			if s[si] == '0' && si+1 < len(s) && strings.ContainsRune("dD", rune(s[si+1])) {
				return nil
			}
			// NO base-prefix arm here, deliberately. `0x`/`0o`/`0b` runs go
			// to the stock scanners, which as of tabnas parser v0.8.3 agree
			// across the ports on every shape boru cares about — including
			// the dot boundary (`0xFF.5`, `0xFF.e5`, a trailing `0xFF.`) and
			// uppercase markers (`0XFF`), the two divergences that kept an
			// arm here until then. Both were reported upstream and fixed
			// there rather than shimmed, per ADR-014; NUR.md §NUR061 records
			// the close.
			//
			// Retirement was proven by the CROSS-PORT probe, not by these
			// suites: 25 shapes AGREE under scripts/parity-probe.sh,
			// parser-crossdiff is IDENTICAL over 1765 rows, and — the part
			// that was missing when an earlier retirement attempt looked
			// safe and was not — parser/spec/lex.tsv's §base-prefixed
			// boundaries rows now COVER the diverging shapes, so their
			// passing is evidence instead of silence. Keep those rows: they
			// are what would catch a regression if the upstream fix ever
			// slipped.
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
			integerEnd = si
			if si < len(s) && s[si] == '.' {
				si++
				for si < len(s) && isDigitOrSep(s[si]) {
					si++
				}
			}
		}

		hasDot := leadingDot || integerEnd >= 0 && integerEnd < si && s[integerEnd] == '.'
		if si < len(s) && (s[si] == 'e' || s[si] == 'E') {
			exponentMark := si
			si++
			if si < len(s) && (s[si] == '+' || s[si] == '-') {
				si++
			}
			exponentStart := si
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
			if si == exponentStart {
				if integerEnd >= 0 && hasDot {
					if dataMode {
						return nil
					}
					return emitDecimalPrefix(lex, s, start, integerEnd)
				}
				if leadingDot {
					si = exponentMark
				} else {
					return nil
				}
			}
		}

		if isFollowingText(s, si) {
			// Data mode never splits a run: declining hands the whole span
			// to the stock lenient text scanner, so digit-led prose stays
			// one string instead of shedding a stray second value.
			if dataMode {
				return nil
			}
			if integerEnd >= 0 && hasDot {
				return emitDecimalPrefix(lex, s, start, integerEnd)
			}
			if !leadingDot {
				return nil
			}
		}

		src := s[start:si]
		val, err := strconv.ParseFloat(stripUnderscores(src), 64)
		if err != nil && !isRangeError(err) {
			// A loose separator run such as `1e_` strips to an invalid
			// float, but the converter will reject `_` before reading Val.
			val = 0
		}
		tkn := lex.Token("#NR", jsonic.TinNR, val, src)
		cursor.SI = si
		cursor.CI += si - start
		return tkn
	})
}

// emitDecimalPrefix emits the integer before a member-access dot. A valid
// prefix is numeric, allowing convertTopLevelItems to diagnose a numeric
// receiver. An already-malformed separator prefix stays text so its numeric
// spelling error is not hidden by the later dot (`1_.x`).
func emitDecimalPrefix(lex *jsonic.Lex, s string, start, end int) *jsonic.Token {
	src := s[start:end]
	cursor := lex.Cursor()
	var tkn *jsonic.Token
	if validUnderscores(src) {
		val, _ := strconv.ParseFloat(stripUnderscores(src), 64)
		tkn = lex.Token("#NR", jsonic.TinNR, val, src)
	} else {
		tkn = lex.Token("#TX", jsonic.TinTX, src, src)
	}
	cursor.SI = end
	cursor.CI += end - start
	return tkn
}

// miniLitVal carries a minilang-literal match (`+name<delim>src<delim>`) from
// the lex matcher through the val rule to the converter, which expands it to
// the splice `mini <name> '<src>'`.
type miniLitVal struct {
	Name string
	Src  string
}

// setupMiniLitMatcher registers a high-priority lex matcher for the minilang
// shortcut `+name<delim>src<delim>` — terse sugar for `mini name 'src'`. The
// delimiter is the first character after the (lowercase) name; the SAME
// character closes. Backslash escapes the delimiter, itself, and whitespace
// (`\<delim>` → the delimiter, `\\` → `\`, `\<space>`/`\<tab>` → the space);
// every OTHER backslash is preserved literally, so regex sources keep their
// escapes raw (`+re/\d+/` → src `\d+`, no doubling).
//
// A literal is a SINGLE whitespace-free span — its source never contains
// unescaped whitespace. Within the span the first unescaped delimiter closes
// it (the closed form, which can abut syntax: `+re/x/).fst`). The closing
// delimiter is OPTIONAL when the source does not contain the delimiter char:
// with no delimiter before the first unescaped whitespace (or end of source),
// the span IS the source — the open form, `+email:alice@example.com` ≡
// `mini email 'alice@example.com'`. There is no scan-ahead past whitespace,
// so a stray delimiter char in a later token (the `:` in `{limit:2}` or
// `{limit: 2}`) can never capture the literal, and a literal never spans
// lines. A source that needs whitespace uses the quoted `mini` form or
// escapes (`+hb/de\ ad/`); hb/bb sources can group with `_` instead
// (`+hb/de_ad_be_ef/`).
//
// The match emits a single #ML token carrying a miniLitVal; the val rule turns
// it into r.Node and convertTopLevelValueInner expands it to the mini splice,
// so all the normal mini machinery (stack subject, a trailing opts Map, the
// expansion-time unknown-kind error, check mode) takes over unchanged. Runs
// only outside template strings; a `+` not followed by a name + non-space
// delimiter + a source char is left to normal lexing.
func setupMiniLitMatcher(j *jsonic.Jsonic, t parserTokens) {
	isNameStart := func(c byte) bool { return c >= 'a' && c <= 'z' }
	isNameChar := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
	}
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
	addMatcher(j, "minilang_literal", 1000002, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule != nil {
			if _, ok := rule.K["boru_tpl"]; ok {
				return nil // inside a template string: not a minilang literal
			}
		}
		cursor := lex.Cursor()
		s := lex.Src
		start := cursor.SI
		si := start
		if si >= len(s) || s[si] != '+' {
			return nil
		}
		si++
		// Name: a lowercase kind atom. (A digit after `+` — e.g. `+0d5` — is
		// not a name start, so signed bignums fall through to big_number.)
		if si >= len(s) || !isNameStart(s[si]) {
			return nil
		}
		nameStart := si
		for si < len(s) && isNameChar(s[si]) {
			si++
		}
		name := s[nameStart:si]
		// Delimiter: the first char after the name. Must exist and not be
		// whitespace (a space means this was not a minilang literal).
		if si >= len(s) || isSpace(s[si]) {
			return nil
		}
		delim := s[si]
		si++
		// Single-span scan: a literal never contains unescaped whitespace.
		// The first unescaped delimiter closes it (so a closed literal can
		// abut syntax: `+re/x/).fst`, `+hb/de_ad/).typeof`); unescaped
		// whitespace before any delimiter ends the literal OPEN — no
		// scan-ahead, so a stray delimiter char in a later token (`{limit:2}`
		// or `{limit: 2}`) can never capture it. Escapes: `\<delim>`,
		// `\\`, `\<space>`, `\<tab>` produce the escaped char; every other
		// backslash is preserved raw.
		var b strings.Builder
		closed := false
		for si < len(s) {
			c := s[si]
			if isSpace(c) {
				break // open form: the literal ends at unescaped whitespace
			}
			if c == '\\' && si+1 < len(s) {
				if nxt := s[si+1]; nxt == delim || nxt == '\\' || nxt == ' ' || nxt == '\t' {
					b.WriteByte(nxt) // \<delim> → delim ; \\ → \ ; \<ws> → ws
					si += 2
					continue
				}
				b.WriteByte('\\') // any other escape is preserved raw (e.g. \d)
				si++
				continue
			}
			if c == delim {
				closed = true
				si++
				break
			}
			b.WriteByte(c)
			si++
		}
		if !closed && b.Len() == 0 {
			return nil // an empty open source (`+name:` before whitespace) is not a literal
		}
		tkn := lex.Token("#ML", t.ML, miniLitVal{Name: name, Src: b.String()}, s[start:si])
		cursor.SI = si
		// The span can never contain a newline (the scan breaks at any
		// unescaped whitespace and only escape-consumes space/tab), so
		// the cursor stays on this row and advances one column per rune.
		cursor.CI += utf8.RuneCountInString(s[start:si])
		return tkn
	})
}

// setupTemplateLiteralMatcher registers a high-priority lex matcher that
// produces #TL tokens for literal text inside template strings. Active only
// when rule.K["boru_tpl"] is set (inside a backtick-opened interp rule).
func setupTemplateLiteralMatcher(j *jsonic.Jsonic, t parserTokens) {
	addMatcher(j, "template_literal", 1000000, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule == nil {
			return nil
		}
		if _, ok := rule.K["boru_tpl"]; !ok {
			return nil
		}
		cursor := lex.Cursor()
		si := cursor.SI
		s := lex.Src
		if si >= len(s) {
			return nil
		}
		// Don't match if at ` or ${ — let fixed token matcher handle those.
		if s[si] == '`' {
			return nil
		}
		if s[si] == '$' && si+1 < len(s) && s[si+1] == '{' {
			return nil
		}
		// Scan forward collecting literal text until ` or ${ or end.
		start := si
		for si < len(s) {
			if s[si] == '`' {
				break
			}
			if s[si] == '$' && si+1 < len(s) && s[si+1] == '{' {
				break
			}
			// Process escape sequences in template literals. A malformed
			// `\x` / `\u` is refused as a quoted string's is (NUR026).
			if s[si] == '\\' && si+1 < len(s) {
				if code, end := escapeFault(s, si+1, '`'); code != "" {
					return badEscapeToken(lex, code, s[si:end])
				}
				si += 2
				continue
			}
			si++
		}
		// si > start always: the entry guards returned on end-of-source,
		// backtick, and `${`, so the first iteration cannot break and
		// every arm advances si.
		lit := s[start:si]
		// Process escape sequences.
		lit = processTemplateEscapes(lit)
		tkn := lex.Token("#TL", t.TL, lit, s[start:si])
		cursor.SI = si
		// Update row/col tracking.
		for _, ch := range s[start:si] {
			if ch == '\n' {
				cursor.RI++
				cursor.CI = 1
			} else {
				cursor.CI++
			}
		}
		return tkn
	})
}

// setupStringEscapeMatcher refuses a malformed `\x` / `\u` escape in a
// quoted string before jsonic's string lexer reads it (NUR026). The two
// tabnas ports report one differently — Go spans from the opening quote,
// TS spans the escape, and `"a\x4"` is an invalid escape in Go but an
// unterminated string in TS — so boru owns the check, with the definition a
// template's text answers to (escapeFault), as the 2026-07-31 verdict put
// string escapes in boru's hands. A well-formed or unterminated string, or a
// raw control character, is left to jsonic.
func setupStringEscapeMatcher(j *jsonic.Jsonic) {
	addMatcher(j, "string_escape", 1000003, func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
		if rule != nil {
			if _, tpl := rule.K["boru_tpl"]; tpl {
				return nil
			}
		}
		s := lex.Src
		si := lex.Cursor().SI
		if si >= len(s) || (s[si] != '"' && s[si] != '\'') {
			return nil
		}
		q := s[si]
		for i := si + 1; i < len(s); i++ {
			c := s[i]
			if c == q || c < 32 {
				return nil
			}
			if c != '\\' || i+1 >= len(s) {
				continue
			}
			if code, end := escapeFault(s, i+1, q); code != "" {
				return badEscapeToken(lex, code, s[i:end])
			}
			i++
		}
		return nil
	})
}

// setupValRule extends the jsonic "val" rule with boru-specific alternates:
// parens, template strings, close-paren markers, semicolons, ?, !, |, and dots.
func setupValRule(j *jsonic.Jsonic, t parserTokens) {
	// The value-open marker batch and the map-pair-value dot-chain
	// closes now live in the declarative grammar artifact
	// (grammar.json, applied by applyDeclEdits before this function
	// runs); their named hooks are valDeclHooks.

	// Dot-chain continuation. After a value closes, a following `.` (or
	// `!.`) means member access — `bf.n`, `a.b.c`, `a!.b`. In WORD
	// context (top level, lists, parens) the dot tokens are already
	// folded into `get`/`getr` calls by convertTopLevelItems, because
	// those contexts collect a flat item sequence. But a MAP VALUE is a
	// single `val`: once `bf` is parsed jsonic has no rule for the
	// trailing `.`, so `{n: bf.n}` errored with `unexpected character`.
	//
	// This Close alternate folds the receiver value plus the trailing
	// dot-chain into a parenGroup — exactly the shape the explicit
	// workaround `{n: (bf.n)}` produces — so the existing converter
	// paths (parenGroup → ParenExpr in data context, ( … ) markers in
	// word context) handle it uniformly with no converter change.
	// Prepended so it is tried before jsonic's default "there's more"
	// backtrack, which is what produced the error.
	// Gate strictly on map-pair-value position: the val whose Close sees
	// the dot is the value side of a `key:` pair exactly when its parent
	// rule is "pair" (in word context — top level, list elements, paren
	// contents — the parent is "elem"/"pelem" and the dot tokens are
	// already collected as a flat item sequence and folded into get/getr
	// by convertTopLevelItems; firing the dotchain there would
	// double-wrap, e.g. `1 foo.bar` → `1 ((foo get bar))`). Only the
	// map-value position lacks that flat-item path and needs this fold.

	// dotchain rule: collects the `.key` / `!.key` segments that follow a
	// value, folding the receiver (the parent val's already-parsed Node)
	// and the segments into a parenGroup. Pushed from the val.Close
	// alternates above with the pointer backtracked onto the first `.` or
	// `!`. The resulting parenGroup is written back to the parent val so
	// downstream conversion treats `bf.n` exactly like the explicit
	// `(bf.n)`.
	//
	// jsonic alternates match at most two tokens, so the chain is
	// consumed one token at a time: a `.` or `!` marker, then the member
	// key, looping via R (replace-sibling) until a non-chain token ends
	// it. Each matched token is appended to the parent val's parenGroup.
	//
	// Each dotchain rule consumes exactly one `. key` (or `! . key`)
	// segment, then loops via R only when ANOTHER `.`/`!` immediately
	// follows. Crucially it must NOT treat a following bare key as a
	// chain segment — in a map, `{a: bf.n b: 9}` has `b` as the next
	// pair's key, not part of `bf.n` — so the loop is gated strictly on
	// a leading `.`/`!`, never on a key alone.
	j.Rule("dotchain", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// On the first dotchain rule for this value, seed the
				// parent val's Node as a parenGroup holding the receiver.
				// Subsequent looped dotchains see a parenGroup already.
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				if _, ok := r.Parent.Node.(parenGroup); ok {
					return
				}
				recv := r.Parent.Node
				if s, ok := recv.(sited); ok {
					recv = s.Node
				}
				r.Parent.Node = parenGroup{recv}
			},
		})
		// Mirror the accumulated chain (built into the PARENT val's Node by the
		// segment actions) into THIS rule's own Node. tabnas's val coalescer
		// (`@val-bc`, re-run after a close-pushed child completes) sets the
		// val's value from r.Child.Node — so the folded parenGroup must live on
		// the dotchain's own node, not only on the parent's, to survive. Each
		// looped dotchain (the val's current child) refreshes its node to the
		// running parenGroup; the final one carries the complete chain.
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				if pg, ok := r.Parent.Node.(parenGroup); ok {
					r.Node = pg
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// `. key` — plain member access: append "." then the key.
			{S: [][]jsonic.Tin{{t.DT}, jsonic.TinSetKEY}, A: dotchainSeg(".")},
			// `!` — strict (getr) marker; the following `. key` is taken
			// by the next loop iteration. `!` only appears mid-chain
			// before a dot, so emitting it alone here is safe.
			{S: [][]jsonic.Tin{{t.BG}}, A: dotchainMarker("!")},
		})
		setClose(rs, []*jsonic.AltSpec{
			// One segment per push: end and backtrack so the enclosing val
			// re-closes. A following `.`/`!` re-fires the val.Close dotchain
			// alt, pushing a FRESH dotchain that accumulates onto the same
			// parent parenGroup. (R-looping within this rule would leave the
			// val's r.Child pointing at the first dotchain, whose node was
			// snapshotted at one segment — tabnas's val coalescer then drops
			// the rest of the chain. Re-pushing per segment keeps r.Child on
			// the latest dotchain, which carries the full chain.)
			{B: 1},
		})
	})

	// Arrow fold. `SIG => BODY` groups as `(SIG afn BODY)` — the arrow
	// binds TIGHTER than any enclosing word's forward collection, so
	// `def double x:Integer => [x mul 2]` is by construction the same
	// program as `def double (x:Integer => [x mul 2])`. Without the
	// fold, the flat token run `SIG afn BODY` races the enclosing word:
	// def's Any slot claims the SIG literal at plan time, binding the
	// name to the sig map/list while the lambda evaporates as an
	// orphaned anonymous value (the parenless forms previously "worked"
	// exactly once by feeding that orphan).
	//
	// Mirrors the dotchain fold above: the val whose Close sees the
	// arrow pushes "arrowfold", which seeds the val's Node as
	// parenGroup{SIG, afn}, consumes the arrow, parses ONE following
	// val as the body, and appends it. Gates:
	//
	//   - parent elem / pelem — word contexts (top-level items, list
	//     elements, paren contents), where the flat run is what races;
	//   - parent arrowfold — the BODY of an enclosing fold, so
	//     `x:Integer => y:Integer => [x add y]` curries
	//     right-associatively: (x ⇒ (y ⇒ body));
	//   - NOT a list-pair value (parent elem with U["pair"]) — `[k:v]`
	//     forms keep their flat shape;
	//   - NOT when the previous sibling is a `.`/`!` marker — a flat
	//     dot-chain (`m.k => body`) is folded into `(m get k)` by
	//     convertTopLevelItems, and folding here would capture only the
	//     final key as the sig.
	//
	// Everywhere else (map-pair values, template exprs, degenerate
	// leading arrows) the val.Open #AR alternate still produces the
	// bare `afn` word, exactly as before.
	arrowFoldable := func(r *jsonic.Rule, ctx *jsonic.Context) bool {
		p := r.Parent
		if p == nil || p == jsonic.NoRule {
			return false
		}
		switch p.Name {
		case "pelem", "arrowfold", "arrowfoldelem":
			// ok — paren contents and fold bodies (the latter make
			// `x => y => body` curry right-associatively).
		case "elem":
			// A list-PAIR's value val must not fold (the sig is the
			// whole pair, which only exists on the elem — see the
			// elem-level fold below). Plain elems fold here.
			if pair, ok := p.U["pair"]; ok && pair == true {
				return false
			}
		default:
			return false
		}
		if r.Prev != nil && r.Prev != jsonic.NoRule {
			prev := r.Prev.Node
			if s, ok := prev.(sited); ok {
				prev = s.Node
			}
			if txt, ok := prev.(jsonic.Text); ok && (txt.Str == "." || txt.Str == "!") {
				return false
			}
		}
		return true
	}
	j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.AR}}, C: arrowFoldable, P: "arrowfold", B: 1},
		})
	})

	j.Rule("arrowfold", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Seed the parent val's Node: the already-parsed value
				// becomes the sig half of (SIG afn BODY). deSite the
				// receiver — the parent val's BC re-sites the whole group.
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				recv := r.Parent.Node
				if s, ok := recv.(sited); ok {
					recv = s.Node
				}
				r.Parent.Node = parenGroup{recv, arrowTag{}}
			},
		})
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Append the body val to the parent's parenGroup.
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				if jsonic.IsUndefined(r.Child.Node) {
					return
				}
				if pg, ok := r.Parent.Node.(parenGroup); ok {
					grp := parenGroup(append([]any(pg), r.Child.Node))
					r.Parent.Node = grp
					// Mirror the assembled group onto this rule's own Node:
					// tabnas's val coalescer (`@val-bc`, re-run after this
					// close-pushed rule completes) sets the parent val's value
					// from r.Child.Node, so the (SIG afn BODY) group must live
					// here, not only on the parent. (Same reason as dotchain.)
					r.Node = grp
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// An arrow with nothing to fold — the source ends, or a closer
			// or separator follows — has no body: refuse it (NUR060).
			arrowNoBody(t),
			// Consume the arrow, parse exactly one following val as the body.
			{S: [][]jsonic.Tin{{t.AR}}, P: "val"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// Single shot — chaining is the BODY val's own fold (see the
			// arrowfold parent gate above). Backtrack so the enclosing
			// context consumes the next token.
			{B: 1},
		})
	})

	// Elem-level arrow fold. A bare pair OUTSIDE parens — `def double
	// x:Integer => [x mul 2]` at top level, or inside an explicit list —
	// parses via the elem rule's list-pair alternate, NOT via the val
	// dive: the value val sees the arrow first (its fold is gated off
	// above — the sig is the whole pair, not the value), and by the time
	// elem.Close examines the arrow, the elem's BC has already appended
	// the assembled one-entry pair map as the list node's LAST element.
	// So the elem-level fold pops that element, seeds (PAIR afn …), and
	// appends the folded group back. Gated on U["pair"] — a plain elem's
	// val folds at the val level, and a flat dot-chain that declined the
	// val fold must not be re-folded here.
	elemPairArrow := func(r *jsonic.Rule, ctx *jsonic.Context) bool {
		pair, ok := r.U["pair"]
		return ok && pair == true
	}
	j.Rule("elem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.AR}}, C: elemPairArrow, P: "arrowfoldelem", B: 1,
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					// The elem's BC runs AGAIN when the elem finally
					// closes after the pushed fold returns; clear the
					// pair flag so that second pass doesn't rebuild a
					// pair from the fold rule's node (the first pass
					// already appended the real pair, which the fold's
					// BO pops as the sig).
					r.EnsureU()["pair"] = false
				}},
		})
	})

	j.Rule("arrowfoldelem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Pop the just-appended pair map off the parent elem's
				// list node and hold it as the sig half. The elem and its
				// list share the slice, so propagate the shortened node
				// to both.
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				switch n := r.Parent.Node.(type) {
				case jsonic.ListRef:
					if len(n.Val) == 0 {
						return
					}
					r.EnsureU()["af_sig"] = n.Val[len(n.Val)-1]
					n.Val = n.Val[:len(n.Val)-1]
					r.Parent.Node = n
					if r.Parent.Parent != nil && r.Parent.Parent != jsonic.NoRule {
						r.Parent.Parent.Node = n
					}
				case []any:
					if len(n) == 0 {
						return
					}
					r.EnsureU()["af_sig"] = n[len(n)-1]
					rest := n[:len(n)-1]
					r.Parent.Node = rest
					if r.Parent.Parent != nil && r.Parent.Parent != jsonic.NoRule {
						r.Parent.Parent.Node = rest
					}
				}
			},
		})
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Append (SIG afn BODY) where the bare pair sat.
				sig, ok := r.U["af_sig"]
				if !ok || r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				body := r.Child.Node
				if jsonic.IsUndefined(body) {
					return
				}
				group := parenGroup{sig, arrowTag{}, body}
				switch n := r.Parent.Node.(type) {
				case jsonic.ListRef:
					n.Val = append(n.Val, group)
					r.Parent.Node = n
					if r.Parent.Parent != nil && r.Parent.Parent != jsonic.NoRule {
						r.Parent.Parent.Node = n
					}
				case []any:
					grown := append(n, group)
					r.Parent.Node = grown
					if r.Parent.Parent != nil && r.Parent.Parent != jsonic.NoRule {
						r.Parent.Parent.Node = grown
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			arrowNoBody(t),
			{S: [][]jsonic.Tin{{t.AR}}, P: "val"},
		})
		setClose(rs, []*jsonic.AltSpec{
			{B: 1},
		})
	})

	// Capture source position: wrap every value node with the row/col of its
	// opening token, so the converter can stamp core.Value.Pos for precise
	// error reporting (mirrors aontu's addsite). This runs in the AFTER-CLOSE
	// (AC) phase, NOT before-close: the tabnas jsonic grammar installs a
	// `@val-ac` hook that re-resolves a primitive value from its token after
	// BC, which would otherwise overwrite the sited wrapper for every value
	// except the first. Running in AC — after `@val-ac` — makes the wrapper
	// the final node for every value. The Open alternates above have already
	// set r.Node (including the marker Texts), so markers carry positions too.
	// The wrapper is unwrapped (deSite) at every node-type switch in parse.go
	// and never leaks.
	j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddAC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if r.Node == nil || jsonic.IsUndefined(r.Node) {
				return
			}
			if _, already := r.Node.(sited); already {
				return
			}
			var pos core.SrcPos
			if r.O0 != nil && !r.O0.IsNoToken() {
				pos = core.SrcPos{Row: r.O0.RI, Col: r.O0.CI, Src: r.O0.Src}
			}
			r.Node = sited{Node: r.Node, Pos: pos}
		})
	})
}

// dotchainAppendTo appends node(s) to the parent val's parenGroup (the
// chain accumulator seeded in the dotchain BO). Shared tail of the
// dotchain Open actions.
func dotchainAppendTo(r *jsonic.Rule, nodes ...any) {
	if r.Parent == nil || r.Parent == jsonic.NoRule {
		return
	}
	if grp, ok := r.Parent.Node.(parenGroup); ok {
		r.Parent.Node = append(grp, nodes...)
	}
}

// dotchainMarker returns an AltAction that appends a single marker Text
// (e.g. the "!" of a strict member access) to the chain.
func dotchainMarker(marker string) jsonic.AltAction {
	return func(r *jsonic.Rule, ctx *jsonic.Context) {
		dotchainAppendTo(r, jsonic.Text{Str: marker, Quote: ""})
	}
}

// dotchainSeg returns an AltAction for a `<marker> key` segment: it
// appends the marker Text (the "." of member access, O0) followed by the
// member key token (O1), preserving the key's quoted/unquoted
// distinction so an unquoted key stays a word and a quoted key a string.
func dotchainSeg(marker string) jsonic.AltAction {
	return func(r *jsonic.Rule, ctx *jsonic.Context) {
		tok := r.O1
		if tok == nil {
			dotchainAppendTo(r, jsonic.Text{Str: marker, Quote: ""})
			return
		}
		quote := ""
		if tok.Tin == jsonic.TinST {
			quote = "'"
		}
		str := tok.Src
		if s, ok := tok.Val.(string); ok {
			str = s
		}
		dotchainAppendTo(r, jsonic.Text{Str: marker, Quote: ""}, jsonic.Text{Str: str, Quote: quote})
	}
}

// setupPairGrammar extends "pair" and "elem" rules for optional-field (?:)
// and computed-key ([key]:) syntax in both map and list contexts.
func setupPairGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Shared helper: extract key from pair-open token.
	pairkey := func(r *jsonic.Rule, ctx *jsonic.Context) {
		keyToken := r.O0
		var key string
		if keyToken.Tin == jsonic.TinST || keyToken.Tin == jsonic.TinTX {
			key, _ = keyToken.Val.(string)
		} else {
			key = keyToken.Src
		}
		r.EnsureU()["key"] = key
	}

	// --- Optional field syntax: key?:value in pair context ---

	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependOpen(rs, []*jsonic.AltSpec{
			// Match KEY ? — save key via K (propagated), push to pair.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {t.QM}},
				P: "pair", K: map[string]any{"boru_qm": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					// Propagate key to inner pair via K.
					r.EnsureK()["key"] = r.U["key"]
				}},
			// Match : when boru_qm flag is set — copy key from K, proceed as pair value.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["boru_qm"]
					return ok
				},
				P: "val", U: map[string]any{"pair": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.EnsureU()["key"] = r.K["key"]
				}},
			// Optional shorthand with NO value — `{foo?}`. The inner pair
			// pushed by the `KEY ?` alt sees the closing brace; backtrack so
			// the map closes, and record the key both as a shorthand (so
			// convertMapData synthesises `foo: foo`) and as optional (Meta
			// "qm"). Without this the empty pushed pair has no alt for the
			// closing brace and errors. (The legacy jsonicjs pair rule closed
			// an empty pushed pair on the brace implicitly; tabnas does not.)
			{S: [][]jsonic.Tin{{jsonic.TinCB}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["boru_qm"]
					return ok
				},
				B: 1,
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					if k, ok := r.K["key"]; ok {
						r.EnsureU()["key"] = k
						if ks, ok := k.(string); ok {
							r.EnsureU()["boru_sh"] = ks
						}
					}
				}},
		})
	})

	// Record optional keys in MapRef.Meta via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["boru_qm"]; !ok {
				return
			}
			key, _ := r.U["key"].(string)
			if key == "" {
				return
			}
			// Store optional key in MapRef.Meta["qm"].
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				qmSet, _ := mr.Meta["qm"].(map[string]bool)
				if qmSet == nil {
					qmSet = make(map[string]bool)
				}
				qmSet[key] = true
				mr.Meta["qm"] = qmSet
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// --- Computed key syntax: {[key]:value} in pair context ---

	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependOpen(rs, []*jsonic.AltSpec{
			// Step 1: match [ — set computed key flag, push to pair.
			{S: [][]jsonic.Tin{{jsonic.TinOS}},
				P: "pair", K: map[string]any{"boru_ck": true}},
			// Step 2: match KEY ] when boru_ck — save key, push to pair.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {jsonic.TinCS}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["boru_ck"]
					return ok
				},
				P: "pair",
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					r.EnsureK()["key"] = r.U["key"]
				}},
			// Step 3: match : when boru_ck and key is set — proceed as pair value.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, hasCK := r.K["boru_ck"]
					_, hasKey := r.K["key"]
					return hasCK && hasKey
				},
				P: "val", U: map[string]any{"pair": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.EnsureU()["key"] = r.K["key"]
				}},
		})
	})

	// Record computed keys in MapRef.Meta via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["boru_ck"]; !ok {
				return
			}
			// Clear the computed-key flag as this pair closes so it cannot
			// LEAK to the sibling pair. tabnas's K map is shared across the
			// sibling pairs of a map, so a lingering boru_ck made a later bare
			// key (`{[k]:1 aa:2}`) wrongly parse as computed — the pre-existing
			// leak the D1 order channel sits on (design/FLEX-ATTRS.1.md §2.6).
			defer delete(r.K, "boru_ck")
			key, _ := r.U["key"].(string)
			if key == "" {
				return
			}
			// Store computed key in MapRef.Meta["ck"].
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				ckSet, _ := mr.Meta["ck"].(map[string]bool)
				if ckSet == nil {
					ckSet = make(map[string]bool)
				}
				ckSet[key] = true
				mr.Meta["ck"] = ckSet
				r.Node = mr
			}
		})
	})

	// --- Shorthand field syntax: {foo} ≡ {foo: foo}, {foo/v} ≡ {foo: foo/v} ---

	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddOpen(&jsonic.AltSpec{
			// A lone unquoted-text key with no following colon. Appended
			// LAST so it never shadows a real `key:value` pair: the base
			// KEY CL alt (and the prepended qm/ck alts) are tried first,
			// and only `{foo}` / `{foo/v}` — where no colon, `?`, or `[`
			// follows — fall through to here. The optional shorthand
			// `{foo?}` is handled by the qm path above (it consumes
			// `foo ?` and records Meta["qm"]); the value is synthesized in
			// convertMapData. No P/U["pair"]: nothing is pushed, so the
			// built-in pairval BC stays inert and the pair closes cleanly.
			S: [][]jsonic.Tin{{jsonic.TinTX}},
			A: func(r *jsonic.Rule, ctx *jsonic.Context) {
				if raw, ok := r.O0.Val.(string); ok {
					r.EnsureU()["boru_sh"] = raw
				}
			},
		})
	})

	// Record shorthand keys in MapRef.Meta["sh"] via pair.BC callback.
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			raw, ok := r.U["boru_sh"].(string)
			if !ok || raw == "" {
				return
			}
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				shList, _ := mr.Meta["sh"].([]string)
				shList = append(shList, raw)
				mr.Meta["sh"] = shList
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// Record quoted keys in MapRef.Meta["qk"] via pair.BC callback. A
	// quoted key (`{'a/b': 1}`) arrives as a string token (TinST) rather
	// than bare text (TinTX); convertMapData can't tell them apart from the
	// plain string map afterwards, so it needs this flag to know the key
	// was an explicit literal — e.g. to allow a `/` in it that would be an
	// illegal word modifier on a bare key.
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if r.O0.Tin != jsonic.TinST {
				return
			}
			key, ok := r.O0.Val.(string)
			if !ok || key == "" {
				return
			}
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				qkSet, _ := mr.Meta["qk"].(map[string]bool)
				if qkSet == nil {
					qkSet = make(map[string]bool)
				}
				qkSet[key] = true
				mr.Meta["qk"] = qkSet
				r.Node = mr // MapRef is a value type, reassign
			}
		})
	})

	// Record source KEY ORDER in MapRef.Meta["ko"] via pair.BC callback —
	// the D1 insertion-order channel (design/FLEX-ATTRS.1.md §3). jsonic
	// hands convertMapData an UNORDERED Go map (MapRef.Val), so without
	// this the parser sorts keys for determinism and a `{b:2 a:1}` literal
	// loses its source order. BC fires once per pair in source order; each
	// records its FINAL union key (the same string convertMapData keys on):
	// a shorthand base name, an explicit/computed/optional key, else the
	// bare pair key. First-position/last-value dedup — a repeated key keeps
	// its first slot (matching OrderedMap.Set), and inner pushed pairs
	// (the qm/ck double-fire) collapse onto the same slot.
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			var key string
			switch {
			case r.U["boru_sh"] != nil:
				if raw, ok := r.U["boru_sh"].(string); ok {
					key = wordBaseName(raw)
				}
			case r.U["key"] != nil:
				key, _ = r.U["key"].(string)
			case r.O0 != nil && !r.O0.IsNoToken():
				// The explicit/computed/optional/quoted pairs all set
				// r.U["key"] above; a pair reaching here is a bare `k:v`,
				// whose key token renders verbatim via Src (its Val is a
				// non-string primitive — see the D1 coverage note).
				key = r.O0.Src
			}
			if key == "" {
				return
			}
			// Sibling recording BCs (qk/sh/ck) gate on the MapRef the same
			// way; the pair always closes into a MapRef, so the miss arm is
			// unreachable and stays out of the guard.
			if mr, ok := r.Node.(jsonic.MapRef); ok {
				ko, _ := mr.Meta["ko"].([]string)
				for _, e := range ko {
					if e == key {
						return // already recorded — first-position dedup
					}
				}
				mr.Meta["ko"] = append(ko, key)
				r.Node = mr
			}
		})
	})

	// --- A `]` never closes a list no `[` opened ---
	//
	// jsonic's list Close takes `]` for every list, so a bracket-less
	// (implicit) top-level list swallowed a stray one: `1 2 ]` parsed as
	// `1 2`, while `0 ]` (no list yet) was refused, and `1 2 ] 3` failed
	// only at the end-of-parse check — which the two tabnas ports report
	// on different tokens (NUR060). A list whose open token is not `[` is
	// implicit; its `]` is an unexpected token, refused where it stands.
	j.Rule("list", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{jsonic.TinCS}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					return r.O0 == nil || r.O0.Tin != jsonic.TinOS
				},
				E: func(_ *jsonic.Rule, ctx *jsonic.Context) *jsonic.Token {
					return ctx.T0.Bad("unexpected", nil)
				}},
		})
	})

	// --- Optional field syntax in list context: [x?:Integer] ---

	j.Rule("elem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependOpen(rs, []*jsonic.AltSpec{
			// An element that OPENS with `:` is a typed list's child; one
			// with no value after the colon (`[:]`, `[1, :]`, `? :`) is an
			// empty element, refused like `{:}` and `[1 :]`. The Go tabnas
			// port records an empty list child as a nil Child — the same as
			// no child — so the converter cannot see it; the TS port keeps
			// the two apart (NUR060).
			{S: [][]jsonic.Tin{{jsonic.TinCL}, {jsonic.TinZZ, jsonic.TinCS, jsonic.TinCA}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, qm := r.K["boru_qm"]
					return !qm
				},
				E: func(_ *jsonic.Rule, ctx *jsonic.Context) *jsonic.Token {
					return ctx.T0.Bad("empty_child", nil)
				}},
			// Step 1: match KEY ? — save key, push to elem.
			{S: [][]jsonic.Tin{jsonic.TinSetKEY, {t.QM}},
				P: "elem", K: map[string]any{"boru_qm": true},
				U: map[string]any{"done": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					pairkey(r, ctx)
					r.EnsureK()["key"] = r.U["key"]
				}},
			// Step 2: match : when boru_qm is set — proceed as list pair.
			{S: [][]jsonic.Tin{{jsonic.TinCL}},
				C: func(r *jsonic.Rule, ctx *jsonic.Context) bool {
					_, ok := r.K["boru_qm"]
					return ok
				},
				P: "val",
				N: map[string]int{"pk": 1, "dmap": 1},
				U: map[string]any{"done": true, "pair": true, "list": true},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					r.EnsureU()["key"] = r.K["key"].(string) + "?"
				}},
		})
	})

	// Propagate the outer elem's updated Node to grandparent (list rule).
	j.Rule("elem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if _, ok := r.K["boru_qm"]; !ok {
				return
			}
			// Only propagate from the OUTER elem (which has done=true).
			done, _ := r.U["done"].(bool)
			if !done {
				return
			}
			if r.Parent != nil && r.Parent != jsonic.NoRule {
				r.Parent.Node = r.Node
			}
		})
	})

	// --- Lambda arrow closes an implicit pair-dive: (x:Integer => body) ---
	//
	// Inside a paren, `x:Integer` parses via val's pair-dive alternate
	// (pk > 0), and the dive's pair/map rules have no alternative for the
	// arrow token — pair.Close falls to the implicit-continuation
	// fallback, re-opens `pair` on `=>`, and errors with "unexpected
	// character". These two Close alternates end the dive instead
	// (backtracking the arrow), so the implicit one-entry map
	// {x:Integer} completes as the preceding value and the arrow
	// re-reads in the enclosing context, where val's #AR alternate emits
	// `afn`. ParseFnParams reads the Implicit map as a named param, so
	// `(x:Integer => [x mul 2])` ≡ `([x:Integer] => [x mul 2])`.
	//
	// The pk condition scopes both alternates to IMPLICIT pairs: a
	// pair-dive runs with pk > 0, the top-level implicit map runs with
	// pk ABSENT, and pairs of an explicit `{…}` map run with pk = 0
	// (set at the OB open) — so `{a: 1 => 2}` keeps erroring exactly as
	// before. The matched parse state was unconditionally fatal before
	// these alternates, so they are additive — nothing that parsed
	// previously changes shape.
	arrowEndsDive := func(r *jsonic.Rule, ctx *jsonic.Context) bool {
		pk, ok := r.N["pk"]
		return !ok || pk > 0
	}
	j.Rule("pair", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.AR}}, C: arrowEndsDive, B: 1},
		})
	})
	j.Rule("map", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.AR}}, C: arrowEndsDive, B: 1},
		})
	})
}

// setupParenGrammar defines the "paren" and "pelem" rules that collect
// values between ( and ) into a parenGroup.
func setupParenGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Paren rule: collects values between ( and ) into a parenGroup.
	// Works like a simplified list rule but closes on ) instead of ].
	j.Rule("paren", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
			},
			// Increment dlist and dmap so val.Close won't create
			// implicit lists or maps inside paren groups.
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if v, ok := r.N["dlist"]; ok {
					r.EnsureN()["dlist"] = v + 1
				} else {
					r.EnsureN()["dlist"] = 1
				}
				if v, ok := r.N["dmap"]; ok {
					r.EnsureN()["dmap"] = v + 1
				} else {
					r.EnsureN()["dmap"] = 1
				}
			},
		})
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if arr, ok := r.Node.([]any); ok {
					r.Node = parenGroup(arr)
				}
			},
		})
		setAC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if r.U["closed"] != true {
					if pg, ok := r.Node.(parenGroup); ok {
						r.Node = unclosedParen{items: []any(pg)}
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// Empty parens: (). Match the ")" but BACKTRACK (B:1) so the
			// Close phase consumes it exactly once — like a non-empty paren,
			// whose pelem also backtracks the ")" for paren.Close. Consuming
			// it here instead would let the Close phase eat a SECOND ")",
			// stealing the closer of an enclosing paren when an empty paren
			// is the last element (e.g. `(())`, `(1 ())`).
			{S: [][]jsonic.Tin{{t.CP}}, B: 1},
			// First element
			{P: "pelem"},
		})
		setClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.CP}}, U: map[string]any{"closed": true}},
			// End of source: auto-close (unclosed paren)
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		})
	})

	// Pelem (paren element): each item inside a paren group.
	// Pushes to val for each value, appends to parent paren's list.
	j.Rule("pelem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if !jsonic.IsUndefined(r.Child.Node) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, r.Child.Node)
						if r.Parent != nil && r.Parent != jsonic.NoRule {
							r.Parent.Node = r.Node
						}
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			{P: "val"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// ) ends the paren group (backtrack so paren.Close consumes it)
			{S: [][]jsonic.Tin{{t.CP}}, B: 1},
			// End of source inside paren: auto-close (unclosed paren)
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next element
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "pelem"},
			// Space-separated: next element (backtrack to re-read token)
			{R: "pelem", B: 1},
		})
	})
}

// setupInterpGrammar defines the "interp", "ielem", "iexpr", and "ieval"
// rules for template string interpolation (`hello ${name}`).
func setupInterpGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Interp rule: collects template string parts between backticks.
	// K["boru_tpl"] is set in BO so the custom matcher produces #TL tokens
	// for literal text segments. Parts are accumulated in Node as an interpGroup.
	j.Rule("interp", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = interpGroup{}
				// Set K so the custom matcher knows we're inside a template.
				// K propagates to child rules.
				r.EnsureK()["boru_tpl"] = true
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// Empty template: `` (immediate closing backtick)
			{S: [][]jsonic.Tin{{t.BT}}},
			// First element: push to ielem.
			{P: "ielem"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// Closing backtick ends the template.
			{S: [][]jsonic.Tin{{t.BT}}},
			// End of source: unterminated template string (auto-close).
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		})
	})

	// Ielem rule: each element inside a template string.
	// Handles #TL (literal text) and #IS (interpolation start).
	// On close, appends its own Node to the parent interp's interpGroup.
	j.Rule("ielem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Collect the node value. For #TL, the action sets r.Node directly.
				// For #IS→iexpr, the child rule sets r.Node (iexprGroup) via push.
				node := r.Node
				if !jsonic.IsUndefined(r.Child.Node) {
					node = r.Child.Node
				}
				if jsonic.IsUndefined(node) {
					return
				}
				if r.Parent != nil && r.Parent != jsonic.NoRule {
					if grp, ok := r.Parent.Node.(interpGroup); ok {
						r.Parent.Node = append(grp, node)
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// Literal text segment.
			{S: [][]jsonic.Tin{{t.TL}},
				A: func(r *jsonic.Rule, ctx *jsonic.Context) {
					// Store literal text as a Text with Quote="tl".
					if str, ok := r.O0.Val.(string); ok {
						r.Node = jsonic.Text{Str: str, Quote: "tl"}
					}
				}},
			// Interpolation start ${ — push to iexpr to collect the expression.
			{S: [][]jsonic.Tin{{t.IS}}, P: "iexpr"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// Closing backtick: backtrack so interp.Close can consume it.
			{S: [][]jsonic.Tin{{t.BT}}, B: 1},
			// End of source.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Next element (another literal or interpolation).
			{R: "ielem", B: 1},
		})
	})

	// Iexpr rule: collects expression values between ${ and }.
	// Pushes to val for each value, collects into a list.
	// Clears boru_tpl in BO so that expression content is parsed normally
	// (the custom matcher won't fire inside expressions).
	j.Rule("iexpr", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
				// Clear template mode so expressions parse normally.
				delete(r.K, "boru_tpl")
				// Increment dlist and dmap so val.Close won't create
				// implicit lists or maps inside interpolation expressions.
				if v, ok := r.N["dlist"]; ok {
					r.EnsureN()["dlist"] = v + 1
				} else {
					r.EnsureN()["dlist"] = 1
				}
				if v, ok := r.N["dmap"]; ok {
					r.EnsureN()["dmap"] = v + 1
				} else {
					r.EnsureN()["dmap"] = 1
				}
			},
		})
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				// Wrap the collected values as an iexprGroup.
				if arr, ok := r.Node.([]any); ok {
					r.Node = iexprGroup(arr)
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// Empty expression: ${}. Backtrack so the Close alternate takes
			// the `}` — consuming it here left the Close wanting a second
			// one, so the template broke after an empty hole (`x${}y` read
			// `y` as unexpected) and only an unterminated one survived, in
			// two different shapes (NUR060).
			{S: [][]jsonic.Tin{{jsonic.TinCB}}, B: 1},
			// First expression value.
			{P: "ieval"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// Closing brace } ends the expression.
			{S: [][]jsonic.Tin{{jsonic.TinCB}}},
			// End of source inside expression.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		})
	})

	// Ieval rule: each value inside an interpolation expression.
	// Similar to pelem but closes on } instead of ).
	j.Rule("ieval", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if !jsonic.IsUndefined(r.Child.Node) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, r.Child.Node)
						if r.Parent != nil && r.Parent != jsonic.NoRule {
							r.Parent.Node = r.Node
						}
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			{P: "val"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// } ends the expression (backtrack so iexpr.Close consumes it).
			{S: [][]jsonic.Tin{{jsonic.TinCB}}, B: 1},
			// End of source.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next expression value.
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "ieval"},
			// Space-separated: next expression value.
			{R: "ieval", B: 1},
		})
	})
}

// setupAngleGrammar defines the generics angle-bracket sugar rules
// (design/legacy/GENERICS.10.ignore Phase 6, decisions D14/D15): `Box<Integer>`.
//
// The consumer is CONTEXTUALLY GATED (D14): an angle group opens only
// when the value that just closed is a capitalised bare name — the
// type-name convention — via a val.Close alternate (the dotchain
// technique). A `<` in any other position matches no rule and is a
// syntax error in v1, only because no other consumer exists yet.
//
// The "angle"/"aelem" rule pair is modeled on "paren"/"pelem": items
// collect into a flat list (commas are optional separators, exactly
// like list/paren elements), and the BC fold replaces the receiver
// val's node with an angleGroup{Name, Items}. Desugaring to the
// canonical stream happens in the conversion walks (parse.go), so no
// angle marker ever reaches the engine value stream — macros, quote,
// and Vm.parse see the canonical form (D15).
func setupAngleGrammar(j *jsonic.Jsonic, t parserTokens) {
	// Gate: the just-closed val is a capitalised bare name.
	angleGate := func(r *jsonic.Rule, ctx *jsonic.Context) bool {
		n := r.Node
		if s, ok := n.(sited); ok {
			n = s.Node
		}
		txt, ok := n.(jsonic.Text)
		if !ok || txt.Quote != "" || txt.Str == "" {
			return false
		}
		c := txt.Str[0]
		return c >= 'A' && c <= 'Z'
	}
	j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		prependClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.LA}}, C: angleGate, P: "angle"},
		})
	})

	j.Rule("angle", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBO(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
			},
			// Suppress implicit list/map formation inside the group
			// (same bump the paren rule applies).
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if v, ok := r.N["dlist"]; ok {
					r.EnsureN()["dlist"] = v + 1
				} else {
					r.EnsureN()["dlist"] = 1
				}
				if v, ok := r.N["dmap"]; ok {
					r.EnsureN()["dmap"] = v + 1
				} else {
					r.EnsureN()["dmap"] = 1
				}
			},
		})
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				recv := r.Parent.Node
				var pos core.SrcPos
				if s, ok := recv.(sited); ok {
					pos, recv = s.Pos, s.Node
				}
				name := ""
				if txt, ok := recv.(jsonic.Text); ok {
					name = txt.Str
				}
				arr, _ := r.Node.([]any)
				grp := sited{Node: angleGroup{Name: name, Items: arr}, Pos: pos}
				r.Parent.Node = grp
				// Mirror onto this rule's own Node: tabnas's val coalescer
				// (`@val-bc`, re-run after this close-pushed rule completes)
				// sets the parent val's value from r.Child.Node, so the
				// angleGroup must live here too (same reason as dotchain).
				r.Node = grp
			},
		})
		// The "closed" flag is set by the Close alternate, which runs
		// AFTER BC — so the unclosed-at-EOF rewrite happens in AC (the
		// unclosed-paren technique).
		setAC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if r.U["closed"] == true {
					return
				}
				if r.Parent == nil || r.Parent == jsonic.NoRule {
					return
				}
				name := ""
				if s, ok := r.Node.(sited); ok {
					if ag, isAg := s.Node.(angleGroup); isAg {
						name = ag.Name
					}
				}
				// Set our own Node (the val coalesces from r.Child.Node); also
				// keep the parent in sync for any direct readers.
				r.Node = unclosedAngle{Name: name}
				r.Parent.Node = unclosedAngle{Name: name}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			// Empty group `Name<>`: backtrack so Close consumes the `>`
			// exactly once (the empty-paren technique).
			{S: [][]jsonic.Tin{{t.RA}}, B: 1},
			{P: "aelem"},
		})
		setClose(rs, []*jsonic.AltSpec{
			{S: [][]jsonic.Tin{{t.RA}}, U: map[string]any{"closed": true}},
			// End of source: unclosed — flagged by the BC fold above.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
		})
	})

	// Aelem: each item inside an angle group (the pelem shape with `>`
	// as the boundary). Nested sugar works naturally: an inner val that
	// closes on another `<` re-enters the angle rule, and the two `>`s
	// close inner then outer (no C++-style `>>` shift problem — `>` is
	// a standalone token).
	j.Rule("aelem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
		setBC(rs, []jsonic.StateAction{
			func(r *jsonic.Rule, ctx *jsonic.Context) {
				if !jsonic.IsUndefined(r.Child.Node) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, r.Child.Node)
						if r.Parent != nil && r.Parent != jsonic.NoRule {
							r.Parent.Node = r.Node
						}
					}
				}
			},
		})
		setOpen(rs, []*jsonic.AltSpec{
			{P: "val"},
		})
		setClose(rs, []*jsonic.AltSpec{
			// > ends the group (backtrack so angle.Close consumes it).
			{S: [][]jsonic.Tin{{t.RA}}, B: 1},
			// End of source inside the group: unclosed.
			{S: [][]jsonic.Tin{{jsonic.TinZZ}}},
			// Comma: next item.
			{S: [][]jsonic.Tin{{jsonic.TinCA}}, R: "aelem"},
			// Space-separated: next item.
			{R: "aelem", B: 1},
		})
	})
}

// setupNumberSub registers a token subscriber that wraps every number
// token in a numberVal struct carrying the original source text. The
// source text lets the converter (1) distinguish "5" (integer) from
// "5.0" (decimal), and (2) parse plain decimal integers from their exact
// digits via strconv.ParseInt rather than round-tripping through the
// float64 jsonic already produced — float64 silently corrupts any
// integer above 2^53, so the source string is the only exact record.
// See numberValToValue and design/INTEGER-OVERFLOW-STRATEGY.5.md.
func setupNumberSub(j *jsonic.Jsonic) {
	j.Sub(func(tkn *jsonic.Token, rule *jsonic.Rule, ctx *jsonic.Context) {
		if tkn.Tin == jsonic.TinNR {
			if f, ok := tkn.Val.(float64); ok {
				tkn.Val = numberVal{Val: f, Src: tkn.Src, Row: tkn.RI, Col: tkn.CI}
			}
		}
	}, nil)
}

// arrowTag is the structural node the `=>` grammar rules inject in
// place of a lambda-constructor word name; the converter maps it to
// the lambda sugar marker (ADR-012 rule 3, 2026-08-04 amendment).
type arrowTag struct{}

// arrowNoBody is the fold's refusal of an arrow with no body: `=>` followed
// by the end of the source, a closer (`]` `}` `)`) or a separator (`,` `;`).
// Before it, the half-built (SIG afn) group fell to the tabnas val coalescer,
// which restored the SIG alone — the arrow vanished (`[1 =>]` read `[1]`) —
// or, where the body val re-read the SIG token, doubled it as the body
// (`def x 2 =>` read `def x (2 => 2)`), differently in the two ports
// (NUR060). The error sits on the arrow.
func arrowNoBody(t parserTokens) *jsonic.AltSpec {
	return &jsonic.AltSpec{
		S: [][]jsonic.Tin{{t.AR}, {jsonic.TinZZ, jsonic.TinCS, jsonic.TinCB, t.CP, jsonic.TinCA, t.SC}},
		E: func(_ *jsonic.Rule, ctx *jsonic.Context) *jsonic.Token {
			return ctx.T0.Bad("arrow_no_body", nil)
		},
	}
}
