package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A host type defined by one predicate must participate in every kernel
// membership path the boru `def`/`refine` path wires automatically:
// signature dispatch (Match, via v.Is), the Unify-driven tests (`is`,
// `case`, return checks), and canon rendering — without a hand-written
// Behavior. EvenInt admits even integers; that single rule drives all of
// it.
func TestMemberBehaviorWiring(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	even := func(v core.Value) bool {
		n, err := v.AsConcreteInteger()
		return err == nil && n%2 == 0
	}
	Even := r.Types.MintMemberType("Even", core.TInteger, even)

	// Match (v.Is) — the dispatch / fast path.
	if !addrOf(core.NewInteger(4)).Is(Even) {
		t.Error("4 should be Even")
	}
	if addrOf(core.NewInteger(3)).Is(Even) {
		t.Error("3 must not be Even")
	}
	// A wrong-family value is rejected, not mis-admitted.
	if addrOf(core.NewString("x")).Is(Even) {
		t.Error("a String must not be Even")
	}
	// A value tagged with the type (the analogue of a stream handle) is a
	// member regardless of its tag — the predicate decides.
	if !addrOf(core.MintRetagged(addrOf(core.NewInteger(8)), Even)).Is(Even) {
		t.Error("an Even-tagged 8 should be Even")
	}

	// Unify — the `is` word, `case`, fn return checks. The concrete
	// member is admitted against the bare type literal, in either order.
	lit := core.NewTypeLiteral(Even)
	if _, ok := core.UnifyR(core.NewInteger(6), lit, r); !ok {
		t.Error("Unify(6, Even) should admit the member")
	}
	if _, ok := core.UnifyR(lit, core.NewInteger(6), r); !ok {
		t.Error("Unify(Even, 6) should admit the member (reverse order)")
	}
	if _, ok := core.UnifyR(core.NewInteger(7), lit, r); ok {
		t.Error("Unify(7, Even) must reject a non-member")
	}

	// Canon — a member renders by its lattice family (an Integer), not
	// routed through the behavior's Format. The FormatDelegate marker the
	// helper supplies is what keeps this working for a minted non-builtin.
	if got := core.Canon([]core.Value{core.MintRetagged(addrOf(core.NewInteger(8)), Even)}); got != "8" {
		t.Errorf("canon of an Even-tagged 8 = %q, want %q", got, "8")
	}

	// The bare type literal is "the type itself", not an inhabitant.
	if lit.Is(Even) && !core.IsBareTypeNode(&lit) {
		t.Error("unexpected: a concrete value masquerading as the literal")
	}
}
