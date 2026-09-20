package core

// The carrier BODY RUNNERS (ADR-013, 2026-08-08 amendment): running a
// nested body — a branch arm, a loop body, a `do`, an `if` condition —
// through a fresh sub-engine under analysis, and reporting what it left
// behind. Four entry points over one implementation, differing only in
// whether the body's defs roll back (a conditional arm) or leak (`do`),
// and whether the run raises CondBodyDepth (a condition fragment runs
// unconditionally exactly once, so it does not).
//
// This is core, not check, because everything it touches is core-owned:
// the sub-engine (New), the def table (r.Defs), the checker's STATE
// (r.Check — CheckState lives in check_state.go here), and the emit
// recorder interface. A word library's control words need it to carry
// their analysis half; see carrier_join.go's header.

// RunCarrierBody runs a list body (a Value with Parent=TList) through a
// fresh sub-engine in check mode and returns the residual carrier
// stack. Returns nil if the body is not a concrete list. Requires
// that the registry is already in CheckMode (callers set it).
//
// Used by branch-aware words (e.g. `if`) to analyse each branch
// symbolically.
func RunCarrierBody(r *Registry, body Value) []Value {
	stk, _ := RunCarrierBodyWithDefs(r, body)
	return stk
}

// RunCarrierBodyKeepDefs is RunCarrierBody WITHOUT the def rollback — the
// check-mode twin of `do`'s runtime scoping, where body defs LEAK to the
// enclosing scope (`do [def x 5] end x add 1` → 6; a do-installed TYPE stays
// bound after the do). Rolling do-defs back was an infidelity with two
// symptoms: post-do reads flagged undefined (the wrapped-context
// false-positive family) and a do-installed type's binding vanishing while
// its minted part survived, so a later re-analysis of the SAME body tripped
// the parts conflict instead of the type-shadow path (validateTypeName's
// Defs.IsType skip). Branch / loop / quotation bodies keep the rollback —
// their execution is conditional and their defs are join-managed.
func RunCarrierBodyKeepDefs(r *Registry, body Value) []Value {
	stk, _ := runCarrierBodyDefsAdds(r, body, true, false)
	return stk
}

// RunCarrierBodyWithDefs is the branch-aware helper that snapshots
// DefStack depths, runs the body through a sub-engine in check
// mode, and returns both the residual carrier stack and a map of
// every DefStacks[name] -> top-of-stack entry that was added
// during analysis. The top entry is popped (restored to snapshot)
// so the caller can decide whether to re-push, join, or discard.
//
// Only per-name "net additions" are reported. If a branch both
// pushes and pops for the same name, the net change is zero and
// the name is not in the returned map.
func RunCarrierBodyWithDefs(r *Registry, body Value) ([]Value, map[string]Value) {
	stk, adds := runCarrierBodyDefsAdds(r, body, false, false)
	return stk, adds
}

// RunCarrierCondBody is RunCarrierBodyWithDefs for an `if` CONDITION or a
// `case` code-body scrutinee: the fragment runs unconditionally exactly
// once BEFORE the branch decision, so it does NOT raise CondBodyDepth —
// an in-place fn redefinition there is not path-dependent and stays
// compilable (the paren-`do` condition twin compiles it with parity).
// Defs still roll back keep=false-style, exactly as before.
func RunCarrierCondBody(r *Registry, body Value) ([]Value, map[string]Value) {
	stk, adds := runCarrierBodyDefsAdds(r, body, false, true)
	return stk, adds
}

func runCarrierBodyDefsAdds(r *Registry, body Value, keep, condFrag bool) ([]Value, map[string]Value) {
	if body.Data == nil {
		return nil, nil
	}
	elems, err := AsList(body)
	if err != nil || elems.IsNil() {
		return nil, nil
	}

	// Nested body analysis is not part of the enclosing straight
	// line: pause bytecode recording — unless a branch-lowering hook
	// armed fragment capture (the `if` ReturnsFn), in which case the
	// body's events record into a fragment for structured lowering.
	// Peek the arm BEFORE the guard consumes it: when recording into a
	// fragment, mark the body sub-engine element-eval-recordable so a
	// residual computed container it returns (`{a: x}` / `[x y]`, an if
	// arm's map/list value) records its OpMakeMap / OpMakeList assembly
	// instead of leaving an unresolvable residual. The branch/loop body
	// runs in the LIVE frame (its def-locals are present), so the
	// re-assembled-per-run operand semantics are sound — the same
	// property that already makes CONSUMED-arg container recording safe
	// in a fn body.
	recordable := r.Check.Recorder().PeekCaptureArm()
	// A KEEP-DEFS run (`do`'s scoping) takes the keep-aware guard so the
	// recorder can bracket the run's bind twins for do-body adoption; every
	// other body (branch / loop / quotation — conditional or multi-run)
	// takes the plain guard, which inside a keep run marks its twins as a
	// tainted sub-range no adoption may place.
	if keep {
		defer r.Check.Recorder().KeepDefsBodyGuard(r, body.ID)()
	} else {
		defer r.Check.Recorder().BodyAnalysisGuard()()
	}

	// Snapshot def-stack depths (all known names).
	snapshot := r.Defs.Snapshot()

	tokens := make([]Value, elems.Len())
	copy(tokens, elems.Slice())
	sub := New(r)
	sub.ElemEvalRecordable = recordable
	// Every body through here is a NESTED region (branch / loop /
	// quotation) — reached-conditionally by construction. Mark the depth
	// so unconditional-only diagnostics (unconditional_raise) stay silent.
	// A def-rolled-back body (keep=false — a branch arm or loop body) also
	// raises CondBodyDepth: unlike `do` (keep=true, which leaks its defs
	// unconditionally), its bindings are conditional, so an in-place fn
	// redefinition that clobbers an enclosing overload there is unsound to
	// compile (installDef consults CondBodyDepth to decline it). Condition/
	// scrutinee fragments (condFrag — RunCarrierCondBody) are exempt: they
	// run unconditionally exactly once before the branch decision, so a
	// redefinition there is not path-dependent.
	r.Check.NestedBodyDepth++
	// A keep=false body's def growth is TRUNCATED below, so every install
	// inside it is SPECULATIVE and the bind ledger must not record it — the
	// binding the pass actually leaves is whatever InstallJoinedDefs puts
	// back, or nothing. Wider than raiseCond on purpose: a condition
	// fragment is truncated too, even though it is not conditional.
	if !keep {
		r.Check.RolledBackBodyDepth++
		defer func() { r.Check.RolledBackBodyDepth-- }()
	}
	raiseCond := !keep && !condFrag
	if raiseCond {
		r.Check.CondBodyDepth++
		// A rolled-back CONDITIONAL body is a speculative region: an
		// `undef` of an enclosing binding inside it must not leak the
		// deletion into the model (SpecUndefBlocked — the wrapped-undef FP
		// class). keep=true (`do`) leaks by design; a condition fragment
		// runs unconditionally, so its undefs are real on both engines.
		r.Check.PushSpecBaseline(snapshot)
	}
	result, err := sub.Run(tokens)
	if raiseCond {
		r.Check.PopSpecBaseline()
		r.Check.CondBodyDepth--
	}
	r.Check.NestedBodyDepth--
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "branch_error",
			Detail: "branch analysis error: " + err.Error(),
		})
		result = nil
	}

	// Keep-defs mode (`do` — leak fidelity): the body's bindings stay,
	// exactly as the runtime leaves them; nothing to report.
	if keep {
		return result, nil
	}
	// Collect the top of each def stack whose depth grew, then
	// restore depths back to snapshot.
	adds := map[string]Value{}
	for _, k := range r.Defs.Names() {
		before := snapshot[k] // zero for names not present before
		depth := r.Defs.Depth(k)
		if depth > before {
			top, _ := r.Defs.Top(k)
			adds[k] = top
			r.Defs.Truncate(k, before)
		}
	}
	return result, adds
}
