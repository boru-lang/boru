package basic

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		// Append element-wise: Slice() would copy the backing once into a
		// throwaway slice and append would copy it AGAIN into result —
		// this runs per selected if/for branch, so the intermediate copy
		// was a per-branch allocation.
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		for i := 0; i < elems.Len(); i++ {
			result = append(result, elems.Get(i))
		}
		result = append(result, NewCloseParen())
		return result
	}
	return []Value{v}
}

// isCodeBody reports whether v is a plain (non-typed, non-table) concrete
// list — i.e. something to be evaluated as a code body rather than used
// as a literal value.
func isCodeBody(v Value) bool {
	return v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v)
}

// ifClause turns the element slice of a clause-list `[c1 b1 c2 b2 … else]`
// into the token stream the engine should run for it.
//
// Walking two at a time: elems[2k] is a condition, elems[2k+1] is that
// clause's body; a trailing odd element is the else clause. A condition
// that is a code-body list is evaluated lazily via the mark / move-if
// machinery (so later clauses don't run once one matches and so side
// effects in the condition only happen when reached); a scalar condition
// is decided immediately with CoerceBoolean. A body is spliced via
// spliceArg (code-body list → `( … )`, scalar → as-is).
//
// Empty slice → no tokens. One element → just that element's tokens (a
// lone else). The else branch of clause k is, recursively, ifClause of
// elems[2k+2:].
func ifClause(elems []Value, pos SrcPos) []Value {
	switch len(elems) {
	case 0:
		return nil
	case 1:
		return spliceArg(elems[0])
	}

	cond := elems[0]
	thenBranch := spliceArg(elems[1])
	elseBranch := ifClause(elems[2:], pos)

	if isCodeBody(cond) {
		_lst, _ := AsList(cond)
		condSlice := _lst.Slice()
		id := NextMarkID()
		tokens := make([]Value, 0, len(condSlice)+2)
		tokens = append(tokens, NewMark(id, condSlice...))
		tokens = append(tokens, condSlice...)
		tokens = append(tokens, WithPosAt(NewMoveIf(id, "if", &IfCont{Then: thenBranch, Else: elseBranch}), pos))
		return tokens
	}

	if CoerceBoolean(cond) {
		return thenBranch
	}
	return elseBranch
}

// IsCaseOpenCallHead reports whether a match-position clause element is a
// bare word bound to a FUNCTION VALUE (a def'd fn or a parked Function).
// Such an element can never be a genuine match — a function value unifies
// with nothing (NUR031) — so it must be the head of an OPEN-CALL default
// arm (`case k [ "q" [quit] vt-screen-key state ev ]`, NUR048/G9): the
// pair walk that would otherwise tear the call into a bogus (match, block)
// pair stops here and the rest of the list is the default body. Bare words
// NOT bound to a function keep their atom-match reading (`case c/q [ red
// "R" … ]`), so only provably-uncallable-as-match heads change meaning.
func IsCaseOpenCallHead(r *Registry, m Value) bool {
	if !IsWord(m) {
		return false
	}
	w, _ := AsWord(m)
	bound, ok := r.ResolveTypedName(w.Name)
	if !ok {
		return false
	}
	return bound.Parent.Equal(TFunction)
}

// caseDefaultStart returns the index where the clause list's trailing
// DEFAULT arm begins, and whether one exists. Pairs walk two at a time;
// the default is normally the single trailing odd element, but an
// open-call head in match position (IsCaseOpenCallHead) starts the
// default early and it spans every remaining element. The pairs region
// elems[:start] always has even length.
func caseDefaultStart(r *Registry, elems []Value) (int, bool) {
	i := 0
	for ; i+1 < len(elems); i += 2 {
		if IsCaseOpenCallHead(r, elems[i]) {
			return i, true
		}
	}
	if i < len(elems) {
		return i, true
	}
	return len(elems), false
}

// caseNormalizeClauses collapses an OPEN-CALL default region — trailing
// tokens headed by a function word (caseDefaultStart stopping early), or
// a single trailing fn-headed word — into ONE synthetic code-body block,
// so every downstream walk (the runtime clause walk, the branch join,
// the compile desugar, the exhaustiveness passes) sees the standard
// pairs-plus-single-default shape and treats the arm exactly like a
// matched code-body arm (NUR048/G9). A clause list already in standard
// shape is returned unchanged.
func caseNormalizeClauses(r *Registry, elems []Value) []Value {
	start, hasDefault := caseDefaultStart(r, elems)
	if !hasDefault {
		return elems
	}
	rest := elems[start:]
	if len(rest) == 1 && !IsCaseOpenCallHead(r, rest[0]) {
		return elems
	}
	out := make([]Value, 0, start+1)
	out = append(out, elems[:start]...)
	out = append(out, NewList(append([]Value(nil), rest...)))
	return out
}

// CaseClauses runs the `case` word's clause walk (see CaseHandler):
// v is the captured value; elems are the raw clause-list elements —
// match/block pairs with an optional trailing default (a single
// element; an open-call tail is first collapsed into a synthetic block
// by caseNormalizeClauses). Returns the matched block's result stack.
func CaseClauses(r *Registry, v Value, elems []Value) ([]Value, error) {
	elems = caseNormalizeClauses(r, elems)
	i := 0
	for ; i+1 < len(elems); i += 2 {
		match := elems[i]
		matched := false
		if isCodeBody(match) {
			// Predicate match: the match list executes as if the value
			// were already on the stack ([gt 3] runs as `v gt 3`), and
			// the result coerces to boolean.
			out, err := runCaseBody(r, v, match)
			if err != nil {
				return nil, err
			}
			if len(out) > 0 {
				matched = CoerceBoolean(out[len(out)-1])
			}
		} else {
			// Value / type match: the clause unifies with the value —
			// equal scalars and atoms match, a type literal (builtin or
			// def'd) matches its members, a map pattern matches
			// structurally. A bare word resolves through the def table
			// first so user types and def'd values work as matches.
			m := match
			if IsWord(m) {
				w, _ := AsWord(m)
				if bound, ok := r.ResolveTypedName(w.Name); ok {
					m = bound
				} else {
					m = ResolveWordValue(m)
				}
			}
			// An angle/type-bound match — `case b [Box<Integer> […]]`
			// — arrives as a sugar marker; lower it so the paren arm
			// below evaluates the spelt-out `(Box of [Integer])`.
			if sinfo, sok := AsSugar(m); sok {
				if exp, serr := SugarExpansion(r, sinfo, m, false); serr == nil && len(exp) == 1 {
					m = exp[0]
				}
			}
			// A parenthesised match — `case b [(Box of [Integer]) […]]`
			// — evaluates inline, the same contract paren annotations
			// follow in typed defs. Generic instantiations are the main
			// client; any expression producing one value works. An
			// evaluation error or multi-value result keeps the raw
			// ParenExpr (which then simply fails to unify).
			if IsParenExpr(m) {
				if toks, perr := AsParenExpr(m); perr == nil {
					input := make([]Value, 0, len(toks)+2)
					input = append(input, NewOpenParen())
					input = append(input, toks...)
					input = append(input, NewCloseParen())
					sub := New(r)
					if out, rerr := sub.Run(input); rerr == nil && len(out) == 1 {
						m = out[0]
					}
				}
			}
			_, ok := UnifyR(m, v, r)
			matched = ok
		}
		if matched {
			return runCaseBody(r, v, elems[i+1])
		}
	}
	if i < len(elems) {
		// Trailing odd element: the default clause (an open-call tail
		// arrives here as one synthetic code-body block and so runs in
		// an isolated sub-engine with the case value pushed first —
		// exactly like a matched arm, NUR048/G9).
		return runCaseBody(r, v, elems[i])
	}
	return nil, nil
}

// CaseReturnsFn type-checks a `case` and, when bytecode emission is active,
// desugars it to a nested-`if` chain so it compiles natively instead of
// declining as a code-body word (design doc "case clause compilation"). Each
// clause becomes `if (v match __casematch) [block] [rest]`; a code-body
// predicate match `[pred]` becomes the guard `(v pred…)`; a block runs with
// v pushed first (mirroring runCaseBody).
//
// It compiles ONLY shapes it can statically prove the single-result branch
// lowering will take — a re-pushable case value (`OperandRepushable`: tested
// against every clause guard, a computed event could not be re-pushed inside
// the else fragments). A trailing default makes the innermost else a definite
// value; without one the chain lowers as a 0-or-1 variadic (see the comment
// on the desugar below) — only the code-body-scrutinee sub-path additionally
// demands the 3-element default shape. Every other shape returns the prior
// conservative dynamic-Any WITHOUT marking the program uncompilable, so the
// island keeps owning it and compile failures never rise.
// Faithfulness rides the differential gate (runtime stays
// CaseHandler/CaseClauses; __casematch reuses its UnifyR).
func CaseReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	dynAny := []Value{NewDynamicCarrier(TAny)}
	v, clauses := args[0], args[1]
	swapped := isCodeBody(v) && !isCodeBody(clauses)
	if swapped {
		v, clauses = clauses, v
	}
	if isCodeBody(v) {
		// A code-body scrutinee is DYNAMIC — its result type is not
		// statically modelled — so the clause list must carry its own
		// catch-all (a trailing default or an Any clause). Emitted on
		// both the plain-check and compile paths (case_exhaustive.go).
		checkCaseCodeBodyScrutinee(r, v, clauses)
		// A code-body scrutinee (`case [body] [clauses]`) runs the body and
		// dispatches on its last result. In compile mode:
		//   - 0-net body → the run-time case_error (CaseHandler): record a TERMINAL
		//     OpTrap raising the byte-identical error. The residual count must come
		//     from the RECORDING run (condResidual) — a type-only RunCarrierBody
		//     gives an empty-body 0-return fn (`[f 1]`) a bogus 1-value count that
		//     would miss the trap — and only from a body that RAN: a carrier
		//     scrutinee is not a 0-net one.
		//   - 1+-net body, a SINGLE clause + default, both blocks VALUES
		//     (`case [1 add 1] [2 "two" "other"]`) → desugar to a nested `if` whose
		//     guard runs the body ONCE via `do`: `if (do [body] m __casematch)
		//     [then] [default]`. Because there is exactly ONE guard and the blocks
		//     are values, `do [body]` is evaluated exactly once — so this needs no
		//     scrutinee seating and is sound for ANY body (no recompute). Any other
		//     shape (multi-clause, code-body blocks, no default) keeps the island.
		// Plain check (no emit) keeps the prior dynAny; a compile pass's
		// SUSPENDED run keeps the scrutinee's bindings (keepScrutineeBindings).
		if es := r.Check.Recorder(); es.Active() {
			return caseCodeBodyRecord(r, es, v, clauses, dynAny)
		}
		keepScrutineeBindings(r, v)
		return dynAny
	}
	if !isCodeBody(clauses) {
		// The clause argument is not a list: CaseHandler raises case_error. The
		// checker is lenient (returns a carrier), but compiled mode raises the
		// byte-identical error via a TERMINAL OpTrap instead of islanding — the
		// trap keeps the events before it, drops the case dispatch (which would
		// otherwise island), and aborts exactly where the interpreter does. A
		// nested case (RecordTrap declines, frames/units != 1) keeps the island.
		//
		// Only a CONCRETE non-list earns the trap. A clause operand the pass
		// holds as a carrier or a dynamic value — a fn's returned list,
		// `case 1 (mk 0)` over `def mk fn [[n:Integer] [List] [quote [1
		// 'one' 'many']]]` — is a list or not at RUN time, where the
		// interpreter reads the concrete value and answers `'one'`; the
		// trap raised the error the interpreter never raises (NUR154). The
		// dispatch's own gate declines the computed clause list instead
		// (a NoEvalArgs body that is not inert data), so the program falls
		// back and answers as the interpreter does. A KNOWN non-list — a
		// scalar, a bare type node (`case 1 Integer`) — is the trap.
		if !clauses.Dynamic && !clauses.Carrier {
			r.Check.Recorder().RecordTrap("case_error",
				"case: clause list must be a concrete list of match/block pairs (optional trailing default)",
				"case", "", args[0].Pos())
		}
		return dynAny
	}
	lst, _ := AsList(clauses)
	if lst.IsNil() {
		return dynAny
	}
	// An open-call default tail collapses into one synthetic code-body
	// block up front (NUR048/G9), so the coverage pass, the compile
	// desugar, and the branch join all see the standard shape and lower
	// the arm exactly like a matched block.
	elems := caseNormalizeClauses(r, lst.Slice())
	// Static coverage pass (case_exhaustive.go) — BEFORE the compile-desugar /
	// plain-check paths diverge, so both report identical findings: a
	// default-less case whose clauses don't cover the scrutinee's static type
	// is a check error, plus the redundant-default / unreachable-clause
	// advisories.
	checkCaseExhaustiveness(r, v, clauses, elems)
	// At least one clause pair. A trailing default (odd length) makes the
	// innermost else a definite value; WITHOUT one (even length) the innermost
	// `if guard [block]` has no else and the chain is variadic (0-or-1), which
	// matches case's no-match semantics (it produces NO value) — the nested-
	// variadic branch lowering propagates that 0-or-1 to the residual.
	if len(elems) < 2 {
		return dynAny
	}
	// COMPILE desugar to a nested-`if` chain — ONLY when emission is active and
	// the case value can be seated inside every clause guard / block fragment it
	// is re-tested in. CanSeatAcrossFragment admits a const / local / type node
	// directly, and a COMPUTED top-level scrutinee (`case (1 add 1) […]`) that
	// planValueDefLocals then promotes to a frame local once the fragment reads
	// are recorded. if3ReturnsFn returns the branch-join type AND records the
	// lowering.
	es := r.Check.Recorder()
	seatable := es.CanSeatAcrossFragment(v)
	if seatable && caseReStepDeclined(r, es, elems) {
		return dynAny
	}
	if seatable && mayRunAsCode(v) {
		v, seatable = recordCaseSubject(r, v, swapped, r.Check.CurCallPos)
	}
	if seatable {
		cond := NewList(caseGuardTokens(v, elems[0]))
		then := NewList(caseBlockTokens(v, elems[1]))
		rest := buildCaseChain(v, elems, 2)
		// The desugared nested-`if` chain lowers every clause guard and block
		// INLINE into the enclosing unit, where the runtime CaseHandler runs
		// each matched block in a sub-engine (runCaseBody → RunResolved) with
		// its own context layer. Bracket the whole desugar as an inline
		// context-boundary region so an ambient-context write inside a clause
		// declines (NUR054) instead of compiling one scope too shallow.
		es.PushInlineCtxBoundary()
		mark := len(r.Check.Diagnostics)
		out := if3ReturnsFn([]Value{cond, then, NewList(rest)}, r)
		dropSynthesizedDeadArmWarnings(r, mark)
		es.PopInlineCtxBoundary()
		return out
	}
	// Otherwise the island owns COMPILATION, but the
	// result TYPE is still computable: the join of every clause block's
	// residual type plus the trailing default. Decoupling the type from the
	// compile-eligibility lets a plain `boru check` (which has no emit state)
	// and a non-re-pushable case narrow instead of poisoning the result with
	// Any.
	return CaseBranchJoin(r, v, elems)
}

// caseReStepDeclined declines the compile desugar of a clause list holding a
// block that is a bare FUNCTION word (caseReStepsBlock), and reports that it
// did. Such a block is not an arm: the handler hands the word back and the
// tape re-steps it AT THE CASE, over the values beneath it and the tokens
// after it (`1 2 case 7 [[lt 10] add]` answers 3), where the chain would run
// it sealed in its own arm. Only a bare call (caseCallBare) has nothing
// around it for the word to reach, so there the chain is exact; anywhere
// else the block is an arm the chain cannot capture, and it declines as the
// clause-list `if`'s re-stepped arm does (ifClauseDecline, NUR332). A
// suspended pass decides nothing: its enclosing dispatch owns the compile.
func caseReStepDeclined(r *Registry, es EmitRecorder, elems []Value) bool {
	if !es.Active() || !caseReStepsBlock(r, elems) || caseCallBare(r, es) {
		return false
	}
	taken := true
	recorderState(r.Check).RecordBranch(BranchRecord{
		ConstCond: &taken, HasElse: true, Pos: r.Check.CurCallPos,
		Uncaptured: "case: a clause block that is a bare function word, re-stepped over the values around the case",
	})
	return true
}

// caseReStepsBlock reports whether a (normalized) clause list holds a block
// — the element after each match, or the trailing default — that the tape
// re-steps as a call (caseBlockReSteps).
func caseReStepsBlock(r *Registry, elems []Value) bool {
	for i := 1; i < len(elems); i += 2 {
		if caseBlockReSteps(r, elems[i]) {
			return true
		}
	}
	return len(elems)%2 == 1 && caseBlockReSteps(r, elems[len(elems)-1])
}

// caseBlockReSteps reports whether a clause block is a bare word the tape
// DISPATCHES when CaseClauses hands it back (runCaseBody returns a non-body
// block as itself) over what surrounds the case: a word bound to a
// function or a registered word that takes an operand or reads the whole
// stack (callReachesContext), or a word whose binding the pass holds only
// abstractly (a computed def may be such a function at run time). A word
// naming a value or a type steps to that value, and a function of no
// operands runs alike wherever it is stepped, as the chain's arm runs it;
// a `/v` reference parks the value.
func caseBlockReSteps(r *Registry, b Value) bool {
	if !IsWord(b) {
		return false
	}
	w, _ := AsWord(b)
	if w.ForceVal {
		return false
	}
	if top, ok := r.Defs.Top(w.Name); ok {
		if fd, isFn := top.Data.(FnDefInfo); isFn {
			return callReachesContext(&fd)
		}
		return top.Carrier || top.Dynamic
	}
	fn := r.Lookup(w.Name)
	return fn != nil && callReachesContext(fn)
}

// callReachesContext reports whether some signature of fn takes an operand
// or reads the whole stack — a dispatch whose outcome depends on the
// values around the word.
func callReachesContext(fn *FnDefInfo) bool {
	for i := range fn.Signatures {
		if fn.Signatures[i].TotalArgs() > 0 || fn.Signatures[i].FullStack() {
			return true
		}
	}
	return false
}

// caseCallBare reports whether the `case` whose ReturnsFn is running sits
// in a BARE context (CheckState.BareCallPos) on the program's root stream.
func caseCallBare(r *Registry, es EmitRecorder) bool {
	pos := r.Check.CurCallPos
	return es.TopFrameOnly() && pos.Row > 0 && r.Check.BareCallPos == pos
}

// dropSynthesizedDeadArmWarnings removes, from the diagnostics added since
// mark, the unreachable_branch warnings of the case desugar's SYNTHESIZED
// `if` tokens (buildCaseChain). A clause guard folds to a concrete Boolean
// (`case 7 [[lt 3] … [lt 10] …]`), so the chain's nested `if` sees a static
// condition and warns (warnStaticIfDeadArm) — about code the user never
// wrote, at no source position (a synthesized token has none), and only on
// the recording pass: a plain check types the `case` by CaseBranchJoin and
// never builds the chain. The `case`'s own coverage pass
// (checkCaseExhaustiveness) is what speaks for its clauses. A user `if`
// inside a clause block carries its own position and keeps its warning.
func dropSynthesizedDeadArmWarnings(r *Registry, mark int) {
	mark = min(mark, len(r.Check.Diagnostics))
	kept := r.Check.Diagnostics[:mark]
	for _, d := range r.Check.Diagnostics[mark:] {
		if d.Code == "unreachable_branch" && d.Row == 0 && d.Col == 0 {
			continue
		}
		kept = append(kept, d)
	}
	r.Check.Diagnostics = kept
}

// mayRunAsCode reports whether a value the pass holds abstractly may be a
// list at run time — a carrier or dynamic value whose type admits one. The
// interpreter RUNS a list in a code-body slot, whatever produced it: case
// dispatches on a list scrutinee's last result (`case (mk) […]` over mk's
// `[1 2]` dispatches on 2, NUR291), and if runs a list condition inline and
// splices a list arm (NUR292), where a compiled chain or branch over the
// value would hold the list itself.
func mayRunAsCode(v Value) bool {
	if !v.Carrier && !v.Dynamic {
		return false
	}
	p := v.Parent
	return p != nil && (TList.ConformsTo(p) || p.ConformsTo(TList))
}

// codeGuards reports which of a branch's value condition and value arms
// the compiled `if` guards at run time (NUR292): a value the pass holds
// abstractly whose type admits a list — which the interpreter runs as code
// there, a list condition inline and a list arm spliced in parens — takes
// __codeguard, which passes any other value and defers on a list. The guard
// is the LOWERING's (BranchRecord.Guard): it runs on the value as the
// branch consumes it, on the taken path for an arm, and adds nothing to the
// pass's model, so a value that may be a fn keeps the landing that applies
// it at the merge (NUR280). A List-typed arm keeps its own path
// (computedArmDoBody's `[__arm <arm>]`), since its value is always a list.
// Recording pass only.
func codeGuards(r *Registry, cond Value, thenValue, elseValue *Value) (condGuard, thenGuard, elseGuard bool) {
	if !r.Check.Recorder().Active() {
		return false, false, false
	}
	armGuard := func(v *Value) bool {
		return v != nil && mayRunAsCode(*v) && !v.Parent.ConformsTo(TList)
	}
	return mayRunAsCode(cond), armGuard(thenValue), armGuard(elseValue)
}

// codeGuardRecord fills a branch record's guard fields (codeGuards).
func codeGuardRecord(r *Registry, rec BranchRecord) BranchRecord {
	c, t, e := codeGuards(r, rec.Cond, rec.ThenValue, rec.ElsValue)
	if c || t || e {
		rec.Guard, rec.CondGuard, rec.ThenGuard, rec.ElseGuard = &codeGuardSignature, c, t, e
		rec.CondCheck, rec.CondCheckPos = &condGuardSignature, r.Check.CurCallPos
	}
	return rec
}

// condGuardSignature is __condguard's one signature, the one the lowering's
// condition guard runs (BranchRecord.CondCheck).
var condGuardSignature = Signature{
	Args:       []*Type{TAny},
	Impl:       Go(CondGuardHandler),
	Returns:    []*Type{TAny},
	BarrierPos: 0,
}

// codeGuardSignature is __codeguard's one signature, the one the lowering's
// guard call runs (BranchRecord.Guard).
var codeGuardSignature = Signature{
	Args:       []*Type{TAny},
	Impl:       Go(CodeGuardHandler),
	Returns:    []*Type{TAny},
	BarrierPos: 0,
}

// recordCaseSubject records case's own scrutinee rule over a scrutinee that
// may be a list at run time (NUR291), and the chain matches what it hands
// on — a value the pass cannot know, so the gradual any. The forward form's
// __casesubject runs a code body and hands its last result on, any other
// value as itself. The stack form's __casestack passes a value that is not
// a code body and defers on one: were the stack value a list, BOTH operands
// would be lists, which CaseHandler reads the forward way round (the
// clause list is the scrutinee), and no chain over these clauses is that.
// Each is the run's to match (a poly record, as any native over a dynamic
// operand); a record that did not seat the result leaves it no provenance,
// so seatable=false.
func recordCaseSubject(r *Registry, v Value, swapped bool, pos SrcPos) (Value, bool) {
	es := r.Check.Recorder()
	word := "__casesubject"
	if swapped {
		word = "__casestack"
	}
	subject := NewDynamicCarrier(TAny)
	es.RecordPolyCall(word, []Value{v}, []Value{subject}, pos, r, nil)
	return subject, es.CanSeatAcrossFragment(subject)
}

// caseCodeBodyRecord is CaseReturnsFn's RECORDING path for a code-body
// scrutinee (the shapes are listed at its call): the terminal trap for a
// body that nets nothing, the single-clause desugar, or the conservative
// dynAny that leaves the dispatch to the generic record.
func caseCodeBodyRecord(r *Registry, es EmitRecorder, v, clauses Value, dynAny []Value) []Value {
	nDiag := len(r.Check.Diagnostics)
	stk, binds, ran := condResidual(r, v)
	r.Check.TruncateDiagnostics(nDiag)
	// A scrutinee the pass cannot run — a CARRIER list, computed at run
	// time — nets what only the run knows: neither the trap nor the desugar
	// is a model of it (condResidual's doc has the miscompile the trap made
	// of it).
	if !ran {
		return dynAny
	}
	// A scrutinee that BINDS a name (`case [def x 5 1] […]`) keeps
	// the binding on the interpreter; only the desugared `if` below
	// runs the body as a kept condition fragment (NUR212), so every
	// other shape — the trap, the islanded multi-clause chain —
	// declines rather than read a stale binding after the `case`.
	if binds && len(stk) == 0 {
		declineCondBinding(r, v.Pos())
		return dynAny
	}
	if len(stk) == 0 {
		es.RecordTrap("case_error",
			"case: value expression produced no value to dispatch on",
			"case", "", v.Pos())
		return dynAny
	}
	if isCodeBody(clauses) {
		if lst, _ := AsList(clauses); !lst.IsNil() {
			elems := caseNormalizeClauses(r, lst.Slice())
			if len(elems) == 3 && !isCodeBody(elems[1]) && !isCodeBody(elems[2]) {
				if caseReStepDeclined(r, es, elems) {
					return dynAny
				}
				// `[do]` + the normal guard tokens: do runs the body once,
				// leaving its value as the scrutinee for the match.
				cond := NewList(append([]Value{NewWord("do")}, caseGuardTokens(v, elems[0])...))
				then := NewList(caseBlockTokens(v, elems[1]))
				rest := NewList(caseBlockTokens(v, elems[2]))
				// The desugared chain lowers its fragments INLINE where the
				// runtime CaseHandler isolates each block in a sub-engine —
				// bracket the desugar so an ambient-context write inside a
				// fragment declines instead of escaping its layer (NUR054).
				es.PushInlineCtxBoundary()
				out := if3ReturnsFn([]Value{cond, then, rest}, r)
				es.PopInlineCtxBoundary()
				return out
			}
		}
	}
	if binds {
		declineCondBinding(r, v.Pos())
	}
	return dynAny
}

// keepScrutineeBindings is the NON-recording twin of the scrutinee's kept
// condition (NUR212). A SUSPENDED run inside a compile pass — a `do` body's
// model run (DoListReturnsFn's RunCarrierBodyKeepDefs), a fn body's
// construction-time check — records nothing for the `case`, but the
// interpreter still runs its scrutinee exactly once, unconditionally, and
// every binding it makes stands after the construct. The model has to hold
// them too, exactly as analyseCondFragment holds an `if` condition's in the
// same runs (it gates on Armed, not Active). Skipping the run left them
// out: `def x 1 end do [case [def x 5] [5 "five" "other"]] end x` islanded
// the do over a model that still held x = 1, baked the 1, and compiled
// `[error(…) 1]` where the interpreter answers `[error(…) 5]`. Kept, the
// install reaches the bind ledger, whose twin regime places it or declines
// the program. The run's diagnostics are dropped — it exists for the
// model, and before it the pass reported nothing here. Plain check (no
// recorder armed) is unchanged, as it is for `if` conditions.
func keepScrutineeBindings(r *Registry, v Value) {
	if !r.Check.Recorder().Armed() || !IsConcrete(v) {
		return
	}
	nDiag := len(r.Check.Diagnostics)
	RunCarrierCondBodyKeepDefs(r, v)
	r.Check.TruncateDiagnostics(nDiag)
}

// CaseBranchJoin computes a `case` result TYPE as the join of every clause
// BLOCK's residual carrier (a block run with the case value pushed, per
// caseBlockTokens) plus the trailing default — the abstract analogue of the
// nested-`if` chain's branch join, without the emit desugar. A block that
// produces no value contributes None. Type-only: RunCarrierBody pauses any
// active recording, so this is safe whether or not emission is live.
func CaseBranchJoin(r *Registry, v Value, elems []Value) []Value {
	var out Value
	have := false
	join := func(block Value) {
		stk := RunCarrierBody(r, NewList(caseBlockTokens(v, block)))
		res := NewCarrier(TNone)
		if len(stk) > 0 {
			res = stk[len(stk)-1]
		}
		if have {
			out = JoinCarriers(out, res)
		} else {
			out, have = res, true
		}
	}
	i := 0
	for ; i+1 < len(elems); i += 2 {
		join(elems[i+1]) // clause block (odd-indexed; the even index is the match)
	}
	if i < len(elems) {
		join(elems[i]) // trailing default block
	}
	if !have {
		return []Value{NewDynamicCarrier(TAny)}
	}
	return []Value{out}
}

// caseGuardTokens builds the guard body for one clause: a code-body
// predicate `[pred]` runs as `v pred…` (matching runCaseBody), any other
// match dispatches `v match __casematch` (the same UnifyR CaseClauses uses).
func caseGuardTokens(v Value, m Value) []Value {
	if isCodeBody(m) {
		ml, _ := AsList(m)
		return append([]Value{v}, ml.Slice()...)
	}
	return []Value{v, m, NewWord("__casematch")}
}

// caseBlockTokens returns the tokens a clause block contributes: a code body
// runs with v pushed first (runCaseBody's convention), a plain value is
// itself.
func caseBlockTokens(v Value, b Value) []Value {
	if isCodeBody(b) {
		bl, _ := AsList(b)
		return append([]Value{v}, bl.Slice()...)
	}
	return []Value{b}
}

// buildCaseChain builds the token stream for the nested-`if` form of the
// clause list from index i: `if (v match __casematch) [block] [<rest>]`,
// recursing into the else body so a later clause only evaluates when the
// earlier guard fails. A trailing odd element is the default block.
func buildCaseChain(v Value, elems []Value, i int) []Value {
	if i+1 >= len(elems) {
		if i < len(elems) {
			return caseBlockTokens(v, elems[i]) // trailing default
		}
		return nil
	}
	guard := []Value{NewOpenParen()}
	guard = append(guard, caseGuardTokens(v, elems[i])...)
	guard = append(guard, NewCloseParen())
	out := []Value{NewWord("if")}
	out = append(out, guard...)
	out = append(out, NewList(caseBlockTokens(v, elems[i+1]))) // then [block]
	if rest := buildCaseChain(v, elems, i+2); rest != nil {
		out = append(out, NewList(rest)) // else [rest] — lazy
	}
	return out
}

// CaseMatchHandler is the runtime of __casematch: UnifyR(match, value) → ok.
// args[0] is the match (stack top), args[1] the case value (BarrierPos 0).
func CaseMatchHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_, ok := UnifyR(args[0], args[1], r)
	return []Value{NewBoolean(ok)}, nil
}

// runCaseBody executes a case block (or default): a code-body list
// runs in a sub-engine with the captured value pushed first — the
// same convention as the `error [handler]` block — so the block can
// consume it; any other value is the result as-is.
func runCaseBody(r *Registry, v Value, body Value) ([]Value, error) {
	if !isCodeBody(body) {
		return []Value{body}, nil
	}
	lst, _ := AsList(body)
	// The captured value is a resolved input: it enters as stack data the
	// block consumes, never re-stepped (arguments are inert; RunResolved).
	return RunResolved(r, []Value{v}, lst.Slice())
}
