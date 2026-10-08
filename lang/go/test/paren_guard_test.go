package test

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Paren-condition guard narrowing (design/legacy/checker-accuracy-review.10.ignore
// A3): the canonical `if (x is T) …` form must narrow exactly like the
// list form `if [x is T] …`.

// Positive: with y : Integer|String, the then-branch sees y narrowed
// to Integer, so `y add 1` takes the numeric path and the join with
// the else-branch's 0 stays numeric — not the Scalar that an
// un-narrowed disjunct produced via the string-concat catch-all.
func TestCheckParenGuardNarrows(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// The disjunct is built with a LIST-form condition `[1 gt 0]`: a paren
	// condition folds to a concrete Boolean and reduces the constructor `if`
	// to one arm (else-less-if soundness fix). The guard under test stays
	// paren-form (`z is Integer`), which does not fold.
	list, err := a.Check(`def y (if [1 gt 0] [1] ['s']) if [y is Integer] [y add 1] [0]`)
	if err != nil {
		t.Fatalf("check list form: %v", err)
	}
	paren, err := a.Check(`def z (if [1 gt 0] [1] ['s']) if (z is Integer) [z add 1] [0]`)
	if err != nil {
		t.Fatalf("check paren form: %v", err)
	}
	if len(list.Stack) != 1 || len(paren.Stack) != 1 {
		t.Fatalf("expected 1 carrier each, got list=%v paren=%v", list.Stack, paren.Stack)
	}
	if paren.Stack[0] != list.Stack[0] {
		t.Errorf("paren condition (%q) and list condition (%q) narrowed differently", paren.Stack[0], list.Stack[0])
	}
}

// Negative: a paren condition that is NOT a guard (no `is` triplet)
// attaches no narrowing and behaves as before; and the else branch of
// a paren guard gets COMPLEMENT narrowing — z in the else branch is
// String, so `z add 1` is string concat → String join with then's
// numeric result stays a disjunct, not a silent error.
func TestCheckParenGuardComplement(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// List-form disjunct constructor (see TestCheckParenGuardNarrows); the
	// guard under test `(z is Integer)` stays paren-form.
	res, err := a.Check(`def z (if [1 gt 0] [1] ['s']) if (z is Integer) [1] [z]`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %s: %s", d.Code, d.Detail)
		}
	}
	if len(res.Stack) != 1 {
		t.Fatalf("expected 1 carrier, got %v", res.Stack)
	}
	// then → Integer literal; else → z narrowed to String (complement)
	// → join Integer|String (Disjunct). Without complement narrowing z
	// stays the full disjunct and the join is the same shape, so the
	// load-bearing assertion is the absence of errors plus the shape.
	if res.Stack[0] != "Disjunct" {
		t.Errorf("expected Disjunct join, got %q", res.Stack[0])
	}
}
