package core

import "testing"

// W9 signature.go coverage: normalizeSig's boru /q merge, MatchSignature's
// pattern-unify decline, sigTypeMatchesAsType's bare-None reject,
// positionalMatch's /q-non-atom and bare-type-for-container declines, and
// CompareSignatures' locked-first and barrier tie-breaks.

func TestW9NormalizeSigQuoteParam(t *testing.T) {
	s := &Signature{Params: []FnParam{{Name: "a", Type: TInteger, Quote: true}}}
	NormalizeSig(s)
	if s.QuoteArgs == nil || !s.QuoteArgs[0] {
		t.Error("a /q param should populate QuoteArgs[0]")
	}
}

func TestW9MatchSignaturePatternDecline(t *testing.T) {
	pat := NewInteger(5)
	sig := Signature{Params: []FnParam{{Name: "a", Type: TInteger, Pattern: &pat}}}
	NormalizeSig(&sig)
	// Stack value 3 does not unify with the pattern 5 → no match.
	if res := MatchSignature([]Signature{sig}, []Value{NewInteger(3)}, WordInfo{ArgCount: -1}); res != nil {
		t.Error("a value that fails the pattern must not match")
	}
	// The matching value 5 does match.
	if res := MatchSignature([]Signature{sig}, []Value{NewInteger(5)}, WordInfo{ArgCount: -1}); res == nil {
		t.Error("the pattern value should match")
	}
}

func TestW9SigTypeMatchesAsTypeBareNone(t *testing.T) {
	// A bare None lattice node (Parent=TNone, Data==nil, no Name) is not a
	// valid type-arg literal.
	bareNone := Value{Parent: TNone}
	if !IsBareTypeNode(&bareNone) {
		t.Fatal("test premise: a payload-less TNone value is a bare type node")
	}
	if sigTypeMatchesAsType(bareNone, TInteger) {
		t.Error("bare None must not match a type-arg slot")
	}
}

func TestW9PositionalMatchDeclines(t *testing.T) {
	// /q modifier: a Word value against a non-atom-compatible slot declines.
	qSig := &Signature{Args: []*Type{TInteger}, QuoteArgs: map[int]bool{0: true}}
	if positionalMatch([]Value{NewWord("w")}, qSig) {
		t.Error("a /q word against an Integer slot must decline")
	}

	// A bare type literal against a concrete List slot declines.
	listSig := &Signature{Args: []*Type{TList}}
	if positionalMatch([]Value{NewTypeLiteral(TList)}, listSig) {
		t.Error("a bare List literal against a concrete List slot must decline")
	}
}

func TestW9CompareSignaturesTieBreaks(t *testing.T) {
	// Locked is NOT an ordering key (design/OPEN-WORDS.1.md §2 R3):
	// two otherwise-identical sigs differing only in Locked tie.
	if got := CompareSignatures(&Signature{Locked: true}, &Signature{Locked: false}); got != 0 {
		t.Errorf("locked as ordering key = %d, want 0 (no tier)", got)
	}
	// Equal locked + equal arity + equal slots: the one WITHOUT an
	// intermediate barrier sorts after the one WITH one.
	a := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 0}
	b := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1}
	if got := CompareSignatures(a, b); got != 1 {
		t.Errorf("no-barrier vs barrier = %d, want 1", got)
	}
}
