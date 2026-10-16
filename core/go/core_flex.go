package core

import "fmt"

// Flex node conversion primitives — the algorithms behind the `flex`
// and `node` words and the Node-targeted `make` overloads
// (`make FlexMap m`, `make Map flexmap`, …). The word names and
// dispatch wiring live in lang/go/native/native_flex.go and
// native_make.go; only the algorithms live kernel-side, mirroring
// core_make.go.
//
// Both conversions are DEEP and COPYING: a flex node must never alias
// the entries of an immutable source (plain Map literals are also
// *OrderedMap-backed, so sharing the pointer would let mutation leak
// into "immutable" data), and a `node` result must be transitively
// immutable. See design/FLEX-NODES.10.md.

// FlexDeepCopy returns a mutable flex copy of a Node value: maps
// (plain or flex) become fresh FlexMaps, lists become fresh FlexLists,
// and nested Nodes are converted recursively so in-place mutation is
// available at every depth. Non-Node values (scalars, Functions,
// Ideal instances, …) pass through unchanged — scalars are immutable
// and pointer-backed Ideals already have shared-mutation semantics.
//
// Map/list-family values whose payload is not a readable container
// (record/options/table type bodies, bare child-type constraints) are
// an error: flexing a type body is meaningless. A concrete {:T}/[:T]
// container KEEPS its element tag (Value.elem) through the flex — flex only
// toggles mutability, it never drops the element-type contract — so a typed
// flex node enforces writes just like its immutable form.
func FlexDeepCopy(v Value) (Value, error) {
	if ParentOf(&v).ConformsTo(TMap) && IsConcrete(&v) {
		m, err := AsMap(v)
		if err != nil || m == nil {
			return Value{}, fmt.Errorf("flex: cannot make a flex copy of %s", v.String())
		}
		om := NewOrderedMap()
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			fv, ferr := FlexDeepCopy(val)
			if ferr != nil {
				return Value{}, ferr
			}
			om.Set(k, fv)
		}
		out := WithPos(NewFlexMap(om), v)
		if elem, ok := v.ElemConstraint(); ok { // flex only toggles mutability — keep {:T}
			out.SetElemConstraint(elem)
		}
		return out, nil
	}
	if ParentOf(&v).ConformsTo(TList) && IsConcrete(&v) {
		lst, err := AsList(v)
		if err != nil || lst.IsNil() {
			return Value{}, fmt.Errorf("flex: cannot make a flex copy of %s", v.String())
		}
		elems := make([]Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			fv, ferr := FlexDeepCopy(lst.Get(i))
			if ferr != nil {
				return Value{}, ferr
			}
			elems[i] = fv
		}
		out := WithPos(NewFlexList(elems), v)
		if elem, ok := v.ElemConstraint(); ok { // keep [:T] through flex
			out.SetElemConstraint(elem)
		}
		return out, nil
	}
	if ParentOf(&v).ConformsTo(TXml) && IsConcrete(&v) {
		tag, attr, cren, ok := xmlParts(v)
		if !ok {
			return Value{}, fmt.Errorf("flex: cannot make a flex copy of %s", v.String())
		}
		nattr, ncren, ferr := flexXmlChildren(attr, cren)
		if ferr != nil {
			return Value{}, ferr
		}
		return WithPos(NewFlexXml(tag, nattr, ncren), v), nil
	}
	return v, nil
}

// flexXmlChildren deep-flex-copies an XML element's attributes and child
// nodes into fresh containers — the shared core of FlexDeepCopy's Xml
// branch. The attribute *OrderedMap is rebuilt (never aliased with an
// immutable source) so a flex element's attributes are independently
// mutable.
func flexXmlChildren(attr *OrderedMap, cren []Value) (*OrderedMap, []Value, error) {
	nattr := NewOrderedMap()
	if attr != nil {
		for _, k := range attr.Keys() {
			av, _ := attr.Get(k)
			fv, ferr := FlexDeepCopy(av)
			if ferr != nil {
				return nil, nil, ferr
			}
			nattr.Set(k, fv)
		}
	}
	ncren := make([]Value, len(cren))
	for i, c := range cren {
		fv, ferr := FlexDeepCopy(c)
		if ferr != nil {
			return nil, nil, ferr
		}
		ncren[i] = fv
	}
	return nattr, ncren, nil
}

// NodeDeepCopy is the inverse of FlexDeepCopy: FlexMaps become plain
// immutable Maps, FlexLists become plain Lists, recursing through
// plain containers too so nested flex nodes are normalised at every
// depth. A container with no flex anywhere inside is returned
// UNCHANGED (same underlying container) — `node plainmap` is identity,
// which keeps container-identity equality (ExactEqual / `eq`) stable.
func NodeDeepCopy(v Value) (Value, error) {
	if !containsFlex(v) {
		return v, nil
	}
	if ParentOf(&v).ConformsTo(TMap) && IsConcrete(&v) {
		m, err := AsMap(v)
		if err != nil || m == nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return Value{}, fmt.Errorf("node: cannot convert %s to an immutable node", v.String())
		}
		om := NewOrderedMap()
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			nv, nerr := NodeDeepCopy(val)
			if nerr != nil {
				return Value{}, nerr
			}
			om.Set(k, nv)
		}
		out := WithPos(NewMap(om), v)
		if elem, ok := v.ElemConstraint(); ok { // node only toggles mutability — keep {:T}
			out.SetElemConstraint(elem)
		}
		return out, nil
	}
	if ParentOf(&v).ConformsTo(TList) && IsConcrete(&v) {
		lst, err := AsList(v)
		if err != nil || lst.IsNil() {
			return Value{}, fmt.Errorf("node: cannot convert %s to an immutable node", v.String())
		}
		elems := make([]Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			nv, nerr := NodeDeepCopy(lst.Get(i))
			if nerr != nil {
				return Value{}, nerr
			}
			elems[i] = nv
		}
		out := WithPos(NewList(elems), v)
		if elem, ok := v.ElemConstraint(); ok { // keep [:T] through un-flex
			out.SetElemConstraint(elem)
		}
		return out, nil
	}
	if ParentOf(&v).ConformsTo(TXml) && IsConcrete(&v) {
		tag, attr, cren, ok := xmlParts(v)
		if !ok { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return Value{}, fmt.Errorf("node: cannot convert %s to an immutable node", v.String())
		}
		nattr := NewOrderedMap()
		if attr != nil {
			for _, k := range attr.Keys() {
				av, _ := attr.Get(k)
				nv, nerr := NodeDeepCopy(av)
				if nerr != nil {
					return Value{}, nerr
				}
				nattr.Set(k, nv)
			}
		}
		ncren := make([]Value, len(cren))
		for i, c := range cren {
			nv, nerr := NodeDeepCopy(c)
			if nerr != nil {
				return Value{}, nerr
			}
			ncren[i] = nv
		}
		return WithPos(NewXmlElement(tag, nattr, ncren), v), nil
	}
	return v, nil //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
}

// containsFlex reports whether v is a flex node or transitively
// contains one. Drives NodeDeepCopy's identity fast path.
func containsFlex(v Value) bool {
	if IsFlexNode(v) || IsWeakFlexNode(v) {
		return true
	}
	if !IsConcrete(&v) {
		return false
	}
	if ParentOf(&v).ConformsTo(TMap) {
		if m, err := AsMap(v); err == nil && m != nil {
			for _, k := range m.Keys() {
				val, _ := m.Get(k)
				if containsFlex(val) {
					return true
				}
			}
		}
		return false
	}
	if ParentOf(&v).ConformsTo(TList) {
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			for i := 0; i < lst.Len(); i++ {
				if containsFlex(lst.Get(i)) {
					return true
				}
			}
		}
		return false
	}
	if ParentOf(&v).ConformsTo(TXml) {
		if _, attr, cren, ok := xmlParts(v); ok {
			if attr != nil {
				for _, k := range attr.Keys() {
					av, _ := attr.Get(k)
					if containsFlex(av) {
						return true
					}
				}
			}
			for _, c := range cren {
				if containsFlex(c) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// AdoptIntoFlex normalises a value being STORED INTO a flex container
// (set / push / unshift / append) so a flex tree stays ENTIRELY
// mutable — the flex column's half of the "never mixed" invariant
// (design/FLEX-NODES.10.md):
//
//   - a concrete plain Map/List is deep-flex-copied. Without this the
//     tree goes mixed, and a later write into the immutable inner is
//     COPY-RETURNING and silently lost: `set a {b:1} f` then
//     `set b 9 f.a` left f.a.b at 1. The copy also keeps the
//     no-aliasing construction rule — the source literal/value is
//     never shared.
//   - a value that is already a flex node passes through as a SHARED
//     handle: both trees stay entirely mutable, and sharing what the
//     caller explicitly passes mirrors the class-default rule
//     (schema defaults freshen; provided values share).
//   - scalars, Ideal instances (Array/Store/objects — the documented
//     shared-identity column), and map/list-family TYPE BODIES
//     (records, options, child constraints — values here, nothing to
//     flex) pass through unchanged.
func AdoptIntoFlex(v Value) (Value, error) {
	// Weak flex nodes are reference cells like their flex parents:
	// storing one into a flex tree shares the handle (never a
	// demoting deep copy). design/FLEX-ATTRS.1.md §4.7.
	if IsFlexNode(v) || IsWeakFlexNode(v) {
		return v, nil
	}
	if !IsConcrete(&v) {
		return v, nil
	}
	if ParentOf(&v).ConformsTo(TMap) {
		if m, err := AsMap(v); err == nil && m != nil {
			return FlexDeepCopy(v)
		}
		return v, nil
	}
	if ParentOf(&v).ConformsTo(TList) {
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			return FlexDeepCopy(v)
		}
		return v, nil
	}
	if ParentOf(&v).ConformsTo(TXml) {
		return FlexDeepCopy(v)
	}
	return v, nil
}

// AdoptIntoNode is the dual for values stored into PLAIN Map/List
// containers (the copy-returning set / push / unshift): any flex
// inside is deep-copied to its immutable shape, so an "immutable"
// container can never change underneath through a live flex handle.
// A value with no flex anywhere inside is returned unchanged —
// sharing immutable structure is safe (NodeDeepCopy's identity fast
// path).
func AdoptIntoNode(v Value) (Value, error) {
	return NodeDeepCopy(v)
}

// MakeNodeHandler is the 2-arg [Node, Any] make handler (TypeArgs at
// position 0). It owns the Node-family targets:
//
//	make FlexMap m    — deep mutable copy of a map-family Node
//	make FlexList l   — deep mutable copy of a list-family Node
//	make Map v        — deep immutable conversion (identity when v
//	                    contains no flex nodes); `make Map flexmap`
//	                    is the inverse of `make FlexMap m`
//	make List v       — same for lists
//
// Anything else that lands here — a structural type body in the
// TypeArgs slot (RecordTypeInfo etc. have Parent=TMap, which conforms
// to Node), or a Node target this handler doesn't own (Node itself,
// Inspect, Args) — defers to the generic MakeHandler so the Ideal
// instantiation paths are never hijacked.
func MakeNodeHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	if !IsBareTypeNode(&targetVal) {
		return MakeHandler([]Value{targetVal, srcVal}, nil, nil, reg)
	}
	// Host-registered Ideal kinds keep precedence over the kernel Node
	// conversions — same registry-first dispatch as MakeHandler /
	// MakeObjHandler, so a host that claims a Node-family target still
	// owns its instantiation.
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
		if m := reg.Ideals.Match(targetVal); m != nil && !m.available() {
			return nil, fmt.Errorf("make: the %s type-kind is not available in this registry", m.Name)
		}
	}
	target := &targetVal
	switch {
	case target.Equal(TFlexMap):
		if !ParentOf(&srcVal).ConformsTo(TMap) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: FlexMap source must be a concrete map, got %s", srcVal.String())
		}
		out, err := FlexDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TFlexList):
		if !ParentOf(&srcVal).ConformsTo(TList) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: FlexList source must be a concrete list, got %s", srcVal.String())
		}
		out, err := FlexDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TMap):
		if !ParentOf(&srcVal).ConformsTo(TMap) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: Map source must be a concrete map, got %s", srcVal.String())
		}
		out, err := NodeDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TList):
		if !ParentOf(&srcVal).ConformsTo(TList) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: List source must be a concrete list, got %s", srcVal.String())
		}
		out, err := NodeDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TFlexXml):
		if !ParentOf(&srcVal).ConformsTo(TXml) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: FlexXml source must be a concrete Xml element, got %s", srcVal.String())
		}
		out, err := FlexDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TXml):
		if !ParentOf(&srcVal).ConformsTo(TXml) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: Xml source must be a concrete Xml element, got %s", srcVal.String())
		}
		out, err := NodeDeepCopy(srcVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	case target.Equal(TWeakFlexMap):
		if !ParentOf(&srcVal).ConformsTo(TMap) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: WeakFlexMap source must be a concrete map, got %s", srcVal.String())
		}
		m, merr := AsMap(srcVal)
		if merr != nil || m == nil {
			return nil, fmt.Errorf("make: WeakFlexMap source must be a concrete map, got %s", srcVal.String())
		}
		out, refusal := PopulateWeakFlexMap(m)
		if refusal != nil {
			return nil, weakRefusalError(reg, "make", "WeakFlexMap", refusal)
		}
		ThreadPos(&out, &srcVal)
		// Weak keeps {:T}, like flex: the constraint rides the header
		// elem on flex/adopted sources, and the ChildTypeInfo payload
		// on a concrete typed-map literal.
		if elem, ok := srcVal.ElemConstraint(); ok {
			out.SetElemConstraint(elem)
		} else if ci, cerr := AsChildType(srcVal); cerr == nil {
			out.SetElemConstraint(ci.Child)
		}
		return []Value{out}, nil
	case target.Equal(TWeakFlexList):
		if !ParentOf(&srcVal).ConformsTo(TList) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: WeakFlexList source must be a concrete list, got %s", srcVal.String())
		}
		lst, lerr := AsList(srcVal)
		if lerr != nil || lst.IsNil() {
			return nil, fmt.Errorf("make: WeakFlexList source must be a concrete list, got %s", srcVal.String())
		}
		out, refusal := PopulateWeakFlexList(lst)
		if refusal != nil {
			return nil, weakRefusalError(reg, "make", "WeakFlexList", refusal)
		}
		ThreadPos(&out, &srcVal)
		// Weak keeps [:T], like flex (header elem or typed-literal payload).
		if elem, ok := srcVal.ElemConstraint(); ok {
			out.SetElemConstraint(elem)
		} else if ci, cerr := AsChildType(srcVal); cerr == nil {
			out.SetElemConstraint(ci.Child)
		}
		return []Value{out}, nil
	case target.Equal(TWeakFlexXml):
		if !ParentOf(&srcVal).ConformsTo(TXml) || !IsConcrete(&srcVal) {
			return nil, fmt.Errorf("make: WeakFlexXml source must be a concrete Xml element, got %s", srcVal.String())
		}
		tag, attr, cren, ok := xmlParts(srcVal)
		if !ok {
			return nil, fmt.Errorf("make: WeakFlexXml source must be a concrete Xml element, got %s", srcVal.String())
		}
		out, refusal := PopulateWeakFlexXml(tag, attr, cren)
		if refusal != nil {
			return nil, weakRefusalError(reg, "make", "WeakFlexXml", refusal)
		}
		return []Value{WithPos(out, srcVal)}, nil
	default:
		return MakeHandler([]Value{targetVal, srcVal}, nil, nil, reg)
	}
}
