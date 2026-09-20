package core

import "testing"

// SwapDefs is the compile pass's body re-run seam: the recorder swaps a
// cloned START table in around a leaking body's compile and the leaked table
// back afterwards. Three things pinned: the swap returns the table it
// replaced (so the caller can put it back), the swapped-in table is the live
// one for every read in between, and the dispatch cache is dropped with the
// swap — an aggregate cached against the old table must not answer for the
// new one.
func TestSwapDefsExchangesTheLiveTable(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("k", NewInteger(5))
	start := r.Defs.Clone()
	r.Defs.Replace("k", NewInteger(9))
	// Prime the dispatch cache on the live table.
	_ = r.Lookup("k")

	prev := r.SwapDefs(start)
	if prev == nil || prev == start {
		t.Fatalf("SwapDefs must return the table it replaced")
	}
	if v, ok := r.Defs.Top("k"); !ok || v.String() != "5" {
		t.Errorf("after the swap the START table is live: k = %v (present=%v)", v, ok)
	}
	if got := r.dispatchCache; got == nil || len(got.m) != 0 {
		t.Error("the swap must drop the dispatch cache")
	}
	r.SwapDefs(prev)
	if v, _ := r.Defs.Top("k"); v.String() != "9" {
		t.Errorf("swapping the previous table back restores the leaked state: k = %v", v)
	}
}

// The guards: a nil registry and a nil table both swap nothing — the registry
// always has a live table, and a nil would strand every later lookup.
func TestSwapDefsGuards(t *testing.T) {
	var nilR *Registry
	if nilR.SwapDefs(NewDefTable()) != nil {
		t.Error("a nil registry swaps nothing")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	live := r.Defs
	if r.SwapDefs(nil) != nil || r.Defs != live {
		t.Error("a nil table is declined and the live table stays")
	}
}
