package core

import "testing"

// placingRecorder is the inactive recorder with the speculative fn def
// PLACED: it answers true and keeps what installDef offered it (the name
// and the dropped outer entry), and it keeps every compile failure installDef
// asks for, so the test can see that a placed def raises none.
type placingRecorder struct {
	inactiveEmit
	offered  []string
	outers   []Value
	declined []string
}

func (p *placingRecorder) RecordSpeculativeFnDef(_ *Registry, name string, outer, _ Value, _ SrcPos) bool {
	p.offered = append(p.offered, name)
	p.outers = append(p.outers, outer)
	return true
}

func (p *placingRecorder) MarkUncompilable(reason string) { p.declined = append(p.declined, reason) }

// installDef's two speculative arms (the seventieth increment), driven by
// core's own suite with the recorder placing: a FRESH capture-free fn def
// inside an undecidable arm is offered with a zero outer and marked; an
// overlapping REDEFINITION there is offered with the dropped outer entry,
// marked, and raises no compile failure (family L's text stays for a decline —
// TestConditionalFnDefIsSpeculative's rows in lang pin that under the
// hatch). Outside the arm neither is offered. NoteSpecFnDef's placed path
// marks the name exactly once per family.
func TestInstallDefSpeculativeArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true
	rec := &placingRecorder{}
	r.Check.Emit = rec
	sig := func() Signature {
		return Signature{Params: []FnParam{{Name: "x", Type: TInteger}}, Args: []*Type{TInteger}}
	}
	fnA := NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{sig()}})
	fnB := NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{sig()}})

	// Outside an undecidable arm: nothing is offered.
	InstallDef(r, "g", fnA)
	if len(rec.offered) != 0 || r.Check.SpecFnNames["g"] {
		t.Fatalf("a def outside an undecidable arm is not speculative: %v", rec.offered)
	}

	leave := r.EnterSpecArm(false)
	InstallDef(r, "f", fnA)
	if len(rec.offered) != 1 || rec.offered[0] != "f" || IsAppliableFn(rec.outers[0]) || !r.Check.SpecFnNames["f"] {
		t.Fatalf("a fresh def is offered with a zero outer and marked: %v %v %v", rec.offered, rec.outers, r.Check.SpecFnNames)
	}
	InstallDef(r, "f", fnB)
	leave()
	if len(rec.offered) != 2 || !IsAppliableFn(rec.outers[1]) || len(rec.declined) != 0 {
		t.Fatalf("an overlapping redefinition is offered with the dropped outer and declines nothing: %v %v %q", rec.offered, rec.outers, rec.declined)
	}
	if stack := r.Defs.Stack("f"); len(stack) != 1 {
		t.Fatalf("the overlap filter drops the outer and pushes the shadow: %d entries", len(stack))
	}

	// NoteSpecFnDef's placed path, direct: marked, and the set is created
	// on first use.
	r2, _ := NewRegistry()
	r2.Check.Emit = &placingRecorder{}
	if !NoteSpecFnDef(r2, "h", Value{}, fnA, SrcPos{Row: 1, Col: 1}) || !r2.Check.SpecFnNames["h"] {
		t.Fatal("a placed def marks the name")
	}
}
