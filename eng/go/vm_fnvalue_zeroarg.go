package eng

import (
	core "github.com/boru-lang/boru/core/go"
)

// vm_fnvalue_zeroarg.go — a NAMED fn value that takes no argument, applied
// over whatever lies beneath it, answered natively.
//
// Every native apply arm matches the value against the WHOLE window it was
// handed (core.MatchFnSig over all the args), because that is what the
// interpreter's plan does for a value that takes arguments. A value whose
// every own signature takes NONE never fits a non-empty window that way, so
// each arm declined and the window went to the interpreter: the token seam's
// `each g/v [1 2]` over `def g fn [[] [Integer] […]]` (RunResolved per
// element, the interp-entry census's user-types.tsv:L368), and the trailing
// apply `5 m.f` over a map holding `h/v`, `def h fn [[] [] []]`
// (callDynamic's island, fn-value.tsv:L319 and L320).
//
// The interpreter's step over such a window is fixed by the value alone.
// execFnDefLiteral's plan picks the value's zero-argument signature — it is
// the only one a plan can pick, and it takes no position — and a value that
// is not a `=>` lambda DISPATCHES it (only an anonymous lambda with nothing
// to take parks as data): the body runs in a frame of its own, over no
// arguments, and its results land on top of the window, which it never
// touched. So the answer is the window, then the body's results — the unit
// run through the token seam's own arm (runFnValueSig: the unit, the foreign
// home, the value's return contract), or nothing at all for an EMPTY body,
// which has no unit to run and nothing to leave.
//
// What stays the interpreter's, unchanged: an anonymous lambda (it parks), a
// value fnValueParkable does not describe (quoted, reach-grouped, a macro, an
// `apply`-marked value, a modifier wrapper, a named value from another
// module), a native (its own arms apply it), a value with any signature that
// takes an argument (the plan's pick depends on the window), a window value
// the end of an island run would touch (residualInert), an empty body that
// declares a return (its count error is the interpreter's to raise), and a
// body whose unit is unavailable.

// zeroArgFnValueApply runs fnVal's zero-argument signature over nothing and
// reports its results, or ran=false where the interpreter's step must decide
// (see the file comment). window is what lies beneath the value on the
// interpreter's tape.
func (vc *vmContext) zeroArgFnValueApply(reg *core.Registry, fnVal core.Value, window []core.Value) ([]core.Value, error, bool) {
	fd, ok := fnVal.Data.(core.FnDefInfo)
	if !ok || fd.Anonymous || !core.FnValueOnlyZeroArgSigs(fd) || !fnValueParkable(reg, fnVal, fd) {
		return nil, nil, false
	}
	for _, v := range window {
		if !residualInert(v) {
			return nil, nil, false
		}
	}
	// The zero-argument signature the interpreter's selection picks (its own
	// matcher, first match in own-signature order); only a boru body is this
	// arm's — a native's own arms apply it.
	sig := core.MatchFnSig(fnVal, nil)
	if sig == nil || !boruSig(sig) {
		return nil, nil, false
	}
	if len(sig.Body()) == 0 {
		if len(sig.Returns) > 0 {
			return nil, nil, false
		}
		return nil, nil, true
	}
	return vc.runFnValueSig(reg, fnVal, fd, sig, nil)
}

// boruSig reports whether sig runs a boru body.
func boruSig(sig *core.Signature) bool {
	_, boru := sig.Impl.(*core.BoruImpl)
	return boru
}

// zeroArgTokenSeam is the token seam's arm (invokeClosureOn): the inputs are
// the stack beneath the value, and they stay beneath its results.
func (vc *vmContext) zeroArgTokenSeam(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error, bool) {
	res, err, ran := vc.zeroArgFnValueApply(reg, body, inputs)
	if !ran || err != nil {
		return nil, err, ran
	}
	return append(append([]core.Value(nil), inputs...), res...), nil, true
}

// zeroArgCallDynamic is callDynamic's arm over the window stack[base:] — the
// value at base, its args after it (the trailing form's rotation undone). The
// island steps every arg — before the value in the trailing form, after it in
// the leading one — so each must be its own result (core.IsSteplessValue);
// the trailing args are the window beneath the value, and the leading ones
// land after its results.
func (vc *vmContext) zeroArgCallDynamic(reg *core.Registry, fnVal core.Value, args []core.Value, stack []core.Value, base int, trailing bool) ([]core.Value, error, bool) {
	for _, a := range args {
		if !core.IsSteplessValue(a) {
			return nil, nil, false
		}
	}
	var beneath []core.Value
	if trailing {
		beneath = args
	}
	res, err, ran := vc.zeroArgFnValueApply(reg, fnVal, beneath)
	if !ran || err != nil {
		return nil, err, ran
	}
	kept := append([]core.Value(nil), args...)
	out := stack[:base:base]
	if trailing {
		return append(append(out, kept...), res...), nil, true
	}
	return append(append(out, res...), kept...), nil, true
}
