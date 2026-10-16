package basic

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.Parent().Equal(TList) && v.Data() != nil && !IsTypedList(&v) && !IsTableType(v) {
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
	return v.Parent().Equal(TList) && v.Data() != nil && !IsTypedList(&v) && !IsTableType(v)
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
		condSlice := _lst.Elems()
		id := NextMarkID()
		tokens := make([]Value, 0, len(condSlice)+2)
		tokens = append(tokens, NewMark(id))
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
	if !IsWord(&m) {
		return false
	}
	w, _ := AsWord(&m)
	bound, ok := r.ResolveTypedName(w.Name)
	if !ok {
		return false
	}
	return bound.Parent().Equal(TFunction)
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
			if IsWord(&m) {
				w, _ := AsWord(&m)
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
			if IsParenExpr(&m) {
				if toks, perr := AsParenExpr(&m); perr == nil {
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
			return caseBlockRegion(r, v, elems[i+1])
		}
	}
	if i < len(elems) {
		// Trailing odd element: the default clause (an open-call tail
		// arrives here as one synthetic code-body block and so runs
		// sealed with the case value pushed first — exactly like a
		// matched arm, NUR048/G9).
		return caseBlockRegion(r, v, elems[i])
	}
	return nil, nil
}

// caseBlockRegion is runCaseBody for the block the handler returns as its
// result — the matched block or the default: the block is the handler's
// last act, so it is returned as a region of the tape (CallRegion) with the
// captured value its sealed input, and steps where the `case` stands. A
// plain value is the result as-is.
func caseBlockRegion(r *Registry, v Value, body Value) ([]Value, error) {
	if !isCodeBody(body) {
		return []Value{body}, nil
	}
	return CallRegion(r, "case", "case block", []Value{v}, body)
}

// CaseReturnsFn type-checks a `case`: the result TYPE is the join of every
// clause block's residual plus the trailing default (CaseBranchJoin), after
// the static coverage pass over the clauses (case_exhaustive.go). A code-body
// scrutinee is dynamic — its result type is not statically modelled — and so
// is a clause list the pass does not hold concretely.
func CaseReturnsFn(args []Value, r *Registry) []Value {
	// A ReturnsFn reads its operands positionally, so a window shorter than
	// its signature (a failed dispatch's recovery, NUR332) is answered with
	// the dynamic Any, never indexed.
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	dynAny := []Value{NewDynamicCarrier(TAny)}
	v, clauses := args[0], args[1]
	if isCodeBody(v) && !isCodeBody(clauses) {
		v, clauses = clauses, v
	}
	if isCodeBody(v) {
		// A code-body scrutinee is DYNAMIC — its result type is not
		// statically modelled — so the clause list must carry its own
		// catch-all (a trailing default or an Any clause). Emitted on
		// both the plain-check and compile paths (case_exhaustive.go).
		checkCaseCodeBodyScrutinee(r, v, clauses)
		// A code-body scrutinee (`case [body] [clauses]`) runs the body and
		// dispatches on its last result, which the pass does not model.
		return dynAny
	}
	if !isCodeBody(clauses) {
		// The clause argument is not a list: CaseHandler raises case_error.
		// The checker is lenient (returns a carrier).
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
	elems := caseNormalizeClauses(r, lst.Elems())
	// Static coverage pass (case_exhaustive.go) — BEFORE the compile-desugar /
	// plain-check paths diverge, so both report identical findings: a
	// default-less case whose clauses don't cover the scrutinee's static type
	// is a check error, plus the redundant-default / unreachable-clause
	// advisories.
	checkCaseExhaustiveness(r, v, clauses, elems)
	// At least one clause pair.
	if len(elems) < 2 {
		return dynAny
	}
	// The result TYPE: the join of every clause block's residual type plus
	// the trailing default.
	return CaseBranchJoin(r, v, elems)
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

// caseBlockTokens returns the tokens a clause block contributes: a code body
// runs with v pushed first (runCaseBody's convention), a plain value is
// itself.
func caseBlockTokens(v Value, b Value) []Value {
	if isCodeBody(b) {
		bl, _ := AsList(b)
		return append([]Value{v}, bl.Elems()...)
	}
	return []Value{b}
}

// runCaseBody executes a case body whose RESULT the handler reads — a
// predicate match, whose last value decides the clause: the code-body list
// runs in a sub-engine with the captured value pushed first — the same
// convention as the `error [handler]` block — so the body can consume it.
// The caller has tested isCodeBody first (a non-body match unifies). The
// matched block and the default are the handler's result and run on the
// tape instead (caseBlockRegion).
func runCaseBody(r *Registry, v Value, body Value) ([]Value, error) {
	lst, _ := AsList(body)
	// The captured value is a resolved input: it enters as stack data the
	// block consumes, never re-stepped (arguments are inert; RunResolved).
	return RunResolved(r, []Value{v}, lst.Elems())
}
