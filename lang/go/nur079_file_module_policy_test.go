package lang

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// TestNUR079CheckReportsRefusedImport: the check pass reports a refused
// top-level import as the error the run raises, under the same code — not
// an opaque module whose names then read as undefined.
func TestNUR079CheckReportsRefusedImport(t *testing.T) {
	pol, err := policy.Load("read-only")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Policy: pol})
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Check(`import "boru:net" 1`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range res.Diagnostics {
		found = found || d.Code == "permission_denied"
	}
	if !found {
		t.Errorf("want a permission_denied diagnostic, got %v", res.Diagnostics)
	}
	// Negative: an allowed import reports nothing.
	b, _ := New(Options{Policy: pol})
	if res, err := b.Check(`import "boru:math-util" 1`); err != nil || len(res.Diagnostics) != 0 {
		t.Errorf("an admitted import must check clean, got %v / %v", res.Diagnostics, err)
	}
}
