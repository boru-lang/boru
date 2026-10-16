package core

import (
	"sort"
	"strings"
)

// compareTypes is a total order on *Type — the tiebreaker CompareValues
// applies once two values share a unified Rank, and the order under
// which type literals sort. Two distinct types never compare equal.
// The keys, in priority order:
//
//  1. Rank — the unified lattice rank (typetable.go::builtinDecls). A
//     builtin child always ranks above its parent; user and external
//     types inherit the parent's Rank, so a builtin and all the user
//     types descending from it share one Rank and fall to key 2.
//  2. depth — the shallower type first: the more general type before a
//     subtype that refines it (List before a `def Foo refine List`).
//  3. name — lexical. Separates sibling types that share Rank and
//     depth — Foo and Bar, both `def … refine List`.
//  4. id — the lattice identity string; the last-ditch floor for the
//     rare pair of distinct types that share a name (a shadowed def).
func compareTypes(a, b *Type) int {
	if a == b {
		return 0
	}
	if c := cmpInt(rankOf(a), rankOf(b)); c != 0 {
		return c
	}
	if c := cmpInt(typeDepth(a), typeDepth(b)); c != 0 {
		return c
	}
	if c := strings.Compare(a.Name(), b.Name()); c != 0 {
		return c
	}
	return strings.Compare(IDOf(a), IDOf(b))
}

// rankOf returns t's unified lattice Rank. Builtins, MintType, and
// RegisterType all set Rank at creation, so this normally
// returns t.Rank directly; the parent-chain walk is a fallback for a
// *Type assembled without one (chiefly in tests).
func rankOf(t *Type) int {
	for ; t != nil; t = t.Parent() {
		if t.Rank() != 0 {
			return t.Rank()
		}
	}
	return 0
}

// typeDepth is the length of t's parent chain — its distance from the
// lattice root. A subtype is one deeper than the type it refines. The
// value is cached in Type.Depth at construction (builtins, MintType,
// RegisterType), so this is an O(1) field read; the walk is a
// fallback for an ad-hoc *Type assembled without a Depth (chiefly tests).
func typeDepth(t *Type) int {
	if t != nil && t.Depth() > 0 {
		return t.Depth()
	}
	d := 0
	for ; t != nil; t = t.Parent() {
		d++
	}
	return d
}

// cmpInt returns -1, 0, or 1 for the ordering of two ints.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// compareStructural breaks a tie between two values that share a Parent
// and a size. A List-like value compares element by element, a
// Map-like value compares its sorted keys and then the value at each
// key — both recursing through CompareValues. Any other value is an
// opaque type with no structure to descend, and falls to a last-resort
// comparison of the canonical (round-trippable) rendering — the one
// deliberate use of a rendered form as an ordering key beyond String,
// Word, and Atom.
func compareStructural(a, b Value) (int, error) {
	if IsConcrete(&a) && IsConcrete(&b) {
		if a.Parent().ConformsTo(TList) {
			return compareListElems(a, b)
		}
		if a.Parent().ConformsTo(TMap) {
			return compareMapEntries(a, b)
		}
	}
	return strings.Compare(CanonValue(a), CanonValue(b)), nil
}

// compareListView surfaces the element view structural comparison
// orders by: the mutable payload when present, else the swept strong
// snapshot of a weak flex list. cmp is CONTENT ordering, mode-blind —
// a weak container orders exactly like its plain counterpart
// (design/FLEX-ATTRS.1.md §4.6), so the weak payload must not fall to
// the canon rendering (whose `(make WeakFlex…)` prefix would order it
// before every plain sibling).
func compareListView(v Value) ([]Value, bool) {
	if ae, err := AsMutableList(v); err == nil {
		return ae, true
	}
	if IsWeakFlexList(v) {
		if lst, err := AsList(v); err == nil {
			return lst.Slice(), true
		}
	}
	return nil, false
}

// compareMapView is the map twin of compareListView.
func compareMapView(v Value) (ReadMap, bool) {
	if m, err := AsMutableMap(v); err == nil {
		return m, true
	}
	if IsWeakFlexMap(v) {
		if m, err := AsMap(v); err == nil {
			return m, true
		}
	}
	return nil, false
}

// compareListElems orders two list-like values element by element.
func compareListElems(a, b Value) (int, error) {
	ae, aok := compareListView(a)
	be, bok := compareListView(b)
	if !aok || !bok {
		// Not a plain element list (a typed list, a table, …) — fall
		// back to the canonical rendering.
		return strings.Compare(CanonValue(a), CanonValue(b)), nil
	}
	if len(ae) != len(be) {
		return cmpInt(len(ae), len(be)), nil
	}
	for i := range ae {
		c, err := CompareValues(ae[i], be[i])
		if err != nil {
			return 0, err
		}
		if c != 0 {
			return c, nil
		}
	}
	return 0, nil
}

// compareMapEntries orders two map-like values by their sorted key
// lists, then by the value stored at each shared key.
func compareMapEntries(a, b Value) (int, error) {
	am, aok := compareMapView(a)
	bm, bok := compareMapView(b)
	if !aok || !bok {
		return strings.Compare(CanonValue(a), CanonValue(b)), nil
	}
	ak := append([]string(nil), am.Keys()...)
	bk := append([]string(nil), bm.Keys()...)
	if len(ak) != len(bk) {
		return cmpInt(len(ak), len(bk)), nil
	}
	sort.Strings(ak)
	sort.Strings(bk)
	for i := range ak {
		if c := strings.Compare(ak[i], bk[i]); c != 0 {
			return c, nil
		}
	}
	// Same key set — compare the value at each key, in key order.
	for _, k := range ak {
		av, _ := am.Get(k)
		bv, _ := bm.Get(k)
		c, err := CompareValues(av, bv)
		if err != nil {
			return 0, err
		}
		if c != 0 {
			return c, nil
		}
	}
	return 0, nil
}
