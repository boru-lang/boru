package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR287AllDynamicDriftWindow pins NUR287's close. Two results the check
// pass holds as dynamic carriers under a forward-eligible word with a
// literal after it (`def mk fn [[][Any][42]] end mk mk add 1`) are the
// interpreter's forward collection over concrete values: `add (Number,
// Number)` takes the 1 and the top 42, `[42 43]`. The check-mode match over
// two carriers took `add (Bytes, Bytes)` all-stack, and the poly re-match
// over that window answered `[84 1]`, silently; three carriers took `add
// (Map, Any, Service)` and raised a claim failure. The forward-drift window
// required a CONCRETE operand beneath the dynamic top, so it — and the
// decline it mirrors — stood aside. A dynamic top is the whole
// precondition now: the window islands the verbatim tokens, and a shape it
// cannot take keeps the decline. Inside a list literal the window's
// variadic result met a fixed-count assembly (`[7 mk add 1]` was `[7 [43]]`
// for `[[7 43]]`, present before): the window stands aside there, and the
// decline answers. A word bound to a literal after the word (`def k 1 end mk
// mk add k`) is collected as its value on the interpreter, so both guards
// read it as the forward operand (core's ForwardOperandValue): it answered
// `[84 1]` too.
func TestNUR287AllDynamicDriftWindow(t *testing.T) {
	const mk = `def mk fn [[][Any][42]] end `
	for _, c := range []struct{ src, want string }{
		{mk + `mk mk add 1`, "[42 43]"},
		{mk + `mk mk mk add 1`, "[42 42 43]"},
		{mk + `mk mk sub 1`, "[42 41]"},
		{mk + `mk mk add`, "[84]"},
		{mk + `7 mk add 1`, "[7 43]"},
		{mk + `mk 7 add 1`, "[42 8]"},
		{mk + `if true [mk mk add 1] [0]`, "[42 43]"},
		{mk + `do [mk mk add 1]`, "[42 43]"},
		{`def mk fn [[][Any]["a"]] end mk mk add 1`, "[aa 1]"},
		{`def mk fn [[][Any][([] => [42])]] end def j (mk) end j j add 1`, "[42 43]"},
		{mk + `def k 1 end mk mk add k`, "[42 43]"},
		{mk + `def s "x" end mk mk add s`, "[42 42x]"},
		{mk + `mk mk add (1)`, "[42 43]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The reserved literal word `true` after the word is collected as its
	// Boolean: `s mk add true` over a `true` result is add's Boolean refusal
	// on both lanes, where the poly re-match answered `[ytrue true]`.
	gotC, _, errC, gotI, errI := runBothEngines(t, `def mk fn [[][Any][true]] end def s "y" end s mk add true`)
	if codeOf(errI) != "type_error" || codeOf(errC) != codeOf(errI) || len(gotC) != 0 {
		t.Errorf("s mk add true: add's Boolean refusal on both lanes; got %v %v / %v %v", gotC, errC, gotI, errI)
	}
	// A PLACED value in the window — a user call's parked result, a paren's
	// placed survivor — is data the interpreter's add meets and refuses; the
	// island re-stepped it over the 5 and answered 7 (present before). It
	// rides into the island inside its own paren, which parks a fn and
	// leaves data as it is.
	const lam = `def mk fn [[][Any][([x:Integer] => [x add 1])]] end def l [([x:Integer] => [x add 1])] end `
	for _, src := range []string{lam + `5 mk add 1`, lam + `5 (mk) add 1`, lam + `5 (l.0) add 1`} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "signature_error" || codeOf(errC) != codeOf(errI) || len(gotC) != 0 {
			t.Errorf("%s: add's refusal of the parked fn on both lanes; got %v %v / %v %v", src, gotC, errC, gotI, errI)
		}
	}
	for _, src := range []string{
		mk + `[mk mk add 1]`,
		mk + `[7 mk add 1]`,
		mk + `mk mk add 1 drop`,
	} {
		_, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled || !strings.Contains(fmt.Sprint(errC), "forward operand accounting across a dynamic/island residual") || errI != nil {
			t.Errorf("%s: declines at the drift guard, the interpreter answers; got compiled=%v %v / %v %v", src, compiled, errC, gotI, errI)
		}
	}
}
