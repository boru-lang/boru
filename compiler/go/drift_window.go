package compiler

import core "github.com/boru-lang/boru/core/go"

// Forward-drift window (COMPILE FAILURE-CLOSURE.0 §1) — the COMPILING model for the
// dispatch declineForwardStackDrift otherwise declines.
//
// The shape: a forward-eligible word matched ALL-STACK under a DYNAMIC
// top-of-stack operand with a concrete leading residual beneath it and a
// concrete forward literal right after it (`5 do [7] error ["x"] add 1`).
// The check-mode match (the dynamic carrier blocks the narrower forward
// overload's slot) reaches PAST the dynamic value to the deeper residual,
// while the interpreter — seeing the operand's CONCRETE runtime value —
// forward-collects the trailing literal instead. No faithful STATIC operand
// record exists: the binding is value-dependent.
//
// The model: record the whole window — [leading residual(s), dynamic value,
// the WORD itself as an inert const, the forward literal] — as ONE
// OpCallDynamicMixed event. The VM islands the window verbatim through an
// interpreter sub-run (islandRun), where the word token DISPATCHES over the
// live values with the interpreter's own forward collection: the island IS
// the token sequence the interpreter ran, so the binding (and the residual
// count it nets) is byte-identical on every path. The event's result is
// VARIADIC (forward-collect nets [5 8]; a genuinely different runtime
// binding nets other counts), so only the variadic-absorbing program
// residual may consume it — the TERMINAL gate below enforces exactly that.
func init() {
	core.DriftWindowRecorder = tryRecordDriftWindow
}

func tryRecordDriftWindow(e *core.Engine, w core.WordInfo, sig *core.Signature, positions []int) bool {
	es, _ := e.Registry.Check.Recorder().(*EmitState)
	if es == nil || !es.Active() || es.SuspendedNow() {
		return false
	}
	// The compile failure-site preconditions (mirrors declineForwardStackDrift): a
	// forward-eligible non-full-stack sig, no code-body positions, at least
	// a dynamic top + one deeper operand.
	if sig == nil || sig.BarrierPos == 0 || sig.FullStack() || len(sig.NoEvalArgs) > 0 || len(positions) < 2 {
		return false
	}
	topPos, minPos := -1, e.Tape.Len()
	for _, p := range positions {
		if p < 0 || p >= e.Tape.Len() {
			return false
		}
		if p > topPos {
			topPos = p
		}
		if p < minPos {
			minPos = p
		}
	}
	deeperConcrete := false
	for _, p := range positions {
		if p != topPos && !e.Tape.At(p).Dynamic {
			deeperConcrete = true
		}
	}
	if !e.Tape.At(topPos).Dynamic || !deeperConcrete {
		return false
	}
	fwdIdx := e.Pointer + 1
	if fwdIdx >= e.Tape.Len() || !core.ForwardLiteralOperand(e.Tape.At(fwdIdx)) {
		return false
	}
	// CONTIGUITY: the matched operands must be exactly the tape span directly
	// under the word (no interleaved bystanders) so the window splice is the
	// same region the island re-steps.
	if topPos != e.Pointer-1 || topPos-minPos+1 != len(positions) {
		return false
	}
	// TERMINAL: the window's variadic result must land in the program
	// residual — nothing may follow the forward literal except statement
	// furniture. A downstream consumer would need a static count the island
	// cannot promise; those shapes keep the compile failure — an open defect, not a
	// settled boundary.
	for i := fwdIdx + 1; i < e.Tape.Len(); i++ {
		t := e.Tape.At(i)
		if !core.IsEnd(t) && !core.IsDefCleanup(t) {
			return false
		}
	}
	// BYSTANDER-FREE: a data value below the window (`1 2 3 do … add 1` —
	// the 1 and 2 the dispatch never touched) breaks the in-order
	// reconciliation once the window re-pushes its const operands above
	// where the bystanders land; those shapes keep the compile failure — an open
	// defect, not a settled boundary.
	for i := 0; i < minPos; i++ {
		t := e.Tape.At(i)
		if !core.IsEnd(t) && !core.IsDefCleanup(t) {
			return false
		}
	}
	// Every window value needs a compiled home. The layout convention is
	// TOP-FIRST (ops[0] lands on the stack top), so the window builds from
	// the forward literal down to the deepest residual: the stack then reads
	// [deepest … dynamic-value word-const forward-literal] bottom-up — the
	// island's tape in source order. A value produced by a VARIADIC event
	// cannot ride a fixed-width window.
	ops := make([]EmitOperand, 0, len(positions)+2)
	fwdOp, ok := es.resolveOperand(e.Tape.At(fwdIdx))
	if !ok { //covergate:allow forwardLiteralOperand admits only concrete scalars/atoms and bare type nodes, all of which resolveOperand materialises as const/type operands (§compiler)
		return false
	}
	ops = append(ops, fwdOp)
	// The word itself rides as an inert const: the island steps it as a live
	// token, so the RUNTIME dispatch (registry-resolved, forward-collecting)
	// is the interpreter's own.
	wordTok := core.WithPos(core.NewWord(w.Name), e.Tape.At(e.Pointer))
	ops = append(ops, ConstOperand(es.intern(wordTok)))
	for p := topPos; p >= minPos; p-- {
		v := e.Tape.At(p)
		if pr, ok := es.producedBy[v.ID]; ok && es.eventInfo[pr.seq].variadicResult {
			return false
		}
		op, ok := es.resolveOperand(v)
		if !ok {
			return false
		}
		ops = append(ops, op)
	}

	out := core.NewDynamicCarrier(core.TAny)
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: w.Name, ops: ops, nout: 1, pos: wordTok.Pos(), dynMixed: true,
	}})
	es.setProducedAt(out, seq, 0)
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f

	// Consume the window from the tape — operands, word, forward literal —
	// and continue the check pass from the modeled result (the execMatch
	// splice-and-repoint discipline).
	e.Tape.Splice(minPos, fwdIdx-minPos+1, out)
	e.Pointer = minPos
	return true
}
