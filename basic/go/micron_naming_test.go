package basic

// The -on naming-rule enforcement tests, moved from eng with the rule:
// the rule is now the family Behavior's SubtypeNamer capability
// (ADR-012 rule 5), so the kernel paths only enforce it where the
// content layer is linked — which is this package's test binary, via
// the init() Behavior attach.

import (
	"strings"
	"testing"
)

func TestS6b0InstallTypeDepScalarOverMicronNeedsOnName(t *testing.T) {
	r := newTestRegistry(t)
	dep := NewDepScalar(DepGT, NewInteger(10))
	if !dep.IsDepScalar() {
		t.Fatal("premise: NewDepScalar builds a DepScalar")
	}
	dep.SetParent(TPathon) // a dependent scalar over a Micron base
	err := InstallType(r, "S6b0Bad", dep)
	if err == nil || !strings.Contains(err.Error(), "must end in the suffix 'on'") {
		t.Fatalf("expected the Micron naming error, got %v", err)
	}
}

func TestS7DefineMemberTypeMicronNaming(t *testing.T) {
	r := newTestRegistry(t)
	// A member type minted under the Micron branch must carry the -on
	// suffix; a plain capitalised name is rejected.
	_, err := r.DefineMemberType("S7Bad", TMicron, func(Value) bool { return true })
	if err == nil {
		t.Error("a Micron-parented member type without the -on suffix must fail")
	}
}
