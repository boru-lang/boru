package native

import "fmt"

// Map overloads for the higher-order words each / for-each / fold (and the
// Function form of filter, in filter.go), plus the keys / vals projections.
//
// A Map is iterated entry by entry in insertion order. Two body forms, mirrored
// across every word:
//
//   - quotation `[body]` — the entry's VALUE is pushed on the stack; the result
//     keeps the map shape (same keys). e.g. `{a:1 b:2} each [mul 10]`.
//   - lambda `(kv => …)` — the body is handed a KeyVal {k v i n} so it can use
//     the key/index/total; the result still keeps the map shape.
//     e.g. `{a:1 b:2} each (kv => [kv.v mul 10])`.
//
// each → Map (values transformed, keys kept); for-each → nothing;
// fold → the accumulator; filter → Map (entries kept by a Boolean predicate).
// To leave the map shape and get a List, use keys / vals (or StructUtil.items).

// mapBody is a prepared iteration body — either a quotation (token list) or a
// lambda (Function value) — ready to run once per map entry.
type mapBody struct {
	lambda  bool
	closure bool       // when a compiled-closure body (VM-driven)
	fn      Value      // when lambda: the Function value
	body    Value      // when closure: the closure value (run via InvokeBody)
	fnDef   *FnDefInfo // when lambda: its definition (captures + defining registry)
	tokens  []Value    // when quotation: the body tokens
}

// newMapBody classifies the body arg: a compiled CLOSURE (the bytecode VM
// driving each/fold over a map) runs per VALUE via the InvokeBody seam, like a
// quotation; a (lambda) Function is handed a KeyVal; anything else must be a
// concrete quotation list (handed the value).
func newMapBody(reg *Registry, body Value, word string) (mapBody, error) {
	if IsCompiledClosure(body) {
		return mapBody{closure: true, body: body}, nil
	}
	if body.Parent.ConformsTo(TFunction) {
		mb := mapBody{lambda: true, fn: body}
		if fd, ok := body.Data.(FnDefInfo); ok {
			mb.fnDef = &fd
		}
		return mb, nil
	}
	if !IsConcrete(body) {
		return mapBody{}, reg.BoruError(word+"_error", word+": body must be a quotation list or a lambda", word)
	}
	bl, _ := AsList(body)
	return mapBody{tokens: bl.Slice()}, nil
}

// value runs the body for one entry with no accumulator. ok=false when the body
// left the stack empty.
func (mb mapBody) value(reg *Registry, k string, v Value, i, n int64) (Value, bool, error) {
	if mb.lambda {
		return mb.callLambda(reg, []Value{NewKeyVal(k, v, i, n)})
	}
	if mb.closure {
		// A closure compiled from a LAMBDA body expects a KeyVal (its named
		// param destructures `kv.v`/`kv.i`); one compiled from a token body
		// sees the bare value, like a quotation. The unit's recorded shape says
		// which (ClosureWantsKeyVal).
		if ClosureWantsKeyVal(mb.body) {
			return invokeBodyTop(reg, mb.body, []Value{NewKeyVal(k, v, i, n)})
		}
		return invokeBodyTop(reg, mb.body, []Value{v})
	}
	return runQuotationBody(reg, mb.tokens, []Value{v})
}

// fold runs the body for one entry with an accumulator. The quotation form
// pushes the accumulator first and the value on top (same stack order as list
// fold: a 2-arg word sees value=top, acc=deeper); the lambda receives
// (accumulator, KeyVal).
func (mb mapBody) fold(reg *Registry, acc Value, k string, v Value, i, n int64) (Value, bool, error) {
	if mb.lambda {
		return mb.callLambda(reg, []Value{acc, NewKeyVal(k, v, i, n)})
	}
	if mb.closure {
		// (accumulator, entry): a lambda-derived closure takes the entry as a
		// KeyVal, a token-derived one as the bare value.
		if ClosureWantsKeyVal(mb.body) {
			return invokeBodyTop(reg, mb.body, []Value{acc, NewKeyVal(k, v, i, n)})
		}
		return invokeBodyTop(reg, mb.body, []Value{acc, v})
	}
	return runQuotationBody(reg, mb.tokens, []Value{acc, v})
}

// invokeBodyTop runs a (closure) body via the fn-VALUE seam
// (InvokeCallbackBody — the closure twin of callLambda's InvokeCallbackFn, so
// a compiled lambda's return contract is applied as CallBoru's would be) and
// returns its residual top of stack — the closure mirror of runQuotationBody.
func invokeBodyTop(reg *Registry, body Value, inputs []Value) (Value, bool, error) {
	res, err := InvokeCallbackBody(reg, body, inputs)
	if err != nil {
		return Value{}, false, err
	}
	if len(res) == 0 {
		return Value{}, false, nil
	}
	return res[len(res)-1], true, nil
}

func (mb mapBody) callLambda(reg *Registry, args []Value) (Value, bool, error) {
	sig := MatchFnSig(mb.fn, args)
	if sig == nil {
		// A BoruError, not a bare fmt.Errorf (NUR164): the compiled-by-default
		// lane reads every non-Boru error off the VM as an internal bail and
		// re-runs the whole program on the interpreter, so a plain Go error
		// here turned a correct compiled verdict into a silent re-run.
		return Value{}, false, reg.BoruError("signature_error",
			fmt.Sprintf("no matching lambda signature for %d argument(s)", len(args)), "")
	}
	// InvokeCallbackFn, not reg.CallBoru: a lambda passed in from another module
	// runs on its DEFINING registry (design/FUNCTION-VALUE-SCOPE.0.md), and a
	// stamped body runs on the VM.
	res, err := InvokeCallbackFn(reg, mb.fnDef, sig, args)
	if err != nil {
		return Value{}, false, err
	}
	if len(res) == 0 {
		return Value{}, false, nil
	}
	return res[len(res)-1], true, nil
}

// runQuotationBody places `pushed` (deepest first) as resolved inputs, then
// runs the body tokens on a pooled sub-engine, and returns the residual top
// of stack.
func runQuotationBody(reg *Registry, tokens []Value, pushed []Value) (Value, bool, error) {
	// An EMPTY quotation is the IDENTITY on its inputs — the engine would place
	// them and hand them straight back — so its residual IS `pushed` and there
	// is nothing to run. `walk`'s optional ascend slot reaches this whenever a
	// trailing value gets forward-collected into it (`walk {…} data (hook) acc`
	// binds `acc`, an empty flex list, as the ascend hook), and running that on
	// an engine once per visited node is an interpreter entry inside a compiled
	// program that buys nothing.
	res := pushed
	if len(tokens) > 0 {
		var err error
		if res, err = RunResolved(reg, pushed, tokens); err != nil {
			return Value{}, false, err
		}
	}
	if len(res) == 0 {
		return Value{}, false, nil
	}
	return res[len(res)-1], true, nil
}

// requireConcreteMap unwraps a concrete Map arg or returns a clear error.
func requireConcreteMap(reg *Registry, v Value, word string) (ReadMap, error) {
	if !IsConcrete(v) || !v.Parent.ConformsTo(TMap) {
		return nil, reg.BoruError(word+"_error", word+": expected a concrete map", word)
	}
	m, _ := AsMap(v)
	if m == nil {
		return nil, reg.BoruError(word+"_error", word+": expected a concrete map", word)
	}
	return m, nil
}

// eachMapHandler maps a body over a map's values, keeping the keys — `mapValues`.
// Backs the `[TList, TMap]` (quotation) and `[TFunction, TMap]` (lambda) sigs.
func eachMapHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteList(args[1]) {
		return eachHandler(args, nil, nil, reg)
	}
	data, err := requireConcreteMap(reg, args[1], "each")
	if err != nil {
		return nil, err
	}
	mb, err := newMapBody(reg, args[0], "each")
	if err != nil {
		return nil, err
	}
	keys := data.Keys()
	n := int64(len(keys))
	out := NewOrderedMap()
	for idx, k := range keys {
		v, _ := data.Get(k)
		res, ok, err := mb.value(reg, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("each: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.BoruError("each_error", fmt.Sprintf("each: key %q: body produced no result", k), "each")
		}
		out.Set(k, res)
	}
	return []Value{NewMap(out)}, nil
}

// forEachMapHandler runs the body once per entry for side effects, producing
// nothing. Backs the `[TList, TMap]` and `[TFunction, TMap]` sigs.
func forEachMapHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[1], "for-each")
	if err != nil {
		return nil, err
	}
	mb, err := newMapBody(reg, args[0], "for-each")
	if err != nil {
		return nil, err
	}
	keys := data.Keys()
	n := int64(len(keys))
	for idx, k := range keys {
		v, _ := data.Get(k)
		if _, _, err := mb.value(reg, k, v, int64(idx), n); err != nil {
			return nil, fmt.Errorf("for-each: key %q: %w", k, err)
		}
	}
	return nil, nil
}

// foldMapInitHandler reduces a map's entries with an explicit seed —
// `init fold [body] {map}`. Backs `[TList, TMap, TAny]` / `[TFunction, TMap, TAny]`.
func foldMapInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteList(args[1]) {
		return foldWithInitHandler(args, nil, nil, reg)
	}
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	return doFoldMap(reg, args[0], args[2], data, 0)
}

// foldMapNoInitHandler reduces a map's entries, seeding from the first value —
// `fold [body] {map}`. Backs `[TList, TMap]` / `[TFunction, TMap]`.
func foldMapNoInitHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteList(args[1]) {
		return foldNoInitHandler(args, nil, nil, reg)
	}
	data, err := requireConcreteMap(reg, args[1], "fold")
	if err != nil {
		return nil, err
	}
	keys := data.Keys()
	if len(keys) == 0 {
		return nil, reg.BoruError("fold_error", "fold: empty map with no initial value", "fold")
	}
	first, _ := data.Get(keys[0])
	return doFoldMap(reg, args[0], first, data, 1)
}

// doFoldMap threads the accumulator from `start` onward over the map's entries,
// passing each entry's original index/total to a lambda's KeyVal.
func doFoldMap(reg *Registry, body, acc Value, data ReadMap, start int) ([]Value, error) {
	mb, err := newMapBody(reg, body, "fold")
	if err != nil {
		return nil, err
	}
	keys := data.Keys()
	n := int64(len(keys))
	for idx := start; idx < len(keys); idx++ {
		k := keys[idx]
		v, _ := data.Get(k)
		res, ok, err := mb.fold(reg, acc, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("fold: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.BoruError("fold_error", fmt.Sprintf("fold: key %q: body produced no result", k), "fold")
		}
		acc = res
	}
	return []Value{acc}, nil
}

// scanMapHandler is the running (prefix) fold over a map's values: the first
// value seeds the accumulator and is the first output, then each later entry's
// body result becomes that key's output. Keeps the map shape (keys preserved).
// Backs the `[TList, TMap]` (quotation) and `[TFunction, TMap]` (lambda) sigs.
func scanMapHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	if collIsConcreteList(args[1]) {
		return scanHandler(args, nil, nil, reg)
	}
	data, err := requireConcreteMap(reg, args[1], "scan")
	if err != nil {
		return nil, err
	}
	mb, err := newMapBody(reg, args[0], "scan")
	if err != nil {
		return nil, err
	}
	keys := data.Keys()
	out := NewOrderedMap()
	if len(keys) == 0 {
		return []Value{NewMap(out)}, nil
	}
	n := int64(len(keys))
	acc, _ := data.Get(keys[0])
	out.Set(keys[0], acc) // first value seeds and is the first output
	for idx := 1; idx < len(keys); idx++ {
		k := keys[idx]
		v, _ := data.Get(k)
		res, ok, err := mb.fold(reg, acc, k, v, int64(idx), n)
		if err != nil {
			return nil, fmt.Errorf("scan: key %q: %w", k, err)
		}
		if !ok {
			return nil, reg.BoruError("scan_error", fmt.Sprintf("scan: key %q: body produced no result", k), "scan")
		}
		acc = res
		out.Set(k, acc)
	}
	return []Value{NewMap(out)}, nil
}

// keysHandler returns a map's keys as a list, in insertion order.
func keysHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[0], "keys")
	if err != nil {
		return nil, err
	}
	ks := data.Keys()
	out := make([]Value, len(ks))
	for i, k := range ks {
		out[i] = NewString(k)
	}
	return []Value{NewList(out)}, nil
}

// valsHandler returns a map's values as a list, in insertion order.
func valsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	data, err := requireConcreteMap(reg, args[0], "vals")
	if err != nil {
		return nil, err
	}
	ks := data.Keys()
	out := make([]Value, len(ks))
	for i, k := range ks {
		v, _ := data.Get(k)
		out[i] = v
	}
	// #5 (Codex round 5): a {:T} map's values are all element-typed, so the
	// values-list projection is [:T] — retain the tag (elem is container-kind
	// agnostic). keys stays untagged (keys are always String).
	return []Value{d2RetainElem(NewList(out), args[0])}, nil
}

// valsReturns is the check-mode mirror: a `{:T}` map projects to a `[:T]` list,
// so a read from `(vals m)` narrows and a write into it stays enforced.
func valsReturns(args []Value, _ *Registry) []Value {
	res := NewCarrier(TList)
	if len(args) == 1 {
		res = d2TypedListResidual(args[0])
	}
	return []Value{res}
}

// mapNatives are the standalone map-projection words. The each / for-each /
// fold Map overloads live on those words' own signatures (native_array.go); the
// filter Function-over-map path lives in filter.go.
var mapNatives = []NativeFunc{
	{
		Name: "keys",
		Signatures: []Signature{
			{Args: []*Type{TMap}, Impl: Go(keysHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "vals",
		Signatures: []Signature{
			{Args: []*Type{TMap}, Impl: Go(valsHandler), Returns: []*Type{TList}, ReturnsFn: valsReturns, BarrierPos: -1},
		},
	},
}
