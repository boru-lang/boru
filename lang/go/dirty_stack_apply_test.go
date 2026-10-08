package lang

import (
	"fmt"
	"strings"
	"testing"
)

// dirty_stack_apply_test.go pins NUR160 (2026-09-18, FIXED 2026-09-23): the
// `apply` word over a factory-built fn value did not fire on the compiled
// lane when a value sat below the operands — `7 5 (mk) apply` compiled
// `[7 5 fn]` for the interpreter's `[7 6]` — while the clean-stack `5 (mk)
// apply` agreed through the residual's 2-entry trailing arm. A factory whose
// body is a capture-free lambda (or a named fn's value) returns a baked fn
// CONST, not a closure, and the `apply` word's pending-application arm
// admitted only a produced closure (producedFnValue): with no pending entry
// the paren-shaped arms parked the carrier as data. The arm admits a
// produced fn const now (producedConstLambda), and the program's pending
// apply lowers over an EMPTY window too (programPendingApplyTop: a 0-arg
// closure fires, a wider one stays data), which closed two silent
// neighbours on main — `(mk0) apply` compiled `fn` for 1, `5 (mk0) apply`
// `[fn 5]` for `[5 1]`.

const dsMk = `def mk fn [[][Function][([n:Integer] => [n add 1])]] end `
const dsMk0 = `def mk fn [[][Function][([] => [1])]] end `
const dsMk2 = `def mk fn [[][Function][([a:Integer b:Integer] => [a sub b])]] end `
const dsInc = `def inc fn [[n:Integer][Integer][n add 1]] end def mk fn [[][Function][inc/v]] end `

// TestDirtyStackApplyParity: every row compiles and agrees with the
// interpreter.
func TestDirtyStackApplyParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{dsMk + `7 5 (mk) apply`, "[7 6]", "the sweep's row: a value beneath the operands"},
		{dsMk + `1 7 5 (mk) apply`, "[1 7 6]", "two beneath"},
		{dsMk + `5 (mk) apply`, "[6]", "the clean stack (as before)"},
		{dsMk + `7 (mk) apply`, "[8]", "one operand only"},
		{dsMk + `(mk) apply`, "[fn (Integer)]", "nothing beneath: no match, the value stays data"},
		{dsMk0 + `7 5 (mk) apply`, "[7 5 1]", "a 0-arg closure fires over nothing, above the untouched window"},
		{dsMk0 + `5 (mk) apply`, "[5 1]", "the same under one value (compiled [fn 5] on main)"},
		{dsMk0 + `(mk) apply`, "[1]", "and over an empty stack (compiled fn on main)"},
		{dsMk2 + `7 5 (mk) apply`, "[-2]", "a 2-arg closure takes both"},
		{dsInc + `7 5 (mk) apply`, "[7 6]", "a factory returning a NAMED fn's value (compiled [7 5 fn inc(Integer)] on main)"},
		{dsInc + `5 (mk) apply`, "[6]", "the named twin, clean stack (as before)"},
		{dsInc + `(mk) apply`, "[fn inc(Integer)]", "the named twin over nothing"},
		{`def z fn [[][Integer][7]] end def mk fn [[][Function][z/v]] end 5 (mk) apply`, "[5 7]", "a named 0-arg fn fires above the value"},
		{`def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end 7 5 (mk 1) apply`, "[7 6]", "a capturing closure took the pending arm already"},
		// The empty window over a produced CLOSURE — these three declined
		// "never dispatched" before the window admitted one entry (their
		// former homes: produced_closure_apply_test.go, top_level_apply_
		// test.go, val_read_alias_test.go).
		{`def kk x:Any => [y:Any => [x/v]] end (kk 7) apply`, "[fn (Any)]", "a produced closure over nothing stays data"},
		{`def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]]  (mk 1)/v apply`, "[fn (Integer)]", "a /v-parked factory result over nothing"},
		{`def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end p/v apply`, "[fn p(Integer)]", "a def-bound closure read by /v over nothing"},
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

// TestDirtyStackApplySoundCompileFailures: a value written AFTER the apply
// word re-steps the result into a dispatch the model cannot make (NUR124's
// standing decline), with the interpreter's answer beside it.
func TestDirtyStackApplySoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		{dsMk + `7 5 (mk) apply 9`, "NUR124", "[7 5 10]"},
		{dsMk + `5 (mk) apply 9`, "NUR124", "[5 10]"},
		{dsMk + `7 5 (mk) apply add 1`, "NUR124", "[7 7]"},
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
