package core

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// Wave 4b coverage: compare.go (handlers, ExactEqual/DeepEqual,
// sameContainer, ordering guards), compare_scalar_behaviors.go (family
// Comparers, exact big-number comparison, non-finite floats, pathon
// ordering), compare_types.go (structural tiebreak), and equal.go
// (valuesEqualDefault dispatch). Positive and negative cases are
// paired throughout.

func mustCmp(t *testing.T, a, b Value) int {
	t.Helper()
	c, err := CompareValues(a, b)
	if err != nil {
		t.Fatalf("CompareValues(%v, %v): %v", a, b, err)
	}
	return c
}

// --- handlers ---------------------------------------------------------------

func TestCmpHandlerSameFamily(t *testing.T) {
	// `1 2 cmp` — args[0] is top (2), args[1] is deeper (1) → -1.
	out, err := CmpHandler([]Value{NewInteger(2), NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("CmpHandler: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != -1 {
		t.Errorf("1 cmp 2 = %d, want -1", n)
	}
	// Equal pair ties.
	out, err = CmpHandler([]Value{NewInteger(3), NewInteger(3)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("CmpHandler equal: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 0 {
		t.Errorf("3 cmp 3 = %d, want 0", n)
	}
}

func TestCmpHandlerCrossFamilyRejects(t *testing.T) {
	_, err := CmpHandler([]Value{NewString("a"), NewInteger(1)}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "incomparable") {
		t.Errorf("cross-family cmp err = %v, want incomparable", err)
	}
}

func TestTcmpHandlerCrossFamilyOrders(t *testing.T) {
	// tcmp deliberately accepts cross-family pairs via the Rank order.
	out, err := TcmpHandler([]Value{NewString("a"), NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TcmpHandler: %v", err)
	}
	n, _ := AsInteger(out[0])
	if n == 0 {
		t.Error("tcmp collapsed distinct-family values to 0")
	}
	// Antisymmetry through the handler.
	out2, err := TcmpHandler([]Value{NewInteger(1), NewString("a")}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TcmpHandler flipped: %v", err)
	}
	n2, _ := AsInteger(out2[0])
	if n2 != -n {
		t.Errorf("tcmp not antisymmetric: %d vs %d", n, n2)
	}
}

func TestEqNeqDeqHandlers(t *testing.T) {
	eqOut, err := EqHandler([]Value{NewInteger(1), NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("EqHandler: %v", err)
	}
	if b, _ := AsBoolean(eqOut[0]); !b {
		t.Error("1 eq 1 = false")
	}
	neqOut, err := NeqHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("NeqHandler: %v", err)
	}
	if b, _ := AsBoolean(neqOut[0]); !b {
		t.Error("1 neq 2 = false")
	}
	l1 := NewList([]Value{NewInteger(1)})
	l2 := NewList([]Value{NewInteger(1)})
	deqOut, err := DeqHandler([]Value{l1, l2}, nil, nil, nil)
	if err != nil {
		t.Fatalf("DeqHandler: %v", err)
	}
	if b, _ := AsBoolean(deqOut[0]); !b {
		t.Error("structurally-equal lists deq = false")
	}
}

func TestRelationalHandlersNaNUnordered(t *testing.T) {
	nan := NewFloat(math.NaN())
	for name, h := range map[string]Handler{
		"lt": LtHandler, "gt": GtHandler, "lte": LteHandler, "gte": GteHandler,
	} {
		out, err := h([]Value{nan, NewFloat(1)}, nil, nil, nil)
		if err != nil {
			t.Fatalf("%s with NaN: %v", name, err)
		}
		if b, _ := AsBoolean(out[0]); b {
			t.Errorf("%s with NaN = true, want false (IEEE unordered)", name)
		}
	}
	// Sanity: without NaN the comparators still order.
	out, err := LtHandler([]Value{NewInteger(2), NewInteger(1)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("lt: %v", err)
	}
	if b, _ := AsBoolean(out[0]); !b {
		t.Error("1 lt 2 = false")
	}
}

// --- ExactEqual --------------------------------------------------------------

func TestExactEqualScalarsAndNone(t *testing.T) {
	if !ExactEqual(NewNone(), NewNone()) {
		t.Error("none != none")
	}
	if !ExactEqual(NewInteger(4), NewInteger(4)) || ExactEqual(NewInteger(4), NewInteger(5)) {
		t.Error("integer eq broken")
	}
	if !ExactEqual(NewString("x"), NewString("x")) || ExactEqual(NewString("x"), NewString("y")) {
		t.Error("string eq broken")
	}
	if !ExactEqual(NewBoolean(true), NewBoolean(true)) || ExactEqual(NewBoolean(true), NewBoolean(false)) {
		t.Error("boolean eq broken")
	}
	if !ExactEqual(NewAtom("a"), NewAtom("a")) || ExactEqual(NewAtom("a"), NewAtom("b")) {
		t.Error("atom eq broken")
	}
	// Cross-leaf numeric magnitude.
	if !ExactEqual(NewInteger(1), NewFloat(1.0)) {
		t.Error("1 != 1.0")
	}
	// Large int64 pair compares exactly (not via float projection).
	big1 := int64(1) << 60
	if ExactEqual(NewInteger(big1), NewInteger(big1+1)) {
		t.Error("distinct large integers compared equal")
	}
}

func TestExactEqualBigNumbers(t *testing.T) {
	bi := NewBigInteger(big.NewInt(7))
	if !ExactEqual(bi, NewInteger(7)) {
		t.Error("BigInteger 7 != Integer 7")
	}
	if ExactEqual(bi, NewInteger(8)) {
		t.Error("BigInteger 7 == Integer 8")
	}
	bd := NewBigDecimal(apd.New(1, -1)) // 0.1 exactly
	if ExactEqual(bd, NewFloat(0.1)) {
		t.Error("decimal 0.1 == binary float 0.1 (must be mathematically honest)")
	}
	if !ExactEqual(bd, NewBigDecimal(apd.New(1, -1))) {
		t.Error("0d0.1 != 0d0.1")
	}
	// NaN never equals a big value (toRatExact declines).
	if ExactEqual(NewFloat(math.NaN()), bd) {
		t.Error("NaN == BigDecimal")
	}
}

func TestExactEqualDepScalars(t *testing.T) {
	a := NewDepScalar(DepGTE, NewInteger(10))
	b := NewDepScalar(DepGTE, NewInteger(10))
	c := NewDepScalar(DepGT, NewInteger(10))
	d := NewDepScalar(DepLTE, NewInteger(10))
	if !ExactEqual(a, b) {
		t.Error("identical DepScalars unequal")
	}
	if ExactEqual(a, c) {
		t.Error("gte vs gt DepScalars equal")
	}
	if ExactEqual(a, d) {
		t.Error("lo vs hi DepScalars equal")
	}
	// DepScalar vs concrete scalar of the same base: never equal.
	if ExactEqual(a, NewInteger(10)) || ExactEqual(NewInteger(10), a) {
		t.Error("DepScalar equal to a concrete scalar")
	}
}

func TestExactEqualContainersByIdentity(t *testing.T) {
	l := NewList([]Value{NewInteger(1)})
	if !ExactEqual(l, l) {
		t.Error("list not eq itself")
	}
	if ExactEqual(l, NewList([]Value{NewInteger(1)})) {
		t.Error("distinct list literals eq")
	}
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	mv := NewMap(m)
	if !ExactEqual(mv, mv) {
		t.Error("map not eq itself")
	}
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(1))
	if ExactEqual(mv, NewMap(m2)) {
		t.Error("distinct map containers eq")
	}
	// Empty lists all alias the single empty list.
	if !ExactEqual(NewList(nil), NewList(nil)) {
		t.Error("empty lists not eq")
	}
}

func TestExactEqualFlatInstances(t *testing.T) {
	r := newTestRegistry(t)
	node := r.Types.MintType("W4bPt", TClass)
	fields := NewOrderedMap()
	fields.Set("x", NewInteger(1))
	ci := ClassInstanceInfo{Fields: fields}
	a := NewClassInstance(node, ci)
	b := NewClassInstance(node, ci) // shares the Fields pointer
	if !ExactEqual(a, b) {
		t.Error("aliased instances not eq")
	}
	other := NewOrderedMap()
	other.Set("x", NewInteger(1))
	c := NewClassInstance(node, ClassInstanceInfo{Fields: other})
	if ExactEqual(a, c) {
		t.Error("structurally-equal but distinct instances eq")
	}
}

func TestSameContainer(t *testing.T) {
	m := NewOrderedMap()
	mp := MapPayload{M: m}
	if !sameContainer(mp, MapPayload{M: m}) {
		t.Error("same *OrderedMap not identical")
	}
	if sameContainer(mp, MapPayload{M: NewOrderedMap()}) {
		t.Error("distinct maps identical")
	}
	elems := []Value{NewInteger(1)}
	lp := ListPayload{Elems: elems}
	if !sameContainer(lp, ListPayload{Elems: elems}) {
		t.Error("same backing array not identical")
	}
	if sameContainer(lp, ListPayload{Elems: []Value{NewInteger(1)}}) {
		t.Error("distinct backing arrays identical")
	}
	if sameContainer(lp, mp) {
		t.Error("list vs map payload identical")
	}
	if !sameContainer(ListPayload{}, ListPayload{}) {
		t.Error("two empty lists not identical")
	}
	if sameContainer(ListPayload{}, lp) {
		t.Error("empty vs non-empty identical")
	}
	if sameContainer(IntPayload{N: 1}, IntPayload{N: 1}) {
		t.Error("non-container payloads claimed identity")
	}
}

// --- DeepEqual ----------------------------------------------------------------

func TestDeepEqualLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewList([]Value{NewString("x")})})
	b := NewList([]Value{NewInteger(1), NewList([]Value{NewString("x")})})
	if !DeepEqual(a, b) {
		t.Error("deep-equal lists reported unequal")
	}
	c := NewList([]Value{NewInteger(1), NewList([]Value{NewString("y")})})
	if DeepEqual(a, c) {
		t.Error("differing nested lists reported equal")
	}
	if DeepEqual(a, NewList([]Value{NewInteger(1)})) {
		t.Error("length mismatch reported equal")
	}
	// Flex list deep-equals a plain list of the same content.
	if !DeepEqual(NewFlexList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(1)})) {
		t.Error("flex list != plain list of same content")
	}
}

// TestDeepEqualTypeLiterals pins NUR034: deq is reflexive on every bare
// type literal and agrees with eq — two literals are deq iff they name
// the same lattice type, so the container/root literals join the scalar
// ones. A mixed literal/concrete pair stays unequal (NUR032).
func TestDeepEqualTypeLiterals(t *testing.T) {
	for _, tc := range []struct {
		a, b Value
		want bool
	}{
		{NewTypeLiteral(TList), NewTypeLiteral(TList), true},
		{NewTypeLiteral(TMap), NewTypeLiteral(TMap), true},
		{NewTypeLiteral(TAny), NewTypeLiteral(TAny), true},
		{NewTypeLiteral(TInteger), NewTypeLiteral(TInteger), true},
		{NewTypeLiteral(TList), NewTypeLiteral(TMap), false},
		{NewTypeLiteral(TInteger), NewTypeLiteral(TFloat), false},
		{NewTypeLiteral(TList), NewInteger(0), false},    // mixed: not both bare
		{NewInteger(0), NewTypeLiteral(TInteger), false}, // NUR032 mixed guard
	} {
		if got := DeepEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("DeepEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestDeepEqualTypeLevelContainers pins the residual render-comparison
// path (NUR033): a type-level list/map operand — a Record/Options type
// (Parent=TMap) or a Table type (Parent=TList) — carries no readable
// entries, so deqMapEntries/deqListElems decline and DeepEqual falls
// back to comparing rendered forms. Two identical type constructors are
// deq; differing ones are not.
func TestDeepEqualTypeLevelContainers(t *testing.T) {
	rf := func(v int) Value {
		f := NewOrderedMap()
		f.Set("a", NewInteger(int64(v)))
		return NewRecordType(f)
	}
	if !DeepEqual(rf(1), rf(1)) {
		t.Error("identical record types must be deq (by render)")
	}
	if DeepEqual(rf(1), rf(2)) {
		t.Error("record types with differing fields must not be deq")
	}
	tt := func(v int) Value {
		f := NewOrderedMap()
		f.Set("a", NewInteger(int64(v)))
		return NewTableType(RecordTypeInfo{Fields: f})
	}
	if !DeepEqual(tt(1), tt(1)) {
		t.Error("identical table types must be deq (by render)")
	}
	if DeepEqual(tt(1), tt(2)) {
		t.Error("table types with differing record schemas must not be deq")
	}
}

func TestDeepEqualMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(1))
	if !DeepEqual(NewMap(m1), NewMap(m2)) {
		t.Error("deep-equal maps unequal")
	}
	m3 := NewOrderedMap()
	m3.Set("b", NewInteger(1))
	if DeepEqual(NewMap(m1), NewMap(m3)) {
		t.Error("different keys equal")
	}
	m4 := NewOrderedMap()
	m4.Set("a", NewInteger(1))
	m4.Set("b", NewInteger(2))
	if DeepEqual(NewMap(m1), NewMap(m4)) {
		t.Error("different sizes equal")
	}
	// NUR033: a typed map compares by its ENTRY VALUES, not by rendered
	// form — the element-type tag is ignored, exactly as deq ignores
	// flexness. Two EMPTY typed maps are the empty map regardless of
	// element type, and a populated typed map is deq-equal to the plain
	// map of the same entries.
	tm1 := NewTypedMap(NewTypeLiteral(TInteger))
	tm2 := NewTypedMap(NewTypeLiteral(TInteger))
	if !DeepEqual(tm1, tm2) {
		t.Error("same typed maps unequal")
	}
	if !DeepEqual(tm1, NewTypedMap(NewTypeLiteral(TString))) {
		t.Error("two empty typed maps must be deq-equal (both empty; tag ignored)")
	}
	if !DeepEqual(tm1, NewMap(NewOrderedMap())) {
		t.Error("an empty typed map must be deq-equal to the plain empty map")
	}
	pop := NewTypedMapWithEntries(NewTypeLiteral(TInteger), []ChildEntry{{Key: "a", Value: NewInteger(1)}})
	if !DeepEqual(pop, NewMap(m1)) {
		t.Error("a populated typed map must be deq-equal to the plain map of the same entries")
	}
	if DeepEqual(pop, NewMap(m3)) {
		t.Error("a populated typed map with different entries must not be equal")
	}
}

func TestDeepEqualDepScalarAndInstances(t *testing.T) {
	a := NewDepScalar(DepLT, NewInteger(9))
	b := NewDepScalar(DepLT, NewInteger(9))
	if !DeepEqual(a, b) {
		t.Error("identical DepScalars not deq")
	}
	if DeepEqual(a, NewInteger(3)) {
		t.Error("DepScalar deq concrete")
	}

	r := newTestRegistry(t)
	node := r.Types.MintType("W4bDeqPt", TClass)
	f1 := NewOrderedMap()
	f1.Set("x", NewInteger(1))
	f2 := NewOrderedMap()
	f2.Set("x", NewInteger(1))
	i1 := NewClassInstance(node, ClassInstanceInfo{Fields: f1})
	i2 := NewClassInstance(node, ClassInstanceInfo{Fields: f2})
	if !DeepEqual(i1, i2) {
		t.Error("structurally-equal instances not deq")
	}
	f3 := NewOrderedMap()
	f3.Set("x", NewInteger(2))
	if DeepEqual(i1, NewClassInstance(node, ClassInstanceInfo{Fields: f3})) {
		t.Error("differing fields deq")
	}
	// A different (exact) type is never deq even with equal fields.
	node2 := r.Types.MintType("W4bDeqPt2", TClass)
	if DeepEqual(i1, NewClassInstance(node2, ClassInstanceInfo{Fields: f2})) {
		t.Error("cross-type instances deq")
	}
	// Unsupported pair falls to false.
	if DeepEqual(NewInteger(1), NewList(nil)) {
		t.Error("integer deq list")
	}
}

// --- family Comparers ---------------------------------------------------------

func TestNumberCompareNaNTotalOrder(t *testing.T) {
	nan := NewFloat(math.NaN())
	if mustCmp(t, nan, NewFloat(1e300)) != 1 {
		t.Error("NaN does not sort greatest")
	}
	if mustCmp(t, NewFloat(1), nan) != -1 {
		t.Error("finite vs NaN not -1")
	}
	if mustCmp(t, nan, NewFloat(math.NaN())) != 0 {
		t.Error("NaN vs NaN not 0")
	}
}

func TestNumberCompareBigExact(t *testing.T) {
	bd := NewBigDecimal(apd.New(1, -1)) // 0.1
	if mustCmp(t, bd, NewBigDecimal(apd.New(2, -1))) != -1 {
		t.Error("0d0.1 vs 0d0.2 not -1")
	}
	if mustCmp(t, NewBigInteger(big.NewInt(5)), NewInteger(5)) != 0 {
		t.Error("BigInteger 5 vs 5 not 0")
	}
	// Binary float 0.1 is slightly ABOVE decimal 0.1.
	if mustCmp(t, NewFloat(0.1), bd) != 1 {
		t.Error("float 0.1 vs decimal 0.1 not +1")
	}
	// Non-finite float vs big value: IEEE total-order slots.
	if mustCmp(t, NewFloat(math.Inf(1)), bd) != 1 {
		t.Error("+Inf vs big not +1")
	}
	if mustCmp(t, NewFloat(math.Inf(-1)), bd) != -1 {
		t.Error("-Inf vs big not -1")
	}
	if mustCmp(t, bd, NewFloat(math.NaN())) != -1 {
		t.Error("big vs NaN not -1")
	}
}

func TestNonFiniteRankAndToRatExact(t *testing.T) {
	if _, nf := nonFiniteRank(NewInteger(1)); nf {
		t.Error("integer claimed non-finite")
	}
	if r, nf := nonFiniteRank(NewFloat(math.Inf(1))); !nf || r != 1 {
		t.Errorf("+Inf rank = %d,%v", r, nf)
	}
	if r, nf := nonFiniteRank(NewFloat(math.Inf(-1))); !nf || r != -1 {
		t.Errorf("-Inf rank = %d,%v", r, nf)
	}
	if _, nf := nonFiniteRank(NewFloat(2.5)); nf {
		t.Error("finite float claimed non-finite")
	}

	if _, ok := toRatExact(NewString("x")); ok {
		t.Error("string converted to rat")
	}
	if _, ok := toRatExact(NewFloat(math.NaN())); ok {
		t.Error("NaN converted to rat")
	}
	if rat, ok := toRatExact(NewInteger(3)); !ok || rat.Cmp(big.NewRat(3, 1)) != 0 {
		t.Errorf("toRatExact(3) = %v, %v", rat, ok)
	}
	if rat, ok := toRatExact(NewBigInteger(big.NewInt(4))); !ok || rat.Cmp(big.NewRat(4, 1)) != 0 {
		t.Errorf("toRatExact(big 4) = %v, %v", rat, ok)
	}
	if rat, ok := toRatExact(NewFloat(0.5)); !ok || rat.Cmp(big.NewRat(1, 2)) != 0 {
		t.Errorf("toRatExact(0.5) = %v, %v", rat, ok)
	}
	if rat, ok := toRatExact(NewBigDecimal(apd.New(25, -1))); !ok || rat.Cmp(big.NewRat(5, 2)) != 0 {
		t.Errorf("toRatExact(0d2.5) = %v, %v", rat, ok)
	}
}

func TestScalarFamilyComparers(t *testing.T) {
	if mustCmp(t, NewString("a"), NewString("b")) != -1 {
		t.Error("string order broken")
	}
	if mustCmp(t, NewBoolean(false), NewBoolean(true)) != -1 {
		t.Error("false not < true")
	}
	if mustCmp(t, NewBoolean(true), NewBoolean(true)) != 0 {
		t.Error("true != true")
	}
	if mustCmp(t, NewBoolean(true), NewBoolean(false)) != 1 {
		t.Error("true not > false")
	}
	if mustCmp(t, NewAtom("a"), NewAtom("b")) != -1 {
		t.Error("atom order broken")
	}
	if mustCmp(t, NewWord("aaa"), NewWord("bbb")) != -1 {
		t.Error("word order broken")
	}
}

func TestTypeLiteralFirstRule(t *testing.T) {
	// A bare type literal sorts strictly below every concrete inhabitant,
	// including the zero value.
	if mustCmp(t, NewTypeLiteral(TInteger), NewInteger(0)) != -1 {
		t.Error("Integer literal not below 0")
	}
	if mustCmp(t, NewString(""), NewTypeLiteral(TString)) != 1 {
		t.Error("'' not above String literal")
	}
	// Two literals order by lattice node.
	if mustCmp(t, NewTypeLiteral(TInteger), NewTypeLiteral(TInteger)) != 0 {
		t.Error("Integer literal != itself")
	}
	c := mustCmp(t, NewTypeLiteral(TInteger), NewTypeLiteral(TFloat))
	if c == 0 {
		t.Error("distinct scalar literals tie")
	}
	if -c != mustCmp(t, NewTypeLiteral(TFloat), NewTypeLiteral(TInteger)) {
		t.Error("literal order not antisymmetric")
	}
}

func TestComparePathons(t *testing.T) {
	// NUR012 natural order: volume → absolute-first → forward lex →
	// shorter-prefix-first (a directory sorts before its contents).
	p2 := NewPathon([]string{"a", "b"}, false)
	p3 := NewPathon([]string{"a", "b", "c"}, false)
	if mustCmp(t, p2, p3) != -1 {
		t.Error("prefix (directory) not first")
	}
	if mustCmp(t, p3, p2) != 1 {
		t.Error("longer path not after its prefix")
	}
	// Forward lexical per segment.
	pa := NewPathon([]string{"a"}, false)
	pb := NewPathon([]string{"b"}, false)
	if mustCmp(t, pa, pb) != -1 {
		t.Error("forward-lex segment order broken")
	}
	// Same segments: absolute before relative.
	rel := NewPathon([]string{"x"}, false)
	abs := NewPathon([]string{"x"}, true)
	if mustCmp(t, abs, rel) != -1 || mustCmp(t, rel, abs) != 1 {
		t.Error("abs/rel order broken")
	}
	if mustCmp(t, rel, NewPathon([]string{"x"}, false)) != 0 {
		t.Error("identical paths not 0")
	}
	// Volume is the first key: a driveless path sorts before a drive one
	// regardless of segments.
	drive := NewPathonVol("c:", []string{"a"}, true)
	if mustCmp(t, NewPathon([]string{"z"}, false), drive) != -1 {
		t.Error("driveless path not before a drive path")
	}
	// Type-literal-first within the Pathon family.
	if mustCmp(t, NewTypeLiteral(TPathon), rel) != -1 {
		t.Error("Pathon literal not below a concrete path")
	}
}

func TestDepScalarPairOrdersByCanonicalForm(t *testing.T) {
	a := NewDepScalar(DepGT, NewInteger(1))
	b := NewDepScalar(DepGT, NewInteger(2))
	c1 := mustCmp(t, a, b)
	c2 := mustCmp(t, b, a)
	if c1 == 0 || c2 == 0 || c1 == c2 {
		t.Errorf("DepScalar canonical order broken: %d, %d", c1, c2)
	}
	if mustCmp(t, a, NewDepScalar(DepGT, NewInteger(1))) != 0 {
		t.Error("identical DepScalars not 0")
	}
}

// --- structural tiebreak (compare_types.go) ------------------------------------

func TestCompareStructuralLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(3)})
	if mustCmp(t, a, b) != -1 {
		t.Error("[1 2] vs [1 3] not -1")
	}
	if mustCmp(t, a, NewList([]Value{NewInteger(1), NewInteger(2)})) != 0 {
		t.Error("equal lists not 0")
	}
	// Different sizes order by size before structure.
	if mustCmp(t, NewList([]Value{NewInteger(9)}), a) != -1 {
		t.Error("shorter list not first")
	}
}

func TestCompareStructuralMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(2))
	if mustCmp(t, NewMap(m1), NewMap(m2)) != -1 {
		t.Error("{a:1} vs {a:2} not -1")
	}
	// Same size, different keys: sorted-key comparison.
	m3 := NewOrderedMap()
	m3.Set("b", NewInteger(1))
	if mustCmp(t, NewMap(m1), NewMap(m3)) != -1 {
		t.Error("{a:1} vs {b:1} not -1")
	}
	m4 := NewOrderedMap()
	m4.Set("a", NewInteger(1))
	if mustCmp(t, NewMap(m1), NewMap(m4)) != 0 {
		t.Error("equal maps not 0")
	}
}

func TestCompareTypesTiebreaks(t *testing.T) {
	r := newTestRegistry(t)
	// Two user types minted under one parent share the external Rank and
	// depth — the name breaks the tie.
	ta := r.Types.MintType("W4bAaa", TInteger)
	tb := r.Types.MintType("W4bBbb", TInteger)
	if compareTypes(ta, tb) != -1 || compareTypes(tb, ta) != 1 {
		t.Error("name tiebreak broken")
	}
	if compareTypes(ta, ta) != 0 {
		t.Error("type not equal itself")
	}
	// Depth: the shallower (more general) type first.
	child := r.Types.MintType("W4bChild", ta)
	if compareTypes(ta, child) != -1 {
		t.Error("parent does not sort before its subtype")
	}
}

func TestTypeDepthAndRankFallbacks(t *testing.T) {
	// Ad-hoc types without cached Depth/Rank walk their chains.
	adhoc := &Type{c: &content{tmeta: &typeMeta{Name: "W4bAdhoc"}, parent: TInteger}}
	if got := typeDepth(adhoc); got != TInteger.Depth()+1 {
		t.Errorf("typeDepth(adhoc) = %d, want %d", got, TInteger.Depth()+1)
	}
	if rankOf(adhoc) != TInteger.Rank() {
		t.Error("rankOf fallback did not walk to the parent")
	}
	orphan := &Type{c: &content{tmeta: &typeMeta{Name: "W4bOrphan"}}}
	if rankOf(orphan) != 0 {
		t.Error("rank of chainless type not 0")
	}
	if typeDepth(nil) != 0 {
		t.Error("typeDepth(nil) != 0")
	}
}

// --- equal.go dispatch ----------------------------------------------------------

func TestValuesEqualDefaultShapes(t *testing.T) {
	// Carrier pairs are conservatively equal; literal pairs by identity.
	if !valuesEqualDefault(NewCarrier(TInteger), NewCarrier(TString)) {
		t.Error("carriers not conservatively equal")
	}
	if !valuesEqualDefault(NewTypeLiteral(TInteger), NewTypeLiteral(TInteger)) {
		t.Error("same literal unequal")
	}
	if valuesEqualDefault(NewTypeLiteral(TInteger), NewTypeLiteral(TString)) {
		t.Error("distinct literals equal")
	}
	// Literal vs concrete: never equal.
	if valuesEqualDefault(NewTypeLiteral(TInteger), NewInteger(1)) {
		t.Error("literal equal to concrete")
	}
	// Bounded Type: equal iff same bound, however built.
	if !valuesEqualDefault(NewBoundedType(TMap), NewBoundedType(TMap)) {
		t.Error("Map/t != Type of [Map]")
	}
	if valuesEqualDefault(NewBoundedType(TMap), NewBoundedType(TList)) {
		t.Error("Map/t == List/t")
	}
	// Typed lists compare children.
	if !valuesEqualDefault(NewTypedList(NewTypeLiteral(TInteger)), NewTypedList(NewTypeLiteral(TInteger))) {
		t.Error("same typed lists unequal")
	}
	if valuesEqualDefault(NewTypedList(NewTypeLiteral(TInteger)), NewTypedList(NewTypeLiteral(TString))) {
		t.Error("distinct typed lists equal")
	}
	// Typed list vs plain list.
	if valuesEqualDefault(NewTypedList(NewTypeLiteral(TInteger)), NewList(nil)) {
		t.Error("typed list equal to plain list")
	}
	// Record types compare field maps.
	f1 := NewOrderedMap()
	f1.Set("a", NewTypeLiteral(TInteger))
	f2 := NewOrderedMap()
	f2.Set("a", NewTypeLiteral(TInteger))
	if !valuesEqualDefault(NewRecordType(f1), NewRecordType(f2)) {
		t.Error("same record types unequal")
	}
	f3 := NewOrderedMap()
	f3.Set("a", NewTypeLiteral(TString))
	if valuesEqualDefault(NewRecordType(f1), NewRecordType(f3)) {
		t.Error("distinct record types equal")
	}
	// Record vs plain map.
	if valuesEqualDefault(NewRecordType(f1), NewMap(NewOrderedMap())) {
		t.Error("record equal to plain map")
	}
	// Plain lists element-wise.
	if !valuesEqualDefault(NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(1)})) {
		t.Error("equal lists unequal")
	}
	if valuesEqualDefault(NewList([]Value{NewInteger(1)}), NewList([]Value{NewString("1")})) {
		t.Error("cross-family elements equal")
	}
	// Micron payloads: content equality; mixed kinds unequal. Built
	// from the kernel payload directly — the Emailon VALIDATOR is
	// content (basic/go/micron.go), but the payload and its equality
	// are kernel machinery, which is what this test pins.
	emailon := func() Value {
		om := NewOrderedMap()
		om.Set("user", NewString("a"))
		om.Set("host", NewString("b.co"))
		return NewValueRaw(TEmailon, MicronPayload{Fields: om})
	}
	e1, e2 := emailon(), emailon()
	if !valuesEqualDefault(e1, e2) {
		t.Error("equal emailons unequal")
	}
	if valuesEqualDefault(e1, NewInteger(1)) || valuesEqualDefault(NewInteger(1), e1) {
		t.Error("emailon equal to integer")
	}
	// Pathon content equality.
	if !valuesEqualDefault(NewPathon([]string{"a"}, true), NewPathon([]string{"a"}, true)) {
		t.Error("equal pathons unequal")
	}
	if valuesEqualDefault(NewPathon([]string{"a"}, true), NewInteger(2)) ||
		valuesEqualDefault(NewInteger(2), NewPathon([]string{"a"}, true)) {
		t.Error("pathon equal to integer")
	}
}

func TestListsEqualHelper(t *testing.T) {
	if !listsEqual([]Value{NewInteger(1)}, []Value{NewInteger(1)}) {
		t.Error("equal slices unequal")
	}
	if listsEqual([]Value{NewInteger(1)}, []Value{NewInteger(1), NewInteger(2)}) {
		t.Error("length mismatch equal")
	}
	if listsEqual([]Value{NewInteger(1)}, []Value{NewInteger(2)}) {
		t.Error("distinct elements equal")
	}
}
