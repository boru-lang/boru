package lang

import (
	"fmt"
	"testing"
)

// TestBranchCarriedDefParity pins the BRANCH-CARRIED def
// (compiler/go/branch_carried.go): a name bound inside an `if` arm and read
// after the merge loads whichever arm ran. Each shape runs on both lanes and
// must agree — value for value, error code for error code. The false-path
// twins are deliberate: a lowering that always took the then-arm's value,
// or that lost the incoming binding through an empty arm, retires the
// then-path rows and still miscompiles these.
func TestBranchCarriedDefParity(t *testing.T) {
	for _, src := range []string{
		// both arms bind, then and else
		`def f fn [[n:Integer] [String] [if (n gt 0) [def tag 'big'] [def tag 'small'] end tag]]  f 5`,
		`def f fn [[n:Integer] [String] [if (n gt 0) [def tag 'big'] [def tag 'small'] end tag]]  f 0`,
		// a pre binding rebound in one arm; the EMPTY arm carries it through
		`def f fn [[] [Integer] [def x 1 end if true [def x 9] [] end x]]  f`,
		`def f fn [[] [Integer] [def x 1 end if false [def x 9] [] end x]]  f`,
		`def x 1 end if false [def x 9] [] end x`,
		`def x 1 end if true [def x 9] [] end x`,
		// the read feeding an operator
		`def f fn [[n:Integer] [Integer] [def r 0 end if (n gt 0) [def r 1] [def r 2] end r add 10]]  f 0`,
		// nested branches, every path
		`def f fn [[b:Boolean] [Integer] [def r 0 end if b [if true [def r 1] [def r 2]] [def r 3] end r]]  f true`,
		`def f fn [[b:Boolean] [Integer] [def r 0 end if b [if true [def r 1] [def r 2]] [def r 3] end r]]  f false`,
		`def f fn [[b:Boolean] [Integer] [def r 0 end if b [if false [def r 1] [def r 2]] [def r 3] end r]]  f true`,
		// a computed pre binding and a computed rebind (the seed is a promoted local)
		`def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] def q (half n) end if (q gt 1) [def q (half q)] [] end q]]  f 8`,
		`def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] def q (half n) end if (q gt 1) [def q (half q)] [] end q]]  f 2`,
		// no pre binding, one arm: bound on the path that ran it (NUR110's taken side)
		`def f fn [[b:Boolean] [Integer] [if b [def z 9] [] end z]]  f true`,
		`def c true  if c [def op 1] [0]  end  op`,
		// no pre binding, one arm, the arm did NOT run: undefined_word on both lanes (NUR110)
		`def f fn [[b:Boolean] [Integer] [if b [def z 9] [] end z]]  f false`,
		`def f fn [[b:Boolean] [Integer] [if b [] [def z 9] end z]]  f true`,
		`if false [def op 1] [0] end op`,
		`if false [def z (1 add 8)] [] end z`,
		// a slot means "bound since this frame started": an arm's binding
		// from one loop iteration is still read in the next (measured on
		// the interpreter before the mechanism was built)
		`for 2 [if (i eq 0) [def z 9] [] end z]`,
		`def z 0 end for 2 [if (i eq 0) [def z 9] [] end z]`,
		// a loop-carried and a branch-carried name share one cell
		`def acc 0 end for 3 [if (i eq 1) [def acc (acc add 10)] [def acc (acc add 1)]] end acc`,
		// the branch result and the carried name are different things
		`def f fn [[c:Boolean] [Integer] [def out (if c [def t2 5 end t2 add 1] [0]) end out]]  f true`,
		// a computed condition the checker cannot fold, at the top level
		`def g fn [[n:Integer] [Boolean] [n gt 5]]  if (g 1) [def op 1] [0] end op`,
		`def g fn [[n:Integer] [Boolean] [n gt 5]]  if (g 9) [def op 1] [0] end op`,
	} {
		gotI, errI := mustNew(t).RunInterp(src)
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			t.Errorf("%s: did not compile: %v", src, errC)
			continue
		}
		if !compiled {
			t.Fatalf("%s: did not run compiled (%v)", src, errC)
		}
		if codeOf(errI) != codeOf(errC) || fmt.Sprint(gotI) != fmt.Sprint(gotC) {
			t.Errorf("%s:\n  interp   %v err=[%s]\n  compiled %v err=[%s]", src, gotI, codeOf(errI), gotC, codeOf(errC))
		}
	}
}

// TestBranchCarriedDefUnreadArmBindLowersAsBefore pins the storability rule:
// a name an arm binds to a value the slot cannot hold (a `word` splice
// value) is simply not carried, so a program that never reads it past the
// merge compiles exactly as it did — the generated sweep's eight `word`
// call-form variants (design/SESSION-HANDOVER.0.md, 2026-09-22).
func TestBranchCarriedDefUnreadArmBindLowersAsBefore(t *testing.T) {
	src := `if true [def dbl word [2 mul] 3 dbl] [0]`
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		t.Errorf("%s: did not compile: %v", src, errC)
		return
	}
	if errC != nil || fmt.Sprint(gotC) != "[6]" {
		t.Errorf("%s: compiled err=%v got=%v, want [6]", src, errC, gotC)
	}
}
