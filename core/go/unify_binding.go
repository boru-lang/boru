package core

// BindingBodyUnifier is the membership Behavior InstallType attaches to
// every catch-all type binding whose body is structural or a literal
// singleton — the record shape (`def R {x:Integer}`), the singleton
// (`def One 1`), typed-container literals (`def L [:Integer]`), and any
// other body the earlier per-kind branches do not claim. Membership is
// decided by unifying the candidate against the BODY, so `is` (which
// unifies against the evaluated body) and dispatch (which walks the
// lattice to the minted node) consult one structural rule — the same
// bridge DisjunctUnifier / FnUndefUnifier provide for their kinds.
// Without it the minted node keeps DefaultBehavior, nothing is ever
// tagged with it, and dispatch rejects every value while `is` accepts
// (NUR090's record-shape row; NUR093's singleton row).
type BindingBodyUnifier struct {
	behaviorWrapper
	body     Value
	typeName string
}

// ContentMembership marks the Behavior as content-deciding: membership
// comes from the body's structure, not from a nominal tag, so the type
// cannot anchor word extensions (OPEN-WORDS §3.1).
func (*BindingBodyUnifier) ContentMembership() {}

// Body returns the structural body this binding was declared with — the
// node-side recovery of the declaration's structure (the seam Stage 1 of
// design/legacy/TYPE-REPRESENTATION.1.ignore generalizes per kind).
func (u *BindingBodyUnifier) Body() Value { return u.body }

func (u *BindingBodyUnifier) Match(v Value, t *Type) bool {
	return u.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the body unify (see isR): the candidate-vs-body walk is a unify
// made from inside a unify, so it keeps the chain's registry rather
// than starting an unarmed one.
func (u *BindingBodyUnifier) matchR(v Value, t *Type, r *Registry) bool {
	if v.Carrier {
		// Sound check-mode over-approximation, the FnUndefUnifier
		// discipline: admit a carrier that could still be a member at
		// runtime — dynamic, tagged at/below the minted node, or in the
		// body's own family — and reject one that provably cannot be.
		if v.Dynamic {
			return true
		}
		if v.Parent != nil && t != nil && v.Parent.ConformsTo(t) {
			return true
		}
		return v.Parent != nil && u.body.Parent != nil && v.Parent.ConformsTo(u.body.Parent)
	}
	return matchMembership(v, t, u.prev, func(v Value) bool {
		_, uerr := unifyWithin(v, u.body, r)
		return uerr == nil
	})
}

// Unify admits a concrete candidate by unifying it against the body,
// yielding the UNIFIED CANDIDATE — never the node literal, so a typed
// def over the node binds the value rather than the type (the swap
// hazard unifySameOrSubtype's narrower-literal arm would introduce).
// A rejected concrete candidate fails definitively per the shared
// membership contract, so the structural fallback cannot re-admit a
// non-member by lattice subtyping alone. The body unify is a unify
// made from inside a unify (dispatchUnifier reached this node from
// unifyInner), so it runs through unifyWithin with the chain's
// registry: a predicate-typed field or a `[:Foo]` child inside the
// body is decided exactly as at the chain's top level.
func (u *BindingBodyUnifier) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	return unifyMembership(a, b, "type "+u.typeName, func(v Value) (Value, bool, error) {
		out, uerr := unifyWithin(v, u.body, r)
		if uerr != nil {
			return Value{}, false, nil
		}
		return out, true, nil
	})
}

// installBindingBodyUnifier attaches a BindingBodyUnifier to def,
// wrapping any existing Behavior. Called by InstallType's catch-all
// branch so every named structural/singleton binding carries a
// content-deciding membership rule on its minted node.
func installBindingBodyUnifier(def *Type, body Value, name string) {
	def.ensureTMeta().Behavior = &BindingBodyUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		body:            body,
		typeName:        name,
	}
}
