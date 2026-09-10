package lang

import (
	"fmt"
	"strings"
	"testing"
)

// region_prefix_test.go is the whole-program half of the inert-prefix seating
// (NUR067's consuming half): a residual shaped [inert…, REGION] — values that
// must end up BENEATH a run whose length is a runtime value.
//
// Before OpSeatBelowMark the shape refused wholesale ("call result above a
// literal"), because a static lowering has nowhere to put the prefix: pushing
// it after the region lands it ON TOP of the run, and there is no static
// offset that reaches past a runtime count. The lowering now opens a mark
// before the region's producing event and closes with SEAT_BELOW_MARK, which
// lifts the prefix down to the mark without naming the run's length.
//
// The rows below are the two producers of a single-slot region — a
// value-producing loop and await's winner-takes-all residual — at the counts
// that matter (many / one / zero), plus the declines.

func rpRun(t *testing.T, src string) (string, bool, error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, cerr := a.RunCompiled(src)
	return fmt.Sprintf("%v", out), compiled, cerr
}

func rpInterp(t *testing.T, src string) string {
	t.Helper()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("RunInterp(%q): %v", src, err)
	}
	return fmt.Sprintf("%v", out)
}

func TestRegionPrefixCompilesWithParity(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A value-producing loop under one inert value — the frontier's
		// `99 for 3 [i]`.
		{`99 for 3 [i]`, "[99 0 1 2]"},
		// ZERO iterations: the run is empty and the prefix is the whole
		// residual. A fixed-count lowering would have had to know that.
		{`99 for 0 [i]`, "[99]"},
		// A TWO-value prefix keeps its own order beneath the run.
		{`1 2 for 3 [i]`, "[1 2 0 1 2]"},
		// A computed body: bodyOut is an event operand inside the loop's
		// FRAGMENT, which must not read as an enclosing-stack operand.
		{`99 for 2 [(1 add 2)]`, "[99 3 3]"},
		// A multi-value body — two values per iteration under the prefix.
		{`5 for 2 [1 2]`, "[5 1 2 1 2]"},
		// Non-Integer prefix values, so nothing depends on the element type.
		{`'a' 'b' for 2 [i mul 2]`, "[a b 0 2]"},
		// The other producer: await's winner-takes-all region (NUR067), at
		// three values and at zero.
		{`import "boru:time-util" 99 TimeUtil.await {mode:"first"} [[1 2 3]]`, "[99 1 2 3]"},
		{`import "boru:time-util" 99 TimeUtil.await {mode:"first"} [[]]`, "[99]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			got, compiled, err := rpRun(t, tc.src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("the inert prefix beneath a region must compile")
			}
			if got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			if in := rpInterp(t, tc.src); in != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", in, tc.want)
			}
		})
	}
}

// TestRegionPrefixEmitsTheMarkAndSeat pins the STREAM, not just the answer: a
// wrong answer is one way this can break, and an accidental fixed-count
// lowering that happens to agree on these rows is the other. The mark must
// open BEFORE the region's operands are pushed, and the seat must close it.
func TestRegionPrefixEmitsTheMarkAndSeat(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(`1 2 for 3 [i]`)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog == nil {
		t.Fatalf("refused: %s", reason)
	}
	dis := prog.Disassemble()
	if !strings.HasPrefix(dis, "0000 STACK_MARK") {
		t.Errorf("the mark must open the program, before the loop's own operands:\n%s", dis)
	}
	if !strings.Contains(dis, "SEAT_BELOW_MARK") {
		t.Errorf("the seat must close the region:\n%s", dis)
	}
	// The prefix is pushed ABOVE the finished run, after FOR_NEXT's loop —
	// so both pushes come after the loop body's jump target, not before the
	// mark.
	seat := strings.Index(dis, "SEAT_BELOW_MARK")
	loop := strings.Index(dis, "FOR_SETUP")
	if seat < loop {
		t.Errorf("the seat must follow the region it closes:\n%s", dis)
	}
}

// TestRegionPrefixDeclinesAStackReadingRegion — the one shape the plan must
// stand aside for, and the reason it exists as a check rather than a hope: the
// loop's BOUND is a live `get` result, which sits BELOW the mark the plan
// would open, so the loop would pop from beneath its own mark. Declining
// keeps the pre-existing refusal and the interpreter's answer.
func TestRegionPrefixDeclinesAStackReadingRegion(t *testing.T) {
	const src = `def m {n:3} 99 for (m get "n") [i]`
	_, compiled, err := rpRun(t, src)
	if compiled {
		t.Fatal("a region whose operand is live on the enclosing stack must decline the plan")
	}
	if err == nil || !strings.Contains(err.Error(), "residual shape beyond Stage 1 (call result above a literal)") {
		t.Fatalf("refusal reason drifted: %v", err)
	}
	if got := rpInterp(t, src); got != "[99 0 1 2]" {
		t.Errorf("interpreter %s, want [99 0 1 2]", got)
	}
}
