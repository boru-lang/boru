package native

import (
	"fmt"
	"sort"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// arrayNatives (core) and ArrayModuleNatives (the boru:array module)
// are both derived from allArrayNatives below. The split follows one
// rule: words that take a quoted code body, the basic constructors,
// and everyday slicing stay in core; the specialised APL-style data
// vocabulary (shape/structure, selection/ordering, membership/
// grouping, neighborhoods) lives in the module and is reached via
// dot-access (ArrayUtil.shape, ArrayUtil.where, …).
//
//	core   — iota, range, each, fold, scan, outer, inner,
//	         take, shed, reverse
//	module — shape, rank, reshape, transpose, where, unique, indices,
//	         grade, at, sortby, member, group, replicate, expand,
//	         compress, eachrank, foldaxis, window, pairs
//
// Per ADR-001 (no module export shadows a core word — see ADR.md in
// the repo root), deep flatten is NOT an array-module word: it is
// `flatten -1` (a depth on the core flatten word, flatten.go). The
// list-membership lookup IS an array-module word, `indices` (for each
// needle, its index in the haystack, -1 when absent) — a distinct name,
// not a shadow of the string word indexof (string-only, native_string.go).
// transpose has no core counterpart, so it keeps its plain name.
//
// Pure helpers (computeShape, flattenList, buildNested,
// arrCompareValues, transposeListOfLists, doFold,
// analyseHigherOrderBody) live alongside their handlers below.
var allArrayNatives = []NativeFunc{
	// ---- core ----
	{
		Name: "iota",

		Signatures: []Signature{{
			Args:      []*Type{TInteger},
			Impl:      Go(iotaHandler),
			ReturnsFn: returnsIotaLen, BarrierPos: -1,
		}},
	},
	{
		// range is iota's start/stop/step cousin: an arithmetic
		// sequence generator. The 3-arg sig is listed first so
		// `range start stop step` forward-collects all three; the
		// 2-arg sig (step defaults to 1) handles `range start stop`.
		Name: "range",

		Signatures: []Signature{
			{
				Args:      []*Type{TInteger, TInteger, TInteger},
				Impl:      Go(rangeThreeHandler),
				ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
			},
			{
				Args:      []*Type{TInteger, TInteger},
				Impl:      Go(rangeTwoHandler),
				ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
			},
		},
	},
	{
		Name: "shape",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(shapeHandler),
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "rank",

		Signatures: []Signature{{
			Args:    []*Type{TList},
			Impl:    Go(rankHandler),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "reshape",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(reshapeHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		// transpose has no core-word counterpart, so it keeps its plain
		// name (no "arr-" prefix needed). Deep flatten and list indexof,
		// by contrast, are now overloads of the core flatten/indexof
		// words rather than separate array words — see flatten.go and
		// native_string.go.
		Name: "transpose",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(arrTransposeHandler),
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "reverse",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(reverseHandler),
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "take",

		Signatures: []Signature{{
			Args:      []*Type{TInteger, TList},
			Impl:      Go(takeHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "shed",

		Signatures: []Signature{{
			Args:      []*Type{TInteger, TList},
			Impl:      Go(shedHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name:          "where",
		CompileEffect: CompileFallbackBody,

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(whereHandler),
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "unique",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(uniqueHandler),
			ReturnsFn: ReturnsPreserveListAt(0), BarrierPos: -1,
		}},
	},
	{
		Name: "indices",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(indicesHandler),
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "grade",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(gradeHandler),
			ReturnsFn: returnsCarrierTypedListInteger, BarrierPos: -1,
		}},
	},
	{
		Name: "at",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(atHandler),
			ReturnsFn: returnsAtChecked, BarrierPos: -1,
		}},
	},
	{
		// insert-at: copy-returning single-element insertion. Sig order is
		// [index, element, list] — subject last, like set — so the forward
		// form reads `insert-at 1 99 [1 2 3]` and the pipeline form
		// `[1 2 3] insert-at 1 99`. The HAMT feasibility study (voxgig DX
		// report, Theme H) asked for this as a cleaner primitive than the
		// take/concat/shed composition.
		Name: "insert-at",

		Signatures: []Signature{{
			Args: []*Type{TInteger, TAny, TList},
			Impl: Go(insertAtHandler),
			// Pure copy-returning edit: a top-level call over concrete
			// literals with a provably out-of-range index flags at check
			// time with the runtime's own index_out_of_range text.
			ReturnsFn: DryPassReturns(insertAtHandler, TList),
			Returns:   []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		// remove-at: copy-returning single-element removal. Sig order is
		// [index, list]: forward `remove-at 1 [1 2 3]`, pipeline
		// `[1 2 3] remove-at 1`.
		Name: "remove-at",

		Signatures: []Signature{{
			Args: []*Type{TInteger, TList},
			Impl: Go(removeAtHandler),
			// Pure copy-returning edit (see insert-at): the dry pass flags
			// a provably out-of-range concrete index; the base ReturnsFn
			// keeps its element-preserving residual model.
			ReturnsFn: DryPassWrap(removeAtHandler, ReturnsPreserveListAt(1)), BarrierPos: -1,
		}},
	},
	{
		Name: "sortby",

		Signatures: []Signature{
			{
				Args:      []*Type{TList, TList},
				Impl:      Go(sortbyHandler),
				ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
			},
			// Lens form: `sortby $.age people` derives the sort key from each
			// element via the reach, then sorts the data by those keys.
			{
				Args:      []*Type{TReach, TList},
				Impl:      Go(sortbyReachHandler),
				ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
			},
		},
	},
	{
		Name: "member",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(memberHandler),
			ReturnsFn: returnsCarrierTypedListBoolean, BarrierPos: -1,
		}},
	},
	{
		Name:          "group",
		CompileEffect: CompileFallbackBody,

		Signatures: []Signature{
			{
				Args:    []*Type{TList, TList},
				Impl:    Go(groupTwoHandler),
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TList},
				Impl:    Go(groupOneHandler),
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "replicate",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(replicateHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "expand",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(expandHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		Name: "window",

		Signatures: []Signature{{
			Args:      []*Type{TInteger, TList},
			Impl:      Go(windowHandler),
			ReturnsFn: windowReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name: "pairs",

		Signatures: []Signature{{
			Args:      []*Type{TList},
			Impl:      Go(pairsHandler),
			ReturnsFn: pairsReturnsFn, BarrierPos: -1,
		}},
	},

	// ---- higher-order ----
	{
		Name:          "each",
		CompileEffect: CompileFallbackBody,
		// each [body] data — the body sees one element and returns the mapped value.
		// A 0-net body is each's own each_error ("body produced no result"), raised
		// faithfully from InvokeBody, so EmptyBodyErrors compiles it natively rather
		// than islanding.
		// BodyMultiRunKeepsDefs: eachHandler drives InvokeBody once per
		// element on the shared registry with no def cleanup, so a body def
		// leaks one install per element with the per-element runtime value
		// (the parity oracle's measured population) — the twin regime's
		// arm-residency license; see the field's doc in core.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, CrossCollectionTokenShape: true, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			return []Value{NewElementCarrier(DataListElemTypeFromValue(a[1]))}
		}},

		Signatures: []Signature{
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(eachHandler),
				ReturnsFn:  eachReturnsFn, BarrierPos: -1,
			},
			// Lens form: `each $.name people` plucks the reach from every
			// element (the receiverless Reach acts as an arity-1 accessor fn).
			{
				Args:    []*Type{TReach, TList},
				Impl:    Go(eachReachHandler),
				Returns: []*Type{TList}, BarrierPos: -1,
			},
			// LIST Function form (NUR086's fix, 2026-08-24): the same
			// handler, the same element — a Function value reaches the
			// callback exactly where a quotation body would, so
			// `each dbl/v [1 2 3]` is `each [dbl] [1 2 3]` without the
			// wrapper. The callback receives the ELEMENT: a per-container
			// form hands the container's natural unit (element for a list,
			// KeyVal for a map). `filter`'s single cross-container form is
			// the documented exception — it hands a position descriptor.
			{Args: []*Type{TFunction, TList}, Impl: Go(eachHandler), ReturnsFn: eachReturnsFn, BarrierPos: -1},
			// Map forms — iterate entries in key order, keeping the map shape
			// (mapValues). Quotation pushes the value; a lambda receives a
			// KeyVal {k v i n}. See native_map_iter.go.
			{Args: []*Type{TList, TMap}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(eachMapHandler), Returns: []*Type{TMap}, BarrierPos: -1},
			{Args: []*Type{TFunction, TMap}, Impl: Go(eachMapHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
	{
		// `data for-each [body]` — like `each` but discards every body
		// result and produces nothing. The body may push a value or leave
		// the stack empty (a None-producing / purely-mutating body is
		// fine), so index-less mutating loops no longer need to push a
		// throwaway sentinel just to satisfy each's "body produced a
		// result" rule. See §7.4 in the DX report.
		Name:          "for-each",
		CompileEffect: CompileFallbackBody,
		// The body compiles to a per-element closure exactly as each's does
		// (the forty-fourth increment). Three of each's flags are NOT set
		// here, and the differences are the word's own:
		//   - BodyOut 0, not 1: forEachHandler discards every invocation's
		//     result, so the unit declares no returns and RETs whatever the
		//     body nets — which is also why EmptyBodyErrors is absent (a
		//     0-net body is not an error here, it is the ordinary case).
		//   - BodyResultTop: the handler reads NOTHING of the residual, so
		//     a fortiori never below its top; unconsumed values (the element
		//     a body that ignores it leaves) may be dropped at the RET.
		//   - CrossCollectionTokenShape is NOT set: it licenses committing
		//     to the List overload for a statically-ambiguous collection
		//     because each's handler cross-delegates to the Map one at run
		//     time. forEachHandler does not — it reads args[1] as a list —
		//     so a gradual collection must keep the ambiguous-overload
		//     refusal rather than commit to a handler that would raise.
		//   - BodyMultiRunKeepsDefs: verified at the handler, which drives
		//     InvokeBody once per element on the shared registry with no def
		//     cleanup — the same seam and the same leak eachHandler has.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 0, BodyResultTop: true, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			return []Value{NewElementCarrier(DataListElemTypeFromValue(a[1]))}
		}},

		Signatures: []Signature{
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(forEachHandler),
				ReturnsFn:  forEachReturnsFn, BarrierPos: -1,
			},
			// LIST Function form (NUR086's fix, 2026-08-24): the same
			// handler, the same element — a Function value reaches the
			// callback exactly where a quotation body would, so
			// `each dbl/v [1 2 3]` is `each [dbl] [1 2 3]` without the
			// wrapper. The callback receives the ELEMENT: a per-container
			// form hands the container's natural unit (element for a list,
			// KeyVal for a map). `filter`'s single cross-container form is
			// the documented exception — it hands a position descriptor.
			{Args: []*Type{TFunction, TList}, Impl: Go(forEachHandler), ReturnsFn: forEachReturnsFn, BarrierPos: -1},
			// Map forms — iterate entries for side effects, produce nothing.
			{Args: []*Type{TList, TMap}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(forEachMapHandler), Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TFunction, TMap}, Impl: Go(forEachMapHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name:          "fold",
		CompileEffect: CompileFallbackBody,
		// fold [body] data init — the body sees (accumulator, element). InvokeBody
		// supplies [acc, elem]; acc generalises to the init's type, or (no-init
		// 2-arg form) to the element type, since the accumulator starts as the
		// first element. A 0-net body is fold's own fold_error, raised faithfully,
		// so EmptyBodyErrors keeps it native rather than islanding.
		// BodyMultiRunKeepsDefs: doFold drives InvokeBody once per element on
		// the shared registry with no def cleanup — the same seam and the same
		// leak eachHandler has (verified at the handler, not inferred from the
		// word's family), so a body def installs once per element with that
		// element's runtime value. Arm-residency's license; see core's field doc.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, CrossCollectionTokenShape: true, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			elem := DataListElemTypeFromValue(a[1])
			if len(a) >= 3 {
				return []Value{foldAccCarrier(a[2]), NewElementCarrier(elem)}
			}
			return []Value{NewElementCarrier(elem), NewElementCarrier(elem)}
		}},

		Signatures: []Signature{
			{
				// With initial value: init fold body data → result.
				// Sig is body-first (matching each/scan) so the infix form
				// `init fold body data` collects body+data forward and
				// init from the stack.
				Args:       []*Type{TList, TList, TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(foldWithInitHandler),
				ReturnsFn:  foldWithInitReturnsFn, BarrierPos: -1,
			},
			{
				// Without initial: body data → result (uses first element as init)
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(foldNoInitHandler),
				ReturnsFn:  foldNoInitReturnsFn, BarrierPos: -1,
			},
			// LIST Function form (NUR086's fix, 2026-08-24): the same
			// handler, the same element — a Function value reaches the
			// callback exactly where a quotation body would, so
			// `each dbl/v [1 2 3]` is `each [dbl] [1 2 3]` without the
			// wrapper. The callback receives the ELEMENT: a per-container
			// form hands the container's natural unit (element for a list,
			// KeyVal for a map). `filter`'s single cross-container form is
			// the documented exception — it hands a position descriptor.
			{Args: []*Type{TFunction, TList, TAny}, Impl: Go(foldWithInitHandler), ReturnsFn: foldWithInitReturnsFn, BarrierPos: -1},
			{Args: []*Type{TFunction, TList}, Impl: Go(foldNoInitHandler), ReturnsFn: foldNoInitReturnsFn, BarrierPos: -1},
			// Map forms — reduce entries (quotation: acc beneath, value on top;
			// lambda: (acc, KeyVal)). Seeded explicitly, or by the first value.
			// The accumulator-type inference is collection-agnostic
			// (DataListElemTypeFromValue reads a map's common value type), so the
			// list ReturnsFns narrow the map forms too — `fold [add] {map} 0`
			// checks as Integer, not Any.
			{Args: []*Type{TList, TMap, TAny}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(foldMapInitHandler), ReturnsFn: foldWithInitReturnsFn, BarrierPos: -1},
			{Args: []*Type{TList, TMap}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(foldMapNoInitHandler), ReturnsFn: foldNoInitReturnsFn, BarrierPos: -1},
			{Args: []*Type{TFunction, TMap, TAny}, Impl: Go(foldMapInitHandler), ReturnsFn: foldWithInitReturnsFn, BarrierPos: -1},
			{Args: []*Type{TFunction, TMap}, Impl: Go(foldMapNoInitHandler), ReturnsFn: foldNoInitReturnsFn, BarrierPos: -1},
		},
	},
	{
		Name:          "scan",
		CompileEffect: CompileFallbackBody,
		// scan [body] data — the body sees (accumulator, element); the accumulator
		// starts as the first element, so both inputs carry the element type. A
		// 0-net body is scan's own scan_error, raised faithfully, so EmptyBodyErrors
		// keeps it native rather than islanding.
		// BodyMultiRunKeepsDefs: scanHandler's own per-element loop calls
		// InvokeBody on the shared registry with no cleanup (handler-verified,
		// like each's) — one leaked install per element, per-element value.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, CrossCollectionTokenShape: true, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			e := DataListElemTypeFromValue(a[1])
			return []Value{NewElementCarrier(e), NewElementCarrier(e)}
		}},

		Signatures: []Signature{
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(scanHandler),
				ReturnsFn:  scanReturnsFn, BarrierPos: -1,
			},
			// LIST Function form (NUR086's fix, 2026-08-24): the same
			// handler, the same element — a Function value reaches the
			// callback exactly where a quotation body would, so
			// `each dbl/v [1 2 3]` is `each [dbl] [1 2 3]` without the
			// wrapper. The callback receives the ELEMENT: a per-container
			// form hands the container's natural unit (element for a list,
			// KeyVal for a map). `filter`'s single cross-container form is
			// the documented exception — it hands a position descriptor.
			{Args: []*Type{TFunction, TList}, Impl: Go(scanHandler), ReturnsFn: scanReturnsFn, BarrierPos: -1},
			// Map forms — running fold over a map's values (the first value
			// seeds), keeping the map shape. Quotation: acc beneath, value on
			// top; lambda: (acc, KeyVal).
			{Args: []*Type{TList, TMap}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(scanMapHandler), Returns: []*Type{TMap}, BarrierPos: -1},
			{Args: []*Type{TFunction, TMap}, Impl: Go(scanMapHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
	{
		Name:          "outer",
		CompileEffect: CompileFallbackBody,
		// outer [body] left right — the body sees (left[i], right[j]) for every
		// pair, producing a 2D list. The body compiles to a closure unit that
		// outerHandler drives per pair via InvokeBody; CompileFallbackBody is
		// the fallback when the body cannot compile.
		// BodyMultiRunKeepsDefs: that per-PAIR InvokeBody loop runs on the
		// shared registry with no cleanup (handler-verified), so a body def
		// leaks one install per pair — the multi-run population is the pair
		// grid rather than a flat element list, which changes the COUNT the
		// oracle measures but not the mechanism.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			le := DataListElemTypeFromValue(a[1])
			re := DataListElemTypeFromValue(a[2])
			return []Value{NewElementCarrier(le), NewElementCarrier(re)}
		}},

		Signatures: []Signature{{
			Args:       []*Type{TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(outerHandler),
			ReturnsFn:  outerReturnsFn, BarrierPos: -1,
		}},
	},
	{
		Name:          "inner",
		CompileEffect: CompileFallbackBody,

		Signatures: []Signature{{
			Args:       []*Type{TList, TList, TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Impl:       Go(innerHandler),
			ReturnsFn:  innerReturnsFn, BarrierPos: -1,
		}},
	},
	{
		// compress: mask-based selection. No code body, so it is a
		// boru:array module word like where/replicate.
		Name: "compress",

		Signatures: []Signature{{
			Args:      []*Type{TList, TList},
			Impl:      Go(compressHandler),
			ReturnsFn: ReturnsPreserveListAt(1), BarrierPos: -1,
		}},
	},
	{
		// eachrank: depth-targeted map — specialised APL/J vocabulary, so
		// it lives in the boru:array module. The quoted body is captured
		// via NoEvalArgs.
		Name: "eachrank",
		// One input per cell, and the cell's type is RANK-dependent: `eachrank 0`
		// sees each scalar leaf, `eachrank 1` each innermost list. Deriving it
		// would mean walking the data's spine by (depth - rank), and nothing
		// measured needs that precision — the gradual carrier is the honest
		// answer for a shape the call's own arguments decide.
		// No BodyMultiRunKeepsDefs, deliberately: eachrankHandler does drive
		// InvokeBody per cell with the same leak its siblings have, but the
		// word refuses EARLIER as a Stage-2 code-body word, so the flag would
		// never fire and nothing could measure it. Flag it when eachrank
		// itself graduates — an unmeasurable graduation is not one.
		// eachrank alone still carries the structural ReturnsPreserveListAt,
		// which never analyses its body — the hazard NUR115 named and foldaxis
		// discharged (foldaxisReturnsFn): a body the check pass never runs
		// records no bind twins, so the twin regime cannot see its defs. Safe
		// here ONLY because the early refusal sends the whole program to the
		// interpreter, the sound direction; the day eachrank graduates, an
		// analysing ReturnsFn comes first (with its parity-oracle rows), the
		// flag second.
		Callable: &CallableSpec{BodyPos: 1, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, Inputs: func(_ []Value) []Value {
			return []Value{NewElementCarrier(TAny)}
		}},

		Signatures: []Signature{{
			Args:       []*Type{TInteger, TList, TList},
			NoEvalArgs: map[int]bool{1: true},
			Impl:       Go(eachrankHandler),
			ReturnsFn:  ReturnsPreserveListAt(2), BarrierPos: -1,
		}},
	},
	{
		// foldaxis: axis reduction — specialised APL/J vocabulary, so it
		// lives in the boru:array module. The quoted body is captured via
		// NoEvalArgs.
		Name: "foldaxis",
		// The body is `fold`'s body one rank down: foldaxisHandler reduces each
		// LANE (a row, or a transposed column) through the same doFold, so each
		// step sees (accumulator, element) where both are elements of an INNER
		// list. Same spec as fold's therefore, with the inputs taken a level
		// deeper — and no handler change at all, because doFold already drives
		// the body through InvokeBody.
		// BodyMultiRunKeepsDefs: the same doFold `fold` carries the flag for —
		// InvokeBody once per lane element past the seed, on the shared
		// registry, no def cleanup — verified at the handler (foldaxisHandler
		// reduces every lane through doFold) and MEASURED by the parity
		// oracle's foldaxis rows: both axes, the elem-valued install order, the
		// one-element lanes that run the body ZERO times, the empty rank-2
		// list, and the read-after / nested-multi-run refusals.
		//
		// The flag alone was NOT enough, and that is the lesson NUR115 recorded
		// (resolved 2026-09-02): a body the check pass never RUNS records no
		// bind twins, and the twin regime's placement gate can only check twins
		// that exist. The structural ReturnsPreserveListAt this word used to
		// carry never called analyseHigherOrderBodyVals, so a def in a foldaxis
		// body installed twice on the interpreter and NOT AT ALL compiled —
		// silently, because nothing refused. foldaxisReturnsFn analyses the
		// body (fold's accumulator fixed point one rank down), which is what
		// makes the twins visible — and is the test for any new body word: if
		// its ReturnsFn does not analyse the body, the twin machinery cannot
		// see it, and no gate will say so.
		Callable: &CallableSpec{BodyPos: 1, BodyOut: 1, EmptyBodyErrors: true, BodyResultTop: true, BodyMultiRunKeepsDefs: true, Inputs: func(a []Value) []Value {
			return []Value{rank2ElemCarrier(a[2]), rank2ElemCarrier(a[2])}
		}},

		Signatures: []Signature{{
			Args:       []*Type{TInteger, TList, TList},
			NoEvalArgs: map[int]bool{1: true},
			Impl:       Go(foldaxisHandler),
			ReturnsFn:  foldaxisReturnsFn, BarrierPos: -1,
		}},
	},
}

// arrayCoreNames is the set of array words that remain built-in. The
// rest of allArrayNatives moves to the boru:array module. See the
// allArrayNatives comment for the rationale behind the split.
var arrayCoreNames = map[string]bool{
	"iota": true, "range": true,
	"each": true, "for-each": true, "fold": true, "scan": true, "outer": true, "inner": true,
	"take": true, "shed": true, "reverse": true,
}

// arrayNatives are the core array words registered globally (see
// register.go). ArrayModuleNatives are the specialised words that the
// boru:array module registers into its own sub-registry instead — they
// are NOT globally available, matching how boru:math gates sin/cos/etc.
var arrayNatives, ArrayModuleNatives = func() (core, module []NativeFunc) {
	for _, n := range allArrayNatives {
		if arrayCoreNames[n.Name] {
			core = append(core, n)
		} else {
			module = append(module, n)
		}
	}
	return core, module
}()

// ---- shared arg guards ----

// arrayWordErrCode derives an array word's pinned error code:
// "<word>_error" with hyphens underscored (insert-at → insert_at_error).
func arrayWordErrCode(word string) string {
	return strings.ReplaceAll(word, "-", "_") + "_error"
}

// requireListArg guards the recurring concrete-list-arg pattern shared by
// the array handlers: when v is a type literal or carrier it raises the
// word's own pinned error — code "<word>_error", detail
// "<word>: expected <what>" — then unwraps the list. The codes and details
// are pinned by the langspec ERROR rows, so this reproduces the historical
// hand-rolled format exactly; it deliberately does NOT delegate to eng's
// RequireConcreteList, whose message format differs.
func requireListArg(r *Registry, v Value, word, what string) (ReadList, error) {
	if !IsConcrete(v) {
		return ReadList{}, r.BoruError(arrayWordErrCode(word), word+": expected "+what, word)
	}
	list, _ := AsList(v)
	return list, nil
}

// requireConcreteArgs is the multi-value variant of requireListArg for the
// handlers that guard several args behind one combined error (e.g. each's
// "each: expected concrete lists" over body and data). Same pinned
// code/detail format; no unwrapping.
func requireConcreteArgs(r *Registry, word, what string, vals ...Value) error {
	for _, v := range vals {
		if !IsConcrete(v) {
			return r.BoruError(arrayWordErrCode(word), word+": expected "+what, word)
		}
	}
	return nil
}

// ---- shared ReturnsFn helpers ----

func returnsCarrierTypedListInteger(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TInteger)}
}

// returnsIotaLen returns a length-refined integer-list carrier: iota's
// result length is exactly its non-negative count argument, so a
// downstream static index check can reason about an `iota n` list (a
// computed list whose length would otherwise be unknown). Falls back to
// an unrefined carrier when the count isn't a concrete integer.
func returnsIotaLen(args []Value, _ *Registry) []Value {
	if len(args) >= 1 {
		if n, err := args[0].AsConcreteInteger(); err == nil && n >= 0 {
			return []Value{NewCarrierTypedListLen(TInteger, int(n))}
		}
	}
	return []Value{NewCarrierTypedList(TInteger)}
}

// returnsAtChecked runs the check-mode index-bounds check for `at` (the
// list of indices against the data list) and then delegates to the
// element-type-preserving return shape.
func returnsAtChecked(args []Value, r *Registry) []Value {
	if len(args) >= 2 {
		CheckAtIndices(r, args[0], args[1], "at")
	}
	return ReturnsPreserveListAt(1)(args, r)
}

func returnsCarrierTypedListBoolean(_ []Value, _ *Registry) []Value {
	return []Value{NewCarrierTypedList(TBoolean)}
}

// ---- iota ----

func iotaHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 {
		return nil, r.BoruError("iota_error", fmt.Sprintf("iota: negative count %d", n), "iota")
	}
	if n > maxArrayElems {
		return nil, r.BoruError("iota_error", fmt.Sprintf("iota: count %d exceeds the cap of %d", n, maxArrayElems), "iota")
	}
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = NewInteger(int64(i))
	}
	return []Value{NewList(elems)}, nil
}

// ---- range ----

func rangeThreeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	start, _ := args[0].AsConcreteInteger()
	stop, _ := args[1].AsConcreteInteger()
	step, _ := args[2].AsConcreteInteger()
	return buildRange(start, stop, step, r)
}

func rangeTwoHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	start, _ := args[0].AsConcreteInteger()
	stop, _ := args[1].AsConcreteInteger()
	return buildRange(start, stop, 1, r)
}

// buildRange produces the arithmetic sequence [start, start+step, ...)
// stopping before stop. A positive step counts up, a negative step
// counts down; a zero step is an error. The half-open convention makes
// `range 0 n 1` equal to `iota n`, and an empty range (start already
// past stop in the step direction) yields [].
func buildRange(start, stop, step int64, r *Registry) ([]Value, error) {
	if step == 0 {
		return nil, r.BoruError("range_error", "range: step must be non-zero", "range")
	}
	elems := []Value{}
	if step > 0 {
		for i := start; i < stop; i += step {
			elems = append(elems, NewInteger(i))
		}
	} else {
		for i := start; i > stop; i += step {
			elems = append(elems, NewInteger(i))
		}
	}
	return []Value{NewList(elems)}, nil
}

// ---- shape ----

func shapeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if _, err := requireListArg(r, args[0], "shape", "concrete list"); err != nil {
		return nil, err
	}
	dims := computeShape(args[0])
	elems := make([]Value, len(dims))
	for i, d := range dims {
		elems[i] = NewInteger(int64(d))
	}
	return []Value{NewList(elems)}, nil
}

func computeShape(v Value) []int {
	list, _ := AsList(v)
	if list.IsNil() {
		return nil
	}
	dims := []int{list.Len()}
	if list.Len() == 0 {
		return dims
	}
	first := list.Get(0)
	if !first.Parent.ConformsTo(TList) || !IsConcrete(first) {
		return dims
	}
	_lst, _ := AsList(first)
	firstLen := _lst.Len()
	for i := 1; i < list.Len(); i++ {
		sub := list.Get(i)
		_subLst, _ := AsList(sub)
		if !sub.Parent.ConformsTo(TList) || !IsConcrete(sub) || _subLst.Len() != firstLen {
			return dims
		}
	}
	return append(dims, computeShape(first)...)
}

// ---- rank ----

func rankHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if _, err := requireListArg(r, args[0], "rank", "concrete list"); err != nil {
		return nil, err
	}
	dims := computeShape(args[0])
	return []Value{NewInteger(int64(len(dims)))}, nil
}

// ---- reshape ----

// maxArrayElems caps a single dimension and the total element count a
// reshape (and iota) will allocate, bounding the memory one call can
// demand and keeping a crafted shape out of the panicking allocation
// path. ~16M Value entries.
const maxArrayElems = 1 << 24

func reshapeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	shapeList, err := requireListArg(r, args[0], "reshape", "concrete shape list")
	if err != nil {
		return nil, err
	}
	if _, err := requireListArg(r, args[1], "reshape", "concrete data list"); err != nil {
		return nil, err
	}
	dims := make([]int, shapeList.Len())
	for i := 0; i < shapeList.Len(); i++ {
		_as1, _ := AsInteger(shapeList.Get(i))
		dims[i] = int(_as1)
		if dims[i] < 0 {
			return nil, r.BoruError("reshape_error", fmt.Sprintf("reshape: negative dimension %d", dims[i]), "reshape")
		}
		// Bound each dimension. Without this, a single huge dim (e.g. 2^62)
		// drives buildNested's make([]Value, dims[0]) into a panicking /
		// OOMing allocation even when the overall product passes the
		// length check (e.g. [2^62, 0] over empty data). See ADR-005.
		if dims[i] > maxArrayElems {
			return nil, r.BoruError("reshape_error", fmt.Sprintf("reshape: dimension %d exceeds the cap of %d", dims[i], maxArrayElems), "reshape")
		}
	}
	flat := flattenList(args[1])
	// Compute the product with overflow detection: int64 wraparound used
	// to let a crafted shape (whose product overflows to len(flat)) pass
	// the check and then panic in buildNested's allocation.
	product := 1
	for _, d := range dims {
		if d == 0 {
			product = 0
			break
		}
		if product > maxArrayElems/d {
			return nil, fmt.Errorf("reshape: shape product exceeds the cap of %d", maxArrayElems)
		}
		product *= d
	}
	if product != len(flat) {
		return nil, fmt.Errorf("reshape: shape product %d does not match data length %d", product, len(flat))
	}
	result := buildNested(flat, dims)
	return []Value{result}, nil
}

func flattenList(v Value) []Value {
	list, _ := AsList(v)
	if list.IsNil() {
		return nil
	}
	var result []Value
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if elem.Parent.ConformsTo(TList) && elem.Data != nil {
			result = append(result, flattenList(elem)...)
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func buildNested(flat []Value, dims []int) Value {
	if len(dims) == 0 {
		if len(flat) == 1 {
			return flat[0]
		}
		return NewList(flat)
	}
	if len(dims) == 1 {
		return NewList(flat)
	}
	size := dims[0]
	subSize := 1
	for _, d := range dims[1:] {
		subSize *= d
	}
	elems := make([]Value, size)
	for i := 0; i < size; i++ {
		start := i * subSize
		end := start + subSize
		elems[i] = buildNested(flat[start:end], dims[1:])
	}
	return NewList(elems)
}

// ---- transpose ----

func arrTransposeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	outer, err := requireListArg(r, args[0], "transpose", "concrete list")
	if err != nil {
		return nil, err
	}
	if outer.Len() == 0 {
		return []Value{NewList(nil)}, nil
	}
	first := outer.Get(0)
	if !first.Parent.ConformsTo(TList) || !IsConcrete(first) {
		return nil, r.BoruError("transpose_error", "transpose: expected rank-2 list", "transpose")
	}
	_lst, _ := AsList(first)
	cols := _lst.Len()
	for i := 1; i < outer.Len(); i++ {
		sub := outer.Get(i)
		_subLst, _ := AsList(sub)
		if !sub.Parent.ConformsTo(TList) || !IsConcrete(sub) || _subLst.Len() != cols {
			return nil, r.BoruError("transpose_error", "transpose: expected rectangular rank-2 list", "transpose")
		}
	}
	rows := outer.Len()
	result := make([]Value, cols)
	for c := 0; c < cols; c++ {
		row := make([]Value, rows)
		for r := 0; r < rows; r++ {
			_rowLst, _ := AsList(outer.Get(r))
			row[r] = _rowLst.Get(c)
		}
		result[c] = NewList(row)
	}
	return []Value{NewList(result)}, nil
}

// ---- reverse ----

func reverseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "reverse", "concrete list")
	if err != nil {
		return nil, err
	}
	n := list.Len()
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		elems[i] = list.Get(n - 1 - i)
	}
	// #4 (round 3): reverse is a pure reorder — retain the source [:T] tag so
	// downstream reads narrow and downstream writes stay enforced.
	return []Value{d2RetainElem(NewList(elems), args[0])}, nil
}

// ---- take ----

func takeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[1], "take", "concrete list")
	if err != nil {
		return nil, err
	}
	_as2, _ := args[0].AsConcreteInteger()
	n := int(_as2)
	length := list.Len()
	var start, end int
	if n >= 0 {
		end = n
		if end > length {
			end = length
		}
		start = 0
	} else {
		abs := -n
		if abs > length {
			abs = length
		}
		start = length - abs
		end = length
	}
	elems := make([]Value, end-start)
	for i := start; i < end; i++ {
		elems[i-start] = list.Get(i)
	}
	// #4 (round 3): take selects a contiguous subset — element types are
	// preserved, so retain the source list's [:T] tag.
	return []Value{d2RetainElem(NewList(elems), args[1])}, nil
}

// ---- shed ----

func shedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[1], "shed", "concrete list")
	if err != nil {
		return nil, err
	}
	_as3, _ := args[0].AsConcreteInteger()
	n := int(_as3)
	length := list.Len()
	var start, end int
	if n >= 0 {
		start = n
		if start > length {
			start = length
		}
		end = length
	} else {
		abs := -n
		if abs > length {
			abs = length
		}
		start = 0
		end = length - abs
	}
	elems := make([]Value, end-start)
	for i := start; i < end; i++ {
		elems[i-start] = list.Get(i)
	}
	// #4 (round 3): shed drops a prefix/suffix — the remaining elements keep
	// their type, so retain the source list's [:T] tag.
	return []Value{d2RetainElem(NewList(elems), args[1])}, nil
}

// ---- where ----

func whereHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "where", "concrete list")
	if err != nil {
		return nil, err
	}
	var result []Value
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if CoerceBoolean(elem) {
			result = append(result, NewInteger(int64(i)))
		}
	}
	if result == nil {
		result = []Value{}
	}
	return []Value{NewList(result)}, nil
}

// ---- unique ----

func uniqueHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "unique", "concrete list")
	if err != nil {
		return nil, err
	}
	// Dedup by deq — the language's VALUE equality (scalars by value,
	// where eq and deq coincide; Nodes/Ideals by deep value, where eq is
	// reference identity). Keying on the rendered form was a THIRD
	// equality notion agreeing with neither: `unique [1 1.0]` kept both
	// (deq-equal, renders differ) while `unique [["a"] ["a"]]` merged
	// values eq distinguishes (NUR015). The DeqIndex holds the kept
	// elements so the dup probe stays near-linear.
	var result []Value
	var kept DeqIndex
	for i := 0; i < list.Len(); i++ {
		elem := list.Get(i)
		if kept.FirstMatch(elem) < 0 {
			kept.Add(elem)
			result = append(result, elem)
		}
	}
	if result == nil {
		result = []Value{}
	}
	// #4 (round 3): unique is a dedup subset — elements are unchanged, so
	// retain the source list's [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[0])}, nil
}

// ---- grade ----

func gradeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "grade", "concrete list")
	if err != nil {
		return nil, err
	}
	n := list.Len()
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = i
	}
	// Stable, like every sorting word (NUR016): equal-comparing elements
	// grade in input order, so `grade` agrees with `sort`'s tie handling.
	sort.SliceStable(indices, func(a, b int) bool {
		va := list.Get(indices[a])
		vb := list.Get(indices[b])
		return arrCompareValues(va, vb) < 0
	})
	elems := make([]Value, n)
	for i, idx := range indices {
		elems[i] = NewInteger(int64(idx))
	}
	return []Value{NewList(elems)}, nil
}

// arrCompareValues is a non-error variant of CompareValues for use in sort functions.
// Falls back to string comparison if CompareValues returns an error.
func arrCompareValues(a, b Value) int {
	cmp, err := CompareValues(a, b)
	if err != nil {
		// Fallback: compare string representations
		as, bs := a.String(), b.String()
		if as < bs {
			return -1
		}
		if as > bs {
			return 1
		}
		return 0
	}
	return cmp
}

// ---- at ----

func atHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	indices, err := requireListArg(r, args[0], "at", "concrete indices list")
	if err != nil {
		return nil, err
	}
	data, err := requireListArg(r, args[1], "at", "concrete data list")
	if err != nil {
		return nil, err
	}
	dataLen := data.Len()
	result := make([]Value, indices.Len())
	for i := 0; i < indices.Len(); i++ {
		_as4, _ := AsInteger(indices.Get(i))
		idx := int(_as4)
		if idx < 0 || idx >= dataLen {
			return nil, r.BoruError("at_error", fmt.Sprintf("at: index %d out of bounds (length %d)", idx, dataLen), "at")
		}
		result[i] = data.Get(idx)
	}
	// #4: `at` gathers a subset of the data list's elements — retain its [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// ---- insert-at / remove-at ----

// insertAtHandler returns a new list with the element (args[1]) inserted
// at index args[0] of the list args[2]. Index 0..len is valid — index ==
// len appends. An out-of-range index is a loud error: these are edits,
// not lookups, so the silent None-for-missing convention of `get` does
// not apply.
func insertAtHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_idx, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("insert_at_error", "insert-at: expected a concrete Integer index", "insert-at")
	}
	data, err := requireListArg(r, args[2], "insert-at", "a concrete data list")
	if err != nil {
		return nil, err
	}
	idx := int(_idx)
	n := data.Len()
	if idx < 0 || idx > n {
		return nil, r.BoruError("index_out_of_range", fmt.Sprintf("insert-at: index %d out of range for list of length %d (valid: 0..%d)", idx, n, n), "insert-at")
	}
	// insert-at adds args[1] as a NEW element — enforce it against the data's
	// [:T] element type (like push/set), storing the recursively-retagged value,
	// and retain the tag on the result copy (#4, Codex round 4).
	tagged, terr := d2AdoptTyped(r, args[2], args[1], "insert-at")
	if terr != nil {
		return nil, terr
	}
	result := make([]Value, 0, n+1)
	for i := 0; i < idx; i++ {
		result = append(result, data.Get(i))
	}
	result = append(result, tagged)
	for i := idx; i < n; i++ {
		result = append(result, data.Get(i))
	}
	return []Value{d2RetainElem(NewList(result), args[2])}, nil
}

// removeAtHandler returns a new list with the element at index args[0]
// of the list args[1] removed. Valid indices are 0..len-1; out of range
// is a loud error (same rationale as insert-at).
func removeAtHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_idx, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("remove_at_error", "remove-at: expected a concrete Integer index", "remove-at")
	}
	data, err := requireListArg(r, args[1], "remove-at", "a concrete data list")
	if err != nil {
		return nil, err
	}
	idx := int(_idx)
	n := data.Len()
	if n == 0 {
		return nil, r.BoruError("index_out_of_range", "remove-at: cannot remove from an empty list", "remove-at")
	}
	if idx < 0 || idx >= n {
		return nil, r.BoruError("index_out_of_range", fmt.Sprintf("remove-at: index %d out of range for list of length %d (valid: 0..%d)", idx, n, n-1), "remove-at")
	}
	result := make([]Value, 0, n-1)
	for i := 0; i < n; i++ {
		if i == idx {
			continue
		}
		result = append(result, data.Get(i))
	}
	// #4 (Codex round 4): remove-at is a subset — retain the source [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// ---- sortby ----

func sortbyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	keys, err := requireListArg(r, args[0], "sortby", "concrete keys list")
	if err != nil {
		return nil, err
	}
	data, err := requireListArg(r, args[1], "sortby", "concrete data list")
	if err != nil {
		return nil, err
	}
	if keys.Len() != data.Len() {
		return nil, fmt.Errorf("sortby: keys length %d does not match data length %d", keys.Len(), data.Len())
	}
	n := keys.Len()
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = i
	}
	// Stable, like the reach-form sortby and sort itself (NUR016):
	// equal keys keep their data elements in input order.
	sort.SliceStable(indices, func(a, b int) bool {
		return arrCompareValues(keys.Get(indices[a]), keys.Get(indices[b])) < 0
	})
	result := make([]Value, n)
	for i, idx := range indices {
		result[i] = data.Get(idx)
	}
	// #4 (round 3): sortby reorders the data list — retain its [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// ---- member ----

func memberHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	needles, err := requireListArg(r, args[0], "member", "concrete needles list")
	if err != nil {
		return nil, err
	}
	haystack, err := requireListArg(r, args[1], "member", "concrete haystack list")
	if err != nil {
		return nil, err
	}
	// Membership is deq — the value equality (NUR015; it was keyed on
	// the rendered form). The haystack is indexed once so each needle
	// probes only its own deq bucket.
	var idx DeqIndex
	for j := 0; j < haystack.Len(); j++ {
		idx.Add(haystack.Get(j))
	}
	result := make([]Value, needles.Len())
	for i := 0; i < needles.Len(); i++ {
		result[i] = NewBoolean(idx.FirstMatch(needles.Get(i)) >= 0)
	}
	return []Value{NewList(result)}, nil
}

// ---- indices ----

// indicesHandler backs ArrayUtil.indices (boru:array-util): for each
// needle in the first list, its index in the second (haystack) list, or
// -1 when absent. Vectorised lookup — returns a list of indices, one per
// needle. The first match wins for duplicate haystack entries.
func indicesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	needles, err := requireListArg(r, args[0], "indices", "concrete needles list")
	if err != nil {
		return nil, err
	}
	haystack, err := requireListArg(r, args[1], "indices", "concrete haystack list")
	if err != nil {
		return nil, err
	}
	// Lookup is deq — the value equality (NUR015); first match wins.
	// DeqIndex.FirstMatch returns the earliest insertion position, so
	// indexing the haystack once preserves the first-match rule.
	var idx DeqIndex
	for j := 0; j < haystack.Len(); j++ {
		idx.Add(haystack.Get(j))
	}
	result := make([]Value, needles.Len())
	for i := 0; i < needles.Len(); i++ {
		result[i] = NewInteger(int64(idx.FirstMatch(needles.Get(i))))
	}
	return []Value{NewList(result)}, nil
}

// ---- group ----

// deqGrouper accumulates grouping classes for the group word over
// STRING keys only (NUR030, resolved 2026-08-15).
//
// The history is worth keeping, because the current shape is the answer
// to it. Grouping was once keyed on the rendered form — a third equality
// notion alongside eq and deq (NUR015) — and was then moved to deq
// classes with the output map's keys still taken from each class's FIRST
// occurrence's render. That left a lossy step: a Map key IS a string, so
// two deq-DISTINCT keys that rendered identically shared one entry
// (`group [Integer Integer/q]` → `{Integer:[0 1]}`), and the same fold
// silently absorbed the NON-REFLEXIVE keys — `nan`, and everything
// reaching DeepEqual's fall-through (NUR031).
//
// Restricting the key domain removes the step rather than judging it
// benign: a String key IS its render, so no two distinct keys can
// collide and the divergence cannot arise. The map key is the string's
// CONTENT (AsString), not `Value.String()` — a refined String
// (`def S (refine String)`) renders with its quotes, so keying on the
// render would reintroduce a spelling the content never had.
//
// Two costs are deliberate, and the refusal below is what makes them
// visible rather than surprising:
//
//   - the 1-arg form loses generality — `group [1 2 3]` now needs a
//     conversion first;
//   - NaN totality changes character — `nan` cannot be a key at all, so
//     a non-reflexive key is FORBIDDEN rather than folded.
type deqGrouper struct {
	reps    DeqIndex       // class representatives, in creation order
	rkIndex map[string]int // key content → class position
	repKeys []string
	groups  [][]Value
}

// groupKeyContent returns key's string content, or an error naming the
// String requirement. A bare `signature_error` would leave a user
// guessing which argument was wrong and why, so the message says the
// word, the offending type and the fix.
func groupKeyContent(r *Registry, key Value) (string, error) {
	if !key.Parent.ConformsTo(TString) {
		return "", r.BoruError(
			arrayWordErrCode("group"),
			"group: grouping keys must be Strings; got "+TypeNameOf(key)+
				" — convert the key first (a Map key is a string, so a"+
				" non-String key would have to be rendered, and two"+
				" distinct keys can render alike)",
			"group",
		)
	}
	s, _ := AsString(key)
	return s, nil
}

func (g *deqGrouper) add(r *Registry, key, v Value) error {
	content, err := groupKeyContent(r, key)
	if err != nil {
		return err
	}
	// Two Strings with the same content are deq-equal (a refined String
	// is deq to its plain twin), so content identity IS class identity
	// and one map lookup decides membership. The DeqIndex is still the
	// class-representative store, keeping creation order stable.
	if ri, ok := g.rkIndex[content]; ok {
		g.groups[ri] = append(g.groups[ri], v)
		return nil
	}
	if g.rkIndex == nil {
		g.rkIndex = map[string]int{}
	}
	g.rkIndex[content] = g.reps.Add(key)
	g.repKeys = append(g.repKeys, content)
	g.groups = append(g.groups, []Value{v})
	return nil
}

func (g *deqGrouper) toMap() Value {
	om := NewOrderedMap()
	for ri := range g.repKeys {
		om.Set(g.repKeys[ri], NewList(g.groups[ri]))
	}
	return NewMap(om)
}

func groupTwoHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	keys, err := requireListArg(r, args[0], "group", "concrete keys list")
	if err != nil {
		return nil, err
	}
	values, err := requireListArg(r, args[1], "group", "concrete values list")
	if err != nil {
		return nil, err
	}
	if keys.Len() != values.Len() {
		return nil, fmt.Errorf("group: keys length %d does not match values length %d", keys.Len(), values.Len())
	}
	var g deqGrouper
	for i := 0; i < keys.Len(); i++ {
		if err := g.add(r, keys.Get(i), values.Get(i)); err != nil {
			return nil, err
		}
	}
	return []Value{g.toMap()}, nil
}

func groupOneHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "group", "concrete list")
	if err != nil {
		return nil, err
	}
	var g deqGrouper
	for i := 0; i < list.Len(); i++ {
		if err := g.add(r, list.Get(i), NewInteger(int64(i))); err != nil {
			return nil, err
		}
	}
	return []Value{g.toMap()}, nil
}

// ---- replicate ----

func replicateHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	counts, err := requireListArg(r, args[0], "replicate", "concrete counts list")
	if err != nil {
		return nil, err
	}
	data, err := requireListArg(r, args[1], "replicate", "concrete data list")
	if err != nil {
		return nil, err
	}
	if counts.Len() != data.Len() {
		return nil, fmt.Errorf("replicate: counts length %d does not match data length %d", counts.Len(), data.Len())
	}
	var result []Value
	for i := 0; i < counts.Len(); i++ {
		_as5, _ := AsInteger(counts.Get(i))
		c := int(_as5)
		if c < 0 {
			return nil, r.BoruError("replicate_error", fmt.Sprintf("replicate: negative count %d at index %d", c, i), "replicate")
		}
		elem := data.Get(i)
		for j := 0; j < c; j++ {
			result = append(result, elem)
		}
	}
	if result == nil {
		result = []Value{}
	}
	// #4: replicate repeats each source element — all elements come from the
	// data list unchanged, so retain its [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// ---- expand ----

func expandHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	mask, err := requireListArg(r, args[0], "expand", "concrete mask list")
	if err != nil {
		return nil, err
	}
	data, err := requireListArg(r, args[1], "expand", "concrete data list")
	if err != nil {
		return nil, err
	}
	result := make([]Value, mask.Len())
	dataIdx := 0
	for i := 0; i < mask.Len(); i++ {
		if CoerceBoolean(mask.Get(i)) {
			if dataIdx >= data.Len() {
				return nil, r.BoruError("expand_error", "expand: not enough data elements for mask", "expand")
			}
			result[i] = data.Get(dataIdx)
			dataIdx++
		} else {
			result[i] = NewInteger(0)
		}
	}
	return []Value{NewList(result)}, nil
}

// ---- window ----

func windowHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[1], "window", "concrete list")
	if err != nil {
		return nil, err
	}
	_as6, _ := args[0].AsConcreteInteger()
	size := int(_as6)
	length := list.Len()
	if size <= 0 {
		return nil, r.BoruError("window_error", fmt.Sprintf("window: size must be positive, got %d", size), "window")
	}
	if size > length {
		return []Value{NewList([]Value{})}, nil
	}
	windows := make([]Value, length-size+1)
	for i := 0; i <= length-size; i++ {
		win := make([]Value, size)
		for j := 0; j < size; j++ {
			win[j] = list.Get(i + j)
		}
		windows[i] = NewList(win)
	}
	return []Value{NewList(windows)}, nil
}

// window yields a TList<TList<sameElem>>: wrap the source-data
// element carrier twice.
func windowReturnsFn(args []Value, _ *Registry) []Value {
	elem := DataListElemTypeFromValue(args[1])
	inner := NewCarrierTypedList(elem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- pairs ----

func pairsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list, err := requireListArg(r, args[0], "pairs", "concrete list")
	if err != nil {
		return nil, err
	}
	length := list.Len()
	if length < 2 {
		return []Value{NewList([]Value{})}, nil
	}
	result := make([]Value, length-1)
	for i := 0; i < length-1; i++ {
		pair := []Value{list.Get(i), list.Get(i + 1)}
		result[i] = NewList(pair)
	}
	return []Value{NewList(result)}, nil
}

// pairs yields TList<TList<sameElem>> (2-tuples).
func pairsReturnsFn(args []Value, _ *Registry) []Value {
	elem := DataListElemTypeFromValue(args[0])
	inner := NewCarrierTypedList(elem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- each ----

// collIsConcreteMap / collIsConcreteList report that the runtime collection is a
// concrete Map / List — the SIBLING shape of the overload the compiler committed
// for a gradual-Any (Dynamic) `each`/`fold`/`scan`. The recorder optimistically
// bakes the first-reachable (List) overload; when the value turns out to be the
// other shape at run time, the committed handler delegates to the sibling handler
// so the SAME compiled closure drives it, matching the interpreter (which picks
// the overload by the runtime type). Unreachable in the interpreter: matchSignature
// gates a Map away from a TList sig and vice-versa, so these only redirect on the
// compiled committed-overload path. See CallableSpec.CrossCollectionTokenShape.
func collIsConcreteMap(v Value) bool  { return IsConcrete(v) && v.Parent.ConformsTo(TMap) }
func collIsConcreteList(v Value) bool { return IsConcrete(v) && v.Parent.ConformsTo(TList) }

func eachHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteMap(args[1]) {
		return eachMapHandler(args, nil, nil, reg)
	}
	if err := requireConcreteArgs(reg, "each", "concrete lists", args[0], args[1]); err != nil {
		return nil, err
	}
	dataList, _ := AsList(args[1])

	results := make([]Value, dataList.Len())
	for i := 0; i < dataList.Len(); i++ {
		elem := dataList.Get(i)
		res, err := InvokeBody(reg, args[0], []Value{elem})
		if err != nil {
			return nil, fmt.Errorf("each: element %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, reg.BoruError("each_error", fmt.Sprintf("each: element %d: body produced no result", i), "each")
		}
		results[i] = res[len(res)-1] // take top of stack
	}
	return []Value{NewList(results)}, nil
}

// eachReachHandler is the lens form of each: it applies a receiverless Reach
// (args[0]) to every element of the data list (args[1]) — `each $.name people`
// → the list of names. The reach acts as an arity-1 accessor function.
func eachReachHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("each: %w", err)
	}
	data, err := requireListArg(reg, args[1], "each", "a concrete data list")
	if err != nil {
		return nil, err
	}
	results := make([]Value, data.Len())
	for i := 0; i < data.Len(); i++ {
		v, err := ApplyReach(reg, info, data.Get(i))
		if err != nil {
			return nil, fmt.Errorf("each: element %d: %w", i, err)
		}
		results[i] = v
	}
	return []Value{NewList(results)}, nil
}

// sortbyReachHandler is the lens form of sortby: it derives a sort key from
// each element via the Reach (args[0]), then returns the data list (args[1])
// sorted by those keys — `sortby $.age people`.
func sortbyReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("sortby: %w", err)
	}
	data, err := requireListArg(r, args[1], "sortby", "a concrete data list")
	if err != nil {
		return nil, err
	}
	n := data.Len()
	keys := make([]Value, n)
	for i := 0; i < n; i++ {
		k, err := ApplyReach(r, info, data.Get(i))
		if err != nil {
			return nil, fmt.Errorf("sortby: element %d: %w", i, err)
		}
		keys[i] = k
	}
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return arrCompareValues(keys[indices[a]], keys[indices[b]]) < 0
	})
	result := make([]Value, n)
	for i, idx := range indices {
		result[i] = data.Get(idx)
	}
	// #4 (round 3): sortby (reach form) reorders the data list — retain its [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// forEachHandler runs the body once per data element for its side effects,
// discarding each body result and producing nothing. Unlike eachHandler
// it does NOT error when the body leaves the stack empty.
func forEachHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if err := requireConcreteArgs(reg, "for-each", "concrete lists", args[0], args[1]); err != nil {
		return nil, err
	}
	dataList, _ := AsList(args[1])

	for i := 0; i < dataList.Len(); i++ {
		elem := dataList.Get(i)
		if _, err := InvokeBody(reg, args[0], []Value{elem}); err != nil {
			return nil, fmt.Errorf("for-each: element %d: %w", i, err)
		}
	}
	return nil, nil
}

// forEachReturnsFn runs the body once in check mode (for its diagnostics)
// and reports that for-each produces nothing.
func forEachReturnsFn(args []Value, r *Registry) []Value {
	analyseHigherOrderBodyVals(r, args[0], ElementCarrierFromValue(args[1]))
	return nil
}

// each returns a list whose element type is whatever the body's
// top-of-stack produces. Pass the concrete data list's element
// carrier into the body so diagnostics fire against realistic types.
func eachReturnsFn(args []Value, r *Registry) []Value {
	stk := analyseHigherOrderBodyVals(r, args[0], ElementCarrierFromValue(args[1]))
	if len(stk) == 0 {
		return []Value{NewCarrier(TList)}
	}
	return []Value{NewCarrierTypedList(stk[len(stk)-1].Parent)}
}

// analyseHigherOrderBody runs a literal code-body list through a
// sub-engine in check mode, prepending the given element carrier
// type(s) so body words see realistic input carriers. Returns the
// residual carrier stack, or nil if the body is not concrete. The
// primary purpose is side-effect: any diagnostics the body produces
// (type mismatches, undefined words) are accumulated on the registry.
func analyseHigherOrderBody(r *Registry, body Value, elems ...*Type) []Value {
	vals := make([]Value, len(elems))
	for i, t := range elems {
		// An UNTYPED element (TAny — an untyped list like `Test.results end`'s
		// declared `[List]`) rides as a DYNAMIC carrier so a body access (`get`,
		// a field read) matches optimistically rather than failing no_signature
		// against the bare Any root. A known element type stays strict.
		vals[i] = NewElementCarrier(t)
	}
	return analyseHigherOrderBodyVals(r, body, vals...)
}

// analyseHigherOrderBodyVals is the carrier-Value variant of
// analyseHigherOrderBody: the prefix inputs are arbitrary carriers
// (disjuncts, typed lists), not just bare types. Used by the fold
// accumulator fixed point, whose accumulator may widen to a disjunct
// between rounds.
func analyseHigherOrderBodyVals(r *Registry, body Value, vals ...Value) []Value {
	// Higher-order bodies run nested sub-engines — pause bytecode
	// recording for their duration (they are not part of the enclosing
	// straight line). Through the MULTI-RUN GUARD, which delegates to the
	// analysis guard — inside a keep-defs bracket (a `do` body run) the
	// sub-run's bind twins still taint, so the twin regime's do-body
	// adoption never places a single replay for a transition a multi-run
	// body performs per element at runtime (`do [[1 2] each [def x 5]]` —
	// Codex P1 on #421) — and additionally latches this run's twin-table
	// range keyed by the body's identity, the seat the arm-residency
	// bridge pairs against the compiled per-invocation unit's def sites.
	defer r.Check.Recorder().MultiRunBodyGuard(r, body.ID)()
	if !IsConcrete(body) {
		return nil
	}
	bodyList, _ := AsList(body)
	if bodyList.IsNil() {
		return nil
	}
	input := make([]Value, 0, len(vals)+bodyList.Len())
	input = append(input, vals...)
	input = append(input, bodyList.Slice()...)
	sub := New(r)
	// A higher-order body (each/fold/scan/outer/inner/…) is CONDITIONALLY
	// reached — the collection may be empty, so the body runs zero times.
	// Raise CondBodyDepth (as the branch/loop bodies do) so an in-place fn
	// redefinition that clobbers an enclosing overload here refuses to compile
	// (installDef): compiled resolution would bake the body's shadow while the
	// interpreter keeps the outer fn on an empty collection. Balanced around
	// the body run; Suspend (above) stops recording but still lets the refusal
	// latch the program's compilability.
	r.Check.CondBodyDepth++
	// The body is a speculative region (zero iterations possible): an
	// `undef` of an enclosing binding inside it must not leak the deletion
	// into the pass model (SpecUndefBlocked — the wrapped-undef FP class).
	r.Check.PushSpecBaseline(r.Defs.Snapshot())
	result, err := sub.Run(input)
	r.Check.PopSpecBaseline()
	r.Check.CondBodyDepth--
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "body_error",
			Detail: "higher-order body analysis error: " + err.Error(),
		})
		return nil
	}
	return result
}

// foldAccumFixedPoint iterates the fold/scan body analysis until the
// accumulator type stabilises (design/checker-accuracy-review.10.md
// A4): a body like [add 0.5] widens an Integer accumulator to Float
// on the first round, and the second round must see the widened
// accumulator or downstream consumers type against the init only.
// Bounded by the same round count as AnalyseLoopBody; only the final
// round's diagnostics are kept. Returns the stabilised accumulator
// carrier, or ok=false when the body is not analysable.
func foldAccumFixedPoint(r *Registry, body Value, initAcc Value, elemCarrier Value) (Value, bool) {
	// Normalise the seed to a container-faithful carrier. A List/Map SUBTYPE seed
	// (`[]` / `[9]`) reaches here either concrete or already stripped to a carrier
	// whose Data==nil — both drop the load-bearing ChildTypeInfo, so a body word
	// needing a concrete container (`push`/`set`/`append`, declared [Any List])
	// wrongly fails no_signature. foldAccCarrier rebuilds the typed-list / map
	// carrier from the seed's element type; a scalar seed is unchanged.
	acc := foldAccCarrier(initAcc)
	// The element rides as the caller-built carrier (ElementCarrierFromValue):
	// an UNTYPED element is a DYNAMIC Any so a body word over it matches
	// optimistically; a heterogeneous concrete list is a strict Disjunct so
	// the body dispatch distributes per alternative.
	diagBase := len(r.Check.Diagnostics)
	for round := 0; ; round++ {
		r.Check.TruncateDiagnostics(diagBase)
		stk := analyseHigherOrderBodyVals(r, body, acc, elemCarrier)
		if len(stk) == 0 {
			return Value{}, false
		}
		joined := JoinCarriers(acc, stk[len(stk)-1])
		if ValuesEqual(joined, acc) || round >= 2 {
			return joined, true
		}
		acc = joined
	}
}

// ---- fold ----

// foldAccCarrier builds the check-mode accumulator carrier from fold's seed.
// A bare NewCarrier(init.Parent) is wrong for a LIST or MAP seed: when the
// seed's type is a List/Map SUBTYPE (a typed or empty list, e.g. `[]` / `[9]`),
// NewCarrier attaches the load-bearing ChildTypeInfo only for the exact TList /
// TMap nodes, so the carrier has Data==nil and fails a body word that needs a
// concrete container (`push`/`set`/`append` declare [Any List]). Preserve the
// container shape: a typed-list carrier for a list seed (keeping the element
// type), an Any-mapped carrier for a map seed, and the plain Parent carrier for
// a scalar seed (`0 fold [add]`, unchanged).
func foldAccCarrier(init Value) Value {
	var out Value
	switch {
	case init.Parent.ConformsTo(TList):
		out = NewCarrierTypedList(DataListElemTypeFromValue(init))
	case init.Parent.ConformsTo(TMap):
		out = NewCarrier(TMap)
	default:
		out = NewCarrier(init.Parent)
	}
	// A gradual (dynamic) seed yields a gradual accumulator: a seed of
	// statically-unknown type (`(do {…})`, a get-result threaded as the init)
	// is "type unknown", so a body word over the accumulator must poly-match
	// optimistically rather than fail no_signature against a strict Any. Without
	// this, `(do {n:0}) … [acc … get] fold` typed the accumulator strict Any and
	// every `acc … get`/`add` in the body errored (radix's common-prefix-len).
	if init.Dynamic {
		out.Carrier = true
		out.Dynamic = true
	}
	return out
}

func foldWithInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	// Sig is [TList, TList, TAny]: args[0]=body, args[1]=data, args[2]=init.
	if collIsConcreteMap(args[1]) {
		return foldMapInitHandler(args, nil, nil, reg)
	}
	if err := requireConcreteArgs(reg, "fold", "concrete lists", args[0], args[1]); err != nil {
		return nil, err
	}
	dataList, _ := AsList(args[1])
	init := args[2]
	return doFold(reg, init, args[0], dataList)
}

// Fold result type is the stabilised accumulator: the body is
// analysed with (accumulator, element) carriers and iterated to a
// bounded fixed point (foldAccumFixedPoint) so a body that widens
// its accumulator types correctly. The join with the init covers the
// empty-list case (result IS the init).
func foldWithInitReturnsFn(args []Value, r *Registry) []Value {
	acc, ok := foldAccumFixedPoint(r, args[0], args[2], ElementCarrierFromValue(args[1]))
	if !ok {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{acc}
}

func foldNoInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteMap(args[1]) {
		return foldMapNoInitHandler(args, nil, nil, reg)
	}
	if err := requireConcreteArgs(reg, "fold", "concrete lists", args[0], args[1]); err != nil {
		return nil, err
	}
	dataList, _ := AsList(args[1])
	if dataList.Len() == 0 {
		return nil, reg.BoruError("fold_error", "fold: empty list with no initial value", "fold")
	}
	init := dataList.Get(0)
	// Create a sub-list from element 1 onwards
	rest := make([]Value, dataList.Len()-1)
	for i := 1; i < dataList.Len(); i++ {
		rest[i-1] = dataList.Get(i)
	}
	restList := NewReadList(rest)
	return doFold(reg, init, args[0], restList)
}

// No init — accumulator type and element type both come from the
// data list; same bounded fixed point as the init form.
func foldNoInitReturnsFn(args []Value, r *Registry) []Value {
	// A statically-EMPTY collection with no initial value is fold's own
	// GUARANTEED runtime error (the accumulator has nothing to seed from) —
	// flag it here with the byte-identical runtime message. Before the
	// element carriers became join-aware these rows were flagged only
	// coincidentally (the strict-Any accumulator failed the body's
	// dispatch as a no_signature on the body word).
	// Routed through the stamping helper (NUR058): staticEmptyFoldDetail
	// fires only over an exactly-known (concrete, statically-empty)
	// collection, so the finding is a RuntimeMirror — the program compiles
	// and fold's own handler raises the byte-identical error — and the
	// dedupe absorbs re-analysis under repeated call shapes.
	if emptyDetail := staticEmptyFoldDetail(args[1]); emptyDetail != "" {
		core.CheckAddUniqueDiagnostic(r, "fold_error", emptyDetail, "fold", args[1].Pos())
		return []Value{NewCarrier(TAny)}
	}
	elemC := ElementCarrierFromValue(args[1])
	acc, ok := foldAccumFixedPoint(r, args[0], elemC, elemC)
	if !ok {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{acc}
}

// staticEmptyFoldDetail returns the runtime error text a no-init fold raises
// over the given collection when it is STATICALLY empty, or "" when the
// collection is non-empty or its size is unknown (a carrier).
func staticEmptyFoldDetail(coll Value) string {
	if n, ok := StaticListLen(coll); ok && n == 0 {
		return "fold: empty list with no initial value"
	}
	if IsConcrete(coll) && coll.Parent.ConformsTo(TMap) {
		if m, _ := AsMap(coll); m != nil && m.Len() == 0 {
			return "fold: empty map with no initial value"
		}
	}
	return ""
}

// rank2ElemCarrier is the per-step carrier a foldaxis body sees: the element
// carrier one rank BELOW a rank-2 data argument, joined over EVERY element of
// EVERY row. A lane is a row (axis 1) or a column (axis 0), so a step's
// (accumulator, element) can come from any row — reading one row, which the
// first-row `rank2ElemType` this replaces did, typed a body over
// `[[1 2] ["a" "b"]]` as Integer and BAKED that overload into the compiled
// body and, once the ReturnsFn analysed the body, into its consumers
// (measured 2026-09-02, found in review: `foldaxis 1 [add] …` answered
// `[3 0]` compiled against the interpreter's `[3 'ab']`). Rectangular means
// equal LENGTH, not equal type. The join is scan's own
// (ElementCarrierFromValue over the flattened elements): a mixed population
// is a strict Disjunct the body dispatch distributes per alternative, a
// uniform one its plain type. A data argument the check pass cannot open —
// non-concrete, a non-list row, or no element at all — is the gradual Any
// carrier: the honest answer when there is no element to take a type from.
func rank2ElemCarrier(data Value) Value {
	rows, err := AsList(data)
	if err != nil || !IsConcrete(data) {
		return NewElementCarrier(TAny)
	}
	var elems []Value
	for i := 0; i < rows.Len(); i++ {
		row := rows.Get(i)
		if !row.Parent.ConformsTo(TList) || !IsConcrete(row) {
			return NewElementCarrier(TAny)
		}
		rl, _ := AsList(row)
		elems = append(elems, rl.Slice()...)
	}
	if len(elems) == 0 {
		return NewElementCarrier(TAny)
	}
	return ElementCarrierFromValue(NewList(elems))
}

// foldaxisReturnsFn is fold's accumulator fixed point one rank down. Each
// lane seeds its accumulator from its own first element and folds the rest,
// so the body is analysed with (lane element, lane element) —
// rank2ElemCarrier for both, exactly the carriers the Callable's Inputs hand
// the compiled body — and the result is the list of per-lane accumulators,
// scan's list-of-accumulator shape. Analysing the body here, where the
// structural ReturnsPreserveListAt the word carried before never did, is
// what records the body's bind twins for the twin regime (NUR115's
// discharge; see the spec comment).
//
// A statically-EMPTY rank-2 list runs no lane and so no body — the handler
// answers `[]` — so, as scan does, the body is NOT analysed over it:
// analysing it would flag operand-starved words (`foldaxis 0 [add add] []`)
// that can never run, refusing at check a program both engines answer `[]`
// (found in review, 2026-09-02). The bare List carrier is the sound answer.
func foldaxisReturnsFn(args []Value, r *Registry) []Value {
	if n, ok := StaticListLen(args[2]); ok && n == 0 {
		return []Value{NewCarrier(TList)}
	}
	// The empty-lane error, mirrored at check time exactly as fold's own
	// statically-empty no-init case is (staticEmptyFoldDetail, NUR058's
	// routing): a GUARANTEED runtime error over exactly-known operands, so
	// the finding is a RuntimeMirror — the program compiles and the handler
	// raises the byte-identical text — and the check-accuracy ratchet sees
	// the row flagged rather than pinned as a checker blind spot.
	if detail := staticEmptyLaneDetail(args); detail != "" {
		// A module-qualified call (ArrayUtil.foldaxis) reaches the ReturnsFn
		// through the module's fn wrapper, whose operands carry no source
		// position — the call site the checker exposed (CurCallPos) is the
		// honest location then, exactly as `make Array` reads it.
		pos := args[2].Pos()
		if pos.Row == 0 {
			pos = r.Check.CurCallPos
		}
		core.CheckAddUniqueDiagnostic(r, "foldaxis_error", detail, "foldaxis", pos)
		return []Value{NewCarrier(TList)}
	}
	elemC := rank2ElemCarrier(args[2])
	acc, ok := foldAccumFixedPoint(r, args[1], elemC, elemC)
	if !ok {
		return []Value{NewCarrier(TList)}
	}
	if IsDisjunct(acc) {
		return []Value{NewCarrierTypedListValue(acc)}
	}
	return []Value{NewCarrierTypedList(acc.Parent)}
}

// staticEmptyLaneDetail is the runtime error text foldaxis raises over a
// rank-2 literal whose rows are EMPTY when reduced along axis 1 — one row,
// zero columns, one empty lane with nothing to seed its accumulator — or ""
// when the shape is not exactly known (a non-concrete axis or data argument)
// or does not raise (axis 0 has no lane at all over such data: the result is
// `[]`). Only the FIRST row decides: the handler enforces rectangularity, so
// zero columns in row 0 is zero columns everywhere, and lane 0 is the row
// that raises.
func staticEmptyLaneDetail(args []Value) string {
	axis, err := args[0].AsConcreteInteger()
	if err != nil || axis != 1 {
		return ""
	}
	rows, err := AsList(args[2])
	if err != nil || !IsConcrete(args[2]) || rows.Len() == 0 {
		return ""
	}
	if n, ok := StaticListLen(rows.Get(0)); ok && n == 0 {
		return "foldaxis: lane 0 is empty (no initial value)"
	}
	return ""
}

// doFold is the shared fold implementation used by both fold signatures.
func doFold(reg *Registry, acc Value, body Value, data ReadList) ([]Value, error) {
	for i := 0; i < data.Len(); i++ {
		elem := data.Get(i)
		res, err := InvokeBody(reg, body, []Value{acc, elem})
		if err != nil {
			return nil, fmt.Errorf("fold: step %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, reg.BoruError("fold_error", fmt.Sprintf("fold: step %d: body produced no result", i), "fold")
		}
		acc = res[len(res)-1]
	}
	return []Value{acc}, nil
}

// ---- scan ----

func scanHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteMap(args[1]) {
		return scanMapHandler(args, nil, nil, reg)
	}
	if err := requireConcreteArgs(reg, "scan", "concrete lists", args[0], args[1]); err != nil {
		return nil, err
	}
	dataList, _ := AsList(args[1])
	if dataList.Len() == 0 {
		return []Value{NewList(nil)}, nil
	}

	results := make([]Value, dataList.Len())
	acc := dataList.Get(0)
	results[0] = acc

	for i := 1; i < dataList.Len(); i++ {
		elem := dataList.Get(i)
		res, err := InvokeBody(reg, args[0], []Value{acc, elem})
		if err != nil {
			return nil, fmt.Errorf("scan: step %d: %w", i, err)
		}
		if len(res) == 0 {
			return nil, reg.BoruError("scan_error", fmt.Sprintf("scan: step %d: body produced no result", i), "scan")
		}
		acc = res[len(res)-1]
		results[i] = acc
	}
	return []Value{NewList(results)}, nil
}

// scan returns the list of accumulator states, so its element type
// is the stabilised accumulator (same fixed point as fold).
func scanReturnsFn(args []Value, r *Registry) []Value {
	// A statically-empty collection runs the body ZERO times ("empty in,
	// empty out"), so the body never executes at runtime — analysing it here
	// would flag operand-starved words (`scan [add] []` reports no_signature on
	// `add` over the empty element type) that can never actually run. Skip the
	// body analysis and return an empty-list carrier. Sound: the runtime result
	// is the empty list, which a bare List carrier admits.
	if n, ok := StaticListLen(args[1]); ok && n == 0 {
		return []Value{NewCarrier(TList)}
	}
	elemC := ElementCarrierFromValue(args[1])
	acc, ok := foldAccumFixedPoint(r, args[0], elemC, elemC)
	if !ok {
		return []Value{NewCarrier(TList)}
	}
	if IsDisjunct(acc) {
		return []Value{NewCarrierTypedListValue(acc)}
	}
	return []Value{NewCarrierTypedList(acc.Parent)}
}

// ---- outer ----

func outerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if err := requireConcreteArgs(reg, "outer", "concrete lists", args[0], args[1], args[2]); err != nil {
		return nil, err
	}
	left, _ := AsList(args[1])
	right, _ := AsList(args[2])

	rows := make([]Value, left.Len())
	for i := 0; i < left.Len(); i++ {
		row := make([]Value, right.Len())
		for j := 0; j < right.Len(); j++ {
			// The body sees (left[i], right[j]) on the stack. InvokeBody is the
			// single body-running seam: under the VM it drives the compiled
			// closure, under the interpreter it runs a fresh sub-engine — so a
			// compiled `outer` is byte-identical to the interpreter.
			res, err := InvokeBody(reg, args[0], []Value{left.Get(i), right.Get(j)})
			if err != nil {
				return nil, fmt.Errorf("outer: (%d,%d): %w", i, j, err)
			}
			if len(res) == 0 {
				return nil, reg.BoruError("outer_error", fmt.Sprintf("outer: (%d,%d): body produced no result", i, j), "outer")
			}
			row[j] = res[len(res)-1]
		}
		rows[i] = NewList(row)
	}
	return []Value{NewList(rows)}, nil
}

func outerReturnsFn(args []Value, r *Registry) []Value {
	stk := analyseHigherOrderBodyVals(r, args[0],
		ElementCarrierFromValue(args[1]), ElementCarrierFromValue(args[2]))
	// outer produces a 2D list: TList<TList<body-result>>.
	innerElem := TAny
	if len(stk) > 0 {
		innerElem = stk[len(stk)-1].Parent
	}
	inner := NewCarrierTypedList(innerElem)
	return []Value{NewCarrierTypedListValue(inner)}
}

// ---- inner ----

func innerHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if err := requireConcreteArgs(reg, "inner", "concrete lists", args[0], args[1], args[2], args[3]); err != nil {
		return nil, err
	}
	left, _ := AsList(args[2])
	right, _ := AsList(args[3])

	// 1D case: zip then fold
	if left.Len() > 0 && !left.Get(0).Parent.ConformsTo(TList) {
		if left.Len() != right.Len() {
			return nil, reg.BoruError("inner_error", "inner: vectors must have same length", "inner")
		}
		// Apply pair-op to each pair. InvokeBody is the single body-running
		// seam (as in `outer`): under the VM it drives the compiled closure,
		// under the interpreter it runs a pooled sub-engine.
		paired := make([]Value, left.Len())
		for i := 0; i < left.Len(); i++ {
			res, err := InvokeBody(reg, args[0], []Value{left.Get(i), right.Get(i)})
			if err != nil {
				return nil, fmt.Errorf("inner: pair %d: %w", i, err)
			}
			if len(res) == 0 {
				return nil, reg.BoruError("inner_error", fmt.Sprintf("inner: pair %d: no result", i), "inner")
			}
			paired[i] = res[len(res)-1]
		}
		// Fold with agg-op
		acc := paired[0]
		for i := 1; i < len(paired); i++ {
			res, err := InvokeBody(reg, args[1], []Value{acc, paired[i]})
			if err != nil {
				return nil, fmt.Errorf("inner: fold %d: %w", i, err)
			}
			if len(res) == 0 {
				return nil, reg.BoruError("inner_error", fmt.Sprintf("inner: fold %d: no result", i), "inner")
			}
			acc = res[len(res)-1]
		}
		return []Value{acc}, nil
	}

	// 2D case: matrix inner product
	// left is list of rows, right is list of rows
	// Need to transpose right to get columns
	rightCols := transposeListOfLists(right)

	rows := make([]Value, left.Len())
	for i := 0; i < left.Len(); i++ {
		leftRow, _ := AsList(left.Get(i))
		cols := make([]Value, len(rightCols))
		for j := 0; j < len(rightCols); j++ {
			rightCol := rightCols[j]
			if leftRow.Len() != len(rightCol) {
				return nil, reg.BoruError("inner_error", "inner: dimension mismatch", "inner")
			}
			// Pair then fold, through the same InvokeBody seam as the 1D case.
			paired := make([]Value, leftRow.Len())
			for k := 0; k < leftRow.Len(); k++ {
				res, err := InvokeBody(reg, args[0], []Value{leftRow.Get(k), rightCol[k]})
				if err != nil {
					return nil, err
				}
				if len(res) == 0 {
					return nil, reg.BoruError("inner_error", fmt.Sprintf("inner: pair (%d,%d,%d): no result", i, j, k), "inner")
				}
				paired[k] = res[len(res)-1]
			}
			acc := paired[0]
			for k := 1; k < len(paired); k++ {
				res, err := InvokeBody(reg, args[1], []Value{acc, paired[k]})
				if err != nil {
					return nil, err
				}
				if len(res) == 0 {
					return nil, reg.BoruError("inner_error", fmt.Sprintf("inner: fold (%d,%d,%d): no result", i, j, k), "inner")
				}
				acc = res[len(res)-1]
			}
			cols[j] = acc
		}
		rows[i] = NewList(cols)
	}
	return []Value{NewList(rows)}, nil
}

func innerReturnsFn(args []Value, r *Registry) []Value {
	// pair op consumes (left-elem, right-elem); agg consumes
	// (accumulator, pair-result). Without carrier list element
	// tracking we use the pair output as TAny for the agg input.
	analyseHigherOrderBodyVals(r, args[0],
		ElementCarrierFromValue(args[2]), ElementCarrierFromValue(args[3]))
	analyseHigherOrderBody(r, args[1], TAny, TAny)
	return []Value{NewCarrier(TList)}
}

// ---- compress ----

// compressHandler selects elements of data where the parallel mask is
// truthy. Like `replicate` with a boolean mask, but reads as a filter.
// Sig is [mask, data] (mask top-of-stack), matching replicate's
// counts-first order.
func compressHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	mask, err := requireListArg(r, args[0], "compress", "concrete mask list")
	if err != nil {
		return nil, err
	}
	data, err := requireListArg(r, args[1], "compress", "concrete data list")
	if err != nil {
		return nil, err
	}
	if mask.Len() != data.Len() {
		return nil, fmt.Errorf("compress: mask length %d does not match data length %d", mask.Len(), data.Len())
	}
	result := []Value{}
	for i := 0; i < mask.Len(); i++ {
		if CoerceBoolean(mask.Get(i)) {
			result = append(result, data.Get(i))
		}
	}
	// #4: compress is a mask-filtered subset — retain the data list's [:T] tag.
	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// ---- eachrank ----

// eachrankHandler applies a code body to every cell at a given rank
// (nesting depth) of a nested list. Rank 0 targets each scalar leaf,
// rank 1 each innermost list, rank 2 each list-of-lists, and so on.
// Sig is [rank, body, data] — body is a quoted code list (NoEvalArgs).
func eachrankHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	depth, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, reg.BoruError("eachrank_error", "eachrank: rank must be an integer", "eachrank")
	}
	if depth < 0 {
		return nil, reg.BoruError("eachrank_error", fmt.Sprintf("eachrank: negative rank %d", depth), "eachrank")
	}
	if err := requireConcreteArgs(reg, "eachrank", "concrete body and data lists", args[1], args[2]); err != nil {
		return nil, err
	}
	// `rank` is the J-style CELL rank, measured from the leaves: 0 is
	// each scalar, 1 each innermost list, 2 each list-of-lists. Convert
	// it to a descent depth from the top by subtracting from the data's
	// total nesting depth.
	total := listDepth(args[2])
	descend := total - int(depth)
	if descend < 0 {
		return nil, reg.BoruError("eachrank_error", fmt.Sprintf("eachrank: rank %d exceeds data rank %d", depth, total), "eachrank")
	}
	return eachrankWalk(reg, descend, args[1], args[2])
}

// listDepth reports the nesting depth of v along its first-element
// spine: a scalar is 0, a flat list 1, a list of lists 2, and so on.
func listDepth(v Value) int {
	d := 0
	for v.Parent.ConformsTo(TList) && IsConcrete(v) {
		list, _ := AsList(v)
		d++
		if list.Len() == 0 {
			break
		}
		v = list.Get(0)
	}
	return d
}

// eachrankWalk recurses into the structure until `depth` reaches 0,
// then runs the body once per cell at that level. The cell value is
// pushed and the body's top-of-stack result replaces it.
func eachrankWalk(reg *Registry, depth int, body Value, cell Value) ([]Value, error) {
	if depth == 0 {
		// InvokeBody, not a raw token run: the body arrives as a compiled
		// CLOSURE when the word's Callable spec let the recorder lower it, and
		// falls back to the same resolved token run otherwise.
		res, err := InvokeBody(reg, body, []Value{cell})
		if err != nil {
			return nil, fmt.Errorf("eachrank: %w", err)
		}
		if len(res) == 0 {
			return nil, reg.BoruError("eachrank_error", "eachrank: body produced no result", "eachrank")
		}
		return []Value{res[len(res)-1]}, nil
	}
	if !cell.Parent.ConformsTo(TList) || !IsConcrete(cell) {
		return nil, reg.BoruError("eachrank_error", fmt.Sprintf("eachrank: rank exceeds nesting depth at %v", cell), "eachrank")
	}
	list, _ := AsList(cell)
	out := make([]Value, list.Len())
	for i := 0; i < list.Len(); i++ {
		sub, err := eachrankWalk(reg, depth-1, body, list.Get(i))
		if err != nil {
			return nil, err
		}
		out[i] = sub[0]
	}
	return []Value{NewList(out)}, nil
}

// ---- foldaxis ----

// foldaxisHandler reduces a rank-2 list along one axis with a binary
// body. axis 0 folds down columns (result has one entry per column),
// axis 1 folds along rows (one entry per row). Sig is [axis, body,
// data] — body is a quoted code list (NoEvalArgs).
func foldaxisHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	axis, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, reg.BoruError("foldaxis_error", "foldaxis: axis must be an integer", "foldaxis")
	}
	if axis != 0 && axis != 1 {
		return nil, reg.BoruError("foldaxis_error", fmt.Sprintf("foldaxis: axis must be 0 or 1, got %d", axis), "foldaxis")
	}
	if err := requireConcreteArgs(reg, "foldaxis", "concrete body and data lists", args[1], args[2]); err != nil {
		return nil, err
	}
	rows, _ := AsList(args[2])
	if rows.Len() == 0 {
		return []Value{NewList([]Value{})}, nil
	}
	// Validate a rectangular rank-2 list.
	if !rows.Get(0).Parent.ConformsTo(TList) || !IsConcrete(rows.Get(0)) {
		return nil, reg.BoruError("foldaxis_error", "foldaxis: expected a rank-2 list", "foldaxis")
	}
	first, _ := AsList(rows.Get(0))
	cols := first.Len()
	for i := 1; i < rows.Len(); i++ {
		ri, _ := AsList(rows.Get(i))
		if !rows.Get(i).Parent.ConformsTo(TList) || !IsConcrete(rows.Get(i)) || ri.Len() != cols {
			return nil, reg.BoruError("foldaxis_error", "foldaxis: expected a rectangular rank-2 list", "foldaxis")
		}
	}
	// axis 1 reduces each row directly; axis 0 reduces each column, i.e.
	// each row of the transpose.
	var lanes [][]Value
	if axis == 1 {
		lanes = make([][]Value, rows.Len())
		for i := 0; i < rows.Len(); i++ {
			ri, _ := AsList(rows.Get(i))
			lanes[i] = ri.Slice()
		}
	} else {
		lanes = transposeListOfLists(rows)
	}
	result := make([]Value, len(lanes))
	for i, lane := range lanes {
		// A lane with no element has nothing to seed its accumulator from —
		// fold's own no-init rule (`fold: empty list with no initial
		// value`), raised as this word's error. `[[]]` reduced along axis 1
		// is the shape: one row, zero columns, one empty lane. Indexing
		// lane[0] here panicked before (2026-09-02; panics are forbidden).
		if len(lane) == 0 {
			return nil, reg.BoruError("foldaxis_error", fmt.Sprintf("foldaxis: lane %d is empty (no initial value)", i), "foldaxis")
		}
		acc := lane[0]
		res, err := doFold(reg, acc, args[1], NewReadList(lane[1:]))
		if err != nil {
			return nil, fmt.Errorf("foldaxis: lane %d: %w", i, err)
		}
		result[i] = res[0]
	}
	return []Value{NewList(result)}, nil
}

// transposeListOfLists transposes a list-of-lists, returning columns as [][]Value.
func transposeListOfLists(rows ReadList) [][]Value {
	if rows.Len() == 0 {
		return nil
	}
	firstRow, _ := AsList(rows.Get(0))
	cols := firstRow.Len()
	result := make([][]Value, cols)
	for j := 0; j < cols; j++ {
		result[j] = make([]Value, rows.Len())
	}
	for i := 0; i < rows.Len(); i++ {
		row, _ := AsList(rows.Get(i))
		for j := 0; j < cols && j < row.Len(); j++ {
			result[j][i] = row.Get(j)
		}
	}
	return result
}
