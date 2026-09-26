package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The fn-VALUE arm of the body seam (S1b of design/FULL-COMPILATION-REPLAN.0.md).
//
// A higher-order word's list arm hands its callback to InvokeBody whatever
// the callback is — a token list, or a fn VALUE (`each h.cb [1 2 3]`, `fold
// r.step xs 0`, a lambda over a gradual collection S1a re-matched at run
// time). For a token list the VM's invoker has nothing to run natively and
// steps the tokens on a pooled sub-engine (RunResolved). A fn value used to
// take the same path — the value is one token, stepped, dispatched by the
// interpreter's own rule — which is exactly the interpreter entry the
// interp-entry census counts once per S1a row (fifty-one of them entered
// through this seam on 2026-09-19).
//
// invokeFnValue is that dispatch, made native. It asks the interpreter's own
// questions in the interpreter's own order, and declines — the stepping path
// stands, byte-identical — whenever the answer would not be the unit:
//
//   - The match is core.MatchFnSig over the inputs in the TOKEN seam's order:
//     the seam pushes the inputs and steps the value, so the sig binds from
//     the stack top down — `fold ([a e] => [a sub e]) [1 2 3] 10` is -8 on
//     both lanes because a is the ELEMENT and e the accumulator (measured
//     2026-09-19). A value with no matching own sig declines: the stepping
//     path decides between data (an anonymous lambda stays on the stack, the
//     element's result — NUR155's interpreter rule) and uncalled_function (a
//     named fn), and this arm must not re-implement that fork.
//   - The unit is the sig's compiled ref — stamped at compile time, at module
//     load, or right now by the lazy detached stamp (compiler.LazyStampFnSig),
//     memoised on the value. A stale detached ref re-stamps (the JIT box); a
//     declined one keeps the stepping path.
//   - A value whose home is another module runs through core.InvokeCallback
//     at that home, which is what the interpreter's own stepping does for a
//     foreign fn value (execFnDefSig's cross-registry branch): the CallBoru
//     discipline there is the interpreter's, not this arm's to change.
//   - A same-home value is hosted here as a nested unit — hostForeign, the
//     arm invokeClosureOn already uses for a closure from another program —
//     with the TOKEN seam's return discipline: the root RET trims nothing,
//     and the value's OWN contract (its name, its declared returns, the
//     unnamed-param allowance) is enforced over the residual exactly as
//     checkClosureReturn enforces a closure's — `each (x:Integer => [x 1])
//     [1 2]` raises `: expected 1 return value(s), got 2 — [1 1]` on both
//     lanes, `each cbad/v [1 2]` the return-type error with cbad's name.
//   - Delivery is OpCallUserPoly's: the ascribed view stripped, list params
//     quoted, the interpreter's binding rule.
//   - An internal error inside the unit PROPAGATES. It used to degrade to the
//     stepping path when no observable effect had escaped, fenced so a unit
//     that had printed did not print twice; nothing re-runs now, so the bail
//     is the compiler defect it is and it surfaces either way.
func (vc *vmContext) invokeFnValue(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error, bool) {
	fd, ok := body.Data.(core.FnDefInfo)
	if !ok || body.Quoted {
		return nil, nil, false
	}
	args := make([]core.Value, len(inputs))
	for i, v := range inputs {
		args[len(inputs)-1-i] = v
	}
	sig := core.MatchFnSig(body, args)
	if sig == nil {
		return nil, nil, false
	}
	home, caps := core.FnHome(reg, &fd)
	ref := compiler.CompiledRef(sig)
	if ref == nil {
		ref = compiler.LazyStampFnSig(reg, fd, sig, body.Pos())
	}
	if ref == nil || ref.Prog == nil {
		return nil, nil, false
	}
	if core.FnHomeForeign(reg, &fd) {
		// The interpreter's own foreign arm, minus the tape: the residual is
		// the call's results in place of the consumed inputs and the value.
		res, err := core.InvokeCallback(home, sig, deliverArgs(args), caps)
		return res, err, true
	}
	if !ref.DepsFresh(reg) {
		if ref = ref.JitRestamp(reg); ref == nil || ref.Prog == nil {
			return nil, nil, false
		}
	}
	if ref.Unit < 0 || ref.Unit >= len(ref.Prog.Fns) {
		return nil, nil, false
	}
	unit := &ref.Prog.Fns[ref.Unit]
	if unit.NParams-len(ref.Captures) != len(args) || unit.FnReadRefused(args) {
		// A compile/run drift: entering on it would bind the wrong locals
		// silently (dynApplyEnter's rule) — the stepping path answers; as it
		// does a fn argument in a slot the unit reads bare (NUR217).
		return nil, nil, false
	}
	delivered := deliverArgs(args)
	// The fn's own frame: the interpreter's dispatch pushes the per-call
	// args list for every frame, and a DynEnv unit reads `args` from it.
	popArgs := pushRootArgs(reg, ref.Prog, delivered)
	res, err := vc.hostFnValueUnit(reg, ref, delivered)
	popArgs()
	if err != nil {
		// A soundness bail here used to hand the call to the stepping path
		// and let the interpreter answer it, guarded by the effect fence so
		// a unit that had already printed did not print twice. Nothing
		// re-runs now: the bail is the defect it is and it surfaces.
		return res, err, true
	}
	return checkFnValueReturn(reg, fd, sig, res, unit.NUnnamed, body.Pos())
}

// invokeFnValueClosure is the token seam's arm for a fn-VALUE CLOSURE — a
// capturing `fn` / `=>` literal minted at run time (a factory's result, a
// def-bound one read back; compiler.ClosureIsFnValue), where invokeFnValue
// above handles an FnDefInfo value. The closure's unit carries the value's
// declared param contract, so the interpreter's own match runs over it the
// same way: the bridged signature (closureFnDef — what the interpreter's
// dispatch of the same closure matches under) against the inputs in the
// seam's top-down order. A match runs the unit over the signature-ordered
// args; no match hands the value to the stepping path, where the sub-engine
// meets the closure at the pointer and the interpreter's data-versus-
// uncalled_function fork decides (NUR155's rule). Measured before the arm
// (2026-09-19, the S1a/S1b-1 head): the unit ran blind over whatever the
// handler pushed — `each (mk 1) ['a' 2]` answered `[1 3]` for the
// interpreter's `[fn (Integer) 3]` — and every such shape was a compile failure on
// `main` ("function-valued operand at each"), so the release was S1a's and
// the arm is what makes it sound.
func (vc *vmContext) invokeFnValueClosure(reg *core.Registry, body core.Value, cl core.ClosurePayload, inputs []core.Value) ([]core.Value, error, bool) {
	if body.Quoted || !compiler.ClosureIsFnValue(body) {
		return nil, nil, false
	}
	p := vc.p
	if fp, foreign := vc.closureProgram(cl); foreign {
		p = fp
	}
	args := make([]core.Value, len(inputs))
	for i, v := range inputs {
		args[len(inputs)-1-i] = v
	}
	if !closureMatchesArgs(&p.Fns[cl.Unit], args) {
		res, err := core.RunResolved(reg, inputs, core.BodyTokens(body))
		return res, err, true
	}
	res, err := vc.applyClosure(reg, cl, deliverArgs(args))
	return res, err, true
}

// hostFnValueUnit hosts the value's unit with the TOKEN seam's root discipline
// and contains a panic inside it as an internal_error, exactly as
// runForeignUnit contains one for the fn-VALUE seam: a corrupted or
// mis-lowered unit must not unwind the enclosing run, whose own recover
// would abort the whole program for one callback the interpreter can still
// run.
func (vc *vmContext) hostFnValueUnit(reg *core.Registry, ref *compiler.CompiledFnRef, args []core.Value) (res []core.Value, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			res, err = nil, vmInternalError(rec, vc.r.Source)
		}
	}()
	prev := vc.rootRetTrim
	vc.rootRetTrim = false
	defer func() { vc.rootRetTrim = prev }()
	return vc.hostForeign(ref.Prog, reg, ref.Unit, args, ref.Captures, false)
}

// deliverArgs applies the interpreter's binding rule to the matched args
// before they fill the unit's param slots: the ascribed view is stripped (the
// match consumed it) and a list param is quoted so body references are data —
// the same discipline OpCallUserPoly and dynApplyEnter apply.
func deliverArgs(args []core.Value) []core.Value {
	out := make([]core.Value, len(args))
	for i, a := range args {
		a = core.StripAscribed(a)
		if a.Parent.Equal(core.TList) && !a.Quoted {
			a.Quoted = true
		}
		out[i] = a
	}
	return out
}

// checkFnValueReturn enforces a fn VALUE's own contract over the residual its
// hosted unit produced, with the TOKEN seam's discipline: the count first,
// spending the unnamed-param allowance from the bottom (an unnamed-param fn
// leaves its untouched input at the frame bottom, and __RC discards up to
// that many before counting), then the declared types — checkClosureReturn's
// non-trimming branch, over the value's sig instead of a closure payload. A
// value declaring no returns passes its residual through untouched.
func checkFnValueReturn(r *core.Registry, fd core.FnDefInfo, sig *core.Signature, res []core.Value, nUnnamed int, at core.SrcPos) ([]core.Value, error, bool) {
	if len(sig.Returns) == 0 {
		return res, nil, true
	}
	fn := &compiler.CompiledFn{Name: fd.Name, Returns: sig.Returns, ReturnPatterns: sig.ReturnPatterns, Decl: sig.Decl, NUnnamed: nUnnamed}
	if extra := len(res) - len(sig.Returns); extra > nUnnamed {
		return res, vmReturnCountErr(r, fn, len(sig.Returns), len(res)-nUnnamed, res[nUnnamed:], at), true
	}
	out, err := checkReturnContract(r, fn, res, 0, false, at)
	return out, err, true
}

// pushRootArgs is the DynEnv args bracket for the ROOT frame of a fn VALUE's
// unit — the frame the interpreter's dispatch would have opened for the
// value, whose `args` is the value's own call args. pushFrameArgs brackets
// the CALL_USER frames a program opens for itself; a unit entered from a
// seam as a fn value (this arm, RunUnit, runUnitNested) is a frame the
// program never opened, so its args ride in from the seam. No-op outside a
// DynEnv program, where nothing reads the list; the returned func restores
// the depth (an error unwind included — callers defer or call it on every
// path).
func pushRootArgs(reg *core.Registry, p *compiler.Program, args []core.Value) func() {
	if p == nil || !p.DynEnv {
		return func() {}
	}
	floor := reg.Args.Depth()
	_ = reg.Args.Push(core.NewList(append([]core.Value(nil), args...)))
	return func() { reg.Args.Truncate(floor) }
}
