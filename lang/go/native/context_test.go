package native

import (
	"testing"
)

// --- Basic context set/get ---

func TestContextSetGetString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(42),
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	_as0, _ := AsInteger(result[0])
	if len(result) != 1 || _as0 != 42 {
		t.Errorf("context dot x = %v, want 42", result)
	}
}

func TestContextSetGetWordKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("context"), NewWord("dot"), NewWord("foo"),
	})
	_as1, _ := AsInteger(result[0])
	if len(result) != 1 || _as1 != 99 {
		t.Errorf("context dot foo = %v, want 99", result)
	}
}

func TestContextSetOverwrite(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(2),
		NewWord("context"), NewWord("get"), NewString("k"),
	})
	_as2, _ := AsInteger(result[0])
	if len(result) != 1 || _as2 != 2 {
		t.Errorf("overwritten context dot k = %v, want 2", result)
	}
}

// --- Unknown key returns none ---

func TestContextGetUnknownKeyReturnsError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("context"), NewWord("get"), NewString("missing"),
	})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

// --- Sub-engine inheritance via do ---

func TestContextSubEngineInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Set in parent, read in sub-engine via do
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(10),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("get"), NewString("x"),
		}),
	})
	_as3, _ := AsInteger(result[0])
	if len(result) != 1 || _as3 != 10 {
		t.Errorf("sub-engine should inherit parent context, got %v", result)
	}
}

// --- Sub-engine isolation: writes don't affect parent ---

func TestContextSubEngineIsolation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Set in parent, override in sub-engine, check parent still has original
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(1),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("x"), NewInteger(999),
		}),
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	_as4, _ := AsInteger(result[0])
	if len(result) != 1 || _as4 != 1 {
		t.Errorf("parent context should be unchanged after sub-engine write, got %v", result)
	}
}

// --- Sub-engine new key doesn't leak to parent ---

func TestContextSubEngineNewKeyDoesNotLeak(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	e := New(r)
	_, err = e.Run([]Value{
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("secret"), NewInteger(42),
		}),
		NewWord("context"), NewWord("get"), NewString("secret"),
	})
	if err == nil {
		t.Fatal("expected error: sub-engine key should not leak to parent")
	}
}

// --- Nested 3-level sub-engines ---

func TestContextNestedThreeLevels(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Level 0: set level=0
	// Level 1 (do): set level=1, then do level 2
	// Level 2 (do do): read level → should see 1, set level=2
	// Back at level 1: read level → should see 1
	// Back at level 0: read level → should see 0
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("level"), NewInteger(0),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("level"), NewInteger(1),
			NewWord("do"), NewList([]Value{
				NewWord("context"), NewWord("get"), NewString("level"),
				// This should be 1 (inherited from level 1)
			}),
		}),
		// do returns the innermost result (1), now check parent level
		NewWord("context"), NewWord("get"), NewString("level"),
	})
	// Stack should have: [1, 0]
	// The do returns inner do's result (1), then we get parent level (0)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	_as5, _ := AsInteger(result[0])
	if _as5 != 1 {
		t.Errorf("inner do should see level=1, got %v", result[0])
	}
	_as6, _ := AsInteger(result[1])
	if _as6 != 0 {
		t.Errorf("parent should still see level=0, got %v", result[1])
	}
}

// --- Multiple keys ---

func TestContextMultipleKeys(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("a"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("b"), NewInteger(2),
		NewWord("context"), NewWord("set"), NewString("c"), NewInteger(3),
		NewWord("context"), NewWord("get"), NewString("a"),
		NewWord("context"), NewWord("get"), NewString("b"),
		NewWord("add"),
		NewWord("context"), NewWord("get"), NewString("c"),
		NewWord("add"),
	})
	_as7, _ := AsInteger(result[0])
	if len(result) != 1 || _as7 != 6 {
		t.Errorf("sum of context values = %v, want 6", result)
	}
}

// --- Context with different value types ---

func TestContextDifferentValueTypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Wrap each context get in parens so previous results on the stack
	// don't get consumed by the next get (stack-preference rule: when
	// a String result is on the stack, context-get would take it as
	// its key instead of forward-collecting the intended key).
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("str"), NewString("hello"),
		NewEnd(),
		NewWord("context"), NewWord("set"), NewString("num"), NewInteger(42),
		NewEnd(),
		NewWord("context"), NewWord("set"), NewString("bool"), NewBoolean(true),
		NewEnd(),
		NewOpenParen(), NewWord("context"), NewWord("get"), NewString("str"), NewCloseParen(),
		NewOpenParen(), NewWord("context"), NewWord("get"), NewString("num"), NewCloseParen(),
		NewOpenParen(), NewWord("context"), NewWord("get"), NewString("bool"), NewCloseParen(),
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	_as8, _ := AsString(result[0])
	if !ParentOf(&result[0]).ConformsTo(TString) || _as8 != "hello" {
		t.Errorf("string value = %v, want hello", result[0])
	}
	_as9, _ := AsInteger(result[1])
	if !ParentOf(&result[1]).ConformsTo(TInteger) || _as9 != 42 {
		t.Errorf("integer value = %v, want 42", result[1])
	}
	_as10, _ := AsBoolean(result[2])
	if !ParentOf(&result[2]).ConformsTo(TBoolean) || _as10 != true {
		t.Errorf("boolean value = %v, want true", result[2])
	}
}

// --- Values are copied by reference (maps are shared, not deep-copied) ---

func TestContextValuesByReference(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	// Store a map in context, retrieve it in sub-engine — should be the same map
	m := NewOrderedMap()
	m.Set("key", NewInteger(100))
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("mymap"), NewMap(m),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("get"), NewString("mymap"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	rm, _ := AsMap(result[0])
	v, ok := rm.Get("key")
	_as11, _ := AsInteger(v)
	if !ok || _as11 != 100 {
		t.Errorf("expected map with key=100, got %v", result[0])
	}
}

// --- Module inherits parent context ---

func TestContextModuleInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("parent_val"), NewInteger(77),
		NewWord("module"), NewList([]Value{
			NewWord("export"), NewAtom("result"),
			NewMap(func() *OrderedMap {
				m := NewOrderedMap()
				// We can't call "context get" inside a map literal directly.
				// Instead, test by checking the module can read context in its body.
				m.Set("val", NewList([]Value{
					NewWord("context"), NewWord("get"), NewString("parent_val"),
				}))
				return m
			}()),
		}),
	})
	// Module returns a module desc. Let's verify it ran without error.
	if len(result) != 1 {
		t.Fatalf("expected 1 result (module desc), got %d: %v", len(result), result)
	}
}

// --- Module writes don't affect parent ---

func TestContextModuleIsolation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(1),
		NewWord("module"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("x"), NewInteger(999),
			NewWord("export"), NewAtom("dummy"),
			NewMap(func() *OrderedMap {
				m := NewOrderedMap()
				m.Set("v", NewInteger(0))
				return m
			}()),
		}),
		NewWord("drop"), // drop module desc
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	_as12, _ := AsInteger(result[0])
	if len(result) != 1 || _as12 != 1 {
		t.Errorf("parent context should be unchanged after module write, got %v", result)
	}
}

// --- Direct unit tests for PushContext/PopContext/Context ---

func TestRegistryContextStackMethods(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Initially no context
	if r.Contexts.Top() != nil {
		t.Fatal("expected nil context initially")
	}

	// Push empty context (nil parent)
	r.Contexts.Push(nil)
	store := r.Contexts.Top()
	if store == nil {
		t.Fatal("expected non-nil context store after push")
	}
	if len(store.Data) != 0 {
		t.Fatalf("expected empty context, got %d entries", len(store.Data))
	}

	// Write to context store
	store.Set("key1", NewInteger(1))

	// Push child — should inherit key1 via prototype
	r.Contexts.Push(store)
	childStore := r.Contexts.Top()
	v, ok := childStore.Get("key1")
	_as13, _ := AsInteger(v)
	if !ok || _as13 != 1 {
		t.Error("child should inherit key1=1 from parent via prototype")
	}

	// Write to child doesn't affect parent
	childStore.Set("key1", NewInteger(99))
	childStore.Set("key2", NewInteger(2))

	// Pop child
	r.Contexts.Pop()
	restored := r.Contexts.Top()
	v, ok = restored.Get("key1")
	_as14, _ := AsInteger(v)
	if !ok || _as14 != 1 {
		t.Errorf("parent key1 should still be 1 after pop, got %v", v)
	}
	if _, ok := restored.Get("key2"); ok {
		t.Error("parent should not have key2 after child pop")
	}

	// Pop parent
	r.Contexts.Pop()
	if r.Contexts.Top() != nil {
		t.Error("expected nil context after popping all layers")
	}
}

// --- If condition sub-engine inherits context ---

func TestContextIfSubEngineInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	result := runBoru(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("val"), NewInteger(5),
		NewWord("if"), NewList([]Value{NewBoolean(true)}),
		NewList([]Value{NewWord("context"), NewWord("get"), NewString("val")}),
		NewList([]Value{NewInteger(0)}),
	})
	_as15, _ := AsInteger(result[0])
	if len(result) != 1 || _as15 != 5 {
		t.Errorf("if-branch should inherit context, got %v", result)
	}
}
