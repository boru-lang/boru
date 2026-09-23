package lang

import (
	"fmt"
	"strings"
	"testing"
)

// paren_trailing_forward_test.go pins NUR184 (recorded 2026-09-23 while
// closing NUR180, FIXED the same day): a paren's TRAILING fn value is not
// applied at the paren's close — it is dispatched like a word AFTER it,
// forward-collecting the tokens past the `)` first and falling to the
// values inside the paren only when nothing collectable follows — and a
// paren closing UNDER A PENDING FORWARD hands its survivors to that
// forward, the fn value re-stepping over the forward's result. The
// compiled trailing-apply record (recordParenTrailingFnApply) applied the
// value over the values INSIDE the paren whatever followed.
//
// The collapse now records that apply only for a lead the interpreter
// dispatches INSIDE the paren — a bare word read of a fn-typed binding
// (`(5 3 comp)`, CheckState.WordReadFnIDs), a lead the `apply` word owns —
// or a fn VALUE nothing after the close can collect; a value with a
// collectable follower is left for the rewind to re-step, and a paren under
// a pending forward marks its trailing value as that collection's LEFTOVER
// (CheckState.ForwardLeftoverFnIDs), which the residual's lead arm never
// applies over later values. The mixed-window island's interpreter then
// SEALS a compiled closure at its collection's completion as it seals a
// FnDefInfo (NUR124's payload axis: unsealed, `(2 (mk 1)) 10 20` walked
// the closure past every literal and applied it to the last). A native
// poly record that collected a re-step-marked carrier as its operand
// declines (polyCallDeclineReason): the check pass stepped the value as
// data where the interpreter dispatches it first.

const ptfMk = `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  `
const ptfInc = `def inc2 fn [[x:Integer][Integer][x add 2]]  `

// TestParenTrailingFnAgrees: every row compiles and agrees with the
// interpreter — the shapes the record always modelled (no collectable
// follower, a LEADING fn) and NUR184's witnesses.
func TestParenTrailingFnAgrees(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{ptfMk + `(2 (mk 1))`, "[3]", "nothing follows: the closure takes the 2"},
		{ptfMk + `(2 (mk 1)) "s"`, "[3 s]", "a follower the param rejects: the closure takes the 2"},
		{ptfMk + `(2 (mk 1)) mul 10`, "[30]", "a word follows"},
		{ptfMk + `(2 (mk 1)) add 10`, "[13]", "a word follows"},
		{ptfMk + `((mk 1) 2) 10`, "[3 10]", "a LEADING fn collects inside the paren"},
		{ptfMk + `def xs [1 2 3]  xs each [(2 (mk 1))]`, "[[3 3 3]]", "a body's tail"},
		// NUR184's witnesses: the trailing closure collects the literal
		// after the paren and the 2 stays
		{ptfMk + `(2 (mk 1)) 10`, "[2 11]", "the closure collects the literal after the paren"},
		{ptfMk + `(2 (mk 1)) 10 20`, "[2 11 20]", "only the first follower (the island's seal)"},
		{ptfMk + `(2 (mk 1)) 10 20 30`, "[2 11 20 30]", "and no further"},
		{ptfMk + `(2 (mk 1)) 10 "s"`, "[2 11 s]", "a later follower the param rejects stays"},
		{ptfMk + `5 (2 (mk 1)) 10`, "[5 2 11]", "under a deeper value"},
		{ptfMk + `5 (2 (mk 1)) 10 20`, "[5 2 11 20]", "under a deeper value, two followers"},
		{ptfMk + `((2 (mk 1)) 10)`, "[2 11]", "inside an enclosing paren"},
		{ptfInc + `(2 inc2/v) 10`, "[2 12]", "a named fn value the same way"},
		{ptfInc + `(2 inc2/v) 10 mul`, "[24]", "and under a later stack word"},
		{ptfInc + `(2 inc2/v) 10 20`, "[2 12 20]", "a named fn value, two followers"},
		{ptfMk + `def xs [1 2 3]  xs each [(2 (mk 1)) 10]`, "[[11 11 11]]", "the follower case inside a code body"},
		{ptfMk + `def xs [1 2 3]  xs each [(2 (mk 1)) 10 mul]`, "[[22 22 22]]", "the collected result under a later stack word, inside a code body"},
		// a paren under a pending forward hands its survivors to it: mul
		// takes the 2, the closure re-steps over the 20
		{ptfMk + `def f fn [[Integer][Any][10 mul (2 (mk 1))]]  f 1`, "[21]", "the leftover closure applies over the word's result inside a fn unit"},
		{ptfMk + `10 mul (2 (mk 1)) 5`, "[20 6]", "the leftover closure collects the token after the group"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestParenTrailingFnSoundCompileFailures: NUR184's residue DECLINES, each
// row with the interpreter's own answer beside it — a leftover closure at
// the main program (no arm seats a re-step over a word's result there), a
// leftover with nothing beneath it (def takes the 2, the closure parks),
// a list literal whose trailing element the rewind would have re-stepped
// over the next element, and a numeric word after the follower, whose
// poly record collected the re-step-marked carrier the interpreter
// dispatches first (22: the closure takes the 10 before `mul` runs).
func TestParenTrailingFnSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		{ptfMk + `10 mul (2 (mk 1))`, "unconsumed fn-value carrier", "[21]"},
		{ptfMk + `def r (2 (mk 1)) end r`, "fn value precedes residual args", "[fn (Integer) 2]"},
		// The `end` between the leftover closure and the values after it is a
		// proven statement boundary (NUR187): the closure is data beneath them
		// on both lanes, and the row declines at the render gate instead.
		{ptfMk + `def r (2 (mk 1)) end r 5`, "unconsumed fn-value carrier", "[fn (Integer) 2 5]"},
		{ptfMk + `[(2 (mk 1)) 10]`, "unknown provenance", "[[2 11]]"},
		{ptfMk + `(2 (mk 1)) 10 mul`, "the paren's rewind re-steps first", "[22]"},
		{ptfMk + `(2 (mk 1)) 10 add`, "the paren's rewind re-steps first", "[13]"},
		{ptfMk + `(2 (mk 1)) 10 drop`, "collected by a later dispatch", "[2]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a compile failure", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}
