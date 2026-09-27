package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR298DefGroupCollectingLanding pins NUR298's close. Inside a def's
// operand group the interpreter's re-step applies a landed fn to the literal
// written after it, and the def takes the group's first value:
// `def j (5 do [(mk)] 7) end 1 j` is `[8 1 5]`. The compiled lane stood
// aside for such a landing (a token the one-value re-step cannot reach) and
// left the fn to the residual arms, which met it only after the def, beside
// whatever the program pushed since: it answered `[fn (Integer) 7 1 5]`. On
// this branch NUR266's crossing rule also refused the arms' apply over the
// plain `end j` form, which main answered by luck. The check pass now notes
// such a landing as a COLLECTING one: the root guard takes it where no arm
// applies it, and its statement island runs the re-step. With an effect
// before the stop no island runs again, and the fn is a designed defer.
func TestNUR298DefGroupCollectingLanding(t *testing.T) {
	const mk = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end `
	const mk2 = `def mk fn [[][Any][([x:Integer y:Integer] => [x sub y])]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `def j (5 do [(mk)] 7) end j`, "[8 5]"},
		{mk + `def j (5 do [(mk)] 7) end 1 j`, "[8 1 5]"},
		{mk + `def j (5 do [(mk)] 7) end 1`, "[8 1]"},
		{mk + `def j (5 do [(mk)] "a") end j`, "[a 6]"},
		{mk + `def j (5 do [(mk)] [7]) end j`, "[[7] 6]"},
		{mk + `def j ("s" do [(mk)] 7) end j`, "[8 s]"},
		{mk + `def j (5 do [(mk)] 7 8) end j`, "[8 8 5]"},
		{mk2 + `def j (5 do [(mk)] 7) end j`, "[2]"},
		{`def l [([x:Integer] => [x add 1])] end def j (5 do [l.0] 7) end j`, "[8 5]"},
		{`def m {f: ([x:Integer] => [x add 1])} end def j (5 m.f 7) end j`, "[8 5]"},
		{mk + `(5 do [(mk)] 7)`, "[5 8]"},
		{mk + `def j (print "x" 5 do [(mk)] 7) end j`, "[8 5]"},
		{mk + `def g fn [[][Any][def j (5 do [(mk)] 7) end 1 j]] end g`, "ERROR:expected 1 return value(s), got 3"},
		{`def g fn [[x:Integer][Integer][x mul 3]]  def m {f: g/v}  [5 6] each [m get "f" drop]`, "ERROR:body produced no result"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, src := range []string{
		mk + `def j ("x" print/s 5 do [(mk)] 7) end j`,
		mk + `def j (5 do [(mk) print "y"] 7) end j`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || !strings.Contains(fmt.Sprint(errC), "NUR298") {
			t.Errorf("%s: an effect before the stop defers; got compiled=%v %v %v / interpreter %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
}
