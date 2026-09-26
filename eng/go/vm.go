package eng

// The bytecode VM — the execution half of Stages 1–3 of
// design/legacy/boru-bytecode-plan.0.ignore: straight-line natives, control flow
// (JMP / JMP_IF_FALSE / FOR_SETUP / FOR_NEXT), and user-fn frames
// with CALL_USER / TAIL_CALL_USER / RET.
//
// Termination and resource parity (plan R6 #27): the only back-edge
// the emitter produces is a counted loop's trailing JMP to its
// FOR_NEXT, and the VM enforces exactly that shape — so every loop
// is bounded by its popped count, and an emitted body nets one value
// per iteration. Unbounded value accumulation (`for huge [i]`) hits
// the same growth ceiling the tape enforces: the VM stack's ceiling
// is computed from the registry's TapeConfig and overflowing it
// raises the interpreter's tape_exhausted taxonomy; frame depth
// shares the same ceiling. A runaway that consumes neither (a
// tail-call spin) trips the step budget with the interpreter's
// evaluation_limit taxonomy.

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// RunProgram executes a compiled Program against a registry and
// returns the residual value stack (bottom → top), matching what the
// interpreter's Run returns for the same source. The step budget is
// the interpreter's DefaultStepLimit: a runaway that never grows the
// stack (a tail-recursive spin — frames are REPLACED, so neither
// ceiling trips) fails with the same evaluation_limit taxonomy the
// interpreter raises, instead of hanging.
//
// Concurrency: a *Registry must not be driven by two executions at once —
// and that means ANY pairing, not just two compiled runs. For its duration
// this run installs/restores r.Invoker (the body-closure seam) and mutates the
// registry's scopes, so a concurrent RunProgram OR a concurrent interpreter
// Run on the same registry would race on r.Invoker and the shared defs/types.
// Two guards run at entry: the vmRunning CAS rejects an overlapping compiled
// run, and an interpRunActive() check rejects starting a compiled run while an
// interpreter run is in flight (the depth counter Engine.Run maintains). What
// neither can catch is a fresh interpreter Run STARTING on another goroutine
// once this compiled run is already underway — the VM's own islands re-enter
// Engine.Run on this same registry, so a registry-level flag cannot tell a
// legitimate island from a foreign run without goroutine identity. That last
// shape stays the caller's responsibility under the same rule the interpreter
// already follows: give each goroutine its own *Registry. boru's concurrent
// words honour this by forking an isolated registry per branch
// (ForkConcurrent); host callers run each instance on its own registry.
func RunProgram(p *compiler.Program, r *core.Registry) ([]core.Value, error) {
	return runProgram(p, r, core.StepLimitFor(r, core.DefaultStepLimit))
}

// vmLoop is one open counted loop's iteration state. exitPC / nextPC / unit /
// iterBase are read only by a cross-frame flow signal (OpFlowBreak /
// OpFlowContinue): a break/continue raised in a callee unwinds to the nearest
// open loop and resumes there. exitPC is the loop's break target (after the
// back-edge), nextPC its FOR_NEXT (continue target), unit the code unit the
// loop lives in, and iterBase the operand-stack depth at the CURRENT
// iteration's start (so a signal drops exactly the current iteration's partial
// pushes, like the interpreter's mark→move splice).
type vmLoop struct {
	cur, end, step int64
	slot           int
	exitPC, nextPC int
	unit           int
	iterBase       int
}

// vmFrame remembers a caller's resumption point across a CALL_USER: the
// unit and pc to return to, the caller's frame locals, the open-loop
// count so RET cannot leak loop state, and the operand-stack depth at
// call entry (after the callee's params were popped) so RET can verify
// the body left EXACTLY its declared return count.
type vmFrame struct {
	retUnit, retPC int
	locals         []core.Value
	loopBase       int
	stackBase      int
	// retFn overrides the RETURN CONTRACT this frame's RET applies. Set only
	// by the Apply kernel's frame push (vm_dyn_apply.go): the entered unit is
	// a stamped fn-value body that declares no returns of its own, while the fn
	// VALUE applied does — and the island path this replaces applies the
	// value's contract. Nil on every ordinary CALL_USER frame, where the unit
	// IS the contract.
	retFn *compiler.CompiledFn
	// argsBase is the r.Args depth at call entry (DynEnv programs only —
	// the frame pushed its args list; RET / flow unwind truncate back).
	argsBase int
	// dynBase is the vc.dynBinds depth at call entry: RET truncates the
	// registry's dynamic-scope bindings back to it (the interpreter's
	// def-cleanup discipline for the frame's OpBindDynScope installs).
	dynBase int
}

// dynBindEntry records one OpBindDynScope install: the name and the def
// stack's depth for it BEFORE the install, so unwinding truncates back to
// exactly the pre-install state (InstallDef may itself pop an overlapping
// same-scope binding, so a paired Pop would drift — Truncate cannot).
type dynBindEntry struct {
	reg   *core.Registry
	name  string
	depth int
}

// vmContext holds the state SHARED across a program run and every re-entrant
// closure invocation it spawns: the program, the registry, the resource
// ceilings, the running step count (one global budget), and the reused island
// sub-engine / args scratch. Per-run state (operand stack, frame locals,
// frames, open loops, pc) lives in run() so a body closure invoked
// mid-dispatch executes on its own stack without disturbing the caller.
type vmContext struct {
	p         *compiler.Program
	r         *core.Registry
	ceiling   int
	stepLimit int
	steps     int
	// argsFloor is the r.Args depth when the run entered (DynEnv programs
	// only): the tail-call frame swap at activation root truncates to it,
	// and runVMEntry's exit restore truncates to it on EVERY path (error
	// unwind included), so a failed run never leaks args entries.
	argsFloor int
	// gateReg/gateWC/gateMC cache the engine policy's checkers per
	// registry — the VM twins of the interpreter's policyGateWord /
	// policyGateModuleCall consult them at every named / module-export
	// dispatch, and the capability-store walk behind LookupWordChecker /
	// LookupModuleCallChecker is too costly per call on the hot path. A
	// pointer-compare refresh keeps the cache correct across
	// foreign-unit registry switches.
	gateReg *core.Registry
	gateWC  core.WordChecker
	gateMC  core.ModuleCallChecker
	// dynBinds is the live dynamic-scope binding trail (OpBindDynScope),
	// shared across re-entrant closure runs (they nest strictly): frames
	// record their entry depth (vmFrame.dynBase), RET truncates back, and
	// the top-level error path restores everything so a failed run never
	// leaks registry bindings.
	dynBinds []dynBindEntry
	// foreignInvokers lists the module sub-registries this run lazily
	// installed the body-closure invoker on (a foreign unit's handler
	// drives its code bodies through InvokeBody on the MODULE registry —
	// test-describe, each — which needs the VM seam there just as the main
	// registry does). Restored to nil at run end by runProgram's defer.
	foreignInvokers []*core.Registry
	// flowEscapes marks a HOSTED context whose enclosing run resolves a
	// break/continue that finds no loop in this context's own frames: the
	// run-time token body's (vm_token_body.go), whose interpreter twin — the
	// InvokeBody seam's sub-engine — exits cleanly with the registry's
	// FlowCtrl set for the outer run to translate (Engine.exitWithFlowCtrl's
	// sub-engine contract, escapedFlow). flowSignal then returns a
	// flowEscape instead of the loop-less internal error, and the seam
	// hands the signal to the enclosing run's registry.
	flowEscapes bool
	// rootRetTrim marks a re-entrant run entered through the fn-VALUE seam
	// (enterCallbackUnit): its root RET applies the CallBoru return
	// discipline (checkCallBoruContract) rather than __RC's.
	rootRetTrim bool
	// rootRetNamed marks a re-entrant run entered as a NAMED fn call —
	// InvokeCompiledStrict, the module-fn dispatch (NUR191): its root RET
	// applies the frame's own contract (checkReturnContract with the
	// frame's count discipline) exactly as the spliced frame's __RC and the
	// interpreter's CallBoruStrict do. Never set together with rootRetTrim.
	rootRetNamed bool
	// frameDepth counts live VM activations — user-call frames AND re-entrant
	// run() invocations (a closure invoked from a native handler via
	// invokeClosure starts a FRESH run with its own frames slice). The per-run
	// frames slice alone does NOT bound depth that flows through such
	// re-entrant invocations, so the count lives here, shared across them, and
	// the per-instruction guard checks it against the same ceiling the operand
	// stack uses. This keeps deep higher-order recursion failing with the
	// tape_exhausted memory taxonomy (as the interpreter does) instead of
	// growing the Go stack until it overflows — a panic the top-level recover
	// would otherwise mask as internal_error.
	frameDepth int
	// islandEng is a single sub-engine reused across every OpFallback /
	// CALL_DYNAMIC island in this run, with reuseTape set so a hot island in
	// a loop does not allocate a fresh engine+tape per iteration. Reuse is
	// sound ONLY because island runs are never nested or concurrent within a
	// run: the main VM loop is suspended while islandEng.Run executes, and a
	// higher-order body reached from inside an island re-enters via a FRESH
	// New(r) sub-engine (invokeClosure's non-closure branch), never this one.
	// A future change that makes an island re-enter islandEng would corrupt
	// its in-place-reloaded tape — keep island execution non-reentrant.
	islandEng *core.Engine
}

// tapeCoupled reports whether any result value is a tape-coupled token
// (Word/Mark/Move/Forward/OpenParen/Splice) — a value the interpreter would
// re-STEP on the tape rather than treat as data. No compiled-reachable handler
// should produce one (the emitter declines fn-invoking / code-splicing words),
// so every dispatch site that funnels handler/island results back onto the
// operand stack screens for them and fails loudly instead of pushing a token
// as data. The single definition keeps the four call sites in lockstep.
func tapeCoupled(results []core.Value) bool {
	for _, rv := range results {
		if core.IsWord(rv) || core.IsMark(rv) || core.IsMove(rv) || core.IsForward(rv) ||
			core.IsOpenParen(rv) || core.IsSplice(rv) {
			return true
		}
	}
	return false
}

// island returns the run's reused interpreter sub-engine, building it lazily on
// first use. One engine serves every OpFallback / CALL_DYNAMIC island in the
// run, with reuseTape set so a hot island in a loop reloads its tape in place
// rather than allocating a fresh engine+tape per iteration. Reuse is sound only
// because island runs are never nested or concurrent within a run (see
// vmContext.islandEng).
func (vc *vmContext) island() *core.Engine {
	if vc.islandEng == nil {
		vc.islandEng = core.New(vc.r)
		vc.islandEng.SetSource(vc.r.Source)
		vc.islandEng.ReuseTape = true
	}
	return vc.islandEng
}

// islandRun runs an island token window on the given dispatch registry. The
// program registry keeps the vm's cached island engine; a FOREIGN (module
// sub-registry) unit runs its window on ITS OWN registry's pooled sub-engine —
// the interpreter twin: the enclosing body would have run there (CallBoru), so
// a same-registry callee SPLICES into the island tape and a break/continue it
// raises exits cleanly with the registry FlowCtrl flag set (exitWithFlowCtrl's
// sub-engine contract) for the VM to translate (escapedFlow). Islanding a
// foreign unit's window on the program registry instead would push the callee
// through CallBoru's NewTop sub-engine, where the same signal is a hard
// flow_error the interpreter never raises.
func (vc *vmContext) islandRun(reg *core.Registry, tokens []core.Value) ([]core.Value, error) {
	if reg == nil || reg == vc.r {
		// Census seam (core/interp_entry.go): names the island so the
		// engine-entry census can separate compiled-lane re-entry from a
		// sanctioned interpreter run. Engine.Run emits its own entry too.
		vc.r.NoteInterp("vm:island")
		eng := vc.island()
		eng.FlowUnwind = true
		res, err := eng.Run(tokens)
		eng.FlowUnwind = false
		return res, err
	}
	return runIslandResolved(reg, nil, tokens)
}

// screenResults rejects handler / island results that carry a tape-coupled
// token (Word/Mark/Move/Forward/OpenParen/Splice) — a value the interpreter
// would re-STEP rather than treat as data. No compiled-reachable handler should
// produce one (the emitter declines fn-invoking / code-splicing words); reaching
// here is a compiler bug, so it fails loudly with the call site's label instead
// of pushing a token as data. Returns nil when the results are clean.
func (vc *vmContext) screenResults(results []core.Value, label string, debug []core.SrcPos, pc int) error {
	if tapeCoupled(results) {
		return vmErrAt(debug, pc, "tape-coupled "+label)
	}
	return nil
}

func runProgram(p *compiler.Program, r *core.Registry, stepLimit int) (result []core.Value, runErr error) {
	if p == nil {
		return nil, fmt.Errorf("bytecode: nil program")
	}
	// §6.5's rollback, carried by the Program: roll this registry's bindings
	// back to the base the recorder captured before the check pass, so the
	// placed twins replay onto it instead of stacking a second install on
	// the pass's kept one. Only on the registry the base was captured from —
	// a program run elsewhere has nothing of its own to roll back there, and
	// a foreign DefTable must never be installed; a base-less hand-built
	// program restores nothing (the caller owns its registry). And only for
	// a program that RECORDED a transition: the twin table mirrors the
	// pass's bind ledger entry for entry (the langspec gate), so an empty
	// table means the pass moved no binding and the registry already stands
	// at the base — the restore would clone the whole def table for nothing,
	// on every run (the compiled-mode alloc guard runs one program 65 times).
	// A program that PLACED a speculative undef restores too, twins or
	// not: the check pass generalised the binding IN PLACE
	// (core.GeneraliseSpecUndef — no ledger entry, no twin), so on a
	// long-lived registry where the binding predates this request the
	// live entry holds the pass's carrier until the base is put back
	// (review of #464: `def k 5` then `if false [undef k] [] k` answered
	// the carrier for the interpreter's 5).
	if p.ReplayReg == r && (len(p.BindTwins) > 0 || len(p.SpecUndefNames) > 0 || len(p.SpecFnNames) > 0) {
		r.RestoreBindingsForReplay(p.ReplayBase)
	}
	return runVMEntry(p, r, stepLimit, func(vc *vmContext) ([]core.Value, error) {
		// The top-level program runs at unit -1 (its own Code), with the
		// program's declared locals and an operand stack pre-sized to the
		// program's static ceiling.
		return vc.run(-1, make([]core.Value, p.NumLocals), make([]core.Value, 0, p.MaxStack))
	})
}

// RunUnit starts a FRESH top-level VM run on r, entered at ref.Prog.Fns[ref.Unit],
// with args bound to the unit's leading param slots and ref.Captures bound to the
// trailing slots. It is the DURABLE-callback twin of vmContext.invokeClosureOn
// (which requires a live vmContext): a callback invoked AFTER the enclosing
// RunProgram has returned — a serve-raw connection handler on its per-connection
// fork, a spawned process — runs its compiled body here. Each such fork starts
// with vmRunning==0 (ForkConcurrent), so concurrent callbacks each drive an
// isolated run and the guard in runVMEntry never rejects them.
func RunUnit(ref *compiler.CompiledFnRef, r *core.Registry, args []core.Value) ([]core.Value, error) {
	if ref == nil || ref.Prog == nil {
		return nil, fmt.Errorf("bytecode: nil unit reference")
	}
	if ref.Unit < 0 || ref.Unit >= len(ref.Prog.Fns) {
		return nil, fmt.Errorf("bytecode: unit index %d out of range", ref.Unit)
	}
	return runUnit(ref, r, args, false)
}

// runUnit is RunUnit with the entry's discipline: named is a NAMED fn call
// (InvokeCompiledStrict), whose root RET takes the frame's return contract
// rather than the fn-value seam's trim (NUR191).
func runUnit(ref *compiler.CompiledFnRef, r *core.Registry, args []core.Value, named bool) ([]core.Value, error) {
	return runVMEntry(ref.Prog, r, core.StepLimitFor(r, core.DefaultStepLimit), func(vc *vmContext) ([]core.Value, error) {
		return vc.enterCallbackUnit(r, ref.Unit, bindUnitLocals(r, &ref.Prog.Fns[ref.Unit], args, ref.Captures), named)
	})
}

// enterCallbackUnit is enterBodyUnit for the fn-VALUE seam (RunUnit and
// runUnitNested — InvokeCallback's compiled path): the unit's root RET takes
// the CallBoru return discipline (rootRetTrim → checkCallBoruContract)
// instead of __RC's — or, for a NAMED fn call (named: InvokeCompiledStrict,
// the module-fn dispatch), the frame's own contract (rootRetNamed, NUR191).
// The flags are scoped to this entry: a closure the body invokes through the
// TOKEN seam (invokeClosureOn) enters with them cleared.
func (vc *vmContext) enterCallbackUnit(reg *core.Registry, unit int, locals []core.Value, named bool) ([]core.Value, error) {
	prev, prevNamed := vc.rootRetTrim, vc.rootRetNamed
	vc.rootRetTrim, vc.rootRetNamed = !named, named
	defer func() { vc.rootRetTrim, vc.rootRetNamed = prev, prevNamed }()
	// The value's own frame: its call args are the leading locals (inputs
	// fill the leading param slots, captures the trailing ones).
	fn := &vc.p.Fns[unit]
	defer pushRootArgs(reg, vc.p, locals[:fn.NParams-fn.NCaptures])()
	return vc.enterBodyUnit(reg, unit, locals)
}

// enterBodyUnit is the VM's SINGLE re-entrant body-entry point: every path
// that begins executing a compiled unit as a nested BODY goes through here.
//
// It exists to give the VM the one seam the interpreter has for free. The
// interpreter runs a nested body exactly one way — spawn a sub-engine — so
// `Engine.Run` is a single site where per-body state can be established, and
// its context frame lives there (engine.go, the Contexts Push/Pop pair). The
// VM has no such natural chokepoint: it reaches a body four different ways,
// and design/verse-report-defects-investigation.0.md §B records what that
// cost — a `context set` inside a compiled `do`/`each` body escapes into the
// parent scope, because a patch that bracketed one of the four paths looked
// complete and was not.
//
// The four paths, and where each stands:
//
//  1. RunUnit          — a durable callback after RunProgram returned  → HERE
//  2. runUnitNested    — a mid-run nested unit invoke                  → HERE
//  3. invokeClosureOn  — the InvokeBody closure seam                   → HERE
//  4. OpCallUser / OpCallUserPoly / OpTailCallUser                     → not here
//  5. inlining into the caller's unit                                  → unreachable
//
// (4) is a frame push INSIDE the run loop rather than a re-entry, so it has no
// call to funnel; its entry is the `frames = append(...)` / `frameDepth++`
// pair and its exit is the matching RET. (5) — the `case` desugaring to a
// nested-`if` chain, `otherwise`'s list argument, and list auto-evaluation —
// emits the body's tokens straight into the caller's unit, so there is no
// call at all and no seam function can ever cover it. Anything claiming to
// bracket "every body" has to say something about both.
//
// This function deliberately does NOT change behaviour: it is the place a
// later per-body concern can be added once, not the addition itself.
// TestVMBodyEntryIsFunnelled keeps the funnel from re-fragmenting.
func (vc *vmContext) enterBodyUnit(reg *core.Registry, unit int, locals []core.Value) ([]core.Value, error) {
	if reg != nil {
		reg.Contexts.Push(reg.Contexts.Top())
		defer reg.Contexts.Pop()
	}
	return vc.run(unit, locals, nil)
}

// bindUnitLocals builds a compiled unit's frame locals: per-call args fill the
// leading param slots (0..NParams-NCaptures-1) and captures the trailing ones —
// the top-first sig-order split every enterBodyUnit caller shares. Args past
// the param count are ignored, matching the interpreter's CallBoru binding.
func bindUnitLocals(r *core.Registry, fn *compiler.CompiledFn, args, captures []core.Value) []core.Value {
	locals := make([]core.Value, fn.NLocals)
	nInputs := fn.NParams - len(captures)
	for i := 0; i < len(args) && i < nInputs; i++ {
		locals[i] = args[i]
	}
	for i, cv := range captures {
		if slot := nInputs + i; slot < len(locals) {
			locals[slot] = cv
		}
	}
	nameFrameFns(r, fn, locals)
	return locals
}

// nameFrameFns names the fn VALUES a frame binds, as the interpreter's frame
// binding does (installDef: `fnDef.Name = name` for a Function-family body):
// a fn passed for a named param `g`, or captured as `g`, renders and
// no-matches as `fn g(…)` / `cannot call `g“ on that lane. The VM bound the
// caller's value verbatim, so `(f (z:Integer => [z])) 3` rendered `fn
// (Integer) 3` for the interpreter's `fn g(Integer) 3` (NUR122's class, the
// twenty-sixth increment). Only an FnDefInfo AT HOME in r is renamed — the
// payload the interpreter's rule names; a module wrapper (a foreign home)
// takes installDef's rebinding path, which this does not mirror, and a
// compiled closure keeps its render.
func nameFrameFns(r *core.Registry, fn *compiler.CompiledFn, locals []core.Value) {
	for i := 0; i < fn.NParams && i < len(locals) && i < len(fn.LocalNames); i++ {
		name := fn.LocalNames[i]
		if name == "" {
			continue
		}
		if _, isClosure := locals[i].Data.(core.ClosurePayload); isClosure {
			// A compiled closure bound for a named param takes the name the
			// same way (the thirtieth increment): `(w (kk 7)) 4` rendered
			// `fn (Any) 4` for the interpreter's `fn g(Any) 4`.
			locals[i] = nameClosureValue(locals[i], name)
			continue
		}
		fd, ok := locals[i].Data.(core.FnDefInfo)
		if !ok || fd.Name == name {
			continue
		}
		if core.FnHomeForeign(r, &fd) {
			// A FOREIGN trivial-delegation wrapper (a module export) bound
			// for a named param is the inner native's overloads under the
			// param's name — installDef's own rebinding, which the payload
			// rename could not mirror (NUR123): `(f MathUtil.sqrt/v) 16.0`
			// rendered `fn sqrt(Number)` for the interpreter's `fn
			// g(BigDecimal) or … 16.0`. Any other foreign value is left alone.
			if v, rebound := core.WrapperUnderName(r, name, fd); rebound {
				locals[i] = v
			}
			continue
		}
		fd.Name = name
		locals[i].Data = fd
	}
}

// runVMEntry is the shared guarded prologue for every fresh VM run: it takes the
// concurrency guard, installs the top-level panic recover, wires the body-closure
// invoker, then calls enter to begin execution at the caller's chosen unit. The
// top-level RunProgram enters unit -1; RunUnit enters a specific fn unit with its
// frame locals pre-bound.
func runVMEntry(p *compiler.Program, r *core.Registry, stepLimit int, enter func(*vmContext) ([]core.Value, error)) (result []core.Value, runErr error) {
	// Concurrency guard: a single registry cannot drive two OVERLAPPING runs —
	// the shared Invoker install/restore below (and the mutable scopes the run
	// touches) would race. Catch the misuse with a clear error instead of
	// silent data corruption; concurrent runs must each own a registry
	// (ForkConcurrent). Nested SEQUENTIAL reuse is unaffected: the flag resets
	// on exit before the next run begins, which is the normal RunCompiled path.
	if r != nil {
		if !atomic.CompareAndSwapInt32(&r.VmRunning, 0, 1) {
			return nil, core.MakeBoruError("concurrency_error",
				"bytecode: a compiled program is already running on this registry; concurrent runs need their own registry (ForkConcurrent)",
				"", "", "")
		}
		defer atomic.StoreInt32(&r.VmRunning, 0)
		// Also reject starting a compiled run while an INTERPRETER run is in
		// flight on this same registry — the cross-engine race the CAS above
		// cannot catch. Safe to check here: no island sub-engine has spawned
		// yet, so a non-zero depth means a DISTINCT interpreter run (this run's
		// own islands increment the depth only later).
		if r.InterpRunActive() {
			// The deferred StoreInt32 above releases vmRunning on this return.
			return nil, core.MakeBoruError("concurrency_error",
				"bytecode: an interpreter run is already active on this registry; concurrent runs need their own registry (ForkConcurrent)",
				"", "", "")
		}
	}
	// Last-resort panic guard, mirroring the interpreter's top-level recover
	// (engine.go Run): a bug in a compiled-reachable handler or in the VM loop
	// must surface as a clean internal_error BoruError — which RunCompiled then
	// reports as a compiler defect — never as a goroutine stack
	// trace. Errors returned normally are untouched.
	defer func() {
		if rec := recover(); rec != nil {
			src := ""
			if r != nil {
				src = r.Source
			}
			result = nil
			runErr = vmInternalError(rec, src)
		}
	}()
	vc := &vmContext{p: p, r: r, ceiling: vmStackCeiling(r), stepLimit: stepLimit, argsFloor: r.Args.Depth()}
	if p != nil && p.DynEnv {
		// DynEnv exit restore: the args bracket pushes per CALL_USER frame and
		// RET/flow truncate back, but an ERROR unwind returns straight out of
		// vc.run — rebalance to the entry depth on every path so a failed run
		// never leaks args entries (the dynBinds discipline, applied to args).
		defer r.Args.Truncate(vc.argsFloor)
	}
	// Install the body-closure invoker so a higher-order word's handler runs
	// its body through the VM (InvokeBody → r.Invoker → invokeClosure). The
	// shared registry means the island sub-engine inherits it too, so the
	// invoker dispatches on the body VALUE: a compiled closure runs in the
	// VM, a raw token-list body (an island's interpreter run reaching a
	// handler) runs through a sub-engine — identical to InvokeBody's nil
	// branch. Restored on exit so nested runs nest cleanly. nestedRunner rides
	// alongside it for the live-run callback path (InvokeCallback).
	prevInvoker := r.Invoker
	prevNested := r.NestedRunner
	r.Invoker = vc.invokeClosureOn
	r.NestedRunner = vc.runUnitNested
	defer func() {
		r.Invoker = prevInvoker
		r.NestedRunner = prevNested
		for _, fr := range vc.foreignInvokers {
			fr.Invoker = nil
		}
	}()
	return enter(vc)
}

// runUnitNested runs a stamped fn unit as a nested activation of THIS VM run —
// the live-run twin of RunUnit, used when a callback (a service handler) is
// invoked synchronously during the run (vmRunning is already 1, so a fresh
// RunUnit would be rejected by the concurrency guard). It re-enters vc.run on a
// fresh operand stack exactly as invokeClosureOn does for a compiled closure, so
// the outer run resumes cleanly when it returns.
//
// Two cases, and the second is the one that used to be missing. A COMPILE-TIME
// stamp baked by the running program shares vc.p, so its unit index is an index
// into the table vc already holds and the body enters directly. A DETACHED
// stamp (StampDetachedFn — a predicate type, a runtime-stamped codec, a service
// handler) carries its own standalone Program, so vc.p is the wrong table for
// it; runForeignUnit hosts it in a nested vmContext bound to ITS program
// instead. This seam previously declined that case outright, which meant every
// runtime-stamped body reached mid-run reported Stamped:true and then ran on
// the interpreter — see vm_foreign_unit.go for what the decline cost.
//
// handled=false is reserved for a ref this seam genuinely cannot run: a non-ref
// payload, or a unit index outside its own program's table (a compile/run
// drift). InvokeCallback then falls back to the interpreter, unchanged.
func (vc *vmContext) runUnitNested(h any, args []core.Value) ([]core.Value, bool, error) {
	var ref *compiler.CompiledFnRef
	named := false
	switch v := h.(type) {
	case namedUnitRef:
		ref, named = v.ref, true
	case *compiler.CompiledFnRef:
		ref = v
	}
	if ref == nil || ref.Prog == nil || ref.Unit < 0 || ref.Unit >= len(ref.Prog.Fns) {
		return nil, false, nil
	}
	if ref.Prog != vc.p {
		return vc.runForeignUnit(ref, args, named)
	}
	res, err := vc.enterCallbackUnit(vc.r, ref.Unit, bindUnitLocals(vc.r, &vc.p.Fns[ref.Unit], args, ref.Captures), named)
	return res, true, err
}

// invokeClosure runs a code body for the InvokeBody seam. A compiled closure
// (OpPushClosure's value) executes in the VM's re-entrant runner: its inputs
// bind to the body unit's leading param slots and its captures to the trailing
// slots, then the unit runs on a fresh operand stack. Any other body value (a
// raw token list — an island's interpreter run reaching a higher-order
// handler) runs through a sub-engine exactly as InvokeBody does with no
// Invoker, so the island path is unchanged.
func (vc *vmContext) invokeClosure(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error) {
	return vc.invokeClosureOn(reg, body, inputs)
}

// invokeClosurePositional runs a closure over args in POSITIONAL order —
// args[0] the first param — the order the fn-value-call ops build from
// their stack window (callDynamic's [fn, args…] read up, callDynTrailTop's
// and callDynApply's window read top-down). The token seam above takes the
// STACK order a native hands it and matches a fn-VALUE closure top-down
// itself (S1b-2, invokeFnValueClosure), so a positional window handed to
// it was reversed TWICE, and a two-param closure bound its first param to
// the value farthest from it: `(2 3 (mk2 1))` compiled 24 for the
// interpreter's 33, and `7 5 (mk2 1)/v apply` likewise (NUR179). A closure
// that is not a fn value keeps the seam's positional binding (applyClosure
// through shapeInputs) and is handed the args as they are.
func (vc *vmContext) invokeClosurePositional(reg *core.Registry, fnVal core.Value, args []core.Value) ([]core.Value, error) {
	if !compiler.ClosureIsFnValue(fnVal) {
		return vc.invokeClosure(reg, fnVal, args)
	}
	rev := make([]core.Value, len(args))
	for i, v := range args {
		rev[len(args)-1-i] = v
	}
	return vc.invokeClosure(reg, fnVal, rev)
}

// applyNativeFnValueTopDown applies a self-contained Go-implemented fn value
// (or a parked native) at the TOKEN seam over the inputs a native handed it
// in STACK order, the way the interpreter steps the value there: each own
// signature's arity names how many inputs it takes from the top, those
// bind top-down (the top input → the first param, NUR179's rule, the
// window reversed into positional order), and the inputs beneath stay as
// the body's residual — `1 fold h/v [1 2]` over a one-param wrapper applies
// h to the element and leaves the accumulator, and fold reads the top.
// Arities are tried widest first, so an overloaded wrapper takes as much as
// it can, and the first matching one applies (tryNativeFnApply); with none
// matching the caller's interpreter fallback steps the value as before.
func (vc *vmContext) applyNativeFnValueTopDown(body core.Value, fd core.FnDefInfo, inputs []core.Value) ([]core.Value, bool, error) {
	for k := len(inputs); k >= 0; k-- {
		takes := false
		for i := range fd.Signatures {
			if !fd.Signatures[i].Fallback && fd.Signatures[i].TotalArgs() == k {
				takes = true
				break
			}
		}
		if !takes {
			continue
		}
		args := make([]core.Value, k)
		for i := 0; i < k; i++ {
			args[i] = inputs[len(inputs)-1-i]
		}
		res, done, err := vc.tryNativeFnApply(body, args)
		if !done {
			continue
		}
		if err != nil {
			return nil, true, err
		}
		out := append(append([]core.Value(nil), inputs[:len(inputs)-k]...), res...)
		return out, true, nil
	}
	return nil, false, nil
}

// invokeClosureOn runs a code body for the InvokeBody seam against the
// CALLING registry: the registry the handler dispatched on (the main
// registry, a module sub-registry, or a per-connection fork that inherited
// the invoker), so a raw token body's sub-engine fallback resolves names
// exactly as the interpreter's dispatch would.
func (vc *vmContext) invokeClosureOn(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error) {
	cl, ok := body.Data.(core.ClosurePayload)
	if !ok {
		// A fn VALUE handed to the seam runs its unit natively when it has
		// one (vm_fnvalue_seam.go, S1b); otherwise — a token body, a value
		// with no matching sig or no unit — pooled + resolved inputs,
		// mirroring InvokeBody's no-Invoker branch (never the island engine —
		// see vmContext.islandEng's non-reentrancy contract).
		if res, err, ran := vc.invokeFnValue(reg, body, inputs); ran {
			return res, err
		}
		// A SELF-CONTAINED Go-implemented fn value (fn-util's produced
		// wrappers) or a parked native: apply it on its own signatures here,
		// as the quotation body's trailing apply does (callDynTrailTop →
		// tryNativeFnApply), rather than stepping it on the interpreter per
		// element — `each h/v [1 2 3]` with `def h (FnUtil.compose …)` paid
		// a RunResolved entry per element where `each [h] [1 2 3]` ran
		// natively (the interp-entry census, callbacks.tsv:L154). The seam's
		// inputs are in STACK order and the value's signature binds
		// top-down, so the window is reversed into positional order exactly
		// as invokeClosurePositional does for a closure (NUR179's rule).
		if fd, isFn := body.Data.(core.FnDefInfo); isFn && vmNativeApplicable(vc.r, fd) {
			if res, done, err := vc.applyNativeFnValueTopDown(body, fd, inputs); done {
				return res, err
			}
		}
		// A TOKEN body that exists only at run time is compiled at run time
		// and hosted here (vm_token_body.go, S3's first slice); what it
		// declines still steps below.
		if res, err, ran := vc.invokeTokenBody(reg, body, inputs); ran {
			return res, err
		}
		return core.RunResolved(reg, inputs, core.BodyTokens(body))
	}
	// A fn-VALUE closure — a capturing `fn` / `=>` literal minted at run
	// time — reaching the TOKEN seam with its inputs in stack order is
	// matched against its own signature first, top down, exactly as the
	// interpreter matches the stepped value, and declines to the stepping
	// path when nothing matches (S1b-2; vm_fnvalue_seam.go). A seam that
	// already matched it hands SigMatched args in signature order.
	if !cl.SigMatched {
		if res, err, ran := vc.invokeFnValueClosure(reg, body, cl, inputs); ran {
			return res, err
		}
		if res, err, ran := vc.unmatchedLambdaBody(reg, body, cl, inputs); ran {
			return res, err
		}
	}
	args := shapeInputs(cl, inputs)
	if res, err, ran := vc.closureSourceStep(reg, cl, inputs, args); ran {
		return res, err
	}
	return vc.applyClosure(reg, cl, args)
}

// closureSourceStep hands one invocation of a callback body unit to the
// interpreter when an input lands in a param slot the body reads bare under a
// gradual carrier (CompiledFn.FnReadParams): the interpreter dispatches a fn
// there as a word — `def g fn [[f:Any] [Any] [f]]  each g/v [([] => [1]) 7]`
// is [1 7] — where the unit pushed the slot, [fn f 7] (NUR219). The callback
// fn VALUE rides on the closure (ClosurePayload.Source) with the closure's
// runtime captures in place of the compile-time ones, and it runs the way the
// handler's own interpreter lane runs it: through the fn-VALUE seam (RetTrim)
// matched and called by InvokeCallbackFn, through the TOKEN seam stepped over
// the inputs as InvokeBody's no-Invoker branch steps it. Data inputs run the
// unit.
func (vc *vmContext) closureSourceStep(reg *core.Registry, cl core.ClosurePayload, inputs, args []core.Value) ([]core.Value, error, bool) {
	if cl.Source == nil {
		return nil, nil, false
	}
	fn, _ := vc.closureUnit(cl)
	if !fn.FnReadRefused(args) {
		return nil, nil, false
	}
	src := *cl.Source
	fd, _ := src.Data.(core.FnDefInfo)
	if len(fd.Captured) > 0 {
		bound := append([]core.CapturedBinding(nil), fd.Captured...)
		for i := range bound {
			if i < len(cl.Captures) {
				bound[i].Value = cl.Captures[i]
			}
		}
		fd.Captured = bound
		src.Data = fd
	}
	if sig := core.MatchFnSig(src, inputs); cl.RetTrim && sig != nil {
		res, err := core.InvokeCallbackFn(reg, &fd, sig, inputs)
		return res, err, true
	}
	res, err := core.RunResolved(reg, inputs, []core.Value{src})
	return res, err, true
}

// unmatchedLambdaBody is the token seam's per-element signature match for a
// callback BODY unit compiled from a TYPED LAMBDA at its call site — `each
// ([x:Integer] => [typeof x]) xs`, whose unit is each$body with the lambda's
// own param contract (CompiledFn.Params) but not a fn VALUE unit (NUR155).
// The interpreter never enters such a body blind: its handler hands the fn
// value to InvokeBody, which STEPS the value over the inputs, and a step no
// signature admits leaves the value as DATA on top of them — `each
// ([x:Integer] => [typeof x]) [1 'a']` is `[Integer fn (Integer)]`, the
// element's result the fn itself — while the map arm's lambda no-match
// raises its own signature_error (native_map_iter.go's callLambda). The
// unit ran on every element regardless: `[Integer ProperString]`.
//
// A unit with no contract of its own (a quotation body: Params empty) and
// a fn VALUE unit (invokeFnValueClosure's, above) are not this arm's; the
// match runs over the positional args the bind would seat (shapeInputs), so
// the match and the bind read one order.
func (vc *vmContext) unmatchedLambdaBody(reg *core.Registry, body core.Value, cl core.ClosurePayload, inputs []core.Value) ([]core.Value, error, bool) {
	if body.Quoted {
		return nil, nil, false
	}
	fn, known := vc.closureUnit(cl)
	if !known || len(fn.Params) == 0 || len(fn.Params) != fn.NArgs {
		return nil, nil, false
	}
	if closureMatchesArgs(fn, shapeInputs(cl, inputs)) {
		return nil, nil, false
	}
	if cl.InShape == compiler.ClosureInKeyVal {
		return nil, reg.BoruError("signature_error",
			fmt.Sprintf("no matching lambda signature for %d argument(s)", len(inputs)), ""), true
	}
	// A NAMED value's no-match is the word's raise, not a park: the
	// interpreter steps `h/v` under its name and raises uncalled_function
	// when no signature admits the step's candidates (execFnDefLiteral) —
	// `0 fold h/v [1 2]` over a body that returns a List raised at step 1
	// interpreted and answered `[fn (Integer, Integer)]` compiled, this arm
	// applying the anonymous value's data rule to a named one (NUR211).
	// RetName is the def's name (nameClosureValue / fnValueRetSpec); an
	// anonymous lambda carries none and keeps the data rule below.
	// Anchored where the interpreter anchors it: at the reference's own
	// token (`h/v`, 1:60 in the fold row), which the value carries as
	// RetPos (fnValueRetSpec records where the reference was written).
	if cl.RetName != "" {
		pos := cl.RetPos
		if pos.Row == 0 {
			pos = body.Pos()
		}
		return nil, reg.BoruErrorHintAt("uncalled_function",
			"call to '"+cl.RetName+"' matched no signature", cl.RetName,
			"hint: check the call's argument types and arity — or use "+cl.RetName+"/v to push the function as a value deliberately", pos), true
	}
	// The value renders as the interpreter's own lambda renders — `fn
	// (Integer)` — not as a body unit's payload (nameStoredClosure's rule,
	// vm_dyn_words.go, over the unit's declared contract).
	if cl.Render == "" {
		if params, ok := closureSigParams(fn); ok {
			cl.Render = core.FormatFnDef(core.FnDefInfo{Signatures: []core.Signature{{Params: params, BarrierPos: len(params)}}, Anonymous: true})
			body = core.Value{Parent: body.Parent, Data: cl, Quoted: body.Quoted}
		}
	}
	return append(append([]core.Value(nil), inputs...), body), nil, true
}

// applyClosure runs a closure's unit over args already in the unit's
// positional order (the token seam's shaped inputs, a matched fn-value
// closure's signature-ordered args, a bridge handler's dispatch args), with
// the closure's own return contract applied to the residual.
func (vc *vmContext) applyClosure(reg *core.Registry, cl core.ClosurePayload, args []core.Value) ([]core.Value, error) {
	// A closure minted by ANOTHER program indexes that program's Fns table, so
	// it runs in that program's own nested context rather than against vc.p
	// (see closureProgram for why this became reachable).
	if p, foreign := vc.closureProgram(cl); foreign {
		// The token seam's foreign arm: the hosted root RET takes __RC's
		// discipline, whatever seam the enclosing unit was entered through.
		prev, prevNamed := vc.rootRetTrim, vc.rootRetNamed
		vc.rootRetTrim, vc.rootRetNamed = false, false
		defer func() { vc.rootRetTrim, vc.rootRetNamed = prev, prevNamed }()
		return vc.hostForeign(p, reg, cl.Unit, args, cl.Captures, false)
	}
	// Inputs fill the leading param slots, captures the trailing ones
	// (StartFnCompile registers params before captures) — the same split
	// RunUnit and runUnitNested bind, so it uses the same helper rather than
	// a second copy of the loop.
	// The TOKEN seam's entry: the root RET takes __RC's discipline, whatever
	// seam the enclosing unit was entered through.
	prev, prevNamed := vc.rootRetTrim, vc.rootRetNamed
	vc.rootRetTrim, vc.rootRetNamed = false, false
	// A unit that stands for a fn VALUE's body — a named fn's or a lambda's,
	// compiled at the callback slot with the value's own param contract
	// (CompiledFn.Params; a quotation body carries none and runs in the
	// caller's frame) — is a frame the interpreter's dispatch would have
	// opened for the value, whose `args` is the value's own call args:
	// pushRootArgs brackets it as the fn-value seam brackets its units.
	// Without it the body's `args` read the ENCLOSING frame's list — none
	// at the top level — so `def g fn [[n:Integer] [Any] [do [args]]]
	// each g/v [1 2]` raised `args: not inside a function` per element for
	// the interpreter's `[[1] [2]]` (NUR166).
	if fn := &vc.p.Fns[cl.Unit]; len(fn.Params) > 0 && fn.NArgs > 0 && fn.NArgs <= len(args) {
		defer pushRootArgs(reg, vc.p, args[:fn.NArgs])()
	}
	res, err := vc.enterBodyUnit(reg, cl.Unit, bindUnitLocals(reg, &vc.p.Fns[cl.Unit], args, cl.Captures))
	vc.rootRetTrim, vc.rootRetNamed = prev, prevNamed
	if err != nil {
		return res, err
	}
	return checkClosureReturn(vc.r, cl, res, vc.p.Fns[cl.Unit].NUnnamed)
}

// checkClosureReturn applies a CALLBACK fn value's own declared return to the
// results its closure produced.
//
// The check is on the VALUES, at invoke time, and that is the whole point:
// the unit's RET cannot do it, because a shared closure unit has no single
// contract and giving it one needs a per-fn memo key — which alone makes a
// shared unit recompile and decline on operand provenance, islanding CONFORMING
// callbacks (measured: TestListFoldCallbackOrderPin). Checking produced values
// needs no static provenance, so nothing stops compiling.
//
// What it closes, measured on main and bisected to the callback-seam move:
//
//	def cbad fn [[n:Integer][Boolean][n]] end  [1 2] each cbad/v
//	  interpreted  each: element 0: [boru/type_error]: cbad: return value 1: …
//	  compiled     [1 2]        — the declaration went unenforced
//
// A closure with no contract (a raw token body, or a named fn declaring no
// returns) passes through untouched. An anonymous lambda's Returns=[Any]
// placeholder is a COUNT contract (compiler fnValueRetSpec /
// check.LambdaCountContract): the interpreter's seam raises `expected 1
// return value(s), got 2` for `each (x:Integer => [x 1]) [1 2]`.
//
// The COUNT is enforced here, over the whole residual, because the frameless
// checkReturnContract below only trims the unnamed-input allowance and checks
// the top values' types — it never raised on an over-count, so a 2-value body
// under a 1-return contract answered `[1 1]` (NUR120, measured 2026-09-05).
// nUnnamed is the closure unit's own allowance: an unnamed-param fn value
// (`fn [[Integer][Integer][add 1 0]]`) leaves its untouched input at the
// frame bottom exactly as the interpreter's frame does, and __RC discards up
// to that many extra bottom values before counting.
func checkClosureReturn(r *core.Registry, cl core.ClosurePayload, res []core.Value, nUnnamed int) ([]core.Value, error) {
	if len(cl.RetTypes) == 0 {
		return res, nil
	}
	// The same contract check the frame path runs at RET, over the produced
	// residual: an overlay CompiledFn is how applyRetContract already hands a
	// value's contract to unit-shaped machinery.
	fn := &compiler.CompiledFn{Name: cl.RetName, Returns: cl.RetTypes, ReturnPatterns: cl.RetPatterns, Decl: cl.RetDecl, NUnnamed: nUnnamed}
	// The fn-VALUE seam (InvokeCallbackBody): the interpreter's handler
	// would run this value through CallBoru, whose return discipline is
	// enforceCallBoruReturns — types over the aligned residual, no count.
	if cl.RetTrim {
		return res, checkCallBoruContract(r, fn, res, cl.RetPos)
	}
	if extra := len(res) - len(cl.RetTypes); extra > nUnnamed {
		// Allowance spent from the bottom — report the top values, the same
		// slice the interpreter reports (design/DIAGNOSTIC-VALUES.0.md).
		return res, vmReturnCountErr(r, fn, len(cl.RetTypes), len(res)-nUnnamed, res[nUnnamed:], cl.RetPos)
	}
	return checkReturnContract(r, fn, res, 0, false, cl.RetPos)
}

// pushFrameArgs is the DynEnv args bracket's frame-entry half: push the
// callee's real args (locals[0:nArgs], sig order) as the frame's args list —
// the interpreter's per-call push, so a dynamic code body's runtime sub-run
// reads `args` identically. No-op outside DynEnv programs.
func (vc *vmContext) pushFrameArgs(nl []core.Value, nArgs int) {
	if vc.p == nil || !vc.p.DynEnv {
		return
	}
	_ = vc.r.Args.Push(core.NewList(append([]core.Value(nil), nl[:nArgs]...)))
}

// swapTailArgs is the bracket's TAIL-call form: the frame is replaced, so the
// top args entry swaps for the new callee's, keeping the bracket depth stable.
func (vc *vmContext) swapTailArgs(frames []vmFrame, nl []core.Value, nArgs int) {
	if vc.p == nil || !vc.p.DynEnv {
		return
	}
	if len(frames) > 0 {
		vc.r.Args.Truncate(frames[len(frames)-1].argsBase + 1)
		_, _ = vc.r.Args.Pop()
	} else {
		vc.r.Args.Truncate(vc.argsFloor)
	}
	_ = vc.r.Args.Push(core.NewList(append([]core.Value(nil), nl[:nArgs]...)))
}

// retFrameArgs is the bracket's frame-exit half: truncate to the popped
// frame's entry depth (RET and flow unwind both route here).
func (vc *vmContext) retFrameArgs(f *vmFrame) {
	if vc.p == nil || !vc.p.DynEnv {
		return
	}
	vc.r.Args.Truncate(f.argsBase)
}

// callPoly dispatches a native word by matching the kernel's own
// MatchSignature over the word's signatures against the top Arity stack
// values — the same first-match the interpreter takes — then calls the
// matched handler (plan P3). A no-match raises signature_error, the same
// taxonomy the interpreter's sigError raises.
func (vc *vmContext) callPoly(pr *compiler.PolyRef, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	return vc.callPolyIn(vc.r, pr, stack, curDebug, pc)
}

// callPolyIn is callPoly against an explicit dispatch registry — the active
// unit's (a module fn's natives run in module scope, like the interpreter's
// CallBoru body run).
func (vc *vmContext) callPolyIn(dispReg *core.Registry, pr *compiler.PolyRef, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	// The word-policy gate mirrors the interpreter's per-dispatch check
	// (see gateWord); poly re-match is still one dispatch of pr.Word.
	if err := vc.gateWord(dispReg, pr.Word); err != nil {
		return nil, err
	}
	vc.ensureInvoker(dispReg)
	r := dispReg
	n := pr.Arity
	if len(stack) < n {
		return nil, vmErrAt(curDebug, pc, "CALL_NATIVE_POLY underflow at "+pr.Word)
	}
	// A MODULE poly word (`StructUtil.getpath`) re-matches over its OWN
	// sub-registry's signatures; a core word over the dispatch registry
	// (the active unit's — module scope for a module fn's body).
	fn := dispatchRegistry(pr.Reg, r).Lookup(pr.Word)
	var sigs []core.Signature
	if fn != nil {
		sigs = fn.Signatures
	}
	// Build the args in sig order (position 0 = top of stack, as OpCallNative
	// does), then match: MatchSignature's positionalMatch reads values[i] as
	// sig position i.
	window := make([]core.Value, n)
	for i := 0; i < n; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	mr := core.MatchSignature(sigs, window, core.WordInfo{ArgCount: n})
	if mr == nil || mr.Sig == nil {
		// The recorded count is the check pass's PICK over a gradual
		// residual — `call {} svc  call {} svc`, whose second call matched
		// the three-operand overload with the first call's undeclared
		// result standing in for the third Map — and the run may refute
		// it: the interpreter's matcher takes the overload the live values
		// fit, so the seat tries the word's other arities over the same
		// stack top before it raises (NUR147: a `vm:poly-no-match` defer,
		// `1 1` by whole-program fallback, an internal error forced).
		for k := n - 1; k >= 1; k-- {
			if !polyHasArity(sigs, k) {
				continue
			}
			if alt := core.MatchSignature(sigs, window[:k], core.WordInfo{ArgCount: k}); alt != nil && alt.Sig != nil && alt.Sig.DispatchHandler() != nil {
				mr, n, window = alt, k, window[:k]
				break
			}
		}
	}
	if mr == nil || mr.Sig == nil || mr.Sig.DispatchHandler() == nil {
		// No runtime match. The interpreter's signature_error is built from its
		// live tape / forward-collection state (engine.go sigError) — the
		// written tuple, a reorder hint, two tape-only layers — which the VM
		// alone cannot reproduce. When the record carried a faithfulness plan
		// (PolyNoMatchSpec — the check pass proved, at the failed-dispatch
		// state it recovered from, that the diagnostic is rebuildable from the
		// window), raise the byte-identical signature_error right here (plan
		// 3c). Otherwise bail: the compiled run cannot take it
		// (internal_error → RunCompiled re-runs the interpreter), which raises
		// the canonical error — sound because the interpreter takes the SAME
		// MatchSignature first-match and so reaches the same no-match.
		if mr == nil || mr.Sig == nil {
			if err := vc.polyNoMatchRaise(r, pr, fn, window, curDebug, pc); err != nil {
				return nil, err
			}
		}
		return nil, vmDeferAlt(r, curDebug, pc, "vm:poly-no-match",
			"CALL_NATIVE_POLY no match for "+pr.Word+"; the compiled runtime cannot execute it for the canonical signature_error",
			bestEffortNoMatch(r, fn, pr.Word, window, curDebug, pc))
	}
	// Per-export module policy gate (NUR045): a module poly word's
	// re-match resolved a stamped sub-registry sig — the same identity
	// the interpreter's execMatch gate reads, checked AFTER the match so
	// the gate applies to the overload that actually dispatches.
	if err := vc.gateModuleCall(dispReg, mr.Sig.ModuleCall); err != nil {
		return nil, err
	}
	// StripAscribed at delivery: the re-match above consumed the ascribed
	// view; the handler receives the REAL values (execMatch parity).
	for i := range mr.Args {
		mr.Args[i] = core.StripAscribed(mr.Args[i])
	}
	results, err := mr.Sig.DispatchHandler()(mr.Args, r.Contexts.TopData(), nil, r)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, r)
	}
	// A get/getr surfacing a 0-arg trivial-delegation METHOD (`r.bool`) is NOT
	// auto-applied here: the recorder owns that landing. Every annotated
	// method-shape read either models the interpreter's instant auto-fire as an
	// explicit arity-0 OpCallDynMethod right after this poly (the shaped-method
	// landing model) or DECLINES compilation (tryShapedMethodDispatch's
	// guard-owned decline), so the poly's job is exactly its recorded claim —
	// return the member value. A runtime auto-apply here would double-fire
	// against the following CALL_DYN_METHOD (span-finish underflow,
	// rand-bool's non-fn operand).
	// Enforce the recorder's result-count claim: the runtime re-match may
	// land on an overload whose arity the checker's model did not commit
	// (`set` over a dynamic receiver — Store writes in place and returns
	// nothing, Map/Flex return the container). A mismatched count would
	// silently shift every downstream operand, so defer to the interpreter
	// instead (runtimeShouldFallback). The defer keeps a wrong answer out;
	// it does not make the miss acceptable — the program did not compile,
	// which is a defect owed a fix.
	if len(results) != pr.NOut {
		return nil, vmDefer(r, curDebug, pc, "vm:poly-nout-drift", fmt.Sprintf(
			"poly dispatch %s: result count %d differs from the recorded claim %d; the compiled runtime cannot execute it",
			pr.Word, len(results), pr.NOut))
	}
	if err := vc.screenResults(results, "poly result at "+pr.Word, curDebug, pc); err != nil {
		return nil, err
	}
	return append(stack[:len(stack)-n], results...), nil
}

// matchUserPoly resolves one OpCallUserPoly dispatch: it re-derives the
// recorded same-arity overload subset of pr.Word from the word's LIVE
// dispatch table, verifies each arm's run-implementation identity against the
// recorded Impls (any drift — a re-def between the compile and the run —
// defers to the interpreter rather than running a stale body unit), then runs
// the kernel's own MatchSignature over the subset against the top Arity stack
// values — the SAME first-match the interpreter's dispatch takes. Returns the
// matched arm's compiled unit and the args in sig order (position 0 = top of
// stack, exactly the window OpCallUser binds). A no-match defers to the
// interpreter, which raises the canonical
// signature_error — sound because the interpreter takes the same first-match
// and so reaches the same no-match (mirroring callPoly's no-match path).
func (vc *vmContext) matchUserPoly(pr *compiler.UserPolyRef, stack []core.Value, curDebug []core.SrcPos, pc int) (int, []core.Value, error) {
	if err := vc.gateWord(vc.r, pr.Word); err != nil {
		return 0, nil, err
	}
	n := pr.Arity
	if len(stack) < n {
		return 0, nil, vmErrAt(curDebug, pc, "CALL_USER_POLY underflow at "+pr.Word)
	}
	var subset []core.Signature
	var units []int
	var fd *core.FnDefInfo
	if len(pr.Sigs) > 0 {
		// STORED mode (COMPILE FAILURE-CLOSURE.0 §6b): a body-local fn's binding is
		// popped before the VM runs, so the dispatch table was frozen at
		// record time (see UserPolyRef.Sigs — the freeze's faithfulness
		// gates live there). No live Lookup, no index/Impl drift guard: the
		// frozen table IS the table.
		subset = make([]core.Signature, 0, len(pr.Sigs))
		units = make([]int, 0, len(pr.Sigs))
		for k := range pr.Sigs {
			if k >= len(pr.Units) || pr.Sigs[k].TotalArgs() != n { //covergate:allow compiler/VM defensive arm; the recorder freezes same-arity sigs with parallel units — unreachable without a bytecode-level fault (§compiler)
				return 0, nil, vmErrAt(curDebug, pc, "CALL_USER_POLY stored-sig shape mismatch at "+pr.Word)
			}
			u := pr.Units[k]
			if u < 0 || u >= len(vc.p.Fns) || vc.p.Fns[u].NParams != n { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return 0, nil, vmErrAt(curDebug, pc, "CALL_USER_POLY unit shape mismatch at "+pr.Word)
			}
			subset = append(subset, pr.Sigs[k])
			units = append(units, u)
		}
	} else {
		fd = dispatchRegistry(pr.Reg, vc.r).Lookup(pr.Word)
		if fd == nil {
			return 0, nil, vmDefer(vc.r, curDebug, pc, "vm:user-poly-unresolved", "CALL_USER_POLY unresolved fn "+pr.Word+"; the compiled runtime cannot execute it")
		}
		subset = make([]core.Signature, 0, len(pr.SigIdx))
		units = make([]int, 0, len(pr.SigIdx))
		for k, si := range pr.SigIdx {
			if k >= len(pr.Units) || k >= len(pr.Impls) ||
				si < 0 || si >= len(fd.Signatures) ||
				fd.Signatures[si].Impl != pr.Impls[k] ||
				fd.Signatures[si].TotalArgs() != n {
				return 0, nil, vmDefer(vc.r, curDebug, pc, "vm:user-poly-drift", "CALL_USER_POLY signature drift at "+pr.Word+"; the compiled runtime cannot execute it")
			}
			u := pr.Units[k]
			if u < 0 || u >= len(vc.p.Fns) || vc.p.Fns[u].NParams != n {
				return 0, nil, vmErrAt(curDebug, pc, "CALL_USER_POLY unit shape mismatch at "+pr.Word)
			}
			subset = append(subset, fd.Signatures[si])
			units = append(units, u)
		}
	}
	// Build the args in sig order (position 0 = top of stack, as OpCallUser
	// binds them), then match — identical to callPoly's window.
	window := make([]core.Value, n)
	for i := 0; i < n; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	mr := core.MatchSignature(subset, window, core.WordInfo{ArgCount: n})
	if mr == nil || mr.Sig == nil {
		// The fence-blocked alt (vmDeferAlt) additionally needs the recorded
		// subset to COVER the live table's non-fallback overloads: an arm
		// appended after the record (same arity, so the index drift guard
		// above stayed quiet) could match at run time where this raise would
		// claim failure — bestEffortNoMatch's own arity screen cannot see it.
		// STORED mode has no live table to compare against (fd is nil): the
		// plain defer re-runs the interpreter, which raises the canonical
		// signature_error over its own live dispatch.
		var alt *core.BoruError
		if fd != nil {
			alt = bestEffortNoMatch(vc.r, fd, pr.Word, window, curDebug, pc)
			if alt != nil {
				nonFallback := 0
				for i := range fd.Signatures {
					if !fd.Signatures[i].Fallback {
						nonFallback++
					}
				}
				if nonFallback != len(subset) {
					alt = nil
				}
			}
		}
		return 0, nil, vmDeferAlt(vc.r, curDebug, pc, "vm:user-poly-no-match",
			"CALL_USER_POLY no match for "+pr.Word+"; the compiled runtime cannot execute it for the canonical signature_error", alt)
	}
	for j := range subset {
		if mr.Sig == &subset[j] {
			return units[j], mr.Args, nil
		}
	}
	return 0, nil, vmErrAt(curDebug, pc, "CALL_USER_POLY matched signature outside the recorded arm set at "+pr.Word) //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
}

// callDynamic applies a runtime fn VALUE (sitting below n trailing args) to
// those args — the fn-value-call boundary (plan P4). A compiled closure runs
// VM-native via the re-entrant runner; any other callable (a Function member
// like `r.int`) is applied through the island sub-engine, which auto-applies
// it exactly as the interpreter does. A NON-callable value is left as the
// residual untouched, so a dynamic value that turns out to be data does not
// diverge.
//
// `trailing` selects the SOURCE shape and only changes the non-callable
// residual: for a LEADING fn (`(mk2 5) 10`) the value stays below its args
// ([value, args]); for a TRAILING fn (`5 m.f`, `[..] r.one-of`) the interpreter
// leaves the value ON TOP of its args, so a non-callable trailing value is
// rotated up from the base. The callable result is identical either way.
func (vc *vmContext) callDynamic(reg *core.Registry, n int, trailing bool, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if len(stack) < n+1 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYNAMIC underflow")
	}
	if trailing && n != 1 {
		// OpCallDynamicTrailing is emitted only with arity 1 (bytecode.go): the
		// non-callable residual rotation below puts the fn back on top of its
		// single arg, but with >1 arg the forward args would be collected in the
		// opposite order to the interpreter's top-down stack collection. Assert
		// it so a future lowering bug degrades to a loud internal_error →
		// fallback rather than silently mis-ordering the residual.
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYNAMIC_TRAILING with arity != 1")
	}
	base := len(stack) - n - 1
	fnVal := stack[base]
	args := stack[base+1:]

	if cl, ok := fnVal.Data.(core.ClosurePayload); ok {
		// A fn VALUE closure the window does not match under the LEADING
		// form is left as the interpreter's re-step leaves it: the window
		// as written, fn first (`((mk 1) "s")` is `[fn s]`). The token
		// seam's own no-match fallback re-steps the value TRAILING — the
		// stack-order inputs then the value — and would answer `[s fn]`,
		// the trailing spelling's residual, silently (NUR178's sibling).
		// The TRAILING form's no-match is the window as written too — the
		// arg beneath, the fn on top: `7 if true (mk true) [2]` over a factory
		// whose closure turns out 0-arg is `[7 fn]` interpreted (the anonymous
		// park), and the rotation the lowering made for the apply must be
		// undone, as it is for a non-callable value below (NUR159's probe:
		// `[fn 7]` without this arm).
		if compiler.ClosureIsFnValue(fnVal) {
			if fn, known := vc.closureUnit(cl); known && !closureMatchesArgs(fn, args) {
				if !trailing {
					return stack, nil, nil
				}
				rotated := append(stack[:base:base], args...)
				return append(rotated, fnVal), nil, nil
			}
		}
		// Pass fnVal directly so the payload's InShape rides along (invokeClosure
		// only fills param slots, but a downstream handler may read the shape).
		results, err := vc.invokeClosurePositional(vc.r, fnVal, append([]core.Value(nil), args...))
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		return append(stack[:base], results...), nil, nil
	}
	// An INERT value — a `/v` read, or a fn value the landing parked as the
	// interpreter's re-step did (landingWalk) — is data on both lanes too.
	if fnVal.Quoted || !core.IsAppliableFn(fnVal) {
		// Not callable: leave the value as the residual, matching the interpreter
		// (it does not apply a non-Function). A trailing fn sits ON TOP of its
		// args there, so rotate it up from the base; a leading fn stays below.
		if trailing {
			rotated := append(stack[:base:base], stack[base+1:]...)
			return append(rotated, fnVal), nil, nil
		}
		return stack, nil, nil
	}
	// A trivial-delegation native method (its dispatchable sig is `[Word(name)]`
	// — a module wrapper like rand-int) dispatches VM-NATIVE: MatchSignature
	// picks the overload the interpreter would and the inner handler runs
	// directly — no sub-engine. A user fn carries a REAL body, NOT a delegation
	// word, so it must NOT take this path: tryNativeFnApply would match the
	// InstallFnDef-registered Handler and call it outside the dispatch frame it
	// expects — diverging. Those fall through to the island, which runs the body
	// faithfully as a nested Run.
	if fnDef, ok := fnVal.Data.(core.FnDefInfo); ok && vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return append(stack[:base], results...), nil, nil
		}
	}
	// The Apply kernel: a callee carrying a compiled unit of this program is
	// ENTERED as a frame, not islanded (vm_dyn_apply.go). The args leave the
	// stack here exactly as OpCallUser's do; the unit's RET pushes the result.
	if ent := vc.dynApplyEnter(fnVal, args); ent != nil {
		return stack[:base], ent, nil
	}
	// A callee carrying a DETACHED unit — a module fn's own stamp — is hosted
	// nested rather than islanded (vm_dyn_apply.go, dynApplyForeign).
	if results, ran, err := vc.dynApplyForeign(fnVal, args, 0); ran {
		return vc.dynForeignResults(results, err, stack, base, "dynamic result", curDebug, pc, reg)
	}
	// A MODIFIER WRAPPER re-dispatches what it wraps, and it does so by
	// returning TOKENS — `( stack-part  orig  forward-part )` for the engine
	// to step. That is why it lands here: a paren group needs a tape, and the
	// VM has none. But the group is a pure RESHUFFLE, and it collapses to a
	// permutation that does not depend on the barrier (core.WrapKind):
	// `usurp` reverses the arg vector, the rebarrier family leaves it alone.
	// So the wrapper can be resolved to what it wraps and dispatched here,
	// with no tokens and no island.
	//
	// The chain is WALKED rather than read off ArgsReversed, which is a
	// one-way mark and reports reversed for `usurp (usurp f)` where the
	// composed permutation is the identity — safe for declining a fast path,
	// wrong for performing one.
	//
	// Both tiers above are retried against the unwrapped value, in the same
	// order: a wrapped native reaches tryNativeFnApply (where the wrapper
	// itself could not, since vmNativeApplicable excludes it), and a wrapped
	// user fn reaches the Apply kernel. Anything still unresolved islands as
	// before.
	if inner, reverse, wrapped := core.UnwrapModifierChain(fnVal); wrapped {
		iargs := args
		if reverse {
			iargs = make([]core.Value, n)
			for i := range args {
				iargs[i] = args[n-1-i]
			}
		}
		if ifd, isFn := inner.Data.(core.FnDefInfo); isFn && vmNativeApplicable(vc.r, ifd) {
			if results, done, err := vc.tryNativeFnApply(inner, iargs); done {
				if err != nil {
					return nil, nil, stampAt(err, curDebug, pc, reg)
				}
				return append(stack[:base], results...), nil, nil
			}
		}
		if ent := vc.dynApplyEnter(inner, iargs); ent != nil {
			return stack[:base], ent, nil
		}
		if results, ran, err := vc.dynApplyForeign(inner, iargs, 0); ran {
			return vc.dynForeignResults(results, err, stack, base, "dynamic result", curDebug, pc, reg)
		}
	}
	// Non-trivial fn (user body): apply via the island sub-engine, which
	// auto-applies the Function to the forward args exactly as a nested Run.
	// The TRAILING form islands the window AS WRITTEN — the arg beneath, the
	// fn on top, re-stepped over it — which answers the same when the fn
	// takes the arg and leaves `[arg fn]` when it does not (a 0-arg lambda
	// parks: `7 if true (mk true) [2]` is `[7 fn]` interpreted), where the
	// fn-first layout parked the fn and then pushed the arg (`[fn 7]`).
	island := make([]core.Value, 0, n+1)
	if trailing {
		island = append(island, args...)
		island = append(island, fnVal)
	} else {
		island = append(island, fnVal)
		island = append(island, args...)
	}
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "dynamic result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, err
	}
	return append(stack[:base], results...), nil, nil
}

// callDynFamily routes every fn-value-call-boundary opcode to its handler —
// the single run-loop case for the family. frameBase is the CURRENT frame's
// operand-stack base (the whole-frame replay's resolved-prefix boundary);
// only OpCallDynFrame reads it.
// The *dynEnter return is the Apply kernel's outcome: non-nil means the callee
// carried a compiled unit of THIS program and the run loop should enter it as a
// frame (vm_dyn_apply.go), rather than the handler having islanded it.
func (vc *vmContext) callDynFamily(reg *core.Registry, op compiler.Opcode, arg, frameBase int, stack []core.Value, curDebug []core.SrcPos, pc int, words []compiler.DynFrameWord, head compiler.DynApplyHead, lword compiler.LandingWord) ([]core.Value, *dynEnter, error) {
	switch op {
	case compiler.OpCallDynTrailTop:
		return vc.callDynTrailTop(reg, arg, stack, curDebug, pc, head)
	case compiler.OpCallDynTrailKeepQ:
		// The event-provenance flavour: the runtime quote state survives
		// (no read substitution to mirror). A Quoted fn stays data — the
		// [args, fn] window IS the interpreter's residual for a quoted
		// call result — and an unquoted one applies via the shared body.
		if top := len(stack) - 1; top >= 0 && stack[top].Quoted {
			return stack, nil, nil
		}
		return vc.callDynTrailTop(reg, arg, stack, curDebug, pc, head)
	case compiler.OpCallDynApplyTop:
		return vc.callDynApplyTop(reg, arg, stack, curDebug, pc)
	case compiler.OpCallDynApplyOne:
		return vc.callDynApply(reg, arg, stack, curDebug, pc, true)
	case compiler.OpCallDynFrame:
		return vc.callDynFrame(reg, arg, frameBase, stack, curDebug, pc, words)
	case compiler.OpCallDynMethod:
		return vc.callDynMethod(reg, &vc.p.DynMethods[arg], stack, curDebug, pc)
	case compiler.OpReStepLanding:
		return vc.reStepLanding(reg, arg, frameBase, stack, curDebug, pc, lword)
	default:
		return vc.callDynamicOp(reg, op, arg, stack, curDebug, pc)
	}
}

// callDynamicOp routes a fn-value-call-boundary opcode to its handler, keeping
// the VM's run loop a single case. Trailing only changes the non-callable
// residual order (see callDynamic); mixed islands an interior-fn window.
func (vc *vmContext) callDynamicOp(reg *core.Registry, op compiler.Opcode, arg int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if op == compiler.OpCallDynamicMixed {
		st, err := vc.callDynamicMixed(reg, arg, stack, curDebug, pc)
		return st, nil, err
	}
	return vc.callDynamic(reg, arg, op == compiler.OpCallDynamicTrailing, stack, curDebug, pc)
}

// reStepLanding executes OpReStepLanding (NUR173) — the guarded landing of a
// reach-lowered group's single survivor. See the opcode's own comment for the
// shape; the body here is the whole decision.
//
// The ladder is callDynTrailTop's, over an EMPTY argument window, with one
// clause removed: there is no no-match raise. A fn whose signatures do not
// take zero arguments is what the interpreter leaves as DATA at this landing
// (`m.g` alone, where g takes one argument, is `fn a1(Integer)` on both
// lanes), so the match failing is an answer here, not an error. That is the
// clause OpCallDynTrailTop over zero args could not give.
//
// Entering the matched unit directly (dynApplyEnter) rather than islanding is
// not an optimisation: an island is an interpreter entry, and the project's
// census counts every one. The island stays as the last resort for a fn the VM
// cannot take — a detached ref, a shape whose params do not match the unit.
func (vc *vmContext) reStepLanding(reg *core.Registry, arg, frameBase int, stack []core.Value, curDebug []core.SrcPos, pc int, lword compiler.LandingWord) ([]core.Value, *dynEnter, error) {
	top := len(stack) - 1
	if top < 0 {
		return nil, nil, vmErrAt(curDebug, pc, "RESTEP_LANDING stack underflow")
	}
	v := stack[top]
	if v.Quoted || !core.IsAppliableFn(v) {
		return stack, nil, nil // data on both lanes — stepLiteral pushes it
	}
	if _, isClosure := v.Data.(core.ClosurePayload); isClosure {
		// The interpreter's ANONYMOUS-0-ARG PARK in its closure representation.
		// A fn-VALUE closure IS a `fn` / `=>` literal's value (ClosureIsFnValue
		// asks the unit's own Lambda flag, the flag closureFnDef reads
		// FnDefInfo.Anonymous off for the interpreter), and execFnDefLiteral
		// leaves such a value as DATA at an empty window — which is what makes
		// `def f ([] => [body])` bind the function and not the body's result.
		// Applying it here spent the wrapper before the user's own paren could
		// call it (module-fn.tsv:L47), and on a curried chain it also bought an
		// interpreter entry for nothing: invokeFnValueClosure declines a
		// parameterised unit and RunResolved steps the body to the same answer
		// (bytecode-migrated.tsv:L285, callbacks.tsv:L150).
		// A NAMED fn value that takes no argument is the other side of the
		// gate: a name always calls, so it fires (NUR235).
		if compiler.ClosureIsFnValue(v) && !compiler.ClosureCallsAtLanding(v) {
			return stack, nil, nil
		}
		results, err := vc.invokeClosure(vc.r, v, nil)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		return vc.landingResults(reg, stack, top, results, curDebug, pc)
	}
	if fnDef, ok := v.Data.(core.FnDefInfo); ok {
		// A FUNCTION WORD follows and nothing sits beneath in the frame: the
		// interpreter's re-step plans over that word, and the walk decides
		// (NUR190) — the arms below are the wordless landing's.
		if arg&2 != 0 && top == frameBase && lword.Name != "" && len(fnDef.OwnSigs()) > 0 {
			return vc.landingWalk(reg, v, fnDef, lword, stack, top, curDebug, pc)
		}
		// No signature satisfiable with ZERO arguments: the re-step leaves the
		// fn as DATA (`m.g` alone, where g takes one), so there is nothing to
		// apply and — just as importantly — no island to pay for. A module
		// DELEGATION wrapper is asked the same question here, unlike in
		// noMatchIfSigged: a wrong "no" costs a landing this model would have
		// skipped anyway, where a wrong "yes" costs an interpreter entry on
		// every read of one (module-rand.tsv:L16, `[10 20 30] r.one-of`).
		if len(fnDef.OwnSigs()) > 0 && core.MatchFnSig(v, nil) == nil {
			// A NAMED fn (a name always calls, ADR-011) with a CANDIDATE after
			// it — a function word (a value-bound word is collected, and the
			// arms model that), or the fn frame's tail markers (the op's
			// argument, landingArg) — and nothing beneath it in the frame matches
			// nothing, and the interpreter's re-step RAISES there rather than
			// leaving the value as data: `def mk fn [[][Any][m.f]] end (mk)` is
			// `uncalled_function: call to 'inc' matched no signature` (NUR186).
			// With values beneath the residual arm decides (an island raises
			// the same way over a mismatch, NUR175's rule).
			if arg&1 != 0 && top == frameBase && fnDef.NamedDef() && !fnDef.Macro {
				return nil, nil, stampAt(uncalledFunctionError(reg, fnDef), curDebug, pc, reg)
			}
			return stack, nil, nil
		}
		// The interpreter's ANONYMOUS-0-ARG PARK (execFnDefLiteral): a lambda
		// or macro VALUE that matched nothing — no forward args, no stack args
		// — is DATA, so `def f ([] => [body])` binds the function and not the
		// body's result. A NAMED 0-arg fn is the opposite: its only call form
		// IS nullary, so it dispatches. ADR-016 forbids letting ORIGIN decide
		// anything else, and the landing is a 0-arg window by construction, so
		// the gate reads here exactly as it reads there. Applied is the one
		// exception in both places — `f/v apply` asked for the application.
		if (fnDef.Anonymous && !fnDef.Applied) || fnDef.Macro {
			return stack, nil, nil
		}
		// THE LANDING APPLIES OVER AN EMPTY WINDOW, so it is faithful only
		// where the interpreter's own match at this point would also be empty.
		// execFnDefLiteral matches over the LIVE TAPE and the LIVE STACK; the
		// op has neither — the tokens after the read compiled into later ops,
		// and consuming a stack operand would leave the stack shallower than
		// the lowering predicted. A value with ANY arg-taking overload can
		// therefore be matched differently there than here (NUR175):
		//
		//	h = [] -> 42 and [n:Integer] -> n add 1
		//	5 m.f     interpreted 6 (the unary takes 5 off the stack)
		//	          landed      5 42 (the nullary fired over nothing)
		//	h = [] -> 42 and [x:Atom/q] -> x
		//	m.f z     interpreted z (the /q slot CAPTURES the word)
		//	          landed      42 z (the nullary fired one token early)
		//
		// Only-0-arg settles both: no overload can take the stack operand, and
		// none can quote-capture the following word. Standing aside costs
		// nothing — the read keeps today's residual apply, which is what
		// answered these correctly before the landing existed.
		if !core.FnValueOnlyZeroArgSigs(fnDef) {
			return stack, nil, nil
		}
		return vc.landingFire(reg, v, fnDef, stack, top, curDebug, pc)
	}
	// Neither a closure nor a FnDefInfo (both arms above return): no frame
	// to push, so the island runs the value.
	results, err := vc.islandRun(reg, []core.Value{v})
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	return vc.landingResults(reg, stack, top, results, curDebug, pc)
}

// landingFire applies the landed named fn's zero-argument overload over the
// empty window, ONE result, because that is what the recorded landing claims
// and the stack shape the lowering predicted. A 0-return member (`def h fn
// [[] [] []] end`) is applied by the interpreter for its effect and leaves
// the stack as it found it; the landing cannot express that, so it stands
// aside here rather than failing the run at landingResults.
func (vc *vmContext) landingFire(reg *core.Registry, v core.Value, fnDef core.FnDefInfo, stack []core.Value, top int, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if sig := core.MatchFnSig(v, nil); sig == nil || len(sig.Returns) != 1 {
		return stack, nil, nil
	}
	if vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(v, nil); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return vc.landingResults(reg, stack, top, results, curDebug, pc)
		}
	}
	if ent := vc.dynApplyEnter(v, nil); ent != nil {
		return stack[:top], ent, nil
	}
	results, err := vc.islandRun(reg, []core.Value{v})
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	return vc.landingResults(reg, stack, top, results, curDebug, pc)
}

// landingWalk is the landing over a FUNCTION WORD (NUR190): the
// interpreter's re-step of a fn value plans over the live tape
// (execFnDefLiteral, PlanMatch), and with that word the next token the plan
// is a function of the run-time fn's overloads — so the walk runs the SAME
// plan over a two-token window, the value and the word, with nothing
// beneath, and the outcome is the interpreter's:
//
//   - no overload matches: a named fn RAISES `uncalled_function` (a name
//     always calls, ADR-011); an anonymous or macro value PARKS — data for
//     the rest of the statement, which the value's inert mark says to every
//     later arm (the residual apply would otherwise take the word's result:
//     `m.l z` was 1 for `[fn lam(Integer) 0]`);
//   - the plan claims the word for an Any-typed slot (a speculative claim,
//     PlanMatch's specAt): the strict barrier raises the interpreter's
//     stranded-forward `signature_error` (`m.a z`);
//   - the zero-argument fallback: it FIRES for a named fn (`m.f z` with h's
//     nullary and unary overloads is `[42 0]`; the wordless landing stood
//     aside for the mixed overload, NUR175, and the residual apply answered
//     1) and PARKS an anonymous or macro value, as the wordless landing
//     does (ADR-016's anonymous-0-arg park);
//   - a `/q` slot CAPTURES the word: the value stands aside as before and
//     the residual apply keeps its answer — the word's call is already
//     compiled after the landing and cannot be skipped, so the capture is
//     the open half of NUR190 (fn-value.tsv's `m.f z` passes because z's
//     result is its own atom), and the run bails loudly.
//
// That is every claim the plan can make on a function word. A typed slot —
// a Function-typed one included — takes a bare fn name by `/v` alone
// (NUR078; the landing never walks a `/v` word, whose value the residual
// arms collect — `m.g z/v` is 7 on both lanes), so `m.g z` is the named
// no-match above, where it used to take the word's REFERENCE and bail.
func (vc *vmContext) landingWalk(reg *core.Registry, v core.Value, fnDef core.FnDefInfo, lword compiler.LandingWord, stack []core.Value, top int, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	own := fnDef
	own.Signatures = fnDef.OwnSigs()
	h := newRegionHostOver(reg, []core.Value{v, core.NewWord(lword.Name)})
	w := core.WordInfo{Name: fnDef.Name, ArgCount: -1}
	if err := h.Collected(core.CollectForward(h, &own, w, 1)); err != nil { //covergate:allow the window is the value and one plain word: the pre-walk evaluates only parens, lists and sugar markers, and a Word token is none of them, so it returns nil; kept as the honest arm for a kernel change (§compiler)
		return nil, nil, vmDefer(reg, curDebug, pc, "vm:landing-walk", "RESTEP_LANDING at "+fnDef.Name+": the re-step's walk over `"+lword.Name+"` needs an evaluation this host cannot perform; the compiled runtime cannot execute it")
	}
	sig, positions, specAt := core.PlanMatch(h, h.win, reg, &own, w, nil, 0, false, false, false)
	switch {
	case sig == nil || sig.Fallback:
		if fnDef.NamedDef() && !fnDef.Macro {
			return nil, nil, stampAt(uncalledFunctionError(reg, fnDef), curDebug, pc, reg)
		}
		parked := v
		parked.Quoted = true
		stack[top] = parked
		return stack, nil, nil
	case specAt >= 0:
		nf := 0
		for _, at := range positions {
			if at > 0 {
				nf++
			}
		}
		return nil, nil, stampAt(core.StrandedForwardDiag(reg.Source, fnDef.Name, nf-specAt, lword.Name, core.BarrierReceiverWord(reg, lword.Name), lword.Pos), curDebug, pc, reg)
	case sig.TotalArgs() == 0:
		// The interpreter's ANONYMOUS-0-ARG PARK (execFnDefLiteral): a lambda
		// or macro VALUE whose plan matched nothing is data — the wordless
		// landing's rule, read here exactly as there (`if true (mk) [2]` with
		// a factory's `([] => [1])` is `[fn]`, and the sweep's do-catch cell
		// answered 1 while this arm was missing). Applied is the one
		// exception in both places.
		if (fnDef.Anonymous && !fnDef.Applied) || fnDef.Macro {
			return stack, nil, nil
		}
		return vc.landingFire(reg, v, fnDef, stack, top, curDebug, pc)
	}
	// What is left is a `/q` slot CAPTURING the word as an atom (`m.q z` is
	// `[z]` interpreted) — the one slot the plan's word arm claims a
	// function word through, beside the speculative Any claim above. The
	// compiled code calls the word after the landing and the residual arm
	// applies the value over its result, so no op after this one can honour
	// the claim: the landing's DEOPT does (NUR190) — the value and the body
	// from the word on go to the interpreter, which captures the word
	// exactly as its own re-step does. A landing with no island (the word
	// sits in a nested fragment, or the unit has no island environment)
	// defers loudly, as it always has.
	if lword.Deopt {
		// The walk runs only over an empty frame region (top == frameBase):
		// the island's prefix is empty and its residual replaces the value.
		return vc.landingDeopt(reg, v, lword, top, stack, top, curDebug, pc)
	}
	if lword.SkipTo > 0 {
		return vc.landingSkip(reg, v, lword, stack, top, curDebug, pc)
	}
	return nil, nil, vmDefer(reg, curDebug, pc, "vm:landing-quote-claim", "RESTEP_LANDING at "+fnDef.Name+": the re-step CAPTURES the word `"+lword.Name+"` (a `/q` slot) where the compiled code calls the word; the compiled runtime cannot execute it")
}

// landingSkip answers a landing's `/q` claim where no island can be rebuilt
// (NUR190, LandingWord.SkipTo): the capture runs on the interpreter over the
// value and the word alone — the interpreter's own re-step, which takes the
// word as an atom and never runs it — and its results take the place of
// the word's call and the paren apply after it, whose claimed count they
// must match (a count the apply did not claim defers, loudly, as the
// landing always did).
func (vc *vmContext) landingSkip(reg *core.Registry, v core.Value, lword compiler.LandingWord, stack []core.Value, top int, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	results, err := vc.islandRun(reg, []core.Value{v, core.WithPosAt(core.NewWord(lword.Name), lword.Pos)})
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	if len(results) != lword.SkipOut {
		return nil, nil, vmDefer(reg, curDebug, pc, "vm:landing-skip-count", fmt.Sprintf("RESTEP_LANDING: the `/q` capture of `%s` left %d value(s) where the apply after it claims %d; the compiled runtime cannot execute it", lword.Name, len(results), lword.SkipOut))
	}
	return append(stack[:top], results...), &dynEnter{jump: true, jumpPC: lword.SkipTo}, nil
}

// landingDeopt runs a landing's `/q` claim on the interpreter (NUR190): the
// user parens the word sits in re-opened, the landed value, then the body
// from the word on (LandingWord.Island) — the word the value captures, and
// everything after it, which the compiled code lowered on the model that
// the word runs — over the frame region beneath
// the value as the resolved prefix (the walk runs only with nothing beneath
// it in the model, so the region holds only what the frame kept). The
// island's residual replaces the frame region and the run continues at
// lword.RetPC: the unit's RET (its runtime-variable discipline, RetReplay),
// or the program's end. A unit's island tears its own defs down, as the
// frame's cleanup would; a top-level one's outlive it, as a top-level def
// does.
func (vc *vmContext) landingDeopt(reg *core.Registry, v core.Value, lword compiler.LandingWord, frameBase int, stack []core.Value, top int, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if len(lword.Island) == 0 || lword.RetPC < 0 || top < frameBase {
		return nil, nil, vmErrAt(curDebug, pc, "RESTEP_LANDING bad deopt entry")
	}
	prefix := append([]core.Value(nil), stack[frameBase:top]...)
	tokens := make([]core.Value, 0, lword.Opens+1+len(lword.Island))
	for i := 0; i < lword.Opens; i++ {
		tokens = append(tokens, core.NewOpenParen())
	}
	tokens = append(append(tokens, v), lword.Island...)
	snapshot := reg.Defs.Snapshot()
	results, err := runIslandResolved(reg, prefix, tokens)
	if !lword.Root {
		core.TruncateFrameDefs(reg, snapshot)
	}
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "landing island result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, nil, err
	}
	return append(stack[:frameBase], results...), &dynEnter{jump: true, jumpPC: lword.RetPC}, nil
}

// uncalledFunctionError is the interpreter's own no-match raise for a named
// fn value re-stepped at the pointer (execFnDefLiteral's uncalled_function
// arm), built for the landing: the same code, detail and hint; the position
// is stamped from the op's debug entry (the noted landing's token).
func uncalledFunctionError(reg *core.Registry, fnDef core.FnDefInfo) error {
	detail := "call to '" + fnDef.Name + "' matched no signature"
	hint := "hint: check the call's argument types and arity — or use " +
		fnDef.Name + "/v to push the function as a value deliberately"
	src := ""
	if reg != nil {
		src = reg.Source
	}
	return core.MakeBoruErrorAt("uncalled_function", detail, fnDef.Name, src, hint, core.SrcPos{})
}

// landingResults seats one applied landing's results over the value it
// replaced. The recorded landing claims ONE value (the read's own carrier), so
// a member whose apply nets another count is a claim failure — loud, at the
// landing, rather than a stack the caller cannot reconcile.
func (vc *vmContext) landingResults(reg *core.Registry, stack []core.Value, top int, results []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if err := vc.screenResults(results, "re-stepped landing at a member read", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, err
	}
	if len(results) != 1 {
		return nil, nil, vmDefer(reg, curDebug, pc, "vm:restep-landing-nout",
			"re-stepped landing: the applied member returned "+strconv.Itoa(len(results))+
				" values where the read's recorded landing claims one")
	}
	return append(stack[:top], results[0]), nil, nil
}

// callDynTrailTop applies a runtime FUNCTION value ON TOP of its n args to those
// args — the paren-bounded trailing fn-value apply (`(prev key comp)`). The fn
// stays on top (no rotation): on apply it auto-applies to the n args beneath it
// exactly as the interpreter's paren auto-dispatch (the island Run([fn]+args) is
// byte-identical to the token sequence the interpreter ran); a non-callable value
// leaves [args, fn] untouched — ALREADY the interpreter's trailing residual. Sound
// for ANY arity (unlike OpCallDynamicTrailing's 1-arg rotation). The args slice is
// the same stack-order window callDynamic's leading case feeds, so the closure /
// island binding matches the proven leading path.
func (vc *vmContext) callDynTrailTop(reg *core.Registry, n int, stack []core.Value, curDebug []core.SrcPos, pc int, head compiler.DynApplyHead) ([]core.Value, *dynEnter, error) {
	if len(stack) < n+1 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_TRAIL_TOP underflow")
	}
	top := len(stack) - 1
	fnVal := stack[top]
	// A VALUE delivery — a `/v` read the recorder marked
	// (DynApplyHead.ValueDelivery) — is applied when an overload takes the
	// window and otherwise left as DATA beside it on the interpreter: a
	// value, not the word dispatch a bare read makes, so no signature_error
	// (NUR124's fifth witness: `(g/v 5)` over a String-only g is `[fn 5]`
	// and the frame's count error on both lanes, where the VM raised
	// `cannot call `g`` through the nameless no-match builder).
	delivered := head.ValueDelivery
	// The op stands for a READ-SUBSTITUTED trailing fn (RecordDynApply fires
	// at the paren collapse of a WORD-read arrival, where the interpreter's
	// substitution strips one quote level before the auto-apply). A compiled
	// LOCAL push carries the STORED value verbatim — including the
	// construction-time quote of a `/v` reference or a `quote (fn …)` arg —
	// so mirror the read here: strip Quoted from the applied copy (probe-
	// found off-corpus divergence: `[1 2] each [(1 2 c)]` with c bound from
	// `(…)/v` islanded the still-quoted fn as INERT and compiled [[1 1]] vs
	// the interpreter's [[3 3]]). The strip is sound ONLY for a substituted
	// arrival: RecordDynApply declines an EVENT-provenance fn (a direct call
	// result, which the interpreter does NOT substitute and whose runtime
	// quote must survive — PR #280 review), so it never reaches this op.
	fnVal.Quoted = false
	base := top - n
	// The args sit BELOW the fn in stack order (deepest first). The interpreter
	// binds a trailing fn's args TOP-DOWN (the top arg → the fn's first param);
	// the island Run / forward apply binds the FIRST following token → the first
	// param. So reverse the stack window into forward order, making the island bind
	// identical to the interpreter's paren auto-dispatch (`(x 2 comp)` → comp's
	// first param = 2 (the top), second = x — verified against the off-corpus
	// comparator regression).
	args := make([]core.Value, n)
	for i := 0; i < n; i++ {
		args[i] = stack[top-1-i]
	}
	if cl, ok := fnVal.Data.(core.ClosurePayload); ok {
		// Under a seated NAME the op stands for a WORD dispatch (a def-bound
		// computed fn read at a closure body's tail, `each [a5] xs`), and the
		// interpreter raises `cannot call `a5`` where the closure's contract
		// matches nothing — a paren-bounded VALUE apply parks instead.
		if fn, known := vc.closureUnit(cl); delivered && known && !closureMatchesArgs(fn, args) {
			return parkedWindow(stack, base, top, head), nil, nil // a /v-delivered closure the window does not fit stays data
		}
		if head.Name != "" {
			if fn, known := vc.closureUnit(cl); known && !closureMatchesArgs(fn, args) {
				// A render-only bridge: the no-match only reads its signature.
				if fnv, built := closureFnDef(fn, cl, nil); built {
					fd, _ := fnv.Data.(core.FnDefInfo)
					view := installedSigView(fd)
					written := args[:min(head.NWritten, len(args))]
					return nil, nil, stampAt(core.NoMatchDiag(reg.Source, head.Name, &view, written, head.Pos, core.ReorderHintFor(head.Name, &view, written)), curDebug, pc, reg)
				}
			}
		}
		results, err := vc.invokeClosurePositional(vc.r, fnVal, args)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		return append(stack[:base], results...), nil, nil
	}
	if !core.IsAppliableFn(fnVal) {
		return stack, nil, nil // not callable: [args, fn] is already the interpreter's trailing residual
	}
	// A NAME-read lead whose only overloads take NO argument, under a window
	// (NUR176: `(k x)` over a 0-arg `k`, `(1 2 c)` over a 0-arg `c`), is not
	// a no-match: the interpreter dispatches the bare read as a WORD where it
	// stands — the fn fires over nothing — and then steps the window's other
	// tokens on their own: a leading window's arguments AFTER the result (a
	// fn value dispatching over it, a literal landing beside it), a trailing
	// window's beneath it. That is the island's own semantics over the
	// window in its WRITTEN order, so hand it the window that way (the args
	// ride top-down, the last-written first; head.Leading says which side of
	// the lead they were written on), the lead marked applied so the
	// island's re-step dispatches an anonymous 0-arg value as the word
	// dispatch did. An event-produced or `/v`-delivered lead has no name to
	// dispatch under, and keeps the no-match below.
	if fd, ok := fnVal.Data.(core.FnDefInfo); ok && n > 0 && head.Name != "" && core.FnValueOnlyZeroArgSigs(fd) {
		fd.Applied = true
		fnVal.Data = fd
		island := make([]core.Value, 0, n+1)
		if !head.Leading {
			for i := n - 1; i >= 0; i-- {
				island = append(island, args[i])
			}
		}
		island = append(island, fnVal)
		if head.Leading {
			for i := n - 1; i >= 0; i-- {
				island = append(island, args[i])
			}
		}
		results, err := vc.islandRun(reg, island)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		if err := vc.screenResults(results, "dynamic trailing-top result at a 0-arg lead", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return nil, nil, err
		}
		return append(stack[:base], results...), nil, nil
	}
	if delivered && (head.Leading || head.WrittenFirst) && core.MatchFnSig(fnVal, args) == nil {
		return parkedWindow(stack, base, top, head), nil, nil // a /v-delivered fn written before the window stays data
	}
	if parked, err := valueTrailNoMatch(reg, fnVal, args, head, curDebug, pc); err != nil {
		return nil, nil, err
	} else if parked {
		return stack, nil, nil
	}
	if err := noMatchIfSigged(reg, fnVal, args, curDebug, pc, reg, head); err != nil {
		return nil, nil, err
	}
	if fnDef, ok := fnVal.Data.(core.FnDefInfo); ok && vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return append(stack[:base], results...), nil, nil
		}
	}
	if ent := vc.dynApplyEnter(fnVal, args); ent != nil {
		return stack[:base], ent, nil
	}
	if results, ran, err := vc.dynApplyForeign(fnVal, args, 0); ran {
		return vc.dynForeignResults(results, err, stack, base, "dynamic trailing-top result at fn-value apply", curDebug, pc, reg)
	}
	island := make([]core.Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "dynamic trailing-top result at fn-value apply", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, err
	}
	return append(stack[:base], results...), nil, nil
}

// valueTrailNoMatch is the no-match of a VALUE applied as a TRAILING window
// — a `/v` delivery, a literal, a produced fn: no bare read the interpreter
// dispatches as a word, and written after its arguments (NUR238). The
// interpreter re-steps the value over the window (execFnDefLiteral): an
// anonymous value that matches nothing is DATA (ADR-016's gate — the window
// stays as written, parked true), and a named one raises uncalled_function.
// A value with no own signature to consult, one the window fits, a bare
// read (head.Name) and a window written the other way are not this rule's.
func valueTrailNoMatch(reg *core.Registry, fnVal core.Value, args []core.Value, head compiler.DynApplyHead, curDebug []core.SrcPos, pc int) (bool, error) {
	if head.Name != "" || head.Leading || head.WrittenFirst {
		return false, nil
	}
	fd, ok := fnVal.Data.(core.FnDefInfo)
	if !ok || len(fd.OwnSigs()) == 0 || core.IsDelegationFnDef(fd) || core.MatchFnSig(fnVal, args) != nil {
		return false, nil
	}
	if (fd.Anonymous && !fd.Applied) || fd.Macro {
		return true, nil
	}
	return false, stampAt(uncalledFunctionError(reg, fd), curDebug, pc, reg)
}

// parkedWindow is the residual a value-delivered fn the window does not fit
// leaves on the interpreter: the window in its WRITTEN order. A trailing
// window (`(5 g/v)`) is already the stack's order, fn on top; one that wrote
// the fn FIRST (`(g/v 5)`, Leading or WrittenFirst) puts it back beneath its
// arguments.
func parkedWindow(stack []core.Value, base, top int, head compiler.DynApplyHead) []core.Value {
	if !head.Leading && !head.WrittenFirst {
		return stack
	}
	win := make([]core.Value, 0, top-base+1)
	win = append(win, stack[top])
	win = append(win, stack[base:top]...)
	return append(stack[:base], win...)
}

// noMatchIfSigged raises when fnVal is a Function carrying OWN SIGNATURES none
// of which admits args — NUR107.
//
// The distinction it draws is the whole point. Leaving `[args, fn]` on the
// stack is RIGHT for a value that is not callable, and the interpreter does the
// same. It is WRONG for a Function whose overloads simply do not take these
// arguments: the interpreter's word dispatch raises signature_error there, and
// the VM used to read "no overload matched" as "not callable" and answer data.
// Measured, that produced a wrong answer with no error at all —
//
//	def ld fn [[g:Function x:Integer] [Function Integer] [(g x)]]
//	ld ([k:String] => [k]) 14
//	  interpreted  signature_error
//	  compiled     [fn (String) 14]
//
// — because the frame's return-count check happened to accept the two-value
// residual. Shapes whose return arity did NOT accept it merely surfaced a
// type_error instead, which is why this looked like a taxonomy quibble rather
// than a silent miscompile.
//
// A value with no FnDefInfo payload (a fn-typed carrier, a closure) has no own
// signatures to consult, so it keeps the data behaviour: MatchFnSig's nil is
// "no opinion" there, and the length check below is what separates the two
// readings of nil.
func noMatchIfSigged(reg *core.Registry, fnVal core.Value, args []core.Value, curDebug []core.SrcPos, pc int, r *core.Registry, head compiler.DynApplyHead) error {
	// Three ways a value has NO own signatures worth consulting here, all
	// meaning the same thing — this guard has no opinion, leave the window to
	// the paths below:
	//
	//   - no FnDefInfo payload at all (a bare `Function` type literal is
	//     appliable by lattice TAG, with nothing to match against);
	//   - an FnDefInfo carrying only fallback sigs;
	//   - a module DELEGATION wrapper, the one fn value whose own signatures
	//     are not the ones dispatch consults — execFnDefLiteral looks the
	//     inner native up by NAME and matches against ITS signatures
	//     (lang/go/CLAUDE.md, "Module FnDef wrappers — inner sig BarrierPos").
	//     Asking MatchFnSig about the wrapper answers the wrong question, and
	//     answering it rejected perfectly well-formed calls —
	//     TestSeam7DelegationApplySuccess caught that immediately. The
	//     delegation branch below does the real matching.
	fnDef, ok := fnVal.Data.(core.FnDefInfo)
	if !ok || len(fnDef.OwnSigs()) == 0 || core.IsDelegationFnDef(fnDef) {
		return nil
	}
	if core.MatchFnSig(fnVal, args) != nil {
		return nil
	}
	// A head the recorder saw read BARE under a frame binding
	// (CompiledFn.DynApplyName): the interpreter dispatched that read as a
	// WORD, so it names the binding, anchors the caret at the read, and
	// explains the failing overloads from the fn it found. The applied value
	// IS that fn here, so hand it to the same builder the interpreter's own
	// dispatch uses rather than looking the name up — a frame binding is a
	// SLOT on this lane, and Registry.Lookup would find nothing.
	if head.Name != "" {
		view := installedSigView(fnDef)
		// The interpreter's tuple is the tokens its forward window CONSUMED,
		// which stops at the first bare read — the recorder counted that
		// leading run (DynApplyHead.NWritten). Passing every applied argument
		// instead reported `the argument was 7` where the interpreter reports
		// `takes 1 argument, but none were supplied` (NUR122).
		written := args[:min(head.NWritten, len(args))]
		return stampAt(core.NoMatchDiag(r.Source, head.Name, &view, written, head.Pos, core.ReorderHintFor(head.Name, &view, written)), curDebug, pc, r)
	}
	return stampAt(core.RuntimeNoMatch(reg, fnDef.Name, args), curDebug, pc, r)
}

// installedSigView is fnDef as the interpreter's REGISTRY holds it, for the
// no-match diagnostic only.
//
// A boru fn authored without a `|` boundary carries BarrierPos ==
// BarrierAllForward (-1); registration resolves that sentinel to the sig's arg
// count (registry.go upsertFnDef), and the diagnostic reads the RESOLVED value
// — HasForwardSigs is what gates the "group the call in parens" suggestion. The
// interpreter dispatches the head through its INSTALLED binding and so prints
// that line; the compiled lane applies the const, which still carries -1, and
// dropped it. This is the same reconciliation closureAsWord makes for the
// replay's bridge (vm_dyn_words.go), kept local to the diagnostic: the shared
// predicate also picks the DISPATCH mode for a raw fn value at the pointer
// (engine.go), which this must not move.
//
// Copies the signature slice — the const is shared program state.
func installedSigView(fnDef core.FnDefInfo) core.FnDefInfo {
	sigs := make([]core.Signature, len(fnDef.Signatures))
	copy(sigs, fnDef.Signatures)
	for i := range sigs {
		if sigs[i].BarrierPos == core.BarrierAllForward {
			sigs[i].BarrierPos = sigs[i].TotalArgs()
		}
	}
	fnDef.Signatures = sigs
	return fnDef
}

// callDynApplyTop is callDynTrailTop under the `apply` WORD's semantics
// (Stage M2a, OpCallDynApplyTop): the interpreter's applyHandler UNQUOTES the
// fn value and re-steps it against the preceding stack, so a /v-parked
// (Quoted) fn value applies here where the paren-bounded trailing apply would
// leave it as data. The n args below the fn bind top-down (top arg → first
// param), identical to callDynTrailTop's reversed-window forward bind. A
// non-FnDefInfo, non-closure payload raises applyHandler's own byte-identical
// error — the same taxonomy the interpreter's dispatch of `apply` yields.
func (vc *vmContext) callDynApplyTop(reg *core.Registry, n int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	return vc.callDynApply(reg, n, stack, curDebug, pc, false)
}

// callDynApply is callDynApplyTop's shared body; one selects the EVENT form
// (OpCallDynApplyOne): exactly one result or a defer.
func (vc *vmContext) callDynApply(reg *core.Registry, n int, stack []core.Value, curDebug []core.SrcPos, pc int, one bool) ([]core.Value, *dynEnter, error) {
	r := vc.r
	if len(stack) < n+1 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_APPLY_TOP underflow")
	}
	top := len(stack) - 1
	fnVal := stack[top]
	base := top - n
	args := make([]core.Value, n)
	for i := 0; i < n; i++ {
		args[i] = stack[top-1-i]
	}
	// commit is the ONE-result discipline of the event form (OpCallDynApplyOne):
	// the model committed exactly one result, so any other count defers the
	// run to the interpreter rather than misaligning the frame's stack.
	commit := func(results []core.Value, err error) ([]core.Value, *dynEnter, error) {
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		if one && len(results) != 1 {
			return nil, nil, vmDefer(r, curDebug, pc, "dyn-apply-one", fmt.Sprintf("apply over a gradual lead netted %d value(s), not the one the model committed", len(results)))
		}
		return append(stack[:base], results...), nil, nil
	}
	if cl, ok := fnVal.Data.(core.ClosurePayload); ok {
		// A compiled closure of the window's own arity runs VM-native; any
		// other arity takes the interpreter's apply re-step below, whose
		// bridge (ClosureAsFnDef) under- or over-applies exactly as the
		// interpreter's fn value does. The arity is the unit's PARAM slots
		// alone: NParams counts the trailing capture slots too, and reading
		// it whole sent every capturing closure — `99 (kk 7) apply`, the
		// twenty-eighth increment's whole family — to the island (the
		// interp-entry census caught it, 40 over its ceiling of 33).
		fn, known := vc.closureUnit(cl)
		if !known || fn.NParams-fn.NCaptures == n {
			return commit(vc.invokeClosurePositional(vc.r, fnVal, args))
		}
		// A 0-ARG closure under the apply WORD fires over nothing and its
		// result lands ABOVE the untouched window — applyHandler's
		// MarkApplied, which the interpreter applies to a fn VALUE and the
		// re-step island below cannot see through a closure payload (the
		// bridged lambda stays an anonymous 0-arg VALUE there and parks:
		// `5 (mk0 1)/v apply` compiled to `[5 fn]` for the interpreter's
		// `[5 1]`, the dynamic-lead group, 2026-09-22). The event form's
		// one-result discipline still holds: two values are not the one
		// the model committed, so it defers as any other count does.
		if !one && fn.NParams-fn.NCaptures == 0 {
			results, err := vc.invokeClosure(vc.r, fnVal, nil)
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return append(stack[:top], results...), nil, nil
		}
		return commit(vc.applyReStep(reg, fnVal, args, curDebug, pc))
	}
	if fnVal.Parent == nil || !fnVal.Parent.ConformsTo(core.TFunction) {
		// A GRADUAL lead (the twenty-seventh increment) that is no fn at
		// run time. A lens takes `apply`'s [Reach Any] overload on the
		// interpreter — a receiver-rebinding get the VM does not model —
		// so defer the run; anything else is the interpreter's own
		// `apply` no-match over the same stack window (its stack-only match
		// looked at the top two values), raised at the apply word's
		// position, which this op carries.
		if fnVal.Parent != nil && fnVal.Parent.Equal(core.TReach) {
			return nil, nil, vmDefer(r, curDebug, pc, "dyn-apply-top", "apply over a lens value; the compiled runtime cannot execute it")
		}
		written := []core.Value{fnVal}
		if n > 0 {
			written = append(written, args[0])
		}
		return nil, nil, stampAt(core.RuntimeNoMatch(reg, "apply", written), curDebug, pc, reg)
	}
	fnDef, ok := fnVal.Data.(core.FnDefInfo)
	if !ok {
		// applyHandler's own error, byte-identical (the interpreter dispatches
		// `apply` over the same runtime value and raises exactly this).
		return nil, nil, stampAt(fmt.Errorf("apply: function value carries no FnDefInfo (got %T)", fnVal.Data), curDebug, pc, reg)
	}
	fnVal.Quoted = false // applyHandler: the parked value becomes a live call site
	fnVal = core.MarkApplied(fnVal)
	if vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			return commit(results, err)
		}
	}
	// The inline unit entry returns through its own RET, whose count the
	// event form cannot check — so it is taken only under a contract
	// promising the one return the model committed. The contract is the
	// APPLIED VALUE's (applyRetContract, the frame's RET enforces it), never
	// the entered unit's own: a stamped fn-value unit declares no returns of
	// its own, and reading the unit sent every stamped lambda — `f:Any =>
	// [f/v]` fetched from a map, Church false's inner lambda — to the island
	// (the twenty-eighth increment; dynMethodClaimOK is the same reading).
	if ent := vc.dynApplyEnter(fnVal, args); ent != nil && (!one || dynMethodClaimOK(ent, 1)) {
		return stack[:base], ent, nil
	}
	nout := 0
	if one {
		nout = 1
	}
	if results, ran, err := vc.dynApplyForeign(fnVal, args, nout); ran {
		return commit(results, err)
	}
	return commit(vc.applyReStep(reg, fnVal, args, curDebug, pc))
}

// applyReStep is the interpreter's applyHandler re-step, islanded: the args
// are the resolved stack (deepest first) and the fn value is stepped over
// them at the pointer, so the fn collects from the stack exactly as the
// interpreter's apply word makes it — a 0-arg fn fires and leaves the stack
// beneath it, a fn of a larger arity under-applies and parks, and a mismatch
// raises the interpreter's own diagnostic. The earlier island stepped the
// fn FIRST with the args as forward tokens, which put a 0-arg fn's result
// beneath the args the interpreter leaves beneath it.
func (vc *vmContext) applyReStep(reg *core.Registry, fnVal core.Value, args []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	inputs := make([]core.Value, len(args))
	for i, a := range args {
		inputs[len(args)-1-i] = a
	}
	if reg == nil {
		reg = vc.r
	}
	results, err := runIslandResolved(reg, inputs, []core.Value{fnVal})
	if err != nil {
		return nil, err
	}
	if err := vc.screenResults(results, "dynamic apply-top result at fn-value apply", curDebug, pc); err != nil { //covergate:allow tape-coupled results cannot leave a resolved island: its inputs are values and its one token a fn (§compiler)
		return nil, err
	}
	return results, nil
}

// closureUnit resolves the CompiledFn a closure payload names — in the
// running program or, for a closure minted elsewhere, its own (a hosted
// foreign unit); known=false for a payload that names no unit.
func (vc *vmContext) closureUnit(cl core.ClosurePayload) (*compiler.CompiledFn, bool) {
	prog := vc.p
	if fp, foreign := vc.closureProgram(cl); foreign {
		prog = fp
	}
	if prog == nil || cl.Unit < 0 || cl.Unit >= len(prog.Fns) {
		return nil, false
	}
	return &prog.Fns[cl.Unit], true
}

// callDynMethod is the GUARDED mid-stream shaped-instance-method apply
// (Stage M2c, OpCallDynMethod): the runtime method value sits ON TOP of
// its spec.NArgs args (the recorder lays operands in sig order, ops[0] on
// top — fn at top, first arg at top-1, exactly callDynTrailTop's
// reversed-window forward bind), and the program CONTINUES past this op
// with spec.NOut results committed downstream. So unlike callDynamic —
// where a non-callable value soundly stays as the residual — EVERY
// shape-claim failure here defers to the interpreter via internal_error
// (runtimeShouldFallback): a non-callable or /v-parked (Quoted) value, or
// a result count differing from the claim. The apply itself is the proven
// boundary machinery: a compiled closure runs VM-native, a
// trivial-delegation method dispatches its inner native directly, and any
// other callable islands [fn, a1..aN] — byte-identical to the
// interpreter's forward auto-dispatch of the same window. A genuine boru
// error from the method surfaces as-is (the interpreter raises the same,
// prior side effects included).
func (vc *vmContext) callDynMethod(reg *core.Registry, spec *compiler.DynMethodSpec, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, *dynEnter, error) {
	if err := vc.gateWord(reg, spec.Word); err != nil {
		return nil, nil, err
	}
	n := spec.NArgs
	if len(stack) < n+1 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_METHOD underflow at "+spec.Word)
	}
	top := len(stack) - 1
	fnVal := stack[top]
	base := top - n
	args := make([]core.Value, n)
	for i := 0; i < n; i++ {
		args[i] = stack[top-1-i]
	}
	guard := func(results []core.Value) ([]core.Value, *dynEnter, error) {
		if len(results) != spec.NOut {
			// A count differing from the shape claim indicts a HOST-CONTRACT
			// violation, not compiler model debt: a boru-source method's
			// count is the checker's own body model (return contracts are
			// engine-enforced), so the only way here is a host registration
			// whose handler returned a count its own signature denies — the
			// recovered-panic class. Raise the plain internal_error
			// (runtimeShouldFallback still resolves it by re-running on the
			// tolerant interpreter, fenced as ever); the runtime-bail census
			// counts DESIGNED model-miss defers only, and this is not one.
			return nil, nil, vmErrAt(curDebug, pc, fmt.Sprintf(
				"shaped method apply %s: result count %d violates the host-registered shape claim %d",
				spec.Word, len(results), spec.NOut))
		}
		if err := vc.screenResults(results, "shaped method result at "+spec.Word, curDebug, pc); err != nil {
			return nil, nil, err
		}
		return append(stack[:base], results...), nil, nil
	}
	if _, ok := fnVal.Data.(core.ClosurePayload); ok && !fnVal.Quoted {
		results, err := vc.invokeClosurePositional(vc.r, fnVal, args)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		return guard(results)
	}
	if !core.IsAppliableFn(fnVal) || fnVal.Quoted {
		// The shape claim failed outright: the read did not surface a live
		// method value. The interpreter would leave it as data and continue
		// with a DIFFERENT stack shape, which this program cannot express —
		// defer wholesale.
		return nil, nil, vmDefer(vc.r, curDebug, pc, "vm:shaped-method-not-appliable", "shaped method apply "+spec.Word+
			": value is not an appliable function at run time; the compiled runtime cannot execute it")
	}
	if fnDef, ok := fnVal.Data.(core.FnDefInfo); ok && vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return guard(results)
		}
	}
	// The Apply kernel (vm_dyn_apply.go): a callee carrying a compiled unit of
	// THIS program is entered as a frame instead of islanded. `args` is already
	// in sig order here (the recorder lays ops[0] on top), which is the order
	// dynApplyEnter binds and the order the island's forward auto-dispatch
	// produces — so the two paths select and bind identically.
	//
	// The shape claim's RESULT half is discharged BEFORE the entry, because a
	// frame push has no results to count — dynMethodClaimOK compares the applied
	// value's DECLARED return count against the claim, and the contract the
	// entry carries makes that count binding at RET. Everything it cannot
	// promise keeps the island and the guard closure's runtime count, unchanged.
	if ent := vc.dynApplyEnter(fnVal, args); ent != nil && dynMethodClaimOK(ent, spec.NOut) {
		return stack[:base], ent, nil
	}
	if results, ran, err := vc.dynApplyForeign(fnVal, args, spec.NOut); ran {
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, reg)
		}
		return guard(results)
	}
	// A MODIFIER WRAPPER resolves to what it wraps, exactly as callDynamic
	// resolves one (the chain walk and the permutation are the same): a
	// def-bound `FnUtil.flip` wrapper over a compiled user fn enters that
	// fn's unit with the args reversed instead of islanding the wrapper's
	// token re-dispatch. Both tiers retry against the unwrapped value under
	// the same claim discipline as above.
	if inner, reverse, wrapped := core.UnwrapModifierChain(fnVal); wrapped {
		iargs := args
		if reverse {
			iargs = make([]core.Value, n)
			for i := range args {
				iargs[i] = args[n-1-i]
			}
		}
		if ifd, isFn := inner.Data.(core.FnDefInfo); isFn && vmNativeApplicable(vc.r, ifd) {
			if results, done, err := vc.tryNativeFnApply(inner, iargs); done {
				if err != nil {
					return nil, nil, stampAt(err, curDebug, pc, reg)
				}
				return guard(results)
			}
		}
		if ent := vc.dynApplyEnter(inner, iargs); ent != nil && dynMethodClaimOK(ent, spec.NOut) {
			return stack[:base], ent, nil
		}
		if results, ran, err := vc.dynApplyForeign(inner, iargs, spec.NOut); ran {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, reg)
			}
			return guard(results)
		}
	}
	island := make([]core.Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	return guard(results)
}

// dynMethodClaimOK reports whether an Apply-kernel entry agrees with the
// shaped-method claim's RESULT half (DynMethodSpec.NOut).
//
// The claim has to be discharged BEFORE the entry, because a frame push has no
// results to count. What makes a static answer sound is the contract the entry
// CARRIES (applyRetContract): the frame's RET enforces exactly those declared
// returns, so a frame entered under a matching claim pushes exactly NOut values
// or raises the interpreter's own return error. Note the count is read off the
// APPLIED VALUE's contract, not the entered unit's — a stamped fn-value unit
// declares no returns of its own, which is the whole reason the contract has to
// ride along.
//
// A sig with NO declared return has no count to promise and declines, keeping
// the island and the guard closure's runtime count.
func dynMethodClaimOK(ent *dynEnter, nout int) bool {
	return len(ent.retFn.Returns) > 0 && len(ent.retFn.Returns) == nout
}

// callDynamicMixed handles the MIXED fn-value-call boundary (`3 m.f 2`): a
// dynamic / fn value sits INTERIOR to a window of static args (some below it,
// some above). The window of `w` stack values is the same token sequence the
// interpreter ran, so islanding it verbatim reproduces the interpreter exactly:
// the fn auto-applies — forward-collecting the after-args into its leading sig
// positions and the before-args from the stack — for whatever arity it turns
// out to have, and a non-callable value simply stays put (the island Run leaves
// it on the stack). The leading / trailing OpCallDynamic layouts cannot express
// this because the args straddle the fn.
func (vc *vmContext) callDynamicMixed(reg *core.Registry, w int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	if w < 1 || len(stack) < w {
		return nil, vmErrAt(curDebug, pc, "CALL_DYNAMIC_MIXED underflow")
	}
	base := len(stack) - w
	window := append([]core.Value(nil), stack[base:]...)
	// A STEPLESS window is its own residual. The island is here because the
	// COMPILER could not rule out a callable value interior to the window; when
	// the runtime values turn out to be plain data, the interpreter places every
	// one of them and hands the window straight back, so running it is an
	// interpreter entry inside a compiled program that changes nothing.
	//
	// `1 2 3 do [7] error [drop 9] add 1` is the shape: both bodies compile to
	// closures, the arithmetic runs native, and the window the mixed apply
	// islands is [1 2 3 8] — four literals.
	if core.IsSteplessWindow(window) {
		return stack, nil
	}
	// A window that is INERT DATA under a single TRAILING fn is the trailing
	// apply, not a general re-step — `10 3 m.s/s` lowers here, and the island
	// it runs is `Run([10 3 fn])`, which places the two literals and then steps
	// the fn. The fn collects them TOP-DOWN (the top value fills its first
	// param), which is callDynTrailTop's binding, so the same window handed to
	// tryNativeFnApply in that order answers identically with no sub-engine.
	//
	// The conditions are all load-bearing. IsSteplessWindow over the PREFIX is
	// what rules out a second callable interior to the window — the very thing
	// the compiler could not rule out, which is why this op exists. The fn must
	// pass vmNativeApplicable for the reasons that gate states (no boru body,
	// not reshaped, the live name still native). And a decline from
	// tryNativeFnApply — no overload takes exactly this many args — falls
	// through to the island, which places the leftovers as the interpreter does.
	if n := len(window); n >= 2 {
		fnVal := window[n-1]
		if fnDef, isFn := fnVal.Data.(core.FnDefInfo); isFn && !fnVal.Quoted &&
			core.IsAppliableFn(fnVal) && vmNativeApplicable(vc.r, fnDef) &&
			core.IsSteplessWindow(window[:n-1]) {
			args := make([]core.Value, n-1)
			for i := range args {
				args[i] = window[n-2-i]
			}
			// Pass the fn VALUE, not its FnDefInfo: tryNativeFnApply anchors a
			// raising handler on the value's own position (NUR113), which is
			// what keeps this lane's diagnostics identical to the island's.
			if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
				if err != nil {
					return nil, stampAt(err, curDebug, pc, reg)
				}
				return append(stack[:base], results...), nil
			}
		}
	}
	results, err := vc.islandRun(reg, window)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "dynamic result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, err
	}
	return append(stack[:base], results...), nil
}

// closureRetAt reads the callback return contract keyed at a PUSH_CLOSURE's
// pc in the table of the code that holds it: the main program's for the
// top-level code (unit < 0), the unit's own otherwise — the pc spaces
// overlap, so the table must be the emitting code's (CompiledFn.ClosureRet).
func closureRetAt(p *compiler.Program, unit, pc int) (compiler.ClosureRetSpec, bool) {
	if unit < 0 {
		spec, has := p.ClosureRet[pc]
		return spec, has
	}
	spec, has := p.Fns[unit].ClosureRet[pc]
	return spec, has
}

// callDynFrame replays the CURRENT frame's end-of-body residual through a
// nested interpreter run — the whole-frame dynamic-apply window
// (OpCallDynFrame). The top w stack entries are the TOKEN region (the values
// the interpreter's pointer would step: unapplied fn reads and the args after
// them); everything between the frame base and the token region is the
// RESOLVED prefix (the frame-bottom unnamed-param re-pushes, which the
// interpreter never steps — arguments are inert). RunResolved starts stepping
// after the prefix, so an fn value in the token region auto-dispatches by
// execFnDefLiteral's own runtime rule — forward-collecting token-region values
// and stack-collecting prefix values below, for whatever name/arity the value
// turns out to have — and a non-callable value stays data. The run's residual
// replaces the whole frame region; the following RET applies the fn's return
// discipline (checkReturnContract, RetReplay).
func (vc *vmContext) callDynFrame(reg *core.Registry, w, frameBase int, stack []core.Value, curDebug []core.SrcPos, pc int, words []compiler.DynFrameWord) ([]core.Value, *dynEnter, error) {
	if w < 1 || len(stack)-frameBase < w {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_FRAME underflow")
	}
	base := len(stack) - w
	prefix := append([]core.Value(nil), stack[frameBase:base]...)
	tokens := append([]core.Value(nil), stack[base:]...)
	// The Apply kernel reaches the replay window when the token region is a fn
	// followed by plain data. The region is then exactly [fn, args…] — the same
	// thing CALL_DYNAMIC's leading form hands dynApplyEnter, and RunResolved
	// would have auto-applied it in written order, which is the order the frame
	// binds. A token region carrying a SECOND fn or a tape-coupled token keeps
	// the island: the interpreter re-steps those and a frame push cannot.
	//
	// A NON-EMPTY prefix is admitted only when the callee is ALL-FORWARD. The
	// prefix is the frame-bottom unnamed-param re-push, and a barrier'd callee
	// STACK-collects from it as well as forward-collecting the token region
	// (callDynFrame's own contract above), so the arg set a frame push would
	// bind is not the arg set the interpreter assembles. All-forward, it cannot
	// reach the prefix at all — dynApplyEnter has already established that the
	// token args exactly fill its params — so the prefix survives underneath
	// and the unit's result lands on top of it, which is the residual the island
	// returns. Hence stack[:base], not stack[:frameBase]: the two coincide only
	// when the prefix is empty.
	if len(tokens) > 0 && dynFrameSimpleWindow(tokens) && !replayLeadParks(tokens[0], words) {
		if ent := vc.dynApplyEnter(tokens[0], tokens[1:]); ent != nil && (len(prefix) == 0 || ent.allForward) {
			return stack[:base], ent, nil
		}
		if len(prefix) == 0 {
			if results, ran, err := vc.dynApplyForeign(tokens[0], tokens[1:], 0); ran {
				return vc.dynForeignResults(results, err, stack, base, "dynamic result", curDebug, pc, reg)
			}
		}
		// A LONE fn token over a non-empty prefix collects every parameter
		// from the prefix, top-down — the interpreter's re-step of a member
		// read at a body's tail (`each [ops.inc] xs`, the quotation-body
		// container reads: the element beneath is its one argument). With no
		// token after the fn its forward window is empty whatever the
		// barrier, so a frame push over the prefix's top n values, top first,
		// binds exactly what the re-step binds; a single-signature callee
		// makes n definite. A no-match keeps the island, which parks the
		// value as the interpreter does.
		if len(tokens) == 1 && len(prefix) > 0 {
			if n, ok := vc.loneTokenArity(tokens[0]); ok && n >= 1 && n <= len(prefix) {
				res, err, ran := vc.invokeLoneToken(reg, tokens[0], prefix[len(prefix)-n:])
				if err != nil {
					return nil, nil, stampAt(err, curDebug, pc, reg)
				}
				if ran {
					return append(stack[:base-n], res...), nil, nil
				}
			}
		}
	}
	// A region carrying BARE READS of fn-valued bindings re-steps them as
	// the WORDS the interpreter dispatches (NUR123, vm_dyn_words.go) — after
	// the Apply kernel above, whose frame push IS the word dispatch when the
	// lead matches (a match is a match under either semantics) and costs no
	// interpreter entry; the words island takes what it declines (a 0-arg
	// fn at a non-lead entry, a no-match, a callee with no compiled unit).
	// A region whose reads hold plain data at run time is the residual as
	// it stands and skips the island.
	if len(words) > 0 {
		if st, handled, err := vc.callDynFrameWords(reg, words, frameBase, base, stack, curDebug, pc); handled {
			return st, nil, err
		}
	}
	results, err := runIslandResolved(reg, prefix, tokens)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "dynamic frame result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the replay island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, nil, err
	}
	return append(stack[:frameBase], results...), nil, nil
}

// replayLeadParks is the interpreter's ANONYMOUS-0-ARG PARK (execFnDefLiteral)
// over the replay's lead: the pointer re-steps a lambda or macro VALUE whose
// only signatures take nothing, and it stays DATA unless an application was
// asked for (`f/v apply` marks it Applied) — whatever unit its 0-arg
// signature carries. The Apply kernel below entered that unit: the replay of
// `each ([kv:Any] => [kv.v]) {x: ([] => [5])}` answered {x:5} for the
// interpreter's {x:fn} (NUR220); the island it falls to parks the value. A
// lead the body read BARE BY NAME (words[0]) is the binding's WORD
// dispatch, which fires a 0-arg fn whatever its origin (`def r (mk)  r` is
// 42), so it is not parked.
func replayLeadParks(lead core.Value, words []compiler.DynFrameWord) bool {
	fd, ok := lead.Data.(core.FnDefInfo)
	if !ok || (len(words) > 0 && words[0].Name != "") || !core.FnValueOnlyZeroArgSigs(fd) {
		return false
	}
	return (fd.Anonymous && !fd.Applied) || fd.Macro
}

// loneTokenArity is the parameter count a stack-collecting re-step of a
// lone fn token is certain to bind: a fn VALUE with exactly one own
// signature (that signature's), or a compiled CLOSURE whose unit is known
// (its params less its captures). An overloaded fn matches by signature
// order against whatever the stack holds, which a fixed n cannot
// reproduce, so it declines.
func (vc *vmContext) loneTokenArity(v core.Value) (int, bool) {
	if cl, ok := v.Data.(core.ClosurePayload); ok {
		fn, known := vc.closureUnit(cl)
		if !known {
			return 0, false
		}
		return fn.NParams - fn.NCaptures, true
	}
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return 0, false
	}
	own := fd.OwnSigs()
	if len(own) != 1 {
		return 0, false
	}
	return own[0].TotalArgs(), true
}

// invokeLoneToken applies a lone fn token over inputs taken from the
// frame's resolved prefix, in STACK order (deepest first) — the TOKEN seam's
// own convention, which matches the value top-down exactly as the
// interpreter's re-step collects. A fn VALUE goes through invokeFnValue
// (its compiled unit hosted whatever program stamped it, a foreign-home
// module fn through its home's callback seam, the value's own return
// contract enforced); a compiled CLOSURE through the token seam's closure
// arm. ran is false when nothing matched and the island must decide.
func (vc *vmContext) invokeLoneToken(reg *core.Registry, fnVal core.Value, inputs []core.Value) ([]core.Value, error, bool) {
	if cl, ok := fnVal.Data.(core.ClosurePayload); ok {
		return vc.invokeFnValueClosure(reg, fnVal, cl, inputs)
	}
	return vc.invokeFnValue(reg, fnVal, inputs)
}

// dynFrameSimpleWindow reports whether a replay token region is a fn followed
// by plain data — the one shape the Apply kernel can enter as a frame.
//
// Everything after the lead must be INERT: a second appliable fn would collect
// its own neighbours when the interpreter re-stepped the region, and a
// tape-coupled token (Word/Mark/Move/Forward/OpenParen/Splice) is re-stepped by
// definition. A frame push does neither, so either one keeps the island.
func dynFrameSimpleWindow(tokens []core.Value) bool {
	if !core.IsAppliableFn(tokens[0]) || tokens[0].Quoted {
		return false
	}
	for _, t := range tokens[1:] {
		if core.IsAppliableFn(t) || tapeCoupled([]core.Value{t}) {
			return false
		}
	}
	return true
}

// escapedFlow reports (and clears) a break/continue signal that escaped an
// island apply. A value-dispatched body that breaks/continues with no
// enclosing loop on the ISLAND's own tape returns cleanly with the registry
// FlowCtrl flag still set (Engine.exitWithFlowCtrl's sub-engine contract) so
// an OUTER run can resolve it — in the interpreter that outer run is the
// shared tape holding the enclosing `for`; here it is the VM, which translates
// the flag to the cross-frame OpFlowBreak / OpFlowContinue unwind (flowSignal:
// nearest open loop in any frame; none at all defers to the interpreter, which
// raises the canonical flow_error). Checked on every registry an island apply
// may have run against (vc.r, and the active unit's curReg).
func (vc *vmContext) escapedFlow(regs ...*core.Registry) compiler.Opcode {
	for _, reg := range regs {
		if reg == nil {
			continue
		}
		switch reg.FlowCtrl {
		case core.FlowBreak:
			reg.FlowCtrl = core.FlowNone
			return compiler.OpFlowBreak
		case core.FlowContinue:
			reg.FlowCtrl = core.FlowNone
			return compiler.OpFlowContinue
		}
	}
	return 0
}

// tryNativeFnApply dispatches a Function VALUE VM-native when it resolves to a
// handler-bearing signature (a trivial-delegation native — a method field like
// rand-int): MatchSignature over the dispatchable signatures picks the
// overload the interpreter would, and the handler runs directly. done is false
// when the fn has a non-trivial (user) body that needs the interpreter — the
// caller then islands. The island stays the correctness backstop, so any
// divergence from this fast path is caught by the differential gate.
// vmNativeApplicable reports whether a runtime Function VALUE can be applied by
// tryNativeFnApply — directly, on this VM — instead of through an island.
//
// Two shapes qualify, for ONE reason: neither has a boru body to run in a
// frame. A trivial-delegation wrapper passes through to an inner native
// (`rand-int`, `MathUtil.sqrt`); a parked native word reference IS one
// (`add/v`, and the same value read back out of a map). A user fn is excluded
// because its registered handler is InstallFnDef's body splicer, which expects
// the dispatch frame the interpreter builds around it.
//
// The gate used to name delegation alone, and the omission was the census's
// largest single cluster: 13 path-modifier.tsv rows, every one the shape
// `def m {a:add/v}  m.a 1 2`, islanding for want of this line.
func vmNativeApplicable(r *core.Registry, fd core.FnDefInfo) bool {
	if core.IsDelegationFnDef(fd) {
		return true
	}
	// A SELF-CONTAINED Go-impl fn value (fn-util's produced wrappers) applies
	// on its OWN signatures — the interpreter's anonymous-value rule. It is
	// admitted HERE, ahead of the parked-native arm below: that arm resolves
	// by NAME through the live registry, and such a value's Name is a label
	// that may coincide with a registered word (`const`, the singleton-type
	// maker), which is how `((FnUtil.const 7) 99)` compiled to 99 for the
	// interpreter's 7. The def-read spelling's compile failure blamed the island for
	// that answer; the island in fact applies the value exactly as the
	// interpreter does. tryNativeFnApply keys on the same predicate.
	if core.IsSelfContainedGoFnDef(fd) {
		return true
	}
	// tryNativeFnApply dispatches a parked native through the LIVE registry
	// sigs, so admit one only while those sigs still describe this value. A
	// modifier wrapper keeps the wrapped word's Name but rewrites its
	// signatures, and its Go handler expects the engine's collection around it
	// — the same reason a user fn is excluded. Those island.
	return core.IsNativeWordFnDef(fd) && !fd.ArgsReversed &&
		core.RegisteredWordIsNative(r, fd.Name)
}

func (vc *vmContext) tryNativeFnApply(fnVal core.Value, args []core.Value) ([]core.Value, bool, error) {
	fnDef, isFn := fnVal.Data.(core.FnDefInfo)
	if !isFn { //covergate:allow compiler/VM defensive arm; every caller gates on the same assertion (§compiler)
		return nil, false, nil
	}
	reg, _ := core.FnHome(vc.r, &fnDef)
	var sigs []core.Signature
	if core.IsSelfContainedGoFnDef(fnDef) {
		// The stable handle: its own sigs, never its name (vmNativeApplicable).
		sigs = fnDef.Signatures
	} else if inner := reg.Lookup(fnDef.Name); inner != nil {
		sigs = inner.Signatures
	} else if len(fnDef.Signatures) > 0 {
		sigs = fnDef.Signatures
	}
	if len(sigs) == 0 {
		return nil, false, nil
	}
	mr := core.MatchSignature(sigs, args, core.WordInfo{ArgCount: len(args)})
	if mr == nil || mr.Sig == nil || mr.Sig.DispatchHandler() == nil {
		return nil, false, nil
	}
	// A BORU-BODIED overload resolved in a FOREIGN sub-registry (a module-
	// preamble fn reached through its /v delegation export, e.g. Repl.serve)
	// must not run its body-splicing handler against the dispatching
	// registry: the body's words resolve in the module's own scope
	// (module-private helpers), which only the interpreter's foreign-wrapper
	// branch (execFnDefLiteral engine.go:4356) provides via match.Reg.
	// Decline so the caller islands — the island applies the value through
	// that branch, module scope and all. Go-handler natives stay on this
	// fast path: they read HOST state from vc.r and never resolve body words.
	if _, isBoru := mr.Sig.Impl.(*core.BoruImpl); isBoru && core.FnHomeForeign(vc.r, &fnDef) {
		return nil, false, nil
	}
	// The handler runs against the DISPATCHING registry (vc.r), not the
	// fn's owning sub-registry: the interpreter's execMatch passes
	// e.registry (the engine the value dispatched on) to every native
	// handler, and the island backstop equally runs on vc.r — so host
	// state read through r (the clock, policy, output, context stack)
	// resolves identically on all three paths. Name RESOLUTION stays in
	// fnDef.Registry above, mirroring execFnDefLiteral's lookup. (The
	// prior form passed the sub-registry, which silently dropped
	// host-installed state — a frozen clock stamped wall time on the
	// compiled fast path only; caught by TestShapedMethodEffectOrdering.)
	results, err := mr.Sig.DispatchHandler()(mr.Args, vc.r.Contexts.TopData(), nil, vc.r)
	// Anchor a raising handler on the fn VALUE, which is where the interpreter
	// anchors it. This lane replaced an island, and the island got the position
	// for free: its nested Run stamped the dispatching token. The direct call
	// has no token — the opcode's own debug entry is 0:0 for anything lowered
	// from a dot chain (NUR113) — so without this the caret is simply lost:
	//
	//	def m {d:div/v} end  m.d 0 10
	//	  interpreted  --> 1:10   (the `div` token inside the map literal)
	//	  compiled     --> source position unknown
	//
	// 1:10 is the fn value's OWN parse-stamped position, not the call site's,
	// which is exactly what fnVal carries here. Measured as a regression this
	// lane introduced against the island it replaced.
	if err != nil {
		err = stampFnValuePos(err, fnVal)
	}
	return results, true, err
}

// stampFnValuePos gives a handler error the position of the FUNCTION VALUE that
// dispatched it, when the error carries none of its own. Errors that already
// know where they happened are left alone, and a value with no position (one
// the compiler synthesised rather than read from source) changes nothing.
// Written without an early return on purpose. The guard arms here are not
// unreachable — an error that already carries a row, or a synthesised value
// with no position, are both ordinary — so a `//covergate:allow` on them would
// be a claim the code cannot support, and the gate fails a pragma that stops
// excluding anything. Nesting the conditions instead leaves every statement on
// the path the corpus actually takes.
func stampFnValuePos(err error, fnVal core.Value) error {
	if ae, ok := err.(*core.BoruError); ok && ae.Row == 0 {
		if p := fnVal.Pos(); p.Row != 0 {
			ae.Row, ae.Col = p.Row, p.Col
			// The token's text rides along too, as the interpreter's
			// stampErrPos carries it: the caret's width is the token's. A
			// handler error built with the WORD as its source text
			// (`r.BoruError(code, detail, "FnUtil.compose")`) would otherwise
			// underline fourteen characters where the interpreter, having
			// dispatched the def-bound wrapper under its one-letter name,
			// underlines one (the thirty-fifth increment).
			if p.Src != "" {
				ae.Src = p.Src
			}
		}
	}
	return err
}

// runFallback executes one interpreter island (OpFallback): it preloads the
// NIn threaded inputs (deepest-first) then the recorded span tokens onto a
// reused sub-engine, runs it, and returns the operand stack with the island's
// residual pushed. break/continue/return raised across the boundary propagate
// via the shared registry FlowCtrl, as in any nested Run. (Deleted in plan
// P7 once every shape compiles natively.)
func (vc *vmContext) runFallback(reg *core.Registry, fb *core.FallbackSpan, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	if len(stack) < fb.NIn {
		return nil, vmErrAt(curDebug, pc, "FALLBACK underflow at "+fb.Desc)
	}
	if fb.NIn > 1 {
		// The lowerer threads only 0 or 1 input into an island (lower.go declines
		// >1): a multi-input island would preload the threaded values bottom→top,
		// the OPPOSITE of the interpreter's top-down collection (the same
		// inversion that bounds OpCallDynamicTrailing to arity 1). Assert it so a
		// future lowering bug degrades to a loud internal_error → whole-program
		// fallback rather than silently mis-ordering the island's inputs.
		return nil, vmErrAt(curDebug, pc, "FALLBACK threads >1 input at "+fb.Desc)
	}
	island := make([]core.Value, 0, fb.NIn+len(fb.Tokens))
	island = append(island, stack[len(stack)-fb.NIn:]...)
	island = append(island, fb.Tokens...)
	stack = stack[:len(stack)-fb.NIn]
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "island result at "+fb.Desc, curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, err
	}
	return append(stack, results...), nil
}

// gateWord consults the engine word policy before a compiled NAMED dispatch
// — the VM twin of the interpreter's policyGateWord (engine.go): the same
// checker object raises the same error, so a denied word fails identically
// on either engine. Internal markers are exempt exactly as there; check mode
// never runs on the VM, so that skip has no twin here.
func (vc *vmContext) gateWord(curReg *core.Registry, name string) error {
	vc.refreshGates(curReg)
	if vc.gateWC == nil || core.IsInternalMarker(name) {
		return nil
	}
	if err := vc.gateWC.CheckWord(name); err != nil {
		return core.PolicyDenied{Err: err}
	}
	return nil
}

// refreshGates re-resolves the cached policy checkers when the dispatch
// registry changed (a foreign-unit switch). One lookup refreshes both
// the word and the module-call checker — they live on the same
// CapPolicy slot.
func (vc *vmContext) refreshGates(curReg *core.Registry) {
	if vc.gateReg != curReg {
		vc.gateReg, vc.gateWC, vc.gateMC = curReg, core.LookupWordChecker(curReg), core.LookupModuleCallChecker(curReg)
	}
}

// gateModuleCall consults the per-export module policy before a compiled
// module-export dispatch — the VM twin of the interpreter's
// policyGateModuleCall (NUR045): the same checker object raises the same
// error, so a denied export fails identically on either engine. gate is
// the ModuleCallID stamped onto the dispatched signature (or derived
// from the unit's owning registry for CALL_USER); nil allows in one
// pointer test. Check mode never runs on the VM, so that skip has no
// twin here — exactly as in gateWord.
func (vc *vmContext) gateModuleCall(curReg *core.Registry, gate *core.ModuleCallID) error {
	if gate == nil {
		return nil
	}
	vc.refreshGates(curReg)
	if vc.gateMC == nil {
		return nil
	}
	if err := vc.gateMC.CheckModuleCall(gate.Module, gate.Export); err != nil {
		return core.PolicyDenied{Err: err}
	}
	return nil
}

// gateNamedCall is the shared prologue of the two INLINE named-dispatch case
// arms (CALL_NATIVE, CALL_USER): the underflow check and the word-policy
// gate in one branch, so the gate adds no cognitive weight to vm.run (the
// helper-based dispatches gate inside their helpers instead).
func (vc *vmContext) gateNamedCall(curReg *core.Registry, word string, have, need int, ufMsg string, curDebug []core.SrcPos, pc int) error {
	if have < need {
		return vmErrAt(curDebug, pc, ufMsg+word)
	}
	return vc.gateWord(curReg, word)
}

// bindDynScopeMode executes one OpBindDynScope / OpBindDynScopePeek: install
// the top value under the name for dynamic-scope readers (OpLookupDynScope),
// through the same installer the interpreter's `def` runs; record the prior
// depth so the frame's RET (or the error unwind) truncates the binding stack
// back. The pop is the choice: OpBindDynScope pops the value it installs
// (the lowering pushed a copy for the install), OpBindDynScopePeek leaves it
// in place (the value is live on the sim for its downstream readers).
func (vc *vmContext) bindDynScopeMode(curReg *core.Registry, p *compiler.Program, arg int, stack []core.Value, curDebug []core.SrcPos, pc int, pop bool) ([]core.Value, error) {
	if len(stack) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, vmErrAt(curDebug, pc, "BIND_DYN_SCOPE underflow")
	}
	name, nerr := p.Consts[arg].AsConcreteString()
	if nerr != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, vmErrAt(curDebug, pc, "BIND_DYN_SCOPE bad name const")
	}
	// Ascription hygiene: a stored binding holds the REAL value.
	v := core.StripAscribed(stack[len(stack)-1])
	vc.dynBinds = append(vc.dynBinds, dynBindEntry{reg: curReg, name: name, depth: curReg.Defs.Depth(name)})
	if _, isClosure := v.Data.(core.ClosurePayload); isClosure {
		// A compiled CLOSURE value (a factory's result bound by a fn-body
		// `def a5 (mk 5)`): the installer's carrier guard installs NOTHING
		// for a Function-family value without an FnDefInfo payload, so the
		// frame's bind left no entry and a read of the name — the island's
		// word dispatch of a raw body, the unit's live lookup — missed it
		// (NUR192: `each [a5] xs` inside the fn raised a false
		// `undefined word: a5`). Push it as the top-level write-back does
		// (bindGlobal): the interpreter dispatches the pushed closure through
		// the compiled-runtime hooks, and the trail's depth truncation
		// (unwindDynBinds) pops it with the frame, the interpreter's own
		// def-cleanup.
		curReg.Defs.Push(name, v)
	} else {
		core.InstallDef(curReg, name, v)
	}
	if !pop {
		return stack, nil
	}
	return stack[:len(stack)-1], nil
}

// deoptIfFn executes one OpDeoptIfFn (compiler.DeoptSpec, NUR123). The
// guarded read is a bare read of a body-local the pass typed dynamic(Any);
// the interpreter dispatches the binding as a WORD when it holds a fn at
// run time, where this unit pushed the value. So when the value IS a fn the
// rest of the body goes to the interpreter: the frame region below the
// read (the read's own stack entry, if it has one, dropped — the
// interpreter's frame never held it) is the resolved prefix, the unit's
// Body tokens from the statement's token on are the region — followed by
// the frame's def-cleanup duty over the frame's live bindings, so a def
// the island makes tears down at its end as the interpreter's __dc does —
// and the
// frame's params and defs are registry-visible (the deopt unit's
// BIND_DYN_SCOPE env). The island's residual replaces the frame region and
// the run loop continues at the unit's RET (its RetReplay discipline).
// Plain data costs the test and nothing else.
func (vc *vmContext) deoptIfFn(reg *core.Registry, fn *compiler.CompiledFn, spec *compiler.DeoptSpec, frameBase int, stack, locals []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, bool, error) {
	if spec.Results > 0 {
		return vc.reStepIfFn(reg, fn, spec, frameBase, stack, locals, curDebug, pc)
	}
	var v core.Value
	at := -1
	if spec.Slot >= 0 {
		if spec.Slot >= len(locals) {
			return nil, false, vmErrAt(curDebug, pc, "DEOPT_IF_FN bad slot")
		}
		v = locals[spec.Slot]
	} else {
		at = len(stack) - 1 - spec.Depth
		if spec.Depth < 0 || at < frameBase {
			return nil, false, vmErrAt(curDebug, pc, "DEOPT_IF_FN underflow")
		}
		v = stack[at]
	}
	if !core.IsAppliableFn(v) {
		return stack, false, nil
	}
	if spec.Bail {
		// A GUARD (DeoptSpec.Bail): the interpreter dispatches this read as
		// a word and no island can take the statement over here, so the
		// slot push the unit lowered would answer wrong. A designed defer,
		// loud (compiledRunError reports it) where it used to be silent
		// (NUR123's `5 j typeof`).
		return nil, false, vmErrAt(curDebug, pc, fmt.Sprintf(
			"gradual read `%s` holds a fn the interpreter dispatches here and the unit could not re-step (NUR123)", spec.Name))
	}
	if spec.Token < 0 || spec.Token >= len(fn.Body) || spec.RetPC < 0 {
		return nil, false, vmErrAt(curDebug, pc, "DEOPT_IF_FN bad table entry")
	}
	prefix, err := deoptPrefix(spec, frameBase, len(stack), stack, locals, curDebug, pc)
	if err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, false, err
	}
	if at >= 0 {
		i := at - frameBase + len(spec.Prefix)
		prefix = append(prefix[:i], prefix[i+1:]...)
	}
	tokens := append([]core.Value(nil), fn.Body[spec.Token:]...)
	// The frame's def-cleanup duty, done by hand: a def the island makes
	// tears down at its end, as the interpreter's __dc marker would (the
	// marker itself is a frame-tape token, not an island residual).
	snapshot := reg.Defs.Snapshot()
	results, err := runIslandResolved(reg, prefix, tokens)
	core.TruncateFrameDefs(reg, snapshot)
	if err != nil {
		return nil, false, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "deopt result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, false, err
	}
	return append(stack[:frameBase], results...), true, nil
}

// reStepIfFn executes a RE-STEP deopt (compiler.DeoptSpec.Results, NUR124).
// The Results values on top are what a native call just left, and what the
// interpreter would splice back onto the tape and step: an unquoted fn
// among them dispatches where it lands, collecting forward over the tokens
// written after the call and then from the stack beneath it. So when one
// of them would dispatch at the pointer — the interpreter's own predicate,
// core.FnValueDispatchesAtPointer — the island runs exactly that: the
// results as its first TOKENS, the unit's body from the resume token after
// them, over the frame region beneath the results as the resolved prefix;
// the residual replaces the frame region and the run loop continues at the
// unit's RET. Plain results cost the test and nothing else.
func (vc *vmContext) reStepIfFn(reg *core.Registry, fn *compiler.CompiledFn, spec *compiler.DeoptSpec, frameBase int, stack, locals []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, bool, error) {
	top := len(stack) - spec.Results
	if top < frameBase {
		return nil, false, vmErrAt(curDebug, pc, "DEOPT_IF_FN underflow")
	}
	hot := false
	for _, v := range stack[top:] {
		if core.FnValueDispatchesAtPointer(v) {
			hot = true
			break
		}
	}
	if !hot {
		return stack, false, nil
	}
	if spec.Token < 0 || spec.Token > len(fn.Body) || spec.RetPC < 0 {
		return nil, false, vmErrAt(curDebug, pc, "DEOPT_IF_FN bad table entry")
	}
	prefix, err := deoptPrefix(spec, frameBase, top, stack, locals, curDebug, pc)
	if err != nil {
		return nil, false, err
	}
	tokens := append(append([]core.Value(nil), stack[top:]...), fn.Body[spec.Token:]...)
	// The frame's def-cleanup duty, as deoptIfFn does it: a def the island
	// makes tears down at its end.
	snapshot := reg.Defs.Snapshot()
	results, err := runIslandResolved(reg, prefix, tokens)
	core.TruncateFrameDefs(reg, snapshot)
	if err != nil {
		return nil, false, stampAt(err, curDebug, pc, reg)
	}
	if err := vc.screenResults(results, "deopt result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, false, err
	}
	return append(stack[:frameBase], results...), true, nil
}

// deoptPrefix builds a deopt island's resolved prefix — the interpreter's
// frame at the point: the unnamed params the unit has not pushed yet
// (spec.Prefix, the frame's stack bottom), then the frame region below top.
func deoptPrefix(spec *compiler.DeoptSpec, frameBase, top int, stack, locals []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	prefix := make([]core.Value, 0, len(spec.Prefix)+top-frameBase)
	for _, s := range spec.Prefix {
		if s < 0 || s >= len(locals) {
			return nil, vmErrAt(curDebug, pc, "DEOPT_IF_FN bad prefix slot")
		}
		prefix = append(prefix, locals[s])
	}
	return append(prefix, stack[frameBase:top]...), nil
}

// bindGlobal executes one OpBindGlobal — the cross-request persistence twin
// of a top-level computed `def`: PEEK the runtime value (the stack is
// untouched, so the lowering's fast path binds a value in place without
// disturbing its downstream consumers; the copy path emits its own OpDrop)
// and write it into the KEPT check-pass binding slot (SetAt replaces in
// place — never a push — so shadow depth and undef behaviour match the
// interpreter). A slot a later check-time undef popped skips the write: the
// interpreter would have discarded the binding too.
//
// A write-back paired with a dyn-scope bind of the same def
// (GlobalBindSpec.AfterDynScope) ADOPTS that bind's install instead: the
// entry bindDynScope pushed through the interpreter's own installer is the
// persisted binding, taken off the dyn-bind trail so no unwind pops it, and
// nothing is pushed — one entry under the name, as the interpreter's one
// `def` leaves (NUR168's second finding). The stack is consumed exactly as
// the write would consume it.
func (vc *vmContext) bindGlobal(curReg *core.Registry, gb *compiler.GlobalBindSpec, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	// The write PUSHES — the interpreter's own `def`: the check pass's
	// install was rolled back before the run (core.RestoreBindingsForReplay),
	// so there is no kept slot to overwrite, and the twin table's
	// carrier-class skip guarantees exactly one of {this push, the def's
	// twin} installs (§6.5's rollback-and-replay, the only regime since the
	// flip).
	write := func(v core.Value) {
		curReg.Defs.Push(gb.Name, core.StripAscribed(v))
	}
	if gb.AfterDynScope && vc.adoptDynBind(curReg, gb.Name) {
		write = func(core.Value) {}
	}
	if gb.Splice {
		// The S5 first-value loop bind: the region's first value sits at a
		// static depth below the top — bind it and splice it out, exactly
		// the interpreter's pending-forward collection from the region.
		idx := len(stack) - 1 - gb.SpliceFromTop
		if idx < 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return nil, vmErrAt(curDebug, pc, "BIND_GLOBAL splice underflow")
		}
		write(stack[idx])
		return append(stack[:idx], stack[idx+1:]...), nil
	}
	if len(stack) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, vmErrAt(curDebug, pc, "BIND_GLOBAL underflow")
	}
	// Ascription hygiene on both bind arms: a stored binding holds the REAL
	// value (interpreter def parity). A fn value takes the def's name here
	// too, as installDef names what it binds (NUR168) — on the stack copy
	// as well, so the local store that follows the peek holds the named
	// value the interpreter's binding holds.
	stack[len(stack)-1] = nameClosureValue(stack[len(stack)-1], gb.Name)
	write(stack[len(stack)-1])
	if gb.Pop {
		return stack[:len(stack)-1], nil
	}
	return stack, nil
}

// unwindDynBinds truncates every dynamic-scope binding installed above the
// given trail depth back to its recorded pre-install def-stack depth,
// innermost-first — the compiled twin of the interpreter's per-frame
// def-cleanup (__DC) for OpBindDynScope installs.
// ensureInvoker installs the run's body-closure invoker on a FOREIGN unit's
// dispatch registry (once per registry per run): the unit's native handlers
// run their code bodies through InvokeBody on that registry, exactly as the
// interpreter's CallBoru dispatch does, so the VM seam must be present there
// too. The main registry's invoker is installed by runProgram; the ones added
// here are removed by its deferred cleanup.
func (vc *vmContext) ensureInvoker(reg *core.Registry) {
	if reg == nil || reg == vc.r || reg.Invoker != nil {
		return
	}
	// One shared implementation: the InvokeBody seam passes the CALLING
	// registry, so a compiled closure runs VM-native (its unit carries its
	// own Reg) while a raw TOKEN body's sub-engine fallback resolves names
	// against the caller's scope — a per-connection fork inherits this
	// field and passes itself.
	reg.Invoker = vc.invokeClosureOn
	vc.foreignInvokers = append(vc.foreignInvokers, reg)
}

// bindFnType executes one OpBindFnType (compiler/go/bytecode.go): the front
// door's name check against the run's reservations, the reservation, the
// check-time node bound as an adopted entry on the dyn-bind trail, so the
// unit's RET pops it as it pops the unit's value defs.
func (vc *vmContext) bindFnType(reg *core.Registry, spec *compiler.FnTypeBindSpec) error {
	if err := core.TypeNameFree(reg, spec.Name); err != nil {
		return err
	}
	vc.dynBinds = append(vc.dynBinds, dynBindEntry{reg: reg, name: spec.Name, depth: reg.Defs.Depth(spec.Name)})
	reg.Defs.PushTypeAdopted(spec.Name, spec.Entry.TypeDef, spec.Entry.Body)
	core.ReserveTypeParts(reg, spec.Name)
	return nil
}

// adoptDynBind takes the dyn-bind trail's top entry off the trail when it is
// the install of name on reg — the OpBindDynScope a paired write-back
// (GlobalBindSpec.AfterDynScope) follows — so that install persists as the
// binding the write-back would have pushed. False, and the trail untouched,
// when the top entry is another name's: the write-back then pushes as before.
func (vc *vmContext) adoptDynBind(reg *core.Registry, name string) bool {
	n := len(vc.dynBinds)
	if n == 0 || vc.dynBinds[n-1].reg != reg || vc.dynBinds[n-1].name != name {
		return false
	}
	vc.dynBinds = vc.dynBinds[:n-1]
	return true
}

// polyHasArity reports whether some non-fallback overload takes exactly k
// arguments — the arities the poly seat retries at (callPoly, NUR147).
func polyHasArity(sigs []core.Signature, k int) bool {
	for i := range sigs {
		if !sigs[i].Fallback && sigs[i].TotalArgs() == k {
			return true
		}
	}
	return false
}

func (vc *vmContext) unwindDynBinds(base int) {
	for i := len(vc.dynBinds) - 1; i >= base; i-- {
		e := vc.dynBinds[i]
		e.reg.Defs.Truncate(e.name, e.depth)
	}
	vc.dynBinds = vc.dynBinds[:base]
}

// run executes from startUnit (unit -1 is the main program; >=0 indexes
// p.Fns) with the given frame locals and initial operand stack, returning the
// residual stack when the unit runs off the end of its code (the main program
// — and body closures, which carry no trailing RET). A RET propagates back to
// its CALL_USER caller within this run. Re-entrant: a body closure invoked
// from a native handler calls run() again on a fresh stack, sharing vc's step
// budget and island engine.
//
// arm per opcode; complexity grows by one with each new op (here OpInterp) and is
// not reducible without obscuring the flat decode loop. Same documented exception
// as the engine.go step dispatch and matchSignature (.golangci.yml).
//
// seatConstLocal backs OpPushConstFreshLocal: a multi-read compound body literal
// is constructed ONCE per call into frame local cl.Slot and shared by every read
// site. Frame locals start zero-valued (Parent==nil) and a compound clone always
// carries a Parent, so Parent==nil is an unambiguous "not yet seated this call"
// sentinel — deep-clone on the first read of the call, re-use the seated instance
// after. Interpreter parity for `def x {…}` read at several sites: one instance
// per call, shared within a call, fresh across calls.
func seatConstLocal(p *compiler.Program, locals []core.Value, cl compiler.ConstLocalRef) core.Value {
	if locals[cl.Slot].Parent == nil {
		locals[cl.Slot] = core.CloneValueKeeping(p.Consts[cl.ConstIdx], p.ConstKeep[cl.ConstIdx])
	}
	return locals[cl.Slot]
}

//nolint:gocyclo,gocognit // the VM instruction dispatch is inherently one big switch — one
func (vc *vmContext) run(startUnit int, locals []core.Value, stack []core.Value) (runOut []core.Value, runErr error) {
	p, r := vc.p, vc.r
	ceiling := vc.ceiling
	// Dynamic-scope bindings installed by this activation (and its callees)
	// must never outlive it: a frameless top RET pops back to this depth,
	// and an ERROR unwind — where the RETs never run — restores it here so a
	// failed run (or a closure error a caller traps) leaks nothing into the
	// registry.
	dynBase := len(vc.dynBinds)
	// A KEEP-DEFS body unit (`do`, CompiledFn.KeepsDefs) is the exception
	// both ways: the interpreter runs that body in the caller's frame and
	// its defs leak, so the installs its OWN frame made stay on the trail
	// — past its top RET (below) for the enclosing frame's exit to pop,
	// and past a raise too (the interpreter's leak-then-raise: `do [def t
	// 5 raise 'x']` leaves t bound). Only the frames still open BENEATH
	// the error — a callee mid-body — are torn down then, from the first
	// such frame's entry depth.
	keepDefs := startUnit >= 0 && startUnit < len(p.Fns) && p.Fns[startUnit].KeepsDefs
	var frames []vmFrame
	defer func() {
		if runErr == nil {
			return
		}
		base := dynBase
		if keepDefs {
			if len(frames) == 0 {
				return
			}
			base = frames[0].dynBase
		}
		vc.unwindDynBinds(base)
	}()
	// This activation counts against the shared frame ceiling; restore the
	// entry baseline on exit so sequential runs and error unwinds never leak
	// (the per-CALL_USER increments below are balanced by their RETs, and the
	// reset catches any frames still open on an early error return).
	entryFrameDepth := vc.frameDepth
	vc.frameDepth++
	defer func() { vc.frameDepth = entryFrameDepth }()
	var loops []vmLoop
	// marks is the variadic-region mark stack (OpStackMark / OpDropToMark /
	// OpPopMark): each entry is a saved stack depth so a 0-or-1 (runtime-variadic)
	// value produced above the mark can be discarded by truncation regardless of
	// its actual count. Per-run, like loops/frames.
	var marks []int
	// argScratch is per-run (NOT shared on vc): a re-entrant closure run's
	// CALL_NATIVE must not clobber an outer handler's args slice, which
	// aliases this buffer until the handler returns (a higher-order handler
	// holds its data arg across the InvokeBody call that drives the nested
	// run).
	var argScratch []core.Value
	curUnit := startUnit
	var curCode []compiler.Instr
	var curDebug []core.SrcPos
	// curReg is the ACTIVE unit's dispatch registry: a module-preamble fn's
	// unit (CompiledFn.Reg) runs its natives against the module's own
	// registry — the interpreter's CallBoru does exactly that — so
	// registry-visible handler effects (Net.listen's per-connection forks,
	// dynamic-scope binds) land in module scope on both engines. Ordinary
	// units (Reg nil) run on the program's registry.
	curReg := r
	enterUnit := func(u int) {
		curUnit = u
		curReg = r
		if u < 0 {
			curCode, curDebug = p.Code, p.Debug
		} else {
			curCode, curDebug = p.Fns[u].Code, p.Fns[u].Debug
			curReg = dispatchRegistry(p.Fns[u].Reg, r)
		}
	}
	enterUnit(startUnit)
	pc := 0
	// resolveEscapedFlow translates a break/continue that escaped an island
	// apply (the registry FlowCtrl contract — see escapedFlow) into the
	// cross-frame flow unwind, mutating the run loop's frames/loops/locals/
	// stack/pc in place. Shared by every island-apply opcode case.
	resolveEscapedFlow := func() error {
		fop := vc.escapedFlow(vc.r, curReg)
		if fop == 0 {
			return nil
		}
		var u int
		var err error
		if frames, loops, locals, stack, pc, u, err = vc.flowSignal(fop, frames, loops, locals, stack, pc, curUnit, curDebug); err != nil {
			return err
		}
		enterUnit(u)
		return nil
	}
	for pc = 0; pc < len(curCode); pc++ {
		if len(stack) > ceiling || vc.frameDepth > ceiling {
			return nil, vmExhaustedAt(curDebug, pc, r, ceiling)
		}
		vc.steps++
		if vc.steps > vc.stepLimit {
			return nil, vmEvalLimitAt(curDebug, pc, r, vc.stepLimit)
		}
		in := curCode[pc]
		// Line-coverage seam (coverage.go): the compiled twin of the interpreter
		// step-site emit. noteVMCoverage short-circuits on the coverID field
		// (untagged units — the ordinary case — cost one branch, no atomic load)
		// and is small enough to inline, so the hot loop keeps its complexity.
		curReg.NoteVMCoverage(curDebug, pc)
		// Each op is one dispatch: a predicate verdict memoised by the
		// previous op (RunPredicate, NUR102) must not answer this one.
		r.ClearPredMemo()
		switch in.Op {
		case compiler.OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case compiler.OpPushConstFresh:
			// Mint a fresh container identity for a compound literal the
			// enclosing fn unit re-evaluates per call — interpreter parity
			// for `(mk) eq (mk)` (see OpPushConstFresh in bytecode.go); the
			// enclosing bindings' containers the literal embeds stay shared
			// (Program.ConstKeep).
			stack = append(stack, core.CloneValueKeeping(p.Consts[in.Arg], p.ConstKeep[int(in.Arg)]))
		case compiler.OpPushConstFreshLocal:
			// A multi-read compound body literal: construct ONE fresh instance per
			// call, seated in a frame local, shared by every read site (see
			// OpPushConstFreshLocal / seatConstLocal for the sentinel + parity).
			stack = append(stack, seatConstLocal(p, locals, p.ConstLocals[in.Arg]))
		case compiler.OpPushLocal:
			stack = append(stack, locals[in.Arg])
		case compiler.OpPushLocalBound:
			// A branch-carried binding read past its merge (bytecode.go): the
			// zero slot is the arm that did not run, and the read raises the
			// interpreter's undefined_word for the name seated at this pc.
			if v := locals[in.Arg]; v.IsUnboundSlot() {
				name, _ := storeNameAt(p, curUnit, pc)
				return nil, stampAt(core.UndefinedWordDiagWith(curReg, curReg.Source, name, debugPosAt(curDebug, pc), localNameCandidates(p, curUnit)), curDebug, pc, curReg)
			}
			stack = append(stack, locals[in.Arg])
		case compiler.OpStoreLocal:
			// Pop the producing event's single result into a frame local;
			// each reference re-pushes it via PUSH_LOCAL (value-def locals).
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "STORE_LOCAL stack underflow")
			}
			// Ascription hygiene: a stored binding holds the REAL value
			// (`def y (m as T)` — the interpreter's def strips at arg
			// delivery; the compiled local store is that same boundary).
			stored := core.StripAscribed(stack[len(stack)-1])
			// A produced fn value bound by `def` takes the def's name, as the
			// interpreter's installDef renames it (StoreNames — the
			// twenty-ninth increment).
			if name, ok := storeNameAt(p, curUnit, pc); ok {
				stored = vc.nameStoredClosure(stored, name)
			}
			locals[in.Arg] = stored
			stack = stack[:len(stack)-1]
		case compiler.OpDrop:
			// Discard the top value — the computed else value on the taken
			// (then) path of `if cond [then] (expr)`.
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "DROP stack underflow")
			}
			stack = stack[:len(stack)-1]
		case compiler.OpStackMark, compiler.OpDropToMark, compiler.OpPopMark, compiler.OpCallDynMixedFromMark,
			compiler.OpSeatBelowMark, compiler.OpMakeListToMark:
			var err error
			if marks, stack, err = vc.vmMarkOp(curReg, in.Op, int(in.Arg), marks, stack, curDebug, pc); err != nil {
				return nil, err
			}
		case compiler.OpMakeList:
			// Assemble the top Arg values into a list (a computed list literal,
			// `[1 add 2]`); order preserved, deepest becomes element 0.
			n := int(in.Arg)
			if len(stack) < n {
				return nil, vmErrAt(curDebug, pc, "MAKE_LIST stack underflow")
			}
			elems := make([]core.Value, n)
			copy(elems, stack[len(stack)-n:])
			// Ascription hygiene: list elements are STORED data (mirrors
			// autoEvalList's element strip).
			for i := range elems {
				elems[i] = core.StripAscribed(elems[i])
			}
			stack = stack[:len(stack)-n]
			stack = append(stack, core.NewList(elems))
		case compiler.OpMakeMap:
			// Assemble the top values into a map paired with the spec's keys (a
			// computed make-construction body, `make Outer {i:(make Inner …)}`).
			var err error
			if stack, err = vmMakeMap(p, stack, in.Arg, curDebug, pc); err != nil {
				return nil, err
			}
		case compiler.OpInterp:
			// Assemble a template string from its computed holes (`` `got ${x}` ``).
			var err error
			if stack, err = vmInterp(p, stack, in.Arg, curDebug, pc); err != nil {
				return nil, err
			}
		case compiler.OpSpliceDyn:
			// Spread a runtime splice payload (§9.2b): a DATA payload
			// contributes spliceExpand's values verbatim; a code-bearing or
			// fn-valued one defers — the marker re-step dispatches against
			// the live stack, which only the interpreter owns.
			if len(stack) < 1 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "SPLICE_DYN underflow")
			}
			payload := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			elems := core.SpliceExpand(payload)
			for _, el := range elems {
				if core.IsWord(el) || core.IsParenExpr(el) || core.IsReach(el) || core.IsInterpString(el) || core.IsSplice(el) ||
					core.IsForward(el) || core.IsOpenParen(el) || core.IsCloseParen(el) || core.IsAppliableFn(el) {
					return nil, vmDefer(vc.r, curDebug, pc, "vm:splice-active-payload",
						"splice of a code-bearing payload; the compiled runtime cannot execute it")
				}
			}
			stack = append(stack, elems...)
		case compiler.OpInterpXml:
			// Assemble an interpolated XML element from its computed holes
			// (`<p>${x}</p>`, §9.2c) — the tree twin of OpInterp.
			var err error
			if stack, err = vmInterpXml(p, stack, in.Arg, curDebug, pc); err != nil {
				return nil, err
			}
		case compiler.OpTrap:
			// A check-mode-suppressed runtime error compiled in place: raise the
			// byte-identical boru error (the interpreter errors at this same point),
			// including its full structured diagnostic payload (spans, notes,
			// suggestions) so the compiled report equals the interpreted one.
			tr := &p.Traps[in.Arg]
			src := ""
			if r != nil {
				src = r.Source
			}
			ae := core.MakeBoruError(tr.Code, tr.Detail, tr.Word, src, tr.Hint)
			ae.Spans = tr.Spans
			ae.Notes = tr.Notes
			ae.Suggestions = tr.Suggestions
			return nil, stampAt(ae, curDebug, pc, curReg)
		case compiler.OpDispatchRematch:
			// Terminal either way: the rematch raises or defers (vm_rematch.go).
			return nil, vc.dispatchRematch(&p.Dispatches[in.Arg], stack, curDebug, pc)
		case compiler.OpCollect:
			// The region oracle (region_oracle.go): walks the descriptor live
			// and reports; stack-neutral, and the call after it runs as it
			// would have.
			if int(in.Arg) >= len(p.Regions) {
				return nil, vmErrAt(curDebug, pc, "COLLECT region index out of range")
			}
			if err := vc.collectOracle(p, curCode, pc, &p.Regions[in.Arg], stack, curReg, curDebug); err != nil {
				return nil, err
			}
		case compiler.OpDispatchGeneric:
			// The routed dispatch (vm_generic.go): the descriptor drives a live
			// collection and match; the committed unit is entered exactly as
			// OpCallUserPoly enters a matched arm, a native result lands on
			// the stack, and every other outcome is a designed defer.
			if int(in.Arg) >= len(p.Generics) {
				return nil, vmErrAt(curDebug, pc, "DISPATCH_GENERIC index out of range")
			}
			fb := 0
			if len(frames) > 0 {
				fb = frames[len(frames)-1].stackBase
			}
			ns, unit, sigArgs, err := vc.dispatchGeneric(p, &p.Generics[in.Arg], stack, locals, fb, curReg, curDebug, pc, curUnit)
			if err != nil {
				return nil, err
			}
			stack = ns
			if unit < 0 {
				break
			}
			fn := &p.Fns[unit]
			nl := make([]core.Value, fn.NLocals)
			copy(nl, sigArgs)
			for i := 0; i < fn.NParams && i < len(nl); i++ {
				nl[i] = core.StripAscribed(nl[i])
				if nl[i].Parent.Equal(core.TList) && !nl[i].Quoted {
					nl[i].Quoted = true
				}
			}
			// Live, not symmetry: the plan-time match (PlanMatch, then
			// unitMatchesSig) proves each arg against the unit's declared
			// TYPES, while the contract re-checks the parameter PATTERNS
			// too — a predicate-typed child such as `[:Pos]` — unarmed,
			// the way top-level dispatch does. A predicate body that
			// dispatches over such a pattern reaches this return (lang/go's
			// TestPredicateBodyDispatchIsUnarmedLikeTopLevel pins it on
			// both engines); it used to be admitted only because the
			// contract's plain Unify read an ambient registry stack that
			// another unify had left armed, which the kernel no longer has.
			if err := checkParamContract(r, fn, nl); err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
			frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth()})
			vc.frameDepth++ // balanced by the matching RET, like OpCallUser
			nameFrameFns(curReg, fn, nl)
			vc.pushFrameArgs(nl, fn.NArgs)
			locals = nl
			enterUnit(unit)
			pc = -1
		case compiler.OpPushClosure:
			nc := p.Fns[in.Arg].NCaptures
			if len(stack) < nc {
				return nil, vmErrAt(curDebug, pc, "PUSH_CLOSURE capture underflow")
			}
			var caps []core.Value
			if nc > 0 {
				caps = make([]core.Value, nc)
				copy(caps, stack[len(stack)-nc:])
				stack = stack[:len(stack)-nc]
			}
			cl := core.ClosurePayload{Prog: p, Unit: int(in.Arg), Captures: caps, InShape: p.Fns[in.Arg].InShape, Render: p.Fns[in.Arg].Render, Ident: core.NewFnIdentity()}
			// The CALLBACK fn value's own declared return, keyed by THIS push's
			// pc: the unit is shared across fn values with identical bodies and
			// inputs, so the contract belongs to the value (see
			// core.ClosurePayload.RetTypes).
			v := core.Value{Parent: core.TFunction, Data: cl}
			if spec, has := closureRetAt(p, curUnit, pc); has {
				cl.RetTypes, cl.RetPatterns = spec.Types, spec.Patterns
				cl.RetDecl, cl.RetName, cl.RetPos = spec.Decl, spec.Name, spec.Pos
				cl.Source, cl.Named = spec.Source, spec.Named
				v.Data = cl
				if spec.DefName != "" {
					// A `/v` read of a def-bound capturing literal: the
					// value carries the def's name (the thirty-third
					// increment), as the binding the interpreter reads does.
					v = nameClosureValue(v, spec.DefName)
				}
			}
			stack = append(stack, v)
		case compiler.OpPushType:
			// Resolve the CANONICAL node at run time — never a pooled
			// copy (eng/go/CLAUDE.md, Canonical *Type Pointers). Types
			// the check pass minted (def Foo …) live in the registry's
			// table; kernel builtins in the package Builtin table.
			// The ACTIVE unit's registry first: a module-preamble fn's minted
			// types (def Pos (refine Integer) in the module body) live in the
			// module's own table (CompiledFn.Reg / curReg), not the importer's
			// — exactly where the interpreter's CallBoru resolves them. An
			// ordinary unit has curReg == r, so the second lookup repeats only
			// for the kernel-builtin path below.
			var t *core.Type
			if curReg != nil {
				t = curReg.Types.LookupByID(p.Types[in.Arg].ID)
			}
			if t == nil && r != nil {
				t = r.Types.LookupByID(p.Types[in.Arg].ID)
			}
			if t == nil {
				t = core.Builtin.LookupByID(p.Types[in.Arg].ID)
			}
			if t == nil {
				return nil, vmErrAt(curDebug, pc, "unresolvable type operand "+p.Types[in.Arg].Name)
			}
			// A node the pass minted over a bound only the run knows pushes
			// the node the run installed in its place (OpBindTypeRun, NUR231).
			stack = append(stack, core.NewTypeLiteral(core.ForwardedType(t)))
		case compiler.OpForSetup:
			var err error
			if stack, loops, err = vc.opForSetup(stack, loops, int(in.Arg), curCode, curUnit, pc, curDebug); err != nil {
				return nil, err
			}
		case compiler.OpForNext:
			if len(loops) == 0 {
				return nil, vmErrAt(curDebug, pc, "FOR_NEXT without a loop")
			}
			lp := &loops[len(loops)-1]
			done := lp.cur >= lp.end
			if lp.step < 0 {
				done = lp.cur <= lp.end
			}
			if done {
				loops = loops[:len(loops)-1]
				pc = int(in.Arg) - 1
				continue
			}
			locals[lp.slot] = core.NewInteger(lp.cur)
			lp.cur += lp.step
			// Record this iteration's operand-stack base so a cross-frame
			// break/continue drops exactly the current iteration's partial pushes
			// (completed iterations' results sit below it and survive).
			lp.iterBase = len(stack)
		case compiler.OpSwap, compiler.OpReverse:
			// SWAP is reverse-of-2; OpReverse reverses the top Arg. Shared helper.
			var err error
			if stack, err = vmShuffle(stack, in.Op, int(in.Arg), curDebug, pc); err != nil {
				return nil, err
			}
		case compiler.OpCallNative:
			s := p.Sigs[in.Arg]
			n := s.Sig.TotalArgs()
			if err := vc.gateNamedCall(curReg, s.Word, len(stack), n, "CALL_NATIVE underflow at ", curDebug, pc); err != nil {
				return nil, err
			}
			// Per-export module policy gate (NUR045): a baked module
			// native (`TimeUtil.sleep 800` — the direct compiled route)
			// carries the stamped identity on its recorded sig; the
			// rebound laundering path (`def s TimeUtil.sleep/v  s 300`)
			// baked a stamped copy too, whatever s.Word says.
			if err := vc.gateModuleCall(curReg, s.Sig.ModuleCall); err != nil {
				return nil, err
			}
			// One argument convention: position 0 is the top of stack.
			// Reuse a per-RunProgram scratch buffer instead of allocating
			// an args slice every dispatch — the dominant per-CALL_NATIVE
			// allocation on the compute path. Safe: the handler's result
			// is COPIED into the operand stack by the append below before
			// the next call reuses the buffer, and compiled-reachable
			// natives (the monomorphic math/compare/etc. words the emitter
			// admits) do not retain the args slice. The 0-divergence gate
			// + combination matrix catch any handler that does — and a
			// -tags borudebug build (vmFreshArgsPerCall) allocates fresh per
			// call to localize a violator directly. See vm_args_release.go.
			var args []core.Value
			if vmFreshArgsPerCall { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				args = make([]core.Value, n)
			} else {
				if cap(argScratch) < n {
					argScratch = make([]core.Value, n)
				}
				args = argScratch[:n]
			}
			for i := 0; i < n; i++ {
				// StripAscribed at delivery: the handler receives the REAL
				// value (execMatch parity); the guard below still matches the
				// ascribed view via sigTypeMatches, but a stripped subtype
				// passes every slot its widened view passed, so the strip is
				// order-independent — see design/OPEN-WORDS.1.md §9.
				args[i] = core.StripAscribed(stack[len(stack)-1-i])
			}
			stack = stack[:len(stack)-n]
			// A GUARDED native call (recovered single-overload dispatch the checker
			// could not statically commit): re-check the concrete args against the
			// committed sig — dispatch on a match (== the interpreter's sole-overload
			// dispatch), raise the byte-identical signature_error on a miss (== the
			// interpreter finding no overload). See SigRef.Guard.
			if s.Guard {
				if err := checkNativeParamContract(curReg, &s, args); err != nil {
					return nil, stampAt(err, curDebug, pc, curReg)
				}
			}
			vc.ensureInvoker(curReg)
			results, err := s.Sig.DispatchHandler()(args, curReg.Contexts.TopData(), nil, curReg)
			if err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
			// Belt-and-braces: a handler that returns tape tokens (to
			// be re-stepped by the engine) must never have been
			// compiled — the emitter declines fn-invoking and
			// code-splicing words. Fail loudly, never push tokens as
			// data.
			if err := vc.screenResults(results, "handler result at "+s.Word, curDebug, pc); err != nil {
				return nil, err
			}
			stack = append(stack, results...)
			// A native that ran a code body through the InvokeBody seam can
			// return with the registry's FlowCtrl set — a break/continue the
			// body raised with no loop of its own, handed back by the seam's
			// sub-engine (Engine.exitWithFlowCtrl's contract) or by a hosted
			// token body (vm_token_body.go) for the ENCLOSING run to resolve.
			// The interpreter's run loop reads the flag after every step;
			// this loop read it only after a fallback or a fn-value apply, so
			// `each (mk) [1 2 3]` over a computed `[break]` answered
			// `[[1 2 3]]` for the interpreter's `break outside loop`, and
			// `for 3 [each (mk) xs i]` ran all three iterations for the
			// interpreter's one (NUR195, 2026-09-24). Translated here as after
			// a fallback: the nearest open loop, or the loop-less internal
			// error that defers to the interpreter's canonical raise.
			if err := resolveEscapedFlow(); err != nil {
				return nil, err
			}
		case compiler.OpBindTyped:
			// Typed value-def validate/reparent (the compiled defTypedHandler
			// refinement step): pop the body value, run the SAME membership check
			// the interpreter runs, push the value the interpreter would bind. A
			// failed validation returns the interpreter's byte-identical plain
			// error unstamped (defTypedHandler raises via fmt.Errorf with no
			// position; stampAt only touches BoruErrors, so it is a no-op here and
			// kept purely for uniformity with the other dispatch sites).
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "BIND_TYPED stack underflow")
			}
			// Ascription hygiene: the typed-def bind stores the REAL value
			// (interpreter parity — defTypedHandler's arg arrived stripped).
			spec := &p.TypedBinds[in.Arg]
			var bound core.Value
			var err error
			if spec.ConsOperand {
				// An inline constraint the run computed (NUR231) sits beneath
				// the value: pop both, bind against the run's constraint.
				if len(stack) < 2 {
					return nil, vmErrAt(curDebug, pc, "BIND_TYPED stack underflow")
				}
				cons := stack[len(stack)-2]
				bound, err = core.RunTypedBindCons(r, spec, cons, core.StripAscribed(stack[len(stack)-1]))
				stack = stack[:len(stack)-1]
			} else {
				bound, err = core.RunTypedBind(r, spec, core.StripAscribed(stack[len(stack)-1]))
			}
			if err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
			// Belt-and-braces, like every dispatch site: a value-transforming
			// predicate body could hand back a tape-coupled token; never push one.
			if err := vc.screenResults([]core.Value{bound}, "typed-bind result at "+p.TypedBinds[in.Arg].Name, curDebug, pc); err != nil {
				return nil, err
			}
			stack[len(stack)-1] = bound
		case compiler.OpFallback:
			ns, err := vc.runFallback(curReg, &p.Fallbacks[in.Arg], stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
			// runFallback re-steps the word through the interpreter, which can
			// leave FlowCtrl set for an escaped break/continue (e.g. a fallback
			// `do <computed>` inside a compiled loop) — translate it the same way
			// as the fn-value seam. resolveEscapedFlow is a no-op when no flow
			// signal escaped (escapedFlow returns 0), so call it unconditionally.
			if err := resolveEscapedFlow(); err != nil {
				return nil, err
			}
		case compiler.OpCallNativePoly:
			ns, err := vc.callPolyIn(curReg, &p.PolyRefs[in.Arg], stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
			// The poly re-match's handler runs bodies too (see OpCallNative).
			if err := resolveEscapedFlow(); err != nil {
				return nil, err
			}
		case compiler.OpCallDynamic, compiler.OpCallDynamicTrailing, compiler.OpCallDynamicMixed,
			compiler.OpCallDynTrailTop, compiler.OpCallDynApplyTop, compiler.OpCallDynApplyOne, compiler.OpCallDynTrailKeepQ, compiler.OpCallDynFrame, compiler.OpCallDynMethod,
			compiler.OpReStepLanding:
			// The fn-value-call boundary family: leading / trailing-1
			// (callDynamic), interior-window (callDynamicMixed), fn-on-top
			// (callDynTrailTop / callDynApplyTop) and the whole-frame replay
			// (callDynFrame). callDynFamily routes by opcode so run's dispatch
			// stays a single case; a break/continue that escaped the applied
			// body is then translated to the cross-frame flow unwind, exactly
			// as the interpreter's shared tape resolves it at the enclosing
			// loop.
			fb := 0
			if len(frames) > 0 {
				fb = frames[len(frames)-1].stackBase
			}
			ns, ent, err := vc.callDynFamily(curReg, in.Op, int(in.Arg), fb, stack, curDebug, pc, dynFrameWordsAt(p, curUnit, pc), dynApplyNameAt(p, curUnit, pc), landingWordAt(p, curUnit, pc))
			if err != nil {
				return nil, err
			}
			stack = ns
			if ent != nil && ent.jump {
				// A landing island took the rest of the body (NUR190).
				pc = ent.jumpPC - 1
				break
			}
			if ent != nil {
				// The Apply kernel's frame push, modelled on OpCallUserPoly
				// (the other site that learns its unit at RUN time): re-check
				// the param contract, push a frame, enter. Deliberately NOT a
				// nested run — a fn APPLICATION is a call, and bracketing it as
				// a body added a per-body context frame the interpreter's call
				// does not (vm_dyn_apply.go).
				fn := &p.Fns[ent.unit]
				if err := checkParamContract(r, fn, ent.locals); err != nil {
					return nil, stampAt(err, curDebug, pc, curReg)
				}
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth(), retFn: ent.retFn})
				vc.frameDepth++ // balanced by the matching RET, like OpCallUser
				nameFrameFns(curReg, fn, ent.locals)
				vc.pushFrameArgs(ent.locals, fn.NArgs)
				locals = ent.locals
				enterUnit(ent.unit)
				pc = -1
				break
			}
			if err := resolveEscapedFlow(); err != nil {
				return nil, err
			}

		case compiler.OpJmp:
			t := int(in.Arg)
			// The only legal back-edge is a counted loop's trailing
			// jump to its FOR_NEXT — termination then rides the loop
			// counter.
			if t <= pc && (t < 0 || t >= len(curCode) || curCode[t].Op != compiler.OpForNext) {
				return nil, vmErrAt(curDebug, pc, "backward jump not to a FOR_NEXT")
			}
			pc = t - 1
		case compiler.OpJmpIfFalse:
			if len(stack) < 1 {
				return nil, vmErrAt(curDebug, pc, "JMP_IF_FALSE underflow")
			}
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !core.CoerceBoolean(cond) {
				if int(in.Arg) <= pc {
					return nil, vmErrAt(curDebug, pc, "backward conditional jump")
				}
				pc = int(in.Arg) - 1
			}
		case compiler.OpCallUserPoly:
			// Runtime-dispatched multi-overload user call: pick the arm via the
			// kernel's own MatchSignature (matchUserPoly), then enter its unit
			// exactly as OpCallUser does — pop the args into frame locals (the
			// match window IS the popped window, sig position 0 = top of stack),
			// re-check the param contract, push a frame. One dispatch (the
			// loop cleared the predicate memo before this op), so the
			// re-match and the re-check share one run of each predicate.
			unit, sigArgs, err := vc.matchUserPoly(&p.UserPolys[in.Arg], stack, curDebug, pc)
			if err != nil {
				return nil, err
			}
			fn := &p.Fns[unit]
			stack = stack[:len(stack)-fn.NParams]
			nl := make([]core.Value, fn.NLocals)
			copy(nl, sigArgs)
			// StripAscribed at delivery (the poly re-match above already
			// consumed the ascribed view). Quote list params so body
			// references are data — the compiled mirror of the
			// interpreter's binding rule (core_helpers.go).
			for i := 0; i < fn.NParams && i < len(nl); i++ {
				nl[i] = core.StripAscribed(nl[i])
				if nl[i].Parent.Equal(core.TList) && !nl[i].Quoted {
					nl[i].Quoted = true
				}
			}
			// The re-check asks each predicate type again; RunPredicate's
			// per-dispatch memo (cleared before the re-match) answers it
			// without a second run of the body (NUR102).
			if err := checkParamContract(r, fn, nl); err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
			frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth()})
			vc.frameDepth++ // balanced by the matching RET, like OpCallUser
			nameFrameFns(curReg, fn, nl)
			vc.pushFrameArgs(nl, fn.NArgs)
			locals = nl
			enterUnit(unit)
			pc = -1
		case compiler.OpCallUser, compiler.OpTailCallUser:
			fn := &p.Fns[in.Arg]
			if err := vc.gateNamedCall(curReg, fn.Name, len(stack), fn.NParams, "CALL_USER underflow at ", curDebug, pc); err != nil {
				return nil, err
			}
			// Per-export module policy gate (NUR045): a module-preamble
			// fn's compiled unit carries its owning sub-registry
			// (CompiledFn.Reg), whose ModuleRef was stamped at module
			// resolution — the CALL_USER twin of the interpreter's
			// execMatch gate over the stamped stored sig.
			if fn.Reg.IsModule() {
				// Read the STAMPED identity: the unit's own name is the
				// module-private fn name, not the export key the policy
				// addresses, so reconstructing one here would miss the rule.
				if err := vc.gateModuleCall(curReg, core.StampedModuleCall(fn.Reg, fn.Name)); err != nil {
					return nil, err
				}
			}
			nl := make([]core.Value, fn.NLocals)
			for i := 0; i < fn.NParams; i++ {
				// StripAscribed at delivery: params bind the REAL value
				// (execFnDefSig parity); a stripped subtype still passes the
				// param contract its widened view passed. Quote list params
				// so body references are data — the compiled mirror of the
				// interpreter's binding rule (core_helpers.go).
				nl[i] = core.StripAscribed(stack[len(stack)-1-i])
				if nl[i].Parent.Equal(core.TList) && !nl[i].Quoted {
					nl[i].Quoted = true
				}
			}
			stack = stack[:len(stack)-fn.NParams]
			// Param-type guard — the compiled mirror of the interpreter's
			// runtime sig match. A gradual (Dynamic) arg optimistically matched a
			// concrete param at check time, but the runtime value may not match;
			// without this a laundered List bound to an `m:Map` param silently runs
			// the body. nl[i] is param i (the body's slot i); Params[i] is its
			// declared type. Raises the same signature_error the interpreter raises,
			// over the window the interpreter's failed dispatch reports (NUR234).
			if err := checkParamContract(r, fn, nl); err != nil {
				if win, ok := callWindowAt(p, curUnit, pc, nl, stack, locals); ok {
					err = core.RuntimeNoMatch(r, fn.Name, win)
				}
				return nil, stampAt(err, curDebug, pc, curReg)
			}
			if in.Op == compiler.OpCallUser {
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth()})
				vc.frameDepth++ // balanced by the matching RET below
				nameFrameFns(curReg, fn, nl)
				vc.pushFrameArgs(nl, fn.NArgs)
			} else {
				// Tail call: REPLACE the frame — the language's
				// tail-call guarantee in compiled form. The caller's
				// return slot is untouched; loop state cannot leak
				// across a tail boundary in the compiled subset (tail
				// position excludes open loops by construction), but
				// trim defensively to the enclosing activation's loop
				// base — the calling frame's loopBase, or 0 at the
				// activation root (no frame; `loops` starts empty per
				// run()) — UNCONDITIONALLY, so a mis-emitted tail-in-loop
				// cannot leak a stale vmLoop into the replacement unit
				// (the old guard skipped the trim entirely when frameless).
				loopBase := 0
				if len(frames) > 0 {
					loopBase = frames[len(frames)-1].loopBase
				}
				loops = loops[:loopBase]
				nameFrameFns(curReg, fn, nl)
				vc.swapTailArgs(frames, nl, fn.NArgs)
			}
			locals = nl
			enterUnit(int(in.Arg))
			pc = -1
		case compiler.OpDeoptIfFn:
			fb := 0
			if len(frames) > 0 {
				fb = frames[len(frames)-1].stackBase
			}
			spec := &p.Fns[curUnit].Deopts[in.Arg]
			ns, fired, err := vc.deoptIfFn(curReg, &p.Fns[curUnit], spec, fb, stack, locals, curDebug, pc)
			if err != nil {
				return nil, err
			}
			stack = ns
			if fired {
				pc = spec.RetPC - 1
			}
		case compiler.OpBindDynScope, compiler.OpBindDynScopePeek:
			ns, err := vc.bindDynScopeMode(curReg, p, int(in.Arg), stack, curDebug, pc, in.Op == compiler.OpBindDynScope)
			if err != nil { //covergate:allow bindDynScopeMode's only error paths are its own allow-listed defensive guards (underflow / bad name const), unreachable without a bytecode-level fault (§compiler)
				return nil, err
			}
			stack = ns
		case compiler.OpBindGlobal:
			ns, err := vc.bindGlobal(curReg, &p.GlobalBinds[in.Arg], stack, curDebug, pc)
			if err != nil { //covergate:allow bindGlobal's only error path is its own allow-listed defensive underflow guard, unreachable without a bytecode-level fault (§compiler)
				return nil, err
			}
			stack = ns
		case compiler.OpBindTwin:
			// The installs were rolled back before this run
			// (RestoreBindingsForReplay), so the op re-performs its recorded
			// transition (Arg indexes Program.BindTwins) at this — its
			// source — position. Replay, never re-execution: the IDENTICAL
			// entry the check pass produced goes back in (§6.5).
			if err := core.ApplyBindTwin(curReg, p.BindTwins[in.Arg], p.BindTwinEntries[in.Arg]); err != nil {
				// A type twin whose name a run-time mint ahead of it holds
				// (core.ApplyBindTwin's doc): the interpreter's `def` at
				// this position raises the same type_error.
				return nil, stampAt(err, curDebug, pc, curReg)
			}
		case compiler.OpBindTypeRun:
			// A root type def over a bound only the run knows (NUR231): the
			// interpreter's own install of the body the run computed, and the
			// pass's node forwarded to the node it bound.
			if len(stack) == 0 {
				return nil, vmErrAt(curDebug, pc, "BIND_TYPE_RUN stack underflow")
			}
			body := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if err := core.RunTypeInstall(curReg, &p.TypeRuns[in.Arg], body); err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
		case compiler.OpBindFnType:
			// A fn unit's own per-call type install (the opcode's doc): the
			// name checked and reserved as the interpreter's `def T` checks
			// and reserves it — a second call of the frame raises the same
			// type_error — and the check-time node bound for the frame.
			if err := vc.bindFnType(curReg, &p.FnTypeBinds[in.Arg]); err != nil {
				return nil, stampAt(err, curDebug, pc, curReg)
			}
		case compiler.OpBindResident:
			// The arm-resident twin (§6.5's each-body recovery): executes
			// inside a compiled per-invocation unit, once per invocation,
			// with the RUNTIME value — the install arm pops it and installs
			// through the interpreter's own installer; the undef arm (a var
			// param's balanced teardown) pops the live binding instead. Rides
			// no unwind trail (leak persistence is the semantics — see
			// core.ApplyResidentBind).
			rb := &p.ResidentBinds[in.Arg]
			if rb.Undef {
				core.ApplyResidentBind(curReg, rb.Name, true, core.Value{})
				break
			}
			if rb.TypeInstall {
				// The TYPE arm: a type binding has no runtime value, so the
				// op re-installs the captured BODY through the interpreter's
				// own type installer — minting a fresh node per element, as
				// the interpreter's per-element body run does (replaying one
				// node instead is measurably wrong: see
				// core.ApplyResidentTypeBind). Sound because the bridge
				// proved the type expression element-independent before
				// stamping the site.
				if terr := core.ApplyResidentTypeBind(curReg, rb.Name, p.BindTwinEntries[rb.Twin]); terr != nil {
					return nil, vmErrAt(curDebug, pc, "BIND_RESIDENT type install: "+terr.Error())
				}
				break
			}
			if len(stack) == 0 { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "BIND_RESIDENT underflow")
			}
			// Peek by default (a live computed value stays for its downstream
			// readers); pop when the lowering pushed a copy (rb.Pop) —
			// GlobalBindSpec's mode split, same reason.
			core.ApplyResidentBind(curReg, rb.Name, false, core.StripAscribed(stack[len(stack)-1]))
			if rb.Pop {
				stack = stack[:len(stack)-1]
			}
		case compiler.OpUndefDynScope:
			// The placed transition of a speculative undef: pop the name's
			// live top binding in the current registry — the interpreter's
			// `undef` at this site — consuming nothing and riding no unwind
			// trail (a binding popped stays popped, as the interpreter's
			// does). A missing binding is the interpreter's no-op too.
			name, nerr := p.Consts[in.Arg].AsConcreteString()
			if nerr != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "UNDEF_DYN_SCOPE bad name const")
			}
			core.PopLiveBinding(curReg, name)
		case compiler.OpLookupDynScope:
			// The interpreter's stepWord simple-value substitution, at run
			// time: read the name's live binding. A miss, or a binding the
			// substitution would DISPATCH instead of push (a Function / class /
			// splice / reach), defers to the interpreter — containment for a
			// shape the VM cannot yet read, not a sanctioned outcome.
			name, nerr := p.Consts[in.Arg].AsConcreteString()
			if nerr != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "LOOKUP_DYN_SCOPE bad name const")
			}
			v, ok := curReg.Defs.Top(name)
			if !ok {
				// A name a placed speculative undef may have popped
				// (Program.SpecUndefNames): the miss IS the interpreter's
				// undefined_word, raised from the read's own position —
				// never deferred, since an effect performed before the read
				// fences the re-run into an internal error.
				if p.SpecUndefNames[name] || p.LiveReadNames[name] || p.CondBoundNames[name] {
					return nil, stampAt(core.UndefinedWordDiagWith(curReg, curReg.Source, name, debugPosAt(curDebug, pc), localNameCandidates(p, curUnit)), curDebug, pc, curReg)
				}
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-miss", "dynamic-scope read miss for `"+name+"`; the compiled runtime cannot execute it")
			}
			switch v.Data.(type) {
			case core.FnDefInfo, *core.ClassTypeInfo:
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-dispatching", "dynamic-scope read of a dispatching binding `"+name+"`; the compiled runtime cannot execute it")
			}
			if core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v) {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-active-token", "dynamic-scope read of an active token `"+name+"`; the compiled runtime cannot execute it")
			}
			stack = append(stack, v)
		case compiler.OpLookupDynScopeData:
			// The DATA-position twin of OpLookupDynScope (see bytecode.go): read
			// the name's live binding and PUSH it, pushing an FnDefInfo (the
			// parser/fn value the emitter proved is consumed as data by
			// parselang-fn-dispatch) rather than deferring — byte-identical to
			// the interpreter passing the /q-captured name as data. Still defers
			// on a genuine miss, a class binding (not a parser), and an active
			// token (splice/reach/word/mark/move).
			name, nerr := p.Consts[in.Arg].AsConcreteString()
			if nerr != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "LOOKUP_DYN_SCOPE_DATA bad name const")
			}
			v, ok := curReg.Defs.Top(name)
			if !ok {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-miss", "dynamic-scope data read miss for `"+name+"`; the compiled runtime cannot execute it")
			}
			if _, isClass := v.Data.(*core.ClassTypeInfo); isClass {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-class", "dynamic-scope data read of a class binding `"+name+"`; the compiled runtime cannot execute it")
			}
			if core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v) {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-active-token", "dynamic-scope data read of an active token `"+name+"`; the compiled runtime cannot execute it")
			}
			stack = append(stack, v)
		case compiler.OpRet:
			// Return-type check — the compiled mirror of the interpreter's
			// ReturnCheck (__RC, engine.go): the body's result must satisfy
			// each declared return type via v.Is(exp), the SAME membership
			// the parameter boundary asks, so a predicate refine runs its
			// predicate, a bare refine stays nominal, and builtins are
			// unchanged. The body nets exactly len(Returns) values (the
			// lowerer enforces single-result bodies), sitting on top.
			// Applies to a nested RET (back to a CALL_USER caller) AND the
			// top RET of a re-entrant unit run. (A closure body unit carries
			// Returns=[Any] — compileClosureBody's declared return — so this
			// check is a guaranteed-pass v.Is(Any) for closures; the cost is
			// one trivial membership test per closure return, deliberately
			// kept uniform with user-fn return enforcement.)
			if curUnit >= 0 {
				stackBase := 0
				contract := &p.Fns[curUnit]
				if len(frames) > 0 {
					stackBase = frames[len(frames)-1].stackBase
					// An Apply-kernel frame carries the APPLIED VALUE's declared
					// contract, because the unit it entered declares none of its
					// own (applyRetContract). Every other frame leaves this nil
					// and the unit is the contract, as before.
					if ov := frames[len(frames)-1].retFn; ov != nil {
						contract = ov
					}
				}
				var trimmed []core.Value
				var err error
				if len(frames) == 0 && vc.rootRetTrim {
					// The root RET of a unit entered through the fn-VALUE seam
					// (RunUnit / runUnitNested: InvokeCallback's compiled path):
					// the interpreter's CallBoru would have run this body, so
					// its return discipline is enforceCallBoruReturns — types
					// over the aligned residual, never the count (`walk … cb/v`
					// over a 0-value body runs clean interpreted).
					trimmed, err = stack, checkCallBoruContract(r, contract, stack, core.SrcPos{})
				} else if len(frames) == 0 && vc.rootRetNamed {
					// The root RET of a unit entered as a NAMED fn call
					// (InvokeCompiledStrict: the module-fn dispatch): the
					// frame's own contract — the count as __RC enforces it,
					// the fresh operand stack being the frame's (NUR191).
					trimmed, err = checkReturnContract(r, contract, stack, 0, true, core.SrcPos{})
				} else {
					trimmed, err = checkReturnContract(r, contract, stack, stackBase, len(frames) > 0, core.SrcPos{})
				}
				if err != nil {
					// A nested frame's contract error anchors at the CALL —
					// the token the interpreter's ReturnCheck marker carries
					// (`h` in `def h fn [[][Integer][1 2]] end h`, 1:33) —
					// not at the body's last instruction (NUR118).
					if len(frames) > 0 {
						fr := frames[len(frames)-1]
						callDebug := p.Debug
						if fr.retUnit >= 0 && fr.retUnit < len(p.Fns) {
							callDebug = p.Fns[fr.retUnit].Debug
						}
						return nil, stampAt(err, callDebug, fr.retPC-1, curReg)
					}
					return nil, stampAt(err, curDebug, pc, curReg)
				}
				stack = trimmed
				// Strip any dispatch ascription (`v as T`) from the frame's
				// return values — the compiled mirror of the interpreter's
				// frame-collapse / CallBoru strip: an ascription is scoped to
				// a dispatch WITHIN the body and cannot ride out to the
				// caller (design/OPEN-WORDS.1.md §9). Unconditional
				// StripAscribed (its own nil-fast-path handles the common no-
				// ascription case) — a compiled body only carries a runtime
				// ascription when `as` took a dynamic operand it could not
				// fold, so a guarded arm here would be a dead compiled branch.
				for i := stackBase; i < len(stack); i++ {
					stack[i] = core.StripAscribed(stack[i])
				}
			}
			if len(frames) == 0 {
				// Top RET of a re-entrant unit run (a body closure invoked
				// via invokeClosure): the residual stack is the unit's
				// result, threaded back through the InvokeBody seam. The
				// main program (unit -1) never RETs — it runs off the end —
				// so this path is closure/fn-root only. Bindings this
				// activation installed pop here, like any frame exit —
				// except a KEEP-DEFS body's (`do`, CompiledFn.KeepsDefs):
				// the interpreter runs that body in the caller's frame and
				// its defs leak, so the installs stay on the trail for the
				// ENCLOSING frame's exit to pop (or the run's end at root).
				if !keepDefs {
					vc.unwindDynBinds(dynBase)
				}
				return stack, nil
			}
			f := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			vc.frameDepth-- // matches the OpCallUser increment
			vc.unwindDynBinds(f.dynBase)
			vc.retFrameArgs(&f)
			loops = loops[:f.loopBase]
			locals = f.locals
			enterUnit(f.retUnit)
			pc = f.retPC - 1
		case compiler.OpFlowBreak, compiler.OpFlowContinue:
			// A break/continue raised in a fn body with no enclosing loop in its
			// own unit targets the nearest open loop in an ANCESTOR frame — the
			// interpreter's cross-frame FlowCtrl, compiled (see flowSignal).
			var u int
			var err error
			if frames, loops, locals, stack, pc, u, err = vc.flowSignal(in.Op, frames, loops, locals, stack, pc, curUnit, curDebug); err != nil {
				return nil, err
			}
			enterUnit(u)
		default:
			return nil, vmErrAt(curDebug, pc, "unknown opcode")
		}
	}
	if len(frames) != 0 {
		return nil, vmErrAt(curDebug, len(curCode)-1, "code unit ended without RET")
	}
	return stack, nil
}

// opForSetup opens a counted loop for OpForSetup: it pops the range triple
// (start on top, then end, then step — the same shape parseRange yields, with
// runForLoop's zero-step error and negative-step semantics) and appends the
// vmLoop. exitPC / nextPC ride the existing instruction stream: the lowerer
// always emits FOR_NEXT immediately after FOR_SETUP, and FOR_NEXT.Arg is the
// loop's exit pc (patched in lowerLoop), so a cross-frame flow signal finds the
// loop's targets without a side table. Returns the trimmed stack and the
// grown loop slice. Split out of run to keep that switch under the complexity
// budget.
func (vc *vmContext) opForSetup(stack []core.Value, loops []vmLoop, slot int, curCode []compiler.Instr, curUnit, pc int, debug []core.SrcPos) ([]core.Value, []vmLoop, error) {
	if len(stack) < 3 {
		return nil, nil, vmErrAt(debug, pc, "FOR_SETUP underflow")
	}
	start, err1 := stack[len(stack)-1].AsConcreteInteger()
	endV, err2 := stack[len(stack)-2].AsConcreteInteger()
	stepV, err3 := stack[len(stack)-3].AsConcreteInteger()
	stack = stack[:len(stack)-3]
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, nil, stampAt(vc.r.BoruError("for_error", "for: range must be concrete Integers", "for"), debug, pc, vc.r)
	}
	if stepV == 0 {
		return nil, nil, stampAt(vc.r.BoruError("for_error", "for: step cannot be zero", "for"), debug, pc, vc.r)
	}
	// The loop's FOR_NEXT usually follows FOR_SETUP directly, but a loop with
	// CARRIED defs seats their slot inits between the two (lowerLoop) — scan
	// forward for the real FOR_NEXT so exitPC / nextPC never read another
	// instruction's Arg as a jump target.
	next := pc + 1
	for next < len(curCode) && curCode[next].Op != compiler.OpForNext {
		next++
	}
	if next >= len(curCode) { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (lowerLoop always pairs FOR_SETUP with a FOR_NEXT) (§compiler)
		return nil, nil, vmErrAt(debug, pc, "FOR_SETUP without a FOR_NEXT")
	}
	loops = append(loops, vmLoop{
		cur: start, end: endV, step: stepV, slot: slot,
		exitPC: int(curCode[next].Arg), nextPC: next, unit: curUnit, iterBase: len(stack),
	})
	return stack, loops, nil
}

// flowSignal resolves a cross-frame break/continue (OpFlowBreak /
// OpFlowContinue — see the opcode docs): a break/continue raised in a fn body
// with no enclosing loop in its OWN unit targets the nearest open loop, which
// lives in an ANCESTOR frame. It unwinds every frame opened since that loop
// (restoring the caller's locals; the returned unit is the loop's, re-entered
// by run's enterUnit), discards the current iteration's partial operand pushes
// (trim to the loop's iteration base — completed iterations' results sit below
// and survive), then points pc at the loop's exit (break) or FOR_NEXT
// (continue). With no open loop at all it returns an internal_error so
// RunCompiled falls back and the interpreter raises the canonical "outside
// loop" taxonomy. Returns the updated frames/loops/locals/stack/pc and the unit
// to re-enter. This is engine.go's handleLoopBreak / handleLoopContinue,
// compiled. Split out of run to keep that switch under the complexity budget.
func (vc *vmContext) flowSignal(op compiler.Opcode, frames []vmFrame, loops []vmLoop, locals, stack []core.Value, pc, curUnit int, debug []core.SrcPos) ([]vmFrame, []vmLoop, []core.Value, []core.Value, int, int, error) {
	if len(loops) == 0 {
		if vc.flowEscapes {
			// A hosted token body: the signal is the ENCLOSING run's to
			// resolve, as the seam's sub-engine would have handed it back
			// (vmContext.flowEscapes).
			return nil, nil, nil, nil, 0, 0, &flowEscape{op: op}
		}
		return nil, nil, nil, nil, 0, 0, vmErrAt(debug, pc, "flow signal with no enclosing loop")
	}
	target := len(loops) - 1
	lp := loops[target]
	unit := curUnit
	for len(frames) > 0 && frames[len(frames)-1].loopBase > target {
		f := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		vc.frameDepth--
		locals = f.locals
		unit = f.retUnit
		// The discarded frame's dynamic-scope bindings tear down with it —
		// the interpreter's unwindLiveFrames replays the frame's cleanup
		// tail when a break/continue escapes it; without this a dead
		// frame's OpBindDynScope install would stay readable in Defs.
		vc.unwindDynBinds(f.dynBase)
		vc.retFrameArgs(&f)
	}
	stack = stack[:lp.iterBase]
	if op == compiler.OpFlowBreak {
		loops = loops[:target]
		pc = lp.exitPC - 1
	} else {
		pc = lp.nextPC - 1
	}
	return frames, loops, locals, stack, pc, unit, nil
}

// stampAt / vmErrAt are the per-unit debug-table variants of the
// program-level error helpers.
// debugPosAt is the debug table's position for pc — the zero position when
// the table does not cover it, which stampAt then leaves unstamped.
func debugPosAt(debug []core.SrcPos, pc int) core.SrcPos {
	if pc >= 0 && pc < len(debug) {
		return debug[pc]
	}
	return core.SrcPos{}
}

// localNameCandidates is the did-you-mean pool's compiled half: the names
// of the running unit's frame locals — its params and captures, its loop
// variables, its promoted body-local defs — which the interpreter holds as
// defs in the registry the suggestion pool reads, and which a compiled
// frame keeps in slots the registry never sees (NUR146: `def k 5  for 2 [
// if (k eq 5) [undef k] [] ]` suggested `i` interpreted and nothing
// compiled). The main unit's table is Program.LocalNames; a fn unit's is
// its CompiledFn.LocalNames. Spill temps are anonymous and skipped.
func localNameCandidates(p *compiler.Program, curUnit int) []string {
	var names []string
	if curUnit >= 0 && curUnit < len(p.Fns) {
		names = p.Fns[curUnit].LocalNames
	} else {
		names = p.LocalNames
	}
	var out []string
	for _, n := range names {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func stampAt(err error, debug []core.SrcPos, pc int, r *core.Registry) error {
	ae, ok := err.(*core.BoruError)
	if !ok {
		return err
	}
	if ae.Row == 0 && pc >= 0 && pc < len(debug) {
		ae.Row = debug[pc].Row
		ae.Col = debug[pc].Col
		// The token's own text too: the caret's width is the token's, and
		// a return-count error the interpreter stamps at the `apply` word
		// underlines all five characters (the twenty-ninth increment). The
		// token's text REPLACES what the error was built with, as the
		// interpreter's stampErrPos replaces it: a handler error carries
		// its WORD as source text (`r.BoruError(code, detail,
		// "FnUtil.compose")`), and where the dispatching token is not that
		// word — a def-bound wrapper applied under its one-letter name — the
		// interpreter underlines the token, not the word (the thirty-fifth
		// increment).
		if debug[pc].Src != "" {
			ae.Src = debug[pc].Src
		}
	}
	if r != nil && ae.FullSource == "" {
		ae.FullSource = r.Source
	}
	// The originating file, as the interpreter's stampErrPos attaches it:
	// a positioned error names the registry's BaseFile whether the position
	// was stamped here or carried in (a routed dispatch's diagnostic is
	// built AT its position, region_diag.go), and r is the registry the
	// UNIT runs on — a module fn's own — so an error raised inside an
	// imported module renders `--> mod.boru:row:col` on both lanes (review
	// of #462). Neither the source nor the file needs a debug entry, which
	// only the row stamp reads.
	if r != nil && ae.File == "" && ae.Row != 0 {
		ae.File = r.BaseFile
	}
	return ae
}

// vmReturnTypeErr / vmReturnCountErr raise the interpreter's
// returnTypeError / returnCountError — same detail/hint, same type_error
// taxonomy, and the SAME two secondary spans (the produced value, and the
// return-contract declaration fn.Decl) — via the shared builders in
// return_check_msg.go. So the compiled and interpreted return diagnostics are
// byte-identical bar the primary caret position (the VM points inside the
// shared fn unit, the interpreter at the call site — the documented, gated
// difference). The primary position is left unset here and stamped by stampAt
// on the RET.
func vmReturnTypeErr(r *core.Registry, fn *compiler.CompiledFn, index int, expected *core.Type, got core.Value, at core.SrcPos) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return core.BuildReturnTypeError(src, fn.Name, index, expected, got, at, fn.Decl)
}

func vmReturnCountErr(r *core.Registry, fn *compiler.CompiledFn, expected, got int, values []core.Value, at core.SrcPos) error {
	src := ""
	if r != nil {
		src = r.Source
	}
	return core.BuildReturnCountError(src, fn.Name, expected, got, values, at, fn.Decl)
}

// vmMark executes the variadic-region opcodes (OpStackMark / OpDropToMark /
// OpPopMark): a runtime-variable count of values produced above a saved depth
// is truncated away (DropToMark) or kept (PopMark). Extracted from the main
// run loop so its branches don't inflate that switch's cyclomatic complexity.
// Returns the updated mark stack and operand stack.
//
// These ops are COUNT-AGNOSTIC, and this comment used to say "0-or-1" as if
// they were not (measured 2026-09-10, NUR067): DropToMark truncates to
// stack[:m] whatever the count above m is, and PopMark keeps whatever is
// there. 0-or-1 describes the only CLIENT the lowerer emits them for today
// (the chained variadic-statement `if`), not the mechanism.
func vmMark(op compiler.Opcode, marks []int, stack []core.Value, debug []core.SrcPos, pc int) ([]int, []core.Value, error) {
	switch op {
	case compiler.OpStackMark:
		// Open a variadic region: remember the current depth so a 0-or-1 value
		// produced above it can be truncated away later.
		return append(marks, len(stack)), stack, nil
	case compiler.OpDropToMark:
		// Close a region on the path that DISCARDS the 0-or-1 eager: pop the mark
		// and truncate the stack back to it.
		if len(marks) == 0 {
			return marks, stack, vmErrAt(debug, pc, "DROP_TO_MARK with no open mark")
		}
		m := marks[len(marks)-1]
		marks = marks[:len(marks)-1]
		if m > len(stack) {
			return marks, stack, vmErrAt(debug, pc, "DROP_TO_MARK above current depth")
		}
		return marks, stack[:m], nil
	default: // OpPopMark
		// Close a region on the path that KEEPS the eager: discard the mark
		// without touching the stack.
		if len(marks) == 0 {
			return marks, stack, vmErrAt(debug, pc, "POP_MARK with no open mark")
		}
		return marks[:len(marks)-1], stack, nil
	}
}

// vmShuffle reverses the top n operand-stack values in place: OpSwap is the
// n=2 case, OpReverse takes n from arg. Used to seat an N-operand call's
// computed args (which evaluate into reverse sig order) onto the stack in sig
// order.
func vmShuffle(stack []core.Value, op compiler.Opcode, arg int, debug []core.SrcPos, pc int) ([]core.Value, error) {
	n := 2
	if op == compiler.OpReverse {
		n = arg
	}
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, op.String()+" underflow")
	}
	for i, j := len(stack)-n, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack, nil
}

// checkReturnContract enforces a compiled fn's declared return contract at a RET
// — the compiled mirror of the interpreter's ReturnCheck (__RC, engine.go). Each
// declared return must satisfy its type via v.Is(exp), the SAME membership the
// parameter boundary asks (so a predicate refine runs its predicate, a bare
// refine stays nominal). When hasFrame is set (a genuine user fn entered via
// CALL_USER), the body must leave EXACTLY len(Returns) values measured from the
// frame's entry stackBase — too few OR too many is the return-count type_error.
// The re-entrant closure / fn-root RET (no frame) runs on a fresh stack and only
// the underflow is a count error there (a surplus is the higher-order caller's
// domain) — preserving the prior closure behaviour. Returns nil when satisfied.
// checkParamContract enforces the declared PARAM types at CALL_USER entry — the
// compiled mirror of the interpreter's runtime signature match (and the symmetric
// twin of checkReturnContract at RET). Each param local nl[i] must satisfy
// Params[i] via v.Is(exp), the SAME membership the param boundary asks. A nil /
// Any param is a guaranteed pass (a closure's [Any] input, or a fn declaring an
// Any param). Multi-overload gradual calls never compile, so the single chosen
// overload's guard mirrors the interpreter exactly. On mismatch it raises the
// byte-identical signature_error the interpreter raises for an unmatched dispatch.
func checkParamContract(r *core.Registry, fn *compiler.CompiledFn, locals []core.Value) error {
	for i, pt := range fn.Params {
		if pt == nil || i >= len(locals) {
			continue
		}
		// Use sigTypeMatches — the interpreter's RUNTIME param match — NOT v.Is.
		// v.Is is a strict SUBSET: it rejects a concrete map at an `Options` slot
		// (Options roots under Ideal, TMap ⋢ TOptions), which the interpreter's
		// sigTypeMatches accepts (signature.go's Options/Record special-cases). A
		// v.Is guard therefore OVER-RAISES on Options / structural params — a
		// regression. sigTypeMatches subsumes v.Is and folds in those special
		// cases, the Any root, and (inert at run time) gradual optimism, so it
		// matches the interpreter exactly for a concrete runtime value. A
		// constraint carried in FnParam.Pattern (inline disjunct / predicate /
		// bounded / structural) is NOT threaded into Params and so is not enforced
		// here — see design/legacy/PARAM-GUARD-SKIP-MISCOMPILE.0.ignore; this guard catches the
		// plain-type laundering (the reported bug) without over-raising.
		if !core.SigTypeMatches(locals[i], pt) {
			return core.RuntimeNoMatch(r, fn.Name, guardArgs(locals, fn.NArgs))
		}
	}
	// An inline disjunct / predicate / bounded / structural param carries its
	// real constraint in ParamPatterns (its Type is a loose root sigTypeMatches
	// passes), so check it the SAME way the interpreter's dispatch does
	// (engine.go: OpenUnifyMap for a concrete map pattern, else Unify).
	for i, pp := range fn.ParamPatterns {
		if pp == nil || i >= len(locals) {
			continue
		}
		pat := *pp
		v := locals[i]
		ok := false
		if pat.Parent.Equal(core.TMap) && v.Parent.Equal(core.TMap) && pat.Data != nil && v.Data != nil && !core.IsOptionsType(pat) {
			ok = core.OpenUnifyMap(pat, v)
		} else {
			_, ok = core.Unify(v, pat)
		}
		if !ok {
			return core.RuntimeNoMatch(r, fn.Name, guardArgs(locals, fn.NArgs))
		}
		// Retag a {:T}/[:T] param's concrete runtime arg with its element type so
		// compiled body writes enforce it — the compiled mirror of the
		// interpreter's RetagTypedContainerParam. Uses the SAME shared core, so a
		// flex arg is retagged in place (reference identity preserved) and a plain
		// arg is re-unified — both paths agree with the interpreter (no divergence).
		if core.IsConcrete(v) {
			locals[i] = core.RetagTypedContainerValue(pat, v)
		}
	}
	return nil
}

// guardArgs returns the leading n locals — the real dispatch arguments,
// sig order, excluding a closure's trailing capture slots — as the
// failing tuple a runtime param-contract guard rebuilds its rich
// no-signature diagnostic from. Clamped to the available locals.
func guardArgs(locals []core.Value, n int) []core.Value {
	if n > len(locals) {
		n = len(locals)
	}
	if n < 0 {
		n = 0
	}
	return locals[:n]
}

// checkNativeParamContract enforces a GUARDED CALL_NATIVE's committed sig at run
// time — the native twin of checkParamContract (CALL_USER). args[i] is sig
// position i (top-of-stack first, as OpCallNative built them). Each must satisfy
// sigArgType(s.Sig, i) via sigTypeMatches — the SAME runtime param match the interpreter's
// matchSignature applies, so a concrete value that the interpreter's sole-overload
// dispatch would accept passes here and one it rejects raises the byte-identical
// signature_error. Sound only for a single-overload word (the recorder's gate): no
// sibling exists for a missing arg to fall through to, so raise == the interpreter.
func checkNativeParamContract(r *core.Registry, s *compiler.SigRef, args []core.Value) error {
	for i := range args {
		if i >= s.Sig.TotalArgs() {
			break
		}
		at := core.SigArgType(s.Sig, i)
		if at == nil || at.Equal(core.TAny) {
			continue // an Any slot is a guaranteed pass
		}
		// A QuoteArgs slot carries a literal Atom key (dot/get's bare-word form);
		// the interpreter binds it as data without a type match, so don't guard it.
		if s.Sig.QuoteArgs != nil && s.Sig.QuoteArgs[i] {
			continue
		}
		if !core.SigTypeMatches(args[i], at) {
			return core.RuntimeNoMatch(r, s.Word, args)
		}
	}
	return nil
}

func checkReturnContract(r *core.Registry, fn *compiler.CompiledFn, stack []core.Value, stackBase int, hasFrame bool, at core.SrcPos) ([]core.Value, error) {
	rets := fn.Returns
	if len(rets) == 0 {
		return stack, nil
	}
	// A whole-frame dynamic-apply replay (RetReplay) in a FOREIGN-registry fn:
	// the interpreter dispatches such a fn via CallBoru, whose return path is
	// TRIM-ONLY (registry.go — up to NUnnamed extra bottom values discarded,
	// count and type NEVER enforced; the documented frame-path asymmetry). The
	// replay's residual count is runtime-variable, but every compiled CALLER
	// was laid out against the static model of len(Returns) results — so after
	// the CallBoru trim, a count that still differs cannot be represented and
	// DEFERS to the interpreter (internal_error → the sound whole-program
	// fallback). A same-registry fn falls through to the frame-path contract
	// below, which the interpreter enforces identically.
	if fn.RetReplay && dispatchRegistry(fn.Reg, r) != r {
		base := 0
		if hasFrame {
			base = stackBase
		}
		produced := len(stack) - base
		if extra := produced - len(rets); extra > 0 {
			trim := extra
			if trim > fn.NUnnamed {
				trim = fn.NUnnamed
			}
			stack = append(stack[:base], stack[base+trim:]...)
			produced -= trim
		}
		if produced != len(rets) {
			return stack, vmDefer(r, nil, 0, "vm:dyn-frame-replay", fmt.Sprintf(
				"dynamic frame replay %s: result count %d differs from the declared %d; the compiled runtime cannot execute it",
				fn.Name, produced, len(rets)))
		}
		return stack, nil
	}
	// The __RC arity discipline (engine.go): the frame must produce at
	// least the declared count; extras are tolerated only up to the
	// unnamed-arg allowance (NUnnamed — an unnamed param the body never
	// consumed, re-pushed at unit entry, sits at the frame's bottom exactly
	// as it sits at the interpreter frame's bottom) and are DISCARDED
	// before the caller sees the result. Beyond the allowance it is the
	// interpreter's count error, byte-identical.
	if hasFrame {
		produced := len(stack) - stackBase
		if produced < len(rets) {
			return stack, vmReturnCountErr(r, fn, len(rets), produced, stack[stackBase:], at)
		}
		if extra := produced - len(rets); extra > 0 {
			if extra > fn.NUnnamed {
				// Allowance spent from the bottom — report the top values,
				// the same slice the interpreter reports
				// (design/DIAGNOSTIC-VALUES.0.md).
				return stack, vmReturnCountErr(r, fn, len(rets), produced-fn.NUnnamed,
					stack[stackBase+fn.NUnnamed:], at)
			}
			stack = append(stack[:stackBase], stack[stackBase+extra:]...)
		}
	} else {
		if len(stack) < len(rets) {
			return stack, vmReturnCountErr(r, fn, len(rets), len(stack), stack, at)
		}
		// Frameless (re-entrant closure / fn-root run): same trim over the
		// whole residual — a closure unit has NUnnamed 0, so this is a
		// no-op for every closure body.
		if extra := len(stack) - len(rets); extra > 0 && fn.NUnnamed > 0 {
			trim := extra
			if trim > fn.NUnnamed {
				trim = fn.NUnnamed
			}
			stack = append(stack[:0], stack[trim:]...)
		}
	}
	base := len(stack) - len(rets)
	for k, exp := range rets {
		if !stack[base+k].Is(core.CanonicalType(r, exp)) {
			return stack, vmReturnTypeErr(r, fn, k+1, exp, stack[base+k], at)
		}
		// A declared return whose *Type degraded to Any carries its real
		// domain in the pattern — the RET-side twin of the ParamPatterns
		// guard at CALL_USER, and the same Unify the interpreter's
		// ReturnCheck runs (engine.go validateReturnTypes). Without it the
		// COMPILED path accepted `def IS (Integer tor String)` /
		// `def f fn x:Integer IS [true]` that the interpreter and the check
		// pass both reject: `Is(Any)` passes everything, so the union read
		// as a comment on the only path most programs take.
		//
		// `Type` aliases `Value`, so the pattern pointer doubles as the
		// "expected" the error builder renders — `expected Integer tor
		// String` rather than the useless `expected Any`.
		if pat := fn.ReturnPattern(k); pat != nil {
			if _, ok := core.Unify(*pat, stack[base+k]); !ok {
				return stack, vmReturnTypeErr(r, fn, k+1, pat, stack[base+k], at)
			}
		}
	}
	return stack, nil
}

// checkCallBoruContract is the VM's mirror of the interpreter's CallBoru
// return discipline (core enforceCallBoruReturns, NUR069): the declared
// return TYPES are checked head-to-head over the residual — position k of
// the declaration against res[extra+k], the surplus (unconsumed unnamed
// inputs, a longer residual) sitting at the bottom — and the COUNT is never
// raised: a guard predicate signals with a None residual, a lambda's
// placeholder `[Any]` sits over a side-effect body, and both are legitimate
// on this seam. Nothing is checked inside a predicate call, exactly as the
// interpreter skips there. res is returned untouched: the handler reads the
// residual it was always handed (the top, for a top-taking word).
func checkCallBoruContract(r *core.Registry, fn *compiler.CompiledFn, res []core.Value, at core.SrcPos) error {
	rets := fn.Returns
	if len(rets) == 0 || (r != nil && r.InPredicateCall()) {
		return nil
	}
	extra := len(res) - len(rets)
	if extra < 0 {
		extra = 0
	}
	n := len(res) - extra
	for k := 0; k < n; k++ {
		v := res[extra+k]
		if !v.Is(core.CanonicalType(r, rets[k])) {
			return vmReturnTypeErr(r, fn, k+1, rets[k], v, at)
		}
		if pat := fn.ReturnPattern(k); pat != nil {
			if _, ok := core.Unify(*pat, v); !ok {
				return vmReturnTypeErr(r, fn, k+1, pat, v, at)
			}
		}
	}
	return nil
}

// vmMakeMap pops the values of an OpMakeMap assembly off the top of stack and
// returns the stack with the assembled map pushed: the deepest of the popped
// run is value 0, paired with Keys[0]. Extracted from vmContext.run to keep that
// loop's cyclomatic complexity bounded.
func vmMakeMap(p *compiler.Program, stack []core.Value, arg int32, debug []core.SrcPos, pc int) ([]core.Value, error) {
	spec := p.MakeMaps[arg]
	n := len(spec.Keys)
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, "MAKE_MAP stack underflow")
	}
	vals := stack[len(stack)-n:]
	om := core.NewOrderedMap()
	om.Implicit = spec.Implicit
	for i, k := range spec.Keys {
		// Ascription hygiene: map values are STORED data (mirrors
		// autoEvalMap's value strip).
		om.Set(k, core.StripAscribed(vals[i]))
	}
	return append(stack[:len(stack)-n], core.NewMap(om)), nil
}

// vmInterp pops one operand-stack value per hole of an OpInterp template
// (deepest popped = hole 0, source order), then interleaves the literal
// segments with ValToString of each hole — byte-identical to the interpreter's
// evalInterpParts — and returns the stack with the assembled string pushed.
// Extracted from vmContext.run to keep that loop's cyclomatic complexity bounded.
func vmInterp(p *compiler.Program, stack []core.Value, arg int32, debug []core.SrcPos, pc int) ([]core.Value, error) {
	spec := &p.Interps[arg]
	n := spec.NHoles
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, "INTERP stack underflow")
	}
	holes := stack[len(stack)-n:]
	var sb strings.Builder
	hi := 0
	for _, seg := range spec.Segs {
		if seg.Hole {
			sb.WriteString(core.ValToString(holes[hi]))
			hi++
		} else {
			sb.WriteString(seg.Lit)
		}
	}
	return append(stack[:len(stack)-n], core.NewString(sb.String())), nil
}

// vmInterpXml executes OpInterpXml: pop the template's holes (deepest = hole
// 0, the traversal order BuildXmlFromTmpl evaluates in) and rebuild the
// element via rebuildXmlFromTmpl — byte-identical to the interpreter's build
// over the same hole values.
func vmInterpXml(p *compiler.Program, stack []core.Value, arg int32, debug []core.SrcPos, pc int) ([]core.Value, error) {
	spec := &p.XmlInterps[arg]
	n := spec.NHoles
	if len(stack) < n {
		return nil, vmErrAt(debug, pc, "INTERP_XML stack underflow")
	}
	holes := stack[len(stack)-n:]
	out, used := core.RebuildXmlFromTmpl(spec.Tmpl, holes)
	if used != n { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, vmErrAt(debug, pc, "INTERP_XML hole count mismatch")
	}
	return append(stack[:len(stack)-n], out), nil
}

// vmErrAt builds an internal_error BoruError for a VM-internal soundness
// violation (a simulated/runtime stack disagreement the lowerer thought
// impossible). It carries the boru taxonomy code — so a direct RunProgram
// caller and error-scraping tooling see a structured error, not a raw Go
// string — and RunCompiled treats it as a fall-back-to-interpreter signal.
// Reaching one is a compiler bug; the message keeps the pc/source detail.
func vmErrAt(debug []core.SrcPos, pc int, msg string) error {
	pos := core.SrcPos{}
	if pc >= 0 && pc < len(debug) {
		pos = debug[pc]
	}
	e := core.MakeBoruErrorAt("internal_error",
		fmt.Sprintf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col),
		"", "", "", pos)
	e.VMDefer = true
	return e
}

// vmEvalLimitAt mirrors the interpreter's evalLimitError: the
// step-count (CPU) guard, distinct from the stack/frame ceiling
// (the memory guard).
func vmEvalLimitAt(debug []core.SrcPos, pc int, r *core.Registry, limit int) error {
	err := r.BoruErrorHint("evaluation_limit",
		fmt.Sprintf("evaluation exceeded the step limit of %d — the program ran too long (an infinite loop or unbounded recursion?)", limit),
		"",
		"if this is a legitimately long computation, raise the limit with `--options steps:N` (or lang.Options.Steps); otherwise check for a loop or recursion that never terminates")
	return stampAt(err, debug, pc, r)
}

func vmExhaustedAt(debug []core.SrcPos, pc int, r *core.Registry, ceiling int) error {
	err := r.BoruErrorHint("tape_exhausted",
		fmt.Sprintf("evaluation stack exhausted its growth ceiling of %d entries — the program consumed unbounded space (an unbounded loop accumulating results, or unbounded non-tail recursion?)", ceiling),
		"",
		"raise the tape size via options (initial size / grow count / growth factor) for a legitimately large program; otherwise check the loop bounds / recursion")
	return stampAt(err, debug, pc, r)
}

// vmStackCeiling mirrors the tape's bounded-growth ceiling for the
// VM value stack: initial · factorᴺ entries from the registry's
// TapeConfig, exactly NewTapeWith's arithmetic, so a program that
// accumulates without bound fails with the same resource taxonomy in
// both engines.
func vmStackCeiling(r *core.Registry) int {
	var cfg core.TapeConfig
	if r != nil {
		cfg = r.TapeConfig
	}
	initial, maxGrows, factor := cfg.Resolve(0)
	return core.GrowthCeiling(initial, maxGrows, factor)
}

// dispatchRegistry is the registry a compiled record resolves its word in:
// the OWNING registry the record was stamped with (a module's sub-registry —
// a native reached through its wrapper, a `module [...]` preamble fn's body),
// or the RUNNING one when none was. A record with no stamp resolves where it
// runs, and that is what lets one compiled program run on any fork of its
// registry (ForkConcurrent hands each concurrent execution its own). This is
// the ONLY reading of a nil PolyRef.Reg / RegionDesc.Reg / CompiledFn.Reg;
// the sites that ask "does this unit run somewhere other than here" compare
// the result against the running registry rather than the field.
func dispatchRegistry(owning, running *core.Registry) *core.Registry {
	if owning != nil {
		return owning
	}
	return running
}
