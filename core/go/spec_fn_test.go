package core

import "testing"

// NoteSpecFnDef marks a fn family SPECULATIVE (the seventieth increment):
// the check state remembers it, the recorder is handed the placed install
// (a nil registry or an empty name is a no-op), and the branch join pushes
// the model's binding for such a name while noting NO transition — the
// arm's install is placed at its own site, and a root twin replayed before
// the branch bound the name whether or not the arm ran.
func TestNoteSpecFnDefAndJoin(t *testing.T) {
	NoteSpecFnDef(nil, "f", Value{}, SrcPos{})
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	NoteSpecFnDef(r, "", Value{}, SrcPos{})
	if len(r.Check.SpecFnNames) != 0 || specFnJoin(r, "f") || specFnJoin(nil, "f") {
		t.Fatal("an empty name marks nothing")
	}
	r.Check.Mode = true
	NoteSpecFnDef(r, "f", Value{}, SrcPos{Row: 1, Col: 3})
	if !r.Check.SpecFnNames["f"] || !specFnJoin(r, "f") {
		t.Fatal("the name is marked speculative")
	}
	fnv := NewFunction(FnDefInfo{Name: "f"})
	base := len(r.Check.BindLedger)
	InstallJoinedDefs(r, map[string]Value{"f": fnv, "g": NewCarrier(TInteger)}, nil)
	if _, ok := r.Defs.Top("f"); !ok {
		t.Fatal("the join pushes the model's binding")
	}
	noted := map[string]bool{}
	for _, tr := range r.Check.BindLedger[base:] {
		noted[tr.Name] = true
	}
	if noted["f"] || !noted["g"] {
		t.Fatalf("the join notes no transition for a speculative fn family, and every other: %v", noted)
	}
	r.Defs.Pop("f")
	base = len(r.Check.BindLedger)
	InstallJoinedDefs(r, nil, map[string]Value{"f": fnv})
	if _, ok := r.Defs.Top("f"); !ok || len(r.Check.BindLedger) != base {
		t.Fatal("the else-only join pushes the binding and notes nothing")
	}
	// Clone deep-copies the set; Begin clears it.
	cp := r.Check.Clone()
	cp.SpecFnNames["h"] = true
	if r.Check.SpecFnNames["h"] {
		t.Fatal("Clone copies the set")
	}
}
