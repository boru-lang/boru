package core

import (
	"testing"
)

// boruPredSig builds a single boru-bodied signature for a predicate fn.
func boruPredSig(params []FnParam, returns []*Type, body []Value) Signature {
	return Signature{Params: params, Returns: returns, Impl: &BoruImpl{Body: body}}
}

func installPred(r *Registry, name string, sigs []Signature) {
	r.Defs.Push(name, NewFunction(FnDefInfo{Name: name, Signatures: sigs}))
}

// predicateImpliedType's signature-shape gates: only a single-overload,
// single-named-param, Boolean-returning boru fn with a recognisable is-T body
// qualifies. Every rejection path is pinned.
func TestPredicateImpliedTypeGates(t *testing.T) {
	if _, ok := predicateImpliedType(nil, "p"); ok {
		t.Error("nil registry must decline")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := predicateImpliedType(r, ""); ok {
		t.Error("empty name must decline")
	}
	if _, ok := predicateImpliedType(r, "nosuchfn"); ok {
		t.Error("unknown fn must decline")
	}

	xParam := []FnParam{{Name: "x", Type: TAny}}
	bodyA := []Value{NewWord("x"), NewWord("is"), NewWord("List")}

	// POSITIVE: the canonical is-List predicate.
	installPred(r, "islist", []Signature{boruPredSig(xParam, []*Type{TBoolean}, bodyA)})
	if tt, ok := predicateImpliedType(r, "islist"); !ok || !tt.Equal(TList) {
		t.Errorf("islist = %v/%v, want List/true", tt, ok)
	}

	// Two real (non-fallback) overloads → decline.
	installPred(r, "multi", []Signature{
		boruPredSig(xParam, []*Type{TBoolean}, bodyA),
		boruPredSig([]FnParam{{Name: "y", Type: TAny}}, []*Type{TBoolean},
			[]Value{NewWord("y"), NewWord("is"), NewWord("Map")}),
	})
	if _, ok := predicateImpliedType(r, "multi"); ok {
		t.Error("multi-overload must decline")
	}

	// Non-Boru implementation (a native GoImpl) → decline.
	installPred(r, "native", []Signature{{Params: xParam, Returns: []*Type{TBoolean}, Impl: &GoImpl{}}})
	if _, ok := predicateImpliedType(r, "native"); ok {
		t.Error("non-Boru impl must decline")
	}

	// Only a fallback signature (no real overload) → decline.
	installPred(r, "onlyfb", []Signature{{Fallback: true, Impl: &GoImpl{}}})
	if _, ok := predicateImpliedType(r, "onlyfb"); ok {
		t.Error("fallback-only predicate must decline")
	}

	// Two params → decline.
	installPred(r, "twop", []Signature{boruPredSig(
		[]FnParam{{Name: "x", Type: TAny}, {Name: "y", Type: TAny}}, []*Type{TBoolean}, bodyA)})
	if _, ok := predicateImpliedType(r, "twop"); ok {
		t.Error("two-param predicate must decline")
	}

	// Unnamed param → decline.
	installPred(r, "unnamed", []Signature{boruPredSig(
		[]FnParam{{Name: "", Type: TAny}}, []*Type{TBoolean}, bodyA)})
	if _, ok := predicateImpliedType(r, "unnamed"); ok {
		t.Error("unnamed-param predicate must decline")
	}

	// Non-Boolean return → decline.
	installPred(r, "intret", []Signature{boruPredSig(xParam, []*Type{TInteger}, bodyA)})
	if _, ok := predicateImpliedType(r, "intret"); ok {
		t.Error("non-Boolean-return predicate must decline")
	}
}

// predicateBodyImpliedType matches shape A (`x is T`) and shape B
// (`if (x is T) [_] [false]`), the latter both marker-expanded (6 tokens) and
// with a raw ParenExpr cond (4 tokens). Every non-matching body declines.
func TestPredicateBodyImpliedType(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := func(v ...Value) Value { return NewList(v) }

	// Shape A.
	if tt, ok := predicateBodyImpliedType(r, "x",
		[]Value{NewWord("x"), NewWord("is"), NewWord("List")}); !ok || !tt.Equal(TList) {
		t.Errorf("shape A = %v/%v, want List/true", tt, ok)
	}
	// Shape A that fails the triple (wrong var) → decline.
	if _, ok := predicateBodyImpliedType(r, "x",
		[]Value{NewWord("y"), NewWord("is"), NewWord("List")}); ok {
		t.Error("shape A wrong-var must decline")
	}

	// Shape B, 6-token, concrete `false` else-arm.
	sixFalse := []Value{NewWord("if"), NewWord("x"), NewWord("is"), NewWord("Map"),
		list(NewBoolean(true)), list(NewBoolean(false))}
	if tt, ok := predicateBodyImpliedType(r, "x", sixFalse); !ok || !tt.Equal(TMap) {
		t.Errorf("shape B (bool false) = %v/%v, want Map/true", tt, ok)
	}
	// Shape B with the `false` WORD else-arm.
	sixWordFalse := []Value{NewWord("if"), NewWord("x"), NewWord("is"), NewWord("Map"),
		list(NewBoolean(true)), list(NewWord("false"))}
	if tt, ok := predicateBodyImpliedType(r, "x", sixWordFalse); !ok || !tt.Equal(TMap) {
		t.Errorf("shape B (word false) = %v/%v, want Map/true", tt, ok)
	}
	// Shape B, 4-token raw ParenExpr cond → normalized.
	fourParen := []Value{NewWord("if"),
		NewParenExpr([]Value{NewWord("x"), NewWord("is"), NewWord("Map")}),
		list(NewBoolean(true)), list(NewBoolean(false))}
	if tt, ok := predicateBodyImpliedType(r, "x", fourParen); !ok || !tt.Equal(TMap) {
		t.Errorf("shape B (paren cond) = %v/%v, want Map/true", tt, ok)
	}

	// Shape B whose else-arm is not `false` → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []Value{NewWord("if"), NewWord("x"),
		NewWord("is"), NewWord("Map"), list(NewBoolean(true)), list(NewBoolean(true))}); ok {
		t.Error("shape B non-false else must decline")
	}
	// Shape B whose triple fails → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []Value{NewWord("if"), NewWord("y"),
		NewWord("is"), NewWord("Map"), list(NewBoolean(true)), list(NewBoolean(false))}); ok {
		t.Error("shape B bad-triple must decline")
	}
	// Shape B whose arms are not lists → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []Value{NewWord("if"), NewWord("x"),
		NewWord("is"), NewWord("Map"), NewInteger(1), NewInteger(2)}); ok {
		t.Error("shape B non-list arms must decline")
	}
	// Shape B whose else-arm is not a single element → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []Value{NewWord("if"), NewWord("x"),
		NewWord("is"), NewWord("Map"), list(NewBoolean(true)), list(NewBoolean(false), NewBoolean(false))}); ok {
		t.Error("shape B multi-element else must decline")
	}
	// 4-token, `if`, but cond is NOT a ParenExpr → falls through (not 6) → decline.
	if _, ok := predicateBodyImpliedType(r, "x",
		[]Value{NewWord("if"), NewWord("x"), list(NewBoolean(true)), list(NewBoolean(false))}); ok {
		t.Error("shape B non-paren 4-token must decline")
	}
	// Unrecognised length → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []Value{NewWord("x"), NewWord("is")}); ok {
		t.Error("2-token body must decline")
	}
}

// guardTripleType resolves `param is T` to T, through the def table, the kernel
// name table, and the external-builtin path resolver; every failure path is
// pinned.
func TestGuardTripleType(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	w := NewWord
	// w0 not the param.
	if _, ok := guardTripleType(r, "x", w("y"), w("is"), w("List")); ok {
		t.Error("w0-mismatch must decline")
	}
	// w1 not `is`.
	if _, ok := guardTripleType(r, "x", w("x"), w("eq"), w("List")); ok {
		t.Error("w1-not-is must decline")
	}
	// Kernel name-table resolution.
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w("List")); !ok || !tt.Equal(TList) {
		t.Errorf("builtin List = %v/%v", tt, ok)
	}
	// External-builtin path resolution (typeNames misses, ResolveTypePath hits).
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w(TMap.Path())); !ok || !tt.Equal(TMap) {
		t.Errorf("path %q = %v/%v, want Map/true", TMap.Path(), tt, ok)
	}
	// Def-table alias resolution.
	r.Defs.PushType("Foo", TList, NewTypeLiteral(TList))
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w("Foo")); !ok || !tt.Equal(TList) {
		t.Errorf("def alias Foo = %v/%v, want List/true", tt, ok)
	}
	// Unresolvable type name.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), w("Zzznotatype")); ok {
		t.Error("unresolvable type must decline")
	}
	// None is excluded.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), w("None")); ok {
		t.Error("None must decline")
	}
	// tv already a bare type literal (not a Word).
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), NewTypeLiteral(TMap)); !ok || !tt.Equal(TMap) {
		t.Errorf("literal tv = %v/%v, want Map/true", tt, ok)
	}
	// tv a concrete non-type value (not a bare node) → decline.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), NewInteger(5)); ok {
		t.Error("concrete non-type tv must decline")
	}
	// tv a malformed Word (Parent=TWord, no WordInfo payload): IsWord passes,
	// AsWord fails → the defensive decline.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), Value{d: &Datum{Parent: TWord}}); ok {
		t.Error("malformed-word tv must decline")
	}
}

// isWordNamed, stripGuardMarkers, defsHasType — the small predicates.
func TestGuardPredicateHelpers(t *testing.T) {
	// isWordNamed.
	if isWordNamed(NewInteger(5), "x") {
		t.Error("a non-Word is not word-named")
	}
	if isWordNamed(Value{d: &Datum{Parent: TWord}}, "x") {
		t.Error("a non-concrete Word is not word-named")
	}
	if isWordNamed(NewWord("foo"), "bar") {
		t.Error("wrong name must be false")
	}
	if !isWordNamed(NewWord("is"), "is") {
		t.Error("matching name must be true")
	}

	// stripGuardMarkers drops paren/end markers, keeps operands.
	got := stripGuardMarkers([]Value{NewOpenParen(), NewWord("x"), NewWord("is"),
		NewWord("Map"), NewCloseParen(), NewEnd()})
	if len(got) != 3 {
		t.Fatalf("stripGuardMarkers kept %d tokens, want 3", len(got))
	}
	// A marker-free slice is returned intact.
	if s := stripGuardMarkers([]Value{NewWord("x")}); len(s) != 1 {
		t.Errorf("marker-free strip len = %d, want 1", len(s))
	}

	// defsHasType.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if defsHasType(r, "Nope") {
		t.Error("absent name must be false")
	}
	r.Defs.PushType("Bar", TMap, NewTypeLiteral(TMap))
	if !defsHasType(r, "Bar") {
		t.Error("bound type name must be true")
	}
}
