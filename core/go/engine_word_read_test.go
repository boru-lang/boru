package core

import (
	"strings"
	"testing"
)

// TestWordPathNilHandlerIsAnError pins the ADR-005 guard in execMatch: a
// binding whose fn value was pushed as a plain def carries no runner on the
// WORD path (an un-installed body), and dispatching it by name is a clean
// internal_error rather than a nil-function panic (NUR125: the const-fold
// sub-run over `{a: k}` with k an Any param holding a 0-arg lambda).
func TestWordPathNilHandlerIsAnError(t *testing.T) {
	r := covRegistry(t, nil)
	raw := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{
		Impl: Boru([]Value{NewInteger(42)}),
	}}})
	r.Defs.Push("k", raw)
	e := NewTop(r)
	_, err := e.Run([]Value{NewWord("k")})
	if err == nil || !strings.Contains(err.Error(), "internal_error") || !strings.Contains(err.Error(), "no runnable implementation for `k`") {
		t.Fatalf("a nil handler on the word path must error cleanly, got %v", err)
	}
}
