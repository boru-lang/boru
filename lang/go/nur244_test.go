package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR244BranchBoundFnIsSpeculative pins NUR244's close: a fn def in a
// branch arm that may not run — the arm a DECIDED condition skips (a
// literal, a def-bound or folded Boolean), or an else-less if's arm — is a
// speculative family, so a call past the arms resolves at run time:
// undefined_word on the path that skipped the arm, the fn where it ran. The
// join used to keep the arm's fn value and the call ran it. The arm a
// decided condition takes stays exact, and an outer binding the skipped arm
// would have replaced is the one called.
func TestNUR244BranchBoundFnIsSpeculative(t *testing.T) {
	const f7 = `def f fn [[a:Integer] [Any] [7]]`
	for _, r := range []struct{ src, want string }{
		{`if false [` + f7 + `] [] end 3 f`, "ERROR:undefined word: f"},
		{`def c false if c [def p (fn [[a:Integer] [Any] [7]])] [] end 3 p`, "ERROR:undefined word: p"},
		{`if (1 gt 2) [` + f7 + `] [] end 3 f`, "ERROR:undefined word: f"},
		{`if true [] [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`if false [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`if [false] [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`def m {e: false} if (m "e" get) [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`def m {e: true} if (m "e" get) [` + f7 + `] end 3 f`, "[7]"},
		{`if true [` + f7 + `] [] end 3 f`, "[7]"},
		{`if [true] [` + f7 + `] end 3 f`, "[7]"},
		{`def f fn [[a:Integer] [Any] [1]] if false [` + f7 + `] [] end 3 f`, "[1]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// The READS no op resolves at run time decline rather than answer with
	// the arm's value: a parser NAME, which the interpreter resolves as a
	// registered kind when the arm did not run (NUR109's rule — the parse
	// reads the binding as a value), and a `/v` value read: neither has a
	// live home.
	for _, r := range []struct{ src, reason, interp string }{
		{`import "boru:parselang" def c false if c [def p (fn [[source:String opts:Map] [Any] [7]])] [] end parse p 'x'`, "no live home", "parse_unknown_lang"},
		{`if false [` + f7 + `] [] end f/v`, "no live home", "undefined_word"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), r.reason) {
			t.Errorf("%s: want a %s decline, got %v compiled=%v err=%v", r.src, r.reason, gotC, compiled, errC)
		}
		if _, errI := mustNew(t).RunInterp(r.src); codeOf(errI) != r.interp {
			t.Errorf("%s: interp got [%s], want %s", r.src, codeOf(errI), r.interp)
		}
	}
}
