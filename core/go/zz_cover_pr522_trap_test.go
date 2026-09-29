package core

import "testing"

// TryRecordUnmatchedDispatchTrap walks the tokens after the word only while
// they lie inside the examined window: an open paren there is an operand the
// interpreter pre-evaluates, so the trap declines; a token past the window's
// last position ends the walk, so an open paren beyond it is not the trap's
// business.
func TestTrapWindowWalkStopsPastTheWindow(t *testing.T) {
	five := NewInteger(5)
	e, _ := trapEngine(t, []Value{NewWord("hd"), five, NewOpenParen()}, 0, []int{1, 2})
	if e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("an open paren inside the window must decline")
	}
	e, _ = trapEngine(t, []Value{NewWord("hd"), five, NewOpenParen()}, 0, []int{1})
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, trapFn(), SrcPos{}) {
		t.Error("an open paren past the window must not stop the trap")
	}
}
