package core

import "fmt"

// LoopDriver drives a DRIVEN Loop — the protocol a native looping construct
// (each, fold, scan, for-each, filter, outer, inner, eachrank, foldaxis) uses
// to run its iterations ON THE TAPE instead of on a pooled sub-engine per
// element. The construct's handler hands StartLoop a driver; the engine then
// splices one sealed region per iteration —
//
//	mark (ₗ inputs… body… ) move
//
// — exactly as `for` splices `mark body… move`: the mark opens the iteration,
// the paren seals the body's stack to its inputs (LoopOpenInfo), the body
// steps on the running tape, and the move hands the region's residual to the
// driver and asks for the next iteration. The loop is visible to the trace,
// the debugger and the step budget; break/continue inside the body pass
// THROUGH the loop to the enclosing `for`/`while` (an iterating native is not
// a loop for break, as it never was — the handler returned nothing on an
// escape and the run resolved the signal); an error raised in the body
// unwinds the loop and carries the driver's attribution (WrapError).
//
// Under the bytecode VM a handler is reached with the registry's Invoker set
// and the body already lowered to a closure: there StartLoop drives the SAME
// driver from Go (DriveLoop), invoking the body through the seam the handler
// used before — one definition of each construct's iteration order, result
// assembly and error texts for both lanes.
type LoopDriver interface {
	// Next prepares iteration it (0-based): the INERT inputs the region
	// opens with — resolved values the body sees as its stack, bottom
	// first, never re-stepped — and the body to step after them (a quoted
	// token list, a Function value, a compiled closure: BodyTokens decides
	// how it enters the tape, InvokeBody how the VM lane runs it). ok is
	// false when the loop has no iteration left; err is the construct's
	// own error (a structural fault the walk meets lazily), reported as
	// the construct's, never wrapped as a body's.
	Next(it int) (inputs []Value, body Value, ok bool, err error)
	// Collect takes iteration it's residual — the values its region left,
	// bottom→top, the sub-engine protocol's result stack — valid only for
	// the call. Its error is the construct's own (`each_error: body
	// produced no result`).
	Collect(it int, residual []Value) error
	// Finish assembles the loop's results, spliced in the loop's place and
	// re-stepped exactly as a handler's results are.
	Finish() ([]Value, error)
	// Describe renders the loop's live state for the trace, the move's
	// render and the debugger: the construct, the iteration, the extent.
	Describe() string
	// WrapError attributes a BODY error raised inside iteration it to the
	// construct (`each: element 2: …`), as the handler wrapped the
	// sub-engine's error.
	WrapError(it int, err error) error
}

// LoopInvoke runs one iteration's body against its inputs when the loop is
// driven from Go (DriveLoop): the seam the handler invoked bodies through
// before the loop moved to the tape. Nil means InvokeBody on the registry.
type LoopInvoke func(body Value, inputs []Value) ([]Value, error)

// StartLoop is a looping native's handler exit: it returns the handler's
// RESULT — either the loop's first region (mark, sealed inputs+body, move)
// for the engine to step, or, with no iteration at all, the driver's
// finished results. Under the VM (the registry's Invoker set, the body a
// compiled closure) it drives the loop from Go instead and returns the
// loop's results outright: a VM dispatch pushes a handler's results and
// never steps them.
func StartLoop(r *Registry, word string, drv LoopDriver, invoke LoopInvoke) ([]Value, error) {
	if r.Invoker != nil {
		return DriveLoop(r, drv, invoke)
	}
	lp := &Loop{Registry: r, Word: word, Count: -1, Driver: drv}
	if r.Check != nil {
		lp.Pos = r.Check.CurWordPos
	}
	inputs, body, ok, err := drv.Next(0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return drv.Finish()
	}
	lp.Iter = 1
	return loopRegionTokens(nil, lp, inputs, body), nil
}

// DriveLoop runs a driven loop from Go: each iteration's body through
// invoke (InvokeBody when nil), the residual to the driver, a body error
// wrapped by the driver, an escaping break/continue ending the loop with no
// result (BodyEscaped). This is the handler's historical loop, kept for the
// VM lane and for any caller that cannot step tokens.
func DriveLoop(r *Registry, drv LoopDriver, invoke LoopInvoke) ([]Value, error) {
	for it := 0; ; it++ {
		inputs, body, ok, err := drv.Next(it)
		if err != nil {
			return nil, err
		}
		if !ok {
			return drv.Finish()
		}
		// The driver may reuse its inputs buffer across iterations; the
		// seam below keeps the slice it is handed, so give it its own.
		in := append([]Value(nil), inputs...)
		var res []Value
		if invoke != nil {
			res, err = invoke(body, in)
		} else {
			res, err = InvokeBody(r, body, in)
		}
		if err != nil {
			return nil, drv.WrapError(it, err)
		}
		if BodyEscaped(r) {
			return nil, nil
		}
		if err := drv.Collect(it, res); err != nil {
			return nil, err
		}
	}
}

// callDriver is the driver of a CALL region (CallRegion): a native's tail
// invocation of a body — a case block, a computed `if` arm — returned to
// the engine as a loop of one iteration, so the body steps on the running
// tape where a sub-engine ran it. Its residual is the handler's result:
// spliced in the region's place and re-stepped, exactly as the sub-engine's
// result stack was when the handler returned it.
type callDriver struct {
	inputs []Value
	body   Value
	kind   string
	out    []Value
	lp     *Loop // the pooled Loop this drives, released at Finish; nil when driven from Go
}

func (d *callDriver) Next(it int) ([]Value, Value, bool, error) {
	if it > 0 {
		return nil, Value{}, false, nil
	}
	return d.inputs, d.body, true, nil
}

// Collect keeps the residual for Finish, which follows it at once: on the
// tape the slice is the loop's own collection, spliced in the region's
// place before anything touches it again; from Go it is the run's copy.
func (d *callDriver) Collect(_ int, residual []Value) error {
	d.out = residual
	return nil
}

// Finish hands the residual back and returns the region's Loop to the
// registry's pool: the region has collapsed, its mark forgotten, so the
// Loop's tokens and mark id are free for the next call.
func (d *callDriver) Finish() ([]Value, error) {
	out := d.out
	if d.lp != nil {
		d.lp.Registry.putCallLoop(d.lp)
	}
	return out, nil
}

func (d *callDriver) Describe() string { return d.kind }

// WrapError leaves a body fault as it is: the sub-engine's error was the
// handler's own result, with no construct to attribute it to.
func (d *callDriver) WrapError(_ int, err error) error { return err }

// CallRegion is a native handler's exit for a body it would otherwise run as
// its LAST act — InvokeBody's or RunResolved's run of a code body over
// resolved inputs, the result stack returned as the handler's own. It
// returns the call as a region of the tape instead (a one-iteration driven
// loop, loop.go): the inputs enter sealed and inert, the body steps on the
// running tape under its own context layer and step budget, a
// break/continue passes through to the enclosing loop as it passed through
// the sub-engine's run, a fault keeps the body's own attribution, and the
// residual replaces the region as the handler's results would, re-stepped.
// The region's close paren carries the word's position, so a signal the
// body lets out with no loop to take it reports at the construct. kind
// names the region for the trace and the debugger. Under the VM (the
// registry's Invoker set) the body runs from Go through InvokeBody, as the
// handler ran it before; a handler that ran it another way there
// (RunResolved) keeps that lane itself.
//
// A one-shot region cannot amortise its tokens over iterations as a loop
// does, so its Loop — the driver, the four minted tokens, the mark id — is
// pooled on the registry and reused once the region has collapsed
// (takeCallLoop / putCallLoop): a call allocates nothing of its own.
//
// Only a TAIL invocation qualifies. A handler that reads the body's result
// — `do` trapping an error, a predicate coerced to a Boolean, a scrutinee's
// last value, a callback whose count the seam trims (InvokeCallbackFn) —
// runs it as it did: the region's residual is the engine's, not the
// handler's.
func CallRegion(r *Registry, word, kind string, inputs []Value, body Value) ([]Value, error) {
	if r.Invoker != nil {
		return DriveLoop(r, &callDriver{inputs: inputs, body: body, kind: kind}, nil)
	}
	lp := r.takeCallLoop(word, kind, inputs, body)
	lp.toks = loopRegionTokens(lp.toks, lp, inputs, body)
	return lp.toks, nil
}

// maxCallLoops bounds the registry's pool of call-region Loops: deeper
// nesting of live call regions than this mints and drops.
const maxCallLoops = 64

// takeCallLoop takes a call-region Loop from the registry's pool — or
// mints one — set up for this call: the word, the region's description,
// the driver's inputs and body, the word's position for the mark, the
// move and the close paren (the minted tokens point at the Loop's own Pos,
// so a reused Loop's tokens take the new call's position).
func (r *Registry) takeCallLoop(word, kind string, inputs []Value, body Value) *Loop {
	var lp *Loop
	if n := len(r.callLoops); n > 0 {
		lp = r.callLoops[n-1]
		r.callLoops[n-1] = nil
		r.callLoops = r.callLoops[:n-1]
	} else {
		lp = &Loop{Registry: r, closeAtWord: true}
		lp.Driver = &callDriver{lp: lp}
	}
	drv := lp.Driver.(*callDriver)
	drv.inputs, drv.body, drv.kind, drv.out = inputs, body, kind, nil
	lp.Word, lp.Iter, lp.Count, lp.Results = word, 1, 1, lp.Results[:0]
	lp.Pos = SrcPos{}
	if r.Check != nil {
		lp.Pos = r.Check.CurWordPos
	}
	return lp
}

// putCallLoop returns a call-region Loop whose region has collapsed to the
// pool, its call's references released. A Loop abandoned mid-region (a
// signal or a fault discarded it) is never returned: its tokens may still
// lie on a dead tape.
func (r *Registry) putCallLoop(lp *Loop) {
	if len(r.callLoops) >= maxCallLoops {
		return
	}
	drv := lp.Driver.(*callDriver)
	drv.inputs, drv.body, drv.out = nil, Value{}, nil
	lp.Results = lp.Results[:0]
	r.callLoops = append(r.callLoops, lp)
}

// loopRegionTokens builds one iteration's token run into buf: the loop's
// mark, the sealing paren with its inert-input span, the inputs, the body's
// tokens, the close paren and the move. The synthetic tokens are minted
// once per loop and reused — one mark id for the loop's life (the region
// is replaced in place, so the id is on the tape exactly once), the paren
// re-minted only when the input span changes — so an iteration allocates
// nothing of its own. The mark and the move carry the owning word's
// position, so an error the loop itself raises at its move (stampErrPos
// reads the pointer's token) is reported at the word, as a handler's error
// is.

// IsLoopRegion reports whether a handler's results are a driven loop's
// first region (StartLoop's tokens) rather than values: the region's body
// tokens are the program's own and keep their positions — execMatch does
// not stamp them with the word's, as it stamps a handler's fresh values.
func IsLoopRegion(results []Value) bool {
	if len(results) == 0 || !IsMark(results[0]) {
		return false
	}
	info, _ := AsMark(results[0])
	return info.Loop != nil
}
func loopRegionTokens(buf []Value, lp *Loop, inputs []Value, body Value) []Value {
	lp.mint(len(inputs))
	out := append(buf[:0], lp.mark, lp.open)
	out = append(out, inputs...)
	out = append(out, lp.bodyTokens(body)...)
	return append(out, lp.close, lp.move)
}

// mint makes the loop's synthetic tokens once — the mark, the close paren
// and the move on the first call, the sealing paren whenever the inert-input
// span differs from the one it was minted for.
func (lp *Loop) mint(argSpan int) {
	if lp.mark.Parent == nil {
		id := NextMarkID()
		lp.mark = NewLoopMark(id, lp)
		lp.close = NewCloseParen()
		lp.move = NewMoveCont(id, lp.word()+" loop", lp)
		if lp.Pos.Row != 0 || lp.closeAtWord {
			// The mark and the move only: a loop's parens stay unpositioned,
			// so a signal or an error reported where the pointer stands
			// after the body (the close paren) reads as it did off the
			// sub-engine's residual — no position — rather than blaming
			// the word. A call region's close paren takes the word's: the
			// construct whose block let a signal out is where the report
			// points (closeAtWord) — and its tokens point at the Loop's Pos
			// whatever it holds now, as the Loop is reused call after call.
			lp.mark.pos, lp.move.pos = &lp.Pos, &lp.Pos
			if lp.closeAtWord {
				lp.close.pos = &lp.Pos
			}
		}
	}
	if lp.open.Parent == nil || lp.openSpan != argSpan {
		lp.open, lp.openSpan = NewLoopOpen(lp, argSpan), argSpan
	}
}

// bodyTokens is BodyTokens without the copy: a concrete list's own elements
// (the splice copies them into the tape; the list is never written), or
// the value itself as the loop's one-token body. BodyTokens copied the
// list per call — one allocation per iteration.
func (lp *Loop) bodyTokens(body Value) []Value {
	if lst, err := AsList(body); err == nil && !lst.IsNil() {
		return lst.elems
	}
	lp.one[0] = body
	return lp.one[:]
}

// word is the loop's label: Word when the builder set one, else the mode.
func (lp *Loop) word() string {
	switch {
	case lp.Word != "":
		return lp.Word
	case lp.Driver != nil:
		return "loop"
	case lp.WhileCond != nil:
		return "while"
	}
	return "for"
}

// Describe renders the loop's live state — the construct, the iteration
// and, where known, the extent and the index — for the trace notes, the
// mark/move renders and the debugger's backtrace.
func (lp *Loop) Describe() string {
	if lp == nil {
		return ""
	}
	switch {
	case lp.Driver != nil:
		return lp.Driver.Describe()
	case lp.WhileCond != nil:
		phase := "cond"
		if lp.WhileInBody {
			phase = "body"
		}
		return fmt.Sprintf("%s #%d %s", lp.word(), lp.Iter, phase)
	case lp.Count > 0:
		return fmt.Sprintf("%s %s=%d %d/%d", lp.word(), lp.IterName, lp.Current, lp.Iter, lp.Count)
	}
	return fmt.Sprintf("%s %s=%d", lp.word(), lp.IterName, lp.Current)
}

// openArgSpan is the inert-input span an open paren carries: a fn frame's
// resolved unnamed arguments (FrameOpenInfo) or a driven loop region's
// inputs (LoopOpenInfo); 0 for a plain paren.
func openArgSpan(v Value) int {
	switch info := v.Data.(type) {
	case FrameOpenInfo:
		return info.ArgSpan
	case LoopOpenInfo:
		return info.ArgSpan
	}
	return 0
}

// beginLoopIteration is stepMark's arm for a driven loop's mark: the
// iteration pushes a scoped context Store, as the sub-engine protocol's
// Run pushed one per body run (the context boundary — `context set` in a
// body stays the iteration's; the VM's enterBodyUnit is the twin), and the
// trace notes the iteration.
func (e *Engine) beginLoopIteration(lp *Loop) {
	if !lp.ctxPushed {
		e.Registry.Contexts.Push(e.Registry.Contexts.Top())
		lp.ctxPushed = true
		// The iteration's own step budget: the scope's base moves to the
		// iteration's entry, so it has the full limit ahead of it, as the
		// sub-engine that ran a body had a budget of its own. The mark's
		// step — counted by the stepping loop before this call — is the
		// iteration's, so the base sits just before it.
		lp.enterSteps, lp.outerBase = e.stepsTaken-1, e.budgetBase
		e.budgetBase = e.stepsTaken - 1
	}
	if e.trace != nil {
		e.traceNote = "loop " + lp.Describe()
	}
}

// endLoopIteration pops the live iteration's context Store, once, and
// hands the enclosing scope its budget back with the iteration's steps
// uncharged: the base moves up by what the iteration spent, so the
// enclosing count reads as it did at the iteration's entry.
func (e *Engine) endLoopIteration(lp *Loop) {
	if lp.ctxPushed {
		e.Registry.Contexts.Pop()
		lp.ctxPushed = false
		e.budgetBase = lp.outerBase + (e.stepsTaken - lp.enterSteps)
	}
}

// stepMoveDriven is stepMoveCont's arm for a driven loop: the region's
// residual — a pending residual container evaluated here, inside the
// iteration's context layer, as the sub-engine's end-of-run sweep
// evaluated it — goes to the driver, then the next region or the results
// replace the region. A break/continue a residual literal let out leaves
// the signal set and the pointer on the move for the run's resolver, which
// passes through this loop to the enclosing one (handleLoopBreak).
func (e *Engine) stepMoveDriven(markIdx, moveIdx int, info MoveInfo) error {
	lp := info.Cont
	lp.Results = lp.Results[:0]
	if escaped, err := e.collectLoopRegion(lp, markIdx, moveIdx); err != nil || escaped {
		return err
	}
	e.endLoopIteration(lp)
	if err := lp.Driver.Collect(lp.Iter-1, lp.Results); err != nil {
		return e.failDrivenLoop(info, err)
	}
	return e.spliceDrivenIteration(markIdx, moveIdx, info)
}

// spliceDrivenIteration asks the driver for the next iteration and splices
// its region over the finished one, or the driver's results when the loop
// is done. The pointer returns to the mark, so the new region is stepped —
// or the results re-stepped, exactly as a handler's results are.
func (e *Engine) spliceDrivenIteration(markIdx, moveIdx int, info MoveInfo) error {
	lp := info.Cont
	inputs, body, ok, err := lp.Driver.Next(lp.Iter)
	if err != nil {
		return e.failDrivenLoop(info, err)
	}
	if !ok {
		delete(e.marks, info.To)
		results, err := lp.Driver.Finish()
		if err != nil {
			return e.stampErrPos(err)
		}
		e.Tape.Splice(markIdx, moveIdx-markIdx+1, results...)
		e.Pointer = markIdx
		if e.trace != nil {
			e.traceNote = "loop done " + lp.word()
		}
		return nil
	}
	// The next region replaces this one in place; the loop's one mark id
	// stays registered, and the iteration begins HERE rather than by
	// re-stepping the mark — the pointer lands on the sealing paren, whose
	// step skips the inputs as ever (one Run-loop step fewer per element).
	lp.Iter++
	e.loopTokens = loopRegionTokens(e.loopTokens, lp, inputs, body)
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, e.loopTokens...)
	e.beginLoopIteration(lp)
	e.Pointer = markIdx + 1
	if e.trace != nil {
		e.traceNote = "loop next " + info.To + " " + lp.Describe()
	}
	return nil
}

// failDrivenLoop ends a driven loop on the driver's OWN error — a residual
// the construct rejects, a structural fault the walk meets: the loop is
// closed first, so the fault's unwind does not attribute the error to the
// loop's body (wrapLoopFault), and the error is positioned at the loop's
// word, as a handler's error would be.
func (e *Engine) failDrivenLoop(info MoveInfo, err error) error {
	e.endLoopIteration(info.Cont)
	delete(e.marks, info.To)
	return e.stampErrPos(err)
}

// abandonDrivenLoops ends every LIVE driven loop whose move lies in
// [from, to) — its mark stepped, its move not yet reached — when the region
// holding it is discarded: a break/continue resolving to an enclosing loop,
// a run exiting with the signal unresolved, a fault. The iteration's
// context layer is popped and the mark forgotten; the driver's state goes
// with the tokens. Innermost first, as the moves lie on the tape, so the
// context layers pop in the order they were pushed.
func (e *Engine) abandonDrivenLoops(from, to int) {
	if e.marks == nil {
		return
	}
	if to > e.Tape.Len() {
		to = e.Tape.Len()
	}
	for i := max(from, 0); i < to; i++ {
		v := e.Tape.At(i)
		if !IsMove(v) {
			continue
		}
		info, _ := AsMove(v)
		if info.Cont == nil || info.Cont.Driver == nil || !e.marks[info.To] {
			continue
		}
		e.abandonDrivenLoop(info)
	}
}

func (e *Engine) abandonDrivenLoop(info MoveInfo) {
	e.endLoopIteration(info.Cont)
	delete(e.marks, info.To)
}

// wrapLoopFault attributes a fault raised inside a driven loop's body to
// the loop — `each: element 2: …` — through every live driven loop whose
// move lies in [from, to), innermost first (the tape ahead of the pointer
// for the run's fault return; a sealed region's extent for the region's,
// failSealed), as each handler wrapped the error its sub-engine returned
// before the next handler out wrapped that.
func (e *Engine) wrapLoopFault(from, to int, err error) error {
	if e.marks == nil {
		return err
	}
	for i := from; i < to; i++ {
		v := e.Tape.At(i)
		if !IsMove(v) {
			continue
		}
		info, _ := AsMove(v)
		if info.Cont == nil || info.Cont.Driver == nil || !e.marks[info.To] {
			continue
		}
		err = info.Cont.Driver.WrapError(info.Cont.Iter-1, err)
	}
	return err
}
