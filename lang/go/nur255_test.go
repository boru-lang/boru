package lang

import "testing"

// TestNUR255UnnamedParamLambdaInAList pins NUR255's close. An anonymous
// lambda with UNNAMED params pushes them beneath its body on the interpreter,
// and its frame keeps its declared count off the top, discarding the
// unconsumed args from the bottom. The call's model seated the pushed arg
// beside the result, and a list after the call underflowed at STORE_LOCAL
// (`[(0 ([0] => [1])) 7]` bailed where the interpreter answers [[1 7]]). The
// check pass now trims the analysed residual as the frame does
// (trimUnnamedArgs), on both routes an anonymous lambda's call takes.
func TestNUR255UnnamedParamLambdaInAList(t *testing.T) {
	for _, r := range []struct{ src, want string }{
		{`[(0 ([0] => [1])) 7]`, "[[1 7]]"},
		{`[(0 ([Integer] => [1])) 7]`, "[[0 1]]"},
		{`def lam ([0] => [1]) end [(0 lam) 7]`, "[[1 7]]"},
		{`[(0 1 ([Integer Integer] => [5])) 7]`, "[[0 5]]"},
		// The shapes that answered before keep their answers.
		{`(0 ([0] => [1])) 7`, "[1 7]"},
		{`[(0 ([0] => [1]))]`, "[[1]]"},
		{`[(0 ([x:Integer] => [1])) 7]`, "[[0 1]]"},
		// A body leaving more than its unnamed args account for is the
		// interpreter's count error on both lanes.
		{`[(0 ([0] => [1 2]))]`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
