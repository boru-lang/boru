package native

import (
	"fmt"
	"strconv"
	"strings"
)

// The "setpath" word is registered via the consolidated Natives slice in
// natives.go.
//
// setpathHandler sets a nested value, returning a NEW structure. The path is
// either a dotted String or a Reach lens; both walk the same native setter
// (setReachNative — an immutable deep set). Position-agnostic: it finds the
// path arg, then determines which of the remaining args is data vs new value.
func setpathHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	// Find the path arg — a String dotted path, or a Reach lens.
	pathIdx := -1
	for i := range args {
		if args[i].Parent().ConformsTo(TString) || IsReach(&args[i]) {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 {
		return nil, r.BoruError("setpath_error", "setpath: path argument must be a string or a reach", "setpath")
	}

	// Collect the two non-path args.
	var others [2]int
	oi := 0
	for i := range args {
		if i != pathIdx && oi < 2 {
			others[oi] = i
			oi++
		}
	}

	// Convention: with forward-first matching for `data setpath "path" newVal`:
	// Forward args fill first positions → args[0]="path", args[1]=newVal, args[2]=data
	// For `setpath data "path" newVal` (all forward): args[0]=data, args[1]="path", args[2]=newVal
	// The data arg is typically a map/list; newVal can be anything.
	// Use positional heuristic: first non-path arg = data, second = newVal.
	// But when data comes from stack (last position), swap.
	a, b := args[others[0]], args[others[1]]
	var data, newVal Value
	if others[0] < others[1] {
		if (a.Parent().ConformsTo(TMap) || a.Parent().ConformsTo(TList)) &&
			!(b.Parent().ConformsTo(TMap) || b.Parent().ConformsTo(TList)) {
			data, newVal = a, b
		} else if (b.Parent().ConformsTo(TMap) || b.Parent().ConformsTo(TList)) &&
			!(a.Parent().ConformsTo(TMap) || a.Parent().ConformsTo(TList)) {
			data, newVal = b, a
		} else {
			data, newVal = a, b
		}
	} else {
		data, newVal = b, a
	}

	// Reach lens: set natively so nesting is honored (an immutable nested
	// set, the lens "set" — the dotted-string voxgigstruct form does not
	// deep-set). The reach's receiver is ignored; data is the base.
	if IsReach(&args[pathIdx]) {
		info, err := AsReach(&args[pathIdx])
		if err != nil {
			return nil, fmt.Errorf("setpath: %w", err)
		}
		keys, err := reachPathKeys(r, info)
		if err != nil {
			return nil, err
		}
		out, err := setReachNative(data, keys, newVal, r)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	}

	path, err := AsString(args[pathIdx])
	if err != nil {
		return nil, fmt.Errorf("setpath: path: %w", err)
	}
	// Walk the dotted path natively (same setter as the reach form). NOT
	// voxgigstruct.SetPath: that function returns the innermost sub-node
	// rather than the updated root, so any nested set lost the outer
	// structure (`setpath {a:{b:1}} "a.b" 99` → {b:99} instead of
	// {a:{b:99}}). The native setter deep-sets correctly + immutably.
	out, err := setReachNative(data, stringPathKeys(path), newVal, r)
	if err != nil {
		return nil, err
	}
	return []Value{out}, nil
}

// stringPathKeys splits a dotted path ("a.b.0") into the key Values to walk:
// an all-digit segment becomes an Integer (a list index / numeric key), any
// other segment a String. Mirrors voxgigstruct's `.`-split, feeding the same
// native setter the reach form uses.
func stringPathKeys(path string) []Value {
	rawParts := strings.Split(path, ".")
	keys := make([]Value, len(rawParts))
	for i, p := range rawParts {
		if n, err := strconv.Atoi(p); err == nil && isAllDigits(p) {
			keys[i] = NewInteger(int64(n))
		} else {
			keys[i] = NewString(p)
		}
	}
	return keys
}

// isAllDigits reports whether s is non-empty and every rune is 0-9 (so "01"
// and "0" are indices, but "-1" / "1a" / "" stay string keys).
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// reachPathKeys decodes a Reach's segments into the key Values to walk
// (literal keys as-is; computed keys m.(expr) evaluated first). The get/getr
// distinction is irrelevant on a write path — setpath creates the path.
func reachPathKeys(r *Registry, info ReachInfo) ([]Value, error) {
	keys := make([]Value, 0, len(info.Segments))
	for i, seg := range info.Segments {
		if seg.Computed {
			sub := New(r)
			out, runErr := sub.Run(seg.KeyExpr)
			if runErr != nil {
				return nil, fmt.Errorf("setpath: computed key %d: %w", i, runErr)
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("setpath: computed key %d produced no value", i)
			}
			keys = append(keys, out[len(out)-1])
		} else {
			keys = append(keys, seg.KeyLit)
		}
	}
	return keys, nil
}

// setReachNative immutably sets val at the key-path within data, returning a
// new structure. A numeric key into a concrete list updates that index (in a
// copy); any other key updates a map (shallow-cloned); a class / Object
// instance level round-trips through construction (a new instance of the
// same type, schema checks enforced for classes). Missing intermediate maps
// are created. Sibling values are shared by reference — safe because nothing
// is mutated in place.
func setReachNative(data Value, keys []Value, val Value, r *Registry) (Value, error) {
	if len(keys) == 0 {
		return val, nil
	}
	k := keys[0]
	rest := keys[1:]

	// Class / Object instance: edit the projected field map, then
	// rebuild type-preservingly. A class instance re-makes through
	// MakeClassInstance, so the same strict validation `make` runs
	// applies to the edit — an unknown top-level field or an
	// off-schema value errors loudly rather than degrading to a Map.
	if IsClassInstance(data) {
		info, _ := AsClassInstance(data)
		fields := ClassFields(&info)
		out := NewOrderedMap()
		for _, key := range fields.Keys() {
			v, _ := fields.Get(key)
			out.Set(key, v)
		}
		keyStr := reachKeyString(k)
		if len(rest) == 0 {
			out.Set(keyStr, val)
		} else {
			existing, _ := out.Get(keyStr)
			child, err := setReachNative(existing, rest, val, r)
			if err != nil {
				return Value{}, err
			}
			out.Set(keyStr, child)
		}
		if info.TypeRef != nil {
			inst, err := MakeClassInstance(*info.TypeRef, out, r)
			if err != nil {
				return Value{}, fmt.Errorf("setpath: %w", err)
			}
			return inst, nil
		}
		return NewClassInstance(data.Parent(), ClassInstanceInfo{
			TypeRef: info.TypeRef,
			Fields:  out,
		}), nil
	}

	// Resource/Entity instance: same type-preserving rebuild as a class
	// instance, routed through MakeResource so the SDK schema validation
	// (missing/unknown field, field types) runs on the edit.
	if IsResourceInstance(data) {
		info, _ := AsResourceInstance(data)
		out := NewOrderedMap()
		if info.Fields != nil {
			for _, key := range info.Fields.Keys() {
				v, _ := info.Fields.Get(key)
				out.Set(key, v)
			}
		}
		keyStr := reachKeyString(k)
		if len(rest) == 0 {
			out.Set(keyStr, val)
		} else {
			existing, _ := out.Get(keyStr)
			child, err := setReachNative(existing, rest, val, r)
			if err != nil {
				return Value{}, err
			}
			out.Set(keyStr, child)
		}
		if info.TypeRef != nil {
			results, err := MakeResource(*info.TypeRef, out, r)
			if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return Value{}, fmt.Errorf("setpath: %w", err)
			}
			return results[0], nil
		}
		return NewResourceInstance(data.Parent(), ResourceInstanceInfo{
			TypeRef: info.TypeRef,
			Fields:  out,
		}), nil
	}

	// List index.
	if k.Parent().ConformsTo(TInteger) && data.Parent().ConformsTo(TList) && IsConcrete(&data) {
		lst, _ := AsList(data)
		n := lst.Len()
		idx64, _ := AsInteger(k)
		idx := int(idx64)
		if idx < 0 || idx >= n {
			return Value{}, fmt.Errorf("setpath: list index %d out of range (len %d)", idx, n)
		}
		cp := make([]Value, n)
		for i := 0; i < n; i++ {
			cp[i] = lst.Get(i)
		}
		child, err := setReachNative(cp[idx], rest, val, r)
		if err != nil {
			return Value{}, err
		}
		// Enforce + re-tag the value written at THIS level against data's
		// element type — a deep path that builds an intermediate must still
		// conform to the OUTER [:T] (`setpath xs 0 "bad"` / a nested map into a
		// scalar-element list are both rejected here, not just the leaf).
		tchild, terr := d2AdoptTyped(r, data, child, "setpath")
		if terr != nil {
			return Value{}, terr
		}
		cp[idx] = tchild
		res := NewList(cp)
		if elem, ok := data.ElemConstraint(); ok { // R3: keep [:T] on the copy
			res.SetElemConstraint(elem)
		}
		return res, nil
	}

	// Map key (the default). Shallow-clone the source map (or start fresh
	// when the level is absent / not a map).
	out := NewOrderedMap()
	if data.Parent().ConformsTo(TMap) && IsConcrete(&data) {
		src, _ := AsMap(data)
		for _, key := range src.Keys() {
			v, _ := src.Get(key)
			out.Set(key, v)
		}
	}
	keyStr := reachKeyString(k)
	toSet := val
	if len(rest) != 0 {
		existing, _ := out.Get(keyStr)
		child, err := setReachNative(existing, rest, val, r)
		if err != nil {
			return Value{}, err
		}
		toSet = child
	}
	// Enforce + re-tag the value written at THIS level against data's element
	// type (leaf val OR the recursively-built intermediate) — covers deep paths
	// that would otherwise slip an off-type child into a tagged outer container.
	tset, terr := d2AdoptTyped(r, data, toSet, "setpath")
	if terr != nil {
		return Value{}, terr
	}
	out.Set(keyStr, tset)
	res := NewMap(out)
	if elem, ok := data.ElemConstraint(); ok { // R3: keep {:T} on the copy
		res.SetElemConstraint(elem)
	}
	return res, nil
}

// reachKeyString renders a reach key Value as its map-key string (a word /
// atom by name, a string by value, anything else via ValToString).
func reachKeyString(k Value) string {
	switch {
	case IsWord(&k):
		w, _ := AsWord(&k)
		return w.Name
	case IsAtom(k):
		a, _ := AsAtom(k)
		return a
	case k.Parent().ConformsTo(TString):
		s, _ := AsString(k)
		return s
	default:
		return ValToString(k)
	}
}
