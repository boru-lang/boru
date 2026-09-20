package core

import "testing"

// The registry of an armed unify travels EXPLICITLY down the whole
// chain (unifyInner's r parameter), including through the kernel
// membership Behaviors whose Match / Unify re-enter the unifier. These
// pins hold the seams a package-level stack used to cover implicitly.

// A surface body reaches unifySurface through the fold table.
func TestUnifyThreadedSurfaceFold(t *testing.T) {
	_, sv, _ := x5Surface("X5ThrShp", map[string]bool{})
	got, ok := Unify(sv, sv)
	if !ok || !IsSurfaceType(got) {
		t.Fatalf("a surface unified with itself through the fold must yield the surface, got %v ok=%v", got, ok)
	}
}

// A type-parameter placeholder reached from inside an ARMED unify keeps
// the chain's registry for its bound walk: a compound bound whose
// alternative is a predicate fn admits the candidate only when the
// registry resolves the fn to its predicate type. The unarmed call
// cannot resolve it and declines — exactly the verdict split the old
// ambient stack produced for the same pair.
func TestUnifyThreadedTypeParamKeepsRegistry(t *testing.T) {
	r := x5reg(t)
	_, fn := x5PredType(t, r, "X5ThrPos")
	bound := NewDisjunct([]Value{fn, NewTypeLiteral(TString)})
	pn := MintTypeParam(r, GenParam{Name: "T", HasBound: true, Bound: bound})
	lit := NewTypeLiteral(pn)

	got, uerr := UnifyExplainR(lit, NewInteger(5), r)
	if uerr != nil {
		t.Fatalf("armed: 5 vs T extends (Pos tor String) must admit via the predicate: %v", uerr)
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("armed: expected 5 back, got %v", got)
	}
	if _, ok := Unify(lit, NewInteger(5)); ok {
		t.Fatal("unarmed: the predicate-fn alternative cannot resolve without a registry")
	}
	// The String alternative needs no registry either way.
	if _, uerr := UnifyExplainR(lit, NewString("s"), r); uerr != nil {
		t.Fatalf("armed: a String satisfies the String alternative: %v", uerr)
	}
	if _, ok := Unify(lit, NewString("s")); !ok {
		t.Fatal("unarmed: a String satisfies the String alternative")
	}
	// An unbounded placeholder admits anything, armed or not.
	un := MintTypeParam(r, GenParam{Name: "U"})
	if _, uerr := UnifyExplainR(NewTypeLiteral(un), NewInteger(1), r); uerr != nil {
		t.Fatalf("armed: unbounded placeholder admits 1: %v", uerr)
	}
}

// A binding-body node (a named typed-list whose child is a predicate
// fn) reached by dispatchUnifier from inside an ARMED unify runs its
// body unify with the chain's registry, so each element resolves the
// predicate; the unarmed call declines.
func TestUnifyThreadedBindingBodyKeepsRegistry(t *testing.T) {
	r := x5reg(t)
	_, fn := x5PredType(t, r, "X5ThrPosB")
	if err := InstallType(r, "X5ThrPL", NewTypedList(fn)); err != nil {
		t.Fatalf("InstallType(X5ThrPL): %v", err)
	}
	def := r.LookupTypeName("X5ThrPL")
	if def == nil {
		t.Fatal("LookupTypeName(X5ThrPL) = nil")
	}
	if _, ok := def.Behavior().(*BindingBodyUnifier); !ok {
		t.Fatalf("a typed-list binding carries a BindingBodyUnifier, got %T", def.Behavior())
	}
	lst := NewList([]Value{NewInteger(1), NewInteger(2)})
	got, uerr := UnifyExplainR(NewTypeLiteral(def), lst, r)
	if uerr != nil {
		t.Fatalf("armed: [1 2] vs X5ThrPL must admit via the predicate child: %v", uerr)
	}
	if l, lerr := AsList(got); lerr != nil || l.Len() != 2 {
		t.Fatalf("armed: expected the list back, got %v", got)
	}
	if _, ok := Unify(NewTypeLiteral(def), lst); ok {
		t.Fatal("unarmed: the predicate-fn child cannot resolve without a registry")
	}
	// The Match side (the `is` fast path) is unarmed by contract and
	// declines the same pair; matchR with the registry admits it.
	u := def.Behavior().(*BindingBodyUnifier)
	if u.Match(lst, def) {
		t.Fatal("Match is unarmed: the predicate-fn child cannot resolve")
	}
	if !u.matchR(lst, def, r) {
		t.Fatal("matchR with the registry resolves the predicate-fn child")
	}
}

// The fn-shape structural rule (FnSigSatisfiesSpec's Pattern-compatibility
// unify) runs from INSIDE the chain — through the ShapeFnUndef fold for an
// anonymous constraint and through FnUndefUnifier for a named fn-shape
// node — so its pattern pair is decided with the chain's registry: a
// predicate-typed list child (`[:Pos]` against `[:Pos]`) resolves the
// predicate when armed and is settled structurally when unarmed. The
// verdict must be exactly what the same pair yields at the chain's own
// level (unifyWithin), armed or not.
func TestUnifyThreadedFnShapePatternKeepsRegistry(t *testing.T) {
	r := x5reg(t)
	_, _ = x5PredType(t, r, "X5ThrPosF")
	specPat := NewTypedList(NewAtom("X5ThrPosF"))
	fnPat := NewTypedList(NewAtom("X5ThrPosF"))
	spec := FnSigSpec{Params: []FnParam{{Name: "xs", Type: TList, Pattern: &specPat}}, Returns: []*Type{TBoolean}}
	fn := fnValueWith("chk", []FnParam{{Name: "xs", Type: TList, Pattern: &fnPat}}, []*Type{TBoolean})

	// The pair's own verdicts, armed and unarmed, are the reference.
	_, armedPair := unifyWithin(specPat, fnPat, r)
	_, unarmedPair := Unify(specPat, fnPat)
	if armedPair == nil || !unarmedPair {
		t.Fatalf("reference: the predicate-child pair must decline armed (%v) and admit unarmed (%v)", armedPair, unarmedPair)
	}

	// Anonymous constraint: the ShapeFnUndef fold.
	undef := NewValueRaw(TFnUndef, FnUndefInfo{Sigs: []FnSigSpec{spec}})
	if _, uerr := UnifyExplainR(undef, fn, r); uerr == nil {
		t.Fatal("armed fold: the pattern pair declines under the registry, so the shape must decline")
	}
	if _, ok := Unify(undef, fn); !ok {
		t.Fatal("unarmed fold: the pattern pair settles structurally, so the shape admits")
	}

	// Named fn-shape node: FnUndefUnifier reached by dispatchUnifier.
	def := MintTestType("FunctionSignature/X5ThrFnU")
	installFnUndefUnifier(def, []FnSigSpec{spec}, "X5ThrFnU")
	if _, uerr := UnifyExplainR(NewTypeLiteral(def), fn, r); uerr == nil {
		t.Fatal("armed node: the pattern pair declines under the registry, so the node must decline")
	}
	if _, ok := Unify(NewTypeLiteral(def), fn); !ok {
		t.Fatal("unarmed node: the pattern pair settles structurally, so the node admits")
	}
	// The Match side: unarmed by contract, matchR with the registry.
	fu := def.Behavior().(*FnUndefUnifier)
	if !fu.Match(fn, def) {
		t.Fatal("Match is unarmed: the pattern pair settles structurally")
	}
	if fu.matchR(fn, def, r) {
		t.Fatal("matchR with the registry resolves the predicate child and declines")
	}
	// The exported entries stay unarmed.
	if !FnUndefMatchesFnDef(undef, fn) || !FnDefHasSig(fn.Data.(FnDefInfo), spec) {
		t.Fatal("the exported fn-shape checks run unarmed and admit the pair")
	}
}
