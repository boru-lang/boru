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

// mkClosure is a factory whose result is a DECLARED Function — a produced
// closure, the value both guards below are about.
const mkClosure = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end `

// TestShuffledClosureRefusesAndTheInterpreterApplies pins NUR131 from the
// side that matters: a full-stack shuffle over a PRODUCED closure must not
// compile, because the interpreter re-steps what the shuffle re-pushes and
// the compiled lane would leave it as data.
//
// The first four rows COMPILED TO WRONG ANSWERS before this guard, and they
// did so on the merge base too — the defect is the fold's, not the residual
// rebuild's, and it was found reviewing the rebuild. Each row's `want` is
// the interpreter's answer, which is what the refusal preserves.
func TestShuffledClosureRefusesAndTheInterpreterApplies(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The closure is duplicated onto the top and fires twice: 5*3*3.
		{mkClosure + `5 (mk 3) 0 pick`, "[45]"},
		// …and rolled to the top, firing once: 5*3.
		{mkClosure + `5 (mk 3) 1 roll`, "[15]"},
		// A deeper roll, with a value left above the application.
		{mkClosure + `9 (mk 3) 9 2 roll`, "[27 9]"},
		// A pick that copies the closure over a value further down.
		{mkClosure + `7 (mk 3) 1 pick`, "[7 21]"},
		// The residual REBUILD's own screen (a different guard, same rule):
		// a literal beneath a produced closure, and two of them permuted.
		{mkClosure + `5 (mk 3)`, "[5 fn (Integer)]"},
		{mkClosure + `(mk 3) (mk 4) 1 roll`, "[fn (Integer) fn (Integer)]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, cerr := func() (any, string, error) {
				p, r, _, e := a.CompileCheck(tc.src)
				if p == nil {
					return nil, r, e
				}
				return p, r, e
			}()
			if cerr != nil {
				t.Fatalf("check: %v", cerr)
			}
			if prog != nil {
				t.Fatalf("a shuffled produced closure must refuse — the interpreter re-steps it")
			}
			if reason == "" {
				t.Error("a refusal must carry a reason")
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

// TestShuffledFnReadStillCompiles is the other half of the guard's line, and
// the reason it tests PROVEN-callable-and-event-produced rather than the
// wider possibly-callable screen the rebuild uses. A def-bound `/v` read is
// not event-produced and the deopt machinery already covers it, so these
// shuffles agree on both lanes today and must keep compiling.
func TestShuffledFnReadStillCompiles(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`def g fn [[x:Integer][Integer][x add 1]] end (1 add 2) g/v 0 pick`, "[5]"},
		{`def g fn [[x:Integer][Integer][x add 1]] end (1 add 2) g/v 1 roll`, "[4]"},
		{`def g fn [[x:Integer][Integer][x add 1]] end g/v (1 add 2) 1 roll`, "[4]"},
		// A non-callable event result is untouched by either guard.
		{`def m {a:1} end (m get 'a') (m get 'a') 1 roll`, "[1 1]"},
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
				t.Errorf("interpreter %s, want %s", got, tc.want)
			}
		})
	}
}
