package lang

import (
	"fmt"
	"strings"
	"testing"
)

// residual_rebuild_test.go is the whole-program half of the program-residual
// REBUILD (the forty-third increment): a residual whose ORDER is not the
// order the events produced it in.
//
// The full-stack words are the producers. `1 roll` over two call results
// models the permutation exactly — the fold has both values — but a static
// lowering cannot re-seat them: each call left its result where it ran, and
// there is no offset that reaches past a value already on the stack. The
// lowering now spills every simulated-stack entry to a frame local and
// pushes the residual back exactly as recorded, which covers the permutation
// and, with it, the residual that DUPLICATES a call result, the one that
// DROPS one, and the one that seats an inert value BENEATH one.
//
// compiler/go/residual_rebuild_test.go is the seam half (the declines).

func rrRun(t *testing.T, src string) string {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, cerr := a.RunCompiled(src)
	if cerr != nil {
		t.Fatalf("RunCompiled(%q): %v", src, cerr)
	}
	if !compiled {
		t.Fatalf("RunCompiled(%q): fell back to the interpreter", src)
	}
	return fmt.Sprintf("%v", out)
}

func TestResidualRebuildCompilesWithParity(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The frontier row: two call results permuted.
		{`(1 add 2) (3 add 4) 1 roll`, "[7 3]"},
		{`(1 add 2) (3 add 4) swap`, "[7 3]"},
		// THREE results rotated — the shape the pairwise swap cannot reach.
		{`(1 add 2) (3 add 4) (5 add 6) 2 roll`, "[7 11 3]"},
		{`(1 add 2) (3 add 4) (5 add 6) 1 roll`, "[3 11 7]"},
		// A DUPLICATED call result: one spill temp, read twice.
		{`(1 add 2) (3 add 4) 1 pick`, "[3 7 3]"},
		{`(1 add 2) (3 add 4) 0 pick`, "[3 7 7]"},
		// A DROPPED call result: the value is computed (its call still runs)
		// and simply not pushed back.
		{`(1 add 2) (3 add 4) drop`, "[3]"},
		// An INERT value beneath a call result — the shape whose refusal read
		// "call result above a literal".
		{`(1 add 2) 9 swap`, "[9 3]"},
		{`(1 add 2) (3 add 4) 'x' 2 roll`, "[7 x 3]"},
		// Non-Integer results, so nothing rides on the element type.
		{`(1 lt 2) (3 gt 4) 1 roll`, "[false true]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := rrRun(t, tc.src); got != tc.want {
				t.Errorf("compiled %s, want %s", got, tc.want)
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			out, ierr := b.RunInterp(tc.src)
			if ierr != nil {
				t.Fatalf("RunInterp: %v", ierr)
			}
			if got := fmt.Sprintf("%v", out); got != tc.want {
				t.Errorf("interpreter %s, want %s — the oracle moved", got, tc.want)
			}
		})
	}
}

// TestResidualRebuildOnlyWhenNeeded is the cost line: the rebuild is a
// fallback, tried only after the in-place seating declines. An ordinary
// residual — call results in production order, inert values above them —
// must still lower with no spill at all.
func TestResidualRebuildOnlyWhenNeeded(t *testing.T) {
	for _, src := range []string{
		`(1 add 2) (3 add 4)`,
		`(1 add 2) 9`,
		`(1 add 2) (3 add 4) (5 add 6)`,
		`1 2 3 1 roll`,
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: refused %q err=%v", src, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "STORE_LOCAL") {
			t.Errorf("%q: an in-order residual must not spill:\n%s", src, prog.Disassemble())
		}
	}
}

// TestResidualRebuildFrameCountsTheSpills — the defect the rebuild exposed,
// pinned so it cannot come back. The spill temps are allocated during the
// RESIDUAL reconciliation, which runs after the event lowering; a frame
// count written back before it left every temp outside the frame, and the
// program then failed its first STORE_LOCAL at run time and fell back to the
// interpreter silently — right answer, no refusal reason, no compiled run.
func TestResidualRebuildFrameCountsTheSpills(t *testing.T) {
	const src = `(1 add 2) (3 add 4) (5 add 6) 2 roll`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("refused %q err=%v", reason, cerr)
	}
	dis := prog.Disassemble()
	if !strings.Contains(dis, "STORE_LOCAL l2") {
		t.Fatalf("expected three spill temps:\n%s", dis)
	}
	if prog.NumLocals < 3 {
		t.Errorf("NumLocals = %d, want at least the 3 spill temps:\n%s", prog.NumLocals, dis)
	}
	// The run must be the COMPILED one — a frame too small falls back.
	if got := rrRun(t, src); got != "[7 11 3]" {
		t.Errorf("compiled %s, want [7 11 3]", got)
	}
}
