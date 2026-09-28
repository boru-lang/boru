package lang

import (
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
// Data at run time costs the test and nothing else. Main's #514 closed the
// same record with a gradual shape claim (check tryShapedFnReadArrival),
// which runs first on the merged tree: the reads it claims dispatch over
// their window, and the ones it declines stay loud.
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
		{mk0 + `j end 5`, "[42 5]"},
		{mk2 + `r 5 3`, "[2]"},
		{ms + `r 5 3`, "[2]"},
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
		// Main's gradual claim (#514, check tryShapedFnReadArrival)
		// dispatches the name over its whole window, where the branch's
		// guard deferred: both lanes answer.
		{mk0 + `7 j typeof`, "[7 Integer]"},
		{mk0 + `5 [j]`, "[5 [42]]"},
		{mk0 + `(j)`, "[42]"},
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
	// Main's claim declines what the program-level paths got wrong (#514:
	// a written token the parameter does not take, a read a pending word
	// collects — NUR216); the branch's island answered these, and at the
	// merge the claim, which runs first, stands. Loud either way. `j j`, the
	// shape it could not seat after a claimed read, compiles since its first
	// result is placed (NUR282, claimParked).
	agreeOnBothLanes(t, mk0+`j j`, "[42 42]")
	for _, r := range []struct{ src, reason string }{
		{mk2 + `r 'x' 3`, "a written argument does not fit the wrapper's parameter"},
		{ms + `r 'x' 3`, "a written argument does not fit the wrapper's parameter"},
		{mk0 + `def k j end k`, "is collected where the interpreter dispatches the name as a word (NUR216)"},
		{mc + `f 'x' def q 1 end q`, "a written argument does not fit the wrapper's parameter"},
	} {
		prog, why, _, err := mustNew(t).CompileCheck(r.src)
		if prog != nil || err != nil || !strings.Contains(why, r.reason) {
			t.Errorf("%s: want the decline %q, got prog=%v reason=%q err=%v", r.src, r.reason, prog != nil, why, err)
		}
	}
}
