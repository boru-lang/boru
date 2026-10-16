package native

// booleanNatives covers the boolean / logical-connective words.
// The classical trio (`not`, `and`, `or`) follows Lisp/Python
// short-circuit semantics — `0 or 5` returns 5, `1 and 2` returns 2,
// and `not v` always returns a Boolean. The remaining connectives
// (`otherwise`, `xor`, `nand`, `nor`, `iff`, `xnor`) coerce
// non-Boolean inputs via CoerceBoolean. `any` and `all` are the
// list-reduction forms of `or` / `and`; they always return Boolean.
//
// Algorithms live in the eng layer (CoerceBoolean,
// FlattenDisjunctAlts, TandValues etc.); this file owns the word
// names, signatures, and dispatch wiring.
var booleanNatives = []NativeFunc{
	{
		Name: "not",

		Signatures: []Signature{
			{Args: []*Type{TBoolean}, Impl: Go(notHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny}, Impl: Go(notHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "and",

		Signatures: []Signature{
			{Args: []*Type{TBoolean, TBoolean}, Impl: Go(andHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(andHandler), ReturnsFn: foldOrJoin(andHandler), BarrierPos: -1},
		},
	},
	{
		Name: "or",

		Signatures: []Signature{
			{Args: []*Type{TBoolean, TBoolean}, BarrierPos: 1, Impl: Go(orHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Impl: Go(orHandler), ReturnsFn: foldOrJoin(orHandler)},
		},
	},
	{
		Name: "otherwise",

		Signatures: []Signature{
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Impl: Go(otherwiseHandler), ReturnsFn: foldOrJoin(otherwiseHandler)},
		},
	},
	boolBinaryNative("xor", func(a, b bool) bool { return a != b }),
	// nand / nor / iff / xnor (and `implies`) moved to the boru:logic-util
	// module — see native/logic_module.go.
	{
		Name: "any",

		Signatures: []Signature{
			{Args: []*Type{TList}, Impl: Go(anyHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "all",

		Signatures: []Signature{
			{Args: []*Type{TList}, Impl: Go(allHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
}

// anyHandler returns true iff any element of the list is truthy.
// Empty list returns false (the identity for OR).
func anyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return []Value{NewBoolean(false)}, nil
	}
	list, _ := AsList(args[0])
	n := list.Len()
	for i := 0; i < n; i++ {
		if CoerceBoolean(list.Get(i)) {
			return []Value{NewBoolean(true)}, nil
		}
	}
	return []Value{NewBoolean(false)}, nil
}

// allHandler returns true iff every element of the list is truthy.
// Empty list returns true (the identity for AND).
func allHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return []Value{NewBoolean(true)}, nil
	}
	list, _ := AsList(args[0])
	n := list.Len()
	for i := 0; i < n; i++ {
		if !CoerceBoolean(list.Get(i)) {
			return []Value{NewBoolean(false)}, nil
		}
	}
	return []Value{NewBoolean(true)}, nil
}

// notHandler always returns a Boolean.
func notHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!CoerceBoolean(args[0]))}, nil
}

// operandJoinReturns is the check-mode return for the value-selecting
// connectives and/or/otherwise: each yields ONE of its two operands
// (Python/Lisp short-circuit — `3 and 0 → 0`, `'x' or 'd' → 'x'`,
// `5 otherwise 9 → 5`), so the result type is the join of the operand
// carriers, not Any. Narrowing only: the runtime result is always one of
// the operands, so the join soundly covers it while a downstream typed
// use now sees the real type instead of a poison Any.
func operandJoinReturns(args []Value, _ *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{JoinCarriers(args[0], args[1])}
}

// foldOrJoin is the check-mode return for a value-selecting connective
// that CONCRETE-FOLDS when it can. When both operands are concrete the
// connective's choice is statically determined, so it runs the (pure)
// handler to get the EXACT selected operand — `and 0 false` types
// Boolean, not the imprecise Disjunct(Integer,Boolean) join. The precise
// type then lets a downstream op reject the real type instead of
// optimistically accepting a union member that won't exist at runtime
// (e.g. `add (and 0 false) 0` correctly fails the checker rather than
// raising signature_error at run). With a non-concrete operand the
// selection is unknown, so it falls back to the operand join — unchanged.
func foldOrJoin(handler func([]Value, map[string]Value, []Value, *Registry) ([]Value, error)) func([]Value, *Registry) []Value {
	return func(args []Value, r *Registry) []Value {
		if len(args) == 2 && IsConcrete(&args[0]) && IsConcrete(&args[1]) {
			if out, err := handler(args, nil, nil, r); err == nil && len(out) == 1 {
				return out
			}
		}
		return operandJoinReturns(args, r)
	}
}

// andHandler returns args[1] when falsy, else args[0]. Short-circuit.
func andHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

// orHandler returns args[1] when truthy, else args[0]. Short-circuit.
func orHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

// boolBinaryNative builds a NativeFunc with two signatures —
// [TBoolean, TBoolean] and [TAny, TAny] — both routing through the
// same boolean-coercing handler. Used for the classical connectives
// (xor, nand, nor, iff, xnor) that share an identical shape.
func boolBinaryNative(name string, fn func(a, b bool) bool) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := CoerceBoolean(args[0])
		b := CoerceBoolean(args[1])
		return []Value{NewBoolean(fn(a, b))}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []Signature{
			{Args: []*Type{TBoolean, TBoolean}, Impl: Go(handler), Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(handler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	}
}

func otherwiseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// IsNoneShape matches both the sentinel `none` AND the bare type
	// literal `None` — handlers throughout the codebase return the
	// latter for "no value", so the LHS check has to cover it too.
	if IsNoneShape(args[1]) {
		return []Value{args[0]}, nil
	}
	return []Value{args[1]}, nil
}
