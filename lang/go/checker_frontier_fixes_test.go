package lang

import (
	"fmt"
	"testing"
)

// countErrors returns the number of error-severity diagnostics for src.
func countErrors(t *testing.T, src string) (int, []string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("Check(%q): %v", src, err)
	}
	n := 0
	var details []string
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			n++
			details = append(details, d.Detail)
		}
	}
	return n, details
}

// A value-or-None branch inside a fn body must type-check cleanly. Before the
// joinCarriersInner None-arm guard, joining Integer (or Any) with the None arm
// produced a Parent-less carrier that HALTED the fn-body analyser
// ("[boru/halt]: undefined stack entry"), so the whole fn wrongly reported a
// fn_body_error and a spurious "produces no return value". This is the exact
// shape of mini-redis's `arg-at` (`if (size gt i) [xs get i] [None]`).
func TestNoneBranchInFnBodyChecksClean(t *testing.T) {
	for _, src := range []string{
		`def f fn [[b:Boolean] [Any] [ if b [99] [None] ]] f true`,
		`def f fn [[b:Boolean] [Any] [ if b [None] [99] ]] f true`,
		`def arg-at fn [[xs:List i:Integer] [Any] [ if ((size xs) gt i) [ xs get i ] [ None ] ]] arg-at [10 20] 0`,
	} {
		if n, ds := countErrors(t, src); n != 0 {
			t.Errorf("expected 0 errors for %q, got %d: %v", src, n, ds)
		}
	}
}

// A side-effect for-loop (body nets 0 per iteration) with a CARRIER count must
// contribute no residual, so a trailing statement is the sole return. Before
// the forCarrierAnalyse 0-net guard the loop wrongly returned a single List
// carrier, over-counting the fn's return arity as a false
// "expected 1 return value(s), got 2". The count form and the const-start/step
// computed-range form (mini-s3's s3-send-resp `for [0 total 65536]`) both check
// clean AND lower; a computed-start/step range checks clean but falls back at
// compile time — either way, no phantom "got 2".
func TestForZeroNetBodyChecksClean(t *testing.T) {
	for _, src := range []string{
		`def f fn [[n:Integer] [Any] [ for n [ 5 drop ] 0 ]] f 3`,
		`def f fn [[n:Integer] [Any] [ for [0 n 1] [ 5 drop ] 0 ]] f 3`,
		`def f fn [[n:Integer] [Any] [ for [0 n 65536] [ 5 drop ] 0 ]] f 3`,
		// Ranges the compiler CANNOT lower (a computed start or step — RecordLoop
		// needs a const start/step) must still check clean and net 0: the check and
		// compile passes agree (compile falls back rather than reporting "got 2").
		`def f fn [[a:Integer b:Integer] [Any] [ for [a b] [ 5 drop ] 0 ]] f 1 4`,
		`def f fn [[n:Integer s:Integer] [Any] [ for [0 n s] [ 5 drop ] 0 ]] f 6 2`,
	} {
		if n, ds := countErrors(t, src); n != 0 {
			t.Errorf("expected 0 errors for %q, got %d: %v", src, n, ds)
		}
	}
}

// A computed-END range loop is a compiler advance, not just a checker one: the
// const-start/step forms lower to FOR_SETUP with a resolved end operand and run
// COMPILED (byte-identical to the interpreter). The computed-start/step forms
// correctly decline and fall back. This pins the compile/interpret parity.
func TestComputedRangeLoopCompilesAndMatches(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	type wc struct {
		src         string
		wantCompile bool
	}
	cases := []wc{
		{`def f fn [[n:Integer] [Any] [ for [0 n 65536] [ 5 drop ] 0 ]] f 3`, true},
		{`def f fn [[n:Integer] [Any] [ for [n] [ 5 drop ] 0 ]] f 4`, true},
		{`def f fn [[n:Integer] [Any] [ for [2 n] [ 5 drop ] 0 ]] f 5`, true},
		// GRADUATED 2026-07-17: a computed range START/STEP that resolves to
		// a frame LOCAL now lowers (computedRangeBounds passes the bounds
		// as-is; RecordLoop admits const + local operands, declining events).
		{`def f fn [[a:Integer b:Integer] [Any] [ for [a b] [ 5 drop ] 0 ]] f 1 4`, true},
		{`def f fn [[n:Integer s:Integer] [Any] [ for [0 n s] [ 5 drop ] 0 ]] f 6 2`, true},
	}
	for _, c := range cases {
		ac, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		gotC, compiled, eC := ac.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, eC) {
			continue
		}
		ai, _ := New()
		gotI, eI := ai.RunInterp(c.src)
		if eC != nil || eI != nil {
			t.Errorf("%q: run errs compiled=%v interp=%v", c.src, eC, eI)
			continue
		}
		if compiled != c.wantCompile {
			t.Errorf("%q: compiled=%v want %v", c.src, compiled, c.wantCompile)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: MISCOMPILE compiled=%v interp=%v", c.src, gotC, gotI)
		}
	}
}

// The negative side of the same contract: a VALUE-producing loop genuinely
// leaves one residual per iteration, so a fn declaring a single return whose
// body is `for n [5] 0` really does over-return (n copies plus the trailing 0).
// The 0-net guard must NOT suppress this true error.
func TestForValueProducingBodyStillErrors(t *testing.T) {
	src := `def f fn [[n:Integer] [Any] [ for n [ 5 ] 0 ]] f 3`
	n, ds := countErrors(t, src)
	if n == 0 {
		t.Fatalf("expected a return-count error for a value-producing loop, got none")
	}
	found := false
	for _, d := range ds {
		if containsSub(d, "return value") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'return value(s)' error, got %v", ds)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
