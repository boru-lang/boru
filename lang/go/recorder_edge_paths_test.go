package lang

import (
	"fmt"
	"strings"
	"testing"
)

// recorder_edge_paths_test.go pins recorder and lowering paths of
// compiler/go/emit.go that whole programs reach and no other test drove:
// each shape either compiles and answers as the interpreter does, or
// declines loudly beside the interpreter's own answer (requireLoudDecline /
// requireLoudDeclineErr — CompileCheck and the compiled lane's plain
// compile_failed, never the defect ledger).

// TestStampOnlyUnitLoweringDeclineStubs — a fn VALUE the const chokepoint
// stamps (stampFnConst) compiles its body speculatively as a stamp-only unit.
// When that unit's lowering declines — "consumes loop results", the reason
// the same fn declines the whole program with when it is CALLED by name — the
// unit is replaced by a trap stub, the value's compiled ref is dropped
// (dropStampRef), and the program compiles: nothing calls the stub, and
// applying the value takes the interpreter's frame, so every lane agrees.
func TestStampOnlyUnitLoweringDeclineStubs(t *testing.T) {
	const f = `def f fn [[n:Integer][Integer][if (n gt 0) [1 2] [3] add 1]]  `
	requireLoudDecline(t, f+`f 0`, "fn f: consumes loop results", "[4]")
	for _, src := range []string{
		f + `def m {g: f/v}  m size`,
		f + `def m {g: f/v}  m.g 0`,
		f + `def m {g: f/v}  m.g 1`,
		f + `def m {g: f/v}  (m.g 0) add 10`,
		f + `def m {g: f/v}  0 m.g/v apply`,
		`def m {g: ([n:Integer] => [if (n gt 0) [1 2] [3] add 1])}  m.g 0`,
		`def m {g: ([] => [(for 3 [i]) add 1])}  m size`,
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(src)
		if err != nil || prog == nil {
			t.Errorf("%q: the stub keeps the program compilable, got reason=%q err=%v", src, reason, err)
			continue
		}
		stubbed := false
		for _, tr := range prog.Traps {
			if strings.Contains(tr.Detail, "stamp-only fn unit") && strings.Contains(tr.Detail, "entered after its lowering declined") {
				stubbed = true
			}
		}
		if !stubbed {
			t.Errorf("%q: the declining stamp-only unit must be replaced by its trap stub:\n%s", src, prog.Disassemble())
		}
		requireEngineParity(t, src, true)
	}
}

// TestBranchArmDefThenUndef — an arm's def is the arm's own block local
// (design/IMMUTABLE-DEF.1.md §2.1, phase 2), so the `undef` after the
// branch pops the frame's binding whichever arm ran, and the read after it
// is the undefined word it is on both lanes: the check pass stops there,
// the interpreter raises. (Before phase 2 the arm's def was carried in the
// unit's cell and the undef declined as the loop-carried undef hazard.)
func TestBranchArmDefThenUndef(t *testing.T) {
	const f = `def f fn [[b:Boolean] [Integer] [def z 1 end if b [def z 9] [] end undef z end z]]  `
	requireBlockRule(t, f+`f true`, "ERROR:undefined word: z", "check diagnostics")
	requireBlockRule(t, f+`f false`, "ERROR:undefined word: z", "check diagnostics")
}

// TestTrailingParenApplyOverFnArgDeclines — a paren-bounded trailing apply
// whose window holds a fn VALUE argument (`(lambda comp)`): the value was a
// token the interpreter stepped inside the paren, so the apply is not
// recorded as an event, and the body-tail lowering refuses the same window —
// the fn declines on the unapplied fn value rather than applying comp over a
// window the interpreter did not build. The interpreter answers 6 (comp
// applies the lambda to 5).
func TestTrailingParenApplyOverFnArgDeclines(t *testing.T) {
	requireLoudDecline(t,
		`def f fn [[comp:Function][Any][(([x:Integer] => [x add 1]) comp)]]  def h fn [[g:Function][Integer][5 g/v apply]]  f h/v`,
		"unapplied fn-value in body residual", "[6]")
}

// TestParenApplyMultiReturnCalleeParity — a paren-bounded trailing apply of
// a CONCRETE callee that is not provably single-valued (`pair/v`, two
// declared returns) is not recorded as the one-result apply event; the
// program compiles and nets BOTH values exactly as the interpreter does. The
// consumers are the witnesses: recorded as a one-result event, the list
// literal assembled `[2 [1]]`, and `swap` / a fn body's `add` met a stack
// the model had counted one value short (a signature_error for the
// interpreter's answer).
func TestParenApplyMultiReturnCalleeParity(t *testing.T) {
	const pair = `def pair fn [[a:Integer b:Integer] [Integer Integer] [a b]]  `
	for _, c := range []struct{ src, want string }{
		{pair + `(1 2 pair/v)`, "[2 1]"},
		{pair + `(1 2 pair/v) add 5`, "[2 6]"},
		{pair + `[(1 2 pair/v)]`, "[[2 1]]"},
		{pair + `(1 2 pair/v) swap`, "[1 2]"},
		{pair + `def f fn [[][Integer Integer][(1 2 pair/v)]]  f`, "[2 1]"},
		{pair + `def f fn [[][Integer][(1 2 pair/v) add]]  f`, "[3]"},
		{pair + `def f fn [[n:Integer][Integer][(n 2 pair/v) add]]  f 5`, "[7]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v %v/%v, interp %v/%v, want %s on both lanes", c.src, compiled, gotC, errC, gotI, errI, c.want)
		}
	}
}

// TestForRangeLiveStartDeclines — a counted `for` whose START is a live
// dynamic-scope read (a computed module def read inside a fn: not a const,
// a frame local or a promotable event) has no stack home the loop setup can
// re-push, so the program declines, beside the interpreter's 1+2+3.
func TestForRangeLiveStartDeclines(t *testing.T) {
	requireLoudDecline(t,
		`def s (0 add 1)  def f fn [[][Integer][var t 0  for [s 4] [var t (t add i)]  t]]  f`,
		"for: computed range start/step", "[6]")
}

// TestSplicePayloadReReadDeclines — a dynamic splice reassigns its payload's
// provenance to the spread, so a re-read of the payload def after the splice
// surfaces the SAME value twice in the program residual; the re-read would
// resolve to the spread, so the program declines beside the interpreter's
// answer.
func TestSplicePayloadReReadDeclines(t *testing.T) {
	requireLoudDecline(t, `def xs (range 1 3) def d word xs d xs`,
		"splice payload re-read after the spread", "[1 2 [1 2]]")
}

// TestModifierWrapperShapeClaimBounds — a def-bound modifier wrapper's shape
// claim (modifierWrappedFnShape) walks the modifier chain to the fn it
// wraps. A chain deeper than the walk's bound makes no claim and the read
// keeps its dynamic apply; a wrapper over a Function PARAM (a frame local,
// not an event) and over a container member (an event with no fn shape)
// make none either. Every row answers as the interpreter does — ten usurps
// restore the operand order, nine reverse it.
func TestModifierWrapperShapeClaimBounds(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]]  `
	// nest(w, n) is `w (w (… w (mk 100)))`, n modifiers deep.
	nest := func(word string, n int) string {
		s := word + " (mk 100)"
		for i := 1; i < n; i++ {
			s = word + " (" + s + ")"
		}
		return s
	}
	for _, c := range []struct{ src, want string }{
		{mk + `def r (` + nest("usurp", 10) + `)  r 10 3`, "[107]"},
		{mk + `def r (` + nest("usurp", 9) + `)  r 10 3`, "[93]"},
		{mk + `def r (` + nest("forward-args", 10) + `)  r 10 3`, "[107]"},
		{`def f fn [[g:Function][Integer][def r (usurp g) r 10 3]]  f ([a:Integer b:Integer] => [a sub b])`, "[-7]"},
		{`def m {s:sub/v}  def r (usurp (m.s))  r 10 3`, "[7]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter %v/%v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestReceiveNestedClauseScreens — at a NESTED position a registry-run
// clause list is admitted only when it names nothing the program or the
// registry knows and is inert-scoped: a paren naming a registered word
// (`add`) and a clause body carrying a flow sentinel (`break`) both decline,
// beside the interpreter's answers.
func TestReceiveNestedClauseScreens(t *testing.T) {
	requireLoudDecline(t, `def f fn [[][Any][receive [{} [(1 add 2)] after 0 [0]]]]  f`, "code-body word receive", "[0]")
	requireLoudDecline(t, `for 1 [receive [{} [(1 add 2)] after 0 [(2 add 3)]]]`, "code-body word receive", "[5]")
	requireLoudDecline(t, `for 1 [receive [{} [1] after 0 [break]]]`, "code-body word receive", "[]")
	requireLoudDeclineErr(t, `def f fn [[][Any][receive [{} [1] after 0 [break]]]]  f`, "code-body word receive", "flow_error")
}
