package core

import "testing"

// NoteSpecFnDef marks a fn family SPECULATIVE (the seventieth increment)
// exactly when the recorder places it — the inactive recorder, a nil
// registry and an empty name place nothing — and the branch join pushes
// the model's binding for such a name while noting NO transition: the
// arm's install is placed at its own site, and a root twin replayed before
// the branch bound the name whether or not the arm ran. EnterSpecArm
// brackets the arms whose condition the model cannot decide.
func TestNoteSpecFnDefAndJoin(t *testing.T) {
	fnv := NewFunction(FnDefInfo{Name: "f"})
	if NoteSpecFnDef(nil, "f", Value{}, fnv, SrcPos{}) {
		t.Fatal("a nil registry places nothing")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if NoteSpecFnDef(r, "", Value{}, fnv, SrcPos{}) || NoteSpecFnDef(r, "f", Value{}, fnv, SrcPos{Row: 1, Col: 3}) {
		t.Fatal("an empty name, and the inactive recorder, place nothing")
	}
	if len(r.Check.SpecFnNames) != 0 || specFnJoin(r, "f") || specFnJoin(nil, "f") || r.analysisInSpecArm() {
		t.Fatal("nothing is marked")
	}
	leave := r.EnterSpecArm(false)
	if !r.analysisInSpecArm() {
		t.Fatal("inside an undecidable arm")
	}
	leave()
	if r.analysisInSpecArm() {
		t.Fatal("left the arm")
	}
	r.EnterSpecArm(true)()
	if r.analysisInSpecArm() {
		t.Fatal("a known arm takes no bracket")
	}
	r.Check.Mode = true
	r.Check.SpecFnNames = map[string]bool{"f": true}
	if !specFnJoin(r, "f") {
		t.Fatal("the name is marked speculative")
	}
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
