package core

import "testing"

// TestClosureSigMatched pins the SigMatched mark (ClosurePayload.SigMatched):
// a closure value is returned as a marked copy, a marked one as itself, and a
// non-closure value untouched.
func TestClosureSigMatched(t *testing.T) {
	plain := NewInteger(1)
	if got := ClosureSigMatched(plain); !ExactEqual(got, plain) {
		t.Errorf("a non-closure value must pass through: %v", got)
	}
	cl := Value{Parent: TFunction, Data: ClosurePayload{Unit: 3}, Quoted: true}
	marked := ClosureSigMatched(cl)
	mcl, ok := marked.Data.(ClosurePayload)
	if !ok || !mcl.SigMatched || mcl.Unit != 3 || !marked.Quoted {
		t.Errorf("the mark must set SigMatched on a copy keeping the payload and the quote: %v", marked)
	}
	if ocl := cl.Data.(ClosurePayload); ocl.SigMatched {
		t.Error("the stored closure must stay unmarked")
	}
	again := ClosureSigMatched(marked)
	if acl := again.Data.(ClosurePayload); !acl.SigMatched {
		t.Error("a marked closure stays marked")
	}
}

// TestClosureAsFnDefInactiveRuntime pins the native seams' bridge reach under
// the interpreter-only runtime: it declines, leaving the value as data.
func TestClosureAsFnDefInactiveRuntime(t *testing.T) {
	r := covRegistry(t, nil)
	cl := Value{Parent: TFunction, Data: ClosurePayload{Unit: 4}}
	got, ok := ClosureAsFnDef(r, cl)
	if gcl, isCl := got.Data.(ClosurePayload); ok || !isCl || gcl.Unit != 4 {
		t.Errorf("the inactive runtime must decline the bridge and hand the value back: %v %v", got, ok)
	}
}
