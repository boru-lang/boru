package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR242DoBodyRunsToNothing pins NUR242's `do` half: a body with no
// definite raise that runs to nothing — it consumed its own values — nets
// NOTHING at run time, or one caught Error when a word in it raises. The
// check pass modelled the Error alone, and a fn body with an unnamed param
// seated that one value in a slot the run never filled (STORE_LOCAL
// underflow). The count is runtime-variable now: the unnamed param seats
// below it at unit start (the apply-loop replay's prefix), and the RET trims
// as the interpreter's frame does — the param answers when the body leaves
// too few.
func TestNUR242DoBodyRunsToNothing(t *testing.T) {
	for _, r := range []struct{ src, want string }{
		{`def f fn [[Integer] [Any] [do [args drop] 7]] end f 5`, "[7]"},
		{`def f fn [[Integer] [Any] [do [args drop]]] end f 5`, "[5]"},
		{`def f fn [[Integer] [Any] [do [3 drop] 7]] end f 5`, "[7]"},
		{`def f fn [[Integer] [Any] [do [3 drop]]] end f 5`, "[5]"},
		{`def f fn [[Integer] [Any] [do [drop] 7]] end f 5`, "ERROR:expected 1 return value(s), got 2"},
		{`do [3 drop] 7`, "[7]"},
		{`(do [1 0 div]).code`, "[arith_error]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// A consumer that needs the run's COUNT declines rather than pop seats
	// the run did not leave: a list literal's assembly, and a fn residual's
	// fixed-width replay window over a dynamic body's run (which bailed
	// with CALL_DYN_FRAME underflow).
	for _, r := range []struct{ src, reason string }{
		{`[do [3 drop] 7]`, "list literal over a result of runtime-variable count"},
		{`def g fn [[b:List] [Any] [do b 7]] end g [5 drop]`, "unapplied fn-value in body residual"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), r.reason) {
			t.Errorf("%s: want a %q decline, got %v compiled=%v err=%v", r.src, r.reason, gotC, compiled, errC)
		}
	}
}
