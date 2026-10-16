package core

import (
	"fmt"
	"weak"
)

// Weak flex nodes — WeakFlexMap / WeakFlexList / WeakFlexXml, the
// weak-valued children of the flex column (design/FLEX-ATTRS.1.md §4).
//
// The value domain follows Python's weakref rule, decided 2026-07-22:
//
//   - Scalars (the Scalar branch — no GC identity) store STRONGLY.
//   - Mutable handles (flex nodes incl. weak ones, Store, Resource and
//     class instances — all identified by a shared pointer) store
//     WEAKLY: the entry lives while the program keeps another handle.
//   - Everything else — immutable Nodes, fn values, type literals,
//     Error values, host extension payloads, none — is REFUSED with a
//     structured WeakRefusal the word layer renders as a rich
//     diagnostic.
//
// Representation is snapshot-at-funnel: the read accessors (AsMap /
// AsList / xmlParts) SWEEP dead slots and materialize a STRONG
// snapshot, so every read operation observes one internally-consistent
// world and the iterate-then-get hazard cannot arise. AsMutableMap /
// AsFlexList / AsFlexXml refuse the weak payloads, so every unported
// mutating handler fails loudly (ADR-005) — mutation goes through the
// dedicated weak `set`/`append` signatures only.
//
// Reaping is lazy (at the funnels). runtime.AddCleanup is deliberately
// NOT used: cleanups run concurrently and the containers are
// unsynchronized.

// ── payloads ─────────────────────────────────────────────────────────

// WeakFlexMapData is the sealed payload of a WeakFlexMap: insertion-
// ordered keys over strong-or-weak slots.
type WeakFlexMapData struct {
	keys  []string
	slots map[string]weakSlot
}

// WeakFlexListData is the sealed payload of a WeakFlexList. Collected
// elements splice-compact at observation points (survivor order is
// preserved; a hole would violate the no-sparse-lists invariant).
type WeakFlexListData struct{ slots []weakSlot }

// WeakFlexXmlData is the sealed payload of a WeakFlexXml element:
// attributes are strings (always strong, stored directly); child
// elements are slots that sweep like list elements.
type WeakFlexXmlData struct {
	Tag   string
	Attr  *OrderedMap
	slots []weakSlot
}

func (*WeakFlexMapData) payloadMarker()  {}
func (*WeakFlexListData) payloadMarker() {}
func (*WeakFlexXmlData) payloadMarker()  {}

// ── slots ────────────────────────────────────────────────────────────

// weakSlot holds either a strong Value (ref == nil) or a weak
// reference with enough template data to revive the Value.
type weakSlot struct {
	strong Value
	ref    weakRevive
}

// weakRevive revives a weakly-held value: (value, true) while the
// referent is alive, (zero, false) once collected.
type weakRevive interface {
	revive() (Value, bool)
}

// weakRefOf is the one generic weak-reference shape: a weak.Pointer to
// the identity allocation plus a wrap closure that rebuilds the Value
// around a revived strong pointer. The closure must never capture the
// strong pointer itself — only template data (Parent tag, flags,
// non-identity struct fields).
type weakRefOf[T any] struct {
	ptr  weak.Pointer[T]
	wrap func(*T) Value
}

func (w weakRefOf[T]) revive() (Value, bool) {
	p := w.ptr.Value()
	if p == nil {
		return Value{}, false
	}
	return w.wrap(p), true
}

// ── predicates / constructors / accessors ────────────────────────────

// IsWeakFlexMap reports whether v is a concrete WeakFlexMap value.
func IsWeakFlexMap(v Value) bool {
	if v.Parent() == nil || !v.Parent().Equal(TWeakFlexMap) {
		return false
	}
	_, ok := v.Data().(*WeakFlexMapData)
	return ok
}

// IsWeakFlexList reports whether v is a concrete WeakFlexList value.
func IsWeakFlexList(v Value) bool {
	if v.Parent() == nil || !v.Parent().Equal(TWeakFlexList) {
		return false
	}
	_, ok := v.Data().(*WeakFlexListData)
	return ok
}

// IsWeakFlexXml reports whether v is a concrete WeakFlexXml value.
func IsWeakFlexXml(v Value) bool {
	if v.Parent() == nil || !v.Parent().Equal(TWeakFlexXml) {
		return false
	}
	_, ok := v.Data().(*WeakFlexXmlData)
	return ok
}

// IsWeakFlexNode reports whether v is a concrete weak flex node of any
// kind.
func IsWeakFlexNode(v Value) bool {
	return IsWeakFlexMap(v) || IsWeakFlexList(v) || IsWeakFlexXml(v)
}

// NewWeakFlexMap creates an empty WeakFlexMap.
func NewWeakFlexMap() Value {
	return NewValueRaw(TWeakFlexMap, &WeakFlexMapData{slots: map[string]weakSlot{}})
}

// NewWeakFlexList creates an empty WeakFlexList.
func NewWeakFlexList() Value {
	return NewValueRaw(TWeakFlexList, &WeakFlexListData{})
}

// NewWeakFlexXml creates a WeakFlexXml element with no attributes or
// children.
func NewWeakFlexXml(tag string) Value {
	return NewValueRaw(TWeakFlexXml, &WeakFlexXmlData{Tag: tag, Attr: NewOrderedMap()})
}

// AsWeakFlexMap unwraps the weak-map payload; mutators write through it.
func AsWeakFlexMap(v Value) (*WeakFlexMapData, error) {
	if wd, ok := v.Data().(*WeakFlexMapData); ok {
		return wd, nil
	}
	return nil, fmt.Errorf("AsWeakFlexMap: not a weak flex map payload (got %T)", v.Data())
}

// AsWeakFlexList unwraps the weak-list payload.
func AsWeakFlexList(v Value) (*WeakFlexListData, error) {
	if wd, ok := v.Data().(*WeakFlexListData); ok {
		return wd, nil
	}
	return nil, fmt.Errorf("AsWeakFlexList: not a weak flex list payload (got %T)", v.Data())
}

// AsWeakFlexXml unwraps the weak-xml payload.
func AsWeakFlexXml(v Value) (*WeakFlexXmlData, error) {
	if wd, ok := v.Data().(*WeakFlexXmlData); ok {
		return wd, nil
	}
	return nil, fmt.Errorf("AsWeakFlexXml: not a weak flex xml payload (got %T)", v.Data())
}

// ── classification ───────────────────────────────────────────────────

// WeakRefusal is the structured "this value cannot live in a weak
// container" verdict. The word layer renders Kind into the message and
// Note/Help into `= note:` / `= help:` diagnostic lines.
type WeakRefusal struct {
	Kind string // e.g. "an immutable Map"
	Note string // why the value has no coherent weak semantics
	Help string // the actionable fix
}

const weakDomainNote = "weak entries live only while the program keeps " +
	"another handle to the stored value alive; only scalars (stored " +
	"strongly) and mutable handles — flex nodes, Store, Resource and " +
	"class instances (stored weakly) — can live in a weak container"

// classifyWeak sorts a candidate value into a strong slot, a weak
// slot, or a refusal, per the decided domain table.
func classifyWeak(v Value) (weakSlot, *WeakRefusal) {
	if IsNoneShape(v) {
		return weakSlot{}, &WeakRefusal{
			Kind: "none",
			Note: "a stored none is indistinguishable from an absent entry — get returns none for missing keys",
			Help: "omit the key instead of storing none",
		}
	}
	if v.Parent() != nil && v.Parent().ConformsTo(TScalar) && IsConcrete(&v) {
		return weakSlot{strong: v}, nil
	}
	if ref, ok := weakRefFor(v); ok {
		return weakSlot{ref: ref}, nil
	}
	return weakSlot{}, refuseWeak(v)
}

// weakRefFor builds the weak reference for a mutable handle, or
// reports false for values outside the weak domain.
func weakRefFor(v Value) (weakRevive, bool) {
	tmpl := v
	tmpl.SetData(nil)
	switch {
	case IsFlexMap(v):
		mp := v.Data().(MapPayload)
		return weakRefOf[OrderedMap]{ptr: weak.Make(mp.M), wrap: func(p *OrderedMap) Value {
			out := tmpl
			out.SetData(MapPayload{M: p})
			return out
		}}, true
	case IsFlexList(v):
		fd := v.Data().(*FlexListData)
		return weakRefOf[FlexListData]{ptr: weak.Make(fd), wrap: func(p *FlexListData) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	case IsFlexXml(v):
		fd := v.Data().(*FlexXmlData)
		return weakRefOf[FlexXmlData]{ptr: weak.Make(fd), wrap: func(p *FlexXmlData) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	case IsWeakFlexMap(v):
		wd := v.Data().(*WeakFlexMapData)
		return weakRefOf[WeakFlexMapData]{ptr: weak.Make(wd), wrap: func(p *WeakFlexMapData) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	case IsWeakFlexList(v):
		wd := v.Data().(*WeakFlexListData)
		return weakRefOf[WeakFlexListData]{ptr: weak.Make(wd), wrap: func(p *WeakFlexListData) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	case IsWeakFlexXml(v):
		wd := v.Data().(*WeakFlexXmlData)
		return weakRefOf[WeakFlexXmlData]{ptr: weak.Make(wd), wrap: func(p *WeakFlexXmlData) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	case IsStore(v):
		si := v.Data().(*StoreInstanceInfo)
		return weakRefOf[StoreInstanceInfo]{ptr: weak.Make(si), wrap: func(p *StoreInstanceInfo) Value {
			out := tmpl
			out.SetData(p)
			return out
		}}, true
	}
	if oi, ok := v.Data().(ClassInstanceInfo); ok && oi.Fields != nil {
		info := oi
		info.Fields = nil
		return weakRefOf[OrderedMap]{ptr: weak.Make(oi.Fields), wrap: func(p *OrderedMap) Value {
			out := tmpl
			revived := info
			revived.Fields = p
			out.SetData(revived)
			return out
		}}, true
	}
	if ri, ok := v.Data().(ResourceInstanceInfo); ok && ri.Fields != nil {
		info := ri
		info.Fields = nil
		return weakRefOf[OrderedMap]{ptr: weak.Make(ri.Fields), wrap: func(p *OrderedMap) Value {
			out := tmpl
			revived := info
			revived.Fields = p
			out.SetData(revived)
			return out
		}}, true
	}
	return nil, false
}

// refuseWeak names the refused kind and pairs it with the teachable
// explanation the diagnostic renders.
func refuseWeak(v Value) *WeakRefusal {
	flexHelp := "store a mutable handle instead (flex the value), or keep " +
		"it in an ordinary FlexMap/FlexList if it should live until removed"
	switch {
	case IsBareTypeNode(&v):
		return &WeakRefusal{Kind: "a type literal", Note: weakDomainNote,
			Help: "type literals are values — " + flexHelp}
	case v.Parent() != nil && v.Parent().ConformsTo(TMap) && IsConcrete(&v):
		if IsRecordType(v) || IsOptionsType(v) || IsTypedMap(&v) {
			return &WeakRefusal{Kind: "a structural type body", Note: weakDomainNote,
				Help: flexHelp}
		}
		return &WeakRefusal{
			Kind: "an immutable Map",
			Note: "an immutable Map is a value — it has no independent identity " +
				"the collector could track, so a weak entry for it could never " +
				"be kept alive by anything you hold",
			Help: "store a mutable handle: flex the map first (set k/q (flex {…}) w), " +
				"or keep it in an ordinary FlexMap if it should live until removed",
		}
	case v.Parent() != nil && v.Parent().ConformsTo(TList) && IsConcrete(&v):
		return &WeakRefusal{
			Kind: "an immutable List",
			Note: "an immutable List is a value — it has no independent identity " +
				"the collector could track",
			Help: "store a mutable handle: flex the list first (append (flex […]) w)",
		}
	case v.Parent() != nil && v.Parent().ConformsTo(TXml) && IsConcrete(&v):
		return &WeakRefusal{
			Kind: "an immutable Xml element",
			Note: "an immutable Xml element is a value — it has no independent " +
				"identity the collector could track",
			Help: "store a mutable handle: flex the element first",
		}
	case func() bool { _, ok := v.Data().(*FnDefInfo); return ok }():
		return &WeakRefusal{Kind: "a fn value", Note: weakDomainNote,
			Help: "wrap the fn in a flex container to cache it: (flex [f/v])"}
	case func() bool { _, ok := v.Data().(ErrorInfo); return ok }():
		return &WeakRefusal{Kind: "an Error value", Note: weakDomainNote,
			Help: flexHelp}
	}
	leaf := "value"
	if v.Parent() != nil {
		leaf = v.Parent().Leaf() + " value"
	}
	return &WeakRefusal{Kind: "a " + leaf, Note: weakDomainNote, Help: flexHelp}
}

// ── map operations ───────────────────────────────────────────────────

// sweep drops dead slots and their keys in place.
func (d *WeakFlexMapData) sweep() {
	if len(d.keys) == 0 {
		return
	}
	keep := d.keys[:0]
	for _, k := range d.keys {
		s := d.slots[k]
		if s.ref != nil {
			if _, alive := s.ref.revive(); !alive {
				delete(d.slots, k)
				continue
			}
		}
		keep = append(keep, k)
	}
	d.keys = keep
}

// snapshot sweeps, then materializes a STRONG OrderedMap of the
// surviving entries — the one consistent world every read observes.
func (d *WeakFlexMapData) snapshot() *OrderedMap {
	d.sweep()
	om := NewOrderedMap()
	for _, k := range d.keys {
		s := d.slots[k]
		if s.ref == nil {
			om.Set(k, s.strong)
			continue
		}
		if v, alive := s.ref.revive(); alive {
			om.Set(k, v)
		}
	}
	return om
}

// SetValue classifies and stores one entry (position kept for an
// existing key, appended for a new one). A non-nil WeakRefusal means
// the value is outside the weak domain and nothing was stored.
func (d *WeakFlexMapData) SetValue(key string, v Value) *WeakRefusal {
	slot, refusal := classifyWeak(v)
	if refusal != nil {
		return refusal
	}
	d.sweep()
	if d.slots == nil {
		d.slots = map[string]weakSlot{}
	}
	if _, exists := d.slots[key]; !exists {
		d.keys = append(d.keys, key)
	}
	d.slots[key] = slot
	return nil
}

// DeleteKey removes one entry, dropping its slot and its position in the
// insertion order. It is SetValue's inverse and cannot refuse: removal
// has no value to classify against the weak domain. A key that is not
// present is a no-op, so `del` stays idempotent.
func (d *WeakFlexMapData) DeleteKey(key string) {
	d.sweep()
	if _, exists := d.slots[key]; !exists {
		return
	}
	delete(d.slots, key)
	for i, k := range d.keys {
		if k == key {
			d.keys = append(d.keys[:i], d.keys[i+1:]...)
			break
		}
	}
}

// ── list operations ──────────────────────────────────────────────────

// sweep splice-compacts dead slots, preserving survivor order.
func (d *WeakFlexListData) sweep() {
	if len(d.slots) == 0 {
		return
	}
	keep := d.slots[:0]
	for _, s := range d.slots {
		if s.ref != nil {
			if _, alive := s.ref.revive(); !alive {
				continue
			}
		}
		keep = append(keep, s)
	}
	d.slots = keep
}

// snapshotElems sweeps, then materializes the surviving elements as a
// strong slice.
func (d *WeakFlexListData) snapshotElems() []Value {
	d.sweep()
	out := make([]Value, 0, len(d.slots))
	for _, s := range d.slots {
		if s.ref == nil {
			out = append(out, s.strong)
			continue
		}
		if v, alive := s.ref.revive(); alive {
			out = append(out, v)
		}
	}
	return out
}

// Len sweeps and reports the surviving element count.
func (d *WeakFlexListData) Len() int {
	d.sweep()
	return len(d.slots)
}

// SetIndex classifies and replaces the element at i (an index into the
// post-sweep view). Bounds are the caller's job via Len — i must be in
// [0, Len()).
func (d *WeakFlexListData) SetIndex(i int, v Value) *WeakRefusal {
	slot, refusal := classifyWeak(v)
	if refusal != nil {
		return refusal
	}
	d.slots[i] = slot
	return nil
}

// Append classifies and appends one element.
func (d *WeakFlexListData) Append(v Value) *WeakRefusal {
	slot, refusal := classifyWeak(v)
	if refusal != nil {
		return refusal
	}
	d.sweep()
	d.slots = append(d.slots, slot)
	return nil
}

// ── xml operations ───────────────────────────────────────────────────

// snapshotCren sweeps, then materializes the surviving children.
func (d *WeakFlexXmlData) snapshotCren() []Value {
	keep := d.slots[:0]
	out := make([]Value, 0, len(d.slots))
	for _, s := range d.slots {
		if s.ref == nil {
			keep = append(keep, s)
			out = append(out, s.strong)
			continue
		}
		if v, alive := s.ref.revive(); alive {
			keep = append(keep, s)
			out = append(out, v)
		}
	}
	d.slots = keep
	return out
}

// SetAttr writes one attribute (attributes are strings — always
// strong).
func (d *WeakFlexXmlData) SetAttr(key string, v Value) {
	if d.Attr == nil {
		d.Attr = NewOrderedMap()
	}
	d.Attr.Set(key, v)
}

// DeleteAttr removes one attribute. Children are not attributes and are
// unaffected; an absent name is a no-op.
func (d *WeakFlexXmlData) DeleteAttr(key string) {
	if d.Attr == nil {
		return
	}
	d.Attr.Delete(key)
}

// AppendChild classifies and appends one child element.
func (d *WeakFlexXmlData) AppendChild(v Value) *WeakRefusal {
	slot, refusal := classifyWeak(v)
	if refusal != nil {
		return refusal
	}
	d.snapshotCren()
	d.slots = append(d.slots, slot)
	return nil
}

// ── shared construction from a source container ──────────────────────

// PopulateWeakFlexMap fills a fresh weak map from a source ReadMap,
// classifying every entry. Used by `make WeakFlexMap m`.
func PopulateWeakFlexMap(src ReadMap) (Value, *WeakRefusal) {
	out := NewWeakFlexMap()
	wd := out.Data().(*WeakFlexMapData)
	for _, k := range src.Keys() {
		v, _ := src.Get(k)
		if refusal := wd.SetValue(k, v); refusal != nil {
			return Value{}, refusal
		}
	}
	return out, nil
}

// PopulateWeakFlexList fills a fresh weak list from source elements.
// Used by `make WeakFlexList l`.
func PopulateWeakFlexList(src ReadList) (Value, *WeakRefusal) {
	out := NewWeakFlexList()
	wd := out.Data().(*WeakFlexListData)
	for i := 0; i < src.Len(); i++ {
		if refusal := wd.Append(src.Get(i)); refusal != nil {
			return Value{}, refusal
		}
	}
	return out, nil
}

// PopulateWeakFlexXml fills a fresh weak xml element from an Xml
// source: tag and attributes copy strongly, children classify. Used by
// `make WeakFlexXml x`.
func PopulateWeakFlexXml(tag string, attr *OrderedMap, cren []Value) (Value, *WeakRefusal) {
	out := NewWeakFlexXml(tag)
	wd := out.Data().(*WeakFlexXmlData)
	if attr != nil {
		for _, k := range attr.Keys() {
			av, _ := attr.Get(k)
			wd.SetAttr(k, av)
		}
	}
	for _, c := range cren {
		if refusal := wd.AppendChild(c); refusal != nil {
			return Value{}, refusal
		}
	}
	return out, nil
}

// ClassifyWeakRefusal reports the WeakRefusal a value would raise when
// stored in a weak container, or nil for domain members. The word
// layer's check-time mirrors consult it so a statically-known refusal
// is flagged before runtime (design/FLEX-ATTRS.1.md §4.4).
func ClassifyWeakRefusal(v Value) *WeakRefusal {
	_, refusal := classifyWeak(v)
	return refusal
}

// WeakRefusalError renders a WeakRefusal as the rich diagnostic the
// design pins (design/FLEX-ATTRS.1.md §4.4): code weak_value_error,
// the refused kind in the message, the identity explanation as a
// `= note:` line, and the actionable fix as a `= help:` suggestion.
func WeakRefusalError(r *Registry, word, container string, refusal *WeakRefusal) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	e := makeBoruError("weak_value_error",
		fmt.Sprintf("%s: cannot store %s in a %s", word, refusal.Kind, container),
		word, src, "")
	e.Notes = append(e.Notes, refusal.Note)
	e.Suggestions = append(e.Suggestions, DiagSuggestion{Message: refusal.Help})
	return e
}

// weakRefusalError is the kernel-internal spelling of WeakRefusalError.
func weakRefusalError(r *Registry, word, container string, refusal *WeakRefusal) error {
	return WeakRefusalError(r, word, container, refusal)
}

// cloneWeakSlots copies a slot list for `clone`: strong values are
// deep-cloned by the caller's cloner, weak references are shared (the
// clone tracks the same referents).
func cloneWeakSlots(slots []weakSlot, cloneVal func(Value) Value) []weakSlot {
	out := make([]weakSlot, len(slots))
	for i, s := range slots {
		if s.ref == nil {
			out[i] = weakSlot{strong: cloneVal(s.strong)}
			continue
		}
		out[i] = s
	}
	return out
}

// CloneWeakFlexMapData deep-copies the payload for `clone`: strong
// slots clone, weak slots re-box the same referents, dead slots drop.
func CloneWeakFlexMapData(d *WeakFlexMapData, cloneVal func(Value) Value) *WeakFlexMapData {
	d.sweep()
	nd := &WeakFlexMapData{slots: map[string]weakSlot{}}
	nd.keys = append(nd.keys, d.keys...)
	for k, s := range d.slots {
		if s.ref == nil {
			nd.slots[k] = weakSlot{strong: cloneVal(s.strong)}
			continue
		}
		nd.slots[k] = s
	}
	return nd
}

// CloneWeakFlexListData is the list twin of CloneWeakFlexMapData.
func CloneWeakFlexListData(d *WeakFlexListData, cloneVal func(Value) Value) *WeakFlexListData {
	d.sweep()
	return &WeakFlexListData{slots: cloneWeakSlots(d.slots, cloneVal)}
}

// CloneWeakFlexXmlData is the xml twin of CloneWeakFlexMapData.
func CloneWeakFlexXmlData(d *WeakFlexXmlData, cloneVal func(Value) Value, cloneAttr func(*OrderedMap) *OrderedMap) *WeakFlexXmlData {
	d.snapshotCren()
	return &WeakFlexXmlData{
		Tag:   d.Tag,
		Attr:  cloneAttr(d.Attr),
		slots: cloneWeakSlots(d.slots, cloneVal),
	}
}
