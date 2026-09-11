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

// --- NUR137: the FIFTH producer, and the second time this predicate was
// incomplete -------------------------------------------------------------

// TestBranchRegionConditionIsAStackRead — a BRANCH region's condition. The
// forty-seventh increment widened `variadicRegionEvent` to admit a
// branch-variant region ("the plan's single-slot gate was inherited from the
// two producers that happened to exist"), which is true and was half the
// job: the SCREEN was inherited the same way. `regionReadsTheStack` read
// `ev.call.ops` for every kind it did not name, and an evBranch's operands
// live in `ev.br` — so the condition was never examined, the mark opened
// above it, and the branch popped from beneath its own mark.
//
// Measured: on the merge base 6bc55db this REFUSED ("call result above a
// literal"); at d663fe1 (the forty-seventh increment) it answered `9 1 9`
// against the interpreter's `1 9 9`, silently, exit 0, on the default lane.
func TestBranchRegionConditionIsAStackRead(t *testing.T) {
	rsrRefuses(t,
		`def zs [0] def zt (zs 0 getr)  1 (if (zt gt 0) [] [9 9])`,
		"residual shape beyond Stage 1", "[1 9 9]")
	// The zero-run arm of the same shape: the count differs, the defect does
	// not — one witness per arm so a fix that only repairs the populated run
	// cannot pass.
	rsrRefuses(t,
		`def zs [1] def zt (zs 0 getr)  1 (if (zt gt 0) [] [9 9])`,
		"residual shape beyond Stage 1", "[1]")
}

// TestBranchRegionConditionInABodyUnit — the same hole, reached through the
// fifty-fifth increment's body-unit seat, where it surfaced as an INTERNAL
// VM error rather than a wrong answer: `bytecode: internal: SEAT_BELOW_MARK
// prefix reaches past the mark`. At the fifty-fourth increment this refused
// cleanly, so the body-unit seat is what made the top-level hole reachable
// here — the hole itself is older.
func TestBranchRegionConditionInABodyUnit(t *testing.T) {
	rsrRefuses(t,
		`def zs [1] def zt (zs 0 getr)  do [1 (if (zt gt 0) [] [9 9])]`,
		"result above a literal", "[1]")
}

// TestInertConditionBranchRegionStillSeats — the twins that must NOT
// regress. A condition that is NOT event-produced leaves nothing on the
// stack when the mark opens, so the prefix plan stays sound: the top-level
// shape the forty-seventh increment graduated, and the body-unit shape the
// fifty-fifth did.
func TestInertConditionBranchRegionStillSeats(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`1 (if true [] [9 9])`, "[1]"},
		{`1 (if false [] [9 9])`, "[1 9 9]"},
		{`do [def b true  1 (if b [] [9 9])]`, "[1]"},
	} {
		ran, gotC, gotI, cerr, ierr := rsrRun(t, tc.src)
		if ierr != nil {
			t.Fatalf("%s: the interpreter oracle errored: %v", tc.src, ierr)
		}
		if gotI != tc.want {
			t.Fatalf("%s: the interpreter ORACLE moved: %s, want %s", tc.src, gotI, tc.want)
		}
		if !ran {
			t.Fatalf("%s: must still compile natively: %v", tc.src, cerr)
		}
		if gotC != gotI {
			t.Errorf("%s: compiled %s, interpreted %s", tc.src, gotC, gotI)
		}
	}
}
