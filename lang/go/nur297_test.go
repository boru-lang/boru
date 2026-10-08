package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR297ParkedFnSubstitutionIsLoud pins NUR297's close. A statement
// island writes the value a paren's call left in the paren's place
// (RestartSubst), so the call does not run twice (NUR292). The island steps
// that value as a token. The interpreter parks a paren's value where it
// would dispatch at the pointer (fnReturnPark) and steps any other one, so
// the two differ on a fn value alone. `[5 (g) (l.0 true)]` over a factory
// of a lambda is `[[5 fn (Integer) true]]` interpreted and answered
// `[[6 true]]` compiled. Such a value is a designed defer now, loud. A data
// value is written as before. A `do`'s result, which the interpreter steps
// in the do's place, replaces the do and its body list, so a do over a
// factory still answers the interpreter's re-step.
func TestNUR297ParkedFnSubstitutionIsLoud(t *testing.T) {
	const l = `def h fn [[x:Atom/q] [Any] [x]] end def mkl fn [[] [List] [[h/v h/v]]] end def l (mkl) end `
	const g = `def g fn [[][Any][([x:Integer] => [x add 1])]] end `
	const gi = `def inc fn [[x:Integer][Integer][x add 1]] end def gi fn [[][Any][inc/v]] end `
	const mkq = `def mkq fn [[][Any][quote [gt 3]]] end `
	const mk = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end `
	for _, src := range []string{
		l + g + `[5 (g) (l.0 true)]`,
		l + gi + `[5 (gi) (l.0 true)]`,
		l + gi + `5 (gi) [(l.0 true)]`,
		l + g + `(g) (l.0 true)`,
		l + g + mkq + `[5 (g) 7 if (mkq) ["big"] ["small"]]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || !strings.Contains(fmt.Sprint(errC), "NUR297") {
			t.Errorf("%s: the island's parked fn defers; got compiled=%v %v %v / interpreter %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
	for _, c := range []struct{ src, want string }{
		{l + `def k fn [[][Any][7]] end [5 (k) (l.0 true)]`, "[[5 7 true]]"},
		{l + `def k fn [[][Any][[1 2]]] end [5 (k) (l.0 true)]`, "[[5 [1 2] true]]"},
		{g + mkq + `5 if (mkq) ["big"] ["small"]`, "[big]"},
		{gi + `def j (5 do [(gi)]) end j`, "[6]"},
		{mk + `do [(mk)] 5`, "[6]"},
		{mk + l + `def j (5 do [(mk)]) end [j (l.0 true)]`, "[[6 true]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
