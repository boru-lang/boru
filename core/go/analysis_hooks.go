package core

// The analysis accessor layer — Stage 2a of the four-piece split
// (design/legacy/ENG-FOUR-PIECE.0.ignore seam S1). Every consultation of the
// checker's state that the PURE INTERPRETER makes routes through the
// small surface below, so the core piece's files carry no direct
// CheckState knowledge: this one file concentrates the coupling, and
// its bodies become AnalysisHooks interface calls (with a cached
// activity bool for the hot loop) when the packages cut. Behavior is
// byte-identical to the direct field access it replaces.

import "fmt"

// analysisActive reports whether an analysis pass (check mode) is on —
// the hot-loop probe (Registry.Lookup, the step loop, dispatch).
func (r *Registry) analysisActive() bool { return r.Check.IsActive() }

// analysisRecorder is the emit-recorder reach; never nil (the inactive
// no-op recorder backs a plain pass).
func (r *Registry) analysisRecorder() EmitRecorder { return r.Check.Recorder() }

// analysisMode reports the plain-check flag (Check.Mode) — the
// carrier-modeling probes distinct from full activity.
func (r *Registry) analysisMode() bool { return r.Check.Mode }

// analysisCompiling reports a compile (recording) pass.
func (r *Registry) analysisCompiling() bool { return r.Check.Compiling }

// noteAnalysisDiagnostic forwards a diagnostic to the checker.
func (r *Registry) noteAnalysisDiagnostic(d CheckDiagnostic) { r.Check.AddDiagnostic(d) }

// noteAnalysisUse marks a name as read (unused-def accounting).
func (r *Registry) noteAnalysisUse(name string) { r.Check.recordUse(name) }

// noteAnalysisFnBinder attributes a body-local def to its enclosing fn
// for the dynamic-scope undefined-word rescue (check mode only; no-op
// at the top level or outside check).
func (r *Registry) noteAnalysisFnBinder(name string) { r.Check.RecordFnBinder(name) }

// analysisInCondBody reports whether the analysed position sits inside
// a conditional body (branch/loop) — the compile-soundness probe for
// overload redefinition.
func (r *Registry) analysisInCondBody() bool { return r.Check.CondBodyDepth > 0 }

// analysisInSpecArm reports analysis inside a branch arm whose condition
// the model cannot decide (CheckState.SpecArmDepth): the arms a fn def is
// speculative in (the seventieth increment).
func (r *Registry) analysisInSpecArm() bool { return r.Check.SpecArmDepth > 0 }

// EnterSpecArm brackets the analysis of a branch arm; the returned func
// leaves it. known is whether the model decides the condition (a literal
// Boolean — the arm runs or not, exactly as the model has it): a known
// arm takes no bracket, an undecidable one raises SpecArmDepth.
func (r *Registry) EnterSpecArm(known bool) func() {
	if known {
		return func() {}
	}
	r.Check.SpecArmDepth++
	return func() { r.Check.SpecArmDepth-- }
}

// analysisSnapshot captures the checker's per-call state for the
// predicate sandbox; restoreAnalysisSnapshot rolls it back IN PLACE
// (not by swapping the pointer) so a module sub-registry transiently
// sharing the state observes the rollback too
// (design/legacy/module-fn-checkstate-ownership.1.ignore §3.2).
func (r *Registry) analysisSnapshot() *CheckState { return r.Check.Clone() }

func (r *Registry) restoreAnalysisSnapshot(s *CheckState) {
	if s != nil && r.Check != nil {
		*r.Check = *s
	} else {
		r.Check = s
	}
}

// noteSuppressedRuntimeError latches the checker's runtime-suppression
// flag (a would-be runtime error absorbed by the model).
func (r *Registry) noteSuppressedRuntimeError() { r.Check.SuppressedRuntimeError = true }

// noteAmbiguousGradualSplit latches the gradual-split ambiguity flag.
func (r *Registry) noteAmbiguousGradualSplit() { r.Check.AmbiguousGradualSplit = true }

// analysisStepMeter advances the check-mode step budget. It reports
// EXCEEDED (the run should stop) after emitting the one budget
// diagnostic; a run with analysis off never trips.
func (r *Registry) analysisStepMeter() (exceeded bool) {
	if !r.Check.IsActive() {
		return false
	}
	// -1 is the "unset" sentinel; resolve to the project default. A
	// literal 0 is honored as "abort immediately" rather than treated
	// as a magic "use default."
	budget := r.Check.StepBudget
	if budget == -1 {
		budget = DefaultCheckStepBudget
	}
	r.Check.StepCount++
	if r.Check.StepCount <= budget {
		return false
	}
	if !r.Check.BudgetTripped {
		r.Check.BudgetTripped = true
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "step_budget_exceeded",
			Detail: fmt.Sprintf("check mode aborted: step budget of %d exceeded", budget),
		})
	}
	return true
}

// analysisFnConstructionPass runs the check piece's construction-time
// static body pass for a newly installed fn (no-op outside check mode).
func (r *Registry) analysisFnConstructionPass(name string, fnDef FnDefInfo) {
	AnalysisImpl.FnConstructionPass(r, name, fnDef)
}

// analysisReturnsFn builds the check piece's per-signature analysis
// model (the ReturnsFunc baked onto every boru-bodied signature).
func (r *Registry) analysisReturnsFn(name string, s FnSig, fnDef FnDefInfo) ReturnsFunc {
	return AnalysisImpl.ReturnsFn(r, name, s, fnDef)
}

// analysisStripToCarriers replaces concrete run inputs with their
// check-mode carriers at the Run boundary (active analysis only —
// callers gate).
func (r *Registry) analysisStripToCarriers(in []Value) []Value {
	return AnalysisImpl.StripToCarriers(in)
}

// analysisZeroOutResiduals drops phantom 0-output statement residues
// from the top-level residual (recording passes).
func (r *Registry) analysisZeroOutResiduals(stk []Value) []Value {
	return AnalysisImpl.ZeroOutResiduals(r, stk)
}

// analysisCarrierResults models a matched dispatch's results under
// analysis — the carrier-propagation seam of execMatch.
func (r *Registry) analysisCarrierResults(word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry, tailConsumed bool) []Value {
	return AnalysisImpl.CarrierResults(r, word, sig, args, pos, ownerReg, tailConsumed)
}

// analysisMixedConform reports whether v is a genuinely MIXED gradual
// carrier conforming to t (matchSignature's gradual arm).
func (r *Registry) analysisMixedConform(v Value, t *Type) bool {
	return AnalysisImpl.MixedConform(v, t)
}

// analysisValueCarriesCarrier reports whether v transitively contains
// a carrier (the interp-string dynamic-collapse probe).
func (r *Registry) analysisValueCarriesCarrier(v Value) bool {
	return AnalysisImpl.ValueCarriesCarrier(v)
}

// analysisAtUncaughtTopLevel reports unconditional top-level reach
// outside any error-trapping region (the guaranteed-error mirrors'
// reachability gate).
func (r *Registry) analysisAtUncaughtTopLevel() bool { return AnalysisImpl.AtUncaughtTopLevel(r) }

// noteAnalysisUniqueDiagnostic forwards a diagnostic deduped against
// the accumulated list (code+detail+word+position). No longer a slot:
// the dedupe itself moved down here with the carrier lattice
// (check_state.go), so there is nothing left for the check piece to
// install.
func (r *Registry) noteAnalysisUniqueDiagnostic(d CheckDiagnostic) { CheckAddUnique(r, d) }

// AnalysisImpl is the S1 implementation table: the check piece installs
// the real analysis operations at init (installAnalysisImpl,
// check_recovery.go), and the accessor bodies above dispatch through
// it. The inactive defaults keep a check-less core linkable and are the
// exact no-ops an inactive pass produces; TestInactiveAnalysisImpl pins
// them.
var AnalysisImpl = struct {
	FnConstructionPass  func(r *Registry, name string, fnDef FnDefInfo)
	ReturnsFn           func(r *Registry, name string, s FnSig, fnDef FnDefInfo) ReturnsFunc
	StripToCarriers     func(in []Value) []Value
	ZeroOutResiduals    func(r *Registry, stk []Value) []Value
	CarrierResults      func(r *Registry, word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry, tailConsumed bool) []Value
	MixedConform        func(v Value, t *Type) bool
	ValueCarriesCarrier func(v Value) bool
	AtUncaughtTopLevel  func(r *Registry) bool
	AnalyseFnBody       func(r *Registry, name string, paramNames []string, body []Value, args []Value, captures []CapturedBinding, declared []*Type, anonymous bool) []Value
	AnalyseLoopBody     func(r *Registry, body Value, bindNames []string, bindVals []Value, provenTrips bool) []Value
}{
	FnConstructionPass:  inactiveFnConstructionPass,
	ReturnsFn:           inactiveReturnsFn,
	StripToCarriers:     inactiveStripToCarriers,
	ZeroOutResiduals:    inactiveZeroOutResiduals,
	CarrierResults:      inactiveCarrierResults,
	MixedConform:        inactiveMixedConform,
	ValueCarriesCarrier: inactiveValueCarriesCarrier,
	AtUncaughtTopLevel:  inactiveAtUncaughtTopLevel,
	AnalyseFnBody:       inactiveAnalyseFnBody,
	AnalyseLoopBody:     inactiveAnalyseLoopBody,
}

// The inactive defaults are NAMED so the seam test pins them directly;
// the check piece's init replaces the table while it is linked, leaving
// these reachable only on a check-less core build.
func inactiveFnConstructionPass(*Registry, string, FnDefInfo)           {}
func inactiveReturnsFn(*Registry, string, FnSig, FnDefInfo) ReturnsFunc { return nil }
func inactiveStripToCarriers(in []Value) []Value                        { return in }
func inactiveZeroOutResiduals(_ *Registry, stk []Value) []Value         { return stk }
func inactiveCarrierResults(*Registry, string, *Signature, []Value, SrcPos, *Registry, bool) []Value {
	return nil
}
func inactiveMixedConform(Value, *Type) bool    { return false }
func inactiveValueCarriesCarrier(Value) bool    { return false }
func inactiveAtUncaughtTopLevel(*Registry) bool { return false }
func inactiveAnalyseFnBody(*Registry, string, []string, []Value, []Value, []CapturedBinding, []*Type, bool) []Value {
	return nil
}
func inactiveAnalyseLoopBody(*Registry, Value, []string, []Value, bool) []Value { return nil }

// RunFnBodyAnalysis and RunLoopBodyAnalysis are the EXPORTED accessors
// for the two body-model slots — the S1 twins of the EmitRecorder seam,
// and for the same reason: the consumer is outside this module. A word
// library implementing `fn` / `for` has an analysis half that must
// re-enter the pass over the body, but the pass itself (memoisation,
// recursion bailing, the per-call-shape quota, the Kleene fixed point)
// is genuinely the checker's and stays there. Everything the two slots
// take and return is a core type, so the seam names no check symbol.
//
// A nil return is IN-BAND, not a failure signal invented for the
// check-less build: AnalyseFnBody already documents an empty result as
// "the analyser aborted — treat as an Any carrier", and every caller
// handles it. On a check-less build the whole carrier regime is
// inert anyway (ReturnsFn returns nil, so no ReturnsFunc runs and
// neither accessor is reached), which is what makes these defaults the
// same no-analysis regime the other slots already define rather than
// wrong analysis.
func RunFnBodyAnalysis(r *Registry, name string, paramNames []string, body []Value,
	args []Value, captures []CapturedBinding, declared []*Type, anonymous bool) []Value {
	return AnalysisImpl.AnalyseFnBody(r, name, paramNames, body, args, captures, declared, anonymous)
}

func RunLoopBodyAnalysis(r *Registry, body Value, bindNames []string, bindVals []Value,
	provenTrips bool) []Value {
	return AnalysisImpl.AnalyseLoopBody(r, body, bindNames, bindVals, provenTrips)
}

// NoteFnBodyPending queues a constructed fn VALUE for the end-of-pass body
// check. The word library calls it where the value is built — `fn`, `afn`,
// and the `=>` lambda that lowers to `afn` — and that is the only place an
// anonymous callback can be caught at all: it never reaches InstallFnDef,
// so until this it was analysed by nobody (NUR105).
//
// It QUEUES rather than analyses because the construction site is too early.
// Every forward reference is still unbound there, a recursive self-call most
// of all, so analysing `def fact fn [… [n mul (fact (n sub 1))]]` in place
// sees the undefined-word placeholder and reports `mul: got (Integer,
// Atom)`. End of pass is where the environment is complete; see
// CheckState.PendingFnBodies.
func NoteFnBodyPending(r *Registry, fnDef FnDefInfo) {
	NoteFnBodyPendingIn(r, r, fnDef)
}

// NoteFnBodyPendingIn queues a fn VALUE written in reg for the end-of-pass
// body check of owner's pass: a module's exported fn is queued at EXPORT
// time on the importing pass (owner), to be analysed in the module
// registry (reg) it was written in — the declaration-shaped run a module
// fn never got, because a module body runs with its own check inactive
// (its exports need concrete names), so its dead branches were reported
// only when someone called it (NUR128). The drain shares owner's check
// state into reg for the analysis (CheckBraid.ShareCheckStateFrom).
func NoteFnBodyPendingIn(owner, reg *Registry, fnDef FnDefInfo) {
	if owner == nil || reg == nil || !owner.Check.IsActive() {
		return
	}
	owner.Check.PendingFnBodies = append(owner.Check.PendingFnBodies, PendingFnBody{Reg: reg, Fn: fnDef})
}

// RunPendingFnBodyChecks drains the queue, at end of pass and before the
// forward-reference rescue so the drain's own diagnostics are rescued too.
//
// A queued fn that reached InstallFnDef in the meantime was analysed there,
// under its NAME — which the dynamic-scope rescue and the return-conformance
// messages both key on — and FnBodyChecked keeps this from repeating it. What
// is left is exactly the set of fn values nothing ever named.
func RunPendingFnBodyChecks(r *Registry) {
	if r == nil || !r.Check.IsActive() {
		return
	}
	// Draining can itself construct fn values (a body analysis runs `fn`),
	// so take the queue and loop until it stops growing rather than ranging
	// over a slice being appended to.
	for len(r.Check.PendingFnBodies) > 0 {
		batch := r.Check.PendingFnBodies
		r.Check.PendingFnBodies = nil
		for _, pb := range batch {
			// In the registry the body was WRITTEN in: a handler lambda inside
			// an imported module reads that module's own words, which the
			// importer's registry cannot see. A module registry's own check
			// is inactive (NUR128): share this pass's state into it for
			// the analysis, so the module fn's diagnostics land here.
			// Under its NAME when it has one (an exported module fn): the
			// dynamic-scope rescue and the return-conformance messages key
			// on the reading fn's name, and a nameless analysis loses them
			// (NUR105's first discovery). A body analysed in ANOTHER
			// registry (an exported module fn, NUR128) is a declaration-
			// shaped run over the module's own scope with the importer's
			// state — speculative where the two disagree — so only its
			// STRUCTURAL findings are kept (a dead branch, the class the
			// record is about); a name or a dispatch it cannot resolve
			// there reports nothing, as NUR105's third discovery rules.
			before := len(r.Check.Diagnostics)
			restore := CheckBraid.ShareCheckStateFrom(pb.Reg, r)
			// Past the pass's call-shape summaries of the same fn: the
			// declaration-shaped run is the one entitled to report.
			r.Check.ForceFnReanalysis = pb.Reg != r
			AnalysisImpl.FnConstructionPass(pb.Reg, pb.Fn.Name, pb.Fn)
			r.Check.ForceFnReanalysis = false
			restore()
			if pb.Reg != r {
				kept := r.Check.Diagnostics[:before]
				for _, d := range r.Check.Diagnostics[before:] {
					if d.Code == "unreachable_branch" {
						kept = append(kept, d)
					}
				}
				r.Check.Diagnostics = kept
			}
		}
	}
}
