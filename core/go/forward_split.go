package core

// The forward split: how many operands of the dispatch at the pointer were
// written after its word, as rearrangeForForward recorded them.
// forwardSplit is how many operands of the dispatch at the pointer were
// written after its word — the leading signature positions — as the last
// rearrangeForForward recorded them, when that record is this word's (its
// pointer and position); none otherwise, which is also what a dispatch that
// collected nothing forward has.
func (e *Engine) forwardSplit() int {
	if e.fwdSplitAt == e.Pointer && e.Pointer < e.Tape.Len() && e.Tape.Ref(e.Pointer).Pos() == e.fwdSplitPos {
		return e.fwdSplitN
	}
	return 0
}

// ForwardSplit is forwardSplit for the compile seams outside core: how many
// operands of the dispatch at the pointer its own forward collection took
// from after the word. A word re-stepped by that collection's completion
// reads every operand beneath it, but the leading ones were written after
// it, so a stack-only model of the dispatch is not the interpreter's
// (NUR362).
func (e *Engine) ForwardSplit() int { return e.forwardSplit() }
