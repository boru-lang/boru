package native

import (
	"bytes"
	"strings"
	"testing"
)

// ========================
// Query words are module-only
// ========================

// TestQueryWordsModuleOnly pins the decision that the SQL-flavoured
// query DSL lives exclusively in the boru:query module sub-registry:
// its words are NOT in the global registry, so unqualified use is an
// undefined_word error, never a silent partial query. (The module
// surface itself is covered in lang/go/modules/query_test.go.)
func TestQueryWordsModuleOnly(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	for _, w := range []string{"from", "select", "where", "star"} {
		e := New(r)
		_, err := e.Run([]Value{NewWord(w)})
		if err == nil {
			t.Errorf("global %q: expected undefined_word error, got nil", w)
			continue
		}
		if !strings.Contains(err.Error(), "undefined word") {
			t.Errorf("global %q: expected undefined_word, got %v", w, err)
		}
	}
}

// ========================
// Unify tests (disjunct, equality, open map)
// ========================

func TestUnifyDisjunctMatch(t *testing.T) {
	// string|none should unify with string "hello"
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewString("hello")
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.Parent.ConformsTo(TString) {
		t.Errorf("expected string type, got %s", result.Parent)
	}
}

func TestUnifyDisjunctNone(t *testing.T) {
	// string|none should unify with none
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewTypeLiteral(TNone)
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !IsNoneShape(result) {
		t.Errorf("expected none type, got %s", result.Parent)
	}
}

func TestUnifyDisjunctFail(t *testing.T) {
	// string|none should NOT unify with integer
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewInteger(42)
	_, ok := Unify(disj, val)
	if ok {
		t.Fatal("expected unification to fail")
	}
}

func TestUnifyDisjunctWithAny(t *testing.T) {
	// string|none unifying with any should preserve the disjunct
	disj := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)})
	val := NewTypeLiteral(TAny)
	result, ok := Unify(disj, val)
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !IsDisjunct(result) {
		t.Errorf("expected disjunct, got %s", result)
	}
}

func TestUnifyDisjunctMapOpen(t *testing.T) {
	// Disjunct with map alternative uses open (subset) matching
	patternMap := NewOrderedMap()
	patternMap.Set("x", NewTypeLiteral(TNumber))
	pattern := NewMap(patternMap)

	candidateMap := NewOrderedMap()
	candidateMap.Set("x", NewInteger(42))
	candidateMap.Set("y", NewString("extra"))
	candidate := NewMap(candidateMap)

	disj := NewDisjunct([]Value{pattern})
	result, ok := Unify(disj, candidate)
	if !ok {
		t.Fatal("expected open map unification to succeed")
	}
	if !result.Parent.Equal(TMap) {
		t.Errorf("expected map, got %s", result.Parent)
	}
}

func TestValuesEqualLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})
	if !ValuesEqual(a, b) {
		t.Error("expected equal lists")
	}
}

func TestValuesEqualListsDiff(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(3)})
	if ValuesEqual(a, b) {
		t.Error("expected unequal lists")
	}
}

func TestValuesEqualMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(1))
	if !ValuesEqual(NewMap(m1), NewMap(m2)) {
		t.Error("expected equal maps")
	}
}

func TestValuesEqualMapsDiffKeys(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("b", NewInteger(1))
	if ValuesEqual(NewMap(m1), NewMap(m2)) {
		t.Error("expected unequal maps")
	}
}

func TestValuesEqualTableTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TNumber))

	tt1 := Value{Parent: TList, Data: TableTypeInfo{Record: RecordTypeInfo{Fields: f1}}}
	tt2 := Value{Parent: TList, Data: TableTypeInfo{Record: RecordTypeInfo{Fields: f2}}}
	if !ValuesEqual(tt1, tt2) {
		t.Error("expected equal table types")
	}
}

func TestValuesEqualTypedLists(t *testing.T) {
	tl1 := NewTypedList(NewTypeLiteral(TString))
	tl2 := NewTypedList(NewTypeLiteral(TString))
	if !ValuesEqual(tl1, tl2) {
		t.Error("expected equal typed lists")
	}
}

func TestValuesEqualTypedListsDiff(t *testing.T) {
	tl1 := NewTypedList(NewTypeLiteral(TString))
	tl2 := NewTypedList(NewTypeLiteral(TNumber))
	if ValuesEqual(tl1, tl2) {
		t.Error("expected unequal typed lists")
	}
}

func TestValuesEqualRecordTypes(t *testing.T) {
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TNumber))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TNumber))
	rt1 := NewRecordType(f1)
	rt2 := NewRecordType(f2)
	if !ValuesEqual(rt1, rt2) {
		t.Error("expected equal record types")
	}
}

func TestValuesEqualTypedMaps(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TString))
	tm2 := NewTypedMap(NewTypeLiteral(TString))
	if !ValuesEqual(tm1, tm2) {
		t.Error("expected equal typed maps")
	}
}

func TestValuesEqualTypedMapsDiff(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TString))
	tm2 := NewTypedMap(NewTypeLiteral(TNumber))
	if ValuesEqual(tm1, tm2) {
		t.Error("expected unequal typed maps")
	}
}

func TestValuesEqualOneNilData(t *testing.T) {
	a := NewTypeLiteral(TString)
	b := NewString("hello")
	if ValuesEqual(a, b) {
		t.Error("expected unequal (type literal vs concrete)")
	}
}

func TestOpenUnifyMap(t *testing.T) {
	pattern := NewOrderedMap()
	pattern.Set("x", NewTypeLiteral(TNumber))
	candidate := NewOrderedMap()
	candidate.Set("x", NewInteger(5))
	candidate.Set("y", NewString("extra"))
	if !OpenUnifyMap(NewMap(pattern), NewMap(candidate)) {
		t.Error("expected open unify to succeed")
	}
}

// A typed-map pattern (`m:{:Integer}` fn param) has ChildTypeInfo
// data, not an OrderedMap — OpenUnifyMap must route it through the
// unifier instead of panicking on a nil Keys() walk.
func TestOpenUnifyMapTypedMapPatternNoPanic(t *testing.T) {
	pattern := NewTypedMap(NewTypeLiteral(TInteger))
	candidate := NewOrderedMap()
	candidate.Set("a", NewInteger(1))
	if !OpenUnifyMap(pattern, NewMap(candidate)) {
		t.Error("typed-map pattern should admit a conforming concrete map")
	}
	bad := NewOrderedMap()
	bad.Set("a", NewString("no"))
	if OpenUnifyMap(pattern, NewMap(bad)) {
		t.Error("typed-map pattern should reject an off-type value")
	}
}

func TestOpenUnifyMapMissingKey(t *testing.T) {
	pattern := NewOrderedMap()
	pattern.Set("z", NewTypeLiteral(TNumber))
	candidate := NewOrderedMap()
	candidate.Set("x", NewInteger(5))
	if OpenUnifyMap(NewMap(pattern), NewMap(candidate)) {
		t.Error("expected open unify to fail")
	}
}

func TestUnifyTypedMaps(t *testing.T) {
	tm1 := NewTypedMap(NewTypeLiteral(TNumber))
	tm2 := NewTypedMap(NewTypeLiteral(TInteger))
	result, ok := Unify(tm1, tm2)
	if !ok {
		t.Fatal("expected typed map unification to succeed")
	}
	if !IsTypedMap(result) {
		t.Errorf("expected typed map, got %s", result)
	}
}

func TestUnifyTypedMapWithConcrete(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber))
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Set("y", NewInteger(2))
	result, ok := Unify(tm, NewMap(m))
	if !ok {
		t.Fatal("expected unification to succeed")
	}
	if !result.Parent.Equal(TMap) {
		t.Errorf("expected map, got %s", result.Parent)
	}
}

func TestUnifyMapsClosedFail(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("y", NewInteger(2))
	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("expected closed map unification to fail (different key counts)")
	}
}

func TestUnifyMapsKeyMismatch(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("y", NewInteger(1))
	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("expected closed map unification to fail (different keys)")
	}
}

func TestUnifyMapWithNonMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	_, ok := Unify(NewMap(m), NewInteger(1))
	if ok {
		t.Error("expected map+int unification to fail")
	}
}

func TestUnifyListLiteralWithConcrete(t *testing.T) {
	listType := NewTypeLiteral(TList)
	concrete := NewList([]Value{NewInteger(1)})
	result, ok := Unify(listType, concrete)
	if !ok {
		t.Fatal("expected list type to unify with concrete list")
	}
	if !result.Parent.Equal(TList) {
		t.Errorf("expected list, got %s", result.Parent)
	}
}

func TestUnifyMapLiteralWithConcrete(t *testing.T) {
	mapType := NewTypeLiteral(TMap)
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	result, ok := Unify(mapType, NewMap(m))
	if !ok {
		t.Fatal("expected map type to unify with concrete map")
	}
	if !result.Parent.Equal(TMap) {
		t.Errorf("expected map, got %s", result.Parent)
	}
}

// ========================
// Dot / Dotr tests
// ========================

func TestDotMapAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	result := runBoru(t, r, []Value{NewMap(m), NewAtom("x"), NewWord("dot")})
	_as0, _ := AsInteger(result[0])
	if len(result) != 1 || _as0 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestDotMapString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runBoru(t, r, []Value{NewMap(m), NewString("key"), NewWord("get")})
	_as1, _ := AsInteger(result[0])
	if len(result) != 1 || _as1 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestDotListIndex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewString("a"), NewString("b"), NewString("c")})
	result := runBoru(t, r, []Value{list, NewInteger(1), NewWord("get")})
	_as2, _ := AsString(result[0])
	if len(result) != 1 || _as2 != "b" {
		t.Errorf("expected 'b', got %v", result)
	}
}

func TestDotListOutOfBounds(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewString("a")})
	result := runBoru(t, r, []Value{list, NewInteger(5), NewWord("get")})
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none, got %v", result)
	}
}

func TestDotMapMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	result := runBoru(t, r, []Value{NewMap(m), NewAtom("y"), NewWord("dot")})
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none for missing key, got %v", result)
	}
}

func TestDotMapIntegerKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("0", NewString("zero"))
	result := runBoru(t, r, []Value{NewMap(m), NewInteger(0), NewWord("get")})
	_as3, _ := AsString(result[0])
	if len(result) != 1 || _as3 != "zero" {
		t.Errorf("expected 'zero', got %v", result)
	}
}

func TestDotNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{NewTypeLiteral(TNone), NewAtom("x"), NewWord("dot")})
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none, got %v", result)
	}
}

// --- dot: list access ---

func TestDotListByIndex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)})
	result := runBoru(t, r, []Value{list, NewInteger(1), NewWord("get")})
	_as4, _ := AsInteger(result[0])
	if len(result) != 1 || _as4 != 20 {
		t.Errorf("expected 20, got %v", result)
	}
}

func TestDotListAtomKeyReturnsNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewInteger(10), NewInteger(20)})
	result := runBoru(t, r, []Value{list, NewAtom("x"), NewWord("dot")})
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none for atom key on list, got %v", result)
	}
}

func TestDotListStringKeyReturnsNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewInteger(10)})
	result := runBoru(t, r, []Value{list, NewString("x"), NewWord("get")})
	if len(result) != 1 || !IsNoneShape(result[0]) {
		t.Errorf("expected none for string key on list, got %v", result)
	}
}

// --- dot: nested map access (chained) ---

func TestDotNestedMapChain(t *testing.T) {
	// {a:{b:{c:1}}} dot a dot b dot c → 1
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	inner := NewOrderedMap()
	inner.Set("c", NewInteger(1))
	mid := NewOrderedMap()
	mid.Set("b", NewMap(inner))
	outer := NewOrderedMap()
	outer.Set("a", NewMap(mid))
	// Chained forward gets: map get a get b get c
	result := runBoru(t, r, []Value{
		NewMap(outer),
		NewWord("dot"), NewWord("a"),
		NewWord("dot"), NewWord("b"),
		NewWord("dot"), NewWord("c"),
	})
	_as5, _ := AsInteger(result[0])
	if len(result) != 1 || _as5 != 1 {
		t.Errorf("expected 1, got %v", result)
	}
}

// --- dot: map containing list ---

func TestDotMapThenList(t *testing.T) {
	// {items:[10 20 30]} dot items dot 1 → 20
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("items", NewList([]Value{NewInteger(10), NewInteger(20), NewInteger(30)}))
	result := runBoru(t, r, []Value{
		NewMap(m),
		NewAtom("items"), NewWord("dot"),
		NewInteger(1), NewWord("get"),
	})
	_as6, _ := AsInteger(result[0])
	if len(result) != 1 || _as6 != 20 {
		t.Errorf("expected 20, got %v", result)
	}
}

// --- dot: list containing maps ---

func TestDotListThenMap(t *testing.T) {
	// [{x:1} {x:2}] dot 0 dot x → 1
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m0 := NewOrderedMap()
	m0.Set("x", NewInteger(1))
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(2))
	list := NewList([]Value{NewMap(m0), NewMap(m1)})
	result := runBoru(t, r, []Value{
		list,
		NewInteger(0), NewWord("dot"),
		NewAtom("x"), NewWord("dot"),
	})
	_as7, _ := AsInteger(result[0])
	if len(result) != 1 || _as7 != 1 {
		t.Errorf("expected 1, got %v", result)
	}
}

func TestDotListThenMapSecondElement(t *testing.T) {
	// [{x:1} {x:2}] dot 1 dot x → 2
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m0 := NewOrderedMap()
	m0.Set("x", NewInteger(1))
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(2))
	list := NewList([]Value{NewMap(m0), NewMap(m1)})
	result := runBoru(t, r, []Value{
		list,
		NewInteger(1), NewWord("dot"),
		NewAtom("x"), NewWord("dot"),
	})
	_as8, _ := AsInteger(result[0])
	if len(result) != 1 || _as8 != 2 {
		t.Errorf("expected 2, got %v", result)
	}
}

// --- dot: . alias works identically ---

func TestDotAliasMapAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runBoru(t, r, []Value{NewMap(m), NewAtom("key"), NewWord("dot")})
	_as9, _ := AsInteger(result[0])
	if len(result) != 1 || _as9 != 99 {
		t.Errorf("expected 99 via . alias, got %v", result)
	}
}

func TestDotAliasListAccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewString("a"), NewString("b")})
	result := runBoru(t, r, []Value{list, NewInteger(0), NewWord("get")})
	_as10, _ := AsString(result[0])
	if len(result) != 1 || _as10 != "a" {
		t.Errorf("expected 'a' via . alias, got %v", result)
	}
}

// --- dot: deeply nested list/map combo ---

func TestDotDeepListMapCombo(t *testing.T) {
	// {a:[{b:[100 200]} {b:[300 400]}]} dot a dot 1 dot b dot 0 → 300
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	b0 := NewList([]Value{NewInteger(100), NewInteger(200)})
	b1 := NewList([]Value{NewInteger(300), NewInteger(400)})
	m0 := NewOrderedMap()
	m0.Set("b", b0)
	m1 := NewOrderedMap()
	m1.Set("b", b1)
	outer := NewOrderedMap()
	outer.Set("a", NewList([]Value{NewMap(m0), NewMap(m1)}))
	result := runBoru(t, r, []Value{
		NewMap(outer),
		NewAtom("a"), NewWord("dot"),
		NewInteger(1), NewWord("dot"),
		NewAtom("b"), NewWord("dot"),
		NewInteger(0), NewWord("get"),
	})
	_as11, _ := AsInteger(result[0])
	if len(result) != 1 || _as11 != 300 {
		t.Errorf("expected 300, got %v", result)
	}
}

func TestDotrMapSuccess(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(42))
	result := runBoru(t, r, []Value{NewMap(m), NewAtom("x"), NewWord("dotr")})
	_as12, _ := AsInteger(result[0])
	if len(result) != 1 || _as12 != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestDotrMapMissingError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	err = runBoruError(t, r, []Value{NewMap(m), NewAtom("y"), NewWord("dotr")})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestDotrNoneError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	err = runBoruError(t, r, []Value{NewTypeLiteral(TNone), NewAtom("x"), NewWord("dotr")})
	if err == nil {
		t.Fatal("expected error for none parent")
	}
	if !strings.Contains(err.Error(), "None") {
		t.Errorf("expected 'None' error, got: %v", err)
	}
}

func TestDotrListOutOfBounds(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	list := NewList([]Value{NewString("a")})
	err = runBoruError(t, r, []Value{list, NewInteger(5), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for out of bounds")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("expected 'out of bounds' error, got: %v", err)
	}
}

func TestDotrMapStringKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("key", NewInteger(99))
	result := runBoru(t, r, []Value{NewMap(m), NewString("key"), NewWord("getr")})
	_as13, _ := AsInteger(result[0])
	if len(result) != 1 || _as13 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestDotrMapStringMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	err = runBoruError(t, r, []Value{NewMap(m), NewString("nope"), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for missing string key")
	}
}

func TestDotrMapIntegerKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("5", NewString("five"))
	result := runBoru(t, r, []Value{NewMap(m), NewInteger(5), NewWord("getr")})
	_as14, _ := AsString(result[0])
	if len(result) != 1 || _as14 != "five" {
		t.Errorf("expected 'five', got %v", result)
	}
}

func TestDotrMapIntegerKeyMissing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("0", NewString("zero"))
	err = runBoruError(t, r, []Value{NewMap(m), NewInteger(9), NewWord("getr")})
	if err == nil {
		t.Fatal("expected error for missing integer key")
	}
}

// ========================
// Make / Record tests
// ========================

func TestMakeRecordPositional(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// def Point type Record [x:number y:number]
	// make Point [1 2]
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Point"),
		NewWord("refine"), NewWord("Record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Point"),
		NewList([]Value{NewInteger(1), NewInteger(2)}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	m, _ := AsMap(result[0])
	xVal, _ := m.Get("x")
	yVal, _ := m.Get("y")
	_as16, _ := AsInteger(xVal)
	_as15, _ := AsInteger(yVal)
	if _as16 != 1 || _as15 != 2 {
		t.Errorf("expected {x:1,y:2}, got %v", result[0])
	}
}

func singleMap(key string, val Value) *OrderedMap {
	m := NewOrderedMap()
	m.Set(key, val)
	return m
}

func TestMakeRecordNamed(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Pt"),
		NewWord("refine"), NewWord("Record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Pt"),
		NewList([]Value{
			NewMap(singleMap("y", NewInteger(20))),
			NewMap(singleMap("x", NewInteger(10))),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	xVal, _ := m.Get("x")
	yVal, _ := m.Get("y")
	_as18, _ := AsInteger(xVal)
	_as17, _ := AsInteger(yVal)
	if _as18 != 10 || _as17 != 20 {
		t.Errorf("expected {x:10,y:20}, got %v", result[0])
	}
}

func TestMakeRecordMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	src := NewOrderedMap()
	src.Set("x", NewInteger(5))
	src.Set("y", NewInteger(6))
	result := runBoru(t, r, []Value{
		NewWord("def"), NewWord("Pt2"),
		NewWord("refine"), NewWord("Record"), NewList([]Value{
			NewMap(singleMap("x", NewTypeLiteral(TNumber))),
			NewMap(singleMap("y", NewTypeLiteral(TNumber))),
		}),
		NewWord("make"), NewWord("Pt2"), NewMap(src),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// ========================
// Convert tests
// ========================

func TestConvertIntToString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(42), NewWord("convert"), NewWord("String"),
	})
	_as19, _ := AsString(result[0])
	if len(result) != 1 || _as19 != "42" {
		t.Errorf("expected '42', got %v", result)
	}
}

func TestConvertIntToStringHex(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	hexOpts := NewOrderedMap()
	hexOpts.Set("base", NewString("hex"))
	result := runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(hexOpts),
	})
	_as20, _ := AsString(result[0])
	if len(result) != 1 || _as20 != "ff" {
		t.Errorf("expected 'ff', got %v", result)
	}
}

func TestConvertIntToStringBin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	binOpts := NewOrderedMap()
	binOpts.Set("base", NewString("bin"))
	result := runBoru(t, r, []Value{
		NewInteger(10), NewWord("convert"), NewWord("String"), NewMap(binOpts),
	})
	_as21, _ := AsString(result[0])
	if len(result) != 1 || _as21 != "1010" {
		t.Errorf("expected '1010', got %v", result)
	}
}

func TestConvertIntToStringOct(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	octOpts := NewOrderedMap()
	octOpts.Set("base", NewString("oct"))
	result := runBoru(t, r, []Value{
		NewInteger(8), NewWord("convert"), NewWord("String"), NewMap(octOpts),
	})
	_as22, _ := AsString(result[0])
	if len(result) != 1 || _as22 != "10" {
		t.Errorf("expected '10', got %v", result)
	}
}

func TestConvertStringToNumber(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewString("99"), NewWord("convert"), NewWord("Number"),
	})
	_as23, _ := AsInteger(result[0])
	if len(result) != 1 || _as23 != 99 {
		t.Errorf("expected 99, got %v", result)
	}
}

func TestConvertBoolToString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewBoolean(true), NewWord("convert"), NewWord("String"),
	})
	_as24, _ := AsString(result[0])
	if len(result) != 1 || _as24 != "true" {
		t.Errorf("expected 'true', got %v", result)
	}
}

func TestConvertIntToBool(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(1), NewWord("convert"), NewWord("Boolean"),
	})
	_as25, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as25 {
		t.Errorf("expected true, got %v", result)
	}
}

func TestConvertIntToBoolZero(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewInteger(0), NewWord("convert"), NewWord("Boolean"),
	})
	_as26, _ := AsBoolean(result[0])
	if len(result) != 1 || _as26 {
		t.Errorf("expected false, got %v", result)
	}
}

func TestConvertStringToBool(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewString("true"), NewWord("convert"), NewWord("Boolean"),
	})
	_as27, _ := AsBoolean(result[0])
	if len(result) != 1 || !_as27 {
		t.Errorf("expected true, got %v", result)
	}
}

func TestConvertWithSettingsMap(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	settings := NewOrderedMap()
	settings.Set("base", NewString("hex"))
	result := runBoru(t, r, []Value{
		NewInteger(255), NewWord("convert"), NewWord("String"), NewMap(settings),
	})
	_as28, _ := AsString(result[0])
	if len(result) != 1 || _as28 != "ff" {
		t.Errorf("expected 'ff', got %v", result)
	}
}

// ========================
// Var edge cases
// ========================

// ========================
// Print / format tests
// ========================

func TestFormatForPrintString(t *testing.T) {
	out := FormatForPrint(NewString("hello"))
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

func TestFormatForPrintInteger(t *testing.T) {
	out := FormatForPrint(NewInteger(42))
	if out != "42" {
		t.Errorf("expected '42', got %q", out)
	}
}

func TestFormatForPrintBoolean(t *testing.T) {
	out := FormatForPrint(NewBoolean(true))
	if out != "true" {
		t.Errorf("expected 'true', got %q", out)
	}
}

func TestFormatForPrintMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	out := FormatForPrint(NewMap(m))
	if !strings.Contains(out, "\"x\"") || !strings.Contains(out, "1") {
		t.Errorf("expected JSON-like map, got %q", out)
	}
}

func TestFormatForPrintList(t *testing.T) {
	list := NewList([]Value{NewInteger(1), NewString("a")})
	out := FormatForPrint(list)
	if !strings.Contains(out, "1") || !strings.Contains(out, "\"a\"") {
		t.Errorf("expected JSON-like list, got %q", out)
	}
}

func TestFormatForPrintTable(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}
	row := NewOrderedMap()
	row.Set("name", NewString("alice"))
	td := TableData{Record: rec, Rows: []Value{NewMap(row)}}
	out := FormatForPrint(Value{Parent: TList, Data: td})
	if !strings.Contains(out, "name") || !strings.Contains(out, "alice") {
		t.Errorf("expected table with 'name' and 'alice', got %q", out)
	}
}

func TestFormatForPrintEmptyTable(t *testing.T) {
	rec := RecordTypeInfo{Fields: NewOrderedMap()}
	td := TableData{Record: rec, Rows: nil}
	out := FormatForPrint(Value{Parent: TList, Data: td})
	if out != "(empty table)" {
		t.Errorf("expected '(empty table)', got %q", out)
	}
}

func TestFormatValueJSONNone(t *testing.T) {
	out := FormatValueJSON(NewTypeLiteral(TNone))
	if out != "null" {
		t.Errorf("expected 'null', got %q", out)
	}
}

func TestFormatValueJSONBoolFalse(t *testing.T) {
	out := FormatValueJSON(NewBoolean(false))
	if out != "false" {
		t.Errorf("expected 'false', got %q", out)
	}
}

func TestFormatValueJSONNestedMap(t *testing.T) {
	inner := NewOrderedMap()
	inner.Set("a", NewInteger(1))
	outer := NewOrderedMap()
	outer.Set("m", NewMap(inner))
	out := FormatValueJSON(NewMap(outer))
	if !strings.Contains(out, "\"m\"") || !strings.Contains(out, "\"a\"") {
		t.Errorf("expected nested map, got %q", out)
	}
}

func TestPadRight(t *testing.T) {
	if PadRight("hi", 5) != "hi   " {
		t.Error("expected padding")
	}
	if PadRight("hello", 3) != "hello" {
		t.Error("expected no truncation")
	}
}

// ========================
// Trace tests
// ========================

func TestTraceColorize(t *testing.T) {
	cases := []struct {
		val  Value
		want string
	}{
		{NewWord("add"), "add"},
		{NewWordModified("x", -1, true, false), "x/s"},
		{NewWordModified("x", -1, false, true), "x/f"},
		{NewString("hi"), `"hi"`},
		{NewInteger(42), "42"},
		{NewBoolean(true), "true"},
		{NewBoolean(false), "false"},
		{NewAtom("foo"), "foo"},
		{NewTypeLiteral(TNumber), "Number"},
		{NewList([]Value{NewInteger(1)}), "1"},
	}
	for _, tc := range cases {
		got := TraceColorize(tc.val)
		// Strip ANSI codes
		visible := stripANSI(got)
		if !strings.Contains(visible, tc.want) {
			t.Errorf("TraceColorize(%s) visible = %q, want contains %q", tc.val, visible, tc.want)
		}
	}
}

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func TestTraceColorizeMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	got := TraceColorize(NewMap(m))
	visible := stripANSI(got)
	if !strings.Contains(visible, "x") || !strings.Contains(visible, "1") {
		t.Errorf("expected map colorize to contain 'x' and '1', got %q", visible)
	}
}

func TestTraceColorizeForward(t *testing.T) {
	fwd := NewForward(ForwardInfo{FuncName: "add", CollectedArgs: 1, ExpectedArgs: 2})
	got := TraceColorize(fwd)
	visible := stripANSI(got)
	if !strings.Contains(visible, "add") || !strings.Contains(visible, "1/2") {
		t.Errorf("expected forward colorize, got %q", visible)
	}
}

func TestTraceColorizeOpenParen(t *testing.T) {
	got := TraceColorize(NewOpenParen())
	visible := stripANSI(got)
	if !strings.Contains(visible, "(") {
		t.Errorf("expected '(', got %q", visible)
	}
}

func TestTraceVisibleLen(t *testing.T) {
	if TraceVisibleLen("hello") != 5 {
		t.Error("plain string length wrong")
	}
	if TraceVisibleLen("\033[31mhello\033[0m") != 5 {
		t.Error("ANSI-wrapped length wrong")
	}
}

func TestRunTrace(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf
	tokens := []Value{NewInteger(1), NewWord("add"), NewInteger(2)}
	result, err := RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatalf("RunTrace error: %v", err)
	}
	_as31, _ := AsInteger(result[0])
	if len(result) != 1 || _as31 != 3 {
		t.Errorf("expected [3], got %v", result)
	}
	out := buf.String()
	if !strings.Contains(out, "trace") {
		t.Errorf("expected trace output, got %q", out)
	}
	if !strings.Contains(out, "result") {
		t.Errorf("expected result in output, got %q", out)
	}
}

func TestRunTraceLong(t *testing.T) {
	// Force multi-line wrapping by using a long expression
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	var buf bytes.Buffer
	r.Output = &buf
	tokens := make([]Value, 0)
	for i := 0; i < 30; i++ {
		tokens = append(tokens, NewInteger(int64(i)))
	}
	_, err = RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatalf("RunTrace error: %v", err)
	}
}

func TestTraceWrapEmpty(t *testing.T) {
	lines := TraceWrap(nil, 0, 80)
	if len(lines) != 1 || !strings.Contains(stripANSI(lines[0]), "[ ]") {
		t.Errorf("expected [ ] for empty, got %v", lines)
	}
}
