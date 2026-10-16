package core

import "testing"

// TestDefaultBehaviorInstalled verifies that every builtin *Type
// carries a non-nil Behavior after init. The Behavior seam relies on
// this invariant — any code path that calls t.Behavior.Match(...)
// without nil-checking would panic on a Type that escaped
// registration without Behavior wiring.
func TestDefaultBehaviorInstalled(t *testing.T) {
	if len(Builtin.byID) == 0 {
		t.Fatal("Builtin TypeTable is empty")
	}
	for id, def := range Builtin.byID {
		if def == nil {
			t.Errorf("Builtin.byID[%q] is nil", id)
			continue
		}
		if def.Behavior() == nil {
			t.Errorf("Builtin type %q (%s) has nil Behavior", def.Path(), id)
		}
	}
}

// TestMintTypeInstallsDefaultBehavior verifies that MintType wires a
// non-nil Behavior on dynamically-minted types.
func TestMintTypeInstallsDefaultBehavior(t *testing.T) {
	tt := NewDynamicTypeTable()
	def := tt.MintType("DynamicFoo", TInteger)
	if def.Behavior() == nil {
		t.Fatal("MintType returned a Type with nil Behavior")
	}
}

// TestMintTypeWithBehaviorRespectsArg verifies that
// MintTypeWithBehavior installs the supplied Behavior, falling back
// to DefaultBehavior on nil.
func TestMintTypeWithBehaviorRespectsArg(t *testing.T) {
	tt := NewDynamicTypeTable()

	custom := &fakeBehavior{}
	a := tt.MintTypeWithBehavior("WithCustom", TInteger, custom)
	if a.Behavior() != custom {
		t.Errorf("WithCustom: got Behavior %T, want custom", a.Behavior)
	}

	b := tt.MintTypeWithBehavior("WithNil", TInteger, nil)
	if b.Behavior() != DefaultBehavior {
		t.Errorf("WithNil: got Behavior %T, want DefaultBehavior", b.Behavior)
	}
}

// TestDefaultBehaviorMatchDelegates verifies DefaultBehavior.Match
// agrees with the historical lattice walk on a representative
// hierarchy (Integer ⊂ Number ⊂ Scalar).
func TestDefaultBehaviorMatchDelegates(t *testing.T) {
	v := NewInteger(42)
	cases := []struct {
		target *Type
		want   bool
	}{
		{TInteger, true},
		{TNumber, true},
		{TScalar, true},
		{TAny, true},
		{TString, false},
		{TList, false},
	}
	for _, c := range cases {
		got := DefaultBehavior.Match(v, c.target)
		if got != c.want {
			t.Errorf("DefaultBehavior.Match(NewInteger(42), %s) = %v, want %v",
				c.target, got, c.want)
		}
	}
}

// fakeBehavior is a test-only TypeBehavior used to verify that
// MintTypeWithBehavior installs the supplied Behavior verbatim.
type fakeBehavior struct{}

func (fakeBehavior) Match(Value, *Type) bool { return false }
func (fakeBehavior) Format(Value) string     { return "fake" }
func (fakeBehavior) Equal(Value, Value) bool { return false }

// TestValueIsRoutesThroughBehavior verifies that v.Is(t) consults
// t.Behavior — a custom Behavior whose Match returns false even on
// a value whose Parent matches t must produce v.Is(t) == false.
func TestValueIsRoutesThroughBehavior(t *testing.T) {
	tt := NewDynamicTypeTable()
	rejecting := &rejectingBehavior{}
	custom := tt.MintTypeWithBehavior("AlwaysReject", TInteger, rejecting)

	// Construct an Integer value whose Parent is the custom type so
	// the lattice walk WOULD say yes; verify Behavior overrides.
	v := MakeValue(custom, IntPayload{N: 5})
	if v.Is(custom) {
		t.Error("v.Is(custom) returned true; custom Behavior should reject")
	}
	if !rejecting.matchCalled {
		t.Error("custom Behavior.Match was never called")
	}
}

// TestValueIsHandlesNilType verifies v.Is(nil) returns false without
// panicking. Safe-on-nil is a hard contract.
func TestValueIsHandlesNilType(t *testing.T) {
	v := NewInteger(42)
	if v.Is(nil) {
		t.Error("v.Is(nil) returned true; expected false")
	}
}

// rejectingBehavior is a test Behavior whose Match always returns
// false and records the call. Used to prove Behavior is consulted.
type rejectingBehavior struct{ matchCalled bool }

func (r *rejectingBehavior) Match(Value, *Type) bool { r.matchCalled = true; return false }
func (rejectingBehavior) Format(Value) string        { return "reject" }
func (rejectingBehavior) Equal(Value, Value) bool    { return false }

// TestValuesEqualRoutesThroughBehavior verifies that a same-Parent
// pair with a custom Behavior delegates to Behavior.Equal. The
// custom Behavior here returns "always equal" regardless of
// payload — proving the delegation is consulted instead of the
// default deep-compare path.
func TestValuesEqualRoutesThroughBehavior(t *testing.T) {
	tt := NewDynamicTypeTable()
	custom := tt.MintTypeWithBehavior("AlwaysEqual", TInteger, alwaysEqualBehavior{})

	a := MakeValue(custom, IntPayload{N: 1})
	b := MakeValue(custom, IntPayload{N: 99})

	if !ValuesEqual(a, b) {
		t.Error("ValuesEqual returned false; custom Behavior should report equal")
	}
}

// TestValuesEqualSkipsBehaviorOnDifferentVTypes verifies the
// delegation is only triggered when both sides share Parent. With
// different VTypes the historical default-path switch runs.
func TestValuesEqualSkipsBehaviorOnDifferentVTypes(t *testing.T) {
	tt := NewDynamicTypeTable()
	custom := tt.MintTypeWithBehavior("AlwaysEqual", TInteger, alwaysEqualBehavior{})

	a := MakeValue(custom, IntPayload{N: 1})
	b := NewInteger(2)

	// Different VTypes (custom vs TInteger): falls into the default
	// switch, which compares Integer payloads — 1 != 2, so equal=false.
	if ValuesEqual(a, b) {
		t.Error("ValuesEqual returned true; default path should compare integers (1 != 2)")
	}
}

// alwaysEqualBehavior reports true for every Equal call. Used to
// prove the delegation route in ValuesEqual.
type alwaysEqualBehavior struct{}

func (alwaysEqualBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (alwaysEqualBehavior) Format(v Value) string       { return DefaultBehavior.Format(v) }
func (alwaysEqualBehavior) Equal(Value, Value) bool     { return true }
