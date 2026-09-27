package eng

import (
	core "github.com/boru-lang/boru/core/go"
)

// vm_fnvalue_park.go — a fn VALUE that no signature admits, answered natively.
//
// When a dynamic apply hands the VM a fn value its window does not fit — the
// leading form's `m.f 'x'` over an Integer-only lambda, a callDynFrame region
// re-stepping `args.0` over a param it rejects, a lambda handed to `fold`'s
// token seam over an accumulator it does not take — every native arm declines
// (they all apply only a MATCH), and the window went to the interpreter so its
// step could decide what an unmatched value does (the interp-entry census's
// fn-value.tsv:L28, module-fnvalue-boundary.tsv:L24, fold-map-filter.tsv:L87
// and L88: `vm:island` / `vm:island-resolved` / RunResolved, 2026-09-27). The
// closure twin of this fork has been native for a while (callDynamic's
// ClosureIsFnValue arm); the FnDefInfo one had not.
//
// The fork is execFnDefLiteral's, read in its order and with its own
// matchers, so what this answers is what the interpreter's step answers:
//
//   - the PLAN (core.PlanMatch over the window laid out as the island's tape —
//     the stack beneath at [0, pointer), the value at the pointer, the forward
//     tokens after it — over the value's dispatch view, the signatures
//     compileFnDef builds), then its /s retry for a forward-collecting value;
//     a Fallback pick is no pick, as there;
//   - the legacy pure-stack path the step falls to on no plan
//     (core.FnValueStackMatches, ExecFnDefSigStackMatch's own selection);
//   - with neither: a NAMED value with at least one candidate operand raises
//     `uncalled_function` at core.UncalledRaisePos (a name always calls,
//     ADR-011); any other value PARKS — the value is data and the window
//     stands as written.
//
// Every guard below keeps a window the fork above does not describe on its
// island, unchanged:
//
//   - a quoted, reach-grouped, macro or `apply`-marked value (each has its own
//     earlier exit in execFnDefLiteral), a modifier wrapper (its own
//     dispatch), and a NAMED value from another module (the step looks the
//     name up there rather than reading the value's signatures);
//   - a value with a real 0-argument signature: the plan's fallback section
//     would pick it and the step would apply it, which is not a no-match;
//   - a forward token that is not stepless: the island STEPS the tokens after
//     a parked value, and only a stepless scalar is its own result
//     (core.IsSteplessValue) — which is also why the forward pre-walk
//     (CollectForward) has nothing to do here: it evaluates parens, lists and
//     sugar markers, none of which a stepless token is;
//   - a stack value the end of the island's run would touch: a pending list or
//     map evaluates as a sub-program there, an inert typed container or a
//     disjunct resolves by shape (autoEvalResidual), and a marker is
//     tape-coupled — so only a value that sweep passes through unchanged is
//     admitted beneath the fn.

// fnValueNoMatch is the interpreter's verdict on an unmatched fn value.
type fnValueNoMatch int

const (
	// noMatchUnproven: the fork above does not settle the window (a guard
	// declined, or something matched) — the caller keeps its island.
	noMatchUnproven fnValueNoMatch = iota
	// noMatchPark: the value is data; the window stands as written.
	noMatchPark
	// noMatchRaise: a named value's uncalled_function.
	noMatchRaise
)

// fnValueNoMatchVerdict answers, for the fn value fnVal stepped at the pointer
// over resolved (the stack beneath it, bottom to top) and forward (the tokens
// written after it), whether the interpreter's step dispatches nothing — and
// if so, whether it parks the value or raises.
func fnValueNoMatchVerdict(reg *core.Registry, fnVal core.Value, resolved, forward []core.Value) (fnValueNoMatch, core.FnDefInfo) {
	fd, ok := fnVal.Data.(core.FnDefInfo)
	if !ok || !fnValueParkable(reg, fnVal, fd) {
		return noMatchUnproven, fd
	}
	for _, v := range resolved {
		if !residualInert(v) {
			return noMatchUnproven, fd
		}
	}
	for _, v := range forward {
		if !core.IsSteplessValue(v) {
			return noMatchUnproven, fd
		}
	}
	view := fnDispatchView(fd)
	for i := range view.Signatures {
		if s := &view.Signatures[i]; !s.Fallback && s.TotalArgs() == 0 {
			return noMatchUnproven, fd
		}
	}
	window := make([]core.Value, 0, len(resolved)+1+len(forward))
	window = append(append(append(window, resolved...), fnVal), forward...)
	h := newRegionHostOver(reg, window)
	w := core.WordInfo{Name: fd.Name, ArgCount: -1}
	if planPicks(h, reg, &view, w, resolved) {
		return noMatchUnproven, fd
	}
	if view.HasForwardSigs() {
		w.ForceStack = true
		if planPicks(h, reg, &view, w, resolved) {
			return noMatchUnproven, fd
		}
	}
	if core.FnValueStackMatches(fd, resolved) {
		return noMatchUnproven, fd
	}
	if fd.NamedDef() && len(resolved)+len(forward) > 0 {
		return noMatchRaise, fd
	}
	return noMatchPark, fd
}

// fnValueParkable is the value-level half of the guards (see the file
// comment): the shapes whose unmatched step is execFnDefLiteral's no-match
// fork over the value's own signatures.
func fnValueParkable(reg *core.Registry, fnVal core.Value, fd core.FnDefInfo) bool {
	if fnVal.Quoted || fnVal.ReachGroup || fd.Macro || fd.Applied || len(fd.Signatures) == 0 {
		return false
	}
	if fd.NamedDef() && core.FnHomeForeign(reg, &fd) {
		return false
	}
	_, _, wrapped := core.UnwrapModifierChain(fnVal)
	return !wrapped
}

// planPicks reports whether the interpreter's plan picks a signature — a
// Fallback pick is none, as the step treats it.
func planPicks(h *regionHost, reg *core.Registry, view *core.FnDefInfo, w core.WordInfo, resolved []core.Value) bool {
	sig, _, _ := core.PlanMatch(h, h.win, reg, view, w, resolved, len(resolved), false, false, false)
	return sig != nil && !sig.Fallback
}

// fnDispatchView is the signature view the interpreter's step matches a fn
// value under (compileFnDef, without the body runners a match would need):
// each signature with its all-forward barrier sentinel resolved, normalised,
// and sorted into dispatch order.
func fnDispatchView(fd core.FnDefInfo) core.FnDefInfo {
	sigs := make([]core.Signature, len(fd.Signatures))
	for i := range fd.Signatures {
		s := fd.Signatures[i]
		s.Params = append([]core.FnParam(nil), s.Params...)
		if s.BarrierPos == core.BarrierAllForward {
			s.BarrierPos = len(s.Params)
		}
		core.NormalizeSig(&s)
		sigs[i] = s
	}
	core.SortSignatures(sigs)
	view := fd
	view.Signatures = sigs
	return view
}

// residualInert reports whether the end of an island run passes v through
// unchanged (see the file comment's last guard).
func residualInert(v core.Value) bool {
	if tapeCoupled([]core.Value{v}) {
		return false
	}
	if v.Quoted {
		return true
	}
	return !v.Eval && !core.IsTypedList(v) && !core.IsTypedMap(v) && !core.IsDisjunct(v)
}

// uncalledAt is the named value's raise for an unmatched window: the
// interpreter's code, detail and hint, at the position its step reports.
func uncalledAt(reg *core.Registry, fnVal core.Value, fd core.FnDefInfo, candidates []core.Value) *core.BoruError {
	return uncalledFunctionErrorAt(reg, fd.Name, core.UncalledRaisePos(fnVal.Pos(), candidates))
}

// callDynamicNoMatch is callDynamic's no-match arm over the window
// stack[base:] — the value at base and its args after it (the trailing form's
// rotation undone). handled=false leaves the island to decide.
func (vc *vmContext) callDynamicNoMatch(reg *core.Registry, fnVal core.Value, args, stack []core.Value, base int, trailing bool, curDebug []core.SrcPos, pc int) ([]core.Value, bool, error) {
	resolved, forward := []core.Value(nil), args
	if trailing {
		// The island places the args before stepping the value: they are
		// the stack beneath it, and they were STEPPED to get there.
		for _, a := range args {
			if !core.IsSteplessValue(a) {
				return nil, false, nil
			}
		}
		resolved, forward = args, nil
	}
	switch verdict, fd := fnValueNoMatchVerdict(reg, fnVal, resolved, forward); verdict {
	case noMatchPark:
		if !trailing {
			return stack, true, nil
		}
		rotated := append(stack[:base:base], args...)
		return append(rotated, fnVal), true, nil
	case noMatchRaise:
		return nil, true, stampAt(uncalledAt(reg, fnVal, fd, args), curDebug, pc, reg)
	}
	return nil, false, nil
}
