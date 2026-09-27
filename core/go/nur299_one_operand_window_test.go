package core

import "testing"

// nur299Recorder is an ACTIVE recorder, suspended as configured.
type nur299Recorder struct {
	inactiveEmit
	suspended bool
}

func (n *nur299Recorder) Active() bool       { return true }
func (n *nur299Recorder) SuspendedNow() bool { return n.suspended }

// nur299Match runs MatchSignature for `dyn w [5]` — a dynamic Any carrier on
// the stack, the word, then a List forward — over sigs on a compiling check
// pass, then the forward-drift guard over its all-stack window, and reports
// the selected signature's forward count and whether the split was flagged.
func nur299Match(t *testing.T, sigs []Signature) (int, bool) {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Mode, r.Check.Compiling, r.Check.Emit = true, true, &nur299Recorder{}
	t.Cleanup(func() { r.Check.Mode, r.Check.Compiling, r.Check.AmbiguousGradualSplit = false, false, false })
	dyn := NewDynamicCarrier(TAny)
	e := NewTop(r)
	e.Tape = NewTape([]Value{dyn, NewWord("w"), NewList([]Value{NewInteger(5)})}, StackHeadroom)
	e.Pointer = 1
	fn := &FnDefInfo{Name: "w", Signatures: sigs}
	sig, positions, _ := e.MatchSignature(fn, WordInfo{Name: "w", ArgCount: -1}, []Value{dyn})
	if sig == nil {
		t.Fatalf("a signature must be selected")
	}
	fwd := 0
	for _, p := range positions {
		if p > 1 {
			fwd++
		}
	}
	if r.Check.AmbiguousGradualSplit {
		t.Fatal("the matcher leaves an all-stack window to the forward-drift guard")
	}
	e.declineForwardStackDrift(fn, WordInfo{Name: "w", ArgCount: -1}, sig, positions)
	return fwd, r.Check.AmbiguousGradualSplit
}

// TestNUR299OneOperandAllStackWindow pins NUR299: a word whose first
// candidate fills its slot from the stack with a dynamic carrier — an
// unproven match, `(mk) do [5]` over do's Map overload — while a later
// candidate forward-collects the token after the word (the List overload
// over `[5]`) is a split the runtime value decides. No drift model answers
// it (the check side's decline wants a match that reached past its top), so
// the forward-drift guard flags it on a compiling pass; no later forward
// claim is no ambiguity, and a pass that is not compiling asks nothing.
func TestNUR299OneOperandAllStackWindow(t *testing.T) {
	norm := func(sigs []Signature) []Signature {
		for i := range sigs {
			NormalizeSig(&sigs[i])
		}
		return sigs
	}
	fwd, flagged := nur299Match(t, norm([]Signature{
		{Args: []*Type{TMap}, BarrierPos: 1},
		{Args: []*Type{TList}, BarrierPos: 1},
	}))
	if fwd != 0 || !flagged {
		t.Errorf("the Map overload takes the carrier from the stack (fwd %d), and the List one's forward claim flags it: %v", fwd, flagged)
	}
	if _, flagged := nur299Match(t, norm([]Signature{
		{Args: []*Type{TMap}, BarrierPos: 1},
		{Args: []*Type{TMap, TAny}, BarrierPos: 2},
	})); flagged {
		t.Error("no later one-operand claim over the token: no ambiguity")
	}
	if _, flagged := nur299Match(t, norm([]Signature{
		{Args: []*Type{TMap}, BarrierPos: 1},
	})); flagged {
		t.Error("no later candidate at all: no ambiguity")
	}
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewDynamicCarrier(TAny), NewWord("w"), NewList(nil)}, StackHeadroom)
	e.Pointer = 1
	sigs := norm([]Signature{{Args: []*Type{TMap}, BarrierPos: 1}, {Args: []*Type{TList}, BarrierPos: 1}})
	fn := &FnDefInfo{Name: "w", Signatures: sigs}
	e.declineForwardStackDrift(fn, WordInfo{Name: "w", ArgCount: -1}, &sigs[0], []int{0})
	if r.Check.AmbiguousGradualSplit {
		t.Error("a pass that is not compiling latches nothing")
	}
	r.Check.Mode, r.Check.Compiling, r.Check.Emit = true, true, &nur299Recorder{suspended: true}
	t.Cleanup(func() { r.Check.Mode, r.Check.Compiling, r.Check.AmbiguousGradualSplit = false, false, false })
	e.declineForwardStackDrift(fn, WordInfo{Name: "w", ArgCount: -1}, &sigs[0], []int{0})
	if r.Check.AmbiguousGradualSplit {
		t.Error("a suspended region records nothing, so it latches nothing: its body runs as an island")
	}
}
