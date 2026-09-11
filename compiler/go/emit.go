package compiler

import (
	"maps"
	"math"
	"sort"
	"strconv"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// The bytecode recording pass — Stage 1 of design/boru-bytecode-plan.0.md.
//
// For the POSITIVE statement of what compiles and why — the rule each refusal
// gate below is defending — see design/COMPILABLE-SUBSET.md. Keep it in lockstep
// when widening the subset; the gates here are a checklist against that rule.
//
// The compiler is the carrier checker with a recording side effect:
// when CheckState.Emit is set, every check-mode dispatch that flows
// through carrierResults records a call event with full operand
// provenance, and Finalize linearises the event trace into a Program.
//
// Site taxonomy (boru-bytecode-readiness.0.md gap 1) — every dispatch
// is classified from the first commit:
//
//   - mono                — single checker-selected signature; compiles.
//   - poly (partitioned)  — a strict disjunct straddles signatures;
//     Stage 1 marks the program uncompilable, later stages emit
//     CALL_NATIVE_POLY here.
//   - dynamic             — a dynamic carrier reached the site; fallback
//     territory.
//   - meta                — RunInCheckMode / code-body / user-fn /
//     recovered dispatches; compile-time-only or fallback.
//
// Operand provenance rides Value.ID: NewValueRaw mints a unique ID
// per value and toCarrier/copies preserve it, so a dispatch arg is
// (a) a recorded output of an earlier call, (b) a literal — concrete
// at the dispatch, or a stripped top-level literal whose original
// was saved by RememberOriginal — or (c) unknown, which marks the
// program uncompilable rather than guessing.

// Site classes for SiteCounts.
const (
	SiteMono    = "mono"
	SitePoly    = "poly"
	SiteDynamic = "dynamic"
	SiteMeta    = "meta"
)

// Synthetic word names for the compiler-internal assembly events — a computed
// list literal (OpMakeList) and a computed map / make body (OpMakeMap). They are
// not real boru words; the recorder stamps them on the emitCall it records and
// producerWord / makeListRange read them back, so the two sides must use the
// same string. Centralised here rather than repeated as literals.
const (
	wordMakeList  = "[…]"
	wordMakeMap   = "{…}"
	wordInterp    = "`…`"
	wordDynApply  = "(…fn)"
	wordTypedBind = "def:…"
)

// operandKind discriminates how an EmitOperand sources its value. The kind
// is an explicit enum rather than a set of "-1 means unset" int fields so the
// struct's ZERO VALUE is the unambiguous opNone (an invalid operand, only ever
// returned paired with an ok=false flag) — never a valid-looking "const index
// 0." See eng/go/CLAUDE.md "No Zero-Value Overload (CRITICAL)": four parallel
// sentinel fields are exactly the smell that rule forbids, and a single missed
// initialization used to mean "Consts[0]" silently.
type operandKind uint8

const (
	opNone      operandKind = iota // zero value: unset / invalid (ok=false only)
	opConst                        // idx → Program.Consts index
	opEvent                        // idx → producing event sequence number
	opLocal                        // idx → frame-local slot (loop iterator / param)
	opType                         // idx → Program.Types index (canonical type)
	opClosure                      // closureUnit + closureCaps (a compiled body)
	opDynScope                     // idx → Program.Consts index of the NAME a runtime dynamic-scope lookup reads
	opDataScope                    // like opDynScope but a READ-AS-DATA lookup (OpLookupDynScopeData): a fn/parser binding pushes as data, no FnDefInfo defer
)

// EmitOperand names where one dispatch argument comes from. For every kind but
// opClosure the single idx field carries the kind-specific index/seq; the
// constructors below are the only construction sites for the indexed kinds, so
// the kind↔idx pairing can never drift.
//
// resIdx is meaningful only for opEvent (P5 multi-result lowering): it names
// WHICH of the producing event's results this operand is. A single-result
// event uses resIdx 0; a multi-result call (e.g. `swap`, a multi-return fn)
// distinguishes its N outputs by 0..N-1, matching the order the VM pushes the
// handler's results (results[0] deepest, results[N-1] on top).
type EmitOperand struct {
	kind   operandKind
	idx    int
	resIdx int
	// closureUnit indexes Program.Fns and lowers to OpPushClosure (plan P2);
	// closureCaps are the body's lexical captures, resolved in the ENCLOSING
	// scope and pushed (in CapturedBinding order) just before OpPushClosure,
	// which pops them into the closure value. Both are meaningful only when
	// kind == opClosure.
	closureUnit int
	closureCaps []EmitOperand
	// closureRet is the CALLBACK fn value's own return contract, carried to the
	// pushed closure VALUE (core.ClosurePayload) rather than to the shared
	// unit — see that type's field comment for why the unit is the wrong home.
	closureRet *ClosureRetSpec
}

// ConstOperand / EventOperand / localOperand / typeOperand build the indexed
// operand kinds — the only places that pair a kind with its idx. EventOperand
// additionally carries the result index within the producing event (P5).
func ConstOperand(idx int) EmitOperand { return EmitOperand{kind: opConst, idx: idx} }
func dynScopeOperand(idx int) EmitOperand {
	return EmitOperand{kind: opDynScope, idx: idx}
}
func dataScopeOperand(idx int) EmitOperand {
	return EmitOperand{kind: opDataScope, idx: idx}
}
func EventOperand(seq, resIdx int) EmitOperand {
	return EmitOperand{kind: opEvent, idx: seq, resIdx: resIdx}
}
func localOperand(slot int) EmitOperand { return EmitOperand{kind: opLocal, idx: slot} }
func typeOperand(idx int) EmitOperand   { return EmitOperand{kind: opType, idx: idx} }

// producer locates a recorded value: the producing event's seq and which of
// that event's results it is (idx 0 for the common single-result case). P5
// generalised producedBy from a bare seq so a multi-result call's N outputs
// stay distinguishable when a downstream operand resolves one of them.
type producer struct{ seq, idx int }

// eventFlags are the per-event compile flags, keyed by event seq in
// EmitState.eventInfo. Each is a property of the producing event:
//   - zeroOut:  branch seq → 0-output statement guard (residual skips it)
//   - typeOut:  event seq → its output is itself a type body
//   - valueDef: event seq → bound to a NAMED value-def (`def x (expr)`)
//   - generic:  event seq → recorded by the GENERIC RecordCall path (a plain
//     native), not a structured hook
type eventFlags struct {
	zeroOut  bool
	typeOut  bool
	valueDef bool
	generic  bool
	// mayBeFn marks a BRANCH event whose result may be a Function at run time
	// (an arm is an fn value), so a trailing arg over it in the residual lowers
	// to a runtime-conditional OpCallDynamic (`if c [99] MathUtil.sqrt 16`).
	mayBeFn bool
	// applyLoop marks a LOOP event whose body runs a per-iteration dynamic
	// apply (setLoopBodyApply). Its variadic value region may sit in a fn-body
	// residual under an inert tail: the RET then takes the RetReplay trim
	// discipline (finish()'s loop-apply residual detection) instead of the
	// variadic refusal.
	applyLoop bool
	// dynBodyResult marks a DYN-BODY dispatch's event (tryRecordDynBody —
	// a CALL_NATIVE whose handler re-runs the body at run time): its
	// runtime results are REAL stack values delivered before the frame's
	// RET, so a DECLARED return tuple pins the count there (the VM RET
	// raises the interpreter's exact "expected N return value(s)" on a
	// mismatch). Distinguishes it from a variadic BRANCH/LOOP residual,
	// whose 0-value runtime shape the interpreter tolerates and fixed
	// consumers must keep refusing.
	dynBodyResult bool
	// variadicResult marks an event whose result count is RUNTIME-VARIABLE — a
	// loop, or a branch whose arms leave different / multiple counts (`if c [] [a
	// b]`). Only a variadic-absorbing position (the program residual or a
	// no-contract `[]`-declared fn RET) may consume it; feeding it to a
	// fixed-arity operand refuses. A fn whose body residual is such an event is a
	// VARIADIC-RETURNING fn (fnUnitRec.variadic), so its call sites mark the call
	// result variadic too. Set structurally at record time (RecordBranch /
	// RecordLoop), mirroring lowerArms' merge-variadic accounting; over-marking is
	// sound (refuses more), under-marking is not.
	variadicResult bool
	// regionN is the loop region's STATIC size (trips x per-iteration net)
	// when every bound is a concrete integer and the loop runs at least one
	// iteration — the gate for the splice-at-depth def bind
	// (REFUSAL-CLOSURE S5). 0 = not statically counted.
	regionN int
	// firstElemType is the type of the region's FIRST-arrived (stack-deepest)
	// value — bodyStk[0].Parent. The S5 split binds that value, so the split
	// element carrier must be typed from it, NOT the loop carrier's child
	// type (which mirrors the LAST value — a heterogeneous body like
	// `for 2 [7 "x"]` binds Integer 7 while the carrier renders String).
	firstElemType *core.Type
	// spliceDyn marks an OpSpliceDyn event (§9.2b): its payload's provenance
	// was reassigned to this event, so a re-read of the payload after the
	// spread must decline (resolveResidualOperands).
	spliceDyn bool
	// regionMayBeFn marks a REGION event whose run can leave a CALLABLE value.
	// The two lanes disagree about one: the INTERPRETER re-steps a Function
	// that arrives on its stack (or in a handler's spliced-back results), the
	// VM appends it as data — `5 for 1 [g/v]` is [6] interpreted and [5 fn]
	// compiled. A region has no static count and no per-value seat, so nothing
	// downstream can re-step one; the only sound answer is not to CONSUME such
	// a region, which is what regionPrefixShape and regionCollectShape read
	// this for (via singleSlotRegion). Set at record time from the values the
	// region's run is modelled to leave.
	regionMayBeFn bool
	// variadicRegion marks a NATIVE call whose result is a runtime-variadic
	// REGION — the GROWING direction: the handler leaves 0-or-MORE values
	// where the recorded event carries ONE slot standing for the whole run
	// (await's winner-takes-all first/any, NUR067). The shrinking mark alone
	// (variadicResult, the L-DO catch: N seats, 1 delivered) cannot express
	// it — a count that EXCEEDS the static seat strands values around every
	// fixed layout — so the region takes the LOOP region's representation
	// instead: one slot, marked lw.variadic at lowering, absorbed only by a
	// variadic-absorbing position (the program residual / a no-contract RET)
	// and refused at every fixed-arity consumer, at promotion, and at the
	// dead-result drop. Self-identifying at record time: the check-side model
	// of "0-or-more values" IS core.NewVariadicCarrier, so the call's own outs
	// carry the mark (callVariadicRegion) — no latch to leak onto a later
	// dispatch.
	variadicRegion bool
	// splitBound marks a variadic loop region whose FIRST value an S5 split
	// bind consumed (SplitLoopRegionBind → RecordDynBind): the remaining
	// regionN-1 values are the statically-counted rest. Inside a LOOP BODY
	// (S9.2a) the rest is the iteration's residual, so RecordLoop admits the
	// region as a multi-value body result instead of refusing the variadic.
	splitBound bool
}

type emitCall struct {
	word              string
	sig               *core.Signature
	ops               []EmitOperand
	nout              int // number of results the call pushes (0 for a side-effect word, N for multi-result)
	pos               core.SrcPos
	poly              bool                  // dispatch via OpCallNativePoly (runtime MatchSignature)
	polyReg           *core.Registry        // the sub-registry to re-match a module poly word in (nil = main registry)
	polyNoMatch       *core.PolyNoMatchSpec // faithful-raise plan for the poly's runtime no-match arm (nil = defer)
	makeList          bool                  // assemble len(ops) operands into a list (OpMakeList) instead of dispatching a word
	dynApply          int                   // >0: apply the TOP operand (a runtime fn value) to the `dynApply` trailing args below it (OpCallDynTrailTop) — a paren-bounded trailing fn-value apply recorded as an EVENT so it seats like any computed result
	dynApplyUnquote   bool                  // the dynApply event came through the `apply` WORD (a consumed pendingApply): lower to OpCallDynApplyTop, which unquotes like applyHandler (Stage M2a)
	dynApplyOne       bool                  // the lead is GRADUAL (apply over a Dynamic value): lower to OpCallDynApplyOne, exactly one result or defer
	dynApplyKeepQuote bool                  // the dynApply fn is EVENT-provenance (a direct call result, no read substitution): lower to OpCallDynTrailKeepQ, which preserves the runtime quote state (quoted stays data)
	dynApplyName      DynApplyHead          // the BINDING name (and read position) the apply's head was read bare under, seated on the unit at the emitted pc (CompiledFn.DynApplyName); zero when the head was not a bare read
	dynMixed          bool                  // forward-drift window (REFUSAL-CLOSURE §1): island the len(ops) laid-out window [residual(s), dynamic value, word const, forward literal] verbatim via OpCallDynamicMixed — the island's own dispatch performs the interpreter's forward collection over the LIVE top value
	makeMap           bool                  // assemble len(ops) value operands into a map (OpMakeMap) with mapKeys
	mapKeys           []string
	mapImpl           bool // the source map's Implicit flag
	interp            bool // assemble len(ops) hole operands into a template string (OpInterp) per interpSegs
	interpSegs        []InterpSeg
	xmlTmpl           *core.XmlTmpl // assemble len(ops) hole operands into an XML element (OpInterpXml, §9.2c)
	spliceDyn         bool          // spread the ONE laid-out payload operand at run time (OpSpliceDyn, §9.2b)
	diverges          bool          // the word ALWAYS raises (CompileDiverges, e.g. raise): control never returns past this call
	// typedBind, when non-nil, marks this event as a typed value-def's runtime
	// validate/reparent step (OpBindTyped over the single operand) instead of a
	// word dispatch — recorded by RecordTypedBind from the def handler's
	// refinement branches. Riding emitCall keeps the whole generic evCall
	// machinery (operand provenance, value-def promotion, dead-drop, fragment
	// walks) working unchanged for the bind result.
	typedBind *core.TypedBindSpec
	// dynMethod, when non-nil, marks this event as a GUARDED shaped-instance-
	// method apply (Stage M2c, OpCallDynMethod): ops[0] is the runtime method
	// value (the dynamic dot-read result), ops[1..] the inert statement-window
	// args, and the spec pins the claimed arity + result count the VM enforces
	// (claim failure → internal_error → interpreter re-run). Riding emitCall
	// keeps the generic evCall machinery working unchanged for the result.
	dynMethod *DynMethodSpec
	// region, when non-nil, is the COMPLETED region descriptor for this
	// dispatch (region_complete.go): what the interpreter read off the tape at
	// this position, and which of those slots the dispatch claimed.
	//
	// It rides the EVENT rather than a table on the recorder, for the same
	// reason DispatchSpec is appended in lowerCall: a recorder-side table is
	// not rollback-safe. A non-final loop-analysis round is discarded by
	// truncating the event slice, so a descriptor appended beside it would
	// survive its own round and shift every later index. Riding the event puts
	// it under the existing rollback, and gives the lowering site the link
	// from a call in unit K to its own descriptor.
	region *RegionDesc
}

// emitBranch is a recorded `if`: a resolved condition operand, the
// captured then/else fragments, and each fragment's single result
// operand. constCond non-nil means the condition was statically
// known and only the taken fragment was captured.
type emitBranch struct {
	cond                  EmitOperand
	condFrag              *EmitFragment // list-form condition: lower inline, ends with one Boolean
	condOut               EmitOperand
	constCond             *bool
	hasElse               bool // false = 2-arg if; its result is VARIADIC (0 or 1 values)
	then, els             *EmitFragment
	thenOut, elsOut       EmitOperand
	hasThenOut, hasElsOut bool        // false when the arm DIVERGES (ends in break/continue)
	thenIsVal             bool        // then arm is a plain VALUE operand (`if cond 99 88`), not a body fragment
	thenVal               EmitOperand // the value-then operand (const/local/type, OR a COMPUTED event when thenComputed) when thenIsVal
	thenComputed          bool        // then value is a COMPUTED event eagerly on the stack below the cond (`if c (expr) e`): SWAP cond up, DROP it on the FALSE path
	elsIsVal              bool        // else arm is a plain VALUE operand (not a body fragment)
	elsVal                EmitOperand // the value-else operand (const/local/type, OR a COMPUTED event when elsComputed) when elsIsVal
	elsComputed           bool        // else value is a COMPUTED event eagerly on the stack below the cond (`if c [t] (expr)`): SWAP cond up, DROP it on the taken path
	pos                   core.SrcPos
}

// armKind classifies one `if` arm, collapsing the emitBranch boolean flags
// (thenIsVal / elsIsVal / elsComputed / hasThenOut / hasElsOut / hasElse) into a
// single discriminant. RecordBranch sets the flags as it discovers each arm's
// shape; consumers ask thenArm() / elseArm() and SWITCH on the result instead of
// re-deriving the kind from flag combinations. The flags stay the storage (one
// — hasThenOut/hasElsOut — is mutated by markTailCalls, so the kind tracks tail
// marking automatically).
type armKind uint8

const (
	armAbsent   armKind = iota // arm not present (a 2-arg if's missing else)
	armBodyOut                 // a […] body fragment that nets one value
	armBodyVoid                // a […] body that nets 0 values or diverges (still runs)
	armValue                   // a plain already-evaluated value operand (`if c 99 88`)
	armComputed                // an eagerly-computed (event) arm value (`if c [t] (expr)` / `if c (expr) e`)
)

// thenArm classifies the then arm. A then arm is always present (even a 2-arg
// if has one); it is a computed event value, a plain value, a value-netting
// body, or a void/diverging body.
func (br *emitBranch) thenArm() armKind {
	switch {
	case br.thenComputed:
		return armComputed
	case br.thenIsVal:
		return armValue
	case br.hasThenOut:
		return armBodyOut
	default:
		return armBodyVoid
	}
}

// elseArm classifies the else arm — armAbsent for a 2-arg if.
func (br *emitBranch) elseArm() armKind {
	switch {
	case !br.hasElse:
		return armAbsent
	case br.elsComputed:
		return armComputed
	case br.elsIsVal:
		return armValue
	case br.hasElsOut:
		return armBodyOut
	default:
		return armBodyVoid
	}
}

// emitLoop is a recorded counted `for`: the count operand, the
// captured body fragment (final fixed-point round), its single
// per-iteration result, and the iterator's local slot. The loop's
// runtime contribution is VARIADIC — one value per iteration stays
// on the stack — so downstream consumption refuses; only the
// residual may absorb it.
type emitLoop struct {
	start, end, step EmitOperand // start/step are always consts in Stage 2
	body             *EmitFragment
	// cond is a CONDITION loop's condition fragment (`while [cond] [body]`,
	// RecordWhile): lowered at the head of every iteration to its one value,
	// condOut, which a falsy test exits the loop on. nil for a counted loop.
	cond       *EmitFragment
	condOut    EmitOperand
	bodyOut    EmitOperand
	hasBodyOut bool // false: the body nets no value per iteration (or diverges)
	multiOut   bool // the body nets >1 value per iteration (net drivers): residualN reconciliation
	iterSlot   int
	pos        core.SrcPos
	// carried seeds the loop-carried def slots (a pre-loop `def` the body
	// REBINDS, read on a later iteration or after the loop) with their
	// pre-loop values — lowered right after FOR_SETUP, before the first
	// FOR_NEXT, so a zero-iteration loop leaves each cell at its pre-loop
	// value. See NoteLoopCarried.
	carried []carriedInit
}

// carriedInit seeds one loop-carried def slot: the unit frame slot and the
// operand holding the pre-loop value stored into it before the loop runs.
type carriedInit struct {
	slot int
	init EmitOperand
}

// emitStore is a recorded frame-local store — the loop-carried def REBIND
// (`def found true` inside a for arm, read on a later iteration or after
// the loop). src resolves in the recording scope; the lowering pushes a
// const/local/type source (or pops an event source off the sim top) into
// locals[slot] via OpStoreLocal. A store produces no values, so a rebind
// nets 0 exactly like the interpreter's def.
type emitStore struct {
	src  EmitOperand
	slot int
	pos  core.SrcPos
}

const (
	evCall = iota + 1
	evBranch
	evLoop
	evBreak
	evContinue
	evCallUser
	evFallback
	evTrap
	evStore
	evDynBind
	evBindTwin
)

// emitUserCall is a recorded call of a compiled boru fn: the target
// unit index, the args in sig order, the number of results the unit
// returns (0 for a side-effect / 0-return fn, N for a multi-return
// fn), and whether the lowering marked it a TAIL call (it then
// replaces the frame and control never returns to the site).
type emitUserCall struct {
	unit int
	ops  []EmitOperand
	nout int
	tail bool
	pos  core.SrcPos
	// poly, when non-nil, marks a runtime-dispatched MULTI-OVERLOAD user call
	// (OpCallUserPoly): the checker could not commit to one same-arity overload
	// (a gradual-Any arg reached two or more), so EVERY arm's body compiled to
	// its own unit and the VM re-runs MatchSignature at entry to pick the arm.
	// unit is -1 for a poly call (there is no single committed unit); ops/nout
	// keep the ordinary CALL_USER operand accounting, so every generic
	// evCallUser consumer (promotion, provenance, fragment residuals) works
	// unchanged. A poly call is never tail-marked (markTailCalls skips it).
	poly *emitUserPolySpec
}

// emitUserPolySpec is the recorded arm table of one poly user call — the
// word, the registry the check pass dispatched it against, and the parallel
// sigIdx/units/impls arm slices lowered verbatim into a UserPolyRef.
type emitUserPolySpec struct {
	word   string
	reg    *core.Registry
	sigIdx []int
	units  []int
	impls  []core.SigImpl
	// sigs is the frozen dispatch table for a body-local word (REFUSAL-
	// CLOSURE.0 §6b) — see UserPolyRef.Sigs. Nil for the live-Lookup mode.
	sigs []core.Signature
}

// emitFallback is a recorded interpreter-island fallback (Stage 5): a
// construct the compiler can't lower, captured as a self-contained
// re-runnable token span plus its threaded stack inputs.
type emitFallback struct {
	spanIdx int
	ins     []EmitOperand
	pos     core.SrcPos
}

// EmitTrap is a recorded terminal trap: a check-mode-suppressed runtime error
// (an orphan gen, an unpack of a missing key) compiled as an OpTrap. The
// program ends at the trap — the recorder drops everything after it.
type EmitTrap struct {
	spec TrapSpec
	pos  core.SrcPos
	// rematchWord, when non-empty, makes this a RUNTIME-REMATCH trap
	// (OpDispatchRematch): the statically-failed dispatch's window operands
	// ride in rematchOps (sig-position order — index 0 is what the failed
	// match examined first), re-matched over the live values at run time.
	// rematchWrittenOff/rematchNWritten are the render bound
	// (DispatchSpec.{WrittenOff,NWritten}): the contiguous rematchOps slice
	// the runtime diagnostic renders as the written tuple. NWritten is
	// always 1..len(rematchOps); the slice fits inside the window.
	rematchWord       string
	rematchOps        []EmitOperand
	rematchWrittenOff int
	rematchNWritten   int
}

// EmitEvent is one node of the recorded trace, tagged by kind. The two largest
// payloads — emitBranch and emitLoop, each carrying several inline emitOperands
// — ride behind pointers (set only for their kind, nil otherwise) so the common
// evCall event does not pay their size on every copy through frames / fragments;
// the small payloads stay inline. Every consumer is kind-guarded, so a nil
// br/loop on a non-matching event is never dereferenced.
type EmitEvent struct {
	seq   int
	kind  int
	call  emitCall
	br    *emitBranch
	loop  *emitLoop
	uc    emitUserCall
	fb    emitFallback
	trap  EmitTrap
	store *emitStore
	dyn   *emitDynBind
	twin  *emitBindTwin
}

// emitBindTwin marks one bind-ledger transition's STREAM POSITION (§6.5's
// inert-emission stage): idx indexes EmitState.bindTwins / the finalized
// Program.BindTwins. No operands and no outputs — the event exists purely so
// the lowering emits an OpBindTwin at the point in production order where the
// check pass performed the transition, which is the placement the
// rollback-and-replay flip will rely on.
type emitBindTwin struct {
	idx int
	pos core.SrcPos
}

// emitDynBind is one recorded `def` site (every value def records one,
// cheaply): the binding name, its interned name const, and the bound
// value's operand, resolved AT RECORD TIME while the unit context is
// live. The lowering emits a registry-visible OpBindDynScope for it ONLY
// when the name is in dynScopeNames (some OpLookupDynScope reads it);
// otherwise the event lowers to nothing.
type emitDynBind struct {
	name string
	// spliceDepth >= 0 marks the S5 first-value loop bind: the bound value
	// sits spliceDepth entries below the region top at bind time, and the
	// lowering emits the splice-at-depth OpBindGlobal (-1 = a normal bind).
	spliceDepth int
	// src carries a LOCAL operand when the bound value is a live frame slot
	// (a param re-bound by `def` — resolved via the unit's localByID at
	// record time, a pure read). An EVENT-produced value rides srcSeq as a
	// plain int, NOT an operand — so forEachOperand's reference counting and
	// the scopeFloor guard never see a phantom extra reference from a def
	// site whose bind the lowering skips (every def records one of these;
	// only dynamically-read names lower). Any other value rides val VERBATIM
	// and is interned UNPOOLED at lowering, only if the bind actually lowers
	// — record-time interning polluted the canonical const pool (a
	// reparented `def y:Pos 2` merging with the plain literal 2).
	src    EmitOperand
	srcSeq int // producing event seq for an event-sourced value; -1 otherwise
	val    core.Value
	pos    core.SrcPos
	// root marks a def recorded at the PROGRAM's top level (the root unit,
	// outside any fn body): its binding persists past the run via the
	// keep-on-compile contract, so a NON-concrete bound value must emit an
	// OpBindGlobal write-back replacing the kept check-pass carrier with the
	// runtime value (lowerDynBind). depth is Defs.Depth(name) right after the
	// check-pass install — the exact slot the write-back targets.
	root  bool
	depth int
	// residentTwin >= 0 marks an ARM-RESIDENT def site (§6.5's each-body
	// recovery): AdoptResidentTwins stamped this in-unit event with the
	// twin-table index its per-invocation install satisfies, and the
	// lowering emits the value operand + OpBindResident here instead of
	// the (root-only) global write-back. -1 — the sentinel set at the two
	// construction sites — is every unstamped def.
	residentTwin int
	// undef marks the TEARDOWN half of a var param's balanced
	// per-iteration pair (`__varundef r` — RecordDynUndef's event, riding
	// the dyn-bind kind so every event walk already routes it): no value
	// operand, no stack effect; stamped, it lowers to OpBindResident's
	// undef arm popping the live binding per element; unstamped it lowers
	// to nothing, and the name-keyed walks (the dyn-scope bind detector,
	// the def-site fn resolver) skip it — an undef binds nothing.
	undef bool
	// typeInstall marks a TYPE binding's push (RecordTypeInstall's event,
	// riding the dyn-bind kind for the same reason the undef half does):
	// no value operand and no stack effect, because the minted node is a
	// COMPILE-TIME product the twin table already holds. Stamped, it lowers
	// to OpBindResident's type arm, which re-installs the twin's captured
	// BODY once per element (minting that element's own node — replaying
	// the captured node instead is measurably wrong, NUR135); unstamped it
	// lowers to nothing, and the name-keyed walks skip it exactly as they
	// skip an undef — a type install binds no runtime value.
	typeInstall bool
}

// bindsValue reports whether this def-site event installs a RUNTIME value
// under its name. An undef binds nothing, and a type install binds a node
// the twin table already holds, so every name-keyed walk — the dyn-scope
// bind detector, the def-site fn resolver, the lowering's unstamped arm —
// asks this rather than testing one flag and drifting when a third
// operand-less shape arrives.
func (d *emitDynBind) bindsValue() bool { return !d.undef && !d.typeInstall }

// EmitFragment is a captured sub-trace: the events a branch body
// recorded, plus the sequence floor — operands inside the fragment
// referencing events BELOW the floor read enclosing computation,
// which Stage 2's closed-branch lowering refuses.
type EmitFragment struct {
	events   []EmitEvent
	startSeq int
	// id is the fragment's monotone identity (beginFragment) — the key the
	// residual-order hazard is tracked under (unit_memo.go). 0 is the root
	// frame, which never has one.
	id int
	// residualN is the RECORDED carrier-residual count for a branch ARM (the
	// number of runtime values the interpreter leaves) — set by RecordBranch. The
	// lowerer requires the lowered sim residual to match it exactly: a genuine
	// multi-value arm (`[n mul 2 m (n sub 1)]` records 2) is accepted, while a
	// single-value expression whose lowering happens to leave an extra sim slot
	// (`[({a:(get…)} get a)]` records 1 but lowers to 2) is REFUSED as before —
	// without this the extra slot would compile an unsound duplicate value. 0 ==
	// unset (non-arm fragments: a loop body / condition expects one value).
	residualN int
	// residualOps carries a fragment's full residual operand list, which
	// lowerFragment re-pushes in order when the lowering left the sim empty.
	// A multi-out LOOP body is captured only when every entry is INERT
	// (const/local/type — no events), because it re-pushes on EVERY iteration
	// (`for 3 [1 2]`); a branch ARM runs once per taken path, so
	// captureArmResidual admits event entries too and planValueDefLocals
	// force-promotes each to a frame local first. Function-typed entries were
	// screened at capture time (parked-fn hazard).
	residualOps []EmitOperand
	// applyArgs, when non-empty, marks a loop body that left a LEADING fn VALUE
	// (a returned closure / Function carrier on the sim top after the body events)
	// with these trailing STATIC arg operands above it — the per-iteration dynamic
	// apply `for n [(mk2 i) 10]`. lowerFragment pushes the args and emits a single
	// OpCallDynamic, netting one applied value per iteration (the leading-fn case
	// resolveDynamicApply lowers for the program residual, here inside a loop body).
	// Resolved at RecordLoop time in the enclosing scope; restricted to re-pushable
	// operands (const / local / type) so a computed arg — already on the sim — never
	// double-pushes (such a residual fails the sole-fn check and refuses instead).
	applyArgs []EmitOperand
	// applyFn, when set, is the leading fn value of the per-iteration apply as a
	// RE-PUSHABLE operand (a frame-local read — an `args.N` fold of a Function
	// param inside a fn unit) instead of an event on the sim: the body events
	// leave NOTHING, and lowerFragment pushes this before the applyArgs. A
	// break/continue raised inside the applied body escapes the island with the
	// registry FlowCtrl flag set; the VM translates it to the cross-frame
	// break/continue unwind (escapedFlow), exactly the interpreter's shared-tape
	// flow resolution.
	applyFn *EmitOperand
	// applyFirst places the per-iteration apply BEFORE the body's other
	// statements — the source order of `for n [(args.0 1) def acc …]`, where a
	// continue raised inside the applied body must skip the statements after
	// it. Set when every recorded body event follows the fn read in source
	// order; the apply-last hoist (the original layout) requires the opposite,
	// and a mid-body apply declines. Only meaningful with applyFn.
	applyFirst bool
}

// EmitState is the recording side of the compile pass. Set
// CheckState.Emit to a NewEmitState before a check run; call
// Finalize afterwards. All methods are nil-receiver-safe so hook
// sites need no guards.
type EmitState struct {
	// Compilable latches false at the first construct Stage 1 cannot
	// lower; Reason names the first offender.
	Compilable bool
	Reason     string

	// pendingRegions holds PHASE-A region captures — extent and written-order
	// Tokens, taken at collection time by tryRecordRegion — keyed by the
	// (word, position) join the record side shares. Phase B (SlotDesc.Source,
	// from the operand model) completes them; only a COMPLETED descriptor is
	// fit to emit, because Phase A leaves every Source at SlotNone and
	// RegionDesc.Validate rejects that by design.
	//
	// A key can be re-captured: the same source position dispatches again on
	// every loop iteration and every call of an enclosing fn. Overwriting is
	// correct rather than merely tolerable — RegionDesc is STATIC ("true of
	// every execution of the region", its own doc), so the extent at one
	// position does not vary, and the last capture equals the first.
	pendingRegions map[regionKey]pendingRegion

	// pendingLoopBind carries a SplitLoopRegionBind verdict to the
	// RecordDynBind of the same installAndRecordDef call (S5).
	pendingLoopBind *pendingLoopBind
	// loopSplitBinds names the S5 first-value loop binds, so dynScopeRescue
	// admits their top-level reads.
	loopSplitBinds map[string]bool
	// defReadGens snapshots each def-read's binding generation
	// (residualReadStable — the end-of-program re-push soundness gate).
	defReadGens map[string]int64

	// bindTwins is the pass's bind-twin table: one entry per bind-ledger
	// transition, appended by RecordBindTwin as NoteBindTransition fires
	// (§6.5's inert-emission stage). Deliberately OUTSIDE the event stream
	// and the Checkpoint/Rollback pools: the population is the ledger's, the
	// ledger's own suppressions are the single filter, and no note fires
	// inside a rolled-back loop round — so the table mirrors the ledger
	// verbatim, which is exactly what the emission gate asserts. Copied to
	// Program.BindTwins at Finalize; carries no instruction until the
	// rollback-and-replay flip gives each entry an op at its position.
	bindTwins []core.BindTransition
	// bindTwinEntries is 1:1 with bindTwins: the DefEntry each push
	// transition INSTALLED, captured at the note (the identical binding
	// object §6.5's replay re-installs — the same FnDefInfo, module
	// instance, minted node). Zero for a BindUndef, whose twin pops the
	// then-live entry at its own position. The sandbox harness replays the
	// corpus from these recorded captures, which is the exact contract the
	// twin ops will execute at the flip.
	bindTwinEntries []core.DefEntry
	// twinPlaced is 1:1 with bindTwins: true when the transition got a
	// stream event — appended live by RecordBindTwin, or ADOPTED after the
	// fact by AdoptBodyTwins when a once-run defs-keeping body word (`do`)
	// records its body as a closure unit (the unit makes the body's defs
	// frame-local, so the leak the interpreter delivers is re-delivered by
	// a placed twin op after the call — §6.5's replay, never
	// re-execution). A table-only twin is unplaced; under the regime the
	// program refuses unless every twin has a placed op. What stays
	// UNPLACED — and refused — is the multi-run body's twin (the each-body
	// class, whose runtime re-runs the body per element where the ledger
	// noted one generalized transition; arm-residency's territory).
	twinPlaced []bool
	// twinAdoptions logs the twin indices AdoptBodyTwins marked placed, in
	// adoption order, so Rollback can un-mark exactly the flags whose
	// adopted events its truncation discards (the adopted seqs ride the
	// frames like any event; the flags need their own undo).
	twinAdoptions []int
	// keepBodyDepth / keepTwinFloor / keepTaints bracket the OUTERMOST
	// keep-defs body run in flight (KeepDefsBodyGuard — `do`'s check-mode
	// body run): the floor is the twin-table length when it opened, and
	// keepTaints collects the sub-ranges noted under a nested NON-keep
	// body run inside it (BodyAnalysisGuard while keepBodyDepth > 0 — an
	// each/fold/branch body, whose runtime multiplicity a single replay
	// cannot represent; Codex P1 ×2 on #421). A nested KEEP run (do
	// inside do) deliberately does not taint: once-run composed with
	// once-run is still once-run, and the outer adoption replays its
	// leaks in note order.
	keepBodyDepth int
	// keepModuleDepth is the subset of keepBodyDepth whose runs are at
	// MODULE scope (FnBodyDepth == 0 when the guard opened), i.e. whose defs
	// the runtime really does leak to the enclosing scope. It exists so a
	// rebind can tell WHY the recorder is suspended — see
	// rebindReachesModuleScope (NUR117).
	keepModuleDepth int
	// multiRunModuleDepth is keepModuleDepth's twin for MultiRunBodyGuard —
	// an each/fold body at module scope, whose last iteration's def leaks the
	// same way a `do` body's does. Same reader, same reason.
	multiRunModuleDepth int
	keepTwinFloor       int
	keepTaints          [][2]int
	// lastKeepRange / lastKeepTaints publish the bracket when the
	// outermost keep run closes, for the dispatch record that follows
	// immediately (AdoptBodyTwins). The zero range is in-band: it is
	// empty, so an adoption with no bracketed run adopts nothing.
	lastKeepRange  [2]int
	lastKeepTaints [][2]int
	// lastMultiRun is MultiRunBodyGuard's latch: the twin-table range the
	// most recent higher-order body analysis noted, keyed by the body
	// Value's ID and the noting registry. The arm-residency bridge (the
	// dispatch record that follows the analysis in the same pipeline)
	// pairs exactly this range's twins against the compiled unit's def
	// sites; a mismatched bodyID — a nested body's analysis overwrote the
	// latch during the outer unit's compile — declines the bridge to a
	// sound refusal. Overwrite semantics: every close overwrites, and the
	// zero latch (empty bodyID) bridges nothing.
	lastMultiRun multiRunLatch
	// lastClosure is the closure-compile latch: the unit index the most
	// recent recordClosureDispatch REAL body compile produced, and whether
	// it was fresh (a memo hit reuses a unit whose def events another
	// dispatch's twins already own — the bridge must decline, leaving the
	// second dispatch's twins unplaced and the program refused, sound).
	lastClosure closureLatch
	// armResidentDepth brackets the closure compiles of a
	// BodyMultiRunKeepsDefs word's body (tryRecordClosure sets it around
	// recordClosureDispatch; forkForProbe copies it so the probe admits
	// the same population): inside the bracket, RecordDynBind's `_`/`$`
	// name skip is lifted so an underscore-named body def gets its event
	// seat (`def _2 …` in row 41 — the ledger notes it, so the bridge
	// needs its event).
	armResidentDepth int
	// armBoundNames are names whose binding, after adoption, exists at
	// runtime ONLY through arm-resident installs — count, values, and
	// even definedness (a zero-iteration collection) are body-run-
	// dependent, so a later root read would bake the check model's
	// generalized value where the interpreter reads the runtime state.
	// NoteDefRead poisons armReadRefusal on such reads; a later LIVE
	// root install of the name (RecordBindTwin's placed arm) clears it —
	// the new binding is the read's referent again.
	armBoundNames map[string]bool
	// supersededTwins are twin-table indices recorded by an analysis round
	// a LATER round of the same body replaced (the fold-accumulator fixed
	// point re-runs its body until the accumulator type settles). They
	// describe no runtime transition — only the surviving round's ops
	// execute — so the placement gate exempts them; see MultiRunBodyGuard.
	supersededTwins map[int]bool
	// armReadRefusal is the placement-gate poison a root read of an
	// arm-bound name latches (first read wins, mirroring
	// MarkUncompilable's first-reason rule). The fence is the REGIME's
	// own machinery — the default recorder compiles the same read by
	// const-folding the kept install — so it refuses through Finalize's
	// placement seam under the "twin regime:" prefix, the layer the
	// full-placement gate already owns, not through a recorder-layer
	// MarkUncompilable site (the refusal-site census counts that layer,
	// and its count only falls).
	armReadRefusal string
	// storedGradualDepth marks a DETACHED stamp compile (StampDetachedFn
	// sets it on the fork's private EmitState). While non-zero,
	// buildFnBodyReturnsFn generalises an Any arg into an Any param as a
	// GRADUAL carrier (ParamInputCarrier) so a nested callee's body keeps
	// poly-matching optimistically — the same modality the unit's own
	// params get from fnValueInputs. Detached-only by design: the fork
	// owns its program AND its Finalize, so a gradual-caused failure at
	// ANY stage (analysis, unit compile, lowering) is contained to one
	// declined stamp. Applied to the in-program stored-handler path the
	// same generalisation leaked whole-program refusals at Finalize (a
	// gradual nested unit recorded into the main program whose Stage-2
	// lowering later refused — the module-repl corpus rows).
	storedGradualDepth int

	// inStampCompile marks an EmitState that IS a stamp compile. It fences
	// stampFnConst against re-entering itself: that helper stamps every fn
	// const it interns, and a stamp compile interns, so a fn const inside a
	// stamped body would compile to stamp it, and so on down — bounded only by
	// fn nesting depth, which is not a bound. Nothing is lost: the inner fn
	// compiles as part of the unit being stamped, and if it escapes to be
	// applied dynamically the OUTER program stamps it where it interns it.
	inStampCompile bool

	// stampDeclined memoises the sig impls whose stamp already refused, keyed
	// by the impl pointer the stamp would write to. The succeeding case
	// memoises itself through impl.Compiled; without this the failing case
	// re-paid a full body compile at every operand occurrence.
	stampDeclined map[*core.BoruImpl]bool

	// stampImpls maps a stamped sig impl to the unit backing its ref, so a
	// stamp-only unit whose lowering refuses can find and clear the ref that
	// would otherwise point at a trap stub.
	stampImpls map[*core.BoruImpl]int

	// reg is the registry the check pass runs against — set once at the top of
	// Engine.Run while emit is active. It lets recorder-internal helpers reuse
	// the lang-layer closure compiler (compileClosureBody) for a fn VALUE a body
	// RETURNS (the factory pattern), which needs r but is reached from a finish
	// closure that has only es. Nil outside a compile pass.
	reg *core.Registry
	// progReg is the TOP-LEVEL program registry — captured at the FIRST
	// BindRegistry (the outermost check Run, before any module body / island
	// sub-engine re-binds reg). Unlike reg it never re-binds, so Finalize can
	// tell an ordinary top-level fn (rec.reg == progReg) from a foreign
	// sub-registry fn (a module preamble body, rec.reg != progReg) and stamp
	// CompiledFn.Reg only for the latter — see the Finalize stamp site.
	progReg *core.Registry
	// bindSnap is the twin regime's ROLLBACK BASE: progReg's runtime-visible
	// bindings as they stood at that first bind, before the check pass
	// performed a single transition. Finalize stamps it on the Program
	// (ReplayBase / ReplayReg) and RunProgram restores it before executing,
	// so the rollback travels WITH the program and every compile-then-run
	// caller inherits it — lang's entry points and the low-level
	// CompileCheck-then-RunProgram flow alike (Codex P1 on #426: that flow,
	// and eng's own compile-then-run tests, had no rollback, so the replay
	// stacked a second install on the pass's kept one).
	bindSnap core.BindingSandbox
	// SiteCounts tallies dispatches per site class while recording is
	// active (counting stops once the program is marked
	// uncompilable, with the rest of the recording).
	SiteCounts map[string]int

	suspended  int
	seq        int            // monotonic event sequence
	frames     [][]EmitEvent  // frames[0] = top level; fragments push
	fragFloors []int          // startSeq per open fragment frame
	captureArm bool           // next RunCarrierBodyWithDefs records a fragment
	loopArm    bool           // next AnalyseLoopBody records its final round
	fnArm      bool           // next AnalyseFnBody records (fn compilation open)
	captured   *EmitFragment  // last completed fragment, until TakeFragment
	units      []*emitUnit    // units[0] = top level; fn compilations push
	fnUnits    map[string]int // fn memo key → Program.Fns index
	fnRecs     []*fnUnitRec
	// storedFnRefs collects the CompiledFnRefs minted while baking store-fn
	// handler consts (compileStoredFnUnit). Finalize back-stamps each ref's
	// Prog once the *Program exists and copies the slice onto Program. Nil
	// until the first store-fn handler is compiled.
	storedFnRefs []*CompiledFnRef
	// storedFnProbeReason holds the LAST stored-fn probe's refusal reason
	// (compileStoredFnUnit), for the -compile-report attribution
	// (stamp_report.go). Set only on a probe failure; the compile-time
	// bake ignores it.
	storedFnProbeReason string
	producedBy          map[string]producer // value ID → producing (event seq, result idx)
	// appliedByWord holds the value IDs a trailing `apply` WORD dispatched,
	// recorded PROGRAM-wide rather than per unit.
	//
	// The park rule (design/PAREN-RESTEP-RULE.0.md) says a user fn's returned
	// closure is PARKED where it lands, so `5 (mk 3)` leaves `[5 fn]`. But
	// `10 (mk2 5) apply` parks the same way and then APPLIES, because the
	// trailing word dispatches the parked value on purpose — and both lower to
	// the same OpCallDynamicTrailing shape, so trailingApply cannot tell them
	// apart from the residual alone (NUR124).
	//
	// The unit-scoped pendingApply cannot supply this: it returns false
	// outright when no fn unit is open, which is exactly the program
	// residual's case. `apply` records as an IDENTITY call (args[0].ID ==
	// outs[0].ID, the check engine re-stepping the fn concrete), and that ID
	// is the one trailingApply meets, so the mark rides here instead.
	appliedByWord map[string]bool
	// trailingApplies maps a Function VALUE's ID → the arg count of a paren-bounded
	// TRAILING fn-value apply (`(prev key comp)`), registered at the paren-collapse
	// boundary (registerTrailingApply) where the paren-group size is known. The body
	// reconciliation reads it back to lower the apply to OpCallDynTrailTop — the
	// flattened residual cannot otherwise recover the apply's arity.
	trailingApplies map[string]int
	// freshenConst marks const-pool indices holding a compound VALUE literal
	// that was materialised while recording a FN UNIT and whose ID is not an
	// enclosing binding's — i.e. a literal written in the body, which the
	// interpreter re-constructs per call. finalize's freshenFnUnitConsts pass
	// rewrites a single-push-site marked const to OpPushConstFresh (per-call
	// identity), keeps a multi-push-site one shared when nothing compound can
	// escape the fn, and refuses otherwise. See OpPushConstFresh (bytecode.go)
	// and design/MISCOMPILE-HUNT-FINDINGS.0.md §A.
	freshenConst map[int]bool
	// fnRiskFields maps a constructed INSTANCE's value ID → the field keys
	// holding genuinely-0-param fn values (noteFnRiskFields /
	// instanceFnFieldRisk — the carrier-receiver auto-dispatch hazard).
	fnRiskFields map[string]map[string]bool
	// fnMemberFields is fnRiskFields' any-arity VALUE-carrying sibling
	// (the instance-field fn-read fix): constructed-instance value ID →
	// field key → the fn member VALUE from the concrete construction map.
	// Consulted at the get-family tag site (instanceFnMember) so a read
	// through the instance CARRIER — whose payload inspection sees only the
	// schema — still tags noteMemberFnRead: the §3 arrival model then
	// compiles the landing with parity, and every shape it declines gets
	// refuseStrandedMemberFn's sound refusal instead of the pre-existing
	// stranded-apply miscompile (`o.f 21 eq 42` → fn-as-data + eq(21,42)).
	fnMemberFields map[string]map[string]core.Value
	// memberFnReads holds the value IDs of get-family reads that surfaced a
	// FUNCTION-valued container member (readsFnMember). The read's static type is
	// dynamic(Any), so downstream code cannot see it is a fn; this side table
	// lets the stranded-member-fn guard (refuseStrandedMemberFn) recognise the
	// value and refuse a mid-expression auto-apply
	// (design/EDGE-SPEC-FINDINGS.0.md §2). Over-approximate + append-only like
	// fnRiskFields — a stale entry only ever over-refuses (sound).
	memberFnReads map[string]core.Value

	// shapedReads mirrors CheckState.MethodShapes annotations (by read-out
	// value ID) so the get-family read guards can exempt a read whose
	// landing the shaped-method model owns. See NoteShapedRead.
	shapedReads map[string]bool
	// defReads maps a value ID to the BINDING NAME the check pass read it
	// from (stepWord's simple-value substitution). When such a value later
	// fails operand resolution inside a fn unit — no param/capture/local/
	// event/const home — the read was a DYNAMIC-SCOPE reference (a callee
	// reading the caller's frame binding), and resolveOperand lowers it to
	// a runtime OpLookupDynScope instead of refusing, gated on the check's
	// binder/call-graph reachability model (dynamicScopeReachable). See
	// NoteDefRead.
	defReads map[string]string
	// collectionHazard marks fn-typed value IDs a later dispatch collected
	// past while they sat unapplied (NoteCollectionHazard, NUR121).
	collectionHazard map[string]bool
	// reStepNotes marks the events whose results the check pass re-steps
	// into a dispatch attempt (NoteFnResultReStep, NUR124), by producing
	// seq: where the interpreter resumes after the results, and whether a
	// noted result is fn-TYPED (the runtime value is certainly a fn) rather
	// than a gradual maybe-fn.
	reStepNotes map[int]reStepNote
	// dynBoundClosures names the dyn-scope binds whose value is a COMPILED
	// closure (a ClosurePayload). Applying one from compiled code is fine —
	// §9b's factory family does exactly that — but an interpreter RE-RUN
	// cannot apply it (payload.go's contract, plan P2), so a code body handed
	// to a token-re-running native must not READ one. See
	// recordCodeBodyClosureRead.
	dynBoundClosures map[string]bool
	// valReadIDs holds the IDs of `/v` reads aliasValRead traced to a
	// def-bound produced fn value: data on both engines, never a paren
	// that failed to collapse (argIsProducedClosure).
	valReadIDs map[string]bool
	// readOps holds the operand a `/v` read of a def-bound CAPTURING fn
	// LITERAL resolves to (the thirty-third increment): the literal's
	// closure unit pushed with its captures, built at the first read
	// (tryReturnedClosure) and consulted by resolveOperand before any other
	// resolution — the read's fresh ID has no producer, and the literal
	// itself has no event.
	readOps map[string]EmitOperand
	// valBindEpoch counts every dyn-bind of a name this pass recorded, in
	// any unit: a valBind made at an older count is stale — a def of the
	// name in a CALLED fn's body replaces an overlapping outer fn binding
	// for good on the interpreter (drop-then-push, §6.5) where the
	// analysis restores its snapshot after the call, so the entry on top
	// again is no proof the run holds it (`def p (kk 7) end def g fn [[]
	// [Integer] [def p (kk 9) 1 p/v apply]] end g 3 p/v apply` answered 10
	// for the interpreter's 12 before this count).
	valBindEpoch map[string]int
	// defNameAt is the def name a produced fn value (a closure a unit
	// returns, a fn-value apply's result) was bound under, keyed by its
	// producing event slot — read by the lowering's promoted store to seat
	// CompiledFn.StoreNames (RecordDynBind notes it).
	defNameAt map[seqIdx]string
	// rootComputedBindIDs holds the value IDs of TOP-LEVEL computed fn-value
	// defs (`def op (Parse.parser g)` and the sibling mini/emit value forms)
	// that installDef DECLINED to install in Defs (the compiled-closure
	// machinery owns the name). snapshotAllBindingIDs iterates Defs, so it
	// misses these — yet they ARE real enclosing bindings at run time. Merging
	// them into a fn unit's enclosingBindIDs lets a deeper body's read of the
	// name (`parse op src`) rescue to a runtime read (OpLookupDynScopeData)
	// instead of taking the unreachable enclosing-producer operand and refusing
	// "reads enclosing computation". Populated by RecordDynBind.
	rootComputedBindIDs map[string]bool
	// dynScopeNames collects every name resolveOperand lowered as an
	// OpLookupDynScope read. The Finalize pass installs an OpBindDynScope
	// twin in every unit (params and body-local defs) and at every
	// top-level def that binds one of these names, so the runtime lookup
	// finds the same binding the interpreter's def stack holds.
	dynScopeNames map[string]bool
	// unitNames parallels the open-unit stack with each unit's FN NAME, so
	// the dynamic-scope rescue knows which fn is READING (the FnNameStack
	// may already have advanced past a memoised body analysis by the time
	// the unit finish resolves its residual).
	unitNames []string
	// openUnitRecs parallels the open-unit stack with each unit's fnRecs
	// INDEX (units[0], the top level, has no rec and no entry here), so a
	// mid-body dispatch can ask what KIND of unit it is recording into —
	// specifically whether the innermost open unit is a CLOSURE body
	// (inClosureUnit), where the `args` frame projection must decline: the
	// closure's analysis args frame is the CallableSpec inputs, not the
	// enclosing fn's per-call args the interpreter reads at run time.
	openUnitRecs []int
	// inlineCtxBounds tracks the OPEN inline context-boundary regions
	// (EmitRecorder.PushInlineCtxBoundary): each entry latches
	// len(openUnitRecs) at region entry, so InInlineCtxBoundary is true only
	// while recording INLINE within the innermost region — a closure/fn unit
	// opened inside the region (whose body the compiled runtime brackets with
	// its own context frame at enterBodyUnit) breaks the equality until it
	// closes. NUR054.
	inlineCtxBounds []int
	// dynEnv arms the program-wide DYNAMIC-ENVIRONMENT mode: a CompileDynBody
	// dispatch was recorded (tryRecordDynBody — a computed or context-word
	// `do` body lowered to a CALL_NATIVE whose handler re-runs the body at
	// run time). The body's sub-run resolves names against r.Defs and reads
	// r.Args, so the compiled program must mirror the interpreter's whole
	// environment: EVERY def emits its OpBindDynScope twin, EVERY named unit
	// param dyn-binds at frame entry, tail calls are disabled (binding
	// lifetime), and the VM brackets each CALL_USER frame with an args-stack
	// push (Program.DynEnv). Costs are paid only by programs that use
	// dynamic code bodies.
	dynEnv bool
	// catchVariadicPending latches the next CompileFallbackBody dispatch's
	// recorded result as VARIADIC (SetCatchVariadic / catchVariadicFor —
	// the fallible multi-value `do` body, plan Phase 5 L-DO).
	catchVariadicPending bool
	// bodyStarts holds, keyed by body value ID, the binding table a leaking
	// body's analysis run STARTED from (published by KeepDefsBodyGuard /
	// MultiRunBodyGuard at close, claimed by tryRecordClosure's takeBodyEnv)
	// — the compile re-run's environment (unit_memo.go). The frozen-read
	// table this replaced (a program-wide name→bake map) now lives on each
	// fnUnitRec (bakes / frozen), because the question it answers is per
	// unit: which call sites can still reuse this unit.
	bodyStarts map[string]bodyStart
	// fragSeq / fragIDs: the monotone fragment identity counter and the ids
	// of the OPEN fragment frames (parallel to frames[1:]; the root frame is
	// id 0 and is never on the stack). See beginFragment.
	fragSeq int
	fragIDs []int
	// fragReads / bindHazard / storeHazard drive the residual-order hazard
	// (unit_memo.go residualReadHazard), keyed by fragment id: the names a
	// fragment has read, and the names (by name) or loop-carried slots (by
	// slot) it, or a fragment nested in it, then rebound while that read
	// was already in it. Refusal-direction only, so — like the frozenReads
	// table they replaced — they are DELIBERATELY absent from
	// emitCheckpoint and Rollback: a retained entry can only over-refuse,
	// and the miscompile direction is a missing one, which Rollback
	// structurally cannot produce.
	fragReads   map[readKey]bool
	bindHazard  map[readKey]bool
	storeHazard map[slotKey]bool

	// storedBodyFnResidual records, for the ONE dispatch whose operands are
	// being built, whether any STORE-BODY-LIST element can leave a CALLABLE
	// value: a compiled branch unit whose residual holds one, or a non-empty
	// element that did not compile (its residual is then unknown). RecordCall
	// reads it a few lines later to decide whether the dispatch's variadic
	// residual may be recorded as a REGION. Reset at the head of
	// RecordCallOperands so it cannot outlive the dispatch that set it — the
	// property of a latch that catchVariadicPending has to buy with a
	// signature test.
	storedBodyFnResidual bool
	// eventInfo holds the per-event compile flags, keyed by event seq. It
	// consolidates the former parallel zeroOutSeq/typeOut/valueDefs/genericSeq
	// maps: each is a "property of event N", read via a producer's seq
	// (producedBy[id].seq), so they stay seq-keyed rather than moving onto the
	// value-copied EmitEvent struct (a flag set after append wouldn't reach the
	// frame/fragment copies).
	eventInfo map[int]eventFlags
	consts    []core.Value
	constIdx  map[string]int // CanonValue → Consts index
	// constIDIdx pools COMPOUND consts by value ID: the same materialised
	// List/Map value (same ID, same payload pointer — already identity-
	// aliased) reuses one Consts slot, so freshenFnUnitConsts' push-site
	// counting sees reads of one binding as pushes of ONE const. Distinct
	// source literals keep distinct IDs, so the never-CanonValue-dedup rule
	// (gotcha #13) is untouched.
	constIDIdx map[string]int
	types      []TypeRef
	typeIdx    map[string]int        // type ID → Types index
	fallbacks  []core.FallbackSpan   // Stage 5 interpreter islands
	origByID   map[string]core.Value // stripped literal ID → original value
	// trapAt is the seq of a recorded TOP-LEVEL terminal trap (a check-mode-
	// suppressed runtime error compiled as OpTrap), or 0 for none. When set,
	// Finalize ends the program at the trap. seqs start at 1, so 0 is a safe
	// "none" sentinel.
	trapAt int
	// markWindowSeq is the REGION-ANCHOR event seq of a planned mark-window
	// island (plan Phase 5, L-DO part 2b), or 0 for none: Finalize's
	// pre-lowering probe (markWindowShape) sets it alongside
	// lw.markBefore[seq], and resolveDynamicApply's post-lowering arm returns
	// OpCallDynMixedFromMark only when this latch armed — the two phases stay
	// in lockstep (the Finalize-order constraint: markBefore is read during
	// lowerEvents, the dynOp is decided after). seqs start at 1.
	markWindowSeq int
	// lastUserPoly notes the OpCallUserPoly event RecordUserPolyCall just
	// recorded, so recordCallElided's poly-alias arm can re-link the SAME
	// dispatch when the caller (carrierResults) rebuilds the out carriers
	// after the ReturnsFn returned — the gradual first-match-partition
	// widening mints FRESH result IDs, which would otherwise orphan the
	// recorded event and re-refuse the dispatch generically ("user fn call …
	// Stage 3") on a program whose poly plan compiled every arm (the
	// §8.2(3) return-join). Cleared by every appendEvent: only the
	// immediately following generic record of the same word may consume it.
	lastUserPoly *lastUserPolyNote
	// loopCarried is the stack of OPEN armed-loop carried-def scopes (one per
	// nested AnalyseLoopBody running with loop capture). Each maps a rebound
	// pre-loop def NAME to the unit frame slot carrying it across iterations;
	// RecordDefRebind consults the stack at `def` sites, innermost-first.
	loopCarried []*loopCarriedScope
	// pendingCarried is the just-closed loop analysis's carried-slot init
	// list (EndLoopCarried), consumed by the RecordLoop that follows.
	pendingCarried []carriedInit
}

// loopCarriedScope is one armed loop's carried-def registrations: the unit
// depth the loop records in (defs inside a nested fn compilation must not
// match), the name→slot map, and the slot inits queued for RecordLoop.
type loopCarriedScope struct {
	unitDepth int
	slots     map[string]int
	inits     []carriedInit
}

// emitUnit scopes local-slot numbering to one code unit (the
// top-level program, or one compiled fn body — locals are
// frame-relative at run time).
type emitUnit struct {
	localByID map[string]int
	numLocals int
	// reg is the compiled fn's OWNING registry (a module sub-registry for a
	// module-preamble fn; the main registry otherwise). Finalize stamps it
	// on the CompiledFn when it differs from the check registry, and the VM
	// then dispatches the unit's natives against it — the interpreter's
	// CallBoru runs a foreign fn's body IN its own registry, so handlers
	// with registry-visible effects (Net.listen forking per-connection
	// registries) see module scope on both engines.
	reg *core.Registry
	// capID marks the value IDs that are this unit's CAPTURES (enclosing-scope
	// values bound into trailing slots). A capture's value may ALSO carry a
	// producedBy entry from the ENCLOSING unit (a computed `def a (h add 1)`
	// snapshotted into the closure) — that event lives in the parent and is
	// unreachable here, so the body must resolve the reference to its own
	// capture SLOT, not the parent event. resolveOperand consults this to
	// override its usual events-first precedence for captured IDs. Pairs with
	// the parent-side promotion of the captured computed def to a frame local
	// (forEachOperand / promoteOperand closureCaps handling), which makes the
	// closureCaps operand a re-pushable LOCAL so the captured VALUE is correct.
	capID map[string]bool
	// deoptEnv marks a unit carrying a per-read deopt (planDeopts, NUR123);
	// deoptNames are the names its islands may read — every param and
	// body-local def among them is registry-visible and every such def's
	// computed source promoted, so an island runs under the interpreter's
	// frame environment; the unit never tail-calls.
	deoptEnv   bool
	deoptNames map[string]bool
	// enclosingIDs snapshots, at unit open, the value IDs of every compound
	// (List/Map) binding visible in the DefTable — i.e. values an ENCLOSING
	// scope already constructed. A compound const whose ID is in this set was
	// read from a binding (one shared instance per binding, interpreter
	// semantics), so it must NOT be freshened per call; a compound const whose
	// ID is absent was minted by evaluating a literal written in THIS body, so
	// the interpreter constructs it fresh per call and the VM must match
	// (OpPushConstFresh; miscompile mechanism A). Body-local defs bind their
	// IDs only DURING body analysis, after this snapshot — so they correctly
	// classify as body-fresh.
	enclosingIDs map[string]bool
	// enclosingBindIDs snapshots, at unit open, the value IDs of EVERY def
	// binding visible in the DefTable — the superset of enclosingIDs (which
	// filters to freshenable compounds). A value whose producing event is
	// recorded but whose ID appears here was read from an ENCLOSING-scope
	// binding (a module-scope `flex`, a computed module `def`), NOT produced
	// inside this fn body: the producer event lives in the parent/module
	// frame and is unreachable from this unit's sim stack. resolveOperand
	// routes such a read to a dynamic-scope lookup (OpLookupDynScope) so the
	// VM re-resolves the LIVE binding per call — the same cell the interpreter
	// reads, by-reference for a mutable flex. (Immutable list/scalar literals
	// have no producing event, so they still const-fold; only computed /
	// mutable enclosing bindings take this path.)
	enclosingBindIDs map[string]bool
	// valBinds is the unit's own dyn-binds of a PRODUCED fn value, by name:
	// the bound value's PRODUCER at the bind, the binding's generation and
	// the installed entry's identity. A `/v` read of the name is a fresh
	// wrap of the binding (ResolveRef mints a new Value over the dispatch
	// aggregate), so the read's ID carries no provenance of its own;
	// NoteValRead aliases it to that producer while the binding is still
	// the bind's — the generation unchanged, or the same entry on top again
	// after a frame binding of the name pushed and popped (`(cfst p/v)`
	// binds cfst's own `p` param). Any later bind of the name in this unit
	// drops the entry, and a bind inside a branch or loop body drops it for
	// good: the analysis rolls that bind back, so the entry on top after
	// the arm is the OLD one where the run may hold the NEW (the thirty-
	// first increment). The producer, not the value's ID: a memoised body
	// returns ONE residual value for every call of one shape, so the ID's
	// producer at read time is the LAST such call's (`def p (kk 7) end def
	// q (kk 8) end 3 p/v apply` read q's closure for p).
	valBinds map[string]valBind
	// enclosingBindNames snapshots, at unit open, the NAMES of every def binding
	// visible in the DefTable — the by-name twin of enclosingBindIDs. It exists
	// for the one case IDs cannot cover: a DETACHED stamp (StampDetachedFn) forks
	// the RUNTIME def table, where a value's ID was ELIDED (no compile-time mint),
	// so a module-scope `flex`/`store` read arrives with an empty ID that is
	// absent from enclosingBindIDs. dynScopeRescue recovers the read's NAME (from
	// DynFrom, tagged at the read site) and consults this set to confirm it is a
	// live enclosing binding — the VM's OpLookupDynScope reads the same live cell
	// the interpreter does (and DEFERS on a miss, a sound fallback), so a name
	// match is a safe commit signal for an otherwise un-ID-able mutable read.
	enclosingBindNames map[string]bool
	// pendingApply lists the value IDs of Function-typed CARRIERS this
	// unit's body dispatched through the `apply` word (a param/captured
	// comparator: `v comp/v apply`). The check engine cannot re-step a carrier
	// the way it re-steps a concrete fn, so the dispatch is elided and the
	// carrier flows to the body residual; StartFnCompile's finish must either
	// lower the ONE pending apply as the whole-residual OpCallDynTrailTop
	// (fn on top, args below — exactly the interpreter's applyHandler re-step
	// against the preceding stack) or refuse, so an unconsumed pending apply
	// can never silently compile the fn+args as unapplied data (Stage M2a,
	// design/STAGE3-INLINING-DESIGN-ROUND.0.md).
	pendingApply []pendingApply
}

// seqIdx addresses one producing-event result slot (seq, out index).
type seqIdx struct{ seq, idx int }

// pendingApply is one `apply`-word application the unit's finish or a paren
// collapse must lower: the applied value's id and the apply WORD's position,
// which the lowered op carries so a runtime no-match raises where the
// interpreter's `apply` dispatch does.
type pendingApply struct {
	id  string
	pos core.SrcPos
	// fn is the applied VALUE when it is a closure this pass produced
	// (recordCallElided's produced-closure arm): the check pass's re-step
	// dispatch asks for it by body (PendingClosureApply) to record the
	// apply over the closure's producer operand. Zero for a carrier lead.
	fn core.Value
}

// fnUnitRec is one compiled fn body awaiting (or holding) its
// lowered form. caps are the fn's construction-time captures: they
// ride as hidden trailing parameters (slots nParams…), so every call
// site re-supplies the captured values from ITS scope — inside the
// fn's own body a recursive call resolves them to the frame's own
// capture slots, which is exactly the snapshot semantics (captures
// are fixed at construction; the same values flow through).
// generic marks an instantiation of a gen fn — one unit per memoised
// instantiation (the key carries the instantiated arg types), kept
// OUT of tail marking, mirroring the interpreter's HasGen exclusion
// from frame elision (plan Stage 4).
type fnUnitRec struct {
	// stampOnly marks a unit that exists ONLY to back a fn value's compiled
	// ref (stampFnConst) — nothing in the program's code calls it. Its
	// lowering is therefore NOT allowed to refuse the program: a stamp is an
	// optimisation, and Finalize's per-unit lowering can decline a body the
	// unit compile accepted. On refusal the unit becomes a trap stub, its ref
	// is dropped, and the fn value interprets exactly as it did before
	// anything stamped it.
	stampOnly bool
	// render is the interpreter's formatFnDef string for a RETURNED-closure
	// unit (tryReturnedClosure) — stamped onto CompiledFn.Render and copied
	// to the ClosurePayload at OpPushClosure, so a compiled closure VALUE
	// renders byte-identically to the interpreter's fn value (interpolation
	// holes, print). Empty for every other unit (default rendering).
	render        string
	name          string
	nParams       int
	nUnnamed      int // unnamed (stack-flowing) params — the RET trim allowance
	caps          []core.CapturedBinding
	generic       bool
	returns       []*core.Type  // declared return types — enforced at the VM's RET
	paramTypes    []*core.Type  // declared PARAM types — enforced at the VM's CALL_USER entry
	paramPatterns []*core.Value // per-param structural/value patterns — also enforced at CALL_USER
	// returnPatterns are the per-return structural/value patterns
	// (FnSig.ReturnPatterns) — the RET-side twin of paramPatterns, enforced
	// at the VM's RET alongside returns.
	returnPatterns []*core.Value
	// decl is the return-contract declaration site (FnSig.Decl), stamped
	// on CompiledFn.Decl so a compiled RET return error labels the
	// declaration as a secondary span exactly as the interpreter does.
	// Zero for closures / anonymous units (no meaningful declaration).
	decl   core.DeclSite
	locals []string // slot→name table (params then captures)
	// reg is the fn's owning registry; stamped on CompiledFn.Reg when it
	// differs from the check registry (see emitUnit.reg).
	reg  *core.Registry
	frag *EmitFragment
	// storedRefUnit marks a unit compiled for a STORED-REF carrier
	// (compileStoredFnUnit's "storedfn" / compileStoredBody's "spawnbody"):
	// it is reachable at run time only through its CompiledFnRef, whose
	// module-dep rebinds are handled PRECISELY by per-ref poisoning
	// (NotifyNameRebound → poisoned → InvokeCallback falls to CallBoru).
	// NoteFrozenRead therefore skips reads attributed to it — the whole-
	// program frozen-read hammer is for ordinary CALL_USER units, which
	// have no per-unit fallback.
	storedRefUnit bool
	// bakes / frozen are the unit's baked enclosing-scope reads (unit_memo.go):
	// bakes maps each name to the binding's DefTable generation at the read —
	// the memo's staleness key — and frozen to WHAT was baked (the escaping
	// latch's refusal text). Nil until the first such read.
	bakes  map[string]int64
	frozen map[string]core.FrozenBake
	// variadic marks a VARIADIC-RETURNING fn: its body residual leaves a
	// runtime-variable count (a `[]`-declared recursive accumulator, an
	// `if c [] [a b]`). A call to it marks the call result variadic (lowerUserCall)
	// so only the program residual or another no-contract RET absorbs it. Set at
	// finish from the body residual's defining event (eventFlags.variadicResult).
	variadic bool
	// outOps are the body's residual operands in stack order (bottom→top):
	// the values the unit leaves for its caller. Empty for a 0-result body
	// OR a diverging body (every path tail-calls) — fragDiverges(frag)
	// distinguishes them (a diverging body emits no RET; a 0-result body
	// emits a bare RET).
	outOps []EmitOperand
	// dynTrailArity > 0 marks a body whose ENTIRE residual is a paren-bounded
	// TRAILING fn-value apply (`(prev key comp)`): outOps are [args…, fn] (fn on
	// top) and the unit lowering emits OpCallDynTrailTop(dynTrailArity) after seating
	// them, collapsing [args, fn] to the one applied value before the RET. The arity
	// is captured at the paren-collapse boundary (registerTrailingApply) where the
	// group size is known; the flattened residual cannot recover it.
	dynTrailArity int
	// dynTrailApply marks a dynTrail that came through the `apply` WORD (the
	// unit's pendingApply, Stage M2a) rather than a paren boundary: the unit
	// lowering emits OpCallDynApplyTop — applyHandler's unquote-then-apply —
	// instead of OpCallDynTrailTop (which leaves a Quoted fn as data, the
	// paren semantics).
	dynTrailApply bool
	// dynTrailPos is the apply WORD's position for a dynTrailApply tail —
	// the op is stamped there so a runtime no-match raises at the word.
	dynTrailPos core.SrcPos
	// dynFrameW > 0 marks a body whose residual carries an UNAPPLIED runtime fn
	// value beyond the frame-bottom re-push window — the shape the interpreter
	// resolves by execFnDefLiteral's runtime rule against the LIVE frame. The
	// unit lowering seats the FULL residual (the unnamed-param re-push prefix
	// included) and emits OpCallDynFrame(dynFrameW) before the RET: the top
	// dynFrameW entries are the token region the interpreter's pointer stepped,
	// replayed with the prefix resolved (see the opcode doc). Sets
	// CompiledFn.RetReplay so the RET applies the CallBoru-path trim discipline.
	dynFrameW int
	// dynFrameWords is CompiledFn.DynFrameWords' entry for this unit's
	// replay: the binding NAME under which each token-region entry was read
	// bare ("" otherwise), so the VM re-steps a fn-valued read as the WORD
	// the interpreter dispatches (NUR123). Nil when no entry is such a read.
	dynFrameWords []DynFrameWord
	// wordReads counts, per frame-local value ID, the BARE READS the engine
	// noted as word dispatches (NoteWordRead: a fn-typed or gradual local
	// read with no Function-expecting forward pending); wordReadNames is
	// the binding name each was read under. wordReadCredit counts the reads
	// a fn-value apply lowering consumed under value semantics (a paren
	// window, a trailing apply) — an ACCEPTED lowering. valReads counts the
	// `/v` reads of the same IDs. At the unit's finish every noted read must
	// be credited or seated in the replay window, and no ID may have been
	// read both ways, else the unit refuses (wordReadAccounting, NUR123).
	wordReads     map[string]int
	wordReadNames map[string]string
	wordReadPos   map[string]core.SrcPos // the LAST bare read's position, per ID
	wordReadFirst map[string]core.SrcPos // the FIRST bare read's position, per ID (a deopt begins there)
	// localReads is every bare read's position per value ID on this unit
	// (NoteLocalRead) — the deferred-operand accounting of planDeopts.
	localReads     map[string][]core.SrcPos
	wordReadCredit map[string]int
	valReads       map[string]int
	// outOpsVals are the residual VALUES outOps were resolved from, in the
	// same order — wordReadAccounting reads the replay window's IDs off it.
	outOpsVals []core.Value
	// retPrefix seats the unnamed-param frame-bottom re-pushes at UNIT START
	// (before the body events), for a body whose residual holds a variadic
	// apply-loop value region under an inert tail — the values must sit BELOW
	// the loop's runtime region, which only exists once the loop has run, so
	// they cannot be re-pushed by the RET seating. Paired with retReplay.
	retPrefix []EmitOperand
	// retReplay marks a body whose RET takes the replay trim discipline
	// (CompiledFn.RetReplay): a whole-frame dynamic apply (dynFrameW) or a
	// variadic apply-loop residual (retPrefix) leaves a runtime-variable
	// count the static contract cannot pin.
	retReplay bool
	numLoc    int
	pos       core.SrcPos
	finished  bool
	// inShape is the closure input convention recorded for a closure body unit
	// (ClosureInValue by default; ClosureInKeyVal for a map-iteration lambda).
	// Copied into CompiledFn.InShape at lowering. Zero (value) for user fns.
	inShape core.ClosureInShape
	// closure marks a higher-order body unit (each/scan/…$body) compiled via
	// compileClosureBody, as opposed to a genuine user fn. A return-count
	// mismatch in a closure body is the higher-order word's OWN runtime error
	// (each_error "body produced no result"), not the fn return-count
	// type_error — so a closure keeps refusing the mismatch (islands) while a
	// user fn compiles the error path (the VM RET raises the matching error).
	closure bool
	// lambdaUnit marks the fn-VALUE flavour of a closure unit (word "fnval"
	// — a returned lambda's body compiled via tryReturnedClosure), as
	// opposed to a native code-body unit (each/do$body, whose analysis
	// frame is the CallableSpec inputs). A lambda unit's frame is the
	// lambda's own declared named params — a real per-call frame like a
	// user fn's — so frame-context admissions currently gated off the
	// conflated `closure` flag (DynApplyLeadEligible's Stage-G lead apply,
	// noteDynFrameReplay) CAN in principle re-admit it. No consumer keys on
	// it yet: the §5.8 campaign's Stage-2 probe showed every candidate
	// witness blocked EARLIER (the chained `((b x) y)` apply's
	// event-provenance lead; capture reachability at the call site), so an
	// admission here would be unwitnessed and unproven — the flag lands as
	// the split's bookkeeping half, and each admission must bring its own
	// probe evidence (the Stage-G discipline).
	lambdaUnit bool
	// lambdaDeopt marks a lambda unit whose deopt points were planned from
	// its OWN frame (planDeopts' lambdaNamesSelfBound route, not
	// seedParentDeopt): emitDynParamBinds then binds its captures as well as
	// its params, so the island resolves every name it spells without the
	// factory frame being live.
	lambdaDeopt bool
	// promoted / dead are the value-def-local plan for THIS unit's body —
	// computed by planValueDefLocals at finish (while the unit is still live) and
	// read by the lowerer (flw.promoted / flw.dead) when the unit lowers. Mirrors
	// the top-level program's plan; nil for a unit with no promotions. A computed
	// result read across a body fragment (a `case` scrutinee re-tested down the
	// if-chain) is promoted to a frame slot here, the same as at frame 0.
	promoted map[int]int
	dead     map[int]bool
	// body is the fn's source body tokens (SetUnitBody); deopts the unit's
	// per-read deopt points (planDeopts), lowered by the unit's lowerer;
	// deoptEnv mirrors emitUnit.deoptEnv on the record.
	body       []core.Value
	deopts     []deoptPoint
	deoptEnv   bool
	deoptNames map[string]bool
	// rootFrame is the index in EmitState.frames of the unit's root frame
	// while it is open (a closure's finish reads its parent's events there
	// to seed the parent's binds — seedParentDeopt); deoptChildren are the
	// closure units whose islands read this unit's bindings, dropped with
	// their points when the unit cannot bind them after all.
	rootFrame     int
	deoptChildren []int
}

// deoptPoint is one gradual word read the unit lowers as a DEOPT
// (OpDeoptIfFn, NUR123): the read's value (its producing event's seq — a
// body-local's computed source), its name and read position, the position
// the statement holding the read begins at (start: where the test runs,
// before any of the statement's effects) and that statement's Body token.
type deoptPoint struct {
	seq   int
	slot  int // a CAPTURE's frame slot (the value's home in a closure unit); -1 for a producer (seq)
	name  string
	pos   core.SrcPos
	start core.SrcPos
	token int
	// atPush marks a point tested where the read's value is PUSHED as its
	// consumer's operand (the consumer word follows the read on the
	// stack: `a j add`, `j typeof`) — the compiled stack then holds
	// exactly what the interpreter's held at the read. Otherwise the test
	// runs before the first op of the statement (a forward consumer, a
	// literal, a branch, a residual read an event follows).
	atPush bool
	// restep marks a RE-STEP point (NUR124): tested right after the event
	// seq's op, over the results it left on top — the values the
	// interpreter splices back onto the tape and steps; token is where the
	// island resumes after them.
	restep bool
}

// plainLambda reports a lambda VALUE unit (tryReturnedClosure's `fnval`
// body) whose params carry no value PATTERN. Such a unit is a fn in every
// way that matters at its finish — its count contract is enforced at
// invoke (checkClosureReturn raises the interpreter's own `expected N return
// value(s)`), and a bare read of a captured fn is the word dispatch NUR123's
// whole-frame replay seats — so it takes the fn path's residual replay
// instead of the closure count refusal (the twenty-sixth increment: `( fn
// [[x:Integer][Integer][g x]] )` over a captured g). A PATTERN param is
// matched by the interpreter's frame binding at the apply, which the
// closure apply ops do not yet enforce (`((mk 5) 1)` over `[[0][Integer]
// [x]]` applied where the interpreter parks the pair), so a pattern lambda
// keeps the count refusal that was guarding it.
func (rec *fnUnitRec) plainLambda() bool {
	if !rec.lambdaUnit {
		return false
	}
	for _, p := range rec.paramPatterns {
		if p != nil {
			return false
		}
	}
	return true
}

// reStepNote is one NoteFnResultReStep on an event: the body position the
// interpreter resumes at after re-stepping the event's results, and whether
// a noted result is fn-TYPED (strict) — a gradual note only deopts, a strict
// one refuses where no deopt serves it (emitReStepAfter).
type reStepNote struct {
	resume core.SrcPos
	strict bool
}

// NewEmitState returns a fresh recording state.
func NewEmitState() *EmitState {
	return &EmitState{
		Compilable: true,
		SiteCounts: map[string]int{},
		frames:     [][]EmitEvent{nil},
		units:      []*emitUnit{{localByID: map[string]int{}}},
		fnUnits:    map[string]int{},
		producedBy: map[string]producer{},
		eventInfo:  map[int]eventFlags{},
		constIdx:   map[string]int{},
		typeIdx:    map[string]int{},
		origByID:   map[string]core.Value{},
	}
}

// residualForceOrder returns the fn-unit residual's promotion set when it is
// OUT OF ORDER — an event result above a non-event bottom — so the finish can
// force every residual event to a frame local (the per-unit mirror of
// Finalize's program-residual forceOrder) and the RET reconciliation re-pushes
// the whole residual in exact order. Returns nil for an in-order residual and
// for any residual carrying a fn value or dynamic value (the auto-apply
// boundary's territory: its stack layout is the apply's contract).
func residualForceOrder(ops []EmitOperand, vals []core.Value) map[int]bool {
	for _, v := range vals {
		if v.Dynamic || core.IsFnValueResidual(v) {
			return nil
		}
	}
	seenNonEvent, outOfOrder := false, false
	for _, op := range ops {
		if op.kind == opEvent && op.resIdx == 0 {
			if seenNonEvent {
				outOfOrder = true
			}
		} else {
			seenNonEvent = true
		}
	}
	if !outOfOrder {
		return nil
	}
	forceOrder := map[int]bool{}
	for _, op := range ops {
		if op.kind == opEvent && op.resIdx == 0 {
			forceOrder[op.idx] = true
		}
	}
	return forceOrder
}

// residualForceOrderFor picks the out-of-order promotion set for a fn-unit's
// finished residual. It is nil for the fn-value / trailing-apply shapes
// (dynTrail != 0) and for a residual whose operand count no longer matches
// its values (in-order by construction). Otherwise, an ARMED whole-frame
// replay (dynFrameW > 0) uses replayForceOrder (re-push the token region in
// exact order — the stylesheet-apply `[nd (rules get …)]` shape); every other
// residual uses residualForceOrder (the `do [x 1 add 2]` → [const-x, event]
// event-above-inert case). Extracted from the finish closure to keep
// StartFnCompile under the cyclomatic-complexity gate.
func (es *EmitState) residualForceOrderFor(dynTrail int, rec *fnUnitRec, ops []EmitOperand, vals []core.Value) map[int]bool {
	if dynTrail != 0 || len(ops) != len(vals) {
		return nil
	}
	if rec.dynFrameW > 0 {
		return es.replayForceOrder(ops)
	}
	return residualForceOrder(ops, vals)
}

// replayForceOrder returns the promotion set that re-pushes an ARMED
// whole-frame replay residual (dynFrameW > 0) in exact source order. The
// token-region layout IS the replay's contract — the values must sit on the
// stack in the order the interpreter's pointer stepped them (args below, the
// fn value at its own step position) — so an out-of-order residual (a
// dyn-event result lowered before the inert operand that must sit under it,
// the `[nd (rules get …)]` stylesheet-apply shape) promotes every residual
// event to a frame local exactly as residualForceOrder does for plain
// residuals. The fn/dynamic bail there protects an UNARMED residual from
// compiling an apply away as data; here OpCallDynFrame owns the apply, so
// ordering the operands is precisely what makes the replay faithful.
// Declines (nil — an out-of-order residual then keeps the seating refusal,
// the sound interpreter fallback) when an event result is VARIADIC (a
// runtime-variable count cannot store to one slot) or a multi-result event
// participates (promotion re-pushes single results only).
func (es *EmitState) replayForceOrder(ops []EmitOperand) map[int]bool {
	seenNonEvent, outOfOrder := false, false
	for _, op := range ops {
		if op.kind == opEvent {
			if op.resIdx != 0 || es.eventInfo[op.idx].variadicResult {
				return nil
			}
			if seenNonEvent {
				outOfOrder = true
			}
		} else {
			seenNonEvent = true
		}
	}
	if !outOfOrder {
		return nil
	}
	forceOrder := map[int]bool{}
	for _, op := range ops {
		if op.kind == opEvent {
			forceOrder[op.idx] = true
		}
	}
	return forceOrder
}

// forkForProbe returns a throwaway recording state for compiling a closure body
// speculatively (recordClosureDispatch's probe), seeded so a RECURSIVE call
// inside the body resolves to the enclosing in-progress unit. The closure body's
// emission (events / code / producedBy / consts) is FRESH — a refusal discards it
// without touching the real program. But the fn-unit resolution tables
// (fnUnits / fnRecs / units) are COPIED from the real state: a self-recursive call
// (`… msd-go` inside an `each` body) is then a fnUnits HIT against the enclosing
// fn's reserved unit — the same recursion guard the in-state non-closure path
// relies on — instead of a MISS that re-compiles the enclosing fn in the
// throwaway, where it re-hits its own closure and never registers its residual
// ("body result of unknown provenance"). The slices get fresh backing arrays so
// the probe's newly-appended units never write into the real state; the shared
// *fnUnitRec / *emitUnit pointers are only READ on the hit path (StartFnCompile
// returns finish==nil, so no re-analysis mutates them).
func (es *EmitState) forkForProbe() *EmitState {
	p := NewEmitState()
	p.reg = es.reg
	p.fnUnits = make(map[string]int, len(es.fnUnits))
	for k, v := range es.fnUnits {
		p.fnUnits[k] = v
	}
	p.fnRecs = make([]*fnUnitRec, len(es.fnRecs))
	copy(p.fnRecs, es.fnRecs)
	p.units = make([]*emitUnit, len(es.units))
	copy(p.units, es.units)
	p.openUnitRecs = make([]int, len(es.openUnitRecs))
	copy(p.openUnitRecs, es.openUnitRecs)
	// The probe inherits the gradual-nesting mode: probe and real must
	// compile under the SAME modality or the probe's verdict is about a
	// different unit (see compileStoredFnUnit / tryReturnedClosure). Same
	// for the environment mode (dynEnv).
	p.storedGradualDepth = es.storedGradualDepth
	p.inStampCompile = es.inStampCompile
	p.dynEnv = es.dynEnv
	// The arm-resident bracket too: probe and real must admit the same
	// def-event population (`_`-named body defs) or the probe's verdict is
	// about a different unit.
	p.armResidentDepth = es.armResidentDepth
	// The residual-order hazard tables too (unit_memo.go): a read the real
	// state saw in an enclosing fragment must count against a bind the
	// probe records, or the probe admits a residual the real compile then
	// refuses program-wide instead of declining to the island. The open
	// fragment ids ride along so the probe marks the same frames; its own
	// fragments continue the counter past the real state's.
	p.fragSeq = es.fragSeq
	p.fragIDs = append([]int(nil), es.fragIDs...)
	p.fragReads = make(map[readKey]bool, len(es.fragReads))
	for k, v := range es.fragReads {
		p.fragReads[k] = v
	}
	p.bindHazard = make(map[readKey]bool, len(es.bindHazard))
	for k, v := range es.bindHazard {
		p.bindHazard[k] = v
	}
	p.storeHazard = make(map[slotKey]bool, len(es.storeHazard))
	for k, v := range es.storeHazard {
		p.storeHazard[k] = v
	}
	return p
}

// inClosureUnit reports whether the innermost OPEN fn unit is a CLOSURE body
// (a higher-order word's each/do$body unit — compileClosureBody stamps
// rec.closure before running the body analysis, so the flag is visible to
// every dispatch the body records). Body dispatches with frame-context
// semantics consult this: the `args` projection (specialWordResults) reads
// the analysis args frame, which for a closure holds the CallableSpec inputs
// — NOT the enclosing fn's per-call args the interpreter reads when the body
// runs through InvokeBody — so the projection must decline and let the
// dispatch fall to RecordCall's context-dependent-word refusal.
// storedGradualActive reports a DETACHED stamp compile in progress
// (storedGradualDepth > 0) — the interface probe for the stored-context
// gradual-generalisation rule (core_helpers.go), Stage-0b promotion.
func (es *EmitState) StoredGradualActive() bool {
	return es != nil && es.storedGradualDepth > 0
}

func (es *EmitState) InClosureUnit() bool {
	if es == nil || len(es.openUnitRecs) == 0 {
		return false
	}
	rec := es.openUnitRecs[len(es.openUnitRecs)-1]
	if rec < 0 || rec >= len(es.fnRecs) {
		return false
	}
	return es.fnRecs[rec].closure
}

func (es *EmitState) Active() bool {
	return es != nil && es.Compilable && es.suspended == 0
}

// PushInlineCtxBoundary opens an inline context-boundary region: a check-run
// region whose runtime twin is a fresh sub-engine (a context-layer push) but
// whose compiled lowering is inline in the enclosing unit. The open-unit
// depth is latched so units opened inside the region un-mark it while their
// bodies record — the VM brackets those with its own context frame. NUR054.
func (es *EmitState) PushInlineCtxBoundary() {
	if es == nil {
		return
	}
	es.inlineCtxBounds = append(es.inlineCtxBounds, len(es.openUnitRecs))
}

// PopInlineCtxBoundary closes the innermost inline context-boundary region.
func (es *EmitState) PopInlineCtxBoundary() {
	if es == nil || len(es.inlineCtxBounds) == 0 {
		return
	}
	es.inlineCtxBounds = es.inlineCtxBounds[:len(es.inlineCtxBounds)-1]
}

// InInlineCtxBoundary reports whether recording currently sits INLINE inside
// an inline context-boundary region: a region is open, and no closure/fn
// unit has been opened since its entry.
func (es *EmitState) InInlineCtxBoundary() bool {
	if es == nil || len(es.inlineCtxBounds) == 0 {
		return false
	}
	return es.inlineCtxBounds[len(es.inlineCtxBounds)-1] == len(es.openUnitRecs)
}

// Active is the exported view of active() for native handlers that
// must mirror the bytecode-recording state — e.g. a 0-output `if`
// statement guard only puts its phantom None on the carrier stack
// while recording is live (the lowering tracks it); a plain or
// uncompilable check must net 0, like the runtime.

// armed reports that a REAL recording state exists (a compile pass installed
// one), whether or not it is currently live — the EmitRecorder twin of the
// historical `Check.Emit != nil` probe (an EmitState may be armed yet
// suspended or already uncompilable; the inactive no-op recorder is never
// armed).
func (es *EmitState) Armed() bool { return es != nil }

// BindRegistry installs the registry back-pointer used by returned-closure
// compilation (tryReturnedClosure) and the flex-hook sig-identity proof.
func (es *EmitState) BindRegistry(r *core.Registry) {
	if es == nil {
		return
	}
	// The FIRST bind names the program registry: the outermost check Run binds
	// the top-level registry before any module-body / island sub-engine run
	// re-binds reg to a foreign sub-registry. Captured once, never re-bound.
	if es.progReg == nil {
		es.progReg = r
		es.bindSnap = r.SnapshotBindings()
	}
	es.reg = r
}

// TopFrameOnly reports whether recording sits at the top event frame (no
// open branch/loop/fn capture) — the const-fold gate for computed container
// elements. A missing recorder counts as top-frame (nothing is being
// captured), matching the historical `es == nil || len(es.frames) == 1`.
func (es *EmitState) TopFrameOnly() bool {
	return es == nil || len(es.frames) == 1
}

// suspendedNow reports whether an ARMED recorder is currently suspended —
// the "analysis probe is reading an enclosing dispatch's result" state the
// unmatched-dispatch recovery consults before latching a refusal.
func (es *EmitState) SuspendedNow() bool {
	return es != nil && es.suspended > 0
}

// Sites returns the per-site-class dispatch tally (live map; read-only for
// callers). Nil when recording never started.
func (es *EmitState) Sites() map[string]int {
	if es == nil {
		return nil
	}
	return es.SiteCounts
}

// zeroOutProduced reports whether the value id was produced by an event
// whose outputs were zeroed (the 0-output `if` phantom None) — the residual
// strip probe (stripZeroOutResiduals).
func (es *EmitState) ZeroOutProduced(id string) bool {
	if es == nil {
		return false
	}
	pr, ok := es.producedBy[id]
	return ok && es.eventInfo[pr.seq].zeroOut
}

// alreadyProduced reports whether the value id already has a recorded
// producer event — the double-record guard (a structured ReturnsFn hook may
// have recorded the dispatch before the generic path sees it).
func (es *EmitState) AlreadyProduced(id string) bool {
	if es == nil {
		return false
	}
	_, ok := es.producedBy[id]
	return ok
}

// unitVariadic reports whether the fn unit's recorded body is
// variadic-returning — the user-poly gate (a poly call site bakes a fixed
// nout, so no arm may be variadic).
func (es *EmitState) UnitVariadic(unit int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	return es.fnRecs[unit].variadic
}

// unitNetsZero reports whether the fn unit's recorded body nets ZERO
// residual values — the zero-return user-poly gate (REFUSAL-CLOSURE.0
// §6a): a zero-declared-return arm set records a 0-output poly call, so
// whichever arm the VM's runtime re-match selects must leave nothing on
// the caller's stack. A declared-[] body with a non-empty residual is the
// interpreter's "residual IS the result" shape — the fixed nout cannot
// carry it, so the set keeps its refusal. A diverging trailing event never
// returns at all, so its empty outOps qualify (the 0-out accounting is
// never consulted on a raise/tail-out path).
// UnitTailApply reports the width of the window the unit's finish lowered as
// the fn-value apply at its body tail (dynTrailArity — the pending
// apply-word form, OpCallDynApplyTop, or a trailing fn value's own): the top
// n residual values and the fn above them net ONE result at run time. The
// check pass's call-site residual still holds the window (`x f/v apply` over
// a captured fn leaves [result, f] — the identity carrier the elided apply
// returned), so it must collapse the same way or a call site records a
// two-value dispatch over a one-value unit.
func (es *EmitState) UnitTailApply(unit int) (int, bool) {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return 0, false
	}
	if rec := es.fnRecs[unit]; rec.dynTrailArity > 0 {
		return rec.dynTrailArity, true
	}
	return 0, false
}

func (es *EmitState) UnitNetsZero(unit int) bool {
	if es == nil || unit < 0 || unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[unit]
	return !rec.variadic && len(rec.outOps) == 0
}

// newIsolatedEmit returns the FRESH EmitState IsolateEmit swaps in for a
// hermetic throwaway evaluation, inheriting only the registry back-pointer
// from the saved recorder. Construction knowledge stays in this file: the
// checker side (CheckState.IsolateEmit) sees only EmitRecorder values.
func newIsolatedEmit(saved core.EmitRecorder) core.EmitRecorder {
	fresh := NewEmitState()
	if prev, ok := saved.(*EmitState); ok && prev != nil {
		fresh.reg = prev.reg
	}
	return fresh
}

// RegisterTrailingApply records that the Function VALUE `fnID` is the trailing
// fn-value of a paren-bounded apply over `arity` preceding args (`(prev key comp)`),
// captured at the paren-collapse boundary where the group size is known. The body
// reconciliation reads it back (TrailingApplyArity) to lower the apply to
// OpCallDynTrailTop — the flattened residual cannot otherwise recover the arity.
func (es *EmitState) RegisterTrailingApply(fnID string, arity int) {
	if es == nil || fnID == "" || arity < 1 {
		return
	}
	if es.trailingApplies == nil {
		es.trailingApplies = map[string]int{}
	}
	es.trailingApplies[fnID] = arity
	es.creditWordRead(fnID)
}

// TrailingApplyArity returns the registered arg count for a paren-bounded trailing
// apply whose fn value is `fnID`, or 0 if none registered.
func (es *EmitState) TrailingApplyArity(fnID string) int {
	if es == nil || es.trailingApplies == nil {
		return 0
	}
	return es.trailingApplies[fnID]
}

// SetCatchVariadic latches the next catch-word dispatch's recorded result
// as variadic (see the EmitRecorder doc; consumed by catchVariadicFor).
func (es *EmitState) SetCatchVariadic(pending bool) {
	if es == nil {
		return
	}
	es.catchVariadicPending = pending
}

// catchVariadicFor consumes the catch-variadic latch for a
// CompileFallbackBody dispatch: true exactly once, for the dispatch whose
// ReturnsFn set it (the fallible multi-value `do` body — its runtime count
// is N on no-raise but 1 on the caught path, so the recorded event must be
// variadic rather than seated at the static N). The variadic mark covers
// only that SHRINKING direction — a count that can EXCEED the modeled
// seats (await's winner-takes-all first/any) has no event-level
// representation and refuses wholesale instead (awaitVariadicResult's
// MarkUncompilable, NUR067).
func (es *EmitState) catchVariadicFor(sig *core.Signature) bool {
	if es == nil || !es.catchVariadicPending || sig == nil ||
		!sig.CompileEffect.Has(core.CompileFallbackBody) {
		return false
	}
	es.catchVariadicPending = false
	return true
}

// MarkUncompilable latches the program uncompilable, keeping the
// FIRST reason (later marks are consequences of the first).
func (es *EmitState) MarkUncompilable(reason string) {
	if es == nil || !es.Compilable {
		return
	}
	if es.trapAt != 0 {
		// A terminal top-level trap already ends the program here. Traps are
		// recorded in execution order and only while active() (Compilable
		// true), so any construct that marks uncompilable AFTER trapAt is set
		// is at-or-after the trap and therefore unreachable — the interpreter
		// raises at the trap and never reaches it. Finalize truncates to the
		// trap and drops the residual, so its refusal is moot (e.g. `getr` of a
		// missing module-namespace key: the getr IS the trap, and its own
		// operand-provenance residual must not refuse the program).
		return
	}
	es.Compilable = false
	es.Reason = reason
}

// Suspend pauses recording for the duration of a nested body
// analysis (branch bodies, fn bodies, higher-order bodies run their
// own sub-engines over the shared registry; their dispatches are not
// part of THIS program's straight line). Returns the resume func.
func (es *EmitState) Suspend() func() {
	if es == nil {
		return func() {}
	}
	es.suspended++
	return func() { es.suspended-- }
}

// ArmBranchCapture makes the NEXT RunCarrierBodyWithDefs record its
// body into a fragment instead of suspending — the branch-lowering
// hook (`if`). One-shot; consumed by the body run.
func (es *EmitState) ArmBranchCapture() {
	if !es.Active() {
		return
	}
	es.captureArm = true
}

// consumeCaptureArm reports and clears the one-shot capture flag.
func (es *EmitState) consumeCaptureArm() bool {
	if es == nil || !es.captureArm {
		return false
	}
	es.captureArm = false
	return es.Active()
}

// peekCaptureArm reports whether the next RunCarrierBodyWithDefs will
// record its body into a fragment (branch/loop lowering armed and the
// recorder live) WITHOUT clearing the one-shot flag — the same condition
// consumeCaptureArm settles. RunCarrierBodyWithDefs consults it to mark
// the branch/loop sub-engine element-eval-recordable, so a residual
// computed container (`{a: x}` / `[x y]` returned from a branch arm) has
// its OpMakeMap / OpMakeList assembly recorded rather than left an
// unresolvable residual — the body runs in the LIVE frame here (unlike
// the end-of-run residual eval), so recording is sound.
func (es *EmitState) PeekCaptureArm() bool {
	return es != nil && es.captureArm && es.Active()
}

// beginFragment opens a recording frame; the returned func closes it
// into es.captured.
func (es *EmitState) beginFragment() func() {
	es.frames = append(es.frames, nil)
	es.fragFloors = append(es.fragFloors, es.seq)
	// A monotone fragment identity (unit_memo.go's residual-order hazard
	// keys on it): two sibling arms can share a start seq when no event
	// separates them, so the seq is not an identity.
	es.fragSeq++
	es.fragIDs = append(es.fragIDs, es.fragSeq)
	return func() {
		n := len(es.frames) - 1
		es.captured = &EmitFragment{
			events:   es.frames[n],
			startSeq: es.fragFloors[len(es.fragFloors)-1],
			id:       es.fragIDs[len(es.fragIDs)-1],
		}
		es.frames = es.frames[:n]
		es.fragFloors = es.fragFloors[:len(es.fragFloors)-1]
		es.fragIDs = es.fragIDs[:len(es.fragIDs)-1]
	}
}

// bodyAnalysisGuard is called by RunCarrierBodyWithDefs: capture a
// fragment when armed, otherwise suspend recording for the nested
// body. Inside an open keep-defs bracket (a `do` body run), the
// suspended sub-run's bind twins are recorded as a TAINTED range —
// a non-keep body (branch / loop / quotation) is conditional or
// multi-run, so a twin noted under it must never be adopted as a
// single replay (see keepTaints). Nil-safe.
func (es *EmitState) BodyAnalysisGuard() func() {
	if es.consumeCaptureArm() {
		return es.beginFragment()
	}
	if es == nil || es.keepBodyDepth == 0 {
		return es.Suspend()
	}
	start := len(es.bindTwins)
	resume := es.Suspend()
	return func() {
		if end := len(es.bindTwins); end > start {
			es.keepTaints = append(es.keepTaints, [2]int{start, end})
		}
		resume()
	}
}

// KeepDefsBodyGuard is BodyAnalysisGuard's keep-defs flavor (`do`'s
// check-mode body run, runCarrierBodyDefsAdds keep=true): the same
// suspension, plus the adoption bracket — the OUTERMOST keep run's twin
// range and its tainted sub-ranges are published on close for the
// dispatch record that follows (AdoptBodyTwins). Fragment capture never
// arms for a do body (ArmBranchCapture is the `if` hook), so this guard
// does not consult the capture arm. Nil-safe.
func (es *EmitState) KeepDefsBodyGuard(r *core.Registry, bodyID string) func() {
	if es == nil {
		return func() {}
	}
	// The body's START state, for the compile re-run that follows the run
	// (unit_memo.go, the body re-run environment). Cloned at open — before
	// the run leaks anything — and published at close, at any scope: a `do`
	// inside a fn body leaks into that frame and its re-run needs the start
	// just the same. Only a run whose close finds the recorder ACTIVE
	// publishes (publishBodyStart): a run nested inside another suspended
	// run records nothing, and its body is re-run — guard and all — during
	// the outer body's own compile.
	var start *core.DefTable
	if r != nil && bodyID != "" && r.Defs != nil {
		start = r.Defs.Clone()
	}
	// THE PUBLICATION GATE, exactly MultiRunBodyGuard's and for the same
	// reason. A keep run at FnBodyDepth > 0 can note NO twin
	// (NoteBindTransitionEntry suppresses fn-body transitions — they are
	// frame-locals, not registry bindings), so its bracket is empty by
	// construction and it has nothing to publish. Publishing it anyway is
	// what loses the OUTER do's bracket: tryRecordClosure's body compile
	// RE-RUNS the do body, and any `do` inside a fn the body CALLS opens
	// its own keep bracket during that re-run and overwrites the one the
	// analysis phase had set correctly. Measured 2026-09-02 on
	// `do [import "boru:sift" (Sift.parse kv/q {} "a: 1")]`: the published
	// range came back [1 1] — empty, floor already past the import's own
	// `Sift` twin — so AdoptBodyTwins had nothing to walk and the program
	// refused for want of a placement it could have made. Its neighbours
	// `do [import "boru:sift" Sift.parse]` (no call) and
	// `do [import "boru:sift" (Sift.kinds)]` (a call that reaches no `do`)
	// compiled throughout, which is what isolated the cause.
	//
	// A nil registry publishes nothing, like the multi-run gate.
	publish := r != nil && r.Check != nil && r.Check.FnBodyDepth == 0
	// keepModuleDepth counts only the keep runs whose defs reach MODULE
	// scope, which is the same FnBodyDepth == 0 test the publication gate
	// above makes — a `do` inside a fn body binds frame-locals, and a rebind
	// written there must not refuse. NotifyNameRebound reads it to tell a
	// suspension that leaks from one that does not (NUR117).
	if publish {
		es.keepModuleDepth++
	}
	es.keepBodyDepth++
	if es.keepBodyDepth == 1 {
		es.keepTwinFloor = len(es.bindTwins)
		es.keepTaints = nil
	}
	resume := es.Suspend()
	return func() {
		resume()
		es.keepBodyDepth--
		if publish {
			es.keepModuleDepth--
		}
		if es.keepBodyDepth == 0 && publish {
			es.lastKeepRange = [2]int{es.keepTwinFloor, len(es.bindTwins)}
			es.lastKeepTaints = es.keepTaints
			es.keepTaints = nil
		}
		es.publishBodyStart(bodyID, r, start, false)
	}
}

// multiRunLatch is MultiRunBodyGuard's close-time record — see the
// lastMultiRun field doc.
type multiRunLatch struct {
	bodyID   string
	from, to int
	reg      *core.Registry
	// evSeq is the event counter at the moment this latch closed. It is the
	// FIXED-POINT IDENTITY the superseding rule needs: body-ID adjacency
	// alone cannot tell consecutive rounds of one analysis from two separate
	// dispatches that happen to share a body value.
	evSeq int
}

// MultiRunBodyGuard is BodyAnalysisGuard for a higher-order body analysis
// run (the interface doc in core/go/emit_recorder.go): the same
// suspension and keep-bracket taint — delegation, so the #421 fences keep
// working unchanged — plus the arm-residency latch published on close.
// Nil-safe.
func (es *EmitState) MultiRunBodyGuard(r *core.Registry, bodyID string) func() {
	if es == nil {
		return func() {}
	}
	// THE PUBLICATION GATE. A body run at FnBodyDepth > 0 has nothing to
	// say about the latch, and saying it anyway is what declines every
	// nested multi-run shape. NoteBindTransitionEntry suppresses on
	// FnBodyDepth > 0 (fn-body defs are frame-locals, not registry
	// bindings), so such a run records NO twin and its range is empty by
	// construction — the condition is exact, not a heuristic. But
	// tryRecordClosure's body COMPILE re-runs the outer body inside
	// AnalyseFnBody, and every nested higher-order ReturnsFn fires again
	// there: without this gate the nested word's guard close OVERWRITES the
	// slot the analysis phase left correct, and AdoptResidentTwins then
	// fails its identity fence and adopts nothing.
	//
	// The clobber is not about what the nested body BINDS — measured
	// 2026-09-02, `fold [ var [[a b] ([1] each [add 1]) (a add b)] ] [1 2] 0`
	// refused although the nested body binds nothing at all, and the same
	// shape with `filter` (which does not route through
	// analyseHigherOrderBodyVals) compiled. Every caller of that function
	// writes this latch, flagged word or not; only the flagged few read it.
	//
	// A nil registry publishes nothing: the guard is called with one in the
	// recorder's own settled tests.
	publish := r != nil && r.Check != nil && r.Check.FnBodyDepth == 0
	// Counted for the same reason KeepDefsBodyGuard counts its own: a
	// multi-run body at MODULE scope leaks its last iteration's def to the
	// enclosing scope, so a rebind written there is one the frozen-read latch
	// has to see. Measured — `def k 5  def f fn [[] [Integer] [k add 2]]  f
	// [1] each [def k 9  k]  f` answers `7 [9] 11` interpreted and `7 [9] 7`
	// compiled. NUR117 recorded this arm as already covered by the
	// arm-resident machinery; `armBoundNames` only refuses a later TOP-LEVEL
	// READ of the name, and the row above reaches it through a CALL instead.
	if publish {
		es.multiRunModuleDepth++
	}
	// The body's START state for the compile re-run (unit_memo.go), cloned
	// before the run leaks its last iteration's defs. Published at close
	// whatever the publication gate says — a multi-run body inside a fn
	// body leaks into that frame, and its re-run needs the start too — but
	// a later ROUND of one fixed point keeps the FIRST round's start: the
	// later round began from the previous round's leak, not from the
	// body's start.
	var start *core.DefTable
	if r != nil && bodyID != "" && r.Defs != nil {
		start = r.Defs.Clone()
	}
	from := len(es.bindTwins)
	inner := es.BodyAnalysisGuard()
	return func() {
		inner()
		if publish {
			es.multiRunModuleDepth--
		}
		laterRound := es.lastMultiRun.bodyID == bodyID && es.lastMultiRun.evSeq == es.seq
		es.publishBodyStart(bodyID, r, start, laterRound)
		if !publish {
			return
		}
		// A SUPERSEDED ROUND's twins are speculative, not real. Some
		// higher-order analyses re-run the same body to a fixed point (fold's
		// accumulator widens between rounds — analyseHigherOrderBodyVals'
		// contract), and every round records the body's transitions afresh.
		// Only the LAST round describes the unit the bridge pairs against, so
		// the earlier rounds' entries are artifacts of the analysis, the same
		// category the ledger already excludes for a rolled-back body run.
		// Mark them exempt from the placement gate rather than demanding an op
		// no lowering will ever emit: at runtime only the surviving round's ops
		// execute, so the dead rows install nothing.
		//
		// The identity has to be the fixed-point ANALYSIS, not the body value:
		// two separate dispatches can share one body (a memoized unit, a body
		// read from a binding), and both perform their installs at runtime, so
		// superseding the first would silently drop them — the exact hazard
		// this increment exists to remove. Rounds of one fixed point run
		// back-to-back with recording SUSPENDED, so no event is appended
		// between them; a second dispatch necessarily records its own call
		// event first. An unmoved event counter is therefore the precise
		// witness, and body-ID adjacency alone is not.
		sameFixedPoint := es.lastMultiRun.bodyID == bodyID &&
			es.lastMultiRun.evSeq == es.seq && es.lastMultiRun.to <= from
		if sameFixedPoint {
			for i := es.lastMultiRun.from; i < es.lastMultiRun.to; i++ {
				if es.supersededTwins == nil {
					es.supersededTwins = map[int]bool{}
				}
				es.supersededTwins[i] = true
			}
		}
		es.lastMultiRun = multiRunLatch{bodyID: bodyID, from: from, to: len(es.bindTwins), reg: r, evSeq: es.seq}
	}
}

// RecordDynUndef notes an `undef`-shaped teardown at its stream position
// (the interface doc in core/go/emit_recorder.go) — today only inside an
// arm-resident body compile (armResidentDepth, the each-unit bracket),
// where the var-param pair's undef half needs its event seat so the
// bridge can pair the BindUndef twin and the unit can tear the binding
// down per element. Everywhere else this records nothing: no other
// consumer exists, and the event's absence keeps every other lane's
// event streams untouched. Nil-safe.
func (es *EmitState) RecordDynUndef(name string, pos core.SrcPos) {
	if es == nil || !es.Active() || es.armResidentDepth == 0 || name == "" {
		return
	}
	es.appendEvent(EmitEvent{kind: evDynBind, dyn: &emitDynBind{
		name: name, srcSeq: -1, pos: pos, residentTwin: -1, undef: true,
	}})
	es.noteBindHazard(name)
}

// RecordTypeInstall notes a TYPE binding's push at its stream position
// (the interface doc in core/go/emit_recorder.go) — like RecordDynUndef,
// today only inside an arm-resident body compile (armResidentDepth, the
// each-unit bracket), where the BindTypeInstall twin needs a def-site
// event for the bridge to pair against and the unit needs an op to
// install the node per element.
//
// Everywhere else this records NOTHING, and that is the whole reason a
// type install could be given an event at all without disturbing any
// other lane: outside the bracket a type binding is a purely check-time
// product — the mint happens once, the TOP-LEVEL twin replays it at its
// own stream position (OpBindTwin), and the compiled stream carries no
// instruction for it. Nil-safe.
func (es *EmitState) RecordTypeInstall(name string, pos core.SrcPos) {
	if es == nil || !es.Active() || es.armResidentDepth == 0 || name == "" {
		return
	}
	es.appendEvent(EmitEvent{kind: evDynBind, dyn: &emitDynBind{
		name: name, srcSeq: -1, pos: pos, residentTwin: -1, typeInstall: true,
	}})
	es.noteBindHazard(name)
}

// TakeFragment returns the last captured fragment (nil when the
// capture never armed — plain check runs, suspended recordings).
func (es *EmitState) TakeFragment() core.EmitFragmentRef {
	if es == nil {
		return nil
	}
	f := es.captured
	es.captured = nil
	return f
}

// appendEvent adds an event to the current frame and returns its seq.
// lastUserPolyNote identifies the just-recorded OpCallUserPoly event for
// recordCallElided's poly-alias arm (see the EmitState.lastUserPoly field).
type lastUserPolyNote struct {
	word string
	seq  int
	nout int
}

func (es *EmitState) appendEvent(ev EmitEvent) int {
	es.lastUserPoly = nil
	n := len(es.frames) - 1
	// Dead code after a divergent terminal, IN A FRAGMENT: a diverging call
	// (raise, or a static-zero div/mod — CompileValueDiverges) never returns, so
	// any event recorded after it in the same CLOSURE/BRANCH fragment is
	// unreachable. Emitting it builds a malformed frame — an op wired to the
	// divergent call's absent result crashes the VM (`do [(raise "x") add 1]`,
	// `do [(0 mod 0) mod 0]` both did). DROP it: the fragment already diverges
	// (fragDiverges sees the divergent call as its terminal, so the closure body
	// compiles with no RET and the catching word wraps the raised error), exactly
	// as the interpreter stops at the raise. The dropped event still consumes a
	// seq so a caller registering its (dead) output writes a harmless slot Finalize
	// never linearises. Gated to n>0 (a pushed fragment frame): at the TOP LEVEL
	// the divergence is the program terminator — Finalize truncates the residual
	// after it (`0 and (20 div 0)` raises and aborts, matching the interpreter),
	// so top-level events past it stay put.
	if n > 0 && len(es.frames[n]) > 0 {
		if prev := es.frames[n][len(es.frames[n])-1]; prev.kind == evCall && prev.call.diverges {
			es.seq++
			return es.seq
		}
	}
	es.seq++
	ev.seq = es.seq
	es.frames[n] = append(es.frames[n], ev)
	return ev.seq
}

// setProduced registers an event's output ID against its sequence,
// recording whether that output is itself a type body (a bare type
// node, or a structural/nominal type literal like an ObjectType). The
// flag lets resolveOperand tell a real type-producing event (typeof,
// type algebra) apart from an ID COLLISION — a `make` result is an
// ObjectInstance that can inherit the type literal's ID, so a later
// type operand (`(make Point {}) is Point`) would otherwise resolve
// the `Point` literal to the make event.
func (es *EmitState) setProduced(out core.Value, seq int) {
	es.setProducedAt(out, seq, 0)
}

// setProducedAt is setProduced for the idx-th result of a multi-result event
// (P5). idx 0 is the single-result case. When two outputs share an ID — a
// stack word like `dup` returns `[args[0], args[0]]`, both the same Value —
// the LAST registration wins; the lowerer's operand layout then refuses the
// ambiguous consume (sound: the program falls back) until carrier-identity
// (the next runtime-independence item) mints distinct ids.
func (es *EmitState) setProducedAt(out core.Value, seq, idx int) {
	// An identity-less value (a runtime mint under the mode-gated ID
	// elision — value.go checkPassDepth) must NEVER key the provenance
	// map: a "" insert would make every later ""-ID lookup a false hit
	// (all identity-less values would alias one producer) — an active
	// miscompile. Skipping keeps "" lookups guaranteed misses, which
	// degrade to dynScopeRescue / refusal.
	if out.ID == "" {
		return
	}
	es.producedBy[out.ID] = producer{seq: seq, idx: idx}
	if core.IsTypeBody(out) {
		f := es.eventInfo[seq]
		f.typeOut = true
		es.eventInfo[seq] = f
	}
}

// MarkValueDef records that value v is bound to a NAMED `def x (expr)` binding,
// so the lowerer promotes v's producing event to a frame LOCAL (STORE_LOCAL +
// PUSH_LOCAL per reference) instead of leaving it on the simulated stack. A
// named binding may be consumed in ANY order — `def a (make …) def b (make …)
// a.x … b.x` uses a (produced first) before b (on top), which the single-
// consume stack discipline cannot seat — whereas a local re-pushes freely. Only
// a single-result producing event qualifies (a multi-result `dup` needs the
// carrier-identity path); a const / local / unproduced value is a no-op.
// Nil-receiver-safe; called from the def handler's install choke point.
func (es *EmitState) MarkValueDef(v core.Value) {
	if es == nil || !es.Compilable {
		return
	}
	if pr, ok := es.producedBy[v.ID]; ok && pr.idx == 0 {
		f := es.eventInfo[pr.seq]
		f.valueDef = true
		es.eventInfo[pr.seq] = f
	}
}

// pendingLoopBind carries a SplitLoopRegionBind verdict to the immediately
// following RecordDynBind (both fire inside one installAndRecordDef call —
// the def word's handler): the bound name takes the loop's FIRST value via
// the splice-at-depth OpBindGlobal (REFUSAL-CLOSURE S5).
type pendingLoopBind struct {
	seq   int // the producing loop event
	depth int // values above the first at bind time = regionN-1
}

// SplitLoopRegionBind implements the check-mode half of REFUSAL-CLOSURE S5:
// a TOP-LEVEL def whose value is a STATICALLY-COUNTED variadic loop region
// binds the region's FIRST value (the interpreter's pending-forward collects
// the first-arrived value; the rest spill as residual — probe-pinned:
// `def xs (for 3 [1]) xs` yields [1 1 1] with xs=1). Returns the ELEMENT
// carrier the binding should take; the caller keeps the region carrier
// itself on the check stack as the N-1 REST residual (still produced by the
// loop event, so the existing variadic disposition owns it). Declines
// (ok=false) outside a recording pass, outside the root scope, or for a
// region without a static count — those shapes keep today's refusal.
func (es *EmitState) SplitLoopRegionBind(name string, v core.Value) (core.Value, bool) {
	if !es.Active() || es.SuspendedNow() {
		return core.Value{}, false
	}
	if len(es.units) != 1 || es.reg == nil || es.reg.Check.FnBodyDepth != 0 ||
		es.reg.Check.NestedBodyDepth != es.reg.Check.LoopBodyDepth {
		// The split lowers at the top level (0 == 0) and inside PROVEN loop
		// bodies (S9.2a — NestedBodyDepth == LoopBodyDepth means every
		// enclosing body analysis is a statically-counted >= 1-trip,
		// sentinel-free loop body: AnalyseLoopBody stamps LoopBodyDepth only
		// under that proof, so a computed-count or break/continue-bearing
		// loop declines here too — PR #280 review). A BRANCH/QUOTATION body
		// keeps the decline: a conditionally-reached split would leak the
		// analysis-only binding (PR #278 review P1-b), and its fragment owns
		// different depth accounting (probe-pinned: the pre-gate widening
		// miscompiled [5 0 5 0] vs [5 5 5 5]).
		return core.Value{}, false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok || pr.idx != 0 {
		return core.Value{}, false
	}
	f := es.eventInfo[pr.seq]
	if !f.variadicResult || f.regionN < 1 || f.firstElemType == nil {
		return core.Value{}, false
	}
	// Only split when RecordDynBind will actually EMIT the splice for this
	// name: without the event there is no splice instruction to remove the
	// spare value at run time (`def _ (for 3 [1])` compiled [1 1 1] vs the
	// interpreter's [1 1]). An empty or capitalised name never records, so
	// those still decline outright; `_`/`$` DO record at the regime's root
	// seat, and asking recordsFilteredDynBind is what keeps this gate and
	// RecordDynBind's from drifting apart again — they did, and the drift
	// was NUR116's silent miscompile.
	if name == "" || core.IsCapitalisedName(name) {
		return core.Value{}, false
	}
	if (name[0] == '_' || name[0] == '$') && !es.recordsFilteredDynBind() { //covergate:allow the guard above already requires len(units)==1 && reg!=nil && FnBodyDepth==0 — recordsFilteredDynBind's root disjunct — so the predicate is true here by construction; asked anyway so this gate and RecordDynBind's stay one predicate (NUR116) (§compiler)
		return core.Value{}, false
	}
	// The split binds the region's FIRST-arrived value, so the element
	// carrier takes THAT value's type (firstElemType, guaranteed non-nil by
	// the gate above), not the loop carrier's child type — which mirrors the
	// LAST value and mis-types a heterogeneous body.
	elem := core.NewCarrier(f.firstElemType)
	es.pendingLoopBind = &pendingLoopBind{seq: pr.seq, depth: f.regionN - 1}
	return elem, true
}

// SplitEventRegionBind is SplitLoopRegionBind's sibling for a def binding the
// FIRST (stack-deepest, idx-0) result of a STATIC multi-out event — the
// fallible do-catch region (`def msg (do [(1 add 2) "no-raise"] error [dot
// code]) msg`, REFUSAL-CLOSURE S9.1 rows 1-2): the interpreter's pending
// forward binds the first-arrived value and the rest spill. The bind takes a
// fresh element carrier (so reads rescue via loopSplitBinds -> OpLookupDynScope
// instead of resolving the spliced-out event slot); the splice-at-depth
// lowering removes the runtime value AND its sim entry. Only the SUCCESS path
// executes the splice — the compiled catch path defers to the interpreter
// wholesale (the raise unwinds via vmDefer), so the static success count is
// the only count the lowering needs. TOP LEVEL only (the loop-body
// composition is S9.2a's separate machinery).
func (es *EmitState) SplitEventRegionBind(name string, v core.Value) (core.Value, bool) {
	if !es.Active() || es.SuspendedNow() {
		return core.Value{}, false
	}
	if len(es.units) != 1 || es.reg == nil || es.reg.Check.FnBodyDepth != 0 ||
		es.reg.Check.NestedBodyDepth != 0 {
		return core.Value{}, false
	}
	// The same single-sourced premise as SplitLoopRegionBind's (see there):
	// this sibling carried the identical stale spelling. It is not
	// divergence-bearing today — both lanes refuse the do-catch region shape
	// for other reasons — but a gate that agrees with RecordDynBind only by
	// coincidence is the bug NUR116 already was once.
	if name == "" || core.IsCapitalisedName(name) {
		return core.Value{}, false
	}
	if (name[0] == '_' || name[0] == '$') && !es.recordsFilteredDynBind() { //covergate:allow the guard above already requires len(units)==1 && reg!=nil && FnBodyDepth==0 — recordsFilteredDynBind's root disjunct — so the predicate is true here by construction; asked anyway so this gate and RecordDynBind's stay one predicate (NUR116) (§compiler)
		return core.Value{}, false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok || pr.idx != 0 {
		return core.Value{}, false
	}
	f := es.eventInfo[pr.seq]
	ev := es.eventBySeq(pr.seq)
	// EXACTLY two outputs: the splice lowering's stack model is proven only
	// for the two-value region (rows 1-2's shape — the bound first value plus
	// ONE spilled rest). A wider region's rest values need not all be SEATED
	// on the runtime stack when the bind executes (consts re-push at their
	// consumers, not eagerly), so SpliceFromTop = nout-1 reaches below the
	// live stack (probe-pinned: the three-value `do [(1 add 2) "a" "b"]`
	// region underflowed BIND_GLOBAL at run time — PR #280 review). nout != 2
	// keeps the decline until a general multi-value seating exists.
	if f.variadicResult || ev == nil || ev.kind != evCall || ev.call.nout != 2 {
		return core.Value{}, false
	}
	// The parked-fn screen: a Function among the SPILLED rest auto-applies in
	// the interpreter when a later value lands above it — keep those refused
	// (the same hazard the loop-region screens guard).
	if v.Parent != nil && v.Parent.ConformsTo(core.TFunction) {
		return core.Value{}, false
	}
	elemType := core.TAny
	if v.Parent != nil {
		elemType = v.Parent
	}
	elem := core.NewCarrier(elemType)
	es.pendingLoopBind = &pendingLoopBind{seq: pr.seq, depth: ev.call.nout - 1}
	return elem, true
}

// eventBySeq finds the event with the given seq in the CURRENT frame (nil
// when absent — a fragment-frame seq after the frame closed).
func (es *EmitState) eventBySeq(seq int) *EmitEvent {
	frame := es.frames[len(es.frames)-1]
	for i := range frame {
		if frame[i].seq == seq {
			return &frame[i]
		}
	}
	return nil
}

// resolveOperand maps a dispatch value to its provenance: a prior
// event's output, or an inert constant (concrete at the dispatch, or
// a stripped literal whose original RememberOriginal saved).
// producedInCurrentUnit reports whether the value's producing event was
// recorded INSIDE the unit now open — at or above that unit's own sequence
// floor — rather than in an enclosing frame.
//
// It guards the capture override below, whose reason is precisely that the
// producing event "lives in the parent frame and is unreachable from inside
// the body". When the event is this unit's OWN, that reason does not apply
// and the override is wrong: a native that returns its argument unchanged
// keeps the carrier's identity, so `( fn [[x:Integer][Any][do [j]]] )` over a
// captured `j` had the do's RESULT resolve back to the capture slot. The
// lowering then emitted `CALL_NATIVE do; DROP; PUSH_LOCAL`, discarding the
// call's value — and with it the word dispatch the body's deopt island had
// just performed, so the lambda answered `fn` for the interpreter's 42. The
// same shape in a plain fn body always agreed, which is what located the
// override rather than `do` or the deopt.
func (es *EmitState) producedInCurrentUnit(id string) bool {
	pr, ok := es.producedBy[id]
	if !ok || len(es.openUnitRecs) == 0 {
		return false
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if rec == nil {
		return false
	}
	// The unit's own sequence floor. While its body records, the live floor is
	// the one beginFragment pushed for its root frame; by the time the finish
	// callback resolves the body RESIDUAL that frame is closed and the floor
	// has moved onto rec.frag (the same value the provenance sweep below
	// compares against).
	//
	// The floors are pushed in lockstep with the frames, but es.frames opens
	// with a ROOT frame that beginFragment never pushed a floor for — so the
	// floor of frame n is fragFloors[n-1], not fragFloors[n]. Indexing them as
	// parallel put every in-body resolution out of range and the guard silently
	// declined: `do [j] typeof` kept the slot push while the body RESIDUAL form
	// `do [j]`, which reads rec.frag instead, was already fixed.
	switch {
	case rec.frag != nil:
		return pr.seq > rec.frag.startSeq
	case rec.rootFrame > 0 && rec.rootFrame <= len(es.fragFloors):
		return pr.seq >= es.fragFloors[rec.rootFrame-1]
	}
	return false
}

func (es *EmitState) resolveOperand(v core.Value) (EmitOperand, bool) {
	// A `/v` read of a def-bound capturing fn literal (readOps) is the
	// literal's closure operand, before any other resolution.
	if op, ok := es.readOps[v.ID]; ok {
		return op, true
	}
	// A CAPTURE of the CURRENT unit overrides events-first: the captured value
	// may carry a producedBy entry from the ENCLOSING unit (a computed
	// `def a (h add 1)` snapshotted into a closure), but that event lives in the
	// parent frame and is unreachable from inside the body — the reference must
	// resolve to this unit's own capture SLOT. Without this the each/scan/…$body
	// reference resolved to the parent event and refused "branch reads enclosing
	// computation". A param capture has no producedBy entry, so this only changes
	// the computed-capture case. (The captured VALUE is carried correctly by the
	// parent-side promotion of the computed def — see forEachOperand /
	// promoteOperand closureCaps handling.)
	if cur := es.units[len(es.units)-1]; cur != nil && cur.capID[v.ID] {
		if slot, ok := cur.localByID[v.ID]; ok && !es.producedInCurrentUnit(v.ID) {
			return localOperand(slot), true
		}
	}
	// Events first, locals second: a join can REUSE a local's value ID
	// for its result (JoinCarriers keeps the then-side ID when types
	// agree), and the event is then the value's stack-discipline truth
	// — the branch pushed it. A plain param/iterator reference has no
	// producing event and resolves to its local slot.
	if pr, ok := es.producedBy[v.ID]; ok {
		// A type operand whose ID matches a producing event whose own
		// output was NOT a type is an ID collision (a `make` result
		// inheriting the type literal's ID): resolve it as its own type
		// operand / const below, not the unrelated event. A DYNAMIC
		// carrier is exempt: it is the checker's gradual stand-in for a
		// runtime VALUE, never a type body the program manipulates as
		// data — narrowing-through-use rebinds a dynamic Any def to a
		// typed-list/typed-map bound ([:Any]) that trips IsTypeBody, but
		// its ID-matched event genuinely produced it (the identity-
		// preserving rebind in narrowDynamicUses).
		if !core.IsTypeBody(v) || v.Dynamic || es.eventInfo[pr.seq].typeOut {
			// An ENCLOSING-scope binding read (a module-scope `flex`, a computed
			// module `def`) carries a producedBy event from the PARENT/module
			// frame — unreachable from this fn unit's sim stack. Route it to a
			// dynamic-scope lookup (OpLookupDynScope) so the VM re-resolves the
			// live binding per call, exactly as the interpreter reads the name
			// against the live def stack (by-reference for a mutable flex). The
			// signal is precise: only a value with BOTH a producing event AND a
			// def-binding ID snapshotted at unit open takes this path, so a
			// body-local producer (its ID absent from enclosingBindIDs) and an
			// immutable const literal (no producing event) are untouched.
			if cur := es.units[len(es.units)-1]; cur != nil && len(es.units) > 1 && cur.enclosingBindIDs[v.ID] {
				if op, ok := es.dynScopeRescue(v); ok {
					return op, true
				}
			}
			return EventOperand(pr.seq, pr.idx), true
		}
	}
	if slot, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		return localOperand(slot), true
	}
	// A bare type node is a TYPE operand: it must reach the runtime
	// as the CANONICAL registry node (a pooled by-value copy goes
	// stale against behaviour/field installs), so it gets its own
	// table, resolved by ID at run time via OpPushType.
	if core.IsBareTypeNode(v) && v.ID != "" {
		// A CLASS TYPE carries fn values too — its field DEFAULTS — and this
		// return is before the const chokepoint's stamp below, so they used to
		// reach no stamp at all. `def C class {op:(fn …)}  (make C {}).op 21`
		// disassembles to `fns=0`: the fn is not in any value const, it is on
		// the type node, and `make` copies it into the instance by pointer.
		es.stampFnConstType(v)
		return typeOperand(es.internType(v)), true
	}
	// A MUTABLE reference value (a `flex` map/list, a Store) read from an
	// ENCLOSING scope must NEVER bake as a PUSH_CONST: the constant is a frozen
	// snapshot, so a compiled `set` and a compiled `get` over one module-scope
	// flex would touch DIFFERENT instances (sift's kind catalog — a runtime
	// Sift.define's registration was invisible to a later Sift.parse). Route it
	// to the live dynamic-scope lookup (OpLookupDynScope) so every reference
	// re-resolves the one shared binding, exactly as the interpreter reads the
	// name against the live def stack. Gated inside a fn unit (dynScopeRescue
	// self-guards on len(units) > 1); a top-level flex has no baking hazard (its
	// single frame IS the live binding), so a failed rescue falls through
	// unchanged. Scoped to the whole-program / module-load compile
	// (storedGradualDepth == 0), where this rescue is validated and the sift
	// module's own fns compile. A DETACHED stamp (StampDetachedFn's isolated
	// one-unit fork, storedGradualDepth > 0) recompiles a runtime-constructed fn
	// VALUE, which never reads the module flexes; it keeps its established
	// lowering (a decline there is a sound per-body interpreter fallback).
	if es.storedGradualDepth == 0 && (core.IsFlexMap(v) || core.IsFlexList(v) || core.IsWeakFlexNode(v) || core.IsStore(v)) {
		if op, ok := es.dynScopeRescue(v); ok {
			return op, true
		}
	}
	lit, ok := es.Materialise(v)
	if !ok {
		return es.dynScopeRescue(v)
	}
	// At MODULE scope a NoEvalArgs body that is inert except for InterpStrings
	// (InterpBodyInert) bakes as code-as-data and is re-interpreted against the
	// registry — sound where there is no enclosing VM frame to shadow (see
	// noEvalBodiesInertScoped). isInertConst stays strict so this admission never
	// reaches a compiled fn frame: there the body refuses at the Stage-2 gate
	// before resolveOperand, so the operand never gets here.
	if !core.IsInertConst(lit) && !(len(es.units) == 1 && InterpBodyInert(lit)) {
		return es.dynScopeRescue(v)
	}
	// An ACTIVE-token MAP (`{line: src}` — a word / paren / interpolation
	// member) must not bake inside a compiled fn frame: autoEvalMap runs on
	// EVERY map argument at dispatch (NoEvalArgs does not suppress it), so
	// the interpreter re-evaluates the member per dispatch against the LIVE
	// FRAME (src is a param), which the VM holds as a frame local the baked
	// const can never see. Lists stay bakeable — a NoEvalArgs code body
	// (`each [dup mul]`, `each $.1`) is legitimately code-as-data, guarded
	// by the position-aware body gates (execBodyRefsNames /
	// noEvalBodiesInertScoped). Module scope keeps the map bake too —
	// resolution there is registry-identical.
	if len(es.units) > 1 {
		if _, isMap := lit.Data.(core.MapPayload); isMap && core.BearsActiveTokens(lit) {
			return es.dynScopeRescue(v)
		}
		// A compound with NO identity (a runtime mint under the mode-gated
		// ID elision) cannot be placed inside a fn unit: the per-call
		// identity machinery below (freshen / share / embed detection) is
		// keyed on value IDs, and an identity-less compound would slip past
		// the enclosing-binding probe and be wrongly freshened (breaking
		// member identity with the live runtime instance). Rescue —
		// dynamic-scope read or refusal — never freshen.
		if v.ID == "" && freshenableConst(lit) {
			return es.dynScopeRescue(v)
		}
	}
	idx := es.intern(lit)
	// A compound VALUE literal materialised inside a fn unit gets per-call
	// identity treatment (OpPushConstFresh / share / refuse — see
	// freshenFnUnitConsts) unless its ID was already a binding's value at
	// unit open, i.e. an enclosing scope constructed it once and the body
	// merely reads it. Compounds are never pooled (intern), so idx belongs
	// to exactly this materialise context.
	if len(es.units) > 1 && freshenableConst(lit) &&
		!es.units[len(es.units)-1].enclosingIDs[v.ID] {
		// A body literal EMBEDDING an enclosing binding's container cannot
		// be freshened OR shared: the interpreter constructs the OUTER
		// literal fresh per call while the binding-read MEMBER keeps its
		// shared instance (`def c [9] def mk fn [[] [List] [[c]]]` —
		// `((mk) get 0) eq c` stays true, `(mk) eq (mk)` stays false).
		// A deep-clone freshen breaks the member identity; a shared const
		// breaks the outer's. Until a selective (spine-only) freshen
		// exists, refuse — the sound fallback (PR #225 P1).
		if embedsEnclosingCompound(lit, es.units[len(es.units)-1].enclosingIDs) {
			es.MarkUncompilable("fn body literal embeds an enclosing binding's container (per-call spine identity over a shared member)")
			return EmitOperand{}, false
		}
		if es.freshenConst == nil {
			es.freshenConst = map[int]bool{}
		}
		es.freshenConst[idx] = true
	}
	es.stampFnConst(lit)
	return ConstOperand(idx), true
}

// embedsEnclosingCompound reports whether a compound literal (recursively)
// contains a compound member whose identity is an ENCLOSING binding's value
// — the shape whose interpreter semantics mix a per-call-fresh spine with a
// shared member, which neither OpPushConstFresh (deep clone) nor a shared
// pooled const can model. See the refusal at the resolveOperand marking
// site (PR #225 P1).
func embedsEnclosingCompound(v core.Value, enclosing map[string]bool) bool {
	switch d := v.Data.(type) {
	case core.ListPayload:
		for _, e := range d.Elems {
			if !freshenableConst(e) {
				continue
			}
			if enclosing[e.ID] || embedsEnclosingCompound(e, enclosing) {
				return true
			}
		}
	case core.MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !freshenableConst(mv) {
				continue
			}
			if enclosing[mv.ID] || embedsEnclosingCompound(mv, enclosing) {
				return true
			}
		}
	}
	return false
}

// freshenFnUnitConsts gives fn-unit compound body literals per-call identity
// (miscompile mechanism A — see OpPushConstFresh, bytecode.go). For each
// marked const (freshenConst) pushed by this unit's finished code:
//   - one push site → rewritten IN PLACE to OpPushConstFresh (no instruction
//     insertion, so jump targets are untouched): the interpreter constructs
//     the literal fresh on every call — and on every loop iteration, which a
//     single re-executed site models exactly;
//   - several push sites → the pushes stand for reads of ONE per-call
//     binding, so they must share within a call; the shared pooled const is
//     indistinguishable from that unless an instance escapes the fn. When
//     every declared return conforms to Scalar nothing compound can escape,
//     and the shared const is exact parity — keep it. Otherwise the reads
//     seat a single per-call construction in a fresh frame local
//     (OpPushConstFreshLocal) — every marked shape now lowers; the pass
//     cannot refuse.
func freshenFnUnitConsts(cf *CompiledFn, es *EmitState, rec *fnUnitRec, p *Program) {
	if len(es.freshenConst) == 0 {
		return
	}
	sites := map[int][]int{}
	for pc, in := range cf.Code {
		if in.Op == OpPushConst && es.freshenConst[int(in.Arg)] {
			sites[int(in.Arg)] = append(sites[int(in.Arg)], pc)
		}
	}
	for idx, pcs := range sites {
		if len(pcs) == 1 {
			// One read site: construct fresh each time this pc executes (and each
			// loop iteration, which a single re-executed site models exactly).
			cf.Code[pcs[0]].Op = OpPushConstFresh
			continue
		}
		if returnsAllScalar(rec.returns) {
			// No compound can escape a Scalar-returning fn, so the shared pooled
			// const is exact within-call parity — keep the bare pushes.
			continue
		}
		// Multiple read sites of ONE compound binding in an escape-capable unit:
		// seat a single per-call construction in a fresh frame local, and rewrite
		// every read to OpPushConstFreshLocal (lazy clone-on-first-read, shared
		// thereafter). This is the "per-call local seat" the shared const cannot
		// give — one instance per call, shared by every read, fresh across calls,
		// exactly the interpreter's `def x {…}` semantics. In-place rewrites only,
		// so jump targets stay valid.
		slot := cf.NLocals
		cf.NLocals++
		for len(cf.LocalNames) < cf.NLocals {
			cf.LocalNames = append(cf.LocalNames, "")
		}
		ref := len(p.ConstLocals)
		p.ConstLocals = append(p.ConstLocals, ConstLocalRef{ConstIdx: idx, Slot: slot})
		for _, pc := range pcs {
			cf.Code[pc].Op = OpPushConstFreshLocal
			cf.Code[pc].Arg = int32(ref)
		}
	}
}

// returnsAllScalar reports whether every declared return conforms to Scalar
// — the cheap sufficient condition that no container instance can escape a
// fn, making a shared multi-read compound const exact within-call parity
// (freshenFnUnitConsts). Unchecked or absent returns assume escape.
func returnsAllScalar(returns []*core.Type) bool {
	if len(returns) == 0 {
		return false
	}
	for _, t := range returns {
		if t == nil || !t.ConformsTo(core.TScalar) {
			return false
		}
	}
	return true
}

// snapshotCompoundBindingIDs collects the value IDs of every compound
// (List/Map) binding currently visible in the DefTable — the values a fn
// unit opening NOW would read from ENCLOSING scope. See
// emitUnit.enclosingIDs for the semantics.
func (es *EmitState) snapshotCompoundBindingIDs() map[string]bool {
	ids := map[string]bool{}
	if es.reg == nil || es.reg.Defs == nil {
		return ids
	}
	for _, name := range es.reg.Defs.Names() {
		for _, bv := range es.reg.Defs.Stack(name) {
			if freshenableConst(bv) && bv.ID != "" {
				ids[bv.ID] = true
			}
		}
	}
	return ids
}

// snapshotAllBindingIDs snapshots the value IDs of EVERY def binding visible
// in the DefTable at unit open (the superset of snapshotCompoundBindingIDs).
// Used to recognise an enclosing-scope binding read whose producing event
// lives in the parent/module frame — resolveOperand routes it to a dynamic-
// scope lookup rather than an unreachable in-frame event operand.
func (es *EmitState) snapshotAllBindingIDs() map[string]bool {
	ids := map[string]bool{}
	// Top-level computed fn value-defs installDef declined to install in Defs
	// (`def op (Parse.parser g)`) are still real enclosing bindings; the Defs
	// scan below misses them, so merge their recorded IDs in.
	for id := range es.rootComputedBindIDs {
		ids[id] = true
	}
	if es.reg == nil || es.reg.Defs == nil {
		return ids
	}
	for _, name := range es.reg.Defs.Names() {
		for _, bv := range es.reg.Defs.Stack(name) {
			if bv.ID != "" {
				ids[bv.ID] = true
			}
		}
	}
	return ids
}

// snapshotAllBindingNames snapshots the NAMES of every def binding visible in
// the DefTable at unit open — the by-name twin of snapshotAllBindingIDs, used
// only to recover an ENCLOSING mutable-reference read whose value ID was elided
// at pure runtime (a detached stamp). See emitUnit.enclosingBindNames.
func (es *EmitState) snapshotAllBindingNames() map[string]bool {
	names := map[string]bool{}
	if es.reg == nil || es.reg.Defs == nil {
		return names
	}
	for _, name := range es.reg.Defs.Names() {
		names[name] = true
	}
	return names
}

// tryReturnedClosure compiles a fn VALUE that a body RETURNS — the factory
// pattern `def mk fn [[x:Integer] [Function] [([y:Integer] => […])]]`, whose
// body residual is an anonymous lambda — into its own closure unit, yielding an
// opClosure operand (lowering to OpPushClosure). The caller (the fn-finish
// residual resolution) falls back here when resolveOperand cannot place the
// value. Probe-guarded so a refusal leaves the real state untouched.
//
// Only an ANONYMOUS, single-own-sig FnDefInfo qualifies: a named ref
// re-dispatches by name; an overloaded fn needs runtime MatchFnSig; a sentinel
// body targets an enclosing frame. Anything else returns false and the program
// falls back faithfully.
//
// A CAPTURING lambda is admitted: the factory pattern `def mk fn [[x][Function]
// [([y]=>[x add y])]]` returns a closure over the factory's PARAM x. Each
// captured binding is resolved in THIS (the factory body's) scope to an operand
// — x is the factory's frame local — and threaded as a closureCap, so the
// lowering pushes the captured value before OpPushClosure exactly as the in-place
// closure-dispatch path does (lower.go opClosure). A capture that cannot resolve
// here (an unreachable enclosing binding) declines and the program falls back.
// fnValueInputs builds the per-param input carriers and name table for compiling
// a fn value's body — a returned lambda (tryReturnedClosure) or a stored handler
// (compileStoredFnUnit). Each declared param type becomes a carrier (Any when the
// param carries only a pattern, hence no bare type), and a named param carries
// its name so AnalyseFnBody binds the body's references to that input.
func fnValueInputs(params []core.FnParam) (inputs []core.Value, names []string) {
	inputs = make([]core.Value, len(params))
	names = make([]string, len(params))
	for i, p := range params {
		t := p.Type
		if t == nil {
			t = core.TAny
		}
		// ParamInputCarrier (not a strict NewCarrier): an explicitly-`Any`
		// handler param binds a GRADUAL carrier, so a body word over it
		// poly-matches at runtime instead of failing no_signature against the
		// strict Any top — the same treatment the user-fn compile path gives
		// (user_poly.go). This lets a `serve-raw` handler `[sock:Any]` compile
		// its socket-word dispatch to the VM (the connection value IS a Socket
		// at runtime); a concrete-typed param stays strict.
		inputs[i] = check.ParamInputCarrier(t)
		names[i] = p.Name
	}
	return inputs, names
}

func (es *EmitState) tryReturnedClosure(v core.Value, pos core.SrcPos) (EmitOperand, bool) {
	if es == nil || es.reg == nil || v.Carrier || v.Dynamic || v.Quoted {
		// A QUOTED fn value is DATA the interpreter keeps unapplied; lowering
		// it to an opClosure drops the Quoted flag, so the VM would
		// auto-apply it (`[quote (fn …)]` returned then applied — PR #279
		// review: compiled [3] vs interp [fn (Integer) 2]). Keep it inert.
		return EmitOperand{}, false
	}
	fd, ok := v.Data.(core.FnDefInfo)
	// An ANONYMOUS lambda (=>/afn) or a NAMELESS verbose `fn` construction
	// (REFUSAL-CLOSURE §9.2d — the curried factory's inner fn) both model as
	// returned closures; a NAMED fn value carries registry dispatch and
	// recursion semantics this model does not own, so it declines.
	if !ok || (!fd.Anonymous && fd.Name != "") {
		return EmitOperand{}, false
	}
	// CAPTURELESS values decline too — the const bake (the caller's
	// materialise path) carries the REAL FnDefInfo, which a native that
	// VALIDATES its fn operand's signatures (parselang-fn-dispatch's
	// [source opts] contract, the service family's MatchFnSig) can read,
	// where an OpPushClosure ClosurePayload carries only the unit ref and
	// is rejected as "not a usable function value" (PR #295 merge: main's
	// `parse (mk) 'hi'` corpus row met §9.2d's widening — the compiled
	// closure return broke the dispatcher the interpreter satisfies). Only
	// a CAPTURING fn — which cannot bake (its captures are runtime values)
	// — takes the closure unit; that is §9.2d's genuine coverage.
	if len(fd.Captured) == 0 {
		return EmitOperand{}, false
	}
	// Resolve the lambda's captures in the ENCLOSING (factory body) scope, the
	// same operand resolution recordClosureDispatch uses for an in-place closure.
	// A capture that does not resolve (unreachable enclosing binding) declines.
	capOps := make([]EmitOperand, len(fd.Captured))
	for i, cb := range fd.Captured {
		op, okCap := es.resolveOperand(cb.Value)
		if !okCap {
			return EmitOperand{}, false
		}
		capOps[i] = op
	}
	// Exactly one own (non-fallback) signature — FirstOwnSig is otherwise not
	// guaranteed to be the overload a runtime MatchFnSig would pick.
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	lam, hasOwn := fd.FirstOwnSig()
	if own != 1 || !hasOwn || len(lam.Body()) == 0 || bodyToksHaveSentinel(lam.Body()) {
		return EmitOperand{}, false
	}
	inputs, paramNames := fnValueInputs(lam.Params)
	ps := lamParamContract(lam)
	r := es.reg
	// PROBE in a throwaway emit state (mirrors recordClosureDispatch), so a body
	// that refuses leaves THIS program untouched and the value stays unresolved.
	probe := NewEmitState()
	probe.reg = r
	// The probe inherits the gradual-nesting mode (a detached stamp arms it on
	// the real state): probe and real must compile under the SAME modality or
	// the probe's verdict is about a different unit — without this, an inner
	// lambda inside a stored service handler (mini-s3's bucket-list filter,
	// whose body runs `convert … e.value` over an Any param) probed STRICT,
	// refused "unmatched dispatch recovered", and surfaced as the misleading
	// "function-valued operand at filter (Stage 3)".
	probe.storedGradualDepth = es.storedGradualDepth
	probe.dynEnv = es.dynEnv
	r.Check.Emit = probe
	// bodyOut 1: a fn VALUE body keeps the single declared return (it is not a
	// 0-output side-effect body like a test case).
	_, probeOK := compileClosureBody(r, "fnval", 1, false, lam.Body(), inputs, paramNames, ps.Patterns, fd.Captured, ClosureInValue, pos)
	r.Check.Emit = es
	if !probeOK {
		return EmitOperand{}, false
	}
	// The probe ran the body END TO END, so it knows the pass's terminal
	// environment mode: a dyn-body dispatch (tryRecordDynBody) anywhere
	// inside armed probe.dynEnv. Arm the REAL state NOW — before the real
	// compile — so every unit it finishes plans under the widened mode.
	// DynEnv arming mid-pass otherwise drifts plan against lowering: a unit
	// finished BEFORE the arming plans without dyn-bind source promotion,
	// then Finalize lowers it widened and refuses "dynamic-scope def of
	// unpromoted computed value" (mini-s3's s3-serve — the store's append
	// handler finishes before the serve-raw closure reaches s3-conn's
	// `do … error …`).
	if probe.dynEnv {
		es.dynEnv = true
	}
	// REAL: compile into this program (deterministic success after a clean probe).
	unit, realOK := compileClosureBody(r, "fnval", 1, false, lam.Body(), inputs, paramNames, ps.Patterns, fd.Captured, ClosureInValue, pos)
	if !realOK || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return EmitOperand{}, false
	}
	es.fnRecs[unit].render = core.FormatFnDef(fd)
	// The returned lambda's declared param contract rides on its unit, so a
	// runtime that dispatches the closure BY NAME (the word-read replay's
	// closureAsWord bridge, NUR123) declares the interpreter's signature.
	es.SetUnitParamTypes(unit, ps.Types, ps.Patterns)
	// The returned lambda's RETURN contract rides on the push (ClosureRet →
	// ClosurePayload.RetTypes), so the VM's invoke enforces the lambda's
	// count contract (LambdaCountContract) exactly as the interpreter's
	// ReturnCheck does at every dispatch of the value: a body that
	// under-applies at its tail (`[7 x f/v apply]` over a 1-arg f) nets two
	// values, and the interpreter raises `expected 1 return value(s), got 2`
	// where an uncontracted closure answered [7 8] (the twenty-ninth
	// increment; latent before it, every such call site refused).
	return EmitOperand{kind: opClosure, closureUnit: unit, closureCaps: capOps, closureRet: fnValueRetSpec(&fd, lam, pos)}, true
}

// compileStoredFnUnit compiles a CAPTURE-FREE store-fn handler body (the fn a
// CompileStoresFn word stashes for later invocation — a serve-raw connection
// handler) to its own fn unit, so the native word can run it on the VM via
// RunUnit instead of CallBoru. Returns the unit index and true, or (0, false)
// when the body refuses (a refusal-class word, a flow sentinel, an ineligible
// sig) — the caller then bakes only the plain const and that overload falls
// back to the interpreter, per-sig and sound. Mirrors tryReturnedClosure's
// probe-then-real compile, but bodyOut 0 (count-agnostic): a stored handler is
// invoked for effect and its residual, like CallBoru's, is the caller's to use
// or discard.
func (es *EmitState) compileStoredFnUnit(fd core.FnDefInfo, sigIdx int, pos core.SrcPos) (int, bool) {
	if es == nil || es.reg == nil {
		return 0, false
	}
	if sigIdx < 0 || sigIdx >= len(fd.Signatures) || !storedSigEligible(&fd.Signatures[sigIdx]) {
		return 0, false
	}
	lam := &fd.Signatures[sigIdx]
	inputs, paramNames := fnValueInputs(lam.Params)
	r := es.reg
	// PROBE in a throwaway state so a refusing body leaves THIS program
	// untouched (mirrors tryReturnedClosure / recordClosureDispatch).
	probe := NewEmitState()
	probe.reg = r
	// The probe inherits the gradual-nesting mode (a detached stamp arms it
	// on the real state) — probe and real must compile under the SAME
	// modality or the probe's verdict is about a different unit.
	probe.storedGradualDepth = es.storedGradualDepth
	probe.dynEnv = es.dynEnv
	r.Check.Emit = probe
	_, probeOK := compileClosureBody(r, "storedfn", 0, true, lam.Body(), inputs, paramNames, nil, fd.Captured, ClosureInValue, pos)
	r.Check.Emit = es
	if !probeOK {
		// Surface the probe's refusal for the -compile-report attribution
		// (stamp_report.go); the compile-time bake ignores the field.
		es.storedFnProbeReason = probe.Reason
		return 0, false
	}
	// Carry the probe's TERMINAL environment mode into the real pass (see
	// tryReturnedClosure): a dyn-body dispatch inside the body arms dynEnv,
	// and units the real pass finishes before reaching that dispatch must
	// plan under the widened mode or Finalize refuses their dyn-binds.
	if probe.dynEnv {
		es.dynEnv = true
	}
	unit, realOK := compileClosureBody(r, "storedfn", 0, true, lam.Body(), inputs, paramNames, nil, fd.Captured, ClosureInValue, pos)
	if !realOK || unit < 0 {
		// Reachable: a body the probe pass accepted can still refuse in the
		// real pass (the variation sweep produces such shapes — a splice-
		// wrapped seed whose stored-fn body declines under the widened env).
		return 0, false
	}
	return unit, true
}

// storedSigEligible reports whether ONE signature of a stored fn value is a
// stampable unit shape: an own (non-fallback) boru body, non-empty and free
// of flow-control sentinels. Every own sig stamps INDEPENDENTLY
// (REFUSAL-CLOSURE §7b): the callback seam dispatches through MatchFnSig
// first, so the matched sig's own Impl ref IS the "sig table" — an
// unstamped sibling simply interprets via CallBoru, per-sig and fail-safe.
// Shared by the compile-time store-fn bake and the runtime detached stamp
// so the two gates cannot drift. Capture-freedom is the CALLER's gate
// (RecordCallOperands / StampFnValue): it is a property of the storing
// context, not of the unit shape.
func storedSigEligible(sig *core.Signature) bool {
	if sig.Fallback {
		return false
	}
	if _, isBoru := sig.Impl.(*core.BoruImpl); !isBoru {
		return false
	}
	return len(sig.Body()) > 0 && !bodyToksHaveSentinel(sig.Body())
}

// firstStampableSig returns fd's first stampable own boru sig — the
// single-sig entry the predicate-type constructor and the module-load
// sweep use (their values are single-overload by construction); multi-sig
// values stamp per sig through the callers' own loops.
func firstStampableSig(fd core.FnDefInfo) (int, bool) {
	for i := range fd.Signatures {
		if storedSigEligible(&fd.Signatures[i]) {
			return i, true
		}
	}
	return -1, false
}

// compileStoredBody compiles a NoEvalArgs CODE-BODY list (spawn's process body) to
// a 0-param unit and returns a synthetic fn-value carrier holding both the raw
// Body tokens (interpreter fallback) and a CompiledFnRef (registered for the
// Finalize back-stamp). The storing word runs the carrier's unit via RunUnit on
// its own registry. Returns ok=false when the body is empty, carries a flow
// sentinel, or refuses to compile — the caller then bakes the plain const list
// and the word runs it on the interpreter, unchanged. Mirrors compileStoredFnUnit
// but for a bare token body rather than a fn value's sig body.
func (es *EmitState) compileStoredBody(bodyList core.Value) (core.Value, bool) {
	if es == nil || es.reg == nil {
		return core.Value{}, false
	}
	lst, err := core.AsList(bodyList)
	if err != nil || lst.IsNil() {
		return core.Value{}, false
	}
	tokens := lst.Slice()
	if len(tokens) == 0 || bodyToksHaveSentinel(tokens) {
		return core.Value{}, false
	}
	r := es.reg
	probe := NewEmitState()
	probe.reg = r
	// Same-modality probe (see tryReturnedClosure / compileStoredFnUnit).
	probe.storedGradualDepth = es.storedGradualDepth
	probe.dynEnv = es.dynEnv
	r.Check.Emit = probe
	_, probeOK := compileClosureBody(r, "spawnbody", 0, true, tokens, nil, nil, nil, nil, ClosureInValue, bodyList.Pos())
	r.Check.Emit = es
	if !probeOK {
		return core.Value{}, false
	}
	// Probe-terminal environment mode → real pass (see tryReturnedClosure).
	if probe.dynEnv {
		es.dynEnv = true
	}
	unit, realOK := compileClosureBody(r, "spawnbody", 0, true, tokens, nil, nil, nil, nil, ClosureInValue, bodyList.Pos())
	if !realOK || unit < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return core.Value{}, false
	}
	if unit < len(es.fnRecs) && es.fnRecs[unit] != nil && regionValsMayBeCallable(es.fnRecs[unit].outOpsVals) {
		// The body compiles, and its residual can leave a CALLABLE. A word
		// that hands such a residual back as its OWN result (await's
		// winner-takes-all modes) may not record it as a region — see
		// eventFlags.regionMayBeFn. Noted rather than refused here: a
		// fixed-arity caller of the same edge is unaffected, and the body
		// itself compiles perfectly well.
		es.storedBodyFnResidual = true
	}
	ref := &CompiledFnRef{Unit: unit, depNames: es.storedHandlerDeps(tokens)}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	carrier := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{
		Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: tokens, Compiled: ref}}},
	}}
	return carrier, true
}

// storedBodySpecFor returns the StoredBodySpec declared for sig position i,
// or nil when the position carries no param-carrying stored body.
func storedBodySpecFor(sig *core.Signature, i int) *core.StoredBodySpec {
	for j := range sig.StoredBodies {
		if sig.StoredBodies[j].Pos == i {
			return &sig.StoredBodies[j]
		}
	}
	return nil
}

// compileStoredParamBody is compileStoredBody for a PARAM-CARRYING stored
// body (a Signature.StoredBodies position — Test.check-prop's gen/property):
// the body compiles to a closure unit whose leading param slots bind the
// declared params (a named param binds the body's reads of that name, the
// handler's own CallBoru frame shape; an unnamed one rides the stack), and
// the carrier's single sig mirrors the handler's CallBoru sig — same Params,
// same raw Body tokens — plus the CompiledFnRef, so the handler dispatches
// it through InvokeCallback with byte-identical interpreter fallback. The
// unit is compiled INLINE into the current program, so a mid-run invoke is
// a same-program nested run (runUnitNested hosts it), not a foreign ref.
func (es *EmitState) compileStoredParamBody(bodyList core.Value, params []core.FnParam) (core.Value, bool) {
	if es == nil || es.reg == nil {
		return core.Value{}, false
	}
	lst, err := core.AsList(bodyList)
	if err != nil || lst.IsNil() {
		return core.Value{}, false
	}
	tokens := lst.Slice()
	if len(tokens) == 0 || bodyToksHaveSentinel(tokens) {
		return core.Value{}, false
	}
	inputs := make([]core.Value, len(params))
	names := make([]string, len(params))
	for i, p := range params {
		t := p.Type
		if t == nil {
			t = core.TAny
		}
		// GRADUAL (dynamic) input carriers, not strict NewCarrier ones. A
		// stored-param body's input is the value the handler feeds per invoke —
		// a check-prop generator result, a spawn arg — dispatched over its
		// RUNTIME type, exactly the gradual contract. A STRICT Any carrier falls
		// to `v.Is(t)` in sigTypeMatches (Any does not conform to List), so a
		// comparator-taking dispatch over the param — `(lst Sort.quick
		// Sort.by-number)` with `lst` the untyped generated list — failed to
		// commit `Sort.quick` (its `lst:List` slot rejected the strict Any) and
		// the trailing comparator `Sort.by-number` dispatched instead, inverting
		// the call to `cmp(lst, quick-fn)` (`cmp: cannot order List and
		// Function`). ParamInputCarrier(TAny) is a DYNAMIC carrier: the not-
		// disjoint rule matches List optimistically, `Sort.quick` commits, and
		// the body compiles the SAME dispatch the interpreter runs — full
		// compilation, not a fallback. Mirrors the storedGradualDepth branch's
		// ParamInputCarrier generalisation (core_helpers), now unconditional for
		// every stored-param body since the input is always runtime-typed.
		inputs[i] = check.ParamInputCarrier(t)
		names[i] = p.Name
	}
	r := es.reg
	probe := NewEmitState()
	probe.reg = r
	// Same-modality probe (see tryReturnedClosure / compileStoredBody).
	probe.storedGradualDepth = es.storedGradualDepth
	probe.dynEnv = es.dynEnv
	r.Check.Emit = probe
	_, probeOK := compileClosureBody(r, "storedfn", core.BodyOutResidual, true, tokens, inputs, names, nil, nil, ClosureInValue, bodyList.Pos())
	r.Check.Emit = es
	if !probeOK {
		return core.Value{}, false
	}
	// Probe-terminal environment mode → real pass (see tryReturnedClosure).
	es.dynEnv = es.dynEnv || probe.dynEnv
	unit, realOK := compileClosureBody(r, "storedfn", core.BodyOutResidual, true, tokens, inputs, names, nil, nil, ClosureInValue, bodyList.Pos())
	if !realOK || unit < 0 {
		// Unlike compileStoredBody's spawn shape, the real pass CAN decline
		// after a clean probe here: it records into the LIVE mid-recording
		// state, whose memo / poisoning / uncompilable latches the fresh
		// probe state doesn't carry. The decline is per-body and sound — the
		// raw list rides and the handler interprets it.
		return core.Value{}, false
	}
	ref := &CompiledFnRef{Unit: unit, depNames: es.storedHandlerDeps(tokens)}
	es.storedFnRefs = append(es.storedFnRefs, ref)
	carrier := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:     append([]core.FnParam(nil), params...),
			Returns:    []*core.Type{core.TAny},
			BarrierPos: -1,
			Impl:       &core.BoruImpl{Body: tokens, Compiled: ref},
		}},
	}}
	return carrier, true
}

// stampCompiledRef records ref on the fn value's first own (non-fallback) boru
// body sig, so the runtime FnDefInfo the store-fn word receives carries the VM
// edge alongside its raw Body. Mutates the shared *BoruImpl pointer, so the
// interned const reflects it. Returns false when no own boru body sig exists (a
// Go-backed or fallback-only fn value — never a stored boru handler).
func StampCompiledRef(fd core.FnDefInfo, ref *CompiledFnRef) bool {
	for i := range fd.Signatures {
		if fd.Signatures[i].Fallback {
			continue
		}
		if a, ok := fd.Signatures[i].Impl.(*core.BoruImpl); ok {
			a.Compiled = ref
			return true
		}
	}
	return false
}

// stampFnConst compiles a fn-value CONST's body to a unit of this program and
// stamps the ref on the value's own sig impls, so a DYNAMIC APPLY of that value
// runs on the VM instead of islanding.
//
// The gap this closes. `def twice fn [[f:Function x:Integer] [Integer] [f (f
// x)]]  twice (…lambda…) 5` lowers to CALL_DYN_* with no OpFallback anywhere —
// `fallbacks=0`, and TestCompiledCoverage calls it "0 islanded". At RUN time
// the dynamic-apply opcodes islanded the callee through a sub-engine, because a
// runtime fn value carrying no compiled unit has nothing else to run. The
// disassembly metric cannot see that; the interp-entry census can, and it is
// where 43 of the corpus's 66 `vm:island` entries came from.
//
// A dynamic apply's callee is unknown at compile time BY CONSTRUCTION, so the
// only place its compiled body can live is on the VALUE. That is what makes
// this the const chokepoint's business rather than any call site's.
//
// IN-PROGRAM, not detached — measured, not assumed. StampDetachedSig would
// isolate each stamp on its own ForkConcurrent, which contains a refusal
// neatly, and costs a registry fork plus a full compile pass PER fn const: 6x
// on lang/go/modules and a transient OOM. compileStoredFnUnit lands the unit in
// this program for the price of a body compile, and the refusal it exposes is
// handled where it belongs — see fnUnitRec.stampOnly.
//
// Scope: a CAPTURING fn is skipped (its captures are the closure lowering's
// business), an already-stamped value is left alone (first stamp wins — the
// STORE-FN edge may have got there), and a declining sig stays plain. Every
// decline is silent and per-sig: a missing ref costs speed, never an answer.
// NO CONTAINER DESCENT — and the reason is measured, not a contract reading.
//
// Where the remaining work is: of the island rows this seam leaves, roughly
// four in five are a fn read out of a container — `def m {f: (fn …)}  m.f 5`,
// `def ops {f: inc/v}  ops.f 5`, a class field method, `def m {a:add/v}  m.a/u
// 1 2`. So descending into list and map consts is the rest of the win, and it
// WORKS: corpus census 141 -> 131, 0 refused, 0 islanded, differential green.
//
// It also makes some programs REFUSE that previously compiled. Measured on the
// variation sampler, which the corpus alone does not reach:
//
//	                 before   after
//	pass               403     401
//	refused             33      36
//
// with three new buckets, and their names say what leaked: "dynamic-scope def
// `files` of unpromoted computed value" and "module binding files rebound after
// a stored handler captured it as a dep". Both come from the ENCLOSING compile,
// not from the stamped body — so compileStoredFnUnit mutates shared emit state
// (dep records, dynamic-scope promotion decisions) and not only the diagnostics
// TruncateDiagnostics already restores. A stamp is a speculative compile of
// code the program may never apply; it must leave the enclosing compile exactly
// as it found it, and today it does not.
//
// AND IT LANDS ANYWAY, because the alternative is worse. Descent OFF, the same
// sampler reports a MISCOMPILE on that seed — a flex map captured by a mount
// handler loses its identity across loop iterations, and the compiled run
// raises `[boru/set_error] expected a FlexMap, got FlexMap` where the
// interpreter round-trips cleanly. It had been pinned since 2026-07-30. Descent
// ON, that seed refuses instead. A refusal is a program that does not run; a
// miscompile is a program that runs and lies, and this project's rule — stated
// in every increment of this work — is that a fix producing a WRONG value is
// worse than one producing none.
//
// So the three refusal buckets are ledgered rather than laundered
// (varyRefusalLedger, with frontier rows), and the emit-state isolation above
// is the named work that retires them. What is NOT acceptable is leaving the
// miscompile in place to protect a refusal count.
//
// (An earlier note here blamed the clone contract — "StampFnValue clones, so
// the user's spec value stays plain". That reading was wrong: the clone
// protects a CALLER's input from the model module's stamp, not the compile
// pass from stamping its own baked const. The real collision it caused was
// ATTRIBUTION, and that is fixed at StampFnValue's already-stamped return.)
func (es *EmitState) stampFnConst(v core.Value) { es.stampFnConstAt(v, 0) }

// stampFnConstDepth bounds the container walk. A fn nested deeper than this
// inside a const is not a shape the corpus writes, and an unbounded walk over
// a value graph is how a compile pass acquires a pathological case.
const stampFnConstDepth = 8

// stampFnConstType stamps the fn values a TYPE NODE carries — today a class's
// field DEFAULTS.
//
// The node reaching the type-operand path is BARE (IsBareTypeNode means
// Data == nil), so the schema is not on the value: it has to be resolved the
// way `make` resolves it, through ResolveTypeLiteralDef against the live
// registry. That resolution is the whole reason this is a separate entry rather
// than another case inside the walk — the walk switches on v.Data, and here
// there is none.
func (es *EmitState) stampFnConstType(v core.Value) {
	if body := core.ResolveTypeLiteralDef(v, es.reg); body.Data != nil {
		es.stampFnConstAt(body, 1)
	}
}

func (es *EmitState) stampFnConstAt(v core.Value, depth int) {
	if depth > stampFnConstDepth || es.inStampCompile {
		return
	}
	switch d := v.Data.(type) {
	case core.ListPayload:
		for _, e := range d.Elems {
			es.stampFnConstAt(e, depth+1)
		}
		return
	case core.MapPayload:
		if d.M == nil {
			return
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			es.stampFnConstAt(mv, depth+1)
		}
		return
	case core.ClassTypeInfo:
		// A class type's FIELD DEFAULTS are a container of values exactly as a
		// map literal is; `make` copies a concrete default into the instance,
		// sharing the fn's *BoruImpl, so a stamp here is the stamp the instance
		// reads. The declared-type entries are bare type nodes and fall out of
		// the fn test below.
		//
		// The PARENT CHAIN is walked with it, because a subclass INHERITS its
		// parent's defaults rather than copying them: `def D refine C {}` holds
		// an empty own Fields and reaches C's `op` only through Parent, so
		// stopping at this node would leave every inherited method islanded.
		//
		// Walked by hand rather than through AllFields, which answers the same
		// question: AllFields MEMOISES onto the parent node it walks, and this
		// is a compile pass reading a type that concurrent forks may share. A
		// read-only walk owes them nothing. Fields is dereferenced unguarded for
		// the same reason AllFields dereferences it unguarded — a class node
		// always has one.
		for info := &d; info != nil; info = info.Parent {
			for _, k := range info.Fields.Keys() {
				fv, _ := info.Fields.Get(k)
				es.stampFnConstAt(fv, depth+1)
			}
		}
		return
	}
	fd, isFn := v.Data.(core.FnDefInfo)
	if !isFn || !core.IsConcrete(v) || len(fd.Captured) > 0 {
		return
	}
	// A MODIFIER WRAPPER is a container too — of exactly one fn — and the walk
	// above cannot see it because the wrapper is not a list or a map.
	//
	// `usurp` / `stack-args` / `forward-args` / `force-arity` REBUILD the value:
	// each own sig gets a GO handler that re-dispatches the original, so
	// storedSigEligible refuses every one of them and the loop below stamps
	// nothing. The boru body is one level down, behind FnDefInfo.Wraps, which is
	// where the value keeps the fn it modifies.
	//
	// Measured: `def sub2 fn […]  def ops {rev: (usurp sub2)}  ops.rev 10 3`
	// disassembles to `fns=0` — the map const folds the usurped value in and
	// NOTHING in the program compiles sub2's body, because sub2 is never called
	// by name either. So the apply had no unit to enter no matter how the apply
	// site was widened, and the two usurp.tsv census rows were a STAMP-REACH
	// hole, not an apply-kernel one.
	//
	// The recursion stamps the INNER value, which is what eng's
	// UnwrapModifierChain hands dynApplyEnter at run time; the wrapper's own
	// sigs stay unstamped, correctly — there is no boru body there to run.
	// AFTER the guard above, not before it, and the reason is a coverage fact
	// rather than a preference: splitting that guard to run this first leaves
	// its `!isFn` arm and its concrete/capturing arm in separate blocks, and
	// nothing in the corpus reaches the second one — a wrapper is concrete and
	// captures nothing, and a wrapper over a CLOSURE wraps a value with no
	// stampable sigs anyway. One guard, one covered block, same reach.
	if fd.Wraps != nil {
		es.stampFnConstAt(*fd.Wraps, depth+1)
	}
	for si := range fd.Signatures {
		if !storedSigEligible(&fd.Signatures[si]) {
			continue
		}
		impl, implOK := fd.Signatures[si].Impl.(*core.BoruImpl)
		if !implOK || impl.Compiled != nil || es.stampDeclined[impl] {
			continue // first stamp wins; a refusal is remembered, not re-paid
		}
		// EVERY unit the stamp compile creates is stamp-only, not just the
		// entry one: a stamped body that defines a nested fn records that as a
		// SUB-unit whose lowering can refuse where the entry unit's did not.
		// Flagging only the entry left two corpus rows refusing on `fn vt-body:
		// branch leaves extra values` — the same fatality one level down.
		// DIAGNOSTIC ISOLATION. compileStoredFnUnit analyses the body against
		// the LIVE check state, so a body whose analysis emits a finding leaks
		// it into the enclosing program — and an error diagnostic REFUSES that
		// program. Measured: `import "boru:math-util"  def n 0  if (n eq 0)
		// MathUtil.sqrt [99] 16` went from compiling to "refused: check
		// diagnostics" the moment fn consts started stamping.
		//
		// The store-fn edge never hit this because it stamps a curated
		// population (CompileStoresFn slots); the const chokepoint sees every
		// fn value in the program. A stamp is a speculative compile of code the
		// program may never apply, so its findings are not the program's —
		// TruncateDiagnostics drops exactly the ones this attempt added, the
		// same seam the bounded fixed-point analyses use.
		diagsBefore := len(es.reg.Check.Diagnostics)
		unitsBefore := len(es.fnRecs)
		// dynScopeNames is the OTHER channel the stamp writes through. A body
		// whose free word cannot resolve locally takes dynScopeRescue, which
		// adds that name here — and Finalize then installs an OpBindDynScope
		// twin in every BINDING unit, so the ENCLOSING program's own `def` of
		// that name must lower a dynamic bind it may have no promoted value
		// for. Measured: `def files (flex {})` + a mount handler reading
		// `files` went from compiling to "dynamic-scope def `files` of
		// unpromoted computed value" the moment the handler stamped.
		//
		// A successful stamp genuinely NEEDS the name (its unit reads through
		// OpLookupDynScope), so this cannot be restored under a live ref. The
		// rule instead: a stamp that would change the enclosing program's own
		// lowering is not worth taking. Decline it, restore the map, and let
		// the apply island — the behaviour the program had before anything
		// stamped, which is what the differential validates.
		//
		// maps.Clone, not a hand-rolled copy loop: the corpus has no program
		// whose dynScopeNames is non-empty at a const-stamp site, so a copy
		// loop's BODY is an unreachable statement. Cloning says the same thing
		// in one always-executed call — the general restore below is unchanged,
		// and nothing here assumes the map starts empty.
		dynBefore := maps.Clone(es.dynScopeNames)
		unit, ok := es.compileStoredFnUnit(fd, si, v.Pos())
		es.reg.Check.TruncateDiagnostics(diagsBefore)
		if ok && len(es.dynScopeNames) != len(dynBefore) {
			for n := range es.dynScopeNames {
				if !dynBefore[n] {
					delete(es.dynScopeNames, n)
				}
			}
			ok = false
		}
		if !ok {
			if es.stampDeclined == nil {
				es.stampDeclined = map[*core.BoruImpl]bool{}
			}
			es.stampDeclined[impl] = true
			es.recordStamp(core.StampEvent{Name: fd.Name, Pos: v.Pos(), Reason: es.storedFnProbeReason})
			continue
		}
		ref := &CompiledFnRef{Unit: unit, depNames: es.storedHandlerDeps(fd.Signatures[si].Body()), optional: true}
		impl.Compiled = ref
		es.storedFnRefs = append(es.storedFnRefs, ref)
		if es.stampImpls == nil {
			es.stampImpls = map[*core.BoruImpl]int{}
		}
		es.stampImpls[impl] = unit
		for u := unitsBefore; u < len(es.fnRecs); u++ {
			if es.fnRecs[u] != nil {
				es.fnRecs[u].stampOnly = true
			}
		}
		es.recordStamp(core.StampEvent{Name: fd.Name, Pos: v.Pos(), Stamped: true})
	}
}

// recordStamp files this stamp attempt in the registry's attribution log, so
// -compile-report and the stamp-report gates see it.
//
// It is not bookkeeping. stampFnConst runs at the const chokepoint, which is
// EARLIER than the store-site and model-action stamps, and first-stamp-wins
// means those then skip. Without recording here, a value this stamped would
// silently lose the attribution its own path used to produce — measured, as
// TestFrontierLedger's "no stamp attempt recorded for gen".
func (es *EmitState) recordStamp(ev core.StampEvent) {
	if es != nil && es.reg != nil {
		es.reg.RecordStampEvent(ev)
	}
}

// dropStampRef clears the compiled ref that pointed at a stamp-only unit whose
// lowering refused, so the fn value falls back to the interpreter exactly as it
// would have if nothing had stamped it.
func (es *EmitState) dropStampRef(unit int) {
	for impl, u := range es.stampImpls {
		if u == unit {
			impl.Compiled = nil
			delete(es.stampImpls, impl)
			return
		}
	}
}

// storedHandlerDeps returns the MODULE-LEVEL names a stored handler / spawn body
// reads — every body word bound as a user `def` at the store site. These are the
// names whose later undef/redefinition (NotifyNameRebound) makes the frozen unit
// stale, so the ref is left unstamped and falls back to CallBoru. Kernel natives
// and the handler's own params/locals are excluded: the store site sits OUTSIDE
// the handler frame, so those are not in r.Defs here. nil when the body reads no
// module-level def.
func (es *EmitState) storedHandlerDeps(body []core.Value) map[string]bool {
	if es == nil || es.reg == nil {
		return nil
	}
	var deps map[string]bool
	core.WalkBodyWords(body, func(w core.WordInfo, _ core.Value) {
		if w.Name == "" || deps[w.Name] {
			return
		}
		if _, ok := es.reg.Defs.Top(w.Name); ok {
			if deps == nil {
				deps = map[string]bool{}
			}
			deps[w.Name] = true
		}
	})
	return deps
}

// NotifyNameRebound poisons any already-created stored-handler / spawn ref whose
// body reads `name`: a def or undef of that name AFTER the ref was created
// changes what the interpreter resolves at CALL time, but the compiled unit is
// frozen at the store-site definition. Poisoned refs are left unstamped at
// Finalize, so InvokeCallback falls back to CallBoru. A def/undef processed BEFORE
// a ref exists poisons nothing (the ref is not in storedFnRefs yet), so a handler
// over stable module helpers (todo-api's live-todos, mini-redis's arg-at/kv-read,
// never redefined) still compiles.
func (es *EmitState) NotifyNameRebound(name string) {
	if es == nil || !es.Compilable {
		return
	}
	// THE FROZEN-READ LATCH RUNS FIRST, above the Active() gate, and that
	// placement is the whole of NUR117's fix. A rebind written inside a `do`
	// body reaches this function with the recorder SUSPENDED — KeepDefsBodyGuard
	// suspends for the body run — so the Active() gate below returned before
	// the latch was ever consulted, and the program compiled against a unit
	// still holding the old bake:
	//
	//	def k 5  def f fn [[] [Integer] [k add 2]]  f  do [def k 9]  f
	//	  interpreted 7 11        compiled 7 7
	//
	// NUR117 recorded the cause as the `len(openUnitRecs) == 0` guard below.
	// Measured, that reading was wrong: at the deciding call openUnitRecs is
	// EMPTY and `suspended` is 1, so the guard would have passed and the
	// early return is what exempted the rebind. The record is corrected.
	//
	// Everything below this block still needs Active(): the stored-ref
	// poisoning walks refs whose recording this suspension is not part of.
	//
	// Since Stage 4b the latch fires only for a bake held by an ESCAPING unit
	// (unit_memo.go): every other unit is re-recorded at its next call site
	// by the binding-sensitive memo, so a rebind is no longer a hazard for
	// it. An escaping unit's later "call" is an apply of the value it
	// escaped into, which the memo cannot see, so for those the refusal —
	// same text — is still the only sound answer.
	if bake, frozen := es.frozenInEscapingUnit(name); frozen && es.rebindReachesModuleScope() {
		es.MarkUncompilable("module binding " + name + " rebound after a fn unit baked its " + bake.String())
	}
	if !es.Active() {
		return
	}
	depHit := false
	for _, ref := range es.storedFnRefs {
		if ref.depNames[name] {
			ref.poisoned = true
			// An OPTIONAL ref (stampFnConst's) does not escalate to the
			// program-level refusal below. Poisoning already dropped it, and
			// its fallback is the island the program used before the stamp
			// existed — the behaviour the differential validates. Refusing the
			// whole program because an OPTIMISATION could not survive a rebind
			// is the same trade fnUnitRec.stampOnly rejects at Finalize.
			//
			// Measured: without this, container descent turned a compiling
			// sampler seed into "module binding files rebound after a stored
			// handler captured it as a dep" — a refusal caused entirely by the
			// stamp having happened.
			if !ref.optional {
				depHit = true
			}
		}
	}
	// Poisoning alone is NOT enough for a module-scope rebind: the poisoned
	// ref's CallBoru fallback resolves the LIVE def table, but module-scope
	// def sites execute only in the compile pass (RunInCheck) — by VM time
	// the table already holds the PASS-FINAL binding, so every call
	// (including calls sequenced BEFORE the rebind in program order) reads
	// the final value where the interpreter reads the point-in-program one
	// (design/RELOAD-INVALIDATION.0.md §3 F1: interpreter 6 105 12,
	// compiled-with-poisoning 12 12 12). The prior discipline's cases —
	// a single call AFTER the last rebind — coincide with pass-final state,
	// which is why per-ref poisoning looked sufficient. Refuse the whole
	// program (interpreter fallback, correct values) until the §5.6 bind
	// twins make VM-time def order real. Same module-scope guard as the
	// frozen-read hammer below: a body-local def inside another unit's
	// analysis shadows independently and must not refuse.
	if depHit && len(es.openUnitRecs) == 0 {
		es.MarkUncompilable("module binding " + name + " rebound after a stored handler captured it as a dep")
	}
	// A splice-expanded binding (expandStaticSplices) is FROZEN inside an
	// OpPushClosure unit, which — unlike a spawn ref — cannot be unstamped
	// post-hoc. Rebinding it means the interpreter would re-resolve where the
	// unit holds the old tokens: refuse the whole program (interpreter
	// fallback) rather than diverge. Macro-style defs never rebind in
	// practice, so this hammer stays cold on real programs.
}

// rebindReachesModuleScope reports whether a rebind seen RIGHT NOW is one the
// runtime will perform against the module-scope binding set — the question the
// frozen-read latch has to answer, and the one NUR117 got wrong by asking it
// with a single `len(openUnitRecs) == 0` test.
//
// Three regimes, and two of them leak:
//
//   - NOT SUSPENDED, no unit open. The ordinary top-level rebind. Every call
//     the latch already caught before NUR117 lands here.
//   - SUSPENDED BY A LEAKING BODY RUN AT MODULE SCOPE. A `do` body, or an
//     each/fold body: the recorder suspends for the body run, but the body's
//     defs LEAK to the enclosing scope — a `do`'s by its keep-defs scoping, a
//     multi-run body's as its last iteration's binding — so the rebind is as
//     real as a top-level one. The equality `suspended == keepModuleDepth +
//     multiRunModuleDepth` is what says every suspension in flight is one of
//     those: a fn body, a branch arm, or any other guard nested inside raises
//     `suspended` without raising either counter, and the equality fails. A
//     `do` or an `each` inside a FN body raises neither (both publication
//     gates test FnBodyDepth), so both are correctly excluded.
//   - ANY OTHER OPEN UNIT. A body-local def inside another unit's analysis
//     shadows independently and must not refuse — the original guard's own
//     case, kept verbatim.
//
// The equality makes this ADDITIVE: an unrelated suspension in flight makes it
// answer false, so the change can only ever add the `do`-body refusals it is
// for, never remove one the latch already made.
func (es *EmitState) rebindReachesModuleScope() bool {
	if es == nil || len(es.openUnitRecs) != 0 {
		return false
	}
	return es.suspended == es.keepModuleDepth+es.multiRunModuleDepth
}

// OperandRepushable reports whether v resolves to a FREELY RE-PUSHABLE
// operand — a const, a frame local, or a (canonical) type node — as opposed
// to a computed EVENT result (on the simulated stack exactly once) or an
// unresolvable value. It mirrors resolveOperand's decision but is SIDE-
// EFFECT FREE (no interning), so a caller can classify an operand WITHOUT
// recording. Used by a multi-reference desugar (the `case` value is tested
// against every clause guard) to decide up front whether it can compile —
// avoiding a probe whose rollback would otherwise pollute the recording.
func (es *EmitState) OperandRepushable(v core.Value) bool {
	if es == nil {
		return false
	}
	// An event operand is the value's stack-discipline truth (pushed once);
	// it cannot be re-pushed for a second reference.
	if pr, ok := es.producedBy[v.ID]; ok && (!core.IsTypeBody(v) || es.eventInfo[pr.seq].typeOut) {
		return false
	}
	if _, ok := es.units[len(es.units)-1].localByID[v.ID]; ok {
		return true
	}
	if core.IsBareTypeNode(v) && v.ID != "" {
		return true
	}
	lit, ok := es.Materialise(v)
	return ok && core.IsInertConst(lit)
}

// CanSeatAcrossFragment reports whether v can be read INSIDE a branch / loop
// fragment that a multi-reference desugar is about to record. Either v is
// already re-pushable (OperandRepushable: a const, frame local, or type node),
// OR it is a COMPUTED single-result event — which planValueDefLocals promotes to
// a frame local once a fragment read is seen, reachable across the scope floor.
// The promotion now runs for EVERY unit (the top-level program AND each fn body
// — see the planValueDefLocals call in StartFnCompile's finish), so a computed
// scrutinee inside a fn unit (a `case` in an `error`/`do` handler closure) seats
// too. A non-repushable computed event reaching here is always produced in the
// CURRENT unit — an enclosing-scope value is a capture, hence a local (repushable
// above) — so its promotion lands in the right unit's slots. Side-effect free,
// like OperandRepushable — the promotion itself happens later, driven purely by
// the fragment reference the desugar then records.
func (es *EmitState) CanSeatAcrossFragment(v core.Value) bool {
	if es == nil {
		return false
	}
	if es.OperandRepushable(v) {
		return true
	}
	pr, ok := es.producedBy[v.ID]
	return ok && pr.idx == 0 && (!core.IsTypeBody(v) || es.eventInfo[pr.seq].typeOut)
}

// materialise recovers the fully concrete value behind a stripped
// literal: the value itself, its RememberOriginal original, or — for a
// concrete container whose MEMBERS were stripped by a sub-engine run
// (autoEvalMap evaluates each field through Run, which strips) — a
// rebuilt copy with each carrier member replaced by its recorded
// original, recursively. ok=false when any member's original is
// unknown.
func (es *EmitState) Materialise(v core.Value) (core.Value, bool) {
	if v.Carrier || v.Dynamic {
		orig, ok := es.origByID[v.ID]
		if !ok {
			return v, false
		}
		// Inside a compiled fn frame an ACTIVE-token original (a word, a
		// paren, an interpolation, a splice/reach) must not bake: the
		// interpreter re-evaluates it per dispatch against the live frame
		// (a param named by the word — `call {line: src} …` inside
		// repl-eval), which the VM holds as a frame local the baked const
		// can never see. Module scope keeps the recovery (code-as-data
		// consts re-run against the registry, where resolution is
		// identical).
		return orig, true
	}
	switch d := v.Data.(type) {
	case core.ListPayload:
		elems := d.Elems
		rebuilt := false
		for i, e := range d.Elems {
			m, changed, ok := es.materialiseMember(e)
			if !ok {
				return v, false
			}
			if changed {
				if !rebuilt { // copy-on-first-change, then patch in place
					elems = append([]core.Value(nil), d.Elems...)
					rebuilt = true
				}
				elems[i] = m
			}
		}
		if rebuilt {
			nv := v
			nv.Data = core.ListPayload{Elems: elems}
			return nv, true
		}
		return v, true
	case core.MapPayload:
		if d.M == nil {
			return v, false
		}
		keys := d.M.Keys()
		var nm *core.OrderedMap
		for i, k := range keys {
			mv, _ := d.M.Get(k)
			m, changed, ok := es.materialiseMember(mv)
			if !ok {
				return v, false
			}
			if changed && nm == nil { // copy the unchanged prefix, then this member
				nm = core.NewOrderedMap()
				for _, pk := range keys[:i] {
					pv, _ := d.M.Get(pk)
					nm.Set(pk, pv)
				}
			}
			if nm != nil {
				nm.Set(k, m)
			}
		}
		if nm != nil {
			nv := v
			nv.Data = core.MapPayload{M: nm}
			return nv, true
		}
		return v, true
	}
	return v, true
}

func (es *EmitState) materialiseMember(e core.Value) (m core.Value, changed, ok bool) {
	m, ok = es.Materialise(e)
	if !ok {
		return e, false, false
	}
	return m, m.Carrier != e.Carrier || m.ID != e.ID, true
}

// fragDiverges reports whether a captured fragment's trailing event
// is a flow-control terminator — the arm never reaches the branch
// join (its lowering emitted the loop jump).
func fragDiverges(frag *EmitFragment) bool {
	if frag == nil || len(frag.events) == 0 {
		return false
	}
	last := &frag.events[len(frag.events)-1]
	if last.kind == evCallUser && last.uc.tail {
		return true
	}
	if last.kind == evCall && last.call.diverges {
		// A CompileDiverges word (raise) always raises: the fragment never
		// produces a residual past it, so it diverges like break/continue. A
		// closure body ending here compiles with no RET; the error propagates
		// out of the VM run and the catching word (do/error) wraps it.
		return true
	}
	return last.kind == evBreak || last.kind == evContinue
}

// fragDivergesDeep reports whether a LOWERED fragment definitely leaves the
// enclosing construct on every reachable path: break, continue, a tail call,
// or a branch all of whose reachable arms do. Unlike fragDiverges (a shallow
// last-event check), it recurses through branch arms, so a fully-diverging
// NESTED branch — e.g. a const-condition branch whose taken arm tail-calls —
// is seen as divergent by the variadic-merge accounting. Sound only AFTER
// markTailCalls has run (it reads the tail flags set there); the only caller
// is lowerArms, in Finalize's lowering pass.
func fragDivergesDeep(frag *EmitFragment) bool {
	if frag == nil || len(frag.events) == 0 {
		return false
	}
	return eventDivergesDeep(&frag.events[len(frag.events)-1])
}

func eventDivergesDeep(ev *EmitEvent) bool {
	switch ev.kind {
	case evBreak, evContinue:
		return true
	case evCall:
		// A CompileDiverges word (raise) never returns past this call — the
		// same divergence the shallow fragDiverges recognises. Without this,
		// an arm ending in `raise` is mis-seen as a 0-value merge contributor
		// (a false variadic merge), so a fn whose if-chain bottoms out in
		// `raise` reports a spurious variadic return.
		return ev.call.diverges
	case evCallUser:
		return ev.uc.tail
	case evBranch:
		if ev.br.constCond != nil {
			// Only the taken (then) arm is reachable.
			return fragDivergesDeep(ev.br.then)
		}
		switch ev.br.elseArm() {
		case armAbsent:
			return false // the implicit false path falls through to the merge
		case armValue, armComputed:
			return false // a value / computed else arm never diverges
		}
		// Both arms are bodies: the branch diverges only if both do.
		return fragDivergesDeep(ev.br.then) && fragDivergesDeep(ev.br.els)
	}
	return false
}

// The argument struct — core.BranchRecord — is declared in core beside the
// EmitRecorder interface, not here: basic's `if` handler fills one in, and it
// must be able to do that without importing this module. Its fragment fields
// are core.EmitFragmentRef (an opaque `any`); asFragment below converts them
// back to *EmitFragment, which is the one place that knows the concrete type.

// RecordBranch records an `if` dispatch: condition (pre-evaluated,
// list-form fragment, or statically known), the captured arm
// fragments and their residual stacks, and the dispatch's joined
// result carrier. An arm may DIVERGE (end in break/continue) — it
// then contributes no value and never reaches the join. The 2-arg
// form (HasElse=false) has a VARIADIC result (0 or 1 values), which
// only the program residual may absorb. Any shape Stage 2 cannot
// lower marks the program uncompilable.
// stripZeroOutPhantoms drops 0-output statement guards' phantom (None)
// results from a residual stack. The program and fn-body residual
// reconciliations skip zeroOut phantoms; arm residuals (RecordBranch) and a
// loop body's per-iteration residual (RecordLoop) must too — otherwise the
// phantom reads as the merge / body-out value and the lowerer refuses with
// "branch leaves extra values" (out=opEvent, vm=0).
func (es *EmitState) stripZeroOutPhantoms(stk []core.Value) []core.Value {
	kept := stk
	for i, rv := range stk {
		pr, ok := es.producedBy[rv.ID]
		if ok && es.eventInfo[pr.seq].zeroOut {
			// First phantom found: copy the prefix and filter the rest.
			kept = append([]core.Value(nil), stk[:i]...)
			for _, r := range stk[i:] {
				if p, o := es.producedBy[r.ID]; o && es.eventInfo[p.seq].zeroOut {
					continue
				}
				kept = append(kept, r)
			}
			break
		}
	}
	return kept
}

// asFragment converts core's opaque fragment handle back to this package's
// concrete fragment. The assertion lives HERE, in the package that owns the
// type — that is the whole point of the seam. A nil or foreign ref yields nil,
// which every arm below already treats as "not captured".
func asFragment(ref core.EmitFragmentRef) *EmitFragment {
	f, _ := ref.(*EmitFragment)
	return f
}

func (es *EmitState) RecordBranch(b core.BranchRecord) {
	if !es.Active() {
		return
	}
	bThen, bEls, bCondFrag := asFragment(b.Then), asFragment(b.Els), asFragment(b.CondFrag)
	// Strip 0-output statement guards' phantom (None) results from the arm
	// residuals BEFORE any counting. A nested both-arms-void `if` (the welford
	// `if … [set … s] [if … [set … s] []]` shape) registers a phantom result the
	// lowerer never seats; the program and fn-body residual reconciliations
	// already skip zeroOut phantoms, and an ARM must too — otherwise resolveArm
	// reads the phantom as the arm's merge value and the lowerer refuses with
	// "branch leaves extra values" (out=opEvent, vm=0). Stripping here keeps
	// resolveArm, residualN, and branchVariadicResult consistent, and lets the
	// both-arms-net-zero → zeroOut marking cascade through nested guards.
	b.ThenStk = es.stripZeroOutPhantoms(b.ThenStk)
	b.ElsStk = es.stripZeroOutPhantoms(b.ElsStk)
	ev := EmitEvent{kind: evBranch, br: &emitBranch{
		constCond: b.ConstCond, hasElse: b.HasElse, pos: b.Pos,
	}}
	resolveArm := func(frag *EmitFragment, stk []core.Value, name string) (EmitOperand, bool, bool) {
		if frag == nil {
			es.MarkUncompilable("if: " + name + "-branch not captured")
			return EmitOperand{}, false, false
		}
		if fragDiverges(frag) {
			return EmitOperand{}, false, true
		}
		if len(stk) == 0 {
			// A 0-value arm (an empty `[]`, a 0-value word, or a raise that
			// fragDiverges doesn't classify): no merge value, like a diverging
			// arm. When the SIBLING arm nets a value the branch is VARIADIC
			// (0-or-1) — lowerArms marks it and only the program residual
			// absorbs it; when BOTH arms net 0 the caller refuses below.
			return EmitOperand{}, false, true
		}
		op, ok := es.resolveOperand(stk[len(stk)-1])
		if !es.residualStands("if: "+name+"-branch ", stk[len(stk)-1], op, ok && !residualLeadReStepped(stk), frag, "result") {
			return EmitOperand{}, false, false
		}
		return op, true, true
	}
	// Condition.
	if b.ConstCond == nil {
		switch {
		case bCondFrag != nil:
			if fragDiverges(bCondFrag) || len(b.CondStk) == 0 {
				es.MarkUncompilable("if: condition body produces no value")
				return
			}
			op, ok := es.resolveOperand(b.CondStk[len(b.CondStk)-1])
			if !ok {
				es.MarkUncompilable("if: condition result of unknown provenance")
				return
			}
			ev.br.condFrag, ev.br.condOut = bCondFrag, op
		default:
			condOp, ok := es.resolveOperand(b.Cond)
			if !ok {
				es.MarkUncompilable("if: condition of unknown provenance")
				return
			}
			ev.br.cond = condOp
		}
	}
	// Arms.
	zeroOut := false
	// mayBeFn: an arm is an fn VALUE, so the branch result may be callable at
	// run time — a trailing residual arg over it is a conditional apply.
	mayBeFn := false
	if b.ConstCond != nil {
		out, has, ok := resolveArm(bThen, b.ThenStk, "taken")
		if !ok {
			return
		}
		ev.br.then, ev.br.thenOut, ev.br.hasThenOut = bThen, out, has
	} else if !b.HasElse && (len(b.ThenStk) == 0 || fragDiverges(bThen)) {
		// 2-arg if (no else) whose then produces 0 values — a 0-value word
		// (raise/set/printstr) or a diverging arm (break/continue/raise): the
		// if produces 0 values on BOTH paths (true→0/diverge, false→0), so it
		// is a statement guard with NO merge value. Record the then for
		// lowering (its body still runs on the true path) and mark the event
		// zeroOut: the lowerer emits no slot, and Finalize skips the (phantom
		// None) result it still registers below — the registration is kept so
		// RecordCall's double-record guard still elides this if dispatch.
		if bThen == nil {
			es.MarkUncompilable("if: then-branch not captured")
			return
		}
		ev.br.then, ev.br.hasThenOut = bThen, false
		zeroOut = true
	} else {
		var hasThen bool
		if b.ThenValue != nil {
			// Value-then: the then arm is an already-evaluated value
			// (`if cond 99 88`), not a `[…]` body — it lowers to a single push in
			// the then arm, mirroring the value-else below. A COMPUTED then (an
			// event result eagerly on the stack) would need the stack juggling the
			// single-result lowering does not do, so refuse it.
			op, ok := es.resolveOperand(*b.ThenValue)
			if !ok {
				es.MarkUncompilable("if: then value of unknown provenance")
				return
			}
			if core.IsFnValueResidual(*b.ThenValue) {
				mayBeFn = true
			}
			if op.kind == opEvent {
				// A COMPUTED then value (`if cond (add 1 2) 88`) is eagerly on the
				// stack BELOW the cond event — the mirror of the computed-else case.
				// The lowerer SWAPs the cond above it, branches, and DROPs it on the
				// FALSE path (it survives on the TRUE path as the result). Only the
				// plain-event-cond layout [cond, thenVal] is handled; a const /
				// condFrag / const-cond condition sits elsewhere, so refuse those.
				if !computedArmCondOK(b, ev.br.cond) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
					es.MarkUncompilable("if: computed then value with non-stack condition (Stage 2)")
					return
				}
				ev.br.thenIsVal, ev.br.thenVal, ev.br.hasThenOut, ev.br.thenComputed, hasThen = true, op, true, true, true
			} else {
				ev.br.thenIsVal, ev.br.thenVal, ev.br.hasThenOut, hasThen = true, op, true, true
			}
		} else {
			thenOut, h, ok := resolveArm(bThen, b.ThenStk, "then")
			if !ok {
				return
			}
			ev.br.then, ev.br.thenOut, ev.br.hasThenOut, hasThen = bThen, thenOut, h, h
		}
		if b.HasElse {
			if b.ElsValue != nil {
				// Value-else: the else arm is an already-evaluated value
				// (`if cond [then] 42`), not a `[…]` body. It lowers to a
				// single push in the else arm. A COMPUTED else (an event
				// result of a paren like `(add 1 2)`) is eagerly on the
				// stack BEFORE the branch — that needs stack juggling the
				// current single-result lowering doesn't do, so refuse it.
				op, ok := es.resolveOperand(*b.ElsValue)
				if !ok {
					es.MarkUncompilable("if: else value of unknown provenance")
					return
				}
				if core.IsFnValueResidual(*b.ElsValue) {
					mayBeFn = true
				}
				if op.kind == opEvent {
					// A COMPUTED else value (`if cond [then] (add 1 2)`) is
					// eagerly on the stack below the cond/then events. The lowerer
					// branches and DROPs the unselected eager value(s).
					if ev.br.thenComputed {
						// `if cond (a) (b)` — BOTH arms computed. An EVENT cond
						// stacks [cond, then, else] (lowerBothComputed's
						// OpReverse select); a condFrag / const / local cond
						// (computedArmCondOK's wider rule — widened 2026-07-17,
						// the S9.4 probe sweep) has no eager stack home, so the
						// lowering MATERIALISES it above the eagers exactly as
						// the single-computed lowerComputedCond does. Anything
						// else (a ConstCond fold) refuses.
						if !computedArmCondOK(b, ev.br.cond) { //covergate:allow the only non-OK shape is a ConstCond, which constant-branch-elimination folds to one arm before both-computed lowering builds — unreachable without a recorder fault; the two single-computed twins carry the identical allow (§compiler)
							es.MarkUncompilable("if: both computed arms need an event condition (Stage 2)")
							return
						}
						ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut, ev.br.elsComputed = true, op, true, true
					} else {
						// Single computed else: only the plain-event-cond layout
						// [cond, elseVal] (SWAP), a list-form cond (inline), or a
						// const/local cond (pushed) is handled — computedArmCondOK.
						if !computedArmCondOK(b, ev.br.cond) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
							es.MarkUncompilable("if: computed else value with non-stack condition (Stage 2)")
							return
						}
						ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut, ev.br.elsComputed = true, op, true, true
					}
				} else {
					ev.br.elsIsVal, ev.br.elsVal, ev.br.hasElsOut = true, op, true
				}
			} else {
				elsOut, hasEls, ok := resolveArm(bEls, b.ElsStk, "else")
				if !ok {
					return
				}
				ev.br.els, ev.br.elsOut, ev.br.hasElsOut = bEls, elsOut, hasEls
				if !hasThen && !hasEls {
					// Both arms produce 0 values — an empty `[]`, a 0-value word
					// (set/printstr), or a diverging break/continue/raise on BOTH
					// sides: the if is a 0-value STATEMENT on every path. Record it
					// zeroOut (no merge slot, like the 2-arg no-else guard) rather
					// than refusing; both arm fragments still lower, so their effects
					// and divergence run.
					zeroOut = true
				}
			}
		}
	}
	// Record each body arm's true (carrier) residual count so the lowerer can
	// distinguish a GENUINE multi-value arm from a single-value expression whose
	// lowering leaves an extra sim artifact (see EmitFragment.residualN).
	if ev.br.then != nil {
		ev.br.then.residualN = len(b.ThenStk)
		es.captureArmResidual(ev.br.then, b.ThenStk)
	}
	if ev.br.els != nil {
		ev.br.els.residualN = len(b.ElsStk)
		es.captureArmResidual(ev.br.els, b.ElsStk)
	}
	seq := es.appendEvent(ev)
	es.SiteCounts[SiteMono]++
	es.setProduced(b.Out, seq)
	if zeroOut {
		// The if produces 0 runtime values; its registered (None) result is a
		// phantom. Mark the seq so Finalize's residual reconciliation skips it
		// rather than expecting a stack slot. Keeping the setProduced above
		// lets RecordCall's double-record guard elide this if dispatch.
		f := es.eventInfo[seq]
		f.zeroOut = true
		es.eventInfo[seq] = f
	}
	if mayBeFn {
		f := es.eventInfo[seq]
		f.mayBeFn = true
		es.eventInfo[seq] = f
	}
	if !zeroOut && es.branchVariadicResult(b) {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
}

// branchVariadicResult reports whether an `if` produces a RUNTIME-VARIABLE result
// count — the structural mirror of lowerArms' merge-variadic marking, computed at
// record time so a fn's variadic-return-ness is known before its call sites lower.
// Variadic when: a 2-arg if leaves 0-or-N; the two arms leave different
// non-diverging counts (`if c [] [a]`); either arm is multi-value (`if c [a b]
// [c]`); or either arm's own result is itself variadic (a nested variadic if).
// Over-marking is sound (it only refuses external fixed-arity consumption); a
// diverging arm never reaches the merge, so it does not count toward a mismatch.
func (es *EmitState) branchVariadicResult(b core.BranchRecord) bool {
	thenN, elsN := len(b.ThenStk), len(b.ElsStk)
	if b.ConstCond != nil {
		// Only the taken (then) arm is inlined; its result IS the branch result.
		return thenN > 1 || es.armOutVariadic(b.ThenStk)
	}
	if !b.HasElse {
		return thenN >= 1 // value on true, nothing on false → 0-or-N
	}
	if thenN > 1 || elsN > 1 {
		return true
	}
	thenFrag, elsFrag := asFragment(b.Then), asFragment(b.Els)
	thenDiv := thenFrag != nil && fragDiverges(thenFrag)
	elsDiv := elsFrag != nil && fragDiverges(elsFrag)
	// A diverging arm (raise / break / continue / tail) never reaches the
	// merge, so the surviving arm's count is unconditional. A runtime-variable
	// 0-or-1 result therefore arises only from a count MISMATCH between two
	// NON-diverging arms (`if c [99] []`); when either arm diverges the other's
	// fixed count governs. (Multi-value arms were rejected by thenN/elsN > 1
	// above; a nested-variadic surviving arm is caught by armOutVariadic below.)
	if !thenDiv && !elsDiv && (thenN > 0) != (elsN > 0) {
		return true
	}
	return es.armOutVariadic(b.ThenStk) || es.armOutVariadic(b.ElsStk)
}

// armOutVariadic reports whether an arm's result value (its residual top) is
// itself a variadic-producing event — a nested variadic if / loop the parent
// branch propagates up to its own merge.
func (es *EmitState) armOutVariadic(stk []core.Value) bool {
	if len(stk) == 0 {
		return false
	}
	pr, ok := es.producedBy[stk[len(stk)-1].ID]
	return ok && es.eventInfo[pr.seq].variadicResult
}

// computedArmCondOK reports whether the branch condition is one the computed-arm
// (eager-value) lowering can materialise as a Boolean on top of the eager value:
// an event cond (SWAPped up from just below the eager value), a list-form
// condition body (lowered inline), or a const / local / type cond (pushed). A
// statically-known (const-folded) condition is handled by the disjoint
// const-cond path, never here.
func computedArmCondOK(b core.BranchRecord, cond EmitOperand) bool {
	if b.ConstCond != nil {
		return false
	}
	if b.CondFrag != nil {
		return true
	}
	switch cond.kind {
	case opEvent, opConst, opLocal, opType:
		return true
	default:
		return false
	}
}

// ArmLoopCapture makes the NEXT AnalyseLoopBody record its final
// fixed-point round as a fragment with the loop bindings registered
// as locals — the loop-lowering hook (`for`). One-shot.
func (es *EmitState) ArmLoopCapture() {
	if !es.Active() {
		return
	}
	es.loopArm = true
}

// ConsumeLoopArm reports and clears the one-shot loop-capture flag.
func (es *EmitState) ConsumeLoopArm() bool {
	if es == nil || !es.loopArm {
		return false
	}
	es.loopArm = false
	return es.Active()
}

// RegisterLocal assigns (or returns) the local slot backing a loop
// binding's carrier, keyed by the carrier's value ID — body
// references to the binding resolve to PUSH_LOCAL.
func (es *EmitState) RegisterLocal(id string) int {
	if es == nil {
		return -1
	}
	// An identity-less value cannot own a local slot: registering two
	// ""-ID values would collapse them onto ONE slot (the documented
	// record-schema-carrier miscompile shape — see the eager ID mint in
	// core_helpers.go's record path). -1 mirrors the inactive recorder's
	// convention; every caller uses RegisterLocal for its side effect and
	// resolves slots later via localByID, where "" now always misses.
	if id == "" {
		return -1
	}
	u := es.units[len(es.units)-1]
	if slot, ok := u.localByID[id]; ok {
		return slot
	}
	slot := u.numLocals
	u.numLocals++
	u.localByID[id] = slot
	return slot
}

// BeginLoopCarried opens a carried-def scope for one armed loop analysis
// (AnalyseLoopBody with loop capture active). Pairs with EndLoopCarried.
func (es *EmitState) BeginLoopCarried() {
	if es == nil {
		return
	}
	es.loopCarried = append(es.loopCarried, &loopCarriedScope{
		unitDepth: len(es.units),
		slots:     map[string]int{},
	})
}

// EndLoopCarried closes the innermost carried-def scope, exposing its slot
// inits to the RecordLoop / RecordWhile that follows the analysis. The inits
// ACCUMULATE: a condition loop analyses its body and its condition as two
// scopes before one record claims them (RecordWhile), and a record claims
// and clears the pending inits up front, so nothing carries across loops.
func (es *EmitState) EndLoopCarried() {
	if es == nil || len(es.loopCarried) == 0 {
		return
	}
	top := es.loopCarried[len(es.loopCarried)-1]
	es.loopCarried = es.loopCarried[:len(es.loopCarried)-1]
	es.pendingCarried = append(es.pendingCarried, top.inits...)
}

// NoteLoopCarried registers one loop-body REBIND of a pre-existing def as
// loop-carried: the name gets a unit frame slot (reusing an enclosing armed
// loop's slot for the same name, so nested loops over one def share one
// cell), each round's JOINED binding ID resolves to that slot, and — for a
// fresh slot — the pre-loop value is queued as the slot's init, stored
// before the loop's first iteration. Called by AnalyseLoopBody at the end
// of every analysis round; the init operand is re-resolved each round
// because a discarded round's Rollback truncates the const pool the prior
// resolution interned into. A name whose pre-loop value cannot resolve, or
// whose value is function-shaped, is left unregistered — its
// cross-iteration reads then refuse at resolveOperand exactly as before
// (sound interpreter fallback).
func (es *EmitState) NoteLoopCarried(name string, joined, pre core.Value) {
	if !es.Active() || len(es.loopCarried) == 0 {
		return
	}
	scope := es.loopCarried[len(es.loopCarried)-1]
	if scope.unitDepth != len(es.units) {
		return
	}
	u := es.units[len(es.units)-1]
	slot, seen := scope.slots[name]
	if !seen {
		// An enclosing armed loop already carrying this name owns the cell —
		// reuse it (the outer slot is live and current at inner-loop entry;
		// a fresh inner slot with its own init would RESET the accumulation
		// every outer iteration). No init recorded on reuse.
		reused := false
		for i := len(es.loopCarried) - 2; i >= 0; i-- {
			outer := es.loopCarried[i]
			if outer.unitDepth != scope.unitDepth {
				break
			}
			if s, ok := outer.slots[name]; ok {
				slot, reused = s, true
				break
			}
		}
		if !reused {
			if core.IsFnValueResidual(pre) || core.IsFnValueResidual(joined) {
				return
			}
			init, ok := es.resolveOperand(pre)
			if !ok {
				return
			}
			slot = u.numLocals
			u.numLocals++
			scope.inits = append(scope.inits, carriedInit{slot: slot, init: init})
		}
		scope.slots[name] = slot
	} else {
		// Re-resolve the init on a repeat round so its const index targets
		// the pool that survives (a non-final round's Rollback truncated the
		// earlier interning). The pre-loop value is round-invariant, so a
		// resolution that now fails is a recording inconsistency — refuse
		// rather than bake a dangling operand.
		for i := range scope.inits {
			if scope.inits[i].slot != slot {
				continue
			}
			init, ok := es.resolveOperand(pre)
			if !ok {
				es.MarkUncompilable("loop-carried def `" + name + "`: pre-loop value no longer resolves")
				return
			}
			scope.inits[i].init = init
			break
		}
	}
	u.localByID[joined.ID] = slot
}

// RecordBindTwin appends one bind-ledger transition to the pass's twin table
// (§6.5's inert-emission stage; the table's contract is on the bindTwins
// field). UNCONDITIONAL by design — no Active()/Suspend gate: the twin
// population is the ledger's, NoteBindTransition already applied every
// suppression before calling here, and a second filter would be a second
// source of truth for the emission gate to drift against. In particular a
// transition noted while recording is SUSPENDED (an each/fold body run whose
// def genuinely leaks) still belongs to the table, exactly as it belongs to
// the ledger.
func (es *EmitState) RecordBindTwin(tr core.BindTransition, entry core.DefEntry) {
	if es == nil {
		return
	}
	es.bindTwins = append(es.bindTwins, tr)
	es.bindTwinEntries = append(es.bindTwinEntries, entry)
	// STREAM PLACEMENT, the narrower half: an evBindTwin event marks where in
	// production order the transition happened, so the lowering emits an
	// (inert) OpBindTwin there. Recorded only while the recorder is LIVE —
	// unlike the table append above, which is the census. A transition noted
	// while recording is suspended (an each/fold body's leaking def) has no
	// stream home until the twin becomes arm-resident, so it stays
	// table-only; the emission gate counts placed ops as a strictly-ordered
	// subset of the table for exactly this reason.
	if es.Active() {
		es.appendEvent(EmitEvent{kind: evBindTwin,
			twin: &emitBindTwin{idx: len(es.bindTwins) - 1, pos: tr.Pos}})
		es.twinPlaced = append(es.twinPlaced, true)
		// A live root install of an arm-bound name re-binds it: later reads
		// resolve THIS binding, so the arm-bound read refusal lifts.
		delete(es.armBoundNames, tr.Name)
	} else {
		es.twinPlaced = append(es.twinPlaced, false)
	}
}

// AdoptBodyTwins is the closure path's twin accounting, the §6.5-faithful
// recovery of the do-body class (replay, never re-execution). It runs
// right after a ONCE-RUN, DEFS-KEEPING body word (CallableSpec.
// BodyOnceKeepsDefs — `do`, whose check-mode body run is
// RunCarrierBodyKeepDefs and whose runtime defs LEAK to the enclosing
// scope) records its body as a compiled closure unit. The unit makes
// body defs frame-local, so the leak the interpreter delivers would be
// lost under the regime's rollback — the exact hole the full-placement
// gate refuses. Adoption closes it by PLACEMENT: every unplaced twin
// whose noted position is a token site inside this body's tree gets an
// evBindTwin event HERE, after the call event, so the lowered OpBindTwin
// replays the check pass's captured identical entry once the unit
// returns. That is observationally the interpreter's order at root
// granularity — nothing outside the do can read the registry between the
// body's own install and the dispatch's return. (Finer grain, and the
// one divergence this placement accepts: a unit that RAISES mid-body
// aborts before the twin op runs, so a leak-then-raise body leaves the
// name unbound where the interpreter leaves it bound — observable only
// by a later request on the same erroring instance, the post-trap-twin
// class the regime already tolerates at Finalize's trap truncation.)
//
// Adoption is FENCED four ways, and each fence answers a real
// miscompile (the first three are Codex's P1 round on #421):
//
//   - THE BRACKET: only twins inside lastKeepRange — the ones noted by
//     this dispatch's own outermost keep-defs body run — are eligible.
//     Without it, token ALIASING lets a do adopt an earlier multi-run
//     driver's twin noted at the same sites (`def q quote [def x 5]
//     ([1 2] each q) q do`): the each's runtime installs twice where
//     one generalized note exists, so replaying it once under-counts;
//     bracketed, the each's twins stay unplaced and the regime refuses
//     the program (sound fallback).
//   - THE TAINT: sub-ranges noted under a nested NON-keep body run
//     (lastKeepTaints) are excluded even inside the bracket. A nested
//     each/fold inside the do body (`do [[1 2] each [def x 5]]`) runs
//     its body per element at runtime while the check pass noted one
//     generalized transition — a single replay is wrong in count, so
//     the twin stays unplaced and the regime refuses. A nested KEEP run
//     (do inside do) is not tainted: once-run composes.
//   - THE ROOT FENCE: adoption only at the root stream (no open unit,
//     no open fragment). A do nested in a callback's compiled body
//     (`[1 2] each [do [def x 5]]`) would otherwise place its twin
//     inside the per-invocation unit and convert a sound regime refusal
//     into execution of that closure shape; declined, the twin stays
//     unplaced and the refusal stands. (That shape's DEFAULT compile
//     diverges today — each_error vs the interpreter's [1 2] — a
//     pre-existing closure-lowering bug this fence keeps the regime
//     strictly sounder than.)
//   - THE SITE TEST: exact membership of tr.Pos in the body tree's
//     token sites (a transition's noted position is a token of its own
//     def expression — the bound value's token, or the transition word
//     via the CurWordPos fallback), never a positional range, which
//     would overclaim through baked data. Zero positions never match
//     (a synthesized token has no source home).
//
// Multi-run body words (each/fold) must never adopt AT ALL — which is
// why the caller's gate is the word's spec flag, not the closure record
// itself; their twins stay table-only and refuse toward the
// arm-residency increment.
//
// Rollback safety is two-layered: adopted event seqs are truncated with
// their frame like any event, and twinAdoptions lets Rollback un-mark
// the twinPlaced flags so a re-recorded dispatch on the next loop round
// can adopt again; even a stale flag only under-places, and Finalize's
// accounting counts REAL OpBindTwin ops, so any miss refuses (sound).
func (es *EmitState) AdoptBodyTwins(body core.Value) {
	if es == nil || !es.Active() {
		return
	}
	if len(es.openUnitRecs) != 0 || len(es.frames) != 1 {
		return
	}
	sites := make(map[core.SrcPos]bool)
	collectTokenSites(body, sites)
	tainted := func(i int) bool {
		for _, r := range es.lastKeepTaints {
			if i >= r[0] && i < r[1] {
				return true
			}
		}
		return false
	}
	for i := es.lastKeepRange[0]; i < es.lastKeepRange[1] && i < len(es.bindTwins); i++ {
		tr := es.bindTwins[i]
		if es.twinPlaced[i] || tainted(i) || tr.Pos.Row == 0 || !sites[tr.Pos] {
			continue
		}
		es.appendEvent(EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: i, pos: tr.Pos}})
		es.twinPlaced[i] = true
		es.twinAdoptions = append(es.twinAdoptions, i)
	}
}

// closureLatch — see the lastClosure field doc.
type closureLatch struct {
	unit  int
	fresh bool
}

// AdoptResidentTwins is the ARM-RESIDENCY bridge (§6.5's each-body
// recovery): after a BodyMultiRunKeepsDefs word's token body compiles to
// a fresh closure unit, pair the twins its own analysis noted
// (lastMultiRun's bracket) against the unit's def-site events, one to
// one, and stamp each matched event with its twin index — the lowering
// then emits the runtime-value install (OpBindResident) at the def site,
// executing once per element with the per-element value, which is what
// the ledger's one generalized carrier-valued capture cannot replay.
//
// The pairing is NAME + OCCURRENCE ORDER, strict and total: the bracket's
// eligible twins (a value def, its balanced undef half, or a TYPE install)
// must equal the unit's def-event sequence name-for-name in order, with a
// position cross-check wherever both sides carry one. ANY mismatch — a
// leftover twin, a leftover event, a name or position disagreement —
// adopts NOTHING: the twins stay unplaced and the regime refuses the
// program, the sound direction the parity oracle pins.
//
// A TYPE install differs from a value def in what its op does: there is no
// runtime value to install, so the op re-installs the twin's captured BODY
// per element instead (core.ApplyResidentTypeBind). That is sound only for
// an element-independent type expression, which
// typeInstallElementIndependent screens for — read its doc before widening
// anything here.
//
// The fences, each measured or judge-raised on the design review:
//
//   - the LATCH IDENTITY: lastMultiRun.bodyID must equal this dispatch's
//     body ID — a nested multi-run body's analysis during this unit's
//     compile overwrote the latch, and pairing against the wrong run
//     would install wrong values;
//   - FRESHNESS: a memoized unit's def events already carry another
//     dispatch's twins (or lowered ops) — the second dispatch's twins
//     stay unplaced;
//   - the ROOT FENCE and REGISTRY FENCE: adoption only at the root
//     stream, and only when the noting registry, the unit's registry,
//     and the program registry agree (a module-driven body's installs
//     land in its own registry — out of this increment's scope);
//   - REGIME ONLY: the resident op exists only under the regime, so
//     adoption outside it would mark placements nothing lowers.
//
// Matched names join armBoundNames — a later root read poisons the
// placement gate (NoteDefRead → armReadRefusal, refused at Finalize's
// seam), because the runtime binding's count, values, and definedness
// are body-run-dependent. Rollback safety mirrors
// AdoptBodyTwins: stamps live on event pointers inside the unit's
// fragment (discarded with it), twinAdoptions un-marks the flags, and
// Finalize's placement scan counts REAL resident ops, so any miss
// refuses.
func (es *EmitState) AdoptResidentTwins(body core.Value) {
	if es == nil || !es.Active() {
		return
	}
	if len(es.openUnitRecs) != 0 || len(es.frames) != 1 {
		return
	}
	cl, ml := es.lastClosure, es.lastMultiRun
	if !cl.fresh || cl.unit < 0 || cl.unit >= len(es.fnRecs) {
		return
	}
	rec := es.fnRecs[cl.unit]
	// Compared against progReg, the TOP-LEVEL program registry, and NOT against
	// es.reg: es.reg is last-bind-wins (BindRegistry re-binds it at the top of
	// every Engine.Run, module-body and island sub-engines included, and never
	// restores it), so a body that merely CALLS a module-defined boru fn left
	// es.reg pointing at that module's sub-registry and this fence declined an
	// adoption whose own registries were perfectly in agreement. The whole
	// program then refused for want of the placement. progReg is captured once
	// and never re-bound, which is what "the program registry" above means —
	// the Finalize stamp site uses it for this same reason.
	if body.ID == "" || ml.bodyID != body.ID || ml.reg == nil || ml.reg != es.progReg ||
		rec.reg != es.progReg || rec.frag == nil {
		return
	}
	// The bracket's twins: every one must be an eligible value def, an
	// undef (a var param's balanced teardown half), or a TYPE install, or
	// the whole bridge declines (a def-replace is the shape left).
	var twins []int
	bound := map[string]bool{}
	for i := ml.from; i < ml.to && i < len(es.bindTwins); i++ {
		if es.twinPlaced[i] {
			return
		}
		tr := es.bindTwins[i]
		switch tr.Kind {
		case core.BindDef, core.BindUndef:
			// A VALUE def's install carries a runtime value; a captured type
			// node under those kinds is a shape this bridge does not carry.
			if es.bindTwinEntries[i].TypeDef != nil {
				return
			}
		case core.BindTypeInstall:
		default:
			return
		}
		twins = append(twins, i)
		// Every binding the body installs is a twin in this bracket — that is
		// the bridge's own premise, since a leftover twin declines — so the
		// bracket's names ARE the body-bound set the type screen needs.
		bound[tr.Name] = true
	}
	if len(twins) == 0 {
		return
	}
	// The unit's def-site events, in order.
	var events []*emitDynBind
	for _, ev := range rec.frag.events {
		if ev.kind == evDynBind {
			events = append(events, ev.dyn)
		}
	}
	if len(events) != len(twins) {
		return
	}
	for k, i := range twins {
		tr, d := es.bindTwins[i], events[k]
		if d.name != tr.Name || d.residentTwin >= 0 {
			return
		}
		// Kind correspondence: a BindDef twin pairs with an install site, a
		// BindUndef twin with a teardown site, a BindTypeInstall twin with a
		// type-install site — never crossed.
		if d.undef != (tr.Kind == core.BindUndef) ||
			d.typeInstall != (tr.Kind == core.BindTypeInstall) {
			return
		}
		if tr.Pos.Row != 0 && d.pos.Row != 0 && tr.Pos != d.pos {
			return
		}
		if tr.Kind == core.BindTypeInstall &&
			!typeInstallElementIndependent(body, rec.frag, tr.Pos, bound) {
			return
		}
	}
	// Total match: stamp, mark, and fence the reads (the installing halves
	// drive the fence; an undef half re-fences nothing).
	for k, i := range twins {
		events[k].residentTwin = i
		es.twinPlaced[i] = true
		es.twinAdoptions = append(es.twinAdoptions, i)
		if k := es.bindTwins[i].Kind; k == core.BindDef || k == core.BindTypeInstall {
			if es.armBoundNames == nil {
				es.armBoundNames = map[string]bool{}
			}
			es.armBoundNames[es.bindTwins[i].Name] = true
		}
	}
}

// typeInstallElementIndependent is the ARM-RESIDENT TYPE twin's soundness
// screen. A value def's resident op installs the RUNTIME value once per
// element, so it is right by construction whatever the body computes; a
// type install has no runtime value — the minted node is a compile-time
// product the twin table already holds — so its op REPLAYS the one captured
// entry per element. That is correct exactly when every element's own
// evaluation of the type expression would have produced an equivalent node,
// and the ledgered witnesses are (`(Integer gt 10)`, `(Integer lt 20)` read
// nothing per-element) — but the bridge must PROVE it, because the opposite
// shape is ordinary boru: measured 2026-09-11,
// `[10 20] each [var [[e] def ZB (Integer gt e) 7]]` leaves TWO different
// `ZB` nodes stacked, so 15 fails against the top and passes against the one
// below it. Replaying one node there would be a silent wrong answer.
//
// The element can reach a type expression two ways, and each has a screen:
//
//   - THROUGH A NAME. Every binding the body installs is a twin in the
//     bracket, so `bound` is the body-bound set exactly; a type expression
//     naming any of them is per-element. Words bound OUTSIDE the body — a
//     module-scope type, an enclosing fn's capture — are constant across the
//     whole loop, so they pass.
//   - THROUGH A DISPATCH that did not const-fold. A word whose call the check
//     pass could not fold records an EVENT at its own position inside the
//     unit, so an expression with no event in its token span const-folded
//     from its own tokens alone. The converse — a word the check pass DID
//     fold — is already licensed everywhere else in this compiler: a folded
//     call's value is baked into the program.
//
// The third route, reading the element off the VALUE STACK, is closed by the
// language and not by this screen: a paren group binds forward-only, so
// `[10 20] each [def ZG (Integer gt) 7]` and `(mk)` for a one-arg `mk` both
// raise "no signature matches" on the interpreter (measured the same day).
//
// Anything this cannot see into — a def site the token walk cannot locate, a
// non-word non-literal token in the expression — declines, which refuses the
// regime program: the sound direction, never a wrong install.
func typeInstallElementIndependent(body core.Value, frag *EmitFragment, pos core.SrcPos, bound map[string]bool) bool {
	expr, ok := typeDefExprAt(body, pos)
	if !ok {
		return false
	}
	if !typeExprInert(expr, bound) {
		return false
	}
	sites := map[core.SrcPos]bool{}
	collectExprSites(expr, sites)
	for _, ev := range frag.events {
		if sites[eventPos(ev)] {
			return false
		}
	}
	return true
}

// typeDefExprAt locates the TYPE EXPRESSION token of the `def NAME <expr>`
// written at pos in a code-body token tree — pos being the `def` token's own
// position, which is what a bind transition's site resolves to
// (core.bindSitePos: a type body carries 0:0, so the note falls back to the
// dispatching word). The expression is the second operand in written order;
// any other layout (a `def` whose operands ride the value stack, a truncated
// tail) is not located and declines.
func typeDefExprAt(v core.Value, pos core.SrcPos) (core.Value, bool) {
	toks := tokenInterior(v)
	for i, el := range toks {
		if el.Pos() == pos && core.IsWord(el) && i+2 < len(toks) {
			if w, werr := core.AsWord(el); werr == nil && w.Name == "def" {
				return toks[i+2], true
			}
		}
		if found, ok := typeDefExprAt(el, pos); ok {
			return found, true
		}
	}
	return core.Value{}, false
}

// typeExprInert reports whether every token in a type expression's subtree is
// either a container to descend (a paren group is list-shaped, as
// collectTokenSites' doc records), an inert const, or a plain WORD the body
// does not bind. A reach, a splice, an interpolated string — anything that is
// none of the three — declines, so the walk is closed by construction rather
// than by a list of known-bad shapes.
func typeExprInert(v core.Value, bound map[string]bool) bool {
	if core.IsWord(v) {
		w, err := core.AsWord(v)
		return err == nil && !bound[w.Name]
	}
	for _, t := range tokenInterior(v) {
		if !typeExprInert(t, bound) {
			return false
		}
	}
	if isTokenContainer(v) {
		return true
	}
	return core.IsInertConst(v)
}

// isTokenContainer / tokenInterior are the token-tree descent the type
// screen's two walks share. A paren group is NOT list-shaped — AsList
// declines it, the payload being ParenExprPayload — so a walk that asks
// AsList alone treats `(Integer gt 5)` as one opaque token: the sound
// direction for typeExprInert (an opaque token is not inert, so it declines,
// which is how this screen first refused the row it exists to admit) and the
// WRONG one for a position gather, which would then screen an empty set.
func isTokenContainer(v core.Value) bool {
	if core.IsParenExpr(v) {
		return true
	}
	_, err := core.AsList(v)
	return err == nil
}

func tokenInterior(v core.Value) []core.Value {
	if core.IsParenExpr(v) {
		toks, err := core.AsParenExpr(v)
		if err != nil { //covergate:allow IsParenExpr above already proved the payload type AsParenExpr asserts; asked anyway so the accessor pair is total (§compiler)
			return nil
		}
		return toks
	}
	rl, err := core.AsList(v)
	if err != nil {
		return nil
	}
	out := make([]core.Value, 0, rl.Len())
	for i := 0; i < rl.Len(); i++ {
		out = append(out, rl.Get(i))
	}
	return out
}

// collectTokenSites gathers every source position in a code-body token
// tree: the token's own, then LIST interiors recursively, so this reaches
// a twin noted anywhere down a nested body. Everything else — words,
// literals, sugar markers, and a PAREN GROUP, which is not list-shaped
// (AsList declines a ParenExprPayload) — contributes its own position
// only; a construct this walk cannot see into leaves its twins unadopted,
// which refuses the regime program (the sound direction). The paren case
// was documented here as list-shaped and is not: widening the walk would
// adopt twins noted inside a paren group, which nothing has argued for.
// collectExprSites below is the walk that does descend, for a screen that
// needs the opposite default.
func collectTokenSites(v core.Value, sites map[core.SrcPos]bool) {
	if p := v.Pos(); p.Row != 0 {
		sites[p] = true
	}
	if rl, err := core.AsList(v); err == nil {
		for i := 0; i < rl.Len(); i++ {
			collectTokenSites(rl.Get(i), sites)
		}
	}
}

// collectExprSites is collectTokenSites over a TYPE EXPRESSION, and it
// descends one shape more: a PAREN group. The two are deliberately separate.
// collectTokenSites decides which twins AdoptBodyTwins may adopt, and its
// blindness to a paren interior is a refusal there — widening it would adopt
// twins nothing has argued for. The type screen needs the opposite default:
// it must see EVERY token inside `(Integer gt 5)` or it would screen an empty
// set and pass anything.
func collectExprSites(v core.Value, sites map[core.SrcPos]bool) {
	if p := v.Pos(); p.Row != 0 {
		sites[p] = true
	}
	for _, t := range tokenInterior(v) {
		collectExprSites(t, sites)
	}
}

// RecordDefRebind records a `def` dispatch of a loop-carried name: the
// rebind STORES its value into the name's carried frame slot at the rebind
// site, so a conditional rebind updates the cell exactly when its arm runs
// and a break/continue skips it exactly as the interpreter skips the def.
// A def of a name no active armed loop carries records nothing. Once a
// name IS carried, every rebind must store or the cell goes stale — an
// unresolvable or function-shaped rebind value refuses the program (sound
// interpreter fallback, never a stale read).
func (es *EmitState) RecordDefRebind(name string, v core.Value, pos core.SrcPos) {
	if !es.Active() || len(es.loopCarried) == 0 {
		return
	}
	slot, found := -1, false
	for i := len(es.loopCarried) - 1; i >= 0; i-- {
		scope := es.loopCarried[i]
		if scope.unitDepth != len(es.units) {
			continue
		}
		if s, ok := scope.slots[name]; ok {
			slot, found = s, true
			break
		}
	}
	if !found {
		return
	}
	if core.IsFnValueResidual(v) {
		es.MarkUncompilable("loop-carried def `" + name + "` rebound to a function value (Stage 3)")
		return
	}
	src, ok := es.resolveOperand(v)
	if !ok {
		es.MarkUncompilable("loop-carried def `" + name + "` rebind of unknown provenance")
		return
	}
	es.appendEvent(EmitEvent{kind: evStore, store: &emitStore{src: src, slot: slot, pos: pos}})
	es.noteStoreHazard(name, slot)
}

// RefuseCarriedUndef marks the program uncompilable when `undef` targets a
// name an active armed loop carries: the undef exposes the PREVIOUS binding
// while the carried slot still holds the rebound value, so compiled reads
// would diverge from the interpreter. An undef of any other name is
// untouched.
func (es *EmitState) RefuseCarriedUndef(name string) {
	if !es.Active() {
		return
	}
	for i := len(es.loopCarried) - 1; i >= 0; i-- {
		if _, ok := es.loopCarried[i].slots[name]; ok {
			es.MarkUncompilable("undef of the loop-carried def `" + name + "` (Stage 3)")
			return
		}
	}
}

// emitCheckpoint snapshots the append-only recording pools and counters so a
// DISCARDED loop-analysis round can be rolled back. AnalyseLoopBody re-runs a
// loop body to a binding fixed point but keeps only the final (stabilised)
// round's fragment; without rollback the earlier rounds' interned consts and
// island fallback spans orphan into the Program and their dispatches inflate
// SiteCounts (the metric surfaced via CheckResult.SiteCounts). The snapshot is
// by LENGTH for the slice pools (intern/internType/RecordFallback only append)
// and by VALUE for the small SiteCounts map.
type emitCheckpoint struct {
	core.EmitCheckpointBase
	seq           int
	consts        int
	types         int
	fallbacks     int
	fnRecs        int
	twinAdoptions int
	siteCounts    map[string]int
}

// Checkpoint captures the rollback point. Nil-safe (returns a zero checkpoint
// that Rollback ignores via the nil-receiver guard at its call site).

func (es *EmitState) Checkpoint() core.EmitCheckpoint {
	if es == nil {
		return emitCheckpoint{}
	}
	sc := make(map[string]int, len(es.SiteCounts))
	for k, v := range es.SiteCounts {
		sc[k] = v
	}
	return emitCheckpoint{
		seq:           es.seq,
		consts:        len(es.consts),
		types:         len(es.types),
		fallbacks:     len(es.fallbacks),
		fnRecs:        len(es.fnRecs),
		twinAdoptions: len(es.twinAdoptions),
		siteCounts:    sc,
	}
}

// Rollback discards everything recorded since cp — used to drop a non-final
// loop-analysis round so only the stabilised round lands in the Program.
//
// SAFETY: a round that compiled a fn UNIT (a closure/island body via
// StartFnCompile) cannot be unwound — the unit's lowered code references shared
// const/type indices, so trimming the pools would dangle them. That shape (a
// fn body inside a loop that ALSO needs multiple fixed-point rounds) is rare;
// for it we keep the round's recording (the prior behaviour: a little bloat,
// always correct) rather than risk corrupting an emitted unit.
func (es *EmitState) Rollback(h core.EmitCheckpoint) {
	cp, ok := h.(emitCheckpoint)
	if !ok {
		return
	}
	if es == nil || len(es.fnRecs) != cp.fnRecs {
		return
	}
	for k, i := range es.constIdx {
		if i >= cp.consts {
			delete(es.constIdx, k)
		}
	}
	for k, i := range es.constIDIdx {
		if i >= cp.consts {
			delete(es.constIDIdx, k)
		}
	}
	for i := range es.freshenConst {
		if i >= cp.consts {
			delete(es.freshenConst, i)
		}
	}
	es.consts = es.consts[:cp.consts]
	for k, i := range es.typeIdx {
		if i >= cp.types {
			delete(es.typeIdx, k)
		}
	}
	es.types = es.types[:cp.types]
	es.fallbacks = es.fallbacks[:cp.fallbacks]
	// Adoptions are append-only, so every adoption past the checkpoint
	// placed an event this rollback's frame truncation discards — un-mark
	// its flag with it, so a dispatch re-recorded on the next round can
	// adopt the twin again (and a twin left unplaced refuses, sound).
	for _, idx := range es.twinAdoptions[cp.twinAdoptions:] {
		es.twinPlaced[idx] = false
	}
	es.twinAdoptions = es.twinAdoptions[:cp.twinAdoptions]
	// Provenance entries minted after cp belong to the discarded round's
	// carriers (fresh ids never referenced again); drop them, mirroring
	// StartFnCompile's fn-unit cleanup so the live map stays tight.
	for id, pr := range es.producedBy {
		if pr.seq > cp.seq {
			delete(es.producedBy, id)
		}
	}
	es.SiteCounts = cp.siteCounts
	es.seq = cp.seq
	es.captured = nil
}

// fnArm bypasses AnalyseFnBody's suspension exactly once — set by
// StartFnCompile so the armed body analysis records into the open
// fn fragment.
func (es *EmitState) FnBodyGuard() func() {
	if es != nil && es.fnArm {
		es.fnArm = false
		return func() {}
	}
	return es.Suspend()
}

// StartFnCompile reserves (or finds) the compiled unit for one fn
// overload at one arg shape and, when fresh, opens a fn recording
// scope: a new local-numbering unit with the arg carriers registered
// as param slots 0..n-1 (sig order — frame locals at run time) and
// the captures as hidden trailing slots, and a fragment capturing
// the body analysis that the caller runs next. finish (non-nil only
// when fresh) closes the scope with the body's residual stack.
// Generic fns compile one unit per memoised instantiation — the key
// carries the instantiated arg types, and the caller has the gen
// bindings installed around the recorded analysis. ok=false when the
// fn is beyond Stage 4 (unchecked or multi-value returns) — the
// program is then marked uncompilable.
func (es *EmitState) StartFnCompile(key, name string, fnReg *core.Registry, args []core.Value, declared []*core.Type, paramNames []string, captures []core.CapturedBinding, generic bool, pos core.SrcPos) (unit int, finish func([]core.Value), ok bool) {
	if !es.Active() {
		return -1, nil, false
	}
	if u, hit := es.fnUnits[key]; hit && !es.unitStale(u) {
		return u, nil, true
	}
	// A miss, or a STALE hit: the memoised unit baked a binding this call
	// site sees differently (unit_memo.go). Compile a fresh unit for this
	// site and re-point the key at it; the stale unit keeps serving the
	// sites that already reference it.
	unit = len(es.fnRecs)
	// Slot→name table (debug only): params in slots 0..n-1, then captures.
	locals := make([]string, 0, len(args)+len(captures))
	for i := range args {
		if i < len(paramNames) {
			locals = append(locals, paramNames[i])
		} else {
			locals = append(locals, "")
		}
	}
	for _, cb := range captures {
		locals = append(locals, cb.Name)
	}
	nUnnamed := 0
	for i := range args {
		if i >= len(paramNames) || paramNames[i] == "" {
			nUnnamed++
		}
	}
	rec := &fnUnitRec{name: name, nParams: len(args), nUnnamed: nUnnamed, caps: captures, generic: generic, returns: declared, locals: locals, pos: pos, reg: fnReg}
	es.fnRecs = append(es.fnRecs, rec)
	es.fnUnits[key] = unit
	u := &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}, enclosingIDs: es.snapshotCompoundBindingIDs(), enclosingBindIDs: es.snapshotAllBindingIDs(), enclosingBindNames: es.snapshotAllBindingNames(), reg: fnReg}
	es.units = append(es.units, u)
	es.unitNames = append(es.unitNames, name)
	es.openUnitRecs = append(es.openUnitRecs, unit)
	for _, a := range args {
		es.RegisterLocal(a.ID)
	}
	// Capture slots: the body analysis binds each captured name to
	// cb.Value (the construction-time snapshot — AnalyseFnBody pushes
	// the SAME Value), so body references resolve to these slots by
	// ID. Registered after params, locals nParams…nParams+nCaps-1 —
	// the numbering is POSITIONAL, so an identity-less capture (a value
	// minted at runtime under the mode-gated ID elision, later compiled)
	// cannot be skipped without shifting every subsequent capture's slot:
	// refuse the unit instead (conservative interpreter fallback).
	for _, cb := range captures {
		if cb.Value.ID == "" {
			es.MarkUncompilable("closure captures a runtime-minted value (no compile identity)")
		}
		es.RegisterLocal(cb.Value.ID)
		u.capID[cb.Value.ID] = true
	}
	resume := es.beginFragment()
	rec.rootFrame = len(es.frames) - 1
	es.fnArm = true
	finish = func(bodyStk []core.Value) {
		resume()
		rec.frag = asFragment(es.TakeFragment())
		// forceOrder mirrors Finalize's program-residual promotion for THIS
		// unit: when the residual is out of order (an event result above an
		// inert bottom), every residual event is promoted to a frame local so
		// the reconciliation re-pushes the whole residual in exact order.
		var forceOrder map[int]bool
		if !fragDiverges(rec.frag) {
			// Resolve every residual value to an operand, in stack order
			// (bottom→top), so the unit leaves the body's N results for its
			// caller. A 0-output statement guard (`if cond [raise]`) registered a
			// phantom None in the residual but produces 0 runtime values, so it
			// leaves NO operand — skip it, exactly as the top-level residual
			// reconciliation does. A 0-result body leaves outOps empty (a bare RET).
			ops := make([]EmitOperand, 0, len(bodyStk))
			vals := make([]core.Value, 0, len(bodyStk))
			for _, v := range bodyStk {
				if pr, ok := es.producedBy[v.ID]; ok && es.eventInfo[pr.seq].zeroOut {
					continue
				}
				// A body that RETURNS an anonymous capture-free lambda (the factory
				// pattern `def mk fn [[x][Function][([y]=>…)]]`) compiles the
				// returned fn to its own closure unit, so the body leaves a runtime
				// closure VALUE on the stack — applied VM-native by a later stack
				// OpCallDynamic (`(mk 5) 10`). Tried BEFORE resolveOperand because a
				// capture-free anonymous fn would otherwise bake as an inert const
				// and apply through a callDynamic interpreter island instead of the
				// VM. A non-fn / named / capturing / multi-sig value declines here
				// and falls through to the normal resolution.
				op, okOut := es.tryReturnedClosure(v, rec.pos)
				if !okOut {
					op, okOut = es.resolveOperand(v)
				}
				if !es.residualStands("fn "+name+": ", v, op, okOut, rec.frag, "body result") {
					return
				}
				ops = append(ops, op)
				vals = append(vals, v)
			}
			// A paren-bounded TRAILING fn-value apply as the ENTIRE body residual
			// (`(prev key comp)` → residual [prev, key, comp], the fn on top with its
			// args below, registered at the paren-collapse boundary with its arity):
			// lower it to OpCallDynTrailTop rather than refusing/miscompiling. outOps
			// stay the full [args…, fn]; the unit lowering applies them to one value
			// before the RET. The args must be plain (non-fn) operands the apply
			// consumes — a nested unapplied fn falls through to refuse below.
			dynTrail := 0
			if len(bodyStk) >= 2 {
				top := bodyStk[len(bodyStk)-1]
				if a := es.TrailingApplyArity(top.ID); a > 0 && a == len(bodyStk)-1 {
					argsOK := true
					for _, v := range bodyStk[:len(bodyStk)-1] {
						if core.IsFnValueResidual(v) {
							argsOK = false
							break
						}
					}
					if argsOK {
						dynTrail = a
					}
				}
			}
			// An `apply`-word pending over a fn CARRIER (Stage M2a): the elided
			// dispatch left the fn on the residual top with its args below —
			// the interpreter's applyHandler re-steps the fn against the whole
			// preceding stack, so the window is the ENTIRE residual. Exactly
			// one pending apply may lower, and only as that whole-residual
			// window with no other fn value in it; any other pending shape
			// (mid-body apply, double apply, apply into a branch join) REFUSES
			// — never compiles the fn+args as unapplied data, and never drops
			// an apply the interpreter performed.
			// A fn VALUE beneath the pending apply is an ARGUMENT, not a
			// second apply (the thirty-first increment): applyHandler
			// re-steps the fn over the RESOLVED stack, where a parked lambda
			// is data the callee's Function param binds (`cfst`'s
			// `(a:Any => [b:Any => [a/v]]) p/v apply` hands the projection
			// to the pair), and the op's window binds it the same way — so
			// the window takes every value beneath, fn-valued or not.
			if pend := u.pendingApply; len(pend) > 0 {
				if dynTrail == 0 && len(pend) == 1 && len(bodyStk) >= 2 &&
					bodyStk[len(bodyStk)-1].ID == pend[0].id {
					dynTrail = len(bodyStk) - 1
					// applyHandler unquotes: a /v-parked fn value still
					// applies (OpCallDynApplyTop), unlike the paren case.
					rec.dynTrailApply = true
					rec.dynTrailPos = pend[0].pos
				}
				if dynTrail == 0 {
					es.MarkUncompilable("fn " + name + ": apply of a dynamic fn value not at the body tail (Stage 3)")
					return
				}
			}
			// A DECLARED fn must leave exactly len(returns) RUNTIME values; a
			// different count (measured over real operands, phantom guards
			// excluded) is a return-COUNT mismatch. For a genuine USER fn that is
			// the type_error the interpreter raises at __RC, so rather than refuse
			// we COMPILE the body and let the VM's RET enforce the count — it raises
			// the byte-identical type_error (shared returnCountErrorText), erroring
			// exactly where the interpreter does instead of falling back. For a
			// CLOSURE body (each/scan/…$body) the mismatch is instead the
			// higher-order word's OWN runtime error (each_error "body produced no
			// result"), a different taxonomy — so a closure keeps refusing and
			// islands, letting the interpreter raise the matching error. An
			// UNDECLARED fn (a 0-return fn, or a closure body compiled
			// count-agnostic) is NOT count-checked: its residual is taken as-is.
			// An anonymous lambda is DECLARED here — its placeholder's count
			// (check.LambdaCountContract) is the contract its RET enforces.
			if dynTrail == 0 && rec.closure && !rec.plainLambda() && len(rec.returns) > 0 && len(ops) != len(rec.returns) {
				es.MarkUncompilable("closure " + name + ": body value count differs from declared returns")
				return
			}
			// A USER fn whose residual COUNT mismatches AND whose residual carries a
			// Function VALUE — or a DYNAMIC value that may turn out to be one
			// (a map get over Any: the stylesheet fn-value-dispatch idiom) — is a
			// possible UNAPPLIED fn-value-call: `(fnv 100)` pushes [fnv, 100] without
			// applying, where the interpreter applies fnv → ONE value. The
			// count-mismatch-compiles path below assumes the interpreter ALSO
			// mismatches (and the VM RET raises the matching type_error), but here it
			// may APPLY and succeed. noteDynFrameReplay classifies the residual as
			// the whole-frame replay (OpCallDynFrame — the region re-steps under
			// execFnDefLiteral's own runtime rule, faithful whether the value applies
			// or stays data) and arms the RET's RetReplay discipline; a shape it
			// cannot prove (a mid-body apply failing the body-tail gate, an
			// undecomposable window) refuses → fall back. (A GENUINE count mismatch
			// that happens to carry a fn value still errors in both engines, so the
			// fallback is sound either way.)
			// The three replay verdicts share ONE refusal site (the refusal-site
			// census only falls): the count-mismatch replay declining, the
			// word-read replay unable to seat (NUR123), and the word-read
			// accounting — see fnResidualReplayReason.
			//
			// A BARE READ of a fn-valued frame binding left in the residual is
			// a WORD dispatch on the interpreter — stepWord routes a bound
			// FnDefInfo through Registry.Lookup under the binding name: a
			// 0-arg fn FIRES, an n-arg fn collects from the tokens after it and
			// the frame's stack below it, a no-match raises `cannot call `g``
			// — where this unit pushed the slot: `def f fn [[g:Function][Any]
			// [g]]  f ([] => [42])` answered `fn` for the interpreter's 42, and
			// `def id fn [[x:Any][Any][x]]  id ([] => [42])` the same, both
			// exit 0 (NUR123, measured 2026-09-05). The count MATCHED, so the
			// arm above never looked. Arm the whole-frame replay over the token
			// region with the binding NAMES (fnUnitRec.dynFrameWords): the VM
			// installs each fn-valued word-read entry as a frame binding and
			// re-steps the region through the interpreter's own word dispatch;
			// a plain-data entry (the identity fn over 5) skips the island. A
			// residual the replay cannot seat refuses.
			if reason := es.fnResidualReplayReason(u, rec, vals, ops, dynTrail); reason != "" {
				es.MarkUncompilable("fn " + name + ": " + reason)
				return
			}
			// A CLOSURE body whose residual carries an UNAPPLIED fn-value — a captured/
			// param comparator applied as `(a b comp)` leaves the residual [a, b, comp]
			// with the fn VALUE on top — must REFUSE, never be trimmed to that fn below:
			// the driving handler (BodyResultTop) would then map to the UNAPPLIED comp
			// instead of its applied result — the off-corpus comparator-each MISCOMPILE
			// the differential cannot see (`[1 2] each [(x x comp)]` → interp [0,0] vs
			// compiled [fn,fn]). This mirrors resolveDynamicApply's main-residual
			// fn-carrier / fn-precedes-args refusals (the fn-body path refuses the same
			// shape via the !rec.closure unapplied-fn-value check above). A SOLE inert
			// fn-reference body (`each [cmp/v]`, mapping every element to the reference)
			// is a CONCRETE const — not a carrier and not preceded by args — so it still
			// compiles; only an unapplied dynamic apply refuses. (Lowering the trailing
			// apply via OpCallDynamicTrailing in a closure body is the follow-on feature.)
			// A plain lambda whose WHOLE residual is one `/v` read of a captured
			// fn (`[g/v]`, the twenty-sixth increment) RETURNS that value as
			// data on both lanes: the read delivers it quoted, the return
			// strips the quote, and the caller decides whether to apply it —
			// `((h 5) 2)` is 6, `(h 5) 2` the parked pair. Nothing is
			// unapplied here.
			if dynTrail == 0 && rec.closure && rec.dynFrameW == 0 && closureResidualHasUnappliedFn(bodyStk) &&
				!(rec.plainLambda() && len(bodyStk) == 1 && (bodyStk[0].Quoted || rec.valReads[bodyStk[0].ID] > 0)) {
				es.MarkUncompilable("closure " + name + ": unapplied fn-value in body residual (dynamic apply not lowered)")
				return
			}
			// A closure body's residual is kept WHOLE, even under a driving
			// handler that reads only its top (each / fold / scan / filter —
			// CallableSpec.BodyResultTop). The values below the top are what a
			// fn VALUE's return contract counts: the interpreter's callback
			// seam runs the fn's own __RC before the handler reads the top, so
			// `def f fn [[n:Integer][Integer][n 1]]  each f/v [1 2]` raises
			// `expected 1 return value(s), got 2` there — and a compile-time
			// trim to the top (the former trimToTopResult) hid the second value
			// from checkClosureReturn and answered `[1 1]` (NUR120). The
			// out-of-order shape the trim once sidestepped (`each [add 1 0]` →
			// [inert input, event]) is handled by residualForceOrderFor's
			// promotion below, and a raw token body carries no contract, so
			// the handler reads its top exactly as before.
			// A per-iteration APPLY-LOOP's variadic value region in the residual,
			// under an inert tail: the RET takes the replay trim discipline.
			if dynTrail == 0 && rec.dynFrameW == 0 && !rec.closure && len(rec.returns) > 0 {
				ops = es.noteApplyLoopReplay(rec, ops)
			}
			rec.dynTrailArity = dynTrail
			rec.outOps = ops
			// VARIADIC-RETURNING fn: the body residual's defining (top) event leaves
			// a runtime-variable count (a variadic branch / loop, or a call to an
			// already-variadic fn — RecordUserCall flags that on the event). A call
			// site marks its result variadic (lowerUserCall) so only a
			// variadic-absorbing position consumes it. EXCEPTION: a DECLARED
			// return tuple over a DYN-BODY residual (dynBodyResult) overrides
			// the marking — the sub-run's results are real stack values at the
			// RET, where the VM enforces the declared count exactly as the
			// interpreter ("expected N return value(s)"), so the call site may
			// seat the declared shape. Without this, `def make-point fn [[…]
			// [Map] [ do {…} ]]` (a fn whose whole body is a dyn-do computed
			// map) marked its result variadic and any fixed-arity consumer
			// refused "consumes loop results". A variadic BRANCH/LOOP residual
			// keeps the marking even under a declared tuple: its runtime
			// 0-value shape flows through the interpreter's return path
			// differently, and the pinned divergence test
			// (TestEmitRaiseArmDivergence) requires the refusal.
			if n := len(ops); n > 0 && ops[n-1].kind == opEvent && es.eventInfo[ops[n-1].idx].variadicResult &&
				!(len(rec.returns) > 0 && es.eventInfo[ops[n-1].idx].dynBodyResult) {
				rec.variadic = true
			}
			forceOrder = es.residualForceOrderFor(dynTrail, rec, ops, vals)
		}
		// Value-def locals for THIS unit (the top-level program's promotion,
		// scoped to a fn body): a computed result referenced more than once, read
		// from INSIDE a body fragment (a `case` scrutinee re-tested down the
		// desugared if-chain — the case-in-closure enabler), or bound to a named
		// def must be stored in a frame slot and re-pushed, not left on the
		// single-consume stack. Run while the unit `u` is live so its slot
		// allocations land in u.numLocals (→ rec.numLoc below); the unit's OUTPUT
		// operands are its residual-equivalent extra references, and are rewritten
		// for any promoted producer (planValueDefLocals rewrote the events in
		// place, but outOps is a separate slice).
		es.planDeopts(u, rec)
		outSeqs := appendResidualSeqs(nil, rec.outOps)
		rec.promoted, rec.dead = es.planValueDefLocals(u, rec.frag.events, outSeqs, forceOrder)
		for i := range rec.outOps {
			promoteOperand(&rec.outOps[i], rec.promoted)
		}
		// The unit is closed: its events' outputs cannot be stack
		// values in the enclosing scope, so drop their provenance
		// entries (after resolving the body result above, which DOES
		// reference them). Without this, a join inside the body that
		// reused a capture/param ID (JoinCarriers keeps the then-side
		// ID) would make an ENCLOSING call site resolve that value to
		// an event of this closed unit.
		//
		// O(live-IDs) scan per fn finish — i.e. O(fns × live-IDs) over a
		// compile. Negligible at current fn densities; revisit (e.g. track
		// per-unit produced IDs) if deeply nested fn-heavy programs make
		// compile time bite.
		for id, pr := range es.producedBy {
			if pr.seq > rec.frag.startSeq {
				delete(es.producedBy, id)
			}
		}
		rec.numLoc = u.numLocals
		rec.finished = true
		es.units = es.units[:len(es.units)-1]
		es.unitNames = es.unitNames[:len(es.unitNames)-1]
		es.openUnitRecs = es.openUnitRecs[:len(es.openUnitRecs)-1]
	}
	return unit, finish, true
}

// RecordUserCall records one call of a compiled fn unit. args are in
// signature order; outs are the dispatch's result carriers (0 for a
// 0-return fn, 1 for the common single-return case, N for a multi-
// return fn — registered idx 0..N-1, matching the order the VM's
// CALL_USER leaves them on the stack). The unit's captures ride as
// hidden trailing operands, resolved in the CALLER's scope: at the
// construction site they are the enclosing frame's values; inside the
// fn's own body (recursion) they resolve to the frame's own capture
// slots and re-flow unchanged. A capture unreachable from the call
// site marks the program uncompilable — the interpreter keeps owning
// that shape.
// SetUnitParamTypes records the declared param types for a compiled fn unit, so
// the VM enforces them at CALL_USER entry (the gradual-Any param-guard, mirroring
// the RET return-check). Set from the matched sig at the dispatch site, where the
// param types are known; a memoised unit is harmlessly re-set with the same types.
func (es *EmitState) SetUnitParamTypes(unit int, paramTypes []*core.Type, paramPatterns []*core.Value) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].paramTypes = paramTypes
	es.fnRecs[unit].paramPatterns = paramPatterns
}

// SetUnitBody seats a fn unit's source body tokens (EmitRecorder): the
// stream a per-read deopt hands to the interpreter (planDeopts, NUR123).
func (es *EmitState) SetUnitBody(unit int, body []core.Value) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].body = body
}

// SetUnitReturnPatterns records the declared return PATTERNS for a compiled fn
// unit (FnSig.ReturnPatterns), so the VM enforces them at RET — the RET-side
// twin of SetUnitParamTypes' patterns.
//
// It is a separate setter rather than a StartFnCompile argument because the
// patterns are only meaningful where the FnSig is in hand, which is the same
// pair of call sites that set the declaration site. Without it the compiled
// path — the DEFAULT path — accepted a body that both the interpreter and the
// check pass reject, because a declared union's *Type degrades to Any and the
// pattern is the only contract there is.
func (es *EmitState) SetUnitReturnPatterns(unit int, returnPatterns []*core.Value) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].returnPatterns = returnPatterns
}

// SetUnitDecl records the return-contract declaration site for a compiled fn
// unit (FnSig.Decl), so a compiled RET return-type/count error labels the
// declaration exactly as the interpreter does. Callers that have the FnSig in
// hand (the named-fn and user-poly compile paths) set it; anonymous closures
// leave it zero.
func (es *EmitState) SetUnitDecl(unit int, decl core.DeclSite) {
	if unit < 0 || unit >= len(es.fnRecs) {
		return
	}
	es.fnRecs[unit].decl = decl
}

func (es *EmitState) RecordUserCall(unit int, args []core.Value, outs []core.Value, pos core.SrcPos) {
	if !es.Active() || unit < 0 {
		return
	}
	rec := es.fnRecs[unit]
	ops := make([]EmitOperand, len(args), len(args)+len(rec.caps))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok {
			es.MarkUncompilable("fn call operand of unknown provenance")
			return
		}
		ops[i] = op
	}
	for _, cb := range rec.caps {
		op, ok := es.resolveOperand(cb.Value)
		if !ok {
			es.MarkUncompilable("capture " + cb.Name + " of " + rec.name + " unreachable at a call site")
			return
		}
		ops = append(ops, op)
	}
	seq := es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: unit, ops: ops, nout: len(outs), pos: pos}})
	es.SiteCounts[SiteMono]++
	// A call to an ALREADY-variadic fn yields a runtime-variable count itself, so
	// the result propagates variadic (a branch arm / body residual carrying it is
	// variadic too). The self-recursive call records before its own fn finishes
	// (rec.variadic not yet set) — fine: that result flows only to the body's own
	// variadic merge, never to a fixed operand.
	if es.fnRecs[unit].variadic {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
}

// RecordUserPolyCall records one runtime-dispatched MULTI-OVERLOAD user-fn
// call (the user-fn mirror of RecordPolyCall): the checker could not commit
// to one same-arity overload (a gradual-Any arg reached two or more), every
// arm's body already compiled to its own unit (tryCompileUserPolyArms), and
// the call lowers to OpCallUserPoly, which re-runs MatchSignature over the
// recorded arm subset at run time — the SAME first-match the interpreter
// takes. args are in signature order; outs are the committed dispatch's
// result carriers (every arm declares the identical Returns, gated by the
// recorder). Captures are gated empty by the recorder, so no hidden trailing
// operands ride here. An operand of unknown provenance marks the program
// uncompilable, exactly as RecordUserCall does.
func (es *EmitState) RecordUserPolyCall(word string, ownerReg *core.Registry, sigIdx, units []int, impls []core.SigImpl, sigs []core.Signature, args, outs []core.Value, pos core.SrcPos) {
	if !es.Active() {
		return
	}
	ops := make([]EmitOperand, len(args))
	for i, a := range args {
		op, ok := es.resolveOperand(a)
		if !ok {
			es.MarkUncompilable("fn call operand of unknown provenance")
			return
		}
		ops[i] = op
	}
	seq := es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{
		unit: -1, ops: ops, nout: len(outs), pos: pos,
		poly: &emitUserPolySpec{word: word, reg: ownerReg, sigIdx: sigIdx, units: units, impls: impls, sigs: sigs},
	}})
	es.SiteCounts[SiteDynamic]++
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
	es.lastUserPoly = &lastUserPolyNote{word: word, seq: seq, nout: len(outs)}
}

// DynApplyLeadEligible reports whether the paren-collapse may record a
// LEADING one-arg fn-carrier apply over v (the Stage-G increment,
// stepCloseParen): true only inside an open NAMED-PARAM fn unit where v is
// one of the unit's own slots (a param or capture read). The admission is
// deliberately narrow, each exclusion probe-backed:
//   - an UNNAMED-param frame re-pushes its args beneath the region, so the
//     interpreter's leading collection can reach past the sealed window the
//     trailing model records (`(args.0 args.1)` over a two-arg runtime fn
//     nets 28 interpreted where the trailing model no-matches) — those
//     frames keep the whole-frame replay;
//   - a CLOSURE unit's analysis frame is the CallableSpec inputs, not a
//     per-call named frame — outside the probe evidence, so it declines;
//   - an EVENT-provenance lead (a direct call result, `((mk 1) 2)`) keeps
//     the curried/auto-dispatch machinery — RecordDynApply hard-refuses an
//     event fn (runtime quote state unknown), so admitting one here would
//     turn a compiling shape into a refusal. A CAPTURE may carry a
//     parent-unit event entry yet resolves to its own slot (capID
//     precedence), so it stays eligible.
//
// Within an eligible unit, one-arg leading and trailing spellings CONVERGE
// for every runtime arity: the paren seals the named frame off, so a
// mismatched runtime fn (0-arg, 2-arg, multi-return) no-matches identically
// in both spellings (probe-pinned across arities in
// lang/go/bytecode_chained_apply_test.go).
func (es *EmitState) DynApplyLeadEligible(v core.Value) bool {
	if !es.Active() || len(es.openUnitRecs) == 0 {
		return false
	}
	// The Stage 2 closure-flag split's consumer half: a native code-body
	// closure unit (each/do$body — its analysis frame is the CallableSpec
	// inputs) still declines, but a LAMBDA unit ("fnval" — a returned
	// lambda's own body, a real named-param frame with capture slots) is
	// admitted: the witness is the factory's inner apply-the-capture body
	// (`def mkc2 fn [[g:Function][Function][( fn [[v:Integer][Integer]
	// [(g v)]] )]]` — without the admission the [g, v] residual
	// count-refuses the fnval probe and the whole factory refuses "body
	// result of unknown provenance"). The nUnnamed guard below still
	// excludes bare-type params.
	innermost := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if innermost.closure && !innermost.lambdaUnit {
		return false
	}
	if innermost.nUnnamed > 0 {
		return false
	}
	u := es.units[len(es.units)-1]
	if _, isLocal := u.localByID[v.ID]; !isLocal {
		return false
	}
	if _, produced := es.producedBy[v.ID]; produced && !u.capID[v.ID] {
		return false
	}
	return true
}

// RecordDynApply records a paren-bounded TRAILING fn-value apply (`(a b comp)`):
// the runtime fn VALUE `fn` applied to `args` (in source order — args[0] pushed
// first / deepest), netting the single result `out`. Recorded as an EVENT so the
// apply seats like any computed result — a def-local (`def c (a b comp)`), an `if`
// operand, a list member, OR the body's trailing residual — not ONLY the body
// residual (the old register-at-collapse / lower-at-reconciliation path handled
// just that). Returns false (sound interpreter fallback) when any operand is
// unresolvable or an ARG is itself an unapplied fn value (a nested apply the flat
// layout cannot order). ops are sig-order (ops[0] = top): the fn on TOP, then its
// args below it deepest-last — exactly the stack OpCallDynTrailTop reads (it
// reverses the arg window into forward order to match the interpreter's paren
// auto-dispatch, where the fn's first param is the arg just below it).
// applyWindowArity answers "how many values does this callee actually consume",
// or declines when that is a runtime question.
//
// It is what the former "runtime quote state unknown" refusal was really
// reaching for, and it applies to BOTH arrival kinds rather than only the event
// lead — which is how it also fixes a silent miscompile the old gate never saw.
//
//   - UNDER-APPLICATION. `(1 2 (mk 4))` with a 1-arg adder nets [1, 6]
//     interpreted: the adder takes the top value and the deeper 1 survives.
//     Handing the op the whole window made it consume both, so the event lead
//     refused outright. Handing it only the top `arity` reproduces the
//     interpreter exactly, and the caller leaves the rest on the tape.
//   - RESIDUAL ORDER, the miscompile. `(9 1 2 add2/v)` — a CONCRETE 2-arg
//     callee under a 3-wide window — answered [3, 9] compiled against the
//     interpreter's [9, 3]. The concrete path had no arity gate at all, so the
//     survivor came out ABOVE the result instead of below it. Trimming the
//     window puts it back where the interpreter leaves it.
//
// The arity claim is sound BY CONSTRUCTION, which is why trimming on it is
// safe. producerReturnedClosureArity answers only when the producer is an
// evCallUser whose unit has exactly ONE out-op and that op IS a closure unit,
// so the runtime value is provably that one unit with that many params — or
// when the producing word CLAIMED the arity of the wrapper it builds
// (CheckState.FnShapes, the thirty-fifth increment: fn-util's `goFnValue(name,
// n, …)` with n its own construction rule). The
// shapes where a static arity could be wrong never reach the trim: a factory
// whose branches return different arities refuses earlier ("if: then-branch
// result of unknown provenance"), and an overloaded callee declines below.
//
// DECLINING IS NOT REFUSING. A fn-typed CARRIER (a `comp:Function` param — the
// comparator convention) has no static arity and must keep compiling as it
// does today, so the caller treats "unknown" as "no trim" on the concrete path
// and keeps the standing refusal only on the event lead.
func applyWindowArity(es *EmitState, fn core.Value) (int, bool) {
	if arity, known := es.producerReturnedClosureArity(fn.ID); known {
		return arity, true
	}
	fd, isFn := fn.Data.(core.FnDefInfo)
	if !isFn {
		return 0, false // a fn-typed CARRIER — no static arity, see the header
	}
	own := fd.OwnSigs()
	if len(own) != 1 {
		return 0, false // overloaded: which arm runs is a runtime question
	}
	return own[0].TotalArgs(), true
}

func (es *EmitState) RecordDynApply(args []core.Value, fn, out core.Value, pos core.SrcPos) (int, bool) {
	if !es.Active() {
		return 0, false
	}
	// A paren-bounded apply that ALSO dispatched through the `apply` word
	// (`(v comp/v apply)`, `(x r apply)` over a gradual r) registered a
	// pending unit apply before this event collapsed the tape: the event
	// now owns the apply. Resolved FIRST because it changes three decisions
	// below — a gradual lead is admitted only under the apply word, the
	// word's unquote lets a `/v`-read lead apply, and the EVENT-lead arity
	// gate may TRIM the window before the ops are laid out — and the pending
	// entry is consumed once the event records.
	applyIdx := -1
	if len(es.units) > 0 {
		u := es.units[len(es.units)-1]
		for i, p := range u.pendingApply {
			if p.id == fn.ID {
				applyIdx = i
				pos = p.pos // the op raises where the interpreter's `apply` does
				break
			}
		}
	}
	// fn must be a genuine fn-value residual — or a gradual value the apply
	// WORD applies, which is a fn or the word's own no-match at run time.
	if !core.IsFnValueResidual(fn) && applyIdx < 0 {
		return 0, false
	}
	// The head's binding name for the lowered op's own diagnostics
	// (CompiledFn.DynApplyName) — read BEFORE the pendingApply consume below,
	// which is what tells a bare read apart from an `apply`-word arrival.
	headName := es.dynApplyHeadName(es.openUnitRec(), fn, args)
	// The paren window consumed a bare read of this local (NUR123
	// accounting): an accepted value-semantics lowering.
	es.creditWordRead(fn.ID)
	// A QUOTED fn value at the trailing position stays INERT in the
	// interpreter (the paren never collapses — `(1 2 (quote (fn …)))` leaves
	// [1 2 fn]); only a READ-substituted arrival (unquoted by the check's
	// word substitution, mirroring the interpreter) applies. The VM op
	// strips the STORED value's construction-time quote to mirror the read
	// (callDynTrailTop), so an inline-quote must never record the apply.
	if fn.Quoted && applyIdx < 0 {
		return 0, false
	}
	// A lead a later dispatch collected past (NUR121): its argument is that
	// dispatch's result, not what the interpreter's lead would collect.
	if es.hazardLead(fn) {
		return 0, false
	}
	// The lowered apply (OpCallDynTrailTop) nets EXACTLY ONE value (nout: 1
	// below). boru fns can return 0 or multiple values, so refuse the lowering
	// for a CONCRETE callee (a baked `/v` reference) that is not provably
	// single-valued: a multi-return callee miscompiles (`(1 2 pair/v)` with
	// pair → [Integer Integer] compiled to [2 1] vs the interpreter's [1 2])
	// and a zero-return callee underflows the following STORE_LOCAL/DROP. A
	// fn-typed CARRIER (a `comp:Function` param/captured value) carries no
	// static return arity here; it stays lowered (the comparator convention),
	// matching the interpreter's single-result paren auto-dispatch.
	if !fnConcreteSingleValuedOrCarrier(fn) {
		return 0, false
	}
	fnOp, ok := es.resolveOperand(fn)
	if !ok {
		return 0, false
	}
	// WINDOW TRIM. The lowered apply consumes exactly as many window values as
	// it is given operands, so that count must be the CALLEE'S OWN ARITY. When
	// the arity is provably smaller than the window the interpreter
	// UNDER-APPLIES, and the compiled lane must too — see applyWindowArity.
	arity, arityKnown := applyWindowArity(es, fn)
	refuse := ""
	switch {
	case arityKnown && arity > len(args):
		// INSUFFICIENT ARGS: the interpreter leaves the fn UNAPPLIED in the
		// residual (`(5 (mk2 10))` with a 2-arg callee nets [5, fn]). Nothing
		// here models that, so it must REFUSE — and a bare decline is not
		// enough: the collapse site's fallback (RegisterTrailingApply → the
		// body-residual lowering) will lower the window anyway and answer a
		// silent 15. Measured, declining quietly here turned the old
		// event-lead refusal into a wrong answer.
		refuse = "trailing fn-value apply with fewer args than the callee's arity"
	case arityKnown:
		args = args[len(args)-arity:] // keep the TOP arity; deeper values survive
	case fnOp.kind == opEvent && applyIdx < 0:
		// An EVENT lead with no provable arity keeps the standing refusal: the
		// KeepQ lowering would consume the whole window on the unquoted path.
		refuse = "trailing fn-value apply over a call result (runtime quote state unknown)"
	}
	// ONE refusal site for both arms, deliberately: the refusal-site census is
	// a downward ratchet (test/go/langspec refusalSiteCeiling), so a second
	// MarkUncompilable here would raise the count even though the shapes it
	// refuses are strictly fewer than before.
	if refuse != "" {
		es.MarkUncompilable(refuse)
		return 0, false
	}
	// Under the `apply` WORD (a pending entry) every value beneath the lead
	// is DATA: applyHandler re-steps the fn over the RESOLVED stack, where
	// a fn value on the value stack never dispatches — the closure re-step
	// matched it against the closure's own signature (`kk/v (ss kk/v)
	// apply` binds kk to the g:Function param), and a fn-typed carrier
	// lead's re-step binds it the same way (`(n/v f/v apply)` hands the
	// numeral to f — the thirty-second increment). A paren window with no
	// apply word keeps declining a fn-valued arg: inside the paren that
	// value was a TOKEN the interpreter stepped, and nothing has established
	// it is not an applicable of its own.
	ops := make([]EmitOperand, 0, len(args)+1)
	ops = append(ops, fnOp)
	for i := len(args) - 1; i >= 0; i-- {
		if core.IsFnValueResidual(args[i]) && applyIdx < 0 {
			return 0, false
		}
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return 0, false
		}
		ops = append(ops, op)
	}
	// An EVENT-provenance fn — the direct result of a compiled call — arrives
	// WITHOUT the interpreter's read substitution, so its runtime quote
	// state must SURVIVE: a callee returning `quote (fn …)` stays INERT in
	// the interpreter ([args, fn] is the residual — probe-pinned: `def
	// choose fn [[][Function][quote (fn …)]]  (1 2 choose)`, PR #280
	// review) while an unquoted anonymous result auto-applies (`(1 2
	// (mk))` → 3). OpCallDynTrailKeepQ preserves exactly that: no strip, a
	// Quoted runtime value leaves the window untouched. (This retires the
	// former hard-refusal here — "runtime quote state unknown" — which
	// held while the only ops were the read-mirror strip and the apply
	// word's unquote.) A local / const / dyn-scope arrival keeps the
	// stripping op — a stored read IS substituted (the strip's contract).
	keepQuote := fnOp.kind == opEvent && applyIdx < 0
	unquote := false
	if applyIdx >= 0 {
		u := es.units[len(es.units)-1]
		u.pendingApply = append(u.pendingApply[:applyIdx], u.pendingApply[applyIdx+1:]...)
		unquote = true
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: wordDynApply, ops: ops, nout: 1, pos: pos, dynApply: len(args), dynApplyUnquote: unquote, dynApplyKeepQuote: keepQuote, dynApplyName: headName, dynApplyOne: !core.IsFnValueResidual(fn)}})
	es.setProduced(out, seq)
	return len(args), true
}

// RecordDynApplyName records the same fn-value apply as RecordDynApply, but
// resolves the FN through the NAME's def-site operand (its evDynBind event)
// instead of the value's own ID — the §4.3 capture fallback (check's
// recordFnValueApplyFallback): a call of an installed factory closure whose
// unit carries CONSTRUCTION-SCOPE captures. The install re-minted the
// binding's value ID (InstallFnDef wraps a fresh Function value), so
// resolveOperand cannot place it; the def site's recorded operand is the
// SAME runtime value under its original identity (the factory call's out —
// typically a promoted local carrying the OpPushClosure result, whose baked
// captures invokeClosure installs VM-native). The event-provenance
// quote-state refusal RecordDynApply applies does not: this is a WORD-read
// arrival (the interpreter dispatches the installed name regardless), and
// the caller declines a quoted binding up front. Scoped to the TOP-LEVEL
// frame — a def site inside a fn unit records its bind in that unit's
// fragment, out of this scan's reach, and declines.
func (es *EmitState) RecordDynApplyName(name string, args []core.Value, fn, out core.Value, pos core.SrcPos) bool {
	if !es.Active() || name == "" || len(es.units) != 1 {
		return false
	}
	if !core.IsFnValueResidual(fn) || fn.Quoted {
		return false
	}
	if !fnConcreteSingleValuedOrCarrier(fn) {
		return false
	}
	var fnOp EmitOperand
	found := false
	for i := len(es.frames[0]) - 1; i >= 0 && !found; i-- {
		ev := &es.frames[0][i]
		if ev.kind != evDynBind || ev.dyn == nil || ev.dyn.name != name || !ev.dyn.bindsValue() {
			continue
		}
		switch {
		case ev.dyn.src.kind != opNone:
			fnOp, found = ev.dyn.src, true
		case ev.dyn.srcSeq >= 0:
			fnOp, found = EventOperand(ev.dyn.srcSeq, 0), true
		default:
			return false // the def site itself had no operand home
		}
	}
	if !found {
		return false
	}
	ops := make([]EmitOperand, 0, len(args)+1)
	ops = append(ops, fnOp)
	for i := len(args) - 1; i >= 0; i-- {
		if core.IsFnValueResidual(args[i]) {
			return false
		}
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops = append(ops, op)
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: wordDynApply, ops: ops, nout: 1, pos: pos, dynApply: len(args)}})
	es.setProduced(out, seq)
	return true
}

// captureArmResidual mirrors the loop side's all-inert residual capture
// (RecordLoop's net-drivers arm) for a BRANCH arm: a multi-value arm's residual
// is recorded as a resolved operand list so lowerFragment's re-push arm can
// reconstruct it per taken path (`if c [99] [1 2]` — the 1-vs-2 variadic
// merge). Unlike the loop side the list MAY name event-produced values: an arm
// runs ONCE per taken path, so a residual event promoted to a frame local
// (planValueDefLocals force-promotes every entry of a whole-captured arm
// residual) re-pushes from its slot in exact residual order, exactly as the
// PROGRAM residual's forceOrder linearisation already does. Without the whole
// list the lowerer could only COUNT the sim slots, and a stray leftover made
// the count match while the values did not: `if true [def c (7777 add 1) 99
// (each …)] [7 8]` seated the `add` result where the interpreter leaves 99.
// A loop body keeps the all-inert restriction (its residual re-pushes on EVERY
// iteration, where a frame slot holds only the last one) — RecordLoop screens
// that list itself. The parked-fn screen matches the loop's: a Function value
// in the region auto-applies in the interpreter when a later value lands above
// it, so a verbatim re-push would diverge — leave those uncaptured (the arm
// then keeps its refusal). Single-value arms (residualN < 2) never need the
// capture.
func (es *EmitState) captureArmResidual(frag *EmitFragment, stk []core.Value) {
	if frag == nil || len(stk) < 2 {
		return
	}
	ops := make([]EmitOperand, 0, len(stk))
	for i := range stk {
		if core.SigTypeMatches(stk[i], core.TFunction) {
			return
		}
		op, ok := es.resolveOperand(stk[i])
		if !ok {
			return
		}
		ops = append(ops, op)
	}
	frag.residualOps = ops
}

// RecordLoop records a counted/range `for`: start/end/step operand
// values (start and step must resolve to constants in Stage 2; the
// end may be computed), the body fragment (final fixed-point round),
// and the iterator's slot. The body either nets exactly one value
// per iteration or nothing (a net-0 or diverging body). out is the
// dispatch's result carrier — registered so the dispatch isn't
// re-recorded, and marked VARIADIC at lowering, so only the program
// residual may absorb the accumulation.
func (es *EmitState) RecordLoop(start, end, step core.Value, bodyRef core.EmitFragmentRef, bodyStk []core.Value, iterID string, out core.Value, regionN int, pos core.SrcPos) {
	body := asFragment(bodyRef)
	if !es.Active() {
		return
	}
	// Claim the just-closed analysis's carried-def slot inits (EndLoopCarried)
	// up front so an early refusal below never leaks them to a LATER loop.
	carried := es.pendingCarried
	es.pendingCarried = nil
	if body == nil {
		es.recordLoopEvent("for", nil, nil, nil, iterID, out, 0, "body not captured")
		return
	}
	startOp, ok1 := es.resolveOperand(start)
	endOp, ok2 := es.resolveOperand(end)
	stepOp, ok3 := es.resolveOperand(step)
	if !ok1 || !ok2 || !ok3 {
		es.MarkUncompilable("for: range of unknown provenance")
		return
	}
	rangeOpOK := func(op EmitOperand) bool { return op.kind == opConst || op.kind == opLocal }
	if !rangeOpOK(startOp) || !rangeOpOK(stepOp) {
		// A const or a re-pushable frame LOCAL (a param read — `for [n 5]
		// [...]`) lowers via the same pushOperand path the computed-END case
		// proved out; an EVENT-produced start/step keeps the refusal (its
		// value lives on the sim at its producer, not re-pushable here).
		es.MarkUncompilable("for: computed range start/step (Stage 2 follow-on)")
		return
	}
	lp := &emitLoop{start: startOp, end: endOp, step: stepOp, iterSlot: -1, pos: pos, carried: carried}
	es.recordLoopEvent("for", lp, body, bodyStk, iterID, out, regionN, "")
}

// RecordWhile records a CONDITION loop (`while [cond] [body]`, the
// thirty-seventh increment) on the counted loop's own frame: the loop runs
// FOR_SETUP / FOR_NEXT over an unbounded count (a scratch iterator, never
// read), and every iteration lowers the condition fragment to its ONE value
// and exits on a falsy one through a FLOW_BREAK — which pops the loop frame
// and trims the round exactly as a `break` does. The interpreter reads the
// condition region's LAST value and drops the rest, and raises on an empty
// region; the lowered shape is a condition netting exactly one value, and
// any other count refuses (the empty condition's runtime error keeps the
// interpreter's fallback). Body classification, the iterator slot, the
// event and its result marks are RecordLoop's (recordLoopEvent).
func (es *EmitState) RecordWhile(condRef, bodyRef core.EmitFragmentRef, condStk, bodyStk []core.Value, iterID string, out core.Value, pos core.SrcPos) {
	cond, body := asFragment(condRef), asFragment(bodyRef)
	if !es.Active() {
		return
	}
	carried := es.pendingCarried
	es.pendingCarried = nil
	lp := &emitLoop{iterSlot: -1, pos: pos, carried: carried, cond: cond}
	refusal := ""
	if cond == nil || body == nil {
		refusal = "body not captured"
	} else if condStk = es.stripZeroOutPhantoms(condStk); len(condStk) != 1 {
		refusal = "condition nets " + strconv.Itoa(len(condStk)) + " values, not one"
	} else {
		op, ok := es.resolveOperand(condStk[0])
		if !es.residualStands("while: ", condStk[0], op, ok, cond, "condition") {
			return
		}
		lp.condOut = op
		lp.start, _ = es.resolveOperand(core.NewInteger(0))
		lp.step, _ = es.resolveOperand(core.NewInteger(1))
		lp.end, _ = es.resolveOperand(core.NewInteger(math.MaxInt64))
	}
	es.recordLoopEvent("while", lp, body, bodyStk, iterID, out, 0, refusal)
}

// recordLoopEvent is the loop RECORD shared by RecordLoop (`for`) and
// RecordWhile: the body's per-iteration residual classification, the
// iterator slot, the event and its result marks. A caller's own refusal
// (refusal != "") is raised here under the loop's word, so the two loops
// share one refusal site.
func (es *EmitState) recordLoopEvent(word string, lp *emitLoop, body *EmitFragment, bodyStk []core.Value, iterID string, out core.Value, regionN int, refusal string) {
	if refusal != "" {
		es.MarkUncompilable(word + ": " + refusal)
		return
	}
	// A body ending in a 0-output statement guard (`if c [def …] []` — the
	// loop-carried conditional-rebind shape) leaves only the guard's phantom
	// None; stripping it classifies the body as the SIDE-EFFECT (zeroOut)
	// loop it is at run time, exactly like the arm/fn residual strips.
	bodyStk = es.stripZeroOutPhantoms(bodyStk)
	if len(bodyStk) > 0 && !fragDiverges(body) {
		if es.setLoopBodyApply(body, bodyStk) {
			// A per-iteration dynamic apply (`(mk2 i) 10`): the body nets one applied
			// value, lowered via OpCallDynamic over the leading fn (body.applyArgs).
			lp.hasBodyOut = true
		} else if len(bodyStk) > 1 {
			// The body nets MORE THAN ONE value per iteration (`for 2 ['e' 'f']`
			// pushes both 'e' and 'f' each pass). The lowered loop nets exactly one
			// value per iteration, so keeping only bodyStk[last] would silently DROP
			// the rest — a miscompile ([e f e f] -> [f f]). Refuse and let the
			// interpreter island run the multi-value body faithfully.
			// NET DRIVERS (plan Phase 5): ride the residualN>1 fragment
			// reconciliation (the Stage-A multi-value arm model): the TOP
			// operand rides as bodyOut, residualN carries the full count, and
			// lowerFragment seats every event-produced value (or refuses the
			// inert-tail shapes it cannot reconstruct — the sound fallback).
			// Per-iteration values then accumulate exactly as the interpreter.
			// PARKED-FN screen (mirrors the program-residual fn-boundary
			// guard): a Function value anywhere in the region auto-applies in
			// the interpreter when a later value lands above it — including
			// ACROSS iterations (iteration k's top sits below k+1's first) —
			// so a verbatim accumulation would diverge. Keep those refused.
			for i := range bodyStk {
				if core.SigTypeMatches(bodyStk[i], core.TFunction) {
					es.MarkUncompilable(word + ": body nets multiple values per iteration")
					return
				}
			}
			bodyOut, ok := es.resolveOperand(bodyStk[len(bodyStk)-1])
			if !es.residualStands(word+": ", bodyStk[len(bodyStk)-1], bodyOut, ok, body, "body result") {
				return
			}
			for i := range bodyStk[:len(bodyStk)-1] {
				if op, okOp := es.resolveOperand(bodyStk[i]); okOp &&
					!es.residualStands(word+": ", bodyStk[i], op, true, body, "") {
					return
				}
			}
			// ALL-INERT residual (`for 3 [1 2]`): no entry is event-produced,
			// so the residualN reconciliation cannot seat them from the sim —
			// capture the full operand list for a per-iteration re-push instead.
			allInert := true
			var inertOps []EmitOperand
			for i := range bodyStk {
				op, okOp := es.resolveOperand(bodyStk[i])
				if !okOp || op.kind == opEvent {
					allInert = false
					break
				}
				inertOps = append(inertOps, op)
			}
			if allInert {
				body.residualOps = inertOps
			}
			body.residualN = len(bodyStk)
			lp.bodyOut, lp.hasBodyOut, lp.multiOut = bodyOut, true, true
		} else {
			bodyOut, ok := es.resolveOperand(bodyStk[len(bodyStk)-1])
			if !es.residualStands(word+": ", bodyStk[len(bodyStk)-1], bodyOut, ok, body, "body result") {
				return
			}
			lp.bodyOut, lp.hasBodyOut = bodyOut, true
			if pr, okP := es.producedBy[bodyStk[0].ID]; okP && bodyOut.kind == opEvent &&
				es.eventInfo[pr.seq].splitBound &&
				!core.SigTypeMatches(bodyStk[0], core.TFunction) {
				// The body's sole residual is a SPLIT-BOUND region (S9.2a: `for 3
				// [ def acc (for 2 [1]) ]` — the def consumed the first value,
				// the statically-counted rest is the iteration's contribution).
				// Ride the multiOut/allowVariadic reconciliation so the region
				// stands as the body residual; a Function-bearing region keeps
				// the refusal (the parked-fn accumulation hazard).
				lp.multiOut = true
			}
		}
	}
	slot, ok := es.units[len(es.units)-1].localByID[iterID]
	if !ok {
		es.MarkUncompilable(word + ": iterator slot not registered")
		return
	}
	lp.body, lp.iterSlot = body, slot
	seq := es.appendEvent(EmitEvent{kind: evLoop, loop: lp})
	es.SiteCounts[SiteMono]++
	es.setProduced(out, seq)
	f := es.eventInfo[seq]
	if len(body.applyArgs) > 0 {
		f.applyLoop = true
	}
	f.regionMayBeFn = regionValsMayBeCallable(bodyStk)
	if lp.hasBodyOut {
		// A value-producing loop leaves a runtime-variable count (one per-iteration
		// value, N unknown at compile time) — variadic, like lowerLoop marks
		// lw.variadic. Only the program residual absorbs it — except the S5
		// first-value def bind, whose splice-at-depth lowering needs the exact
		// STATIC region size the caller computed (0 when not static).
		f.variadicResult = true
		f.regionN = regionN
		if len(bodyStk) > 0 && bodyStk[0].Parent != nil {
			f.firstElemType = bodyStk[0].Parent
		}
	} else {
		// A SIDE-EFFECT loop (body nets 0 per iteration — `for n [acc set …]`)
		// leaves ZERO runtime values, deterministically: a zero-output event, NOT a
		// variadic result. Marking it zeroOut lets a fn/closure body that ends in
		// (or discards, via `def _ (for …)`) such a loop drop its result and RET
		// cleanly, instead of refusing "body leaves extra values"/"variadic loop
		// value". The loop's side effects are emitted events and still run.
		f.zeroOut = true
	}
	es.eventInfo[seq] = f
}

// setLoopBodyApply detects a loop body whose residual is a LEADING fn VALUE with
// trailing STATIC args — the per-iteration dynamic apply `for n [(mk2 i) 10]`,
// where each iteration mk2 returns a closure that is applied to 10. It mirrors
// resolveDynamicApply's leading-fn-carrier case, but seats the apply on the loop
// body fragment (body.applyArgs) so lowerFragment emits the OpCallDynamic per
// iteration. Returns true when the shape was recognised and seated.
//
// Soundness: bodyStk[0] must be a Function-typed CARRIER (always callable —
// the one inert fn shape, an `f/v` reference, is a concrete const, not a carrier)
// produced by an event (so it lands on the sim top after the body events lower),
// and every trailing arg must be a non-fn, non-dynamic value resolving to a
// RE-PUSHABLE operand (const / local / type). A computed (event) arg is already
// on the sim, so it is excluded here and the residual instead fails lowerFragment's
// sole-fn check and refuses — never a double-push.
func (es *EmitState) setLoopBodyApply(body *EmitFragment, bodyStk []core.Value) bool {
	if es == nil || body == nil || len(bodyStk) < 2 || !core.IsFnTypedCarrier(bodyStk[0]) {
		return false
	}
	fnOp, ok := es.resolveOperand(bodyStk[0])
	if !ok {
		return false
	}
	// A frame-LOCAL fn read (an `args.N` fold of a Function param inside a fn
	// unit): nothing sits on the sim after the body events, so the fragment
	// re-pushes the local before the args (applyFn). An event-produced fn (the
	// factory `(mk2 i)`) is already on the sim top — the original layout.
	if fnOp.kind == opLocal {
		// The lowered apply is emitted at one of the iteration's two ends, so
		// the fn read must bound the body's other statements in source order:
		// every event strictly before it (apply-last, the hoist) or strictly
		// after it (apply-first — a continue in the applied body must skip
		// them). A mid-body apply declines and keeps the refusal.
		fnPos := bodyStk[0].Pos()
		if fnPos.Row == 0 && fnPos.Col == 0 {
			// A folded args.N read loses its own position; the apply's ARG
			// literals sit inside the same paren, so any of theirs stands in.
			for _, a := range bodyStk[1:] {
				if a.Pos().Row != 0 || a.Pos().Col != 0 {
					fnPos = a.Pos()
					break
				}
			}
		}
		if fnPos.Row == 0 && fnPos.Col == 0 {
			return false // no source anchor — cannot order the apply
		}
		before, after := 0, 0
		for i := range body.events {
			p := eventPos(body.events[i])
			if p.Row == 0 && p.Col == 0 {
				continue // no position recorded — count as neither side
			}
			if p.Row < fnPos.Row || (p.Row == fnPos.Row && p.Col < fnPos.Col) {
				before++
			} else {
				after++
			}
		}
		if before > 0 && after > 0 {
			return false
		}
		body.applyFn = &fnOp
		body.applyFirst = after > 0
	} else if fnOp.kind != opEvent {
		return false
	}
	applyArgs := make([]EmitOperand, 0, len(bodyStk)-1)
	for _, a := range bodyStk[1:] {
		if a.Dynamic || core.IsFnValueResidual(a) {
			return false
		}
		op, okArg := es.resolveOperand(a)
		if !okArg || (op.kind != opConst && op.kind != opLocal && op.kind != opType) {
			return false
		}
		applyArgs = append(applyArgs, op)
	}
	body.applyArgs = applyArgs
	return true
}

// RecordFallback records an interpreter-island fallback (Stage 5). span
// is the self-contained re-runnable token sequence; ins are the threaded
// stack inputs (sig order, deepest first) — empty for a fully-baked
// span; out is the dispatch's single result carrier, registered so the
// island's result flows downstream (to the residual, or another
// fallback; a downstream TYPED dispatch consuming a dynamic result still
// refuses via anyDynamicCarrier, preserving soundness). Returns false
// when an input's provenance is unknown — the caller then lets the
// normal refusal stand.
func (es *EmitState) RecordFallback(span core.FallbackSpan, ins []core.Value, out core.Value, pos core.SrcPos) bool {
	if !es.Active() {
		return false
	}
	ops := make([]EmitOperand, len(ins))
	for i, in := range ins {
		op, ok := es.resolveOperand(in)
		if !ok {
			return false
		}
		ops[i] = op
	}
	span.NIn = len(ins)
	idx := len(es.fallbacks)
	es.fallbacks = append(es.fallbacks, span)
	seq := es.appendEvent(EmitEvent{kind: evFallback, fb: emitFallback{spanIdx: idx, ins: ops, pos: pos}})
	es.SiteCounts[SiteDynamic]++
	// A VARIADIC REGION result (the forty-eighth increment): the word's
	// check model IS "0-or-more values", so record the island as a region.
	// The island's ONE sim slot is already the region's representation —
	// runFallback appends whatever the re-run produced — so nothing changes
	// in what is emitted; the mark is what stops a consumer seating the run
	// at a fixed count. `error` over a maybe-raising body is the producer:
	// the caught path nets zero, the pass-through nets one.
	if callVariadicRegion([]core.Value{out}) {
		f := es.eventInfo[seq]
		f.variadicResult = true
		f.variadicRegion = true
		// …and it may leave a CALLABLE, which no consumer of a region can
		// re-step (eventFlags.regionMayBeFn / NUR129). An island's run is the
		// INTERPRETER executing arbitrary code, so what it appends is not
		// bounded by the modelled out at all — `def xs [1] def g fn x:Integer
		// Integer [x add 1] 5 do [if ((xs 0 getr) eq 1) [g/v] [1 div 0]]
		// error [drop]` passes a fn value through the handler, which the
		// interpreter re-steps against the 5 beneath it for [6] and the
		// compiled lane seated as data, raising uncalled_function (Codex, PR
		// #448). Marked unconditionally: a narrower claim would have to be a
		// claim about interpreted code the recorder never saw.
		f.regionMayBeFn = true
		es.eventInfo[seq] = f
	}
	es.setProduced(out, seq)
	return true
}

// RememberOriginal records a CONCRETE value against its own ID so that
// when a carrier-strip reduces it — toCarrier keeps Value.ID, so the
// stripped carrier shares the original's ID — the lowerer can recover the
// concrete value via materialise/origByID. The single recorder behind both
// strip provenance paths:
//
//   - top-level literals that StripToCarriers reduces to carriers (the
//     caller in engine.Run gates on the strip actually having happened:
//     same ID, now a carrier);
//   - impure-but-pure-data constructors that run in check mode and whose
//     result is stripped before reaching a downstream operand — notably the
//     predicate constructors (`Integer gt 10`), whose DepScalarInfo payload
//     toCarrier strips to a bare base-type carrier, losing the bound.
//
// A value that is already a carrier/dynamic, or has no ID, is a no-op
// (nothing concrete to recover). Skips while suspended or once the program
// is uncompilable (an uncompilable program is never lowered, so its
// origByID entries are never read).
func (es *EmitState) RememberOriginal(v core.Value) {
	if es == nil || es.suspended > 0 || !es.Active() {
		return
	}
	if v.Carrier || v.Dynamic || v.ID == "" {
		return
	}
	es.origByID[v.ID] = v
}

// atUnitRootFrame reports whether recording is at the CURRENT unit's root
// frame — no fragment open above it. The top unit's root frame is frames[0];
// a body unit's is the frame it opened at (fnUnitRec.rootFrame). Used by
// FoldFullStack, whose exactness argument is about THIS unit's stack.
func (es *EmitState) atUnitRootFrame() bool {
	if len(es.openUnitRecs) == 0 {
		return len(es.frames) == 1
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	return len(es.frames)-1 == rec.rootFrame
}

// FoldFullStack statically folds a full-stack word — depth / pick / roll —
// when the recorder can prove the simulated stack is EXACT, so the dispatch
// ELIDES: no event records, no opcode runs, and the fold's outputs carry
// known provenance (the P1.2 capability, checker-compiler-completeness-
// review §8.2(2); the class was previously a blanket provenance refusal).
//
//   - depth  → the count is the preserved stack's length, baked as a fresh
//     CONCRETE Integer (resolveOperand materialises it as a const);
//   - pick n → a COPY of the picked entry (same ID — the re-push resolves
//     to the original's operand; an event-produced pick target is promoted
//     to a value-def frame local so both references have a home);
//   - roll n → the true permutation of the preserved entries (every ID kept;
//     the residual reconciliation's ordering machinery lays it out).
//
// Exactness is the whole soundness argument: the fold bakes the CHECK
// model's stack, so it is admitted only where that model provably mirrors
// the interpreter — the top frame of the top unit, no open mark window, and
// every preserved entry either a known operand home (event / local /
// materialisable const / type node) with no variadic producer (a variadic
// region's runtime count is not its model count). Anything else declines
// and the historical refusal path stands (slow, not wrong). An
// out-of-range n declines too: the interpreter raises there, and the
// fallback keeps the raise byte-identical.
func (es *EmitState) FoldFullStack(word string, args, preserved []core.Value) ([]core.Value, bool) {
	if es == nil || !es.Active() || es.suspended > 0 || !es.Compilable || es.markWindowSeq != 0 {
		return nil, false
	}
	// The exactness condition is per-UNIT, not per-program: what it needs is
	// that the recorder's simulated stack is the one the VM will hold HERE,
	// and a compiled body unit has its own stack discipline exactly as the
	// top unit does. What it cannot survive is an unclosed FRAGMENT — a
	// branch arm or a loop body mid-capture — whose events have not been
	// reconciled into any scope's residual yet.
	//
	// So: the current frame must be the CURRENT unit's root frame. For the
	// top unit that is frames[0], which is the old `len(es.frames) != 1`
	// test verbatim; for a body unit it is the frame the unit opened at
	// (fnUnitRec.rootFrame).
	unit := len(es.units) - 1
	if !es.atUnitRootFrame() {
		return nil, false
	}
	for _, v := range preserved {
		if v.ID == "" {
			return nil, false
		}
		if pr, ok := es.producedBy[v.ID]; ok {
			if es.eventInfo[pr.seq].variadicResult {
				return nil, false
			}
			// Inside a BODY UNIT the fold is admitted only over entries that
			// need no residual REBUILD, and an event-produced entry is
			// exactly the one that might.
			//
			// The top unit can take a permuted residual: seatResidualRebuild
			// (the forty-third increment) spills every simulated entry to a
			// frame local and re-pushes it in the recorded order. A body
			// unit has no such rebuild — its residual seating refuses a
			// result that ends up above a literal — so folding a permutation
			// there turns a sound ISLAND into a REFUSAL, which is backwards.
			// Measured: `(1 add 2) (3 add 4) 1 pick` islands in a fn / do /
			// each / module body and compiles at the top level, and an
			// unscreened per-unit fold refused all four
			// (TestVariationDifferential, the fifty-second increment).
			//
			// A const or local entry cannot be permuted into that shape: the
			// fold's output is re-pushable from the same operand homes in any
			// order. That is the whole of what a body unit can support today,
			// and it is what the graduated row needs.
			if unit > 0 {
				return nil, false
			}
			continue
		}
		if _, ok := es.units[unit].localByID[v.ID]; ok {
			continue
		}
		if core.IsConcrete(v) || core.IsBareTypeNode(v) {
			continue
		}
		return nil, false
	}
	switch word {
	case "depth":
		n := core.NewInteger(int64(len(preserved)))
		n.ID = core.GenerateID(core.IDPrefixForType(core.TInteger))
		return append(append([]core.Value(nil), preserved...), n), true
	case "pick", "roll":
		if len(args) != 1 || !core.IsConcrete(args[0]) {
			return nil, false
		}
		// A shuffle RE-PUSHES its values, and a value that arrives on the
		// stack is RE-STEPPED — a Function dispatches over what is beneath
		// it (NUR124's rule). The interpreter's pick/roll splices the
		// permutation back onto the tape, where the pointer fires any fn
		// that matches; the fold's output is data, so a preserved closure
		// answers `[5 fn fn]` where the interpreter applies it twice and
		// answers `[45]`. Measured, and PRE-DATING the residual rebuild
		// this guard was added beside (NUR131).
		//
		// The test is PROVEN-callable, not the wider possibly-callable one
		// the rebuild's screen uses, and the asymmetry is deliberate: the
		// rebuild is new machinery, so a wide screen costs only graduations
		// that were never realised, while this fold has live correct
		// compiles a wide screen would take with it — a def-bound `/v` read
		// shuffled over a literal (`5 g/v 0 pick` → 7) agrees on both lanes
		// today, because a read is not event-produced and the deopt
		// machinery covers it.
		for _, v := range preserved {
			if _, produced := es.producedBy[v.ID]; !produced {
				continue
			}
			if core.IsFnValueResidual(v) || core.SigTypeMatches(v, core.TFunction) {
				return nil, false
			}
		}
		nn, err := core.AsInteger(args[0])
		if err != nil || nn < 0 || int(nn) >= len(preserved) {
			return nil, false
		}
		idx := len(preserved) - 1 - int(nn)
		if word == "pick" {
			picked := preserved[idx]
			es.MarkValueDef(picked)
			return append(append([]core.Value(nil), preserved...), picked), true
		}
		out := make([]core.Value, 0, len(preserved))
		out = append(out, preserved[:idx]...)
		out = append(out, preserved[idx+1:]...)
		out = append(out, preserved[idx])
		return out, true
	}
	return nil, false
}

// RecordTrap records a TERMINAL trap for a check-mode-suppressed runtime error
// (an orphan gen, an unpack of a missing key): the checker is lenient at this
// point but the interpreter errors, so the compiled program raises the
// byte-identical error here via OpTrap instead of refusing the whole program.
// Only a TOP-LEVEL trap is recorded (frames and units both at depth 1): a trap
// inside a branch/loop/fn fragment is conditional and not yet modelled, so it
// returns false and the caller keeps the SuppressedRuntimeError blanket refusal.
// The first trap wins (execution can reach only one). Returns true when the trap
// is owned here (recorded now, or already trapping).
func (es *EmitState) RecordTrap(code, detail, word, hint string, pos core.SrcPos) bool {
	if !es.Active() || len(es.frames) != 1 || len(es.units) != 1 {
		return false
	}
	if es.trapAt != 0 {
		return true
	}
	es.trapAt = es.appendEvent(EmitEvent{kind: evTrap, trap: EmitTrap{
		spec: TrapSpec{Code: code, Detail: detail, Word: word, Hint: hint},
		pos:  pos,
	}})
	return true
}

// RecordTrapErr is RecordTrap for a fully-built interpreter BoruError: it
// serialises the whole diagnostic — code, detail, word, hint, and the
// structured payload (secondary spans, notes, suggestions) — into the trap so
// the compiled OpTrap raises a report byte-identical to the interpreter's. Used
// where the check pass builds the interpreter's own error at compile time (a
// statically-definite unmatched dispatch, whose runtime values are provably the
// same the check pass saw — tryRecordUnmatchedDispatchTrap). Same top-level-only
// guard as RecordTrap.
func (es *EmitState) RecordTrapErr(ae *core.BoruError, pos core.SrcPos) bool {
	if ae == nil {
		return false
	}
	if !es.Active() || len(es.frames) != 1 || len(es.units) != 1 {
		return false
	}
	if es.trapAt != 0 {
		return true
	}
	es.trapAt = es.appendEvent(EmitEvent{kind: evTrap, trap: EmitTrap{
		spec: TrapSpec{
			Code: ae.Code, Detail: ae.Detail, Word: ae.Src, Hint: ae.Hint,
			Spans: ae.Spans, Notes: ae.Notes, Suggestions: ae.Suggestions,
		},
		pos: pos,
	}})
	return true
}

// RecordDispatchRematchValues is the value-level entry over
// RecordDispatchRematch: it resolves each window VALUE to its operand
// (a make-result carrier resolves to its producing event; a concrete
// forward token to a const) and declines — leaving the caller's refusal to
// stand — when any value has no resolvable provenance. writtenOff/nWritten
// are the render bound (DispatchSpec.{WrittenOff,NWritten}): the contiguous
// vals slice forming the written tuple the interpreter's error renders.
func (es *EmitState) RecordDispatchRematchValues(word string, vals []core.Value, writtenOff, nWritten int, pos core.SrcPos) bool {
	if !es.Active() || len(vals) == 0 {
		return false
	}
	ops := make([]EmitOperand, len(vals))
	for i, v := range vals {
		// A READ-substituted fn-carrier window value (Stage 1's fn-carrier
		// side table — defReads carries its ID, no FnDefInfo payload)
		// poisons the window: the interpreter WORD-dispatches that read
		// before this word ever collects, so the static window this
		// rematch would replay never exists at run time (`def q (if c
		// [fn-arm] [fn-arm])  add 1 (q 5)` re-matched add over [1, fn, 5]
		// and raised signature_error where the interpreter computes 7).
		// Decline; the caller's refusal stands and the program falls back.
		if core.IsFnTypedCarrier(v) {
			if _, isFn := v.Data.(core.FnDefInfo); !isFn {
				if _, read := es.defReads[v.ID]; read {
					return false
				}
			}
		}
		op, ok := es.resolveOperand(v)
		if !ok {
			return false
		}
		ops[i] = op
	}
	return es.RecordDispatchRematch(word, ops, writtenOff, nWritten, pos)
}

// RecordDispatchRematch records a TERMINAL runtime-rematch trap
// (OpDispatchRematch) for a statically-failed dispatch whose window held
// CARRIER operands: the failure is expected but not statically DEFINITE (a
// carrier's runtime tag could still match), so instead of serialising the
// error now (RecordTrapErr) the compiled program re-runs the match over the
// live values and raises the shared rich diagnostic built over them — or
// defers to the interpreter when the match unexpectedly succeeds. ops are
// the window operands in the order the failed match examined them;
// writtenOff/nWritten are the render bound over their contiguous slice
// (nWritten 1.., the slice inside the window — anything else declines, the
// producer's proof did not hold). Same top-level-only guard and
// first-trap-wins latch as RecordTrap.
func (es *EmitState) RecordDispatchRematch(word string, ops []EmitOperand, writtenOff, nWritten int, pos core.SrcPos) bool {
	if word == "" || len(ops) == 0 {
		return false
	}
	if nWritten < 1 || writtenOff < 0 || writtenOff+nWritten > len(ops) {
		return false
	}
	if !es.Active() || len(es.frames) != 1 || len(es.units) != 1 {
		return false
	}
	if es.trapAt != 0 {
		return true
	}
	es.trapAt = es.appendEvent(EmitEvent{kind: evTrap, trap: EmitTrap{
		rematchWord:       word,
		rematchOps:        append([]EmitOperand(nil), ops...),
		rematchWrittenOff: writtenOff,
		rematchNWritten:   nWritten,
		pos:               pos,
	}})
	return true
}

// RecordTypedBind records the runtime validate/reparent step of a typed
// value-def (`def x:Pos n`) whose constraint is a refinement and whose body is
// DYNAMIC — the compiled replacement for the "dynamic refinement
// reparent/validate is interpreter-only" refusal. The event pops the body
// operand, runs RunTypedBind (the interpreter-mirroring OpBindTyped), and
// pushes the value the interpreter binds; `out` is the CHECK-mode binding the
// def handler is about to install (the reparented carrier / the base-tagged
// carrier for a DepScalar), returned with a FRESH provenance ID registered
// against the new event so downstream references to the binding resolve to the
// bind's RESULT, not to the raw body operand (out shares the body's ID —
// ReparentValue preserves it — and without the remint a reference would
// resolve straight to the un-reparented param local: miscompile B's exact
// mechanism, design/MISCOMPILE-HUNT-FINDINGS.0.md §B).
//
// Declines (returning out unchanged and false) when recording is inactive or
// the body is CONCRETE — a static typed-def's reparent rides the const pool
// faithfully and must keep today's proven path — or when the body operand has
// no resolvable provenance; the caller then falls back to the refusal mark,
// which itself no-ops for the concrete/inactive cases.
func (es *EmitState) RecordTypedBind(spec core.TypedBindSpec, in, out core.Value, pos core.SrcPos) (core.Value, bool) {
	if !es.Active() {
		return out, false
	}
	// A CONCRETE operand declines for the refine/DepScalar kinds (their
	// const-pool bake is proven), but a fn-PREDICATE bind is a runtime
	// evaluation for every body shape — the predicate can transform, raise,
	// or read live state — so concrete operands record too (the 2026-07-15
	// flip finding: a check-lenient bake bound the raw value where the
	// interpreter runs the transform).
	if core.IsConcrete(in) && spec.Kind != core.TypedBindPredicate {
		return out, false
	}
	op, ok := es.resolveOperand(in)
	if !ok {
		return out, false
	}
	sp := spec
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: wordTypedBind, ops: []EmitOperand{op}, nout: 1, pos: pos, typedBind: &sp,
	}})
	out.ID = core.GenerateID(core.IDPrefixForType(out.Parent))
	es.setProduced(out, seq)
	return out, true
}

// RememberStrippedOriginals records the pre-strip original of each value that
// StripToCarriers actually reduced to a carrier (same preserved ID), so the
// lowerer can later recover the concrete literal. Values toCarrier kept
// concrete need no recovery and are skipped. pre and stripped are parallel.
func (es *EmitState) RememberStrippedOriginals(pre, stripped []core.Value) {
	for i := range stripped {
		if stripped[i].Carrier && pre[i].ID != "" && pre[i].ID == stripped[i].ID {
			es.RememberOriginal(pre[i])
		}
	}
}

// RecordPoly classifies a partitioned (per-alternative) dispatch.
// Stage 1 cannot lower it; later stages emit CALL_NATIVE_POLY.
func (es *EmitState) RecordPoly(word string) {
	if !es.Active() {
		return
	}
	es.SiteCounts[SitePoly]++
	es.MarkUncompilable("polymorphic dispatch at " + word)
}

// RecordCall records one resolved dispatch. args are in signature
// order (position 0 = top of stack); outs are the carrier results.
// forceDynOut bypasses the dynamic-output refusal when the caller
// (dynOutNativeOK) has proven the dispatch is a concrete-args core builtin
// whose dynamic result is merely a declared-Any return. quoteInertOK bypasses
// the quoted-operand refusal when the caller (quoteOperandInertOK) has proven
// the dispatch is a module inner native whose quoted operands are inert Atom
// consts (the query DSL's table names).
func (es *EmitState) RecordCall(word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos, forceDynOut, quoteInertOK bool) {
	es.noteAppliedByWord(word, args, outs)
	if !es.Active() {
		return
	}
	es.noteFnRiskFields(word, args, outs)
	if es.recordCallElided(word, sig, args, outs, pos) {
		return
	}
	if es.recordShuffleElided(word, sig, args, outs) {
		return
	}
	if es.recordCallRefusal(word, sig, args, outs, pos, forceDynOut, quoteInertOK) {
		return
	}
	ops, ok := es.RecordCallOperands(word, sig, args)
	if !ok {
		return
	}
	// Phase B: claim this dispatch's region capture, if it has one, and fill
	// the sources of the slots it actually took forward. Inert — nothing reads
	// Program.Regions yet; what it buys now is that the descriptor model is
	// exercised and gated over the whole corpus before OpCollect executes one.
	region := es.completeRegion(word, pos, args, ops)
	es.SiteCounts[SiteMono]++
	// A CompileValueDiverges word (div/mod) raises value-dependently: its
	// check-mode ReturnsFn drops the declared result (len(outs)==0) exactly on
	// the divergent path (a static-zero divisor), so treat that call as a
	// divergent terminal too — a closure body ending in it compiles with no RET
	// and the catching word wraps the raised error, instead of islanding.
	diverges := sig.CompileEffect.Has(core.CompileDiverges) ||
		(sig.CompileEffect.Has(core.CompileValueDiverges) && len(outs) == 0)
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos, diverges: diverges, region: region}})
	// A fallible multi-value catch body reaching the generic path (the
	// closure probe declined): same variadic mark as RecordClosureCall —
	// the caught path nets 1 where the static seat expects N (L-DO).
	if es.catchVariadicFor(sig) {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
	// A VARIADIC REGION result (the GROWING direction, NUR067): the word's
	// check model IS "0-or-more values", so record the event as a region —
	// variadicResult keeps every runtime-variable-count rule, variadicRegion
	// adds the loop-region lowering on top.
	if callVariadicRegion(outs) {
		if es.storedBodyFnResidual {
			// A branch body can leave a CALLABLE, and the winner's residual is
			// handed back as this handler's results — which the INTERPRETER
			// splices onto its tape and RE-STEPS, so `await {mode:'first'}
			// [[5 g/v]]` answers 6 where a region would leave [5 fn]. A region
			// carries no per-value seat to re-step from, so the program falls
			// back: NUR067's remaining edge, and a far narrower one than the
			// wholesale refusal it replaced.
			es.MarkUncompilable(word + ": a branch body can leave a CALLABLE value, which the interpreter re-steps out of the handler's results and a region cannot (NUR067)")
		} else {
			f := es.eventInfo[seq]
			f.variadicResult = true
			f.variadicRegion = true
			es.eventInfo[seq] = f
		}
	}
	// Carrier-identity de-collision (the deferred runtime-independence item, in
	// its targeted form). A call OUTPUT whose ID already maps to a PRIOR event is
	// a repeated identical computed call: `(context get 'n') add (context get
	// 'n')` issues two get events that each return the same deterministic-ID
	// result, so the second registration would overwrite the first and `add`
	// would resolve BOTH operands to the second event — the residual layout then
	// refuses "call results reordered". Mint a fresh ID so the two stack values
	// stay distinct (the outs slice is the carrierResults return, so the fresh ID
	// flows to the downstream consumer). Guarded twice: a SAME-event collision is
	// `dup`/`swap` returning an input by identity (`[args[0], args[0]]`), handled
	// by the DUP lowering, so skip pr.seq == seq; and a result that IS one of the
	// call's inputs (an identity/stack word passing a value through) keeps its ID,
	// so skip an output whose ID appears in args.
	gf := es.eventInfo[seq]
	gf.generic = true
	es.eventInfo[seq] = gf
	var argIDs map[string]bool
	for i := range outs {
		// A MODULE-FAMILY result with NO identity — the shared descriptor a
		// `$module` read surfaces (`ns.Module` is a stored Value, never
		// minted) — takes the event's: without an ID the registration below
		// is a no-op and the value reaches its consumer (`eq`, a residual)
		// with no compiled home. Minting here is what makes the recorded
		// dispatch (recordCallElided declines to elide it) reachable
		// downstream; the outs slice is the carrierResults return, so the
		// ID flows to the consumer exactly as the de-collision mint does.
		if outs[i].ID == "" && core.IsModuleFamilyValue(outs[i]) {
			outs[i].ID = core.GenerateID(core.IDPrefixForType(outs[i].Parent))
		}
		if pr, ok := es.producedBy[outs[i].ID]; ok && pr.seq != seq {
			if argIDs == nil {
				argIDs = make(map[string]bool, len(args))
				for _, a := range args {
					argIDs[a.ID] = true
				}
			}
			if !argIDs[outs[i].ID] {
				outs[i].ID = core.GenerateID(core.IDPrefixForType(outs[i].Parent))
			}
		}
		es.setProducedAt(outs[i], seq, i)
	}
}

// residualHasVariadicRegion reports whether any residual entry was produced by
// a VARIADIC REGION event — the one recorded slot standing for a runtime count
// (eventFlags.variadicRegion). Its presence disqualifies the residual from
// every fn-value-call classification: none of those arms can read a count.
func (es *EmitState) residualHasVariadicRegion(residual []core.Value) bool {
	for _, v := range residual {
		if pr, ok := es.producedBy[v.ID]; ok && es.eventInfo[pr.seq].variadicRegion {
			return true
		}
	}
	return false
}

// regionValsMayBeCallable reports whether any value a region's run is modelled
// to leave could be CALLABLE at run time. A fn value or a Function-typed
// carrier plainly can; so can a DYNAMIC one, whose runtime type the model does
// not bound. It is the question eventFlags.regionMayBeFn records, and it is
// asked in the widening direction on purpose: a region that cannot be
// re-stepped must not be consumed if it MIGHT carry something the interpreter
// would re-step.
func regionValsMayBeCallable(vals []core.Value) bool {
	for _, v := range vals {
		if v.Dynamic || core.IsFnValueResidual(v) || core.SigTypeMatches(v, core.TFunction) {
			return true
		}
	}
	return false
}

// storedBodyIsEmpty reports whether a stored-body-list element is a list of
// ZERO tokens — the one shape compileStoredBody declines whose residual is
// nonetheless KNOWN, namely none at all.
func storedBodyIsEmpty(elem core.Value) bool {
	lst, err := core.AsList(elem)
	return err == nil && !lst.IsNil() && len(lst.Slice()) == 0
}

// callVariadicRegion reports whether a dispatch's modelled residual is a
// VARIADIC REGION: exactly ONE out, and that out a variadic-spread carrier
// (core.NewVariadicCarrier — the check-side "0-or-more values of element
// type"). One out is what makes the region representable: the single recorded
// slot stands for the whole runtime run, exactly as a value-producing loop's
// does. A residual of any other length is an ordinary fixed-arity result and
// is left alone; no compile-pass producer mints a spread alongside other
// values, and one that did would owe its own region recording rather than
// riding this one.
func callVariadicRegion(outs []core.Value) bool {
	if len(outs) != 1 {
		return false
	}
	_, ok := core.IsVariadicSpread(outs[0])
	return ok
}

// recordCallElided reports whether a dispatch is ELIDED — already recorded by a
// structured hook, or a compile-time name resolution that produces nothing the
// VM runs. The caller returns without recording when this is true.
func (es *EmitState) recordCallElided(word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos) bool {
	// §8.2(3) poly-alias: the ReturnsFn recorded THIS dispatch as
	// OpCallUserPoly, and the caller then rebuilt the out carriers (the
	// gradual first-match-partition widening mints fresh IDs) — alias the
	// rebuilt IDs onto the recorded event and elide the generic record, so
	// downstream operand resolution reaches the poly event under the
	// widened carriers' identities.
	if lp := es.lastUserPoly; lp != nil && lp.word == word && len(outs) == lp.nout {
		for i := range outs {
			es.setProducedAt(outs[i], lp.seq, i)
		}
		es.lastUserPoly = nil
		return true
	}
	// `apply` over a closure this pass PRODUCED (an OpPushClosure out op —
	// `99 (kk 7) apply`, the twenty-eighth increment): the word hands the
	// value back and the check engine re-steps it over the values beneath,
	// exactly as the interpreter's applyHandler does, so the dispatch to
	// record is the RE-STEP's, not this one. Register the pending
	// application on the open unit — the program unit included — under the
	// value's own id: the re-step's anonymous dispatch records it as the
	// fn-value apply over the closure's producer operand (check's
	// recordPendingClosureApply → RecordDynApply, which consumes the entry),
	// and an entry no dispatch consumed refuses at the unit's finish or at
	// Finalize — never compiles the closure as unapplied data. Resolved
	// BEFORE the registered-output arm below: apply's identity result
	// carries the producer's id, which that arm would elide silently.
	if word == "apply" && len(args) == 1 && len(es.units) > 0 && es.producedFnValue(args[0].ID) {
		if _, isFn := args[0].Data.(core.FnDefInfo); isFn {
			u := es.units[len(es.units)-1]
			u.pendingApply = append(u.pendingApply, pendingApply{id: args[0].ID, pos: pos, fn: args[0]})
			return true
		}
	}
	// A dispatch whose output is already registered was recorded by a
	// structured hook (RecordBranch owns the `if` dispatch; a user-fn
	// ReturnsFn owns its RecordUserCall — including multi-return calls) —
	// the generic path must not double-record or refuse it. A structured
	// hook registers all results together, so checking the first suffices.
	// The producer must be a STRUCTURED hook, not a prior GENERIC native call:
	// a repeated identical computed call (`(context get 'n') add (context get
	// 'n')` — both gets return the same deterministic-ID result once their key
	// is concrete) collides with a prior generic event, and skipping the second
	// would orphan its receiver push. Those fall through to the carrier-identity
	// de-collision in RecordCall (which mints a fresh ID), so guard on !generic.
	if len(outs) > 0 {
		if pr, ok := es.producedBy[outs[0].ID]; ok && !es.eventInfo[pr.seq].generic {
			return true
		}
	}
	// A user fn returning ZERO values (`def v fn [[x:Integer] [] []]`): its
	// ReturnsFn recorded a 0-output CALL_USER and returned no carriers, so there
	// is no output ID to key the elision above on. The compiled 0-output call is
	// unambiguous — a user-fn dispatch whose body unit DECLINED to compile
	// returns a single Any approximation instead (len(outs)==1), so it falls
	// through to recordCallRefusal and the program falls back. Only the compiled
	// case reaches here with an empty residual.
	if sig != nil && sig.FnFrame() != nil && len(outs) == 0 {
		return true
	}
	// `apply` of a fn VALUE (`…args fn apply`): apply's ReturnsFn returns the
	// fn concrete, so the check engine RE-STEPS it — the fn then dispatches
	// against its preceding stack args and records as an ordinary CALL_USER.
	// Elide apply's own dispatch (it produces nothing the VM runs); without
	// this RecordCall would refuse it as "function value reaches apply". The
	// Reach-apply sig (a TReach operand, not an FnDef) is untouched.
	if word == "apply" && len(args) >= 1 {
		if _, ok := args[0].Data.(core.FnDefInfo); ok {
			return true
		}
		// `apply` of a Function-typed CARRIER (a `comp:Function` param or
		// captured comparator inside a fn body — `v comp/v apply`, Stage M2a):
		// the check engine cannot re-step a carrier, so the identity result
		// flows to the body residual as an fn value. Elide the dispatch and
		// register the PENDING apply on the enclosing unit; the unit's finish
		// either lowers it as the whole-residual OpCallDynTrailTop (the fn
		// applied to every value below it, exactly the interpreter's
		// applyHandler re-step over the preceding stack) or refuses — a pending
		// apply can never be silently dropped. Top-level applies (len(units)
		// == 1) keep today's refusal: the program residual has no equivalent
		// single-consumer window.
		if len(es.units) > 1 && sig != nil && sig.FnFrame() == nil && core.IsFnTypedCarrier(args[0]) {
			u := es.units[len(es.units)-1]
			u.pendingApply = append(u.pendingApply, pendingApply{id: args[0].ID, pos: pos})
			return true
		}
		// A GRADUAL lead (a Dynamic carrier — a `x:Any` param, a map get's
		// result, a fn-typed carrier's own apply result) under the [Reach
		// Any] overload — the one the check's gradual match takes whenever a
		// second value is beneath the lead (CompareSignatures tries the
		// longer overload first): the check consumed the lead and its
		// receiver and modelled ONE gradual result (the declared Any rides
		// gradual), so the dispatch records here as an apply EVENT over that
		// one arg — the receiver — lowered to OpCallDynApplyOne, which
		// commits exactly one result or defers (the twenty-seventh
		// increment). A gradual lead ALONE on the stack matches [Function]
		// instead and keeps the refusal below: nothing beneath it to apply
		// to, no single-consumer window at the unit's finish either.
		if es.recordGradualApplyEvent(sig, args, outs, pos) {
			return true
		}
		// A DYNAMIC lead that is neither a concrete value nor a fn-typed
		// carrier: the check's gradual match may have seated the
		// [Reach Any] overload where runtime dispatches [Function] —
		// reachable since the BROAD park (NUR073 clause 3) lets a fetched
		// fn arrive at `apply` as an untyped carrier — so a recorded poly
		// call carries the wrong arity and can only die as a VM-internal
		// no-match. No overload is provable at record time: refuse.
		if !core.IsConcrete(args[0]) && !core.IsFnTypedCarrier(args[0]) {
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("apply over a dynamic lead (overload unprovable)")
			return true
		}
	}
	// Compile-time NAME RESOLUTION: a get/getr whose result is a
	// statically-known callable or namespace (a raw fn export, a module
	// namespace) executed during the check pass —
	// `MathUtil.sqrt 16.0` is the tokens `MathUtil get sqrt 16.0`, and
	// the resolved export's own dispatch records the REAL call (the
	// inner native's sig through execMatch on this engine, so CALL_
	// NATIVE parity holds). Elide the resolution event; if the value
	// instead flows somewhere data-like, downstream provenance refuses
	// and the program falls back.
	if (core.IsGetWord(word) || core.IsGetrWord(word)) && len(outs) == 1 && core.IsConcrete(outs[0]) {
		switch outs[0].Data.(type) {
		case core.FnDefInfo:
			return true
		case core.ExtensionPayload:
			// A Module DESCRIPTOR (`M.$module`) is the one extension result
			// that flows as a VALUE — eq/deq identity (NUR031), a residual,
			// a def body — and the const gate refuses it on purpose, so an
			// elided read has no downstream home. Record the real dispatch
			// instead: the namespace operand reads live (dynScopeRescue's
			// module-family arm) and the runtime `dot` returns the shared
			// descriptor instance, so identity holds exactly as interpreted.
			// Any other extension result keeps the elision it had.
			return outs[0].Parent == nil || !outs[0].Parent.Equal(core.TModule)
		}
	}
	return false
}

// recordCallRefusal classifies a dispatch the recorder cannot lower as a generic
// CALL_NATIVE. It returns true (the dispatch is handled) after either marking the
// program uncompilable, or recording a flow-control terminator (break/continue —
// the enclosing loop's lowering turns these into jumps). It returns false for an
// ordinary lowerable call, which RecordCall then records.
func (es *EmitState) recordCallRefusal(word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos, forceDynOut, quoteInertOK bool) bool {
	shuffleOK := es.dynamicStackShuffleOK(word, sig)
	switch {
	case sig == nil:
		es.MarkUncompilable("dispatch without a signature at " + word)
	case word == "":
		// Anonymous / fn-value dispatch (usurp wrappers, F4 value
		// calls): the callee is a runtime value, Stage 3 territory.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("anonymous function dispatch (Stage 3)")
	case sig.RunInCheckMode():
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("compile-time word " + word)
	case sig.FnFrame() != nil:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("user fn call " + word + " (Stage 3)")
	case sig.FullStack():
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("full-stack word " + word)
	case isGetFamilyWord(word) && !es.shapedReadOut(outs) && (containerFnAutoDispatchRisk(args) || zeroArgFnOut(outs) || es.instanceFnFieldRisk(args)) && !es.zeroArgMemberFnLandingOut(outs):
		// A get/dot/getr/dotr read from a container HOLDING a function member
		// may surface that fn, and the interpreter AUTO-DISPATCHES a surfaced
		// fn value in every delivery context (probe-verified: `{f:make42/v}.f`
		// → 42; bare `{b:f/v} dot b` → 7; `… dot b add 1` → 8 — it even
		// collects forward args). The VM would push it as inert data — a
		// silent wrong value (miscompile mechanism E, the deferred-field
		// auto-invoke, design/MISCOMPILE-HUNT-FINDINGS.0.md). Refuse on the
		// RECEIVER signal: reads from fn-free containers are unaffected. An
		// ANNOTATED shaped-method read (shapedReadOut) is exempt: its landing
		// is modelled by tryShapedMethodDispatch, whose guard-owned decline
		// re-refuses, so the guard is re-homed rather than weakened. A
		// PINPOINTED genuine-0-arg member read (zeroArgMemberFnLandingOut)
		// is exempt the same way: tryMemberFnArrivalDispatch owns its
		// landing (the break-2 closure) and re-refuses what it cannot claim.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("fn value read from a container auto-dispatches (Stage 3)")
	case word == "args" || word == "__pa":
		// `args` reads the interpreter's per-call args stack, which the
		// VM's CALL_USER frame does not maintain (it binds params to
		// frame locals instead). A compiled fn body that reads `args`
		// would fail with "args: not inside a function" — refuse so the
		// program falls back to the interpreter.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("context-dependent word " + word)
	case len(sig.NoEvalArgs) > 0 && ((sig.Callable != nil && execBodyRefsNames(sig, args)) || (!sig.CompileEffect.Has(core.CompileRunsBodyIsolated) && !es.noEvalBodiesInertScoped(sig, args))):
		// A code-body word is refused when:
		//   - its body is not inert data (UNLESS the word runs the body in an
		//     ISOLATED CallBoru frame — CompileRunsBodyIsolated — where name
		//     resolution is identical under interpreter and VM: Test.check-prop's
		//     dynamic gen/property bodies bake as CALL_NATIVE operands and run
		//     through the same captured-parent handler in both modes); OR
		//   - it SPLICES the body onto the tape (a block-with-locals word like
		//     `var`): the handler returns tape-coupled tokens the VM cannot run, so
		//     baking a CALL_NATIVE (which an inert word-list body would otherwise
		//     permit) trips the VM's tape-coupled-result screen. This class had its
		//     own opt-out flag (CompileExecutesBody) until the inert-scope test
		//     below grew to cover it; the flag reached ZERO declaration sites and
		//     was retired (design/FULL-COMPILATION.0.md Stage 0), so the case now
		//     rests on the inert-scope disjunct plus the runtime screen that caught
		//     it originally; OR
		//   - it EXECUTES the body via InvokeBody (sig.Callable != nil — each / fold
		//     / scan / filter / do / case / …) AND the body references a NAME
		//     (execBodyRefsNames). Such a word is normally compiled by the closure
		//     path (PUSH_CLOSURE); if that path declined, const-baking the body and
		//     RE-RUNNING it in a sub-engine is unsound when the body names a binding,
		//     because the sub-engine resolves a name the COMPILED context holds as a
		//     VM frame local (a fn param/capture, a `for` iterator, a promoted
		//     value-`def`) against the registry instead — diverging (the property
		//     fuzzer's var-block bodies caught this across all three frame-local
		//     kinds). A body of pure inert DATA with no name references (`do [10 20
		//     30]`) re-runs identically, so it still bakes. (Data-reading code words
		//     like the query-DSL clauses declare NO CallableSpec, so their inert
		//     clause always bakes a plain CALL_NATIVE.)
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("code-body word " + word + " (Stage 2)")
	case hasUncoveredQuoteArg(sig) && !core.IsGetWord(word) && !core.IsGetrWord(word) && !setDelKernelSig(es.reg, word, sig) && !quoteInertOK:
		// Implicit-quote operands (usurp, force-arity, ref-family):
		// dispatch-manipulating meta words whose results the engine
		// re-steps. get/getr/set/del are exempt — plain accessors/mutators whose
		// quoted key is an inert Atom const (its fn-valued module-resolution
		// case is elided above; a dynamic or fn-valued result still refuses via
		// the later cases). For `set` the quoted key is the atom field name of
		// an object/class/store/flex field write (`p set x 7`); the receiver is
		// a non-const instance (mutation-safety holds — instance types are
		// absent from isInertConst, exactly as the integer-keyed array `set 1 v
		// a` already relies on), and the set/del exemption is keyed on BINDING
		// IDENTITY (setDelKernelSig, NUR057) — the matched sig must be the
		// kernel registration's own Locked signature, so an open-words
		// extension of set/del never rides an argument made for the mutator.
		// `del` is `set`'s inverse (atom-keyed map-entry removal, copy-return
		// on Map / in-place on FlexMap) and inherits the same argument verbatim.
		// quoteInertOK is the principled extension of that exemption to a MODULE
		// INNER NATIVE whose quoted operands are inert Atom consts — the query
		// DSL's table names (`Query.from people`, `Query.join visits`): the inner
		// native is reached via the wrapper's trivial delegation, so the
		// interpreter runs the SAME handler with the same baked atom.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("quoted-operand word " + word)
	case sig.CoreDefault && check.AnyNonConcreteOperand(args):
		// A CoreDefault overload (the within-type scalar/Micron arithmetic
		// defaults) is UNLOCKED: a runtime value whose tag is a strict
		// SUBTYPE of the static carrier's type (the refinement escape —
		// `def Flag (refine Boolean)` with a merged [Flag Flag] overload)
		// re-matches to the more-specific user overload that sorts before
		// it. The static CoreDefault match over a non-concrete operand is
		// therefore not a dispatch proof. The dispatch-outcome chain routes
		// this shape to OpCallNativePoly FIRST (tryRecordPoly's
		// coreDefaultCarrier arm — the VM re-matches over the live table,
		// exactly the interpreter's dispatch); this refusal is the safety
		// net for the shapes poly declines. Concrete operands bake fine:
		// their runtime tag IS the static tag. (Locked native sigs never
		// need any of this: locked-first means no user overload can
		// pre-empt them at runtime.)
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("core-default dispatch over a carrier operand at " + word)
	case check.AnyDynamicCarrier(args) && !shuffleOK && !es.DynInputsProven(sig, args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
	case check.AnyDynamicCarrier(outs) && !forceDynOut && !shuffleOK:
		// Dynamic outputs mean the checker could not type the word
		// (missing annotations, opaque wrappers like a def-bound
		// usurp value): the recorded signature is a best guess, not a
		// proof — don't bake it in. forceDynOut (dynOutNativeOK) is the
		// exception: a CONCRETE-args core builtin whose dynamic result is a
		// declared-Any return bakes faithfully and falls through here.
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("unannotated or opaque word " + word)
	case word == "break" && len(outs) == 0:
		// A flow-control terminator, not a call: the enclosing loop's
		// lowering turns it into a JMP to the loop end.
		es.appendEvent(EmitEvent{kind: evBreak, call: emitCall{word: word, pos: pos}})
	case word == "continue" && len(outs) == 0:
		es.appendEvent(EmitEvent{kind: evContinue, call: emitCall{word: word, pos: pos}})
	case word == "for" && makeListRange(es, args):
		// `for` over a COMPUTED range list (`for [1, (1 add 2)] [i]`) — the range
		// assembled via OpMakeList. A literal-const or local range lowers fine
		// (OpForSetup, or a CALL_NATIVE over the const list), but the CALL_NATIVE
		// for-handler over a RUNTIME-assembled range diverges, so keep it refused
		// (the interpreter handles the computed range).
		es.MarkUncompilable("for: computed range list (Stage 2 follow-on)")
	default:
		return false
	}
	return true
}

// recordShuffleElided reports whether a dispatch is a pure ID-PRESERVING stack
// shuffle (swap / rot / swap2 — a permutation, no duplication or drop) over
// gradual (dynamic) operands, in which case NOTHING is recorded: the outputs
// ARE the inputs by identity (ReturnsIdentity keeps each value's ID), so every
// downstream consumer resolves the same IDs to their ORIGINAL producers
// (params, captures, consts, prior events) and the lowerer re-derives any
// stack motion from dataflow (swapTop2 / spillSeat). Baking a CALL_NATIVE
// instead strands the pass-through copy of a frame-local operand on the
// simulated stack — the local re-pushes at its consumer, so the shuffle's
// output for it is never popped and the unit refuses "body leaves extra
// values" (the `input swap eval-pred` each-body shape). Gated to DYNAMIC
// operands — the case that refused outright before — so already-compiling
// concrete shuffles keep their existing CALL_NATIVE layout, byte-identical.
// A duplicating shuffle (dup/over/…) mints fresh output IDs (ReturnsIdentity),
// fails the multiset check, and falls through to the normal bake.
func (es *EmitState) recordShuffleElided(word string, sig *core.Signature, args, outs []core.Value) bool {
	if !check.AnyDynamicCarrier(args) || !es.dynamicStackShuffleOK(word, sig) {
		return false
	}
	if len(outs) != len(args) {
		return false
	}
	remaining := make(map[string]int, len(args))
	for _, a := range args {
		remaining[a.ID]++
	}
	for _, o := range outs {
		if remaining[o.ID] == 0 {
			return false
		}
		remaining[o.ID]--
	}
	return true
}

// dynamicStackShuffleOK reports whether a dispatch with dynamic (gradual-Any)
// operands is still sound to bake as a plain CALL_NATIVE: the word is one of
// the kernel's pure stack-shuffle primitives. Their ONE signature is all-Any,
// so every runtime value matches it — there is no sibling overload the checker
// could have mis-committed against (the hazard the anyDynamicCarrier refusal
// guards) — and the handler moves values without inspecting them
// (ReturnsIdentity / empty returns), so the outputs ARE the inputs and KEEP
// their dynamic modality: every downstream typed dispatch still sees Dynamic
// and gates itself (poly re-match, guarded CALL_USER, or refusal) exactly as
// before. This is the `input swap eval-pred` shape inside an each body over an
// untyped list. Guarded on pointer identity with the registry's own binding so
// a shadowed name (a user `def swap …`, whose sig has an fnFrame anyway) never
// rides the exemption. depth/pick/roll are full-stack words and refused earlier.

// setDelKernelSig is the binding-identity key that replaced the bare name
// test in the two set/del quote-arg exemptions (NUR057). Those exemptions
// were argued for the kernel mutator ("`set` cannot be shadowed (it is a
// builtin)"), but `set`/`del` are NOT in sealedWords — they are extendable —
// so the name alone could admit a shape the argument does not cover. The
// admitted set is exactly what the corpus differential proves sound:
//
//   - a LOCKED sig — a Go registration (the kernel mutator, or a module
//     inner native reached by delegation). Locked is stamped only by the
//     Go registration path, so it is a registration identity no runtime
//     construction can counterfeit; pointer identity into Lookup's table
//     was tried and is fragile (the aggregate rebuilds when an extension
//     entry lands, invalidating element addresses).
//   - a BORU-BODIED sig under the name — an open-words extension
//     (`def set fn [[k:Atom/q …] …]`): its /q param is an ordinary
//     forward-capture bound into a CALL_USER frame, nothing re-steps, and
//     the as.tsv/open-words.tsv extension rows compile with verified parity.
//
// What can no longer ride is a RUNTIME-MINTED handler sig under the name —
// the usurp-wrapper class the old comment feared (`def set (usurp …)`
// copies QuoteArgs onto a handler that RE-STEPS its result): never Locked,
// no boru body, and precisely the shape the quoted-operand refusal exists
// for.
func setDelKernelSig(_ *core.Registry, word string, sig *core.Signature) bool {
	if word != "set" && word != "del" {
		return false
	}
	if sig == nil {
		return false
	}
	return sig.Locked || len(sig.Body()) > 0
}

func (es *EmitState) dynamicStackShuffleOK(word string, sig *core.Signature) bool {
	if !core.DynStackShuffleWords[word] {
		return false
	}
	if es == nil || es.reg == nil || sig == nil {
		return false
	}
	fn := es.reg.Lookup(word)
	if fn == nil || len(fn.Signatures) != 1 || &fn.Signatures[0] != sig {
		return false
	}
	for _, t := range sig.ArgTypes() {
		if t == nil || !t.Equal(core.TAny) {
			return false
		}
	}
	return true
}

// RecordCallOperands resolves a lowerable native dispatch's operands. It refuses
// (marking the program uncompilable, returning ok=false) when a fn-valued operand
// would reach a fn-INVOKING word — that handler re-steps the fn on the tape,
// which the VM cannot honour (Stage 3). A word declaring CompileReadsFn
// (introspection) or CompileStoresFn (minilang/parselang register) treats its
// fn-valued argument as INERT data — read or stashed, never invoked on the VM
// tape — so the fn rides as a plain const operand; introspect (READS only) bakes
// even a CAPTURING fn, since only the immutable shape is read.
//
// MUTATION-SAFETY (load-bearing, see isInertConst): the only 0-result words are
// in-place mutators on Array/Object/Store/context receivers, and those instance
// types are NOT const-bakeable (absent from isInertConst's whitelist), so a
// pooled compound const can never reach one — the receiver is always a computed
// event or a frame local.
func (es *EmitState) RecordCallOperands(word string, sig *core.Signature, args []core.Value) ([]EmitOperand, bool) {
	es.storedBodyFnResidual = false
	introspect := sig.CompileEffect.Has(core.CompileReadsFn)
	inertFn := introspect || sig.CompileEffect.Has(core.CompileStoresFn)
	for i, t := range sig.ArgTypes() {
		if t != nil && t.ConformsTo(core.TFunction) {
			if inertFn || sig.FnInertArgs[i] {
				continue
			}
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
			return nil, false
		}
	}
	for i, a := range args {
		// A PREDICATE-TYPE NODE used to be refused here alongside a raw fn
		// value: the name evaluates to its minted node (the Stage 2 flip),
		// and `5 is Positive` runs the node's predicate body — which read
		// like the same re-step hazard.
		//
		// It never was one. A fn VALUE is refused because the handler puts
		// it back on the tape, and the VM has no tape. A predicate node is
		// a bare type literal: it rides as DATA, and the body runs through
		// the CALLBACK seam (RunPredicate -> InvokeCallbackFn), never the
		// tape. What actually blocked the graduation was that the seam's
		// nested arm declined every detached unit, so the body interpreted
		// and admitting these rows would have hidden an island inside a
		// handler rather than removed one. That decline is gone
		// (eng/go/vm_foreign_unit.go), so the node rides as an ordinary
		// const and its body runs on the VM. A declined stamp still falls
		// back to CallBoru — correct, slower, and visible to the
		// interp-entry census, which is the gate that keeps this honest.
		if _, isFnVal := a.Data.(core.FnDefInfo); !isFnVal {
			continue
		}
		if inertFn || sig.FnInertArgs[i] {
			continue
		}
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
		return nil, false
	}
	ops := make([]EmitOperand, len(args))
	for i, a := range args {
		// STORE-FN edge: a word that stashes a fn to invoke LATER (serve-raw)
		// gets its capture-free handler body compiled to its own unit, and a
		// durable CompiledFnRef stamped on the handler value's shared *BoruImpl —
		// so the interned const the word receives carries the VM edge alongside
		// its raw Body, and the word runs it on the VM (RunUnit) instead of the
		// interpreter (CallBoru). Done here, BEFORE resolveOperand's outcome is
		// consulted, because resolveOperand already interns a concrete fn value
		// as a const (ok=true) and would skip the inert-fn branch below; the
		// stamp mutates the pointer that interned copy shares, so it lands either
		// way. A body that refuses to compile leaves the const un-stamped and the
		// handler falls back to the interpreter, per-body and sound.
		if sig.CompileEffect.Has(core.CompileStoresFn) {
			// A CAPTURING handler at a STRICT store slot (service/add — the
			// native validates + dispatches the handler as an FnDefInfo)
			// cannot stamp and would fall through to a bare OpPushClosure
			// the native rejects: refuse so the interpreter owns it (the
			// factory-body service-handler miscompile §9.2e's paren-apply
			// unmasked). A non-strict store word (Patrun) invokes a stored
			// closure fine and is untouched.
			if sig.CompileEffect.Has(core.CompileFnHandlerStrict) {
				if fd, isFn := a.Data.(core.FnDefInfo); isFn && core.IsConcrete(a) && len(fd.Captured) > 0 {
					es.MarkUncompilable("capturing handler stored at " + word + " (validated as a function value)")
					return nil, false
				}
			}
			if fd, isFn := a.Data.(core.FnDefInfo); isFn && core.IsConcrete(a) && len(fd.Captured) == 0 {
				// EVERY stampable own sig gets its own unit + ref
				// (REFUSAL-CLOSURE §7b): the callback seam dispatches via
				// MatchFnSig, so the matched sig's Impl ref is the sig table.
				// A sig whose body refuses stays un-stamped and interprets —
				// per-sig and sound.
				for si := range fd.Signatures {
					if !storedSigEligible(&fd.Signatures[si]) {
						continue
					}
					aImpl := fd.Signatures[si].Impl.(*core.BoruImpl)
					if aImpl.Compiled != nil {
						continue // first stamp wins
					}
					if unit, cOK := es.compileStoredFnUnit(fd, si, a.Pos()); cOK {
						ref := &CompiledFnRef{Unit: unit, depNames: es.storedHandlerDeps(fd.Signatures[si].Body())}
						aImpl.Compiled = ref
						es.storedFnRefs = append(es.storedFnRefs, ref)
					}
				}
			}
		}
		// STORE-BODY edge: a word that stashes a NoEvalArgs code-body to run later
		// on its own registry (spawn) gets that body compiled to a 0-param unit,
		// and receives a synthetic fn-value carrier (raw tokens + CompiledFnRef) in
		// place of the raw list — so it runs the unit via RunUnit on its fork. A
		// body that refuses to compile falls through to the plain list const and
		// the word runs it on the interpreter, unchanged.
		if sig.CompileEffect.Has(core.CompileStoresBody) && sig.NoEvalArgs != nil && sig.NoEvalArgs[i] {
			if carrier, cok := es.compileStoredBody(a); cok {
				ops[i] = ConstOperand(es.intern(carrier))
				continue
			}
		}
		// STORE-BODY-LIST edge: a word that stores a LIST of code bodies to run
		// later on per-branch forks (await's parallels). Each element compiles
		// to its own 0-param unit and rides as a carrier in a rebuilt list;
		// an element that refuses keeps its raw list and that branch runs on
		// the interpreter, per-element and unchanged.
		if sig.CompileEffect.Has(core.CompileStoresBodyList) && sig.NoEvalArgs != nil && sig.NoEvalArgs[i] {
			if lst, lerr := core.AsList(a); lerr == nil && !lst.IsNil() {
				elems := lst.Slice()
				rebuilt := make([]core.Value, len(elems))
				any := false
				for j := range elems {
					if carrier, cok := es.compileStoredBody(elems[j]); cok {
						rebuilt[j], any = carrier, true
						continue
					}
					rebuilt[j] = elems[j]
					// The element keeps its raw list and that branch runs on
					// the interpreter, so its residual is UNKNOWN here — and a
					// residual the recorder cannot see may hold a callable.
					// An EMPTY body is the exception, and the common one:
					// compileStoredBody's first guard declines a zero-token
					// list, and a body that runs nothing leaves nothing.
					if !storedBodyIsEmpty(elems[j]) {
						es.storedBodyFnResidual = true
					}
				}
				if any {
					ops[i] = ConstOperand(es.intern(core.WithPos(core.NewList(rebuilt), a)))
					continue
				}
			}
		}
		// STORED-PARAM-BODY edge: a declared param-carrying stored body
		// (Signature.StoredBodies — Test.check-prop's gen/property) compiles
		// to a closure unit with the declared params bound and rides as a
		// carrier mirroring the handler's own CallBoru sig; a body that
		// refuses keeps its raw list and the handler interprets, unchanged.
		// MODULE SCOPE ONLY (the noEvalBodiesInertScoped discipline): inside
		// a compiled fn frame the body's ${interp} / bare reads of frame
		// locals resolve against the REGISTRY in the handler's isolated
		// frame, which the VM's locals never reach — the fn-scope check-prop
		// guard (TestCheckPropInterpStringFnScopeRefuses) must keep refusing.
		if len(es.units) == 1 && len(sig.StoredBodies) > 0 && sig.NoEvalArgs != nil && sig.NoEvalArgs[i] {
			if spec := storedBodySpecFor(sig, i); spec != nil {
				if carrier, cok := es.compileStoredParamBody(a, spec.Params); cok {
					ops[i] = ConstOperand(es.intern(carrier))
					continue
				}
			}
		}
		op, ok := es.resolveOperand(a)
		if !ok && (introspect || inertFn || sig.FnInertArgs[i]) {
			if fd, isFn := a.Data.(core.FnDefInfo); isFn {
				switch {
				case core.IsConcrete(a) && len(fd.Captured) == 0:
					// A CAPTURE-FREE concrete (immutable) fn value bakes as a const
					// the introspection / fn-inert-slot / store-fn handler reads at
					// run time. A CAPTURING fn must NOT bake as a const: its baked
					// body leaves the captured names as bare Words with no binding
					// (`word(bse)`), so a later invocation reads them unbound — the
					// capturing-sink miscompile. It falls to the closure path below.
					op, ok = ConstOperand(es.intern(a)), true
				default:
					// A CAPTURING anonymous lambda in a store-fn / fn-inert slot
					// (`Net.serve-raw {…} ([conn] => [ handle conn store ])`,
					// mini-S3's per-connection actor closing over the shared store):
					// resolveOperand cannot materialise it as a const, but it lowers
					// to an OpPushClosure — the captures resolved in THIS frame and
					// packed into the closure value the storing native invokes per
					// call, exactly as the interpreter dispatches the boru handler
					// with its captures. tryReturnedClosure declines (leaving es
					// untouched) when a capture is unreachable or the body refuses,
					// so an uncompilable handler still falls back faithfully.
					if cop, cok := es.tryReturnedClosure(a, a.Pos()); cok {
						op, ok = cop, true
					}
				}
			}
		}
		if !ok {
			es.MarkUncompilable("operand of unknown provenance or not statically materialisable at " + word)
			return nil, false
		}
		// A FnDataArgs slot (parselang-fn-dispatch arg0, the computed parser fn)
		// resolved to a dyn-scope read: re-key it to the READ-AS-DATA lookup so
		// the VM pushes the FnDefInfo parser binding instead of deferring on it.
		// Only a dyn-scope resolution is rewritten — a concrete parser (an event
		// / const operand) is delivered normally.
		if sig.FnDataArgs[i] && op.kind == opDynScope {
			op = dataScopeOperand(op.idx)
		}
		ops[i] = op
	}
	return ops, true
}

// RecordPolyCall records a native dispatch the checker could not commit to
// one overload for (a dynamic operand widened to Any): the call lowers to
// OpCallNativePoly, which re-matches the word's signatures at run time (plan
// P3). Operands resolve normally (the dynamic one is a prior event's result);
// returns false, leaving es untouched, when one is of unknown provenance.
// noMatch, when non-nil, rides onto the PolyRef as the faithful-raise plan
// for the runtime no-match arm (plan 3c) — the caller derived and gated it at
// the failed-dispatch tape state; nil keeps the sound defer.
func (es *EmitState) RecordPolyCall(word string, args, outs []core.Value, pos core.SrcPos, ownerReg *core.Registry, noMatch *core.PolyNoMatchSpec) bool {
	if !es.Active() {
		return false
	}
	if isGetFamilyWord(word) && !es.shapedReadOut(outs) && (containerFnAutoDispatchRisk(args) || zeroArgFnOut(outs) || es.instanceFnFieldRisk(args)) && !es.zeroArgMemberFnLandingOut(outs) {
		// Same auto-dispatch divergence as the mono path (recordCallRefusal):
		// the interpreter invokes a container-read fn value as it lands; the
		// VM would push it as data. Refuse the program (sound fallback).
		// Annotated shaped-method reads and pinpointed genuine-0-arg member
		// reads are exempt (see recordCallRefusal).
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("fn value read from a container auto-dispatches (Stage 3)")
		return true
	}
	ops := make([]EmitOperand, len(args))
	for i := range args {
		a := args[i]
		// A TYPE-NAME word in the claimed window — the un-stepped `Bytes` in
		// `convert Bytes <dynamic>`: the dispatch could not commit statically
		// (the data operand's type is unknown), so the plan's claimed args
		// still hold the RAW name token. At runtime the interpreter steps the
		// name to its canonical type literal before the word dispatches; give
		// the poly window the same value — the canonical literal lowers via
		// resolveOperand's type-operand path (OpPushType by ID), so callPoly's
		// re-match sees exactly the operand the interpreter's dispatch does.
		// Only a PLAIN word naming a live type qualifies; anything else keeps
		// the unresolvable-operand decline below.
		if core.IsWord(a) && es.reg != nil {
			if w, wErr := core.AsWord(a); wErr == nil &&
				w.ArgCount == -1 && !w.ForceStack && !w.ForceForward && !w.ForceVal && !w.ForceUsurp {
				// Mirror stepWord's type-name cascade exactly — the
				// builtin arm of the canonical resolver (resolve.go).
				if t, tOK := core.ResolveBuiltinTypeName(w.Name); tOK {
					a = core.NewTypeLiteral(t)
				}
			}
		}
		op, ok := es.resolveOperand(a)
		if !ok {
			return false
		}
		ops[i] = op
	}
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: word, ops: ops, nout: len(outs), pos: pos, poly: true, polyReg: ownerReg, polyNoMatch: noMatch}})
	switch len(outs) {
	case 0:
		// A 0-output poly (a side-effect word like the test framework's
		// `test-record`) produces no stack value to register.
	case 1:
		es.setProduced(outs[0], seq)
	default:
		// A multi-result poly (flex `pop` → [remaining, popped]) seats each
		// result under its own index — the same per-index registration the
		// generic RecordCall path uses, which the VM's PolyRef.NOut claim
		// enforces. Mark the event generic (no structured hook ran) and
		// de-collide result IDs: two results can share an ID (a value the
		// handler hands back in more than one slot, or a stale ID colliding
		// with an earlier event's output), which would make provenance lookups
		// alias one producer. Mint a fresh ID for a colliding result UNLESS it
		// is an input passthrough — a receiver flowing straight through keeps
		// its identity so downstream reads still resolve to it.
		gf := es.eventInfo[seq]
		gf.generic = true
		es.eventInfo[seq] = gf
		argIDs := make(map[string]bool, len(args))
		for i := range args {
			if args[i].ID != "" {
				argIDs[args[i].ID] = true
			}
		}
		for i := range outs {
			if pr, ok := es.producedBy[outs[i].ID]; ok && pr.seq != seq && !argIDs[outs[i].ID] {
				outs[i].ID = core.GenerateID(core.IDPrefixForType(outs[i].Parent))
			}
			es.setProducedAt(outs[i], seq, i)
		}
	}
	return true
}

// RecordDynMethod records a GUARDED shaped-instance-method apply (Stage M2c):
// fn is the dynamic method-read carrier (its producing event supplies the
// RUNTIME value — nothing of the check-mode shape instance is baked, the
// freeze-gate), args the inert statement-window operands the check-mode match
// consumed, outs the matched member signature's declared results. Lowers to
// OpCallDynMethod with the arity + result-count claim the VM enforces (claim
// failure → internal_error → interpreter re-run). Returns false, leaving es
// untouched, when an operand has no compiled home — the caller then refuses.
func (es *EmitState) RecordDynMethod(fn core.Value, args, outs []core.Value, word string, pos core.SrcPos) bool {
	if !es.Active() {
		return false
	}
	fnOp, ok := es.resolveOperand(fn)
	if !ok {
		return false
	}
	ops := make([]EmitOperand, 0, len(args)+1)
	ops = append(ops, fnOp)
	for i := range args {
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops = append(ops, op)
	}
	// A def-read fn carrier the read model dispatches here (a bare word the
	// interpreter dispatches — `def r (mk)  r`) is consumed by an accepted
	// lowering: credit the read so the NUR123 accounting does not count it
	// as a read lost where the interpreter dispatches it.
	es.creditWordRead(fn.ID)
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: word, ops: ops, nout: len(outs), pos: pos,
		dynMethod: &DynMethodSpec{Word: word, NArgs: len(args), NOut: len(outs)},
	}})
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
	return true
}

// containerFnAutoDispatchRisk reports whether a get-family dispatch may
// surface a container member the interpreter could AUTO-DISPATCH after it
// lands: a function value carrying a 0-arg-satisfiable signature. Probe-
// verified divergences: `{f:make42/v}.f` → 42, bare `{b:f/v} dot b` → 7,
// and `{f:add1/v}.f 5` → 6 (a landed fn even collects forward args), while
// the VM would push the fn as inert data (miscompile mechanism E). NOTE:
// parked user-fn values structurally carry a 0-arg Signature alongside
// their declared overloads, so in practice every user-fn member READ
// refuses — the safe side, since landing-then-forward-collecting makes even
// multi-param members invocable. The precision that pays is the KEY
// resolution: when another concrete arg resolves as the read key, only that
// member is inspected, so reads of NON-fn keys from fn-carrying containers
// keep compiling; an unresolvable (computed) key inspects every member
// conservatively. The receiver signal behind the get-family auto-dispatch
// refusals (recordCallRefusal, RecordPolyCall). Census cost at landing:
// 2 rows (3,875-row corpus), both previously divergence-exposed.
// noteFnRiskFields records, at a construction dispatch (make), which field
// keys of the produced INSTANCE hold genuinely-0-param fn values — the
// auto-dispatch-on-read hazard containerFnAutoDispatchRisk cannot see when
// the later get-family receiver is the instance CARRIER (schema only, no
// field values; probe: `def C class {f:Function} def o (make C
// {f:make42/v}) o.f` → 42 interpreted vs the fn value compiled — PR #225
// P1). The construction MAP is concrete at record time, so the hazard is
// decidable here and consulted by ID at the get-family guard sites. A field
// later overwritten with a non-fn value over-refuses (sound, rare —
// documented trade).
func (es *EmitState) noteFnRiskFields(word string, args, outs []core.Value) {
	if word != "make" || len(outs) == 0 {
		return
	}
	var risky map[string]bool
	var members map[string]core.Value
	for _, a := range args {
		mp, ok := a.Data.(core.MapPayload)
		if !ok || a.Carrier || mp.M == nil {
			continue
		}
		for _, k := range mp.M.Keys() {
			mv, _ := mp.M.Get(k)
			if core.FnValueZeroArg(mv) {
				if risky == nil {
					risky = map[string]bool{}
				}
				risky[k] = true
			}
			// The any-arity sibling: EVERY fn-valued field is remembered
			// with its VALUE, so a later read through the instance carrier
			// can tag the member-fn landing (see fnMemberFields).
			if _, isFn := mv.Data.(core.FnDefInfo); isFn {
				if members == nil {
					members = map[string]core.Value{}
				}
				members[k] = mv
			}
		}
	}
	if members != nil {
		if es.fnMemberFields == nil {
			es.fnMemberFields = map[string]map[string]core.Value{}
		}
		for _, o := range outs {
			if o.ID != "" {
				es.fnMemberFields[o.ID] = members
			}
		}
	}
	if risky == nil {
		return
	}
	if es.fnRiskFields == nil {
		es.fnRiskFields = map[string]map[string]bool{}
	}
	for _, o := range outs {
		if o.ID != "" {
			es.fnRiskFields[o.ID] = risky
		}
	}
}

// instanceFnFieldRisk consults noteFnRiskFields' record for a get-family
// dispatch whose receiver is (a carrier of) a tracked instance: a concrete
// key inspects only that field, an unresolvable key any tracked field.
func (es *EmitState) instanceFnFieldRisk(args []core.Value) bool {
	if len(es.fnRiskFields) == 0 {
		return false
	}
	for _, a := range args {
		risky := es.fnRiskFields[a.ID]
		if risky == nil {
			continue
		}
		if key, ok := concreteMapKey(args); ok {
			if risky[key] {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// instanceFnMember consults fnMemberFields for a get-family dispatch whose
// receiver is (a carrier of) a tracked constructed instance with a CONCRETE
// key: it returns that field's fn member value so the read result can be
// tagged (noteMemberFnRead) exactly like a concrete-container member read.
// An unresolvable key reports nothing — the fnRiskFields guard already owns
// the conservative refusal for those.
func (es *EmitState) instanceFnMember(args []core.Value) (core.Value, bool) {
	if len(es.fnMemberFields) == 0 {
		return core.Value{}, false
	}
	for _, a := range args {
		members := es.fnMemberFields[a.ID]
		if members == nil {
			continue
		}
		if key, ok := concreteMapKey(args); ok {
			if mv, hit := members[key]; hit {
				return mv, true
			}
		}
	}
	return core.Value{}, false
}

// zeroArgFnOut is the OUT-side backstop to the receiver heuristic: when a
// get-family dispatch's check-mode result IS a concrete fn value with a
// genuine 0-param overload, the read demonstrably surfaced an
// auto-dispatchable member regardless of how the receiver was represented
// (a class-instance CARRIER receiver dodges the payload inspection —
// probe: `def C class {f:Function} def o (make C {f:make42/v}) o.f` → 42
// interpreted vs the fn value compiled; PR #225 P1). Zero FP risk: the out
// is the very value whose landing diverges.
func zeroArgFnOut(outs []core.Value) bool {
	for _, o := range outs {
		if !o.Carrier && core.FnValueZeroArg(o) {
			return true
		}
	}
	return false
}

// NoteShapedRead mirrors a CheckState.NoteMethodShape annotation into the
// recorder: the get-family read that produced `id` resolved a shaped-
// instance member whose LANDING the shaped-method model owns, so the
// read-guard auto-dispatch refusals skip it (shapedReadOut).
// NoteDefRead records that value ID was produced by reading binding `name`
// (stepWord's simple-value substitution). Consulted by resolveOperand's
// dynamic-scope rescue when the value has no compiled home.
// NoteFrozenRead records a module-scope binding read that happened INSIDE an
// open fn/closure unit analysis, and WHAT the unit froze about it: a value
// baked as a const, a type baked as an identity, or a call target baked as a
// unit index. All three are frozen across calls where the interpreter
// re-resolves the name per call, so a later module-scope rebind must refuse
// the program (NotifyNameRebound). No-op at top level, where analysis order
// equals program order and the bake is the read the interpreter makes.
//
// An unclassified note (FrozenBakeNone) is DROPPED rather than recorded as
// some default: the refusal it would produce names a bake, and naming the
// wrong one is worse than the caller's bug going unrecorded here.
func (es *EmitState) NoteDefRead(id, name string) {
	if !es.Active() || id == "" || name == "" {
		return
	}
	es.noteFragRead(name)
	if es.armBoundNames[name] {
		// The name's runtime binding exists only through arm-resident
		// installs — count, values, and definedness are body-run-dependent
		// (`[] each [def x 5] x`: the interpreter raises undefined_word
		// where the check model still holds x) — so a read here would bake
		// the model's generalized value. Poison the placement gate: the
		// regime's Finalize seam refuses, and the interpreter owns the
		// shape (the parity oracle pins it). After a terminal top-level
		// trap the read is unreachable — the compiled program truncates at
		// the trap — so it poisons nothing, MarkUncompilable's own trap
		// discipline.
		if es.trapAt == 0 && es.armReadRefusal == "" {
			es.armReadRefusal = "twin regime: read of `" + name + "` after a multi-run body binds it: " +
				"the runtime binding is per-element (or absent at zero iterations), so the check " +
				"model's value cannot stand in (arm-resident twins, §6.5)"
		}
		return
	}
	if es.defReads == nil {
		es.defReads = map[string]string{}
	}
	es.defReads[id] = name
	// Snapshot the binding's generation at the read: a RESIDUAL re-push of
	// this read (resolveResidualOperands) executes at PROGRAM END, so it is
	// sound only while no later rebind moved the binding (residualReadStable).
	if es.reg != nil {
		if es.defReadGens == nil {
			es.defReadGens = map[string]int64{}
		}
		es.defReadGens[id] = es.reg.Defs.Gen(name)
	}
}

// DefReadName answers NoteDefRead for the read model (core.EmitRecorder).
func (es *EmitState) DefReadName(id string) (string, bool) {
	name, ok := es.defReads[id]
	return name, ok
}

// residualReadStable reports whether a def-read value's binding is UNCHANGED
// since the read (same DefTable generation): the end-of-program
// OpLookupDynScope re-push then resolves the same value the read saw. A
// consumer-position rescue needs no such gate (its op executes at the read's
// own position); only the residual disposition calls this.
func (es *EmitState) residualReadStable(v core.Value) bool {
	name := es.defReads[v.ID]
	if name == "" || es.reg == nil {
		return false
	}
	gen, ok := es.defReadGens[v.ID]
	return ok && es.reg.Defs.Gen(name) == gen
}

// RecordDynBind records a value-def site (name + the bound value's operand)
// as an evDynBind event. Every def records one cheaply; the lowering emits a
// registry-visible OpBindDynScope ONLY for names in dynScopeNames (some
// OpLookupDynScope reads them) and skips the event otherwise. Engine-internal
// and capitalised (type) names never bind dynamically.
// recordCodeBodyClosureRead refuses when a token BODY argument reads a name
// dyn-bound to a compiled closure. Such a body is re-run by its native
// through the INTERPRETER (`do [(h 1)]`, and every other NoEvalArgs code
// slot), and a ClosurePayload is invokable only through the VM's re-entrant
// runner — the re-run reaches the closure's TOKEN body with no captures
// installed and raises (`undefined word: g` where the interpreter answers
// 8). Scanning every concrete LIST argument is deliberately conservative:
// a DATA list mentioning the name carries it as an Atom, not a Word, so it
// does not match, and a body that never reads such a name is untouched —
// which is what keeps §9b's family compiling.
// argIsProducedClosure refuses a dispatch whose ARGUMENT is a compiled
// closure this pass produced (a call result the recorder knows returned an
// OpPushClosure — producerReturnedClosure). Such a value reaching a
// word's slot means the paren that should have APPLIED it did not collapse
// into an apply, so the compiled program hands the word the FUNCTION where
// the interpreter hands it the applied result.
//
// The `apply` word's one-arg overload over a CONCRETE closure value is the
// exception (the twenty-eighth increment): there the closure at the slot
// is exactly what the word applies — applyHandler hands it back and the
// check engine re-steps it over the values beneath, as the interpreter
// does — so the dispatch is not a stranded apply but the apply itself.
// recordCallElided registers it as a PENDING application the re-step's
// dispatch records (check's recordPendingClosureApply) or the program's
// finish refuses. A fn-typed CARRIER (a declared `[Function]` return) keeps
// the refusal: the check engine cannot re-step a carrier, so at the program
// level nothing models the apply — the residual has no single-consumer
// window (`1 99 (mk 7) apply` seated all three as data when this was
// lifted for carriers too). The two-arg Reach overload keeps it as well: a
// closure at its receiver slot is data the paren left unapplied.
//
// A slot DECLARED `Function` is the other exception (the thirtieth
// increment): the word asked for a fn value there — a user fn's
// `p:Function` param, which the interpreter's frame binding installs as
// data (`cif (cnot ctrue/v)`) — so a produced closure at that slot is the
// argument, not a stranded apply, and the call's operand carries the
// payload. An `Any` slot keeps the refusal: `typeof (h 5)` is the paren
// that failed to collapse, and its slot would swallow the FUNCTION. The
// apply word's own Function slot is not this exception: apply APPLIES its
// value, and the rules above are its whole account.
func (es *EmitState) argIsProducedClosure(word string, sig *core.Signature, args []core.Value) bool {
	if !es.Active() {
		return false
	}
	if word == "apply" && len(args) == 1 {
		if _, isFn := args[0].Data.(core.FnDefInfo); isFn {
			return false
		}
	}
	for i := range args {
		if !core.IsAppliableFn(args[i]) || (word != "apply" && slotDeclaresFunction(sig, i)) {
			continue
		}
		// A `/v` read of a def-bound closure (valReadIDs) is DATA on both
		// engines — the interpreter never dispatches the value spelling —
		// so no paren failed to collapse: the slot takes the value.
		if es.valReadIDs[args[i].ID] {
			continue
		}
		if es.producerReturnedClosure(args[i].ID) {
			es.MarkUncompilable(
				"computed closure at a word's argument slot (its apply did not collapse — Stage 2)")
			return true
		}
	}
	return false
}

// slotDeclaresFunction reports whether a signature's slot i is declared
// exactly `Function` — a named param's type (Params) or a positional one
// (Args); a nil signature or an untyped slot declares nothing.
func slotDeclaresFunction(sig *core.Signature, i int) bool {
	if sig == nil {
		return false
	}
	var t *core.Type
	if i < len(sig.Params) {
		t = sig.Params[i].Type
	} else if i < len(sig.Args) {
		t = sig.Args[i]
	}
	return t != nil && t.Equal(core.TFunction)
}

func (es *EmitState) recordCodeBodyClosureRead(args []core.Value) bool {
	if len(es.dynBoundClosures) == 0 {
		return false
	}
	for i := range args {
		// AsList is the single screen: it declines every non-list value
		// AND every List-parented one with no readable payload (a carrier,
		// a typed-list shape), which is exactly "no tokens to read here".
		lst, aerr := core.AsList(args[i])
		if aerr != nil {
			continue
		}
		body := make([]core.Value, 0, lst.Len())
		for j := 0; j < lst.Len(); j++ {
			body = append(body, lst.Get(j))
		}
		found := false
		core.WalkBodyWords(body, func(w core.WordInfo, _ core.Value) {
			if es.dynBoundClosures[w.Name] {
				found = true
			}
		})
		if found {
			es.MarkUncompilable(
				"code body reads a def-bound compiled closure (an interpreter re-run cannot apply it — Stage 2)")
			return true
		}
	}
	return false
}

// recordsFilteredDynBind reports whether RecordDynBind will actually record
// an event for a `_`/`$`-prefixed name in the CURRENT recorder state — the
// root twin-regime seat, or an arm-resident body compile.
//
// It exists because TWO gates need the same answer and drifted apart when
// only one of them was updated: RecordDynBind opened for these names under
// the regime, while SplitLoopRegionBind kept declining them on the premise
// that they "record no dyn-bind event" — which by then was false. The split
// then never happened, the def bound the whole region instead of its first
// value, and `def _ (for [1 4] [i]) _` compiled to `1 2 3` where the
// interpreter answers `2 3 1` (NUR116, silent, on the DEFAULT lane too).
// Asking one predicate is what keeps the answer single-sourced; never
// re-spell this condition at a call site.
func (es *EmitState) recordsFilteredDynBind() bool {
	if es == nil {
		return false
	}
	root := len(es.units) == 1 && es.reg != nil && es.reg.Check.FnBodyDepth == 0
	return root || es.armResidentDepth > 0
}

func (es *EmitState) RecordDynBind(name string, v core.Value, pos core.SrcPos) {
	if !es.Active() || name == "" || core.IsCapitalisedName(name) {
		return
	}
	if es.valBindEpoch == nil {
		es.valBindEpoch = map[string]int{}
	}
	es.valBindEpoch[name]++
	es.noteClosureShapeBind(v)
	if name[0] == '_' || name[0] == '$' {
		// The historical skip for these names is a keep-installs-era economy:
		// under the default regime the check pass's install IS the kept
		// binding, so a name nobody dyn-reads needs no op. Under the TWIN
		// REGIME it is a correctness hole at the root: the rollback removes
		// the install, a computed def's placed twin captures a CARRIER that
		// ApplyBindTwin's carrier-class skip consumes, and with no Push-mode
		// OpBindGlobal partner the binding is silently LOST cross-request —
		// measured before the fix as `def _ ([1 2] each [1]) 9` then `_` →
		// undefined_word against the interpreter's [[1 1]] (the parity
		// oracle's founding row, bind_multirun_parity_test.go). So a ROOT
		// def of such a name records under the regime, and so does a def
		// inside an ARM-RESIDENT body compile (armResidentDepth — the
		// bridge needs `def _2 …`'s event seat, since the ledger notes the
		// name); everywhere else — the default regime, fn bodies, nested
		// units — the historical skip stands and default bytecode stays
		// byte-identical.
		if !es.recordsFilteredDynBind() {
			return
		}
	}
	// NOTE a name bound to a COMPILED CLOSURE. The value is invokable only
	// through the VM's re-entrant runner, never the interpreter
	// (payload.go's contract, plan P2) — so it is fine to apply from
	// compiled code (that is what §9b's factory family does) and NOT fine
	// for an interpreter RE-RUN to reach. recordCodeBodyClosureRead below
	// catches the latter at the word that re-runs tokens.
	// Only a CONCRETE closure value is noted (the thirty-eighth increment):
	// a name bound to a Function CARRIER — a typed factory's declared
	// return, `def f (mk 1)` — is read through the fn-carrier side table
	// inside a code body too, and the read models as the dispatch it is
	// (`do [(f 2)]` compiles and agrees), where a concrete
	// closure's read inside the body's unit resolves to the FnDefInfo
	// itself, whose home is an event outside the unit: without the gate
	// `do [(h 1)]` over a lambda factory's closure compiled and raised
	// `undefined word: g` (the captured param) for the interpreter's 8,
	// and `[1 2] each [p/v apply]` islanded to `[[1 2]]` for `[[8 9]]`.
	if es.producerReturnedClosure(v.ID) && core.IsConcrete(v) {
		if es.dynBoundClosures == nil {
			es.dynBoundClosures = map[string]bool{}
		}
		es.dynBoundClosures[name] = true
	}
	// NOTE the def NAME of a produced fn value by its producing slot, so the
	// lowering seats it on the value's promoted store (CompiledFn.StoreNames)
	// and the VM renames the stored closure as installDef renames a fn value
	// bound by `def` (the twenty-ninth increment).
	if pr, ok := es.producedBy[v.ID]; ok && es.producedFnValue(v.ID) {
		if es.defNameAt == nil {
			es.defNameAt = map[seqIdx]string{}
		}
		es.defNameAt[seqIdx(pr)] = name
	}
	src, srcSeq := EmitOperand{}, -1
	cur := es.units[len(es.units)-1]
	es.noteValBind(cur, name, v)
	if slot, ok := cur.localByID[v.ID]; ok && cur.capID[v.ID] {
		// A CAPTURE of the CURRENT unit overrides events-first, mirroring
		// resolveOperand's capID override (emit.go ~1241): the captured value
		// may still carry a producedBy entry from the ENCLOSING unit (a
		// persistent module-scope instance built by a parent `def`, or a
		// computed def snapshotted into the closure) whose event lives in the
		// parent frame and is unreachable here — so binding from that foreign
		// seq refuses "dynamic-scope def of unpromoted computed value" (the
		// promotion has no local event to promote). Re-push this unit's own
		// capture SLOT instead. Gating on capID (never a join result) is
		// load-bearing: a plain localByID-first reorder would misbind the
		// JoinCarriers ID-reuse case (a branch result reusing a live local's
		// ID) to the local instead of the branch event.
		src = localOperand(slot)
	} else if pr, ok := es.producedBy[v.ID]; ok && pr.idx == 0 {
		srcSeq = pr.seq
	} else if slot, ok := cur.localByID[v.ID]; ok {
		src = localOperand(slot)
	}
	// A def recorded in the ROOT unit outside any fn body persists past the
	// run (keep-on-compile): stamp the slot its install just created so the
	// lowering can emit the OpBindGlobal write-back. Fn-body and nested-unit
	// defs are frame-scoped (DefCleanup tears them down) — never stamped.
	root, depth := false, 0
	if len(es.units) == 1 && es.reg != nil && es.reg.Check.FnBodyDepth == 0 {
		root, depth = true, es.reg.Defs.Depth(name)
	}
	if root && !core.IsConcrete(v) && v.Parent.ConformsTo(core.TFunction) {
		// A top-level computed fn value-def (`def op (Parse.parser g)`) that
		// installDef declined to install in Defs (the compiled-closure machinery
		// owns the name): record its ID + name so a deeper fn body reading the
		// name (`parse op src`) rescues to a runtime dyn-scope read. The
		// OpBindDynScope twin this same def records makes the runtime lookup
		// resolve, byte-identical to the interpreter reading the name against the
		// live def stack. (ConformsTo is receiver-nil-safe; IsConcrete already
		// excludes concrete values.)
		if es.rootComputedBindIDs == nil {
			es.rootComputedBindIDs = map[string]bool{}
		}
		es.rootComputedBindIDs[v.ID] = true
		es.NoteDefRead(v.ID, name)
	}
	spliceDepth := -1
	if plb := es.pendingLoopBind; plb != nil {
		// The S5 first-value bind (SplitLoopRegionBind latched it in the same
		// installAndRecordDef call): the bound value is the loop region's
		// first value, live at a static depth inside the region — no operand
		// home, the splice-at-depth lowering owns it. Track the name so
		// dynScopeRescue admits its TOP-LEVEL reads (the runtime binding is
		// installed by the splice bind before any read executes).
		srcSeq, spliceDepth = plb.seq, plb.depth
		es.pendingLoopBind = nil
		if es.loopSplitBinds == nil {
			es.loopSplitBinds = map[string]bool{}
		}
		es.loopSplitBinds[name] = true
		f := es.eventInfo[plb.seq]
		f.splitBound = true
		es.eventInfo[plb.seq] = f
		if es.reg.Check.LoopBodyDepth > 0 && depth > 1 {
			// Inside a LOOP BODY (S9.2a) the captured (final) analysis round
			// runs with the PREVIOUS round's joined install still live, so
			// Defs.Depth counts one analysis-only shadow the runtime name
			// stack never has — the kept binding the splice must SetAt sits
			// one below (stamping the inflated depth made the write-back a
			// silent no-op and the in-loop OpLookupDynScope read the kept
			// check-time carrier: the [5 0 5 0] miscompile).
			depth--
		}
	}
	es.appendEvent(EmitEvent{kind: evDynBind, dyn: &emitDynBind{
		name: name, src: src, srcSeq: srcSeq, val: v, pos: pos,
		root: root, depth: depth, spliceDepth: spliceDepth,
		residentTwin: -1,
	}})
	es.noteBindHazard(name)
}

// dynScopeRescue is resolveOperand's last resort inside a fn unit: a value
// that was READ from a def binding (NoteDefRead) but has no compiled home is
// a DYNAMIC-SCOPE reference — the interpreter resolves the name against the
// live def stack at run time (a callee reading the caller's param, a
// recursive base case reading the previous frame's body-local). Lower it to
// a runtime OpLookupDynScope, gated on the check's binder/call-graph model
// (dynamicScopeReachable): some fn that BINDS the name must reach the
// reading fn, so a plain typo still refuses. The name joins dynScopeNames so
// Finalize installs the OpBindDynScope twin in every binding unit.
func (es *EmitState) dynScopeRescue(v core.Value) (EmitOperand, bool) {
	if es.reg == nil {
		return EmitOperand{}, false
	}
	name := v.DynFrom()
	if name == "" {
		name = es.defReads[v.ID]
	}
	if name == "" {
		return EmitOperand{}, false
	}
	if len(es.units) <= 1 {
		// Top level: an S5 first-value loop bind reads through the registry
		// here (its value has no event/local home by construction; the
		// splice bind installed the runtime binding before any read), and so
		// does a MODULE-FAMILY value — an import-bound namespace or a Module
		// descriptor read as a VALUE (`IO deq IO`, `M.$module eq M.$module`,
		// a namespace residual): the const gate refuses it on purpose (a
		// pointer-shared map of fn exports; ConstBakeable is closed to module
		// instances), its identity is the binding's, and the live lookup is
		// what honours a re-import after undef. Inside a unit the same read
		// already routes live through the enclosing-binding arm below.
		// Widening this arm to every def-read name poisons dynScopeNames
		// for defs whose bind then cannot lower (probe-pinned: the quoted
		// interp-body def refused "unknown provenance").
		if es.loopSplitBinds[name] || core.IsModuleFamilyValue(v) {
			if es.dynScopeNames == nil {
				es.dynScopeNames = map[string]bool{}
			}
			es.dynScopeNames[name] = true
			return dynScopeOperand(es.intern(core.NewString(name))), true
		}
		return EmitOperand{}, false
	}
	c := es.reg.Check
	reader := ""
	if n := len(es.unitNames); n > 0 {
		reader = es.unitNames[n-1]
	}
	if reader == "" {
		if n := len(c.FnNameStack); n > 0 {
			reader = c.FnNameStack[n-1]
		}
	}
	// An ENCLOSING-scope binding (a value snapshotted in the reading unit's
	// DefTable at open — a module-scope `flex`, a computed module `def`) is
	// unconditionally in dynamic scope for this fn: the interpreter resolves
	// the name against the live def stack and the VM's OpLookupDynScope does
	// the same against the module registry. It need not satisfy the fn-binder
	// reachability model (dynamicScopeReachable), which only covers names bound
	// INSIDE a fn frame; a plain typo is still refused because it is absent from
	// the snapshot AND has no fn binder.
	enclosing := false
	if len(es.units) > 1 {
		if cur := es.units[len(es.units)-1]; cur != nil {
			enclosing = cur.enclosingBindIDs[v.ID]
			// An ID-ELIDED enclosing read (a detached stamp forked the runtime
			// def table, where the mutable ref's ID was never minted) cannot be
			// matched by ID. Fall back to the by-name snapshot: the recovered
			// binding name being a live enclosing binding is a sound commit
			// signal — the VM's OpLookupDynScope reads the same live cell and
			// defers on a miss. Scoped to (a) the empty-ID case, so an ID-bearing
			// read stays strictly ID-matched (no by-name widening at depth 0), and
			// (b) a DynFrom-tagged read, so the name came from the module-scope
			// mutable-ref tag (engine.stepWord) — NOT the unreliable defReads[""]
			// that every elided-ID read collides on.
			if !enclosing && v.ID == "" && v.DynFrom() != "" {
				enclosing = cur.enclosingBindNames[name]
			}
		}
	}
	if !enclosing && !c.DynamicScopeReachable(name, reader) {
		return EmitOperand{}, false
	}
	if es.dynScopeNames == nil {
		es.dynScopeNames = map[string]bool{}
	}
	es.dynScopeNames[name] = true
	return dynScopeOperand(es.intern(core.NewString(name))), true
}

func (es *EmitState) NoteShapedRead(id string) {
	if !es.Active() || id == "" {
		return
	}
	if es.shapedReads == nil {
		es.shapedReads = map[string]bool{}
	}
	es.shapedReads[id] = true
}

// zeroArgMemberFnLandingOut reports whether any dispatch out is a
// PINPOINTED container-member fn read (noteMemberFnRead with the member
// value) whose member carries a GENUINE 0-arg overload — the break-2
// closure (design/FN-VALUE-OPEN-WORK.0.md §4). The read guard skips its
// auto-dispatch refusal for such a read because the arrival model owns
// the landing: tryMemberFnArrivalDispatch claims an empty-window arity-0
// OpCallDynMethod, and its guard-owned decline re-refuses any 0-arg
// landing it cannot claim — the guard is re-homed, never weakened. A tag
// WITHOUT the member value (a computed-key scan) never exempts: the
// landing model cannot pinpoint what it would apply.
func (es *EmitState) zeroArgMemberFnLandingOut(outs []core.Value) bool {
	for _, o := range outs {
		if m, ok := es.MemberFnReadValue(o.ID); ok && core.FnValueZeroArg(m) {
			return true
		}
	}
	return false
}

// shapedReadOut reports whether any dispatch out is an annotated shaped
// method read — the landing model's responsibility, not the read guard's.
func (es *EmitState) shapedReadOut(outs []core.Value) bool {
	if len(es.shapedReads) == 0 {
		return false
	}
	for _, o := range outs {
		if es.shapedReads[o.ID] {
			return true
		}
	}
	return false
}

// noteMemberFnRead records that the value with id came from a get-family read
// of a fn-valued container member, along with the MEMBER value itself when the
// read resolved it uniquely (a concrete container + concrete key —
// readFnMemberValue); a zero member records the tag alone (a computed-key scan
// knows a fn member exists somewhere but not which). Lazily-inits the side
// table; nil-safe and active-gated (a suspended / uncompilable pass records
// nothing).
func (es *EmitState) NoteMemberFnRead(id string, member core.Value) {
	if !es.Active() || id == "" {
		return
	}
	if es.memberFnReads == nil {
		es.memberFnReads = map[string]core.Value{}
	}
	es.memberFnReads[id] = member
}

// NoteCollectionHazard / CollectionHazard are the recorder side of the
// check pass's collection-hazard note (Engine.noteCollectionHazards,
// NUR121): the fn-typed value id sat unapplied below a value a later
// dispatch stack-collected in the same scope, so an apply of id over the
// values after it would run over that dispatch's RESULT. Every lead
// lowering declines a marked id — RecordDynApply (the paren lead window
// records through it), noteDynFrameReplay, resolveDynamicApply's lead arms.
func (es *EmitState) NoteCollectionHazard(id string) {
	if !es.Active() || id == "" {
		return
	}
	if es.collectionHazard == nil {
		es.collectionHazard = map[string]bool{}
	}
	es.collectionHazard[id] = true
}

func (es *EmitState) CollectionHazard(id string) bool {
	return es != nil && es.collectionHazard[id]
}

// NoteFnResultReStep is the recorder side of the check pass's RE-STEP note
// (Engine.noteFnResultReSteps, NUR124): a native dispatch's result v — a
// fn-typed or fn-admitting gradual carrier — is re-stepped into a dispatch
// attempt by the interpreter at the call's position, resuming at resume,
// where the pass stepped it past as data. The note lands on the PRODUCING
// event (the result's provenance), where planReStepDeopts reads it; a
// result with no event (a fold, a const) has no op to test after and notes
// nothing. A fn-TYPED result marks the note strict: the runtime value is
// certainly a fn, so a unit no deopt serves must refuse rather than keep
// the value as data.
func (es *EmitState) NoteFnResultReStep(v core.Value, resume core.SrcPos) {
	if !es.Active() || v.ID == "" {
		return
	}
	pr, ok := es.producedBy[v.ID]
	if !ok {
		return
	}
	if es.reStepNotes == nil {
		es.reStepNotes = map[int]reStepNote{}
	}
	n := es.reStepNotes[pr.seq]
	n.resume = resume
	n.strict = n.strict || (core.IsFnTypedCarrier(v) && !v.Dynamic)
	es.reStepNotes[pr.seq] = n
}

// hazardLead reports whether v is a marked lead the lowerings must decline:
// marked, and EAGER — a lead the interpreter dispatches where it is read (a
// param / capture word, a native word's fn result, an `args.N` read). A
// PLACED lead is lazy: a paren-placed single survivor is re-stepped only at
// the enclosing paren's close, over whatever survives there, and a user
// call's returned closure is parked where it lands (callResultPlaced) — so
// `((mk) 7 add 1)` is 9 on both lanes and the apply over add's result IS the
// interpreter's, hazard mark or not.
func (es *EmitState) hazardLead(v core.Value) bool { return es.hazardLeadIn(v, nil) }

// hazardLeadIn is hazardLead with the fragment whose events produced v (a
// unit's finish, an arm): the placed-call-result exclusion must find the
// producing event where it lives.
func (es *EmitState) hazardLeadIn(v core.Value, frag *EmitFragment) bool {
	return es.CollectionHazard(v.ID) && !es.parenPlacedMemberFn(v) && !es.callResultPlacedIn(v, frag) && !es.applyPending(v.ID)
}

// applyPending reports whether the innermost open unit holds a PENDING
// `apply` for the fn value id (a `/v`-read value the elided apply dispatch
// registered): such a value is applied by the apply word at ITS position,
// never eagerly where it was read, so it is not a collection-hazard lead —
// the pending-apply discipline at the unit's finish owns it.
func (es *EmitState) applyPending(id string) bool {
	if es == nil || len(es.units) == 0 {
		return false
	}
	for _, p := range es.units[len(es.units)-1].pendingApply {
		if p.id == id {
			return true
		}
	}
	return false
}

// ApplyPending is applyPending on the recorder seam (core's paren collapse
// asks it for a Dynamic last value).
func (es *EmitState) ApplyPending(id string) bool { return es.applyPending(id) }

// PendingClosureApply is the recorder seam the check pass's user-fn record
// site asks when a fn VALUE's re-step dispatch reaches it with no name to
// resolve the value through: the pending `apply`-word application of a
// produced closure on the innermost open unit whose value carries `body` —
// an own sig's body slice, matched by BACKING ARRAY (the dispatching
// ReturnsFn and the applied value hold the same construction's sig; a
// position would not do — a body whose one token is a `=>` group carries
// none). The entry is consumed by the RecordDynApply the caller then
// records, so a second closure of the same source applied later finds only
// its own entry.
func (es *EmitState) PendingClosureApply(body []core.Value) (core.Value, bool) {
	if es == nil || len(es.units) == 0 || len(body) == 0 {
		return core.Value{}, false
	}
	for _, p := range es.units[len(es.units)-1].pendingApply {
		fd, isFn := p.fn.Data.(core.FnDefInfo)
		if !isFn {
			continue
		}
		for _, sig := range fd.OwnSigs() {
			if b := sig.Body(); len(b) > 0 && &b[0] == &body[0] {
				return p.fn, true
			}
		}
	}
	return core.Value{}, false
}

// recordGradualApplyEvent records `apply` over a GRADUAL lead that the check
// matched against the [Reach Any] overload inside a unit: args[0] is the
// Dynamic lead (a `x:Any` param, a map get's result, a fn-typed carrier's own
// apply result), args[1] the one value beneath it the overload consumed, and
// outs[0] the one gradual result the model committed. The event lays out
// [receiver, lead] with the lead on top and lowers to OpCallDynApplyOne. It
// declines — leaving the standing refusal — outside a unit (the program
// residual has no single-consumer window), for a lead with no identity, and
// for operands the recorder cannot resolve. A receiver that is itself a fn
// value is DATA under the apply word (the thirty-second increment): the
// interpreter's applyHandler re-steps the lead over the RESOLVED stack,
// where a fn value never dispatches, so `cfalse/v (q/v p/v apply) apply`
// hands cfalse to the applied closure; a lead that is no fn at run time
// meets the word's own no-match on the top value, exactly as the op does.
func (es *EmitState) recordGradualApplyEvent(sig *core.Signature, args, outs []core.Value, pos core.SrcPos) bool {
	if len(es.units) <= 1 || sig == nil || sig.FnFrame() != nil || sig.TotalArgs() != 2 ||
		len(args) != 2 || len(outs) != 1 {
		return false
	}
	lead := args[0]
	if core.IsConcrete(lead) || !lead.Dynamic || lead.ID == "" {
		return false
	}
	fnOp, ok := es.resolveOperand(lead)
	if !ok {
		return false
	}
	argOp, ok := es.resolveOperand(args[1])
	if !ok {
		return false
	}
	// The apply word consumed a bare read of the lead (NUR123 accounting).
	es.creditWordRead(lead.ID)
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: wordDynApply, ops: []EmitOperand{fnOp, argOp}, nout: 1, pos: pos, dynApply: 1, dynApplyUnquote: true, dynApplyOne: true}})
	es.setProduced(outs[0], seq)
	return true
}

// memberFnRead reports whether id was tagged by noteMemberFnRead.
func (es *EmitState) MemberFnRead(id string) bool {
	if es == nil || es.memberFnReads == nil {
		return false
	}
	_, ok := es.memberFnReads[id]
	return ok
}

// memberFnReadValue returns the uniquely-resolved member FN value tagged for
// id (REFUSAL-CLOSURE.0 §3 — the arrival-apply model needs the member's
// signature to claim its window). ok=false for an untagged id or a tag whose
// read could not pinpoint the member.
func (es *EmitState) MemberFnReadValue(id string) (core.Value, bool) {
	if es == nil || es.memberFnReads == nil {
		return core.Value{}, false
	}
	m, ok := es.memberFnReads[id]
	if !ok {
		return core.Value{}, false
	}
	if _, isFn := m.Data.(core.FnDefInfo); !isFn {
		return core.Value{}, false
	}
	return m, true
}

// readsFnMember reports whether a get-family dispatch surfaces a FUNCTION-valued
// member from a concrete container — the member the interpreter auto-applies
// the moment a value lands on it (design/EDGE-SPEC-FINDINGS.0.md §2). Unlike
// containerFnAutoDispatchRisk (which gates on GENUINE 0-param fns only, so the
// statement-tail apply `m.double 21` keeps compiling), this reports ANY arity:
// the caller tags the read so a MID-EXPRESSION stranded apply can refuse while
// the tail apply is unaffected. Mirrors containerFnAutoDispatchRisk's
// container/key walk (map member, list element, flat-instance field).
func readsFnMember(args []core.Value) bool {
	isFn := func(v core.Value) bool { _, ok := v.Data.(core.FnDefInfo); return ok }
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch d := a.Data.(type) {
		case core.ListPayload:
			if idx, ok := concreteIntKey(args); ok {
				if idx >= 0 && idx < int64(len(d.Elems)) && isFn(d.Elems[idx]) {
					return true
				}
				continue
			}
			for _, e := range d.Elems {
				if isFn(e) {
					return true
				}
			}
		case core.MapPayload:
			if d.M == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := d.M.Get(key); hit && isFn(mv) {
					return true
				}
				continue
			}
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				if isFn(mv) {
					return true
				}
			}
		default:
			fields, _, isInst := core.FlatInstanceParts(a)
			if !isInst || fields == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := fields.Get(key); hit && isFn(mv) {
					return true
				}
				continue
			}
			for _, k := range fields.Keys() {
				mv, _ := fields.Get(k)
				if isFn(mv) {
					return true
				}
			}
		}
	}
	return false
}

// readFnMemberValue is readsFnMember's value-returning sibling (REFUSAL-
// CLOSURE.0 §3): when the get-family dispatch resolves a UNIQUE fn member —
// a concrete container with a concrete key/index — it returns that member so
// the arrival-apply model can claim its signature's window. A computed-key
// scan (readsFnMember's every-member walk) cannot pinpoint one member, so it
// reports ok=false and the read carries the bool tag alone (the model then
// declines and today's refusal paths stand).
func readFnMemberValue(args []core.Value) (core.Value, bool) {
	isFn := func(v core.Value) bool { _, ok := v.Data.(core.FnDefInfo); return ok }
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch d := a.Data.(type) {
		case core.ListPayload:
			if idx, ok := concreteIntKey(args); ok {
				if idx >= 0 && idx < int64(len(d.Elems)) && isFn(d.Elems[idx]) {
					return d.Elems[idx], true
				}
			}
		case core.MapPayload:
			if d.M == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := d.M.Get(key); hit && isFn(mv) {
					return mv, true
				}
			}
		}
		// Class-instance receivers never reach this payload walk: the make
		// result is a CARRIER at the read site (schema only), so instance
		// members ride the construction-time fnMemberFields note instead
		// (instanceFnMember at the tag site) — the fix for the formerly
		// pre-existing `o.f 21 eq 42` stranded-apply miscompile.
	}
	return core.Value{}, false
}

func containerFnAutoDispatchRisk(args []core.Value) bool {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch d := a.Data.(type) {
		case core.ListPayload:
			if idx, ok := concreteIntKey(args); ok {
				if idx >= 0 && idx < int64(len(d.Elems)) && core.FnValueZeroArg(d.Elems[idx]) {
					return true
				}
				continue
			}
			for _, e := range d.Elems {
				if core.FnValueZeroArg(e) {
					return true
				}
			}
		case core.MapPayload:
			if d.M == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := d.M.Get(key); hit && core.FnValueZeroArg(mv) {
					return true
				}
				continue
			}
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				if core.FnValueZeroArg(mv) {
					return true
				}
			}
		default:
			// Flat instances (class / Resource) expose FIELD reads through
			// the same get family, and the interpreter auto-dispatches a
			// landed fn field exactly like a map member (probe-verified:
			// `def C class {f:Function} def o (make C {f:make42/v}) o.f`
			// -> 42 interpreted, `fn make42` compiled — PR #225 P1).
			// Same key-precision rule as MapPayload.
			fields, _, isInst := core.FlatInstanceParts(a)
			if !isInst || fields == nil {
				continue
			}
			if key, ok := concreteMapKey(args); ok {
				if mv, hit := fields.Get(key); hit && core.FnValueZeroArg(mv) {
					return true
				}
				continue
			}
			for _, k := range fields.Keys() {
				mv, _ := fields.Get(k)
				if core.FnValueZeroArg(mv) {
					return true
				}
			}
		}
	}
	return false
}

// concreteMapKey returns the map-key string a get-family dispatch reads,
// when one of the args is a concrete Atom or String key.
func concreteMapKey(args []core.Value) (string, bool) {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		switch k := a.Data.(type) {
		case core.AtomPayload:
			return k.Name, true
		case core.StrPayload:
			return k.S, true
		}
	}
	return "", false
}

// concreteIntKey returns the list index a get-family dispatch reads, when
// one of the args is a concrete Integer key.
func concreteIntKey(args []core.Value) (int64, bool) {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		if k, ok := a.Data.(core.IntPayload); ok {
			return k.N, true
		}
	}
	return 0, false
}

// producerWord returns the word of the event that produced value id, when id
// resolves to a recorded event (not a const / local / unproduced value). Used to
// gate makelist on its elements being core-builtin (deterministic) results.
func (es *EmitState) producerWord(id string) (string, bool) {
	pr, ok := es.producedBy[id]
	if !ok {
		return "", false
	}
	for _, fr := range es.frames {
		for i := range fr {
			if fr[i].seq == pr.seq {
				return fr[i].call.word, true
			}
		}
	}
	return "", false
}

// isGetFamilyWord reports whether word is a container member accessor
// (get/dot and their r-variants) — the words whose read of a FUNCTION value
// auto-dispatches it in the interpreter (recordCallRefusal).
func isGetFamilyWord(w string) bool {
	return w == "get" || w == "dot" || w == "getr" || w == "dotr"
}

// producerReturnedClosureArity resolves the declared arity of the closure a
// leading fn-typed CARRIER holds, when its producer is a compiled user call
// whose fn returns exactly one anonymous lambda (the factory pattern).
// Recoverable arity lets resolveDynamicApply distinguish the single N-arg
// apply `(mk 5) 10 20` from a curried CHAIN `((mk 1) 2) 3` — the flattened
// residual is identical for both, and committing one OpCallDynamic over a
// chain leaks the intermediate closure (miscompile mechanism E,
// nested-factory apply, design/MISCOMPILE-HUNT-FINDINGS.0.md).
//
// The factory returns its lambda one of two ways, and both are recoverable:
// a CLOSURE when the body reads an enclosing binding (`( fn [[x:Integer]
// [Integer][x add n]] )` over the factory's `n` — a PUSH_CLOSURE out op
// naming the compiled unit), and a CONST when it captures nothing (`[x add
// 1]` — the fold bakes the whole lambda as one value). Only the closure
// arm existed, so every capture-free factory refused as "closure shape
// unknown" though its arity is written on the const's own signature.
//
// A third source (the thirty-fifth increment): a computed-fn carrier NOTED
// with its arity by the producing word's own check-mode ReturnsFn
// (CheckState.FnShapes — the fn-util wrappers, `def k (FnUtil.const 7)`).
// The word built the runtime value with exactly that many params, so the
// claim is a statement of construction like the two arms below, and the
// def-bound wrapper's apply classifies like a compiled factory's closure.
func (es *EmitState) producerReturnedClosureArity(id string) (int, bool) {
	if es.reg != nil {
		if n, ok := es.reg.Check.FnShapeArity(id); ok {
			return n, true
		}
	}
	s, ok := es.producerReturnedClosureShape(id)
	return s.Arity, ok
}

// producerReturnedClosureShape is the SHAPE of the fn value a producer's
// returned out-op builds (closureOpShape), for the claim a def of it writes.
func (es *EmitState) producerReturnedClosureShape(id string) (core.FnShape, bool) {
	op, ok := es.producerReturnedOutOp(id)
	if !ok {
		return core.FnShape{}, false
	}
	return es.closureOpShape(op, 0)
}

// closureOpShape is the shape of the fn value an out-op PRODUCES: a closure
// unit's param count, its RESULT recursing through the unit's own single
// out-op when that is a closure too (a factory of factories claims the whole
// chain: `mk2`'s `(f1 2)` returns the next level), or a const lambda's param
// count. depth bounds the recursion — a chain deeper than any program writes
// is a construction fault, not a shape.
func (es *EmitState) closureOpShape(op EmitOperand, depth int) (core.FnShape, bool) {
	if depth > 8 {
		return core.FnShape{}, false
	}
	switch op.kind {
	case opClosure:
		cu := op.closureUnit
		if cu < 0 || cu >= len(es.fnRecs) {
			return core.FnShape{}, false
		}
		s := core.FnShape{Arity: es.fnRecs[cu].nParams}
		if outs := es.fnRecs[cu].outOps; len(outs) == 1 {
			if r, ok := es.closureOpShape(outs[0], depth+1); ok {
				s.Result = &r
			}
		}
		return s, true
	case opConst:
		n, ok := constLambdaArity(es.consts, op.idx)
		return core.FnShape{Arity: n}, ok
	}
	return core.FnShape{}, false
}

// noteClosureShapeBind claims, at a def of a PRODUCED closure, the shape its
// producer builds (the thirty-sixth increment): the read model then owns the
// def-bound closure's word dispatch exactly as it owns a fn-util wrapper's
// (check's tryShapedFnReadArrival) — the residual classifier's flattened
// window lost the statement for these too (`h 2 ; 3` over a two-param
// closure lowered as `h 2 3`, compiled 12 for the interpreter's
// signature_error). A producing word's own claim stands.
func (es *EmitState) noteClosureShapeBind(v core.Value) {
	if es.reg == nil || v.ID == "" || !core.IsFnTypedCarrier(v) {
		return
	}
	if _, claimed := es.reg.Check.FnShapeOf(v.ID); claimed {
		return
	}
	if s, ok := es.producerReturnedClosureShape(v.ID); ok {
		es.reg.Check.NoteFnShape(v, s)
	}
}

// appendResidualSeqs collects the producing-event seqs a residual's
// operands reference — the unit's own out-ops AND, for a closure operand,
// its lexical CAPTURES. A capture is an enclosing-scope operand like any
// other: unless the promotion planner counts it, its producer is never
// given a frame slot, the operand stays an opEvent, and the closure push
// materialises it through pushOperand's const arm — baking the event's
// SEQ as a const index (NUR126). The event-stream operands are surfaced
// by forEachOperand's closure arm for exactly this reason; a RESIDUAL
// closure (a returned lambda) reaches the planner only through here.
func appendResidualSeqs(dst []int, ops []EmitOperand) []int {
	for _, op := range ops {
		switch op.kind {
		case opEvent:
			if op.resIdx == 0 {
				dst = append(dst, op.idx)
			}
		case opClosure:
			dst = appendResidualSeqs(dst, op.closureCaps)
		}
	}
	return dst
}

// producerReturnedOutOp is the single out op of the unit a compiled user
// call ran for id's producer — the factory's one returned value, or
// ok=false when the producer is anything else (a native call, a
// multi-output unit, an unknown id).
func (es *EmitState) producerReturnedOutOp(id string) (EmitOperand, bool) {
	pr, ok := es.producedBy[id]
	if !ok {
		return EmitOperand{}, false
	}
	for _, fr := range es.frames {
		for i := range fr {
			if fr[i].seq != pr.seq {
				continue
			}
			if fr[i].kind != evCallUser {
				return EmitOperand{}, false
			}
			unit := fr[i].uc.unit
			if unit < 0 || unit >= len(es.fnRecs) {
				return EmitOperand{}, false
			}
			rec := es.fnRecs[unit]
			if len(rec.outOps) != 1 {
				return EmitOperand{}, false
			}
			return rec.outOps[0], true
		}
	}
	return EmitOperand{}, false
}

// producedFnValue reports whether id holds a fn value the compiled program
// PRODUCES at run time — a closure a user call's unit returns
// (producerReturnedClosure) or the result of a compiled fn-value apply
// (a wordDynApply event: the staged combinators' `(x (bb f) apply) apply`,
// whose inner apply nets the next closure). Either is a runtime fn value
// the apply op applies from its producer operand — the `apply` word's
// pending-application arm (recordCallElided) admits both.
func (es *EmitState) producedFnValue(id string) bool {
	if es.producerReturnedClosure(id) {
		return true
	}
	w, ok := es.producerWord(id)
	return ok && w == wordDynApply
}

// producerReturnedClosure reports whether id holds a compiled CLOSURE this
// pass produced (an OpPushClosure out op). That value is a ClosurePayload,
// invokable only through the VM's re-entrant runner — the property the two
// callers below guard. A capture-free lambda the fold baked as a CONST is
// an ordinary interpreter-invokable fn value and is deliberately not one.
func (es *EmitState) producerReturnedClosure(id string) bool {
	op, ok := es.producerReturnedOutOp(id)
	return ok && op.kind == opClosure
}

// constLambdaArity reads the arity off a baked ANONYMOUS lambda const —
// one own signature with a boru body, no name and no module origin. The
// exclusions are the fn-value apply's soundness argument (resolveDynamicApply):
// OpCallDynamic runs anonymous-VALUE semantics, which leave a NAMED Go-impl
// fn (`add/v`, a module export, a `FnUtil.const` result) as data where the
// interpreter's word dispatch would apply it, so those keep the refusal.
func constLambdaArity(consts []core.Value, idx int) (int, bool) {
	if idx < 0 || idx >= len(consts) {
		return 0, false
	}
	fd, ok := consts[idx].Data.(core.FnDefInfo)
	if !ok || fd.Name != "" || fd.Module != "" {
		return 0, false
	}
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	lam, hasOwn := fd.FirstOwnSig()
	if own != 1 || !hasOwn || len(lam.Body()) == 0 {
		return 0, false
	}
	return len(lam.Params), true
}

// makeListRange reports whether any of a dispatch's args was produced by an
// OpMakeList assembly (the synthetic "[…]" word) — used to keep `for` off a
// computed range list.
func makeListRange(es *EmitState, args []core.Value) bool {
	for i := range args {
		if w, ok := es.producerWord(args[i].ID); ok && w == wordMakeList {
			return true
		}
	}
	return false
}

// RecordMakeList records the assembly of a COMPUTED list literal (`[1 add 2]`,
// `[1 (2 add 3) 4]`): autoEvalList evaluated the elements (their dispatches
// already recorded their own events) and `ins` are the resulting element
// values, in order; `out` is the list. The N element operands resolve normally
// (an event result, a const, a local) and the dispatch lowers to OpMakeList N,
// which pops them and pushes the list. Returns false — leaving es untouched, so
// the list stays an unresolvable residual and the program falls back — when an
// element has no compiled home (e.g. a fn value, a nested dynamic carrier).
func (es *EmitState) RecordMakeList(r *core.Registry, ins []core.Value, out core.Value, pos core.SrcPos) bool {
	if !es.Active() {
		return false
	}
	// Only the TOP-LEVEL frame. A list inside a fn body / higher-order closure /
	// branch arm (a nested fragment) is RE-EVALUATED per call or iteration, often
	// with a different scope (`fn […[[c1]]]`, where c1 rebinds). Baking ONE
	// assembly of the check-mode evaluation would freeze that — so those keep
	// refusing and fall back. A top-level computed list is evaluated once.
	if len(es.frames) != 1 {
		return false
	}
	return es.RecordMakeListInner(r, ins, out, pos)
}

// RecordMakeListInner is the guard-free core of RecordMakeList: it resolves the
// element operands and appends the OpMakeList event. RecordMakeList wraps it with
// the top-frame restriction (a fn-body residual list must fall back). The other
// caller is RecordMakeMap, for a LIST-valued map entry (`{n:[expr]}`) whose list
// is itself a CONSUMED-in-frame operand of the enclosing OpMakeMap — there the
// top-frame guard does NOT apply (OpMakeList re-assembles from its operands per
// run, exactly like OpMakeMap, so it is sound in a fn body / branch / loop).
func (es *EmitState) RecordMakeListInner(r *core.Registry, ins []core.Value, out core.Value, pos core.SrcPos) bool {
	// A SUSPENDED recorder (a higher-order body run for type inference —
	// analyseHigherOrderBodyVals suspends recording) records no events. The
	// autoEvalList / autoEvalMap CONSUMED-list callers gate on Armed() (a
	// recording state exists) not active() (armed AND not suspended), so during a
	// fold/each/scan accumulator fixed point they would otherwise record a
	// consumed list — e.g. `{… q:[q]}` inside split-args' fold — into the OUTER
	// unit's frame, producing an OpLookupDynScope with no coherent bind that
	// misses at runtime (the voxgig-template liquid `q` fold divergence). Guard at
	// the recording boundary so every caller (RecordMakeList already pre-checks;
	// the two autoEval paths do not) is covered. RecordMakeList's own active()
	// check stays reachable — its isTop caller still reaches it under suspend.
	if !es.Active() {
		return false
	}
	// A CODE BODY the read substitution corrupted. Stage 1 substitutes a
	// def-bound computed fn's carrier for a read of its name, which is
	// right where the read is an operand — and wrong inside an
	// UNEVALUATED body, where the name is a body TOKEN. `each [1 2 3]
	// [(f 5)]` assembled the body as the DATA list [f, 5] (this record),
	// dropping each's own input list, and compiled `[3 3]` for the
	// interpreter's `[3]`; `do [(f 2)]` compiled to an island that
	// raised `undefined word: f` where the interpreter answers 3. A
	// table carrier reaching a list MEMBER is exactly that corruption and
	// nothing else — a genuine data list of a computed fn spells the
	// member `f/v`, and stepWordVal deliberately never consults the
	// table — so refuse and let the interpreter fallback own it.
	if r != nil {
		for i := range ins {
			if _, tabled := core.CheckFnCarrierBoundName(r, ins[i].ID); tabled {
				es.MarkUncompilable(
					"computed fn read inside an unevaluated body (the read substitution cannot model a code body — Stage 1)")
				return false
			}
		}
	}
	// A list literal whose LEAD element an enclosing paren re-stepped into a
	// call (the paren re-step rule, design/PAREN-RESTEP-RULE.0.md).
	// `[((mk 1) 2)]` is `[3]` interpreted — the inner paren leaves two
	// survivors, the park declines, and the rewind dispatches the carrier —
	// while this assembly bakes the pair as TWO elements and answers
	// `[fn (Integer) 2]`. That was NUR101's original symptom, and it was
	// silent.
	//
	// The re-step record is what makes the two spellings separable at all:
	// `[(mk 1) 2]` reaches here with the SAME `[carrier, 2]` elements and the
	// interpreter really does place them, so a test on the lead's shape alone
	// would break the correct one. Return false rather than MarkUncompilable —
	// an unrecorded list is already an unresolvable residual and the program
	// falls back — until Stage 3 can record the apply as an element event.
	if len(ins) >= 2 && es.parenReSteppedFn(ins[0]) {
		return false
	}
	// ops are in SIG order (ops[0] = top of stack), but a list assembles with
	// element 0 DEEPEST, so reverse: ops[0] is the LAST element (laid out on
	// top), ops[N-1] the first (deepest). OpMakeList then pops [first..last] and
	// builds [first..last] in order. Each element must be produced by a CORE
	// BUILTIN (or be a const): a builtin that yields a value is deterministic and
	// side-effect-free, so the lowered re-computation matches. A MODULE / user
	// word may be stateful — `list-of [Rand.int 0 10] 3` leaks its NoEval
	// generator to the residual, and freezing one `rand-int` (which advances the
	// seed) would replicate it instead of re-running per iteration. Those refuse.
	ops := make([]EmitOperand, len(ins))
	for i := range ins {
		// A bare type node element is a first-class SINGLETON VALUE (ADR-010 /
		// NUR051), not declaration-only syntax: it assembles through the same
		// interned canonical-ID type operand a standalone bare type node
		// resolves to (OpPushType), so the runtime element is the canonical
		// registry node — `[None]` compiles exactly as it interprets, and a
		// computed list that feeds the type machinery (`x is […]`) carries the
		// identical canonical nodes the interpreter's list would. A node with
		// no canonical ID has no runtime home and still refuses.
		if core.IsBareTypeNode(ins[i]) {
			if ins[i].ID == "" {
				return false
			}
			ops[len(ins)-1-i] = typeOperand(es.internType(ins[i]))
			continue
		}
		// A non-builtin (user-fn / module) producing word is NOT refused: the element
		// resolves to its recorded EVENT (CALL_USER / CALL_NATIVE_POLY / make), and
		// the OpMakeList assembly RE-RUNS that event at runtime — it never freezes the
		// check-mode result. So a list of user-fn / module results (`def specs
		// [(Test.test …) …]`, the voxgig spec-list pattern) assembles faithfully,
		// exactly as `make` instances and builtin results already do. A genuinely
		// stateful generator (`list-of [Rand.int] N`) is a NoEval CODE BODY run per
		// iteration, not a list-literal element, so it never reaches RecordMakeListInner.
		// resolveOperand only ever yields a re-running event or an inert const here
		// (never a frozen module result), so the assembly stays sound; gated by the
		// bytecode differential + the voxgig --compile==interpret sweep.
		op, ok := es.resolveOperand(ins[i])
		if !ok {
			return false
		}
		ops[len(ins)-1-i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: wordMakeList, ops: ops, nout: 1, pos: pos, makeList: true}})
	es.setProduced(out, seq)
	return true
}

// RecordMakeMap records the assembly of a COMPUTED map literal whose values are
// not bakeable as an inert const — `make`'s construction body with a computed
// field value (`make Outer {i:(make Inner …)}`, `{a:(context get 'x')}`).
// autoEvalMap evaluated each value (their dispatches already recorded their own
// events) and `vals` are the resulting values in `keys` order; `out` is the map.
// The N value operands resolve normally (an event result, a const, a local) and
// the dispatch lowers to OpMakeMap, which pops them and pairs each with its key.
// The keys ride in the Program (MakeMapSpec) rather than the stack, so only the
// values are operands. Returns false — leaving es untouched, so the map stays an
// unresolvable residual and the program falls back — when a value has no compiled
// home (a fn value, a nested dynamic carrier) or is a bare type node (a
// type-pattern map, not a data map). The sole caller (autoEvalMap) gates this on
// dataMap — make's CONSUMED construction body — which make evaluates in the
// current scope, so it is sound inside a fn body / branch arm: the OpMakeMap
// re-assembles from its operands per call, never frozen, and is never a deferred
// residual. No top-frame restriction here.
func (es *EmitState) RecordMakeMap(r *core.Registry, keys []string, vals []core.Value, implicit bool, out core.Value, pos core.SrcPos) bool {
	if !es.Active() || len(keys) != len(vals) || len(keys) == 0 {
		return false
	}
	// ops are in value order (vals[0] pairs with keys[0]); OpMakeMap reads the
	// popped run deepest-first as value 0, so reverse like RecordMakeList:
	// ops[0] is the LAST value (laid out on top), ops[N-1] the first (deepest).
	ops := make([]EmitOperand, len(vals))
	for i := range vals {
		// A bare type node member is a first-class SINGLETON VALUE (ADR-010 /
		// NUR051), not declaration-only syntax: it assembles through the same
		// interned canonical-ID type operand a standalone bare type node
		// resolves to (OpPushType), so the runtime member is the canonical
		// registry node — identical to the interpreter's map, `is`/`typeof`
		// dispatch over the assembled map included. `{x: a  r: None}` therefore
		// compiles exactly as it interprets. A node with no canonical ID has no
		// runtime home and still refuses.
		if core.IsBareTypeNode(vals[i]) {
			if vals[i].ID == "" {
				return false
			}
			ops[len(vals)-1-i] = typeOperand(es.internType(vals[i]))
			continue
		}
		// A computed value produced by `make` (a mutable instance) is exactly what
		// this assembly exists to thread as a fresh per-run event — unlike
		// RecordMakeList, which keeps instance lists on the typed-def const-bake
		// path. So no make-producer exclusion here.
		op, ok := es.resolveOperand(vals[i])
		if !ok {
			return false
		}
		ops[len(vals)-1-i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: wordMakeMap, ops: ops, nout: 1, pos: pos,
		makeMap: true, mapKeys: append([]string(nil), keys...), mapImpl: implicit,
	}})
	es.setProduced(out, seq)
	return true
}

// RecordInterp records the assembly of a template string whose holes are
// computed — “ `got ${x}` “, “ `n=${1 add 2}` “, “ `t=${typeof x}` “ —
// into an OpInterp dispatch. The hole expressions ran in evalInterpParts (their
// own dispatches already recorded their events); holeVals are the per-hole
// result values in source order, and out is the resulting String. Each hole
// resolves like a call operand (an event result, a frame local, a bare-type
// node, or an inert const); the literal segments ride in the Program's
// InterpSpec, so OpInterp only pops the hole VALUES — exactly the MakeMap shape.
//
// Like OpMakeMap (and unlike the const-baking OpMakeList), OpInterp re-assembles
// from its operands on every run, so it is sound inside a fn body / branch arm /
// loop — no top-frame restriction. Returns false, leaving es untouched (the
// caller then marks the program uncompilable and it falls back), when a hole has
// no compiled home or did not produce exactly one value.
func (es *EmitState) RecordInterp(parts []core.InterpPart, holeVals []core.Value, out core.Value, pos core.SrcPos) bool {
	if !es.Active() || len(holeVals) == 0 {
		return false
	}
	// ops in stack order (ops[0] = top): OpInterp pops the run and reads it
	// deepest-first as hole 0, so reverse like RecordMakeMap — ops[0] is the LAST
	// hole (on top), ops[N-1] the first (deepest).
	ops := make([]EmitOperand, len(holeVals))
	for k := range holeVals {
		op, ok := es.resolveOperand(holeVals[k])
		if !ok {
			return false
		}
		ops[len(holeVals)-1-k] = op
	}
	segs := make([]InterpSeg, 0, len(parts))
	for _, p := range parts {
		if p.Expr == nil {
			segs = append(segs, InterpSeg{Lit: p.Lit})
		} else {
			segs = append(segs, InterpSeg{Hole: true})
		}
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: wordInterp, ops: ops, nout: 1, pos: pos,
		interp: true, interpSegs: segs,
	}})
	es.setProduced(out, seq)
	return true
}

// RecordInterpXml is RecordInterp's XML twin (REFUSAL-CLOSURE §9.2c): the
// template skeleton rides in Program.XmlInterps and OpInterpXml pops the
// hole VALUES in traversal order (BuildXmlFromTmpl's attrs-then-children
// depth-first walk — the same order the hole dispatches recorded their
// events). Returns false, leaving es untouched, when a hole has no
// compiled home — the caller then refuses and the program falls back.
func (es *EmitState) RecordInterpXml(tmpl core.XmlTmpl, holeVals []core.Value, out core.Value, pos core.SrcPos) bool {
	if !es.Active() || len(holeVals) == 0 {
		return false
	}
	// ops in stack order (ops[0] = top): OpInterpXml pops the run and reads
	// it deepest-first as hole 0, so reverse exactly like RecordInterp.
	ops := make([]EmitOperand, len(holeVals))
	for k := range holeVals {
		op, ok := es.resolveOperand(holeVals[k])
		if !ok {
			return false
		}
		ops[len(holeVals)-1-k] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: "__interpxml", ops: ops, nout: 1, pos: pos,
		xmlTmpl: &tmpl,
	}})
	es.setProduced(out, seq)
	return true
}

// RecordSpliceDyn records a splice marker's spread over a COMPUTED payload
// (REFUSAL-CLOSURE §9.2b): the payload rides as the one operand, the VM
// spreads or defers (OpSpliceDyn), and the result is VARIADIC — the runtime
// count is the payload's own — so only the program residual absorbs it. The
// The spread result gets a FRESH carrier (distinct provenance) so it never
// clobbers the payload def's own producer.
func (es *EmitState) RecordSpliceDyn(payload core.Value, pos core.SrcPos) bool {
	if !es.Active() {
		return false
	}
	if len(es.units) != 1 || (es.reg != nil && es.reg.Check.NestedBodyDepth != 0) {
		// TOP-FRAME straight-line only: inside a closure unit or a
		// branch/loop body analysis the variadic spread has no residual
		// home (probe-pinned: `do [word xs]` closure-compiled the spread
		// as [7 [7 8]] vs the interpreter's [7 8]) — declining leaves the
		// dyn-body backstop / the refusal, both parity-faithful.
		return false
	}
	op, ok := es.resolveOperand(payload)
	if !ok {
		return false
	}
	es.SiteCounts[SiteDynamic]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{
		word: "__splicedyn", ops: []EmitOperand{op}, nout: 1, pos: pos,
		spliceDyn: true,
	}})
	f := es.eventInfo[seq]
	f.variadicResult = true
	f.spliceDyn = true
	es.eventInfo[seq] = f
	es.setProduced(payload, seq)
	return true
}

// RecordClosureCall records a higher-order word's dispatch where the code BODY
// at position bodyPos was compiled to closure unit `unit` (plan P2). The body
// operand lowers to OpPushClosure (the handler invokes it through the VM via
// the InvokeBody seam); the other operands resolve normally, except any slot
// in extraOps (an extra LAMBDA hook — walk's ASCEND slot, Stage M2d — compiled
// to its own closure unit by recordClosureDispatch), which rides its prepared
// opClosure operand. Returns false, leaving es UNTOUCHED, when an operand is
// dynamic or of unknown provenance — the caller then keeps the island path.
func (es *EmitState) RecordClosureCall(word string, sig *core.Signature, args []core.Value, bodyPos, unit int, capOps []EmitOperand, extraOps map[int]EmitOperand, outs []core.Value, retSpec *ClosureRetSpec, pos core.SrcPos) bool {
	// A whole-residual word (CallableSpec.BodyOutResidual — `do`) may seat
	// N > 1 results: recordClosureDispatch has already asserted the unit's
	// compiled residual count equals len(outs), and the multi-result seating
	// below mirrors the generic RecordCall tail. Other callers stay 0/1-out.
	if !es.Active() || sig == nil ||
		(len(outs) > 1 && (sig.Callable == nil || sig.Callable.BodyOut != core.BodyOutResidual)) {
		return false
	}
	// A dynamic OUTPUT is fine — the body is compiled and the result type being
	// Any only means a downstream typed dispatch over it polys or refuses. A
	// dynamic non-body INPUT is handled by resolveOperand below: when it is a
	// resolvable event (a caught `do` error reaching `error [handler]`, whose
	// closure input is a FIXED carrier independent of the dynamic value — so the
	// handler runs faithfully over whatever the value turns out to be) it rides
	// as a stack operand; when it cannot resolve, resolveOperand declines and the
	// island path stands. Faithfulness rides the differential gate.
	ops := make([]EmitOperand, len(args))
	for i := range args {
		if i == bodyPos {
			ops[i] = EmitOperand{kind: opClosure, closureUnit: unit, closureCaps: capOps, closureRet: retSpec}
			continue
		}
		if exOp, isExtra := extraOps[i]; isExtra {
			ops[i] = exOp
			continue
		}
		op, ok := es.resolveOperand(args[i])
		if !ok {
			return false
		}
		ops[i] = op
	}
	es.SiteCounts[SiteMono]++
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: word, sig: sig, ops: ops, nout: len(outs), pos: pos}})
	// A fallible multi-value catch body (the ReturnsFn latched it): the
	// runtime count is N on no-raise but 1 on the caught path, so the
	// result region is VARIADIC — the residual absorbs it; a fixed-arity
	// consumer keeps the refusal (plan Phase 5, L-DO).
	if es.catchVariadicFor(sig) {
		f := es.eventInfo[seq]
		f.variadicResult = true
		es.eventInfo[seq] = f
	}
	// VARIADIC PROPAGATION through a strip-input dispatch (L-DO part 2):
	// `error` over a variadic region consumes exactly the region's TOP at
	// run time (a depth-agnostic pop) and pushes its result — the region
	// stays a region, so this event's result is variadic too and the
	// program residual absorbs it. Fixed-arity consumers keep refusing.
	if sig.Callable != nil && sig.Callable.StripsUnconsumedInput {
		for i := range args {
			if i == bodyPos {
				continue
			}
			if pr, ok := es.producedBy[args[i].ID]; ok && es.eventInfo[pr.seq].variadicResult {
				f := es.eventInfo[seq]
				f.variadicResult = true
				es.eventInfo[seq] = f
				break
			}
		}
	}
	for i := range outs {
		es.setProducedAt(outs[i], seq, i)
	}
	return true
}

// hasUncoveredQuoteArg reports whether sig has a QuoteArgs position that is NOT
// also a NoEvalArgs position. The QuoteArgs refusal targets dispatch-
// manipulating meta operands (usurp / force-arity / ref-family) whose result the
// engine re-steps; a quoted operand that is ALSO a NoEvalArgs code body
// (`timeout 1000 [body]`, `interval`) is already validated as inert by the
// noEvalBodiesInert path and bakes as a plain const, so it must not be double-
// refused here.
func hasUncoveredQuoteArg(sig *core.Signature) bool {
	for i := range sig.QuoteArgs {
		if sig.QuoteArgs[i] && !sig.NoEvalArgs[i] {
			return true
		}
	}
	return false
}

// noEvalBodiesInert reports whether every NoEvalArgs (un-evaluated code-body)
// position holds INERT data — a const-bakeable word-list / scalar with no
// computed paren or carrier (e.g. a query clause `[name age]` / `[age gt 1]`,
// read or stored as data by the handler). Such a body bakes as a const and the
// dispatch lowers to a plain CALL_NATIVE: the handler does exactly what it does
// under the interpreter, byte-identically. A body with a computed paren (a test
// assertion `[(1 add 1) …]`) is NOT inert and keeps the conservative refusal.
func noEvalBodiesInert(sig *core.Signature, args []core.Value) bool {
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		if !core.IsInertConst(args[i]) {
			return false
		}
		// A flow-control sentinel (break/continue/return) inside the body
		// targets an ENCLOSING loop/frame; running the body inside the handler
		// (the CALL_NATIVE this enables) cannot propagate that across the call
		// boundary, so it would diverge (`each [break]`). Keep those refused.
		if check.BodyHasSentinel(args[i]) {
			return false
		}
		if bodyHasReplayHazard(args[i]) {
			return false
		}
	}
	return true
}

// bodyHasReplayHazard reports whether a code body that would bake as an
// inert const and be RE-RUN by its handler at VM time (InvokeBody replaying
// the token list through a sub-engine over the live registry) contains a
// statement whose check-time execution left registry state the replay
// double-applies or half-misses — the do-unit registry-replay miscompile
// class (design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md, Phase 6 item):
//
//   - a CAPITALISED def/var (a type install): the check-time run of the body
//     (RunCarrierBodyWithDefs) rolls back only the Defs binding — the minted
//     lattice node and the Types name-part registration survive into the
//     kept compiled-path state, so the replayed InstallType raises a
//     name-part conflict where the interpreter installs cleanly;
//   - an `import`: the module-loaded record survives while the namespace
//     binding is truncated, so the replay re-binds through the
//     already-loaded path with different state than a first-load.
//
// Value defs (`do [def b 5 …]`) replay soundly (InstallDef re-push over the
// truncated binding) and keep baking. `undef` of a capitalised name is the
// mirror mutation (retires a minted type) and is equally hazardous.
// Graduation: the Phase 6 JIT detached-unit cache compiles these bodies as
// units instead of baking tokens, making the check-time install the only
// install.
func bodyHasReplayHazard(v core.Value) bool {
	var toks []core.Value
	switch d := v.Data.(type) {
	case core.ListPayload:
		toks = d.Elems
	case core.ParenExprPayload:
		toks = d.Toks
	default:
		return false
	}
	for i, t := range toks {
		if w, ok := t.Data.(core.WordInfo); ok {
			switch w.Name {
			case "import":
				return true
			case "def", "var", "undef":
				if i+1 < len(toks) && core.IsCapitalisedName(bindNameToken(toks[i+1])) {
					return true
				}
			}
		}
		if bodyHasReplayHazard(t) {
			return true
		}
	}
	return false
}

// bindNameToken extracts the name a def/var/undef token binds when its
// operand token is a bare word or a quoted atom; "" otherwise (computed
// names cannot statically install a type).
func bindNameToken(v core.Value) string {
	switch d := v.Data.(type) {
	case core.WordInfo:
		return d.Name
	case core.AtomPayload:
		return d.Name
	}
	return ""
}

// noEvalBodiesInertScoped is noEvalBodiesInert plus a MODULE-SCOPE allowance for
// InterpString-bearing bodies. A re-interpreting NoEvalArgs handler (the
// property harness's test-check-prop, Callable==nil) bakes its body as data and
// re-runs it in a sub-engine against the registry; an InterpString `${name}`
// there resolves against the registry exactly as the interpreter does — SOUND
// only when there is no enclosing compiled fn frame (len(es.units)==1). Inside a
// fn frame a `${frame-local}` would resolve against the registry instead of the
// VM slot and diverge (`def f fn [[pfx] … check-prop … `${pfx}` …]`), so there
// the strict isInertConst path keeps the body refused → sound interpreter
// fallback. (The same fn-frame hazard already exists for a bare-Word/paren
// member; this widening deliberately does NOT extend it — it admits InterpString
// only where every name resolves through the registry.)
func (es *EmitState) noEvalBodiesInertScoped(sig *core.Signature, args []core.Value) bool {
	moduleScope := len(es.units) == 1
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		inert := core.IsInertConst(args[i])
		if !inert && moduleScope {
			inert = InterpBodyInert(args[i])
		}
		if !inert {
			return false
		}
		if check.BodyHasSentinel(args[i]) {
			return false
		}
		if bodyHasReplayHazard(args[i]) {
			return false
		}
	}
	return true
}

// dynInputsProven reports whether a dynamic-operand dispatch is nonetheless a
// PROVEN sig match — safe to bake a plain CALL_NATIVE despite the dynamic
// carriers — for a word that runs its bodies in ISOLATED CallBoru frames
// (CompileRunsBodyIsolated, i.e. Test.check-prop). The dynamic-input refusal
// defends against a WIDENED sig: a gradual-Any operand (Parent=Any) matched a
// concrete sig position only by the Any→anything rule, so the runtime value
// could violate it and the compiled CALL_NATIVE would not raise where the
// interpreter's re-match would. That widening is visible as a bare-Any carrier.
// A dynamic operand whose CONCRETE Parent type strictly conforms to its sig
// position matched by REAL type membership, not widening — it is a runtime-
// valued value of a statically PROVEN type (e.g. `p get "runs"` over a
// `p:PropertySpec` param, an Integer guaranteed by the record contract the VM's
// guarded CALL_USER enforces at run-property's entry). So the runtime value
// conforms, the VM invokes the same single sig the interpreter matches, and the
// bodies run through the same captured-parent handler in both modes. The gate is
// deliberately strict: EVERY operand must conform by a non-Any concrete type, so
// a single genuinely-widened (Any) operand keeps the whole call refused.
func (es *EmitState) DynInputsProven(sig *core.Signature, args []core.Value) bool {
	if sig == nil || !sig.CompileEffect.Has(core.CompileRunsBodyIsolated) {
		return false
	}
	sigArgs := sig.ArgTypes()
	if len(sigArgs) != len(args) {
		return false
	}
	for i, a := range args {
		if !a.Dynamic {
			continue
		}
		// A widened gradual-Any operand: Parent conforms to nothing concrete,
		// so this is exactly the unproven case the guard defends — refuse.
		if a.Parent == nil || a.Parent.Equal(core.TAny) {
			return false
		}
		pt := sigArgs[i]
		if pt == nil {
			return false
		}
		if !a.Parent.ConformsTo(pt) {
			return false
		}
	}
	return true
}

// InterpBodyInert is isInertConst widened so a NESTED InterpString counts as
// inert. Its TOP-LEVEL semantics match isInertConst EXACTLY — a standalone
// ParenExpr / Word / InterpString is NOT bakeable data (a standalone ParenExpr
// is deferred code; baking `(loopy 1)` would wrongly let a nested too-deep
// macroexpand compile) — so only a List/Map at the top recurses through the
// InterpString-admitting member check; everything else defers to isInertConst.
// Only noEvalBodiesInertScoped (and resolveOperand) call this, and only at
// MODULE scope — an InterpString must NEVER bake inside a compiled fn frame.
func InterpBodyInert(v core.Value) bool {
	switch v.Data.(type) {
	case core.ListPayload, core.MapPayload:
		return InterpMemberInert(v)
	default:
		return core.IsInertConst(v)
	}
}

// InterpMemberInert is isInertConstMember widened to admit an InterpString whose
// hole expressions are themselves inert members — immutable code-as-data the VM
// pushes verbatim and a re-interpreting handler runs against the registry.
// Container members (List/Map/ParenExpr) recurse HERE so a nested InterpString
// at any depth is reached; every other leaf defers to the strict
// isInertConstMember so leaf soundness (mutable-instance exclusion, canonical
// type pointers) stays single-sourced.
func InterpMemberInert(v core.Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	if core.IsInterpString(v) {
		parts, err := core.AsInterpString(v)
		if err != nil {
			return false
		}
		for _, p := range parts {
			for _, tk := range p.Expr {
				if !InterpMemberInert(tk) {
					return false
				}
			}
		}
		return true
	}
	switch d := v.Data.(type) {
	case core.ListPayload:
		for _, e := range d.Elems {
			if !InterpMemberInert(e) {
				return false
			}
		}
		return true
	case core.MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !InterpMemberInert(mv) {
				return false
			}
		}
		return true
	case core.ParenExprPayload:
		for _, tk := range d.Toks {
			if !InterpMemberInert(tk) {
				return false
			}
		}
		return true
	default:
		return core.IsInertConstMember(v)
	}
}

// execBodyRefsNames reports whether a body-EXECUTING word's (sig.Callable != nil)
// inert NoEvalArgs body references a NAME — any Word or Reach token, at any depth.
// Such a token is resolved at run time: if the const-baked body is re-run in a
// sub-engine, the name resolves against the registry, but the compiled context may
// hold it as a VM frame local (fn param/capture, `for` iterator, promoted
// value-`def`) — so the re-run diverges. A body of pure inert DATA (scalars, data
// lists/maps — `do [10 20 30]`) references nothing and re-runs identically. Found
// by the property fuzzer's var-block closure bodies.
func execBodyRefsNames(sig *core.Signature, args []core.Value) bool {
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		if valueRefsName(args[i]) {
			return true
		}
	}
	return false
}

// valueRefsName recursively reports whether v contains a Word or Reach token —
// a name that resolves to a binding at run time — anywhere in its structure
// (list elements, map values, paren-expr tokens). A Splice marker also wraps a
// name-bearing payload, so it counts. Conservative: an unknown payload that
// could carry a name is NOT data, so only the known pure-data shapes return
// false.
func valueRefsName(v core.Value) bool {
	if core.IsWord(v) || core.IsReach(v) || core.IsSplice(v) {
		return true
	}
	switch d := v.Data.(type) {
	case core.ListPayload:
		for _, e := range d.Elems {
			if valueRefsName(e) {
				return true
			}
		}
	case core.MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if valueRefsName(mv) {
				return true
			}
		}
	case core.ParenExprPayload:
		for _, tk := range d.Toks {
			if valueRefsName(tk) {
				return true
			}
		}
	}
	return false
}

// internType pools a type operand by canonical ID.
func (es *EmitState) internType(v core.Value) int {
	if i, ok := es.typeIdx[v.ID]; ok {
		return i
	}
	es.types = append(es.types, TypeRef{Name: v.String(), ID: v.ID})
	es.typeIdx[v.ID] = len(es.types) - 1
	return len(es.types) - 1
}

// internUnpooled appends a const WITHOUT registering it in the canonical
// pool: a dynamic-scope bind's value must never merge with (or be merged
// into by) a source literal of the same canon — the bind may carry a
// reparented tag the literal must not inherit. Lowering-time only.
func (es *EmitState) internUnpooled(v core.Value) int {
	es.consts = append(es.consts, v)
	return len(es.consts) - 1
}

// intern pools a constant by canonical form. Compounds (lists,
// maps) are NEVER pooled: `eq` on compounds compares by identity
// (compare-restrict), so two source literals must stay two constants
// with their two distinct IDs — pooling them made `[1] eq [1]` true
// under the VM where the interpreter says false (the report's
// gotcha #13, caught by the differential gate).
// constPoolKey is the canon pool's key, and it carries the value's TYPE as
// well as its rendering.
//
// core.CanonValue renders a value through its nearest base-type arm
// (core/go/canon.go — a Pos that refines Integer renders as the digits), so
// canon ALONE merges values whose only difference is their nominal type. That
// is wrong for a const pool: the survivor donates its Parent to every other
// site that shares the rendering, and `def Pos (refine Integer)  def x:Pos 42
// typeof 42  typeof x` answered `Integer Integer` compiled against the
// interpreter's `Integer Pos`.
//
// Keying on the type ID as well keeps a refinement, a newtype and their base
// in separate slots. It costs nothing for the overwhelmingly common case:
// every plain Integer literal shares one Parent, so it shares one prefix.
// Parent is dereferenced unguarded here for the same reason intern's own
// identity gate does it two branches above (`v.Parent.Equal(core.TList)`):
// any value reaching this line has already been through that test, so a nil
// Parent would have failed there first. A guard here would be an unreachable
// branch carrying a pragma to say so.
func constPoolKey(v core.Value) string {
	return v.Parent.ID + "\x00" + core.CanonValue(v)
}

func (es *EmitState) intern(v core.Value) int {
	if _, isFn := v.Data.(core.FnDefInfo); isFn {
		// A fn value (introspection operand): never pool — CanonValue is not a
		// reliable identity key for fn bodies, so dedup could merge distinct
		// fns. Each bakes as its own const.
		es.consts = append(es.consts, v)
		return len(es.consts) - 1
	}
	identPayload := false
	switch v.Data.(type) {
	case core.ExtensionPayload, core.XmlElementPayload:
		identPayload = true
	}
	if identPayload || v.Parent.Equal(core.TList) || v.Parent.Equal(core.TMap) ||
		isTypeBodyPayload(v) || core.IsParenExpr(v) {
		// Compounds, structural type bodies, codequote'd ParenExprs, and
		// identity-bearing instance payloads (an Xml element literal, an
		// Extension-backed host container) are never CanonValue-deduped:
		// like the list/map identity rule, two source literals stay two
		// distinct const values rather than merging into one (`(<a/>) eq
		// (<a/>)` is false — eq is container identity — but a canon-pooled
		// merge made the compiled run push ONE instance twice and answer
		// true).
		// The SAME materialised value (same non-empty ID) is one logical
		// instance though — its payload pointer is already identity-aliased —
		// so it pools by ID: semantics-preserving, and it keeps
		// freshenFnUnitConsts' push-site counting truthful (two reads of one
		// binding count as pushes of ONE const, never two slots).
		if v.ID != "" {
			if i, ok := es.constIDIdx[v.ID]; ok {
				return i
			}
			es.consts = append(es.consts, v)
			if es.constIDIdx == nil {
				es.constIDIdx = map[string]int{}
			}
			es.constIDIdx[v.ID] = len(es.consts) - 1
			return len(es.consts) - 1
		}
		es.consts = append(es.consts, v)
		return len(es.consts) - 1
	}
	key := constPoolKey(v)
	if i, ok := es.constIdx[key]; ok {
		return i
	}
	es.consts = append(es.consts, v)
	es.constIdx[key] = len(es.consts) - 1
	return len(es.consts) - 1
}

// isTypeBodyPayload reports a structural type-body payload — pooled
// without dedup, like compounds (identity must not merge).
func isTypeBodyPayload(v core.Value) bool {
	switch v.Data.(type) {
	case core.RecordTypeInfo, core.OptionsTypeInfo, core.ChildTypeInfo, core.DisjunctInfo:
		return true
	}
	return false
}

// isInertConst reports whether v can live in a Program's constant
// pool: a fully concrete value whose payload is PLAIN DATA. The rule
// is a whitelist — scalars, temporal values, structural type bodies,
// and lists/maps of the same — because anything else is
// engine-coupled in some way: a check-mode carrier must not be
// materialised; structural tokens (Word, Splice, Reach, ParenExpr)
// would be expanded or re-stepped by the engine; bare type NODES go
// stale by value-copy against the canonical pointer (eng/go/
// CLAUDE.md, Canonical *Type Pointers — they route through
// OpPushType instead); class/surface bodies can embed method
// fn-values; function values are fn-value call sites (Stage 4 F4).
//
// MUTATION-SAFETY (load-bearing): a pooled const is pushed by the SAME
// Value — and thus the same pointer-backed payload — on every execution,
// including each iteration of a loop, so it must never reach a word that
// mutates it in place. That holds because the whitelist admits only
// immutable shapes: the mutable containers (Array/Object/Store instances)
// are absent, so their constructors never produce an inert const. The
// in-place mutators (Array/Object/Store `set`) return 0 values and DO now
// compile (P5 multi-result lowering), but their receiver is always a
// computed event (the constructor) or a frame local — never a pooled const,
// precisely because those instance types are absent here. Map/List `set` is
// copy-returning. Keep this whitelist free of mutable instance types: adding
// one would let a pooled compound const reach an in-place mutator and corrupt
// it across iterations.
// freshenableConst reports whether a value belongs to the compound
// VALUE-literal class whose interpreter evaluation CONSTRUCTS a fresh
// instance per evaluation, making per-call container identity observable
// through `eq` (miscompile mechanism A,
// design/MISCOMPILE-HUNT-FINDINGS.0.md §A). That is ListPayload and
// MapPayload — sameContainer (compare.go) identifies them by backing array /
// *OrderedMap pointer, and CloneValue mints both fresh. Everything else
// stays shared: scalars and Microns compare by value; type bodies, fn
// values, and surfaces are identity-inert descriptors; XML literals are
// parse-time constants in the INTERPRETER too (the same element instance is
// spliced per call — probe-verified `(mk) eq (mk)` → true interpreted), so
// sharing the pooled const IS parity there. The empty-list case needs no
// exclusion: sameContainer treats all empty lists as the single empty list,
// so interp (fresh empties, eq true) and VM (cloned empty, eq true) agree.
// Consumed by the resolveOperand fn-unit marking (freshenConst) and the
// enclosing-binding snapshot (snapshotCompoundBindingIDs).
func freshenableConst(v core.Value) bool {
	if v.Carrier || v.Dynamic {
		return false
	}
	switch v.Data.(type) {
	case core.ListPayload, core.MapPayload:
		return true
	}
	return false
}

// planMarkWindow arms the mark-window island (L-DO part 2b) when the
// program residual has the shape: it opens an OpStackMark before the
// region-starting event (markBefore is read during lowerEvents) and latches
// markWindowSeq for the post-lowering disposition, which takes
// OpCallDynMixedFromMark ONLY where the residual would otherwise refuse.
// Declines when the chained variadic-statement-if machinery already planned
// marks — the chain leaves its mark OPEN across events and consumes the
// TOPMOST mark at its claiming branch, so an interleaved window mark would
// corrupt the LIFO pairing (the TestChainedVariadicIfCompiles regression the
// first wiring caused).
func (es *EmitState) planMarkWindow(lw *lowerer, residual []core.Value) {
	if len(lw.markBefore) > 0 {
		return
	}
	seq, ok := es.markWindowShape(residual, lw.promoted)
	if !ok {
		return
	}
	if lw.markBefore == nil {
		lw.markBefore = map[int]bool{}
	}
	lw.markBefore[seq] = true
	es.markWindowSeq = seq
}

// markWindowShape reports the REGION-ANCHOR event seq of a mark-window
// residual (plan Phase 5, L-DO part 2b), or (0,false): residual[0] is
// produced by a VARIADIC top-level event (a fallible do-catch's
// runtime-variable results, re-marked through strip-input hops), and EVERY
// residual entry is an unpromoted event result — live on the stack where its
// event left it, so once an OpStackMark opens before the region-STARTING
// event the whole residual IS stack[mark:] at run time and
// OpCallDynMixedFromMark re-steps it verbatim (dynamic and fn-valued entries
// included — auto-apply is exactly what the island reproduces). Order and
// completeness are enforced separately against the post-lowering sim stack
// (verifyMarkWindow). residual[0]'s producer IS the region start: it is the
// DEEPEST surviving entry, so any earlier variadic event in a strip-input
// chain either had its outs consumed away entirely (its survivors would sit
// beneath residual[0], contradiction) or still owns residual[0] itself — a
// later hop's result can never be the window's bottom. A producer outside
// the top-level frame declines: lowerEvents reads markBefore only over
// frames[0], so a fragment anchor would arm the window with no OpStackMark
// ever emitted and the VM would raise where the interpreter succeeds.
func (es *EmitState) markWindowShape(residual []core.Value, promoted map[int]int) (int, bool) {
	if len(residual) < 2 {
		return 0, false
	}
	pr, ok := es.producedBy[residual[0].ID]
	if !ok || !es.eventInfo[pr.seq].variadicResult {
		return 0, false
	}
	for _, rv := range residual {
		p2, ok := es.producedBy[rv.ID]
		if !ok || es.eventInfo[p2.seq].zeroOut {
			return 0, false // a non-event entry would need a re-push the window forbids
		}
		if _, isProm := promoted[p2.seq]; isProm {
			return 0, false // a promoted result was POPPED to its slot — not live in the window
		}
	}
	if es.topLevelEventBySeq(pr.seq) == nil {
		return 0, false // the producer lowers outside lowerEvents' markBefore reads
	}
	return pr.seq, true
}

// planRegionPrefix arms the INERT-PREFIX seating beneath a runtime-variadic
// REGION (NUR067's consuming half — `99 for 3 [i]`, `99 await {mode:'first'}
// [[]]`). Declines when another mark plan already owns the frame: one mark
// client per program, exactly as planMarkWindow declines to planVariadicClaims.
// excusePrefixRegion drops the region the PREFIX plan will seat from the
// force-promotion set, in place.
//
// A force-promoted region is unsound: its stores pop exactly nout values
// while the run's runtime length is a different number, which lowerCall
// refuses a stage later ("variadic result promoted to frame slots"). Left on
// the sim it stays a region and OpSeatBelowMark lifts the prefix beneath it
// without ever naming the length (the forty-seventh increment).
//
// Only THIS seq is excused, and only from forceOrder. A variadic region
// bound to a NAME (`def x (do …) x`) is promoted by its DYN-BIND source
// instead, which this does not touch — so those keep the earlier and more
// informative refusal rather than falling through to a later, vaguer one.
//
// Split out of Finalize rather than written inline because Finalize sits on
// the gocyclo ceiling: one more branch there is one branch too many.
func (es *EmitState) excusePrefixRegion(residual []core.Value, forceOrder map[int]bool) {
	if seq, ok := es.regionPrefixShape(residual); ok {
		delete(forceOrder, seq)
	}
}

func (es *EmitState) planRegionPrefix(lw *lowerer, residual []core.Value) {
	seq, ok := es.regionPrefixShape(residual)
	if !ok || len(lw.markBefore) > 0 {
		return
	}
	lw.markBefore = map[int]bool{seq: true}
	lw.regionPrefixSeq = seq
}

// regionPrefixShape reports the producing seq of a residual shaped
// [inert…, REGION] — a runtime-variadic region LAST, with every entry beneath
// it inert (a const or a type: no producing event at all). That is the one
// shape OpSeatBelowMark can close, and it is the shape the ordinary seating
// refuses as "call result above a literal": the prefix cannot be pushed after
// the region (it would land on top of the run) and cannot be indexed past it
// from the top (the run's length is a runtime value).
//
// The producer must be a TOP-LEVEL event, for planMarkWindow's reason:
// lowerEvents reads markBefore only over frames[0], so a fragment-resident
// anchor would arm the plan with no OpStackMark ever emitted and the VM would
// raise where the interpreter succeeds.
func (es *EmitState) regionPrefixShape(residual []core.Value) (int, bool) {
	if len(residual) < 2 {
		return 0, false
	}
	pr, ok := es.producedBy[residual[len(residual)-1].ID]
	if !ok {
		return 0, false
	}
	// variadicRegionEvent answers false for a nil event, and it is checked
	// FIRST so regionReadsTheStack is never handed one.
	ev := es.topLevelEventBySeq(pr.seq)
	if !es.variadicRegionEvent(ev) || regionReadsTheStack(ev) {
		return 0, false
	}
	// The region's RUN is the trailing entries the producer left, and there
	// may be more than one of them: a do-catch records nout seats for a run
	// whose runtime length is a different number (2 recorded, 2-or-4
	// delivered). Walk back over the contiguous run of THIS producer's
	// entries; everything beneath it must be inert (no producing event at
	// all), which is what makes the prefix pushable after the run.
	i := len(residual) - 1
	for i > 0 {
		p, isEvent := es.producedBy[residual[i-1].ID]
		if !isEvent || p.seq != pr.seq {
			break
		}
		i--
	}
	if i == 0 {
		return 0, false // the run IS the whole residual; no prefix to seat
	}
	for _, rv := range residual[:i] {
		if _, isEvent := es.producedBy[rv.ID]; isEvent {
			return 0, false
		}
	}
	return pr.seq, true
}

// regionReadsTheStack reports whether the region event ev takes an operand
// that is ALREADY on the simulated stack when its OpStackMark opens, and so
// sits BELOW the mark: a prior EVENT's result, or a CLOSURE whose captures
// may themselves be such results. The region would then pop from beneath its
// own mark (`def m {n:3}  99 for (m get "n") [i]` — the bound is a live `get`
// result), leaving the mark above the stack top and SEAT_BELOW_MARK with
// nothing coherent to lift. Const, local and type operands are all pushed
// AFTER the mark, so those keep the plan.
//
// Only the operands that can reach the ENCLOSING stack are walked, which is
// why this does not reuse forEachOperand: a loop's condOut / bodyOut are its
// FRAGMENTS' results, live on the fragment's own scope and never on this one,
// and treating them as stack reads would decline every value-producing loop
// (their bodyOut is an event operand whenever the body is computed —
// `99 for 2 [(1 add 2)]`).
//
// EVERY region-producing event kind must be represented here, and the switch
// below is the whole list. It read `ev.call.ops` and the loop's operands
// alone until 2026-09-10, which was complete for the two producers that then
// existed — a value-producing loop and await's winner, both evCall — and
// silently wrong the moment the forty-seventh and forty-eighth increments
// admitted two more (Codex found all three witnesses on PR #448, and all
// three were real):
//
//   - evCallUser (a variadic-returning fn). `def f fn [[n:Integer] [] [for n
//     [i]]] 9 f (1 add 2)` compiled the mark AFTER the `add` whose result the
//     call then popped from beneath it, and answered `0 9 1 2` for the
//     interpreter's `9 0 1 2`;
//   - evFallback (the island's own 0-or-more run). `def xs [1] [do [1 div
//     (xs 0 getr)] error [drop]]` opened the mark above the do-result the
//     FALLBACK's own input then consumed, and MAKE_LIST_TO_MARK collected the
//     empty run: `1 []` for the interpreter's `[1]`.
//
// A kind whose operands are not listed here does not "have none": it is
// unscreened. If a new region producer is added, add its operands.
func regionReadsTheStack(ev *EmitEvent) bool {
	var ops []EmitOperand
	switch ev.kind {
	case evLoop:
		ops = []EmitOperand{ev.loop.start, ev.loop.end, ev.loop.step}
		for _, c := range ev.loop.carried {
			ops = append(ops, c.init)
		}
	case evCallUser:
		ops = ev.uc.ops
	case evFallback:
		ops = ev.fb.ins
	default:
		ops = ev.call.ops
	}
	for _, op := range ops {
		if op.kind == opEvent || op.kind == opClosure {
			return true
		}
	}
	return false
}

// planRegionCollect arms the COLLECT of a runtime-variadic region into one
// List (NUR067's consuming half — `size [(for 3 [i])]`, `size [(await
// {mode:'any'} [[7 8]])]`). Declines when another mark plan already owns the
// frame: one mark client per program.
func (es *EmitState) planRegionCollect(lw *lowerer) {
	region, list, ok := es.regionCollectShape(es.frames[0])
	if !ok || len(lw.markBefore) > 0 {
		return
	}
	lw.markBefore = map[int]bool{region: true}
	lw.collectAtSeq = list
}

// regionCollectShape finds a top-level [REGION, list-literal-over-it] ADJACENT
// pair and returns the two seqs. Adjacency is the whole safety argument: the
// mark opens before the region's event, so everything above it at run time
// must be the region and nothing else, and no event runs between the two to
// leave a value there or consume one from beneath. A list literal with any
// other operand beside the region (`[9 (for 3 [i])]`, `[(for 3 [i]) 9]`)
// keeps refusing — its elements would need seating either side of a run whose
// length is a runtime value, which is the prefix problem OpSeatBelowMark
// solves only for the program residual.
func (es *EmitState) regionCollectShape(events []EmitEvent) (int, int, bool) {
	for i := 0; i+1 < len(events); i++ {
		ev, next := &events[i], &events[i+1]
		if !es.singleSlotRegion(ev) || regionReadsTheStack(ev) ||
			next.kind != evCall || !next.call.makeList || len(next.call.ops) != 1 ||
			next.call.ops[0].kind != opEvent || next.call.ops[0].idx != ev.seq {
			continue
		}
		return ev.seq, next.seq, true
	}
	return 0, 0, false
}

// singleSlotRegion reports whether ev's result is ONE recorded slot standing
// for a RUNTIME-VARIABLE count of values: a value-producing loop (`for` /
// `while` — RecordLoop's hasBodyOut arm), or a variadic REGION call
// (callVariadicRegion — await's winner-takes-all residual). A do-catch's
// variadicResult is NOT one: it records nout slots for a run whose runtime
// length is a different number. The COLLECT needs this narrower question —
// its shape is a list literal over exactly ONE recorded operand — where the
// PREFIX does not (variadicRegionEvent). A nil event (no top-level event
// with that seq) is not one either.
func (es *EmitState) singleSlotRegion(ev *EmitEvent) bool {
	if ev == nil {
		return false
	}
	f := es.eventInfo[ev.seq]
	return !f.zeroOut && !f.regionMayBeFn &&
		(f.variadicRegion || (ev.kind == evLoop && f.variadicResult))
}

// variadicRegionEvent is singleSlotRegion's question WITHOUT the single-slot
// half: does ev leave a run whose runtime length is not its recorded count?
// That admits the do-catch region too (nout recorded seats, 2-or-4 delivered
// by a branch-variant body; one caught Error on the raise path).
//
// The prefix seating can ask the looser question because OpSeatBelowMark
// never names the run's length — it lifts the top n values down to the mark,
// whatever lies between. The forty-seventh increment is exactly that
// observation: the plan's single-slot gate was inherited from the two
// producers that happened to exist, not from anything the mechanism needs.
func (es *EmitState) variadicRegionEvent(ev *EmitEvent) bool {
	if ev == nil {
		return false
	}
	f := es.eventInfo[ev.seq]
	return !f.zeroOut && !f.regionMayBeFn &&
		(f.variadicRegion || f.variadicResult)
}

// topLevelEventBySeq finds the top-level frame's event with the given seq
// (nil when the seq belongs to a nested fragment or unit).
func (es *EmitState) topLevelEventBySeq(seq int) *EmitEvent {
	for i := range es.frames[0] {
		if es.frames[0][i].seq == seq {
			return &es.frames[0][i]
		}
	}
	return nil
}

// resolveDynamicApply classifies the residual's fn-value-call boundary (report
// §9.1) and returns the residual (rotated for a trailing apply), the apply
// opcode to emit once the residual is on the stack (0 = none), and a refusal
// reason for an fn-value shape the static residual cannot reproduce.
//
// Handled: a dynamic value LEADING the residual with static args after it
// (`r.int 0 100`); a Function CARRIER leading it (the factory `(mk2 5)
// 10`); and a single dynamic / fn value TRAILING one static arg (`5 m.f`,
// `[..] r.one-of`) — rotated to [fn, arg] so the reconciliation lays it out
// like the leading boundary, with OpCallDynamicTrailing restoring the fn-on-top
// order if the value is not callable. Every other dynamic / fn-value-precedes-
// args shape, and any unconsumed fn-value carrier, refuses.
func (es *EmitState) resolveDynamicApply(lw *lowerer, residual []core.Value) ([]core.Value, Opcode, string) {
	// Leading dynamic value (statically Any — the checker cannot tell a Function
	// from data) with every following arg static. An ANNOTATED method-read
	// carrier is excluded: the statement-window model (method_shape.go) owns
	// its apply, and a decline there means the residual tail may cross the
	// value's statement boundary — see methodShapeAnnotated.
	applyDynamic := false
	// A VARIADIC REGION entry is a COUNT, not a value (NUR067): at run time it
	// stands for 0-or-MORE stack values, so there is no single entry for any
	// apply arm to classify — and the carrier is Dynamic by construction (it
	// must match optimistically for the soundness oracle), which is exactly
	// what the trailing-apply arm reads as "a fn value over one arg". Measured
	// before the guard: `99 await {mode:'first'} [[1 2 3]]` lowered to
	// CALL_DYNAMIC_TRAILING and answered [1 2 99 3] against the interpreter's
	// [99 1 2 3]. Decline the whole fn-value-call boundary and let the ordinary
	// residual seating rule instead — a lone region IS the residual and seats,
	// a region above an inert tail refuses "call result above a literal".
	//
	// It runs BEFORE the NUR121 hazard scan below (the forty-eighth
	// increment moved it there): that scan asks whether a fn-value LEAD had
	// its argument collected by a later dispatch, and a region is not a
	// lead — it is a count. Asked of one, the scan answered yes for the
	// zero-netting handler's 0-or-1 run and refused a program that has no
	// fn value in it at all.
	if es.residualHasVariadicRegion(residual) {
		return residual, 0, ""
	}
	// A fn-value lead a later dispatch collected past, ANYWHERE in the
	// residual (NUR121: `g x add 1` — the model's `add` took `x`, so the
	// residual's args are its RESULT, not the lead's; `do [(f 5) 2] drop` —
	// the model's drop took the 2 the frame's rewind would have applied the
	// lead to, leaving the lead alone), refuses before any arm can apply it;
	// a placed (lazy) lead is the arms' own business (hazardLead).
	for _, v := range residual {
		if es.hazardLead(v) {
			return residual, 0, "fn-value lead's argument was collected by a later dispatch (NUR121)"
		}
	}
	if len(residual) >= 2 && residual[0].Dynamic && !es.methodShapeAnnotated(residual[0].ID) &&
		!es.leadPlacedNotRead(residual[0]) && !es.callResultPlaced(residual[0]) {
		applyDynamic = !anyDynamicTail(residual)
	}
	// Leading Function CARRIER (the factory pattern: a returned closure now
	// on the stack) with no dynamic / fn value after it.
	//
	// NUR101, ruled 2026-08-26 — PLACE UNIFORMLY. This arm used to read "a
	// carrier always applies — the one non-applied shape (an inert `f/v`) is
	// a CONCRETE const, not a carrier — so the carrier bit resolves the
	// apply-vs-inert ambiguity". That premise was true only while the
	// interpreter's paren rewind RE-STEPPED a collapsed Function into a call.
	// It no longer does (core/go/engine.go, fnReturnPark): a paren places its
	// Function whatever else survived beside it, so a Function carrier
	// LEADING a program residual is placed data, not a pending call, and
	// applying it here would compile `((mk 1) 2)` to 3 where the interpreter
	// now answers `fn (Integer) 2`.
	//
	// The discriminator is PLACEMENT, and the mechanism already existed:
	// ParenPlacedFnIDs (core/go/check_state.go), whose own doc says it is
	// "read by the compiler's residual lowering, which must not lower a
	// placed lead as an apply — `(m dot f) 5` is two values; `m.f 5` still
	// applies". It was recorded only for member-fn reads; the park now
	// records every carrier it places, so this arm can ask it directly.
	//
	// A narrower gate on def-read provenance was tried first and OVER-REACHED:
	// it refused `c.op 5`, a member-read apply that ADR-011 lists among the
	// three explicit application forms (a bare name, `apply`, a member read)
	// and that the interpreter still applies. Placement is the question, not
	// how the lead was named.
	if !applyDynamic && len(residual) >= 2 && core.IsFnTypedCarrier(residual[0]) &&
		!es.leadPlacedNotRead(residual[0]) && !es.callResultPlaced(residual[0]) {
		applyDynamic = !anyFnOrDynamicTail(residual)
		// When the carrier's closure arity is statically recoverable (its
		// producer is a compiled factory fn returning one anonymous closure),
		// a tail-arg count that disagrees is a curried CHAIN (`((mk 1) 2) 3`)
		// or a partial apply — one OpCallDynamic would leak the intermediate
		// closure (miscompile mechanism E). Refuse; the interpreter applies
		// the chain faithfully.
		if applyDynamic {
			arity, known := es.producerReturnedClosureArity(residual[0].ID)
			if known && arity != len(residual)-1 {
				return residual, 0, "fn-value apply arity mismatch — curried chain or partial apply (Stage 3)"
			}
			// A READ-substituted lead (the fn-carrier side table: `def k
			// (FnUtil.const 7)  (k 99)` — the read carries NoteDefRead
			// provenance) is a WORD dispatch in the interpreter, collecting
			// its forward args inside the read's STATEMENT. With a claimed
			// shape (CheckState.FnShapes — the producing word's own arity
			// claim) that dispatch is modelled AT THE READ, over the
			// statement window (check's tryShapedFnReadArrival), and never
			// reaches this residual: the flattened residual cannot see a
			// `;` (`bigger 3 ; 5` would lower as `bigger 3 5`). With no
			// claim and no compiled-factory producer there is no shape to
			// model by — refuse; the interpreter owns it. (This refusal's
			// earlier note blamed OpCallDynamic's island for answering 99
			// where the interpreter answers 7; the island applies the value
			// exactly as the interpreter does, and the 99 was the VM
			// resolving the wrapper's LABEL `const` through the live
			// registry — eng's vmNativeApplicable, the thirty-fifth
			// increment.)
			if !known {
				if _, read := es.defReads[residual[0].ID]; read {
					return residual, 0, "def-bound computed fn apply (closure shape unknown — Stage 1)"
				}
			}
		}
	}
	// Leading BRANCH result that MAY be a fn (an arm is an fn value, so the
	// merge produced a value that is sometimes callable — `if c [99]
	// MathUtil.sqrt 16`) over static args: a runtime-conditional apply.
	// callDynamic applies the value when the branch produced the callable and
	// leaves [value, arg] when it produced the data alternative, so the single
	// OpCallDynamic is faithful on both runtime branches. (The merge widens the
	// fn arm's type to its lattice parent — Word — so the disjunct carrier no
	// longer reads as Function; the branch-event mayBeFn flag is the precise
	// signal the static residual type cannot recover.)
	if !applyDynamic && len(residual) >= 2 {
		if pr, ok := es.producedBy[residual[0].ID]; ok && es.eventInfo[pr.seq].mayBeFn {
			applyDynamic = !anyFnOrDynamicTail(residual)
		}
	}
	if applyDynamic {
		return residual, OpCallDynamic, ""
	}
	if rot, ok := es.trailingApply(lw, residual); ok {
		return rot, OpCallDynamicTrailing, ""
	}
	// MIXED boundary: a single dynamic / fn value INTERIOR to the residual with
	// static args both below and above it (`3 m.f 2`). The producing event was
	// promoted to a frame local (Finalize's mixedDynamicApplyShape gate), so the
	// residual now re-pushes in source order; OpCallDynamicMixed islands the whole
	// window verbatim. The promoted entry stays Dynamic in `residual`, so the
	// in-order ops loop maps it to its local and the window lays out unchanged.
	// A PARKED user-fn result (callResultPlaced) is placed data the
	// interpreter never re-steps; the verbatim island steps it LIVE and
	// applies it — `5 (mk 3) 7` answered `5 21` for the interpreter's `5 fn
	// 7`, pre-existing for a named fn value and reachable for a closure once
	// the sub-engine bridges those too (NUR124's payload axis) — so both
	// window arms decline it and the shape refuses.
	if i, ok := es.mixedDynamicApplyShape(residual); ok && !es.callResultPlaced(residual[i]) {
		return residual, OpCallDynamicMixed, ""
	}
	// TRAILING window (Stage M2b): the single dynamic / fn value is LAST over
	// ≥2 static args (`10 3 m.s/s` → residual [10, 3, wrapper]). The verbatim
	// window island reproduces the interpreter exactly — the fn value lands ON
	// TOP of an already-populated stack and dispatches under its OWN barrier
	// discipline (a stack-args wrapper is BarrierPos 0 and collects nothing
	// forward, so the trailing-1 rotation contract of OpCallDynamicTrailing /
	// OpCallDynTrailTop cannot express it), and a non-callable value simply
	// stays put. The producing event was promoted to a frame local (Finalize's
	// gate below), so the whole window re-pushes in source order. The 2-entry
	// trailing shape stays with trailingApply above — landed and pinned.
	if es.trailingWindowApplyShape(residual) && !es.callResultPlaced(residual[len(residual)-1]) {
		return residual, OpCallDynamicMixed, ""
	}
	// Unhandled: a dynamic value mid-residual, a fn value preceding args, or an
	// unconsumed fn-value carrier (a VM closure renders unlike the interpreter's
	// FnDefInfo). All refuse so the program falls back faithfully — UNLESS the
	// MARK-WINDOW island armed (L-DO part 2b): Finalize's pre-lowering probe
	// opened an OpStackMark before the region-starting event, so the whole
	// residual IS stack[mark:] at run time and OpCallDynMixedFromMark re-steps
	// it verbatim, auto-apply hazard included — the arm fires ONLY at these
	// refusal points, never on a shape the ordinary lowering handles. Also
	// dynamic whose static bound provably excludes every callable (the
	// sigTypeMatches not-disjoint rule against Function): such a value
	// can never auto-apply to the values above it, so both engines leave it
	// as data and the residual renders identically (a narrowed flex-shape
	// read — dynamic(FlexMap) — sitting under later statement results,
	// edge-containers-2.tsv:76).
	for i := 0; i+1 < len(residual); i++ {
		// …and a value the COLLAPSE recorded as placed, which is the same
		// argument from a recorded fact rather than a static type (the paren
		// re-step rule, design/PAREN-RESTEP-RULE.0.md). The rewind that would
		// have applied this value is the interpreter's own, and it declined:
		// one survivor, so the pointer stepped past. Nothing downstream
		// re-steps a program residual, so both engines leave it as data and
		// the residual renders identically — `(mk 1) 2` is `fn (Integer) 2`
		// on both lanes.
		//
		// Gated on NOT re-stepped, not merely on placed: an enclosing paren
		// undoes the placement one level out (`((mk 1) 2)` is 3), and that
		// shape is a real apply the machinery above owns.
		if es.placedNotReStepped(residual[i]) || es.callResultPlaced(residual[i]) {
			continue
		}
		if residual[i].Dynamic &&
			core.SigTypeMatches(residual[i], core.TFunction) {
			if es.markWindowSeq != 0 {
				return residual, OpCallDynMixedFromMark, ""
			}
			return residual, 0, "dynamic value precedes residual args (fn-value-call boundary)"
		}
		if core.IsFnValueResidual(residual[i]) {
			if es.markWindowSeq != 0 {
				return residual, OpCallDynMixedFromMark, ""
			}
			return residual, 0, "fn value precedes residual args (auto-dispatch boundary)"
		}
	}
	// An unconsumed fn-value CARRIER refuses on the stated grounds that "a VM
	// closure renders unlike the interpreter's FnDefInfo".
	//
	// Measured 2026-08-27 (Stage 3): for a carrier a user paren PLACED and no
	// enclosing paren re-stepped, that fear does not materialise — `(mk 1) 2`,
	// `(mk2 5) 10` and a bare `(mk 5)` all render byte-identically on the two
	// lanes. What the refusal was actually holding back is two OTHER hazards,
	// and both are excluded here rather than assumed away:
	//
	//   - a bare-NAME read of a def-bound placed closure must DISPATCH, not
	//     sit as data (ADR-011). Without the defReads exclusion
	//     `def mk … def f (mk 7)  f` compiled `[fn]` against the
	//     interpreter's `[7]` — a live divergence this relaxation introduced
	//     and the exclusion removes. Placement is a LAYOUT fact; a read that
	//     calls is a DISPATCH fact, and this loop needs both.
	//   - a captured closure baked as a CONST loses its closure state
	//     (TestEmitFnValueData). The placed-and-not-read carriers reaching
	//     here do not take that path.
	//
	// What remains genuinely blocked is §6.3's universal fn value, and the
	// residue is now visible rather than hidden behind a blanket refusal.
	for i := range residual {
		if core.IsFnTypedCarrier(residual[i]) {
			// A parked call result passes this RENDER gate only when the
			// compiler knows how the value renders — the callee returns a
			// compiled anonymous closure (callResultRenderKnown). A user fn
			// that returns a fn it was HANDED (`def app fn
			// [[g:Function][Function][g/v]]`) renders that value under the
			// param's name on the interpreter (`fn g(Integer)`), which the
			// raw runtime value cannot reproduce; that shape keeps the
			// refusal (NUR119).
			if (es.placedNotReStepped(residual[i]) ||
				(es.callResultPlaced(residual[i]) && es.callResultRenderKnown(residual[i]))) &&
				!es.isDefRead(residual[i]) {
				continue
			}
			return residual, 0, "unconsumed fn-value carrier in residual (closure render)"
		}
	}
	return residual, 0, ""
}

// anyDynamicTail reports whether any residual entry after the first is dynamic.
func anyDynamicTail(residual []core.Value) bool {
	for i := 1; i < len(residual); i++ {
		if residual[i].Dynamic {
			return true
		}
	}
	return false
}

// anyFnOrDynamicTail reports whether any residual entry after the first is a
// dynamic or fn value (so a leading carrier's args are not all static). An
// fn-SHAPE-typed carrier counts as an fn value here and in the shape
// detectors below — its runtime inhabitant is a function (IsFnTypedCarrier).
func anyFnOrDynamicTail(residual []core.Value) bool {
	for i := 1; i < len(residual); i++ {
		if residual[i].Dynamic || core.IsFnValueResidual(residual[i]) ||
			core.IsFnTypedCarrier(residual[i]) {
			return true
		}
	}
	return false
}

// trailingApply detects the trailing fn-value auto-apply shape (`5 m.f`,
// `[data] r.one-of`): the residual's LAST entry is a dynamic / fn value produced
// by an event on top of the simulated stack, and the SINGLE entry before it is
// the static value it auto-applies to. On a match it returns the residual
// ROTATED to [fn, arg] (so the reconciliation lays it out like the leading
// boundary) and true. Bounded to one arg: with >1 the island's forward
// collection would order them opposite to the interpreter's top-down stack
// collection.
func (es *EmitState) trailingApply(lw *lowerer, residual []core.Value) ([]core.Value, bool) {
	if len(residual) != 2 {
		return residual, false
	}
	fnv := residual[1]
	pr, isEvent := es.producedBy[fnv.ID]
	if !isEvent || pr.idx != 0 ||
		!(fnv.Dynamic || core.IsFnValueResidual(fnv) || core.IsFnTypedCarrier(fnv)) {
		return residual, false
	}
	if len(lw.vm) < 1 || lw.vm[len(lw.vm)-1].seq != pr.seq || lw.vm[len(lw.vm)-1].idx != 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return residual, false
	}
	arg := residual[0]
	if _, argIsEvent := es.producedBy[arg.ID]; argIsEvent || arg.Dynamic ||
		core.IsFnValueResidual(arg) || core.IsFnTypedCarrier(arg) {
		return residual, false
	}
	// The park rule, which every SIBLING arm of resolveDynamicApply already
	// consults: a user fn's single returned closure is PARKED where it lands,
	// so `5 (mk 3)` leaves `[5 fn (Integer)]` and this arm must not apply it.
	// Unless a trailing `apply` WORD dispatched it on purpose — `10 (mk2 5)
	// apply` parks identically and then applies — which is what appliedByWord
	// separates and the residual shape cannot (NUR124).
	if !es.appliedByWord[fnv.ID] && es.callResultPlaced(fnv) {
		return residual, false
	}
	return []core.Value{fnv, arg}, true
}

// trailingWindowApplyShape detects the TRAILING fn-value window (Stage M2b):
// EXACTLY one residual entry is a dynamic / fn value, it is the LAST entry with
// ≥2 entries below it, and it is event-produced (so Finalize can promote the
// producer to a frame local and re-push the whole window in source order for
// OpCallDynamicMixed). The 2-entry trailing shape is deliberately excluded —
// that is trailingApply's landed rotation contract (OpCallDynamicTrailing).
func (es *EmitState) trailingWindowApplyShape(residual []core.Value) bool {
	if len(residual) < 3 {
		return false
	}
	last := len(residual) - 1
	for i, rv := range residual {
		if (rv.Dynamic || core.IsFnValueResidual(rv) || core.IsFnTypedCarrier(rv)) != (i == last) {
			return false
		}
	}
	pr, ok := es.producedBy[residual[last].ID]
	return ok && pr.idx == 0
}

// mixedDynamicApplyShape detects the MIXED fn-value-call boundary: EXACTLY one
// residual entry is a dynamic / fn value, it sits STRICTLY INTERIOR (≥1 entry
// below it AND ≥1 above it), and it is event-produced (so the producing event
// can be promoted to a frame local and the residual re-pushed in source order).
// Returns the interior index. `3 m.f 2` → residual [3, m.f, 2], index 1. The
// before/after entries need not be checked here for materialisability — the ops
// loop refuses any that cannot resolve, falling back faithfully.
func (es *EmitState) mixedDynamicApplyShape(residual []core.Value) (int, bool) {
	if len(residual) < 3 {
		return 0, false
	}
	dynIdx := -1
	for i, rv := range residual {
		if rv.Dynamic || core.IsFnValueResidual(rv) || core.IsFnTypedCarrier(rv) {
			if dynIdx != -1 {
				return 0, false // more than one dynamic / fn value
			}
			dynIdx = i
		}
	}
	if dynIdx <= 0 || dynIdx >= len(residual)-1 {
		return 0, false // not strictly interior (leading / trailing are separate)
	}
	// An ANNOTATED method-read carrier declines the mixed island too: its
	// after-args are the statement-window model's territory, and the island's
	// forward collection would cross the value's statement End boundary
	// (`3 c.add 2 ; {a:1} …` — the island could bind the NEXT statement's map
	// as the method's 2-arg overload's field arg). See methodShapeAnnotated.
	if es.methodShapeAnnotated(residual[dynIdx].ID) {
		return 0, false
	}
	if pr, ok := es.producedBy[residual[dynIdx].ID]; !ok || pr.idx != 0 {
		return 0, false // not event-produced — cannot promote to a local
	}
	return dynIdx, true
}

// resolveResidualOperands resolves each program-residual value to its operand
// — skipping a 0-output statement guard's phantom None (zeroOut), re-pushing a
// promoted value-def local from its slot, and materialising a bare type node /
// inert const — for Finalize to hand to the shared seat primitive. A variadic
// loop result is allowed here (the program residual may absorb it), unlike a
// fn body. A non-empty reason refuses the program.
func (es *EmitState) resolveResidualOperands(lw *lowerer, residual []core.Value) ([]EmitOperand, string) {
	// A dynamic splice (§9.2b) reassigns its payload's provenance to the
	// spread event, so a re-read of the payload def AFTER the splice
	// (`def xs (range 1 3) def d word xs d xs`) surfaces the SAME payload
	// value twice in the residual — the spread and the re-read. The re-read
	// would resolve to the variadic spread instead of the original list, so
	// decline (PR #279 review): the interpreter owns the re-read shape.
	seenSplice := map[string]bool{}
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok && es.eventInfo[pr.seq].spliceDyn {
			if seenSplice[rv.ID] {
				return nil, "splice payload re-read after the spread (Stage 2)"
			}
			seenSplice[rv.ID] = true
		}
	}
	ops := make([]EmitOperand, 0, len(residual))
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok {
			if es.eventInfo[pr.seq].zeroOut {
				continue
			}
			if slot, isProm := lw.promoted[pr.seq]; isProm {
				// slot is the producer's BASE slot; output idx i lives at slot+i
				// (multi-output stack words store one slot per result).
				ops = append(ops, localOperand(slot+pr.idx))
				continue
			}
			ops = append(ops, EventOperand(pr.seq, pr.idx))
			continue
		}
		// A frame-0 local (a loop-carried def's slot — the post-loop read of a
		// module-scope rebind resolves the joined binding to its cell). Events
		// first, mirroring resolveOperand's precedence.
		if slot, okLoc := es.units[0].localByID[rv.ID]; okLoc {
			ops = append(ops, localOperand(slot))
			continue
		}
		if core.IsBareTypeNode(rv) && rv.ID != "" {
			ops = append(ops, typeOperand(es.internType(rv)))
			continue
		}
		lit, okLit := es.Materialise(rv)
		if !okLit {
			// An S5 first-value loop bind's READ has no event/local home by
			// construction — it re-resolves the live registry binding the
			// splice bind installed (dynScopeRescue's top-level arm).
			if op, okDyn := es.dynScopeRescue(rv); okDyn && es.residualReadStable(rv) {
				ops = append(ops, op)
				continue
			}
			return nil, "residual value of unknown provenance"
		}
		if !core.IsInertConst(lit) {
			// A MODULE-FAMILY residual (an import-bound namespace, a Module
			// descriptor bound by def) materialises as a map the const gate
			// refuses on purpose; it re-reads the live binding instead
			// (dynScopeRescue's module-family arm), sound while that binding
			// still holds the very instance the read saw — the residual
			// re-push runs at the END of the program, so a rebind in between
			// would surface the later value where the interpreter pushed the
			// earlier one.
			if core.IsModuleFamilyValue(rv) {
				if op, okDyn := es.dynScopeRescue(rv); okDyn && es.moduleResidualStable(rv) {
					ops = append(ops, op)
					continue
				}
			}
			return nil, "residual value not statically materialisable"
		}
		ops = append(ops, ConstOperand(es.intern(lit)))
	}
	return ops, ""
}

// moduleResidualStable reports whether the binding a module-family residual
// was read from STILL holds the same instance — the namespace facet or the
// boxed descriptor, compared by pointer, which is the identity `eq`/`deq`
// compare (NUR031). It is the residual-position twin of residualReadStable
// for reads whose value carries no ID to key a generation on (the namespace
// `import` binds is a stored Value, never minted).
func (es *EmitState) moduleResidualStable(v core.Value) bool {
	if es == nil || es.reg == nil {
		return false
	}
	name := v.DynFrom()
	if name == "" {
		name = es.defReads[v.ID]
	}
	if name == "" {
		return false
	}
	top, ok := es.reg.Defs.Top(name)
	if !ok || !core.IsModuleFamilyValue(top) {
		return false
	}
	if ns := core.ModuleNSOf(v); ns != nil {
		return core.ModuleNSOf(top) == ns
	}
	tv, okT := top.Data.(core.ExtensionPayload)
	vv, okV := v.Data.(core.ExtensionPayload)
	return okT && okV && tv.Body == vv.Body
}

// Finalize linearises the recorded events into a Program. residual
// is the check run's final carrier stack — the program's declared
// result: event-produced entries must match the simulated stack in
// order; literal entries may only sit above the last event-produced
// entry and are pushed at the end. ok=false (with reason) when the
// source was marked uncompilable or a shape is beyond the current
// stage's lowering.
// eventsBindDynScope reports whether any event in the slice (recursing into
// branch arms, list-form conditions, and loop bodies) records a def of a name
// in `names` — the unit then installs registry-visible bindings and must not
// TAIL-call (the interpreter keeps a frame's bindings live across the calls
// in its body tail; a tail replacement would tear them down early).
func eventsBindDynScope(events []EmitEvent, names map[string]bool) bool {
	if len(names) == 0 {
		return false
	}
	var frag func(f *EmitFragment) bool
	walk := func(evs []EmitEvent) bool {
		for i := range evs {
			ev := &evs[i]
			switch ev.kind {
			case evDynBind:
				if ev.dyn.bindsValue() && names[ev.dyn.name] {
					return true
				}
			case evBranch:
				if frag(ev.br.condFrag) || frag(ev.br.then) || frag(ev.br.els) {
					return true
				}
			case evLoop:
				if frag(ev.loop.cond) || frag(ev.loop.body) {
					return true
				}
			}
		}
		return false
	}
	frag = func(f *EmitFragment) bool {
		if f == nil {
			return false
		}
		return walk(f.events)
	}
	return walk(events)
}

// unitBindsDynScope reports whether a unit installs dynamic-scope bindings:
// a body-local def of a dyn-read name (evDynBind in its fragment tree), or a
// PARAM some fn reads dynamically.
func (es *EmitState) unitBindsDynScope(rec *fnUnitRec) bool {
	// A deopt unit binds every name (emitDynParamBinds, lowerDynBind) and
	// never tail-calls: its island runs under the frame's live bindings.
	if rec.deoptEnv {
		return true
	}
	// DynEnv mode: every unit with a NAMED param installs bindings at entry
	// (emitDynParamBinds widens to all names), so tail calls are disabled
	// exactly as for a targeted dyn-bound param.
	if es.dynEnv {
		for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
			if rec.locals[i] != "" {
				return true
			}
		}
	}
	if eventsBindDynScope(rec.frag.events, es.dynScopeNames) {
		return true
	}
	if len(es.dynScopeNames) == 0 {
		return false
	}
	for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
		if es.dynScopeNames[rec.locals[i]] {
			return true
		}
	}
	return false
}

// emitDynParamBinds installs a unit's dynamically-read PARAMS into r.Defs at
// frame entry (a callee reading the caller's `n` — recursion.tsv:72), exactly
// where the interpreter's InstallFrameBinding makes them visible; the frame's
// RET truncates them back.
func (es *EmitState) emitDynParamBinds(flw *lowerer, rec *fnUnitRec) {
	if len(es.dynScopeNames) == 0 && !es.dynEnv && !rec.deoptEnv {
		return
	}
	// A LAMBDA unit's islands read its CAPTURES, not the enclosing frame's
	// bindings: the lambda escapes its factory, so by apply time that frame
	// is gone (which is why planDeopts' seedParentDeopt route is the code-body
	// one). Its captures ride in slots nParams…nParams+nCaps-1, live for the
	// whole call, so binding them here is the same duty done from this unit's
	// own frame. Every other unit binds params only, exactly as before.
	n := rec.nParams
	if rec.lambdaDeopt {
		n += len(rec.caps)
	}
	for i := 0; i < n && i < len(rec.locals); i++ {
		// DynEnv mode: every NAMED param dyn-binds (the interpreter's
		// InstallFrameBinding makes all of them registry-visible; a dynamic
		// code body may read any); a deopt unit binds the params its
		// islands spell. Unnamed slots have no name to bind.
		if rec.locals[i] == "" || (!es.dynEnv && !rec.deoptNames[rec.locals[i]] && !es.dynScopeNames[rec.locals[i]]) {
			continue
		}
		flw.emit(OpPushLocal, i, rec.pos)
		flw.emit(OpBindDynScope, es.internUnpooled(core.NewString(rec.locals[i])), rec.pos)
		flw.vm = append(flw.vm, nonEventSlot)
		flw.note()
		flw.vm = flw.vm[:len(flw.vm)-1]
	}
}

// truncateAtTrap ends the root event stream at a recorded TERMINAL TRAP and
// returns the twin-table rows the regime's full-placement gate must not
// demand an op for, because they describe no runtime transition at all.
//
// The trap (a check-mode-suppressed runtime error compiled as an OpTrap)
// ends the program: the events up to and including it are kept — their side
// effects run first, exactly as in the interpreter — and everything after is
// dropped as unreachable. The twins that truncation drops are UNREACHABLE
// transitions, not lost ones: the check pass walks past a trap where
// execution would not, so it records twins for defs the interpreter never
// performs. With the installs rolled back and no op to replay them those
// bindings correctly do not exist at runtime, which is stricter than the
// keep-installs default — that strands the pass's post-trap install in the
// registry. Collected by twin INDEX, not by position: an adopted twin's
// event carries its body's position, not the trap's.
//
// Rows a SUPERSEDED analysis round recorded join the same set — see
// MultiRunBodyGuard. Returns nil when there is nothing to exempt.
func (es *EmitState) truncateAtTrap() map[int]bool {
	var exempt map[int]bool
	mark := func(i int) {
		if exempt == nil {
			exempt = map[int]bool{}
		}
		exempt[i] = true
	}
	for i := range es.supersededTwins {
		mark(i)
	}
	if es.trapAt == 0 {
		return exempt
	}
	kept := eventsThroughSeq(es.frames[0], es.trapAt)
	for _, ev := range es.frames[0][len(kept):] {
		if ev.kind == evBindTwin && ev.twin != nil {
			mark(ev.twin.idx)
		}
	}
	es.frames[0] = kept
	return exempt
}

// finalizeBlocked reports the reason Finalize cannot deliver a Program
// before it lays anything out: the pass marked the program uncompilable, or
// a pending `apply`-word application on the PROGRAM unit that no dispatch
// consumed (recordCallElided's produced-closure arm) — the interpreter's
// re-step found no matching arguments beneath the closure and left it as
// data, a shape nothing here models, so the program refuses rather than
// seat the closure unapplied. A fn unit's finish owns its own entries.
func (es *EmitState) finalizeBlocked() (string, bool) {
	if !es.Compilable {
		return es.Reason, true
	}
	if len(es.units[0].pendingApply) > 0 {
		return "apply of a produced closure the program never dispatched (no matching arguments beneath it)", true
	}
	return "", false
}

func (es *EmitState) Finalize(residual []core.Value) (*Program, string, bool) {
	if es == nil {
		return nil, "no emit state", false
	}
	if reason, blocked := es.finalizeBlocked(); blocked {
		return nil, reason, false
	}
	twinExempt := es.truncateAtTrap()
	if es.trapAt != 0 {
		residual = nil
	}
	p := &Program{DynEnv: es.dynEnv,
		// The twin table travels with the finalized Program verbatim (a copy,
		// so a later pass on the same EmitState cannot mutate a delivered
		// Program), with the rollback base the run restores before the
		// placed twins replay onto it (§6.5 — see the fields).
		BindTwins:       append([]core.BindTransition(nil), es.bindTwins...),
		BindTwinEntries: append([]core.DefEntry(nil), es.bindTwinEntries...),
		ReplayBase:      es.bindSnap,
		ReplayReg:       es.progReg}
	lw := &lowerer{es: es, p: p, code: &p.Code, debug: &p.Debug, closureRet: &p.ClosureRet, storeNames: &p.StoreNames, sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}}
	// Value-def locals: a top-level computed result referenced more than once
	// (counting the program residual) is promoted to a frame local so the
	// single-consume stack discipline holds. Count the residual references,
	// then plan + rewrite before lowering.
	var residualSeqs []int
	for _, rv := range residual {
		if pr, ok := es.producedBy[rv.ID]; ok && pr.idx == 0 {
			residualSeqs = append(residualSeqs, pr.seq)
		}
	}
	// Residual ordering. The reconciliation below seats event results in stack
	// order with literals/types as a trailing tail (on top), so it requires the
	// residual to be in event*-literal* order. A residual with an event ABOVE a
	// literal (`1 2 word [add] 10` → [literal, event]) refused as "call result
	// above a literal". When the residual is out of order — and it is NOT the
	// fn-value / dynamic auto-apply lead shape, which lays the residual out on the
	// stack for OpCallDynamic and must not be promoted — force EVERY residual
	// event to a frame local: a local re-pushes in any order, so the
	// reconciliation pushes the whole residual (locals + consts + types) in exact
	// order. The promotion only fires for an out-of-order residual, so the common
	// in-order case is untouched.
	// A residual that holds ANY fn value or dynamic value is the auto-apply
	// boundary's territory (a leading or TRAILING fn applied to its neighbours —
	// `5 m.f` → [5, fn], `[..] r.one-of`): reordering it would drop the apply and
	// diverge. Leave those to the fn-value / refusal logic below; only a residual
	// of pure resolvable data reorders.
	var forceOrder map[int]bool
	residualHasFnOrDynamic := false
	for _, rv := range residual {
		if rv.Dynamic || core.IsFnValueResidual(rv) {
			residualHasFnOrDynamic = true
			break
		}
	}
	if !residualHasFnOrDynamic {
		seenLiteral, outOfOrder := false, false
		for _, rv := range residual {
			pr, isEvent := es.producedBy[rv.ID]
			if isEvent && pr.idx == 0 && !es.eventInfo[pr.seq].zeroOut {
				if seenLiteral {
					outOfOrder = true
				}
			} else {
				seenLiteral = true
			}
		}
		if outOfOrder {
			forceOrder = make(map[int]bool, len(residualSeqs))
			for _, seq := range residualSeqs {
				forceOrder[seq] = true
			}
		}
	}
	// MIXED fn-value-call boundary (`3 m.f 2`) and its TRAILING-window sibling
	// (`10 3 m.s/s`, Stage M2b): the fn sits above a before-arg literal, so the
	// in-order reconciliation would refuse "result above a literal". Promote
	// EVERY residual event to a frame local so the whole window (before-args,
	// the fn, any after-args) re-pushes in source order, ready for
	// OpCallDynamicMixed to island it. (residualHasFnOrDynamic suppressed the
	// reorder block above; these are the fn-value shapes that DO reorder, since
	// the island consumes the window in source order, not the apply-on-top
	// layout.)
	_, mixedOK := es.mixedDynamicApplyShape(residual)
	if mixedOK || es.trailingWindowApplyShape(residual) {
		forceOrder = make(map[int]bool, len(residualSeqs))
		for _, seq := range residualSeqs {
			forceOrder[seq] = true
		}
	}
	es.excusePrefixRegion(residual, forceOrder)
	lw.promoted, lw.dead = es.planValueDefLocals(es.units[0], es.frames[0], residualSeqs, forceOrder)
	lw.bindConsumes = collectRootBindConsumes(es.frames[0], lw.dead)
	lw.markBefore, lw.variadicElse = planVariadicClaims(es.frames[0])
	// Mark-window plan (L-DO part 2b): see planMarkWindow.
	es.planMarkWindow(lw, residual)
	// Region-prefix plan (NUR067's consuming half): see planRegionPrefix.
	es.planRegionPrefix(lw, residual)
	// Region-collect plan (NUR067's consuming half): see planRegionCollect.
	es.planRegionCollect(lw)
	// Seed the lowerer's frame-local counter from the unit's planned locals;
	// spillSeat bumps it for spill temps. Written back below so Program.NumLocals
	// covers them.
	lw.numLocals = es.units[0].numLocals
	if reason := lw.lowerEvents(es.frames[0], 0); reason != "" {
		return nil, reason, false
	}
	// Residual reconciliation.
	lastPos := core.SrcPos{}
	if n := len(es.frames[0]); n > 0 {
		lastPos = eventPos(es.frames[0][n-1])
	}
	// Fn-value-call boundary (report §9.1): classify the residual for a runtime
	// fn-value apply (a leading dynamic / carrier value, or a trailing dynamic /
	// fn value over one arg), returning the apply opcode and refusing the
	// shapes the static residual cannot reproduce. dynOp is emitted after the
	// residual is laid out below.
	residual, dynOp, dynReason := es.resolveDynamicApply(lw, residual)
	if dynReason != "" {
		return nil, dynReason, false
	}
	ops, opsReason := es.resolveResidualOperands(lw, residual)
	if opsReason != "" {
		return nil, opsReason, false
	}
	// A trap-truncated program seats nothing: the residual was dropped above,
	// and results the kept event prefix leaves on the stack are legitimately
	// dangling — the interpreter aborts at this same point with those values
	// still on its stack (`5 inc apply`: inc's result is live when apply
	// raises), and OpTrap aborts before anything could read them. Only a
	// program that RUNS TO COMPLETION owes the residual seating discipline.
	if es.trapAt == 0 && dynOp == OpCallDynMixedFromMark {
		// The mark window re-pushes NOTHING — the residual must BE the lowered
		// stack, in place and in order; anything else declines to a refusal
		// (the shape would have refused before the window landed anyway).
		if reason := lw.verifyMarkWindow(ops); reason != "" {
			return nil, reason, false
		}
	}
	if es.trapAt == 0 && dynOp != OpCallDynMixedFromMark {
		if reason := lw.seatProgramResidual(ops, residual, lastPos); reason != "" {
			// Reachable: a dirty-stack prefix under a dynamic-apply residual
			// (the variation sweep's prefix-stack transform) seats a shape
			// this refuses — a genuine Stage-1 refusal path, not a fault arm.
			return nil, reason, false
		}
	}
	if dynOp != 0 {
		// The fn value (leading, or trailing rotated to the front) sits at the
		// base of the residual with its args above; apply it at run time. The
		// trailing op additionally restores the fn-on-top order if the value
		// turns out not to be callable (see resolveDynamicApply / callDynamic).
		// The MIXED op instead islands the WHOLE window (the fn is interior), so it
		// takes the full residual length, not len-1.
		arg := len(residual) - 1
		if dynOp == OpCallDynamicMixed {
			arg = len(residual)
		}
		if dynOp == OpCallDynMixedFromMark {
			arg = 0 // the mark is the boundary; the op takes no count
		}
		lw.emit(dynOp, arg, lastPos)
	}

	// Lower the compiled fn units. Tail positions are marked first so
	// the lowering emits TAIL_CALL_USER (frame replacement — the
	// language's tail-call guarantee carries into compiled mode).
	for _, rec := range es.fnRecs {
		if !rec.finished {
			if es.trapAt != 0 {
				// The program ends at a terminal top-level trap, and an
				// UNFINISHED unit is unreachable from the kept prefix: a
				// CALL_USER event only records against a unit whose body
				// compile already finished, so no event at or before the trap
				// can enter this one (the unit was opened by a `def` whose
				// body compile the trapping dispatch cut short — e.g. `{f}`
				// evaluating a 1-arg fn with no args). Emit a defensive trap
				// stub so unit indices stay aligned and any future reach of
				// the stub fails loudly instead of silently returning nothing.
				ti := len(p.Traps)
				p.Traps = append(p.Traps, TrapSpec{Code: "internal_error",
					Detail: "unreachable fn unit " + rec.name + " entered after terminal trap", Word: rec.name})
				p.Fns = append(p.Fns, CompiledFn{Name: rec.name,
					Code:  []Instr{{Op: OpTrap, Arg: int32(ti)}},
					Debug: []core.SrcPos{rec.pos}})
				continue
			}
			return nil, "fn " + rec.name + " was never compiled", false
		}
		// A unit finished BEFORE a later dispatch armed es.dynEnv planned its
		// value-def promotion un-widened; seat its dyn-bound computed sources
		// now so lowerDynBind can re-push them (no-op unless es.dynEnv).
		es.promoteLateDynBind(rec)
		diverged := fragDiverges(rec.frag)
		// A unit that installs dynamic-scope bindings — a body-local def of a
		// dyn-read name, or a PARAM some other fn reads dynamically — must not
		// TAIL-call: the interpreter keeps the frame's bindings live until the
		// body's cleanup tail runs AFTER the call returns, so the compiled
		// frame must survive the call too (plain CALL_USER; RET pops the
		// bindings at the same point the interpreter's __DC does).
		bindsDyn := es.unitBindsDynScope(rec)
		if !rec.generic && !bindsDyn && len(rec.outOps) == 1 && rec.outOps[0].kind == opEvent {
			// Tail marking is single-result only: a tail call's results
			// become the fn's results wholesale, so a multi-return tail
			// boundary needs no rewrite here (it stays a plain CALL_USER).
			// Generic instantiations stay out of tail marking too — the
			// interpreter's HasGen exclusion, mirrored (plan Stage 4).
			if !es.markTailCalls(rec.frag, &rec.outOps[0], true, rec.returns) {
				// Every path tail-called: the body diverges (a trailing
				// all-arms-tail branch isn't caught by fragDiverges, so
				// track it here) and emits no reachable RET.
				rec.outOps = nil
				diverged = true
			}
		}
		// NParams counts everything the call site pushes — declared
		// params AND hidden capture slots; the VM pops them into frame
		// locals uniformly.
		// Pad the slot→name table to NLocals; body-local iterator slots
		// (added during loop lowering) stay anonymous.
		names := make([]string, rec.numLoc)
		copy(names, rec.locals)
		cf := CompiledFn{Name: rec.name, NParams: rec.nParams + len(rec.caps), NArgs: rec.nParams, NCaptures: len(rec.caps), NUnnamed: rec.nUnnamed, NLocals: rec.numLoc, InShape: rec.inShape, Returns: rec.returns, ReturnPatterns: rec.returnPatterns, Params: rec.paramTypes, ParamPatterns: rec.paramPatterns, Decl: rec.decl, LocalNames: names, Render: rec.render, Lambda: rec.lambdaUnit}
		if rec.reg != nil && rec.reg != es.progReg {
			// Stamp the unit's dispatch registry ONLY for a FOREIGN sub-registry
			// (a `module [...]` preamble fn — decision.cond, repl-eval-line):
			// its body resolves module-private words there, exactly where the
			// interpreter's CallBoru runs it, so the VM must override curReg. An
			// ORDINARY top-level fn (rec.reg == the program registry) must NOT be
			// stamped: the VM run may be handed a DIFFERENT registry
			// (ForkConcurrent gives each concurrent execution its own fork), so
			// stamping the shared compile registry would (a) defeat that
			// isolation and (b) race — every fork's CALL_NATIVE would call
			// ensureInvoker on the one shared registry, mutating its Invoker
			// field concurrently (the -race gate TestCompiledConcurrencyRaceFree).
			// progReg is the FIRST-bound registry (BindRegistry), stable across
			// the pass unlike es.reg which re-binds on every module-body / island
			// sub-engine run. A foreign sub-registry stays the correct shared
			// pointer (module bodies are not fork-parallelised in the corpus);
			// an ordinary fn falls through to curReg == vc.r (the fork).
			cf.Reg = rec.reg
		}
		flw := &lowerer{es: es, p: p, code: &cf.Code, debug: &cf.Debug, closureRet: &cf.ClosureRet, storeNames: &cf.StoreNames, dynApplyName: &cf.DynApplyName, sigIdx: lw.sigIdx, variadic: map[int]bool{}, numLocals: rec.numLoc, promoted: rec.promoted, dead: rec.dead, bindConsumes: collectResidentBindConsumes(rec.frag.events, rec.dead), isFnUnit: true}
		seatUnitDeopts(flw, rec, &cf, diverged)
		es.emitDynParamBinds(flw, rec)
		// The apply-loop replay's unnamed-param re-pushes seat at UNIT START —
		// they must sit BELOW the loop's runtime value region, which exists only
		// once the loop has run. They are runtime-only frame bottoms outside the
		// sim's model (the RET trim owns them), so the sim resets after.
		for _, op := range rec.retPrefix {
			flw.pushOperand(op, rec.pos)
		}
		flw.vm = flw.vm[:0]
		// Where this unit's region descriptors start. The stamp-only recovery
		// below replaces the unit's whole body with a trap, so descriptors
		// lowerCall already appended for it describe code that no longer
		// exists; they would be unreachable rather than wrong, but the census
		// counts descriptors, and a table that carries entries for discarded
		// code is a table whose count means something else.
		regionFloor := len(p.Regions)
		if reason := flw.lowerEvents(rec.frag.events, rec.frag.startSeq); reason != "" {
			if rec.stampOnly {
				p.Regions = p.Regions[:regionFloor]
				// A stamp-only unit is unreachable from the program's code —
				// its only consumer is a fn value's compiled ref, which
				// dropStampRef then clears. Refusing the whole PROGRAM because
				// an optimisation could not lower is the wrong trade, and it is
				// the one this arm prevents: two corpus rows went from
				// compiling to "refused: fn storedfn$body: consumes loop
				// results" the day stampFnConst was written without it. Emit the
				// same defensive trap stub the unreachable-unit arm above uses,
				// so unit indices stay aligned and any future reach fails loudly.
				ti := len(p.Traps)
				p.Traps = append(p.Traps, TrapSpec{Code: "internal_error",
					Detail: "stamp-only fn unit " + rec.name + " entered after its lowering refused", Word: rec.name})
				p.Fns = append(p.Fns, CompiledFn{Name: rec.name,
					Code:  []Instr{{Op: OpTrap, Arg: int32(ti)}},
					Debug: []core.SrcPos{rec.pos}})
				es.dropStampRef(len(p.Fns) - 1)
				continue
			}
			return nil, "fn " + rec.name + ": " + reason, false
		}
		// spillSeat may have allocated frame-local temps during lowering; grow
		// NLocals (and the debug name table) so the VM frame holds them.
		if flw.numLocals > cf.NLocals {
			cf.NLocals = flw.numLocals
			for len(cf.LocalNames) < cf.NLocals {
				cf.LocalNames = append(cf.LocalNames, "")
			}
		}
		if !diverged {
			// The __RC unnamed-arg allowance, applied at LOWERING time: a
			// declared fn's residual bottoms that are (a) within the
			// NUnnamed window and (b) pure PARAM-LOCAL references are the
			// frame's unconsumed unnamed args — the interpreter's __RC
			// discards them, and since operands lower lazily they were
			// never emitted, so dropping them here is the trim with zero
			// runtime cost (the VM RET's NUnnamed trim remains as the
			// runtime backstop). Bottoms that are anything else keep the
			// full residual and let reconcileResults refuse as before.
			if len(rec.returns) > 0 && rec.nUnnamed > 0 && len(rec.outOps) > len(rec.returns) && !rec.retReplay {
				if extra := len(rec.outOps) - len(rec.returns); extra <= rec.nUnnamed {
					drop := 0
					for drop < extra && rec.outOps[drop].kind == opLocal && rec.outOps[drop].idx < rec.nParams {
						drop++
					}
					if drop == extra {
						rec.outOps = rec.outOps[extra:]
					}
				}
			}
			// Reconcile the body's N result operands with the simulated
			// stack and emit a RET. Event results must already sit on the
			// stack in order (they were left by their own events); inert
			// operands (const / local / type) are pushed as a trailing
			// tail above the last event result. This is the fn-unit mirror
			// of the program-residual reconciliation below.
			// A deopt point no event followed (a residual read the tail test
			// declined) runs before the residual is laid out.
			flw.emitDeoptsBefore(core.SrcPos{})
			if reason := flw.reconcileResults(rec.outOps, "fn "+rec.name, len(rec.returns) == 0 || rec.dynTrailArity > 0 || rec.dynFrameW > 0, rec.retReplay, rec.pos); reason != "" {
				return nil, reason, false
			}
			// A paren-bounded trailing fn-value apply body: outOps were seated as the
			// full [args…, fn] (fn on top); collapse them to the one applied value with
			// OpCallDynTrailTop before the RET (the captured/param fn auto-applies to
			// its args exactly as the interpreter's paren auto-dispatch).
			// No DynApplyName is seated here. Measured (cover-gate, 2026-09-06):
			// every body-tail dynTrail the corpus reaches carries the `apply`
			// flavour, so a name seated on this route would be dead code — the
			// paren body-tail form the seat was written for either refuses or
			// lowers through the EVENT route (lowerCall), which does seat it.
			if rec.dynTrailArity > 0 {
				op := OpCallDynTrailTop
				pos := rec.pos
				if rec.dynTrailApply {
					op = OpCallDynApplyTop // the `apply` word's unquote-then-apply
					if rec.dynTrailPos != (core.SrcPos{}) {
						pos = rec.dynTrailPos
					}
				}
				flw.emit(op, rec.dynTrailArity, pos)
			}
			// A whole-frame dynamic-apply replay: outOps seated the FULL residual
			// (frame re-push prefix included); replay the top dynFrameW token-region
			// entries against it and let the RET apply the RetReplay discipline.
			if rec.dynFrameW > 0 {
				seatDynFrameWords(&cf, len(cf.Code), rec.dynFrameWords)
				flw.emit(OpCallDynFrame, rec.dynFrameW, rec.pos)
			}
			cf.RetReplay = rec.retReplay
			stampDeoptRet(&cf, flw.emit(OpRet, 0, rec.pos))
		}
		// A fully diverging body (every path tail-calls) emits no RET —
		// control leaves via the callee's eventual RET.
		freshenFnUnitConsts(&cf, es, rec, p)
		p.Fns = append(p.Fns, cf)
	}

	lw.p.Consts = es.consts // interning may have grown during reconciliation
	lw.p.Types = es.types
	lw.p.Fallbacks = es.fallbacks
	lw.p.MaxStack = lw.maxDepth
	// AFTER the residual reconciliation, not before it: the residual's own
	// seating allocates spill temps too (seatResidualRebuild), and a count
	// written back before it left those locals outside the frame — every
	// STORE_LOCAL past NLocals then failed at run time and the program fell
	// back to the interpreter with no refusal reason to show for it.
	es.units[0].numLocals = lw.numLocals
	lw.p.NumLocals = es.units[0].numLocals
	// Back-stamp every stored-fn handler ref with the now-built *Program so a
	// callback invoked after this run returns (a serve-raw connection handler on
	// its own fork) can locate its unit. The refs are already reachable from the
	// baked consts (their *BoruImpl); the side-list keeps this O(refs) rather than
	// re-scanning Consts. Fns is fully assembled above, so every ref.Unit indexes
	// a valid entry.
	for _, ref := range es.storedFnRefs {
		if ref.poisoned {
			// A dep the body reads was undef'd/redefined after this ref was
			// created — the frozen unit is stale. Leave Prog nil so
			// InvokeCallback falls back to CallBoru (live resolution).
			continue
		}
		ref.Prog = lw.p
	}
	lw.p.storedFnRefs = es.storedFnRefs
	if es.armReadRefusal != "" {
		return nil, es.armReadRefusal, false
	}
	if !twinsFullyPlaced(lw.p, twinExempt) {
		return nil, "twin regime: a bind transition has no stream placement (a multi-run-body or post-trap twin), so the rollback would lose it", false
	}
	return lw.p, "", true
}

// twinsFullyPlaced is the regime's FULL-PLACEMENT gate, the strengthening
// of the ordered-subset invariant Finalize applies when TwinRegime is
// armed: with the installs rolled back before the run, every twin-table
// entry needs a placed OpBindTwin — recorded live at its own position,
// or adopted after a once-run defs-keeping body's closure record
// (AdoptBodyTwins) — to replay its transition. A twin with none — a
// multi-run body's, where no single op can stand for a per-element
// transition — is one the rollback would silently lose, so the program
// refuses to the interpreter. The scan counts REAL ops in the lowered
// code, table index by table index, never the recorder's intent flags:
// an op a rollback dropped refuses no matter what was once marked.
//
// exempt names the twins that describe no runtime transition at all, so
// demanding an op for them would refuse a program the regime models
// exactly right (Finalize builds the set): rows a TERMINAL TRAP truncated
// away — unreachable, because the interpreter raises at the trap and never
// performs them — and rows a SUPERSEDED analysis round recorded, which the
// surviving round's ops replace.
func twinsFullyPlaced(p *Program, exempt map[int]bool) bool {
	placed := make([]bool, len(p.BindTwins))
	mark := func(code []Instr) {
		for _, in := range code {
			if in.Op == OpBindTwin && int(in.Arg) < len(placed) {
				placed[in.Arg] = true
			}
			// An arm-resident op satisfies its twin the per-invocation way
			// (ResidentBindSpec.Twin — the install executes with the
			// runtime value inside the unit); the scan still counts only
			// REAL ops, so a stamp the lowering never emitted refuses.
			if in.Op == OpBindResident && int(in.Arg) < len(p.ResidentBinds) {
				if t := p.ResidentBinds[in.Arg].Twin; t >= 0 && t < len(placed) {
					placed[t] = true
				}
			}
		}
	}
	mark(p.Code)
	for i := range p.Fns {
		mark(p.Fns[i].Code)
	}
	for i, ok := range placed {
		if !ok && !exempt[i] {
			return false
		}
	}
	return true
}

// noteApplyLoopReplay classifies a fn-body residual holding a per-iteration
// APPLY-LOOP's variadic value region under an inert tail (`def acc 0 for 5 […
// (args.0 1)] acc` → residual [param re-pushes…, loop values…, acc]): the
// loop's runtime count is variable, so the RET takes the replay trim
// discipline (retReplay). The param prefix seats at UNIT START (below the
// loop's region — it cannot be re-pushed after the loop has run), the loop
// event stays the seated residual, and the inert tail pushes above it as
// usual. Returns ops with the prefix split off (or unchanged when the shape
// does not fit — the normal reconciliation then refuses as before).
func (es *EmitState) noteApplyLoopReplay(rec *fnUnitRec, ops []EmitOperand) []EmitOperand {
	j := -1
	for i, op := range ops {
		if op.kind == opEvent && es.eventInfo[op.idx].variadicResult && es.eventInfo[op.idx].applyLoop {
			j = i
			break
		}
	}
	if j < 0 || j > rec.nUnnamed {
		return ops
	}
	for i := 0; i < j; i++ {
		if ops[i].kind != opLocal || ops[i].idx >= rec.nParams {
			return ops
		}
	}
	for i := j + 1; i < len(ops); i++ {
		if ops[i].kind == opEvent {
			return ops
		}
	}
	rec.retPrefix = ops[:j]
	rec.retReplay = true
	return ops[j:]
}

// noteDynFrameReplay scans a count-mismatched fn-body residual for an
// unapplied runtime fn VALUE beyond the frame-bottom re-push window. An
// UNNAMED PARAM's entry re-push in that window is inert DATA in both engines
// (arguments-are-inert: the interpreter never auto-fires a frame argument),
// and the RET trim discards it (NUnnamed allowance) — never an unapplied
// apply. A Function value ABOVE the window was produced at the pointer (an
// args.N fold, a paren apply), where the interpreter's execFnDefLiteral rule
// fires by RUNTIME Name/arity — statically unknowable in the recorder, so the
// WHOLE frame residual is replayed at run time (OpCallDynFrame /
// dynFrameWindow), where that same rule decides. A DYNAMIC value (a bounded
// gradual carrier — a map get over Any, the fn-value-dispatch stylesheet
// idiom `[nd (rules get (Fmt.kind nd))]`) takes the same replay: whether it
// turns out to be a fn (the interpreter applies it) or plain data (both
// engines raise the count error at RET) is decided by the SAME runtime rule
// the replay re-steps under, so arming is faithful for both outcomes — where
// the previous compile-with-symmetric-RET-error assumption diverged the
// moment the runtime value was callable. Returns true when no such value
// exists or the replay window was seated; false keeps the refusal.
//
// The window must hold exactly ONE applicable value. The replay re-pushes
// VALUES — the source's paren structure is gone — so a window carrying TWO
// applicables is the chained forward apply `f (g x)` whose INNER group never
// collapsed in the check run: the flat re-step hands the outer fn the RAW
// inner fn where the interpreter hands it the inner APPLICATION's result
// (`compose` → compiled count-error, interpreted 14). One applicable is
// faithful by construction (the pinned stylesheet shapes: every other window
// value is already the evaluated form its source group collapses to).
func (es *EmitState) noteDynFrameReplay(u *emitUnit, rec *fnUnitRec, vals []core.Value, extra int) bool {
	for i, v := range vals {
		if i < extra && i < rec.nUnnamed {
			if slot, isLocal := u.localByID[v.ID]; isLocal && slot < rec.nParams {
				continue
			}
		}
		// A user call's returned closure is PARKED where it lands — the frame
		// never re-steps it (callResultPlaced) — so it is not a possible
		// unapplied call: the count mismatch is genuine, and the RET raises
		// the interpreter's type_error over the same two values.
		if es.callResultPlacedIn(v, rec.frag) {
			continue
		}
		if v.Dynamic || (v.Parent != nil && v.Parent.ConformsTo(core.TFunction)) {
			// (A lead a later dispatch collected past — `[g x add 1]`, whose
			// replay would run over add's result, NUR121 — never reaches here:
			// residualStands refused it while resolving the residual above.)
			// The window holds exactly ONE applicable the re-step could
			// apply — the lead — where a GRADUAL WORD-READ entry beside a
			// FN-TYPED lead does not count (a gradual `x:Any` param beside
			// a captured `g` in `[(g x)]`, the twenty-sixth increment): such
			// an entry re-steps as the interpreter's own word dispatch,
			// faithful whether it holds a fn or data. A second FN-TYPED
			// entry still declines, word read or not — `f (g x y)`
			// collapses to no event and its window holds both f and g,
			// which the value replay flattens (the compiled lane raised
			// `cannot call `f`` for the interpreter's 14 before this was
			// measured) — and so does a gradual EVENT result beside the
			// lead, whose runtime fn the replay would apply under value
			// semantics. With no fn-typed value the count is the original
			// one-applicable rule (replayLeadApplicables).
			if w, ok := dynFrameWindow(u, rec, vals); ok && es.replayIsBodyTail(rec.frag, vals[len(vals)-w:]) &&
				replayLeadApplicables(vals[len(vals)-w:], es.dynFrameWordsFor(u, rec, vals[len(vals)-w:])) == 1 {
				rec.dynFrameW = w
				rec.retReplay = true
				// A word-read lead in the window takes the interpreter's WORD
				// dispatch in the replay (NUR123): `[g x]` over a 0-arg g
				// fires it, over a mismatched g raises `cannot call `g``.
				rec.dynFrameWords = es.dynFrameWordsFor(u, rec, vals[len(vals)-w:])
				return true
			}
			return false
		}
	}
	return true
}

// NoteWordRead counts a bare read of a fn-typed or gradual frame local on
// the innermost open unit (EmitRecorder). A read of a value that is not a
// local of that unit — a module-scope def, an enclosing unit's binding read
// through a closure body — is not this unit's to seat and is not counted:
// the closure paths refuse fn-typed carriers on their own gates.
func (es *EmitState) NoteWordRead(v core.Value, name string, pos core.SrcPos) {
	if !es.Active() || v.ID == "" || name == "" || len(es.openUnitRecs) == 0 {
		return
	}
	u := es.units[len(es.units)-1]
	if _, isLocal := u.localByID[v.ID]; !isLocal {
		// A body-local `def` bound to a value an event of THIS unit
		// produced (`def j (m get "f")  j`) resolves through its producing
		// event, not a slot — resolveOperand's events-first rule — and is
		// this unit's read all the same; an ENCLOSING-scope binding's
		// value (enclosingBindIDs, resolveOperand's own test) is not.
		if _, produced := es.producedBy[v.ID]; !produced || u.enclosingBindIDs[v.ID] {
			return
		}
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if rec.wordReadNames == nil {
		rec.wordReadNames = map[string]string{}
		rec.wordReadPos = map[string]core.SrcPos{}
	}
	rec.wordReadNames[v.ID] = name
	rec.wordReadPos[v.ID] = pos
	if rec.wordReadFirst == nil {
		rec.wordReadFirst = map[string]core.SrcPos{}
	}
	if _, seen := rec.wordReadFirst[v.ID]; !seen {
		rec.wordReadFirst[v.ID] = pos
	}
	// Only a FN-TYPED read is accounted strictly: the interpreter
	// dispatches it whatever the call passed. A GRADUAL read (an `x:Any`
	// param, a Dynamic local) dispatches only when the runtime value is a
	// fn; its residual read arms the replay best-effort, and its other
	// consumptions keep the slot push — refusing them would refuse every
	// `m k get` over an Any param (measured: the corpus's own fn bodies).
	if core.IsFnTypedCarrier(v) {
		if rec.wordReads == nil {
			rec.wordReads = map[string]int{}
		}
		rec.wordReads[v.ID]++
	}
}

// NoteLocalRead records a bare read's position per value ID on the
// innermost open unit (EmitRecorder) — planDeopts' deferred-operand
// accounting: a read written before a deopt's statement whose consumer
// comes after it is a value the interpreter's stack holds at the
// statement and the compiled stack does not yet.
func (es *EmitState) NoteLocalRead(id string, pos core.SrcPos) {
	if !es.Active() || id == "" || pos.Row == 0 || len(es.openUnitRecs) == 0 {
		return
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if rec.localReads == nil {
		rec.localReads = map[string][]core.SrcPos{}
	}
	rec.localReads[id] = append(rec.localReads[id], pos)
}

// NoteValRead counts a `/v` read on the innermost open unit (EmitRecorder).
func (es *EmitState) NoteValRead(id, name string) {
	if !es.Active() || id == "" {
		return
	}
	es.aliasValRead(id, name)
	if len(es.openUnitRecs) == 0 {
		return
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if rec.valReads == nil {
		rec.valReads = map[string]int{}
	}
	rec.valReads[id]++
}

// ArmTailApply is the branch-arm twin of the unit finish's pending-apply
// arm (the thirty-fourth increment): an arm whose residual ends in a
// PENDING `apply`-word fn (`[ 1 k/v apply ]`, the CPS rows' then arm) with
// at least one value beneath it INSIDE the arm collapses to the one
// gradual value the apply nets — RecordDynApply over that window, recorded
// into the arm's still-open fragment, so the arm's lowering applies the fn
// where the interpreter's applyHandler does. The window is the arm's own:
// the arm frame seals the enclosing stack off on both lanes. A residual
// that is no such shape (no pending fn on top, or nothing beneath it in the
// arm — the interpreter would then apply over an empty stack and park)
// passes through unchanged and keeps the unit finish's verdict.
func (es *EmitState) ArmTailApply(stk []core.Value) []core.Value {
	if !es.Active() || len(stk) < 2 || len(es.units) == 0 {
		return stk
	}
	top := stk[len(stk)-1]
	if !es.applyPending(top.ID) {
		return stk
	}
	out := core.NewCarrier(core.TAny)
	out.Dynamic = true
	if _, ok := es.RecordDynApply(stk[:len(stk)-1], top, out, core.SrcPos{}); !ok {
		return stk
	}
	return []core.Value{out}
}

// valBind is one unit-local dyn-bind of a produced fn value (emitUnit.valBinds).
type valBind struct {
	pr    producer
	gen   int64
	top   string // the installed entry's ID (Defs.Top at the bind)
	epoch int    // valBindEpoch[name] at the bind
	// lit is the bound CAPTURING fn LITERAL (a `def kk (fn …)` inside a
	// unit) when no event produced the value; capEpochs are its captures'
	// bind epochs at the bind (a later rebind of a captured fn-local would
	// make a read-site construction see the new value where the literal
	// snapshotted the old); op caches the closure operand the first read
	// built.
	lit       core.Value
	capEpochs map[string]int
	op        *EmitOperand
}

// noteValBind records, on the binding unit, a dyn-bind whose value is a
// fn value an event of this pass PRODUCED (a factory call's returned
// lambda, an apply's closure) or a capturing fn LITERAL: the binding kinds
// whose `/v` read re-wraps the value under a fresh ID. Every bind of the
// name drops the unit's entry first; a bind inside a branch or loop body
// serves the reads of its own arm and no later one — the arm's rollback
// moves the binding's generation and puts another entry (or none) on top,
// which aliasValRead checks — and a bind of any other value records
// nothing.
func (es *EmitState) noteValBind(cur *emitUnit, name string, v core.Value) {
	delete(cur.valBinds, name)
	if es.reg == nil || v.Parent == nil || !v.Parent.ConformsTo(core.TFunction) {
		return
	}
	b := valBind{gen: es.reg.Defs.Gen(name), epoch: es.valBindEpoch[name]}
	if pr, ok := es.producedBy[v.ID]; ok {
		b.pr = pr
	} else if fd, isFn := v.Data.(core.FnDefInfo); isFn && len(fd.Captured) > 0 && (fd.Anonymous || fd.Name == "") && !v.Quoted {
		// A capturing fn LITERAL bound by `def` (the thirty-third
		// increment): no event, no const — its `/v` read builds the closure
		// at the read site, so remember the literal and its captures'
		// epochs.
		b.lit = v
		b.capEpochs = map[string]int{}
		for _, cb := range fd.Captured {
			b.capEpochs[cb.Name] = es.valBindEpoch[cb.Name]
		}
	} else {
		return
	}
	top, _ := es.reg.Defs.Top(name)
	b.top = top.ID
	if cur.valBinds == nil {
		cur.valBinds = map[string]valBind{}
	}
	cur.valBinds[name] = b
}

// aliasValRead traces a `/v` read's fresh ID to the bound value's producer
// when the read is in the binding unit and the binding is still the
// bind's — no dyn-bind of the name recorded since, in any unit
// (valBindEpoch), and its generation unchanged or the bind's entry on top
// again after a frame binding of the name came and went — so the read
// resolves like the bound value: `def p (kk 7) end (w p/v)` hands the
// stored closure to w where the read used to refuse "fn call operand of
// unknown provenance". The alias is remembered (valReadIDs) for the
// argument-slot guard: the value spelling is data on both engines.
func (es *EmitState) aliasValRead(id, name string) {
	if name == "" || es.reg == nil || len(es.units) == 0 {
		return
	}
	cur := es.units[len(es.units)-1]
	b, ok := cur.valBinds[name]
	if !ok || es.valBindEpoch[name] != b.epoch {
		return
	}
	if es.reg.Defs.Gen(name) != b.gen {
		if top, found := es.reg.Defs.Top(name); !found || top.ID == "" || top.ID != b.top {
			return
		}
	}
	if b.lit.Data != nil {
		// A capturing literal: every captured name must be unbound since
		// the def (the literal snapshotted its captures), then the read is
		// the literal's closure operand, built once and cached on the bind.
		for cn, ep := range b.capEpochs {
			if es.valBindEpoch[cn] != ep {
				return
			}
		}
		if b.op == nil {
			op, ok := es.tryReturnedClosure(b.lit, b.lit.Pos())
			if !ok {
				return
			}
			// The value the interpreter reads is the binding's, named by
			// the def (`fn kk(Integer)`): the push carries the def name and
			// the VM names the closure as a store would.
			spec := ClosureRetSpec{DefName: name}
			if op.closureRet != nil {
				spec = *op.closureRet
				spec.DefName = name
			}
			op.closureRet = &spec
			b.op = &op
			cur.valBinds[name] = b
		}
		if es.readOps == nil {
			es.readOps = map[string]EmitOperand{}
		}
		es.readOps[id] = *b.op
	} else {
		es.producedBy[id] = b.pr
	}
	if es.valReadIDs == nil {
		es.valReadIDs = map[string]bool{}
	}
	es.valReadIDs[id] = true
}

// creditWordRead records that a fn-value apply lowering consumed one bare
// read of the local `id` under value semantics — the paren window
// (RecordDynApply) and the trailing apply (RegisterTrailingApply). Those
// are accepted lowerings (their 0-arg and no-match faces are NUR122's), so
// the read is accounted for.
func (es *EmitState) creditWordRead(id string) {
	if es == nil || id == "" || len(es.openUnitRecs) == 0 {
		return
	}
	rec := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
	if rec.wordReads[id] == 0 {
		return
	}
	if rec.wordReadCredit == nil {
		rec.wordReadCredit = map[string]int{}
	}
	rec.wordReadCredit[id]++
}

// fnResidualReplayReason is the fn-unit finish's replay verdict over a
// resolved residual, returning the refusal reason or "" — three arms behind
// one MarkUncompilable site. (1) A USER fn whose residual COUNT mismatches
// and carries a Function value, or a Dynamic value that may be one, is a
// possible unapplied fn-value call: noteDynFrameReplay arms the whole-frame
// replay, or the shape refuses (a mid-body apply failing the body-tail gate,
// an undecomposable window). (2) A bare read of a fn-valued binding left in
// the residual arms the replay whatever the count, or refuses when a
// fn-typed one cannot seat (NUR123). (3) Every fn-typed bare read the engine
// noted must be seated or credited (wordReadAccounting). Closure units take
// none of the three: their count mismatch is the higher-order word's own
// error and their reads are the enclosing frame's.
func (es *EmitState) fnResidualReplayReason(u *emitUnit, rec *fnUnitRec, vals []core.Value, ops []EmitOperand, dynTrail int) string {
	// The residual VALUES are recorded for EVERY unit, a closure included, and
	// BEFORE the closure early-return: they are what compileStoredBody reads to
	// decide whether a stored branch body can leave a CALLABLE
	// (eventFlags.regionMayBeFn), and a `spawnbody` unit IS a closure. Leaving
	// them unset there was silent — the callable test simply read an empty
	// slice and admitted the region, so `9 await {mode:'first'} [[g/v]]`
	// compiled to [9 fn] against the interpreter's [10]. The replay accounting
	// below is a plain-unit concern and still skips a closure.
	rec.outOpsVals = vals
	if rec.closure && !rec.plainLambda() {
		return ""
	}
	// A body-tail dynamic apply (dynTrail) owns the residual: the count and
	// replay arms do not apply, but the READ accounting still does — a bare
	// read consumed as a window ARGUMENT beneath the tail apply (`(x (n
	// f/v apply) apply)`, the numeral's bare `n`) is a word dispatch the
	// window lowered as data, and skipping the accounting here let it
	// compile to `f` applied to `n` for the interpreter's `n` applied to
	// `f` once the window admitted fn-valued args (the thirty-second
	// increment).
	if dynTrail == 0 {
		if len(rec.returns) > 0 && len(ops) != len(rec.returns) &&
			!es.noteDynFrameReplay(u, rec, vals, len(ops)-len(rec.returns)) {
			return "unapplied fn-value in body residual (dynamic apply not compiled in a fn body)"
		}
		if rec.dynFrameW == 0 && len(rec.returns) > 0 && !es.noteWordReadReplay(u, rec, vals) {
			return "bare read of a fn-valued binding is a word dispatch the frame replay cannot seat (NUR123)"
		}
	}
	return es.wordReadAccounting(rec)
}

// seatUnitDeopts hands a deopt unit's points (planDeopts) to its lowerer:
// the lowerer emits each where its statement begins, and the table and
// the body ride on the unit for the VM. A diverging body has no RET to
// resume at and seats none.
func seatUnitDeopts(flw *lowerer, rec *fnUnitRec, cf *CompiledFn, diverged bool) {
	if !rec.deoptEnv || diverged {
		return
	}
	flw.deoptNames = rec.deoptNames
	flw.deoptTable = &cf.Deopts
	cf.Body = rec.body
	// The interpreter's frame holds the unit's UNNAMED params on its stack
	// bottom (named ones are bindings); an island resuming mid-body seats
	// the ones this unit has not pushed yet beneath its region
	// (lowerer.deoptPrefix).
	for i := 0; i < rec.nParams && i < len(rec.locals); i++ {
		if rec.locals[i] == "" {
			flw.unnamedParams = append(flw.unnamedParams, i)
		}
	}
	for _, d := range rec.deopts {
		if d.restep {
			// Tested right after the event's op, over the results it left
			// (emitReStepAfter, NUR124).
			if flw.deoptAfterSeq == nil {
				flw.deoptAfterSeq = map[int]deoptPoint{}
			}
			flw.deoptAfterSeq[d.seq] = d
			continue
		}
		if !d.atPush {
			flw.deopts = append(flw.deopts, d)
			continue
		}
		// Tested where the read's value is pushed: a capture's own slot, or
		// the def's promoted source (a deopt unit promotes the sources its
		// islands read).
		slot := d.slot
		if slot < 0 {
			s, ok := rec.promoted[d.seq]
			if !ok {
				continue
			}
			slot = s
		}
		if flw.deoptAtSlot == nil {
			flw.deoptAtSlot = map[int]deoptPoint{}
		}
		flw.deoptAtSlot[slot] = d
	}
}

// stampDeoptRet seats the unit's RET pc on every deopt point the lowerer
// emitted (the island's residual continues there) and marks the RET's
// discipline runtime-variable (RetReplay).
func stampDeoptRet(cf *CompiledFn, retPC int) {
	for i := range cf.Deopts {
		cf.Deopts[i].RetPC = retPC
	}
	if len(cf.Deopts) > 0 {
		cf.RetReplay = true
	}
}

// planDeopts computes the unit's per-read deopt points (NUR123): a bare
// read of a BODY-LOCAL bound to a value an event of this unit produced —
// `def j (m get "f")  j typeof` — that the pass types dynamic(Any) and the
// model consumed as a value where the whole-frame replay cannot re-step it
// (a container member, an argument, a branch or loop body, a read an event
// follows). The interpreter dispatches the binding as a WORD when it holds
// a fn at run time. A fn-typed read is accounted strictly instead
// (wordReadAccounting) and a read a fn-value apply lowering credited needs
// none; a gradual PARAM's read is re-run under the argument's runtime type
// and stays best effort here (a value that is a slot, not a producer). Each
// point tests where the read's STATEMENT begins — the earliest of the first
// consuming event, that event's operand producers and the first read — so
// no effect of the statement runs twice (deoptPointFor). A start that is no
// Body token, a unit with no body and a closure unit keep the slot push.
func (es *EmitState) planDeopts(u *emitUnit, rec *fnUnitRec) {
	if len(rec.body) > 0 {
		es.planReStepDeopts(u, rec)
	}
	if len(rec.body) == 0 || (len(rec.wordReadNames) == 0 && len(rec.deopts) == 0) {
		es.planDeoptsEnv(u, rec)
		return
	}
	// A lambda value or a stored / spawned body escapes the frame its
	// bindings live in, so the code-body route below (seedParentDeopt — the
	// enclosing unit binds the captured names, valid because each / do /
	// fold / for run the body IN that frame) is unavailable to it. It plans
	// from its OWN frame instead: every name its islands spell must be one
	// of this unit's locals — a param or a CAPTURE, both live for the whole
	// call — or a def the island itself makes.
	lambda := rec.closure && (rec.lambdaUnit || rec.storedRefUnit)
	for id, name := range rec.wordReadNames {
		if rec.wordReads[id] > 0 || rec.wordReadCredit[id] > 0 {
			continue
		}
		seq, slot := -1, -1
		if u.capID[id] {
			slot = u.localByID[id]
		} else {
			pr, produced := es.producedBy[id]
			if !produced || u.enclosingBindIDs[id] {
				continue
			}
			seq = pr.seq
		}
		if d, ok := es.deoptPointFor(u, rec, id, seq, slot, name, rec.wordReadFirst[id]); ok {
			rec.deopts = append(rec.deopts, d)
		}
	}
	if len(rec.deopts) == 0 {
		es.planDeoptsEnv(u, rec)
		return
	}
	// The island reads the names the body's tail spells (from the earliest
	// statement on, code bodies and literals included): those params and
	// defs must be registry-visible at the deopt — this unit's own, or for
	// a closure's captures the enclosing unit's (seedParentDeopt). A def the
	// island reads whose source has no re-pushable home — a fragment
	// result, a variadic or multi-output producer, a def made inside a
	// branch or loop body — would refuse the whole unit at its bind
	// (lowerDynBind), so the points decline instead and the reads keep
	// their slot push.
	minTok := len(rec.body)
	for _, d := range rec.deopts {
		if d.token < minTok {
			minTok = d.token
		}
	}
	names := map[string]bool{}
	collectWordNames(rec.body[minTok:], names)
	for n := range rec.deoptNames {
		names[n] = true
	}
	ok := es.deoptDefsBindable(rec.frag.events, names)
	switch {
	case !ok:
	case lambda:
		// Planned from this unit's OWN frame: every name the islands spell
		// must be a local of this lambda (a param or a capture — both live
		// for the whole call, and emitDynParamBinds binds them), a def the
		// island itself makes, or a word the registry resolves. A name that
		// is none of those would have resolved from the FACTORY's frame,
		// which is gone by apply time, so the points decline.
		ok = es.lambdaNamesSelfBound(rec, names)
	case rec.closure:
		ok = es.seedParentDeopt(rec, names)
	}
	if !ok {
		rec.deopts = nil
		es.dropDeoptChildren(rec)
		return
	}
	rec.lambdaDeopt = lambda
	rec.deoptEnv = true
	rec.deoptNames = names
	u.deoptEnv = true
	u.deoptNames = names
	sort.Slice(rec.deopts, func(i, j int) bool { return posAfter(rec.deopts[j].start, rec.deopts[i].start) })
}

// planReStepDeopts plans the unit's RE-STEP points (NUR124). A root event
// whose results the check pass noted as re-stepped into a dispatch attempt
// (NoteFnResultReStep — a native's fn-typed or fn-admitting result with a
// plain body token written after it) gets a point right after it: the VM
// tests the results the op left on top and, when one would dispatch at the
// pointer, hands them as TAPE TOKENS followed by the body from the resume
// token to the interpreter over the frame region beneath them — the
// interpreter's own splice-and-step, so the fn collects forward over the
// written tokens and then from the stack, exactly where it does on that
// lane. A point declines when the resume position is no token of this body
// (the call sits inside a nested form) or the compiled stack at the point
// lacks an operand the interpreter's holds (deoptDeferred); a fn-TYPED
// note no point serves refuses at the lowering (emitReStepAfter), a
// gradual one keeps the model it always had.
func (es *EmitState) planReStepDeopts(u *emitUnit, rec *fnUnitRec) {
	if len(es.reStepNotes) == 0 {
		return
	}
	events := rec.frag.events
	for i := range events {
		ev := &events[i]
		note, noted := es.reStepNotes[ev.seq]
		if !noted {
			continue
		}
		tok := bodyTokenAt(rec.body, note.resume)
		if tok < 0 {
			continue
		}
		// The point's start is the RESUME token: everything the event's own
		// window wrote (its forward args) has been consumed by it on both
		// lanes, so the deferred-operand accounting runs from there.
		d := deoptPoint{seq: ev.seq, slot: -1, pos: eventPos(*ev), start: note.resume, token: tok, restep: true}
		if es.deoptDeferred(u, rec, &d, i) {
			continue
		}
		rec.deopts = append(rec.deopts, d)
	}
}

// lambdaNamesSelfBound reports whether a LAMBDA / stored-ref unit can serve
// its islands from its own frame. Such a unit escapes the frame its captured
// bindings came from — by apply time the factory's frame is gone — so
// seedParentDeopt's promise (the enclosing unit binds the names, valid while
// each / do / fold / for run the body IN that frame) does not hold. What DOES
// hold is the unit's own locals: params and captures both ride in slots for
// the whole call, and emitDynParamBinds makes them registry-visible under
// their names. So a name the islands spell is servable when it is one of
// those, a def the island's own tokens make, or a word the registry already
// resolves.
//
// So the ONE name that cannot be served is one an ENCLOSING unit supplies —
// its own def or param — and that this lambda did not capture: the island
// would read it out of a frame that no longer exists. Everything else the
// island spells resolves exactly as the interpreter resolves it there: this
// unit's own locals, a def the island's own tokens make, a native or module
// word, a top-level (global) def that outlives every frame, and a bare
// literal-word like `true`. That last one is why this is NOT a
// registry-membership test: it was one at first, and `if true [j] [0]` was
// the witness — `true` is spelled as a word, `Registry.Lookup` does not
// resolve it, and the whole point declined over a token that needs no
// binding at all.
func (es *EmitState) lambdaNamesSelfBound(rec *fnUnitRec, names map[string]bool) bool {
	own := map[string]bool{}
	for i, n := range rec.locals {
		if n != "" && i < rec.nParams+len(rec.caps) {
			own[n] = true
		}
	}
	for i := range rec.frag.events {
		if ev := &rec.frag.events[i]; ev.kind == evDynBind && ev.dyn != nil {
			own[ev.dyn.name] = true
		}
	}
	// The units still open around this one: their frame defs and their
	// params are exactly what dies with the factory's frame.
	for i := 0; i+1 < len(es.openUnitRecs); i++ {
		p := es.fnRecs[es.openUnitRecs[i]]
		for _, n := range p.locals {
			if n != "" && names[n] && !own[n] {
				return false
			}
		}
		if p.rootFrame < 0 || p.rootFrame >= len(es.frames) {
			continue
		}
		for k := range es.frames[p.rootFrame] {
			ev := &es.frames[p.rootFrame][k]
			if ev.kind == evDynBind && ev.dyn != nil && names[ev.dyn.name] && !own[ev.dyn.name] {
				return false
			}
		}
	}
	return true
}

// planDeoptsEnv keeps the environment a unit's closure children seeded on
// it (seedParentDeopt) when the unit plans no points of its own — the
// children's islands read its bindings — or drops the children's points
// when the unit cannot bind the names after all (a def of one made inside
// a later branch or loop body).
func (es *EmitState) planDeoptsEnv(u *emitUnit, rec *fnUnitRec) {
	if len(rec.deoptNames) == 0 {
		return
	}
	if !es.deoptDefsBindable(rec.frag.events, rec.deoptNames) {
		es.dropDeoptChildren(rec)
		return
	}
	// A LAMBDA seeded by its own closure child (`( fn [[x:Integer][Any][do
	// [j]]] )` — the do-body's island reads the lambda's capture `j`) binds
	// from its OWN frame, exactly as it does when it plans points itself:
	// the child runs INSIDE this lambda's call, so these slots are live, but
	// the factory's frame that `j` came from is not. Without the mark
	// emitDynParamBinds would bind the params only and the child's island
	// would not resolve the name.
	if rec.closure && (rec.lambdaUnit || rec.storedRefUnit) {
		if !es.lambdaNamesSelfBound(rec, rec.deoptNames) {
			es.dropDeoptChildren(rec)
			return
		}
		rec.lambdaDeopt = true
	}
	rec.deoptEnv = true
	u.deoptEnv = true
	u.deoptNames = rec.deoptNames
}

// dropDeoptChildren clears the points of every closure unit whose islands
// read this unit's bindings, with the unit's own seeded names.
func (es *EmitState) dropDeoptChildren(rec *fnUnitRec) {
	for _, c := range rec.deoptChildren {
		child := es.fnRecs[c]
		child.deopts, child.deoptEnv, child.deoptNames = nil, false, nil
	}
	rec.deoptNames, rec.deoptChildren = nil, nil
}

// seedParentDeopt asks the enclosing unit to bind, registry-visibly, the
// CAPTURED names a closure's islands spell: a code body runs inside the
// enclosing frame (each, do, fold, for call it there), so the parent's
// binds are live when the island runs. The parent's root frame is still
// open — its events so far decide whether each name can bind (a root-level
// def of a literal or a single-output call, or a param); a def of the
// name inside one of the parent's open nested frames (the arm the closure
// sits in) cannot. Reports false when the closure has no enclosing unit
// or a captured name cannot be bound.
func (es *EmitState) seedParentDeopt(rec *fnUnitRec, names map[string]bool) bool {
	captured := map[string]bool{}
	for i, n := range rec.locals {
		if i >= rec.nParams && n != "" && names[n] {
			captured[n] = true
		}
	}
	if len(captured) == 0 {
		return true
	}
	if len(es.openUnitRecs) < 2 {
		return false
	}
	parent := es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-2]]
	if parent.rootFrame < 0 || parent.rootFrame >= len(es.frames) || !es.deoptDefsBindable(es.frames[parent.rootFrame], captured) {
		return false
	}
	for f := parent.rootFrame + 1; f < len(es.frames); f++ {
		for i := range es.frames[f] {
			if ev := &es.frames[f][i]; ev.kind == evDynBind && ev.dyn != nil && captured[ev.dyn.name] {
				return false
			}
		}
	}
	if parent.deoptNames == nil {
		parent.deoptNames = map[string]bool{}
	}
	for n := range captured {
		parent.deoptNames[n] = true
	}
	parent.deoptChildren = append(parent.deoptChildren, es.openUnitRecs[len(es.openUnitRecs)-1])
	return true
}

// collectWordNames adds every word name spelled in tokens — inside lists,
// maps and paren groups too — to names.
func collectWordNames(tokens []core.Value, names map[string]bool) {
	for _, t := range tokens {
		if t.Data == nil {
			continue
		}
		if w, err := core.AsWord(t); err == nil {
			names[w.Name] = true
			continue
		}
		if inner, err := core.AsParenExpr(t); err == nil {
			collectWordNames(inner, names)
			continue
		}
		if l, err := core.AsList(t); err == nil {
			collectWordNames(l.Slice(), names)
			continue
		}
		if m, err := core.AsMap(t); err == nil {
			for _, k := range m.Keys() {
				if v, ok := m.Get(k); ok {
					collectWordNames([]core.Value{v}, names)
				}
			}
		}
	}
}

// deoptDefsBindable reports whether every def among names that the unit
// binds can be made registry-visible by lowerDynBind: a root-level bind of
// an inert literal (or a value resolving to a const or a local), or of a
// single-output call's result that no fragment owns; a def bound inside a
// branch or loop body, from a fragment result, a variadic or a
// multi-output producer, or of a FN value, cannot.
func (es *EmitState) deoptDefsBindable(events []EmitEvent, names map[string]bool) bool {
	bySeq := map[int]*EmitEvent{}
	for i := range events {
		bySeq[events[i].seq] = &events[i]
	}
	var nested func(frag *EmitFragment) bool
	nested = func(frag *EmitFragment) bool {
		if frag == nil {
			return false
		}
		for i := range frag.events {
			ev := &frag.events[i]
			if ev.kind == evDynBind && ev.dyn != nil && names[ev.dyn.name] {
				return true
			}
			for _, f := range childFragments(ev) {
				if nested(f) {
					return true
				}
			}
		}
		return false
	}
	for i := range events {
		ev := &events[i]
		for _, f := range childFragments(ev) {
			if nested(f) {
				return false
			}
		}
		if ev.kind != evDynBind || ev.dyn == nil || !names[ev.dyn.name] {
			continue
		}
		if ev.dyn.srcSeq < 0 {
			// A literal binding binds only what lowerDynBind can bake or
			// re-push: inert data, or a value that resolves to a const or
			// a local. A body-local FN def (`def check-paths fn […]`), a
			// stripped carrier with no original, cannot — the unit would
			// refuse at its bind, so the points decline instead.
			if ev.dyn.src.kind == opNone && !core.IsInertConst(ev.dyn.val) {
				if op, ok := es.resolveOperand(ev.dyn.val); !ok || (op.kind != opConst && op.kind != opLocal) {
					return false
				}
			}
			continue
		}
		src := bySeq[ev.dyn.srcSeq]
		if !singleOutputCall(src) || es.eventInfo[ev.dyn.srcSeq].variadicResult {
			return false
		}
	}
	return true
}

// deoptPointFor places one deopt (planDeopts). The read's STATEMENT begins
// at the read itself when its consumer follows it on the stack (`j
// typeof`, `a j add` — tested at the read's push, atPush) or when nothing
// consumes it (a residual read an event follows); at the consumer's own
// token when the consumer is a forward word collecting the read (`def k
// j`, `print j`, `for 1 [j]`) or a branch holding it in an arm (`if c [j]
// [0]`); at the literal's or paren's token when the read sits inside one
// (`{a: j}`, `(j typeof)`, `(j) typeof`). The start must be a body token,
// and the interpreter's stack there must be the compiled stack at the
// test: an operand of the consumer (a point tested before it) or of a
// LATER root event written before the start — a literal, a bare read of
// a local or a def whose reads before the start outnumber their
// consumptions there, or an earlier event's result no def consumed — is a
// value the compiled stack still lacks, so such a point is declined and
// the read keeps its slot push (best effort). The def binding the read's
// own name is its binding, not a consumer. A closure unit's CAPTURED value
// lives in its capture slot (slot >= 0) rather than at a producer (seq).
func (es *EmitState) deoptPointFor(u *emitUnit, rec *fnUnitRec, id string, seq, slot int, name string, first core.SrcPos) (deoptPoint, bool) {
	ci, direct := es.deoptConsumer(rec, id, seq, slot)
	d, ok := es.deoptStatementStart(rec, seq, name, first, ci, direct)
	if !ok {
		return d, false
	}
	d.slot = slot
	if es.deoptDeferred(u, rec, &d, ci) {
		return d, false
	}
	return d, true
}

// deoptConsumer finds the first root event consuming the read's value —
// as an operand (direct), inside one of its fragments, or as the source
// of a def made THROUGH a read of it (the token after the def's name is,
// or holds, that read: `def k j`, `def k (j)`; the def binding the value
// straight from its producer, `def j (m get "f")`, is its binding, not a
// consumer). -1 when nothing consumes it.
func (es *EmitState) deoptConsumer(rec *fnUnitRec, id string, seq, slot int) (int, bool) {
	events := rec.frag.events
	is := func(op EmitOperand) bool {
		return (op.kind == opEvent && seq >= 0 && op.idx == seq) || (op.kind == opLocal && slot >= 0 && op.idx == slot)
	}
	readIn := func(t int) bool {
		for _, q := range rec.localReads[id] {
			if bodyTokenContaining(rec.body, q) == t {
				return true
			}
		}
		return false
	}
	for i := range events {
		ev := &events[i]
		uses, direct := false, false
		if ev.kind == evDynBind && ev.dyn != nil && ((seq >= 0 && ev.dyn.srcSeq == seq) || is(ev.dyn.src)) {
			if t := bodyTokenAt(rec.body, ev.dyn.pos); t >= 0 && t+1 < len(rec.body) && readIn(t+1) {
				uses = true
			}
		}
		forEachOperand(ev, func(op EmitOperand) {
			if is(op) {
				uses, direct = true, true
			}
		})
		forEachFragmentOperand(ev, func(op EmitOperand) {
			if is(op) {
				uses = true
			}
		})
		if uses {
			return i, direct
		}
	}
	return -1, false
}

// deoptStatementStart places the point: at the read's push when its
// consumer follows it on the stack (atPush); at the read when nothing
// consumes it and an event follows (the replay's word island seats a lone
// residual read instead); at the `if` before the arm holding it; at the
// literal it sits in; at the def word or the forward word collecting it.
// The start must be a body token.
func (es *EmitState) deoptStatementStart(rec *fnUnitRec, seq int, name string, first core.SrcPos, ci int, direct bool) (deoptPoint, bool) {
	events := rec.frag.events
	readTok := bodyTokenAt(rec.body, first)
	contTok := bodyTokenContaining(rec.body, first)
	d := deoptPoint{seq: seq, slot: -1, name: name, pos: first, start: first}
	switch {
	case ci < 0:
		if readTok < 0 {
			return d, false
		}
		later := false
		for i := range events {
			if posAfter(eventPos(events[i]), first) {
				later = true
			}
		}
		if !later {
			for _, w := range rec.dynFrameWords {
				if w.Name == name {
					return d, false
				}
			}
			d.atPush = true
		}
	case readTok >= 0 && direct && posAfter(eventPos(events[ci]), first):
		// Tested at the read's push when no event lies between the read
		// and its consumer; otherwise (`j (m get "g") add` — the get runs
		// before the push) before that event, where the stack is the
		// read's.
		d.atPush = true
		for i := range events {
			if p := eventPos(events[i]); p.Row > 0 && posAfter(p, first) && posAfter(eventPos(events[ci]), p) {
				d.atPush = false
				break
			}
		}
	case events[ci].kind == evBranch || events[ci].kind == evLoop:
		// A branch event stands at its condition, a loop at its count, or
		// nowhere: the statement begins at the `if` / `for` word before
		// the body holding the read (`if c [j] [0]`, `if c [0] [j]`, `for
		// 2 [j]`) — the nearest one within the form.
		if contTok < 0 {
			return d, false
		}
		want := "if"
		if events[ci].kind == evLoop {
			want = "for"
		}
		t := -1
		for k := contTok - 1; k >= 0 && k >= contTok-4; k-- {
			if w, err := core.AsWord(rec.body[k]); err == nil && w.Name == want {
				t = k
				break
			}
		}
		if t < 0 {
			return d, false
		}
		d.start = rec.body[t].Pos()
	default:
		d.start = eventPos(events[ci])
		if readTok < 0 {
			// A read inside a LITERAL (`{a: j}`, `[j 1]`), a PAREN (`(j
			// typeof)`, `[(j typeof)]`) or a loop body (`for 1 [j]`): the
			// statement is the top-level token holding the read when its
			// consumer is that token's own maker, an event inside it or a
			// word after it (`(j) typeof`) — nothing of the token has run
			// at the test — and the consuming word's own token when that
			// word collects the token forward (`for 1 [j typeof]`). A read
			// nested anywhere else (a body a closure unit owns) is not
			// this unit's to place.
			if contTok < 0 {
				return d, false
			}
			tokPos := rec.body[contTok].Pos()
			c := &events[ci].call
			maker := events[ci].kind == evCall && (c.makeList || c.makeMap)
			switch {
			case maker || (d.start.Row > 0 && (bodyTokenContaining(rec.body, d.start) == contTok || posAfter(d.start, tokPos))):
				d.start = tokPos
			case bodyTokenAt(rec.body, d.start) < 0:
				return d, false
			}
		}
		// A def's event stands at its NAME token: the statement begins at
		// the binding word before it (`def k j`).
		if events[ci].kind == evDynBind {
			if t := bodyTokenAt(rec.body, d.start); t > 0 && core.IsWord(rec.body[t-1]) {
				d.start = rec.body[t-1].Pos()
			} else {
				return d, false
			}
		}
	}
	if d.token = bodyTokenAt(rec.body, d.start); d.token < 0 {
		return d, false
	}
	return d, true
}

// deoptDeferred is the deferred-operand accounting over the events the
// island takes over: an operand of a LATER root event (or of the residual)
// the compiled stack still lacks at the start while the interpreter's
// holds it — a literal written before the start (a pooled scalar counted
// by canon against its consumptions, a compound by its own position), a
// local or def read there more often than consumed, or the result of an
// event before the consumer (promotion may defer its push; the island
// produces nothing before the start) unless a def consumed that result —
// the island reads it by name.
func (es *EmitState) deoptDeferred(u *emitUnit, rec *fnUnitRec, d *deoptPoint, ci int) bool {
	events := rec.frag.events
	later := func(i int) bool {
		if ci >= 0 {
			return i > ci
		}
		return !posAfter(d.start, eventPos(events[i]))
	}
	readsBefore := func(id string) int {
		n := 0
		for _, p := range rec.localReads[id] {
			if posAfter(d.start, p) {
				n++
			}
		}
		return n
	}
	consumedBefore := func(match func(EmitOperand) bool, matchVal func(core.Value) bool, bindSeq int) int {
		n := 0
		count := func(op EmitOperand) {
			if match(op) {
				n++
			}
		}
		for i := range events {
			// A RE-STEP point sits AFTER its event, whose consumptions
			// have happened on both lanes by then.
			if later(i) || (i == ci && !d.restep) {
				continue
			}
			forEachOperand(&events[i], count)
			forEachFragmentOperand(&events[i], count)
			ev := &events[i]
			if ev.kind != evDynBind || ev.dyn == nil {
				continue
			}
			if bindSeq >= 0 && ev.dyn.srcSeq == bindSeq {
				n++
			}
			// A def of a literal rides the literal verbatim (no operand):
			// `def y 1` consumed its `1`.
			if matchVal != nil && ev.dyn.srcSeq < 0 && ev.dyn.src.kind == opNone && ev.dyn.val.Data != nil && matchVal(ev.dyn.val) {
				n++
			}
		}
		return n
	}
	deferredRead := func(id string) bool {
		pr, hasPr := es.producedBy[id]
		slot, isLocal := u.localByID[id]
		bindSeq := -1
		if hasPr {
			bindSeq = pr.seq
		}
		return readsBefore(id) > consumedBefore(func(op EmitOperand) bool {
			return (op.kind == opEvent && hasPr && op.idx == pr.seq) || (op.kind == opLocal && isLocal && op.idx == slot)
		}, nil, bindSeq)
	}
	deferredConst := func(c core.Value) bool {
		if len(rec.localReads[c.ID]) > 0 {
			return deferredRead(c.ID)
		}
		compound := isCompoundValue(c)
		if p := c.Pos(); compound && p.Row > 0 {
			return posAfter(d.start, p)
		}
		// A pooled scalar, or a compound the fold re-minted without its
		// position: its tokens before the start against its consumptions.
		canon := core.CanonValue(c)
		sameCanon := func(v core.Value) bool {
			return !core.IsWord(v) && isCompoundValue(v) == compound && core.CanonValue(v) == canon
		}
		tokens := 0
		for _, t := range rec.body {
			if q := t.Pos(); q.Row > 0 && posAfter(d.start, q) && sameCanon(t) {
				tokens++
			}
		}
		return tokens > 0 && tokens > consumedBefore(func(op EmitOperand) bool {
			return op.kind == opConst && op.idx >= 0 && op.idx < len(es.consts) && core.CanonValue(es.consts[op.idx]) == canon
		}, sameCanon, -1)
	}
	defBound := func(seq int) bool {
		for i := range events {
			if ev := &events[i]; ev.kind == evDynBind && ev.dyn != nil && ev.dyn.srcSeq == seq {
				return true
			}
		}
		return false
	}
	deferred := func(op EmitOperand) bool {
		switch op.kind {
		case opConst:
			if op.idx >= 0 && op.idx < len(es.consts) {
				return deferredConst(es.consts[op.idx])
			}
		case opLocal:
			for id, slot := range u.localByID {
				if slot == op.idx && deferredRead(id) {
					return true
				}
			}
		case opEvent:
			// A RE-STEP point's own event just left its results on top:
			// they are exactly what the island re-steps.
			if d.restep && ci >= 0 && op.idx == events[ci].seq {
				return false
			}
			for j := range events {
				if events[j].seq != op.idx {
					continue
				}
				if defBound(op.idx) {
					// A def consumed it on both lanes (promoted to its slot
					// here, bound in the interpreter's frame); the island
					// reads it by name through the dyn-scope bind.
					return false
				}
				p := eventPos(events[j])
				return p.Row > 0 && posAfter(d.start, p)
			}
		}
		return false
	}
	return deoptAnyDeferred(events, rec, d, ci, later, deferred)
}

// deoptAnyDeferred walks the operands a deopt island takes over — every
// later event's, the consumer's own, and the residual's inert ones, pushed
// last of all — and reports the first one deferred. A point tested before
// its consumer's first op (never at the read's push, where the stack is
// exact, nor after the event a RE-STEP point follows, which has consumed
// them) needs the consumer's own operands in hand too — the read's value
// aside.
func deoptAnyDeferred(events []EmitEvent, rec *fnUnitRec, d *deoptPoint, ci int, later func(int) bool, deferred func(EmitOperand) bool) bool {
	for i := range events {
		if !later(i) {
			continue
		}
		unsafe := false
		forEachOperand(&events[i], func(op EmitOperand) {
			if deferred(op) {
				unsafe = true
			}
		})
		if unsafe {
			return true
		}
	}
	if ci >= 0 && !d.atPush && !d.restep {
		unsafe := false
		forEachOperand(&events[ci], func(op EmitOperand) {
			if !((op.kind == opEvent && op.idx == d.seq) || (op.kind == opLocal && op.idx == d.slot)) && deferred(op) {
				unsafe = true
			}
		})
		if unsafe {
			return true
		}
	}
	for _, op := range rec.outOps {
		if op.kind != opEvent && deferred(op) {
			return true
		}
	}
	return false
}

// isCompoundValue reports a list or map value (never pooled as a const).
func isCompoundValue(v core.Value) bool {
	return v.Parent != nil && (v.Parent.Equal(core.TList) || v.Parent.Equal(core.TMap))
}

// bodyTokenContaining is the index of the last body token standing at or
// before position p — the top-level token a nested position lies in — or
// -1.
func bodyTokenContaining(body []core.Value, p core.SrcPos) int {
	at := -1
	for i, t := range body {
		q := t.Pos()
		if q.Row == 0 || posAfter(q, p) {
			continue
		}
		at = i
	}
	return at
}

// bodyTokenAt is the index of the body token standing at position p, or -1.
func bodyTokenAt(body []core.Value, p core.SrcPos) int {
	if p.Row == 0 {
		return -1
	}
	for i, t := range body {
		if q := t.Pos(); q.Row == p.Row && q.Col == p.Col {
			return i
		}
	}
	return -1
}

// wordReadAccounting is the unit-finish check that every bare read the
// engine noted as a word dispatch (NUR123) has a faithful home: consumed by
// a fn-value apply lowering (creditWordRead) or seated in the armed replay
// window (dynFrameWords, which re-steps it as the word). A read the model
// consumed anywhere else — a list or map literal's member, an if-arm's
// residual, a stack-collected argument, a `def` — is a place where the
// interpreter dispatched the binding and this unit pushed its slot; and a
// binding read both bare and by `/v` shares one value ID across the two
// spellings, so the residual cannot say which read it holds. Both refuse.
// Returns the refusal reason, or "" when the unit's reads are accounted.
func (es *EmitState) wordReadAccounting(rec *fnUnitRec) string {
	if len(rec.wordReads) == 0 {
		return ""
	}
	seated := map[string]int{}
	if rec.dynFrameW > 0 && len(rec.dynFrameWords) > 0 {
		window := rec.outOpsVals[len(rec.outOpsVals)-rec.dynFrameW:]
		for i, v := range window {
			if i < len(rec.dynFrameWords) && rec.dynFrameWords[i].Name != "" {
				seated[v.ID]++
			}
		}
	}
	for id, n := range rec.wordReads {
		name := rec.wordReadNames[id]
		if rec.valReads[id] > 0 {
			return "binding `" + name + "` is read both bare and by /v in one body (one value ID, two dispatch semantics — NUR123)"
		}
		if n > rec.wordReadCredit[id]+seated[id] {
			return "bare read of `" + name + "` is consumed where the interpreter dispatches it (a container member, a branch residual, a stack-collected argument — NUR123)"
		}
	}
	return ""
}

// seatDynFrameWords records a replay's word table on the unit at the pc of
// the OpCallDynFrame about to be emitted (CompiledFn.DynFrameWords); a
// replay with no word read records nothing.
func seatDynFrameWords(cf *CompiledFn, pc int, words []DynFrameWord) {
	if len(words) == 0 {
		return
	}
	if cf.DynFrameWords == nil {
		cf.DynFrameWords = map[int][]DynFrameWord{}
	}
	cf.DynFrameWords[pc] = words
}

// wordReadName reports the binding NAME a residual value was read under
// when that read is a WORD DISPATCH on the interpreter, or "" when the value
// is not such a read.
//
// The interpreter's stepWord substitutes a binding's value only for a
// non-fn binding; a binding holding a FnDefInfo "goes through normal Lookup"
// — a word dispatch by name (a 0-arg fn fires, a no-match raises `cannot
// call `g“). The check model binds a CARRIER for the same param (a
// Function carrier through the fn-carrier side table, an Any carrier
// through Defs), which no signature can dispatch, so the read lands in the
// residual as a value and lowers as a slot push (NUR123). The read is a
// word read when the ENGINE noted it (NoteWordRead: the bare-read path, a
// fn-admitting carrier, no Function-expecting forward pending) on this
// unit; a `/v` value with a pending `apply` is the apply word's own.
func (es *EmitState) wordReadName(rec *fnUnitRec, v core.Value) string {
	if es == nil || rec == nil || v.ID == "" || v.Quoted || es.applyPending(v.ID) {
		return ""
	}
	// A binding read both bare and by `/v` shares one value ID across two
	// dispatch semantics: the residual entry may be the `/v` delivery, not
	// the word. That mix is the accounting's refusal (wordReadAccounting),
	// never a seat.
	if rec.valReads[v.ID] > 0 {
		return ""
	}
	return rec.wordReadNames[v.ID]
}

// openUnitRec is the INNERMOST open fn-unit record — the frame a recording
// site is currently inside — or nil in the main code, which has no frame.
func (es *EmitState) openUnitRec() *fnUnitRec {
	if len(es.openUnitRecs) == 0 {
		return nil
	}
	return es.fnRecs[es.openUnitRecs[len(es.openUnitRecs)-1]]
}

// dynApplyHeadName is CompiledFn.DynApplyName's entry for a trailing fn-value
// apply whose head is v: the binding NAME v was read bare under and the
// READ's position. The interpreter dispatches such a read as a WORD, so its
// no-match names the binding and points at the read (`cannot call `g“ at
// 1:37); the compiled apply holds a frame SLOT with neither, which is the
// whole reason the pair has to ride in the bytecode. Zero when the head was
// not a bare read (an event-produced fn, a `/v` delivery) — there the
// interpreter has no binding name to print either.
func (es *EmitState) dynApplyHeadName(rec *fnUnitRec, v core.Value, args []core.Value) DynApplyHead {
	if name := es.wordReadName(rec, v); name != "" {
		return DynApplyHead{Name: name, Pos: rec.wordReadPos[v.ID], NWritten: writtenRun(rec, args)}
	}
	return DynApplyHead{}
}

// readSubstituted reports whether v arrived by a READ of a binding on this
// unit — bare (`y`) or through a value reference (`y/v`).
//
// Both are SUBSTITUTIONS: the pointer replaces the token with the binding's
// value before the head dispatches, so the forward window consumed no token
// for either and neither reaches the interpreter's written tuple. Measured
// 2026-09-07 — `def f fn [[g:Function y:Integer][Integer][(g y/v)]]` reports
// `takes 1 argument, but none were supplied`, exactly as the bare spelling
// does. Consulting only the bare half over-counted the `/v` one as written.
//
// NoteLocalRead and NoteValRead record EVERY such read, whatever the value's
// type, which is what makes this the SYNTACTIC test the written tuple needs: a
// body-local bound to a literal folds its read to a const operand
// indistinguishable from a written literal's, and only the read itself tells
// them apart. Deliberately NOT wordReadName, whose gate is noteWordRead's
// fn-admitting one — that signal answers NUR123's "does this read DISPATCH",
// a different question.
//
// Both tables are keyed by value ID for the whole unit, so an ID read anywhere
// in the unit reads as substituted everywhere in it. That is sound HERE
// because the two classes never share an ID: a WRITTEN occurrence is a literal
// or a paren-computed event, each minted with its own fresh ID, while every
// occurrence that shares a binding's ID is by construction a read of it.
func readSubstituted(rec *fnUnitRec, v core.Value) bool {
	if rec == nil || v.ID == "" {
		return false
	}
	return len(rec.localReads[v.ID]) > 0 || rec.valReads[v.ID] > 0
}

// writtenRun is how many of a dispatch's arguments the interpreter's forward
// window CONSUMED as tokens: it walks them left to right and stops at the
// first bare read, which the pointer had already substituted onto the value
// stack. So the tuple its no-match prints is the longest LEADING RUN of
// non-read arguments — `(g 5 y 6)` reports `[5]`, not `[5 6]`.
func writtenRun(rec *fnUnitRec, args []core.Value) int {
	for i, v := range args {
		if readSubstituted(rec, v) {
			return i
		}
	}
	return len(args)
}

// dynFrameWordsFor is the replay's word table for a token region: index i
// names the binding region entry i was read bare under ("" otherwise). Nil
// when no entry is a word read — the replay then keeps value semantics.
func (es *EmitState) dynFrameWordsFor(u *emitUnit, rec *fnUnitRec, window []core.Value) []DynFrameWord {
	// The NIL contract is load-bearing and must not widen: callers read a nil
	// table as "this window carries no word read" and arm the replay on it
	// (noteWordReadReplay, replayValueApplicables). Read marks are diagnostic
	// only, so they ride on a table that already exists for a NAME — allocating
	// for a read alone armed replays that had no business arming, and refused
	// three module rows that used to compile.
	var names []DynFrameWord
	for i, v := range window {
		name := es.wordReadName(rec, v)
		if name == "" {
			continue
		}
		if names == nil {
			names = make([]DynFrameWord, len(window))
		}
		names[i] = DynFrameWord{Name: name, Pos: rec.wordReadPos[v.ID]}
	}
	if names == nil {
		return nil
	}
	// The table exists: mark every entry's read-ness for the no-match tuple.
	for i, v := range window {
		names[i].Read = readSubstituted(rec, v)
	}
	return names
}

// noteWordReadReplay arms the whole-frame replay for a residual whose COUNT
// matched but which carries a bare read of a fn-valued binding (NUR123):
// the interpreter dispatches that read as a word, so the region must
// re-step under the binding name (fnUnitRec.dynFrameWords). True when no
// such read exists or the replay seated; false keeps the refusal — a read
// the window cannot reach (an unnamed-param prefix position), a body whose
// events run after the window's production, or a second value-semantics
// applicable beside it (the flat re-step cannot order two).
func (es *EmitState) noteWordReadReplay(u *emitUnit, rec *fnUnitRec, vals []core.Value) bool {
	all := es.dynFrameWordsFor(u, rec, vals)
	if all == nil {
		return true
	}
	// A replay that cannot seat refuses only a FN-TYPED read (the
	// interpreter dispatches it unconditionally); a gradual read keeps the
	// slot push it always had — best effort, never a new refusal.
	strict := false
	for i, v := range vals {
		if all[i].Name != "" && core.IsFnTypedCarrier(v) {
			strict = true
		}
	}
	w, ok := dynFrameWindow(u, rec, vals)
	if !ok {
		return !strict
	}
	window := vals[len(vals)-w:]
	names := es.dynFrameWordsFor(u, rec, window)
	// A word-read entry is anchored at its READ, not at the binding's
	// value: a param carrier carries the declaration's position and a
	// body-local's value its producer's, both of which precede every
	// body event — `[def y 1  g]` and `[def j (m get "f")  def y 1  j]`
	// would never read as tails. SetPos overrides (WithPosAt keeps an
	// existing position, which every such value has).
	anchored := append([]core.Value(nil), window...)
	wordRead := make([]bool, len(window))
	for i := range anchored {
		if names[i].Name != "" && names[i].Pos.Row > 0 {
			anchored[i].SetPos(names[i].Pos)
			wordRead[i] = true
		}
	}
	if !es.replayIsBodyTailAnchored(rec.frag, anchored, wordRead) || replayValueApplicables(window, names) > 1 {
		return !strict
	}
	rec.dynFrameW = w
	rec.retReplay = true
	rec.dynFrameWords = names
	return true
}

// replayValueApplicables counts the window values the replay re-steps under
// VALUE semantics that could apply — every applicable minus the word-read
// entries, which re-step as WORDS through the interpreter's own dispatch
// and need no one-applicable bound.
func replayValueApplicables(window []core.Value, names []DynFrameWord) int {
	n := 0
	for i, v := range window {
		if i < len(names) && names[i].Name != "" {
			continue
		}
		if v.Dynamic || (v.Parent != nil && v.Parent.ConformsTo(core.TFunction)) {
			n++
		}
	}
	return n
}

// replayLeadApplicables counts the window values that COMPETE for the
// whole-frame replay's apply. Every applicable counts — a Function-typed
// value or a Dynamic (maybe-callable) carrier — with ONE discount: beside a
// FN-TYPED lead, a GRADUAL WORD-READ entry (Dynamic, not fn-typed, read bare
// under a frame name) does not compete, because the replay re-steps it as
// the interpreter's own word dispatch, faithful whether it holds a fn or
// data (a gradual `x:Any` param beside a captured `g` in `[(g x)]`, the
// twenty-sixth increment). With no fn-typed value in the window the rule is
// the original one — exactly one applicable of any kind — so a gradual
// body-local read alone (`def j (m get "f")  j 3`, NUR123) is still the
// lead and two gradual reads still decline. noteDynFrameReplay arms only
// when this is exactly 1: a second fn-typed value, word read or not
// (`f (g x y)` collapses to no event and flattens), and a gradual EVENT
// result beside a fn-typed lead (`(g (x get "k"))`, whose runtime fn the
// value re-step would apply) both decline.
func replayLeadApplicables(window []core.Value, names []DynFrameWord) int {
	fnTyped, gradual, gradualReads := 0, 0, 0
	for i, v := range window {
		switch {
		case v.Parent != nil && v.Parent.ConformsTo(core.TFunction):
			fnTyped++
		case !v.Dynamic:
		case i < len(names) && names[i].Name != "":
			gradualReads++
		default:
			gradual++
		}
	}
	if fnTyped > 0 {
		return fnTyped + gradual
	}
	return gradual + gradualReads
}

// replayIsBodyTail reports whether the replay window is the body's LAST
// statement — the whole-frame replay fires at the RET, so any recorded event
// that ran AFTER the window's production would have its effects reordered
// ahead of the apply's (the interpreter runs the apply at its own source
// position: `[(args.0 args.1) print "after"]` prints the callee's output
// first) — such a body declines and falls back. An event-free body is
// trivially a tail. Two proofs, by window shape:
//
//   - A window holding an EVENT RESULT orders by the recorded trace: the
//     trace is the check run's execution order, so the window is the tail
//     exactly when no event was recorded after the window's last producer.
//     (Source columns cannot prove this — a nested-paren argument
//     `(rules get (Fmt.kind nd))` sits textually after its consumer but ran
//     before it, and a check-run carrier often carries no Pos at all.)
//   - An all-inert window (param re-reads) has no producer to anchor on;
//     order by source position — the apply fires where the fn value is
//     read. With events but no positioned window value, the order cannot
//     be proven and the replay declines.
func (es *EmitState) replayIsBodyTail(frag *EmitFragment, window []core.Value) bool {
	return es.replayIsBodyTailAnchored(frag, window, nil)
}

// replayIsBodyTailAnchored is replayIsBodyTail with the window's WORD-READ
// entries (wordRead[i], NUR123) anchored at their READ rather than at their
// producer: the interpreter dispatches the word where the read stands, so
// every event positioned before the read ran before the dispatch on both
// lanes — a body-local's producer and its `def`, an unrelated `def y 1` —
// and only an event after the read reorders (`def j (m get "f")  def y 1
// j` arms; `def j (m get "f")  j drop  j` does not).
func (es *EmitState) replayIsBodyTailAnchored(frag *EmitFragment, window []core.Value, wordRead []bool) bool {
	if frag == nil || len(frag.events) == 0 {
		return true
	}
	anchorSeq := -1
	readAnchor := core.SrcPos{}
	for i, v := range window {
		if i < len(wordRead) && wordRead[i] {
			if p := v.Pos(); posAfter(p, readAnchor) {
				readAnchor = p
			}
			continue
		}
		if pr, ok := es.producedBy[v.ID]; ok && pr.seq > anchorSeq {
			anchorSeq = pr.seq
		}
	}
	if anchorSeq >= 0 || readAnchor.Row > 0 {
		for i := range frag.events {
			if frag.events[i].seq <= anchorSeq {
				continue
			}
			if readAnchor.Row > 0 && !posAfter(eventPos(frag.events[i]), readAnchor) {
				continue
			}
			// A dyn-BIND of a value the window itself READS is not a
			// reorderable effect: the window can only read the def-bound
			// value AFTER the bind (same value instance — ID equality, so a
			// rebind of a different value never matches), which puts the
			// interpreter's tail apply after the bind, and the VM keeps the
			// same order (the bind op at its position, the replay at RET).
			// This is what lets the def-split idiom `def r (f x) f r` arm:
			// the def's evDynBind lands between r's producer and the tail
			// (§9.4 graduation, checker-compiler-completeness-review).
			if ev := &frag.events[i]; ev.kind == evDynBind && ev.dyn != nil &&
				ev.dyn.val.ID != "" && windowReadsID(window, ev.dyn.val.ID) {
				continue
			}
			return false
		}
		return true
	}
	anchor := core.SrcPos{}
	for _, v := range window {
		if v.Pos().Row > anchor.Row || (v.Pos().Row == anchor.Row && v.Pos().Col > anchor.Col) {
			anchor = v.Pos()
		}
	}
	if anchor.Row == 0 && anchor.Col == 0 {
		return false
	}
	for i := range frag.events {
		p := eventPos(frag.events[i])
		if p.Row > anchor.Row || (p.Row == anchor.Row && p.Col > anchor.Col) {
			return false
		}
	}
	return true
}

// posAfter reports whether p lies after q in source order.
func posAfter(p, q core.SrcPos) bool {
	return p.Row > q.Row || (p.Row == q.Row && p.Col > q.Col)
}

// windowReadsID reports whether the replay window holds the value with the
// given ID — the read that makes a later dyn-bind of that same value a
// non-reorderable event for replayIsBodyTail.
func windowReadsID(window []core.Value, id string) bool {
	for _, v := range window {
		if v.ID == id {
			return true
		}
	}
	return false
}

// dynFrameWindow classifies a fn-body residual that carries an unapplied
// runtime fn value as the whole-frame dynamic-apply replay (OpCallDynFrame): a
// leading PREFIX of unnamed-param frame re-pushes (param locals, capped at
// nUnnamed — the values the interpreter's pointer never steps) under a TOKEN
// region holding everything the pointer did step, at least one entry of which
// is the fn value that could not statically dispatch. Returns the token-region
// width. The replay is faithful by construction — the region's values re-step
// under execFnDefLiteral's own runtime rule, with the prefix (and nothing
// deeper: both engines bound collection at the frame) as the stack below — so
// the only decline is a residual with no token region at all.
func dynFrameWindow(u *emitUnit, rec *fnUnitRec, vals []core.Value) (int, bool) {
	k := 0
	for k < len(vals) && k < rec.nUnnamed {
		slot, isLocal := u.localByID[vals[k].ID]
		if !isLocal || slot >= rec.nParams {
			break
		}
		k++
	}
	if w := len(vals) - k; w >= 1 {
		return w, true
	}
	return 0, false
}

// residualLeadReStepped reports whether a branch ARM's residual LEADS with a
// value the interpreter's frame-close rewind re-steps INTO A CALL while this
// arm's merge would model it as placed data (NUR101, measured 2026-08-27).
//
// The arm body executes inside a frame, and a frame closes through
// stepCloseParen like any other paren: with MORE than one survivor the park
// declines, the rewind lands ON the leading value, and a Function there
// dispatches. `if true [(mk 1) 2]` is therefore 3 interpreted — while
// resolveArm reads only stk[len-1] and merges the arm as the placed pair,
// compiling to `fn (Integer) 2`. That was a SILENT divergence, the last of the
// five NUR101 shapes; refuse until the apply is modelled (Stage 3).
//
// The re-step RECORD is deliberately not consulted here: the arm frame is the
// rewinding group, so there is no enclosing paren to have recorded anything —
// the shape IS the fact at this point.
//
// Narrower than closureResidualHasUnappliedFn on purpose. A closure body
// refuses a carrier ANYWHERE, because its driving handler maps every residual
// value; an arm's SOLE carrier is legitimately placed data on both lanes
// (`if c [(mk 1)]`), so only a LEAD over >=2 survivors — the shape the rewind
// re-steps — is unmodelled here.
func residualLeadReStepped(stk []core.Value) bool {
	if len(stk) < 2 || stk[0].Quoted {
		return false
	}
	return core.IsFnTypedCarrier(stk[0]) ||
		(stk[0].Dynamic && core.SigTypeMatches(stk[0], core.TFunction))
}

// closureResidualHasUnappliedFn reports whether a closure body's residual
// leaves an fn value the driving handler (BodyResultTop / BodyOutResidual)
// would map UNAPPLIED — the off-corpus comparator-each MISCOMPILE (`[1 2]
// each [(x x comp)]` → interp [0,0] vs compiled [fn,fn]). Three shapes
// refuse, mirroring resolveDynamicApply's main-residual fn-carrier /
// fn-precedes-args refusals:
//   - a Function-TYPED carrier anywhere (isFnTypedCarrier);
//   - a DYNAMIC value whose static type does not EXCLUDE Function (an
//     Any-typed member read — `r.string charset 8`, where `r` is a Map param
//     and the member is a fn only at runtime) that PRECEDES args: it
//     auto-applies in the interpreter but this unit leaves it unapplied
//     (the `r.<gen> args` → trailing-arg check-prop miscompile), which
//     isFnTypedCarrier misses because Any does not conform DOWN to Function;
//   - a concrete fn value not in the sole position (isFnValueresidual).
//
// A SOLE inert fn-reference body (`each [cmp/v]`) is a concrete const — not a
// carrier, not preceded by args — so it still compiles.
func closureResidualHasUnappliedFn(bodyStk []core.Value) bool {
	for i, v := range bodyStk {
		dynMaybeFn := v.Dynamic && core.SigTypeMatches(v, core.TFunction) && i+1 < len(bodyStk)
		if core.IsFnTypedCarrier(v) || dynMaybeFn || (core.IsFnValueResidual(v) && (i > 0 || i+1 < len(bodyStk))) {
			return true
		}
	}
	return false
}

// fnConcreteSingleValuedOrCarrier reports whether the fn VALUE v is safe to
// lower as a single-result paren-bounded apply (OpCallDynTrailTop, which nets
// EXACTLY ONE value). Two shapes pass:
//
//   - A fn-typed CARRIER (no concrete FnDefInfo payload) — a `comp:Function`
//     param/captured value whose return arity is unknown statically. The
//     comparator-apply convention relies on this still lowering, matching the
//     interpreter's single-result paren auto-dispatch; refusing it would lose
//     compiled coverage with no soundness gain.
//   - A concrete FnDefInfo whose EVERY authored overload declares exactly one
//     return type. A `fn […]`-declared callee always carries an explicit return
//     block (a single return → a length-1 Returns; a lambda → [Any]); only an
//     empty `[]` block yields nil/length-0, i.e. a genuine zero-return fn. So a
//     concrete callee with any sig of length != 1 (0 returns, or 2+ like
//     `pair → [Integer Integer]`) is NOT single-valued and the lowering is
//     unsound — underflow on zero-return, wrong-order/leftover values on
//     multi-return. Such a callee is refused so the faithful interpreter island
//     runs it.
func fnConcreteSingleValuedOrCarrier(v core.Value) bool {
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return true // carrier — arity unknown; keep lowering (comparator convention)
	}
	sigs := fd.OwnSigs()
	if len(sigs) == 0 {
		return false
	}
	for i := range sigs {
		if len(sigs[i].Returns) != 1 {
			return false
		}
	}
	return true
}

func eventPos(ev EmitEvent) core.SrcPos {
	switch ev.kind {
	case evCall:
		return ev.call.pos
	case evFallback:
		return ev.fb.pos
	case evLoop:
		return ev.loop.pos
	case evCallUser:
		return ev.uc.pos
	case evTrap:
		return ev.trap.pos
	case evStore:
		return ev.store.pos
	case evDynBind:
		return ev.dyn.pos
	case evBindTwin:
		return ev.twin.pos
	}
	if ev.br == nil {
		// A break / continue event carries no position.
		return core.SrcPos{}
	}
	return ev.br.pos
}

// eventsThroughSeq returns the prefix of events up to and including the one with
// the given seq (top-level events are appended in seq order). Used to truncate
// the trace at a terminal trap, dropping the unreachable tail the lenient check
// pass recorded after it.
func eventsThroughSeq(events []EmitEvent, seq int) []EmitEvent {
	for i := range events {
		if events[i].seq == seq {
			return events[:i+1]
		}
	}
	return events
}

// methodShapeAnnotated reports whether a value ID carries a method-shape
// annotation this pass. Consulted by resolveDynamicApply: an ANNOTATED
// carrier reaching the program residual as a LEADING or INTERIOR apply
// window means the statement-window model DECLINED it (a computed arg, a
// word in the window, a stack-reaching match), so the residual window's
// "tail = args" assumption is unverified for it — the leading/mixed
// windows may absorb values from LATER statements into the apply (the
// interpreter's forward collection stops at the statement End; the
// flattened residual has no such boundary), a silent-divergence class
// probe-confirmed pre-existing (`c.add (1 add 2) ; Log.measurements
// size` compiled to the wrong value). Such carriers now refuse instead —
// sound fallback. Trailing shapes are unaffected: they draw args from
// the STACK below the value, exactly the interpreter's stack-form
// dispatch, which crosses statements by design.
func (es *EmitState) methodShapeAnnotated(id string) bool {
	if es == nil || es.reg == nil {
		return false
	}
	_, ok := es.reg.Check.MethodShapeMember(id)
	return ok
}

func init() {
	core.NewEmitStateHook = func() core.EmitRecorder { return NewEmitState() }
	core.NewIsolatedEmitHook = newIsolatedEmit
}

// parenPlacedMemberFn reports whether a residual lead is a pinpointed
// member-fn read — the shape a USER paren PLACES rather than applies since
// NUR073's BROAD park (`(m dot f) 5` is two values interpreted, where dot
// SUGAR `m.f 5` still dispatches through its reach group). Lowering such a
// lead as an apply diverges from the interpreter, so resolveDynamicApply
// declines it and the program falls back faithfully. The arrival model
// (method_shape.go) still owns the un-parenthesised mid-expression apply,
// which is why this test is on the residual lead only.
// leadPlacedNotRead reports whether a residual's leading fn carrier is
// PLACED DATA rather than a pending call (NUR101, ruled 2026-08-26).
//
// Placement alone cannot answer this, and that is the whole subtlety.
// ParenPlacedFnIDs is keyed by value ID, and an ID travels with a binding:
// `def h (mk 1) end  h 2` marks h's value placed, because the paren that
// PRODUCED it placed it — but `h` is a bare-NAME dispatch and a bare name
// always calls. Measured: both `h 2` and `((mk 1) 2)` report placed=true, so
// a placement-only gate refuses the one that must apply. That is the
// sticky-inertness fnReturnPark's header forbids, arriving through the side
// table instead of the value.
//
// Read provenance alone cannot answer it either: gating on defReads was
// tried and OVER-REACHED, refusing `c.op 5` — a member-read apply that
// ADR-011 lists among the three explicit application forms.
//
// The conjunction is what works. A lead is placed data only when the paren
// placed it AND it did not arrive through a read that dispatches: a def-read
// (a bare name) or a member read both CALL, whatever mark the value carries
// from wherever it was built.
func (es *EmitState) leadPlacedNotRead(v core.Value) bool {
	if !es.parenPlacedMemberFn(v) {
		return false
	}
	if _, read := es.defReads[v.ID]; read {
		return false
	}
	// An ENCLOSING paren re-stepped it, so the placement was undone one level
	// out and the lead is a pending call after all (the paren re-step rule,
	// design/PAREN-RESTEP-RULE.0.md). This is what separates `((mk 1) 2)` — 3
	// on both lanes — from `(mk 1) 2`, whose identical residual the
	// interpreter places. Recorded at the collapse by recordParenReStep,
	// because nothing downstream can still tell them apart.
	return !es.parenReSteppedFn(v)
}

// placedNotReStepped reports whether the collapse recorded v as PLACED by a
// user paren and NOT re-stepped by an enclosing one — the two facts that
// together prove the interpreter leaves it as data where it now sits.
//
// This is the residual-layout twin of leadPlacedNotRead, and deliberately
// does NOT consult read provenance: a def-read or member-read lead is a
// DISPATCH question (does this name call?), while this is a LAYOUT question
// (will anything re-step this value?). A read that dispatches is handled by
// the apply arms above; by the time control reaches the refusal loop the
// only question left is whether the value sits inert.
func (es *EmitState) placedNotReStepped(v core.Value) bool {
	return es.parenPlacedMemberFn(v) && !es.parenReSteppedFn(v)
}

// callResultPlaced reports whether v is the single RESULT of a completed
// USER-FN call that no enclosing paren re-stepped and no read delivered. The
// interpreter PARKS such a value where it lands: a user fn's returned
// closure is placed data (`mk 7` is `fn (Integer) 7`, `mk 7 add 1` is
// `fn (Integer) 8`, and inside a fn body `[mk 7]` is a two-value residual
// the return check rejects), and only a paren rewind over two or more
// survivors (`(mk 7)` is 8) or a read that dispatches — a bare name, a
// member read — turns it into a call (design/PAREN-RESTEP-RULE.0.md). The
// residual lowerings must therefore lay such a lead out as data, never
// apply it; before this probe the fn-carrier and dynamic-lead arms applied
// every lead a paren had not placed, and `mk 7` compiled to 8 (measured
// 2026-09-05; design/FULL-COMPILATION-HANDOFF.0.md, "A returned closure is
// parked").
//
// Three things do NOT park, and each keeps the arm it had:
//   - a MULTI-output user call: its frame rewound over its own survivors
//     before returning (`def g fn [[] [Any Any] [mk 7]]  g` is 8), which
//     the check model did not perform, so the caller-side apply arm is
//     that rewind's compiled twin;
//   - a NATIVE word's returned Function, re-stepped by its own delivery —
//     a code body's frame rewinds (`do [mk 7]` is 8) and a returned fn
//     auto-applies to what follows (`do [mk] 7` is 8, `(FnUtil.const 7)
//     99` islands);
//   - a def-read lead (a bare name always calls, ADR-011) and a pinpointed
//     member read (the arrival model's).
//
// The arrival apply of a USER member (`m.p 5` over `{p: mk/v}`, recorded
// as a dyn-method event) is a user call by another route, so its single
// result parks too (`m.p 5 7` is `fn (Integer) 7`).
// noteAppliedByWord marks the value a trailing `apply` WORD dispatched, so
// trailingApply can tell a parked result a later word applies on purpose from
// one nothing applies (EmitState.appliedByWord, NUR124). `apply` records as an
// IDENTITY — the check engine returns the fn concrete and re-steps it — which
// is both how the mark is recognised and why the ID survives to the residual.
func (es *EmitState) noteAppliedByWord(word string, args, outs []core.Value) {
	if es == nil || word != "apply" || len(args) == 0 || len(outs) != 1 {
		return
	}
	if args[0].ID == "" || args[0].ID != outs[0].ID {
		return
	}
	if es.appliedByWord == nil {
		es.appliedByWord = map[string]bool{}
	}
	es.appliedByWord[outs[0].ID] = true
}

func (es *EmitState) callResultPlaced(v core.Value) bool {
	return es.callResultPlacedIn(v, nil)
}

// callResultPlacedIn is callResultPlaced for a value produced inside a
// captured fragment — a fn unit's own residual at finish, after
// TakeFragment moved its events off the frame stack.
func (es *EmitState) callResultPlacedIn(v core.Value, frag *EmitFragment) bool {
	if es == nil || v.ID == "" {
		return false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok {
		return false
	}
	var ev *EmitEvent
	if frag != nil {
		for i := range frag.events {
			if frag.events[i].seq == pr.seq {
				ev = &frag.events[i]
				break
			}
		}
	}
	if ev == nil {
		ev = es.eventBySeq(pr.seq)
	}
	if ev == nil {
		return false
	}
	switch ev.kind {
	case evCallUser:
		if ev.uc.nout != 1 {
			return false
		}
	case evCall:
		if ev.call.dynMethod == nil || ev.call.nout != 1 || !es.userMemberFn(ev.call.word) {
			return false
		}
	default:
		return false
	}
	if es.parenReSteppedFn(v) || es.isDefRead(v) {
		return false
	}
	if _, member := es.MemberFnReadValue(v.ID); member {
		return false
	}
	return true
}

// callResultRenderKnown reports whether the user call that produced v
// returns a COMPILED ANONYMOUS CLOSURE — its single out operand is an
// opClosure whose unit carries the interpreter's render string
// (tryReturnedClosure stamps it) — so the parked value renders
// byte-identically on both lanes. Any other fn-valued result (a fn the
// callee was handed, a member's, a poly call's) is unknown here: the
// interpreter may render it under a binding name the runtime value does
// not carry (NUR119), and the residual gate keeps its refusal.
func (es *EmitState) callResultRenderKnown(v core.Value) bool {
	if es == nil || v.ID == "" {
		return false
	}
	pr, ok := es.producedBy[v.ID]
	if !ok {
		return false
	}
	ev := es.eventBySeq(pr.seq)
	if ev == nil || ev.kind != evCallUser || ev.uc.poly != nil || ev.uc.unit < 0 || ev.uc.unit >= len(es.fnRecs) {
		return false
	}
	rec := es.fnRecs[ev.uc.unit]
	if rec == nil || len(rec.outOps) != 1 {
		return false
	}
	out := rec.outOps[0]
	switch out.kind {
	case opClosure:
		cu := out.closureUnit
		return cu >= 0 && cu < len(es.fnRecs) && es.fnRecs[cu] != nil && es.fnRecs[cu].render != ""
	case opConst:
		// A CAPTURE-FREE lambda literal bakes as a const FnDefInfo (the
		// returned-closure path declines it on purpose); the interpreter
		// formats that very value, so its render is the compiler's too.
		if out.idx < 0 || out.idx >= len(es.consts) {
			return false
		}
		_, isFn := es.consts[out.idx].Data.(core.FnDefInfo)
		return isFn
	}
	return false
}

// userMemberFn reports whether word names a module binding holding a user
// fn (a boru body on some overload) — the member a dyn-method event applied
// (tryMemberFnArrivalDispatch records the member fn's NAME as the word).
// A Go-impl member, a class method, or the paren-apply pseudo-word resolve
// to nothing here and keep their arms.
func (es *EmitState) userMemberFn(word string) bool {
	if es == nil || es.reg == nil || word == "" {
		return false
	}
	v, ok := es.reg.Defs.Top(word)
	if !ok {
		return false
	}
	fd, isFn := v.Data.(core.FnDefInfo)
	if !isFn {
		return false
	}
	for i := range fd.Signatures {
		if len(fd.Signatures[i].Body()) > 0 {
			return true
		}
	}
	return false
}

// isDefRead reports whether v arrived through a read of a def-bound name —
// the provenance that makes a value a DISPATCH rather than data, since a bare
// name always calls (ADR-011). leadPlacedNotRead folds the same test into its
// conjunction; the residual-layout loops ask for it separately because they
// answer a different question and only some of them need it.
func (es *EmitState) isDefRead(v core.Value) bool {
	_, read := es.defReads[v.ID]
	return read
}

func (es *EmitState) parenReSteppedFn(v core.Value) bool {
	if es == nil || es.reg == nil || es.reg.Check == nil || v.ID == "" {
		return false
	}
	return es.reg.Check.ParenReSteppedFnIDs[v.ID]
}

func (es *EmitState) parenPlacedMemberFn(v core.Value) bool {
	if es == nil || es.reg == nil || es.reg.Check == nil || v.ID == "" {
		return false
	}
	return es.reg.Check.ParenPlacedFnIDs[v.ID]
}
