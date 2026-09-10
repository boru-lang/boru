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
		// The lowering admits a condition netting exactly one value. The
		// EMPTY condition is no longer here: it is a statically-provable
		// error, so it compiles to a terminal trap (see
		// TestWhileEmptyConditionTraps below). Two values is not provable
		// either way — the region's last value is the condition and the
		// rest are dropped, which the lowering does not model — so it
		// keeps the refusal.
		{`while [1 false] ['x'] end 'z'`, "while: condition nets 2 values, not one", ""},
		// The empty condition BELOW the top level: the trap is terminal
		// and only a top-level program has one terminal point, so a
		// fn body's empty condition keeps the arity refusal.
		{`def f fn [[][Integer][while [] [1]]] end (f)`, "while: condition nets 0 values, not one", "condition produced no value"},
		{`if true [while [] [1]] []`, "while: condition nets 0 values, not one", "condition produced no value"},
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

// TestWhileEmptyConditionTraps — the forty-second increment. `while [] [1]`
// is not an approximation the recorder has to stand aside for: the
// condition region holds NO TOKENS, so the interpreter's very first round
// nets nothing and raises before the body has run once. The compiled
// program therefore raises the byte-identical error through a terminal
// OpTrap, and the whole program still compiles.
func TestWhileEmptyConditionTraps(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`while [] [1]`, "while: condition produced no value"},
		// A PREFIX before the loop still runs: the trap is terminal, not a
		// whole-program refusal, so everything recorded before it is kept.
		{`5 while [] [1]`, "while: condition produced no value"},
		// The condition is empty; the BODY is irrelevant, it never runs.
		{`while [] []`, "while: condition produced no value"},
	} {
		t.Run(c.src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("check: %v", cerr)
			}
			if prog == nil {
				t.Fatalf("refused %q — the empty condition is a provable error, not an unmodelled shape", reason)
			}
			if !strings.Contains(prog.Disassemble(), "TRAP") {
				t.Errorf("compiled without a terminal trap:\n%s", prog.Disassemble())
			}
			_, compiled, errC := a.RunCompiled(c.src)
			if !compiled {
				t.Fatal("the trapping program must run compiled")
			}
			if errC == nil || !strings.Contains(errC.Error(), c.want) {
				t.Errorf("compiled error %v, want %q", errC, c.want)
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			if _, errI := b.RunInterp(c.src); errI == nil || !strings.Contains(errI.Error(), c.want) {
				t.Errorf("interpreter error %v, want %q — the oracle moved", errI, c.want)
			}
		})
	}
}

// TestWhileNonEmptyConditionDoesNotTrap is the negative that keeps the trap
// keyed on the SOURCE rather than on the check pass's residual count: a
// condition with tokens is never trapped, whatever the analysis nets — the
// runtime count is the condition's to decide.
func TestWhileNonEmptyConditionDoesNotTrap(t *testing.T) {
	for _, src := range []string{
		`while [false] ['x'] end 'done'`,
		`def n 0 end while [n lt 3] [def n (n add 1)] end n`,
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: refused %q err=%v", src, reason, cerr)
		}
		if strings.Contains(prog.Disassemble(), "TRAP") {
			t.Errorf("%q: a non-empty condition must not trap:\n%s", src, prog.Disassemble())
		}
	}
}

// TestWhileEmptyConditionTrapPosition pins NUR130 as MEASURED, not as
// parity: the two lanes raise the identical error and anchor it in different
// places. The trap carries the position the recorder saw — the condition
// operand, which is the thing that is wrong — while the interpreter raises
// from stepMoveWhile with the engine's current pointer, which after the
// loop's mark/move triple has been spliced is wherever the tape happens to
// sit. The compiled anchor is the useful one; this row exists so the day the
// interpreter is moved to it, the change is deliberate.
func TestWhileEmptyConditionTrapPosition(t *testing.T) {
	const src = `while [] [1] end 5`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, _, errC := a.RunCompiled(src)
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, errI := b.RunInterp(src)
	if errC == nil || errI == nil {
		t.Fatalf("both lanes must raise: compiled=%v interp=%v", errC, errI)
	}
	// The ERROR is the same on both lanes — that half is parity, and it is
	// what the corpus row asserts.
	const want = "while: condition produced no value"
	if !strings.Contains(errC.Error(), want) || !strings.Contains(errI.Error(), want) {
		t.Fatalf("message drifted: compiled=%v interp=%v", errC, errI)
	}
	// The POSITION is not (NUR130).
	if !strings.Contains(errC.Error(), "1:7") {
		t.Errorf("the trap must anchor at the condition operand (1:7): %v", errC)
	}
	if !strings.Contains(errI.Error(), "1:14") {
		t.Errorf("NUR130 moved — the interpreter no longer anchors at the tape pointer (1:14): %v", errI)
	}
}
