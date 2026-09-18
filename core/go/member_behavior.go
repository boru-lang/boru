package core

// MemberUnifier is a TypeBehavior built from a single Go membership
// predicate. It is the host-Go counterpart of the wiring the boru
// `def`/`refine`/`tor` path installs automatically: a host that can say
// "does value v belong to this type?" gets every kernel touch-point for
// free, instead of hand-implementing (and keeping consistent) Match,
// Unify, Format and the canon format-delegate marker.
//
//   - Match — the predicate. Dispatch (sigTypeMatches), the `is` word's
//     fast path, options/record fields: all ask Match.
//   - Unify — the SAME predicate, admitting the concrete side when it
//     belongs. The `is` word, `case`, fn return checks and typed
//     containers ask Unify, not Match; deriving one from the other is
//     what keeps the two answers from drifting (the gap that made the
//     first hand-written host enum so fiddly).
//   - Format / FormatDelegate — render by the kernel default (the value's
//     lattice family), so a member renders as itself and canon keeps its
//     family form (e.g. an Atom subtype's `name/q`).
//   - Equal — kernel default.
type MemberUnifier struct {
	member func(v Value) bool
}

// MemberBehavior builds a TypeBehavior whose inhabitants are exactly the
// concrete values satisfying member. Use it for host (Go) types that are
// defined by a membership rule — a closed set of values, a tag check, a
// range — so the type participates in `is`, signature dispatch, `case`,
// and return checks identically to a boru-defined refinement, from one
// predicate. Pair it with TypeTable.MintMemberType (or pass it to
// MintTypeWithBehavior / RegisterType) to attach it to a node.
func MemberBehavior(member func(v Value) bool) TypeBehavior {
	return MemberUnifier{member: member}
}

// Match reports membership via the shared contract: a non-inhabitant
// defers to the lattice walk; a concrete value is put to the predicate.
func (MemberUnifier) ContentMembership() {}

func (b MemberUnifier) Match(v Value, t *Type) bool {
	return matchMembership(v, t, nil, b.member)
}

// Unify admits a concrete member, in either argument order, via the
// shared contract — a non-member with a concrete operand fails
// definitively (no structural re-admission), a type-level pair defers to
// the structural rule. The Go predicate yields the candidate unchanged on
// a match.
func (b MemberUnifier) Unify(a, c Value, _ *Registry) (Value, *UnifyError) {
	return unifyMembership(a, c, "the member type", func(v Value) (Value, bool, error) {
		return v, b.member(v), nil
	})
}

func (b MemberUnifier) Equal(a, c Value) bool           { return baseBehavior(nil).Equal(a, c) }
func (b MemberUnifier) Format(v Value) string           { return baseBehavior(nil).Format(v) }
func (b MemberUnifier) Compare(a, c Value) (int, error) { return baseCompare(nil, a, c) }

// FormatDelegate marks the behavior as delegating Format to the kernel
// default, so canon / Value.String render a member by its lattice family
// rather than routing through Format (which would drop family-specific
// source forms such as an Atom subtype's `/q` suffix).
func (b MemberUnifier) FormatDelegate() {}

// MintMemberType mints `name` as a subtype of parent whose inhabitants
// are exactly the concrete values that conform to parent AND satisfy
// member, with the full Match/Unify/Format wiring (MemberBehavior)
// attached. It is the host-Go equivalent of a boru refinement
// (`def Name (refine Parent …)`): one call yields a node that behaves
// like a first-class type across dispatch, `is`, `case` and return
// checks, and the returned *Type is ready to drop into native signatures
// and to tag values with.
//
// The declared parent is enforced as a gate around `member` — the same
// guarantee the boru predicate path gives via its input-type check — so a
// permissive or partial predicate cannot admit values outside the
// family (a `TInteger` member type never accepts a String, however lax
// the predicate). A value tagged with this node passes the gate via
// ancestry.
//
// Like every minted type the node carries no FixedID and is absent from
// the builtin name index and the FixedID snapshot — it is owned by
// whoever minted it (a module sub-registry, a host plugin), not a kernel
// builtin.
func (tt *TypeTable) MintMemberType(name string, parent *Type, member func(v Value) bool) *Type {
	gated := member
	if parent != nil {
		gated = func(v Value) bool {
			return v.Parent != nil && v.Parent.ConformsTo(parent) && member(v)
		}
	}
	return tt.MintTypeWithBehavior(name, parent, MemberBehavior(gated))
}
