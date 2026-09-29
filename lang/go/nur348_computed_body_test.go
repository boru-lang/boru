package lang

import "testing"

// nur348_computed_body_test.go pins NUR348's close: two computed-body shapes
// the compiled runtime deferred where the interpreter answers.

// TestNUR348LiveReadUnderATrap: a read seated live after a computed body
// whose model type no overload of its consumer takes. `do (mk) end x.0` over
// `def x 0` is a static no-match to the pass — the program ends in a terminal
// trap, which dropped the body's run as dead, one checked value — while the
// run's binding is the body's `[1 2]`, which the interpreter's dot takes:
// `[1]` interpreted, the internal_error "do over a computed body left 0
// value(s)" compiled. The read's live point is now planned under the trap
// too (compiler trapHeldBeneath): in the trap's own statement, at its first
// token, over the stack the pass told there, whose results the compiled code
// keeps where the interpreter keeps them — so the point hands the statement
// to the interpreter over exactly its stack, whatever the run's count.
func TestNUR348LiveReadUnderATrap(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{mkList + `do (mk) end x.0`, "[1]"}, // the register's witness
		{mkList + `do (mk) end [x.0]`, "[[1]]"},
		{mkList + `do (mk) end 9 x.0`, "[9 1]"},
		{`def x 0 end def mk fn [[][List][quote [def x [1 2] 4]]] end do (mk) end x.0`, "[4 1]"},
		{`def x 0 end def mk fn [[][List][quote [def x [1 2] 4 5]]] end do (mk) end x.0`, "[4 5 1]"},
		{`def x 0 end def mk fn [[][List][quote [def x [7 8] 4]]] end do (mk) end x.1`, "[4 8]"},
		{mkList + `do (mk) end x end x.0`, "[[1 2] 1]"},
		// The island runs the program after the statement too: a later
		// statement's raise is the interpreter's.
		{mkList + `do (mk) end x.0 end keys 5`, "ERROR:cannot call `keys`"},
		// A read in another statement than the trap's plans no point
		// there: the trap's own raise.
		{mkList + `do (mk) end x end keys 5`, "ERROR:cannot call `keys`"},
		// The negative half: a body that leaves the name's type as the
		// model has it meets the trap, whose raise is the interpreter's —
		// caret included (the rematch now underlines the dispatching
		// token, `x` of `x.0`, where it underlined the word's name).
		{`def x 0 end def mk fn [[][List][quote [4]]] end do (mk) end x.0`, "ERROR:cannot call `dot`"},
		{`def x 0 end def mk fn [[][List][quote []]] end do (mk) end x.0`, "ERROR:cannot call `dot`"},
		{`def x 0 end def mk fn [[][List][quote [def x 7]]] end do (mk) end x.0`, "ERROR:cannot call `dot`"},
		{`def x 0 end x.0`, "ERROR:cannot call `dot`"},
		// A body that rebinds the name to a value of the model's type and a
		// consumer that takes it: no trap, the plain point as before.
		{`def x [0] end def mk fn [[][List][quote [def x [1 2]]]] end do (mk) end x.0`, "[1]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// Still loud (never a wrong answer): the read in the body's own
	// statement, whose first token the test would run before the body.
	requireLoudDefer(t, mkList+`do (mk) x.0`, "left 0 value(s)", "[1]")
}

// TestNUR348SpliceResult: a `do` over a literal body that reads a `word`
// value by value hands back the splice marker, which the interpreter
// splices back at the `do` and fires. The pass fired the marker it
// modelled and recorded the expansion after the call, but the compiled `do`
// pushed the marker and the VM's screen rejected it: `def w word [1 2] end
// do [w/v]` was `[1 2]` interpreted, the internal_error "tape-coupled
// handler result at do" compiled. The fired marker is now taken out of the
// call's results where the tape removed it (compiler splice_outs.go), and
// the VM seats the run without it when it renders as the pass's
// (vm_splice_outs.go).
func TestNUR348SpliceResult(t *testing.T) {
	const w = `def w word [1 2] end `
	for _, c := range []struct{ src, want string }{
		{w + `do [w/v]`, "[1 2]"}, // the register's witness
		{w + `do [w/v] end`, "[1 2]"},
		{w + `[do [w/v]]`, "[[1 2]]"},
		{w + `(do [w/v])`, "[1 2]"},
		{w + `do [1 w/v]`, "[1 1 2]"},
		{w + `do [w/v 3]`, "[1 2 3]"},
		{w + `[do [w/v 3]]`, "[[1 2 3]]"},
		{w + `do [w/v w/v]`, "[1 2 1 2]"},
		{w + `do [w/v] do [w/v]`, "[1 2 1 2]"},
		{w + `3 do [w/v] add`, "[3 3]"},
		{w + `do [w/v 3] add`, "[1 5]"},
		{w + `do [w/v] swap`, "[2 1]"},
		{w + `def z (do [w/v]) end z`, "[2 1]"},
		{w + `do [if true [w/v] [0]]`, "[1 2]"},
		{w + `for 2 [do [w/v]]`, "[1 2 1 2]"},
		{w + `[1 2] each [drop do [w/v]]`, "[[2 2]]"},
		{w + `def f fn [[][Any][do [w/v] add]] end f`, "[3]"},
		{w + `def f fn [[][Any][do [w/v]]] end f`, "ERROR:expected 1 return value(s), got 2"},
		{`def w word [1 add] end 5 do [w/v]`, "[6]"},
		{`def w word [1 add] end 5 do [w/v 3]`, "[5 4]"},
		{`def w word [dup] end 4 do [w/v]`, "[4 4]"},
		{`def w word 5 end do [w/v]`, "[5]"},
		{`def a 7 end def w word [a] end def a 8 end do [w/v]`, "[8]"},
		// The negative half: a body whose value is no splice, and a splice
		// the body quotes, stay data as before.
		{`def w 5 end do [w/v]`, "[5]"},
		{w + `do [w]`, "[1 2]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A computed body's splice result is no marker the pass stepped: the
	// screen's loud defer, as before, never the marker as data.
	requireLoudDefer(t, w+`def mk fn [[][List][quote [w/v]]] end do (mk)`, "tape-coupled handler result at do", "[1 2]")
}
