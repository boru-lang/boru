package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNUR267BodyUndefs pins the stamp's screen: an `undef` anywhere in a
// token body — at its top, in a nested list, in a paren group — keeps the
// body the interpreter's; a body that only defs and reads is stamped.
func TestNUR267BodyUndefs(t *testing.T) {
	undef := []core.Value{core.NewWord("undef"), core.NewWord("x")}
	paren := core.NewValueRaw(core.TParenExpr, core.ParenExprPayload{Toks: undef})
	for _, c := range []struct {
		name string
		toks []core.Value
		want bool
	}{
		{"top level", undef, true},
		{"nested list", []core.Value{core.NewList(undef), core.NewWord("do")}, true},
		{"paren group", []core.Value{paren}, true},
		{"def and read", []core.Value{core.NewWord("def"), core.NewWord("x"), core.NewInteger(5), core.NewWord("x")}, false},
		{"empty", nil, false},
	} {
		if got := bodyUndefs(c.toks); got != c.want {
			t.Errorf("%s: bodyUndefs = %v, want %v", c.name, got, c.want)
		}
	}
	r := newTestRegistry(t)
	if _, ok := StampTokenBody(r, undef, nil, core.SrcPos{}); ok {
		t.Error("a body that undefs is not stamped")
	}
}
