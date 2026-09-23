package lang

import (
	"fmt"
	"strings"
	"testing"
)

// def_bound_closure_park_test.go pins NUR181 (2026-09-23): a call result is
// PARKED where it lands, on the compiled lane too, whatever route the call
// took. A shaped method call over a def-bound factory closure (`2 r 3`, the
// FnShapes claim's CALL_DYN_METHOD) and a compiled fn-value apply of one
// (`2 ((mk3 1) 3)`, the paren's KEEPQ / produced-lead applies) are user calls
// by another route — the callee is a boru closure and fnReturnPark parks what
// it returns — but callResultPlaced knew only a named user call and a user
// MEMBER's method call, so the trailing arm applied the returned closure to
// the value beneath (6 for `[2 fn]`), and the mixed windows islanded it live
// (`7 (r 3) 2` was `[7 6]`). The recorder now resolves the dyn-method's
// callee unit from the method value's own producer, a fn-value apply's from
// its closure operand, and the program residual's ordering treats a parked
// result as the data it is.
//
// NUR185 (2026-09-23, found probing NUR184's neighbours; present on main):
// a `/v` READ of a def-bound closure is placed too — `def c (mk 3) end 2
// c/v 10` is `[2 fn c(Integer) 10]` interpreted, and the residual's mixed
// window islanded it and applied the closure (`[2 30]`). The carrier read
// notes the value spelling now (NoteValRead on the fn-carrier side table),
// callResultPlaced treats a `/v`-only delivery as the parked result it is,
// and the rows decline at the render gate (the interpreter names the value
// after the def).

const dbMk3 = `def mk3 fn [[a:Integer][Function][( fn [[b:Integer][Function][( fn [[c:Integer][Integer][a add b add c]] )]] )]]  `
const dbMk = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  `

// TestDefBoundClosureParkParity: every row compiles and agrees with the
// interpreter, the returned closure parked where it landed.
func TestDefBoundClosureParkParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// NUR181's own rows: the def's collection binds r to mk3's closure
		// and leaves the 2 beneath; `r 3` nets a closure the interpreter parks
		{dbMk3 + `def r ((mk3 1) 2) end r 3`, "[2 fn (Integer)]", "the shaped method call's result parks above the leftover"},
		{dbMk3 + `def r ((mk3 1) 2) end (r 3)`, "[2 fn (Integer)]", "the same in a paren"},
		{dbMk3 + `def r (mk3 1) end 2 r 3`, "[2 fn (Integer)]", "the literal written before the call"},
		{dbMk3 + `def r (mk3 1) end 2 (r 3)`, "[2 fn (Integer)]", "the same in a paren"},
		{dbMk3 + `def r (mk3 1) end 7 (r 3) 2`, "[7 fn (Integer) 2]", "the mixed window no longer islands a parked result"},
		{dbMk3 + `def r ((mk3 1) 2) end r 3 4`, "[2 fn (Integer) 4]", "the trailing window either"},
		{dbMk3 + `def r (mk3 1) end (r 3) 2`, "[fn (Integer) 2]", "a value after the call (as before)"},
		// a compiled fn-value apply's result parks the same way
		{dbMk3 + `2 ((mk3 1) 3)`, "[2 fn (Integer)]", "the paren's apply of a factory result"},
		{dbMk3 + `def r (mk3 1) end 2 ((r 3))`, "[2 fn (Integer)]", "a double paren over the method call"},
		// a closure returning DATA is unaffected: the value seats
		{dbMk + `def r (mk 3) end 2 r 5`, "[2 15]", "a data result seats above the literal"},
		{dbMk + `def r (mk 3) end 2 (r 5)`, "[2 15]", "the same in a paren"},
		{dbMk + `def c (mk 3) end c 10`, "[30]", "a bare read of a def-bound closure dispatches (NUR185's control)"},
		// a named factory's parked result above a literal seats too (it
		// declined "call result above a literal" before)
		{dbMk + `5 (mk 3)`, "[5 fn (Integer)]", "an unclaimed parked result seats as data"},
		{dbMk + `5 (mk 3) 7`, "[5 fn (Integer) 7]", "and under a later literal (as before)"},
		// a `word` splice RE-STEPS its payload against the live stack: a
		// parked closure wrapped by the marker dispatches where it expands
		// (the generated sweep's `word` × factory seed, which the ordering
		// change would otherwise have seated as `[5 fn]`: Engine.markReStepped)
		{`def mk fn [[][Function][([n:Integer] => [n add 1])]] end def dbl word (mk) end 5 dbl`, "[6]", "a spliced parked closure applies at the expansion"},
		{`def mk fn [[][Function][([n:Integer] => [n add 1])]] end 5 word (mk)`, "[6]", "the same inline"},
		{`def mk fn [[][Function][([n:Integer] => [n add 1])]] end def dbl word [(mk)] end 5 dbl`, "[5 fn (Integer)]", "a LIST payload's elements are data on both lanes"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestDefBoundClosureParkSoundCompileFailures: the neighbours that still
// DECLINE, each with the interpreter's own answer beside it — a bare read
// of the def-bound closure (the read's statement window) and the shuffles
// the interpreter re-steps at the shuffle (NUR124's timing axis, open at
// the main program).
func TestDefBoundClosureParkSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		{dbMk3 + `def r ((mk3 1) 2) end r`, "statement ends short", "[fn (Integer)]"},
		{dbMk + `5 (mk 3) 1 roll`, "unknown provenance", "[15]"},
		{dbMk + `5 (mk 3) swap`, "did not collapse", "[15]"},
		// NUR185: a `/v` read of a def-bound closure is placed, never islanded
		{dbMk + `def c (mk 3) end 2 c/v 10`, "unconsumed fn-value carrier", "[2 fn c(Integer) 10]"},
		{dbMk + `def c (mk 3) end 2 c/v 10 20`, "unconsumed fn-value carrier", "[2 fn c(Integer) 10 20]"},
		{dbMk + `def c (mk 3) end c/v 10`, "unconsumed fn-value carrier", "[fn c(Integer) 10]"},
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
