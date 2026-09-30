package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// nur336_348_remainders_test.go pins the 2026-09-29 pass over the loud
// remainders of NUR348 (a computed body's count island past an infix paren,
// and in a fn unit) and NUR336 (a `/v` paren lead, and a unit's island over
// an earlier statement's value).

// TestNUR348CountIslandPastAnInfixParen: a root `do` over a computed body
// whose statement opens with a paren holding an infix call, `(3 add 4) do
// (mk) x.0`. The call's run opens on its stack operand, not its word, so
// parenOf found no paren; its run (callRun) was written inside the paren,
// which then stood before the do as a collection barrier (inertBefore), and
// no count island was placed: "do over a computed body left 0 value(s)"
// compiled, `[7 1]` interpreted. A call run over every token of its paren is
// the paren's one value (wholeParen), and a paren whose tokens are all
// barrier-free or written as values the VM never lets dispatch is no barrier
// (writtenParen): the island writes the paren's value, the call never runs
// twice.
func TestNUR348CountIslandPastAnInfixParen(t *testing.T) {
	const counter = `def c (flex {n:0}) end def g fn [[][Integer][c set 'n' (c.n add 1) drop c.n]] end `
	for _, c := range []struct{ src, want string }{
		{mkList + `(3 add 4) do (mk) x.0`, "[7 1]"}, // the register's witness
		{mkList + `(3 4 add) do (mk) x.0`, "[7 1]"},
		{mkList + `(3 4 add 5 mul) do (mk) x.0`, "[27 1]"},
		{mkList + `(3 add 4 mul 5) do (mk) x.0`, "[35 1]"},
		{mkList + `(3 add 4) 5 do (mk) x.0`, "[7 5 1]"},
		{mkList + `((3 add 4)) do (mk) x.0`, "[7 1]"},
		{mkList + `(3 add 4) (5 add 6) do (mk) x.0`, "[7 11 1]"},
		{mkList + `(3 add 4 5) do (mk) x.0`, "[3 9 1]"},
		{mkList + `(print "a" 5) do (mk) x.0`, "[5 1]"},
		{mkList + `("a" add "b") do (mk) x.0`, "[ab 1]"},
		{mkList + `(3 add 4) do (mk) x.0 x.1`, "[7 1 2]"},
		{mkList + `(3 add 4) add 1 do (mk) x.0`, "[8 1]"},
		{`def x 0 end def mk fn [[][List][quote [def x [1 2] 4]]] end (3 add 4) do (mk) x.0`, "[7 4 1]"},
		// The paren's call runs once: the island writes its value.
		{counter + mkList + `(1 add (g)) do (mk) x.0 c.n`, "[2 1 1]"},
		{counter + mkList + `((g) add 1) do (mk) x.0 c.n`, "[2 1 1]"},
		// The negative half: a body leaving the name as the model has it
		// meets the trap, whose raise is the interpreter's.
		{`def x 0 end def mk fn [[][List][quote [def x 7]]] end (3 add 4) do (mk) x.0`, "ERROR:cannot call `dot`"},
		{`def x 0 end def mk fn [[][List][quote [4]]] end (3 add 4) do (mk) x.0`, "ERROR:cannot call `dot`"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR348UnitComputedRun: a fn unit's `do b` over a computed body whose
// run is the unit's result, `f (quote [w/v])` over `def w word [1 2]`. The
// interpreter's tape splices the run back at the do and steps it — the
// splice fires — where the compiled unit seated the marker as data and the
// VM's screen refused it ("tape-coupled handler result at do"): units
// planned no count island for a computed run. They plan it now
// (planUnitRestarts' run arm): the island runs the unit's body from the
// do's statement with the run written in the do's place.
func TestNUR348UnitComputedRun(t *testing.T) {
	const w = `def w word [1 2] end `
	const g1 = `def g fn [[n:Integer][Integer][n add 1]] end `
	for _, c := range []struct{ src, want string }{
		{w + `def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:expected 1 return value(s), got 2"}, // the register's witness
		{w + `def f fn [[b:List][Any][do b]] end f (quote [w/v 3])`, "ERROR:expected 1 return value(s), got 3"},
		{w + `def f fn [[b:List][Any][do b]] end f (quote [w/v w/v])`, "ERROR:expected 1 return value(s), got 4"},
		{w + `def f fn [[b:List][Any][do b]] end [f (quote [w/v])]`, "ERROR:expected 1 return value(s), got 2"},
		{w + `def f fn [[b:List][Any][do b end]] end f (quote [w/v])`, "ERROR:expected 1 return value(s), got 2"},
		{w + `def f fn [[b:List][Any][print "x" do b]] end f (quote [w/v])`, "ERROR:expected 1 return value(s), got 2"},
		{w + `def f fn [[b:List][Any][do b]] end f (quote [w/v]) 5`, "ERROR:expected 1 return value(s), got 2"},
		{`def w word [1] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[1]"},
		{`def w word [] end def f fn [[b:List][Any][do b]] end f (quote [w/v 4])`, "[4]"},
		{`def w word [] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:expected 1 return value(s), got 0"},
		{`def w word [[1 2]] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[[1 2]]"},
		{`def w word [{a:1}] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[{a:1}]"},
		{`def w word [dup] end def f fn [[b:List][Any][do b]] end 4 f (quote [w/v])`, "ERROR:cannot call `dup`"},
		{`def g fn [[][Integer][7]] end def w word [g] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[7]"},
		{g1 + `def w word [g 4] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[5]"},
		{g1 + `def w word [4 g] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[5]"},
		{`def l ([x:Integer] => [x]) end def w word l end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:cannot call `l`"},
		{g1 + `def w word [g] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:cannot call `g`"},
		// The negative half: a run of plain values seats as before.
		{`def f fn [[b:List][Any][do b]] end f (quote [1])`, "[1]"},
		{`def f fn [[b:List][Any][do b]] end f (quote [Integer])`, "[Integer]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	if dis := compileDisasm(t, w+`def f fn [[b:List][Any][do b]] end f (quote [w/v])`); !strings.Contains(dis, "[count island, unit]") {
		t.Errorf("the unit's do takes its count island; got:\n%s", dis)
	}
	// The island runs the unit's body to its end on its own, where the
	// interpreter's frame runs on into the caller's tape: a named fn value
	// it steps last, with nothing after it, raises uncalled_function there
	// and would be data on the island. The VM defers loudly instead
	// (SigRef.CountFrame) — on a run holding a value the tape dispatches,
	// as it left it or as a splice's payload, before the island runs, and on
	// an island residual holding one after — never the value as data.
	const seatedAsData = "where the run is seated as data"
	for _, c := range []struct{ src, want string }{
		{g1 + `def f fn [[b:List][Any][do b]] end f (quote [g/v])`, "ERROR:call to 'g' matched no signature"},
		{g1 + `def f fn [[b:List][Any][do b]] end f (quote [g/v]) 5`, "ERROR:call to 'g' matched no signature"},
		{g1 + `def w word [] end def f fn [[b:List][Any][do b]] end f (quote [w/v g/v])`, "ERROR:call to 'g' matched no signature"},
		{g1 + `def w word [1 2] end def f fn [[b:List][Any][do b]] end f (quote [g/v w/v])`, "ERROR:call to 'g' matched no signature"},
		{`def w word [] end def f fn [[b:List][Any][do b]] end f (quote [w/v ([x:Integer] => [x])])`, "[fn (Integer)]"},
	} {
		requireLoudDefer(t, c.src, seatedAsData, c.want)
	}
	// A 0-argument fn the run left fires in place (NUR359): the answer, where
	// the frame's check deferred.
	agreeOnBothLanes(t, `def g fn [[][Integer][7]] end def f fn [[b:List][Any][do b]] end f (quote [g/v])`, "[7]")
	const spliceHolds = "left a splice holding a value the interpreter re-steps"
	for _, c := range []struct{ src, want string }{
		{g1 + `def w word g/v end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:call to 'g' matched no signature"},
		{g1 + `def mk fn [[][Map][{a: g/v}]] end def m (mk) end def w word [m.a] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:call to 'g' matched no signature"},
	} {
		requireLoudDefer(t, c.src, spliceHolds, c.want)
	}
	const islandLeft = "the unit's island left a value the interpreter re-steps"
	for _, c := range []struct{ src, want string }{
		{g1 + `def w word [[g/v] 0 get] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "ERROR:call to 'g' matched no signature"},
		{g1 + `def w word [g/v] end def f fn [[b:List][Any][do b]] end f (quote [w/v])`, "[fn g(Integer)]"},
		{g1 + `def w word [(g/v)] end def f fn [[b:List][Any][do b]] end f (quote [w/v]) 5`, "[fn g(Integer) 5]"},
	} {
		requireLoudDefer(t, c.src, islandLeft, c.want)
	}
}

// TestNUR336SlashVLead: a paren apply whose lead is a `/v` member read,
// `(m.f/v y) 9` over a member holding data. The parser mints the `/v` as a
// modifier token after the read, at the read's own position, so the lead's
// path (tokenPath) reached the modifier and the lead's plan wrote the value
// over it, leaving the reach token standing as a barrier before `y`: no
// statement island, and the apply's designed defer ("not an appliable
// function") where the interpreter answers `[5 42 9]`. The lead's plan now
// covers its whole run of tokens at that position (leadRun), written as the
// value the apply found, placed.
func TestNUR336SlashVLead(t *testing.T) {
	const y = `def y fn [[] [Integer] [42]] end `
	const str = `def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end `
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{nurDataMk + y + `(m.f/v y) 9`, "[5 42 9]"}, // the register's witness
		{nurDataMk + y + `(m.f/v y)`, "[5 42]"},
		{nurDataMk + y + `3 end (m.f/v y) 9`, "[3 5 42 9]"},
		{nurDataMk + y + `(m.f/v y 8)`, "[5 42 8]"},
		{nurDataMk + y + `(m.f/v y) add 1`, "[5 43]"},
		{nurDataMk + y + `[(m.f/v y) 9]`, "[[5 42 9]]"},
		{nurDataMk + `(m.f/v 7) 9`, "[5 7 9]"},
		{nurDataMk + y + `for 2 [(m.f/v y) 9 drop]`, "[5 42 5 42]"},
		{nurDataMk + y + `if true [(m.f/v y) 9] [0]`, "[5 42 9]"},
		{nurDataMk + y + `def g fn [[] [Any] [(m.f/v y) drop]] end (g)`, "[5]"},
		{`def mk fn [[] [Map] [{f: 5}]] end ` + y + `def g fn [[q:Map][Any][(q.f/v y) 9 drop]] end g (mk)`, "ERROR:expected 1 return value(s), got 2"},
		// A fn lead: applied, placed, or re-stepped past the paren as the
		// interpreter's paren does.
		{str + y + `("s" dup drop) end (m.f/v y)`, "[s 42]"},
		{str + y + `(m.f/v y) 9`, "[fn (String) 42 9]"},
		{nurFnMk + y + `(m.f/v y) 9`, "[43 9]"},
		{nurFnMk + y + `(m.f/v y)`, "[43]"},
		{two + y + `(m.f/v y) 9`, "[33]"},
		{`def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end ` + y + `(m.f/v y) 9`, "[fn 42 9]"},
		{`def h fn [[] [Integer] [42]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end ` + y + `3 end (m.f/v y)`, "[3 42 42]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336UnitIslandOverAnEarlierStatement: a unit's paren apply over a
// data member after a statement that left a value, `(1 add 2) end (q.f 7)
// drop drop`. The accounting called the earlier value a deferred operand
// (the compiled code may push it late), so no island was planned and the
// apply deferred ("not an appliable function"). It is now admitted where
// the walk finds it: every such value held on the compiled frame beneath
// the statement at its start, and that region intact at the stop
// (deoptPoint.onFrame, frameIntact) — the island's prefix is then the
// interpreter's frame.
func TestNUR336UnitIslandOverAnEarlierStatement(t *testing.T) {
	const mk = `def mk fn [[] [Map] [{f: 5}]] end `
	const inc = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (q.f 7) drop drop]] end g (mk)`, "[3]"}, // the register's witness
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (q.f 7)]] end g (mk)`, "ERROR:expected 1 return value(s), got 3"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) (q.f 7) drop drop]] end g (mk)`, "[3]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (4 add 5) end (q.f 7) drop drop add]] end g (mk)`, "[12]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (4 add 5) end swap end (q.f 7) drop drop drop]] end g (mk)`, "[9]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) dup end (q.f 7) drop drop add]] end g (mk)`, "[6]"},
		{mk + `def g fn [[q:Map n:Integer][Any][(1 add n) end (q.f 7) drop drop]] end g (mk) 5`, "[6]"},
		{mk + `def h fn [[][Integer][9]] end def g fn [[q:Map][Any][(h) end (q.f 7) drop drop]] end g (mk)`, "[9]"},
		{mk + `def h fn [[][Integer Integer][9 8]] end def g fn [[q:Map][Any][(h) end (q.f 7) drop drop add]] end g (mk)`, "[17]"},
		{mk + `def g fn [[q:Map][Any][[1 2] size end (q.f 7) drop drop]] end g (mk)`, "[2]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end [(q.f 7)] drop]] end g (mk)`, "[3]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (q.f 7) drop drop]] end [g (mk) g (mk)]`, "[[3 3]]"},
		{inc + `def g fn [[q:Map][Any][(1 add 2) end (q.f 7) add]] end g (mk)`, "[11]"},
		{inc + `def g fn [[q:Map][Any][(1 add 2) end (q.f 7) drop]] end g (mk)`, "[3]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def g fn [[q:Map][Any][(1 add 2) end (q.f 7)]] end g (mk)`, "[4]"},
		{`def mk fn [[] [Map] [{f: ([] => [9])}]] end def g fn [[q:Map][Any][(1 add 2) end (q.f 7) drop drop]] end g (mk)`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR336IslandOverATakenOrLateValue: a statement that takes an earlier
// statement's value before the stop (`drop (q.f 7)`, `swap (q.f 7)`) left
// the frame region the island seats changed at the stop (frameIntact), and a
// value an earlier statement wrote that the compiled code pushes late (`4 end
// (q.f 7)`, a read `z end (q.f 7)`) was a deferred operand: both deferred
// loudly where the interpreter answers. The first now takes the statement
// over from the stop's own token, where the interpreter holds nothing
// pending (lateStart; at the root over the stack the pass told the paren
// opens over, NoteParenStack); the second seats the value among the frame's
// entries by where it was written (deoptPoint.lits, mergeLits): a literal as
// its constant, a read from the value's slot.
func TestNUR336IslandOverATakenOrLateValue(t *testing.T) {
	const mk = `def mk fn [[] [Map] [{f: 5}]] end `
	const m = mk + `def m (mk) end `
	const inc = `def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop (q.f 7) drop]] end g (mk)`, "[5]"}, // the register's witnesses
		{mk + `def g fn [[q:Map][Any][(1 add 2) end (4 add 5) end swap (q.f 7) drop drop drop]] end g (mk)`, "[9]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end 4 end (q.f 7) drop drop add]] end g (mk)`, "[7]"},
		// Taken before the stop.
		{mk + `def g fn [[q:Map][Any][(1 add 2) end dup (q.f 7) drop drop add]] end g (mk)`, "[6]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop 4 (q.f 7) drop drop]] end g (mk)`, "[4]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop [(q.f 7)]]] end g (mk)`, "[[5 7]]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop 4 [(q.f 7)] drop]] end g (mk)`, "[4]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop (5 (q.f 7)) drop drop]] end g (mk)`, "[5]"},
		{mk + `def g fn [[q:Map n:Integer][Any][n end drop (q.f 7) drop]] end g (mk) 5`, "[5]"},
		{mk + `def g fn [[q:Map Integer][Any][drop (q.f 7) drop]] end g (mk) 5`, "[5]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop (q.f 7) drop]] end [g (mk) g (mk)]`, "[[5 5]]"},
		{m + `(1 add 2) end drop (m.f 7) drop`, "[5]"},
		{m + `(1 add 2) end (4 add 5) end swap (m.f 7) drop drop drop`, "[9]"},
		{m + `1 end 2 end drop (m.f 7)`, "[1 5 7]"},
		{m + `(1 add 2) end dup (m.f 7)`, "[3 3 5 7]"},
		{m + `(1 add 2) end (4 add 5) end over (m.f 7)`, "[3 9 3 5 7]"},
		{m + `(1 add 2) end print/s (m.f 7)`, "[5 7]"},
		{m + `(1 add 2) end drop (m.f 7) (m.f 8)`, "[5 7 5 8]"},
		{m + `(1 add 2) end drop 4 (m.f 7)`, "[4 5 7]"},
		{m + `(1 add 2) end drop (5 (m.f 7)) drop drop`, "[5]"},
		{inc + `def m (mk) end (1 add 2) end drop (m.f 7)`, "[8]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end (1 add 2) end drop 10 (m.f 7)`, "[-3]"},
		{m + `def c (flex {n:0}) end def cg fn [[][Integer][c set 'n' (c.n add 1) drop c.n]] end 3 end drop (cg) (m.f 7) c.n`, "[1 5 7 1]"},
		// Written by an earlier statement, pushed late.
		{mk + `def g fn [[q:Map][Any][4 end (q.f 7) drop drop]] end g (mk)`, "[4]"},
		{mk + `def g fn [[q:Map][Any]["s" end (q.f 7) drop drop]] end g (mk)`, "[s]"},
		{mk + `def g fn [[q:Map][Any]["a" end 4 end (q.f 7) drop drop drop]] end g (mk)`, "[a]"},
		{mk + `def g fn [[q:Map][Any][[1 2] end (q.f 7) drop drop]] end g (mk)`, "[[1 2]]"},
		{mk + `def g fn [[q:Map][Any][{a:1} end (q.f 7) drop drop]] end g (mk)`, "[{a:1}]"},
		{mk + `def g fn [[q:Map][Any][4 end (q.f 7) drop drop]] end [g (mk) g (mk)]`, "[[4 4]]"},
		{mk + `def g fn [[q:Map][Any][def z 3 end z end (q.f 7) drop drop]] end g (mk)`, "[3]"},
		{mk + `def g fn [[q:Map][Any][def z 3 end z end drop (q.f 7) drop]] end g (mk)`, "[5]"},
		{mk + `def g fn [[q:Map][Any][def z [1 2] end z end (q.f 7) drop drop]] end g (mk)`, "[[1 2]]"},
		{mk + `def g fn [[q:Map][Any][def z (1 add 2) end z end (q.f 7) drop drop]] end g (mk)`, "[3]"},
		{mk + `def g fn [[q:Map n:Integer][Any][n end (q.f 7) drop drop]] end g (mk) 4`, "[4]"},
		{mk + `def g fn [[q:Map n:Integer][Any][n end n end (q.f 7) drop drop add]] end g (mk) 4`, "[8]"},
		{inc + `def g fn [[q:Map][Any][4 end (q.f 7) add]] end g (mk)`, "[12]"},
		{m + `(1 add 2) end 4 end (m.f 7) drop drop add`, "[7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The negative half: a word that collected the stop's paren forward
	// dispatches after it, so no token of the statement stands where the
	// interpreter holds nothing pending (lateStart): loud, never a wrong
	// answer.
	for _, c := range []struct{ src, want string }{
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop print (q.f 7)]] end g (mk)`, "[7]"},
		{mk + `def g fn [[q:Map][Any][(1 add 2) end drop size [(q.f 7)]]] end g (mk)`, "[2]"},
		{m + `(1 add 2) end drop print (m.f 7)`, "[7]"},
	} {
		requireLoudDefer(t, c.src, "not an appliable function", c.want)
	}
}

// TestNUR336LoopContinuation: a paren apply's lead that no signature takes
// the paren's values over, in a counted loop's body, `for 2 [(m.f y) 9
// drop]`. Its statement island ran the loop's statement again from its first
// token, which only the loop's first iteration may do, and the lead's own
// run (the landing that stepped y) could not be written into a body every
// iteration runs — so no island was seated and the apply deferred with a
// message that misnamed it ("violates the host-registered shape claim").
// The island is now the loop's per-iteration continuation
// (compiler.LoopCont, loopContRestart): on whatever iteration the stop
// fires, the interpreter resumes its own loop there — its mark, the values
// the iteration left, the stop's statement with the paren written as its
// survivors, the loop's continuation over the body with the earlier
// iterations' values as its results — then the tokens after the loop.
func TestNUR336LoopContinuation(t *testing.T) {
	const two = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end def y fn [[] [Integer] [42]] end `
	const unit = `def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def y fn [[] [Integer] [42]] end `
	const mixed = `def mk fn [[] [List] [[([x:Integer] => [x]) 5]]] end `
	for _, c := range []struct{ src, want string }{
		{two + `for 2 [(m.f y) 9 drop]`, "[]"}, // the register's witness
		{two + `for 3 [(m.f y) 9]`, "[33 33 33]"},
		{two + `for 3 [(m.f y) i]`, "[42 41 40]"},
		{two + `for [1 4] [(m.f y) i] end 5`, "[41 40 39 5]"},
		{two + `for 3 [(m.f y) 9 if (i eq 1) [break] []]`, "[33]"},
		{two + `for 3 [(m.f y) 9 raise oops 'x']`, "ERROR:x"},
		{two + `1 end for 2 [(m.f y) 9 drop]`, "[1]"},
		{two + `"a" end [1 2] end for 2 [(m.f y) 9 drop]`, "[a [1 2]]"},
		{two + `def k 3 end k end for 2 [(m.f y) 9 drop]`, "[3]"},
		{two + `def i 7 end for 2 [(m.f y) 9 drop] end i`, "[7]"},
		{two + `for 2 [(m.f y) 9 drop] end for 2 [(m.f y) 8 drop]`, "[]"},
		{two + `def g fn [[][Any][for 2 [(m.f y) 9 drop] 5]] end (g)`, "[5]"},
		{unit + `def g fn [[q:Map][Any][10 end for 2 [(q.f y) 9 drop]]] end g (mk)`, "[10]"},
		{unit + `def g fn [[q:Map][Any][(1 add 2) end for 2 [(q.f y) 9 drop]]] end g (mk)`, "[3]"},
		{unit + `def g fn [[q:Map n:Integer][Any][n end for 2 [(q.f y) 9 drop]]] end g (mk) 4`, "[4]"},
		{unit + `def g fn [[q:Map Integer][Any][for 2 [(q.f y) 9 drop]]] end g (mk) 4`, "[4]"},
		{unit + `def g fn [[q:Map][Any][for 2 [(q.f y) 9 drop] 5]] end [g (mk) g (mk)]`, "[[5 5]]"},
		{`def mk fn [[] [Map] [{f: 5}]] end def m (mk) end for 3 [(m.f i)]`, "[5 0 5 1 5 2]"},
		// A stop on a later iteration only: the lead is a fn on the first.
		{mixed + `def l (mk) end for 2 [(l.(i) 7) drop]`, "[5]"},
		{mixed + `def l (mk) end for 3 [(l.(i mod 2) 7)]`, "[7 5 7 7]"},
		{mixed + `def g fn [[l:List][Any][for 2 [(l.(i) 7) drop] 9]] end g (mk)`, "ERROR:expected 1 return value(s), got 2"},
		{mixed + `def g fn [[l:List][Any][4 end for 2 [(l.(i) 7) drop]]] end g (mk)`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// Effects run once: the lead's, and each iteration's print.
	for _, src := range []string{
		two + `for 3 [(m.f y) i print]`,
		`def mk fn [[] [Map] [{f: ([x:Integer y:Integer] => [x sub y])}]] end def m (mk) end def y fn [[] [Integer] [print "y" 42]] end for 2 [(m.f y) 9 drop]`,
	} {
		var outC, outI bytes.Buffer
		bc, bi := mustNew(t), mustNew(t)
		bc.SetOutput(&outC)
		bi.SetOutput(&outI)
		gotC, compiled, errC := bc.RunCompiled(src)
		gotI, errI := bi.RunInterp(src)
		if !compiled || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) || outC.String() != outI.String() || outI.Len() == 0 {
			t.Errorf("%s: compiled %v / %v %q, interpreter %v / %v %q", src, gotC, errC, outC.String(), gotI, errI, outI.String())
		}
	}
	// The negative half: a loop body that binds a name — a binding the
	// continuation's later iterations could not see where the compiled code
	// holds it — takes no continuation, and the apply keeps its loud stop,
	// now named for what it is.
	requireLoudDefer(t, two+`for 2 [def u 1 (m.f y) 9 drop]`, "no statement island takes the survivors", "[]")
	// A stop after an earlier statement of the body, whose values the
	// compiled code may push late (`i end`), and a body whose effect before
	// the stop its island would repeat, take no continuation: loud.
	requireLoudDefer(t, mixed+`def l (mk) end for 2 [i end (l.(i) 7) drop]`, "not an appliable function", "[0 1 5]")
	requireLoudDefer(t, mixed+`def l (mk) end def c (flex {n:0}) end for 2 [c set 'n' (c.n add 1) drop (l.(i) 7) drop] c.n`, "not an appliable function", "[5 2]")
}
