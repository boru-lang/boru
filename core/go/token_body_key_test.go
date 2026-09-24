package core

import "testing"

// TestTokenBodyKey: a body of words and scalars is named by its text with
// positions; a body carrying a reference value by its ID; one with neither
// is not named. An input with no type reads as Any.
func TestTokenBodyKey(t *testing.T) {
	add := NewWord("add")
	one := NewInteger(1)
	text := []Value{add, one, NewList([]Value{NewString("s")})}
	body := NewList(text)
	body.ID = ""
	k, ok := TokenBodyKey(body, text)
	if !ok || k[:4] != "txt:" {
		t.Fatalf("a text body keys by its text: %q %v", k, ok)
	}
	om := NewOrderedMap()
	om.Set("a", one)
	ref := []Value{NewMap(om), NewWord("size")}
	withID := NewList(ref)
	withID.ID = "L_body"
	if k, ok := TokenBodyKey(withID, ref); !ok || k != "id:L_body" {
		t.Fatalf("a reference-bearing body keys by its ID: %q %v", k, ok)
	}
	withID.ID = ""
	if _, ok := TokenBodyKey(withID, ref); ok {
		t.Fatal("a reference-bearing body with no ID is not named")
	}
	if TokenBodyContentKeyable([]Value{NewList(ref)}) {
		t.Fatal("a reference inside a nested list is not identity-free")
	}
	if TokenBodyInputType(Value{}) != TAny {
		t.Fatal("an input with no type declares Any")
	}
	if TokenBodyInputType(one) != TInteger {
		t.Fatal("a typed input declares its concrete type")
	}
}
