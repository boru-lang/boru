package core

import (
	"testing"
)

// zzFormatBehavior is a base whose Formatter reads a VALUE of the base, the
// way Bytes' does — it has no refinement to read.
type zzFormatBehavior struct{ defaultBehavior }

func (zzFormatBehavior) Format(Value) string { return "Zz<?>" }

// TestRefinementBaseIsDeclared pins NUR009's capability: a type is a
// refinement base because it DECLARED itself one, not because a resolver
// lists it — core's six leaves are declared, a subtype refines as its
// declaring ancestor, an undeclared type is no base, and a type declared
// later (basic's Bytes, here a fresh node) becomes one.
func TestRefinementBaseIsDeclared(t *testing.T) {
	for _, tc := range []struct{ t, want *Type }{
		{TInteger, TInteger}, {TFloat, TFloat}, {TNumber, TNumber},
		{TString, TString}, {TBoolean, TBoolean}, {TAtom, TAtom},
		{TList, nil}, {TMap, nil}, {TScalar, nil}, // undeclared: no base
	} {
		if got := canonicalBaseType(tc.t); got != tc.want {
			t.Errorf("canonicalBaseType(%s) = %v, want %v", tc.t, got, tc.want)
		}
	}
	// A subtype refines as its declaring ancestor: 'abc' is a ProperString.
	if got := canonicalBaseType(NewString("abc").Parent()); got != TString {
		t.Errorf("a String subtype's base = %v, want String", got)
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	zz := r.Types.MintType("Zz", TScalar)
	child := r.Types.MintType("ZzChild", zz)
	if canonicalBaseType(zz) != nil || canonicalBaseType(child) != nil {
		t.Fatal("an undeclared type is no refinement base")
	}
	DeclareRefinementBase(zz)
	DeclareRefinementBase(nil) // a failed registration declares nothing
	if canonicalBaseType(zz) != zz || canonicalBaseType(child) != zz {
		t.Fatalf("a declared type is its own base and its subtypes': %v %v", canonicalBaseType(zz), canonicalBaseType(child))
	}
	if dep := NewDepScalar(DepGT, NewValueRaw(child, IntPayload{N: 1})); !dep.IsDepScalar() || dep.Parent() != zz {
		t.Fatalf("a bound under a declared base builds a refinement over it: %v", dep)
	}
}

// TestRefinementRendersOverAFormatterBase pins the display half of NUR009: a
// refinement renders in the comparison vocabulary even when its base has a
// Formatter (Bytes' printed `Bytes<?>` for every Bytes refinement, and the
// compile pass's const pool, keyed on that rendering, merged two of them).
// A plain value of the base still renders through the Formatter.
func TestRefinementRendersOverAFormatterBase(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	zz := r.Types.MintTypeWithBehavior("Zz", TScalar, zzFormatBehavior{})
	DeclareRefinementBase(zz)
	dep := NewValueRaw(zz, DepScalarInfo{Hi: &DepBound{Value: NewInteger(7)}})
	if got := dep.String(); got != "(Zz lt 7)" {
		t.Errorf("a refinement over a Formatter base renders %q, want (Zz lt 7)", got)
	}
	if got := NewValueRaw(zz, IntPayload{N: 1}).String(); got != "Zz<?>" {
		t.Errorf("a plain value of the base renders %q, want its Formatter's Zz<?>", got)
	}
	// The kernel's default renderer — reached directly by any Behavior that
	// embeds defaultBehavior — renders a refinement the same way.
	if got := (defaultBehavior{}).Format(NewDepScalar(DepGT, NewInteger(3))); got != "(Integer gt 3)" {
		t.Errorf("the default renderer gives %q for a refinement, want (Integer gt 3)", got)
	}
}

// TestUnknownBoundDecidesNothing pins the membership half of NUR308: a
// carrier bound orders below every value, so a verdict over it was the
// lattice's — `Integer lte (size s)` refused 3 at check time whatever s
// held. An unknown bound admits (gradually), and so does an interval whose
// OTHER side is known — that verdict would render the placeholder; a
// refinement over known bounds still decides.
func TestUnknownBoundDecidesNothing(t *testing.T) {
	unknown := &DepBound{Value: NewCarrier(TInteger)}
	known5 := &DepBound{Inclusive: true, Value: NewInteger(5)}
	for _, info := range []DepScalarInfo{
		{Lo: unknown}, {Hi: unknown}, {Lo: known5, Hi: unknown}, {Lo: unknown, Hi: known5},
	} {
		if !depScalarCheck(info, NewInteger(3)) {
			t.Errorf("%+v decides nothing: it must admit 3", info)
		}
	}
	if depScalarCheck(DepScalarInfo{Lo: known5, Hi: &DepBound{Inclusive: true, Value: NewInteger(9)}}, NewInteger(3)) {
		t.Error("[5, 9] over known bounds refuses 3")
	}
	if depBoundCheck(&DepBound{Value: NewInteger(3)}, true, NewInteger(3)) {
		t.Error("a known strict lower bound 3 must refuse 3")
	}
	if depBoundCheck(&DepBound{Value: NewInteger(3)}, false, NewInteger(5)) {
		t.Error("a known strict upper bound 3 must refuse 5")
	}
}

// TestRefinementConstOnlyOverKnownBounds pins the const gate: a refinement
// bakes only when every bound it carries is itself a const.
func TestRefinementConstOnlyOverKnownBounds(t *testing.T) {
	known := NewDepScalar(DepGT, NewInteger(3))
	if !IsInertConst(&known) {
		t.Error("a refinement over a known bound is a const")
	}
	if IsInertConst(addrOf(NewDepScalar(DepGT, NewCarrier(TInteger)))) {
		t.Error("a refinement over an unknown bound is no const")
	}
	half := NewValueRaw(TInteger, DepScalarInfo{
		Lo: &DepBound{Inclusive: true, Value: NewInteger(1)},
		Hi: &DepBound{Inclusive: true, Value: NewCarrier(TInteger)},
	})
	if IsInertConst(&half) {
		t.Error("an interval with one unknown side is no const")
	}
}
