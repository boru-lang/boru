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

// TestNUR346LeafFrameArgsElision pins NUR346's close. A fn body that needs
// no frame state and never reads `args` (core bodyReferencesArgs, macros
// resolved when the fn is built) is a LEAF: the interpreter's handler pushes
// the shared EMPTY args list for it (buildFnBodyHandler), so code the frame
// runs that the construction-time walk cannot see — a computed body (`each
// (mk) […]` over a list holding `args`), a word macro bound after the fn —
// reads `[]`. The VM pushed the real args in every DynEnv frame, and a
// macro's `args` projected the params: `[[7] [7]]` for the interpreter's
// `[[] []]`, silent. The decision now rides the sig's frame identity
// (FnFrameMeta.ArgsElided) onto the unit (CompiledFn.ArgsElided), the VM's
// dispatches push the same empty list — a call, an apply, the token seam
// stepping the value — and an args-elided frame projects `[]`; the callback
// seam (InvokeCallback's CallBoru) and a call from another registry push
// the real list on both lanes.
func TestNUR346LeafFrameArgsElision(t *testing.T) {
	const mk = `def mk fn [[] [List] [quote [args]]] end `
	const w = `def w fn [[x:Integer] [Any] [each (mk) [x 5]]] end `
	for _, tc := range []struct{ src, want string }{
		// the register's repro, two calls, a list of calls
		{mk + w + `w 7`, `[[[] []]]`},
		{mk + w + `w 7 end w 8`, `[[[] []] [[] []]]`},
		{mk + w + `[(w 7) (w 8)]`, `[[[[] []] [[] []]]]`},
		// a tail call, an overloaded fn, a call from a fn that reads args
		{mk + `def w fn [[x:Integer][Any][if (x gt 3) [each (mk) [x 5]] [w (x add 1)]]] end w 1`, `[[[] []]]`},
		{mk + `def w fn [[x:Integer][Any][each (mk) [x 5]] [s:String][Any][each (mk) [s 5]]] end [(w 1) (w "a")]`, `[[[[] []] [[] []]]]`},
		{mk + w + `def v fn [[x:Integer][Any][args drop w x]] end v 3`, `[[[] []]]`},
		{mk + w + `def v fn [[y:Integer][Any][args drop [(each (mk) [y]) (w y)]]] end v 3`, `[[[[3]] [[] []]]]`},
		// an unnamed param, a branch body
		{mk + `def w fn [[Integer][Any][each (mk) [1 5]]] end w 3`, `[[[] []]]`},
		{mk + `def w fn [[x:Integer][Any][if true (mk) [0]]] end w 3`, `[[]]`},
		// a lambda: a named call, a paren apply, a member apply
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end lam 7`, `[[[] []]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end (lam/v 7)`, `[[[] []]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end def m {f:lam/v} end m.f 7`, `[[[] []]]`},
		{mk + `def lam ([x:Integer] => [each (mk) [x 5]]) end def m {f:lam/v} end each m.f [1 2]`, `[[[[] []] [[] []]]]`},
		// the token seam stepping a fn value, apply, a Function param
		{mk + w + `each w/v [1 2]`, `[[[[] []] [[] []]]]`},
		{mk + w + `(w/v 7)`, `[[[] []]]`},
		{mk + w + `w/v apply 7`, `[[[] []]]`},
		{mk + w + `def g fn [[f:Function] [Any] [f 7]] end g w/v`, `[[[] []]]`},
		// a word macro bound after the fn splices an `args` read
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end w 3`, `[[]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args] end [(w 3) (w 4)]`, `[[[] []]]`},
		{`def w fn [[x:Integer][Any][m]] end def m word [args.0] end w 3`, `[None]`},
		{`def w ([x:Integer] => [m]) end def m word [args] end w 3`, `[[]]`},
		{`def w fn [[Integer][Any][m]] end def m word [args] end w 3`, `[[]]`},
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
		{mk + `each ([kv:Any] => [each (mk) [1]]) {a:1 b:2}`, `[{a:[[{k:'a' v:1 i:0 n:2}]] b:[[{k:'b' v:2 i:1 n:2}]]}]`},
		{mk + `def w fn [[kv:Any][Any][each (mk) [1]]] end each w/v {a:1}`, `[{a:[[{k:'a' v:1 i:0 n:1}]]}]`},
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

// TestNUR346ArgsElisionOtherUnitCompiles pins the two remaining unit-compile
// paths that carry the sig's args-list decision (NUR346): a run-time
// dispatched overload arm (CALL_USER_POLY — compileUserPolyArm, call-site
// specialisation off so the generic arm table runs) and the outer overload
// a conditional redefinition replaces (check.CompileFnSigUnit, the routed
// dispatch of a speculative fn def).
func TestNUR346ArgsElisionOtherUnitCompiles(t *testing.T) {
	const mk = `def mk fn [[] [List] [quote [args]]] end `
	poly := mk + `def wrapfn fn [[m:Map] [Any] [def helper fn [[a:Integer] [Any] [each (mk) [a 5]] [b:String] [Any] [7]] helper (m get k/q)]] wrapfn {k:3}`
	if dis := compileDisasmNoSpec(t, poly); !strings.Contains(dis, "CALL_USER_POLY") {
		t.Errorf("expected a CALL_USER_POLY lowering:\n%s", dis)
	}
	gotC, compiled, errC, gotI, errI := runBothEnginesNoSpec(t, poly)
	if !compiled || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) || fmt.Sprint(gotI) != `[[[] []]]` {
		t.Errorf("%q: compiled %v [%v] (compiled=%v), interpreted %v [%v], want [[[] []]]", poly, gotC, errC, compiled, gotI, errI)
	}
	const outer = `def f fn [[x:Integer][Any][each (mk) [x 5]]] end `
	const arm = `[def f fn [[x:Integer][Any][x add 100]] end]`
	for _, tc := range []struct{ src, want string }{
		{mk + outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, `[[[] []]]`},
		{mk + outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, `[101]`},
	} {
		requireCompiledParity(t, tc.src)
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%q: interpreted %v [%v], want %s", tc.src, got, err, tc.want)
		}
	}
}
