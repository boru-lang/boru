package lang

import (
	"strings"
	"testing"
)

// while_compile_test.go pins the thirty-seventh increment: `while [cond]
// [body]` lowers on the counted loop's own frame — an unbounded
// FOR_SETUP/FOR_NEXT over a scratch iterator, the condition fragment
// lowered at the head of every iteration, a falsy value exiting through
// FLOW_BREAK (which pops the loop frame and trims the round exactly as a
// `break` does). The interpreter's semantics, measured first: the
// condition region's LAST value decides; break and continue discard the
// current round's values; body values accumulate; a def the body rebinds
// persists after the loop; an empty condition raises. The lowering admits
// a condition netting exactly one value and refuses every other count.

// TestWhileCompileParity pins the shapes that COMPILE, agree on both lanes
// and run VM-native.
func TestWhileCompileParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{`while [false] ['x'] end 'done'`, "done — a falsy condition runs the body zero times (ledger row)"},
		{`while [true] [break] end 'ended'`, "ended — break ends the loop (ledger row)"},
		{`while ['ok'] [break] end 'truthy'`, "truthy — the condition is a truthiness read (ledger row)"},
		{`def c (flex {n:0}) end while [(c get 'n') lt 3] [ (c get 'n') set 'n' ((c get 'n') add 1) c ]`, "0 {n:3} 1 {n:3} 2 {n:3} — the counter loop over a flex map (ledger row)"},
		{`def c (flex {n:0}) end while [(c get 'n') lt 3] [ set 'n' ((c get 'n') add 1) c end if ((c get 'n') eq 2) [1] [2] end (c get 'n') ]`, "a two-arm if over the enclosing computation inside the body"},
		{`def n 0 end while [n lt 3] [def n (n add 1) n] end 'z'`, "1 2 3 z — a carried rebind read by the condition; body values accumulate"},
		{`def n 0 end while [n lt 3] [def n (n add 1)] end n`, "3 — the carried rebind persists after the loop"},
		{`def n 5 end while [n lt 3] [def n (n add 1)] end n`, "5 — zero iterations leave the pre-loop value"},
		{`def n 0 end while [(n add 0) lt 3] [def n (n add 1)] end n`, "3 — a computed condition over the carried slot"},
		{`while [true] [1 break] end 'x'`, "x — break discards the round's values"},
		{`def n 0 end while [n lt 3] [def n (n add 1) 1 continue 2] end 'z'`, "z — continue discards the round's values"},
		{`def n 0 end while [n lt 3] [def n (n add 1) if (n eq 2) [continue] end n] end 'z'`, "1 3 z — a continue inside a branch arm"},
		{`def n 0 end while [n lt 3] [def n (n add 1) if (n eq 2) [break] end n] end 'z'`, "1 z — a break inside a branch arm"},
		{`def f fn [[k:Integer][Integer][def n 0 while [n lt k] [def n (n add 1)] n]] end (f 4)`, "4 — inside a fn unit, counting against a param"},
		{`def n 0 end def f fn [[][Integer][while [n lt 3] [def n (n add 1)] n]] end (f)`, "3 — a fn body rebinding a module def"},
		{`def n 0 end while [n lt 2] [def n (n add 1) while [true] [1 break]] end 'z'`, "z — a nested while's break ends only the inner loop"},
		{`def n 3  while [n gt 0] [def acc n  def n (n sub 1)] end 1`, "1 — two carried rebinds"},
	}
	for _, c := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		if len(islands) > 0 {
			t.Errorf("%q: re-enters the interpreter (%s)", c.src, islands[0])
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestWhileCompileSoundRefusals pins the neighbours that REFUSE, and that
// the interpreter answers each one (the refusal is a fallback, not a
// miscompile).
func TestWhileCompileSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interpErr string }{
		// The lowering admits a condition netting exactly one value.
		{`while [] [1]`, "while: condition nets 0 values, not one", "condition produced no value"},
		{`while [1 false] ['x'] end 'z'`, "while: condition nets 2 values, not one", ""},
		// A pre-existing gate `for` shares: a multi-value body with a rebind.
		{`def n 0 end while [n lt 3] [def n (n add 1) n n] end 'z'`, "dynamic-scope def `n` of unpromoted computed value", ""},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a sound refusal", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refused %q, want %q", c.src, reason, c.reason)
		}
		_, ierr := a.RunInterp(c.src)
		if c.interpErr == "" {
			if ierr != nil {
				t.Errorf("%q: interpreter error %v", c.src, ierr)
			}
		} else if ierr == nil || !strings.Contains(ierr.Error(), c.interpErr) {
			t.Errorf("%q: interpreter error %v, want %q", c.src, ierr, c.interpErr)
		}
	}
}
