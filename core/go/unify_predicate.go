package core

import "fmt"

// predicateUnifier is the kernel-installed Unifier for predicate types
// — types whose body is a `fn [[x:BaseType] [Boolean] [body]]`.
// When the LCA walk in dispatchUnifier reaches the predicate type's
// *Type, this Unifier runs the predicate body against the structural
// candidate and admits the result only when the predicate accepts.
//
// Why this exists: before Phase 4, `Unify(Pos-literal, integer-5)`
// fell through to the lattice subtype rule and returned 5 wrapped as
// Pos without ever checking the predicate. The Reparent-at-typed-bind
// path in lang's `def` handler patched the common case (`def x:Pos
// 5`), but every other Unify call site (signature matching, options
// fields, record fields, the `unify` word, `make` constraints) bypassed
// the check. With a Unifier on the predicate type's lattice node, every
// path through Unify consults it.
//
// Holds a *Registry because RunPredicate needs to invoke the body
// through CallBoru — a registry-rooted operation. One Unifier per
// (predicate type, registry); fresh registries get fresh Unifiers
// installed by InstallType at type-declaration time.
type PredicateUnifier struct {
	behaviorWrapper // prev Behavior + Format/Equal/Compare delegation
	registry        *Registry
	constraint      Value // the predicate fn body
	typeName        string
}

// Match runs the predicate against v. The result of `v.Is(Pos)` —
// reached via sigTypeMatches during signature dispatch, by the `is`
// word, and by anything else that asks "is v a Pos?" — must reflect
// the predicate's verdict, not just the lattice walk. Without this
// override, predicate types would only accept values previously
// reparented via typed-def; raw `f 5` for `def f fn [[x:Pos] …]`
// would degrade to a lattice subtype check that always fails because
// Integer (5's parent) is not a descendant of Pos.
//
// Two gates:
//  1. v's declared type must satisfy the predicate's input type.
//     `"hello".Is(Pos)` rejects at this gate — String is not Integer.
//  2. The predicate body returns truthy (RunPredicate's "matched").
//
// Bare type literals (no Data) and carriers (CheckMode abstract
// values) pass through: a type literal is "the type itself", not an
// inhabitant, and carriers are placeholder values whose concreteness
// is asserted at runtime by some other path.
func (*PredicateUnifier) ContentMembership() {}

func (p *PredicateUnifier) Match(v Value, t *Type) bool {
	return matchMembership(v, t, p.prev, func(v Value) bool {
		if p.registry == nil {
			// No registry attached — fall back to the lattice walk so
			// behaviour is no worse than before predicateUnifier existed.
			return baseBehavior(p.prev).Match(v, t)
		}
		// Gate 1: input-type compatibility (`"hello".Is(Pos)` rejects —
		// String is not Integer).
		if inputT := PredicateInputType(p.constraint); inputT != nil {
			if !v.Parent.ConformsTo(inputT) {
				return false
			}
		}
		// Gate 2: run the predicate body.
		_, matched, err := p.registry.RunPredicate(p.constraint, v)
		return err == nil && matched
	})
}

// Unify runs the predicate body against a concrete operand via the shared
// membership contract: admit the candidate when the body accepts it, fail
// definitively when a concrete operand is present and rejected, and defer
// a type-level pair to the structural rule. (This replaced an earlier
// unifySameOrSubtype-first candidate step whose "narrower literal →
// admit" branch could admit a non-member without ever running the body
// — the same hole the Go path avoids; the two now share one rule.)
// The body runs under the Unifier's OWN registry (the one InstallType
// attached), not the chain's — one Unifier per (predicate type,
// registry) — so the threaded registry is not consulted here, and the
// body's own dispatch starts unarmed like top-level code (see
// UnifyExplainR: the chain ends where the engine begins).
func (p *PredicateUnifier) Unify(a, b Value, _ *Registry) (Value, *UnifyError) {
	return unifyMembership(a, b, "predicate "+p.typeName, func(v Value) (Value, bool, error) {
		if p.registry == nil {
			return Value{}, false, fmt.Errorf("predicate type %s has no registry attached", p.typeName)
		}
		return p.registry.RunPredicate(p.constraint, v)
	})
}

// IsPredicateTypeNode reports whether v is a bare lattice node whose
// Behavior is a PredicateUnifier — the evaluated NAME of a predicate
// type after the Stage 2 flip. The typed-def handler uses it to route
// node-valued predicate constraints through the same run-then-reparent
// path an inline fn constraint takes.
func IsPredicateTypeNode(v Value) bool {
	if !IsBareTypeNode(v) {
		return false
	}
	for b := v.Behavior(); b != nil; {
		if _, ok := b.(*PredicateUnifier); ok {
			return true
		}
		if d, ok := b.(MatchDelegating); ok {
			b = d.DelegatesMatchTo()
			continue
		}
		if p, ok := PrevBehavior(b); ok {
			b = p
			continue
		}
		break
	}
	return false
}

// installPredicateUnifier attaches a predicateUnifier to def, wrapping
// any existing Behavior. Called by InstallType when minting a predicate
// type so the constraint runs at every Unify call site.
func installPredicateUnifier(def *Type, constraint Value, r *Registry, name string) {
	def.ensureTMeta().Behavior = &PredicateUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		registry:        r,
		constraint:      constraint,
		typeName:        name,
	}
}
