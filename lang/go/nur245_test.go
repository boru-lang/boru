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

	// Arms that disagree on a parameter or return TYPE (NUR245's open half,
	// closed 2026-09-26): a decided condition's model is the running arm's
	// fn whatever the other declares; an undecided one's is WIDENED — each
	// type joined, no declaration site — so the pass types the call against
	// a slot either arm's value fits, and the routed op's live plan decides
	// the match: the running arm's unit answers, and a value it refuses
	// raises the interpreter's no-match.
	const (
		fS8 = `def f fn [[a:String] [Any] [8]]`
		und = `if (m 'e' get) [`
	)
	for _, r := range []struct{ src, want string }{
		{mOff + und + f7 + `] [` + fS8 + `] end 3 f`, "ERROR:cannot call `f`"},
		{mOn + und + f7 + `] [` + fS8 + `] end 3 f`, "[7]"},
		{mOff + und + f7 + `] [` + fS8 + `] end "x" f`, "[8]"},
		{mOn + und + f7 + `] [` + fS8 + `] end "x" f`, "ERROR:cannot call `f`"},
		{`if false [` + f7 + `] [` + fS8 + `] end "x" f`, "[8]"},
		{`if true [` + f7 + `] [` + fS8 + `] end 3 f`, "[7]"},
		{`if false [` + f7 + `] [` + fS8 + `] end 3 f`, "ERROR:cannot call `f`"},
		{`def g fn [[b:Boolean] [Any] [if b [` + f7 + `] [` + fS8 + `] "x" f]] end [(g false)]`, "[[8]]"},
		// Returns of different types: the downstream consumer sees the value.
		{mOff + und + `def f fn [[a:Integer] [Integer] [a add 1]]] [def f fn [[a:Integer] [String] ['s']]] end (3 f) typeof`, "[ProperString]"},
		{mOn + und + `def f fn [[a:Integer] [Integer] [a add 1]]] [def f fn [[a:Integer] [String] ['s']]] end (3 f) typeof`, "[Integer]"},
		// Each arm's own unit enforces its own contract, under the family's name.
		{mOn + und + `def f fn [[a:Integer] [Integer] ['x']]] [def f fn [[a:String] [Integer] [8]]] end 3 f`, "ERROR:f: return value 1: expected Integer"},
		{mOff + und + `def f fn [[a:Integer] [Integer] [7]]] [def f fn [[a:String] [Integer] ['y']]] end "s" f`, "ERROR:f: return value 1: expected Integer"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// Arms that disagree on the routed op's CLAIM — here the arity — have
	// no model that can stand for both: the payload-less join stands, and
	// its call declines.
	src := mOff + und + f7 + `] [def f fn [[a:String b:String] [Any] [8]]] end 3 f`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !noteCompileDefect(t, src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "unconsumed fn-value carrier") {
		t.Errorf("%s: want the standing decline, got %v compiled=%v err=%v", src, gotC, compiled, errC)
	}
	if _, errI := mustNew(t).RunInterp(src); codeOf(errI) != "signature_error" {
		t.Errorf("%s: interp got [%s], want signature_error", src, codeOf(errI))
	}
}
