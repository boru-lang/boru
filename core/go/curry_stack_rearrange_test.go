package core

// curry_stack_rearrange_test.go pins curryOrStack's force-stack rescue for
// a pending forward that had CLAIMED stack args on a forward-capable word
// (the `fn.HasForwardSigs() && sac > 0` rearrange arm). The arm was
// previously reached only by generated fuzzer programs, so the 2026-08
// generator-stream reshuffle (genHofProg) silently de-covered it — this
// white-box drive makes the coverage deterministic, following the
// TestS7ImplicitEndForwardBeforeFunc pattern.

import "testing"

func TestCurryOrStackRearrangesClaimedStack(t *testing.T) {
	r := covRegistry(t, nil)
	// A 2-arg forward-capable word (BarrierPos -1 → all-forward eligible),
	// so Lookup succeeds and HasForwardSigs reports true.
	r.Register("czadd", Signature{
		Args: []*Type{TInteger, TInteger}, BarrierPos: -1,
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(42)}, nil
		}),
	})
	e := NewTop(r)
	// One claimed stack value below the word, no collected forwards — the
	// early-resolved pending's shape (implicitEnd / resolveOrphanedForwards
	// hand exactly (funcIdx, CollectedArgs, StackArgs) here). The 2-arg sig
	// cannot match the single resolved value, so after the rearrange the
	// force-stack match declines and the rescue falls through — the arm's
	// contract is the rearrange call itself, not a successful dispatch.
	e.Tape = NewTape([]Value{NewInteger(5), NewWord("czadd")}, StackHeadroom)
	e.Pointer = 0
	e.curryOrStack(1, 0, 1)

	// The word survives on the tape (no dispatch fired) and the pointer
	// parked at the rescue's word index.
	found := false
	for i := 0; i < e.Tape.Len(); i++ {
		if IsWord(e.Tape.Ref(i)) {
			if w, err := AsWord(e.Tape.Ref(i)); err == nil && w.Name == "czadd" {
				found = true
			}
		}
	}
	if !found {
		t.Error("the unmatched word must survive the force-stack rescue")
	}

	// And with BOTH args present the same rescue path force-stack matches
	// and dispatches: the positive twin.
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewInteger(5), NewInteger(2), NewWord("czadd")}, StackHeadroom)
	e2.Pointer = 0
	e2.curryOrStack(2, 0, 2)
	if e2.Pointer != 2 {
		t.Errorf("pointer = %d, want parked at the matched word for re-step", e2.Pointer)
	}
}
