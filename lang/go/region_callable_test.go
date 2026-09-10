package lang

import (
	"fmt"
	"strings"
	"testing"
)

// region_callable_test.go pins the one thing a runtime-variadic REGION must
// never do: carry a value the interpreter would RE-STEP.
//
// A Function that arrives on the stack dispatches over what is beneath it —
// that is the rule NUR124 and OpDeoptIfFn exist for. A region has no
// per-value seat, so nothing in the compiled lane can re-step one, and the
// two lanes then disagree outright:
//
//	def g fn [[x:Integer] [Integer] [x add 1]]  5 for 1 [g/v]
//	  interpreted  [6]        — g/v arrives above 5 and dispatches
//	  compiled     [5 fn]     — the region's values are appended as data
//
// So every region CONSUMER declines a region whose run may carry a callable,
// and `await`'s winner residual is not recorded as a region at all when a
// branch body can leave one. The rows below are the refusals; each is
// asserted with the interpreter's answer beside it, because a refusal that
// changed the answer would be no better than the divergence it prevents.
//
// The BARE loop residual (`for 2 [g/v]`) is NOT here: it diverges the same
// way and did so before any of this, and refusing it needs a precise callable
// test rather than this deliberately widening one — recorded as NUR129.

func TestRegionCarryingACallableRefuses(t *testing.T) {
	const g = `def g fn [[x:Integer] [Integer] [x add 1]] `
	const tu = `import "boru:time-util" `
	for _, tc := range []struct{ src, reason, want string }{
		// await's winner residual: the branch body leaves [5, g/v], which the
		// interpreter splices back out of the handler's results and re-steps.
		{g + tu + `TimeUtil.await {mode:'first'} [[5 g/v]]`,
			"a branch body can leave a CALLABLE value", "[6]"},
		{g + tu + `TimeUtil.await {mode:'any'} [[5 g/v]]`,
			"a branch body can leave a CALLABLE value", "[6]"},
		{g + tu + `99 TimeUtil.await {mode:'first'} [[5 g/v]]`,
			"a branch body can leave a CALLABLE value", "[99 6]"},
		{g + tu + `size [(TimeUtil.await {mode:'first'} [[5 g/v]])]`,
			"a branch body can leave a CALLABLE value", "[1]"},
		// The LOOP producer, at the two consumers this batch added. Both fall
		// back to the refusal each had before the consumer existed.
		{g + `5 for 1 [g/v]`,
			"residual shape beyond Stage 1 (call result above a literal)", "[6]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, compiled, cerr := a.RunCompiled(tc.src)
			if compiled {
				t.Fatal("a region that may carry a callable must not compile: only the interpreter re-steps one")
			}
			if cerr == nil || !strings.Contains(cerr.Error(), tc.reason) {
				t.Fatalf("refusal reason drifted: want %q, got %v", tc.reason, cerr)
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := b.RunInterp(tc.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if got := fmt.Sprintf("%v", gotI); got != tc.want {
				t.Errorf("interpreter oracle %s, want %s", got, tc.want)
			}
		})
	}
}

// The COLLECT twin, split out because its interpreter answer is an error
// rather than a value (`g/v` inside a loop body arrives with nothing beneath
// it, so the interpreter raises where it would otherwise dispatch).
func TestRegionCollectOfACallableRefuses(t *testing.T) {
	const src = `def g fn [[x:Integer] [Integer] [x add 1]] [(for 2 [g/v])]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, compiled, cerr := a.RunCompiled(src)
	if compiled {
		t.Fatal("collecting a region that may carry a callable must not compile")
	}
	if cerr == nil || !strings.Contains(cerr.Error(), "consumes loop results") {
		t.Fatalf("refusal reason drifted: %v", cerr)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.RunInterp(src); err == nil || !strings.Contains(err.Error(), "uncalled_function") {
		t.Fatalf("interpreter oracle drifted: %v", err)
	}
}
