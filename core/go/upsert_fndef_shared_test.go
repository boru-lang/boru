package core

import "testing"

// TestUpsertFnDefLeavesEarlierHolders — registering another signature under
// a name builds the merged set as a new array: a value read before the
// registration (a Lookup, an `f/v` value, a table clone) keeps dispatching
// on the signatures it saw, in the order it saw them. The merge appended
// into the previous array's spare capacity and sorted it in place, so an
// earlier holder's signatures were reordered under it.
func TestUpsertFnDefLeavesEarlierHolders(t *testing.T) {
	r := newTestRegistry(t)
	echo := Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil })
	// Register until the binding's array has spare capacity: the shape in
	// which an append writes into the array an earlier holder shares.
	var heldFn *FnDefInfo
	for _, ty := range []*Type{TAny, TString, TBoolean, TList, TMap, TAtom, TFloat, TPathon} {
		r.upsertFnDef("w", Signature{Args: []*Type{ty}, Impl: echo})
		held, _ := r.Defs.Top("w")
		if fn, _ := FnDefOf(held.Data); cap(fn.Signatures) > len(fn.Signatures) {
			heldFn = fn
			break
		}
	}
	if heldFn == nil {
		t.Fatal("no registration left spare capacity")
	}
	want := make([]*Type, len(heldFn.Signatures))
	for i := range heldFn.Signatures {
		want[i] = heldFn.Signatures[i].Params[0].Type
	}
	// More specific than Any: sorts ahead of it.
	r.upsertFnDef("w", Signature{Args: []*Type{TInteger}, Impl: echo})
	for i := range heldFn.Signatures {
		if got := heldFn.Signatures[i].Params[0].Type; got != want[i] {
			t.Errorf("an earlier holder's signature %d is %s, was %s", i, got.Name(), want[i].Name())
		}
	}
	top, _ := r.Defs.Top("w")
	if topFn, _ := FnDefOf(top.Data); len(topFn.Signatures) != len(want)+1 {
		t.Errorf("the binding has %d signatures, want %d", len(topFn.Signatures), len(want)+1)
	}
}
