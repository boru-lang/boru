package native

import (
	"fmt"
	"time"

	voxgigstruct "github.com/voxgig/struct/go"
)

// Natives is the consolidated NativeFunc slice for the data-manipulation
// words owned by the native package. It replaces the per-word RegisterFoo
// functions and their aggregator (registerAll). The public Register entry
// point in native.go installs every entry into a registry.
var Natives = []NativeFunc{
	// `implies` (with nand/nor/iff/xnor) moved to the boru:logic-util module —
	// see native/logic_module.go.

	// ---- control flow ----
	{
		Name: "quote",
		// The /q'd-Atom sig produces an inert quoted symbol the VM can bake +
		// CALL_NATIVE faithfully (the TAny sig is RunInCheckMode and unaffected).
		CompileEffect: CompileQuoteInert,

		Signatures: []Signature{
			{
				// /q captures the upcoming Word as an Atom for us; the
				// handler just marks it Quoted=true.
				//
				// RunInCheck for the same reason the TAny sibling below has
				// it, and it is the whole VALUE of quoting a name: /q hands
				// the handler a CONCRETE Atom, so running it yields the
				// actual quoted symbol instead of a bare Atom carrier, and a
				// consumer that reads the key gets one. `m dot (quote a)`
				// checked dynamic(Any) against a payload-free carrier while
				// the identical `m.a` checked Integer — the key was there to
				// be read and the model threw it away.
				Args:      []*Type{TAtom},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(quoteWordHandler, RunInCheck()),
				Returns:   []*Type{TAtom}, BarrierPos: -1,
			},
			{
				Args:       []*Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(quoteAnyHandler, RunInCheck()),
				ReturnsFn:  ReturnsIdentity(0), BarrierPos: -1,
			},
		},
	},

	// `codequote` is `quote`'s code-capturing sibling: it also captures a
	// forward *paren* RAW (as a ParenExpr value) instead of evaluating it.
	// `quote (expr)` evaluates expr then quotes the result (the inert-value
	// idiom); `codequote (expr)` keeps the paren as code — the structural
	// quotability the macro layer wants. Words → atoms and lists → raw list
	// behave exactly like `quote`. See design/legacy/PAREN-REPRESENTATION.9.ignore §2.2.
	{
		Name: "codequote",
		// Like quote: the /q'd-Atom sig bakes its inert symbol + CALL_NATIVE.
		CompileEffect: CompileQuoteInert,

		Signatures: []Signature{
			{
				// RunInCheck for quote's reason, on quote's handler: the
				// header above promises "words → atoms … behave exactly like
				// `quote`", and a bare-word codequote takes the identical /q
				// capture through the identical quoteWordHandler. Leaving it
				// off made that promise false in check mode alone —
				// `m dot (codequote a)` stayed dynamic(Any) where
				// `m dot (quote a)` reads Integer (PR #351 review, Codex P2).
				Args:      []*Type{TAtom},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(quoteWordHandler, RunInCheck()),
				Returns:   []*Type{TAtom}, BarrierPos: -1,
			},
			{
				Args:       []*Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				RawParens:  map[int]bool{0: true},
				Impl:       Go(quoteAnyHandler, RunInCheck()),
				ReturnsFn:  ReturnsIdentity(0), BarrierPos: -1,
			},
		},
	},

	// `reach <receiver> [keys]` builds a first-class Reach value (a lens)
	// programmatically: an inert (non-evaluating) dot-access over the
	// receiver with literal `get` segments. Unlike a parsed m.a.b (which
	// evaluates eagerly), a constructed reach is data you can inspect,
	// pass, and convert (see design/REACH.10.md §7). getr/computed segments
	// come from source via codequote; the list-encoding for them is TBD.
	{
		Name: "reach",

		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       Go(reachHandler),
				Returns:    []*Type{TReach}, BarrierPos: -1,
			},
		},
	},

	// `word <value>` wraps its argument (unevaluated) in an __SP splice
	// marker. When the marker reaches the stack pointer its payload is
	// spliced in: a plain list contributes its top-level elements, any
	// other value contributes itself, and the result is re-stepped against
	// the live stack. `def name word value` binds the marker so a later
	// reference splices. The arg is NoEvalArgs so the body is stored raw.
	{
		Name: "word",

		Signatures: []Signature{
			{
				Args:       []*Type{TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(wordHandler),
				Returns:    []*Type{TAny}, BarrierPos: -1,
			},
		},
	},

	// `folder` (filesystem op) moved to the boru:io module — see io_module.go.

	// ---- string slice ----
	stringSliceNative(),

	// ---- stack ----
	{
		Name: "stack",

		Signatures: []Signature{{
			Args:       []*Type{TInteger},
			Impl:       Go(stackCollectHandler, FullStack(), CheckFullStack(stackCollectCheckFullStackFn)),
			BarrierPos: 0,
		}},
	},

	// now / sleep / interval / cancel (with timeout / await) moved to the
	// boru:time-util module — see native/time_async_module.go.

	// ---- list (table query) ----
	{
		Name: "list",

		Signatures: []Signature{
			// Every form — entity, API, table, record — builds a NewList.
			{Args: []*Type{TMap, TResourceEntity}, Impl: Go(listEntityOptsHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Impl: Go(listEntityHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(listAPIOptsHandler), Patterns: map[int]Value{1: apiPatternValue()}, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(listAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Impl: Go(listFilterHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(listAllHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(listRecordFilterHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(listRecordAllHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

	// ---- create ----
	{
		Name: "create",

		Signatures: []Signature{
			// Entity / API forms return whatever the SDK yields (convertResultItem): Any.
			{Args: []*Type{TMap, TResourceEntity}, Impl: Go(createEntityOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Impl: Go(createEntityHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(createAPIOptsHandler), Patterns: map[int]Value{1: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(createAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			// Table / record forms always build a NewList.
			{Args: []*Type{TMap, TList}, Impl: Go(createHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(createRecordHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

	// ---- load ----
	{
		Name: "load",

		Signatures: []Signature{
			// Entity / API forms return whatever the SDK yields (convertResultItem): Any.
			{Args: []*Type{TMap, TResourceEntity}, Impl: Go(loadEntityOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Impl: Go(loadEntityHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(loadAPIOptsHandler), Patterns: map[int]Value{1: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(loadAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			// The table form returns the matched Map row (non-map rows are
			// skipped; no match raises); the record form an empty Map.
			{Args: []*Type{TMap, TList}, Impl: Go(loadHandler), Returns: []*Type{TMap}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(loadRecordHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},

	// ---- update ----
	{
		Name: "update",

		Signatures: []Signature{
			// Entity / API forms return whatever the SDK yields (convertResultItem): Any.
			{Args: []*Type{TMap, TResourceEntity}, Impl: Go(updateEntityOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Impl: Go(updateEntityHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(updateAPIOptsHandler), Patterns: map[int]Value{1: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(updateAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			// Table / record forms always build a NewList.
			{Args: []*Type{TMap, TList}, Impl: Go(updateHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(updateRecordHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

	// ---- remove ----
	{
		Name: "remove",

		Signatures: []Signature{
			// Entity / API forms return whatever the SDK yields (convertResultItem): Any.
			{Args: []*Type{TMap, TResourceEntity}, Impl: Go(removeEntityOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TResourceEntity}, Impl: Go(removeEntityHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(removeAPIOptsHandler), Patterns: map[int]Value{1: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TMap}, Impl: Go(removeAPIHandler), Patterns: map[int]Value{0: apiPatternValue()}, Returns: []*Type{TAny}, BarrierPos: -1},
			// Table / record forms always build a NewList.
			{Args: []*Type{TMap, TList}, Impl: Go(removeHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TMap, TMap}, Impl: Go(removeRecordHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

	// ---- size ----
	{
		Name:          "size",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(sizeHandler), ReturnsFn: sizeReturns, BarrierPos: -1, Returns: []*Type{TInteger}},
		},
	},

	// `pad` (with the rest of the string words) moved to the boru:string-util
	// module — see native/string_module.go.

	// fetch / prepare / direct moved to the boru:net module — see net_module.go.

	// ---- flatten ----
	{
		Name: "flatten",

		Signatures: []Signature{
			{Args: []*Type{TInteger, TList}, Impl: Go(flattenDepthHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(flattenDefaultHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},

	// ---- filter ----
	{
		Name:          "filter",
		CompileEffect: CompileFallbackBody | CompileDynBody,
		// filter [body] data — the body sees one element and returns a Boolean.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, BodyResultTop: true, Inputs: func(a []Value) []Value {
			return []Value{NewElementCarrier(DataListElemTypeFromValue(a[1]))}
		}},

		Signatures: []Signature{
			{Args: []*Type{TFunction, TAny}, Impl: Go(filterHandler), ReturnsFn: filterReturnsFn, BarrierPos: -1},
			// Lens form: `filter $.active xs` keeps the elements whose reach
			// applies to a truthy value (the reach reads the ELEMENT, not the
			// {key,value} wrapper the Function form receives).
			{Args: []*Type{TReach, TAny}, Impl: Go(filterReachHandler), ReturnsFn: filterReturnsFn, BarrierPos: -1},
			// Quotation form: `filter [body] xs` runs the quoted body once
			// per element (element pushed first, like each/fold) and keeps
			// the elements whose body result is Boolean true.
			{Args: []*Type{TList, TAny}, NoEvalArgs: map[int]bool{0: true}, Impl: Go(filterBodyHandler), ReturnsFn: filterReturnsFn, BarrierPos: -1},
		},
	},

	// ---- join ----
	{
		Name: "join",

		Signatures: []Signature{
			{Args: []*Type{TString, TList}, Impl: Go(joinSepHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(joinDefaultHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},

	// jsonify moved to the boru:struct module — see struct_module.go.

	// ---- listops (push/pop/unshift/shift) ----
	// Each word carries a FlexList sig alongside the plain List sig.
	// The FlexList sig is more specific (deeper lattice Rank) so it
	// wins for flex inputs and mutates IN PLACE, returning the same
	// node (pop/shift additionally return the removed element on
	// top). Plain lists keep the immutable new-copy semantics.
	{
		Name: "push",

		Signatures: []Signature{
			{Args: []*Type{TAny, TFlexList}, Impl: Go(pushFlexHandler), Returns: []*Type{TFlexList}, ReturnsFn: flexGrowReturns("push"), BarrierPos: -1},
			// Returns a List (was undeclared → Any, which widened a fold/scan
			// accumulator to Any on the second round and then wrongly rejected the
			// next `push` — `[] fold [push] xs`). Mirrors unshift's List overload.
			{Args: []*Type{TAny, TList}, Impl: Go(pushHandler), Returns: []*Type{TList}, ReturnsFn: plainListGrowReturns("push"), BarrierPos: -1},
		},
	},
	{
		Name: "pop",

		Signatures: []Signature{
			{Args: []*Type{TFlexList}, Impl: Go(popFlexHandler), Returns: []*Type{TFlexList, TAny}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(popHandler), Returns: []*Type{TList, TAny}, ReturnsFn: listEdgeElemReturns(true), BarrierPos: -1},
		},
	},
	{
		Name: "unshift",

		Signatures: []Signature{
			{Args: []*Type{TAny, TFlexList}, Impl: Go(unshiftFlexHandler), Returns: []*Type{TFlexList}, ReturnsFn: flexGrowReturns("unshift"), BarrierPos: -1},
			{Args: []*Type{TAny, TList}, Impl: Go(unshiftHandler), Returns: []*Type{TList}, ReturnsFn: plainListGrowReturns("unshift"), BarrierPos: -1},
		},
	},
	{
		Name: "shift",

		Signatures: []Signature{
			{Args: []*Type{TFlexList}, Impl: Go(shiftFlexHandler), Returns: []*Type{TFlexList, TAny}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(shiftHandler), Returns: []*Type{TList, TAny}, ReturnsFn: listEdgeElemReturns(false), BarrierPos: -1},
		},
	},

	// ---- istype ----
	{
		Name: "istype",

		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(istypeHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},

	// ---- walk (generic, iterative, visit-only structure traversal) ----
	// Distinct from StructUtil.walk (the recursive voxgig-struct word).
	// The hook slots are TAny so a quotation `[...]` (passed raw via
	// NoEvalArgs) and a lambda `(m => …)` both match; classification
	// happens in the handler. See walk_core.go.
	{
		Name: "walk",
		// The DESCEND hook (sig position 2) compiles to a closure unit the
		// handler drives through InvokeBody (walkClassifyHook already
		// classifies a compiled closure). Visit-only: hook results are
		// DISCARDED (callWalkHook ignores the residual), so BodyOut 0 keeps
		// the closure count-agnostic. Quotation and lambda hooks both receive
		// the ONE `{key value path parent depth}` payload
		// (LambdaSharesTokenShape). The optional ASCEND slot (position 3) is
		// guarded recorder-side (extraNoEvalHookSlotsOK): only a
		// provably-empty flex reference rides as a value operand; every other
		// ascend shape keeps today's refusal/bake behaviour.
		Callable: &CallableSpec{BodyPos: 2, BodyOut: 0, LambdaSharesTokenShape: true, Inputs: func(_ []Value) []Value {
			return []Value{walkHookArgCarrier()}
		}},

		Signatures: []Signature{
			{
				Args:       []*Type{TMap, TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{2: true, 3: true},
				Impl:       Go(walkCoreHandler),
				// Visit-only: walk returns its input data (arg 1) unchanged,
				// so the result carries the data's exact provenance/type.
				ReturnsFn: ReturnsIdentity(1), BarrierPos: -1,
			},
			{
				Args:       []*Type{TMap, TAny, TAny},
				NoEvalArgs: map[int]bool{2: true},
				Impl:       Go(walkCoreHandler),
				ReturnsFn:  ReturnsIdentity(1), BarrierPos: -1,
			},
		},
	},
}

// apiPatternValue returns the pattern map {kind:"api"} used by signature
// matching to discriminate API maps from plain maps.
func apiPatternValue() Value {
	apiPattern := NewOrderedMap()
	apiPattern.Set("kind", NewString("api"))
	return NewMap(apiPattern)
}

// stringSliceNative builds the "slice" NativeFunc covering substring and
// sublist extraction with three forward-first signatures (3-arg
// start+end+data, 2-arg start+data, 1-arg data) for both String and List
// inputs.
func stringSliceNative() NativeFunc {
	return NativeFunc{
		Name: "slice",

		Signatures: []Signature{
			{Args: []*Type{TInteger, TInteger, TString}, Impl: Go(sliceStartEndHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TInteger, TList}, Impl: Go(sliceStartEndHandler), Returns: []*Type{TList}, ReturnsFn: sliceListReturns(2), BarrierPos: -1},
			{Args: []*Type{TInteger, TString}, Impl: Go(sliceStartHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TList}, Impl: Go(sliceStartHandler), Returns: []*Type{TList}, ReturnsFn: sliceListReturns(1), BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(sliceAllHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(sliceAllHandler), Returns: []*Type{TList}, ReturnsFn: sliceListReturns(0), BarrierPos: -1},
		},
	}
}

// sliceStringByRunes slices a string by CHARACTER (rune) indices —
// the one character unit every string word counts in (NUR007; slice
// previously sliced bytes and could split a multi-byte rune into
// invalid UTF-8). The index normalization replicates voxgigstruct.
// Slice's arithmetic exactly, with the unit changed: a negative start
// drops |start| characters from the END (start 0, end vlen+start); a
// negative end counts back from the end; everything clamps to the
// rune length; an inverted range is the empty string.
func sliceStringByRunes(s string, start int, endP *int) string {
	r := []rune(s)
	vlen := len(r)
	end := vlen
	if start < 0 {
		end = vlen + start
		if end < 0 {
			end = 0
		}
		start = 0
	} else if endP != nil {
		end = *endP
		if end < 0 {
			end = vlen + end
			if end < 0 {
				end = 0
			}
		} else if vlen < end {
			end = vlen
		}
	}
	if vlen < start {
		start = vlen
	}
	if start <= end && end <= vlen {
		return string(r[start:end])
	}
	return ""
}

// sliceStartEndHandler implements `slice start end data` (forward-first:
// args[0]=start, args[1]=end, args[2]=data). Used by both string and
// list overloads; the string overload slices by runes (NUR007), lists
// keep the voxgigstruct path.
func sliceStartEndHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	start := int(_as0)
	_as1, _ := args[1].AsConcreteInteger()
	end := int(_as1)
	if s, serr := args[2].AsConcreteString(); serr == nil {
		return []Value{NewString(sliceStringByRunes(s, start, &end))}, nil
	}
	data := valueToSliceArg(args[2])
	result := voxgigstruct.Slice(data, start, end)
	res, err := sliceResult(result)
	return sliceRetain(res, err, args[2])
}

// sliceStartHandler implements `slice start data` (forward-first:
// args[0]=start, args[1]=data). Slices from start to the end of the
// input; the string overload slices by runes (NUR007).
func sliceStartHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	s := int(_as0)
	if str, serr := args[1].AsConcreteString(); serr == nil {
		return []Value{NewString(sliceStringByRunes(str, s, nil))}, nil
	}
	data := valueToSliceArg(args[1])
	result := voxgigstruct.Slice(data, s)
	res, err := sliceResult(result)
	return sliceRetain(res, err, args[1])
}

// sliceAllHandler implements `slice data` — the identity/copy form that
// returns the input unchanged through voxgigstruct.Slice.
func sliceAllHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	data := valueToSliceArg(args[0])
	result := voxgigstruct.Slice(data)
	res, err := sliceResult(result)
	return sliceRetain(res, err, args[0])
}

// ---- handlers extracted from per-word RegisterFoo closures ----

// impliesHandler implements the "implies" boolean operator. Args[1] is
// the antecedent (left), args[0] is the consequent (right): !left||right.
func impliesHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	left := CoerceBoolean(args[1])
	right := CoerceBoolean(args[0])
	return []Value{NewBoolean(!left || right)}, nil
}

// quoteWordHandler marks the captured atom (already converted from the
// upcoming Word by /q) as Quoted=true and, when its name is currently bound,
// snapshots the binding as the atom's referent — so a quoted name records
// what it referred to at quote time.
func quoteWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{captureAtomReferent(v, r)}, nil
}

// quoteAnyHandler returns the value with Quoted=true, suppressing
// downstream auto-evaluation. Lists/maps are left structurally intact. An
// atom value additionally captures its referent (see quoteWordHandler).
func quoteAnyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	v.Quoted = true
	return []Value{captureAtomReferent(v, r)}, nil
}

// captureAtomReferent snapshots the current binding of an atom's name onto
// the atom as its referent, when the value is an atom that has none yet and
// the name is bound. Non-atoms and already-captured atoms are returned
// unchanged.
func captureAtomReferent(v Value, r *Registry) Value {
	name, err := AsAtom(v)
	if err != nil {
		return v
	}
	if _, has := AtomReferent(v); has {
		return v
	}
	if bound, ok := r.Defs.Top(name); ok {
		return SetAtomReferent(v, bound)
	}
	return v
}

// reachHandler builds an inert Reach over a receiver value (args[0]) and a
// key list (args[1], NOT evaluated). The list encodes one segment per key:
//
//   - a bare word / atom / string / number → a `get` (lenient) segment
//   - a `!` marker element → the NEXT key is a `getr` (strict) segment
//     (so `reach m [a !b]` ≡ m.a!.b — `!b` lexes as `! b`)
//   - a `(expr)` paren element → a computed segment (key evaluated at apply)
//
// e.g. `reach m [a !b (k)]` → an inert m.a!.b.(k) lens (data).
func reachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	keys, err := RequireConcreteList(args[1], "reach")
	if err != nil {
		return nil, err
	}
	segs, err := decodeReachSegments(keys.Slice(), r)
	if err != nil {
		return nil, err
	}
	return []Value{NewReach(ReachInfo{Receiver: []Value{args[0]}, Segments: segs, Eval: false})}, nil
}

// decodeReachSegments turns a `reach` key list into Reach segments (see
// reachHandler for the encoding).
func decodeReachSegments(elems []Value, r *Registry) ([]ReachSeg, error) {
	segs := make([]ReachSeg, 0, len(elems))
	getrNext := false
	for _, el := range elems {
		if isReachBang(el) {
			getrNext = true
			continue
		}
		seg := ReachSeg{Getr: getrNext}
		getrNext = false
		if IsParenExpr(el) {
			toks, _ := AsParenExpr(el)
			seg.Computed = true
			seg.KeyExpr = toks
		} else {
			seg.KeyLit = el
		}
		segs = append(segs, seg)
	}
	if getrNext {
		return nil, r.BoruError("reach_error", "reach: trailing `!` with no following key", "reach")
	}
	return segs, nil
}

// isReachBang reports whether a key-list element is the `!` getr marker (a
// bare `!` lexes to a word; an atom `!` covers the quoted form).
func isReachBang(v Value) bool {
	if IsWord(v) {
		w, _ := AsWord(v)
		return w.Name == "!"
	}
	if IsAtom(v) {
		a, _ := AsAtom(v)
		return a == "!"
	}
	return false
}

// wordHandler wraps its (unevaluated) argument in an __SP splice marker. The
// splice itself happens later, when the marker reaches the engine pointer.
func wordHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewSplice(args[0])}, nil
}

// folderOptsHandler implements `folder` with a leading {parents:bool} options map.
func folderOptsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	optsVal := args[0]
	pathVal := args[1]
	if !IsPathon(pathVal) {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.Parent.String())
	}
	parents := true
	if optsMap, _ := AsMap(optsVal); optsMap != nil {
		if v, ok := optsMap.Get("parents"); ok && v.Parent.ConformsTo(TBoolean) {
			parents, _ = AsBoolean(v)
		}
	}
	_as0, _ := AsPathon(pathVal)
	return doFolder(_as0, parents, reg)
}

// folderHandler implements `folder` with a single Path arg (parents=true).
func folderHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	pathVal := args[0]
	if !IsPathon(pathVal) {
		return nil, fmt.Errorf("folder: expected Path, got %s", pathVal.Parent.String())
	}
	_as1, _ := AsPathon(pathVal)
	return doFolder(_as1, true, reg)
}

// doFolder is the shared body for both folder signatures; resolves and
// creates a directory via the configured FileOps.
func doFolder(p PathonInfo, parents bool, reg *Registry) ([]Value, error) {
	ops := EffectiveFileOps(reg)
	pathStr := p.String()

	// C1 effect fence: directory creation mutates the filesystem — noted on
	// the attempt, since MkdirAll can create some parents before failing.
	reg.NoteEffect()
	if parents {
		if err := ops.MkdirAll(pathStr, 0755); err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	} else {
		resolved, err := ops.ResolvePath(pathStr)
		if err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
		if err := ops.MkdirAll(resolved, 0755); err != nil {
			return []Value{NewError(fmt.Errorf("folder: %w", err))}, nil
		}
	}

	return []Value{NewPathonVol(p.Volume, p.Parts, p.Abs)}, nil
}

// stackCollectHandler runs at execution time: wraps the top N stack
// entries into a list, preserving the rest of the stack underneath.
func stackCollectHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n > len(stack) {
		return nil, fmt.Errorf("stack: count %d out of range (stack depth %d)", n, len(stack))
	}
	items := make([]Value, n)
	copy(items, stack[len(stack)-n:])
	return append(stack, NewList(items)), nil
}

// stackCollectCheckFullStackFn is the check-mode model for `stack N`:
// we don't know N statically, so produce a typed-list carrier whose
// element type joins all preserved stack carriers, leaving the original
// stack intact below it.
func stackCollectCheckFullStackFn(_ []Value, stack []Value, _ *Registry) []Value {
	elem := TAny
	if len(stack) > 0 {
		elem = stack[0].Parent
		for i := 1; i < len(stack); i++ {
			elem = CommonAncestorType(elem, stack[i].Parent)
		}
	}
	return append(append([]Value(nil), stack...), NewCarrierTypedList(elem))
}

// nowHandler returns the current instant as an Instant value, read from
// the registry's Clock capability (a wall clock by default; a FixedClock
// under test/spec) so temporal output is reproducible when frozen.
func nowHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return []Value{NewInstant(EffectiveClock(r).Now())}, nil
}

// sleepHandler pauses the current goroutine for the given milliseconds.
func sleepHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, r.BoruError("sleep_error", fmt.Sprintf("sleep: milliseconds must be non-negative, got %d", ms), "sleep")
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil, nil
}

// intervalListHandler / intervalAtomHandler schedule a repeated callback
// (a quoted code list or word) at the given millisecond interval.
func intervalListHandler(tt TemporalModuleTypes) Handler {
	return func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
		return startInterval(tt, args, r, true)
	}
}

func intervalAtomHandler(tt TemporalModuleTypes) Handler {
	return func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
		return startInterval(tt, args, r, false)
	}
}

func startInterval(tt TemporalModuleTypes, args []Value, r *Registry, isList bool) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms <= 0 {
		return nil, r.BoruError("interval_error", fmt.Sprintf("interval: milliseconds must be positive, got %d", ms), "interval")
	}
	callback := args[1]

	id := GenerateID("T_")
	ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
	done := make(chan struct{})

	// Fork now, on the scheduling goroutine, so every tick runs the
	// callback on an isolated registry and never races the main
	// interpreter. The fork is reused across ticks; its private scopes
	// persist between invocations like a long-lived handler's state.
	fork := r.ForkConcurrent()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				RunTimerCallback(fork, callback, isList)
			}
		}
	}()

	info := &IntervalInfo{
		ID:     id,
		Ms:     ms,
		Ticker: ticker,
		Done:   done,
	}
	return []Value{tt.NewInterval(info)}, nil
}

// cancelTimeoutHandler stops a pending Timeout timer.
func cancelTimeoutHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ti, ok := args[0].Data.(*TimeoutInfo)
	if !ok {
		return nil, r.BoruError("cancel-timeout_error", fmt.Sprintf("cancel-timeout: not a Timeout value (got %s)", args[0].Parent), "cancel-timeout")
	}
	if ti.Timer != nil {
		ti.Timer.Stop()
		ti.Timer = nil
	}
	return nil, nil
}

// cancelIntervalofHandler stops a running Interval ticker and signals its
// goroutine to exit.
func cancelIntervalofHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	ii, ok := args[0].Data.(*IntervalInfo)
	if !ok {
		return nil, r.BoruError("cancel-interval_error", fmt.Sprintf("cancel-interval: not an Interval value (got %s)", args[0].Parent), "cancel-interval")
	}
	if ii.Ticker != nil {
		ii.Ticker.Stop()
		close(ii.Done)
		ii.Ticker = nil
	}
	return nil, nil
}

// listEdgeElemReturns is the check-mode narrower for pop (last=true) and
// shift (last=false) over a plain List: the removed ELEMENT's type is the
// statically-known edge element's type when the list is concrete —
// `pop [1 2 3]` yields (…, Integer) instead of (…, dynamic(Any)). The
// remaining-list slot keeps the declared List carrier. A non-concrete or
// statically-empty list (the runtime raises on empty) falls back to the
// declared shape.
func listEdgeElemReturns(last bool) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		fallback := []Value{NewCarrier(TList), NewDynamicCarrier(TAny)}
		if len(args) != 1 || !IsConcrete(args[0]) {
			return fallback
		}
		list, err := AsList(args[0])
		if err != nil || list.IsNil() || list.Len() == 0 {
			return fallback
		}
		elem := list.Get(0)
		if last {
			elem = list.Get(list.Len() - 1)
		}
		if elem.Undefined {
			return fallback
		}
		et := ValueType(elem)
		if et == nil || et.Equal(TAny) {
			return fallback
		}
		c := NewCarrier(et)
		// A carrier / dynamic element propagates its own gradual claim.
		if elem.Dynamic {
			c.Dynamic = true
		}
		return []Value{NewCarrier(TList), c}
	}
}
