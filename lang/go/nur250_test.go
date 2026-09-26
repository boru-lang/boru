package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR250BareReadBeforeApply pins NUR250's silent half: a BARE read of a
// fn-typed param before the `apply` word is a call on the interpreter
// (NUR078) — the fn fires at the word, or raises its no-match there, and
// `apply` meets its result — while the compiled lane handed the VALUE to
// the word, answering where the interpreter raises. Such a lead is no
// longer the word's pending apply: it takes the dynamic lead's decline. A
// `/v` read is the word's lead on both lanes, and a def-bound name agrees.
func TestNUR250BareReadBeforeApply(t *testing.T) {
	const inc, str = `([s:Integer] => [s add 1])`, `([s:String] => [s])`
	for _, r := range []struct{ src, want string }{
		{`def h fn [[f:Function] [Any] [(5 f/v apply)]] end h ` + inc, "[6]"},
		{`def lam ` + str + ` end (5 lam apply)`, "ERROR:cannot call `lam`"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	for _, r := range []struct{ src, interp string }{
		{`def h fn [[f:Function] [Any] [(5 f apply)]] end h ` + inc, "cannot call `apply`"},
		{`def h fn [[f:Function] [] [(5 f apply)]] end h ` + str, "cannot call `f`"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "apply over a dynamic lead") {
			t.Errorf("%s: want the dynamic lead's decline, got %v compiled=%v err=%v", r.src, gotC, compiled, errC)
		}
		if _, errI := mustNew(t).RunInterp(r.src); !strings.Contains(fmt.Sprint(errI), r.interp) {
			t.Errorf("%s: interp got %v, want %q", r.src, errI, r.interp)
		}
	}
}
