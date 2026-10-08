package core

import "fmt"

// Shared structural-fold combinators for the unifier families. The per-family
// handlers (unify_list.go, unify_map.go, …) differ in their exclusive shapes
// and result containers, but they share two mechanical traversals that used to
// be copied verbatim into each: the nominal flex-literal arm and the
// element-wise "unify each, tag the path, collect" loop. Centralising them
// keeps the error-path tagging and the canonicalisation in one place.

// unifyFlexLiteral handles the nominal flex-subtype literal arm shared by the
// list and map families: a `FlexList` / `FlexMap` type literal is a newtype
// that unifies ONLY with a concrete value of that same flex type (or another
// such literal) — a plain list/map does not qualify. Returns handled=false
// when neither side is the flex literal, so the caller falls through to the
// ordinary family rules.
func unifyFlexLiteral(a Value, sa ValueShape, b Value, sb ValueShape, flexType *Type, isFlex func(Value) bool, name string) (result Value, handled bool, uerr *UnifyError) {
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(flexType)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(flexType)
	if !aLit && !bLit {
		return Value{}, false, nil
	}
	if aLit && bLit {
		return a, true, nil
	}
	lit, other := a, b
	if bLit {
		lit, other = b, a
	}
	if isFlex(other) {
		return other, true, nil
	}
	// Subtree-nominal: a concrete value TAGGED with a descendant of the
	// flex type (a WeakFlex* value, a user `refine FlexMap` newtype)
	// satisfies the parent literal — ordinary nominal subtyping. Plain
	// nodes still do not qualify (they sit ABOVE the flex type).
	if IsConcrete(other) && other.Parent != nil && other.Parent.ConformsTo(flexType) {
		return other, true, nil
	}
	// Gradual: a check-mode CARRIER tagged at (or under) the flex type
	// — the residual of `(flex …)` and every flex-returning word — has
	// no structure to inspect, but its static tag already proves family
	// membership. Without this arm a typed def of a refine-of-flex
	// newtype (`def S (refine FlexMap)  def w:S (flex m)`) check-fails
	// while the identical program unifies at run time — the flex-retag
	// gap that blocked pure-Boru sorted nodes (design/OPEN-WORDS.1.md
	// follow-up).
	if other.Carrier && other.Parent != nil && other.Parent.ConformsTo(flexType) {
		return other, true, nil
	}
	return Value{}, true, unifyFail(name+" type literal needs a concrete "+name, lit, other)
}

// unifyZip unifies n positional pairs, sourcing the left and right value of
// each from leftAt / rightAt, and tags any failure with the indexed path
// "[i]". It is the shared element-wise traversal behind concrete-list and
// typed-list-vs-concrete unification: the former pairs a[i] with b[i], the
// latter pairs the single child type against each element. r is the
// enclosing chain's registry, handed to every pair.
func unifyZip(n int, leftAt, rightAt func(int) Value, r *Registry) ([]Value, *UnifyError) {
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		u, err := unifyInner(leftAt(i), rightAt(i), r)
		if err != nil {
			return nil, err.withPath(fmt.Sprintf("[%d]", i))
		}
		out[i] = u
	}
	return out, nil
}

// constAt returns an index function that always yields v — the "broadcast"
// left-hand side for unifying a single constraint type against every element
// of a container.
func constAt(v Value) func(int) Value { return func(int) Value { return v } }

// sliceAt returns an index function that reads elems[i].
func sliceAt(elems []Value) func(int) Value { return func(i int) Value { return elems[i] } }

// unifyCarrierVsTyped handles an abstract CARRIER (ShapeCarrier — Data==nil,
// Carrier=true) unified against a typed-container constraint (typedShape =
// ShapeTypedList / ShapeTypedMap). The canonical case is a check-mode
// `(flex […])` residual (a bare Flex{Map,List} carrier with no shape tracking)
// meeting a `{:T}` / `[:T]` def annotation: accept GRADUALLY and tag the
// carrier with the element type so the flex binding's reads narrow and writes
// enforce; the concrete elements are re-validated at runtime (the flex body is
// concrete there). Without this the carrier fails the IsListShape/IsMapShape
// family guard — the flex-{:T} false-reject. Returns (_, false) when neither
// side is that pair. Called from within a family handler, so the carrier's
// Parent already conforms to that family. See TYPED-CONTAINER-TAG-RETENTION.0.md.
func unifyCarrierVsTyped(a Value, sa ValueShape, b Value, sb ValueShape, typedShape ValueShape, flexType *Type) (Value, bool) {
	var carrier, typed Value
	switch {
	case sa == ShapeCarrier && sb == typedShape:
		carrier, typed = a, b
	case sb == ShapeCarrier && sa == typedShape:
		carrier, typed = b, a
	default:
		return Value{}, false
	}
	// Scope to a genuine FLEX-family carrier (Parent == FlexMap/FlexList). A
	// general dynamic carrier (e.g. `context get`, whose bound may be provably
	// disjoint from the container family) must fall through to the ordinary
	// narrowing / disjointness rules, NOT be optimistically tagged here — that
	// over-broad match suppressed the dynamic narrowing-through-use diagnostic.
	if carrier.Parent == nil || !carrier.Parent.ConformsTo(flexType) {
		return Value{}, false
	}
	ct, _ := AsChildType(typed)
	out := carrier
	out.SetElemConstraint(ct.Child)
	return out, true
}

// unifyMapValues unifies the value at each key of m, sourcing the left side
// from leftFor(key), and tags failures with "key:<k>". Backing the
// typed-map-vs-concrete traversal: leftFor returns the shared child type.
// r is the enclosing chain's registry, handed to every pair.
func unifyMapValues(m ReadMap, leftFor func(key string) Value, r *Registry) (Value, *UnifyError) {
	result := NewOrderedMap()
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		unified, err := unifyInner(leftFor(key), val, r)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		result.Set(key, unified)
	}
	return NewMap(result), nil
}
