package lang

import (
	"fmt"
	"testing"
)

// paren_trailing_forward_test.go pins NUR184 (recorded 2026-09-23 while
// closing NUR180): a paren's TRAILING fn value is not applied at the
// paren's close — it is dispatched like a word AFTER it, forward-collecting
// the tokens past the `)` first and falling to the values inside the paren
// only when nothing collectable follows — and a paren closing UNDER A
// PENDING FORWARD hands its survivors to that forward, the fn value
// re-stepping over the forward's result. The compiled trailing-apply
// record (recordParenTrailingFnApply) applies the value over the values
// INSIDE the paren, which is right only for the no-follower case.

const ptfMk = `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  `
const ptfInc = `def inc2 fn [[x:Integer][Integer][x add 2]]  `

// TestParenTrailingFnForwardCollectsPending pins the open divergences: the
// pin fails the day a row agrees (move it to TestParenTrailingFnAgrees and
// retire NUR184 when none is left) or becomes loud.
func TestParenTrailingFnForwardCollectsPending(t *testing.T) {
	rows := []struct{ src, interp, note string }{
		{ptfMk + `(2 (mk 1)) 10`, "[2 11]", "the closure collects the literal after the paren, the 2 stays"},
		{ptfMk + `(2 (mk 1)) 10 20`, "[2 11 20]", "only the first"},
		{ptfMk + `5 (2 (mk 1)) 10`, "[5 2 11]", "under a deeper value"},
		{ptfMk + `((2 (mk 1)) 10)`, "[2 11]", "inside an enclosing paren"},
		{ptfMk + `[(2 (mk 1)) 10]`, "[[2 11]]", "inside a list literal"},
		{ptfMk + `(2 (mk 1)) 10 mul`, "[22]", "the collected result under a later stack word"},
		{ptfInc + `(2 inc2/v) 10`, "[2 12]", "a named fn value the same way"},
		{ptfInc + `(2 inc2/v) 10 mul`, "[24]", "and under a later stack word"},
		{ptfMk + `10 mul (2 (mk 1))`, "[21]", "a paren under a pending forward hands its survivors to it: mul takes the 2, the closure re-steps over 20"},
		{ptfMk + `def f fn [[Integer][Any][10 mul (2 (mk 1))]]  f 1`, "[21]", "the same inside a fn unit"},
		{ptfMk + `def xs [1 2 3]  xs each [(2 (mk 1)) 10]`, "[[11 11 11]]", "the follower case inside a code body"},
		{ptfMk + `def r (2 (mk 1)) end r`, "[fn (Integer) 2]", "def's pending forward takes the closure, the 2 stays"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter oracle moved: %v err=%v, want %s — re-derive NUR184 (%s)", c.src, gotI, errI, c.interp, c.note)
			continue
		}
		if !compiled || errC != nil {
			t.Errorf("%q: NUR184 became loud (%v) — record that and retire this pin's row (%s)", c.src, errC, c.note)
			continue
		}
		if fmt.Sprint(gotC) == c.interp {
			t.Errorf("%q: NUR184 retired for this row (compiled %v agrees) — move it to TestParenTrailingFnAgrees (%s)", c.src, gotC, c.note)
		}
	}
}

// TestParenTrailingFnAgrees: the shapes the record models correctly — no
// collectable follower (the end, a word, a literal the fn's param rejects,
// a LEADING fn that collects inside the paren).
func TestParenTrailingFnAgrees(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{ptfMk + `(2 (mk 1))`, "[3]", "nothing follows: the closure takes the 2"},
		{ptfMk + `(2 (mk 1)) "s"`, "[3 s]", "a follower the param rejects: the closure takes the 2"},
		{ptfMk + `(2 (mk 1)) mul 10`, "[30]", "a word follows"},
		{ptfMk + `(2 (mk 1)) add 10`, "[13]", "a word follows"},
		{ptfMk + `((mk 1) 2) 10`, "[3 10]", "a LEADING fn collects inside the paren"},
		{ptfMk + `def xs [1 2 3]  xs each [(2 (mk 1))]`, "[[3 3 3]]", "a body's tail"},
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
