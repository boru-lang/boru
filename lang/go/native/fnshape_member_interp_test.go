package native

import (
	"testing"
)

// Interpreter-only pin for the TRAILING spelling of a fn-shape-typed
// class-member apply (`5 ((make C {…}) dot op)`), kept OUT of the spec
// corpus so the compiled ratchets (compiled_coverage refusalCeiling=0)
// stay untouched — the same split fn-triple.tsv documents. The compiled
// lane REFUSES this shape ("unconsumed fn-value carrier in residual"):
// with fn-shape carriers recognised as maybe-callable (core.
// IsFnTypedCarrier — the NUR095 retirement), the trailing residual is a
// carrier the trailing-apply lowering does not claim, and refusal is the
// refusal the interpreter absorbs where the pre-fix recorder silently compiled the fn as
// inert data. The leading spellings compile and are pinned as class.tsv
// fn-members rows.
func TestFnShapeMemberTrailingApplyInterp(t *testing.T) {
	r := seam5Reg(t)
	out, err := seam5Run(r, `def T fnsig Integer Integer
def C class {op:T}
def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])})
5 c.op`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want the applied result alone, got %v", out)
	}
	if n, nerr := out[0].AsConcreteInteger(); nerr != nil || n != 6 {
		t.Fatalf("trailing member apply must yield 6, got %v (%v)", out, nerr)
	}
}
