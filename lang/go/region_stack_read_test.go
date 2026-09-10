package lang

import (
	"fmt"
	"strings"
	"testing"
)

// region_stack_read_test.go pins the three review findings on PR #448 and the
// older defect the third of them uncovered. All four are the same question
// asked of four different producers: WHAT does this region's own event
// consume, and where does its run actually live?
//
// Measured on the merge base 6bc55db before fixing, which is what separates
// them: the first two were introduced by the forty-eighth increment (both
// shapes REFUSED there); the fourth diverged there identically, so the
// review's diagnosis of it — the widened prefix predicate — was wrong about
// the cause while its witness was right about the divergence.

func rsrRun(t *testing.T, src string) (ran bool, gotC, gotI string, cerr, ierr error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vC, ran, cerr := a.RunCompiled(src)
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vI, ierr := b.RunInterp(src)
	return ran, fmt.Sprint(vC), fmt.Sprint(vI), cerr, ierr
}

// rsrRefuses asserts a sound refusal: the compiled lane declines, and the
// fallback answer is the interpreter's.
func rsrRefuses(t *testing.T, src, wantReason, wantInterp string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if prog != nil {
		t.Fatalf("must refuse, it compiled:\n%s", prog.Disassemble())
	}
	if !strings.Contains(reason, wantReason) {
		t.Errorf("refusal reason = %q, want it to name %q", reason, wantReason)
	}
	ran, _, gotI, _, ierr := rsrRun(t, src)
	if ran {
		t.Error("the refused program must not run compiled")
	}
	if ierr != nil {
		t.Fatalf("the interpreter oracle errored: %v", ierr)
	}
	if gotI != wantInterp {
		t.Fatalf("the interpreter ORACLE moved: %s, want %s", gotI, wantInterp)
	}
}

// TestFallbackRegionMayCarryACallable — finding 1. An island's run is the
// INTERPRETER executing arbitrary code, so what it appends is not bounded by
// the modelled out: a maybe-raising body can pass a Function through the
// handler, and the interpreter re-steps it against the value beneath.
func TestFallbackRegionMayCarryACallable(t *testing.T) {
	rsrRefuses(t,
		`def xs [1] def g fn x:Integer Integer [x add 1] 5 do [if ((xs 0 getr) eq 1) [g/v] [1 div 0]] error [drop]`,
		"", "[6]")
}

// TestFallbackInputIsAStackRead — finding 2. The FALLBACK op pops its own
// inputs; a mark opened above one of them leaves the op popping from beneath
// its own mark, and MAKE_LIST_TO_MARK then collects an empty run.
func TestFallbackInputIsAStackRead(t *testing.T) {
	rsrRefuses(t,
		`def xs [1] [do [1 div (xs 0 getr)] error [drop]]`,
		"", "[[1]]")
}

// TestVariadicUserCallPromotionRefuses — finding 3's witness, whose cause was
// TWO defects stacked. The mark plan's half is the same stack-read question
// (a user call's operands were not examined either); underneath it, a
// variadic-returning callee's result was force-promoted to ONE frame slot,
// which pops one value from a runtime-variable run. That half diverged
// identically on the merge base.
func TestVariadicUserCallPromotionRefuses(t *testing.T) {
	rsrRefuses(t,
		`def f fn [[n:Integer] [] [for n [i]]] 9 f (1 add 2)`,
		"variadic fn result promoted to a frame slot", "[9 0 1 2]")
}

// TestConstArgVariadicUserCallStillSeats — the twin that must NOT regress:
// with a CONST argument nothing is on the stack when the mark opens, so the
// prefix plan is sound and the run is seated by OpSeatBelowMark exactly as
// the fortieth increment intended.
func TestConstArgVariadicUserCallStillSeats(t *testing.T) {
	const src = `def f fn [[n:Integer] [] [for n [i]]] 9 f 3`
	ran, gotC, gotI, cerr, _ := rsrRun(t, src)
	if !ran {
		t.Fatalf("must still compile natively: %v", cerr)
	}
	if gotI != "[9 0 1 2]" {
		t.Fatalf("the interpreter ORACLE moved: %s", gotI)
	}
	if gotC != gotI {
		t.Errorf("compiled %s, interpreted %s", gotC, gotI)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, _ := a.CompileCheck(src)
	if prog == nil {
		t.Fatal("must compile")
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "STACK_MARK") || !strings.Contains(dis, "SEAT_BELOW_MARK") {
		t.Errorf("the prefix must still be seated by the mark:\n%s", dis)
	}
}
