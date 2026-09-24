package lang

import (
	"fmt"
	"strings"
	"testing"
)

// do_body_read_test.go pins NUR193: a def-bound computed fn read inside a
// `do` body. The interpreter dispatches the WORD inside the body's frame,
// which `do` opens empty — nothing to call over, the raise caught into an
// Error value — or over the tokens after it. The compiled lane used to
// model the do's result as the fn carrier and apply it over the value
// beneath the do (`7 do [a5]` was 12, silent), and the island stepped the
// def-bound compiled closure as anonymous DATA (parked over the empty
// frame) where the interpreter's fn definition raises. Two fixes: the
// closure a compiled program binds by `def` is bridged into the word
// dispatch under its name (core's lookupUncachedBridged, dispatchesAsWord),
// and `do`'s result model takes the computed-body hatch when the body
// leaves such a carrier (DoListReturnsFn).

const doBodyMk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def a5 (mk 5) end `

func TestDoBodyReadParity(t *testing.T) {
	for _, row := range []struct{ label, src, want string }{
		{"the empty frame's caught no-match beneath a value", doBodyMk + `7 do [a5]`, "[7 error(cannot call `a5` — no signature matches the arguments)]"},
		{"the empty frame's caught no-match", doBodyMk + `do [a5]`, "[error(cannot call `a5` — no signature matches the arguments)]"},
		{"the written operand", doBodyMk + `do [a5 7]`, "[12]"},
		{"the paren-bounded apply", doBodyMk + `do [(a5 7)]`, "[12]"},
		{"the written operand under a later word", doBodyMk + `do [a5 7 add 1]`, "[13]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, row.src)
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
	}
}

// TestDoBodyReadWrittenNoMatchBails pins NUR194 as measured: a written
// operand the closure's contract does not take. The interpreter's matcher
// falls back from the written token to the frame (`each [a5 "s" add] [1
// 2]` is `['6s' '7s']`) or raises into `do`'s Error value; the shaped
// read's window claims the written token as the argument, and the run
// bails on the claim (loud; `do [a5 "s"]` was `[fn a5(Integer) s]` on
// main, silent).
func TestDoBodyReadWrittenNoMatchBails(t *testing.T) {
	for _, row := range []struct{ src, interp string }{
		{doBodyMk + `do [a5 "s"]`, "[error(cannot call `a5` — no signature matches the arguments)]"},
		{doBodyMk + `each [a5 "s" add] [1 2]`, "[['6s' '7s']]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, row.src)
		if errI != nil || fmt.Sprint(gotI) != row.interp {
			t.Errorf("%q: interp %v / %v, want %s", row.src, gotI, errI, row.interp)
		}
		if !compiled || !strings.Contains(fmt.Sprint(errC), "violates the host-registered shape claim") {
			t.Errorf("%q: measured as a bail on the shape claim, got compiled=%v %v / %v", row.src, compiled, gotC, errC)
		}
		requireCompileDefect(t, row.src, gotC, errC)
	}
}
