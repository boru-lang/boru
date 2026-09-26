package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR247ApplyWordParkCount pins NUR247's close: the `apply` word re-steps
// its lead over the stack, and a value that matches nothing parks beside its
// window on both lanes — n+1 values where the recorded event claims one. A
// list over it seated the one, a silent wrong answer under a no-contract fn
// (`[(5 f/v apply)]` compiled `[5 [fn]]`). A list literal over the word's
// event alone — or over a run of such regions — now collects the run's
// count through the region mark, in a fn frame as at top level (the unit's
// collect plan); every other consuming layout takes the word's one-result
// form (OpCallDynApplyOne), which raises on any other count. In place, and
// over a lead that fits, the answer is the interpreter's.
func TestNUR247ApplyWordParkCount(t *testing.T) {
	const str, inc = `([s:String] => [s])`, `([s:Integer] => [s add 1])`
	for _, r := range []struct{ src, want string }{
		{`def h fn [[f:Function] [] [(5 f/v apply)]] end h ` + str, "[5 fn f(String)]"},
		{`def h fn [[f:Function] [] [[(5 f/v apply)]]] end h ` + inc, "[[6]]"},
		// The collect: the park's run, counted at run time.
		{`def h fn [[f:Function] [] [[(5 f/v apply)]]] end h ` + str, "[[5 fn f(String)]]"},
		{`def h fn [[f:Any] [] [[(5 f/v apply)]]] end h ` + str, "[[5 fn f(String)]]"},
		{`def h fn [[f:Any] [Any] [[(5 f/v apply)] size]] end h ` + str, "[2]"},
		{`def h fn [[f:Any] [] [[(5 f/v apply) (6 f/v apply)]]] end h ` + str, "[[5 fn f(String) 6 fn f(String)]]"},
		{`def h fn [[f:Any] [] [[(5 f/v apply) (6 f/v apply)]]] end h ` + inc, "[[6 7]]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	// Beside a const the run cannot be collected in order: the one-result
	// form stands, and raises on the park.
	for _, r := range []struct{ src, interp string }{
		{`def h fn [[f:Function] [] [[7 (5 f/v apply)]]] end h ` + str, "[[7 5 fn f(String)]]"},
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
