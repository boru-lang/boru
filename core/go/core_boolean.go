package core

// This file owns the algorithms behind the type-level connective
// words (tor / tand) and the helper TandValues which is reused by
// lang's typed-all reduction. The matching word registrations now
// live in lang/go/engine/native_boolean.go (not/and/or) and
// lang/go/engine/native_type.go (tor/tand) — eng exposes only the
// algorithm primitives.
//
// tor / tand operate on *Type-shaped values:
//
//   T tor U      — disjunct union; flattens nested disjuncts,
//                  removes Never (the disjunct identity), and
//                  dedupes structurally identical alternatives.
//                  Single-alt result returned bare; empty result
//                  collapses to Never.
//   T tand U     — type intersection, distributing over disjuncts.
//                  Concrete maps merge field-wise. Anything that
//                  fails to unify collapses to Never.

// TorHandler is the type-level disjunct-builder handler. Flattens any
// disjunct alternatives in either input, simplifies (Never is the
// identity, structural duplicates are removed), and returns:
//   - Never if no alternatives remain
//   - the lone alternative if only one remains
//   - a fresh disjunct otherwise
//
// Exported so lang's tor registration (lang/go/engine/native_type.go)
// can wire dispatch into it without forking the algorithm.

// resolveTypeOperand resolves a NAMED type operand — a bare node,
// which is what a type name evaluates to after the Stage 2 flip
// (design/legacy/TYPE-REPRESENTATION.1.ignore §5) — to its declared content for
// the type-algebra words, which operate on STRUCTURE: `M tor String`
// flattens M's alternatives exactly as it did when the name evaluated
// to its body. Builtin and refine nodes record no content and stay
// nominal operands.
func resolveTypeOperand(v Value) Value {
	if b, ok := TypeContentOf(v); ok && IsBareTypeNode(v) {
		return b
	}
	return v
}

func TorHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	args = []Value{resolveTypeOperand(args[0]), resolveTypeOperand(args[1])}
	// De Morgan: (tnot A) tor (tnot B) = tnot (A tand B). Fold the two
	// negations into the negation of their inners' intersection rather
	// than leaving a disjunct of negations that denotes the same set.
	if IsNegation(args[0]) && IsNegation(args[1]) {
		a0, _ := AsNegation(args[0])
		a1, _ := AsNegation(args[1])
		return []Value{NegateType(TandValues(a1.Inner, a0.Inner))}, nil
	}
	alts := append(FlattenDisjunctAlts(args[1]), FlattenDisjunctAlts(args[0])...)
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if len(simplified) == 1 {
		return []Value{simplified[0]}, nil
	}
	return []Value{NewDisjunct(simplified)}, nil
}

// TorReturnsFn is the carrier-mode counterpart. `tor` is a type UNION, so the
// check-mode result must mirror TorHandler's disjunct construction
// (flatten / simplify / NewDisjunct via unionType), NOT a branch-merge
// JoinCarriers — the latter applies subsumption / sibling-collapse / width-cap
// and mishandles a nil-Parent None type literal (the union arm of
// `String tor None`), producing a zero Value that halts the check-mode tape
// (`def Maybe (String tor None) … tcmp …`, compare.tsv). Mirroring the handler
// also keeps `(A tor B)` usable as a `def` type body in check mode exactly as
// it is at run time.
func TorReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TAny)}
	}
	if IsNegation(args[0]) && IsNegation(args[1]) {
		a0, _ := AsNegation(args[0])
		a1, _ := AsNegation(args[1])
		return []Value{NegateType(TandValues(a1.Inner, a0.Inner))}
	}
	return []Value{UnionType(args[1], args[0])}
}

// unionType builds the simplified disjunction of two type values
// (flatten, dedupe, subsume): Never for an empty union, the lone
// alternative for a singleton, else a fresh disjunct. The same
// reduction TorHandler applies, factored out for the De Morgan folds
// and the DepScalar complement.
func UnionType(a, b Value) Value {
	s := SimplifyDisjunctAlts(append(FlattenDisjunctAlts(a), FlattenDisjunctAlts(b)...))
	switch len(s) {
	case 0:
		return NewTypeLiteral(TNever)
	case 1:
		return s[0]
	default:
		return NewDisjunct(s)
	}
}

// TandReturnsFn is `tand`'s check-mode result: the intersection is a PURE
// type-level computation over the operand values, so run the real handler —
// the meet (a DepScalar interval intersection, a narrowed disjunct, Never)
// then survives as a valid type body for a typed-def annotation
// (`def x:(A tand B) 15`), exactly as TorReturnsFn does for unions. A
// non-type operand (a carrier from a runtime computation) degrades to the
// declared Any.
func TandReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) != 2 || !IsTypeBody(args[0]) || !IsTypeBody(args[1]) {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{TandValues(args[1], args[0])}
}

// TandHandler delegates to TandValues for the actual intersection
// computation. Args are passed in [args[1], args[0]] order so the
// natural infix reading (`A tand B → tandValues(A, B)`) matches the
// b-op-a handler convention — see mirror.tsv.
func TandHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{TandValues(args[1], args[0])}, nil
}

// TandValues computes the type-level intersection of a and b.
// Distributes over disjuncts, propagates Never (annihilator), merges
// concrete maps field-wise, and falls back to Unify for everything
// else. Always returns a value — disjoint inputs collapse to Never.
//
// Distribution rule: (A tor B) tand C = (A tand C) tor (B tand C).
// When both sides are disjuncts, the cross product is computed and
// each pair recursively reduced. Never-valued cross-product entries
// are filtered (Never is the identity for tor), and structurally
// identical alternatives are deduped.
//
// Exported because boru's `tall` (typed-all reduction) folds over a
// list of values via this same intersection. Future higher-order
// type combinators may want it too.
func TandValues(a, b Value) Value {
	a, b = resolveTypeOperand(a), resolveTypeOperand(b)
	// Either operand is Never (sentinel or bare type literal) →
	// intersection is Never.
	if IsNeverShape(a) || IsNeverShape(b) {
		return NewTypeLiteral(TNever)
	}

	// De Morgan: (tnot A) tand (tnot B) = tnot (A tor B). Without this
	// the two negations fall through to Unify and wrongly collapse to
	// Never; folding the inners into a union and negating is both correct
	// and the canonical single-negation form.
	if IsNegation(a) && IsNegation(b) {
		ai, _ := AsNegation(a)
		bi, _ := AsNegation(b)
		return NegateType(UnionType(ai.Inner, bi.Inner))
	}

	if IsDisjunct(a) || IsDisjunct(b) {
		aAlts := FlattenDisjunctAlts(a)
		bAlts := FlattenDisjunctAlts(b)
		var result []Value
		for _, ax := range aAlts {
			for _, bx := range bAlts {
				result = append(result, TandValues(ax, bx))
			}
		}
		simplified := SimplifyDisjunctAlts(result)
		switch len(simplified) {
		case 0:
			return NewTypeLiteral(TNever)
		case 1:
			return simplified[0]
		default:
			return NewDisjunct(simplified)
		}
	}

	if isPlainConcreteMap(a) && isPlainConcreteMap(b) {
		ma, _ := AsMap(a)
		mb, _ := AsMap(b)
		merged, ok := mergeMaps(ma, mb)
		if !ok {
			return NewTypeLiteral(TNever)
		}
		return NewMap(merged)
	}

	unified, ok := Unify(a, b)
	if !ok {
		return NewTypeLiteral(TNever)
	}
	return unified
}

// isNeverShape reports whether v is any form of Never: the sentinel
// (Parent=TNever) or the bare type literal (NewTypeLiteral(TNever),
// Data=nil, lattice node == TNever).
func IsNeverShape(v Value) bool {
	if v.Parent.Equal(TNever) {
		return true
	}
	return IsBareTypeNode(v) && (&v).Equal(TNever)
}

// isAnyShape reports whether v is any form of Any: the carrier/sentinel
// (Parent=TAny) or the bare type literal (NewTypeLiteral(TAny)).
func isAnyShape(v Value) bool {
	if v.Parent.Equal(TAny) {
		return true
	}
	return IsBareTypeNode(v) && (&v).Equal(TAny)
}

// NegateType computes the set-theoretic complement of a type value,
// applying the algebraic identities that have a closed form:
//
//	tnot Never    → Any    (complement of the empty type is the top)
//	tnot Any      → Never  (complement of the top is empty)
//	tnot (tnot A) → A      (double-negation elimination)
//
// Every other inner type wraps in a NegationInfo value, whose matching
// semantics (v matches `tnot A` iff v does not match A — see
// unify_negation.go) are correct without further structural reduction.
// De Morgan distribution over disjuncts is unnecessary for correctness:
// `tnot (A tor B)` already admits exactly the values matching neither A
// nor B.
func NegateType(inner Value) Value {
	return negateTypeResolved(resolveTypeOperand(inner))
}

// negateTypeResolved is NegateType after operand resolution. The guard
// complement (ApplyComplementNarrowing) calls it DIRECTLY with the
// minted node so a refinement guard's else-arm complement stays the
// nominal `tnot Node` it always was — resolving the node to its
// DepScalar content there would swap in the interval complement, a
// different (stronger) claim than the guard licenses.
func negateTypeResolved(inner Value) Value {
	if IsNeverShape(inner) {
		return NewTypeLiteral(TAny)
	}
	if isAnyShape(inner) {
		return NewTypeLiteral(TNever)
	}
	if IsNegation(inner) {
		if ni, err := AsNegation(inner); err == nil {
			return ni.Inner
		}
	}
	if inner.IsDepScalar() {
		if di, err := inner.AsDepScalar(); err == nil {
			// Closed-form complement of a refinement. Over the universe
			// the complement of `Integer gt 0` is "integers that are not
			// > 0" PLUS "everything that isn't an Integer at all", i.e.
			// (Integer lte 0) tor (tnot Integer). The within-base part is
			// a positive DepScalar (or a union of two rays for an
			// interval), so `Integer tand (tnot (Integer gt 0))` reduces
			// to `Integer lte 0` through the disjunct distribution in
			// TandValues.
			base := inner.Parent
			return UnionType(complementWithinBase(base, di), NewNegation(NewTypeLiteral(base)))
		}
	}
	return NewNegation(inner)
}

// TnotHandler is the runtime handler for the `tnot` type-negation word.
// `tnot T` produces the complement type of T.
func TnotHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NegateType(args[0])}, nil
}

// TnotReturnsFn is the carrier-mode counterpart: produces the negation
// carrier so `boru check` propagates the complement type. The result
// carries Carrier=true so the carrier-stripping pass preserves its
// NegationInfo payload (mirrors how TorReturnsFn's disjunct carrier
// survives toCarrier).
func TnotReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) != 1 {
		return []Value{NewCarrier(TAny)}
	}
	out := NegateType(args[0])
	out.Carrier = true
	return []Value{out}
}

// isPlainConcreteMap reports whether v is a non-typed, non-record,
// non-options concrete map (Data is *OrderedMap).
func isPlainConcreteMap(v Value) bool {
	if !v.Parent.Equal(TMap) || !IsConcrete(v) {
		return false
	}
	if IsRecordType(v) || IsOptionsType(v) || IsTypedMap(v) {
		return false
	}
	m, err := AsMap(v)
	return err == nil && m != nil
}

// mergeMaps walks keys of a then b in order, intersecting values for
// keys present in both via TandValues. Keys present in only one side
// are kept as-is. Returns ok=false when any overlapping key has
// incompatible values — the caller propagates that as Never (the
// empty intersection).
func mergeMaps(a, b ReadMap) (*OrderedMap, bool) {
	result := NewOrderedMap()
	for _, key := range a.Keys() {
		aVal, _ := a.Get(key)
		if bVal, present := b.Get(key); present {
			combined := TandValues(aVal, bVal)
			// Combined is Never — check both forms: sentinel (Parent=
			// TNever) and bare type literal (NewTypeLiteral(TNever),
			// Parent=nil after the degenerate-root setup).
			if combined.Parent.Equal(TNever) || (IsBareTypeNode(combined) && (&combined).Equal(TNever)) {
				return nil, false
			}
			result.Set(key, combined)
			continue
		}
		result.Set(key, aVal)
	}
	for _, key := range b.Keys() {
		if _, ok := a.Get(key); ok {
			continue
		}
		bVal, _ := b.Get(key)
		result.Set(key, bVal)
	}
	return result, true
}
