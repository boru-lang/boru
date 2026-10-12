package native

import (
	"testing"
)

// Wave-7 coverage for minilang_compile.go (the no-state arms of
// miniCompileStateFor / miniGoHook) and native_temporal_await.go (the
// branch-error and multi-value awaitFull arms) — design/TEST-SEAMS.10.md.

func TestMiniCompileNoStateArms(t *testing.T) {
	r := b2Reg(t)
	// No state installed and create=false → miniCompileStateFor returns nil.
	if s := miniCompileStateFor(r, false); s != nil {
		t.Fatal("miniCompileStateFor(create=false) with no state should be nil")
	}
	// miniGoHook with no state → (nil, false).
	if h, ok := miniGoHook(r, "re"); ok || h != nil {
		t.Fatalf("miniGoHook with no state = %v/%v, want nil/false", h, ok)
	}
	// miniHookFaithful with no state → false (the s == nil arm).
	if miniHookFaithful(r, "re") {
		t.Fatal("miniHookFaithful with no state should be false")
	}
	// Positive pair: after registering, the hook is discoverable and state
	// is created.
	called := false
	RegisterMiniCompileGoHook(r, "re", func(string, Value, *Registry) ([]Value, error) {
		called = true
		return nil, nil
	})
	h, ok := miniGoHook(r, "re")
	if !ok || h == nil {
		t.Fatal("registered hook should be discoverable")
	}
	if _, err := h("x", NewTypeLiteral(TNone), r); err != nil || !called {
		t.Fatalf("hook call = %v, called=%v", err, called)
	}
	// With state now created, a hook is faithful only after it is marked.
	if miniHookFaithful(r, "re") {
		t.Fatal("hook should not be faithful until marked")
	}
	MarkMiniCompileHookFaithful(r, "re")
	if !miniHookFaithful(r, "re") {
		t.Fatal("a marked hook should be faithful")
	}
}

func TestRunParallelBranchError(t *testing.T) {
	r := b2Reg(t)
	// A list branch whose body raises produces an error result.
	pr := runParallelBranch(r, NewList([]Value{NewWord("zzz_undef_word_xyz")}))
	if !pr.err {
		t.Fatal("runParallelBranch of erroring body should set err=true")
	}
	// Positive pair: a non-list element passes through.
	pr = runParallelBranch(r, NewInteger(5))
	if pr.err || len(pr.values) != 1 {
		t.Fatalf("runParallelBranch(5) = %+v", pr)
	}
	// A plain fn VALUE element carrying no compiled ref (an fn stored in a
	// def'd parallels list) is not a compiled-branch carrier: it passes
	// through as-is, exactly like any other non-list element.
	fnVal := Value{Parent: TFunction, Data: &FnDefInfo{
		Signatures: []Signature{{Impl: &BoruImpl{Body: []Value{NewInteger(1)}}}},
	}}
	pr = runParallelBranch(r, fnVal)
	if pr.err || len(pr.values) != 1 || !pr.values[0].Parent.Equal(TFunction) {
		t.Fatalf("runParallelBranch(plain fn) = %+v, want the fn value passed through", pr)
	}
}

func TestAwaitFullMultiValueBranch(t *testing.T) {
	r := b2Reg(t)
	// A branch whose body leaves two values on the stack takes awaitFull's
	// "wrap in a List" arm; a scalar branch takes the single-value arm.
	elems := []Value{
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewInteger(9),
	}
	out, err := awaitFull(r, elems)
	if err != nil || len(out) != 1 {
		t.Fatalf("awaitFull = %v / %v", out, err)
	}
	rl, aerr := AsList(out[0])
	if aerr != nil || rl.Len() != 2 {
		t.Fatalf("awaitFull result should have 2 status maps, got %v", out[0])
	}
}
