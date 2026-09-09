package native

import (
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// The string words' options map is a CONTRACT, and this file exercises it
// through the words themselves rather than through validateStrOpts.
//
// That distinction is the whole point. `native_string_cov_test.go` already
// calls validateStrOpts directly, which proves the validator's own logic —
// but it says nothing about whether a word ever CONSULTS it. Every opts
// handler carries its own `if err := validateStrOpts(…); err != nil` arm,
// and a word that dropped that arm would silently go back to the failure
// mode the validator was written to end: an unknown key parsed fine and was
// then thrown away, so `replace … {all:true}` returned the first-occurrence
// result and a typo was indistinguishable from the default.
//
// So the tests below run real source. If any one of the ten words stops
// calling validateStrOpts, its row in TestStringOptionUnknownKeyIsRefused
// turns from "refused" into "returned the default answer" and fails.
//
// (`replace` and `changecase` are deliberately absent: they carry a
// DryPassReturns model, so their rejection is already pinned at CHECK time
// by the ERROR rows in lang/spec/edge-dispatch-3.tsv. The ten words here
// have no dry pass — refusal is a runtime property, which is exactly what
// these rows assert.)

// soptRun parses and runs source against a registry with the
// boru:string-util words seeded under their bare names, returning the
// canonical render of the final stack.
func soptRun(t *testing.T, src string) (string, error) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	toks, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	out, rerr := NewTop(r).Run(toks)
	if rerr != nil {
		return "", rerr
	}
	return Canon(out), nil
}

// soptHintOptions splits a string_option_error's "known options: a, b, c"
// hint back into names. Compared as a set rather than by substring so a
// name that happens to be a prefix of another cannot be mistaken for a
// match.
func soptHintOptions(hint string) []string {
	rest, ok := strings.CutPrefix(hint, "known options: ")
	if !ok {
		return nil
	}
	return strings.Split(rest, ", ")
}

// soptBoruError narrows an error to a *BoruError with the expected code,
// failing the test (rather than panicking on a bad assertion) otherwise.
func soptBoruError(t *testing.T, label string, err error, code string) *BoruError {
	t.Helper()
	if err == nil {
		t.Errorf("%s: accepted, want %s", label, code)
		return nil
	}
	be, ok := err.(*BoruError)
	if !ok {
		t.Errorf("%s: got %T (%v), want *BoruError", label, err, err)
		return nil
	}
	if be.Code != code {
		t.Errorf("%s: code = %q, want %q (%v)", label, be.Code, code, err)
		return nil
	}
	return be
}

// soptWords is the option-bearing string words that validate at RUN time,
// one row each. `dflt` is the same call without an options map; `opted`
// adds a legal option that must visibly change the answer.
//
// The pair is the point: `want` alone would pass for a word that parsed the
// map and ignored it, which is the bug the key sets exist to prevent, so
// every row asserts that the two answers DIFFER.
var soptWords = []struct {
	word      string // the word under test
	dflt      string // source: the no-options call
	dfltWant  string // its canonical result
	opted     string // source: the same call plus one legal option
	optedWant string // its canonical result — must differ from dfltWant
}{
	// concat is the odd one out in shape: its options map is the FIRST
	// argument (sig [Map List]), not a trailing one.
	{"concat", `concat ["a" "b"]`, `'ab'`,
		`concat {sep:"-"} ["a" "b"]`, `'a-b'`},
	{"split", `split "," " a , b "`, `[' a ' ' b ']`,
		`split "," " a , b " {trimParts:true}`, `['a' 'b']`},
	{"trim", `trim " x "`, `'x'`,
		`trim " x " {side:"left"}`, `'x '`},
	{"contains", `contains "A" "abc"`, `false`,
		`contains "A" "abc" {cs:"insensitive"}`, `true`},
	{"indexof", `indexof "b" "abcb"`, `1`,
		`indexof "b" "abcb" {occ:"last"}`, `3`},
	{"normalize", `normalize "a  b"`, `'a  b'`,
		`normalize "a  b" {collapseWs:true}`, `'a b'`},
	{"repeat", `repeat 2 "ab"`, `'abab'`,
		`repeat 2 "ab" {sep:"-"}`, `'ab-ab'`},
	// pad's options sit BETWEEN the width and the subject (sig
	// [Integer Map String]) — another positional shape, same contract.
	{"pad", `pad 5 "ab"`, `'ab   '`,
		`pad 5 {side:"left" fill:"0"} "ab"`, `'000ab'`},
	{"match", `match "b" "abcb"`,
		`{ok:true ms:[{m:'b' i:1 e:2}] fst:{m:'b' i:1 e:2} lst:{m:'b' i:1 e:2} n:1}`,
		`match "b" "abcb" {scope:"all"}`,
		`{ok:true ms:[{m:'b' i:1 e:2} {m:'b' i:3 e:4}] fst:{m:'b' i:1 e:2} lst:{m:'b' i:3 e:4} n:2}`},
	{"escape", `escape "a b"`, `'a\\ b'`,
		`escape "a b" {quote:"single"}`, `'\'a\\ b\''`},
}

// The positive half: a legal option is accepted AND honoured.
//
// Without the honoured half this suite could be satisfied by a word that
// validates the map and then discards it — which is precisely the shape of
// the original defect, only moved one step later.
func TestStringOptionMapIsHonoured(t *testing.T) {
	for _, c := range soptWords {
		got, err := soptRun(t, c.dflt)
		if err != nil {
			t.Errorf("%s: no-options call %q errored: %v", c.word, c.dflt, err)
		} else if got != c.dfltWant {
			t.Errorf("%s: %q = %s, want %s", c.word, c.dflt, got, c.dfltWant)
		}

		got, err = soptRun(t, c.opted)
		if err != nil {
			t.Errorf("%s: legal option rejected in %q: %v", c.word, c.opted, err)
			continue
		}
		if got != c.optedWant {
			t.Errorf("%s: %q = %s, want %s", c.word, c.opted, got, c.optedWant)
		}
		if c.optedWant == c.dfltWant {
			t.Errorf("%s: the option in %q does not change the answer, so this row "+
				"cannot tell an honoured option from an ignored one", c.word, c.opted)
		}
	}
}

// The negative half, and the one that matters most: a key the word does not
// honour is REFUSED, naming the word and the key, with the word's own
// option list as the hint.
//
// Each `unknown` below is a key that exists for some OTHER string word (or
// is outright invented), so a word that accepted the union of all keys —
// or that skipped validation entirely — would return a plausible-looking
// answer here instead of an error. `lacks` pins that the hint is per-word:
// a hint listing the union would tell the reader to write a key this word
// still rejects.
func TestStringOptionUnknownKeyIsRefused(t *testing.T) {
	cases := []struct {
		word    string
		src     string // the call, with one unhonoured key in the options map
		unknown string // the key that must be named in the message
		has     string // an option this word DOES honour, named by the hint
		lacks   string // a real option of another word, absent from the hint
	}{
		{"concat", `concat {scope:"all"} ["a" "b"]`, "scope", "sep", "scope"},
		{"split", `split "," "a,b" {trunc:true}`, "trunc", "keepEmpty", "trunc"},
		{"trim", `trim " x " {scope:"all"}`, "scope", "side", "scope"},
		{"contains", `contains "a" "abc" {sep:"-"}`, "sep", "wholeWord", "sep"},
		{"indexof", `indexof "b" "abc" {sep:"-"}`, "sep", "occ", "sep"},
		{"normalize", `normalize "a b" {cs:"insensitive"}`, "cs", "collapseWs", "cs"},
		{"repeat", `repeat 2 "ab" {cs:"insensitive"}`, "cs", "sep", "cs"},
		{"pad", `pad 5 {sep:"-"} "ab"`, "sep", "trunc", "sep"},
		{"match", `match "b" "abc" {sep:"-"}`, "sep", "scope", "sep"},
		// The one invented key, to show the refusal is not a
		// borrowed-from-another-word special case.
		{"escape", `escape "a b" {bogus:true}`, "bogus", "tgt", "sep"},
	}
	for _, c := range cases {
		got, err := soptRun(t, c.src)
		if err == nil {
			t.Errorf("%s: unknown option %q accepted, returned %s — an unhonoured key "+
				"must not read as the default", c.word, c.unknown, got)
			continue
		}
		be := soptBoruError(t, c.word+"/"+c.unknown, err, "string_option_error")
		if be == nil {
			continue
		}
		// The message has to name BOTH the word and the offending key: a
		// bare "unknown option" leaves the reader hunting through a call
		// with several maps in it.
		if !strings.Contains(be.Detail, c.word+":") {
			t.Errorf("%s: detail %q does not name the word", c.word, be.Detail)
		}
		if !strings.Contains(be.Detail, `"`+c.unknown+`"`) {
			t.Errorf("%s: detail %q does not name the rejected key %q",
				c.word, be.Detail, c.unknown)
		}
		// …and the hint has to say what WOULD have been accepted, or the
		// error is a dead end for anyone who mistyped.
		opts := soptHintOptions(be.Hint)
		if len(opts) == 0 {
			t.Errorf("%s: hint %q carries no known-options list", c.word, be.Hint)
			continue
		}
		if !contains(opts, c.has) {
			t.Errorf("%s: hint %q omits %q, an option the word honours",
				c.word, be.Hint, c.has)
		}
		if contains(opts, c.lacks) {
			t.Errorf("%s: hint %q offers %q, which this word rejects",
				c.word, be.Hint, c.lacks)
		}
	}
}

// The second negative: a legal key carrying a value outside its enumerated
// domain is refused too, with the domain as the hint.
//
// This is the quieter half of the same defect. `scope:"bogus"` was never
// an unknown KEY, so the key set alone would let it through — it simply
// was not "all", and behaved as "first". A caller who mistyped a value got
// a real answer to a question they did not ask.
func TestStringOptionValueOutsideDomainIsRefused(t *testing.T) {
	cases := []struct {
		word string
		key  string
		src  string // the call, with an out-of-domain value for `key`
	}{
		{"split", "cs", `split "," "a,b" {cs:"nope"}`},
		{"split", "mode", `split "," "a,b" {mode:"regex"}`},
		{"trim", "cs", `trim " x " {cs:"nope"}`},
		{"trim", "side", `trim " x " {side:"sideways"}`},
		{"contains", "cs", `contains "a" "abc" {cs:"nope"}`},
		{"contains", "mode", `contains "a" "abc" {mode:"regex"}`},
		{"indexof", "cs", `indexof "b" "abc" {cs:"nope"}`},
		{"indexof", "mode", `indexof "b" "abc" {mode:"regex"}`},
		{"indexof", "occ", `indexof "b" "abc" {occ:"middle"}`},
		{"normalize", "eol", `normalize "a b" {eol:"cr"}`},
		{"pad", "side", `pad 5 {side:"middle"} "ab"`},
		{"match", "cs", `match "b" "abc" {cs:"nope"}`},
		{"match", "mode", `match "b" "abc" {mode:"regex"}`},
		{"match", "scope", `match "b" "abc" {scope:"some"}`},
	}
	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.word+"."+c.key] = true
		got, err := soptRun(t, c.src)
		if err == nil {
			t.Errorf("%s: %s out of domain accepted, returned %s — a mistyped value "+
				"must not silently take the default", c.word, c.key, got)
			continue
		}
		be := soptBoruError(t, c.word+"/"+c.key, err, "string_option_error")
		if be == nil {
			continue
		}
		if !strings.Contains(be.Detail, c.word+":") ||
			!strings.Contains(be.Detail, `"`+c.key+`"`) {
			t.Errorf("%s: detail %q names neither the word nor the key %q",
				c.word, be.Detail, c.key)
		}
		// The hint must spell the domain out; "must be one of:" with
		// nothing after it is no better than the silent default was.
		want := `"` + c.key + `" must be one of: ` + strings.Join(strOptEnums[c.key], ", ")
		if be.Hint != want {
			t.Errorf("%s/%s: hint = %q, want %q", c.word, c.key, be.Hint, want)
		}
	}

	// A table of hand-written rows goes stale the moment a domain is added
	// to an existing key, and a stale row is invisible — the missing case
	// simply is not tested. So derive the full (word, key) set the runtime
	// can enforce and require the table to cover it.
	//
	// concat, repeat and escape appear nowhere above on purpose: none of
	// their keys (sep / skipEmpty / skipNullish / quote / tgt) has an
	// enumerated domain, so there is no out-of-domain value to refuse.
	// Give `tgt` one and this loop fails until a row is written for it.
	for word, allowed := range strOptKeys {
		if word == "replace" || word == "changecase" {
			continue // validated at check time; pinned in edge-dispatch-3.tsv
		}
		for key := range allowed {
			if _, isEnum := strOptEnums[key]; !isEnum {
				continue
			}
			if !covered[word+"."+key] {
				t.Errorf("%s: option %q has an enumerated domain %v but no row here "+
					"pins that an out-of-domain value is refused",
					word, key, strOptEnums[key])
			}
		}
	}
}

// The third negative: nothing SNIFFS an options map out of the argument
// list. The options slot is an ordinary typed position, so a non-map
// standing in it is not options at all — it never reaches validateStrOpts,
// and no string_option_error can come of it.
//
// This is what keeps the two halves above honest. If the words started
// scanning their arguments for a map instead of dispatching on the
// signature, `split "," "a,b" 42` would begin failing with
// string_option_error and the well-formed no-options calls of
// TestStringOptionMapIsHonoured would still pass.
func TestStringOptionSlotIsPositional(t *testing.T) {
	cases := []struct {
		word    string
		src     string
		want    string // canonical stack, when the call succeeds
		wantErr string // error code instead, when it does not
	}{
		// Eight words take options LAST. The shorter signature matches and
		// the stray value is simply left on the stack, untouched.
		{word: "split", src: `split "," " a , b " 42`, want: `[' a ' ' b '] 42`},
		{word: "trim", src: `trim " x " 42`, want: `'x' 42`},
		{word: "contains", src: `contains "A" "abc" 42`, want: `false 42`},
		{word: "indexof", src: `indexof "b" "abcb" 42`, want: `1 42`},
		{word: "normalize", src: `normalize "a  b" 42`, want: `'a  b' 42`},
		{word: "repeat", src: `repeat 2 "ab" 42`, want: `'abab' 42`},
		{word: "match", src: `match "b" "abcb" 42`,
			want: `{ok:true ms:[{m:'b' i:1 e:2}] fst:{m:'b' i:1 e:2} lst:{m:'b' i:1 e:2} n:1} 42`},
		{word: "escape", src: `escape "a b" 42`, want: `'a\\ b' 42`},
		// concat's map is its FIRST argument, so a non-map there leaves no
		// signature to match — the call is refused, but for dispatch, not
		// for options.
		{word: "concat", src: `concat 42 ["a" "b"]`, wantErr: "signature_error"},
		// pad's map sits between the width and the subject, so a non-map
		// there is read as the SUBJECT ([Integer Any]) and "ab" is left
		// over — the same rule as `pad 4 2` in lang/spec/corpus-modules.tsv.
		{word: "pad", src: `pad 5 42 "ab"`, want: `'42   ' 'ab'`},
	}
	for _, c := range cases {
		got, err := soptRun(t, c.src)
		if c.wantErr != "" {
			soptBoruError(t, c.word, err, c.wantErr)
			continue
		}
		if err != nil {
			// The specific failure this guards against: a non-map treated
			// as an options map with unreadable keys.
			if be, ok := err.(*BoruError); ok && be.Code == "string_option_error" {
				t.Errorf("%s: %q raised string_option_error (%v) — a non-map in the "+
					"options slot is not options", c.word, c.src, err)
				continue
			}
			t.Errorf("%s: %q errored: %v", c.word, c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: %q = %s, want %s", c.word, c.src, got, c.want)
		}
	}
}
