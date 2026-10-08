package core

import "testing"

// TestForwardOperandValue pins the drift guards' question (NUR287): the one
// value a forward-collecting word takes for the token after it. A literal is
// itself; a plain word bound to a literal is its bound value, at the word's
// position; a word bound to a fn, a modified word, an unbound name and a
// structural token take nothing.
func TestForwardOperandValue(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("k", NewInteger(1))
	r.Defs.Push("fnk", Value{Parent: TFunction, Data: FnDefInfo{Name: "fnk"}})
	e := NewTop(r)
	lit := NewInteger(7)
	if v, ok := e.ForwardOperandValue(lit); !ok || v.Data != lit.Data {
		t.Errorf("a literal is itself: %v %v", v, ok)
	}
	word := NewWord("k")
	word.SetPos(SrcPos{Row: 1, Col: 9})
	if v, ok := e.ForwardOperandValue(word); !ok || !v.Is(TInteger) || v.Pos().Col != 9 {
		t.Errorf("a word bound to a literal is its value at the word: %v %v", v, ok)
	}
	for name, want := range map[string]Value{"true": NewBoolean(true), "false": NewBoolean(false)} {
		if v, ok := e.ForwardOperandValue(NewWord(name)); !ok || v.Data != want.Data || !v.Parent.Equal(want.Parent) {
			t.Errorf("the reserved literal word %s is its value: %v %v", name, v, ok)
		}
	}
	for name, tok := range map[string]Value{
		"a fn binding":   NewWord("fnk"),
		"a /v word":      NewWordRef("k"),
		"a /s word":      NewWordModified("k", -1, true, false),
		"an unbound one": NewWord("nope"),
		"none":           NewWord("none"),
		"a list":         NewList([]Value{NewInteger(1)}),
	} {
		if _, ok := e.ForwardOperandValue(tok); ok {
			t.Errorf("%s takes nothing", name)
		}
	}
}
