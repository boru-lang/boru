package lang

import (
	"fmt"
	"testing"
)

// TestFnValueCallbackHasOwnArgs pins NUR166's close. A fn VALUE applied by
// a higher-order word opens the frame its definition declares, with the
// value's own call args as `args` — the interpreter's dispatch does that for
// a named fn value and for a lambda alike. The compiled lane lowered the
// value's body as the call site's closure unit and entered it through the
// token seam, which pushed no args list, so a body that reads `args` at run
// time (a `do` body inside it, DynEnv) read the ENCLOSING frame's — none at
// the top level. applyClosure now brackets a unit that carries a param
// contract (a fn value's body; a quotation body has none and runs in the
// caller's frame) with pushRootArgs, as the fn-value seam brackets its own.
func TestFnValueCallbackHasOwnArgs(t *testing.T) {
	for _, src := range []string{
		"def g fn [[n:Integer] [Any] [do [args] size]] end each g/v [1 2]",
		"def g fn [[n:Integer] [Any] [do [args]]] end each g/v [1 2]",
		"def g fn [[n:Integer] [Any] [do [args.0]]] end each g/v [1 2]",
		"def g fn [[Integer] [Any] [do [args]]] end each g/v [1 2]",
		"def g ([n:Integer] => [do [args]]) end each g/v [1 2]",
		"each ([n:Integer] => [do [args]]) [1 2]",
		"def g fn [[n:Integer] [Any] [args]] end each g/v [1 2]",
		"def g fn [[n:Integer] [Any] [do [args]]] end [1 2] each [g]",
		"def h fn [[a:Integer] [Any] [do [args]]] end def w fn [[x:Integer] [Any] [each h/v [x 5]]] end w 7",
		"def w fn [[x:Integer] [Any] [each [do [args]] [x 5]]] end w 7",
		"each [do [args]] [1 2]",
		"def g fn [[n:Integer] [Any] [do [args]]] end each g/v [1 2] do [args]",
	} {
		requireSameVerdict(t, src)
	}
	for _, c := range []struct{ src, want string }{
		{"def g fn [[n:Integer] [Any] [do [args] size]] end each g/v [1 2]", "[[1 1]]"},
		{"def g fn [[n:Integer] [Any] [do [args]]] end each g/v [1 2]", "[[[1] [2]]]"},
		{"def g ([n:Integer] => [do [args]]) end each g/v [1 2]", "[[[1] [2]]]"},
		{"def w fn [[x:Integer] [Any] [each [do [args]] [x 5]]] end w 7", "[[[7] [7]]]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}
