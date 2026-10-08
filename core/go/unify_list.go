package core

import "fmt"

// unifyListFamily owns unification when either side is in the List
// family (List, TypedList, Table) or is a bare List type literal.
//
// Canonicalization: if exactly one side is a type literal, normalize so
// `lit` is the type literal and `other` is the concrete side. If both
// sides are in the family, sort by shape rank so the more-general side
// comes first. This collapses the mirrored "aTyped vs concrete" and
// "concrete vs bTyped" arms in the prior implementation into one path.
//
// r is the enclosing chain's registry (nil when unarmed), handed to
// every element / child recursion.
func unifyListFamily(a Value, sa ValueShape, b Value, sb ValueShape, r *Registry) (Value, *UnifyError) {
	// Bare FlexList type literal: nominal-subtype rule — unifies only
	// with a concrete FlexList (or another FlexList literal). A plain
	// list is NOT a FlexList; the supertype literal `List` accepts
	// flex values below via the ordinary family rule.
	// WeakFlexList literal first (deeper node — more specific than the
	// FlexList arm below, which also accepts weak values by subtree).
	if res, handled, err := unifyFlexLiteral(a, sa, b, sb, TWeakFlexList, IsWeakFlexList, "WeakFlexList"); handled {
		return res, err
	}
	if res, handled, err := unifyFlexLiteral(a, sa, b, sb, TFlexList, IsFlexList, "FlexList"); handled {
		return res, err
	}

	// A flex-family carrier (check-mode `(flex […])` residual) vs a typed list
	// [:T]: gradual accept, element-tagged. (See unifyCarrierVsTyped.)
	if res, handled := unifyCarrierVsTyped(a, sa, b, sb, ShapeTypedList, TFlexList); handled {
		return res, nil
	}

	// If one side is the bare List type literal (`List`), it unifies
	// with any List-family value except a table.
	aLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TList)
	bLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TList)
	if aLit {
		if sb == ShapeTable {
			return Value{}, unifyFail("List type literal does not unify with Table", a, b)
		}
		if IsListShape(sb) || bLit {
			return b, nil
		}
		return Value{}, unifyFail("List type literal needs a list-family right-hand side", a, b)
	}
	if bLit {
		if sa == ShapeTable {
			return Value{}, unifyFail("List type literal does not unify with Table", a, b)
		}
		if IsListShape(sa) {
			return a, nil
		}
		return Value{}, unifyFail("List type literal needs a list-family left-hand side", a, b)
	}

	// At this point both sides must be in the List family for
	// unification to succeed.
	if !IsListShape(sa) || !IsListShape(sb) {
		return Value{}, unifyFail("list family requires list-shaped values on both sides", a, b)
	}

	// Table is exclusive — only unifies with another table.
	if sa == ShapeTable || sb == ShapeTable {
		if sa != sb {
			return Value{}, unifyFail("Table only unifies with Table", a, b)
		}
		aTT, _ := AsTableType(a)
		bTT, _ := AsTableType(b)
		unified, err := unifyRecordTypes(aTT.Record, bTT.Record, r)
		if err != nil {
			return Value{}, err
		}
		uRec, _ := AsRecordType(unified)
		return NewTableType(uRec), nil
	}

	// Both typed lists → unify child types.
	if sa == ShapeTypedList && sb == ShapeTypedList {
		aCT, _ := AsChildType(a)
		bCT, _ := AsChildType(b)
		unified, err := unifyInner(aCT.Child, bCT.Child, r)
		if err != nil {
			return Value{}, err.withPath("child")
		}
		return NewTypedList(unified), nil
	}

	// One side typed, the other concrete → each element must unify
	// with the child type. Canonicalize: typed on left.
	if sa == ShapeTypedList || sb == ShapeTypedList {
		var typed, concrete Value
		if sa == ShapeTypedList {
			typed, concrete = a, b
		} else {
			typed, concrete = b, a
		}
		ct, _ := AsChildType(typed)
		return unifyTypedListWithConcrete(concrete, ct.Child, r)
	}

	// Both concrete lists → element-by-element.
	aLst, _ := AsList(a)
	bLst, _ := AsList(b)
	aElems := aLst.Slice()
	bElems := bLst.Slice()
	if len(aElems) != len(bElems) {
		return Value{}, unifyFail(
			fmt.Sprintf("list length mismatch: %d vs %d", len(aElems), len(bElems)), a, b)
	}
	result, err := unifyZip(len(aElems), sliceAt(aElems), sliceAt(bElems), r)
	if err != nil {
		return Value{}, err
	}
	return NewList(result), nil
}

// unifyTypedListWithConcrete unifies a child type constraint against each
// element of the concrete list side. The result RETAINS childType as its
// element tag (Value.elem) and PRESERVES mutability: a FlexList stays a
// FlexList (flex only toggles mutability, never strips the element-type
// contract), a plain List stays plain. A CARRIER (check-mode abstract list,
// IsConcrete false) has no readable elements — unify gradually to its own
// kind, tagged, rather than dereferencing nil (the flex-[:T] panic twin).
// See design/TYPED-CONTAINER-TAG-RETENTION.0.md.
// reparentSwappedElem applies the Typed-Def Reparent discipline
// (eng/go/CLAUDE.md) to a container-child unify result: Unify of a
// concrete element against a bare-refine child returns the TYPE
// LITERAL (the documented Unify-swap), and binding that literal would
// replace the element's data with the type name. Return the source
// element reparented to the canonical minted type instead — the same
// construction `def x:Foo 42` performs at scalar typed defs.
// Non-swap results pass through untouched. r is the enclosing chain's
// registry: when armed, the minted type is resolved to its canonical
// node (CanonicalType) before the reparent, so a retagged element
// carries the registry's own *Type pointer.
func reparentSwappedElem(src, unified Value, r *Registry) Value {
	if !IsBareTypeNode(unified) || !IsConcrete(src) {
		return unified
	}
	t := ValueType(unified)
	if r != nil {
		t = CanonicalType(r, t)
	}
	return ReparentValue(src, t)
}

func unifyTypedListWithConcrete(concrete, childType Value, r *Registry) (Value, *UnifyError) {
	if !IsConcrete(concrete) {
		// A carrier (a check-mode abstract list with no readable elements) tags
		// gradually; its concrete elements are validated at runtime.
		out := concrete
		out.SetElemConstraint(childType)
		return out, nil
	}
	lst, _ := AsList(concrete) // concrete list-family (plain or flex) → readable
	elems := lst.Slice()
	result, err := unifyZip(lst.Len(), constAt(childType), sliceAt(elems), r)
	if err != nil {
		return Value{}, err
	}
	for i := range result {
		result[i] = reparentSwappedElem(elems[i], result[i], r)
	}
	var out Value
	if IsFlexList(concrete) {
		out = NewFlexList(result)
	} else {
		out = NewList(result)
	}
	out.SetElemConstraint(childType)
	return out, nil
}
