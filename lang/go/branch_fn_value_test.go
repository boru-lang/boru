package lang

import (
	"fmt"
	"strings"
	"testing"
)

// branch_fn_value_test.go pins NUR159 (recorded 2026-09-18 by the generated
// sweep's `if` × named-fn cell, FIXED 2026-09-23) and NUR187 (found and
// fixed the same day): a branch whose arm is a fn VALUE returns a value the
// interpreter RE-STEPS — a named 0-arg fn fires (`if true one/v [2]` is 1),
// an arg-taking one takes the values beneath it (`7 if true inc/v [2]` is
// 8) or the token after it (`if true inc/v [2] 5` is 6), and nothing to
// take leaves it as data — where the compiled lane laid the merged value
// out as data: the merge widened the fn arm's type to Word, so no static
// fn test saw it. The recorder's branch event carries the fact (MayBeFn,
// with an arg-taking flag): the check pass notes the re-step landing on
// the merged value, the lowering lands it right after the merge, the
// residual's trailing and mixed arms admit the unsettled value, and the
// collapse's park and re-step marks ask the same seam. NUR187 — the
// residual's apply arms carried a fn value's collection across a statement
// boundary (`7 m.f ; 3` islanded to `[7 4]` for the interpreter's `[8 3]`,
// `m.f ; 5` applied for `[fn 5]`) — is closed by a positional note of every
// boundary the pass steps: an entry written past a boundary that follows
// the value is a later statement's, and no arm applies the value over it.

const bfOne = `def one fn [[][Integer][1]] end `
const bfInc = `def inc fn [[n:Integer][Integer][n add 1]] end `
const bfM = bfInc + `def m {f: inc/v} `

// TestBranchFnValueAgrees: every row compiles and agrees with the
// interpreter.
func TestBranchFnValueAgrees(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// A named 0-arg fn fires at the merge (the sweep's cell).
		{bfOne + `if true one/v [2]`, "[1]", "the sweep's row: compiled fn one on main"},
		{bfOne + `if false [2] one/v`, "[1]", "the else arm"},
		{bfOne + `if false one/v [2]`, "[2]", "the data arm taken"},
		{bfOne + `7 if true one/v [2]`, "[7 1]", "under a value"},
		{bfOne + `if true one/v [2] 5`, "[1 5]", "a 0-arg fn collects nothing"},
		{bfOne + `if true one/v [2] ; 3`, "[1 3]", "before a boundary"},
		{bfOne + `(if true one/v [2])`, "[1]", "inside a paren"},
		{bfOne + `7 (if true one/v [2])`, "[7 1]", "inside a paren under a value"},
		{bfOne + `[if true one/v [2]]`, "[[1]]", "a list literal's only element"},
		{bfOne + `def r (if true one/v [2]) r`, "[1]", "bound by def, read bare"},
		{bfOne + `def r (if true one/v [2]) 7 r`, "[7 1]", "bound by def, read under a value"},
		{bfOne + `def r (if true one/v [2]) r/v`, "[1]", "bound by def, read inert"},
		{bfOne + `def g fn [[][Any][if true one/v [2]]] end (g)`, "[1]", "a fn body's tail"},
		{bfOne + `def g fn [[m:Integer][Any][if true one/v [2]]] end (g 7)`, "[1]", "a fn body's tail over a param"},
		{bfOne + `def xs [1 2] xs each [if true one/v [2]]`, "[[1 1]]", "a code body"},
		{`if true ([] => [1]) [2]`, "[fn]", "a 0-arg LAMBDA parks as data (the anonymous park)"},
		// An arg-taking fn takes the values beneath or the token after.
		{bfInc + `if true inc/v [2]`, "[fn inc(Integer)]", "nothing to take: data"},
		{bfInc + `if true inc/v [2] 5`, "[6]", "the token after (the lead arm, as before)"},
		{bfInc + `7 if true inc/v [2]`, "[8]", "the value beneath (compiled [7 fn inc] on main)"},
		{bfInc + `7 if false inc/v [2]`, "[7 2]", "the data arm under a value"},
		{bfInc + `1 7 if true inc/v [2]`, "[1 8]", "two beneath: the top one"},
		{bfInc + `7 if true inc/v [2] 10`, "[7 11]", "the token after wins over the value beneath"},
		{bfInc + `7 if true inc/v [2] "s"`, "[8 s]", "a token the param rejects: the value beneath"},
		{bfInc + `7 if true [2] inc/v`, "[7 2]", "the fn in the else arm, not taken"},
		{bfInc + `7 if (1 lt 2) inc/v [2]`, "[8]", "a computed condition"},
		{bfInc + `if true inc/v [2] ; 5`, "[fn inc(Integer) 5]", "a boundary: the 5 is the next statement's (compiled 6 on main)"},
		{bfInc + `def c true if c inc/v [2] ; 5`, "[fn inc(Integer) 5]", "the same under a bound condition"},
		{bfInc + `(7 if true inc/v [2])`, "[8]", "inside a paren"},
		{bfInc + `7 (if true inc/v [2])`, "[7 fn inc(Integer)]", "a paren PLACES the lone survivor"},
		{bfInc + `7 ((if true inc/v [2]))`, "[7 fn inc(Integer)]", "and a paren around that places it again"},
		{bfInc + `(7 (if true inc/v [2]))`, "[8]", "an enclosing paren re-steps the placed value over the 7"},
		{bfInc + `(7 ((if true inc/v [2])))`, "[8]", "at any depth"},
		{bfInc + `def xs [1 2] xs each [if true inc/v [2]]`, "[[2 3]]", "a code body over its element (compiled [fn fn] on main)"},
		{bfInc + `def xs [1 2] xs each [if true inc/v [2] 5]`, "[[6 6]]", "a code body with the token after"},
		{bfInc + `do [7 if true inc/v [2]]`, "[8]", "a do body"},
		{bfInc + `7 do [if true inc/v [2]]`, "[8]", "a do body over the caller's value"},
		// A factory's closure in the arm: a recovered 0-arg shape is settled
		// (the anonymous park), a 1-arg shape takes the value beneath, and a
		// shape the pass cannot recover is decided at run time — the trailing
		// apply islands the window as written, so a 0-arg lambda parks over
		// the 7 and a 1-arg one takes it.
		{`7 def mk fn [[][Function][([] => [1])]] end if true (mk) [2]`, "[7 fn]", "the sweep's prefix-stack variant (compiled [fn 7] mid-increment)"},
		{`def mk fn [[][Function][([] => [1])]] end def g fn [[][Any][if true (mk) [2]]] end (g)`, "[fn]", "the sweep's fn-body variant"},
		{`def mk1 fn [[][Function][([n:Integer] => [n add 1])]] end 7 if true (mk1) [2]`, "[8]", "a factory's 1-arg closure takes the value beneath"},
		{`def mk fn [[b:Boolean][Function][if b [([] => [1])] [([n:Integer] => [n add 1])]]] end 7 if true (mk true) [2]`, "[7 fn]", "an unrecoverable shape, 0-arg at run time"},
		{`def mk fn [[b:Boolean][Function][if b [([] => [1])] [([n:Integer] => [n add 1])]]] end 7 if true (mk false) [2]`, "[8]", "an unrecoverable shape, 1-arg at run time"},
		{`def mk fn [[b:Boolean][Function][if b [([] => [1])] [([n:Integer] => [n add 1])]]] end 1 7 if true (mk true) [2]`, "[1 7 fn]", "the same under two values"},
		{`def mk fn [[b:Boolean][Function][if b [([] => [1])] [([n:Integer] => [n add 1])]]] end if true (mk true) [2] 5`, "[fn 5]", "the same with the token after"},
		// NUR187: the member-read twin of the boundary.
		{bfM + `m.f ; 5`, "[fn inc(Integer) 5]", "a member read before a boundary stays data (compiled 6 on main)"},
		{bfM + `7 m.f`, "[8]", "the trailing apply, as before"},
		{bfM + `1 7 m.f`, "[1 8]", "the trailing window, as before"},
		{bfM + `3 m.f 2`, "[3 3]", "the mixed island, as before"},
		{bfM + `7 m.f
3`, "[7 4]", "a newline is not a boundary: the fn collects the 3"},
		{`def zero fn [[][Integer][0]] end def m {z: zero/v} m.z ; 5`, "[0 5]", "a 0-arg member's landing before a boundary"},
		{`def zero fn [[][Integer][0]] end def m {z: zero/v} 7 m.z ; 5`, "[7 0 5]", "under a value"},
		{`def mk fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]] (mk 1) ; 5`, "[fn (Integer) 5]", "a parked closure before a boundary"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s): %v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestBranchFnValueErrorParity: a row that raises identically on both lanes
// — the interpreter's own no-match at the merge (`uncalled_function`).
func TestBranchFnValueErrorParity(t *testing.T) {
	for _, src := range []string{
		bfInc + `"s" if true inc/v [2]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
			continue
		}
		if errI == nil {
			t.Errorf("%q: the interpreter must raise, got %v", src, gotI)
		}
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestBranchFnValueSoundCompileFailures: what is left DECLINES, each row
// with the interpreter's own answer beside it — a later dispatch that
// collected the branch result the interpreter re-steps first, an
// arg-taking result interior to the residual past a boundary, a list
// literal's element with siblings, a fn body's residual over a param, a
// def-bound read (the interpreter installs the fn under the name and
// dispatches the name as a word), and NUR187's member-read twins.
func TestBranchFnValueSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		{bfInc + `7 if true inc/v [2] add 1`, "the interpreter re-steps first (NUR159)", "[9]"},
		{bfInc + `7 if true inc/v [2] mul 2`, "the interpreter re-steps first (NUR159)", "[16]"},
		{bfInc + `7 if true inc/v [2] typeof`, "the interpreter re-steps first (NUR159)", "[Integer]"},
		{bfInc + `7 if true inc/v [2] eq 8`, "the interpreter re-steps first (NUR159)", "[true]"},
		{bfOne + `7 if true one/v [2] add 1`, "the interpreter re-steps first (NUR159)", "[7 2]"},
		{bfOne + `if true one/v [2] add 1`, "the interpreter re-steps first (NUR159)", "[2]"},
		{bfInc + `7 if true inc/v [2] ; 3`, "precedes residual args (NUR159)", "[8 3]"},
		{bfInc + `1 7 if true inc/v [2] ; 3`, "precedes residual args (NUR159)", "[1 8 3]"},
		{bfInc + `[7 if true inc/v [2]]`, "unknown provenance", "[[8]]"},
		{bfInc + `[if true inc/v [2] 5]`, "unknown provenance", "[[6]]"},
		{bfInc + `def g fn [[m:Integer][Any][m if true inc/v [2]]] end (g 7)`, "sits in the residual (NUR159)", "[8]"},
		{bfInc + `def x (if true inc/v [2]) x 5`, "bound to a branch result whose fn arm takes arguments", "[6]"},
		{bfInc + `def x (if true inc/v [2]) 7 x`, "bound to a branch result whose fn arm takes arguments", "[8]"},
		{bfInc + `def x (if true inc/v [2]) 7 x/v`, "bound to a branch result whose fn arm takes arguments", "[7 fn x(Integer)]"},
		{bfInc + `def g fn [[][Any][def x (if true inc/v [2]) x 5]] end (g)`, "bound to a branch result whose fn arm takes arguments", "[6]"},
		{bfM + `7 m.f ; 3`, "dynamic value precedes residual args", "[8 3]"},
		{bfM + `1 7 m.f ; 3`, "dynamic value precedes residual args", "[1 8 3]"},
		{bfM + `7 m.f end 3`, "dynamic value precedes residual args", "[8 3]"},
		{bfM + `7 (m get f/q) ; 3`, "call result above a literal", "[7 fn inc(Integer) 3]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a compile failure", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}
