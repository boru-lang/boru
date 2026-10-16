package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// unifyNatives covers the `unify` word — the surface-level entry
// point to the engine's structural unification algorithm.
//
//	a b unify     → [unified, true]  on success
//	a b unify     → ["~unify-fail", false]  on failure
//
// The algorithm (Unify and friends) lives in eng/go/unify.go; this
// file owns the word name, dispatch wiring, and the Go-level adapter
// (unifyHandler) that converts an core.Unify result into the boru
// stack shape.
var unifyNatives = []NativeFunc{
	{
		Name: "unify",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(unifyHandler),
			Returns:   []*Type{TAny, TBoolean},
			ReturnsFn: unifyFoldReturns, BarrierPos: -1,
		}},
	},
}

// unifyFoldReturns folds `unify` at check time when BOTH operands are
// statically-known INERT values — a bare type node, or concrete scalar /
// list / map data (recursively; type literals allowed inside containers,
// exactly the pattern-map shape unify consumes). UnifyR is a pure
// function of such operands, so the checked result IS the runtime result:
// `unify 2 3` types as (String, Boolean) — the "~unify-fail" arm — and
// `Node unify {a:1}` as (Map, Boolean). Identity-bearing operands (flex
// stores, class instances) and carriers fall back to the declared shape.
func unifyFoldReturns(args []Value, r *Registry) []Value {
	fallback := []Value{NewDynamicCarrier(TAny), NewCarrier(TBoolean)}
	if len(args) != 2 || !unifyFoldable(args[0]) || !unifyFoldable(args[1]) {
		return fallback
	}
	out, err := unifyHandler(args, nil, nil, r)
	if err != nil || len(out) != 2 { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return fallback
	}
	// A Disjunct result is a type-as-VALUE — ride it dynamic so carrier-side
	// consumers don't misread it as a union carrier (same rule as the
	// tany/tall fold).
	if IsDisjunct(&out[0]) { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		out[0] = NewDynamicCarrierValue(out[0])
	}
	return out
}

// unifyFoldable: a bare type node, or fully-concrete inert data (scalars,
// none, lists/maps of the same — type literals admitted inside containers).
func unifyFoldable(v Value) bool {
	if IsBareTypeNode(&v) {
		return true
	}
	if v.Carrier() || IsDynamic(&v) || IsUndefined(&v) || !IsConcrete(&v) {
		return false
	}
	switch {
	case v.Parent().ConformsTo(TList):
		l, err := AsList(v)
		if err != nil || l.IsNil() {
			return false
		}
		for i := 0; i < l.Len(); i++ {
			if !unifyFoldable(l.Get(i)) {
				return false
			}
		}
		return true
	case v.Parent().ConformsTo(TMap):
		m, err := AsMap(v)
		if err != nil || m == nil {
			return false
		}
		for _, k := range m.Keys() {
			e, _ := m.Get(k)
			if !unifyFoldable(e) {
				return false
			}
		}
		return true
	default:
		return v.Parent().ConformsTo(TScalar) || v.Parent().Equal(TNone)
	}
}

func unifyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	unified, ok := core.UnifyR(args[0], args[1], r)
	if ok {
		return []Value{unified, NewBoolean(true)}, nil
	}
	return []Value{NewString("~unify-fail"), NewBoolean(false)}, nil
}
