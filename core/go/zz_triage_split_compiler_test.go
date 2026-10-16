package core

import "testing"

// Test blocks re-homed by compiler-driven triage at the carve.

func TestImplicitEndOnTypeMismatchArrival(t *testing.T) {
	// `ccat "a" 1 2 cadd` — ccat collects "a", then the Integer arrival
	// mismatches its String slot: implicit end resolves the forward
	// early (curryOrStack), and the trailing cadd consumes 1 2.
	r := covRegistry(t, nil)
	out, err := NewTop(r).Run([]Value{
		NewWord("ccat"), NewString("a"), NewString("b"),
		NewInteger(1), NewInteger(2), NewWord("cadd"),
	})
	if err != nil {
		t.Fatalf("mixed statement: %v", err)
	}
	if got := renderAll(out); got != "'ab' | 3" {
		t.Logf("mixed statement rendered %q", got)
	}

	// Genuine starvation: the arriving value mismatches and nothing can
	// complete the forward — the statement errors.
	_, err = NewTop(r).Run([]Value{
		NewWord("ccat"), NewString("a"), NewInteger(1), NewEnd(),
	})
	if err == nil {
		t.Error("starved ccat did not error")
	}
}
