package native

import "fmt"

// The "filter" word is registered via the consolidated Natives slice in
// natives.go. Three predicate forms share the one rule: the predicate sees
// the container's natural unit — the ELEMENT of a list, the VALUE of a map
// entry — whether it is a quotation (filterBodyHandler), a Reach lens
// (filterReachHandler) or a Function value (filterHandler); a Function that
// types its param KeyVal is handed the whole map entry (filterMapFunction).
//
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

// filterHandler is the Function form of filter — `filter (predicate) xs` with
// a Function VALUE (a lambda, a named fn's /v reference, a compiled closure)
// invoked once per element. The predicate is handed the container's natural
// unit, exactly what the quotation form pushes: over a LIST the ELEMENT
// (`filter ([p:Integer] => [p gt 3]) xs`), over a MAP the entry's VALUE — or
// the whole entry as a KeyVal {k v i n} when the predicate types its param
// KeyVal (`filter ([kv:KeyVal] => [kv.k neq 'a']) m`; core.CallbackWantsKeyVal)
// — keeping the map shape (filterMapFunction). The result keeps the elements
// (entries) whose result is Boolean true; a non-Boolean result is a loud
// error, never a silent drop (voxgig DX report T9.2), and so is a receiver
// that is neither a concrete list nor a concrete map.
//
// (Until 2026-10-06 the list form handed a {key value} pair map — the index
// and the element — read via `.value`, "filter's documented exception" to
// the natural-unit rule; design/IMMUTABLE-DEF.1.md §5 phase 1 retired the
// descriptor so every Function form hands what its quotation form does.)
func filterHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	cb := newFilterCallback(r, args[0])
	switch {
	case args[1].Parent.ConformsTo(TMap) && IsConcrete(args[1]):
		return filterMapFunction(cb, args[1], r)
	case args[1].Parent.ConformsTo(TList) && IsConcrete(args[1]):
		data, _ := AsList(args[1])
		out := make([]Value, 0, data.Len())
		for i := 0; i < data.Len(); i++ {
			elem := data.Get(i)
			keep, err := cb.keep(r, []Value{elem}, fmt.Sprintf("element %d", i))
			if err != nil {
				return nil, err
			}
			if keep {
				out = append(out, elem)
			}
		}
		// #4 (round 3): filter keeps a subset of unchanged elements — retain
		// the source list's [:T] tag so downstream reads/writes stay enforced.
		return []Value{d2RetainElem(NewList(out), args[1])}, nil
	default:
		return nil, r.BoruError("filter_error", "filter: Function form expects a concrete list or map", "filter")
	}
}

// filterMapFunction is the Function form of filter over a map: it calls the
// predicate once per entry — with the entry's VALUE, or with the KeyVal
// {k v i n} a KeyVal-typed param asks for (filterCallback.keyVal) — keeping the
// entries whose result is Boolean true and returning a Map (shape-preserving,
// like the quotation and lens forms).
func filterMapFunction(cb filterCallback, mapVal Value, r *Registry) ([]Value, error) {
	data, _ := AsMap(mapVal)
	keys := data.Keys()
	n := int64(len(keys))
	out := NewOrderedMap()
	for idx, k := range keys {
		v, _ := data.Get(k)
		entry := v
		if cb.keyVal {
			entry = NewKeyVal(k, v, int64(idx), n)
		}
		keep, err := cb.keep(r, []Value{entry}, fmt.Sprintf("key %q", k))
		if err != nil {
			return nil, err
		}
		if keep {
			out.Set(k, v)
		}
	}
	// #4 (round 3): filter (Function map form) keeps a subset of entries —
	// retain the source map's {:T} tag.
	return []Value{d2RetainElem(NewMap(out), mapVal)}, nil
}

// filterCallback is filter's Function-form predicate, prepared ONCE per
// dispatch. A compiled closure with a param contract of its own — a fn-VALUE
// closure (a capturing `fn` / `=>` literal minted at run time: a factory's
// result, a def-bound one read back) or a lambda compiled at the call site —
// is a fn value to this word exactly as it is to the interpreter, so it
// carries the bridged FnDefInfo its declared signature is matched under
// (sigFn) and the value itself carries the SigMatched mark; the bridge is
// built here rather than per element, where it would be an allocation per
// element. Every other shape leaves sigFn empty. keyVal is the signature's
// answer to the entry rule (core.CallbackWantsKeyVal): over a map, a
// KeyVal-typed param is handed the entry, any other the value.
type filterCallback struct {
	val    Value
	sigFn  Value
	keyVal bool
}

func newFilterCallback(r *Registry, cb Value) filterCallback {
	fc := filterCallback{val: cb, keyVal: CallbackWantsKeyVal(cb)}
	if IsCompiledClosure(cb) {
		if fnv, ok := ClosureAsFnDef(r, cb); ok {
			fc.sigFn = fnv
			fc.val = ClosureSigMatched(cb)
			fc.keyVal = CallbackWantsKeyVal(fnv)
		}
	}
	return fc
}

// keep runs the predicate over cbArgs (one element or entry) and reads its
// verdict: the Boolean result, or an error for a callback error, an empty
// result or a non-Boolean result — label names the element or key.
func (fc filterCallback) keep(r *Registry, cbArgs []Value, label string) (bool, error) {
	res, err := runFilterCallback(r, fc, cbArgs)
	if err != nil {
		return false, fmt.Errorf("filter: %s: %w", label, err)
	}
	if len(res) == 0 {
		return false, r.BoruError("filter_error", fmt.Sprintf("filter: %s: predicate produced no result", label), "filter")
	}
	top := res[len(res)-1]
	if !top.Parent.ConformsTo(TBoolean) || !IsConcrete(top) {
		return false, r.BoruError("filter_error", fmt.Sprintf("filter: %s: predicate must produce a Boolean, got %s", label, top.Parent.Name()), "filter")
	}
	b, _ := AsBoolean(top)
	return b, nil
}

// runFilterCallback invokes filter's Function-form predicate once for one
// element or entry, returning its result stack. A compiled CLOSURE (the
// bytecode VM driving filter natively, the body operand lowered to
// OpPushClosure) with a contract of its own is matched against its bridged
// signature first (sigFn), raising the same no-match the lambda branch raises
// (S1b-2: `filter (mk 1) [1 2]` over a `[n:Integer]` closure answered `[]` for
// the interpreter's signature_error), then runs through the fn-VALUE seam
// (InvokeCallbackBody); a contract-less closure runs through the seam
// unmatched. An interpreter FnDefInfo lambda matches a signature and runs
// through InvokeCallbackFn — the VM when the body carries a stamped unit,
// CallBoru otherwise, either way on the fn's DEFINING registry so a predicate
// written in another module resolves its free words there
// (design/FUNCTION-VALUE-SCOPE.0.md). The shapes are byte-identical to the
// handler: all consume cbArgs and yield a Boolean predicate result.
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
// the elements whose body result is Boolean true. The body sees the
// element directly — as the Function form's predicate does — so
// `filter [2 gt] [1 2 3 4]` keeps the elements greater than 2. A body
// result that is not a Boolean is a loud error rather
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
		if BodyEscaped(r) {
			return false, nil // the body's break/continue ends the filter; the loops below return
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
			if BodyEscaped(r) {
				return nil, nil
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
			if BodyEscaped(r) {
				return nil, nil
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
// applies to Boolean true. The reach reads the element directly, as the
// Function form's predicate does — `filter $.active xs` keeps each x where
// x.active is true.
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
