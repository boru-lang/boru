package lang

import "testing"

// TestNUR211StackFormCountOverAComputedBody pins NUR211's close. A
// statically-failed dispatch whose window holds a carrier re-matches at run
// time (DISPATCH_REMATCH). The rematch read the window flat, stack values at
// the leading positions, so `3 for (mk)` matched `for 3 [i]` and bailed with
// an internal_error. The interpreter's forward phase puts the written `(mk)`
// first: a List where the count goes stops it, the stack's 3 fills the count,
// and the body finds nothing beneath, so the interpreter raises
// signature_error. The rematch now plans the window with the interpreter's
// split (DispatchSpec.NFwd), and both lanes raise the same error.
func TestNUR211StackFormCountOverAComputedBody(t *testing.T) {
	const mk = `def mk fn [[][List][quote [i]]] end `
	for _, src := range []string{
		mk + `3 for (mk)`,
		mk + `(1 add 2) for (mk)`,
		mk + `3 each (mk)`,
	} {
		requireEngineParity(t, src, true)
		if _, err := mustNew(t).RunInterp(src); codeOf(err) != "signature_error" {
			t.Errorf("%s: the interpreter raises signature_error, got %v", src, err)
		}
	}
	// Negative: the written count and the range form answer, and a written
	// range that is not an integer list raises the loop's own error, on
	// both lanes.
	requireEngineParity(t, mk+`for 3 (mk)`, true)
	requireEngineParity(t, mk+`[1 2] for (mk)`, true)
	requireEngineParity(t, `def mk fn [[][List][quote [add 1]]] end [1 2] each (mk)`, true)
}
