package lang

import (
	"fmt"
	"strings"
	"testing"
)

// multivalue_arm_residual_test.go pins the whole-captured MULTI-value branch
// arm. RecordBranch records how many values an arm leaves, but the lowering
// only COUNTED the slots the arm's events had left on the simulated stack —
// so a stray leftover made the count match while the values did not, and the
// arm's interior stayed unpromotable (collectPromotableEvents declined to walk
// a residualN>1 fragment at all, so a `def` inside it could never move to a
// frame slot). Both faults show in one witness:
//
//	if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]
//
// The interpreter answers `99 [7779 7780]`; the compiled lane answered
// `7778 [7778 7779]` — the `add` result (which the global bind does not pop)
// stood in for the literal 99, and the lambda's capture of `c`, never
// promoted, reached the closure push as an EVENT operand and lowered as
// `PUSH_CONST <event seq>`, capturing const 7777 instead of 7778.
//
// captureArmResidual now records the arm's residual WHOLE (event entries
// included), planValueDefLocals force-promotes each residual event to a frame
// slot, and lowerFragment re-pushes the captured list in the interpreter's
// exact order — the same linearisation the PROGRAM residual has always used.
func TestMultiValueArmResidualParity(t *testing.T) {
	rows := []struct{ src, note string }{
		// the miscompile's own shape and its mirrors: the capture reads the
		// DEFINED value, and the literal keeps its place in the residual
		{`if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]`,
			"99 [7779 7780] — was 7778 [7778 7779]"},
		{`if false [ 7 8 ] [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ]`,
			"the ELSE arm, same shape"},
		{`if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 ]`,
			"arms of unequal residual width"},
		{`def m {a: 3}  if true [ def c (m get "a")  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]`,
			"99 [4 5] — a map read as the capture; was 3 [1 2]"},
		// arms the old count-only reconciliation DECLINED ("branch leaves extra
		// values"): the bound value is now stored once and re-pushed per use
		{`if true [ def c (7777 add 1)  99 c ] [ 7 8 ]`, "99 7778 — the def read as the arm's own residual"},
		{`if true [ def c (7777 add 1)  (each [ (r:Integer => [ r add c ]) apply ] [1 2]) 99 ] [ 7 8 ]`,
			"a trailing literal above a computed residual"},
		{`if true [ def c (7777 add 1)  def d (c mul 2)  c d ] [ 7 8 ]`, "7778 15556 — a def chain, both read"},
		{`if true [ def c (7777 add 1)  99 c c ] [ 7 8 ]`, "99 7778 7778 — one def, two residual reads"},
		{`if true [ def c (7777 add 1)  99 (if (c gt 0) [ c ] [ 0 ]) ] [ 7 8 ]`,
			"a nested branch merge as the second residual"},
		// unchanged shapes: the single-value arm and the all-inert residual
		// (the loop side's capture) must lower exactly as before
		{`if true [ def c (7777 add 1)  (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]`,
			"a SINGLE-value arm — already promotable"},
		{`if true [ 99 ] [ 1 2 ]`, "99 — the all-inert 1-vs-2 merge, then arm"},
		{`if false [ 99 ] [ 1 2 ]`, "1 2 — the all-inert merge, else arm"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// The whole-arm capture DECLINES a residual that leads with a parked Function
// (the auto-apply hazard captureArmResidual screens): the arm keeps the
// count-only reconciliation, which declines, and the program falls back to the
// interpreter. The `for` side keeps its all-inert restriction untouched — a
// loop body re-pushes its residual on EVERY iteration, where a frame slot
// would hold only the last value — so its multi-out bodies lower exactly as
// before. Parity is what is pinned in every row; the declining rows are the
// negative pair for the graduations above.
func TestMultiValueArmResidualDeclines(t *testing.T) {
	decline := []struct{ src, want string }{
		{`if true [ 99 (x:Integer => [ x ]) ] [ 7 8 ]`, "[99]"},
		{`if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ (x:Integer => [ x ]) 8 ]`,
			"[99 [7779 7780]]"},
	}
	for _, c := range decline {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: a parked-Function arm residual must not compile", c.src)
			continue
		}
		if !strings.Contains(reason, "branch leaves extra values") {
			t.Errorf("%q: compile failure = %q, want the branch-residual reason", c.src, reason)
		}
		d, _ := New()
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q interp = %v/%v, want %s", c.src, gotI, errI, c.want)
		}
	}
	loops := []string{
		`for 3 [ 1 2 ]`,
		`def n 3  for n [ (n add 1) 2 ]`,
	}
	for _, src := range loops {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: the loop side must lower as before; err=%v", src, errC)
			continue
		}
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}
