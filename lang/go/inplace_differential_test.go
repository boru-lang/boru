package lang

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
)

// TestInPlaceDifferential is the in-place compilation prototype's oracle
// (core/go/inplace.go, design/IN-PLACE-COMPILATION.0.md §8): every lang/spec
// row is interpreted with the legacy completion and with in-place
// compilation, and the two runs must agree on the result, the output and the
// rendered error. The legacy lane runs twice first; a row whose two legacy
// runs disagree is nondeterministic (time, randomness, minted IDs) and is
// excluded from the comparison rather than reported. Long — the whole corpus,
// three interpretations per row — so it runs only when asked:
//
//	BORU_INPLACE_DIFF=1 go test -run TestInPlaceDifferential ./lang/go
//
// BORU_SPEC_FILES (comma-separated globs, as test/go/langspec reads it)
// narrows it to some spec files. BORU_INPLACE_DIFF=verify runs the verify
// lane instead and reports the plan/re-plan disagreements it counts.
func TestInPlaceDifferential(t *testing.T) {
	mode := os.Getenv("BORU_INPLACE_DIFF")
	if mode == "" {
		t.Skip("set BORU_INPLACE_DIFF=1 (or verify) to run the in-place differential")
	}
	rows := inPlaceSpecRows(t)
	if mode == "verify" {
		inPlaceVerifyLane(t, rows)
		return
	}
	before := inPlaceCounterSnapshot()
	exemplars := map[string][]string{}
	var diffs, nondet, traceOnly []string
	for _, r := range rows {
		ref := inPlaceRun(r.src, InPlaceForceOff)
		if again := inPlaceRun(r.src, InPlaceForceOff); again != ref {
			nondet = append(nondet, r.key)
			continue
		}
		pre := inPlaceCounterSnapshot()
		got := inPlaceRun(r.src, InPlaceOn)
		noteInPlaceExemplars(exemplars, r.key, inPlaceCounterSnapshot().minus(pre))
		switch {
		case got == ref:
		case inPlaceOutcomeVaries(r.src, got):
			// A random row whose legacy runs happened to agree: the
			// in-place outcome is one a legacy run also produces.
			nondet = append(nondet, r.key)
		case ref.result == got.result && ref.err == got.err && isStepTrace(ref.output):
			// The one observable the design lets differ: a printed step
			// trace shows the new cells (IN-PLACE-COMPILATION.0.md §2,
			// decision 7). Results and errors still have to agree.
			traceOnly = append(traceOnly, r.key)
		default:
			diffs = append(diffs, fmt.Sprintf("%s\n    src:    %s\n    legacy: %s\n    inplace: %s", r.key, r.src, ref, got))
		}
	}
	after := inPlaceCounterSnapshot()
	t.Logf("rows %d, compared %d, nondeterministic (excluded) %d, divergent %d, step-trace output only %d %v",
		len(rows), len(rows)-len(nondet), len(nondet), len(diffs), len(traceOnly), traceOnly)
	t.Logf("in-place counters over the run: %s", after.minus(before))
	names := make([]string, 0, len(exemplars))
	for k := range exemplars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		t.Logf("exemplars %-22s %v", k, exemplars[k])
	}
	for _, d := range diffs {
		t.Errorf("in-place diverges from legacy: %s", d)
	}
}

// TestInPlaceRealPrograms runs the repository's real boru programs — the
// knowledge-graph pipeline's suites (kg/tests) and the coreutils clones'
// (utils/tests), 1,213 boru:test cases between them — interpreted with the
// legacy completion and with in-place compilation, each from its own
// directory as its Makefile runs it (the programs read their fixtures
// relative to it), and requires the two runs to agree on the result, the
// output, the error stream, the error and the boru:test report. Each
// program's wall time on both lanes is logged. Opt-in with the differential
// (BORU_INPLACE_DIFF=1), and minutes long: kg's codegraph suite alone builds
// the project graph interpreted.
func TestInPlaceRealPrograms(t *testing.T) {
	if os.Getenv("BORU_INPLACE_DIFF") == "" {
		t.Skip("set BORU_INPLACE_DIFF=1 to run the real programs on both lanes")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// A sequential test: no parallel test runs while the directory moves.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	var total [2]time.Duration
	programs := 0
	for _, suite := range []string{"kg", "utils"} {
		dir := filepath.Join(root, suite)
		files, err := filepath.Glob(filepath.Join(dir, "tests", "*_test.boru"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no %s suites (%v)", suite, err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var outs [2]string
			var took [2]time.Duration
			for k, mode := range []InPlaceOption{InPlaceForceOff, InPlaceOn} {
				outs[k], took[k] = inPlaceProgram(t, dir, string(src), mode)
				total[k] += took[k]
			}
			programs++
			name := suite + "/" + filepath.Base(f)
			if outs[0] != outs[1] {
				t.Errorf("%s: in-place diverges from legacy\n--- legacy\n%s\n--- in place\n%s", name, outs[0], outs[1])
			}
			t.Logf("%-32s legacy %9.1fms  in place %9.1fms", name, inPlaceMillis(took[0]), inPlaceMillis(took[1]))
		}
	}
	t.Logf("%d programs: legacy %.1fs, in place %.1fs", programs, total[0].Seconds(), total[1].Seconds())
}

// inPlaceProgram interprets one program on a fresh instance anchored at
// baseDir and renders everything it leaves observable.
func inPlaceProgram(t *testing.T, baseDir, src string, mode InPlaceOption) (string, time.Duration) {
	t.Helper()
	a, err := New(Options{BaseDir: baseDir, InPlace: mode})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a.SetOutput(&out)
	a.NativeRegistry().ErrOutput = &errOut
	start := time.Now()
	res, rerr := a.RunInterp(src)
	took := time.Since(start)
	errText := ""
	if rerr != nil {
		errText = rerr.Error()
	}
	report := "no boru:test report"
	if vals, err := a.RunInterpValues("Test.summary Test.report"); err == nil {
		report = fmt.Sprint(vals)
	}
	return fmt.Sprintf("result %v\noutput %s\nerrors %s\nerror %s\nreport %s", res, out.String(), errOut.String(), errText, report), took
}

func inPlaceMillis(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// inPlaceVerifyLane runs every row on the verify lane and reports where the
// legacy completion's re-plan chose a different signature from the plan.
func inPlaceVerifyLane(t *testing.T, rows []inPlaceRow) {
	core.TakeInPlaceDisagreements()
	before := inPlaceCounterSnapshot()
	byRow := map[string][]core.InPlaceDisagreement{}
	for _, r := range rows {
		inPlaceRun(r.src, InPlaceVerify)
		if ds := core.TakeInPlaceDisagreements(); len(ds) > 0 {
			byRow[r.key] = ds
		}
	}
	after := inPlaceCounterSnapshot().minus(before)
	t.Logf("verify lane over %d rows: %s", len(rows), after)
	keys := make([]string, 0, len(byRow))
	for k := range byRow {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, d := range byRow[k] {
			t.Logf("disagreement %s: %s at %d:%d planned %s, re-planned %q (commit=%v, plan held=%v)", k, d.Name, d.Pos.Row, d.Pos.Col, d.Planned, d.Replanned, d.Commit, d.PlanHeld)
		}
	}
}

type inPlaceRow struct{ key, src string }

// inPlaceSpecRows reads the spec corpus the way the langspec walks do.
func inPlaceSpecRows(t *testing.T) []inPlaceRow {
	t.Helper()
	dir := filepath.Join("..", "spec")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var globs []string
	if f := os.Getenv("BORU_SPEC_FILES"); f != "" {
		globs = strings.Split(f, ",")
	}
	var rows []inPlaceRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") || !inPlaceSelected(e.Name(), globs) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(strings.TrimSuffix(line, "\r"), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			src := strings.TrimSpace(strings.Split(line, "\t")[0])
			rows = append(rows, inPlaceRow{key: fmt.Sprintf("%s:%d", e.Name(), i+1), src: src})
		}
	}
	return rows
}

func inPlaceSelected(name string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	for _, g := range globs {
		if ok, _ := filepath.Match(strings.TrimSpace(g), name); ok {
			return true
		}
	}
	return false
}

// inPlaceOutcome is everything a user can observe of a run.
type inPlaceOutcome struct{ result, output, err string }

func (o inPlaceOutcome) String() string {
	return fmt.Sprintf("result=%s output=%q error=%s", o.result, o.output, o.err)
}

// inPlaceRun interprets src on a fresh instance in the given mode.
func inPlaceRun(src string, mode InPlaceOption) (out inPlaceOutcome) {
	a, err := New(Options{InPlace: mode})
	if err != nil {
		return inPlaceOutcome{err: "new: " + err.Error()}
	}
	var buf bytes.Buffer
	a.SetOutput(&buf)
	defer func() {
		if r := recover(); r != nil {
			out = inPlaceOutcome{err: fmt.Sprintf("panic: %v", r)}
		}
	}()
	res, rerr := a.RunInterp(src)
	out = inPlaceOutcome{result: fmt.Sprintf("%v", res), output: buf.String()}
	if rerr != nil {
		out.err = rerr.Error()
	}
	return out
}

// inPlaceOutcomeVaries reports whether a legacy run of src can produce got —
// a nondeterministic row (randomness, time) whose first two legacy runs
// agreed by chance. A deterministic row's legacy runs never produce a
// different outcome, so a real divergence is never excused.
func inPlaceOutcomeVaries(src string, got inPlaceOutcome) bool {
	for i := 0; i < 6; i++ {
		if inPlaceRun(src, InPlaceForceOff) == got {
			return true
		}
	}
	return false
}

// isStepTrace reports whether output is a printed step trace — IO.trace's
// frame table or the debug stepper's numbered frames — whose cells the
// in-place lane renders differently by design.
func isStepTrace(output string) bool {
	return strings.Contains(output, "─── trace") || stepFrame.MatchString(output)
}

var stepFrame = regexp.MustCompile(`(?m)^\s+\d+\s+\[ .*\^`)

type inPlaceCounts struct {
	calls, completed, stack, normalized, genFallback, replanFallback, compactions, nops, verified, disagreed int64
	normAt                                                                                                   [7]int64
}

// noteInPlaceExemplars keeps the first rows that moved each counter — the
// rows that exercise each in-place path, for the tests that pin them.
func noteInPlaceExemplars(ex map[string][]string, key string, d inPlaceCounts) {
	add := func(name string, n int64) {
		if n > 0 && len(ex[name]) < 4 {
			ex[name] = append(ex[name], key)
		}
	}
	add("rebind fallback", d.genFallback)
	add("re-plan fallback", d.replanFallback)
	add("compaction", d.compactions)
	for i, site := range []string{"norm:commit", "norm:implicit-end", "norm:statement-end", "norm:paren-close", "norm:input-end", "norm:loop-region", "norm:fallback"} {
		add(site, d.normAt[i])
	}
}

func inPlaceCounterSnapshot() inPlaceCounts {
	s := &core.InPlaceStats
	c := inPlaceCounts{calls: s.Calls.Load(), completed: s.Completed.Load(), stack: s.StackCalls.Load(), normalized: s.Normalized.Load(),
		genFallback: s.GenFallback.Load(), replanFallback: s.ReplanFallback.Load(), compactions: s.Compactions.Load(), nops: s.NopsDropped.Load(),
		verified: s.Verified.Load(), disagreed: s.Disagreed.Load()}
	for i := range c.normAt {
		c.normAt[i] = s.NormalizedAt[i].Load()
	}
	return c
}

func (c inPlaceCounts) minus(o inPlaceCounts) inPlaceCounts {
	d := inPlaceCounts{calls: c.calls - o.calls, completed: c.completed - o.completed, stack: c.stack - o.stack, normalized: c.normalized - o.normalized,
		genFallback: c.genFallback - o.genFallback, replanFallback: c.replanFallback - o.replanFallback, compactions: c.compactions - o.compactions,
		nops: c.nops - o.nops, verified: c.verified - o.verified, disagreed: c.disagreed - o.disagreed}
	for i := range d.normAt {
		d.normAt[i] = c.normAt[i] - o.normAt[i]
	}
	return d
}

func (c inPlaceCounts) String() string {
	return fmt.Sprintf("call cells executed %d (forward %d, value-stack %d), normalised %d %v, rebind fallbacks %d, re-plan fallbacks %d, compactions %d, nops dropped %d, verified %d, disagreed %d",
		c.calls, c.completed, c.stack, c.normalized, c.normAt, c.genFallback, c.replanFallback, c.compactions, c.nops, c.verified, c.disagreed)
}
