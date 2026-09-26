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

// A speculative fn family BOTH arms define joins to a MODEL (NUR245): the
// then arm's fn under an undecided condition, noting no transition (both
// installs are placed at their sites), or the running arm's fn under a
// decided one, its install noted as a taken arm's — whatever the other arm
// declares. Undecided arms that differ only in a parameter or return TYPE
// join to a WIDENED model: the then arm's fn with each type joined and no
// declaration site. Arms that differ on the routed op's claim — the
// signature count, an arity, a pattern, the return count — keep the
// payload-less join, as does a non-fn arm and a name that is no
// speculative family.
func TestSpecFamilyBothArmsJoin(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true
	r.Check.SpecFnNames = map[string]bool{"f": true}
	zero, one := NewInteger(0), NewInteger(1)
	sig := func(param, ret *Type, pat *Value) FnSig {
		return FnSig{Params: []FnParam{{Name: "a", Type: param, Pattern: pat}}, Returns: []*Type{ret}, BarrierPos: 1}
	}
	fn := func(sigs ...FnSig) Value { return NewFunction(FnDefInfo{Name: "f", Signatures: sigs}) }
	join := func(then, els Value, runs string) (Value, bool) {
		t.Helper()
		base := len(r.Check.BindLedger)
		th, el := map[string]Value{"f": then}, map[string]Value{"f": els}
		switch runs {
		case "then":
			InstallDecidedJoinedDefs(r, th, el, false)
		case "else":
			InstallDecidedJoinedDefs(r, th, el, true)
		default:
			InstallJoinedDefs(r, th, el)
		}
		top, ok := r.Defs.Top("f")
		if !ok {
			t.Fatal("the join pushes a binding")
		}
		r.Defs.Pop("f")
		return top, len(r.Check.BindLedger) > base
	}
	a, b := fn(sig(TInteger, TAny, nil)), fn(sig(TInteger, TAny, nil))
	if top, noted := join(a, b, ""); top.ID != a.ID || noted {
		t.Fatalf("undecided: the then arm's fn, no transition — got %v noted=%v", top, noted)
	}
	if top, noted := join(a, b, "else"); top.ID != b.ID || !noted {
		t.Fatalf("decided else: the else arm's fn, noted — got %v noted=%v", top, noted)
	}
	if top, noted := join(a, b, "then"); top.ID != a.ID || !noted {
		t.Fatalf("decided then: the then arm's fn, noted — got %v noted=%v", top, noted)
	}
	p0, q0 := fn(sig(TInteger, TInteger, &zero)), fn(sig(TInteger, TInteger, &zero))
	if top, _ := join(p0, q0, ""); top.ID != p0.ID {
		t.Fatal("equal patterns and returns agree")
	}
	// Decided, the running arm's fn stands whatever the other arm declares.
	s8 := fn(sig(TString, TAny, nil))
	if top, noted := join(a, s8, "else"); top.ID != s8.ID || !noted {
		t.Fatalf("decided else over differing shapes: the else arm's fn, noted — got %v noted=%v", top, noted)
	}
	if top, noted := join(a, s8, "then"); top.ID != a.ID || !noted {
		t.Fatalf("decided then over differing shapes: the then arm's fn, noted — got %v noted=%v", top, noted)
	}
	// Undecided, a TYPE difference widens the then arm's fn: each parameter
	// and return type joined, no declaration site, no transition noted.
	for _, c := range []struct {
		name      string
		then, els Value
		arg, ret  *Type
	}{
		{"a parameter type", a, s8, CommonAncestorType(TInteger, TString), TAny},
		{"a return", p0, fn(sig(TInteger, TString, &zero)), TInteger, CommonAncestorType(TInteger, TString)},
		// A signature built through the legacy Args widens there.
		{"a legacy parameter type", fn(FnSig{Args: []*Type{TInteger}, Returns: []*Type{TAny}, BarrierPos: 1}),
			fn(FnSig{Args: []*Type{TString}, Returns: []*Type{TAny}, BarrierPos: 1}), CommonAncestorType(TInteger, TString), TAny},
	} {
		c.then.Data = withDecl(c.then.Data.(FnDefInfo))
		top, noted := join(c.then, c.els, "")
		fd, ok := top.Data.(FnDefInfo)
		if !ok || noted || top.ID != c.then.ID {
			t.Fatalf("%s: the widened then arm's fn, no transition — got %v noted=%v", c.name, top, noted)
		}
		s := fd.OwnSigs()[0]
		if !SigArgType(&s, 0).Equal(c.arg) || !s.Returns[0].Equal(c.ret) || s.Decl != (DeclSite{}) {
			t.Errorf("%s: want arg %v ret %v and no site, got %v %v %v", c.name, c.arg, c.ret, SigArgType(&s, 0), s.Returns[0], s.Decl)
		}
		if c.then.Data.(FnDefInfo).Signatures[0].Decl == (DeclSite{}) {
			t.Errorf("%s: the arm's own fn keeps its site", c.name)
		}
	}
	// A fallback signature rides along unwidened.
	fb := fn(sig(TInteger, TAny, nil))
	fbd := fb.Data.(FnDefInfo)
	fbd.Signatures = append(fbd.Signatures, Signature{Fallback: true})
	fb.Data = fbd
	if top, _ := join(fb, s8, ""); len(top.Data.(FnDefInfo).Signatures) != 2 || !top.Data.(FnDefInfo).Signatures[1].Fallback {
		t.Errorf("the fallback rides along: got %v", top)
	}
	for _, c := range []struct {
		name      string
		then, els Value
	}{
		{"a pattern", p0, fn(sig(TInteger, TInteger, &one))},
		{"the return count", p0, fn(FnSig{Params: []FnParam{{Name: "a", Type: TInteger, Pattern: &zero}}, Returns: []*Type{TInteger, TInteger}, BarrierPos: 1})},
		{"an arity", a, fn(FnSig{Params: []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}}, Returns: []*Type{TAny}, BarrierPos: 2})},
		{"the signature count", a, fn(sig(TInteger, TAny, nil), sig(TString, TAny, nil))},
		{"a non-fn arm", a, NewCarrier(TInteger)},
	} {
		if top, noted := join(c.then, c.els, ""); isFnModel(top) || !noted {
			t.Errorf("%s: arms that disagree keep the payload-less join, noted — got %v noted=%v", c.name, top, noted)
		}
	}
	r.Check.SpecFnNames = nil
	if top, noted := join(a, b, ""); isFnModel(top) || !noted {
		t.Errorf("no speculative family: the ordinary join — got %v noted=%v", top, noted)
	}
}

// isFnModel reports whether a join pushed an arm's own fn value — a model —
// rather than the payload-less join of the two.
// withDecl stamps each signature of fd with a declaration site, as a boru
// def's are.
func withDecl(fd FnDefInfo) FnDefInfo {
	sigs := append([]Signature(nil), fd.Signatures...)
	for i := range sigs {
		sigs[i].Decl = DeclSite{Source: "spec", Pos: SrcPos{Row: 1, Col: i + 1}}
	}
	fd.Signatures = sigs
	return fd
}

func isFnModel(v Value) bool {
	_, ok := v.Data.(FnDefInfo)
	return ok
}
