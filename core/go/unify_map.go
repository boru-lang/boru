package core

// unifyMapFamily owns unification when either side is in the Map
// family (Map, TypedMap, Record, Options) or is a bare Map type
// literal.
//
// The Map family has more sub-shapes than List, but the same
// canonicalization principle applies: type literals normalize to the
// `lit` slot, exclusive shapes (Record, Options) only unify with their
// own kind, and typed-vs-concrete arms are collapsed by ordering.
//
// r is the enclosing chain's registry (nil when unarmed), handed to
// every field / value / child recursion.
func unifyMapFamily(a Value, sa ValueShape, b Value, sb ValueShape, r *Registry) (Value, *UnifyError) {
	// Bare FlexMap type literal: nominal-subtype rule — unifies only
	// with a concrete FlexMap (or another FlexMap literal). A plain
	// map is NOT a FlexMap; the supertype literal `Map` accepts flex
	// values below via the ordinary family rule.
	// WeakFlexMap literal first (deeper node — more specific than the
	// FlexMap arm below, which also accepts weak values by subtree).
	if res, handled, err := unifyFlexLiteral(a, sa, b, sb, TWeakFlexMap, IsWeakFlexMap, "WeakFlexMap"); handled {
		return res, err
	}
	if res, handled, err := unifyFlexLiteral(a, sa, b, sb, TFlexMap, IsFlexMap, "FlexMap"); handled {
		return res, err
	}

	// A flex-family carrier (check-mode `(flex …)` residual with no shape
	// tracking) vs a typed map {:T}: gradual accept, element-tagged. (The
	// shape-tracked map carrier is ShapeMap and flows through the concrete arm
	// below; this covers the bare-carrier fallback.) See unifyCarrierVsTyped.
	if res, handled := unifyCarrierVsTyped(a, sa, b, sb, ShapeTypedMap, TFlexMap); handled {
		return res, nil
	}

	// Bare Map type literal: unifies with any Map-family value except
	// a record (records are nominal). A record-SCHEMA CARRIER is the
	// exception to the exception (NUR068): it abstracts a runtime map
	// INSTANCE conforming to the schema — and a concrete record instance
	// IS a plain map, which the family rule below accepts — so the
	// carrier passes a bare Map constraint exactly as its runtime value
	// would. Only the check-mode carrier takes this arm; a record type
	// BODY as a value keeps the nominal compile failure.
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TMap)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TMap)
	if aLit {
		if sb == ShapeRecord {
			if b.Carrier {
				return b, nil
			}
			return Value{}, unifyFail("Map type literal does not unify with Record", a, b)
		}
		if sb == ShapeOptions {
			info, _ := AsOptionsType(b)
			return NewOptionsType(info.Fields), nil
		}
		if IsMapShape(sb) || bLit {
			return b, nil
		}
		return Value{}, unifyFail("Map type literal needs a map-family right-hand side", a, b)
	}
	if bLit {
		if sa == ShapeRecord {
			if a.Carrier {
				return a, nil
			}
			return Value{}, unifyFail("Map type literal does not unify with Record", a, b)
		}
		if sa == ShapeOptions {
			info, _ := AsOptionsType(a)
			return NewOptionsType(info.Fields), nil
		}
		if IsMapShape(sa) {
			return a, nil
		}
		return Value{}, unifyFail("Map type literal needs a map-family left-hand side", a, b)
	}

	if !IsMapShape(sa) || !IsMapShape(sb) {
		return Value{}, unifyFail("map family requires map-shaped values on both sides", a, b)
	}

	// Record is exclusive — only unifies with another record. Field
	// order is part of a record's identity.
	if sa == ShapeRecord || sb == ShapeRecord {
		if sa != sb {
			if res, handled, err := unifyRecordSchemaCarrierVsMap(a, sa, b, sb, r); handled {
				return res, err
			}
			return Value{}, unifyFail("Record only unifies with Record", a, b)
		}
		aRT, _ := AsRecordType(a)
		bRT, _ := AsRecordType(b)
		return unifyRecordTypes(aRT, bRT, r)
	}

	// Options dispatches to its own handler — the field rules
	// (defaults, disjunct alternatives, concrete-vs-literal) are
	// substantial enough to keep separate.
	if sa == ShapeOptions || sb == ShapeOptions {
		return unifyOptionsFamily(a, sa, b, sb, r)
	}

	// Both typed maps → unify child types.
	if sa == ShapeTypedMap && sb == ShapeTypedMap {
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, err := unifyInner(aCT.Child, bCT.Child, r)
		if err != nil {
			return Value{}, err.withPath("child")
		}
		return NewTypedMap(unified), nil
	}

	// One side typed, other concrete: every value must unify with the
	// child type.
	if sa == ShapeTypedMap || sb == ShapeTypedMap {
		var typed, concrete Value
		if sa == ShapeTypedMap {
			typed, concrete = a, b
		} else {
			typed, concrete = b, a
		}
		ct, _ := AsChildType(typed)
		return unifyTypedMapWithConcrete(concrete, ct.Child, r)
	}

	// Both concrete maps: key-by-key unification, with absent-on-one-
	// side keys defaulting against None.
	aMap, _ := AsMap(a)
	bMap, _ := AsMap(b)
	return unifyConcreteMaps(aMap, bMap, r)
}

// unifyConcreteMaps unifies two concrete maps key by key; r is the
// enclosing chain's registry, handed to every per-key unify.
func unifyConcreteMaps(aMap, bMap ReadMap, r *Registry) (Value, *UnifyError) {
	absentVal := NewTypeLiteral(TAbsent)
	result := NewOrderedMap()

	// Missing-key rule: synthesise an Absent value and unify the
	// present-side value against it. A disjunct that contains Absent
	// (the `?:T` desugaring) accepts it via the disjunct fold; any
	// other shape rejects it — so non-optional missing keys fail
	// naturally without a marker check.
	//
	// Omission rule: a unified value of Absent does not occupy a slot
	// in the result map. Absent is the type of non-presence, so a key
	// whose value is Absent is by definition not present in the map.
	for _, key := range aMap.Keys() {
		aVal, _ := aMap.Get(key)
		bVal, ok := bMap.Get(key)
		if !ok {
			unified, err := unifyInner(aVal, absentVal, r)
			if err != nil {
				return Value{}, err.withPath("key:" + key)
			}
			if Shape(unified) == ShapeAbsent {
				continue
			}
			result.Set(key, unified) //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			continue
		}
		unified, err := unifyInner(aVal, bVal, r)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		if Shape(unified) == ShapeAbsent {
			continue
		}
		result.Set(key, unified)
	}
	for _, key := range bMap.Keys() {
		if _, ok := aMap.Get(key); ok {
			continue
		}
		bVal, _ := bMap.Get(key)
		unified, err := unifyInner(bVal, absentVal, r)
		if err != nil {
			return Value{}, err.withPath("key:" + key)
		}
		if Shape(unified) == ShapeAbsent {
			continue
		}
		result.Set(key, unified) //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
	}
	return NewMap(result), nil
}

// unifyTypedMapWithConcrete unifies a child type constraint against each
// value of the concrete map side. Every value must unify. The result RETAINS
// childType as its element tag (Value.elem) so writes can be enforced and
// reads narrowed, and PRESERVES the concrete side's mutability: a FlexMap
// stays a FlexMap (flex only toggles mutability — it never strips the
// element-type contract), a plain Map stays a plain Map. A CARRIER (a
// check-mode abstract map, IsConcrete false — e.g. the residual of
// `(flex {…})`) has no readable entries: unify gradually to the carrier's own
// kind, tagged, rather than dereferencing nil (the flex-{:T} panic).
// See design/TYPED-CONTAINER-TAG-RETENTION.0.md. r is the enclosing
// chain's registry, handed to every value unify and to the reparent.
func unifyTypedMapWithConcrete(concrete, childType Value, r *Registry) (Value, *UnifyError) {
	if !IsConcrete(concrete) {
		// A carrier (a check-mode abstract map with no readable entries) tags
		// gradually; its concrete elements are validated at runtime.
		out := concrete
		out.SetElemConstraint(childType)
		return out, nil
	}
	m, _ := AsMap(concrete) // concrete map-family (plain or flex) → readable
	res, err := unifyMapValues(m, func(string) Value { return childType }, r)
	if err != nil {
		return Value{}, err
	}
	if om, mErr := AsMutableMap(res); mErr == nil {
		for _, k := range om.Keys() {
			uv, _ := om.Get(k)
			sv, _ := m.Get(k)
			om.Set(k, reparentSwappedElem(sv, uv, r))
		}
	}
	if IsFlexMap(concrete) {
		om, _ := AsMutableMap(res) // res is a fresh plain Map — reuse its OrderedMap
		res = NewFlexMap(om)
	}
	res.SetElemConstraint(childType)
	return res, nil
}

// unifyFieldBags unifies two field-schema maps key-by-key. Both must
// hold the same number of fields and each field-type pair must unify.
// When orderStrict is true the keys must also appear in the same order
// (record-type semantics); when false key order is irrelevant
// (options-type semantics). r is the enclosing chain's registry,
// handed to every field-pair unify.
func unifyFieldBags(a, b *OrderedMap, orderStrict bool, r *Registry) (*OrderedMap, *UnifyError) {
	if a.Len() != b.Len() {
		return nil, &UnifyError{Reason: "field-count mismatch"}
	}
	aKeys := a.Keys()
	bKeys := b.Keys()
	result := NewOrderedMap()
	for i, key := range aKeys {
		if orderStrict && bKeys[i] != key {
			return nil, &UnifyError{Reason: "field order mismatch at " + key}
		}
		bVal, ok := b.Get(key)
		if !ok {
			return nil, &UnifyError{Reason: "field " + key + " missing on right side"}
		}
		aVal, _ := a.Get(key)
		unified, err := unifyInner(aVal, bVal, r)
		if err != nil {
			return nil, err.withPath("field:" + key)
		}
		result.Set(key, unified)
	}
	return result, nil
}

// unifyRecordTypes unifies two record types by unifying their field
// schemas. Keys must match in the same order.
func unifyRecordTypes(a, b RecordTypeInfo, r *Registry) (Value, *UnifyError) {
	result, err := unifyFieldBags(a.Fields, b.Fields, true, r)
	if err != nil {
		return Value{}, err
	}
	return NewRecordType(result), nil
}

// unifyRecordSchemaCarrierVsMap admits a check-mode record-SCHEMA CARRIER
// (Carrier=true, Data=RecordTypeInfo — a record-typed param binding, a `make R`
// residual, or a declared record RETURN, NUR068) against a CONCRETE map by the
// same verdict the RUNTIME reaches for the map instance the carrier abstracts.
// A record param's dispatch pattern is a concrete field-bag map (ResolveSigType
// flattens the record to TMap + NewMap(fields)), and the runtime admits an
// instance via OpenUnifyMap: every pattern key must be present and unify, extra
// instance keys are open. The carrier's schema pins exactly which keys its
// runtime instance has — a record instance carries exactly its schema's keys —
// so mirror that rule over the schema: each map key present in the schema must
// unify with the schema's field type; a map key the schema lacks must accept
// Absent (the `?:T` desugaring), else the pair is provably disjoint. The
// result is the CARRIER itself — the narrower record-typed view, preserving
// its gradual modality and provenance identity. A TYPED-map side ({:T}) takes
// the same admission with the child constraint checked against every schema
// field type (the runtime admits an instance iff every stored value meets the
// child; a field type that cannot meet it makes every instance fail).
// handled=false (fall through to the nominal compile failure) for a non-carrier
// record side or an unreadable map; a record type BODY as a value keeps
// Record-only-unifies-with-Record. r is the enclosing chain's
// registry, handed to every field / key unify.
func unifyRecordSchemaCarrierVsMap(a Value, sa ValueShape, b Value, sb ValueShape, r *Registry) (Value, bool, *UnifyError) {
	var rec, m Value
	switch {
	case sa == ShapeRecord && a.Carrier && (sb == ShapeMap || sb == ShapeTypedMap):
		rec, m = a, b
	case sb == ShapeRecord && b.Carrier && (sa == ShapeMap || sa == ShapeTypedMap):
		rec, m = b, a
	default:
		return Value{}, false, nil
	}
	rt, _ := AsRecordType(rec)
	if rt.Fields == nil {
		return Value{}, false, nil
	}
	// A TYPED map ({:T}) admits the carrier iff every schema field type can
	// meet the child constraint — the runtime rule projected over the schema
	// (unifyTypedMapWithConcrete unifies every stored VALUE with the child,
	// and a field's values inhabit its field type). An impossible field
	// (String field vs {:Integer}) is provably disjoint, exactly as every
	// runtime instance of the schema would be.
	if ct, err := AsChildType(m); err == nil {
		for _, key := range rt.Fields.Keys() {
			fVal, _ := rt.Fields.Get(key)
			if _, uerr := unifyInner(fVal, ct.Child, r); uerr != nil {
				return Value{}, true, uerr.withPath("field:" + key)
			}
		}
		return rec, true, nil
	}
	// Read the field bag through the payload probe, NOT AsMap: a ShapeMap
	// value whose payload is not a readable MapPayload (an ExtensionPayload
	// under a TMap parent, a Go-API-built MapPayload{M: nil} — AsMap boxes
	// that nil *OrderedMap in a non-nil ReadMap, so an interface-nil guard
	// misses it) declines to the nominal compile failure instead of dereferencing.
	pm, pok := m.Data.(MapPayload)
	if !pok || pm.M == nil {
		return Value{}, false, nil
	}
	mm := pm.M
	absentVal := NewTypeLiteral(TAbsent)
	for _, key := range mm.Keys() {
		mVal, _ := mm.Get(key)
		fVal, ok := rt.Fields.Get(key)
		if !ok {
			if _, err := unifyInner(mVal, absentVal, r); err != nil {
				return Value{}, true, err.withPath("key:" + key)
			}
			continue
		}
		if _, err := unifyInner(fVal, mVal, r); err != nil {
			return Value{}, true, err.withPath("key:" + key)
		}
	}
	return rec, true, nil
}
