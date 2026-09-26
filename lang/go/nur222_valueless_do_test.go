package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR222ValuelessDoSeatedDeclines pins NUR222: a value-less `do` body
// (non-empty, empty residual, no certain raise) is modelled as the one
// Error a caught raise would leave, latched runtime-variable — and a clean
// run leaves nothing. Seated at a fixed count — promoted to a local in a
// branch fragment, dropped under a condition's decision value — the phantom
// compiled and bailed at run time (STORE_LOCAL / DROP underflow). Each such
// seat declines now, loudly, and the program answers through the
// interpreter. A word that CONSUMES the phantom as its operand (`1 do [1
// drop] drop`) still bails: declining every such consumer also declined a
// loop that always raises (`for 1 [do [for 2 [raise 'x']] drop i]`), which
// the pass cannot prove; that half is NUR222's open part.
func TestNUR222ValuelessDoSeatedDeclines(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`if [do [1 drop] true] [2] [3]`, "[2]"},
		{`if [true] [do [1 drop] 2] [3]`, "[2]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled || codeOf(errC) != "compile_failed" {
			t.Errorf("%q: the phantom's fixed seat declines, got compiled=%v %v %v", c.src, compiled, gotC, errC)
		}
		requireCompileDefect(t, c.src, gotC, errC)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: the interpreter answers %s, got %v %v", c.src, c.want, gotI, errI)
		}
	}
	// Negative: where the latch's count is absorbed — the program's residual,
	// an arm whose out IS the do — and where the raise is certain (a static
	// zero division is `raise`'s twin: one Error, never the phantom), the
	// program compiles with parity.
	for _, src := range []string{
		`do [1 drop]`,
		`do [1 drop] 2`,
		`2 do [1 drop]`,
		`if true [do [1 drop]] [3]`,
		`(do [1 0 div]).code`,
		`do [1 0 div] error [dot code]`,
		`if [true] [do [raise 'x'] 2] [3]`,
	} {
		requireEngineParity(t, src, true)
	}
	// A certain raise ends the body: a def after it never happens.
	src := `do [(0 div 0) def q 5] end q`
	_, errI := mustNew(t).RunInterp(src)
	_, _, _, errC := mustNew(t).CompileCheck(src)
	if codeOf(errI) != "undefined_word" || (errC != nil && !strings.Contains(errC.Error(), "q")) {
		t.Errorf("%s: the def after a certain raise is never made, got interp %v, check %v", src, errI, errC)
	}
}
