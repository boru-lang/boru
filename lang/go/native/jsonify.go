package native

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
	voxgigstruct "github.com/voxgig/struct/go"
)

// The "jsonify" word is registered via the consolidated Natives slice in
// natives.go.
//
// Custom-type projection hook: before serialising, the handlers call
// core.NodifyValue, which walks the value's type chain looking for a
// Nodifier behavior installed via `behave nodify/q (fn [[T] [Any]
// [body]])`. The body produces a Node or Scalar (data-shape, not a
// JSON string); the serialiser then encodes that. With no custom
// behavior the value passes through unchanged, preserving the
// pre-existing semantics for plain Maps / Lists / scalars.
//
// jsonifyDefaultHandler calls voxgigstruct.Jsonify with default settings.
func jsonifyDefaultHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	projected, err := core.NodifyValue(args[0])
	if err != nil {
		return nil, err
	}
	data := valueToAnySer(projected)
	result := voxgigstruct.Jsonify(data)
	return []Value{NewString(result)}, nil
}

// valueToAnySer is the serialization projection behind jsonify: like
// valueToAny, but (a) a CLASS instance carries its identity as a
// "$class" metadata key (the user-facing class name — "$" sorts before
// letters, so it leads the output), and (b) user map keys beginning
// with "$" escape to "$$", so a spoofed "$class" inside plain data is
// structurally impossible and MongoDB-style $-keys round-trip through
// reify's unescape. Output is pure JSON (design/legacy/CLASS-OBJECT.10.ignore §3e).
func valueToAnySer(v Value) any {
	if !IsConcrete(v) {
		return nil
	}
	if IsClassInstance(v) {
		oi, err := AsClassInstance(v)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return v.String()
		}
		flat := ClassFields(&oi)
		out := make(map[string]any, flat.Len()+1)
		if oi.TypeRef != nil {
			out["$class"] = classShortName(oi.TypeRef)
		}
		for _, key := range flat.Keys() {
			val, _ := flat.Get(key)
			out[escapeSerKey(key)] = valueToAnySer(val)
		}
		return out
	}
	if IsResourceInstance(v) {
		// Resource/Entity instances serialize as their flat field map.
		// They carry no `$class` key — Resource/Entity are not part of
		// the class reify contract (StructUtil.reify targets classes), so
		// emitting one would produce JSON reify could not round-trip.
		flat, ok := FlatInstanceFields(v)
		if !ok { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return v.String()
		}
		out := make(map[string]any, flat.Len())
		for _, key := range flat.Keys() {
			val, _ := flat.Get(key)
			out[escapeSerKey(key)] = valueToAnySer(val)
		}
		return out
	}
	if v.Parent.ConformsTo(TMap) {
		if m, _ := AsMap(v); m != nil {
			out := make(map[string]any, m.Len())
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				out[escapeSerKey(key)] = valueToAnySer(val)
			}
			return out
		}
	}
	if v.Parent.ConformsTo(TList) {
		if _lst, _ := AsList(v); !_lst.IsNil() {
			elems := _lst.Slice()
			out := make([]any, len(elems))
			for i, elem := range elems {
				out[i] = valueToAnySer(elem)
			}
			return out
		}
	}
	return valueToAny(v)
}

// escapeSerKey escapes a user data key for serialization: a leading
// "$" gains a second one ("$class" → "$$class"), reserving the single
// "$" prefix for metadata. reify (and a future parse) unescape.
func escapeSerKey(k string) string {
	if len(k) > 0 && k[0] == '$' {
		return "$" + k
	}
	return k
}

// jsonifyFlagsHandler calls voxgigstruct.Jsonify with a flags map.
func jsonifyFlagsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	flags, ok := valueToAny(args[0]).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("jsonify: expected map for flags, got %T", valueToAny(args[0]))
	}
	projected, err := core.NodifyValue(args[1])
	if err != nil {
		return nil, err
	}
	data := valueToAnySer(projected)
	result := voxgigstruct.Jsonify(data, flags)
	return []Value{NewString(result)}, nil
}
