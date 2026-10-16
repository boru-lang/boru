package core

// The variadic-spread carrier (ADR-013, 2026-08-08 amendment): the
// residual shape a `[]`-declared recursive fn's body produces, and the
// `if`-arm fold that collapses two arms carrying one. Moved down from
// the check piece with the rest of the carrier lattice — every symbol
// here is a plain constructor/predicate over core Value, and `basic`'s
// `if` needs FoldVariadicArms without naming the checker.

// SpreadPayload is the payload of a "variadic spread" carrier: a residual
// entry denoting 0-or-more values of element type Elem. It exists ONLY on the
// plain-check surface (a `[]`-declared recursive fn whose body leaks a
// per-frame value — recursion.tsv:53 — cannot be modelled by a fixed-length
// residual because the depth is a runtime value; `await`'s winner-takes-all
// `first`/`any` residual — NUR067) and is consumed only by the soundness
// oracle. Elem is a Value (a type literal or a disjunct of the
// per-frame leaked types), never Any/Dynamic for a producer whose element
// types are STATICALLY KNOWABLE — a variadic-Any marker would let the oracle
// admit a wrong-typed leak. The one deliberate Any element is `await`'s
// winner-takes-all model (awaitVariadicResult): the branches are unevaluated
// code bodies run on isolated forks, so the winning residual's types
// genuinely cannot be bounded — there the Any IS the honest claim, not a
// laundered one.
type SpreadPayload struct {
	PayloadBase
	Elem Value
}

// NewVariadicCarrier builds a variadic-spread carrier over element `elem`.
// Parent is TAny so no TList/TMap carrier machinery touches it; it is
// discriminated only via IsVariadicSpread. It is also Dynamic so that if a
// variadic result is CONSUMED by a downstream word (`m 3 add 1`) it matches
// optimistically like dynamic(Any) instead of failing dispatch — the soundness
// oracle intercepts it via IsVariadicSpread BEFORE any Dynamic check, so the
// dynamic flag never weakens the element-type coverage.
func NewVariadicCarrier(elem Value) Value {
	v := NewValueRaw(TAny, SpreadPayload{Elem: elem})
	v.SetCarrier(true)
	SetDynamic(&v, true)
	return v
}

// IsVariadicSpread reports whether v is a NewVariadicCarrier, returning its
// element value.
func IsVariadicSpread(v Value) (Value, bool) {
	if sp, ok := v.Data().(SpreadPayload); ok {
		return sp.Elem, true
	}
	return Value{}, false
}

// FoldVariadicArms models a plain-check `if` whose arm residual contains a
// variadic-spread carrier — the shape a `[]`-declared recursive fn's body
// produces once its self-call's in-flight bail is seeded with a variadic (see
// AnalyseFnBody). It folds BOTH arms into one variadic spread whose element
// joins every per-frame LEAKED type: the non-None, non-variadic slot types
// (the fixed lead below the recursive tail, e.g. `n mul 2` → Integer) plus the
// variadic arms' own elements, dropping Never (the seed). Returns ok=false when
// neither arm carries a variadic, so the ordinary if-join then applies.
func FoldVariadicArms(then, els []Value) (Value, bool) {
	hasVar := false
	var elems []Value
	for _, stk := range [][]Value{then, els} {
		for _, v := range stk {
			if e, ok := IsVariadicSpread(v); ok {
				hasVar = true
				if e.Parent() != nil && !ValueType(e).Equal(TNever) {
					elems = append(elems, e)
				}
				continue
			}
			if v.Parent() == nil || v.Parent().Equal(TNone) {
				continue // None padding / base-case emptiness
			}
			elems = append(elems, NewTypeLiteral(v.Parent()))
		}
	}
	if !hasVar {
		return Value{}, false
	}
	elem := NewTypeLiteral(TNever)
	if len(elems) > 0 {
		elem = elems[0]
		for _, e := range elems[1:] {
			elem = UnionType(elem, e) // drops Never, dedups, keeps a disjunct for genuine unions
		}
	}
	return NewVariadicCarrier(elem), true
}
