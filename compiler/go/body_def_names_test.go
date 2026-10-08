package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestBodyDefNames pins the token-body def scanner a keep-defs stamp uses
// to leave the body's own bindings out of its dependency snapshot
// (stampDetachedSig, NUR202): a `def NAME`, nested lists included, and the
// `var` splice's three declaration forms — a bare word, a `[name value]`
// pair, a string — while a `def` with no name after it and a `var` whose
// operand is not a declaration list contribute nothing.
func TestBodyDefNames(t *testing.T) {
	w := core.NewWord
	l := func(vs ...core.Value) core.Value { return core.NewList(vs) }
	body := []core.Value{
		w("def"), w("t"), l(w("t"), w("add"), core.NewInteger(1)), w("t"),
		l(w("if"), w("c"), l(w("def"), w("u"), core.NewInteger(5)), l()),
		w("var"), l(l(w("a"), l(w("b"), core.NewInteger(2)), core.NewString("c")), w("a")),
		w("var"), core.NewInteger(3), // not a declaration list: nothing
		w("var"), l(core.NewInteger(4)), // first element not a list: nothing
		w("def"), // no name follows: nothing
	}
	got := bodyDefNames(body)
	for _, name := range []string{"t", "u", "a", "b", "c"} {
		if !got[name] {
			t.Errorf("bodyDefNames: missing %q in %v", name, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("bodyDefNames: want exactly {t u a b c}, got %v", got)
	}
	// A declaration the splice would reject (a number) names nothing.
	if got := bodyDefNames([]core.Value{w("var"), l(l(core.NewInteger(1)), w("x"))}); len(got) != 0 {
		t.Errorf("bodyDefNames: a non-name declaration must name nothing, got %v", got)
	}
}
