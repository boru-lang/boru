package native

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// comparisonNatives is the consolidated set of comparison words —
// lt / gt / lte / gte / cmp / tcmp / eq / neq / deq — plus the closed-
// interval DepScalar constructor `between`. The ordering words
// lt / gt / lte / gte also accept a `Type N` form that builds a
// DepScalar refinement of the named scalar type (`Integer lt 10`,
// `Integer between 0 100`, …).
//
// The ordering words (cmp / lt / lte / gt / gte) are FAMILY-RESTRICTED:
// they compare only same-type values, or values a shared same-family
// comparer can handle (Integer-vs-Float, two Dates, …). A cross-family
// pair (Integer-vs-String, List-vs-Map) raises [boru/incomparable]. The
// total order over ALL values — what sort and the collection words use —
// is surfaced by `tcmp`, which compares anything. Equality words
// (eq / neq / deq) stay total: cross-type compares as not-equal, never
// an error.
//
// Argument convention follows the b-op-a mirror rule:
//
//	a b lt     → args[0]=b args[1]=a → compare(a, b) → a < b
//	10 lt 3    → infix reading: 10 < 3 → false
//	a b cmp    → -1 / 0 / 1 for a sorting before / with / after b
//	a b tcmp   → same, across any types (unrestricted total order)
//
// Algorithms (LtHandler / GtHandler / CmpHandler / TcmpHandler /
// EqHandler / DeqHandler / CompareValues / MakeDepScalarSig /
// BetweenHandler) live in eng; this file owns the word names and
// dispatch wiring.
var comparisonNatives = []NativeFunc{
	{
		Name:          "lt",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			core.MakeDepScalarSig("lt", core.DepLT),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(core.LtHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: check.OrderingReturnsFn(core.LtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "gt",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			core.MakeDepScalarSig("gt", core.DepGT),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(core.GtHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: check.OrderingReturnsFn(core.GtHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "lte",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			core.MakeDepScalarSig("lte", core.DepLTE),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(core.LteHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: check.OrderingReturnsFn(core.LteHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		Name:          "gte",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{
			core.MakeDepScalarSig("gte", core.DepGTE),
			{
				Args:      []*Type{TAny, TAny},
				Impl:      Go(core.GteHandler),
				Returns:   []*Type{TBoolean},
				ReturnsFn: check.OrderingReturnsFn(core.GteHandler, TBoolean), BarrierPos: -1,
			},
		},
	},
	{
		// cmp is the three-way comparison: `a b cmp` yields the
		// Integer -1, 0, or 1 for a sorting before / with / after b.
		// Family-restricted — cross-family operands raise
		// [boru/incomparable]; use tcmp for a cross-type total order.
		Name:          "cmp",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(core.CmpHandler),
			Returns:   []*Type{TInteger},
			ReturnsFn: check.OrderingReturnsFn(core.CmpHandler, TInteger), BarrierPos: -1,
		}},
	},
	{
		// tcmp is cmp without the family restriction: `a b tcmp`
		// yields -1 / 0 / 1 for ANY two values, via the unified
		// lattice total order (the order sort and the collection
		// words use). Reach for it when you want cross-type ordering
		// that cmp declines, e.g. `1 tcmp "a"`.
		Name:          "tcmp",
		CompileEffect: CompileIslandPure | CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(core.TcmpHandler),
			Returns: []*Type{TInteger}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name: "between",

		Signatures: []Signature{{
			Args:       []*Type{TScalar, TScalar, TScalar},
			TypeArgs:   map[int]bool{2: true},
			Impl:       Go(core.BetweenHandler, RunInCheck()),
			Returns:    []*Type{TScalar},
			BarrierPos: -1,
		}},
	},
	{
		Name:          "eq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(core.EqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // reads a fn value's identity/content, never invokes it
		}},
	},
	{
		Name:          "neq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(core.NeqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // reads a fn value's identity/content, never invokes it
		}},
	},
	{
		Name:          "deq",
		CompileEffect: CompileScalarFold,

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(core.DeqHandler),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // reads a fn value's identity/content, never invokes it
		}},
	},
}
