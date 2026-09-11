package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The arm-resident TYPE twin's element-independence screen, driven
// directly. The corpus and the cross-request parity oracle exercise the
// admitted shape end to end; these arms pin each half of the decision in
// isolation, and — because every decline REFUSES a program the interpreter
// runs — the negatives matter as much as the positives.
func TestTypeInstallElementIndependence(t *testing.T) {
	at := func(v core.Value, row, col int) core.Value {
		v.SetPos(core.SrcPos{Row: row, Col: col})
		return v
	}
	word := func(name string, row, col int) core.Value { return at(core.NewWord(name), row, col) }
	defPos := core.SrcPos{Row: 1, Col: 1}

	// `def Big (Integer gt 5)` — the admitted shape: a paren of two words
	// the body does not bind and one literal.
	expr := at(core.NewParenExpr([]core.Value{
		word("Integer", 1, 10), word("gt", 1, 18), at(core.NewInteger(5), 1, 21),
	}), 1, 9)
	body := core.NewList([]core.Value{word("def", 1, 1), word("Big", 1, 5), expr})

	empty := &EmitFragment{}
	if !typeInstallElementIndependent(body, empty, defPos, map[string]bool{}) {
		t.Fatal("a literal-bounded refinement over unbound words must pass the screen")
	}
	// THROUGH A NAME: the body binds `gt`'s operand, so each element's node
	// differs and the screen must decline.
	depExpr := at(core.NewParenExpr([]core.Value{
		word("Integer", 1, 10), word("gt", 1, 18), word("e", 1, 21),
	}), 1, 9)
	depBody := core.NewList([]core.Value{word("def", 1, 1), word("Big", 1, 5), depExpr})
	if typeInstallElementIndependent(depBody, empty, defPos, map[string]bool{"e": true}) {
		t.Fatal("a bound whose word the body binds is per-element and must decline")
	}
	// THROUGH A DISPATCH: an event recorded at a position inside the
	// expression means the check pass could not fold that call, so it may
	// read anything at run time.
	live := &EmitFragment{events: []EmitEvent{
		{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 18}}},
	}}
	if typeInstallElementIndependent(body, live, defPos, map[string]bool{}) {
		t.Fatal("an unfolded dispatch inside the expression must decline")
	}
	// An event OUTSIDE the expression's span (the body's own `drop`) is
	// none of the screen's business.
	outside := &EmitFragment{events: []EmitEvent{
		{kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 3}}},
	}}
	if !typeInstallElementIndependent(body, outside, defPos, map[string]bool{}) {
		t.Fatal("an event outside the expression's token span must not decline")
	}
	// A def site the walk cannot locate declines: no `def` at the position.
	if typeInstallElementIndependent(body, empty, core.SrcPos{Row: 9, Col: 9}, map[string]bool{}) {
		t.Fatal("an unlocatable def site must decline")
	}
}

// typeDefExprAt's own arms: it must reach a def written inside a nested
// list or a paren group, and decline every layout it cannot read as
// `def NAME <expr>` in written order.
func TestTypeDefExprAtLayouts(t *testing.T) {
	at := func(v core.Value, row, col int) core.Value {
		v.SetPos(core.SrcPos{Row: row, Col: col})
		return v
	}
	word := func(name string, row, col int) core.Value { return at(core.NewWord(name), row, col) }
	pos := core.SrcPos{Row: 1, Col: 1}

	nested := core.NewList([]core.Value{core.NewList([]core.Value{
		word("def", 1, 1), word("Big", 1, 5), at(core.NewInteger(7), 1, 9),
	})})
	got, ok := typeDefExprAt(nested, pos)
	if !ok || got.String() != "7" {
		t.Fatalf("nested list: got %v ok=%v, want the expression 7", got, ok)
	}
	inParen := core.NewList([]core.Value{core.NewParenExpr([]core.Value{
		word("def", 1, 1), word("Big", 1, 5), at(core.NewInteger(8), 1, 9),
	})})
	if got, ok := typeDefExprAt(inParen, pos); !ok || got.String() != "8" {
		t.Fatalf("inside a paren: got %v ok=%v, want the expression 8", got, ok)
	}
	// A leaf is not a container: nothing to walk.
	if _, ok := typeDefExprAt(core.NewInteger(1), pos); ok {
		t.Fatal("a leaf token located a def site")
	}
	// The token at the position is not `def`.
	other := core.NewList([]core.Value{word("undef", 1, 1), word("Big", 1, 5), core.NewInteger(1)})
	if _, ok := typeDefExprAt(other, pos); ok {
		t.Fatal("a non-def word at the position located a def site")
	}
	// The token at the position is not a word at all.
	lit := core.NewList([]core.Value{at(core.NewInteger(0), 1, 1), word("Big", 1, 5), core.NewInteger(1)})
	if _, ok := typeDefExprAt(lit, pos); ok {
		t.Fatal("a literal at the position located a def site")
	}
	// Truncated: no second operand to be the expression.
	short := core.NewList([]core.Value{word("def", 1, 1), word("Big", 1, 5)})
	if _, ok := typeDefExprAt(short, pos); ok {
		t.Fatal("a truncated def located an expression")
	}
}

// typeExprInert's whitelist is closed by construction: a container to
// descend, an inert const, or a plain unbound WORD. Anything else — a
// reach, a splice — declines rather than being enumerated as known-bad.
func TestTypeExprInertWhitelist(t *testing.T) {
	none := map[string]bool{}
	if !typeExprInert(core.NewWord("Integer"), none) {
		t.Fatal("an unbound word must be inert")
	}
	if typeExprInert(core.NewWord("e"), map[string]bool{"e": true}) {
		t.Fatal("a body-bound word must not be inert")
	}
	if !typeExprInert(core.NewInteger(5), none) {
		t.Fatal("a literal must be inert")
	}
	if !typeExprInert(core.NewList([]core.Value{core.NewWord("Integer"), core.NewInteger(5)}), none) {
		t.Fatal("a list of inert tokens must be inert")
	}
	if typeExprInert(core.NewParenExpr([]core.Value{core.NewWord("e")}), map[string]bool{"e": true}) {
		t.Fatal("a paren holding a body-bound word must not be inert")
	}
	if typeExprInert(core.NewSplice(core.NewWord("a")), none) {
		t.Fatal("a splice is neither a container, a literal, nor a word — it must decline")
	}
}

// collectExprSites descends a paren where collectTokenSites deliberately
// does not: the two walks answer different questions, and conflating them
// would either blind the screen or widen AdoptBodyTwins.
func TestCollectExprSitesDescendsParens(t *testing.T) {
	inner := core.NewWord("gt")
	inner.SetPos(core.SrcPos{Row: 1, Col: 18})
	p := core.NewParenExpr([]core.Value{inner})
	p.SetPos(core.SrcPos{Row: 1, Col: 9})

	expr := map[core.SrcPos]bool{}
	collectExprSites(p, expr)
	if !expr[core.SrcPos{Row: 1, Col: 18}] {
		t.Fatal("collectExprSites must reach a token inside a paren group")
	}
	tok := map[core.SrcPos]bool{}
	collectTokenSites(p, tok)
	if tok[core.SrcPos{Row: 1, Col: 18}] {
		t.Fatal("collectTokenSites must stay out of a paren interior (widening it changes AdoptBodyTwins)")
	}
}
