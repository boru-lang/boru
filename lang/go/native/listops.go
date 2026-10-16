package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// The list-mutation words (push/pop/unshift/shift) are registered via the
// consolidated Natives slice in natives.go.
//
// plainListGrowReturns is the check-mode mirror for an immutable-list grow
// (push / unshift over a plain [:T] list): args are [value, list]. It flags a
// provably non-conforming top-level grow (d2CheckWrite) and retains the source
// list's tag on the residual (a both-fields typed carrier) so a chained write
// stays enforced — the plain grows otherwise had no ReturnsFn (#10, Codex round
// 10), unlike their FlexList counterparts (flexGrowReturns).
func plainListGrowReturns(word string) func([]Value, *Registry) []Value {
	return func(args []Value, r *Registry) []Value {
		res := NewCarrier(TList)
		if len(args) == 2 {
			d2CheckWrite(r, args[1], args[0], word, args[0].Pos())
			res = d2TypedListResidual(args[1])
		}
		return []Value{res}
	}
}

// push appends a single element to the end of a list, returning a new list.
//
//	push 99 [1,2,3] → [1,2,3,99]
//	[1,2,3] 99 push → [1,2,3,99]
func pushHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	// A typed list ([:T]) enforces + recursively re-tags the grown element —
	// the immutable grow must uphold the same element invariant `set` does.
	tagged, terr := d2AdoptTyped(r, args[1], args[0], "push")
	if terr != nil {
		return nil, terr
	}
	// The copy-returning column stays ENTIRELY immutable: an element
	// with flex inside is snapshot to its plain shape (AdoptIntoNode).
	newElem, aerr := core.AdoptIntoNode(tagged)
	if aerr != nil {
		return nil, r.BoruError("push_error", aerr.Error(), "push")
	}
	// ReadList.Slice always allocates (make), so it never returns nil — a
	// "list == nil" guard here would be dead (design/TEST-SEAMS.10.md,
	// ADR-005). A non-concrete List arg yields an empty slice and pushes
	// onto it, matching the pre-existing behaviour of the dead guard.
	_lst, _ := AsList(args[1])
	list := _lst.Elems()

	result := make([]Value, len(list)+1)
	copy(result, list)
	result[len(list)] = newElem

	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// pop removes the last element from a list, returning the new list
// and the removed element.
//
//	pop [a,b,c] → [a,b] c
//	[a,b,c] pop → [a,b] c
func popHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Elems()
	if len(list) == 0 {
		return nil, r.BoruError("pop_error", "pop: cannot pop from empty list", "pop")
	}

	newList := make([]Value, len(list)-1)
	copy(newList, list[:len(list)-1])
	popped := list[len(list)-1]

	return []Value{d2RetainElem(NewList(newList), args[0]), popped}, nil
}

// unshift prepends a single element to the beginning of a list, returning a new list.
//
//	unshift 99 [1,2,3] → [99,1,2,3]
//	[1,2,3] 99 unshift → [99,1,2,3]
func unshiftHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	// A typed list ([:T]) enforces + recursively re-tags the grown element.
	tagged, terr := d2AdoptTyped(r, args[1], args[0], "unshift")
	if terr != nil {
		return nil, terr
	}
	// Entirely-immutable invariant for the copy-returning column: see
	// pushHandler.
	newElem, aerr := core.AdoptIntoNode(tagged)
	if aerr != nil {
		return nil, r.BoruError("unshift_error", aerr.Error(), "unshift")
	}
	// Dead "list == nil" guard removed: ReadList.Slice always allocates
	// (design/TEST-SEAMS.10.md, ADR-005). See pushHandler.
	_lst, _ := AsList(args[1])
	list := _lst.Elems()

	result := make([]Value, len(list)+1)
	result[0] = newElem
	copy(result[1:], list)

	return []Value{d2RetainElem(NewList(result), args[1])}, nil
}

// shift removes the first element from a list, returning the new list
// and the removed element.
//
//	shift [a,b,c] → [b,c] a
//	[a,b,c] shift → [b,c] a
func shiftHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Elems()
	if len(list) == 0 {
		return nil, r.BoruError("shift_error", "shift: cannot shift from empty list", "shift")
	}

	shifted := list[0]
	newList := make([]Value, len(list)-1)
	copy(newList, list[1:])

	return []Value{d2RetainElem(NewList(newList), args[0]), shifted}, nil
}

// ---- FlexList variants: mutate IN PLACE through the pointer-backed
// store and return the same node (plus the removed element for
// pop/shift). The shapes mirror the plain handlers above so the two
// semantics stay side by side.

func pushFlexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.BoruError("push_error", "push: expected a FlexList, got "+args[1].Parent().String(), "push")
	}
	// A typed flex list ([:T]) enforces + recursively re-tags on a grow.
	tagged, werr := d2AdoptTyped(r, args[1], args[0], "push")
	if werr != nil {
		return nil, werr
	}
	// A flex tree stays ENTIRELY mutable: a plain Node element is deep-
	// flexed on the way in (core.AdoptIntoFlex; flex handles share).
	elem, aerr := core.AdoptIntoFlex(tagged)
	if aerr != nil {
		return nil, r.BoruError("push_error", aerr.Error(), "push")
	}
	fd.Elems = append(fd.Elems, elem)
	return []Value{args[1]}, nil
}

func popFlexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[0])
	if err != nil {
		return nil, r.BoruError("pop_error", "pop: expected a FlexList, got "+args[0].Parent().String(), "pop")
	}
	if len(fd.Elems) == 0 {
		return nil, r.BoruError("pop_error", "pop: cannot pop from empty list", "pop")
	}
	popped := fd.Elems[len(fd.Elems)-1]
	fd.Elems = fd.Elems[:len(fd.Elems)-1]
	return []Value{args[0], popped}, nil
}

func unshiftFlexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.BoruError("unshift_error", "unshift: expected a FlexList, got "+args[1].Parent().String(), "unshift")
	}
	// A typed flex list ([:T]) enforces + recursively re-tags on a grow.
	tagged, werr := d2AdoptTyped(r, args[1], args[0], "unshift")
	if werr != nil {
		return nil, werr
	}
	// Entirely-mutable invariant: adopt a plain Node element into flex.
	elem, aerr := core.AdoptIntoFlex(tagged)
	if aerr != nil {
		return nil, r.BoruError("unshift_error", aerr.Error(), "unshift")
	}
	fd.Elems = append([]Value{elem}, fd.Elems...)
	return []Value{args[1]}, nil
}

func shiftFlexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[0])
	if err != nil {
		return nil, r.BoruError("shift_error", "shift: expected a FlexList, got "+args[0].Parent().String(), "shift")
	}
	if len(fd.Elems) == 0 {
		return nil, r.BoruError("shift_error", "shift: cannot shift from empty list", "shift")
	}
	shifted := fd.Elems[0]
	fd.Elems = append([]Value(nil), fd.Elems[1:]...)
	return []Value{args[0], shifted}, nil
}
