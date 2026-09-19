package lang

import (
	"fmt"
	"strings"
	"testing"
)

// `do […] error [handler]` where the handler leaves the stack EMPTY used to
// be modelled as producing one value. `errorReturnsFn` runs the handler
// during the compile pass, so it can see the real residual — and on
// `len(stk) != 1` it returned the wide one-value `dynamic(Any)` bound, the
// same answer it gives for the cases where it genuinely does not know.
//
// Everything downstream was faithful to that wrong model: the dispatch was
// recorded as a single-output fallback island, `FallbackSpan` has no
// out-count field to say otherwise, `lowerFallback` pushed exactly one
// simulated slot, and at run time `runFallback` appended the island's real
// ZERO results — leaving the stack one short. The phantom slot then read as
// a runtime fn-value, so the failure surfaced as `CALL_DYNAMIC underflow`
// or `BIND_GLOBAL underflow`: an `internal_error` from ordinary source.
//
// The repair is to refuse rather than widen. A known-wrong arity is not an
// unknown one, the island model cannot express any out-count but 1, and
// under the refusal architecture the interpreter fallback is always sound —
// `tryRecordFallback` already declines `len(outs) != 1` for the same reason,
// and its sibling guard explicitly prefers "a clean refusal" to a new
// island.
//
// For most shapes this buys honesty rather than answers: the whole-program
// fallback caught the underflow and silently re-ran, so DEFAULT mode was
// already returning the interpreter's result at the cost of a wasted
// compile and an unreported runtime bail. The exception is the shape that
// matters most — once the guarded block has emitted OUTPUT the fallback is
// deliberately blocked, because re-running would duplicate it, and the
// `internal_error` reaches the user in the default configuration.
//
// UPDATE 2026-08-03 (completeness-review §9.13): the PROVEN-raise shapes
// GRADUATED — a strict Error do-result fixes the handler's zero as the
// call's true arity, so the dispatch records a 0-output call and compiles
// natively (no island, no phantom slot, no re-run hazard). The refusal
// remains only where zero is not the whole story (a def over the 0-value
// result; a never-raising body whose pass-through nets one) — see
// TestErrorHandlerZeroResidualDisposition.

// TestErrorHandlerZeroResidualDisposition pins the post-§9.13 split of the
// zero-residual handler shapes. The PROVEN-raise cases (a strict Error
// do-result — the pass-through arm statically dead) compile NATIVELY: the
// zero is a fixed arity, errorReturnsFn returns it truthfully as a 0-output
// dispatch, and the strip-input shape screen's want-0 arm admits the empty
// handler residual. The shapes where zero is NOT the whole story keep the
// refusal with faithful interpreter parity: a def over the 0-value result
// (nothing to bind — both engines error identically), and a NEVER-raising
// body (the pass-through delivers one value the handler model does not).
func TestErrorHandlerZeroResidualDisposition(t *testing.T) {
	// GRADUATED — proven-raise shapes compile natively with parity.
	fnValueM2Native(t, "trailing expression", "do [1 div 0] error [drop]\n2 add 3", "[5]")
	fnValueM2Native(t, "leading value", "1\ndo [1 div 0] error [drop]", "[1]")

	// KEPT — refusal + identical outcome on both engines.
	for _, c := range []struct{ name, src string }{
		{"def-bound (nothing to bind)", "def x (do [1 div 0] error [drop])\nx"},
		{"handler never fires (pass-through nets one)", "do [1] error [drop]\n2 add 3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, _, _, _ := a.CompileCheck(c.src)
			if prog != nil {
				t.Fatalf("must refuse, compiled to:\n%s", prog.Disassemble())
			}
			ra, _ := New()
			got, err := ra.Run(c.src)
			b, _ := New()
			want, wantErr := b.RunInterp(c.src)
			if noteCompileDefect(t, c.src, got, err) {
				return
			}
			if fmt.Sprint(err) != fmt.Sprint(wantErr) {
				t.Errorf("default-mode error %v != interpreter %v", err, wantErr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("default-mode %v != interpreter %v", got, want)
			}
			// And no internal_error may escape under -force-compile: a refusal
			// there is a refusal, not a bytecode fault.
			if _, ferr := a.RunCompiledStrict(c.src); ferr != nil &&
				strings.Contains(fmt.Sprint(ferr), "internal:") {
				t.Errorf("force-compile leaked an internal error: %v", ferr)
			}
		})
	}
}

// TestErrorHandlerFencedOutputCompilesNatively — historically the variant
// that made the zero-residual bug a default-path defect: output inside the
// guarded block blocked the whole-program fallback (re-running would
// duplicate it), so the island model's phantom slot surfaced an
// internal_error to the user. Post-§9.13 the shape compiles NATIVELY — a
// 0-output dispatch, no island, no re-run — so the output is emitted once
// and the result matches the interpreter.
func TestErrorHandlerFencedOutputCompilesNatively(t *testing.T) {
	const src = `do [1 print  1 div 0] error [drop]
2 add 3`
	a, _ := New()
	prog, reason, _, _ := a.CompileCheck(src)
	if prog == nil {
		t.Fatalf("the proven-raise fenced shape must compile natively, refused: %q", reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Fatalf("must compile WITHOUT an island (a fallback would re-run the emitted output):\n%s", prog.Disassemble())
	}
	ra, _ := New()
	got, err := ra.Run(src)
	if err != nil {
		t.Fatalf("default mode errored: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("default-mode %v != interpreter %v", got, want)
	}
}

// TestErrorHandlerMultiResidualStillCompiles is the boundary that scopes the
// refusal to ZERO rather than to "any arity but one".
//
// A residual of two or more is NOT broken. Its bottom is the unconsumed
// seeded error, and the paired closure path nets one from it with a runtime
// strip. The compile-time strip in errorReturnsFn only removes that bottom
// when its identity probe matches, and after a dup/drop it does not — so
// `error [dup drop "k"]` measures 2 there while running perfectly well.
// Refusing on `!= 1` regressed exactly that shape (TestErrorStripInputClosure
// caught it), which is why the condition is `== 0`.
func TestErrorHandlerMultiResidualStillCompiles(t *testing.T) {
	const src = `do [raise x "e"] error [dup drop "k"]`
	a, _ := New()
	prog, reason, _, _ := a.CompileCheck(src)
	if prog == nil {
		t.Fatalf("a handler whose extra residual is the unconsumed error is "+
			"handled by the runtime strip and must still compile, refused: %q",
			reason)
	}
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("RunCompiledStrict: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
	}
}

// TestErrorHandlerOneResidualStillCompiles is the negative half: the fix must
// narrow ONLY the arity it cannot model. A handler that nets exactly one
// value is what the island expresses, so it must keep compiling — and keep
// compiling to the same answer.
func TestErrorHandlerOneResidualStillCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"handler nets one, raise fires", `do [1 div 0] error [drop 9]
2 add 3`, "[9 5]"},
		{"handler nets one, no raise", `do [7] error [drop 9]
2 add 3`, "[7 5]"},
		{"handler nets one, def-bound", `def x (do [1 div 0] error [drop 9])
x`, "[9]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			if prog, reason, _, _ := a.CompileCheck(c.src); prog == nil {
				t.Fatalf("a one-value handler is exactly what the island models; "+
					"it must still compile, refused: %q", reason)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
