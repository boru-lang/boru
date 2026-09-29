package lang

import (
	"testing"
)

// nur351_352_test.go pins NUR351 (a forward collection the pass decided
// over a live read's stale type after a computed keep-defs body) and the
// four NUR352 neighbours of the union-branch rematch and the member apply.
// Positive rows answer on both lanes, compiled; negative rows pin the
// shapes that decline soundly, where the interpreter's answer stands.

const nur351Mk = `def x 0 end def mk fn [[][List][quote [`

// TestNUR351LiveReadAfterCollector: a word written before a read seated live
// after a computed body may collect it forward — the pass decided that over
// the binding's pre-body type (Integer), the interpreter decides it over the
// value the body bound. The read's live point is tested before that word's
// dispatch, so a binding of another type takes the statement's island and
// one of the model's type runs as compiled.
func TestNUR351LiveReadAfterCollector(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end keys x`, "[4 ['a']]"},
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end keys x end`, "[4 ['a']]"},
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end keys x end 9`, "[4 ['a'] 9]"},
		{nur351Mk + `def x {a:1} 4 5]]] end do (mk) end keys x`, "[4 5 ['a']]"},
		{nur351Mk + `def x {a:1}]]] end do (mk) end keys x`, "[['a']]"},
		{nur351Mk + `def x {a:1} {b:1}]]] end do (mk) end keys x`, "[{b:1} ['a']]"},
		{nur351Mk + `def x {a:1}]]] end do (mk) end {b:1} keys x`, "[{b:1} ['a']]"},
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end print "p" keys x`, "[4 ['a']]"},
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end x keys`, "[4 ['a']]"},
		{nur351Mk + `def x {a:1} 4]]] end do (mk) end size x`, "[4 1]"},
		{nur351Mk + `def x "s" 4]]] end do (mk) end add x 1`, "[4 1s]"},
		{`def x 0 end def y 0 end def mk fn [[][List][quote [def x 1 def y "a"]]] end do (mk) end x add y`, "[1a]"},
		{nur351Mk + `def x {a:1}]]] end do (mk) end [{b:1} keys x]`, "[[{b:1} ['a']]]"},
		{nur351Mk + `def x {a:1}]]] end do (mk) end print "p" ({b:1} keys x)`, "[{b:1} ['a']]"},
		{`def x {c:3} end def mk fn [[][List][quote [def x {a:1} {b:1}]]] end do (mk) end keys x`, "[{b:1} ['a']]"},
		// The binding keeps the model's type: the compiled statement runs.
		{nur351Mk + `def x 5 {b:1}]]] end do (mk) end keys x`, "[['b'] 5]"},
		{`def x {c:3} end def mk fn [[][List][quote [def x 5 {b:1}]]] end do (mk) end keys x`, "[['b'] 5]"},
		{nur351Mk + `def x 2 4]]] end do (mk) end add x 1`, "[4 3]"},
		// NEGATIVE: the run matches nothing on both lanes.
		{nur351Mk + `4]]] end do (mk) end keys x`, "ERROR:cannot call `keys`"},
		{nur351Mk + `def x 7 4]]] end do (mk) end keys x`, "ERROR:cannot call `keys`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	prog, reason, _, err := mustNew(t).CompileCheck(nur351Mk + `def x {a:1} 4]]] end do (mk) end keys x`)
	if prog == nil || err != nil {
		t.Fatalf("must compile: %q %v", reason, err)
	}
	live := 0
	for _, d := range prog.Deopts {
		if d.Live {
			live++
		}
	}
	if live != 1 {
		t.Errorf("want the read's live point before keys: %+v", prog.Deopts)
	}
}

// TestNUR351UnservedReadDeclines: NEGATIVE — where no live point can be
// tested before the word that may collect the read, the program declines
// rather than run the pass's collection over the stale type (`{b:1} keys x`
// once answered [['b'] {a:1}] compiled): a statement opening with the
// computed body's own `do`, whose test would run before the body, and a fn
// unit whose statement operand the unit's rules cannot hold. A terminal
// trap the pass met before the read is no proof either (the pass never
// made the read): it is not recorded, and the program declines.
func TestNUR351UnservedReadDeclines(t *testing.T) {
	for _, tc := range []struct{ src, why, want string }{
		{nur351Mk + `def x {a:1} 4]]] end do (mk) keys x`, "(NUR351)", "[4 ['a']]"},
		{nur351Mk + `def x {a:1}]]] end do (mk) keys x`, "(NUR351)", "[['a']]"},
		{`def f fn [[b:List][Any][def x 0 do b end keys x]] end [f (quote [def x {a:1}])]`, "(NUR351)", "[[['a']]]"},
		{`def f fn [[b:List][Any][def x 0 do b keys x]] end f (quote [def x {a:1}])`, "(NUR351)", "[['a']]"},
		{nur351Mk + `def x {a:1} 9]]] end do (mk) end (4 keys x)`, "unmatched dispatch recovered at keys", "[9 4 ['a']]"},
		{nur351Mk + `def x {a:1} 9]]] end do (mk) end [4 keys x]`, "unmatched dispatch recovered at keys", "[9 [4 ['a']]]"},
	} {
		declinesWithInterpAnswer(t, tc.src, tc.why, tc.want)
	}
	// The failed `keys` over the body's gradual run beneath the 4 renders
	// that run's value (withRenderedPrefix, NUR351's note window): the
	// runtime rematch takes it, where the static trap declined.
	for _, tc := range []struct{ src, want string }{
		{nur351Mk + `def x {a:1}]]] end do (mk) end 4 keys x`, "[4 ['a']]"},
		{nur351Mk + `def x 5]]] end do (mk) end 4 keys x`, "ERROR:cannot call `keys`"},
		{`def mk fn [[][List][quote [def y {a:1}]]] end do (mk) end 4 keys y`, "[4 ['a']]"},
		{nur351Mk + `def x 7 9]]] end do (mk) end 4 keys x`, "ERROR:the arguments were 4 (an Integer) and 9 (an Integer)"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR352BranchRematchBeneathValues (NUR352 #1): values beneath a runtime
// rematch over a union branch — the statement island seats them.
func TestNUR352BranchRematchBeneathValues(t *testing.T) {
	const c = `def c true end `
	for _, tc := range []struct{ src, want string }{
		{c + `7 8 each (if c [[1]] [3]) [2]`, "[7 8 [1]]"},
		{c + `9 7 8 each (if c [[1]] [3]) [2]`, "[9 7 8 [1]]"},
		{c + `7 8 each (if c [[1]] [3]) [2] end 5`, "[7 8 [1] 5]"},
		{`def c false end 7 8 each (if c [[1]] [3]) [2]`, "ERROR:cannot call `each`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR352ArmPendingListRaiseDeclines (NUR352 #2): an arm that leaves a
// list literal the interpreter keeps PENDING — evaluated where it is
// consumed, or never, when a code-body slot takes it raw (`each (if c
// [[dup]] [3]) [2 3]` runs `[dup]` as each's body) — while the model
// evaluates it where the arm ends and raised there. The raise is no arm
// trap: none is recorded, and the program declines on the pass's own
// unmatched dispatch (it used to raise dup's no-match compiled, and to raise
// before an effect the interpreter runs first). An arm list that
// evaluates cleanly still compiles.
func TestNUR352ArmPendingListRaiseDeclines(t *testing.T) {
	const c = `def c true end `
	for _, tc := range []struct{ src, want string }{
		{c + `each (if c [[dup]] [3]) [2 3]`, "[[2 3]]"},
		{c + `each (if c [[dup]] [3]) [2]`, "[[2]]"},
		{c + `each (if c [[1 add]] [3]) [2 3]`, "[[3 4]]"},
		{c + `each (if c [[dup]] [[3]]) [2 3]`, "[[2 3]]"},
		{c + `(if c [[dup]] [3]) typeof`, "ERROR:cannot call `dup`"},
		{c + `(if c [[dup]] [3]) print "a" typeof`, "ERROR:cannot call `dup`"},
		{c + `if c [[dup]] [3]`, "ERROR:cannot call `dup`"},
	} {
		declinesWithInterpAnswer(t, tc.src, "unmatched dispatch recovered at", tc.want)
	}
	for _, tc := range []struct{ src, want string }{
		{c + `if c [[1 2]] [3]`, "[[1 2]]"},
		{c + `if c [[1 add 2]] [3]`, "[[3]]"},
		{c + `(if c [[1 2]] [3]) size`, "[2]"},
		{c + `if c [dup] [3]`, "ERROR:cannot call `dup`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR352NoMatchRendersTheLiteral (NUR352 #3): a poly's no-match over an
// operand the compiled code assembled from a list literal before the match
// renders the literal as written, as the interpreter's report does — it
// evaluates a pending literal only once a signature takes it.
func TestNUR352NoMatchRendersTheLiteral(t *testing.T) {
	const h = `def h fn [[] [Any] [3]] end `
	for _, tc := range []struct{ src, want string }{
		{h + `each (h) [1 add 2]`, "ERROR:the arguments were 3 (an Integer) and [1 word(add) 2] (a List)"},
		{h + `each (h) [1 add 2] end 5`, "ERROR:[1 word(add) 2]"},
		{h + `each (h) [[1 add 2] 5]`, "ERROR:[[1 word(add) 2] 5]"},
		{h + `def x 4 end each (h) [x add 1]`, "ERROR:[word(x) word(add) 1]"},
		{h + `def g fn [[] [Any] [each (h) [1 add 2]]] end g`, "ERROR:[1 word(add) 2]"},
		{h + `each (h) [1 2]`, "ERROR:[1 2] (a List)"},
		// The match takes the evaluated literal where a signature fits.
		{`def h fn [[] [Any] [[4]]] end each (h) [1 add 2]`, "[[4]]"},
		{h + `size (h) [1 add 2]`, "[3 [3]]"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR352GradualDefReadStatementIsland (NUR352 #4): a root read of a
// def-bound member value (gradual) that holds a fn at run time, a literal
// written before it in its statement: the interpreter dispatches the read as
// a word, its forward phase taking the 7, and `add` then takes the 1. The
// read's point is tested before the statement's first op and hands the
// statement to the interpreter from its first token; data runs on.
func TestNUR352GradualDefReadStatementIsland(t *testing.T) {
	const fnM = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end def x m.f end `
	for _, tc := range []struct{ src, want string }{
		{fnM + `1 x 7 add`, "[9]"},
		{fnM + `3 1 x 7 add add`, "[12]"},
		{fnM + `print "a" 1 x 7 add`, "[9]"},
		{fnM + `1 x 7 add end 5`, "[9 5]"},
		{fnM + `8 x 7 eq`, "[true]"},
		{fnM + `10 x 7 sub`, "[2]"},
		{fnM + `1 x 7 add print "b"`, "[9]"},
		{`def mk fn [[] [Any] [([x:Integer] => [x add 1])]] end def x (mk) end 1 x 7 add`, "[9]"},
		// Data runs on as the model has it.
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end def x m.f end 1 x 7 add`, "[1 12]"},
		{`def mk fn [[] [Any] [5]] end def x (mk) end 1 x 7 add`, "[1 12]"},
		// NEGATIVE: the fn matches nothing it could collect, on both lanes.
		{`def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end def x m.f end 1 x 7 add`, "ERROR:cannot call `x`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}
