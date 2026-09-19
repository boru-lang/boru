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
	// A service handler built inside a factory body: the handler is
	// constructed at RUN time, so the STORE-SITE stamp fires and records.
	//
	// The fixture used to CAPTURE the factory's parameter, which does not
	// compile — the stamp was reached only because the interpreter fallback
	// ran the factory. There is no fallback, so the capturing form now fails
	// outright and the fixture is its non-capturing twin, which compiles and
	// still exercises the surface this test is about. (The capturing shape
	// remains an open defect; it is booked in the lang package's ledger.)
	src := `def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ 42 ]) svc svc ]]) def s (mk 7) (call {cmd:"X"} s)`

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

	// A program with NO runtime-constructed callback gets the explanatory
	// header, so an empty report says why it is empty.
	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"-compile-report", "-e", "1 add 2"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no runtime-stamp attempts") {
		t.Fatalf("expected the empty-report header, got: %q", stderr.String())
	}
}
