package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR228GradualStackWindowDeclines pins NUR228: `send`'s overloads split
// its operands by the runtime type of the value beneath it — a Pid there is
// taken as the destination after ONE forward token, anything else leaves it
// and both forward tokens go to `send (Any, String)`. The check pass saw a
// dynamic carrier there (a `whereis` result), matched the Pid overload
// optimistically and compiled that window, so the compiled lane sent
// `{a: 1}` to None and raised signature_error where the interpreter sent it
// to "nobody". The compile now declines loudly, the gradual-split reason
// named, and the interpreter's answer stands.
func TestNUR228GradualStackWindowDeclines(t *testing.T) {
	for _, src := range []string{
		`def v (whereis "x") v send {a: 1} "nobody"`,
		`def v (whereis "x") def n "nobody" v send {a: 1} n`,
		`whereis "x" send {a: 1} (self)`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if errI != nil || fmt.Sprint(gotI) != "[None]" {
			t.Errorf("%s: interpreter %v / %v, want [None]", src, gotI, errI)
		}
		if compiled || errC == nil || !strings.Contains(errC.Error(), "forward/stack split depends on a gradual operand") {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want the gradual-split decline", src, gotC, errC, compiled)
		}
	}
}

// TestNUR228ProvenWindowsStillCompile is the paired negative: a window the
// runtime value cannot change compiles and agrees with the interpreter — a
// concrete value beneath, a token no other overload claims (both lanes raise
// the same no-match), and a paren that seals the call off from the stack.
func TestNUR228ProvenWindowsStillCompile(t *testing.T) {
	for _, src := range []string{
		`"q" send {a: 1} "nobody"`,
		`def v:Any 5 v send {a: 1} "nobody"`,
		`def v (whereis "x") v send {a: 1} 5`,
		`def v (whereis "x") [v (send {a: 1} "nobody")]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%s: must compile, got %v", src, errC)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || codeOf(errC) != codeOf(errI) {
			t.Errorf("%s: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
		}
	}
}
