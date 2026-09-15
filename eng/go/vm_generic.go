package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The generic lane's first ROUTED dispatch — OpDispatchGeneric
// (design/FULL-COMPILATION.0.md §6.2, §6.5; Stage 4). The oracle
// (region_oracle.go) walked every descriptor and CHANGED NOTHING; this
// executes one. The shape is the recorder's routeRegion (compiler,
// region_route.go): a user-fn dispatch inside a fn unit whose claim carries
// a live word slot over a span the descriptor host can drive.
//
// The window is laid out as the interpreter's tape at the moment stepWord
// reaches the word: the frame's resolved stack values below, the word at
// `pointer`, the forward tokens after it — a claimed value slot presenting
// the operand the lowering pushed (the index rule: slot i is signature
// position i over the leading claimed positions, and position i is
// stack[top-i]), a live word slot its token, a slot beyond the claim its
// token. Then the kernel's own two routines run over it, exactly as the
// interpreter runs them: CollectForward (the phase-1 plan walk, over the
// descriptor host, which declines every evaluation) and PlanMatch (the
// plan-level matcher, seated on the seam in the sixty-fourth increment so
// both hosts read one implementation). What comes back is the interpreter's
// own plan — the signature, the positions, the speculative slot — and the
// dispatch proceeds from the plan as the interpreter's arrival would:
// forward positions resolve their tokens (a word to its live binding), stack
// positions take the frame's values, and the matched signature runs.
//
// What the first slice ANSWERS: the committed unit (the CALL_USER target the
// check pass chose) when the live match is a signature of that shape, and a
// native handler when the live binding is one. Every other outcome is a
// DESIGNED DEFER (vmDefer, the interp-entry census's choke point): a lead no
// binding names, a walk the host cannot drive, no match (the interpreter's
// rich signature_error is built from its tape), a speculative slot (the
// strict-barrier strand, likewise), a matched boru signature the program
// holds no unit for. A defer is slow, never wrong — and each is a named
// site the census counts, so the slice's remaining shapes are measured, not
// guessed.
//
// UNIT IDENTITY, stated. The live matched signature is taken to be the
// committed unit's when it is a boru body of the unit's shape — the same
// arity and the same declared parameter types. That is not the
// implementation identity the poly seat compares (UserPolyRef.Impls), and it
// does not need to be here: the route makes the OPERAND side live and
// leaves the TARGET side as it was — a call-target bake (noteBakedCallTarget)
// is still noted for the routed call, so a redefinition of the callee
// between record and run is still the memo's to re-record or the escaping
// latch's to refuse. The implementation identity joins the spec when the
// escaping shapes route.

// dispatchGeneric executes one OpDispatchGeneric. It returns the stack the
// dispatch leaves and, when the live match is the committed unit's, the unit
// to enter with its sig-order args (the run loop pushes the frame exactly as
// OpCallUserPoly does); unit -1 means the dispatch completed on the stack.
func (vc *vmContext) dispatchGeneric(p *compiler.Program, gs *compiler.GenericSpec, stack, locals []core.Value, frameBase int, reg *core.Registry, curDebug []core.SrcPos, pc int) ([]core.Value, int, []core.Value, error) {
	if gs.Region < 0 || gs.Region >= len(p.Regions) {
		return nil, -1, nil, vmErrAt(curDebug, pc, "DISPATCH_GENERIC region index out of range")
	}
	d := &p.Regions[gs.Region]
	if len(stack)-frameBase < d.NFwd || frameBase < 0 {
		return nil, -1, nil, vmErrAt(curDebug, pc, "DISPATCH_GENERIC underflow at "+d.Word)
	}
	if err := vc.gateWord(reg, d.Word); err != nil {
		return nil, -1, nil, err
	}
	fn := reg.Lookup(d.Word)
	if fn == nil {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-unbound", "DISPATCH_GENERIC: no binding for "+d.Word+"; deferring to the interpreter")
	}
	// The window: the frame's resolved values, the word, the forward tokens.
	base := stack[:len(stack)-d.NFwd]
	resolved := base[frameBase:]
	pointer := len(resolved)
	toks := make([]core.Value, 0, pointer+1+len(d.Slots))
	toks = append(toks, resolved...)
	toks = append(toks, core.NewWord(d.Word))
	top := len(stack) - 1
	for i := range d.Slots {
		if i < d.NFwd && d.Slots[i].Source != compiler.SlotWordRef {
			toks = append(toks, stack[top-i])
			continue
		}
		toks = append(toks, d.Slots[i].Token)
	}
	h := newRegionHostOver(reg, toks)
	w := core.WordInfo{Name: d.Word, ArgCount: -1}
	if err := h.Collected(core.CollectForward(h, fn, w, pointer+1)); err != nil {
		if RegionCannotEval(err) {
			return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-declined", "DISPATCH_GENERIC at "+d.Word+": the walk needs an evaluation this host cannot perform; deferring to the interpreter")
		}
		return nil, -1, nil, stampAt(err, curDebug, pc, reg) //covergate:allow every error the plan walk can return through this host is a decline — the host declines each evaluation and each sugar expansion with errRegionCannotEval, and the walk raises nothing of its own; kept as the honest arm for a kernel raise a future host could surface (§compiler)
	}
	sig, positions, specAt := core.PlanMatch(h, h.win, reg, fn, w, resolved, pointer, false, false, false)
	if sig == nil || sig.Fallback {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-no-match", "DISPATCH_GENERIC at "+d.Word+": no live signature matches; deferring to the interpreter for the canonical signature_error")
	}
	if specAt >= 0 {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-speculative", "DISPATCH_GENERIC at "+d.Word+": a claimed slot dispatches at run time (the strict barrier); deferring to the interpreter")
	}
	// The plan's positions, resolved as the arrival loop would resolve them:
	// a forward position's token — a word to its live binding — and a stack
	// position's value. The stack args are the nearest resolved values.
	args := make([]core.Value, len(positions))
	stk := 0
	for i, at := range positions {
		if at > pointer {
			tok := h.win.At(at)
			if wi, err := core.AsWord(tok); err == nil {
				if wi.ForceVal || wi.ForceUsurp {
					return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-word-form", "DISPATCH_GENERIC at "+d.Word+": a modified word in the claim; deferring to the interpreter")
				}
				v, ok := reg.Defs.Top(wi.Name)
				if !ok {
					return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-unbound-slot", "DISPATCH_GENERIC at "+d.Word+": no binding for the slot `"+wi.Name+"`; deferring to the interpreter")
				}
				tok = v
			}
			args[i] = tok
			continue
		}
		args[i] = resolved[at]
		stk++
	}
	out := base[:len(base)-stk]
	if h := sig.DispatchHandler(); h != nil && !isBoruSig(sig) {
		if err := vc.gateModuleCall(reg, sig.ModuleCall); err != nil {
			return nil, -1, nil, err
		}
		for i := range args {
			args[i] = core.StripAscribed(args[i])
		}
		results, err := h(args, reg.Contexts.TopData(), nil, reg)
		if err != nil {
			return nil, -1, nil, stampAt(err, curDebug, pc, reg)
		}
		if len(results) != gs.NOut {
			return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-nout-drift", "DISPATCH_GENERIC at "+d.Word+": the live handler's result count differs from the recorded claim; deferring to the interpreter")
		}
		if err := vc.screenResults(results, "generic result at "+d.Word, curDebug, pc); err != nil {
			return nil, -1, nil, err
		}
		return append(out, results...), -1, nil, nil
	}
	if gs.Unit < 0 || gs.Unit >= len(p.Fns) || !unitMatchesSig(&p.Fns[gs.Unit], sig) {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-foreign-unit", "DISPATCH_GENERIC at "+d.Word+": the live signature is not the committed unit's; deferring to the interpreter")
	}
	return out, gs.Unit, args, nil
}

// isBoruSig reports whether sig runs a boru body — the shape a compiled
// unit implements — as opposed to a Go handler the VM calls directly.
func isBoruSig(sig *core.Signature) bool {
	_, ok := sig.Impl.(*core.BoruImpl)
	return ok
}

// unitMatchesSig reports whether the committed unit implements a boru
// signature of sig's shape: the same arity and the same declared parameter
// types. See the file doc for what this identity is and is not.
func unitMatchesSig(fn *compiler.CompiledFn, sig *core.Signature) bool {
	if !isBoruSig(sig) || sig.TotalArgs() != fn.NArgs || len(fn.Params) < fn.NArgs {
		return false
	}
	for i := 0; i < fn.NArgs; i++ {
		if !core.SigArgType(sig, i).Equal(fn.Params[i]) {
			return false
		}
	}
	return true
}
