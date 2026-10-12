package native

import (
	"fmt"
)

// valueToSliceArg converts a Value into the interface{} expected by
// voxgigstruct.Slice: strings flow through unchanged, lists become
// []interface{} whose elements are the original Values boxed verbatim,
// anything else falls back to its String() form. Used by
// stringSliceNative in natives.go.
//
// List elements are boxed (not recursively converted to Go natives)
// because Slice only reorders/sub-ranges top-level elements and never
// introspects them; boxing preserves every element type (Integer,
// Float, Map, nested List, …) across the round trip — previously the
// non-string/non-list fallback stringified scalar elements, so a sliced
// integer list came back as strings. sliceResultItem unboxes via its
// Value case.
func valueToSliceArg(v Value) interface{} {
	if v.Parent.ConformsTo(TString) {
		_as3, _ := AsString(v)
		return _as3
	}
	if v.Parent.ConformsTo(TList) {
		list, _ := AsList(v)
		result := make([]interface{}, list.Len())
		for i, elem := range list.Elems() {
			result[i] = elem
		}
		return result
	}
	return v.String()
}

// sliceResult converts a voxgigstruct.Slice result back into engine
// values. Strings stay strings; []interface{} becomes a List of converted
// items via sliceResultItem; nil becomes a None type literal.
func sliceResult(result interface{}) ([]Value, error) {
	switch r := result.(type) {
	case string:
		return []Value{NewString(r)}, nil
	case []interface{}:
		vals := make([]Value, len(r))
		for i, item := range r {
			v, err := sliceResultItem(item)
			if err != nil {
				return nil, err
			}
			vals[i] = v
		}
		return []Value{NewList(vals)}, nil
	case nil:
		return []Value{NewTypeLiteral(TNone)}, nil
	default:
		return nil, fmt.Errorf("slice: unexpected result type %T", result)
	}
}

// sliceRetain wraps a sliceResult return: when the result is a list (a
// list-slice, not a string-slice or empty None), it copies src's [:T] element
// tag onto the result so `slice` of a typed list stays typed — a slice is a
// pure subset/reorder that preserves element types, so downstream reads narrow
// and downstream writes stay enforced (#4, round 3). A string source has no
// element tag, so d2RetainElem no-ops there.
func sliceRetain(res []Value, err error, src Value) ([]Value, error) {
	if err == nil && len(res) == 1 && res[0].Parent.ConformsTo(TList) {
		res[0] = d2RetainElem(res[0], src)
	}
	return res, err
}

// sliceListReturns is the check-mode residual for a LIST slice overload (data
// arg at index dataIdx): slice is a subset that preserves element types, so the
// residual is a both-fields typed-list carrier retaining the source [:T] tag —
// so `((slice xs) set 0 "bad")` for xs:[:Integer] is diagnosed at check, like
// reverse/take/shed (#10, Codex round 10). Only wired on the TList overloads
// (the TString overloads return a string with no element tag).
func sliceListReturns(dataIdx int) func([]Value, *Registry) []Value {
	return func(args []Value, _ *Registry) []Value {
		res := NewCarrier(TList)
		if dataIdx < len(args) {
			res = d2TypedListResidual(args[dataIdx])
		}
		return []Value{res}
	}
}

// sliceResultItem converts a single item produced by voxgigstruct.Slice
// into an Value, recursing into nested []interface{} slices.
func sliceResultItem(item interface{}) (Value, error) {
	switch v := item.(type) {
	case Value:
		// List elements are boxed verbatim by valueToSliceArg; unbox
		// them unchanged to preserve their original type.
		return v, nil
	case string:
		return NewString(v), nil
	case int:
		return NewInteger(int64(v)), nil
	case int64:
		return NewInteger(v), nil
	case float64:
		return NewFloat(v), nil
	case bool:
		return NewBoolean(v), nil
	case nil:
		return NewTypeLiteral(TNone), nil
	case []interface{}:
		vals := make([]Value, len(v))
		for i, elem := range v {
			val, err := sliceResultItem(elem)
			if err != nil {
				return Value{}, err
			}
			vals[i] = val
		}
		return NewList(vals), nil
	default:
		return Value{}, fmt.Errorf("slice: unsupported element type %T", item)
	}
}
