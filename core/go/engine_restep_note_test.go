package core

import "testing"

// TestPlainReStepToken pins the token classes the interpreter steps after a
// call's results: words, paren groups and written literals are plain; an
// unpositioned value, a carrier, a dispatch modifier and a structural marker
// are not.
func TestPlainReStepToken(t *testing.T) {
	at := SrcPos{Row: 1, Col: 4}
	pos := func(v Value) Value {
		v.SetPos(at)
		return v
	}
	mod := pos(NewDispatchMod(DispatchModInfo{Val: true}))
	dyn := pos(NewDynamicCarrier(TAny))
	yes := []Value{pos(NewWord("drop")), pos(NewParenExpr(nil)), pos(NewInteger(7))}
	no := []Value{NewInteger(7), pos(NewCarrier(TInteger)), dyn, mod, pos(NewEnd()), pos(NewCloseParen())}
	for _, v := range yes {
		if !plainReStepToken(v) {
			t.Errorf("plain: %v", v)
		}
	}
	for _, v := range no {
		if plainReStepToken(v) {
			t.Errorf("not plain: %v", v)
		}
	}
}
