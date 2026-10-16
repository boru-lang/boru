package native

import "testing"

// PathSubtype is the lexical, path-prefix-only sibling of ConformsTo.
// These tests pin its semantics — no `Any` matches-everything, no
// metatype rules, no Dep<Leaf> override. The simpler check is
// what callers want when they're about to do `v.AsX()` and a
// `DepX` payload would be a silent miscompile.

func TestPathSubtype_Identity(t *testing.T) {
	if !TInteger.PathSubtype(TInteger) {
		t.Errorf("Integer.PathSubtype(Integer) = false, want true")
	}
}

func TestPathSubtype_StrictChild(t *testing.T) {
	// Integer is at Integer; Number is at Number.
	// Integer's parts strictly extend Number's, so it's a path
	// subtype.
	if !TInteger.PathSubtype(TNumber) {
		t.Errorf("Integer.PathSubtype(Number) = false, want true")
	}
}

func TestPathSubtype_ParentNotChild(t *testing.T) {
	// Number is NOT a path subtype of Integer — Number's parts are
	// shorter, so the prefix-of-pattern check fails.
	if TNumber.PathSubtype(TInteger) {
		t.Errorf("Number.PathSubtype(Integer) = true, want false")
	}
}

func TestPathSubtype_DisjointTypes(t *testing.T) {
	if TString.PathSubtype(TInteger) {
		t.Errorf("String.PathSubtype(Integer) = true, want false")
	}
}

// Under the Any-root lattice, `Any` IS the structural top — every
// type chains to it via Parent — so PathSubtype now agrees with
// ConformsTo: Integer.PathSubtype(Any) is true (Any is on Integer's
// parent chain), and ConformsTo returns true for the same reason
// (Any is still TAny-special-cased in ConformsTo as a fast path, but
// the ancestor walk would find it anyway).
func TestPathSubtype_AnyIsLatticeRoot(t *testing.T) {
	if !TInteger.PathSubtype(TAny) {
		t.Errorf("Integer.PathSubtype(Any) = false (Any is the lattice root — every type chains up to it)")
	}
	if !TInteger.ConformsTo(TAny) {
		t.Errorf("Integer.ConformsTo(Any) = false")
	}
}

// A DepScalar value's Parent IS the base scalar (e.g. TInteger), so
// PathSubtype is trivially the identity relation here and ConformsTo is
// true by ancestry walk — no bolt-on override is needed. The
// constraint payload (DepScalarInfo) carries the refinement, detected
// via v.IsDepScalar() at unify time.
func TestPathSubtype_DepScalarParentIsBase(t *testing.T) {
	dep := NewDepScalar(DepGT, NewInteger(10))
	if !dep.Parent().Equal(TInteger) {
		t.Errorf("DepScalar(Integer).Parent = %s, want Integer", dep.Parent().String())
	}
	if !dep.Parent().ConformsTo(TInteger) {
		t.Errorf("DepScalar(Integer).ConformsTo(Integer) = false")
	}
	if !dep.IsDepScalar() {
		t.Errorf("DepScalar(Integer).IsDepScalar() = false")
	}
}

// PathSubtype agrees with ConformsTo on the cases neither special-rule
// touches — strict path-prefix relationships at the bottom of the
// lattice.
func TestPathSubtype_AgreesWithConformsToOnPrefixCases(t *testing.T) {
	cases := []struct {
		name            string
		t               *Type
		pattern         *Type
		wantPathSubtype bool
		wantConforms    bool
	}{
		{"Integer⊆Number", TInteger, TNumber, true, true},
		{"Number⊆Scalar", TNumber, TScalar, true, true},
		{"String⊆Scalar", TString, TScalar, true, true},
		{"Integer⊄String", TInteger, TString, false, false},
		{"Number⊄Integer", TNumber, TInteger, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.PathSubtype(tc.pattern); got != tc.wantPathSubtype {
				t.Errorf("PathSubtype: got %v, want %v", got, tc.wantPathSubtype)
			}
			if got := tc.t.ConformsTo(tc.pattern); got != tc.wantConforms {
				t.Errorf("ConformsTo: got %v, want %v", got, tc.wantConforms)
			}
		})
	}
}
