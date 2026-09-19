package run

import (
	"bytes"
	"strings"
	"testing"
)

func TestEvalSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "1 add 2", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3' in output, got %q", buf.String())
	}
}

func TestEvalParseError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, `"unterminated`, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	// Parse failures surface as boru-voice syntax_errors
	// (design/DIAGNOSTICS.0.md phase 2).
	if !strings.Contains(err.Error(), "[boru/syntax_error]") ||
		!strings.Contains(err.Error(), "this string is never closed") {
		t.Errorf("expected a boru-voice syntax_error, got %q", err.Error())
	}
}

func TestEvalEngineError(t *testing.T) {
	var buf bytes.Buffer
	err := Eval(&buf, "10 div 0", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

// CompileForce must run a compilable program through the VM and surface a
// refusal (not silently fall back) for one the emitter cannot lower.
func TestEvalCompilesOrFails(t *testing.T) {
	// A plain arithmetic program compiles and runs on the VM.
	var buf bytes.Buffer
	if err := EvalOptions(&buf, "1 add 2", OptionsFor("", 0, nil)); err != nil {
		t.Fatalf("a compilable program: %v", err)
	}
	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected '3', got %q", buf.String())
	}

	// One that does not compile (a loop result consumed by `size`) ABORTS,
	// carrying the emitter's reason. There is no mode in which it does
	// anything else.
	buf.Reset()
	err := EvalOptions(&buf, "(size (for 5 [i]))", OptionsFor("", 0, nil))
	if err == nil {
		t.Fatal("a program that does not compile: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "bytecode compilation FAILED") {
		t.Errorf("expected the compile failure named, got %q", err.Error())
	}

	// Nothing is printed for a program that did not run.
	if buf.Len() != 0 {
		t.Errorf("a program that does not compile wrote %q", buf.String())
	}
}

func TestCheckByDefault(t *testing.T) {
	run := func(args ...string) (int, string, string) {
		var out, errB bytes.Buffer
		code := Execute(args, strings.NewReader(""), &out, &errB)
		return code, out.String(), errB.String()
	}

	// A clean program runs with ZERO checker noise on stderr.
	code, out, stderr := run("-e", "add 1 2")
	if code != 0 || !strings.Contains(out, "3") {
		t.Fatalf("clean run: code=%d out=%q", code, out)
	}
	if stderr != "" {
		t.Errorf("clean run stderr = %q; want empty (quiet default gate)", stderr)
	}

	// An Error-severity finding aborts BEFORE execution, diagnostics shown.
	code, _, stderr = run("-e", "flurble 1 2")
	if code == 0 {
		t.Fatal("undefined word ran; want pre-flight abort")
	}
	if !strings.Contains(stderr, "undefined_word") || !strings.Contains(stderr, "check failed") {
		t.Errorf("abort stderr = %q; want the diagnostic + check failed", stderr)
	}

	// --no-check skips the pre-flight: the failure is a RUNTIME error.
	code, _, stderr = run("-no-check", "-e", "flurble 1 2")
	if code == 0 {
		t.Fatal("--no-check run of undefined word succeeded")
	}
	if strings.Contains(stderr, "check failed") {
		t.Errorf("--no-check stderr = %q; want runtime error, not the pre-flight gate", stderr)
	}

	// An advisory-only program (forward-strand info) stays QUIET by default…
	code, _, stderr = run("-e", "1 2 add 3 mul")
	if code != 0 {
		t.Fatalf("advisory-only program aborted: %q", stderr)
	}
	if strings.Contains(stderr, "forward_strands_operand") {
		t.Errorf("default gate printed advisory: %q", stderr)
	}
	// …and --check surfaces it.
	code, _, stderr = run("-check", "-e", "1 2 add 3 mul")
	if code != 0 {
		t.Fatalf("--check advisory run aborted: %q", stderr)
	}
	if !strings.Contains(stderr, "forward_strands_operand") {
		t.Errorf("--check stderr = %q; want the advisory", stderr)
	}
}
