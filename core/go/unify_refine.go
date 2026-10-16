package core

// bareRefineUnifier is the kernel-installed Behavior for bare nominal
// subtypes — types whose body has no added structure beyond a base
// type, e.g. `def Pos refine Integer`. Pos is a fresh lattice node
// parented at Integer; its only difference from Integer is identity.
//
// Semantics: a bare refine is a NOMINAL NEWTYPE. A value is a member
// only when its own tag IS the refine type (or a subtype of it) — a
// plain base-typed value is NOT an inhabitant. So `42.Is(Pos)` is
// false; a `Pos` is obtained by explicit construction (`def x:Pos 42`,
// which reparents via Unify — a separate path). This keeps every
// boundary that asks `v.Is(Pos)` — sig dispatch, the `is` word, and
// the fn return check — in agreement, and matches the newtype
// discipline of Haskell/Rust/Go/Scala. See
// design/REFINE-NEWTYPE-VS-SUBSET.10.md.
//
// (An earlier revision made Match lenient — admitting any base-family
// value — which split param matching from the nominal `is`/return
// check; that asymmetry is exactly what this Behavior now removes.)
//
// Type literals (Data==nil, !Carrier) pass through to the
// prev/DefaultBehavior walk — the type itself isn't an inhabitant.
type bareRefineUnifier struct {
	behaviorWrapper
	typeName string
}

// formatDelegate: a bare-refine newtype adds IDENTITY only, so its
// values render by their payload family (kernelFormatDefault) — a
// retagged flex map prints as a map, not as an opaque wrapper. Without
// the marker the behaviorWrapper hop landed flex payloads outside the
// family switch and leaked a pointer render (`S({0xc…})`).
func (b *bareRefineUnifier) formatDelegate() {}

func (b *bareRefineUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(&v) {
		return baseBehavior(b.prev).Match(v, t)
	}
	// Nominal: the value's tag must be the refine type itself (or a
	// subtype), NOT merely the base type.
	return ParentOf(&v).ConformsTo(t)
}

// installBareRefineUnifier attaches a bareRefineUnifier to def. Called
// by InstallType when minting/renaming a bare-refinement prefab so the
// nominal newtype rule governs every v.Is(def) boundary.
func installBareRefineUnifier(def *Type, name string) {
	def.ensureTMeta().Behavior = &bareRefineUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		typeName:        name,
	}
}
