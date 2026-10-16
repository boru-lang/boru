package core

import "testing"

// nur356_layout_test.go pins NUR356's additions to the dispatch layout: a
// written operand the pass read from a data word is published with the word
// the tape still holds (DispatchLayout.Words), and a word past every forward
// position the word's overloads reach closes the After run.

func TestIsPendingLiteral(t *testing.T) {
	lit := NewList([]Value{NewWord("x")})
	SetEval(&lit, true)
	quoted := lit
	SetQuoted(&quoted, true)
	m := NewMap(NewOrderedMap())
	SetEval(&m, true)
	if !IsPendingLiteral(lit) || !IsPendingLiteral(m) || IsPendingLiteral(quoted) || IsPendingLiteral(NewDynamicCarrier(TList)) || IsPendingLiteral(NewInteger(1)) {
		t.Error("IsPendingLiteral")
	}
}
