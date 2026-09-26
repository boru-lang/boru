package lang

import (
	"fmt"
	"testing"
)

// TestNUR259LambdaCountErrorAnchor pins NUR259's close: a literal lambda's
// return-count error is one report on both lanes. The interpreter anchors
// the return check at the fn value's own position, which a lambda built in
// place does not carry; the compiled call borrowed its first argument's
// (`[(0 ([0] => [1 2])) 7]` reported 1:3 beside the interpreter's unknown
// position). A def-bound lambda and a named fn anchor at their word on both
// lanes, as before.
func TestNUR259LambdaCountErrorAnchor(t *testing.T) {
	for _, src := range []string{
		`[(0 ([0] => [1 2])) 7]`,
		`(0 ([0] => [1 2])) 7`,
		`[(0 ([x:Integer] => [1 2])) 7]`,
		`def lam ([0] => [1 2]) end [(0 lam) 7]`,
		`def f fn [[0] [Any] [1 2]] end [(f 0) 7]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errI == nil || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) {
			t.Errorf("%s: one report on both lanes, got compiled=%v %v / %v, interpreter %v / %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
}
