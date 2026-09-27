package core

import (
	"strings"
	"testing"
)

// FnIdentityKey renders the identity ExactEqual compares for a fn value, so
// two values with one key are one function and two authored functions never
// share a key; anything without an identity has no key.
func TestFnIdentityKey(t *testing.T) {
	f := NewFunction(FnDefInfo{Name: "f"})
	k1, ok := FnIdentityKey(f)
	if !ok || !strings.HasPrefix(k1, "p") {
		t.Fatalf("an authored fn keys by its token, got %q %v", k1, ok)
	}
	copyOf := f
	if k2, _ := FnIdentityKey(copyOf); k2 != k1 {
		t.Errorf("a copy of one fn keys alike: %q vs %q", k2, k1)
	}
	if k3, _ := FnIdentityKey(NewFunction(FnDefInfo{Name: "f"})); k3 == k1 {
		t.Errorf("a second authored fn must key differently: %q", k3)
	}
	id := NewFnIdentity()
	a := NewFunctionIdentified(FnDefInfo{Name: "a"}, id)
	b := NewFunctionIdentified(FnDefInfo{Name: "b"}, id)
	ka, _ := FnIdentityKey(a)
	kb, okb := FnIdentityKey(b)
	if !okb || ka != kb || !strings.HasPrefix(ka, "c") {
		t.Errorf("two bridges of one closure key alike by its sequence: %q vs %q", ka, kb)
	}
	for _, v := range []Value{NewInteger(1), {Parent: TFunction, Data: FnDefInfo{Name: "hand-built"}}} {
		if k, ok := FnIdentityKey(v); ok {
			t.Errorf("%v has no identity, got key %q", v, k)
		}
	}
}
