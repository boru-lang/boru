package native

import (
	"fmt"
	"testing"
	"time"
)

func mustTestType(t *testing.T, path string) *Type {
	t.Helper()
	return MintTestType(path)
}

// --- Multi-level type construction and String() ---

func TestNewTypeMultiLevel(t *testing.T) {
	tests := []struct {
		path  string
		parts int
	}{
		{"A", 1},
		{"A/B", 2},
		{"A/B/C", 3},
		{"A/B/C/D", 4},
		{"A/B/C/D/E", 5},
		{"A/B/C/D/E/F", 6},
		{"A/B/C/D/E/F/G", 7},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			typ := mustTestType(t, tt.path)
			if typ.Specificity() != tt.parts {
				t.Errorf("NewType(%q).Specificity() = %d, want %d", tt.path, typ.Specificity(), tt.parts)
			}
			if typ.Path() != tt.path {
				t.Errorf("Path() = %q, want %q", typ.Path(), tt.path)
			}
		})
	}
}

// --- Specificity scales with depth ---

func TestSpecificityScalesWithDepth(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "A"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/%c", 'A'+i)
		}
		typ := mustTestType(t, path)
		if typ.Specificity() != depth {
			t.Errorf("depth %d: Specificity() = %d, want %d", depth, typ.Specificity(), depth)
		}
	}
}

// --- Supertype matches subtype (child matches parent pattern) ---

func TestSupertypeMatchesSubtype(t *testing.T) {
	levels := []string{
		"Animal",
		"Animal/Mammal",
		"Animal/Mammal/Canine",
		"Animal/Mammal/Canine/Dog",
		"Animal/Mammal/Canine/Dog/Labrador",
		"Animal/Mammal/Canine/Dog/Labrador/Golden",
		"Animal/Mammal/Canine/Dog/Labrador/Golden/Champion",
	}

	for i := 0; i < len(levels); i++ {
		child := mustTestType(t, levels[i])
		for j := 0; j <= i; j++ {
			parent := mustTestType(t, levels[j])
			if !child.ConformsTo(parent) {
				t.Errorf("%q should match parent pattern %q", levels[i], levels[j])
			}
		}
	}
}

// --- Parent does NOT match child pattern ---

func TestParentDoesNotMatchChildPattern(t *testing.T) {
	levels := []string{
		"Animal",
		"Animal/Mammal",
		"Animal/Mammal/Canine",
		"Animal/Mammal/Canine/Dog",
		"Animal/Mammal/Canine/Dog/Labrador",
		"Animal/Mammal/Canine/Dog/Labrador/Golden",
		"Animal/Mammal/Canine/Dog/Labrador/Golden/Champion",
	}

	for i := 0; i < len(levels); i++ {
		parent := mustTestType(t, levels[i])
		for j := i + 1; j < len(levels); j++ {
			childPattern := mustTestType(t, levels[j])
			if parent.ConformsTo(childPattern) {
				t.Errorf("%q should NOT match child pattern %q", levels[i], levels[j])
			}
		}
	}
}

// --- IsSubtypeOf for deep hierarchies ---

func TestIsSubtypeOfDeepHierarchy(t *testing.T) {
	levels := []string{
		"Data",
		"Data/Numeric",
		"Data/Numeric/Integer",
		"Data/Numeric/Integer/Signed",
		"Data/Numeric/Integer/Signed/I32",
		"Data/Numeric/Integer/Signed/I32/Nonzero",
		"Data/Numeric/Integer/Signed/I32/Nonzero/Positive",
	}

	for i := 0; i < len(levels); i++ {
		typ := mustTestType(t, levels[i])

		// A type is NOT a subtype of itself
		if typ.IsSubtypeOf(typ) {
			t.Errorf("%q should NOT be a subtype of itself", levels[i])
		}

		// Each type is a subtype of all its ancestors
		for j := 0; j < i; j++ {
			ancestor := mustTestType(t, levels[j])
			if !typ.IsSubtypeOf(ancestor) {
				t.Errorf("%q should be a subtype of %q", levels[i], levels[j])
			}
		}

		// Each type is NOT a subtype of its descendants
		for j := i + 1; j < len(levels); j++ {
			descendant := mustTestType(t, levels[j])
			if typ.IsSubtypeOf(descendant) {
				t.Errorf("%q should NOT be a subtype of %q", levels[i], levels[j])
			}
		}
	}
}

// --- Equal for multi-level types ---

func TestEqualMultiLevel(t *testing.T) {
	paths := []string{
		"X",
		"X/Y",
		"X/Y/Z",
		"X/Y/Z/W",
		"X/Y/Z/W/V",
		"X/Y/Z/W/V/U",
		"X/Y/Z/W/V/U/T",
	}
	for _, p := range paths {
		a := mustTestType(t, p)
		b := mustTestType(t, p)
		if !a.Equal(b) {
			t.Errorf("NewType(%q) should Equal itself", p)
		}
	}

	// Different paths should not be equal
	if mustTestType(t, "A/B/C").Equal(mustTestType(t, "A/B/D")) {
		t.Error("a/b/c should not equal a/b/d")
	}
	if mustTestType(t, "A/B/C").Equal(mustTestType(t, "A/B")) {
		t.Error("a/b/c should not equal a/b")
	}
}

// --- Sibling types do not match each other ---

func TestSiblingTypesDoNotMatch(t *testing.T) {
	siblings := []string{
		"Vehicle/Car/Sedan",
		"Vehicle/Car/Suv",
		"Vehicle/Truck/Pickup",
		"Vehicle/Motorcycle/Sport",
	}
	for i := 0; i < len(siblings); i++ {
		for j := i + 1; j < len(siblings); j++ {
			a := mustTestType(t, siblings[i])
			b := mustTestType(t, siblings[j])
			if a.ConformsTo(b) {
				t.Errorf("%q should not match %q", siblings[i], siblings[j])
			}
			if b.ConformsTo(a) {
				t.Errorf("%q should not match %q", siblings[j], siblings[i])
			}
		}
	}
}

// --- "any" matches deep types, deep types don't match "any" ---

func TestAnyMatchesDeepTypes(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "Data"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/Level%d", i)
		}
		typ := mustTestType(t, path)
		if !typ.ConformsTo(TAny) {
			t.Errorf("depth %d: %q should match 'any'", depth, path)
		}
	}
}

// --- "scalar" matches number subtypes ---

func TestScalarMatchesNumberSubtypes(t *testing.T) {
	subtypes := []string{
		"Number",
		"Number/Integer",
		"Number/Integer/5",
		"Number/Float",
		"Number/Float/Double",
		"Number/Float/Double/Positive",
		"Number/Float/Double/Positive/Small",
	}
	for _, path := range subtypes {
		typ := mustTestType(t, path)
		if !typ.ConformsTo(TScalar) {
			t.Errorf("%q should match scalar", path)
		}
	}
}

// --- Well-known type relationships with number/integer ---

func TestNumberIntegerWellKnownRelationships(t *testing.T) {
	// number/integer matches number
	if !TInteger.ConformsTo(TNumber) {
		t.Error("number/integer should match number")
	}
	// number/integer is a subtype of number
	if !TInteger.IsSubtypeOf(TNumber) {
		t.Error("number/integer should be subtype of number")
	}
	// number does NOT match number/integer
	if TNumber.ConformsTo(TInteger) {
		t.Error("number should NOT match number/integer")
	}
	// number is NOT a subtype of number/integer
	if TNumber.IsSubtypeOf(TInteger) {
		t.Error("number should NOT be subtype of number/integer")
	}
	// number/integer matches any
	if !TInteger.ConformsTo(TAny) {
		t.Error("number/integer should match any")
	}
	// number/integer matches scalar
	if !TInteger.ConformsTo(TScalar) {
		t.Error("number/integer should match scalar")
	}
}

// ===== Unify tests for multi-level types =====

// --- Unify identical multi-level type literals ---

func TestUnifyIdenticalDeepTypeLiterals(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "Category"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/Sub%d", i)
		}
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			tp := mustTestType(t, path)
			a := NewTypeLiteral(tp)
			b := NewTypeLiteral(tp)
			result, ok := Unify(a, b)
			if !ok {
				t.Fatalf("Unify should succeed for identical type %q", path)
			}
			if !(&result).Equal(tp) {
				t.Errorf("result type = %s, want %s", result.String(), path)
			}
		})
	}
}

// --- Unify subtype with supertype returns narrower (subtype) ---

func TestUnifySubtypeWithSupertype(t *testing.T) {
	levels := []string{
		"Number",
		"Number/Integer",
		"Number/Integer/42",
	}

	// Unifying each deeper type with each shallower type should return the deeper type
	for i := 1; i < len(levels); i++ {
		for j := 0; j < i; j++ {
			t.Run(fmt.Sprintf("%s_with_%s", levels[i], levels[j]), func(t *testing.T) {
				child := Value{Parent: mustTestType(t, levels[i]), Data: nil}
				parent := Value{Parent: mustTestType(t, levels[j]), Data: nil}

				// child unify parent
				result, ok := Unify(child, parent)
				if !ok {
					t.Fatalf("Unify(%s, %s) should succeed", levels[i], levels[j])
				}
				if !result.Parent.Equal(mustTestType(t, levels[i])) {
					t.Errorf("Unify(%s, %s) = %s, want %s", levels[i], levels[j], result.Parent, levels[i])
				}

				// parent unify child (reversed)
				result, ok = Unify(parent, child)
				if !ok {
					t.Fatalf("Unify(%s, %s) should succeed", levels[j], levels[i])
				}
				if !result.Parent.Equal(mustTestType(t, levels[i])) {
					t.Errorf("Unify(%s, %s) = %s, want %s", levels[j], levels[i], result.Parent, levels[i])
				}
			})
		}
	}
}

// --- Unify with "any" returns the specific type at all depths ---

func TestUnifyAnyWithDeepTypes(t *testing.T) {
	for depth := 1; depth <= 7; depth++ {
		path := "Shape"
		for i := 1; i < depth; i++ {
			path += fmt.Sprintf("/Sub%d", i)
		}
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			specific := Value{Parent: mustTestType(t, path), Data: nil}
			any := Value{Parent: TAny, Data: nil}

			// any unify specific
			result, ok := Unify(any, specific)
			if !ok {
				t.Fatal("Unify(any, specific) should succeed")
			}
			if !result.Parent.Equal(mustTestType(t, path)) {
				t.Errorf("result type = %s, want %s", result.Parent, path)
			}

			// specific unify any
			result, ok = Unify(specific, any)
			if !ok {
				t.Fatal("Unify(specific, any) should succeed")
			}
			if !result.Parent.Equal(mustTestType(t, path)) {
				t.Errorf("result type = %s, want %s", result.Parent, path)
			}
		})
	}
}

// --- Unify sibling types fails ---

func TestUnifySiblingTypesFails(t *testing.T) {
	siblings := [][2]string{
		{"Number/Integer", "Number/Float"},
		{"Animal/Mammal/Cat", "Animal/Mammal/Dog"},
		{"A/B/C/D/E/F/G1", "A/B/C/D/E/F/G2"},
	}
	for _, pair := range siblings {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0], pair[1]), func(t *testing.T) {
			a := Value{Parent: mustTestType(t, pair[0]), Data: nil}
			b := Value{Parent: mustTestType(t, pair[1]), Data: nil}
			_, ok := Unify(a, b)
			if ok {
				t.Errorf("Unify(%s, %s) should fail for siblings", pair[0], pair[1])
			}
		})
	}
}

// --- Unify concrete integer values with number/integer type ---

func TestUnifyIntegerWithNumberType(t *testing.T) {
	val := NewInteger(42) // type: number/integer/42
	numType := Value{Parent: TNumber, Data: nil}

	// integer literal is subtype of number → should unify, returning the literal
	result, ok := Unify(val, numType)
	if !ok {
		t.Fatal("Unify(42, number) should succeed")
	}
	_as0, _ := AsInteger(result)
	if _as0 != 42 {
		_as1, _ := AsInteger(result)
		t.Errorf("expected 42, got %d", _as1)
	}
}

func TestUnifyIntegerWithIntegerType(t *testing.T) {
	val := NewInteger(7) // type: number/integer/7
	intType := Value{Parent: TInteger, Data: nil}

	// number/integer/7 is subtype of number/integer → should unify
	result, ok := Unify(val, intType)
	if !ok {
		t.Fatal("Unify(7, number/integer) should succeed")
	}
	_as2, _ := AsInteger(result)
	if _as2 != 7 {
		_as3, _ := AsInteger(result)
		t.Errorf("expected 7, got %d", _as3)
	}
}

// --- Unify two different integer literals fails ---

func TestUnifyDifferentIntegerLiteralsFails(t *testing.T) {
	a := NewInteger(3) // number/integer/3
	b := NewInteger(5) // number/integer/5

	_, ok := Unify(a, b)
	if ok {
		t.Error("Unify(3, 5) should fail — different literal values")
	}
}

// --- Unify same integer literal succeeds ---

func TestUnifySameIntegerLiteralSucceeds(t *testing.T) {
	a := NewInteger(10)
	b := NewInteger(10)

	result, ok := Unify(a, b)
	if !ok {
		t.Fatal("Unify(10, 10) should succeed")
	}
	_as4, _ := AsInteger(result)
	if _as4 != 10 {
		_as5, _ := AsInteger(result)
		t.Errorf("expected 10, got %d", _as5)
	}
}

// --- Unify deeply nested custom types ---

func TestUnifyDeepHierarchy7Levels(t *testing.T) {
	// 7-level type hierarchy
	t1 := mustTestType(t, "A/B/C/D/E/F/G")
	t2 := mustTestType(t, "A/B/C/D/E/F")
	t3 := mustTestType(t, "A/B/C/D/E")
	t4 := mustTestType(t, "A/B/C/D")
	t5 := mustTestType(t, "A/B/C")
	t6 := mustTestType(t, "A/B")
	t7 := mustTestType(t, "A")

	deepest := Value{Parent: t1, Data: nil}
	ancestors := []*Type{t2, t3, t4, t5, t6, t7}

	for _, anc := range ancestors {
		ancestor := Value{Parent: anc, Data: nil}
		result, ok := Unify(deepest, ancestor)
		if !ok {
			t.Fatalf("Unify(%s, %s) should succeed", t1, anc)
		}
		// Should return the deeper (more specific) type
		if !result.Parent.Equal(t1) {
			t.Errorf("Unify(%s, %s) = %s, want %s", t1, anc, result.Parent, t1)
		}
	}
}

// --- Unify incompatible hierarchies fails ---

func TestUnifyIncompatibleHierarchiesFails(t *testing.T) {
	tests := [][2]string{
		{"Number/Integer", "String/Proper"},
		{"A/B/C", "X/Y/Z"},
		{"Data/Num/Int/I32/Signed/Big/Huge", "Data/Str/Utf8/Ascii/Printable/Alpha/Upper"},
	}
	for _, pair := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0], pair[1]), func(t *testing.T) {
			a := Value{Parent: mustTestType(t, pair[0]), Data: nil}
			b := Value{Parent: mustTestType(t, pair[1]), Data: nil}
			_, ok := Unify(a, b)
			if ok {
				t.Errorf("Unify(%s, %s) should fail for incompatible hierarchies", pair[0], pair[1])
			}
		})
	}
}

// --- Unify none with deep type fails ---

func TestUnifyNoneWithDeepTypeFails(t *testing.T) {
	none := Value{Parent: TNone, Data: nil}
	deep := Value{Parent: mustTestType(t, "A/B/C/D/E/F/G"), Data: nil}
	_, ok := Unify(none, deep)
	if ok {
		t.Error("Unify(none, deep-type) should fail")
	}
	_, ok = Unify(deep, none)
	if ok {
		t.Error("Unify(deep-type, none) should fail")
	}
}

// --- Unify none with none succeeds ---

func TestUnifyNoneWithNoneSucceeds(t *testing.T) {
	a := Value{Parent: TNone, Data: nil}
	b := Value{Parent: TNone, Data: nil}
	_, ok := Unify(a, b)
	if !ok {
		t.Error("Unify(none, none) should succeed")
	}
}

// --- MatchSignature with multi-level type hierarchies ---

func TestMatchSignatureDeepTypeHierarchy(t *testing.T) {
	// Register signatures at different specificity levels
	sigs := []Signature{
		{Args: []*Type{mustTestType(t, "Data")}, Impl: Go(dummyHandler), BarrierPos: -1},                 // depth 1
		{Args: []*Type{mustTestType(t, "Data/Num")}, Impl: Go(dummyHandler), BarrierPos: -1},             // depth 2
		{Args: []*Type{mustTestType(t, "Data/Num/Int")}, Impl: Go(dummyHandler), BarrierPos: -1},         // depth 3
		{Args: []*Type{mustTestType(t, "Data/Num/Int/I32")}, Impl: Go(dummyHandler), BarrierPos: -1},     // depth 4
		{Args: []*Type{mustTestType(t, "Data/Num/Int/I32/Signed")}, Impl: Go(dummyHandler), BarrierPos: - // depth 5
		1},
		{Args: []*Type{mustTestType(t, "Data/Num/Int/I32/Signed/Big")}, Impl: Go(dummyHandler), BarrierPos: // depth 6
		-1},
		{Args: []*Type{mustTestType(t, "Data/Num/Int/I32/Signed/Big/Huge")}, Impl: Go(dummyHandler), BarrierPos: // depth 7
		-1},
	}
	SortSignatures(sigs)

	// A value with the deepest type should match the most specific signature.
	// A carrier stands for the value: a payload-less non-carrier is a TYPE
	// literal, which a value slot refuses (NUR323).
	val := Value{Parent: mustTestType(t, "Data/Num/Int/I32/Signed/Big/Huge"), Carrier: true}
	stack := []Value{val}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(mustTestType(t, "Data/Num/Int/I32/Signed/Big/Huge")) {
		t.Errorf("expected deepest match, got %s", m.Sig.Args[0])
	}
}

func TestMatchSignatureMidLevelType(t *testing.T) {
	// Only register signatures up to depth 4
	sigs := []Signature{
		{Args: []*Type{mustTestType(t, "Data")}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{mustTestType(t, "Data/Num")}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{mustTestType(t, "Data/Num/Int")}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{mustTestType(t, "Data/Num/Int/I32")}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	SortSignatures(sigs)

	// A value at depth 6 should match the deepest available signature (depth 4)
	val := Value{Parent: mustTestType(t, "Data/Num/Int/I32/Signed/Big"), Carrier: true} // a value, not the type (NUR323)
	stack := []Value{val}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(mustTestType(t, "Data/Num/Int/I32")) {
		t.Errorf("expected data/num/int/i32 match, got %s", m.Sig.Args[0])
	}
}

// --- Unify with lists containing deep-typed values ---

func TestUnifyListsWithDeepTypedValues(t *testing.T) {
	// Two lists with integer values should unify element-by-element
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})

	result, ok := Unify(a, b)
	if !ok {
		t.Fatal("Unify of identical integer lists should succeed")
	}
	_lst, _ := AsList(result)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	_as7, _ := AsInteger(elems[0])
	_as6, _ := AsInteger(elems[1])
	if _as7 != 1 || _as6 != 2 {
		t.Errorf("unexpected values: %v", elems)
	}
}

// --- Unify maps with deep-typed values ---

func TestUnifyMapsWithDeepTypedValues(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("X", NewInteger(10))
	m1.Set("y", NewInteger(20))

	m2 := NewOrderedMap()
	m2.Set("X", NewInteger(10))
	m2.Set("y", NewInteger(20))

	result, ok := Unify(NewMap(m1), NewMap(m2))
	if !ok {
		t.Fatal("Unify of identical integer-valued maps should succeed")
	}
	rm, _ := AsMap(result)
	xv, _ := rm.Get("X")
	yv, _ := rm.Get("y")
	_as9, _ := AsInteger(xv)
	_as8, _ := AsInteger(yv)
	if _as9 != 10 || _as8 != 20 {
		_as11, _ := AsInteger(xv)
		_as10, _ := AsInteger(yv)
		t.Errorf("unexpected map values: x=%d y=%d", _as11, _as10)
	}
}

// --- Unify maps with mismatched deep-typed values fails ---

func TestUnifyMapsWithMismatchedValuesFails(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("X", NewInteger(10))

	m2 := NewOrderedMap()
	m2.Set("X", NewInteger(99))

	_, ok := Unify(NewMap(m1), NewMap(m2))
	if ok {
		t.Error("Unify of maps with different integer values should fail")
	}
}

// --- Unify symmetry: order of arguments doesn't matter ---

func TestUnifySymmetry(t *testing.T) {
	pairs := [][2]Value{
		{Value{Parent: mustTestType(t, "A/B/C"), Data: nil}, Value{Parent: mustTestType(t, "A/B"), Data: nil}},
		{Value{Parent: mustTestType(t, "Number/Integer"), Data: nil}, Value{Parent: mustTestType(t, "Number"), Data: nil}},
		{Value{Parent: TAny, Data: nil}, Value{Parent: mustTestType(t, "X/Y/Z/W"), Data: nil}},
		{NewInteger(5), Value{Parent: TNumber, Data: nil}},
		{NewInteger(5), Value{Parent: TInteger, Data: nil}},
	}

	for _, pair := range pairs {
		t.Run(fmt.Sprintf("%s_vs_%s", pair[0].Parent, pair[1].Parent), func(t *testing.T) {
			r1, ok1 := Unify(pair[0], pair[1])
			r2, ok2 := Unify(pair[1], pair[0])
			if ok1 != ok2 {
				t.Fatalf("asymmetric unification: (%s,%s)=%v but (%s,%s)=%v",
					pair[0].Parent, pair[1].Parent, ok1,
					pair[1].Parent, pair[0].Parent, ok2)
			}
			if ok1 && !r1.Parent.Equal(r2.Parent) {
				t.Errorf("asymmetric result: Unify(%s,%s)=%s but Unify(%s,%s)=%s",
					pair[0].Parent, pair[1].Parent, r1.Parent,
					pair[1].Parent, pair[0].Parent, r2.Parent)
			}
		})
	}
}

// ===== Efficiency tests: thousands of sibling types =====

// TestConformsToEfficiencyThousandsOfSiblings confirms that matching a/b/x against
// a/b is O(len(pattern)) and not affected by how many sibling types a/b/x<N> exist.
// We create 10,000 distinct types under the same parent and verify that matching
// each one against the parent pattern takes constant time per check.
func TestConformsToEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := mustTestType(t, "A/B")

	// Create 10,000 sibling types: a/b/0, a/b/1, ..., a/b/9999
	siblings := make([]*Type, numSiblings)
	for i := 0; i < numSiblings; i++ {
		siblings[i] = mustTestType(t, fmt.Sprintf("A/B/%d", i))
	}

	// Every sibling must match the parent
	for i, sib := range siblings {
		if !sib.ConformsTo(parent) {
			t.Fatalf("sibling A/B/%d should match A/B", i)
		}
	}

	// Time the full pass: 10,000 ConformsTo calls should be very fast (<50ms easily)
	start := time.Now()
	for _, sib := range siblings {
		sib.ConformsTo(parent)
	}
	elapsed := time.Since(start)

	// 10,000 prefix comparisons should be sub-millisecond; generous 50ms limit
	if elapsed > 50*time.Millisecond {
		t.Errorf("10,000 ConformsTo calls took %v — expected <50ms", elapsed)
	}
	t.Logf("10,000 ConformsTo calls completed in %v", elapsed)
}

// TestIsSubtypeOfEfficiencyThousandsOfSiblings confirms IsSubtypeOf is O(len(parent)).
func TestIsSubtypeOfEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := mustTestType(t, "A/B")

	siblings := make([]*Type, numSiblings)
	for i := 0; i < numSiblings; i++ {
		siblings[i] = mustTestType(t, fmt.Sprintf("A/B/%d", i))
	}

	for i, sib := range siblings {
		if !sib.IsSubtypeOf(parent) {
			t.Fatalf("sibling A/B/%d should be subtype of A/B", i)
		}
	}

	start := time.Now()
	for _, sib := range siblings {
		sib.IsSubtypeOf(parent)
	}
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("10,000 IsSubtypeOf calls took %v — expected <50ms", elapsed)
	}
	t.Logf("10,000 IsSubtypeOf calls completed in %v", elapsed)
}

// TestMatchSignatureEfficiencyThousandsOfSiblings confirms that MatchSignature
// picks the right signature efficiently when a value has one of thousands of
// sibling types and only a parent-level signature is registered.
func TestMatchSignatureEfficiencyThousandsOfSiblings(t *testing.T) {
	const numSiblings = 10_000
	parent := mustTestType(t, "A/B")

	sigs := []Signature{
		{Args: []*Type{parent}, Impl: Go(dummyHandler), BarrierPos: -1},
	}

	start := time.Now()
	for i := 0; i < numSiblings; i++ {
		val := Value{Parent: mustTestType(t, fmt.Sprintf("A/B/%d", i)), Carrier: true} // a value, not the type (NUR323)
		m := MatchSignature(sigs, []Value{val}, WordInfo{ArgCount: -1})
		if m == nil {
			t.Fatalf("A/B/%d should match signature [A/B]", i)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("10,000 MatchSignature calls took %v — expected <200ms", elapsed)
	}
	t.Logf("10,000 MatchSignature calls completed in %v", elapsed)
}

// TestUnifyEfficiencyThousandsOfSiblings confirms Unify(a/b/x, a/b) is efficient
// across thousands of distinct sibling types.
func TestUnifyEfficiencyThousandsOfSiblings(t *testing.T) {
	// A wall-clock efficiency assertion — meaningless under the race
	// detector, which inflates CPU ~5-10x. The race lane runs -short so this
	// perf gate is skipped there while the concurrency tests still run under
	// -race (Makefile test-race).
	if testing.Short() {
		t.Skip("perf timing assertion; skipped under -short (race lane)")
	}
	const numSiblings = 10_000
	parentVal := Value{Parent: mustTestType(t, "A/B"), Data: nil}

	start := time.Now()
	for i := 0; i < numSiblings; i++ {
		child := Value{Parent: mustTestType(t, fmt.Sprintf("A/B/%d", i)), Data: nil}
		result, ok := Unify(child, parentVal)
		if !ok {
			t.Fatalf("Unify(A/B/%d, A/B) should succeed", i)
		}
		// Should return the child (narrower type)
		if result.Parent.Leaf() != fmt.Sprintf("%d", i) {
			t.Fatalf("Unify(A/B/%d, A/B) returned wrong type: %s", i, result.Parent)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("10,000 Unify calls took %v — expected <200ms", elapsed)
	}
	t.Logf("10,000 Unify calls completed in %v", elapsed)
}
