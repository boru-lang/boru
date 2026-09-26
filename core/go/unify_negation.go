package core

// Negation (complement) types — the set-theoretic complement that
// closes the type algebra under Boolean operations alongside
// DisjunctInfo (union) and TandValues (intersection). A value satisfies
// `tnot Inner` iff it does NOT satisfy Inner.
//
// Two matching paths, mirroring unify_disjunct.go:
//
//   1. Anonymous constraint value (`5 is (tnot String)`): the negation
//      value is the RHS; Unify's negation fold dispatches here via
//      unifyNegation.
//   2. Named negation type (`def NotStr (tnot String); 5 is NotStr`):
//      InstallType attaches a negationUnifier Behavior carrying the
//      inner type, so every Is/Match call site consults the complement.

// unifyNegation admits val against `tnot Inner`. Asymmetric by design:
// neg is always the negation side. Returns val (the admitted value) on
// success, or a failure when val satisfies the negated inner type. r is
// the enclosing chain's registry, threaded into the inner membership
// test.
func unifyNegation(neg NegationInfo, val Value, r *Registry) (Value, *UnifyError) {
	// Any narrows to the negation constraint itself: Any tand (tnot T)
	// = tnot T (the top intersected with the complement is the
	// complement).
	if !IsConcrete(val) && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewNegation(neg.Inner), nil
	}

	// Concrete value: membership is exact. val satisfies `tnot Inner`
	// iff it does NOT unify with Inner. unifyInner carries the full
	// membership test — scalar subtyping, disjunct alternatives, nested
	// negation, DepScalar bounds — so a refined inner like (Integer gt
	// 0) is checked pointwise against the value.
	if IsConcrete(val) {
		// The complement of a refinement over a bound the analysis pass
		// does not know: the refinement admits every value (depBoundCheck),
		// so its complement would refuse every value, and neither verdict
		// is the bound's. The pass admits, gradually; the run decides
		// (NUR231).
		if HasUnknownRefinement(neg.Inner) {
			return val, nil
		}
		if _, err := unifyInner(neg.Inner, val, r); err != nil {
			return val, nil
		}
		return Value{}, unifyFail("value satisfies the negated type", NewNegation(neg.Inner), val)
	}

	// Abstract operand (type literal / carrier): this is the type-level
	// intersection `val tand tnot Inner`. Unlike a concrete value, "val
	// unifies with Inner" only means their denotations *overlap*, not
	// that val ⊆ Inner — so the unify test would wrongly prove
	// emptiness for a supertype (e.g. Number tand tnot Integer). The
	// intersection is empty (Never) only when val is *wholly* contained
	// in Inner, which the lattice subtype check decides exactly for a
	// plain-type Inner. For a refined or compound Inner (DepScalar /
	// disjunct / negation) a plain abstract val is not wholly contained,
	// so admit it: boru has no positive representation for an exact set
	// difference, and admitting val is the sound over-approximation (it
	// never wrongly proves emptiness).
	// ConformsTo is the containment test (val is Inner or a descendant
	// of Inner — i.e. val ⊆ Inner), unlike the strict IsSubtypeOf which
	// excludes the equal case.
	if isPlainTypeRef(neg.Inner) && denotedType(val).ConformsTo(denotedType(neg.Inner)) {
		return Value{}, unifyFail("type is wholly contained in the negated type", NewNegation(neg.Inner), val)
	}
	return val, nil
}

// isPlainTypeRef reports whether v denotes a plain lattice type — not a
// refinement (DepScalar), disjunct, or negation. For a plain Inner,
// lattice subtyping decides negation emptiness exactly; for a compound
// Inner it does not, and unifyNegation falls back to admitting the
// abstract operand.
func isPlainTypeRef(v Value) bool {
	return !v.IsDepScalar() && !IsDisjunct(v) && !IsNegation(v)
}

// negationUnifier is the kernel-installed Behavior for named negation
// types — `def NotStr (tnot String)`. Like disjunctUnifier, it lives on
// the minted lattice node so every "is v a NotStr?" path consults the
// complement.
type NegationUnifier struct {
	behaviorWrapper
	inner    Value
	typeName string
}

// Match admits v iff v does not satisfy the inner type. A bare type
// literal (the type itself, not an inhabitant) passes through to the
// prev/Default walk — mirroring disjunctUnifier.
func (*NegationUnifier) ContentMembership() {}

func (n *NegationUnifier) Match(v Value, t *Type) bool {
	return n.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the complement walk (see isR).
func (n *NegationUnifier) matchR(v Value, t *Type, r *Registry) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(n.prev).Match(v, t)
	}
	_, err := unifyNegation(NegationInfo{Inner: n.inner}, v, r)
	return err == nil
}

// Unify admits a concrete candidate iff it does NOT satisfy the inner
// type, yielding the candidate; a concrete member of the inner type
// fails definitively, and a type-level pair defers to the structural
// rule — the Unify capability every membership kind carries
// (design/legacy/TYPE-REPRESENTATION.1.ignore §N3).
func (n *NegationUnifier) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	// The complement rule decides whenever exactly one side IS this
	// negation's node — concrete, carrier, or type-level candidate
	// alike, exactly as the payload fold decided them for the body.
	aSelf := IsBareTypeNode(a) && a.Behavior() == TypeBehavior(n)
	bSelf := IsBareTypeNode(b) && b.Behavior() == TypeBehavior(n)
	if aSelf == bSelf {
		return unifySameOrSubtype(a, b)
	}
	candidate := a
	if aSelf {
		candidate = b
	}
	return unifyNegation(NegationInfo{Inner: n.inner}, candidate, r)
}

// installNegationUnifier attaches a negationUnifier to def, wrapping any
// existing Behavior. Called by InstallType when minting a negation type
// so the complement drives every Is/Match call site.
func installNegationUnifier(def *Type, inner Value, name string) {
	def.ensureTMeta().Behavior = &NegationUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		inner:           inner,
		typeName:        name,
	}
}
