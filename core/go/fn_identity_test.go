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

// TestParenTrailingFnApply pins the paren classification's trailing arm
// (the twenty-seventh increment): a window shorter than two values or
// with no last value is not one; a non-Dynamic last value is the fn it
// looks like; a Dynamic last value is the lead only when the recorder holds
// a pending `apply` for it (a gradual r in `(x r apply)`), and never
// without an identity.
func TestParenTrailingFnApply(t *testing.T) {
	rec := &pendingRecorder{pending: "lead"}
	fn := NewFunction(FnDefInfo{})
	if parenTrailingFnApply(rec, fn, 1, 0) || parenTrailingFnApply(rec, fn, 2, -1) {
		t.Error("one value, or no last value: not a trailing apply")
	}
	if !parenTrailingFnApply(rec, fn, 2, 1) || parenTrailingFnApply(rec, NewInteger(1), 2, 1) {
		t.Error("a non-dynamic last value is the fn it is")
	}
	dyn := NewDynamicCarrier(TAny)
	dyn.ID = "lead"
	other := NewDynamicCarrier(TAny)
	other.ID = "other"
	noID := NewDynamicCarrier(TAny)
	noID.ID = ""
	if !parenTrailingFnApply(rec, dyn, 2, 1) || parenTrailingFnApply(rec, other, 2, 1) || parenTrailingFnApply(rec, noID, 2, 1) {
		t.Error("a dynamic last value is the lead exactly when the apply word holds it pending")
	}
	if parenTrailingFnApply(TheInactiveEmit, dyn, 2, 1) {
		t.Error("the inactive recorder holds nothing pending")
	}
	// MarkApplied: only an anonymous, non-macro fn whose every signature
	// is 0-arg takes the mark.
	zero := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{BarrierPos: 0}}})
	if fd := MarkApplied(zero).Data.(FnDefInfo); !fd.Applied {
		t.Error("a 0-arg anonymous fn is marked")
	}
	if fd := MarkApplied(fn).Data.(FnDefInfo); fd.Applied {
		t.Error("a fn with no 0-arg signature is untouched")
	}
	if MarkApplied(NewInteger(1)).Data.(IntPayload).N != 1 {
		t.Error("data is untouched")
	}
}

// pendingRecorder is the inactive recorder with one pending apply id.
type pendingRecorder struct {
	inactiveEmit
	pending string
}

func (p *pendingRecorder) ApplyPending(id string) bool { return id == p.pending }
