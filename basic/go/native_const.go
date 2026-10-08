package basic

import (
	"fmt"
	"math"

	core "github.com/boru-lang/boru/core/go"
)

// The `const` word — singleton types (design/legacy/CLASS-OBJECT.10.ignore §3d).
//
//	const 1                 a value whose TYPE has one inhabitant
//	typeof (const 1)        the singleton — its name renders as `1`
//	def S class {kind:(const 'point'), x:1}
//
// `const v` mints (per registry, interned by base type + canonical
// value) a singleton lattice node under v's own type, installs a
// value-equality Behavior, and returns v TAGGED with the singleton —
// so `{x:(const 1)}` rides the typed-default schema rule unchanged:
// the field's type is the singleton, the default is the value.
// Membership is same-base strict: `1` inhabits `(const 1)`, while `2`
// and `1.0` do not (cmp's cross-leaf magnitude equivalence does not
// leak into membership). `make S {x:1}` is accepted — the plain value
// IS the inhabitant — and any other value is a loud type error at
// make and at instance `set`.
//
// Rules: the exemplar must be an immutable concrete value — scalars,
// Lists, Maps. Mutable containers (class instances, Arrays, Stores)
// are rejected (their contents drift under aliasing, so they cannot
// pin a value), and a Float NaN exemplar is rejected (NaN's non-equal-
// to-itself semantics would make the type uninhabitable).
var ConstNative = NativeFunc{
	Name: "const",

	Signatures: []Signature{{
		Args:       []*Type{TAny},
		Impl:       Go(ConstHandler, RunInCheck()),
		Returns:    []*Type{TAny},
		BarrierPos: -1,
	}},
}

func ConstHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	if !IsConcrete(v) {
		return nil, r.BoruError("const_error",
			fmt.Sprintf("const: needs a concrete value, got %s", v.String()), "const")
	}
	switch v.Data.(type) {
	case ClassInstanceInfo, ResourceInstanceInfo:
		return nil, r.BoruErrorHint("const_error",
			"const: a mutable instance cannot pin a value", "const",
			"const exemplars must be immutable — scalars, Lists, or Maps")
	case *core.StoreInstanceInfo:
		return nil, r.BoruErrorHint("const_error",
			"const: a mutable Store cannot pin a value", "const",
			"const exemplars must be immutable — scalars, Lists, or Maps")
	}
	if v.Parent.ConformsTo(TFloat) {
		if f, err := core.AsFloat(v); err == nil && math.IsNaN(f) {
			return nil, r.BoruError("const_error",
				"const: NaN cannot pin a value (NaN is not equal to itself)", "const")
		}
	}

	base := CanonicalType(r, v.Parent)
	canon := core.CanonValue(v)

	// Intern per registry via a hidden type binding, so two
	// occurrences of (const 1) in one scope share a lattice node.
	// (Bindings made inside fn bodies are scope-cleaned like any def;
	// a re-mint in another scope is semantically identical — Match is
	// by value, not node identity.)
	key := "__const:" + base.ID + ":" + canon
	if entry, ok := r.Defs.TopEntry(key); ok && entry.TypeDef != nil {
		return []Value{ReparentValue(v, entry.TypeDef)}, nil
	}

	// The singleton type's membership is "equals the exemplar within the
	// exemplar's own base type" — same-base STRICT, so the cross-leaf
	// numeric magnitude equivalence (1 cmp 1.0 → 0) does not leak into
	// membership. This is a membership rule, so it rides the shared
	// MemberBehavior wiring (Match / Unify / canon / `is` agreement)
	// rather than a hand-written Behavior — the same path every host
	// membership type uses (eng/go/membership.go). matchMembership only
	// calls the predicate on concrete values, so a non-inhabitant (a bare
	// literal, or a check-mode carrier already tagged with this node)
	// defers to the lattice walk.
	exemplar := v
	node := r.Types.MintMemberType(canon, base, func(x Value) bool {
		return x.Parent != nil && x.Parent.ConformsTo(exemplar.Parent) && DeepEqual(x, exemplar)
	})
	r.Defs.PushType(key, node, NewTypeLiteral(node))
	return []Value{ReparentValue(v, node)}, nil
}
