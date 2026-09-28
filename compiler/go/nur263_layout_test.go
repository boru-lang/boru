package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestLayoutForBeforeARegistryIsBound pins the nil-safe read of a published
// dispatch layout (NUR263): an emit state no check run has bound yet reads
// none.
func TestLayoutForBeforeARegistryIsBound(t *testing.T) {
	if NewEmitState().layoutFor([]core.Value{core.NewInteger(1)}) != nil {
		t.Error("no bound registry: no layout")
	}
}
