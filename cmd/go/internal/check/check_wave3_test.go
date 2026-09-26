package check

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// --- Command wrapper (New / Name / Synopsis / Run) ---

func TestCommandWrapper(t *testing.T) {
	c := New()
	if got := c.Name(); got != "check" {
		t.Errorf("Name() = %q, want check", got)
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
	var stdout, stderr bytes.Buffer
	if code := c.Run([]string{"-e", "1 add 2"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("Run exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Integer") {
		t.Errorf("stdout = %q, want Integer carrier", stdout.String())
	}
}

// --- RunCLI flag / argument handling ---

func TestRunCLINoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI(nil, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "requires a script file") {
		t.Errorf("stderr = %q, want 'requires a script file'", stderr.String())
	}
}

func TestRunCLIDashEWithoutExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "requires an expression") {
		t.Errorf("stderr = %q, want 'requires an expression'", stderr.String())
	}
}

func TestRunCLIMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nope.boru")
	if code := RunCLI([]string{missing}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want read error", stderr.String())
	}
}

func TestRunCLICleanFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "p.boru")
	if err := os.WriteFile(src, []byte("1 add 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{src}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "check: Integer") {
		t.Errorf("stdout = %q, want 'check: Integer'", stdout.String())
	}
	if !strings.Contains(stderr.String(), "0 error(s)") {
		t.Errorf("stderr = %q, want summary line", stderr.String())
	}
}

func TestRunCLIErrorExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e", "nonexistent"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want undefined_word diagnostic", stderr.String())
	}
	if !strings.Contains(stderr.String(), "check failed: 1 error(s)") {
		t.Errorf("stderr = %q, want 'check failed'", stderr.String())
	}
}

func TestRunCLISoftDowngradesErrors(t *testing.T) {
	// Both --soft and -soft spellings parse.
	for _, flagForm := range []string{"--soft", "-soft"} {
		var stdout, stderr bytes.Buffer
		if code := RunCLI([]string{flagForm, "-e", "nonexistent"}, &stdout, &stderr); code != 0 {
			t.Errorf("%s: exit = %d, want 0; stderr: %s", flagForm, code, stderr.String())
		}
	}
}

func TestRunCLIJSONOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--json", "-e", "1 add 2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	var res struct {
		Stack []string `json:"stack"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if len(res.Stack) != 1 || res.Stack[0] != "Integer" {
		t.Errorf("stack = %v, want [Integer]", res.Stack)
	}
}

func TestRunCLIJSONWithErrorsFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-json", "-e", "nonexistent"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Errorf("stdout is not JSON: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "check failed") {
		t.Errorf("stderr = %q, want 'check failed'", stderr.String())
	}
}

func TestRunCLIStrict(t *testing.T) {
	for _, flagForm := range []string{"--strict", "-strict"} {
		var stdout, stderr bytes.Buffer
		if code := RunCLI([]string{flagForm, "-e", "1 add 2"}, &stdout, &stderr); code != 0 {
			t.Errorf("%s: exit = %d, want 0; stderr: %s", flagForm, code, stderr.String())
		}
	}
}

func TestRunCLIParseErrorFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-e", `"unterminated`}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "check error") {
		t.Errorf("stderr = %q, want 'check error'", stderr.String())
	}
}

// --- RunCLI --emit ---

func TestRunCLIEmitCompilable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--emit", "-e", "1 add 2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "CALL_NATIVE") {
		t.Errorf("stdout = %q, want disassembly", out)
	}
	if !strings.Contains(out, "; sites: mono=1") {
		t.Errorf("stdout = %q, want site report", out)
	}
}

func TestRunCLIEmitUncompilableDiagnostics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"-emit", "-e", "nonexistent"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "uncompilable: check diagnostics") {
		t.Errorf("stdout = %q, want uncompilable reason", stdout.String())
	}
	if !strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want diagnostic echo", stderr.String())
	}
}

func TestRunCLIEmitParseErrorFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--emit", "-e", `"unterminated`}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("stderr = %q, want error output", stderr.String())
	}
}

// --- Emit direct ---

func TestEmitIslandReport(t *testing.T) {
	// A two-body `inner` islands (interpreter fallback span), so the compiled
	// program carries a fallback and Emit prints the island list.
	//
	// The fixture used to be a literal-list `each` with a lambda body — a
	// check-clean program, chosen over the `[dup mul]` spelling that does not
	// type-check (a failed dispatch inside it declines the compile outright,
	// design/legacy/FN-VALUE-DISPATCH.0.ignore, a different report than the
	// islanding this test is about). Since S1a of
	// design/FULL-COMPILATION-REPLAN.0.md (2026-09-19) that each compiles
	// natively — the corpus's island ceiling is 0 — and `inner`, whose two
	// code bodies the recorder does not yet lower, is the honest fixture for
	// "the compiler islanded and Emit said so" (the generated sweep's
	// `inner × literal` cell, test/go/langspec/SWEEP_STATUS.md).
	var stdout, stderr bytes.Buffer
	if err := Emit(&stdout, &stderr, "inner [add] [mul] [1 2] [3 4]"); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "fallbacks=1") {
		t.Errorf("stdout = %q, want a fallback", out)
	}
	if !strings.Contains(out, "; islands: inner") {
		t.Errorf("stdout = %q, want island report", out)
	}
}

func TestEmitUncompilableWithSiteCounts(t *testing.T) {
	// A `for` over a COMPUTED body fails to compile ("for: body not
	// captured" — the interpreter's inline splice, which no body activation
	// models; code-bodies.tsv L141) but still tallies dispatch sites. The
	// fixture used to be a computed-START range, which compiles natively
	// since 2026-09-26 (the planner promotes the start's producer to a frame
	// local and FOR_SETUP re-pushes it).
	var stdout, stderr bytes.Buffer
	if err := Emit(&stdout, &stderr, "def mk fn [[][List][quote [i]]] end for 3 (mk)"); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "uncompilable:") {
		t.Errorf("stdout = %q, want uncompilable reason", out)
	}
	if !strings.Contains(out, "; sites:") {
		t.Errorf("stdout = %q, want site report", out)
	}
}

func TestEmitEmptyProgramNoSiteReport(t *testing.T) {
	// `def x 1` compiles to an empty program with no dispatch sites and a
	// warning diagnostic (covers the diagnostics echo loop).
	var stdout, stderr bytes.Buffer
	if err := Emit(&stdout, &stderr, "def x 1"); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if strings.Contains(stdout.String(), "; sites:") {
		t.Errorf("stdout = %q, want no site report for empty program", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unused_def") {
		t.Errorf("stderr = %q, want unused_def warning", stderr.String())
	}
}

func TestEmitRunError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Emit(&stdout, &stderr, "gen [1]"); err == nil {
		t.Fatal("expected an error for a check-time failure")
	}
}

// --- Run direct ---

func TestRunEmptyStackAndWarning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(&stdout, &stderr, "def x 1", "", 0, false, false, false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "(empty stack)") {
		t.Errorf("stdout = %q, want empty-stack marker", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[warning] unused_def") {
		t.Errorf("stderr = %q, want warning diagnostic", stderr.String())
	}
}

func TestRunJSONSoftWithErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(&stdout, &stderr, "nonexistent", "", 0, true, true, false); err != nil {
		t.Fatalf("Run soft json: %v", err)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Errorf("stdout is not JSON: %s", stdout.String())
	}
}

func TestRunJSONCheckError(t *testing.T) {
	// gen with a bad parameter list raises a check-time error even in
	// JSON mode (after the result is emitted).
	var stdout, stderr bytes.Buffer
	err := Run(&stdout, &stderr, "gen [1]", "", 0, true, false, false)
	if err == nil || !strings.Contains(err.Error(), "check error") {
		t.Fatalf("err = %v, want check error", err)
	}
}

func TestRunCheckError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(&stdout, &stderr, "gen [1]", "", 0, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "check error") {
		t.Fatalf("err = %v, want check error", err)
	}
}

func TestRunStrictDirect(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(&stdout, &stderr, "1 add 2", "", 0, false, false, true); err != nil {
		t.Fatalf("Run strict: %v", err)
	}
}

// --- printDiagnostics ---

func TestPrintDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	printDiagnostics(&buf, []lang.CheckDiagnostic{
		{Row: 2, Col: 5, Severity: lang.SeverityError, Code: "boom", Detail: "bad thing", Word: "add",
			Suggestions: []lang.DiagSuggestion{{Message: "try harder"}}},
		{Code: "note", Detail: "no position, no severity"},
	}, "1 2 3\n1 add 2 x", false)
	out := buf.String()
	if !strings.Contains(out, "check: 2:5: [error] boom: bad thing") {
		t.Errorf("output = %q, want positioned error line", out)
	}
	if !strings.Contains(out, "check: [info] note: no position, no severity") {
		t.Errorf("output = %q, want positionless info line", out)
	}
	// The rich block renders under the stable one-liner: the source
	// excerpt with a caret at the word, and the suggestion.
	if !strings.Contains(out, "1 add 2 x") || !strings.Contains(out, "^^^") {
		t.Errorf("output = %q, want the source excerpt block", out)
	}
	if !strings.Contains(out, "= help: try harder") {
		t.Errorf("output = %q, want the suggestion line", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain rendering must be ANSI-free: %q", out)
	}
}

func TestPrintDiagnosticsEmpty(t *testing.T) {
	var buf bytes.Buffer
	printDiagnostics(&buf, nil, "", false)
	if buf.Len() != 0 {
		t.Errorf("output = %q, want empty", buf.String())
	}
}

// --- atPos / writeSiteReport ---

func TestAtPos(t *testing.T) {
	if got := atPos(0, 0); got != "-" {
		t.Errorf("atPos(0,0) = %q, want -", got)
	}
	if got := atPos(3, 7); got != "3:7" {
		t.Errorf("atPos(3,7) = %q, want 3:7", got)
	}
}

func TestWriteSiteReport(t *testing.T) {
	var buf bytes.Buffer
	writeSiteReport(&buf, map[string]int{"mono": 2, "poly": 1, "dynamic": 0, "meta": 3}, []string{"each", "fold"})
	out := buf.String()
	if !strings.Contains(out, "; sites: mono=2 poly=1 dynamic=0 meta=3") {
		t.Errorf("output = %q, want sites line", out)
	}
	if !strings.Contains(out, "; islands: each, fold") {
		t.Errorf("output = %q, want islands line", out)
	}

	buf.Reset()
	writeSiteReport(&buf, nil, nil)
	if buf.Len() != 0 {
		t.Errorf("output = %q, want empty for no counts/islands", buf.String())
	}
}

// --- Preflight ---

func TestPreflightCleanIsQuiet(t *testing.T) {
	var stderr bytes.Buffer
	if err := Preflight(&stderr, "1 add 2", "", 0, false); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want silence", stderr.String())
	}
}

func TestPreflightQuietSuppressesWarnings(t *testing.T) {
	// A warning-only program passes silently when verbose is off...
	var stderr bytes.Buffer
	if err := Preflight(&stderr, "def x 1", "", 0, false); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want silence when not verbose", stderr.String())
	}
	// ...and surfaces the warning when verbose is on.
	stderr.Reset()
	if err := Preflight(&stderr, "def x 1", "", 0, true); err != nil {
		t.Fatalf("Preflight verbose: %v", err)
	}
	if !strings.Contains(stderr.String(), "unused_def") {
		t.Errorf("stderr = %q, want unused_def warning", stderr.String())
	}
}

func TestPreflightErrorAborts(t *testing.T) {
	var stderr bytes.Buffer
	err := Preflight(&stderr, "nonexistent", "", 0, false)
	if err == nil || !strings.Contains(err.Error(), "check failed: 1 error(s)") {
		t.Fatalf("err = %v, want check failed", err)
	}
	if !strings.Contains(stderr.String(), "undefined_word") {
		t.Errorf("stderr = %q, want diagnostics printed", stderr.String())
	}
}

func TestPreflightCheckError(t *testing.T) {
	var stderr bytes.Buffer
	err := Preflight(&stderr, "gen [1]", "", 0, false)
	if err == nil || !strings.Contains(err.Error(), "check error") {
		t.Fatalf("err = %v, want check error", err)
	}
}

func TestPreflightColorAtAnchorsRelativeImports(t *testing.T) {
	// `boru build`'s pre-flight (NUR044) must resolve relative file imports
	// against the ENTRY directory — what the built binary will do — not the
	// process cwd. The test cwd is the package dir, foreign to dir, so the
	// anchored call passes only because baseDir wins over the cwd.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.boru"), []byte(`export "Lib" {x: 1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "import \"./lib.boru\"\nLib.x\n"
	var stderr bytes.Buffer
	if err := PreflightColorAt(&stderr, source, "", 0, false, false, dir); err != nil {
		t.Fatalf("anchored preflight: %v; stderr: %s", err, stderr.String())
	}
	// An empty baseDir keeps the cwd behaviour run/debug rely on: from this
	// foreign cwd the import misses and the check declines.
	stderr.Reset()
	if err := PreflightColorAt(&stderr, source, "", 0, false, false, ""); err == nil {
		t.Fatal("unanchored preflight resolved ./lib.boru from a foreign cwd; want compile failure")
	}
}

// --- --color flag parsing ---------------------------------------------------

func TestRunCLIColorFlag(t *testing.T) {
	// With a value: consumed together with its mode argument.
	var stdout, stderr bytes.Buffer
	if code := RunCLI([]string{"--color", "never", "-e", "1 add 2"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--color never: exit %d, stderr %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Errorf("--color never must stay plain: %q", stderr.String())
	}
	// Bare --color as the LAST flag: consumed alone, the source arg
	// still required (exercises the no-value arm).
	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{"--color"}, &stdout, &stderr); code != 1 {
		t.Fatalf("bare --color with no source: exit %d, want 1", code)
	}
	// --color always paints the rich diagnostic block.
	stdout.Reset()
	stderr.Reset()
	if code := RunCLI([]string{"--color", "always", "-e", "99 uppr"}, &stdout, &stderr); code != 1 {
		t.Fatalf("--color always on an erroring program: exit %d", code)
	}
	if !strings.Contains(stderr.String(), "\x1b[") {
		t.Errorf("--color always must carry ANSI in the rich block: %q", stderr.String())
	}
}
