package lang

import (
	"fmt"
	"testing"
)

// fn_value_park_test.go pins the VM's no-match fork for a fn VALUE
// (eng vm_fnvalue_park.go, 2026-09-27 — the interp-entry census's
// fn-value.tsv:L28, module-fnvalue-boundary.tsv:L24, fold-map-filter.tsv:L87
// and L88) at each of its three seats — a CALL_DYNAMIC window (leading and
// trailing), a callDynFrame region, and the token seam a higher-order word
// hands its callback to — against the interpreter, and with no interpreter
// entry: an anonymous value no signature admits PARKS (data, the window as
// written), a named one RAISES uncalled_function. A window the step would
// dispatch — a predicate-typed param the value's plan admits — or one whose
// tokens the island must step keeps the island, pinned open.

const parkLam = `def m {f: (fn [[a:Integer][Integer][a add 1]])}  `
const parkNamed = `def g fn [[a:Integer][Integer][a add 1]] end def m {f: g/v}  `

var fnValueParkRows = []struct {
	label, src string
	native     bool
}{
	{"leading: a rejected arg parks the lambda", parkLam + `m.f 'x'`, true},
	{"leading: two rejected args", parkLam + `m.f 'x' 'y'`, true},
	{"leading: a two-param lambda over one rejected arg", `def m {f: (fn [[a:Integer b:String][Integer][a add 1]])}  m.f 5 6`, true},
	{"trailing: a rejected arg beneath parks", parkLam + `'x' m.f`, true},
	{"frame region: args.0 re-stepped over a param it rejects", `import module [def keep fn [[Function] [Function] [args.0]] export "L" {keep: keep/v, mk: (fn [[x:Integer] [Integer] [x mul 3]])}] end typeof (L.keep L.mk)`, true},
	{"token seam: fold's accumulator the lambda rejects", `{a:1} fold ([acc:Map e:Integer] => [acc add e]) [1 2]`, true},
	{"token seam: a list of functions folded", `10 fold ([a:Integer f:Function] => [(f a)]) [(fn [[n:Integer][Integer][n add 1]])]`, true},
	{"token seam: rejected elements", `0 fold ([a:Integer b:Integer] => [a add b]) ['a' 'b']`, true},
	// The step would DISPATCH: a predicate-typed param admits 5 (the plan's
	// own matcher runs the predicate), so the fork is not this one.
	{"a predicate-typed param that admits (open)", `def P (Integer gt 0) def m {f: (fn [[a:P][Integer][a add 1]])}  m.f 5`, false},
	// A forward list is stepped by the island after the park: not stepless.
	{"a forward list (open)", parkLam + `m.f [1 2]`, false},
	{"trailing: a list beneath (open)", parkLam + `[1 2] m.f`, false},
}

func TestFnValueParkParityAndNoEntry(t *testing.T) {
	for _, row := range fnValueParkRows {
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
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestFnValueParkRaisesAlike: a NAMED value no signature admits is a call
// that failed — uncalled_function, named and positioned alike on both lanes,
// with no interpreter entry — at each seat.
func TestFnValueParkRaisesAlike(t *testing.T) {
	for _, src := range []string{
		parkNamed + `'x' m.f`,
		`import module [def mk fn [[x:Integer] [Integer] [x mul 3]] def keep fn [[Function] [Function] [args.0]] export "L" {keep: keep/v, mk: mk/v}] end typeof (L.keep L.mk)`,
		`def g fn [[a:Integer b:Integer][Integer][a add b]] end 0 fold g/v ['a' 'b']`,
	} {
		a, _ := New()
		gotI, errI := a.RunInterp(src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" && !ev.CheckMode {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(src)
		disarm()
		if !compiled || codeOf(errI) != "uncalled_function" || fmt.Sprint(errC) != fmt.Sprint(errI) || len(gotC) != 0 || len(gotI) != 0 {
			t.Errorf("%q: compiled %v %v (%v), interp %v %v", src, gotC, errC, compiled, gotI, errI)
		}
		if len(entries) != 0 {
			t.Errorf("%q: the compiled lane entered the interpreter via %v", src, entries)
		}
	}
}
