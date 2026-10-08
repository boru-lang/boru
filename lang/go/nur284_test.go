package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR284BinderFrameReadsTheStoredUnit pins NUR284's close. A container
// member's lambda that reads a free name (`k`) is a stored-ref unit that
// reads the name live, and the interpreter resolves the read against the
// def stack where the value is applied — inside h, h's `def k 7` or its
// param k. From the value's home the name looked module-scope, so h bound
// it as a frame local the unit's lookup could not see: `undefined word: k`
// compiled for the interpreter's 7. A read some fn that binds the name
// reaches (the check's binder model over the value's own reader identity,
// NUR257) now joins the dynamic-scope names, so every binder installs it
// where the lookup finds it. And a fn argument in a slot a callee reads bare
// (NUR218's interpreter route) is a resolved value beneath the call word,
// never a token the island steps: `run m.c` raised `is still waiting for 1
// argument(s)` where the lambda's collection met run's barrier.
func TestNUR284BinderFrameReadsTheStoredUnit(t *testing.T) {
	const m = `def m {c: ([x:Any] => [k])} end `
	for _, c := range []struct{ src, want string }{
		{m + `def h fn [[][Any] [def k 7 m.c 1]] end h`, "[7]"},
		{m + `def h fn [[k:Integer][Any] [m.c 1]] end h 7`, "[7]"},
		{m + `def run fn [[f:Any][Any] [f 1]] end def h fn [[k:Integer][Any] [run m.c]] end h 7`, "[7]"},
		{m + `def k 1 end def h fn [[][Any] [def k 7 m.c 1]] end h`, "[7]"},
		{m + `def h fn [[][Any] [def k 7 [1 2] each [m.c]]] end h`, "[[7 7]]"},
		{m + `def h fn [[k:Integer][Any] [[1 2] each m.c/v]] end h 7`, "[[7 7]]"},
		{m + `def g fn [[][Any] [m.c 1]] end def h fn [[k:Integer][Any] [g]] end h 7`, "[7]"},
		{`def m {c: ([x:Any] => [k add x])} end def h fn [[k:Integer][Any] [m.c 1]] end h 7`, "[8]"},
		{`def l [([x:Any] => [k])] end def h fn [[k:Integer][Any] [l.0 1]] end h 7`, "[7]"},
		{`def run fn [[f:Any][Any] [f 1]] end run ([x:Integer] => [x add 1])`, "[2]"},
		{`def run fn [[f:Any][Any] [f]] end run ([] => [42])`, "[42]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A read no binder reaches is the interpreter's undefined_word, and the
	// check pass stops at it, so the program never compiles: a typo, and a
	// binding h made and dropped before the read.
	for _, src := range []string{
		m + `m.c 1`,
		m + `def h fn [[][Any] [def k 7 0]] end h m.c 1`,
	} {
		_, compiled, errC, _, errI := runBothEngines(t, src)
		if codeOf(errI) != "undefined_word" || compiled || !strings.Contains(fmt.Sprint(errC), "undefined word: k") {
			t.Errorf("%s: the interpreter's undefined_word, and no compile; got compiled=%v %v / %v", src, compiled, errC, errI)
		}
	}
}
