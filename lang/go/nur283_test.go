package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR283InexactLayoutRaisesTheReport pins NUR283's close: an optimistic
// closure bake whose operands are not all the interpreter's tape — a
// constant beneath them, a literal or a function word after them — raises
// the word's full report on both lanes, notes and help included, because
// the layout carries what the plan could reach (core DispatchLayout's
// Beneath and After) and the VM plans that tape. `… 7` shows the plan
// collecting the literal after the written pair into the window.
func TestNUR283InexactLayoutRaisesTheReport(t *testing.T) {
	const mk = `def mk fn [[][Any][5]] end `
	for _, src := range []string{
		mk + `1 2 fold [add] (mk)`,
		mk + `0 scan [add] (mk)`,
		mk + `0 fold [add] (mk) drop`,
		mk + `9 0 fold [add] (mk)`,
		mk + `0 fold [add] (mk) 7`,
		mk + `"a" 0 fold [add] (mk) 7 8`,
		mk + `def g fn [[][Any][1 0 scan [add] (mk)]] end g`,
	} {
		agreeOnBothLanes(t, src, "ERROR:no signature matches the arguments")
		_, _, errC, _, _ := runBothEngines(t, src)
		if !strings.Contains(fmt.Sprint(errC), "= note: the arguments were") {
			t.Errorf("%s: the compiled raise carries the report's notes; got %v", src, errC)
		}
	}
	// A live value an overload takes runs it on both lanes.
	for _, r := range []struct{ src, want string }{
		{`def mk fn [[][Any][[1 2]]] end 9 0 fold [add] (mk)`, "[9 3]"},
		{`def mk fn [[][Any][{a: 1 b: 2}]] end 0 fold [add] (mk) 7`, "[0 10]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
