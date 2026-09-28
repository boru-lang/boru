package core

import "testing"

// TestWithFreshFnIdentity pins NUR288's re-identification: a fn value comes
// back a NEW function — not eq to the one it was minted from, eq to itself —
// and any other value comes back unchanged.
func TestWithFreshFnIdentity(t *testing.T) {
	fn := NewFunction(FnDefInfo{Anonymous: true})
	fresh := WithFreshFnIdentity(fn)
	if ValuesEqual(fn, fresh) {
		t.Error("a fresh identity is another function")
	}
	if !ValuesEqual(fresh, fresh) {
		t.Error("a function is eq to itself")
	}
	n := NewInteger(3)
	if got := WithFreshFnIdentity(n); got.Data != n.Data {
		t.Error("a value that is no fn is returned as it is")
	}
}
