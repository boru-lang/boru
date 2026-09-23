package lang

import (
	"fmt"
	"strings"
	"testing"
)

// quotation_body_member_read_test.go pins the container reads inside
// quotation bodies (2026-09-22): a fn-valued container member read as the
// LAST token of a code body — `each [ops.inc] xs`, `each [M.tbl.inc] xs`
// (each-variants L205, fold-map-filter L215, module-composition L98). The
// interpreter's reach collapse re-steps the member over the element beneath
// (a reach-lowered group never parks, NUR173); the compiled body pushed the
// member as data above the element and its layout declined "result above a
// literal". A code-body closure unit now takes the whole-frame replay a
// named fn unit already took for the identical residual
// (noteClosureBodyReplay → OpCallDynFrame): the element is the resolved
// prefix, the member re-steps under execFnDefLiteral's own rule.
//
// Two facts the increment measured and built on:
//
//   - inside ANY unit a paren-PLACED value is data — a fn frame returns
//     `[5 fn]` for `[5 (ops.inc)]`, a callback body hands `(m.f)` to its
//     driver as the value itself; the whole-frame replay used to re-step it
//     (NUR182, silent on `main`);
//   - a `do` body's residual returns to the CALLER's tape, where the
//     interpreter re-steps a fn result over its sibling (`do [5 (ops.inc)]`
//     is 6): a placed value with siblings stays declined there.

const qbOps = `def ops {inc: (fn [[n:Integer][Integer][n add 1]])} end `

// TestQuotationBodyMemberReadParity: every row compiles and agrees with the
// interpreter.
func TestQuotationBodyMemberReadParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// the three corpus rows
		{qbOps + `each [ops.inc] [1 2 3]`, "[[2 3 4]]", "fold-map-filter L215: the member named inside the body"},
		{`def m {f:([x:Integer] => [x mul 2])} end each [(m.f)] [1 2]`, "[[fn (Integer) fn (Integer)]]", "each-variants L205: a paren PLACES the member — the body nets the fn itself"},
		{`import module [def h1 fn n:Integer Integer [n add 1] def tbl {inc: h1/v} export "M" {tbl: tbl}] end fold [add] (each [M.tbl.inc] [1 2 3]) 0`, "[9]", "module-composition L98: a two-step dotted path to a handler"},
		{`import module [def h1 fn n:Integer Integer [n add 1] def tbl {inc: h1/v} export "M" {tbl: tbl}] end each [M.tbl.inc] [1 2 3]`, "[[2 3 4]]", "the each alone"},
		// the neighbours
		{`def m {f:([x:Integer] => [x mul 2])} end each [m.f] [1 2]`, "[[2 4]]", "a lambda member"},
		{`def ops {inc: ([n:Integer] => [n add 1])} end each [ops.inc] [1 2 3]`, "[[2 3 4]]", "an arrow-lambda member"},
		{qbOps + `each [(ops.inc)] [1 2]`, "[[fn (Integer) fn (Integer)]]", "the placed member is data"},
		{qbOps + `each [5 (ops.inc)] [1 2]`, "[[fn (Integer) fn (Integer)]]", "placed above a literal: still data, the driver reads the top"},
		{qbOps + `each [5 ops.inc] [1 2 3]`, "[[6 6 6]]", "the member re-steps over the literal beneath it"},
		{qbOps + `each [dup ops.inc] [1 2 3]`, "[[2 3 4]]", "NUR183: over a duplicated element (compiled the fn as data before)"},
		{qbOps + `each [ops.inc] [1 "s"]`, "[[2 fn (Integer)]]", "a no-match parks the member"},
		{`def ops {add2: (fn [[a:Integer b:Integer][Integer][a add b]])} end each [ops.add2] [1 2 3]`, "[[fn (Integer, Integer) fn (Integer, Integer) fn (Integer, Integer)]]", "a two-param member over one element parks"},
		{`def ops {z: (fn [[][Integer][42]])} end each [ops.z] [1 2 3]`, "[[42 42 42]]", "a 0-arg member is the landing's"},
		{qbOps + `each ([e:Integer] => [e ops.inc]) [1 2 3]`, "[[2 3 4]]", "a named-param lambda body"},
		{qbOps + `do [5 ops.inc]`, "[6]", "a do body"},
		{qbOps + `do [(ops.inc)]`, "[fn (Integer)]", "a placed member alone in a do body"},
		{qbOps + `7 do [(ops.inc)]`, "[8]", "…re-stepped by the CALLER over the stack beneath the do"},
		// NUR182: the fn-unit replay honours placement
		{qbOps + `def f fn [[Integer][Any][(ops.inc)]] end f 5`, "[fn (Integer)]", "NUR182: a placed member is the fn's return, not an apply"},
		{qbOps + `def f fn [[Integer][Integer][ops.inc]] end f 5`, "[6]", "the unplaced twin applies (the fn-unit replay as before)"},
		{qbOps + `def f fn [[x:Integer][Any][x ops.inc]] end f 5`, "[6]", "over a named param"},
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

// TestPlacedMemberInFnUnitErrorsWithParity pins NUR182's error rows: a fn
// frame returns a placed member as data, and its return contract raises
// the interpreter's own error on both lanes (the replay used to apply it
// and answer 6).
func TestPlacedMemberInFnUnitErrorsWithParity(t *testing.T) {
	for _, src := range []string{
		qbOps + `def f fn [[Integer][Integer][(ops.inc)]] end f 5`,
		qbOps + `def f fn [[Integer][Any][5 (ops.inc)]] end f 1`,
		qbOps + `def f fn [[Integer][Any][(ops.inc) 5]] end f 1`,
		qbOps + `def f fn [[x:Integer][Any][x (ops.inc)]] end f 5`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "type_error" {
			t.Fatalf("%q: interpreter oracle moved: %v err=[%s] — re-derive NUR182", src, gotI, codeOf(errI))
		}
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
			continue
		}
		// The error's first line: the frame-path diagnostic annotates the
		// declaration's position differently on the two lanes.
		requireParityHead(t, src, gotC, errC, gotI, errI)
		if codeOf(errC) != "type_error" {
			t.Errorf("%q: compiled error [%s] %v, want the interpreter's type_error", src, codeOf(errC), errC)
		}
	}
}

// TestQuotationBodyMemberReadSoundCompileFailures: the sub-shapes the
// increment leaves declined, loudly — never the silent data push.
func TestQuotationBodyMemberReadSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// a placed value with a sibling in a do body: the CALLER re-steps it
		{qbOps + `do [5 (ops.inc)]`, "result above a literal", "[6]"},
		// a member read in a WORD's forward slot inside a fold body: the
		// reach re-steps before the word collects, which the recorded
		// dispatch does not model
		{qbOps + `0 fold [add ops.inc] [1 2 3]`, "result above a literal", "[9]"},
		{qbOps + `0 fold [add (ops.inc)] [1 2]`, "result above a literal", "[5]"},
	}
	for _, c := range rows {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a compile failure (graduate the row if it agrees)", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}
