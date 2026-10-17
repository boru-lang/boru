package core

import (
	"errors"
	"testing"
)

// parentIsKinds are the builtin nodes the token-kind predicates test with
// parentIs.
var parentIsKinds = []*Type{
	TWord, TForward, TOpenParen, TCloseParen, TEnd, TParenExpr, TInterpString,
	TMark, TMove, TSplice, TDispatchMod, TReturnCheck, TDefCleanup, TReach,
	TXmlInterp, TSugar, TError, TFunction,
}

// TestParentIsAgreesWithEqual holds parentIs to Equal's answer for a value
// whose Parent is any builtin node — the node itself, a handle copy of it
// (a type literal's copy, sharing its datum), or a copy-on-write copy (a
// datum of its own, built from the node's parts) — against every kind the
// predicates test, and for the values with no builtin Parent: a minted
// type, no Parent, no datum.
func TestParentIsAgreesWithEqual(t *testing.T) {
	r := w8reg(t)
	minted := r.Types.MintType("ParentIsZ", TInteger)
	parents := []*Type{minted}
	for _, b := range Builtin.byID {
		handle := *b
		cowed := *b
		cowed.cow()
		parents = append(parents, b, &handle, &cowed)
	}
	checked := 0
	for _, p := range parents {
		v := NewValueRaw(p, nil)
		for _, k := range parentIsKinds {
			if got, want := parentIs(&v, k), v.Parent().Equal(k); got != want {
				t.Errorf("parentIs(Parent=%s, %s) = %v, Equal says %v", p.Name(), k.Name(), got, want)
			}
			checked++
		}
	}
	if checked < 100*len(parentIsKinds) {
		t.Fatalf("checked only %d pairs: the builtin table looks empty", checked)
	}
	var none Value
	noParent := ValueOf(&Datum{})
	for _, k := range parentIsKinds {
		if parentIs(&none, k) || parentIs(&noParent, k) {
			t.Errorf("a value with no datum or no Parent is no %s", k.Name())
		}
	}
	// The kind predicates built on it answer for their own kinds.
	w := NewWord("x")
	if !IsWord(&w) || IsForward(&w) || IsOpenParen(&w) {
		t.Error("a word token is a Word and nothing else")
	}
	if e := NewError(errors.New("boom")); !IsError(e) {
		t.Error("an error value is an Error")
	}
}

// TestConformsToLabelledFastPathAgrees holds ConformsTo's labelled fast path
// to the answer of the structural rules it skips — Equal against Any, then
// IsAncestor — for every pair of builtin nodes.
func TestConformsToLabelledFastPathAgrees(t *testing.T) {
	var nodes []*Type
	for _, b := range Builtin.byID {
		nodes = append(nodes, b)
	}
	for _, a := range nodes {
		for _, p := range nodes {
			want := p.Equal(TAny) || a.IsAncestor(p)
			if got := a.ConformsTo(p); got != want {
				t.Errorf("%s ConformsTo %s = %v, the structural rules say %v", a.Name(), p.Name(), got, want)
			}
		}
	}
	// Off the fast path: an unlabelled minted type keeps the walk.
	r := w8reg(t)
	minted := r.Types.MintType("ConformsZ", TInteger)
	if !minted.ConformsTo(TNumber) || !minted.ConformsTo(TAny) || minted.ConformsTo(TString) {
		t.Error("a minted Integer subtype conforms to Number and Any, not String")
	}
}
