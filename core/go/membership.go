package core

// Membership-defined types — the convergence point for the two ways the
// kernel installs a "type whose inhabitants are the values satisfying a
// rule": the boru path (predicateUnifier, rule = a boru fn body run via
// RunPredicate) and the host-Go path (MemberUnifier, rule = a Go func).
// Both answer the same question — "does v inhabit this type?" — at the
// same kernel touch-points (Match for dispatch, Unify for `is`/`case`/
// return checks, Format/canon, Compare). Only the evaluator differs, so
// the Match and Unify CORES live here and both Behaviors route through
// them; the per-path structs hold just the evaluator.

// baseBehavior returns the Behavior a membership Behavior delegates
// Format/Equal/Compare to: the Behavior it wrapped, or DefaultBehavior
// when it wrapped none.
func baseBehavior(prev TypeBehavior) TypeBehavior {
	if prev != nil {
		return prev
	}
	return DefaultBehavior
}

// baseCompare delegates Compare to prev when prev is a Comparer, else
// opts out (ErrNoComparer) — so a membership Behavior never invents an
// ordering it lacks, yet a `behave compare/q` wrapper underneath still
// participates.
func baseCompare(prev TypeBehavior, a, b Value) (int, error) {
	if c, ok := baseBehavior(prev).(Comparer); ok {
		return c.Compare(a, b)
	}
	return 0, ErrNoComparer
}

// matchMembership is the shared Match contract. A non-inhabitant — a bare
// type literal ("the type itself") or a carrier (a CheckMode abstract
// value) — defers to the lattice walk; a concrete value is put to member.
func matchMembership(v Value, t *Type, prev TypeBehavior, member func(Value) bool) bool {
	if !IsConcrete(&v) {
		// A check-mode CARRIER carrying a real closure payload (the
		// concrete-closure residual buildFnBodyReturnsFn / toCarrier
		// preserve) is payload-decidable: the FnDefInfo IS the value
		// that runs, so the predicate can answer now. Without this, a
		// member type over Function (the boru:minilang partial kinds,
		// MiniLang.Re / …) false-flags at check time — the strict
		// lattice walk rejects Parent=Function even though the payload
		// proves membership. Other carrier payloads (ChildTypeInfo on
		// list/map carriers, …) stay on the abstract path: they
		// describe a SHAPE, not the value itself.
		if v.Carrier {
			if _, ok := v.Data.(*FnDefInfo); ok {
				return member(v)
			}
		}
		return baseBehavior(prev).Match(v, t)
	}
	return member(v)
}

// unifyMembership is the shared Unify contract. The membership question —
// "does the concrete candidate inhabit this type?" — only applies when
// EXACTLY ONE operand is concrete (the candidate) and the other is the
// type (a bare literal, or a check-mode carrier). When BOTH operands are
// concrete it is a value-vs-value question (e.g. `stdin is stdout`, two
// distinct members of the SAME type), and when NEITHER is it is purely
// type-level; both defer to the structural rule (value / lattice
// equality), NOT membership — admitting the first satisfying member would
// otherwise treat distinct members as equal.
//
// On the membership branch it admits the candidate the oracle accepts
// (yielding the oracle's possibly-narrowed output), and fails
// DEFINITIVELY when the candidate is rejected — not ErrNoUnifier — so the
// kernel's structural fallback cannot re-admit a non-member by lattice
// subtyping alone.
//
// admit returns (output, matched, err): output is the value to yield on a
// match — the candidate itself for a Go predicate, or RunPredicate's
// result for a boru one.
func unifyMembership(a, c Value, typeName string, admit func(Value) (Value, bool, error)) (Value, *UnifyError) {
	aConc, cConc := IsConcrete(&a), IsConcrete(&c)
	if aConc == cConc {
		// Both concrete (value vs value) or both abstract (type level):
		// settle by the structural rule, not the predicate.
		return unifySameOrSubtype(a, c)
	}
	candidate := a
	if cConc {
		candidate = c
	}
	out, matched, err := admit(candidate)
	if err != nil {
		return Value{}, &UnifyError{Reason: typeName + ": " + err.Error(), A: a, B: c}
	}
	if matched {
		return out, nil
	}
	return Value{}, unifyFail("value is not a member of "+typeName, a, c)
}
