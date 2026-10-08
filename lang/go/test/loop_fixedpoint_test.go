package test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Loop-body fixed point (design/legacy/checker-accuracy-review.10.ignore A4):
// rebindings inside a loop body must join back into the enclosing
// binding and re-analyse until stable — post-loop reads used to see
// the PRE-loop type (Integer) while the runtime produced Float.

// Positive: `def acc 0  for 3 [def acc (acc add 0.5)]  acc` — the
// runtime yields 1.5 (Float). The post-loop binding must cover the
// Float path; the old one-pass analysis reported bare Integer.
func TestCheckLoopRebindJoins(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def acc 0 for 3 [def acc (acc add 0.5)] acc`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) == 0 {
		t.Fatalf("no residual stack: %+v", res)
	}
	got := res.Stack[len(res.Stack)-1]
	if got == "Integer" {
		t.Fatalf("post-loop binding kept the pre-loop type Integer; runtime yields Float 1.5")
	}
	if got != "Disjunct" && got != "Float" && got != "Number" {
		t.Errorf("expected a join covering Float, got %q", got)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %s: %s", d.Code, d.Detail)
		}
	}
}

// Positive: the fold accumulator widens through the body — the
// result must cover Float, not type against the Integer init only.
func TestCheckFoldAccumulatorWidens(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`fold [add 0.5] [1 2 3] 0`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if res.Stack[0] == "Integer" {
		t.Errorf("fold accumulator typed against the init only; runtime yields 3.5 (Float)")
	}
}

// Negative: a genuine type error inside the loop body is still
// reported, and exactly once — the fixed-point re-runs must not
// duplicate diagnostics.
func TestCheckLoopBodyErrorReportedOnce(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def acc 0 for 3 [def acc (acc add 0.5) mod 's' 2] acc`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	count := 0
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" && d.Word == "mod" {
			count++
		}
	}
	if count == 0 {
		t.Errorf("loop-body type error not reported; diagnostics: %+v", res.Diagnostics)
	}
	if count > 1 {
		t.Errorf("loop-body diagnostic duplicated %d times across fixed-point rounds", count)
	}
}
