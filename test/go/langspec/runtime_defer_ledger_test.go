// runtime_defer_ledger_test.go — the per-file ledger of DEFERRED compilation
// failures: corpus rows that compile and then BAIL — the compiled run dying
// with the compiler-defect note (compiled_defect_test.go's population), the
// VM abandoning the run at a designed defer site (vmDefer; core.BailEvent,
// named beside the row) or raising an internal error the compiler admitted
// at compile time. It is compile_failures.tsv's twin for the ledger's worse half
// (compile_defect_test.go in lang/go draws the same line for the unit
// suite): a row that fails to compile is a defect the compiler admits
// before running; a row that bails is one it admits after, and the
// maintainer asked (2026-09-24) that those be KEPT ON A LEDGER rather than
// declined over-wide at compile time — NUR190's `/q` capture and
// Function-typed reference are the first rows booked by choice.
//
// runtime_defers.tsv commits one bailed-row count per lang/spec file, and
// the corpus walk (TestSpecCompiledOrFallback) asserts it for every file the
// run walks — under BORU_SPEC_FILES exactly as over the whole corpus, the
// same both-ways ratchet as the compile-failure ledger: a count above the
// ledger's is debt put back (the file's bailing rows are listed, with the
// defer site and reason each raised), a count below it is the ratchet
// tightening (lower the line in the same change, delete it at zero). The
// corpus-wide "runtime defers" gate (engine_entry_census_test.go's
// deferCeiling) counts defer EVENTS by site; this ledger counts ROWS by
// file, and names them. Nothing regenerates the file.
package langspec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// runtimeDeferLedgerFile is the ledger, beside this file.
const runtimeDeferLedgerFile = "runtime_defers.tsv"

// bailedRow is one corpus row that compiled and then bailed: the row, the
// defer sites that fired (empty for an internal error with no vmDefer) and
// the reason the run surfaced.
type bailedRow struct {
	file   string
	line   int
	input  string
	sites  string // the row's defer sites, in order; empty when none fired
	reason string // the surfaced error's detail
}

// runtimeDeferCensus gathers the corpus walk's bailed rows per file, and
// the set of files walked (zero counts included), for the ledger check.
type runtimeDeferCensus struct {
	mu     sync.Mutex
	byFile map[string]int
	rows   []bailedRow
}

func newRuntimeDeferCensus() *runtimeDeferCensus {
	return &runtimeDeferCensus{byFile: map[string]int{}}
}

// walked records that a file was visited, so a ledger line for it is
// asserted against zero when none of its rows bail.
func (c *runtimeDeferCensus) walked(file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.byFile[file]; !ok {
		c.byFile[file] = 0
	}
}

func (c *runtimeDeferCensus) add(r bailedRow) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byFile[r.file]++
	c.rows = append(c.rows, r)
}

// bailSites joins a row's defer sites in order, deduplicated adjacent.
func bailSites(evs []struct{ Site, Reason string }) string {
	sites := make([]string, 0, len(evs))
	for _, ev := range evs {
		if n := len(sites); n == 0 || sites[n-1] != ev.Site {
			sites = append(sites, ev.Site)
		}
	}
	return strings.Join(sites, ", ")
}

// checkRuntimeDeferLedger asserts the ledger against live, the census's
// per-file counts over every file this run walked (zero included), naming
// the bailing rows of a file that rose. filtered says a corpus filter chose
// the walked files, so a ledger line for a file the walk did not visit is
// not stale; unfiltered, such a line names a file the corpus no longer has.
// On the direction lane a file's open debt is an error, as the corpus-wide
// gate's is.
func checkRuntimeDeferLedger(t testing.TB, ledger, live map[string]int, rows []bailedRow, filtered bool) {
	t.Helper()
	byFile := map[string][]bailedRow{}
	for _, r := range rows {
		byFile[r.file] = append(byFile[r.file], r)
	}
	listed := 0
	for _, file := range sortedFileNames(live) {
		got, want := live[file], ledger[file]
		if want > 0 {
			listed++
		}
		switch {
		case got > want:
			var b strings.Builder
			sort.Slice(byFile[file], func(i, j int) bool { return byFile[file][i].line < byFile[file][j].line })
			for _, r := range byFile[file] {
				sites := r.sites
				if sites == "" {
					sites = "(no defer site: an internal error)"
				}
				fmt.Fprintf(&b, "\n    %s:L%d  %s\n      %s — %s", r.file, r.line, firstN(r.input, 88), sites, r.reason)
			}
			t.Errorf("%s: %d rows compile and then bail, the ledger says %d — a change put debt BACK; the file's bailing rows:%s\n  Fix the compiler so they run. Raising the line in %s to `%s\t%d\t<the rows that moved it>` records the deferral by choice and is never the fix.",
				file, got, want, b.String(), runtimeDeferLedgerFile, file, got)
		case got < want && got == 0:
			t.Errorf("%s: no bailing rows, the ledger says %d — the ratchet tightened; delete the %s line from %s in this change so the ledger keeps catching regressions",
				file, want, file, runtimeDeferLedgerFile)
		case got < want:
			t.Errorf("%s: %d bailing rows, the ledger says %d — the ratchet tightened; lower its line in %s to `%s\t%d` in this change so the ledger keeps catching regressions",
				file, got, want, runtimeDeferLedgerFile, file, got)
		case got > 0:
			directionFailure(t, "%s: %d rows compile and then bail — open debt on the direction lane, every one a defect (%s)", file, got, runtimeDeferLedgerFile)
		}
	}
	if !filtered {
		for _, file := range sortedFileNames(ledger) {
			if _, ok := live[file]; !ok {
				t.Errorf("%s names %s, a spec file the corpus does not have — delete its line", runtimeDeferLedgerFile, file)
			}
		}
	}
	t.Logf("runtime-defer ledger: %d files walked, %d of them listed; ledger total %d (%s)", len(live), listed, ledgerTotal(ledger), runtimeDeferLedgerFile)
}

// assertRuntimeDeferLedger reads the ledger and checks it against the walk's
// census; a missing ledger file is an empty ledger.
func assertRuntimeDeferLedger(t testing.TB, c *runtimeDeferCensus) {
	t.Helper()
	ledger, err := readCompileFailureLedger(runtimeDeferLedgerFile)
	if err != nil {
		if !strings.Contains(err.Error(), "no such file") {
			t.Fatalf("reading %s: %v", runtimeDeferLedgerFile, err)
		}
		ledger = map[string]int{}
	}
	c.mu.Lock()
	live := make(map[string]int, len(c.byFile))
	for k, v := range c.byFile {
		live[k] = v
	}
	rows := append([]bailedRow(nil), c.rows...)
	c.mu.Unlock()
	checkRuntimeDeferLedger(t, ledger, live, rows, filteredCorpus())
}

func TestRuntimeDeferLedgerAssertsEveryWalkedFile(t *testing.T) {
	t.Parallel()
	rows := []bailedRow{
		{file: "a.tsv", line: 9, input: "x z", sites: "vm:landing-claim", reason: "why a9"},
		{file: "a.tsv", line: 3, input: "x y", sites: "vm:landing-quote-claim", reason: "why a3"},
		{file: "b.tsv", line: 1, input: "q", sites: "vm:poly-no-match", reason: "why b1"},
	}
	ledger := map[string]int{"a.tsv": 2, "b.tsv": 1, "gone.tsv": 4}
	live := map[string]int{"a.tsv": 2, "b.tsv": 1, "c.tsv": 0}

	// Exact, under a filter: nothing fails, and the line for the file the
	// walk did not visit is not this run's to judge.
	rec := &recTB{}
	checkRuntimeDeferLedger(rec, ledger, live, rows, true)
	if rec.failed {
		t.Errorf("exact, filtered: errs = %q", rec.errs)
	}
	// Exact, whole corpus: the line for a file the corpus does not have is
	// stale.
	rec = &recTB{}
	checkRuntimeDeferLedger(rec, ledger, live, rows, false)
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0], "gone.tsv") || !strings.Contains(rec.errs[0], "delete its line") {
		t.Errorf("stale line: errs = %q", rec.errs)
	}
	// A rise names the file, both counts and the bailing rows in line
	// order, each with its defer sites and reason.
	rec = &recTB{}
	checkRuntimeDeferLedger(rec, map[string]int{"a.tsv": 1, "b.tsv": 1}, live, rows, true)
	if len(rec.errs) != 1 {
		t.Fatalf("rise: errs = %q", rec.errs)
	}
	for _, want := range []string{"a.tsv: 2 rows compile and then bail, the ledger says 1", "a.tsv:L3  x y", "vm:landing-quote-claim — why a3", "why a9", "`a.tsv\t2\t"} {
		if !strings.Contains(rec.errs[0], want) {
			t.Errorf("rise: the error lacks %q:\n%s", want, rec.errs[0])
		}
	}
	if strings.Index(rec.errs[0], "a.tsv:L3") > strings.Index(rec.errs[0], "a.tsv:L9") {
		t.Errorf("rise: the rows are listed in line order:\n%s", rec.errs[0])
	}
	// A file the ledger does not list rises against zero.
	rec = &recTB{}
	checkRuntimeDeferLedger(rec, map[string]int{}, map[string]int{"c.tsv": 1}, []bailedRow{{file: "c.tsv", line: 2}}, true)
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0], "c.tsv: 1 rows compile and then bail, the ledger says 0") {
		t.Errorf("unlisted rise: errs = %q", rec.errs)
	}
	// A fall is the ratchet tightening: lower the line, or delete it at zero.
	rec = &recTB{}
	checkRuntimeDeferLedger(rec, ledger, map[string]int{"a.tsv": 1, "b.tsv": 0}, rows[:1], true)
	if len(rec.errs) != 2 || !strings.Contains(rec.errs[0], "lower its line") || !strings.Contains(rec.errs[0], "`a.tsv\t1`") ||
		!strings.Contains(rec.errs[1], "b.tsv: no bailing rows") || !strings.Contains(rec.errs[1], "delete the b.tsv line") {
		t.Errorf("fall: errs = %q", rec.errs)
	}
}

// TestRuntimeDeferCensusAndSites: the walk's census counts a file walked
// at zero, a bailed row per file, and joins a row's defer sites in order
// with adjacent repeats folded; the surfaced detail drops its position.
func TestRuntimeDeferCensusAndSites(t *testing.T) {
	t.Parallel()
	c := newRuntimeDeferCensus()
	c.walked("a.tsv")
	c.walked("a.tsv")
	c.add(bailedRow{file: "b.tsv", line: 4})
	if c.byFile["a.tsv"] != 0 || c.byFile["b.tsv"] != 1 || len(c.rows) != 1 {
		t.Errorf("census: %v %v", c.byFile, c.rows)
	}
	evs := []struct{ Site, Reason string }{{"vm:x", "r1"}, {"vm:x", "r2"}, {"vm:y", "r3"}, {"vm:x", "r4"}}
	if got := bailSites(evs); got != "vm:x, vm:y, vm:x" {
		t.Errorf("bailSites = %q", got)
	}
	if got := bailSites(nil); got != "" {
		t.Errorf("bailSites(nil) = %q", got)
	}
	if got := bailDetailOf(fmt.Errorf("plain failure (pc=3)")); got != "plain failure" {
		t.Errorf("bailDetailOf(plain) = %q", got)
	}
}

// TestRuntimeDeferLedgerIsWellFormed: the ledger parses, names only spec
// files lang/spec has, and sums to the walk's exact row count
// (bailDefectCeiling) — the two pins count one population, and moving
// one without the other is the mistake this catches.
func TestRuntimeDeferLedgerIsWellFormed(t *testing.T) {
	t.Parallel()
	ledger, err := readCompileFailureLedger(runtimeDeferLedgerFile)
	if err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	for _, file := range sortedFileNames(ledger) {
		if _, err := os.Stat(filepath.Join(specDir, file)); err != nil {
			t.Errorf("%s names %s, which lang/spec does not have: %v", runtimeDeferLedgerFile, file, err)
		}
	}
	if total := ledgerTotal(ledger); total != bailDefectCeiling {
		t.Errorf("%s sums to %d rows, bailDefectCeiling says %d — the ledger names the rows that ceiling counts; move both in one change", runtimeDeferLedgerFile, total, bailDefectCeiling)
	}
	t.Logf("%s: %d files, %d bailing rows in total", runtimeDeferLedgerFile, len(ledger), ledgerTotal(ledger))
}
