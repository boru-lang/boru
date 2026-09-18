package core

// unifyOptionsFamily owns unification when at least one side is an
// Options type. Options fields can carry concrete defaults, type-
// literal constraints, or disjuncts — three sub-rules that compose
// per-field. r is the enclosing chain's registry, handed to every
// field unify.
func unifyOptionsFamily(a Value, sa ValueShape, b Value, sb ValueShape, r *Registry) (Value, *UnifyError) {
	// Two Options → unify field schemas (order-independent).
	if sa == ShapeOptions && sb == ShapeOptions {
		aOT, _ := AsOptionsType(a)
		bOT, _ := AsOptionsType(b)
		return unifyOptionsPair(aOT, bOT, r)
	}

	// Canonicalize: opts on the left, concrete on the right.
	var opts OptionsTypeInfo
	var concrete Value
	if sa == ShapeOptions {
		opts, _ = AsOptionsType(a)
		concrete = b
	} else {
		opts, _ = AsOptionsType(b)
		concrete = a
	}

	// Bare Map type literal vs Options → preserve the Options schema.
	if !IsConcrete(concrete) {
		return NewOptionsType(opts.Fields), nil
	}

	// Options only accepts plain concrete maps, never structural map
	// subtypes (Record / TypedMap / nested Options).
	if IsRecordType(concrete) || IsTypedMap(concrete) || IsOptionsType(concrete) {
		return Value{}, unifyFail("Options only unifies with a plain Map", a, b)
	}

	cMap, _ := AsMap(concrete)

	// Extra keys in concrete not in Options → fail.
	for _, key := range cMap.Keys() {
		if _, ok := opts.Fields.Get(key); !ok {
			return Value{}, unifyFail("unknown key "+key+" not in Options schema", a, b)
		}
	}

	result := NewOrderedMap()
	for _, key := range opts.Fields.Keys() {
		optVal, _ := opts.Fields.Get(key)
		cVal, present := cMap.Get(key)
		if !present {
			defVal, ok := optionsDefault(optVal)
			if !ok {
				return Value{}, &UnifyError{
					Reason: "missing required key " + key + " with no default",
					Path:   []string{"field:" + key},
				}
			}
			result.Set(key, defVal)
			continue
		}
		unified, err := unifyOptionsField(optVal, cVal, r)
		if err != nil {
			return Value{}, err.withPath("field:" + key)
		}
		result.Set(key, unified)
	}
	return NewMap(result), nil
}

// unifyOptionsPair unifies two options types by unifying their field
// schemas. Key order is not significant.
func unifyOptionsPair(a, b OptionsTypeInfo, r *Registry) (Value, *UnifyError) {
	result, err := unifyFieldBags(a.Fields, b.Fields, false, r)
	if err != nil {
		return Value{}, err
	}
	return NewOptionsType(result), nil
}

// optionsDefault determines the default value for an Options field when
// the key is absent from the concrete map.
//   - Concrete value → use as default
//   - None → use None
//   - Type literal (Data==nil) → fail (requires a value)
//   - Disjunct → None if present, else first concrete alternative, else fail
func optionsDefault(v Value) (Value, bool) {
	if IsDisjunct(v) {
		disj, _ := AsDisjunct(v)
		alts := disj.Alternatives
		for _, alt := range alts {
			if IsNoneShape(alt) {
				return NewTypeLiteral(TNone), true
			}
		}
		for _, alt := range alts {
			if alt.Data != nil && !IsDisjunct(alt) {
				return alt, true
			}
		}
		return Value{}, false
	}
	if IsNoneShape(v) {
		return v, true
	}
	if v.Data != nil {
		return v, true
	}
	return Value{}, false
}

// FillConcreteOptionDefaults returns m with every schema field that is
// (a) ABSENT from the concrete options map and (b) declares a genuine
// CONCRETE default value materialized into it, so the receiving handler
// or fn param sees a complete map instead of re-deriving defaults. Only
// concrete defaults are injected: a required (bare type-literal) field
// carries no default and stays absent (dispatch already rejected the
// call if it was mandatory), and an optional `T tor None` field's
// default is None — "unset", not a value — so it is left absent too.
//
// Existing keys keep their order and values (the caller's value always
// wins); injected defaults are appended in schema order. A fresh map is
// built only when something is actually filled, so the common
// nothing-to-add case returns m untouched (no alloc, no aliasing).
// Returns m unchanged when pattern is not an Options type or m is not a
// plain concrete map (AsMap returns nil for a type literal, carrier, or
// structural map subtype).
func FillConcreteOptionDefaults(pattern, m Value) Value {
	if !IsOptionsType(pattern) {
		return m
	}
	pot, perr := AsOptionsType(pattern)
	if perr != nil || pot.Fields == nil {
		return m
	}
	cur, err := AsMap(m)
	if err != nil || cur == nil {
		return m
	}
	var filled *OrderedMap
	for _, key := range pot.Fields.Keys() {
		if _, present := cur.Get(key); present {
			continue
		}
		fieldSchema, _ := pot.Fields.Get(key)
		def, dok := optionsDefault(fieldSchema)
		if !dok || !IsConcrete(def) {
			continue // required field, or None/unset default — leave absent
		}
		if filled == nil {
			filled = NewOrderedMap()
			for _, k := range cur.Keys() {
				v, _ := cur.Get(k)
				filled.Set(k, v)
			}
		}
		// FreshenDefault deep-copies a shared-mutable default (flex / Store
		// / Array / instance) so each call gets its OWN copy — injecting the
		// schema's exact Value would let one caller's `set` leak into later
		// calls and into the schema itself. Immutable defaults pass through
		// unchanged (no copy).
		filled.Set(key, FreshenDefault(def))
	}
	if filled == nil {
		return m
	}
	// Preserve the caller map's flavor. A FlexMap is accepted at an Options
	// slot; rebuilding it as a plain Map would silently drop mutability, so
	// downstream `set` would copy instead of mutating in place. Plain Map
	// and FlexMap share the MapPayload shape — only the Parent tag differs.
	if m.Parent.ConformsTo(TFlexMap) {
		return NewFlexMap(filled)
	}
	return NewMap(filled)
}

// unifyOptionsField applies Options unification rules for a single
// field when the key IS present in the concrete map.
//   - Concrete Options value: accept cVal if same parent type (cVal wins)
//   - Type literal: standard Unify (type narrowing)
//   - Disjunct: apply rules to each alternative
//
// r is the enclosing chain's registry, handed to the type-literal arm.
func unifyOptionsField(optVal, cVal Value, r *Registry) (Value, *UnifyError) {
	if IsDisjunct(optVal) {
		disj, _ := AsDisjunct(optVal)
		for _, alt := range disj.Alternatives {
			if unified, err := unifyOptionsField(alt, cVal, r); err == nil {
				return unified, nil
			}
		}
		return Value{}, unifyFail("no disjunct alternative matched", optVal, cVal)
	}
	if optVal.Data != nil {
		baseType := optionsBaseType(optVal)
		if cVal.Parent.ConformsTo(baseType) {
			return cVal, nil
		}
		return Value{}, unifyFail("value does not match field's base type", optVal, cVal)
	}
	return unifyInner(optVal, cVal, r)
}

// optionsBaseType returns the base (non-literal) type for a concrete
// value. For example, integer 42 (Scalar/Number/Integer/42) returns
// TInteger.
func optionsBaseType(v Value) *Type {
	switch {
	case v.Parent.ConformsTo(TInteger):
		return TInteger
	case v.Parent.ConformsTo(TFloat):
		return TFloat
	case v.Parent.ConformsTo(TString):
		return TString
	case v.Parent.ConformsTo(TBoolean):
		return TBoolean
	case v.Parent.Equal(TMap):
		return TMap
	case v.Parent.Equal(TList):
		return TList
	case v.Parent.Equal(TNone):
		return TNone
	default:
		return v.Parent
	}
}
