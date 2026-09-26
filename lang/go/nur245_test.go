package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR245BothArmsDefineOneFn pins NUR245's close for arms that agree on
// the fn's shape: a fn family both arms of a branch define is joined to a
// MODEL the pass types the call against — the running arm's fn when the
// condition is decided, the then arm's otherwise — where it used to be a
// payload-less carrier no call past the merge could dispatch
// ("unconsumed fn-value carrier in residual"). The family's dispatches
// route live, so the running arm's binding picks the body: a decided
// condition's running arm is an ordinary install, noted as a taken arm's
// (its bind twin keeps the live registry current), and an undecided one's
// other arm has its unit compiled where it is placed, under the family's
// name.
func TestNUR245BothArmsDefineOneFn(t *testing.T) {
	const (
		f7   = `def f fn [[a:Integer] [Any] [7]]`
		f8   = `def f fn [[a:Integer] [Any] [8]]`
		mOff = `def m {e: false} `
		mOn  = `def m {e: true} `
	)
	for _, r := range []struct{ src, want string }{
		// Decided: the running arm's fn, whichever arm it is.
		{`if false [` + f7 + `] [` + f8 + `] end 3 f`, "[8]"},
		{`if true [` + f7 + `] [` + f8 + `] end 3 f`, "[7]"},
		{`def c false if c [` + f7 + `] [` + f8 + `] end 3 f`, "[8]"},
		{`if (1 gt 2) [` + f7 + `] [` + f8 + `] end 3 f`, "[8]"},
		{`if [false] [` + f7 + `] [` + f8 + `] end 3 f`, "[8]"},
		// Undecided: whichever arm ran.
		{mOff + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end 3 f`, "[8]"},
		{mOn + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end 3 f`, "[7]"},
		{mOff + `if (m 'e' get) [` + f7 + `] [` + f8 + `] end [(f 3) (f 4)]`, "[[8 8]]"},
		{`def g fn [[b:Boolean] [Any] [if b [` + f7 + `] [` + f8 + `] 3 f]] end [(g true) (g false)]`, "[[7 8]]"},
		{mOff + `if (m 'e' get) [def f fn [[a:Integer] [Integer] [a add 7]]] [def f fn [[a:Integer] [Integer] [a add 8]]] end (f 3) mul 2`, "[22]"},
		// The other arm's unit enforces its contract under the family's name.
		{mOff + `if (m 'e' get) [def f fn [[a:Integer] [Integer] [a add 7]]] [def f fn [[a:Integer] [Integer] [a add 8 'x']]] end f 3`, "ERROR:f: expected 1 return value(s), got 2"},
		{mOff + `if (m 'e' get) [def f fn [[a:Integer] [Integer] [a add 7]]] [def f fn [[a:Integer] [Integer] ['x']]] end f 3`, "ERROR:f: return value 1: expected Integer"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// Arms that disagree on the fn's shape have no one model to type the
	// call against: the payload-less join stands, and its call declines.
	src := mOff + `if (m 'e' get) [` + f7 + `] [def f fn [[a:String] [Any] [8]]] end 3 f`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !noteCompileDefect(t, src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "unconsumed fn-value carrier") {
		t.Errorf("%s: want the standing decline, got %v compiled=%v err=%v", src, gotC, compiled, errC)
	}
	if _, errI := mustNew(t).RunInterp(src); codeOf(errI) != "signature_error" {
		t.Errorf("%s: interp got [%s], want signature_error", src, codeOf(errI))
	}
}
