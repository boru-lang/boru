package core

import "testing"

// forwardPatternRejects gates resolveForwardArgs' viable-set pruning on
// a sig position's Pattern, mirroring patternsOk's forward-position
// rules. The integration-level consequence (fn's tnot-List triple sig
// releasing its 3-token window on a spec-list call so a following paren
// group is NOT pre-evaluated) is pinned by lang/spec/fn-triple.tsv §5
// and the pre-existing modifiers.tsv/ref.tsv `(name/v)` rows; these
// tests pin the helper's branch table.

func TestForwardPatternRejectsBranches(t *testing.T) {
	notList := NewNegation(NewTypeLiteral(TList))

	// Load-bearing coupling: patternsOk (match.go) enforces a pattern on
	// FORWARD positions only when IsConcrete(pattern) — a negation
	// carries a NegationInfo payload and must stay concrete, or fn's
	// tnot-List input constraint silently stops gating forward dispatch.
	if !IsConcrete(&notList) {
		t.Fatal("a negation pattern must be IsConcrete (forward-position enforcement depends on it)")
	}

	noPattern := Signature{Args: []*Type{TAny}}
	if forwardPatternRejects(&noPattern, 0, NewList([]Value{NewInteger(1)})) {
		t.Fatal("no pattern declared: must not reject")
	}

	negSig := Signature{Args: []*Type{TAny}, Patterns: map[int]Value{0: notList}}
	if !forwardPatternRejects(&negSig, 0, NewList([]Value{NewInteger(1)})) {
		t.Fatal("tnot List vs a concrete list: must reject")
	}
	if forwardPatternRejects(&negSig, 0, NewInteger(7)) {
		t.Fatal("tnot List vs an integer: must not reject")
	}
	if forwardPatternRejects(&negSig, 0, NewTypeLiteral(TInteger)) {
		t.Fatal("tnot List vs the Integer type literal: must not reject (abstract operand admitted)")
	}

	// Structural map patterns are stack-only (patternsOk skips them on
	// forward positions) — a mismatching concrete map must NOT prune.
	kindMap := NewOrderedMap()
	kindMap.Set("kind", NewString("api"))
	mapPat := Signature{Args: []*Type{TMap}, Patterns: map[int]Value{0: NewMap(kindMap)}}
	otherMap := NewOrderedMap()
	otherMap.Set("kind", NewString("db"))
	if forwardPatternRejects(&mapPat, 0, NewMap(otherMap)) {
		t.Fatal("structural map pattern vs map value: stack-only enforcement, must not reject")
	}

	// A non-concrete pattern (bare type literal) is not forward-enforced.
	litPat := Signature{Args: []*Type{TAny}, Patterns: map[int]Value{0: NewTypeLiteral(TString)}}
	if forwardPatternRejects(&litPat, 0, NewInteger(7)) {
		t.Fatal("non-concrete pattern: must not reject on forward positions")
	}

	// A concrete scalar pattern rejects a definite mismatch (the §1.1
	// literal-pattern rule, e.g. `def fact[0]`).
	zeroPat := Signature{Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(0)}}
	if !forwardPatternRejects(&zeroPat, 0, NewInteger(3)) {
		t.Fatal("literal 0 pattern vs 3: must reject")
	}
	if forwardPatternRejects(&zeroPat, 0, NewInteger(0)) {
		t.Fatal("literal 0 pattern vs 0: must not reject")
	}
}

// A CARRIER pattern is a check-mode placeholder for a computed pattern
// value (`fn (2 add 3) [String] ['five']` constructing during the check
// pass) — its real value is unknown statically, so pattern enforcement
// is skipped at match time. Pins the stack-matcher (MatchSignature)
// side; the engine matcher's patternsOk skip is pinned by
// lang/go/test/fn_triple_compiled_test.go.
func TestCarrierPatternNotEnforced(t *testing.T) {
	carrier := NewCarrier(TInteger)
	sigs := []Signature{{
		Args:     []*Type{TInteger},
		Patterns: map[int]Value{0: carrier},
	}}
	res := MatchSignature(sigs, []Value{NewInteger(9)}, WordInfo{ArgCount: -1})
	if res == nil {
		t.Fatal("a carrier pattern must not be enforced: 9 must match the Integer slot")
	}
	// The concrete-pattern contract is unchanged: a literal pattern
	// still rejects a mismatching value on the same path.
	strict := []Signature{{
		Args:     []*Type{TInteger},
		Patterns: map[int]Value{0: NewInteger(0)},
	}}
	if MatchSignature(strict, []Value{NewInteger(9)}, WordInfo{ArgCount: -1}) != nil {
		t.Fatal("a concrete literal pattern must still reject a mismatch")
	}
}
