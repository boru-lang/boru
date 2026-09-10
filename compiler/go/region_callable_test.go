package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// region_callable_test.go pins the predicate behind the callable guard —
// regionValsMayBeCallable — and the deliberate WIDTH of its dynamic arm.
//
// The guard exists because a Function that arrives on the stack is re-stepped
// by the interpreter and appended as data by the VM, and a region has no
// per-value seat to re-step from. The predicate therefore answers in the
// widening direction: a value whose runtime type the model does not bound is
// treated as possibly-callable, which is why a DYNAMIC residual counts.
//
// That width has a cost worth pinning rather than discovering: it declines
// regions the model simply cannot vouch for, not only ones that carry a fn.
// The narrowing (a dynamic residual that provably excludes Function) is
// NUR129's, along with the bare loop residual the guard deliberately does not
// reach.

func TestRegionValsMayBeCallableAnswersWide(t *testing.T) {
	fnCarrier := core.NewCarrier(core.TFunction)
	dyn := core.NewDynamicCarrier(core.TAny)

	for _, c := range []struct {
		name string
		vals []core.Value
		want bool
	}{
		// The shapes that plainly can dispatch.
		{"a Function carrier", []core.Value{fnCarrier}, true},
		{"a Function among fixed values", []core.Value{core.NewInteger(5), fnCarrier}, true},
		// A DYNAMIC value: the model does not bound its runtime type, so it
		// might be callable. This is the arm that makes the guard wide.
		{"a dynamic value", []core.Value{dyn}, true},
		{"a dynamic among fixed values", []core.Value{core.NewInteger(5), dyn}, true},
		// The shapes that cannot.
		{"concrete scalars", []core.Value{core.NewInteger(1), core.NewString("x")}, false},
		{"typed carriers", []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TList)}, false},
		// Nothing at all — an empty body leaves nothing to re-step, which is
		// why `await {mode:'first'} [[]]` keeps compiling.
		{"no values", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := regionValsMayBeCallable(c.vals); got != c.want {
				t.Errorf("regionValsMayBeCallable = %v, want %v", got, c.want)
			}
		})
	}
}

// TestSingleSlotRegionRejectsACallableCarrier — the guard's seat. A region
// event that may carry a callable is not a region either consumer may take,
// so both plans decline through singleSlotRegion rather than each repeating
// the test.
func TestSingleSlotRegionRejectsACallableCarrier(t *testing.T) {
	es := rpState(t)
	seq, _ := rpLoop(es, emitLoop{hasBodyOut: true})
	if !es.singleSlotRegion(es.topLevelEventBySeq(seq)) {
		t.Fatal("the loop's region is a single-slot region to begin with")
	}
	f := es.eventInfo[seq]
	f.regionMayBeFn = true
	es.eventInfo[seq] = f
	if es.singleSlotRegion(es.topLevelEventBySeq(seq)) {
		t.Error("a region that may carry a callable must not be consumable")
	}
}

// TestStoredBodyIsEmptyReadsTheZeroTokenList — the one declined body shape
// whose residual is still KNOWN. Without it an empty branch would read as
// "residual unknown, so possibly callable" and `await {mode:'first'} [[]]`
// would refuse for a reason that is not true of it.
func TestStoredBodyIsEmptyReadsTheZeroTokenList(t *testing.T) {
	if !storedBodyIsEmpty(core.NewList([]core.Value{})) {
		t.Error("a zero-token list is an empty body")
	}
	if storedBodyIsEmpty(core.NewList([]core.Value{core.NewInteger(1)})) {
		t.Error("a one-token list is not an empty body")
	}
	if storedBodyIsEmpty(core.NewInteger(1)) {
		t.Error("a non-list element is not an empty body")
	}
	// A List with NO payload is a carrier, not a body: it never reaches the
	// branch runner's interpret path at all (runParallelBranch requires a
	// payload), so it is not this predicate's "empty".
	if storedBodyIsEmpty(core.NewList(nil)) {
		t.Error("a payload-less List is not an empty body")
	}
}
