package core

import "testing"

// containsCapturingFn finds a closure — a fn value with captures — at any
// depth of a container, and nothing else (the map-literal const fold's
// exclusion, the container-member calls, 2026-09-22).
func TestContainsCapturingFn(t *testing.T) {
	plain := Value{Parent: TFunction, Data: FnDefInfo{Name: "inc"}}
	closure := Value{Parent: TFunction, Data: FnDefInfo{Name: "adder", Captured: []CapturedBinding{{Name: "k", Value: NewInteger(1)}}}}
	if containsCapturingFn(plain) {
		t.Error("a capture-free fn value is not a closure")
	}
	if !containsCapturingFn(closure) {
		t.Error("a fn value with captures is a closure")
	}
	if containsCapturingFn(NewInteger(5)) || containsCapturingFn(NewCarrier(TMap)) {
		t.Error("data and carriers hold no closure")
	}
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", closure)
	if !containsCapturingFn(NewMap(m)) {
		t.Error("a map holding a closure member contains one")
	}
	m2 := NewOrderedMap()
	m2.Set("a", plain)
	if containsCapturingFn(NewMap(m2)) {
		t.Error("a map of capture-free fn values contains none")
	}
	if !containsCapturingFn(NewList([]Value{NewInteger(1), NewList([]Value{closure})})) {
		t.Error("a nested list holding a closure contains one")
	}
	if containsCapturingFn(NewList([]Value{NewInteger(1)})) {
		t.Error("a list of data contains none")
	}
}
