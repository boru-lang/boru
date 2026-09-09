package native

import (
	"strings"
	"testing"
)

// Coverage tests for native_string.go and native_string_helpers.go: the
// opts-variant handlers (split/trim/contains/indexof/replace/changecase/
// normalize/repeat/pad/match/escape), the shell/insensitive/normalized
// modes behind them, and the option-parsing helper. Positive rows are
// paired with rejection rows per the repo test discipline.

// covOpts builds a concrete boru options map from Go pairs.
func covOpts(pairs map[string]Value) Value {
	om := NewOrderedMap()
	for k, v := range pairs {
		om.Set(k, v)
	}
	return NewMap(om)
}

// covStrOut unwraps a single-string handler result.
func covStrOut(t *testing.T, out []Value, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	s, cerr := out[0].AsConcreteString()
	if cerr != nil {
		t.Fatalf("result not a concrete string: %v", cerr)
	}
	return s
}

// covListOut unwraps a list-of-strings handler result, joined with "|".
func covListOut(t *testing.T, out []Value, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lst, lerr := AsList(out[0])
	if lerr != nil {
		t.Fatalf("result not a list: %v", lerr)
	}
	elems := lst.Slice()
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		s, serr := e.AsConcreteString()
		if serr != nil {
			t.Fatalf("list element not a string: %v", serr)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "|")
}

// covBoolOut unwraps a single-boolean handler result.
func covBoolOut(t *testing.T, out []Value, err error) bool {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, berr := out[0].AsConcreteBoolean()
	if berr != nil {
		t.Fatalf("result not a boolean: %v", berr)
	}
	return b
}

// covIntOut unwraps a single-integer handler result.
func covIntOut(t *testing.T, out []Value, err error) int64 {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n, nerr := out[0].AsConcreteInteger()
	if nerr != nil {
		t.Fatalf("result not an integer: %v", nerr)
	}
	return n
}

// ---- parseStrOpts ----

func TestParseStrOptsAllStringKeys(t *testing.T) {
	o := parseStrOpts(covOpts(map[string]Value{
		"norm":        NewString("nfd"),
		"cs":          NewString("insensitive"),
		"mode":        NewString("shell"),
		"side":        NewString("left"),
		"sep":         NewString("-"),
		"fill":        NewString("*"),
		"style":       NewString("title"),
		"tgt":         NewString("sed"),
		"quote":       NewString("single"),
		"scope":       NewString("all"),
		"occ":         NewString("last"),
		"from":        NewInteger(2),
		"count":       NewInteger(3),
		"lim":         NewInteger(4),
		"skipEmpty":   NewBoolean(true),
		"skipNullish": NewBoolean(true),
		"keepEmpty":   NewBoolean(true),
		"trimParts":   NewBoolean(true),
		"wholeWord":   NewBoolean(true),
		"anchored":    NewString("start"),
		"trunc":       NewBoolean(true),
		"trim":        NewBoolean(true),
		"collapseWs":  NewBoolean(true),
		"eol":         NewString("lf"),
		"form":        NewString("nfkc"),
		"chars":       NewString("xy"),
	}))
	if o.normForm != "NFD" || o.cs != "insensitive" || o.mode != "shell" {
		t.Errorf("core opts wrong: %+v", o)
	}
	if o.side != "left" || !o.hasSep || o.sep != "-" || o.style != "title" {
		t.Errorf("side/sep/style wrong: %+v", o)
	}
	if o.tgt != "sed" || o.quote != "single" {
		t.Errorf("tgt/quote wrong: %+v", o)
	}
	if o.scope != "all" || o.occ != "last" {
		t.Errorf("scope/occ wrong: %+v", o)
	}
	if !o.hasFrom || o.from != 2 || !o.hasCount || o.count != 3 || !o.hasLim || o.lim != 4 {
		t.Errorf("from/count/lim wrong: %+v", o)
	}
	if !o.skipEmpty || !o.skipNullish || !o.keepEmpty || !o.trimParts || !o.wholeWord {
		t.Errorf("bool flags wrong: %+v", o)
	}
	if o.anchored != "start" || !o.trunc {
		t.Errorf("anchored/trunc wrong: %+v", o)
	}
	if !o.trim || !o.collapseWs || o.eol != "lf" || o.form != "NFKC" {
		t.Errorf("normalize opts wrong: %+v", o)
	}
	// chars is read after fill and reuses the fill slot.
	if o.fill != "xy" {
		t.Errorf("chars did not overwrite fill: %q", o.fill)
	}
}

func TestParseStrOptsBooleanVariants(t *testing.T) {
	o := parseStrOpts(covOpts(map[string]Value{
		"norm":     NewBoolean(true),
		"anchored": NewBoolean(true),
	}))
	if o.normForm != "NFC" {
		t.Errorf("norm:true → %q, want NFC", o.normForm)
	}
	if o.anchored != "both" {
		t.Errorf("anchored:true → %q, want both", o.anchored)
	}

	// norm:false / anchored:false leave the fields unset.
	o = parseStrOpts(covOpts(map[string]Value{
		"norm":     NewBoolean(false),
		"anchored": NewBoolean(false),
	}))
	if o.normForm != "" || o.anchored != "" {
		t.Errorf("false booleans set fields: norm=%q anchored=%q", o.normForm, o.anchored)
	}

	// norm as atom.
	o = parseStrOpts(covOpts(map[string]Value{"norm": NewAtom("nfkd")}))
	if o.normForm != "NFKD" {
		t.Errorf("norm atom → %q, want NFKD", o.normForm)
	}
}

func TestParseStrOptsNonMapDefaults(t *testing.T) {
	o := parseStrOpts(NewInteger(1))
	if o.cs != "sensitive" || o.mode != "literal" || o.side != "both" ||
		o.scope != "first" || o.occ != "first" || o.eol != "preserve" ||
		o.form != "NFC" || o.quote != "none" {
		t.Errorf("defaults wrong for non-map input: %+v", o)
	}
	// A non-concrete map (type literal) also yields defaults.
	o = parseStrOpts(NewTypeLiteral(TMap))
	if o.cs != "sensitive" || o.mode != "literal" {
		t.Errorf("defaults wrong for map type literal: %+v", o)
	}
}

// ---- helper functions ----

func TestApplyNormForms(t *testing.T) {
	composed := "é"    // é
	decomposed := "é" // e + combining acute
	if got := applyNorm(decomposed, "NFC"); got != composed {
		t.Errorf("NFC = %q, want %q", got, composed)
	}
	if got := applyNorm(composed, "NFD"); got != decomposed {
		t.Errorf("NFD = %q, want %q", got, decomposed)
	}
	if got := applyNorm("ﬁ", "NFKC"); got != "fi" {
		t.Errorf("NFKC(ﬁ) = %q, want fi", got)
	}
	if got := applyNorm("ﬁ", "NFKD"); got != "fi" {
		t.Errorf("NFKD(ﬁ) = %q, want fi", got)
	}
	if got := applyNorm(decomposed, "bogus"); got != decomposed {
		t.Errorf("unknown form must be identity, got %q", got)
	}
}

func TestShellMatchFindFindAll(t *testing.T) {
	if !shellMatch("a*", "abc", false) {
		t.Error("shellMatch(a*, abc) = false, want true")
	}
	if shellMatch("z*", "abc", false) {
		t.Error("shellMatch(z*, abc) = true, want false")
	}
	if !shellMatch("A*", "abc", true) {
		t.Error("case-insensitive shellMatch(A*, abc) = false, want true")
	}

	if idx, n := shellFind("xx1yy", "[0-9]", false); idx != 2 || n != 1 {
		t.Errorf("shellFind = (%d,%d), want (2,1)", idx, n)
	}
	if idx, n := shellFind("xxxyy", "[0-9]", false); idx != -1 || n != 0 {
		t.Errorf("shellFind miss = (%d,%d), want (-1,0)", idx, n)
	}
	if idx, _ := shellFind("xxAyy", "a", true); idx != 2 {
		t.Errorf("case-insensitive shellFind = %d, want 2", idx)
	}

	all := shellFindAll("a1b22c", "[0-9]", false)
	if len(all) != 3 {
		t.Fatalf("shellFindAll = %v, want 3 matches", all)
	}
	if all[0] != [2]int{1, 2} || all[2] != [2]int{4, 5} {
		t.Errorf("shellFindAll positions = %v", all)
	}
	if got := shellFindAll("abc", "[0-9]", false); len(got) != 0 {
		t.Errorf("shellFindAll miss = %v, want empty", got)
	}
	if got := shellFindAll("A1", "a", true); len(got) != 1 {
		t.Errorf("case-insensitive shellFindAll = %v, want 1 match", got)
	}
}

func TestIsWordBoundaryCases(t *testing.T) {
	if !isWordBoundary("abc", 0) || !isWordBoundary("abc", 3) {
		t.Error("string edges must be boundaries")
	}
	if isWordBoundary("ab", 1) {
		t.Error("mid-word must not be a boundary")
	}
	if !isWordBoundary("a b", 1) {
		t.Error("letter/space must be a boundary")
	}
	if isWordBoundary("a_b", 1) {
		t.Error("underscore counts as a word char")
	}
}

func TestFoldLastIndex(t *testing.T) {
	if got := foldLastIndex("aXaXa", "x"); got != 3 {
		t.Errorf("foldLastIndex(aXaXa, x) = %d, want 3", got)
	}
	if got := foldLastIndex("abc", "z"); got != -1 {
		t.Errorf("foldLastIndex miss = %d, want -1", got)
	}
	if got := foldLastIndex("abc", ""); got != 3 {
		t.Errorf("foldLastIndex empty substr = %d, want 3", got)
	}
}

// ---- unaryStringNative (upper/lower builder) ----

func TestUnaryStringNativeHandler(t *testing.T) {
	nf := unaryStringNative("upper", strings.ToUpper)
	h := nf.Signatures[0].DispatchHandler()

	out, err := h([]Value{NewString("ab")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "AB" {
		t.Errorf("upper(ab) = %q, want AB", got)
	}
	out, err = h([]Value{NewAtom("cd")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "CD" {
		t.Errorf("upper(atom cd) = %q, want CD", got)
	}
	if _, err = h([]Value{NewInteger(5)}, nil, nil, nil); err == nil {
		t.Error("upper(5): expected error, got nil")
	}
}

// ---- concat ----

func TestConcatOptsHandler(t *testing.T) {
	list := NewList([]Value{
		NewString("a"), {Parent: TNone}, NewString(""), NewString("b"),
	})
	out, err := concatOptsHandler([]Value{covOpts(map[string]Value{
		"sep":         NewString(","),
		"skipEmpty":   NewBoolean(true),
		"skipNullish": NewBoolean(true),
	}), list}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "a,b" {
		t.Errorf("concat skip opts = %q, want a,b", got)
	}

	// Without skips: None renders empty, empties kept.
	out, err = concatOptsHandler([]Value{covOpts(map[string]Value{
		"sep": NewString("-"),
	}), list}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "a--"+"-b" {
		t.Errorf("concat keep = %q, want a---b", got)
	}

	// Non-concrete list is rejected.
	if _, err = doConcat(NewTypeLiteral(TList), strOpts{}); err == nil {
		t.Error("doConcat(type literal): expected error, got nil")
	}
}

// ---- split ----

func TestSplitHandlerBasic(t *testing.T) {
	out, err := splitHandler([]Value{NewString(","), NewString("a,b,c")}, nil, nil, nil)
	if got := covListOut(t, out, err); got != "a|b|c" {
		t.Errorf("split = %q, want a|b|c", got)
	}
}

func TestSplitOptsVariants(t *testing.T) {
	cases := []struct {
		name       string
		sep, input string
		opts       map[string]Value
		want       string
	}{
		{"insensitive", "X", "aXbxc", map[string]Value{"cs": NewString("insensitive")}, "a|b|c"},
		{"literal lim", ",", "a,b,c,d", map[string]Value{"lim": NewInteger(2)}, "a|b,c,d"},
		{"shell mode", "[0-9]", "a1b2c", map[string]Value{"mode": NewString("shell")}, "a|b|c"},
		{"shell lim", "[0-9]", "a1b2c", map[string]Value{"mode": NewString("shell"), "lim": NewInteger(2)}, "a|b[0-9]c"},
		{"keepEmpty trimParts", ",", " a , ,b ", map[string]Value{"keepEmpty": NewBoolean(true), "trimParts": NewBoolean(true)}, "a||b"},
		{"drop empty default", ",", "a,,b", map[string]Value{}, "a|b"},
		{"norm applied", ",", "é,x", map[string]Value{"norm": NewString("nfc")}, "é|x"},
		{"empty input", ",", "", map[string]Value{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := splitOptsHandler(
				[]Value{NewString(c.sep), NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			if got := covListOut(t, out, err); got != c.want {
				t.Errorf("split %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---- trim ----

func TestTrimHandlerAndOpts(t *testing.T) {
	out, err := trimHandler([]Value{NewString("  hi  ")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "hi" {
		t.Errorf("trim = %q, want hi", got)
	}

	cases := []struct {
		name, input string
		opts        map[string]Value
		want        string
	}{
		{"chars both", "xxhixx", map[string]Value{"chars": NewString("x")}, "hi"},
		{"chars left", "xxhixx", map[string]Value{"chars": NewString("x"), "side": NewString("left")}, "hixx"},
		{"chars right", "xxhixx", map[string]Value{"chars": NewString("x"), "side": NewString("right")}, "xxhi"},
		{"chars insensitive", "XxhiXx", map[string]Value{"chars": NewString("x"), "cs": NewString("insensitive")}, "hi"},
		{"ws left", "  hi  ", map[string]Value{"side": NewString("left")}, "hi  "},
		{"ws right", "  hi  ", map[string]Value{"side": NewString("right")}, "  hi"},
		{"ws both", "  hi  ", map[string]Value{}, "hi"},
		{"norm", "é", map[string]Value{"norm": NewString("nfc")}, "é"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := trimOptsHandler([]Value{NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("trim %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---- contains ----

func TestContainsHandlerBasic(t *testing.T) {
	out, err := containsHandler([]Value{NewString("lo"), NewString("hello")}, nil, nil, nil)
	if !covBoolOut(t, out, err) {
		t.Error("contains(lo, hello) = false, want true")
	}
	out, err = containsHandler([]Value{NewString("zz"), NewString("hello")}, nil, nil, nil)
	if covBoolOut(t, out, err) {
		t.Error("contains(zz, hello) = true, want false")
	}
}

func TestContainsOptsVariants(t *testing.T) {
	cases := []struct {
		name             string
		needle, haystack string
		opts             map[string]Value
		want             bool
	}{
		{"ci", "LO", "hello", map[string]Value{"cs": NewString("insensitive")}, true},
		{"anchored start", "he", "hello", map[string]Value{"anchored": NewString("start")}, true},
		{"anchored start miss", "lo", "hello", map[string]Value{"anchored": NewString("start")}, false},
		{"anchored end", "lo", "hello", map[string]Value{"anchored": NewString("end")}, true},
		{"anchored end miss", "he", "hello", map[string]Value{"anchored": NewString("end")}, false},
		{"anchored both eq", "hello", "hello", map[string]Value{"anchored": NewBoolean(true)}, true},
		{"anchored both neq", "hell", "hello", map[string]Value{"anchored": NewBoolean(true)}, false},
		{"whole word hit", "cat", "a cat sat", map[string]Value{"wholeWord": NewBoolean(true)}, true},
		{"whole word miss", "cat", "concatenate", map[string]Value{"wholeWord": NewBoolean(true)}, false},
		{"whole word ci", "CAT", "a cat sat", map[string]Value{"wholeWord": NewBoolean(true), "cs": NewString("insensitive")}, true},
		{"start wholeWord", "hi", "hi there", map[string]Value{"anchored": NewString("start"), "wholeWord": NewBoolean(true)}, true},
		{"start wholeWord miss", "hi", "hit", map[string]Value{"anchored": NewString("start"), "wholeWord": NewBoolean(true)}, false},
		{"end wholeWord", "there", "hi there", map[string]Value{"anchored": NewString("end"), "wholeWord": NewBoolean(true)}, true},
		{"end wholeWord miss", "here", "hithere", map[string]Value{"anchored": NewString("end"), "wholeWord": NewBoolean(true)}, false},
		{"shell", "[0-9]", "ab3cd", map[string]Value{"mode": NewString("shell")}, true},
		{"shell miss", "[0-9]", "abcd", map[string]Value{"mode": NewString("shell")}, false},
		{"shell anchored both", "a*", "abc", map[string]Value{"mode": NewString("shell"), "anchored": NewBoolean(true)}, true},
		{"shell anchored start", "a?", "abc", map[string]Value{"mode": NewString("shell"), "anchored": NewString("start")}, true},
		{"shell anchored start miss", "z*", "abc", map[string]Value{"mode": NewString("shell"), "anchored": NewString("start")}, false},
		{"shell anchored end", "*c", "abc", map[string]Value{"mode": NewString("shell"), "anchored": NewString("end")}, true},
		{"shell anchored end miss", "z*", "abc", map[string]Value{"mode": NewString("shell"), "anchored": NewString("end")}, false},
		{"norm", "é", "xéy", map[string]Value{"norm": NewString("nfc")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := containsOptsHandler(
				[]Value{NewString(c.needle), NewString(c.haystack), covOpts(c.opts)}, nil, nil, nil)
			if got := covBoolOut(t, out, err); got != c.want {
				t.Errorf("contains %s = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// ---- indexof ----

func TestIndexOfOptsVariants(t *testing.T) {
	cases := []struct {
		name             string
		needle, haystack string
		opts             map[string]Value
		want             int64
	}{
		{"from", "a", "banana", map[string]Value{"from": NewInteger(2)}, 3},
		{"from negative clamps", "a", "banana", map[string]Value{"from": NewInteger(-5)}, 1},
		{"from beyond len", "a", "abc", map[string]Value{"from": NewInteger(10)}, -1},
		{"occ last", "a", "banana", map[string]Value{"occ": NewString("last")}, 5},
		{"ci", "NA", "banana", map[string]Value{"cs": NewString("insensitive")}, 2},
		{"ci last", "NA", "banana", map[string]Value{"cs": NewString("insensitive"), "occ": NewString("last")}, 4},
		{"shell", "[0-9]", "ab1c2", map[string]Value{"mode": NewString("shell")}, 2},
		{"shell from", "[0-9]", "ab1c2", map[string]Value{"mode": NewString("shell"), "from": NewInteger(3)}, 4},
		{"shell last", "[0-9]", "ab1c2", map[string]Value{"mode": NewString("shell"), "occ": NewString("last")}, 4},
		{"shell miss", "[0-9]", "abc", map[string]Value{"mode": NewString("shell")}, -1},
		{"norm", "é", "xéy", map[string]Value{"norm": NewString("nfc")}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := indexOfOptsHandler(
				[]Value{NewString(c.needle), NewString(c.haystack), covOpts(c.opts)}, nil, nil, nil)
			if got := covIntOut(t, out, err); got != c.want {
				t.Errorf("indexof %s = %d, want %d", c.name, got, c.want)
			}
		})
	}
}

func TestShellFindLastMiss(t *testing.T) {
	if got := shellFindLast("abc", "[0-9]", false); got != -1 {
		t.Errorf("shellFindLast miss = %d, want -1", got)
	}
}

// ---- replace ----

func TestReplaceHandlerBasic(t *testing.T) {
	out, err := replaceHandler(
		[]Value{NewString("l"), NewString("_"), NewString("hello")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "he_lo" {
		t.Errorf("replace = %q, want he_lo", got)
	}
}

func TestReplaceOptsVariants(t *testing.T) {
	cases := []struct {
		name                string
		search, repl, input string
		opts                map[string]Value
		want                string
	}{
		{"all", "l", "_", "hello", map[string]Value{"scope": NewString("all")}, "he__o"},
		{"all count", "a", "_", "aaaa", map[string]Value{"scope": NewString("all"), "count": NewInteger(2)}, "__aa"},
		{"all from", "a", "_", "aaaa", map[string]Value{"scope": NewString("all"), "from": NewInteger(2)}, "aa__"},
		{"all ci", "L", "_", "hello", map[string]Value{"scope": NewString("all"), "cs": NewString("insensitive")}, "he__o"},
		{"all ci count", "A", "_", "aaaa", map[string]Value{"scope": NewString("all"), "cs": NewString("insensitive"), "count": NewInteger(1)}, "_aaa"},
		{"first ci", "L", "_", "hello", map[string]Value{"cs": NewString("insensitive")}, "he_lo"},
		{"first from found", "a", "_", "aaaa", map[string]Value{"from": NewInteger(2)}, "aa_a"},
		{"first from missing", "z", "_", "aaaa", map[string]Value{"from": NewInteger(2)}, "aaaa"},
		{"shell first", "[0-9]", "#", "a1b2", map[string]Value{"mode": NewString("shell")}, "a#b2"},
		{"shell all", "[0-9]", "#", "a1b2", map[string]Value{"mode": NewString("shell"), "scope": NewString("all")}, "a#b#"},
		{"shell count", "[0-9]", "#", "a1b2c3", map[string]Value{"mode": NewString("shell"), "scope": NewString("all"), "count": NewInteger(2)}, "a#b#c3"},
		{"shell nomatch", "[0-9]", "#", "abc", map[string]Value{"mode": NewString("shell")}, "abc"},
		{"shell from", "[0-9]", "#", "a1b2", map[string]Value{"mode": NewString("shell"), "scope": NewString("all"), "from": NewInteger(2)}, "a1b#"},
		{"norm", "é", "X", "é", map[string]Value{"norm": NewString("nfc")}, "X"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := replaceOptsHandler(
				[]Value{NewString(c.search), NewString(c.repl), NewString(c.input), covOpts(c.opts)},
				nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("replace %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---- changecase ----

func TestChangeCaseHandlerAndOpts(t *testing.T) {
	out, err := changeCaseHandler([]Value{NewString("HeLLo")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "hello" {
		t.Errorf("changecase default = %q, want hello", got)
	}

	cases := []struct {
		name, input string
		opts        map[string]Value
		want        string
	}{
		{"upper", "hello", map[string]Value{"style": NewString("upper")}, "HELLO"},
		{"capitalize", "hello world", map[string]Value{"style": NewString("capitalize")}, "Hello world"},
		{"title", "hello world", map[string]Value{"style": NewString("title")}, "Hello World"},
		{"title punct", "foo-bar", map[string]Value{"style": NewString("title")}, "Foo-Bar"},
		{"sentence", "HELLO WORLD", map[string]Value{"style": NewString("sentence")}, "Hello world"},
		{"fold", "HeLLo", map[string]Value{"style": NewString("fold")}, "hello"},
		{"lower explicit", "HeLLo", map[string]Value{"style": NewString("lower")}, "hello"},
		{"empty style defaults", "HeLLo", map[string]Value{}, "hello"},
		{"norm then case", "É", map[string]Value{"norm": NewString("nfc"), "style": NewString("lower")}, "é"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := changeCaseOptsHandler(
				[]Value{NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("changecase %s = %q, want %q", c.name, got, c.want)
			}
		})
	}

	if got := capitalize(""); got != "" {
		t.Errorf("capitalize(empty) = %q, want empty", got)
	}
}

// ---- normalize ----

func TestNormalizeHandlerAndOpts(t *testing.T) {
	out, err := normalizeHandler([]Value{NewString("é")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "é" {
		t.Errorf("normalize default NFC = %q, want é", got)
	}

	cases := []struct {
		name, input string
		opts        map[string]Value
		want        string
	}{
		{"nfd", "é", map[string]Value{"form": NewString("nfd")}, "é"},
		{"eol lf", "a\r\nb\rc", map[string]Value{"eol": NewString("lf")}, "a\nb\nc"},
		{"eol crlf", "a\nb\rc", map[string]Value{"eol": NewString("crlf")}, "a\r\nb\r\nc"},
		{"collapse ws", "a \t b", map[string]Value{"collapseWs": NewBoolean(true)}, "a b"},
		{"collapse keeps newline", "a  \n  b", map[string]Value{"collapseWs": NewBoolean(true)}, "a \n b"},
		{"trim", "  x  ", map[string]Value{"trim": NewBoolean(true)}, "x"},
		{"combined", " a  b \r\n", map[string]Value{"eol": NewString("lf"), "collapseWs": NewBoolean(true), "trim": NewBoolean(true)}, "a b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := normalizeOptsHandler(
				[]Value{NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("normalize %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---- repeat ----

func TestRepeatHandlerAndOpts(t *testing.T) {
	out, err := repeatHandler([]Value{NewInteger(2), NewString("ab")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "abab" {
		t.Errorf("repeat = %q, want abab", got)
	}

	out, err = repeatOptsHandler(
		[]Value{NewInteger(3), NewString("ab"), covOpts(map[string]Value{"sep": NewString("-")})},
		nil, nil, nil)
	if got := covStrOut(t, out, err); got != "ab-ab-ab" {
		t.Errorf("repeat sep = %q, want ab-ab-ab", got)
	}

	out, err = repeatOptsHandler(
		[]Value{NewInteger(0), NewString("ab"), covOpts(map[string]Value{"sep": NewString("-")})},
		nil, nil, nil)
	if got := covStrOut(t, out, err); got != "" {
		t.Errorf("repeat 0 with sep = %q, want empty", got)
	}

	// Huge count with separator must be bounded (ADR-005).
	_, err = repeatOptsHandler(
		[]Value{NewInteger(1 << 40), NewString("ab"), covOpts(map[string]Value{"sep": NewString("-")})},
		nil, nil, nil)
	if err == nil {
		t.Error("repeat huge count with sep: expected error, got nil")
	}
}

// ---- pad ----

func TestPadHandlerAndOpts(t *testing.T) {
	out, err := padHandler([]Value{NewInteger(5), NewString("ab")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "ab   " {
		t.Errorf("pad = %q, want %q", got, "ab   ")
	}

	cases := []struct {
		name  string
		width int64
		opts  map[string]Value
		input string
		want  string
	}{
		{"left fill", 5, map[string]Value{"side": NewString("left"), "fill": NewString("0")}, "ab", "000ab"},
		{"both", 6, map[string]Value{"side": NewString("both"), "fill": NewString("-")}, "ab", "--ab--"},
		{"default side right", 5, map[string]Value{}, "ab", "ab   "},
		{"trunc", 3, map[string]Value{"trunc": NewBoolean(true)}, "hello", "hel"},
		{"no trunc keeps input", 3, map[string]Value{}, "hello", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := padOptsHandler(
				[]Value{NewInteger(c.width), covOpts(c.opts), NewString(c.input)}, nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("pad %s = %q, want %q", c.name, got, c.want)
			}
		})
	}

	// Non-concrete opts map is rejected.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = padOptsHandler(
		[]Value{NewInteger(5), NewTypeLiteral(TMap), NewString("ab")}, nil, nil, r)
	if err == nil {
		t.Error("pad with type-literal opts: expected error, got nil")
	}

	// Bounds (ADR-005): negative and over-large targets error.
	if _, err = doPad("ab", -1, strOpts{}); err == nil {
		t.Error("doPad negative: expected error, got nil")
	}
	if _, err = doPad("ab", int64(maxStringResultBytes)+1, strOpts{}); err == nil {
		t.Error("doPad huge: expected error, got nil")
	}
}

// ---- match ----

// covMatchOut reads (ok, n, firstIndex) from a match-result map.
// firstIndex is -1 when there is no fst entry.
func covMatchOut(t *testing.T, out []Value, err error) (bool, int64, int64) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, merr := AsMap(out[0])
	if merr != nil {
		t.Fatalf("match result not a map: %v", merr)
	}
	okv, _ := m.Get("ok")
	ok, _ := AsBoolean(okv)
	nv, _ := m.Get("n")
	n, _ := AsInteger(nv)
	fstIdx := int64(-1)
	if fst, found := m.Get("fst"); found {
		fm, _ := AsMap(fst)
		iv, _ := fm.Get("i")
		fstIdx, _ = AsInteger(iv)
	}
	return ok, n, fstIdx
}

func TestMatchHandlerAndOpts(t *testing.T) {
	out, err := matchHandler([]Value{NewString("l"), NewString("hello")}, nil, nil, nil)
	ok, n, fst := covMatchOut(t, out, err)
	if !ok || n != 1 || fst != 2 {
		t.Errorf("match first = (%v,%d,%d), want (true,1,2)", ok, n, fst)
	}

	cases := []struct {
		name           string
		pattern, input string
		opts           map[string]Value
		wantOK         bool
		wantN, wantFst int64
	}{
		{"scope all", "l", "hello", map[string]Value{"scope": NewString("all")}, true, 2, 2},
		{"ci", "L", "hello", map[string]Value{"cs": NewString("insensitive")}, true, 1, 2},
		{"ci all", "A", "banana", map[string]Value{"cs": NewString("insensitive"), "scope": NewString("all")}, true, 3, 1},
		{"shell all", "[0-9]", "a1b2", map[string]Value{"mode": NewString("shell"), "scope": NewString("all")}, true, 2, 1},
		{"shell first", "[0-9]", "a1b2", map[string]Value{"mode": NewString("shell")}, true, 1, 1},
		{"no match", "z", "hello", map[string]Value{}, false, 0, -1},
		{"norm", "é", "é", map[string]Value{"norm": NewString("nfc")}, true, 1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := matchOptsHandler(
				[]Value{NewString(c.pattern), NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			ok, n, fst := covMatchOut(t, out, err)
			if ok != c.wantOK || n != c.wantN || fst != c.wantFst {
				t.Errorf("match %s = (%v,%d,%d), want (%v,%d,%d)",
					c.name, ok, n, fst, c.wantOK, c.wantN, c.wantFst)
			}
		})
	}
}

// ---- escape ----

func TestEscapeHandlerAndOpts(t *testing.T) {
	out, err := escapeHandler([]Value{NewString("a b")}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != `a\ b` {
		t.Errorf("escape default = %q, want %q", got, `a\ b`)
	}

	cases := []struct {
		name, input string
		opts        map[string]Value
		want        string
	}{
		{"sh", "a$b", map[string]Value{"tgt": NewString("sh")}, `a\$b`},
		{"bash", "a b", map[string]Value{"tgt": NewString("bash")}, `a\ b`},
		{"sed", "a.b", map[string]Value{"tgt": NewString("sed")}, `a\.b`},
		{"awk", "a+b", map[string]Value{"tgt": NewString("awk")}, `a\+b`},
		{"grep", "a*b", map[string]Value{"tgt": NewString("grep")}, `a\*b`},
		{"single quote", "ab", map[string]Value{"quote": NewString("single")}, "'ab'"},
		{"double quote", "ab", map[string]Value{"quote": NewString("double")}, `"ab"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := escapeOptsHandler(
				[]Value{NewString(c.input), covOpts(c.opts)}, nil, nil, nil)
			if got := covStrOut(t, out, err); got != c.want {
				t.Errorf("escape %s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---- residual edge branches ----

func TestStringEdgeBranches(t *testing.T) {
	// foldIndex clamps a negative from offset.
	if idx, _ := foldIndex("abc", "a", -3); idx != 0 {
		t.Errorf("foldIndex negative from = %d, want 0", idx)
	}

	// Case-insensitive split with an empty separator splits per rune.
	out, err := splitOptsHandler([]Value{
		NewString(""), NewString("ab"),
		covOpts(map[string]Value{"cs": NewString("insensitive")}),
	}, nil, nil, nil)
	if got := covListOut(t, out, err); got != "a|b" {
		t.Errorf("ci empty-sep split = %q, want a|b", got)
	}

	// doReplace clamps a negative from offset.
	out, err = replaceOptsHandler([]Value{
		NewString("a"), NewString("_"), NewString("aa"),
		covOpts(map[string]Value{"scope": NewString("all"), "from": NewInteger(-2)}),
	}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "__" {
		t.Errorf("replace negative from = %q, want __", got)
	}

	// Case-insensitive first replace with no match returns the input.
	out, err = replaceOptsHandler([]Value{
		NewString("z"), NewString("_"), NewString("aa"),
		covOpts(map[string]Value{"cs": NewString("insensitive")}),
	}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "aa" {
		t.Errorf("ci replace miss = %q, want aa", got)
	}

	// Count-limited replace with no match copies the input through.
	out, err = replaceOptsHandler([]Value{
		NewString("z"), NewString("_"), NewString("aa"),
		covOpts(map[string]Value{"scope": NewString("all"), "count": NewInteger(3)}),
	}, nil, nil, nil)
	if got := covStrOut(t, out, err); got != "aa" {
		t.Errorf("counted replace miss = %q, want aa", got)
	}

	// Repeating an empty input is bounded and returns empty.
	out, err = doRepeat("", 3, strOpts{})
	if got := covStrOut(t, out, err); got != "" {
		t.Errorf("repeat empty input = %q, want empty", got)
	}

	// doPad defaults an unset fill to a space.
	out, err = doPad("ab", 4, strOpts{side: "right"})
	if got := covStrOut(t, out, err); got != "ab  " {
		t.Errorf("pad default fill = %q, want %q", got, "ab  ")
	}

	// find-all stops cleanly on a match that ends at the input's end.
	if got := findLiteralMatches("hell", "l", false, true); len(got) != 2 {
		t.Errorf("findLiteralMatches(hell,l,all) = %d matches, want 2", len(got))
	}
}

// An option key a word does not honour is an ERROR, not a silent no-op.
//
// One parser serves twelve words and each reads only its own subset, so a
// key outside that subset used to parse fine and then be dropped:
// `replace … {all:true}` returned the FIRST-occurrence result because the
// real key is `scope:"all"`. A misspelling was indistinguishable from the
// default, which is the worst possible failure mode for an option.
func TestStrOptsRejectUnknownKeys(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		word, key string
	}{
		{"replace", "all"},        // the original bug: real key is scope
		{"replace", "trunc"},      // real, but pad's — not replace's
		{"pad", "scope"},          // real, but replace's — not pad's
		{"concat", "cs"},          // real, but concat does not read it
		{"escape", "bogus_key_x"}, // outright unknown
	}
	for _, c := range cases {
		v := covOpts(map[string]Value{c.key: NewBoolean(true)})
		err := validateStrOpts(reg, v, c.word)
		if err == nil {
			t.Errorf("%s: option %q accepted, want rejection", c.word, c.key)
			continue
		}
		ae, ok := err.(*BoruError)
		if !ok || ae.Code != "string_option_error" {
			t.Errorf("%s/%s: got %T %v, want string_option_error", c.word, c.key, err, err)
		}
	}
}

// The positive half: a key the word DOES honour is accepted, and so is the
// no-options call. Without these the rejection above could be satisfied by
// refusing everything.
func TestStrOptsAcceptKnownKeys(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Each row carries a value legal for ITS OWN key. This used to pass
	// "all" for every one of them, which is meaningful only for `scope` —
	// the others were accepted because their keys had no declared domain,
	// so the row proved the key was honoured and nothing about the value.
	// NUR127 declared `quote`'s domain and the row failed, correctly: "all"
	// is not a quote style. A per-key value is what the test meant to say.
	for _, c := range []struct{ word, key, val string }{
		{"replace", "scope", "all"},
		{"pad", "trunc", "true"},
		{"concat", "sep", "-"},
		{"trim", "chars", " "},
		{"escape", "quote", "single"},
	} {
		v := covOpts(map[string]Value{c.key: NewString(c.val)})
		if err := validateStrOpts(reg, v, c.word); err != nil {
			t.Errorf("%s: option %q=%q rejected: %v", c.word, c.key, c.val, err)
		}
	}
	// A non-map (the no-options call) validates trivially.
	if err := validateStrOpts(reg, NewString("x"), "replace"); err != nil {
		t.Errorf("non-map options rejected: %v", err)
	}
}

// An enumerated key's value must be in its domain. `scope:"bogus"` was as
// silent as an unknown key — it simply was not "all", so it behaved as
// "first".
func TestStrOptsRejectBadEnumValues(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ word, key, val string }{
		{"replace", "scope", "bogus"},
		{"replace", "cs", "loose"},
		{"split", "mode", "regex"},
		{"pad", "side", "middle"},
		{"indexof", "occ", "third"},
		{"normalize", "eol", "cr"},
	} {
		v := covOpts(map[string]Value{c.key: NewString(c.val)})
		if err := validateStrOpts(reg, v, c.word); err == nil {
			t.Errorf("%s: %s=%q accepted, want rejection", c.word, c.key, c.val)
		}
	}
	// Every legal value of an enum is accepted.
	for _, val := range []string{"first", "all"} {
		v := covOpts(map[string]Value{"scope": NewString(val)})
		if err := validateStrOpts(reg, v, "replace"); err != nil {
			t.Errorf("scope=%q rejected: %v", val, err)
		}
	}
}

// A word with no registered key set is a caller bug, not user input, and
// is reported rather than silently accepting anything.
func TestStrOptsUnregisteredWord(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	v := covOpts(map[string]Value{"sep": NewString("-")})
	if err := validateStrOpts(reg, v, "not-a-string-word"); err == nil {
		t.Error("an unregistered word accepted options; want an error")
	}
}
