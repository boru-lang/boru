package core

// EmitFragmentRef is an OPAQUE recorded-fragment handle. The recorder mints
// one (TakeFragment) and receives it back unread (RecordBranch / RecordLoop);
// core never inspects one, and neither does any word library.
//
// It is `any` for the same reason core/ts's RecorderOperand is `unknown`: the
// concrete fragment is the compiler's representation, and naming it here would
// invert the dependency this seam exists to prevent. A word library that
// downcasts this to reach the compiler's type has defeated the seam — which is
// exactly what basic/go's `if` did before this contract existed.
type EmitFragmentRef any

// BranchRecord is the `if` shape a word library hands the recorder. It lives
// HERE, not in the compiler, so a word implementing branching names only core.
// Its fragment fields are opaque refs; every other field is a core value.
type BranchRecord struct {
	Cond            Value           // pre-evaluated condition (paren/value form)
	CondFrag        EmitFragmentRef // list-form condition body, when analysed
	CondStk         []Value         // its residual stack
	ConstCond       *bool           // statically-known condition: only Then captured
	HasElse         bool
	Then, Els       EmitFragmentRef
	ThenStk, ElsStk []Value
	ThenValue       *Value // non-nil: the then arm is this already-evaluated VALUE
	ElsValue        *Value // non-nil: the else arm is this already-evaluated VALUE
	Out             Value
	Pos             SrcPos
}

// EmitRecorder is the checker-side view of the bytecode recording pass —
// the NARROW seam between static analysis and compilation (G9 / completion
// plan 4.5; comprehensive review Tier-2 item 6). Everything the check side
// (engine.go, carrier.go, core_helpers.go, user_poly.go, check.go, and the
// lang/native handlers) needs from the recorder goes through this interface:
// the checker compiles and runs with NO knowledge of the concrete *EmitState
// beyond it. The emit implementation cluster — emit.go, lower.go,
// callable_words.go — owns the concrete type and may type-assert
// (a concrete-type assert on the recorder) to reach recording internals.
//
// Lifecycle: CheckState.Emit holds the recorder. A PLAIN check pass runs
// against the inactive no-op recorder (Begin installs it); the compile entry
// points (lang CompileCheck / Vm.compile) install a real *EmitState for the
// pass. Read the field through CheckState.Recorder(), which never returns
// nil — the recorder methods are then always callable, mirroring the
// nil-receiver-safe discipline the concrete methods already follow.
//
// The method set is exactly the surface the check side calls (enumerated by
// grepping Check.Emit usages outside the emit cluster). Exported methods are
// callable from lang/native; the unexported tail is eng-internal. The
// unexported methods also mean out-of-package types cannot implement the
// interface — the recorder contract is owned here, by design.
type EmitRecorder interface {
	// --- activity / lifecycle ------------------------------------------
	// Active / active report that recording is LIVE (armed, compilable,
	// not suspended). armed reports that a REAL recording state exists at
	// all (a compile pass installed one), live or not — the interface twin
	// of the historical `Check.Emit != nil` probe. Suspend pauses
	// recording, returning the resume func.
	Active() bool
	Armed() bool
	Suspend() func()
	BindRegistry(r *Registry)
	TopFrameOnly() bool
	SuspendedNow() bool
	BodyAnalysisGuard() func()
	// KeepDefsBodyGuard is BodyAnalysisGuard for a KEEP-DEFS body run
	// (runCarrierBodyDefsAdds keep=true — `do`'s check-mode scoping,
	// where body defs leak): same suspension, but the recorder may
	// additionally bracket the run — the twin regime's do-body adoption
	// adopts only bind twins noted inside the dispatch's own outermost
	// keep-defs run, excluding sub-ranges noted under a nested NON-keep
	// (multi-run / conditional) body run. Inactive: plain no-op.
	//
	// Takes the registry for the same reason MultiRunBodyGuard does: the
	// bracket may only be PUBLISHED by a run that could note a twin at
	// all, which is FnBodyDepth == 0. A do body re-analysed at depth
	// inside a called fn's body has an empty bracket by construction, and
	// publishing it would overwrite the outer do's.
	//
	// bodyID is the body Value's ID, and it is what lets the recorder hand
	// the body's START state to the compile re-run that follows: the guard
	// clones the binding table when it opens and publishes the clone keyed
	// by this ID when it closes (Stage 4b — a leaking body's re-run must
	// begin where the interpreter's single run began, not where the
	// suspended analysis run left off). Empty when the caller has no body
	// value; nothing is published then.
	KeepDefsBodyGuard(r *Registry, bodyID string) func()
	// MultiRunBodyGuard is BodyAnalysisGuard for a HIGHER-ORDER body
	// analysis run (analyseHigherOrderBodyVals — each/fold/scan…, the
	// bodies the runtime re-runs per element): the same suspension and
	// keep-bracket taint, plus a latch — on close the recorder remembers
	// {bodyID, the twin-table range this run noted, r} so the dispatch
	// record that follows can bridge those table-only twins to the
	// compiled per-invocation unit's def sites (arm-resident twins,
	// §6.5's each-body recovery). bodyID is the body Value's ID — the
	// latch's identity guard: a nested body's analysis during the outer
	// unit's compile overwrites the latch, and the mismatched ID makes
	// the outer bridge decline to a sound refusal instead of pairing
	// against the wrong run. r is the noting registry, for the
	// module-registry fence. Inactive: plain no-op.
	MultiRunBodyGuard(r *Registry, bodyID string) func()
	// RecordDynUndef notes an `undef`-shaped teardown at its stream
	// position — today only the var-param cleanup (`__varundef`) inside
	// a multi-run body's compiled unit, where the balanced per-iteration
	// def/undef pair must both execute per element for interpreter
	// parity (the pair's ledger notes carry Pos 0:0, so position-based
	// bridging cannot see them; the recorder pairs them by name and
	// order instead). Inactive: no-op.
	RecordDynUndef(name string, pos SrcPos)
	FnBodyGuard() func()

	// --- refusal + site accounting --------------------------------------
	MarkUncompilable(reason string)
	Sites() map[string]int

	// --- inline context-boundary regions (NUR054) ------------------------
	// PushInlineCtxBoundary / PopInlineCtxBoundary bracket a check-run
	// region whose RUNTIME twin is a fresh sub-engine — and therefore a
	// context-layer push (Engine.Run's Contexts Push/Pop pair) — but whose
	// COMPILED lowering is INLINE in the enclosing unit with no frame to
	// push: the `case` desugar's clause fragments, list auto-evaluation,
	// interp-string holes. The recorder latches the open-unit depth at
	// entry, so a closure unit opened INSIDE the region (a `do` body in a
	// case arm — bracketed at run time by the VM's enterBodyUnit) is not
	// attributed to the region while it records. The compiler's dispatch
	// recorder uses the bracket to refuse a `context` read recorded inline
	// inside a region — the handle would denote the region's own layer,
	// which has no compiled twin, so every layer-distinguishing consumption
	// (a write, an alias, an identity probe) would diverge (NUR054).
	// Inactive: no-ops.
	PushInlineCtxBoundary()
	PopInlineCtxBoundary()

	// SetCatchVariadic latches the NEXT catch-word (CompileFallbackBody)
	// dispatch's recorded result as VARIADIC: `do` catches a body raise into
	// ONE Error value, so a fallible multi-value body's runtime count varies
	// (N no-raise vs 1 caught) and a static N-seat underflows on the caught
	// path. The word's ReturnsFn — the single point that both computes the
	// fallibility (lang doBodyMayRaise) and runs immediately before its own
	// dispatch records — sets/clears it per dispatch; the record paths
	// consume it keyed to the CompileFallbackBody sig, so it can never leak
	// onto an unrelated word's event. Plan Phase 5, L-DO. (The mark covers
	// only the SHRINKING direction; a count that can EXCEED the modeled
	// seats — await first/any — refuses wholesale instead, NUR067.)
	SetCatchVariadic(pending bool)

	// --- dispatch / value recording -------------------------------------
	RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos, forceDynOut, quoteInertOK bool)
	RecordPoly(word string)
	RecordPolyCall(word string, args, outs []Value, pos SrcPos, ownerReg *Registry, noMatch *PolyNoMatchSpec) bool
	RecordUserCall(unit int, args []Value, outs []Value, pos SrcPos)
	RecordUserPolyCall(word string, ownerReg *Registry, sigIdx, units []int, impls []SigImpl, sigs []Signature, args, outs []Value, pos SrcPos)
	// RecordDynApply records a paren-bounded TRAILING fn-value apply and
	// reports how many of `args` the lowered apply CONSUMES, counted from the
	// TOP of the window (the values nearest the fn). That is normally all of
	// them, but a callee whose arity is provably SMALLER under-applies exactly
	// as the interpreter does — `(1 2 (mk 4))` with a 1-arg adder nets [1, 6],
	// the deeper 1 surviving — so the caller must collapse only the consumed
	// suffix of the window and leave the rest on the tape. consumed is
	// meaningful only when ok is true.
	RecordDynApply(args []Value, fn, out Value, pos SrcPos) (consumed int, ok bool)
	// RecordDynApplyName is RecordDynApply with the fn resolved through
	// the NAME's recorded def-site operand (its evDynBind event) — the
	// §4.3 capture fallback for calls of installed factory closures whose
	// units carry construction-scope captures (check's
	// recordFnValueApplyFallback).
	RecordDynApplyName(name string, args []Value, fn, out Value, pos SrcPos) bool
	DynApplyLeadEligible(v Value) bool
	RecordDynMethod(fn Value, args, outs []Value, word string, pos SrcPos) bool
	RecordFallback(span FallbackSpan, ins []Value, out Value, pos SrcPos) bool
	RecordTrap(code, detail, word, hint string, pos SrcPos) bool
	RecordTrapErr(ae *BoruError, pos SrcPos) bool
	RecordDispatchRematchValues(word string, vals []Value, writtenOff, nWritten int, pos SrcPos) bool
	RecordTypedBind(spec TypedBindSpec, in, out Value, pos SrcPos) (Value, bool)
	RecordMakeList(r *Registry, ins []Value, out Value, pos SrcPos) bool
	RecordMakeListInner(r *Registry, ins []Value, out Value, pos SrcPos) bool
	RecordMakeMap(r *Registry, keys []string, vals []Value, implicit bool, out Value, pos SrcPos) bool
	RecordInterp(parts []InterpPart, holeVals []Value, out Value, pos SrcPos) bool
	RegisterTrailingApply(fnID string, arity int)
	NoteMemberFnRead(id string, member Value)
	MemberFnRead(id string) bool
	// NoteCollectionHazard marks the fn-typed value id as an UNAPPLIED lead
	// a later dispatch collected past (Engine.noteCollectionHazards,
	// NUR121); CollectionHazard reads the mark. A marked lead is never
	// lowered as an apply over the values after it.
	NoteCollectionHazard(id string)
	CollectionHazard(id string) bool
	// NoteFnResultReStep marks a NATIVE dispatch's result v — a fn-typed
	// or fn-admitting gradual carrier — that the interpreter re-steps into
	// a dispatch attempt at the call's position, with a plain body token
	// (resume) written after it, where the pass steps the carrier past as
	// data (Engine.noteFnResultReSteps, NUR124). The recorder plans a
	// re-step deopt over the call's results for it, or declines the unit.
	NoteFnResultReStep(v Value, resume SrcPos)
	// Stage-0b promotions (design/ENG-FOUR-PIECE.0.md): the probes that
	// used to require a concrete recorder assert outside the emit
	// cluster. Inactive: false / zero / no-op.
	InClosureUnit() bool
	StoredGradualActive() bool
	FoldFullStack(word string, args, preserved []Value) ([]Value, bool)
	RecordSpliceDyn(payload Value, pos SrcPos) bool
	NoteShapedRead(id string)
	MemberFnReadValue(id string) (Value, bool)
	DynInputsProven(sig *Signature, args []Value) bool
	Materialise(v Value) (Value, bool)
	ZeroOutProduced(id string) bool
	AlreadyProduced(id string) bool

	// --- defs / locals ---------------------------------------------------
	// RecordBindTwin mirrors ONE bind-ledger entry into the compile pass's
	// twin table (design/FULL-COMPILATION.0.md §6.5, the inert-emission
	// stage). Fired by NoteBindTransition itself, AFTER every ledger
	// suppression, so the twin population and the ledger share a single
	// funnel by construction — the corpus gate then asserts the finalized
	// Program's table equals the pass's ledger elementwise, which is what
	// the rollback-and-replay flip will trust.
	//
	// entry is the DefEntry the transition INSTALLED, captured at the note —
	// the only moment the identical binding object is knowably on top —
	// because "replay, never re-execution" re-installs that object: the same
	// FnDefInfo, the same module instance, the same minted node. A BindUndef
	// carries the zero DefEntry: its twin needs nothing captured — at VM
	// time it pops whatever is then live and retires a minted type from the
	// popped entry itself. Inactive: no-op (a plain check pass builds no
	// program).
	RecordBindTwin(tr BindTransition, entry DefEntry)
	MarkValueDef(v Value)
	RecordDefRebind(name string, v Value, pos SrcPos)
	RecordDynBind(name string, v Value, pos SrcPos)
	NoteDefRead(id, name string)
	// NoteLocalRead records a bare read of a frame binding at its read
	// position (both substitution paths of stepWord) — a per-read deopt's
	// deferred-operand accounting (compiler planDeopts, NUR123).
	NoteLocalRead(id string, pos SrcPos)
	// NoteWordRead records a BARE READ of a frame binding that the
	// interpreter would DISPATCH if the binding held a fn at run time —
	// stepWord routes a bound FnDefInfo through Registry.Lookup under the
	// binding name (a 0-arg fn fires, a no-match raises `cannot call
	// `g``) — while the check model pushed a carrier (a fn-typed or
	// gradual Any/Dynamic one) that lowers as a slot push. Noted only when
	// no pending forward expects a Function at the read (that arrival
	// delivers the VALUE on both engines). The compiler counts the reads
	// per unit and refuses any it cannot re-step as a word (NUR123).
	NoteWordRead(v Value, name string, pos SrcPos)
	// NoteValRead records a `/v` read of a binding (stepWordVal): the value
	// spelling, which the interpreter never dispatches. A binding read BOTH
	// ways in one unit cannot be told apart in the residual (one value ID),
	// so the compiler refuses the unit rather than guess (NUR123).
	NoteValRead(id string)
	// NoteFrozenRead's gen is the binding's DefTable generation
	// (DefTable.Gen) at the read, taken by the caller from the registry the
	// read resolved in. It is the staleness key of the binding-sensitive
	// unit memo (compiler StartFnCompile): a finished unit whose bake was
	// noted at generation g is reusable at a later call site exactly while
	// Gen(name) is still g there.
	NoteFrozenRead(name string, bake FrozenBake, gen int64)
	RefuseCarriedUndef(name string)
	NotifyNameRebound(name string)
	RegisterLocal(id string) int
	RememberOriginal(v Value)
	RememberStrippedOriginals(pre, stripped []Value)

	// --- branches / loops ------------------------------------------------
	//
	// TakeFragment / RecordBranch / RecordLoop are what let a word library
	// implement control flow WITHOUT naming a compiler symbol. They were added
	// when basic/go's `if` was found downcasting CheckState.Recorder() to the
	// concrete *EmitState to reach them — the seam existed, it just did not
	// cover branching, so the one caller that needed it punched through.
	TakeFragment() EmitFragmentRef
	RecordBranch(b BranchRecord)
	RecordLoop(start, end, step Value, body EmitFragmentRef, bodyStk []Value, iterID string, out Value, regionN int, pos SrcPos)
	ArmBranchCapture()
	PeekCaptureArm() bool
	ArmLoopCapture()
	ConsumeLoopArm() bool
	SplitLoopRegionBind(name string, v Value) (Value, bool)
	SplitEventRegionBind(name string, v Value) (Value, bool)
	RecordInterpXml(tmpl XmlTmpl, holeVals []Value, out Value, pos SrcPos) bool
	BeginLoopCarried()
	EndLoopCarried()
	NoteLoopCarried(name string, joined, pre Value)
	Checkpoint() EmitCheckpoint
	Rollback(cp EmitCheckpoint)
	CanSeatAcrossFragment(v Value) bool

	// --- fn-unit compilation ---------------------------------------------
	StartFnCompile(key, name string, fnReg *Registry, args []Value, declared []*Type, paramNames []string, captures []CapturedBinding, generic bool, pos SrcPos) (unit int, finish func([]Value), ok bool)
	SetUnitParamTypes(unit int, paramTypes []*Type, paramPatterns []*Value)
	SetUnitReturnPatterns(unit int, returnPatterns []*Value)
	// SetUnitBody seats a fn unit's source body tokens (the sig's Boru body)
	// — what a per-read deopt hands to the interpreter (compiler DeoptSpec).
	SetUnitBody(unit int, body []Value)
	SetUnitDecl(unit int, decl DeclSite)
	UnitVariadic(unit int) bool
	UnitNetsZero(unit int) bool
}

// inactiveEmit is the no-op EmitRecorder a NON-compiling pass runs against:
// every method is the inactive/none answer the corresponding nil-receiver
// *EmitState method returns, so swapping it in for the historical nil field
// is behaviour-identical — and a compile-free check pass touches no
// *EmitState code at all.
type inactiveEmit struct{}

// TheInactiveEmit is the shared no-op recorder instance (it is stateless).
var TheInactiveEmit EmitRecorder = inactiveEmit{}

// Recorder returns the CheckState's emit recorder, never nil: the inactive
// no-op stands in when no recorder is installed (a zero-value CheckState, or
// between passes). All read sites go through this accessor — the field
// itself is written only by the pass entry points and the probe forks.
func (c *CheckState) Recorder() EmitRecorder {
	if c == nil || c.Emit == nil {
		return TheInactiveEmit
	}
	return c.Emit
}

func (inactiveEmit) InClosureUnit() bool                                    { return false }
func (inactiveEmit) StoredGradualActive() bool                              { return false }
func (inactiveEmit) FoldFullStack(string, []Value, []Value) ([]Value, bool) { return nil, false }
func (inactiveEmit) RecordSpliceDyn(Value, SrcPos) bool                     { return false }
func (inactiveEmit) NoteShapedRead(string)                                  {}
func (inactiveEmit) MemberFnReadValue(string) (Value, bool)                 { return Value{}, false }
func (inactiveEmit) Active() bool                                           { return false }
func (inactiveEmit) Armed() bool                                            { return false }
func (inactiveEmit) Suspend() func()                                        { return func() {} }
func (inactiveEmit) BindRegistry(*Registry)                                 {}
func (inactiveEmit) TopFrameOnly() bool                                     { return true }
func (inactiveEmit) SuspendedNow() bool                                     { return false }
func (inactiveEmit) BodyAnalysisGuard() func()                              { return func() {} }
func (inactiveEmit) KeepDefsBodyGuard(*Registry, string) func()             { return func() {} }
func (inactiveEmit) MultiRunBodyGuard(*Registry, string) func()             { return func() {} }
func (inactiveEmit) RecordDynUndef(string, SrcPos)                          {}
func (inactiveEmit) FnBodyGuard() func()                                    { return func() {} }

func (inactiveEmit) TakeFragment() EmitFragmentRef { return nil }
func (inactiveEmit) RecordBranch(BranchRecord)     {}
func (inactiveEmit) RecordLoop(Value, Value, Value, EmitFragmentRef, []Value, string, Value, int, SrcPos) {
}

func (inactiveEmit) MarkUncompilable(string) {}
func (inactiveEmit) SetCatchVariadic(bool)   {}
func (inactiveEmit) PushInlineCtxBoundary()  {}
func (inactiveEmit) PopInlineCtxBoundary()   {}

func (inactiveEmit) RecordDynBind(string, Value, SrcPos) {}
func (inactiveEmit) NoteDefRead(string, string)          {}
func (inactiveEmit) NoteLocalRead(string, SrcPos)        {}
func (inactiveEmit) NoteWordRead(Value, string, SrcPos)  {}
func (inactiveEmit) NoteValRead(string)                  {}
func (inactiveEmit) Sites() map[string]int               { return nil }

func (inactiveEmit) RecordCall(string, *Signature, []Value, []Value, SrcPos, bool, bool) {}
func (inactiveEmit) RecordPoly(string)                                                   {}
func (inactiveEmit) RecordPolyCall(string, []Value, []Value, SrcPos, *Registry, *PolyNoMatchSpec) bool {
	return false
}
func (inactiveEmit) RecordUserCall(int, []Value, []Value, SrcPos) {}
func (inactiveEmit) RecordUserPolyCall(string, *Registry, []int, []int, []SigImpl, []Signature, []Value, []Value, SrcPos) {
}
func (inactiveEmit) RecordDynApply([]Value, Value, Value, SrcPos) (int, bool) { return 0, false }
func (inactiveEmit) RecordDynApplyName(string, []Value, Value, Value, SrcPos) bool {
	return false
}
func (inactiveEmit) DynApplyLeadEligible(Value) bool { return false }
func (inactiveEmit) RecordDynMethod(Value, []Value, []Value, string, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordFallback(FallbackSpan, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RecordTrap(string, string, string, string, SrcPos) bool   { return false }
func (inactiveEmit) RecordTrapErr(*BoruError, SrcPos) bool                    { return false }
func (inactiveEmit) RecordDispatchRematchValues(string, []Value, int, int, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordTypedBind(_ TypedBindSpec, _, out Value, _ SrcPos) (Value, bool) {
	return out, false
}
func (inactiveEmit) RecordMakeList(*Registry, []Value, Value, SrcPos) bool      { return false }
func (inactiveEmit) RecordMakeListInner(*Registry, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RecordMakeMap(*Registry, []string, []Value, bool, Value, SrcPos) bool {
	return false
}
func (inactiveEmit) RecordInterp([]InterpPart, []Value, Value, SrcPos) bool { return false }
func (inactiveEmit) RegisterTrailingApply(string, int)                      {}
func (inactiveEmit) NoteMemberFnRead(string, Value)                         {}
func (inactiveEmit) MemberFnRead(string) bool                               { return false }
func (inactiveEmit) NoteCollectionHazard(string)                            {}
func (inactiveEmit) CollectionHazard(string) bool                           { return false }
func (inactiveEmit) NoteFnResultReStep(Value, SrcPos)                       {}
func (inactiveEmit) DynInputsProven(*Signature, []Value) bool               { return false }
func (inactiveEmit) Materialise(v Value) (Value, bool)                      { return v, false }
func (inactiveEmit) ZeroOutProduced(string) bool                            { return false }
func (inactiveEmit) AlreadyProduced(string) bool                            { return false }

func (inactiveEmit) RecordBindTwin(BindTransition, DefEntry)    {}
func (inactiveEmit) MarkValueDef(Value)                         {}
func (inactiveEmit) RecordDefRebind(string, Value, SrcPos)      {}
func (inactiveEmit) RefuseCarriedUndef(string)                  {}
func (inactiveEmit) NotifyNameRebound(string)                   {}
func (inactiveEmit) NoteFrozenRead(string, FrozenBake, int64)   {}
func (inactiveEmit) RegisterLocal(string) int                   { return -1 }
func (inactiveEmit) RememberOriginal(Value)                     {}
func (inactiveEmit) RememberStrippedOriginals([]Value, []Value) {}

func (inactiveEmit) ArmBranchCapture()                                {}
func (inactiveEmit) PeekCaptureArm() bool                             { return false }
func (inactiveEmit) ArmLoopCapture()                                  {}
func (inactiveEmit) ConsumeLoopArm() bool                             { return false }
func (inactiveEmit) SplitLoopRegionBind(string, Value) (Value, bool)  { return Value{}, false }
func (inactiveEmit) SplitEventRegionBind(string, Value) (Value, bool) { return Value{}, false }

func (inactiveEmit) RecordInterpXml(XmlTmpl, []Value, Value, SrcPos) bool { return false }

func (inactiveEmit) BeginLoopCarried()                    {}
func (inactiveEmit) EndLoopCarried()                      {}
func (inactiveEmit) NoteLoopCarried(string, Value, Value) {}
func (inactiveEmit) Checkpoint() EmitCheckpoint           { return nil }
func (inactiveEmit) Rollback(EmitCheckpoint)              {}
func (inactiveEmit) CanSeatAcrossFragment(Value) bool     { return false }

func (inactiveEmit) StartFnCompile(string, string, *Registry, []Value, []*Type, []string, []CapturedBinding, bool, SrcPos) (int, func([]Value), bool) {
	return -1, nil, false
}
func (inactiveEmit) SetUnitParamTypes(int, []*Type, []*Value) {}
func (inactiveEmit) SetUnitReturnPatterns(int, []*Value)      {}
func (inactiveEmit) SetUnitBody(int, []Value)                 {}
func (inactiveEmit) SetUnitDecl(int, DeclSite)                {}
func (inactiveEmit) UnitVariadic(int) bool                    { return false }
func (inactiveEmit) UnitNetsZero(int) bool                    { return false }

// EmitCheckpoint is the opaque handle for a recording-pool snapshot: the
// checker holds and returns it without any knowledge of the compiler's
// concrete checkpoint contents (the S2 opaque-handle rule). The inactive
// recorder hands out nil; the concrete Rollback ignores anything that is
// not its own snapshot type.
type EmitCheckpoint interface{ isEmitCheckpoint() }

// EmitCheckpointBase is the embeddable marker (the PayloadBase pattern)
// so a compiler-piece checkpoint type outside core satisfies the sealed
// EmitCheckpoint handle.
type EmitCheckpointBase struct{}

func (EmitCheckpointBase) isEmitCheckpoint() {}

// Compile-pass constructor slots (Stage 4b): the pass-arming methods on
// CheckState (BeginCompilePass / IsolateEmit) construct the compiler's
// concrete recorder, which core cannot name — the compiler piece
// installs its constructors here at init (the S9 slot pattern), and the
// inactive fallbacks keep a compiler-less core linkable.
// The fallbacks are NAMED so the seam test can pin them directly: the
// compiler piece's init replaces the slots while it is linked, leaving
// the fallback bodies reachable only on a compiler-less core build
// (the post-cut configuration Stage 5 gates).
func inactiveEmitStateHook() EmitRecorder                { return TheInactiveEmit }
func inactiveIsolatedEmitHook(EmitRecorder) EmitRecorder { return TheInactiveEmit }

var (
	NewEmitStateHook    = inactiveEmitStateHook
	NewIsolatedEmitHook = inactiveIsolatedEmitHook
)

// FrozenBake names WHAT a compiled fn/closure unit froze about a module-scope
// binding it read. It is the argument to NoteFrozenRead, and the whole reason
// that note takes one at all: the freeze discipline defends three DIFFERENT
// baked artifacts, they are repaired by three different mechanisms, and a
// name-keyed bit cannot tell a router which one it is looking at.
//
// The three were not enumerated from the design — each was measured as a
// LIVE, SILENT divergence on the default lane, in that order:
//
//	def k 5        def f fn [[] [Integer] [k add 2]]   f  def k 9       f
//	def T Integer  def f fn [[] [Boolean] [5 is T]]    f  def T String  f
//	def g fn [[][Integer][1]]  def f fn [[][Integer][g]]  f  def g fn [[][Integer][2]]  f
//
// The interpreter answers `7 11`, `true false` and `1 2`; before each arm
// existed the compiled lane answered `7 7`, `true true` and `1 1`.
//
// The zero value is INVALID, per the kernel's "No Zero-Value Overload
// (CRITICAL)" rule: a note that reached no classifier is a defect, not a
// value bake. NoteFrozenRead drops it rather than guessing.
type FrozenBake uint8

const (
	// FrozenBakeNone is the invalid zero.
	FrozenBakeNone FrozenBake = iota
	// FrozenBakeValue — the read's VALUE is interned and lowered to a
	// PUSH_CONST inside the unit. Repaired by making the read live.
	FrozenBakeValue
	// FrozenBakeType — the read is a bare type node lowered to a PUSH_TYPE
	// carrying the node's compile-time IDENTITY. The node stays live; the ID
	// does not, so a rebind of the NAME leaves the unit on the old node.
	FrozenBakeType
	// FrozenBakeCall — the read is a fn NAME whose call the lowering resolved
	// to a specific compiled unit (CALL_USER / TAIL_CALL_USER). Repaired only
	// by a runtime LOOKUP at the call — §6.9's OpDispatchGeneric — because no
	// operand substitution re-resolves a call target the lowering already
	// chose.
	FrozenBakeCall
)

// String names the bake in the refusal a rebind produces, so the diagnostic
// says which artifact went stale rather than always saying "its value".
func (b FrozenBake) String() string {
	switch b {
	case FrozenBakeValue:
		return "value"
	case FrozenBakeType:
		return "type"
	case FrozenBakeCall:
		return "call target"
	}
	return "binding"
}
