package core

import "testing"

// TestCloneValueKeeping: the selective (spine-only) freshen. A compound
// whose ID is in the keep set comes back as the same instance while the
// literal's own spine — the outer list, a nested literal — is minted anew;
// a keep the value does not contain changes nothing, and a nil keep is
// CloneValue.
func TestCloneValueKeeping(t *testing.T) {
	member := NewList([]Value{NewInteger(9)})
	member.ID = "bind-c"
	inner := NewList([]Value{member})
	inner.ID = "lit-inner"
	om := NewOrderedMap()
	om.Set("m", member)
	mapLit := NewMap(om)
	mapLit.ID = "lit-map"
	outer := NewList([]Value{inner, member, mapLit})
	outer.ID = "lit-outer"

	keep := map[string]bool{"bind-c": true}
	got := CloneValueKeeping(outer, keep)
	if got.ID == outer.ID || got.ID == "" {
		t.Fatalf("the outer spine must be a fresh instance: id %q (was %q)", got.ID, outer.ID)
	}
	elems := got.Data.(ListPayload).Elems
	if len(elems) != 3 {
		t.Fatalf("clone has %d elems, want 3", len(elems))
	}
	if elems[0].ID == inner.ID {
		t.Errorf("the nested literal must be fresh too: id %q", elems[0].ID)
	}
	if kept := elems[0].Data.(ListPayload).Elems[0]; kept.ID != "bind-c" {
		t.Errorf("the member inside the nested literal must be the kept instance: id %q", kept.ID)
	}
	if elems[1].ID != "bind-c" {
		t.Errorf("the direct member must be the kept instance: id %q", elems[1].ID)
	}
	if elems[2].ID == mapLit.ID {
		t.Errorf("the map literal must be fresh: id %q", elems[2].ID)
	}
	if mv, _ := elems[2].Data.(MapPayload).M.Get("m"); mv.ID != "bind-c" {
		t.Errorf("the map's member must be the kept instance: id %q", mv.ID)
	}
	// A keep naming nothing the value contains is a plain deep clone.
	plain := CloneValueKeeping(outer, map[string]bool{"elsewhere": true})
	if pe := plain.Data.(ListPayload).Elems; pe[1].ID == "bind-c" || pe[0].ID == inner.ID {
		t.Errorf("a keep the value does not contain must not share: %q %q", pe[1].ID, pe[0].ID)
	}
	// Nil keep is CloneValue.
	if ce := CloneValue(outer).Data.(ListPayload).Elems; ce[1].ID == "bind-c" {
		t.Errorf("CloneValue must clone the member: %q", ce[1].ID)
	}
}
