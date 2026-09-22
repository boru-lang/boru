package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// freshResidual re-mints each residual value's identity for one call and
// nothing else: the payload, the type and the flags are the memoised value's
// (NUR177, 2026-09-22).
func TestFreshResidual(t *testing.T) {
	c := core.NewCarrier(core.TInteger)
	c.Dynamic = true
	memo := []core.Value{c, core.NewInteger(7)}
	got := freshResidual(memo)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for i := range memo {
		if got[i].ID == "" || got[i].ID == memo[i].ID {
			t.Errorf("value %d must carry a fresh, non-empty ID (memo %q, got %q)", i, memo[i].ID, got[i].ID)
		}
	}
	if !got[0].Carrier || !got[0].Dynamic || !got[0].Parent.Equal(core.TInteger) {
		t.Errorf("a carrier keeps its type and flags, got %#v", got[0])
	}
	if n, err := core.AsInteger(got[1]); err != nil || n != 7 {
		t.Errorf("a concrete value keeps its payload, got %#v (%v)", got[1], err)
	}
	if memo[0].ID == "" && got[0].ID == "" {
		t.Error("the fresh ID must be minted even for an identity-less memo value")
	}
	if fresh := freshResidual(nil); len(fresh) != 0 {
		t.Errorf("an empty residual stays empty, got %d", len(fresh))
	}
}
