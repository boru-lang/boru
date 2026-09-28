package lang

import (
	"fmt"
	"testing"
)

// behave_stepless_seam_test.go pins the behaviour-body run on a compiled
// program (2026-09-27, the interp-entry census's code-bodies.tsv row): a
// body of scalar literals is answered without an engine — it is its own
// residual under the NewTop regime every behaviour body runs in — so a
// constant canon/compare/size body enters no interpreter. A body that
// computes, or one carrying an atom (the top engine stamps its referent),
// keeps the pooled top run, pinned open.

const behaveSteplessTemp = `def Temp refine Integer end `

var behaveSteplessRows = []struct {
	label, src, want string
	native           bool
}{
	{"a constant canon body", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String]['T']]) end canon (make Temp 5)`, "[T]", true},
	{"a constant canon body rendering a list", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String]['T']]) end [(make Temp 5) (make Temp 6)]`, "[[T T]]", true},
	{"a canon body's top wins", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String]['a' 'b']]) end canon (make Temp 5)`, "[b]", true},
	{"a canon body of the wrong type reports alike", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String][42]]) end canon (make Temp 5)`, "[<Temp canon-error: body must return String, got Integer>]", true},
	{"a constant compare body", behaveSteplessTemp + `behave compare/q (fn [[Temp Temp][Integer][0]]) end sort [(make Temp 5) (make Temp 3)]`, "[[5 3]]", true},
	{"a constant size body", behaveSteplessTemp + `behave size/q (fn [[Temp][Integer][7]]) end size (make Temp 5)`, "[7]", true},
	// An atom's referent is stamped by the top engine: the engine keeps it.
	{"an atom body (open)", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String][x/q]]) end canon (make Temp 5)`, "[<Temp canon-error: body must return String, got Atom>]", false},
	// A body that dispatches runs on the engine.
	{"a computing body (open)", behaveSteplessTemp + `behave canon/q (fn [[t:Temp][String][convert String (1 add 1)]]) end canon (make Temp 5)`, "", false},
}

func TestBehaveSteplessParityAndNoEntry(t *testing.T) {
	for _, row := range behaveSteplessRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" && !ev.CheckMode {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if errI != nil || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) || (row.want != "" && fmt.Sprint(gotC) != row.want) {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if !compiled {
			// Measured, not assumed: a row that stops compiling proves
			// nothing about the seam.
			if row.native {
				t.Errorf("%s: did not compile\n  %s", row.label, row.src)
			}
			continue
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}
