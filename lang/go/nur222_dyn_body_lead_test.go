package lang

import (
	"fmt"
	"testing"
)

// TestNUR222DynBodySettlesItsOwnLead pins NUR222's close. A `do` whose body
// the closure path declined runs as a DYN BODY: its handler runs the body
// with the interpreter's semantics, re-steps included, and the result is the
// body's own residual. The check pass modelled a dynamic member read in that
// body as a lead left over its argument (`do [m.f 5]` → [fn, 5]), and the
// program's residual arm applied it — a second time, over a region the body
// had already settled to [6]: `CALL_DYNAMIC underflow`, an internal error
// where the interpreter answers 6. A lead whose argument is another of the
// same dyn body's results is the body's to settle, and the residual arm
// leaves it; a dyn-body result with nothing of its own above it is still the
// lead the interpreter re-steps over a later token.
func TestNUR222DynBodySettlesItsOwnLead(t *testing.T) {
	const m = `def inc fn [[n:Integer] [Integer] [n add 1]] end def mk fn [[] [Map] [{f: inc/v}]] end def m (mk) end `
	const q = `def y fn [[] [Integer] [42]] end def h fn [[] [Integer] [42]] end def h fn [[x:Atom/q] [Atom] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	const strict = `def y fn [[] [Integer] [42]] end def h fn [[x:Integer] [Integer] [x]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{m + `do [m.f 5]`, "[6]"},
		{m + `7 do [m.f 5]`, "[7 6]"},
		// The `/q` capture inside the dyn body (NUR190's class): the body
		// captures the word, and the residual keeps its answer.
		{q + `do [m.f y]`, "[y]"},
		// The body's own no-match, caught by `do` as the interpreter does.
		{strict + `do [m.f y]`, "[error(call to 'h' matched no signature)]"},
		// A dyn-body result the interpreter re-steps over a LATER token is
		// still the residual arm's apply.
		{m + `do [inc/v] 5`, "[6]"},
		{m + `do [m.f/v] 5`, "[6]"},
		{m + `5 do [m.f/v]`, "[6]"},
		{m + `do [m.f]`, "[fn inc(Integer)]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
}
