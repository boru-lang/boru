package core

import "testing"

// NoteBindTransition is the bind ledger's ONLY writer, and every arm in it
// decides something a twin depends on: whether a transition is replayed at
// all, and where the twin op gets placed. core/go's own suite reaches the
// function only through installDef with check mode off — it always returns
// at the first guard — so the deciding arms are driven here directly.

// noteLedger drives one call against a fresh registry and returns the ledger.
func noteLedger(t *testing.T, setup func(*Registry), kind BindKind, name string, pos SrcPos) []BindTransition {
	t.Helper()
	r := newTestRegistry(t)
	r.Check.Mode = true
	setup(r)
	r.NoteBindTransition(kind, name, pos)
	return r.Check.BindLedger
}

// The three suppressions. Each is a case the twins must NOT see: a pass that
// is not checking has no ledger to build, a nameless synthetic install has
// nothing for a twin to address, and a def inside a fn body is a frame-local
// the compiled lane gives a slot rather than a registry binding.
func TestNoteBindTransitionSuppressions(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Registry)
		word  string
	}{
		{"check mode off", func(r *Registry) { r.Check.Mode = false }, "x"},
		{"empty name", func(*Registry) {}, ""},
		{"inside a fn body", func(r *Registry) { r.Check.FnBodyDepth = 1 }, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := noteLedger(t, tc.setup, BindDef, tc.word, SrcPos{Row: 3, Col: 5}); len(got) != 0 {
				t.Errorf("recorded %d transition(s) for %s — the twins would replay one that has no runtime binding", len(got), tc.name)
			}
		})
	}
}

// A nil registry and a registry with no CheckState are the two shapes a
// native can be called with outside a pass; neither may panic.
func TestNoteBindTransitionWithoutCheckState(t *testing.T) {
	var nilReg *Registry
	nilReg.NoteBindTransition(BindDef, "x", SrcPos{})
	r := newTestRegistry(t)
	r.Check = nil
	r.NoteBindTransition(BindDef, "x", SrcPos{})
	// The type funnel is as total as the note it wraps: with no registry
	// there is nothing to record on, and with no CheckState the recorder
	// accessor hands back the inactive no-op rather than needing a guard.
	nilReg.NoteTypeInstall("Tq", SrcPos{})
	r.NoteTypeInstall("Tq", SrcPos{})
	if got := bindSitePos(nil, SrcPos{Row: 3, Col: 4}); got != (SrcPos{Row: 3, Col: 4}) {
		t.Fatalf("bindSitePos with no registry = %v, want the position it was given", got)
	}
	if got := bindSitePos(r, SrcPos{Row: 3, Col: 4}); got != (SrcPos{Row: 3, Col: 4}) {
		t.Fatalf("bindSitePos with no CheckState = %v, want the position it was given", got)
	}
}

// POSITION, the three-way rule. PendingBindPos (the def SITE, staged by the
// def word's own handler) wins outright; the value's own position is used
// when nothing is staged; and CurWordPos is the floor for the values that
// carry no position at all — an `undef`, a word extension, a fn body.
func TestNoteBindTransitionPositionPrecedence(t *testing.T) {
	site := SrcPos{Row: 1, Col: 5}
	value := SrcPos{Row: 1, Col: 7}
	word := SrcPos{Row: 4, Col: 2}

	cases := []struct {
		name  string
		setup func(*Registry)
		pos   SrcPos
		want  SrcPos
	}{
		{"staged site wins over the value token", func(r *Registry) {
			r.Check.PendingBindPos = site
			r.Check.CurWordPos = word
		}, value, site},
		{"the value's own position when nothing is staged", func(r *Registry) {
			r.Check.CurWordPos = word
		}, value, value},
		{"the dispatching word when the value has none", func(r *Registry) {
			r.Check.CurWordPos = word
		}, SrcPos{}, word},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := noteLedger(t, tc.setup, BindDef, "x", tc.pos)
			if len(got) != 1 {
				t.Fatalf("recorded %d transitions, want 1", len(got))
			}
			if got[0].Pos != tc.want {
				t.Errorf("entry positioned at %d:%d, want %d:%d — a twin emitted here lands at the wrong token",
					got[0].Pos.Row, got[0].Pos.Col, tc.want.Row, tc.want.Col)
			}
		})
	}
}

// The recorded DEPTH is read from the def table at note time, so a shadowing
// install records the depth it created rather than the one it hid. That is
// what makes the ledger's depths compose into the interpreter's tape.
func TestNoteBindTransitionRecordsKindNameAndDepth(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewInteger(1))
	r.NoteBindTransition(BindDef, "x", SrcPos{Row: 1, Col: 5})
	r.Defs.Push("x", NewInteger(2))
	r.NoteBindTransition(BindDefReplace, "x", SrcPos{Row: 2, Col: 5})
	r.Defs.Pop("x")
	r.NoteBindTransition(BindUndef, "x", SrcPos{Row: 3, Col: 5})

	got := r.Check.BindLedger
	if len(got) != 3 {
		t.Fatalf("recorded %d transitions, want 3", len(got))
	}
	want := []BindTransition{
		{Kind: BindDef, Name: "x", Pos: SrcPos{Row: 1, Col: 5}, Depth: 1},
		{Kind: BindDefReplace, Name: "x", Pos: SrcPos{Row: 2, Col: 5}, Depth: 2},
		{Kind: BindUndef, Name: "x", Pos: SrcPos{Row: 3, Col: 5}, Depth: 1},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// Clone must deep-copy the ledger: a speculative pass that clones the state,
// records, and is discarded must not leave its transitions in the original.
func TestCheckStateCloneDeepCopiesBindLedger(t *testing.T) {
	c := &CheckState{BindLedger: []BindTransition{{Kind: BindDef, Name: "x"}}}
	clone := c.Clone()
	clone.BindLedger[0].Name = "mutated"
	clone.BindLedger = append(clone.BindLedger, BindTransition{Kind: BindUndef, Name: "y"})

	if len(c.BindLedger) != 1 || c.BindLedger[0].Name != "x" {
		t.Errorf("the clone's writes bled into the original: %+v", c.BindLedger)
	}
	if empty := (&CheckState{}).Clone(); empty.BindLedger != nil {
		t.Errorf("an empty ledger cloned as %v, want nil", empty.BindLedger)
	}
}

// A signature-specific undef's ledger contract, split out of BindDefReplace
// after a live probe showed the conflation misstating depth by one: each
// REMOVAL notes BindSigUndef (delta -1) with the post-removal depth and the
// REMOVED entry as its capture, and a sig-undef whose match is locked (or
// absent) notes NOTHING — a no-op is not a transition.
func TestSigUndefLedgerSemantics(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.Mode = true
	sig := func(pt, rt *Type, pn string) Signature {
		return Signature{
			Params:     []FnParam{{Name: pn, Type: pt}},
			Returns:    []*Type{rt},
			Impl:       Boru([]Value{NewWord(pn)}),
			BarrierPos: BarrierAllForward,
		}
	}
	InstallFnDef(r, "suq", FnDefInfo{Signatures: []Signature{sig(TInteger, TInteger, "n")}})
	InstallFnDef(r, "suq", FnDefInfo{Signatures: []Signature{sig(TString, TString, "s")}})
	base := len(r.Check.BindLedger)

	UninstallFnSigs(r, "suq", FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Type: TString}},
		Returns: []*Type{TString},
	}}})
	led := r.Check.BindLedger[base:]
	if len(led) != 1 || led[0].Kind != BindSigUndef || led[0].Depth != 1 {
		t.Fatalf("removal must note one BindSigUndef at the post-removal depth 1; got %+v", led)
	}
	if r.Defs.Depth("suq") != 1 {
		t.Fatalf("live depth = %d, want 1", r.Defs.Depth("suq"))
	}

	// A spec matching nothing notes nothing.
	UninstallFnSigs(r, "suq", FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Type: TBoolean}},
		Returns: []*Type{TBoolean},
	}}})
	if len(r.Check.BindLedger) != base+1 {
		t.Fatalf("a no-op sig-undef must not note; ledger grew to %d entries past base", len(r.Check.BindLedger)-base)
	}
}
