package core

// Value <-> Go-type conversion helpers.
//
// Host programs (CLIs, REPLs, integrations) frequently need to round-
// trip data between boru Values and plain Go values: a CLI passes a Go
// `map[string]any` returned by some library down into a boru handler,
// and renders a boru Value back as Go data for serialisation. These
// helpers centralise that conversion so callers don't reinvent the
// payload-unwrapping logic per project.
//
// ToNative maps a Value down to a plain Go value:
//   String   → string
//   Integer  → int64
//   Float  → float64
//   Boolean  → bool
//   Atom     → string (the atom name)
//   List     → []any  (each element recursively ToNative'd)
//   Map      → map[string]any (likewise)
//   None     → nil
//   anything else → v.String() (best-effort textual fallback)
//
// FromNative lifts a plain Go value up to a boru Value:
//   nil           → None
//   string        → String
//   bool          → Boolean
//   int / int64   → Integer
//   float64       → Integer if integral-valued, else Float
//   []any         → List (each element recursively FromNative'd)
//   map[string]any→ Map (likewise)
//   fmt.Stringer / anything else → String of fmt.Sprintf("%v", x)
//
// FromNative is intentionally lenient — it never errors. Callers passing
// data of an unknown shape get a stringified fallback so the value at
// least surfaces in the boru stream.

import "fmt"

// ToNative converts a boru Value into a plain Go value. See the package
// header comment for the mapping.
func ToNative(v Value) any {
	switch {
	case ParentOf(&v) == nil:
		return nil
	case ParentOf(&v).ConformsTo(TNone):
		return nil
	case ParentOf(&v).ConformsTo(TString):
		s, _ := AsString(v)
		return s
	case ParentOf(&v).ConformsTo(TInteger):
		n, _ := AsInteger(v)
		return n
	case ParentOf(&v).ConformsTo(TFloat):
		f, _ := AsFloat(v)
		return f
	case ParentOf(&v).ConformsTo(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case ParentOf(&v).ConformsTo(TAtom):
		a, _ := AsAtom(v)
		return a
	case ParentOf(&v).ConformsTo(TMap):
		rm, err := AsMap(v)
		if err != nil {
			return v.String()
		}
		out := make(map[string]any, rm.Len())
		for _, k := range rm.Keys() {
			vv, _ := rm.Get(k)
			out[k] = ToNative(vv)
		}
		return out
	case ParentOf(&v).ConformsTo(TList):
		rl, err := AsList(v)
		if err != nil {
			return v.String()
		}
		out := make([]any, rl.Len())
		for i := 0; i < rl.Len(); i++ {
			out[i] = ToNative(rl.Get(i))
		}
		return out
	}
	if bytesToNativeHook != nil {
		if b, ok := bytesToNativeHook(v); ok {
			return b
		}
	}
	return v.String()
}

// bytesFromNativeHook / bytesToNativeHook bridge []byte <-> the Bytes
// type. The Bytes type is owned by the lang layer (which eng cannot
// import), so it installs the conversion via RegisterBytesBridge at type
// registration. Until then the []byte case falls back to a string.
var (
	bytesFromNativeHook func([]byte) Value
	bytesToNativeHook   func(Value) ([]byte, bool)
)

// RegisterBytesBridge installs the []byte <-> Bytes conversions used by
// FromNative / ToNative. Called once by the lang layer when it registers
// the Bytes type; eng stays ignorant of the concrete type.
func RegisterBytesBridge(from func([]byte) Value, to func(Value) ([]byte, bool)) {
	bytesFromNativeHook = from
	bytesToNativeHook = to
}

// FromNative lifts a plain Go value to a boru Value. See the package
// header comment for the mapping. Never returns an error — unknown
// shapes fall back to a stringified Value.
func FromNative(x any) Value {
	switch v := x.(type) {
	case nil:
		return NewNone()
	case Value:
		return v
	case string:
		return NewString(v)
	case bool:
		return NewBoolean(v)
	case int:
		return NewInteger(int64(v))
	case int32:
		return NewInteger(int64(v))
	case int64:
		return NewInteger(v)
	case uint:
		return NewInteger(int64(v))
	case uint32:
		return NewInteger(int64(v))
	case uint64:
		return NewInteger(int64(v))
	case float32:
		return floatToValue(float64(v))
	case float64:
		return floatToValue(v)
	case []byte:
		if bytesFromNativeHook != nil {
			return bytesFromNativeHook(v)
		}
		return NewString(string(v))
	case []any:
		out := make([]Value, len(v))
		for i, e := range v {
			out[i] = FromNative(e)
		}
		return NewList(out)
	case map[string]any:
		m := NewOrderedMap()
		for k, vv := range v {
			m.Set(k, FromNative(vv))
		}
		return NewMap(m)
	}
	return NewString(fmt.Sprintf("%v", x))
}

// floatToValue promotes integer-valued floats to Integer to keep CLI
// output compact (e.g. JSON's `1.0` renders as `1` rather than `1.0`).
// Non-integral floats stay as Float.
func floatToValue(f float64) Value {
	if !isFinite(f) {
		return NewFloat(f)
	}
	if f == float64(int64(f)) {
		return NewInteger(int64(f))
	}
	return NewFloat(f)
}

func isFinite(f float64) bool {
	return f == f && f-f == 0
}
