package core

import "testing"

// A /q (QuoteArgs) signature position captures a LITERAL word/atom. In check
// mode a computed operand arrives as a DYNAMIC Any carrier (e.g. `quote
// (s get k)` where the get on a generic Map returns a dynamic Any); such a
// value optimistically conforms to every type (the dynamic-modality rule), so
// without a guard it claims quote's word-capture sig (TAtom, QuoteArgs) and then
// fails to compile instead of falling to the value sig (TAny, ReturnsIdentity).
// The guard rejects a non-concrete carrier at a /q position. This is
// check-mode-only — at runtime the operand is concrete, so positionalMatch is
// unaffected there. See design/legacy/module-fn-checkstate-ownership.2.ignore.
func TestPositionalMatchQuoteArgRejectsNonConcreteCarrier(t *testing.T) {
	dynAny := func() Value { c := NewCarrier(TAny); c.Dynamic = true; return c }

	// A dynamic Any carrier (a computed check-mode value) must NOT match a /q
	// position, even though it optimistically conforms to TAtom.
	qsig := &Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}}
	if positionalMatch([]Value{dynAny()}, qsig) {
		t.Error("a /q position must not match a dynamic Any carrier")
	}
	// A concrete Atom (a real captured word) MUST still match.
	if !positionalMatch([]Value{NewAtom("foo")}, qsig) {
		t.Error("a /q position must match a concrete Atom")
	}
	// A raw Word (the forward-collection word->atom path) MUST still match.
	if !positionalMatch([]Value{NewWord("foo")}, qsig) {
		t.Error("a /q position must match a Word")
	}
	// A genuine ATOM carrier (a non-concrete but atom-typed value, e.g. the
	// result of `quote name` flowing to `set (quote name) v`) MUST still match —
	// the guard rejects only NON-atom carriers, so it must not break a real /q
	// key carrier (regression: storage.tsv `context set (quote name) …`).
	atomCarrier := NewCarrier(TAtom)
	if !positionalMatch([]Value{atomCarrier}, qsig) {
		t.Error("a /q position must match a genuine Atom carrier")
	}

	// Control: the SAME dynamic Any carrier at a NON-/q TAtom position still
	// matches optimistically — the dynamic-modality rule is unchanged elsewhere;
	// the guard is scoped to /q positions only.
	plain := &Signature{Args: []*Type{TAtom}}
	if !positionalMatch([]Value{dynAny()}, plain) {
		t.Error("a non-/q TAtom position should still match a dynamic Any carrier optimistically")
	}
}
