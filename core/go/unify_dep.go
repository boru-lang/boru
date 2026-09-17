package core

// depScalarUnifier is the kernel-installed Unifier for user-defined
// refined-scalar types — types whose body is a DepScalar with a
// comparison constraint, e.g. `def Big (Integer gt 10)`.
//
// Before this Unifier existed, InstallType's generic "else" branch
// minted a lattice node parented at the base scalar (Integer) but
// left Behavior as DefaultBehavior. That meant `100.Is(Big)` did a
// plain lattice walk — 100's parent chain is Integer → Number → …,
// none of which is Big — so dispatch rejected every value, even ones
// that satisfied the predicate.
//
// The Unifier:
//   - Gate 1: v's type must satisfy the DepScalar's base type
//     (`Big` constrains Integers — 100 passes, "hi" fails).
//   - Gate 2: v's payload must satisfy the constraint
//     (depScalarCheck handles the bounds check).
//
// Bare type literals (Data==nil, !Carrier) pass through to the
// prev/DefaultBehavior walk — the type itself isn't an inhabitant.
type DepScalarUnifier struct {
	behaviorWrapper
	baseType *Type
	depInfo  DepScalarInfo
	typeName string
}

func (*DepScalarUnifier) ContentMembership() {}

func (d *DepScalarUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(d.prev).Match(v, t)
	}
	// An abstract CARRIER already tagged as t (the predicate type) or a subtype
	// satisfies t nominally: the tag is the checker's record that the value was
	// produced under t's contract (a `Big`-returning fn, a `Big` param/def), so
	// the predicate is already guaranteed — and a value-level predicate cannot
	// be re-verified on an abstract carrier anyway, so depScalarCheck below
	// would conservatively (and wrongly) reject it. `def mk fn [[] [Big] [50]]
	// use (mk)` failed exactly here. A CONCRETE value (Data present) still runs
	// the predicate check, so a plain `5` is correctly refused for `Big`.
	if v.Carrier && !IsConcrete(v) && v.Parent != nil && t != nil && v.Parent.ConformsTo(t) {
		return true
	}
	if !v.Parent.ConformsTo(d.baseType) {
		return false
	}
	return depScalarCheck(d.depInfo, v)
}

// Unify runs the bounds check against a concrete operand via the shared
// membership contract: gate 1 (base-type conformance) then gate 2
// (depScalarCheck), admitting the candidate itself — never the node
// literal — and failing DEFINITIVELY on a concrete non-member so the
// structural fallback cannot re-admit it by lattice subtyping alone.
// This is the Unify capability PredicateUnifier already carries; without
// it a typed bind against the bare minted node (`def x:Big 5` once
// evaluation yields nodes — design/legacy/TYPE-REPRESENTATION.1.ignore §N3) would
// fall to unifySameOrSubtype's narrower-literal arm and bind without
// ever running the constraint.
func (d *DepScalarUnifier) Unify(a, b Value) (Value, *UnifyError) {
	// TWO refinement sides — the other operand is itself DepScalar
	// content (an inline body, or a sibling NAME's node recording one):
	// the pair meets to the interval intersection, exactly as the
	// payload fold combined the two bodies (`A unify B` →
	// `(Integer gt 10 lt 20)`, pinned in user-types.tsv; the generic
	// bound check `(T extends Pos)` over an inline refinement rides the
	// same rule).
	if ad, ok := depScalarContent(a); ok {
		if bd, ok := depScalarContent(b); ok {
			return unifyDepScalar(ad, ShapeDepScalar, bd, ShapeDepScalar)
		}
	}
	out, uerr := unifyMembership(a, b, d.typeName, func(v Value) (Value, bool, error) {
		if !v.Parent.ConformsTo(d.baseType) {
			return Value{}, false, nil
		}
		return v, depScalarCheck(d.depInfo, v), nil
	})
	if uerr != nil && uerr != ErrNoUnifier {
		// Keep the fold's established failure text (unifyDepScalar) so
		// the named-node path reports byte-identically to the inline
		// DepScalar body it replaced.
		return Value{}, unifyFail("value does not satisfy DepScalar bounds", a, b)
	}
	return out, uerr
}

// depScalarContent returns the DepScalar VALUE a unify operand stands
// for: the operand itself when it carries the payload, or a bare
// node's recorded refinement body (the named spelling after the Stage
// 2 flip — design/legacy/TYPE-REPRESENTATION.1.ignore §N2).
func depScalarContent(v Value) (Value, bool) {
	if v.IsDepScalar() {
		return v, true
	}
	if IsBareTypeNode(v) {
		if body, ok := v.TypeBody(); ok && body.IsDepScalar() {
			return body, true
		}
	}
	return Value{}, false
}

// installDepScalarUnifier attaches a depScalarUnifier to def. Called
// by InstallType when minting a DepScalar-bodied user type so the
// constraint runs at every Is/Match call site (sig dispatch, the `is`
// word, options/record/Make slot checks).
func installDepScalarUnifier(def *Type, base *Type, info DepScalarInfo, name string) {
	def.ensureTMeta().Behavior = &DepScalarUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		baseType:        base,
		depInfo:         info,
		typeName:        name,
	}
}

// unifyDepScalar handles unification when at least one side is a
// DepScalar (refined scalar with bounds, e.g. `Integer ≥10`).
//
// Three cases:
//  1. DepScalar vs concrete scalar: succeeds iff the scalar's type
//     matches the base AND the value satisfies the comparison. Returns
//     the plain scalar (not the DepScalar) so downstream consumers see
//     a normal value.
//  2. DepScalar vs DepScalar over the same base: combine the
//     constraints (intersection) — same-side bounds tighten,
//     opposite-side bounds form an interval. Empty result fails.
//  3. DepScalar vs DepScalar over different bases: fails (incompatible
//     bases).
func unifyDepScalar(a Value, sa ValueShape, b Value, sb ValueShape) (Value, *UnifyError) {
	// Both DepScalar: intersect constraints.
	if sa == ShapeDepScalar && sb == ShapeDepScalar {
		aType := denotedType(a)
		bType := denotedType(b)
		if !aType.Equal(bType) {
			return Value{}, unifyFail("DepScalar bases do not match", a, b)
		}
		aInfo, err := a.AsDepScalar()
		if err != nil {
			return Value{}, unifyFail("could not read DepScalar payload on left", a, b)
		}
		bInfo, err := b.AsDepScalar()
		if err != nil {
			return Value{}, unifyFail("could not read DepScalar payload on right", a, b)
		}
		combined, ok := combineDepScalars(aInfo, bInfo)
		if !ok {
			return Value{}, unifyFail("DepScalar constraint intersection is empty", a, b)
		}
		return NewValueRaw(aType, combined), nil
	}

	// Canonicalize: dep on the left, other on the right.
	var dep, other Value
	if sa == ShapeDepScalar {
		dep, other = a, b
	} else {
		dep, other = b, a
	}

	// DepScalar vs type literal / carrier: not a DepScalar concern —
	// fall through to general subtype narrowing. The DepScalar's
	// Parent is its base scalar, so the subtype walk handles e.g.
	// `Pos refine Integer` (Pos sub Integer) by returning the
	// narrower side.
	if !IsConcrete(other) {
		return unifySameOrSubtype(dep, other)
	}

	// DepScalar vs concrete scalar over the same base.
	depType := denotedType(dep)
	otherType := denotedType(other)
	if !otherType.ConformsTo(depType) {
		return Value{}, unifyFail("DepScalar base type does not match value's type", a, b)
	}
	info, err := dep.AsDepScalar()
	if err != nil {
		return Value{}, unifyFail("could not read DepScalar payload", a, b)
	}
	if depScalarCheck(info, other) {
		return other, nil
	}
	return Value{}, unifyFail("value does not satisfy DepScalar bounds", a, b)
}
