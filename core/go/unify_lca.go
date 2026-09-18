package core

// ErrNoUnifier is the sentinel a Unifier returns when it holds a
// placeholder slot (e.g. a wrapped Behavior whose user-defined unifier
// body is empty). dispatchUnifier recognises it via pointer equality
// and continues the parent-chain walk, treating the Behavior as if it
// didn't satisfy the Unifier interface at all.
//
// Mirrors ErrNoComparer in compare.go — single-slot wrapper Behaviors
// carrying multiple capabilities (compare / canon / unify / …) need
// per-capability opt-out so missing slots don't terminate dispatch
// prematurely.
var ErrNoUnifier = &UnifyError{Reason: "no unifier in this Behavior"}

// dispatchUnifier walks the lattice looking for a Unifier capability
// to handle (a, b). Returns (value, err, true) if a Unifier owned the
// result, (zero, nil, false) if no Unifier applied and the caller
// should fall through to the kernel's structural dispatch.
//
// Walk strategy: start from the MORE SPECIFIC of the two denoted types
// (the one that's a subtype of the other), then walk parent-ward. This
// differs from Comparer's LCA walk because unification's "narrowing
// into a constrained type" case asks the constraint (the more specific
// type) to validate — e.g. `Unify(Pos-literal, integer-5)` must
// trigger Pos's predicate Unifier even though the LCA is Integer.
//
// When neither type is a subtype of the other, fall back to the LCA
// walk (same as Comparer). Sibling DEPSCALAR refinements meet
// implicitly — `Unify((Integer gt 10), (Integer lt 20))` produces the
// interval intersection via unifyDepScalar/combineDepScalars, named or
// inline (pinned in user-types.tsv). The residual limitation is scoped
// to OPAQUE PREDICATE bodies (fn-shaped refinements): their
// intersection cannot be computed, so combining them stays explicit —
// `(Pos tand Even)` — by design.
//
// Bare type literals participate via denotedType so unifying a refined
// type's type literal with a value still triggers the type's Unifier.
// Carriers expose their declared type via Parent — the same walk
// applies.
//
// r is the enclosing chain's registry (nil when unarmed), handed to
// the Unifier so a rule that re-enters the unifier (a binding body, a
// disjunct's alternatives) keeps the chain armed.
func dispatchUnifier(a, b Value, r *Registry) (Value, *UnifyError, bool) {
	aType := denotedType(a)
	bType := denotedType(b)
	if aType == nil || bType == nil {
		return Value{}, nil, false
	}

	var starts []*Type
	switch {
	case bType.IsSubtypeOf(aType):
		starts = []*Type{bType}
	case aType.IsSubtypeOf(bType):
		starts = []*Type{aType}
	// A membership question — one side is a bare TYPE NODE, the other a
	// candidate — walks from the NODE's denotation even when the two
	// sit in unrelated lattice branches: `Unify(fn-value, Mapper-node)`
	// pairs a Function-branch value with a FunctionSignature-branch
	// node, whose LCA (Type) knows no membership rule, yet Mapper's
	// FnUndefUnifier is exactly the rule to consult.
	case IsBareTypeNode(b) && !IsBareTypeNode(a):
		starts = []*Type{bType}
	case IsBareTypeNode(a) && !IsBareTypeNode(b):
		starts = []*Type{aType}
	case IsBareTypeNode(a) && IsBareTypeNode(b):
		// TWO bare nodes: a type-level pair. Either side's kind may
		// own the rule (`OptNum unify None` — the disjunct node's
		// alternatives admit the None literal), and their LCA may not
		// even exist (None is a degenerate root), so walk from each
		// denotation in turn.
		starts = []*Type{aType, bType}
	default:
		starts = []*Type{lowestCommonAncestor(aType, bType)}
	}

	for _, start := range starts {
		for t := start; t != nil; t = t.Parent {
			u, ok := unifierOf(t.Behavior())
			if !ok {
				continue
			}
			v, err := u.Unify(a, b, r)
			if err == ErrNoUnifier {
				continue
			}
			return v, err, true
		}
	}
	return Value{}, nil, false
}

// unifierOf resolves a Unifier from a Behavior CHAIN: the behavior
// itself, or one beneath a `behave` wrapper (MatchDelegating) or a
// kernel wrapper chain (Prev). A behave compare/canon install over a
// predicate/depscalar type must not hide the kind's constraint from
// the walk.
func unifierOf(b TypeBehavior) (Unifier, bool) {
	for b != nil {
		if u, ok := b.(Unifier); ok {
			return u, true
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
	return nil, false
}
