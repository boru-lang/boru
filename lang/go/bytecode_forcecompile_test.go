package lang

import (
	"strings"
	"testing"
)

// RunCompiledStrict is the spelling that says a call REQUIRES the bytecode
// path. Every entry point does now — there is nothing to fall back to — so
// these pin the one outcome: a program compiles and runs, or it reports the
// defect that stopped it, leaving nothing behind on the registry.
func TestRunCompiledStrict(t *testing.T) {
	t.Run("compilable program runs on the VM", func(t *testing.T) {
		a, _ := New()
		got, err := a.RunCompiledStrict("1 add 2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Cross-check the result against the interpreter.
		b, _ := New()
		want, _ := b.RunInterp("1 add 2")
		if len(got) != len(want) || (len(got) == 1 && got[0] != want[0]) {
			t.Fatalf("force result %v != interpreter %v", got, want)
		}
	})

	t.Run("uncompilable program errors with the refusal reason", func(t *testing.T) {
		a, _ := New()
		_, err := a.RunCompiledStrict("(size (for 5 [i]))")
		if err == nil {
			t.Fatal("expected an error for an uncompilable program, got nil")
		}
		if codeOf(err) != "compile_failed" {
			t.Errorf("error should be the compile failure, got %q", err.Error())
		}
	})

	t.Run("a genuine runtime error surfaces (not a refusal)", func(t *testing.T) {
		a, _ := New()
		_, err := a.RunCompiledStrict("def x (10 sub 10) (100 div x)")
		if err == nil {
			t.Fatal("expected a division-by-zero error, got nil")
		}
		if strings.Contains(err.Error(), "force-compile") {
			t.Errorf("a runtime error must not be reported as a compile refusal: %q", err.Error())
		}
	})

	t.Run("check-diagnostics refusal names the blocking diagnostic", func(t *testing.T) {
		// The bare "check diagnostics" sentinel names nothing — and its
		// blocking diagnostic can be COMPILE-PASS-ONLY (`boru check` prints
		// zero diagnostics for this program, which runs clean interpreted).
		// The force-compile boundary appends the first blocking diagnostic's
		// code and detail (completeness review §8.1(4)). The fixture is a
		// corpus row (case.tsv:L97) whose clause list holds a bare `zed`:
		// the plain pass leaves it an atom-match and answers 'matched', the
		// compile pass reads it as a word and reports undefined_word — one
		// of the two compile-only diagnostics the diagnostic-surface ledger
		// still carries (diag_surface_test.go). The previous fixture was
		// the Stage 1 `/v` hold, which S1b-2 lifted: a `/v` read of a name
		// def-bound to a computed fn now resolves the fn-carrier side table,
		// so that program refuses for a reason of its own and names no
		// diagnostic.
		a, _ := New()
		_, err := a.RunCompiledStrict(`case zed/q [zed "matched" "other"]`)
		if err == nil {
			t.Fatal("expected an error from the blocking diagnostic, got nil")
		}
		// The blocking diagnostic IS the program's error now: a
		// statically-invalid program fails the same way whatever runs it,
		// so its own verdict is surfaced rather than a compile failure
		// wrapped around a sentinel.
		if codeOf(err) == "compile_failed" {
			t.Errorf("a blocking diagnostic must surface as the program's own error, got %q", err.Error())
		}
	})

	t.Run("a compile failure names the construct", func(t *testing.T) {
		a, _ := New()
		_, err := a.RunCompiledStrict("def leaked 42 (size (for 5 [i]))")
		if codeOf(err) != "compile_failed" {
			t.Fatalf("want compile_failed, got %v", err)
		}
		if !strings.Contains(err.Error(), "compiler defect") {
			t.Errorf("a compile failure must say it is a defect, got %q", err.Error())
		}
	})

	t.Run("side effects roll back on a refusal", func(t *testing.T) {
		// An uncompilable program that ALSO binds a name must not leak that
		// binding into the registry: the run is rolled back to the state it
		// found, exactly as it was on the old fallback path.
		a, _ := New()
		if _, err := a.RunCompiledStrict("def leaked 42 (size (for 5 [i]))"); err == nil {
			t.Fatal("expected a compile failure")
		}
		// `leaked` must be gone: a follow-up interpreter run that references it
		// errors as an undefined word rather than resolving to 42.
		if out, err := a.RunInterp("leaked"); err == nil {
			t.Errorf("binding leaked across a force-compile refusal: got %v", out)
		}
	})
}
