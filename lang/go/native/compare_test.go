package native

import "testing"

func TestCompareValuesIntegers(t *testing.T) {
	tests := []struct {
		name string
		a, b int64
		want int
	}{
		{"less", 1, 2, -1},
		{"greater", 3, 1, 1},
		{"equal", 5, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewInteger(tt.a), NewInteger(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesStrings(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"less", "abc", "def", -1},
		{"greater", "xyz", "abc", 1},
		{"equal", "same", "same", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewString(tt.a), NewString(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesBooleans(t *testing.T) {
	tests := []struct {
		name string
		a, b bool
		want int
	}{
		{"false_lt_true", false, true, -1},
		{"true_gt_false", true, false, 1},
		{"equal_true", true, true, 0},
		{"equal_false", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewBoolean(tt.a), NewBoolean(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompareValuesAtoms(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"less", "abc", "def", -1},
		{"greater", "xyz", "abc", 1},
		{"equal", "foo", "foo", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(NewAtom(tt.a), NewAtom(tt.b))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompareValuesCrossScalar(t *testing.T) {
	// Cross-branch scalar pairs are ordered by the Scalar root's
	// Comparer: Atom < Boolean < Number < String < Path.
	path := NewPathon([]string{"a"}, false)
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"atom_lt_boolean", NewAtom("z"), NewBoolean(true), -1},
		{"boolean_gt_atom", NewBoolean(true), NewAtom("z"), 1},
		{"boolean_lt_number", NewBoolean(false), NewInteger(1), -1},
		{"number_gt_boolean", NewInteger(1), NewBoolean(false), 1},
		{"number_lt_string", NewInteger(1), NewString("a"), -1},
		{"string_gt_number", NewString("a"), NewInteger(1), 1},
		{"string_lt_path", NewString("a"), path, -1},
		{"path_gt_string", path, NewString("a"), 1},
		{"atom_lt_path", NewAtom("z"), path, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesPaths(t *testing.T) {
	// Path-vs-Path falls through to the Scalar comparator, which orders
	// paths by the NUR012 natural order: volume, then absolute before
	// relative, then segment-by-segment forward lexical comparison,
	// then shorter-prefix-first (a directory before its contents).
	abc := NewPathon([]string{"a", "b", "c"}, false) // 3 segments
	ab := NewPathon([]string{"a", "b"}, false)       // 2 segments
	ac := NewPathon([]string{"a", "c"}, false)       // 2 segments
	zzz := NewPathon([]string{"z", "z", "z"}, false) // 3 segments
	aDashA := NewPathon([]string{"a-", "a"}, false)  // 2 segments
	absAB := NewPathon([]string{"a", "b"}, true)     // /a/b — absolute
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"prefix_sorts_first", ab, abc, -1},
		{"extension_sorts_after", abc, ab, 1},
		{"lexical_beats_length", ab, zzz, -1}, // forward lexical: "a" < "z" decides before length
		{"equal_len_segment", ac, ab, 1},      // forward lexical: "c" > "b" → ab before ac
		{"equal_len_segment_rev", ab, ac, -1},
		{"per_element_beats_render", aDashA, ab, 1}, // forward lexical: "a-" > "a" → ab before aDashA
		{"abs_before_rel", absAB, ab, -1},
		{"rel_after_abs", ab, absAB, 1},
		{"identical", ab, ab, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValuesCrossBranch(t *testing.T) {
	// Cross-branch pairs order by the top-level precedence
	// Never < Any < None < Word < Type < Scalar < Node < Ideal.
	arr := NewStore(TStore)                // Ideal branch
	lst := NewList([]Value{NewInteger(1)}) // Node branch
	num := NewInteger(5)                   // Scalar branch
	none := NewTypeLiteral(TNone)          // None branch
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"none_before_scalar", none, num, -1},
		{"scalar_before_node", num, lst, -1},
		{"node_before_ideal", lst, arr, -1},
		{"ideal_after_node", arr, lst, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestCompareValuesSameBranchBySize — same-branch pairs with no
// Comparer order by size, smaller first (less complex values lead).
func TestCompareValuesSameBranchBySize(t *testing.T) {
	long := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	short := NewList([]Value{NewInteger(1)})
	if got, err := CompareValues(short, long); err != nil || got != -1 {
		t.Errorf("CompareValues(len 1, len 3) = %d, %v; want -1, nil (smaller first)", got, err)
	}
	if got, err := CompareValues(long, short); err != nil || got != 1 {
		t.Errorf("CompareValues(len 3, len 1) = %d, %v; want 1, nil", got, err)
	}
	// Equal size — fall through to the type and structural tiebreaks
	// (see TestCompareValuesStructural); element-wise here, 1 < 9.
	if got, err := CompareValues(NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(9)})); err != nil || got != -1 {
		t.Errorf("CompareValues([1], [9]) = %d, %v; want -1, nil (element-wise)", got, err)
	}
}

// TestCompareTypes — the type order used as the post-size tiebreaker
// and for sorting type literals: family rank (List < Map, Object <
// Record < Table), then depth (a type before its subtype),
// then name (siblings Foo and Bar).
func TestCompareTypes(t *testing.T) {
	foo := addrOf(MakeValue(TList, nil))
	foo.SetName("Foo")
	bar := addrOf(MakeValue(TList, nil))
	bar.SetName("Bar")
	tests := []struct {
		name string
		a, b Value
		want int
	}{
		{"list_before_map", NewTypeLiteral(TList), NewTypeLiteral(TMap), -1},
		{"map_after_list", NewTypeLiteral(TMap), NewTypeLiteral(TList), 1},
		{"record_before_table", NewTypeLiteral(TRecord), NewTypeLiteral(TTable), -1},
		{"type_before_subtype", NewTypeLiteral(TList), NewTypeLiteral(foo), -1},
		{"bar_before_foo_by_name", NewTypeLiteral(bar), NewTypeLiteral(foo), -1},
		{"foo_after_bar_by_name", NewTypeLiteral(foo), NewTypeLiteral(bar), 1},
		{"same_type_equal", NewTypeLiteral(foo), NewTypeLiteral(foo), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareValues(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CompareValues(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}

	// Value level: a Foo value and a Bar value of equal size resolve
	// by the type order — Bar before Foo, by name.
	fooVal := NewList([]Value{NewInteger(1)})
	SetParent(&fooVal, foo)
	barVal := NewList([]Value{NewInteger(1)})
	SetParent(&barVal, bar)
	if got, err := CompareValues(barVal, fooVal); err != nil || got != -1 {
		t.Errorf("Bar-value vs Foo-value = %d, %v; want -1, nil", got, err)
	}
}

// TestCompareValuesStructural — two values of the same type and size
// break the tie structurally: lists element-wise, maps by sorted keys
// then by the value at each key.
func TestCompareValuesStructural(t *testing.T) {
	// Lists of equal length compare element by element.
	if got, err := CompareValues(
		NewList([]Value{NewInteger(1), NewInteger(2)}),
		NewList([]Value{NewInteger(1), NewInteger(3)})); err != nil || got != -1 {
		t.Errorf("[1 2] vs [1 3] = %d, %v; want -1, nil", got, err)
	}
	// Identical lists compare equal.
	if got, err := CompareValues(
		NewList([]Value{NewInteger(5)}),
		NewList([]Value{NewInteger(5)})); err != nil || got != 0 {
		t.Errorf("[5] vs [5] = %d, %v; want 0, nil", got, err)
	}
	// Maps of equal key count compare by the value at each key.
	m1 := NewOrderedMap()
	m1.Set("a", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("a", NewInteger(2))
	if got, err := CompareValues(NewMap(m1), NewMap(m2)); err != nil || got != -1 {
		t.Errorf("{a:1} vs {a:2} = %d, %v; want -1, nil", got, err)
	}
	// Differing key sets compare by sorted keys.
	m3 := NewOrderedMap()
	m3.Set("b", NewInteger(1))
	if got, err := CompareValues(NewMap(m1), NewMap(m3)); err != nil || got != -1 {
		t.Errorf("{a:1} vs {b:1} = %d, %v; want -1, nil", got, err)
	}
}

func TestCompareValuesWords(t *testing.T) {
	// Word, like String and Atom, compares lexicographically.
	got, err := CompareValues(NewWord("apple"), NewWord("banana"))
	if err != nil || got != -1 {
		t.Errorf("CompareValues(apple, banana) = %d, %v; want -1, nil", got, err)
	}
}

func TestExactEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"int_equal", NewInteger(1), NewInteger(1), true},
		{"int_notequal", NewInteger(1), NewInteger(2), false},
		{"str_equal", NewString("abc"), NewString("abc"), true},
		{"str_notequal", NewString("a"), NewString("b"), false},
		{"bool_equal", NewBoolean(true), NewBoolean(true), true},
		{"bool_notequal", NewBoolean(true), NewBoolean(false), false},
		{"atom_equal", NewAtom("x"), NewAtom("x"), true},
		{"atom_notequal", NewAtom("x"), NewAtom("y"), false},
		{"none_equal", NewTypeLiteral(TNone), NewTypeLiteral(TNone), true},
		{"cross_type", NewInteger(1), NewString("1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExactEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ExactEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExactEqualDifferentTypes(t *testing.T) {
	// Different types should not be equal
	if ExactEqual(NewInteger(1), NewBoolean(true)) {
		t.Error("int and bool should not be exactly equal")
	}
	if ExactEqual(NewString("1"), NewAtom("1")) {
		t.Error("string and atom should not be exactly equal")
	}
}

func TestExactEqualMapIdentity(t *testing.T) {
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	a := NewMap(m)
	if !ExactEqual(a, a) {
		t.Error("same map should be equal to itself")
	}
}

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"int_equal", NewInteger(1), NewInteger(1), true},
		{"int_notequal", NewInteger(1), NewInteger(2), false},
		{"str_equal", NewString("abc"), NewString("abc"), true},
		{"str_notequal", NewString("a"), NewString("b"), false},
		{"bool_equal", NewBoolean(true), NewBoolean(true), true},
		{"bool_notequal", NewBoolean(true), NewBoolean(false), false},
		{"atom_equal", NewAtom("x"), NewAtom("x"), true},
		{"atom_notequal", NewAtom("x"), NewAtom("y"), false},
		{"none_equal", NewTypeLiteral(TNone), NewTypeLiteral(TNone), true},
		{"cross_type", NewInteger(1), NewString("1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("DeepEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeepEqualLists(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	b := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	c := NewList([]Value{NewInteger(1), NewInteger(2)})
	d := NewList([]Value{NewInteger(1), NewInteger(9), NewInteger(3)})

	if !DeepEqual(a, b) {
		t.Error("equal lists should be deeply equal")
	}
	if DeepEqual(a, c) {
		t.Error("different length lists should not be deeply equal")
	}
	if DeepEqual(a, d) {
		t.Error("different element lists should not be deeply equal")
	}
}

func TestDeepEqualMaps(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m1.Set("y", NewInteger(2))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("y", NewInteger(2))

	m3 := NewOrderedMap()
	m3.Set("x", NewInteger(1))
	m3.Set("y", NewInteger(9))

	m4 := NewOrderedMap()
	m4.Set("x", NewInteger(1))

	if !DeepEqual(NewMap(m1), NewMap(m2)) {
		t.Error("equal maps should be deeply equal")
	}
	if DeepEqual(NewMap(m1), NewMap(m3)) {
		t.Error("different value maps should not be deeply equal")
	}
	if DeepEqual(NewMap(m1), NewMap(m4)) {
		t.Error("different size maps should not be deeply equal")
	}
}

func TestDeepEqualMapsMissingKey(t *testing.T) {
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m1.Set("y", NewInteger(2))

	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(1))
	m2.Set("z", NewInteger(2))

	if DeepEqual(NewMap(m1), NewMap(m2)) {
		t.Error("maps with different keys should not be deeply equal")
	}
}

// revPathBehavior is a test fixture for TestRevPathComparator: a
// Comparer whose order is the reverse of the normal Path comparator.
// Match/Format/Equal delegate to the kernel default; Compare re-types
// both operands to Path, runs the normal comparison, and negates it.
type revPathBehavior struct{}

func (revPathBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (revPathBehavior) Format(v Value) string       { return DefaultBehavior.Format(v) }
func (revPathBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }

func (revPathBehavior) Compare(a, b Value) (int, error) {
	pa, pb := a, b
	SetParent(&pa, TPathon)
	SetParent(&pb, TPathon)
	n, err := CompareValues(pa, pb)
	return -n, err
}

// TestRevPathComparator exercises the behavior system: it defines
// RevPath, a subtype of Path, and installs a Comparer that is the
// reverse of the normal Pathon comparator. RevPath values must compare
// in reversed order, while plain Pathon values keep the normal order —
// proving a subtype can override an inherited capability and that
// CompareValues' lattice walk picks the most-specific Comparer.
func TestRevPathComparator(t *testing.T) {
	revPath := addrOf(MakeValue(TPathon, nil))
	revPath.SetName("RevPath")
	revPath.SetBehavior(revPathBehavior{})
	rev := func(parts ...string) Value {
		p := NewPathon(parts, false)
		SetParent(&p, revPath)
		return p
	}

	tests := []struct {
		name       string
		a, b       []string
		normalWant int // CompareValues for the same pair as plain Paths
	}{
		{"longer_after", []string{"a", "b", "c"}, []string{"a", "b"}, 1},
		{"segment_order", []string{"a", "b"}, []string{"a", "c"}, -1}, // NUR012 forward lex: b < c
		{"identical", []string{"a", "b"}, []string{"a", "b"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Plain Path keeps the normal order.
			got, err := CompareValues(NewPathon(tt.a, false), NewPathon(tt.b, false))
			if err != nil || got != tt.normalWant {
				t.Fatalf("plain Path: got %d, %v; want %d", got, err, tt.normalWant)
			}
			// RevPath reverses it.
			got, err = CompareValues(rev(tt.a...), rev(tt.b...))
			if err != nil || got != -tt.normalWant {
				t.Errorf("RevPath: got %d, %v; want %d (reverse of plain %d)",
					got, err, -tt.normalWant, tt.normalWant)
			}
		})
	}
}
