package lang

import (
	"fmt"
	"strings"
	"testing"
)

// top_level_apply_test.go pins the dynamic-lead group (2026-09-22): the
// `apply` WORD at the MAIN program over a lead the check cannot type as a
// concrete fn. Two leads, two mechanisms, both of which were unit-only
// before:
//
//   - a GRADUAL lead (a map member read, a fetched export) with one value
//     beneath it records the apply EVENT (recordGradualApplyEvent →
//     OpCallDynApplyOne, the twenty-seventh increment's op) at the program
//     level too — it commits one result or defers, exactly as in a unit;
//   - a produced fn-typed CARRIER (a declared-Function factory's result,
//     `5 (mk 1)/v apply`) registers the program unit's PENDING apply, which
//     Finalize lowers as the whole-residual OpCallDynApplyTop — the word's
//     own semantics over every value beneath: a closure of the window's
//     arity runs VM-native, a 0-arg one fires above the untouched window,
//     a wider one under-applies through the interpreter's re-step.

const tlaMk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]]  `

// TestTopLevelApplyParity: every row compiles, runs VM-native for the
// window's own arity, and agrees with the interpreter.
func TestTopLevelApplyParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// the gradual lead: callbacks L50/L51 and module-composition L100
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  5 (m 'f' get) apply`, "6", "a fetched map member applied at the top level"},
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  5 m.f/v apply`, "6", "the /v member read"},
		{`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def m {f: M.inc/v} end 5 (m 'f' get) apply`, "6", "a module export stored in a map"},
		{`def m {f: ([a:Integer] => [a add 1])}  5 m.f/v apply`, "6", "a lambda member"},
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  7 5 m.f/v apply`, "7 6", "a deeper value survives beneath the receiver"},
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  5 m.f/v apply add 10`, "16", "the event's result feeds a later word"},
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  def r (5 m.f/v apply) end r`, "6", "the event's result def-bound"},
		// the produced carrier: callbacks L40/L41 and the arity family
		{tlaMk + `5 (mk 1)/v apply`, "6", "a /v-parked factory result under apply"},
		{`def adder fn [[k:Integer][Function][([n:Integer] => [n add k])]]  10 (adder 3) apply`, "13", "the bare factory result"},
		{tlaMk + `7 5 (mk 1)/v apply`, "7 6", "a 1-arg closure over a 2-wide window takes the top value"},
		{`def mk2 fn [[k:Integer][Function][([a:Integer b:Integer] => [(a add b) add k])]]  7 5 (mk2 1)/v apply`, "13", "a 2-arg closure over a 2-wide window runs VM-native"},
		{`def mk0 fn [[k:Integer][Function][([] => [k])]]  5 (mk0 1)/v apply`, "5 1", "a 0-arg closure fires above the untouched window"},
		{`def mk0 fn [[k:Integer][Function][([] => [k])]]  7 5 (mk0 1)/v apply`, "7 5 1", "the same over a 2-wide window"},
		// the former produced_closure_apply_test.go sound-failure rows: the
		// values beneath match no signature and the interpreter parks the
		// closure as data — the apply word's re-step does the same
		{`def kk x:Integer => [y:Integer => [x add y]] end "s" (kk 7) apply`, "s fn (Integer)", "a concrete produced closure over a value its param rejects: parked"},
		{`def mk fn [[x:Integer][Function][(fn [[y:Integer][Integer][x add y]])]] end "s" (mk 7) apply`, "s fn (Integer)", "the carrier twin: parked"},
		{`def mk fn [[x:Integer][Function][(fn [[y:Integer][Integer][x add y]])]] end 1 99 (mk 7) apply`, "1 106", "a 1-arg closure carrier over a 2-wide window under-applies"},
		{`def mk fn [[x:Integer][Function][(fn [[y:Integer][Integer][x add y]])]] end 1 "s" (mk 7) apply`, "1 s fn (Integer)", "parked over a 2-wide window"},
		// the former val_read_alias_test.go sound-failure row: a def-bound
		// produced closure's /v read applied over a value its param rejects
		{`def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end 'x' p/v apply`, "x fn p(Integer)", "a /v-read alias of a def-bound closure: parked under its def name"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestTopLevelApplyNoMatchParity: a lead that is no fn raises the
// interpreter's own `apply` no-match on both lanes.
func TestTopLevelApplyNoMatchParity(t *testing.T) {
	src := `def m {f: 42}  5 m.f/v apply`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if compiled {
		requireParity(t, src, gotC, errC, gotI, errI)
	}
	if codeOf(errI) != "signature_error" || (compiled && codeOf(errC) != "signature_error") {
		t.Errorf("%q: interp err=[%s] compiled err=[%s], want signature_error on both", src, codeOf(errI), codeOf(errC))
	}
}

// TestTopLevelGradualApplyDefers: the event form commits ONE result, so a
// gradual lead whose runtime fn is not 1-arg defers — loud, never a value of
// its own (TestGradualApplyDefers' contract, now at the program level too).
func TestTopLevelGradualApplyDefers(t *testing.T) {
	src := `def m {f: (fn [[a:Integer b:Integer][Integer][a add b]])}  3 5 m.f/v apply`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	if codeOf(errI) != "" || fmt.Sprint(gotI) != "[8]" {
		t.Fatalf("interpreter oracle moved: %v err=[%s]", gotI, codeOf(errI))
	}
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if errC == nil || len(gotC) != 0 {
		t.Errorf("%q: compiled %v err=[%v], want a loud defer — the one-result model cannot carry a 2-arg apply", src, gotC, errC)
	}
}

// TestTopLevelApplySoundCompileFailures: the neighbours that still DECLINE,
// each with the interpreter's own answer beside it.
func TestTopLevelApplySoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// nothing beneath the closure: the interpreter leaves it as data
		// the pending apply is not the program's tail: a later word consumes it
		{tlaMk + `5 (mk 1)/v apply add 10`, "NUR124", "[16]"},
		// a def-bound factory result read back: the read's statement window
		{tlaMk + `def p (mk 1) end 5 p/v apply`, "statement", "[6]"},
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
