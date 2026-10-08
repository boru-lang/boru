package core

import "testing"

// TestContainsFlexOrStore: the fold-acceptance predicate reads a flex node, a
// weak flex node and a store as references — at the top, inside a map member
// or a list element — and nothing else: a plain map or list, a scalar, a
// value with no concrete payload.
func TestContainsFlexOrStore(t *testing.T) {
	one := NewInteger(1)
	fl := NewFlexList([]Value{one})
	for _, v := range []Value{fl, NewWeakFlexList(), NewStore(TStore)} {
		if !containsFlexOrStore(v) {
			t.Errorf("%v reads as a flex or store", v)
		}
	}
	om := NewOrderedMap()
	om.Set("a", fl)
	if !containsFlexOrStore(NewMap(om)) {
		t.Error("a map holding a flex member reads as one")
	}
	if !containsFlexOrStore(NewList([]Value{one, fl})) {
		t.Error("a list holding a flex element reads as one")
	}
	plainMap := NewOrderedMap()
	plainMap.Set("a", one)
	for _, v := range []Value{one, NewMap(plainMap), NewList([]Value{one}), NewList(nil), {}} {
		if containsFlexOrStore(v) {
			t.Errorf("%v holds no flex or store", v)
		}
	}
}
