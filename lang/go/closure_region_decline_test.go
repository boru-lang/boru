package lang

import (
	"fmt"
	"testing"
)

// The REAL-unit re-check on a whole-residual dispatch's region residual (the
// fifty-seventh increment), and the unreachable-unit recovery it needs.
//
// closureResidualRegion is a SHAPE test, and the probe it is first asked of
// does not see the shape the real compile builds: the probe carries no
// producedBy, so an enclosing binding read that an EVENT produces bakes there
// as a CONST and routes live in the real compile. closureResidualExact
// survives that because it COUNTS operands and the count does not change;
// regionPrefixShapeOps does not, because it requires everything beneath the
// run to be inert.
//
// Measured on this row: the probe's do$body residual is [CONST CONST REGION],
// the real one [EVENT:0 EVENT:1 REGION]. Admitting on the probe alone
// recorded a closure whose unit then had no seating, and the LOWERING refuses
// with no fall-through — so a row that compiled through the dyn-body strategy
// became a hard refusal. The variation lane caught it
// (TestVariationDifferential, transform `for-body`), which is exactly what
// that lane is for: the SEED is a corpus row and the VARIANT is not.
//
// The row is pinned here rather than in lang/spec because it reaches the
// dyn-body strategy, and the interp-entry census is a ratchet that may only
// fall.
func TestWholeResidualRegionDeclinesOnTheRealUnit(t *testing.T) {
	const src = `for 2 [def b true  do [1 2 (if b [] [9 9])]]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, cerr := a.RunCompiledStrict(src)
	if cerr != nil {
		t.Fatalf("must compile: %v", cerr)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want, ierr := b.RunInterp(src)
	if ierr != nil {
		t.Fatalf("the interpreter must run it clean: %v", ierr)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("lane disagreement: compiled %v, interpreted %v", got, want)
	}
}
