package native

import (
	"testing"
)

// --- helpers ---

func newMap(kvs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i].(string), kvs[i+1].(Value))
	}
	return NewMap(m)
}

func newList(vals ...Value) Value {
	return NewList(vals)
}

// --- Register ---

func TestRegister(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	Register(r)
	// If we get here without panic, registration worked.
}

// --- clone ---

func TestCloneHandler(t *testing.T) {
	orig := newMap("a", NewInteger(1), "b", NewString("hello"))
	result, err := cloneHandler([]Value{orig}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	v, _ := m.Get("a")
	vi, _ := AsInteger(v)
	if vi != 1 {
		t.Errorf("expected 1, got %d", vi)
	}
	v, _ = m.Get("b")
	vs, _ := AsString(v)
	if vs != "hello" {
		t.Errorf("expected hello, got %s", vs)
	}
}

func TestCloneHandlerList(t *testing.T) {
	orig := newList(NewInteger(1), NewInteger(2))
	result, err := cloneHandler([]Value{orig}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

// --- flatten ---

func TestFlattenDefaultHandler(t *testing.T) {
	inner := newList(NewInteger(3), NewInteger(4))
	data := newList(NewInteger(1), NewInteger(2), inner)
	result, err := flattenDefaultHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 4 {
		t.Errorf("expected 4 elements, got %d", len(list))
	}
}

func TestFlattenDepthHandler(t *testing.T) {
	deep := newList(NewInteger(4))
	mid := newList(NewInteger(3), deep)
	data := newList(NewInteger(1), mid)

	// depth=1 should only flatten one level
	result, err := flattenDepthHandler([]Value{NewInteger(1), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	// [1, 3, [4]] -> 3 elements
	if len(list) != 3 {
		t.Errorf("expected 3 elements, got %d", len(list))
	}
}

// --- getpath ---

func TestGetpathHandler(t *testing.T) {
	inner := newMap("b", NewInteger(42))
	data := newMap("a", inner)
	result, err := getpathHandler([]Value{NewString("a.b"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := AsInteger(result[0])
	if ri != 42 {
		t.Errorf("expected 42, got %d", ri)
	}
}

func TestGetpathHandlerTopLevel(t *testing.T) {
	data := newMap("x", NewString("hello"))
	result, err := getpathHandler([]Value{NewString("x"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rs, _ := AsString(result[0])
	if rs != "hello" {
		t.Errorf("expected hello, got %s", rs)
	}
}

// --- setpath ---

func TestSetpathHandler(t *testing.T) {
	data := newMap("a", NewInteger(1))
	result, err := setpathHandler([]Value{data, NewString("b"), NewInteger(2)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("b")
	if !ok {
		t.Fatal("expected key 'b'")
	}
	vi, _ := AsInteger(v)
	if vi != 2 {
		t.Errorf("expected 2, got %d", vi)
	}
}

func TestSetpathHandlerNewKey(t *testing.T) {
	data := newMap("a", NewInteger(1))
	result, err := setpathHandler([]Value{data, NewString("c"), NewString("new")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify the new key was set by reading it back with getpath
	check, err := getpathHandler([]Value{NewString("c"), result[0]}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := AsString(check[0])
	if cs != "new" {
		t.Errorf("expected new, got %s", cs)
	}
}

// --- inject ---

func TestInjectHandler(t *testing.T) {
	tmpl := newMap("greeting", NewString("`name`"))
	store := newMap("name", NewString("Alice"))
	result, err := injectHandler([]Value{tmpl, store}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, _ := m.Get("greeting")
	vs, _ := AsString(v)
	if vs != "Alice" {
		t.Errorf("expected Alice, got %s", vs)
	}
}

// --- items ---

func TestItemsHandler(t *testing.T) {
	data := newMap("x", NewInteger(1), "y", NewInteger(2))
	result, err := itemsHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(list))
	}
	// Each item is [key, value]
	_lst2, _ := AsList(list[0])
	pair := _lst2.Slice()
	if len(pair) != 2 {
		t.Fatalf("expected pair of 2, got %d", len(pair))
	}
}

// --- join ---

func TestJoinDefaultHandler(t *testing.T) {
	data := newList(NewString("a"), NewString("b"), NewString("c"))
	result, err := joinDefaultHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if s != "a,b,c" {
		t.Errorf("expected a,b,c got %s", s)
	}
}

func TestJoinSepHandler(t *testing.T) {
	data := newList(NewString("a"), NewString("b"))
	result, err := joinSepHandler([]Value{NewString("-"), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if s != "a-b" {
		t.Errorf("expected a-b got %s", s)
	}
}

// --- jsonify ---

func TestJsonifyDefaultHandler(t *testing.T) {
	data := newMap("a", NewInteger(1))
	result, err := jsonifyDefaultHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if s == "" {
		t.Error("expected non-empty JSON string")
	}
}

func TestJsonifyFlagsHandler(t *testing.T) {
	data := newMap("a", NewInteger(1))
	flags := newMap()
	result, err := jsonifyFlagsHandler([]Value{data, flags}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if s == "" {
		t.Error("expected non-empty JSON string")
	}
}

// --- merge ---

func TestMergeHandler(t *testing.T) {
	a := newMap("x", NewInteger(1))
	b := newMap("y", NewInteger(2))
	result, err := mergeHandler([]Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, ok := m.Get("x")
	vi, _ := AsInteger(v)
	if !ok || vi != 1 {
		t.Error("expected x=1")
	}
	v, ok = m.Get("y")
	vi, _ = AsInteger(v)
	if !ok || vi != 2 {
		t.Error("expected y=2")
	}
}

func TestMergeHandlerOverwrite(t *testing.T) {
	a := newMap("x", NewInteger(1))
	b := newMap("x", NewInteger(99))
	result, err := mergeHandler([]Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := AsMap(result[0])
	v, _ := m.Get("x")
	vi, _ := AsInteger(v)
	if vi != 99 {
		t.Errorf("expected 99, got %d", vi)
	}
}

// --- pad ---

func TestPadDefaultHandler(t *testing.T) {
	result, err := padDefaultHandler([]Value{NewString("hi")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if len(s) == 0 {
		t.Error("expected non-empty padded string")
	}
}

func TestPadWidthHandler(t *testing.T) {
	result, err := padWidthHandler([]Value{NewInteger(10), NewString("hi")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := AsString(result[0])
	if len(s) < 10 {
		t.Errorf("expected at least 10 chars, got %d", len(s))
	}
}

// --- selector ---

func TestSelectorHandler(t *testing.T) {
	children := newMap(
		"a", newMap("color", NewString("red")),
		"b", newMap("color", NewString("blue")),
	)
	query := newMap("color", NewString("red"))
	result, err := selectorHandler([]Value{children, query}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- size ---

func TestSizeHandlerList(t *testing.T) {
	data := newList(NewInteger(1), NewInteger(2), NewInteger(3))
	result, err := sizeHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := AsInteger(result[0])
	if ri != 3 {
		t.Errorf("expected 3, got %d", ri)
	}
}

func TestSizeHandlerMap(t *testing.T) {
	data := newMap("a", NewInteger(1), "b", NewInteger(2))
	result, err := sizeHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := AsInteger(result[0])
	if ri != 2 {
		t.Errorf("expected 2, got %d", ri)
	}
}

func TestSizeHandlerString(t *testing.T) {
	result, err := sizeHandler([]Value{NewString("hello")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ri, _ := AsInteger(result[0])
	if ri != 5 {
		t.Errorf("expected 5, got %d", ri)
	}
}

func TestSizeHandlerBehaviour(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want int64
	}{
		{"decimal_floors", NewFloat(7.9), 7},
		{"integer_magnitude", NewInteger(42), 42},
		{"boolean_true", NewBoolean(true), 1},
		{"boolean_false", NewBoolean(false), 0},
		{"atom_name_length", NewAtom("hello"), 5},
		{"path_segment_count", NewPathon([]string{"a", "b", "c"}, false), 3},
		{"none_is_zero", NewTypeLiteral(TNone), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sizeHandler([]Value{tt.val}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := AsInteger(result[0])
			if got != tt.want {
				t.Errorf("size(%s) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

// --- slice ---

func TestSliceAllHandler(t *testing.T) {
	data := newList(NewInteger(1), NewInteger(2), NewInteger(3))
	result, err := sliceAllHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
}

func TestSliceStartHandler(t *testing.T) {
	data := newList(NewInteger(1), NewInteger(2), NewInteger(3))
	result, err := sliceStartHandler([]Value{NewInteger(1), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestSliceStartEndHandler(t *testing.T) {
	data := newList(NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4))
	result, err := sliceStartEndHandler([]Value{NewInteger(1), NewInteger(3), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

// TestSlicePreservesElementTypes guards against the stringification
// regression: slicing a value list must return elements of their
// original type, not their String() form. The length-only tests above
// never exercised this, which is exactly how the defect hid.
func TestSlicePreservesElementTypes(t *testing.T) {
	data := newList(NewInteger(3), NewInteger(1), NewInteger(2))
	result, err := sliceStartEndHandler([]Value{NewInteger(0), NewInteger(2), data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	got := _lst.Slice()
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
	for i, elem := range got {
		if !elem.Parent.ConformsTo(TInteger) {
			t.Errorf("element %d: expected Integer, got %s (%q)", i, elem.Parent.String(), elem.String())
		}
	}
	// Mixed scalar types round-trip unchanged.
	mixed := newList(NewBoolean(true), NewString("x"), NewFloat(1.5))
	mres, err := sliceAllHandler([]Value{mixed}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ml, _ := AsList(mres[0])
	ml := _ml.Slice()
	if len(ml) != 3 || !ml[0].Parent.ConformsTo(TBoolean) ||
		!ml[1].Parent.ConformsTo(TString) || !ml[2].Parent.ConformsTo(TFloat) {
		t.Errorf("mixed slice did not preserve types: %v", ml)
	}
	// Negative/contrast: a string slice still yields a string (not a list).
	sres, err := sliceStartEndHandler([]Value{NewInteger(1), NewInteger(3), NewString("hello")}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sres[0].Parent.ConformsTo(TString) {
		t.Errorf("string slice should stay a string, got %s", sres[0].Parent.String())
	}
}

// --- validate ---

func TestValidateHandler(t *testing.T) {
	data := newMap("name", NewString("Alice"))
	spec := newMap("name", NewString("required$"))
	result, err := validateHandler([]Value{data, spec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- walk (no callback) ---

func TestWalkHandler(t *testing.T) {
	data := newMap(
		"a", NewInteger(1),
		"b", newMap("c", NewInteger(2)),
	)
	result, err := walkHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) < 2 {
		t.Errorf("expected at least 2 leaf nodes, got %d", len(list))
	}
	// Each leaf should have "path" and "value" keys
	for _, leaf := range list {
		m, _ := AsMap(leaf)
		if _, ok := m.Get("path"); !ok {
			t.Error("missing 'path' key")
		}
		if _, ok := m.Get("value"); !ok {
			t.Error("missing 'value' key")
		}
	}
}

func TestWalkHandlerEmpty(t *testing.T) {
	data := newMap()
	result, err := walkHandler([]Value{data}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 leaves for empty map, got %d", len(list))
	}
}

// --- Register functions ---

func TestRegisterFunctions(t *testing.T) {
	// After consolidation, registration is driven by a single Natives slice
	// installed via the public Register entry point. Verify each formerly
	// per-word name still resolves to a registered function with at least
	// one signature whose handler is non-nil.
	// The voxgig-struct words (clone, getpath, setpath, inject, merge, walk,
	// items, transform, validate, selector) moved out of core into the
	// boru:struct module — see native/struct_module.go — so they are no
	// longer expected in the global registry.
	// jsonify moved to boru:struct; fetch/prepare/direct moved to boru:net —
	// so they are no longer expected in the global registry.
	names := []string{
		"create", "filter", "flatten", "join", "list", "load",
		"remove", "size", "slice", "update",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			r, err := NewRegistry()
			if err != nil {
				t.Fatal(err)
			}
			Register(r)
			fn := r.Lookup(name)
			if fn == nil {
				t.Fatalf("expected word %q to be registered", name)
			}
			if len(fn.Signatures) == 0 {
				t.Error("expected at least one signature")
			}
			for i, sig := range fn.Signatures {
				if sig.DispatchHandler() == nil {
					t.Errorf("signature %d has nil handler", i)
				}
			}
		})
	}
}

// --- record handlers ---

func TestListRecordAllHandler(t *testing.T) {
	recType := newMap()
	result, err := listRecordAllHandler([]Value{recType}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestListRecordFilterHandler(t *testing.T) {
	recType := newMap()
	filter := newMap("city", NewString("paris"))
	result, err := listRecordFilterHandler([]Value{recType, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestCreateRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	rec := newMap("id", NewString("1"), "name", NewString("Alice"))
	result, err := createRecordHandler([]Value{recType, table, rec}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestLoadRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	filter := newMap("id", NewString("1"))
	result, err := loadRecordHandler([]Value{recType, table, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Returns an empty map
	m, _ := AsMap(result[0])
	if m.Len() != 0 {
		t.Errorf("expected empty map, got %d keys", m.Len())
	}
}

func TestUpdateRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	patch := newMap("id", NewString("1"), "city", NewString("Berlin"))
	result, err := updateRecordHandler([]Value{recType, table, patch}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestRemoveRecordHandler(t *testing.T) {
	recType := newMap()
	table := makeEntityTable(nil)
	filter := newMap("id", NewString("1"))
	result, err := removeRecordHandler([]Value{recType, table, filter}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	list := _lst.Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

// --- helpers for callback-based tests ---

// makeTrueFilterFn creates a boru function that takes one arg of any type
// (filter's Function form hands it the element, or a map entry's value) and
// returns true.
func makeTrueFilterFn() Value {
	return NewFunction(FnDefInfo{
		Signatures: []FnSig{
			{
				Params: []FnParam{
					{Name: "item", Type: TAny},
				},
				Impl: Boru([]Value{NewBoolean(true)}), BarrierPos: -1,
			},
		},
	})
}

func defaultRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	return r
}

// --- filter (callback) ---

func TestFilterHandler(t *testing.T) {
	r := defaultRegistry(t)
	data := newMap("a", NewInteger(1), "b", NewInteger(2))
	fn := makeTrueFilterFn()
	result, err := filterHandler([]Value{fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// All entries pass the filter (fn always returns true), and the Function
	// form over a map keeps the map shape.
	m, _ := AsMap(result[0])
	if m == nil || m.Len() != 2 {
		t.Errorf("expected a 2-key map, got %v", result[0])
	}
}

// --- walk with before callback ---

// makeWalkValueFn creates a boru function that extracts the "value" field
// from the walk node map. Body: [getpath node "value"]
func makeWalkValueFn() Value {
	return NewFunction(FnDefInfo{
		Signatures: []FnSig{
			{
				Params: []FnParam{
					{Name: "node", Type: TMap},
				},
				Impl: Boru([]Value{
					NewWord("getpath"),
					NewString("value"),
					NewWord("node"),
				}), BarrierPos: -1,
			},
		},
	})
}

func TestWalkBeforeHandler(t *testing.T) {
	r := defaultRegistry(t)
	Register(r)
	// getpath moved to boru:struct; the walk callback body uses it, so make
	// it resolvable in this handler-level test registry.
	for _, n := range StructModuleNatives {
		r.RegisterNativeFunc(n)
	}
	data := newMap("a", NewInteger(1))
	fn := makeWalkValueFn()
	result, err := walkBeforeHandler([]Value{fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- walk with before+after callbacks ---

func TestWalkBeforeAfterHandler(t *testing.T) {
	r := defaultRegistry(t)
	Register(r)
	for _, n := range StructModuleNatives {
		r.RegisterNativeFunc(n)
	}
	data := newMap("x", NewString("hello"))
	fn := makeWalkValueFn()
	result, err := walkBeforeAfterHandler([]Value{fn, fn, data}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}
