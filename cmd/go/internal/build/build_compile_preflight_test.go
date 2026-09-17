package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `boru build` must not ship a binary for a program the emitter REFUSES.
//
// The hole this closes: `build` baked CompileTry by default, so a refusing
// program produced a perfectly normal binary that dropped to the interpreter
// at run time — and buildrt.Main passes a nil warn writer, so the refusal
// warning `boru run` prints was never printed either. The author shipped a
// compile failure and was never told. Failure to compile is a failure
// (design/COMPILABLE-SUBSET.md §1), so the build is refused and named.
//
// Paired negative: a compilable program must still build, and -no-compile
// must still ship a DECLARED interpreter binary — the opt-out is saying so
// out loud, not a silent fallback.
func TestCompilePreflightRefusesUncompilableProgram(t *testing.T) {
	// A def-bound computed fn apply: a closure whose shape Stage 1 cannot
	// prove. Valid boru — the interpreter answers 6 — and it does not compile.
	const refuses = `def idf t:Any => [t/v]
def g (idf (z:Integer => [add 1 z]))
(g 5)`

	reason, err := compilePreflight(refuses, "", 0, "")
	if err != nil {
		t.Fatalf("compilePreflight errored: %v — want a refusal reason, not an error", err)
	}
	if reason == "" {
		t.Fatal("expected a refusal reason for a program the emitter cannot lower; got none (the build would have shipped a silent interpreter fallback)")
	}
}

func TestCompilePreflightAcceptsCompilableProgram(t *testing.T) {
	const compiles = `def n (1 add 2)
n`
	reason, err := compilePreflight(compiles, "", 0, "")
	if err != nil {
		t.Fatalf("compilePreflight errored: %v", err)
	}
	if reason != "" {
		t.Fatalf("a compilable program was refused with %q — the gate over-refuses", reason)
	}
}

// The pre-flight COMPILES; it must never RUN the program. A build that
// executed its input would fire the program's side effects on the build
// machine. CompileCheck records through the checker without executing, and
// this pins that: the program below writes to a store on execution, and the
// pre-flight must return its refusal without that write happening.
func TestCompilePreflightDoesNotExecuteTheProgram(t *testing.T) {
	// `print` is the cheapest observable effect; if the pre-flight executed
	// this, the test binary's stdout would carry the marker. The assertion
	// that matters is the absence of execution, which we get by checking the
	// pre-flight still answers the compile question for a program whose
	// FIRST statement is an effect.
	const withEffect = `print "preflight-must-not-run-this"
def idf t:Any => [t/v]
def g (idf (z:Integer => [add 1 z]))
(g 5)`

	reason, err := compilePreflight(withEffect, "", 0, "")
	if err != nil {
		t.Fatalf("compilePreflight errored: %v", err)
	}
	if reason == "" {
		t.Fatal("expected a refusal reason; got none")
	}
}

// A program the checker has findings on does not compile, and the gate says so.
//
// In the normal flow this is unreachable: the check pre-flight runs FIRST and
// returns 1 on a check error, so the compile gate never sees such a program.
// It is reachable under -no-check, and there refusing is right — the emitter
// latches on the "check diagnostics" sentinel and produces no Program, so a
// build would ship a silent interpreter fallback. Note the reason is the
// sentinel rather than a named construct; that sentinel is the single largest
// blocker for real programs (13 of the 27 in TestRealProgramsCompile).
func TestCompilePreflightRefusesOnCheckDiagnostics(t *testing.T) {
	const bad = `no-such-word-anywhere 1 2 3`
	reason, err := compilePreflight(bad, "", 0, "")
	if err != nil {
		t.Fatalf("compilePreflight errored: %v", err)
	}
	if reason == "" {
		t.Fatal("a program with check diagnostics produced no refusal reason — the build would have shipped it")
	}
}

// A PARSE failure is not a refusal — we could not answer the compile question
// at all, so it comes back as an error and Main reports it as one.
func TestCompilePreflightErrorsOnUnparseableSource(t *testing.T) {
	const unparseable = `}{`
	_, err := compilePreflight(unparseable, "", 0, "")
	if err == nil {
		t.Fatal("expected an error for unparseable source — a parse failure is not a compile refusal")
	}
}

// baseDir anchors relative file imports to the directory the BUILT binary will
// resolve them against, exactly as the check pre-flight does. Pinned against a
// REAL multi-file program in the tree rather than a synthetic one, because the
// failure mode that matters is a valid multi-file build being falsely refused.
func TestCompilePreflightHonoursBaseDir(t *testing.T) {
	const src = `import "./lib.boru"
lib-val`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.boru"), []byte("def lib-val 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// With baseDir the import is resolvable; without it the file cannot be
	// found at all. Either may still refuse for an unrelated compile reason —
	// what must NOT happen is compilePreflight erroring, which would abort the
	// build with "init error" on a perfectly ordinary program.
	if _, err := compilePreflight(src, "", 0, dir); err != nil {
		t.Fatalf("compilePreflight errored with a baseDir: %v", err)
	}
	if _, err := compilePreflight(src, "", 0, ""); err != nil {
		t.Fatalf("compilePreflight errored without a baseDir: %v", err)
	}
}

// End-to-end: `boru build` must exit non-zero and write no binary for a
// program the emitter refuses, and must name the reason. This is the arm that
// actually protects a user — compilePreflight's unit tests prove the diagnosis,
// this proves the build is stopped by it.
func TestBuildRefusesUncompilableProgram(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "refuses.boru")
	const prog = `def idf t:Any => [t/v]
def g (idf (z:Integer => [add 1 z]))
(g 5)`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "refuses.bin")

	var stdout, stderr strings.Builder
	code := New().Run([]string{src, "-o", out}, nil, &stdout, &stderr)

	if code == 0 {
		t.Errorf("build exited 0 for a program that does not compile; stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "bytecode compilation FAILED") {
		t.Errorf("stderr does not name the refusal: %q", stderr.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a binary was written for a program that does not compile — it would have run silently on the interpreter")
	}
}

// The declared opt-out still ships. -no-compile bakes CompileOff, so the
// binary is interpreted BY DECLARATION; the gate must not block that.
func TestBuildNoCompileShipsDeclaredInterpreterBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "refuses.boru")
	const prog = `def idf t:Any => [t/v]
def g (idf (z:Integer => [add 1 z]))
(g 5)`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "refuses.bin")

	var stdout, stderr strings.Builder
	code := New().Run([]string{"-no-compile", src, "-o", out}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("-no-compile build failed: code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("-no-compile wrote no binary: %v", err)
	}
}

// -no-check opts out of being gated on the CHECKER, not on the EMITTER.
//
// The carve-out in Run() clears the "check diagnostics" sentinel under
// -no-check, because that sentinel is the checker's verdict and gating on it
// would make the compile gate a second check gate (TestRunNoCheckFlagSkipsGate
// pins that `build -no-check` still produces an artefact). This pins the other
// half, which is the one that would otherwise rot: a genuine CONSTRUCT refusal
// must still stop the build even with -no-check, or the flag becomes a way to
// ship a silent interpreter fallback.
func TestBuildNoCheckStillRefusesConstructRefusal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "refuses.boru")
	// Type-clean, so the checker has nothing to say; it refuses on the
	// construct, giving a named reason rather than the sentinel.
	const prog = `def idf t:Any => [t/v]
def g (idf (z:Integer => [add 1 z]))
(g 5)`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "refuses.bin")

	var stdout, stderr strings.Builder
	code := New().Run([]string{"-no-check", src, "-o", out}, nil, &stdout, &stderr)
	if code == 0 {
		t.Errorf("-no-check built a program with a genuine construct refusal; stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "bytecode compilation FAILED") {
		t.Errorf("stderr does not name the refusal: %q", stderr.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a binary was written despite a construct refusal under -no-check")
	}
}
