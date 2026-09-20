package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Gate tests for the post-opacity resolution arms the spec suites reach
// only through composed behaviour (ADR-012 rule 4), paired with their
// negatives.

// caseNumericValue declines a non-concrete scrutinee outright — the
// carrier shape static analysis feeds it.
func TestCaseNumericValueNonConcrete(t *testing.T) {
	if _, _, _, ok := caseNumericValue(NewCarrier(TInteger)); ok {
		t.Fatal("a carrier must not read as a numeric value")
	}
	if isInt, n, _, ok := caseNumericValue(NewInteger(7)); !ok || !isInt || n != 7 {
		t.Fatal("a concrete Integer must read exactly")
	}
}

// lookupResourceTypeByName declines a hidden-key binding that is not a
// ResourceType — nothing but installResourceTypes writes these keys,
// so the guard is the seam against a mis-installed schema.
func TestLookupResourceTypeByNameNonResourcePayload(t *testing.T) {
	r := seam5Reg(t)
	InstallDef(r, core.ResourceDefKey("BogusRes"), NewInteger(1))
	if _, ok := lookupResourceTypeByName(r, "BogusRes"); ok {
		t.Fatal("a non-ResourceType hidden binding must not resolve")
	}
}

// getResourceReturns' decline arms: an instance whose type names no
// resource schema stays dynamic, and a schema field declared as a
// Function declines to a dynamic carrier (fn-valued reads are the
// method-shape path's concern).
func TestGetResourceReturnsDeclineArms(t *testing.T) {
	r := seam5Reg(t)
	// Miss: Integer names no resource schema.
	out := getResourceReturns([]Value{NewString("kind"), NewInteger(1)}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("non-resource instance must stay dynamic, got %v", out)
	}
	// Function-typed schema field declines.
	ft := MintTestType("Ideal/FakeRes")
	fields := NewOrderedMap()
	fields.Set("run", NewTypeLiteral(TFunction))
	InstallDef(r, core.ResourceDefKey("FakeRes"),
		NewResourceType(ft, ResourceTypeInfo{Fields: fields, Name: "FakeRes"}))
	inst := core.NewValueRaw(ft, core.IntPayload{N: 1})
	out = getResourceReturns([]Value{NewString("run"), inst}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("a Function-typed field must decline to dynamic, got %v", out)
	}
	// Absent field: reads as None (the schema is closed).
	out = getResourceReturns([]Value{NewString("nope"), inst}, r)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TNone) {
		t.Fatalf("an absent field must read as a None carrier, got %v", out)
	}
}
