package lang

import (
	"strings"
	"testing"
)

// nur333_334_live_deopt_test.go pins the live-read deopt (compiler
// kept_live_deopt.go), the loud remainders of NUR333/NUR334. A read seated
// live after a computed keep-defs body reads the registry the body installed
// into, and the statement around it was compiled for the value the model
// guessed. Where the binding turned out to be one that statement cannot take
// — a fn or a class a bare read dispatches, an active token (a `word`
// splice), a value of another type — the compiled run raised internal_error
// ("dynamic-scope read of a dispatching binding", "… of an active token",
// "CALL_NATIVE_POLY no match"). The read's statement is now a deopt point
// whose value is the binding itself: tested where the statement begins, it
// hands the statement (and the rest of the unit, or of the program) to the
// interpreter, which does with the binding what its step does.

const (
	mkList   = `def x 0 end def mk fn [[][List][quote [def x [1 2]]]] end `
	mkSplice = `def x 0 end def mk fn [[][List][quote [def x word [1 2]]]] end `
	mkFn     = `def g fn [[][Integer][7]] end def x 0 end def mk fn [[][List][quote [def x g/v]]] end `
	unitHead = `def f fn [[b:List][Any][def t 0 do b drop `
)

// TestLiveDeoptRootNoMatch: a root read rebound to a value no overload of
// its consumer takes. The consumer's poly re-match found no match and had no
// layout to raise from (the root's tape beneath the operands holds the
// body's gradual result), so the run deferred; the statement is now the
// interpreter's, and so is the error, byte for byte.
func TestLiveDeoptRootNoMatch(t *testing.T) {
	for _, src := range []string{
		mkList + `do (mk) end x add 1`, // the register's witness
		mkList + `do (mk) end 1 add x`, // the infix word pending over the read
		mkList + `do (mk) x add 1`,
		mkList + `do (mk) end x/v add 1`,
		mkList + `do (mk) end x add 1 end 5`,
		mkList + `do (mk) end x x add`,
		// An each body is a block (phase 2): the body ASSIGNS the var.
		`var x 0 end def mk fn [[][List][quote [var x [1 2]]]] end [1 2] each (mk) end x add 1`,
		mkFn + `do (mk) end x/v add 1`,
		`def x 0 end def mk fn [[][List][quote [def x {a:1}]]] end do (mk) end x add 1`,
		`def x 0 end def mk fn [[][List][quote [def x true]]] end do (mk) end x add 1`,
		`def x 0 end def mk fn [[][List][quote [def x Integer]]] end do (mk) end x add 1`,
		`def x 0 end def mk fn [[][List][quote [def x None]]] end do (mk) end x add 1`,
		`def x 0 end def mk fn [[][List][quote [def x [5]]]] end do (mk) end x end x add 1`,
		`def x 0 end def y 0 end def mk fn [[][List][quote [def x [5] def y "a"]]] end do (mk) end x y add`,
		// The unit twins. The first raised the interpreter's error already;
		// the infix one laid its operands out as the compiled window, and
		// its report listed the rebound [1 2] beside the 1 where the
		// interpreter's add, pending over the read, found the 1 alone —
		// now the island starts at the 1 the add takes off the stack.
		unitHead + `t add 1]] end f (quote [def t [1 2] 1])`,
		unitHead + `1 add t]] end f (quote [def t [1 2] 1])`,
		unitHead + `t/v add 1]] end f (quote [def t [1 2] 1])`,
		unitHead + `t t add]] end f (quote [def t [1] 1])`,
		`def g fn [[][Integer][7]] end ` + unitHead + `t/v add 1]] end f (quote [def t g/v 1])`,
	} {
		agreeOnBothLanes(t, src, "ERROR:cannot call `add`")
	}
	// A body that unbinds the name: the interpreter's add meets the unbound
	// word as the end of its forward collection and raises its own
	// signature_error, where the lookup raised undefined_word first.
	agreeOnBothLanes(t, `def x 0 end def mk fn [[][List][quote [undef x]]] end do (mk) end 1 add x`, "ERROR:cannot call `add`")
	agreeOnBothLanes(t, `def x 0 end def mk fn [[][List][quote [undef x]]] end do (mk) end x add 1`, "ERROR:undefined word: x")
}

// TestLiveDeoptValReadSplice: a `/v` read of a name the body bound to a
// `word` splice. The value spelling delivers the splice as data (stepWordVal),
// and a later step splices it wherever the interpreter steps it again — a
// paren's result, a def of it read bare. The run deferred on the active token;
// the statement is now the interpreter's.
func TestLiveDeoptValReadSplice(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{mkSplice + `do (mk) end x/v`, "[word()({[1 2]})]"}, // the register's witness
		{mkSplice + `do (mk) end [x/v]`, "[[word()({[1 2]})]]"},
		{mkSplice + `do (mk) end x/v drop 5`, "[5]"},
		{mkSplice + `do (mk) end x/v typeof`, "[word()]"},
		{mkSplice + `do (mk) end size [x/v]`, "[1]"},
		{mkSplice + `do (mk) end x/v 5`, "[word()({[1 2]}) 5]"},
		{mkSplice + `do (mk) end (x/v)`, "[1 2]"},
		{mkSplice + `do (mk) end def z x/v end z`, "[1 2]"},
		{unitHead + `[t/v]]] end f (quote [def t word [1 2] 1])`, "[[word()({[1 2]})]]"},
		{unitHead + `t/v typeof]] end f (quote [def t word [1 2] 1])`, "[word()]"},
		{unitHead + `(t/v)]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 2"},
		// A fn the value spelling reads stays data on both lanes.
		{mkFn + `do (mk) end x/v`, "[fn x]"},
		{mkFn + `do (mk) end [x/v]`, "[[fn x]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestLiveDeoptSplice: a bare read of a name the body bound to a `word`
// splice is the splice's tokens, stepped where the read stood.
func TestLiveDeoptSplice(t *testing.T) {
	const y = `def y 0 end def mk fn [[][List][quote [def y word [1 2]]]] end `
	for _, c := range []struct{ src, want string }{
		{y + `do (mk) end y`, "[1 2]"}, // the register's witness
		{y + `do (mk) end [y]`, "[[1 2]]"},
		{y + `do (mk) end y add 1`, "[1 3]"},
		{y + `do (mk) y`, "[1 2]"},
		{y + `do (mk) end y end 9`, "[1 2 9]"},
		{mkSplice + `do (mk) end x x`, "[1 2 1 2]"},
		{`def x 0 end def mk fn [[][List][quote [def x word [add 1]]]] end do (mk) end x 5`, "[6]"},
		{unitHead + `t]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `[t]]] end f (quote [def t word [1 2] 1])`, "[[1 2]]"},
		{unitHead + `t add 1]] end f (quote [def t word [5] 1])`, "[6]"},
		{unitHead + `size [t]]] end f (quote [def t word [1 2] 1])`, "[2]"},
		{unitHead + `(t)]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `if true [t] [0]]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `[1 2] each [drop t]]] end f (quote [def t word [5] 1])`, "[[5 5]]"},
		// The island starts where the interpreter holds nothing pending: at
		// a literal an infix word takes off the stack, at a `case` whose arm
		// reads the name, at the list the read sits in.
		{unitHead + `10 t add]] end f (quote [def t word [5] 1])`, "[15]"},
		{unitHead + `[10 t]]] end f (quote [def t word [5 6] 1])`, "[[10 5 6]]"},
		{unitHead + `[1 2 t]]] end f (quote [def t word [add] 1])`, "[[3]]"},
		{unitHead + `def u [t] u]] end f (quote [def t word [5 6] 1])`, "[[5 6]]"},
		{unitHead + `case 1 [[1] [t] [2] [0]]]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 3"},
		{unitHead + `3 case [[gt 1] [t] [0]]]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 3 — [3 1 2]"},
		{unitHead + `for 1 [t drop]]] end f (quote [def t word [1 2] 1])`, "[1]"},
		{unitHead + `if (1 eq 1) [t] [0]]] end f (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `if (1 eq 1) [t] [0]]] end f (quote [def t 4 1])`, "[4]"},
		// Beside a gradual read's own point (NUR123): both islands serve.
		{`def f fn [[b:List m:Map][Any][def t 0 def j (m get "f") j typeof end drop do b drop t]] end f (quote [def t word [5] 1]) {f: 1}`, "[5]"},
		{`def g fn [[][Integer][3]] end def f fn [[b:List m:Map][Any][def t 0 def j (m get "f") j typeof end drop do b drop t]] end f (quote [def t word [5] 1]) {f: g/v}`, "[5]"},
		{mkSplice + `do (mk) end [10 x 30]`, "[[10 1 2 30]]"},
		{mkSplice + `do (mk) end [10 [x] 30]`, "[[10 [1 2] 30]]"},
		{mkSplice + `do (mk) end {a: x b: 2}`, "[{a:[1 2] b:2}]"},
		{mkSplice + `do (mk) end (x) add 1`, "[1 3]"},
		{mkSplice + `do (mk) end x dup`, "[1 2 2]"},
		{mkSplice + `do (mk) end def z [x] end z`, "[[1 2]]"},
		{mkSplice + `do (mk) end [x] each [typeof]`, "[[Integer Integer]]"},
		// A read after a body in the same list: the body runs before the
		// test could, so no point is placed, and the list is data anyway.
		{mkSplice + `[do (mk) x]`, "[[1 2]]"},
		// A read the pass met in a spliced word's tokens, which carry the
		// definition's positions: no point starts at the definition's list
		// (the sweep's for-each splice ran the tail twice, [3 3]).
		{`def zzvsp word [def acc (flex []) end def mk fn [[][Function][([e:Integer] => [acc push e])]] end for-each (mk) [1 2 3] end size acc] zzvsp`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// An each body is a block (phase 2): its splice def ends with the
	// element's run and x after the loop is the module's 0; the compiled lane
	// declines the shadow until its block scopes land.
	ruleOrDecline(t, `def x 0 end def mk fn [[][List][quote [def x word [3 4]]]] end [1 2] each (mk) end x`, "[[1 2] 0]")
}

// TestLiveDeoptFn: a bare read of a name the body bound to a fn is the
// interpreter's call of it — over the arguments written after it, too.
func TestLiveDeoptFn(t *testing.T) {
	const h = `def h fn [[a:Integer][Integer][a add 1]] end `
	for _, c := range []struct{ src, want string }{
		{mkFn + `do (mk) end x`, "[7]"}, // the register's witness
		{mkFn + `do (mk) end [x]`, "[[7]]"},
		{mkFn + `do (mk) end x add 1`, "[8]"},
		{mkFn + `do (mk) end size [x]`, "[1]"},
		{mkFn + `do (mk) end (x add 1)`, "[8]"},
		{h + `def x 0 end def mk fn [[][List][quote [def x h/v]]] end do (mk) end x 5`, "[6]"},
		{`def x 0 end def mk fn [[][List][quote [def x fn [[][Integer][8]]]]] end do (mk) end x`, "[8]"},
		{`def C class {a:1} end def x 0 end def mk fn [[][List][quote [def x C]]] end do (mk) end x typeof`, "[Class]"},
		{`def g fn [[][Integer][7]] end ` + unitHead + `t]] end f (quote [def t g/v 1])`, "[7]"},
		{`def g fn [[][Integer][7]] end ` + unitHead + `t add 1]] end f (quote [def t g/v 1])`, "[8]"},
		{`def g fn [[][Integer][7]] end ` + unitHead + `[t]]] end f (quote [def t g/v 1])`, "[[7]]"},
		{`def g fn [[][Integer][7]] end ` + unitHead + `size [t]]] end f (quote [def t g/v 1])`, "[1]"},
		{h + unitHead + `t 5]] end f (quote [def t h/v 1])`, "[6]"},
		{mkFn + `do (mk) end [10 x 30]`, "[[10 7 30]]"},
		{mkFn + `do (mk) end x dup`, "[7 7]"},
		{`def h fn [[a:Integer b:Integer][Integer][a sub b]] end def x 0 end def mk fn [[][List][quote [def x h/v]]] end do (mk) end x 10 3`, "[7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestLiveDeoptPlainBindings is the negative half: a body that rebinds the
// name to a value the compiled statement takes — the same type, a def of it,
// a literal around it — answers as before on both lanes, and the forms the
// point is placed over (a def, a map, a branch condition) keep their answers
// when it fires.
func TestLiveDeoptPlainBindings(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end x add 1`, "[6]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end do (mk) end 1 add x`, "[6]"},
		{`def x 0 end def mk fn [[][List][quote [def x "s"]]] end do (mk) end 1 add x`, "[1s]"},
		{`def x 0 end def mk fn [[][List][quote [def x 2.5]]] end do (mk) end x/v add 1`, "[3.5]"},
		{`def x 0 end def y 0 end def mk fn [[][List][quote [def x 1 def y "a"]]] end do (mk) end x add y`, "[1a]"},
		{mkList + `do (mk) end x`, "[[1 2]]"},
		{mkList + `do (mk) end size x`, "[2]"},
		{mkList + `do (mk) end def z x end z`, "[[1 2]]"},
		{mkList + `do (mk) end {a: x}`, "[{a:[1 2]}]"},
		{mkList + `do (mk) end x typeof`, "[List]"},
		{mkList + `do (mk) end if (x eq 1) [1] [2]`, "[2]"},
		{`def x 0 end def mk fn [[][List][quote [def x None]]] end do (mk) end x`, "[None]"},
		{`def x 0 end def mk fn [[][List][quote [def x Integer]]] end do (mk) end 5 is x`, "[true]"},
		{unitHead + `t add 1]] end f (quote [def t 5 1])`, "[6]"},
		{`def f fn [[b:List][Integer][def t 0 do b drop t add 1]] end f (quote [def t 2.5 1])`, "ERROR:expected Integer, got Float"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestLiveDeoptSpliceReturned: a unit's `/v` read of a splice is its RESULT,
// which the interpreter steps again where the call stood (`f` answers the
// splice's tokens). The island's residual went back to a compiled caller
// that would keep it as data, and the screen deferred ("tape-coupled deopt
// result"). Where the call is the program's last op — the unit returns
// straight to the root's end — the RET steps the results there on the
// interpreter over the stack beneath (vmContext.rootEndStep,
// rootEndResults, NUR334). Any other caller takes the call's result island
// (compiler call_result_island.go, CompiledFn.CallResults): the call's
// statement runs again on the interpreter, the call's run written as the
// results it left.
func TestLiveDeoptSpliceReturned(t *testing.T) {
	const call = `f (quote [def t word [1 2] 1])`
	const g = `def g fn [[][Any][`
	for _, c := range []struct{ src, want string }{
		{unitHead + `t/v]] end ` + call, "[1 2]"}, // the register's witness
		{unitHead + `t/v]] end ` + call + ` end`, "[1 2]"},
		{unitHead + `t/v]] end print "z" ` + call, "[1 2]"},
		{unitHead + `t/v]] end def h fn [[][Any][` + call + `]] end (h)`, "[1 2]"},
		{unitHead + `t/v]] end f (quote [def t word [[1 add 2]] 1])`, "[[3]]"},
		{unitHead + `t/v]] end f (quote [def t word [] 1])`, "[]"},
		{`def g fn [[][Integer][7]] end ` + unitHead + `t/v]] end f (quote [def t word [g] 1])`, "[7]"},
		{unitHead + `t/v]] end f (quote [def t word [add] 1])`, "ERROR:cannot call `add`"},
		{unitHead + `t/v]] end f (quote [def t word [nosuch] 1])`, "ERROR:undefined word: nosuch"},
		{`def f fn [[b:List][Integer][def t 0 do b drop t/v]] end ` + call, "ERROR:expected Integer, got word()"},
		// A caller that goes on after the call — a value it pushes at the
		// program's end, a word, a list literal, a paren — takes the call's
		// result island (the register's former loud rows).
		{unitHead + `t/v]] end 9 ` + call, "[9 1 2]"},
		{unitHead + `t/v]] end ` + call + ` 5`, "[1 2 5]"},
		{unitHead + `t/v]] end [` + call + `]`, "[[1 2]]"},
		{unitHead + `t/v]] end def r (` + call + `) end r`, "[2 1]"},
		{unitHead + `t/v]] end 4 5 f (quote [def t word [add] 1])`, "[9]"},
		{unitHead + `t/v]] end 1 f (quote [def t word [add] 1]) 5`, "[6]"},
		{unitHead + `t/v]] end ` + call + ` add 5`, "[1 7]"},
		{unitHead + `t/v]] end (` + call + `) 5`, "[1 2 5]"},
		{unitHead + `t/v]] end {a: (f (quote [def t word [1] 1]))}`, "[{a:1}]"},
		{unitHead + `t/v]] end if true [` + call + `] [0] 5`, "[1 2 5]"},
		{unitHead + `t/v]] end print "a" ` + call + ` print "b"`, "[1 2]"},
		{unitHead + `t/v]] end f (quote [def t word [nosuch] 1]) 5`, "ERROR:undefined word: nosuch"},
		{`def f fn [[b:List][Integer][def t 0 do b drop t/v]] end 9 ` + call, "ERROR:expected Integer, got word()"},
		// In a fn body: the island runs the rest of the unit, whose return
		// check takes the stepped results.
		{unitHead + `t/v]] end ` + g + call + ` drop 4]] end (g)`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `t/v]] end ` + g + `f (quote [def t word [7] 1]) add 1]] end (g)`, "[8]"},
		{unitHead + `t/v]] end ` + g + `[` + call + `]]] end (g)`, "[[1 2]]"},
		{unitHead + `t/v]] end ` + g + `f (quote [def t word [5] 1]) end]] end 1 (g)`, "[1 5]"},
		{unitHead + `t/v]] end ` + g + call + ` end]] end (g) 5`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `t/v]] end def g fn [[b:List][Any][f b 5]] end 3 g (quote [def t word [1 2] 1])`, "ERROR:expected 1 return value(s), got 3"},
		{unitHead + `t/v]] end def g fn [[b:List][Any][f b 5]] end 3 g (quote [def t word [] 1])`, "[3 5]"},
		// The negative half: a binding the statement takes stays compiled.
		{unitHead + `t/v]] end f (quote [def t 5 1])`, "[5]"},
		{unitHead + `t/v]] end f (quote [def t 5 1]) 9`, "[5 9]"},
		{unitHead + `t/v]] end 9 f (quote [def t 5 1])`, "[9 5]"},
		{unitHead + `t/v]] end [f (quote [def t 5 1])]`, "[[5]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	if dis := compileDisasm(t, unitHead+`t/v]] end 9 `+call); !strings.Contains(dis, "[call-result island]") {
		t.Errorf("the call takes its result island; got:\n%s", dis)
	}
	// Where no island can take the call's statement the screen keeps its
	// designed defer: loud, never the data. A call in a fn body's tail,
	// whose frame the interpreter may eliminate or not by the caller's context
	// (Engine.tcoEligible's forward-paren rule), keeps no island there. (A
	// word before the call on its level, `k`, and a read the interpreter
	// makes before a lazy list's elements, `c.n`, are written as the values
	// the compiled code read: TestNUR334CallResultReadsWritten.)
	const tail = `def g fn [[][Any][(` + call + `)]] end `
	for _, c := range []struct{ src, want string }{
		{unitHead + `t/v]] end ` + tail + `def r (g) end r`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `t/v]] end ` + tail + `1 add (g)`, "ERROR:expected 1 return value(s), got 2"},
		{unitHead + `t/v]] end ` + tail + `[(g)]`, "[[1 2]]"},
		{unitHead + `t/v]] end def g fn [[b:List][Any][f b]] end 3 g (quote [def t word [1 2] 1])`, "[3 1 2]"},
	} {
		requireLoudDefer(t, c.src, "tape-coupled deopt result", c.want)
	}
}

// TestLiveDeoptIslandMadeLoopDef: a unit whose island's names include a def
// made inside a loop or branch body — which no registry-visible bind can
// make — planned no live point (and dropped it beside the unit's other
// points), so the read kept the lookup's own designed defer ("dynamic-scope
// read of an active token `t`"). A body written after every island's start
// is the islands' own: each runs the whole form again before it reads the
// name, so the def needs no bind (compiler markIslandMadeDefs, NUR334).
func TestLiveDeoptIslandMadeLoopDef(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{unitHead + `t drop for 1 [def u 2] 7]] end f (quote [def t word [5] 1])`, "[7]"}, // the register's witness
		{`def f fn [[b:List m:Map][Any][def t 0 def j (m get "f") j typeof end drop do b drop t drop for 1 [def u 2] 7]] end f (quote [def t word [5] 1]) {f: 1}`, "[7]"},
		{unitHead + `t drop for 1 [def u 2] 7]] end f (quote [def t 4 1])`, "[7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The loop body's def is the iteration's own since phase 2: the read
	// after the loop is the var the body assigns, which — declared after the
	// computed `do` body — the unit reads live and declines to place (the
	// kept-defs read rule); the interpreter's 2 is the answer.
	for _, src := range []string{
		unitHead + `t drop var u 0 for 2 [var u 2] u]] end f (quote [def t word [5] 1])`,
		unitHead + `t drop var u 0 for 2 [var u 2] u]] end f (quote [def t 3 1])`,
	} {
		ruleOrDecline(t, src, "[2]")
	}
}

// TestLiveDeoptSplicedWord: a read the pass met in a spliced word's tokens
// carries the definition's positions, so no point started in the program's
// own tokens (silentWordBefore — the island would start at the definition's
// list) and the lookup kept its designed defer where the binding is one the
// compiled statement cannot take. The point's island is now the program as
// the splice left it (compiler spliceBody, DeoptSpec.Island): `def w word [
// … ]` fires where the program's one later bare `w` stands, and the island
// runs from the read's statement in the word's tokens, then the program
// after `w`.
func TestLiveDeoptSplicedWord(t *testing.T) {
	const x5 = `def x 0 end def mk fn [[][List][quote [def x word [5]]]] end `
	for _, c := range []struct{ src, want string }{
		{mkSplice + `def w word [do (mk) end x] w`, "[1 2]"}, // the register's witnesses
		{mkFn + `def w word [do (mk) end x] w`, "[7]"},
		{mkList + `def w word [do (mk) end x] w`, "[[1 2]]"},
		{mkSplice + `def w word [do (mk) end x] end w`, "[1 2]"},
		{mkSplice + `def w word [do (mk) end x] end w end 9`, "[1 2 9]"},
		{x5 + `def w word [do (mk) end x add 1] end w`, "[6]"},
		{x5 + `def w word [do (mk) end [x]] end w`, "[[5]]"},
		{x5 + `def w word [do (mk) end x/v] end w`, "[word()({[5]})]"},
		{`def x 0 end def mk fn [[][List][quote [def x 5]]] end def w word [do (mk) end x] end w`, "[5]"},
		{`def x 0 end def mk fn [[][List][quote [undef x]]] end def w word [do (mk) end x] end w`, "ERROR:undefined word: x"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The negative half: a word before `w` in its statement may collect what
	// the splice leaves, so the program as the splice left it is no island's
	// to run from the read: the lookup keeps its designed defer, loud.
	requireLoudDefer(t, mkSplice+`def w word [do (mk) end x] end print "a" w`, "dynamic-scope read of an active token `x`", "[1 2]")
}
