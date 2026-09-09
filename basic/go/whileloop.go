package basic

// RunWhileLoop builds the mark + condition + move tokens for a while
// loop — the condition region runs first, and the move's while-mode
// ForCont (core engine.go stepMoveWhile) alternates condition and body
// regions from there. Both operands are quoted code lists (NoEvalArgs).
// Every region is engine-stepped, so the Run loop's step budget meters
// the loop: `while [true] []` is bounded by evaluation_limit, never a
// silent hang. Body values accumulate across iterations and splice onto
// the stack when the condition goes falsy, exactly as `for` leaves its
// per-iteration values; `break` ends the loop with the values collected
// so far, `continue` abandons the current body round.
func RunWhileLoop(r *Registry, cond, body Value) ([]Value, error) {
	if !IsConcrete(cond) {
		return nil, r.BoruError("while_error",
			"while: condition must be a concrete list, got type literal", "while")
	}
	if !IsConcrete(body) {
		return nil, r.BoruError("while_error",
			"while: body must be a concrete list, got type literal", "while")
	}
	condLst, _ := AsList(cond)
	bodyLst, _ := AsList(body)
	condSlice := condLst.Slice()
	bodySlice := bodyLst.Slice()

	condCopy := make([]Value, len(condSlice))
	copy(condCopy, condSlice)
	bodyCopy := make([]Value, len(bodySlice))
	copy(bodyCopy, bodySlice)

	cont := &ForCont{
		Registry:  r,
		Body:      bodyCopy,
		WhileCond: condCopy,
	}

	// First region: the condition. Its move fires stepMoveWhile with
	// WhileInBody false, which reads the region's last value and either
	// splices the body or finishes the loop.
	id := NextMarkID()
	tokens := make([]Value, 0, len(condCopy)+2)
	tokens = append(tokens, NewMark(id, condCopy...))
	condTokens := make([]Value, len(condCopy))
	copy(condTokens, condCopy)
	tokens = append(tokens, condTokens...)
	tokens = append(tokens, NewMoveCont(id, "while loop", cont))
	return tokens, nil
}

// WhileHandler is the run-mode implementation of the `while` word.
func WhileHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return RunWhileLoop(r, args[0], args[1])
}

// whileReturnsFn is the check-mode model of a while loop: analyse the
// CONDITION and the BODY each through the loop-body fixed point
// (AnalyseLoopBody with no iterator binding — reads join the enclosing
// bindings exactly as a for body's do), then approximate the residual
// the way for's non-static arm does: a typed-List carrier of the body's
// residual top, or nothing for a zero-net body. A while loop's trip
// count is NEVER static, so there is no zero-prune and no exact-count
// spread; the bytecode lowering (RecordWhile, the thirty-seventh
// increment) runs the loop on the counted loop's frame over an
// unbounded count, testing the condition fragment's one value at the
// head of every iteration. Modelling here — instead of letting the
// analysis pass step the spliced regions with carrier conditions — is
// what keeps a carrier-conditioned `while` from looping the checker.
func whileReturnsFn(args []Value, r *Registry) []Value {
	// RECORDING (the thirty-seventh increment): the condition and the body
	// are captured as two fragments — each analysis armed like `for`'s — and
	// recorded through RecordWhile with a scratch iterator slot, since the
	// lowering runs the loop on the counted loop's frame. The loop EVENT
	// must then stay linked to the residual (`out` returned even for a
	// zero-net body, which RecordWhile marks zeroOut), exactly as `for`'s
	// model keeps its own.
	// The BODY is analysed first, the condition second: a def the body
	// REBINDS (`while [n lt 3] [def n (n add 1)]`) is registered
	// loop-carried by the body's analysis (NoteLoopCarried), and only a
	// condition analysed after it reads the carried slot — analysed first,
	// its read resolved to the pre-loop value and the compiled loop never
	// terminated. Both analyses run to their fixed points over the joined
	// bindings, so the order changes no verdict.
	es := r.Check.Recorder()
	recording := es.Active()
	if recording {
		es.ArmLoopCapture()
	}
	stk := AnalyseLoopBody(r, args[1], nil, nil, false)
	var bodyFrag EmitFragmentRef
	if recording {
		bodyFrag = es.TakeFragment()
		es.ArmLoopCapture()
	}
	condStk := AnalyseLoopBody(r, args[0], nil, nil, false)
	out := NewCarrier(TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if IsDisjunct(top) {
			out = NewCarrierTypedListValue(top)
		} else {
			out = NewCarrierTypedList(top.Parent)
		}
	}
	if recording {
		condFrag := es.TakeFragment()
		iter := NewCarrier(TInteger)
		es.RegisterLocal(iter.ID)
		es.RecordWhile(condFrag, bodyFrag, condStk, stk, iter.ID, out, args[0].Pos())
		return []Value{out}
	}
	if len(stk) == 0 {
		return []Value{}
	}
	return []Value{out}
}
