package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR254UndecidedPatternPark pins NUR254: an anonymous fn value whose
// value pattern meets a CARRIER at its re-step — `(n ([0] => [1]))` over an
// Integer param — is undecided on the check pass: the run applies it where
// n meets the pattern's literal and parks it where it does not. The pass
// parked it statically, so any layout that did not re-record the apply baked
// the park: `[(n ([0] => [1])) 7]` over n = 0 compiled `[[0 fn 7]]` for the
// interpreter's `[[1 7]]` — silent. The re-step records the trailing
// dynamic apply the run decides now (recordUndecidedApply), a variadic
// region under NUR246's rules: it seats in place and collects in a list
// over it alone, on both lanes, and a fixed layout beside it declines.
func TestNUR254UndecidedPatternPark(t *testing.T) {
	const lam = `([0] => [1])`
	for _, r := range []struct{ src, want string }{
		{`def h fn [[n:Integer] [Any] [(n ` + lam + `)]] end h 0`, "[1]"},
		{`def h fn [[n:Integer] [Any] [(n ` + lam + `)]] end h 5`, "ERROR:h: expected 1 return value(s), got 2"},
		{`def h fn [[n:Integer] [List] [[(n ` + lam + `)]]] end h 0`, "[[1]]"},
		{`def h fn [[n:Integer] [List] [[(n ` + lam + `)]]] end h 5`, "[[5 fn (Integer)]]"},
		{`def h fn [[m:Map] [Any] [(m ({a:1} => [7]))]] end h {a:1}`, "[7]"},
		{`def h fn [[m:Map] [Any] [(m ({a:1} => [7]))]] end h {a:2}`, "ERROR:h: expected 1 return value(s), got 2"},
		// A concrete value decides the pattern at the pass: no region.
		{`(0 ` + lam + `)`, "[1]"},
		{`def lam ` + lam + ` end (0 lam) 7`, "[1 7]"},
		// Without the paren, the interpreter's re-step collects forward
		// first and parks: nothing undecided to record.
		{`def h fn [[n:Integer] [List] [[n ` + lam + ` 7]]] end h 0`, "[[0 fn (Integer) 7]]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	// The witness: beside another element the region's count cannot be
	// laid out, so the program declines where it answered wrong.
	for _, n := range []string{"0", "5"} {
		src := `def h fn [[n:Integer] [List] [[(n ` + lam + `) 7]]] end h ` + n
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		if !noteCompileDefect(t, src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "consumes loop results") {
			t.Errorf("%s: want the region's decline, got %v compiled=%v err=%v", src, gotC, compiled, errC)
		}
	}
	if gotI, errI := mustNew(t).RunInterp(`def h fn [[n:Integer] [List] [[(n ` + lam + `) 7]]] end h 0`); errI != nil || fmt.Sprint(gotI) != "[[1 7]]" {
		t.Errorf("interp got %v / %v, want [[1 7]]", gotI, errI)
	}
}
