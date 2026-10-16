package native

import "testing"

// --- Object mutability: set mutates Object instance fields ---

func TestObjectSetFieldAtom(t *testing.T) {
	// object {name:String} def Person
	// make Person {name:"Alice"} def alice
	// alice set name "Bob" end
	// alice . name => "Bob"
	r, _ := DefaultRegistry()
	registerIOWords(r)

	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	objType := ClassTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Person",
	}

	instance := ClassInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("name", NewString("Alice"))
			return m
		}(),
	}
	instanceVal := NewClassInstance(TClass, instance)

	// Verify initial value
	result := runBoru(t, r, []Value{
		instanceVal, NewWord("dot"), NewWord("name"),
	})
	_as0, _ := AsString(result[0])
	if len(result) != 1 || _as0 != "Alice" {
		t.Fatalf("initial: got %v, want Alice", result)
	}

	// Mutate: set name "Bob" on the instance
	runBoru(t, r, []Value{
		instanceVal, NewWord("set"), NewWord("name"), NewString("Bob"),
	})

	// Verify mutation persisted (same instance)
	result = runBoru(t, r, []Value{
		instanceVal, NewWord("dot"), NewWord("name"),
	})
	_as1, _ := AsString(result[0])
	if len(result) != 1 || _as1 != "Bob" {
		t.Fatalf("after set: got %v, want Bob", result)
	}
}

func TestObjectSetFieldString(t *testing.T) {
	// set with string key
	r, _ := DefaultRegistry()
	registerIOWords(r)

	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	objType := ClassTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Point",
	}

	instance := ClassInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("x", NewInteger(10))
			return m
		}(),
	}
	instanceVal := NewClassInstance(TClass, instance)

	// Mutate via string key
	result := runBoru(t, r, []Value{
		instanceVal, NewWord("set"), NewString("x"), NewInteger(99),
	})
	_ = result

	// Verify
	result = runBoru(t, r, []Value{
		instanceVal, NewWord("get"), NewString("x"),
	})
	_as2, _ := AsInteger(result[0])
	if len(result) != 1 || _as2 != 99 {
		t.Fatalf("got %v, want 99", result)
	}
}

func TestObjectMutationSharedReference(t *testing.T) {
	// Two references to the same Object instance see mutations
	r, _ := DefaultRegistry()
	registerIOWords(r)

	fields := NewOrderedMap()
	fields.Set("v", NewTypeLiteral(TInteger))
	objType := ClassTypeInfo{
		Fields: fields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Counter",
	}

	instance := ClassInstanceInfo{
		TypeRef: &objType,
		Fields: func() *OrderedMap {
			m := NewOrderedMap()
			m.Set("v", NewInteger(0))
			return m
		}(),
	}
	ref1 := NewClassInstance(TClass, instance)
	ref2 := NewClassInstance(TClass, instance) // same underlying Fields pointer

	// Mutate via ref1
	runBoru(t, r, []Value{
		ref1, NewWord("set"), NewWord("v"), NewInteger(42),
	})

	// Read via ref2 — should see the mutation
	result := runBoru(t, r, []Value{
		ref2, NewWord("dot"), NewWord("v"),
	})
	_as4, _ := AsInteger(result[0])
	if len(result) != 1 || _as4 != 42 {
		t.Fatalf("ref2 got %v, want 42 (shared mutation)", result)
	}
}

// --- Node immutability: set is copy-returning on Map, rejected on List ---

func TestNodeMapIsImmutable(t *testing.T) {
	// `set` on a Map is copy-returning: the result is a NEW map with
	// the key bound, and the receiver Map is untouched. (The original
	// contract — set rejects Map outright — was replaced 2026-06-09:
	// Map keeps its immutability, but the obvious word now works, the
	// way push does. In-place semantics remain exclusive to the
	// mutable containers; see TestNodeListIsImmutable below.)
	r, _ := DefaultRegistry()
	registerIOWords(r)
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	result := runBoru(t, r, []Value{
		mapVal, NewWord("set"), NewWord("x"), NewInteger(99),
	})
	if len(result) != 1 {
		t.Fatalf("expected one result (the new map), got %d", len(result))
	}
	out, err := AsMap(result[0])
	if err != nil || out == nil {
		t.Fatalf("expected a map result, got %v", result[0])
	}
	v, ok := out.Get("x")
	if !ok {
		t.Fatal("new map is missing key x")
	}
	if n, _ := AsInteger(v); n != 99 {
		t.Fatalf("new map: expected x=99, got %v", v)
	}
	// The receiver is NOT mutated.
	rv, _ := m.Get("x")
	if n, _ := AsInteger(rv); n != 1 {
		t.Fatalf("receiver map was mutated: x=%v, want 1", rv)
	}
}

func TestNodeListIsImmutable(t *testing.T) {
	// `set` on a List is copy-returning (completing the immutable
	// column: Map and List both return the updated copy). The result
	// is a NEW list; the receiver is untouched. (The original
	// contract — set rejects List outright — was replaced 2026-06-09.)
	r, _ := DefaultRegistry()
	registerIOWords(r)
	listVal := NewList([]Value{NewInteger(10), NewInteger(20)})

	result := runBoru(t, r, []Value{
		listVal, NewWord("set"), NewInteger(0), NewInteger(99),
	})
	if len(result) != 1 {
		t.Fatalf("expected one result (the new list), got %d", len(result))
	}
	out, err := AsList(result[0])
	if err != nil || out.IsNil() {
		t.Fatalf("expected a list result, got %v", result[0])
	}
	if n, _ := AsInteger(out.Get(0)); n != 99 {
		t.Errorf("new list: expected [0]=99, got %d", n)
	}
	// The receiver is NOT mutated.
	orig, _ := AsList(listVal)
	if n, _ := AsInteger(orig.Get(0)); n != 10 {
		t.Errorf("receiver list was mutated: [0]=%d, want 10", n)
	}
}

func TestNodeMapUnchangedAfterObjectSet(t *testing.T) {
	// Creating a Map, storing it, and doing set on an Object should not
	// affect the Map — they are different types with different semantics.
	r, _ := DefaultRegistry()
	registerIOWords(r)

	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	// Map value remains unchanged
	result := runBoru(t, r, []Value{
		mapVal, NewWord("dot"), NewWord("x"),
	})
	_as5, _ := AsInteger(result[0])
	if len(result) != 1 || _as5 != 1 {
		t.Fatalf("map x: got %v, want 1", result)
	}
}

// --- ReadMap interface enforces Node immutability at compile time ---

func TestAsMapReturnsReadMap(t *testing.T) {
	// AsMap() returns ReadMap which has no Set or Delete methods.
	// This test verifies the interface at the type level.
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	mapVal := NewMap(m)

	rm, _ := AsMap(mapVal)
	if rm == nil {
		t.Fatal("AsMap returned nil")
	}

	// ReadMap supports Get, Keys, SortedKeys, Len
	v, ok := rm.Get("x")
	_as6, _ := AsInteger(v)
	if !ok || _as6 != 1 {
		t.Fatalf("Get x: got %v, want 1", v)
	}
	if rm.Len() != 1 {
		t.Fatalf("Len: got %d, want 1", rm.Len())
	}
	if len(rm.Keys()) != 1 || rm.Keys()[0] != "x" {
		t.Fatalf("Keys: got %v, want [x]", rm.Keys())
	}

	// AsMutableMap() returns *OrderedMap for raw map data (internal use)
	rawMap := NewMap(m)
	om, err := AsMutableMap(rawMap)
	if err != nil {
		t.Fatalf("AsMutableMap returned err for raw map: %v", err)
	}
	// *OrderedMap supports Set (for internal construction paths)
	om.Set("y", NewInteger(2))
	v2, ok2 := om.Get("y")
	_as7, _ := AsInteger(v2)
	if !ok2 || _as7 != 2 {
		t.Fatalf("after mutation: got %v, want 2", v2)
	}

	// Object instances expose Fields directly (not through AsMap/AsMutableMap)
	objFields := NewOrderedMap()
	objFields.Set("v", NewInteger(0))
	objType := ClassTypeInfo{
		Fields: objFields,
		ID:     GenerateObjectTypeID(),
		Name:   "Object/Test",
	}
	inst := NewClassInstance(TClass, ClassInstanceInfo{
		TypeRef: &objType,
		Fields:  objFields,
	})
	// AsMap returns an error for Object (Data is ClassInstanceInfo, not *OrderedMap)
	if _m, err := AsMap(inst); err == nil && _m != nil {
		t.Fatal("AsMap should return nil/error for Object instance")
	}
	// But Fields are mutable via the ClassInstanceInfo
	oi := DataOf(&inst).(ClassInstanceInfo)
	oi.Fields.Set("v", NewInteger(42))
	v3, _ := oi.Fields.Get("v")
	_as8, _ := AsInteger(v3)
	if _as8 != 42 {
		t.Fatalf("object field mutation: got %v, want 42", v3)
	}
}

// --- ReadList interface enforces List immutability at compile time ---

func TestAsListReturnsReadList(t *testing.T) {
	listVal := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})

	rl, _ := AsList(listVal)
	if rl.IsNil() {
		t.Fatal("AsList returned nil ReadList")
	}

	// ReadList supports Get, Len, Slice, IsNil
	if rl.Len() != 3 {
		t.Fatalf("Len: got %d, want 3", rl.Len())
	}
	_as9, _ := AsInteger(rl.Get(0))
	if _as9 != 1 {
		t.Fatalf("Get(0): got %v, want 1", rl.Get(0))
	}
	_as10, _ := AsInteger(rl.Get(2))
	if _as10 != 3 {
		t.Fatalf("Get(2): got %v, want 3", rl.Get(2))
	}

	// Slice returns a copy — mutating the copy doesn't affect the original
	sliceCopy := rl.Slice()
	sliceCopy[0] = NewInteger(99)
	_as11, _ := AsInteger(rl.Get(0))
	if _as11 != 1 {
		t.Fatal("mutating Slice() copy should not affect ReadList")
	}
}

// --- Store copy-on-write ---

func TestStoreCOWBasic(t *testing.T) {
	// set on a Store creates a new COW layer; get resolves through prototype.
	r, _ := DefaultRegistry()
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("k"), NewInteger(7),
		NewEnd(),
		NewWord("context"), NewWord("dot"), NewWord("k"),
	})
	_as15, _ := AsInteger(result[0])
	if len(result) != 1 || _as15 != 7 {
		t.Fatalf("got %v, want 7", result)
	}
}

func TestStoreCOWDoesNotMutateOriginal(t *testing.T) {
	// After COW set, the original Store is unchanged.
	store := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"x": NewInteger(1)},
	}
	r, _ := DefaultRegistry()
	registerIOWords(r)
	r.InitRootContext()
	// Put store in context
	ctx := r.Contexts.Top()
	ctx.Set("s", NewStoreValue(TStore, store))

	e := New(r)
	// Get the nested store, set a key on it
	_, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("s"),
		NewWord("set"), NewWord("y"), NewInteger(99),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The original store should NOT have key "y"
	if _, ok := store.Data["y"]; ok {
		t.Fatal("COW violated: original store was mutated")
	}
	// Original "x" is still 1
	_as16, _ := AsInteger(store.Data["x"])
	if _as16 != 1 {
		t.Fatal("original store's x was changed")
	}
}

func TestStoreCOWParentPropagation(t *testing.T) {
	// Nested stores: context → parent → child
	// Set on child should COW child AND propagate to parent in context.
	r, _ := DefaultRegistry()
	registerIOWords(r)
	r.InitRootContext()
	ctx := r.Contexts.Top()

	child := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	child.Set("val", NewInteger(10))
	parent := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     make(map[string]Value),
	}
	parent.Set("child", NewStoreValue(TStore, child))
	ctx.Set("parent", NewStoreValue(TStore, parent))

	e := New(r)
	// context get parent → get child → set val 42
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("parent"),
		NewWord("dot"), NewWord("child"),
		NewWord("set"), NewWord("val"), NewInteger(42),
		NewEnd(),
		// Now read it back through the context
		NewWord("context"), NewWord("dot"), NewWord("parent"),
		NewWord("dot"), NewWord("child"),
		NewWord("dot"), NewWord("val"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as17, _ := AsInteger(result[0])
	if len(result) != 1 || _as17 != 42 {
		t.Fatalf("got %v, want 42 (COW propagated through parent)", result)
	}

	// Original child store should be unchanged
	_as18, _ := AsInteger(child.Data["val"])
	if _as18 != 10 {
		t.Fatal("COW violated: original child was mutated")
	}
}

func TestStoreCOWPrototypeResolution(t *testing.T) {
	// After COW, unchanged keys resolve through the prototype chain.
	r, _ := DefaultRegistry()
	registerIOWords(r)
	r.InitRootContext()
	ctx := r.Contexts.Top()

	store := &StoreInstanceInfo{
		TypeName: "Object/Store",
		Data:     map[string]Value{"a": NewInteger(1), "b": NewInteger(2)},
	}
	ctx.Set("s", NewStoreValue(TStore, store))

	e := New(r)
	// Set "a" to 99, then read both "a" and "b"
	result, err := e.Run([]Value{
		NewWord("context"), NewWord("dot"), NewWord("s"),
		NewWord("set"), NewWord("a"), NewInteger(99),
		NewEnd(),
		// Read "a" (from COW layer)
		NewOpenParen(),
		NewWord("context"), NewWord("dot"), NewWord("s"),
		NewWord("dot"), NewWord("a"),
		NewCloseParen(),
		// Read "b" (from prototype, unchanged)
		NewOpenParen(),
		NewWord("context"), NewWord("dot"), NewWord("s"),
		NewWord("dot"), NewWord("b"),
		NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	_as19, _ := AsInteger(result[0])
	if _as19 != 99 {
		t.Errorf("a = %v, want 99 (from COW layer)", result[0])
	}
	_as20, _ := AsInteger(result[1])
	if _as20 != 2 {
		t.Errorf("b = %v, want 2 (from prototype)", result[1])
	}
}
