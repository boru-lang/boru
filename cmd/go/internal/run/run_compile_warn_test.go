package run

import (
	"strings"
	"testing"
)

// A program the compiler cannot lower FAILS, and stderr names the construct.
//
// This used to be a WARNING: the run fell back to the interpreter, answered
// correctly, and said so on one line — the warning existed because the
// silence was the problem (design/COMPILABLE-SUBSET.md §1). There is no
// fallback to warn about now. The failure says the same thing, without
// needing a second engine to have produced an answer first.
func TestExecuteNamesTheConstructThatDidNotCompile(t *testing.T) {
	// An off-corpus shape — a `def` consuming a variadic loop region with a
	// DYNAMIC count; the S5 split needs the static region size, so this is
	// the stable fixture now that the statically-counted sibling graduated
	// 2026-07-17. -no-check skips the pre-flight so the failure comes from
	// the EMITTER rather than the checker.
	doesNotCompile := `def m {n: 3} def xs (for (m get "n") [1]) xs`
	const wantFail = "bytecode compilation FAILED"

	var stdout, stderr strings.Builder
	if code := Execute([]string{"-no-check", "-e", doesNotCompile}, strings.NewReader(""), &stdout, &stderr); code == 0 {
		t.Fatalf("exit 0 for a program that does not compile; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), wantFail) {
		t.Fatalf("expected the compile failure named, got stderr: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "consumes loop results") {
		t.Fatalf("the failure should name the offending construct, got: %q", stderr.String())
	}

	// A compilable program runs and says nothing on stderr.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-e", "1 add 2"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3") {
		t.Fatalf("run result missing: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), wantFail) {
		t.Fatalf("a compiled program must say nothing, got: %q", stderr.String())
	}
}
