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
		// do [body] — runs the body with no inputs and returns its ENTIRE
		// residual (DoListHandler returns InvokeBody's full result), so the
		// closure compiles count-agnostic (BodyOutResidual) and a multi-value
		// literal body (`do [10 20 30]`) lowers to a true closure whose unit
		// RETs all N values — no longer a baked-const list re-run through an
		// interpreter sub-engine at run time.
		// BodyOnceKeepsDefs: DoListHandler runs the body exactly once and
		// its defs leak to the enclosing scope (RunCarrierBodyKeepDefs is
		// the check-mode twin), which licenses the twin regime to replay
		// body-noted bind twins after the call — see the field's doc.
		Callable: &CallableSpec{BodyPos: 0, BodyOut: BodyOutResidual, BodyOnceKeepsDefs: true, Inputs: func(_ []Value) []Value {
			return []Value{}
		}},

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
				// refusing — the handler's runtime execution is the
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
			},
			{
				Args:       []*Type{TAny, TAny},
				NoEvalArgs: map[int]bool{0: true, 1: true},
				Impl:       Go(if2Handler),
				ReturnsFn:  If2ReturnsFn, BarrierPos:

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
		// __casematch is the internal guard the compiled `case` desugar
		// emits for a non-predicate clause: `v match __casematch` applies the
		// SAME UnifyR the interpreter's CaseClauses uses, so a bare-refine
		// newtype (`Pos`) matches structurally exactly as case does — which
		// the `is` word (nominal) would not. Not user-facing.
		Name: "__casematch",
		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			Impl:       Go(CaseMatchHandler),
			Returns:    []*Type{TBoolean},
			BarrierPos: 0,
		}},
	},
	{
		Name: "for",

		Signatures: []Signature{
			{
				Args:       []*Type{TInteger, TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       Go(ForCountHandler),
				ReturnsFn:  forIntegerListReturnsFn, BarrierPos: -1,
			},
			{
				Args:       []*Type{TList, TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       Go(ForRangeHandler),
				ReturnsFn:  forListListReturnsFn, BarrierPos: -1,
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
		// trips evaluation_limit). The compile lane refuses the word —
		// the interpreter owns it (lang/spec/frontier/frontier-while.tsv).
		Name: "while",
		Signatures: []Signature{{
			Args:       []*Type{TList, TList},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			Impl:       Go(WhileHandler),
			ReturnsFn:  whileReturnsFn, BarrierPos: -1,
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
		CompileEffect: CompileFallbackBody,
		Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, StripsUnconsumedInput: true, Inputs: func(_ []Value) []Value {
			return []Value{NewCarrier(TError)}
		}},

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
	if !IsConcrete(args[0]) {
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
		// it with.
		if _, isExit := ExitCode(err); isExit {
			return nil, err
		}
		return []Value{NewError(err)}, nil
	}
	return result, nil
}

func DoListReturnsFn(args []Value, r *Registry) []Value {
	body := args[0]
	if IsWord(body) {
		w, _ := AsWord(body)
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
	// static bound, not a proof. Gated to !Compiling (mirroring the
	// producer, which never attaches during a compile pass): the
	// recording pass keeps the dynamic(Any) hatch so a `do` over a
	// computed body carrier keeps REFUSING to lower (whole-program
	// interpreter fallback) — checker precision must not imply compile
	// coverage (lang/go/code_effect_test.go pins the refusal).
	if body.Carrier && !r.Check.Compiling {
		if eff, ok := body.Data.(CodeEffectInfo); ok && eff.Analysed && len(eff.In) == 0 && len(eff.Out) > 0 {
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
	// (design/dynamic-modality-report.10.md, do/eval hatch.) A concrete
	// body is analyzed normally; one that runs to nothing stays strict.
	if !(IsConcrete(body) && body.Parent.ConformsTo(TList)) {
		return []Value{NewDynamicCarrier(TAny)}
	}
	// `do` TRAPS every body error at runtime (DoListHandler surfaces it as
	// an Error value), so a guaranteed-runtime-error mirror firing inside
	// this body is not a program error — raise CaughtBodyDepth so those
	// emitters (CheckAddUniqueDiagnostic, emitIndexOOB) stay silent here.
	r.Check.CaughtBodyDepth++
	// Leak fidelity: do-body defs stay bound in the enclosing scope, exactly
	// as the runtime leaves them (RunCarrierBodyKeepDefs doc).
	stk := RunCarrierBodyKeepDefs(r, body)
	r.Check.CaughtBodyDepth--
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
		if bl, err := AsList(body); err == nil && !bl.IsNil() && bl.Len() > 0 {
			return []Value{NewCarrier(TError)}
		}
		return nil
	}
	// `do` leaves the body's ENTIRE residual stack (DoListHandler returns the
	// full InvokeBody result), so a multi-value literal body — `do [10 20 30]`
	// — nets three values, not one. Returning only the last (stk[len-1])
	// under-reported the arity and made the checked stack contradict the
	// runtime. Mirror the handler: return the full residual. The common
	// single-value `do [expr]` is unaffected (len(stk)==1). The emit closure /
	// island paths require a single output, so a genuinely multi-value body
	// (rare) declines those and rides the whole-program fallback — correct,
	// just not natively compiled.
	//
	// COMPILE PASS: a multi-value body that can RAISE has a runtime-VARIABLE
	// count — `do` CATCHES a body raise into ONE Error value (DoListHandler),
	// so the count is N on no-raise but 1 caught, and a static N-seat
	// underflows on the caught path (`def msg (do [(s decode) "x"] error
	// […])`). This ReturnsFn is the single point that both computes the
	// fallibility and runs immediately before the dispatch records, so it
	// LATCHES the recorder: the record paths (RecordClosureCall / the generic
	// RecordCall / tryRecordDynBody) mark the event's result VARIADIC — the
	// residual absorbs the variable region and a fixed-arity consumer keeps
	// the refusal (plan Phase 5, L-DO). A pure / infallible multi-value body
	// (`do [10 20 30]`, `do [1 add 2 10 mul 4]`) keeps its exact residual and
	// fixed seating. Gated to Compiling so check-mode precision is intact.
	if r.Check.Compiling {
		r.Check.Recorder().SetCatchVariadic(len(stk) > 1 && doBodyMayRaise(body, r))
		// A single-value FALLIBLE body's do-residual is scalar-OR-Error at run time:
		// do CATCHES a body raise into an Error value (DoListHandler's NewError), so a
		// body whose declared happy-path residual is a SCALAR actually yields an Error
		// when it raises. Widen the scalar to the (scalar tor Error) union so a
		// downstream Error accessor (`.code`, `.message`, `convert Map e`) dispatches
		// its Error overload instead of no_signature-ing on a bare scalar — the stats
		// `((do [Stats.mean [] end]).code)` shape, which raised on empty input but typed
		// the residual as the declared Float (#stats code-body refusal). Mirrors the
		// `if [scalar] [raise]` arm-union. Node/Error residuals already match the
		// accessor sigs (excluded); an infallible body keeps its exact scalar.
		// Gated to Compiling ONLY: check-mode precision is intact (`do [1 add 2]` stays
		// Integer even though `add` is now overflow-fallible) — the union is just the
		// compile pass's dispatch shape, and both engines dispatch the caught Error.
		// !IsBareTypeNode guards a ROOT type-literal residual (Any/None/Never,
		// `.Parent == nil`) — `do [add 1 2 drop Any]` leaves the bare `Any` node
		// after the now-overflow-fallible add. The widening is meant for a SCALAR
		// residual; a bare type node is not a scalar value (Any tor Error = Any,
		// and None/Never aren't scalars either), so skipping it is semantically
		// correct. It also keeps the nil-safety EXPLICIT at the call site instead
		// of leaning on ConformsTo's receiver-nil guard (types.go: nil t → false).
		if len(stk) == 1 && doBodyMayRaise(body, r) && !IsBareTypeNode(stk[0]) &&
			!stk[0].Parent.ConformsTo(TNode) && !stk[0].Parent.ConformsTo(TError) {
			return []Value{NewDynamicCarrierValue(NewDisjunct([]Value{stk[0], NewTypeLiteral(TError)}))}
		}
	}
	return stk
}

// doBodyMayRaise reports whether a `do` body can RAISE at run time — the
// discriminator for whether a MULTI-VALUE body is safe to seat at a fixed
// arity (a raise makes `do` net ONE Error instead of the N-value residual). See
// tokensMayRaise for the fallibility rule. A non-list / nil body is conservative
// (fallible).
func doBodyMayRaise(body Value, r *Registry) bool {
	bl, err := AsList(body)
	if err != nil || bl.IsNil() { //covergate:allow do's TList sig + the len(stk)>1 caller guard guarantee a concrete non-empty list body
		return true
	}
	return tokensMayRaise(bl.Slice(), r)
}

// tokensMayRaise recursively scans body tokens for a fallible invocation,
// descending into PAREN-EXPRESSIONS and nested lists so a fallible call buried
// in a group is not missed. A token is fallible if it is a REACH (`m.decode` —
// a dot/method/module dispatch), or a word that resolves to ANY callable
// (wordMayRaise). Only pure literals and plain value reads cannot raise, so a
// WORDLESS multi-value body (`do [10 20 30]`) keeps its native fixed seating.
func tokensMayRaise(toks []Value, r *Registry) bool {
	for i := range toks {
		t := toks[i]
		switch {
		case IsReach(t):
			return true
		case IsParenExpr(t):
			inner, _ := AsParenExpr(t)
			if tokensMayRaise(inner, r) {
				return true
			}
		case IsConcrete(t) && t.Parent != nil && t.Parent.ConformsTo(TList):
			if inner, err := AsList(t); err == nil && !inner.IsNil() && tokensMayRaise(inner.Slice(), r) {
				return true
			}
		case IsWord(t):
			if w, _ := AsWord(t); wordMayRaise(w.Name, r) {
				return true
			}
		}
	}
	return false
}

// wordMayRaise reports whether the word `name` resolves to something that can
// raise: a MODULE-export value, a REGISTERED word, or a callable def binding
// (fnDefMayRaise). A plain value read (`x` → 5) cannot raise; an unbound name
// cannot raise HERE either — it raises undefined_word at dispatch, which the
// check mirrors as its own model-undermining diagnostic, refusing the program
// before any seat is laid.
func wordMayRaise(name string, r *Registry) bool {
	v, ok := r.Defs.Top(name)
	if !ok {
		// A REGISTERED word (a core native) is not a def-stack binding, so
		// the FnDefInfo scan below never saw it. Any registered word counts
		// FALLIBLE: every Go handler can return an error the type system
		// does not see — integer overflow (`add`), a strict-accessor miss
		// (`getr`), a cross-family order (`cmp`) — and the pre-PR-#280
		// under-approximation (only div/mod/raise via their divergence
		// flags, and only when def-bound) statically seated catch regions
		// whose raise path underflowed the seat at run time (probe-pinned:
		// maxint `add` 1 and `m getr "zz"` inside `do [… "a" "b"] error
		// […]`).
		return r.Lookup(name) != nil
	}
	if ModuleNSOf(v) != nil {
		// A bound module NAMESPACE (facet-carrying Map): stepping through
		// it can reach module machinery, so it counts fallible.
		return true
	}
	fd, isFn := v.Data.(FnDefInfo)
	if !isFn {
		return false
	}
	return fnDefMayRaise(&fd)
}

// fnDefMayRaise reports whether invoking fd can raise. EVERY callable is
// fallible: a USER fn body can `raise` (or dispatch a word that does), and a
// NATIVE Go handler can return an error the type system does not see —
// integer overflow (`add`), a strict-accessor miss (`getr`), a cross-family
// order (`cmp`). The pre-PR-#280 flag test (a body, or
// CompileDiverges|CompileValueDiverges) under-approximated exactly there,
// statically seating multi-value catch regions whose raise path delivered
// ONE caught Error where N success values were promoted — an internal
// underflow at run time. Only a degenerate zero-sig value (nothing to
// invoke) stays infallible.
func fnDefMayRaise(fd *FnDefInfo) bool {
	return len(fd.Signatures) > 0
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
	input := make([]Value, len(elems))
	copy(input, elems)
	result, err := sub.Run(input)
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
	sub := New(r)
	input := make([]Value, len(elems))
	copy(input, elems)
	return sub.Run(input)
}

// DoEvalMapValue recursively evaluates list values within a map. Used
// by `do` to walk a map literal and evaluate any embedded code lists.
func DoEvalMapValue(r *Registry, v Value) (Value, error) {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		_lst, _ := AsList(v)
		results, err := doEvalDataList(r, _lst.Slice())
		if err != nil {
			return Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return NewList(results), nil
	}
	if v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v) {
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
func ifMarkMoveTokens(cond Value, thenBranch, elseBranch []Value) ([]Value, bool) {
	if !(cond.Parent.Equal(TList) && cond.Data != nil && !IsTypedList(cond) && !IsTableType(cond)) {
		return nil, false
	}
	_lst, _ := AsList(cond)
	condSlice := _lst.Slice()
	id := NextMarkID()
	tokens := make([]Value, 0, len(condSlice)+2)
	tokens = append(tokens, NewMark(id, condSlice...))
	tokens = append(tokens, condSlice...)
	tokens = append(tokens, NewMoveIf(id, "if", &IfCont{
		Then: thenBranch,
		Else: elseBranch,
	}))
	return tokens, true
}

func if3Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])
	elseBranch := spliceArg(args[2])

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, elseBranch); ok {
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return elseBranch, nil
}

func if2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	cond := args[0]
	thenBranch := spliceArg(args[1])

	if tokens, ok := ifMarkMoveTokens(cond, thenBranch, nil); ok {
		return tokens, nil
	}

	if CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return nil, nil
}

func if3ReturnsFn(args []Value, r *Registry) []Value {
	es := r.Check
	// Plain-check static reduction (the else-less-if soundness fix,
	// forward-barrier.tsv:83): a paren comparison folds to a bare concrete
	// Boolean, so reduce to the taken arm and return a bare-VALUE arm as-is,
	// letting a trailing forward token dispatch against it. Gated to
	// non-recording — the emit path keeps the folded condition EVENT and its
	// existing const/join lowering, so the compiled differential is untouched.
	if !es.Recorder().Active() {
		if out, ok := ReduceStaticIf(r, args[0], args[1], &args[2]); ok {
			return out
		}
	}
	if lit, ok := LiteralCondValue(args[0]); ok {
		branch := "else"
		if !lit {
			branch = "then"
		}
		EmitUnreachableBranch(r, lit, branch)
		var stk []Value
		var defs map[string]Value
		if lit {
			restoreThen := ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[1])
			stk = es.Recorder().ArmTailApply(stk)
			restoreThen()
			InstallJoinedDefs(r, defs, nil)
		} else {
			restoreElse := ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[2])
			stk = es.Recorder().ArmTailApply(stk)
			restoreElse()
			InstallJoinedDefs(r, nil, defs)
		}
		frag := recorderState(es).TakeFragment()
		if len(stk) == 0 { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			es.Recorder().MarkUncompilable("if: branch produces no value (Stage 2 lowers single-result branches)")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		recorderState(es).RecordBranch(BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []Value{out}
	}
	// List-form condition: when emitting, analyse the condition body
	// as its own fragment so the lowering can run it inline before
	// JMP_IF_FALSE (the checker otherwise never evaluates if3
	// conditions — guard extraction is syntactic). Emit-gated so
	// plain checks keep their diagnostics surface unchanged.
	condFrag, condStk := analyseCondFragment(r, args[0])
	// The then arm may be a `[…]` code body (captured as its own fragment) or an
	// already-evaluated VALUE (`if cond 99 88` — a literal, a def-bound value, a
	// paren result), exactly like the else arm below. A non-body then pushes its
	// value directly in the then arm; no capture is armed (there is no body).
	thenIsBody := IsConcrete(args[1]) && args[1].Parent.ConformsTo(TList)
	var thenFrag EmitFragmentRef
	var thenStk []Value
	var thenDefs map[string]Value
	var thenValue *Value
	if thenIsBody {
		restoreThen := ApplyGuardNarrowing(r, args[0])
		es.Recorder().ArmBranchCapture()
		thenStk, thenDefs = RunCarrierBodyWithDefs(r, args[1])
		thenStk = es.Recorder().ArmTailApply(thenStk)
		thenFrag = recorderState(es).TakeFragment()
		restoreThen()
	} else if body, ok := computedArmDoBody(r, args[1]); ok {
		// REFUSAL-CLOSURE.0 §4: a COMPUTED List-conforming arm is the
		// interpreter's spliced code body (spliceArg executes it), and
		// arm-splice ≡ `do <arm>` on every probed axis (multi-values,
		// def leaking, flow escape) — so synthesize the `[do <arm>]` body
		// and take the ordinary body path; the dyn-body machinery compiles
		// the computed `do` (recording pass only; plain checks keep the
		// value-arm surface unchanged).
		restoreThen := ApplyGuardNarrowing(r, args[0])
		es.Recorder().ArmBranchCapture()
		thenStk, thenDefs = RunCarrierBodyWithDefs(r, body)
		thenStk = es.Recorder().ArmTailApply(thenStk)
		thenFrag = recorderState(es).TakeFragment()
		restoreThen()
	} else {
		v := args[1]
		thenValue = &v
		thenStk = []Value{v}
	}
	// The else arm may be a `[…]` code body (captured as its own fragment)
	// or an already-evaluated VALUE (`if cond [then] 42` — a literal, a
	// def-bound value, a paren result). For a non-body else, do NOT arm a
	// capture (there is no body to run); pass the value through so the
	// lowering pushes it directly in the else arm.
	elseIsBody := IsConcrete(args[2]) && args[2].Parent.ConformsTo(TList)
	var elseFrag EmitFragmentRef
	var elseStk []Value
	var elseDefs map[string]Value
	var elseValue *Value
	if elseIsBody {
		restoreElse := ApplyComplementNarrowing(r, args[0])
		es.Recorder().ArmBranchCapture()
		elseStk, elseDefs = RunCarrierBodyWithDefs(r, args[2])
		elseStk = es.Recorder().ArmTailApply(elseStk)
		elseFrag = recorderState(es).TakeFragment()
		restoreElse()
	} else if body, ok := computedArmDoBody(r, args[2]); ok {
		// §4 — the else-arm twin of the then-arm synthesis above.
		restoreElse := ApplyComplementNarrowing(r, args[0])
		es.Recorder().ArmBranchCapture()
		elseStk, elseDefs = RunCarrierBodyWithDefs(r, body)
		elseStk = es.Recorder().ArmTailApply(elseStk)
		elseFrag = recorderState(es).TakeFragment()
		restoreElse()
	} else {
		v := args[2]
		elseValue = &v
		elseStk = []Value{v}
	}
	InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		// BOTH arms produce 0 values (empty `[]`, a 0-value word, or a
		// diverging break/continue/raise): the if is a 0-value STATEMENT, not a
		// value-producing branch. Record it (RecordBranch marks the event
		// zeroOut and the lowering emits no merge slot) rather than refusing —
		// mirroring the 2-arg if2 guard. The registered result is a phantom None
		// the residual reconciliation skips.
		out := NewCarrier(TNone)
		recorderState(es).RecordBranch(BranchRecord{
			Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		// The phantom None is only meaningful while bytecode recording is
		// live (the lowering tracks the zeroOut slot and the top-level
		// residual strips it). On a plain or uncompilable check there is no
		// recorded event to strip, so the if must net 0 like the runtime —
		// otherwise the None leaks onto CheckResult.Stack.
		if !es.Recorder().Active() {
			return nil
		}
		return []Value{out}
	}
	// Plain-check variadic fold (recursion.tsv:53): when an arm's residual
	// carries a variadic-spread (a `[]`-declared recursive fn's seeded
	// self-call), the whole `if` yields a single variadic spread of the
	// per-frame leaked type, absorbing the base arm's emptiness. Gated to
	// non-recording — the compiled path keeps its own STAGE A variadic model.
	if !es.Recorder().Active() {
		if v, ok := FoldVariadicArms(thenStk, elseStk); ok {
			return []Value{v}
		}
	}
	out := joined[len(joined)-1]
	recorderState(es).RecordBranch(BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []Value{out}
}

// computedArmDoBody synthesizes the `[do <arm>]` body for a COMPUTED
// List-conforming branch arm (REFUSAL-CLOSURE.0 §4): the interpreter's
// spliceArg EXECUTES a computed list arm as a code body, and probes prove
// the splice ≡ `do <arm>` (multi-values, def leaking via do's keep-defs,
// break/continue via the FlowCtrl escape) — so the arm compiles through the
// ordinary body path with the dyn-body machinery owning the computed `do`.
// Recording pass only: plain checks keep today's value-arm surface (no
// ratchet churn), and a concrete arm (a real body or a scalar value) never
// reaches here (the body/value paths own those).
func computedArmDoBody(r *Registry, arm Value) (Value, bool) {
	if !r.Check.Recorder().Active() {
		return Value{}, false
	}
	if IsConcrete(arm) || arm.Parent == nil || !arm.Parent.ConformsTo(TList) {
		return Value{}, false
	}
	return NewList([]Value{NewWord("do"), arm}), true
}

// staticCondArm reports the taken arm for a statically-known BARE concrete
// Boolean condition — the shape a paren comparison folds to in plain check
// (`(n eq 0)` → concrete false via CompileScalarFold, carrier.go). It is the
// bare-value analogue of LiteralCondValue (which reads a length-1 LIST body)
// and reuses condSelectsArm's concrete-Boolean probe. ok=false for a
// list-form / dynamic / non-Boolean condition, so the caller falls through to
// the existing const/join analysis.
func staticCondArm(cond Value) (takeThen bool, ok bool) {
	if IsConcrete(cond) && cond.Parent.ConformsTo(TBoolean) {
		if b, err := AsBoolean(cond); err == nil {
			return b, true
		}
	}
	return false, false
}

// ReduceStaticIf reduces an `if` whose condition is a statically-known bare
// Boolean to its TAKEN arm, in PLAIN-CHECK mode only (callers gate on
// !es.Recorder().Active() — the emit lowering has no representation for an
// `if` that evaporates into its arm value; see native_control.go's other
// !Active() reductions). It returns the taken arm's residual directly, with NO
// join against the dead arm — the soundness fix for forward-barrier.tsv:83,
// where the else arm is a bare module-export Function value that must
// forward-collect the token following the `if`. elseArm is nil for the 2-arg
// (if2) form: a false condition then nets no value (matching the runtime).
func ReduceStaticIf(r *Registry, cond, thenArm Value, elseArm *Value) ([]Value, bool) {
	takeThen, ok := staticCondArm(cond)
	if !ok {
		return nil, false
	}
	// Warn on the dead arm, mirroring the const path — but only when a dead
	// arm actually exists (a 2-arg true `if` has no else to call unreachable).
	if !takeThen {
		EmitUnreachableBranch(r, false, "then")
	} else if elseArm != nil {
		EmitUnreachableBranch(r, true, "else")
	}
	if takeThen {
		return reduceStaticArm(r, cond, thenArm, true), true
	}
	if elseArm == nil {
		return nil, true // if2, false: the then is unreachable and nothing runs
	}
	return reduceStaticArm(r, cond, *elseArm, false), true
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
		InstallJoinedDefs(r, defs, nil)
	} else {
		restore := ApplyComplementNarrowing(r, cond)
		stk, defs = RunCarrierBodyWithDefs(r, arm)
		restore()
		InstallJoinedDefs(r, nil, defs)
	}
	return stk
}

// analyseCondFragment captures a list-form `if` condition body (or a
// `case` code-body scrutinee) as an emit fragment (nil when the condition
// is a pre-evaluated value, or when no bytecode recording is active). The
// fragment runs unconditionally exactly once before the branch decision,
// so it rides RunCarrierCondBody — the CondBodyDepth-exempt body run: an
// in-place fn redefinition in a condition is not path-dependent and stays
// compilable, exactly like its paren-`do` condition twin.
func analyseCondFragment(r *Registry, cond Value) (EmitFragmentRef, []Value) {
	es := r.Check.Recorder()
	if !es.Armed() || !IsConcrete(cond) || !cond.Parent.ConformsTo(TList) {
		return nil, nil
	}
	es.ArmBranchCapture()
	stk, _ := RunCarrierCondBody(r, cond)
	return es.TakeFragment(), stk
}

func If2ReturnsFn(args []Value, r *Registry) []Value {
	es := r.Check
	// Plain-check static reduction (else-less if): a folded bare-Boolean
	// condition reduces to the then residual (true) or nothing (false),
	// instead of the phantom Disjunct(then, None) the join path produces.
	// Gated to non-recording, like if3ReturnsFn.
	if !es.Recorder().Active() {
		if out, ok := ReduceStaticIf(r, args[0], args[1], nil); ok {
			return out
		}
	}
	if lit, ok := LiteralCondValue(args[0]); ok && !lit { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		EmitUnreachableBranch(r, false, "then")
	}
	condFrag, condStk := analyseCondFragment(r, args[0])
	restore := ApplyGuardNarrowing(r, args[0])
	es.Recorder().ArmBranchCapture()
	thenStk, thenDefs := RunCarrierBodyWithDefs(r, args[1])
	thenFrag := recorderState(es).TakeFragment()
	restore()
	InstallJoinedDefs(r, thenDefs, nil)
	var out Value
	zeroGuard := len(thenStk) == 0
	if zeroGuard {
		out = NewCarrier(TNone)
	} else {
		out = JoinCarriers(thenStk[len(thenStk)-1], NewCarrier(TNone))
	}
	// 2-arg if: a VARIADIC result (0 or 1 values at run time). An empty
	// then-stack (a 0-value/diverging then) makes it a 0-value statement
	// guard — RecordBranch lowers that with no merge slot.
	recorderState(es).RecordBranch(BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: false,
		Then: thenFrag, ThenStk: thenStk, Out: out, Pos: args[0].Pos(),
	})
	// A 0-value statement guard's phantom None only belongs on the carrier
	// stack while recording is live (mirrors if3ReturnsFn): a plain or
	// uncompilable check has no recorded event to strip it, so it must net
	// 0 like the runtime rather than leak a None onto the residual.
	if zeroGuard && !es.Recorder().Active() {
		return nil
	}
	return []Value{out}
}

// IfListHandler implements the clause-list form `if [c1 b1 c2 b2 … else]`.
// It hands the (raw, NoEval'd) list's elements to ifClause, which produces
// the token stream the engine then runs.
func IfListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("if_error", "if: clause-list argument must be a concrete list, got a type literal", "if")
	}
	_lst, _ := AsList(args[0])
	return ifClause(_lst.Slice()), nil
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
	if isCodeBody(v) {
		sub := New(r)
		lst, _ := AsList(v)
		input := make([]Value, lst.Len())
		copy(input, lst.Slice())
		out, err := sub.Run(input)
		if err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return nil, r.BoruError("case_error",
				"case: value expression produced no value to dispatch on", "case")
		}
		v = out[len(out)-1]
	}
	if !isCodeBody(clauses) {
		return nil, r.BoruError("case_error",
			"case: clause list must be a concrete list of match/block pairs (optional trailing default)", "case")
	}
	lst, _ := AsList(clauses)
	return CaseClauses(r, v, lst.Slice())
}

// IfListReturnsFn type-checks the clause-list form: the result is the
// join of every clause body's last value plus the else clause (or None
// when there is no else, since an unmatched `if` produces nothing).
// Condition bodies are still run for their diagnostics but don't
// contribute to the return type. Unlike if3/if2 this does no per-clause
// guard narrowing — multi-clause narrowing isn't modelled.
func IfListReturnsFn(args []Value, r *Registry) []Value {
	if !IsConcrete(args[0]) || !args[0].Parent.Equal(TList) {
		return []Value{NewCarrier(TAny)}
	}
	_lst, _ := AsList(args[0])
	elems := _lst.Slice()

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

// ---- for / break / continue handlers ----

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
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("for_error", "for: range must be a concrete list, got type literal", "for")
	}
	_lst, _ := AsList(args[0])
	rangeSpec := _lst.Slice()
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
// design/checker-accuracy-review.10.md A4): body rebindings like
// `def acc (acc add 0.5)` join back into the enclosing binding and
// the body re-runs until the bindings stabilise, so post-loop reads
// see Integer|Float, not the pre-loop Integer. Returns a typed list
// whose element type mirrors the final round's residual top.
//
// The List carrier is a STATIC APPROXIMATION of the result type, not a
// claim that the loop leaves one List value: at run time BOTH engines
// splice the per-iteration values onto the stack as separate entries
// (`for 3 [i]` leaves `0 1 2`, not `[0,1,2]`). That is why the bytecode
// lowerer treats a loop result as VARIADIC — consumable only by the
// program residual, never fed to a downstream operand (eng/go/lower.go
// lowerLoop, RecordLoop's `out` marked variadic).
//
// countArg >= 0 names the count/range operand and arms bytecode loop
// recording: the final round's events are captured as a fragment and
// RecordLoop lowers the loop (FOR_SETUP/FOR_NEXT with the iterator
// as a VM local). The count form lowers as the range [0, n, 1]; the
// range form decomposes a LITERAL integer range via ParseRange
// (computed ranges record nothing and the generic path refuses).
func forCarrierAnalyse(r *Registry, iterName string, iterType *Type, args []Value, countArg int) []Value {
	body := args[len(args)-1]
	iter := NewCarrier(iterType)
	es := r.Check.Recorder()

	// Statically-zero loop pruning: a `for` whose count operand is a CONCRETE
	// non-positive Integer never enters its body — at run time both engines
	// iterate zero times and push zero values (`for 0 [body]` leaves the stack
	// untouched). Its body is unreachable, so analysing it is both wasted work
	// and a source of false refusals: a body that only type-checks (or only
	// compiles) for a live iteration — e.g. module-test:38's `for (subs size)
	// [subspec run-spec]` over `subs: []`, whose recursive `run-spec` over a
	// carrier `subspec` cannot dispatch — would otherwise poison the program for
	// a branch that never runs. Prune it: record nothing, contribute no values,
	// and skip body analysis entirely. Faithful because the interpreter's
	// `for 0` runs the body zero times (no side effects, no values), so the
	// compiled form (emitting no loop at all) is observably identical.
	//
	// The fold only fires for a STATIC count; a carrier/computed count keeps the
	// full analyse-and-record path below. countArg-as-list (a literal range) is
	// pruned too when it decodes to an empty span.
	if countArg >= 0 {
		if cv := args[countArg]; IsConcrete(cv) {
			zero := false
			switch {
			case cv.Parent.ConformsTo(TInteger):
				if n, err := AsInteger(cv); err == nil && n <= 0 {
					zero = true
				}
			case cv.Parent.ConformsTo(TList):
				if lst, err := AsList(cv); err == nil && !lst.IsNil() {
					if st, en, sp, perr := ParseRange(lst.Slice()); perr == nil && LoopIterations(st, en, sp) == 0 {
						zero = true
					}
				}
			}
			if zero {
				return []Value{}
			}
		}
	}

	// Decompose the loop bounds for recording. `lowerable` means RecordLoop can
	// emit FOR_SETUP (const start/step, a resolvable end). `staticBounds` is the
	// stronger property that ALL THREE bounds are concrete integers, so the exact
	// iteration count is known — required for the spread-residual model below,
	// which AsInt64Or-defaults an unknown bound to 0 and would otherwise starve a
	// computed loop's residual.
	var startV, endV, stepV Value
	lowerable, staticBounds := false, false
	if countArg >= 0 {
		cv := args[countArg]
		switch {
		case cv.Parent.ConformsTo(TInteger):
			startV, endV, stepV = NewInteger(0), cv, NewInteger(1)
			lowerable, staticBounds = true, IsConcrete(cv)
		case IsConcrete(cv) && cv.Parent.ConformsTo(TList):
			if lst, err := AsList(cv); err == nil && !lst.IsNil() {
				elems := lst.Slice()
				if st, en, sp, perr := ParseRange(elems); perr == nil {
					startV, endV, stepV = NewInteger(st), NewInteger(en), NewInteger(sp)
					lowerable, staticBounds = true, true
				} else if s, e, sp, cok := computedRangeBounds(elems); cok {
					// Const start/step, a COMPUTED end (`for [0 total 65536]` —
					// mini-s3's s3-send-resp): lowerable as FOR_SETUP with the end
					// resolved to its runtime operand, but NOT statically counted, so
					// the exact-count spread below stays off.
					startV, endV, stepV = s, e, sp
					lowerable = true
				}
			}
		}
	}
	if lowerable {
		es.ArmLoopCapture()
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
		if IsDisjunct(top) {
			out = NewCarrierTypedListValue(top)
		} else {
			out = NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		// The STATIC region size (trips x per-iteration net) arms the S5
		// first-value def bind (SplitLoopRegionBind): known only when all
		// three bounds are concrete and the loop runs at least once.
		regionN := 0
		if staticBounds && len(stk) > 0 {
			if n := LoopIterations(AsInt64Or(startV, 0), AsInt64Or(endV, 0), AsInt64Or(stepV, 1)); n >= 1 {
				regionN = int(n) * len(stk)
			}
		}
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, out, regionN, args[countArg].Pos())
	}
	// A body that nets ZERO values per iteration (every pass drops / is pure
	// side effect) leaves the stack untouched at run time — BOTH engines net
	// zero values from the loop, whatever the (possibly carrier) count. On a
	// PLAIN check (no bytecode recording) model that exactly: contribute no
	// residual. Returning the List-carrier approximation here instead over-
	// counts a trailing statement's arity — the false "expected 1 return
	// value(s), got 2" a side-effect loop with a carrier count produced
	// (mini-s3's s3-send-resp / s3-read-body-into / s3-drain-body). When
	// RECORDING a LOWERABLE loop, `out` must still be returned so the loop EVENT
	// stays linked to the residual (dropping it re-emits `for` as an interpreted
	// CALL_NATIVE — a double-lowered loop); RecordLoop already marked that event
	// zeroOut for an empty bodyStk, so the fn/closure return reconciliation strips
	// it there. When the loop is NOT lowerable (a computed start/step range the
	// compiler refuses), NO loop event exists to link and the program falls back,
	// so return the empty residual in the recording pass too — otherwise the check
	// and compile passes disagree (plain check nets 0, compile keeps `out` and
	// reports a phantom "got 2"), violating the same-diagnostics contract.
	if len(stk) == 0 && (!es.Active() || !lowerable) {
		return []Value{}
	}
	// Plain check (no bytecode recording): a STATICALLY-COUNTED loop leaves the
	// SPREAD of its per-iteration residual on the stack, not one List value
	// (`for 3 ['x']` leaves `'x' 'x' 'x'`; `for [1 4] [7 8]` leaves six ints).
	// Model that exactly so the residual's arity and element types match the
	// runtime — the List above is a variadic APPROXIMATION the bytecode lowerer
	// requires, kept for the recording path (which never reaches here, es!=nil).
	// The per-iteration residual is the loop's fixed-point join, so repeating it
	// count times is a sound over-approximation: a break/continue loop runs
	// FEWER iterations, and a shorter runtime stack is still covered by the
	// longer checked one. Bounded to keep the residual small; past the cap the
	// List approximation stands. CONCRETE count only: a carrier count decomposes
	// to a carrier endV, which AsInt64Or would default to 0 — spreading a
	// runtime-unknown iteration count to an EMPTY residual and starving every
	// downstream consumer (`size (for n [n])` failed no_signature on nothing).
	if !es.Active() && staticBounds && len(stk) > 0 {
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

// computedRangeBounds decomposes a `for` range list whose START and STEP are
// concrete integers but whose END may be computed (a carrier / event value —
// `for [0 total 65536]`, mini-s3's s3-send-resp). It mirrors ParseRange's arity
// defaults (1 elem → [0, end, 1]; 2 → [start, end, 1]; 3 → [start, end, step])
// but requires only start/step to be statically known: RecordLoop const-bakes
// those and resolves the end to its runtime operand. The end value is returned
// AS-IS (carrying its ID) so resolveOperand finds its producing event/local.
// ok=false when start or step is not a concrete integer (RecordLoop refuses a
// computed start/step) or the arity is not 1–3.
func computedRangeBounds(elems []Value) (startV, endV, stepV Value, ok bool) {
	// Every bound may be computed (carrier / event values returned AS-IS so
	// resolveOperand finds their homes): RecordLoop admits const AND local
	// operands for start/step and keeps refusing event-produced ones — the
	// VM's opForSetup pops the full triple generically with the
	// interpreter's own runtime Integer/zero-step taxonomy either way.
	switch len(elems) {
	case 1:
		return NewInteger(0), elems[0], NewInteger(1), true
	case 2:
		return elems[0], elems[1], NewInteger(1), true
	case 3:
		return elems[0], elems[1], elems[2], true
	}
	return Value{}, Value{}, Value{}, false
}

// AsInt64Or returns v's integer value, or def when v is not a concrete integer.
func AsInt64Or(v Value, def int64) int64 {
	if IsConcrete(v) && v.Parent.ConformsTo(TInteger) {
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
// same forward collection the interpreter takes, so refuseForwardStackDrift
// has nothing to refuse. Anything inconclusive — a non-token handler, a
// multi-value or empty handler residual, a nil parent — keeps the historical
// dynamic(Any), so genuinely dynamic boundaries keep refusing.
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
// would have tripped the refusal gate at 0).
func ErrorReturnsFn(args []Value, r *Registry) []Value {
	wide := []Value{NewDynamicCarrier(TAny)}
	if !IsConcrete(args[0]) || args[1].Parent == nil {
		return wide
	}
	body, err := AsList(args[0])
	if err != nil || body.IsNil() {
		return wide
	}
	seeded := append([]Value{NewCarrier(TError)}, body.Slice()...)
	stk := RunCarrierBody(r, NewList(seeded))
	// The handler nets ONE value (BodyOut 1): the seeded error still at the
	// residual bottom is the strip-unconsumed shape (`["fallback"]`), so drop
	// it before the count check — mirroring ErrorHandler's identity probe.
	if len(stk) >= 2 && stk[0].ID == seeded[0].ID {
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
		// Refusing is the sanctioned response: the island model cannot
		// express this program, and under the refusal architecture the
		// interpreter fallback is always sound. It is also what the adjacent
		// no-error path already does for its own unrepresentable shape (a
		// baked arg beyond BarrierPos — carrier.go's island decline).
		//
		// ZERO specifically, not "!= 1". A residual of two or more is NOT
		// broken: its bottom is the unconsumed seeded error, and the paired
		// CLOSURE path nets one from it with a runtime strip
		// (TestErrorStripInputClosure pins `error [dup drop "k"]`, which
		// measures 2 here because the compile-time strip's identity probe
		// does not match after a dup/drop). Refusing those regressed a shape
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
		// graduation). A DYNAMIC Error bound keeps the refusal: there the
		// runtime may not raise, the pass-through nets one where the caught
		// path nets zero, and a fixed seat cannot carry both.
		if !args[1].Dynamic && args[1].Parent != nil && args[1].Parent.ConformsTo(TError) {
			return nil
		}
		// The arity is variable, not unknown: ZERO on the caught path, ONE
		// on the pass-through. A fixed seat cannot carry both — which is
		// what the refusal here said — but a runtime-variadic REGION can,
		// and it is the same device await's winner-takes-all residual and a
		// value-producing loop already ride (the forty-eighth increment).
		// One recorded slot stands for the whole run, callVariadicRegion
		// marks the dispatch, and the residual absorbs whatever the run
		// delivers. A consumer that needs a fixed count still refuses, at
		// its own gate, over a region the recorder can name.
		return []Value{NewVariadicCarrier(NewTypeLiteral(TAny))}
	}
	if len(stk) != 1 || stk[0].Parent == nil {
		return wide
	}
	// A PROVEN-Error do-result (a strict Error carrier — the body always
	// raises, DoListReturnsFn's raising-residual arm) makes the pass-through
	// arm statically dead: the result is the handler's alone, no join. A
	// dynamic(Error) bound keeps the join — the bound is best-effort, not
	// proof, so the pass-through may still run.
	if !args[1].Dynamic && args[1].Parent.ConformsTo(TError) {
		return []Value{NewDynamicCarrier(stk[0].Parent)}
	}
	return []Value{NewDynamicCarrier(CommonAncestorType(args[1].Parent, stk[0].Parent))}
}

func ErrorHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
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
	if len(out) >= 2 && out[0].Data == args[1].Data {
		out = out[1:]
	}
	return out, nil
}

// recorderState returns the check state's recorder through core's INTERFACE.
// It used to downcast to the concrete *EmitState, because RecordBranch /
// RecordLoop / TakeFragment were not on EmitRecorder — that downcast was the
// only reason this package depended on the compiler at all. The seam now
// covers branching, so a word library implements control flow naming core
// alone. Under a plain-check pass the recorder is core's named inactive
// default and every call declines.
func recorderState(c *CheckState) EmitRecorder {
	return c.Recorder()
}
