package core

import (
	"errors"
	"fmt"
	"math"
	"reflect"
)

// ErrNoComparer is returned by Comparer.Compare implementations that
// hold a placeholder slot (e.g. a wrapped Behavior whose user-defined
// comparator body is empty). CompareValues recognises it and continues
// the parent-chain walk, treating the Behavior as if it didn't satisfy
// the Comparer interface at all. This lets a single Behavior wrapper
// carry multiple optional capabilities (compare / canon / …) where
// only some are installed without prematurely terminating dispatch.
var ErrNoComparer = errors.New("eng: no comparer in this Behavior")

// CompareValues returns -1, 0, or 1 for natural ordering of two values.
//
// Dispatch is type-driven: the compare logic for a pair of values lives
// on the type's Behavior (Comparer capability), not in a switch ladder
// here. The dispatch routes through the lowest common ancestor of the
// two operand VTypes — e.g. Integer-vs-Float walks up to Number,
// which owns the numeric ordering; Integer-vs-String walks up to
// Scalar, which owns the cross-branch ordering; Date-vs-Date stays
// on Date.
//
// When the lattice walk finds no Comparer, the order falls to the
// unified lattice Rank: every type carries one Rank giving a total
// order over the whole lattice (see typetable.go::builtinDecls), so
// any two values of different types order by Rank alone. Two values
// of equal Rank — the identical type, or two user/external types that
// inherit one builtin's Rank — break to compareTypes (name/id), then
// to size, then to an element-wise structural comparison. Distinct
// values never collapse to 0.
//
// DepScalar values (type-level constraints) flow through this same
// path.
func CompareValues(a, b Value) (int, error) {
	n, _, err := compareValuesClassified(a, b)
	return n, err
}

// compareValuesClassified is CompareValues plus a report of HOW the
// order was decided. viaFamily is true when the result came from a
// same-family Comparer (a Comparer on a lattice node other than the
// cross-family catch-all TScalar) OR the two operands are the exact same
// type. It is false when the pair only orders via the cross-family
// scalar catch-all (scalarCompareBehavior@TScalar) or the lattice Rank
// fallback — i.e. values of unrelated families.
//
// The restricted ordering words (cmp / lt / lte / gt / gte) accept a
// pair only when viaFamily is true; tcmp and every internal caller use
// CompareValues, which keeps the full total order over all values.
func compareValuesClassified(a, b Value) (n int, viaFamily bool, err error) {
	// A bare type literal IS its lattice node; its type-for-comparison
	// is itself, not its Parent (now the supertype).
	aType := ValueType(a)
	bType := ValueType(b)
	if aType == nil || bType == nil {
		return 0, false, fmt.Errorf("cannot compare values with nil type")
	}
	// Concrete flex nodes order by content exactly like their immutable
	// family (flexness is a mutability mode, not value identity), so a
	// deq-equal flex/plain pair also cmp-equals. Bare type literals are
	// NOT normalised — `FlexList cmp List` stays a Rank ordering.
	if IsConcrete(a) {
		aType = nodeFamily(aType)
	}
	if IsConcrete(b) {
		bType = nodeFamily(bType)
	}
	sameType := aType.Equal(bType)
	for t := lowestCommonAncestor(aType, bType); t != nil; t = t.Parent {
		cmp, ok := t.Behavior().(Comparer)
		if !ok {
			continue
		}
		res, e := cmp.Compare(a, b)
		if errors.Is(e, ErrNoComparer) {
			// Wrapper Behavior signalled "I satisfy Comparer
			// structurally but have no body installed" (DepScalar
			// opt-out; the Time comparer declining instant-vs-duration;
			// …) — keep walking the parent chain.
			continue
		}
		// Resolved by a Comparer. It counts as a same-family resolution
		// unless it is the cross-family catch-all on TScalar applied to
		// two different types (Integer-vs-String, Boolean-vs-Number).
		return res, sameType || !t.Equal(TScalar), e
	}
	// No Comparer in the shared lattice — order by the type lattice.
	// compareTypes is a total order on *Type (Rank, then depth, name,
	// id), so any two values of distinct types resolve here. Reached
	// only for cross-branch pairs, so this is never a family resolution.
	if c := compareTypes(aType, bType); c != 0 {
		return c, false, nil
	}
	// Identical type — order by value: size, then element-wise
	// structure, so distinct values never collapse to 0.
	if sa, sb := SizeOf(a), SizeOf(b); sa != sb {
		return cmpInt(sa, sb), sameType, nil
	}
	res, e := compareStructural(a, b)
	return res, sameType, e
}

// orderedCompare runs the family-restricted comparison behind the
// ordering words. It returns the -1/0/1 result, or an [boru/incomparable]
// error when the pair only orders via the cross-type total order — the
// caller should reach for tcmp instead.
func orderedCompare(op string, a, b Value) (int, error) {
	n, viaFamily, err := compareValuesClassified(a, b)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	if !viaFamily {
		return 0, incomparableError(op, a, b)
	}
	return n, nil
}

// incomparableError is the [boru/incomparable] error the restricted
// ordering words raise for cross-family operands.
func incomparableError(op string, a, b Value) error {
	return &BoruError{
		Code: "incomparable",
		Detail: fmt.Sprintf("%s: cannot order %s and %s",
			op, ValueType(a).String(), ValueType(b).String()),
		Hint: "different types with no shared ordering; use tcmp for a cross-type total order",
	}
}

// lowestCommonAncestor returns the closest type that is an ancestor
// of both a and b on the parent chain. Returns nil only if a and b
// share no common ancestor (the type tables guarantee a single root,
// so in practice this returns at worst the root type).
func lowestCommonAncestor(a, b *Type) *Type {
	// Depth-aligned walk: lift the deeper node to its sibling's depth, then
	// climb both in lockstep until the chains meet — O(d) instead of the old
	// O(d_a · d_b) chain-vs-chain scan. typeDepth reads the cached Type.Depth
	// (O(1) for registered types), so the alignment is O(1) too. Compared via
	// Type.Equal (pointer OR canonical ID): a type literal is a by-value copy
	// of its node, so ValueType() may hand us a copy whose address differs
	// from the canonical node's, while ID-less ad-hoc types still match by
	// pointer.
	if a == nil || b == nil {
		return nil
	}
	da, db := typeDepth(a), typeDepth(b)
	for da > db {
		a = a.Parent
		da--
	}
	for db > da {
		b = b.Parent
		db--
	}
	for a != nil && b != nil {
		if a.Equal(b) {
			return a
		}
		a = a.Parent
		b = b.Parent
	}
	return nil
}

// ExactEqual returns true if two values are exactly equal.
// For scalars (integer, string, boolean, atom, none): compares by value.
// For types: compares structurally via ValuesEqual.
// For non-scalars (list, map): compares by identity (same container).
// scalarSemanticEqual is the scalar-leaf equality dispatch shared by
// ExactEqual (eq) and DeepEqual (deq): a scalar is identified by
// content, not container identity, so the two words must agree on
// every scalar pair. handled=false means the pair is not a
// scalar-semantic pair and the caller's own dispatch continues.
//
// Two clauses:
//
//  1. Same-Parent scalar leaves with a custom Behavior (Bytes, the
//     Time family, plugin scalars) compare by Behavior.Equal. The
//     same-Parent gate keeps this to matching leaf types, and
//     ConformsTo(TScalar) keeps reference-backed non-scalars
//     (List/Map/Object/Tensor/…) on the callers' identity /
//     structural paths. The dispatch is the one ValuesEqual already
//     uses.
//
//  2. Cross-node, NESTED scalar subtype: a `refine`-newtype value
//     (def B (refine Bytes); def x:B …) carries the minted node, not
//     the base, so the same-Parent gate misses `x eq <plain Bytes>`.
//     When one operand's type is an ancestor of the other's — i.e.
//     their lowest common ancestor IS one of the two parents — they
//     share a base scalar leaf, so compare by that base's Comparer
//     (a newtype's wrapper Behavior delegates Compare to the base).
//     eq/deq then agree with cmp on a tagged subtype, as the ordering
//     LCA walk in CompareValues already does. The NESTED guard is
//     load-bearing: it admits newtype-vs-base but EXCLUDES sibling
//     leaves (Date vs DateTime — LCA Time, neither parent — which
//     timeCompareBehavior would otherwise rank as chronologically
//     equal), keeping those on the callers' identity paths,
//     unchanged.
//
// scalarFamilyEqual is the ONE scalar-leaf equality both ExactEqual (eq)
// and DeepEqual (deq) dispatch through: the four by-value families
// (Number/String/Boolean/Atom) followed by the semantic-equality walk for
// every other scalar type (Bytes, the Time family, …). handled=false
// means a and b are not a scalar-leaf pair and the caller proceeds to its
// own identity (eq) or structural (deq) clauses. Callers must dispatch
// None and DepScalar payloads BEFORE calling (DepScalar would otherwise
// route into AsNumber and silently compare zero values).
//
// Sharing one implementation is what keeps eq and deq from drifting on a
// leaf: the exact-int64 arm below was originally added to eq only, so two
// distinct large Integers (> 2^53) compared deq-equal through the float64
// projection until the copies were unified.
func scalarFamilyEqual(a, b Value) (equal, handled bool) {
	if a.Parent.ConformsTo(TNumber) && b.Parent.ConformsTo(TNumber) {
		// NUR032: a bare numeric type literal (`Integer`, `Float`) is not
		// the value 0. It reaches this branch only because the numeric
		// lattice has an extra level — `Integer`'s parent is `Number`,
		// which still conforms to TNumber, where every other family's
		// literal parent is `Scalar` (so `"" eq String` is already
		// false). Without this guard the error-ignoring AsNumber below
		// projects the literal to 0, so `0 eq Integer` and `Integer deq
		// Float` wrongly report equal. Two numeric literals are equal
		// iff they name the same type (keeping `Integer eq Integer`,
		// matching `eq`'s type-body arm); a literal never equals a
		// concrete number.
		if aLit, bLit := IsBareTypeNode(a), IsBareTypeNode(b); aLit || bLit {
			return aLit && bLit && ValueType(a).Equal(ValueType(b)), true
		}
		// An arbitrary-precision operand is compared exactly via apd
		// (cross-leaf magnitude: 1 == 0d1 == 1.0). A NaN/non-finite Float
		// fails toRatExact, so it is never equal — preserving nan≠nan.
		if numIsBig(a) || numIsBig(b) {
			ar, aok := toRatExact(a)
			br, bok := toRatExact(b)
			return aok && bok && ar.Cmp(br) == 0, true
		}
		// Two int64-backed Integers compare EXACTLY as int64; the float64
		// projection below rounds magnitudes above 2^53 and would report
		// distinct large integers as equal. A Float on either side keeps
		// the float comparison (cross-leaf magnitude: 1 == 1.0).
		if a.Parent.ConformsTo(TInteger) && b.Parent.ConformsTo(TInteger) {
			ai, _ := AsInteger(a)
			bi, _ := AsInteger(b)
			return ai == bi, true
		}
		af, _ := AsNumber(a)
		bf, _ := AsNumber(b)
		return af == bf, true
	}
	if a.Parent.ConformsTo(TString) && b.Parent.ConformsTo(TString) {
		as, _ := AsString(a)
		bs, _ := AsString(b)
		return as == bs, true
	}
	if a.Parent.ConformsTo(TBoolean) && b.Parent.ConformsTo(TBoolean) {
		ab, _ := AsBoolean(a)
		bb, _ := AsBoolean(b)
		return ab == bb, true
	}
	if a.Parent.ConformsTo(TAtom) && b.Parent.ConformsTo(TAtom) {
		aa, _ := AsAtom(a)
		ba, _ := AsAtom(b)
		return aa == ba, true
	}
	return scalarSemanticEqual(a, b)
}

// Callers must have dispatched the by-value scalar families
// (Number/String/Boolean/Atom) and DepScalar payloads before calling,
// so those never reach here.
func scalarSemanticEqual(a, b Value) (equal, handled bool) {
	if a.Parent == b.Parent && a.Parent != nil && a.Parent.ConformsTo(TScalar) &&
		a.Parent.Behavior() != nil && a.Parent.Behavior() != DefaultBehavior {
		return a.Parent.Behavior().Equal(a, b), true
	}
	if a.Parent != b.Parent {
		if lca := lowestCommonAncestor(a.Parent, b.Parent); lca != nil &&
			!lca.Equal(TScalar) && lca.ConformsTo(TScalar) &&
			(lca.Equal(a.Parent) || lca.Equal(b.Parent)) {
			if cmp, ok := lca.Behavior().(Comparer); ok {
				if res, e := cmp.Compare(a, b); e == nil {
					return res == 0, true
				}
			}
		}
	}
	return false, false
}

func ExactEqual(a, b Value) bool {
	// none == none
	if ValueType(a).Equal(TNone) && ValueType(b).Equal(TNone) {
		return true
	}

	// DepScalar pre-empts the ConformsTo(TNumber)/ConformsTo(TString)/...
	// dispatch below: the lattice override would otherwise route
	// DepInteger payloads into AsNumber and silently compare zero
	// values. Two DepScalars are equal iff their constraint shapes
	// match (delegated through ValuesEqual).
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Function values (NUR031). eq is REFERENCE identity, per NUR011's rule
	// for every Ideal — and the reference is the payload's identity token
	// (FnDefInfo.ident), minted once per authored function and copied by
	// every derivation that leaves it the same function. Two bindings of one
	// function share it; two independently-written functions never do.
	//
	// This is the record's "box FnDefInfo", narrowed to the one field that
	// needed boxing: a full box would change the representation every kernel
	// arm keying on `FnDefInfo` reads, for an identity a single unexported
	// pointer supplies. Before this, eq fell to the type-body arm below,
	// which compares the payload STRUCT — including Name — so `def a (f/v)`
	// and `def b (f/v)` were not eq though they name one function.
	// A compiled closure (NUR124's payload axis) is a function too: it
	// carries the identity token its push minted (ClosurePayload.Ident), and
	// the FnDefInfo the VM bridges it to for the interpreter's dispatch
	// carries the same one — so two copies of one closure, bridged or not,
	// are one function, as the source lambda's copies are on the
	// interpreter. A payload with no token (a hand-built value) has no
	// identity and is eq to nothing.
	if eq, handled := closureIdentityEqual(a, b); handled {
		return eq
	}
	if af, aok := a.Data.(FnDefInfo); aok {
		if bf, bok := b.Data.(FnDefInfo); bok {
			return sameFnIdentity(af, bf)
		}
		return false
	}

	// Types: structural comparison.
	if IsTypeBody(a) && IsTypeBody(b) {
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Scalars: compare by value — one shared implementation with
	// DeepEqual (scalarFamilyEqual) so eq and deq can never drift on a
	// scalar leaf.
	if equal, handled := scalarFamilyEqual(a, b); handled {
		return equal
	}

	// Non-scalars: identity comparison — both values must refer to the
	// same underlying container. Flexness is part of identity here:
	// nodeFamily-normalised comparison would still fail on container
	// identity (a flex copy is a fresh container), but normalising the
	// family keeps the dispatch uniform for flex-vs-flex pairs.
	if nodeFamily(a.Parent).Equal(TList) && nodeFamily(b.Parent).Equal(TList) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	if nodeFamily(a.Parent).Equal(TMap) && nodeFamily(b.Parent).Equal(TMap) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	// XML elements: identity like Map/List — `eq` is container identity
	// (a flex copy / a fresh literal is not eq to its source), while `deq`
	// (DeepEqual) is structural. See design/XML-LITERAL.0.md §5.
	if IsXmlValue(a) && IsXmlValue(b) {
		return a.Parent.Equal(b.Parent) && sameContainer(a.Data, b.Data)
	}
	// Flat instances (class + Resource/Entity) identify by their
	// underlying field map — the same aliasing rule as Map: two bindings
	// to one instance are eq, two structurally-equal instances are not
	// (that's deq). A shared Fields pointer is the identity key; a
	// class/resource cross-pair can never share one, so it stays unequal.
	if af, _, aok := FlatInstanceParts(a); aok {
		bf, _, bok := FlatInstanceParts(b)
		return bok && af != nil && af == bf
	}

	// Opaque Ideal handles (NUR031). eq is the REFERENCE half of the
	// two-equalities rule: a pointer-backed handle (Store, Timeout,
	// Interval, the Module descriptor) is eq iff it is the SAME handle.
	// Error is a value-like Ideal with no reference, so eq compares its
	// fields (eq ≡ deq there, like a scalar leaf). Before this rule every
	// handle fell through to the terminal `false` — not even eq to
	// itself. The code values (Function, Word) stay below: they carry no
	// stable reference these value-copied payloads could compare (NUR031
	// keeps them as an argued remainder).
	if eq, handled := opaqueIdealExactEqual(a, b); handled {
		return eq
	}

	return false
}

// sameContainer reports whether two non-scalar payloads refer to the
// same underlying container — the identity test behind ExactEqual for
// lists and maps. A MapPayload identifies by its *OrderedMap pointer;
// a ListPayload by the backing array of its element slice, so a value
// dup'd from a list is identical to its source while two separate
// literals are not. Payloads with no aliasable identity (table data,
// materializers, …) are never equal here.
//
// It must not apply `==` to a Payload directly: ListPayload holds a
// slice and is therefore not a comparable type — a bare
// `a.Data == b.Data` panics at runtime.
// HasContainerIdentity reports whether v's equality is CONTAINER IDENTITY
// rather than structure — i.e. whether sameContainer below, not
// scalarFamilyEqual, decides `v eq w`. The arms mirror sameContainer's
// exactly and must be kept in step with it.
//
// The caller this exists for is the CHECK pass: check mode's values are
// copies by construction (a container read hands back a CloneValue so the
// emitter's operand-provenance tracking gets a fresh ID), so it can never
// model runtime container identity, and any static claim it makes about
// one is unfounded. A concrete-operand const-fold over such a value
// computed `false` for `(m get 'a') eq (m get 'a')` — two clones of one
// stored list — where the runtime answers true, and the false condition
// pruned a live branch into a false-positive diagnostic. See
// check/go/carrier.go ScalarFoldOperand.
func HasContainerIdentity(v Value) bool {
	switch v.Data.(type) {
	case MapPayload, ListPayload, XmlElementPayload,
		*FlexListData, *FlexXmlData,
		*WeakFlexMapData, *WeakFlexListData, *WeakFlexXmlData:
		return true
	}
	return false
}

func sameContainer(a, b Payload) bool {
	switch av := a.(type) {
	case MapPayload:
		bv, ok := b.(MapPayload)
		return ok && av.M == bv.M
	case ListPayload:
		bv, ok := b.(ListPayload)
		if !ok {
			return false
		}
		// An empty list has no backing array to alias by — treat all
		// empty lists as the single empty list.
		if len(av.Elems) == 0 || len(bv.Elems) == 0 {
			return len(av.Elems) == len(bv.Elems)
		}
		return &av.Elems[0] == &bv.Elems[0]
	case *FlexListData:
		// Pointer identity — simpler and stronger than the
		// backing-array probe: the store pointer IS the container.
		bv, ok := b.(*FlexListData)
		return ok && av == bv
	case *FlexXmlData:
		// Pointer identity — the FlexXml store pointer IS the container.
		bv, ok := b.(*FlexXmlData)
		return ok && av == bv
	case *WeakFlexMapData:
		// Weak containers are reference cells — pointer identity.
		bv, ok := b.(*WeakFlexMapData)
		return ok && av == bv
	case *WeakFlexListData:
		bv, ok := b.(*WeakFlexListData)
		return ok && av == bv
	case *WeakFlexXmlData:
		bv, ok := b.(*WeakFlexXmlData)
		return ok && av == bv
	case XmlElementPayload:
		// An immutable Node/Xml has no pointer-backed store; it aliases
		// by its shared attribute map and children slice (a value dup'd
		// from one binding stays eq to its source; two fresh literals do
		// not). Empty children alias as the single empty children.
		bv, ok := b.(XmlElementPayload)
		if !ok || av.Tag != bv.Tag || av.Attr != bv.Attr {
			return false
		}
		if len(av.Cren) == 0 || len(bv.Cren) == 0 {
			return len(av.Cren) == len(bv.Cren)
		}
		return &av.Cren[0] == &bv.Cren[0]
	default:
		return false
	}
}

// deqListElems returns the element slice DeepEqual compares for a
// list-FAMILY value, and true; (nil, false) for a type-level operand
// (a list carrier) that has no value content and must fall to the
// render comparison. A populated typed list (`[1 :Integer]`) surfaces
// its elements through AsList (which reads the ChildTypeInfo.Elements
// arm); an EMPTY typed list value (`[:Integer]` — ChildTypeInfo with no
// elements, not a carrier) is the empty list, so it is deq-equal to
// `[]`, consistent with `(flex []) deq []`. NUR033.
func deqListElems(v Value) ([]Value, bool) {
	if v.Carrier {
		return nil, false
	}
	if rl, err := AsList(v); err == nil {
		return rl.elems, true
	}
	if _, ok := v.Data.(ChildTypeInfo); ok {
		return []Value{}, true
	}
	return nil, false
}

// deqMapEntries is deqListElems for the map family: a populated typed
// map surfaces its entries via AsMap, an empty typed map value is the
// empty map, and a type-level operand (map carrier, Record/Options type
// constructor) returns false to fall to the render comparison. NUR033.
func deqMapEntries(v Value) (ReadMap, bool) {
	if v.Carrier {
		return nil, false
	}
	if rm, err := AsMap(v); err == nil {
		return rm, true
	}
	if _, ok := v.Data.(ChildTypeInfo); ok {
		return NewOrderedMap(), true
	}
	return nil, false
}

// DeepEqual returns true if two values are deeply equal.
// Traverses lists and maps depth-first comparing all leaf values.
func DeepEqual(a, b Value) bool {
	// none
	if ValueType(a).Equal(TNone) && ValueType(b).Equal(TNone) {
		return true
	}

	// DepScalar pre-empts scalar dispatch — see ExactEqual for the
	// reasoning. Two DepScalars compare equal iff their type and
	// constraint payload match.
	if a.IsDepScalar() || b.IsDepScalar() {
		if !a.IsDepScalar() || !b.IsDepScalar() {
			return false
		}
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// Type literals (NUR034): two bare type nodes are deq iff they are
	// the same lattice type — reflexive and agreeing with `eq`'s
	// type-body arm (`List eq List` → true). This covers the container /
	// root literals (`List`, `Map`, `Any`) that the scalar path below
	// leaves out, so `List deq List` → true and `List deq Map` → false.
	// A mixed literal/concrete pair (`0 deq Integer`) needs both sides
	// bare, so it falls through to scalarFamilyEqual's NUR032 guard.
	if IsBareTypeNode(a) && IsBareTypeNode(b) {
		return ValueType(a).Equal(ValueType(b))
	}

	// Scalars — the same shared leaf comparison eq uses
	// (scalarFamilyEqual): deq must never disagree with eq on a scalar
	// leaf (there is no identity / structural distinction to draw for an
	// immutable scalar). Sharing one implementation also gives deq the
	// exact int64 comparison for large Integers — the float64 projection
	// this path used before 2026-07 rounds magnitudes above 2^53 and
	// reported distinct large integers as deq-equal while eq correctly
	// said false.
	if equal, handled := scalarFamilyEqual(a, b); handled {
		return equal
	}

	// Lists: same length, each element deeply equal. Flex nodes are
	// normalised to their family — a FlexList deep-equals a plain List
	// of the same content. NUR033: a TYPED list (`[1 :Integer]`) is
	// compared by its element VALUES, not by rendered form — so `[1 2
	// :Integer] deq [1 2]` is true and the relation stays transitive
	// with cross-leaf pairs (`[1.0] deq [1 :Integer]`). The render-string
	// fallback survives only for genuine type-level operands (a list
	// carrier), which carry no value content.
	if nodeFamily(a.Parent).Equal(TList) && nodeFamily(b.Parent).Equal(TList) {
		aElems, aOk := deqListElems(a)
		bElems, bOk := deqListElems(b)
		if !aOk || !bOk {
			return a.String() == b.String()
		}
		if len(aElems) != len(bElems) {
			return false
		}
		for i := range aElems {
			if !DeepEqual(aElems[i], bElems[i]) {
				return false
			}
		}
		return true
	}

	// Maps: same keys, each value deeply equal. NUR033: a TYPED map
	// (`{a:1 :Integer}`) is compared by its entry VALUES, like typed
	// lists. The render-string fallback survives only for type-level
	// operands with no entries to read (a map carrier, a Record/Options
	// type constructor).
	if nodeFamily(a.Parent).Equal(TMap) && nodeFamily(b.Parent).Equal(TMap) {
		aMap, aOk := deqMapEntries(a)
		bMap, bOk := deqMapEntries(b)
		if !aOk || !bOk {
			return a.String() == b.String()
		}
		if aMap.Len() != bMap.Len() {
			return false
		}
		for _, key := range aMap.Keys() {
			aVal, _ := aMap.Get(key)
			bVal, bHas := bMap.Get(key)
			if !bHas {
				return false
			}
			if !DeepEqual(aVal, bVal) {
				return false
			}
		}
		return true
	}

	// XML elements (immutable Node/Xml or mutable FlexXml): structural
	// equality over tag / attributes / children, so a `parse xml` result
	// and the equivalent embedded literal — and a flex copy and its source
	// — compare deeply equal (attribute order is not significant). See
	// core_xml.go::xmlElementsEqual and design/XML-LITERAL.0.md §5.6.
	if IsXmlValue(a) && IsXmlValue(b) {
		return xmlElementsEqual(a, b)
	}

	// Flat instances (class + Resource/Entity): structural equality
	// requires the SAME (exact) type — a Point3 is never deq-equal to a
	// Point even with equal visible fields — and field-wise deep
	// equality. The key set is the union of schema fields and own
	// fields; both instance kinds store a flat Fields map, so a lookup
	// is a plain map hit. See design/CLASS-OBJECT.10.md.
	if IsFlatInstance(a) && IsFlatInstance(b) {
		if !a.Parent.Equal(b.Parent) {
			return false
		}
		aFields, aSchema, aok := FlatInstanceParts(a)
		bFields, bSchema, bok := FlatInstanceParts(b)
		if !aok || !bok || aFields == nil || bFields == nil {
			return false
		}
		seen := map[string]bool{}
		var keys []string
		addKeys := func(ks []string) {
			for _, k := range ks {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
		addKeys(aSchema)
		_ = bSchema // a.Parent == b.Parent, so their schemas coincide
		addKeys(aFields.Keys())
		addKeys(bFields.Keys())
		for _, k := range keys {
			av, aok := aFields.Get(k)
			bv, bok := bFields.Get(k)
			if aok != bok {
				return false
			}
			if aok && !DeepEqual(av, bv) {
				return false
			}
		}
		return true
	}

	// Opaque Ideal handles (NUR031). deq is the DEEP-VALUE half of the
	// rule: a Store by its own entries (the same projection as `convert
	// Map`), Error by its fields; the pure handles (Timeout, Interval,
	// the Module descriptor) have no deeper structure than their
	// identity, so deq is their reference identity, matching eq. Code
	// values (Function, Word) stay below at the terminal `false`.
	if eq, handled := opaqueIdealDeepEqual(a, b); handled {
		return eq
	}

	// Function values (NUR031). deq is DEEP VALUE equality, per NUR011: a
	// function's value is its signatures and body, so two references to one
	// function are deq, and so is a function re-parsed from its own canon —
	// which is what ADR-015 needs, and what reference identity alone could
	// never give (a re-parsed function is a fresh array).
	//
	// The NAME is deliberately excluded. It is the binding a function was
	// reached through, not part of the function: including it is exactly
	// what made `a/v deq b/v` false for two bindings of one function, and
	// what kept `f/v deq f/v` false at all.
	if af, aok := a.Data.(FnDefInfo); aok {
		if bf, bok := b.Data.(FnDefInfo); bok {
			return fnStructurallyEqual(af, bf)
		}
		return false
	}

	// Last chance before the terminal verdict: a type that installed the
	// DeepEqualer capability answers for its own values. Placing the walk
	// HERE rather than at the top is what makes it additive — it can only
	// turn this `false` into a real answer, never override an arm above.
	// See deepequal_capability.go.
	if eq, handled := deepEqualCapability(a, b); handled {
		return eq
	}

	// Declared type VALUES (NUR031): a `class` and refinements of one, a
	// disjunction / `enum`, a `fnsig` or `surface`, an uninstantiated
	// `gen` schema. A type is an immutable DECLARATION, so eq and deq
	// coincide on it exactly as they do on a scalar leaf and on an Error
	// — and deq gets that by SHARING eq's arm rather than deriving a
	// second answer that could disagree with it.
	//
	// This sits at the bottom, next to the terminal, for the same reason
	// the capability walk does: from here it can only turn `false` into a
	// real answer. Placing it above would capture the type bodies that
	// ARE containers (an implicit-map record shape, a Table type) and
	// change answers the container arms already give correctly.
	//
	// The bare literals (`List deq List`) never reach here — the NUR034
	// arm near the top answers them, and answers them the same way.
	//
	// A compiled CLOSURE lands here too (IsTypeBody admits a Function-typed
	// value, and the fn arm above claimed only the FnDefInfo payloads), and
	// that is the answer it wants: the comparison reaches its Render, which
	// IS the fn's canon — so the VM's closure and the interpreter's fn value
	// answer deq the same way, by content.
	if IsTypeBody(a) && IsTypeBody(b) {
		return a.Parent.Equal(b.Parent) && ValuesEqual(a, b)
	}

	// A SEALED HOST payload (NUR031): box identity, the same answer eq
	// gives. It sits below the capability walk deliberately — a host type
	// that installed a DeepEqualer has a real value equality to offer, and
	// this fallback must not pre-empt it. On the eq side there is no such
	// walk, so that arm lives up in opaqueIdealExactEqual instead.
	if ap, ok := a.Data.(ExtensionPayload); ok {
		if eq, handled := hostPayloadIdentity(ap, b); handled {
			return eq
		}
	}

	// Different types or unsupported — not equal.
	return false
}

// opaqueIdealExactEqual is the `eq` (reference) half of NUR031 for the
// opaque Ideal handles that would otherwise fall through to `false`.
// Returns (result, true) when a is such a handle; (false, false) to let
// the caller reach its own fall-through. A pointer-backed handle (Store,
// Timeout, Interval, the boxed Module descriptor) is eq iff it is the
// SAME pointer; Error (a value struct) is eq by fields.
func opaqueIdealExactEqual(a, b Value) (bool, bool) {
	switch av := a.Data.(type) {
	case *StoreInstanceInfo:
		bv, ok := b.Data.(*StoreInstanceInfo)
		return ok && av == bv, true
	case *TimeoutInfo:
		bv, ok := b.Data.(*TimeoutInfo)
		return ok && av == bv, true
	case *IntervalInfo:
		bv, ok := b.Data.(*IntervalInfo)
		return ok && av == bv, true
	case ErrorInfo:
		bv, ok := b.Data.(ErrorInfo)
		// Error is value-like: eq ≡ deq, so it requires the same exact
		// type as well as equal fields (a `refine Error` subtype is not
		// eq to a plain Error).
		return ok && a.Parent.Equal(b.Parent) && errorInfoEqual(av, bv), true
	case WordInfo:
		bv, ok := b.Data.(WordInfo)
		// A word is the other value-like code value (NUR031): an
		// immutable name-plus-modifiers with no reference behind it, so
		// eq ≡ deq and both compare the whole struct — the modifiers
		// included, since `f/v` and `f/s` are different words. It is
		// comparable by construction (only string/int/bool fields), the
		// same property that lets Error compare by fields.
		return ok && a.Parent.Equal(b.Parent) && av == bv, true
	case ExtensionPayload:
		if eq, handled := moduleDescIdentity(av, b); handled {
			return eq, handled
		}
		return hostPayloadIdentity(av, b)
	}
	return false, false
}

// moduleDescIdentity is the shared eq/deq arm for the Ideal/Module
// DESCRIPTOR (NUR031, narrow half): an ExtensionPayload boxing a
// *ModuleDesc. The descriptor is an opaque handle whose identity IS its
// value — like Timeout/Interval, eq and deq are both pointer identity
// (per-import-instance: every namespace bound by one import shares one
// boxed pointer through Value copies). handled=false for any OTHER
// ExtensionPayload body (host/plugin payloads the kernel does not
// inspect — they keep the terminal fall-through). It must NEVER apply
// Go == to a bare ModuleDesc: the struct holds a map and is not a
// comparable type, so that would panic at runtime.
func moduleDescIdentity(av ExtensionPayload, b Value) (bool, bool) {
	ad, ok := av.Body.(*ModuleDesc)
	if !ok {
		return false, false
	}
	bp, ok := b.Data.(ExtensionPayload)
	if !ok {
		return false, true
	}
	bd, ok := bp.Body.(*ModuleDesc)
	return ok && ad == bd, true
}

// hostPayloadIdentity is REFERENCE identity for a SEALED host payload
// whose Body is a POINTER — an `IO.open` file handle, a lock, a watcher,
// an mmap, a query builder (NUR031). These were the last members of the
// record's fall-through set: not eq to themselves, not deq to themselves.
//
// Reference identity is the only equality the kernel may give them, and
// it is the right one: a host handle is an opaque reference, like a
// timer, so its identity IS its value and eq and deq coincide. The
// Sealed Payload rule holds throughout — the pointer is compared, never
// read through.
//
// The POINTER restriction is the whole contract, not a convenience.
// `ExtensionPayload` is a value struct, so a Value copy shares nothing
// but the Body; if the Body is not a reference there is no identity to
// compare, only its contents — and comparing contents would make two
// independently constructed payloads `eq`, which is exactly what
// reference identity must not say. A non-pointer body therefore
// DECLINES, leaving construction of an equality to the host: a type that
// wants one installs the DeepEqualer capability, whose walk runs before
// this arm on the deq side.
//
// Declining also removes the `==`-on-`any` panic entirely. A map- or
// slice-bodied payload is uncomparable in Go and would have crashed the
// process; it is not a pointer, so it never reaches the comparison.
func hostPayloadIdentity(a ExtensionPayload, b Value) (bool, bool) {
	if !isPointerBody(a.Body) {
		return false, false
	}
	bp, ok := b.Data.(ExtensionPayload)
	if !ok {
		return false, true
	}
	if !isPointerBody(bp.Body) {
		return false, true
	}
	return a.Body == bp.Body, true
}

// isPointerBody reports whether a sealed payload's Body is a pointer —
// the only shape carrying an identity independent of its contents.
// reflect is the only way to ask this of an `any`; the cost is confined
// to the equality fall-through, which no hot path reaches.
func isPointerBody(body any) bool {
	if body == nil {
		return false
	}
	return reflect.TypeOf(body).Kind() == reflect.Ptr
}

// opaqueIdealDeepEqual is the `deq` (deep-value) half of NUR031. It
// differs from the eq half only for Store, whose deep value is its own
// entry set (structural), and matches it for the pure handles and Error.
//
// The value-comparing arms (Store, Error) additionally require the SAME
// exact type — a `refine Store` subtype is never deq to a plain Store
// even with equal entries, matching the flat-instance rule (a Point3 is
// never deq to a Point). This also keeps DeqKey sound: deqFam buckets a
// deq-comparable handle by its type ID, so two values DeepEqual can
// equate must share a type (else the collection words would miss the
// pair — the store-subtype bug the NUR031 review caught).
func opaqueIdealDeepEqual(a, b Value) (bool, bool) {
	switch av := a.Data.(type) {
	case *StoreInstanceInfo:
		bv, ok := b.Data.(*StoreInstanceInfo)
		return ok && a.Parent.Equal(b.Parent) && storeDeepEqual(av, bv), true
	case *TimeoutInfo:
		bv, ok := b.Data.(*TimeoutInfo)
		return ok && av == bv, true
	case *IntervalInfo:
		bv, ok := b.Data.(*IntervalInfo)
		return ok && av == bv, true
	case ErrorInfo:
		bv, ok := b.Data.(ErrorInfo)
		return ok && a.Parent.Equal(b.Parent) && errorInfoEqual(av, bv), true
	case WordInfo:
		bv, ok := b.Data.(WordInfo)
		return ok && a.Parent.Equal(b.Parent) && av == bv, true
	case ExtensionPayload:
		// The Module descriptor: deq is its reference identity, matching
		// eq (see moduleDescIdentity — an opaque handle with no deeper
		// structure than its identity, like the timers). A SEALED HOST
		// payload is deq-comparable too (NUR031), but its arm sits at the
		// bottom of DeepEqual, below the DeepEqualer walk, so a host type
		// that installed the capability answers for its own values rather
		// than being pre-empted by the box-identity fallback.
		return moduleDescIdentity(av, b)
	}
	return false, false
}

// storeDeepEqual compares two Stores by their OWN entries — the same
// key/value set `convert Map` projects (prototype-inherited keys are not
// part of a Store's own value, so they are excluded, matching the
// projection). Pointer-identical stores short-circuit.
func storeDeepEqual(a, b *StoreInstanceInfo) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// The VISIBLE entries, not each store's newest copy-on-write layer
	// (NUR052). NUR031 defined a Store's deq as "the same own-entry
	// projection as `convert Map`", and that projection now walks the
	// prototype chain — so comparing a.Data to b.Data would leave two
	// stores that convert to the same map, and answer identically to
	// every lookup, comparing UNEQUAL because their newest layers happen
	// to hold different keys. Which layer a key was written on is not
	// part of a store's value.
	ae, be := a.VisibleEntries(), b.VisibleEntries()
	if len(ae) != len(be) {
		return false
	}
	// Both slices are sorted by key, so one pass compares them.
	for i, e := range ae {
		if e.Key != be[i].Key || !DeepEqual(e.Value, be[i].Value) {
			return false
		}
	}
	return true
}

// errorInfoEqual compares two Errors field-wise: code, message, and the
// optional spec-map Data (deeply, nil-safe). An Error is a value-like
// Ideal, so eq and deq coincide on it (like a scalar leaf).
func errorInfoEqual(a, b ErrorInfo) bool {
	if a.Code != b.Code || a.Message != b.Message {
		return false
	}
	am, bm := a.Data, b.Data
	if am == nil {
		am = NewOrderedMap()
	}
	if bm == nil {
		bm = NewOrderedMap()
	}
	return DeepEqual(NewMap(am), NewMap(bm))
}

// isDeqComparableHandle reports whether v is an opaque Ideal handle that
// NUR031 made deq-comparable (Store, Error, Timeout, Interval, the
// Module descriptor) — as opposed to a code value (Function, Word) that
// remains equal to nothing. Used by DeqKey to bucket these into a
// pairwise-scan family (by handleKind) instead of the DeqNeverEqual
// fast path.
func isDeqComparableHandle(v Value) bool {
	return handleKind(v) != ""
}

// The comparison-word registrations (lt / gt / lte / gte / eq /
// neq / deq / between) live in lang/go/engine/native_compare.go. The
// handlers and MakeDepScalarSig helper are exported eng primitives.

// The ordering words lt / gt / lte / gte / cmp are family-restricted:
// they compare only same-type values, or values a shared same-family
// Comparer can handle (Integer-vs-Float, two Dates, …). A cross-family
// pair (Integer-vs-String, List-vs-Map) raises [boru/incomparable] and
// directs the caller to tcmp, which keeps the full cross-type total
// order. The guard lives in orderedCompare; tcmp (TcmpHandler) bypasses
// it.

// numericUnordered reports whether a relational comparison of a and b is
// IEEE-754 *unordered*: both are concrete numbers and at least one is
// NaN. The ordering words lt / lte / gt / gte all yield false in that
// case (NaN is neither less, equal, nor greater). Cross-family pairs
// (e.g. Float-vs-String) are NOT unordered here — they fall through to
// the family guard, which raises [boru/incomparable]. The total-order
// words cmp / tcmp / sort deliberately bypass this and give NaN a defined
// slot (see numberCompareBehavior.Compare).
func numericUnordered(a, b Value) bool {
	return isConcreteNumber(a) && isConcreteNumber(b) && (isNaNValue(a) || isNaNValue(b))
}

func isConcreteNumber(v Value) bool {
	return !v.IsDepScalar() && IsConcrete(v) && ValueType(v).ConformsTo(TNumber)
}

func isNaNValue(v Value) bool {
	if !isConcreteNumber(v) {
		return false
	}
	f, err := AsNumber(v)
	return err == nil && math.IsNaN(f)
}

// signedZeroPair reports whether a relational comparison of a and b is
// an IEEE ±0 pair: both concrete numbers of zero magnitude with the
// Float negative zero on at least one side. IEEE §5.11 defines
// -0 == +0, so lt/lte/gt/gte must treat the pair as EQUAL even though
// the total order behind cmp/tcmp/sort now places -0.0 strictly before
// every positive zero (totalOrder §5.10, NUR013) — without this guard
// the totalOrder fix would CREATE an IEEE violation (-0.0 lt 0.0 →
// true). The zero-magnitude probe runs in the exact big.Rat domain so
// the cross-leaf pairs (Integer 0, BigInteger/BigDecimal zeros vs
// -0.0) are guarded too.
func signedZeroPair(a, b Value) bool {
	if !isNegZeroFloat(a) && !isNegZeroFloat(b) {
		return false
	}
	return isZeroNumber(a) && isZeroNumber(b)
}

// isZeroNumber reports whether v is a concrete number of exactly zero
// magnitude (either sign), across all four numeric leaves.
func isZeroNumber(v Value) bool {
	if !isConcreteNumber(v) {
		return false
	}
	r, ok := toRatExact(v)
	return ok && r.Sign() == 0
}

// relationalHandler builds the handler for one ordering word: the IEEE
// *unordered* rule first (NaN on either side → false, see
// numericUnordered), then the IEEE signed-zero equality (±0 pairs are
// EQUAL per §5.11, see signedZeroPair — the total order's -0 < +0 slot
// must not leak into the relationals), then the family-restricted
// orderedCompare with keep() mapping the three-way result onto the
// word's truth condition. The four words are the same function with
// one comparator swapped — building them from one factory keeps the
// NaN and ±0 rules and the arg order (args[1] vs args[0], the `a OP b`
// reading convention) from drifting.
func relationalHandler(op string, keep func(int) bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
	return func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		if numericUnordered(args[0], args[1]) {
			return []Value{NewBoolean(false)}, nil
		}
		if signedZeroPair(args[0], args[1]) {
			return []Value{NewBoolean(keep(0))}, nil
		}
		cmp, err := orderedCompare(op, args[1], args[0])
		if err != nil {
			return nil, err
		}
		return []Value{NewBoolean(keep(cmp))}, nil
	}
}

var (
	LtHandler  = relationalHandler("lt", func(c int) bool { return c < 0 })
	GtHandler  = relationalHandler("gt", func(c int) bool { return c > 0 })
	LteHandler = relationalHandler("lte", func(c int) bool { return c <= 0 })
	GteHandler = relationalHandler("gte", func(c int) bool { return c >= 0 })
)

// CmpHandler implements `cmp` — a three-way comparison restricted to
// same-family operands. `a b cmp` returns -1 when a sorts before b, 0
// when they tie, and 1 when a sorts after b, using the same family
// ordering as lt / gt. Cross-family operands raise [boru/incomparable];
// use tcmp for a cross-type total order. The result is normalised to
// its sign, so a custom `behave compare` body that returns a nonzero
// magnitude other than ±1 still yields exactly -1 / 0 / 1.
func CmpHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := orderedCompare("cmp", args[1], args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewInteger(int64(signOf(cmp)))}, nil
}

// TcmpHandler implements `tcmp` — the total-order three-way comparison.
// Unlike cmp, it compares ANY two values via the unified lattice order
// (the same order sort and the collection words use), returning -1 / 0
// / 1. Use it when you deliberately want cross-type ordering that cmp
// refuses (e.g. `1 tcmp "a"`).
func TcmpHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cmp, err := CompareValues(args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("tcmp: %w", err)
	}
	return []Value{NewInteger(int64(signOf(cmp)))}, nil
}

// signOf normalises an arbitrary comparison magnitude to -1 / 0 / 1.
func signOf(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func EqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(ExactEqual(args[0], args[1]))}, nil
}

func NeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!ExactEqual(args[0], args[1]))}, nil
}

func DeqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(DeepEqual(args[0], args[1]))}, nil
}

// sameFnIdentity reports whether two fn payloads are the SAME function —
// reference identity, keyed on the identity token NewFunction mints and
// every same-function derivation copies (NUR031).
//
// The token exists because the payload offers no other stable reference.
// The Signatures backing array is the obvious candidate and is wrong: for a
// boru-bodied word aggregateDispatch rebuilds that slice per NAME, so two
// bindings of one function (`def a (f/v)` / `def b (f/v)`) land on two
// arrays and read as two functions — the record's complaint exactly.
//
// A payload with no token — a FnDefInfo literal built outside NewFunction,
// i.e. a compile-time carrier or a probe with no fn value behind it — has
// no identity, and answering true would make every one of them eq to every
// other.
// closureIdentityEqual is ExactEqual's arm for a pair with a compiled closure
// on at least one side: handled reports that; eq is the identity comparison —
// the closure's token against the other side's, which is another closure's
// token or a bridged FnDefInfo's ident. A side that is neither (data, a type)
// or a token-less closure never matches.
func closureIdentityEqual(a, b Value) (eq, handled bool) {
	as, aIs := closureIdentSeqOf(a)
	bs, bIs := closureIdentSeqOf(b)
	if !aIs && !bIs {
		return false, false
	}
	return as != 0 && as == bs, true
}

// closureIdentSeqOf reads the closure identity sequence a value carries — a
// closure's own, or the one a bridged FnDefInfo's token stands in for — and
// whether v IS a closure. Zero: no closure identity.
func closureIdentSeqOf(v Value) (seq uint64, isClosure bool) {
	switch d := v.Data.(type) {
	case ClosurePayload:
		return d.Ident.seq, true
	case FnDefInfo:
		if d.ident != nil {
			return d.ident.closure, false
		}
	}
	return 0, false
}

func sameFnIdentity(a, b FnDefInfo) bool {
	if a.ident == nil || b.ident == nil {
		return false
	}
	// Two tokens minted for one compiled closure (two bridges of it) are one
	// function: they share the closure's sequence.
	return a.ident == b.ident || (a.ident.closure != 0 && a.ident.closure == b.ident.closure)
}

// fnStructurallyEqual reports whether two fn payloads have the same VALUE
// (NUR031). deq is deep value equality per NUR011, and a function's value
// is its CONTENT: parameters, returns and body.
//
// The comparison is canon — the content serialisation the language already
// has, now that NUR059 and this record made it name-INDEPENDENT. That
// choice is borrowed from Unison's content-addressed code, and it is
// borrowed for exactly one job: different body, different canon, not deq.
//
// Walking `Impl` instead does not work, and the failure is silent rather
// than loud: after InstallFnDef a signature's Impl is a Go handler
// (buildFnBodyHandler), so there is no body to reach and every installed
// function reads as identical to every other of the same arity — a false
// POSITIVE, strictly worse than the false negative it replaced. canonFnDef
// renders from OwnSigs and still sees the body after installation, which is
// what makes it the reachable content.
//
// The DEFINING MODULE is part of the content. boru resolves a function's
// free words in the module that defined it (design/FUNCTION-VALUE-SCOPE.0.md,
// "lexical module, dynamic within it"), so two identical bodies in different
// modules do not mean the same thing — which is also why boru cannot adopt
// content addressing wholesale the way Unison does: Unison hashes
// dependencies transitively and has no ambient namespace, while boru keeps
// one deliberately (module-level dynamic binding is what hot reload rides
// on). Registry is set only at module-export resolution, so two locally
// defined fns both carry nil and compare on content alone.
func fnStructurallyEqual(a, b FnDefInfo) bool {
	// The same function is trivially deq to itself, and that is the common
	// case — worth short-circuiting before rendering two canons.
	if sameFnIdentity(a, b) {
		return true
	}
	// The DEFINING SCOPE is part of the content, and a nil Registry is a
	// scope like any other — it means "wherever this is running", which is
	// not the same place as a named module. Admitting a nil/non-nil pair
	// here (as an earlier `both non-nil` guard did) let a module-owned fn
	// and a locally defined one with identical text compare deq although
	// their free words resolve in different registries.
	if a.Registry != b.Registry {
		return false
	}
	// CAPTURES are content too. canonFnDef renders params, returns and
	// body — not the closure environment — so two closures built from one
	// factory over different arguments have identical canon and entirely
	// different behaviour. Comparing the captures is what stops
	// `(make-adder 5)` and `(make-adder 9)` reading as the same function,
	// and stops `unique` discarding one of them.
	if !capturedBindingsEqual(a.Captured, b.Captured) {
		return false
	}
	return canonFnDef(a) == canonFnDef(b)
}

// capturedBindingsEqual compares two closure environments by name and by
// captured VALUE. Both lists are sorted by name at construction
// (ComputeCaptures), so a positional walk is a set comparison.
func capturedBindingsEqual(a, b []CapturedBinding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !DeepEqual(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}
