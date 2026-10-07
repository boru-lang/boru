package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR344ParkedFnTakesTheLaterGroup pins NUR344's close. A paren whose
// LAST survivor is a fn value (`("s" lam/v)`) is re-stepped by the paren's
// rewind, and the re-stepped value forward-collects the token after the
// close FIRST (execFnDefLiteral: forward tokens, then the stack): a paren
// group, a dot reach, an interpolated string or a word bound to a value or a
// data splice is evaluated for its slot and taken when it fits —
// `("s" lam/v) (2 add 3)` is `[s 105]`. The check pass's collapse asked
// "does the value collect what follows?" only of a LITERAL (and an open
// paren marker), so a paren expression, a reach, a template or a value word
// read as "no": it recorded an apply over the values INSIDE the paren, which
// parks at run time, and the later value landed beside the unapplied fn
// (`[s fn lam(Integer) 5]`, silent). The collapse now leaves every token
// whose value exists only once it runs to the re-step (only a function
// word — the strict barrier — and the structural stops say no), and a value
// its re-step then matches against nothing parks as paren-placed data
// (CheckState.TrailingDeferredFnIDs → ParenPlacedFnIDs).
func TestNUR344ParkedFnTakesTheLaterGroup(t *testing.T) {
	const lam = `def lam ([x:Integer] => [x add 100]) end `
	for _, tc := range []struct{ src, want string }{
		// the register's three silent repros
		{lam + `("s" lam/v) (2 add 3)`, `[s 105]`},
		{lam + `({a:1} lam/v) (5)`, `[{a:1} 105]`},
		{lam + `def w word [5] end ("s" lam/v) w`, `[s 105]`},
		// the literal twin that always agreed
		{lam + `("s" lam/v) 5`, `[s 105]`},
		// a value word, a reach, a user call in a group, a template
		{lam + `def v 5 end ("s" lam/v) v`, `[s 105]`},
		{lam + `def m {a:5} end ("s" lam/v) m.a`, `[s 105]`},
		{lam + `def g fn [[] [Integer] [5]] end ("s" lam/v) (g)`, `[s 105]`},
		{`def lam ([x:String] => [x]) end (5 lam/v) ` + "`a${1}`", `[5 a1]`},
		// a multi-value group or splice: the first value is taken
		{lam + `("s" lam/v) (1 2)`, `[s 101 2]`},
		{lam + `def w word [5 6] end ("s" lam/v) w`, `[s 105 6]`},
		// a later statement, a later literal, an effect in the group
		{lam + `("s" lam/v) (2 add 3) end 7`, `[s 105 7]`},
		{lam + `("s" lam/v) (2 add 3) 9`, `[s 105 9]`},
		{lam + `("s" lam/v) (print "p" 5)`, `[s 105]`},
		// a two-param lead collecting two groups
		{`def lam2 ([x:Integer y:Integer] => [x sub y]) end ("s" lam2/v) (10) (3)`, `[s 7]`},
		// an enclosing paren, list and arm; a def; a consumer
		{lam + `(("s" lam/v) (2 add 3))`, `[s 105]`},
		{lam + `[("s" lam/v) (2 add 3)]`, `[['s' 105]]`},
		{lam + `if true [("s" lam/v) (2 add 3)] [0]`, `[s 105]`},
		{lam + `def r (("s" lam/v) (2 add 3)) end r`, `[105 s]`},
		{lam + `print ("s" lam/v) (2 add 3)`, `[105]`},
		// inside a fn body: a group over a param, a bare param
		{lam + `def f fn [[n:Integer] [Any] [[("s" lam/v) (n add 3)]]] end f 4`, `[['s' 107]]`},
		{lam + `def f fn [[n:Integer] [Any] [[("s" lam/v) n]]] end f 4`, `[['s' 104]]`},
		{lam + `def g fn [[f:Function] [Any] [[("s" f/v) (2 add 3)]]] end g lam/v`, `[['s' 105]]`},
		// a member lead, a produced lead, a returned closure
		{lam + `def m {f:lam/v} end ("s" m.f) (2 add 3)`, `[s 105]`},
		{lam + `def mk fn [[] [Any] [lam/v]] end ("s" (mk)) (2 add 3)`, `[s 105]`},
		{`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] end ("s" (mk 1)) (2 add 3)`, `[s 6]`},
		// a List param over a pending list literal
		{`def lam ([x:List] => [x]) end ("s" lam/v) [1 (1 add 1)]`, `[s [1 2]]`},
	} {
		requireCompiledParity(t, tc.src)
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}

// TestNUR344GroupTheLeadDoesNotTake is NUR344's negative half: the value the
// group (or word) yields is no argument of the re-stepped fn, which then
// parks where it lands — data beside it on both lanes, as the literal twin
// `("s" lam/v) "x"` always was — and a function word after the close is the
// strict barrier, which no collection crosses.
func TestNUR344GroupTheLeadDoesNotTake(t *testing.T) {
	const lam = `def lam ([x:Integer] => [x add 100]) end `
	for _, tc := range []struct{ src, want string }{
		{lam + `("s" lam/v) "x"`, `[s fn lam(Integer) x]`},
		{lam + `("s" lam/v) ("x")`, `[s fn lam(Integer) x]`},
		{lam + `("s" lam/v) ()`, `[s fn lam(Integer)]`},
		{lam + `("s" lam/v) ` + "`a${1}`", `[s fn lam(Integer) a1]`},
		{lam + `def v "q" end ("s" lam/v) v`, `[s fn lam(Integer) q]`},
		{lam + `def v "q" end [("s" lam/v) v]`, `[['s' fn lam(Integer) 'q']]`},
		{lam + `("s" lam/v) [1 2]`, `[s fn lam(Integer) [1 2]]`},
		{lam + `("s" lam/v) ("x") 5`, `[s fn lam(Integer) x 5]`},
		{lam + `(("s" lam/v) ("x") 5)`, `[s fn lam(Integer) x 5]`},
		// the stack beneath still takes what the group does not
		{lam + `(5 lam/v) ("x")`, `[105 x]`},
		{lam + `((5 lam/v) ("x"))`, `[105 x]`},
		{`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] end ("s" (mk 1)) ("x")`, `[s fn (Integer) x]`},
		{lam + `def f fn [[n:Any] [Any] [[("s" lam/v) (n)]]] end [(f 4) (f "x")]`, `[[['s' 104] ['s' fn lam(Integer) 'x']]]`},
		// a function word after the close: the barrier, then its own call
		{lam + `def w word [add 1] end 3 ("s" lam/v) w`, ``},
	} {
		requireCompiledParity(t, tc.src)
		if tc.want == "" {
			continue
		}
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}

// TestNUR344LandingAtTheProgramEnd pins NUR344's loud remainder: a `do`'s
// fn result landing over values beneath it that a later event took
// (LandingBeneathGuard, NUR286) — `({a:1} lam/v) do [lam/v]` — was the
// designed defer, since the VM could not tell whether a token after the
// landing would be collected nor whether the stack beneath it was the
// interpreter's. At the program's end neither is in doubt: the value landed
// at the tape's end at every step, and when its landing is the program's
// last op the compiled stack beneath it is the interpreter's (compiler
// LandingBeneathHeld). The interpreter's step over it is then the VM's to
// take: the no-match verdict parks the value as data or raises a named
// one's uncalled_function, and any other step runs on the island, whose
// residual is the program's.
func TestNUR344LandingAtTheProgramEnd(t *testing.T) {
	const lam = `def lam ([x:Integer] => [x add 100]) end `
	for _, tc := range []struct{ src, want string }{
		{lam + `({a:1} lam/v) do [lam/v]`, `[{a:1} fn lam(Integer) fn lam(Integer)]`}, // the register's witness
		{lam + `("s" lam/v) do [lam/v]`, `[s fn lam(Integer) fn lam(Integer)]`},
		{`def g fn [[x:Map][Any][x]] end ` + lam + `({a:1} lam/v) do [lam/v]`, `[{a:1} fn lam(Integer) fn lam(Integer)]`},
		// the step applies the fn over the values beneath: the island's
		{lam + `(5 lam/v) do [lam/v]`, `[205]`},
		{lam + `({a:1} 3 lam/v) do [lam/v]`, `[{a:1} 203]`},
		{`def lam ([x:Integer] => [x print "p" add 100]) end (5 lam/v) do [lam/v]`, `[205]`},
		{`def lam ([x:Map] => [x keys]) end ({a:1} lam/v) do [lam/v]`, `[['a'] fn lam(Map)]`},
		{`def lam ([x:Integer] => [x div 0]) end (5 lam/v) do [lam/v]`, `ERROR:division by zero`},
		{`def lam ([x:Integer] => [4 div (x sub 4)]) end (5 lam/v) do [lam/v]`, `ERROR:division by zero`}, // the landing's own step raises
		// a named fn no signature admits raises where it lands
		{`def lam fn [[x:Integer][Integer][x add 100]] end ({a:1} lam/v) do [lam/v]`, `ERROR:call to 'lam' matched no signature`},
		{`def lam fn [[x:Integer][Integer][x add 100]] end "s" 7 drop do [lam/v]`, `ERROR:call to 'lam' matched no signature`},
		{`def an ([x:Integer] => [x]) end def lam fn [[x:Integer][Integer][x add 100]] end ({a:1} an/v) do [lam/v]`, `ERROR:call to 'lam' matched no signature`},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	// Negative: a landing some op follows is no program's end, and keeps
	// the designed defer.
	requireLoudDefer(t, lam+`({a:1} lam/v) do [lam/v] end 7`, "no compiled apply re-steps it (NUR286)", `[{a:1} fn lam(Integer) fn lam(Integer) 7]`)
}

// TestNUR350FrameArgsAlwaysReal pins NUR350's verdict (it replaces NUR346's
// mirrored elision). A fn body that needs no frame state and never reads
// `args` used to push the shared EMPTY args list (buildFnBodyHandler), so
// code the frame runs that the construction-time walk cannot see — a
// computed body (`each (mk) […]` over a list holding `args`), a word macro
// bound after the fn — read `[]`, and the VM mirrored it (ArgsElided). Every
// frame now holds the call's REAL args on both lanes: the interpreter pushes
// the leaf frame's copy lazily (ArgsStack.PushLazy, no allocation of its
// own), the VM's frames and seams push the real list.
func TestNUR350FrameArgsAlwaysReal(t *testing.T) {
	const mk = `def mk fn [[] [List] [quote [args]]] end `
	const w = `def w fn [[x:Integer] [Any] [each (mk) [x 5]]] end `
	for _, tc := range []struct{ src, want string }{
		// the register's repro, two calls, a list of calls
		{mk + w + `w 7`, `[[[7] [7]]]`},
		{mk + w + `w 7 end w 8`, `[[[7] [7]] [[8] [8]]]`},
		{mk + w + `[(w 7) (w 8)]`, `[[[[7] [7]] [[8] [8]]]]`},
		// a tail call, an overloaded fn, a call from a fn that reads args
		{mk + `def w fn [[x:Integer][Any][if (x gt 3) [each (mk) [x 5]] [w (x add 1)]]] end w 1`, `[[[4] [4]]]`},
		{mk + `def w fn [[x:Integer][Any][each (mk) [x 5]] [s:String][Any][each (mk) [s 5]]] end [(w 1) (w "a")]`, `[[[[1] [1]] [['a'] ['a']]]]`},
		{mk + w + `def v fn [[x:Integer][Any][args drop w x]] end v 3`, `[[[3] [3]]]`},
		{mk + w + `def v fn [[y:Integer][Any][args drop [(each (mk) [y]) (w y)]]] end v 3`, `[[[[3]] [[3] [3]]]]`},
		// an unnamed param, a branch body
		{mk + `def w fn [[Integer][Any][each (mk) [1 5]]] end w 3`, `[[[3] [3]]]`},
		{mk + `def w fn [[x:Integer][Any][if true (mk) [0]]] end w 3`, `[[3]]`},
		// a lambda: a named call, a paren apply, a member apply
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end lam 7`, `[[[7] [7]]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end (lam/v 7)`, `[[[7] [7]]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end def m {f:lam/v} end m.f 7`, `[[[7] [7]]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end def m {f:lam/v} end each m.f [1 2]`, `[[[[1] [1]] [[2] [2]]]]`},
		// the token seam stepping a fn value, apply, a Function param
		{mk + w + `each w/v [1 2]`, `[[[[1] [1]] [[2] [2]]]]`},
		{mk + w + `(w/v 7)`, `[[[7] [7]]]`},
		{mk + w + `w/v apply 7`, `[[[7] [7]]]`},
		{mk + w + `def g fn [[f:Function] [Any] [f 7]] end g w/v`, `[[[7] [7]]]`},
		// a word macro bound after the fn splices an `args` read
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end w 3`, `[[3]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end [(w 3) (w 4)]`, `[[[3] [4]]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args.0] end w 3`, `[3]`},
		{`def w ([x:Integer] => [m]) end def m word [args] end w 3`, `[[3]]`},
		{`def w fn [[Integer][Any][m]] end def m word [args] end w 3`, `[[3]]`},
	} {
		requireCompiledParity(t, tc.src)
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}

// TestNUR346RealArgsFramesStayReal is NUR346's negative half: every frame
// that pushes the REAL args list keeps it on both lanes — a body that reads
// `args` or needs frame state (`do`, `def`), a macro bound BEFORE the fn
// (the construction-time walk sees it), and a callback seam's CallBoru (the
// map-iteration `each`, `filter`, a stored lambda under a map `each`).
func TestNUR346RealArgsFramesStayReal(t *testing.T) {
	const mk = `def mk fn [[] [List] [quote [args]]] end `
	for _, tc := range []struct{ src, want string }{
		{mk + `def w fn [[x:Integer] [Any] [args drop each (mk) [x 5]]] end w 7`, `[[[7] [7]]]`},
		{mk + `def w fn [[x:Integer] [Any] [do (mk)]] end [(w 7) (w 8)]`, `[[[7] [8]]]`},
		{mk + `def w fn [[x:Integer] [Any] [def z 1 do (mk)]] end w 7`, `[[7]]`},
		{mk + `def lam ([x:Integer] => [do (mk)]) end lam 7`, `[[7]]`},
		{mk + `each ([x:Integer] => [do (mk)]) [1 2]`, `[[[1] [2]]]`},
		{`def m word [args] end def w fn [[x:Integer][Any][m]] end w 3`, `[[3]]`},
		{mk + `def m word [args] end def w fn [[x:Integer][Any][[(each (mk) [x]) m]]] end w 3`, `[[[[3]] [3]]]`},
		{mk + `each ([kv:Any] => [each (mk) [1]]) {a:1 b:2}`, `[{a:[[1]] b:[[2]]}]`},
		{mk + `each ([kv:KeyVal] => [each (mk) [1]]) {a:1 b:2}`, `[{a:[[{k:'a' v:1 i:0 n:2}]] b:[[{k:'b' v:2 i:1 n:2}]]}]`},
		{mk + `def w fn [[kv:Any][Any][each (mk) [1]]] end each w/v {a:1}`, `[{a:[[1]]}]`},
		{mk + `def w fn [[x:Any][Any][each (mk) [x] drop true]] end filter w/v [1 2]`, `[[1 2]]`},
		// a later-bound macro's args read in a fn that returns too much
		// raises the same count error on both lanes
		{mk + `def w fn [[x:Integer][Any][each (mk) [x] m]] end def m word [args] end w 3`, ``},
	} {
		requireCompiledParity(t, tc.src)
		if tc.want == "" {
			continue
		}
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}

// TestNUR346ForeignCallKeepsRealArgs: a module fn called from the importer
// (the handler's CallBoruStrict arm) and from inside the module (the leaf
// handler) holds the call's real args either way (NUR350) — the review of
// #522's split between the two (real from outside, empty from inside) is
// gone, and a module-home unit's late `args` read now compiles and agrees
// instead of declining.
func TestNUR346ForeignCallKeepsRealArgs(t *testing.T) {
	const mod = `import module [def w fn [[x:Integer][Any][m]] end def m word [args] end def v fn [[x:Integer][Any][w x]] end export "M" {w:w/v v:v/v}] end `
	const modBody = `import module [def mk fn [[] [List] [quote [args]]] end def w fn [[x:Integer] [Any] [each (mk) [x 5]]] end def v fn [[x:Integer][Any][w x]] end export "M" {w:w/v v:v/v}] end `
	for _, tc := range []struct{ src, want string }{
		// a module fn whose body runs a computed `quote [args]` body: the
		// real args from the importer, the empty list from inside
		{modBody + `M.w 7`, `[[[7] [7]]]`},
		{modBody + `M.v 8`, `[[[8] [8]]]`},
		{modBody + `[(M.w 7) (M.v 8)]`, `[[[[7] [7]] [[8] [8]]]]`},
		{modBody + `each M.w/v [1 2]`, `[[[[1] [1]] [[2] [2]]]]`},
		{modBody + `7 M.w/v apply`, `[[[7] [7]]]`},
		{modBody + `(M.w/v 7)`, `[[[7] [7]]]`},
		// the module late-macro fn reached only through paths that run it
		// on the interpreter's own terms
		{mod + `def m {f:M.w/v} end m.f 7`, `[[7]]`},
		{mod + `def f M.w/v end f 7`, `[[7]]`},
		{mod + `def g fn [[f:Function][Any][f 7]] end g M.w/v`, `[[7]]`},
		// a body that reads args itself pushes the real list at home too
		{`import module [def w fn [[x:Integer][Any][args drop m]] end def m word [args] end export "M" {w:w/v}] end M.w 7`, `[[7]]`},
		// program-home late-macro reads: the real list
		{`def w fn [[x:Integer][Any][m] [s:String][Any][m]] end def m word [args] end def g fn [[a:Any][Any][w a]] end [(g 1) (g "s")]`, `[[[1] ['s']]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end def v fn [[y:Integer][Any][w y]] end v 7`, `[[7]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end 7 w/v apply`, `[[7]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end each w/v [1 2]`, `[[[1] [2]]]`},
		{`def w fn [[acc:Any kv:Any][Any][m]] end def m word [args] end fold w/v {a:1} 0`, `[[0 1]]`},
		{`def w fn [[acc:Any kv:KeyVal][Any][m]] end def m word [args] end fold w/v {a:1} 0`, `[[0 {k:'a' v:1 i:0 n:1}]]`},
		{`def w fn [[kv:Any][Any][m]] end def m word [args] end each w/v {a:1}`, `[{a:[1]}]`},
	} {
		requireCompiledParity(t, tc.src)
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
	// A module-home unit's late `args` read: the real list, both lanes.
	for _, tc := range []struct{ src, want string }{
		{mod + `M.w 7`, `[[7]]`},
		{mod + `7 M.w/v apply`, `[[7]]`},
		{mod + `(M.w/v 7)`, `[[7]]`},
		{mod + `each M.w/v [1 2]`, `[[[1] [2]]]`},
		{mod + `M.v 8`, `[[8]]`},
		{mod + `[(M.w 7) (M.v 8) (M.w 9)]`, `[[[7] [8] [9]]]`},
		{`import module [def w fn [[x:Integer][Any][m] [s:String][Any][m]] end def m word [args] end export "M" {w:w/v}] end def g fn [[a:Any][Any][M.w a]] end [(g 1) (g "s")]`, `[[[1] ['s']]]`},
	} {
		requireCompiledParity(t, tc.src)
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}

// TestNUR346ArgsElisionOtherUnitCompiles pins the two remaining unit-compile
// paths a frame's args list rides (NUR346, NUR350: always the real list): a
// run-time dispatched overload arm (CALL_USER_POLY — compileUserPolyArm,
// call-site specialisation off so the generic arm table runs) and the outer
// overload a conditional redefinition replaces (check.CompileFnSigUnit, the
// routed dispatch of a speculative fn def).
func TestNUR346ArgsElisionOtherUnitCompiles(t *testing.T) {
	const mk = `def mk fn [[] [List] [quote [args]]] end `
	poly := mk + `def wrapfn fn [[m:Map] [Any] [def helper fn [[a:Integer] [Any] [each (mk) [a 5]] [b:String] [Any] [7]] helper (m get k/q)]] wrapfn {k:3}`
	if dis := compileDisasmNoSpec(t, poly); !strings.Contains(dis, "CALL_USER_POLY") {
		t.Errorf("expected a CALL_USER_POLY lowering:\n%s", dis)
	}
	gotC, compiled, errC, gotI, errI := runBothEnginesNoSpec(t, poly)
	if !compiled || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) || fmt.Sprint(gotI) != `[[[3] [3]]]` {
		t.Errorf("%q: compiled %v [%v] (compiled=%v), interpreted %v [%v], want [[[3] [3]]]", poly, gotC, errC, compiled, gotI, errI)
	}
	const outer = `def f fn [[x:Integer][Any][each (mk) [x 5]]] end `
	const arm = `[def f fn [[x:Integer][Any][x add 100]] end]`
	// Since phase 2 the arm's redefinition is the arm's own shadow: the call
	// after the branch is the OUTER f on both paths, and the compiled lane
	// agrees or declines the shadow (the block gate).
	for _, src := range []string{
		mk + outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`,
		mk + outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`,
	} {
		ruleOrDecline(t, src, `[[[1] [1]]]`)
	}
}
