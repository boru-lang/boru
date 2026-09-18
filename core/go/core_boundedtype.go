package core

import "fmt"

// Bounded Type — the `Type of [B]` body: the type of TYPE LITERALS
// conforming to B. This is what the `/t` word suffix and the angle
// form `Type<B>` desugar to (`Map/t` ≡ `Type<Map>` ≡ `(Type of [Map])`).
//
// Shape: Parent = TType, Data = ChildTypeInfo{Child: B} — exactly
// parallel to a typed list (Parent TList + ChildTypeInfo). Being a
// BODY VALUE (not a minted lattice node) gives structural identity
// for free: two applications of `Type of [Map]` compare equal via
// the canonical Child pointer, with no cache.
//
// Matching: a bounded Type appears in fn signatures as a param
// PATTERN (Type: TType, Pattern: &body — see ResolveSigType), so
// both dispatch paths enforce it through Unify; the Unify case lives
// in unify.go. Membership: the value must be a TYPE (TypeMembership,
// mirroring the `is Type` rule) AND denote a single lattice node that
// ConformsTo the bound — so bare literals and named types (including
// named disjuncts used as bounds) participate, while structural
// BODIES (an inline `(Map tor List)` value) satisfy bare `Type` but
// no bound (they denote no single node).

// NewBoundedType constructs the `Type of [bound]` body value for a
// NOMINAL bound (builtin / refine): the bound rides as its type
// literal (ChildTypeInfo.Child is a Value, the typed-list convention).
func NewBoundedType(bound *Type) Value {
	return NewValueRaw(TType, ChildTypeInfo{Child: NewTypeLiteral(bound)})
}

// NewBoundedTypeBody constructs the bounded Type for a BEHAVIORAL
// bound — a named body type (disjunct, predicate): the body rides as
// the Child, reparented to the minted node so the name survives for
// rendering and identity while satisfaction unifies against the body
// (the disjunct fold admits each alternative's literals).
func NewBoundedTypeBody(node *Type, body Value) Value {
	return NewValueRaw(TType, ChildTypeInfo{Child: ReparentValue(body, node)})
}

// boundedBoundNode returns the lattice node a bounded Type's Child
// denotes: a bare literal denotes itself; a reparented body denotes
// its Parent (the minted node).
func boundedBoundNode(child Value) *Type {
	if n := DenotedTypeNode(child); n != nil {
		return n
	}
	return child.Parent
}

// IsBoundedType reports whether v is a `Type of [B]` body.
func IsBoundedType(v Value) bool {
	if v.Parent == nil || !v.Parent.Equal(TType) {
		return false
	}
	_, ok := v.Data.(ChildTypeInfo)
	return ok
}

// AsBoundedType returns the bound B of a `Type of [B]` body as a
// lattice node.
func AsBoundedType(v Value) (*Type, error) {
	info, ok := v.Data.(ChildTypeInfo)
	if !ok || v.Parent == nil || !v.Parent.Equal(TType) {
		return nil, fmt.Errorf("AsBoundedType: not a bounded Type (got %T)", v.Data)
	}
	node := boundedBoundNode(info.Child)
	if node == nil {
		return nil, fmt.Errorf("AsBoundedType: bound does not denote a lattice node")
	}
	return node, nil
}

// TypeMembership reports whether v is a TYPE — the membership rule of
// `v is Type`, shared verbatim with isHandler (lang/native_type.go):
// a bare type literal, a structural type body (record shape, typed
// list/map, disjunct, fn-shape, bounded Type), or a Type-branch value
// (Function, Disjunct, Enum). Carriers are abstract VALUES; concrete
// scalars/lists/maps and `none` (whose sentinel payload makes
// IsBareTypeNode false) are not types.
func TypeMembership(v Value) bool {
	if v.Carrier {
		return false
	}
	return IsBareTypeNode(v) || IsTypeBody(v) || IsRecordShape(v) ||
		(v.Parent != nil && v.Parent.ConformsTo(TType))
}

// DenotedTypeNode returns the single lattice node a type VALUE
// denotes, when it denotes one: a bare type literal denotes &v (the
// by-value copy carries the full parent chain, which is all
// ConformsTo needs). Structural bodies denote no single node and
// return nil — they satisfy bare `Type` but no bound.
func DenotedTypeNode(v Value) *Type {
	if !IsBareTypeNode(v) {
		return nil
	}
	node := v
	return &node
}

// boundedTypeSatisfied reports whether the type value v satisfies the
// bound Child: v is a TYPE and either (nominal bounds) denotes a node
// within the bound node's ancestry / matches its Behavior, or
// (behavioral bounds — a named disjunct or predicate carried as a
// reparented body) unifies against the body, whose disjunct fold
// admits each alternative's literals. Together these mirror how the
// bound answers `is` for ordinary values. r is the enclosing unify
// chain's registry: both the membership walk and the body unify are
// made from inside a unify, so they keep the chain armed.
func boundedTypeSatisfied(v Value, child Value, r *Registry) bool {
	if !TypeMembership(v) {
		return false
	}
	bound := boundedBoundNode(child)
	if bound != nil {
		if node := DenotedTypeNode(v); node != nil && node.ConformsTo(bound) {
			return true
		}
		if isR(v, bound, r) {
			return true
		}
	}
	if !IsBareTypeNode(child) {
		if _, uerr := unifyWithin(v, child, r); uerr == nil {
			return true
		}
	}
	return false
}

// unifyBoundedType handles Unify when either side is a `Type of [B]`
// body. A type value vs the bound: the value wins when its denoted
// node conforms. Bounded vs bounded: the narrower bound wins. Anything
// else fails with a structured error. r is the enclosing chain's
// registry, threaded into the bound check.
func unifyBoundedType(a, b Value, r *Registry) (Value, *UnifyError) {
	ac, aok := boundedChild(a)
	bc, bok := boundedChild(b)
	switch {
	case aok && bok:
		an, bn := boundedBoundNode(ac), boundedBoundNode(bc)
		if an != nil && bn != nil {
			if an.ConformsTo(bn) {
				return a, nil
			}
			if bn.ConformsTo(an) {
				return b, nil
			}
		}
	case aok:
		if boundedTypeSatisfied(b, ac, r) {
			return b, nil
		}
	case bok:
		if boundedTypeSatisfied(a, bc, r) {
			return a, nil
		}
	}
	return Value{}, &UnifyError{Reason: "value is not a type within the bound", A: a, B: b}
}

// boundedChild returns the Child of a bounded Type body.
func boundedChild(v Value) (Value, bool) {
	if !IsBoundedType(v) {
		return Value{}, false
	}
	info, ok := v.Data.(ChildTypeInfo)
	if !ok { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
		return Value{}, false
	}
	return info.Child, true
}

// typeMembershipBehavior is TType's Behavior: Match answers the ONE
// membership question (`v is Type`, `t:Type` params, `[Type]` return
// checks) with TypeMembership, keeping the three boundaries symmetric
// per the refine doctrine. The answer set strictly CONTAINS the
// default Parent-conformance set (Type-branch values still match), so
// installing it only widens acceptance — to the literals and bodies
// that `is Type` already admitted. Format and Equal delegate to the
// kernel defaults.
type typeMembershipBehavior struct{}

func (typeMembershipBehavior) ContentMembership() {}

func (typeMembershipBehavior) Match(v Value, t *Type) bool {
	if t != nil && t.Equal(TType) {
		return TypeMembership(v)
	}
	return DefaultBehavior.Match(v, t)
}

func (typeMembershipBehavior) Format(v Value) string { return DefaultBehavior.Format(v) }

func (typeMembershipBehavior) Equal(a, b Value) bool { return DefaultBehavior.Equal(a, b) }

// formatDelegate marks the Behavior as rendering-neutral so
// Value.String / CanonValue skip it and fall through to the kernel
// default (see the formatDelegatesToDefault marker in value.go).
func (typeMembershipBehavior) formatDelegate() {}
