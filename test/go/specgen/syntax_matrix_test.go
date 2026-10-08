//go:build specgen

package main

// This runner carries the `specgen` build tag so the heavy matrix-replay
// tests are EXCLUDED from the normal unit-test run (`make test` /
// `go test ./...`, which compile this package but find no test files).
// They re-execute the interpreter, checker, and compiler over tens of
// thousands of generated rows (~7 min sampled, ~24 min with
// SPECGEN_FULL=1) — a regression corpus, not a fast unit test. Run them
// explicitly with `make spec-test` (or `go test -tags specgen
// ./specgen/`). The generator (main.go) stays UNtagged so `make
// spec-gen` and `go run ./specgen` keep working without the tag.
//
// Self-contained runner for the generated syntax-combination matrix
// (syntax-matrix.tsv, produced by this package's generator). It pins the
// three deterministic trust properties the matrix exists to defend —
// stated in the package doc comment of main.go:
//
//   1. Interpreter correctness (TestSyntaxMatrixInterpreter): every row
//      re-evaluates to the recorded canonical value, or to the recorded
//      error class. This is the frozen snapshot — if the interpreter's
//      output for any of the ~7k sequences drifts, this fails. (Identity
//      with the generator's own recipe is the point: the spec is exactly
//      what the interpreter produces, captured deterministically.)
//
//   2. Compiler parity (TestSyntaxMatrixCompilerParity): every non-error
//      row the bytecode compiler accepts must produce a result IDENTICAL
//      to the interpreter. A floor on the accepted-row count keeps the
//      gate honest (a compiler that silently declined everything would
//      pass vacuously). This is the same differential contract the
//      langspec suite enforces, applied to the whole combination space.
//
//   3. Checker determinism (TestSyntaxMatrixCheckerDeterministic): the
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

// frontierSet groups the four derived files for one length window — the
// passing subset plus the three minimal-failing-prefix buckets — with the
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
	label                                        string
	passing, failCheck, failCompile, failRuntime string
	minPassing, minCheck, minCompile, minRuntime int
	sample                                       int
}

var (
	// fullSet — the length-1..4 separation. Counts measured at 29798 /
	// 10015 / 2056 / 190 (after the multi-output stack-word compiler fix:
	// `dup`/`swap` results above a residual literal now compile, moving
	// compile→passing; and the none-ordering soundness fix moved the
	// remaining cross-family ordering rows runtime→check); floors pinned
	// just below as headroom.
	fullSet = frontierSet{
		label:       "full(len1-4)",
		passing:     "syntax-matrix-passing.tsv",
		failCheck:   "syntax-matrix-fail-check.tsv",
		failCompile: "syntax-matrix-fail-compile.tsv",
		failRuntime: "syntax-matrix-fail-runtime.tsv",
		minPassing:  29000, minCheck: 9800, minCompile: 2000, minRuntime: 180,
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
		failCompile: "syntax-matrix-len123-fail-compile.tsv",
		failRuntime: "syntax-matrix-len123-fail-runtime.tsv",
		minPassing:  2000, minCheck: 580, minCompile: 50, minRuntime: 0,
		sample: 0, // every row
	}
	// len5Set — the length-5 layer (length-4 passing rows × one atom).
	// Counts measured at 375549 / 104662 / 41492 / 5395 (after the
	// multi-output stack-word compiler fix moved ~19k `dup`/`swap`-above-
	// literal rows compile→passing, and the none-ordering soundness fix
	// moved ~2.6k rows runtime→check); compiler-mismatch stays 0 across all
	// 527,098 length-5 programs. Floors pinned below. Sampled by default
	// (these files are large); SPECGEN_FULL=1 forces exhaustive
	// verification.
	len5Set = frontierSet{
		label:       "len5",
		passing:     "syntax-matrix-len5-passing.tsv",
		failCheck:   "syntax-matrix-len5-fail-check.tsv",
		failCompile: "syntax-matrix-len5-fail-compile.tsv",
		failRuntime: "syntax-matrix-len5-fail-runtime.tsv",
		minPassing:  374000, minCheck: 104000, minCompile: 41000, minRuntime: 5300,
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

// minCompiledRows is the floor for the compiler-parity gate: at least
// this many non-error rows must take the bytecode-compiled path, or the
// parity assertion is vacuous (a compiler that declined everything would
// pass with zero comparisons). Measured at 29798 over the length-4 matrix
// (up from 27712 once the multi-output stack-word fix let `dup`/`swap`
// results above a residual literal compile); pinned a little below the
// live count as headroom against incidental jitter. RAISE it when the
// compilable subset widens; only lower it with a documented reason.
const minCompiledRows = 29000

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
// state — the same concurrency the langspec suite's
// TestSpecCompiledConcurrentRowsRaceFree relies on). Workers report via
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

// TestSyntaxMatrixCompilerParity asserts the bytecode compiler agrees
// with the interpreter on every non-error row it accepts.
func TestSyntaxMatrixCompilerParity(t *testing.T) {
	rows := readMatrix(t)
	var compiled, errorRows int64
	sink := &failSink{t: t, max: 50}
	forEachRow(rows, func(r matrixRow) {
		if strings.HasPrefix(r.expected, "ERROR:") {
			atomic.AddInt64(&errorRows, 1)
			return
		}

		ac, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		ac.SetClock(specClock)
		gotC, wasCompiled, errC := ac.RunCompiled(r.input)
		if !wasCompiled {
			return // outside the compilable subset — compile failure covers it
		}
		atomic.AddInt64(&compiled, 1)

		ai, err := lang.New()
		if err != nil {
			sink.fail("lang.New: %v", err)
			return
		}
		ai.SetClock(specClock)
		gotI, errI := ai.RunInterp(r.input)

		if (errC != nil) != (errI != nil) {
			sink.fail("L%d %q: error divergence compiled=%v interpreted=%v", r.line, r.input, errC, errI)
			return
		}
		if errC != nil {
			return
		}
		if renderAny(gotC) != renderAny(gotI) {
			sink.fail("L%d %q: compiled=%q interpreted=%q", r.line, r.input, renderAny(gotC), renderAny(gotI))
		}
	})
	sink.report()
	t.Logf("compiler parity: %d rows compiled (%d error rows skipped), %d mismatches", compiled, errorRows, sink.total())
	if compiled < minCompiledRows {
		t.Errorf("only %d rows took the compiled path (floor %d) — the compiler regressed to declining the matrix",
			compiled, minCompiledRows)
	}
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
//     already covered exhaustively by the interpreter and compiler gates,
//     which is where row-specific result bugs would surface anyway.
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

// hasCheckError reports whether a CompileCheck/Check result carries any
// error-severity diagnostic (the checker rejected the program).
func hasCheckError(res lang.CheckResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			return true
		}
	}
	return false
}

// TestSyntaxMatrixFailFrontier re-verifies the three minimal-failing-
// prefix files of every frontier set (the full length-1..4 separation and
// the len123 length-1..3 separation) and, with them, the "compiler runs
// the checker first" contract.
//
//   - Every fail-check row must be REJECTED BY THE CHECKER (a parse/run
//     error or an error-severity diagnostic) AND the compiler must emit NO
//     Program for it. The second clause is the contract: a program the
//     checker rejects is never lowered to bytecode, because CompileCheck
//     runs the checker first and declines on any error diagnostic. Any
//     check-rejected prefix that still produced a Program is a violation.
//   - Every fail-compile row must PASS THE CHECKER (no error diagnostic)
//     yet still produce NO Program — a genuine compile-stage compile failure,
//     disjoint from the check-fail set.
//   - Every fail-runtime row must PASS THE CHECKER AND COMPILE to a
//     Program, yet ERROR when interpreted — a runtime fault the static
//     stages cannot see, disjoint from both other sets.
//
// Counts are floored so no file can silently collapse.
func TestSyntaxMatrixFailFrontier(t *testing.T) {
	for _, set := range frontierSets {
		t.Run(set.label, func(t *testing.T) { verifyFailFrontier(t, set) })
	}
}

func verifyFailFrontier(t *testing.T, set frontierSet) {
	checkRows := readRows(t, set.failCheck)
	compileRows := readRows(t, set.failCompile)
	runtimeRows := readRows(t, set.failRuntime)

	// Floors gate the FULL row counts; disposition is re-derived on a
	// sample (every row unless the file is huge — see frontierSet.sample).
	checkN, compileN, runtimeN := len(checkRows), len(compileRows), len(runtimeRows)
	checkRows = sampleRows(checkRows, set.sample)
	compileRows = sampleRows(compileRows, set.sample)
	runtimeRows = sampleRows(runtimeRows, set.sample)

	// fail-check: checker rejects, and the compiler emits no Program.
	var checkVerified, compiledDespiteReject int64
	csink := &failSink{t: t, max: 50}
	forEachRow(checkRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			csink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc == nil && !hasCheckError(res) {
			csink.fail("L%d %q: in fail-check but the checker did NOT reject it", r.line, r.input)
			return
		}
		if prog != nil {
			atomic.AddInt64(&compiledDespiteReject, 1)
			csink.fail("L%d %q: checker rejected it yet a Program was emitted — compiler did not run the checker first", r.line, r.input)
			return
		}
		atomic.AddInt64(&checkVerified, 1)
	})
	csink.report()

	// fail-compile: checker clean, but the compiler still emits no Program.
	var compileVerified int64
	psink := &failSink{t: t, max: 50}
	forEachRow(compileRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			psink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc != nil || hasCheckError(res) {
			psink.fail("L%d %q: in fail-compile but the checker rejected it (belongs in fail-check)", r.line, r.input)
			return
		}
		if prog != nil {
			psink.fail("L%d %q: in fail-compile but it DID compile to a Program", r.line, r.input)
			return
		}
		atomic.AddInt64(&compileVerified, 1)
	})
	psink.report()

	// fail-runtime: checker clean AND compiles, yet errors when interpreted.
	var runtimeVerified int64
	rsink := &failSink{t: t, max: 50}
	forEachRow(runtimeRows, func(r matrixRow) {
		a, err := lang.New()
		if err != nil {
			rsink.fail("lang.New: %v", err)
			return
		}
		a.SetClock(specClock)
		prog, _, res, errc := a.CompileCheck(r.input)
		if errc != nil || hasCheckError(res) {
			rsink.fail("L%d %q: in fail-runtime but the checker rejected it (belongs in fail-check)", r.line, r.input)
			return
		}
		if prog == nil {
			rsink.fail("L%d %q: in fail-runtime but it did NOT compile (belongs in fail-compile)", r.line, r.input)
			return
		}
		if _, errRun := a.RunInterp(r.input); errRun == nil {
			rsink.fail("L%d %q: in fail-runtime but it ran without error", r.line, r.input)
			return
		}
		atomic.AddInt64(&runtimeVerified, 1)
	})
	rsink.report()

	t.Logf("fail-check: %d/%d sampled rows verified (file has %d); checker-runs-first violations: %d",
		checkVerified, len(checkRows), checkN, compiledDespiteReject)
	t.Logf("fail-compile: %d/%d sampled rows verified (file has %d)", compileVerified, len(compileRows), compileN)
	t.Logf("fail-runtime: %d/%d sampled rows verified (file has %d)", runtimeVerified, len(runtimeRows), runtimeN)

	if compiledDespiteReject != 0 {
		t.Errorf("%d check-rejected prefixes still compiled — the compiler did NOT run the checker first", compiledDespiteReject)
	}
	// Floors are on the FULL file row counts, so a shrunk file is caught
	// even though disposition is only re-derived on a sample.
	if checkN < set.minCheck {
		t.Errorf("%s fail-check has %d rows (floor %d) — the file shrank", set.label, checkN, set.minCheck)
	}
	if compileN < set.minCompile {
		t.Errorf("%s fail-compile has %d rows (floor %d) — the file shrank", set.label, compileN, set.minCompile)
	}
	if runtimeN < set.minRuntime {
		t.Errorf("%s fail-runtime has %d rows (floor %d) — the file shrank", set.label, runtimeN, set.minRuntime)
	}
}

const len5MismatchPath = "syntax-matrix-len5-compiler-mismatch.tsv"

// readInputsTolerant reads just the input (first) column of a generated
// tsv, tolerating an empty file (unlike readRows, which fatals on zero
// data rows — the mismatch file legitimately empties once the compiler
// bug is fixed).
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
//   - The length-5 layer's five buckets (passing + 3 fails + mismatch)
//     PARTITION the candidate set exactly — their counts sum to
//     (#length-4 passing rows) × (#alphabet atoms), with nothing dropped
//     or double-counted.
func TestSyntaxMatrixFileStructure(t *testing.T) {
	files := []string{
		matrixPath,
		fullSet.passing, fullSet.failCheck, fullSet.failCompile, fullSet.failRuntime,
		len123Set.passing, len123Set.failCheck, len123Set.failCompile, len123Set.failRuntime,
		len5Set.passing, len5Set.failCheck, len5Set.failCompile, len5Set.failRuntime,
		len5MismatchPath,
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
	disjoint("full", fullSet.passing, fullSet.failCheck, fullSet.failCompile, fullSet.failRuntime)
	disjoint("len123", len123Set.passing, len123Set.failCheck, len123Set.failCompile, len123Set.failRuntime)

	len5Files := []string{len5Set.passing, len5Set.failCheck, len5Set.failCompile, len5Set.failRuntime, len5MismatchPath}
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

// TestSyntaxMatrixLen5Mismatch keeps the length-5 compiler-mismatch file
// honest: every row it records must STILL reproduce a genuine
// compiler/interpreter divergence on a fresh, isolated lang instance
// (via freshDivergence — the same fresh-run check the extender uses).
// These are real compiler bugs the length-5 differential sweep surfaced
// (e.g. `and X false not Y`, where the compiler coerces `and`'s result to
// a boolean instead of preserving the falsy operand). If the file is
// empty — the compiler fixed and the layer regenerated — the test passes
// trivially. The file is small (tens of rows), so this stays cheap.
func TestSyntaxMatrixLen5Mismatch(t *testing.T) {
	const path = len5MismatchPath
	inputs := readInputsTolerant(t, path)

	var stillDiverges int
	for _, in := range inputs {
		if _, real := freshDivergence(in); real {
			stillDiverges++
		} else {
			t.Errorf("%q: recorded as a compiler mismatch but no longer diverges — regenerate the length-5 layer", in)
		}
	}
	t.Logf("len5 compiler-mismatch: %d/%d rows still diverge (compiler bugs)", stillDiverges, len(inputs))
}

// TestSyntaxMatrixPassing re-verifies each curated passing subset (the
// full length-1..4 file and the len123 length-1..3 file): every row must
// clear ALL THREE pipelines — interpret to the recorded value, type-check
// with zero errors, and compile to a result identical to the
// interpreter's. This is the invariant the extractor (specgen -extract)
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
		// 3. compile → accepted AND identical to the interpreter.
		gotC, wasCompiled, errC := a.RunCompiled(r.input)
		if !wasCompiled {
			sink.fail("L%d %q: passing row did not compile", r.line, r.input)
			return
		}
		gotI, errI := a.RunInterp(r.input)
		if errC != nil || errI != nil {
			sink.fail("L%d %q: run error compiled=%v interpreted=%v", r.line, r.input, errC, errI)
			return
		}
		if renderAny(gotC) != renderAny(gotI) {
			sink.fail("L%d %q: compiled=%q interpreted=%q", r.line, r.input, renderAny(gotC), renderAny(gotI))
			return
		}
		atomic.AddInt64(&verified, 1)
	})
	sink.report()
	t.Logf("passing subset %s: %d/%d sampled rows verified through interpret+check+compile (file has %d), %d failures",
		set.label, verified, len(rows), total, sink.total())
	if total < set.minPassing {
		t.Errorf("%s passing has %d rows (floor %d) — the subset shrank or the extractor regressed",
			set.label, total, set.minPassing)
	}
}
