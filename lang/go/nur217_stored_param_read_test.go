package lang

import (
	"fmt"
	"testing"
)

// TestNUR217StoredFnParamReadIsTheWord pins NUR217's close. A bare read of a
// frame binding that holds a fn is a WORD dispatch on the interpreter (NUR123),
// whatever the param's declared type — and a fn value applied through a
// container member runs its STORED unit (storedfn$body), compiled once under
// the declared param types with no per-call re-analysis and no replay, so the
// read was a slot push: `def g fn [[f:Any] [Any] [f]] def m {g: g/v} m.g ([]
// => [42])` answered `fn f` compiled for the interpreter's 42. A stored unit
// whose body reads such a binding bare declines, and the apply takes the
// interpreter's own dispatch at the seam; the same fn applied by name, through
// a def or a module member already compiled per call and keeps doing so.
func TestNUR217StoredFnParamReadIsTheWord(t *testing.T) {
	const g = `def g fn [[f:Any] [Any] [f]] end def m {g: g/v} end `
	for _, c := range []struct{ src, want string }{
		{g + `m.g ([] => [42])`, "[42]"},
		{g + `def z fn [[] [Integer] [0]] end m.g (z/v)`, "[0]"},
		{`def g fn [[f:Function] [Any] [f]] end def m {g: g/v} end m.g ([] => [42])`, "[42]"},
		{`def g fn [[f:Any x:Integer] [Any] [x f]] end def m {g: g/v} end m.g ([n:Integer] => [n add 1]) 5`, "[6]"},
		{`def g fn [[f:Any] [Any] [[f]]] end def m {g: g/v} end m.g ([] => [42])`, "[[42]]"},
		// Data through the same unit answers as it always did.
		{g + `m.g 5`, "[5]"},
		// The per-call routes the fix leaves alone.
		{`import module [def g fn [[f:Any] [Any] [f]] export "M" {g: g/v}] end M.g ([] => [42])`, "[42]"},
		{`def g fn [[f:Any] [Any] [f]] end def h g/v end h ([] => [42])`, "[42]"},
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v, want %s", c.src, gotC, errC, c.want)
		}
	}
	// Negative: a 1-arg fn the bare read cannot call raises the word's
	// no-match on both lanes — never the value.
	src := g + `m.g ([n:Integer] => [n])`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	if codeOf(errI) != "signature_error" {
		t.Fatalf("%s: interpreter %v / %v, want the no-match", src, gotI, errI)
	}
	if !noteCompileDefect(t, src, gotC, errC) && (codeOf(errC) != codeOf(errI) || len(gotC) != 0) {
		t.Errorf("%s: compiled %v / %v, want the interpreter's %v", src, gotC, errC, errI)
	}
}
