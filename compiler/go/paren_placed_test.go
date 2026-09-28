package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestParenPlacedMemberFnGuards covers parenPlacedMemberFn's defensive
// declines. resolveDynamicApply asks it for every residual lead, so a nil
// emit state, a stateless registry and an id-less value all reach it.
func TestParenPlacedMemberFnGuards(t *testing.T) {
	var es *EmitState
	if es.parenPlacedMemberFn(core.NewInteger(1)) {
		t.Error("a nil EmitState must decline")
	}
	live := &EmitState{}
	if live.parenPlacedMemberFn(core.NewInteger(1)) {
		t.Error("an EmitState with no registry must decline")
	}
}

// TestRestepOverloadReachable pins tryRecordPoly's re-step decline, keyed on
// an overload's DECLARATION rather than its arity (NUR100 §2): a word with an
// overload declaring CompileResteps declines a poly window whose operands
// can reach that overload — an Any carrier reaches any slot — and keeps it
// when they cannot, or when no overload re-steps, WHATEVER the overloads'
// arities. The lookup miss is a live answer too: tryRecordPoly asks with the
// dispatching word's name, which is not always a registered builtin.
func TestRestepOverloadReachable(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	anyCarrier := core.NewCarrier(core.TAny)
	if restepOverloadReachable(reg, "no-such-native-xyz", []core.Value{anyCarrier}) {
		t.Error("an unregistered word has no overloads at all, so none re-steps")
	}
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	// A 1-slot re-stepping overload beside a 2-slot plain one — `apply`'s
	// shape — and a word of the same shape whose overloads push results.
	reg.RegisterNativeFunc(core.NativeFunc{
		Name: "restep-xyz",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TFunction}, Impl: core.Go(noop), Returns: []*core.Type{core.TAny}, CompileEffect: core.CompileResteps},
			{Args: []*core.Type{core.TReach, core.TAny}, Impl: core.Go(noop), Returns: []*core.Type{core.TAny}},
		},
	})
	reg.RegisterNativeFunc(core.NativeFunc{
		Name: "plain-xyz",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TFunction}, Impl: core.Go(noop), Returns: []*core.Type{core.TAny}},
			{Args: []*core.Type{core.TReach, core.TAny}, Impl: core.Go(noop), Returns: []*core.Type{core.TAny}},
		},
	})
	if !restepOverloadReachable(reg, "restep-xyz", []core.Value{anyCarrier, core.NewInteger(1)}) {
		t.Error("an Any carrier on top reaches the re-stepping [Function] overload")
	}
	if restepOverloadReachable(reg, "restep-xyz", []core.Value{core.NewInteger(7), anyCarrier}) {
		t.Error("an Integer on top cannot reach a [Function] slot")
	}
	if restepOverloadReachable(reg, "plain-xyz", []core.Value{anyCarrier, core.NewInteger(1)}) {
		t.Error("no overload re-steps, so the window's arity alone declines nothing")
	}
	// A slot past the window reads a value the record never saw.
	if !restepOverloadReachable(reg, "restep-xyz", nil) {
		t.Error("an overload reading only operands past the window is not ruled out")
	}
}
