package core

// disjunctUnifier is the kernel-installed Unifier for user-defined
// disjunct types — types whose body is a (`tor`-joined) disjunction
// of alternatives such as `def Maybe (Integer tor none)`.
//
// Before this Unifier existed, `def Maybe (Integer tor none)` minted
// a lattice node parented at TDisjunct but left its Behavior as the
// kernel DefaultBehavior. DefaultBehavior.Match falls back to a plain
// lattice walk: it asks `v.Parent.ConformsTo(Maybe)`, which is false for
// every value (Integer's lattice path is Integer → Number → Scalar →
// Any, none of which is Maybe). So `42.Is(Maybe)` returned false at
// every dispatch site — fn sig matching, the `is` word, options
// fields, record fields — even though the language-level `unify` word
// (which goes through unifyDisjunct directly) accepted 42 happily.
//
// Like predicateUnifier, this lives on the lattice node so every
// "is v a Maybe?" path consults it.
type DisjunctUnifier struct {
	behaviorWrapper         // prev Behavior + Format/Equal/Compare delegation
	Alternatives    []Value // copy of the disjunct's alternatives
	typeName        string
}

// Match runs the disjunct alternatives against v via unifyDisjunct.
// The disjunct accepts v iff some alternative unifies with it. Type
// literals (Data==nil, !Carrier) pass through to the prev/DefaultBehavior
// walk — a bare Maybe-literal is "the type itself", not an inhabitant.
func (*DisjunctUnifier) ContentMembership() {}

func (d *DisjunctUnifier) Match(v Value, t *Type) bool {
	// Deliberately LOOSE on the newtype-alternative swap: `42` against
	// `(P tor String)` admits via the subtype rule even though
	// `42 is M` is false — dispatch decomposes the union to base
	// families, the one pinned boundary where a newtype alternative is
	// looser than `is` (edge-types-2's DIVERGENCE PIN). The `is` word
	// applies its post-unify value-identity check on top of this Match
	// (IsDisjunctTypeNode routes it past the short-circuit).
	return d.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the alternatives walk (see isR).
func (d *DisjunctUnifier) matchR(v Value, t *Type, r *Registry) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(d.prev).Match(v, t)
	}
	_, err := unifyDisjunct(DisjunctInfo{Alternatives: d.Alternatives}, v, r)
	return err == nil
}

// IsDisjunctTypeNode reports whether v is a bare lattice node whose
// Behavior chain carries a DisjunctUnifier — the evaluated NAME of a
// disjunct type after the Stage 2 flip. The `is` handler uses it to
// route a concrete candidate through the full Unify + value-identity
// path (where the newtype-alternative swap is declined) instead of the
// Match short-circuit (which stays deliberately loose for dispatch).
func IsDisjunctTypeNode(v Value) bool {
	if !IsBareTypeNode(v) {
		return false
	}
	u, ok := unifierOf(v.Behavior())
	if !ok {
		return false
	}
	_, ok = u.(*DisjunctUnifier)
	return ok
}

// Unify admits a concrete candidate iff some alternative unifies with
// it, yielding the alternative-unified result — never the node literal
// (the typed-def swap hazard). A concrete non-member fails
// definitively; a type-level pair defers to the structural rule. The
// Unify capability every membership kind carries
// (design/legacy/TYPE-REPRESENTATION.1.ignore §N3), so `def x:Maybe 5` against the
// node constraint runs the alternatives.
func (d *DisjunctUnifier) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	// The alternatives rule decides whenever exactly one side IS this
	// disjunct's node; the candidate may be concrete, a carrier, or a
	// TYPE-level operand (`OptNum unify None` — None is an
	// alternative), all of which the payload fold decided identically
	// for the body the node replaced. A same-node (or node-free) pair
	// settles structurally.
	aSelf := IsBareTypeNode(a) && a.Behavior() == TypeBehavior(d)
	bSelf := IsBareTypeNode(b) && b.Behavior() == TypeBehavior(d)
	if aSelf == bSelf {
		return unifySameOrSubtype(a, b)
	}
	candidate := a
	if aSelf {
		candidate = b
	}
	return unifyDisjunct(DisjunctInfo{Alternatives: d.Alternatives}, candidate, r)
}

// installDisjunctUnifier attaches a disjunctUnifier to def, wrapping
// any existing Behavior. Called by InstallType when minting a disjunct
// type so the alternatives drive every Is/Match call site.
func installDisjunctUnifier(def *Type, alternatives []Value, name string) {
	def.ensureTMeta().Behavior = &DisjunctUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		Alternatives:    alternatives,
		typeName:        name,
	}
}

// unifyDisjunct tries to unify a value against each alternative in a
// disjunct. Returns the first successful unification. For map
// alternatives, uses open (subset) matching where the candidate only
// needs to contain the alternative's key-value pairs.
//
// Asymmetric by design: disj is always the disjunct side, val is the
// other side. The top dispatcher in unify.go handles the swap. r is the
// enclosing chain's registry, threaded into every alternative's walk.
func unifyDisjunct(disj DisjunctInfo, val Value, r *Registry) (Value, *UnifyError) {
	// "any" unifies with the whole disjunct, preserving it. Covers
	// two value shapes: the bare type literal NewTypeLiteral(TAny)
	// (Data=nil; the value IS the TAny lattice node) and the Any-
	// typed carrier (Data=nil, Carrier=true, Parent=TAny).
	if !IsConcrete(val) && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), nil
	}

	for _, alt := range disj.Alternatives {
		// Concrete map alternative against a concrete map value uses
		// open (subset) matching — the disjunct alternative acts as a
		// pattern, not a full schema.
		if alt.Parent.Equal(TMap) && val.Parent.Equal(TMap) &&
			!IsRecordType(alt) && !IsRecordType(val) &&
			!IsTypedMap(alt) && !IsTypedMap(val) &&
			!IsOptionsType(alt) && !IsOptionsType(val) {
			if alt.Data != nil && val.Data != nil {
				if openUnifyMap(alt, val, r) {
					return val, nil
				}
				continue
			}
		}
		if unified, err := unifyInner(alt, val, r); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}
