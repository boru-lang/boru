package lang

import "testing"

// TestNUR258UnconsumedUnnamedArgsReturn pins NUR258's close. A body that
// leaves FEWER values than its declared count returns its unconsumed
// unnamed args on the interpreter: they sit at the frame's bottom, and the
// return check takes the count off everything in the frame. The body
// analysis returned nothing for an EMPTY body, so the compiled unit was a
// bare RET and raised `got 0` (`def f fn [[Integer] [Integer] []] end [(f
// 3) 7]` answers [[3 7]] interpreted). An empty body's residual is its
// unnamed args now (emptyBodyResidual).
func TestNUR258UnconsumedUnnamedArgsReturn(t *testing.T) {
	for _, r := range []struct{ src, want string }{
		{`def f fn [[Integer] [Integer] []] end [(f 3) 7]`, "[[3 7]]"},
		{`def f fn [[Integer] [Integer] []] end f 3`, "[3]"},
		{`[(3 ([Integer] => [])) 7]`, "[[3 7]]"},
		{`(3 ([Integer] => []))`, "[3]"},
		{`def f fn [[Integer Integer] [Integer] []] end f 3 4`, "[4]"},
		{`def f fn [[Integer Integer] [Integer Integer] []] end [(f 3 4) 9]`, "[[3 4 9]]"},
		// A 0-return fn's unnamed args flow out; a named param does not.
		{`def v fn [[Integer] [] []] end v 1 99`, "[1 99]"},
		{`def v fn [[x:Integer] [] []] end v 1 99`, "[99]"},
		// The declared contract still holds over them, and a fn with no
		// unnamed arg to return raises the count error.
		{`def f fn [[Integer] [String] []] end f 3`, "ERROR:return value 1: expected String, got Integer"},
		{`def g fn [[] [Integer] []] end g`, "ERROR:expected 1 return value(s), got 0"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
