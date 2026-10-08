package langspec

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recTB records what a walk body did to its TB; the embedded nil TB is
// never called because every method a body reaches is overridden here.
type recTB struct {
	testing.TB
	mu     sync.Mutex
	errs   []string
	logs   []string
	failed bool
}

func (r *recTB) Helper() {}
func (r *recTB) Fail()   { r.mu.Lock(); r.failed = true; r.mu.Unlock() }
func (r *recTB) Error(args ...any) {
	r.mu.Lock()
	r.failed = true
	r.errs = append(r.errs, strings.TrimSpace(strings.Join(toStrings(args), " ")))
	r.mu.Unlock()
}
func (r *recTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	r.failed = true
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}
func (r *recTB) Log(args ...any) {
	r.mu.Lock()
	r.logs = append(r.logs, strings.Join(toStrings(args), " "))
	r.mu.Unlock()
}
func (r *recTB) Logf(format string, args ...any) {
	r.mu.Lock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

func TestParseSpecRowsCountsEveryLine(t *testing.T) {
	t.Parallel()
	rows := parseSpecRows("x.tsv", "# head\n\n1 add 2\t3\r\n  \n\tsecond\tcell  \n")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2: %+v", len(rows), rows)
	}
	if rows[0].Line != 3 || rows[0].Input != "1 add 2" || len(rows[0].Cells) != 2 || rows[0].Cells[1] != "3" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Line != 5 || rows[1].Input != "" || rows[1].Key() != "x.tsv:L5" || len(rows[1].Cells) != 3 {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestRowTBAbortsTheRowNotTheTest(t *testing.T) {
	t.Parallel()
	rec := &recTB{}
	rt := rowTB{rec}
	reached := 0
	runGuarded(rt, "a.tsv:L1", func() { rt.Fatalf("bad %d", 1); reached++ })
	runGuarded(rt, "a.tsv:L2", func() { rt.Skip("skipped"); reached++ })
	runGuarded(rt, "a.tsv:L3", func() { rt.FailNow(); reached++ })
	runGuarded(rt, "a.tsv:L4", func() { panic("boom") })
	runGuarded(rt, "a.tsv:L5", func() { reached++ })
	if reached != 1 {
		t.Errorf("reached = %d, want 1 (only the plain row runs to its end)", reached)
	}
	if !rec.failed || len(rec.errs) != 2 {
		t.Fatalf("errs = %q (failed=%v), want the Fatalf and the panic", rec.errs, rec.failed)
	}
	if rec.errs[0] != "bad 1" || !strings.HasPrefix(rec.errs[1], "a.tsv:L4: panic in the walk body: boom") {
		t.Errorf("errs = %q", rec.errs)
	}
	if len(rec.logs) != 1 || rec.logs[0] != "skipped" {
		t.Errorf("logs = %q, want the skip's message", rec.logs)
	}
}

// TestSpecFilterMustSelectEveryPattern pins the filter seam: every name or
// glob in BORU_SPEC_FILES must select at least one spec file, and must be a
// pattern. It sets the environment, so it runs before the parallel batch.
func TestSpecFilterMustSelectEveryPattern(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")

	t.Setenv("BORU_SPEC_FILES", "callbacks.tsv, fold-*.tsv")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "callbacks.tsv,fold-map-filter.tsv" {
		t.Errorf("selected %v, want callbacks.tsv and fold-map-filter.tsv", names)
	}
	files, err := readSpecCorpusErr()
	if err != nil || len(files) != 2 {
		t.Errorf("corpus = %d files, err %v; want the two selected files", len(files), err)
	}

	bad := []struct{ name, filter, want string }{
		{"a misspelt name", "calbacks.tsv", `"calbacks.tsv" selects no file`},
		{"a typo among several", "callbacks.tsv,nope-*.tsv", `"nope-*.tsv" selects no file`},
		{"a glob that matches nothing", "zz-*.tsv", `"zz-*.tsv" selects no file`},
		{"not a pattern", "[", `"[" is not a file name or glob`},
	}
	for _, c := range bad {
		t.Setenv("BORU_SPEC_FILES", c.filter)
		if _, err := specEntries(specDir); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: specEntries err = %v, want one containing %q", c.name, err, c.want)
		}
		if _, err := readSpecCorpusErr(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: readSpecCorpusErr err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestSpecWalkVisitsEveryRowOnce(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	seen := map[string]int{}
	files := map[string]bool{}
	specWalk(t, func(t testing.TB, r specRow) {
		mu.Lock()
		seen[r.Key()]++
		files[r.File] = true
		mu.Unlock()
	})
	for k, n := range seen {
		if n != 1 {
			t.Errorf("%s visited %d times", k, n)
		}
	}
	perFile := 0
	specWalkFiles(t, func(t testing.TB, file string, rows []specRow) {
		mu.Lock()
		perFile += len(rows)
		mu.Unlock()
		if !files[file] {
			t.Errorf("%s: file the row walk did not visit", file)
		}
	})
	if perFile != len(seen) || len(seen) == 0 {
		t.Errorf("row walk saw %d rows, file walk %d", len(seen), perFile)
	}
}

func toStrings(args []any) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = fmt.Sprintf("%v", a)
	}
	return out
}
