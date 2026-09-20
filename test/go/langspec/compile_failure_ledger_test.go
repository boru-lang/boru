// compile_failure_ledger_test.go — the per-file compile-failure ratchets:
// P0 of design/FULL-COMPILATION-REPLAN.0.md (§3).
//
// compile_failures.tsv commits one compile-failure count per lang/spec file,
// and TestCompiledCoverage asserts it for every file a run walks — under
// BORU_SPEC_FILES exactly as over the whole corpus. That is what the
// corpus-wide "compile failures" gate cannot do: its one number is not
// comparable with a subset's, so under a filter it reports (lanes_test.go),
// and a regression in one family stayed invisible until the whole-corpus
// run. On 2026-09-18 a change moved lang/spec/as.tsv 52–54 from compiling
// to failing and every six-second filtered run over as.tsv was green; the
// whole-corpus gate found it twelve minutes later, in review. With the
// ledger the filtered run fails on the spot, naming the rows.
//
// The ledger is pinned BOTH ways per file. A count above the ledger's is a
// regression: a change put debt back, and the file's failing rows are
// listed so the fix starts from them. A count below it is the ratchet
// tightening: lower the line (delete it at zero) in the same change, so the
// ledger is always the live value and cannot sit above it where it would
// miss the next regression. The corpus-wide ceiling is the ledger's sum. A
// file the ledger does not name has no failing rows. Nothing regenerates
// the file — every line is edited by hand, the rows that moved it in the
// note column — because a switch that rewrote it would bake a regression in
// as quietly as the unasserted filter used to let one through.
package langspec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// compileFailureLedgerFile is the ledger, beside this file.
const compileFailureLedgerFile = "compile_failures.tsv"

// readCompileFailureLedger reads and parses the ledger.
func readCompileFailureLedger(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseCompileFailureLedger(path, string(data))
}

// parseCompileFailureLedger parses ledger text: one `<spec file> TAB <count>
// [TAB <note>]` per line, `#` lines and blank lines skipped. The names are
// lang/spec basenames, unique and in byte order (so a diff reads and a
// merge conflicts honestly); every count is a positive integer, because a
// file with no failing rows is left out rather than listed at zero.
func parseCompileFailureLedger(path, data string) (map[string]int, error) {
	ledger := map[string]int{}
	prev := ""
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(strings.TrimSuffix(line, "\r"), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cells := strings.Split(line, "\t")
		if len(cells) < 2 || len(cells) > 3 {
			return nil, fmt.Errorf("%s:%d: want <spec file>\\t<count>[\\t<note>], got %q", path, i+1, line)
		}
		name := cells[0]
		if !strings.HasSuffix(name, ".tsv") || strings.ContainsAny(name, "/\\ ") {
			return nil, fmt.Errorf("%s:%d: %q is not a lang/spec basename", path, i+1, name)
		}
		n, err := strconv.Atoi(cells[1])
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s:%d: %s: the count must be a positive integer (a file with no failing rows is left out), got %q", path, i+1, name, cells[1])
		}
		if _, dup := ledger[name]; dup {
			return nil, fmt.Errorf("%s:%d: %s is listed twice", path, i+1, name)
		}
		if name < prev {
			return nil, fmt.Errorf("%s:%d: %s is out of order — the ledger is sorted by file name", path, i+1, name)
		}
		ledger[name] = n
		prev = name
	}
	return ledger, nil
}

// ledgerTotal is the sum of the ledger — the corpus-wide compile-failure
// ceiling.
func ledgerTotal(ledger map[string]int) int {
	total := 0
	for _, n := range ledger {
		total += n
	}
	return total
}

// sortedFileNames is a ledger's or a census's file names in byte order.
func sortedFileNames(m map[string]int) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// checkCompileFailureLedger asserts the ledger against live, the census's
// per-file counts over every file this run walked (zero included), naming
// the failing rows of a file that rose. filtered says a corpus filter chose
// the walked files, so a ledger line for a file the walk did not visit is
// not stale — it is simply not this run's; unfiltered, such a line names a
// file the corpus no longer has. On the direction lane a file's open debt
// is an error, as the corpus-wide gate's is.
func checkCompileFailureLedger(t testing.TB, ledger, live map[string]int, rows []failedRow, filtered bool) {
	t.Helper()
	byFile := map[string][]failedRow{}
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
			for _, r := range byFile[file] {
				fmt.Fprintf(&b, "\n    %s:L%d  %s\n      %s", r.file, r.line, firstN(r.input, 88), r.reason)
			}
			t.Errorf("%s: %d compile failures, the ledger says %d — a change put debt BACK; the file's failing rows:%s\n  Fix the compiler so they compile. Raising the line in %s to `%s\t%d\t<the rows that moved it>` records a regression and is never the fix.",
				file, got, want, b.String(), compileFailureLedgerFile, file, got)
		case got < want && got == 0:
			t.Errorf("%s: no compile failures, the ledger says %d — the ratchet tightened; delete the %s line from %s in this change so the ledger keeps catching regressions",
				file, want, file, compileFailureLedgerFile)
		case got < want:
			t.Errorf("%s: %d compile failures, the ledger says %d — the ratchet tightened; lower its line in %s to `%s\t%d` in this change so the ledger keeps catching regressions",
				file, got, want, compileFailureLedgerFile, file, got)
		case got > 0:
			directionFailure(t, "%s: %d compile failures — open debt on the direction lane, every one a BUG (%s)", file, got, compileFailureLedgerFile)
		}
	}
	if !filtered {
		for _, file := range sortedFileNames(ledger) {
			if _, ok := live[file]; !ok {
				t.Errorf("%s names %s, a spec file the corpus does not have — delete its line", compileFailureLedgerFile, file)
			}
		}
	}
	t.Logf("compile-failure ledger: %d files walked, %d of them listed; ledger total %d (%s)", len(live), listed, ledgerTotal(ledger), compileFailureLedgerFile)
}

// TestCompileFailureLedgerIsWellFormed parses the committed ledger and checks
// that every file it names exists — in milliseconds, so a filtered or smoke
// run catches a stale line the census walk would not visit.
func TestCompileFailureLedgerIsWellFormed(t *testing.T) {
	t.Parallel()
	ledger, err := readCompileFailureLedger(compileFailureLedgerFile)
	if err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	for _, file := range sortedFileNames(ledger) {
		if _, err := os.Stat(filepath.Join(specDir, file)); err != nil {
			t.Errorf("%s names %s, which lang/spec does not have: %v", compileFailureLedgerFile, file, err)
		}
	}
	t.Logf("%s: %d files, %d compile failures in total", compileFailureLedgerFile, len(ledger), ledgerTotal(ledger))
}

func TestCompileFailureLedgerParses(t *testing.T) {
	t.Parallel()
	good := "# a comment\n\na.tsv\t2\tthe note\r\nb.tsv\t1\n"
	ledger, err := parseCompileFailureLedger("l.tsv", good)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 2 || ledger["a.tsv"] != 2 || ledger["b.tsv"] != 1 || ledgerTotal(ledger) != 3 {
		t.Errorf("ledger = %v", ledger)
	}
	if names := sortedFileNames(ledger); strings.Join(names, ",") != "a.tsv,b.tsv" {
		t.Errorf("names = %v", names)
	}
	bad := []struct{ name, text, want string }{
		{"one cell", "a.tsv\n", "want <spec file>"},
		{"four cells", "a.tsv\t1\tnote\textra\n", "want <spec file>"},
		{"a path, not a basename", "lang/spec/a.tsv\t1\n", "not a lang/spec basename"},
		{"not a spec file", "a.md\t1\n", "not a lang/spec basename"},
		{"zero", "a.tsv\t0\n", "positive integer"},
		{"negative", "a.tsv\t-1\n", "positive integer"},
		{"not a number", "a.tsv\tthree\n", "positive integer"},
		{"duplicate", "a.tsv\t1\na.tsv\t2\n", "listed twice"},
		{"out of order", "b.tsv\t1\na.tsv\t2\n", "out of order"},
	}
	for _, c := range bad {
		_, err := parseCompileFailureLedger("l.tsv", c.text)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestCompileFailureLedgerAssertsEveryWalkedFile(t *testing.T) {
	t.Parallel()
	rows := []failedRow{
		{file: "a.tsv", line: 3, input: "x y", reason: "why a3"},
		{file: "a.tsv", line: 9, input: "x z", reason: "why a9"},
		{file: "b.tsv", line: 1, input: "q", reason: "why b1"},
	}
	ledger := map[string]int{"a.tsv": 2, "b.tsv": 1, "gone.tsv": 4}
	live := map[string]int{"a.tsv": 2, "b.tsv": 1, "c.tsv": 0}

	// Exact, under a filter: nothing fails, and the line for the file the
	// walk did not visit is not this run's to judge.
	rec := &recTB{}
	checkCompileFailureLedger(rec, ledger, live, rows, true)
	if rec.failed {
		t.Errorf("exact, filtered: errs = %q", rec.errs)
	}
	// Exact, whole corpus: the line for a file the corpus does not have is
	// stale.
	rec = &recTB{}
	checkCompileFailureLedger(rec, ledger, live, rows, false)
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0], "gone.tsv") || !strings.Contains(rec.errs[0], "delete its line") {
		t.Errorf("stale line: errs = %q", rec.errs)
	}
	// A rise names the file, both counts and the failing rows.
	rec = &recTB{}
	checkCompileFailureLedger(rec, map[string]int{"a.tsv": 1, "b.tsv": 1}, live, rows, true)
	if len(rec.errs) != 1 {
		t.Fatalf("rise: errs = %q", rec.errs)
	}
	for _, want := range []string{"a.tsv: 2 compile failures, the ledger says 1", "a.tsv:L3  x y", "why a9", "`a.tsv\t2\t"} {
		if !strings.Contains(rec.errs[0], want) {
			t.Errorf("rise: the error lacks %q:\n%s", want, rec.errs[0])
		}
	}
	// A file the ledger does not list rises against zero.
	rec = &recTB{}
	checkCompileFailureLedger(rec, map[string]int{}, map[string]int{"c.tsv": 1}, []failedRow{{file: "c.tsv", line: 2}}, true)
	if len(rec.errs) != 1 || !strings.Contains(rec.errs[0], "c.tsv: 1 compile failures, the ledger says 0") {
		t.Errorf("unlisted rise: errs = %q", rec.errs)
	}
	// A fall is the ratchet tightening: lower the line, or delete it at zero.
	rec = &recTB{}
	checkCompileFailureLedger(rec, ledger, map[string]int{"a.tsv": 1, "b.tsv": 0}, rows[:1], true)
	if len(rec.errs) != 2 || !strings.Contains(rec.errs[0], "lower its line") || !strings.Contains(rec.errs[0], "`a.tsv\t1`") ||
		!strings.Contains(rec.errs[1], "b.tsv: no compile failures") || !strings.Contains(rec.errs[1], "delete the b.tsv line") {
		t.Errorf("fall: errs = %q", rec.errs)
	}
}
