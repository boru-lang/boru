package native

import (
	"fmt"
	"math"
	"sort"

	voxgigstruct "github.com/voxgig/struct/go"
)

// The "transform" word is registered via the consolidated Natives slice
// in natives.go. This file keeps the transform handler plus the shared
// Value↔any conversion helpers (valueToAny / anyToValue / valueToMap)
// used across the package.
//
// transformHandler calls voxgig struct Transform.
// args[0]=spec (Map, forward-collected), args[1]=data (Any, from stack).
func transformHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	spec := valueToAny(args[0])
	data := valueToAny(args[1])

	result := voxgigstruct.Transform(data, spec)

	// Route convert-back through the shared structConvert seam
	// (design/TEST-SEAMS.10.md) so the failure arm is drivable — the
	// voxgig-struct round-trip guarantee is owned by the library, not here.
	val, err := structConvert(result)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	return []Value{val}, nil
}

// ValueToAny is the exported entry point to the value→Go-any
// conversion. The unexported valueToAny is the in-package alias
// kept for existing handler call sites; new external callers (e.g.
// lang/go/modules) should use this name.
func ValueToAny(v Value) any { return valueToAny(v) }

// valueToAny converts an Value to a Go any for use with voxgig struct.
func valueToAny(v Value) any {
	// Type literals (e.g. bare Map, List, Integer) have Data==nil.
	// Return nil rather than panicking on accessor calls.
	if !IsConcrete(&v) {
		return nil
	}
	switch {
	case v.Parent.ConformsTo(TInteger):
		i, _ := AsInteger(v)
		return float64(i)
	case v.Parent.ConformsTo(TFloat):
		// A Float is a JSON number, not a string (a missing case here
		// made jsonify emit "x": "1.5"). DepScalar payloads decline the
		// concrete accessor and keep the rendering fallback.
		if f, err := v.AsConcreteFloat(); err == nil {
			return f
		}
		return v.String()
	case v.Parent.ConformsTo(TString):
		s, _ := AsString(v)
		return s
	case v.Parent.ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case v.Parent.Equal(TAtom):
		a, _ := AsAtom(v)
		return a
	case v.Parent.Equal(TNone):
		return nil
	case v.Parent.ConformsTo(TMap):
		m, _ := AsMap(v)
		if m == nil {
			// A TMap-conforming value whose payload is not a concrete
			// OrderedMap (a record/options/typed-map descriptor) — AsMap
			// returns nil rather than an OrderedMap. Fall back to the
			// rendered form instead of dereferencing nil (ADR-005).
			return v.String()
		}
		out := make(map[string]any, m.Len())
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			out[key] = valueToAny(val)
		}
		return out
	case v.Parent.ConformsTo(TList):
		_lst, _ := AsList(v)
		elems := _lst.Slice()
		out := make([]any, len(elems))
		for i, elem := range elems {
			out[i] = valueToAny(elem)
		}
		return out
	case IsFlatInstance(v):
		// Class and Resource/Entity instances project to their flattened
		// field map, so items / walk / transform / jsonify see real data
		// rather than the debug rendering. (The $class metadata key is
		// added by the serialization layer, not here — enumeration
		// reports fields only.)
		//
		// FlatInstanceFields always returns ok==true here: IsFlatInstance
		// matches exactly ClassInstanceInfo / ResourceInstanceInfo, and
		// flatInstanceParts returns ok==true (non-nil field map) for both,
		// so a "!ok" guard would be dead (design/TEST-SEAMS.10.md).
		flat, _ := FlatInstanceFields(v)
		out := make(map[string]any, flat.Len())
		for _, key := range flat.Keys() {
			val, _ := flat.Get(key)
			out[key] = valueToAny(val)
		}
		return out
	default:
		return v.String()
	}
}

// anyToValue converts a Go any (as returned by voxgig struct) back to an Value.
func anyToValue(v any) (Value, error) {
	switch val := v.(type) {
	case nil:
		return NewTypeLiteral(TNone), nil
	case bool:
		return NewBoolean(val), nil
	case float64:
		// A whole-valued JSON number stays an Integer (the historical
		// behaviour every reify of {"n": 2} relies on); a fractional one
		// is a Float — int64(1.5) silently truncated it to 1 before.
		if val == math.Trunc(val) &&
			val >= float64(math.MinInt64) && val < float64(math.MaxInt64) {
			return NewInteger(int64(val)), nil
		}
		return NewFloat(val), nil
	case int:
		return NewInteger(int64(val)), nil
	case int64:
		return NewInteger(val), nil
	case string:
		return NewString(val), nil
	case []any:
		elems := make([]Value, len(val))
		for i, item := range val {
			e, err := anyToValue(item)
			if err != nil {
				return Value{}, err
			}
			elems[i] = e
		}
		return NewList(elems), nil
	case map[string]any:
		om := NewOrderedMap()
		for _, key := range sortedAnyMapKeys(val) {
			child, err := anyToValue(val[key])
			if err != nil {
				return Value{}, err
			}
			om.Set(key, child)
		}
		return NewMap(om), nil
	default:
		return Value{}, fmt.Errorf("unsupported type from transform: %T", v)
	}
}

// valueToMap converts a map-typed Value to map[string]any for use with SDK calls.
func valueToMap(v Value) map[string]any {
	m, _ := AsMap(v)
	if m == nil {
		return nil
	}
	out := make(map[string]any, m.Len())
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		out[key] = valueToAny(val)
	}
	return out
}

// sortedAnyMapKeys returns map keys in sorted order for deterministic
// output. The single implementation shared across the package (fileio's
// sortedMapKeys delegates here).
func sortedAnyMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
