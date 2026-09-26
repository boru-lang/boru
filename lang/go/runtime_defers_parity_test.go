package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
)

// runtime_defers_parity_test.go — both-lane pins for the three mechanisms
// that took 44 rows off the runtime-defers ledger on 2026-09-26
// (test/go/langspec/runtime_defers.tsv). Each program runs COMPILED on one
// instance and INTERPRETED on another; the compiled run must answer or raise
// exactly what the interpreter does, with no compiler-defect note.

// bothLanes runs src on each lane and returns the two outcomes.
func bothLanes(t *testing.T, src string) (gotC []any, ran bool, errC error, gotI []any, errI error) {
	t.Helper()
	gotC, ran, errC = mustNew(t).RunCompiled(src)
	gotI, errI = mustNew(t).RunInterp(src)
	return
}

func hasDefectNote(err error) bool {
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		return false
	}
	for _, n := range ae.Notes {
		if strings.Contains(n, "compiler defect") {
			return true
		}
	}
	return false
}

// A handler's PLAIN Go error (fmt.Errorf) is the program's own result on both
// lanes: the interpreter's dispatch returns it untouched, and the compiled
// run now does too — the same text, no internal_error wrapper, no defect
// note. It used to be wrapped and booked as a compiler defect (forty corpus
// rows: convert's own errors, make's field errors, a predicate type's
// rejection, the module words' guards).
func TestPlainHandlerErrorIsTheProgramsOwnOnBothLanes(t *testing.T) {
	for _, src := range []string{
		`convert BigInteger 3.14`,
		`def Even fnpred n:Integer [eq 0 (mod 2 n)]  def q:Even 5  q`,
		`each [convert Integer] ['x' 'y']`,
	} {
		_, ran, errC, _, errI := bothLanes(t, src)
		if !ran {
			t.Errorf("%q: did not run compiled (err=%v)", src, errC)
			continue
		}
		if errC == nil || errI == nil {
			t.Errorf("%q: both lanes must raise: compiled=%v interpreted=%v", src, errC, errI)
			continue
		}
		if hasDefectNote(errC) {
			t.Errorf("%q: the handler's own error was booked as a compiler defect: %v", src, errC)
		}
		if errC.Error() != errI.Error() {
			t.Errorf("%q: error text differs\n  compiled:    %s\n  interpreted: %s", src, errC.Error(), errI.Error())
		}
	}
}

// A typed def's construction (`def b:Type {map}` — a class or an Entity)
// raises make's error WRAPPED as the typed-def handler wraps it, `def b: …`,
// on both lanes: the recorded make runs a per-def copy of make's signature
// whose handler adds the wrap (core.RecordTypedDefMake). Recording make's own
// signature raised the bare `make: …` on the compiled lane (generics.tsv L56,
// resource.tsv L156).
func TestTypedDefMakeErrorKeepsTheDefWrap(t *testing.T) {
	const box = `def Box gen [T] class {value:T} end `
	for _, c := range []struct{ src, prefix string }{
		{box + `def b:(Box of [Integer]) {value:'no'}`, "def b: make: "},
		{`def e:Entity { kind:'api' } e`, "def e: make: "},
	} {
		_, ran, errC, _, errI := bothLanes(t, c.src)
		if !ran || errC == nil || errI == nil {
			t.Errorf("%q: want a raise on both lanes, compiled=%v (ran=%v) interpreted=%v", c.src, errC, ran, errI)
			continue
		}
		if errC.Error() != errI.Error() || !strings.HasPrefix(errC.Error(), c.prefix) {
			t.Errorf("%q: want the interpreter's %q-wrapped error on both lanes\n  compiled:    %s\n  interpreted: %s", c.src, c.prefix, errC, errI)
		}
	}
	// Negative: the wrap touches failures only — a valid construction answers
	// the instance on both lanes.
	src := box + `def b:(Box of [Integer]) {value:7} b.value`
	gotC, _, errC, gotI, errI := bothLanes(t, src)
	if errC != nil || errI != nil || fmt.Sprint(gotC) != "[7]" || fmt.Sprint(gotI) != "[7]" {
		t.Errorf("%q: compiled=%v/%v interpreted=%v/%v, want [7] on both", src, gotC, errC, gotI, errI)
	}
}

// The negative half: the VM's OWN failures stay defects. Its refusal to start
// (a nil program) carries the VMDefer marker — it was a plain fmt.Errorf,
// which the new disposition would otherwise have passed through as the
// program's result — and compiledRunError annotates it.
func TestVMEntryRefusalStaysADefect(t *testing.T) {
	a := mustNew(t)
	_, err := eng.RunProgram(nil, a.NativeRegistry())
	if err == nil || !core.IsVMDefer(err) {
		t.Fatalf("a nil program must be refused with a VMDefer-marked error, got %v", err)
	}
	got, defect := compiledRunError(a.NativeRegistry(), err)
	if !defect || !hasDefectNote(got) {
		t.Errorf("the VM's entry refusal must be reported as a compiler defect, got %v (defect=%v)", got, defect)
	}
}

// A type the check pass minted AND retired (`def Point class … undef Point`)
// must be live again for the replayed run between its type-install twin and
// its undef twin: `make Point` resolves the node, and the instance keeps its
// identity after the undef — class.tsv L98–L101, which bailed with
// "unresolvable type operand Point".
func TestRetiredTypeNodeReplaysForTheRun(t *testing.T) {
	const pre = `def Point class {x:1} def p (make Point {x:5}) undef Point end `
	for _, c := range []struct{ src, want string }{
		{pre + `p typeof`, "[Point]"},
		{pre + `p dot x`, "[5]"},
		{pre + `p set x 9 end p.x`, "[9]"},
	} {
		gotC, _, errC, gotI, errI := bothLanes(t, c.src)
		if errC != nil || errI != nil {
			t.Errorf("%q: compiled=%v interpreted=%v", c.src, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interpreted=%v, want %s on both", c.src, gotC, gotI, c.want)
		}
	}
	// Negative: an off-schema write through the retired type is still
	// rejected, with the interpreter's own error.
	src := pre + `p set x 'hi'`
	_, _, errC, _, errI := bothLanes(t, src)
	if errC == nil || errI == nil || hasDefectNote(errC) || errC.Error() != errI.Error() {
		t.Errorf("%q: want the interpreter's own error on both lanes:\n  compiled:    %v\n  interpreted: %v", src, errC, errI)
	}
	// Negative: the re-adoption does not outlive the undef. The undef twin
	// retires the node again, so after the run the registry's ID index no
	// longer resolves it — exactly as the interpreter's undef leaves it.
	ac := mustNew(t)
	vals, _, _, err := ac.RunAutoValues(pre + `p`)
	if err != nil || len(vals) != 1 {
		t.Fatalf("compiled: %v %v", vals, err)
	}
	ai := mustNew(t)
	valsI, err := ai.RunInterpValues(pre + `p`)
	if err != nil || len(valsI) != 1 {
		t.Fatalf("interpreted: %v %v", valsI, err)
	}
	liveC := ac.NativeRegistry().Types.LookupByID(vals[0].Parent.ID) != nil
	liveI := ai.NativeRegistry().Types.LookupByID(valsI[0].Parent.ID) != nil
	if liveC || liveI {
		t.Errorf("Point still resolves by ID after its undef: compiled=%v interpreted=%v, want false on both", liveC, liveI)
	}
}

// A builtin type-name word counts as the ONE value it steps to in the poly
// no-match plan's reach bound, so `convert Integer <x>` with no overload for
// x raises the interpreter's signature_error — detail, notes, position —
// instead of bailing at vm:poly-no-match (convert-ideal.tsv L33,
// edge-scalars-3.tsv L218).
func TestConvertNoMatchRaisesTheInterpretersError(t *testing.T) {
	for _, src := range []string{
		`convert Integer none`,
		`def Foo (class {}) convert Integer (make Foo {})`,
	} {
		_, ran, errC, _, errI := bothLanes(t, src)
		var aeC, aeI *core.BoruError
		if !ran || !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
			t.Errorf("%q: want a structured raise on both lanes, compiled=%v (ran=%v) interpreted=%v", src, errC, ran, errI)
			continue
		}
		if aeC.Code != "signature_error" || hasDefectNote(errC) {
			t.Errorf("%q: compiled lane raised [%s] %s, want the interpreter's signature_error", src, aeC.Code, aeC.Detail)
		}
		if aeC.Code != aeI.Code || aeC.Detail != aeI.Detail || aeC.Row != aeI.Row || aeC.Col != aeI.Col ||
			strings.Join(aeC.Notes, "|") != strings.Join(aeI.Notes, "|") {
			t.Errorf("%q: raise differs\n  compiled:    [%s] %s @%d:%d %q\n  interpreted: [%s] %s @%d:%d %q",
				src, aeC.Code, aeC.Detail, aeC.Row, aeC.Col, aeC.Notes, aeI.Code, aeI.Detail, aeI.Row, aeI.Col, aeI.Notes)
		}
	}
	// Negative: a convert that DOES match answers on both lanes — the raise is
	// the no-match arm's only.
	gotC, _, errC, gotI, errI := bothLanes(t, `convert Integer '12'`)
	if errC != nil || errI != nil || fmt.Sprint(gotC) != "[12]" || fmt.Sprint(gotI) != "[12]" {
		t.Errorf("convert Integer '12': compiled=%v/%v interpreted=%v/%v, want [12] on both", gotC, errC, gotI, errI)
	}
}
