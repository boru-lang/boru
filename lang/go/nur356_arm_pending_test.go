package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// nur356_arm_pending_test.go pins NUR356: a list or map literal an `if` arm
// ends with stays PENDING on the interpreter's tape past the `if` (the arm
// is spliced as a paren group, which evaluates nothing it leaves) until a
// word takes it or the enclosing run ends, where the compiled lane — whose
// check pass ran the arm in a sub-engine that swept it — assembled it at the
// arm. And its twin: a literal a call matched at RUN time takes is evaluated
// by the interpreter only once a signature takes it, where the compiled lane
// assembled it before the call.

// TestNUR356ArmPendingLiteralDeclines: NEGATIVE — something runs between the
// arm and the interpreter's evaluation of its pending literal that could
// tell (a rebinding of a name it reads, an effect, a slot that takes it raw,
// a run-time match that may refuse it): the program declines with the
// NUR356 reason, and the interpreter's answer stands. Each used to compile
// to the arm's eager answer, silently: `if c [[x]] [0] end def x 2 end` was
// `[[1]]` for the interpreter's `[[2]]`.
func TestNUR356ArmPendingLiteralDeclines(t *testing.T) {
	const x = `def x 1 end def c true end `
	for _, tc := range []struct{ src, want string }{
		{x + `if c [[x]] [0] end def x 2 end`, "[[2]]"},
		{x + `if c [[x]] [0] def x 2 end`, "[[2]]"},
		{x + `if c [[x]] [0] end def x 2 end 5`, "[[2] 5]"},
		{x + `if c [[x 1]] [[0]] end def x 2 end`, "[[2 1]]"},
		{x + `if c [{a:x}] [0] end def x 2 end`, "[{a:2}]"},
		{x + `if c [[x] 5] [0 0] end def x 2 end`, "[[2] 5]"},
		{`def x 1 end def c false end if c [0] [[x]] end def x 2 end`, "[[2]]"},
		// The else-less, list-condition, clause-list and literal-condition forms.
		{x + `if c [[x]] end def x 2 end`, "[[2]]"},
		{x + `if [c] [[x]] [0] end def x 2 end`, "[[2]]"},
		{x + `if [c [[x]] [0]] end def x 2 end`, "[[2]]"},
		// A nested `if`'s literal is its enclosing arm's, and a paren
		// evaluates nothing it leaves.
		{x + `if c [if c [[x]] [0]] [0] end def x 2 end`, "[[2]]"},
		{x + `(if c [[x]] [0]) end def x 2 end`, "[[2]]"},
		{x + `[(if c [[x]] [0]) (def x 2 end 3)]`, "[[[2] 3]]"},
		{x + `def w word [if c [[x]] [0]] end w end def x 2 end`, "[[2]]"},
		{x + `if c [[x]] [0] end undef x`, "ERROR:undefined word: x"},
		// A computed-arm value the pass holds as a literal arm.
		{x + `if c (quote [[x]]) [0] end def x 2 end`, "[[2]]"},
		// An effect between: the interpreter prints a, then q.
		{`def c true end if c [[print "q" 2]] [3] end print "a"`, "[[2]]"},
		{`def c true end def f fn [[] [Any] [if c [[print "q" 2]] [3] end print "a"]] end f`, "[[2]]"},
		{`def c true end for 2 [if c [[print "q" 2]] [3] end print "a"]`, "[[2] [2]]"},
		// A code-body slot takes the literal raw: each runs it per element.
		{`def c true end [3 4] each (if c [[print "q" 1]] [[5]])`, "[[1 1]]"},
		// A call whose param contract refuses it at run time reports the
		// literal as written.
		{x + `def f fn [[a:Integer] [Any] [a]] end f (if c [[x]] [0])`, "ERROR:cannot call `f`"},
	} {
		declinesWithInterpAnswer(t, tc.src, "(NUR356)", tc.want)
	}
}

// TestNUR356ArmPendingLiteralCompiles: POSITIVE — where nothing between the
// arm and the interpreter's evaluation can tell, the arm's eager assembly is
// the interpreter's answer and the program compiles: the run's end takes
// the literal at once (the program's, a fn frame's, a loop iteration's, an
// enclosing literal's element run), a committed word or a def takes it, a
// literal of scalars evaluates to itself, a `case` block ends where it is
// evaluated, and a lambda's parameter stays bound while its arm's literal
// is evaluated.
func TestNUR356ArmPendingLiteralCompiles(t *testing.T) {
	const x = `def x 1 end def c true end `
	for _, tc := range []struct{ src, want string }{
		{x + `if c [[x]] [0]`, "[[1]]"},
		{x + `if c [[x]] [0] end`, "[[1]]"},
		{x + `[if c [[x]] [0]] end def x 2 end`, "[[[2]]]"},
		{x + `def y (if c [[x]] [0]) end def x 2 end y`, "[[1]]"},
		{x + `if c [[1 2]] [0] end def x 2 end`, "[[1 2]]"},
		{x + `def f fn [[a:List] [Any] [a]] end f (if c [[x]] [[0]])`, "[[1]]"},
		{`def x 1 end def f fn [[c:Boolean] [Any] [if c [[x]] [0]]] end f true end def x 2 end`, "[[1]]"},
		{`def f fn [[c:Boolean x:Integer] [Any] [if c [[x]] [0]]] end f true 5`, "[[5]]"},
		{x + `for 2 [if c [[x]] [0]] end def x 2 end`, "[[1] [1]]"},
		{x + `do [if c [[x]] [0]] end def x 2 end`, "[[1]]"},
		{`def x 1 end def c 1 end case c [1 [[x]] 2 [3]] end def x 2 end`, "[1 [1]]"},
		{`def c true end if c [[print "q" 2]] [3] end`, "[[2]]"},
		// A lambda's parameter is the body's own frame binding, live while the
		// arm's literal is evaluated (the var construct's teardown unbound it
		// first, so this shape used to fail the interpreter's read).
		{`def cid "a" end each ([e] => [if (e eq cid) [[e]] [[]]]) ["a" "b"]`, "[[['a'] []]]"},
		{`def cid "a" end each ([e] => [def efrom (e get "from") def eto (e get "to") if (efrom eq cid) [[eto]] [if (eto eq cid) [[efrom]] [[]]]]) [{from:"a" to:"b"} {from:"c" to:"a"} {from:"x" to:"y"}]`, "[[['b'] ['c'] []]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// bothLanesOutput is one program's run on both lanes, with what each printed.
type bothLanesOutput struct {
	gotC, gotI []any
	compiled   bool
	errC, errI error
	outC, outI string
}

// runBothWithOutput runs src on both lanes, capturing what each prints.
func runBothWithOutput(t *testing.T, src string) bothLanesOutput {
	t.Helper()
	var r bothLanesOutput
	var oc, oi bytes.Buffer
	b := mustNew(t)
	b.SetOutput(&oc)
	r.gotC, r.compiled, r.errC = b.RunCompiled(src)
	d := mustNew(t)
	d.SetOutput(&oi)
	r.gotI, r.errI = d.RunInterp(src)
	r.outC, r.outI = oc.String(), oi.String()
	return r
}

// TestNUR356EffectOrderAgrees: the arm's effect runs where the interpreter's
// evaluation runs it, on both lanes — at the end of the program or of the
// fn frame that ends right after the arm.
func TestNUR356EffectOrderAgrees(t *testing.T) {
	for _, tc := range []struct{ src, out string }{
		{`def c true end if c [[print "q" 2]] [3] end`, "q\n"},
		{`def c true end def f fn [[] [Any] [if c [[print "q" 2]] [3]]] end f end print "a"`, "q\na\n"},
	} {
		r := runBothWithOutput(t, tc.src)
		if !r.compiled || fmt.Sprint(r.gotC, r.errC) != fmt.Sprint(r.gotI, r.errI) || r.outC != r.outI || r.outI != tc.out {
			t.Errorf("%s: compiled=%v %v %v %q, interpreted %v %v %q, want output %q", tc.src, r.compiled, r.gotC, r.errC, r.outC, r.gotI, r.errI, r.outI, tc.out)
		}
	}
}

// TestNUR356EagerLiteralDeclines: NEGATIVE — a call matched at run time (a
// poly dispatch, a user call whose contract may refuse) over a literal whose
// assembly may have an effect: the interpreter evaluates the literal only
// once a signature takes it, so the program declines. `each (h) [print "p"
// 1]` over an h answering 3 used to print p compiled.
func TestNUR356EagerLiteralDeclines(t *testing.T) {
	const h = `def h fn [[] [Any] [3]] end `
	for _, tc := range []struct{ src, want string }{
		{h + `each (h) [print "p" 1]`, "ERROR:cannot call `each`"},
		{h + `[print "p" 1] each (h)`, "ERROR:cannot call `each`"},
		{h + `each (h) {a:(print "p" 1)}`, "ERROR:cannot call `each`"},
		{h + `each (h) [[print "p" 1] 2]`, "ERROR:cannot call `each`"},
		{h + `each (h) [(print "p" 1)]`, "ERROR:cannot call `each`"},
		{h + `def g fn [[] [Any] [each (h) [print "p" 1]]] end g`, "ERROR:cannot call `each`"},
		// The run matches: the interpreter evaluates it then, once.
		{`def h fn [[] [Any] [[7 8]]] end each (h) [print "p" 1]`, "[[8]]"},
		{`def f fn [[a:List b:Integer] [Any] [a]] end def h fn [[] [Any] ["s"]] end f [print "p" 1] (h)`, "ERROR:cannot call `f`"},
	} {
		declinesWithInterpAnswer(t, tc.src, "(NUR356)", tc.want)
	}
}

// TestNUR356EagerLiteralCompiles: POSITIVE — a quiet literal under a poly
// (pure reads, a value word over constants) assembles to the same value
// whenever it runs, and the poly's no-match report renders it as written
// (NUR352); a committed call takes any literal as the interpreter does.
func TestNUR356EagerLiteralCompiles(t *testing.T) {
	const h = `def h fn [[] [Any] [3]] end `
	for _, tc := range []struct{ src, want string }{
		{h + `each (h) [1 add 2]`, "ERROR:[1 add 2]"},
		{h + `each (h) [1 2]`, "ERROR:[1 2] (a List)"},
		{h + `each (h) {a:1}`, "ERROR:{a:1} (a Map)"},
		{`def x 4 end ` + h + `[x add 1] each (h)`, "ERROR:[x add 1]"},
		{h + `size (h) [print "p" 1]`, "[3 [1]]"},
		{`def f fn [[a:List] [Any] [a]] end def h fn [[] [Any] [[1]]] end f (h) [print "p" 1]`, "[[1] [1]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR356ComputedArmDefers: a COMPUTED arm (a runtime list the `if`
// splices, run by the compiled `__arm`) holding a literal the interpreter
// would leave pending past the `if` is a designed defer — loud — where the
// compiled run would evaluate it at the arm's end; one of scalars runs.
func TestNUR356ComputedArmDefers(t *testing.T) {
	const mk = `def x 1 end def mk fn [[] [List] [quote [[x]]]] end def c true end `
	for _, src := range []string{mk + `if c (mk) [0] end def x 2 end`, mk + `if c (mk) [0]`} {
		_, compiled, errC := mustNew(t).RunCompiled(src)
		if !compiled || errC == nil || !strings.Contains(errC.Error(), "NUR356") {
			t.Errorf("%s: want the NUR356 defer, got compiled=%v %v", src, compiled, errC)
		}
	}
	if got, err := mustNew(t).RunInterp(mk + `if c (mk) [0] end def x 2 end`); err != nil || fmt.Sprint(got) != "[[2]]" {
		t.Errorf("interpreted %v %v", got, err)
	}
	agreeOnBothLanes(t, `def mk fn [[] [List] [quote [1 add 2]]] end def c true end if c (mk) [0]`, "[3]")
}
