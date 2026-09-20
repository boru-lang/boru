package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// User-fn poly dispatch (OpCallUserPoly) — the recorder side.
//
// A gradual-Any arg to a MULTI-overload user fn is an ambiguous dispatch: the
// checker cannot statically commit an overload, and the interpreter re-matches
// at run time. Natives already have a sound runtime re-match (OpCallNativePoly
// / vm.callPoly); this file gives user fns the mirror: bake EVERY same-arity
// overload's body unit and let the VM re-run the interpreter's MatchSignature
// at entry to select the arm. Parity argument: the interpreter's dispatch
// takes the same MatchSignature first-match over the same (live) table, so the
// arm the VM enters is the arm the interpreter splices; any drift or no-match
// bails, and the compiled run reports it.
//
// Everything here is ALL-OR-NOTHING: if any same-arity overload cannot be
// compiled under these rules, the caller keeps the original compile failure
// (MarkUncompilable) and the interpreter owns the program. If in doubt,
// decline — compile == interpret is non-negotiable.

// userPolyPlan is the compiled arm table of one poly user call, handed from
// tryCompileUserPolyArms to the RecordUserPolyCall site: parallel slices of
// the arm's index in the word's aggregated dispatch table, its compiled body
// unit, and its run-implementation identity (the VM's drift guard).
type userPolyPlan struct {
	sigIdx []int
	units  []int
	impls  []core.SigImpl
	// sigs is the FROZEN dispatch table for a BODY-LOCAL word (COMPILE FAILURE-
	// CLOSURE.0 §6b — see UserPolyRef.Sigs): non-nil arms the VM's stored
	// re-match mode, which never Lookups the (popped-before-run) name.
	sigs []core.Signature
	// outs/joined carry the RETURN-JOIN for arm sets whose declared returns
	// DIFFER (completeness-review §8.2(3)): joined[i] marks a position where
	// the arms disagree, and outs[i] is the branch-join carrier (JoinCarriers
	// folded over each arm's declared type) the record site substitutes for
	// the committed overload's carrier — the runtime value is whichever arm
	// the VM's re-match selects, which is exactly a branch join. Positions
	// where every arm agrees keep the committed carrier (joined[i] false),
	// so previously-compiling identical-return sets are byte-identical.
	outs   []core.Value
	joined []bool
}

// tryCompileUserPolyArms attempts the poly compile for one ambiguous
// multi-overload user-fn dispatch: it collects EVERY non-fallback overload of
// `word` whose arity matches the call, gates the set to the shapes the VM's
// runtime re-match handles faithfully, and compiles each arm's body to its
// own unit. Returns nil — leaving the caller to keep the original compile failure —
// when any gate fails or any arm does not compile.
//
// Gates (each keeps compile == interpret):
//   - >= 2 same-arity arms (else the single-overload path already handles it);
//   - every arm is a boru-bodied overload with plain (un-quoted, non-form,
//     non-type-literal) params — the runtime window re-match assumes plain
//     value args, exactly like OpCallNativePoly;
//   - every arm declares the IDENTICAL Returns as the committed overload: the
//     checker's downstream typing rides the committed return carriers, so an
//     arm returning a different type could bake a wrong downstream dispatch
//     (a zero-return contract additionally requires every arm's unit to NET
//     zero — the 0-output call site cannot absorb a "residual IS the result"
//     body);
//   - every arm's owning def entry is capture-free, named, and non-macro —
//     captures ride as hidden trailing CALL_USER operands resolved per call
//     site, which the fixed-arity poly window cannot carry;
//   - no arm is a deferred-param-list body (compiled units cannot model the
//     interpreter's module-scope late binding — see buildFnBodyReturnsFn);
//   - no arm's unit is variadic-returning (the call site bakes a fixed nout).
func tryCompileUserPolyArms(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, committedReturns []*core.Type) *userPolyPlan {
	if !es.Active() || len(args) == 0 {
		return nil
	}
	// An EMPTY committedReturns admits the all-zero-return overload set
	// (COMPILE FAILURE-CLOSURE.0 §6a): userPolyArmShapeOK requires every arm's
	// Returns to match the committed contract position-wise, so 0==0 keeps
	// the arms consistent, and a zero-return call contributes nothing to
	// the residual — no downstream typing exists to diverge. Anonymity
	// (whose declaredReturns is also empty) is still declined below by
	// findOwningFnDef's owner.Anonymous gate.
	// A word bound INSIDE an enclosing fn body (Depth above the innermost fn
	// baseline — the same test closure capture uses) is popped before the VM
	// runs, so the runtime Lookup could never resolve it. Since the §6b
	// landing (COMPILE FAILURE-CLOSURE.0) the plan FREEZES the dispatch table
	// instead of declining: the VM re-matches over the stored signatures
	// (UserPolyRef.Sigs), which are faithful because a body-local fn's
	// construction is source-determined and per-call identical — captures
	// and conditional redefinitions already decline upstream. The one way
	// the live table can still drift from the freeze is DYNAMIC-SCOPE
	// mutation: a callee run between the local def and this call whose own
	// body rebinds the same name overlap-replaces the local IN PLACE (and
	// the replacement survives the callee's teardown — interpreter
	// semantics), which the frozen table cannot see. Gate it: when any
	// OTHER fn in the program binds this name as a body-local
	// (Check.FnBinders — the dynamic-scope attribution map), keep the
	// compile failure and let the interpreter own the shape.
	bodyLocal := false
	if baseline := r.TopFnBaseline(); baseline != nil && r.Defs.Depth(word) > baseline[word] {
		bodyLocal = true
		self := ""
		if n := len(r.Check.FnNameStack); n > 0 {
			self = r.Check.FnNameStack[n-1]
		}
		for fnName := range r.Check.FnBinders[word] {
			if fnName != self {
				return nil
			}
		}
	}
	agg := r.Lookup(word)
	if agg == nil {
		return nil
	}
	var sigIdx []int
	for i := range agg.Signatures {
		s := &agg.Signatures[i]
		if s.Fallback || s.TotalArgs() != len(args) {
			continue
		}
		sigIdx = append(sigIdx, i)
	}
	if len(sigIdx) < 2 {
		return nil
	}
	plan := &userPolyPlan{
		sigIdx: sigIdx,
		units:  make([]int, 0, len(sigIdx)),
		impls:  make([]core.SigImpl, 0, len(sigIdx)),
	}
	for _, si := range sigIdx {
		s := &agg.Signatures[si]
		if !userPolyArmShapeOK(s, committedReturns) {
			return nil
		}
		owner, ok := findOwningFnDef(r, word, s.Impl)
		if !ok || owner.Anonymous || owner.Macro || len(owner.Captured) != 0 {
			return nil
		}
		unit, ok := compileUserPolyArm(r, es, word, s, owner)
		if !ok {
			return nil
		}
		// A ZERO-declared-return set records a 0-output call site, so the
		// VM-selected arm must net exactly zero residual values. A declared-[]
		// arm whose body leaves a residual is the interpreter's "residual IS
		// the result" shape — a fixed nout of 0 cannot carry it, so the whole
		// set keeps its compile failure (all-or-nothing).
		if len(committedReturns) == 0 && !es.UnitNetsZero(unit) {
			return nil
		}
		plan.units = append(plan.units, unit)
		plan.impls = append(plan.impls, s.Impl)
	}
	// A body-local word's binding is gone at VM time: freeze the aggregate's
	// arm signatures so matchUserPoly re-matches over the stored table (the
	// aggregate is built fresh per check-mode Lookup, so the value copies
	// alias nothing that mutates).
	if bodyLocal {
		plan.sigs = make([]core.Signature, 0, len(sigIdx))
		for _, si := range sigIdx {
			plan.sigs = append(plan.sigs, agg.Signatures[si])
		}
	}
	// DIFFERING arm returns record the position-wise JOIN (§8.2(3)): the
	// VM's re-match selects one arm at run time, so the call's result is a
	// branch join of the arms' declared types — fold the same JoinCarriers
	// the if/loop merges use, yielding the distributing strict-Disjunct (or
	// collapsed-parent) carrier downstream dispatch already partitions
	// (TestDistributeOverDispatchInvariant). A joined Any goes DYNAMIC,
	// mirroring the committed-out convention ("statically unknown", not the
	// strict Any root). Positions where every arm agrees stay untouched.
	plan.outs = make([]core.Value, len(committedReturns))
	plan.joined = make([]bool, len(committedReturns))
	for pos, ct := range committedReturns {
		// A nil position agrees across every arm (userPolyArmShapeOK's
		// nil-ness gate), so it always takes the allEqual path — the nil
		// probe rides inside the comparison so the join below only ever
		// sees non-nil types.
		allEqual := true
		for _, si := range sigIdx {
			rt := agg.Signatures[si].Returns[pos]
			if (rt == nil) != (ct == nil) || (rt != nil && !rt.Equal(ct)) {
				allEqual = false
				break
			}
		}
		if allEqual {
			continue
		}
		bound := agg.Signatures[sigIdx[0]].Returns[pos]
		for _, si := range sigIdx[1:] {
			bound = core.CommonAncestorType(bound, agg.Signatures[si].Returns[pos])
		}
		j := core.NewCarrier(bound)
		// DYNAMIC at the join bound — "one of the arms' types, decided at run
		// time" — the same gradual shape a mixed branch merge or a dynamic
		// native poly result carries; downstream dispatch matches it
		// optimistically and re-checks at run time. (A strict Disjunct
		// carrier would distribute more precisely, but the partition
		// machinery re-mints the result carrier and orphans the recorded
		// event's identity — the elision then misses and the dispatch
		// re-declines generically.)
		j.Dynamic = true
		plan.outs[pos] = j
		plan.joined[pos] = true
	}
	return plan
}

// SubstituteJoinedOuts replaces the committed overload's out carriers with
// the plan's return-join carriers at every position where the arms' declared
// returns differ. The record site calls it just before RecordUserPolyCall so
// downstream typing rides the join, never one arm's unproven commitment.
func (p *userPolyPlan) SubstituteJoinedOuts(out []core.Value) {
	for i := range out {
		if i < len(p.joined) && p.joined[i] {
			out[i] = p.outs[i]
		}
	}
}

// The UserPolyPlan handle accessors (dispatch_hooks.go): the check
// piece's record sites unpack the plan through these instead of naming
// the concrete type.
func (p *userPolyPlan) SigIdx() []int          { return p.sigIdx }
func (p *userPolyPlan) Units() []int           { return p.units }
func (p *userPolyPlan) Impls() []core.SigImpl  { return p.impls }
func (p *userPolyPlan) Sigs() []core.Signature { return p.sigs }

// userPolyArmShapeOK gates one arm's SIGNATURE shape: a boru body, plain
// value params (no quote / raw-form / no-eval / type-literal slots — the
// runtime window re-match binds plain values only), and Returns matching the
// committed overload's in COUNT and position-wise declaredness (nil-ness).
// The TYPES may differ (the §8.2(3) return-join graduation): the call site
// bakes a fixed nout, so the count must agree, but a differing type joins —
// tryCompileUserPolyArms records the branch join of the arms' returns and
// the VM re-match keeps each arm's own return contract at its unit.
func userPolyArmShapeOK(s *core.Signature, committedReturns []*core.Type) bool {
	if len(s.Body()) == 0 {
		return false
	}
	if len(s.QuoteArgs) != 0 || len(s.TypeArgs) != 0 ||
		len(s.NoEvalArgs) != 0 || len(s.NoEvalMapArgs) != 0 ||
		len(s.RawParens) != 0 || len(s.FormArgs) != 0 {
		return false
	}
	for i := range s.Params {
		if s.Params[i].Quote {
			return false
		}
	}
	if len(s.Returns) != len(committedReturns) {
		return false
	}
	for i, t := range s.Returns {
		if (t == nil) != (committedReturns[i] == nil) {
			return false
		}
	}
	return true
}

// findOwningFnDef locates the def-stack entry whose own overloads include the
// signature with the given run-implementation identity (Signature.Impl is a
// stable pointer per installed overload, surviving aggregateDispatch's value
// copies). The entry carries the per-def metadata the arm compile needs (Gen,
// Captured, Anonymous, Macro) that the aggregate view drops or takes from the
// newest entry only. ok=false when no entry owns it (a synthetic sig).
func findOwningFnDef(r *core.Registry, word string, impl core.SigImpl) (core.FnDefInfo, bool) {
	if impl == nil {
		return core.FnDefInfo{}, false
	}
	stack := r.Defs.Stack(word)
	for i := len(stack) - 1; i >= 0; i-- {
		fnDef, ok := stack[i].Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for j := range fnDef.Signatures {
			if fnDef.Signatures[j].Impl == impl {
				return fnDef, true
			}
		}
	}
	return core.FnDefInfo{}, false
}

// compileUserPolyArm compiles ONE overload's body as its own code unit,
// exactly as the single-overload path in buildFnBodyReturnsFn does
// (StartFnCompile keyed on the arm's generalised args, SetUnitParamTypes with
// the arm's declared params, AnalyseFnBody + finish) — but against carriers
// of the arm's DECLARED param types rather than the call's args: the call's
// gradual args say nothing about which arm runs, while the declared types are
// the contract the VM's entry guard (checkParamContract) and the runtime
// re-match both enforce. A generic arm installs its gen bindings around the
// analysis (the placeholder nodes are parented at their bounds, so the body
// dispatches against the constraint — sound: the entry guard re-checks the
// runtime value against the same placeholder membership). Returns ok=false on
// any compile failure, leaving the caller to keep the original MarkUncompilable.
func compileUserPolyArm(r *core.Registry, es core.EmitRecorder, word string, s *core.Signature, owner core.FnDefInfo) (int, bool) {
	body := append([]core.Value(nil), s.Body()...)
	if len(body) == 0 {
		return -1, false
	}
	sigParams := append([]core.FnParam(nil), s.Params...)
	paramNames := make([]string, len(sigParams))
	genArgs := make([]core.Value, len(sigParams))
	pts := make([]*core.Type, len(sigParams))
	pats := make([]*core.Value, len(sigParams))
	for i, p := range sigParams {
		paramNames[i] = p.Name
		genArgs[i] = check.ParamBodyCarrier(p)
		pts[i] = p.Type
		pats[i] = p.Pattern
	}
	// A deferred-param-list body returns its raw list for MODULE-scope late
	// evaluation (see buildFnBodyReturnsFn) — a unit's call-time result cannot
	// model that, so the arm (and with it the whole poly set) declines.
	if _, deferred := check.DeferredParamListResidual(body, paramNames); deferred {
		return -1, false
	}
	declared := append([]*core.Type(nil), s.Returns...)
	var genNames []string
	if owner.Gen != nil {
		genNames = core.InstallGenBindingMap(r, owner.Gen, core.InferGenBindings(owner.Gen, sigParams, genArgs))
	}
	defer func() {
		for i := len(genNames) - 1; i >= 0; i-- {
			r.Defs.Pop(genNames[i])
		}
	}()
	key := check.FnAnalysisKey(r.AnalysisScopeID(), word, genArgs, owner.Captured, body)
	fnPos := body[0].Pos()
	unit, finishFn, ok := es.StartFnCompile(key, word, r, genArgs, declared, paramNames, owner.Captured, owner.Gen != nil, fnPos)
	if !ok || unit < 0 {
		return -1, false
	}
	// Declared PARAM types/patterns: the VM enforces them at entry (the same
	// guard OpCallUser runs), so the runtime re-match's pick is double-checked
	// against the arm's own contract.
	es.SetUnitParamTypes(unit, pts, pats)
	es.SetUnitReturnPatterns(unit, s.ReturnPatterns)
	es.SetUnitDecl(unit, s.Decl)
	if finishFn != nil {
		// Drop any summary cached by a prior plain analysis so AnalyseFnBody
		// re-runs and records the body into THIS unit (the same memo-key
		// discipline as the single-overload path).
		delete(r.Check.FnSummaries, key)
		stk := check.AnalyseFnBody(r, word, paramNames, body, genArgs, owner.Captured, declared, owner.Anonymous)
		finishFn(stk)
	}
	if !es.Active() { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return -1, false
	}
	if es.UnitVariadic(unit) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return -1, false
	}
	return unit, true
}
