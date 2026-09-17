package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Per-alternative disjunct dispatch (design/legacy/checker-accuracy-review.10.ignore
// A1): a strict disjunct input must dispatch as the JOIN of its
// alternatives' first-match dispatches, never as a single first-match
// on the whole disjunct.

// Positive: Integer|String reaching add joins the Integer path
// ([Number Number]) with the String path ([Scalar Scalar] → String) —
// the result must NOT be the bare String the whole-disjunct match
// used to produce (the runtime takes the Number path for the Integer
// alternative).
func TestCheckDisjunctDispatchJoinsAlternatives(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// The disjunct is built with a LIST-form condition: a paren condition
	// `(1 gt 0)` now folds to a concrete Boolean and reduces the `if` to one
	// arm (else-less-if soundness fix), which would collapse the disjunct we
	// need here. The list-form `[1 gt 0]` is a deferred code body that stays a
	// branch join.
	res, err := a.Check(`def y (if [1 gt 0] [1] ['s']) y add 1`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	if res.Stack[0] == "String" {
		t.Fatalf("whole-disjunct first-match regression: got bare String, want a join including the Integer path")
	}
	if res.Stack[0] != "Disjunct" {
		t.Errorf("expected Disjunct join of per-alternative returns, got %q", res.Stack[0])
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %s: %s", d.Code, d.Detail)
		}
	}
}

// Negative: an alternative that reaches NO overload is flagged with a
// partial_dispatch warning — that path would fail dispatch at runtime.
func TestCheckDisjunctDispatchPartialWarning(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// mod: [Number Number] only — no catch-all. Integer|String
	// straddles it: the String alternative has no overload.
	// List-form condition — see TestCheckDisjunctDispatchJoinsAlternatives.
	res, err := a.Check(`def y (if [1 gt 0] [1] ['s']) mod y 2`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	foundPartial := false
	for _, d := range res.Diagnostics {
		if d.Code == "partial_dispatch" && strings.Contains(d.Detail, "mod") {
			foundPartial = true
		}
		if d.Code == "no_signature" {
			t.Errorf("blanket no_signature emitted although the Integer alternative dispatches: %s", d.Detail)
		}
	}
	if !foundPartial {
		t.Errorf("expected a partial_dispatch warning for the String alternative of `mod`; diagnostics: %+v", res.Diagnostics)
	}
}
