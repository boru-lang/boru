package run

import (
	"strings"
	"testing"
)

// The -compile-report flag (design/legacy/RUNTIME-STAMPING.0.ignore Phase 5): a
// compiled-mode run that stamps a runtime-constructed callback prints its
// attribution to stderr; without the flag nothing prints; under -no-compile
// the header line explains the empty report.
func TestExecuteCompileReport(t *testing.T) {
	// A CAPTURING service handler in a factory body refuses to compile (the
	// handler is validated as a function value — CompileFnHandlerStrict), so
	// RunCompiled's armed interpreter fallback runs the factory and the
	// STORE-SITE stamp fires at runtime and records. (A fully compiled
	// program pre-stamps its handlers; the skip is deliberately unrecorded —
	// first stamp wins. The former paren-apply prefix graduated 2026-07-17,
	// §9.2e, so it no longer forces the fallback on its own.)
	src := `def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 42) (call {cmd:"X"} s)`

	var stdout, stderr strings.Builder
	if code := Execute([]string{"-compile-report", "-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "42") {
		t.Fatalf("run result missing: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "compile-report: stamped") {
		t.Fatalf("expected a stamped attribution line, got: %q", stderr.String())
	}

	// Without the flag: no report lines.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr.String(), "compile-report") {
		t.Fatalf("report printed without the flag: %q", stderr.String())
	}

	// -no-compile: never armed → the explanatory header.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-no-compile", "-compile-report", "-e", src}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "no runtime-stamp attempts") {
		t.Fatalf("expected the empty-report header, got: %q", stderr.String())
	}

	// A CAPTURING handler now STAMPS (plan Phase 6.3 — capture-slot
	// detached units; the pre-landing behaviour was a "lexical captures"
	// refusal line) and reports under the anonymous display name.
	stdout.Reset()
	stderr.Reset()
	capSrc := `def m {f: ([y:Integer] => [y add 1])} add 1 (5 (m get "f") apply) drop def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"N"} s)`
	if code := Execute([]string{"-compile-report", "-e", capSrc}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "compile-report: stamped (anonymous fn)") {
		t.Fatalf("expected the capturing handler's stamped line, got: %q", stderr.String())
	}
}
