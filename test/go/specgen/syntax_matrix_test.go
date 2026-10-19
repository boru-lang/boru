//go:build specgen

package main

// This runner carries the `specgen` build tag so the heavy matrix-replay
// tests are EXCLUDED from the normal unit-test run (`make test` /
// `go test ./...`, which compile this package but find no test files).
// They re-execute the interpreter and the checker over tens of
// thousands of generated rows (~2.5 min sampled, ~15 min with
// SPECGEN_FULL=1, on ten cores; the daily build splits the full run six
// ways with SPECGEN_SHARD) — a regression corpus, not a fast unit test. Run them
// explicitly with `make spec-test` (or `go test -tags specgen
// ./specgen/`). The generator (main.go) stays UNtagged so `make
// spec-gen` and `go run ./specgen` keep working without the tag.
//
// Self-contained runner for the generated syntax-combination matrix
// (syntax-matrix.tsv, produced by this package's generator). It pins the
// two deterministic trust properties the matrix exists to defend —
// stated in the package doc comment of main.go:
//
//   1. Interpreter correctness (TestSyntaxMatrixInterpreter): every row
//      re-evaluates to the recorded canonical value, or to the recorded
//      error class. This is the frozen snapshot — if the interpreter's
//      output for any of the ~7k sequences drifts, this fails. (Identity
//      with the generator's own recipe is the point: the spec is exactly
//      what the interpreter produces, captured deterministically.)
//
//   2. Checker determinism (TestSyntaxMatrixCheckerDeterministic): the
//      type checker yields byte-identical diagnostics and residual stack
//      on repeated runs of the same row, and never panics. Determinism is
//      the trust property here — the curated accuracy/soundness ratchets
//      live in the langspec suite, against the hand-written corpus.
//
// The matrix is regenerated, not edited; keep these gates in lockstep
// with the generator's evaluation recipe (a divergence would surface as
// an interpreter-correctness failure on the next regeneration).

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

const matrixPath = "syntax-matrix.tsv"

// frontierSet groups the three derived files for one length window — the
// passing subset plus the two minimal-failing-prefix buckets — with the
// floors that keep each from silently collapsing. The full set spans the
// whole length-4 matrix; the len123 set is the same separation restricted
// to combinations of length 1..3; the len5 set is the length-5 layer
// (each length-4 passing row extended by one atom).
//
// `sample` caps how many rows of EACH file the disposition tests re-run
// through the engine (0 = every row). The huge len5 files are sampled by
// default — a deterministic 1-in-k slice, enough to catch any systematic
// classification regression — while the floors still gate the FULL row
// counts. Set SPECGEN_FULL=1 to verify every row of every file.
type frontierSet struct {
	label                            string
	passing, failCheck, failRuntime  string
	minPassing, minCheck, minRuntime int
	sample                           int
}

var (
	// fullSet — the length-1..4 separation. Floors pinned just below the
	// measured counts as headroom.
	//
	// minPassing RE-BASED 29000 -> 28000 at the 2026-10-19 regeneration
	// (measured 28032 / 11741 / 298). The matrix had gone stale against
	// three documented language rules (REFERENCE.md): the argument-order
	// rule's barrier — another function word stops forward filling, so
	// `0 not dup` is now a signature_error, not `0 false` — Boolean
	// arithmetic as a defined type_error, and the String occurrence
	// package (`'x' 'x' sub` is now `''`). 8604 of the 137560 length-1..4
	// programs changed result, ~6100 of them from a value to an error, so
	// the passing subset shrank by design, not by an extractor regression.
	fullSet = frontierSet{
		label:       "full(len1-4)",
		passing:     "syntax-matrix-passing.tsv",
		failCheck:   "syntax-matrix-fail-check.tsv",
		failRuntime: "syntax-matrix-fail-runtime.tsv",
		minPassing:  28000, minCheck: 9800, minRuntime: 180,
		sample: 0, // every row — the full set is small enough
	}
	// len123Set — the same separation over length-1..3 combinations only.
	// Counts measured at 2056 / 595 / 56 / 0 — the none-ordering soundness
	// fix moved every remaining length-1..3 runtime failure (all were
	// cross-family ordering against `none`) to check, so the runtime
	// bucket is now legitimately EMPTY (minRuntime 0). Floors pinned just
	// below the others.
	len123Set = frontierSet{
		label:       "len123",
		passing:     "syntax-matrix-len123-passing.tsv",
		failCheck:   "syntax-matrix-len123-fail-check.tsv",
		failRuntime: "syntax-matrix-len123-fail-runtime.tsv",
		minPassing:  2000, minCheck: 580, minRuntime: 0,
		sample: 0, // every row
	}
	// len5Set — the length-5 layer (length-4 passing rows × one atom).
	// Floors pinned below the measured counts. Sampled by default (these
	// files are large); SPECGEN_FULL=1 forces exhaustive verification.
	// minPassing RE-BASED 374000 -> 355000 at the same regeneration as
	// fullSet's (measured 355726 / 131859 / 6415): fewer length-4 programs
	// pass, so fewer length-5 extensions exist to pass.
	len5Set = frontierSet{
		label:       "len5",
		passing:     "syntax-matrix-len5-passing.tsv",
		failCheck:   "syntax-matrix-len5-fail-check.tsv",
		failRuntime: "syntax-matrix-len5-fail-runtime.tsv",
		minPassing:  355000, minCheck: 104000, minRuntime: 5300,
		sample: 12000,
	}
	frontierSets = []frontierSet{fullSet, len123Set, len5Set}
)

// fullMode reports whether SPECGEN_FULL=1 was set, forcing the disposition
// tests to verify every row rather than a sample.
func fullMode() bool { return os.Getenv("SPECGEN_FULL") == "1" }

// sampleRows returns a deterministic subset of rows of at most cap entries
// (every k-th row), or all rows when cap<=0, the set already fits, or
// fullMode is on. Sampling bounds the engine work for the huge len5 files
// while still exercising a representative spread of each.
func sampleRows(rows []matrixRow, cap int) []matrixRow {
	if cap <= 0 || len(rows) <= cap || fullMode() {
		return rows
	}
	stride := len(rows) / cap
	if stride < 1 {
		stride = 1
	}
	out := make([]matrixRow, 0, cap+1)
	for i := 0; i < len(rows); i += stride {
		out = append(out, rows[i])
	}
	return out
}

// matrixRow is one parsed data row of the matrix.
type matrixRow struct {
	line     int
	input    string
	expected string // canonical value, or "ERROR:<class>"
	n        int    // element count (1..4), parsed from the note column
}

// readMatrix parses the full matrix file into data rows. The base matrix is
// never legitimately empty, so a 0-row read is a generation failure.
func readMatrix(t *testing.T) []matrixRow {
	rows := readRows(t, matrixPath)
	if len(rows) == 0 {
		t.Fatalf("%s has no data rows (regenerate with `make spec-gen`)", matrixPath)
	}
	return rows
}

// readRows parses a generated tsv (full matrix or a front-coded derived
// subset) into data rows, skipping the comment/blank header lines. Both
// formats are decoded by forEachDataRow (shared with the generator): the
// plain base matrix yields a note column (its "N-elem …" lead gives the
// element count), while the front-coded subsets carry no note (n = 0).
func readRows(t *testing.T, path string) []matrixRow {
	t.Helper()
	var rows []matrixRow
	err := forEachDataRow(path, func(input, expected, note string, line int) {
		// The note column ("N-elem …") leads with the element count; default
		// 0 (treated as "short", always fully covered) if absent.
		elems := 0
		if len(note) > 0 && note[0] >= '1' && note[0] <= '9' {
			elems = int(note[0] - '0')
		}
		rows = append(rows, matrixRow{line: line, input: input, expected: expected, n: elems})
	})
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with `make spec-gen`)", path, err)
	}
	// A derived bucket may legitimately be empty (e.g. no length-1..3 program
	// fails ONLY at runtime once concrete cross-family ordering is caught at
	// check time). The per-file floors — not a blanket non-empty check — gate
	// against an unexpected collapse; the base matrix is guarded in readMatrix.
	return rows
}

// The generator's own evaluation recipe (run), error-class extraction
// (errorClass), and frozen clock (specClock) are reused verbatim from
// main.go, so a generated row reproduces here byte-for-byte.

// The matrix grows as len(alphabet)^maxLen, so at length 4 it is ~137k
// rows. Each gate therefore fans its rows across NumCPU workers (every
// row builds an isolated lang/registry instance with no shared mutable
// state). Workers report via
// failSink (goroutine-safe t.Errorf, capped to avoid flooding output on
// a systemic regression); they must never call t.Fatalf / t.FailNow.

// failSink funnels worker-goroutine failures to t.Errorf, capping the
// number actually printed while counting them all.
type failSink struct {
	t   *testing.T
	n   int64
	max int64
}

func (s *failSink) fail(format string, args ...any) {
	if atomic.AddInt64(&s.n, 1) <= s.max {
		s.t.Errorf(format, args...)
	}
}

func (s *failSink) total() int64 { return atomic.LoadInt64(&s.n) }

// report logs the failure tally and notes any suppressed-from-print
// overflow so the count is never silently hidden.
func (s *failSink) report() {
	if n := s.total(); n > s.max {
		s.t.Errorf("%d failures total; %d shown above, %d suppressed", n, s.max, n-s.max)
	}
}

// forEachRow runs work on every row across NumCPU worker goroutines.
func forEachRow(rows []matrixRow, work func(r matrixRow)) {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	ch := make(chan matrixRow, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range ch {
				work(r)
			}
		}()
	}
	for _, r := range rows {
		ch <- r
	}
	close(ch)
	wg.Wait()
}

// TestSyntaxMatrixInterpreter re-evaluates every row and asserts the
// interpreter reproduces the recorded expectation exactly — the frozen
// deterministic snapshot.
func TestSyntaxMatrixInterpreter(t *testing.T) {
	rows := readMatrix(t)
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		out, err := run(r.input)
		var got string
		if err != nil {
			got = errorClass(err)
		} else {
			got = core.Canon(out)
		}
		if got != r.expected {
			sink.fail("L%d %q: interpreter gave %q, spec records %q", r.line, r.input, got, r.expected)
		}
	})
	sink.report()
	t.Logf("interpreter: %d rows, %d mismatches", len(rows), sink.total())
}

// Checker-gate sampling. The checker is the costliest path (~1.7ms per
// Check even with instance reuse), so at length 4 an exhaustive
// twice-over-everything pass is ~7min — too slow for `make test`. The
// gate is therefore scoped to what each stride controls:
//
//   - longNoPanicStride: length-4 rows get the no-panic + classification
//     check on every Nth row; length<=3 rows (all 3-grams and shorter)
//     are ALWAYS checked. So no-panic coverage is exhaustive through
//     length 3 and a 1/8 sample at length 4 — and the length-4 space is
//     already covered exhaustively by the interpreter gate, which is
//     where row-specific result bugs would surface anyway.
//   - determinismStride: among the rows it checks, the gate re-runs a
//     ~1/64 sample a second time and compares. Determinism is a global
//     property, not per-row, so a sparse but diverse sample suffices; on
//     a reused instance the re-run also asserts check idempotency (state
//     accumulation would diverge on the second pass).
const (
	longNoPanicStride = 8
	determinismStride = 64
)

// TestSyntaxMatrixCheckerDeterministic asserts the type checker never
// panics and is deterministic, over the sampled scope described above.
// It does not gate on accuracy — the curated false-positive / soundness
// ratchets live in the langspec suite — only on the trust properties
// this matrix is responsible for: no-crash and determinism.
//
// Because the alphabet contains NO state-mutating word (no
// def/set/import/macro/var — every atom is a literal, a stack shuffle,
// or a pure operator), a check leaves the registry untouched, so each
// worker reuses ONE checker instance across all its rows (this is what
// makes the gate affordable; the determinism re-run guards the reuse).
func TestSyntaxMatrixCheckerDeterministic(t *testing.T) {
	rows := readMatrix(t)
	var checked, clean, flagged, sampled int64
	sink := &failSink{t: t, max: 50}

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	ch := make(chan matrixRow, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := lang.New()
			if err != nil {
				sink.fail("lang.New: %v", err)
				return
			}
			a.SetClock(specClock)
			for r := range ch {
				if r.n >= 4 && r.line%longNoPanicStride != 0 {
					continue // length-4 no-panic coverage is sampled
				}
				atomic.AddInt64(&checked, 1)
				fa := checkWith(sink, a, r.input) // no-panic + classify
				if strings.HasPrefix(fa, "errors=0 ") {
					atomic.AddInt64(&clean, 1)
				} else {
					atomic.AddInt64(&flagged, 1)
				}
				if r.line%determinismStride == 0 {
					atomic.AddInt64(&sampled, 1)
					if fb := checkWith(sink, a, r.input); fa != fb {
						sink.fail("L%d %q: checker non-deterministic:\n  run1=%s\n  run2=%s", r.line, r.input, fa, fb)
					}
				}
			}
		}()
	}
	for _, r := range rows {
		ch <- r
	}
	close(ch)
	wg.Wait()

	sink.report()
	t.Logf("checker: %d/%d rows checked (%d clean, %d flagged), %d determinism-sampled, %d failures",
		checked, len(rows), clean, flagged, sampled, sink.total())
}

// checkWith runs one row through the supplied checker instance and
// renders a stable fingerprint (error count + residual stack types) used
// to compare two runs for determinism. A panic is recovered into a
// sentinel so the no-panic property is asserted as a normal test
// failure, never a crash.
func checkWith(sink *failSink, a *lang.Boru, input string) (fp string) {
	defer func() {
		if rec := recover(); rec != nil {
			sink.fail("checker PANICKED on %q: %v", input, rec)
			fp = fmt.Sprintf("PANIC:%v", rec)
		}
	}()
	res, err := a.Check(input)
	if err != nil {
		return "parse-or-run-error:" + errorClass(err)
	}
	return fmt.Sprintf("errors=%d warnings=%d [%s]", res.Summary.Errors, res.Summary.Warnings, strings.Join(res.Stack, " "))
}

// hasCheckError reports whether a Check result carries any
// error-severity diagnostic (the checker rejected the program).
func hasCheckError(res lang.CheckResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			return true
		}
	}
	return false
}

// TestSyntaxMatrixFailFrontier re-verifies the two minimal-failing-prefix
// files of every frontier set (the full length-1..4 separation, the len123
// length-1..3 separation and the length-5 layer).
//
//   - Every fail-check row must be REJECTED BY THE CHECKER (a parse/run
//     error or an error-severity diagnostic).
//   - Every fail-runtime row must PASS THE CHECKER, yet ERROR when
//     interpreted — a runtime fault the static stage cannot see, disjoint
//     from the check-fail set.
//
// Counts are floored so no file can silently collapse.
func TestSyntaxMatrixFailFrontier(t *testing.T) {
	for _, set := range frontierSets {
		t.Run(set.label, func(t *testing.T) { verifyFailFrontier(t, set) })
	}
}

func verifyFailFrontier(t *testing.T, set frontierSet) {
	checkRows := readRows(t, set.failCheck)
	runtimeRows := readRows(t, set.failRuntime)

	// Floors gate the FULL row counts; disposition is re-derived on a
	// sample (every row unless the file is huge — see frontierSet.sample).
	checkN, runtimeN := len(checkRows), len(runtimeRows)
	checkRows = sampleRows(checkRows, set.sample)
	runtimeRows = sampleRows(runtimeRows, set.sample)

	// fail-check: the checker rejects.
	var checkVerified int64
	csink := &failSink{t: t, max: 50}
	forEachRow(checkRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			csink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		res, errc := a.Check(r.input)
		if errc == nil && !hasCheckError(res) {
			csink.fail("L%d %q: in fail-check but the checker did NOT reject it", r.line, r.input)
			return
		}
		atomic.AddInt64(&checkVerified, 1)
	})
	csink.report()

	// fail-runtime: checker clean, yet errors when interpreted.
	var runtimeVerified int64
	rsink := &failSink{t: t, max: 50}
	forEachRow(runtimeRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			rsink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		res, errc := a.Check(r.input)
		if errc != nil || hasCheckError(res) {
			rsink.fail("L%d %q: in fail-runtime but the checker rejected it (belongs in fail-check)", r.line, r.input)
			return
		}
		if _, errRun := a.RunInterp(r.input); errRun == nil {
			rsink.fail("L%d %q: in fail-runtime but it ran without error", r.line, r.input)
			return
		}
		atomic.AddInt64(&runtimeVerified, 1)
	})
	rsink.report()

	t.Logf("fail-check: %d/%d sampled rows verified (file has %d)", checkVerified, len(checkRows), checkN)
	t.Logf("fail-runtime: %d/%d sampled rows verified (file has %d)", runtimeVerified, len(runtimeRows), runtimeN)

	// Floors are on the FULL file row counts, so a shrunk file is caught
	// even though disposition is only re-derived on a sample.
	if checkN < set.minCheck {
		t.Errorf("%s fail-check has %d rows (floor %d) — the file shrank", set.label, checkN, set.minCheck)
	}
	if runtimeN < set.minRuntime {
		t.Errorf("%s fail-runtime has %d rows (floor %d) — the file shrank", set.label, runtimeN, set.minRuntime)
	}
}

// readInputsTolerant reads just the input (first) column of a generated
// tsv, tolerating an empty file (unlike readRows, which fatals on zero
// data rows — a bucket may legitimately be empty).
func readInputsTolerant(t *testing.T, path string) []string {
	t.Helper()
	var out []string
	err := forEachDataRow(path, func(input, expected, note string, line int) {
		out = append(out, input)
	})
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with `make spec-gen`)", path, err)
	}
	return out
}

// TestSyntaxMatrixFileStructure is the cheap (no-engine) structural guard
// over EVERY generated tsv: it catches composition regressions that the
// disposition tests (which sample the huge files) could miss.
//
//   - No file contains a duplicate input — the dedup the whole effort is
//     built on.
//   - Within each length window the buckets are pairwise DISJOINT: no
//     combination is filed under two dispositions.
//   - The length-5 layer's three buckets (passing + 2 fails) PARTITION
//     the candidate set exactly — their counts sum to
//     (#length-4 passing rows) × (#alphabet atoms), with nothing dropped
//     or double-counted.
func TestSyntaxMatrixFileStructure(t *testing.T) {
	files := []string{
		matrixPath,
		fullSet.passing, fullSet.failCheck, fullSet.failRuntime,
		len123Set.passing, len123Set.failCheck, len123Set.failRuntime,
		len5Set.passing, len5Set.failCheck, len5Set.failRuntime,
	}
	inputs := make(map[string][]string, len(files))
	for _, f := range files {
		in := readInputsTolerant(t, f)
		inputs[f] = in
		seen := make(map[string]struct{}, len(in))
		dups := 0
		for _, s := range in {
			if _, ok := seen[s]; ok {
				dups++
			} else {
				seen[s] = struct{}{}
			}
		}
		if dups != 0 {
			t.Errorf("%s: %d duplicate input(s) — dedup regressed", f, dups)
		}
	}

	disjoint := func(label string, files ...string) {
		owner := map[string]string{}
		for _, f := range files {
			for _, s := range inputs[f] {
				if prev, ok := owner[s]; ok {
					t.Errorf("%s: %q is filed in BOTH %s and %s", label, s, prev, f)
				} else {
					owner[s] = f
				}
			}
		}
	}
	disjoint("full", fullSet.passing, fullSet.failCheck, fullSet.failRuntime)
	disjoint("len123", len123Set.passing, len123Set.failCheck, len123Set.failRuntime)

	len5Files := []string{len5Set.passing, len5Set.failCheck, len5Set.failRuntime}
	disjoint("len5", len5Files...)

	// The length-5 layer is a true partition of the candidate set:
	// (#length-4 passing rows) × (#alphabet atoms). Compute the expected
	// total from the live file sizes so this stays correct if the passing
	// set changes.
	len4Passing := len(inputs[fullSet.passing]) - len(inputs[len123Set.passing])
	want := len4Passing * len(alphabet)
	sum := 0
	for _, f := range len5Files {
		sum += len(inputs[f])
	}
	if sum != want {
		t.Errorf("len5 buckets sum to %d, want %d (= %d length-4 passing × %d atoms) — the partition is incomplete or overlapping",
			sum, want, len4Passing, len(alphabet))
	}
	t.Logf("structure OK: %d files, no dup inputs, buckets disjoint, len5 partition %d = %d×%d",
		len(files), sum, len4Passing, len(alphabet))
}

// TestSyntaxMatrixPassing re-verifies each curated passing subset (the
// full length-1..4 file and the len123 length-1..3 file): every row must
// clear BOTH pipelines — interpret to the recorded value and type-check
// with zero errors. This is the invariant the extractor (specgen -extract)
// builds the files on; the test is their guard, so they can't silently
// drift out of agreement with the engine. None of these rows is an error
// row (an error row can never pass interpret), and each count is floored
// so the subset can't collapse to empty unnoticed.
func TestSyntaxMatrixPassing(t *testing.T) {
	for _, set := range frontierSets {
		t.Run(set.label, func(t *testing.T) { verifyPassing(t, set) })
	}
}

func verifyPassing(t *testing.T, set frontierSet) {
	rows := readRows(t, set.passing)
	total := len(rows)
	rows = sampleRows(rows, set.sample) // floor gates the full count; disposition on a sample
	var verified int64
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		if strings.HasPrefix(r.expected, "ERROR:") {
			sink.fail("L%d %q: error row present in the passing subset", r.line, r.input)
			return
		}
		// 1. interpret → recorded canonical value (the frozen result).
		out, err := run(r.input)
		if err != nil {
			sink.fail("L%d %q: passing row failed to interpret: %v", r.line, r.input, err)
			return
		}
		if got := core.Canon(out); got != r.expected {
			sink.fail("L%d %q: interpreter gave %q, passing row records %q", r.line, r.input, got, r.expected)
			return
		}
		// 2. check → zero error-severity diagnostics.
		a, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		res, errChk := a.Check(r.input)
		if errChk != nil {
			sink.fail("L%d %q: passing row failed to check: %v", r.line, r.input, errChk)
			return
		}
		if res.Summary.Errors != 0 {
			sink.fail("L%d %q: passing row has %d checker error(s)", r.line, r.input, res.Summary.Errors)
			return
		}
		atomic.AddInt64(&verified, 1)
	})
	sink.report()
	t.Logf("passing subset %s: %d/%d sampled rows verified through interpret+check (file has %d), %d failures",
		set.label, verified, len(rows), total, sink.total())
	if total < set.minPassing {
		t.Errorf("%s passing has %d rows (floor %d) — the subset shrank or the extractor regressed",
			set.label, total, set.minPassing)
	}
}
