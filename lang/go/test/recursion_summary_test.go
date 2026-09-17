package test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Recursion summaries (design/legacy/checker-accuracy-review.10.ignore A2).

// Positive: an UNCHECKED recursive fn's summary is refined past the
// Any bail-out — the recursive call's result participates in the
// final type instead of poisoning the cached summary with Any.
func TestCheckUncheckedRecursionRefines(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// No declared returns ([] output): the in-flight bail used to
	// cache dynamic/Any permanently; refinement re-runs with the
	// first round's result as the hypothesis.
	res, err := a.Check(`def s fn [[n:Integer] [] [if (n lte 0) [0] [n add (s (n sub 1))]]] s 5`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) == 0 {
		t.Fatalf("no residual stack")
	}
	got := res.Stack[len(res.Stack)-1]
	if got == "Any" || got == "dynamic(Any)" {
		t.Errorf("unchecked recursion still collapses to %s; refinement should recover a numeric join", got)
	}
}

// Negative/guarantee: a DECLARED recursive fn keeps using its
// declaration as the induction hypothesis — a misuse of the
// recursive result inside the body is flagged.
func TestCheckDeclaredRecursionBodyMisuse(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`def f fn [[n:Integer] [Integer] [if (n lte 0) [0] [mod 's' (f (n sub 1))]]] f 3`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if (d.Code == "no_signature" || d.Code == "partial_dispatch") && d.Word == "mod" {
			found = true
		}
	}
	if !found {
		t.Errorf("misuse of declared recursive result not flagged; diagnostics: %+v", res.Diagnostics)
	}
}
