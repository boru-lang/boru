package lang

import (
	"strings"
	"testing"
)

// cover_pr505_main_test.go pins, on real programs, recording and lowering
// arms from main that no suite reached: the args projection a folding get
// cannot retract, the dyn-body backstop's zero-result and hook-hazard
// declines, the fallback island's refusal of a lens body, and three arms of
// the branch-carried def (a source not on the stack top, a source from
// outside the arm, a seed that is no re-pushable value). Declines are
// asserted with soundDecline (cover_pr505_lower_test.go): CompileCheck's
// reason and the interpreter's own answer.

// TestArgsProjectionAFoldCannotRetract: `args.N` folds to the param's
// local and retracts the projection's MAKE_LIST only while that list is the
// frame's last event. With another event between the projection and the
// get, the list stays and the get reads it: same answer, one more op.
func TestArgsProjectionAFoldCannotRetract(t *testing.T) {
	const pre = `def f fn [[a:Integer b:Integer] [Any] [`
	for _, c := range []struct {
		body     string
		makeList bool
	}{
		{`args get 1`, false},
		{`args 9 drop get 1`, true},
		{`args def k 1 get k`, true},
	} {
		src := pre + c.body + `]] end f 5 6`
		if got := strings.Contains(compileDisasm(t, src), "MAKE_LIST"); got != c.makeList {
			t.Errorf("%q: the projection's MAKE_LIST present = %v, want %v", src, got, c.makeList)
		}
		agreeOnBothLanes(t, src, "[6]")
	}
}

// TestDynBodyBackstopDeclines pins the dyn-body backstop's refusals
// (tryRecordDynBody), each where the closure path has already declined:
//
//   - an `error` handler that nets NOTHING under a proven raise records zero
//     results, and zero is `error`'s own count only for a word that declares
//     a 0-out body (it does not). The handler reads `args`, which a closure
//     unit cannot project, so the closure path declined first; the
//     args-free handler compiles as a closure;
//   - `walk`'s ascend hook is held to the body's rules: a type def (a
//     registry mutation the replayed hook would re-run) or a break (a
//     sentinel no handler boundary crosses) in it declines, where the
//     descend hook is a factory's fn value the closure path cannot compile;
//     a hazard-free ascend hook compiles.
func TestDynBodyBackstopDeclines(t *testing.T) {
	soundDecline(t, `def f fn [[n:Integer] [Any] [do [raise boom "x"] error [args drop drop] n]] end f 5`,
		"code-body word error (Stage 2)", "[5]")
	agreeOnBothLanes(t, `def f fn [[n:Integer] [Any] [do [raise boom "x"] error [drop] n]] end f 5`, "[5]")

	const mk = `def mk fn [[] [Function] [(m:Any => [m.depth drop])]] end `
	soundDecline(t, mk+`walk {mode: "depth"} {a:1} (mk) [def T Integer]`, "code-body word walk (Stage 2)", "[{a:1}]")
	soundDecline(t, mk+`for 2 [walk {mode: "depth"} {a:1} (mk) [break]]`, "code-body word walk (Stage 2)", "[]")
	agreeOnBothLanes(t, mk+`walk {mode: "depth"} {a:1} (mk) [drop]`, "[{a:1}]")
}

// TestLensBodyIsNoFallbackIsland: a higher-order word dispatched on a LENS
// with a computed segment (`$.("n")`, no inert const) reaches the fallback
// island with no code body to run, and declines there rather than becoming
// a new interpreter island. The inert lens compiles.
func TestLensBodyIsNoFallbackIsland(t *testing.T) {
	soundDecline(t, `each $.("n") [{n:1} {n:2}]`,
		"operand of unknown provenance or not statically materialisable at each", "[[1 2]]")
	soundDecline(t, `filter $.("on") [{on:true} {on:false}]`,
		"operand of unknown provenance or not statically materialisable at filter", "[[{on:true}]]")
	agreeOnBothLanes(t, `each $.n [{n:1} {n:2}]`, "[[1 2]]")
}

// TestBranchCarriedDefSources pins three arms of the branch-carried def
// (branch_carried.go), each beside a twin that compiles:
//
//   - an arm def of a multi-result call binds its first value, which is
//     not the stack top: the store declines (storeArmBind); a single-result
//     call stores;
//   - an arm def of a value computed OUTSIDE the arm (`def x v`) is a
//     cross-floor reference the planner promotes: both lanes answer the
//     taken arm's binding, and undefined_word where the arm did not run;
//   - a pre binding a user fn's multi-result call produced is a seed the
//     planner never seats in a frame local (a user fn's multi-return stays
//     Stage 3), so it still reads as an event at the branch and declines; a
//     single-result producer's seed re-pushes from its local.
func TestBranchCarriedDefSources(t *testing.T) {
	const one = `def one fn [[][Integer][1]] end `
	const two = `def two fn [[][Integer Integer][1 2]] end `
	soundDecline(t, two+`def c true end if c [def x (two)] [] end x`,
		"branch-carried def `x` source is not on top of the stack", "[2 1]")
	agreeOnBothLanes(t, one+`def c true end if c [def x (one)] [] end x`, "[1]")

	agreeOnBothLanes(t, one+`def c true end def v (one) end if c [def x v] [] end x`, "[1]")
	agreeOnBothLanes(t, `def c true end def v (1 add 2) end if c [def x v] [] end x`, "[3]")
	agreeOnBothLanes(t, one+`def c false end def v (one) end if c [def x v] [] end x`, "ERROR:undefined word: x")

	const seed = `def f fn [[c:Boolean] [Any] [def x (two nip) if c [def x 7] [] x]] end `
	soundDecline(t, two+seed+`f true`, "if: carried def seed is not a re-pushable value (Stage 2)", "[7]")
	soundDecline(t, two+seed+`f false`, "if: carried def seed is not a re-pushable value (Stage 2)", "[2]")
	const seed1 = `def f fn [[c:Boolean] [Any] [def x (one) if c [def x 7] [] x]] end `
	agreeOnBothLanes(t, one+seed1+`f true`, "[7]")
	agreeOnBothLanes(t, one+seed1+`f false`, "[1]")
}
