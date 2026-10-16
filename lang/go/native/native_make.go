package native

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// makeNatives installs the `make` word — the universal constructor
// for typed values (scalars, class instances, records, tables).
//
//	make T data       — build a T-typed value from data; T may be a
//	                    Scalar type, a ClassType, a RecordType, a
//	                    TableType, or any subtype thereof.
//	make T data opts  — same with an options map (currently the
//	                    `use_base:true` flag for classes/records).
//
// The algorithm primitives (MakeHandler, MakeScalarHandler,
// MakeScalarOptsHandler, MakeObjHandler, MakeWithOpts, plus
// MakeObject / MakeConvert / MakeFieldValue / ResolveFieldType) live
// in eng/go/core_make.go; this file owns the word name, signature
// shape, and dispatch wiring.
// makeObjReturns is the class/Resource make sig's check-mode result:
// ReturnsFreshInstance(0) plus the schema-decidable construction validation
// (check.CheckMakeConstruction — unknown / missing / mistyped fields on a
// concrete construction map, with the byte-identical runtime messages).
func makeObjReturns() ReturnsFunc {
	fresh := ReturnsFreshInstance(0)
	return func(args []Value, r *Registry) []Value {
		if len(args) >= 2 && !core.HasMaker(r, args[0]) {
			check.CheckMakeConstruction(r, args[0], args[1], args[0].Pos())
		}
		return fresh(args, r)
	}
}

// makeScalarReturns is the scalar make sig's check-mode result:
// ReturnsFreshInstance(0) plus the Micron construction validation
// (basicCheckMicronConstruction — string parsing, unknown / missing /
// mistyped fields, the abstract root — on a fully-concrete source,
// with the byte-identical runtime messages). Micron targets dispatch
// through the scalar sig (the family roots under Scalar), so this is
// their CheckMakeConstruction analogue.
func makeScalarReturns() ReturnsFunc {
	fresh := ReturnsFreshInstance(0)
	return func(args []Value, r *Registry) []Value {
		if len(args) >= 2 {
			basicCheckMicronConstruction(r, args[0], args[1], args[0].Pos())
			// An Ideal-routed construction over a fully-concrete source is
			// a PURE parse of immutable scalar data — the same Instantiate
			// the runtime runs, deterministic in its arguments — so the
			// model runs it and surfaces the CONCRETE result (the typeof
			// precedent: concrete-gated, blast radius off every
			// non-concrete make). Downstream models then see the real
			// value: two made Qions feed micronOpReturns' concrete error
			// mirror, a made micron dispatches its exact overload, instead
			// of every consumer widening at a bare carrier. The gate
			// mirrors MakeScalarHandler's own routing test (Ideals.For)
			// AND requires the routed kind to declare PureInstantiate —
			// the Ideal contract puts no purity requirement on
			// Instantiate in general, so an unmarked kind (a host
			// constructor that may touch services) is never executed at
			// analysis time (PR #346 review). A failed construction falls
			// through to the fresh carrier — the validation above already
			// carried the diagnostic.
			if r != nil && core.IsBareTypeNode(&args[0]) && core.IsConcrete(&args[1]) {
				if id := r.Ideals.For(args[0]); id != nil && id.PureInstantiate && id.Instantiate != nil {
					if out, err := id.Instantiate(args[0], args[1], r); err == nil && len(out) == 1 && core.IsConcrete(&out[0]) {
						return out
					}
				}
			}
		}
		return fresh(args, r)
	}
}

// makeNodeReturns wraps ReturnsFreshInstance for the node-make sig: the
// check residual additionally carries the source's element tag for the
// targets whose RUNTIME carries it — FlexMap/FlexList keep a header
// {:T}/[:T] through FlexDeepCopy, and the weak targets keep both the
// header tag and a typed-literal's ChildTypeInfo tag (MakeNodeHandler's
// weak arms) — so a later non-conforming write into the made container
// is check-flagged exactly where the runtime raises (d2CheckWrite).
func makeNodeReturns() ReturnsFunc {
	fresh := ReturnsFreshInstance(0)
	return func(args []Value, r *Registry) []Value {
		out := fresh(args, r)
		if len(args) != 2 || len(out) != 1 || !core.IsBareTypeNode(&args[0]) {
			return out
		}
		target, src := args[0], args[1]
		weakTarget := target.Equal(TWeakFlexMap) || target.Equal(TWeakFlexList)
		if !weakTarget && !target.Equal(TFlexMap) && !target.Equal(TFlexList) {
			return out
		}
		if elem, ok := src.ElemConstraint(); ok {
			out[0].SetElemConstraint(elem)
		} else if ci, cerr := core.AsChildType(src); weakTarget && cerr == nil {
			out[0].SetElemConstraint(ci.Child)
		}
		return out
	}
}

var makeNatives = []NativeFunc{
	{
		Name:          "make",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{
			// make is an IMPURE constructor: each call mints a fresh
			// instance at run time (NewValueRaw), so its check-mode result
			// carrier must also be fresh-identity per call — ReturnsIdentity
			// would return the SAME type-literal value (one Value.ID), which
			// collapses two `make P {}` operands onto one in the bytecode
			// lowerer's per-value provenance. See ReturnsFreshInstance.
			{Args: []*Type{TScalar, TMap, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(core.MakeScalarOptsHandler), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TIdeal, TMap}, TypeArgs: map[int]bool{0: true}, Impl: Go(core.MakeObjHandler), ReturnsFn: makeObjReturns(), BarrierPos: -1},
			// Node-family targets: make FlexMap/FlexList (mutable deep
			// copy) and make Map/List (deep immutable conversion — the
			// inverse). Structural type bodies that land in the Node
			// TypeArgs slot are deferred back to MakeHandler inside
			// the handler.
			{Args: []*Type{TNode, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(core.MakeNodeHandler), ReturnsFn: makeNodeReturns(), BarrierPos: -1},
			{Args: []*Type{TScalar, TAny}, TypeArgs: map[int]bool{0: true}, Impl: Go(core.MakeScalarHandler), ReturnsFn: makeScalarReturns(), BarrierPos: -1},
			{Args: []*Type{TAny, TAny, TMap}, Impl: Go(core.MakeWithOpts), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(core.MakeHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}
