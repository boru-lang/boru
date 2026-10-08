package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestTailCallReturnContract pins a soundness fix (found by the param-guard
// adversarial sweep): a TAIL_CALL_USER replaces the caller's frame, so the
// callee's RET returns straight to the caller's caller, BYPASSING the caller's
// declared return-type check. A `[Map]`-declared fn tail-calling an
// `[Integer]`-returning one therefore returned the Integer UNCAUGHT — the
// compiled path returned a value where the interpreter raises type_error
// (compile==interpret VIOLATION). The fix only tail-marks when the callee's
// returns conform to the caller's (tailCompatibleReturns); an incompatible
// return becomes a regular CALL_USER so the caller's RET check runs.
func TestTailCallReturnContract(t *testing.T) {
	// The unsound tail call must now RAISE the same type_error as the interpreter.
	guarded := []string{
		`def f fn [[x:Integer] [Map] [ def g fn [[y:Integer] [Integer] [y]] (g x) ]] (f 5)`,
		`def g fn [[y:Integer] [String] [y]] def f fn [[x:Integer] [Map] [ (g x) ]] (f 7)`,
	}
	for _, src := range guarded {
		t.Run("guarded", func(t *testing.T) {
			a, _ := New()
			gotC, _, errC := a.RunCompiled(src)
			if noteCompileDefect(t, src, gotC, errC) {
				return
			}
			if errC == nil {
				t.Fatalf("a [Map] fn tail-calling a non-Map returner must RAISE, not return %v", gotC)
			}
			b, _ := New()
			_, errI := b.RunInterp(src)
			if errI == nil {
				t.Fatalf("interpreter must raise too")
			}
			// Same taxonomy (the caller's return-contract type_error).
			if !strings.Contains(fmt.Sprint(errC), "type_error") || !strings.Contains(fmt.Sprint(errI), "type_error") {
				t.Errorf("want type_error in both: compiled=%v interp=%v", errC, errI)
			}
		})
	}

	// COMPATIBLE tail calls must STAY tail (O(1) frame): self-recursion (same
	// return type) and a vacuous [Any] caller contract. These compile natively and
	// run deep without exhausting the stack.
	compatible := []struct{ name, src, want string }{
		{"self-recursion same return type",
			`def sum fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [sum (n sub 1) (acc add n)]]] (sum 1000 0)`, "[500500]"},
		{"[Any]-declared caller tail-calls [Integer] (vacuous check)",
			`def g fn [[y:Integer] [Integer] [y add 1]] def f fn [[x:Integer] [Any] [(g x)]] (f 41)`, "[42]"},
	}
	for _, c := range compatible {
		t.Run("tail/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile, declined: %q", reason)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("must run, error: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) || fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s (interp %v)", got, c.want, want)
			}
		})
	}
}
