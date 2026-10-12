package check_test

// The check module's OWN .tsv corpus runner.
//
// check sits at 51.5% under `make cover-gate-check` while reading 100%
// under the merged ADR-008 gate, and the gap is entirely about WHERE the
// driving tests live: the spec corpora that exercise the checker end to
// end run from test/go, one layer above, so none of their coverage
// counts for check's own gate. This runner brings that shape of test
// INSIDE check.
//
// Three things had to be rebuilt rather than reused, each for a
// structural reason:
//
//   - The TOKENISER below is local, and that is not a shortcut. check's
//     module rule is that its go.mod names no boru sibling but core
//     (check/go/CLAUDE.md, "No upward imports"), so pulling parser/go in
//     even as a test-only require would break the rule the module is
//     built on. A corpus needs source text turned into []core.Value; the
//     subset a check row needs — integers, strings, words, quotes — is a
//     few lines, so the corpus tokenises its own rows and check's go.mod
//     stays exactly as its guide says it must be.
//   - The FIXTURE WORDS are declared here (spec_fixtures_test.go) rather
//     than taken from test/specfix: specfix REQUIRES check, so importing
//     it would close a cycle. The set is deliberately tiny — just the
//     shapes check's analysis needs — not a copy of specfix's corpus-wide
//     vocabulary.
//   - The RENDERER is reimplemented below for the same reason, and is
//     kept byte-identical in format to specfix.RenderCheck so a row moved
//     between the two corpora means the same thing.
//
// No driver had to be invented: check installs itself into
// core.AnalysisImpl at init (check_recovery.go installAnalysisImpl), so
// running a program on a core Registry inside r.Check.Begin() drives the
// checker exactly as the language layer drives it.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// tokenise turns a corpus row's input into []core.Value. It covers the
// row subset only: decimal integers, booleans, double-quoted strings with no
// escapes, bracketed QUOTE LITERALS, and bare words (including the
// n:Type param-spec form defq's triples use). Anything else is a
// row-authoring error and is reported as one rather than silently
// becoming a word.
//
// `[ … ]` builds a nested core list VALUE, matching what the real parser
// produces for a boru quote: a deferred literal whose elements are data
// until something invokes them. It must NOT become OpenParen/CloseParen
// tokens — those are the GROUPING construct, whose contents the engine
// (and the check pass) evaluate in place, which is a different program.
// The first cut of this tokeniser made exactly that mistake, and the
// fn-def triples handed to defq were carrier-checked as loose code.
func tokenise(src string) ([]core.Value, error) {
	vals, rest, err := tokeniseSeq(strings.Fields(src), false)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("unbalanced ]: %d token(s) left after the outermost list closed", len(rest))
	}
	return vals, nil
}

// tokeniseSeq consumes tokens until end-of-input (nested=false) or a
// closing bracket (nested=true), returning the values and the unconsumed
// remainder.
func tokeniseSeq(toks []string, nested bool) ([]core.Value, []string, error) {
	var out []core.Value
	for len(toks) > 0 {
		tok := toks[0]
		toks = toks[1:]
		switch {
		case tok == "[":
			inner, rest, err := tokeniseSeq(toks, true)
			if err != nil {
				return nil, nil, err
			}
			toks = rest
			out = append(out, core.NewList(inner))
		case tok == "]":
			if !nested {
				return nil, nil, fmt.Errorf("unbalanced ] with no open list")
			}
			return out, toks, nil
		case tok == "true", tok == "false":
			out = append(out, core.NewBoolean(tok == "true"))
		case strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`) && len(tok) >= 2:
			out = append(out, core.NewString(tok[1:len(tok)-1]))
		default:
			if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
				out = append(out, core.NewInteger(n))
				continue
			}
			if !wordRe.MatchString(tok) {
				return nil, nil, fmt.Errorf("token %q is neither an integer, a string, a bracket nor a word", tok)
			}
			out = append(out, core.NewWord(tok))
		}
	}
	if nested {
		return nil, nil, fmt.Errorf("unclosed [ at end of input")
	}
	return out, toks, nil
}

// wordRe is deliberately strict: a corpus row that fat-fingers a literal
// should fail loudly, not dispatch an accidental word.
var wordRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]*$`)

// renderCheck renders a check result — the residual carrier stack plus the
// collected diagnostics — to one comparable string, in the format
// specfix.RenderCheck uses:
//
//	<stack> :: <diag> <diag> …
//
// A dynamic carrier renders as `dynamic(Leaf)`, a strict one as `Leaf`;
// each diagnostic is a severity sigil (`!` error, `~` warning, `?` info)
// followed by its code, in emission order. The ` :: …` suffix is omitted
// when there are no diagnostics.
func renderCheck(stack []core.Value, diags []core.CheckDiagnostic) string {
	parts := make([]string, len(stack))
	for i, v := range stack {
		if core.IsDynamic(&v) {
			parts[i] = "dynamic(" + v.Parent.Leaf() + ")"
		} else {
			parts[i] = v.Parent.Leaf()
		}
	}
	s := strings.Join(parts, " ")
	if len(diags) == 0 {
		return s
	}
	ds := make([]string, len(diags))
	for i, d := range diags {
		sigil := "?"
		switch d.Severity {
		case core.SeverityError:
			sigil = "!"
		case core.SeverityWarning:
			sigil = "~"
		}
		ds[i] = sigil + d.Code
	}
	return strings.TrimSpace(s + " :: " + strings.Join(ds, " "))
}

// checkRow parses one row's input and runs it through a check pass on a
// registry carrying only the local fixture words.
func checkRow(input string) (string, error) {
	values, err := tokenise(input)
	if err != nil {
		return "", err
	}
	r, err := core.NewRegistry()
	if err != nil {
		return "", err
	}
	registerCheckFixtures(r)
	registerControlFixtures(r)
	registerDryFixtures(r)
	registerIndexFixtures(r)
	registerMopupFixtures(r)
	r.InitRootContext()
	r.Source = input

	done := r.Check.Begin()
	out, runErr := core.NewTop(r).Run(values)
	r.RescueForwardRefDiagnostics()
	r.Check.EmitUnusedDefDiagnostics()
	diags := r.Check.Diagnostics
	done()

	if runErr != nil {
		return "", runErr
	}
	return renderCheck(out, diags), nil
}

// TestCheckSpec runs every check/go/spec/*.tsv row through the checker and
// compares the rendered result to the row's expected column. Row format is
// the corpus format used repo-wide:
//
//	<input><TAB><expected>[<TAB><note>]
//
// Blank lines and lines opening with `#` are skipped.
func TestCheckSpec(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("spec", "*.tsv"))
	if err != nil {
		t.Fatalf("glob spec: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no spec/*.tsv files found — the corpus is the point of this test")
	}
	rows := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			cols := strings.Split(line, "\t")
			if len(cols) < 2 {
				t.Errorf("%s:%d: want <input><TAB><expected>, got %q", f, i+1, line)
				continue
			}
			input, want := cols[0], cols[1]
			rows++
			got, err := checkRow(input)
			if sub, ok := strings.CutPrefix(want, "ERROR:"); ok {
				// Expected-error row, the corpus format's ERROR:<substring>
				// form: the run must fail and the message must carry the
				// substring.
				if err == nil {
					t.Errorf("%s:%d: %q: want an error containing %q, ran clean with %q", f, i+1, input, sub, got)
				} else if !strings.Contains(err.Error(), sub) {
					t.Errorf("%s:%d: %q: error %q does not contain %q", f, i+1, input, err, sub)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s:%d: %q: run error: %v", f, i+1, input, err)
				continue
			}
			if got != want {
				t.Errorf("%s:%d: %q\n  got  %q\n  want %q", f, i+1, input, got, want)
			}
		}
	}
	t.Logf("check spec corpus: %d rows across %d file(s)", rows, len(files))
}
