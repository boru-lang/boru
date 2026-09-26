package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR247ApplyWordParkCount pins NUR247's silent half: the `apply` word
// re-steps its lead over the stack, and a value that matches nothing parks
// beside its window on both lanes — n+1 values where the recorded event
// claims one. A list over it seated the one, a silent wrong answer under a
// no-contract fn (`[(5 f/v apply)]` compiled `[5 [fn]]`). An `apply`-word
// event a later event consumes takes the word's one-result form
// (OpCallDynApplyOne) now, which raises on any other count; in place, and
// over a lead that fits, the answer is the interpreter's.
func TestNUR247ApplyWordParkCount(t *testing.T) {
	const str, inc = `([s:String] => [s])`, `([s:Integer] => [s add 1])`
	for _, r := range []struct{ src, want string }{
		{`def h fn [[f:Function] [] [(5 f/v apply)]] end h ` + str, "[5 fn f(String)]"},
		{`def h fn [[f:Function] [] [[(5 f/v apply)]]] end h ` + inc, "[[6]]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	for _, r := range []struct{ src, interp string }{
		{`def h fn [[f:Function] [] [[(5 f/v apply)]]] end h ` + str, "[[5 fn f(String)]]"},
		{`def h fn [[f:Any] [] [[(5 f/v apply)]]] end h ` + str, "[[5 fn f(String)]]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || !compiled || !strings.Contains(fmt.Sprint(errC), "netted 2 value(s)") {
			t.Errorf("%s: want the one-result raise, got %v compiled=%v err=%v", r.src, gotC, compiled, errC)
		}
		if gotI, errI := mustNew(t).RunInterp(r.src); errI != nil || fmt.Sprint(gotI) != r.interp {
			t.Errorf("%s: interp got %v / %v, want %s", r.src, gotI, errI, r.interp)
		}
	}
}
