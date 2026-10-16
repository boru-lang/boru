package check

import core "github.com/boru-lang/boru/core/go"

// tryFoldScalarConst const-folds a CompileScalarFold dispatch whose operands
// are ALL inert consts, by running the real handler concretely — twice, with
// a determinism-agreement guard — and returning the concrete result when it
// is itself an inert const. An erroring dispatch (family-restricted ordering
// over cross-family operands) declines, keeping the ordinary diagnostic
// path. See CompileScalarFold (core/go/value.go) for the motivation
// (concrete-condition folding: the check pass sees `(n eq 0)` as a concrete
// false when n is a known const, so a branch on it is decided). (Moved here
// from the compiler module when the compiled lane was removed, 2026-10-16:
// the fold ran on the plain check pass all along.)
func tryFoldScalarConst(r *core.Registry, sig *core.Signature, args []core.Value) (core.Value, bool) {
	if sig == nil || !sig.CompileEffect.Has(core.CompileScalarFold) ||
		sig.DispatchHandler() == nil || len(sig.NoEvalArgs) > 0 || len(args) == 0 {
		return core.Value{}, false
	}
	// The handler runs on args in sig order, one per declared param. A window
	// of another length never reaches a real dispatch — the check pass's
	// no-match recovery (checkModeAssumeSig) models its best-fit overload
	// over the args the real match REJECTED, which can be fewer than the sig
	// declares (`n div 0 gt 0` inside a fn body assumed `gt`'s two-slot
	// DepScalar constructor over a one-value window) — and the handler indexes
	// its params unguarded: the fold panicked with an index out of range.
	if len(args) != sig.TotalArgs() {
		return core.Value{}, false
	}
	for i, a := range args {
		// A type literal at a declared type slot — `convert Bytes "m"`'s
		// target — is known by construction.
		if sig.TypeArgs[i] && core.IsBareTypeNode(&a) {
			continue
		}
		if !ScalarFoldOperand(a) {
			return core.Value{}, false
		}
	}
	one, ok := concreteHandlerEval(r, sig, args)
	if !ok {
		return core.Value{}, false
	}
	two, ok := concreteHandlerEval(r, sig, args)
	if !ok || !core.ConstFoldAgrees(one, two) {
		return core.Value{}, false
	}
	if !core.IsInertConst(&one) {
		return core.Value{}, false
	}
	return one, true
}

// concreteHandlerEval runs sig.DispatchHandler() on the already-resolved args
// with check mode OFF, so a pure reader produces its REAL value rather than
// the declared-type carrier the check-mode ReturnsFn emits. The def stack is
// snapshotted / restored so a stray binding cannot leak. Returns the single
// result when it is a concrete value or a bare type node (typeof's type
// literal). Mirrors concreteEvalOnce, but dispatches the one matched native
// instead of re-running a token stream — the args are already in sig order.
func concreteHandlerEval(r *core.Registry, sig *core.Signature, args []core.Value) (core.Value, bool) {
	// A handler runs over its signature's full arity, as every dispatch
	// hands it; a recovery's assumed signature over a shorter window
	// (`filter (mk) [gt 1]`'s auto-evaluated `gt 1`, one operand for gt's
	// two) reached a handler that indexes its second argument, and the
	// check pass panicked (NUR265).
	if len(args) != sig.TotalArgs() {
		return core.Value{}, false
	}
	snap := r.Defs.Snapshot()
	prev := r.Check.Mode
	r.Check.Mode = false
	res, err := sig.DispatchHandler()(args, nil, nil, r)
	r.Check.Mode = prev
	r.Defs.Restore(snap)
	if err != nil || len(res) != 1 {
		return core.Value{}, false
	}
	if core.IsConcrete(&res[0]) || (core.IsBareTypeNode(&res[0]) && core.IDOf(&res[0]) != "") {
		return res[0], true
	}
	return core.Value{}, false
}
