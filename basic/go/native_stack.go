package basic

import "fmt"

// StackNatives covers the stack-manipulation primitives. All are
// stack-only (ForwardArgs=false). Argument convention is
// post-§1.4 unified: args[0] is the top of stack, args[1] is the
// next-deeper element, etc. Splice ordering: the returned []Value
// is laid back onto the stack in source order, so an N-arg word
// that returns the same N values produces the inputs unchanged
// (see swap for a worked example).
var StackNatives = []NativeFunc{
	{
		Name: "dup",

		Signatures: []Signature{{
			Args:      []*Type{TAny},
			Impl:      Go(dupHandler),
			ReturnsFn: ReturnsIdentity(0, 0), BarrierPos: 0,
		}},
	},
	{
		Name: "swap",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(swapHandler),
			ReturnsFn: ReturnsIdentity(0, 1), BarrierPos: 0,
		}},
	},
	{
		Name: "drop",

		Signatures: []Signature{{
			Args:    []*Type{TAny},
			Impl:    Go(dropHandler),
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "over",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(overHandler),
			ReturnsFn: ReturnsIdentity(1, 0, 1), BarrierPos: 0,
		}},
	},
	{
		Name: "rot",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny, TAny},
			Impl:      Go(rotHandler),
			ReturnsFn: ReturnsIdentity(1, 0, 2), BarrierPos: 0,
		}},
	},
	{
		Name: "nip",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(nipHandler),
			ReturnsFn: ReturnsIdentity(0), BarrierPos: 0,
		}},
	},
	{
		Name: "tuck",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(tuckHandler),
			ReturnsFn: ReturnsIdentity(0, 1, 0), BarrierPos: 0,
		}},
	},
	{
		Name: "dup2",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny},
			Impl:      Go(dup2Handler),
			ReturnsFn: ReturnsIdentity(1, 0, 1, 0), BarrierPos: 0,
		}},
	},
	{
		Name: "swap2",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny, TAny, TAny},
			Impl:      Go(swap2Handler),
			ReturnsFn: ReturnsIdentity(1, 0, 3, 2), BarrierPos: 0,
		}},
	},
	{
		Name: "drop2",

		Signatures: []Signature{{
			Args:    []*Type{TAny, TAny},
			Impl:    Go(drop2Handler),
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "over2",

		Signatures: []Signature{{
			Args:      []*Type{TAny, TAny, TAny, TAny},
			Impl:      Go(over2Handler),
			ReturnsFn: ReturnsIdentity(3, 2, 1, 0, 3, 2), BarrierPos: 0,
		}},
	},
	{
		Name: "depth",

		Signatures: []Signature{{
			Impl:       Go(depthHandler, FullStack(), CheckFullStack(depthCheckFullStack)),
			BarrierPos: 0,
		}},
	},
	{
		Name: "pick",

		Signatures: []Signature{{
			Args:       []*Type{TInteger},
			Impl:       Go(pickHandler, FullStack(), CheckFullStack(PickCheckFullStack)),
			BarrierPos: 0,
		}},
	},
	{
		Name: "roll",

		Signatures: []Signature{{
			Args:       []*Type{TInteger},
			Impl:       Go(rollHandler, FullStack(), CheckFullStack(RollCheckFullStack)),
			BarrierPos: 0,
		}},
	},
}

func dupHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[0]}, nil
}

func swapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[1]}, nil
}

func dropHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return nil, nil
}

func overHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[1]}, nil
}

func rotHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[2]}, nil
}

func nipHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0]}, nil
}

func tuckHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[1], args[0]}, nil
}

func dup2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[1], args[0]}, nil
}

func swap2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[3], args[2]}, nil
}

func drop2Handler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return nil, nil
}

func over2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[3], args[2], args[1], args[0], args[3], args[2]}, nil
}

func depthHandler(_ []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	return append(stack, NewInteger(int64(len(stack)))), nil
}

func depthCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	return append(append([]Value(nil), stack...), NewCarrier(TInteger))
}

func pickHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n >= len(stack) {
		return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
	}
	return append(stack, stack[len(stack)-1-n]), nil
}

func PickCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	if len(stack) == 0 {
		return append(append([]Value(nil), stack...), NewCarrier(TAny))
	}
	t := ParentOf(&stack[0])
	for i := 1; i < len(stack); i++ {
		t = CommonAncestorType(t, ParentOf(&stack[i]))
		if t.Equal(TAny) {
			break
		}
	}
	return append(append([]Value(nil), stack...), NewCarrier(t))
}

func rollHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n >= len(stack) {
		return nil, fmt.Errorf("roll: index %d out of range (stack depth %d)", n, len(stack))
	}
	idx := len(stack) - 1 - n
	result := make([]Value, 0, len(stack))
	result = append(result, stack[:idx]...)
	result = append(result, stack[idx+1:]...)
	result = append(result, stack[idx])
	return result, nil
}

func RollCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	if len(stack) == 0 {
		return nil
	}
	out := append([]Value(nil), stack...)
	t := ParentOf(&stack[0])
	for i := 1; i < len(stack); i++ {
		t = CommonAncestorType(t, ParentOf(&stack[i]))
	}
	out[len(out)-1] = NewCarrier(t)
	return out
}
