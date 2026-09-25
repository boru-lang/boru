package core

// Engine dispatch-hook slots — Stage 3b of the four-piece split
// (design/legacy/ENG-FOUR-PIECE.0.ignore seam S9). A compiler-piece behavior the
// core step loop must be able to OFFER without naming compiler symbols
// registers itself here at init; a nil slot simply declines. At the
// package cut these become the compiler's registrations onto core's
// exported hook points.

// DriftWindowRecorder is the compiler's stack-drift island hook: offered
// a matched dispatch whose forward window drifted, it may record the
// window as a runtime island (drift_window.go) and report true to skip
// the compile failure path. The compiler piece installs the real recorder at
// init; the NAMED default below is what a compiler-less build runs, so
// the decline path is reachable and pinned like every other seam slot
// (TestInactiveDriftWindowRecorder).
var DriftWindowRecorder = inactiveDriftWindowRecorder

func inactiveDriftWindowRecorder(*Engine, WordInfo, *Signature, []int) bool { return false }

// RegionRecorder is the compiler's region-descriptor hook: offered every
// forward-collecting dispatch, with the window, the registry, the dispatching
// word and its INDEX in that window — everything CaptureRegionSlots needs to
// read a region's extent and written order.
//
// It fires at collection time on purpose, because that is the only place the
// extent is knowable. A region's two halves are known at DIFFERENT times, and
// this is the earlier one:
//
//   - HERE: the extent and the written-order Tokens, which are properties of
//     the tape. No operands exist yet.
//   - LATER, at the recorder's own call site: SlotDesc.Source — which values
//     are consts, frame locals, prior events, compiled fragments. No tape
//     index exists there.
//
// The two are joined on (word, SrcPos), which both sides already hold; it was
// measured across the corpus rather than assumed. Two asymmetries came with
// that measurement and the compiler side must honour them: not every recorded
// call has a region (`concat "a" "b"` never reaches forward collection, and a
// region is by definition a forward-collecting dispatch), and not every region
// becomes a recorded call (`def` is a check-mode word). Neither is an error.
//
// The compiler installs the real recorder at init; the NAMED default below is
// what a compiler-less build runs, so the decline path is reachable and pinned
// like every other slot (TestInactiveRegionRecorder).
var RegionRecorder = inactiveRegionRecorder

func inactiveRegionRecorder(CollectWindow, *Registry, WordInfo, int) {}

// CheckBraid is the S9 dispatch-hook table for the check piece's
// dispatch-recovery braid: the step loop OFFERS each recovery/model
// point through these slots, and the check piece installs its
// implementations at init (installCheckBraid, check_recovery.go). The
// slots are nil only on a core build linked without the check piece —
// a configuration where analysis mode cannot be meaningfully armed.
var CheckBraid = struct {
	CheckMixedFormAdvisories     func(e *Engine, w WordInfo, sig *Signature, positions []int, pos SrcPos, fwdCount, stkCount int)
	CheckModeAssumeSig           func(e *Engine, w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error
	CheckModeFallbackPositions   func(e *Engine, n int) []int
	CheckModeParenFnCollapse     func(e *Engine, openIdx, closeIdx int) int
	CheckModeSurfaceShape        func(e *Engine, w WordInfo, pos SrcPos) (bool, error)
	ConcreteEvalOnce             func(e *Engine, items []Value) (Value, bool)
	DrainUndefinedAtoms          func(e *Engine)
	ExprRefsCarrier              func(e *Engine, items []Value) bool
	NoteSpeculativeBarrierCommit func(e *Engine, fwd ForwardInfo)
	DeclineForwardStackDrift     func(e *Engine, sig *Signature, positions []int)
	DeclineStrandedMemberFn      func(e *Engine, positions []int)
	ShareCheckState              func(e *Engine, capturedReg *Registry) func()
	// ShareCheckStateFrom points owner's Check at caller's for the returned
	// restore's lifetime (the same transient sharing ShareCheckState does
	// for a dispatch): the end-of-pass drain analyses an exported module
	// fn's body in its own registry under the importing pass (NUR128).
	ShareCheckStateFrom        func(owner, caller *Registry) func()
	SpliceAnonCheckResult      func(e *Engine, valIdx, nArgs int, sig *FnSig, args []Value, captures []CapturedBinding) error
	SpliceCheckResults         func(e *Engine, positions []int, results []Value)
	SpliceFnValueCheckResult   func(e *Engine, valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error
	TagCheckModeDefRead        func(e *Engine, top *Value, name string, pos SrcPos)
	TryDynamicFnValueDispatch  func(e *Engine, valIdx int) bool
	TryMemberFnArrivalDispatch func(e *Engine, valIdx int) bool
	NoteReStepLanding          func(e *Engine, valIdx int)
	// ParenPlacedFnCarrier reports whether the value at idx is an
	// analysis-pass carrier the check side knows to be a FUNCTION (a
	// pinpointed member-fn read, whose fn identity lives in the recorder's
	// side table rather than the value's type). fnReturnPark asks it so a
	// user paren places such a carrier exactly as the interpreter places
	// the concrete Function it stands for (NUR073's BROAD park).
	ParenPlacedFnCarrier func(e *Engine, idx int) bool
	// NoteStrandedTypeCall judges the finished TOP-level residual for the
	// call that never happened: a capitalised `def` given a fn body binds a
	// TYPE, so the name in call position places its lattice node and leaves
	// the operands after it unconsumed, exit 0 and all
	// (design/legacy/HIGHER-ORDER-FUNCTIONS.0.ignore §5.1). Offered the reconciled
	// residual — the exact list CheckResult.Stack reports — so the judgement
	// reads what the user is shown.
	NoteStrandedTypeCall    func(e *Engine, residual []Value)
	TryShapedMethodDispatch func(e *Engine, valIdx int) bool
	UndefinedWordCheckDiag  func(e *Engine, name string, pos SrcPos) CheckDiagnostic
}{
	CheckMixedFormAdvisories:     inactiveCheckMixedFormAdvisories,
	CheckModeAssumeSig:           inactiveCheckModeAssumeSig,
	CheckModeFallbackPositions:   inactiveCheckModeFallbackPositions,
	CheckModeParenFnCollapse:     inactiveCheckModeParenFnCollapse,
	CheckModeSurfaceShape:        inactiveCheckModeSurfaceShape,
	ConcreteEvalOnce:             inactiveConcreteEvalOnce,
	DrainUndefinedAtoms:          inactiveDrainUndefinedAtoms,
	ExprRefsCarrier:              inactiveExprRefsCarrier,
	NoteSpeculativeBarrierCommit: inactiveNoteSpeculativeBarrierCommit,
	DeclineForwardStackDrift:     inactiveDeclineForwardStackDrift,
	DeclineStrandedMemberFn:      inactiveDeclineStrandedMemberFn,
	ShareCheckState:              inactiveShareCheckState,
	ShareCheckStateFrom:          inactiveShareCheckStateFrom,
	SpliceAnonCheckResult:        inactiveSpliceAnonCheckResult,
	SpliceCheckResults:           inactiveSpliceCheckResults,
	SpliceFnValueCheckResult:     inactiveSpliceFnValueCheckResult,
	TagCheckModeDefRead:          inactiveTagCheckModeDefRead,
	TryDynamicFnValueDispatch:    inactiveTryDynamicFnValueDispatch,
	TryMemberFnArrivalDispatch:   inactiveTryMemberFnArrivalDispatch,
	NoteReStepLanding:            inactiveNoteReStepLanding,
	ParenPlacedFnCarrier:         inactiveParenPlacedFnCarrier,
	NoteStrandedTypeCall:         inactiveNoteStrandedTypeCall,
	TryShapedMethodDispatch:      inactiveTryShapedMethodDispatch,
	UndefinedWordCheckDiag:       inactiveUndefinedWordCheckDiag,
}

// The inactive defaults are NAMED so the seam test pins them: a core
// build without the check piece runs them as the exact inactive no-ops
// (identity for the paren collapse, empty restores, zero results).
func inactiveCheckMixedFormAdvisories(e *Engine, w WordInfo, sig *Signature, positions []int, pos SrcPos, fwdCount, stkCount int) {
}

func inactiveCheckModeAssumeSig(e *Engine, w WordInfo, fn *FnDefInfo, fallback *Signature, pos SrcPos) error {
	return nil
}

func inactiveCheckModeFallbackPositions(e *Engine, n int) []int { return nil }

func inactiveCheckModeParenFnCollapse(e *Engine, openIdx, closeIdx int) int { return closeIdx }

func inactiveCheckModeSurfaceShape(e *Engine, w WordInfo, pos SrcPos) (bool, error) {
	return false, nil
}

func inactiveConcreteEvalOnce(e *Engine, items []Value) (Value, bool) { return Value{}, false }

func inactiveDrainUndefinedAtoms(e *Engine) {}

func inactiveNoteStrandedTypeCall(e *Engine, residual []Value) {}

func inactiveExprRefsCarrier(e *Engine, items []Value) bool { return false }

func inactiveNoteSpeculativeBarrierCommit(e *Engine, fwd ForwardInfo) {}

func inactiveDeclineForwardStackDrift(e *Engine, sig *Signature, positions []int) {}

func inactiveDeclineStrandedMemberFn(e *Engine, positions []int) {}

func inactiveNoteReStepLanding(e *Engine, valIdx int) {}

func inactiveShareCheckState(e *Engine, capturedReg *Registry) func() { return func() {} }
func inactiveShareCheckStateFrom(owner, caller *Registry) func()      { return func() {} }

func inactiveSpliceAnonCheckResult(e *Engine, valIdx, nArgs int, sig *FnSig, args []Value, captures []CapturedBinding) error {
	return nil
}

func inactiveSpliceCheckResults(e *Engine, positions []int, results []Value) {}

func inactiveSpliceFnValueCheckResult(e *Engine, valIdx, nArgs int, fnDef FnDefInfo, sig *FnSig, args []Value) error {
	return nil
}

func inactiveTagCheckModeDefRead(e *Engine, top *Value, name string, pos SrcPos) {}

func inactiveTryDynamicFnValueDispatch(e *Engine, valIdx int) bool { return false }

func inactiveTryMemberFnArrivalDispatch(e *Engine, valIdx int) bool { return false }

// inactiveParenPlacedFnCarrier is the NAMED inactive default for
// ParenPlacedFnCarrier (core/go/CLAUDE.md: every seam slot has one).
func inactiveParenPlacedFnCarrier(e *Engine, idx int) bool { return false }

func inactiveTryShapedMethodDispatch(e *Engine, valIdx int) bool { return false }

func inactiveUndefinedWordCheckDiag(e *Engine, name string, pos SrcPos) CheckDiagnostic {
	return CheckDiagnostic{}
}
