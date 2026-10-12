package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A gradual (dynamic) carrier matches a slot unless its bound is PROVABLY
// disjoint from the slot. The `tand` disjointness probe wrongly reports two
// container-family carriers as disjoint when one conforms to the other
// (tand(List, Node) = Never even though List ⊑ Node), so sigTypeMatches now
// checks conformance in both directions first. Without it, a value pulled from
// a fn whose declared return narrowed to a dynamic List/Map carrier failed to
// match a Node-typed `get`/`set` receiver — the trie client's node-walker
// cascade (`(nd kid-items)` / `build-row`-result → `get`).
func TestDynamicCarrierMatchesNodeFamily(t *testing.T) {
	dyn := func(b *core.Type) core.Value { v := core.NewCarrier(b); v.Dynamic = true; return v }

	// Positive: a dynamic container bound matches the broader Node slot
	// (X ⊑ t), and a dynamic Node matches a narrower List/Map slot (t ⊑ X,
	// gradual optimism), and Any matches everything.
	match := []struct {
		name string
		v    core.Value
		slot *core.Type
	}{
		{"dynamic(List) vs Node", dyn(core.TList), core.TNode},
		{"dynamic(Map) vs Node", dyn(core.TMap), core.TNode},
		{"dynamic(Node) vs List", dyn(core.TNode), core.TList},
		{"dynamic(Node) vs Map", dyn(core.TNode), core.TMap},
		{"dynamic(List) vs List", dyn(core.TList), core.TList},
		{"dynamic(Integer) vs Number", dyn(core.TInteger), core.TNumber},
		{"dynamic(Number) vs Integer", dyn(core.TNumber), core.TInteger},
		{"dynamic(Any) vs Node", dyn(core.TAny), core.TNode},
	}
	for _, c := range match {
		if !core.SigTypeMatches(&c.v, c.slot) {
			t.Errorf("%s: expected match, got false", c.name)
		}
	}

	// Negative: provably-disjoint bounds still do NOT match — the gradual
	// looseness must not erase real type distinctions.
	noMatch := []struct {
		name string
		v    core.Value
		slot *core.Type
	}{
		{"dynamic(Integer) vs String", dyn(core.TInteger), core.TString},
		{"dynamic(String) vs Integer", dyn(core.TString), core.TInteger},
		{"dynamic(List) vs String", dyn(core.TList), core.TString},
	}
	for _, c := range noMatch {
		if core.SigTypeMatches(&c.v, c.slot) {
			t.Errorf("%s: expected NO match (provably disjoint), got true", c.name)
		}
	}
}
