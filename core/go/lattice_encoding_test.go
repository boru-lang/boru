package core

import "testing"

// walkAncestor is the structural reference for IsAncestor — the parent-chain
// walk, independent of the In/Out interval fast path. The optimisation is
// sound iff it agrees with this for every pair.
func walkAncestor(t, ancestor *Type) bool {
	for x := t; x != nil; x = x.Parent() {
		if x.Equal(ancestor) {
			return true
		}
	}
	return false
}

// walkLCA is the O(d_a·d_b) chain-vs-chain reference the depth-aligned
// lowestCommonAncestor replaced.
func walkLCA(a, b *Type) *Type {
	var ac []*Type
	for t := a; t != nil; t = t.Parent() {
		ac = append(ac, t)
	}
	for t := b; t != nil; t = t.Parent() {
		for _, at := range ac {
			if at.Equal(t) {
				return t
			}
		}
	}
	return nil
}

// TestBuiltinsFullyLabelled pins that every builtin type gets a Depth and a
// DFS interval at table construction — the precondition for the IsAncestor
// fast path and the cached typeDepth.
func TestBuiltinsFullyLabelled(t *testing.T) {
	for _, ty := range Builtin.byID {
		if ty.Depth() <= 0 {
			t.Errorf("builtin %q has unset Depth %d", ty.Path(), ty.Depth())
		}
		if ty.In() <= 0 || ty.Out() < ty.In() {
			t.Errorf("builtin %q has invalid interval [%d,%d]", ty.Path(), ty.In(), ty.Out())
		}
	}
}

// TestIntervalAncestryMatchesWalk is the soundness gate for item 2: the
// O(1) interval test must agree with the structural walk for EVERY ordered
// pair of builtins (both the true and the false cases), and the depth-aligned
// LCA must agree with the chain-scan reference.
func TestIntervalAncestryMatchesWalk(t *testing.T) {
	var all []*Type
	for _, ty := range Builtin.byID {
		all = append(all, ty)
	}
	for _, a := range all {
		for _, b := range all {
			if got, want := a.IsAncestor(b), walkAncestor(a, b); got != want {
				t.Errorf("IsAncestor(%s,%s)=%v, walk=%v", a.Path(), b.Path(), got, want)
			}
			got, want := lowestCommonAncestor(a, b), walkLCA(a, b)
			switch {
			case (got == nil) != (want == nil):
				t.Errorf("LCA(%s,%s) nil-ness mismatch: %v vs %v", a.Path(), b.Path(), got, want)
			case got != nil && want != nil && !got.Equal(want):
				t.Errorf("LCA(%s,%s)=%s, walk=%s", a.Path(), b.Path(), got.Path(), want.Path())
			}
		}
	}
}

// TestDepthMatchesChainLength checks the cached Depth equals the walked
// chain length for a representative spread, including a leaf and a root.
func TestDepthMatchesChainLength(t *testing.T) {
	for _, ty := range []*Type{TAny, TScalar, TNumber, TInteger, TString} {
		want := 0
		for x := ty; x != nil; x = x.Parent() {
			want++
		}
		if ty.Depth() != want {
			t.Errorf("%s: Depth=%d, chain length=%d", ty.Path(), ty.Depth(), want)
		}
	}
}

// TestMintedTypeRoutesThroughWalk is the negative case: a dynamically minted
// type is NOT labelled (In==0), so IsAncestor must fall back to the walk and
// still answer correctly — both the positive (its real ancestor) and the
// negative (an unrelated builtin) directions.
func TestMintedTypeRoutesThroughWalk(t *testing.T) {
	tt := NewDynamicTypeTable()
	pos := tt.MintType("Pos", TInteger)

	if pos.In() != 0 {
		t.Fatalf("minted type should be unlabelled, got In=%d", pos.In())
	}
	if pos.Depth() != TInteger.Depth()+1 {
		t.Errorf("minted Pos Depth=%d, want %d", pos.Depth(), TInteger.Depth()+1)
	}
	// Positive: Pos descends from Integer, Number, Scalar, Any.
	for _, anc := range []*Type{TInteger, TNumber, TScalar, TAny} {
		if !pos.IsAncestor(anc) {
			t.Errorf("Pos should conform to %s", anc.Path())
		}
	}
	// Negative: Pos is not a String, and Integer is not a Pos.
	if pos.IsAncestor(TString) {
		t.Error("Pos must not conform to String")
	}
	if TInteger.IsAncestor(pos) {
		t.Error("Integer must not be a subtype of minted Pos")
	}
	// LCA with a sibling resolves to the shared builtin ancestor.
	if lca := lowestCommonAncestor(pos, TFloat); lca == nil || !lca.Equal(TNumber) {
		t.Errorf("LCA(Pos,Float)=%v, want Number", lca)
	}
}
