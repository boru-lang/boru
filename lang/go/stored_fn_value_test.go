package lang

import (
	"fmt"
	"testing"
)

// TestStoredFnValueParity pins S1b-3, re-landed 2026-09-22: `set`, `push`,
// `unshift` and `append`'s element form declare CompileStoresFn — a word that
// STORES what it is handed never steps it, so a fn-valued operand is inert
// to the recorder and the program compiles. The stored fn is invoked LATER,
// from the container, and each shape below runs on both lanes and must agree.
//
// The first landing (2026-09-19) was reverted because it unmasked NUR169: a
// one-value paren netting a function, applied by the interpreter and pushed
// as data by the compiled lane. NUR173 fixed that on 2026-09-20 (the guarded
// re-step landing), so the review's first witness — `(m.f)` after a set —
// now answers 42 on both lanes; it is the first row here.
func TestStoredFnValueParity(t *testing.T) {
	for _, src := range []string{
		// NUR169's first witness: a stored 0-arg fn read through a one-value paren
		`def h fn [[] [Integer] [42]] end def m ({} set 'f' h/v) end (m.f)`,
		// a stored callback's type, read back
		`def reg (flex {}) end def h fn [[n:Integer][Integer][n add 1]] end reg set 'cb' h/v drop end typeof (reg.cb)`,
		// a reducer and a predicate registered in a flex map, driven from it
		`def reg (flex {}) end def step fn [[a:Integer e:Integer][Integer][a add e]] end reg set 'f' step/v drop end 0 fold reg.f [1 2 3]`,
		`def reg (flex {}) end def p fn [[kv:Map][Boolean][kv.value gt 1]] end reg set 'f' p/v drop end filter reg.f [1 2 3]`,
		// a body-local fn pushed into a list and applied from it
		`def f fn [[] [Integer] [def g fn [[x:Integer] [Integer] [x add 1]] def lst (push g/v []) end 41 (lst.0)/v apply]]  f`,
		// a loop building a list of closures
		`def f fn [[] [Integer] [def fns [] end for [0 2] [def fns (push ([] => [i]) fns)] end size fns]]  f`,
		// unshift and append, the same rule (the element read back and
		// introspected — applying it at the top level is the next gate,
		// "apply over a dynamic lead", not this one's)
		`def g fn [[x:Integer] [Integer] [x mul 3]] end def lst (unshift g/v []) end typeof (lst.0)`,
		`def g fn [[x:Integer] [Integer] [x mul 3]] end def lst (flex []) end lst append g/v drop end typeof (lst.0)`,
		`def f fn [[] [Integer] [def g fn [[x:Integer] [Integer] [x mul 3]] def lst (unshift g/v []) end 4 (lst.0)/v apply]]  f`,
		// a module export stored in a flex registry
		`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def r (flex {}) end r set 'cb' M.inc/v drop end typeof (r.cb)`,
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

// TestStoredFnValueNeverSilentlyWrong pins the second NUR169 witness the
// revert cited: closures pushed from a loop body and applied through a
// lambda. The lambda reads the loop iterator `i` AFTER the loop, when the
// interpreter no longer binds it, so the interpreter raises undefined_word
// (measured 2026-09-22 — the `[0 1]` the revert recorded was an earlier
// oracle); the compiled lane fails to compile at the check on the same read.
// What this test forbids is the state the review found: compiling and
// answering `[fn g fn g]`. A loud failure on either lane is the bar.
func TestStoredFnValueNeverSilentlyWrong(t *testing.T) {
	src := `def fns [] end for [0 2] [def fns (push ([] => [i]) fns)] end each ([g:Function] => [(g)]) fns`
	gotI, errI := mustNew(t).RunInterp(src)
	if codeOf(errI) != "undefined_word" {
		t.Fatalf("interpreter oracle moved: err=[%s] got=%v, re-derive this pin", codeOf(errI), gotI)
	}
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return // a loud compile failure — the iterator read inside the lambda, owed its own fix
	}
	if codeOf(errC) != "undefined_word" || len(gotC) != 0 {
		t.Errorf("%s: compiled err=[%s] got=%v, want undefined_word — a stored closure list must not apply as data", src, codeOf(errC), gotC)
	}
}
