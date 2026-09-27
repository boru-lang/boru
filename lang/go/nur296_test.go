package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestNUR296LoopIslands pins the statement island inside loops (NUR296). A
// stop inside a counted loop restarts the loop's statement on the
// interpreter when the stop fires on the loop's FIRST iteration — the island
// runs the loop from its start, so no earlier iteration may have run — which
// the VM checks at run time (compiler.RestartFirst): the index slot still
// holds the loop's start. A read of the loop's index and a bind written after
// the stop no longer block it, and a branch guard over a list takes the same
// island. A paren's value the statement consumed before the stop is kept in
// a slot of its own when the call leaves it (the stash), and an effect before
// the stop is written as its run (TestNUR296EffectBeforeTheStop). A stop on a
// later iteration, an effect that took an operand off the stack and a paren
// substituted inside the loop stay designed defers.
func TestNUR296LoopIslands(t *testing.T) {
	l := `def h fn [[x:Atom/q] [Any] [x]] end def mk fn [[] [List] [[h/v h/v]]] end def l (mk) end `
	for _, c := range []struct{ src, want string }{
		{l + `for 2 [[(l.(i) true)]]`, "[[true] [true]]"},
		{l + `for 2 [[(l.0 true)] def q 1 end]`, "[[true] [true]]"},
		{`def l [(quote [gt 3]) true] end for 2 [5 if l.(i) ["t"] ["f"] drop]`, "[5]"},
		{`def mk fn [[][Any][quote [gt 3]]] end def c (mk) end for 2 [5 if c ["t"] ["f"] drop]`, "[]"},
		{l + `def g fn [[] [Any] [7]] end [(g) drop (l.0 true)]`, "[[true]]"},
		{l + `print "x" [(l.0 true)]`, "[[true]]"},
		{l + `"x" print/s [(l.0 true)]`, "[[true]]"},
		{`def mk fn [[][Any][quote [gt 3]]] end def g fn [[] [Any] [7]] end 5 (g) drop if (mk) ["big"] ["small"]`, "[big]"},
		{`def mk fn [[][Any][quote [gt 3]]] end def c fn [[][Any][true]] end 5 if (c) (mk) ["f"]`, "ERROR:cannot call `gt`"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	mixed := `def h fn [[x:Atom/q] [Any] [x]] end def g2 fn [[x:Any] [Any] [x]] end def mk fn [[] [List] [[g2/v h/v]]] end def l (mk) end `
	for _, c := range []struct{ src, sub string }{
		{mixed + `for 2 [[(l.(i) true)]]`, "first iteration"},
		{l + `def n (flex [0]) end for 2 [push i n [(l.(i) true)]]`, "NUR219"},
		{l + `"x" end print/s [(l.0 true)]`, "NUR219"},
		{`def l [true (quote [gt 3])] end for 2 [5 if l.(i) ["t"] ["f"] drop]`, "NUR292"},
		{`def mk fn [[][Any][quote [gt 3]]] end def n 0 end for 2 [5 if (mk) ["t"] ["f"] drop]`, "NUR292"},
	} {
		gotC, compiled, errC, _, errI := runBothEngines(t, c.src)
		if !compiled || errI != nil || errC == nil || !strings.Contains(errC.Error(), c.sub) || len(gotC) != 0 {
			t.Errorf("%s: a statement no island can run again defers compiled; got %v %v (interpreter %v)", c.src, gotC, errC, errI)
		}
	}
}

// TestNUR296EffectBeforeTheStop pins NUR296's effect form. A bare call
// before the stop in its statement whose arguments were written beside its
// word — after it, or right before it for those it took off the stack
// (`"x" print/s`) — is written in the island as the run it left: an effect
// as nothing (`print "a"`, RestartNone), a value as that value. So the
// island never runs it again, and the program prints what the interpreter
// prints, once. A call whose operand lies beneath its statement is no such
// run and still defers (TestNUR292ComputedIfConditionOrArmThatIsAList).
func TestNUR296EffectBeforeTheStop(t *testing.T) {
	l := `def h fn [[x:Atom/q] [Any] [x]] end def mk fn [[] [List] [[h/v h/v]]] end def l (mk) end `
	gt := `def mk fn [[][Any][quote [gt 3]]] end `
	for _, c := range []struct{ src, want, out string }{
		{gt + `print "a" 5 if (mk) ["big"] ["small"]`, "[big]", "a\n"},
		{gt + `print "a" print "b" 5 if (mk) ["big"] ["small"]`, "[big]", "a\nb\n"},
		{gt + `def x "q" end print x 5 if (mk) ["big"] ["small"]`, "[big]", "q\n"},
		{gt + `def g fn [[][Any][7]] end print (g) 5 if (mk) ["big"] ["small"]`, "[big]", "7\n"},
		{gt + `5 print "a" if (mk) ["big"] ["small"]`, "[big]", "a\n"},
		{l + `print "a" [(l.0 true)] print "z"`, "[[true]]", "a\nz\n"},
		{l + `(print "a") [(l.0 true)]`, "[[true]]", "a\n"},
		{l + `def n (flex []) end push 1 n [(l.0 true)] n`, "[[1] [true] [1]]", ""},
		{l + `def f fn [[][Any][print "a" [(l.0 true)]]] end f`, "[[true]]", "a\n"},
		{`def f fn [[][Any][print "a" 1 do [(1 add 1) drop] drop 7]] end f`, "[7]", "a\n"},
		{gt + `"a" print/s 5 if (mk) ["big"] ["small"]`, "[big]", "a\n"},
		{gt + `3 4 add print/s 5 if (mk) ["big"] ["small"]`, "[big]", "7\n"},
		{gt + `10 sub 3 print/s 5 if (mk) ["big"] ["small"]`, "[big]", "7\n"},
		{gt + `def g fn [[][Any][7]] end (g) print/s 5 if (mk) ["big"] ["small"]`, "[big]", "7\n"},
		{l + `"a" print/s [(l.0 true)]`, "[[true]]", "a\n"},
	} {
		var oc, oi bytes.Buffer
		a := mustNew(t)
		a.SetOutput(&oc)
		gotC, compiled, errC := a.RunCompiled(c.src)
		b := mustNew(t)
		b.SetOutput(&oi)
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: want %s on both lanes, compiled; got compiled=%v %v %v / interpreter %v %v", c.src, c.want, compiled, gotC, errC, gotI, errI)
		}
		if oc.String() != c.out || oi.String() != c.out {
			t.Errorf("%s: prints %q once on both lanes; got compiled %q, interpreter %q", c.src, c.out, oc.String(), oi.String())
		}
	}
	// A call run that returned a fn value: the interpreter parks it, and the
	// island would step it and apply it to the list after it, so it defers
	// (RestartSubst.Placed) — the first cut answered a type_error here.
	src := l + `def mk2 fn [[n:Integer][Any][([x:List] => [x n])]] end mk2 5 [(l.0 true)]`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[fn (List) [true]]" || !compiled || codeOf(errC) != "internal_error" || !strings.Contains(errC.Error(), "NUR297") {
		t.Errorf("%s: a placed fn value defers; got compiled=%v %v %v / interpreter %v %v", src, compiled, gotC, errC, gotI, errI)
	}
	agreeOnBothLanes(t, l+`def mk2 fn [[n:Integer][Any][n]] end mk2 5 [(l.0 true)]`, "[5 [true]]")
}
