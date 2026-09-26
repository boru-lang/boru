package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR207RootGradualDefRead pins NUR207's close: a bare read at the
// PROGRAM ROOT of a def-bound value the pass types gradually — a fn's
// declared `Any` result, a container member read — is the interpreter's
// WORD dispatch whenever the binding holds a fn at run time. The root had
// no plan for it (a fn unit's twin read deopts, NUR123), so the read
// lowered to a data push: `def j (mk) end j` answered `[fn j]` for the
// interpreter's 42, and a written window parked the value where the word
// raises `cannot call r`. The root now tests such a read as the fn units
// do: a fn at run time hands the interpreter the program from the read's
// statement on, over the values beneath it, with the value installed under
// its name for the word dispatch; the island's residual is the program's.
// Data at run time costs the test and nothing else.
func TestNUR207RootGradualDefRead(t *testing.T) {
	const (
		mk0 = `def mk fn [[][Any][([] => [42])]] end def j (mk) end `
		mk2 = `def mk fn [[][Any][([a:Integer b:Integer] => [a sub b])]] end def r (mk) end `
		ms  = `def m {s: ([a:Integer b:Integer] => [a sub b])} end def r (m.s) end `
		mk7 = `def mk fn [[][Any][7]] end def j (mk) end `
		mc  = `def m {f: ([n:Integer] => [n add 1])} end def f m.f end `
	)
	for _, r := range []struct{ src, want string }{
		// A read the program residual holds: the island from the read's
		// token, the residual beneath it its prefix.
		{mk0 + `j`, "[42]"},
		{mk0 + `5 j`, "[5 42]"},
		{mk0 + `j j`, "[42 42]"},
		{mk0 + `j end 5`, "[42 5]"},
		{mk2 + `r 5 3`, "[2]"},
		{ms + `r 5 3`, "[2]"},
		{mk2 + `r 'x' 3`, "ERROR:cannot call `r`"},
		{ms + `r 'x' 3`, "ERROR:cannot call `r`"},
		// A read an event consumes: the island from its statement's start.
		{mk0 + `j typeof`, "[Integer]"},
		{mk0 + `[j]`, "[[42]]"},
		{mk0 + `{a: j}`, "[{a:42}]"},
		{mk0 + `if true [j] [0]`, "[42]"},
		{mk0 + `5 j add`, "[47]"},
		{mk0 + `(1 add 2) [j]`, "[3 [42]]"},
		// Data at run time: the test passes the value through.
		{mk7 + `j`, "[7]"},
		{mk7 + `5 j`, "[5 7]"},
		{mk7 + `j typeof`, "[Integer]"},
		{mk7 + `7 j typeof`, "[7 Integer]"},
		{mk7 + `[j]`, "[[7]]"},
		// A read that LEADS the residual's dynamic apply over a window the
		// value matches is that apply's own answer, so a root event after
		// the read, which leaves no island, costs nothing: the guard bails
		// on a no-match alone (the sweep's `def` × container · suffix-def
		// and · splice bailed where both lanes answer 6).
		{mc + `f 5`, "[6]"},
		{mc + `f 5 def zzvpost 8`, "[6]"},
		{"def zzvsp word [" + mc + "f 5] zzvsp", "[6]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	if dis := compileDisasm(t, mc+`f 5 def zzvpost 8`); !strings.Contains(dis, "bail if the read holds a fn its window does not match (guard)") {
		t.Errorf("the leading read's guard must bail on a no-match alone:\n%s", dis)
	}
	// The error an island raises is the interpreter's own, notes included.
	for _, src := range []string{mk2 + `r 'x' 3`, ms + `r 'x' 3`} {
		_, _, errC, _, errI := runBothEngines(t, src)
		if fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%s: compiled %v, interpreter %v", src, errC, errI)
		}
	}

	// A read whose statement the island cannot start from the interpreter's
	// own state — a value written before it that the root lays out at the
	// program's end, a read inside a paren group, a def made through the
	// read — is a GUARD: when the value is a fn it raises a designed defer,
	// loud where it answered wrong silently (`[7 Function]`, `[5 [fn j]]`,
	// `[fn j]`, `[fn k]`).
	for _, r := range []struct{ src, interp string }{
		{mk0 + `7 j typeof`, "[7 Integer]"},
		{mk0 + `5 [j]`, "[5 [42]]"},
		{mk0 + `(j)`, "[42]"},
		{mk0 + `def k j end k`, "ERROR:def is still waiting"},
		// The leading read's guard over a window the value does not match:
		// the word raises, and no island can take the statement over.
		{mc + `f 'x' def q 1 end q`, "ERROR:cannot call `f`"},
	} {
		gotC, _, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || !strings.Contains(fmt.Sprint(errC), "gradual read") {
			t.Errorf("%s: want the guard's defer, got %v / %v", r.src, gotC, errC)
		}
		gotI, errI := mustNew(t).RunInterp(r.src)
		if sub, isErr := strings.CutPrefix(r.interp, "ERROR:"); isErr {
			if !strings.Contains(fmt.Sprint(errI), sub) {
				t.Errorf("%s: interp got %v / %v, want an error containing %q", r.src, gotI, errI, sub)
			}
		} else if errI != nil || fmt.Sprint(gotI) != r.interp {
			t.Errorf("%s: interp got %v / %v, want %s", r.src, gotI, errI, r.interp)
		}
	}
}
