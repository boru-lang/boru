package check

import core "github.com/boru-lang/boru/core/go"

// method_shape.go — the shaped-instance-method dispatch model (Phase 6
// Stage M2c, design/STAGE3-INLINING-DESIGN-ROUND.0.md §6 M2c).
//
// A module word like Log.with / Log.counter / Rand.with-seed returns an
// INSTANCE — a Map of trivial-delegation method wrappers closing over
// per-instance state. Its check-mode ReturnsFn builds a SHAPE-ONLY twin
// (state-independent method names + signatures) so the dot read resolves;
// but the runtime instance carries per-call state, so neither the member
// value nor its handler may ever bake into the program (the freeze-gate:
// a baked check-time closure would log with the shape state). The read
// therefore stays DYNAMIC — and before this model, a statement-position
// method call (`l.info "req" ; …`) stranded [dyn, args] mid-residual and
// the program refused ("dynamic value precedes residual args").
//
// The model, in three steps:
//
//  1. ANNOTATE — the accessor ReturnsFn (lang getNodeReturns) calls
//     NoteMethodShape with the read's fresh dynamic carrier and the
//     resolved member. The member is only NOTED (CheckState side table,
//     keyed by carrier ID — the fnRiskFields/CtxShapes precedent), never
//     surfaced: the carrier stays dynamic(Any), byte-identical to the
//     unannotated read, so plain-check behaviour is unchanged.
//  2. MODEL — when the compile pass steps the annotated carrier at the
//     pointer (exactly where the interpreter auto-dispatches the concrete
//     member), TryShapedMethodDispatch mirrors execFnDefLiteral: the
//     ENGINE'S OWN matcher picks the member overload over the same tape,
//     and the model commits only when the match is PURE-FORWARD over an
//     inert, evaluation-fixed statement window — so the compiled window
//     is the very token sequence the interpreter's forward collection
//     consumes, bounded by the same statement boundary. The dispatch is
//     then modelled through CarrierResults over the member's inner
//     signature (declared returns, gradual contagion — identical to a
//     real dispatch of that native).
//  3. RECORD — the outcome seam routes the modelled dispatch to
//     RecordDynMethod (via CheckState.PendingMethodApply, consumed FIRST
//     in recordDispatchOutcome so the member's native can never record as
//     a check-time CALL_NATIVE): a mid-stream OpCallDynMethod whose fn
//     operand is the dot-read EVENT (the runtime value) and whose spec
//     claims the matched arity + declared result count. The VM enforces
//     the claim and defers to the interpreter via internal_error when the
//     runtime value ever fails it (RunCompiled's runtimeShouldFallback —
//     slow, not wrong).
//
// The miscompile-E auto-dispatch guard is NOT weakened — it is RE-HOMED
// onto the landing. A shaped member with a genuine 0-arg overload
// (Span.finish, Rand.bool) is annotated and modelled as an arity-0 apply
// (shapedMethodApplyWindow's all-0-arg path), and a pinpointed PLAIN-fn
// 0-arg member read (the break-2 closure) is claimed the same way by
// tryMemberFnArrivalDispatch; in both cases the get-family read guards
// (containerFnAutoDispatchRisk / zeroArgFnOut) skip the annotated /
// pinpointed read and the landing model's guard-owned decline re-refuses
// whatever it cannot claim.

// evalFixedWindowToken reports whether a raw tape token is an INERT,
// evaluation-fixed VALUE the shaped-method model may bake as a const
// operand: a concrete scalar/atom, or a list/map whose members are
// recursively such. Evaluation-fixed means the interpreter's arg
// processing (forward collection + autoEvalList/Map at dispatch) is the
// identity on it — no words, parens, interpolations, reaches, splices,
// fn values, computed map keys, carriers, or undefined placeholders —
// so the baked const IS the value the interpreter's handler receives.
func evalFixedWindowToken(v core.Value) bool {
	if v.Carrier || v.Dynamic || v.Undefined {
		return false
	}
	switch d := v.Data.(type) {
	case core.IntPayload, core.StrPayload, core.BoolPayload, core.FloatPayload, core.AtomPayload,
		core.BigIntPayload, core.DecimalPayload:
		return true
	case core.ListPayload:
		for _, ev := range d.Elems {
			if !evalFixedWindowToken(ev) {
				return false
			}
		}
		return true
	case core.MapPayload:
		if d.M == nil {
			return false
		}
		if d.M.Meta != nil {
			if ck, _ := d.M.Meta["ck"].(map[string]bool); len(ck) > 0 {
				return false // computed keys evaluate at dispatch time
			}
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !evalFixedWindowToken(mv) {
				return false
			}
		}
		return true
	}
	return false
}

// TryShapedMethodDispatch models the interpreter's auto-dispatch of an
// annotated dynamic method-read carrier sitting at the pointer (see the
// file comment). Returns true when it consumed the dispatch (tape spliced,
// event recorded or the program marked uncompilable); false leaves the
// carrier to today's paths (residual windows, refusals) untouched.
func TryShapedMethodDispatch(e *core.Engine, valIdx int) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || v.ID == "" {
		return false
	}
	member, ok := r.Check.MethodShapeMember(v.ID)
	if !ok {
		return false
	}
	if !r.Check.Recorder().Active() {
		// Plain-check surface (not compiling): a shaped dynamic method apply —
		// a delegation-wrapper method value followed by its contiguous inert
		// forward-arg window — collapses to a single dynamic(Any), consuming
		// the window, so the residual covers any runtime result. Without this
		// the args strand on the check stack and the checker mis-models the
		// arity (module-rand.tsv:37: `r.string "abc" 5` left
		// `[dynamic(Any) ProperString Integer]` vs runtime `[ProperString]`).
		// Gated to the non-compiling ratchet surface: the compile pass below
		// keeps the [dynamic, args] residual intact so resolveDynamicApply →
		// OpCallDynMethod still fires, and a suspended pass stays byte-identical.
		if !r.Check.Compiling {
			if sig, positions, wok := shapedMethodApplyWindow(e, valIdx, member); wok {
				// Collapse to the matched signature's ARITY, not always one value.
				// Side-effect-only shaped methods (logger info, metric add/record)
				// declare zero returns; splicing a lone dynamic(Any) would fabricate
				// a value the runtime never produces, so `(l.info "msg") add 1` would
				// wrongly check clean. Resolve the arity exactly as a real dispatch
				// would (ReturnsFn / declared Returns), then splice that many gradual
				// carriers — 0 for a side-effect method, 1 for a value method.
				args := make([]core.Value, len(positions))
				for i, p := range positions {
					args[i] = e.Tape.At(p)
					args[i].Eval = false
					args[i].Undefined = false
				}
				reps := make([]core.Value, shapedMethodReturnArity(e, sig, args, v.Pos()))
				for i := range reps {
					reps[i] = core.NewDynamicCarrier(core.TAny)
				}
				e.Tape.Splice(valIdx, 1+len(positions), reps...)
				return true
			}
		}
		return false
	}
	sig, positions, ok := shapedMethodApplyWindow(e, valIdx, member)
	if !ok {
		// Guard-owned decline: the get-family read guard was SKIPPED for this
		// annotated read (NoteShapedRead), so a genuine-0-arg member whose
		// landing the model cannot claim must refuse HERE — the auto-dispatch
		// guard is re-homed onto the landing, never weakened.
		if core.FnValueZeroArg(member) {
			r.Check.Recorder().MarkUncompilable(
				"shaped 0-arg method landing not modelable at " + fnDefName(member))
		}
		return false
	}
	fnDef, _ := member.Data.(core.FnDefInfo) // validated by shapedMethodApplyWindow
	args := make([]core.Value, len(positions))
	for i, p := range positions {
		args[i] = e.Tape.At(p)
		args[i].Eval = false
		args[i].Undefined = false
	}
	// Model the dispatch through the shared check-mode machinery (declared
	// returns, folds, contagion — identical to a real dispatch of the inner
	// native) with the outcome seam routed to RecordDynMethod.
	r.Check.PendingMethodApply = &core.PendingMethodApply{Origin: v, Word: fnDef.Name}
	outs := CarrierResults(r, fnDef.Name, sig, args, v.Pos(), nil, false)
	if r.Check.PendingMethodApply != nil {
		// Not consumed — an unexpected short-circuit upstream of the outcome
		// seam. Decline wholesale; the carrier keeps today's paths.
		r.Check.PendingMethodApply = nil
		return false
	}
	// Consume the carrier + the matched window, splice in the modelled
	// results; the pointer re-steps them (spliceMatchResults' convention).
	k := len(positions)
	e.Tape.Splice(valIdx, 1+k, outs...)
	return true
}

// shapedMethodApplyWindow returns the matched signature and forward-arg
// positions for a shaped dynamic method carrier at valIdx followed by a
// contiguous inert forward-arg window — or ok=false when the shape is not a
// plain-native statement-window apply. Pure (no tape mutation): shared by the
// compile-pass model (which runs the real dispatch) and the plain-check
// collapse (which folds the apply to dynamic(Any)).
func shapedMethodApplyWindow(e *core.Engine, valIdx int, member core.Value) (*core.Signature, []int, bool) {
	fnDef, ok := member.Data.(core.FnDefInfo)
	if !ok || fnDef.Registry == nil {
		return nil, nil, false
	}
	fn := fnDef.Registry.Lookup(fnDef.Name)
	if fn == nil {
		return nil, nil, false
	}
	// The statement window: every token from the carrier to the statement
	// boundary must be inert and evaluation-fixed. Anything else — a word, a
	// paren, a marker, a carrier — declines the whole model (the interpreter
	// could dispatch or collect through it in ways the window cannot bake).
	// An ALL-0-ARG member skips the scan entirely: it NEVER forward-collects
	// (it auto-fires with no operands the moment it lands), so the following
	// tokens belong to the NEXT dispatch and are irrelevant to the arity-0
	// model — `r get "bool" eq false` was declining on the `eq` word the
	// member cannot consume (probe-verified; the dot form of the same
	// landing already compiled through the identical arity-0 claim).
	winEnd := valIdx
	if !allZeroArgSigs(fn) {
		var winOK bool
		if winEnd, winOK = inertStatementWindow(e, valIdx); !winOK {
			return nil, nil, false
		}
	}
	if winEnd == valIdx || allZeroArgSigs(fn) {
		// No forward window (or every overload is 0-arg, so the landing
		// never collects): the interpreter auto-fires a GENUINE 0-arg
		// overload the moment the member lands — the miscompile-E family
		// (Span.finish, Rand.bool). Model it as an arity-0 apply through
		// the member's 0-arg signature; a member without a genuine 0-arg
		// overload stays data in both engines.
		if !core.FnValueZeroArg(member) {
			return nil, nil, false
		}
		for i := range fn.Signatures {
			sg := &fn.Signatures[i]
			if sg.Fallback || sg.TotalArgs() != 0 {
				continue
			}
			if sg.DispatchHandler() == nil || sg.FnFrame() != nil || sg.FullStack() ||
				sg.RunInCheckMode() || sg.Callable != nil || len(sg.NoEvalArgs) > 0 ||
				sg.ParkResult() {
				return nil, nil, false // not a plain Go-handler apply
			}
			return sg, nil, true
		}
		return nil, nil, false
	}
	// The interpreter's own overload choice over the same tape. The window
	// admission above guarantees no paren pre-evaluation is needed
	// (resolveForwardArgs would be a no-op), so the plan-time match here IS
	// the match the interpreter performs on the concrete member.
	w := core.WordInfo{Name: fnDef.Name, ArgCount: -1}
	sig, positions, _ := e.MatchSignature(fn, w, e.EffectiveResolved())
	if sig == nil || sig.Fallback || len(positions) == 0 {
		return nil, nil, false
	}
	// Pure-forward, contiguous-prefix coverage: the matched positions must be
	// exactly the window slots valIdx+1 .. valIdx+k. A match that reaches the
	// stack below the carrier (a trailing/mixed shape) or skips a slot is not
	// the statement-window apply — those keep today's paths.
	for i, p := range positions {
		if p != valIdx+1+i {
			return nil, nil, false
		}
	}
	if positions[len(positions)-1] > winEnd { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, false
	}
	// Only a plain Go-handler native models: no code bodies, no quotation
	// beyond atom capture, no check-mode side effects, no user-fn frames —
	// the shaped-method class is exactly the delegation-wrapper methods.
	if sig.DispatchHandler() == nil || sig.FnFrame() != nil || sig.FullStack() ||
		sig.RunInCheckMode() || sig.Callable != nil || len(sig.NoEvalArgs) > 0 ||
		sig.ParkResult() {
		return nil, nil, false
	}
	return sig, positions, true
}

// statementWindowBoundary reports whether v ends a statement window for
// the check-side apply models: an engine marker or an explicit
// statement/paren boundary. The single boundary set shared by every
// scanner below — it must stay aligned with what the interpreter's own
// forward collection treats as a hard stop.
func statementWindowBoundary(v core.Value) bool {
	return core.IsMark(v) || core.IsMove(v) || core.IsCloseParen(v) || core.IsEnd(v)
}

// inertStatementWindow scans the statement window after valIdx: every
// token up to the first boundary (statementWindowBoundary) must be inert
// and evaluation-fixed. Returns the index of the last window token
// (valIdx itself when the window is empty) and ok=false when a non-fixed
// token — a word, a paren, a carrier — sits inside the window: the
// interpreter could dispatch or collect through it in ways a flat
// consume cannot mirror, so the caller's model declines. The ONE scanner
// behind both the shaped-method window and the dynamic fn-value window;
// tryMemberFnArrivalDispatch applies the same per-token test over its
// arity-bounded span.
func inertStatementWindow(e *core.Engine, valIdx int) (winEnd int, ok bool) {
	winEnd = valIdx
	for i := valIdx + 1; i < e.Tape.Len(); i++ {
		tv := e.Tape.At(i)
		if statementWindowBoundary(tv) {
			break
		}
		if !evalFixedWindowToken(tv) {
			return winEnd, false
		}
		winEnd = i
	}
	return winEnd, true
}

// allZeroArgSigs delegates to the kernel's canonical pure-property-fn
// predicate (fnValueOnlyZeroArgSigs, engine.go) — the shaped-method
// 0-arg landing model and the interpreter's NUR035 deferral exemption
// must answer this question identically, so there is exactly one
// implementation.
func allZeroArgSigs(fn *core.FnDefInfo) bool {
	return core.FnValueOnlyZeroArgSigs(*fn)
}

// fnDefName names a function value for a refusal message.
func fnDefName(v core.Value) string {
	if fd, ok := v.Data.(core.FnDefInfo); ok && fd.Name != "" {
		return fd.Name
	}
	return "fn value"
}

// shapedMethodReturnArity is the runtime result count of a shaped-method apply,
// resolved exactly as declaredReturnCarriers does — a ReturnsFn's produced arity,
// else the declared Returns length (nil → 0) — but WITHOUT the missing-returns
// diagnostic, since the plain-check collapse is silent. This keeps the collapse
// arity-faithful: a side-effect-only method (0 returns) collapses to 0 values,
// not a fabricated one.
func shapedMethodReturnArity(e *core.Engine, sig *core.Signature, args []core.Value, pos core.SrcPos) int {
	if sig.ReturnsFn != nil {
		e.Registry.Check.CurCallPos = pos
		return len(sig.ReturnsFn(args, e.Registry))
	}
	return len(sig.Returns)
}

// dynamicBoundConformsToFunction reports whether a dynamic carrier's static
// BOUND could be a callable — its Parent conforms to Function, or (the
// typed-patrun `find` shape) one alternative of its disjunct bound does. Only
// a Function-bearing bound may auto-dispatch a forward window; a
// dynamic(String|None) etc. must strand its trailing values as data (the
// runtime never calls them), keeping the residual stack depth honest.
func dynamicBoundConformsToFunction(v core.Value) bool {
	if v.Parent.ConformsTo(core.TFunction) {
		return true
	}
	if disj, err := core.AsDisjunct(v); err == nil {
		for _, alt := range disj.Alternatives {
			// A disjunct alternative is a bare type-literal Value whose own
			// lattice identity (typeNodeOf) is the represented type — its
			// .Parent is TType, not the type itself.
			at := core.TypeNodeOf(alt)
			if at.ConformsTo(core.TFunction) {
				return true
			}
		}
	}
	return false
}

// tryDynamicFnValueDispatch is the general-value analogue of
// TryShapedMethodDispatch: a DYNAMIC carrier whose bound is Function-bearing
// (a typed-patrun `find` result — dynamic(Function ∪ None) — or any dynamic
// fn value) sitting at the pointer with a contiguous inert forward-arg window
// after it is the interpreter's auto-dispatch site. WHICH concrete fn the
// carrier holds is a runtime fact, so the model is optimistic on the callable
// alternative (like every dynamic-modality escape hatch): consume the window
// and produce a single dynamic(Any), which the oracle's Dynamic rule covers.
// This clears the arg-stranding that leaves `h {x:3 y:4}` as
// `[dynamic(Function|None) Map]` (vs runtime `[Integer]`) — patrun.tsv:40.
//
// PLAIN-CHECK ONLY (not compiling, no live recorder). The compile pass keeps
// the [dynamic, args] residual so resolveDynamicApply / OpCallDynamicTrailing
// still lowers the call, and a suspended pass stays byte-identical
// (TestSpecCompiledDifferential). Optimism is sound for every clean corpus row
// — a clean miss-then-call (a None reader immediately applied) does not occur;
// it is the same gradual gap dynamic modality accepts elsewhere.
func tryDynamicFnValueDispatch(e *core.Engine, valIdx int) bool {
	r := e.Registry
	if r.Check.Compiling || r.Check.Recorder().Active() {
		return false
	}
	if tryShapedFnReadWindow(e, valIdx) {
		return true
	}
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || !dynamicBoundConformsToFunction(v) {
		return false
	}
	// The forward window: the same inert, evaluation-fixed statement-window
	// admission the shaped-method model uses (one shared scanner).
	winEnd, winOK := inertStatementWindow(e, valIdx)
	if !winOK {
		return false
	}
	if winEnd == valIdx {
		return false // no args — a bare dynamic fn value stays data (both engines)
	}
	e.Tape.Splice(valIdx, 1+(winEnd-valIdx), core.NewDynamicCarrier(core.TAny))
	return true
}

// tryRecordMethodApply is the FIRST specialist in recordDispatchOutcome's
// chain: it consumes a pending shaped-method model (set by
// TryShapedMethodDispatch around its CarrierResults call) and records the
// guarded OpCallDynMethod event. It must run before every other recorder —
// falling through would record the member's inner native as a check-time
// CALL_NATIVE against the shape instance's sub-registry, baking shape
// state (the freeze-gate violation this model exists to avoid).
func TryRecordMethodApply(r *core.Registry, word string, args, out []core.Value, pos core.SrcPos) bool {
	pm := r.Check.PendingMethodApply
	if pm == nil {
		return false
	}
	if pm.Word != word {
		return false // an interleaved dispatch — leave the pending for its owner
	}
	r.Check.PendingMethodApply = nil
	es := r.Check.Recorder()
	if !es.RecordDynMethod(pm.Origin, args, out, word, pos) {
		es.MarkUncompilable("shaped method apply: operand of unknown provenance at " + word)
	}
	return true
}

// tryMemberFnArrivalDispatch models the interpreter's ARRIVAL-APPLY of a
// container-member fn read mid-expression (REFUSAL-CLOSURE.0 §3): the
// interpreter applies a surfaced member fn (`m.double`) the moment its
// argument window fills — `m.double 21 eq 42` runs `(m.double 21)` BEFORE
// `eq` — while the recorder previously only saw word dispatches, so the
// downstream word stole the operand and refuseStrandedMemberFn refused the
// program. This hook fires where the check pass steps the member-read
// carrier: when the read pinpointed the member (memberFnReadValue — a
// concrete container + key) and the member's SINGLE plain signature's whole
// arity of inert tokens sits immediately after the carrier, it models the
// dispatch through the shared PendingMethodApply → RecordDynMethod seam
// (the M2c machinery: a guarded mid-stream OpCallDynMethod whose runtime
// value — the real map read's product — drives the apply, byte-identical to
// the interpreter's forward auto-dispatch of the same window).
//
// The window claim is the ARITY, not the statement: the interpreter's parked
// fn fires the moment its single signature's args arrive, so the token after
// the window (a word, `eq`) never enters the collection. Everything this
// hook declines keeps today's paths — the statement-tail Finalize apply for
// shapes it never sees, refuseStrandedMemberFn's sound refusal for the rest:
//   - COMPILE pass only (live recording; plain checks and suspended passes
//     stay byte-identical);
//   - a uniquely-resolved, NAMED, non-anonymous, non-macro, capture-free
//     member with exactly ONE non-fallback signature of plain params (the
//     model's CarrierResults and the arity claim assume plain value args —
//     multi-sig first-match and captures are follow-on scope);
//   - the full arity of evaluation-fixed tokens inside the statement.
//     Arity 0 is claimed too (the break-2 closure, FN-VALUE-OPEN-WORK §4):
//     the interpreter's courtesy dispatch fires the moment the member
//     lands, so the model is an empty-window arity-0 OpCallDynMethod —
//     the VM islands [fn] and the interpreter's own courtesy dispatch
//     runs inside the island, byte-identical.
//
// Because the get-family read guard SKIPS its auto-dispatch refusal for a
// pinpointed genuine-0-arg member (zeroArgMemberFnLandingOut), a 0-arg
// landing this model cannot claim must refuse HERE — the guard is
// re-homed onto the landing, never weakened (TryShapedMethodDispatch's
// guard-owned-decline precedent). Every decline below routes through
// declineMemberFnArrival for exactly that reason; for an arity >= 1
// member (no genuine 0-arg overload) it stays a plain decline.
func tryMemberFnArrivalDispatch(e *core.Engine, valIdx int) bool {
	r := e.Registry
	es := r.Check.Recorder()
	if !es.Active() || es.SuspendedNow() {
		return false
	}
	if tryShapedFnReadArrival(e, valIdx, es) {
		return true
	}
	v := e.Tape.At(valIdx)
	if !v.Dynamic || v.Quoted || v.ID == "" {
		return false
	}
	member, ok := es.MemberFnReadValue(v.ID)
	if !ok {
		return false
	}
	decline := func() bool { return declineMemberFnArrival(es, member) }
	fnDef, _ := member.Data.(core.FnDefInfo) // validated by memberFnReadValue
	if fnDef.Name == "" || fnDef.Anonymous || fnDef.Macro || len(fnDef.Captured) != 0 {
		return decline()
	}
	var sig *core.Signature
	for i := range fnDef.Signatures {
		s := &fnDef.Signatures[i]
		if s.Fallback {
			continue
		}
		if sig != nil {
			return decline() // multi-overload: runtime first-match not modelled here
		}
		sig = s
	}
	// A boru body and plain value params only (the model and the arity
	// claim assume them). FnParam.Quote needs no separate probe: the body
	// gate proves a boru impl, whose normalizeSig derives QuoteArgs FROM
	// the params.
	if sig == nil || len(sig.Body()) == 0 ||
		len(sig.QuoteArgs) != 0 || len(sig.TypeArgs) != 0 ||
		len(sig.NoEvalArgs) != 0 || len(sig.NoEvalMapArgs) != 0 ||
		len(sig.RawParens) != 0 || len(sig.FormArgs) != 0 {
		return decline()
	}
	if len(sig.Returns) != 1 {
		return decline() // the arity/result claim assumes one downstream value
	}
	n := sig.TotalArgs()
	if valIdx+n >= e.Tape.Len() {
		return false // arity >= 1 only: a 0-arg member needs no window
	}
	args := make([]core.Value, n)
	for i := 1; i <= n; i++ {
		tv := e.Tape.At(valIdx + i)
		if statementWindowBoundary(tv) || !evalFixedWindowToken(tv) {
			return false
		}
		args[i-1] = tv
		args[i-1].Eval = false
		args[i-1].Undefined = false
	}
	// Model the dispatch SUSPENDED (a plain user fn's ReturnsFn records its
	// own CALL_USER through core_helpers, not the outcome seam — a live probe
	// would leak a phantom event the lowering cannot seat), then record the
	// guarded dyn-method event directly. No body unit is needed: the VM's
	// callDynMethod applies the RUNTIME member value — a plain user fn takes
	// the island path, byte-identical to the interpreter's auto-dispatch —
	// and the member's declared return contract is engine-enforced at run
	// time, so the modelled out carrier is sound.
	resume := es.Suspend()
	outs := CarrierResults(r, fnDef.Name, sig, args, v.Pos(), nil, false)
	resume()
	if len(outs) != 1 {
		return decline()
	}
	if !es.RecordDynMethod(v, args, outs, fnDef.Name, v.Pos()) {
		return decline()
	}
	e.Tape.Splice(valIdx, 1+n, outs...)
	return true
}

// declineMemberFnArrival is tryMemberFnArrivalDispatch's guard-owned
// decline: the get-family read guard skipped its auto-dispatch refusal for
// a pinpointed GENUINE-0-arg member on the promise that the arrival model
// owns the landing, so a 0-arg landing the model cannot claim refuses here
// with the guard's own reason — re-homed, never weakened. A member without
// a genuine 0-arg overload was never exempted at the read, so its decline
// stays silent and the carrier keeps today's paths.
func declineMemberFnArrival(es core.EmitRecorder, member core.Value) bool {
	if core.FnValueZeroArg(member) {
		return refuseArrival(es,
			"fn value read from a container auto-dispatches (Stage 3): 0-arg landing not modelable at "+fnDefName(member))
	}
	return false
}

// refuseArrival is the arrival models' shared guard-owned decline: the
// landing refuses with the reason its model owns, and the model reports
// "not consumed" so the engine steps on to the refusal's fallback.
func refuseArrival(es core.EmitRecorder, reason string) bool {
	es.MarkUncompilable(reason)
	return false
}

// tryShapedFnReadArrival models the WORD DISPATCH of a def-bound computed fn
// whose shape the producing word claimed (CheckState.FnShapes — the fn-util
// wrappers: `def k (FnUtil.const 7)  (k 99)`, `def bigger (FnUtil.on gt2/v
// sq/v)  (bigger 3 5)`). The check pass substitutes the bound CARRIER for
// the read (stepWord's fn-carrier side table), and a carrier at the pointer
// is data the pass steps past; the interpreter's `k` is a WORD whose bound
// fn value collects its forward args the moment it is read — inside the
// statement, up to its all-forward barrier (the wrapper's arity) — and
// dispatches. The residual classifier used to lower the flattened
// [carrier, args…] as a leading apply, and flattening loses the statement:
// `bigger 3 ; 5` lowered as `bigger 3 5` (compiled false where the
// interpreter raises "cannot call `bigger`"), and `(bigger 3 ; 5)` the same
// inside a paren — the word dispatched at the `;`, before the collapse.
//
// So the model fires WHERE the interpreter dispatches — at the read — over
// the same window the member-fn arrival model claims: the wrapper's whole
// arity of evaluation-fixed tokens inside the statement, consumed into one
// guarded OpCallDynMethod (RecordDynMethod: the runtime read supplies the
// value, the VM applies a self-contained Go-impl wrapper natively on its
// own signatures, and the result-count claim of one defers any wrapper
// whose applied fn returns another count). The extra tokens of a longer
// window stay on the tape, exactly as the interpreter leaves them
// (`(k 1 2)` is `7 2`).
//
// Everything else REFUSES rather than declines: a window short of the
// arity (the interpreter fills the rest from the stack, or raises), a
// non-fixed token inside it (a word, a paren, a carrier — the interpreter
// evaluates it under the pending collection), a read the recorder cannot
// seat. A silent decline would hand the carrier back to the residual
// classifier, and the classifier's flattened window is the miscompile
// above; a claimed def-read lead is this model's alone.
func tryShapedFnReadArrival(e *core.Engine, valIdx int, es core.EmitRecorder) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if v.Quoted || v.ID == "" || !core.IsFnTypedCarrier(v) {
		return false
	}
	name, read := es.DefReadName(v.ID)
	if !read {
		return false
	}
	n, claimed := r.Check.FnShapeArity(v.ID)
	if !claimed {
		return false
	}
	refuse := func(what string) bool {
		return refuseArrival(es, "def-bound computed fn `"+name+"`: "+what+" (the read's statement window — Stage 1)")
	}
	args, why := shapedFnReadWindow(e, valIdx, n)
	if why != "" {
		return refuse(why)
	}
	out := core.NewDynamicCarrier(core.TAny)
	if !es.RecordDynMethod(v, args, []core.Value{out}, name, v.Pos()) {
		return refuse("an operand has no compiled home")
	}
	e.Tape.Splice(valIdx, 1+n, out)
	return true
}

// shapedFnReadWindow scans the wrapper's arity of tokens after valIdx for the
// two read models: every token evaluation-fixed and inside the statement. why
// names the first failure, and is empty when the window is whole.
func shapedFnReadWindow(e *core.Engine, valIdx, n int) (args []core.Value, why string) {
	if valIdx+n >= e.Tape.Len() {
		return nil, "the statement ends short of the wrapper's arity"
	}
	args = make([]core.Value, n)
	for i := 1; i <= n; i++ {
		tv := e.Tape.At(valIdx + i)
		if statementWindowBoundary(tv) {
			return nil, "the statement ends short of the wrapper's arity"
		}
		if !evalFixedWindowToken(tv) {
			return nil, "an argument is not an evaluation-fixed value"
		}
		args[i-1] = tv
		args[i-1].Eval = false
		args[i-1].Undefined = false
	}
	return args, ""
}

// tryShapedFnReadWindow is the PLAIN-check half of the read model (the
// compile pass's is tryShapedFnReadArrival, which records the apply): a
// def-bound fn carrier with a claimed arity at the pointer, followed by its
// arity of evaluation-fixed tokens inside the statement, is the interpreter's
// word dispatch — the window collapses to one dynamic(Any), the same
// optimistic collapse the dynamic fn-value window takes, so the check stack
// carries the dispatch's one result rather than the carrier and its
// arguments (the type-soundness ratchet saw `(bigger 3 5)` as [Function
// Integer Integer] for the runtime's [Boolean]). A window the model cannot
// claim is left as it is: a plain check has no refusal to make. The
// def-bound test is the fn-carrier side table itself (CheckFnCarrierBoundName)
// — a plain check has no live recorder to remember the read — so an EVENT
// carrier (`((FnUtil.const 7) 99)`) is not in the table and keeps its shape.
func tryShapedFnReadWindow(e *core.Engine, valIdx int) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if v.Quoted || v.Dynamic || v.ID == "" || !core.IsFnTypedCarrier(v) {
		return false
	}
	if _, bound := core.CheckFnCarrierBoundName(r, v.ID); !bound {
		return false
	}
	n, claimed := r.Check.FnShapeArity(v.ID)
	if !claimed {
		return false
	}
	if _, why := shapedFnReadWindow(e, valIdx, n); why != "" {
		return false
	}
	e.Tape.Splice(valIdx, 1+n, core.NewDynamicCarrier(core.TAny))
	return true
}
