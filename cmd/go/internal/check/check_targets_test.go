package check

// check_targets_test.go — `boru check` over MANY targets: the FlagSet,
// multi-file and directory expansion, per-file import anchoring, and the
// argument tolerances the hand-rolled parser used to provide.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// write creates dir/name (creating dir) and returns its path.
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- the defect: every named file is checked ------------------------------

// Positive: several clean files all check, and every one of them is named
// in the report.
func TestRunCLIMultipleCleanFiles(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "3 mul 4")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, b}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	for _, p := range []string{a, b} {
		if !strings.Contains(stderr.String(), "check: file "+p) {
			t.Errorf("stderr = %q, want a banner for %s", stderr.String(), p)
		}
		if !strings.Contains(stdout.String(), "check: "+p+": ") {
			t.Errorf("stdout = %q, want a carrier line for %s", stdout.String(), p)
		}
	}
	if !strings.Contains(stderr.String(), "across 2 file(s)") {
		t.Errorf("stderr = %q, want the aggregate summary", stderr.String())
	}
}

// Negative — the reported defect: an error in the SECOND file must be
// reported and must fail the run. Before the FlagSet only args[0] was
// ever read, so this invocation printed "0 error(s)" and exited 0.
func TestRunCLISecondFileErrorFails(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "nosuchword 1")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, b}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want the second file's diagnostic", stderr.String())
	}
	if !strings.Contains(stderr.String(), "check failed: 1 of 2 file(s)") {
		t.Errorf("stderr = %q, want the multi-file failure line", stderr.String())
	}
	// The clean file was still checked and reported: diagnostics
	// accumulate, they do not stop at the first failure.
	if !strings.Contains(stdout.String(), "check: "+a+": Integer") {
		t.Errorf("stdout = %q, want the first file's carrier line", stdout.String())
	}
}

// A failure in the FIRST file does not stop the second from being checked.
func TestRunCLIFirstFileErrorStillChecksRest(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "nosuchword 1")
	b := write(t, dir, "b.boru", "otherbadword 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, b}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "otherbadword") {
		t.Errorf("stderr = %q, want the second file checked too", stderr.String())
	}
	if !strings.Contains(stderr.String(), "check failed: 2 of 2 file(s)") {
		t.Errorf("stderr = %q, want both files counted as failed", stderr.String())
	}
	if !strings.Contains(stderr.String(), "check: total 2 error(s)") {
		t.Errorf("stderr = %q, want the accumulated error count", stderr.String())
	}
}

// --soft downgrades across every file, so the run exits 0 with the
// diagnostics still reported.
func TestRunCLIMultiSoft(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "nosuchword 1")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--soft", a, b}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want the diagnostic reported anyway", stderr.String())
	}
}

// A file whose ANALYSIS fails (not merely a diagnostic) is reported under
// its own banner and the remaining files are still checked.
func TestRunCLIMultiCheckErrorContinues(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "gen [1]")
	b := write(t, dir, "b.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, b}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "check: check error:") {
		t.Errorf("stderr = %q, want the per-file check error line", stderr.String())
	}
	if !strings.Contains(stdout.String(), "check: "+b+": Integer") {
		t.Errorf("stdout = %q, want the second file still checked", stdout.String())
	}
}

// A path named twice is checked once.
func TestRunCLIDuplicateTargetCheckedOnce(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, a}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "across 2 file(s)") {
		t.Errorf("stderr = %q, want the duplicate collapsed", stderr.String())
	}
	if !strings.Contains(stdout.String(), "check: Integer") {
		t.Errorf("stdout = %q, want the single-file report shape", stdout.String())
	}
}

// --- flags -----------------------------------------------------------------

// -h prints the usage listing instead of being read as a filename.
func TestRunCLIHelpFlag(t *testing.T) {
	// Asking for help is not an error: `boru help check` points the reader
	// at `boru check -h`, so it prints the documented usage on STDOUT and
	// exits 0. Before this surface existed, -h was read as a filename and
	// failed with `open -h: no such file or directory`.
	for _, arg := range []string{"-h", "--help", "help"} {
		var stdout, stderr bytes.Buffer
		if code := RunCLI([]string{arg}, &stdout, &stderr); code != 0 {
			t.Fatalf("%s: exit = %d, want 0; stderr: %s", arg, code, stderr.String())
		}
		out := stdout.String()
		if strings.Contains(out+stderr.String(), "no such file") {
			t.Fatalf("%s was read as a filename: %q", arg, out+stderr.String())
		}
		for _, want := range []string{"Usage: boru check", "--json", "--soft", "--strict", "--pedantic", "-e EXPR", "-r PATH", "-s SEED"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: usage = %q, want it to mention %q", arg, out, want)
			}
		}
	}
	// Negative half: a flag that does NOT exist is still an error, so the
	// help path cannot be mistaken for "check accepts anything".
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--nope", "x.boru"}, &stdout, &stderr); code == 0 {
		t.Fatalf("exit = 0 for an unknown flag, want non-zero; stderr: %s", stderr.String())
	}
}

// Negative: an unknown flag is REJECTED, not treated as a file.
func TestRunCLIUnknownFlagRejected(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--nope", a}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Errorf("stderr = %q, want the undefined-flag message", stderr.String())
	}
}

// The trap the interleaved parse exists to avoid: a flag AFTER the files
// must be a flag, not a third (unreadable) file.
func TestRunCLIFlagAfterFiles(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "3 add 4")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, b, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "no such file") {
		t.Fatalf("--json was read as a file: %q", stderr.String())
	}
	var got []fileResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout.String())
	}
	if len(got) != 2 || got[0].File != a || got[1].File != b {
		t.Errorf("json = %+v, want one entry per file", got)
	}
}

// A single target keeps the bare-object JSON shape editors already parse.
func TestRunCLISingleFileJSONShapeUnchanged(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	var res lang.CheckResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not a bare CheckResult object: %v\n%s", err, stdout.String())
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Integer" {
		t.Errorf("stack = %v, want [Integer]", res.Stack)
	}
}

// A multi-file JSON run still fails, and names the failing file.
func TestRunCLIMultiJSONReportsFailure(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "gen [1]")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--json", a, b}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, stderr.String())
	}
	var got []fileResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout.String())
	}
	if len(got) != 2 || got[1].Error == "" {
		t.Errorf("json = %+v, want the analysis error on the second entry", got)
	}
}

// `--` ends flag parsing for its segment, so a file whose name starts
// with a dash can still be named.
func TestRunCLIDashDashEscapesFilename(t *testing.T) {
	dir := t.TempDir()
	odd := write(t, dir, "-odd.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--", odd}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	// The absolute path from t.TempDir() does not start with a dash, so
	// name the awkward form explicitly too, relative to its directory.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{"--", "-odd.boru"}, &stdout, &stderr); code != 0 {
		t.Fatalf("relative dash name: exit = %d, want 0; stderr: %s", code, stderr.String())
	}
}

// Negative: -e and file targets are mutually exclusive — silently
// preferring one over the other is how the original defect hid.
func TestRunCLIExprAndFilesRejected(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e", "1 add 2", a}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "either -e EXPR or file targets") {
		t.Errorf("stderr = %q, want the mutual-exclusion message", stderr.String())
	}
}

// -e "" is a legal (empty) program: presence of the flag, not emptiness
// of its value, selects expression mode.
func TestRunCLIEmptyExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e", ""}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(empty stack)") {
		t.Errorf("stdout = %q, want the empty-stack marker", stdout.String())
	}
}

// -r and -s are the flags CLI.md documents; they must reach lang.Options
// rather than being dropped for hard-coded defaults.
func TestRunCLIRegistryAndSeedReachOpts(t *testing.T) {
	var got lang.Options
	orig := langNew
	langNew = func(opts ...lang.Options) (*lang.Boru, error) {
		if len(opts) > 0 {
			got = opts[0]
		}
		return orig(opts...)
	}
	t.Cleanup(func() { langNew = orig })

	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-r", "/reg/path", "-s", "42", a}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if got.Registry != "/reg/path" || got.Seed != 42 {
		t.Errorf("lang.Options = %+v, want Registry=/reg/path Seed=42", got)
	}
}

// --- directory targets -----------------------------------------------------

func TestRunCLIDirectoryTargetRecurses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.boru", "1 add 2")
	write(t, dir, "nested/b.boru", "3 add 4")
	write(t, dir, "notes.md", "not boru")
	// .boru/ holds fetched and generated package artifacts: never walked.
	write(t, dir, ".boru/vendored.boru", "nosuchword 1")

	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "a.boru") || !strings.Contains(out, "b.boru") {
		t.Errorf("stderr = %q, want both .boru files checked", out)
	}
	if strings.Contains(out, "vendored.boru") || strings.Contains(out, "undefined_word") {
		t.Errorf("stderr = %q, want .boru/ skipped", out)
	}
	if strings.Contains(out, "notes.md") {
		t.Errorf("stderr = %q, want non-.boru files ignored", out)
	}
	if !strings.Contains(out, "across 2 file(s)") {
		t.Errorf("stderr = %q, want exactly two files checked", out)
	}
}

// Negative: a target that expands to nothing is an error, not a quiet 0 —
// "checked nothing, reported green" is the defect class this command is
// being fixed for.
func TestRunCLIDirectoryWithNoSourcesFails(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "notes.md", "not boru")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no .boru files under") {
		t.Errorf("stderr = %q, want the empty-expansion error", stderr.String())
	}
}

// Directory expansion is sorted, so the report is reproducible.
func TestResolveTargetsSortsDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "z.boru", "1")
	write(t, dir, "a.boru", "2")
	got, err := resolveTargets([]string{dir})
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(got) != 2 || filepath.Base(got[0].Path) != "a.boru" || filepath.Base(got[1].Path) != "z.boru" {
		t.Errorf("targets = %+v, want a.boru before z.boru", got)
	}
}

// --- per-file import anchoring (the NUR044 question, per target) -----------

func TestRunCLIAnchorsImportsPerFile(t *testing.T) {
	// Two programs in DIFFERENT directories, each importing its own
	// sibling: no single cwd can answer both, so each file's imports
	// resolve against its own directory.
	dir := t.TempDir()
	write(t, dir, "one/lib.boru", `export "Lib" {x: 1}`+"\n")
	one := write(t, dir, "one/m.boru", "import \"./lib.boru\"\nLib.x\n")
	write(t, dir, "two/lib.boru", `export "Lib" {y: 2}`+"\n")
	two := write(t, dir, "two/m.boru", "import \"./lib.boru\"\nLib.y\n")

	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{one, two}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
}

// A RELATIVE target whose bare-module package sits in an ANCESTOR
// `.boru/` directory. This is the case every other anchoring test misses:
// they all use t.TempDir() absolute paths, and the one relative test
// chdirs so the anchor is ".", the single relative form that happens to
// work. A relative anchor is assigned to BaseDir, and resolveBareModule
// walks parents from there until filepath.Dir(dir) == dir — which a
// relative path reaches immediately. The walk truncates and the checker
// invents `undefined_word` errors against clean code, which is why
// anchorOf must return an absolute directory.
func TestRunCLIRelativeTargetResolvesAncestorPackage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".boru/mylib/index.boru",
		"def helper fn x:Number Number [mul x 3]\nexport \"My\" {helper: helper/v}\n")
	write(t, dir, "src/m.boru", "import \"mylib\"\nMy.helper 7\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	// The target is named RELATIVELY, and its package is one level up.
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{filepath.Join("src", "m.boru")}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 — the ancestor package should resolve; stderr: %s",
			code, stderr.String())
	}
	// Negative half: a real error in the same shape must still be reported,
	// so the fix cannot be mistaken for "relative targets never fail".
	write(t, dir, "src/bad.boru", "import \"mylib\"\nnosuchword 1 2\n")
	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{filepath.Join("src", "bad.boru")}, &stdout, &stderr); code == 0 {
		t.Fatalf("exit = 0, want non-zero for an undefined word; stderr: %s", stderr.String())
	}
	if got := stderr.String() + stdout.String(); !strings.Contains(got, "undefined_word") {
		t.Errorf("want an undefined_word diagnostic, got: %s", got)
	}
}

// The library entry points keep the cwd anchor `boru run`'s pre-flight
// relies on: a Target with no Path is source text, not a file.
func TestRunColorKeepsCwdAnchor(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib.boru", `export "Lib" {x: 1}`+"\n")
	var stdout, stderr bytes.Buffer
	err := RunColor(&stdout, &stderr, "import \"./lib.boru\"\nLib.x\n", "", 0, false, false, false, false)
	if err == nil {
		t.Fatal("expected the import to miss from a foreign cwd; want a refusal")
	}
}

// --- --emit over several targets -------------------------------------------

func TestRunCLIEmitMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	b := write(t, dir, "b.boru", "3 add 4")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--emit", a, b}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "; file: "+a) || !strings.Contains(out, "; file: "+b) {
		t.Errorf("stdout = %q, want a file banner per disassembly", out)
	}
	if strings.Count(out, "CALL_NATIVE") < 2 {
		t.Errorf("stdout = %q, want both programs disassembled", out)
	}
}

// Negative: --emit stops at the first file it cannot record.
func TestRunCLIEmitStopsOnError(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", `"unterminated`)
	b := write(t, dir, "b.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--emit", a, b}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), "; file: "+b) {
		t.Errorf("stdout = %q, want the run stopped before the second file", stdout.String())
	}
}

// EmitAt anchors relative imports like PreflightColorAt does.
func TestEmitAtAnchorsImports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib.boru", `export "Lib" {x: 1}`+"\n")
	var stdout, stderr bytes.Buffer
	if err := EmitAt(&stdout, &stderr, "import \"./lib.boru\"\nLib.x\n", "", 0, dir); err != nil {
		t.Fatalf("EmitAt: %v; stderr: %s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want the anchored import to resolve", stderr.String())
	}
}

// --- resolution failures ---------------------------------------------------

func TestResolveTargetsMissingPath(t *testing.T) {
	if _, err := resolveTargets([]string{filepath.Join(t.TempDir(), "nope.boru")}); err == nil {
		t.Fatal("expected a stat error for a missing target")
	}
}

// Seam: a file os.Stat resolved cannot then fail to read under the
// suite's uid, so the read-failure arm is driven through osReadFile.
func TestResolveTargetsReadError(t *testing.T) {
	orig := osReadFile
	osReadFile = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFile = orig })
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{a}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "read boom") {
		t.Errorf("stderr = %q, want the read error", stderr.String())
	}
}

// Seam: filepath.WalkDir never surfaces a per-entry error under the
// suite's uid, so the walk-failure arm is driven through walkDir.
func TestResolveTargetsWalkError(t *testing.T) {
	orig := walkDir
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk boom"))
	}
	t.Cleanup(func() { walkDir = orig })
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{t.TempDir()}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "walk boom") {
		t.Errorf("stderr = %q, want the walk error", stderr.String())
	}
}

// --- RunTargets as a library entry point -----------------------------------

func TestRunTargetsEmptyIsNoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunTargets(&stdout, &stderr, nil, Opts{JSON: true}); err != nil {
		t.Fatalf("RunTargets(nil): %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("output = (%q, %q), want silence", stdout.String(), stderr.String())
	}
}

func TestRunTargetsMultiJSONMarshalError(t *testing.T) {
	orig := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshalIndent = orig })
	var stdout, stderr bytes.Buffer
	err := RunTargets(&stdout, &stderr, []Target{
		{Path: "a.boru", Source: "1 add 2"},
		{Path: "b.boru", Source: "3 add 4"},
	}, Opts{JSON: true})
	if err == nil || !strings.Contains(err.Error(), "json marshal") {
		t.Fatalf("err = %v, want json marshal error", err)
	}
}

func TestRunTargetsInitError(t *testing.T) {
	swapLangNewFail(t)
	var stdout, stderr bytes.Buffer
	err := RunTargets(&stdout, &stderr, []Target{{Path: "a.boru", Source: "1 add 2"}}, Opts{})
	if err == nil || !strings.Contains(err.Error(), "init error") {
		t.Fatalf("err = %v, want init error", err)
	}
}

// Strict mode reaches every target, not just the first.
func TestRunTargetsStrictAcrossFiles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunTargets(&stdout, &stderr, []Target{
		{Path: "a.boru", Source: "1 add 2"},
		{Path: "b.boru", Source: "3 add 4"},
	}, Opts{Strict: true}); err != nil {
		t.Fatalf("RunTargets strict: %v", err)
	}
}

// A multi-target run prints the empty-stack marker under the file's name.
func TestRunTargetsMultiEmptyStack(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunTargets(&stdout, &stderr, []Target{
		{Path: "a.boru", Source: "def x 1"},
		{Path: "b.boru", Source: "1 add 2"},
	}, Opts{Soft: true})
	if err != nil {
		t.Fatalf("RunTargets: %v", err)
	}
	if !strings.Contains(stdout.String(), "check: a.boru: (empty stack)") {
		t.Errorf("stdout = %q, want the named empty-stack line", stdout.String())
	}
}

// --- tolerateLegacyTail ----------------------------------------------------

func TestTolerateLegacyTail(t *testing.T) {
	var stderr bytes.Buffer
	// Untouched when there is nothing to tolerate.
	got, ok := tolerateLegacyTail([]string{"a.boru"}, &stderr)
	if !ok || len(got) != 1 || got[0] != "a.boru" {
		t.Errorf("tolerateLegacyTail(a.boru) = (%v, %v), want it unchanged", got, ok)
	}
	got, ok = tolerateLegacyTail(nil, &stderr)
	if !ok || got != nil {
		t.Errorf("tolerateLegacyTail(nil) = (%v, %v), want (nil, true)", got, ok)
	}
	// A trailing bare --color is dropped, as the old parser dropped it.
	for _, form := range []string{"--color", "-color"} {
		got, ok = tolerateLegacyTail([]string{"a.boru", form}, &stderr)
		if !ok || len(got) != 1 || got[0] != "a.boru" {
			t.Errorf("%s: got (%v, %v), want the flag dropped", form, got, ok)
		}
	}
	// A trailing bare -e keeps its own message.
	for _, form := range []string{"-e", "--e"} {
		stderr.Reset()
		if _, ok := tolerateLegacyTail([]string{form}, &stderr); ok {
			t.Errorf("%s: want a refusal", form)
		}
		if !strings.Contains(stderr.String(), "requires an expression") {
			t.Errorf("%s: stderr = %q, want the legacy message", form, stderr.String())
		}
	}
}

// --color with a value still parses, and a trailing bare --color still
// leaves the run to fail on the missing target rather than on the flag.
func TestRunCLIColorTolerances(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.boru", "1 add 2")
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--color", "never", a}, &stdout, &stderr); code != 0 {
		t.Fatalf("--color never: exit = %d; stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{"--color"}, &stdout, &stderr); code != 1 {
		t.Fatalf("bare --color: exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "requires a script file") {
		t.Errorf("stderr = %q, want the missing-target message", stderr.String())
	}
}

// Drive Abs failure directly: Windows cannot remove its cwd, and Darwin's
// Go getcwd wrapper can return the stale path after removal.
func TestAnchorOfFallsBackWhenCwdUnavailable(t *testing.T) {
	resolve := func(dir string) (string, error) {
		if dir != "x" {
			t.Fatalf("resolved %q, want the script's directory", dir)
		}
		return "", &os.PathError{Op: "getwd", Err: os.ErrNotExist}
	}
	if got := anchorOfWithAbs(filepath.Join("x", "m.boru"), resolve); got != "x" {
		t.Fatalf("anchorOf must fall back to the relative dir when Abs fails, got %q", got)
	}
}

func TestAnchorOfResolvesDirectory(t *testing.T) {
	want := t.TempDir()
	if got := anchorOf(filepath.Join(want, "m.boru")); got != want {
		t.Fatalf("anchorOf = %q, want %q", got, want)
	}
	if got := anchorOfWithAbs("", func(string) (string, error) {
		t.Fatal("an empty path must not resolve a directory")
		return "", nil
	}); got != "" {
		t.Fatalf("empty path anchor = %q", got)
	}
}
