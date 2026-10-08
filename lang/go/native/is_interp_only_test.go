package native

import (
	"strings"
	"testing"
)

// Interpreter-only pins for two `is`/`exposes` arms the compiled lane
// deliberately declines (a fn value reaching a fn-invoking word is the
// Stage 3 boundary), kept OUT of the spec corpus so the compiled
// ratchets (compiled_coverage failureCeiling=0) stay untouched — the
// same split fn-triple.tsv documents for computed operands.

// An inline predicate whose body RAISES answers false, not an error —
// the RunPredicate error arm of the concrete-fn RHS.
func TestIsInlinePredicateErrorAnswersFalse(t *testing.T) {
	r := seam5Reg(t)
	out, err := seam5Run(r, `5 is (fn [[x:Integer] [Boolean] [(1 div 0) gt 0]])`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want one result, got %v", out)
	}
	if b, berr := out[len(out)-1].AsConcreteBoolean(); berr != nil || b {
		t.Fatalf("a raising predicate answers false, got %v (%v)", out, berr)
	}
}

// An ANONYMOUS class exposer resolves through its payload's minted node
// — and is held to the surface contract (the class-payload arm of
// exposerNode).
func TestExposesAnonymousClassExposer(t *testing.T) {
	r := seam5Reg(t)
	_, err := seam5Run(r, `def Shape surface {area: (fnsig [[Self] [Float]])}
(class {r:1.0}) exposes Shape`)
	if err == nil || !strings.Contains(err.Error(), "does not expose") {
		t.Fatalf("an anonymous exposer without the ops must be declined, got %v", err)
	}
}
