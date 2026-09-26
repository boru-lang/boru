package lang

import (
	"fmt"
	"testing"
)

// TestNUR219CallbackParamReadIsTheWord pins NUR219's close. A fn value or a
// lambda handed to a higher-order word compiles to a callback BODY unit
// (each$body, fold$body, …) with the value's own named params, and a bare
// read of a param is a WORD dispatch on the interpreter when the element it
// holds is a fn (NUR123) — `def g fn [[f:Any] [Any] [f]]  each g/v [([] =>
// [1]) 7]` is [1 7] — where the unit pushed the slot: [fn f 7]. The unit now
// lists the params it reads that way (CompiledFn.FnReadParams), the pushed
// closure carries the callback value it was compiled from, and an invocation
// with a fn in such a slot runs the value the handler's own interpreter lane
// runs it; data elements run the unit. Every row compiles and agrees.
func TestNUR219CallbackParamReadIsTheWord(t *testing.T) {
	const g = `def g fn [[f:Any] [Any] [f]] end `
	for _, c := range []struct{ src, want string }{
		// The token seam (each over a list): the named fn, the lambda, a
		// consuming read, a gradual collection, a paren.
		{g + `each g/v [([] => [1]) 7]`, "[[1 7]]"},
		{`each ([x:Any] => [x]) [([] => [1]) 7]`, "[[1 7]]"},
		{`def g fn [[x:Any] [Any] [x typeof]] end each g/v [([] => [1]) 7]`, "[[Integer Integer]]"},
		{g + `def run fn [[xs:List] [List] [each g/v xs]] end run [([] => [1]) 7]`, "[[1 7]]"},
		{`def g fn [[f:Any] [Any] [(f)]] end each g/v [([] => [1]) 7]`, "[[1 7]]"},
		// A capturing lambda: the source runs over the closure's runtime
		// captures (k = 5), not the check pass's carrier.
		{`def run fn [[k:Integer xs:List] [List] [each ([x:Any] => [(x add k)]) xs]] end run 5 [([] => [1]) 7]`, "[[6 12]]"},
		// The fn-VALUE seam (the map-iteration fold): the accumulator is
		// the fn, read bare.
		{`fold ([a:Any kv:Any] => [a]) {x: 1} ([] => [5])`, "[5]"},
		{`def g fn [[a:Any kv:Any] [Any] [a]] end fold g/v {x: 1} ([] => [5])`, "[5]"},
		// A fn-typed param read bare is the same word (the element list is
		// all fns, so the carrier is Function); one the body applies in a
		// paren was always the apply.
		{`def g fn [[f:Function] [Any] [f]] end each g/v [([] => [1])]`, "[[1]]"},
		{`def g fn [[f:Function] [Any] [(f 3)]] end each g/v [([n:Integer] => [n add 1])]`, "[[4]]"},
		// Data through the same units answers as it always did.
		{g + `each g/v [1 2 3]`, "[[1 2 3]]"},
		{`each ([x:Any] => [x]) [7 8]`, "[[7 8]]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// Negative: a fn element the bare read cannot call raises the word's
	// no-match on both lanes, at the read — never the value as data.
	src := g + `each g/v [([n:Integer] => [n])]`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if codeOf(errI) != "signature_error" || len(gotI) != 0 {
		t.Fatalf("%s: interpreter %v / %v, want the no-match", src, gotI, errI)
	}
	if !compiled || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 {
		t.Errorf("%s: compiled %v / %v (compiled=%v), want the interpreter's %v", src, gotC, errC, compiled, errI)
	}
}
