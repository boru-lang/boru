package eng

import (
	core "github.com/boru-lang/boru/core/go"
)

// vm_mixed_plan.go — CALL_DYNAMIC_MIXED's window bound by the interpreter's
// own plan, and the fn value applied natively.
//
// The mixed apply exists because the compiler could not rule out a callable
// value INTERIOR to a window of static args (`3 m.f 2`): the value's arity,
// and so which values it takes from each side, is a run-time fact. Columns (q)
// and (v) of the interp-entry census took the two shapes that need no binding
// at all (a stepless window is its own residual; a trailing native takes the
// window top-down). What was left islanded is the SPLIT: one forward token
// fills param 0 and the stack supplies param 1, which is why `3 m.f 2` over
// `(fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])` is 203 and not
// 302 (the interp-entry census's fn-value.tsv:L19).
//
// That split is not re-derived here. core.PlanMatch IS the interpreter's
// collection kernel — execFnDefLiteral asks it the same question over the
// same window — and this file asks it over the island's tape (the window laid
// out with the value at the pointer, as fnValueNoMatchVerdict does), then
// follows the step's own path past the pick for the one shape where that path
// is fixed by the pick:
//
//   - one own signature, a boru body, every param NAMED: a single signature
//     leaves the post-collection re-match nothing else to choose, and named
//     params bind top-down on every path the step can take (the handler's
//     sig-order args, the legacy stack match's named branch);
//   - the plan's positions are the window's contiguous halves — the top of
//     the stack beneath the value and the tokens right after it — so what the
//     value does not take stays where the island leaves it: the untaken stack
//     beneath, the untaken tokens (stepless, so their own results) after;
//   - a value that is not a `=>` lambda re-steps through the legacy pure-stack
//     path once its forward args are collected, and that path must dispatch
//     the same signature over the collected layout (core.FnValueStackMatches
//     over the stack the collection leaves: the untaken prefix, then the args
//     with the first on top).
//
// The value then runs through the token seam's arm (runFnValueSig: its unit,
// its home, its return contract). Everything else keeps the island, unchanged:
// a window with a second non-stepless value, a value fnValueParkable does not
// describe, several signatures, an unnamed param, a zero-argument signature
// (vm_fnvalue_zeroarg.go's shape), no pick, a pick that straddles, a legacy
// re-match that would not dispatch, and a value with no unit.

// mixedPlanApply answers the mixed window natively, reporting ran=false where
// the island must (see the file comment).
func (vc *vmContext) mixedPlanApply(reg *core.Registry, window []core.Value) ([]core.Value, error, bool) {
	p := 0
	for p < len(window) && core.IsSteplessValue(window[p]) {
		p++
	}
	if p == len(window) || !core.IsSteplessWindow(window[p+1:]) {
		return nil, nil, false
	}
	fnVal := window[p]
	fd, ok := fnVal.Data.(core.FnDefInfo)
	if !ok || !fnValueParkable(reg, fnVal, fd) {
		return nil, nil, false
	}
	own := fd.OwnSigs()
	if len(own) != 1 || !boruSig(&own[0]) || core.FnValueOnlyZeroArgSigs(fd) || !allParamsNamed(own[0].Params) {
		return nil, nil, false
	}
	nStk, nFwd, args, ok := mixedPlanArgs(reg, fd, window, p)
	if !ok || (!fd.Anonymous && !legacyReMatchAgrees(fd, window[:p-nStk], args)) {
		return nil, nil, false
	}
	res, err, ran := vc.runFnValueSig(reg, fnVal, fd, &own[0], args)
	if !ran || err != nil {
		return nil, err, ran
	}
	out := append(append([]core.Value(nil), window[:p-nStk]...), res...)
	return append(out, window[p+1+nFwd:]...), nil, true
}

// legacyReMatchAgrees reports whether the step's legacy pure-stack path — the
// one a value that is not a `=>` lambda re-steps through once its forward
// args are collected — dispatches the value's signature over the stack the
// collection leaves: the untaken values beneath, then the args with the
// first on top (core.FnValueStackMatches, the path's own selection).
func legacyReMatchAgrees(fd core.FnDefInfo, below, args []core.Value) bool {
	laid := append([]core.Value(nil), below...)
	for i := len(args) - 1; i >= 0; i-- {
		laid = append(laid, args[i])
	}
	return core.FnValueStackMatches(fd, laid)
}

// mixedPlanArgs runs the interpreter's plan (and its /s retry) for the value
// at window[p] and returns how many values it takes from beneath and after
// the value, and the args in signature order — ok=false for no pick, a
// Fallback pick, or positions that are not the window's contiguous halves.
func mixedPlanArgs(reg *core.Registry, fd core.FnDefInfo, window []core.Value, p int) (nStk, nFwd int, args []core.Value, ok bool) {
	view := fnDispatchView(fd)
	h := newRegionHostOver(reg, window)
	w := core.WordInfo{Name: fd.Name, ArgCount: -1}
	resolved := window[:p]
	sig, positions, _ := core.PlanMatch(h, h.win, reg, &view, w, resolved, p, false, false, false)
	if (sig == nil || sig.Fallback) && view.HasForwardSigs() {
		w.ForceStack = true
		sig, positions, _ = core.PlanMatch(h, h.win, reg, &view, w, resolved, p, false, false, false)
	}
	nStk, nFwd, halves := contiguousHalves(positions, p)
	if sig == nil || sig.Fallback || !halves {
		return 0, 0, nil, false
	}
	args = make([]core.Value, len(positions))
	for i, pos := range positions {
		args[i] = window[pos]
	}
	return nStk, nFwd, args, true
}

// contiguousHalves counts the plan's positions beneath (nStk) and after (nFwd)
// the pointer p and reports whether they are exactly the nStk values right
// beneath it and the nFwd tokens right after it — the only layout the
// collection kernel produces (its forward phase takes the next tokens in
// order, its stack phase the top values down), checked rather than assumed.
func contiguousHalves(positions []int, p int) (nStk, nFwd int, ok bool) {
	for _, pos := range positions {
		if pos < p {
			nStk++
		} else {
			nFwd++
		}
	}
	ok = true
	for _, pos := range positions {
		ok = ok && pos != p && pos >= p-nStk && pos <= p+nFwd
	}
	return nStk, nFwd, ok
}

// allParamsNamed reports whether every param binds by name.
func allParamsNamed(params []core.FnParam) bool {
	for _, prm := range params {
		if prm.Name == "" {
			return false
		}
	}
	return true
}
