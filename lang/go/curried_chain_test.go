package lang

import (
	"fmt"
	"strings"
	"testing"
)

// curried_chain_test.go pins the curried chain (2026-09-22): a paren whose
// LEADING value is a closure a compiled factory call PRODUCED, re-stepped
// by the paren's rewind over the arguments its closure provably takes —
// `((mk 1) 2)`, `(((mk3 1) 2) 3)` (callbacks.tsv L151) — recorded at the
// collapse as the same apply event the trailing spelling seats (core's
// parenProducedLeadApplyIdx over the ProducedLeadApplies seam), so the
// enclosing scope meets ONE value. Three defects the increment closed:
//
//   - NUR178: the window LEAKED. At the top level the residual classifier
//     applied the closure only after a later word had collected the
//     window's argument: `((mk 1) 2) mul 10` lowered `mul` over 2 and 10
//     and applied the closure to the product — 21, silent, for 30.
//   - NUR179: a two-param closure under the fn-value-call ops bound its
//     first param to the value farthest from it (the VM handed a positional
//     window to the token seam, which reversed it again): `(2 3 (mk2 1))`
//     compiled 24 for 33, `7 5 (mk2 1)/v apply` 76 for 58.
//   - the leading form's no-match order: `((mk 1) "s")` compiled `[s fn]`
//     for the interpreter's `[fn s]`.

const ccMk = `def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]]  `
const ccMk2 = `def mk2 fn [[n:Integer][Function][( fn [[x:Integer y:Integer][Integer][(x mul 10) add y add n]] )]]  `
const ccMk3 = `def mk3 fn [[a:Integer][Function][( fn [[b:Integer][Function][( fn [[c:Integer][Integer][a add b add c]] )]] )]]  `

// TestCurriedChainParity: every row compiles and agrees with the
// interpreter.
func TestCurriedChainParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{ccMk + `((mk 1) 2) mul 10`, "[30]", "NUR178's witness: the window no longer leaks to mul"},
		{ccMk + `((mk 1) 2) add 10`, "[13]", "the add twin (the leak was hidden there by arithmetic)"},
		{ccMk + `((mk 1) 2)`, "[3]", "the one-level chain alone"},
		{ccMk3 + `(((mk3 1) 2) 3)`, "[6]", "the two-level chain"},
		{`def mk fn [[a:Integer] [Function] [([b:Integer] => [([c:Integer] => [a add b add c])])]]  (((mk 1) 2) 3)`, "[6]", "callbacks.tsv L151's own spelling: `=>` lambdas, count contracts only"},
		{ccMk3 + `(((mk3 1) 2) 3) add 1`, "[7]", "the chain's value feeds a later word"},
		{ccMk3 + `((mk3 1) 2) 3`, "[fn (Integer) 3]", "the apply's closure result is PLACED where it lands, like a call's"},
		{ccMk2 + `((mk2 1) 2 3)`, "[24]", "a two-argument window binds in WRITTEN order"},
		{ccMk2 + `((mk2 1) 2 3) mul 2`, "[48]", "…and its value feeds a later word"},
		{ccMk + `5 ((mk 1) 2)`, "[5 3]", "a deeper value survives beneath"},
		{ccMk + `((mk 1) 2) 7`, "[3 7]", "a value after the chain"},
		{ccMk + `[((mk 1) 2) 7]`, "[[3 7]]", "a list element"},
		{ccMk + `((mk 1) (2 add 3)) mul 10`, "[60]", "a computed argument"},
		{ccMk + `((mk 1) ((mk 2) 3))`, "[6]", "a chain as the argument of a chain"},
		{ccMk + `(((mk 1) 2) add 3)`, "[6]", "the chain's value inside an outer paren"},
		{ccMk + `((mk 1) 2) ((mk 5) 6)`, "[3 11]", "two chains"},
		{ccMk + `def f fn [[][Integer][((mk 1) 2) mul 10]]  f`, "[30]", "inside a fn body"},
		{ccMk + `def f fn [[q:Integer][Integer][((mk q) 2) mul 10]]  f 1`, "[30]", "inside a fn body over a param"},
		{ccMk3 + `def f fn [[][Integer][(((mk3 1) 2) 3) add 1]]  f`, "[7]", "the two-level chain inside a fn body"},
		{ccMk + `do [((mk 1) 2) mul 10]`, "[30]", "inside a do body"},
		{ccMk + `if true [((mk 1) 2) mul 10] [0]`, "[30]", "inside a branch arm"},
		{ccMk + `def xs [1 2 3]  xs each ([e:Integer] => [((mk 1) e) mul 10])`, "[[20 30 40]]", "inside a named-param callback over its element"},
		{ccMk + `def xs [1 2 3]  xs each [((mk 1) 2) mul 10]`, "[[30 30 30]]", "an unnamed-param body keeps its whole-frame replay"},
		{ccMk + `def c (mk 1) end (c 2) mul 10`, "[30]", "a def-read lead stays with the read model"},
		{ccMk + `((mk 1) "s")`, "[fn (Integer) s]", "a no-match under the leading form leaves the window as written"},
		{ccMk + `((mk 1) 2.5)`, "[fn (Integer) 2.5]", "…for a Decimal too"},
		{`def mk fn [[n:Integer][Function][( fn [[x:Number][Number][x add n]] )]]  ((mk 1) 2.5) mul 2`, "[7.0]", "a Number param takes a Decimal"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s): %v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestFnValueApplyBindingOrderParity pins NUR179 across every fn-value-call
// op that hands a compiled fn-VALUE closure its window: the trailing paren
// apply, the `apply` word, a Function param's leading and trailing
// windows, a bare param dispatch, and a member call. The closure's body is
// order-sensitive (`(x mul 10) add y`), so a swapped binding cannot hide.
func TestFnValueApplyBindingOrderParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{ccMk2 + `(2 3 (mk2 1))`, "[33]", "the trailing paren apply binds top-down (x = 3)"},
		{ccMk2 + `7 5 (mk2 1)/v apply`, "[58]", "the apply word binds top-down (x = 5)"},
		{`def kk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a mul 10) add b add k])]]  1 2 (kk 7) apply`, "[28]", "a lambda closure under apply"},
		{ccMk2 + `def f fn [[g:Function][Integer][(2 3 g)]]  f (mk2 1)`, "[33]", "a Function param's trailing window"},
		{ccMk2 + `def f fn [[g:Function][Integer][(g 2 3)]]  f (mk2 1)`, "[24]", "a Function param's leading window binds in written order"},
		{ccMk2 + `def f fn [[g:Function][Integer][g 2 3]]  f (mk2 1)`, "[24]", "a bare param dispatch"},
		{`def m {g: ([x:Integer y:Integer] => [(x mul 10) add y])}  m.g 2 3`, "[23]", "a member call"},
		{`def m {g: ([x:Integer y:Integer] => [(x mul 10) add y])}  (m.g 2 3)`, "[23]", "a paren-bounded member call"},
		{`def m {g: ([x:Integer y:Integer] => [(x mul 10) add y])}  ((m 'g' get) 2 3)`, "[23]", "a fetched member applied"},
		// the method op over a CAPTURING closure member (a ClosurePayload,
		// where the const lambda above never reached the seam): 33 for 24
		// on every spelling before the fix
		{ccMk2 + `def m {g: (mk2 1)}  (m.g 2 3)`, "[24]", "a paren-bounded member call of a capturing closure"},
		{ccMk2 + `def m {g: (mk2 1)}  m.g 2 3`, "[24]", "the tail form"},
		{ccMk2 + `def m {g: (mk2 1)}  ((m 'g' get) 2 3)`, "[24]", "the fetched member"},
		{ccMk2 + `def fs [(mk2 1)]  (fs.0 2 3)`, "[24]", "a list element"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s): %v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestCurriedChainSoundCompileFailures: the windows the arm leaves to the
// residual classifier decline LOUDLY — never the silent leak NUR178 was.
func TestCurriedChainSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// a window wider than the closure's arity: the re-step leaves a survivor
		{ccMk + `((mk 1) 2 3)`, "arity mismatch", "[3 3]"},
		// a no-match window with a value after it
		{ccMk + `((mk 1) "s") 7`, "arity mismatch", "[fn (Integer) s 7]"},
		// a gradual argument that may be a fn: the arm stands aside, and the
		// later word's collection of the window is the hazard NUR121 names
		// (the leak NUR178 was, made loud: the inner paren's placed mark no
		// longer exempts a lead the outer paren re-stepped)
		{ccMk + `def f fn [[m:Map][Integer][((mk 1) m.x) mul 10]]  f {x: 2}`, "NUR121", "[30]"},
		// a chain under a `def`'s pending collection: the outer paren hands
		// its survivors to the def as arguments — r binds the closure, 3
		// stays on the stack — and the read model declines r's bare read
		{ccMk3 + `def r (((mk3 1) 2) 3) end r`, "ends short", "[6]"},
		// a chain's VALUE def-bound and read back: the read model classes
		// every recorded apply's result as a produced fn value, whatever
		// its type (producedFnValue), so the bare read declines as a
		// computed fn's — loud, and the read model's own item
		{ccMk + `def r (((mk 1) 2)) end r`, "computed fn", "[3]"},
	}
	for _, c := range rows {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a compile failure", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}

// TestCurriedChainPendingCollection: a collapse driven by a PENDING forward
// collection hands its survivors to that collection as arguments and
// re-steps nothing — `a add ((mk 1) e)` is add's no-match over the closure
// on the interpreter — so the arm stands aside there and both lanes raise.
func TestCurriedChainPendingCollection(t *testing.T) {
	src := ccMk + `def xs [1 2 3]  0 fold ([a:Integer e:Integer] => [a add ((mk 1) e)]) xs`
	gotI, errI := mustNew(t).RunInterp(src)
	if codeOf(errI) != "signature_error" {
		t.Fatalf("interpreter oracle moved: got %v err=[%s] — re-derive", gotI, codeOf(errI))
	}
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return // a loud decline satisfies the bar
	}
	if compiled && errC == nil {
		t.Errorf("compiled answered %v where the interpreter raises", gotC)
	}
}

// TestUnnamedFrameApplyResultTyped pins the NUR180 mitigation: a trailing
// paren apply of a NAMED concrete fn nets a value of its declared return,
// so a typed consumer inside an unnamed-param frame matches statically
// instead of taking the checker's recovery over the frame's input.
func TestUnnamedFrameApplyResultTyped(t *testing.T) {
	rows := []struct{ src, want string }{
		{`def inc2 fn [[x:Integer][Integer][x add 2]]  def xs [1 2 3]  xs each [(2 inc2/v) mul 10]`, "[[40 40 40]]"},
		{`def inc2 fn [[x:Integer][Integer][x add 2]]  def f fn [[Integer][Integer][(2 inc2/v) mul 10]]  f 1`, "[40]"},
		{`def inc2 fn [[x:Integer][Integer][x add 2]]  (2 inc2/v) mul 10`, "[40]"},
		// NUR180's Any-result rows (2026-09-23): the checker's recovery lays
		// its window out forward-first, as the interpreter's matcher does, so
		// a typed word over a strict-Any result takes the WRITTEN argument in
		// an unnamed frame too (checkModeFallbackPositionsFor, check/go).
		{ccMk + `def xs [1 2 3]  xs each [(2 (mk 1)) mul 10]`, "[[30 30 30]]"},
		{`def xs [1 2 3]  xs each [(2 ([x:Integer] => [x add 2])) mul 10]`, "[[40 40 40]]"},
		{ccMk + `def f fn [[Integer][Integer][(2 (mk 1)) mul 10]]  f 1`, "[30]"},
		{ccMk + `def xs [1 2 3]  xs each ([e:Integer] => [(2 (mk 1)) mul 10])`, "[[30 30 30]]"},
		// The same window fault, one level down (generics-fn.tsv:L55): a
		// fold body's two inputs satisfied `dot`'s two-arg window and the
		// written key was stepped as a word — a FALSE `undefined word:
		// value` diagnostic, and no Program. It checks clean and compiles
		// now (the body a raw token body at the callback seam).
		{`def Box gen [T] class {value:T} def sumvals gen [T] fn [[bs:[:T]] [Integer] [0 fold [dot value add] bs]] def xs [(make (Box of [Integer]) {value:10}) (make (Box of [Integer]) {value:20})] end sumvals xs`, "[30]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", c.src, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s", c.src, gotC, c.want)
		}
	}
}

// TestUnnamedFrameApplyResultAnyPending kept NUR180's OPEN half until
// 2026-09-23, when the recovery's window became the interpreter's; what is
// left of it is NUR184's: a paren closing UNDER A PENDING FORWARD hands its
// survivors to that forward (`add (2 (mk 1))` collects the 2, and the
// closure then re-steps over the sum), which the trailing-apply record
// does not model. The pin fails the day the row agrees: retire it with
// NUR184 (TestParenTrailingFnForwardCollectsPending holds its siblings).
func TestUnnamedFrameApplyResultAnyPending(t *testing.T) {
	src := ccMk + `def xs [1 2 3]  0 fold [add (2 (mk 1))] xs`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[6]" {
		t.Fatalf("%q: interpreter oracle moved: %v err=%v — re-derive NUR184", src, gotI, errI)
	}
	if !compiled || errC != nil {
		t.Errorf("%q: NUR184's fold row became loud (%v) — record that and retire this pin", src, errC)
	} else if fmt.Sprint(gotC) == "[6]" {
		t.Errorf("%q: NUR184's fold row agrees (%v) — move it to TestUnnamedFrameApplyResultTyped", src, gotC)
	}
}
