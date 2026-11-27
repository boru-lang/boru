package lang

import (
	gofmt "fmt"
	"testing"
)

// check_fn_param_apply_def_fp_test.go — GRADUATED 2026-08-03. This file was
// the expected-open ledger entry for the def-split checker FALSE POSITIVE
// (triaged 2026-08, surfaced by the widened property fuzzer —
// checker-compiler-completeness-review.0.md §8.1(3)/§9.4, the +74 class in
// test/go/langspec/check_run_fp_test.go):
//
//	def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]]
//	zh ([za:Integer] => [za add 1]) 5
//
// The def-split idiom — binding a Function-PARAM application's result, then
// using it — used to flag `undefined_word: zr` at error severity on a program
// that RUNS CLEAN (7): the body analysis left `(f x)` as the un-collapsed
// [Function-carrier, arg] residual, so the pending `def` collection never
// completed and `zr` stayed unbound in the model. The fix is
// `checkModeParenFnCollapse` (core/go/engine.go): on the PLAIN check surface
// (no active recorder — a bare `check` run, or the construction-time
// AnalyseFnBody under a suspended compile pass) a fn-carrier apply window
// collapses to the ONE dynamic(Any) value the interpreter nets, for exactly
// the shapes the compile pass's RecordDynApply admits (trailing `(a b comp)`,
// leading one-arg `(g x)`), keeping the two diagnostic surfaces aligned
// (§8.4.2). The assertions below are the graduated POSITIVE forms.
func TestFnParamApplyDefCheckClean(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"leading def-split (the FP witness)",
			`def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]] zh ([za:Integer] => [za add 1]) 5`,
			"7"},
		{"trailing def-split spelling",
			`def zt fn [[g:Function x:Integer] [Integer] [def zr (x g) g zr]] zt ([za:Integer] => [za add 1]) 5`,
			"7"},
		{"construction alone (no call) stays clean of the FP",
			`def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]] zh/v drop`,
			""},
	} {
		// 1. No error-severity diagnostic — the FP class is dead.
		a, _ := New()
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%s: check harness error: %v", c.name, err)
		}
		for _, d := range res.Diagnostics {
			if d.Severity == SeverityError {
				t.Errorf("%s: unexpected error diagnostic [%s] %s — the def-split FP class regressed", c.name, d.Code, d.Detail)
			}
		}

		// 2. The program runs clean with the expected value.
		if c.want == "" {
			continue
		}
		b, _ := New()
		out, rerr := b.RunInterp(c.src)
		if rerr != nil {
			t.Fatalf("%s: witness must run clean, got %v", c.name, rerr)
		}
		if len(out) != 1 || gofmt.Sprint(out[0]) != c.want {
			t.Errorf("%s: witness result drifted: %v (want [%s])", c.name, out, c.want)
		}
	}
}

// TestFnParamApplyDefFPNegatives pins what the plain-surface collapse must
// NOT silence: a genuinely undefined name still flags, and a multi-arg
// leading window (where the spellings' collection orders diverge and the
// collapse deliberately declines) still nets its un-collapsed residual
// without inventing a binding.
func TestFnParamApplyDefFPNegatives(t *testing.T) {
	// A real typo keeps its undefined_word error.
	a, _ := New()
	res, err := a.Check(`def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zq]] zh ([za:Integer] => [za add 1]) 5`)
	if err != nil {
		t.Fatalf("check harness error: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && d.Severity == SeverityError {
			found = true
		}
	}
	if !found {
		t.Errorf("a genuine typo (zq) must keep its undefined_word error; diagnostics: %v", res.Diagnostics)
	}
}
