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
	c := &content{parent: t, carrier: true}
	if t.Equal(TList) || t.Equal(TMap) {
		c.data = ChildTypeInfo{Child: MakeCarrierValue(TAny, nil)}
	}
	return Value{c: c, occ: idOcc(GenerateID(IDPrefixForType(t)))}
}

// NewCarrierOf constructs a carrier of t whose payload is data — what the
// check pass knows about the values it stands for (a code effect, a store
// shape) — with an identity of its own, as NewCarrier's.
func NewCarrierOf(t *Type, data Payload) Value {
	return Value{c: &content{parent: t, carrier: true, data: data}, occ: idOcc(GenerateID(IDPrefixForType(t)))}
}

// NewDynamicRecordCarrier is dynamic(Map) carrying the record schema
// fields: the carrier a record-typed fn param or return is analysed under
// (check's recordSchemaCarrier and recordReturnCarrier), with a fresh ID
// per carrier because the bytecode compiler keys a frame local and a
// recorded result by it. Built in one literal, the dynamic mark included.
func NewDynamicRecordCarrier(fields *OrderedMap) Value {
	return Value{c: &content{parent: TMap, carrier: true, data: RecordTypeInfo{Fields: fields}},
		occ: idOcc(GenerateID(IDPrefixForType(TMap))), flags: occDynamic}
}

// NewDynamicCarrier constructs a bounded gradual carrier dynamic(t):
// a carrier whose Parent is the BOUND t and whose Dynamic flag flips
// matching to the not-disjoint rule (design/legacy/dynamic-modality-report.10.ignore).
// dynamic(Any) is the classic gradual `any` — compatible with every
// slot. Use this at an escape hatch where the checker has a best static
// bound but cannot prove the exact type.
func NewDynamicCarrier(t *Type) Value {
	v := NewCarrier(t)
	SetDynamic(&v, true)
	return v
}

// CarrierOfLiteral converts a bare type-literal value (a copy of the
// node, as stored in DisjunctInfo.Alternatives) into a carrier OF that
// node — i.e. Parent points at the literal's node, not at the literal's
// lattice parent, and not at a copy of it.
func CarrierOfLiteral(lit Value) Value {
	return NewCarrier(literalNode(&lit))
}

// ValueCarrier is the carrier a check-mode model widens the value v to: a
// carrier of v's type, keeping its gradual modality. A type literal (a bare
// type node, IsTypeLiteral) is the one value whose Parent is not its type:
// its Parent is its supertype in the lattice (the Integer node's is Number),
// so a carrier of its Parent claims a Number where the run holds a type, and
// a dispatch commits over it — `sub m.e 3` over `{e: Integer}` ran sub's
// Number handler on the type literal, where the interpreter's dispatch
// refuses a type at a value slot (NUR323). It widens to a Type carrier: the
// value slots refuse it as they refuse the run's type literal, a Type or Any
// slot takes it, and `is` / `typeof`, which read the node's own place in the
// lattice, fold nothing over it (as over typeof's own Type carrier). A
// gradual Type would reach a Function slot, since Function is a Type. The
// None literal is a value at dispatch and widens to the None carrier.
func ValueCarrier(v Value) Value {
	switch {
	case IsTypeLiteral(&v):
		return NewCarrier(TType)
	case IsBareTypeNode(&v):
		return NewCarrier(TNone)
	}
	c := NewCarrier(ParentOf(&v))
	SetDynamic(&c, IsDynamic(&v))
	return c
}

// NewDynamicCarrierValue promotes an existing carrier value (e.g. a
// disjunct carrier for dynamic(A tor B), or a narrowed bound) to the
// dynamic modality, preserving its Parent/Data bound.
func NewDynamicCarrierValue(bound Value) Value {
	v := MintCarrierKeeping(&bound, DataOf(&bound))
	SetDynamic(&v, true)
	return v
}

// NewCarrierTypedList constructs a typed-list carrier — a list
// carrier whose element type is known. Implemented as a regular
// Value with Parent=TList and Data=ChildTypeInfo{Child: NewCarrier(elem)}.
// The Carrier flag is still set so the rest of the engine treats it
// as abstract. Downstream list-consuming words can recover the
// element carrier via dataListElemType.
func NewCarrierTypedList(elem *Type) Value {
	v := NewTypedList(NewCarrier(elem))
	SetCarrier(&v, true)
	return v
}

// NewCarrierTypedListValue constructs a typed-list carrier whose
// element is an arbitrary carrier Value. Use this when the element
// itself is a typed list (nested lists), a disjunct, or otherwise
// needs more structure than a bare Parent.
func NewCarrierTypedListValue(child Value) Value {
	v := NewTypedList(child)
	SetCarrier(&v, true)
	return v
}

// CarrierTypedListOf is the typed-list carrier whose element has v's shape:
// a disjunct as it stands, a GRADUAL value's type as a dynamic element, and
// any other value's type strict. A gradual value is one the run may hold
// something else in place of (a flex element, a typed container's element
// read at its supertype, a read that may be None); a strict element type
// lets the next body commit a direct op over it, and the run's value then
// answers through that op where the interpreter's dispatch raises or picks
// another overload (NUR316: `ys each [add 1]` over a list `each` built from
// such a read answered [[1]] for [['s1']]).
func CarrierTypedListOf(v Value) Value {
	if IsDisjunct(&v) {
		return NewCarrierTypedListValue(v)
	}
	if IsTypeLiteral(&v) {
		return NewCarrierTypedListValue(ValueCarrier(v)) // a type VALUE (NUR323)
	}
	if IsDynamic(&v) {
		c := NewCarrier(ParentOf(&v))
		SetDynamic(&c, true)
		return NewCarrierTypedListValue(c)
	}
	return NewCarrierTypedList(ParentOf(&v))
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
	SetCarrier(&dv, true)
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
// hand back one value — one identity — as several stack outputs. The
// recorder tells event results apart by their (event, index) key now, but a
// pass also tracks values by identity (reads, captures, folds), so each
// output of a repeated source is MINTED with an identity of its own
// (MintFresh — the carrier-identity DUP path): the N copies are N values,
// and the source keeps its own identity and producer. Identity-only —
// runtime dispatch is unaffected (ReturnsFn is check-mode).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		counts := make(map[int]int, len(mapping))
		for _, m := range mapping {
			counts[m]++
		}
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			switch {
			case m < 0 || m >= len(args):
				out[i] = NewCarrier(TAny)
			case counts[m] > 1:
				out[i] = MintFresh(&args[m])
			default:
				out[i] = args[m]
			}
		}
		return out
	}
}
