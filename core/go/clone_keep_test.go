package core

import "testing"

// TestCloneValueKeeping: the selective (spine-only) freshen. A compound
// whose ID is in the keep set comes back as the same instance while the
// literal's own spine — the outer list, a nested literal — is minted anew;
// a keep the value does not contain changes nothing, and a nil keep is
// CloneValue.
func TestCloneValueKeeping(t *testing.T) {
	member := NewList([]Value{NewInteger(9)})
	SetID(&member, "bind-c")
	inner := NewList([]Value{member})
	SetID(&inner, "lit-inner")
	om := NewOrderedMap()
	om.Set("m", member)
	mapLit := NewMap(om)
	SetID(&mapLit, "lit-map")
	outer := NewList([]Value{inner, member, mapLit})
	SetID(&outer, "lit-outer")

	keep := map[string]bool{"bind-c": true}
	got := CloneValueKeeping(outer, keep)
	if IDOf(&got) == IDOf(&outer) || IDOf(&got) == "" {
		t.Fatalf("the outer spine must be a fresh instance: id %q (was %q)", IDOf(&got), IDOf(&outer))
	}
	elems := got.Data.(ListPayload).Elems
	if len(elems) != 3 {
		t.Fatalf("clone has %d elems, want 3", len(elems))
	}
	if IDOf(&elems[0]) == IDOf(&inner) {
		t.Errorf("the nested literal must be fresh too: id %q", IDOf(&elems[0]))
	}
	if kept := elems[0].Data.(ListPayload).Elems[0]; IDOf(&kept) != "bind-c" {
		t.Errorf("the member inside the nested literal must be the kept instance: id %q", IDOf(&kept))
	}
	if IDOf(&elems[1]) != "bind-c" {
		t.Errorf("the direct member must be the kept instance: id %q", IDOf(&elems[1]))
	}
	if IDOf(&elems[2]) == IDOf(&mapLit) {
		t.Errorf("the map literal must be fresh: id %q", IDOf(&elems[2]))
	}
	if mv, _ := elems[2].Data.(MapPayload).M.Get("m"); IDOf(&mv) != "bind-c" {
		t.Errorf("the map's member must be the kept instance: id %q", IDOf(&mv))
	}
	// A keep naming nothing the value contains is a plain deep clone.
	plain := CloneValueKeeping(outer, map[string]bool{"elsewhere": true})
	if pe := plain.Data.(ListPayload).Elems; IDOf(&pe[1]) == "bind-c" || IDOf(&pe[0]) == IDOf(&inner) {
		t.Errorf("a keep the value does not contain must not share: %q %q", IDOf(&pe[1]), IDOf(&pe[0]))
	}
	// Nil keep is CloneValue.
	if ce := CloneValue(outer).Data.(ListPayload).Elems; IDOf(&ce[1]) == "bind-c" {
		t.Errorf("CloneValue must clone the member: %q", IDOf(&ce[1]))
	}
}
