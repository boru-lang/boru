package core

// Universal deep clone.
//
// CloneValue returns a deep, independent copy of any Value. The rule is
// payload-driven: every kernel payload variant (see payload.go) is
// classified as either MUTABLE — a container whose contents can change
// after construction, which must be duplicated so the clone and the
// original cannot alias — or IMMUTABLE — a scalar, time, type body,
// function, or engine marker that is safe to share because nothing can
// mutate it in place.
//
//   - Mutable, deep-copied: List, Map, FlexList, Store, Object
//     instance, Table, and an Error's raise-payload map. Pointer-backed
//     graphs (Store/FlexList/Object via their prototype or element
//     links) are cloned cycle-safely — a self-referential or shared node
//     is reproduced with the SAME sharing, never expanded infinitely.
//   - Immutable, shared: Integer/Float/Big*/String/Boolean/Atom/Pathon/
//     None, the Time family, every type body / descriptor, functions
//     (FnDef/FnUndef), and control markers. Sharing them is correct
//     because boru never mutates them in place.
//   - ExtensionPayload (host types): cloned via the optional DeepCloner
//     capability if the body implements it; otherwise shared.
//
// The clone preserves the original's Parent type, so a refine subtype, a
// typed list/map, or an object type survives the copy. Container clones
// receive a fresh ID (they are independent values), matching the kernel
// constructors. CloneValue never panics (ADR-005).

// DeepCloner is the optional capability a host type stored in an
// ExtensionPayload implements to take part in deep cloning. DeepClone
// must return a fully independent copy of the body. Bodies that do not
// implement it are shared by the clone — safe for immutable host types,
// and the explicit, documented fallback for the kernel's opaque escape
// hatch.
type DeepCloner interface {
	DeepClone() any
}

// CloneValue returns a deep, independent copy of v. See the file comment
// for the per-payload contract.
func CloneValue(v Value) Value {
	return CloneValueKeeping(v, nil)
}

// CloneValueKeeping is CloneValue with a KEEP set: a compound whose ID is in
// keep is returned as it is, the same instance, and the clone continues
// around it. It is the selective (spine-only) freshen a compiled fn unit
// needs for a body literal that EMBEDS an enclosing binding's container —
// `def c [9]  def mk fn [[] [List] [[c]]]`: the interpreter constructs the
// outer list fresh per call and the member is the binding's one instance,
// so `(mk) eq (mk)` is false and `((mk) get 0) eq c` true. A deep clone
// broke the member's identity and a shared const the outer's, so the shape
// declined until 2026-09-24 (PR #225 P1's open item); the keep set names
// the members, the spine clones, and both identities hold. A nil or empty
// keep is CloneValue.
func CloneValueKeeping(v Value, keep map[string]bool) Value {
	c := cloner{seen: make(map[any]any), keep: keep}
	return c.clone(v)
}

// cloner carries the identity map that makes pointer-backed graphs
// clone cycle-safely: each original pointer payload maps to its single
// clone, so shared substructure stays shared and cycles terminate — and
// the keep set (CloneValueKeeping) of the values it returns unchanged.
type cloner struct {
	seen map[any]any
	keep map[string]bool
}

// withPayload returns a copy of v carrying a new payload and a fresh ID,
// preserving Parent/Quoted/Eval/Pos/Carrier. Used for the mutable
// containers, whose clone is a distinct value from the original.
func (c *cloner) withPayload(v Value, data Payload) Value {
	v.Data = data
	v.ID = GenerateID(IDPrefixForType(v.Parent))
	return v
}

func (c *cloner) clone(v Value) Value {
	if v.ID != "" && c.keep[v.ID] {
		return v // an enclosing binding's instance the literal embeds: shared, as the interpreter shares it
	}
	switch p := v.Data.(type) {
	case nil:
		// Type literal / bare lattice node — no payload to copy.
		return v

	case ListPayload:
		return c.withPayload(v, ListPayload{Elems: c.cloneSlice(p.Elems)})

	case MapPayload:
		return c.withPayload(v, MapPayload{M: c.cloneOrderedMap(p.M)})

	case *FlexListData:
		if cp, ok := c.seen[p]; ok {
			return c.withPayload(v, cp.(*FlexListData))
		}
		nd := &FlexListData{}
		c.seen[p] = nd
		nd.Elems = c.cloneSlice(p.Elems)
		return c.withPayload(v, nd)

	case *WeakFlexMapData:
		if cp, ok := c.seen[p]; ok {
			return c.withPayload(v, cp.(*WeakFlexMapData))
		}
		nd := CloneWeakFlexMapData(p, c.clone)
		c.seen[p] = nd
		return c.withPayload(v, nd)

	case *WeakFlexListData:
		if cp, ok := c.seen[p]; ok {
			return c.withPayload(v, cp.(*WeakFlexListData))
		}
		nd := CloneWeakFlexListData(p, c.clone)
		c.seen[p] = nd
		return c.withPayload(v, nd)

	case *WeakFlexXmlData:
		if cp, ok := c.seen[p]; ok {
			return c.withPayload(v, cp.(*WeakFlexXmlData))
		}
		nd := CloneWeakFlexXmlData(p, c.clone, c.cloneOrderedMap)
		c.seen[p] = nd
		return c.withPayload(v, nd)

	case *StoreInstanceInfo:
		return c.withPayload(v, c.cloneStore(p))

	case ClassInstanceInfo:
		return c.withPayload(v, c.cloneObject(p))

	case TableData:
		nd := p                        // copy the value struct (Record/flags/name)
		nd.Rows = c.cloneSlice(p.Rows) // duplicate the row values
		return c.withPayload(v, nd)

	case ErrorInfo:
		nd := p // Message/Code are immutable strings
		if p.Data != nil {
			nd.Data = c.cloneOrderedMap(p.Data)
		}
		return c.withPayload(v, nd)

	case ExtensionPayload:
		if dc, ok := p.Body.(DeepCloner); ok {
			return c.withPayload(v, ExtensionPayload{Body: dc.DeepClone()})
		}
		// Opaque host body with no DeepCloner — share it (immutable by
		// contract, or the host opted out of deep cloning).
		return v

	default:
		// Immutable payload (scalars, time family, type bodies,
		// functions, markers) — sharing is correct; nothing mutates it.
		return v
	}
}

// cloneSlice deep-clones each element of a []Value into a fresh slice.
func (c *cloner) cloneSlice(src []Value) []Value {
	if src == nil {
		return nil
	}
	out := make([]Value, len(src))
	for i, e := range src {
		out[i] = c.clone(e)
	}
	return out
}

// cloneOrderedMap deep-clones an *OrderedMap, preserving key order and
// cloning each value. Pointer identity is tracked so a map shared by two
// branches of the graph (or a cycle through a map) clones once.
func (c *cloner) cloneOrderedMap(m *OrderedMap) *OrderedMap {
	if m == nil {
		return nil
	}
	if cp, ok := c.seen[m]; ok {
		return cp.(*OrderedMap)
	}
	out := NewOrderedMap()
	out.Implicit = m.Implicit
	c.seen[m] = out
	for _, k := range m.Keys() {
		mv, _ := m.Get(k)
		out.Set(k, c.clone(mv))
	}
	if m.Meta != nil {
		out.Meta = make(map[string]any, len(m.Meta))
		for k, mv := range m.Meta {
			out.Meta[k] = mv
		}
	}
	return out
}

// cloneStore deep-clones a *StoreInstanceInfo. The own Data layer is
// duplicated and its values cloned; the prototype chain is cloned too
// (cycle-guarded) so the copy inherits the same scope structure without
// aliasing it. The clone is detached — Parent/ParentKey reset — so it is
// a fresh root until re-inserted into a container, exactly like a store
// from `make Store`.
func (c *cloner) cloneStore(s *StoreInstanceInfo) *StoreInstanceInfo {
	if s == nil {
		return nil
	}
	if cp, ok := c.seen[s]; ok {
		return cp.(*StoreInstanceInfo)
	}
	out := &StoreInstanceInfo{TypeName: s.TypeName}
	c.seen[s] = out
	if s.Data != nil {
		out.Data = make(map[string]Value, len(s.Data))
		for k, mv := range s.Data {
			cv := c.clone(mv)
			out.Data[k] = cv
			// Re-establish the COW back-link for nested stores so the
			// clone's own propagation stays internally consistent.
			if child, ok := cv.Data.(*StoreInstanceInfo); ok {
				child.Parent = out
				child.ParentKey = k
			}
		}
	}
	// Tombstones travel with the layer: a clone that dropped them would
	// resurrect every deleted key through the cloned prototype chain.
	if s.Deleted != nil {
		out.Deleted = make(map[string]bool, len(s.Deleted))
		for k, d := range s.Deleted {
			out.Deleted[k] = d
		}
	}
	out.Prototype = c.cloneStore(s.Prototype)
	return out
}

// cloneObject deep-clones an ClassInstanceInfo: the field map is
// duplicated value-by-value. Instances are flat, so there is no
// prototype to clone. TypeRef is a shared type descriptor (immutable).
func (c *cloner) cloneObject(o ClassInstanceInfo) ClassInstanceInfo {
	out := ClassInstanceInfo{TypeRef: o.TypeRef}
	if o.Fields != nil {
		out.Fields = c.cloneOrderedMap(o.Fields)
	}
	return out
}
