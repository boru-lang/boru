package core

import "testing"

// fn_identity_test.go pins the closure identity token (Codex P1 on PR #444):
// a compiled closure carries the token its push minted, `eq` compares it —
// two copies of one closure are one function, two constructions are two, a
// token-less payload is nothing — and the FnDefInfo the VM bridges a closure
// to adopts the same token (NewFunctionIdentified), so a bridged copy is eq
// to the closure and to every other bridge of it, either way round. A zero
// identity handed to NewFunctionIdentified mints afresh, as NewFunction does.
func TestClosureIdentity(t *testing.T) {
	id := NewFnIdentity()
	c1 := Value{Parent: TFunction, Data: ClosurePayload{Ident: id}}
	c2 := c1
	other := Value{Parent: TFunction, Data: ClosurePayload{Ident: NewFnIdentity()}}
	bare := Value{Parent: TFunction, Data: ClosurePayload{}}
	if !ExactEqual(c1, c2) {
		t.Error("two copies of one closure are one function")
	}
	if ExactEqual(c1, other) {
		t.Error("two constructions are two functions")
	}
	if ExactEqual(bare, bare) || ExactEqual(bare, c1) {
		t.Error("a token-less closure has no identity")
	}
	bridged := NewFunctionIdentified(FnDefInfo{Anonymous: true}, id)
	again := NewFunctionIdentified(FnDefInfo{Anonymous: true}, id)
	if !ExactEqual(bridged, c1) || !ExactEqual(c1, bridged) || !ExactEqual(bridged, again) {
		t.Error("a bridged copy is the closure, either way round, and so is a second bridge")
	}
	plain := NewFunction(FnDefInfo{})
	if ExactEqual(plain, c1) || ExactEqual(c1, plain) || ExactEqual(c1, NewInteger(1)) || ExactEqual(NewInteger(1), c1) {
		t.Error("another function, or data, is not the closure")
	}
	minted := NewFunctionIdentified(FnDefInfo{}, FnIdentity{})
	if md := minted.Data.(FnDefInfo); md.ident == nil || ExactEqual(minted, plain) || !ExactEqual(minted, minted) {
		t.Error("a zero identity mints afresh")
	}
	// Two plain fns still compare by their own tokens (the closure arm is
	// not handled for them), and a plain fn carries no closure sequence.
	if eq, handled := closureIdentityEqual(plain, NewFunction(FnDefInfo{})); eq || handled {
		t.Error("no closure on either side: not this arm")
	}
	if seq, isClosure := closureIdentSeqOf(plain); seq != 0 || isClosure {
		t.Error("a plain fn has no closure identity")
	}
	if seq, isClosure := closureIdentSeqOf(NewInteger(1)); seq != 0 || isClosure {
		t.Error("data has no closure identity")
	}
	// The identity is a sequence, not a heap token: constructing one
	// allocates nothing (the VM mints one per closure push on its hot path).
	if n := testing.AllocsPerRun(100, func() { _ = NewFnIdentity() }); n != 0 {
		t.Errorf("NewFnIdentity allocates %v per call", n)
	}
}
