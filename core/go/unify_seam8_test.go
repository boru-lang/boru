package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in unify.go — predicate-reference resolution guards,
// the registry-aware disjunct walk, and the bare-Node type-literal arms.
// Per design/TEST-SEAMS.10.md.

import "testing"

// w8predFn builds a value that satisfies isPredicateFnValue: parented at
// TFunction with a single-param non-fallback signature.
func w8predFn() Value {
	return NewValueRaw(TFunction, FnDefInfo{
		Signatures: []Signature{{Params: []FnParam{{Name: "x", Type: TInteger}}}},
	})
}

func TestW8IsPredicateFnValueGuards(t *testing.T) {
	// Parent is TFunction but payload is not a FnDefInfo.
	if isPredicateFnValue(NewValueRaw(TFunction, IntPayload{N: 1})) {
		t.Fatal("non-FnDefInfo payload is not a predicate fn")
	}
	// FnDefInfo with no own signature.
	if isPredicateFnValue(NewValueRaw(TFunction, FnDefInfo{})) {
		t.Fatal("FnDefInfo with no own sig is not a predicate fn")
	}
	// Positive: a single-param sig IS a predicate fn.
	if !isPredicateFnValue(w8predFn()) {
		t.Fatal("single-param FnDef must be a predicate fn value")
	}
}

func TestW8ResolvePredicateRefGuards(t *testing.T) {
	// nil registry short-circuits.
	if _, ok := resolvePredicateRef(NewWord("x"), nil); ok {
		t.Fatal("nil registry resolves no predicate")
	}
	// Word case: a name that resolves to no type returns false, but the
	// IsWord arm is exercised.
	r := w8reg(t)
	if _, ok := resolvePredicateRef(NewWord("W8UndefType"), r); ok {
		t.Fatal("undefined word names no predicate type")
	}
}

func TestW8UnifyResolvedPredicateNotPredicate(t *testing.T) {
	// A type whose Behavior is not a predicateUnifier fails cleanly.
	if _, err := unifyResolvedPredicate(TInteger, NewInteger(1), nil); err == nil {
		t.Fatal("non-predicate type must fail unifyResolvedPredicate")
	}
}

func TestW8UnifyDisjunctRAny(t *testing.T) {
	r := w8reg(t)
	disj := DisjunctInfo{Alternatives: []Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}}
	got, err := unifyDisjunctR(disj, NewTypeLiteral(TAny), r)
	if err != nil {
		t.Fatalf("Any vs a disjunct yields the disjunct: %v", err)
	}
	if !IsDisjunct(got) {
		t.Fatalf("expected a disjunct result, got %v", got)
	}
}

func TestW8UnifyInnerDisjunctWithPredicate(t *testing.T) {
	r := w8reg(t)
	// A disjunct whose first alternative is a predicate fn routes through
	// the registry-aware disjunct walk; the Integer alternative matches 5.
	disj := NewDisjunct([]Value{w8predFn(), NewTypeLiteral(TInteger)})
	got, err := UnifyExplainR(disj, NewInteger(5), r)
	if err != nil {
		t.Fatalf("disjunct-with-predicate unify: %v", err)
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("expected 5, got %v", got)
	}
}

func TestW8UnifyNodeLiteralBoth(t *testing.T) {
	got, ok := Unify(NewTypeLiteral(TNode), NewTypeLiteral(TNode))
	if !ok {
		t.Fatal("Node vs Node must unify")
	}
	if !ValueType(got).Equal(TNode) {
		t.Fatalf("expected Node, got %v", got)
	}
}

func TestW8UnifyNodeLiteralNarrower(t *testing.T) {
	// Node type literal vs a narrower node-family literal (Map) yields the
	// narrower side.
	got, ok := Unify(NewTypeLiteral(TNode), NewTypeLiteral(TMap))
	if !ok {
		t.Fatal("Node vs Map must unify")
	}
	if !ValueType(got).Equal(TMap) {
		t.Fatalf("expected Map, got %v", got)
	}
}
