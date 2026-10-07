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
		`var x 1 end if [true] [var x 9 end 5] [0] end x`,
		`var x 1 end if [false] [0] [var x 9 end 5] end x`,
		`def f fn [[] [Integer] [var x 1 end if [true] [var x 9 end 5] [0] end drop x]]  f`,
		`var y 0 end if [true] [var y 9 end 5] [0] end y`,
	} {
		bcsLanes(t, src)
	}
}

// TestBranchCarriedOuterSourceParity: an arm def whose value was computed
// OUTSIDE the arm (`[def r q]` over an enclosing `def q (half n)`) stores a
// cross-floor reference, which the planner promotes to a frame local so the
// arm can re-push it.
func TestBranchCarriedOuterSourceParity(t *testing.T) {
	const f = `def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] def q (half n) var r 0 end if (n gt 1) [var r q] [] end r]]  `
	for _, src := range []string{
		f + `f 8`,
		f + `f 1`,
		`def g fn [[n:Integer] [Integer] [n add 1]]  def q (g 1)  var r 0 end if (q gt 1) [var r q] [] end r`,
	} {
		bcsLanes(t, src)
	}
}

// TestBranchCarriedSplitBindParity pins a top-level SPLIT-bound name — a var
// declared from a loop's first value (`var x (for [1 4] [i])`, the rest
// spilling) or from a two-value region's first (`var x (5 dup)`) — assigned
// in an arm. Its binding has no compiled home, and a root read of it
// resolves live through the registry, so the join is seated with the
// registry read as its seed and the arm's store also installs the binding.
// Before, the join was left unseated and the read after the merge looked up
// a registry the arm's def never reached whenever that read was only the
// program's residual: the first row answered `2 3 1` for the interpreter's
// `2 3 9`, silently. (Since phase 2 the cell is a var; a `def` in the arm is
// the arm's block local.)
func TestBranchCarriedSplitBindParity(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	for _, src := range []string{
		g + `var x (for [1 4] [i])  if (g 9) [var x 9] [] end x`,
		g + `var x (for [1 4] [i])  if (g 1) [var x 9] [] end x`,
		g + `var x (for [1 4] [i])  if (g 9) [var x 9] [var x 7] end x`,
		g + `var x (for [1 4] [i])  if (g 1) [var x 9] [var x 7] end x`,
		g + `var x (for [1 4] [i])  if (g 1) [] [var x 7] end x`,
		g + `var x (for [1 4] [i])  if (g 9) [var x 9] [] end x add 1`,
		g + `var x (for [1 4] [i])  if (g 9) [var x 9] [] end 0`,
		g + `var x (for [1 4] [i])  if (g 9) [var x 9] [] end if (g 9) [var x 10] [] end x`,
		g + `var x (for [1 4] [i])  if (g 9) [if (g 1) [var x 8] [var x 9]] [] end x`,
		g + `var x (for [1 4] [i])  if (g 9) [var x (size [1 2 3])] [] end x`,
		g + `var x (for [1 4] [i])  if (g 9) [var x (g 3)] [] end x`,
		g + `var x (for [1 4] [i])  def h fn [[] [Any] [x]]  if (g 9) [var x 9] [] end h`,
		g + `var x (for [1 4] [i])  if [true] [var x 9 end 5] [0] end x`,
		`var x (for [1 4] [i])  if true [var x 9] [] end x`,
		// Read BEFORE the branch: the pre binding resolves to the registry
		// read and the seed is refused; the read committed the name, so the
		// arm's def installs it and the join stays unseated.
		g + `var x (for [1 4] [i])  x drop  if (g 9) [var x 9] [] end x`,
		g + `var x (for [1 4] [i])  x drop  if (g 1) [var x 9] [] end x`,
		// The two-value region's split, including an operand read after the
		// merge (which bailed at run time: the read committed the name, and
		// the committed split's splice found its spare value gone).
		g + `var x (5 dup)  if (g 9) [var x 9] [] end x`,
		g + `var x (5 dup)  if (g 1) [var x 9] [] end x`,
		g + `var x (5 dup)  if (g 9) [var x 9] [] end x add 1`,
		g + `var x (5 dup drop)  if (g 9) [var x 9] [] end x`,
	} {
		interp, compiled, ok := bcsLanes(t, src)
		if !ok {
			continue
		}
		// The install STACK of the var after the program, measured on the
		// next request: one cell on both lanes. A var declaration's twin
		// replayed a plain def until 2026-10-07 (core InstallVar marked the
		// entry after the ledger captured it), so the arm's assignment
		// declared a second cell beside it — one level deeper, which only
		// `undef x` exposed.
		wantStack := bcsInstalls(t, interp.RunInterp, "x")
		gotStack := bcsInstalls(t, compiled.RunCompiledStrict, "x")
		if fmt.Sprint(gotStack) != fmt.Sprint(wantStack) {
			t.Errorf("%s: next request's install stack of x: compiled %v, interp %v", src, gotStack, wantStack)
		}
	}
	// The declaration INSIDE an inlined loop body is the body's own since
	// phase 2 — retired with the iteration on the interpreter, so the next
	// request finds nothing — while the compiled loop body has no block of
	// its own yet (the compiler's block scopes land with phase 2's second
	// step) and its write-backs stay installed: value parity only, the
	// stack measured with that landing (design/IMMUTABLE-DEF.1.md §5).
	bcsLanes(t, `for 2 [var x (for [1 3] [i]) if (i eq 0) [var x 9] [] end x]`)
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

// TestBranchCarriedSharedSourceAgrees: an arm's own def feeding a var
// assignment (`def y (3 add 4) var x y`, x read past the merge) — the arm's
// `y` is its block local, consumed inside the arm; the assignment carries
// the value out. Before phase 2 both were branch-carried defs over ONE
// computed value and the second store found nothing on top (a decline).
func TestBranchCarriedSharedSourceAgrees(t *testing.T) {
	for _, src := range []string{
		`def g fn [[n:Integer] [Boolean] [n gt 5]]  var x 1  if (g 9) [def y (3 add 4) var x y] [] end x`,
		`def f fn [[b:Boolean] [Integer] [var x 1 end if b [def y (3 add 4) var x y] [] end x]]  f true`,
	} {
		if got, err := mustNew(t).RunInterp(src); err != nil || fmt.Sprint(got) != "[7]" {
			t.Fatalf("%s: the interpreter answers %v (%v), want [7]", src, got, err)
		}
		// The assignment's source is the arm's own def: the compiled lane
		// agrees where it compiles and declines where its store finds the
		// source consumed (the branch-carried store's own fence).
		requireEngineParity(t, src, false)
	}
}
