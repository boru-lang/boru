package lang

import (
	"fmt"
	"strings"
	"testing"
)

// branch_carried_split_test.go — more of the BRANCH-CARRIED def
// (compiler/go/branch_carried.go) end to end, beside TestBranchCarriedDefParity:
// the constant-condition join, a store whose source was computed outside its
// arm, a top-level SPLIT-bound name rebound in an arm, and the decline a
// store meets when a second def already consumed its source.

// bcsLanes runs src on a fresh interpreter and a fresh compiled instance and
// asserts the two agree, value for value and error code for error code. It
// hands both instances back for a follow-up request.
func bcsLanes(t *testing.T, src string) (interp, compiled *Boru, ok bool) {
	t.Helper()
	interp, compiled = mustNew(t), mustNew(t)
	gotI, errI := interp.RunInterp(src)
	gotC, ran, errC := compiled.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		t.Errorf("%s: did not compile: %v", src, errC)
		return interp, compiled, false
	}
	if !ran {
		t.Fatalf("%s: did not run compiled (%v)", src, errC)
	}
	if codeOf(errI) != codeOf(errC) || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
		t.Errorf("%s:\n  interp   %v err=[%s]\n  compiled %v err=[%s]", src, gotI, codeOf(errI), gotC, codeOf(errC))
		return interp, compiled, false
	}
	return interp, compiled, true
}

// bcsInstalls enumerates name's install stack top-down through run — the
// multi-run parity oracle's read/undef alternation (read the name, record
// the top, undef one level, until the read raises undefined_word).
func bcsInstalls(t *testing.T, run func(string) ([]any, error), name string) []string {
	t.Helper()
	var seq []string
	for i := 0; i < 16; i++ {
		out, err := run(name)
		if err != nil {
			if strings.Contains(err.Error(), "undefined_word") {
				return seq
			}
			t.Fatalf("probe read of %q: %v", name, err)
		}
		seq = append(seq, fmt.Sprint(out))
		if _, err := run("undef " + name); err != nil {
			t.Fatalf("probe undef of %q: %v", name, err)
		}
	}
	t.Fatalf("probe of %q did not drain (so far %v)", name, seq)
	return nil
}

// TestBranchCarriedConstConditionParity: on a constant condition only the
// taken arm is analysed, and it always runs — a pre binding is seated with
// no seed, and with no pre binding the arm's own value stands.
func TestBranchCarriedConstConditionParity(t *testing.T) {
	for _, src := range []string{
		`def x 1 end if [true] [def x 9 end 5] [0] end x`,
		`def x 1 end if [false] [0] [def x 9 end 5] end x`,
		`def f fn [[] [Integer] [def x 1 end if [true] [def x 9 end 5] [0] end drop x]]  f`,
		`if [true] [def y 9 end 5] [0] end y`,
	} {
		bcsLanes(t, src)
	}
}

// TestBranchCarriedOuterSourceParity: an arm def whose value was computed
// OUTSIDE the arm (`[def r q]` over an enclosing `def q (half n)`) stores a
// cross-floor reference, which the planner promotes to a frame local so the
// arm can re-push it.
func TestBranchCarriedOuterSourceParity(t *testing.T) {
	const f = `def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] def q (half n) def r 0 end if (n gt 1) [def r q] [] end r]]  `
	for _, src := range []string{
		f + `f 8`,
		f + `f 1`,
		`def g fn [[n:Integer] [Integer] [n add 1]]  def q (g 1)  def r 0 end if (q gt 1) [def r q] [] end r`,
	} {
		bcsLanes(t, src)
	}
}

// TestBranchCarriedSplitBindParity pins a top-level SPLIT-bound name — a def
// of a loop's first value (`def x (for [1 4] [i])`, the rest spilling) or of
// a two-value region's first (`def x (5 dup)`) — rebound in an arm. Its
// binding has no compiled home, and a root read of it resolves live through
// the registry, so the join is seated with the registry read as its seed and
// the arm's store also installs the binding. Before, the join was left
// unseated and the read after the merge looked up a registry the arm's def
// never reached whenever that read was only the program's residual: the
// first row answered `2 3 1` for the interpreter's `2 3 9`, silently. Every
// row also compares the name's whole install stack in a following request.
func TestBranchCarriedSplitBindParity(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	for _, src := range []string{
		g + `def x (for [1 4] [i])  if (g 9) [def x 9] [] end x`,
		g + `def x (for [1 4] [i])  if (g 1) [def x 9] [] end x`,
		g + `def x (for [1 4] [i])  if (g 9) [def x 9] [def x 7] end x`,
		g + `def x (for [1 4] [i])  if (g 1) [def x 9] [def x 7] end x`,
		g + `def x (for [1 4] [i])  if (g 1) [] [def x 7] end x`,
		g + `def x (for [1 4] [i])  if (g 9) [def x 9] [] end x add 1`,
		g + `def x (for [1 4] [i])  if (g 9) [def x 9] [] end 0`,
		g + `def x (for [1 4] [i])  if (g 9) [def x 9] [] end if (g 9) [def x 10] [] end x`,
		g + `def x (for [1 4] [i])  if (g 9) [if (g 1) [def x 8] [def x 9]] [] end x`,
		g + `def x (for [1 4] [i])  if (g 9) [def x (size [1 2 3])] [] end x`,
		g + `def x (for [1 4] [i])  if (g 9) [def x (g 3)] [] end x`,
		g + `def x (for [1 4] [i])  def h fn [[] [Any] [x]]  if (g 9) [def x 9] [] end h`,
		g + `def x (for [1 4] [i])  if [true] [def x 9 end 5] [0] end x`,
		`def x (for [1 4] [i])  if true [def x 9] [] end x`,
		`for 2 [def x (for [1 3] [i]) if (i eq 0) [def x 9] [] end x]`,
		// Read BEFORE the branch: the pre binding resolves to the registry
		// read and the seed is refused; the read committed the name, so the
		// arm's def installs it and the join stays unseated.
		g + `def x (for [1 4] [i])  x drop  if (g 9) [def x 9] [] end x`,
		g + `def x (for [1 4] [i])  x drop  if (g 1) [def x 9] [] end x`,
		// The two-value region's split, including an operand read after the
		// merge (which bailed at run time: the read committed the name, and
		// the committed split's splice found its spare value gone).
		g + `def x (5 dup)  if (g 9) [def x 9] [] end x`,
		g + `def x (5 dup)  if (g 1) [def x 9] [] end x`,
		g + `def x (5 dup)  if (g 9) [def x 9] [] end x add 1`,
		g + `def x (5 dup drop)  if (g 9) [def x 9] [] end x`,
	} {
		interp, compiled, ok := bcsLanes(t, src)
		if !ok {
			continue
		}
		wantStack := bcsInstalls(t, interp.RunInterp, "x")
		gotStack := bcsInstalls(t, compiled.RunCompiledStrict, "x")
		if fmt.Sprint(gotStack) != fmt.Sprint(wantStack) {
			t.Errorf("%s: next request's install stack of x: compiled %v, interp %v", src, gotStack, wantStack)
		}
	}
}

// TestArgsProjectionReadAfterDef: `args.N` folds to the param's own home only
// while the args projection's MAKE_LIST is the frame's last event; once
// something follows it (the def that names the list) the fold stands aside
// and the read runs over the assembled list.
func TestArgsProjectionReadAfterDef(t *testing.T) {
	for _, src := range []string{
		`def f fn [[a:Integer b:Integer] [Integer] [def xs (args) xs.1]]  f 3 4`,
		`def f fn [[a:Integer b:Integer] [Integer] [def xs (args) (a add b) drop xs.0]]  f 3 4`,
	} {
		bcsLanes(t, src)
	}
}

// TestUserPolyDeadDefConsumedByWriteBack: a root def of a runtime-dispatched
// user call's result that nothing reads leaves the result for its write-back
// to consume — no DROP — and the next request reads the arm the run picked.
func TestUserPolyDeadDefConsumedByWriteBack(t *testing.T) {
	const pf = `def pf fn [[a:Integer] [Integer] [1] [a:String] [Integer] [2]]  `
	for _, c := range []struct{ src, want string }{
		{pf + `def g fn [[] [Any] [5]]  def q (pf (g))  99`, "[1]"},
		{pf + `def g fn [[] [Any] ['s']]  def q (pf (g))  99`, "[2]"},
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(c.src)
		if prog == nil {
			t.Fatalf("%s: declined: %s %v", c.src, reason, err)
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, "CALL_USER_POLY") || strings.Contains(dis, "DROP") {
			t.Errorf("%s: the dead result must ride from the poly call to its write-back:\n%s", c.src, dis)
		}
		interp, compiled, ok := bcsLanes(t, c.src)
		if !ok {
			continue
		}
		gotI, errI := interp.RunInterp(`q`)
		gotC, errC := compiled.RunCompiledStrict(`q`)
		if errI != nil || errC != nil || fmt.Sprint(gotI) != c.want || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: next request q = interp %v (%v), compiled %v (%v), want %s", c.src, gotI, errI, gotC, errC, c.want)
		}
	}
}

// TestBranchCarriedSharedSourceDeclines pins a decline, not a pass: two
// carried defs in one arm over ONE computed value (`def y (3 add 4) def x
// y`, y and x both read past the merge) — the first store consumes the
// refcount-dead source, and the second finds nothing on top. The program
// declines loudly under the store's own reason, where the interpreter
// answers 7: a compile defect, never a wrong answer.
func TestBranchCarriedSharedSourceDeclines(t *testing.T) {
	for _, c := range []struct{ src, reason string }{
		{`def g fn [[n:Integer] [Boolean] [n gt 5]]  def x 1  if (g 9) [def y (3 add 4) def x y] [] end x`,
			"branch-carried def `x` source is not on top of the stack"},
		{`def f fn [[b:Boolean] [Integer] [def x 1 end if b [def y (3 add 4) def x y] [] end x]]  f true`,
			"fn f: branch-carried def `x` source is not on top of the stack"},
	} {
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != "[7]" {
			t.Fatalf("%s: the interpreter answers %v (%v), want [7]", c.src, got, err)
		}
		prog, reason, _, err := mustNew(t).CompileCheck(c.src)
		if prog != nil || err != nil || reason != c.reason {
			t.Errorf("%s: want the decline %q, got program=%v reason=%q err=%v", c.src, c.reason, prog != nil, reason, err)
		}
	}
}
