package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// variadic_region_test.go pins the two RECORD-side predicates of the
// runtime-variadic REGION (NUR067's graduation): which dispatch residual is
// a region, and which program residual therefore may not be classified as a
// fn-value apply.
//
// Both are total functions over values the recorder already holds, so they
// are pinned here at the seam rather than only through the corpus — the
// corpus reaches each predicate through exactly one word (`await`'s
// winner-takes-all modes), and a second producer arriving later must find
// the contract written down rather than inferred from that one row.
//
// The whole-program halves are lang/go/bytecode_await_test.go
// (TestAwaitCompiledBranchParity's three counts and
// TestAwaitWinnerRegionRefusesFixedArityConsumers' four refusals).

// vrSpread is the check-side model of "0-or-more values of element type" —
// what a region-producing word's ReturnsFn hands back on both passes.
func vrSpread() core.Value { return core.NewVariadicCarrier(core.NewTypeLiteral(core.TAny)) }

func TestCallVariadicRegionIdentifiesTheOneOutSpread(t *testing.T) {
	if !callVariadicRegion([]core.Value{vrSpread()}) {
		t.Error("one spread out is a region")
	}
	// An ORDINARY residual: the overwhelmingly common call, and the arm that
	// keeps this predicate from marking every dispatch.
	if callVariadicRegion([]core.Value{core.NewInteger(7)}) {
		t.Error("a concrete result is not a region")
	}
	if callVariadicRegion([]core.Value{core.NewCarrier(core.TAny)}) {
		t.Error("a plain dynamic-free carrier is not a region")
	}
	// A side-effect word leaves nothing; there is no slot to stand for a run.
	if callVariadicRegion(nil) {
		t.Error("a zero-out dispatch is not a region")
	}
	// The count is what makes a region representable: ONE recorded slot for
	// the whole run. A spread beside other values has no such slot, so it is
	// deliberately NOT marked — it stays an ordinary fixed-arity residual and
	// refuses at its consumers rather than being lowered as a region.
	if callVariadicRegion([]core.Value{vrSpread(), core.NewInteger(7)}) {
		t.Error("a spread in a multi-out residual is not a region")
	}
}

func TestResidualHasVariadicRegionFindsTheRecordedEvent(t *testing.T) {
	es := NewEmitState()
	es.BindRegistry(seam7Reg(t))

	plain := core.NewInteger(99)
	region := vrSpread()
	region.ID = core.GenerateID("zzrg")

	// Nothing recorded yet: an unproduced value is not a region, which is
	// also the arm every const-only residual takes.
	if es.residualHasVariadicRegion([]core.Value{plain, region}) {
		t.Error("an unrecorded value cannot be a region")
	}

	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzawait", nout: 1}})
	es.setProduced(region, seq)

	// Produced, but recorded as an ORDINARY event — the flag is what marks a
	// region, not the carrier's shape, so provenance alone must not trip it.
	if es.residualHasVariadicRegion([]core.Value{plain, region}) {
		t.Error("a produced value whose event is not a region must not trip the guard")
	}

	f := es.eventInfo[seq]
	f.variadicResult = true
	f.variadicRegion = true
	es.eventInfo[seq] = f

	if !es.residualHasVariadicRegion([]core.Value{plain, region}) {
		t.Error("a region entry anywhere in the residual disqualifies the fn-value-call boundary")
	}
	if es.residualHasVariadicRegion([]core.Value{plain}) {
		t.Error("a residual with no region entry is classified as usual")
	}
}
