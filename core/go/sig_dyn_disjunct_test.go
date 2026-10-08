package core

import "testing"

// TestSigTypeMatchesDynamicDisjunct pins the ANY-alternative rule for a
// dynamic disjunct bound: the runtime holds exactly one alternative, so
// gradual optimism matches a slot any alternative could satisfy, and
// still declines a slot provably disjoint from every alternative — the
// optimistic dual of the strict-disjunct EVERY rule.
func TestSigTypeMatchesDynamicDisjunct(t *testing.T) {
	union := NewDynamicCarrierValue(NewDisjunct([]Value{
		NewTypeLiteral(TMap),
		NewTypeLiteral(TNone),
	}))

	// The Map alternative satisfies a Map (and Node) receiver slot.
	if !SigTypeMatches(union, TMap) {
		t.Error("dynamic(Map|None) must match a Map slot on its Map alternative")
	}
	if !SigTypeMatches(union, TNode) {
		t.Error("dynamic(Map|None) must match a Node slot on its Map alternative")
	}
	// The None alternative satisfies a None slot.
	if !SigTypeMatches(union, TNone) {
		t.Error("dynamic(Map|None) must match a None slot on its None alternative")
	}
	// No alternative reaches a provably-disjoint scalar slot.
	if SigTypeMatches(union, TInteger) {
		t.Error("dynamic(Map|None) must decline an Integer slot — every alternative is disjoint")
	}

	// A CARRIER alternative (not a bare type literal) rides the same rule.
	carrierUnion := NewDynamicCarrierValue(NewDisjunct([]Value{
		NewCarrier(TString),
		NewTypeLiteral(TNone),
	}))
	if !SigTypeMatches(carrierUnion, TString) {
		t.Error("a carrier alternative must match its own type's slot")
	}
	if SigTypeMatches(carrierUnion, TList) {
		t.Error("dynamic(String|None) must decline a List slot")
	}
}
