// oraclegate_test.go stops NUR106 from recurring.
//
// Stage J flipped `lang.Run` from the tree-walking interpreter to the COMPILED
// path (with an interpreter fallback only on refusal). 75 parity assertions
// across five files were not swept with it and kept reading `Run` as their
// interpreter oracle — so each compared the compiled lane to ITSELF and passed
// unconditionally. Five NUR101 miscompiles and the divergence filed as NUR107
// sat behind them, green.
//
// That is worse than having no harness: it converts a whole class of defect
// into a passing check, and the passing check is then cited as evidence.
//
// THE SWEEP IS NOT THE FIX. It has now been run three times and found residue
// each time. The first pass keyed on one naming convention (`gotI…Run(`) and
// missed `errI`, `eI`, `gi`/`ei` and an effect closure. The second completed
// lang/go and surfaced NUR108 and NUR109. The third (2026-08-28) found four
// more in the SIBLING package lang/go/test, where variables literally named
// `interp` were read from `Run` and compared against `RunCompiled` under an
// error message that said "(miscompile)". A convention-keyed sweep is not a
// sweep, and a sweep does not stay swept.
//
// So this gate pins the ARCHITECTURE instead, the way fissiongate does: in a
// test file that also calls RunCompiled, a bare `Run` is presumed to be an
// oracle read and must be justified BY FILE, with a count. Adding one to a
// pinned file trips the count; adding one to an unpinned file trips the table.
//
// If you are here because the gate failed:
//
//   - Adding a PARITY assertion? Use RunInterp. It is the named oracle and its
//     own doc comment says so. That is the fix, not an entry here.
//   - Adding a single-lane test to a pinned file? Bump its count and say why
//     in the reason if the reason no longer covers it.
//   - A pinned count went DOWN? That is the ratchet tightening — lower it.
package oraclegate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// allowed maps a test file (repo-relative) to the number of non-oracle `Run`
// calls it may contain, with the reason they are not lane comparisons.
//
// Every entry is a file that runs a program on ONE lane on purpose. None of
// them compares an answer against RunCompiled — that is the property this
// table asserts, and the reason each entry names is what a reviewer checks.
var allowed = map[string]struct {
	count int
	why   string
}{
	// Asserts that the PUBLIC Run IS the compiled path — the very flip that
	// caused NUR106. Sweeping this one would delete the check that pins it.
	"lang/go/frontier_cases_test.go": {1, "p11/public-run-is-compiled asserts Run is the compiled lane"},

	// I/O behaviour over a fake stdin: what the program PRINTS and reads, on
	// whichever lane Run picks. Its one genuine parity test (stream probes)
	// already uses RunInterp explicitly.
	"lang/go/test/io_lines_test.go": {4, "stdin/tty behaviour helpers; the parity test in this file uses RunInterp. 12 -> 4 on 2026-09-19 (the fallback removal): eight helpers route through runReference, which books the compile failure and reads the meaning off the reference engine"},

	// Error-shape assertions (`err == nil` checks) and a canon round-trip that
	// reads one lane only.
	"lang/go/test/fn_triple_compiled_test.go":   {2, "single-lane error-shape checks; the parity loop uses RunInterp. 3 -> 2 on 2026-09-19 (the fallback removal)"},
	"lang/go/test/module_typed_exports_test.go": {1, "single-lane error-shape check; the parity loop uses RunInterp"},
}

// runCall matches a `.Run(` that is not one of the explicitly-named entry
// points. t.Run/b.Run are subtests; RunCompiled/RunInterp are the two lanes
// named on purpose; RunUnit/RunResolved are engine internals. NewTop(...).Run
// is the ENGINE's run over a token slice — a different API that takes values,
// not source, so it cannot be an oracle read of a program.
// selfPath is this gate's own file: it necessarily contains both patterns it
// searches for, and is not a test of any boru program.
const selfPath = "test/go/oraclegate/oraclegate_test.go"

var (
	runCall  = regexp.MustCompile(`\.Run\(`)
	excluded = regexp.MustCompile(`\bt\.Run\(|\bb\.Run\(|\.RunCompiled\(|\.RunInterp\(|\.RunUnit\(|RunResolved\(|NewTop\([^)]*\)\.Run\(`)
)

func TestNoStrandedOracleReads(t *testing.T) {
	root := repoRoot(t)
	found := map[string]int{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if !strings.Contains(string(src), "RunCompiled(") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == selfPath {
			return nil // this gate's own source names both patterns to match them
		}
		for _, line := range strings.Split(string(src), "\n") {
			if runCall.MatchString(line) && !excluded.MatchString(line) {
				found[rel]++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for file, n := range found {
		entry, ok := allowed[file]
		if !ok {
			t.Errorf("%s calls RunCompiled AND has %d bare Run call(s) that are not pinned.\n"+
				"  If any of them is a PARITY oracle, it is comparing the compiled lane to "+
				"itself (NUR106) — switch it to RunInterp.\n"+
				"  If they are single-lane on purpose, add the file to `allowed` with a reason.", file, n)
			continue
		}
		if n != entry.count {
			t.Errorf("%s: %d bare Run call(s), pinned at %d (%s).\n"+
				"  More: check the new one is not an oracle read, then bump the count.\n"+
				"  Fewer: the ratchet tightened — lower it.", file, n, entry.count, entry.why)
		}
	}
	for file, entry := range allowed {
		if _, ok := found[file]; !ok {
			t.Errorf("%s is pinned at %d (%s) but has no bare Run calls — remove the entry "+
				"(the ratchet only tightens)", file, entry.count, entry.why)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, serr := os.Stat(filepath.Join(dir, "go.work")); serr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal(fmt.Sprint("could not find the repo root (no go.work above ", dir, ")"))
	return ""
}
