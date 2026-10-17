package basic

// ControlNatives covers the control-flow words: do, if, for, break,
// continue, error.
//
// Helpers used by these handlers (spliceArg, RunForLoop, ParseRange,
// forCarrierReturns, etc.) live alongside the slice in this file or
// in conditional.go / forloop.go for the helpers that are
// independently testable.
var ControlNatives = []NativeFunc{
	{
		Name: "do",

		Signatures: []Signature{
			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(DoListHandler),
				ReturnsFn:  DoListReturnsFn, BarrierPos: -1,
				// Only the LIST (code-body) sig islands — its NoEvalArgs body
				// re-enters the interpreter. The Map sig is a pure value eval
				// whose arg auto-evaluates BEFORE the handler, so it bakes a
				// plain CALL_NATIVE (do {a:1 b:2} no longer islands).
				// CompileDynBody is the universal backstop: a body the closure
				// path declines (computed carriers, args-bearing bodies) lowers
				// to a CALL_NATIVE under the program's DynEnv mode instead of
				// declining — the handler's runtime execution is the
				// interpreter's own semantics once names and args resolve
				// identically (see eng CompileDynBody).
				CompileEffect: CompileFallbackBody | CompileDynBody,
			},
			{
				Args:    []*Type{TMap},
				Impl:    Go(DoMapHandler),
				Returns: []*Type{TAny}, BarrierPos: -1,
				// CompileDynBody here too: a GRADUAL operand can commit to
				// this sig at check time while the runtime value is a List —
				// the dyn-body poly re-match picks the real overload at run
				// time (tryRecordDynBody's body.Dynamic branch).
				CompileEffect: CompileDynBody,
			},
		},
	},
	{
		Name: "if",

		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
				Impl:       Go(if3Handler),
				ReturnsFn:  if3ReturnsFn, BarrierPos: -1,
				// The branch is lowered by the ReturnsFn (RecordBranch), never
				// as a dispatch over the arm bodies (S2b's declaration).
				CompileEffect: CompileOwnLowering,
			},
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Impl:       Go(if2Handler),
				// As the three-operand form: RecordBranch lowers it (S2b).
				CompileEffect: CompileOwnLowering,
				ReturnsFn:     If2ReturnsFn, BarrierPos:

				// Clause-list form: `if [c1 b1 c2 b2 … else]`. Even elements
				// are conditions, the following odd element is that clause's
				// body, and a trailing element (odd-length list) is the
				// else. Conditions are tried left-to-right; the first truthy
				// one's body runs, the rest are not evaluated. Each element
				// may be a code-body list (evaluated / spliced) or a plain
				// value (used as-is). Must be tried after if3/if2 so the
				// legacy `if <listCond> <then> [<else>]` forms still win when
				// extra args are present. See ifClause in conditional.go.
				-1,
			},

			{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(IfListHandler),
				ReturnsFn:  IfListReturnsFn, BarrierPos: -1,
				// The handler returns ifClause's tokens — the chosen body
				// SPLICED for the tape to re-step — so the dispatch is never
				// a CALL_NATIVE: under the recording pass the ReturnsFn
				// lowers the chain as the if3 / if2 branch events it is
				// equivalent to (ifClauseRecord, 2026-09-26; declared
				// CompileResteps until then, when a clause list holding a
				// code body compiled to that CALL_NATIVE and died at run
				// time on the tape-coupled splice).
				CompileEffect: CompileOwnLowering,
			},
		},
	},
	{
		// case <value> [m1 b1 m2 b2 … default] — dispatch on a value.
		// The value expression is executed and its result captured
		// (a code-body list evaluates; parens evaluate before
		// collection; a plain value is used as-is). Clauses are
		// match/block pairs with an optional trailing default. A
		// match that is a code-body list executes as if the value
		// were already on the stack ([gt 3] runs `v gt 3`) and the
		// result coerces to boolean; any other match UNIFIES with
		// the value (equal scalars/atoms, a type literal matches its
		// members). The first matching clause's block runs the same
		// way — value pushed first, like the `error [handler]` block
		// — and its result is case's result. No match and no default
		// produces nothing, like `if` without an else — but the CHECKER
		// requires the clauses to cover the scrutinee's static type
		// (case_not_exhaustive, case_exhaustive.go): a default-less
		// case reaches that produce-nothing path only for a dynamic
		// (untyped) scrutinee.
		Name:          "case",
		CompileEffect: CompileFallbackBody,

		Signatures: []Signature{
			// One Any/Any sig; the handler disambiguates the two call
			// shapes (forward `case v [clauses]` vs stack-value
			// `v case [clauses]`) by which arg is the clause list —
			// a type-level split mis-sorts because both shapes are
			// (List, List) when the value is itself a code body.
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Impl:       Go(CaseHandler),
				ReturnsFn:  CaseReturnsFn,
				Returns:    []*Type{TAny}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "for",

		Signatures: []Signature{
			{
				Args:       []*Type{TInteger, TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       Go(ForCountHandler),
				ReturnsFn:  forIntegerListReturnsFn, BarrierPos: -1,
				// The loop is lowered by the ReturnsFn (RecordLoop), never as
				// a dispatch over the body (S2b's declaration). A COMPUTED
				// body (`for 3 (mk 0)`, code-bodies.tsv L141) has no tokens
				// for RecordLoop to capture: CompileDynBody hands that
				// dispatch to the dyn-body backstop, which HOSTS the
				// handler's splice on the VM's interpreter island — admitted
				// only where the splice and the island cannot be told apart
				// (the program's last statement over an empty stack; see
				// compiler's hostsSplice, 2026-09-26).
				CompileEffect: CompileOwnLowering | CompileDynBody,
			},
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       Go(ForRangeHandler),
				ReturnsFn:  forListListReturnsFn, BarrierPos: -1,
				// RecordLoop, as the count form (S2b); a computed body takes
				// the hosted splice, as the count form's does.
				CompileEffect: CompileOwnLowering | CompileDynBody,
			},
		},
	},
	{
		// while [cond] [body] — the traditional condition loop. Both
		// operands are quoted code lists; the condition re-evaluates
		// before every iteration and the body's values accumulate onto
		// the stack, exactly as `for` leaves its per-iteration values.
		// break/continue work as in `for`. Engine-stepped regions keep
		// the loop inside the step budget (a non-terminating condition
		// trips evaluation_limit). The compile lane declines the word —
		// the interpreter owns it (lang/spec/frontier/frontier-while.tsv).
		Name: "while",
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Impl:       Go(WhileHandler),
			ReturnsFn:  whileReturnsFn, BarrierPos: -1,
			// The loop is lowered by the ReturnsFn (RecordLoop, the
			// "while" loop event), never as a dispatch over the condition
			// or the body (S2b's declaration).
			CompileEffect: CompileOwnLowering,
		}},
	},
	// break and continue signal via Registry.FlowCtrl rather than
	// returning an error. The Run loop in eng/engine.go reads the
	// signal after every step and dispatches it through the nearest
	// loop's flow-control resolver.
	{
		Name: "break",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				r.FlowCtrl = FlowBreak
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "continue",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				r.FlowCtrl = FlowContinue
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: 0,
		}},
	},
	{
		Name: "error",
		// do [body] error [handler] — the handler runs as a closure with the
		// caught Error pushed as its one input (BodyPos 0, BodyOut 1). A handler
		// that CONSUMES the error (`[get message]`, `[get code case …]`) nets 1
		// and compiles; one that IGNORES it (`["fallback"]`) leaves error+result
		// (nets 2) — StripsUnconsumedInput admits that shape too: the handler's
		// runtime identity probe strips the unconsumed error from the residual
		// bottom (ErrorHandler), so the closure nets ONE value either way and
		// compiles natively instead of islanding.
		// CompileDynBody (2026-09-25): a COMPUTED handler body — a List
		// param, a fn's result — lowers to a plain CALL_NATIVE under the
		// program's DynEnv mode where the closure path declined, exactly as
		// `do`'s does: ErrorHandler runs the body through the InvokeBody
		// seam, which stamps a run-time token body and hosts it on the VM.
		CompileEffect: CompileFallbackBody | CompileDynBody,

		// ONE sig (List, Any): the handler runs when the body raised (the do
		// result is an Error), else the result passes through. The two cases were
		// formerly two sigs (TError vs TAny), but a STATIC pick is unsound for the
		// compiled path — the checker types `do [raise …]` as Any and would bake
		// the pass-through sig even when the runtime value is an Error. One sig
		// with a runtime IsError branch keeps dispatch mono (so the handler body
		// compiles as a closure) and correct on both paths.
		Signatures: []Signature{
			{
				Args:       []*Type{TList, TAny},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(ErrorHandler),
				// BarrierPos 1: the handler list is forward-collected, but the
				// do-result (position 1) MUST come from the stack — never a trailing
				// token. The former TError sig filtered a following `3` by type; the
				// merged Any sig would otherwise grab it (`error [print] 3 mul 4`).
				ReturnsFn: ErrorReturnsFn,
				Returns:   []*Type{TAny}, BarrierPos: 1,
			},
		},
	},
}

// ---- do handlers ----

func DoListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return nil, r.BoruError("do_error", "do: argument must be a concrete list, got type literal", "do")
	}
	// `do` runs its body with no per-call inputs and catches a body error,
	// surfacing it as an Error VALUE rather than propagating (the escape
	// hatch semantics). Routed through the InvokeBody seam so the VM can run
	// the body as a compiled closure.
	result, err := InvokeBody(r, args[0], nil)
	if err != nil {
		// An `IO.exit` request crosses a HANDLER-LESS `do` unchanged. The
		// escape-hatch semantics turn a body error into an Error value, and
		// for a failure that is the point — but an exit request is a
		// control transfer, not a failure, and converting it to data would
		// silently demote `IO.exit 4` to exit 0 for any program whose exit
		// happens to sit inside a `do`. The `do … error …` form still
		// observes it (an exit IS an error value there, and a handler that
		// does not recognise a foreign error must re-raise it —
		// design/CLI-PROGRAMS.0.md §4); this arm has no handler to observe
		// it with. A designed defer crosses unchanged for the same reason,
		// one layer down (bodyErrorPropagates / NUR149's second half).
		if bodyErrorPropagates(err) {
			return nil, err
		}
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

// bodyErrorPropagates reports whether a `do` body error must be
// RE-RAISED rather than trapped as an Error value. Two kinds cross the
// escape hatch unchanged:
//
//   - an IO.exit request — a control transfer, not a failure (trapping it
//     as data would demote `IO.exit 4` to exit 0);
//   - a designed VM defer (IsVMDefer: an internal_error the compiled VM
//     raises when it cannot continue — DISPATCH_GENERIC with no
//     live unit, a poly no-match, a recovered lowering panic). Trapping it
//     as an Error value STRANDS the fallback — the defer never reaches the
//     top-level run that re-runs interpreted, so the internal message
//     surfaces as data (a top-level `do [risky]` printed it) or an enclosing
//     fn's return contract rejects the Error (`type_error … got Error`)
//     where the interpreter answers cleanly.
//
// The marker (not the public `internal_error` code) is what distinguishes a
// defer: a user `raise internal_error "…"` carries the same code but no
// VMDefer marker, so it stays trapped and `do [raise internal_error …] error
// […]` still catches it — as does every other genuine boru error
// (type_error, undefined_word, a user `raise`), the escape-hatch semantics
// intend.
func bodyErrorPropagates(err error) bool {
	if _, isExit := ExitCode(err); isExit {
		return true
	}
	return IsVMDefer(err)
}

func DoListReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 1 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	body := args[0]
	if IsWord(&body) {
		w, _ := AsWord(&body)
		if v, ok := r.Defs.Top(w.Name); ok {
			body = v
		}
	}
	// A non-literal body carrier CARRYING an analysed stack effect — a
	// quoted code list read out of data, converted by the stage-1
	// typed-code-value producer (AnalyseCodeEffect, design/
	// checker-precision-fronts.0.md §1) — types as the effect's Out
	// instead of falling to the dynamic(Any) hatch below. The outs stay
	// DYNAMIC (bounded dynamic(T), never strict T): the effect was
	// computed at the READ site and the body's free words re-resolve
	// against the runtime environment at `do` time, so it is a best
	// static bound, not a proof.
	if body.Carrier() {
		if eff, ok := body.Data().(CodeEffectInfo); ok && eff.Analysed && len(eff.In) == 0 && len(eff.Out) > 0 {
			out := make([]Value, len(eff.Out))
			for i, t := range eff.Out {
				out[i] = NewDynamicCarrier(t)
			}
			return out
		}
	}
	// Escape hatch: a computed body the checker cannot run statically (a
	// list carrier rather than concrete tokens) has a genuinely unknown
	// residual, so emit a bounded gradual dynamic(Any) — optimistically
	// usable downstream — rather than strict Carry<Any>.
	// (design/legacy/dynamic-modality-report.10.ignore, do/eval hatch.) A concrete
	// body is analyzed normally; one that runs to nothing stays strict.
	if !(IsConcrete(&body) && body.Parent().ConformsTo(TList)) {
		generaliseRootValues(r)
		return []Value{NewDynamicCarrier(TAny)}
	}
	// `do` TRAPS every body error at runtime (DoListHandler surfaces it as
	// an Error value), so a guaranteed-runtime-error mirror firing inside
	// this body is not a program error — raise CaughtBodyDepth so those
	// emitters (CheckAddUniqueDiagnostic, emitIndexOOB) stay silent here.
	r.Check.CaughtBodyDepth++
	// A body that raises UNCONDITIONALLY at its own level — a module
	// export's definite no-match, `(true 5 M.dec)` (NUR134) — leaves the
	// failed call's wreckage in the analysed residual, and the wreckage
	// escaped the bracket: re-stepped on the enclosing tape, it dispatched
	// again outside the caught region and was reported as an uncaught
	// program error. At run time `do` catches the raise and yields ONE
	// Error value; model exactly that.
	r.Check.PushRaiseWatch()
	// Leak fidelity: do-body defs stay bound in the enclosing scope, exactly
	// as the runtime leaves them (RunCarrierBodyKeepDefs doc).
	stk := RunCarrierBodyKeepDefs(r, body)
	raised, snap := r.Check.PopRaiseWatch()
	r.Check.CaughtBodyDepth--
	if raised {
		// The defs the body makes AFTER the raise never happen: `do [raise
		// bad_input "boom" def x 1] … x` is undefined_word (or the earlier
		// binding) on the interpreter, and the keep-defs model leaked x = 1
		// into the enclosing scope, which the compiled lane then folded.
		for _, k := range r.Defs.Names() {
			if r.Defs.Depth(k) > snap[k] {
				r.Defs.Truncate(k, snap[k])
			}
		}
		return []Value{NewCarrier(TError)}
	}
	// A def-bound COMPUTED fn read inside the body stands in the residual
	// as its CARRIER (the side table's; the body's check-time run notes no
	// read), where the interpreter's word dispatch calls it — over the
	// frame, which `do` opens empty, or over the tokens after it. The
	// residual is then not the body's: `7 do [a5]` modelled a Function
	// result the program residual applied over the 7 (`[12]` compiled for
	// the interpreter's caught `cannot call`), and `do [a5 7]` an apply
	// the run underflowed (NUR193). Take the computed-body hatch: one
	// bounded dynamic(Any), the run's own result.
	for _, v := range stk {
		if v.Carrier() && v.Parent() != nil && v.Parent().ConformsTo(TFunction) {
			if _, bound := CheckFnCarrierBoundName(r, IDOf(&v)); bound {
				return []Value{NewDynamicCarrier(TAny)}
			}
		}
	}
	if len(stk) == 0 {
		// A NON-EMPTY body that produced an empty residual ran to nothing —
		// for `do` (the error-catching word) that is exactly the shape a
		// raising body leaves: `raise …` (and an integer div/mod by a static
		// zero) yields no carrier, and at runtime `do` catches the error and
		// surfaces it as an Error VALUE (DoListHandler's `NewError(err)`).
		// Model that residual as an Error carrier so a downstream field read
		// (`e dot code`, `e.message`, `convert Map e`) matches the [_ Error]
		// accessor sigs instead of failing no_signature.
		//
		// An EMPTY body (`do []`) genuinely completes with an empty stack — it
		// did NOT raise — so it must stay empty: inventing an Error here would
		// wrongly admit `do [] convert Map` at check time (Error → Map) while
		// the runtime leaves `convert` no argument. Distinguish the two by the
		// body's token count.
		//
		// A body with no DEFINITE raise (that returned above) that ran to
		// nothing may equally have CONSUMED its own values — `do [3 drop]`,
		// `do [args drop]` — and then `do` nets nothing at run time: the
		// count is 0 or (caught) 1, runtime-VARIABLE.
		if bl, err := AsList(body); err == nil && !bl.IsNil() && bl.Len() > 0 {
			// A body of literals and plain stack shuffles that ran to nothing
			// without a raise cannot raise at run time either: `do` runs it
			// isolated (InvokeBody), so each shuffle meets the values the
			// body itself pushed, exactly as it did here — `do [1 drop]` nets
			// nothing, and there is no Error to latch (NUR222: `1 do [1 drop]
			// drop` is [] on both lanes; its consumer had bailed).
			if shuffleOnlyBody(bl.Elems(), r) {
				return nil
			}
			r.Check.ValuelessDoBodies++
			return []Value{NewCarrier(TError)}
		}
		return nil
	}
	// `do` leaves the body's ENTIRE residual stack (DoListHandler returns the
	// full InvokeBody result), so a multi-value literal body — `do [10 20 30]`
	// — nets three values, not one. Returning only the last (stk[len-1])
	// under-reported the arity and made the checked stack contradict the
	// runtime. Mirror the handler: return the full residual.
	return stk
}

// generaliseRootValues is the model's answer to a COMPUTED `do` body run at
// the program root (NUR210): the body keeps its defs in the root's scope,
// and the pass never sees its tokens, so any root value binding may be
// rebound or unbound by it. Each one is generalised IN PLACE — a fresh
// carrier of its type, the speculative undef's own transition
// (GeneraliseSpecUndef) — so no later read can bake the value the pass held
// before the body: a list or map literal, a def's operand and a forward slot
// all read live, where they folded the pre-body value (`do (mk) end [x]`
// answered `[7 [99]]` for `[7 [5]]` over `[def x 5 7]`). The bindings stay,
// so nothing is reported. Only at the root: inside a fn body the body's
// defs land in the frame, whose reads the unit's own leak seats live
// (NUR203), and a body analysed for a call is no statement of the root's.
// Fn values, types and frame bindings keep their models (the transition's
// own exclusions).
func generaliseRootValues(r *Registry) {
	if r.Check.FnBodyDepth > 0 {
		return
	}
	for _, name := range r.Defs.Names() {
		GeneraliseSpecUndef(r, name)
	}
}

// shuffleOnlyBody reports whether a body's tokens are only scalar or list
// literals and plain words of the closed stack-shuffle set whose every
// dispatch has the shuffle's effect and cannot raise (shuffleDispatchSafe),
// and whether its run over only the values it pushes itself — `do` runs a body
// isolated — takes no shuffle past them and leaves nothing: such a run cannot
// raise. `[3 drop]` is one; `[drop]` raises, as `drop` finds nothing to take,
// and `do` nets that Error.
func shuffleOnlyBody(toks []Value, r *Registry) bool {
	depth := 0
	for i, t := range toks {
		if IsConcrete(&t) && t.Parent() != nil && (t.Parent().ConformsTo(TScalar) || t.Parent().Equal(TList)) {
			depth++
			continue
		}
		w, err := AsWord(&t)
		if err != nil || w.ForceVal || w.ForceStack || w.ForceForward || w.ForceUsurp || !DynStackShuffleWords[w.Name] {
			return false
		}
		eff := shuffleEffect[w.Name]
		if !shuffleDispatchSafe(r.Lookup(w.Name), eff, i == len(toks)-1) {
			return false
		}
		if depth < eff[0] {
			return false
		}
		depth += eff[1] - eff[0]
	}
	return depth == 0
}

// shuffleDispatchSafe reports whether every overload a stack-shuffle word
// dispatches to has the shuffle's (takes, leaves) effect and cannot raise
// over values the body pushed. Its dispatch must hold the registered all-Any
// native — stack-only, so it takes every value and is the total fallback — and
// any other overload must be a user extension that CONSUMES its arguments and
// does nothing else: every param named (bound, not left beneath the body), no
// body, no declared return, as many params as the shuffle takes and a shuffle
// that leaves nothing (`def drop fn [[x:P] [] []]` over `def P (refine
// Integer)`). Signature matching never raises (a predicate type's error is a
// no-match), so whichever overload takes the values the effect is the same
// and nothing raises. A forward-collecting extension is admitted only as the
// body's last token, where the isolated body leaves it nothing to collect.
func shuffleDispatchSafe(fd *FnDefInfo, eff [2]int, last bool) bool {
	if fd == nil || fd.Macro {
		return false
	}
	natives := 0
	for i := range fd.Signatures {
		s := &fd.Signatures[i]
		if _, native := s.Impl.(*GoImpl); native {
			if s.BarrierPos != 0 || len(s.Args) != eff[0] {
				return false
			}
			for _, a := range s.Args {
				if a != TAny {
					return false
				}
			}
			natives++
			continue
		}
		if !consumingNoOpSig(s, eff, last) {
			return false
		}
	}
	return natives == 1
}

// consumingNoOpSig reports whether a user overload binds its arguments and
// does nothing else, with the shuffle's effect (see shuffleDispatchSafe).
func consumingNoOpSig(s *Signature, eff [2]int, last bool) bool {
	if _, boru := s.Impl.(*BoruImpl); !boru || s.Fallback || len(s.Body()) != 0 || len(s.Returns) != 0 ||
		eff[1] != 0 || len(s.Params) != eff[0] || (s.BarrierPos != 0 && !last) {
		return false
	}
	for _, p := range s.Params {
		if p.Name == "" || p.Optional || p.Quote {
			return false
		}
	}
	return true
}

// shuffleEffect is each stack-shuffle word's (takes, leaves) count.
var shuffleEffect = map[string][2]int{
	"dup": {1, 2}, "swap": {2, 2}, "drop": {1, 0}, "over": {2, 3}, "rot": {3, 3},
	"nip": {2, 1}, "tuck": {2, 3}, "dup2": {2, 4}, "swap2": {4, 4}, "drop2": {2, 0},
	"over2": {4, 6},
}

func DoMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	result, err := DoEvalMapValue(r, args[0])
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// DoEvalList evaluates a top-level list of tokens in a sub-engine.
// Errors are caught and returned as a single error value on the stack.
func DoEvalList(r *Registry, elems []Value) ([]Value, error) {
	sub := New(r)
	result, err := sub.Run(elems)
	if err != nil {
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

// doEvalDataList evaluates a list value inside a `do` map as code.
// Unquoted words in the list arrive as Word values and run normally
// (`do {a:[add 1 2]}` → {a:3}); quoted strings and atoms are DATA and
// are left untouched — a `do {a:["if"]}` stores the string "if", it
// does not dispatch the `if` word. Respecting the quote is what keeps
// data values whose text happens to name a word (`"if"`, `"get"`,
// `"do"`) storable without boxing tricks (voxgig DX report T4).
func doEvalDataList(r *Registry, elems []Value) ([]Value, error) {
	// A STEPLESS list is its own residual, so the sub-engine run is the
	// identity on it and is skipped.
	//
	// This exists because the COMPILED lane arrives here with the work already
	// done. `do {n:[a add 1]}` inside a fn body records as the computation
	// followed by MAKE_LIST over its RESULT — the disassembly is PUSH_LOCAL /
	// CALL_NATIVE add / MAKE_LIST / MAKE_MAP — so the list this walks is `[6]`,
	// and starting an engine to step the literal 6 is an interpreter entry
	// inside a compiled program that buys nothing. The INTERPRETER reaches the
	// same source as [Word(a) Word(add) 1], which is not stepless, so its lane
	// is untouched and the two engines still agree.
	if IsSteplessWindow(elems) {
		return append([]Value(nil), elems...), nil
	}
	return New(r).Run(elems)
}

// DoEvalMapValue recursively evaluates list values within a map. Used
// by `do` to walk a map literal and evaluate any embedded code lists.
func DoEvalMapValue(r *Registry, v Value) (Value, error) {
	if v.Parent().Equal(TList) && v.Data() != nil && !IsTypedList(&v) && !IsTableType(v) {
		_lst, _ := AsList(v)
		results, err := doEvalDataList(r, _lst.Elems())
		if err != nil {
			return Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return NewList(results), nil
	}
	if v.Parent().Equal(TMap) && v.Data() != nil && !IsTypedMap(&v) && !IsRecordType(v) && !IsOptionsType(v) {
		m, _ := AsMap(v)
		out := NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := DoEvalMapValue(r, val)
			if err != nil {
				return Value{}, err
			}
			out.Set(key, evaluated)
		}
		return NewMap(out), nil
	}
	return v, nil
}

// ---- if handlers ----

// ifMarkMoveTokens builds the mark+move token sequence for a list-form `if`
// condition: the condition list is run inline (Mark…Move) and its result
// selects the branch via the IfCont. Returns (nil, false) when cond is not a
// runnable plain list, so the caller falls back to scalar-condition
// coercion. Shared by if2Handler (elseBranch=nil) and if3Handler.
func ifMarkMoveTokens(cond Value, thenBranch, elseBranch []Value, pos SrcPos) ([]Value, bool) {
	if !(cond.Parent().Equal(TList) && cond.Data() != nil && !IsTypedList(&cond) && !IsTableType(cond)) {
		return nil, false
	}
	_lst, _ := AsList(cond)
	condSlice := _lst.Elems()
	id := NextMarkID()
	tokens := make([]Value, 0, len(condSlice)+2)
	tokens = append(tokens, NewMark(id))
	tokens = append(tokens, condSlice...)
	// The move carries the `if`'s position: a condition that nets no value
	// raises there (stepMoveIf), as the compiled guard does (NUR292).
	tokens = append(tokens, WithPosAt(NewMoveIf(id, "if", &IfCont{
		Then: thenBranch,
		Else: elseBranch,
	}), pos))
	return tokens, true
}

func if3Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])
	elseBranch := spliceArg(args[2])

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, elseBranch, r.Check.CurWordPos); ok {
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return elseBranch, nil
}

func if2Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, nil, r.Check.CurWordPos); ok {
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return nil, nil
}

// if3ReturnsFn is the check-mode model of the three-operand `if`: the
// condition is analysed statically (a literal or folded Boolean decides the
// arm; anything else leaves both arms possible), each code-body arm is
// analysed under the narrowing its condition licenses, and the result is the
// join of the arms' residuals.
func if3ReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 3 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	// The dead-arm warning for a bare concrete-Boolean condition is a claim
	// about the CODE (warnStaticIfDeadArm).
	warnStaticIfDeadArm(r, args[0], true)
	// Static reduction (the else-less-if soundness fix,
	// forward-barrier.tsv:83): a paren comparison folds to a bare concrete
	// Boolean, so reduce to the taken arm and return a bare-VALUE arm as-is,
	// letting a trailing forward token dispatch against it.
	if out, ok := ReduceStaticIf(r, args[0], args[1], &args[2]); ok {
		return out
	}
	if lit, ok := LiteralCondValue(args[0]); ok {
		branch, arm := "else", args[1]
		if !lit {
			branch, arm = "then", args[2]
		}
		EmitUnreachableBranch(r, lit, branch)
		return ifTakenArmReturns(r, args[0], arm, lit)
	}
	// The then arm may be a `[…]` code body or an already-evaluated VALUE
	// (`if cond 99 88` — a literal, a def-bound value, a paren result),
	// exactly like the else arm below. A non-body arm is its own value.
	// An arm that may not run is bracketed (EnterSpecArm): a fn def inside
	// one is speculative. Only the arm a DECIDED condition takes goes
	// unbracketed — its defs are the post-branch bindings exactly
	// (armsKnownToRun).
	thenRuns, elseRuns := armsKnownToRun(args[0])
	thenStk, thenDefs := ifArmResidual(r, args[0], args[1], thenRuns, ApplyGuardNarrowing)
	elseStk, elseDefs := ifArmResidual(r, args[0], args[2], elseRuns, ApplyComplementNarrowing)
	InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		// BOTH arms produce 0 values (empty `[]`, a 0-value word, or a
		// diverging break/continue/raise): the if is a 0-value STATEMENT, not
		// a value-producing branch, and nets 0 like the runtime.
		return nil
	}
	// Variadic fold (recursion.tsv:53): when an arm's residual carries a
	// variadic-spread (a `[]`-declared recursive fn's seeded self-call), the
	// whole `if` yields a single variadic spread of the per-frame leaked
	// type, absorbing the base arm's emptiness.
	if v, ok := FoldVariadicArms(thenStk, elseStk); ok {
		return []Value{v}
	}
	return []Value{joined[len(joined)-1]}
}

// ifArmResidual analyses one arm of an undecided `if`: a `[…]` code body
// runs under the narrowing its condition licenses (narrow), bracketed as a
// speculative arm unless the model knows it runs; any other arm is its own
// value.
func ifArmResidual(r *Registry, cond, arm Value, runs bool, narrow func(*Registry, Value) func()) ([]Value, map[string]Value) {
	if !(IsConcrete(&arm) && arm.Parent().ConformsTo(TList)) {
		return []Value{arm}, nil
	}
	restore := narrow(r, cond)
	defer restore()
	var stk []Value
	var defs map[string]Value
	func() {
		defer r.EnterSpecArm(runs)()
		stk, defs = RunCarrierBodyWithDefs(r, arm)
	}()
	return stk, defs
}

// ifTakenArmReturns models a branch whose condition is DECIDED at check
// time: only the taken arm runs. Two callers: if3's literal condition
// (`if [true] … …`, lit the literal) and the clause-list `if`'s statically
// decided clause (a scalar condition, or the lone else, always lit=true).
// The arm is a code body; the narrowing the condition licenses is installed
// around it. An arm that nets no value is a 0-value statement (NUR243).
func ifTakenArmReturns(r *Registry, cond, arm Value, lit bool) []Value {
	var stk []Value
	var defs map[string]Value
	if lit {
		restoreThen := ApplyGuardNarrowing(r, cond)
		stk, defs = RunCarrierBodyWithDefs(r, arm)
		restoreThen()
		InstallTakenArmDefs(r, defs, nil)
	} else {
		restoreElse := ApplyComplementNarrowing(r, cond)
		stk, defs = RunCarrierBodyWithDefs(r, arm)
		restoreElse()
		InstallTakenArmDefs(r, nil, defs)
	}
	if len(stk) == 0 {
		// The taken arm leaves no value (`if [true] [def x 1] [2]`, NUR243):
		// a 0-value STATEMENT, so the if nets 0, as the run does.
		return nil
	}
	// An arm that leaves a FN VALUE (`if [true] [g/v] [1]`) hands back
	// nothing: the handler splices the arm as a paren, whose fn result
	// stays a placed value on the interpreter (`fn g`), but handed back from
	// here it would be re-stepped by the check engine, which models the
	// call — 5, and `10 if [true] [g/v] [1]` applied g to the 10 beneath.
	if IsFnValueResidual(stk[len(stk)-1]) {
		return nil
	}
	return []Value{stk[len(stk)-1]}
}

// staticCondArm reports the taken arm for a statically-known BARE concrete
// Boolean condition — the shape a paren comparison folds to in plain check
// (`(n eq 0)` → concrete false via CompileScalarFold, carrier.go). It is the
// bare-value analogue of LiteralCondValue (which reads a length-1 LIST body)
// and reuses condSelectsArm's concrete-Boolean probe. ok=false for a
// list-form / dynamic / non-Boolean condition, so the caller falls through to
// the existing const/join analysis.
func staticCondArm(cond Value) (takeThen bool, ok bool) {
	if IsConcrete(&cond) && cond.Parent().ConformsTo(TBoolean) {
		if b, err := AsBoolean(cond); err == nil {
			return b, true
		}
	}
	return false, false
}

// ReduceStaticIf reduces an `if` whose condition is a statically-known bare
// Boolean to its TAKEN arm. It returns the taken arm's residual directly, with NO
// join against the dead arm — the soundness fix for forward-barrier.tsv:83,
// where the else arm is a bare module-export Function value that must
// forward-collect the token following the `if`. elseArm is nil for the 2-arg
// (if2) form: a false condition then nets no value (matching the runtime).
func ReduceStaticIf(r *Registry, cond, thenArm Value, elseArm *Value) ([]Value, bool) {
	takeThen, ok := staticCondArm(cond)
	if !ok {
		return nil, false
	}
	// The dead-arm warning is NOT made here: the caller made it already
	// (warnStaticIfDeadArm), on both passes, before choosing this reduction.
	if takeThen {
		return reduceStaticArm(r, cond, thenArm, true), true
	}
	if elseArm == nil {
		return nil, true // if2, false: the then is unreachable and nothing runs
	}
	return reduceStaticArm(r, cond, *elseArm, false), true
}

// warnStaticIfDeadArm emits the unreachable_branch warning for an `if`
// whose condition is a statically-known BARE concrete Boolean
// (staticCondArm) — but only when a dead arm actually exists: a true
// condition on the 2-arg form (hasElse false) has no else to call
// unreachable. It is the warning half of ReduceStaticIf.
func warnStaticIfDeadArm(r *Registry, cond Value, hasElse bool) {
	takeThen, ok := staticCondArm(cond)
	switch {
	case !ok:
	case !takeThen:
		EmitUnreachableBranch(r, false, "then")
	case hasElse:
		EmitUnreachableBranch(r, true, "else")
	}
}

// EmitUnreachableBranch records the constant-condition dead-branch warning
// shared by the if2/if3 const paths.
func EmitUnreachableBranch(r *Registry, lit bool, dead string) {
	// A dead branch is a claim about the CODE; a body analysed with a
	// concrete argument bound is a claim about ONE CALL. Suppress inside a
	// call-shape specialisation — the declaration-shaped run of the same body
	// (carrier args, CallShapeDepth == 0) is the one entitled to speak, and it
	// is the run whose verdict holds for every caller.
	if r.Check.CallShapeDepth > 0 {
		return
	}
	r.Check.AddDiagnostic(CheckDiagnostic{
		Code:   "unreachable_branch",
		Detail: "if condition is a constant " + BoolWord(lit) + "; " + dead + "-branch is unreachable",
		Word:   "if",
		Row:    r.Check.CurCallPos.Row,
		Col:    r.Check.CurCallPos.Col,
	})
}

// reduceStaticArm returns the taken arm's residual. A list code body runs via
// RunCarrierBodyWithDefs (with the arm's guard narrowing installed, exactly as
// the const path) and contributes its FULL residual stack, matching the
// runtime splice. A bare VALUE arm (a module-export fn value, a literal, a
// def-bound value, a typed list) is returned AS-IS — so a trailing forward
// token dispatches against it in the shared engine loop (`… MathUtil.sqrt 16`
// → the else fn value forward-collects 16 → sqrt(16)=Float), exactly as the
// runtime's spliceArg does. The isCodeBody test mirrors spliceArg's own
// wrap-and-execute condition (conditional.go), so check and runtime agree on
// which arms are bodies.
func reduceStaticArm(r *Registry, cond, arm Value, isThen bool) []Value {
	if !isCodeBody(arm) {
		return []Value{arm}
	}
	var stk []Value
	var defs map[string]Value
	if isThen {
		restore := ApplyGuardNarrowing(r, cond)
		stk, defs = RunCarrierBodyWithDefs(r, arm)
		restore()
		InstallTakenArmDefs(r, defs, nil)
	} else {
		restore := ApplyComplementNarrowing(r, cond)
		stk, defs = RunCarrierBodyWithDefs(r, arm)
		restore()
		InstallTakenArmDefs(r, nil, defs)
	}
	return stk
}

// armsKnownToRun reports which arm of a branch the model knows RUNS: the
// taken arm of a DECIDED condition — a literal `[true]` / `[false]` — whose
// defs are the post-branch bindings exactly. Every other arm may not run, so
// a fn def in it is bound at run time only if it does — speculative, as an
// undecided arm's is. The arm a decided condition skips is the NEVER case of
// that (NUR244: left unbracketed, the join kept its fn value and a read past
// the merge called it, `if false [def f fn […]] [] end 3 f` answering the
// fn's result for undefined_word). A bare concrete Boolean (a def-bound or
// folded one) never reaches here: ReduceStaticIf takes it first, on both
// `if` forms, and analyses the taken arm alone.
func armsKnownToRun(cond Value) (thenRuns, elseRuns bool) {
	if lit, ok := LiteralCondValue(cond); ok {
		return lit, !lit
	}
	return false, false
}

func If2ReturnsFn(args []Value, r *Registry) []Value {
	// A statically-false bare condition's dead then-arm warns (a true one
	// has no dead arm without an else), as in if3ReturnsFn.
	warnStaticIfDeadArm(r, args[0], false)
	// Static reduction (else-less if): a folded bare-Boolean condition
	// reduces to the then residual (true) or nothing (false), instead of the
	// phantom Disjunct(then, None) the join path produces.
	if out, ok := ReduceStaticIf(r, args[0], args[1], nil); ok {
		return out
	}
	if lit, ok := LiteralCondValue(args[0]); ok && !lit {
		EmitUnreachableBranch(r, false, "then")
	}
	restore := ApplyGuardNarrowing(r, args[0])
	// The arm runs only when the condition holds: bracketed unless the model
	// decides it true, like if3's arms (NUR244 — unbracketed, a fn def in
	// the arm was the join's own value, called past the merge on the path
	// that skipped it).
	thenRuns, _ := armsKnownToRun(args[0])
	var thenStk []Value
	var thenDefs map[string]Value
	func() {
		defer r.EnterSpecArm(thenRuns)()
		thenStk, thenDefs = RunCarrierBodyWithDefs(r, args[1])
	}()
	restore()
	InstallJoinedDefs(r, thenDefs, nil)
	// 2-arg if: a VARIADIC result (0 or 1 values at run time). An empty
	// then-stack (a 0-value/diverging then) makes it a 0-value statement
	// guard, which nets 0 like the runtime.
	if len(thenStk) == 0 {
		return nil
	}
	return []Value{JoinCarriers(thenStk[len(thenStk)-1], NewCarrier(TNone))}
}

// IfListHandler implements the clause-list form `if [c1 b1 c2 b2 … else]`.
// It hands the (raw, NoEval'd) list's elements to ifClause, which produces
// the token stream the engine then runs.
func IfListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return nil, r.BoruError("if_error", "if: clause-list argument must be a concrete list, got a type literal", "if")
	}
	_lst, _ := AsList(args[0])
	return ifClause(_lst.Elems(), r.Check.CurWordPos), nil
}

// CaseHandler implements both call shapes of `case`:
//
//	case <value> [m1 b1 … default]    forward form (canonical)
//	<value> case [m1 b1 … default]    stack-value form
//
// Disambiguation: in the forward form args[0]=value, args[1]=clause
// list; in the stack form the matcher delivers args[0]=clause list
// (forward) and args[1]=value (stack). When exactly one arg is a
// plain list it is the clause list; when BOTH are lists the forward
// reading wins (args[0] is the value — so `case [1 add 1] [clauses]`
// evaluates the code body; dispatching on a LIST value requires the
// forward form). The value, if a code body, is executed and its LAST
// result captured — it must produce one, loudly.
func CaseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v, clauses := args[0], args[1]
	if isCodeBody(v) && !isCodeBody(clauses) {
		v, clauses = clauses, v
	}
	v, err := caseSubject(r, v)
	if err != nil {
		return nil, err
	}
	if !isCodeBody(clauses) {
		return nil, r.BoruError("case_error",
			"case: clause list must be a concrete list of match/block pairs (optional trailing default)", "case")
	}
	lst, _ := AsList(clauses)
	return CaseClauses(r, v, lst.Elems())
}

// caseSubject is case's scrutinee rule: a code body runs in a sub-engine and
// its LAST result is the scrutinee — it must produce one, loudly — and any
// other value is itself.
func caseSubject(r *Registry, v Value) (Value, error) {
	if !isCodeBody(v) {
		return v, nil
	}
	lst, _ := AsList(v)
	out, err := New(r).Run(lst.Elems())
	if err != nil {
		return Value{}, err
	}
	if len(out) == 0 {
		return Value{}, r.BoruError("case_error",
			"case: value expression produced no value to dispatch on", "case")
	}
	return out[len(out)-1], nil
}

// IfListReturnsFn type-checks the clause-list form: the result is the
// join of every clause body's last value plus the else clause (or None
// when there is no else, since an unmatched `if` produces nothing).
// Condition bodies are still run for their diagnostics but don't
// contribute to the return type. Unlike if3/if2 this does no per-clause
// guard narrowing — multi-clause narrowing isn't modelled.
func IfListReturnsFn(args []Value, r *Registry) []Value {
	if !IsConcrete(&args[0]) || !args[0].Parent().Equal(TList) {
		return []Value{NewCarrier(TAny)}
	}
	_lst, _ := AsList(args[0])
	elems := _lst.Elems()

	var joined []Value
	add := func(stk []Value) {
		if joined == nil {
			joined = stk
		} else {
			joined = JoinCarrierStacks(joined, stk)
		}
	}

	i := 0
	for ; i+1 < len(elems); i += 2 {
		if isCodeBody(elems[i]) {
			RunCarrierBody(r, elems[i]) // run the condition body for diagnostics only
		}
		add(RunCarrierBody(r, elems[i+1]))
	}
	if i < len(elems) {
		add(RunCarrierBody(r, elems[i])) // lone else
	} else {
		add([]Value{NewCarrier(TNone)}) // no else: an unmatched if yields nothing
	}

	if len(joined) == 0 {
		return nil
	}
	return []Value{joined[len(joined)-1]}
}

func ForCountHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// Reject a non-concrete count (a DepScalar/refinement Integer, a carrier)
	// rather than silently coercing it to zero and running the loop zero
	// times — the VM's OpForSetup (eng/go/vm.go) raises for_error here, so
	// both engines must agree instead of one looping and the other erroring.
	n, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("for_error", "for: count must be a concrete Integer", "for")
	}
	body := args[1]
	return RunForLoop(r, 0, n, 1, "i", body)
}

func ForRangeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return nil, r.BoruError("for_error", "for: range must be a concrete list, got type literal", "for")
	}
	_lst, _ := AsList(args[0])
	rangeSpec := _lst.Elems()
	body := args[1]
	start, end, step, err := ParseRange(rangeSpec)
	if err != nil {
		// for_error matches the VM's OpForSetup taxonomy (eng/go/vm.go) so a
		// malformed/non-concrete range errors the same way in both engines.
		return nil, r.BoruError("for_error", "for: "+err.Error(), "for")
	}
	return RunForLoop(r, start, end, step, "i", body)
}

func forIntegerListReturnsFn(args []Value, r *Registry) []Value {
	return forCarrierAnalyse(r, "i", TInteger, args, 0)
}

func forListListReturnsFn(args []Value, r *Registry) []Value {
	return forCarrierAnalyse(r, "i", TInteger, args, 0)
}

// forCarrierAnalyse analyses the body to a bounded fixed point with
// the iterator bound as a typed carrier (AnalyseLoopBody —
// design/legacy/checker-accuracy-review.10.ignore A4): body rebindings like
// `def acc (acc add 0.5)` join back into the enclosing binding and
// the body re-runs until the bindings stabilise, so post-loop reads
// see Integer|Float, not the pre-loop Integer. Returns a typed list
// whose element type mirrors the final round's residual top.
//
// The List carrier is a STATIC APPROXIMATION of the result type, not a
// claim that the loop leaves one List value: at run time the engine
// splices the per-iteration values onto the stack as separate entries
// (`for 3 [i]` leaves `0 1 2`, not `[0,1,2]`); a STATICALLY-COUNTED loop
// is modelled as that spread (below).
//
// countArg >= 0 names the count/range operand: the count form is the
// range [0, n, 1]; the range form decomposes a LITERAL integer range via
// ParseRange.
func forCarrierAnalyse(r *Registry, iterName string, iterType *Type, args []Value, countArg int) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	body := args[len(args)-1]
	iter := NewCarrier(iterType)

	// Statically-zero loop pruning: a `for` whose count operand is a CONCRETE
	// non-positive Integer never enters its body — at run time both engines
	// iterate zero times and push zero values (`for 0 [body]` leaves the stack
	// untouched). Its body is unreachable, so analysing it is both wasted work
	// and a source of false findings: a body that only type-checks for a live
	// iteration — e.g. module-test:38's `for (subs size) [subspec run-spec]`
	// over `subs: []`, whose recursive `run-spec` over a carrier `subspec`
	// cannot dispatch — would otherwise poison the program for a branch that
	// never runs. Prune it: contribute no values, and skip body analysis
	// entirely. Faithful because the interpreter's `for 0` runs the body zero
	// times (no side effects, no values).
	//
	// The fold only fires for a STATIC count; a carrier/computed count keeps the
	// full analysis below. countArg-as-list (a literal range) is pruned too
	// when it decodes to an empty span.
	if countArg >= 0 {
		if cv := args[countArg]; IsConcrete(&cv) {
			zero := false
			switch {
			case cv.Parent().ConformsTo(TInteger):
				if n, err := AsInteger(cv); err == nil && n <= 0 {
					zero = true
				}
			case cv.Parent().ConformsTo(TList):
				if lst, err := AsList(cv); err == nil && !lst.IsNil() {
					if st, en, sp, perr := ParseRange(lst.Elems()); perr == nil && LoopIterations(st, en, sp) == 0 {
						zero = true
					}
				}
			}
			if zero {
				return []Value{}
			}
		}
	}

	// Decompose the loop bounds. staticBounds means ALL THREE bounds are
	// concrete integers, so the exact iteration count is known — required
	// for the spread-residual model below, which AsInt64Or-defaults an
	// unknown bound to 0 and would otherwise starve a computed loop's
	// residual.
	var startV, endV, stepV Value
	staticBounds := false
	if countArg >= 0 {
		cv := args[countArg]
		switch {
		case cv.Parent().ConformsTo(TInteger):
			startV, endV, stepV = NewInteger(0), cv, NewInteger(1)
			staticBounds = IsConcrete(&cv)
		case IsConcrete(&cv) && cv.Parent().ConformsTo(TList):
			if lst, err := AsList(cv); err == nil && !lst.IsNil() {
				if st, en, sp, perr := ParseRange(lst.Elems()); perr == nil {
					startV, endV, stepV = NewInteger(st), NewInteger(en), NewInteger(sp)
					staticBounds = true
				}
			}
		}
	}
	// provenTrips: the loop provably runs at least once (all three bounds
	// static, count >= 1 — the statically-zero prune above already returned
	// for a static zero-trip). It arms the S9.2a LoopBodyDepth stamp inside
	// AnalyseLoopBody; a COMPUTED count must not (a runtime count of 0 runs
	// the body zero times and would leak an admitted split's analysis-only
	// binding — PR #280 review).
	provenTrips := staticBounds &&
		LoopIterations(AsInt64Or(startV, 0), AsInt64Or(endV, 0), AsInt64Or(stepV, 1)) >= 1
	stk := AnalyseLoopBody(r, body, []string{iterName}, []Value{iter}, provenTrips)
	out := NewCarrier(TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		out = CarrierTypedListOf(top)
	}
	// A body that nets ZERO values per iteration (every pass drops / is pure
	// side effect) leaves the stack untouched at run time, whatever the
	// (possibly carrier) count. Model that exactly: contribute no residual.
	// Returning the List-carrier approximation here instead over-counts a
	// trailing statement's arity — the false "expected 1 return value(s),
	// got 2" a side-effect loop with a carrier count produced (mini-s3's
	// s3-send-resp / s3-read-body-into / s3-drain-body).
	if len(stk) == 0 {
		return []Value{}
	}
	// A STATICALLY-COUNTED loop leaves the SPREAD of its per-iteration
	// residual on the stack, not one List value (`for 3 ['x']` leaves `'x'
	// 'x' 'x'`; `for [1 4] [7 8]` leaves six ints). Model that exactly so the
	// residual's arity and element types match the runtime — the List above
	// is the approximation for the counts the model cannot spread.
	// The per-iteration residual is the loop's fixed-point join, so repeating it
	// count times is a sound over-approximation: a break/continue loop runs
	// FEWER iterations, and a shorter runtime stack is still covered by the
	// longer checked one. Bounded to keep the residual small; past the cap the
	// List approximation stands. CONCRETE count only: a carrier count decomposes
	// to a carrier endV, which AsInt64Or would default to 0 — spreading a
	// runtime-unknown iteration count to an EMPTY residual and starving every
	// downstream consumer (`size (for n [n])` failed no_signature on nothing).
	if staticBounds {
		if n := LoopIterations(AsInt64Or(startV, 0), AsInt64Or(endV, 0), AsInt64Or(stepV, 1)); n >= 0 && n*int64(len(stk)) <= loopSpreadResidualCap {
			spread := make([]Value, 0, int(n)*len(stk))
			for i := int64(0); i < n; i++ {
				spread = append(spread, stk...)
			}
			return spread
		}
	}
	return []Value{out}
}

// loopSpreadResidualCap bounds the check-mode spread of a statically-counted
// loop's residual (native_control forCarrierAnalyse). A loop with more total
// residual values than this keeps the single-List approximation rather than
// materialising a large residual stack — the exact arity of a big loop is not
// worth the memory, and such loops rarely leave a consumed residual.
const loopSpreadResidualCap = 256

// AsInt64Or returns v's integer value, or def when v is not a concrete integer.
func AsInt64Or(v Value, def int64) int64 {
	if IsConcrete(&v) && v.Parent().ConformsTo(TInteger) {
		if n, err := AsInteger(v); err == nil {
			return n
		}
	}
	return def
}

// ---- error handler ----

// errorPassHandler is `error`'s success path: the guarded body
// produced a normal value, so the handler list is discarded and the
// value passes through unchanged.
// ErrorReturnsFn narrows `error`'s result bound to the JOIN of its two
// runtime paths — the pass-through do-result (no raise) and the handler
// body's netted residual over a caught Error (the check twin of
// ErrorHandler's InvokeBody([err, body…]) stream). The bound stays DYNAMIC
// (which path runs is a runtime fact), so a downstream dispatch over it uses
// not-disjoint matching against a REAL family instead of Any — the L-EACH
// graduation (`5 do [7] error [drop 9] add 1`): with dynamic(Integer) the
// String catch-all overload of `add` is disjoint and check mode selects the
// same forward collection the interpreter takes, so declineForwardStackDrift
// has nothing to decline. Anything inconclusive — a non-token handler, a
// multi-value or empty handler residual, a nil parent — keeps the historical
// dynamic(Any), so genuinely dynamic boundaries keep declining.
//
// The seeded body run covers the PLAIN pass too (NUR049, un-gated
// 2026-08-03): `error` handler bodies were the one body the checker never
// visited, so `do [raise bad_input "x"] error [zzz-undefined]` checked
// clean and failed at runtime. Running the same seeded analysis on both
// passes makes the handler-body diagnostic surface identical to the
// compile pass's (one-diagnostic-surface,
// checker-compiler-completeness-review.0.md §8.4.2) — and it is
// corpus-safe by construction: every corpus row already passes this
// analysis in the compile pass (an error-severity handler diagnostic
// would have tripped the compile failure gate at 0).
func ErrorReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	wide := []Value{NewDynamicCarrier(TAny)}
	if !IsConcrete(&args[0]) || args[1].Parent() == nil {
		return wide
	}
	body, err := AsList(args[0])
	if err != nil || body.IsNil() {
		return wide
	}
	seeded := append([]Value{NewCarrier(TError)}, body.Elems()...)
	stk := RunCarrierBody(r, NewList(seeded))
	// The handler nets ONE value (BodyOut 1): the seeded error still at the
	// residual bottom is the strip-unconsumed shape (`["fallback"]`), so drop
	// it before the count check — mirroring ErrorHandler's identity probe.
	if len(stk) >= 2 && IDOf(&stk[0]) == IDOf(&seeded[0]) {
		stk = stk[1:]
	}
	if len(stk) == 0 {
		// We RAN the handler, so this arity is KNOWN — not unknown, which is
		// what `wide` means everywhere else in this function. Widening a
		// known-ZERO arity to a one-value bound is the whole of defect C:
		// `error`'s dispatch is then recorded as a single-output fallback
		// island (RecordFallback takes outs[0]), FallbackSpan has no
		// out-count field to say otherwise, lowerFallback pushes exactly one
		// simulated slot, and at run time runFallback appends the island's
		// real zero results — leaving the stack one short and surfacing as
		// `CALL_DYNAMIC underflow` / `BIND_GLOBAL underflow`, an
		// internal_error leaked to the user from `do [risky] error [drop]`
		// followed by any expression.
		//
		// Declining is the sanctioned response: the island model cannot
		// express this program, and under the compile-failure contract the
		// compile failure is the honest outcome. It is also what the adjacent
		// no-error path already does for its own unrepresentable shape (a
		// baked arg beyond BarrierPos — carrier.go's island decline).
		//
		// ZERO specifically, not "!= 1". A residual of two or more is NOT
		// broken: its bottom is the unconsumed seeded error, and the paired
		// CLOSURE path nets one from it with a runtime strip
		// (TestErrorStripInputClosure pins `error [dup drop "k"]`, which
		// measures 2 here because the compile-time strip's identity probe
		// does not match after a dup/drop). Declining those regressed a shape
		// that compiles correctly today. A residual >1 that the strip cannot
		// reduce is already declined further down the pipeline, so it needs
		// nothing from here either.
		//
		// A PROVEN raise (a strict, non-dynamic Error do-result — the body
		// always raises, so the pass-through arm is statically dead) makes
		// the zero a FIXED arity, not a variable one: the handler runs on
		// every execution and nets nothing, so the dispatch records a
		// 0-output call — the same truth-telling the zero-return user-fn
		// path performs — and the residual matches the runtime exactly
		// (completeness-review §8.2(6), the zero-netting-handler
		// graduation). A DYNAMIC Error bound keeps the compile failure: there the
		// runtime may not raise, the pass-through nets one where the caught
		// path nets zero, and a fixed seat cannot carry both.
		if !IsDynamic(&args[1]) && args[1].Parent() != nil && args[1].Parent().ConformsTo(TError) {
			return nil
		}
		// The arity is variable, not unknown: ZERO on the caught path, ONE
		// on the pass-through. A fixed seat cannot carry both — which is
		// what the compile failure here said — but a runtime-variadic REGION can,
		// and it is the same device await's winner-takes-all residual and a
		// value-producing loop already ride (the forty-eighth increment).
		// One recorded slot stands for the whole run, callVariadicRegion
		// marks the dispatch, and the residual absorbs whatever the run
		// delivers. A consumer that needs a fixed count still declines, at
		// its own gate, over a region the recorder can name.
		return []Value{NewVariadicCarrier(NewTypeLiteral(TAny))}
	}
	if len(stk) != 1 || stk[0].Parent() == nil {
		return wide
	}
	// A PROVEN-Error do-result (a strict Error carrier — the body always
	// raises, DoListReturnsFn's raising-residual arm) makes the pass-through
	// arm statically dead: the result is the handler's alone, no join. A
	// dynamic(Error) bound keeps the join — the bound is best-effort, not
	// proof, so the pass-through may still run.
	if !IsDynamic(&args[1]) && args[1].Parent().ConformsTo(TError) {
		return []Value{NewDynamicCarrier(stk[0].Parent())}
	}
	return []Value{NewDynamicCarrier(CommonAncestorType(args[1].Parent(), stk[0].Parent()))}
}

func ErrorHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(&args[0]) {
		return nil, r.BoruError("error_error", "error: handler must be a concrete list, got type literal", "error")
	}
	// Success pass-through: a non-Error do result skips the handler and passes
	// through unchanged (`do [risky] error [handler]` composes when risky
	// SUCCEEDS too). The runtime branch is what lets `error` keep ONE (List, Any)
	// sig — a mono dispatch whose handler body compiles as a closure — while still
	// doing the right thing on the dynamically-error-or-value do result.
	if !IsError(args[1]) {
		return []Value{args[1]}, nil
	}
	// Run the handler with the caught error pushed as its one input. Routed
	// through InvokeBody so a compiled handler closure runs VM-native (r.Invoker
	// set); with no Invoker a fresh sub-engine runs the reconstructed token
	// stream — byte-identical to the historical `New(r).Run([err, body…])`.
	out, err := InvokeBody(r, args[0], []Value{args[1]})
	if err != nil {
		return nil, err
	}
	// Stack-neutrality (decision DX report finding 6): the caught error
	// is PUSHED so the handler can bind it (`var [[e] …]`, `get code`,
	// `dup`, …), but a handler that ignores it must not leak it beneath
	// its result — `def r do [risky] error ["fallback"]` used to bind r
	// to the error's neighbour and auto-print the stray error. If the
	// pushed error is still sitting unconsumed at the BOTTOM of the
	// handler's stack, strip it, so the error branch leaves exactly the
	// handler's result — mirroring the success pass-through. (A bare
	// `error []` handler keeps the error as the result: pass-through.
	// The identity probe compares ErrorInfo payloads; a non-ErrorInfo
	// bottom is a different dynamic type and compares false.)
	if len(out) >= 2 && out[0].Data() == args[1].Data() {
		out = out[1:]
	}
	return out, nil
}
