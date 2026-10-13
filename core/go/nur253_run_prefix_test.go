package core

import "testing"

// phantomRecorder marks one recorder key as a 0-output statement guard's
// phantom None — the result a both-arms-void `if` registers (the recorder's
// zeroOut).
type phantomRecorder struct {
	inactiveEmit
	phantom RecKey
}

func (p *phantomRecorder) ZeroOutProduced(k RecKey) bool {
	return k != (RecKey{}) && k == p.phantom
}

// TestNUR253RunPrefixSkipsPhantoms pins NUR253's report half: the stack
// prefix a no-match report renders is the RUN's — a phantom the recorder
// marks is dropped wherever it sits, and every other entry kept in order —
// while a pass whose recorder marks nothing reports the tape's prefix.
func TestNUR253RunPrefixSkipsPhantoms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rec := &phantomRecorder{}
	r.Check.Emit = rec
	t.Cleanup(func() { r.Check.Emit = nil })
	ph := NewCarrier(TNone)
	SetID(&ph, "nur253-phantom")
	// The `if`'s event stamped its phantom — after the identity, which
	// would clear the key.
	SetRecKey(&ph, RecKey{Pass: 1, Seq: 1})
	five, x := NewInteger(5), NewString("x")
	e := NewTop(r)
	e.Tape = NewTape([]Value{five, ph, x, NewWord("f")}, StackHeadroom)
	e.Pointer = 3
	if got := e.runPrefix(); len(got) != 3 {
		t.Fatalf("an unmarked tape reports its prefix whole: got %v", got)
	}
	rec.phantom = RecKeyOf(&ph)
	got := e.runPrefix()
	if len(got) != 2 || !ExactEqual(got[0], x) || !ExactEqual(got[1], five) {
		t.Fatalf("the phantom is dropped and the run's entries kept top-first: got %v", got)
	}
	// A phantom on top, and one alone.
	e.Tape = NewTape([]Value{five, ph, NewWord("f")}, StackHeadroom)
	e.Pointer = 2
	if got := e.runPrefix(); len(got) != 1 || !ExactEqual(got[0], five) {
		t.Fatalf("a phantom on top: got %v", got)
	}
	e.Tape = NewTape([]Value{ph, NewWord("f")}, StackHeadroom)
	e.Pointer = 1
	if got := e.runPrefix(); len(got) != 0 {
		t.Fatalf("a phantom alone leaves nothing: got %v", got)
	}
}
