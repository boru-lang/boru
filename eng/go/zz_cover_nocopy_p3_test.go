package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestP3VMNativeApplicableNeedsAFnDefinition: only a value carrying a fn
// definition can apply natively on the VM; a scalar and a compiled closure
// (no Go handler of its own) cannot. The callers screen for a definition
// first since a binding names its holder (no-copy phase 3), so the guard is
// pinned here.
func TestP3VMNativeApplicableNeedsAFnDefinition(t *testing.T) {
	r := seam7Reg(t)
	n := core.NewInteger(1)
	if vmNativeApplicable(r, &n) {
		t.Error("a scalar applied natively")
	}
	cl := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{}}
	if vmNativeApplicable(r, &cl) {
		t.Error("a compiled closure applied natively")
	}
}
