package core

import "fmt"

// Value equality — the Equal half of the TypeBehavior trio. Lives here
// (not in unify.go) so the kernel's Equal default sits next to
// defaultBehavior in typebehavior.go rather than buried inside the
// unification module.
//
// ValuesEqual is the public entry; valuesEqualDefault is the path
// defaultBehavior.Equal delegates to and the fall-through for
// per-type Behaviors that override only Format. listsEqual and
// mapsEqual are the structural helpers.

// nodeFamily normalises a flex node type to its immutable family root:
// TFlexMap → TMap, TFlexList → TList, anything else unchanged.
// Flexness is a mutability mode, not part of value identity — equality
// and ordering compare flex nodes by content, exactly like their plain
// counterparts. Deliberately exact (not ConformsTo) so other Node
// subtypes (Inspect, Args) keep their own identity.
func nodeFamily(t *Type) *Type {
	if t == nil {
		return nil
	}
	if t.Equal(TFlexMap) {
		return TMap
	}
	if t.Equal(TFlexList) {
		return TList
	}
	// Weak flex nodes fold like their flex parents: weakness is a
	// lifecycle mode, not value identity (design/FLEX-ATTRS.1.md §4).
	// WeakFlexXml folds to FlexXml (which does not itself fold — the
	// Xml family owns its own equality through its Behavior).
	if t.Equal(TWeakFlexMap) {
		return TMap
	}
	if t.Equal(TWeakFlexList) {
		return TList
	}
	if t.Equal(TWeakFlexXml) {
		return TFlexXml
	}
	return t
}

// containerFamily is nodeFamily for the EQUALITY arms: a USER-minted
// refinement of a container (`def S (refine FlexMap)`, `def M (refine
// Map)`) folds through its nearest kernel ancestor first. A refine is a
// member of the family — dispatch admits it as one and `is` says so — and
// a value is eq to itself whatever its tag's ancestry, so `def w:S (flex
// {a:1})  w eq w` is true, where the exact fold over the minted tag reached
// no container arm and answered false (NUR142). The walk stops at the first
// kernel node, so Inspect and Args keep their identity as before. Ordering
// keeps the exact nodeFamily: a refinement may carry its own Comparer
// (`behave`), which the LCA walk must reach before any fold.
func containerFamily(t *Type) *Type {
	for t != nil && t.Origin == OriginUserDef && t.Parent != nil {
		t = t.Parent
	}
	return nodeFamily(t)
}

// ValuesEqual compares the data payloads of two values with the same type.
//
// Routes through Behavior.Equal for the same-Parent case so types
// with normalisation semantics (CalendarDuration, DepScalar in a future
// step, and plugin types) can supply their own equality. The
// cross-Parent case falls through to the default switch since
// equality across types is a matching-strategy concern, not a
// per-type concern.
func ValuesEqual(a, b Value) bool {
	// Pluggable equality: when both sides share a Parent with a
	// non-default Behavior, delegate. Type literals (Data==nil) are
	// excluded — bare type equality is a lattice-identity check, not
	// a per-type semantic compare.
	if a.Data != nil && b.Data != nil &&
		a.Parent != nil && a.Parent == b.Parent &&
		a.Parent.Behavior() != nil && a.Parent.Behavior() != DefaultBehavior {
		return a.Parent.Behavior().Equal(a, b)
	}
	return valuesEqualDefault(a, b)
}

// valuesEqualDefault is the kernel's default equality path,
// bypassing the Behavior dispatch in ValuesEqual. Used by
// DefaultBehavior.Equal and by Behavior implementations that
// override Format but want fall-through equality without
// triggering infinite re-entry.
func valuesEqualDefault(a, b Value) bool {
	// Two Data==nil values: carriers are abstract (conservatively
	// equal); two type literals are equal iff they are the same
	// lattice identity.
	if a.Data == nil && b.Data == nil {
		if a.Carrier || b.Carrier {
			return true
		}
		return a.Equal(&b)
	}
	// One is a type literal and the other is a concrete value — not equal.
	if a.Data == nil || b.Data == nil {
		return false
	}
	// Bounded Type (`Type of [B]` / `B/t`): equal iff the bounds are
	// the same lattice identity — structural identity, no minting, so
	// `Map/t eq (Type of [Map])` holds however each side was built.
	if IsBoundedType(a) || IsBoundedType(b) {
		an, aerr := AsBoundedType(a)
		bn, berr := AsBoundedType(b)
		if aerr != nil || berr != nil {
			return false
		}
		return an.Equal(bn)
	}
	// Dependent scalar: route to payload comparison BEFORE the
	// ConformsTo(TString)/ConformsTo(TInteger)/... dispatch below. The
	// lattice override makes DepString.ConformsTo(TString)=true, so
	// without this branch a DepScalar would fall into AsString and
	// silently compare zero-value payloads.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		ai, err := a.AsDepScalar()
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return false
		}
		bi, err := b.AsDepScalar()
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return false
		}
		return depScalarsEqual(ai, bi)
	}
	// Micron instances (Emailon / Urlon / user kinds) and Pathons:
	// content equality, as a backstop for values whose minted node
	// carries a wrapper Behavior that delegates Equal here. Without
	// these branches the default %v-format arm would compare the
	// OrderedMap POINTER, making equal-content Microns unequal.
	if am, aok := a.Data.(MicronPayload); aok {
		bm, bok := b.Data.(MicronPayload)
		return bok && mapsEqual(am.Fields, bm.Fields)
	}
	if _, bok := b.Data.(MicronPayload); bok {
		return false
	}
	if ap, aok := a.Data.(PathonPayload); aok {
		bp, bok := b.Data.(PathonPayload)
		return bok && PathonContentEqual(ap.Info, bp.Info)
	}
	if _, bok := b.Data.(PathonPayload); bok {
		return false
	}
	switch {
	case a.Parent.ConformsTo(TString):
		as, _ := AsString(a)
		bs, _ := AsString(b)
		return as == bs
	case a.Parent.ConformsTo(TInteger):
		ai, _ := AsInteger(a)
		bi, _ := AsInteger(b)
		return ai == bi
	case a.Parent.ConformsTo(TBoolean):
		ab, _ := AsBoolean(a)
		bb, _ := AsBoolean(b)
		return ab == bb
	case a.Parent.ConformsTo(TAtom):
		// Atom identity is by name only — the AtomPayload.Referent snapshot
		// is metadata and must not affect equality (otherwise the default
		// %v-format branch below would compare the referent pointer too).
		an, _ := AsAtom(a)
		bn, _ := AsAtom(b)
		return an == bn
	case nodeFamily(a.Parent).Equal(TList):
		aTT, aTbl := a.Data.(TableTypeInfo)
		bTT, bTbl := b.Data.(TableTypeInfo)
		if aTbl && bTbl {
			return mapsEqual(aTT.Record.Fields, bTT.Record.Fields)
		}
		if aTbl != bTbl {
			return false
		}
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		aLSS, aLShaped := a.Data.(*StoreShapeInfo)
		bLSS, bLShaped := b.Data.(*StoreShapeInfo)
		if aLShaped || bLShaped {
			// Same aliasing model as the map branch below: one minted
			// shape per container, pointer identity is value identity.
			return aLShaped && bLShaped && aLSS == bLSS
		}
		aLst, aLstErr := AsList(a)
		bLst, bLstErr := AsList(b)
		if aLstErr != nil || bLstErr != nil {
			// Not list-readable (abstract or foreign payload on a
			// list-family value): rendering compare, never a nil walk.
			return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
		}
		return listsEqual(aLst.Slice(), bLst.Slice())
	case nodeFamily(a.Parent).Equal(TMap):
		aRT, aRec := a.Data.(RecordTypeInfo)
		bRT, bRec := b.Data.(RecordTypeInfo)
		if aRec && bRec {
			return mapsEqual(aRT.Fields, bRT.Fields)
		}
		if aRec != bRec {
			return false
		}
		aOT, aOpt := a.Data.(OptionsTypeInfo)
		bOT, bOpt := b.Data.(OptionsTypeInfo)
		if aOpt && bOpt {
			return mapsEqual(aOT.Fields, bOT.Fields)
		}
		if aOpt != bOpt {
			return false
		}
		aCT, aOk := a.Data.(ChildTypeInfo)
		bCT, bOk := b.Data.(ChildTypeInfo)
		if aOk && bOk {
			return aCT.Child.Parent.Equal(bCT.Child.Parent) && ValuesEqual(aCT.Child, bCT.Child)
		}
		if aOk != bOk {
			return false
		}
		aSS, aShaped := a.Data.(*StoreShapeInfo)
		bSS, bShaped := b.Data.(*StoreShapeInfo)
		if aShaped || bShaped {
			// Check-mode store-shaped carriers: ONE shape is minted per
			// container and every carrier copy aliases it (store_shape.go),
			// so the pointer IS the identity — equal iff the same shape.
			return aShaped && bShaped && aSS == bSS
		}
		aMap, aMapErr := AsMap(a)
		bMap, bMapErr := AsMap(b)
		if aMapErr != nil || bMapErr != nil {
			// A map-family value whose payload is not map-readable (an
			// abstract check-mode payload, a foreign marker): fall through
			// to the rendering compare rather than walking a nil ReadMap.
			return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
		}
		return mapsEqual(aMap, bMap)
	default:
		return fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)
	}
}

// listsEqual compares two list payloads element by element.
func listsEqual(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !nodeFamily(a[i].Parent).Equal(nodeFamily(b[i].Parent)) || !ValuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// mapsEqual compares two map payloads by keys and values.
func mapsEqual(a, b ReadMap) bool {
	if a.Len() != b.Len() {
		return false
	}
	for _, k := range a.Keys() {
		aVal, _ := a.Get(k)
		bVal, ok := b.Get(k)
		if !ok {
			return false
		}
		if !nodeFamily(aVal.Parent).Equal(nodeFamily(bVal.Parent)) || !ValuesEqual(aVal, bVal) {
			return false
		}
	}
	return true
}
