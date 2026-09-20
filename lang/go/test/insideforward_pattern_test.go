package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// matchSignature's inside-pending-forward selection branch (an fn
// dispatching while an enclosing word's Forward is parked, filling
// part-forward part-stack) must enforce signature Patterns exactly like
// the full-forward and normal-stack selection points. Historically it
// returned without consulting patternsOk, so a literal-pattern sig
// (`def p fn [[0 y:Integer] …]`) was selected for a mismatching operand
// — and fn's own tnot-List triple sig would have claimed spec-list
// calls made in that context. The def parks its Forward on `w`, p then
// dispatches inside it with `3` forward and `7` from the stack.
func TestInsideForwardSelectionEnforcesPatterns(t *testing.T) {
	// Negative: 3 fails the literal-0 pattern → the patterned sig is
	// skipped and p (which has no other sig) declines to dispatch.
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	_, rerr := a.Run(`def p fn [[0 y:Integer] [String] ["hit"]]  7 def w p 3`)
	if rerr == nil || !strings.Contains(rerr.Error(), "signature_error") {
		t.Fatalf("pattern-0 sig must be rejected for operand 3 inside a pending forward, got err=%v", rerr)
	}

	// Positive twin: 0 satisfies the pattern — the same mixed
	// forward+stack split dispatches (plain form; the def-parked
	// variant has its own pending-forward binding semantics).
	a2, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	out, rerr := runReference(t, a2, `def p fn [[0 y:Integer] [String] ["hit"]]  7 p 0`)
	if rerr != nil {
		t.Fatalf("pattern-satisfying call must dispatch: %v", rerr)
	}
	if len(out) != 1 || out[0] != "hit" {
		t.Fatalf("want [hit], got %v", out)
	}
}
