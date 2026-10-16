package core

import (
	"math"
	"testing"
)

// The no-copy phase 4 representation (design/VALUE-NO-COPY.0.md §6, "Phase 4
// as built"): a Value is a holder handle over shared, immutable content.
// These pin the content's accessors and copy-on-write setters, the interned
// contents, the marker cells and the derived-content memo on core's side.

// TestP4ZeroHolderHasNoContent: the zero Value is a holder with no content,
// and every accessor answers the zero value for it.
func TestP4ZeroHolderHasNoContent(t *testing.T) {
	var v Value
	if DataOf(&v) != nil || ParentOf(&v) != nil || IsCarrier(&v) || OriginOf(&v) != OriginBuiltin || IsInternal(&v) {
		t.Fatalf("the zero holder reads as no content: %v %v %v %v %v", DataOf(&v), ParentOf(&v), IsCarrier(&v), OriginOf(&v), IsInternal(&v))
	}
	if tmetaOf(&v) != nil || elemOf(&v) != nil || modnsOf(&v) != nil {
		t.Fatal("the zero holder has no type metadata, element constraint or namespace facet")
	}
}

// TestP4SettersWriteOnlyTheirHolder: a copy of a holder shares its content;
// a setter gives the holder it is called on content of its own, so the
// other holder keeps what it had.
func TestP4SettersWriteOnlyTheirHolder(t *testing.T) {
	orig := MakeValue(TInteger, IntPayload{N: 5000})
	cp := orig
	if cp.c != orig.c {
		t.Fatal("a copy of a holder shares the content")
	}
	SetData(&cp, IntPayload{N: 6000})
	SetParent(&cp, TFloat)
	SetCarrier(&cp, true)
	SetOrigin(&cp, OriginUserDef)
	SetInternal(&cp, true)
	setTMeta(&cp, &typeMeta{Name: "p4"})
	elem := NewInteger(1)
	setElemField(&cp, &elem)
	setModNSField(&cp, &ModuleNSInfo{Name: "P4"})
	if n, _ := AsInteger(orig); n != 5000 || ParentOf(&orig) != TInteger || IsCarrier(&orig) || OriginOf(&orig) != OriginBuiltin || IsInternal(&orig) || tmetaOf(&orig) != nil || elemOf(&orig) != nil || modnsOf(&orig) != nil {
		t.Fatalf("the first holder kept its content: %v", orig)
	}
	if DataOf(&cp) != (IntPayload{N: 6000}) || ParentOf(&cp) != TFloat || !IsCarrier(&cp) || OriginOf(&cp) != OriginUserDef || !IsInternal(&cp) {
		t.Fatalf("the second holder took every write: %v", cp)
	}
	if tmetaOf(&cp).Name != "p4" || elemOf(&cp) != &elem || modnsOf(&cp).Name != "P4" {
		t.Fatal("the second holder took the facet writes")
	}
	// A setter on a holder with no content builds it.
	var z Value
	SetParent(&z, TString)
	if ParentOf(&z) != TString || DataOf(&z) != nil {
		t.Fatalf("a setter on the zero holder builds content: %v", z)
	}
	if c := MakeCarrierValue(TList, nil); !IsCarrier(&c) || ParentOf(&c) != TList {
		t.Fatal("MakeCarrierValue mints a carrier")
	}
}

// TestP4InternedScalarsShareContent: the small integers, the booleans, none,
// the empty string and the structural markers mint without content of their
// own, with an identity only while a pass mints them, and a write to one
// holder of them reaches no other.
func TestP4InternedScalarsShareContent(t *testing.T) {
	pairs := [][2]Value{
		{NewInteger(internIntMin), NewInteger(internIntMin)},
		{NewInteger(internIntMax), NewInteger(internIntMax)},
		{NewBoolean(true), NewBoolean(true)},
		{NewBoolean(false), NewBoolean(false)},
		{NewNone(), NewNone()},
		{NewString(""), NewString("")},
		{NewOpenParen(), NewOpenParen()},
		{NewCloseParen(), NewCloseParen()},
		{NewEnd(), NewEnd()},
	}
	for _, p := range pairs {
		if p[0].c != p[1].c {
			t.Errorf("%v: two mints share one content", p[0])
		}
		if (IDOf(&p[0]) != "") != CheckPassActive() {
			t.Errorf("%v: an identity exactly when a pass is minting them", p[0])
		}
	}
	if a, b := NewInteger(internIntMax+1), NewInteger(internIntMax+1); a.c == b.c {
		t.Error("an integer outside the interned range has content of its own")
	}
	if n, _ := AsInteger(NewInteger(-7)); n != -7 {
		t.Errorf("an interned integer reads its value: %d", n)
	}
	if b, _ := AsBoolean(NewBoolean(false)); b {
		t.Error("false reads false")
	}
	if s, _ := AsString(NewString("")); s != "" || !ParentOf(addrOf(NewString(""))).Equal(TStringEmpty) {
		t.Error("the empty string is EmptyString")
	}
	one := NewInteger(1)
	SetData(&one, IntPayload{N: 2})
	if n, _ := AsInteger(NewInteger(1)); n != 1 {
		t.Fatal("a write to one holder of an interned integer reaches no other")
	}
	// Inside a pass an interned mint takes an identity of its own; the
	// content stays shared.
	end := BeginIDMintScope()
	x, y := NewInteger(3), NewInteger(3)
	end()
	if IDOf(&x) == "" || IDOf(&x) == IDOf(&y) || x.c != y.c {
		t.Errorf("a pass mints identities over the shared content: %q %q", IDOf(&x), IDOf(&y))
	}
}

// TestP4MarkerCells: a forward, a mark and a move hold their payloads in the
// cell with their content, read back through the accessors; an advance
// repoints the marker's slot at a new cell and leaves the old one as it was.
func TestP4MarkerCells(t *testing.T) {
	fwd := NewForward(ForwardInfo{FuncName: "f", ExpectedArgs: 2})
	snapshot := fwd
	info, err := AsForward(&fwd)
	if err != nil || info.FuncName != "f" {
		t.Fatalf("AsForward reads the cell: %v %v", info, err)
	}
	info.CollectedArgs = 1
	advanceForward(&fwd, info)
	if got, _ := AsForward(&fwd); got.CollectedArgs != 1 || !IsForward(&fwd) {
		t.Fatalf("the slot took the advance: %+v", got)
	}
	if got, _ := AsForward(&snapshot); got.CollectedArgs != 0 {
		t.Fatal("a copy taken before the advance keeps its state")
	}
	if _, err := AsForward(addrOf(NewInteger(1))); err == nil {
		t.Error("AsForward of a non-forward is an error")
	}
	mark := NewMark("m1", NewInteger(1))
	if m, err := AsMark(&mark); err != nil || m.ID != "m1" || len(m.Body) != 1 {
		t.Fatalf("AsMark reads the cell: %+v %v", m, err)
	}
	if _, err := AsMark(addrOf(NewInteger(1))); err == nil {
		t.Error("AsMark of a non-mark is an error")
	}
	move := NewMove("m1", "test")
	if m, err := AsMove(&move); err != nil || m.To != "m1" || m.Reason != "test" {
		t.Fatalf("AsMove reads the cell: %+v %v", m, err)
	}
	if _, err := AsMove(addrOf(NewInteger(1))); err == nil {
		t.Error("AsMove of a non-move is an error")
	}
}

// TestP4DerivedContents: a word's force-stack form and its atom are built
// once per word content and shared from then on; a holder that does not
// hold the word it is asked about gets content of its own.
func TestP4DerivedContents(t *testing.T) {
	w := NewWord("add")
	wi, _ := AsWord(&w)
	fs1, fs2 := forceStackContent(&w, wi), forceStackContent(&w, wi)
	if fs1 != fs2 {
		t.Fatal("the force-stack form is built once per word content")
	}
	if got := fs1.data.(WordInfo); got != (WordInfo{Name: "add", ArgCount: -1, ForceStack: true}) {
		t.Fatalf("the force-stack form: %+v", got)
	}
	a1, a2 := atomContent(&w, wi), atomContent(&w, wi)
	if a1 != a2 || a1.data != (AtomPayload{Name: "add"}) || a1.parent != TAtom {
		t.Fatal("the atom is built once per word content")
	}
	// A holder of another word, and a holder with no content, mint their own.
	other := WordInfo{Name: "sub", ArgCount: -1}
	if c := forceStackContent(&w, other); c == fs1 || c.data.(WordInfo).Name != "sub" {
		t.Error("another word's force-stack form is its own")
	}
	if c := atomContent(&w, other); c == a1 || c.data != (AtomPayload{Name: "sub"}) {
		t.Error("another word's atom is its own")
	}
	var empty Value
	if c := forceStackContent(&empty, wi); c == fs1 || !c.data.(WordInfo).ForceStack {
		t.Error("a holder with no content gets a force-stack form of its own")
	}
	if c := atomContent(&empty, wi); c == a1 || c.data != (AtomPayload{Name: "add"}) {
		t.Error("a holder with no content gets an atom of its own")
	}
	// A copy of the content (a setter's) carries no memo.
	cp := w
	SetOrigin(&cp, OriginUserDef)
	if cp.c.derived.Load() != nil {
		t.Error("new content starts with no memo")
	}
}

// TestP4MirroredLabels: every labelled builtin node mirrors its interval
// labels into its content, the labels stay within the mirror's range, and
// identity and ancestry answer the same through the mirror as through the
// metadata (a holder whose content carries the node's metadata but no
// mirror takes the metadata path).
func TestP4MirroredLabels(t *testing.T) {
	labelled := 0
	for _, n := range Builtin.byID {
		tm := tmetaOf(n)
		if tm == nil || tm.In == 0 {
			continue
		}
		labelled++
		if int(n.c.in) != tm.In || int(n.c.out) != tm.Out {
			t.Fatalf("%s: mirrored labels %d/%d, metadata %d/%d", n.Path(), n.c.in, n.c.out, tm.In, tm.Out)
		}
		if tm.Out > math.MaxUint16 {
			t.Fatalf("%s: label %d beyond the mirror's range", n.Path(), tm.Out)
		}
	}
	if labelled == 0 {
		t.Fatal("the builtin table is labelled")
	}
	bare := func(n *Type) *Type { return &Type{c: &content{parent: ParentOf(n), tmeta: tmetaOf(n)}} }
	for _, pair := range [][2]*Type{{TInteger, TNumber}, {TInteger, TString}, {TNumber, TInteger}, {TInteger, TInteger}} {
		a, b := pair[0], pair[1]
		if a.Equal(b) != bare(a).Equal(b) || a.Equal(b) != a.Equal(bare(b)) {
			t.Errorf("%s = %s: the mirror and the metadata disagree", a.Path(), b.Path())
		}
		if a.IsAncestor(b) != bare(a).IsAncestor(b) || a.IsAncestor(b) != a.IsAncestor(bare(b)) {
			t.Errorf("%s under %s: the mirror and the metadata disagree", a.Path(), b.Path())
		}
	}
	// New metadata drops the mirror: the holder answers from what it now has.
	cp := *TInteger
	setTMeta(&cp, &typeMeta{Name: "fresh"})
	if cp.c.in != 0 || cp.c.out != 0 || cp.Equal(TInteger) {
		t.Fatal("a node copy given new metadata keeps no mirrored labels")
	}
}
