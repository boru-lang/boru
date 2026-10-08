package native

import (
	"errors"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// StructModuleNatives holds the voxgig-struct data-manipulation words that
// were moved out of the core registry into the loadable `boru:struct` module
// (namespace `Struct`). They are registered ONLY into that module's
// sub-registry by modules.BuildStructModule — deliberately absent from the
// global registry (they are no longer in the core Natives slice in
// natives.go). The handlers themselves still live in their feature files
// (clone.go, merge.go, walk.go, transform.go, …) in this package.
//
// Each word is a thin wrapper over a github.com/voxgig/struct utility:
//
//	clone     deep copy of any value
//	getpath   read a dotted path out of a structure
//	setpath   write a value at a dotted path (returns a new structure)
//	inject    resolve `$`-path references embedded in a structure
//	merge     deep, index-wise merge of two structures
//	walk      structural walk, optionally with before/after transforms
//	items     key/value pairs of a map (or index/value of a list)
//	transform shape one structure into another via a spec
//	validate  validate a structure against a shape spec
//	selector  query-select out of a structure
//	jsonify   serialise a value to a jsonic/JSON string
//	nodify    project a value to its Node/Scalar form (voxgig/struct)
//
// All words are all-forward eligible (BarrierPos -1) so they dispatch in
// both forward (`StructUtil.clone {a:1}`) and stack (`{a:1} StructUtil.clone`) form
// through the namespace — the recommended shape for module words (see the
// "Module FnDef Wrappers" note in lang/go/CLAUDE.md). clone/walk were
// stack-only (0) as core words; -1 is a pure dispatch widening (the inner
// native only ever runs via the trivial-delegation short-circuit, where
// there are no forward tokens, so -1 and 0 behave identically there).
var StructModuleNatives = []NativeFunc{
	{
		Name: "transform",
		Signatures: []Signature{
			// The result is shaped by the SPEC (an arbitrary template), so
			// its static type is genuinely unknowable: declared Any.
			{Args: []*Type{TMap, TAny}, Impl: Go(transformHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "merge",
		Signatures: []Signature{
			// Both index-merge forms always build a NewList.
			{Args: []*Type{TList, TMap}, Impl: Go(mergeListMapHandler), Returns: []*Type{TList}, ReturnsFn: mergeListMapReturns, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Impl: Go(mergeMapListHandler), Returns: []*Type{TList}, ReturnsFn: mergeMapListReturns, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(mergeHandler), Returns: []*Type{TAny}, ReturnsFn: mergeReturns, BarrierPos: -1},
		},
	},
	{
		Name: "validate",
		Signatures: []Signature{
			{Args: []*Type{TMap, TAny}, Impl: Go(validateHandler), Returns: []*Type{TAny}, ReturnsFn: structDataShapeReturns(1), BarrierPos: -1},
		},
	},
	{
		Name: "getpath",
		Signatures: []Signature{
			// A path read out of an arbitrary structure: genuinely Any.
			{Args: []*Type{TString, TAny}, Impl: Go(getpathHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			// Lens form: `getpath $.a.b m` reads through a Reach (honors
			// per-segment getr strictness + computed keys, natively).
			{Args: []*Type{TReach, TAny}, Impl: Go(getpathReachHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "setpath",
		Signatures: []Signature{
			{Args: []*Type{TString, TAny, TAny}, Impl: Go(setpathHandler), Returns: []*Type{TAny}, ReturnsFn: setpathReturns, BarrierPos: -1},
			{Args: []*Type{TAny, TString, TAny}, Impl: Go(setpathHandler), Returns: []*Type{TAny}, ReturnsFn: setpathReturns, BarrierPos: -1},
			// Lens forms: a Reach path in the leading or middle slot
			// (`setpath $.a.b 9 m` / `setpath m $.a.b 9`).
			{Args: []*Type{TReach, TAny, TAny}, Impl: Go(setpathHandler), Returns: []*Type{TAny}, ReturnsFn: setpathReturns, BarrierPos: -1},
			{Args: []*Type{TAny, TReach, TAny}, Impl: Go(setpathHandler), Returns: []*Type{TAny}, ReturnsFn: setpathReturns, BarrierPos: -1},
		},
	},
	{
		Name: "inject",
		Signatures: []Signature{
			{Args: []*Type{TAny, TAny}, Impl: Go(injectHandler), Returns: []*Type{TAny}, ReturnsFn: structDataShapeReturns(0), BarrierPos: -1},
		},
	},
	{
		Name: "clone",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(cloneHandler), ReturnsFn: cloneReturnsFn, BarrierPos: -1},
		},
	},
	{
		Name: "walk",
		Signatures: []Signature{
			// The callback forms return the TRANSFORMED tree — the callback
			// may replace any node including the root, so the type is Any.
			{Args: []*Type{TFunction, TFunction, TAny}, Impl: Go(walkBeforeAfterHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TFunction, TAny}, Impl: Go(walkBeforeHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			// The 1-arg form always builds the List of {path value} leaf rows.
			{Args: []*Type{TAny}, Impl: Go(walkHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "selector",
		Signatures: []Signature{
			// The ELEMENTS depend on the queried structure, but the
			// container never does: voxgigstruct.Select's Go return
			// type is []any, so the convert-back always builds a List
			// — a no-match selects the empty one. List, not Any.
			{Args: []*Type{TMap, TAny}, Impl: Go(selectorHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "items",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(itemsHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "jsonify",
		Signatures: []Signature{
			{Args: []*Type{TMap, TAny}, Impl: Go(jsonifyFlagsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAny}, Impl: Go(jsonifyDefaultHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		// parse — jsonic/JSON text → data, the decode complement of
		// jsonify. DATA context (nothing evaluates: unquoted text →
		// strings, numbers → numbers, true/false → booleans); accepts
		// the jsonic superset so strict JSON parses too; malformed
		// input raises [boru/parse_error]. See design/legacy/PARSING.10.ignore §2.
		Name: "parse",
		Signatures: []Signature{
			{Args: []*Type{TString}, Impl: Go(parseTextHandler), Returns: []*Type{TAny}, ReturnsFn: parseTextReturns, BarrierPos: -1},
		},
	},
	{
		// reify — hydrate a class instance from JSON text or a Node.
		// The inverse of the instance-aware jsonify; the target is an
		// explicit class type or a tor union of classes ($class then
		// selects the member). See reify.go + design/legacy/CLASS-OBJECT.10.ignore §3e.
		Name: "reify",
		Signatures: []Signature{
			{Args: []*Type{TAny, TMap}, Impl: Go(reifyHandler), Returns: []*Type{TAny}, ReturnsFn: reifyReturns, BarrierPos: -1},
			{Args: []*Type{TAny, TString}, Impl: Go(reifyHandler), Returns: []*Type{TAny}, ReturnsFn: reifyReturns, BarrierPos: -1},
		},
	},
	{
		Name: "nodify",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(nodifyHandler), Returns: []*Type{TAny}, ReturnsFn: structDataShapeReturns(0), BarrierPos: -1},
		},
	},
}

// ---- check-mode ReturnsFns for the StructUtil words -------------------
//
// Every claim below is a DYNAMIC carrier — a gradual bound, discharged by
// a guard at run time — so an exotic runtime shape (a user `behave`
// nodify hook, a spec-driven transform) can never be unsoundly committed.
// Only what the handler guarantees structurally is claimed; anything
// value-dependent stays dynamic(Any).

// structDataShapeReturns claims the container KIND of the data operand at
// sig position i: the voxgig words validate/inject/nodify return a
// structure of the same top-level kind as their data input (validate
// returns the defaulted data, inject the reference-resolved copy, nodify
// the Node projection). A non-container or unknown-typed input stays
// dynamic(Any).
func structDataShapeReturns(i int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		dyn := []Value{NewDynamicCarrier(TAny)}
		if i >= len(args) {
			return dyn
		}
		return []Value{dynamicContainerKind(args[i])}
	}
}

// dynamicContainerKind is the shared claim: dynamic(Map) for a Map-typed
// value, dynamic(List) for a List, dynamic(Any) otherwise.
func dynamicContainerKind(v Value) Value {
	t := ValueType(v)
	switch {
	case t == nil || v.Dynamic && t.Equal(TAny):
		return NewDynamicCarrier(TAny)
	case t.ConformsTo(TMap):
		return NewDynamicCarrier(TMap)
	case t.ConformsTo(TList):
		return NewDynamicCarrier(TList)
	default:
		return NewDynamicCarrier(TAny)
	}
}

// d2DynamicTypedResidual is dynamicContainerKind PLUS the element tag when data
// is a typed container: the element rides in BOTH the carrier's child payload
// (so a read from a typed setpath/merge result narrows via getIntKeyReturns /
// getNodeReturns instead of degrading to dynamic(Any)) AND the elem pointer (so a
// chained write stays enforced — d2CheckWrite reads ElemConstraint). Preserves
// the dynamic modality of the base carrier (#5, Codex round 5).
func d2DynamicTypedResidual(data Value) Value {
	base := dynamicContainerKind(data)
	elem, ok := data.ElemConstraint()
	if !ok {
		return base
	}
	// elem is set only on a Map/List container, and dynamicContainerKind reflects
	// that kind, so base is Map or List here — not-Map is List (no unreachable arm).
	var typed Value
	if base.Parent.ConformsTo(TMap) {
		typed = core.NewTypedMap(elem)
		typed.Carrier = true
	} else {
		typed = core.NewCarrierTypedListValue(elem)
	}
	typed.Dynamic = true
	typed.SetElemConstraint(elem)
	return typed
}

// mergeReturns: voxgigstruct.Merge over two same-kind containers yields
// that kind; a mixed or scalar pair is value-dependent (later node wins)
// and stays dynamic(Any).
func mergeReturns(args []Value, _ *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 {
		return dyn
	}
	a, b := ValueType(args[0]), ValueType(args[1])
	if a == nil || b == nil {
		return dyn
	}
	// A same-kind merge yields that kind; retain the governing operand's element
	// tag (d2DynamicTypedResidual) so a chained write after a typed merge is
	// diagnosed in check — the residual otherwise loses child + elem (#7, round 7).
	if (a.ConformsTo(TMap) && b.ConformsTo(TMap)) || (a.ConformsTo(TList) && b.ConformsTo(TList)) {
		return []Value{d2DynamicTypedResidual(d2typedMergeOperand(args[0], args[1]))}
	}
	return dyn
}

// mergeListMapReturns / mergeMapListReturns are the check residuals for the two
// index-merge overloads, whose result is a LIST governed by the LIST operand's
// [:T] (the runtime handlers re-tag against it — round 7). The residual carries
// child + elem so a chained write after an index merge is diagnosed in check
// (#8, Codex round 8). List operand: args[0] for list-map, args[1] for map-list.
func mergeListMapReturns(args []Value, _ *Registry) []Value {
	res := NewDynamicCarrier(TList)
	if len(args) == 2 {
		res = d2DynamicTypedResidual(args[0])
	}
	return []Value{res}
}

func mergeMapListReturns(args []Value, _ *Registry) []Value {
	res := NewDynamicCarrier(TList)
	if len(args) == 2 {
		res = d2DynamicTypedResidual(args[1])
	}
	return []Value{res}
}

// setpathReturns mirrors setpathHandler's data-operand selection (the
// position-agnostic path/data/newVal heuristic) and claims the data
// container's kind — setReachNative always returns an updated copy of the
// SAME top-level container.
func setpathReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	pathIdx := -1
	for i := range args {
		if args[i].Parent.ConformsTo(TString) || IsReach(args[i]) {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 || len(args) != 3 {
		return dyn
	}
	var others []int
	for i := range args {
		if i != pathIdx {
			others = append(others, i)
		}
	}
	a, b := args[others[0]], args[others[1]]
	data, newVal := a, b
	if !(a.Parent.ConformsTo(TMap) || a.Parent.ConformsTo(TList)) &&
		(b.Parent.ConformsTo(TMap) || b.Parent.ConformsTo(TList)) {
		data, newVal = b, a
	}
	// R3: a SHALLOW (single-segment) write into a top-level typed container
	// mirrors the runtime element-tag check (setReachNative). A deep path is
	// enforced at runtime only — sound because the compiled + interpreted runs
	// raise the identical type_error (the census stays byte-identical).
	if s, err := AsString(args[pathIdx]); err == nil && !strings.Contains(s, ".") {
		d2CheckWrite(r, data, newVal, "setpath", args[pathIdx].Pos())
	}
	return []Value{d2DynamicTypedResidual(data)}
}

// reifyReturns claims the hydrated instance's type from the TARGET
// operand: a bare type literal yields dynamic(<that type>), a tor union
// (Disjunct — `$class` selects the member at run time) yields the dynamic
// disjunct, anything else stays dynamic(Any).
func reifyReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 {
		return dyn
	}
	target := args[0]
	if target.Carrier || target.Dynamic {
		return dyn // computed target — the class is unknown statically
	}
	// Dry pass (top-level, statically-known target + CONCRETE source):
	// reifyHandler is a pure hydration of the source against the target
	// schema, so a shape violation — unknown field, predicate-field
	// failure, a union source missing its $class discriminator, a
	// non-class target — is a guaranteed runtime error, flagged with the
	// runtime's own code + detail.
	if atUncaughtTopLevel(r) && IsConcrete(args[1]) {
		if _, err := reifyHandler(args, nil, nil, r); err != nil {
			code, detail := "reify_error", err.Error()
			var ae *BoruError
			if errors.As(err, &ae) {
				code, detail = ae.Code, ae.Detail
			}
			core.CheckAddUniqueDiagnostic(r, code, detail, "reify", args[1].Pos())
		}
	}
	if IsDisjunct(target) {
		return []Value{NewDynamicCarrierValue(target)}
	}
	// A bare type node (`reify Foo …`) or a structural type body (`reify
	// Point …` where Point's binding pushes its ClassTypeInfo body):
	// ValueType yields the minted *Type in both cases, exactly as
	// ReturnsFreshInstance resolves make's target.
	if !IsBareTypeNode(target) && !IsTypeBody(target) {
		return dyn
	}
	t := ValueType(target)
	if t == nil || t.Equal(TAny) {
		return dyn
	}
	return []Value{NewDynamicCarrier(t)}
}

// parseTextReturns folds a CONCRETE-source `parse` at check time: the
// jsonic decode is a pure function of the text, so the checked result is
// the runtime result. A non-concrete source, or a text the decoder
// rejects (the raise is the runtime's job — it may be caught by an
// enclosing `do`), falls back to dynamic(Any), exactly the declared-Any
// carrier this sig produced before.
func parseTextReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 1 || !IsConcrete(args[0]) || !args[0].Parent.ConformsTo(TString) {
		return dyn
	}
	out, err := parseTextHandler(args, nil, nil, r)
	if err != nil || len(out) != 1 {
		// A malformed CONCRETE literal is a guaranteed runtime
		// parse_error — surface it on the top-level straight line with
		// the runtime's own code + detail (the dry-pass discipline).
		if err != nil && atUncaughtTopLevel(r) {
			code, detail := "parse_error", err.Error()
			var ae *BoruError
			if errors.As(err, &ae) {
				code, detail = ae.Code, ae.Detail
			}
			core.CheckAddUniqueDiagnostic(r, code, detail, "parse", args[0].Pos())
		}
		return dyn
	}
	return out
}
