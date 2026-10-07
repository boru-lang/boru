package lang

import (
	"fmt"
	"testing"
)

// TestBranchCarriedDefParity pins the branch-joined VAR (phase 2 of
// design/IMMUTABLE-DEF.1.md): a var assigned inside an `if` arm and read
// after the merge holds whichever arm ran, and the branch VALUE spelling
// `def tag (if c ['big'] ['small'])` binds what an arm computed — a `def`
// inside an arm is the arm's own block local now (block_scope_rule_test.go
// keeps those shapes). Each shape runs on both lanes and must agree — value
// for value, error code for error code. The false-path twins are deliberate:
// a lowering that always took the then-arm's value, or that lost the
// incoming cell through an empty arm, retires the then-path rows and still
// miscompiles these.
func TestBranchCarriedDefParity(t *testing.T) {
	for _, src := range []string{
		// both arms compute the value: the branch VALUE is bound
		`def f fn [[n:Integer] [String] [def tag (if (n gt 0) ['big'] ['small']) end tag]]  f 5`,
		`def f fn [[n:Integer] [String] [def tag (if (n gt 0) ['big'] ['small']) end tag]]  f 0`,
		// a pre-declared var assigned in one arm; the EMPTY arm leaves it
		`def f fn [[] [Integer] [var x 1 end if true [var x 9] [] end x]]  f`,
		`def f fn [[] [Integer] [var x 1 end if false [var x 9] [] end x]]  f`,
		`var x 1 end if false [var x 9] [] end x`,
		`var x 1 end if true [var x 9] [] end x`,
		// the read feeding an operator
		`def f fn [[n:Integer] [Integer] [var r 0 end if (n gt 0) [var r 1] [var r 2] end r add 10]]  f 0`,
		// nested branches, every path
		`def f fn [[b:Boolean] [Integer] [var r 0 end if b [if true [var r 1] [var r 2]] [var r 3] end r]]  f true`,
		`def f fn [[b:Boolean] [Integer] [var r 0 end if b [if true [var r 1] [var r 2]] [var r 3] end r]]  f false`,
		`def f fn [[b:Boolean] [Integer] [var r 0 end if b [if false [var r 1] [var r 2]] [var r 3] end r]]  f true`,
		// a computed declaration and a computed assignment
		`def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] var q (half n) end if (q gt 1) [var q (half q)] [] end q]]  f 8`,
		`def f fn [[n:Integer] [Integer] [def half fn [[k:Integer] [Integer] [k div 2]] var q (half n) end if (q gt 1) [var q (half q)] [] end q]]  f 2`,
		// a declaration before the branch, assigned on the path that ran the arm
		`def f fn [[b:Boolean] [Integer] [var z 0 end if b [var z 9] [] end z]]  f true`,
		`def f fn [[b:Boolean] [Integer] [var z 0 end if b [var z 9] [] end z]]  f false`,
		`def c true  var op 0  if c [var op 1] [0]  end  op`,
		// an assignment made in one loop iteration is read in the next
		`var z 0 end for 2 [if (i eq 0) [var z 9] [] end z]`,
		// a loop-carried and a branch-assigned name are one cell
		`var acc 0 end for 3 [if (i eq 1) [var acc (acc add 10)] [var acc (acc add 1)]] end acc`,
		// a loop assigning a var an arm may have assigned before it
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z (z add 1)] end z]]  f true 2`,
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z (z add 1)] end z]]  f true 0`,
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z (z add 1)] end z]]  f false 0`,
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z (z add 1)] end z]]  f false 2`,
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z 7] end z]]  f false 0`,
		`def f fn [[b:Boolean n:Integer] [Integer] [var z 0 end if b [var z 9] [] end for n [var z 7] end z]]  f false 3`,
		// the branch result and the var are different things
		`def f fn [[c:Boolean] [Integer] [def out (if c [def t2 5 end t2 add 1] [0]) end out]]  f true`,
		// a computed condition the checker cannot fold, at the top level
		`def g fn [[n:Integer] [Boolean] [n gt 5]]  def op (if (g 1) [1] [0]) end op`,
		`def g fn [[n:Integer] [Boolean] [n gt 5]]  def op (if (g 9) [1] [0]) end op`,
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
