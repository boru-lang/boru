package lang

import (
	"fmt"
	"strings"
	"testing"
)

// loop_flow_trim_test.go is the whole-program half of the fiftieth
// increment (NUR132): a `break` / `continue` whose loop is in the SAME unit
// used to lower to a bare OpJmp. The jump's destination was right and the
// jump was wrong, because the two things the VM's flow signal ALSO does are
// the two things the interpreter does:
//
//   - TRIM THE ROUND — the interpreter splices the round's tape back to its
//     mark, so a value this round already produced is discarded;
//   - POP THE LOOP on a break — a jump to the loop's end lands past its
//     FOR_NEXT, the only op that pops, so the loop's counter leaked into
//     whatever read `loops` next.
//
// Both were silent wrong answers on the DEFAULT lane, and the second was a
// non-terminating program.

// lftRun runs a source on both lanes and reports whether the compiled lane
// ran natively and whether the two agree.
func lftRun(t *testing.T, src string) (ran bool, gotC, gotI string, cerr, ierr error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vC, ran, cerr := a.RunCompiled(src)
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vI, ierr := b.RunInterp(src)
	return ran, fmt.Sprint(vC), fmt.Sprint(vI), cerr, ierr
}

// TestLoopFlowSignalTrimsTheRound — a value the round already produced does
// not survive the round's own break/continue. Every row needs a COMPUTED
// prefix: the const twin (`for 3 [ 9 if … ]`) refuses at "branch leaves
// extra values", which is why the corpus never carried a witness.
func TestLoopFlowSignalTrimsTheRound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The minimal witness: one round, one computed value, one break.
		{`while [true] [ (7 add 2) if true [break] [5] end ]`, "[]"},
		// continue and break over three rounds, with and without a tail.
		{`for 3 [ (7 add 2) if (i eq 2) [continue] [5] end ]`, "[9 5 9 5]"},
		{`for 3 [ (7 add 2) if (i eq 2) [continue] [5] end i ]`, "[9 5 0 9 5 1]"},
		{`for 3 [ (7 add 2) if (i eq 2) [break] [5] end i ]`, "[9 5 0 9 5 1]"},
		{`for 3 [ (7 add 2) (1 add 1) if (i eq 2) [continue] [5] end ]`, "[9 2 5 9 2 5]"},
		{`for 3 [ (7 add 2) if (i eq 2) [continue] [5] end (i add 1) ]`, "[9 5 1 9 5 2]"},
		// The negatives that always agreed and must keep agreeing: a round
		// with nothing to trim, and a condition that never takes the arm.
		{`for 3 [ (7 add 2) continue ]`, "[]"},
		{`for 3 [ (7 add 2) if false [continue] [5] end i ]`, "[9 5 0 9 5 1 9 5 2]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, gotC, gotI, cerr, ierr := lftRun(t, tc.src)
			if !ran {
				t.Fatalf("must still compile: %v", cerr)
			}
			if cerr != nil || ierr != nil {
				t.Fatalf("unexpected errors: compiled %v / interp %v", cerr, ierr)
			}
			if gotI != tc.want {
				t.Fatalf("the interpreter ORACLE moved: %s, want %s", gotI, tc.want)
			}
			if gotC != gotI {
				t.Errorf("compiled %s, interpreted %s", gotC, gotI)
			}
		})
	}
}

// TestLoopBreakPopsItsLoop — the second half, and the one that did not merely
// answer wrongly. An inner loop's break left its counter on the VM's open-loop
// stack, so the OUTER loop's FOR_NEXT stepped the inner loop's counter and the
// program never terminated (it died at the growth ceiling).
func TestLoopBreakPopsItsLoop(t *testing.T) {
	const src = `for 2 [ (i add 0) end for 3 [ if (i eq 1) [break] [0] end ] ]`
	ran, gotC, gotI, cerr, ierr := lftRun(t, src)
	if !ran {
		t.Fatalf("must still compile: %v", cerr)
	}
	if ierr != nil {
		t.Fatalf("the interpreter oracle errored: %v", ierr)
	}
	if cerr != nil {
		if strings.Contains(cerr.Error(), "growth ceiling") {
			t.Fatalf("the inner break leaked its loop — the outer loop never terminates: %v", cerr)
		}
		t.Fatalf("compiled error: %v", cerr)
	}
	if gotI != "[0 0 1 0]" {
		t.Fatalf("the interpreter ORACLE moved: %s", gotI)
	}
	if gotC != gotI {
		t.Errorf("compiled %s, interpreted %s", gotC, gotI)
	}
}
