package native

import (
	"fmt"
	"strconv"

	voxgigstruct "github.com/voxgig/struct/go"
)

// The "merge" word is registered via the consolidated Natives slice in
// natives.go.
//
// mergeHandler calls voxgigstruct.Merge on two values, returning the merged result.
func mergeHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	a := valueToAny(args[0])
	b := valueToAny(args[1])

	result := voxgigstruct.Merge([]any{a, b})

	val, err := structConvert(result)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}
	// #2: a merge INTO a typed container ({:T}) enforces its element type on
	// every merged value and retains the tag — the deep merge otherwise drops
	// `elem` (valueToAny/structConvert rebuild a plain node) and would accept a
	// non-conforming value, bypassing the invariant `set` enforces.
	tagged, terr := d2ReTagContainer(r, d2typedMergeOperand(args[0], args[1]), val, "merge")
	if terr != nil {
		return nil, terr
	}
	return []Value{tagged}, nil
}

// mergeListMapHandler creates a new list with map's integer keys replacing
// elements at those positions. Non-integer keys and out-of-range indices
// are ignored. The original list is unchanged.
//
//	[a,b,c] merge {1:d} → [a,d,c]
func mergeListMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Elems()
	m, _ := AsMap(args[1])
	if list == nil || m == nil {
		return nil, r.BoruError("merge_error", "merge: expected concrete list and map", "merge")
	}

	// Copy the list.
	result := make([]Value, len(list))
	copy(result, list)

	// Apply map's integer-keyed values.
	for _, key := range m.Keys() {
		idx, err := strconv.Atoi(key)
		if err != nil {
			continue // non-integer key, ignore
		}
		if idx < 0 {
			continue // negative index, ignore
		}
		val, _ := m.Get(key)
		if idx < len(result) {
			result[idx] = val
		} else if idx == len(result) {
			result = append(result, val)
		}
		// idx > len(result): gap, ignore
	}

	// The result is a LIST built from args[0] (the list operand), so the LIST's
	// [:T] governs — every merged value must conform to it. Re-tag against args[0]
	// (NOT d2typedMergeOperand, which would pick the map patch's tag and validate
	// the list result against the wrong contract — Codex round 7).
	out, terr := d2ReTagContainer(r, args[0], NewList(result), "merge")
	if terr != nil {
		return nil, terr
	}
	return []Value{out}, nil
}

// mergeMapListHandler creates a new list from the list argument, with
// map's in-range integer-keyed values appended at their positions.
// Non-integer keys are ignored. Keys beyond the list length extend it.
// Keys within range replace existing elements.
//
//	{3:d,x:X} merge [a,b,c] → [a,b,c,d]
func mergeMapListHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	m, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	list := _lst.Elems()
	if m == nil || list == nil {
		return nil, r.BoruError("merge_error", "merge: expected concrete map and list", "merge")
	}

	// Start with a copy of the list.
	result := make([]Value, len(list))
	copy(result, list)

	// Apply map's integer-keyed values.
	for _, key := range m.Keys() {
		idx, err := strconv.Atoi(key)
		if err != nil {
			continue // non-integer key, ignore
		}
		if idx < 0 {
			continue // negative index, ignore
		}
		val, _ := m.Get(key)
		if idx < len(result) {
			result[idx] = val
		} else if idx == len(result) {
			// Extend by one — append at the end.
			result = append(result, val)
		}
		// idx > len(result): gap, ignore
	}

	// The result is a LIST built from args[1] (the list operand), so the LIST's
	// [:T] governs — the map patch's integer-keyed values are written INTO it and
	// must conform to it. Re-tag against args[1] (NOT d2typedMergeOperand, which
	// picks args[0], the map patch, and validates against the wrong contract —
	// `{:String}{"0":"bad"} merge [:Integer][1]` must reject; Codex round 7).
	out, terr := d2ReTagContainer(r, args[1], NewList(result), "merge")
	if terr != nil {
		return nil, terr
	}
	return []Value{out}, nil
}
