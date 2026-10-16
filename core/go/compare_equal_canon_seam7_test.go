package core

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in equal.go, compare.go, compare_types.go,
// compare_scalar_behaviors.go, size.go and canon.go. Most target the
// error / mismatch arms of the structural equality and comparison
// helpers, driven by crafting Values directly. Per design/TEST-SEAMS.10.md.

import "testing"

// --- equal.go: valuesEqualDefault mismatch/error arms ---------------------

func TestS7ValuesEqualDefaultBoundedUnreadable(t *testing.T) {
	// A bounded Type whose Child denotes no lattice node makes
	// AsBoundedType fail; equality must return false.
	bad := NewValueRaw(TType, ChildTypeInfo{Child: MakeValue(nil, IntPayload{N: 1})})
	if valuesEqualDefault(bad, bad) {
		t.Error("unreadable bounded Type must not compare equal")
	}
}

func TestS7ValuesEqualDefaultDepVsNonDep(t *testing.T) {
	dep := NewDepScalar(DepGT, NewInteger(10))
	if valuesEqualDefault(dep, NewInteger(5)) {
		t.Error("a DepScalar and a plain Integer are not equal")
	}
}

func TestS7ValuesEqualDefaultListTableMismatch(t *testing.T) {
	table := NewValueRaw(TList, TableTypeInfo{})
	plain := NewList(nil)
	if valuesEqualDefault(table, plain) {
		t.Error("a table and a plain list are not equal")
	}
}

func TestS7ValuesEqualDefaultMapVariantMismatches(t *testing.T) {
	opt := NewValueRaw(TMap, OptionsTypeInfo{Fields: NewOrderedMap()})
	opt2 := NewValueRaw(TMap, OptionsTypeInfo{Fields: NewOrderedMap()})
	if !valuesEqualDefault(opt, opt2) {
		t.Error("two empty Options types should be equal")
	}
	plainMap := NewMap(NewOrderedMap())
	if valuesEqualDefault(opt, plainMap) {
		t.Error("Options vs plain map must not be equal")
	}
	typedMap := NewValueRaw(TMap, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	if valuesEqualDefault(typedMap, plainMap) {
		t.Error("typed map vs plain map must not be equal")
	}
}

// --- compare_types.go: list/map structural compare arms -------------------

func TestS7CompareListElemsArms(t *testing.T) {
	typed := NewValueRaw(TList, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	plain := NewList([]Value{NewInteger(1)})
	// AsMutableList fails on the typed list -> canonical-render fallback.
	if _, err := compareListElems(typed, plain); err != nil {
		t.Fatalf("typed-list fallback should not error: %v", err)
	}
	// Different lengths -> length ordering.
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1)})
	if c, err := compareListElems(a, b); err != nil || c <= 0 {
		t.Errorf("longer list should sort after: c=%d err=%v", c, err)
	}
	// Element comparison error: elements with a nil lattice type.
	ea := NewList([]Value{MakeValue(nil, IntPayload{N: 1})})
	eb := NewList([]Value{MakeValue(nil, IntPayload{N: 2})})
	if _, err := compareListElems(ea, eb); err == nil {
		t.Error("element with nil type must surface a compare error")
	}
}

func TestS7CompareMapEntriesArms(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m1.Set("b", NewInteger(2))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(1))
	if c, err := compareMapEntries(NewMap(m1), NewMap(m2)); err != nil || c <= 0 {
		t.Errorf("more keys should sort after: c=%d err=%v", c, err)
	}
	// Same key set but a value with a nil lattice type -> compare error.
	ma := NewOrderedMap()
	ma.Set("x", MakeValue(nil, IntPayload{N: 1}))
	mb := NewOrderedMap()
	mb.Set("x", MakeValue(nil, IntPayload{N: 2}))
	if _, err := compareMapEntries(NewMap(ma), NewMap(mb)); err == nil {
		t.Error("map value with nil type must surface a compare error")
	}
}

// --- compare.go -----------------------------------------------------------

func TestS7CompareValuesNilType(t *testing.T) {
	if _, err := CompareValues(MakeValue(nil, IntPayload{N: 1}), NewInteger(1)); err == nil {
		t.Error("a value with a nil lattice type must fail to compare")
	}
}

func TestS7TcmpHandlerError(t *testing.T) {
	args := []Value{NewInteger(1), MakeValue(nil, IntPayload{N: 1})}
	if _, err := TcmpHandler(args, nil, nil, nil); err == nil {
		t.Error("tcmp with a nil-typed operand must error")
	}
}

func TestS7SameContainerXmlAndFlex(t *testing.T) {
	xe := NewXmlElement("a", nil, []Value{NewString("x")})
	if !sameContainer(DataOf(&xe), DataOf(&xe)) {
		t.Error("an xml element aliases itself")
	}
	fx := NewFlexXml("a", nil, nil)
	if !sameContainer(DataOf(&fx), DataOf(&fx)) {
		t.Error("a flex-xml store aliases itself")
	}
	// Distinct flex-xml stores are not the same container.
	fx2 := NewFlexXml("a", nil, nil)
	if sameContainer(DataOf(&fx), DataOf(&fx2)) {
		t.Error("two distinct flex-xml stores are not the same container")
	}
}

// HasContainerIdentity must answer for exactly the payloads sameContainer
// decides — the check pass declines to const-fold over them because its
// values are copies and cannot carry runtime container identity.
func TestS7HasContainerIdentity(t *testing.T) {
	// Every identity-bearing payload sameContainer has an arm for.
	identity := []Value{
		NewList([]Value{NewInteger(1)}),
		NewMap(NewOrderedMap()),
		NewXmlElement("a", nil, []Value{NewString("x")}),
		NewFlexList(nil),
		NewFlexXml("a", nil, nil),
		NewWeakFlexMap(),
		NewWeakFlexList(),
		NewWeakFlexXml("a"),
	}
	for _, v := range identity {
		if !HasContainerIdentity(v) {
			t.Errorf("%s (%T) compares by container identity but is not reported as such", ParentOf(&v), DataOf(&v))
		}
		// The mirror property that makes the predicate meaningful: the
		// value aliases ITSELF under sameContainer.
		if !sameContainer(DataOf(&v), DataOf(&v)) {
			t.Errorf("%T does not alias itself — the arms have drifted apart", DataOf(&v))
		}
	}
	// Scalars compare BY VALUE, so they keep folding. The immutable
	// structured scalars are included deliberately: a Micron/Pathon/Time
	// compares structurally, so a check-mode copy answers exactly as the
	// runtime does.
	byValue := []Value{
		NewInteger(1), NewFloat(1.5), NewString("s"), NewBoolean(true), NewAtom("a"),
		NewNone(),
	}
	for _, v := range byValue {
		if HasContainerIdentity(v) {
			t.Errorf("%s (%T) compares by value and must keep folding", ParentOf(&v), DataOf(&v))
		}
	}
	// A payload with no aliasable identity at all (table data) is not a
	// container for this purpose either — sameContainer's default arm.
	if HasContainerIdentity(Value{}) {
		t.Error("a zero value has no container identity")
	}
}

func TestS7DeepEqualTypedListFallback(t *testing.T) {
	a := NewValueRaw(TList, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	b := NewValueRaw(TList, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	// AsMutableList fails -> structural String() fallback (equal here).
	if !DeepEqual(a, b) {
		t.Error("identical typed lists should be deep-equal via String() fallback")
	}
}

func TestS7DeepEqualFlatInstanceArms(t *testing.T) {
	cls := addrOf(withID(Type{c: &content{tmeta: &typeMeta{Name: "S7Cls"}}}, "S7Cls"))
	// One instance with a nil Fields map -> false via the nil-guard.
	nilFields := NewClassInstance(cls, ClassInstanceInfo{})
	if DeepEqual(nilFields, nilFields) {
		t.Error("a class instance with nil Fields must not deep-equal (nil guard)")
	}
	// Same type, key present in one, absent in the other -> false.
	fa := NewOrderedMap()
	fa.Set("x", NewInteger(1))
	fb := NewOrderedMap()
	insA := NewClassInstance(cls, ClassInstanceInfo{Fields: fa})
	insB := NewClassInstance(cls, ClassInstanceInfo{Fields: fb})
	if DeepEqual(insA, insB) {
		t.Error("instances with differing key presence must not be deep-equal")
	}
}

// --- compare_scalar_behaviors.go ------------------------------------------

func TestS7ScalarFormatDelegateMarkers(t *testing.T) {
	// The formatDelegate markers are no-op tags (zero-statement bodies).
	numberCompareBehavior{}.formatDelegate()
	stringCompareBehavior{}.formatDelegate()
	booleanCompareBehavior{}.formatDelegate()
	atomCompareBehavior{}.formatDelegate()
	scalarCompareBehavior{}.formatDelegate()
	wordCompareBehavior{}.formatDelegate()
}

func TestS7NumberCompareBigOptOut(t *testing.T) {
	// numIsBig on the left, but the payload is unreadable and the other
	// side is not a rat-exact number -> ErrNoComparer opt-out.
	big := MakeValue(TBigInteger, StrPayload{S: "x"})
	if _, err := (numberCompareBehavior{}).Compare(big, NewString("y")); err != ErrNoComparer {
		t.Errorf("unreadable big operand should opt out with ErrNoComparer, got %v", err)
	}
}

func TestS7ToRatExactErrorArms(t *testing.T) {
	if _, ok := toRatExact(MakeValue(TBigDecimal, StrPayload{S: "x"})); ok {
		t.Error("unreadable BigDecimal payload must not convert")
	}
	if _, ok := toRatExact(MakeValue(TBigInteger, StrPayload{S: "x"})); ok {
		t.Error("unreadable BigInteger payload must not convert")
	}
	if _, ok := toRatExact(MakeValue(TInteger, StrPayload{S: "x"})); ok {
		t.Error("unreadable Integer payload must not convert")
	}
}

func TestS7NonFiniteRankBadFloat(t *testing.T) {
	if r, ok := nonFiniteRank(MakeValue(TFloat, StrPayload{S: "x"})); ok || r != 0 {
		t.Errorf("unreadable Float payload must not rank: r=%d ok=%v", r, ok)
	}
}

func TestS7WordCompareLiteralPrologue(t *testing.T) {
	// Two bare Word type literals settle in the prologue (litVsLitOrder).
	if c, err := (wordCompareBehavior{}).Compare(NewTypeLiteral(TWord), NewTypeLiteral(TWord)); err != nil || c != 0 {
		t.Errorf("two Word literals should tie: c=%d err=%v", c, err)
	}
}

func TestS7ComparePathonsArms(t *testing.T) {
	// Exactly one bare literal -> type-literal-first rule returns early.
	if c := comparePathons(NewTypeLiteral(TString), NewString("x")); c != -1 {
		t.Errorf("literal-first rule: got %d, want -1", c)
	}
	// Two non-Pathon concretes -> rendered-form fallback.
	if comparePathons(NewInteger(1), NewInteger(2)) == 0 {
		t.Error("distinct non-pathon concretes should not tie via render")
	}
}

// --- size.go --------------------------------------------------------------

func TestS7IdealSizeBehavior(t *testing.T) {
	idealRootBehavior{}.formatDelegate() // zero-statement marker

	store := &StoreInstanceInfo{Data: map[string]Value{"a": NewInteger(1), "b": NewInteger(2)}}
	sv := NewValueRaw(TIdeal, store)
	if got := (idealRootBehavior{}).Size(sv); got != 2 {
		t.Errorf("store size = %d, want 2", got)
	}
	tv := NewValueRaw(TIdeal, TableData{Rows: []Value{NewInteger(1), NewInteger(2), NewInteger(3)}})
	if got := (idealRootBehavior{}).Size(tv); got != 3 {
		t.Errorf("table size = %d, want 3", got)
	}
}

// --- canon.go -------------------------------------------------------------

func TestS7CanonValueEdges(t *testing.T) {
	// A carrier with a nil Parent has no type node -> "none".
	if got := CanonValue(MakeCarrierValue(nil, nil)); got != "none" {
		t.Errorf("nil-parent carrier canon = %q, want none", got)
	}
	// A bounded Type with an unreadable bound falls back to String().
	bad := NewValueRaw(TType, ChildTypeInfo{Child: MakeValue(nil, IntPayload{N: 1})})
	if got := CanonValue(bad); got != bad.String() {
		t.Errorf("unreadable bounded canon = %q, want String() = %q", got, bad.String())
	}
}

func TestS7CanonReachNonReach(t *testing.T) {
	if got := canonReach(NewInteger(1)); got != NewInteger(1).String() {
		t.Errorf("canonReach on a non-reach = %q, want the String() form", got)
	}
}

func TestS7RenderDepScalarNonDep(t *testing.T) {
	if got := renderDepScalar(NewInteger(1)); got != "" {
		t.Errorf("renderDepScalar(non-dep) = %q, want empty", got)
	}
}
