package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR295FnElementInListLiteralReSteps pins NUR295. The interpreter
// EVALUATES a list literal, so an element that is a fn value where the tape
// steps it — a gradual member read, a word's fn result, a list element —
// applies over its neighbours: `[m.g 5]` over a factory's map is `[6]`, and
// the compiled assembly baked `[fn g(Integer) 5]` (silent, on main; a folded
// literal map's member was the one shape that answered). Such a list lowers
// to OpMakeListReStep: data elements assemble as before, and a fn element
// re-steps the window through the interpreter's island where every element
// after it is a literal; anything else is a designed defer.
func TestNUR295FnElementInListLiteralReSteps(t *testing.T) {
	g := `def g fn [[x:Integer] [Any] [x add 1]] end `
	mk := g + `def mk fn [[] [Map] [{g: g/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{g + `def m (flex {g: g/v}) end [m.g 5]`, "[[6]]"},
		{mk + `[m.g 5]`, "[[6]]"},
		{mk + `[7 m.g 5]`, "[[7 6]]"},
		{mk + `[m.g 5 7]`, "[[6 7]]"},
		{mk + `[5 m.g]`, "[[6]]"},
		{mk + `[(1 add 1) m.g 5]`, "[[2 6]]"},
		{g + `def mk fn [[] [Any] [{g: g/v}]] end def m (mk) end [m.g 5]`, "[[6]]"},
		{g + `def mk fn [[] [Map] [{g: g/v}]] end [(mk).g 5]`, "[[6]]"},
		{mk + `[m get "g" 5]`, "[[6]]"},
		{mk + `def l [m.g 5] end l`, "[[6]]"},
		{g + `def mk fn [[] [Map] [{g: g/v}]] end def f fn [[m:Map][Any][[m.g 5]]] end f (mk)`, "[[6]]"},
		{g + `def mk fn [[] [List] [[g/v]]] end def l (mk) end [l.0 5]`, "[[6]]"},
		// A loop's lists are their own: the island's results are copied out
		// of its pooled buffer, which the next iteration's run reuses.
		{g + `def m (flex {g: g/v}) end for 2 [[m.g i]]`, "[[1] [2]]"},
		{mk + `[1 2] each [drop [m.g 5]]`, "[[[6] [6]]]"},
		{`def mk fn [[] [List] [[([x:Integer] => [x add 1])]]] end def l (mk) end [l.0 5]`, "[[6]]"},
		// Data elements assemble as they did; a placed or quoted fn is data.
		{`def mk fn [[] [Map] [{n: "a" a: 3}]] end def m (mk) end [m.n m.a]`, "[['a' 3]]"},
		{`def mk fn [[] [Map] [{n: "a" a: 3}]] end def m (mk) end [m.n "x" 1]`, "[['a' 'x' 1]]"},
		{mk + `[(m.g) 5]`, "[[fn g(Integer) 5]]"},
		{mk + `[m.g/v 5]`, "[[fn g(Integer) 5]]"},
		{mk + `[m.g]`, "[[fn g(Integer)]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A fn element followed by a word's result: the interpreter's forward
	// collection stops at the word, so the island over the values is not
	// its evaluation — a designed defer, never the wrong list.
	for _, src := range []string{
		mk + `def five fn [[][Integer][5]] end [m.g five]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled && errC == nil && errI == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: silent, %v for %v", src, gotC, gotI)
		}
		if compiled && errC != nil && !strings.Contains(errC.Error(), "NUR295") && errI == nil {
			t.Errorf("%s: a defer names NUR295, got %v", src, errC)
		}
	}
}

// TestNUR219QuoteCapturesCollectedWord pins NUR219's closed forms. A `/q`
// slot captures the next WORD as an atom whatever it is bound to; the pass
// folds a value-bound word and the reserved `true` / `false` to a value, and
// the landing applied the fn over that value (`m.f true` raised
// uncalled_function for the interpreter's `[true]`). The landing notes the
// word (LandingWord.Collected); over a fn every signature of which quotes
// its first slot, its sealed claim enters the fn over the atom, and a list
// literal's re-step takes the word in its island. The forms with no sealed
// claim — values beneath, a wider residual, a paren, a fn body — defer.
func TestNUR219QuoteCapturesCollectedWord(t *testing.T) {
	pre := `def h fn [[x:Atom/q] [Any] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{pre + `m.f true`, "[true]"},
		{pre + `m.f false`, "[false]"},
		{pre + `def k 2 end m.f k`, "[k]"},
		{pre + `[m.f true]`, "[[true]]"},
		{pre + `[m.f true 5]`, "[[true 5]]"},
		{pre + `def k 2 end [m.f k]`, "[[k]]"},
		{`def mk2 fn [[] [List] [[([x:Atom/q] => [x])]]] end def l (mk2) end [l.0 true]`, "[[true]]"},
		{`def mk2 fn [[] [List] [[([x:Atom/q] => [x])]]] end def l (mk2) end l.0 true`, "[true]"},
		// A fn that collects the word's value is untouched.
		{`def b fn [[x:Boolean] [Any] [x not]] end def mk fn [[] [Map] [{b: b/v}]] end def m (mk) end m.b true`, "[false]"},
		{`def b fn [[x:Boolean] [Any] [x not]] end def mk fn [[] [Map] [{b: b/v}]] end def m (mk) end [m.b true]`, "[[false]]"},
		// No claim is sealed, and the statement's island runs the capture
		// (landing_restart.go): values beneath, a wider residual, a paren,
		// a def's paren, an earlier statement's constant, a fn body.
		{pre + `m.f true 5`, "[true 5]"},
		{pre + `7 m.f true`, "[7 true]"},
		{pre + `(m.f true)`, "[true]"},
		{pre + `def k (m.f true) end k`, "[true]"},
		{pre + `5 end 7 m.f true`, "[5 7 true]"},
		{pre + `def f fn [[][Any][m.f true]] end f`, "[true]"},
		{pre + `def f fn [[][Any][if true [(m.f true)] [0]]] end f`, "[true]"},
		// An earlier statement's result, which the root keeps in a slot, is
		// seated from there; a loop over a loop-invariant read restarts on its
		// first iteration; a code body's statement restarts in its frame.
		{pre + `(mk) end 7 m.f true`, "[{f:fn h(Atom)} 7 true]"},
		// A user call before the landing opens a paren whose value the
		// island writes in its place (restartSubsts): it does not run twice.
		{pre + `def g fn [[] [Any] [7]] end [(g) (m.f true)]`, "[[7 true]]"},
		// A user call whose value the statement consumed before the landing
		// keeps a copy in a slot of its own (NUR296's stash), which the
		// island writes in its paren's place.
		{pre + `def g fn [[] [Any] [7]] end [(g) drop (m.f true)]`, "[[true]]"},
		// An effect before the landing is written as nothing (NUR296's
		// call run): it does not run twice.
		{pre + `print "x" [(m.f true)]`, "[[true]]"},
		{pre + `"x" print/s [(m.f true)]`, "[[true]]"},
		{pre + `for 2 [[(m.f true)]]`, "[[true] [true]]"},
		{pre + `[1 2] each [drop [(m.f true)]]`, "[[[true] [true]]]"},
		// The caret sibling: a named fn no window fits raises at its own
		// token on both lanes (`h/v` in the factory's map).
		{`def h fn [[x:Atom/q y:Integer] [Any] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end m.f true`,
			"ERROR:call to 'h' matched no signature"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// An effect before the landing whose operand lies beneath its statement
	// is no call run an island can write: it would run twice, so that
	// capture stays a designed defer.
	src := pre + `"x" end print/s [(m.f true)]`
	gotC, compiled, errC, _, errI := runBothEngines(t, src)
	if !compiled || errI != nil || errC == nil || !strings.Contains(errC.Error(), "NUR219") || len(gotC) != 0 {
		t.Errorf("%s: an unsealed capture defers compiled; got %v %v (interpreter %v)", src, gotC, errC, errI)
	}
}
