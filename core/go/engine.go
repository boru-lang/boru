package core

import (
	"fmt"
	"os"
	"strings"
)

// strictForwardBarrier is the forward-collection rule and the language
// DEFAULT: every bare function word beginning its own dispatch is a
// forward-collection barrier — uniformly, regardless of arity. It is a
// fundamental design property, not a mode. When the nearest parked forward
// cannot commit with the args it already holds (commitBarrierForward's
// exact-arity probe fails), the stranded word raises a signature error
// rather than letting the fn word's result arrive into the open slot — so
// `print add 1 2` is an error and `print (add 1 2)` is the spelling.
// `context eq context` strands the same way (a nullary is still a function
// dispatch): group it as `(context) eq (context)`. The SOLE exemption is
// structural, not arity-based: a dot-access chain (`m.a`, `MathUtil.now`)
// is implicitly-parenthesized navigation, so its `dot` dispatch is grouped
// and the outer word sees one value. BORU_NO_STRICT_BARRIER=1 restores the
// legacy wait-through behaviour (transitional; slated for removal). See
// design/STRICT-FORWARD-BARRIER.0.md.
var strictForwardBarrier = os.Getenv("BORU_NO_STRICT_BARRIER") == ""

// StackHeadroom is the extra capacity allocated beyond current need,
// so that most insert/splice operations avoid heap allocation.
const StackHeadroom = 8

// TraceCallback is called before each step of execution when tracing is enabled.
// It receives the step number, pointer position, full stack, and an annotation
// describing what happened on the previous step.
type TraceCallback func(step int, pointer int, stack []Value, note string)

// Engine is the boru stack machine.
//
// Execution model: `tape` is a rewriting tape, not a LIFO stack. The
// input program is loaded onto it whole; `pointer` then walks forward
// and the value under the pointer dispatches (word → run, forward →
// advance, open-paren → group, ...). A dispatched word may splice
// values into the tape in place — consuming neighbours, inserting
// results. Execution ends when the pointer walks off the end; the
// residual tape is the result.
//
// The tape is a gap buffer (eng/go/tape.go): edits land at or near the
// pointer, and the gap riding the edit sites makes them O(edit size)
// instead of O(tail length) — the fix for the O(depth²) recursion cost
// measured in design/RECURSION-PERFORMANCE.10.md and benchmarked in
// design/TAPE-DATA-STRUCTURE.10.md.
type Engine struct {
	Tape      *Tape
	Pointer   int
	Registry  *Registry
	trace     TraceCallback
	traceNote string   // annotation set during execution for the next trace call
	recorder  Recorder // optional StackForm recorder; see stackform package
	// sawFnFrame latches once this engine splices a fn frame, so inFnFrame can
	// skip its prefix scan entirely for a program that has none. Monotonic by
	// design — see inFnFrame.
	sawFnFrame bool
	stepLimit  int             // hard cap on the Run loop; always positive, set by the New/NewTop constructors below
	marks      map[string]bool // active mark IDs (for mark/move control flow)
	// sealFnValue / sealFnValueIdx: one-shot commit seal for a VALUE-called
	// function whose forward collection just COMPLETED. Completion re-steps
	// the callee stack-only; a WORD callee gets that via the /s token
	// rewrite (forceStackWord), but a first-class Function value has no
	// WordInfo to stamp — before this seal its re-step re-planned
	// forward-first, so a completed Any window stayed open and reached
	// across the statement boundary, swallowing the next statement's
	// tokens (NUR038: `IO.printstr "A" IO.printstr "B"` printed B, then A,
	// then stranded a fn value — silently). The completion site arms the
	// seal for the callee's index; execFnDefLiteral consumes it on the
	// immediately-following step and matches stack-only, exactly like the
	// word twin's /s retry.
	sealFnValue    bool
	sealFnValueIdx int
	// debugLabel names the CALL this engine's run realises, when the
	// dispatch knows it (CallBoruNamed: a module fn body run in its own
	// sub-engine, whose Defs-based frame leaves no tape marks). A debug
	// host reads it back through EngineState.Label so a backtrace can
	// name the module fn. Empty for every other engine.
	debugLabel string
	// rrValues / rrReordered are reusable scratch buffers for
	// rearrangeForForward's two per-call []Value allocations (forward
	// collection is on the interpreter's hot path — see
	// design/INTERPRETER-SPEED-PLAN.10.md #3). Both are purely local to
	// one rearrangeForForward call (extracted from the tape, permuted,
	// written straight back via tape.Set — never retained past the call),
	// and rearrangeForForward triggers no nested dispatch, so a single
	// engine-owned buffer is race- and aliasing-safe. Retained across
	// pooled sub-engine reuse (harmless: reset to [:0] on entry).
	rrValues    []Value
	rrReordered []Value
	// loopTokens is a reusable scratch buffer for stepMoveCont's per-
	// iteration `mark + body + move` re-splice (for/each-style loops run
	// their body in place on this tape — design/INTERPRETER-SPEED-PLAN.10.md
	// #4). Tape.Splice COPIES the tokens in, so the buffer is free the
	// moment Splice returns; a nested loop's stepMoveCont builds+splices
	// atomically before control returns, so a single engine-owned buffer is
	// reentrancy-safe.
	loopTokens []Value
	// peScratch is the reusable span buffer for expandParenExprScratch:
	// a ParenExpr expands to `( items… )` immediately before a
	// Tape.Splice (which copies the tokens in), so the buffer is free the
	// moment the splice returns — same lifetime argument as loopTokens.
	// The runPooledSub expansion sites keep the allocating variant (a
	// sub-engine run intervenes there).
	peScratch []Value
	// resolvedScratch / excludeScratch are reusable buffers for
	// effectiveResolved's two per-dispatch allocations — the resolved-stack
	// snapshot and the forward-exclusion set — which forward-collecting
	// dispatch pays on every hot-loop step (design/INTERPRETER-SPEED-PLAN.10.md
	// #3). effectiveResolved runs a single backward tape scan with NO nested
	// dispatch, and its returned slice is consumed only by matchSignature —
	// which merely READS it by index and never retains it — before any
	// further dispatch runs. So one engine-owned buffer per engine is
	// aliasing- and reentrancy-safe by the same argument as rrValues /
	// loopTokens above. Both are reset at the top of effectiveResolved;
	// excludeScratch is never returned (fully consumed inside the scan).
	resolvedScratch []Value
	excludeScratch  map[int]bool
	source          string // original source text for error reporting
	IsTop           bool   // true for engines created via NewTop; an unhandled FlowCtrl at end-of-Run is an error here, propagates upward otherwise
	// elemEvalRecordable marks a SUB-engine spawned by autoEvalList/autoEvalMap
	// to evaluate CONTAINER ELEMENTS of a recordable container eval (top-level or
	// consumed): a map/list literal ELEMENT inside it is residual-evaluated
	// (consumed=false) yet its assembly is anchored by the enclosing recordable
	// container, so its OpMakeMap/OpMakeList may record — this is what lets a
	// nested spec literal (`def specs [ {…cases:[{…in:[(make …)]}…]…} … ]`, the
	// voxgig bloom class-instance spec list) resolve level by level instead of
	// breaking at the first map-in-list layer. The fn-body deferred-residual
	// hazard (a frame that already popped) cannot occur here: the enclosing
	// container eval runs in-frame at its own recordable site.
	ElemEvalRecordable bool
	ReuseTape          bool // when set, Run reloads the existing tape in place instead of allocating (the VM's reusable island engine)
	// flowUnwind marks a VM ISLAND engine: a break/continue that escapes the
	// island's tape (no enclosing loop there) must TEAR DOWN the island's live
	// spliced frames before returning — in the interpreter the frame and the
	// enclosing loop always share one tape (handleLoopBreak's unwindLiveFrames
	// runs at the loop), but the island boundary separates them, so the frame
	// cleanup (args pop, def truncation, capture teardown) would otherwise be
	// lost with the discarded tape. exitWithFlowCtrl then returns NO values —
	// the VM discards the signalled iteration's partials anyway (flowSignal) —
	// with the registry FlowCtrl flag left set for the VM to translate.
	//
	// The field is the ISLAND marker itself, and it has a second consequence
	// beyond flow — see isIsland, which Run consults for scoping. Anything
	// that opens a new island MUST set it, which is why there is one field
	// rather than one per consequence: a second island entry point that set
	// only the flow half would silently regress the scoping half.
	FlowUnwind bool
	// startAt is a one-shot start offset for the next Run: the leading
	// startAt input values are RESOLVED arguments (a callback's inputs, a
	// fn call's unnamed args) and enter as stack data below the pointer,
	// never re-stepped — the sub-engine twin of FrameOpenInfo.ArgSpan
	// (arguments are inert; design/ARG-SEMANTICS-UNIFICATION.0.md).
	// Consumed (zeroed) by Run so it cannot leak into a later reuse.
	StartAt int
	// InertPrefix is a one-shot declaration for the next Run: its leading
	// InertPrefix tape values are call-site-resolved ARGUMENTS laid out
	// before the body (a fn-body analysis pushes the unnamed params this way,
	// check AnalyseFnBody), inert by the arguments-are-inert rule even though
	// the pointer starts at 0. Consumed (zeroed) by Run into inertPrefix.
	InertPrefix int
	// inertPrefix is the tape index just past the resolved-argument prefix
	// of the CURRENT run (StartAt as consumed by Run, or InertPrefix): the
	// values below it are call-site-resolved arguments the pointer never
	// steps as calls, so a Function value there can never collect a
	// neighbour. The collection-hazard scan (noteCollectionHazards, NUR121)
	// stops at it.
	inertPrefix int
	// voidGroups records the candidate consumers of paren groups that
	// resolved to ZERO values in the current statement: the pending
	// word names sitting below such a group when it closed. A
	// following signature failure on one of those words is reported
	// as "argument expression produced no value" at the causing site
	// rather than as a generic mismatch — the blame-shift fix of
	// design/ERRORS.8.md §3 (VOXGIG B3). Cleared at every statement
	// boundary (stepEnd).
	voidGroups []string
	// parenEvalDepth counts the forward-paren-group evaluations
	// (evalParenGroupAt) currently open on THIS engine's call stack. It
	// gates tail-call elimination: evalParenGroupAt drives its own loop
	// with a local paren-`depth` counter to find the group's matching
	// `)`, and a TCO frame-region rewrite (full replacement / shell
	// elision splices the tape underneath that loop) desynchronises the
	// counter from the tape — the group's close paren is deleted or
	// shifted before the loop decrements on it, so the group never
	// collapses and its result is never bound (`def x (f n)` for a
	// tail-recursive `f` errored `undefined_word: x`). While this is
	// >0 the tail call simply NESTS (tcoEligible declines) — correctness
	// never depends on TCO firing (design/TCO-STAGED.10.md), and the
	// nested frame is exactly what evalParenGroupAt's depth counter is
	// built to track. Incremented/decremented in lockstep by
	// evalParenGroupAt; sub-engine runs get their own zeroed counter.
	parenEvalDepth int
}

// RecorderSkipper is an optional extension to Recorder. When a
// Recorder also implements RecorderSkipper, the engine can tell it
// to suppress the next N OnPushLit events — used by stepCloseParen
// (and other rewind paths) where the engine's main loop re-visits
// stack values that have already been emitted to the recorder via
// their original push event.
type RecorderSkipper interface {
	Skip(n int)
}

// Recorder receives events as the Engine executes a program. Used by
// the lang/go/stackform package to build a canonical strict-stack
// representation of a program (see design/PBT-PLAN.10.md and
// design/boru-bytecode-report.0.md). Nil by default; install via
// Engine.SetRecorder.
//
// Recorder is called at the two semantic actions that define a
// strict-stack form:
//
//   - OnPushLit fires when a source literal gets pushed onto the
//     stack at the current pointer. Handler return values that the
//     engine re-encounters at the pointer also trigger this — the
//     recorder is responsible for distinguishing source-literals
//     from handler-results via the `returns` count delivered by
//     OnCall (the next `returns` OnPushLit events are handler
//     outputs, not source data).
//   - OnCall fires AFTER a successful matchSignature dispatch and
//     handler invocation. `arity` is len(matched-sig.Args); `returns`
//     is the number of values the handler actually produced; `name`
//     is the word the caller invoked. Recorders typically suppress
//     the next `returns` OnPushLit events.
//
// Recorders that need to capture nested sub-programs (e.g. quoted
// list bodies passed as NoEvalArgs) install themselves on the
// sub-engine that runs them.
type Recorder interface {
	OnPushLit(v Value)
	OnCall(name string, arity, returns int)
}

// SetRecorder installs a Recorder on this engine. Pass nil to clear.
// Recorder events fire during Run(); see the Recorder interface
// for the semantics of each callback.
func (e *Engine) SetRecorder(r Recorder) { e.recorder = r }

// SetTrace installs a TraceCallback on this engine. Pass nil to clear.
// The callback fires once before every step of Run() with the step
// index, the pointer position, a snapshot of the tape, and the
// pending trace note. This is the seam debug tooling builds on to
// count steps, profile per-word cost, or drive interactive stepping
// (see design/DEBUG-MODULE.0.md §6.1); the existing RunTrace uses the
// same field internally. The cost is paid only when a callback is
// installed — a normal Run leaves e.trace nil and never snapshots.
func (e *Engine) SetTrace(t TraceCallback) { e.trace = t }

// Default step limits for the Run loop. Exposed as named constants so
// every Engine constructor names them explicitly — there is no
// "zero means default" sentinel on `stepLimit`; the field is always
// set to a positive value by the constructors below.
//
// These are runaway-program guards (a genuinely non-terminating program
// must fail rather than hang forever), NOT a budget real programs are
// expected to brush against. The old values (22222 / 2222) were tuned
// for the pre-gap-buffer engine, where deep work was quadratic and so
// self-limiting; they capped a plain `for` loop at ~3700 iterations.
// With the gap-buffer tape (design/TAPE-DATA-STRUCTURE.10.md) loops and
// tail recursion are linear, so the cap is raised to a generous ceiling.
//
// Calibration (measured): a `for` loop costs ~6 top-level steps per
// iteration, tail recursion ~24 per level; data-bulk words (each/sort/
// map ops) do their work inside handlers and cost O(1) top-level steps.
// 10M steps therefore admits ~1.6M loop iterations / ~400k recursion
// levels — far beyond any real workload — while a genuine infinite loop
// trips in a few seconds and within tens of MB of tape growth (a higher
// ceiling like 100M lets a runaway run ~100s and balloon to ~600MB
// before tripping, which is worse than the cure). When the cap IS hit it
// now raises an explicit `evaluation_limit` error (see the Run loop and
// evalParenGroupAt), never the old phantom "unmatched opening
// parenthesis". Callers that genuinely need more raise the budget by
// setting Registry.StepLimit — from a host via lang.Options.Steps, or on
// the CLI via `--options steps:N`. Unset (zero) keeps the defaults below.
const (
	DefaultStepLimit    = 10_000_000 // top-level engine cap
	DefaultSubStepLimit = 10_000_000 // sub-engine cap (autoEvalMap, CallBoru, etc.)
)

// stepLimitFor resolves the effective step budget: the registry's
// StepLimit when a host configured one, else the supplied default. This is
// the single resolution boundary — every engine constructor and the VM
// entry point go through it, so no consumer sees an unresolved zero.
func StepLimitFor(r *Registry, def int) int {
	if r != nil && r.StepLimit > 0 {
		return r.StepLimit
	}
	return def
}

// New creates an Engine with the given function registry.
// The returned engine uses the sub-engine step limit.
// Use NewTop for the top-level engine with a higher limit.
func New(registry *Registry) *Engine {
	e := &Engine{Registry: registry, stepLimit: StepLimitFor(registry, DefaultSubStepLimit)}
	if registry != nil {
		e.trace = registry.effectiveDebugTrace()
	}
	return e
}

// NewTop creates a top-level Engine with the maximum step limit.
// isTop is set so an unhandled FlowCtrl signal at end-of-Run is reported
// as an error rather than propagating outward.
func NewTop(registry *Registry) *Engine {
	e := &Engine{Registry: registry, stepLimit: StepLimitFor(registry, DefaultStepLimit), IsTop: true}
	if registry != nil {
		e.trace = registry.effectiveDebugTrace()
	}
	return e
}

// SetSource sets the original source text for error reporting.
// When set, BoruErrors include source extracts showing the error location.
func (e *Engine) SetSource(src string) {
	e.source = src
}

// faultReturn fires the trace one final time — with a "fault: <err>"
// note and step -1 — before Run surfaces err, so a debug host can pause
// AT the raise with the tape and pointer still live (pause-before-
// unwind, design/BORU-DEBUGGER.0.md §6.1). Every Run-loop error return
// routes through it; a nil trace makes it a pass-through, so the
// non-debug error path is unchanged.
func (e *Engine) faultReturn(err error) error {
	if e.trace != nil {
		e.trace(-1, e.Pointer, e.Tape.Snapshot(), "fault: "+err.Error())
	}
	return err
}

// effectiveSource returns the source text for error reporting.
// Prefers the engine's own source; falls back to the registry's.
func (e *Engine) effectiveSource() string {
	if e.source != "" {
		return e.source
	}
	return e.Registry.Source
}

// reorderHint reports a "did you swap the arguments?" hint when the
// values the failed dispatch saw match one of fn's signatures under a
// non-identity PERMUTATION (decision DX report finding 2): the caller
// of `with-policy policy:String table:Map` who writes
// `with-policy table "collect"` gets the declared order spelled out
// instead of the forward-grouping hint, which points at parsing.
// Conservative: plain value sigs only (no quote/raw/form/type capture,
// no patterns), 2–4 args, first matching signature wins. Returns ""
// when no reorder explains the failure.
func (e *Engine) reorderHint(name string, fn *FnDefInfo) string {
	return ReorderHintFor(name, fn, reorderCandidates(e.Tape.Prefix(e.Pointer)))
}

// reorderCandidates collects up to 4 plain values from the top of the
// stack (walking down, stopping at engine markers / words) — the tuple
// a failed STACK dispatch saw, in the assignment order matchSignature
// uses (top-first: sig[i] ↔ vals[i]).
func reorderCandidates(stack []Value) []Value {
	var vals []Value
	for i := len(stack) - 1; i >= 0 && len(vals) < 4; i-- {
		v := stack[i]
		if IsOpenParen(v) || IsForward(v) || IsWord(v) || IsEnd(v) ||
			v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) {
			break
		}
		vals = append(vals, v)
	}
	return vals
}

// reorderForwardCandidates collects up to 4 UNCLAIMED forward value
// tokens after the word — concrete literals only; words, parens, and
// markers stop the scan. The result is in SOURCE order, which is the
// assignment order the forward plan would have used (sig[i] ↔
// token[i]) — exactly the failing-tuple view reorderHintFor wants.
func reorderForwardCandidates(tape *Tape, pointer int) []Value {
	var written []Value
	for i := pointer + 1; i < tape.Len() && len(written) < 4; i++ {
		v := tape.At(i)
		// An engine marker ends the written tuple exactly as it ends the
		// stack one below: a fn frame's tail markers (the DefCleanup `__dc`,
		// the pop-args `__pa`) sit right after the body's last token, and a
		// no-match there used to list `__dc (a __DC)` as the argument the
		// caller supplied — a marker no one wrote, in a user-facing note.
		if !IsConcrete(v) || IsWord(v) || IsParenExpr(v) || IsForward(v) ||
			IsOpenParen(v) || IsEnd(v) || isEngineMarker(v) {
			break
		}
		written = append(written, v)
	}
	return written
}

// isEngineMarker reports whether v is one of the engine's own control values
// — a Mark, a Move, or an internal marker (`Word/__IN/…`: DefCleanup,
// pop-args, the return check) — which never stand for an argument a user
// wrote. Shared by the two failing-tuple walks.
func isEngineMarker(v Value) bool {
	return v.Parent != nil && (v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) || v.Parent.ConformsTo(TInternal))
}

// rematchWritten is the CHECK-TIME twin of sigError's written-tuple
// derivation (unclaimed concrete forward tokens in source order, else the
// stack prefix top-first), widened to admit CARRIERS where the runtime
// tape holds the concrete value — the check pass mirrors the interpreter
// step-for-step, so position-for-position the runtime derivation walks
// the same shape and yields the concrete twins of these values. Used by
// the runtime-rematch record to prove its operand window IS the tuple the
// interpreter's error renders.
func (e *Engine) rematchWritten() []Value {
	var written []Value
	for i := e.Pointer + 1; i < e.Tape.Len() && len(written) < 4; i++ {
		v := e.Tape.At(i)
		if IsWord(v) || IsParenExpr(v) || IsForward(v) || IsOpenParen(v) || IsEnd(v) || isEngineMarker(v) {
			break
		}
		if !IsConcrete(v) && !v.Carrier {
			break
		}
		written = append(written, v)
	}
	if len(written) > 0 {
		return written
	}
	stack := e.Tape.Prefix(e.Pointer)
	for i := len(stack) - 1; i >= 0 && len(written) < 4; i-- {
		v := stack[i]
		if IsOpenParen(v) || IsForward(v) || IsWord(v) || IsEnd(v) ||
			v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) ||
			v.Parent.ConformsTo(TInternal) {
			break
		}
		written = append(written, v)
	}
	return written
}

// polyNoMatchProbe snapshots, at a FAILED dispatch's tape state, the pieces
// of sigError's diagnostic that live only on the tape — before the recovery
// path (checkModeAssumeSig's operand-resolution loop) mutates that state.
// The runtime interpreter raises sigError from exactly this state, so a poly
// recorded during the recovery can prove its no-match raise faithful against
// this snapshot (PolyNoMatchSpec, plan 3c).
type polyNoMatchProbe struct {
	// ok: the two TAPE-ONLY diagnostic layers a runtime rebuild has no access
	// to — the void-argument-group error and the fn-shape typed-binding hint
	// — do not apply at this state.
	ok bool
	// written / stackVals are sigError's two tape tuples at this state: the
	// WRITTEN tuple its notes render (rematchWritten — the carrier-aware twin
	// of the forward-else-stack derivation) and the SECONDARY reorder-probe
	// tuple (reorderCandidates over the stack prefix).
	written   []Value
	stackVals []Value
	// reach over-estimates how many operands ANY signature's collection could
	// claim at this state; reachOK marks the bound trustworthy. Used to prove
	// a WIDER-arity overload can never match at run time (it fails on operand
	// availability, not on drift-prone types).
	reach   int
	reachOK bool
	pos     SrcPos
}

func (e *Engine) PolyNoMatchProbe(name string, pos SrcPos) polyNoMatchProbe {
	p := polyNoMatchProbe{pos: pos}
	if e.voidArgErrorFor(name, pos) != nil || e.IsFnShapeTypedBindingContext() {
		return p
	}
	p.ok = true
	p.written = e.rematchWritten()
	p.stackVals = reorderCandidates(e.Tape.Prefix(e.Pointer))
	p.reach, p.reachOK = e.polyReachBound()
	return p
}

// polyReachBound over-estimates the operands a dispatch at the current
// pointer could collect: claimable forward tokens (concrete values, carriers,
// def-bound plain values, the reserved literals) until a structural stop (a
// function word, a paren boundary, a statement end), plus the stack-prefix
// values down to the nearest boundary. Tokens whose runtime contribution is
// unboundable — an unbound word, a splice, a marker inside the prefix — make
// the bound untrustworthy (reachOK=false), which declines every wider-arity
// exclusion that would rely on it. Over-estimating is sound (declines more);
// under-estimating is not.
func (e *Engine) polyReachBound() (int, bool) {
	n := 0
	for i := e.Pointer + 1; i < e.Tape.Len(); i++ {
		v := e.Tape.At(i)
		if IsOpenParen(v) || IsCloseParen(v) || IsEnd(v) {
			break // structural collection boundary
		}
		if polyReachUnboundable(v) {
			return 0, false
		}
		if IsWord(v) {
			wi, werr := AsWord(v)
			if werr != nil { //covergate:allow AsWord cannot fail after an IsWord guard — the payload IS a WordInfo (§engine)
				return 0, false
			}
			if top, ok := e.Registry.Defs.Top(wi.Name); ok {
				if _, isFn := top.Data.(FnDefInfo); isFn {
					if wi.ForceVal {
						// `/v`: the word denotes its REFERENCE value —
						// one claimable datum, not a call (NUR050/G12).
						n++
						continue
					}
					break // a function word stops forward collection
				}
				n++ // a plain value binding: the word steps to exactly one value
				continue
			}
			switch wi.Name {
			case "true", "false", "none":
				n++ // reserved literals resolve to one value
				continue
			}
			// Unbound word: undefined_word preempts the dispatch. (A
			// REGISTERED word never reaches here — registration pushes its
			// FnDefInfo binding, so Defs.Top caught it above.)
			return 0, false
		}
		if v.ReachGroup && !v.Quoted && isFnDefValue(v) && e.ReachFnWouldClaim(v, i+1) {
			break // a reach-collapsed named fn with a claim is the next CALL — a barrier (NUR038)
		}
		if IsConcrete(v) || v.Carrier {
			n++
			continue
		}
		return 0, false // anything else: unboundable
	}
	stack := e.Tape.Prefix(e.Pointer)
	for i := len(stack) - 1; i >= 0; i-- {
		v := stack[i]
		if IsOpenParen(v) || IsEnd(v) {
			break // dispatch never collects across a paren/statement boundary
		}
		if polyReachUnboundable(v) || IsWord(v) || !(IsConcrete(v) || v.Carrier) {
			return 0, false // word/marker in the prefix: unboundable
		}
		n++
	}
	return n, true
}

// polyReachUnboundable — tokens whose runtime contribution to a collection
// window has no static bound: an in-flight collection marker, a deferred
// expression that expands at step time (a reach, an unexpanded paren, a
// template string, a splice), or an engine marker. NOTE the IsConcrete trap:
// every one of these carries a payload, so the bare concrete probe counts
// them — screen them first (the reorderCandidates stop-set lesson).
func polyReachUnboundable(v Value) bool {
	return IsForward(v) || IsReach(v) || IsParenExpr(v) || IsInterpString(v) || IsSplice(v) || IsSugar(v) ||
		v.Parent.ConformsTo(TMark) || v.Parent.ConformsTo(TMove) || v.Parent.ConformsTo(TInternal)
}

// spec resolves the probe into a PolyNoMatchSpec against the recorded operand
// window (sig order, the callPoly layout), or nil when any faithfulness gate
// declines: a tape-only diagnostic layer applies, a NARROWER-arity overload
// exists (its runtime match would dispatch where the raise claims failure), a
// wider-arity overload is not structurally excluded, or a tape tuple is not
// exactly window values (the deeper-stack local-add lesson).
func (p polyNoMatchProbe) Spec(fn *FnDefInfo, window []Value) *PolyNoMatchSpec {
	if !p.ok || fn == nil || len(window) == 0 {
		return nil
	}
	arity := len(window)
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback || s.TotalArgs() == arity {
			continue
		}
		if s.TotalArgs() < arity || !p.reachOK || p.reach >= s.TotalArgs() {
			return nil
		}
	}
	written, ok := mapTupleToWindow(p.written, window)
	if !ok {
		return nil
	}
	stackTuple, ok := mapTupleToWindow(p.stackVals, window)
	if !ok {
		return nil
	}
	return &PolyNoMatchSpec{Written: written, StackTuple: stackTuple, NSigs: len(fn.Signatures), Pos: p.pos}
}

// mapTupleToWindow resolves each tape-tuple value to a distinct operand-window
// index by Value.ID (the RecordDispatchRematchValues identity gate). An empty
// ID or an unlocatable value declines — the runtime rebuild could not prove it
// re-renders the interpreter's tuple.
func mapTupleToWindow(tuple, window []Value) ([]int, bool) {
	idx := make([]int, len(tuple))
	used := make([]bool, len(window))
	for i, v := range tuple {
		if v.ID == "" {
			return nil, false
		}
		found := -1
		for j := range window {
			if !used[j] && window[j].ID == v.ID {
				found = j
				break
			}
		}
		if found < 0 {
			return nil, false
		}
		used[found] = true
		idx[i] = found
	}
	return idx, true
}

// reorderHintFor is the shared probe behind the signature errors.
// written is the failing tuple in the ASSIGNMENT order the dispatch
// used (sig[i] ↔ written[i]): source order for forward tokens,
// top-first for stack values.
func ReorderHintFor(name string, fn *FnDefInfo, written []Value) string {
	if fn == nil {
		return ""
	}
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		n := sig.TotalArgs()
		if sig.Fallback || n < 2 || n > 4 || n != len(written) {
			continue
		}
		if len(sig.QuoteArgs) > 0 || len(sig.RawParens) > 0 ||
			len(sig.FormArgs) > 0 || len(sig.TypeArgs) > 0 || sig.Patterns != nil {
			continue
		}
		// The identity assignment must FAIL (otherwise this sig would
		// have matched) and some permutation must succeed.
		if assignsPositionally(written, sig, true) || !assignsPositionally(written, sig, false) {
			continue
		}
		got := make([]string, n)
		want := make([]string, n)
		params := make([]string, n)
		for i := 0; i < n; i++ {
			got[i] = ValueType(written[i]).Leaf()
			t := SigArgType(sig, i)
			want[i] = t.Leaf()
			if i < len(sig.Params) && sig.Params[i].Name != "" {
				params[i] = sig.Params[i].Name + ":" + t.Leaf()
			} else {
				params[i] = t.Leaf()
			}
		}
		return "no signature matches (" + strings.Join(got, ", ") +
			"); one exists for (" + strings.Join(want, ", ") +
			") — did you swap the arguments? expected: " + name + " " +
			strings.Join(params, " ")
	}
	return ""
}

// assignsPositionally reports whether written (caller order, =
// reversed sig order) satisfies sig either positionally (identity =
// true) or under SOME assignment of distinct written args to sig
// slots (identity = false; backtracking over n ≤ 4 slots).
func assignsPositionally(written []Value, sig *Signature, identity bool) bool {
	n := sig.TotalArgs()
	if identity {
		for i := 0; i < n; i++ {
			if !SigTypeMatches(written[i], SigArgType(sig, i)) {
				return false
			}
		}
		return true
	}
	used := make([]bool, n)
	var try func(slot int) bool
	try = func(slot int) bool {
		if slot == n {
			return true
		}
		t := SigArgType(sig, slot)
		for j := 0; j < n; j++ {
			if used[j] || !SigTypeMatches(written[j], t) {
				continue
			}
			used[j] = true
			if try(slot + 1) {
				return true
			}
			used[j] = false
		}
		return false
	}
	return try(0)
}

// fnCourtesyDispatches mirrors fnFallbackSig's 0-arg courtesy-dispatch
// condition (registry.go): a fn whose active binding is an FnDefInfo and
// which declares a 0-arg non-fallback overload with a handler is
// dispatched (not errored) when the fallback sig is selected. Callers
// use it to decide whether a fallback selection is a real no-match
// (raise the rich sigError) or a courtesy dispatch (let fnFallbackSig
// run). Keep in lockstep with fnFallbackSig.
func fnCourtesyDispatches(r *Registry, name string, fn *FnDefInfo) bool {
	if r == nil || fn == nil {
		return false
	}
	top, ok := r.Defs.Top(name)
	if !ok {
		return false
	}
	if _, isFnDef := top.Data.(FnDefInfo); !isFnDef {
		return false
	}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() == 0 && !s.Fallback && s.DispatchHandler() != nil {
			return true
		}
	}
	return false
}

// sigError builds the full diagnostic for a signature mismatch: the
// shared name-only Detail, the received-arguments note, per-candidate
// verdicts, and the fix suggestions.
func (e *Engine) sigError(name string, fn *FnDefInfo, pos SrcPos) *BoruError {
	// A word starved by a VOID argument group (a parenthesised call in
	// its argument range that produced no value, recorded by
	// stepCloseParen) reports the causing expression, not the generic
	// mismatch (ERRORS.8.md §3).
	if verr := e.voidArgErrorFor(name, pos); verr != nil {
		return verr
	}
	// The failing tuple in assignment order: unclaimed forward tokens
	// (source order) when present, else the stack prefix (top-first) —
	// the same two views the swap probe reads.
	written := reorderForwardCandidates(e.Tape, e.Pointer)
	if len(written) == 0 {
		written = reorderCandidates(e.Tape.Prefix(e.Pointer))
	}
	// Reorder probe: when the actual argument types match some declared
	// signature under a PERMUTATION, the arguments are almost certainly
	// swapped — say so, with the declared parameter order, and suppress
	// the forward-grouping suggestion, which would point at parsing
	// (the wrong fix). Decision DX report finding 2.
	reorder := ReorderHintFor(name, fn, written)
	if reorder == "" {
		reorder = e.reorderHint(name, fn)
	}
	ae := e.noMatchError(name, fn, written, pos, reorder)
	return e.maybeAddFnShapeHint(ae).(*BoruError)
}

// noMatchError assembles the unmatched-dispatch diagnostic. It is a
// thin wrapper over the tape-free shared builder noMatchDiag
// (diag_msg.go) — the SAME builder the compiled VM's runtime guards
// call, so an interpreter and a compiled no-signature error are
// byte-identical over the same failing tuple.
func (e *Engine) noMatchError(name string, fn *FnDefInfo, written []Value, pos SrcPos, reorder string) *BoruError {
	return NoMatchDiag(e.effectiveSource(), name, fn, written, pos, reorder)
}

// IsFnShapeTypedBindingContext reports whether the failing word is
// positioned at the body slot of a typed binding whose constraint is
// a function-shape type (TFnUndef).
//
// Deferred forward-arg dispatch works by inserting a Forward marker
// after the func word and letting the engine evaluate upcoming tokens
// normally; the marker holds the matched signature plus arg-count
// bookkeeping. When a fn dispatch fails inside that deferred
// collection, walking back through the stack finds the Forward marker
// for the outer collector. If that collector is `def` and its
// typed-name map (sitting at FuncIndex - CollectedArgs, since
// stepLiteral splices each collected arg in immediately before the
// func word) carries a fn-shape constraint, the failing dispatch is
// exactly the §7.2 "user wrote a fn name where they meant
// `(quote name)`" case.
func (e *Engine) IsFnShapeTypedBindingContext() bool {
	if e.Registry == nil || e.Pointer == 0 {
		return false
	}
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			return false
		}
		if !IsForward(e.Tape.At(i)) {
			continue
		}
		fwd, _ := AsForward(e.Tape.At(i))
		if fwd.FuncName != "def" || fwd.Sig == nil {
			return false
		}
		// def's typed-name sig is the only one with TMap at position 0.
		if fwd.Sig.TotalArgs() < 2 || !SigArgType(fwd.Sig, 0).Equal(TMap) {
			return false
		}
		// stepLiteral moves each collected forward arg to the slot
		// immediately before the func word, in collection order
		// (first-collected = deepest). So position 0's value sits at
		// FuncIndex - CollectedArgs, position 1 at FuncIndex - CollectedArgs + 1, etc.
		if fwd.CollectedArgs < 1 {
			return false
		}
		mapIdx := fwd.FuncIndex - fwd.CollectedArgs
		if mapIdx < 0 || mapIdx >= e.Tape.Len() {
			return false
		}
		m, _ := AsMap(e.Tape.At(mapIdx))
		if m == nil || m.Len() != 1 {
			return false
		}
		constraint, _ := m.Get(m.Keys()[0])
		if IsWord(constraint) {
			cw, _ := AsWord(constraint)
			if tv, ok := e.Registry.ResolveTypedName(cw.Name); ok {
				constraint = tv
			}
		}
		return constraint.Parent.Equal(TFnUndef)
	}
	return false
}

// pendingForwardFunc returns the name of the nearest enclosing
// forward-collecting word (the function currently gathering arguments at
// the pointer), or "" if there is none before an open-paren barrier.
// Used to tailor error hints to the collecting context (e.g. a bare
// undefined word being collected as `def`'s body).
func (e *Engine) pendingForwardFunc() string {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			return ""
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			return fwd.FuncName
		}
	}
	return ""
}

// insufficientArgsError builds a detailed BoruError for forward argument
// collection failure (not enough arguments after the word).
func (e *Engine) insufficientArgsError(name string, expected int, pos SrcPos) *BoruError {
	src := e.effectiveSource()
	ae := makeBoruErrorAt("signature_error", insufficientArgsDetail(name, expected), name, src, "", pos)
	ae.Notes = append(ae.Notes, "stack: "+describeStackTypes(e.Tape, e.Pointer))
	return ae
}

// undefinedWordHint tailors the undefined_word hint to the two known
// causes worth pointing at:
//   - `def name foo` where foo is undefined: the user almost
//     certainly meant to bind foo's VALUE, not the bare word. `def`
//     does not auto-quote its body, so a bare word is dispatched (and
//     errors when undefined). Point at the fixes: `(foo)` to
//     evaluate, or `foo/q` to bind the name.
//   - The undefined name was pending next to a paren group that
//     produced NO value (`def r (returns-nothing …)` — the def
//     silently never bound, and this reference is the blame-shifted
//     victim). Name the real cause (ERRORS.8.md §3, VOXGIG B3).
func (e *Engine) undefinedWordHint(name string) string {
	if e.pendingForwardFunc() == "def" {
		return "did you mean `def … (" + name + ")` to bind its value, " +
			"or `def … " + name + "/q` to bind the name itself?"
	}
	for _, vg := range e.voidGroups {
		if vg == name {
			return "an earlier expression produced no value to bind to '" + name +
				"' — the def never happened; give that expression a return value"
		}
	}
	return ""
}

// undefinedWordError builds the runtime undefined_word diagnostic: the
// stable grep-friendly Detail (undefinedWordDetail), the context
// suggestion for the two known blame-shift shapes (undefinedWordHint),
// the did-you-mean near-miss over everything nameable in this registry,
// and the describe pointer when the nearest miss is a builtin word.
func (e *Engine) undefinedWordError(name string, pos SrcPos) *BoruError {
	ae := &BoruError{
		Code:       "undefined_word",
		Detail:     UndefinedWordDetail(name),
		Src:        name,
		Row:        pos.Row,
		Col:        pos.Col,
		FullSource: e.effectiveSource(),
	}
	if hint := e.undefinedWordHint(name); hint != "" {
		ae.Suggestions = append(ae.Suggestions, DiagSuggestion{Message: hint})
	}
	ae.Suggestions = append(ae.Suggestions, e.DidYouMeanSuggestions(name)...)
	return ae
}

// didYouMeanSuggestions builds the near-miss suggestion(s) for an
// unbound name: the did-you-mean line, plus the describe pointer when
// the nearest miss is a builtin word (so the fix and its documentation
// arrive together). Failure-path only — the candidate enumeration is
// never paid on a successful step.
func (e *Engine) DidYouMeanSuggestions(name string) []DiagSuggestion {
	matches := SuggestNames(name, e.Registry.SuggestionCandidates())
	if len(matches) == 0 {
		return nil
	}
	out := []DiagSuggestion{{Message: didYouMeanMessage(matches)}}
	if e.Registry.IsBuiltinWord(matches[0]) {
		out = append(out, DiagSuggestion{Message: describeSuggestion(matches[0])})
	}
	return out
}

// voidArgErrorFor reports the §3 "argument expression produced no
// value" error when the failing word was a candidate consumer of a
// paren group that resolved to ZERO values in the current statement;
// nil otherwise so the caller falls through to its generic error.
// `def` gets the tailored message (with the bound name read from the
// nearest collected atom below the pointer), since a void value
// expression is its classic blame-shift shape (VOXGIG B3:
// `def r (returns-nothing 1)`).
func (e *Engine) voidArgErrorFor(name string, pos SrcPos) *BoruError {
	matched := false
	for _, vg := range e.voidGroups {
		if vg == name {
			matched = true
			break
		}
	}
	if !matched {
		return nil
	}
	src := e.effectiveSource()
	if name == "def" {
		n := "<name>"
		for i := e.Pointer - 1; i >= 0; i-- {
			if IsOpenParen(e.Tape.At(i)) {
				break
			}
			if IsAtom(e.Tape.At(i)) {
				n, _ = AsAtom(e.Tape.At(i))
				break
			}
		}
		return makeBoruErrorAt("def_error",
			"def: expression produced no value to bind to '"+n+"'",
			"def", src,
			"hint: the called word returns nothing — call it without def, or give it a return value",
			pos)
	}
	return makeBoruErrorAt("no_value_error",
		"argument expression produced no value for "+name,
		name, src,
		"hint: a parenthesised argument evaluated to nothing — give it a return value, or drop it from the call",
		pos)
}

// stampResultPos stamps pos onto handler-produced values that lack a source
// position: ReturnCheck markers (so a return-type error built later knows the
// call site) and freshly-constructed Function values (so an anonymous
// `fn`/`afn` value carries its construction site for downstream errors). Only
// zero-Pos entries are touched, so values that already carry a position — a
// stored fn passed through, a literal — are left alone.
//
// pos arrives as the CALL-SITE TOKEN's own *SrcPos and is shared onto the
// stamped values rather than re-escaped per dispatch (positions are minted
// once at parse and never mutated — the documented sharing rule on
// Value.pos). A nil pos means the token carries no position: nothing to stamp.
func stampResultPos(vals []Value, pos *SrcPos) {
	if pos == nil || pos.Row == 0 {
		return
	}
	for i := range vals {
		if vals[i].Pos().Row != 0 {
			continue
		}
		switch {
		case IsReturnCheck(vals[i]):
			if rc, err := AsReturnCheck(vals[i]); err == nil && rc.Pos.Row == 0 {
				rc.Pos = *pos
				vals[i] = NewReturnCheck(rc)
			}
		case vals[i].Parent.Equal(TFunction):
			vals[i].pos = pos
		}
	}
}

// stampErrPos attaches the currently-dispatched word's position to a
// handler-produced BoruError that has none. A handler raises its error while
// the engine is executing a specific word (at the pointer), so that word's
// position is the genuine location of the failure — no text-search guess.
// Errors that already carry a position, and non-BoruError errors, are left
// untouched.
func (e *Engine) stampErrPos(err error) error {
	ae, ok := err.(*BoruError)
	if !ok {
		return err
	}
	if ae.Row == 0 {
		pos := e.currentPos()
		if pos.Row != 0 {
			ae.Row, ae.Col = pos.Row, pos.Col
			if pos.Src != "" {
				ae.Src = pos.Src
			}
			// The position comes from THIS engine's tokens, so this
			// engine's source is the text it points into — attach it
			// (with the originating file) so an error raised inside an
			// imported module renders with the module's excerpt and
			// filename rather than a bare position (decision DX report
			// finding 4).
			if ae.FullSource == "" {
				ae.FullSource = e.effectiveSource()
			}
		}
	}
	if ae.File == "" && ae.Row != 0 && e.Registry != nil {
		ae.File = e.Registry.BaseFile
	}
	return err
}

// returnCountError builds a detailed BoruError for wrong number of return
// values. The detail text is shared with the VM via returnCountErrorText;
// the declaration span is interpreter-side enrichment (phase 5).
// stripTapeAscriptions removes any dispatch ascription (`v as T`) from the
// tape values in [lo, hi) — the frame-return strip: an ascription is scoped
// to a dispatch WITHIN a body and must not ride out of the frame into the
// caller (design/OPEN-WORDS.1.md §9). Extracted from stepCloseParen so that
// hot path stays under the cyclomatic-complexity gate.
func (e *Engine) stripTapeAscriptions(lo, hi int) {
	for j := lo; j < hi; j++ {
		if e.Tape.At(j).AscribedType() != nil {
			e.Tape.Set(j, StripAscribed(e.Tape.At(j)))
		}
	}
}

// values are the residual values the `got` count was taken over, shown in
// the message (design/DIAGNOSTIC-VALUES.0.md) — the caller slices them
// so the rendered list
// and the count can never describe different things.
func (e *Engine) returnCountError(rc ReturnCheckInfo, expected, got int, values []Value) *BoruError {
	return BuildReturnCountError(e.effectiveSource(), rc.FuncName, expected, got, values, rc.Pos, rc.Decl)
}

// validateReturnTypes checks the top nret residual values (results[extra:])
// against a ReturnCheck's declared return types, returning the first mismatch
// as a BoruError (nil when all conform). Extracted from stepCloseParen so that
// hot path stays under the cyclomatic-complexity gate.
//
// Uses the membership predicate v.Is(exp) — the SAME question the parameter
// boundary asks (sigTypeMatches → v.Is) — so a type's Behavior governs both
// ends symmetrically: a predicate refine runs its predicate on the way out
// (subset semantics), a bare refine stays nominal (newtype), and
// builtins/objects are unchanged (v.Is ≡ v.Parent.ConformsTo on concrete
// values). See design/REFINE-NEWTYPE-VS-SUBSET.10.md.
func (e *Engine) validateReturnTypes(rc ReturnCheckInfo, results []Value, extra int) error {
	return validateReturnTypesIn(e.Registry, rc, results, extra, e.effectiveSource())
}

// validateReturnTypesIn is validateReturnTypes with the Engine's two
// dependencies passed explicitly — the registry (for the check-mode
// gradual exemption) and the source text (for error rendering). The
// split exists so the REGISTRY-side dispatch path (CallBoruNamed) can
// enforce the identical contract without an Engine in hand: NUR069's
// verdict is that a declared return means the same thing at every
// boundary, and the way to guarantee that is one implementation, not
// two that agree today.
func validateReturnTypesIn(reg *Registry, rc ReturnCheckInfo, results []Value, extra int, src string) error {
	for k, exp := range rc.Returns {
		got := results[extra+k]
		// A GRADUAL (dynamic) residual optimistically conforms to any declared
		// return in check mode — the checker genuinely does not know the type
		// (e.g. a value read off a shapeless carrier, `m.n` over an abstract
		// Map), so gating here would be a false positive. This mirrors the
		// parameter boundary, where a dynamic arg matches a concrete slot, and
		// stays sound because the compiled/interpreted RET re-checks the real
		// runtime value (Dynamic is never set on a concrete runtime value).
		// Without this, a fn dispatched through a path that surfaces a dynamic
		// body residual (module / mini-kind dispatch) wrongly fails its own
		// declared-return check while the identical body called directly passes.
		if got.Dynamic && reg != nil && reg.analysisActive() {
			continue
		}
		if !got.Is(exp) {
			return BuildReturnTypeError(src, rc.FuncName, k+1, exp, got, rc.Pos, rc.Decl)
		}
		// A declared return whose *Type degraded to Any carries its real
		// domain in the pattern — a union output (`def IS (Integer tor
		// String)`) has no lattice node to name, so `exp` is Any and admits
		// everything. Checking the pattern is what makes such a return a
		// contract rather than a comment.
		// `Type` is an alias of `Value`, so the pattern pointer IS a *Type
		// and renders as the declared union rather than as the useless
		// "expected Any" the degraded type would produce.
		if pat := rc.ReturnPattern(k); pat != nil {
			if _, ok := Unify(*pat, got); !ok {
				return BuildReturnTypeError(src, rc.FuncName, k+1, pat, got, rc.Pos, rc.Decl)
			}
		}
	}
	return nil
}

// returnTypeError builds a detailed BoruError for a return type mismatch. The
// detail/hint text is shared with the VM via returnTypeErrorText; the two
// secondary spans — where the offending value was produced, and where the
// return contract was declared — are interpreter-side enrichment (phase 5).
func (e *Engine) returnTypeError(rc ReturnCheckInfo, index int, expected *Type, got Value) *BoruError {
	return BuildReturnTypeError(e.effectiveSource(), rc.FuncName, index, expected, got, rc.Pos, rc.Decl)
}

// attachDeclSpan labels the return contract's declaration site as a
// secondary span. A zero declaration site attaches nothing — the
// no-guessed-locations rule; the declared type is already in the Detail.
func attachDeclSpan(ae *BoruError, decl DeclSite, label string) {
	if decl.Pos.Row <= 0 {
		return
	}
	ae.Spans = append(ae.Spans, DiagSpan{
		Pos:    decl.Pos,
		Label:  label,
		Source: decl.Source,
		File:   decl.File,
	})
}

// currentPos returns the source position of the value at the pointer — the
// token currently being processed — or the unknown SrcPos when the pointer
// is out of range. Engine-side error builders use it so an error is located
// at the token that triggered it.
func (e *Engine) currentPos() SrcPos {
	if e.Pointer >= 0 && e.Pointer < e.Tape.Len() {
		return e.Tape.At(e.Pointer).Pos()
	}
	return SrcPos{}
}

// syntaxError builds a detailed BoruError for a syntax error.
func (e *Engine) syntaxError(msg, token string) *BoruError {
	src := e.effectiveSource()
	return makeBoruErrorAt("syntax_error", msg, token, src, "", e.currentPos())
}

// runtimeError builds a detailed BoruError for a runtime error.
func (e *Engine) runtimeError(code, detail, word, hint string) *BoruError {
	src := e.effectiveSource()
	return makeBoruErrorAt(code, detail, word, src, hint, e.currentPos())
}

// evalLimitError reports that evaluation hit the step-count guard before
// the program finished — the explicit, honest diagnosis for a runaway
// (non-terminating or pathologically deep) program, replacing the
// phantom "unmatched opening parenthesis" the old silent break produced.
func (e *Engine) evalLimitError(limit int) *BoruError {
	return e.runtimeError("evaluation_limit",
		fmt.Sprintf("evaluation exceeded the step limit of %d — the program ran too long (an infinite loop or unbounded recursion?)", limit),
		"",
		"if this is a legitimately long computation, raise the limit with `--options steps:N` (or lang.Options.Steps); otherwise check for a loop or recursion that never terminates")
}

// tapeExhaustedError reports that the tape hit its growth ceiling — the
// loud failure for unbounded consumption (a runaway splicing onto the
// tape without bound). Distinct from evalLimitError, which is the
// step-count (CPU) guard; this is the memory guard.
func (e *Engine) tapeExhaustedError() *BoruError {
	return e.runtimeError("tape_exhausted",
		fmt.Sprintf("evaluation tape exhausted its growth ceiling of %d entries — the program consumed unbounded space (an infinite loop or unbounded recursion?)", e.Tape.MaxCap()),
		"",
		"raise the tape size via options (initial size / grow count / growth factor) for a legitimately large program; otherwise check for a loop or recursion that never terminates")
}

// tapeWarn forwards a tape capacity warning to the registry's error
// writer. Wired as the Tape's warn sink so 90/95/99% crossings surface
// without the tape importing io.
func (e *Engine) tapeWarn(msg string) {
	if e.Registry != nil && e.Registry.ErrOutput != nil {
		fmt.Fprintf(e.Registry.ErrOutput, "boru: warning: %s\n", msg)
	}
}

// traceSigStr formats a signature as "name(type, type) prec=N" for trace annotations.
func traceSigStr(name string, sig *Signature) string {
	args := make([]string, sig.TotalArgs())
	for i := range args {
		args[i] = SigArgType(sig, i).String()
	}
	return name + "(" + strings.Join(args, ", ") + ")"
}

// Run executes the input values through the stack machine and returns
// the residual stack — there is no single "result value". After the
// pointer walks off the end, end-of-input cleanup runs (resolve
// pending forwards, reject stray `(`, strip mark/move markers,
// auto-evaluate leftover lists/maps); whatever Values remain are
// returned. Callers decide the shape they want: take [0] for a single
// value, keep the slice for a list, or splice it back into a parent
// tape.
// resolveAtomReferents walks a loaded program and stamps each atom that has
// no referent yet with a snapshot of what its name is currently bound to,
// when such a binding exists. It recurses into list payloads so quoted
// atoms nested in code/data lists are covered. Atom identity is unaffected
// (the referent is metadata; see AtomPayload). Names with no current binding
// are left unresolved — that is the honest "bound only at runtime" case.
func resolveAtomReferents(r *Registry, vals []Value) {
	for i := range vals {
		switch data := vals[i].Data.(type) {
		case AtomPayload:
			if data.Referent != nil {
				continue
			}
			if bound, ok := r.Defs.Top(data.Name); ok {
				vals[i] = SetAtomReferent(vals[i], bound)
			}
		case ListPayload:
			// FlexListData is deliberately not handled: this walk runs
			// on loaded program tapes, and flex nodes exist only at
			// runtime — they can never appear here.
			resolveAtomReferents(r, data.Elems)
		}
	}
}

func (e *Engine) Run(input []Value) (result []Value, runErr error) {
	// Observability seam (interp_entry.go): report this tree-walk entry to an
	// armed frontier hook. One atomic load when unarmed.
	e.Registry.noteInterp("Engine.Run")
	// Count this interpreter activation against the registry so a compiled
	// RunProgram can see (at its entry) that an interpreter run is in flight.
	// Balanced on every exit path, including panic unwind.
	e.Registry.EnterInterpRun()
	defer e.Registry.ExitInterpRun()

	// Track this engine as the current one for on-demand stack
	// introspection (Debug.stack). Defer-balanced like EnterInterpRun.
	e.Registry.pushEngine(e)
	defer e.Registry.popEngine()

	// Last-resort panic guard at the top-level engine boundary. A bug in
	// any handler or in the step loop should surface to the user as a
	// clean boru error, never as a goroutine stack trace. Only the
	// outermost (NewTop) engine recovers; sub-engines let the panic
	// propagate so it unwinds to here with the original stack intact for
	// the debug detail. Errors returned normally are untouched.
	if e.IsTop {
		defer func() {
			if rec := recover(); rec != nil {
				result = nil
				runErr = makeBoruErrorAt("internal_error",
					fmt.Sprintf("internal engine error: %v", rec),
					"", e.effectiveSource(),
					"this is a bug in boru; please report it", e.currentPos())
			}
		}()
	}

	// Truth-over-symptom guard: once any tape edit hit the growth
	// ceiling (and was dropped to avoid an out-of-bounds write), every
	// later behaviour runs on a TRUNCATED tape — downstream errors are
	// phantoms of the drop (a starved ReturnCheck's "expected 1 return
	// value(s), got 0", the genre the recursion pins documented), and a
	// "successful" completion may have silently lost results. Whatever
	// this Run was about to report, the truthful diagnosis is
	// tape_exhausted.
	defer func() {
		if e.Tape != nil && e.Tape.Exhausted() {
			result = nil
			runErr = e.tapeExhaustedError()
		}
	}()

	// Push a scoped context Store whose prototype is the parent context.
	// This is THE context boundary: a nested body runs in a sub-engine, so
	// one push here is what makes `do` / `each` / a `case` clause / an
	// auto-evaluated list contain their `context set` writes. The VM's
	// enterBodyUnit is the compiled twin.
	//
	// An ISLAND is not a body, so it does not push (see isIsland). An island
	// CONTINUES an in-progress compiled expression on the interpreter — a
	// fn-value apply the emitter could not lower, a FALLBACK span — and the
	// interpreter reaching the same tokens inline pushes nothing, so pushing
	// here made the compiled path MORE isolated than the interpreter it is
	// supposed to mirror. Measured: `(m.f 1) drop` with a `context set` in
	// the map-slot lambda contained the write compiled (CALL_DYN_METHOD
	// islands the apply) and leaked it interpreted. A body reached from
	// INSIDE an island still gets its own frame, because that body opens its
	// own sub-engine. See design/verse-report-defects-investigation.0.md §B.
	if !e.isIsland() {
		parent := e.Registry.Contexts.Top()
		e.Registry.Contexts.Push(parent)
		defer e.Registry.Contexts.Pop()
	}

	// In static type-check mode, convert concrete literal values to
	// carriers before execution. The same dispatch/matching machinery
	// then runs over carrier values; execMatch short-circuits handler
	// calls to push carrier return values declared on the signature.
	if e.Registry.analysisActive() {
		es := e.Registry.analysisRecorder()
		es.BindRegistry(e.Registry) // back-pointer for returned-closure compilation
		pre := input
		input = e.Registry.analysisStripToCarriers(input)
		es.RememberStrippedOriginals(pre, input)
	}

	// Post-parse referent resolution: stamp each /q-style atom in the
	// program with a snapshot of what its name refers to, for any name
	// already bound when the program loads (pre-installed module/global
	// defs). Names bound only later, during execution, stay unresolved here —
	// the `quote` word captures those at quote time instead. Top engine only
	// (sub-engines run fragments already walked) and not in check mode (the
	// program has been stripped to carriers). Stamping happens on a private
	// copy of the input BEFORE the tape is built, so the caller's slice is
	// never mutated and the tape constructor owns a fully-prepared program.
	prog := input
	if e.IsTop && !e.Registry.analysisActive() {
		prog = make([]Value, len(input))
		copy(prog, input)
		resolveAtomReferents(e.Registry, prog)
	}

	// Load the program onto the gap-buffer tape with bounded growth
	// (NewTapeWith copies, so later splices never touch the caller's input
	// slice). The tape grows at most N times by factor M and then fails
	// loudly — see TapeConfig / the tape_exhausted check below. The
	// registry's TapeConfig (zero value = defaults) flows through
	// unchanged so resolve() applies the initial-size floor.
	// A reusable island engine reloads its tape in place (no per-island
	// allocation); a hot fallback island in a loop would otherwise spin
	// up a fresh tape every execution. Falls back to a fresh tape when
	// the existing buffer is too small. Per-run scratch state is cleared
	// so nothing leaks across reuses.
	if !(e.ReuseTape && e.Tape != nil && e.Tape.Reload(prog)) {
		e.Tape = NewTapeWith(prog, e.Registry.TapeConfig, e.tapeWarn)
	}
	// Per-run scratch state is cleared on EVERY entry (not only the
	// Reload branch) so a reused engine whose previous tape was too
	// small for this program — the fresh-tape fallback above — cannot
	// leak marks/void-group state from its prior run.
	e.marks = nil
	e.voidGroups = nil
	e.traceNote = ""
	// The one-shot completion seal is per-run scratch too: a prior run
	// that armed it on its very last allowed step (eval_limit before the
	// callee's re-step consumed it) must not force an unrelated value at
	// the same index of the NEXT program into stack-only mode (NUR038).
	e.sealFnValue = false
	e.Pointer = e.consumeStartAt()
	e.inertPrefix = e.Pointer
	if e.InertPrefix > e.inertPrefix {
		e.inertPrefix = e.InertPrefix
	}
	e.InertPrefix = 0

	// stepLimit is always set by the constructors (New / NewTop); the
	// defensive check that used to substitute a default if the field
	// was zero was load-bearing for callers that built Engine{}
	// directly, but no longer — the constructors are the only entry.
	limit := e.stepLimit
	completed := false
	for step := 0; step < limit; step++ {
		// Memory guard FIRST: a previous edit hit the tape's growth
		// ceiling (and was dropped to avoid an out-of-bounds write).
		// This must precede the completion check — a dropped FINAL
		// splice leaves the pointer past the truncated tape, which is
		// indistinguishable from normal completion and used to return
		// success with the results silently gone.
		if e.Tape.Exhausted() {
			return nil, e.tapeExhaustedError()
		}
		if e.Pointer >= e.Tape.Len() {
			completed = true
			break
		}

		// Check-mode global step budget (analysis_hooks.go): a
		// deliberate, already-reported stop — not the runtime
		// step-limit exhaustion below — so the drain proceeds
		// normally.
		if e.Registry.analysisStepMeter() {
			completed = true
			break
		}

		val := e.Tape.At(e.Pointer)

		// Line-coverage seam (coverage.go): record the executing token's source
		// row when a coverage run has armed the hook AND this registry is a
		// tagged coverage target. Inert (one atomic load) otherwise.
		e.Registry.noteCoverage(val.Pos())

		if e.trace != nil {
			snapshot := e.Tape.Snapshot()
			note := e.traceNote
			e.traceNote = ""
			e.trace(step, e.Pointer, snapshot, note)
		}

		switch {
		case IsWord(val):
			if err := e.stepWord(val); err != nil {
				return nil, e.faultReturn(err)
			}

		case IsForward(val):
			e.Pointer++

		case IsOpenParen(val):
			e.stepPastOpenParen(val)

		case IsCloseParen(val):
			if err := e.stepCloseParen(true); err != nil {
				return nil, e.faultReturn(err)
			}

		case IsEnd(val):
			if err := e.stepEnd(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				return nil, e.faultReturn(err)
			}

		case IsParenExpr(val):
			// A word-context ParenExpr (paren-nesting work, Step 2 —
			// design/PAREN-REPRESENTATION.9.md): expand it back to its
			// OpenParen … CloseParen marker span in place and let the
			// existing in-place collapse machinery evaluate it on THIS
			// engine. That keeps exact parity with the former marker
			// representation (recorder-transparent, same stack/registry
			// semantics) — the four contracts hold because markers already
			// honor them: errors propagate, defs leak, the OpenParen is a
			// stack barrier, and results flow out. Do not advance — the
			// OpenParen now sits at the pointer.
			//
			// Step 4: a codequote-captured ParenExpr (Quoted) is data, and a
			// raw-capture pending forward (pendingForwardWantsRawParen) wants
			// the paren collected as-is — both route through stepLiteral
			// (push data / collect the forward arg) rather than expanding.
			if val.Quoted || e.pendingForwardWantsRawParen() {
				if err := e.stepLiteral(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					return nil, e.faultReturn(err)
				}
			} else {
				items, _ := AsParenExpr(val)
				e.Tape.Splice(e.Pointer, 1, e.expandParenExprScratch(items)...)
			}

		case IsReach(val):
			// A parsed Reach (dot-access node, m.a.b — Eval=true) evaluates
			// by lowering to its get/getr chain in place, exactly like the
			// ParenExpr it replaced. An inert reach (Eval=false, from `reach`)
			// or a codequote'd one (Quoted) is data — left via stepLiteral.
			if isEvalReach(val) && !e.pendingForwardWantsRawParen() {
				info, _ := AsReach(val)
				// A dot-access chain (`m.a`, `MathUtil.now`) is a self-
				// delimiting navigation that produces exactly ONE value — it
				// reads as an implicit `(m.a)` group and feeds forward
				// collection like any other value, so it is NOT a barrier
				// under the strict rule. (The barrier exists for a function
				// word that forward-collects its OWN args — `print add 1 2`;
				// a Reach collects nothing further, its key is bound in the
				// chain.) Lower to its get-chain marker span in place.
				// design/STRICT-FORWARD-BARRIER.0.md.
				e.Tape.Splice(e.Pointer, 1, expandReach(info)...)
			} else {
				if err := e.stepLiteral(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					return nil, e.faultReturn(err)
				}
			}

		case IsInterpString(val):
			result, err := e.evalInterpString(val)
			if err != nil {
				return nil, e.faultReturn(err)
			}
			// Replace with the evaluated string but do NOT advance the
			// pointer. The resulting string value needs to go through
			// stepLiteral so forward collection works correctly.
			e.Tape.Set(e.Pointer, result)

		case IsXmlInterp(val):
			// Interpolated XML literal: evaluate the skeleton in place to
			// a concrete Node/Xml, then re-step it as the value (no pointer
			// advance) so forward collection sees a Node/Xml — mirrors the
			// IsInterpString case above.
			result, err := e.EvalXmlInterp(val)
			if err != nil {
				return nil, e.faultReturn(err)
			}
			e.Tape.Set(e.Pointer, result)

		case IsMark(val):
			e.stepMark(val)

		case IsMove(val):
			if err := e.stepMove(val); err != nil {
				return nil, e.faultReturn(err)
			}

		case IsReturnCheck(val):
			e.Pointer++

		case IsDefCleanup(val):
			if err := e.stepDefCleanup(val, e.Pointer); err != nil {
				return nil, e.faultReturn(err)
			}
			e.Pointer++

		default:
			if val.Parent == nil && val.Behavior() == nil {
				return nil, e.faultReturn(e.runtimeError("halt", fmt.Sprintf("undefined stack entry at position %d", e.Pointer), "", ""))
			}
			if err := e.stepLiteral(); err != nil {
				return nil, e.faultReturn(err)
			}
		}

		// Flow-control signal raised during the step (by a break/
		// continue handler or by a sub-engine sharing this registry).
		// Try to resolve locally; if no enclosing loop is on this
		// tape, leave the flag set and bail out of the loop so an
		// outer Run frame can catch it.
		if e.Registry.FlowCtrl != FlowNone {
			if e.handleFlowCtrl() {
				continue
			}
			return e.exitWithFlowCtrl()
		}
	}

	// If the loop exited naturally (pointer walked off the end) with a
	// signal still set, fall through to the same handler.
	if e.Registry.FlowCtrl != FlowNone { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		return e.exitWithFlowCtrl()
	}

	// The loop ran out of step budget before the program finished. Report
	// it explicitly: the program was cut off mid-evaluation, so the drain
	// below would otherwise see a half-processed tape (e.g. an open paren
	// the run never reached) and blame a phantom "unmatched opening
	// parenthesis". This is the honest diagnosis instead.
	if !completed {
		return nil, e.faultReturn(e.evalLimitError(limit))
	}

	// Implicit end-of-input: resolve any pending forwards from the stack.
	// No longer an unreachable arm: core/spec now reaches it. `bothq __pa
	// true` parks a forward whose operands never all arrive — a word
	// between the marker and its operands produced no residual — and the
	// program ends with the forward still pending, which is exactly what
	// this resolves and refuses. The pragma that stood here is gone with
	// the unreachability it claimed.
	if err := e.resolveOrphanedForwards(); err != nil {
		return nil, e.faultReturn(err)
	}

	for i := 0; i < e.Tape.Len(); i++ {
		if IsOpenParen(e.Tape.At(i)) {
			return nil, e.faultReturn(e.syntaxError("unmatched opening parenthesis", "("))
		}
	}

	// Remove any leftover marks and moves from the stack.
	e.cleanMarks()

	// Auto-evaluate unquoted lists and maps on the final stack.
	// Lists are evaluated as sub-programs: [1 add 2] → [3].
	// Maps have their values evaluated recursively.
	// Values marked Quoted (by the quote word) are left as-is.
	if err := e.autoEvalStack(); err != nil {
		return nil, e.faultReturn(err)
	}

	// Orphan GenSpec residue (generics plan D1/D2): a `gen [...]`
	// whose spec no constructor consumed leaves placeholder type
	// bindings behind — pop them and report the gen loudly. TOP-level
	// engines only: sub-engine Runs (paren groups, list/map arg
	// auto-evaluation) legitimately execute while a spec is pending
	// for the enclosing constructor — `refine Record [value:T]`
	// auto-evaluates its list arg in a sub-engine BETWEEN gen and the
	// refine handler.
	if e.IsTop {
		if spec := e.Registry.TakePendingGen(); spec != nil {
			PopGenBindings(e.Registry, spec)
			if !e.Registry.analysisActive() {
				return nil, makeBoruError("gen_without_constructor",
					"gen: parameter spec was not consumed by a type constructor",
					"gen", e.effectiveSource(),
					"hint: follow gen [...] with refine Record [...], class {...}, fnsig [...], or fn [...]")
			}
			// Check mode is lenient (no error), but the compiled stream
			// elides gen. Record a TERMINAL trap so a compiled program raises
			// the byte-identical gen_without_constructor error at this point
			// (orphan gen errors at end-of-run, exactly where this fires)
			// instead of refusing; if the trap can't be recorded (nested), keep
			// the blanket-refusal flag so the program falls back.
			if !e.Registry.analysisRecorder().RecordTrap("gen_without_constructor",
				"gen: parameter spec was not consumed by a type constructor", "gen",
				"hint: follow gen [...] with refine Record [...], class {...}, fnsig [...], or fn [...]",
				e.currentPos()) {
				e.Registry.noteSuppressedRuntimeError()
			}
		}
	}

	// (There is no uncalled-function RESIDUE pass here any more: a failed
	// fn-value dispatch is an error at the dispatch site, so nothing
	// reaches the drain to judge — design/FN-VALUE-DISPATCH.0.md.)

	// Drain any Undefined-Atom values left on the stack. Outside check
	// mode `stepWord` errors on undefined words so this loop is a
	// no-op. Under CheckMode `stepWord` already emitted the diagnostic
	// at the source token; here we only need to replace any dangling
	// Undefined atoms with `Any` carriers so the residual stack stays
	// type-clean for downstream consumers of CheckResult.Stack.
	CheckBraid.DrainUndefinedAtoms(e)

	// Judge the finished residual for the call that never happened: a
	// capitalised `def` given a fn body binds a TYPE, so the name in call
	// position places its node and strands the operands after it
	// (design/HIGHER-ORDER-FUNCTIONS.0.md §5.1). Offered the RECONCILED
	// list — what CheckResult.Stack reports — so a phantom None from a
	// trailing 0-output guard cannot read as a stranded operand.
	residual := e.reconcileTopResidual(e.Tape.TakeAll())
	CheckBraid.NoteStrandedTypeCall(e, residual)
	return residual, nil
}

// reconcileTopResidual reconciles the top-level program residual the same
// way fn-body summaries are (carrier.go stripZeroOutResiduals at
// StartFnCompile finish): a trailing 0-output statement guard — `if cond
// [printstr …] [printstr …]` — registers a phantom None carrier but
// produces 0 runtime values, so the residual must skip it. Recording-active
// only; the uncompilable case nets 0 at the source (if2/if3ReturnsFn).
func (e *Engine) reconcileTopResidual(out []Value) []Value {
	if e.IsTop && e.Registry.analysisActive() {
		return e.Registry.analysisZeroOutResiduals(out)
	}
	return out
}

// resolveOrphanedForwards handles end-of-input by resolving pending forwards.
func (e *Engine) resolveOrphanedForwards() error {
	for attempt := 0; attempt < 222; attempt++ {
		fwdIdx := -1
		for i := 0; i < e.Tape.Len(); i++ {
			if IsForward(e.Tape.At(i)) {
				fwdIdx = i
				break
			}
		}
		if fwdIdx < 0 {
			return nil
		}

		fwd, _ := AsForward(e.Tape.At(fwdIdx))
		funcIdx := fwd.FuncIndex
		collectedCount := fwd.CollectedArgs
		stackArgCount := fwd.StackArgs

		// Remove the forward marker.
		e.Tape.Remove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx--
		}

		// Try stack match or create curry list.
		e.curryOrStack(funcIdx, collectedCount, stackArgCount)

		// Retry from the current pointer position.
		for step := 0; step < 100; step++ {
			if e.Pointer >= e.Tape.Len() {
				break
			}
			val := e.Tape.At(e.Pointer)
			// Line-coverage seam (coverage.go): this forward-retry loop steps
			// tokens (a paren/forward re-evaluated after curryOrStack) outside
			// the main loop, so it mirrors the main loop's per-step emit.
			e.Registry.noteCoverage(val.Pos())
			switch {
			case IsWord(val):
				if err := e.stepWord(val); err != nil {
					return err
				}
			case IsCloseParen(val):
				if err := e.stepCloseParen(false); err != nil {
					return err
				}
			case IsEnd(val):
				if err := e.stepEnd(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					return err
				}
			case IsForward(val):
				e.Pointer++
			case IsOpenParen(val):
				e.Pointer++
			default:
				if err := e.stepLiteral(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					return err
				}
			}
			// Propagate any flow-control signal raised by the
			// step; the outer Run frame will resolve it.
			if e.Registry.FlowCtrl != FlowNone {
				return nil
			}
		}
	}
	return nil //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
}

// rawParenForward reports whether any of fn's signatures captures a forward
// ParenExpr RAW at sig position pos (RawParens[pos]). See
// design/PAREN-REPRESENTATION.9.md Step 4.
func rawParenForward(fn *FnDefInfo, pos int) bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].RawParens != nil && fn.Signatures[i].RawParens[pos] {
			return true
		}
	}
	return false
}

// rawFormForward reports whether any of fn's signatures captures the forward
// operand at sig position pos as a raw FORM (FormArgs[pos]) — the macro
// raw-capture mode. Like rawParenForward, it gates preEvalParens so a paren /
// reach at that position is left unevaluated. See design/MACROS-PHASE1.10.md §3.
func rawFormForward(fn *FnDefInfo, pos int) bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].FormArgs != nil && fn.Signatures[i].FormArgs[pos] {
			return true
		}
	}
	return false
}

// bindsReferent reports whether the named word is a BINDER whose operand
// slot copies a word's binding rather than its expansion. `def y xs` with
// xs splice-bound must rebind the marker itself — the new name ALIASES the
// splice — because expanding there would lose the referent (and code-bearing
// splices already alias on def by skipping expansion, so this keeps data and
// code splices consistent at binders). This is the one deliberate exception
// to the `f w ≡ f (w)` equivalence; write `def y (xs)` to force expansion.
// def is frozen (reserved_word), so the name is a reliable identity.
func bindsReferent(name string) bool {
	return name == "def"
}

// capturesForward reports whether any of fn's signatures captures the
// forward operand at sig position pos STRUCTURALLY — /q word-name capture
// (QuoteArgs), raw paren capture (RawParens), macro form capture (FormArgs),
// or type-literal capture (TypeArgs) — rather than as an ordinary value.
// Gates the splice-word paren expansion in resolveForwardArgs: capture slots
// take the token itself (the word's name, the raw form) regardless of any
// def binding, so the `f w ≡ f (w)` equivalence deliberately does not apply
// there (`quote vs` captures the NAME vs even when vs is splice-bound —
// word-splice spec §3).
// capturesForwardToken reports whether any of fn's signatures captures
// THIS token structurally at position pos. A /q slot carrying a
// concrete Atom PATTERN (a KEYWORD slot — it admits only that one
// literal word, e.g. def's `fn` form) captures only a token with that
// exact name; every other word keeps its barrier/expansion treatment
// at this position. Unpatterned /q slots and raw/form/type slots
// capture unconditionally. Without the token check, one keyword
// signature would flip the per-word capture gate and disable the
// function-word barrier at its position for EVERY statement of that
// word — the cross-barrier pre-evaluation bug class the scan's
// barrier stop exists to prevent.
func capturesForwardToken(fn *FnDefInfo, pos int, tok Value) bool {
	if fn == nil {
		return false
	}
	tokName := ""
	if w, err := AsWord(tok); err == nil {
		tokName = w.Name
	}
	for i := range fn.Signatures {
		sig := &fn.Signatures[i]
		if sig.QuoteArgs != nil && sig.QuoteArgs[pos] {
			if pat, ok := SigPattern(sig, pos); ok && IsAtom(pat) {
				if pn, err := AsAtom(pat); err == nil && pn == tokName {
					return true
				}
				continue
			}
			return true
		}
		if sigRawSlot(sig, pos) {
			return true
		}
	}
	return false
}

// pendingForwardWantsRawParen reports whether the nearest enclosing pending
// Forward (collecting args at the pointer) was matched to a signature that
// captures a ParenExpr raw. When true, a ParenExpr reached during forward
// collection is collected as data (via stepLiteral) rather than expanded.
// Stops at an OpenParen barrier. See Step 4.
func (e *Engine) pendingForwardWantsRawParen() bool {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			return false
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			// RawParens or FormArgs (macro raw capture) both want a forward
			// ParenExpr / Reach left raw at the pointer rather than evaluated
			// during collection.
			return fwd.Sig != nil && (len(fwd.Sig.RawParens) > 0 || len(fwd.Sig.FormArgs) > 0)
		}
	}
	return false
}

// resolveForwardArgs implements structure-first, lazy forward-argument
// resolution (design/LAZY-ARG-RESOLUTION.10.md). It replaces the former
// eager `preEvalParens(MaxForwardArgs)` scan, which evaluated EVERY forward
// paren group up to the highest-arity overload's needs before any signature
// was chosen — the cause of the `import "mod" (expr)` hazard (gotcha N1),
// where the trailing paren was evaluated before `import` installed the
// namespace it referenced.
//
// The scan is identical to the old one in every respect EXCEPT one: before
// evaluating a forward paren group, it checks whether any still-viable
// overload actually consumes a forward argument at that position. A
// signature is pruned from the viable set only when a concrete forward
// value (a literal, or a previously-evaluated group's result) DEFINITELY
// cannot satisfy a non-raw, non-Any slot the signature consumes — exactly
// the rejection `matchSignature` would make on the same value. So the
// signature `matchSignature` ultimately selects is never pruned, its
// claimed groups are always evaluated, and the only behavioural change is
// that a group NO surviving overload consumes is left raw (an OpenParen
// boundary that `matchSignature` stops at) rather than speculatively run.
//
// For `import "boru:string-util" (StringUtil.indexof ...)`: the leading
// String literal prunes the viable set to `[String]` (arity 1); at position
// 1 the paren is consumed by no viable overload, so it is left raw — import
// selects `[String]`, installs the namespace, and the paren then runs as an
// ordinary trailing statement.
// ViableSig pairs a forward-eligible signature with its effective
// barrier during resolveForwardArgs' pre-evaluation scan.
type ViableSig struct {
	Sig     *Signature
	Barrier int
}

// sigsHaveKeywordSlot reports whether any viable signature carries a
// KEYWORD slot — a /q position with a concrete Atom pattern, admitting
// exactly one literal word (see patternsOk).
func sigsHaveKeywordSlot(viable []ViableSig) bool {
	for _, vs := range viable {
		if vs.Sig.QuoteArgs == nil {
			continue
		}
		for p := range vs.Sig.QuoteArgs {
			if pat, ok := SigPattern(vs.Sig, p); ok && IsAtom(pat) {
				return true
			}
		}
	}
	return false
}

// pruneKeywordViable filters the viable set in place, dropping every
// signature whose slot at pos is a KEYWORD slot this token cannot
// satisfy: only a Word token bearing the pattern's name matches, and
// the match is binding-agnostic like every /q capture. Without this
// prune a keyword overload keeps the viable set wide past its own
// miss, raising the scan's reach so a LATER paren group is
// pre-evaluated across the true dispatch's barrier.
func pruneKeywordViable(viable []ViableSig, pos int, tok Value) []ViableSig {
	tokName := ""
	if IsWord(tok) {
		if wi, err := AsWord(tok); err == nil {
			tokName = wi.Name
		}
	}
	kept := viable[:0]
	for _, vs := range viable {
		keep := true
		if pos < vs.Barrier && vs.Sig.QuoteArgs != nil && vs.Sig.QuoteArgs[pos] {
			if pat, ok := SigPattern(vs.Sig, pos); ok && IsAtom(pat) {
				if pn, err := AsAtom(pat); err != nil || pn != tokName {
					keep = false
				}
			}
		}
		if keep {
			kept = append(kept, vs)
		}
	}
	return kept
}

// resolveForwardArgs is the interpreter's seat on the phase-1 plan walk:
// it hands the Engine to the collection kernel as the host, with the tape
// as the window and the token after the dispatching word as the start.
// The walk itself lives in collect_kernel.go, where the three collection
// loops meet one seam (design/FULL-COMPILATION.0.md §6.2).
func (e *Engine) resolveForwardArgs(fn *FnDefInfo, w WordInfo) error {
	// Offer the region to the descriptor recorder BEFORE collecting: the
	// walk mutates the window (a group collapses, sugar lowers), so the
	// written-order extent a descriptor must record is the one that exists
	// now. Reading it afterwards would record the post-collection shape,
	// which is not what a later execution re-derives from.
	RegionRecorder(e.Tape, e.Registry, w, e.Pointer)
	return CollectForward(e, fn, w, e.Pointer+1)
}

// --- the Engine's CollectHost seat ------------------------------------
//
// Each method is a direct call so it inlines: the two largest collection
// loops are 13.4% and 18.0% of interpreter CPU, which is the budget the
// re-seat has to stay inside, and the alloc-ceiling tests gate the rest.

func (e *Engine) Window() CollectWindow { return e.Tape }

func (e *Engine) EvalGroupAt(i int) error { return e.evalParenGroupAt(i) }

func (e *Engine) EvalInterp(tok Value) (Value, error) { return e.evalInterpString(tok) }

func (e *Engine) EvalXml(tok Value) (Value, error) { return e.EvalXmlInterp(tok) }

func (e *Engine) ExpandSugarAt(tok Value, pos, i int, viable []ViableSig) (bool, error) {
	return e.expandScanSugar(tok, pos, i, viable)
}

func (e *Engine) FlowInterrupted() bool { return e.Registry.FlowCtrl != FlowNone }

func (e *Engine) ScratchParenSpan(items []Value) []Value { return e.expandParenExprScratch(items) }

func (e *Engine) DefTop(name string) (Value, bool) { return e.Registry.Defs.Top(name) }

func (e *Engine) IsFnWordBarrier(tok Value) bool { return e.fnWordBarrierAt(tok) }

func (e *Engine) IsReachCallHead(tok Value, viable []ViableSig, pos, i int) bool {
	return e.reachCallHeadBarrier(tok, viable, pos, i)
}

func (e *Engine) LookupWord(name string) *FnDefInfo { return e.Registry.Lookup(name) }

func (e *Engine) ExpandSugarTokens(sinfo SugarInfo, tok Value, head bool) ([]Value, error) {
	return SugarExpansion(e.Registry, sinfo, tok, head)
}

// fnWordBarrierAt reports whether a scan token is a bare function word
// acting as a forward-collection barrier: registered as a function and
// NOT `/v`-marked. An `/v`-marked word is NO barrier — it denotes its
// REFERENCE value (inert data), never a call; the call-site marker is
// explicit intent (NUR050/G12). The scan then counts it as an ordinary
// optimistic position, and stepWordVal resolves and DELIVERS the
// Function value to the parked forward at arrival (the phase-2 half of
// this rule — keep in sync per design/FORWARD-COLLECTION-PHASES.10.md).
func (e *Engine) fnWordBarrierAt(tok Value) bool {
	return FnWordBarrierOn(e.Registry, tok)
}

// FnWordBarrierOn is the fn-word collection barrier over an explicit
// registry: a bare word BOUND to a dispatching definition stops forward
// collection, and `/v` opts out because it takes the value rather than
// calling. Registry-only — it never needed a window.
func FnWordBarrierOn(reg *Registry, tok Value) bool {
	wi, werr := AsWord(tok)
	return werr == nil && reg.Lookup(wi.Name) != nil && !wi.ForceVal
}

// sigRawSlot reports whether signature sig captures position pos structurally
// (raw paren / macro form / type-literal slot) rather than by ordinary value
// type — positions where a concrete-value type-prune would be unsound.
func sigRawSlot(sig *Signature, pos int) bool {
	return (sig.RawParens != nil && sig.RawParens[pos]) ||
		(sig.FormArgs != nil && sig.FormArgs[pos]) ||
		(sig.TypeArgs != nil && sig.TypeArgs[pos])
}

// FwdKind classifies a forward token by what it presents to signature
// matching WITHOUT evaluation.
type FwdKind int

const (
	FwdValue    FwdKind = iota // a token whose match-value/type is knowable now
	FwdGroup                   // an unevaluated paren/expr/reach group, type unknown
	FwdBoundary                // stops or is transparent to forward matching
)

// StaticForwardType classifies a forward token by what it presents to
// signature matching WITHOUT evaluation, returning a match-value (for
// FwdValue) plus the classification.
//
// Only a concrete LITERAL value yields FwdValue: its type is final and
// matchSignature type-tests it identically (its literal-arg branch calls the
// same sigArgMatches), so it is sound to prune the viable set on it. A WORD
// is deliberately NOT a prunable value — matchSignature's treatment of a word
// is contextual (a `/q` QuoteArgs slot captures it as an Atom *name*
// regardless of any current binding; a word bound to an FnDef can still match
// a type-name slot via fall-through). Pruning on a word's resolved binding
// would diverge from the binder, so a word is classified as FwdBoundary:
// counted as one resolved position with the viable set left intact, exactly
// as the former eager scan treated it.
func (e *Engine) StaticForwardType(tok Value) (Value, FwdKind) {
	return StaticForwardTypeOf(tok)
}

// StaticForwardTypeOf is StaticForwardType as a free function. It needed
// NOTHING from the Engine — not even a window — which is worth stating,
// because it is the classification whose doc explains at length why a WORD
// is deliberately NOT prunable. A second host reading a different copy of
// that reasoning is how the two lanes would drift.
func StaticForwardTypeOf(tok Value) (match Value, kind FwdKind) {
	if IsOpenParen(tok) || IsParenExpr(tok) || IsReach(tok) {
		return Value{}, FwdGroup
	}
	if IsWord(tok) {
		return Value{}, FwdBoundary
	}
	if IsConcrete(tok) {
		return tok, FwdValue
	}
	// Type literals, carriers, markers: not a prunable concrete value.
	return Value{}, FwdBoundary
}

// evalParenGroupAt collapses the forward paren group at stack index scanIdx
// in place, evaluating its contents on this engine and leaving the result
// value(s) at scanIdx. Extracted verbatim from the former preEvalParens
// OpenParen branch; e.pointer is saved and restored. A flow-control signal
// raised inside the paren is left on the registry for the outer Run frame.
func (e *Engine) evalParenGroupAt(scanIdx int) error {
	savedPointer := e.Pointer

	// This loop tracks the group's extent with a local paren-`depth`
	// counter. TCO's frame-region rewrite would splice the tape out from
	// under that counter, so tail calls dispatched inside here must nest,
	// not elide — tcoEligible reads this flag. Balanced restore below.
	e.parenEvalDepth++
	defer func() { e.parenEvalDepth-- }()

	// Check mode: snapshot the group's inner tokens before evaluation
	// reduces them. If the group collapses to a single Boolean
	// carrier, the tokens are attached as a GuardFactInfo payload so
	// guard narrowing can recover the `x is T` structure from the
	// canonical paren condition form (checker-accuracy-review.10.md A3).
	var guardToks []Value
	var groupSpan, lenBefore int
	if e.Registry.analysisActive() {
		gdepth := 0
		for i := scanIdx; i < e.Tape.Len(); i++ {
			v := e.Tape.At(i)
			if IsOpenParen(v) {
				gdepth++
				if i > scanIdx {
					guardToks = append(guardToks, v)
				}
				continue
			}
			if IsCloseParen(v) {
				gdepth--
				if gdepth == 0 {
					groupSpan = i - scanIdx + 1
					break
				}
				guardToks = append(guardToks, v)
				continue
			}
			if i > scanIdx {
				guardToks = append(guardToks, v)
			}
		}
		lenBefore = e.Tape.Len()
	}

	e.Pointer = scanIdx

	// Advance past the OpenParen marker.
	if err := e.stepOpenParen(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		e.Pointer = savedPointer
		return err
	}

	// Step through contents until we reach the matching ")". Track paren
	// depth so inner parens are processed without prematurely breaking on
	// their ")" tokens.
	depth := 1
	for limit := 0; limit < e.stepLimit && depth > 0; limit++ {
		if e.Pointer >= e.Tape.Len() {
			break
		}
		v := e.Tape.At(e.Pointer)
		// Line-coverage seam (coverage.go): a forward-arg paren group evaluates
		// its inner tokens HERE, off the main loop — the dominant path for
		// `def x (f …)`-style fn-body statements. Mirror the main loop's emit so
		// nested fn-body rows attribute to the module-under-test.
		e.Registry.noteCoverage(v.Pos())

		if IsOpenParen(v) {
			depth++
			e.Pointer++
			continue
		}
		if IsCloseParen(v) {
			depth--
			if err := e.stepCloseParen(false); err != nil {
				e.Pointer = savedPointer
				return err
			}
			if depth == 0 {
				break
			}
			continue
		}

		// Normal evaluation inside paren.
		switch {
		case IsWord(v):
			if err := e.stepWord(v); err != nil {
				e.Pointer = savedPointer
				return err
			}
		case IsEnd(v):
			if err := e.stepEnd(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				e.Pointer = savedPointer
				return err
			}
		case IsMark(v):
			e.stepMark(v)
		case IsMove(v):
			if err := e.stepMove(v); err != nil {
				e.Pointer = savedPointer
				return err
			}
		case IsForward(v):
			e.Pointer++
		case IsReturnCheck(v):
			e.Pointer++
		case IsDefCleanup(v):
			if err := e.stepDefCleanup(v, e.Pointer); err != nil {
				e.Pointer = savedPointer
				return err
			}
			e.Pointer++
		case IsInterpString(v):
			// Mirror the main loop: evaluate the template in place and
			// re-step the resulting string (no pointer advance), so a
			// paren-wrapped template — `raise (`bad: ${x}`)` — collapses
			// to a String rather than a raw InterpString token.
			result, err := e.evalInterpString(v)
			if err != nil {
				e.Pointer = savedPointer
				return err
			}
			result.pos = v.pos
			e.Tape.Set(e.Pointer, result)
		case IsXmlInterp(v):
			// Mirror the InterpString case: a paren-wrapped interpolated
			// XML literal — `f (<p>${x}</p>)` — collapses to a Node/Xml.
			result, err := e.EvalXmlInterp(v)
			if err != nil {
				e.Pointer = savedPointer
				return err
			}
			result.pos = v.pos
			e.Tape.Set(e.Pointer, result)
		default:
			if err := e.stepLiteral(); err != nil {
				e.Pointer = savedPointer
				return err
			}
		}
		// Propagate any flow-control signal raised by the step; the outer
		// Run frame will resolve it.
		if e.Registry.FlowCtrl != FlowNone {
			e.Pointer = savedPointer
			return nil
		}
	}

	// Budget exhausted before the group closed (depth still open with tape
	// left to process). Report it explicitly rather than silently
	// returning a half-evaluated group, which would later surface as a
	// phantom "unmatched opening parenthesis" at the top-level drain.
	if depth > 0 && e.Pointer < e.Tape.Len() { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		e.Pointer = savedPointer
		return e.evalLimitError(e.stepLimit)
	}

	// Guard-fact attachment (A3): the group reduced to exactly one
	// value and it is a Boolean carrier — keep the original tokens on
	// it for guard narrowing. A 2-token group (`(is-map x)`, a predicate
	// guard) qualifies alongside the 3-token `x is T` triple. DYNAMIC
	// Boolean carriers qualify too (a dynamic-binding guard still narrows
	// its then-branch); the reduced payload is preserved in Prev so a
	// statically-decided cond stays readable by the unreachable-branch
	// analysis (LiteralCondValue).
	if groupSpan > 0 && len(guardToks) >= 2 {
		nResults := e.Tape.Len() - lenBefore + groupSpan
		if nResults == 1 && scanIdx < e.Tape.Len() {
			res := e.Tape.At(scanIdx)
			if res.Carrier && res.Parent.ConformsTo(TBoolean) {
				res.Data = GuardFactInfo{Toks: guardToks, Prev: res.Data}
				e.Tape.Set(scanIdx, res)
			}
		}
	}

	e.Pointer = savedPointer
	return nil
}

// stepWord handles a word (function reference) at the current pointer.
//
// The reserved tape-syntactic tokens — `(`, `)`, `end` — never reach
// here: the parser emits them as typed OpenParen / CloseParen / End
// values, and the Run-loop switch dispatches them directly. stepWord
// therefore deals only with regular named words.
// stepWordUsurp handles the /u (ForceUsurp) word modifier: resolve the
// name to its bound Function value and wrap it so its signature
// argument order is reversed (usurped a b c ≡ f c b a). Unlike /v — which
// is total over binding kinds — /u is legal only for function words:
// there is no reversed wrapper to build over a non-fn value, so a non-fn
// binding raises illegal_ref. /u alone dispatches the wrapper
// immediately (like a bare word); /uv (combined with /v) leaves the
// wrapper on the stack as inert data.
func (e *Engine) stepWordUsurp(val Value, w WordInfo) error {
	v, ok := ResolveUsurp(e.Registry, w.Name)
	if !ok {
		if e.Registry != nil && e.Registry.analysisActive() {
			e.Registry.noteAnalysisDiagnostic(CheckBraid.UndefinedWordCheckDiag(e, w.Name, val.Pos()))
			placeholder := NewAtom(w.Name)
			placeholder.pos = val.pos
			placeholder.Undefined = true
			e.Tape.Set(e.Pointer, placeholder)
			return e.stepLiteral()
		}
		return e.undefinedWordError(w.Name, val.Pos())
	}
	// /u may usurp only function words — a non-fn binding has no
	// signature to reverse. ResolveUsurp returns the raw binding in
	// that case so the IsFunctionRef check below raises illegal_ref.
	if !IsFunctionRef(v) {
		detail := "/u requires a function word: " + w.Name + " is bound to " + v.Parent.String()
		if e.Registry != nil && e.Registry.analysisActive() {
			e.Registry.noteAnalysisDiagnostic(CheckDiagnostic{
				Code:   "illegal_ref",
				Detail: detail,
				Word:   w.Name,
				Row:    val.Pos().Row,
				Col:    val.Pos().Col,
			})
			// Check mode is lenient (the illegal_ref diagnostic is advisory), but
			// the interpreter raises illegal_ref here at runtime. Record a TERMINAL
			// trap so a compiled program raises the byte-identical error in place
			// instead of refusing on the downstream Undefined placeholder. Only a
			// top-level trap is recordable; a nested /u keeps the placeholder path
			// and refuses (falls back) as before.
			e.Registry.analysisRecorder().RecordTrap("illegal_ref", detail, w.Name, "", e.currentPos())
			placeholder := NewAtom(w.Name)
			placeholder.pos = val.pos
			placeholder.Undefined = true
			e.Tape.Set(e.Pointer, placeholder)
			return e.stepLiteral()
		}
		return &BoruError{
			Code:       "illegal_ref",
			Detail:     detail,
			Src:        w.Name,
			Row:        val.Pos().Row,
			Col:        val.Pos().Col,
			FullSource: e.effectiveSource(),
		}
	}
	v.pos = val.pos
	if w.ForceVal {
		// /uv: leave the usurped wrapper as inert data (mirrors /v) — it
		// still dispatches if args follow or it is later stepped. As
		// DATA it is a legitimate arrival for a pending forward, so no
		// barrier commit here.
		e.Tape.Set(e.Pointer, v)
		if e.recorder != nil && !e.inFnFrame() && IsRecordableLiteral(v) {
			e.recorder.OnPushLit(v)
		}
		e.Pointer++
		return nil
	}
	// /u alone dispatches the wrapper like a bare function word — a
	// statement boundary. Commit the nearest parked forward that can
	// already fire BEFORE rewriting the token (mirroring the bare-word
	// hook in stepWord); without this, the /u path bypassed the
	// barrier and `if (c) [t] sub/u 10 3` swallowed the usurped
	// wrapper as a phantom else instead of running it. On commit the
	// loop re-steps this /u word with no forward pending.
	if e.commitBarrierForward() {
		return nil
	}
	if strictForwardBarrier {
		if serr := e.strandedForwardError(w.Name); serr != nil {
			return serr
		}
	}
	e.Tape.Set(e.Pointer, v)
	// Dispatch the unquoted wrapper now: stepLiteral routes it through
	// execFnDefLiteral, which forward-collects any trailing args.
	return e.stepLiteral()
}

// stepWordVal handles the /v modifier (the ForceVal branch of stepWord):
// resolve the name to its bound Function value as data, with no argument
// collection or dispatch. Extracted from stepWord (mirroring stepWordUsurp)
// so the dispatch hub stays under the cyclomatic-complexity bound; the body
// is a verbatim move — behaviour is unchanged.
func (e *Engine) stepWordVal(val Value, w WordInfo) error {
	v, ok := ResolveRef(e.Registry, w.Name)
	if !ok {
		if e.Registry != nil && e.Registry.analysisActive() {
			// Deliberately NO fn-carrier side-table consult here, unlike
			// stepWord's twin branch: substituting the carrier for a `/v`
			// read of a carrier-bound name lets the check pass green-light
			// a unit whose lowering DROPS the operand — the top-level read
			// has no producing event and no fn-unit home for the dyn-scope
			// rescue, so `(pmany digit/v)` compiled to a 0-arg call and
			// raised signature_error where the interpreter succeeds
			// (frontier-hof-audit.tsv's pmany/pseq rows). Until the
			// residual re-push machinery can lower such a read, the
			// undefined_word diagnostic below keeps those units refused —
			// slow, never wrong.
			e.Registry.noteAnalysisDiagnostic(CheckBraid.UndefinedWordCheckDiag(e, w.Name, val.Pos()))
			placeholder := NewAtom(w.Name)
			placeholder.pos = val.pos
			placeholder.Undefined = true
			e.Tape.Set(e.Pointer, placeholder)
			return e.stepLiteral()
		}
		return e.undefinedWordError(w.Name, val.Pos())
	}
	// `/v` denotes the binding's VALUE, whatever kind it is. There is no
	// function-only gate: for a fn binding it suppresses the call, and for
	// any other binding it is the identity — the same spelling reads a slot
	// whose kind is not known statically (NUR085).
	//
	// Freeze discipline, the `/v` READ. This path resolves and pushes the
	// binding without passing through stepWord's substitution branches, so
	// until 2026-09-04 neither the value arm nor the type arm saw a `/v` read
	// and BOTH bakes escaped the latch. Measured, on the default lane:
	//
	//	def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  def T String  f
	//	  interpreted -> true false      compiled -> true true
	//	def k 5  def f fn [[] [Integer] [k/v add 2]]  f  def k 9  f
	//	  interpreted -> 7 11            compiled -> 7 7
	//
	// The type row was reported by review on the type arm's own PR; the value
	// row is older than that arm and had been open since the discipline
	// landed. One more instance of the same structural fault the arms
	// document: the note is attached to READ PATHS, so each path that
	// resolves a binding its own way escapes it silently.
	//
	// The two predicates MIRROR the two stepWord arms exactly — a bare type
	// node bakes its identity unconditionally, a value bakes only when it is
	// concrete — so `/v` and the ordinary spelling cannot answer the freeze
	// question two ways.
	e.noteBindingRead(w.Name, v)
	if e.Registry.analysisActive() {
		e.Registry.analysisRecorder().NoteValRead(v.ID, w.Name)
	}
	v.pos = val.pos
	// A reference denotes DATA. When a parked forward in this paren
	// scope is still collecting, deliver the reference through the
	// normal literal step so it ARRIVES like any value and fills the
	// waiting slot (`wa {x:1} some-fn/v` — NUR050/G12; the phase-1 scan
	// counts the /v-marked word as an ordinary position, this is the
	// phase-2 half). Delivered UNQUOTED, exactly like the
	// expecting-Function arrival path below — a Quoted delivery binds a
	// quoted param, which the VM honours as data while the interpreter
	// dispatches it: an engine divergence the differential gate catches.
	// The former unconditional set+advance left the value BEHIND the
	// pointer, so the forward stranded at end of run.
	if e.hasPendingForwardCollecting() {
		e.Tape.Set(e.Pointer, v)
		return e.stepLiteral()
	}
	e.Tape.Set(e.Pointer, v)
	// (The use is recorded inside ResolveRef, covering this `/v` path, the
	// `ref` word, and export-map reference values alike.)
	// `/v` resolves the name to its bound value and ADVANCES the
	// pointer, exactly like pushing a literal — it does NOT dispatch a
	// resolved function (that is what a bare word does). The value
	// stays a plain Function, so it still dispatches when later stepped
	// (e.g. `get` for `pkg.fn` / `m.fn arg`, or a bare word).
	if e.recorder != nil && !e.inFnFrame() && IsRecordableLiteral(v) {
		e.recorder.OnPushLit(v)
	}
	e.Pointer++
	return nil
}

// noteWordRead tells the recorder that a bare read of a binding pushed a
// check-mode CARRIER the interpreter would DISPATCH were the binding to hold
// a fn at run time: a fn-typed carrier (a `g:Function` param), or a gradual
// one (an `x:Any` param, a Dynamic value) whose static type admits a fn. The
// runtime rule this mirrors is stepWord's own: a bound FnDefInfo is not
// substituted, it "goes through normal Lookup" — a word dispatch under the
// binding name — EXCEPT when a pending forward expects a Function, where
// the value is delivered as data (the branch above the binding cases). A
// concrete or non-fn-admitting carrier is a plain value substitution on
// both engines and notes nothing.
// pos is the READ's position (the word token's), which is where the
// interpreter anchors the dispatch's errors (`cannot call `g“).
func (e *Engine) noteWordRead(v Value, name string, pos SrcPos) {
	if v.Quoted || e.hasPendingForwardExpectingFunction() {
		return
	}
	if IsFnTypedCarrier(v) || (v.Dynamic && SigTypeMatches(v, TFunction)) {
		e.Registry.analysisRecorder().NoteWordRead(v, name, pos)
	}
}

// hasPendingForwardCollecting reports whether a parked Forward in the
// current paren scope is still collecting arguments — the generic twin
// of hasPendingForwardExpectingFunction, used by stepWordVal to decide
// whether a `/v` reference should ARRIVE (feed the forward) rather than
// be pushed behind the pointer.
func (e *Engine) hasPendingForwardCollecting() bool {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			return fwd.CollectedArgs < fwd.Sig.TotalArgs()
		}
	}
	return false
}

func (e *Engine) stepWord(val Value) error {
	w, werr := AsWord(val)
	if werr != nil {
		// A Word-TYPED value with no WordInfo payload is a CARRIER, not a
		// token: check mode's carrier-strip nulls the payload, so there is
		// no name to look up and no arguments to collect. It is the
		// abstract value "some word", and the only sound step for an
		// abstract value is to collect it as data. Dispatching it instead
		// fell through every name arm to the undefined-word tail and
		// emitted a NAMELESS `undefined_word` diagnostic — an error no
		// source position could explain, raised on the compile path alone
		// (NUR103). Reachable for any word-typed carrier: a `w:Word`
		// record field read, a declared-Word return, a Word-typed def.
		return e.stepLiteral()
	}

	// /u modifier — see stepWordUsurp. Handled before the /v branch
	// so the /uv combo usurps rather than plain-referencing.
	if w.ForceUsurp {
		return e.stepWordUsurp(val, w)
	}

	// /v modifier: resolve the name to its bound Function value as data,
	// with no argument collection or dispatch. The FnDef binding comes
	// back as an (unquoted) Function value that sits on the stack like any
	// other piece of data — exactly the case `valof` exists to enable. /v
	// is TOTAL over binding kinds: for a fn it suppresses the call, for
	// anything else it is the identity, so one spelling reads a slot whose
	// kind is not known statically. Only an unbound name refuses (NUR085).
	if w.ForceVal {
		return e.stepWordVal(val, w)
	}

	// If there is a pending forward whose next slot is /q-marked
	// (QuoteArgs), capture this Word as data (converted to an Atom
	// further down the pipeline) rather than executing it. This is the
	// general word-capture mechanism: def, undef, type, untype, quote,
	// inspect, and similar words all declare /q on their name slot, so
	// `undef foo` works even when foo is currently defined. See
	// signature.go §1.5 on /q.
	if e.hasPendingForwardQuoteArg() {
		return e.stepLiteral()
	}

	// If a pending forward's next slot is FormArgs (macro raw capture),
	// collect this Word as a raw Word — do NOT execute it, and (unlike
	// QuoteArgs) do NOT coerce it to an Atom. stepLiteral collects the value
	// at the pointer; the Word matches the macro's Any-typed slot, so no
	// conversion fires. See design/MACROS-PHASE1.10.md §3.
	if e.hasPendingForwardFormArg() {
		return e.stepLiteral()
	}

	// If a pending forward expects TFunction, resolve this word to a
	// function reference value rather than executing it. The word must
	// have an FnDefInfo entry in DefStacks.
	if e.hasPendingForwardExpectingFunction() {
		// Wrap the aggregate dispatch view so the reference carries every
		// overload of the name (across stacked defs), not just the topmost
		// entry's own sigs.
		if fnDef := e.Registry.Lookup(w.Name); fnDef != nil {
			// Resolving a name INTO a Function slot is a use of that def, for
			// the same reason ResolveRef records one (core_ref.go:31-35): the
			// name is consumed as a value rather than called, so nothing else
			// on this path marks it. Without the note, `boru check` reports
			// `unused_def` for every fn handed bare to a callback API — the
			// canonical `Sort.quick mycmp xs` idiom — which is a false positive
			// on the single most common way a library takes a function.
			//
			// Noted only on a SUCCESSFUL Lookup, so a genuinely unused def
			// still warns: the fall-through below is a non-fn word.
			e.Registry.noteAnalysisUse(w.Name)
			e.Tape.Set(e.Pointer, NewFunction(*fnDef))
			return e.stepLiteral()
		}
		// Not a def fn — fall through to normal execution.
	}

	// Named user-defined types take priority over DefStacks: type
	// bindings stack independently from def bindings, and a shadow-
	// then-reveal pattern (`type Foo Integer; type Foo fn […]`)
	// would otherwise see the legacy InstallDef mirror in DefStacks
	// instead of the active fn-type binding. Pushed with Quoted=true
	// for fn-shape types so execFnDefLiteral treats them as data.
	//
	// Word-capture cases (untype Foo, etc.) are intercepted earlier
	// via hasPendingForwardExpectingWord — by the time we reach this
	// priority block, no /q-Atom or Word slot is waiting for the
	// name. The pushed value flows through stepLiteral so a pending
	// forward can still consume it (e.g. `Color` as the value side
	// of an export map entry).
	if e.Registry != nil {
		if entry, ok := e.Registry.Defs.TopEntry(w.Name); ok && entry.TypeDef != nil {
			// A type name DENOTES its minted (or adopted) lattice node
			// (the Stage 2 flip, design/TYPE-REPRESENTATION.1.md §5):
			// evaluating `M` pushes the M node for every declaration
			// kind, exactly as `P` always pushed the refine node. The
			// declared structure stays reachable from the node
			// (Value.TypeBody); nothing on the evaluation path reads
			// the stored body any more. A bare node never dispatches,
			// so the fn-shape Quoted special case the body push needed
			// is gone with the body.
			//
			// Freeze discipline, TYPE half. A module-scope type binding read
			// inside an open fn/closure unit BAKES: resolveOperand's bare-
			// type-node arm returns a typeOperand, which lowers to OpPushType
			// carrying the node's IDENTITY, chosen at compile time. The node
			// it names stays live, but the ID does not, so a later rebind of
			// the NAME leaves the unit resolving the old node while the
			// interpreter re-resolves the name per call (NUR097's late half,
			// which the single binding store gives type names too).
			//
			// Measured before this note existed — a silent wrong answer on
			// the DEFAULT lane, not a -force-compile curiosity:
			//
			//	def T Integer  def f fn [[] [Boolean] [5 is T]]  f  def T String  f
			//	  interpreted -> true false      compiled -> true true
			//
			// and the `undef` twin answers `true true` where the interpreter
			// raises undefined_word, because an ALIAS binding is not Minted
			// and so retires no node (basic/go/native_definition.go's undef
			// type arm) — the baked ID keeps resolving after its binding is
			// gone. The value-name twin of both rows has been refused since
			// the freeze discipline landed; only the type half was missing.
			//
			// No IsConcrete conjunct here, deliberately. On the value side
			// that test is a PROXY for "the unit baked it" and is neither
			// necessary nor sufficient; here the bake is unconditional — a
			// bare type node reaching resolveOperand has exactly one arm —
			// so the real decision is available and the proxy is not needed.
			push := NewTypeLiteral(entry.TypeDef)
			e.noteBindingRead(w.Name, push)
			push.pos = val.pos
			e.Tape.Set(e.Pointer, push)
			return e.stepLiteral()
		}
	}

	// Simple value def: substitute the word with its value directly,
	// bypassing function dispatch entirely. FnDefInfo and ClassTypeInfo
	// entries are not simple values — they go through normal Lookup.
	if top, ok := e.Registry.Defs.Top(w.Name); ok {
		switch top.Data.(type) {
		case FnDefInfo, *ClassTypeInfo:
			// Not a simple value — fall through to Lookup.
		default:
			// f w ≡ f (w): a word bound to a DATA __SP splice marker that
			// is being collected by a pending forward expands as the paren
			// group (w) instead of substituting the raw marker — the
			// payload's values arrive from a barrier-protected segment (a
			// multi-value splice fills several slots, exactly as the
			// written form does; without this the marker itself would be
			// collected into an Any slot or implicit-end a typed one). The
			// /q and FormArgs intercepts above this block keep their
			// word-capture semantics; a raw-paren-capturing forward
			// collects the wrapped (w) as code via stepLiteral's existing
			// ParenExpr gate; code-bearing splices keep their live-stack
			// macro semantics (spliceIsData); and a pending BINDER
			// collects the raw marker so `def y xs` rebinds it — the new
			// name aliases the splice (bindsReferent). Inside the group
			// the OpenParen barrier hides the pending forward, so the
			// re-stepped word takes the standalone splice-fire path — no
			// recursion (and recordUse fires on that inner step).
			// IsSplice gates the AsSplice destructure: bindings are almost
			// never splices, and AsSplice's failure path builds a discarded
			// fmt.Errorf — an allocation on EVERY def-value substitution
			// without the guard (the same guard the collection-side twin at
			// the stepLiteral splice check already carries).
			if IsSplice(top) {
				if info, serr := AsSplice(top); serr == nil && spliceIsData(info) {
					if fwdIdx := e.pendingForwardIdx(); fwdIdx >= 0 {
						if fwd, ferr := AsForward(e.Tape.At(fwdIdx)); ferr == nil && !bindsReferent(fwd.FuncName) {
							pe := NewParenExpr([]Value{val})
							pe.pos = val.pos
							e.Tape.Set(e.Pointer, pe)
							return e.stepLiteral()
						}
					}
				}
			}
			// Record the substitution as a "use" for unused-def
			// tracking in check mode.
			e.Registry.noteAnalysisUse(w.Name)
			// Remember which NAME produced this value (compile passes): if the
			// value later has no compiled home in a fn unit, the read was a
			// dynamic-scope reference and lowers to a runtime name lookup
			// (resolveOperand's dynScopeRescue). Gated on an active check —
			// NoteDefRead is a no-op outside a pass, but the bare call still
			// evaluated top.ID unconditionally on the run-mode hot path.
			if e.Registry.analysisActive() {
				e.Registry.analysisRecorder().NoteDefRead(top.ID, w.Name)
				e.Registry.analysisRecorder().NoteLocalRead(top.ID, val.Pos())
				e.noteWordRead(top, w.Name, val.Pos())
				// Freeze discipline: a CONCRETE module-scope binding read inside
				// an open fn/closure unit bakes into the unit across calls, where
				// the interpreter re-resolves the name per call — a later module
				// rebind would diverge. Note it so NotifyNameRebound refuses.
				e.noteBindingRead(w.Name, top)
			}
			// A def'd word binds a VALUE: push it as-is. Lists bind like
			// maps — `def xs [1,2,3]` makes `xs` the list value, evaluated
			// at def time (so `size xs` → 3). To splice a list's elements
			// onto the stack (the old implicit behaviour / Forth-style
			// macros) use the explicit `def name word [list]` form, whose
			// __SP marker is handled in stepLiteral.
			if e.Registry.analysisActive() {
				// Tag a COPY and write it back: taking &top toward the
				// opaque S9 slot would make every interpreter def read
				// heap-allocate top (the braid call is invisible to
				// escape analysis) — the one measured regression of the
				// slot conversion (TestInterpAllocCeilings).
				tagged := top
				CheckBraid.TagCheckModeDefRead(e, &tagged, w.Name)
				top = tagged
			}
			e.Tape.Set(e.Pointer, top)
			return e.stepLiteral()
		}
	}

	fn := e.Registry.Lookup(w.Name)
	if fn != nil {
		// User-code dispatch — record the name as "used" for
		// unused-def analysis in check mode.
		e.Registry.noteAnalysisUse(w.Name)
		if err := e.policyGateWord(w.Name); err != nil {
			return err
		}
		// Statement boundary: a function word beginning its own dispatch
		// is a forward-collection barrier (the argument-order rule), so
		// first commit the nearest ARRIVAL-parked forward that can
		// already fire — otherwise an optional trailing slot (an
		// else-less `if`'s else) swallows this statement's result, or
		// this statement runs before a guard's then-branch does. See
		// commitBarrierForward.
		if e.commitBarrierForward() {
			return nil
		}
		// Every bare function word beginning its own dispatch is a barrier —
		// uniformly, regardless of arity. The strict rule does not ask "how
		// many args does this word collect"; a nullary `context` and a
		// binary `add` are both function dispatches, so both strand a parked
		// forward that cannot commit (`(context) eq (context)`, `print (add
		// 1 2)`). The ONLY exemption is structural, not arity-based: a
		// dot-access chain is implicitly parenthesized navigation, so its
		// `dot` dispatch is grouped and the outer word sees one value (the
		// Reach hook above never strands). design/STRICT-FORWARD-BARRIER.0.md.
		if strictForwardBarrier {
			if serr := e.strandedForwardError(w.Name); serr != nil {
				return serr
			}
		}
		// Macro dispatch (design/MACROS-PHASE1.10.md §5): a macro word is
		// applied to its raw operands ahead on the tape — BEFORE preEvalParens
		// (1228) or any forward collection, so operands arrive as code. The
		// word sits at e.pointer; execMacro replaces `mac operand…` with the
		// spliced expansion and lets the __SP marker re-step the result.
		if fn.Macro {
			return e.execMacro(e.Pointer, fn)
		}
	}

	if fn == nil {
		if w.Name == "true" {
			e.Tape.Set(e.Pointer, NewBoolean(true))
			return nil
		}
		if w.Name == "false" {
			e.Tape.Set(e.Pointer, NewBoolean(false))
			return nil
		}
		if w.Name == "none" {
			e.Tape.Set(e.Pointer, NewNone())
			return nil
		}
		if w.Name == "null" {
			e.Tape.Set(e.Pointer, NewAtom("null"))
			return nil
		}
		if t, ok := ResolveBuiltinTypeName(w.Name); ok {
			// Route through stepLiteral, exactly like the TopTypeBody
			// priority block: a parked forward must get its arrival
			// event for the resolved literal (loud no-match commits
			// included) — `tape.Set + return nil` would silently strand
			// a pending collection that the pre-opacity parser literal
			// used to satisfy or refuse loudly.
			lit := NewTypeLiteral(t)
			lit.pos = val.pos
			e.Tape.Set(e.Pointer, lit)
			return e.stepLiteral()
		}
		// (r.Types resolution lives in the priority block at the
		// top of stepWord — before DefStacks substitution — so a
		// named user-defined type is never reached here.)
		// Strict rule: an undefined word at the pointer is an error.
		// Names that need to be values must be quoted explicitly (`quote
		// foo` or a literal atom) or land at a /q-quoted argument
		// position, where forward collection captures the word as an
		// Atom before it ever reaches stepWord.
		//
		// In CheckMode the engine emits a diagnostic and continues with
		// an `Atom{Undefined:true}` so static analysis can keep going.
		// The diagnostic is recorded HERE rather than at end-of-Run
		// because the placeholder atom can be consumed by a downstream
		// operation (e.g. a checkModeAssumeSig for `add`) and never
		// reach the result stack — recording at the source guarantees
		// every undefined word produces exactly one diagnostic.
		if !e.Registry.analysisActive() {
			return e.undefinedWordError(w.Name, val.Pos())
		}
		// An analysis pass first resolves a name def-bound to a
		// Function-family CARRIER (a computed fn — installDef installs no
		// Defs binding for those; core_helpers.go's fn arm) through the
		// per-pass side table: the read substitutes the carrier exactly as
		// a def-value read would, with the same use + NoteDefRead
		// provenance, so `def h (mk 1)  (h 2)` feeds the carrier-lead
		// apply machinery (IsFnTypedCarrier) instead of refusing the unit
		// with a false undefined_word. This used to be compile-pass-scoped
		// on the premise that a PLAIN check never holds a table entry (a
		// factory's `fn` handler runs in check and constructs the concrete
		// fn). A native that RETURNS a fn carrier breaks that premise —
		// `def k (FnUtil.const 7)  (k 99)` had `boru check` report
		// undefined_word and unused_def on a correct program, and the
		// check-accuracy and diagnostic-parity gates counted every such
		// row the moment it left the frontier ledger (the thirty-fifth
		// increment). Both passes now resolve the read; the recorder
		// notes are no-ops on the plain check's inactive recorder.
		// A NESTED BODY substitutes too (the thirty-eighth increment). This
		// used to decline at NestedBodyDepth > 0 on the premise that a
		// branch / loop / quotation body is re-run from its TOKENS, where
		// the compiled body carries no binding for the name: `def f (mk 1)
		// do [(f 2)]` once compiled to an island that raised `undefined
		// word: f`. That premise is gone — a code body compiles as a
		// closure unit whose read is modelled as the dispatch it is, and a
		// body a native re-runs through a sub-engine resolves the name
		// through the program's DynEnv twin to a closure the invoker seam
		// applies — while the decline left `if c [(f 2)] [0]` and `for 2
		// [(f 2)]` reporting a FALSE undefined_word on the plain check.
		// The list-member twin of the old corruption is still caught in
		// the compiler (RecordMakeListInner).
		if cv, hit := CheckFnCarrierBind(e.Registry, w.Name); hit {
			e.Registry.noteAnalysisUse(w.Name)
			e.Registry.analysisRecorder().NoteDefRead(cv.ID, w.Name)
			e.Registry.analysisRecorder().NoteLocalRead(cv.ID, val.Pos())
			e.noteWordRead(cv, w.Name, val.Pos())
			// Mark a COMPILE pass: if it ends in a refusal anyway, the
			// compile entry points keep the SILENT interpreter
			// fallback this program class had before Stage 1 (the
			// read used to raise the check-diagnostics sentinel).
			if e.Registry.analysisCompiling() {
				e.Registry.Check.FnCarrierReadSubstituted = true
			}
			cv = WithPos(cv, val)
			e.Tape.Set(e.Pointer, cv)
			return e.stepLiteral()
		}
		e.Registry.noteAnalysisDiagnostic(CheckBraid.UndefinedWordCheckDiag(e, w.Name, val.Pos()))
		v := NewAtom(w.Name)
		v.pos = val.pos
		v.Undefined = true
		e.Tape.Set(e.Pointer, v)
		return nil
	}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values (rule 1.5). Only needed
	// when at least one signature wants forward args (BarrierPos > 0)
	// and the call hasn't been forced to stack mode via /s; or when /f
	// explicitly forces forward collection.
	if (fn.HasForwardSigs() && !w.ForceStack) || w.ForceForward {
		if err := e.resolveForwardArgs(fn, w); err != nil {
			return err
		}
	}

	// Unified signature matching: one path for all words.
	resolved := e.EffectiveResolved()
	sig, positions, specAt := e.MatchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Handles CallBoru sub-engines where FnDef args are
	// placed in deepest-first order on the input stack.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions, specAt = e.MatchSignature(fn, wDeep, resolved)
	}
	// The word-dispatch commit, as planned (dispatch_probe.go): the
	// admission-agreement census reads it here, before the check-mode
	// Fallback recovery below, which analysis alone reaches.
	e.probeDispatch(fn, w, sig, positions, specAt)

	// In check mode, if matchSignature fell through to the 0-arg /
	// Fallback handler because no typed signature matched (but
	// typed signatures exist), treat it as an unmatched call and go
	// through the assume-sig recovery path so the user gets a
	// diagnostic with the typed sig's Returns/ReturnsFn synthesis.
	if sig != nil && sig.Fallback && e.Registry.analysisActive() {
		hasTyped := false
		for i := range fn.Signatures {
			if !fn.Signatures[i].Fallback {
				hasTyped = true
				break
			}
		}
		if hasTyped {
			sig = nil
		}
	}

	// The fallback (0-arg catch-all) was selected. Unless this fn
	// courtesy-dispatches a 0-arg overload (which fnFallbackSig handles),
	// the selection is a genuine no-match — raise the FULL rich
	// diagnostic (received args, per-candidate verdicts, swap probe, fix
	// suggestions) via sigError here, rather than letting fnFallbackSig
	// raise a barer error. This keeps the interpreter's dispatch-failure
	// report uniform across every path, and byte-identical to the
	// compiled OpTrap (which serialises this same sigError) and the
	// compiled runtime param-contract guards.
	if sig != nil && sig.Fallback && !e.Registry.analysisActive() &&
		!fnCourtesyDispatches(e.Registry, w.Name, fn) {
		return e.sigError(w.Name, fn, val.Pos())
	}

	if sig == nil {
		// In check mode, a missing signature is a soft diagnostic
		// rather than a hard error: pick the first-ranked candidate,
		// synthesise carrier return values from it, and splice them
		// in place of the word + up to N adjacent arg slots.
		// We bypass insertForward here because forward collection
		// would re-trigger sigTypeMatches and loop indefinitely.
		if e.Registry.analysisActive() && len(fn.Signatures) > 0 {
			// S2 (design/SURFACES.10.md): a required operation called on
			// a SURFACE-typed carrier types via the contract's shape
			// (Self := the surface node) — the contract guarantees the
			// operation for every member, so this is a correct typing,
			// not a degrade; no diagnostic.
			if handled, herr := CheckBraid.CheckModeSurfaceShape(e, w, val.Pos()); handled {
				return herr
			}
			return CheckBraid.CheckModeAssumeSig(e, w, fn, &fn.Signatures[0], val.Pos())
		}
		return e.sigError(w.Name, fn, val.Pos())
	}

	// Count forward vs stack args from positions.
	fwdCount := 0
	stkCount := 0
	for _, pos := range positions {
		if pos > e.Pointer {
			fwdCount++
		} else {
			stkCount++
		}
	}

	// Check-mode forward-greediness advisories (forward-strand + mixed-form).
	// Extracted from stepWord to keep this hot dispatch path under the
	// cyclomatic-complexity gate; diagnostics only — no effect on execution
	// or dispatch. See design/FORWARD-STRAND-ADVISORY.10.md.
	CheckBraid.CheckMixedFormAdvisories(e, w, sig, positions, val.Pos(), fwdCount, stkCount)

	// Compile-mode stranded-member-fn guard (design/EDGE-SPEC-FINDINGS.0.md §2):
	// a parked user-fn value surfaced from a container read (`m.double`) AUTO-
	// APPLIES the moment a value lands on it in the interpreter. A dispatch that
	// instead consumes that value while the fn sits stranded just below it
	// (`m.double 21 eq 42` — `eq` grabs 21, and the fn is then applied WRONGLY at
	// the residual tail) diverges. Refuse so the body falls back. The
	// statement-tail apply (`m.double 21`, nothing dispatches above the fn) is
	// unaffected — its residual [fn, 21] lowers to the correct trailing apply.
	CheckBraid.RefuseStrandedMemberFn(e, positions)

	// Forward collection needed: defer execution.
	if fwdCount > 0 {
		// traceSigStr formats via fmt on every dispatch; the note is read
		// only when a trace hook is installed (see the e.trace guard in
		// Run's step loop). Gate the build — ~5% of interpreter CPU on
		// dispatch-hot shapes was this dead string (design/
		// INTERPRETER-SPEED-PLAN.10.md #6).
		if e.trace != nil {
			e.traceNote = "forward→ " + traceSigStr(w.Name, sig)
		}
		return e.insertForward(w, sig, fwdCount, stkCount, specAt)
	}

	// Compile-mode forward-collection drift guard
	// (design/EDGE-SPEC-FINDINGS.0.md §1). A forward-eligible word that matched
	// ALL-STACK (fwdCount==0) under a DYNAMIC top-of-stack operand — while a
	// concrete forward token sits right after it — diverges: the interpreter,
	// seeing the operand's CONCRETE runtime value, forward-collects that token
	// into the narrower overload (`add 1` → 7+1=8), whereas the check-mode match
	// (the dynamic carrier blocks that overload's stack slot) reached PAST the
	// dynamic value to a deeper leading residual (`add` over [dyn, 5] → 5+7).
	// The residual-window operand accounting across the reified-error / island
	// boundary is unsound as a STATIC record — but a TERMINAL window models it
	// faithfully as a runtime island (tryRecordDriftWindow, REFUSAL-CLOSURE §1):
	// the window re-steps [residual, dynamic value, word, forward literal]
	// verbatim, so the island's own dispatch performs the interpreter's
	// forward collection. Shapes the window declines (non-terminal, variadic
	// operands, non-contiguous) keep the refusal and fall back. The
	// interpreter and plain check mode are untouched (recorder inactive).
	if DriftWindowRecorder(e, w, sig, positions) {
		return nil
	}
	CheckBraid.RefuseForwardStackDrift(e, sig, positions)

	// Immediate execution: read args from recorded positions.
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

// mixedFormStackSlotAny reports whether the deepest stack-bound sig slot of a
// mixed-form call is Any-typed. The mixed_form_call advisory fires only when it
// is: an Any slot has no type discipline, so a value stacked into it (the
// `(cond) if [a] [b]` footgun, if3's {Any,Any,Any}) silently misbinds — the
// case the advisory exists to flag. A concretely-typed deepest slot (the
// receiver-first `set`/`slice`/user-fn-typed-last idiom) binds correctly, so
// the advisory must stay quiet there. Mirrors checkForwardStrandsOperand's
// deepest-stack walk and its Any-slot bail.
func MixedFormStackSlotAny(e *Engine, sig *Signature, positions []int) bool {
	minStack := -1
	minSigPos := -1
	for sp, p := range positions {
		if p < e.Pointer && (minStack == -1 || p < minStack) {
			minStack = p
			minSigPos = sp
		}
	}
	if minSigPos < 0 {
		return false
	}
	t := SigArgType(sig, minSigPos)
	return t == nil || t.Equal(TAny)
}

// forwardLiteralOperand reports whether t is a single ATOMIC LITERAL that
// forward collection would grab as one operand: a concrete scalar/atom or a
// bare type node. It deliberately excludes structural tokens (paren markers,
// End, Forward, DefCleanup, Mark/Move/Internal/ReturnCheck) and ParenExpr
// data — none of which a forward-collecting word treats as a lone literal
// operand, and all of which showed up as false positives in the drift sweep
// (an `eq`/`and` comparison residual ending on `)` compiles faithfully
// all-stack). The structural exclusions mirror checkForwardStrandsOperand's
// scope-boundary set (a CloseParen marker's Parent quirkily conforms to
// TScalar, so the type test below is not sufficient on its own). Used only by
// refuseForwardStackDrift.
func ForwardLiteralOperand(t Value) bool {
	if IsOpenParen(t) || IsCloseParen(t) || IsForward(t) || IsEnd(t) ||
		IsDefCleanup(t) || IsParenExpr(t) ||
		t.Parent.ConformsTo(TMark) || t.Parent.ConformsTo(TMove) ||
		t.Parent.ConformsTo(TInternal) || t.Parent.ConformsTo(TReturnCheck) {
		return false
	}
	if IsBareTypeNode(t) {
		return true
	}
	if !IsConcrete(t) || t.Parent == nil {
		return false
	}
	return t.Parent.ConformsTo(TScalar) || t.Parent.ConformsTo(TAtom)
}

// dynShuffleConsumerAt reports whether the tape token at idx is a PLAIN
// (modifier-free) Forth-style stack-shuffle word — dup/swap/drop/… with a
// single all-Any registered signature (the dynStackShuffleWords set the
// emitter already trusts over a dynamic stack). Such a word is stack-only
// (BarrierPos 0 — it never forward-collects) and its dispatch shape cannot
// be flipped by one extra stack value, so an optimistic gradual result
// modeled right before it is consumed as safely as a paren-tail one
// (execMatch's consumed-tail lookahead).
func (e *Engine) dynShuffleConsumerAt(idx int) bool {
	if idx < 0 || idx >= e.Tape.Len() {
		return false
	}
	tok := e.Tape.At(idx)
	if !IsWord(tok) {
		return false
	}
	w, err := AsWord(tok)
	if err != nil || !DynStackShuffleWords[w.Name] {
		return false
	}
	// A modifier (/N, /f, /s, /q, /v, /u) changes the collection shape —
	// stay conservative and treat the modified word as an ordinary
	// statement-position consumer.
	if w.ArgCount != -1 || w.ForceForward || w.ForceStack || w.ForceVal || w.ForceUsurp {
		return false
	}
	fn := e.Registry.Lookup(w.Name)
	if fn == nil || len(fn.Signatures) != 1 {
		return false
	}
	for _, t := range fn.Signatures[0].ArgTypes() {
		if t == nil || !t.Equal(TAny) {
			return false
		}
	}
	return true
}

// execMatch executes a matched signature, splicing args and results.
func (e *Engine) execMatch(match *MatchResult) error {
	// Per-export module policy gate (NUR045): every named- and value-
	// dispatch route funnels its matched signature through here — the
	// direct wrapper call (`TimeUtil.sleep 800`), the module-preamble
	// fn call, AND the rebound laundering path (`def s TimeUtil.sleep/v
	// s 300`, whose rebinding copied the stamped inner sigs) — so ONE
	// gate covers them all. Nil ModuleCall (any non-module signature)
	// costs one pointer test; check mode is skipped inside the helper.
	if err := e.policyGateModuleCall(match.Sig.ModuleCall); err != nil {
		return err
	}
	n := match.Sig.TotalArgs()

	// Use recorded positions if available, otherwise derive from stack.
	indices := match.Positions
	if len(indices) == 0 && n > 0 {
		indices = e.ResolvedIndicesBefore(n)
	}
	// Sort indices ascending for splice operations.
	sortedIndices := make([]int, len(indices))
	copy(sortedIndices, indices)
	for i := 1; i < len(sortedIndices); i++ {
		for j := i; j > 0 && sortedIndices[j] < sortedIndices[j-1]; j-- {
			sortedIndices[j], sortedIndices[j-1] = sortedIndices[j-1], sortedIndices[j]
		}
	}

	// Process consumed arguments:
	// - Maps with Eval=true: auto-evaluate their values now, so word
	//   handlers receive resolved data (e.g. {base:hex} → {base:atom(hex)}).
	// - Lists with Eval=true: auto-evaluate their contents now, so word
	//   handlers receive resolved data (e.g. [c1 c2] → [map1, map2]).
	//   Lists at QuoteArgs positions are NOT evaluated (code bodies for
	//   def, if, for, do, etc.).
	//
	// A pending gen spec is SUSPENDED for the duration: it belongs to
	// the word being dispatched (the constructor following `gen
	// [...]`), not to constructors nested inside its arguments — a
	// record field like `f:(fnsig [[T] [T]])` must build a plain
	// fn-shape, not steal the spec.
	restoreGen := e.Registry.SuspendPendingGen()
	defer restoreGen()
	// For fn-body dispatches, snapshot the binding-mutation counter
	// before arg auto-evaluation: the TCO eligibility gate declines
	// eager teardown when the auto-eval below touched any binding.
	var defMutsBefore int64
	if match.Sig.FnFrame() != nil {
		defMutsBefore = e.Registry.Defs.Mutations()
	}
	for i := range match.Args {
		// Dispatch ascription (`v as T`): consumed by the signature match
		// that just selected this sig — the handler / fn body receives the
		// REAL value, so the ascription can never ride into a stored
		// binding, a container write, or a returned receiver (design/
		// OPEN-WORDS.1.md §9's match-time-only rule).
		match.Args[i] = StripAscribed(match.Args[i])
		if match.Args[i].Eval && !match.Args[i].Quoted {
			if match.Args[i].Parent.Equal(TMap) &&
				match.Args[i].Data != nil && !IsTypedMap(match.Args[i]) && !IsRecordType(match.Args[i]) && !IsOptionsType(match.Args[i]) {
				// NoEvalMapArgs (separate from the list-only
				// NoEvalArgs) suppresses map auto-evaluation at this
				// slot. Used by def's typed-name sig so a Word at the
				// type position arrives raw — important when the type
				// is a fn that's also a registered callable.
				noEval := match.Sig.NoEvalMapArgs != nil && match.Sig.NoEvalMapArgs[i]
				if !noEval {
					// A `make` construction body is DATA: its computed shared-mutable
					// values record as per-run OpMakeMap events rather than baking as
					// aliasable consts (a class/refine SCHEMA body keeps folding its
					// field defaults). Keyed on the dispatched word.
					evaluated, err := e.AutoEvalMap(match.Args[i], match.Name == "make", true)
					if err != nil {
						return err
					}
					match.Args[i] = evaluated
				}
			} else if match.Args[i].Parent.Equal(TList) &&
				match.Args[i].Data != nil && !IsTypedList(match.Args[i]) && !IsTableType(match.Args[i]) {
				// NoEvalArgs suppresses list auto-evaluation for code-body
				// positions (def body, if branches, for body, etc.).
				noEval := match.Sig.NoEvalArgs != nil && match.Sig.NoEvalArgs[i]
				if noEval {
					// Check-mode use-scan: a code body (a Test.test / each /
					// fold / do / generator quotation) is NOT evaluated here,
					// so a name referenced ONLY inside it would never reach
					// recordUse and would be falsely flagged unused_def — the
					// "opaque body" FP (a shared fixture value, a generator's
					// charset, a test helper). Walk the body's bare words and
					// record each as a use. Sound for unused-def: a name a
					// runnable body references is a genuine use. Walk the
					// ELEMENTS (the body list itself may be marked quoted,
					// which WalkBodyWords would skip).
					if e.Registry.analysisActive() {
						if lst, lerr := AsList(match.Args[i]); lerr == nil {
							WalkBodyWords(lst.Slice(), func(w WordInfo, _ Value) {
								e.Registry.noteAnalysisUse(w.Name)
							})
						}
					}
				} else {
					// Bare words never degrade to data: a list element
					// that fails to evaluate (an undefined name, or a
					// valid name dispatched with the wrong arity) is an
					// error, not a silent fallback to its literal word.
					// Use `foo/q` for an atom, `quote […]` for a literal
					// word list / quotation.
					evaluated, err := e.autoEvalList(match.Args[i], true)
					if err != nil {
						return err
					}
					match.Args[i] = evaluated
				}
			}
		}
		e.resolveConsumedInertTypeShape(match, i)
		match.Args[i].Eval = false
		// A check-mode Undefined placeholder (a forward-referenced fn name, or
		// an undefined word already diagnosed at its source) reaching a dispatch
		// arg gradualizes to a dynamic Any carrier — otherwise the phantom
		// concrete Atom drives a CASCADING false no_signature on the consuming
		// word. The primary undefined_word diagnostic was already emitted at
		// stepWord; note the def-read so the dynamic-scope rescue still
		// attributes it. Undefined is only ever set in check mode (stepWord
		// errors at runtime), so this never masks a runtime error.
		if match.Args[i].Undefined {
			c := NewDynamicCarrier(TAny)
			if a, aerr := AsAtom(match.Args[i]); aerr == nil && a != "" {
				e.Registry.analysisRecorder().NoteDefRead(c.ID, a)
			}
			match.Args[i] = WithPos(c, match.Args[i])
		}
		// Materialize concrete Options defaults into the map the handler /
		// fn param will receive: a field whose schema declares a real
		// default value, omitted by the caller, is filled in so the
		// consumer sees a complete map (quality DX — no re-deriving
		// defaults). Optional `T tor None` fields carry no concrete value
		// and stay absent. No-op unless this slot's pattern is an Options
		// type and the arg is a plain concrete map.
		if pat, ok := SigPattern(match.Sig, i); ok {
			match.Args[i] = FillConcreteOptionDefaults(pat, match.Args[i])
		}
	}
	// Arg evaluation done — the dispatched word's handler is the
	// intended consumer of any suspended gen spec.
	restoreGen()

	// Static type-check mode: skip the handler, splice carrier results
	// derived from Signature.ReturnsFn / Signature.Returns. The rest of
	// the dispatch machinery (positions, splicing, forward resolution)
	// is shared with normal execution, so runtime and checker stay in
	// parity.
	//
	// Signatures marked RunInCheckMode opt out of this intercept —
	// used by words whose side effects (def, undef, fn, type, …)
	// are prerequisites for subsequent analysis.
	if e.Registry.analysisActive() && !match.Sig.RunInCheckMode() {
		// The dispatch name: the word at the pointer, or — for a
		// VALUE dispatch (a module wrapper's trivial-delegation
		// short-circuit steps the Function literal, not a Word) — the
		// match's own name. A true anonymous lambda has both empty,
		// which is what the emit pass keys its fn-value refusal on.
		name := match.Name
		var pos SrcPos
		if e.Pointer < e.Tape.Len() {
			pos = e.Tape.At(e.Pointer).Pos()
			if w, err := AsWord(e.Tape.At(e.Pointer)); err == nil {
				name = w.Name
			}
		}

		// FullStack signatures in check mode: if a CheckFullStackFn
		// is declared, it receives the preserved carrier stack
		// (below args) and returns the complete replacement for
		// base..end (matching the runtime FullStack path). end
		// covers both the word itself and any forward-collected
		// arg positions so the splice consumes every token the
		// call actually bound.
		if match.Sig.FullStack() && match.Sig.checkFullStackFn() != nil {
			base := 0
			for i := e.Pointer - 1; i >= 0; i-- {
				if IsOpenParen(e.Tape.At(i)) {
					base = i + 1
					break
				}
			}
			end := e.Pointer
			for _, p := range sortedIndices {
				if p > end { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					end = p
				}
			}
			preserved := e.resolvedStackBeforeFrom(base, sortedIndices)
			// A full-stack word reads the WHOLE scope: every fn-typed value
			// in it is a collection hazard (noteCollectionHazards' rule
			// applied to the scope's own base).
			e.noteCollectionHazardsBelow(base, e.Pointer)
			results := match.Sig.checkFullStackFn()(match.Args, preserved, e.Registry)
			// Compile pass: a full-stack word over a provably-exact stack
			// folds statically — the dispatch elides and the fold's outputs
			// carry known provenance (EmitState.FoldFullStack), so
			// depth/pick/roll compile instead of dying at Finalize as
			// residuals of unknown provenance. A declined fold keeps the
			// twin's carrier results and the historical refusal path.
			if folded, ok := e.Registry.analysisRecorder().FoldFullStack(name, match.Args, preserved); ok {
				results = folded
			}
			e.Tape.Splice(base, end+1-base, results...)
			e.Pointer = base
			return nil
		}

		// Consumed-tail lookahead: is the token right after this call's
		// consumed range one that CONSUMES the result without forward-
		// collecting past it? Then carrierResults may safely model the
		// mixed-arity gradual arity under a real compile (a `set` over a
		// dynamic receiver bound by `def`). Two qualifying shapes:
		//   - a CloseParen — the result is the enclosing group's value; and
		//   - a PLAIN stack-shuffle word (drop/dup/… — dynShuffleConsumerAt):
		//     stack-only, single all-Any sig, so it consumes the modeled
		//     value from the stack exactly as the interpreter's does, and one
		//     modeled value can never flip its dispatch shape. The mixed-
		//     arity risk (the runtime overload returns 0 where the model
		//     claimed 1 — a Store/Class receiver) is owned by the VM:
		//     callPoly enforces the recorded result-count claim and defers
		//     to the interpreter on mismatch (slow, not wrong). This is what
		//     compiles the mini-s3/mini-redis statement idiom
		//     `X set (k) v` newline `drop` without source grouping.
		// Any other statement-position call (next token is a word that could
		// forward-collect the result as its own arg — the
		// TestSetOverDynamicReceiverPolyCompiles underflow) gets
		// tailConsumed=false and the faithful 0-arity stands.
		callEnd := e.Pointer
		for _, p := range sortedIndices {
			if p > callEnd { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				callEnd = p
			}
		}
		tailConsumed := callEnd+1 < e.Tape.Len() &&
			(IsCloseParen(e.Tape.At(callEnd+1)) || e.dynShuffleConsumerAt(callEnd+1))
		results := e.Registry.analysisCarrierResults(name, match.Sig, match.Args, pos, match.Reg, tailConsumed)
		// Stamp a positionless FUNCTION result with this call's position,
		// AFTER the recorder has re-IDed the outputs — the interpreter's
		// stampResultPos equivalent for the check pass. A module export
		// (`Assert.not-equal`) is handed back verbatim by moduleNSGetReturns
		// with no position of its own, and it is the token a later VALUE
		// dispatch reads its own position from, so without this every opcode
		// downstream records 0:0 (NUR113).
		stampCallResultPositions(results, pos)
		e.noteFnResultReSteps(match.Sig, callEnd, results)
		e.noteCollectionHazards(match.Sig, sortedIndices)
		return e.spliceMatchResults(match, sortedIndices, n, results)
	}

	// Compute context (cheap O(1) call).
	ctx := e.Registry.Contexts.TopData()

	// Publish the dispatching word's position for the handler about to run
	// (CheckState.CurWordPos). Set HERE, above the FullStack branch, so both
	// dispatch arms below are covered by one assignment — and set for every
	// dispatch, not only in check mode, so no caller has to reason about
	// which lane it is on. It is the same value stampErrPos would stamp an
	// unpositioned error with after the handler returns; a handler that
	// RECORDS a position for the compiled lane needs it before that.
	e.Registry.Check.CurWordPos = e.currentPos()

	var fullStack []Value
	if match.Sig.FullStack() {
		// Find the nearest open-paren barrier so that FullStack handlers
		// only replace within the current paren scope, not below it.
		base := 0
		for i := e.Pointer - 1; i >= 0; i-- {
			if IsOpenParen(e.Tape.At(i)) {
				base = i + 1
				break
			}
		}
		// Collect the full resolved stack before the pointer (from base),
		// excluding the matched args and forwards.
		fullStack = e.resolvedStackBeforeFrom(base, sortedIndices)
		results, err := match.Sig.DispatchHandler()(match.Args, ctx, fullStack, e.Registry)
		if err != nil {
			return e.stampErrPos(err)
		}
		if e.recorder != nil {
			e.recorder.OnCall(match.Name, n, len(results))
		}
		// FullStack handler returns the complete replacement for
		// everything from base through the pointer (inclusive).
		e.Tape.Splice(base, e.Pointer+1-base, results...)
		e.Pointer = base
		return nil
	}

	// Tail calls (design/TCO-STAGED.10.md): a boru fn-body dispatch
	// (Sig.FnFrame non-nil — natives skip on the nil check) sitting in
	// tail position of an enclosing fn frame is counted; when the
	// eligibility gate passes (no binding mutations during arg
	// auto-eval, plain teardown names, no generics, kill switch off)
	// the enclosing frame's teardown runs eagerly, before the handler
	// pushes the callee's per-call state — replacement by ordering,
	// for self AND mutual tail calls alike. Clean frames whose
	// ReturnCheck the callee's own check subsumes (returnsConform)
	// take FULL replacement: the callee's frame tokens replace the
	// caller's entire frame region after the handler returns, keeping
	// tape and stacks O(1) across the chain. Values-below frames and
	// non-conforming returns take the shell variant — the marker run
	// is deleted but the caller's shell (parens + ReturnCheck) stays,
	// so leftover values and the caller's return contract behave
	// exactly as under nesting. A declined call nests as before —
	// correctness never depends on firing.
	var fullReplace *frameTailScan
	if match.Sig.FnFrame() != nil {
		if scan, ok := e.probeTailCall(sortedIndices, n); ok {
			e.Registry.TCO.Detected++
			if e.tcoEligible(scan, match.Sig, defMutsBefore) {
				if scan.ValuesBelow || !e.returnsConform(scan, match.Sig) {
					if err := e.elideTailFrame(scan); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
						return err
					}
					e.Registry.TCO.Elided++
				} else {
					// State teardown now; the frame-region splice
					// happens below, once the handler has produced
					// the replacement tokens. The handler edits no
					// tape, so the scan's indices stay valid.
					if err := e.teardownFrameState(scan); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
						return err
					}
					fullReplace = &scan
				}
			}
		}
	}

	handler := match.Sig.DispatchHandler()
	if handler == nil {
		// A signature with no runner on the WORD path: an un-installed body
		// — a lambda value pushed as a plain def (the check pass's
		// RunFnBodyOnce binds a concrete fn argument that way) — whose
		// dispatch would otherwise call a nil function. The const-fold
		// sub-run (check mode off) reached exactly that for a map literal
		// over an Any param holding a 0-arg lambda and PANICKED
		// (`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`, NUR125). An
		// error is what ADR-005 allows: the fold declines on it and the
		// literal records normally.
		return makeBoruErrorAt("internal_error",
			"no runnable implementation for `"+match.Name+"` on the word path (an un-installed fn body)",
			match.Name, e.effectiveSource(), "this is a bug in boru; please report it", e.currentPos())
	}
	results, err := handler(match.Args, ctx, nil, e.Registry)
	if err != nil {
		return e.stampErrPos(e.maybeAddFnShapeHint(err))
	}
	if e.recorder != nil {
		e.recordDispatch(match.Name, n, results)
	}

	// Stamp handler-produced ReturnCheck markers and fresh Function
	// values that lack a position with the call-site word's position, so a
	// return-type error (named-fn body or anonymous fn value) points at the
	// call/construction rather than the last textual occurrence of the name.
	if e.Pointer >= 0 && e.Pointer < e.Tape.Len() {
		cur := e.Tape.At(e.Pointer)
		stampResultPos(results, cur.pos)
	}

	// Full frame replacement: the callee's frame (the handler result,
	// a complete `( body… tail )` carrying its own ReturnCheck)
	// replaces the caller's entire frame region. The pointer lands on
	// the new frame's open paren — the same re-step position a normal
	// splice would give relative to the spliced tokens. Fn-body sigs
	// never set ParkResult, so skipping that block is moot.
	if fullReplace != nil {
		e.Tape.Splice(fullReplace.FrameOpen, fullReplace.CloseIdx+1-fullReplace.FrameOpen, results...)
		e.Pointer = fullReplace.FrameOpen
		e.Registry.TCO.Replaced++
		return nil
	}

	if err := e.spliceMatchResults(match, sortedIndices, n, results); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		return err
	}
	// ParkResult words (notably `valof`) leave their result as inert data at
	// the call site rather than re-stepping it: advance the pointer past the
	// spliced result so an unquoted Function value does NOT auto-dispatch
	// here (matching the `/v` word-suffix). The value still dispatches when
	// re-stepped elsewhere — retrieved from a map, unwrapped from a paren.
	if match.Sig.ParkResult() {
		e.Pointer += len(results)
	}
	return nil
}

// maybeAddFnShapeHint wraps a signature_error from a fn-dispatch
// failure with a §7.2 hint when the caller is `def`'s typed-binding
// body slot expecting a fn-shape value. Without this, the user sees
// a confusing "no matching signature for double" error pointing at
// double's call site rather than the typed-binding context that
// caused the fn to be invoked at all.
func (e *Engine) maybeAddFnShapeHint(err error) error {
	if err == nil {
		return nil
	}
	boruErr, ok := err.(*BoruError)
	if !ok || boruErr.Code != "signature_error" {
		return err
	}
	if !e.IsFnShapeTypedBindingContext() {
		return err
	}
	boruErr.Suggestions = append(boruErr.Suggestions, DiagSuggestion{
		Message: "this is a typed-binding context expecting a function value — did you mean `" + boruErr.Src + "/q`?",
	})
	return boruErr
}

// noteCollectionHazards is the check pass's COLLECTION-HAZARD note (NUR121).
// A fn-typed value the model leaves UNAPPLIED on its stack — a Function
// carrier read from a param or capture, a fn value with no static
// signatures — would, at run time, collect the values written after it
// before any later word runs: `g x add 1` is `(g x) add 1`. The model
// cannot dispatch such a value, so a later word's STACK collection reaches
// past it and takes what it would have collected (`add 1` takes `x`), and
// every lowering that applies a lead to the values after it — the paren
// lead window, the whole-frame replay, the residual's fn-carrier arm —
// then applies `g` to `add`'s RESULT (18 for the interpreter's 16).
// Nothing in the recorded trace tells this apart from `(g (add x 1))`,
// where the nested paren seals `g` off and 18 is right on both lanes: the
// operands and events are the same, only the SCOPE differs — and the
// scope is the engine's. So the engine notes it here, at the one point
// where a stack collection and its scope are both in hand: every
// unapplied fn-typed value sitting BELOW the lowest stack-collected index
// in the same paren scope is marked, and the lowerings decline a marked
// lead (EmitRecorder.CollectionHazard).
//
// The scope stops at the nearest open paren, and never reaches into a
// frame's resolved-argument prefix — FrameOpenInfo.ArgSpan after that
// paren, or the run's StartAt prefix (inertPrefix) — because those values
// are call-site-resolved arguments the pointer never steps: a Function
// there collects nothing (arguments are inert). Forward-collected indices
// (past the pointer) consume nothing below the word and mark nothing. A
// STRIP-INPUT hop (CallableSpec.StripsUnconsumedInput — `error`'s catch
// clause) marks nothing either: on the path where a lead below it could
// apply, the hop passes the region through untouched, which is exactly
// what the mark-window lowering re-steps at the program's end
// (`do [(f 5) 2] error [dot code]` — the L-DO do-catch rows).
func (e *Engine) noteCollectionHazards(sig *Signature, sortedIndices []int) {
	if len(sortedIndices) == 0 || sortedIndices[0] >= e.Pointer {
		return
	}
	if sig != nil && sig.Callable != nil && sig.Callable.StripsUnconsumedInput {
		return
	}
	e.noteCollectionHazardsBelow(-1, sortedIndices[0])
}

// noteCollectionHazardsBelow marks the unapplied fn-typed values at tape
// indices in [floor, top) that belong to top's paren scope (floor -1 walks
// down to the scope's own open paren). See noteCollectionHazards.
func (e *Engine) noteCollectionHazardsBelow(floor, top int) {
	es := e.Registry.analysisRecorder()
	if !es.Active() {
		return
	}
	open := -1
	for j := top - 1; j >= 0 && j >= floor; j-- {
		if IsOpenParen(e.Tape.At(j)) {
			open = j
			break
		}
	}
	lo := open + 1
	if open >= 0 {
		if info, ok := e.Tape.At(open).Data.(FrameOpenInfo); ok && info.ArgSpan > 0 {
			lo += info.ArgSpan
		}
	}
	if lo < e.inertPrefix {
		lo = e.inertPrefix
	}
	if floor > lo {
		lo = floor
	}
	for j := lo; j < top; j++ {
		v := e.Tape.At(j)
		if v.Quoted || IsForward(v) || IsMark(v) || IsMove(v) || IsOpenParen(v) {
			continue
		}
		if IsFnValueResidual(v) || IsFnTypedCarrier(v) || (v.Dynamic && SigTypeMatches(v, TFunction)) {
			es.NoteCollectionHazard(v.ID)
		}
	}
}

// spliceMatchResults replaces the word and its matched args on the
// stack with the supplied results. Shared between handler execution
// and carrier-based check-mode execution so both paths stay in parity.
// stampCallResultPositions gives each positionless FUNCTION or dynamic-Any
// result the position of the call that produced it — the check-pass
// equivalent of the interpreter's stampResultPos, and the compiled half of
// NUR113.
//
// Both shapes are load-bearing, and neither is obvious:
//
//	FUNCTION      a module export (`Assert.not-equal`) is handed back verbatim
//	              by moduleNSGetReturns, built at module-construction time with
//	              no position, and it is the token a later VALUE dispatch reads
//	              its own position from.
//	dynamic(Any)  a dispatch-bearing field read (`m.d`) is deliberately NOT
//	              narrowed by getNodeReturns, so it arrives as a dynamic Any
//	              carrier — which is what a modifier word (`/s`, `/u`) takes as
//	              its subject, and what recordGradualWrap reads the recorded
//	              position from.
//
// Without either, every opcode downstream of a dot-access recorded 0:0 and
// the compiled lane rendered "source position unknown" where the interpreter
// has a caret. Extracted from execMatch rather than inlined: the loop pushed
// that function past the gocyclo ceiling, and it reads better named.
//
// It must run AFTER the recorder has re-IDed the outputs. Stamping earlier
// (inside CarrierResults) does set the position and is still useless — the
// re-ID replaces the value before it reaches the tape.
func stampCallResultPositions(results []Value, pos SrcPos) {
	for i := range results {
		if p := results[i].Parent; p != nil &&
			(p.Equal(TFunction) || (results[i].Dynamic && p.Equal(TAny))) {
			results[i] = WithPosAt(results[i], pos)
		}
	}
}

func (e *Engine) spliceMatchResults(match *MatchResult, sortedIndices []int, n int, results []Value) error {
	if len(sortedIndices) == n && n > 0 {
		firstArgIdx := sortedIndices[0]

		// Compact: slide non-skip elements over skip elements in
		// [firstArgIdx..pointer] to preserve internal forwards.
		skipSet := make(map[int]bool, n+1)
		for _, idx := range sortedIndices {
			skipSet[idx] = true
		}
		skipSet[e.Pointer] = true // skip the word itself

		dst := firstArgIdx
		for i := firstArgIdx; i <= e.Pointer; i++ {
			if !skipSet[i] {
				e.Tape.Set(dst, e.Tape.At(i))
				dst++
			}
		}
		// Splice out the compacted garbage, insert results.
		e.Tape.Splice(dst, e.Pointer+1-dst, results...)
		e.Pointer = firstArgIdx
	} else if n == 0 {
		// No args, just replace the word with results.
		e.Tape.Splice(e.Pointer, 1, results...)
		// Pointer stays at same position to re-examine results.
	} else {
		// Fallback: simple contiguous splice.
		argStart := e.Pointer - n
		if argStart < 0 {
			argStart = 0
		}
		e.Tape.Splice(argStart, e.Pointer+1-argStart, results...)
		e.Pointer = argStart
	}

	return nil
}

// rearrangeForForward reorders the N = stackArgs + forwardArgs resolved
// values before the current pointer to match what the unified matcher
// (post §1.4) will read with ForceStack on retry: stack-top-down is sig
// order. The first-collected forward arg is the canonical sig[0], so it
// has to sit on top of the stack. The pre-existing prefix args stay
// where they were (their order maps to sig[fwdCount..N-1] under the
// same top-down read).
//
// Before: [..., stk_0, stk_1, ..., stk_{S-1}, fwd_0, fwd_1, ..., fwd_{F-1}, WORD]
// After:  [..., stk_0, stk_1, ..., stk_{S-1}, fwd_{F-1}, ..., fwd_1, fwd_0, WORD]
//
// Under the post-rearrange layout, stack-top-down = [fwd_0, fwd_1, …,
// fwd_{F-1}, stk_{S-1}, stk_{S-2}, …, stk_0], which the unified
// matcher reads as sig[0..N-1] in order.
func (e *Engine) rearrangeForForward(stackArgs, forwardArgs int) {
	total := stackArgs + forwardArgs
	if total == 0 {
		return
	}

	indices := e.ResolvedIndicesBefore(total)
	if len(indices) < total {
		return
	}

	// Extract values in current order into a reused scratch buffer.
	values := e.rrValues[:0]
	for _, idx := range indices {
		values = append(values, e.Tape.At(idx))
	}
	e.rrValues = values // retain any grown capacity

	// Reorder into a second reused buffer: stack args stay in source
	// order; forward args go after them in REVERSED collection order so
	// fwd_0 sits at the top.
	reordered := e.rrReordered
	if cap(reordered) < total {
		reordered = make([]Value, total)
	} else {
		reordered = reordered[:total]
	}
	e.rrReordered = reordered
	for i := 0; i < stackArgs; i++ {
		reordered[i] = values[i]
	}
	for i := 0; i < forwardArgs; i++ {
		reordered[stackArgs+i] = values[total-1-i]
	}

	// Write back.
	for i, idx := range indices {
		e.Tape.Set(idx, reordered[i])
	}
}

// resolvedIndicesBefore returns the indices of the last n resolved values
// before the current pointer, stopping at open-paren barriers.
func (e *Engine) ResolvedIndicesBefore(n int) []int {
	return e.resolvedIndicesBeforeInto(nil, n)
}

// resolvedIndicesBeforeInto is resolvedIndicesBefore appending into a
// caller-supplied buffer (matchSignature's per-invocation int buffer —
// its cap must be ≥ n to stay allocation-free; a nil buf allocates as
// before). The buffer collects at most n indices, so a cap-n tail
// region never reallocates.
func (e *Engine) resolvedIndicesBeforeInto(buf []int, n int) []int {
	indices := buf
	for i := e.Pointer - 1; i >= 0 && len(indices) < n; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) || IsMark(e.Tape.At(i)) || IsMove(e.Tape.At(i)) {
			continue
		}
		indices = append(indices, i)
	}
	// Reverse so indices are in stack order (ascending).
	for i, j := 0, len(indices)-1; i < j; i, j = i+1, j-1 {
		indices[i], indices[j] = indices[j], indices[i]
	}
	return indices
}

// resolvedStackBeforeFrom returns all resolved values from position 'from'
// up to the pointer, excluding forwards, open-parens, marks, moves,
// and the matched arg indices.
func (e *Engine) resolvedStackBeforeFrom(from int, excludeIndices []int) []Value {
	exclude := make(map[int]bool, len(excludeIndices))
	for _, idx := range excludeIndices {
		exclude[idx] = true
	}
	var stack []Value
	for i := from; i < e.Pointer; i++ {
		if exclude[i] || IsForward(e.Tape.At(i)) || IsOpenParen(e.Tape.At(i)) || IsMark(e.Tape.At(i)) || IsMove(e.Tape.At(i)) {
			continue
		}
		stack = append(stack, e.Tape.At(i))
	}
	return stack
}

// insertForward records a deferred dispatch by placing a Forward
// marker after the func word on the stack. The marker carries the
// matched signature plus arg-count bookkeeping; subsequent literals
// are routed into its arg slots until ExpectedArgs is reached, at
// which point the marker triggers handler execution.
// forceStackWord rewrites the word at idx into its force-stack form
// (ForceStack=true) while preserving the source position. Used at the
// forward-collection completion points where the word must re-dispatch
// against the stack; callers set e.pointer themselves as needed.
func (e *Engine) forceStackWord(idx int, w WordInfo) {
	pos := e.Tape.At(idx).Pos()
	nw := NewWordModified(w.Name, w.ArgCount, true, false)
	nw.pos = &pos // preserve source position across force-stack rewrite
	e.Tape.Set(idx, nw)
}

func (e *Engine) insertForward(w WordInfo, sig *Signature, forwardNeeded, stackArgs, specAt int) error {
	var pos SrcPos
	if e.Pointer >= 0 && e.Pointer < e.Tape.Len() {
		pos = e.Tape.At(e.Pointer).Pos()
	}
	fwd := NewForward(ForwardInfo{
		FuncName:     w.Name,
		ExpectedArgs: forwardNeeded,
		StackArgs:    stackArgs,
		FuncIndex:    e.Pointer,
		Sig:          sig,
		Pos:          pos,
		// The plan's stop condition, threaded from matchSignature:
		// specAt >= 0 means slot specAt was planned from a word that
		// will DISPATCH rather than arrive (see ForwardInfo docs).
		Speculative:   specAt >= 0,
		SpeculativeAt: max(specAt, 0),
	})

	e.Tape.Insert(e.Pointer+1, fwd)

	e.Pointer += 2
	return nil
}

// stepLiteral handles a resolved (non-word, non-forward) value at the pointer.
// spliceExpand returns the stack entries an __SP marker contributes when it
// reaches the pointer. A plain (non-typed) list splices its top-level
// elements; every other value — scalars, maps, typed lists, tables — splices
// as a single entry. The data is returned verbatim (unevaluated); the engine
// re-steps over it after splicing.
func SpliceExpand(data Value) []Value {
	if data.Parent.Equal(TList) && data.Data != nil && !IsTypedList(data) && !IsTableType(data) {
		elems, _ := AsList(data)
		out := make([]Value, elems.Len())
		copy(out, elems.Slice())
		return out
	}
	return []Value{data}
}

// spliceIsData reports whether an __SP marker's payload expands to plain
// VALUES only — no words, paren groups, reaches, or interpolated strings at
// the top level (the level `word` splices; nested lists stay values). A data
// splice contributes the same values in any context, so the `f w ≡ f (w)`
// paren expansion of a splice-bound word in a forward-argument position is
// sound for it. A code-bearing splice is a Forth-style macro that runs
// against the LIVE stack — paren isolation would change its meaning (`def
// inc word [1 add]  1 inc inc inc` needs each `add` to see the caller's
// values) — so it stays on the existing standalone-fire / boundary-word
// paths; group explicitly (`f (p)`) to use its result as an operand.
func spliceIsData(info SpliceInfo) bool {
	for _, el := range SpliceExpand(info.Data) {
		if IsWord(el) || IsParenExpr(el) || IsReach(el) || IsInterpString(el) || IsSplice(el) {
			return false
		}
	}
	return true
}

// pendingForwardIdx returns the stack index of the nearest pending Forward
// below the pointer, stopping at an OpenParen barrier; -1 when none. This is
// the "is the value at the pointer being collected?" probe shared by
// stepLiteral's collection dispatch and the splice-word paren expansion in
// stepWord's def-substitution branch.
// The scan reads each cell ONCE: Tape.At returns a Value by value, and a
// Value is 104 bytes, so probing the same cell twice doubled this loop's
// runtime.duffcopy — measured at 36% of its own profile on a long flat
// program (design/INTERPRETER-SPEED-PLAN.10.md #1A shrank the struct; this
// halves the number of copies).
func (e *Engine) pendingForwardIdx() int {
	// No Forward anywhere on the tape ⇒ no index to find, so skip the walk.
	// This is the dominant case (a plain literal push, a dispatch with all
	// args already on the stack) and the walk it skips is O(stack depth):
	// without it, a residual-accumulating program with no paren or forward
	// barrier below the pointer — `for N [add 1 2]` — is O(N^2).
	if !e.Tape.hasForward() {
		return -1
	}
	for i := e.Pointer - 1; i >= 0; i-- {
		v := e.Tape.At(i)
		if IsOpenParen(v) {
			return -1
		}
		if IsForward(v) {
			return i
		}
	}
	return -1
}

func (e *Engine) stepLiteral() error {
	valIdx := e.Pointer

	// A ParenExpr reaching stepLiteral (nested inside a collapsing paren
	// span, where the in-place collapse loops in preEvalParens and
	// stepCloseParen fall through to here) is expanded to its marker span
	// in place and re-stepped — the surrounding loop's OpenParen handling
	// then collapses it on this engine. Without this, a nested ParenExpr
	// would be pushed unevaluated. See design/PAREN-REPRESENTATION.9.md
	// (paren-nesting Steps 2/3).
	// Step 4: a Quoted ParenExpr (codequote-captured data) and a ParenExpr
	// being collected by a raw-capture pending forward are NOT expanded —
	// they fall through to normal literal handling (pushed as data /
	// collected as the forward arg). Otherwise expand and re-step (the
	// nested-paren case).
	if IsParenExpr(e.Tape.At(valIdx)) && !e.Tape.At(valIdx).Quoted && !e.pendingForwardWantsRawParen() {
		items, _ := AsParenExpr(e.Tape.At(valIdx))
		e.Tape.Splice(valIdx, 1, e.expandParenExprScratch(items)...)
		return nil
	}
	// A Reach reaching stepLiteral (nested in a collapsing span, or a
	// collected list/map element) lowers to its get-chain in place, like a
	// ParenExpr (Reach Phase B). Quoted/raw-pending reaches fall through.
	if isEvalReach(e.Tape.At(valIdx)) && !e.pendingForwardWantsRawParen() {
		info, _ := AsReach(e.Tape.At(valIdx))
		e.Tape.Splice(valIdx, 1, expandReach(info)...)
		return nil
	}
	// A sugar marker lowers to its word expansion in place (ADR-012
	// rule 3, 2026-08-04 amendment — sugar.go), exactly like the
	// ParenExpr and Reach expansions above. Quoted markers (captured
	// data) and raw-capture collection fall through.
	if IsSugar(e.Tape.At(valIdx)) && !e.Tape.At(valIdx).Quoted && !e.pendingForwardWantsRawParen() {
		return e.stepSugar(valIdx)
	}

	// Look backwards for the nearest forward entry, stopping at open-paren barriers.
	fwdIdx := e.pendingForwardIdx()

	if fwdIdx < 0 {
		// __SP (splice) marker: replace it, unevaluated, with its payload
		// and re-step. A plain list contributes its top-level elements;
		// any other value contributes itself. The spliced content runs
		// against the live stack (Forth-style macro). No pending forward
		// means it is being processed as a standalone stack entry — the
		// only place a splice should fire (as an arg it is collected by
		// value via the TAny match in the collection branch below).
		if IsSplice(e.Tape.At(valIdx)) {
			info, _ := AsSplice(e.Tape.At(valIdx))
			// A splice over a COMPUTED (carrier) payload that MIGHT BE A LIST
			// cannot RECORD: the runtime marker SPREADS a list's elements, but
			// the analysis can only step the carrier as itself, so a compiled
			// unit would bake IDENTITY where the runtime spreads (`def xs (mk)
			// do [word xs]` compiled [7 8] against the interpreter's 7 8 — the
			// captured-carrier closure shape). A carrier PROVABLY not a list
			// (a strict Integer/Atom/None result — `word (1 add 2)`) is safe:
			// both engines contribute the value ITSELF, and the carrier keeps
			// its event provenance, so the recording is faithful. A Dynamic
			// carrier's type is a bound, not a proof — it stays poisoned.
			// Poisoning marks the program uncompilable; the check proceeds
			// with the carrier approximation, and the dyn-body backstop (or
			// the whole-program fallback) owns the shape with the
			// interpreter's own runtime semantics. Suspended analyses (a
			// ReturnsFn body run) stay silent — only a LIVE recording is
			// poisoned.
			if rec := e.Registry.analysisRecorder(); rec.Active() &&
				!IsConcrete(info.Data) && !IsBareTypeNode(info.Data) &&
				(info.Data.Dynamic || info.Data.Parent == nil ||
					TList.ConformsTo(info.Data.Parent) || info.Data.Parent.ConformsTo(TList)) {
				// REFUSAL-CLOSURE §9.2b: record the spread as an OpSpliceDyn
				// event over the payload operand — the VM spreads a DATA
				// payload exactly as the marker re-step (spliceExpand) and
				// DEFERS to the interpreter for a code-bearing one. The
				// result count is runtime-variable, so the event is variadic
				// (only the program residual absorbs it; fixed-arity
				// consumers keep refusing). An unresolvable payload keeps
				// the refusal.
				if !rec.RecordSpliceDyn(info.Data, e.Tape.At(valIdx).Pos()) {
					rec.MarkUncompilable("splice over a computed payload (runtime spread unknown at compile time)")
				}
			}
			e.Tape.Splice(valIdx, 1, SpliceExpand(info.Data)...)
			return nil
		}
		// A dispatch-modifier marker reaching the pointer standalone means
		// the preceding value was NOT a pending function (so execFnDefLiteral
		// never consumed it) — e.g. `(1 add 2)/s`. The modifier is a no-op on
		// a non-function result: drop the marker.
		if IsDispatchMod(e.Tape.At(valIdx)) {
			e.Tape.Remove(valIdx)
			return nil
		}
		// Shaped-instance-method model (Stage M2c, method_shape.go): a DYNAMIC
		// method-read carrier at the pointer — where the interpreter would
		// auto-dispatch the concrete member — models that dispatch on the
		// compile pass and records a guarded mid-stream OpCallDynMethod.
		// Declines (leaving today's paths untouched) outside a live compile
		// pass and for any window/match shape it cannot prove.
		if e.Registry.analysisActive() && CheckBraid.TryShapedMethodDispatch(e, valIdx) {
			return nil
		}
		// Container-member fn arrival-apply (REFUSAL-CLOSURE.0 §3): a
		// pinpointed member-fn read carrier whose single signature's arity
		// of inert tokens follows — model the interpreter's auto-dispatch
		// mid-expression (`m.double 21 eq 42` applies BEFORE `eq`). Declines
		// leave the carrier to today's paths (the statement-tail Finalize
		// apply, refuseStrandedMemberFn's sound refusal).
		if e.Registry.analysisActive() && CheckBraid.TryMemberFnArrivalDispatch(e, valIdx) {
			return nil
		}
		// General dynamic-fn-value dispatch (method_shape.go): a DYNAMIC carrier
		// whose bound is Function-bearing (a typed-patrun `find` result) followed
		// by an inert forward window collapses to one dynamic(Any) on the
		// plain-check surface, clearing the arg-stranding the compiled path lowers
		// via resolveDynamicApply. Declines outside plain check and for any
		// non-callable bound or non-inert window.
		if e.Registry.analysisActive() && CheckBraid.TryDynamicFnValueDispatch(e, valIdx) {
			return nil
		}
		// If the value is a Function, execute it. Quoted function
		// values are treated as data (not executed).
		val := e.Tape.At(valIdx)
		if val.Parent.Equal(TFunction) &&
			val.Data != nil && !val.Quoted {
			if _, ok := val.Data.(FnDefInfo); ok {
				return e.execFnDefLiteral(valIdx)
			}
			// A compiled closure VALUE (an island's sub-engine re-stepping
			// a shuffled `each` element, NUR124's payload axis) is the fn
			// the interpreter would have minted here: execFnDefLiteral
			// bridges it to a dispatchable FnDefInfo, or leaves it data.
			if _, ok := val.Data.(ClosurePayload); ok {
				return e.execFnDefLiteral(valIdx)
			}
		}
		// Record the literal-push event for any installed Recorder.
		// Skip engine-internal control values (markers, the recorded
		// FnDef-as-data above is handled by OnCall when it dispatches).
		if e.recorder != nil && !e.inFnFrame() && IsRecordableLiteral(val) {
			e.recorder.OnPushLit(val)
		}
		e.Pointer++
		return nil
	}

	fwd, _ := AsForward(e.Tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex

	// Check if the value matches the next expected arg positionally.
	// Once matchSignature has chosen a signature, args are collected in
	// order — no permutation or sig switching is permitted.
	//
	// Does the arriving value fill the pending slot? The decision runs on
	// the collection kernel (collect_kernel.go); the three non-collect
	// verdicts are DISPATCHES, and dispatching stays here with the host.
	// The kernel's own in-place edits — the /q Word→Atom conversion, the
	// `/v` marker consumption — are read back off the tape below.
	switch CollectArrival(e, fwd, valIdx) {
	case ArrivalDispatchFn:
		return e.execFnDefLiteral(valIdx)
	case ArrivalBarrierClose:
		if e.commitBarrierForward() {
			return nil
		}
		return e.implicitEnd(fwdIdx)
	case ArrivalImplicitEnd:
		return e.implicitEnd(fwdIdx)
	}

	// Remove the value from its current position.
	val := e.Tape.At(valIdx)
	// A collected value's reach-collapse tag is spent — the slot that
	// admitted it (a Function slot / a quoted reference) has decided the
	// call-vs-data question, and the tag must not ride into the binding.
	val.ReachGroup = false
	e.Tape.Remove(valIdx)

	// After removal, adjust indices if valIdx was before them.
	if valIdx < funcIdx {
		funcIdx--
	}
	if valIdx < fwdIdx { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		fwdIdx--
	}

	// Insert the forward value right before the function word. Forward values
	// are appended in collection order (first collected = deepest).
	// After collection the stack is:
	// [..., stack_args..., fwd0, fwd1, ..., func_word]
	// The rearrangeForForward call at completion time reorders to:
	// [..., fwd0, fwd1, ..., stack_reversed..., func_word]
	insertIdx := funcIdx

	e.Tape.Insert(insertIdx, val)

	funcIdx++
	fwdIdx++

	fwd.CollectedArgs++
	fwd.FuncIndex = funcIdx

	if e.trace != nil {
		e.traceNote = fmt.Sprintf("collect %s %d/%d",
			fwd.FuncName, fwd.CollectedArgs, fwd.ExpectedArgs)
	}

	if fwd.CollectedArgs >= fwd.ExpectedArgs {
		// All forward args collected. Remove forward, force stack, retry.
		e.Tape.Remove(fwdIdx)
		// Adjust funcIdx if forward was before it (shouldn't normally happen).
		if fwdIdx < funcIdx {
			funcIdx--
		}

		if funcIdx < e.Tape.Len() {
			if IsWord(e.Tape.At(funcIdx)) {
				w, _ := AsWord(e.Tape.At(funcIdx))
				e.forceStackWord(funcIdx, w)
			} else if v := e.Tape.At(funcIdx); isFnDefValue(v) &&
				!e.fnValueWouldWiden(v, fwd.CollectedArgs+fwd.StackArgs, funcIdx+1) {
				// A VALUE-called function (a dot-read export, a stored
				// fn, a lambda) has no WordInfo to stamp /s on. Arm the
				// one-shot seal instead, so the re-step below matches
				// stack-only exactly like the word twin — without it the
				// re-plan preferred the forward window again and a
				// completed Any collection reached across the statement
				// boundary (NUR038). EXCEPTION: a WIDER overload that
				// claims the next forward token keeps the collection
				// open (arity widening — `concat parts {sep}`); the
				// plan barrier still stops a re-plan at a reach-read
				// call head, so widening cannot cross a statement.
				e.sealFnValue, e.sealFnValueIdx = true, funcIdx
			}
		}

		// Rearrange values for forward-first matching: forward args at
		// the deep end (sigArgs[0..F-1]), stack args reversed after them.
		e.Pointer = funcIdx
		e.rearrangeForForward(fwd.StackArgs, fwd.CollectedArgs)

		// Recorder hook: emit OnPushLit for each forward-collected
		// arg now that they're laid out on the stack in the order
		// the upcoming stack-only re-dispatch will see (deepest at
		// funcIdx-F, top just below WORD at funcIdx-1). Emitting
		// deepest-first matches the replay push order — a Flatten +
		// re-run will rebuild the same stack shape.
		//
		// Stack args fired OnPushLit earlier when their source
		// literals reached the pointer (via the bare-stack branch
		// in stepLiteral). Only the forward args need this hook.
		if e.recorder != nil && !e.inFnFrame() && fwd.CollectedArgs > 0 {
			start := funcIdx - fwd.CollectedArgs
			for i := start; i < funcIdx; i++ {
				if i >= 0 && i < e.Tape.Len() {
					v := e.Tape.At(i)
					if IsRecordableLiteral(v) {
						e.recorder.OnPushLit(v)
					}
				}
			}
		}
	} else {
		e.Tape.Set(fwdIdx, NewForward(fwd))
		e.Pointer = fwdIdx + 1
	}

	return nil
}

// execFnDefLiteral handles a Function value (an FnDefInfo payload) that has landed on the
// stack without a pending forward. It tries to match the function's signatures
// against preceding resolved stack values and, if a match is found, executes
// autoEvalStack walks the final stack and auto-evaluates lists and maps
// that were created by the parser (Eval=true) and not explicitly quoted.
// Runtime-created values (from word handlers, def bodies, etc.) are
// not auto-evaluated. This is called at the end of Run().
// resolveInertTypeShape resolves the type-name Words inside an inert
// type-shaped value — a typed container's child (`[:String]`), a
// disjunct's alternatives (the `{a?:T}` desugar) — through the
// canonical cascade (ADR-012 rule 4). Auto-evaluation is consumption,
// so these resolve exactly where plain word elements resolve; quoted
// values never reach the auto-eval paths and stay opaque.
// resolveConsumedInertTypeShape resolves an inert type-shaped consumed
// arg — a typed container (`inspect [:Integer]`, `def Ints [:Integer]`)
// or a desugared disjunct — at consumption (ADR-012 rule 4), unless
// the slot asked for the raw body (NoEvalArgs / NoEvalMapArgs) or the
// value is quoted.
func (e *Engine) resolveConsumedInertTypeShape(match *MatchResult, i int) {
	if match.Args[i].Quoted {
		return
	}
	if (match.Sig.NoEvalArgs != nil && match.Sig.NoEvalArgs[i]) ||
		(match.Sig.NoEvalMapArgs != nil && match.Sig.NoEvalMapArgs[i]) {
		return
	}
	if rv, ok := e.resolveInertTypeShape(match.Args[i]); ok {
		match.Args[i] = rv
	}
}

func (e *Engine) resolveInertTypeShape(v Value) (Value, bool) {
	switch {
	case IsTypedList(v) || IsTypedMap(v):
		ci, err := AsChildType(v)
		if err != nil { //covergate:allow IsTypedList/IsTypedMap require a ChildTypeInfo payload, so AsChildType cannot fail here
			return v, false
		}
		rc := ResolveWordsDeepR(ci.Child, e.Registry)
		if ExactEqual(rc, ci.Child) {
			return v, false
		}
		// In-place Data replacement: Elements/Entries and every other
		// field survive — only the child constraint resolves.
		nd := ci
		nd.Child = rc
		out := v
		out.Data = nd
		return out, true
	case IsDisjunct(v):
		rv := ResolveWordsDeepR(v, e.Registry)
		if ExactEqual(rv, v) {
			return v, false
		}
		return rv, true
	}
	return v, false
}

func (e *Engine) autoEvalStack() error {
	for i := 0; i < e.Tape.Len(); i++ {
		val := e.Tape.At(i)
		if !val.Quoted {
			// Typed containers carry no Eval flag — resolve by shape.
			if rv, ok := e.resolveInertTypeShape(val); ok {
				e.Tape.Set(i, rv)
				continue
			}
		}
		if !val.Eval || val.Quoted {
			continue
		}
		if val.Parent.Equal(TList) && val.Data != nil && !IsTypedList(val) && !IsTableType(val) {
			result, err := e.autoEvalList(val, false)
			if err != nil {
				return err
			}
			e.Tape.Set(i, result)
		} else if val.Parent.Equal(TMap) && val.Data != nil && !IsTypedMap(val) && !IsRecordType(val) && !IsOptionsType(val) {
			result, err := e.AutoEvalMap(val, false, false)
			if err != nil {
				return err
			}
			e.Tape.Set(i, result)
		}
	}
	return nil
}

// autoEvalList evaluates the contents of a plain list in a sub-engine,
// returning a new list containing the results. For example, [1 add 2] → [3].
// consumed marks the list as a word/fn ARGUMENT being auto-evaluated (execMatch /
// execFnDefSig), as opposed to the end-of-Run residual eval (autoEvalStack).
// AutoEvalConsumedList evaluates a list operand exactly as execMatch
// does for a consumed List argument. Exported for synthesized
// composite forms whose gen-chain DEFERS a tail operand's evaluation
// until after the pending-gen placeholder bindings are installed
// (def's `gen [T] refine Record [v:T]` form): the chain signature
// suppresses execMatch's eager evaluation, and the form handler
// re-applies the tail constructor's own evaluation policy here.
func AutoEvalConsumedList(r *Registry, v Value) (Value, error) {
	return NewTop(r).autoEvalList(v, true)
}

// AutoEvalConsumedMap is AutoEvalConsumedList's map counterpart
// (autoEvalMap with consumed=true; dataMap follows the caller's
// constructor semantics — false for everything except `make`).
func AutoEvalConsumedMap(r *Registry, v Value, dataMap bool) (Value, error) {
	return NewTop(r).AutoEvalMap(v, dataMap, true)
}

// runInlineCtxRegion runs input on a pooled sub-engine, bracketing the run
// as an inline context-boundary region while an analysis pass is live
// (EmitRecorder.PushInlineCtxBoundary — NUR054): the sub-run's RUNTIME twin
// pushes a context layer (Engine.Run's Contexts Push/Pop pair), but its
// recorded events lower INLINE into the enclosing unit (OpMakeList /
// OpInterp assembly), so an ambient-context write inside it must refuse
// rather than compile one scope too shallow. Outside analysis the wrapper is
// exactly RunPooledSub — the hot interpreter path pays nothing.
func (e *Engine) runInlineCtxRegion(input []Value, elemEvalRecordable bool) ([]Value, error) {
	if !e.Registry.analysisActive() {
		return RunPooledSub(e.Registry, input, elemEvalRecordable)
	}
	es := e.Registry.analysisRecorder()
	es.PushInlineCtxBoundary()
	res, err := RunPooledSub(e.Registry, input, elemEvalRecordable)
	es.PopInlineCtxBoundary()
	return res, err
}

func (e *Engine) autoEvalList(val Value, consumed bool) (Value, error) {
	elems, _ := AsList(val)
	if elems.Len() == 0 {
		return val, nil
	}
	input := make([]Value, elems.Len())
	copy(input, elems.Slice())
	result, err := e.runInlineCtxRegion(input, e.IsTop || consumed || e.ElemEvalRecordable)
	if err != nil {
		return Value{}, err
	}
	// An evaluated element is STORED (the list is data): consume any
	// dispatch ascription here (`[(m as T)]` must not smuggle a live
	// ascription into a container — the match-time-only rule). Inert
	// type-shaped elements resolve their names here, where every other
	// element resolves.
	for i := range result {
		result[i] = StripAscribed(result[i])
		if rv, ok := e.resolveInertTypeShape(result[i]); ok {
			result[i] = rv
		}
	}
	out := NewList(result)
	// In RECORDING mode a list whose elements are COMPUTED (an event carrier, not
	// plain data — `[1 add 2]`, `[1 (2 add 3) 4]`) cannot bake as an inert const,
	// so record it as an OpMakeList assembly of the evaluated elements; otherwise
	// the list is an unresolvable residual and the program falls back. A
	// fully-literal list (`[1 2 3]`) stays inert and bakes as a pooled const.
	if e.Registry.analysisActive() {
		if es := e.Registry.analysisRecorder(); es.Armed() && !IsInertConst(out) {
			switch {
			case e.IsTop:
				// Top-level (frames==1): the canonical case, evaluated once.
				es.RecordMakeList(e.Registry, result, out, val.Pos())
			case consumed || e.ElemEvalRecordable:
				// A CONSUMED computed list ARG inside a fn body / closure
				// (`make Array [i 99]`, `f [j (g x)]`): the interpreter auto-evaluates
				// it against the LIVE def stack here, so its element locals/params
				// resolve (i → its frame slot) exactly as RecordMakeListInner re-pushes
				// them per call — OpMakeList re-assembles from operands per run, sound
				// in a nested frame (the RecordMakeMap caller already relies on this).
				// The end-of-Run RESIDUAL eval (consumed=false) is NOT recorded here:
				// a fn body returning a bare-word list (`[y y]`) raises undefined_word
				// at run time (the residual sub-engine lacks the body's def-locals), so
				// baking OpMakeList would diverge — it stays unresolved and falls back.
				// A stateful generator never reaches here: its body is a NoEval arg,
				// so execMatch does not auto-evaluate it as a data list.
				es.RecordMakeListInner(e.Registry, result, out, val.Pos())
			}
		}
	}
	return out, nil
}

// evalInterpString evaluates an interpolated string by running each
// expression part in a sub-engine, converting results to strings, and
// concatenating everything into a single string value.
func (e *Engine) evalInterpString(val Value) (Value, error) {
	parts, err := AsInterpString(val)
	if err != nil || parts == nil {
		return NewString(""), nil
	}
	s, dynamic, holes, holesOK, err := e.evalInterpParts(parts)
	if err != nil {
		return Value{}, err
	}
	if dynamic {
		// A `${expr}` part evaluated to a CARRIER — its value is only known at run
		// time (a fn param, or any native result, which check mode synthesises as a
		// carrier). The interpolated string is therefore NOT a compile-time
		// constant. When recording, lower it to OpInterp: the hole expressions
		// already recorded their own dispatch events, and OpInterp pops their
		// results and rebuilds the string at run time (byte-identical to
		// evalInterpParts). The static result is a String carrier either way; on
		// the rare unlowerable shape (a hole producing 0 or >1 values) refuse and
		// fall back. Mirrors RecordMakeMap — re-assembled per run, no const bake.
		out := NewCarrier(TString)
		if es := e.Registry.analysisRecorder(); es.Active() {
			if !holesOK || !es.RecordInterp(parts, holes, out, val.Pos()) {
				es.MarkUncompilable("interpolated string with a runtime-computed part")
			}
		}
		return out, nil
	}
	return NewString(s), nil
}

// evalInterpParts evaluates a list of InterpParts (literal segments plus
// `${expr}` holes), running each expression part in a sub-engine,
// stringifying its results, and concatenating into one string. Shared by
// evalInterpString and the XML-attribute / text evaluation in
// EvalXmlInterp.
//
// holes carries the per-hole single result value in source order, and holesOK
// reports that EVERY hole produced exactly one value (and no flow-control
// signal interrupted the walk) — the precondition evalInterpString needs to
// lower the template to OpInterp (one operand-stack value per hole). When a
// hole yields zero or several values, holesOK is false and holes is unused
// (the caller refuses compilation and falls back); the string itself is still
// built faithfully.
func (e *Engine) evalInterpParts(parts []InterpPart) (s string, dynamic bool, holes []Value, holesOK bool, err error) {
	var buf strings.Builder
	holesOK = true
	for _, part := range parts {
		if part.Expr == nil {
			buf.WriteString(part.Lit)
			continue
		}
		result, runErr := e.runInlineCtxRegion(part.Expr, false)
		if runErr != nil {
			return "", dynamic, nil, false, runErr
		}
		if len(result) == 1 {
			holes = append(holes, result[0])
		} else {
			// A hole with 0 or >1 results cannot map to one stack slot for
			// OpInterp; the string is still built below, but the template is
			// not lowerable (the caller falls back).
			holesOK = false
		}
		for _, r := range result {
			// A CHECK-MODE CARRIER part means the value is only known at
			// runtime — flag it so the caller does not bake the carrier's render
			// as a constant string. ValToString of a carrier yields a type tag
			// ("dynamic(Any)"), which the runtime never produces. The probe is
			// RECURSIVE: a hole like `${[1 add 2]}` auto-evaluates to a real
			// LIST whose element is the dispatch's carrier — the container's own
			// Carrier flag is false, but its render still embeds the type tag.
			// It stays a carrier probe specifically, NOT !IsConcrete: at run
			// time there are no carriers, but there ARE legitimate
			// non-concrete values — None and bare type literals (e.g.
			// `${typeof x}`) — which stringify to real text ("None",
			// "Integer") and must not collapse the whole interpolation to a
			// String carrier.
			if e.Registry.analysisValueCarriesCarrier(r) {
				dynamic = true
			}
			buf.WriteString(ValToString(r))
		}
		// If the expression raised a flow-control signal, stop
		// evaluating further parts. The outer Run loop will catch
		// the flag and unwind. Continuing would call sub.Run with
		// a stale flag still set and could produce observable
		// side effects from later parts.
		if e.Registry.FlowCtrl != FlowNone {
			holesOK = false
			break
		}
	}
	return buf.String(), dynamic, holes, holesOK, nil
}

// EvalXmlInterp evaluates an interpolated XML literal skeleton (Word/__XI)
// to a concrete Node/Xml value, running each embedded `${expr}` against
// the live registry — the structural analogue of evalInterpString. See
// design/XML-LITERAL.0.md §4.
func (e *Engine) EvalXmlInterp(val Value) (Value, error) {
	tmpl, err := AsXmlInterp(val)
	if err != nil {
		return Value{}, err
	}
	result, dynamic, holes, holesOK, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		return Value{}, err
	}
	if dynamic {
		// A `${expr}` hole evaluated to a NON-CONCRETE value — a carrier that
		// exists only during static analysis (a fn param, or a Node/Xml const,
		// which strips to a type-only carrier in record mode). The interpolated
		// element therefore depends on runtime data and is NOT a compile-time
		// constant: baking the check-mode tree would freeze a wrong child (a
		// carrier renders as its type tag, e.g. "Xml") and diverge from the
		// interpreter, which builds the real tree at run time. When recording,
		// lower it to OpInterpXml (REFUSAL-CLOSURE §9.2c): the hole
		// expressions already recorded their own dispatch events in traversal
		// order, and the op pops their results and rebuilds the element at
		// run time (rebuildXmlFromTmpl — byte-identical to this build). The
		// rare unlowerable shape (a 0-or-many-valued hole) keeps the refusal
		// and falls back, exactly as the string sibling.
		if es := e.Registry.analysisRecorder(); es.Active() {
			out := NewCarrier(TXml)
			out.ID = GenerateID(IDPrefixForType(TXml))
			if !holesOK || !es.RecordInterpXml(tmpl, holes, out, val.Pos()) {
				es.MarkUncompilable("interpolated XML with a runtime-computed part")
			}
			return out, nil
		}
	}
	return result, nil
}

// BuildXmlFromTmpl recursively evaluates an XmlTmpl into a Node/Xml value.
// Attribute values evaluate via evalInterpParts; children evaluate per
// XmlCren kind — a literal becomes a text node, a nested template recurses,
// and an expression hole splices per the design's rule (a List contributes
// each element, a Node/Xml is one child element, any other value becomes a
// text node). Adjacent text is merged so a `hello ${name}` run yields one
// text node rather than two.
//
// The returned dynamic flag is true when any hole (an attribute part, a child
// expression, or a nested template) evaluated to a NON-CONCRETE value — a
// carrier seen only under static analysis. EvalXmlInterp uses it to refuse
// const-folding while recording (the InterpString contract).
// BuildXmlFromTmpl additionally collects the per-hole single result values
// in TRAVERSAL order (attrs first, then children left-to-right, depth-first
// through nested elements) with holesOK reporting every hole produced
// exactly one value — the precondition RecordInterpXml needs to lower the
// element to OpInterpXml (one operand-stack value per hole), mirroring
// evalInterpParts' holes contract.
func (e *Engine) BuildXmlFromTmpl(t XmlTmpl) (Value, bool, []Value, bool, error) {
	dynamic := false
	holesOK := true
	var holes []Value
	attr := NewOrderedMap()
	for _, a := range t.Attr {
		s, dyn, aHoles, aOK, err := e.evalInterpParts(a.Parts)
		if err != nil {
			return Value{}, false, nil, false, err
		}
		if dyn {
			dynamic = true
		}
		if !aOK {
			holesOK = false
		}
		holes = append(holes, aHoles...)
		attr.Set(a.Name, NewString(s))
		if e.Registry.FlowCtrl != FlowNone {
			return NewXmlElement(t.Tag, attr, nil), dynamic, holes, false, nil
		}
	}

	var cren []Value
	addText := func(s string) {
		if s == "" {
			return
		}
		if n := len(cren); n > 0 {
			if sp, ok := cren[n-1].Data.(StrPayload); ok {
				cren[n-1] = NewString(sp.S + s)
				return
			}
		}
		cren = append(cren, NewString(s))
	}
	// addOne adds a single evaluated value as one child node: an XML
	// element (immutable Node/Xml OR mutable FlexXml) becomes a child
	// element, any other value becomes a text node. A non-concrete result
	// (a carrier under static analysis) marks the build dynamic — its
	// ValToString render ("Xml", "dynamic(Any)") is a check-mode artefact
	// the runtime never produces, so the element must not const-fold.
	addOne := func(r Value) {
		if IsXmlValue(r) {
			cren = append(cren, r)
			return
		}
		if !IsConcrete(r) {
			dynamic = true
		}
		addText(ValToString(r))
	}
	// addChild applies the ${...} child-hole splice rule: a List
	// contributes each of its TOP-LEVEL elements (one level only — a
	// nested list element is kept as a single value, not flattened, the
	// same as `append [..]` on a FlexXml); any other value is one child.
	addChild := func(r Value) {
		if IsConcrete(r) && r.Parent.ConformsTo(TList) {
			if rl, err := AsList(r); err == nil {
				for i := 0; i < rl.Len(); i++ {
					addOne(rl.Get(i))
				}
				return
			}
		}
		addOne(r)
	}

	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenLit:
			addText(c.Lit)
		case XmlCrenChild:
			if c.Child == nil {
				continue
			}
			child, dyn, cHoles, cOK, err := e.BuildXmlFromTmpl(*c.Child)
			if err != nil {
				return Value{}, false, nil, false, err
			}
			if dyn {
				dynamic = true
			}
			if !cOK {
				holesOK = false
			}
			holes = append(holes, cHoles...)
			cren = append(cren, child)
		case XmlCrenExpr:
			// An inline context-boundary region exactly like an interp-string
			// hole: the child hole's runtime twin is a sub-engine (a context-
			// layer push), but its events lower inline into OpInterpXml's
			// enclosing unit (NUR054 — the attribute holes ride evalInterpParts
			// and are bracketed there).
			results, err := e.runInlineCtxRegion(c.Expr, false)
			if err != nil {
				return Value{}, false, nil, false, err
			}
			if len(results) == 1 {
				holes = append(holes, results[0])
			} else {
				// A 0-or-many-valued child hole cannot map to one stack
				// slot for OpInterpXml; the element is still built below,
				// but the template is not lowerable (the caller falls back).
				holesOK = false
			}
			for _, r := range results {
				addChild(r)
			}
			if e.Registry.FlowCtrl != FlowNone {
				return NewXmlElement(t.Tag, attr, cren), dynamic, holes, false, nil
			}
		}
	}
	return NewXmlElement(t.Tag, attr, cren), dynamic, holes, holesOK, nil
}

// expandParenExpr returns a ParenExpr's tokens wrapped in OpenParen …
// CloseParen markers — the span the existing in-place collapse machinery
// (stepCloseParen / preEvalParens) evaluates. Used to expand a word-context
// ParenExpr value back to markers on encounter (paren-nesting Steps 2/3),
// keeping exact parity with the former marker representation. See
// design/PAREN-REPRESENTATION.9.md.
func expandParenExpr(items []Value) []Value {
	span := make([]Value, 0, len(items)+2)
	span = append(span, NewOpenParen())
	span = append(span, items...)
	span = append(span, NewCloseParen())
	return span
}

// expandParenExprScratch is expandParenExpr into the engine's reusable
// span buffer, for the call sites that Splice the span into the tape
// immediately (the splice copies the tokens, freeing the buffer). Not
// for spans handed to a sub-engine run — those use the allocating
// variant above.
func (e *Engine) expandParenExprScratch(items []Value) []Value {
	span := append(e.peScratch[:0], NewOpenParen())
	span = append(span, items...)
	span = append(span, NewCloseParen())
	e.peScratch = span
	return span
}

// lowerReach turns a Reach into its equivalent get/getr chain tokens:
// `recv get k1 getr k2 …`. A computed segment's key becomes a ParenExpr so
// the chain evaluates it before get/getr consumes it. This is the Stage-1
// evaluator (design/REACH.10.md §4): the chain is identical to the former
// dot-access ParenExpr, so wrapping it with expandParenExpr and running it
// in place reproduces exact get/getr semantics.
func lowerReach(info ReachInfo) []Value {
	out := make([]Value, 0, len(info.Receiver)+len(info.Segments)*2)
	out = append(out, info.Receiver...)
	var anchor Value
	if len(info.Receiver) > 0 {
		anchor = info.Receiver[0]
	}
	for _, seg := range info.Segments {
		if seg.Getr {
			out = append(out, WithPos(NewWord("dotr"), anchor))
		} else {
			out = append(out, WithPos(NewWord("dot"), anchor))
		}
		if seg.Computed {
			out = append(out, NewParenExpr(seg.KeyExpr))
		} else {
			out = append(out, seg.KeyLit)
		}
	}
	return out
}

// isEvalReach reports whether v is a Reach that should auto-evaluate now:
// a parsed dot-access (Eval=true) that has not been quote-captured. An inert
// constructor-built reach (Eval=false, from `reach …`) or a codequote'd one
// (Quoted) is data and is left alone.
func isEvalReach(v Value) bool {
	if !IsReach(v) || v.Quoted {
		return false
	}
	info, _ := AsReach(v)
	return info.Eval
}

// expandReach returns the marker span an Eval Reach evaluates to — its
// lowered get-chain wrapped in paren markers, run in place exactly like a
// ParenExpr (Stage-1 lowering).
func expandReach(info ReachInfo) []Value {
	span := expandParenExpr(lowerReach(info))
	// Tag the group as reach-lowered so a function value it resolves to does
	// not dispatch inside it (execFnDefLiteral). The group exists to scope
	// the get-chain and to let a `/` modifier apply to the whole path; it is
	// not a call site, and treating it as one is what made `Mod.word arg`
	// match against an empty argument window (NUR035).
	span[0].ReachGroup = true
	return span
}

// ApplyReach evaluates a Reach against a concrete receiver value — the lens
// "get" (design/REACH.10.md §7). The reach's own Receiver tokens are ignored
// (a receiverless lens has none); recv becomes the base and the segments
// (get/getr, literal/computed, in order) walk from it via the same Stage-1
// lowering bare m.a.b uses, so getr strictness and computed-key evaluation
// are identical. It is the primitive behind the `apply` word and the
// receiverless-reach-as-Function higher-order behaviour.
func ApplyReach(r *Registry, info ReachInfo, recv Value) (Value, error) {
	// The compiled lane first: a stamped lens runs its unit on the VM instead of
	// re-entering the interpreter for every application (reach_unit.go). ran is
	// false for an unarmed registry, a lens whose body declined, and a unit that
	// bailed with no observable effect — all of which keep the chain below.
	if sig := compiledLensSig(r, info.unit, info.Segments); sig != nil {
		vres, verr, ran := compiledRuntime.InvokeCompiled(r, sig, []Value{recv})
		if ran {
			return lastReachResult(vres, verr)
		}
		// A non-nil verr here says the unit RAN and DEFERRED (`5 $.name apply`:
		// CALL_NATIVE_POLY finds no `dot` for an Integer receiver and defers for
		// the canonical signature_error). The chain below is that defer's
		// replay, not an island — see bailReplayAttribution.
		defer bailReplayAttribution(r, verr)()
	}
	toks := lowerReach(ReachInfo{Receiver: []Value{recv}, Segments: info.Segments})
	res, err := RunPooledSub(r, expandParenExpr(toks), false)
	return lastReachResult(res, err)
}

// lastReachResult is the shared tail of ApplyReach's two lanes: an error wins,
// an empty result is the lens finding nothing to hand back, and otherwise the
// chain's last value is the read. Shared so the compiled lane cannot drift from
// the interpreted one on what "the value of a lens" means.
func lastReachResult(res []Value, err error) (Value, error) {
	if err != nil {
		return Value{}, err
	}
	if len(res) == 0 {
		return Value{}, fmt.Errorf("apply: reach produced no value")
	}
	return res[len(res)-1], nil
}

// evalParenExprResults evaluates a ParenExpr's tokens in a sub-engine and
// returns its result value(s). Used by autoEvalMap for paren values in map
// (data) context, where a single result value is collected for a key. It
// shares the registry (defs leak) and propagates errors (a paren is not an
// error boundary, unlike `do`).
func (e *Engine) evalParenExprResults(items []Value) ([]Value, error) {
	return RunPooledSub(e.Registry, expandParenExpr(items), false)
}

// autoEvalMap evaluates each value in a plain map using a sub-engine.
// Word values resolve directly; lists auto-evaluate via autoEvalStack:
//
//	{r:rv}        → {r:10}      (word evaluated to its def'd value)
//	{x:[1 add 2]} → {x:[3]}     (list evaluated, stays as list)
//	{a:[1,2]}     → {a:[1,2]}   (literal list unchanged)
//	{x:"hello"}   → {x:"hello"} (strings pass through unchanged)
//
// consumed reports whether the map is being evaluated as an in-frame CONSUMED
// argument (a word/fn-def arg) rather than a DEFERRED residual (autoEvalStack
// at end of run). Only a consumed map may record an OpMakeMap event: a deferred
// residual is evaluated LATE, after its enclosing fn frame has popped, so its
// value bindings (a fn param) are gone — recording it in-frame would diverge
// from the interpreter (which errors / re-binds at the later time).
func (e *Engine) AutoEvalMap(val Value, dataMap, consumed bool) (Value, error) {
	m, _ := AsMutableMap(val)
	out := NewOrderedMap()
	if m.Implicit {
		out.Implicit = true
	}

	// Computed keys: evaluate key expressions at runtime.
	var ckSet map[string]bool
	if m.Meta != nil {
		ckSet, _ = m.Meta["ck"].(map[string]bool)
	}

	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		resolvedKey := key

		// Computed key: evaluate the key text as boru code to get
		// the actual string key. E.g., {[a]:1} with def a 'x' → {x:1}
		if ckSet[key] {
			keyResult, err := RunPooledSub(e.Registry, []Value{NewWord(key)}, false)
			if err != nil {
				return Value{}, fmt.Errorf("computed key [%s]: %w", key, err)
			}
			if len(keyResult) == 1 {
				if keyResult[0].Parent.ConformsTo(TString) {
					resolvedKey, _ = AsString(keyResult[0])
				} else if IsAtom(keyResult[0]) {
					resolvedKey, _ = AsAtom(keyResult[0])
				} else {
					resolvedKey = ValToString(keyResult[0])
				}
			}
		}

		// Interpolated string: evaluate inline.
		if IsInterpString(v) {
			result, err := e.evalInterpString(v)
			if err != nil {
				return Value{}, err
			}
			out.Set(resolvedKey, result)
			continue
		}

		// Interpolated XML literal as a map value: evaluate inline to a
		// Node/Xml, mirroring the InterpString case.
		if IsXmlInterp(v) {
			result, err := e.EvalXmlInterp(v)
			if err != nil {
				return Value{}, err
			}
			out.Set(resolvedKey, result)
			continue
		}

		// Paren expression: evaluate items as an isolated sub-expression
		// (shared via evalParenExprResults with the main-stack path).
		if IsParenExpr(v) {
			items, _ := AsParenExpr(v)
			// CHECK-MODE const-fold: a computed container value (a class field
			// default like (make Foo 1), or a data-map (1 add 2)) evaluated
			// abstractly leaves a recorded event the container then swallows
			// ("unconsumed call results"), so the program refuses. When the
			// expression is DETERMINISTIC it is a compile-time constant — fold
			// it to its concrete value so the container bakes as a const. The
			// downstream const-bake gate (typeBodyConstOK for a schema default,
			// isInertConst for a data map) decides mutation-safety, so an
			// instance still bakes only where `make` copies it per instance.
			// ONLY at the TOP frame: a container inside a fn
			// body / for body / closure (`for 3 [{a: (3 mul i)} get a]`) is
			// RE-EVALUATED per call or iteration, often with a different binding
			// (the loop iterator `i`). The fold's determinism check (two equal
			// concrete evals) does NOT catch that — `i` is stable WITHIN the fold —
			// so freezing the value would replicate it across iterations. Those
			// keep refusing and fall back (mirrors the OpMakeList gate).
			// The expression must also not REFERENCE a CARRIER binding (a def-local
			// bound to a computed value, `def v0 (0 add 3) ... {a: (5 mul v0)}`): the
			// concrete fold coerces the carrier (e.g. to 0) and freezes a WRONG value
			// (the determinism check sees the same coerced 0 twice). exprRefsCarrier
			// catches that; a user TYPE binding (Carrier=false) still folds.
			topFrame := e.Registry.analysisRecorder().TopFrameOnly()
			if e.Registry.analysisActive() && topFrame && !CheckBraid.ExprRefsCarrier(e, items) {
				if folded, ok := e.constFoldContainerVal(items); ok {
					// Bake the computed value as a const EXCEPT in a `make`
					// construction body (dataMap) when the value is shared-mutable: a
					// data-map instance is stored VERBATIM by make (MakeClassFieldValue),
					// so a baked const would alias across runs. Leave it to the recording
					// eval below — its make event records, and RecordMakeMap re-assembles
					// the map per run. A SCHEMA default (dataMap=false) still folds + bakes
					// as a template that make's FreshenDefault copies, unchanged.
					if (!dataMap && !e.ElemEvalRecordable) || !containsSharedMutable(folded) {
						out.Set(resolvedKey, folded)
						continue
					}
				}
			}
			result, err := e.evalParenExprResults(items)
			if err != nil {
				return Value{}, err
			}
			if len(result) == 1 {
				out.Set(resolvedKey, result[0])
			} else if len(result) > 1 {
				out.Set(resolvedKey, NewList(result))
			}
			continue
		}

		// Reach map value (e.g. {x: m.a}) — evaluate its lowered get-chain
		// as an isolated sub-expression, like a ParenExpr (Reach Phase B).
		if isEvalReach(v) {
			info, _ := AsReach(v)
			result, err := e.evalParenExprResults(lowerReach(info))
			if err != nil {
				return Value{}, err
			}
			if len(result) == 1 {
				out.Set(resolvedKey, result[0])
			} else if len(result) > 1 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				out.Set(resolvedKey, NewList(result))
			}
			continue
		}

		// CHECK-MODE const-fold for a bare value — the mirror of the ParenExpr
		// fold above for the un-parenthesised form. A bare word that is a 0-arg
		// fn auto-fires as a map value (`{a:g}` → `{a:42}`, exactly like the
		// parenthesised `{a:(g)}`); evaluated abstractly in the sub-engine below
		// it leaves a carrier of unknown provenance and the map refuses. When the
		// value is DETERMINISTIC at the top frame and references no carrier
		// binding, fold it to its concrete result (identical to the sub-engine
		// eval the interpreter runs) so the map bakes as a const. Same gating and
		// mutation-safety screen as the ParenExpr branch.
		if e.Registry.analysisActive() &&
			e.Registry.analysisRecorder().TopFrameOnly() && !CheckBraid.ExprRefsCarrier(e, []Value{v}) {
			if folded, ok := e.constFoldContainerVal([]Value{v}); ok {
				if (!dataMap && !e.ElemEvalRecordable) || !containsSharedMutable(folded) {
					out.Set(resolvedKey, folded)
					continue
				}
			}
		}

		// An inert type-shaped value (`{a?:T}`'s desugared disjunct, a
		// typed-container constraint) has no dispatches to run — it
		// resolves its names directly.
		if rv, ok := e.resolveInertTypeShape(v); ok {
			out.Set(resolvedKey, rv)
			continue
		}
		// Evaluate each value in a pooled sub-engine.
		result, err := RunPooledSub(e.Registry, []Value{v},
			e.IsTop || consumed || e.ElemEvalRecordable)
		if err != nil {
			return Value{}, err
		}
		if len(result) == 1 {
			out.Set(resolvedKey, result[0])
		} else if len(result) > 1 {
			out.Set(resolvedKey, NewList(result))
		}
		// RECORDING mode: a LIST-valued entry (`{n:[expr]}` — the `do {map}` idiom,
		// where each value is a code list left AS a list) is a COMPUTED list whose
		// elements have provenance (their dispatches recorded above) but whose list
		// WRAPPER was never recorded — RecordMakeList's top-frame guard refuses a
		// fn-body list. Record the OpMakeList HERE, inline, right after this value's
		// dispatches, so the assembly is interleaved with the per-value events in
		// stack order (`get_n, wrap_n, get_m, wrap_m, …`) — wrapping in RecordMakeMap
		// afterward would find the next value's result on top. Sound (and gated like
		// the enclosing OpMakeMap) only when the map is CONSUMED in-frame: OpMakeList
		// re-assembles from its operands per run, so it never freezes a per-call
		// binding. RecordMakeListInner declines (leaving es untouched) for an
		// unresolvable / stateful / type-pattern element, so the map then falls back.
		if (consumed || e.ElemEvalRecordable) && e.Registry.analysisActive() {
			if es := e.Registry.analysisRecorder(); es.Armed() {
				if lv, _ := out.Get(resolvedKey); lv.Parent.Equal(TList) && !IsInertConst(lv) {
					if lp, isList := lv.Data.(ListPayload); isList {
						es.RecordMakeListInner(e.Registry, lp.Elems, lv, lv.Pos())
					}
				}
			}
		}
	}
	// An evaluated map value is STORED (the map is data): consume any
	// dispatch ascription (`{a:(m as T)}`), mirroring autoEvalList's
	// element strip — the match-time-only rule.
	for _, k := range out.Keys() {
		if mv, ok := out.Get(k); ok && mv.AscribedType() != nil {
			out.Set(k, StripAscribed(mv))
		}
	}
	res := NewMap(out)
	// RECORDING mode: a `make` construction body whose values are COMPUTED (an
	// event carrier, not bakeable as an inert const) records an OpMakeMap assembly
	// of the evaluated values, so the map resolves to a real per-run event the
	// outer make consumes — otherwise it is an unresolvable residual and the
	// program falls back. A fully-literal / const-folded map stays inert and bakes
	// as a pooled const. dataMap is set ONLY for make's construction body (a
	// CONSUMED arg, match.Name == "make" at the execMatch call site), which make
	// evaluates IN the current scope before constructing — so it records even
	// inside a fn body / branch arm (the OpMakeMap re-assembles per call from its
	// operands, matching make's per-call evaluation). It is never a deferred
	// residual, so the late-rebinding hazard that gates plain residual lists/maps
	// does not apply, and the top-frame restriction is unnecessary here.
	// A COMPUTED (non-inert) map literal records an OpMakeMap assembly so its
	// result is a real per-run event the rest of the program can consume — not
	// just make's construction body (dataMap), but ANY computed map literal,
	// including one returned as a fn body result or bound to a value-def local
	// (`def m {x:(a 1 add)} … m`). The const case (a fully-literal map) stays
	// inert and bakes as a pooled const (isInertConst), so it never reaches here.
	// OpMakeMap re-assembles a FRESH map from its operands on every run, so it is
	// sound inside a fn body / branch arm / loop body (no freezing, never a
	// deferred-residual late-rebind hazard) — the same property that made the
	// dataMap case safe without a top-frame restriction. RecordMakeMap leaves es
	// untouched and returns false when a value has no compiled home, so the map
	// stays an unresolvable residual and the program falls back faithfully.
	// Gated on `consumed`: a DEFERRED residual (end-of-run autoEvalStack) is
	// evaluated after its frame pops, so recording it in-frame would diverge.
	if (consumed || e.ElemEvalRecordable) && e.Registry.analysisActive() {
		if es := e.Registry.analysisRecorder(); es.Armed() && !IsInertConst(res) {
			keys := out.Keys()
			vals := make([]Value, len(keys))
			for i, k := range keys {
				vals[i], _ = out.Get(k)
			}
			es.RecordMakeMap(e.Registry, keys, vals, out.Implicit, res, val.Pos())
		}
	}
	return res, nil
}

// exprHasEffect reports whether items contain a word whose evaluation performs a
// side effect that the const-fold (which re-runs the expression off the emit
// path) must not double or strand: the effectful core word `print`, or a
// module-bound word fronting an IO/Net/etc. call. The walk mirrors
// exprRefsCarrier's structural recursion.
func (e *Engine) exprHasEffect(items []Value) bool {
	r := e.Registry
	found := false
	var walk func(vs []Value)
	walk = func(vs []Value) {
		for _, v := range vs {
			if found {
				return
			}
			if IsWord(v) {
				if w, err := AsWord(v); err == nil {
					if w.Name == "print" {
						found = true
						return
					}
					if bound, ok := r.Defs.Top(w.Name); ok && IsModuleFamilyValue(bound) {
						found = true
						return
					}
				}
			}
			if IsParenExpr(v) {
				if toks, err := AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			if IsReach(v) {
				if ri, err := AsReach(v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Slice())
				continue
			}
			if mp, err := AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]Value{mv})
				}
			}
		}
	}
	walk(items)
	return found
}

// constFoldContainerVal evaluates a container's computed value (a class field
// default or a data-map paren-expr) CONCRETELY at check time and returns the
// constant, so the container bakes as a const instead of recording an event the
// container swallows. It only folds a DETERMINISTIC expression: the items are
// evaluated twice in a throwaway non-recording sub-engine, and the fold is taken
// only when both runs yield the SAME single deeply-concrete value — so a
// clock/rand/mutation-bearing default (whose two runs differ) is left to the
// normal recording path and stays uncompiled rather than freezing a runtime
// value. A reference to a check-mode value binding evaluates to a carrier (not
// concrete) and so does not fold. Mutation-safety of the folded value is the
// downstream const-bake gate's job (a mutable instance bakes only as a schema
// default that make copies, never as a data-map member).
func (e *Engine) constFoldContainerVal(items []Value) (Value, bool) {
	// The fold runs the expression CONCRETELY (twice). That is only sound for a
	// pure computation: an effectful word (`print`, a module IO/Net call) would
	// perform its effect at compile time — and double it, across the two runs —
	// while the compiled program just pushes the folded constant. Decline the
	// fold for such an expression and let it record normally instead.
	if e.exprHasEffect(items) {
		return Value{}, false
	}
	one, ok := CheckBraid.ConcreteEvalOnce(e, items)
	if !ok {
		return Value{}, false
	}
	two, ok := CheckBraid.ConcreteEvalOnce(e, items)
	if !ok || !ConstFoldAgrees(one, two) {
		return Value{}, false
	}
	return one, true
}

// the function. If the FnDef carries a captured Registry (closure from a
// module), execution happens in a sub-engine using that registry so that
// module-internal words are available. Otherwise, body tokens are spliced
// into the current engine's stack.
// takeAppliedMark consumes `apply`'s one-shot application signal
// (FnDefInfo.Applied) from the value at valIdx, returning the cleared value
// and whether the mark was set.
//
// The mark answers "did someone ask to CALL this value here?", which is a
// different question from "is this value data?" — the one
// execFnDefLiteral's inert-lambda gate answers from origin. Keeping them
// separate is what lets that gate stay (it is what parks `f/v` and holds a
// lambda in a container) while an explicit `apply` still dispatches at
// arity 0, whatever the fn's origin (ADR-016, NUR077 §5 Hole 1).
//
// Clearing is unconditional and happens HERE, before any branch can store
// the value: the mark targets the immediately-following dispatch only, so
// it must never ride into a binding or a container. Same one-shot
// discipline as ReachGroup and the sealFnValue seal.
func (e *Engine) takeAppliedMark(valIdx int, val Value) (Value, bool) {
	fd, isFn := val.Data.(FnDefInfo)
	if !isFn || !fd.Applied {
		return val, false
	}
	fd.Applied = false
	val.Data = fd
	e.Tape.Set(valIdx, val)
	return val, true
}

func (e *Engine) execFnDefLiteral(valIdx int) error {
	// Consume the one-shot completion seal (armed by the collection-
	// completion site when the callee is a VALUE, not a word). Consumed
	// unconditionally — the seal targets the immediately-following
	// re-step only, so a stale flag must never leak to a later value at
	// the same index.
	sealed := e.sealFnValue && e.sealFnValueIdx == valIdx
	e.sealFnValue = false

	val := e.Tape.At(valIdx)
	// The reach-collapse tag is spent the moment the call-vs-data
	// decision is made here — it must never ride into a binding or a
	// container (the Quoted transience discipline).
	if val.ReachGroup {
		val.ReachGroup = false
		e.Tape.Set(valIdx, val)
	}
	// `apply`'s one-shot application signal is spent by the same rule and
	// for the same reason — see takeAppliedMark. The gate below reads the
	// returned local, never the field, which by then is already cleared.
	val, applied := e.takeAppliedMark(valIdx, val)
	val, fnDef, ok := e.fnDefAtPointer(val)
	if !ok {
		e.Pointer++
		return nil
	}

	// A PAREN EXPRESSION RESOLVING TO A FUNCTION WORD NEVER INDUCES A CALL.
	//
	// The group's job is to produce the value; the call belongs to whatever
	// encloses the group. This matters because `Mod.word arg` lowers to
	// `( Mod dot word ) arg` (lowerReach → expandReach), so a module word's
	// resolved fn value always lands alone inside a synthetic group with its
	// argument on the far side of the close paren — a forward-scan boundary —
	// and the open paren is a stack barrier hiding a prefix operand the other
	// way. Dispatching here matched against an argument window that is empty
	// by construction: a single-signature word came out unscathed (nothing
	// matched, so it stayed data and escaped), but a word with a 0-arg
	// overload matched THAT, silently, and its arg-taking signatures became
	// unreachable — `IO.env "HOME"` answering with the whole environment
	// (NUR035).
	//
	// Deferring costs nothing and needs no lookahead: stepCloseParen removes
	// both markers and sets the pointer back to the group's position, so the
	// value is re-stepped by the main loop in the enclosing context, where
	// its arguments ARE visible, and dispatches there under the ordinary
	// rules. Nested groups simply peel one layer per step.
	//
	// The test is "alone inside a REACH-LOWERED group", and it is O(1) — the
	// tokens either side are the group's own markers, and the marker itself
	// says who wrote it. Both halves are load-bearing. A group the USER wrote
	// is their call and dispatches here as it always has (`(g)` means "call
	// g"); only the synthetic group a dot-chain expands to is a pure
	// resolution step with nothing to call yet, which is what the tag
	// (expandReach) distinguishes with no lookahead. And a group with other
	// content (`(Mod.f a b)`) is a real call whatever emitted it. Peeling
	// happens exactly once, because collapsing the group discards the marker
	// along with it.
	// EXCEPTION: a 0-ARG-ONLY function has no arg-taking overload the
	// empty window could shadow — its only call form IS nullary, so the
	// dot-read is a PROPERTY call (`TimeUtil.today-utc`, `IO.stdin`) that
	// dispatches right here and the group collapses to its result. (The
	// deferral exists to keep a 0-arg OVERLOAD from silently eclipsing
	// the arg-taking signatures — `IO.env "HOME"` answering with the
	// whole environment, NUR035 — which cannot happen when there are no
	// arg-taking signatures to eclipse.) The exception itself yields to
	// an EXPLICIT modifier: a Word/__DM marker after the group's close
	// paren (`m.z/v`, `m.z/q`) states data intent, and dispatching here
	// would consume the fn before the post-collapse marker peek could
	// see it — so a marked 0-arg read defers like any other fn.
	if valIdx > 0 && valIdx+1 < e.Tape.Len() &&
		e.Tape.At(valIdx-1).ReachGroup && IsOpenParen(e.Tape.At(valIdx-1)) &&
		IsCloseParen(e.Tape.At(valIdx+1)) &&
		(!FnValueOnlyZeroArgSigs(fnDef) || e.dispatchModAt(valIdx+2)) {
		e.Pointer++
		return nil
	}

	// A macro FnDef reaching here is a VALUE, not an application — e.g. the
	// `(macro …)` result being bound by `def`, or a parked macro. It must
	// stay as DATA: a macro is applied only by name (the stepWord branch
	// captures its raw operands before collection). The anonymous-0-arg
	// short-circuit below also returns macros as data. (Applying a macro is
	// never a stack-value dispatch — design/MACROS-PHASE1.10.md §5, D4.)

	// Resolve the dispatchable signatures. A self-contained Function value
	// (an anonymous closure, or a fn defined in THIS registry) is a STABLE
	// handle: we compile and use its OWN signatures rather than a fresh
	// registry lookup, so `undef foo; def foo …` doesn't change the meaning
	// of a previously-captured value. compileFnDef normalises and barrier-
	// resolves the authored sigs (it is idempotent on already-compiled
	// ones) and attaches the body-runner for anonymous fns.
	//
	// A value carrying a FOREIGN sub-registry (a module wrapper, or a boru
	// fn defined inside a module preamble) is NOT a stable own-sig handle:
	// it must resolve the real (inner) definition in that sub-registry, so
	// it falls through to the name-lookup branch below. Anonymous closures
	// keep their own sigs even when they captured a module registry.
	var fn *FnDefInfo
	foreignReg := fnDef.Registry != nil && fnDef.Registry != e.Registry
	if len(fnDef.Signatures) > 0 && (fnDef.Anonymous || !foreignReg) {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.Registry
		}
		fn = compileFnDef(reg, fnDef)
	}
	if fn == nil && fnDef.Name != "" {
		reg := fnDef.Registry
		if reg == nil {
			reg = e.Registry
		}
		fn = reg.Lookup(fnDef.Name)
	}
	if fn == nil && len(fnDef.Signatures) > 0 {
		// An UNNAMED value from a foreign sub-registry — e.g. an
		// anonymous `fn` literal placed in a module export map. There
		// is nothing to look up, so its authored sigs are the only
		// description there is: compile and dispatch on them. The
		// body still runs with module scope — execFnDefSig /
		// ExecFnDefSigStackMatch receive fnDef.Registry, and the
		// sub-registry branch below handles handler-bearing matches.
		fn = compileFnDef(fnDef.Registry, fnDef)
	}
	if fn == nil {
		e.Pointer++
		return nil
	}

	w := WordInfo{Name: fnDef.Name, ArgCount: -1}
	if sealed {
		// The value's own forward collection just completed and laid its
		// args out in stack form beneath it — match stack-only and commit,
		// the word twin's /s retry. Without this the forward-first re-plan
		// reopened the window over the NEXT statement's tokens (NUR038).
		w.ForceStack = true
	}

	// A `/v` or `/q` modifier on a paren / dotted-path result is emitted by
	// the parser as a Word/__DM marker right after the group (/u /s /f /N
	// are the usurp / stack-args / forward-args / force-arity words). Peek
	// and consume it: it leaves the function inert (data).
	if valIdx+1 < e.Tape.Len() {
		if _, ok := AsDispatchMod(e.Tape.At(valIdx + 1)); ok {
			e.Tape.Remove(valIdx + 1)
			v := e.Tape.At(valIdx)
			v.Quoted = true
			e.Tape.Set(valIdx, v)
			e.Pointer++
			return nil
		}
	}

	// Pre-evaluate paren expressions in the forward scan range so that
	// matchSignature sees fully resolved values. Mirrors stepWord's
	// pre-eval pass — the unified rule says Function values at the
	// pointer dispatch with the same forward+stack matching as words.
	if (fn.HasForwardSigs() && !w.ForceStack) || w.ForceForward {
		if err := e.resolveForwardArgs(fn, w); err != nil {
			return err
		}
	}

	resolved := e.EffectiveResolved()
	sig, positions, specAt := e.MatchSignature(fn, w, resolved)

	// Retry fallback for words with forward-collecting sigs: when
	// nearest-first matching fails, retry with deepest-first
	// (ForceStack). Mirrors stepWord's CallBoru-input recovery.
	if sig == nil && fn.HasForwardSigs() && !w.ForceStack {
		wDeep := w
		wDeep.ForceStack = true
		sig, positions, specAt = e.MatchSignature(fn, wDeep, resolved)
	}

	// Function-value dispatch does NOT fire Fallback sigs. Fallback
	// handlers (installed by InstallFnDef as 0-arg catch-alls) exist
	// to raise a clean "no matching signature for X" error when a
	// *bare word* arrives without args. For a Function value sitting
	// at the pointer, the right behavior is to leave it on the stack
	// as data — the user explicitly captured it and may consume it
	// later.
	if sig != nil && sig.Fallback {
		sig = nil
	}

	// Count forward vs stack positions of the matched sig (nil-safe:
	// positions is empty when sig == nil).
	fwdCount := 0
	for _, pos := range positions {
		if pos > e.Pointer {
			fwdCount++
		}
	}

	// Fall through to FnSig-based pure-stack matching when
	// matchSignature finds nothing — this preserves the legacy
	// anonymous-fn-on-stack dispatch for boru fns whose Sigs carry
	// named params. The same path runs when matched but the sig
	// has no Go Handler AND this isn't an `afn`-produced lambda:
	// predicate-type FnDefs landing bare are intentionally inert.
	//
	// EXCEPT when the handler-less match includes FORWARD positions:
	// a non-anonymous `fn` value at the pointer (typically a `get`
	// result — `m.f 5`, `Mod.word arg`) must collect its forward args
	// like any word (the unified rule above). It proceeds to the
	// insertForward branch below, which parks the future tokens and
	// re-processes this value with all args on the stack — landing
	// back here with fwdCount == 0 and dispatching via the stack
	// path. Macro values are excluded: they stay on the legacy path
	// (data — applied only by name; MACROS-PHASE1.10.md §5).
	if sig == nil || (sig.DispatchHandler() == nil && !fnDef.Anonymous && (fwdCount == 0 || fnDef.Macro)) {
		return e.ExecFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Anonymous lambdas (afn / =>) are VALUES that auto-dispatch only
	// when args are actually available (forward tokens, or stack args
	// for the infix form). A 0-arg lambda sitting alone on the stack
	// has positions=[] AND no forward — it's just data, let downstream
	// consumers (def, a stored map entry, call) take it as-is rather
	// than auto-invoking. This is what makes `def f ([] => [body])`
	// bind f to the Function value instead of to the body's result.
	// Macro values are likewise data here — a `(macro …)` result must bind
	// to its name, not auto-expand (it expands only via the named stepWord
	// branch). See design/MACROS-PHASE1.10.md §5.
	// EXCEPT when an application was explicitly ASKED for: `f/v apply`
	// sets FnDefInfo.Applied (native_valof.go) for a 0-arg-only fn, and
	// that is the one case where the value is not data. Without this the
	// gate let ORIGIN decide — a named 0-arg fn dispatched, an anonymous
	// one stayed inert — which ADR-016 forbids (NUR077 §5 Hole 1). Macros
	// keep parking regardless: applying a macro is never a stack-value
	// dispatch (design/MACROS-PHASE1.10.md §5, D4).
	if ((fnDef.Anonymous && !applied) || fnDef.Macro) &&
		fwdCount == 0 && len(positions) == 0 {
		e.Pointer++
		return nil
	}

	// Forward-collecting match: defer dispatch until the remaining
	// tokens have been consumed. When the Forward marker completes,
	// the engine re-processes the Function value with all args on
	// the stack — which routes through this same execFnDefLiteral
	// entry. This branch runs whether the sig has a Go Handler
	// (registered native) or only a boru body (anonymous FnDef from
	// `afn` / `=>`); in both cases matchSignature found valid
	// positions and we need the forward args on the stack before
	// the body / handler runs.
	if fwdCount > 0 {
		stkCount := len(positions) - fwdCount
		return e.insertForward(w, sig, fwdCount, stkCount, specAt)
	}

	// The fn-value dispatch COMMIT (dispatch_probe.go): past every
	// non-dispatch exit — the handler-less/nil reroute, the
	// anonymous-lambda/macro parking (the value stays DATA; probing there
	// counted non-dispatches, Codex P2 on PR #420), and the insertForward
	// deferral (which re-enters here with args on the stack, so a parked
	// dispatch is probed exactly once, at its real commit).
	e.probeDispatch(fn, w, sig, positions, specAt)

	// All args resolved on the stack. Anonymous FnDefs (no Go
	// Handler) take the legacy stack-match path, which splices the
	// body via execFnDefSig and binds named params via def-stack.
	if sig.DispatchHandler() == nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		return e.ExecFnDefSigStackMatch(valIdx, fnDef, resolved)
	}

	// Module closures: the FnDef carries a captured sub-registry
	// (Registry != e.registry). Two cases:
	//
	//  1. **Trivial-delegation wrapper** — the wrapper FnSig body is
	//     a single Word referencing the inner native of the same name
	//     (e.g. rand.string's wrapper body `[Word(rand-string)]`).
	//     matchSignature already found and matched that inner native;
	//     its handler is `sig.DispatchHandler()`. Direct-call it via execMatch
	//     so args flow straight through in sig order — no body
	//     execution, no token splicing, no push reordering.
	//
	//  2. **Boru fn defined inside a module preamble** (e.g.
	//     decision.cond, defined via `def cond fn […]` in the
	//     module's boru source). The body references module-private
	//     types/words that only resolve in fnDef.Registry, so we must
	//     run it in that registry via CallBoru. These fns use NAMED
	//     params, so CallBoru's named-param binding (InstallDef by
	//     name) sidesteps any unnamed-param push ordering issues.
	//
	// See design/SIG-ORDER-REFACTOR.10.md for the architecture history.
	//
	// Only BORU-BODIED definitions take the sub-registry path: a trivial-
	// delegation wrapper (Body=[Word(inner)]) or a module-preamble fn (real
	// Body). A reference to a Go NATIVE living in the sub-registry carries
	// Body-less sigs and a real Go Handler — it must dispatch straight
	// through execMatch below, exactly like any other native, so we require
	// a body-bearing own sig before entering this branch.
	if fnDef.Registry != nil && fnDef.Registry != e.Registry {
		ownSigs := fnDef.OwnSigs()
		var wrapperSig *FnSig
		// Select the own sig CORRESPONDING TO THE MATCHED sig — same
		// param names and types, not merely the same arity. With two
		// same-arity overloads (e.g. a generic `[rhs:T op:String
		// lhs:T]` sig and its `[rhs:Any op:String lhs:Any]` catch-all)
		// the old arity-only pick ran the FIRST body with the OTHER
		// sig's matched args — the exact body/sig mis-pairing the
		// per-sig handler attachment exists to prevent. Arity remains
		// the fallback when no exact correspondence is found.
		var arityPick *FnSig
		for i := range ownSigs {
			if len(ownSigs[i].Body()) == 0 {
				continue // native valof sig — not a wrapper/preamble body
			}
			if wrapperSig == nil {
				wrapperSig = &ownSigs[i] // last resort: first body-bearing sig
			}
			if len(ownSigs[i].Params) != len(positions) {
				continue
			}
			if arityPick == nil {
				arityPick = &ownSigs[i]
			}
			if sig != nil && sigParamsCorrespond(ownSigs[i].Params, sig.Params) {
				arityPick = &ownSigs[i]
				break
			}
		}
		if arityPick != nil {
			wrapperSig = arityPick
		}
		if wrapperSig != nil {
			// Surface a module-export dispatch in the trace so a profiler
			// (Debug.profile) can attribute it: a dotted call like
			// `Math.sqrt` otherwise shows only the `get` expansion, never the
			// function it resolves to. Gated on a module origin so ordinary
			// user-fn and bare-word traces are unaffected.
			if fnDef.Module != "" && e.trace != nil {
				e.traceNote = "call " + fnDef.Name
			}
			// ONE dispatch path, no exceptions: a module wrapper — trivial-delegation
			// OR a real boru body — dispatches through execMatch, exactly like a named
			// fn and a bare-word call. So a fn body is ANALYSED/COMPILED the SAME way
			// regardless of how the fn was reached: a param-slot unit via the matched
			// sig's ReturnsFn, NOT a separate def-stack CallBoru run that left Function
			// params slot-less and unreachable to a closure capture (the sort
			// comp-capture leaf — a fundamental dispatch-path divergence). match.Reg
			// carries the sub-registry so the body's module-private words resolve there
			// (the inner native's poly re-match, and the body's own scope);
			// shareCheckState routes the body's unit-compile into the MAIN program's
			// emit so RecordUserCall references it (else "user fn call (Stage 3)"). A
			// no-op outside check mode — interpret runs the matched handler in the
			// sub-registry it was installed in, exactly as before.
			if sig.DispatchHandler() != nil {
				match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name, Reg: fnDef.Registry}
				if len(positions) > 0 {
					match.Args = make([]Value, len(positions))
					for i, pos := range positions {
						match.Args[i] = e.Tape.At(pos)
					}
				}
				restoreCheck := CheckBraid.ShareCheckState(e, fnDef.Registry)
				err := e.execMatch(match)
				restoreCheck()
				return err
			}
			// Degenerate: a wrapper sig with no body-runner handler — fall through.
			args := make([]Value, len(positions)) //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			for i, pos := range positions {       //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				args[i] = e.Tape.At(pos)
			}
			return e.execFnDefSig(valIdx, wrapperSig, args, fnDef.Registry, fnDef.Anonymous) //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		}
	}

	// Pure-stack match: dispatch via execMatch the same way a bare
	// word with no forward args would. A NATIVE REFERENCE into a module
	// sub-registry (`assert-equal/v` in an export map — body-less sigs, a
	// real Go handler, so the wrapper branch above deliberately skips it)
	// still carries its owning registry on match.Reg, exactly like the
	// trivial-delegation dispatch: the recorder's poly re-match
	// (tryRecordPoly) validates the matched sig against THAT registry's
	// own binding and the VM re-matches over it (PolyRef.Reg). Without it
	// a dynamic-operand dispatch of a module native refused as "dynamic
	// input at <word>" although the runtime re-match over the sub-registry
	// IS the interpreter's dispatch. Interpretation is unchanged — Reg is
	// read only at the check-mode carrierResults seam (execMatch:2625).
	match := &MatchResult{Sig: sig, Positions: positions, Name: fnDef.Name}
	if fnDef.Registry != nil && fnDef.Registry != e.Registry {
		match.Reg = fnDef.Registry
	}
	if len(positions) > 0 {
		match.Args = make([]Value, len(positions))
		for i, pos := range positions {
			match.Args[i] = e.Tape.At(pos)
		}
	}
	return e.execMatch(match)
}

// sigParamsCorrespond reports whether an authored FnSig's params and
// a compiled Signature's params describe the same overload: same
// count, same names, same declared types. Used by execFnDefLiteral's
// sub-registry path to run the body belonging to the signature
// matchSignature actually selected.
func sigParamsCorrespond(a, b []FnParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		at, bt := a[i].Type, b[i].Type
		if (at == nil) != (bt == nil) {
			return false
		}
		if at != nil && !at.Equal(bt) {
			return false
		}
	}
	return true
}

// isRecordableLiteral reports whether a stepLiteral value should be
// surfaced to a Recorder. Control-flow markers and engine-internal
// shapes (Forward/Mark/Move/ReturnCheck/DefCleanup/OpenParen/CloseParen/End/Internal)
// are NOT user-visible literals — they're scaffolding around the
// real data, and a stack-form recording wants only the data.
func IsRecordableLiteral(v Value) bool {
	if v.Parent == nil {
		return false
	}
	switch {
	case IsForward(v), IsOpenParen(v), IsCloseParen(v), IsEnd(v):
		return false
	case v.Parent.ConformsTo(TMark), v.Parent.ConformsTo(TMove),
		v.Parent.ConformsTo(TReturnCheck), v.Parent.ConformsTo(TInternal):
		return false
	}
	return true
}

// inFnFrame reports whether the pointer currently sits INSIDE a fn frame —
// i.e. an unmatched frame-open paren lies before it. Recorder events fired
// from in there are a callee's internals, not the caller's strict-stack
// program, so every recorder site suppresses on it (design/PBT-PLAN.10.md:
// a `Call` is one Op per dispatch, with bodies belonging to a nested Quote,
// never to the top level).
//
// Deliberately COMPUTED FROM THE TAPE rather than tracked in a counter.
// A counter has to be incremented where a frame is spliced and decremented
// where it collapses, and those are not balanced: TCO REPLACES a frame with
// another (one open, one close, no nesting), an error unwinds out of a frame
// without ever reaching its close paren, and the step limit and tape-growth
// ceiling both abandon a frame mid-flight. A counter that mis-tracks on any
// of those leaves suppression latched on and silently truncates the rest of
// the form — the exact failure mode this function exists to end. Reading the
// tape re-derives the answer from scratch at every event, so it cannot
// desynchronise.
//
// The scan is O(pointer) and runs ONLY when a recorder is installed — every
// call site guards on `e.recorder != nil` first, and Go's && short-circuits,
// so an ordinary Run never executes it. The two production recorder consumers
// (`Debug.disasm`, the PBT gen-program shrinker) both run small programs.
func (e *Engine) inFnFrame() bool {
	// Fast path for the common shape: if this engine has never spliced a fn
	// frame, no unmatched frame-open can exist and the scan is pure waste. A
	// flat recorded program — which is what `Debug.disasm` gets handed most
	// often — would otherwise pay a full prefix walk per literal, making
	// Compile quadratic in program length for a program with no functions at
	// all. The flag is MONOTONIC (set, never cleared), so it carries none of
	// the balance hazard that rules a depth counter out: it can only ever say
	// "no frame has existed yet", which is exactly when skipping is safe.
	if !e.sawFnFrame {
		return false
	}
	depth := 0
	for i := e.Pointer - 1; i >= 0; i-- {
		v := e.Tape.At(i)
		if IsCloseParen(v) {
			depth++
			continue
		}
		if IsOpenParen(v) {
			if depth > 0 {
				depth--
				continue
			}
			if IsFrameOpen(v) {
				return true
			}
		}
	}
	return false
}

// recordDispatch reports one dispatch to an installed Recorder, choosing the
// `returns` count the recorder's skip accounting actually needs.
//
// For a NATIVE, `results` is the handler's return values: the main loop
// re-encounters each one at the pointer and fires OnPushLit, so the recorder
// must skip exactly that many — `len(results)` is right.
//
// For a BORU FN, `results` is the spliced fn-frame SKELETON (frame-open paren,
// bound arg cells, body tokens, teardown), which is not the return count at
// all — a 1-arg/1-return fn splices eight tokens. Passing that through poisoned
// the skip counter and swallowed unrelated later literals: `def z fn
// [[][Integer][42]] z 99` recorded a form that dropped the 99, and the PBT
// shrinker consequently reported counterexamples its generator cannot produce.
// A frame's return values are accounted for elsewhere — stepCloseParen's
// RecorderSkipper hook skips the survivors when the frame collapses — so the
// right count HERE is zero.
//
// The discriminator is the spliced tokens themselves, NOT `Sig.FnFrame()`,
// which is neither necessary nor sufficient: buildFnBodyHandler's
// foreign-registry early return (core_helpers.go) runs a module export's body
// in a sub-engine and returns real VALUES while FnFrame stays non-nil, so
// keying on the sig would mis-account the hottest module-call path.
func (e *Engine) recordDispatch(name string, arity int, results []Value) {
	if e.inFnFrame() {
		// A callee's own dispatch. The enclosing Call already stands for it.
		return
	}
	if len(results) > 0 && IsFrameOpen(results[0]) {
		e.sawFnFrame = true
		e.recorder.OnCall(name, arity, 0)
		return
	}
	e.recorder.OnCall(name, arity, len(results))
}

// trivialDelegationTarget reports the inner native name a wrapper FnSig
// purely delegates to — body of the form `[Word(inner)]` with all-
// unnamed Params — and whether the sig has that shape at all. Unlike
// isTrivialDelegationBody it does NOT require the inner name to equal
// the wrapper's own name, so it also recognises a wrapper rebound under
// a different name (`def w pkg.word`, `unpack [word] pkg`): the body
// word still names the original inner native to look up in the
// sub-registry. See InstallDef's module-wrapper rebinding branch.
func trivialDelegationTarget(sig *FnSig) (string, bool) {
	if len(sig.Body()) != 1 {
		return "", false
	}
	for _, p := range sig.Params {
		if p.Name != "" {
			return "", false
		}
	}
	if !IsWord(sig.Body()[0]) {
		return "", false
	}
	w, err := AsWord(sig.Body()[0])
	if err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		return "", false
	}
	return w.Name, true
}

// upcomingArgs returns the value/literal tokens that follow valIdx up to
// the next statement or group boundary (End / CloseParen / Mark / Move)
// — the forward args a function-value call had available to collect.
// Forward-collection markers are skipped; everything before the first
// boundary is a candidate arg. Lets a failed forward-form call
// (`Pkg.fn a b`) be recognised as a call, not a bare value reference.
func (e *Engine) upcomingArgs(valIdx int) []Value {
	var out []Value
	for i := valIdx + 1; i < e.Tape.Len(); i++ {
		v := e.Tape.At(i)
		switch {
		case IsForward(v):
			continue
		case IsMark(v) || IsMove(v) || IsCloseParen(v) || IsEnd(v):
			return out
		default:
			out = append(out, v)
		}
	}
	return out
}

// ExecFnDefSigStackMatch is the legacy pure-stack dispatch path for
// boru-defined functions whose signatures carry named params. Used as a
// fallback when matchSignature's aggregate match returns nothing.
func (e *Engine) ExecFnDefSigStackMatch(valIdx int, fnDef FnDefInfo, resolved []Value) error {
	resolvedIdx := e.ResolvedIndicesBefore(len(resolved))
	checkMode := e.Registry != nil && e.Registry.analysisMode() && fnDef.Anonymous
	// A NON-anonymous body-bearing fn VALUE reached as a CALL while the
	// BYTECODE EMITTER is active (a `fn` literal resolved from a map / module
	// export, e.g. `ParseLang.parse_json 'x' {}`) is dispatched like a named
	// user fn: through buildFnBodyReturnsFn (spliceFnValueCheckResult), which
	// arms the body compile so the per-call `__pa` tail is captured inside its
	// own CALL_USER unit instead of leaking into the top-level residual.
	//
	// Gated on an ACTIVE emit state so it only changes the COMPILE path, never
	// the pure type-check pass: buildFnBodyReturnsFn runs AnalyseFnBody, which
	// can emit body diagnostics under generalised carrier args that the legacy
	// inline-splice path (execFnDefSig) did not — the check-accuracy ratchet
	// pins that pure-check behaviour. Excludes foreign-sub-registry fns (their
	// body must run via CallBoru in that registry — the execFnDefLiteral
	// sub-registry branch handles them) and macros.
	checkFnValue := e.Registry != nil && e.Registry.analysisMode() && !fnDef.Anonymous && !fnDef.Macro &&
		(fnDef.Registry == nil || fnDef.Registry == e.Registry) &&
		e.Registry.analysisRecorder().Active()
	ownSigs := fnDef.OwnSigs()
	for i := range ownSigs {
		sig := &ownSigs[i]
		nArgs := len(sig.Params)
		if nArgs == 0 {
			if checkMode {
				return CheckBraid.SpliceAnonCheckResult(e, valIdx, 0, sig, nil, fnDef.Captured)
			}
			if checkFnValue && len(sig.Body()) > 0 {
				return CheckBraid.SpliceFnValueCheckResult(e, valIdx, 0, fnDef, sig, nil)
			}
			return e.execFnDefSig(valIdx, sig, nil, fnDef.Registry, fnDef.Anonymous)
		}
		if len(resolved) < nArgs {
			continue
		}

		hasNamed := false
		for _, p := range sig.Params {
			if p.Name != "" {
				hasNamed = true
				break
			}
		}

		match := true
		if hasNamed {
			for j, p := range sig.Params {
				ri := len(resolved) - 1 - j
				if !SigTypeMatches(resolved[ri], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.Parent.Equal(TMap) && resolved[ri].Parent.Equal(TMap) &&
						pat.Data != nil && resolved[ri].Data != nil &&
						!IsOptionsType(pat) {
						if !OpenUnifyMap(pat, resolved[ri]) {
							match = false
							break
						}
					} else {
						if _, uOk := Unify(resolved[ri], pat); !uOk {
							match = false
							break
						}
					}
				}
			}
			if match {
				args := make([]Value, nArgs)
				for j := 0; j < nArgs; j++ {
					ri := len(resolvedIdx) - 1 - j
					args[j] = e.Tape.At(resolvedIdx[ri])
				}
				if checkMode {
					return CheckBraid.SpliceAnonCheckResult(e, valIdx, nArgs, sig, args, fnDef.Captured)
				}
				if checkFnValue && len(sig.Body()) > 0 {
					return CheckBraid.SpliceFnValueCheckResult(e, valIdx, nArgs, fnDef, sig, args)
				}
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry, fnDef.Anonymous)
			}
		} else {
			candidate := resolved[len(resolved)-nArgs:]
			for j, p := range sig.Params {
				if !SigTypeMatches(candidate[j], p.Type) {
					match = false
					break
				}
				if p.Pattern != nil {
					pat := *p.Pattern
					if pat.Parent.Equal(TMap) && candidate[j].Parent.Equal(TMap) &&
						pat.Data != nil && candidate[j].Data != nil &&
						!IsOptionsType(pat) {
						if !OpenUnifyMap(pat, candidate[j]) {
							match = false
							break
						}
					} else {
						if _, uOk := Unify(candidate[j], pat); !uOk {
							match = false
							break
						}
					}
				}
			}
			if match {
				args := make([]Value, nArgs)
				startIdx := len(resolvedIdx) - nArgs
				for j := 0; j < nArgs; j++ {
					args[j] = e.Tape.At(resolvedIdx[startIdx+j])
				}
				if checkMode {
					return CheckBraid.SpliceAnonCheckResult(e, valIdx, nArgs, sig, args, fnDef.Captured)
				}
				if checkFnValue && len(sig.Body()) > 0 {
					return CheckBraid.SpliceFnValueCheckResult(e, valIdx, nArgs, fnDef, sig, args)
				}
				return e.execFnDefSig(valIdx, sig, args, fnDef.Registry, fnDef.Anonymous)
			}
		}
	}

	// A NAMED function reached as a call — args on the stack
	// (swap/prefix form) or upcoming forward tokens (`Pkg.fn a b`) — that
	// matched no signature is an ERROR HERE, at the dispatch site
	// (design/FN-VALUE-DISPATCH.0.md). Guards keep the detection precise:
	// named (not an anonymous lambda value), not explicitly inert (`/v` /
	// `quote` set Quoted), and at least one candidate arg available — so a
	// bare function-as-value reference with no args is left alone, which is
	// how a function is passed as data.
	//
	// This supersedes ERRORS.8.md §5 option 2, which marked the value and
	// let the top-level end-of-run drain raise only if NOTHING consumed it.
	// "Unconsumed" turned out not to mean "not consumed by a higher-order
	// word": any Any-typed slot cleared the residue, so `print (IO.read
	// "/nonexistent")` printed the FUNCTION and exited 0, and `boru check`
	// reported it clean. Composition that wants the value as data now says
	// so — `f/v` — and the judgement no longer depends on what happens to
	// the value afterwards.
	//
	// Check mode reports the same finding as a diagnostic instead, under the
	// guaranteed-error-mirror discipline (eng/go/CLAUDE.md): only where the
	// analysis position is unconditionally reached and untrapped, because a
	// fn body analysed against generalised carrier args can fail to match
	// for want of precision rather than because the program is wrong.
	if e.Registry != nil &&
		fnDef.Name != "" && !fnDef.Anonymous &&
		valIdx < e.Tape.Len() && !e.Tape.At(valIdx).Quoted {
		candidates := append(append([]Value{}, resolved...), e.upcomingArgs(valIdx)...)
		if len(candidates) > 0 {
			pos := e.Tape.At(valIdx).Pos()
			// Borrow a span from the nearest argument when the FnDef value
			// itself carries none, so the report points somewhere real.
			if pos.Row == 0 {
				for _, c := range candidates {
					if c.Pos().Row > 0 {
						pos = c.Pos()
						break
					}
				}
			}
			// The detail no longer says "was left on the stack as data" — that
			// described what the OLD contract did with the value, and saying it
			// while raising would tell the reader the opposite of what happened.
			detail := "call to '" + fnDef.Name + "' matched no signature"
			if e.Registry.analysisActive() {
				// Analysis continues past the finding, so the value stays on the
				// tape and downstream check-mode consumers must be able to tell
				// that it is dispatch WRECKAGE rather than a deliberate value —
				// see defWordExtension, which would otherwise read the failed
				// call's locked signatures as a word-extension attempt.
				fv := e.Tape.At(valIdx)
				fv.FailedDispatch = true
				e.Tape.Set(valIdx, fv)
				if e.Registry.analysisAtUncaughtTopLevel() {
					// NOT a RuntimeMirror: a mirror promises the program still
					// compiles and raises the identical error, and there is no
					// call here to compile — dispatch did not resolve, exactly
					// like no_signature. So the compile pipeline must refuse on
					// it (eng/go/CLAUDE.md, the model-undermining class).
					e.Registry.noteAnalysisUniqueDiagnostic(CheckDiagnostic{
						Code:   "uncalled_function",
						Detail: detail,
						Word:   fnDef.Name,
						Row:    pos.Row,
						Col:    pos.Col,
					})
				}
			} else {
				return makeBoruErrorAt("uncalled_function", detail,
					fnDef.Name, e.effectiveSource(),
					"hint: check the call's argument types and arity — or use "+fnDef.Name+"/v to push the function as a value deliberately",
					pos)
			}
		}
	}

	e.Pointer++
	return nil
}

// fnDefAtPointer reads the FnDefInfo the value at the pointer dispatches as.
// A compiled closure (ClosurePayload) is the FnDefInfo the interpreter would
// have minted for the same source, and the VM piece bridges it to one
// (CompiledRuntime.ClosureAsFnDef): a signature over the unit's declared
// param contract, the closure applied through the registry's body-closure
// invoker, under the closure's own identity. The bridge stands in for THIS
// dispatch only — what stays data (a parked 0-arg lambda, a no-match) and
// what dispatches is decided by execFnDefLiteral's rules exactly as for the
// interpreter's own value, but the TAPE keeps the closure itself: a park
// leaves the payload where it was, never a handler bound to the running
// VM's context (Codex P2 on PR #444 — the bridged value used to replace the
// closure on the tape, so a parked copy could escape into a binding and
// keep applying through a finished run's invoker). Outside a VM run, or for
// a unit the bridge cannot describe, the closure stays data (ok=false).
func (e *Engine) fnDefAtPointer(val Value) (Value, FnDefInfo, bool) {
	if fnDef, ok := val.Data.(FnDefInfo); ok {
		return val, fnDef, true
	}
	bridged, ok := compiledRuntime.ClosureAsFnDef(e.Registry, val)
	if !ok {
		return val, FnDefInfo{}, false
	}
	val = WithPos(bridged, val)
	fnDef, ok := val.Data.(FnDefInfo)
	return val, fnDef, ok
}

// compileFnDef produces the compiled-dispatch view of a constructed
// Function value (an afn closure or a captured FnDef) whose Signatures are
// still in authored form (named params + body, unresolved BarrierPos, no
// handler). It normalises each signature in place — resolving the BarrierPos
// sentinel, attaching handlers, sorting into dispatch order — and computes
// MaxForwardArgs. It is idempotent on already-compiled signatures.
//
// This centralisation is the seam the function-model consolidation builds
// on: every site that needs a dispatchable FnDefInfo from authored sigs
// routes through here rather than constructing the struct inline.
//
// Each compiled Signature is given a Go Handler (the shared boru body-
// runner, buildFnBodyHandler) and a check-mode ReturnsFn
// (buildFnBodyReturnsFn), so a Function value dispatches through the
// uniform execMatch path exactly like a registered native — no
// handler-less fallback. Handlers are attached per-sig BEFORE
// SortSignatures so each handler stays paired with its own signature
// (the sort reorders sigs, so attaching by post-sort index would
// mis-pair them).
//
// r is the registry the body runs in: captures are already snapshotted
// into fnDef.Captured, and free body words resolve dynamically against r.
//
// Handlers are attached ONLY for anonymous fns (afn / `=>` lambdas and
// the closures they return). A non-anonymous bare FnDef value —
// notably a predicate-type FnDef sitting on the stack — must stay inert
// data (no auto-dispatch): leaving its Handler nil routes it through the
// legacy stack-match fall-through in execFnDefLiteral, which is the
// behaviour `def Bbd …; Bbd "c"` relies on (the FnDef and "c" remain on
// the stack rather than the predicate firing). See execFnDefLiteral's
// "predicate-type FnDefs landing bare are intentionally inert" guard.
func compileFnDef(r *Registry, fnDef FnDefInfo) *FnDefInfo {
	out := make([]Signature, len(fnDef.Signatures))
	for i := range fnDef.Signatures {
		sig := fnDef.Signatures[i]
		barrier := sig.BarrierPos
		if barrier == BarrierAllForward {
			barrier = len(sig.Params)
		}
		// Keep the full authored sig (Params with names, Returns, Body,
		// NoEval*) and layer the compiled dispatch fields on top, so a
		// constructed Function value stays a single full-fidelity slice.
		compiled := sig
		compiled.Params = append([]FnParam(nil), sig.Params...)
		compiled.BarrierPos = barrier
		// An anonymous fn's authored sig carries its boru body and gets the
		// body-runner attached here. A sig that already dispatches through a
		// Go handler — the VM's bridge for a compiled closure VALUE
		// (CompiledRuntime.ClosureAsFnDef), Anonymous so a 0-arg lambda parks
		// as the interpreter's own does — keeps its handler: there is no body
		// to run, and overwriting it ran an EMPTY frame that returned the
		// argument itself.
		if _, isGo := sig.Impl.(*GoImpl); fnDef.Anonymous && !isGo {
			meta := &FnFrameMeta{
				Name:         fnDef.Name,
				HasGen:       fnDef.Gen != nil,
				InstallNames: fnInstallNames(sig, fnDef.Captured),
			}
			compiled.Impl = &BoruImpl{
				Body:     sig.Body(),
				FnFrame:  meta,
				dispatch: buildFnBodyHandler(r, fnDef.Name, sig, fnDef, meta),
			}
			compiled.ReturnsFn = r.analysisReturnsFn(fnDef.Name, sig, fnDef)
		}
		NormalizeSig(&compiled)
		out[i] = compiled
	}
	SortSignatures(out)
	return &FnDefInfo{
		Name:           fnDef.Name,
		Signatures:     out,
		MaxForwardArgs: calcMaxForwardArgs(out),
		Registry:       fnDef.Registry,
		Anonymous:      fnDef.Anonymous,
		Captured:       fnDef.Captured,
		MiniKind:       fnDef.MiniKind,
		// Compiling a body does not make a different function (NUR031).
		ident: fnDef.ident,
	}
}

// execFnDefSig executes a matched FnDef signature. If capturedReg is non-nil
// (module closure), execution uses CallBoru on that registry. Otherwise, body
// tokens are spliced into the current engine's stack.
func (e *Engine) execFnDefSig(valIdx int, sig *FnSig, args []Value, capturedReg *Registry, anonymous bool) error {
	// Per-export module policy gate (NUR045): the body-splicing twin of
	// execMatch's gate — a module boru-fn VALUE dispatched through the
	// legacy stack-match path (ExecFnDefSigStackMatch) or the degenerate
	// wrapper branch carries the stamped identity on its own FnSig.
	if err := e.policyGateModuleCall(sig.ModuleCall); err != nil {
		return err
	}
	nArgs := len(sig.Params)
	indices := e.ResolvedIndicesBefore(nArgs)

	// Capture the call-site position (the fn value being invoked) before the
	// stack is mutated, so a return-type error can point at the call.
	var callPos SrcPos
	if valIdx >= 0 && valIdx < e.Tape.Len() {
		callPos = e.Tape.At(valIdx).Pos()
	}

	// Auto-evaluate consumed arguments with Eval=true so FnDef handlers
	// receive resolved data. Maps: {base:hex} → {base:atom(hex)}.
	// Lists: [c1 c2] → [map1, map2].
	//
	// NoEvalArgs / NoEvalMapArgs on the sig suppresses auto-eval at
	// specific positions — required for module wrappers that take
	// quoted code bodies (e.g. rand.list-of [gen] N) so the body
	// reaches the inner handler intact rather than being sub-Run'd.
	// Mirrors the NoEvalArgs handling in execMatch (lines 977-1002).
	for i := range args {
		// Dispatch ascription: consumed at delivery, exactly as execMatch
		// strips it — the fn body binds the REAL value.
		args[i] = StripAscribed(args[i])
		if args[i].Eval && !args[i].Quoted {
			if args[i].Parent.Equal(TMap) &&
				args[i].Data != nil && !IsTypedMap(args[i]) && !IsRecordType(args[i]) && !IsOptionsType(args[i]) {
				noEval := sig.NoEvalMapArgs != nil && sig.NoEvalMapArgs[i]
				if !noEval {
					// This FnDef/module-wrapper path never dispatches the core `make`
					// word (a core native goes through execMatch), so its map args are
					// not construction bodies — keep the const-fold path.
					evaluated, err := e.AutoEvalMap(args[i], false, true)
					if err != nil {
						return err
					}
					args[i] = evaluated
				}
			} else if args[i].Parent.Equal(TList) &&
				args[i].Data != nil && !IsTypedList(args[i]) && !IsTableType(args[i]) {
				noEval := sig.NoEvalArgs != nil && sig.NoEvalArgs[i]
				if !noEval {
					// Bare words never degrade to data — propagate the
					// auto-eval failure (see execMatch).
					evaluated, err := e.autoEvalList(args[i], true)
					if err != nil {
						return err
					}
					args[i] = evaluated
				}
			}
		}
		args[i].Eval = false
		args[i].Undefined = false
	}

	if capturedReg != nil && (capturedReg != e.Registry || e.Registry.Lookup("__pa") == nil) {
		// Execute in the captured module's registry via CallBoru.
		// Pass the FnDef's lexical captures so the body sees them as
		// defs (alongside the module-registry's own bindings).
		//
		// Same-registry values fall through to the splice branch below: a
		// module-fn VALUE applied inside its own module (a callback passed
		// back in) needs no registry hop, so it takes the same main-tape
		// frame path a named fn call does — one fewer sub-engine per call,
		// and flow-control/def-cleanup semantics identical to named
		// dispatch (the TCO-STAGED Stage-5 residual flip; boundary rows in
		// lang/spec/module-fnvalue-boundary.tsv). Gated on the frame
		// protocol being EXECUTABLE: the splice tail's `__pa` word is
		// registered by the language layer, so a bare kernel registry (an
		// eng-only embedder, the kernel test harnesses) keeps the CallBoru
		// path, whose per-call cleanup is Go-side and needs no words.
		var captures []CapturedBinding
		var fnLabel string
		if valIdx < e.Tape.Len() {
			if fd, ok := e.Tape.At(valIdx).Data.(FnDefInfo); ok {
				captures = fd.Captured
				fnLabel = fd.Name
			}
		}
		restoreCheck := CheckBraid.ShareCheckState(e, capturedReg)
		var result []Value
		var err error
		if e.Registry.analysisMode() {
			// Check mode ANALYSES the body — always the interpreter path
			// (running a stamped unit here would execute real side effects
			// during static analysis).
			result, err = capturedReg.CallBoruNamed(sig, args, captures, fnLabel)
		} else {
			// Runtime: a module fn stamped at load (StampFnValueInPlace,
			// RunModuleBody) runs its unit on the VM — the module
			// sub-registry is idle from the caller's perspective, so
			// InvokeCallback starts a fresh RunUnit there; an unstamped or
			// stale-dep sig falls to CallBoru inside the seam, byte-identical
			// to the old direct call. This retires the per-call interpreter
			// hop for module-export application (the mini-redis client loop:
			// `MiniRedis.cmd` per iteration).
			result, err = InvokeCallback(capturedReg, sig, args, captures)
		}
		restoreCheck()
		if err != nil {
			return err
		}
		// Splice: remove consumed args + FnDef, insert results.
		if len(indices) == nArgs && nArgs > 0 {
			firstArgIdx := indices[0]
			skipSet := make(map[int]bool, nArgs+1)
			for _, idx := range indices {
				skipSet[idx] = true
			}
			skipSet[valIdx] = true
			dst := firstArgIdx
			for i := firstArgIdx; i <= valIdx; i++ {
				if !skipSet[i] { //covergate:allow execFnDefSig cross-registry CallBoru result-splice interior (structural cell copy-down): post arguments-are-inert flip, foreign fn values dispatch via the compiled-value path first, so no corpus shape reaches these splice arms; kept as defensive splice-correctness arms (design/ARG-SEMANTICS-UNIFICATION.0.md §7) (§kernel)
					e.Tape.Set(dst, e.Tape.At(i))
					dst++
				}
			}
			e.Tape.Splice(dst, valIdx+1-dst, result...)
			e.Pointer = firstArgIdx
		} else if nArgs == 0 { //covergate:allow execFnDefSig cross-registry 0-arg splice arm; see 5015.20 entry (§kernel)
			e.Tape.Splice(valIdx, 1, result...)
		} else { //covergate:allow execFnDefSig cross-registry forward-fallback splice arm; see 5015.20 entry (§kernel)
			argStart := valIdx - nArgs
			if argStart < 0 { //covergate:allow execFnDefSig cross-registry forward-fallback splice arm; see 5015.20 entry (§kernel)
				argStart = 0
			}
			e.Tape.Splice(argStart, valIdx+1-argStart, result...) //covergate:allow execFnDefSig cross-registry forward-fallback splice arm; see 5015.20 entry (§kernel)
			e.Pointer = argStart
		}
		return nil
	}

	// No captured registry — splice body tokens into the current stack.
	var tokens []Value
	tokens = append(tokens, NewFrameOpen(fnValueFrameMeta))

	// Push the fn-entry baseline before installing anything. Inner
	// fn/afn constructions inside this body consult TopFnBaseline
	// to identify enclosing-fn-local bindings. Paired with __pa
	// below, which pops the baseline.
	e.Registry.PushFnBaseline(e.Registry.Defs.Snapshot())

	// Retag typed-container args so the args stack (args.N) and unnamed body
	// pushes carry the {:T}/[:T] tag too, not just the named binding — a body
	// write via args.N must enforce the same contract (Codex round 4).
	args = RetagTypedContainerArgs(sig.Params, args)
	argsCopy := make([]Value, len(args))
	copy(argsCopy, args)
	if err := e.Registry.Args.Push(NewList(argsCopy)); err != nil {
		e.Registry.PopFnBaseline()
		return err
	}

	// Lexical captures from the FnDefInfo that produced this dispatch.
	// Pulled from the stack value at valIdx since execFnDefSig's signature
	// doesn't carry the FnDefInfo directly. Install before params so
	// params shadow same-named captures (innermost wins).
	var captures []CapturedBinding
	if valIdx < e.Tape.Len() {
		if fd, ok := e.Tape.At(valIdx).Data.(FnDefInfo); ok {
			captures = fd.Captured
		}
	}
	var names []string
	for _, cb := range captures {
		InstallFrameBinding(e.Registry, cb.Name, cb.Value)
		names = append(names, cb.Name)
	}

	// args in top-first sig order (matchSignature convention).
	// Named params bind by name; unnamed params push to body tokens in
	// i-order. No reordering — same convention as InstallFnDef and
	// CallBoru. See design/SIG-ORDER-REFACTOR.10.md.
	unnamedCount := 0
	for i, p := range sig.Params {
		if p.Name != "" {
			InstallFrameBinding(e.Registry, p.Name, RetagTypedContainerParam(p, args[i]))
			names = append(names, p.Name)
		} else {
			tokens = append(tokens, args[i])
			unnamedCount++
		}
	}
	// Stamp the resolved-argument span on the frame open so the step
	// loop skips the unnamed args (arguments are inert —
	// FrameOpenInfo.ArgSpan; same stamp as buildFnBodyHandler).
	if unnamedCount > 0 {
		tokens[0] = NewFrameOpenSpan(fnValueFrameMeta, unnamedCount)
	}
	// Snapshot AFTER captures+params so the tail's DefCleanup tears
	// down only body-local defs — the same placement as
	// buildFnBodyHandler. (This tail historically omitted the
	// DefCleanup marker; it is synthesized by the shared
	// AppendFrameTail now, so the two splice paths cannot diverge.)
	defSnapshot := e.Registry.Defs.Snapshot()

	// Append the sig's body tokens directly: append COPIES them into
	// tokens' backing array, and sig.Body() (the shared BoruImpl.Body) is
	// never mutated here, so the previous intermediate make+copy was a
	// redundant per-call allocation (design/INTERPRETER-SPEED-PLAN.10.md #5).
	tokens = append(tokens, sig.Body()...)

	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry:       e.Registry,
		Snapshot:       defSnapshot,
		Names:          names,
		Returns:        sig.Returns,
		ReturnPatterns: sig.ReturnPatterns,
		Decl:           sig.Decl,
		UnnamedCount:   unnamedCount,
		FuncName:       "<fn>",
		Pos:            callPos,
		EvalResidual:   !anonymous || BodyEvalsResidual(sig.Body()),
	})
	tokens = append(tokens, NewCloseParen())

	// Report the application to an installed Recorder with an EMPTY name,
	// which stackform.Replayable refuses (NUR077).
	//
	// This splice path bypasses execMatch entirely, so without any event a fn
	// VALUE applied off a container or a param produced NO op at all and the
	// recorded form silently dropped the call — replaying to the function
	// itself instead of its result. Refusing is the fix; recording a real
	// Call is not, EVEN when the value carries a name. `Call{Name, Arity}`
	// re-invokes by name and does not consume a receiver, whereas an
	// application consumes the fn value the stack already holds — so a named
	// Call would strand that value and produce it twice. Expressing this
	// faithfully needs an apply-style Op the vocabulary does not have; until
	// it does, the honest form is one that refuses rather than one that lies.
	if e.recorder != nil && !e.inFnFrame() {
		e.sawFnFrame = true
		e.recorder.OnCall("", nArgs, 0)
	}

	if len(indices) == nArgs && nArgs > 0 {
		firstArgIdx := indices[0]
		skipSet := make(map[int]bool, nArgs+1)
		for _, idx := range indices {
			skipSet[idx] = true
		}
		skipSet[valIdx] = true
		dst := firstArgIdx
		for i := firstArgIdx; i <= valIdx; i++ {
			if !skipSet[i] {
				e.Tape.Set(dst, e.Tape.At(i))
				dst++
			}
		}
		e.Tape.Splice(dst, valIdx+1-dst, tokens...)
		e.Pointer = firstArgIdx
	} else if nArgs == 0 {
		e.Tape.Splice(valIdx, 1, tokens...)
	} else {
		argStart := valIdx - nArgs
		if argStart < 0 {
			argStart = 0
		}
		e.Tape.Splice(argStart, valIdx+1-argStart, tokens...)
		e.Pointer = argStart
	}

	return nil
}

// ReachFnWouldClaim reports whether the reach-read function value fnVal
// would CLAIM the token at idx as its own first forward argument — the
// call half of the NUR038 arrival gate's call-vs-data decision. The test
// is SIG-AWARE: the token's statically-knowable value must fit some
// signature's leading forward-eligible slot (sqrt(Number) claims a `16`
// but not a `[99]` list — the list is the enclosing if's arm, the fn is
// its OTHER arm, the branch-apply feature). A statement boundary, a
// close paren, an fn-word barrier, an unresolvable word, or nothing at
// all leaves the fn with no claim (data — the pinned `typeof IO.stdin`
// / `def sqrt MathUtil.sqrt` reference idioms). A group/reach/interp
// token has an unknowable result type and counts as claimable (the
// runtime would collect it optimistically).
func (e *Engine) ReachFnWouldClaim(fnVal Value, idx int) bool {
	return ReachFnWouldClaimOn(e.Tape, e.Registry, fnVal, idx)
}

// ReachFnWouldClaimOn is ReachFnWouldClaim over an explicit window and
// registry — the same one-implementation-two-hosts rule as
// ForwardClaimProbeOn, and for the same reason: this is the NUR038
// call-vs-data decision, and a VM adapter that answered it differently would
// diverge from the interpreter on exactly the shapes the census still holds.
func ReachFnWouldClaimOn(win CollectWindow, reg *Registry, fnVal Value, idx int) bool {
	fd, isFn := fnVal.Data.(FnDefInfo)
	if !isFn || !fnHasForwardSigPast(fd, 0) {
		return false
	}
	probe, kind := ForwardClaimProbeOn(win, reg, idx)
	switch kind {
	case probeNone:
		return false
	case probeOptimistic:
		return true
	}
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if sig.TotalArgs() == 0 || sig.Fallback || sig.BarrierPos == 0 {
			continue // nothing to claim forward
		}
		if SigArgMatches(sig, 0, probe) {
			return true
		}
	}
	return false
}

// fnValueWouldWiden is the completion site's ARITY-WIDENING test: after a
// value-called collection completes at `completed` args, could a WIDER
// overload (more total args) claim the token at idx instead? The unified
// split rule lets the re-plan REDISTRIBUTE the already-collected args to
// the stack side of the wider sig (`concat parts {sep}` completes the
// 1-arg [List] plan; the 2-arg [Map List] overload then takes the map
// forward at sig[0] with the list deep on the stack at sig[1]), so the
// next token is tested against EVERY forward-eligible slot of each wider
// sig, not just position `completed`.
func (e *Engine) fnValueWouldWiden(fnVal Value, completed, idx int) bool {
	fd, isFn := fnVal.Data.(FnDefInfo)
	if !isFn || !fnHasForwardSigPast(fd, completed) {
		// No overload is wider than the completed collection — nothing
		// can widen, whatever the next token is. Checked BEFORE the
		// probe: an optimistic probe (a group / reach next) must not
		// suppress the seal of a single-arity value call (`m.l 5 m.l 7`
		// with a 1-arg lambda — the second reach is the next STATEMENT,
		// and skipping the seal here re-opened the misfire this whole
		// mechanism exists to close).
		return false
	}
	probe, kind := e.forwardClaimProbe(idx)
	switch kind {
	case probeNone:
		return false
	case probeOptimistic:
		return true
	}
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if sig.TotalArgs() <= completed || sig.Fallback || sig.BarrierPos == 0 {
			continue // not wider / nothing forward-eligible
		}
		limit := sig.BarrierPos
		if limit < 0 || limit > sig.TotalArgs() {
			limit = sig.TotalArgs()
		}
		for slot := 0; slot < limit; slot++ {
			if SigArgMatches(sig, slot, probe) {
				return true
			}
		}
	}
	return false
}

// dispatchModAt reports whether a dispatch-modifier marker (Word/__DM —
// the parser's `/v` / `/q` emission) sits at tape index idx. Used by the
// 0-arg property-call exception above to yield to explicit data intent.
func (e *Engine) dispatchModAt(idx int) bool {
	if idx >= e.Tape.Len() {
		return false
	}
	_, ok := AsDispatchMod(e.Tape.At(idx))
	return ok
}

// fnHasForwardSigPast reports whether the function carries some REAL
// forward-eligible overload with more than `floor` args — the
// precondition for both claim tests above. Without one, the fn can
// neither claim a first argument (floor 0) nor widen a completed
// collection (floor = the completed count), so even an optimistic probe
// (a group / reach next, result type unknowable) proves nothing.
func fnHasForwardSigPast(fd FnDefInfo, floor int) bool {
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if sig.TotalArgs() > floor && !sig.Fallback && sig.BarrierPos != 0 {
			return true
		}
	}
	return false
}

// tagReachCollapsedFn stamps ReachGroup provenance onto the single value
// a reach-lowered group (`m.p` → `( m dot p )`) collapsed to at idx,
// when that value is a NAMED function: an unmarked dot-access to a
// function is a CALL, never claimable data for a pending Any window
// (NUR038 — the value twin of the fn-word barrier). The tag is
// transient, exactly like Quoted: the arrival path clears it when the
// value is collected (a Function-typed slot — the designed reference
// intercept), and execFnDefLiteral clears it the moment the
// call-vs-data decision is made, so it never rides into a binding or a
// container. User-written parens carry no ReachGroup and tag nothing;
// `/v` data intent arrives Quoted and is never refused.
func (e *Engine) tagReachCollapsedFn(idx, closeIdx int, wasReachGroup bool) {
	if !wasReachGroup || closeIdx != idx+2 || idx >= e.Tape.Len() {
		return
	}
	v := e.Tape.At(idx)
	if fd, isFn := v.Data.(FnDefInfo); isFn &&
		fd.Name != "" && !fd.Anonymous && !v.Quoted {
		v.ReachGroup = true
		e.Tape.Set(idx, v)
	}
}

// fnReturnPark reports how far past a collapsed paren's start the pointer must
// land: 1 when the value the rewind would re-step is an unquoted Function, 0
// otherwise.
//
// THE SURVIVOR COUNT IS THE QUESTION, and `closeIdx == idx+2` is how it is
// asked. NUR101's 2026-08-26 "place uniformly" ruling was first implemented by
// DELETING that clause, on the reasoning that whether the rewind re-steps a
// Function is a property of the value alone and not of how many values sit
// beside it. Measured 2026-08-27, that is false: with more than one survivor
// the park must DECLINE, because the rewind then lands ON the leading value
// and stepLiteral dispatches it — which is the whole of
// `(x:Integer => [x mul 2] 5)` being 10. Deleting the clause made that program
// `fn (Integer) 5` and broke seven suites. Do not remove it again
// (design/PAREN-RESTEP-RULE.0.md; TestFnReturnPark's more-than-one-survivor
// row pins it).
//
// So placement is the ONE-survivor case, and the enclosing group is a SECOND
// decision taken one paren out — not a context that modifies this one.
// `(inc/v) 7` places while `((inc/v) 7)` is 8, and both are this function
// answering correctly at two different parens: the inner collapse has one
// survivor and parks, the outer has two and declines. recordParenReStep, called
// by stepCloseParen right after this, records that second decision for the
// compiler, which sees the identical residual for both spellings and cannot
// otherwise tell them apart.
//
// A paren places its value on the forward stack and never re-steps it —
// reference and inline literal alike (NUR073's BROAD verdict, maintainer
// clause 3, 2026-08-17: "parens do not re-step; they place a value or values
// on the forward stack") — so stepCloseParen's rewind must step PAST that
// value rather than re-step it into a call
// (design/FUNCTION-VALUE-SCOPE.0.md §12.6; design/FN-VALUE-OPEN-WORK.0.md §2).
//
// This is the ParkResult idiom (see spliceMatchResults), which `valof` already
// uses, and it is deliberately POSITIONAL: inertness is a property of the
// pointer at one index at one moment, and nothing is stamped on the value.
// That matters because `/v` is NOT sticky — the parked value must still
// dispatch at its next use, read back from a map or handed to `apply`. The two
// value-borne markers nearby are both wrong here: `Quoted` travels with the
// value and would make it permanently inert, and `ReachGroup` is a barrier
// hint of the opposite polarity.
//
// The ONE exclusion is a reach-lowered group (`m.p` → `( m dot p )`), whose
// re-step IS the dispatch — `MathUtil.sqrt 16` fails without it — so the
// caller hands in notReachGroup (snapshotted before the pair removals destroy
// the open paren) rather than the pre-BROAD frame-open test. The
// exactly-one-survivor test is the same one tagReachCollapsedFn uses.
// Returning the OFFSET rather than a bool keeps the branch out of
// stepCloseParen, which sits at its cyclomatic cap (NUR038).
//
// The value test MIRRORS stepLiteral's dispatch guard exactly — same three
// clauses, same order — because the invariant is "park iff the re-step would
// have CALLED it". A looser test (dropping the TFunction check, so a Function
// reparented to a refined function type also parks) would step past a value
// stepLiteral would merely have pushed, silently losing its OnPushLit for any
// installed Recorder. That mirroring is why the test lives in the shared
// fnValueDispatchesAtPointer rather than being spelled twice.
func (e *Engine) fnReturnPark(idx, closeIdx int, notReachGroup bool) int {
	if !notReachGroup || closeIdx != idx+2 || idx >= e.Tape.Len() {
		return 0
	}
	v := e.Tape.At(idx)
	if v.ReachGroup {
		// The VALUE-borne twin of the group exclusion: a dot-accessed named
		// fn collapsed by an inner reach group (tagReachCollapsedFn) and now
		// sitting alone in a USER paren. An unmarked dot-access to a
		// function is a CALL, never claimable data (NUR038) — the re-step
		// IS its dispatch, so the park must decline it.
		return 0
	}
	if fnValueDispatchesAtPointer(v) {
		// Record the placement here too. This arm catches the value the CHECK
		// pass folded to a concrete Function — an inert `/v` reference, a
		// `valof`, an inline literal — and it returns before the carrier arm
		// below ever asks the braid, so without this the residual layout never
		// learns the lead was placed and refuses `(inc/v) 7` against its own
		// interpreted answer of `fn inc(Integer) 7`.
		//
		// Reach groups are excluded above, so reaching here proves a USER
		// paren. The record is read by placedNotReStepped (compiler/go/emit.go),
		// which needs BOTH halves of the pair — placed here, re-stepped by
		// recordParenReStep — to prove nothing downstream applies the value.
		CheckBraid.ParenPlacedFnCarrier(e, idx)
		return 1
	}
	// The CHECK pass's twin of the same decision. Where the interpreter
	// holds a concrete Function, an analysis pass holds a DYNAMIC fn-typed
	// carrier (a member read, a call result) — and that carrier is what
	// would dispatch at the pointer there, so it must park identically or
	// the lanes disagree about a user paren. Measured: `(m dot f) 5` parked
	// interpreted and applied compiled, the divergence
	// TestCompiledCombinationParity caught when the BROAD park landed.
	// Reach groups never reach here (both exclusions above), so dot SUGAR
	// keeps dispatching on both lanes.
	// The carrier need not be DYNAMIC. A returned closure reaches the check
	// pass as a NON-dynamic fn-typed carrier, and gating this arm on Dynamic
	// alone let it fall through both arms — no park, so the rewind re-stepped
	// it into a call and `((mk 1) 2)` answered 3 compiled where the
	// interpreter placed (NUR101). Measured: disp=false dyn=false
	// carrier=true, which no arm claimed.
	//
	// KNOWN COST, and it is the sticky-inertness hazard this function's own
	// header warns about. Parking here is positional and stamps nothing on
	// the value, exactly as intended — but the compiler learns about it
	// through ParenPlacedFnIDs, which is keyed by value ID, and an ID travels
	// with a binding. So `def h (mk 1) end  h 2` now refuses ("fn value
	// precedes residual args"): h's value carries the placed mark from the
	// paren that produced it, and the residual lowering declines to apply a
	// lead it believes was placed — even though `h` is a bare-NAME dispatch
	// that must still call. Sound (it falls back and answers 3), but a
	// compiled-coverage loss.
	//
	// The member-fn case does not hit this because a member read is consumed
	// at its site and never rebound. The general carrier is. Closing it means
	// making the compiler's placement signal POSITIONAL like this one, rather
	// than value-keyed — not widening the ID table further.
	if !v.Quoted && (v.Dynamic || IsFnTypedCarrier(v)) {
		// Ask the braid FIRST, and unconditionally. It is not only a
		// predicate: it RECORDS the placement in ParenPlacedFnIDs for the
		// compiler's residual lowering, which must not lower a placed lead
		// as an apply. Behind an `||` after IsFnTypedCarrier it never ran for
		// a genuine carrier — the short-circuit meant the one case the
		// compiler most needs to know about was the one case never recorded,
		// so `((mk 1) 2)` placed interpreted and applied compiled (NUR101).
		placed := CheckBraid.ParenPlacedFnCarrier(e, idx)
		// A DYNAMIC value whose static bound does not exclude Function parks
		// too. At run time the interpreter holds the concrete value and parks
		// through fnValueDispatchesAtPointer above; the check pass holds
		// dynamic(Any) and cannot prove callability, so without this arm no
		// park claims it, the pass models an apply, and the compiler bakes
		// one — `((tbl get k) 5)` answering 10 compiled against a placed
		// `fn (Integer) 5` interpreted (NUR101, frontier-hof-audit:72).
		//
		// Parking a MAYBE-callable is the sound direction, and the asymmetry
		// is the point: placing a value that turns out non-callable costs
		// nothing, because a non-callable value sitting before other args
		// just sits there — exactly what the unparked path would have left.
		// Applying one that turns out callable is a wrong answer. The
		// not-disjoint-against-Function test is the same rule the residual
		// lowering's auto-dispatch guard already uses, so both ends agree on
		// what "might be callable" means.
		if IsFnTypedCarrier(v) || placed {
			return 1
		}
	}
	return 0
}

// fnValueDispatchesAtPointer reports whether the main loop, on re-encountering
// v at the pointer, would hand it to execFnDefLiteral rather than PUSH it as a
// literal. It is stepLiteral's dispatch guard verbatim — same three clauses,
// same order — and is the single home of that test: fnReturnPark asks it to
// decide whether a fn frame's return is a delivery, and stepCloseParen's
// recorder accounting asks it to decide whether a survivor will fire OnPushLit.
// Keep it in lockstep with stepLiteral if that guard ever moves.
//
// The recorder half matters because the two answers must agree: a value that
// dispatches (or is stepped past) never fires OnPushLit, so crediting it a
// skip leaves an unspendable credit that silently swallows the NEXT real
// literal — design/FN-VALUE-OPEN-WORK.0.md §5.2.
func fnValueDispatchesAtPointer(v Value) bool {
	return v.Parent.Equal(TFunction) && isFnDefValue(v) && !v.Quoted
}

// FnValueDispatchesAtPointer is the main loop's own dispatch predicate, for
// the VM's re-step deopt (NUR124): would v, back on the tape, dispatch when
// stepped — an unquoted fn value (fnValueDispatchesAtPointer), or an unquoted
// compiled closure, which execFnDefLiteral bridges to one
// (CompiledRuntime.ClosureAsFnDef) and then treats the same way.
func FnValueDispatchesAtPointer(v Value) bool {
	if fnValueDispatchesAtPointer(v) {
		return true
	}
	_, closure := v.Data.(ClosurePayload)
	return closure && v.Parent.Equal(TFunction) && !v.Quoted
}

// noteFnResultReSteps is the check pass's RE-STEP note (NUR124). A native
// word's results go back onto the tape at the call's position
// (spliceMatchResults) and the main loop steps them, so an unquoted fn
// value among them DISPATCHES where it lands — collecting forward over the
// tokens written after the call, then from the stack beneath it
// (execFnDefLiteral): `[g/v] each [5 swap 7]` is [21], and `[5 swap drop]`
// applies g to 5 before drop runs. The pass cannot make that dispatch: a
// fn-typed CARRIER has no signatures, so stepLiteral steps it past as
// data, and a unit compiled from the model keeps the value inert where the
// runtime applies it. The note hands the recorder each result the
// interpreter would re-step and the token it resumes at, so the unit's
// lowering can give exactly that re-step to the interpreter (a deopt over
// the results) or decline.
//
// Not noted, each because the interpreter would not dispatch there either
// or because another model owns the point: a concrete FnDefInfo result
// (the pass dispatches it itself); a quoted value (data on both lanes); a
// user fn's result (a sig with an fn frame — parked where it lands,
// design/PAREN-RESTEP-RULE.0.md); a code-body word's results (a Callable
// sig — `do`'s residual, `fold`'s accumulator: the body's own residual,
// which the closure lowering models at the body's finish — its trailing
// apply, the park rule, the residual-lead arms — and the pass's carrier
// view of it is not the lowering's); results a pending forward is collecting
// (the arrival rules own them); and results nothing plain follows — the
// enclosing paren's close (the paren places them and the collapse
// re-steps under the park rule), a dispatch modifier (`/v` leaves the
// value inert), a frame or control marker, an already-placed value, or
// the end of the tape (the residual arms own a trailing fn).
func (e *Engine) noteFnResultReSteps(sig *Signature, callEnd int, results []Value) {
	es := e.Registry.analysisRecorder()
	if !es.Active() || sig == nil || sig.FnFrame() != nil || sig.Callable != nil || callEnd+1 >= e.Tape.Len() {
		return
	}
	next := e.Tape.At(callEnd + 1)
	if !plainReStepToken(next) || e.hasPendingForwardCollecting() {
		return
	}
	for _, r := range results {
		if r.Quoted || r.ID == "" || IsConcrete(r) {
			continue
		}
		if IsFnTypedCarrier(r) || (r.Dynamic && SigTypeMatches(r, TFunction)) {
			es.NoteFnResultReStep(r, next.Pos())
		}
	}
}

// plainReStepToken reports whether t is an ordinary, not-yet-stepped body
// token the interpreter steps after a call's results — a word, a paren
// group, a reach, a sugar marker, or a literal the source wrote — rather
// than a structural marker, a dispatch modifier, or a value the pass has
// already placed there (a carrier, or anything without a source position).
func plainReStepToken(t Value) bool {
	if t.Pos().Row == 0 || t.Carrier || t.Dynamic {
		return false
	}
	if _, mod := AsDispatchMod(t); mod {
		return false
	}
	if IsWord(t) || IsParenExpr(t) || isEvalReach(t) || IsSugar(t) {
		return true
	}
	return IsRecordableLiteral(t)
}

// creditParenSurvivorSkips tells an installed RecorderSkipper how many of the
// just-collapsed paren's survivors it should ignore when the rewind makes the
// main loop re-encounter them: they were already emitted to the recorder
// during the in-paren execution (via stepLiteral, or via execMatch for handler
// results). Free-function body of stepCloseParen's recorder hook, extracted
// for that function's cyclomatic cap (NUR038).
//
// A survivor earns a credit only if the re-step would fire OnPushLit for it.
// Two kinds never do, and crediting either leaves an unspendable skip that
// silently swallows the NEXT real literal:
//   - a FUNCTION VALUE, which execFnDefLiteral dispatches (or, for the
//     ADR-016 0-arg anonymous gate, steps past) — never a push;
//   - the PARKED return, the one survivor the rewind steps past.
//
// fnValueDispatchesAtPointer answers both: a park is by construction a value
// that satisfies it, so excluding dispatchers covers the parked case too and
// no `park` term is subtracted here. (The park half was raised on PR #375;
// the function-value half is design/FN-VALUE-OPEN-WORK.0.md §5.2, same class
// as the frame-skeleton over-count fixed in 9bffdd3.)
//
// closeIdx is the CloseParen's PRE-removal index, so the surviving contents
// now sit at [openIdx .. closeIdx-2] inclusive — the same span as
// [openIdx, closeIdx-1) in the original indices, minus 1 for the removed
// OpenParen.
func (e *Engine) creditParenSurvivorSkips(openIdx, closeIdx int, reStepped bool) {
	skipper, ok := e.recorder.(RecorderSkipper)
	if !ok || e.recorder == nil {
		return
	}
	end := closeIdx - 1
	if end > e.Tape.Len() { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		end = e.Tape.Len()
	}
	// Collapsed off the main loop (reStepped false) the span starts at
	// openIdx, NOT the post-park pointer: a PARKED survivor is about to be
	// COLLECTED exactly like its unparked siblings — the collection hook
	// fires OnPushLit for it at match completion — so its in-paren emission
	// is owed the same credit. Since the BROAD park (NUR073 clause 3) a
	// user paren parks its collapsed fn too, which is how `g (z/v) 777`
	// re-found the PR #387 double-record without this start. On the main
	// loop (reStepped true) the parked value is stepped past and never
	// re-encountered, so it stays outside the span.
	start := e.Pointer
	if !reStepped {
		start = openIdx
	}
	// A Function forfeits its credit ONLY when the main loop will re-step it,
	// because only there does stepLiteral hand it to execFnDefLiteral without
	// a push. Collapsed off the main loop (reStepped false) the survivor is
	// about to be COLLECTED as a forward argument — which is the whole point
	// of a `Function`-typed slot — and the collection hook fires OnPushLit for
	// it once the match completes, so the credit is owed. Raised by the PR
	// #387 review: `g (z/v) 777` into a Function slot recorded `fn z` twice
	// without this split.
	//
	// Kept as one boolean rather than guard clauses: an early `continue` is a
	// statement of its own, and a non-recordable survivor in this span is not
	// reachable from the suites, so the guard form cannot meet ADR-008.
	survived := 0
	for i := start; i < end; i++ {
		sv := e.Tape.At(i)
		if IsRecordableLiteral(sv) && !(reStepped && fnValueDispatchesAtPointer(sv)) {
			survived++
		}
	}
	if survived > 0 {
		skipper.Skip(survived)
	}
}

// viableConsumesAt reports whether any still-viable signature collects a
// forward argument at position pos (i.e. pos is within its barrier).
// Free-function body of resolveForwardArgs' viableConsumes closure,
// extracted for that function's complexity cap.
func viableConsumesAt(viable []ViableSig, pos int) bool {
	for _, vs := range viable {
		if pos < vs.Barrier {
			return true
		}
	}
	return false
}

// scanBoundaryToken reports whether a token unconditionally stops the
// forward pre-scan: engine-internal structurals (parked forwards,
// marks, moves, internals, return checks) and the statement bounds
// (end / close paren). Free-function body of resolveForwardArgs' two
// boundary tests, extracted for that function's complexity cap.
// effectiveForwardLimit is the forward/stack split point for ONE
// signature under ONE word's dispatch modifiers: the declared BarrierPos,
// overridden wholesale by /s (nothing collects forward) and /f (everything
// does).
//
// Shared by the two collection walks that both need it — the phase-1 plan
// walk (resolveForwardArgs, which maxes it over the viable set) and the
// per-candidate scan inside MatchSignature (which applies it per
// candidate). They compute it identically and always must: the split point
// is a property of the signature and the modifiers, not of which walk is
// asking. resolveForwardArgs' comment already described its copy as
// "mirroring matchSignature's forwardLimit computation" — a mirror is a
// drift waiting to happen, so there is now one of it
// (design/FULL-COMPILATION.0.md §6.2).
func effectiveForwardLimit(sig *Signature, w WordInfo) int {
	switch {
	case w.ForceStack:
		return 0
	case w.ForceForward:
		return sig.TotalArgs()
	}
	return sig.BarrierPos
}

func scanBoundaryToken(tok Value) bool {
	if IsForward(tok) || tok.Parent.ConformsTo(TMark) || tok.Parent.ConformsTo(TMove) ||
		tok.Parent.ConformsTo(TInternal) || tok.Parent.ConformsTo(TReturnCheck) {
		return true
	}
	return IsEnd(tok) || IsCloseParen(tok)
}

// expandScanSugar is resolveForwardArgs' sugar-marker arm (extracted for
// that function's complexity cap): it lowers the marker at scanIdx in
// place and reports whether the scan should reprocess the slot (false =
// treat the marker as a boundary). The Angle marker's head/use-site
// choice is made from the viable overload set: a /q (QuoteArgs) slot at
// this position wants the generic-def HEAD (`Name gen [params]` — every
// binder overload shares the /q name shape); anything else gets the
// use-site apply. The expansion is gated on viableConsumesAt — a TAPE
// MUTATION that outlives this dispatch, so a marker in the window of a
// pruned overload must survive intact for the NEXT word's dispatch to
// expand with ITS viable set (`import "m" def Box<T> …` scans past `def`
// — a /q slot keeps it walkable — and must not commit the def-head
// marker to import's use-site form).
func (e *Engine) expandScanSugar(tok Value, pos, scanIdx int, viable []ViableSig) (bool, error) {
	if !viableConsumesAt(viable, pos) {
		return false, nil
	}
	sinfo, sok := AsSugar(tok)
	if !sok { //covergate:allow IsSugar guarantees a SugarInfo payload
		return false, nil
	}
	headForm := false
	if sinfo.Kind == SugarAngle {
		for _, vs := range viable {
			if vs.Sig.QuoteArgs != nil && vs.Sig.QuoteArgs[pos] {
				headForm = true
				break
			}
		}
	}
	exp, serr := SugarExpansion(e.Registry, sinfo, tok, headForm)
	if serr != nil {
		if headForm {
			// The viable set SELECTED the generic-def head — a /q binder
			// slot wants this marker — so a failing head expansion is
			// the USER'S error (`def Box<t>`: params must be
			// capitalised; a dangling `extends`; an unbound gen-head
			// role), surfaced now. A use-site failure stays a boundary:
			// the marker survives for step time to surface it.
			return false, serr
		}
		return false, nil
	}
	e.Tape.Splice(scanIdx, 1, exp...)
	return true, nil
}

// reachCallHeadBarrier is resolveForwardArgs' NUR038 stop test: a tagged
// reach-collapsed named fn at scanIdx that WOULD CLAIM its next token is
// a CALL head — the fn-word barrier's value twin — and stops the forward
// scan. A claim-less fn is an operand, and so is one filling a
// FUNCTION-conforming slot of some still-viable overload at pos
// (`usurp (m dot a)` — the higher-order consumer wants the fn itself;
// Any slots stay barred: Any also admits a fn value, but as a swallowed
// call head, which is the misfire the barrier exists for).
func (e *Engine) reachCallHeadBarrier(tok Value, viable []ViableSig, pos, scanIdx int) bool {
	return ReachCallHeadBarrierOn(e.Tape, e.Registry, tok, viable, pos, scanIdx)
}

// ReachCallHeadBarrierOn is the fn-word barrier's VALUE twin (NUR038) over an
// explicit window and registry, so the interpreter and the VM's region
// adapter share one answer rather than two that agree until they do not.
func ReachCallHeadBarrierOn(win CollectWindow, reg *Registry, tok Value, viable []ViableSig, pos, scanIdx int) bool {
	if !tok.ReachGroup || tok.Quoted || !isFnDefValue(tok) {
		return false
	}
	for _, vs := range viable {
		if pos < vs.Barrier && sigWantsFunctionAt(vs.Sig, pos) {
			return false // the fn is this overload's own Function operand
		}
	}
	return ReachFnWouldClaimOn(win, reg, tok, scanIdx+1)
}

// sigWantsFunctionAt reports whether sig position pos declares a
// Function-conforming operand slot — a slot for which a reach-collapsed
// fn value is DATA (a higher-order word's Function param), exempt from
// the NUR038 call-head barrier. An Any slot is NOT a Function slot: Any
// also admits a fn value, but as a swallowed call head, which is exactly
// the misfire the barrier exists for.
func sigWantsFunctionAt(sig *Signature, pos int) bool {
	if pos >= sig.TotalArgs() {
		return false
	}
	st := SigArgType(sig, pos)
	return st != nil && st.ConformsTo(TFunction)
}

// Probe classifications returned by forwardClaimProbe.
const (
	probeNone       = iota // boundary / barrier / unbound: no claim provable
	probeOptimistic        // expression result unknowable: optimistic claim
	probeValue             // a concrete probe value to sig-match
)

// forwardClaimProbe classifies the token at idx for the claim tests
// above: probeNone (a boundary / fn-word barrier / unbound word /
// nothing there — no claim is provable), probeOptimistic (a group /
// reach / interp expression whose result type is unknowable — the
// runtime collects it optimistically), or probeValue with the value the
// token would contribute (a literal, a binding's value, a `/v`
// reference).
func (e *Engine) forwardClaimProbe(idx int) (Value, int) {
	return ForwardClaimProbeOn(e.Tape, e.Registry, idx)
}

// ForwardClaimProbeOn is forwardClaimProbe over an explicit window and
// registry rather than an Engine's own.
//
// The extraction is what keeps ONE implementation for TWO hosts. The VM's
// region adapter has to answer this question identically to the interpreter
// — "compiled code must work exactly the same as interpreted" is not a
// property that survives a second, parallel implementation of a
// classification this subtle (an fn word is a barrier, a plain binding steps
// to its value, `/v` is one Function datum, true/false/none are reserved
// literals). Reading the same code is the only version of that guarantee
// that cannot drift.
//
// Nothing beyond (window, registry) was ever needed: the method body used
// e.Tape and e.Registry and no other Engine state, so this is a
// receiver-to-parameter change and not a refactor of the logic.
func ForwardClaimProbeOn(win CollectWindow, reg *Registry, idx int) (Value, int) {
	if idx >= win.Len() {
		return Value{}, probeNone
	}
	v := win.At(idx)
	switch {
	case IsEnd(v) || IsCloseParen(v) || IsForward(v):
		return Value{}, probeNone
	case IsOpenParen(v) || IsParenExpr(v) || IsReach(v) || IsInterpString(v) || IsXmlInterp(v):
		return Value{}, probeOptimistic
	case IsWord(v):
		wi, werr := AsWord(v)
		if werr != nil { //covergate:allow AsWord cannot fail after an IsWord guard — the payload IS a WordInfo (§engine)
			return Value{}, probeNone
		}
		if wi.ForceVal {
			// `/v`: one Function reference datum
			return Value{Parent: TFunction, Data: FnDefInfo{}}, probeValue
		}
		top, bound := reg.Defs.Top(wi.Name)
		switch {
		case bound:
			if _, topFn := top.Data.(FnDefInfo); topFn {
				return Value{}, probeNone // an fn word is a barrier
			}
			return top, probeValue // a plain value binding steps to this value
		case wi.Name == "true" || wi.Name == "false":
			return NewBoolean(wi.Name == "true"), probeValue
		case wi.Name == "none":
			return NewNone(), probeValue // reserved literal, like true/false (polyReachBound parity)
		default:
			return Value{}, probeNone // unbound / type-name words: not a proven claim
		}
	default:
		return v, probeValue // a concrete literal
	}
}

// fnValueHasZeroArgSig reports whether a function value carries a 0-arg
// overload — the shape that makes an unmarked dot-read a PROPERTY call
// (it dispatches consuming nothing, so it can never swallow a following
// statement). Part of the NUR038 arrival gate's call-vs-data decision.
func fnValueHasZeroArgSig(v Value) bool {
	fd, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	for i := range fd.Signatures {
		if fd.Signatures[i].TotalArgs() == 0 && !fd.Signatures[i].Fallback {
			return true
		}
	}
	return false
}

// fnValueOnlyZeroArgSigs reports whether every REAL (non-Fallback)
// signature of a function is 0-arg and at least one exists — a pure
// property-read fn (`IO.stdin`, `TimeUtil.today-utc`). Such a member
// never forward-collects at a landing: it auto-fires with no operands
// and any following values belong to the NEXT dispatch. The single
// canonical predicate for both engines' models of that shape: the
// interpreter's NUR035 reach-group deferral exemption (execFnDefLiteral
// — it dispatches inside its group) and the shaped-method 0-arg landing
// model (method_shape.go — the 0-arg model applies even with an inert
// window after the member). A sig-less or fallback-only value answers
// false (nothing provable).
// MarkApplied stamps `apply`'s one-shot application signal on a fn value
// whose only signatures are 0-arg, and is the SINGLE source of that
// decision: the apply word's runtime handler, its check-mode model and the
// VM's apply-word op all call it, so no two engines can disagree about
// which values the re-step's inert-lambda gate must yield for (ADR-016,
// NUR077 §5 Hole 1). Anything else is returned untouched. See
// FnDefInfo.Applied for why the gate needs telling, and execFnDefLiteral
// for where the mark is consumed and cleared.
func MarkApplied(v Value) Value {
	fd, ok := v.Data.(FnDefInfo)
	if !ok || !FnValueOnlyZeroArgSigs(fd) || !fd.Anonymous || fd.Macro {
		return v
	}
	fd.Applied = true
	v.Data = fd
	return v
}

func FnValueOnlyZeroArgSigs(fd FnDefInfo) bool {
	real := false
	for i := range fd.Signatures {
		if fd.Signatures[i].Fallback {
			continue
		}
		if fd.Signatures[i].TotalArgs() != 0 {
			return false
		}
		real = true
	}
	return real
}

// implicitEnd resolves a forward early when a type mismatch occurs.
func (e *Engine) implicitEnd(fwdIdx int) error {
	fwd, _ := AsForward(e.Tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex
	collectedCount := fwd.CollectedArgs
	stackArgCount := fwd.StackArgs

	e.Tape.Remove(fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}

	e.curryOrStack(funcIdx, collectedCount, stackArgCount)
	return nil
}

// policyGateWord consults the engine scope's word policy before a
// registered word dispatches. Skips check mode (static analysis should
// see every word so type-checking remains meaningful) and engine
// markers (`__`-prefixed, used by internal lowering — never directly
// addressable from user code).
func (e *Engine) policyGateWord(name string) error {
	if e.Registry.analysisActive() || IsInternalMarker(name) {
		return nil
	}
	if wc := LookupWordChecker(e.Registry); wc != nil {
		return wc.CheckWord(name)
	}
	return nil
}

// policyGateModuleCall consults the per-export module policy before a
// module-export dispatch — the modules.call twin of policyGateWord
// (NUR045). gate is the ModuleCallID stamped onto the dispatched
// signature at module-resolution time (StampModuleCallGates); nil (a
// non-module signature) allows in one pointer test. Like the word
// gate it MUST skip check mode — static analysis sees every dispatch
// so type-checking stays meaningful, and denial is a runtime verdict.
// The checker error is returned verbatim so the compiled twin
// (vmContext.gateModuleCall) raises the identical denial.
func (e *Engine) policyGateModuleCall(gate *ModuleCallID) error {
	return policyGateModuleCallReg(e.Registry, gate)
}

// commitBarrierForward applies the argument-order rule's "another
// function word is a barrier" to a Forward that is parked waiting for
// ARRIVALS (its args came from paren results rather than the token
// walk, so the walk-time barrier check never saw the boundary). Called
// from stepWord when a function word is about to begin its own
// dispatch: if the nearest pending forward in the current paren scope
// can already fire with the args it holds — the same dispatch an
// explicit `end` would trigger — it is committed NOW, and the current
// word re-steps afterwards.
//
// This is what makes an else-less guard fire before the next statement
// runs: in `if (bad) [raise …] def q (10 div n)`, the parked `if`
// previously kept waiting for a possible else while `def` ran first —
// so the div-by-zero pre-empted the guard's raise, and any
// value-producing statement could be swallowed into the else slot. A
// pending forward that CANNOT yet fire keeps waiting, since its
// missing args may be the very results the stepping word produces
// (with `def g fn [[a:Any b:Any] [Any] [add a b]]`, `g 1 def x 5 x`
// keeps g waiting through the boundary and def's bound value feeds
// the second slot — forward-barrier.tsv §6; the same shape on a
// typed native like `add 1 def x 5 x` never parks at all, it fails
// loudly at plan time).
//
// Returns true when a forward was committed; the caller must return
// to the engine loop (the pointer has moved to the committed word).
func (e *Engine) commitBarrierForward() bool {
	// Nearest pending forward, stopping at open-paren scope barriers —
	// the same scan stepEnd performs.
	fwdIdx := -1
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwdIdx = i
			break
		}
	}
	if fwdIdx < 0 {
		return false
	}

	fwd, _ := AsForward(e.Tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex
	claimed := fwd.CollectedArgs + fwd.StackArgs
	if claimed == 0 {
		// Nothing collected yet — no smaller-arity dispatch to commit.
		return false
	}
	if funcIdx < 0 || funcIdx >= e.Tape.Len() || !IsWord(e.Tape.At(funcIdx)) {
		return false
	}
	w, _ := AsWord(e.Tape.At(funcIdx))
	fn := e.Registry.Lookup(w.Name)
	if fn == nil {
		return false
	}

	// Probe: would the parked word dispatch with ONLY its claimed args
	// (collected forward + claimed stack)? Deliberately tighter than a
	// whole-scope match — an implicit commit must not reach below the
	// args the word actually claimed. The claimed region sits directly
	// below the word: stack args first, then collected forward args in
	// arrival order (first-collected deepest) — which IS sig order, the
	// layout MatchSignature reads bottom-first.
	start := funcIdx - claimed
	if start < 0 {
		return false
	}
	var resolved []Value
	for i := start; i < funcIdx; i++ {
		v := e.Tape.At(i)
		if IsForward(v) || IsOpenParen(v) || IsMark(v) || IsMove(v) {
			continue
		}
		resolved = append(resolved, v)
	}
	if len(resolved) == 0 {
		return false
	}
	testW := WordInfo{Name: w.Name, ArgCount: -1, ForceStack: true}
	m := MatchSignature(fn.Signatures, resolved, testW)
	if m == nil || m.Sig == nil {
		return false
	}
	// The commit must consume EXACTLY the claimed args through a real
	// overload. boru-bodied fns carry a synthetic 0-arg Fallback in the
	// aggregate dispatch table (it exists to raise a clean
	// "no matching signature" error); matching it here would commit a
	// waiting word to its own failure — `g 1 def x 5 x` must keep
	// waiting for def's result when g has only a 2-arg overload, not
	// error at the boundary. Likewise a shorter real overload must not
	// fire while claimed args would be stranded.
	if m.Sig.Fallback || m.Sig.TotalArgs() != len(resolved) {
		return false
	}

	// The commit is correct; in check mode additionally leave a
	// non-gating note when the parked plan was SPECULATIVE — the
	// else-less-guard shape, where the planner had filled a trailing
	// slot from the very word now acting as the boundary. Emitted
	// before the mechanics below so e.pointer still names the
	// boundary word.
	CheckBraid.NoteSpeculativeBarrierCommit(e, fwd)

	// Commit exactly like the collection-complete path (stepLiteral's
	// CollectedArgs >= ExpectedArgs branch): drop the marker, force the
	// word to stack mode, and rearrange so the first-collected forward
	// arg sits on top — the engine matcher's top-first read then sees
	// the args in sig order. (NOT curryOrStack: its rearrange is gated
	// on StackArgs > 0, so a purely-arrival forward would re-dispatch
	// with its collected args reversed.)
	e.Tape.Remove(fwdIdx)
	if fwdIdx < funcIdx {
		funcIdx--
	}
	if funcIdx < e.Tape.Len() && IsWord(e.Tape.At(funcIdx)) {
		e.forceStackWord(funcIdx, w)
	}
	e.Pointer = funcIdx
	e.rearrangeForForward(fwd.StackArgs, fwd.CollectedArgs)
	return true
}

// strandedForwardError implements the strict-barrier rule's failure
// case (PROTOTYPE, see strictForwardBarrier): called from stepWord
// after commitBarrierForward has declined, it reports the nearest
// pending forward in the current paren scope as STRANDED — the
// boundary word will never feed it under the strict rule, and no
// overload can fire with what it holds. Returns nil when no forward
// is pending (the normal case). Engine-internal frame-tail words
// (__pa and friends) are exempt: they are not source-level statement
// boundaries.
func (e *Engine) strandedForwardError(boundary string) *BoruError {
	if strings.HasPrefix(boundary, "__") {
		return nil
	}
	fwdIdx := -1
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwdIdx = i
			break
		}
	}
	if fwdIdx < 0 {
		return nil
	}
	fwd, _ := AsForward(e.Tape.At(fwdIdx))
	missing := fwd.ExpectedArgs - fwd.CollectedArgs
	detail := fmt.Sprintf(
		"%s is still waiting for %d argument(s) when `%s` begins its own dispatch — "+
			"a function word is a barrier and never feeds forward collection (strict rule); "+
			"group the call in parens so its RESULT becomes the argument: %s (%s …)",
		fwd.FuncName, missing, boundary, fwd.FuncName, boundary)
	// A boundary word with a stack-barrier slot (`dot` and the accessor
	// family: the receiver sits beyond BarrierPos, readable only from the
	// ENCLOSING stack) may not work grouped — a paren seals the stack the
	// barrier slot must reach (NUR049: `def why (dot message)` starves every
	// candidate where the sequential form works). Offer the sequential
	// spelling alongside, so the help never names only a dead form.
	if e.barrierReceiverWord(boundary) {
		detail += fmt.Sprintf(
			"; note `%s` reads its receiver from the enclosing stack, which a paren "+
				"group seals off — if the grouped form cannot match, run it first and "+
				"bind its result in sequence instead: %s … %s",
			boundary, boundary, fwd.FuncName)
	}
	return makeBoruErrorAt("signature_error", detail, fwd.FuncName,
		e.effectiveSource(), "", fwd.Pos)
}

// barrierReceiverWord reports whether any registered signature of word
// reads a slot from the enclosing stack (BarrierPos < TotalArgs). For such
// a word the "group the call in parens" fix can starve the barrier slot —
// the paren seals the enclosing stack (NUR049) — so suggestions offer the
// sequential spelling too.
func (e *Engine) barrierReceiverWord(name string) bool {
	fd := e.Registry.Lookup(name)
	if fd == nil {
		return false
	}
	for i := range fd.Signatures {
		s := &fd.Signatures[i]
		if s.BarrierPos >= 0 && s.BarrierPos < s.TotalArgs() {
			return true
		}
	}
	return false
}

// stepEnd handles the "end" keyword.
func (e *Engine) stepEnd() error {
	// Statement boundary: void-group records do not blame failures
	// across statements (ERRORS.8.md §3).
	e.voidGroups = e.voidGroups[:0]
	endIdx := e.Pointer

	// Find nearest pending forward, stopping at open-paren barriers.
	fwdIdx := -1
	for i := endIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwdIdx = i
			break
		}
	}

	if fwdIdx < 0 {
		e.Tape.Remove(endIdx)
		return nil
	}

	fwd, _ := AsForward(e.Tape.At(fwdIdx))
	funcIdx := fwd.FuncIndex

	// Remove forward and end from the stack.
	// Remove higher index first to preserve lower indices.
	if endIdx > fwdIdx {
		e.Tape.Remove(endIdx)
		e.Tape.Remove(fwdIdx)
		if fwdIdx < funcIdx {
			funcIdx-- // forward removal
		}
		// end was already removed (endIdx > fwdIdx), endIdx > funcIdx always
	} else { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		e.Tape.Remove(fwdIdx)
		newEndIdx := endIdx
		if fwdIdx < endIdx { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			newEndIdx--
		}
		e.Tape.Remove(newEndIdx) //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		if fwdIdx < funcIdx {    //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			funcIdx--
		}
		if newEndIdx < funcIdx { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			funcIdx--
		}
	}

	e.curryOrStack(funcIdx, fwd.CollectedArgs, fwd.StackArgs)
	return nil
}

// stepMark records the mark's ID in the marks hash table and advances.

// isPendingResidualContainer reports whether v is a pending
// (unevaluated) plain list/map — exactly the shape stepDefCleanup's
// EvalResidual scan evaluates in-frame. Shared with probeTailCall so
// the TCO gate and the marker can never disagree about what the
// eager teardown would evaluate.
func isPendingResidualContainer(v Value) bool {
	if !v.Eval || v.Quoted || v.Parent == nil {
		return false
	}
	if v.Parent.Equal(TMap) {
		return v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v)
	}
	if v.Parent.Equal(TList) {
		return v.Data != nil && !IsTypedList(v) && !IsTableType(v)
	}
	return false
}

// stepDefCleanup removes defs that were created during fn body execution.
// The DefCleanupInfo carries a snapshot of DefStacks lengths taken before
// the body ran. Any defs added since are popped via UninstallDef.
func (e *Engine) stepDefCleanup(val Value, markerIdx int) error {
	info, _ := AsDefCleanup(val)
	if info.EvalResidual {
		// A COMPUTING body's residual pending containers evaluate
		// IN-frame — before the body-local defs pop — so the spliced
		// dispatch path agrees with the CallBoru sub-run drain (mini-s3's
		// s3-parse-range trailing `{from: from upto: upto}`; previously
		// the spliced path deferred the container to the CONSUMER scope,
		// where the body-locals are gone — the residual-timing fork,
		// design/NET-COMPILE-FRONTIER.0.md addendum 2). The frame is a
		// paren group and every nested group below has already collapsed,
		// so the first OpenParen below the marker is the frame's own open:
		// the scan touches exactly the frame's residual, never a caller
		// value. Single-literal bodies leave EvalResidual false and keep
		// the no-closures transparency (def-node-binding.tsv §3).
		//
		// Multiple residuals evaluate BOTTOM-UP (frame order = source
		// order), matching the end-of-run sweep's stack order — walking
		// down from the marker would run their side effects in reverse.
		lo := 0
		for i := markerIdx - 1; i >= 0; i-- {
			if IsOpenParen(e.Tape.At(i)) {
				lo = i + 1
				break
			}
		}
		for i := lo; i < markerIdx; i++ {
			v := e.Tape.At(i)
			if !isPendingResidualContainer(v) {
				continue
			}
			var ev Value
			var err error
			if v.Parent.Equal(TMap) {
				ev, err = e.AutoEvalMap(v, false, true)
			} else {
				ev, err = e.autoEvalList(v, true)
			}
			if err != nil {
				// The error unwinds the run, so the frame's remaining
				// parked tail (__pa, the undef pairs) never steps —
				// replay its registry effects before propagating,
				// exactly as CallBoru's inline cleanup runs on a body
				// error. Otherwise a do-error trap upstream would
				// resume with the callee's params/args/locals still
				// bound in the caller's scope.
				e.unwindFrameTailOnError(info, markerIdx)
				return err
			}
			ev.Eval = false
			e.Tape.Set(i, ev)
		}
	}
	if info.SkipCleanup {
		// The frame installs no body-local defs — nothing to truncate,
		// and no Names() scan to pay (design/INTERPRETER-SPEED-PLAN.10.md #5).
		return nil
	}
	truncateFrameDefs(info)
	return nil
}

// TruncateFrameDefs pops every def binding installed in reg since the
// snapshot (Defs.Snapshot) — the DefCleanup marker's truncation duty, for a
// caller that ran a body region outside a frame (the VM's deopt island).
func TruncateFrameDefs(reg *Registry, snapshot map[string]int) {
	truncateFrameDefs(DefCleanupInfo{Snapshot: snapshot, Registry: reg})
}

// truncateFrameDefs pops every def binding installed since the frame's
// entry snapshot — the DefCleanup marker's truncation duty.
func truncateFrameDefs(info DefCleanupInfo) {
	reg := info.Registry
	for _, name := range reg.Defs.Names() {
		prevLen := info.Snapshot[name] // 0 for names not in snapshot
		for reg.Defs.Depth(name) > prevLen {
			UninstallDef(reg, name)
		}
	}
}

// unwindFrameTailOnError replays the frame tail's registry effects when
// the in-frame residual evaluation raises: the truncation this marker
// owns, then — best-effort, only when the canonical parked tail is
// actually next on the tape — the __pa Args/baseline pop and the undef
// pairs, exactly the operations the parked tokens would have performed
// had the error not discarded them. Mirrors probeTailCall's forward walk.
func (e *Engine) unwindFrameTailOnError(info DefCleanupInfo, markerIdx int) {
	if !info.SkipCleanup {
		truncateFrameDefs(info)
	}
	i := markerIdx + 1
	if i >= e.Tape.Len() {
		return
	}
	if w, err := AsWord(e.Tape.At(i)); err != nil || w.Name != "__pa" {
		// A bare marker outside a canonical frame tail (a sweep re-run,
		// a synthetic tape) — nothing further to replay.
		return
	}
	if err := PopFrameArgs(e.Registry); err != nil {
		return
	}
	i++
	for i+1 < e.Tape.Len() {
		u, err := AsWord(e.Tape.At(i))
		if err != nil || u.Name != "undef" || !u.ForceForward {
			break
		}
		nm, err := AsWord(e.Tape.At(i + 1))
		if err != nil {
			break
		}
		UninstallDef(e.Registry, nm.Name)
		i += 2
	}
}

func (e *Engine) stepMark(val Value) {
	info, _ := AsMark(val)
	if e.marks == nil {
		e.marks = make(map[string]bool)
	}
	e.marks[info.ID] = true
	if e.trace != nil {
		e.traceNote = "mark " + info.ID
	}
	e.Pointer++
}

// stepMove jumps the pointer back to the corresponding mark, replaying the
// original body. Both the mark and the move are removed from the stack after
// the jump to prevent infinite loops. If the target mark is not found, an
// error is returned using the move's reason metadata.
//
// When the move carries a ForCont (for-loop continuation), stepMoveCont is
// called instead of the basic one-shot replay.
func (e *Engine) stepMove(val Value) error {
	info, _ := AsMove(val)
	moveIdx := e.Pointer

	if e.marks == nil || !e.marks[info.To] {
		return e.runtimeError("move_error", fmt.Sprintf("mark %q not found (%s)", info.To, info.Reason), info.To, "")
	}

	// Scan the stack to find the mark's current position.
	markIdx := -1
	for i := 0; i < e.Tape.Len(); i++ {
		_as2, _ := AsMark(e.Tape.At(i))
		if IsMark(e.Tape.At(i)) && _as2.ID == info.To {
			markIdx = i
			break
		}
	}
	if markIdx < 0 {
		// Mark was removed from the stack (e.g. by a for-loop controller
		// signalling loop completion). Remove this orphaned move quietly.
		delete(e.marks, info.To)
		e.Tape.Remove(e.Pointer)
		if e.trace != nil {
			e.traceNote = fmt.Sprintf("move orphan %s", info.To)
		}
		return nil
	}

	// Delegate to continuation handler for for-loops.
	if info.Cont != nil {
		return e.stepMoveCont(markIdx, moveIdx, info)
	}

	// Delegate to if-statement continuation handler.
	if info.IfCont != nil {
		return e.stepMoveIf(markIdx, moveIdx, info)
	}

	// Get the saved body from the mark.
	markInfo, _ := AsMark(e.Tape.At(markIdx))

	// Remove from hash table.
	delete(e.marks, info.To)

	// Replace everything from mark through move (inclusive) with the body copy.
	body := make([]Value, len(markInfo.Body))
	copy(body, markInfo.Body)
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, body...)

	if e.trace != nil {
		e.traceNote = fmt.Sprintf("move→mark %s", info.To)
	}

	// Set pointer to where the mark was (now the start of the replayed body).
	e.Pointer = markIdx
	return nil
}

// stepMoveCont handles a for-loop continuation move. It collects this
// iteration's results, advances the iterator, and either splices in a new
// mark+body+move for the next iteration or finalizes the loop.
func (e *Engine) stepMoveCont(markIdx, moveIdx int, info MoveInfo) error {
	cont := info.Cont

	// A while-mode continuation alternates condition and body regions
	// rather than counting an iterator — its own driver owns the move.
	if cont.WhileCond != nil {
		return e.stepMoveWhile(markIdx, moveIdx, info)
	}

	// Collect resolved values between mark and move (this iteration's output).
	for j := markIdx + 1; j < moveIdx; j++ {
		cont.Results = append(cont.Results, e.Tape.At(j))
	}

	// Advance iterator.
	cont.Current += cont.Step

	// Check if more iterations remain.
	moreIterations := (cont.Step > 0 && cont.Current < cont.End) ||
		(cont.Step < 0 && cont.Current > cont.End)

	if moreIterations {
		// Update iterator: uninstall old value, install new one.
		// This keeps the DefStacks depth at 1 throughout the loop.
		UninstallDef(cont.Registry, cont.IterName)
		InstallDef(cont.Registry, cont.IterName, NewInteger(cont.Current))

		// Generate new mark ID.
		id := NextMarkID()

		// Build replacement `mark + body + move` into a reused scratch
		// buffer. The mark stores cont.Body (stable, for the next re-mark);
		// the body elements appended after it are the executable copy — no
		// separate bodyCopy is needed because Tape.Splice copies the whole
		// token run into the tape, leaving cont.Body untouched.
		body := cont.Body
		tokens := e.loopTokens[:0]
		tokens = append(tokens, NewMark(id, body...))
		tokens = append(tokens, body...)
		tokens = append(tokens, NewMoveCont(id, info.Reason, cont))
		e.loopTokens = tokens

		// Remove old mark ID, register new one.
		delete(e.marks, info.To)
		e.Tape.Splice(markIdx, moveIdx-markIdx+1, tokens...)
		if e.marks == nil {
			e.marks = make(map[string]bool)
		}
		e.marks[id] = true

		// Set pointer to the new mark so stepMark processes it.
		e.Pointer = markIdx
		if e.trace != nil {
			e.traceNote = fmt.Sprintf("for next %s i=%d", id, cont.Current)
		}
		return nil
	}

	// Done — uninstall iterator, splice in accumulated results.
	UninstallDef(cont.Registry, cont.IterName)
	delete(e.marks, info.To)
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, cont.Results...)
	e.Pointer = markIdx
	if e.trace != nil {
		e.traceNote = "for done"
	}
	return nil
}

// stepMoveWhile drives a while loop's alternating regions. A CONDITION
// region's last value decides — truthy splices the body region, falsy
// splices the accumulated results and ends the loop; a BODY region's
// values accumulate into cont.Results and the condition region replays.
// Both regions run through the ordinary Run loop, so the step budget
// meters the loop (`while [true] []` trips evaluation_limit) and
// break/continue's forward scans find the move exactly as they find a
// for loop's (cont.Results is what break splices; the discarded region
// a continue leaves is a body whose collection round nets nothing).
func (e *Engine) stepMoveWhile(markIdx, moveIdx int, info MoveInfo) error {
	cont := info.Cont

	if cont.WhileInBody {
		for j := markIdx + 1; j < moveIdx; j++ {
			cont.Results = append(cont.Results, e.Tape.At(j))
		}
		cont.WhileInBody = false
		e.spliceWhileRegion(markIdx, moveIdx, info, cont.WhileCond, "while cond")
		return nil
	}

	var condResult Value
	for j := markIdx + 1; j < moveIdx; j++ {
		condResult = e.Tape.At(j)
	}
	if condResult.Parent == nil {
		delete(e.marks, info.To)
		e.Tape.Splice(markIdx, moveIdx-markIdx+1)
		e.Pointer = markIdx
		return e.runtimeError("runtime_error", "while: condition produced no value", "while", "")
	}
	if CoerceBoolean(condResult) {
		cont.WhileInBody = true
		e.spliceWhileRegion(markIdx, moveIdx, info, cont.Body, "while body")
		return nil
	}

	// Condition falsy — the loop is done: splice in the accumulated results.
	delete(e.marks, info.To)
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, cont.Results...)
	e.Pointer = markIdx
	if e.trace != nil {
		e.traceNote = "while done"
	}
	return nil
}

// spliceWhileRegion replaces the current mark..move span with a fresh
// mark + region + move triple (the same reused-scratch shape as
// stepMoveCont's re-mark) and points the engine at the new mark.
func (e *Engine) spliceWhileRegion(markIdx, moveIdx int, info MoveInfo, region []Value, note string) {
	id := NextMarkID()
	tokens := e.loopTokens[:0]
	tokens = append(tokens, NewMark(id, region...))
	tokens = append(tokens, region...)
	tokens = append(tokens, NewMoveCont(id, info.Reason, info.Cont))
	e.loopTokens = tokens

	delete(e.marks, info.To)
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, tokens...)
	if e.marks == nil {
		e.marks = make(map[string]bool)
	}
	e.marks[id] = true
	e.Pointer = markIdx
	if e.trace != nil {
		e.traceNote = note
	}
}

// stepMoveIf handles an if-statement continuation move. It collects the
// condition result (all resolved values between mark and move), evaluates
// the last value for truthiness, and splices in the chosen branch.
func (e *Engine) stepMoveIf(markIdx, moveIdx int, info MoveInfo) error {
	ifCont := info.IfCont

	// Collect condition results between mark and move.
	var condResult Value
	for j := markIdx + 1; j < moveIdx; j++ {
		condResult = e.Tape.At(j)
	}

	// Remove mark from hash table.
	delete(e.marks, info.To)

	// Check if condition produced a value.
	if condResult.Parent == nil {
		e.Tape.Splice(markIdx, moveIdx-markIdx+1)
		e.Pointer = markIdx
		return e.runtimeError("runtime_error", "if: condition produced no value", "if", "")
	}

	// Evaluate truthiness and choose branch.
	cond := CoerceBoolean(condResult)

	var branch []Value
	if cond {
		branch = ifCont.Then
	} else {
		branch = ifCont.Else
	}

	// Splice chosen branch (or nothing) in place of mark+condition+move.
	e.Tape.Splice(markIdx, moveIdx-markIdx+1, branch...)
	e.Pointer = markIdx
	if e.trace != nil {
		e.traceNote = fmt.Sprintf("if %v", cond)
	}
	return nil
}

// handleFlowCtrl dispatches the active flow-control signal to the
// matching tape-level resolver. Returns true if the signal was consumed
// (the resolver found an enclosing loop on this tape and rewrote it),
// false if no resolver was applicable and the flag should bubble up.
//
// On a true return, FlowCtrl has been cleared. On false, it is left
// set for an outer Run frame to handle.
func (e *Engine) handleFlowCtrl() bool {
	var handled bool
	switch e.Registry.FlowCtrl {
	case FlowBreak:
		handled = e.handleLoopBreak()
	case FlowContinue:
		handled = e.handleLoopContinue()
	}
	if handled {
		e.Registry.FlowCtrl = FlowNone
	}
	return handled
}

// isIsland reports whether this engine runs a VM ISLAND — a token window that
// CONTINUES an in-progress compiled expression on the interpreter (a fn-value
// apply the emitter could not lower, a FALLBACK span), rather than entering a
// nested body. The two island entry points (vmContext.islandRun and
// runIslandResolved) are the only setters of the underlying marker.
//
// Two consumers, both reading the same fact: exitWithFlowCtrl needs the frame
// teardown because the island's tape is separate from any enclosing loop's,
// and Run needs to SKIP the per-body context frame because an island is not a
// body. Named so each call site says which consequence it wants.
func (e *Engine) isIsland() bool { return e.FlowUnwind }

// exitWithFlowCtrl returns from Run when a flow-control signal could
// not be resolved on this tape. For a top-level engine this is the
// "break/continue outside loop" error path; for a sub-engine, the flag
// stays set on the shared registry and the residual tape is returned
// cleanly so an outer Run can resolve it.
func (e *Engine) exitWithFlowCtrl() ([]Value, error) {
	if e.IsTop {
		ctrl := e.Registry.FlowCtrl
		e.Registry.FlowCtrl = FlowNone
		return nil, e.runtimeError("flow_error", fmt.Sprintf("%s outside loop", ctrl), ctrl.String(), "")
	}
	if e.FlowUnwind {
		// A VM island: no outer TAPE exists to adopt the residual — tear down
		// the live spliced frames (their registry state: args stack, body-local
		// defs, captures) and return nothing; the VM translates the signal.
		e.unwindLiveFrames(0, e.Tape.Len())
		e.Tape.TakeAll()
		return nil, nil
	}
	return e.Tape.TakeAll(), nil
}

// handleLoopBreak resolves a FlowBreak signal by finding the nearest
// enclosing for-loop (move with continuation) on this tape and
// terminating it. Returns true if a loop was found and rewritten,
// false if no enclosing loop was on the tape.
func (e *Engine) handleLoopBreak() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.Pointer; i < e.Tape.Len(); i++ {
		if IsMove(e.Tape.At(i)) {
			info, _ := AsMove(e.Tape.At(i))
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as3, _ := AsMark(e.Tape.At(j))
					if IsMark(e.Tape.At(j)) && _as3.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Unwind any fn frame the break is escaping — its cleanup
				// tail is about to be discarded with the loop region and
				// would otherwise leak the per-call stacks (fn_frame.go).
				e.unwindLiveFrames(markIdx, i)

				// Uninstall iterator, splice in accumulated results.
				UninstallDef(info.Cont.Registry, info.Cont.IterName)
				delete(e.marks, info.To)
				e.Tape.Splice(markIdx, i-markIdx+1, info.Cont.Results...)
				e.Pointer = markIdx
				return true
			}
		}
	}
	return false
}

// handleLoopContinue resolves a FlowContinue signal by finding the
// nearest enclosing for-loop and advancing to the next iteration
// (discarding the current iteration's partial results). Returns true
// if a loop was found, false if no enclosing loop was on the tape.
func (e *Engine) handleLoopContinue() bool {
	// Scan forward from current pointer for a move with continuation.
	for i := e.Pointer; i < e.Tape.Len(); i++ {
		if IsMove(e.Tape.At(i)) {
			info, _ := AsMove(e.Tape.At(i))
			if info.Cont != nil {
				// Found the for-loop's move. Find its mark.
				markIdx := -1
				for j := 0; j < i; j++ {
					_as4, _ := AsMark(e.Tape.At(j))
					if IsMark(e.Tape.At(j)) && _as4.ID == info.To {
						markIdx = j
						break
					}
				}
				if markIdx < 0 {
					delete(e.marks, info.To)
					continue
				}

				// Unwind any fn frame the continue is escaping — its cleanup
				// tail is about to be discarded with the iteration region and
				// would otherwise leak the per-call stacks (fn_frame.go).
				e.unwindLiveFrames(markIdx, i)

				// Remove values between mark and move (discard partial results).
				if i-markIdx > 1 {
					e.Tape.Splice(markIdx+1, i-markIdx-1)
					// Recalculate move position.
					i = markIdx + 1
				}
				// Set pointer to the move so stepMove fires next.
				e.Pointer = i
				return true
			}
		}
	}
	return false
}

// cleanMarks removes any leftover mark and move entries from the stack.
func (e *Engine) cleanMarks() {
	i := 0
	for i < e.Tape.Len() {
		if IsMark(e.Tape.At(i)) || IsMove(e.Tape.At(i)) {
			e.Tape.Remove(i)
		} else {
			i++
		}
	}
	e.marks = nil
}

// stepOpenParen replaces the "(" word with an open-paren marker.
func (e *Engine) stepOpenParen() error {
	np := NewOpenParen()
	// Preserve the reach-lowered provenance: a `m.p` group's OpenParen
	// carries ReachGroup, which the no-call-inside-the-group rule
	// (execFnDefLiteral, NUR035) and the collapsed-result call tag
	// (stepCloseParen, NUR038) both read. The fresh marker must not
	// silently wipe it — the pre-evaluated window path (evalParenGroupAt)
	// steps the marker through here, unlike the main-loop path.
	np.ReachGroup = e.Tape.At(e.Pointer).ReachGroup
	e.Tape.Set(e.Pointer, np)
	e.Pointer++
	return nil
}

// consumeStartAt resolves the one-shot resolved-argument prefix for the
// Run that is starting: the leading startAt input values are call-site-
// resolved arguments, so stepping starts after them and they enter as
// stack data (see the startAt field doc). Zeroes the field so it cannot
// leak into a later reuse of a pooled engine.
func (e *Engine) consumeStartAt() int {
	start := 0
	if e.StartAt > 0 && e.StartAt <= e.Tape.Len() {
		start = e.StartAt
	}
	e.StartAt = 0
	return start
}

// stepPastOpenParen advances the pointer over an open paren at the
// pointer. A fn frame's open paren carries the span of unnamed arguments
// spliced at the frame head; they were resolved at the call site, so the
// pointer skips them and they enter as stack data, never re-stepped — a
// Function value or __SP marker argument must not fire on placement
// (arguments are inert; design/ARG-SEMANTICS-UNIFICATION.0.md).
func (e *Engine) stepPastOpenParen(val Value) {
	e.Pointer++
	if info, ok := val.Data.(FrameOpenInfo); ok && info.ArgSpan > 0 {
		e.Pointer += info.ArgSpan
	}
}

// stepCloseParen handles the ")" word. It resolves any pending forwards
// inside the paren scope via implicit end, then collapses the sub-expression.
// recordParenLeadingApply records a leading-dynamic fn-value apply bounded
// by a paren (REFUSAL-CLOSURE §9.2e): the value at `first` IS the fn being
// applied to the values after it — `((m get "f") x)`. Recorded as a guarded
// OpCallDynMethod (the §3 arrival chassis): the VM applies the RUNTIME value
// to the args exactly as the interpreter's paren auto-dispatch and DEFERS on
// a non-callable value or a result-count mismatch (interpreter re-run —
// never a wrong stack). The window collapses to the one modeled carrier, so
// a trailing consumer (`add 1 (...)`) seats the apply's RESULT — the
// paren-unaware reorder the old refusal guarded against cannot happen. Only
// a CONTAINER MEMBER read models here (memberFnRead provenance); any other
// leading dynamic keeps its existing paths, as the old refusal did. Returns
// the possibly-shrunk closeIdx (args spliced out).
func (e *Engine) recordParenLeadingApply(es EmitRecorder, first, openIdx, closeIdx int) int {
	fnVal := e.Tape.At(first)
	// This window IS the enclosing paren's re-step: it fires only for a paren
	// with >=2 net values and a DYNAMIC lead, which is exactly the shape whose
	// park declines and whose rewind lands on the lead (the paren re-step rule,
	// design/PAREN-RESTEP-RULE.0.md). So a placement recorded by the INNER
	// paren that produced the lead — `(tbl get k)` in `((tbl get k) 5)` — says
	// nothing here: the outer paren undoes it. A gate on that mark was added
	// 2026-08-26 under the since-falsified "place uniformly" ruling and
	// refused this row against its own interpreted answer of 10; it is gone.
	if !es.MemberFnRead(fnVal.ID) {
		es.MarkUncompilable("fn-value application bounded by a paren (dynamic value precedes args)")
		return closeIdx
	}
	var argVals []Value
	var argIdxs []int
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsRecordableLiteral(e.Tape.At(i)) && i != first {
			argVals = append(argVals, e.Tape.At(i))
			argIdxs = append(argIdxs, i)
		}
	}
	out := NewCarrier(TAny)
	out.ID = GenerateID(IDPrefixForType(TAny))
	out.pos = fnVal.pos
	if es.RecordDynMethod(fnVal, argVals, []Value{out}, "(paren apply)", fnVal.Pos()) {
		e.Tape.Set(first, out)
		for j := len(argIdxs) - 1; j >= 0; j-- {
			e.Tape.Remove(argIdxs[j])
			closeIdx--
		}
	} else { //covergate:allow RecordDynMethod resolves fnVal (a member-read EVENT, gated above) and each argVal (an isRecordableLiteral — a concrete const or an event-backed carrier resolveOperand handles), so it cannot decline here — the belt keeps the sound refusal if a future window shape breaks that invariant (§compiler)
		es.MarkUncompilable("fn-value application bounded by a paren (dynamic value precedes args)")
	}
	return closeIdx
}

// parenLeadFnApplyIdx classifies the Stage-G LEADING one-arg fn-carrier
// apply window — `(g x)` where g is a Function-typed param/capture slot of
// an open NAMED-PARAM fn unit (checker-compiler-completeness-review
// §8.2(1)/§9.6b): inside such a unit the paren seals the frame off, so the
// one-arg leading and trailing spellings CONVERGE for EVERY runtime fn (a
// mismatched arity no-matches identically in both), and the shape may
// record through the SAME RecordDynApply event `(x g)` would. Returns the
// lead's tape index only when the window is exactly [eligible lead, one
// non-fn argument]; -1 keeps every other shape on its own machinery: a
// DYNAMIC lead recordParenLeadingApply's guarded method path, a CONCRETE
// fn the auto-dispatch paths (it applied for real during the check step),
// an EVENT lead the curried paths (DynApplyLeadEligible declines it —
// RecordDynApply would hard-refuse), an unnamed-param frame the
// whole-frame replay (its leading collection can reach beneath the
// window), and a multi-arg lead (count > 2) is never collapsed — beyond
// one argument the spellings' collection orders diverge — so a bare
// multi-arg body tail rides the single-applicable whole-frame replay
// while a CHAINED one (`f (g x y)`) refuses on the two-applicable window.
func (e *Engine) parenLeadFnApplyIdx(es EmitRecorder, openIdx, closeIdx, count, lastIdx int) int {
	if count != 2 {
		return -1
	}
	// A NESTED BODY declines, for the same reason the fn-carrier read
	// substitution does (stepWord): the admission models the window as
	// the trailing spelling's event, and inside a branch / loop /
	// quotation body the compiled body does not carry the bindings that
	// model needs — `def mkg g:Function => [v:Integer => [(g v)]]  def h
	// (mkg …)  do [(h 1)]` compiled to an island that raised `undefined
	// word: g` (the factory's captured param) where the interpreter
	// answers 8. The proven operand contexts, where §9/§9b live, are
	// unaffected.
	if e.Registry != nil && e.Registry.Check.NestedBodyDepth > 0 {
		return -1
	}
	last := e.Tape.At(lastIdx)
	// The ARGUMENT gate, and it is load-bearing in BOTH clauses. The two
	// spellings converge only while the argument is not a function: a
	// FUNCTION-valued argument is never applied by the interpreter, whose
	// leading collection meets a function word — a barrier that never
	// feeds forward collection — and RAISES, where the trailing model
	// binds and applies. A statically-known fn argument is excluded
	// outright; a GRADUAL one (`x:Any`) is excluded because nothing here
	// can prove it will not be a function at run time.
	//
	// This cannot be repaired by resolving it at run time. The
	// interpreter's raise is a property of WORD dispatch, not of the
	// values: an island over the resolved window `[lead, fnArg]` leaves
	// both inert (probe: the pair comes back as the residual `fn (Integer)
	// fn (Integer)`, no apply and no error), while the word-read spelling
	// raises — and it raises with one of TWO texts (the stranded-forward
	// barrier when the lead parked a forward, the lead's own no-match when
	// no overload could), selected by engine-internal collection state the
	// window does not carry. So there is no faithful lowering to admit the
	// shape with, and the refusal stands (design/HIGHER-ORDER-FUNCTIONS.0.md
	// §5.8; pinned by TestS5BParenLeadFnApplyIdxGradualArgDeclines).
	if last.Dynamic || IsFnValueResidual(last) {
		return -1
	}
	for i := openIdx + 1; i < closeIdx; i++ {
		v := e.Tape.At(i)
		if IsRecordableLiteral(v) {
			// A lead whose argument a LATER dispatch already collected in
			// this scope (`(g x add 1)`: the model's `add` took `x`) is a
			// collection hazard — the window's argument is that dispatch's
			// result, not the lead's (NUR121).
			if !v.Dynamic && !v.Quoted && IsFnTypedCarrier(v) && es.DynApplyLeadEligible(v) && !es.CollectionHazard(v.ID) {
				return i
			}
			break
		}
	}
	return -1
}

// recordParenLeadFnApply records the classified [lead, arg] window
// (parenLeadFnApplyIdx) as the trailing spelling's RecordDynApply event —
// the compiled artifact is literally `(x g)`'s, so parity holds by
// construction — substituting the event's out carrier for the lead and
// splicing the argument out. This is what compiles compose natively: the
// inner `(g x)` becomes an event, and the outer `f <event>` rides the
// single-applicable RetReplay body tail. On a decline the window is left
// intact for the downstream machinery (sound refusal-or-replay). Returns
// the possibly-shrunk closeIdx.
func (e *Engine) recordParenLeadFnApply(es EmitRecorder, leadFn, lastIdx, closeIdx int) int {
	lead := e.Tape.At(leadFn)
	out := NewCarrier(TAny)
	out.ID = GenerateID(IDPrefixForType(TAny))
	out.pos = lead.pos
	// A one-value window: the callee either consumes it or the record
	// declines, so there is no partial-consumption case to thread here.
	if _, ok := es.RecordDynApply([]Value{e.Tape.At(lastIdx)}, lead, out, lead.Pos()); ok {
		e.Tape.Set(leadFn, out)
		e.Tape.Remove(lastIdx)
		closeIdx--
	}
	return closeIdx
}

// parenTrailingFnApply classifies a paren whose LAST value is the fn a
// paren-bounded apply applies to the values before it: a concrete or
// fn-typed value that is not Dynamic, or a Dynamic value the recorder holds
// a pending `apply`-word application for (`(x r apply)` over a gradual r —
// the apply word owns the application, so the value is the lead, not the
// leading-dynamic reorder hazard recordParenLeadingApply refuses).
func parenTrailingFnApply(es EmitRecorder, last Value, count, lastIdx int) bool {
	if count < 2 || lastIdx < 0 {
		return false
	}
	if last.Dynamic {
		return last.ID != "" && es.ApplyPending(last.ID)
	}
	return IsFnValueResidual(last)
}

// reStepped tells stepCloseParen whether the MAIN loop will re-encounter the
// survivors at the pointer (true), or whether this collapse is happening off
// the main loop on behalf of a pending forward collection (false), where the
// survivors become ARGUMENTS instead. Only the recorder accounting reads it —
// see creditParenSurvivorSkips.
func (e *Engine) stepCloseParen(reStepped bool) error {
	closeIdx := e.Pointer

	openIdx := -1
	for i := closeIdx - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			openIdx = i
			break
		}
	}

	if openIdx < 0 {
		return e.syntaxError("unmatched closing parenthesis", ")")
	}
	// Snapshot BEFORE the pair removals below destroy the open paren. The
	// reach-group tag feeds both tagReachCollapsedFn and fnReturnPark's ONE
	// exclusion: a reach-lowered group's re-step IS the dispatch, while every
	// other paren — user-written and fn frame alike — parks a collapsed
	// Function as placed data (NUR073's BROAD verdict, clause 3).
	wasReachGroup := e.Tape.At(openIdx).ReachGroup

	// Resolve any forwards inside the paren scope via implicit end.
	// We loop because resolving a forward may cause re-evaluation.
	for attempt := 0; attempt < 222; attempt++ {
		hasFwd := false
		for i := openIdx + 1; i < closeIdx; i++ {
			if IsForward(e.Tape.At(i)) {
				hasFwd = true
				fwd, _ := AsForward(e.Tape.At(i))
				funcIdx := fwd.FuncIndex
				collectedCount := fwd.CollectedArgs
				stackArgCount := fwd.StackArgs

				// Remove the forward.
				e.Tape.Remove(i)
				if i < funcIdx {
					funcIdx--
				}

				// Try stack match or create curry list.
				e.curryOrStack(funcIdx, collectedCount, stackArgCount)

				// Recalculate closeIdx after potential stack changes.
				closeIdx = e.findCloseParenAfter(openIdx)
				if closeIdx < 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
					return e.syntaxError("unmatched closing parenthesis", ")")
				}

				// Re-evaluate from current pointer up to closeIdx.
				for e.Pointer < closeIdx {
					val := e.Tape.At(e.Pointer)
					// Line-coverage seam (coverage.go): this nested forward-
					// resolution loop steps tokens off the main loop; mirror
					// its per-step emit.
					e.Registry.noteCoverage(val.Pos())
					switch {
					case IsWord(val):
						if err := e.stepWord(val); err != nil {
							return err
						}
						// Recalculate closeIdx: stack may have changed.
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsCloseParen(val): //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
						if err := e.stepCloseParen(false); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx) //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
						if closeIdx < 0 {                         //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsEnd(val):
						if err := e.stepEnd(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					case IsForward(val):
						e.Pointer++
					case IsOpenParen(val):
						e.Pointer++
					case IsReturnCheck(val):
						e.Pointer++
					case IsDefCleanup(val):
						if err := e.stepDefCleanup(val, e.Pointer); err != nil { //covergate:allow interpreter step/dispatch defensive error arm: a marker reaching the implicit-end re-run was already stepped by the main loop (its residual containers are no longer pending, so the EvalResidual scan no-ops); a first-time step here needs a frame spliced DURING forward resolution carrying a pending failing residual — a pathological shape the census does not produce (§engine)
							return err
						}
						e.Pointer++
					default:
						if err := e.stepLiteral(); err != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return err
						}
						closeIdx = e.findCloseParenAfter(openIdx)
						if closeIdx < 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
							return e.syntaxError("unmatched closing parenthesis", ")")
						}
					}
					// Propagate any flow-control signal raised by
					// the step; the outer Run frame will resolve it.
					if e.Registry.FlowCtrl != FlowNone {
						return nil
					}
				}
				break // restart the outer loop to check for more forwards
			}
		}
		if !hasFwd {
			break
		}
	}

	// Check for any remaining orphaned forwards.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsForward(e.Tape.At(i)) { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			fwd, _ := AsForward(e.Tape.At(i))
			if verr := e.voidArgErrorFor(fwd.FuncName, fwd.Pos); verr != nil { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				return verr
			}
			return e.insufficientArgsError(fwd.FuncName, fwd.ExpectedArgs, fwd.Pos) //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
		}
	}

	// A group that resolved to ZERO values is recorded together with
	// its candidate consumers — the pending words below it on the
	// stack — for the blame-shift shape of ERRORS.8.md §3 (VOXGIG B3):
	// a void call in an argument position starves the consuming word,
	// which then fails LATER with a generic error at an innocent site.
	// Collection may legitimately resume past a void group
	// (`add () 5 6` → 11), so nothing errors here; the record speaks
	// only if one of those candidates raises a signature failure in
	// the same statement (sigError / insufficientArgsError consult it
	// via voidArgErrorFor).
	if closeIdx == openIdx+1 {
		for i := openIdx - 1; i >= 0; i-- {
			v := e.Tape.At(i)
			if IsOpenParen(v) {
				break
			}
			switch {
			case IsWord(v):
				w, _ := AsWord(v)
				e.voidGroups = append(e.voidGroups, w.Name)
			case IsForward(v):
				fwd, _ := AsForward(v)
				e.voidGroups = append(e.voidGroups, fwd.FuncName)
			}
		}
	}

	// Remove any surviving def-cleanup markers.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsDefCleanup(e.Tape.At(i)) {
			if err := e.stepDefCleanup(e.Tape.At(i), i); err != nil {
				return err
			}
			e.Tape.Remove(i)
			closeIdx--
			i--
		}
	}

	// Check for return type validation.
	for i := openIdx + 1; i < closeIdx; i++ {
		if IsReturnCheck(e.Tape.At(i)) {
			rc, _ := AsReturnCheck(e.Tape.At(i))
			e.Tape.Remove(i)
			closeIdx--

			// Collect resolved values in scope.
			var results []Value
			for j := openIdx + 1; j < closeIdx; j++ {
				results = append(results, e.Tape.At(j))
			}

			// Unconsumed unnamed args sit at the bottom of the scope,
			// body results sit at the top. Allow extra values up to the
			// number of unnamed params that were pushed before the body.
			nret := len(rc.Returns)
			if len(results) < nret {
				return e.returnCountError(rc, nret, len(results), results)
			}
			extra := len(results) - nret
			if extra > rc.UnnamedCount {
				// The unnamed-arg allowance is spent from the BOTTOM, so
				// the values the count is about are the top ones.
				return e.returnCountError(rc, nret, len(results)-rc.UnnamedCount,
					results[rc.UnnamedCount:])
			}

			// Validate the top nret values match declared return types.
			if err := e.validateReturnTypes(rc, results, extra); err != nil {
				return err
			}

			// Strip any dispatch ascription (`v as T`) from the return
			// values: an ascription is scoped to a single dispatch WITHIN
			// the body and must not ride out of the frame into the caller's
			// dispatch (design/OPEN-WORDS.1.md §9 — "can never ride into a
			// returned receiver"). The top nret values are the returns.
			e.stripTapeAscriptions(openIdx+1+extra, closeIdx)

			// Discard unconsumed unnamed args from the bottom of the scope.
			for j := 0; j < extra; j++ {
				e.Tape.Remove(openIdx + 1)
				closeIdx--
			}
			break
		}
	}

	// Check-mode fn-value-call boundary guard. A paren whose net contents are
	// a leading DYNAMIC value followed by >=1 more value is a deferred fn-value
	// application bounded by THIS paren — `(m.g 3)`, where m.g is a dynamic
	// method-field get the checker cannot dispatch in place (it stays a value
	// and flows to the residual's OpCallDynamic). That residual reconciliation
	// is paren-UNAWARE: if the paren result is then consumed by a trailing op,
	// the op reorders ahead of the apply and the value is applied to the wrong
	// argument — `((m.g 3) add 1)` compiled m.g(3 add 1)=8 instead of
	// (m.g 3) add 1=7. The interpreter dispatches the concrete fn AT the paren,
	// so refuse here and let the faithful interpreter fallback run it.
	if es := e.Registry.analysisRecorder(); es.Active() {
		first, count, lastIdx := -1, 0, -1
		for i := openIdx + 1; i < closeIdx; i++ {
			if IsRecordableLiteral(e.Tape.At(i)) {
				if count == 0 && e.Tape.At(i).Dynamic {
					first = i
				}
				count++
				lastIdx = i
			}
		}
		leadFn := e.parenLeadFnApplyIdx(es, openIdx, closeIdx, count, lastIdx)
		// Classify the paren's fn-value-call boundary. The TRAILING case is checked
		// FIRST: when the LAST value is a concrete Function applied to the preceding
		// args, it is the sound paren-bounded apply (`(prev key comp)`, or
		// `((arr get x) key comp)` where an ARG is dynamic but comp is the fn) — even
		// if an arg is dynamic, the dynamic is an ARGUMENT, not the applied fn, so the
		// leading-dynamic reorder hazard does not apply. Only when the last value is
		// NOT the fn does a LEADING dynamic value mean the dynamic IS the fn being
		// applied before its args (`(m.g 3)`, `((m.g 3) add 1)`) — the unsound
		// paren-unaware reorder the residual reconciliation cannot reproduce.
		last := Value{}
		if lastIdx >= 0 {
			last = e.Tape.At(lastIdx)
		}
		switch {
		case parenTrailingFnApply(es, last, count, lastIdx):
			// TRAILING fn-value apply (`(a b comp)`): record it as an EVENT producing
			// ONE carrier and COLLAPSE the [args…, fn] tape residual to that carrier —
			// exactly as the interpreter's paren auto-dispatch nets one result. The
			// event seats like any computed result (a def-local `def c (a b comp)`, an
			// `if` operand, a list member, the body residual), so a comparator apply
			// bound to a local compiles, not ONLY the body's trailing residual. On
			// refusal (an unresolvable operand or a nested unapplied fn arg) the residual
			// is left intact and the body-residual lowering (RegisterTrailingApply) still
			// handles the trailing-residual case soundly.
			var argVals []Value
			var argIdxs []int
			for i := openIdx + 1; i < closeIdx; i++ {
				if IsRecordableLiteral(e.Tape.At(i)) && i != lastIdx {
					argVals = append(argVals, e.Tape.At(i))
					argIdxs = append(argIdxs, i)
				}
			}
			out := NewCarrier(TAny)
			// A lead the `apply` WORD owns — a gradual value, or a fn-typed
			// carrier under the word (`(x f/v apply)`) — has no static
			// return: its result is GRADUAL (Dynamic), the honest type, so a
			// later dispatch over it matches gradually and records a runtime
			// re-match instead of the checker's best-fit recovery (a strict
			// Any there refused `x (x f/v apply) apply` as "unmatched
			// dispatch recovered at apply", the twenty-seventh increment).
			// A plain paren apply of a fn-typed carrier (`(1 2 c)`) keeps its
			// strict Any: the comparator convention's consumers were built on
			// it, and a gradual result there refused an each body's residual
			// ("result above a literal") that compiles today.
			if last.Dynamic || es.ApplyPending(last.ID) {
				out.Dynamic = true
			}
			out.ID = GenerateID(IDPrefixForType(TAny))
			out.pos = last.pos
			// consumed counts from the TOP of the window (the values
			// nearest the fn). It is normally every arg, but a callee whose
			// arity is provably smaller UNDER-APPLIES exactly as the
			// interpreter does — `(1 2 (mk 4))` with a 1-arg adder nets
			// [1, 6] — so only the consumed SUFFIX collapses and the deeper
			// values stay on the tape as residual.
			if consumed, ok := es.RecordDynApply(argVals, last, out, last.Pos()); ok {
				e.Tape.Set(lastIdx, out)
				for j := len(argIdxs) - 1; j >= len(argIdxs)-consumed; j-- {
					e.Tape.Remove(argIdxs[j])
					closeIdx--
				}
			} else {
				es.RegisterTrailingApply(last.ID, count-1)
			}
		case leadFn >= 0:
			// LEADING one-arg fn-carrier apply (the Stage-G increment) —
			// classified by parenLeadFnApplyIdx, recorded by
			// recordParenLeadFnApply (both extracted for the stepCloseParen
			// complexity cap).
			closeIdx = e.recordParenLeadFnApply(es, leadFn, lastIdx, closeIdx)
		case first >= 0 && count >= 2:
			// LEADING dynamic apply (REFUSAL-CLOSURE §9.2e) — extracted to
			// recordParenLeadingApply for the stepCloseParen complexity cap.
			closeIdx = e.recordParenLeadingApply(es, first, openIdx, closeIdx)
		}
	} else {
		// PLAIN check surface (no active recorder — a bare `check` run, or a
		// construction-time AnalyseFnBody under a suspended compile pass):
		// collapse a fn-carrier apply window to the ONE dynamic value the
		// interpreter nets — checkModeParenFnCollapse (the §9.4 def-split
		// FP fix; it guards on check mode itself and was extracted for the
		// stepCloseParen complexity cap).
		closeIdx = CheckBraid.CheckModeParenFnCollapse(e, openIdx, closeIdx)
	}

	// Remove the close paren (higher index first) and open paren.
	// The values between them are already in place.
	e.Tape.Remove(closeIdx)
	e.Tape.Remove(openIdx)

	// A REACH-LOWERED group that collapsed to a single named function
	// value tags the result — extracted to tagReachCollapsedFn for the
	// stepCloseParen complexity cap (NUR038).
	e.tagReachCollapsedFn(openIdx, closeIdx, wasReachGroup)

	// The park drives the REWIND below, and nothing else: the recorder's skip
	// count asks fnValueDispatchesAtPointer about each survivor directly
	// rather than subtracting this (§5.2), so the two no longer have to be
	// kept in agreement. Read here — after the pair removals, with closeIdx
	// still the pre-removal index CheckModeParenFnCollapse may have rewritten
	// above.
	park := e.fnReturnPark(openIdx, closeIdx, !wasReachGroup)

	// The park's negative twin, and the second half of the paren re-step rule
	// (design/PAREN-RESTEP-RULE.0.md). A park of 0 over MORE than one survivor
	// means the rewind below lands ON the leading value, and a Function there
	// is re-stepped INTO A CALL — `((mk 1) 2)` is 3 for exactly this reason.
	// Record it for the compiler, which sees the same `[carrier, 2]` residual
	// for this shape and for `(mk 1) 2`, where no rewind ever arrives.
	e.recordParenReStep(openIdx, closeIdx, park, wasReachGroup)

	// Recorder hook: the values that survived inside the paren will
	// be re-encountered by the main loop after we set pointer back
	// to openIdx (below). They were already emitted to the recorder
	// Rewind onto the collapsed region so the main loop re-encounters what
	// survived — stepping past a fn frame's Function return, which is a
	// delivery rather than a fresh use (fnReturnPark).
	e.Pointer = openIdx + park

	// Recorder hook: the survivors the rewind is about to re-present were
	// already emitted during the in-paren execution (via stepLiteral, or via
	// execMatch for handler results), so tell a SkipRecorder to ignore that
	// second round. Runs AFTER the rewind on purpose: it asks
	// pendingForwardIdx what the re-step will see, and that answer is read
	// from the pointer. Values before the pointer — a parked return — are
	// never re-encountered and so are never credited. Extracted to
	// creditParenSurvivorSkips for the stepCloseParen complexity cap
	// (NUR038), same reason as fnReturnPark and tagReachCollapsedFn above.
	e.creditParenSurvivorSkips(openIdx, closeIdx, reStepped)
	return nil
}

// recordParenReStep notes a Function-typed carrier this collapse is about to
// re-step into a call, so the compiler's residual lowering can tell an APPLIED
// lead from a PLACED one. Called with the POST-removal indices, where the
// survivors occupy [openIdx, closeIdx-2]: more than one of them means the park
// declined and the rewind lands on the lead.
//
// The park's own exclusions apply here too — a reach-lowered group's re-step is
// its dispatch, not a user paren's, and it never needed recording. Only the
// LEAD matters: the rewind reaches exactly one value.
func (e *Engine) recordParenReStep(openIdx, closeIdx, park int, wasReachGroup bool) {
	if park != 0 || wasReachGroup || closeIdx <= openIdx+2 || openIdx >= e.Tape.Len() {
		return
	}
	if e.Registry == nil || e.Registry.Check == nil {
		return
	}
	v := e.Tape.At(openIdx)
	if v.Quoted || v.ID == "" {
		return
	}
	// The same "might be callable" test the residual lowering's auto-dispatch
	// guard uses, so both ends agree on what the rewind would have called: a
	// genuine fn-typed carrier, or a dynamic value whose static bound does not
	// exclude Function.
	if !IsFnTypedCarrier(v) && !(v.Dynamic && SigTypeMatches(v, TFunction)) {
		return
	}
	if e.Registry.Check.ParenReSteppedFnIDs == nil {
		e.Registry.Check.ParenReSteppedFnIDs = map[string]bool{}
	}
	e.Registry.Check.ParenReSteppedFnIDs[v.ID] = true
}

// findCloseParenAfter finds the index of the matching close-paren marker
// after the given openIdx.
func (e *Engine) findCloseParenAfter(openIdx int) int {
	depth := 0
	for i := openIdx + 1; i < e.Tape.Len(); i++ {
		if IsOpenParen(e.Tape.At(i)) {
			depth++
		} else if IsCloseParen(e.Tape.At(i)) {
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// effectiveResolved returns the resolved portion of the stack visible for
// stack matching. Function words and their collected forward args that are
// tracked by active forwards are excluded — they belong to the outer
// forward's context and should not be consumed by inner stack matching.
func (e *Engine) EffectiveResolved() []Value {
	start := 0
	// Reused exclusion set, cleared each call (see resolvedScratch/
	// excludeScratch on Engine). Most dispatches have no active Forward in
	// the window, so hasExclude stays false and the set is never touched.
	// design/INTERPRETER-SPEED-PLAN.10.md #3.
	excludeIndices := e.excludeScratch
	hasExclude := false
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			start = i + 1
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			if excludeIndices == nil {
				excludeIndices = make(map[int]bool)
				e.excludeScratch = excludeIndices
			} else if !hasExclude {
				clear(excludeIndices)
			}
			hasExclude = true
			// Exclude the function word itself.
			excludeIndices[fwd.FuncIndex] = true
			// Exclude collected forward args (positioned before function word).
			for j := 0; j < fwd.CollectedArgs; j++ {
				idx := fwd.FuncIndex - 1 - j
				if idx >= 0 {
					excludeIndices[idx] = true
				}
			}
			// Exclude claimed stack args (positioned before collected forward args).
			stackStart := fwd.FuncIndex - fwd.CollectedArgs - fwd.StackArgs
			for j := stackStart; j < fwd.FuncIndex-fwd.CollectedArgs; j++ {
				if j >= 0 {
					excludeIndices[j] = true
				}
			}
		}
	}
	// Reused result buffer: after the first few dispatches it has grown to
	// the window size and append never reallocates. Its sole consumer,
	// matchSignature, reads it and returns before any further dispatch, so
	// reuse is aliasing-safe (see the field comment).
	resolved := e.resolvedScratch[:0]
	for i := start; i < e.Pointer; i++ {
		v := e.Tape.At(i)
		if IsForward(v) || IsOpenParen(v) || IsMark(v) || IsMove(v) || (hasExclude && excludeIndices[i]) {
			continue
		}
		resolved = append(resolved, v)
	}
	e.resolvedScratch = resolved
	return resolved
}

// isInsidePendingForward returns true if the current pointer is within the
// collection scope of a pending forward (i.e., another function is waiting
// to collect this function's result as a forward arg).
//
// This is pendingForwardIdx's predicate form and MUST stay a delegation, not
// a second copy of the scan: the barrier stop conditions belong in one place.
// Forward gathering already spans two phases that "live in different
// functions, walk different representations, and — before the fix — enforced
// different stop conditions" (design/FORWARD-COLLECTION-PHASES.10.md); a
// third, hand-duplicated copy of the backward barrier walk is the same trap.
func (e *Engine) isInsidePendingForward() bool {
	return e.pendingForwardIdx() >= 0
}

// curryOrStack handles a terminated forward. If the word at funcIdx can
// match a stack signature with the available resolved values, it forces
// stack mode for normal execution. Otherwise, it packages the word and
// its collectedCount forward args into a list value (partial application).
// When the list is later expanded (e.g., via def body substitution), the
// word and args are spliced back onto the stack for completion.
func (e *Engine) curryOrStack(funcIdx int, collectedCount int, stackArgCount ...int) {
	sac := 0
	if len(stackArgCount) > 0 {
		sac = stackArgCount[0]
	}

	if funcIdx >= e.Tape.Len() || !IsWord(e.Tape.At(funcIdx)) {
		// A VALUE callee re-steps and re-plans in full — its own forward
		// window (`(concat parts {sep})`) must stay claimable so a wider
		// arity can still match. A reach-read CALL head in that window
		// cannot re-open the closed statement: the tagged value is a plan
		// barrier (matchSignature / resolveForwardArgs, NUR038).
		e.Pointer = funcIdx
		return
	}

	w, _ := AsWord(e.Tape.At(funcIdx))
	fn := e.Registry.Lookup(w.Name)

	// Check if stack match exists with current resolved values.
	if fn != nil {
		// Build resolved slice up to funcIdx, excluding function words
		// and their collected forward args that are tracked by active
		// forwards. This prevents stack matching from consuming values
		// that belong to an outer forward's context.
		start := 0
		excludeIndices := make(map[int]bool)
		for i := funcIdx - 1; i >= 0; i-- {
			if IsOpenParen(e.Tape.At(i)) {
				start = i + 1
				break
			}
			if IsForward(e.Tape.At(i)) {
				fwd, _ := AsForward(e.Tape.At(i))
				// Exclude the function word itself.
				excludeIndices[fwd.FuncIndex] = true
				// Exclude collected forward args (before function word).
				for j := 0; j < fwd.CollectedArgs; j++ {
					idx := fwd.FuncIndex - 1 - j
					if idx >= 0 {
						excludeIndices[idx] = true
					}
				}
				// Exclude claimed stack args.
				stackStart := fwd.FuncIndex - fwd.CollectedArgs - fwd.StackArgs
				for j := stackStart; j < fwd.FuncIndex-fwd.CollectedArgs; j++ {
					if j >= 0 {
						excludeIndices[j] = true
					}
				}
			}
		}
		var resolved []Value
		for i := start; i < funcIdx; i++ {
			v := e.Tape.At(i)
			if IsForward(v) || IsOpenParen(v) || excludeIndices[i] {
				continue
			}
			resolved = append(resolved, v)
		}

		testW := WordInfo{Name: w.Name, ArgCount: -1, ForceStack: true}

		// For words whose sigs collect forward args, rearrange values
		// so forward args are first and stack args are reversed before
		// matching.
		if fn.HasForwardSigs() && sac > 0 {
			e.Pointer = funcIdx
			e.rearrangeForForward(sac, collectedCount)
		}

		match := MatchSignature(fn.Signatures, resolved, testW)
		if match != nil {
			e.forceStackWord(funcIdx, w)
			e.Pointer = funcIdx
			return
		}
	}

	// Check if there's a pending outer forward that would collect the result.
	// Only create a curry list when an outer context is waiting for a value;
	// otherwise, fall through to normal stack retry (which may error).
	hasOuterForward := false
	checkStart := funcIdx - collectedCount
	if checkStart < 0 {
		checkStart = 0
	}
	for i := checkStart - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			hasOuterForward = true
			break
		}
	}

	if hasOuterForward {
		// Create a curry list: [word, arg1, arg2, ...].
		// When this list is expanded by def body substitution, it re-emits
		// the word and collected args for completion with additional args.
		startIdx := funcIdx - collectedCount
		if startIdx < 0 { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
			startIdx = 0
		}

		elems := make([]Value, 0, 1+collectedCount)
		elems = append(elems, NewWord(w.Name))
		for i := startIdx; i < funcIdx; i++ {
			elems = append(elems, e.Tape.At(i))
		}

		e.Tape.Splice(startIdx, collectedCount+1, NewList(elems))
		e.Pointer = startIdx
		return
	}

	// No outer forward - force stack (may result in error on next step).
	e.forceStackWord(funcIdx, w)
	e.Pointer = funcIdx
}

// hasPendingForwardQuoteArg reports whether there is a pending forward
// whose next slot is marked /q (QuoteArgs) — meaning the upcoming Word
// should be captured as an Atom rather than executed. This is the
// general word-capture mechanism used by def, undef, type, untype,
// quote, inspect, etc.; see signature.go §1.5 on /q.
func (e *Engine) hasPendingForwardQuoteArg() bool {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			// Forward args fill from sigArgs[0]; the next forward slot
			// is at index CollectedArgs.
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return fwd.Sig.QuoteArgs != nil && fwd.Sig.QuoteArgs[nextIdx]
			}
			break
		}
	}
	return false
}

// hasPendingForwardFormArg reports whether the nearest enclosing pending
// Forward's next slot is FormArgs — meaning the upcoming Word should be
// collected as a raw Word (not executed, not coerced to an Atom). Mirrors
// hasPendingForwardQuoteArg. See design/MACROS-PHASE1.10.md §3.
func (e *Engine) hasPendingForwardFormArg() bool {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return fwd.Sig.FormArgs != nil && fwd.Sig.FormArgs[nextIdx]
			}
			break
		}
	}
	return false
}

// hasPendingForwardExpectingFunction checks if there is a pending forward
// whose next expected argument is TFunction.
func (e *Engine) hasPendingForwardExpectingFunction() bool {
	for i := e.Pointer - 1; i >= 0; i-- {
		if IsOpenParen(e.Tape.At(i)) {
			break
		}
		if IsForward(e.Tape.At(i)) {
			fwd, _ := AsForward(e.Tape.At(i))
			// Forward args fill from sigArgs[0].
			nextIdx := fwd.CollectedArgs
			if nextIdx < fwd.Sig.TotalArgs() {
				return SigArgType(fwd.Sig, nextIdx).Equal(TFunction)
			}
			break
		}
	}
	return false
}

// MatchSignature is the unified signature matching function.
//
// Algorithm:
//
//	0.1 Using the ordered signatures, attempt to match in order,
//	    stopping at the first match.
//	1.1 If stack-only (or /s) and not /f: skip forward, go to step 2.
//	    If stack-only but /f: override → do forward scan.
//	1.2 Match each parameter in order against future tokens.
//	1.3 Stop if all params matched, or if /N params reached.
//	1.4 Move to step 2 if you hit a boundary condition:
//	    a function word, a pipe barrier, or "end".
//	1.5 If you hit an open paren, treat as boundary (pre-evaluated).
//	2.1 Match the remaining parameters against the stack, working
//	    backwards (top of stack first).
//	2.2 Stop once all or /N params reached.
//
// This is implemented as one outer loop over signatures and one inner
// loop over parameters. No separate functions are called for matching.
//
// Returns: matched signature, arg positions (absolute stack indices
// in signature order), and the speculation marker. Positions > pointer
// are forward args that need deferred collection. Positions < pointer
// are stack args. Returns nil sig if no signature matches.
//
// The third return is the sig-order index of the FIRST forward slot
// the plan filled with a WORD bound to a dispatching definition (an
// FnDefInfo binding accepted as an operand — typically through an
// Any-typed slot), or -1 when no slot was filled that way. Such a
// token is planned as an operand but DISPATCHES at runtime — the
// plan-time stop condition insertForward records on ForwardInfo so
// the arrival side can observe it. See
// design/FORWARD-COLLECTION-PHASES.10.md.
//
// COMPLEXITY: this function once carried //nolint:gocyclo,gocognit at
// 87/211 — a reading of the pre-carve eng/go/match.go version. Stage 2's
// re-seat 2 (54d8830) extracted the per-candidate scan to
// CollectCandidateScan and the live measure is 68/131, UNDER the repo
// caps (70/200), so the exemption is deleted and the caps now bind here
// for real. Headroom is thin by design (2 gocyclo points): a change that
// would breach a cap extracts the block it grew — textually identically,
// in its own commit, per the §10 triggers in design/FULL-COMPILATION.0.md
// — rather than re-earning an exemption.
func (e *Engine) MatchSignature(fn *FnDefInfo, w WordInfo, resolved []Value) (*Signature, []int, int) {

	// Unified dispatch (post §1.4 fix): no more stackOnly/forward-prec
	// dichotomy at the word level. Each sig declares its own boundary
	// via BarrierPos — the count of leading args that may be collected
	// from forward tokens. Args at sig[BarrierPos..N-1] always come
	// from the stack, top-down. The /s and /f modifiers override
	// BarrierPos at the call site:
	//   - /s (ForceStack)   → boundary at 0, all stack
	//   - /f (ForceForward) → boundary at N, all forward
	insideForward := e.isInsidePendingForward()

	// Forward/stack split ambiguity (check mode only, compile-time advisory).
	// If a more-specific overload is rejected because the stack-top operand
	// is a genuinely MIXED gradual carrier (carrierMixedConform), and the
	// overload finally SELECTED forward-collects instead — leaving that
	// carrier on the stack — the static split diverges from the runtime one
	// (a concrete value would have matched the more-specific overload and
	// been grabbed). noteSplit flags it so the compiler refuses; dispatch
	// itself is unchanged. See CheckState.AmbiguousGradualSplit.
	checkActive := e.Registry != nil && e.Registry.analysisActive()
	// Only ever read under checkActive (the scan's gradual-Any arm), so the
	// runtime hot path never asks; hoisted here so the per-candidate loop
	// asks once per dispatch rather than once per candidate.
	compiling := checkActive && e.Registry.analysisCompiling()
	mixedCarrierRejectIdx := -1
	noteSplit := func(positions []int, fwd int) {
		if !checkActive || mixedCarrierRejectIdx < 0 || fwd == 0 {
			return
		}
		for _, p := range positions {
			if p == mixedCarrierRejectIdx { //covergate:allow interpreter step/dispatch defensive index+error arm; unreachable via eng harness (design/COVERAGE-ALLOWLIST.10.md §engine)
				return // the carrier was consumed after all — not skipped
			}
		}
		e.Registry.noteAmbiguousGradualSplit()
	}

	// When the next forward token is a Word, prefer signatures with
	// /q at position 0 (inspect-style name capture). The user wrote a
	// Word, not a String — the /q sig captures the user's intent that
	// the name is data, not a call site. The non-/q TString sister
	// sig is for callers who pass a string literal. This also covers
	// untype Foo (Foo in r.Types), `m.Color` after import (Color is a
	// key in the imported map), and inspect-style name capture.
	preferWordSig := false
	if e.Pointer+1 < e.Tape.Len() {
		next := e.Tape.At(e.Pointer + 1)
		if IsWord(next) {
			preferWordSig = true
		}
	}

	// Track the best non-preferred match so that if no preferred sig
	// matches, we can fall back to it without a second pass.
	type matchResult struct {
		sig       *Signature
		positions []int
		specAt    int
	}
	var bestDeferred *matchResult

	// ONE int buffer per matchSignature INVOCATION backs both the
	// per-candidate positions (first maxSigArgs cells, re-sliced and
	// re-zeroed per candidate — the previous per-candidate make was ~17%
	// of all interpreter allocations) and the resolved-index map (tail
	// cells). Per-invocation (NOT engine-level) is load-bearing: a
	// predicate-typed param runs boru during sigTypeMatches (RunPredicate),
	// so matchSignature can nest — each nested call owns its own buffer.
	// A success return hands the positions slice to the caller (ownership
	// transfers; this call never touches the buffer again and the
	// resolvedIdx region is dead after return); bestDeferred keeps its
	// explicit copy since later candidates overwrite the buffer.
	maxSigArgs := 0
	for si := range fn.Signatures {
		if n := fn.Signatures[si].TotalArgs(); n > maxSigArgs {
			maxSigArgs = n
		}
	}
	intBuf := make([]int, maxSigArgs, maxSigArgs+len(resolved))
	posBuf := intBuf[:maxSigArgs]

	// Absolute stack indices of the resolved values — the positions of
	// stack-matched args. Filled into the tail region of intBuf.
	resolvedIdx := e.resolvedIndicesBeforeInto(intBuf[maxSigArgs:maxSigArgs], len(resolved))

	// ── 0.1: one outer loop over sorted signatures ───────────────
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]

		if sig.Fallback {
			continue
		}
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}

		nArgs := sig.TotalArgs()

		// 0-arg sigs are deferred to the fallback section at the bottom.
		if nArgs == 0 {
			continue
		}

		// Check if this is a preferred (/q at arg[0]) signature.
		isPreferred := preferWordSig && nArgs > 0 &&
			sig.QuoteArgs != nil && sig.QuoteArgs[0]

		// Effective forward limit for this match attempt — the same
		// predicate the phase-1 plan walk uses (see effectiveForwardLimit).
		forwardLimit := effectiveForwardLimit(sig, w)

		// ── Step 1: forward matching ─────────────────────────────

		positions := posBuf[:nArgs]
		for i := range positions {
			positions[i] = 0
		}
		// The per-candidate scan runs on the collection kernel
		// (collect_kernel.go): once per candidate signature, over THIS
		// signature's own forward limit. If forwardLimit is 0 the walk
		// simply does not execute and every arg comes from the stack below.
		// fwd is how many params the forward tokens filled; specAt the
		// first slot a dispatching word filled, -1 for none.
		fwd, specAt := CollectCandidateScan(e, sig, forwardLimit, positions, e.Pointer+1, checkActive, compiling)

		// 1.3: all params matched by forward?
		if fwd == nArgs {
			// Pattern check (post §1.1 fix): scalar literals route
			// through Signature.Patterns instead of value-tagged
			// type paths, so the pattern check has to run for
			// forward-matched positions too. The previous code
			// short-circuited here without consulting Patterns,
			// which made `def fact[0] (1)` fire for any integer.
			if !patternsOk(sig, positions, e.Tape, fwd, e.Registry) {
				continue
			}
			if preferWordSig && !isPreferred {
				if bestDeferred == nil {
					bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
				}
				continue
			}
			noteSplit(positions, fwd)
			return sig, positions, specAt
		}

		// Inside a pending forward scope: all args must come from
		// forward. Accept only if forward+stack would satisfy the sig.
		if insideForward && fwd > 0 {
			remaining := nArgs - fwd
			if len(resolved) >= remaining {
				canStack := true
				for j := 0; j < remaining; j++ {
					stackVal := resolved[len(resolved)-1-j]
					if !SigArgMatches(sig, fwd+j, stackVal) {
						canStack = false
						break
					}
				}
				if canStack {
					// Fill remaining positions from stack (nearest first).
					for j := 0; j < remaining; j++ {
						ri := len(resolvedIdx) - 1 - j
						positions[fwd+j] = resolvedIdx[ri]
					}
					// Pattern gate — this selection point must enforce
					// Patterns exactly like the full-forward (above) and
					// normal-stack (below) returns, or a sig whose
					// pattern rejects a filled position is selected
					// anyway (fn's tnot-List triple sig would claim a
					// spec-list call made inside an enclosing pending
					// forward with stack values available).
					if !patternsOk(sig, positions, e.Tape, fwd, e.Registry) {
						continue
					}
					if preferWordSig && !isPreferred {
						if bestDeferred == nil {
							bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
						}
						continue
					}
					noteSplit(positions, fwd)
					return sig, positions, specAt
				}
			}
			continue
		}

		// /f means all args must come from forward — if any args
		// remain unmatched after the forward scan, this sig fails.
		if w.ForceForward && fwd < nArgs {
			continue
		}

		// ── Step 2: stack matching ───────────────────────────────

		remaining := nArgs - fwd
		if len(resolved) < remaining {
			continue // not enough stack values
		}

		// 2.1: match remaining sig positions against the stack,
		// top-down. sig[fwd] = top of stack, sig[fwd+1] = next deeper,
		// etc. This is the "stack in reverse order" half of the
		// unified rule — same for stack-only sigs (BarrierPos=0)
		// as for partial-boundary sigs.

		allMatch := true
		for j := 0; j < remaining; j++ {
			ri := len(resolvedIdx) - 1 - j
			stackVal := resolved[ri]
			sigIdx := fwd + j

			// /q is a forward-only rule (see Signature.QuoteArgs doc).
			// stackVal cannot be a Word in normal execution: stepWord
			// has already resolved any Word at the pointer to a function
			// call, defined value, or Atom, and quote produces Atoms.
			// The branch below is defensive only — a stack Atom matches
			// an [Atom/q, ...] sig via the regular sigTypeMatches path
			// just below, no /q involvement required.
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Parent.Equal(TWord) {
				if !TAtom.ConformsTo(SigArgType(sig, sigIdx)) {
					allMatch = false
					break
				}
				positions[sigIdx] = resolvedIdx[ri]
				continue
			}
			// A /q position captures a literal word/atom; a NON-CONCRETE carrier
			// (a computed check-mode value) must not fill it via the
			// Any-conforms-to-everything rule — it belongs to a value overload.
			// Mirrors the forward-scan and positionalMatch /q guards. Inert at
			// runtime (operands are concrete there).
			if sig.QuoteArgs != nil && sig.QuoteArgs[sigIdx] && stackVal.Carrier && !IsConcrete(stackVal) && !stackVal.Parent.ConformsTo(TAtom) {
				allMatch = false
				break
			}
			if !SigArgMatches(sig, sigIdx, stackVal) {
				// A more-specific overload rejected because the stack-top
				// operand is a mixed gradual carrier: a concrete value drawn
				// from it might have matched here (and been grabbed). Remember
				// it; if a forward-collecting overload is then selected, the
				// split is ambiguous (noteSplit at the return points).
				if checkActive && j == 0 && e.Registry.analysisMixedConform(stackVal, SigArgType(sig, sigIdx)) {
					mixedCarrierRejectIdx = resolvedIdx[ri]
				}
				allMatch = false
				break
			}
			isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[sigIdx]
			if !isTypeArg && rejectsTypeLiteral(stackVal, SigArgType(sig, sigIdx)) {
				allMatch = false
				break
			}
			positions[sigIdx] = resolvedIdx[ri]
		}
		if !allMatch {
			continue
		}

		// Check structural patterns on every matched position. Post
		// §1.1 fix: scalar literals route through Patterns regardless
		// of whether they came from forward or stack matching, so the
		// pattern check no longer skips forward positions.
		if !patternsOk(sig, positions, e.Tape, fwd, e.Registry) {
			continue
		}

		// Full match found.
		if preferWordSig && !isPreferred {
			if bestDeferred == nil {
				bestDeferred = &matchResult{sig, append([]int(nil), positions...), specAt}
			}
			continue
		}
		return sig, positions, specAt
	}

	// Return deferred non-preferred match if one was found.
	if bestDeferred != nil {
		return bestDeferred.sig, bestDeferred.positions, bestDeferred.specAt
	}

	// Try fallback (0-arg or Fallback handler).
	for si := range fn.Signatures {
		sig := &fn.Signatures[si]
		if w.ArgCount >= 0 && sig.TotalArgs() != w.ArgCount {
			continue
		}
		if sig.TotalArgs() == 0 || sig.Fallback {
			return sig, nil, -1
		}
	}

	return nil, nil, -1
}

// sigOrderArgs reorders the recovery path's tape-ordered operands
// (the first nStack are stack args, ascending toward the pointer; the
// rest are forward args in source order) into SIGNATURE order, where
// sig[0] is the top of the stack. Per the one kernel arg convention,
// forward args fill the leading sig positions in source order and the
// stack args fill the remainder top-down — so the ascending stack run
// is reversed. The result feeds RecordPolyCall, which (like every other
// recorded dispatch) takes args in sig order so the VM re-match reads
// them faithfully.
func SigOrderArgs(args []Value, nStack int) []Value {
	if nStack < 0 || nStack > len(args) {
		nStack = len(args)
	}
	out := make([]Value, 0, len(args))
	// Forward args (sig[0..k-1]) — already in source order.
	out = append(out, args[nStack:]...)
	// Stack args (sig[k..N-1]) — top of stack is the last (closest to the
	// pointer) of the ascending run, so walk it in reverse.
	for i := nStack - 1; i >= 0; i-- {
		out = append(out, args[i])
	}
	return out
}

// tryRecordRecoveredUserFn records a GUARDED CALL_USER for a user-fn dispatch that
// matchSignature could not statically commit (an Any- or disjunct-typed operand),
// when the fn has EXACTLY ONE real (arg-bearing) overload and no 0-arg real sig the
// synthetic fallback would courtesy-dispatch. The sole sig either matches the
// runtime arg (dispatch == interpreter) or misses it (the CALL_USER param contract
// raises == the interpreter's fallback raise). Returns true — and splices the
// recovered returns — when it records; false leaves the caller's refusal to stand
// (multi-overload → Cluster C). The L4 leaf: design/VOXGIG-COMPILE-LEAVES.1.md.
// singleOverloadRecoverable reports whether fn is a user fn with EXACTLY ONE
// real (arg-bearing, non-fallback) overload — the shape whose dispatch over an
// Any/disjunct-carrier arg is RECOVERABLE: runtime dispatch is unambiguous, and
// the VM's CALL_USER param contract raises exactly as the interpreter would if
// the concrete value misses the sole sig. Used both to RECORD a guarded call on
// an armed (compile) pass — tryRecordRecoveredUserFn below — and to SUPPRESS the
// no_signature diagnostic on a plain check pass (Emit inactive), so a plain
// check agrees with a compile pass instead of flagging what compile silently
// recovers (the `engine-known engine` FP: an `is String`-guarded Options-`get`
// value whose type the plain-check pass leaves a strict-Any carrier).
func SingleOverloadRecoverable(sig *Signature, fn *FnDefInfo) bool {
	if fn == nil || sig == nil || sig.ReturnsFn == nil || sig.Fallback || sig.TotalArgs() == 0 {
		return false
	}
	realOverloads := 0
	has0ArgReal := false
	var sole *Signature
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue // synthetic 0-arg catch-all: raises for an arg-bearing fn
		}
		realOverloads++
		sole = s
		if s.TotalArgs() == 0 {
			has0ArgReal = true
		}
	}
	if realOverloads != 1 || has0ArgReal || sole == nil {
		return false
	}
	// BORU-BODIED user fn ONLY (Codex PR #211 review #2). A core native (one-sig
	// with a ReturnsFn, e.g. `enum`) and a trivial-delegation module wrapper
	// (body = [Word(inner)] short-circuiting to an inner native) do NOT get the
	// guarded CALL_USER param contract, so their unmatched dispatch is not
	// deferrable. The discriminator is a real boru body (len(Body) > 0), NOT a nil
	// Handler: a boru fn defined in a module preamble carries a synthesized CallBoru
	// Handler yet is genuinely boru-bodied (the `engine-known` case) — gating on
	// Handler==nil wrongly excludes it. Core natives have an empty Body; delegation
	// wrappers are caught by trivialDelegationTarget.
	if _, isDelegation := trivialDelegationTarget(sole); isDelegation {
		return false
	}
	return len(sole.Body()) > 0
}

func (e *Engine) TryRecordRecoveredUserFn(sig *Signature, fn *FnDefInfo, args []Value, nStack int, positions []int) bool {
	if !SingleOverloadRecoverable(sig, fn) {
		return false
	}
	recovered := sig.ReturnsFn(SigOrderArgs(args, nStack), e.Registry)
	CheckBraid.SpliceCheckResults(e, positions, recovered)
	return true
}

// concreteArgsMatch reports whether every NON-Any-carrier operand still MATCHES
// its sole-sig param position — so the failed dispatch is attributable SOLELY to
// the Any-typed (statically-unknown) operand(s), not a provably-wrong concrete
// arg. Guards the multi-arg case `g x 123` (g[a:Integer b:String]; x:Any->a ok,
// but 123->b:String wrong): the concrete mismatch stays a genuine error and must
// NOT be suppressed alongside the deferrable Any (Codex PR #211 review #1). Used
// only to gate the plain-check no_signature diagnostic; the armed recovery records
// a guarded CALL_USER that raises == interpreter regardless, so it needs no such check.
func ConcreteArgsMatch(sig *Signature, args []Value, nStack int) bool {
	window := SigOrderArgs(args, nStack)
	n := sig.TotalArgs()
	if len(window) < n {
		return false // arity shortfall — a genuine mismatch, do not suppress
	}
	for i := 0; i < n; i++ {
		v := window[i]
		if v.Carrier && v.Parent != nil && v.Parent.Equal(TAny) {
			continue // the deferrable unknown operand
		}
		if !SigArgMatches(sig, i, v) {
			return false
		}
	}
	return true
}

// tryRecordUnmatchedDispatchTrap compiles a STATICALLY-DEFINITE unmatched
// dispatch to a terminal OpTrap raising the interpreter's byte-identical
// error, so a spec ERROR row still yields a Program instead of falling back.
// Reached from checkModeAssumeSig's no-carrier fall-through, i.e. after
// matchSignature failed every candidate AND the disjunct-partition / Any-
// carrier poly recoveries declined.
//
// Definiteness is the soundness condition: the run-time dispatch must fail at
// this same point with the same error on EVERY run. That holds exactly when
// every value any candidate signature could examine is identical at run time —
// a concrete const, a bare type literal, or a raw word token (matchSignature
// is deterministic over identical inputs, and the check pass mirrors the
// interpreter's tape step-for-step over concrete values). It does NOT hold
// for:
//   - a CARRIER operand — UNLESS the carrier-disjointness extension proves
//     the failure anyway (Phase 6 M4, design/STAGE3-INLINING-DESIGN-ROUND.0.md
//     §6 Stage M4). The base hazard: a carrier's static tag is a declared
//     type, but the runtime value may carry a refined subtype tag (a fn
//     declared [Integer] can return a Pos-reparented value) or satisfy a
//     value-sensitive predicate param — the runtime match can succeed where
//     the static one failed. The extension re-admits exactly the shapes where
//     that cannot happen: for EVERY non-fallback overload, every carrier in
//     the overload's own candidate window is provably disjoint from EVERY
//     slot type of that overload (sigDefinitelyUnmatched) — the runtime
//     value's tag conforms to the carrier's static tag, so a Never meet with
//     no value-level membership admits no runtime refinement either (the
//     same proof checkBodyReturnConformance rides, residualProvablyDisjoint).
//     An overload whose window holds no carrier saw only runtime-identical
//     values, so its static failure replays verbatim; one whose window
//     cannot even fill (arity shortfall over the same tape) fails
//     deterministically too;
//   - a DYNAMIC operand (no static type at all) — routed to the runtime
//     rematch alongside carriers (REFUSAL-CLOSURE.0 §2): the rematch reads
//     only the operand's live runtime value, never the static tag;
//   - an UNDEFINED-word placeholder (the interpreter raises undefined_word,
//     a different taxonomy — and the pass already carries an error diagnostic
//     that refuses the program before Finalize);
//   - a 0-arg real (non-fallback) signature on the word: matchSignature's
//     fallback scan and the synthetic-fallback courtesy dispatch can run it
//     at run time instead of raising.
//
// The trap error mirrors the interpreter's raise at this point exactly: the
// void-argument-group override (def_error / no_value_error — voidArgErrorFor,
// whose void-group records the check pass populates identically) when a paren
// arg produced no value, else the plain "no matching signature" signature
// mismatch. Hints ride along verbatim where the interpreter's are static;
// sigError's stack-snapshot hint is advisory DX text, not part of the error
// taxonomy (code + detail + position) the differential gates.
//
// RecordTrap's own guard keeps this top-level-only (frames and units both at
// depth 1): a trap inside a branch arm or fn unit is conditional and stays a
// refusal. Returns true when the trap now owns the program's tail; false
// leaves the caller's MarkUncompilable refusal to stand.
func (e *Engine) TryRecordUnmatchedDispatchTrap(w WordInfo, fn *FnDefInfo, pos SrcPos) bool {
	es := e.Registry.analysisRecorder()
	if !es.Active() || !e.Registry.analysisCompiling() {
		return false
	}
	maxN := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.Fallback {
			continue
		}
		n := s.TotalArgs()
		if n == 0 {
			return false // courtesy-dispatchable at run time — not a definite failure
		}
		if n > maxN {
			maxN = n
		}
	}
	window := CheckBraid.CheckModeFallbackPositions(e, maxN)
	// The forward walk can collect positions INSIDE a not-yet-evaluated paren
	// group (checkModeFallbackPositions depth-tracks rather than stopping at
	// an open paren). The interpreter pre-evaluates the paren before its
	// match runs, so in-paren tokens are NOT the values the runtime match
	// examines — never definite. (`add 100 (for 4 [add i 1])`: the static
	// window holds the raw `for` word where the runtime sees the loop's
	// value; latent under the pre-M4 screens only because such programs
	// still refused at Finalize's residual seating, which the trap
	// truncation now legitimately skips.)
	for i := e.Pointer + 1; i < e.Tape.Len(); i++ {
		inWindow := false
		for _, p := range window {
			if p >= i {
				inWindow = true
				break
			}
		}
		if !inWindow {
			break
		}
		if IsOpenParen(e.Tape.At(i)) {
			return false
		}
	}
	vals := make([]Value, 0, len(window))
	hasCarrier := false
	for _, p := range window {
		v := e.Tape.At(p)
		if IsWord(v) {
			if wi, werr := AsWord(v); werr == nil {
				if top, ok := e.Registry.Defs.Top(wi.Name); ok {
					v = top // the binding is what matchSignature examined
				} else {
					// A name def-bound to a COMPUTED fn (the fn-carrier side
					// table — installDef installs no Defs binding for those)
					// is check-invisible but BOUND at run time: `a/v` there
					// delivers the real Function value the runtime match
					// examines, so this static no-match is a modeling
					// artifact, not a definite runtime failure (the pmany /
					// pseq shape: `def digits (pmany digit/v)` trapped
					// signature_error where the interpreter succeeds).
					// Decline; the caller's refusal stands and the program
					// falls back faithfully.
					if _, hit := CheckFnCarrierBind(e.Registry, wi.Name); hit {
						return false
					}
					// Known literals resolve exactly as the match's forward
					// walk resolved them (true/false → Boolean) — the value
					// the runtime dispatch examines.
					switch wi.Name {
					case "true":
						v = NewBoolean(true)
					case "false":
						v = NewBoolean(false)
					case "none":
						v = NewNone()
					}
				}
			}
		}
		vals = append(vals, v)
		if v.Undefined {
			return false
		}
		// A deferred-expression token (a reach path, an unexpanded paren
		// expr, a template string, a word splice) EXPANDS at dispatch/step
		// time, and its expansion can read state the check pass models only
		// abstractly — a reach over a mutated flex cell resolved at run time
		// where the static match saw the raw Reach token (flex.tsv L88/L95).
		// Its presence makes the failure non-definite; decline.
		if IsReach(v) || IsParenExpr(v) || IsInterpString(v) {
			return false
		}
		// A CARRIER operand is not concrete at compile time, so the rich
		// diagnostic this trap would bake (received-argument note,
		// per-candidate value verdicts) is built over the carrier — but the
		// interpreter builds it at run time over the carrier's CONCRETE
		// value, so the two would diverge (design/DIAGNOSTICS.0.md phase 7:
		// the compiled and interpreted reports must be byte-identical).
		// Decline; the whole program then falls back to the interpreter,
		// which raises the exact rich error at run time — free, since a
		// trap is terminal, so the program errors here either way and only
		// the (irrelevant, error-path) compilation of the tail is given up.
		// This supersedes the Phase-6 M4 carrier-disjointness trap, which
		// could not carry a matching rich diagnostic. Since the RUNTIME
		// REMATCH landed the decline is no longer terminal: the window
		// classifies as rematchable below, and the compiled program
		// re-runs the match over the CONCRETE values and builds the same
		// rich diagnostic at run time (or defers when it matches).
		//
		// A DYNAMIC operand (statically-unknown type — an evaluated flex
		// read, a do-result) classifies the same way (REFUSAL-CLOSURE.0 §2):
		// the rematch never reads the static tag, only the operand's LIVE
		// runtime value — which is exactly what the interpreter's dispatch
		// examines — so a bounded-dynamic that failed the static match
		// re-matches faithfully (defer on match, the byte-identical rich
		// raise on no-match). The provenance requirement still gates: a
		// dynamic with no compiled home fails RecordDispatchRematchValues'
		// operand resolution and the refusal stands.
		if v.Carrier || v.Dynamic {
			hasCarrier = true
		}
	}
	if hasCarrier {
		// Not statically definite — but every position is a runtime-stable
		// value or a provenance-carrying carrier: record the runtime
		// rematch (OpDispatchRematch), under three byte-identity guards.
		// (1) The written tuple sigError renders (the carrier-aware twin
		// of its forward-else-stack derivation) must be a CONTIGUOUS
		// SLICE of the window, proven by ID identity — its offset+length
		// ride as the spec's render bound (WrittenOff/NWritten), so a
		// wider match view than the raise view (the local-add shape's
		// match probed 3 positions where the error renders the single
		// stack value at offset 0; the each shape's body operand sits at
		// offset 1 after the region carrier) re-runs the match over the
		// full window while rendering over the bounded slice. An empty
		// tuple, or one absent from the window, cannot be rebuilt
		// faithfully and declines. (2)+(3) The
		// two TAPE-state diagnostic layers the runtime rebuild has no
		// access to — the tape reorder probe and the fn-shape
		// typed-binding hint — must not apply; runtimeNoMatch rebuilds
		// the value-based reorderHintFor itself. Declines leave the
		// caller's refusal.
		written := e.rematchWritten()
		if len(written) == 0 || len(written) > len(vals) {
			return false
		}
		off := -1
		for o := 0; o+len(written) <= len(vals) && off < 0; o++ {
			match := true
			for i := range written {
				if written[i].ID != vals[o+i].ID {
					match = false
					break
				}
			}
			if match {
				off = o
			}
		}
		if off < 0 {
			return false
		}
		if e.voidArgErrorFor(w.Name, pos) != nil {
			return false
		}
		if e.reorderHint(w.Name, fn) != "" || e.IsFnShapeTypedBindingContext() {
			return false
		}
		return es.RecordDispatchRematchValues(w.Name, vals, off, len(written), pos)
	}
	// Serialise the FULL interpreter error into the trap so the compiled
	// OpTrap raises byte-identical to the interpreter (Detail + spans +
	// notes + suggestions). Definiteness (screened above) guarantees the
	// runtime values equal what sigError saw here, so the error built now
	// is the error the interpreter builds at run time.
	if verr := e.voidArgErrorFor(w.Name, pos); verr != nil {
		return es.RecordTrapErr(verr, pos)
	}
	return es.RecordTrapErr(e.sigError(w.Name, fn, pos), pos)
}

// argTypeSummary renders the operand types of a failed dispatch for the
// no_signature expected-vs-actual message: comma-separated Parent names,
// dynamic carriers marked. Empty for a 0-arg dispatch.
func ArgTypeSummary(args []Value) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		switch {
		case a.Parent == nil:
			parts[i] = "?"
		case a.Dynamic:
			parts[i] = "dynamic(" + a.Parent.Leaf() + ")"
		default:
			parts[i] = a.Parent.Leaf()
		}
	}
	return strings.Join(parts, ", ")
}

// sigTypeSummary renders the best-fit candidate's declared arg types for the
// no_signature message. Empty for a 0-arg sig.
func SigTypeSummary(sig *Signature) string {
	if sig == nil || len(sig.Args) == 0 {
		return ""
	}
	parts := make([]string, len(sig.Args))
	for i, t := range sig.Args {
		if t == nil {
			parts[i] = "?"
			continue
		}
		parts[i] = t.Leaf()
	}
	return strings.Join(parts, " ")
}
