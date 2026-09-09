package native

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// strOpts holds common option fields parsed from a boru options map.
type strOpts struct {
	normForm string // "", "NFC", "NFD", "NFKC", "NFKD"
	cs       string // "sensitive" or "insensitive"
	mode     string // "literal" or "shell"
	side     string // "left", "right", "both"
	sep      string // separator
	hasSep   bool   // whether sep was explicitly set
	fill     string // fill character for pad
	style    string // case style
	tgt      string // escape target
	quote    string // escape quote style
	scope    string // "first" or "all"
	from     int64  // start index
	hasFrom  bool
	count    int64 // max replacements
	hasCount bool
	lim      int64 // max parts for split
	hasLim   bool
	occ      string // "first" or "last"

	// boolean flags
	skipEmpty   bool
	skipNullish bool
	keepEmpty   bool
	trimParts   bool
	wholeWord   bool
	anchored    string // "", "start", "end", "both" or "true"
	trunc       bool

	// normalize-specific
	trim       bool
	collapseWs bool
	eol        string // "preserve", "lf", "crlf"

	// form for normalize function
	form string
}

// strOptKeys lists the option keys each string word honours.
//
// One parser serves twelve words, each reading only its own subset, and it
// was written as a flat sequence of probes with no else — so any key
// outside a word's subset parsed successfully and was then dropped on the
// floor. `replace "-" "+" "a-a-a" {all:true}` returned `a+a-a`: the real
// key is `scope:"all"`, `all` is not a key at all, and nothing said so.
// A misspelling behaved exactly like the default.
//
// Keys are per WORD, not global, because the union is not a contract: only
// `replace` honours `count`, only `pad` honours `trunc`. Accepting the
// union would still silently ignore `{trunc:true}` on `replace`.
var strOptKeys = map[string]map[string]bool{
	"concat":     keySet("sep", "skipEmpty", "skipNullish"),
	"split":      keySet("cs", "keepEmpty", "lim", "mode", "norm", "trimParts"),
	"trim":       keySet("cs", "fill", "chars", "norm", "side"),
	"contains":   keySet("anchored", "cs", "mode", "norm", "wholeWord"),
	"indexof":    keySet("cs", "from", "mode", "norm", "occ"),
	"replace":    keySet("count", "cs", "from", "mode", "norm", "scope"),
	"changecase": keySet("norm", "style"),
	"normalize":  keySet("collapseWs", "eol", "form", "trim"),
	"repeat":     keySet("sep"),
	"pad":        keySet("fill", "chars", "side", "trunc"),
	"match":      keySet("cs", "mode", "norm", "scope"),
	"escape":     keySet("quote", "tgt"),
}

// strOptEnums lists the legal values for the enumerated option keys. An
// out-of-domain value was as silent as an unknown key: `scope:"bogus"`
// simply was not "all", so it behaved as "first".
var strOptEnums = map[string][]string{
	"cs":    {"sensitive", "insensitive"},
	"mode":  {"literal", "shell"},
	"side":  {"left", "right", "both"},
	"scope": {"first", "all"},
	"occ":   {"first", "last"},
	"eol":   {"preserve", "lf", "crlf"},
	// The five below were MISSING, and the silence they left is exactly the
	// one this table was introduced to end (NUR127). Each is consumed by a
	// switch with a quiet default, so an out-of-domain value did not fail —
	// it picked an arm the caller did not ask for:
	//   changecase {style:'typo'} -> 'hello'  (the `default: // "lower"` arm)
	//   escape     {tgt:'typo'}   -> 'a\ b'   (the `default: // "sh"` arm)
	//   escape     {quote:'typo'} -> unquoted (the switch has no default)
	//   normalize  {form:'typo'}  -> unchanged (applyNorm's `default: return s`)
	//   split      {norm:'typo'}  -> unnormalised (same applyNorm default)
	// `quote` carries "none" because the corpus spells the no-quoting request
	// that way (corpus-modules.tsv:121), so it is a member of the domain and
	// not merely the absence of one.
	"style": {"lower", "upper", "capitalize", "title", "sentence", "fold"},
	"tgt":   {"sh", "bash", "sed", "awk", "grep"},
	"quote": {"none", "single", "double"},
	"form":  {"NFC", "NFD", "NFKC", "NFKD"},
	"norm":  {"NFC", "NFD", "NFKC", "NFKD"},
}

// strOptEnumsFold names the enumerated keys whose value is upper-cased before
// it is used, so their domain is checked case-INSENSITIVELY: `form:'nfc'` and
// `form:'NFC'` are the same request and both must pass. Checking these
// case-sensitively would refuse a spelling that has always worked.
var strOptEnumsFold = map[string]bool{"form": true, "norm": true}

func keySet(names ...string) map[string]bool {
	s := make(map[string]bool, len(names))
	for _, n := range names {
		s[n] = true
	}
	return s
}

// validateStrOpts rejects keys the word does not honour and values outside
// an enumerated key's domain. Returns nil when v is not a concrete map
// (the no-options call); an unknown WORD is a programming error in the
// caller, not user input, so it is reported as such.
func validateStrOpts(r *Registry, v Value, word string) error {
	if !v.Parent.Equal(TMap) || !IsConcrete(v) {
		return nil
	}
	allowed, ok := strOptKeys[word]
	if !ok {
		return r.BoruErrorAt("string_option_error",
			word+": no option key set is registered for this word", word, v.Pos())
	}
	m, _ := AsMap(v)
	for _, k := range m.Keys() {
		if !allowed[k] {
			return r.BoruErrorHintAt("string_option_error",
				word+": unknown option "+quoteKey(k), word,
				"known options: "+strings.Join(sortedKeys(allowed), ", "), v.Pos())
		}
		legal, enum := strOptEnums[k]
		if !enum {
			continue
		}
		val, _ := m.Get(k)
		// `norm` doubles as a BOOLEAN switch — `norm:true` means NFC — so a
		// boolean here is the other spelling of the option, not a member of
		// the string domain. It is legal and there is nothing to check.
		if k == "norm" && val.Parent.ConformsTo(TBoolean) {
			continue
		}
		got := ValToString(val)
		probe := got
		if strOptEnumsFold[k] {
			probe = strings.ToUpper(got)
		}
		if !contains(legal, probe) {
			return r.BoruErrorHintAt("string_option_error",
				word+": option "+quoteKey(k)+" got "+quoteKey(got), word,
				quoteKey(k)+" must be one of: "+strings.Join(legal, ", "), v.Pos())
		}
	}
	return nil
}

func quoteKey(s string) string { return "\"" + s + "\"" }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// parseStrOpts extracts common string options from a boru map value.
func parseStrOpts(v Value) strOpts {
	var o strOpts
	o.cs = "sensitive"
	o.mode = "literal"
	o.side = "both"
	o.scope = "first"
	o.occ = "first"
	o.eol = "preserve"
	o.form = "NFC"
	o.quote = "none"

	if !v.Parent.Equal(TMap) || !IsConcrete(v) {
		return o
	}
	m, _ := AsMap(v)

	if val, ok := m.Get("norm"); ok {
		if val.Parent.ConformsTo(TBoolean) {
			_as1, _ := AsBoolean(val)
			if _as1 {
				o.normForm = "NFC"
			}
		} else if val.Parent.ConformsTo(TString) || IsAtom(val) {
			o.normForm = strings.ToUpper(ValToString(val))
		}
	}
	if val, ok := m.Get("cs"); ok {
		o.cs = ValToString(val)
	}
	if val, ok := m.Get("mode"); ok {
		o.mode = ValToString(val)
	}
	if val, ok := m.Get("side"); ok {
		o.side = ValToString(val)
	}
	if val, ok := m.Get("sep"); ok {
		o.sep = ValToString(val)
		o.hasSep = true
	}
	if val, ok := m.Get("fill"); ok {
		o.fill = ValToString(val)
	}
	if val, ok := m.Get("style"); ok {
		o.style = ValToString(val)
	}
	if val, ok := m.Get("tgt"); ok {
		o.tgt = ValToString(val)
	}
	if val, ok := m.Get("quote"); ok {
		o.quote = ValToString(val)
	}
	if val, ok := m.Get("scope"); ok {
		o.scope = ValToString(val)
	}
	if val, ok := m.Get("occ"); ok {
		o.occ = ValToString(val)
	}
	if n, ok := MapFieldInteger(m, "from"); ok {
		o.from = n
		o.hasFrom = true
	}
	if n, ok := MapFieldInteger(m, "count"); ok {
		o.count = n
		o.hasCount = true
	}
	if n, ok := MapFieldInteger(m, "lim"); ok {
		o.lim = n
		o.hasLim = true
	}

	// boolean flags
	if b, ok := MapFieldBoolean(m, "skipEmpty"); ok {
		o.skipEmpty = b
	}
	if b, ok := MapFieldBoolean(m, "skipNullish"); ok {
		o.skipNullish = b
	}
	if b, ok := MapFieldBoolean(m, "keepEmpty"); ok {
		o.keepEmpty = b
	}
	if b, ok := MapFieldBoolean(m, "trimParts"); ok {
		o.trimParts = b
	}
	if b, ok := MapFieldBoolean(m, "wholeWord"); ok {
		o.wholeWord = b
	}
	if val, ok := m.Get("anchored"); ok {
		if val.Parent.ConformsTo(TBoolean) {
			_as10, _ := AsBoolean(val)
			if _as10 {
				o.anchored = "both"
			}
		} else {
			o.anchored = ValToString(val)
		}
	}
	if b, ok := MapFieldBoolean(m, "trunc"); ok {
		o.trunc = b
	}

	// normalize-specific
	if b, ok := MapFieldBoolean(m, "trim"); ok {
		o.trim = b
	}
	if b, ok := MapFieldBoolean(m, "collapseWs"); ok {
		o.collapseWs = b
	}
	if val, ok := m.Get("eol"); ok {
		o.eol = ValToString(val)
	}
	if val, ok := m.Get("form"); ok {
		o.form = strings.ToUpper(ValToString(val))
	}

	// chars for trim
	if val, ok := m.Get("chars"); ok {
		o.fill = ValToString(val) // reuse fill field for trim chars
	}

	return o
}

// applyNorm normalizes input string if normForm is set.
func applyNorm(s string, normForm string) string {
	switch normForm {
	case "NFC":
		return norm.NFC.String(s)
	case "NFD":
		return norm.NFD.String(s)
	case "NFKC":
		return norm.NFKC.String(s)
	case "NFKD":
		return norm.NFKD.String(s)
	default:
		return s
	}
}

// shellMatch performs shell-style glob matching using filepath.Match.
// The pattern uses *, ?, and [...] wildcards.
func shellMatch(pattern, s string, caseInsensitive bool) bool {
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		s = strings.ToLower(s)
	}
	matched, _ := filepath.Match(pattern, s)
	return matched
}

// runeBounds returns every rune-boundary byte offset of s, including
// the trailing len(s). Shell candidates are cut at these boundaries so
// every reported offset is a valid position in the ORIGINAL string.
func runeBounds(s string) []int {
	b := make([]int, 0, len(s)+1)
	for i := range s {
		b = append(b, i)
	}
	return append(b, len(s))
}

// foldShell lowercases per-rune (simple fold) for shell matching. It is
// applied to the PATTERN and to each candidate SUBSTRING COPY — never
// to the searched string itself — so reported offsets always index the
// original string. Lowering a whole copy and searching it returned
// offsets into the COPY, which drift whenever folding changes a rune's
// byte width (İ U+0130 is 2 bytes, its lowercase i is 1 — flagged in
// PR #306 review).
func foldShell(s string) string {
	return strings.Map(unicode.ToLower, s)
}

// shellFind finds the first occurrence of a shell pattern in a string
// by trying the pattern against every rune-boundary substring.
// Returns (byte index, byte length) into the ORIGINAL string, or
// (-1, 0) if not found.
func shellFind(s, pattern string, caseInsensitive bool) (int, int) {
	pat := pattern
	if caseInsensitive {
		pat = foldShell(pat)
	}
	bounds := runeBounds(s)
	for bi := 0; bi < len(bounds)-1; bi++ {
		for bj := bi + 1; bj < len(bounds); bj++ {
			sub := s[bounds[bi]:bounds[bj]]
			if caseInsensitive {
				sub = foldShell(sub)
			}
			if matched, _ := filepath.Match(pat, sub); matched {
				return bounds[bi], bounds[bj] - bounds[bi]
			}
		}
	}
	return -1, 0
}

// shellFindAll finds all non-overlapping occurrences of a shell
// pattern, as (start, end) byte-offset pairs into the ORIGINAL string.
func shellFindAll(s, pattern string, caseInsensitive bool) [][2]int {
	pat := pattern
	if caseInsensitive {
		pat = foldShell(pat)
	}
	bounds := runeBounds(s)
	var results [][2]int
	pos := 0
	for pos < len(bounds)-1 {
		found := false
		for i := pos; i < len(bounds)-1 && !found; i++ {
			for j := i + 1; j < len(bounds); j++ {
				sub := s[bounds[i]:bounds[j]]
				if caseInsensitive {
					sub = foldShell(sub)
				}
				if matched, _ := filepath.Match(pat, sub); matched {
					results = append(results, [2]int{bounds[i], bounds[j]})
					pos = j
					found = true
					break
				}
			}
			if !found {
				pos = i + 1
			}
		}
		if !found {
			break
		}
	}
	return results
}

// isWordBoundary checks if position i in string s is at a word boundary.
func isWordBoundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	next, _ := utf8.DecodeRuneInString(s[i:])
	prevWord := unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_'
	nextWord := unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_'
	return prevWord != nextWord
}
