package lang

import (
	"strings"
	"testing"
)

// TestRunCompiledReason pins the third return of RunCompiledReason: the
// whole-program compilation-refusal reason the CLI surfaces as a warning. A
// reason is reported ONLY for a genuine refusal (a valid program the compiler
// cannot lower — a defect — which is then silently re-run on the interpreter);
// it is EMPTY for a compiled run and for a statically-invalid program (which
// fails in both engines and so is not a refusal at all).
func TestRunCompiledReason(t *testing.T) {
	// POSITIVE — a compiled program reports ran=true and no reason.
	t.Run("compiled", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("1 add 2")
		if !ran || reason != "" || err != nil {
			t.Fatalf("compiled program: ran=%v reason=%q err=%v (want ran=true, no reason, no error)", ran, reason, err)
		}
	})

	// POSITIVE — a genuine whole-program refusal reports ran=false and names
	// the first offending construct. Every CORPUS refusal has graduated, so
	// the pin rides an off-corpus shape: a `def` consuming a variadic loop
	// region with a DYNAMIC count (the S5 split needs the static region
	// size; a runtime-only count keeps the refusal) — it falls back to the
	// interpreter, which runs it fine. (The statically-counted fixture
	// graduated 2026-07-17 — the S5 first-value split compiles it.)
	t.Run("refusal names the offender", func(t *testing.T) {
		const src = `def m {n: 3} def xs (for (m get "n") [1]) xs`
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason(src)
		if ran {
			t.Fatalf("expected no compiled run (ran=false), got ran=true")
		}
		if !strings.Contains(reason, "def `xs` consumes loop results") {
			t.Fatalf("refusal reason %q does not name the offending construct", reason)
		}
		if codeOf(err) != "compile_failed" {
			t.Fatalf("Stage J: refusal must return compile_failed, got [%s] %v", codeOf(err), err)
		}
	})

	// A program the check pass stops on reports the sentinel as its reason
	// and fails. It used to report NO reason, because the compiler was about
	// to re-run it on the interpreter and the reason existed only to warn
	// about a slow path. There is no slow path: the compile failed, and the
	// reason says where.
	t.Run("a blocking diagnostic is the reason", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("no_such_word")
		if ran {
			t.Fatalf("a program that does not compile must not run compiled")
		}
		if reason != "check diagnostics" {
			t.Fatalf("reason = %q, want the check-diagnostics sentinel", reason)
		}
		if codeOf(err) != "compile_failed" {
			t.Fatalf("err = %v, want compile_failed", err)
		}
	})

	// NEGATIVE — a parse error (the err != nil compile path) reports NO reason
	// and surfaces the parse error, exactly as RunCompiled does.
	t.Run("parse error reports no reason", func(t *testing.T) {
		a, _ := New()
		_, ran, reason, err := a.RunCompiledReason("(1 add 2")
		if ran || reason != "" || err == nil {
			t.Fatalf("parse error: ran=%v reason=%q err=%v (want ran=false, no reason, an error)", ran, reason, err)
		}
	})

	// RunCompiled is the reason-free wrapper: its three returns must match
	// RunCompiledReason's (minus the reason) for the same source.
	t.Run("RunCompiled delegates", func(t *testing.T) {
		const src = "1 add 2"
		a, _ := New()
		out, ran, err := a.RunCompiled(src)
		if noteCompileDefect(t, src, out, err) {
			return
		}
		if !ran || err != nil || len(out) != 1 {
			t.Fatalf("RunCompiled(%q): out=%v ran=%v err=%v", src, out, ran, err)
		}
	})
}
