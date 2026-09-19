package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct/go"
)

// The "filter" word is registered via the consolidated Natives slice in
// natives.go.
//
// filterHandler calls voxgigstruct.Filter with a boru callback as predicate.
// The callback receives a map with "key" and "value" fields and should return
// a boolean indicating whether to keep the item.
// filterHandler keeps the elements for which the callback returns true. The
// callback (args[0]) is a Function VALUE invoked once per element. Over a LIST
// it receives a {key, value} pair Map (key = index); a predicate reads the
// element via `.value`, e.g. `filter ([p:Any] => [p.value gt 3]) xs`. Over a
// MAP it receives a KeyVal {k v i n} (read the value via `.v`) and the result
// keeps the map shape — see filterMapFunction. (The afn param must be typed —
// a bare `[p]` parses as a type name.)
// filterReturnsFn narrows `filter` to its INPUT collection type: filtering
// keeps a SUBSET of the same shape, so a List stays a List of the same
// element type and a Map stays a Map. Without it filter declared no Returns
// and poisoned every result with dynamic(Any). The predicate body is compiled
// separately by the closure path (filter's declared Callable spec); this types only
// the result. A non-collection or computed receiver keeps dynamic(Any).
func filterReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	data := args[1]
	switch {
	case data.Parent.ConformsTo(TList):
		return []Value{NewCarrierTypedList(DataListElemTypeFromValue(data))}
	case data.Parent.ConformsTo(TMap):
		return []Value{NewCarrier(data.Parent)}
	default:
		return []Value{NewDynamicCarrier(TAny)}
	}
}

func filterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	cb := newFilterCallback(r, args[0])

	// Map input: hand the callback a KeyVal {k v i n} and keep the map shape,
	// consistent with filter's quotation and lens forms. (List input keeps the
	// legacy {key, value} pair with an index key, below.)
	if args[1].Parent.ConformsTo(TMap) && IsConcrete(args[1]) {
		return filterMapFunction(cb, args[1], r)
	}

	data := valueToAny(args[1])

	var callErr error
	result := voxgigstruct.Filter(data, func(pair [2]any) bool {
		if callErr != nil {
			return false
		}

		item := NewOrderedMap()

		keyVal, err := structConvert(pair[0])
		if err != nil {
			keyVal = NewString(fmt.Sprintf("%v", pair[0]))
		}
		item.Set("key", keyVal)

		valVal, err := structConvert(pair[1])
		if err != nil {
			valVal = NewString(fmt.Sprintf("%v", pair[1]))
		}
		item.Set("value", valVal)

		cbArgs := []Value{NewMap(item)}
		cbResult, err := runFilterCallback(r, cb, cbArgs)
		if err != nil {
			callErr = err
			return false
		}
		if len(cbResult) > 0 && cbResult[0].Parent.ConformsTo(TBoolean) {
			b, _ := AsBoolean(cbResult[0])
			return b
		}
		return false
	})

	if callErr != nil {
		return nil, fmt.Errorf("filter: callback error: %w", callErr)
	}

	val, err := structConvert(result)
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	// #4 (round 3): filter keeps a subset of unchanged elements — retain the
	// source list's [:T] tag so downstream reads/writes stay enforced.
	return []Value{d2RetainElem(val, args[1])}, nil
}

// filterMapFunction is the Function form of filter over a map: it calls the
// callback once per entry with a KeyVal {k v i n}, keeping the entries whose
// result is Boolean true and returning a Map (shape-preserving, like the
// quotation and lens forms). A non-Boolean result is a loud error, matching the
// quotation map form rather than dropping silently.
func filterMapFunction(cb filterCallback, mapVal Value, r *Registry) ([]Value, error) {
	data, _ := AsMap(mapVal)
	keys := data.Keys()
	n := int64(len(keys))
	out := NewOrderedMap()
	for idx, k := range keys {
		v, _ := data.Get(k)
		cbArgs := []Value{NewKeyVal(k, v, int64(idx), n)}
		res, err := runFilterCallback(r, cb, cbArgs)
		if err != nil {
			return nil, fmt.Errorf("filter: key %q: %w", k, err)
		}
		if len(res) == 0 {
			return nil, r.BoruError("filter_error", fmt.Sprintf("filter: key %q: predicate produced no result", k), "filter")
		}
		top := res[len(res)-1]
		if !top.Parent.ConformsTo(TBoolean) || !IsConcrete(top) {
			return nil, r.BoruError("filter_error", fmt.Sprintf("filter: key %q: predicate must produce a Boolean, got %s", k, top.Parent.Name()), "filter")
		}
		if b, _ := AsBoolean(top); b {
			out.Set(k, v)
		}
	}
	// #4 (round 3): filter (Function map form) keeps a subset of entries —
	// retain the source map's {:T} tag.
	return []Value{d2RetainElem(NewMap(out), mapVal)}, nil
}

// filterCallback is filter's Function-form callback, prepared ONCE per
// dispatch. A fn-VALUE closure (a capturing `fn` / `=>` literal minted at run
// time: a factory's result, a def-bound one read back) is a fn value to this
// word exactly as it is to the interpreter, so it carries the bridged
// FnDefInfo its declared signature is matched under (sigFn) and the value
// itself carries the SigMatched mark; the bridge is built here rather than
// per entry, where it would be an allocation per element. Every other shape
// leaves sigFn empty and runs as before.
type filterCallback struct {
	val   Value
	sigFn Value
}

func newFilterCallback(r *Registry, cb Value) filterCallback {
	fc := filterCallback{val: cb}
	if IsCompiledClosure(cb) && ClosureIsFnValue(cb) {
		if fnv, ok := ClosureAsFnDef(r, cb); ok {
			fc.sigFn = fnv
			fc.val = ClosureSigMatched(cb)
		}
	}
	return fc
}

// runFilterCallback invokes filter's Function-form callback once for one entry,
// returning its result stack. A compiled CLOSURE (the bytecode VM driving
// filter natively, the body operand lowered to OpPushClosure) runs through the
// InvokeBody seam — its named param binds to the cbArgs shape the closure was
// compiled against ({key,value} pair for a list, KeyVal for a map) — unless it
// is a fn VALUE, which is matched against its own signature first and raises
// the same no-match the lambda branch raises (S1b-2: `filter (mk 1) [1 2]`
// over a `[n:Integer]` closure answered `[]` for the interpreter's
// signature_error). An interpreter FnDefINFO lambda matches a signature and
// runs through InvokeCallbackFn — the VM when the body carries a stamped unit,
// CallBoru otherwise, either way on the fn's DEFINING registry so a predicate
// written in another module resolves its free words there
// (design/FUNCTION-VALUE-SCOPE.0.md). The
// shapes are byte-identical to the handler: all consume cbArgs and yield a
// Boolean predicate result.
func runFilterCallback(r *Registry, fc filterCallback, cbArgs []Value) ([]Value, error) {
	cb := fc.val
	if IsCompiledClosure(cb) {
		if fc.sigFn.Data != nil && MatchFnSig(fc.sigFn, cbArgs) == nil {
			return nil, r.BoruError("signature_error", "filter: no matching callback signature", "filter")
		}
		// The fn-VALUE seam: the FnDefInfo branch below is InvokeCallbackFn's.
		return InvokeCallbackBody(r, cb, cbArgs)
	}
	sig := MatchFnSig(cb, cbArgs)
	if sig == nil {
		// A BoruError, not a bare fmt.Errorf (NUR164): a non-Boru error off
		// the VM reads as an internal bail and re-runs the interpreter.
		return nil, r.BoruError("signature_error", "filter: no matching callback signature", "filter")
	}
	var fnDef *FnDefInfo
	if fd, ok := cb.Data.(FnDefInfo); ok {
		fnDef = &fd
	}
	return InvokeCallbackFn(r, fnDef, sig, cbArgs)
}

// filterBodyHandler is the quotation form of filter: `filter [body] xs`
// runs the quoted body once per element — the element is pushed first,
// then the body tokens, exactly as each/fold run their bodies — and keeps
// the elements whose body result is Boolean true. Unlike the Function
// form there is no {key, value} wrapper: the body sees the element
// directly, so `filter [2 gt] [1 2 3 4]` keeps the elements greater
// than 2. A body result that is not a Boolean is a loud error rather
// than a silent drop (voxgig DX report T9.2 — silent failures are the
// expensive ones). Lists filter to lists; maps filter to maps by value
// (matching the Reach form, filterReachHandler).
func filterBodyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("filter_error", "filter: expected a concrete body list", "filter")
	}
	keep := func(elem Value, label string) (bool, error) {
		res, err := InvokeBody(r, args[0], []Value{elem})
		if err != nil {
			return false, fmt.Errorf("filter: %s: %w", label, err)
		}
		if len(res) == 0 {
			return false, r.BoruError("filter_error", fmt.Sprintf("filter: %s: body produced no result", label), "filter")
		}
		top := res[len(res)-1]
		if !top.Parent.ConformsTo(TBoolean) || !IsConcrete(top) {
			return false, r.BoruError("filter_error", fmt.Sprintf("filter: %s: body must produce a Boolean, got %s", label, top.Parent.Name()), "filter")
		}
		b, _ := AsBoolean(top)
		return b, nil
	}

	switch {
	case args[1].Parent.ConformsTo(TList) && IsConcrete(args[1]):
		data, _ := AsList(args[1])
		out := make([]Value, 0, data.Len())
		for i := 0; i < data.Len(); i++ {
			elem := data.Get(i)
			ok, err := keep(elem, fmt.Sprintf("element %d", i))
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, elem)
			}
		}
		// #4 (round 3): filter (quotation list form) — retain the source [:T] tag.
		return []Value{d2RetainElem(NewList(out), args[1])}, nil
	case args[1].Parent.ConformsTo(TMap) && IsConcrete(args[1]):
		data, _ := AsMap(args[1])
		out := NewOrderedMap()
		for _, k := range data.Keys() {
			v, _ := data.Get(k)
			ok, err := keep(v, fmt.Sprintf("key %q", k))
			if err != nil {
				return nil, err
			}
			if ok {
				out.Set(k, v)
			}
		}
		// #4 (round 3): filter (quotation map form) — retain the source {:T} tag.
		return []Value{d2RetainElem(NewMap(out), args[1])}, nil
	default:
		return nil, r.BoruError("filter_error", "filter: quotation form expects a concrete list or map", "filter")
	}
}

// filterReachHandler is the lens form of filter: it keeps the elements of a
// list (or the values of a map) for which the receiverless Reach (args[0])
// applies to Boolean true. Unlike the Function form, the reach reads the
// element directly — `filter $.active xs` keeps each x where x.active is true.
func filterReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	keep := func(elem Value) (bool, error) {
		res, err := ApplyReach(r, info, elem)
		if err != nil {
			return false, err
		}
		if res.Parent.ConformsTo(TBoolean) {
			b, _ := AsBoolean(res)
			return b, nil
		}
		return false, nil
	}

	switch {
	case args[1].Parent.ConformsTo(TList) && IsConcrete(args[1]):
		data, _ := AsList(args[1])
		out := make([]Value, 0, data.Len())
		for i := 0; i < data.Len(); i++ {
			elem := data.Get(i)
			ok, err := keep(elem)
			if err != nil {
				return nil, fmt.Errorf("filter: element %d: %w", i, err)
			}
			if ok {
				out = append(out, elem)
			}
		}
		// #4 (round 3): filter (lens list form) — retain the source [:T] tag.
		return []Value{d2RetainElem(NewList(out), args[1])}, nil
	case args[1].Parent.ConformsTo(TMap) && IsConcrete(args[1]):
		data, _ := AsMap(args[1])
		out := NewOrderedMap()
		for _, k := range data.Keys() {
			v, _ := data.Get(k)
			ok, err := keep(v)
			if err != nil {
				return nil, fmt.Errorf("filter: key %q: %w", k, err)
			}
			if ok {
				out.Set(k, v)
			}
		}
		// #4 (round 3): filter (lens map form) — retain the source {:T} tag.
		return []Value{d2RetainElem(NewMap(out), args[1])}, nil
	default:
		return nil, r.BoruError("filter_error", "filter: lens form expects a concrete list or map", "filter")
	}
}
