package check

// Gate coverage for carrierStacksEqual's arity guard (carrier.go:3902) —
// the arm that keeps refineRecursiveSummary's bounded Kleene loop running
// when a refinement round widens the return ARITY rather than a type.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestGatecarrier3902RefineWidensArity drives the length-mismatch arm through
// its only production caller, refineRecursiveSummary.
//
// The seeded hypothesis is a 1-wide stack; round 1 re-analyses the body and
// returns a 2-wide stack (the recursive fn turned out to yield a second value
// once the in-flight Any bail was replaced by the seed). core.JoinCarrierStacks
// pads to max(len(a), len(b)), so `joined` is 2 wide against a 1-wide
// `result` — carrierStacksEqual's `len(a) != len(b)` guard fires, reports
// "not stable", and the loop takes another round with the widened stack.
//
// The arity guard is the only thing that can keep this loop alive: two
// same-length carrier stacks always compare equal (valuesEqualDefault treats
// two Data==nil values as conservatively equal when either is a Carrier), so
// without this arm refineRecursiveSummary would break on round 1 and hand
// back the too-narrow 1-wide summary.
func TestGatecarrier3902RefineWidensArity(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.FnSummaries = map[string][]core.Value{}

	const key = "gate3902#recur"
	seed := []core.Value{core.NewCarrier(core.TInteger)}

	rounds := 0
	runOnce := func() []core.Value {
		rounds++
		if r.Check.FnSummaries[key] == nil {
			t.Fatalf("round %d: hypothesis must be seeded before re-analysis", rounds)
		}
		// Every round reports a 2-wide stack. Round 1 therefore widens the
		// 1-wide hypothesis (arity mismatch -> keep going); round 2 matches
		// the now 2-wide hypothesis and the loop settles.
		return []core.Value{
			core.NewCarrier(core.TInteger),
			core.NewCarrier(core.TString),
		}
	}

	out := refineRecursiveSummary(r, key, 0, seed, runOnce)

	if rounds != 2 {
		t.Fatalf("arity widening must force a second refinement round, got %d round(s)", rounds)
	}
	if len(out) != 2 {
		t.Fatalf("refined summary must carry the widened arity, got %d value(s): %+v", len(out), out)
	}
	if !out[0].Carrier() || !out[0].Parent().Equal(core.TInteger) {
		t.Errorf("position 0 must stay an Integer carrier: %+v", out[0])
	}
	if !out[1].Carrier() {
		t.Errorf("position 1 must be a carrier joined from the padded None arm: %+v", out[1])
	}
	if got := r.Check.FnSummaries[key]; len(got) != 2 {
		t.Errorf("memo must hold the widened hypothesis, got %d value(s)", len(got))
	}
}

// TestGatecarrier3902StacksEqualArity pins the guard's contract directly:
// arity decides before any positional compare, in both argument orders, and
// the empty-vs-empty stack is stable.
func TestGatecarrier3902StacksEqualArity(t *testing.T) {
	short := []core.Value{core.NewCarrier(core.TInteger)}
	long := []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)}

	if carrierStacksEqual(short, long) {
		t.Error("a shorter stack must not compare equal to a longer one")
	}
	if carrierStacksEqual(long, short) {
		t.Error("a longer stack must not compare equal to a shorter one")
	}
	if carrierStacksEqual(nil, short) {
		t.Error("an empty stack must not compare equal to a 1-wide stack")
	}
	if !carrierStacksEqual(nil, nil) {
		t.Error("two empty stacks are stable")
	}
	if !carrierStacksEqual(short, short) {
		t.Error("a stack must be stable against itself")
	}
	// Same arity, differing at a position: the positional arm decides.
	if carrierStacksEqual([]core.Value{core.NewInteger(1)}, []core.Value{core.NewInteger(2)}) {
		t.Error("same-arity stacks that differ positionally must not compare equal")
	}
}
