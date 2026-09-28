package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// cover_pr505_main_test.go closes merged-coverage gaps (ADR-008) with
// programs that reach them on both lanes: the typed def's predicate-raise
// arm (basic), the strict-Any dyn-body recovery's fallback skip (check), the
// branch-carried def's taken-arm and dyn-scope-pre arms and the user-poly
// zero-return decline (compiler), the fn-value seam's count contract (eng),
// and walk's fn-value closure hook (lang/native).

// TestCoverPR505MainPredicateBodyRaises: a predicate type whose body RAISES
// over the candidate fails the typed def with the predicate's own error,
// wrapped with the def's name and the type (defFnPredicateBind's error arm) —
// the same error on both lanes. The member and non-member twins keep the
// verdict convention: a raise is neither.
func TestCoverPR505MainPredicateBodyRaises(t *testing.T) {
	const p = `def P fnpred n:Integer [(10 div n) gt 1] `
	for _, tc := range []struct{ src, want string }{
		{p + `def v:P 2 v`, "[2]"},
		{p + `def v:P 0 v`, "ERROR:def v: predicate type P: [boru/arith_error]: division by zero"},
		{`def Boom fnpred n:Integer [raise io_error "boom"] def v:Boom 4 v`, "ERROR:def v: predicate type Boom: [boru/io_error]: boom"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	// A non-member is a verdict, not a raise: the type_error names the value.
	// The pure predicate runs for real at analysis time (NUR141), so the check
	// pass refuses it statically; the interpreter raises the same refusal.
	if _, err := mustNew(t).RunInterp(p + `def v:P 20 v`); err == nil || !strings.Contains(err.Error(), "def v: value 20 does not satisfy predicate type P") {
		t.Errorf("a non-member must be refused as a verdict, got %v", err)
	}
}

// TestCoverPR505MainDynBodyRecoverySkipsFallback: `fold` extended with a boru
// overload (anchored on a class the program mints, as extend_owner requires)
// carries the synthetic 0-arg fallback in its dispatch aggregate. The
// strict-Any dyn-body recovery — `fold` over a class field typed Any — takes
// the WIDEST satisfiable non-fallback overload (widestSatisfiableOverload
// skips the fallback, which every window satisfies), so the seeded form folds
// the live list with its seed and the seedless form without one, on both
// lanes; the extension itself still dispatches on its own type.
func TestCoverPR505MainDynBodyRecoverySkipsFallback(t *testing.T) {
	const ext = `def Bag class {n: Integer} end def fold fn [[b:Bag] [Integer] [b.n]] end `
	const box = `def Box class {data: Any} end def b (make Box {data: [1 2 3]}) end `
	for _, tc := range []struct{ src, want string }{
		{ext + box + `10 fold [add] b.data`, "[16]"},
		{ext + box + `fold [add] b.data`, "[6]"},
		{ext + `fold (make Bag {n: 4})`, "[4]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestCoverPR505MainConstBranchTakenJoins: a LITERAL-condition `if` analyses
// only the taken arm, whose defs join as TAKEN (core.InstallTakenArmDefs).
// The branch-carried seating skips a taken def with no pre binding — the arm
// pushed its own value, whose home stands — and seats one over a pre binding
// into the name's slot, whichever arm was taken (carryBranchJoin's taken
// arm). The untaken arm's def never binds: the interpreter's read of it
// raises.
func TestCoverPR505MainConstBranchTakenJoins(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`if [true] [def y 1 y] [2] end y`, "[1 1]"},
		{`def x 0 if [true] [def x 1 x] [3] end x`, "[1 1]"},
		{`def x 0 if [false] [3] [def x 1 x] end x`, "[1 1]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	if _, err := mustNew(t).RunInterp(`if [false] [def y 1 y] [2] end y`); err == nil || !strings.Contains(err.Error(), "undefined word: y") {
		t.Errorf("the untaken arm's def must leave the name unbound, got %v", err)
	}
}

// TestCoverPR505MainLoopSplitPreJoin: a top-level def of a statically counted
// loop region binds the region's FIRST value (S5) — a binding with no event
// home, which a read resolves through the registry (a dyn-scope operand). A
// branch arm rebinding that name cannot seed the branch-carried slot from a
// dyn-scope read (carryBranchJoin refuses the seed), so the name is left
// unseated and its read after the merge takes the path it always did — the
// program compiles and agrees whichever arm runs.
func TestCoverPR505MainLoopSplitPreJoin(t *testing.T) {
	const pre = `def x (for 2 [5]) x drop `
	for _, tc := range []struct{ src, want string }{
		{`def c true ` + pre + `if c [def x 1] [] end x`, "[5 1]"},
		{`def c false ` + pre + `if c [def x 1] [] end x`, "[5 5]"},
		{`def c true ` + pre + `if c [def x 1] [def x 2] end x`, "[5 1]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestCoverPR505MainZeroReturnPolyDeclines: a multi-overload user fn with a
// fn-PREDICATE param and two reachable arms — an Integer carrier reaches both
// [x:Pos] and [x:Any] — is a predicate hazard, which must ride the user-poly
// run-time re-match. A ZERO-declared-return set whose arm leaves a residual
// (`[x:Pos] [] [x]`) cannot ride a 0-output call site, so the bake declines
// (all-or-nothing, tryCompileUserPolyArms) and the program keeps the hazard's
// named compile failure (planUserPolyDispatch); the interpreter selects the
// Any arm at run time. The twin whose arms all net zero bakes, rides
// CALL_USER_POLY, and prints what the interpreter prints.
func TestCoverPR505MainZeroReturnPolyDeclines(t *testing.T) {
	const pos = "def Pos fnpred [[n:Integer] [n gt 0]]\n"
	declining := pos + "def zpick fn [\n  [x:Pos] [] [x]\n  [x:Any] [] [0]\n]\ndef w fn [[n:Integer] [Any] [zpick n]]\nw -3"
	prog, reason, _, err := mustNew(t).CompileCheck(declining)
	const want = "fn-predicate-typed overload dispatch at `zpick` is runtime-evaluated (no poly re-match)"
	if err != nil || prog != nil || reason != want {
		t.Errorf("the residual-leaving zero-return arm must decline the bake: prog=%v reason=%q err=%v", prog != nil, reason, err)
	}
	if got, ierr := mustNew(t).RunInterp(declining); ierr != nil || fmt.Sprint(got) != "[0]" {
		t.Errorf("the interpreter selects the Any arm: got %v err %v", got, ierr)
	}

	baking := pos + "def zp fn [\n  [x:Pos] [] [\"p\" print]\n  [x:Any] [] [\"o\" print]\n]\ndef w fn [[n:Integer] [] [zp n]]\nw -3"
	if dis := compileDisasm(t, baking); !strings.Contains(dis, "CALL_USER_POLY") {
		t.Errorf("an all-zero-net arm set bakes to CALL_USER_POLY:\n%s", dis)
	}
	c, i := mustNew(t), mustNew(t)
	var cOut, iOut bytes.Buffer
	c.SetOutput(&cOut)
	i.SetOutput(&iOut)
	gotC, compiled, errC := c.RunCompiled(baking)
	gotI, errI := i.RunInterp(baking)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || cOut.String() != "o\n" || iOut.String() != "o\n" {
		t.Errorf("baked poly parity: compiled=%v %v/%v out=%q, interp %v/%v out=%q", compiled, gotC, errC, cOut.String(), gotI, errI, iOut.String())
	}
}

// TestCoverPR505MainFnValueSeamCountContract: a NAMED fn value read off a
// class field (`each h.cb [...]`) reaches the token seam as a value: its unit
// is hosted natively and its OWN return contract enforced over the residual
// (checkFnValueReturn). A body leaving two values for one declared return
// raises the count error under the fn's name on both lanes; a conforming
// body maps the list.
func TestCoverPR505MainFnValueSeamCountContract(t *testing.T) {
	const cls = `def Handler class {cb: Function} `
	for _, tc := range []struct{ src, want string }{
		{`def two fn [[n:Integer][Integer][n 1]] end ` + cls + `def h (make Handler {cb: two/v}) each h.cb [1 2 3]`, "ERROR:two: expected 1 return value(s), got 2 — [1 1]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end ` + cls + `def h (make Handler {cb: inc/v}) each h.cb [1 2 3]`, "[[2 3 4]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestCoverPR505MainWalkFnValueClosureHook: a CAPTURING lambda minted by a
// factory is a compiled closure at run time. walk's hook classification
// matches such a fn-value closure against its own signature before it runs
// (walkClassifyHook's ClosureIsFnValue arm, callWalkHook's no-match), exactly
// as the lambda branch matches a lambda: a hook over Any runs at every node,
// and a hook whose signature cannot take the node payload raises walk's
// no-match — the interpreter's own error on the compiled lane.
func TestCoverPR505MainWalkFnValueClosureHook(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def acc (flex []) end def mk fn [[tag:String][Function][([m:Any] => [acc (tag add m.path) append])]] end def h (mk "x") end walk {mode: "depth"} {a:1 b:2} h/v ; acc`, "[{a:1 b:2} ['x' 'xa' 'xb']]"},
		{`def mk fn [[tag:String][Function][([m:Integer] => [tag print])]] end def h (mk "x") end walk {mode: "depth"} {a:1} h/v`, "ERROR:walk: no matching hook signature"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}
