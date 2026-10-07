package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The NUR361 fixture: g answers 5; mk hands g back as a GRADUAL value (its
// declared result is Any), so a bare read of a def bound to (mk) is the
// interpreter's word dispatch of g — 5 — where a plain slot push answers the
// fn value itself. mkd is its data twin.
const (
	nur361Fn   = `def g fn [[] [Integer] [5]] end def mk fn [[] [Any] [g/v]] end `
	nur361Data = `def mk fn [[] [Any] [5]] end `
)

// requireAgreeOrGuarded asserts the compiled lane of src compiles and either
// agrees with the interpreter or raises the read-site guard's designed defer
// (the NUR123 message, loud): never another answer.
func requireAgreeOrGuarded(t *testing.T, src string) {
	t.Helper()
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if !compiled {
		t.Errorf("%q: must compile; err=%v", src, errC)
		return
	}
	if fmt.Sprint(gotC) == fmt.Sprint(gotI) && fmt.Sprint(errC) == fmt.Sprint(errI) {
		return
	}
	if errC == nil || !isBailDefect(errC) || !strings.Contains(errC.Error(), "holds a fn the interpreter dispatches here") {
		t.Errorf("%q:\n  compiled %v [%v]\n  interp   %v [%v]\n  want the interpreter's answer or the read's guard", src, gotC, errC, gotI, errI)
	}
}

// TestNUR361NestedGradualReadGuarded pins NUR361: a gradual read nested in a
// body the unit or the root runs inline — a `var` body inside an `each`
// callback, an arm, a loop body — had no deopt point (no statement of its
// own) or one whose test ran before the value's producer, and lowered as a
// plain push: `[[fn v]]` for the interpreter's `[[5]]`. Each read is now
// guarded where it happens: loud when it holds a fn.
func TestNUR361NestedGradualReadGuarded(t *testing.T) {
	for _, body := range []string{
		`each [var [[q] def v (mk) v]] [1]`,
		`each [var [[q] def v (mk) [v]]] [1]`,
		`each [var [[q] def v (mk) if true [[v]] [[]]]] [1]`,
		`each [def v (mk) v] [1]`,
		`each [var [[q] def v (mk) if true [v] [0]]] [1]`,
		`def h fn [[q:Any] [Any] [def v (mk) if q [v] [0] drop def w (mk) if q [w] [0]]] end h true`,
		`0 fold [var [[q] def v (mk) v]] [1]`,
		`0 fold [def v (mk) [v]] [1]`,
		`def h fn [[] [Any] [each [var [[q] def v (mk) v]] [1]]] end h`,
		`def h fn [[q:Any] [Any] [def v (mk) [v]]] end h 1`,
		`def h fn [[q:Any] [Any] [def v (mk) v add 1]] end h 1`,
		`def h fn [[q:Any] [Any] [if true [def v (mk) [v]] [0]]] end h 1`,
		`def h fn [[q:Any] [Any] [if true [def v (mk) v] [0]]] end h 1`,
		`def h fn [[q:Any] [Any] [for 1 [def v (mk) [v]]]] end h 1`,
		`def v (mk) [v]`,
		`if true [def v (mk) [v]] [0]`,
		`for 1 [def v (mk) [v]]`,
	} {
		src := nur361Fn + body
		requireAgreeOrGuarded(t, src)
		// The data twin runs the guard's test alone and agrees.
		requireCompiledParity(t, nur361Data+body)
	}
}

// TestNUR361GuardOnlyOnTheReadsPath pins that the guard stands where the
// read happens: a dead or untaken arm holding the read never raises, and a
// read whose consumer dispatches the fn itself (a shaped method call, the
// program residual's island) keeps its right answer.
func TestNUR361GuardOnlyOnTheReadsPath(t *testing.T) {
	for _, src := range []string{
		nur361Fn + `if false [def v (mk) [v]] [0]`,
		nur361Fn + `def h fn [[q:Any] [Any] [if q [def v (mk) [v]] [0]]] end h false`,
		`def mk fn [[][Any][([] => [42])]] end def j (mk) end j`,
		`def mk fn [[][Any][([] => [42])]] end def j (mk) end 5 j`,
		`def h fn [[m:Map][Any][def j (m get "f")  5  (j typeof)  drop]]  h {f: ([] => [42])}`,
		nur361Data + `def h fn [[q:Any] [Any] [if q [def v (mk) [v]] [0]]] end h true`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR361WordCollectedListIsland pins the island a read inside a list
// token takes when the word BEFORE the token collects it — its dispatch
// (`print [v]`, `size [v]`) or a body run inline (`def v (mk) [v]`): the
// statement begins at the word. The island began at the token and lost the
// word — `[1]` compiled `[[5]]`, and the inline `[v]` answered the def's own
// argument list `[[] [5]]`.
func TestNUR361WordCollectedListIsland(t *testing.T) {
	for _, body := range []string{
		`def v (mk) end size [v]`,
		`def v (mk) end print [v]`,
		`def v (mk) end [v]`,
		`def v (mk) end [v] 1`,
		`def v (mk) end [v] 3`,
		`def h fn [[q:Any][Any][def v (mk) size [v]]] end h 0`,
		`def h fn [[q:Any][Any][def v (mk) print [v] 1]] end h 0`,
		`def h fn [[q:Any][Any][def v (mk) [v]]] end h 0`,
	} {
		requireCompiledParity(t, nur361Fn+body)
		requireCompiledParity(t, nur361Data+body)
	}
}

// TestNUR359PlainRunReStepsItsFn pins NUR359: a computed `do` body seated as
// plain data whose run leaves a fn value the interpreter's step loop
// dispatches where the `do` stood. A fn that takes no argument fires there
// over nothing, whatever lies beneath or after the run, so the VM re-steps it
// in place — its results, or the break / continue it raises for the
// enclosing loop to take — where the plain seat deferred (vm:dyn-body-plain).
func TestNUR359PlainRunReStepsItsFn(t *testing.T) {
	const mk = `def mk fn [[][List][quote [f/v]]] end `
	for _, src := range []string{
		`def f fn [[] [Any] [break]] end ` + mk + `for 2 [do (mk)] 7`,
		`def f fn [[] [Any] [continue]] end ` + mk + `for 2 [do (mk)] 7`,
		`def f fn [[] [Any] [8]] end ` + mk + `for 2 [do (mk)] 7`,
		`def f fn [[] [Any] [break]] end ` + mk + `for 2 [for 3 [do (mk)] 5] 7`,
		`def f fn [[] [Any] [break]] end ` + mk + `def h fn [[] [Any] [for 2 [do (mk)] 7]] end h`,
		`def f fn [[] [Any] [break]] end ` + mk + `for 2 [if true [do (mk)] [0]] 7`,
		`def f fn [[] [Any] [8]] end ` + mk + `do (mk) 7`,
		`def f fn [[] [Any] [8]] end def mk fn [[][List][quote [4 f/v]]] end for 2 [do (mk)] 7`,
		`def f fn [[] [Any] [1 2]] end ` + mk + `for 2 [do (mk)] 7`,
		// at the program root and at a unit's tail, where a count island
		// or the frame's own check took the run before
		`def f fn [[] [Any] [break]] end ` + mk + `do (mk) 7`,
		`def f fn [[] [Any] [8]] end ` + mk + `def h fn [[][Any][do (mk)]] end h`,
		`def f fn [[] [Any] [8]] end ` + mk + `def h fn [[][Any][do (mk)]] end [(h)]`,
		`def f fn [[] [Any] [break]] end ` + mk + `[1 2] each [do (mk)]`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR359ArgTakingFnStaysLoud pins the remainder: a run's fn that takes an
// argument collects from the values around the `do`, which the in-place
// re-step does not hold, so it keeps the plain seat's loud defer.
func TestNUR359ArgTakingFnStaysLoud(t *testing.T) {
	for _, src := range []string{
		`def f fn [[x:Integer] [Any] [x]] end def mk fn [[][List][quote [3 f/v]]] end for 2 [do (mk)] 7`,
		`def f fn [[x:Integer] [Any] [x]] end def mk fn [[][List][quote [3 f/v]]] end def h fn [[][Any][do (mk)]] end h`,
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if !compiled {
			t.Fatalf("%q: must compile; err=%v", src, errC)
		}
		if fmt.Sprint(gotC) == fmt.Sprint(gotI) && fmt.Sprint(errC) == fmt.Sprint(errI) {
			continue
		}
		if errC == nil || !isBailDefect(errC) || !strings.Contains(errC.Error(), "re-steps") {
			t.Errorf("%q: compiled %v [%v], interp %v [%v]: want parity or the plain seat's defer", src, gotC, errC, gotI, errI)
		}
	}
}

// TestNUR360TailCallAnchors pins NUR360: a TAIL_CALL_USER replaces the
// caller's frame, and the replaced frame's RET anchored the tail-called
// unit's contract error at the call that entered the frame (`(h)`), where
// the interpreter anchors it at the tail call (`f` inside h). The recovered
// user call's CALL_USER took the callee body's stale cursor (`keys` in k)
// for its position. A tail call at an activation's root (a callback body)
// lost the callee's count contract altogether: `[1] each [drop h]` answered
// `[[2]]` for the interpreter's count error.
func TestNUR360TailCallAnchors(t *testing.T) {
	for _, src := range []string{
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end (h)`,
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end h`,
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end def h2 fn [[][Any][h]] end h2`,
		`def f fn [[][Integer]["s"]] end def h fn [[][Any][f]] end h`,
		`def f fn [[x:Integer][Any][1 2]] end def h fn [[][Any][f 3]] end h`,
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end [(h)]`,
		`def f fn [[][Any][1 2]] end def h fn [[n:Integer][Any][if (n gt 0) [f] [0]]] end h 1`,
		`def cd fn [[n:Integer][Any][if (n lte 0) [1 2] [cd (n sub 1)]]] end cd 3`,
		`def f fn [[][Any][1 2]] end def mk fn [[][Function][([] => [f])]] end def q (mk) end q`,
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end [1] each [drop h]`,
		`def f fn [[][Any][]] end def h fn [[][Any][f]] end [1] each [drop h]`,
		`def f fn [[][Any][1 2]] end def h fn [[][Any][f]] end 0 fold [drop drop h] [1]`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Map][Any][k m.a]] end g {a:0}`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Map][Any][k m.a]] end (g {a:0})`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Map][Any][(k m.a)]] end g {a:0}`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Any][Any][k m]] end g 0`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Map][Any][k m.a]] end [{a:0}] each [g]`,
		// the passing twins: no contract error, the tail calls answer
		`def f fn [[][Any][5]] end def h fn [[][Any][f]] end [1] each [drop h]`,
		`def k fn [[m:Map][List][keys m]] end def g fn [[m:Map][Any][k m.a]] end g {a:{b:1}}`,
	} {
		requireCompiledParity(t, src)
	}
}
