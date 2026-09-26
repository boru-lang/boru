package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR246ParkedApplyCount pins NUR246's close: a paren-bounded fn-value
// apply whose lead the window may not fit PARKS on both lanes — an anonymous
// or `/v`-delivered value that matches nothing is data (ADR-016's gate), so
// the paren nets the window AND the value — while the recorder claimed the
// apply's one result. A list, a map, an interpolation and a reordered
// residual seated the one, which was a silent wrong answer at top level
// (`[(5 lam/v)]` compiled `[5 [fn]]`) and a return-count raise in a fn
// frame. Unless the window provably fits, the result is a variadic region
// now: it seats in place — the program residual, a RET tail, whose count
// check raises as the interpreter's frame does — a top-level list literal
// over it alone collects it through the region mark, a prefix beneath it
// seats below the mark, and every other fixed layout declines.
func TestNUR246ParkedApplyCount(t *testing.T) {
	const (
		lam = `def lam ([s:String] => [s]) end `
		mkS = `def mk fn [[k:Integer][Function][([s:String] => [s k])]] end `
		mkI = `def mk fn [[k:Integer][Function][([s:Integer] => [s k add])]] end `
	)
	for _, r := range []struct{ src, want string }{
		// In place: the park is the answer on both lanes.
		{lam + `(5 lam/v)`, "[5 fn lam(String)]"},
		{mkS + `(5 (mk 1))`, "[5 fn (String)]"},
		{`def h fn [[f:Function] [Any] [(5 f/v)]] end h ([s:String] => [s])`, "ERROR:expected 1 return value(s), got 2"},
		// A gradual argument, or a carrier under a value pattern, proves
		// nothing about the match: the RET tail seats either outcome.
		{`def h fn [[m:Map] [Any] [(m.x ([s:Integer] => [s add 1]))]] end h {x: 5}`, "[6]"},
		{`def h fn [[m:Map] [Any] [(m.x ([s:Integer] => [s add 1]))]] end h {x: 'a'}`, "ERROR:expected 1 return value(s), got 2"},
		{`def h fn [[n:Integer] [Any] [(n ([0] => [1]))]] end h 0`, "[1]"},
		{`def h fn [[n:Integer] [Any] [(n ([0] => [1]))]] end h 5`, "ERROR:expected 1 return value(s), got 2"},
		{`def h fn [[m:Map] [Any] [(m ({a:1} => [7]))]] end h {a:1}`, "[7]"},
		{`def h fn [[m:Map] [Any] [(m ({a:1} => [7]))]] end h {a:2}`, "ERROR:expected 1 return value(s), got 2"},
		// The region mark: a list literal over the park alone, a prefix below it.
		{lam + `[(5 lam/v)]`, "[[5 fn lam(String)]]"},
		{lam + `[(5 lam/v)] size`, "[2]"},
		{lam + `1 (5 lam/v)`, "[1 5 fn lam(String)]"},
		{`[(5 ([s:String] => [s]))]`, "[[5 fn (String)]]"},
		{mkS + `def lam (mk 1) end [(5 lam/v)]`, "[[5 fn lam(String)]]"},
		// A window that provably fits keeps its one result in every layout.
		{`[7 (5 ([s:Integer] => [s add 1]))]`, "[[7 6]]"},
		{mkI + `def lam (mk 1) end [7 (5 lam/v)]`, "[[7 6]]"},
		{mkI + `[7 (5 (mk 1))]`, "[[7 6]]"},
		// A named value raises rather than parks: its count is fixed.
		{`def g fn [[s:String] [Any] [s]] end [7 (5 g/v)]`, "ERROR:matched no signature"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// Every other fixed layout over a window that may park declines; the
	// interpreter's answer is what the compiled lane used to get wrong.
	for _, r := range []struct{ src, reason, interp string }{
		{lam + `[(5 lam/v) 7]`, "consumes loop results", "[[5 fn lam(String) 7]]"},
		{lam + `[7 (5 lam/v)]`, "consumes loop results", "[[7 5 fn lam(String)]]"},
		{lam + `{a: (5 lam/v)}`, "consumes loop results", "[{a:[5 fn lam(String)]}]"},
		{lam + "`x${(5 lam/v)}y`", "consumes loop results", "[x5fn lam(String)y]"},
		{lam + `1 (5 lam/v) 3`, "variadic region promoted", "[1 5 fn lam(String) 3]"},
		// A produced lead reads the stack, so the region mark cannot own it.
		{mkS + `[(5 (mk 1))]`, "consumes loop results", "[[5 fn (String)]]"},
		// In a fn frame: the mark plans are the program's, not a unit's.
		{mkS + `def h fn [[f:Function] [List] [[(5 f/v)]]] end h (mk 1)`, "consumes loop results", "[[5 fn f(String)]]"},
		{mkS + `def h fn [[f:Function] [Any] [[(5 f/v)] size]] end h (mk 1)`, "consumes loop results", "[2]"},
		{mkS + `def h fn [[f:Function] [Any] [(5 f/v) size]] end h (mk 1)`, "consumes loop results", ""},
		// A fn-typed carrier proves nothing, even over a fitting closure, nor
		// does a carrier under a value pattern.
		{`def h fn [[f:Function] [List] [[(5 f/v)]]] end h ([s:Integer] => [s add 1])`, "consumes loop results", "[[6]]"},
		{`def h fn [[n:Integer] [List] [[(n ([0] => [1]))]]] end h 0`, "consumes loop results", "[[1]]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), r.reason) {
			t.Errorf("%s: want a %q decline, got %v compiled=%v err=%v", r.src, r.reason, gotC, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(r.src)
		if r.interp == "" {
			if !strings.Contains(fmt.Sprint(errI), "expected 1 return value(s), got 2") {
				t.Errorf("%s: interp got %v / %v, want the frame's count error", r.src, gotI, errI)
			}
		} else if errI != nil || fmt.Sprint(gotI) != r.interp {
			t.Errorf("%s: interp got %v / %v, want %s", r.src, gotI, errI, r.interp)
		}
	}
}
