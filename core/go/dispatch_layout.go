package core

// DispatchLayout is where a compiling pass's dispatch found its operands on
// the tape, published for the recorder while the dispatch is recorded
// (CheckState.CurLayout) — NUR242's `fold` over a gradual class field.
//
// A poly record re-matches at run time, and when nothing matches it has
// to raise the interpreter's signature_error, which the interpreter builds
// from its TAPE at the word: the collection plan over the stack beneath
// and the operands written after, then the attempted window over the same
// tape. The poly op holds only the operands, in signature order, so it
// cannot tell a written operand from a stack one, and for a word with
// overloads of more than one arity (fold's 2- and 3-operand forms) the
// split decides what the interpreter's plan claims. The layout says it.
//
// It is published only when it is EXACT: the operands are the tape's own
// values, contiguous on each side of the word (the written ones after it
// in signature order, the stack ones beneath it top first), and nothing
// else on the tape is reachable — a statement or group boundary, or the
// tape's end, on both sides. The interpreter's tape at a failed dispatch
// is then exactly [stack operands, word, written operands], so a rebuild
// from the operands is its plan and its report, byte for byte. Any other
// shape publishes nothing, and the record keeps the defer it had.
type DispatchLayout struct {
	// args is the operand slice the layout describes; a record reads the
	// layout only for this slice (LayoutFor).
	args []Value
	// NFwd is how many leading signature positions were written after the
	// word; the rest came off the stack beneath it, top first.
	NFwd int
}

// LayoutFor returns the published layout when it describes args — the
// very slice the dispatch published it for, which the record path hands
// down unchanged — and nil for any other record, which a nested analysis
// may make while the layout is published.
func (c *CheckState) LayoutFor(args []Value) *DispatchLayout {
	l := c.CurLayout
	if l == nil || len(args) == 0 || len(l.args) != len(args) || &l.args[0] != &args[0] {
		return nil
	}
	return l
}

// PublishLayout publishes the exact layout of the dispatch at the pointer
// whose operands args (signature order) sit at the tape indices at, and
// returns the restore the caller runs once the dispatch is recorded. A
// layout that is not exact publishes nil, so no record reads a stale one.
// Only a recording pass publishes.
func (e *Engine) PublishLayout(args []Value, at []int, pos SrcPos) func() {
	prev := e.Registry.Check.CurLayout
	e.Registry.Check.CurLayout = e.exactLayout(args, at, pos)
	return func() { e.Registry.Check.CurLayout = prev }
}

// exactLayout is the layout when it is exact (see DispatchLayout), else nil.
func (e *Engine) exactLayout(args []Value, at []int, pos SrcPos) *DispatchLayout {
	n := len(args)
	if n == 0 || len(at) != n || !e.Registry.analysisRecorder().Active() {
		return nil
	}
	w, err := AsWord(e.Tape.At(e.Pointer))
	if err != nil || w.ArgCount != -1 || w.ForceStack || w.ForceForward || w.ForceVal || w.ForceUsurp {
		return nil
	}
	// The report's two tape-only layers must not apply (the probe's rule,
	// PolyNoMatchProbe): a void argument group, the fn-shape binding hint.
	if e.voidArgErrorFor(w.Name, pos) != nil || e.IsFnShapeTypedBindingContext() {
		return nil
	}
	k := 0
	for k < n && at[k] > e.Pointer {
		k++
	}
	for i := 0; i < n; i++ {
		want := e.Pointer + 1 + i
		if i >= k {
			want = e.Pointer - (i - k) - 1
		}
		if at[i] != want || want < 0 || want >= e.Tape.Len() || args[i].ID == "" || e.Tape.At(want).ID != args[i].ID {
			return nil
		}
	}
	if below := e.Pointer - (n - k) - 1; below >= 0 && !IsOpenParen(e.Tape.At(below)) && !IsEnd(e.Tape.At(below)) {
		return nil
	}
	if after := e.Pointer + k + 1; after < e.Tape.Len() && !IsCloseParen(e.Tape.At(after)) && !IsEnd(e.Tape.At(after)) {
		return nil
	}
	return &DispatchLayout{args: args, NFwd: k}
}

// SigOrderPositions is SigOrderArgs over tape indices: positions lists the
// stack run (nStack of them, ascending toward the pointer) and then the
// forward run in source order; the result lists them in signature order.
func SigOrderPositions(positions []int, nStack int) []int {
	if nStack < 0 || nStack > len(positions) {
		nStack = len(positions)
	}
	out := make([]int, 0, len(positions))
	out = append(out, positions[nStack:]...)
	for i := nStack - 1; i >= 0; i-- {
		out = append(out, positions[i])
	}
	return out
}
