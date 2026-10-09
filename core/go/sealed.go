package core

// A SEALED REGION evaluates a container literal's elements — a pending
// list's contents, a map member or computed key, an interpolation hole —
// on the running tape, where a pooled container sub-engine (RunContainerSub)
// ran them until 2026-10-09. The region is a driven loop of ONE iteration
// (loop.go), spliced right after the pointer and stepped by evalSealed until
// its move collapses it:
//
//	mark (ₗ elements… ) move
//
// Each piece does what the sub-engine's run did. The paren seals the stack
// (LoopOpenInfo: a word inside the literal sees nothing beneath it, exactly
// as it saw nothing beneath the sub-engine's tape); the mark pushes the
// iteration's context layer and opens a step budget of the region's own
// (beginLoopIteration — Engine.Run's Contexts push and its own step
// counter); the close paren resolves the pending forwards of the region's
// scope (the run's implicit end) and fires the move, whose collection
// evaluates a nested pending literal the elements left as the end-of-run
// sweep evaluated it (collectLoopRegion); the driver keeps the residual as
// the literal's result instead of splicing it in the region's place, and
// the tokens leave the tape with the pointer back where it stood. Nothing
// beyond the region moves, so a caller holding tape indices — a dispatch
// mid-collection, the end-of-run sweep's index — finds them as it left them.
//
// What changes is what a sub-engine cost: no engine taken and returned, no
// tape reloaded, no per-run scratch reset, no interpreter entry (the census
// no longer counts a literal), one trace and one step budget — and a
// literal's elements are visible to the trace and the debugger as the
// program's own tokens, inside a region the debugger reads as a body.
//
// The analysis pass keeps the sub-engine: its recorder brackets, carrier
// stripping and top-frame rules read the sub-run's flags (ElemEvalRecordable,
// IsTop), and that model is not this change's to move. So does an engine
// with no tape to run on (AutoEvalConsumedList's NewTop).

// sealedDriver is the driver of a sealed region: a loop of one iteration
// whose residual IS the result — kept for evalSealed rather than spliced in
// the region's place and re-stepped, which a driven loop's results are and
// a literal's elements never were.
type sealedDriver struct {
	kind string  // what the region evaluates, for the trace and the debugger
	out  []Value // the residual, caller-owned (the sub-engine protocol's copy)
	done bool
	// resolved is the region's own resolved-stack scratch (Engine.
	// resolvedScratch), swapped in for its duration (sealedHold).
	resolved []Value
}

// Next is asked for iteration 1 only — the region itself is spliced by
// evalSealed — and there is none.
func (d *sealedDriver) Next(int) ([]Value, Value, bool, error) { return nil, Value{}, false, nil }

// Collect keeps the residual: the slice is the loop's, valid for the call,
// and the result is the caller's — the copy runPooledSub made of a
// sub-engine's result stack.
func (d *sealedDriver) Collect(_ int, residual []Value) error {
	d.out = make([]Value, len(residual))
	copy(d.out, residual)
	return nil
}

// Finish ends the region with nothing in its place.
func (d *sealedDriver) Finish() ([]Value, error) {
	d.done = true
	return nil, nil
}

func (d *sealedDriver) Describe() string { return d.kind }

// WrapError leaves a fault as it is: a sub-engine's error was the literal's
// own, with no construct to attribute it to.
func (d *sealedDriver) WrapError(_ int, err error) error { return err }

// sealsLiterals reports whether this engine evaluates a literal's elements
// as a sealed region of its tape: at run time, on an engine that has one.
func (e *Engine) sealsLiterals() bool {
	return e.Tape != nil && !e.Registry.analysisActive()
}

// runSealed evaluates toks — a literal's elements, a map member or computed
// key, an interpolation hole — as a sealed region of this tape, or on a
// pooled container sub-engine where the region is not available
// (sealsLiterals). kind names the region for the trace and the debugger;
// elemEvalRecordable configures the sub-engine (Engine.ElemEvalRecordable)
// and means nothing to the region, whose evaluation is never recorded.
func (e *Engine) runSealed(toks []Value, kind string, elemEvalRecordable bool) ([]Value, error) {
	if !e.sealsLiterals() {
		return RunContainerSub(e.Registry, toks, elemEvalRecordable)
	}
	return e.evalSealed(toks, kind)
}

// takeSealed hands out the sealed-region Loop for the current nesting
// depth — one per depth, minted once and reused. A literal inside a literal
// needs a region of its own while the outer one is live, and two regions at
// one depth are never live together (a region is stepped to its end, or
// abandoned, before its evaluator returns), so each keeps its mark id — on
// the tape at most once — and its minted tokens: an evaluation allocates
// nothing but its result.
func (e *Engine) takeSealed(kind string) (*Loop, *sealedDriver) {
	if e.sealDepth == len(e.sealed) {
		e.sealed = append(e.sealed, &Loop{Registry: e.Registry, Word: "literal", Count: 1, Driver: &sealedDriver{}})
	}
	lp := e.sealed[e.sealDepth]
	e.sealDepth++
	drv := lp.Driver.(*sealedDriver)
	drv.kind, drv.out, drv.done = kind, nil, false
	lp.Iter = 1
	return lp, drv
}

// sealedRegionTokens builds a sealed region's token run into buf: the
// loop's mark, its sealing paren (no inert inputs — the elements are
// stepped from the first), the elements, the close paren and the move.
func sealedRegionTokens(buf []Value, lp *Loop, toks []Value) []Value {
	lp.mint(0)
	out := append(buf[:0], lp.mark, lp.open)
	out = append(out, toks...)
	return append(out, lp.close, lp.move)
}

// evalSealed evaluates toks as a sealed region of this tape (see the file
// comment) and returns the residual the elements left, caller-owned.
//
// The region is spliced right after the pointer and stepped here, token by
// token as the Run loop steps them (stepToken), until its move collapses it;
// the region's extent is read from the tape's length (the tokens beyond the
// region never move), so a frame rewrite inside it — a tail call — is as
// free here as on the main loop. The pointer returns to where it stood on
// every exit.
//
// A break/continue the elements let out is NUR358's: a loop of the region's
// own takes it in place (`[for 2 [break]]`); otherwise the literal is not a
// loop, so the region is abandoned — the escaping run's position held for
// the `outside loop` report, its live frames and loops unwound, its tokens
// spliced away — with the signal left set and nothing returned: the caller
// abandons the literal and its run resolves the signal. An error leaves the
// region on the tape for the run's fault return to attribute and unwind, as
// a paren group's does; the step budget and the tape ceiling report as they
// do on the main loop.
//
// The parser balances a literal's parens, so a region always collapses at
// its own close paren; a Go-built literal can leave the seal open (an
// unmatched `(` among the elements, or a token that steps past the region),
// and then the elements' own run's report stands: unmatched opening
// parenthesis.
func (e *Engine) evalSealed(toks []Value, kind string) ([]Value, error) {
	saved := e.Pointer
	at := min(max(saved+1, 0), e.Tape.Len())
	tail := e.Tape.Len() - at
	lp, drv := e.takeSealed(kind)
	hold := e.holdForSealed(drv)
	defer func() {
		e.releaseSealed(drv, hold)
		e.sealDepth--
	}()
	e.loopTokens = sealedRegionTokens(e.loopTokens, lp, toks)
	e.Tape.Splice(at, 0, e.loopTokens...)
	e.Pointer = at
	for step := 0; !drv.done; step++ {
		if e.stepsTaken-e.budgetBase >= e.stepLimit {
			return nil, e.failSealed(saved, at, tail, e.evalLimitError(e.stepLimit))
		}
		e.stepsTaken++
		if e.Tape.Exhausted() {
			return nil, e.failSealed(saved, at, tail, e.tapeExhaustedError())
		}
		end := e.Tape.Len() - tail
		if e.Pointer >= end {
			break
		}
		val := e.Tape.At(e.Pointer)
		// Line-coverage seam (coverage.go): the region's tokens step here,
		// off the main loop — the main loop's per-step emit, mirrored.
		e.Registry.noteCoverage(val.Pos())
		if e.trace != nil {
			snapshot := e.Tape.Snapshot()
			note := e.traceNote
			e.traceNote = ""
			e.trace(step, e.Pointer, snapshot, note)
		}
		if err := e.stepToken(val); err != nil {
			return nil, e.failSealed(saved, at, tail, err)
		}
		if e.Registry.FlowCtrl != FlowNone {
			end = e.Tape.Len() - tail
			if e.loopContWithin(end) && e.handleFlowCtrl() {
				continue
			}
			e.escapeSealed(at, end)
			e.Pointer = saved
			return nil, nil
		}
	}
	if !drv.done {
		return nil, e.failSealed(saved, at, tail, unmatchedOpenParen(e))
	}
	e.Pointer = saved
	if hasOpenParen(drv.out) {
		return nil, unmatchedOpenParen(e)
	}
	return drv.out, nil
}

// unmatchedOpenParen is the elements' own run's report of a seal left open:
// no position, as the run's end-of-tape drain had none.
func unmatchedOpenParen(e *Engine) error {
	return makeBoruErrorAt("syntax_error", "unmatched opening parenthesis", "(", e.effectiveSource(), "", SrcPos{})
}

// failSealed ends the sealed region at [at, Len-tail) on an error raised
// inside it exactly as the sub-engine's fault return ended its run
// (faultReturn): the trace is told first — the debugger's post-mortem
// reads the raise's scope at this note, before the unwind — the error is
// attributed to the driven loops still live inside the region (`each:
// element 2: …`, innermost first), the region's live loops and frames are
// unwound (the region's own one-iteration loop among them: its context
// layer pops) and its tokens leave the tape, so a caller that trapped the
// error finds the tape as it left it. The run's own fault return then
// attributes and unwinds what encloses the literal, as it did the
// sub-engine's error. The pointer returns to where it stood.
func (e *Engine) failSealed(saved, at, tail int, err error) error {
	if e.trace != nil {
		e.trace(-1, e.Pointer, e.Tape.Snapshot(), "fault: "+err.Error())
	}
	end := e.Tape.Len() - tail
	err = e.wrapLoopFault(at, end, err)
	e.unwindLiveLoops(at, end)
	e.unwindLiveFrames(at, end)
	e.Tape.Splice(at, end-at)
	e.Pointer = saved
	return err
}

// sealedHold is the engine state a sealed region sets aside for its
// duration and hands back as it was (holdForSealed / releaseSealed): what
// the dispatch HOLDING the literal keeps live across the evaluation, and
// what a sub-engine kept apart as state of its own.
//
//   - resolvedScratch: the holder's match reads its resolved stack through
//     a view of this buffer (EffectiveResolved) while a pattern evaluates
//     the literal mid-match (evalPatternOperand); a dispatch inside the
//     region would rebuild the buffer under that view. The region's
//     dispatches get a buffer of their own.
//   - parenEvalDepth: a tail call nests under a forward paren group's
//     evaluation because that loop counts the group's parens as it steps
//     them (tcoEligible); the region's tokens are stepped here, inside one
//     of that loop's steps, so nothing it counts can move — tail calls
//     inside the literal replace their frames as they did on a sub-engine.
//   - patternEvalMake / patternEvalDid: the holder's dispatch is mid-match
//     when a pattern evaluates the literal; a dispatch inside the region
//     resets them for a match of its own.
//   - voidGroups: the void-group candidates are a run's; the region's are
//     the literal's and go with it, as a sub-engine's went.
//   - recoveryRaw: the holder's no-match recovery keeps the literals it
//     replaced by tape index; a dispatch inside the region records and
//     clears its own.
//   - recorder: the StackForm recorder saw a literal's elements on no
//     engine of its own; it does not see them now.
type sealedHold struct {
	resolved        []Value
	parenDepth      int
	patMake, patDid bool
	voidGroups      int
	recoveryRaw     map[int]Value
	recorder        Recorder
}

func (e *Engine) holdForSealed(drv *sealedDriver) sealedHold {
	h := sealedHold{
		resolved: e.resolvedScratch, parenDepth: e.parenEvalDepth,
		patMake: e.patternEvalMake, patDid: e.patternEvalDid,
		voidGroups: len(e.voidGroups), recoveryRaw: e.recoveryRaw, recorder: e.recorder,
	}
	e.resolvedScratch, e.parenEvalDepth, e.recoveryRaw, e.recorder = drv.resolved, 0, nil, nil
	return h
}

func (e *Engine) releaseSealed(drv *sealedDriver, h sealedHold) {
	drv.resolved = e.resolvedScratch
	e.resolvedScratch, e.parenEvalDepth = h.resolved, h.parenDepth
	e.patternEvalMake, e.patternEvalDid = h.patMake, h.patDid
	e.voidGroups = e.voidGroups[:h.voidGroups]
	e.recoveryRaw, e.recorder = h.recoveryRaw, h.recorder
}

// escapeSealed abandons the sealed region [at, end) a break/continue
// escaped. Where the elements' run stood is held for the `outside loop`
// report (Registry.HoldFlowAt): the token at the pointer when it is one of
// the elements' own — on the region's close paren or its move the run
// stood past its last token, as a sub-engine's pointer stood past its tape,
// and holds no position. The frames and loops still live inside the region
// are unwound, the region's own one-iteration loop among them (its context
// layer pops, its mark is forgotten), and the tokens leave the tape.
func (e *Engine) escapeSealed(at, end int) {
	pos, set := SrcPos{}, e.Pointer < end-1
	if set && e.Pointer == end-2 && IsCloseParen(e.Tape.At(e.Pointer)) {
		set = false
	}
	if set {
		pos = e.Tape.At(e.Pointer).Pos()
	}
	e.Registry.HoldFlowAt(pos, set)
	e.unwindLiveFrames(at, end)
	e.abandonDrivenLoops(at, end)
	e.Tape.Splice(at, end-at)
}

// hasOpenParen reports an open paren among a region's survivors: a seal
// left open by an unmatched `(` among the elements.
func hasOpenParen(vals []Value) bool {
	for _, v := range vals {
		if IsOpenParen(v) {
			return true
		}
	}
	return false
}
