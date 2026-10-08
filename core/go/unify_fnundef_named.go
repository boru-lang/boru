package core

// Named function-SHAPE types — `def IntToStr fnsig Integer String`,
// then `def h fn g:IntToStr String [(g 5)]`.
//
// This closes the last gap in "Function is opaque": the structural
// subtyping rule already existed (FnSigSatisfiesSpec — contravariant
// params, covariant returns) and Unify already routed an anonymous
// constraint through it (unify_fnsig.go), so `f/v is (fnsig …)` worked.
// What did not work was DISPATCH against a NAMED fn-shape type, for
// exactly the reason the disjunct / negation / DepScalar branches of
// InstallType document: a minted lattice node is on no value's parent
// chain, so the plain lattice walk rejects every function. Those three
// branches each install a Behavior to bridge it; this is the fourth,
// and the last metatype that was missing one.
//
// Two matching paths, mirroring unify_disjunct.go and unify_negation.go:
//
//  1. Anonymous constraint value (`f/v is (fnsig Integer String)`):
//     the FunctionSignature value is the RHS and Unify's FnUndef fold
//     dispatches to unifyFnUndefShape.
//  2. Named fn-shape type (`def IntToStr fnsig Integer String` then
//     `g:IntToStr`): InstallType attaches a FnUndefUnifier carrying the
//     signature specs, so every Is/Match call site — `is`, parameter
//     dispatch, and the checker's carrier test alike — consults the
//     same structural rule.

// FnUndefUnifier is the kernel-installed Behavior for named fn-shape
// types. Like DisjunctUnifier and NegationUnifier it lives on the
// minted lattice node, so "is this value an IntToStr?" resolves
// structurally rather than by ancestry.
type FnUndefUnifier struct {
	behaviorWrapper
	sigs     []FnSigSpec
	typeName string
}

// ContentMembership marks membership as a property of the VALUE's
// content (its signatures), not of its lattice position — the same
// declaration the disjunct and negation unifiers make.
func (*FnUndefUnifier) ContentMembership() {}

// Match admits v iff v is a function value with at least one overload
// satisfying every declared signature. A bare type literal (the type
// itself, not an inhabitant) passes through to the prev/Default walk,
// mirroring DisjunctUnifier — `IntToStr is Type` must stay true.
func (f *FnUndefUnifier) Match(v Value, t *Type) bool {
	return f.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the signature rule's Pattern-compatibility unify (see isR).
func (f *FnUndefUnifier) matchR(v Value, t *Type, r *Registry) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(f.prev).Match(v, t)
	}
	// A carrier (abstract value) carries no signatures to inspect. Admit
	// it when it could still be a function, so the checker does not
	// reject a program the runtime accepts — the sound over-approximation
	// the negation unifier makes for the same reason.
	if v.Carrier {
		return v.Parent.ConformsTo(TFunction) || v.Parent.Equal(TAny)
	}
	return fnUndefMatchesFnDefR(NewValueRaw(TFnUndef, FnUndefInfo{Sigs: f.sigs}), v, r)
}

// Unify admits a concrete function candidate by the same structural
// signature check Match runs, yielding the CANDIDATE (never the node
// literal — the typed-def swap hazard), and failing definitively on a
// concrete non-member; a type-level pair defers to the structural rule.
// The Unify capability every membership kind carries
// (design/legacy/TYPE-REPRESENTATION.1.ignore §N3): without it, `def f:IntToStr
// fn […]` against the node constraint would fall to unifySameOrSubtype's
// narrower-literal arm and bind the literal instead of the function.
// r is the enclosing chain's registry: the signature rule's
// Pattern-compatibility unify runs with it (fnSigSatisfiesSpecR), so a
// predicate-typed pattern child is decided as at the chain's top level.
func (f *FnUndefUnifier) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	// The signature rule decides whenever exactly one side IS this
	// shape's node; the fn-shape structural unifier handles the
	// candidate (function value, carrier, or another fn shape) exactly
	// as the ShapeFnUndef fold handled it for the body.
	aSelf := IsBareTypeNode(a) && a.Behavior() == TypeBehavior(f)
	bSelf := IsBareTypeNode(b) && b.Behavior() == TypeBehavior(f)
	if aSelf == bSelf {
		return unifySameOrSubtype(a, b)
	}
	candidate := a
	if aSelf {
		candidate = b
	}
	if candidate.Carrier || !IsConcrete(candidate) {
		// Sound over-approximation, mirroring Match's carrier arm.
		if candidate.Parent != nil && (candidate.Parent.ConformsTo(TFunction) ||
			candidate.Parent.Equal(TAny) || candidate.Parent.Equal(TFnUndef)) {
			return candidate, nil
		}
		return Value{}, unifyFail("value is not a "+f.typeName+" function", a, b)
	}
	if fnUndefMatchesFnDefR(NewValueRaw(TFnUndef, FnUndefInfo{Sigs: f.sigs}), candidate, r) {
		return candidate, nil
	}
	return Value{}, unifyFail("function does not satisfy fn shape "+f.typeName, a, b)
}

// installFnUndefUnifier attaches a FnUndefUnifier to def, wrapping any
// existing Behavior. Called by InstallType when minting a named
// fn-shape type so the signature specs drive every Is/Match call site.
func installFnUndefUnifier(def *Type, sigs []FnSigSpec, name string) {
	def.ensureTMeta().Behavior = &FnUndefUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		sigs:            sigs,
		typeName:        name,
	}
}
