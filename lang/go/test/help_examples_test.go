package test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/native/help"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestHandAuthoredExamplesWin verifies §5.2/§9.5: when a word's help
// Entry carries hand-authored Examples, `describe` renders those instead
// of the auto-generated positional permutations.
func TestHandAuthoredExamplesWin(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	cases := []struct {
		word     string
		wantLine string // a hand-authored example substring that must appear
		notWant  string // an auto-generated permutation that must NOT appear
	}{
		{"set", `c 1 "count" set`, `set 'a'`},
		{"get", `[10 20 30] 0 get`, ``},
		{"make", `make Point {x:3 y:4}`, ``},
		{"each", `[1 2 3] each [dup mul]`, ``},
		{"fold", `0 fold [add] [1 2 3 4]`, ``},
	}
	for _, c := range cases {
		info := native.BuildFuncInfo(reg, c.word)
		if info == nil {
			t.Errorf("%s: no FuncInfo", c.word)
			continue
		}
		out := help.FormatDynamic(*info)
		if !strings.Contains(out, c.wantLine) {
			t.Errorf("describe %s missing hand-authored example %q:\n%s", c.word, c.wantLine, out)
		}
		if c.notWant != "" && strings.Contains(out, c.notWant) {
			t.Errorf("describe %s still shows auto-generated permutation %q (hand-authored should win):\n%s", c.word, c.notWant, out)
		}
	}
}

// TestHelpAllWords checks that every registered word produces valid
// dynamic help output with the expected sections.
func TestHelpAllWords(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)

	words := allRegisteredWords(reg)
	if len(words) == 0 {
		t.Fatal("no words registered")
	}

	for _, word := range words {
		t.Run(word, func(t *testing.T) {
			info := native.BuildFuncInfo(reg, word)
			if info == nil {
				t.Skipf("no func info for %q (simple def)", word)
				return
			}
			helpText := help.FormatDynamic(*info)

			// Must contain the word name and " — "
			if !strings.Contains(helpText, word+" — ") {
				t.Errorf("missing header for %q", word)
			}

			// Must contain Precedence section
			if !strings.Contains(helpText, "Precedence:") {
				t.Errorf("missing Precedence section for %q", word)
			}

			// Must contain Signatures section
			if !strings.Contains(helpText, "Signatures:") {
				t.Errorf("missing Signatures section for %q", word)
			}

			// Must have at least one signature line with [ ... ]
			if !strings.Contains(helpText, "[ [") && !strings.Contains(helpText, "[ ]") {
				// 0-arg words have [ ] without inner brackets
				if !strings.Contains(helpText, "(none)") {
					t.Errorf("missing signature entries for %q", word)
				}
			}

			// Must contain Description section
			if !strings.Contains(helpText, "Description:") {
				t.Errorf("missing Description section for %q", word)
			}
		})
	}
}

// TestHelpExamplesCorrect extracts dynamically generated examples from
// help output, runs each expression through the engine, and verifies
// the result matches the documented output. Uses in-memory filesystem
// for read/write validation.
func TestHelpExamplesCorrect(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.SetParseFunc(parser.Parse)

	// Enable in-memory filesystem for read/write examples.
	mem := capabilities.NewMem()
	if err := reg.Capabilities.Set(native.CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}

	// Seed in-memory files that the generated read examples will access.
	// The help system generates single-letter filenames ('a', 'b', etc.)
	// These are plain text files so read returns their content as a string.
	mem.Files["a"] = []byte("file-a-content")
	mem.Files["b"] = []byte("file-b-content")
	mem.Files["c"] = []byte("file-c-content")
	mem.Files["d"] = []byte("file-d-content")
	mem.Files["e"] = []byte("file-e-content")

	// Set __sys.fs.mem = true in the root context so EffectiveFileOps
	// returns the in-memory filesystem.
	enableMemFS(t, reg)

	// Words with non-deterministic output.
	skipWords := map[string]bool{"module": true}

	words := allRegisteredWords(reg)
	testedCount := 0
	// Every placeholder site actually observed this run. Compared against
	// placeholderSites afterwards so the ratchet cannot go stale: an entry
	// whose example started evaluating must be REMOVED, or the map slowly
	// becomes a list of things that used to be broken.
	seenPlaceholders := map[string]bool{}

	for _, word := range words {
		if skipWords[word] {
			continue
		}
		// Hand-authored examples (Entry.Examples) are curated prose +
		// results, not the `;# <exact-stack-render>` shape this matcher
		// validates — and some are side-effecting or multi-statement.
		// They replace the auto-generated permutations in `describe`, so
		// there is nothing auto-generated left to check here. See
		// §5.2/§9.5 and help.Entry.Examples.
		if e := help.Lookup(word); e != nil && len(e.Examples) > 0 {
			continue
		}
		info := native.BuildFuncInfo(reg, word)
		if info == nil {
			continue
		}
		helpText := help.FormatDynamic(*info)
		examples := extractExamples(helpText)

		// Every rendered example is now checkable: a concrete stack
		// render, `(no value)` for an empty stack, or
		// `error [boru/CODE]` for one the engine declines. Only the
		// placeholder is not, and placeholderWords is the tracked,
		// shrink-only record of where one still ships.
		var runnable []helpExample
		for _, ex := range examples {
			if ex.expected == examplePlaceholder {
				site := word + " | " + ex.expr
				seenPlaceholders[site] = true
				if !placeholderSites[site] {
					t.Errorf("%s: example %q renders the %q placeholder; "+
						"`describe` output is documented as engine-derived, so either "+
						"make it evaluate or add %q to placeholderSites with a reason",
						word, ex.expr, examplePlaceholder, site)
				}
				continue
			}
			runnable = append(runnable, ex)
		}
		if len(runnable) == 0 {
			continue
		}

		t.Run(word, func(t *testing.T) {
			for _, ex := range runnable {
				t.Run(ex.expr, func(t *testing.T) {
					vals, err := parser.Parse(ex.expr)
					if err != nil {
						t.Fatalf("parse %q: %v", ex.expr, err)
					}
					// Use a fresh engine per example to avoid state leaks
					eng := native.NewTop(reg)
					result, err := eng.Run(vals)

					// An `error [boru/CODE]` render is a claim about a
					// compile failure: the run must fail, and with that code.
					if code, isErr := strings.CutPrefix(ex.expected, "error [boru/"); isErr {
						code = strings.TrimSuffix(code, "]")
						if err == nil {
							t.Fatalf("%s ;# documented as error [boru/%s], but it succeeded with %q",
								ex.expr, code, formatStack(result))
						}
						var be *core.BoruError
						if !errors.As(err, &be) {
							t.Fatalf("%s ;# documented as error [boru/%s], but the failure carries no code: %v",
								ex.expr, code, err)
						}
						if be.Code != code {
							t.Errorf("%s ;# documented as error [boru/%s], got [boru/%s]",
								ex.expr, code, be.Code)
						}
						testedCount++
						return
					}

					if err != nil {
						t.Fatalf("run %q: %v", ex.expr, err)
					}
					got := formatStack(result)
					// An empty stack renders as the marker, not as "".
					if got == "" {
						got = noValueMarker
					}
					if got != ex.expected {
						t.Errorf("%s ;# got %q, want %q", ex.expr, got, ex.expected)
					}
					testedCount++
				})
			}
		})
	}

	// NO staleness assertion here, deliberately. `help`'s dynamic example
	// map is process-global and is populated as a side effect of any test
	// that imports a module, so the placeholder set depends on what else
	// ran first in this package: measured alone, the module words are
	// undefined and render `...`; measured after another test imported
	// `boru:string-util`, roughly half of them evaluate. An
	// "entry no longer needed" check over that set therefore fails or
	// passes by test order, which is worse than not having it —
	// verified, not assumed: asserting it made `make test` fail while
	// `go test -run TestHelpExamplesCorrect` passed.
	//
	// The direction that IS sound is kept above: placeholderSites is the
	// set measured in the sparsest state, so a fuller state can only
	// produce FEWER placeholders, never a site outside it. New
	// placeholders are what the ratchet exists to catch.
	//
	// Making this two-directional needs the example state injectable
	// rather than package-global; that is its own change.
	_ = seenPlaceholders

	t.Logf("validated %d examples across all words (%d placeholder sites tracked)",
		testedCount, len(placeholderSites))
}

// enableMemFS sets __sys.fs.mem = true in the registry's root context.
func enableMemFS(t *testing.T, reg *native.Registry) {
	t.Helper()
	eng := native.New(reg)
	_, err := eng.Run([]native.Value{
		native.NewWord("context"), native.NewWord("dot"), native.NewWord("__sys"),
		native.NewWord("dot"), native.NewWord("fs"),
		native.NewWord("set"), native.NewWord("mem"), native.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("failed to enable mem fs: %v", err)
	}
}

// allRegisteredWords returns a sorted list of all words in the registry
// that have function definitions (not just simple defs).
func allRegisteredWords(reg *native.Registry) []string {
	// Collect words that have help entries
	helpWords := help.Words()

	// Also collect words from the registry's function table
	// by trying BuildFuncInfo on help words
	seen := map[string]bool{}
	var words []string
	for _, w := range helpWords {
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	sort.Strings(words)
	return words
}

type helpExample struct {
	expr     string
	expected string
}

// extractExamples parses the Examples section of help output, extracting
// each "expr ;# result" line.
func extractExamples(helpText string) []helpExample {
	var examples []helpExample
	inExamples := false
	for _, line := range strings.Split(helpText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Examples:" {
			inExamples = true
			continue
		}
		if inExamples {
			if trimmed == "" || (!strings.HasPrefix(trimmed, "") && strings.HasSuffix(trimmed, ":")) {
				break // End of examples section
			}
			if idx := strings.Index(trimmed, ";#"); idx >= 0 {
				expr := strings.TrimSpace(trimmed[:idx])
				expected := strings.TrimSpace(trimmed[idx+2:])
				// Strip trailing NOTE annotation if present
				if ni := strings.Index(expected, "  NOTE:"); ni >= 0 {
					expected = strings.TrimSpace(expected[:ni])
				}
				if expr != "" && expected != "" {
					examples = append(examples, helpExample{expr: expr, expected: expected})
				}
			}
		}
	}
	return examples
}

// examplePlaceholder is what the Go-side fallback in
// lang/go/native/help renders when the generated map has no entry for an
// expression. It is the one example form this file cannot verify, which
// is why its remaining sites are tracked rather than tolerated.
const examplePlaceholder = "..."

// noValueMarker mirrors cmd/go/genhelp's marker for an example that
// leaves the stack empty.
const noValueMarker = "(no value)"

// placeholderSites records every (word, expression) that still renders
// the `...` placeholder, keyed "word | expr". A NEW placeholder — any
// site not listed here — fails the test. The reverse check (an entry
// that no longer needs to be here) is NOT asserted, because the set
// depends on process-global state other tests mutate; see the note at
// the end of TestHelpExamplesCorrect.
//
// Keying by site rather than by word is deliberate: a word-keyed
// allowlist accepts every future placeholder under an already-listed
// word, which is how a ratchet turns into a skip list.
//
// Nearly every entry has ONE cause, measured 2026-08-18: cmd/go/genhelp
// evaluates each example on a `native.DefaultRegistry` with no imports,
// so a word implemented in a loadable module (`boru:string-util`,
// `boru:bin-util`, `boru:logic-util`, `boru:type-util`, `boru:debug`)
// raises `undefined_word` there. That code is deliberately not
// documented — the word exists, it needs an import — so these fall
// through to the Go-side fallback. Teaching the generator to load the
// modules before evaluating empties them; see
// design/ROC-ADOPTION-PLAN.0.md (A4).
//
// `roll | 2 roll` is the one exception, with a different cause: the
// generator records no result for it at all, so the display expression
// and the generated expression diverge somewhere between
// help.ExampleExprs and writeExamples.
var placeholderSites = map[string]bool{
	"band | band 2 3":                         true,
	"bnot | bnot 2":                           true,
	"bor | bor 2 3":                           true,
	"bsl | bsl 2 3":                           true,
	"bsr | bsr 2 3":                           true,
	"busr | busr 2 3":                         true,
	"bxor | bxor 2 3":                         true,
	"changecase | changecase 'a' {a:1,b:2}":   true,
	"changecase | changecase 'b'":             true,
	"changecase | changecase a/q {c:3,d:4}":   true,
	"changecase | changecase b/q":             true,
	"concat | concat ['c','d']":               true,
	"concat | concat {a:1,b:2} ['a','b']":     true,
	"contains | contains 'a' 'b' {a:1,b:2}":   true,
	"contains | contains 'c' 'd'":             true,
	"create | create {c:3,d:4} ['a','b']":     true,
	"escape | escape 'a' {a:1,b:2}":           true,
	"escape | escape 'b'":                     true,
	"fnsig | fnsig ['a','b']":                 true,
	"forward-args | forward-args a/q":         true,
	"iff | iff 2 3":                           true,
	"iff | iff true false":                    true,
	"implies | 3 implies 2":                   true,
	"implies | false implies true":            true,
	"implies | implies 2 3":                   true,
	"implies | implies true false":            true,
	"indexof | indexof 'a' 'b' {a:1,b:2}":     true,
	"indexof | indexof 'c' 'd'":               true,
	"lower | lower 'a'":                       true,
	"lower | lower a/q":                       true,
	"match | match 'a' 'b' {a:1,b:2}":         true,
	"match | match 'c' 'd'":                   true,
	"nand | nand 2 3":                         true,
	"nand | nand true false":                  true,
	"nor | nor 2 3":                           true,
	"nor | nor true false":                    true,
	"normalize | normalize 'a' {a:1,b:2}":     true,
	"normalize | normalize 'b'":               true,
	"pad | pad 2 {a:1,b:2} 'a'":               true,
	"pad | pad 3 'b'":                         true,
	"pad | pad 3":                             true,
	"pad | pad 4 2":                           true,
	"pick | 2 pick":                           true,
	"printstr | printstr 2":                   true,
	"ref | ref a/q":                           true,
	"remove | remove {c:3,d:4} ['a','b']":     true,
	"repeat | repeat 2 'a' {a:1,b:2}":         true,
	"repeat | repeat 3 'b'":                   true,
	"replace | replace 'a' 'b' 'c' {a:1,b:2}": true,
	"replace | replace 'd' 'e' 'f'":           true,
	"roll | 2 roll":                           true,
	"split | split 'a' 'b' {a:1,b:2}":         true,
	"split | split 'c' 'd'":                   true,
	"stack | 2 stack":                         true,
	"stack-args | stack-args a/q":             true,
	"tpartial | tpartial 2":                   true,
	"trace | trace ['a','b']":                 true,
	"trim | trim 'a' {a:1,b:2}":               true,
	"trim | trim 'b'":                         true,
	"trim | trim a/q {c:3,d:4}":               true,
	"trim | trim b/q":                         true,
	"update | update {c:3,d:4} ['a','b']":     true,
	"upper | upper 'a'":                       true,
	"upper | upper a/q":                       true,
	"xnor | xnor 2 3":                         true,
	"xnor | xnor true false":                  true,
}
