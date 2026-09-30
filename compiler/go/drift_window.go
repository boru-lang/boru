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
	// A list or map literal's element run (an inline context region of this
	// unit) ends its own tape at the literal's last element, so the TERMINAL
	// gate below would pass there — but the literal assembles a FIXED count
	// of its elements, and the window's variadic result is not the program
	// residual: `[7 mk add 1]` answered `[7 [43]]` for `[[7 43]]` (NUR287).
	// The shape keeps the compile failure.
	if es.InInlineCtxBoundary() {
		return false
	}
	// The compile failure-site preconditions (mirrors declineForwardStackDrift): a
	// forward-eligible non-full-stack sig, no code-body positions, at least
	// a dynamic top + one deeper operand.
	if sig == nil || sig.BarrierPos == 0 || sig.FullStack() || len(sig.NoEvalArgs) > 0 || len(positions) < 2 {
		return false
	}
	// The completion of the word's own forward collection: its top operand
	// was WRITTEN after the word, and the island's source-order window would
	// lay it beneath the word instead — `1 g (h) 7` over `g [a:Integer |
	// b:Integer]` islanded as `1 (h) g 7`, which collects the 7 (NUR362).
	// The interpreter re-collects nothing there; the dispatch records as the
	// collection laid it out.
	if e.ForwardSplit() > 0 {
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
	// A DYNAMIC top is the whole precondition: the operands beneath it may
	// be dynamic too. The check-mode match over carriers reached past the
	// top to its deeper operands either way, and the interpreter, seeing
	// concrete runtime values, may forward-collect the literal instead —
	// `def mk fn [[][Any][42]] end mk mk add 1` is `[42 43]`, where the
	// match over two carriers took `add (Bytes, Bytes)` all-stack and the
	// poly re-match answered `[84 1]` (NUR287).
	if !e.Tape.At(topPos).Dynamic {
		return false
	}
	fwdIdx := e.Pointer + 1
	if fwdIdx >= e.Tape.Len() {
		return false
	}
	// The forward operand is a literal, or a word bound to one, which the
	// interpreter's forward phase collects as its value: the window carries
	// that value, the island's own token for it (NUR287, `mk mk add k`).
	fwdVal, fwdOK := e.ForwardOperandValue(e.Tape.At(fwdIdx))
	if !fwdOK {
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
	fwdOp, ok := es.resolveOperand(fwdVal)
	if !ok { //covergate:allow ForwardOperandValue yields only concrete scalars/atoms and bare type nodes, all of which resolveOperand materialises as const/type operands (§compiler)
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
		// A PLACED value — a user call's parked result, a paren's placed
		// survivor — is data the interpreter never re-steps, and the island
		// stepped it live and applied it: `5 mk add 1` over mk's lambda
		// answered 7 where the interpreter's add meets the parked fn and
		// raises (NUR287). It rides into the island inside its own paren,
		// the interpreter's placement: a one-survivor paren parks a fn as
		// data (fnReturnPark) and leaves any other value as it is. Top-first,
		// so the close marker goes first. A bare read of a def bound to such
		// a value undoes the placement, as callResultPlaced's own rule has it
		// (ADR-011): the interpreter's read dispatches the fn, and so does the
		// island's step of it — `each [def v (mk) v add 1] [1]` over a paren-
		// placed fn islanded `( v )` and met the open paren (NUR363).
		readDispatches := es.isDefRead(v) && !es.placedValRead(v.ID)
		if es.callResultPlaced(v) || (es.placedNotReStepped(v) && !readDispatches) {
			ops = append(ops, ConstOperand(es.intern(core.NewCloseParen())), op, ConstOperand(es.intern(core.NewOpenParen())))
			continue
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
