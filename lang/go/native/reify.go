package native

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// The "reify" word — StructUtil.reify — hydrates a class instance from
// JSON text or an already-parsed Node (Map). It is the inverse of the
// instance-aware jsonify (see jsonify.go):
//
//	StructUtil.reify Point {x:1}            from a Node
//	StructUtil.reify Point "{...json...}"   from JSON text
//	StructUtil.reify Shape data             union-bounded: Shape is a
//	                                        tor union of classes and the
//	                                        input's $class selects WHICH
//	                                        member to construct — and
//	                                        nothing outside the union
//
// Rules (design/legacy/CLASS-OBJECT.10.ignore §3e):
//   - the target is explicit: a class type, or a tor union of class
//     types ($class then selects the member). There is no unbounded
//     "instantiate whatever $class names" form.
//   - a "$class" key in the input is cross-checked against the target
//     (mismatch errors); "$$"-prefixed keys unescape to "$"-keys.
//   - hydration IS construction: the data routes through the same flat
//     sealed make path — unknown keys error, missing required fields
//     error, defaults fill, predicate field types run.
//   - JSON's type poverty is recovered from the schema: a JSON number
//     lands as Integer or Float by the field's declared type, a string
//     becomes an Atom where the field says Atom, and a nested map
//     hydrates recursively where the field's type is itself a class.
func reifyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	target := args[0]
	input := args[1]

	// JSON-text form: decode to a generic tree first.
	if input.Parent.ConformsTo(TString) && IsConcrete(input) {
		text, _ := AsString(input)
		var raw any
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: invalid JSON: %s", err.Error()), "reify")
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: expected a JSON object, got %T", raw), "reify")
		}
		return reifyFromAnyMap(target, m, r)
	}

	// Node form: a concrete Map of already-parsed Values.
	m, err := RequireConcreteMap(input, "reify")
	if err != nil {
		return nil, r.BoruError("reify_error", err.Error(), "reify")
	}
	anyMap := make(map[string]any, m.Len())
	keys := make([]string, 0, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		anyMap[k] = v
		keys = append(keys, k)
	}
	return reifyFromAnyMapOrdered(target, anyMap, keys, r)
}

// reifyFromAnyMap hydrates from a decoded JSON object (unordered Go
// map — JSON objects carry no order, so schema order governs).
func reifyFromAnyMap(target Value, m map[string]any, r *Registry) ([]Value, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return reifyFromAnyMapOrdered(target, m, keys, r)
}

func reifyFromAnyMapOrdered(target Value, m map[string]any, keys []string, r *Registry) ([]Value, error) {
	// Extract metadata + unescape user $-keys.
	className := ""
	fields := make(map[string]any, len(m))
	order := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		switch {
		case k == "$class":
			if s, ok := v.(string); ok {
				className = s
			} else if val, isVal := v.(Value); isVal && val.Parent.ConformsTo(TString) {
				className, _ = AsString(val)
			} else {
				return nil, r.BoruError("reify_error", "reify: $class must be a string", "reify")
			}
		case strings.HasPrefix(k, "$$"):
			key := k[1:] // "$$x" unescapes to "$x"
			fields[key] = v
			order = append(order, key)
		case strings.HasPrefix(k, "$"):
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: unknown metadata key %q (user $-keys serialize escaped as $$)", k), "reify")
		default:
			fields[k] = v
			order = append(order, k)
		}
	}

	info, err := resolveReifyTarget(target, className, r)
	if err != nil {
		return nil, err
	}

	// Build the provided map, recovering JSON-poor types from the
	// schema field by field.
	allFields := info.AllFields()
	provided := NewOrderedMap()
	for _, k := range order {
		constraint, declared := allFields.Get(k)
		if !declared {
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: unknown field %q for class %s", k, info.Name), "reify")
		}
		val, err := reifyFieldValue(fields[k], constraint, r)
		if err != nil {
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: field %q: %s", k, err.Error()), "reify")
		}
		provided.Set(k, val)
	}

	// Hydration IS construction: route through make (defaults fill,
	// required fields enforced, predicates run, sealing applies).
	results, err := core.MakeObject(*info, NewMap(provided), r)
	if err != nil {
		return nil, r.BoruError("reify_error", err.Error(), "reify")
	}
	return results, nil
}

// resolveReifyTarget returns the class ClassTypeInfo to construct:
// a direct class target (with $class cross-checked when present), or
// a member selected by $class from a tor union of classes.
func resolveReifyTarget(target Value, className string, r *Registry) (*core.ClassTypeInfo, error) {
	// A NAMED target evaluates to its minted node (the Stage 2 flip);
	// its declared schema is the node's recorded content.
	if body, ok := core.TypeContentOf(target); ok {
		target = body
	}
	if IsClassType(target) {
		info, _ := AsClassType(target)
		if className != "" && className != classShortName(&info) {
			return nil, r.BoruError("reify_error",
				fmt.Sprintf("reify: $class %q does not match target class %s", className, classShortName(&info)), "reify")
		}
		return &info, nil
	}
	if di, ok := target.Data.(DisjunctInfo); ok {
		if className == "" {
			return nil, r.BoruError("reify_error",
				"reify: a union target needs a $class key to select the member", "reify")
		}
		var names []string
		for _, alt := range di.Alternatives {
			// A named member arrives as its NODE; its schema is the
			// node's recorded content.
			if body, ok := TypeContentOf(alt); ok && IsBareTypeNode(alt) {
				alt = body
			}
			if !IsClassType(alt) {
				continue
			}
			info, _ := AsClassType(alt)
			names = append(names, classShortName(&info))
			if classShortName(&info) == className {
				return &info, nil
			}
		}
		return nil, r.BoruError("reify_error",
			fmt.Sprintf("reify: $class %q is not a member of the union (members: %s)", className, strings.Join(names, " ")), "reify")
	}
	return nil, r.BoruError("reify_error",
		"reify: target must be a class (or a tor union of classes)", "reify")
}

// classShortName is the user-facing class name — the last segment of
// the installed path ("Class/Point" → "Point").
func classShortName(info *core.ClassTypeInfo) string {
	name := info.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// reifyFieldValue converts one raw input value (a decoded-JSON any or
// an already-parsed Value) toward a schema field's declared type. The
// schema drives recovery of JSON's poor scalars; the result is then
// re-validated strictly by the make path, so recovery never bypasses
// the field contract.
func reifyFieldValue(raw any, constraint Value, r *Registry) (Value, error) {
	// A NAMED field constraint holds the class NODE (the Stage 2 flip);
	// hydration needs the schema recorded on it.
	if IsBareTypeNode(constraint) {
		if body, ok := constraint.TypeBody(); ok && IsClassType(body) {
			constraint = body
		}
	}
	// Nested class field + map input → recursive hydration.
	if IsClassType(constraint) {
		switch m := raw.(type) {
		case map[string]any:
			res, err := reifyFromAnyMap(constraint, m, r)
			if err != nil {
				return Value{}, err
			}
			return res[0], nil
		case Value:
			if mm, err := RequireConcreteMap(m, "reify"); err == nil {
				anyMap := make(map[string]any, mm.Len())
				keys := make([]string, 0, mm.Len())
				for _, k := range mm.Keys() {
					v, _ := mm.Get(k)
					anyMap[k] = v
					keys = append(keys, k)
				}
				res, err := reifyFromAnyMapOrdered(constraint, anyMap, keys, r)
				if err != nil {
					return Value{}, err
				}
				return res[0], nil
			}
		}
	}

	val, err := rawToValue(raw)
	if err != nil {
		return Value{}, err
	}

	// Already conforming (per the strict class rule) — done.
	if _, ferr := core.MakeClassFieldValue(val, constraint, r); ferr == nil {
		return val, nil
	}

	// Schema-driven recovery: convert toward the declared type (JSON
	// has no Integer/Float/Atom distinction). The converted value is
	// re-validated by makeClassInstance, so this cannot loosen the
	// contract — it only recovers representation.
	ct := targetTypeOf(constraint)
	if ct != nil {
		if converted, cerr := core.MakeConvert(val, ct); cerr == nil {
			return converted, nil
		}
	}
	return val, nil
}

// targetTypeOf extracts the lattice type a schema constraint aims at:
// the node itself for a bare type constraint, the default's own type
// for a concrete default. Predicate/disjunction constraints return nil
// (no single conversion target — the value must already conform).
func targetTypeOf(constraint Value) *core.Type {
	if IsBareTypeNode(constraint) {
		return core.ValueType(constraint)
	}
	// A DepScalar predicate constraint ((Float gte 0.0)) recovers
	// toward its BASE type — the predicate itself re-runs strictly in
	// the make path after conversion, so a JSON integer 5 can become
	// the Float 5.0 the predicate wants without loosening the bound.
	if constraint.IsDepScalar() {
		return constraint.Parent
	}
	if pt := core.PredicateInputType(constraint); pt != nil {
		return pt
	}
	if IsConcrete(constraint) && !core.IsTypeBody(constraint) {
		return constraint.Parent
	}
	return nil
}

// rawToValue converts a decoded-JSON leaf (or passes through an
// already-parsed Value). A JSON number with a fractional part is a
// Float; an integral one is an Integer — the schema-driven conversion
// above settles the cases where the field wants the other leaf.
func rawToValue(raw any) (Value, error) {
	switch v := raw.(type) {
	case Value:
		return v, nil
	case nil:
		// JSON null hydrates as the CONCRETE none value, not the bare
		// None type literal — jsonify emits null for `none`, so reify
		// must return `none` to keep the jsonify→reify inverse exact
		// (field reads and deq round-trips must match the original).
		return NewNone(), nil
	case bool:
		return NewBoolean(v), nil
	case string:
		return NewString(v), nil
	case float64:
		if v == float64(int64(v)) {
			return NewInteger(int64(v)), nil
		}
		return NewFloat(v), nil
	case map[string]any:
		om := NewOrderedMap()
		for _, k := range sortedAnyMapKeys(v) {
			cv, err := rawToValue(v[k])
			if err != nil {
				return Value{}, err
			}
			key := k
			if strings.HasPrefix(k, "$$") {
				key = k[1:]
			}
			om.Set(key, cv)
		}
		return NewMap(om), nil
	case []any:
		elems := make([]Value, len(v))
		for i, item := range v {
			ev, err := rawToValue(item)
			if err != nil {
				return Value{}, err
			}
			elems[i] = ev
		}
		return NewList(elems), nil
	default:
		return Value{}, fmt.Errorf("reify: unsupported JSON value %T", raw)
	}
}
