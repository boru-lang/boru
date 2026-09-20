// citations_test.go gates the repo's `file:line` citations.
//
// The design docs — NUR.md above all — pin their claims to source with
// `path/to/file.go:123` and `path:123-145` references. Those citations are
// the whole value of a register whose job is that "the acceptance cannot
// silently rot" (NUR.md): a reader follows one to check a claim. But a
// citation is only as good as the file it points into, and editing a cited
// file silently invalidates every reference below the edit. That failure
// mode is not hypothetical — the 2026-08-02 register review broke its own
// citations in five consecutive passes, each time by editing REFERENCE.md
// or EXPLANATION.md and leaving the NUR.md line numbers behind.
//
// This gate makes the drift mechanical rather than a matter of care:
//
//   - the cited file must exist, and
//   - the cited line (or range) must be within it, and
//   - a range must be non-inverted.
//
// It deliberately does NOT try to verify that the cited lines say what the
// citing sentence claims — that needs a reader. What it catches is the
// whole class of "this used to point at the right function".
//
// Scope note: a citation into a file this gate cannot resolve is SKIPPED,
// not failed. Prose says things like `native_storage.go:25-193` with a bare
// basename, and `foo.tsv:12` inside a sentence about a hypothetical. The
// gate resolves bare basenames when they are unambiguous in the repo and
// skips them when they are not, so it never blocks on prose it cannot
// understand. Ambiguity is reported (t.Log) so the count stays visible.
package docexamples

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// citedDocs are the prose and design documents whose citations are gated.
// Every one of them cites source; NUR.md is the densest by a wide margin.
var citedDocs = []string{
	"NUR.md",
	"ADR.md",
	"REFERENCE.md",
	"EXPLANATION.md",
	"AGENTS.md",
	"CLAUDE.md",
	"HOWTO.md",
	"eng/go/CLAUDE.md",
	"lang/go/CLAUDE.md",
}

// citationRe matches `path:N` and `path:N-M`, where path has a file
// extension this repo actually uses. Requiring the extension is what keeps
// the matcher off `NUR.md:26-30`-style self-references (matched, and
// resolvable) while ignoring `§5.10`, `ADR-004`, ratios and version
// strings. The path may be bare (`compare.go:359`) or rooted
// (`eng/go/compare.go:359`).
var citationRe = regexp.MustCompile(
	`([A-Za-z0-9_./-]+\.(?:go|boru|tsv|md|jsonic|json))` + // path
		`:(\d+)(?:-(\d+))?`) // :N or :N-M

// citationExempt lists paths a citation may name that this gate must not
// resolve: generated or vendored trees, and paths that only ever appear
// inside an illustrative sentence.
func citationExempt(p string) bool {
	return strings.HasPrefix(p, "kg/out/") ||
		strings.Contains(p, "node_modules/") ||
		strings.Contains(p, "/testdata/")
}

// TestDocCitationsResolve is the gate. See the file comment.
func TestDocCitationsResolve(t *testing.T) {
	root := docRoot()
	index := buildBasenameIndex(t, root)

	var checked, skippedAmbiguous, skippedUnknown int
	for _, doc := range citedDocs {
		docPath := filepath.Join(root, doc)
		body, err := os.ReadFile(docPath)
		if err != nil {
			// A doc listed here but absent is itself a defect worth failing on.
			t.Errorf("read %s: %v", doc, err)
			continue
		}
		lines := strings.Split(string(body), "\n")
		for i, line := range lines {
			for _, m := range citationRe.FindAllStringSubmatch(line, -1) {
				cited, startS, endS := m[1], m[2], m[3]
				if citationExempt(cited) {
					continue
				}
				target, status := resolveCited(root, cited, index)
				switch status {
				case resolveAmbiguous:
					skippedAmbiguous++
					continue
				case resolveUnknown:
					skippedUnknown++
					continue
				}

				n := countLines(t, target)
				start, _ := strconv.Atoi(startS)
				where := doc + ":" + strconv.Itoa(i+1)

				if start < 1 || start > n {
					t.Errorf("%s cites %s:%s — %s has %d lines, so line %d does not exist",
						where, cited, startS, cited, n, start)
					continue
				}
				if endS != "" {
					end, _ := strconv.Atoi(endS)
					if end < start {
						t.Errorf("%s cites %s:%s-%s — the range is inverted",
							where, cited, startS, endS)
						continue
					}
					if end > n {
						t.Errorf("%s cites %s:%s-%s — %s has only %d lines",
							where, cited, startS, endS, cited, n)
						continue
					}
				}
				checked++
			}
		}
	}
	t.Logf("resolved %d citations; skipped %d ambiguous basenames, %d unresolvable paths",
		checked, skippedAmbiguous, skippedUnknown)
}

type resolveStatus int

const (
	resolveOK resolveStatus = iota
	resolveAmbiguous
	resolveUnknown
)

// resolveCited turns a cited path into an on-disk path. A rooted path is
// used as written; a bare basename is resolved through the index and is
// ambiguous when several files share it.
func resolveCited(root, cited string, index map[string][]string) (string, resolveStatus) {
	if strings.Contains(cited, "/") {
		full := filepath.Join(root, cited)
		if _, err := os.Stat(full); err == nil {
			return full, resolveOK
		}
		return "", resolveUnknown
	}
	matches := index[cited]
	switch len(matches) {
	case 0:
		return "", resolveUnknown
	case 1:
		return matches[0], resolveOK
	default:
		return "", resolveAmbiguous
	}
}

// buildBasenameIndex maps each basename in the repo to every path holding
// it, so an unqualified `compare.go:359` can be resolved when unambiguous.
func buildBasenameIndex(t *testing.T, root string) map[string][]string {
	t.Helper()
	index := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: not this gate's business
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "coverage", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		index[base] = append(index[base], path)
		return nil
	})
	if err != nil {
		t.Fatalf("index repo: %v", err)
	}
	return index
}

// countLines reports the line count of a file, matching the 1-based
// numbering an editor shows: a trailing newline does not add a line.
func countLines(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(body)
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// TestCitationResolutionNegatives pins the resolver's compile failures. The gate is
// only useful if it distinguishes "cannot resolve" (skip, so prose about a
// hypothetical file never blocks a commit) from "resolves and is wrong"
// (fail) — assert both directions, not just the happy path.
func TestCitationResolutionNegatives(t *testing.T) {
	root := docRoot()
	index := buildBasenameIndex(t, root)

	// A rooted path that does not exist is UNKNOWN, never OK.
	if _, st := resolveCited(root, "eng/go/no_such_file.go", index); st != resolveUnknown {
		t.Errorf("a rooted nonexistent path must be unknown, got %v", st)
	}
	// A basename nothing in the repo carries is UNKNOWN.
	if _, st := resolveCited(root, "definitely_not_a_real_name.go", index); st != resolveUnknown {
		t.Errorf("an unknown basename must be unknown, got %v", st)
	}
	// A basename several files share is AMBIGUOUS, so the gate skips it
	// rather than resolving to an arbitrary one and reporting a bogus
	// line count. `registry.go` exists in both eng/go and cmd/go.
	if _, st := resolveCited(root, "registry.go", index); st != resolveAmbiguous {
		t.Errorf("a basename shared by several files must be ambiguous, got %v", st)
	}
	// A rooted path that does exist resolves.
	if _, st := resolveCited(root, "core/go/compare.go", index); st != resolveOK {
		t.Errorf("a rooted real path must resolve, got %v", st)
	}

	// The exemption list keeps generated trees out.
	if !citationExempt("kg/out/graph.json") {
		t.Error("generated kg output must be exempt")
	}
	if citationExempt("eng/go/compare.go") {
		t.Error("ordinary source must not be exempt")
	}

	// countLines matches editor numbering: a trailing newline is a
	// terminator, not an extra line. Off-by-one here would make the gate
	// reject the last legitimately-citable line of every file.
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}
	if got := countLines(t, write("three.txt", "a\nb\nc\n")); got != 3 {
		t.Errorf("trailing-newline file: countLines = %d, want 3", got)
	}
	if got := countLines(t, write("noeol.txt", "a\nb\nc")); got != 3 {
		t.Errorf("no-trailing-newline file: countLines = %d, want 3", got)
	}
	if got := countLines(t, write("empty.txt", "")); got != 0 {
		t.Errorf("empty file: countLines = %d, want 0", got)
	}
}

// TestCitationRegexShape pins what the matcher does and does NOT treat as a
// citation. Over-matching is the failure that would make this gate noisy
// enough to disable: section markers, ADR ids, ratios and version strings
// all look like `x:N` and must not be resolved as file references.
func TestCitationRegexShape(t *testing.T) {
	mustMatch := []string{
		"see `eng/go/compare.go:359` for the arm",
		"`compare.go:359`",
		"REFERENCE.md:1160 carries the row",
		"`native_storage.go:25-193` (`set`)",
		"utils/cut.boru:329-335 computes",
		"`lang/spec/case.tsv:75`",
	}
	for _, s := range mustMatch {
		if citationRe.FindStringSubmatch(s) == nil {
			t.Errorf("must match a citation: %q", s)
		}
	}
	mustNotMatch := []string{
		"IEEE-754 §5.10 requires",
		"ADR-004 says every word ships forward-eligible",
		"the ratio is 3:1 in favour",
		"boru 0.1.0-dev (git bef11070748c)",
		"a 2:1 split",
		"http://example.com:8080/path",
	}
	for _, s := range mustNotMatch {
		if m := citationRe.FindStringSubmatch(s); m != nil {
			t.Errorf("must NOT match a citation: %q (matched %q)", s, m[0])
		}
	}
}
