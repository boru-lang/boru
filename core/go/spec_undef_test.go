package core

import "testing"

// GeneraliseSpecUndef: the model's half of a placed speculative undef. A
// VALUE binding at module scope is generalised IN PLACE — a fresh carrier
// of its type at the same depth, the generation moved, the carrier noted
// so a second undef of the same binding re-mints nothing — and every
// binding the compiled lane cannot pop-and-read live is declined with the
// model untouched: unbound, a type, a frame binding of an enclosing fn, a
// fn-family value, an active token, the fn-carrier side table's.
func TestGeneraliseSpecUndef(t *testing.T) {
	if GeneraliseSpecUndef(nil, "k") {
		t.Fatal("a nil registry generalises nothing")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if GeneraliseSpecUndef(r, "") || GeneraliseSpecUndef(r, "nope") {
		t.Fatal("an empty or unbound name generalises nothing")
	}
	if err := InstallType(r, "T", NewTypeLiteral(TInteger)); err != nil {
		t.Fatal(err)
	}
	if GeneraliseSpecUndef(r, "T") {
		t.Fatal("a type binding is declined")
	}
	// A frame binding: pushed above the innermost fn baseline.
	r.PushFnBaseline(r.Defs.Snapshot())
	r.Defs.Push("p", NewInteger(1))
	if GeneraliseSpecUndef(r, "p") {
		t.Fatal("a frame binding of an enclosing fn is declined")
	}
	r.Defs.Pop("p")
	r.PopFnBaseline()
	r.Defs.Push("f", NewCarrier(TFunction))
	if GeneraliseSpecUndef(r, "f") {
		t.Fatal("a fn-family binding is declined")
	}
	r.Defs.Push("w", NewWord("x"))
	if GeneraliseSpecUndef(r, "w") {
		t.Fatal("an active token is declined")
	}
	r.Defs.Push("c", NewInteger(3))
	NoteCheckFnCarrierBind(r, "c", NewCarrier(TFunction))
	if GeneraliseSpecUndef(r, "c") {
		t.Fatal("a name in the fn-carrier side table is declined")
	}
	ResetCheckFnCarrierBinds(r)

	r.Defs.Push("k", NewInteger(5))
	gen := r.Defs.Gen("k")
	if !GeneraliseSpecUndef(r, "k") {
		t.Fatal("a module-scope value binding generalises")
	}
	top, ok := r.Defs.Top("k")
	if !ok || IsConcrete(top) || !top.Parent.Equal(TInteger) || r.Defs.Depth("k") != 1 {
		t.Fatalf("the binding stays at its depth with a carrier of its type: %v depth=%d", top, r.Defs.Depth("k"))
	}
	if r.Defs.Gen("k") == gen || r.Check.SpecUndefGen != 1 || r.Check.SpecUndefCarriers["k"] != top.ID {
		t.Fatalf("the generation moves and the carrier is noted: gen %d→%d SpecUndefGen=%d noted=%q", gen, r.Defs.Gen("k"), r.Check.SpecUndefGen, r.Check.SpecUndefCarriers["k"])
	}
	// A second undef of the generalised binding: still placeable, nothing
	// re-minted, the generation still.
	gen = r.Defs.Gen("k")
	if !GeneraliseSpecUndef(r, "k") || r.Defs.Gen("k") != gen || r.Check.SpecUndefGen != 1 {
		t.Fatal("an already-generalised binding neither re-mints nor moves the generation")
	}
	if again, _ := r.Defs.Top("k"); again.ID != top.ID {
		t.Fatal("the same carrier stays in place")
	}
	// A NEW concrete binding of the name generalises afresh.
	r.Defs.Push("k", NewInteger(7))
	if !GeneraliseSpecUndef(r, "k") || r.Check.SpecUndefGen != 2 {
		t.Fatalf("a new binding of the name generalises again: SpecUndefGen=%d", r.Check.SpecUndefGen)
	}
	// Clone deep-copies the note; Begin's reset clears it.
	cp := r.Check.Clone()
	cp.SpecUndefCarriers["z"] = "zz"
	if _, leaked := r.Check.SpecUndefCarriers["z"]; leaked {
		t.Fatal("Clone must deep-copy SpecUndefCarriers")
	}
}

// PopLiveBinding: the run-time pop every placed or replayed undef takes —
// a missing binding is a no-op, a value binding pops, and a binding that
// MINTED its type node retires the node with it.
func TestPopLiveBinding(t *testing.T) {
	PopLiveBinding(nil, "k")
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	PopLiveBinding(r, "nope")
	r.Defs.Push("k", NewInteger(5))
	PopLiveBinding(r, "k")
	if r.Defs.Depth("k") != 0 {
		t.Fatal("a value binding pops")
	}
	if err := InstallType(r, "Pos", NewTypeLiteral(r.Types.MintRefinePrefab(TInteger))); err != nil {
		t.Fatal(err)
	}
	e, ok := r.Defs.TopEntry("Pos")
	if !ok || e.TypeDef == nil || !e.Minted {
		t.Fatalf("a refine binding mints its node: %+v", e)
	}
	PopLiveBinding(r, "Pos")
	if r.Defs.Depth("Pos") != 0 {
		t.Fatal("the type binding pops")
	}
	if r.Types.LookupByID(e.TypeDef.ID) != nil {
		t.Fatal("the minted node is retired with its binding")
	}
}
