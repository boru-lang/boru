package lang

import (
	"fmt"
	"testing"
)

// rand_map_from_seam_test.go pins Rand.map-from's bodies at the InvokeBody
// seam (2026-09-27, the interp-entry census's module-rand.tsv row): each
// schema value is a raw token body the handler ran on a pooled sub-engine
// (RunPooled) — an interpreter entry per key on a compiled run. It runs
// through InvokeBody now, which on the interpreter is that same run and on
// a compiled run hosts the body as a run-time-stamped unit
// (eng vm_token_body.go). A body the token host declines (a flow sentinel)
// keeps the interpreter, pinned open.
//
// A break/continue escaping a body ends the build with no result on both
// lanes (core.BodyEscaped, NUR196's contract for the iterating natives), so
// the enclosing loop resolves it; before, the interpreter read the escaped
// run's residual as the key's value.

const randSeam = `import "boru:rand"  `

var randMapFromSeamRows = []struct {
	label, src, want string
	native           bool
}{
	{"a seeded instance's draws", randSeam + `def s (Rand.with-seed 7) end Rand.map-from {a:[s.int 0 100] b:[s.bool]}`, "", true},
	{"plain computed bodies", randSeam + `Rand.map-from {a:[1 add 2] b:['x']}`, "[{a:3 b:'x'}]", true},
	{"a fn called from a body breaks the enclosing loop", randSeam + `def f fn [[] [Any] [break]] end for 3 [Rand.map-from {a:[f]}] 7`, "[7]", true},
	// A body carrying a flow sentinel is declined by the token host and
	// keeps the pooled run (open): the escape still ends the build.
	{"continue in a body (open)", randSeam + `var n 0 for 3 [var n (n add 1) Rand.map-from {a:[continue] b:[9]}] n`, "[3]", false},
}

func TestRandMapFromSeamParityAndNoEntry(t *testing.T) {
	for _, row := range randMapFromSeamRows {
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
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || (row.want != "" && fmt.Sprint(gotC) != row.want) {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestRandMapFromSeamErrorsAlike: what a body must NOT be allowed to do is
// refused alike on both lanes — a body that leaves no value, and a body that
// raises, whose error is wrapped with the key it evaluated.
func TestRandMapFromSeamErrorsAlike(t *testing.T) {
	for _, c := range []struct{ src, code string }{
		{randSeam + `Rand.map-from {a:[1 2] b:[]}`, "rand_error"},
		{randSeam + `Rand.map-from {a:[1 drop]}`, "rand_error"},
		{randSeam + `Rand.map-from {a:[s7-undefined-word]}`, "undefined_word"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || codeOf(errI) != c.code || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
			t.Errorf("%q: compiled %v [%s] %s (%v), interp %v [%s] %s", c.src, gotC, codeOf(errC), detailOf(errC), compiled, gotI, codeOf(errI), detailOf(errI))
		}
	}
}
