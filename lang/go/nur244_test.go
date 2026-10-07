package lang

import "testing"

// TestNUR244BranchBoundFnIsSpeculative pins NUR244's close: a fn def in a
// branch arm that may not run — the arm a DECIDED condition skips (a
// literal, a def-bound or folded Boolean), or an else-less if's arm — is a
// speculative family, so a call past the arms resolves at run time:
// undefined_word on the path that skipped the arm, the fn where it ran. The
// join used to keep the arm's fn value and the call ran it. The arm a
// decided condition takes stays exact, and an outer binding the skipped arm
// would have replaced is the one called.
func TestNUR244BranchBoundFnIsSpeculative(t *testing.T) {
	// Since phase 2 (design/IMMUTABLE-DEF.1.md §2.1) an arm is a BLOCK: a fn
	// defined only in an arm ends with it, so the call past the arms is
	// undefined_word on EVERY path (taken or not) and an outer binding the
	// arm would have replaced is the one called — the shadow ends with the
	// arm. The compiled lane agrees or declines (the check stop for the
	// fresh family, the block gate for the shadow). The speculative family
	// NUR244 closed is mooted; phase 4 retires it.
	const f7 = `def f fn [[a:Integer] [Any] [7]]`
	for _, r := range []struct{ src, want string }{
		{`if false [` + f7 + `] [] end 3 f`, "ERROR:undefined word: f"},
		{`def c false if c [def p (fn [[a:Integer] [Any] [7]])] [] end 3 p`, "ERROR:undefined word: p"},
		{`if (1 gt 2) [` + f7 + `] [] end 3 f`, "ERROR:undefined word: f"},
		{`if true [] [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`if false [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`if [false] [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`def m {e: false} if (m "e" get) [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`def m {e: true} if (m "e" get) [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`if true [` + f7 + `] [] end 3 f`, "ERROR:undefined word: f"},
		{`if [true] [` + f7 + `] end 3 f`, "ERROR:undefined word: f"},
		{`def f fn [[a:Integer] [Any] [1]] if false [` + f7 + `] [] end 3 f`, "[1]"},
		{`def f fn [[a:Integer] [Any] [1]] if true [` + f7 + `] [] end 3 f`, "[1]"},
		// The intended spelling binds the branch VALUE: the fn the taken arm
		// leaves, called after the branch.
		{`def f (if false [fn [[a:Integer] [Any] [7]]] [fn [[a:Integer] [Any] [8]]]) end 3 f`, "[8]"},
		{`def f (if true [fn [[a:Integer] [Any] [7]]] [fn [[a:Integer] [Any] [8]]]) end 3 f`, "[7]"},
	} {
		ruleOrDecline(t, r.src, r.want)
	}

	// A parser NAME and a `/v` value read of the arm's fn: unbound after the
	// arm on the interpreter (the parse reads a registered kind, NUR109's
	// rule); the compiled lane declines.
	for _, r := range []struct{ src, interp string }{
		{`import "boru:parselang" def c false if c [def p (fn [[source:String opts:Map] [Any] [7]])] [] end parse p 'x'`, "parse_unknown_lang"},
		{`if false [` + f7 + `] [] end f/v`, "undefined_word"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled {
			t.Errorf("%s: want a decline, got %v compiled=%v err=%v", r.src, gotC, compiled, errC)
		}
		if _, errI := mustNew(t).RunInterp(r.src); codeOf(errI) != r.interp {
			t.Errorf("%s: interp got [%s], want %s", r.src, codeOf(errI), r.interp)
		}
	}
}
