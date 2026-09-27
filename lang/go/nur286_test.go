package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR286LandingBeneathIsLoud pins NUR286's close. A gradual run's result —
// a `do` body's fn, a list member's lambda — lands where values sat beneath
// it in its frame, and the interpreter's re-step applies an argument-taking
// fn over them there: `def j (5 do [(mk)]) end j` binds 6. The landing never
// consumes a stack operand (NUR175's rule), so the residual arms apply it —
// and where a def took the group's first value, a later word took the fn, or
// the island's flat window would collect a value written after the group,
// no arm did, and the lane answered `[fn (Integer) 5]`. Such a landing
// carries LandingBeneathGuard, and where the fn takes an argument its
// STATEMENT runs again on the interpreter (the statement island): a `do`
// whose body ran only reads runs again whole, and a body of one call — its
// paren or its bare word — is written as the do's own result, the landed
// value (compiler's doBody). The shapes an arm re-steps as the interpreter
// does, and data or a nullary fn, answer as before. An effect before the
// stop is written as its run (NUR296's call run). A `do` body the island
// cannot write — two calls, a call among other tokens — and an effect that
// took an operand off the stack stay designed defers.
func TestNUR286LandingBeneathIsLoud(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end `
	const mk2 = `def mk fn [[][Any][([x:Integer y:Integer] => [x sub y])]] end `
	for _, src := range []string{
		mk + `def j (5 do [(mk) drop (mk)]) end j`,
		mk + `def j (5 do [1 drop (mk)]) end j`,
		mk + `"x" print/s 5 do [(mk)] typeof`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || !strings.Contains(fmt.Sprint(errC), "NUR286") {
			t.Errorf("%s: the landing's guard raises; got compiled=%v %v %v / interpreter %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
	for _, c := range []struct{ src, want string }{
		{mk + `def j (5 do [(mk)]) end j`, "[6]"},
		{mk + `def j (5 do [(mk)]) end`, "[]"},
		{mk + `def j (5 do [mk]) end j`, "[6]"},
		{mk + `(5 do [(mk)]) typeof`, "[Integer]"},
		{mk + `5 do [(mk)] typeof`, "[Integer]"},
		{mk + `print "x" 5 do [(mk)] typeof`, "[Integer]"},
		{mk + `(5 do [(mk)]) 9`, "[6 9]"},
		{mk + `def j ("a" do [(mk)]) end j`, "[fn (Integer) a]"},
		{`def mk fn [[][Function][([x:Integer] => [x add 1])]] end def j (5 do [(mk)]) end j`, "[6]"},
		{`def mk fn [[n:Integer][Any][([x:Integer] => [x add n])]] end def j (5 do [(mk 1)]) end j`, "[6]"},
		{`def l [([x:Integer] => [x add 1])] end def j (5 do [l.0]) end j`, "[6]"},
		{`def l [([x:Integer] => [x add 1])] end (5 do [l.0]) typeof`, "[Integer]"},
		{mk + `def g fn [[][Any][(5 do [(mk)]) typeof]] end g`, "[Integer]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want string }{
		{mk + `(5 do [(mk)])`, "[6]"},
		{mk + `5 do [(mk)]`, "[6]"},
		{mk + `(5 do [(mk)]) add 1`, "[7]"},
		{mk + `3 (5 do [(mk)])`, "[3 6]"},
		{mk2 + `3 (5 do [(mk)])`, "[2]"},
		{mk + `def g fn [[][Any][def j (5 do [(mk)]) j]] end g`, "[6]"},
		// A fn body's whole-frame replay re-steps the landed value as the
		// interpreter does, so its guard stands aside.
		{mk + `def g fn [[][Any][(5 do [(mk)])]] end g`, "[6]"},
		{mk + `def g fn [[][Any][5 do [(mk)]]] end g`, "[6]"},
		{mk + `def g fn [[][Any][(5 do [(mk)]) add 1]] end g`, "[7]"},
		{`def tbl {inc: ([n:Integer] => [n add 1])} end def g fn [[][Any][def f tbl.inc 5 f]] end g`, "[6]"},
		{`def mk fn [[][Any][(7)]] end def j (5 do [(mk)]) end j`, "[7 5]"},
		{`def mk fn [[][Any][(7)]] end (5 do [(mk)]) add 1`, "[5 8]"},
		{`def mk fn [[][Any][(7)]] end (5 do [(mk)]) 9`, "[5 7 9]"},
		{`def mk fn [[][Any][([] => [42])]] end def j (5 do [(mk)]) end j`, "[fn 5]"},
		{mk + `def j (5 (mk)) end j`, "[fn (Integer) 5]"},
		{mk + `def j (do [(mk)]) end 5 j`, "[6]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
