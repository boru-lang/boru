package test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// runReferent runs one program and returns its canonical result.
func runReferent(t *testing.T, src string) string {
	t.Helper()
	res, err := runNativeSteps(t, nil, []string{src})
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return core.Canon(res)
}

// TestReferentQuoteCaptures pins that `quote` snapshots the binding a name
// refers to at quote time, and `referent` reads it back.
func TestReferentQuoteCaptures(t *testing.T) {
	if got := runReferent(t, `def x 5  (quote x) referent`); got != "5" {
		t.Errorf("(quote x) referent = %q, want 5", got)
	}
	if got := runReferent(t, `def m {a:1}  (quote m) referent`); got != "{a:1}" {
		t.Errorf("(quote m) referent = %q, want {a:1}", got)
	}
}

// TestReferentSnapshotIsFrozen pins snapshot semantics: a referent captured by
// `quote` reflects the binding AT QUOTE TIME, not a later rebinding.
func TestReferentSnapshotIsFrozen(t *testing.T) {
	got := runReferent(t, `def x 5  def q (quote x)  def x 9  q referent`)
	if got != "5" {
		t.Errorf("frozen snapshot = %q, want 5 (the value when quoted)", got)
	}
}

// TestReferentLazyFallback pins that a bare `name/q` atom (no captured
// referent) resolves against the CURRENT binding when `referent` is called.
func TestReferentLazyFallback(t *testing.T) {
	if got := runReferent(t, `def x 5  def x 9  x/q referent`); got != "9" {
		t.Errorf("x/q referent = %q, want 9 (current binding)", got)
	}
}

// TestReferentPostParsePass pins the run-start resolution pass: when a name is
// already bound at the time a program loads (here, from a prior step on the
// same registry), its /q atom is stamped with the referent up front.
func TestReferentPostParsePass(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`def x 7`, `x/q referent`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := core.Canon(res); got != "7" {
		t.Errorf("post-parse referent = %q, want 7", got)
	}
}

// TestReferentUnboundErrors pins that an atom whose name is unbound has no
// referent and reports an error rather than degrading silently.
func TestReferentUnboundErrors(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{`nope/q referent`})
	if err == nil {
		t.Fatal("expected an error for an unbound atom's referent")
	}
	if !strings.Contains(err.Error(), "referent") {
		t.Errorf("error %q does not mention referent", err.Error())
	}
}

// TestReferentNonAtomRejected pins that `referent` requires an atom — a
// non-atom argument finds no matching signature.
func TestReferentNonAtomRejected(t *testing.T) {
	if _, err := runNativeSteps(t, nil, []string{`referent 5`}); err == nil {
		t.Fatal("expected a signature error for `referent 5`")
	}
}

// TestReferentAtomEqualityUnaffected guards the AtomPayload change: adding a
// Referent snapshot must not perturb atom identity — same-named atoms stay
// equal and canonicalise by name regardless of any captured referent.
func TestReferentAtomEqualityUnaffected(t *testing.T) {
	if got := runReferent(t, `def x 5  (x/q eq x/q)`); got != "true" {
		t.Errorf("x/q eq x/q = %q, want true", got)
	}
	// A quoted atom (with a captured referent) still equals a bare /q atom of
	// the same name (without one).
	if got := runReferent(t, `def x 5  ((quote x) eq x/q)`); got != "true" {
		t.Errorf("(quote x) eq x/q = %q, want true", got)
	}
	// Canon renders an atom by name (as name/q) — a captured referent must
	// not change that, so quote-captured and bare /q atoms canonise alike.
	if got := runReferent(t, `def x 5  x/q`); got != "x/q" {
		t.Errorf("x/q canon = %q, want x/q", got)
	}
	if got := runReferent(t, `def x 5  (quote x)`); got != "x/q" {
		t.Errorf("(quote x) canon = %q, want x/q", got)
	}
}

// TestReferentNestedAtomPerCall pins that the run-start pass stamps a
// nested atom per run and writes nothing a later run reads. A behaviour
// body runs on a top engine once per call, its nested list shared by every
// call; the pass stamped that shared array in place, so the first call's
// referent stuck for every later one ('7' '7' '7').
func TestReferentNestedAtomPerCall(t *testing.T) {
	src := `def Temp refine Integer end ` +
		`behave canon/q (fn [[t:Temp][String][def k [a/q] def a 99 ((k 0 get) referent) canon]]) end ` +
		`[(canon (make Temp 7)) (canon (make Temp 5)) (canon (make Temp 3))]`
	if got := runReferent(t, src); got != `['7' '5' '3']` {
		t.Errorf("nested referent per call = %s, want ['7' '5' '3']", got)
	}
}
