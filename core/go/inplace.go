package core

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// In-place compilation — PROTOTYPE, off by default
// (design/IN-PLACE-COMPILATION.0.md).
//
// When a word's call is fully matched, the engine stops moving cells. The
// call's LAST cell — its last forward operand, or the word itself when every
// operand came from the value stack (or there are none) — is overwritten with
// a CALL cell holding the matched dispatch; every other cell the call claimed
// (the word, its forward marker, its other operands) is overwritten with a
// NOP cell; and the pointer is set on the call cell. The next step executes
// the call cell through execMatch with no further matching, and its results
// replace that one cell. A forward call's operands never move: they stay
// where forward collection found them — no rearrangement into stack order —
// and the call binds them in signature order from their positions.
//
// Nop cells are transparent: no scan treats one as a value, a barrier or an
// argument. They only ever exist below the pointer, and they are short-lived:
// a call cell, when it executes, first drops the nops its own claim left
// directly beneath it (dropCallSpan — one Splice beside the gap, the common
// case), so the backward scans every later dispatch makes never walk them.
// What that leaves — a span a mark interrupts — goes at a paren group's
// collapse, a loop iteration's collection or the run's end, and a statement
// stream that piles more up compacts them every nopCompactEvery writes.
//
// Both cells are engine markers parented on Word/__IN with their own
// payloads, so every predicate that already treats an internal marker as
// "not a value" (isEngineMarker, IsRecordableLiteral, ForwardLiteralOperand)
// classifies them that way without a new lattice node.
//
// The switch is Registry.InPlace (process default BORU_INPLACE: "1"/"on",
// or "verify"). It is never honoured under an analysis pass or a stack-form
// recorder, both of which read the legacy layout.

// InPlaceMode selects the interpreter's call-completion mechanism.
type InPlaceMode int8

const (
	// InPlaceOff is the legacy completion: each arrival is moved before the
	// word, the word is rewritten to its /s form, the operands are
	// rearranged into stack order, the word re-steps (a second plan), and
	// the result is spliced over the span.
	InPlaceOff InPlaceMode = iota
	// InPlaceOn completes a fully matched word call in place (above).
	InPlaceOn
	// InPlaceVerify runs the legacy completion and counts, at each re-step,
	// whether the re-plan chose the signature the plan chose — the question
	// in-place completion answers by assumption (the note's risk R1).
	InPlaceVerify
)

// defaultInPlaceMode is the process default every new registry starts with.
var defaultInPlaceMode = ParseInPlaceMode(os.Getenv("BORU_INPLACE"))

// ParseInPlaceMode reads a BORU_INPLACE value: "1", "on" or "true" selects
// InPlaceOn, "verify" InPlaceVerify, anything else InPlaceOff.
func ParseInPlaceMode(s string) InPlaceMode {
	switch s {
	case "1", "on", "true":
		return InPlaceOn
	case "verify":
		return InPlaceVerify
	}
	return InPlaceOff
}

// nopCompactEvery is how many nops an engine may write before the step loop
// tries to drop the ones below the pointer (compactNops).
const nopCompactEvery = 64

// NopInfo is the payload of a nop cell: a cell a completed in-place call
// claimed. It does nothing, is always skipped, and is never an argument. A
// zero-size payload, so writing one never allocates.
type NopInfo struct{}

func (NopInfo) payloadMarker()            {}
func (NopInfo) IsTypeContent(*Value) bool { return false }

// CallInfo is the payload of a call cell: a fully matched dispatch compiled
// in place of the call's last cell. Executing the cell runs Sig over Args
// with no further matching.
type CallInfo struct {
	match MatchResult
}

// callInfo1, callInfo2 and callInfo3 hold a record together with its operand
// storage, sized to the call: one allocation, of the bytes the legacy lane
// spends on a match and its argument slice in two. The cell points at the
// embedded record.
type callInfo1 struct {
	CallInfo
	argv [1]Value
}

type callInfo2 struct {
	CallInfo
	argv [2]Value
}

type callInfo3 struct {
	CallInfo
	argv [3]Value
}

func (*CallInfo) payloadMarker()            {}
func (*CallInfo) IsTypeContent(*Value) bool { return false }

// Name is the dispatched word; Args the operands in signature order.
func (c *CallInfo) Name() string    { return c.match.Name }
func (c *CallInfo) Args() []Value   { return c.match.Args }
func (c *CallInfo) Sig() *Signature { return c.match.Sig }

// newCallInfo builds a call record over n operands.
func newCallInfo(name string, sig *Signature, n, lo int) *CallInfo {
	var ci *CallInfo
	var args []Value
	switch n {
	case 0:
		ci = &CallInfo{}
	case 1:
		c := &callInfo1{}
		ci, args = &c.CallInfo, c.argv[:0:1]
	case 2:
		c := &callInfo2{}
		ci, args = &c.CallInfo, c.argv[:0:2]
	case 3:
		c := &callInfo3{}
		ci, args = &c.CallInfo, c.argv[:0:3]
	default:
		ci, args = &CallInfo{}, make([]Value, 0, n)
	}
	ci.match = MatchResult{Sig: sig, Name: name, Args: args, InPlace: true, SpanLo: lo}
	return ci
}

// nopCell is the one nop value every in-place write stores.
var nopCell = Value{Parent: TInternal, Data: NopInfo{}}

// IsNop reports whether v is a nop cell.
func IsNop(v Value) bool {
	if v.Parent != TInternal {
		return false
	}
	_, ok := v.Data.(NopInfo)
	return ok
}

// AsCall returns a call cell's record.
func AsCall(v Value) (*CallInfo, bool) {
	if v.Parent != TInternal {
		return nil, false
	}
	ci, ok := v.Data.(*CallInfo)
	return ci, ok
}

// IsCall reports whether v is a call cell.
func IsCall(v Value) bool {
	_, ok := AsCall(v)
	return ok
}

// InPlaceCounters are the prototype's process-wide counters, read by the
// measurement tests. Atomic: a module sub-registry is shared across
// goroutines, and forked registries run concurrently.
type InPlaceCounters struct {
	Calls      atomic.Int64 // call cells executed
	Completed  atomic.Int64 // forward calls compiled in place at their last arrival
	StackCalls atomic.Int64 // value-stack and nullary calls compiled in place
	Normalized atomic.Int64 // in-place forwards rewritten to the legacy layout (cold paths)
	// NormalizedAt splits Normalized by the path that asked (NormSite).
	NormalizedAt   [normSites]atomic.Int64
	GenFallback    atomic.Int64 // completions that took the legacy path because the word was rebound
	ReplanFallback atomic.Int64 // completions that took the legacy path because the plan no longer held
	Compactions    atomic.Int64 // statement-stream nop compactions
	NopsDropped    atomic.Int64 // nops dropped by any compaction
	Verified       atomic.Int64 // verify mode: the re-plan chose the plan's signature
	Disagreed      atomic.Int64 // verify mode: it did not
}

// NormSite names the cold path that normalised an in-place forward.
type NormSite int

const (
	NormCommit     NormSite = iota // a barrier commit (commitBarrierForward)
	NormImplicit                   // an arrival the next slot refuses (implicitEnd)
	NormStmtEnd                    // an `end` (stepEnd)
	NormParenClose                 // a paren group's close (stepCloseParen)
	NormInputEnd                   // the end of input (resolveOrphanedForwards)
	NormLoopRegion                 // a loop iteration's collection (collectLoopRegion)
	NormFallback                   // a completion handed back to the legacy path (completeLegacy)
	normSites
)

// InPlaceStats is the process-wide counter set.
var InPlaceStats InPlaceCounters

// InPlaceDisagreement is one verify-mode finding: the plan chose Planned, the
// legacy re-step's re-plan chose Replanned (nil when it found none).
type InPlaceDisagreement struct {
	Name               string
	Pos                SrcPos
	Planned, Replanned string
	Commit             bool
	// PlanHeld is whether the planned signature still matched the arrived
	// operands (planHolds): when false, in-place completion falls back to
	// the legacy re-plan and agrees with it; when true, in-place completion
	// would run the planned signature where the legacy lane runs another.
	PlanHeld bool
}

var (
	inPlaceFindingsMu sync.Mutex
	inPlaceFindings   []InPlaceDisagreement
)

// TakeInPlaceDisagreements returns the verify-mode findings recorded so far
// and clears them.
func TakeInPlaceDisagreements() []InPlaceDisagreement {
	inPlaceFindingsMu.Lock()
	defer inPlaceFindingsMu.Unlock()
	out := inPlaceFindings
	inPlaceFindings = nil
	return out
}

// inPlaceActive reports whether this engine compiles calls in place now.
func (e *Engine) inPlaceActive() bool {
	return e.Registry != nil && e.Registry.InPlace == InPlaceOn && e.recorder == nil && !e.Registry.analysisActive()
}

// inPlaceWord reports whether a word call that matched sig compiles in place.
// A FullStack signature replaces its whole scope and keeps the legacy path.
func (e *Engine) inPlaceWord(sig *Signature) bool {
	return sig != nil && !sig.FullStack() && e.inPlaceActive()
}

// planGen is the word's binding generation when the engine compiles calls
// in place (a forward completion checks it — ForwardInfo.Gen), and 0 on the
// legacy lane, which never reads it and so never pays for the lookup.
func (e *Engine) planGen(name string) int64 {
	if !e.inPlaceActive() {
		return 0
	}
	return e.Registry.Defs.Gen(name)
}

// setNop overwrites the cell at i with a nop.
func (e *Engine) setNop(i int) {
	e.Tape.Set(i, nopCell)
	e.nopPending++
	e.sawNop = true
}

// inPlaceOperands returns the indices of the first n collected operands of
// the in-place forward whose marker sits at fwdIdx: the non-nop cells after
// the marker, below limit. The layout invariant puts nothing else there.
func (e *Engine) inPlaceOperands(fwdIdx, limit, n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, 0, n)
	for i := fwdIdx + 1; i < limit && len(out) < n; i++ {
		if !e.Tape.nopAt(i) {
			out = append(out, i)
		}
	}
	return out
}

// claimedStack returns the indices (ascending) of the n value-stack operands
// a forward word at funcIdx claimed: the nearest resolved cells below it —
// the cells PlanMatch assigned to its stack slots, which no arrival touches.
func (e *Engine) claimedStack(funcIdx, n int) []int {
	if n <= 0 {
		return nil
	}
	return resolvedIndicesBeforeInto(e.Tape, funcIdx, make([]int, 0, n), n)
}

// arriveInPlace collects the value at valIdx into the in-place forward at
// fwdIdx WITHOUT moving it — the cell stays where forward collection found it
// — and compiles the call when it was the last operand.
func (e *Engine) arriveInPlace(fwd ForwardInfo, fwdIdx, valIdx int) error {
	// A collected value's reach-collapse tag is spent (the legacy arrival
	// clears it on the moved value).
	if val := e.Tape.At(valIdx); val.ReachGroup {
		val.ReachGroup = false
		e.Tape.Set(valIdx, val)
	}
	fwd.CollectedArgs++
	if e.trace != nil {
		e.traceNote = fmt.Sprintf("collect %s %d/%d", fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)
	}
	if fwd.CollectedArgs < fwd.ExpectedArgs {
		e.Tape.Set(fwdIdx, NewForward(fwd))
		e.Pointer = valIdx + 1
		return nil
	}
	return e.completeInPlace(fwd, fwdIdx, valIdx)
}

// completeInPlace compiles a fully collected in-place forward call into a
// call cell at cell (its last operand), with nops over every other claimed
// cell, and leaves the pointer on the call cell.
func (e *Engine) completeInPlace(fwd ForwardInfo, fwdIdx, cell int) error {
	funcIdx := fwd.FuncIndex
	ops := e.inPlaceOperands(fwdIdx, cell+1, fwd.CollectedArgs)
	stack := e.claimedStack(funcIdx, fwd.StackArgs)
	// The planned signature is executed directly only while the word still
	// names the binding it was planned against (a group among the operands
	// may have re-def'd it), only over exactly the cells the layout
	// invariant promises, and only while it still matches the operands as
	// the legacy completion's re-plan tests them (planHolds); anything else
	// takes the legacy completion, whose re-step resolves the name and
	// re-plans.
	positions := make([]int, 0, len(ops)+len(stack))
	positions = append(positions, ops...)
	for k := len(stack) - 1; k >= 0; k-- {
		positions = append(positions, stack[k])
	}
	if e.Registry.Defs.Gen(fwd.FuncName) != fwd.Gen || len(ops) != fwd.CollectedArgs ||
		len(stack) != fwd.StackArgs || ops[len(ops)-1] != cell {
		InPlaceStats.GenFallback.Add(1)
		return e.completeLegacy(fwd, fwdIdx, cell)
	}
	e.patternEvalMake, e.patternEvalDid = fwd.FuncName == "make", false
	holds := e.planHolds(fwd.Sig, positions, patternDispatch{e})
	if e.patternEvalAbandoned() {
		// A pattern operand's evaluation raised or let a signal out: the
		// dispatch is abandoned exactly as the re-plan abandons it.
		err := e.patternEvalErr
		e.patternEvalErr = nil
		return err
	}
	if !holds {
		InPlaceStats.ReplanFallback.Add(1)
		return e.completeLegacy(fwd, fwdIdx, cell)
	}
	lo := funcIdx
	if len(stack) > 0 {
		lo = stack[0]
	}
	ci := newCallInfo(fwd.FuncName, fwd.Sig, len(positions), lo)
	// Signature order: the forward operands in written order, then the
	// value-stack operands top-down.
	for _, i := range positions {
		ci.match.Args = append(ci.match.Args, e.Tape.At(i))
	}
	pos := e.Tape.At(funcIdx).pos
	for _, i := range stack {
		e.setNop(i)
	}
	e.setNop(funcIdx)
	e.setNop(fwdIdx)
	for _, i := range ops[:len(ops)-1] {
		e.setNop(i)
	}
	e.Tape.Set(cell, Value{Parent: TInternal, Data: ci, pos: pos})
	e.Pointer = cell
	if e.trace != nil {
		e.traceNote = "compile " + traceSigStr(fwd.FuncName, fwd.Sig)
	}
	InPlaceStats.Completed.Add(1)
	return nil
}

// completeLegacy finishes the in-place forward at fwdIdx the legacy way once
// the value at valIdx — its last operand — has arrived: the forward goes back
// to the count before that arrival, is normalised to the legacy layout, and
// the value is collected again by the legacy arrival, which completes the
// call (rearrange, /s, re-step, re-plan).
func (e *Engine) completeLegacy(fwd ForwardInfo, fwdIdx, valIdx int) error {
	fwd.CollectedArgs--
	e.Tape.Set(fwdIdx, NewForward(fwd))
	e.normalizeInPlaceForward(fwdIdx, NormFallback)
	e.Pointer = valIdx
	return e.stepLiteral()
}

// planHolds reports whether sig still matches the operands at positions
// (signature order) the way the legacy completion's re-plan tests them: each
// slot's type over the value that arrived, no type literal where the slot
// does not take one, a /q slot's word as a name, and every pattern judged
// as a STACK operand. The plan judged its forward slots by the tokens it saw
// and skips structural map patterns and non-concrete patterns there
// (patternsOk's isForward leniency); the re-plan, seeing the arrived values
// on the stack, enforces them — a map literal's shape, a `tnot List` input.
// pe evaluates a pending container a pattern reads (NUR235), as the re-plan
// does; nil judges the raw value.
func (e *Engine) planHolds(sig *Signature, positions []int, pe patternOperandHost) bool {
	for j, p := range positions {
		v := e.Tape.At(p)
		t := SigArgType(sig, j)
		if sig.QuoteArgs != nil && sig.QuoteArgs[j] && v.Parent.Equal(TWord) {
			if !TAtom.ConformsTo(t) {
				return false
			}
			continue
		}
		if !SigArgMatches(sig, j, v) {
			return false
		}
		if !(sig.TypeArgs != nil && sig.TypeArgs[j]) && rejectsTypeLiteral(v, t) {
			return false
		}
	}
	return patternsOk(sig, positions, e.Tape, 0, e.Registry, pe)
}

// prevNonNop returns the index of the nearest cell below i that is not a nop
// — a neighbour test reads past in-place compilation's claimed cells exactly
// as it read past the cells the legacy splice removed — or -1.
func (e *Engine) prevNonNop(i int) int {
	for i--; i >= 0; i-- {
		if !e.Tape.nopAt(i) {
			return i
		}
	}
	return -1
}

// compileStackCall compiles a word call whose operands all came from the
// value stack (or that takes none) into a call cell in the word's own cell,
// with nops over the operands; the pointer stays on the cell.
func (e *Engine) compileStackCall(w WordInfo, sig *Signature, positions []int) error {
	cell := e.Pointer
	lo := cell
	for _, p := range positions {
		if p < lo {
			lo = p
		}
	}
	ci := newCallInfo(w.Name, sig, len(positions), lo)
	for _, p := range positions {
		ci.match.Args = append(ci.match.Args, e.Tape.At(p))
	}
	for _, p := range positions {
		e.setNop(p)
	}
	e.Tape.Set(cell, Value{Parent: TInternal, Data: ci, pos: e.Tape.At(cell).pos})
	if e.trace != nil {
		e.traceNote = "compile " + traceSigStr(w.Name, sig)
	}
	InPlaceStats.StackCalls.Add(1)
	return nil
}

// dispatchStack executes a word call whose operands all came from the value
// stack — in place when the engine compiles calls in place, else at once
// (the legacy immediate execution).
func (e *Engine) dispatchStack(w WordInfo, sig *Signature, positions []int, stkCount int) error {
	if e.inPlaceWord(sig) {
		return e.compileStackCall(w, sig, positions)
	}
	match := &MatchResult{Sig: sig, Positions: positions, Name: w.Name}
	if stkCount > 0 {
		match.Args = make([]Value, stkCount)
		for i, pos := range positions {
			match.Args[i] = e.Tape.At(pos)
		}
	}
	if e.trace != nil {
		e.traceNote = "stack " + traceSigStr(w.Name, sig)
	}
	return e.execMatch(match)
}

// stepInPlaceCell steps a cell of this mechanism: a call cell drops its own
// nops and executes, a nop is skipped. ok is false for any other cell.
func (e *Engine) stepInPlaceCell(v Value) (ok bool, err error) {
	if ci, isCall := AsCall(v); isCall {
		e.dropCallSpan(ci)
		InPlaceStats.Calls.Add(1)
		if e.trace != nil {
			e.traceNote = "call " + traceSigStr(ci.match.Name, ci.match.Sig)
		}
		return true, e.execMatch(&ci.match)
	}
	if IsNop(v) {
		e.Pointer++
		return true, nil
	}
	return false, nil
}

// dropCallSpan removes the nops an executing call cell's own claim left
// directly beneath it — its span [SpanLo, pointer), when nothing else lies
// there, which is the common case — with one Splice beside the gap: the cell
// was written where the last operand arrived, just past the gap the marker's
// insert left, so the edit moves only the claimed cells. No later backward
// scan in the statement walks them. A span anything else interrupts (a mark
// a value-stack operand was claimed across) keeps its nops; they go with
// the region that holds them (dropAllNops, compactGroup, compactNops).
func (e *Engine) dropCallSpan(ci *CallInfo) {
	lo := ci.match.SpanLo
	n := e.Pointer - lo
	if n <= 0 {
		return
	}
	for j := lo; j < e.Pointer; j++ {
		if !e.Tape.nopAt(j) {
			return
		}
	}
	e.Tape.Splice(lo, n)
	e.Pointer = lo
	e.droppedNops(n)
}

// droppedNops accounts for n nops a pass removed: they no longer count
// toward the next statement compaction.
func (e *Engine) droppedNops(n int) {
	e.nopPending -= n
	if e.nopPending < 0 {
		e.nopPending = 0
	}
	InPlaceStats.NopsDropped.Add(int64(n))
}

// placeResults puts an executed call cell's results where the cell stands
// and leaves the pointer on the first of them: one result overwrites the
// cell, several replace the one cell, and none removes it — the pointer then
// stands on the cell after the call, where the legacy splice of the call's
// span leaves it, and where a signal the call raised (a `break` outside any
// loop) or the next error reads its source position. The cell sits at the
// gap once its span is dropped, so the removal moves nothing.
func (e *Engine) placeResults(results []Value) {
	switch len(results) {
	case 0:
		e.Tape.Splice(e.Pointer, 1)
	case 1:
		e.Tape.Set(e.Pointer, results[0])
	default:
		e.Tape.Splice(e.Pointer, 1, results...)
	}
}

// matchIndices is execMatch's operand positions: the recorded ones, else the
// resolved cells below the pointer — and none for an in-place call, whose
// operands are already in the match and whose claimed cells are nops.
func (e *Engine) matchIndices(match *MatchResult, n int) []int {
	if match.InPlace {
		return nil
	}
	indices := match.Positions
	if len(indices) == 0 && n > 0 {
		indices = e.ResolvedIndicesBefore(n)
	}
	return indices
}

// normalizeInPlaceForward rewrites the in-place forward at fwdIdx into the
// legacy layout — each collected operand moved from after the marker to just
// before the word, in arrival order, exactly where the legacy arrival would
// have inserted it — and clears its InPlace flag, so every cold path that
// reads a pending forward's layout (an implicit end, a statement end, a paren
// close, the end of input, a barrier commit, a loop iteration's collection)
// runs unchanged. Returns the marker's new index; a legacy forward, or any
// other cell, is returned as is.
func (e *Engine) normalizeInPlaceForward(fwdIdx int, site NormSite) int {
	fwd, err := AsForward(e.Tape.At(fwdIdx))
	if err != nil || !fwd.InPlace {
		return fwdIdx
	}
	funcIdx := fwd.FuncIndex
	for _, i := range e.inPlaceOperands(fwdIdx, e.Tape.Len(), fwd.CollectedArgs) {
		v := e.Tape.At(i)
		e.Tape.Remove(i)
		e.Tape.Insert(funcIdx, v)
		funcIdx++
		fwdIdx++
	}
	fwd.InPlace = false
	fwd.FuncIndex = funcIdx
	e.Tape.Set(fwdIdx, NewForward(fwd))
	InPlaceStats.Normalized.Add(1)
	InPlaceStats.NormalizedAt[site].Add(1)
	return fwdIdx
}

// normalizeForwardsIn normalises every in-place forward in [lo, hi). The
// moves stay inside the range, so hi still bounds it afterwards.
func (e *Engine) normalizeForwardsIn(lo, hi int) {
	if !e.Tape.hasForward() {
		return
	}
	for i := lo; i < hi; i++ {
		if IsForward(e.Tape.At(i)) {
			i = e.normalizeInPlaceForward(i, NormLoopRegion)
		}
	}
}

// excludeInPlaceClaims adds to set the cells the pending in-place forward at
// fwdIdx has claimed — its word, its collected operands after the marker
// (below limit) and its value-stack operands below the word — the in-place
// twin of the exclusions EffectiveResolved and curryOrStack compute from the
// legacy layout.
func (e *Engine) excludeInPlaceClaims(fwdIdx, limit int, fwd ForwardInfo, set map[int]bool) {
	set[fwd.FuncIndex] = true
	for _, i := range e.inPlaceOperands(fwdIdx, limit, fwd.CollectedArgs) {
		set[i] = true
	}
	for _, i := range e.claimedStack(fwd.FuncIndex, fwd.StackArgs) {
		set[i] = true
	}
}

// compactNops drops the nops between the innermost open paren and the
// pointer, so a statement stream that piles them up does not keep them below
// every backward scan. It declines while a call cell stands at the pointer
// (its span is read by index when it executes — the next attempt follows
// it) and while a forward is pending in that range (its FuncIndex would
// move; the next attempt then waits for more nops). The step loop never
// calls it while a value callee's one-shot seal is armed (the seal holds an
// index).
func (e *Engine) compactNops() {
	if e.Pointer < e.Tape.Len() && IsCall(e.Tape.At(e.Pointer)) {
		return
	}
	e.nopPending = 0
	lo := 0
	for i := e.Pointer - 1; i >= 0; i-- {
		v := e.Tape.At(i)
		if IsOpenParen(v) {
			lo = i + 1
			break
		}
		if IsForward(v) {
			return
		}
	}
	n := e.Tape.DropNopsIn(lo, e.Pointer)
	e.Pointer -= n
	InPlaceStats.Compactions.Add(1)
	InPlaceStats.NopsDropped.Add(int64(n))
}

// compactGroup drops the nops inside a closing paren group (openIdx,
// closeIdx) before its collapse reads the scope — a frame's return check
// counts values, the void-group record asks whether anything is left — and
// returns the close paren's new index (the pointer follows it).
func (e *Engine) compactGroup(openIdx, closeIdx int) int {
	if !e.sawNop {
		return closeIdx
	}
	n := e.Tape.DropNopsIn(openIdx+1, closeIdx)
	if e.Pointer == closeIdx {
		e.Pointer -= n
	}
	e.droppedNops(n)
	return closeIdx - n
}

// dropAllNops removes every nop from the tape — the run's end, before its
// residual is handed out.
func (e *Engine) dropAllNops() {
	if e.sawNop {
		InPlaceStats.NopsDropped.Add(int64(e.Tape.DropNopsIn(0, e.Tape.Len())))
	}
}

// diagPrefix is the stack prefix a failed dispatch's diagnostic reads. Under
// a pending in-place forward the cells between its marker and the pointer
// are its collected operands, which the legacy layout keeps before the word —
// so the legacy prefix ends at the marker, and the view a diagnostic takes of
// it is empty. Returning nothing here keeps the two renderings identical.
func (e *Engine) diagPrefix() []Value {
	if fi := e.pendingForwardIdx(); fi >= 0 {
		if f, err := AsForward(e.Tape.At(fi)); err == nil && f.InPlace {
			return nil
		}
	}
	return e.Tape.Prefix(e.Pointer)
}

// armVerify (verify mode) records the signature a legacy completion's plan
// chose, for the re-step of the word at funcIdx to compare against, and
// whether that signature still held over the rearranged operands (n of them,
// top first below the word) — the case in-place completion keeps.
func (e *Engine) armVerify(sig *Signature, funcIdx, n int, commit bool) {
	if e.Registry == nil || e.Registry.InPlace != InPlaceVerify || e.recorder != nil || e.Registry.analysisActive() {
		return
	}
	idx := resolvedIndicesBeforeInto(e.Tape, funcIdx, nil, n)
	for i, j := 0, len(idx)-1; i < j; i, j = i+1, j-1 {
		idx[i], idx[j] = idx[j], idx[i]
	}
	e.verifySig, e.verifyIdx, e.verifyCommit = sig, funcIdx, commit
	e.verifyHeld = len(idx) == n && e.planHolds(sig, idx, nil)
}

// checkVerify (verify mode) compares a re-step's re-plan with the armed plan.
func (e *Engine) checkVerify(name string, sig *Signature) {
	if e.verifySig == nil {
		return
	}
	planned := e.verifySig
	e.verifySig = nil
	if e.Pointer != e.verifyIdx {
		return
	}
	if sig == planned {
		InPlaceStats.Verified.Add(1)
		return
	}
	InPlaceStats.Disagreed.Add(1)
	d := InPlaceDisagreement{Name: name, Pos: e.currentPos(), Planned: traceSigStr(name, planned), Commit: e.verifyCommit, PlanHeld: e.verifyHeld}
	if sig != nil {
		d.Replanned = traceSigStr(name, sig)
	}
	inPlaceFindingsMu.Lock()
	inPlaceFindings = append(inPlaceFindings, d)
	inPlaceFindingsMu.Unlock()
}
