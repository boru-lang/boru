// walk_test.go — the one corpus walk: every lang/spec file, in parallel.
//
// Sixteen of this package's tests each walked the 8,500-row corpus on one
// goroutine — 70 to 125 seconds each, some 1,450 CPU-seconds a run — while
// the runner's other cores idled. specWalk and specWalkFiles are the shared
// walk: the files BORU_SPEC_FILES selects (lanes_test.go), parsed once on
// the test goroutine, handed to one worker per CPU largest file first (so
// the tail of the walk is a small file), each file's rows in order. A test
// body sees one row, or one file, at a time and never reads the directory
// itself.
//
// What a body may do. It runs on a worker goroutine, so it gets a
// testing.TB whose Fatal*, FailNow and Skip* abort THE ROW (or the file),
// never the test — testing's own FailNow may only run on the test
// goroutine — and every per-row helper takes testing.TB for the same
// reason. Name the body's parameter t so the test's own t is shadowed and
// cannot be reached from a worker. Errorf and Logf are goroutine-safe.
// Shared accumulators live behind a mutex in the test; a counting gate is
// order-independent, so no file order is promised, and a body that prints
// a list a reader compares (a committed status file, a worklist) sorts it
// first. A panic in a body is reported against its row and the walk goes
// on, so one broken row names itself instead of taking the binary down.
//
// BORU_SPEC_WORKERS=1 makes the walk sequential and in directory order —
// the shape to reach for with a temporary println at a decision site.
package langspec

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// specRow is one corpus row: a non-blank, non-comment line of a lang/spec
// file, split on tabs. Input is the program cell, Cells[0], trimmed.
type specRow struct {
	File  string   // basename, e.g. "callbacks.tsv"
	Line  int      // 1-based line in the file, blank and comment lines counted
	Cells []string // the tab-separated cells, the line's trailing blanks dropped
	Input string   // strings.TrimSpace(Cells[0])
}

// Key is the row's ledger key, "file.tsv:Lnn" — the form knownDivergences
// and every per-row log use.
func (r specRow) Key() string { return r.File + ":L" + strconv.Itoa(r.Line) }

// specFile is one parsed corpus file.
type specFile struct {
	name string
	rows []specRow
}

// specWalk runs body over every corpus row, files in parallel and each
// file's rows in order.
func specWalk(t testing.TB, body func(t testing.TB, r specRow)) {
	t.Helper()
	specWalkFiles(t, func(t testing.TB, _ string, rows []specRow) {
		for _, r := range rows {
			runGuarded(t, r.Key(), func() { body(t, r) })
		}
	})
}

// specWalkFiles runs body once per corpus file with all of the file's
// rows, files in parallel. It is the form for a body that keeps per-file
// state — one instance per file, a previous row.
func specWalkFiles(t testing.TB, body func(t testing.TB, file string, rows []specRow)) {
	t.Helper()
	walkSpecFiles(readSpecCorpus(t), func(f specFile) {
		runGuarded(t, f.name, func() { body(rowTB{t}, f.name, f.rows) })
	})
}

// specWalkFilesErr is specWalkFiles for a walk that has no test to report
// to — the compiled census, computed once under a sync.Once and handed to
// four tests (compiled_census_test.go). It returns what that walk would
// have failed a test with: the corpus read error, or the first panic in a
// body, named by its file.
func specWalkFilesErr(body func(file string, rows []specRow)) error {
	files, err := readSpecCorpusErr()
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var first error
	walkSpecFiles(files, func(f specFile) {
		defer func() {
			if r := recover(); r != nil {
				mu.Lock()
				if first == nil {
					first = fmt.Errorf("%s: panic in the walk body: %v\n%s", f.name, r, debug.Stack())
				}
				mu.Unlock()
			}
		}()
		body(f.name, f.rows)
	})
	return first
}

// walkSpecFiles is the scheduler under both file walks: one worker per CPU,
// largest file first, or directory order under BORU_SPEC_WORKERS=1.
func walkSpecFiles(files []specFile, run func(f specFile)) {
	workers := specWorkers()
	if workers <= 1 {
		for _, f := range files {
			run(f)
		}
		return
	}
	sort.SliceStable(files, func(i, j int) bool { return len(files[i].rows) > len(files[j].rows) })
	jobs := make(chan specFile)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				run(f)
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
}

// specWorkers is the walk's parallelism: BORU_SPEC_WORKERS, else one per
// CPU.
func specWorkers() int {
	if s := os.Getenv("BORU_SPEC_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return runtime.GOMAXPROCS(0)
}

// readSpecCorpus parses every selected corpus file on the test goroutine,
// the one place a read failure is allowed to be fatal.
func readSpecCorpus(t testing.TB) []specFile {
	t.Helper()
	files, err := readSpecCorpusErr()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// readSpecCorpusErr is the reader behind readSpecCorpus, returning the read
// error for a walk with no test to fail (specWalkFilesErr).
func readSpecCorpusErr() ([]specFile, error) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", specDir, err)
	}
	var files []specFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		files = append(files, specFile{name: e.Name(), rows: parseSpecRows(e.Name(), string(data))})
	}
	return files, nil
}

// parseSpecRows splits one file into its rows exactly as the scanner walks
// did: every line counted, a trailing CR and trailing blanks dropped,
// blank and # lines skipped, cells split on tab.
func parseSpecRows(name, data string) []specRow {
	var rows []specRow
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(strings.TrimSuffix(line, "\r"), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cells := strings.Split(line, "\t")
		rows = append(rows, specRow{File: name, Line: i + 1, Cells: cells, Input: strings.TrimSpace(cells[0])})
	}
	return rows
}

// rowTB is the testing.TB a walk body sees: the test's own TB with the
// calls that may not leave a worker goroutine turned into a row abort.
// Everything else — Errorf, Logf, Helper, Cleanup, TempDir — is the test's.
type rowTB struct{ testing.TB }

// rowAbort is the panic value a row abort travels as; runGuarded recovers
// it.
type rowAbort struct{}

func (r rowTB) FailNow() { r.TB.Fail(); panic(rowAbort{}) }

func (r rowTB) Fatal(args ...any) { r.TB.Helper(); r.TB.Error(args...); panic(rowAbort{}) }

func (r rowTB) Fatalf(format string, args ...any) {
	r.TB.Helper()
	r.TB.Errorf(format, args...)
	panic(rowAbort{})
}

func (r rowTB) SkipNow() { panic(rowAbort{}) }

func (r rowTB) Skip(args ...any) { r.TB.Helper(); r.TB.Log(args...); panic(rowAbort{}) }

func (r rowTB) Skipf(format string, args ...any) {
	r.TB.Helper()
	r.TB.Logf(format, args...)
	panic(rowAbort{})
}

// runGuarded runs fn: a row abort returns, any other panic fails the test
// against key and returns, so the walk goes on.
func runGuarded(t testing.TB, key string, fn func()) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(rowAbort); ok {
			return
		}
		t.Errorf("%s: panic in the walk body: %v\n%s", key, r, debug.Stack())
	}()
	fn()
}
