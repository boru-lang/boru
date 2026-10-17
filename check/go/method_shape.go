package check

import core "github.com/boru-lang/boru/core/go"

// method_shape.go — the shaped-instance-method dispatch model (Phase 6
// Stage M2c, design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6 M2c).
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
// the program declined ("dynamic value precedes residual args").
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
//     runtime value ever fails it (RunCompiled's runtimeShouldFallback),
//     which contains the failure silently rather than closing it.
//
// The miscompile-E auto-dispatch guard is NOT weakened — it is RE-HOMED
// onto the landing. A shaped member with a genuine 0-arg overload
// (Span.finish, Rand.bool) is annotated and modelled as an arity-0 apply
// (shapedMethodApplyWindow's all-0-arg path), and a pinpointed PLAIN-fn
// 0-arg member read (the break-2 closure) is claimed the same way by
// tryMemberFnArrivalDispatch; in both cases the get-family read guards
// (containerFnAutoDispatchRisk / zeroArgFnOut) skip the annotated /
// pinpointed read and the landing model's guard-owned decline re-declines
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
	if v.Carrier() || core.IsDynamic(&v) || core.IsUndefined(&v) {
		return false
	}
	switch d := v.Data().(type) {
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

// modifiedAsData reports whether a dispatch modifier (`/v`, `/q`) follows
// the carrier at valIdx — directly, or after the close of the reach group
// it stands alone in (`( m dot f ) /v`): the marker says DATA, so the member
// the read delivers stays a value where the interpreter's peek leaves it —
// `def m {f: g/v} m.f/v` is `fn g`, where a 0-arg member's arrival model
// fired it (7; NUR318). The marker then quotes the carrier (the
// standalone-marker drop, NUR277).
func modifiedAsData(e *core.Engine, valIdx int) bool {
	i := valIdx + 1
	if i < e.Tape.Len() && core.IsCloseParen(e.Tape.Ref(i)) && valIdx > 0 && core.IsOpenParen(e.Tape.Ref(valIdx-1)) {
		i++
	}
	return i < e.Tape.Len() && core.IsDispatchMod(e.Tape.Ref(i))
}

// TryShapedMethodDispatch models the interpreter's auto-dispatch of an
// annotated dynamic method-read carrier sitting at the pointer (see the
// file comment): a shaped dynamic method apply — a delegation-wrapper
// method value followed by its contiguous inert forward-arg window —
// collapses to gradual carriers, consuming the window, so the residual
// covers any runtime result. Without this the args strand on the check
// stack and the checker mis-models the arity (module-rand.tsv:37:
// `r.string "abc" 5` left `[dynamic(Any) ProperString Integer]` vs runtime
// `[ProperString]`). Returns true when it consumed the dispatch (tape
// spliced); false leaves the carrier to today's paths untouched.
func TryShapedMethodDispatch(e *core.Engine, valIdx int) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if !core.IsDynamic(&v) || core.IsQuoted(&v) || core.IDOf(&v) == "" || modifiedAsData(e, valIdx) {
		return false
	}
	member, ok := r.Check.MethodShapeMember(core.IDOf(&v))
	if !ok {
		return false
	}
	sig, positions, wok := shapedMethodApplyWindow(e, valIdx, member)
	if !wok {
		return false
	}
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
		core.SetEval(&args[i], false)
		core.SetUndefined(&args[i], false)
	}
	reps := make([]core.Value, shapedMethodReturnArity(e, sig, args, v.Pos()))
	for i := range reps {
		reps[i] = core.NewDynamicCarrier(core.TAny)
	}
	e.Tape.Splice(valIdx, 1+len(positions), reps...)
	return true
}

// shapedMethodApplyWindow returns the matched signature and forward-arg
// positions for a shaped dynamic method carrier at valIdx followed by a
// contiguous inert forward-arg window — or ok=false when the shape is not a
// plain-native statement-window apply. Pure (no tape mutation): shared by the
// compile-pass model (which runs the real dispatch) and the plain-check
// collapse (which folds the apply to dynamic(Any)).
func shapedMethodApplyWindow(e *core.Engine, valIdx int, member core.Value) (*core.Signature, []int, bool) {
	fnDef, ok := member.Data().(*core.FnDefInfo)
	if !ok {
		return nil, nil, false
	}
	fn := core.FnHomeLookup(fnDef)
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
				sg.RunInCheckMode() || len(sg.NoEvalArgs) > 0 || sg.ParkResult() {
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
		sig.RunInCheckMode() || len(sig.NoEvalArgs) > 0 || sig.ParkResult() {
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
	return core.IsMark(&v) || core.IsMove(&v) || core.IsCloseParen(&v) || core.IsEnd(&v)
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
	return core.FnValueOnlyZeroArgSigs(fn)
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
		// No WORD dispatched this apply, so the ReturnsFn's region claim must
		// miss rather than key on whatever the previous dispatch published.
		e.Registry.Check.CurCallWord = ""
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
	if v.Parent().ConformsTo(core.TFunction) {
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
// Optimism is sound for every clean corpus row — a clean miss-then-call (a
// None reader immediately applied) does not occur; it is the same gradual gap
// dynamic modality accepts elsewhere.
func tryDynamicFnValueDispatch(e *core.Engine, valIdx int) bool {
	if tryFnShapeTypedWindow(e, valIdx) || tryShapedFnReadWindow(e, valIdx) {
		return true
	}
	v := e.Tape.At(valIdx)
	if !core.IsDynamic(&v) || core.IsQuoted(&v) || !dynamicBoundConformsToFunction(v) {
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

// shortReadWindow is shapedFnReadWindow's verdict when the statement ends
// before the wrapper's arity of tokens.
const shortReadWindow = "the statement ends short of the wrapper's arity"

// unfixedWindowToken is shapedFnReadWindow's verdict when a token inside
// the window is not evaluation-fixed (a word, a paren, a carrier): the
// interpreter evaluates it under the pending collection.
const unfixedWindowToken = "an argument is not an evaluation-fixed value"

// unfitWindowToken is shapedFnReadWindow's verdict when a written token
// does not conform to the wrapper's parameter the claim knows: the
// interpreter's matcher tries the token against the signature and, failing,
// dispatches over the frame instead (or raises with nothing there) — a
// fallback the window claim cannot model, so the read declines (NUR194).
const unfitWindowToken = "a written argument does not fit the wrapper's parameter"

// shapedFnReadWindow scans the wrapper's arity of tokens after valIdx for the
// two read models: every token evaluation-fixed, inside the statement and —
// where the claim knows the parameter types — conforming to them. why names
// the first failure, and is empty when the window is whole.
func shapedFnReadWindow(e *core.Engine, valIdx int, shape core.FnShape) (args []core.Value, why string) {
	n := shape.Arity
	if valIdx+n >= e.Tape.Len() {
		return nil, shortReadWindow
	}
	args = make([]core.Value, n)
	for i := 1; i <= n; i++ {
		tv := e.Tape.At(valIdx + i)
		if statementWindowBoundary(tv) {
			return nil, shortReadWindow
		}
		if !evalFixedWindowToken(tv) {
			return nil, unfixedWindowToken
		}
		if i-1 < len(shape.Params) && shape.Params[i-1] != nil && !core.SigTypeMatches(&tv, shape.Params[i-1]) {
			return nil, unfitWindowToken
		}
		args[i-1] = tv
		core.SetEval(&args[i-1], false)
		core.SetUndefined(&args[i-1], false)
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
// claim is left as it is: a plain check has no compile failure to make. The
// def-bound test is the fn-carrier side table itself (CheckFnCarrierBoundName)
// — a plain check has no live recorder to remember the read — so an EVENT
// carrier (`((FnUtil.const 7) 99)`) is not in the table and keeps its shape.
func tryShapedFnReadWindow(e *core.Engine, valIdx int) bool {
	r := e.Registry
	v := e.Tape.At(valIdx)
	if core.IsQuoted(&v) || core.IsDynamic(&v) || core.IDOf(&v) == "" || !core.IsFnTypedCarrier(&v) {
		return false
	}
	if _, bound := core.CheckFnCarrierBoundName(r, core.IDOf(&v)); !bound {
		return false
	}
	shape, claimed := r.Check.FnShapeOf(core.IDOf(&v))
	if !claimed {
		return false
	}
	if _, why := shapedFnReadWindow(e, valIdx, shape); why != "" {
		return false
	}
	e.Tape.Splice(valIdx, 1+shape.Arity, shapedReadOut(r, core.IDOf(&v)))
	return true
}

// tryFnShapeTypedWindow is the plain check's model of a fn-SHAPE-typed
// carrier's apply (NUR096). A value typed by a fn shape (`def T fnsig
// [[Integer] [Integer Integer]]`, then a T-typed class field read) IS a
// function at run time — the shape's membership, enforced at make and set,
// admits nothing else — and the interpreter re-steps it here, collecting
// its argument window: `c.op 10` is [10 10]. The pass held the carrier and
// its argument instead, [T Integer], which a single-return shape hid (the
// top slot matched) and a multi-return shape exposed. The shape's declared
// signature is the model: its parameter count of evaluation-fixed tokens
// that fit its parameter types is consumed, and one carrier per declared
// return takes their place — sound for any stored fn, whose returns the
// shape covers. Anything else is left as it was: an arity-0 shape (whether
// a stored 0-arg fn fires depends on the fn, not the shape), a parameter
// the types alone do not decide, a window the claim cannot fill.
func tryFnShapeTypedWindow(e *core.Engine, valIdx int) bool {
	v := e.Tape.At(valIdx)
	if core.IsQuoted(&v) || core.IsDynamic(&v) || core.IDOf(&v) == "" || !v.Carrier() || !core.TypeIsFnShape(v.Parent()) {
		return false
	}
	shape, ok := fnShapeTypedClaim(e.Registry, v)
	if !ok || shape.Arity == 0 {
		return false
	}
	if _, why := shapedFnReadWindow(e, valIdx, shape); why != "" {
		return false
	}
	out := make([]core.Value, len(shape.Returns))
	for i, rt := range shape.Returns {
		if rt == nil || rt.Equal(core.TAny) {
			out[i] = core.NewDynamicCarrier(core.TAny)
		} else {
			out[i] = core.NewCarrier(rt)
		}
	}
	e.Tape.Splice(valIdx, 1+shape.Arity, out...)
	return true
}

// fnShapeTypedClaim is the declared signature a fn-shape-typed carrier
// stands for: the claim noted where the carrier was minted (an anonymous
// shape's field read — the carrier's own type is then the bare
// FunctionSignature node, which has lost the shape), else the content of
// the named shape node that types it. One signature of plain parameters,
// or no claim.
func fnShapeTypedClaim(r *core.Registry, v core.Value) (core.FnShape, bool) {
	if s, ok := r.Check.FnShapeOf(core.IDOf(&v)); ok {
		return s, s.ReturnsKnown
	}
	content, ok := core.TypeContentOf(*v.Parent())
	if !ok {
		return core.FnShape{}, false
	}
	info, ok := content.Data().(core.FnUndefInfo)
	if !ok {
		return core.FnShape{}, false
	}
	return FnShapeOfSpec(info)
}

// FnShapeOfSpec is the exact claim a fn-shape spec makes: its one
// signature's parameter types and declared returns. A shape of several
// signatures, or a parameter that is optional, patterned or quoted (whose
// arity or admission the types alone do not decide), makes no claim.
func FnShapeOfSpec(info core.FnUndefInfo) (core.FnShape, bool) {
	if len(info.Sigs) != 1 {
		return core.FnShape{}, false
	}
	spec := info.Sigs[0]
	params := make([]*core.Type, len(spec.Params))
	for i, p := range spec.Params {
		if p.Optional || p.Pattern != nil || p.Quote {
			return core.FnShape{}, false
		}
		params[i] = p.Type
	}
	return core.FnShape{Arity: len(params), Params: params, Returns: spec.Returns, ReturnsKnown: true}, true
}

// shapedReadOut mints the read models' one result for the carrier id's
// claim: a dynamic value, or — when the claim says the wrapper RETURNS a
// claimed fn (a curried chain's next level, a factory's factory) — a
// Function carrier claimed with that result shape, so a def of it binds a
// shaped carrier and the next read models its own dispatch the same way
// (`def c10 (c 10)  (c10 3)`, `def f2 (f1 2)  (f2 3)`).
func shapedReadOut(r *core.Registry, id string) core.Value {
	if s, ok := r.Check.FnShapeOf(id); ok && s.Result != nil {
		out := core.NewCarrier(core.TFunction)
		r.Check.NoteFnShape(out, *s.Result)
		return out
	}
	return core.NewDynamicCarrier(core.TAny)
}
