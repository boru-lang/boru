package eng

import (
	"strconv"

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
// `pointer` — with the modifiers the tape wrote on it (RegionDesc.Mods: a
// `w/f` lead reads the same forward limit live as it did at the record) —
// the forward tokens after it: a claimed value slot presenting the operand
// the lowering pushed (the index rule: slot i is signature position i over
// the leading claimed positions, and position i is stack[top-i]), a live
// word slot its token, a slot beyond the claim its token. Then the kernel's
// own two routines run over it, exactly as the interpreter runs them:
// CollectForward (the phase-1 plan walk, over the descriptor host, which
// declines every evaluation) and PlanMatch (the plan-level matcher, seated
// on the seam in the sixty-fourth increment so both hosts read one
// implementation). What comes back is the interpreter's own plan — the
// signature, the positions, the speculative slot — and the dispatch
// proceeds from the plan as the interpreter's arrival would: forward
// positions resolve their tokens (a word to its live binding), stack
// positions take the frame's values, and the matched signature runs.
//
// THE CLAIM IS THE RECORD'S. The code after the op was lowered for the
// record's stack effect — NFwd operands pushed for the claim, the record's
// arity consumed, NOut produced — so the live plan must claim exactly the
// record's forward slots and exactly the record's arity (GenericSpec.NArgs).
// A shorter live claim would drop tokens the interpreter leaves to run
// after the call; a longer one would consume tokens whose own lowered code
// then runs them again; a stack half of another size would leave the frame
// at a depth the following code was not lowered for. Each is a defer, and
// the check is made before anything runs (found in review of #460).
//
// What the first slice ANSWERS: the committed unit (the CALL_USER target the
// check pass chose) when the live match is a signature of that shape, and a
// native handler when the live binding is one. Every other outcome is a
// DESIGNED DEFER (vmDefer, the interp-entry census's choke point): a lead no
// binding names, a walk the host cannot drive, a claim of another extent, a
// matched boru signature the program holds no unit for, a full-stack native
// (its handler reads the whole resolved stack, which this op does not
// present),
// a native overload the record did not take (its result count is unknown
// before it runs, and an effect it performs would fence the fallback —
// unless the word is declared pure, or the record was a POLY native
// dispatch whose set is the live table (GenericSpec.LiveSet), when the
// count is checked after, as CALL_NATIVE_POLY checks it). A defer is slow,
// never wrong — and each is a named site the census counts, so the slice's
// remaining shapes are measured, not guessed.
//
// THE DIAGNOSTICS ARE THE INTERPRETER'S (the sixty-sixth increment). A no
// match, a strict-barrier strand and an unbound slot are RAISED from the
// window, not deferred: the interpreter builds each from its tape at the
// word, and the window is that tape (core/go/region_diag.go —
// NoMatchOverWindow, StrandedForwardDiag, UndefinedWordDiag, the seats the
// engine's own sigError / strandedForwardError / undefinedWordError sit on),
// so the error is byte-identical. The defer was slow and never wrong only
// while the interpreter could be re-run; an effect already performed
// fences that re-run, and the user saw the defer's internal error. The one
// tape-only layer a drivable window could owe is the pending-`def` hint on
// an unbound slot, so a routed `def` lead keeps the defer there.
//
// UNIT IDENTITY, stated. The live matched signature is taken to be the
// committed unit's when it is a boru body of the unit's shape — the same
// arity, the same declared parameter types and the same parameter patterns
// (two overloads `[0]` and `[n:Integer]` differ only in the pattern, and
// the unit compiled for one raises the other's argument; found in review
// of #460). That is not the implementation identity the poly seat compares
// (UserPolyRef.Impls), and it does not need to be here: the route makes the
// OPERAND side live and leaves the TARGET side as it was — a call-target
// bake (noteBakedCallTarget) is still noted for the routed call, so a
// redefinition of the callee between record and run is still the memo's to
// re-record or the escaping latch's to refuse, and among one binding's
// overloads a shape names one signature (the first of two identical shapes
// wins the match on both lanes). The implementation identity joins the
// spec when the escaping shapes route; the native seat already carries it
// (GenericSpec.Impl).

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
	// The lead resolves in the registry the record dispatched it in
	// (RegionDesc.Reg — a module's sub-registry for a native reached through
	// its wrapper), as CALL_NATIVE_POLY resolves its word in PolyRef.Reg;
	// the operands and the handler's registry are the running one's.
	fn := dispatchRegistry(d.Reg, reg).Lookup(d.Word)
	if fn == nil {
		if liveLeadWord(p, d.Word) {
			// A fn family a conditional body defines (Program.SpecFnNames)
			// is unbound exactly when the arm did not run: the miss IS the
			// interpreter's undefined_word at the word, raised — a defer
			// would re-run past the arm's effects (the seventieth increment).
			return nil, -1, nil, stampAt(core.UndefinedWordDiag(reg, reg.Source, d.Word, d.Pos), curDebug, pc, reg)
		}
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
	if d.Mods != nil {
		w = *d.Mods
	}
	if err := h.Collected(core.CollectForward(h, fn, w, pointer+1)); err != nil {
		if RegionCannotEval(err) {
			return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-declined", "DISPATCH_GENERIC at "+d.Word+": the walk needs an evaluation this host cannot perform; deferring to the interpreter")
		}
		return nil, -1, nil, stampAt(err, curDebug, pc, reg) //covergate:allow every error the plan walk can return through this host is a decline — the host declines each evaluation and each sugar expansion with errRegionCannotEval, and the walk raises nothing of its own; kept as the honest arm for a kernel raise a future host could surface (§compiler)
	}
	sig, positions, specAt := core.PlanMatch(h, h.win, reg, fn, w, resolved, pointer, false, false, false)
	if sig == nil || sig.Fallback {
		// The interpreter's sigError over the window (the sixty-sixth
		// increment): the same failing tuple, the same reorder probe, the
		// shared builder — raised here, byte for byte, where a defer would
		// hand the program back and an effect already performed would fence
		// the hand-back into an internal error.
		return nil, -1, nil, stampAt(core.NoMatchOverWindow(reg.Source, h.win, pointer, d.Word, fn, d.Pos), curDebug, pc, reg)
	}
	nf := 0
	for _, at := range positions {
		if at > pointer {
			nf++
		}
	}
	if specAt >= 0 {
		// The strict barrier, as the interpreter raises it when the barrier
		// word arrives: the routed word still waiting for the forward
		// positions the plan claimed from the barrier on. The speculative
		// slot is a word (PlanMatch marks a slot speculative only for a
		// word bound to a dispatching definition) at the written position
		// the sig-order index names (the index rule).
		if specAt < len(d.Slots) {
			if wi, err := core.AsWord(d.Slots[specAt].Token); err == nil {
				return nil, -1, nil, stampAt(core.StrandedForwardDiag(reg.Source, d.Word, nf-specAt, wi.Name, core.BarrierReceiverWord(reg, wi.Name), d.Pos), curDebug, pc, reg)
			}
		}
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-speculative", "DISPATCH_GENERIC at "+d.Word+": a claimed slot dispatches at run time (the strict barrier); deferring to the interpreter") //covergate:allow PlanMatch marks a slot speculative only for a WORD token of the window, and the window's forward tokens are the descriptor's slots, so the arms above always answer; kept as the honest defer for a kernel change (§compiler)
	}
	if nf != d.NFwd || len(positions) != gs.NArgs {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-claim-drift", "DISPATCH_GENERIC at "+d.Word+": the live plan claims "+strconv.Itoa(nf)+" forward of "+strconv.Itoa(len(positions))+" where the record claimed "+strconv.Itoa(d.NFwd)+" of "+strconv.Itoa(gs.NArgs)+"; deferring to the interpreter")
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
					// The interpreter's undefined_word at the token (the
					// sixty-sixth increment). Its one tape-only hint the window
					// could carry is the pending-`def` hint, which fires when
					// the collecting word is `def` itself — that lead keeps the
					// defer.
					if d.Word == "def" {
						return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-unbound-slot", "DISPATCH_GENERIC at "+d.Word+": no binding for the slot `"+wi.Name+"`; deferring to the interpreter")
					}
					return nil, -1, nil, stampAt(core.UndefinedWordDiag(reg, reg.Source, wi.Name, tok.Pos()), curDebug, pc, reg)
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
		if sig.FullStack() {
			return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-full-stack", "DISPATCH_GENERIC at "+d.Word+": the live signature reads the full stack; deferring to the interpreter")
		}
		if !gs.LiveSet && (gs.Impl == nil || sig.Impl != gs.Impl) && !pureNative(sig) {
			return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-foreign-native", "DISPATCH_GENERIC at "+d.Word+": a native overload the record did not take; deferring to the interpreter before it runs")
		}
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
	if liveLeadWord(p, d.Word) {
		// A speculative fn family's live binding is the outer overload or
		// the arm's shadow — the same shape, a different body — so the unit
		// is the LIVE signature's own, located by its declaration site (the
		// seventieth increment): an identity the shape rule below cannot
		// give. None compiled for the signature leaves the foreign-unit
		// defer.
		if u := specFnUnit(p, sig); u >= 0 {
			return out, u, args, nil
		}
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-foreign-unit", "DISPATCH_GENERIC at "+d.Word+": the live signature has no unit; deferring to the interpreter")
	}
	if gs.Unit < 0 || gs.Unit >= len(p.Fns) || !unitMatchesSig(&p.Fns[gs.Unit], sig) {
		return nil, -1, nil, vmDefer(reg, curDebug, pc, "vm:generic-foreign-unit", "DISPATCH_GENERIC at "+d.Word+": the live signature is not the committed unit's; deferring to the interpreter")
	}
	return out, gs.Unit, args, nil
}

// liveLeadWord reports whether a routed dispatch of word resolves its lead
// live: a speculative fn family's (Program.SpecFnNames) or a stored
// handler's dep (Program.LiveLeadNames — the seventy-first increment).
func liveLeadWord(p *compiler.Program, word string) bool {
	return p.SpecFnNames[word] || p.LiveLeadNames[word]
}

// specFnUnit locates the unit compiled for sig itself — the one stamped
// with sig's declaration site (CompiledFn.Decl: the output-sig token of
// the triple plus the declaring source and file, which is unique per
// signature, across files, and present for an empty body) — or -1 for a
// signature no pass compiled, or a Go-registered one (no site).
func specFnUnit(p *compiler.Program, sig *core.Signature) int {
	if sig.Decl == (core.DeclSite{}) {
		return -1
	}
	for i := range p.Fns {
		if p.Fns[i].Decl == sig.Decl {
			return i
		}
	}
	return -1
}

// isBoruSig reports whether sig runs a boru body — the shape a compiled
// unit implements — as opposed to a Go handler the VM calls directly.
func isBoruSig(sig *core.Signature) bool {
	_, ok := sig.Impl.(*core.BoruImpl)
	return ok
}

// pureNative reports whether a native signature is declared PURE for the
// recorder (CompileEffect): a word with no side effect, whose handler may
// run before its result count is known because a count drift then defers
// with nothing for the effect fence to block.
func pureNative(sig *core.Signature) bool {
	return sig.CompileEffect&(core.CompileIslandPure|core.CompileModuleFold) != 0
}

// unitMatchesSig reports whether the committed unit implements a boru
// signature of sig's shape: the same arity, the same declared parameter
// types and the same parameter patterns. See the file doc for what this
// identity is and is not.
func unitMatchesSig(fn *compiler.CompiledFn, sig *core.Signature) bool {
	if !isBoruSig(sig) || sig.TotalArgs() != fn.NArgs || len(fn.Params) < fn.NArgs {
		return false
	}
	for i := 0; i < fn.NArgs; i++ {
		if !core.SigArgType(sig, i).Equal(fn.Params[i]) || !samePattern(fn, sig, i) {
			return false
		}
	}
	return true
}

// samePattern reports whether the unit's parameter pattern at i (nil for
// none) is the signature's: both absent, or both present and equal.
func samePattern(fn *compiler.CompiledFn, sig *core.Signature, i int) bool {
	var up *core.Value
	if i < len(fn.ParamPatterns) {
		up = fn.ParamPatterns[i]
	}
	sp, ok := core.SigPattern(sig, i)
	if up == nil || !ok {
		return up == nil && !ok
	}
	return core.ExactEqual(*up, sp)
}
