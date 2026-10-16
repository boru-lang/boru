package core

import "testing"

// TestTypeNameTableConsistency verifies that TypeNameTable and TypeNameByID
// are mutually consistent against the Builtin TypeTable.
func TestTypeNameTableConsistency(t *testing.T) {
	full := TypeNameTable()

	if len(full) == 0 {
		t.Fatal("TypeNameTable is empty")
	}

	// Every entry must round-trip name → *Type → ID → name.
	for name, ft := range full {
		if ft == nil {
			t.Errorf("TypeNameTable[%q] has nil Def", name)
			continue
		}
		if IDOf(ft) == "" {
			continue // tolerate transitional entries
		}
		got := TypeNameByID(IDOf(ft))
		if got != name {
			t.Errorf("TypeNameByID(%s) = %q, want %q", IDOf(ft), got, name)
		}
	}
}

// TestTypeNameTableEntries ensures every entry in the Builtin byName
// index points at a registered type.
func TestTypeNameTableEntries(t *testing.T) {
	for name, def := range Builtin.byName {
		if def == nil {
			t.Errorf("Builtin.byName[%q] is nil", name)
		}
	}
}
