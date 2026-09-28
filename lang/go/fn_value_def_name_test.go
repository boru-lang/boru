package lang

import (
	"fmt"
	"testing"
)

// TestDefBoundFnValueIsNamed pins NUR168's close and its two neighbours. The
// interpreter's installDef names EVERY fn value a `def` binds (`fnDef.Name =
// name`, closure or not); the compiled lane named only a produced CLOSURE at
// its promoted store, so a factory's capture-free lambda — baked as a const,
// `def f (mk 1)` — escaped as data anonymous (`each f/v [1 2 3]` read `fn
// (String)` for `fn f(String)`). Two more on the same value: the root's
// dyn-scope bind and global write-back stacked two entries under `f`, which
// Registry.Lookup unions (`do [f/v]` read `fn f(String) or (String)`), and a
// bare-word call of it inside a code body (`do [f 'z']`) lowered its callee
// read to the dispatch lookup, which defers on a fn definition (an internal
// error for the interpreter's `z`). The capturing twin never diverged on any
// of the three, which is why they hid.
func TestDefBoundFnValueIsNamed(t *testing.T) {
	const mk = "def mk fn [[k:Integer][Function][([s:String] => [s])]] end "
	for _, src := range []string{
		mk + "def f (mk 1) end each f/v [1 2 3]",
		mk + "def f (mk 1) end do [f/v]",
		mk + "def f (mk 1) end [(do [f/v])] join ','",
		mk + "def f (mk 1) end do [[f/v] join ',']",
		mk + "def f (mk 1) end (each f/v [1 2 3]) join ','",
		mk + "def f (mk 1) end def f (mk 2) end do [f/v]",
		mk + "def f (mk 1) end do [def f (mk 2) end f/v]",
		mk + "def f (mk 1) end do [f 'z']",
		mk + "def f (mk 1) end do [f 1]",
		mk + "def f (mk 1) end do [f 'z' f 'y']",
		mk + "def f (mk 1) end if true [f 'z'] []",
		mk + "def f (mk 1) end for 2 [f 'z']",
		mk + "def f (mk 1) end each [f] ['a' 'b']",
		mk + "def f (mk 1) end do [undef f 1]",
		mk + "def g fn [[][Any][def f (mk 1) end do [f 'z']]] end g",
		mk + "def use fn [[][Any][def f (mk 1) end f/v]] end use",
		"def mk fn [[][Function][([s:String] => [s])]] end def f (mk) end each f/v [1 2]",
		"def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end each f/v ['a']",
		"def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 1) end do [f/v]",
		"def f ([s:String] => [s]) end each f/v [1 2]",
		"def t [1 2] def t [3] do [t]",
	} {
		requireSameVerdict(t, src)
	}
	// The interpreter's own answers, so the parity above is parity with
	// the rule and not with a shared drift.
	for _, c := range []struct{ src, want string }{
		{mk + "def f (mk 1) end each f/v [1 2 3]", "[[fn f(String) fn f(String) fn f(String)]]"},
		{mk + "def f (mk 1) end do [f/v]", "[fn f(String)]"},
		{mk + "def f (mk 1) end def f (mk 2) end do [f/v]", "[fn f(String)]"},
		{mk + "def f (mk 1) end do [f 'z']", "[z]"},
		{mk + "def f (mk 1) end for 2 [f 'z']", "[z z]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled = %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
}
