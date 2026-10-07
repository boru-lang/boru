package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The seventieth increment: a fn def inside a runtime-conditional body at
// module scope is SPECULATIVE. Its install is placed at its site
// (OpBindResident through the interpreter's own installer, so an
// overlapping redefinition — family L — drops the standing overload exactly
// as the arm's run does), its dispatches route with a live lead
// (DISPATCH_GENERIC, at root too, with or without a forward slot) and run
// the live signature's own unit, and a miss raises the interpreter's
// undefined_word at the word. Measured on main before the increment: the
// arm's def replayed as a root twin BEFORE the branch and the dispatch was
// a committed call, so `def m {e: false}  if (m "e" get) [def f fn […]] []
// f 1` answered the arm's 101 where the interpreter raises.
func TestConditionalFnDefIsSpeculative(t *testing.T) {
	// Since phase 2 (design/IMMUTABLE-DEF.1.md §2.1) an arm, a loop body and
	// a callback body are BLOCKS: a fn def inside one is the block's own, so
	// a call past it finds the enclosing binding — the OUTER f where there
	// was one, undefined_word where the arm's def was fresh — on every path,
	// taken or not. The speculative family this increment placed (the
	// resident install, the routed dispatch, the generalised undef) is
	// mooted, and phase 4 retires it; each row below is the interpreter's
	// answer under the rule with the compiled lane agreeing or declining
	// (the check stop for a fresh family, the block gate for a shadow).
	const arm = `[def f fn [[x:Integer][Integer][x add 100]] end]`
	const outer = `def f fn [[x:Integer][Integer][x add 1]] end `
	const unbound = "ERROR:undefined word: f"
	for _, c := range []struct{ src, want string }{
		// A fresh def: the arm's own.
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] 1 f`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] (print "x") 1 f`, unbound},
		// A redefinition: the arm's shadow ends with the arm, the outer stands.
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "[2]"},
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, "[2]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] (print "x") 1 f`, "[2]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [def f fn [[x:Integer][Integer][x add 200]] end] f 1`, "[2]"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [def f fn [[x:Integer][Integer][x add 200]] end] f 1`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [def f fn [[x:Integer][Integer][x add 200]] end] f 1`, unbound},
		// An undef or a root redefinition after the arm acts on the outer.
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] undef f 9`, "[9]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] undef f 9`, "[9]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] def f fn [[x:Integer][Integer][x add 7]] end f 1`, "[8]"},
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] def f fn [[x:Integer][Integer][x add 7]] end f 1`, "[8]"},
		// Dispatched from a unit, a nested arm, a paren group, an each body.
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] def g fn [[][Integer][f 1]] end g`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] def g fn [[][Integer][f 1]] end g`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] if true [f 1] [0]`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] if true [f 1] [0]`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] (f 1) add 1`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] (f 1)`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] [1 2] each [f]`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] f (1 add 1)`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f (1 add 1)`, unbound},
		{`def m {e: true} end if (m "e" get) [def h fn [[xs:List n:Integer][Integer][(size xs) add n]] end] [] h [1 2] (1 add 1)`, "ERROR:undefined word: h"},
		{`def m {e: false} end if (m "e" get) [def h fn [[xs:List n:Integer][Integer][(size xs) add n]] end] [] h [1 2] (1 add 1)`, "ERROR:undefined word: h"},
		{`def id fn [[x:Any][Any][x]] end def m {e: true} end if (m "e" get) ` + arm + ` [] f (id 5)`, unbound},
		{`def id fn [[x:Any][Any][x]] end def m {e: false} end if (m "e" get) ` + arm + ` [] f (id 5)`, unbound},
		// Inside a fn body the arm is a block of the frame all the same.
		{`def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, unbound},
		{`def m {e: true} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, unbound},
		{`def m {e: true} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g g`, unbound},
		{`def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] (print "x") f 1]] end g`, unbound},
		{`def m {e: true} end if (m "e" get) [def f fn [[][][]] end] [] (print "x") f`, unbound},
		// The replaced outer is an exported module fn: its own unit answers.
		{`import module [def k 10 end def q fn [[x:Integer][Integer][x add k]] end export "A" {q:q/v}] end def k 50 end def f A.q/v end def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "[11]"},
		{`import module [def k 10 end def q fn [[x:Integer][Integer][x add k]] end export "A" {q:q/v}] end def k 50 end def f A.q/v end def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, "[11]"},
		// An in-arm undef of the arm's own def: nothing left either way.
		{`def m {e: true} end if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end undef f 1] [] 9`, "[1 9]"},
		// A loop body, a while body, an each body, a `do` body in a loop:
		// the same rule for every block; a lambda VALUE's def too.
		{outer + `for 2 ` + arm + ` f 1`, "[2]"},
		{outer + `var m {e: true} end while [m "e" get] [def f fn [[x:Integer][Integer][x add 100]] end var m {e: false} end] f 1`, "[2]"},
		{outer + `def m {e: true} end for 2 [if (m "e" get) ` + arm + ` []] f 1`, "[2]"},
		{`def m {e: false} end for 2 [if (m "e" get) ` + arm + ` []] 9`, "[9]"},
		{outer + `def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, "[2]"},
		{`def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end if true [def p (kk 8)] 3 p/v apply`, "[fn p(Integer)]"},
		{`def f (x:Integer => [x add 1]) end def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "[2]"},
		{`def m {e: true} end if (m "e" get) [def f (x:Integer => [x add 100]) end] [] f 1`, unbound},
		{`def m {e: false} end [1 2] each [if (m "e" get) ` + arm + ` []] f 1`, unbound},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f/v`, unbound},
		{`def k 5 end def m {e: false} end if (m "e" get) [undef k] [] if (m "e" get) [def f fn [[x:Integer y:Integer][Integer][x add y]] end] [] f (1 add 1) k`, unbound},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] 1 f/v apply`, unbound},
	} {
		ruleOrDecline(t, c.src, c.want)
	}
}

// The binding is placed on a long-lived registry (the check pass's join
// pushes the model's fn for the request; the run restores the base before
// it runs, as a placed undef's does), so a request after a not-taken arm
// finds the name unbound, and one after a taken arm finds the arm's fn.
func TestConditionalFnDefAcrossRequests(t *testing.T) {
	// Since phase 2 the arm's def is the arm's own: a request after a taken
	// arm finds the name unbound on the interpreter exactly as one after a
	// not-taken arm does. The compiled lane's root arm is still an inlined
	// region with no block of its own, so its install persists into the
	// next request — a known gap this test MEASURES until the compiler's
	// block scopes land (phase 2's second step); flip the `leaks` pin then.
	const arm = `if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end] [] 1`
	b := mustNew(t)
	for _, src := range []string{`def m {e: false}`, arm, `f 1`, `def m {e: true}`, arm, `f 1`} {
		gotI, errI := b.RunInterp(src)
		if src == `f 1` {
			if errI == nil || !strings.Contains(errI.Error(), "undefined word: f") {
				t.Errorf("%q: the arm's f is the arm's own on the interpreter, got %v / %v", src, gotI, errI)
			}
		} else if errI != nil {
			t.Errorf("%q: interpreter: %v", src, errI)
		}
	}
	a := mustNew(t)
	leaks := false
	for _, src := range []string{`def m {e: true}`, arm} {
		gotC, ran, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) || !ran || errC != nil {
			t.Fatalf("%q: the arm program compiles and runs: ran=%v %v", src, ran, errC)
		}
	}
	if gotC, _, errC := a.RunCompiled(`f 1`); errC == nil && fmt.Sprint(gotC) == "[101]" {
		leaks = true
	}
	if !leaks {
		t.Error("the compiled lane no longer leaks the arm's fn into the next request — the compiler's block scopes have landed for this shape: drop this pin and assert parity with the interpreter")
	}
}
