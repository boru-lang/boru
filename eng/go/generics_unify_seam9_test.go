package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// W9 generics_unify.go coverage: the defensive AsWord / AsParenExpr error
// arms of ResolveSigChildParam / ResolveChildTypeExpr, reachable by a
// ChildType whose Child carries a mismatched (Parent, payload) pair.

func TestW9ResolveSigChildParamMalformedWord(t *testing.T) {
	r := newTestRegistry(t)
	// Child has a Word parent but a non-WordInfo payload: IsWord passes,
	// AsWord fails → v returned unchanged.
	v := core.ValueOf(&core.Datum{Parent: core.TList, Data: core.ChildTypeInfo{Child: core.ValueOf(&core.Datum{Parent: core.TWord, Data: core.ListPayload{}})}})
	got := core.ResolveSigChildParam(r, v)
	if _, ok := got.Data().(core.ChildTypeInfo); !ok {
		t.Error("a malformed child word should return v unchanged")
	}
}

func TestW9ResolveChildTypeExprMalformedParen(t *testing.T) {
	r := newTestRegistry(t)
	// Child has a ParenExpr parent but a non-ParenExpr payload: IsParenExpr
	// passes, AsParenExpr fails → v returned unchanged, no error.
	v := core.ValueOf(&core.Datum{Parent: core.TList, Data: core.ChildTypeInfo{Child: core.ValueOf(&core.Datum{Parent: core.TParenExpr, Data: core.ListPayload{}})}})
	got, err := core.ResolveChildTypeExpr(r, v)
	if err != nil {
		t.Fatalf("a malformed paren child should not error: %v", err)
	}
	if _, ok := got.Data().(core.ChildTypeInfo); !ok {
		t.Error("a malformed paren child should return v unchanged")
	}
}
