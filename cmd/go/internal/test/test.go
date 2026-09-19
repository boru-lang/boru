// Package test implements `boru test` — discover *_test.boru suites, compile
// and run each one, print boru:test's per-case report, and exit non-zero if
// any case failed, any file errored, or any file did not compile. A suite
// that does not compile is an error like any other: there is no second engine
// to run it on.
//
// With --coverage the runner arms the engine's line-coverage hook BEFORE each
// file runs, so a `*_test.boru` that imports a user module (`import "./mod.boru"`)
// records that module's executed source rows (feature C tags the module because
// the hook is armed at import time). After the run it reports each imported
// module's line coverage.
//
// Coverage is measured on the compiled run, because that is the only run.
// The bytecode VM folds some source positions (e.g. a trailing bare-word
// return), so a folded row reads as uncovered where the interpreter would
// have recorded it — the report UNDERSTATES coverage rather than overstating
// it, which is the safe direction for a number you gate on. Each folded
// position is a position the VM should be carrying, and closing them is
// compiler work, not a reason to measure somewhere else.
package test

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	"github.com/boru-lang/boru/cmd/go/internal/run"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
)

// walkDir and newBoru are test seams (design/TEST-SEAMS.10.md): filepath.WalkDir
// never surfaces a read error under the test's root uid, and lang.New's only
// error path (registry init) is unreachable in a healthy build, so both
// error arms are driven by swapping these in a test.
var (
	walkDir = filepath.WalkDir
	newBoru = lang.New
)

type cmd struct{}

// New returns the test subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "test" }
func (*cmd) Synopsis() string { return "discover and run *_test.boru suites (compiled by default)" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	registry := fs.String("r", "", "registry path")
	coverage := fs.Bool("coverage", false, "measure imported-user-module line coverage; print a summary and write an HTML report")
	coverageDir := fs.String("coverage-dir", "coverage", "directory for the HTML coverage report written with --coverage")
	coverageMin := fs.Float64("coverage-min", 0, "fail the run (exit 1) when aggregate line coverage is below this percentage; implies coverage measurement")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)
	if err := fs.Parse(args); err != nil {
		return 1
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	files, err := discover(targets)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "no *_test.boru files found")
		return 1
	}

	o := run.OptionsFor(pathutil.Expand(*registry), 0, pol)

	// A --coverage-min threshold implies coverage measurement even without
	// --coverage (you can't gate on a number you don't measure).
	var accum *covAccum
	if *coverage || *coverageMin > 0 {
		accum = newCovAccum()
	}
	var passed, failed int
	anyErr := false
	for _, f := range files {
		p, fl, errored := runFile(stdout, stderr, f, o, accum)
		passed += p
		failed += fl
		if errored {
			anyErr = true
		}
	}
	fmt.Fprintf(stdout, "\n=== %d files: %d passed, %d failed ===\n", len(files), passed, failed)
	belowMin := false
	if accum != nil {
		accum.report(stdout)
		if *coverage {
			index, err := accum.writeHTML(*coverageDir)
			if err != nil {
				fmt.Fprintf(stderr, "warning: coverage report: %s\n", err)
			} else if index != "" {
				fmt.Fprintf(stdout, "coverage report: %s\n", index)
			}
		}
		if *coverageMin > 0 {
			cov, total := accum.aggregate()
			if pct := percent(cov, total); pct < *coverageMin {
				fmt.Fprintf(stderr, "coverage %.1f%% is below the required %.1f%% (--coverage-min)\n", pct, *coverageMin)
				belowMin = true
			}
		}
	}
	if failed > 0 || anyErr || belowMin {
		return 1
	}
	return 0
}

// discover expands each target into test files: a file target is taken
// verbatim (run even if it lacks the _test.boru suffix — an explicit request),
// a directory is walked recursively for *_test.boru. Results are de-duplicated
// and sorted for a stable run order. A target that does not exist is an error.
func discover(targets []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, t := range targets {
		t = pathutil.Expand(t)
		info, err := os.Stat(t)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(t)
			continue
		}
		if err := walkDir(t, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, "_test.boru") {
				add(path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// runFile runs one suite and prints its report. When accum is non-nil the run
// is coverage-armed and the suite's imported-module coverage is folded into it.
// It returns the suite's passed and failed case counts, plus an errored flag set
// when the file could not be read, initialised, run, or its results read — an
// errored suite forces a non-zero final exit.
//
// A suite that errors PART WAY THROUGH still reports the cases it completed:
// the framework's tally lives on the instance and survives the error, so it is
// read back rather than discarded. Reporting 0/0 for a file whose first ten
// cases passed made the summary actively misleading — worse, a file of purely
// passing cases followed by one stray error was indistinguishable from an
// empty file. Only a suite that errored before boru:test loaded counts nothing,
// because there is then no tally to read.
func runFile(stdout, stderr io.Writer, path string, o lang.Options, accum *covAccum) (passed, failed int, errored bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: %s\n", path, err)
		return 0, 0, true
	}
	a, err := newBoru(o)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: init: %s\n", path, err)
		return 0, 0, true
	}
	// Route the framework's streams to the runner's writers: a test body's
	// `print` and boru:test's loud per-case `FAIL` line go to the runner's
	// stdout/stderr rather than leaking to os.Stderr.
	a.SetOutput(stdout)
	a.NativeRegistry().ErrOutput = stderr
	if accum != nil {
		disarm := modules.ArmCoverageCollector(a.NativeRegistry())
		defer disarm()
	}
	fmt.Fprintf(stdout, "# %s\n", path)
	if runErr := runSource(a, string(data)); runErr != nil {
		// An `IO.exit` inside a suite ends THAT FILE, not the test run: it
		// stops the file's remaining cases exactly as any other raise does,
		// and the cases already recorded are still salvaged and reported
		// below. It is not this file's process status — a suite that exits
		// 0 half-way has not passed the cases it never reached, and the
		// runner's own exit code stays "did every case pass".
		if code, isExit := lang.ExitCode(runErr); isExit {
			fmt.Fprintf(stderr, "error: %s: exited with code %d before the suite finished\n", path, code)
		} else {
			fmt.Fprintf(stderr, "error: %s: %s\n", path, runErr)
		}
		// Salvage whatever the framework tallied before the error. A read
		// failure here is the ordinary case for a suite that died before
		// `import "boru:test"` — Test.summary is simply not a word yet — so
		// it is not reported as a second error on top of the first.
		_, passed, failed, report, readErr := readOutcome(a)
		if readErr != nil {
			return 0, 0, true
		}
		fmt.Fprintln(stdout, report)
		return passed, failed, true
	}
	_, passed, failed, report, err := readOutcome(a)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s: reading results: %s\n", path, err)
		return 0, 0, true
	}
	fmt.Fprintln(stdout, report)
	if accum != nil {
		accum.collect(a.NativeRegistry())
	}
	return passed, failed, false
}

// runSource executes src on the instance. There is one engine: the suite
// compiles and its bytecode runs, or it does not compile and that is an
// error. A recorded test FAILURE does not error the run (the framework
// records it and continues); only a genuine runtime, parse or compile error
// does.
func runSource(a *lang.Boru, src string) error {
	_, err := a.Run(src)
	return err
}

// readOutcome reads boru:test's accumulated results off the (already-run)
// instance: `Test.summary Test.report` leaves a {total,passed,failed} map and
// the human report string on the stack in that order. The helpers below skip
// whichever value they don't consume, so one read yields both.
func readOutcome(a *lang.Boru) (total, passed, failed int, report string, err error) {
	vals, err := a.RunInterpValues("Test.summary Test.report")
	if err != nil {
		return 0, 0, 0, "", err
	}
	total, passed, failed = tallyFromSummary(vals)
	report = stringFromStack(vals)
	return total, passed, failed, report, nil
}

// tallyFromSummary extracts {total,passed,failed} from the first concrete map
// on a residual stack, tolerating non-map values (skipped) so a malformed read
// never panics.
func tallyFromSummary(vals []native.Value) (total, passed, failed int) {
	for _, v := range vals {
		m, err := native.AsMap(v)
		if err != nil {
			continue
		}
		total = mapInt(m, "total")
		passed = mapInt(m, "passed")
		failed = mapInt(m, "failed")
	}
	return total, passed, failed
}

// stringFromStack returns the last string value on a residual stack ("" when
// none), skipping non-string values.
func stringFromStack(vals []native.Value) string {
	out := ""
	for _, v := range vals {
		s, err := native.AsString(v)
		if err != nil {
			continue
		}
		out = s
	}
	return out
}

// mapInt reads an integer field, defaulting to 0 for an absent or non-integer
// value (branch-free: the ignored errors collapse to the zero value).
func mapInt(m native.ReadMap, key string) int {
	v, _ := m.Get(key)
	n, _ := native.AsInteger(v)
	return int(n)
}

// coverageLine renders one module's coverage summary, appending the uncovered
// line numbers when any remain. Pure (no I/O) so both arms are unit-testable.
func coverageLine(id string, covered, total int, uncovered []int) string {
	line := fmt.Sprintf("  cover %s  %.1f%% (%d/%d)", id, percent(covered, total), covered, total)
	if len(uncovered) > 0 {
		line += "  uncovered: " + joinInts(uncovered)
	}
	return line + "\n"
}

// joinInts renders a row-number list as "3, 6, 7".
func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ", ")
}

// percent is covered/total as a percentage, defined as 100 for an empty
// denominator (a module with no executable rows is vacuously fully covered).
func percent(covered, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(covered) / float64(total) * 100
}
