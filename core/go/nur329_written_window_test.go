package core

import (
	"slices"
	"testing"
)

// TestNUR329GradualWrittenOperands pins the widening of a failed dispatch's
// window by its written run (withGradualWrittenOperands): a carrier or a
// dynamic written after the word joins the window, with every other written
// operand, so the failure goes to the runtime rematch that renders the run's
// value — an unexpanded reach or template after it included, as its own
// token (`7 f m.a m.b`); a written run of concrete values and no written run
// at all leave the window as it was.
func TestNUR329GradualWrittenOperands(t *testing.T) {
	w := NewWord("f")
	reach := NewReachFromKeys(NewWord("m"), []Value{NewAtom("b")})
	for _, c := range []struct {
		what   string
		tape   []Value
		window []int
		want   []int
	}{
		{"a carrier written after the word", []Value{NewInteger(7), w, NewCarrier(TInteger)}, []int{0}, []int{0, 2}},
		{"a dynamic, after a concrete one", []Value{NewInteger(7), w, NewInteger(1), NewDynamicCarrier(TAny)}, []int{0}, []int{0, 2, 3}},
		{"a window already past the word", []Value{w, NewCarrier(TInteger)}, []int{1}, []int{1}},
		{"concrete written values", []Value{NewInteger(7), w, NewInteger(1)}, []int{0}, []int{0}},
		{"a deferred reach in the run", []Value{NewInteger(7), w, NewCarrier(TInteger), reach}, []int{0}, []int{0, 2, 3}},
		{"a deferred reach alone", []Value{NewInteger(7), w, NewInteger(1), reach}, []int{0}, []int{0}},
		{"nothing written", []Value{NewInteger(7), w}, []int{0}, []int{0}},
	} {
		ptr := slices.IndexFunc(c.tape, func(v Value) bool { return IsWord(v) })
		e := engWithTape(t, c.tape, ptr)
		if got := e.withGradualWrittenOperands(c.window, nil); !slices.Equal(got, c.want) {
			t.Errorf("%s: window %v, want %v", c.what, got, c.want)
		}
	}
}

// TestNUR329ForwardReach pins how many written operands a word's forward
// phase may take: its signatures' widest barrier, every position of an
// all-forward one or under `/f`, none under `/s`, fallbacks aside.
func TestNUR329ForwardReach(t *testing.T) {
	sig := func(n, barrier int, fallback bool) Signature {
		s := Signature{Args: make([]*Type, n), BarrierPos: barrier, Fallback: fallback}
		for i := range s.Args {
			s.Args[i] = TAny
		}
		return s
	}
	fn := &FnDefInfo{Signatures: []Signature{sig(2, 0, false), sig(3, 1, false), sig(5, 5, true)}}
	for _, c := range []struct {
		what string
		fn   *FnDefInfo
		w    WordInfo
		want int
	}{
		{"the widest barrier", fn, WordInfo{}, 1},
		{"under /f", fn, WordInfo{ForceForward: true}, 3},
		{"under /s", fn, WordInfo{ForceStack: true}, 0},
		{"an all-forward signature", &FnDefInfo{Signatures: []Signature{sig(2, -1, false)}}, WordInfo{}, 2},
		{"a barrier past the arity", &FnDefInfo{Signatures: []Signature{sig(1, 4, false)}}, WordInfo{}, 1},
	} {
		if got := forwardReach(c.fn, c.w); got != c.want {
			t.Errorf("%s: %d, want %d", c.what, got, c.want)
		}
	}
}

// TestNUR336StatementStackPos pins the key a statement-stack note takes: the
// token's own position, or — for a paren the engine expanded to its markers
// before stepping it (`def k (3 dup) (m.f 7)`) — its first inner token's;
// an open paren with nothing after it keeps its own.
func TestNUR336StatementStackPos(t *testing.T) {
	inner := NewWord("m")
	inner.SetPos(SrcPos{Row: 1, Col: 65})
	own := NewWord("k")
	own.SetPos(SrcPos{Row: 1, Col: 3})
	e := engWithTape(t, []Value{own, NewOpenParen(), inner, NewCloseParen(), NewOpenParen()}, 0)
	if got := e.statementStackPos(0); got.Col != 3 {
		t.Errorf("a token's own position: %v", got)
	}
	if got := e.statementStackPos(1); got.Col != 65 {
		t.Errorf("an expanded paren's first inner token: %v", got)
	}
	if got := e.statementStackPos(4); got.Row != 0 {
		t.Errorf("an open paren with nothing after it: %v", got)
	}
}
