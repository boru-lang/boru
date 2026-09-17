package core

// Core carrier constructors (Stage 2c of the four-piece split, seam S3b):
// the pure Value constructors the core dispatch/unification paths call
// (sigTypeMatches, the gradual arms).
//
// The ADR-013 2026-08-08 amendment moved the rest of the carrier
// LATTICE down here too — the join family (carrier_join.go), the body
// runners (carrier_body.go), guard narrowing (guard_narrow.go) — so a
// word library can carry an analysis half without naming the checker.
// What stayed in check/go is the analysis PASS itself: the fn/loop body
// models, dispatch modelling and recovery, reached through the
// AnalysisImpl slots.

// NewCarrier constructs a carrier Value for the given type. Data is
// nil for scalar types. For TList and TMap, Data is set to a
// ChildTypeInfo wrapping an Any carrier so the carrier satisfies
// positionalMatch's "concrete list/map" rule (it rejects values
// whose Data==nil when the signature requires a concrete TList or
// TMap). Typed-list carriers (element type known) are produced via
// NewCarrierTypedList / NewCarrierTypedListValue.
func NewCarrier(t *Type) Value {
	v := NewValueRaw(t, nil)
	v.Carrier = true
	if t.Equal(TList) || t.Equal(TMap) {
		v.Data = ChildTypeInfo{Child: Value{Parent: TAny, Carrier: true}}
	}
	return v
}

// NewDynamicCarrier constructs a bounded gradual carrier dynamic(t):
// a carrier whose Parent is the BOUND t and whose Dynamic flag flips
// matching to the not-disjoint rule (design/legacy/dynamic-modality-report.10.ignore).
// dynamic(Any) is the classic gradual `any` — compatible with every
// slot. Use this at an escape hatch where the checker has a best static
// bound but cannot prove the exact type.
func NewDynamicCarrier(t *Type) Value {
	v := NewCarrier(t)
	v.Dynamic = true
	return v
}

// carrierOfLiteral converts a bare type-literal value (the node
// itself, as stored in DisjunctInfo.Alternatives) into a carrier OF
// that node — i.e. Parent points at the literal's type, not at the
// literal's lattice parent.
func CarrierOfLiteral(lit Value) Value {
	lt := lit
	return NewCarrier(&lt)
}

// NewDynamicCarrierValue promotes an existing carrier value (e.g. a
// disjunct carrier for dynamic(A tor B), or a narrowed bound) to the
// dynamic modality, preserving its Parent/Data bound.
func NewDynamicCarrierValue(bound Value) Value {
	bound.Carrier = true
	bound.Dynamic = true
	return bound
}

// NewCarrierTypedList constructs a typed-list carrier — a list
// carrier whose element type is known. Implemented as a regular
// Value with Parent=TList and Data=ChildTypeInfo{Child: NewCarrier(elem)}.
// The Carrier flag is still set so the rest of the engine treats it
// as abstract. Downstream list-consuming words can recover the
// element carrier via dataListElemType.
func NewCarrierTypedList(elem *Type) Value {
	v := NewTypedList(NewCarrier(elem))
	v.Carrier = true
	return v
}

// NewCarrierTypedListValue constructs a typed-list carrier whose
// element is an arbitrary carrier Value. Use this when the element
// itself is a typed list (nested lists), a disjunct, or otherwise
// needs more structure than a bare Parent.
func NewCarrierTypedListValue(child Value) Value {
	v := NewTypedList(child)
	v.Carrier = true
	return v
}

// UnionCarrierForType returns the DISTRIBUTING carrier for a user-defined
// union/enum type — a strict Disjunct of the type's alternatives, the exact
// shape a branch join of distant cousins produces (JoinCarriers), so
// sigTypeMatches' strict-disjunct branch and disjunctPartitionReturns treat
// it identically. ok=false for any type without a disjunctUnifier Behavior.
// This is the third multi-denotation carrier shape (after dynamic carriers
// and payload-bearing joins); the distribute-over-dispatch invariant
// (TestDistributeOverDispatchInvariant) pins all of them.
func UnionCarrierForType(t *Type) (Value, bool) {
	if t == nil {
		return Value{}, false
	}
	du, ok := t.Behavior().(*DisjunctUnifier)
	if !ok || len(du.Alternatives) == 0 {
		return Value{}, false
	}
	dv := NewDisjunct(SimplifyDisjunctAlts(du.Alternatives))
	dv.Carrier = true
	return dv, true
}

// ReturnsIdentity is a ReturnsFunc helper that returns its inputs
// unchanged (as carriers). Use for stack operations that preserve
// their inputs — dup, swap, over, rot, etc. — where the output types
// are directly expressible in terms of the input types.
//
// The mapping is a permutation-description slice: result[i] = args[mapping[i]].
// Example: swap is ReturnsIdentity(1, 0); over is ReturnsIdentity(0, 1, 0).
//
// A DUPLICATED source index (dup `(0, 0)`, over `(0, 1, 0)`) would otherwise
// return the same Value — one Value.ID — for several stack outputs, which
// the bytecode emitter's per-value provenance (emit.go producedBy) cannot
// tell apart: a `dup`-bodied higher-order word (`each [dup add]`) records
// both of add's operands onto the LAST output, so the operand layout refuses
// them as "not adjacent." Each output of a repeated source gets a fresh
// identity (the carrier-identity DUP path) so the N copies stay distinct;
// the source's own provenance is left untouched (no output keeps its ID).
// Identity-only — runtime dispatch is unaffected (ReturnsFn is check-mode).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		counts := make(map[int]int, len(mapping))
		for _, m := range mapping {
			counts[m]++
		}
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			if m < 0 || m >= len(args) {
				out[i] = NewCarrier(TAny)
				continue
			}
			v := args[m] // struct copy: the ID write below is local to v.
			if counts[m] > 1 {
				v.ID = GenerateID(IDPrefixForType(v.Parent))
			}
			out[i] = v
		}
		return out
	}
}
