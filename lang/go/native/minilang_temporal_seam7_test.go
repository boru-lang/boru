package native

import (
	"testing"
)

// Wave-7 coverage for minilang_compile.go (the no-state arms of
// miniCompileStateFor / miniGoHook) and native_temporal_await.go (the
// branch-error and multi-value awaitFull arms) — design/TEST-SEAMS.10.md.

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
	fnVal := ValueOf(&Datum{Parent: TFunction, Data: &FnDefInfo{
		Signatures: []Signature{{Impl: &BoruImpl{Body: []Value{NewInteger(1)}}}},
	}})
	pr = runParallelBranch(r, fnVal)
	if pr.err || len(pr.values) != 1 || !pr.values[0].Parent().Equal(TFunction) {
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
