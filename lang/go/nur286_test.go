package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR286LandingBeneathIsLoud pins NUR286's close (silent → loud). A gradual
// run's result — a `do` body's fn, a list member's lambda — lands where values
// sat beneath it in its frame, and the interpreter's re-step applies an
// argument-taking fn over them there: `def j (5 do [(mk)]) end j` binds 6. The
// landing never consumes a stack operand (NUR175's rule), so the residual arms
// apply it — and where a def took the group's first value, a later word took
// the fn, or the island's flat window would collect a value written after the
// group, no arm did, and the lane answered `[fn (Integer) 5]`. Such a landing
// now raises a designed defer when the fn takes an argument; the shapes an arm
// re-steps as the interpreter does — the trailing apply's lead, a window entry
// under a function word — and data or a nullary fn answer as before. A fn that
// would not match the values beneath is loud too: the landing cannot see them
// (they are pushed at their consumer).
func TestNUR286LandingBeneathIsLoud(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end `
	const mk2 = `def mk fn [[][Any][([x:Integer y:Integer] => [x sub y])]] end `
	for _, src := range []string{
		mk + `def j (5 do [(mk)]) end j`,
		mk + `def j (5 do [(mk)]) end`,
		mk + `def j (5 do [mk]) end j`,
		mk + `(5 do [(mk)]) typeof`,
		mk + `5 do [(mk)] typeof`,
		mk + `(5 do [(mk)]) 9`,
		mk + `def j ("a" do [(mk)]) end j`,
		`def mk fn [[][Function][([x:Integer] => [x add 1])]] end def j (5 do [(mk)]) end j`,
		`def mk fn [[n:Integer][Any][([x:Integer] => [x add n])]] end def j (5 do [(mk 1)]) end j`,
		`def l [([x:Integer] => [x add 1])] end def j (5 do [l.0]) end j`,
		`def l [([x:Integer] => [x add 1])] end (5 do [l.0]) typeof`,
		// A fn body's form: its RET counted the fn and the 5 as two results
		// where the interpreter answers `[Integer]` (guardUnitLandings).
		mk + `def g fn [[][Any][(5 do [(mk)]) typeof]] end g`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || !strings.Contains(fmt.Sprint(errC), "NUR286") {
			t.Errorf("%s: the landing's guard raises; got compiled=%v %v %v / interpreter %v %v", src, compiled, gotC, errC, gotI, errI)
		}
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
