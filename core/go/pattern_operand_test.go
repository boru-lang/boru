package core

import "testing"

// stubPatternHost records whether the matcher asked it to evaluate.
type stubPatternHost struct{ asked int }

func (h *stubPatternHost) evalPatternOperand(_ int, v Value) (Value, bool) {
	h.asked++
	return v, true
}

// TestPatternOperandTakesRawWhereTheSlotDoes pins patternOperand's routing
// (NUR235): a pending literal is evaluated by the dispatching host unless
// the slot takes it raw (NoEvalArgs for a list, NoEvalMapArgs for a map), a
// literal with no active token and a host that does not evaluate leave it
// as written.
func TestPatternOperandTakesRawWhereTheSlotDoes(t *testing.T) {
	m := NewOrderedMap()
	m.Set("f", NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(1)}))
	pendingMap := NewEvalMap(m)
	pendingList := NewEvalList([]Value{NewParenExpr([]Value{NewInteger(2)})})
	plain := NewEvalList([]Value{NewInteger(2)})

	h := &stubPatternHost{}
	raw := &Signature{NoEvalArgs: map[int]bool{0: true}, NoEvalMapArgs: map[int]bool{0: true}}
	for _, v := range []Value{pendingMap, pendingList} {
		if _, ok := patternOperand(h, raw, 0, 0, v); !ok || h.asked != 0 {
			t.Fatalf("a raw slot must take %v as written (asked %d)", v, h.asked)
		}
	}
	eval := &Signature{}
	for _, v := range []Value{pendingMap, pendingList} {
		if _, ok := patternOperand(h, eval, 0, 0, v); !ok {
			t.Fatalf("an evaluating slot must accept %v", v)
		}
	}
	if h.asked != 2 {
		t.Fatalf("the host evaluates each pending literal at an evaluating slot: asked %d, want 2", h.asked)
	}
	if _, ok := patternOperand(h, eval, 0, 0, plain); !ok || h.asked != 2 {
		t.Fatal("a literal with no active token is its own value")
	}
	if _, ok := patternOperand(nil, eval, 0, 0, pendingMap); !ok || h.asked != 2 {
		t.Fatal("a host that does not evaluate judges the raw token")
	}
}
