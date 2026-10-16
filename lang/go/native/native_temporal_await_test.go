package native

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// await mode:'all (default) tests
// =============================================================================

func TestAwaitAllDefault(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// await [[1 add 2] [3 add 4]]
	// Both branches succeed → [3, 7]
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
		NewList([]Value{NewInteger(3), NewWord("add"), NewInteger(4)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	list, _ := AsList(result[0])
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}
	v0 := list.Get(0)
	v1 := list.Get(1)
	i0, _ := AsInteger(v0)
	i1, _ := AsInteger(v1)
	if i0 != 3 || i1 != 7 {
		t.Errorf("expected [3, 7], got [%d, %d]", i0, i1)
	}
}

func TestAwaitAllWithError(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// await [[1 add 2] [1 div 0]]
	// Second branch errors → result is the error
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !IsError(result[0]) {
		t.Fatalf("expected error value, got %s", result[0].String())
	}
}

func TestAwaitAllRunsInParallel(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// Each branch sleeps 50ms. If parallel, total should be ~50ms, not ~150ms.
	inner := NewList([]Value{
		NewList([]Value{NewWord("sleep"), NewInteger(50), NewInteger(1)}),
		NewList([]Value{NewWord("sleep"), NewInteger(50), NewInteger(2)}),
		NewList([]Value{NewWord("sleep"), NewInteger(50), NewInteger(3)}),
	})
	SetQuoted(&inner, true)

	start := time.Now()
	result, err := e.Run([]Value{NewWord("await"), inner})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	list, _ := AsList(result[0])
	if list.IsNil() || list.Len() != 3 {
		t.Fatalf("expected list of 3, got %v", result[0])
	}
	// Should complete in roughly 50ms, not 150ms.
	if elapsed > 120*time.Millisecond {
		t.Errorf("expected parallel execution (~50ms), took %v", elapsed)
	}
}

func TestAwaitAllEmpty(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	inner := NewList([]Value{})
	SetQuoted(&inner, true)
	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	list, _ := AsList(result[0])
	if list.Len() != 0 {
		t.Fatalf("expected empty list, got %v", result[0])
	}
}

// =============================================================================
// await mode:'full tests
// =============================================================================

func TestAwaitFull(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// Build options: make Options {mode:'full}
	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("full"))
	optsVal := NewOptionsType(optsFields)

	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}),
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}

	list, _ := AsList(result[0])
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2 results, got %v", result[0])
	}

	// First result: {status:ok, value:3}
	r0 := list.Get(0)
	m0, _ := AsMap(r0)
	if m0 == nil {
		t.Fatalf("expected map for result[0], got %v", r0)
	}
	status0, _ := m0.Get("status")
	s0, _ := AsAtom(status0)
	if s0 != "ok" {
		t.Errorf("expected status 'ok', got %q", s0)
	}
	val0, _ := m0.Get("value")
	iv0, _ := AsInteger(val0)
	if iv0 != 3 {
		t.Errorf("expected value 3, got %d", iv0)
	}

	// Second result: {status:error, value:error(...)}
	r1 := list.Get(1)
	m1, _ := AsMap(r1)
	if m1 == nil {
		t.Fatalf("expected map for result[1], got %v", r1)
	}
	status1, _ := m1.Get("status")
	s1, _ := AsAtom(status1)
	if s1 != "error" {
		t.Errorf("expected status 'error', got %q", s1)
	}
	val1, _ := m1.Get("value")
	if !IsError(val1) {
		t.Errorf("expected error value, got %v", val1)
	}
}

// =============================================================================
// await mode:'first tests
// =============================================================================

func TestAwaitFirst(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)

	// Register a word that records call order.
	var order atomic.Int32
	reg.Register("testorder", Signature{
		Args: []*Type{},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(int64(order.Add(1)))}, nil
		}), BarrierPos: 0,
	})

	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("first"))
	optsVal := NewOptionsType(optsFields)

	// First branch is fast (no sleep), second is slow.
	inner := NewList([]Value{
		NewList([]Value{NewInteger(42)}),
		NewList([]Value{NewWord("sleep"), NewInteger(100), NewInteger(99)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	// The fast branch (42) should win.
	v, _ := AsInteger(result[0])
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestAwaitFirstWithError(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("first"))
	optsVal := NewOptionsType(optsFields)

	// First branch errors immediately, second succeeds but slowly.
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewList([]Value{NewWord("sleep"), NewInteger(50), NewInteger(99)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First (the error) should win since it finishes first.
	if len(result) != 1 || !IsError(result[0]) {
		t.Fatalf("expected error value from first-to-finish, got %v", result)
	}
}

// =============================================================================
// await mode:'any tests
// =============================================================================

func TestAwaitAny(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("any"))
	optsVal := NewOptionsType(optsFields)

	// First branch errors, second succeeds.
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewList([]Value{NewInteger(42)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	// Should get the successful result (42), not the error.
	v, _ := AsInteger(result[0])
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestAwaitAnyAllReject(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("any"))
	optsVal := NewOptionsType(optsFields)

	// All branches error.
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewList([]Value{NewInteger(2), NewWord("div"), NewInteger(0)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !IsError(result[0]) {
		t.Fatalf("expected error when all reject, got %v", result)
	}
}

func TestAwaitAnyFirstSuccess(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("any"))
	optsVal := NewOptionsType(optsFields)

	// First branch is slow error, second is fast success.
	inner := NewList([]Value{
		NewList([]Value{NewWord("sleep"), NewInteger(100), NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewList([]Value{NewInteger(7)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, _ := AsInteger(result[0])
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

// =============================================================================
// await with sleep for timing tests
// =============================================================================

func TestAwaitWithSleep(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// Two branches that sleep then return a value.
	// sleep 20 consumes 20 as forward arg. Then 1 remains on stack.
	inner := NewList([]Value{
		NewList([]Value{NewWord("sleep"), NewInteger(20), NewInteger(1)}),
		NewList([]Value{NewWord("sleep"), NewInteger(40), NewInteger(2)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, _ := AsList(result[0])
	v0 := list.Get(0)
	v1 := list.Get(1)
	i0, _ := AsInteger(v0)
	i1, _ := AsInteger(v1)
	if i0 != 1 || i1 != 2 {
		t.Errorf("expected [1, 2], got [%d, %d]", i0, i1)
	}
}

// =============================================================================
// await error handling edge cases
// =============================================================================

func TestAwaitAllMultipleErrors(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// All branches error — should return the first error in order.
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewWord("div"), NewInteger(0)}),
		NewList([]Value{NewInteger(2), NewWord("div"), NewInteger(0)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !IsError(result[0]) {
		t.Fatalf("expected error, got %v", result)
	}
}

func TestAwaitFullAllSucceed(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("full"))
	optsVal := NewOptionsType(optsFields)

	inner := NewList([]Value{
		NewList([]Value{NewInteger(10)}),
		NewList([]Value{NewInteger(20)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, _ := AsList(result[0])
	for i := 0; i < list.Len(); i++ {
		v := list.Get(i)
		m, _ := AsMap(v)
		if m == nil {
			t.Fatalf("expected map at [%d], got %v", i, v)
		}
		st, _ := m.Get("status")
		s, _ := AsAtom(st)
		if s != "ok" {
			t.Errorf("expected status ok at [%d], got %q", i, s)
		}
	}
}

// =============================================================================
// await invalid mode
// =============================================================================

func TestAwaitInvalidMode(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	optsFields := NewOrderedMap()
	optsFields.Set("mode", NewAtom("bogus"))
	optsVal := NewOptionsType(optsFields)

	inner := NewList([]Value{
		NewList([]Value{NewInteger(1)}),
	})
	SetQuoted(&inner, true)

	_, err := e.Run([]Value{NewWord("await"), optsVal, inner})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should mention invalid mode, got: %v", err)
	}
}

// =============================================================================
// await type literal no-panic
// =============================================================================

func TestAwaitTypeLiteralNoPanic(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	e := NewTop(reg)
	// Pass a type literal (Data==nil) — should error, not panic.
	_, _ = e.Run([]Value{NewTypeLiteral(TList), NewWord("await")})
}

// =============================================================================
// await non-list elements
// =============================================================================

func TestAwaitNonListElement(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// Non-list elements are returned as-is.
	inner := NewList([]Value{
		NewInteger(42),
		NewString("hello"),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, _ := AsList(result[0])
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}
	v0 := list.Get(0)
	i0, _ := AsInteger(v0)
	if i0 != 42 {
		t.Errorf("expected 42, got %d", i0)
	}
	v1 := list.Get(1)
	s1, _ := AsString(v1)
	if s1 != "hello" {
		t.Errorf("expected 'hello', got %q", s1)
	}
}

// =============================================================================
// await with multi-value branch results
// =============================================================================

func TestAwaitMultiValueBranch(t *testing.T) {
	reg, _ := DefaultRegistry()
	registerIOWords(reg)
	e := NewTop(reg)

	// Branch that leaves multiple values on the stack: [1 2 3]
	inner := NewList([]Value{
		NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)}),
		NewList([]Value{NewInteger(42)}),
	})
	SetQuoted(&inner, true)

	result, err := e.Run([]Value{NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, _ := AsList(result[0])
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}

	// First result: multi-value → wrapped in a list [1,2,3]
	v0 := list.Get(0)
	sublist, _ := AsList(v0)
	if sublist.IsNil() || sublist.Len() != 3 {
		t.Fatalf("expected sub-list of 3 for multi-value branch, got %v", v0)
	}

	// Second result: single value → unwrapped
	v1 := list.Get(1)
	i1, _ := AsInteger(v1)
	if i1 != 42 {
		t.Errorf("expected 42, got %d", i1)
	}
}
