package lang

import "testing"

// TestTypedDefUnifyErrorCaretIsTheToken pins NUR114: the rendered
// diagnostic — position AND caret width — agrees on both lanes; the
// compiled error carries the token text the parser stamped on the event's
// position (`def` underlined `^^^` at 1:1), as the interpreter's does.
func TestTypedDefUnifyErrorCaretIsTheToken(t *testing.T) {
	for _, src := range []string{
		"def x:(Integer gt 10) 5 x",
		"def x:(Integer gt 10) 50 x add 1",
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}
