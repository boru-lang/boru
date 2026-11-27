package core

// The carrier BODY RUNNERS (ADR-013, 2026-08-08 amendment): running a
// nested body — a branch arm, a loop body, a `do`, an `if` condition —
// through a fresh sub-engine under analysis, and reporting what it left
// behind. Four entry points over one implementation, differing only in
// whether the body's defs roll back (a conditional arm) or leak (`do`),
// and whether the run is speculative (a condition fragment runs
// unconditionally exactly once, so it is not).
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
	return runCarrierBodyDefsAdds(r, body, false, false)
}

// RunCarrierCondBody is RunCarrierBodyWithDefs for an `if` CONDITION or a
// `case` code-body scrutinee: the fragment runs unconditionally exactly
// once BEFORE the branch decision, so it is not a speculative region.
// Defs still roll back keep=false-style.
func RunCarrierCondBody(r *Registry, body Value) ([]Value, map[string]Value) {
	stk, adds := runCarrierBodyDefsAdds(r, body, false, true)
	return stk, adds
}

// RunCarrierCondBodyKeepDefs is RunCarrierCondBody WITHOUT the def rollback
// (NUR212): an `if` condition or a `case` code-body scrutinee runs
// unconditionally, exactly once, BEFORE the branch decision — the
// interpreter runs it inline (a Mark/Move over the tape) — so every binding
// it makes is REAL on both engines and stands for the arms and for
// everything after the construct. The run therefore keeps its defs like
// `do`'s (RunCarrierBodyKeepDefs), and its installs are ledgered like any
// straight-line install, but it takes the branch-capture guard, not the
// keep-defs one: the body records into the condition FRAGMENT the lowering
// runs inline, so each install's bind twin is placed inside that fragment at
// its own site rather than adopted after a closure call.
func RunCarrierCondBodyKeepDefs(r *Registry, body Value) []Value {
	stk, _ := runCarrierBodyDefsAdds(r, body, true, true)
	return stk
}

// runCarrierBodyDefsAdds is the one body runner.
func runCarrierBodyDefsAdds(r *Registry, body Value, keep, condFrag bool) (stk []Value, adds map[string]Value) {
	if body.Data() == nil {
		return nil, nil
	}
	elems, err := AsList(body)
	if err != nil || elems.IsNil() {
		return nil, nil
	}

	// Snapshot def-stack depths (all known names).
	snapshot := r.Defs.Snapshot()

	sub := New(r)
	// Every body through here is a NESTED region (branch / loop /
	// quotation) — reached-conditionally by construction. Mark the depth
	// so unconditional-only diagnostics (unconditional_raise) stay silent.
	r.Check.NestedBodyDepth++
	speculative := !keep && !condFrag
	if speculative {
		// A rolled-back CONDITIONAL body (a branch arm or loop body) is a
		// speculative region: an `undef` of an enclosing binding inside it
		// must not leak the deletion into the model (SpecUndefBlocked — the
		// wrapped-undef FP class). keep=true (`do`) leaks by design; a
		// condition fragment runs unconditionally exactly once before the
		// branch decision, so its undefs are real.
		r.Check.PushSpecBaseline(snapshot)
	}
	result, err := sub.Run(elems.Elems())
	if speculative {
		r.Check.PopSpecBaseline()
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
	adds = map[string]Value{}
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
