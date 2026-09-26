package lang

import (
	"fmt"
	"strings"
	"testing"
)

// if_clause_list_test.go pins the clause-list `if [c1 b1 c2 b2 … else]`
// on the compiled lane (2026-09-26).
//
// The form was declared CompileResteps and its ReturnsFn recorded no branch,
// so every clause list holding a code body compiled to a plain CALL_NATIVE
// of IfListHandler — whose result is ifClause's SPLICE (a mark/move-if over
// a code-body condition, a `( body )` paren per arm) — and died at run time
// with `internal_error: tape-coupled handler result at if` where the
// interpreter answered: `if [[1 lt 2] [10] [20]]`, `def f fn [[x:Integer]
// [Any][if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]]] end f 5`, `for 3
// [if [[i lt 1] ['a'] [i lt 2] ['b'] ['c']]]`, a recursive fn's base case —
// every shape probed but the all-scalar list. The recording pass now lowers
// the chain as the if3 / if2 branch events it is equivalent to (basic's
// ifClauseRecord); what it cannot place declines loudly.

// TestClauseListIfCompilesWithParity: the shapes that died at run time now
// compile and answer as the interpreter does — literal and computed
// conditions, top level and fn bodies, with and without an else, bodies
// leaving one value, several, or none, statically decided clauses, nested
// clause lists, flow signals and side effects in the arms.
func TestClauseListIfCompilesWithParity(t *testing.T) {
	for _, src := range []string{
		// literal code-body conditions
		`if [true [1] false [2] [3]]`,
		`if [false [1] false [2] [3]]`,
		`if [false [1] true [2] [3]]`,
		`if [[true] [10] [20]]`,
		`if [[false] [10] [20]]`,
		// computed conditions, with and without an else
		`if [[1 lt 2] [10] [20]]`,
		`if [[1 gt 2] [10] [20]]`,
		`if [[1 gt 2] [10] [2 gt 1] [20] [30]]`,
		`if [[1 gt 2] [10] [2 gt 3] [20] [30]]`,
		`if [[1 gt 2] [10] [2 gt 3] [20]]`,
		`def x 5 end if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]`,
		`def x 50 end if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]`,
		`def x 1 end if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]`,
		// bodies leaving values, consumed downstream
		`if [[1 lt 2] [1 add 2] [3 mul 4]]`,
		`if [[1 lt 2] [1 add 2] [3 mul 4]] add 100`,
		`def x 5 end if [[x lt 3] [x] [x add 1]] mul 2`,
		`if [[1 lt 2] [10 20] [30]]`,
		`if [[1 lt 2] [1 2 3] [4]]`,
		`if [[1 lt 2] []  [3]]`,
		`if [[1 gt 2] [3] []]`,
		`if [[1 lt 2] 5 6] add 1`,
		`if [[1 lt 2] true false]`,
		// statically decided (scalar) conditions and arms
		`if [true 1 false 2 3]`,
		`if [false 1 true]`,
		`if [false 1 false 2]`,
		`if [false [1] 0 [2] '' [3] none [4] [5]]`,
		`if [true none 1]`,
		// a decided arm that nets nothing is a 0-value statement (NUR243, the
		// reverse-order NUR run's; main's #512 declined it before the merge)
		`if [true []]`,
		`if [true []] end 5`,
		`if [true [] false [1]]`,
		// inside fn bodies
		`def f fn [[x:Integer][Any][if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]]] end f 1 f 5 f 50`,
		`def f fn [[x:Integer][String][if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]]] end f 1 f 5 f 50`,
		`def f fn [[x:Integer][Integer][if [[x lt 3] [x add 1] [x mul 2]]]] end f 1 f 5`,
		`def f fn [[x:Integer][Integer][if [[x lt 3] [x] [x gt 10] [x] [0]]]] end f 1 f 50 f 5`,
		`def f fn [[x:Integer][Any][if [[x lt 3] [x add 1]]]] end f 1`,
		`def f fn [[x:Boolean][Any][if [x [1] [2]]]] end f true f false`,
		`def f fn [[x:Integer][Any][def y (x mul 2) if [[y lt 3] [y] [y add 100]]]] end f 5`,
		`def f fn [[x:Integer][Any][if [[x lt 3] [def z 1 z] [def z 2 z]]]] end f 5`,
		`def f fn [[n:Integer][Integer][if [[n lte 1] [1] [n mul (f (n sub 1))]]]] end f 5`,
		`def f fn [[n:Integer][Integer][if [[n lte 1] [n] [(f (n sub 1)) add (f (n sub 2))]]]] end f 10`,
		`def f fn [[x:Integer][Any][if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]]] end [1 5 50] each [f]`,
		`[1 5 50] each ([y:Integer] => [if [[y lt 3] ['small'] [y lt 10] ['mid'] ['big']]])`,
		// loop bodies, flow signals in the arms
		`for 3 [if [[i lt 1] ['a'] [i lt 2] ['b'] ['c']]]`,
		`for 5 [if [[i eq 3] [break] [i]]]`,
		`for 5 [if [[i eq 3] [continue] [i]]]`,
		`for 5 [if [[i eq 1] [continue] [i eq 3] [break] [i]]]`,
		// nested clause lists, defs and side effects in the arms
		`def x 1 end if [[x lt 3] [if [[x lt 2] ['a'] ['b']]] ['c']]`,
		`if [[1 lt 2] [def z 7 z] [def z 8 z]]`,
		`def log (flex []) end if [[false] [log push 'a' 1] [true] [log push 'b' 2] [log push 'c' 3]] end log`,
		`def g fn [[][Integer][5]] end if [[false] [1] [true] [g/v]]`,
		`def g fn [[][Integer][5]] end if [[1 lt 2] [g/v] [1]]`,
	} {
		requireEngineParity(t, src, true)
	}
	// The lowering is the branch events, not the handler's splice.
	if dis := compileDisasm(t, `def x 5 end if [[x lt 3] ['small'] [x lt 10] ['mid'] ['big']]`); !strings.Contains(dis, "JMP_IF_FALSE") {
		t.Errorf("a computed clause chain lowers as branches; got:\n%s", dis)
	}
}

// requireLoudDecline asserts a program DECLINES at compile time with the
// given reason — never compiles to an internal_error — and that the
// interpreter answers want. It asserts through CompileCheck and does not
// book the compile-defect ledger: these are the lowering's own declines, the
// shapes it cannot place exactly, pinned where they used to compile and die.
func requireLoudDecline(t *testing.T, src, wantReason, want string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil || err != nil {
		t.Errorf("%q: want a compile decline, got prog=%v err=%v", src, prog != nil, err)
		return
	}
	if !strings.Contains(reason, wantReason) {
		t.Errorf("%q: decline reason %q, want substring %q", src, reason, wantReason)
	}
	if _, _, errC := mustNew(t).RunCompiled(src); codeOf(errC) != "compile_failed" {
		t.Errorf("%q: the compiled lane must fail loudly as compile_failed, got %v", src, errC)
	}
	got, errI := mustNew(t).RunInterp(src)
	if errI != nil || fmt.Sprint(got) != want {
		t.Errorf("%q: interpreter answered %v / %v, want %s", src, got, errI, want)
	}
}

// TestClauseListIfDeclinesLoudly: an element the lowering cannot place — a
// condition whose truthiness is not decided at compile time, an arm the
// tape would re-step — declines through RecordBranch's uncaptured-arm
// decline with the element's reason; a clause list computed at run time
// keeps the generic record's code-body refusal; a taken
// arm leaving a fn value declines (the interpreter places it, the compile
// model would re-step it — the if3 literal-condition path compiled that
// shape to the fn's APPLICATION before 2026-09-26, `if [true] [g/v] [1]`
// answering 5 for the interpreter's `fn g`, and `10 if [true] [g/v] [1]` 15
// for `[10 fn g(Integer)]`).
func TestClauseListIfDeclinesLoudly(t *testing.T) {
	uncaptured := "if: taken-branch not captured — clause-list if: "
	for _, c := range []struct{ src, reason, want string }{
		{`def x 3 end if [true x]`, uncaptured + "a clause body that is neither a code body nor a placed value", "[3]"},
		{`def x 3 end if [false 1 x]`, uncaptured + "a clause body that is neither a code body nor a placed value", "[3]"},
		{`def x 3 end if [[true] x 4]`, uncaptured + "a clause body that is neither a code body nor a placed value", "[3]"},
		{`if [[1 lt 2] {a:1} [2]]`, uncaptured + "a clause body that is neither a code body nor a placed value", "[{a:1}]"},
		{`if [(1 lt 2) [1] [2]]`, uncaptured + "a clause condition that is neither a code body nor a scalar", "[1]"},
		{`if [{a:1} [1] [2]]`, uncaptured + "a clause condition that is neither a code body nor a scalar", "[1]"},
		{`def mk fn [[][List][quote [true [1] [2]]]] end if (mk)`, "code-body word if", "[1]"},
		{`def g fn [[][Integer][5]] end if [true [g/v]]`, "if: the taken arm leaves a fn value", "[fn g]"},
		{`def g fn [[][Integer][5]] end if [[false] [1] [g/v]]`, "if: the taken arm leaves a fn value", "[fn g]"},
		{`def g fn [[][Integer][5]] end if [true] [g/v] [1]`, "if: the taken arm leaves a fn value", "[fn g]"},
		{`def g fn [[x:Integer][Integer][x add 5]] end 10 if [true] [g/v] [1]`, "if: the taken arm leaves a fn value", "[10 fn g(Integer)]"},
		{`def h fn [[f:Function][Any][if [[true] [f/v] [1]]]] end def g fn [[][Integer][5]] end h g/v`, "if: the taken arm leaves a fn value", "[fn f]"},
		// A condition that BINDS a name: the interpreter keeps the binding
		// past the condition; the rolled-back condition fragment lost it
		// (Codex P1 on PR #512 — present on main for if2 / if3 too).
		{`def x 1 end if [[def x 5 true] [2] [3]] end x`, "the condition binds a name the interpreter keeps past it", "[2 5]"},
		{`def x 1 end if [def x 5 true] [2] [3] end x`, "the condition binds a name the interpreter keeps past it", "[2 5]"},
		{`def x 1 end if [def x 5 true] [x] [3]`, "the condition binds a name the interpreter keeps past it", "[5]"},
		{`def x 1 end if [def x 5 true] [2] end x`, "the condition binds a name the interpreter keeps past it", "[2 5]"},
		{`def x 1 end case [def x 5 1] [1 "one" "other"] end x`, "the condition binds a name the interpreter keeps past it", "[one 5]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
}
