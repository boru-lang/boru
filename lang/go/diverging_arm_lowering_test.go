package lang

import (
	"fmt"
	"strings"
	"testing"
)

// diverging_arm_lowering_test.go is the whole-program half of the fifty-first
// increment: the last `while` frontier row, and the two facts its refusal
// conflated.
//
// FACT ONE — "nets no value" is not "diverges". The gate's message said
// diverges and its test was hasOut; a 0-netting arm reaches the merge having
// produced nothing (the join is 1-or-0 where the slot models 1), while a
// DIVERGING arm leaves the construct and never reaches the merge at all, so
// every path that arrives carries the eager value.
//
// FACT TWO — a computed arm can sit UNDER its own condition. The written
// spelling `if c [t] (expr)` evaluates the arm last, so it is on top; an arm
// filled from the VALUE STACK by the argument-order rule was there first, and
// the condition — a forward token evaluated at the dispatch — is above it.
// The lowering owed no swap in that layout and refused instead.

// daRun runs a source on both lanes.
func daRun(t *testing.T, src string) (ran bool, gotC, gotI string, cerr, ierr error) {
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

// TestDivergingArmUnderAComputedPrefixCompiles — the ledger row, its `for`
// twin (which the ledger claimed refused too), and the shapes either fact
// alone would still have refused.
func TestDivergingArmUnderAComputedPrefixCompiles(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// THE ledger row: frontier-while.tsv's last entry.
		{`def c (flex {n:0}) end while [(c get 'n') lt 3] [ set 'n' ((c get 'n') add 1) c end if ((c get 'n') eq 2) [continue] end (c get 'n') ]`,
			"[{n:3} 1 {n:3} 3]"},
		// The `for` twin the ledger's note named, shrunk to its mechanism.
		{`for 3 [ (7 add 2) if (i eq 2) [continue] end i ]`, "[9 0 9 1]"},
		{`for 3 [ (7 add 2) if (i eq 2) [break] end i ]`, "[9 0 9 1]"},
		// FACT TWO alone: a stack-supplied arm whose non-eager arm DOES net a
		// value was refused only by the layout, and compiles now too.
		{`for 3 [ (7 add 2) if (i eq 2) [3] end i ]`, "[9 0 9 1 3 2]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, gotC, gotI, cerr, ierr := daRun(t, tc.src)
			if !ran {
				t.Fatalf("must compile: %v", cerr)
			}
			if cerr != nil || ierr != nil {
				t.Fatalf("unexpected errors: compiled %v / interp %v", cerr, ierr)
			}
			if gotI != tc.want {
				t.Fatalf("the interpreter ORACLE moved: %s, want %s", gotI, tc.want)
			}
			if gotC != gotI {
				t.Errorf("compiled %s, interpreted %s", gotC, gotI)
			}
		})
	}
}

// TestDivergingArmRaisesAndReturnsIdentically — the other two divergence
// kinds the deep test admits: a `raise` arm, and the same shape inside a fn
// body (where the return-count contract sees the two-value residual).
//
// byteIdentical is false for the fn-body row and only for its POSITION: the
// compiled RET check is stamped with the unit's own position because one unit
// serves every call site, so it blames the body's first token where the
// interpreter blames the call. That is NUR118, pre-existing and recorded; the
// code, detail and the declaration note all agree.
func TestDivergingArmRaisesAndReturnsIdentically(t *testing.T) {
	for _, tc := range []struct {
		src, want     string
		byteIdentical bool
	}{
		{`for 3 [ (7 add 2) if (i eq 2) [raise oops "x"] end i ]`, "[boru/oops]: x", true},
		{`def zg fn [[n:Integer][Any][ (7 add 2) if (n eq 2) [raise oops "x"] end n ]] end (zg 1)`,
			"[boru/type_error]: zg: expected 1 return value(s), got 2 — [9 1]", false},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, _, _, cerr, ierr := daRun(t, tc.src)
			if !ran {
				t.Fatalf("must compile: %v", cerr)
			}
			if cerr == nil || ierr == nil {
				t.Fatalf("both lanes must raise: compiled %v / interp %v", cerr, ierr)
			}
			if !strings.HasPrefix(ierr.Error(), tc.want) {
				t.Fatalf("the interpreter ORACLE moved: %v", ierr)
			}
			if !strings.HasPrefix(cerr.Error(), tc.want) {
				t.Errorf("the compiled code and detail must match: %v", cerr)
			}
			if tc.byteIdentical && fmt.Sprint(cerr) != fmt.Sprint(ierr) {
				t.Errorf("the compiled raise must be byte-identical:\ncompiled %v\ninterp   %v", cerr, ierr)
			}
		})
	}
}

// TestZeroNettingArmKeepsItsRefusal — the negative that carries the whole
// argument. An EMPTY arm nets nothing and does NOT diverge, so it arrives at
// the merge with nothing to contribute and the single slot cannot describe
// it. It keeps its refusal, under the message that now says what it means.
func TestZeroNettingArmKeepsItsRefusal(t *testing.T) {
	const src = `for 3 [ (7 add 2) if (i eq 2) [] end i ]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if prog != nil {
		t.Fatalf("a 0-netting non-diverging arm must keep the refusal:\n%s", prog.Disassemble())
	}
	if reason != "if: computed-branch non-eager arm nets no value (Stage 2)" {
		t.Fatalf("refusal reason drifted: %q", reason)
	}
	ran, _, gotI, _, _ := daRun(t, src)
	if ran {
		t.Error("the refused program must not run compiled")
	}
	if gotI != "[9 0 9 1 2]" {
		t.Fatalf("the interpreter ORACLE moved: %s", gotI)
	}
}
