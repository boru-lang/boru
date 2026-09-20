package core

// This file is the home of bare-name type resolution — the canonical
// cascade of ADR-012 rule 4. A capitalised name arriving as a Word
// resolves in exactly two arms, in order:
//
//  1. the registry's binding store (`TopTypeBody` — user types and
//     dynamic bindings; disjoint from the builtin arm because
//     InstallType declines minting over an existing type name), then
//  2. the LIVE builtin name table plus the slash-path resolver
//     (ResolveBuiltinTypeName below).
//
// Sites with a Registry in hand run both arms (stepWord, the forward
// planner, the R-variants below); registry-free sites run arm 2 only.
// Do NOT re-implement either arm inline — that is how the pre-ADR-012
// per-site cascades drifted apart (NUR059).

// ResolveBuiltinTypeName resolves a bare capitalised name against the
// live builtin name table, then the slash-path resolver
// (`String/ProperString`). The registry-free arm of the canonical
// cascade. Returns (nil, false) for names that are not builtin types —
// user-defined type names resolve through the registry arm, not here.
func ResolveBuiltinTypeName(name string) (*Type, bool) {
	if t, ok := TypeNames[name]; ok {
		return t, true
	}
	return ResolveTypePath(name)
}

// resolveWordValue is the shared scalar-word resolver behind
// ResolveWordValue / ResolveWordValueR. A nil registry skips the
// registry arm.
func resolveWordValue(v Value, r *Registry) Value {
	if !IsWord(v) {
		return v
	}
	_as1, _ := AsWord(v)
	name := _as1.Name
	switch name {
	case "true":
		return NewBoolean(true)
	case "false":
		return NewBoolean(false)
	}
	if r != nil {
		if tv, ok := r.TopTypeBody(name); ok {
			return tv
		}
	}
	if t, ok := ResolveBuiltinTypeName(name); ok {
		return NewTypeLiteral(t)
	}
	return NewAtom(name)
}

// ResolveWordsDeep recursively resolves word values to their semantic form.
// For lists, each element is resolved; for maps, each value is resolved.
// Scalar words are resolved via ResolveWordValue.
//
// Lives here (not in unify.go) because the operation is value
// preprocessing — interpreting `true`/`false`/type names that the
// parser left as bare Words — and is independent of unification.
func ResolveWordsDeep(v Value) Value {
	return resolveWordsDeep(v, nil)
}

// ResolveWordsDeepR is ResolveWordsDeep with the registry arm enabled
// — the resolver the unify prepass uses when a Registry is in hand,
// so user-typed words inside patterns, typed-container children
// (`[:Foo]`), and map values resolve instead of degrading to Atoms.
func ResolveWordsDeepR(v Value, r *Registry) Value {
	return resolveWordsDeep(v, r)
}

func resolveWordsDeep(v Value, r *Registry) Value {
	if IsWord(v) {
		return resolveWordValue(v, r)
	}
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		lst, _ := AsList(v)
		elems := lst.Slice()
		resolved := make([]Value, len(elems))
		for i, e := range elems {
			resolved[i] = resolveWordsDeep(e, r)
		}
		return NewList(resolved)
	}
	// Disjunct alternatives: a bare type-name alternative arrives as a
	// Word (the `{a?:T}` optional-field desugar, a stored `tor` shape)
	// and must resolve before unification can see the union. When an
	// alternative changed, re-simplify so the resolved disjunct reduces
	// to the same canonically-ordered value `tor` builds — preserving
	// the pinned `{a?:T}` ≡ `{a:(T tor None tor Absent)}` equality.
	// The Data replacement keeps the Parent (Enum subtypes) and every
	// other field intact.
	if d, ok := v.Data.(DisjunctInfo); ok && v.Parent.ConformsTo(TDisjunct) {
		changed := false
		alts := make([]Value, len(d.Alternatives))
		for i, a := range d.Alternatives {
			alts[i] = resolveWordsDeep(a, r)
			if !changed && !ExactEqual(a, alts[i]) {
				changed = true
			}
		}
		if !changed {
			return v
		}
		nd := d
		nd.Alternatives = SimplifyDisjunctAlts(alts)
		out := v
		out.Data = nd
		return out
	}
	// Typed list / typed map: resolve the child constraint so a Word
	// referring to a predicate type (e.g. `[:Pos]`) reaches its FnDef
	// body, which unifyInner can then detect as a predicate constraint.
	// Inline elements/entries (`[:Integer 1 2]`, `{:Integer a:1}`) are
	// PRESERVED and resolved deep — the child-only rebuild silently
	// emptied populated typed-container defaults in record/class
	// schemas (found by the TS-parity review; the TS twin preserves).
	if IsTypedList(v) {
		ct, _ := AsChildType(v)
		child := resolveWordsDeep(ct.Child, r)
		if len(ct.Elements) == 0 {
			return NewTypedList(child)
		}
		elems := make([]Value, len(ct.Elements))
		for i, e := range ct.Elements {
			elems[i] = resolveWordsDeep(e, r)
		}
		return NewTypedListWithElements(child, elems)
	}
	if IsTypedMap(v) {
		ct, _ := AsChildType(v)
		child := resolveWordsDeep(ct.Child, r)
		if len(ct.Entries) == 0 {
			return NewTypedMap(child)
		}
		entries := make([]ChildEntry, len(ct.Entries))
		for i, e := range ct.Entries {
			entries[i] = ChildEntry{Key: e.Key, Value: resolveWordsDeep(e.Value, r)}
		}
		return NewTypedMapWithEntries(child, entries)
	}
	if v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
		m, _ := AsMap(v)
		result := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			result.Set(key, resolveWordsDeep(val, r))
		}
		return NewMap(result)
	}
	return v
}
