package lang

import (
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
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
	// A loud decline is a GENERIC-path pin: a constant fn arg would take a
	// call-site specialised unit instead.
	prog, reason, _, err := mustNewNoSpec(t).CompileCheck(src)
	if prog != nil || err != nil {
		t.Errorf("%q: want a compile decline, got prog=%v err=%v", src, prog != nil, err)
		return
	}
	if !strings.Contains(reason, wantReason) {
		t.Errorf("%q: decline reason %q, want substring %q", src, reason, wantReason)
	}
	if _, _, errC := mustNewNoSpec(t).RunCompiled(src); codeOf(errC) != "compile_failed" {
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
		// A condition or scrutinee that binds a name with `def` is a block's
		// shadow since phase 2 (design/IMMUTABLE-DEF.1.md §2.1): the
		// binding ends with the condition, the interpreter answers over the
		// enclosing binding, and the compiled lane declines the shadow until
		// its own block scopes land (the `var` spellings compile:
		// TestConditionBindingCompilesWithParity).
		{`def x 1 end if [do [def x 5] true] [2] [3] end x`, "block-local def `x` shadows an enclosing binding", "[2 1]"},
		{`def x 1 end 7 if [do [def x 5] drop true] [2] [3] end x`, "block-local def `x` shadows an enclosing binding", "[2 1]"},
		{`def x 1 end if [5 def x 2 true] [2] [3] end x`, "block-local def `x` shadows an enclosing binding", "[2 1]"},
		{`def x 1 end case [def x 5 1 2] [2 "two" "other"] end x`, "block-local def `x` shadows an enclosing binding", "[two 1]"},
		{`def x 1 end case [def x 5 x] [5 "five" 6 "six" "other"] end x`, "block-local def `x` shadows an enclosing binding", "[five 1]"},
		{`def x 1 end case [def x 5 x] [5 [x] [0]] end x`, "block-local def `x` shadows an enclosing binding", "[5 1 1]"},
		{`def x 1 end do [if [def x 5 true] [2] [3]] end x`, "block-local def `x` shadows an enclosing binding", "[2 1]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
}

// TestConditionBindingCompilesWithParity: a code-body `if` / `case`
// condition that ASSIGNS a var. The condition runs unconditionally, exactly
// once, before the branch decision — the interpreter runs it inline — and
// it is a BLOCK (design/IMMUTABLE-DEF.1.md §2.1, phase 2): a `def` inside
// it ends with the condition (block_scope_rule_test.go keeps those shapes;
// NUR212's kept binding is retired), where a `var` assignment reaches the
// enclosing cell for the arms and for everything after the construct. The
// assigning condition compiles on every form — if2, if3, the clause-list
// `if`, `case`'s code-body scrutinee — at the top level, in fn and loop
// bodies and nested, with results identical to the interpreter's. Every
// compiled row also replays its bind twins in the pass's order
// (TestBindTwinOpsArePlacedOrderedSubset's invariant).
func TestConditionBindingCompilesWithParity(t *testing.T) {
	for _, src := range []string{
		// read after the if — if3, if2, the clause-list if, case
		`var x 1 end if [var x 5 true] [2] [3] end x`,
		`var x 1 end if [var x 5 false] [2] [3] end x`,
		`var x 1 end if [var x 5 true] [2] end x`,
		`var x 1 end if [var x 5 false] [2] end x`,
		`var x 1 end if [[var x 5 true] [2] [3]] end x`,
		`var x 1 end if [[var x 2 false] [10] [var x (x add 3) true] [20] [30]] end x`,
		`var x 1 end if [[var x 2 true] [10] [var x (x add 3) true] [20] [30]] end x`,
		`var x 1 end if [[var x 2 false] [10] [var x (x add 3) false] [20] [30]] end x`,
		`var x 1 end if [[var x 2 false] [10] [var x (x add 3) false] [20]] end x`,
		`var x 1 end case [var x 5 1] [1 "one" "other"] end x`,
		`var x 1 end case [var x 5 x] [5 "five" "other"] end x`,
		`var x 1 end case [var x 5 x] [6 "six" "other"] end x`,
		// read in the taken arm, and in the else arm
		`var x 1 end if [var x 5 true] [x] [3]`,
		`var x 1 end if [var x 5 false] [2] [x]`,
		`var x 1 end if [[var x 2 false] [x] [var x (x add 3) true] [x] [30]]`,
		// a declared var, a computed value, a condition reading its own assignment
		`var y 0 end if [var y 5 true] [y] [3] end y`,
		`var y 0 end if [var y 5 false] [y] [y add 1] end y`,
		`def a 3 end var y 0 end if [var y (a add 4) true] [y] [3] end y`,
		`def a 3 end var y 0 end if [var y (a add 4) (y gt 5)] [y mul 2] [y] end y`,
		`var x 1 end if [var x (x add 1) (x gt 1)] [x] [0] end x`,
		`var x [1 2] end if [var x (x push 3) true] [x] [0] end x`,
		`var m {} end if [var m {a:1} true] [m.a] [0] end m`,
		`var y 0 end case [var y 7 y] [7 "seven" "other"] end y`,
		// an arm assigning the condition's var again
		`def a 3 end var y 0 end if [var y (a add 4) (y gt 5)] [var y 0] [] end y`,
		`def a 3 end var y 0 end if [var y (a add 4) (y gt 9)] [var y 0] [] end y`,
		`def a 3 end var y 0 end if [var y (a add 4) (y gt 9)] [var y 0] end y`,
		// the condition's assignment read by a fn called from the arm, and after
		`var x 1 def g fn [[][Integer][x]] end if [var x 5 (g eq 5)] [g] [0] end g`,
		`var x 1 end if [var x 5 true] [2] [3] end def g fn [[][Integer][x]] end g`,
		// a `do` inside the condition assigns the var there
		`var x 1 end if [do [var x 5 true]] [x] [3] end x`,
		`var x 1 end if [do [var x 5 6] drop true] [2] [3] end x`,
		// inside a fn body (a frame-local var; a module var is not a fn body's to assign)
		`def f fn [[x:Integer][Integer][var y 0 if [var y (x add 1) (y gt 3)] [y] [0 sub y]]] end f 1 f 5`,
		`def f fn [[x:Integer][Integer][var y 0 if [var y (x add 1) (y gt 3)] [y] [0] drop y]] end f 1 f 5`,
		`def f fn [[x:Integer][Integer][var y 0 if [var y (x add 1) (y gt 3)] [var y 0] [] end y]] end f 1 f 5`,
		`def x 1 end def f fn [[][Integer][var z 1 if [var z 9 true] [z] [0]]] end f x`,
		// inside a loop body
		`var y 0 end for 3 [if [var y (i mul 2) (y gt 1)] [y] [0]]`,
		`var y 0 end for 3 [if [var y (i mul 2) (y gt 1)] [y] [0]] end y`,
		`var y 0 end for 3 [if [var y (i mul 2) (y gt 1)] [y] [0] drop y]`,
		`var t 0 end for 3 [if [var t (t add 1) (t gt 1)] [t] [0]] end t`,
		`var t 0 end for 3 [if [var t (t add 1) true] [1] [2]] end t`,
		`var t 0 end for 3 [if [[var t (t add 1) (t gt 2)] [t] [0]]] end t`,
		`var t 0 end for 3 [if [var t (t add 1) true] [break] [2]] end t`,
		`var t 0 end for 3 [if [var t (t add 1) (t gt 1)] [continue] [t]] end t`,
		// nested: an assignment inside an arm reaches the same cell
		`var x 1 end if [var x 5 true] [if [var x (x add 1) true] [x] [0]] [3] end x`,
		`var x 1 end if [var x 5 false] [2] [if [var x (x add 1) (x gt 5)] [x] [0]] end x`,
		`var x 1 end if [var x 5 true] [2] [3] end if [var x (x add 1) true] [x] [0] end x`,
		`def c true end var x 0 end if c [var x 1 if [var x 2 true] [5] [6]] [7] end x`,
		`def c true end var x 0 end if c [var x 1 if [var x (x add 5) true] [5] [6]] [7] end x`,
		`def c false end var x 0 end if c [var x 1 if [var x 2 true] [5] [6]] [7] end x`,
		`def c true end var x 0 end if c [var x 1 if [var x 2 true] [5] [6]] [var x 3 7] end x`,
		`def c true end var x 0 end if c [if [var x 2 true] [5] [6]] [7] end x`,
		`def c false end var x 0 end if c [if [var x 2 true] [5] [6]] [7] end x`,
		`def c true end var x 0 end if c [if [var x 2 true] [var x 9 5] [6]] [7] end x`,
		`def c true end var x 0 end if c [if [var x 2 false] [var x 9 5] [6]] [7] end x`,
		`def c true end var x 0 end if c [if [var x 2 true] [if [var x 4 true] [1] [2]] [6]] [7] end x`,
		`def f fn [[c:Boolean][Integer][var x 0 if c [var x 1 if [var x 2 true] [5] [6]] [7] drop x]] end f true f false`,
		`def f fn [[c:Boolean][Integer][var x 0 if c [if [var x (x add 2) true] [5] [6]] [7] drop x]] end f true f false`,
	} {
		requireEngineParity(t, src, true)
		requireTwinsInPassOrder(t, src)
	}
}

// requireTwinsInPassOrder asserts a compiled program's BIND_TWIN ops replay
// the bind ledger in its own order within every code unit.
func requireTwinsInPassOrder(t *testing.T, src string) {
	t.Helper()
	prog, _, _, err := mustNew(t).CompileCheck(src)
	if prog == nil || err != nil {
		return
	}
	units := [][]compiler.Instr{prog.Code}
	for i := range prog.Fns {
		units = append(units, prog.Fns[i].Code)
	}
	for _, code := range units {
		last := -1
		for _, in := range code {
			if in.Op != compiler.OpBindTwin {
				continue
			}
			if int(in.Arg) <= last {
				t.Errorf("%q: BIND_TWIN %d replays after %d — out of the pass's order", src, in.Arg, last)
			}
			last = int(in.Arg)
		}
	}
}
