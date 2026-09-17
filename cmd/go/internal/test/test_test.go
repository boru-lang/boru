package test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/run"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// write creates path with content and returns path.
func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runCmd invokes the subcommand with args and returns (exitCode, stdout, stderr).
func runCmd(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := New().Run(args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

const passSuite = "import \"boru:test\"\nTest.test \"ok\" [1 1 Assert.equal]\n"
const failSuite = "import \"boru:test\"\nTest.test \"bad\" [1 2 Assert.equal]\n"

func TestNameSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "test" {
		t.Errorf("Name = %q, want test", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis is empty")
	}
}

func TestRunFlagParseError(t *testing.T) {
	code, _, stderr := runCmd("--nonexistent-flag")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if stderr == "" {
		t.Error("expected flag error on stderr")
	}
}

func TestRunPermsResolveError(t *testing.T) {
	// --perms and --perms-inline are mutually exclusive, so Resolve fails.
	code, _, stderr := runCmd("--perms", "none", "--perms-inline", "{}", ".")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want a resolve error", stderr)
	}
}

func TestRunDiscoverError(t *testing.T) {
	code, _, stderr := runCmd(filepath.Join(t.TempDir(), "does-not-exist"))
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want a stat error", stderr)
	}
}

func TestRunNoFiles(t *testing.T) {
	code, _, stderr := runCmd(t.TempDir())
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no *_test.boru files") {
		t.Errorf("stderr = %q, want no-files message", stderr)
	}
}

// Run with no positional targets defaults to the cwd.
func TestRunDefaultTargetCwd(t *testing.T) {
	t.Chdir(t.TempDir()) // empty cwd → discovers nothing → no-files exit
	code, _, stderr := runCmd()
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no *_test.boru files") {
		t.Errorf("stderr = %q, want no-files message", stderr)
	}
}

func TestRunAllPass(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a_test.boru"), passSuite)
	write(t, filepath.Join(dir, "b_test.boru"), passSuite)
	code, stdout, _ := runCmd(dir)
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "2 files: 2 passed, 0 failed") {
		t.Errorf("stdout = %q, want grand total", stdout)
	}
}

func TestRunFailures(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "x_test.boru"), failSuite)
	code, stdout, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "0 passed, 1 failed") {
		t.Errorf("stdout = %q, want the fail tally", stdout)
	}
	// The framework's loud per-case FAIL line routes to the runner's stderr.
	if !strings.Contains(stderr, "FAIL") {
		t.Errorf("stderr = %q, want the loud FAIL line", stderr)
	}
}

// A file that raises at top level errors the run (not a recorded failure).
func TestRunErroredFile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "boom_test.boru"),
		"import \"boru:test\"\nraise boom \"top-level\"\n")
	code, _, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "error:") || !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the run error", stderr)
	}
}

// A file that runs fine but never imports boru:test can't be read for results.
func TestRunReadResultsError(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "noframework_test.boru"), "1 add 2\n")
	code, _, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "reading results") {
		t.Errorf("stderr = %q, want a results-read error", stderr)
	}
}

// A suite that errors PART WAY THROUGH still reports the cases it completed.
// Discarding them made a file of passing cases plus one stray error report
// "0 passed, 0 failed" — indistinguishable from an empty file.
func TestRunErroredFileKeepsCompletedCases(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "partial_test.boru"),
		"import \"boru:test\"\nTest.test \"ok\" [1 1 Assert.equal]\nraise boom \"after the case\"\n")
	code, stdout, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1 — the error still fails the run", code)
	}
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr = %q, want the run error", stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout = %q, want the completed case counted", stdout)
	}
}

// A suite that errors BEFORE boru:test loads has no tally to salvage, so it
// counts nothing — and the failed read is not piled on as a second error.
func TestRunErroredBeforeFrameworkCountsNothing(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "early_test.boru"), "nosuchword 1 2\n")
	code, stdout, stderr := runCmd(f)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "0 passed, 0 failed") {
		t.Errorf("stdout = %q, want no cases counted", stdout)
	}
	if strings.Contains(stderr, "reading results") {
		t.Errorf("stderr = %q, want no second results-read error", stderr)
	}
}

// coverageFixture writes calc.boru (add2 is exercised, triple is not — its body
// on row 5 stays uncovered → 71.4% coverage) plus a suite importing it, and
// returns their paths.
func coverageFixture(t *testing.T, dir string) (mod, suite string) {
	t.Helper()
	mod = write(t, filepath.Join(dir, "calc.boru"),
		"def add2 fn [[x:Integer] [Integer] [\n  x add 2\n]]\n"+
			"def triple fn [[x:Integer] [Integer] [\n  x mul 3\n]]\n"+
			"export \"Calc\" { add2: add2/v  triple: triple/v }\n")
	suite = write(t, filepath.Join(dir, "calc_test.boru"),
		"import \"boru:test\"\nimport \""+mod+"\"\nTest.test \"add2\" [(Calc.add2 3) 5 Assert.equal]\n")
	return mod, suite
}

func TestRunCoverage(t *testing.T) {
	dir := t.TempDir()
	mod, f := coverageFixture(t, dir)
	htmlDir := filepath.Join(t.TempDir(), "cov")
	// Run on the interpreter so the uncovered set is exact (no VM position
	// folding): add2 is exercised, triple is not, so triple's body (row 5) is
	// the uncovered line.
	code, stdout, _ := runCmd("--coverage", "--coverage-dir", htmlDir, "--no-compile", f)
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "cover "+mod) || !strings.Contains(stdout, "%") {
		t.Errorf("stdout = %q, want a coverage line for the module", stdout)
	}
	if !strings.Contains(stdout, "uncovered: 5") {
		t.Errorf("stdout = %q, want the uncovered line numbers (5)", stdout)
	}
	// The HTML report is written and its path announced.
	index := filepath.Join(htmlDir, "index.html")
	if !strings.Contains(stdout, "coverage report: "+index) {
		t.Errorf("stdout = %q, want the report path", stdout)
	}
	idxHTML, err := os.ReadFile(index)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(idxHTML), mod) || !strings.Contains(string(idxHTML), "mod0.html") {
		t.Errorf("index.html missing the module row:\n%s", idxHTML)
	}
	modHTML, err := os.ReadFile(filepath.Join(htmlDir, "mod0.html"))
	if err != nil {
		t.Fatalf("read mod0.html: %v", err)
	}
	// The module page colours the covered add2 body (row 2) and the uncovered
	// triple body (row 5).
	if !strings.Contains(string(modHTML), "covered") || !strings.Contains(string(modHTML), "uncovered") {
		t.Errorf("mod0.html missing coverage classes:\n%s", modHTML)
	}
}

// --coverage-min fails the run when aggregate coverage is below the threshold,
// even though every test passed, and (alone) writes no HTML report.
func TestRunCoverageMinFail(t *testing.T) {
	dir := t.TempDir()
	_, f := coverageFixture(t, dir) // 71.4% coverage
	code, stdout, stderr := runCmd("--coverage-min", "90", "--no-compile", f)
	if code != 1 {
		t.Errorf("code = %d, want 1 (coverage below minimum)", code)
	}
	if !strings.Contains(stderr, "below the required 90.0%") {
		t.Errorf("stderr = %q, want the coverage-min message", stderr)
	}
	// The summary still prints, but no HTML report is written without --coverage.
	if !strings.Contains(stdout, "cover ") {
		t.Errorf("stdout = %q, want the coverage summary", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "coverage")); !os.IsNotExist(err) {
		t.Error("--coverage-min alone must not write an HTML report")
	}
}

// --coverage-min at or below the actual coverage passes.
func TestRunCoverageMinPass(t *testing.T) {
	dir := t.TempDir()
	_, f := coverageFixture(t, dir) // 71.4% coverage
	code, stdout, stderr := runCmd("--coverage-min", "50", "--no-compile", f)
	if code != 0 {
		t.Errorf("code = %d, want 0 (coverage meets minimum)", code)
	}
	if strings.Contains(stderr, "below the required") {
		t.Errorf("stderr = %q, want no coverage-min failure", stderr)
	}
	if !strings.Contains(stdout, "cover ") {
		t.Errorf("stdout = %q, want the coverage summary", stdout)
	}
}

// A --coverage run whose suites import no user module writes no report.
func TestRunCoverageNoModules(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "p_test.boru"), passSuite)
	htmlDir := filepath.Join(t.TempDir(), "cov")
	code, stdout, _ := runCmd("--coverage", "--coverage-dir", htmlDir, f)
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if strings.Contains(stdout, "coverage report:") {
		t.Errorf("stdout = %q, want no report path (no user modules)", stdout)
	}
	if _, err := os.Stat(htmlDir); !os.IsNotExist(err) {
		t.Errorf("coverage dir should not be created when nothing was covered")
	}
}

// coverageLine appends uncovered row numbers only when some remain.
func TestCoverageLine(t *testing.T) {
	full := coverageLine("m.boru", 4, 4, nil)
	if strings.Contains(full, "uncovered") {
		t.Errorf("fully-covered line = %q, want no uncovered suffix", full)
	}
	partial := coverageLine("m.boru", 2, 4, []int{3, 6})
	if !strings.Contains(partial, "uncovered: 3, 6") {
		t.Errorf("partial line = %q, want uncovered: 3, 6", partial)
	}
}

func TestRunNoCompile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "n_test.boru"), passSuite)
	if code, _, _ := runCmd("--no-compile", f); code != 0 {
		t.Errorf("--no-compile code = %d, want 0", code)
	}
}

func TestRunForceCompile(t *testing.T) {
	f := write(t, filepath.Join(t.TempDir(), "fc_test.boru"), passSuite)
	if code, _, _ := runCmd("--force-compile", f); code != 0 {
		t.Errorf("--force-compile code = %d, want 0", code)
	}
}

// discover: explicit files are added verbatim (even without the suffix) and
// de-duplicated; a directory is walked for *_test.boru.
func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	a := write(t, filepath.Join(dir, "a_test.boru"), passSuite)
	write(t, filepath.Join(dir, "not-a-suite.boru"), passSuite) // ignored by walk
	plain := write(t, filepath.Join(dir, "plain.boru"), passSuite)

	// Directory walk finds only the _test.boru file.
	got, err := discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != a {
		t.Errorf("walk = %v, want [%s]", got, a)
	}

	// Explicit file (any name) + dedup of a repeated target.
	got, err = discover([]string{plain, plain})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != plain {
		t.Errorf("explicit+dedup = %v, want [%s]", got, plain)
	}
}

func TestDiscoverWalkError(t *testing.T) {
	old := walkDir
	t.Cleanup(func() { walkDir = old })
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("boom"))
	}
	if _, err := discover([]string{t.TempDir()}); err == nil {
		t.Fatal("expected a walk error")
	}
}

// runFile surfaces a file-read error as an errored suite.
func TestRunFileReadError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p, f, errored := runFile(&stdout, &stderr, filepath.Join(t.TempDir(), "gone_test.boru"), lang.Options{}, defaultMode(), nil)
	if !errored || p != 0 || f != 0 {
		t.Errorf("runFile(missing) = (%d,%d,%v), want (0,0,true)", p, f, errored)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a read error", stderr.String())
	}
}

// runFile surfaces an instance-init error (newBoru seam).
func TestRunFileInitError(t *testing.T) {
	old := newBoru
	t.Cleanup(func() { newBoru = old })
	newBoru = func(...lang.Options) (*lang.Boru, error) { return nil, errors.New("init boom") }
	f := write(t, filepath.Join(t.TempDir(), "i_test.boru"), passSuite)
	var stdout, stderr bytes.Buffer
	if _, _, errored := runFile(&stdout, &stderr, f, lang.Options{}, defaultMode(), nil); !errored {
		t.Error("runFile should report an init error as errored")
	}
	if !strings.Contains(stderr.String(), "init") {
		t.Errorf("stderr = %q, want an init error", stderr.String())
	}
}

func TestPercent(t *testing.T) {
	if got := percent(0, 0); got != 100 {
		t.Errorf("percent(0,0) = %v, want 100", got)
	}
	if got := percent(1, 2); got != 50 {
		t.Errorf("percent(1,2) = %v, want 50", got)
	}
}

// tallyFromSummary and stringFromStack skip the value type they don't consume.
func TestExtractHelpers(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("total", native.NewInteger(3))
	om.Set("passed", native.NewInteger(2))
	om.Set("failed", native.NewInteger(1))
	stack := []native.Value{native.NewMap(om), native.NewString("report text")}

	total, passed, failed := tallyFromSummary(stack)
	if total != 3 || passed != 2 || failed != 1 {
		t.Errorf("tally = (%d,%d,%d), want (3,2,1)", total, passed, failed)
	}
	if got := stringFromStack(stack); got != "report text" {
		t.Errorf("stringFromStack = %q, want report text", got)
	}
}

// defaultMode is CompileTry, the mode a no-flag invocation resolves to.
func defaultMode() run.CompileMode { return run.CompileTry }

// An IO.exit inside a suite ends THAT FILE, not the test run. The runner's
// own status stays "did every case pass" — a suite that exits 0 half-way
// has not passed the cases it never reached, so the file is reported as
// errored with the code it asked for, and the cases already recorded are
// still salvaged (the same salvage path a mid-suite raise takes).
func TestRunFileExitEndsTheFileNotTheRun(t *testing.T) {
	// `Test.test`, not `Test.case`: case is a data constructor taking
	// (expected, input, name), so the two-argument spelling this fixture
	// used never dispatched — it left the function on the stack and
	// recorded no case at all, which the loud dispatch contract now
	// refuses outright (design/legacy/FN-VALUE-DISPATCH.0.ignore).
	src := "import \"boru:test\"\nimport \"boru:io\"\n" +
		"Test.test \"one\" [Assert.equal 1 1]\n" +
		"IO.exit 0\n" +
		"Test.test \"two\" [Assert.equal 1 1]\n"
	f := write(t, filepath.Join(t.TempDir(), "x_test.boru"), src)
	var stdout, stderr bytes.Buffer
	passed, failed, errored := runFile(&stdout, &stderr, f, lang.Options{}, defaultMode(), nil)
	if !errored {
		t.Error("a suite that exits mid-way must be reported as errored, not as a clean pass")
	}
	if !strings.Contains(stderr.String(), "exited with code 0") {
		t.Errorf("stderr = %q, want the exit reported with its code", stderr.String())
	}
	// The salvage this test's contract names: the case BEFORE the exit is
	// still counted, and the one after it is not reached.
	if passed != 1 || failed != 0 {
		t.Errorf("passed=%d failed=%d, want the pre-exit case salvaged (1, 0)", passed, failed)
	}
}
