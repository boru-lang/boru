package eng

// The bytecode VM — the execution half of Stages 1–3 of
// design/boru-bytecode-plan.0.md: straight-line natives, control flow
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
	// rootRetTrim marks a re-entrant run entered through the fn-VALUE seam
	// (enterCallbackUnit): its root RET applies the CallBoru return
	// discipline (checkCallBoruContract) rather than __RC's.
	rootRetTrim bool
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
// should produce one (the emitter refuses fn-invoking / code-splicing words),
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
// produce one (the emitter refuses fn-invoking / code-splicing words); reaching
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
	if p.ReplayReg == r && len(p.BindTwins) > 0 {
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
	return runVMEntry(ref.Prog, r, core.StepLimitFor(r, core.DefaultStepLimit), func(vc *vmContext) ([]core.Value, error) {
		return vc.enterCallbackUnit(r, ref.Unit, bindUnitLocals(&ref.Prog.Fns[ref.Unit], args, ref.Captures))
	})
}

// enterCallbackUnit is enterBodyUnit for the fn-VALUE seam (RunUnit and
// runUnitNested — InvokeCallback's compiled path): the unit's root RET takes
// the CallBoru return discipline (rootRetTrim → checkCallBoruContract)
// instead of __RC's. The flag is scoped to this entry: a closure the body
// invokes through the TOKEN seam (invokeClosureOn) enters with it cleared.
func (vc *vmContext) enterCallbackUnit(reg *core.Registry, unit int, locals []core.Value) ([]core.Value, error) {
	prev := vc.rootRetTrim
	vc.rootRetTrim = true
	defer func() { vc.rootRetTrim = prev }()
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
func bindUnitLocals(fn *compiler.CompiledFn, args, captures []core.Value) []core.Value {
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
	nameFrameFns(fn, locals)
	return locals
}

// nameFrameFns names the fn VALUES a frame binds, as the interpreter's frame
// binding does (installDef: `fnDef.Name = name` for a Function-family body):
// a fn passed for a named param `g`, or captured as `g`, renders and
// no-matches as `fn g(…)` / `cannot call `g“ on that lane. The VM bound the
// caller's value verbatim, so `(f (z:Integer => [z])) 3` rendered `fn
// (Integer) 3` for the interpreter's `fn g(Integer) 3` (NUR122's class, the
// twenty-sixth increment). Only an FnDefInfo of THIS registry is renamed —
// the payload the interpreter's rule names; a module wrapper (a foreign
// Registry) takes installDef's rebinding path, which this does not mirror,
// and a compiled closure keeps its render.
func nameFrameFns(fn *compiler.CompiledFn, locals []core.Value) {
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
		if !ok || fd.Name == name || fd.Registry != nil {
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
	// resolves by falling back to the interpreter — never as a goroutine stack
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
	ref, ok := h.(*compiler.CompiledFnRef)
	if !ok || ref.Prog == nil || ref.Unit < 0 || ref.Unit >= len(ref.Prog.Fns) {
		return nil, false, nil
	}
	if ref.Prog != vc.p {
		return vc.runForeignUnit(ref, args)
	}
	res, err := vc.enterCallbackUnit(vc.r, ref.Unit, bindUnitLocals(&vc.p.Fns[ref.Unit], args, ref.Captures))
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

// invokeClosureOn runs a code body for the InvokeBody seam against the
// CALLING registry: the registry the handler dispatched on (the main
// registry, a module sub-registry, or a per-connection fork that inherited
// the invoker), so a raw token body's sub-engine fallback resolves names
// exactly as the interpreter's dispatch would.
func (vc *vmContext) invokeClosureOn(reg *core.Registry, body core.Value, inputs []core.Value) ([]core.Value, error) {
	cl, ok := body.Data.(core.ClosurePayload)
	if !ok {
		// Pooled + resolved inputs, mirroring InvokeBody's no-Invoker branch
		// (never the island engine — see vmContext.islandEng's
		// non-reentrancy contract).
		return core.RunResolved(reg, inputs, core.BodyTokens(body))
	}
	// A closure minted by ANOTHER program indexes that program's Fns table, so
	// it runs in that program's own nested context rather than against vc.p
	// (see closureProgram for why this became reachable).
	if p, foreign := vc.closureProgram(cl); foreign {
		// The token seam's foreign arm: the hosted root RET takes __RC's
		// discipline, whatever seam the enclosing unit was entered through.
		prev := vc.rootRetTrim
		vc.rootRetTrim = false
		defer func() { vc.rootRetTrim = prev }()
		return vc.hostForeign(p, reg, cl.Unit, shapeInputs(cl, inputs), cl.Captures)
	}
	// Inputs fill the leading param slots, captures the trailing ones
	// (StartFnCompile registers params before captures) — the same split
	// RunUnit and runUnitNested bind, so it uses the same helper rather than
	// a second copy of the loop.
	// The TOKEN seam's entry: the root RET takes __RC's discipline, whatever
	// seam the enclosing unit was entered through.
	prev := vc.rootRetTrim
	vc.rootRetTrim = false
	res, err := vc.enterBodyUnit(reg, cl.Unit, bindUnitLocals(&vc.p.Fns[cl.Unit], shapeInputs(cl, inputs), cl.Captures))
	vc.rootRetTrim = prev
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
// shared unit recompile and refuse on operand provenance, islanding CONFORMING
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
	lookupReg := r
	if pr.Reg != nil {
		lookupReg = pr.Reg
	}
	fn := lookupReg.Lookup(pr.Word)
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
	if mr == nil || mr.Sig == nil || mr.Sig.DispatchHandler() == nil {
		// No runtime match. The interpreter's signature_error is built from its
		// live tape / forward-collection state (engine.go sigError) — the
		// written tuple, a reorder hint, two tape-only layers — which the VM
		// alone cannot reproduce. When the record carried a faithfulness plan
		// (PolyNoMatchSpec — the check pass proved, at the failed-dispatch
		// state it recovered from, that the diagnostic is rebuildable from the
		// window), raise the byte-identical signature_error right here (plan
		// 3c). Otherwise route through the whole-program fallback
		// (internal_error → RunCompiled re-runs the interpreter), which raises
		// the canonical error — sound because the interpreter takes the SAME
		// MatchSignature first-match and so reaches the same no-match.
		if mr == nil || mr.Sig == nil {
			if err := vc.polyNoMatchRaise(r, pr, fn, window, curDebug, pc); err != nil {
				return nil, err
			}
		}
		return nil, vmDeferAlt(r, curDebug, pc, "vm:poly-no-match",
			"CALL_NATIVE_POLY no match for "+pr.Word+"; deferring to interpreter for the canonical signature_error",
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
	// landing model) or REFUSES compilation (tryShapedMethodDispatch's
	// guard-owned decline), so the poly's job is exactly its recorded claim —
	// return the member value. A runtime auto-apply here would double-fire
	// against the following CALL_DYN_METHOD (span-finish underflow,
	// rand-bool's non-fn operand).
	// Enforce the recorder's result-count claim: the runtime re-match may
	// land on an overload whose arity the checker's model did not commit
	// (`set` over a dynamic receiver — Store writes in place and returns
	// nothing, Map/Flex return the container). A mismatched count would
	// silently shift every downstream operand, so defer to the interpreter
	// instead (runtimeShouldFallback — slow, not wrong).
	if len(results) != pr.NOut {
		return nil, vmDefer(r, curDebug, pc, "vm:poly-nout-drift", fmt.Sprintf(
			"poly dispatch %s: result count %d differs from the recorded claim %d; deferring to the interpreter",
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
// interpreter through the whole-program fallback, which raises the canonical
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
		// STORED mode (REFUSAL-CLOSURE.0 §6b): a body-local fn's binding is
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
		lookupReg := vc.r
		if pr.Reg != nil {
			lookupReg = pr.Reg
		}
		fd = lookupReg.Lookup(pr.Word)
		if fd == nil {
			return 0, nil, vmDefer(vc.r, curDebug, pc, "vm:user-poly-unresolved", "CALL_USER_POLY unresolved fn "+pr.Word+"; deferring to interpreter")
		}
		subset = make([]core.Signature, 0, len(pr.SigIdx))
		units = make([]int, 0, len(pr.SigIdx))
		for k, si := range pr.SigIdx {
			if k >= len(pr.Units) || k >= len(pr.Impls) ||
				si < 0 || si >= len(fd.Signatures) ||
				fd.Signatures[si].Impl != pr.Impls[k] ||
				fd.Signatures[si].TotalArgs() != n {
				return 0, nil, vmDefer(vc.r, curDebug, pc, "vm:user-poly-drift", "CALL_USER_POLY signature drift at "+pr.Word+"; deferring to interpreter")
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
			"CALL_USER_POLY no match for "+pr.Word+"; deferring to interpreter for the canonical signature_error", alt)
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
	r := vc.r
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

	if _, ok := fnVal.Data.(core.ClosurePayload); ok {
		// Pass fnVal directly so the payload's InShape rides along (invokeClosure
		// only fills param slots, but a downstream handler may read the shape).
		results, err := vc.invokeClosure(vc.r, fnVal, append([]core.Value(nil), args...))
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, r)
		}
		return append(stack[:base], results...), nil, nil
	}
	if !core.IsAppliableFn(fnVal) {
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
				return nil, nil, stampAt(err, curDebug, pc, r)
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
					return nil, nil, stampAt(err, curDebug, pc, r)
				}
				return append(stack[:base], results...), nil, nil
			}
		}
		if ent := vc.dynApplyEnter(inner, iargs); ent != nil {
			return stack[:base], ent, nil
		}
	}
	// Non-trivial fn (user body): apply via the island sub-engine, which
	// auto-applies the Function to the forward args exactly as a nested Run.
	island := make([]core.Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, r)
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
func (vc *vmContext) callDynFamily(reg *core.Registry, op compiler.Opcode, arg, frameBase int, stack []core.Value, curDebug []core.SrcPos, pc int, words []compiler.DynFrameWord, head compiler.DynApplyHead) ([]core.Value, *dynEnter, error) {
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
	r := vc.r
	if len(stack) < n+1 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_TRAIL_TOP underflow")
	}
	top := len(stack) - 1
	fnVal := stack[top]
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
	if _, ok := fnVal.Data.(core.ClosurePayload); ok {
		results, err := vc.invokeClosure(vc.r, fnVal, args)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, r)
		}
		return append(stack[:base], results...), nil, nil
	}
	if !core.IsAppliableFn(fnVal) {
		return stack, nil, nil // not callable: [args, fn] is already the interpreter's trailing residual
	}
	if err := noMatchIfSigged(reg, fnVal, args, curDebug, pc, r, head); err != nil {
		return nil, nil, err
	}
	if fnDef, ok := fnVal.Data.(core.FnDefInfo); ok && vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, r)
			}
			return append(stack[:base], results...), nil, nil
		}
	}
	if ent := vc.dynApplyEnter(fnVal, args); ent != nil {
		return stack[:base], ent, nil
	}
	island := make([]core.Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, r)
	}
	if err := vc.screenResults(results, "dynamic trailing-top result at fn-value apply", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
		return nil, nil, err
	}
	return append(stack[:base], results...), nil, nil
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
			return nil, nil, stampAt(err, curDebug, pc, r)
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
		if fn, known := vc.closureUnit(cl); !known || fn.NParams-fn.NCaptures == n {
			return commit(vc.invokeClosure(vc.r, fnVal, args))
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
			return nil, nil, vmDefer(r, curDebug, pc, "dyn-apply-top", "apply over a lens value: deferred to the interpreter")
		}
		written := []core.Value{fnVal}
		if n > 0 {
			written = append(written, args[0])
		}
		return nil, nil, stampAt(core.RuntimeNoMatch(reg, "apply", written), curDebug, pc, r)
	}
	fnDef, ok := fnVal.Data.(core.FnDefInfo)
	if !ok {
		// applyHandler's own error, byte-identical (the interpreter dispatches
		// `apply` over the same runtime value and raises exactly this).
		return nil, nil, stampAt(fmt.Errorf("apply: function value carries no FnDefInfo (got %T)", fnVal.Data), curDebug, pc, r)
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
	r := vc.r
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
		results, err := vc.invokeClosure(vc.r, fnVal, args)
		if err != nil {
			return nil, nil, stampAt(err, curDebug, pc, r)
		}
		return guard(results)
	}
	if !core.IsAppliableFn(fnVal) || fnVal.Quoted {
		// The shape claim failed outright: the read did not surface a live
		// method value. The interpreter would leave it as data and continue
		// with a DIFFERENT stack shape, which this program cannot express —
		// defer wholesale.
		return nil, nil, vmDefer(vc.r, curDebug, pc, "vm:shaped-method-not-appliable", "shaped method apply "+spec.Word+
			": value is not an appliable function at run time; deferring to the interpreter")
	}
	if fnDef, ok := fnVal.Data.(core.FnDefInfo); ok && vmNativeApplicable(vc.r, fnDef) {
		if results, done, err := vc.tryNativeFnApply(fnVal, args); done {
			if err != nil {
				return nil, nil, stampAt(err, curDebug, pc, r)
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
					return nil, nil, stampAt(err, curDebug, pc, r)
				}
				return guard(results)
			}
		}
		if ent := vc.dynApplyEnter(inner, iargs); ent != nil && dynMethodClaimOK(ent, spec.NOut) {
			return stack[:base], ent, nil
		}
	}
	island := make([]core.Value, 0, n+1)
	island = append(island, fnVal)
	island = append(island, args...)
	results, err := vc.islandRun(reg, island)
	if err != nil {
		return nil, nil, stampAt(err, curDebug, pc, r)
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
					return nil, stampAt(err, curDebug, pc, vc.r)
				}
				return append(stack[:base], results...), nil
			}
		}
	}
	results, err := vc.islandRun(reg, window)
	if err != nil {
		return nil, stampAt(err, curDebug, pc, vc.r)
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
	if len(tokens) > 0 && dynFrameSimpleWindow(tokens) {
		if ent := vc.dynApplyEnter(tokens[0], tokens[1:]); ent != nil && (len(prefix) == 0 || ent.allForward) {
			return stack[:base], ent, nil
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
		return nil, nil, stampAt(err, curDebug, pc, vc.r)
	}
	if err := vc.screenResults(results, "dynamic frame result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the replay island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, nil, err
	}
	return append(stack[:frameBase], results...), nil, nil
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
	// interpreter's 7. The def-read spelling's refusal blamed the island for
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
	reg := fnDef.Registry
	if reg == nil {
		reg = vc.r
	}
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
	if _, isBoru := mr.Sig.Impl.(*core.BoruImpl); isBoru && reg != vc.r {
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
	r := vc.r
	if len(stack) < fb.NIn {
		return nil, vmErrAt(curDebug, pc, "FALLBACK underflow at "+fb.Desc)
	}
	if fb.NIn > 1 {
		// The lowerer threads only 0 or 1 input into an island (lower.go refuses
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
		return nil, stampAt(err, curDebug, pc, r)
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

// bindDynScope executes one OpBindDynScope: install the top value under the
// name for dynamic-scope readers (OpLookupDynScope), through the same
// installer the interpreter's `def` runs; record the prior depth so the
// frame's RET (or the error unwind) truncates the binding stack back.
func (vc *vmContext) bindDynScope(curReg *core.Registry, p *compiler.Program, arg int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
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
	core.InstallDef(curReg, name, v)
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
		return nil, false, stampAt(err, curDebug, pc, vc.r)
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
		return nil, false, stampAt(err, curDebug, pc, vc.r)
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
func bindGlobal(curReg *core.Registry, gb *compiler.GlobalBindSpec, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	// The write PUSHES — the interpreter's own `def`: the check pass's
	// install was rolled back before the run (core.RestoreBindingsForReplay),
	// so there is no kept slot to overwrite, and the twin table's
	// carrier-class skip guarantees exactly one of {this push, the def's
	// twin} installs (§6.5's rollback-and-replay, the only regime since the
	// flip).
	write := func(v core.Value) {
		curReg.Defs.Push(gb.Name, core.StripAscribed(v))
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
	// value (interpreter def parity).
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
		locals[cl.Slot] = core.CloneValue(p.Consts[cl.ConstIdx])
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
	defer func() {
		if runErr != nil {
			vc.unwindDynBinds(dynBase)
		}
	}()
	// This activation counts against the shared frame ceiling; restore the
	// entry baseline on exit so sequential runs and error unwinds never leak
	// (the per-CALL_USER increments below are balanced by their RETs, and the
	// reset catches any frames still open on an early error return).
	entryFrameDepth := vc.frameDepth
	vc.frameDepth++
	defer func() { vc.frameDepth = entryFrameDepth }()
	var loops []vmLoop
	var frames []vmFrame
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
			if p.Fns[u].Reg != nil {
				curReg = p.Fns[u].Reg
			}
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
		switch in.Op {
		case compiler.OpPushConst:
			stack = append(stack, p.Consts[in.Arg])
		case compiler.OpPushConstFresh:
			// Mint a fresh container identity for a compound literal the
			// enclosing fn unit re-evaluates per call — interpreter parity
			// for `(mk) eq (mk)` (see OpPushConstFresh in bytecode.go).
			stack = append(stack, core.CloneValue(p.Consts[in.Arg]))
		case compiler.OpPushConstFreshLocal:
			// A multi-read compound body literal: construct ONE fresh instance per
			// call, seated in a frame local, shared by every read site (see
			// OpPushConstFreshLocal / seatConstLocal for the sentinel + parity).
			stack = append(stack, seatConstLocal(p, locals, p.ConstLocals[in.Arg]))
		case compiler.OpPushLocal:
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
						"splice of a code-bearing payload; deferring to the interpreter")
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
			return nil, stampAt(ae, curDebug, pc, r)
		case compiler.OpDispatchRematch:
			// Terminal either way: the rematch raises or defers (vm_rematch.go).
			return nil, vc.dispatchRematch(&p.Dispatches[in.Arg], stack, curDebug, pc)
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
			stack = append(stack, core.NewTypeLiteral(t))
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
					return nil, stampAt(err, curDebug, pc, r)
				}
			}
			vc.ensureInvoker(curReg)
			results, err := s.Sig.DispatchHandler()(args, curReg.Contexts.TopData(), nil, curReg)
			if err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			// Belt-and-braces: a handler that returns tape tokens (to
			// be re-stepped by the engine) must never have been
			// compiled — the emitter refuses fn-invoking and
			// code-splicing words. Fail loudly, never push tokens as
			// data.
			if err := vc.screenResults(results, "handler result at "+s.Word, curDebug, pc); err != nil {
				return nil, err
			}
			stack = append(stack, results...)
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
			bound, err := core.RunTypedBind(r, &p.TypedBinds[in.Arg], core.StripAscribed(stack[len(stack)-1]))
			if err != nil {
				return nil, stampAt(err, curDebug, pc, r)
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
		case compiler.OpCallDynamic, compiler.OpCallDynamicTrailing, compiler.OpCallDynamicMixed,
			compiler.OpCallDynTrailTop, compiler.OpCallDynApplyTop, compiler.OpCallDynApplyOne, compiler.OpCallDynTrailKeepQ, compiler.OpCallDynFrame, compiler.OpCallDynMethod:
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
			ns, ent, err := vc.callDynFamily(curReg, in.Op, int(in.Arg), fb, stack, curDebug, pc, dynFrameWordsAt(p, curUnit, pc), dynApplyNameAt(p, curUnit, pc))
			if err != nil {
				return nil, err
			}
			stack = ns
			if ent != nil {
				// The Apply kernel's frame push, modelled on OpCallUserPoly
				// (the other site that learns its unit at RUN time): re-check
				// the param contract, push a frame, enter. Deliberately NOT a
				// nested run — a fn APPLICATION is a call, and bracketing it as
				// a body added a per-body context frame the interpreter's call
				// does not (vm_dyn_apply.go).
				fn := &p.Fns[ent.unit]
				if err := checkParamContract(r, fn, ent.locals); err != nil {
					return nil, stampAt(err, curDebug, pc, r)
				}
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth(), retFn: ent.retFn})
				vc.frameDepth++ // balanced by the matching RET, like OpCallUser
				nameFrameFns(fn, ent.locals)
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
			// re-check the param contract, push a frame.
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
			if err := checkParamContract(r, fn, nl); err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth()})
			vc.frameDepth++ // balanced by the matching RET, like OpCallUser
			nameFrameFns(fn, nl)
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
			if fn.Reg != nil && fn.Reg.ModuleRef != "" {
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
			// declared type. Raises the same signature_error the interpreter raises.
			if err := checkParamContract(r, fn, nl); err != nil {
				return nil, stampAt(err, curDebug, pc, r)
			}
			if in.Op == compiler.OpCallUser {
				frames = append(frames, vmFrame{retUnit: curUnit, retPC: pc + 1, locals: locals, loopBase: len(loops), stackBase: len(stack), dynBase: len(vc.dynBinds), argsBase: r.Args.Depth()})
				vc.frameDepth++ // balanced by the matching RET below
				nameFrameFns(fn, nl)
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
				nameFrameFns(fn, nl)
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
		case compiler.OpBindDynScope:
			ns, err := vc.bindDynScope(curReg, p, int(in.Arg), stack, curDebug, pc)
			if err != nil { //covergate:allow bindDynScope's only error paths are its own allow-listed defensive guards (underflow / bad name const), unreachable without a bytecode-level fault (§compiler)
				return nil, err
			}
			stack = ns
		case compiler.OpBindGlobal:
			ns, err := bindGlobal(curReg, &p.GlobalBinds[in.Arg], stack, curDebug, pc)
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
			core.ApplyBindTwin(curReg, p.BindTwins[in.Arg], p.BindTwinEntries[in.Arg])
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
		case compiler.OpLookupDynScope:
			// The interpreter's stepWord simple-value substitution, at run
			// time: read the name's live binding. A miss, or a binding the
			// substitution would DISPATCH instead of push (a Function / class /
			// splice / reach), defers to the interpreter (slow, not wrong).
			name, nerr := p.Consts[in.Arg].AsConcreteString()
			if nerr != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
				return nil, vmErrAt(curDebug, pc, "LOOKUP_DYN_SCOPE bad name const")
			}
			v, ok := curReg.Defs.Top(name)
			if !ok {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-miss", "dynamic-scope read miss for `"+name+"`; deferring to the interpreter")
			}
			switch v.Data.(type) {
			case core.FnDefInfo, *core.ClassTypeInfo:
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-dispatching", "dynamic-scope read of a dispatching binding `"+name+"`; deferring to the interpreter")
			}
			if core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v) {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-active-token", "dynamic-scope read of an active token `"+name+"`; deferring to the interpreter")
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
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-miss", "dynamic-scope data read miss for `"+name+"`; deferring to the interpreter")
			}
			if _, isClass := v.Data.(*core.ClassTypeInfo); isClass {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-class", "dynamic-scope data read of a class binding `"+name+"`; deferring to the interpreter")
			}
			if core.IsSplice(v) || core.IsReach(v) || core.IsWord(v) || core.IsMark(v) || core.IsMove(v) {
				return nil, vmDefer(vc.r, curDebug, pc, "vm:dyn-scope-data-active-token", "dynamic-scope data read of an active token `"+name+"`; deferring to the interpreter")
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
				} else {
					trimmed, err = checkReturnContract(r, contract, stack, stackBase, len(frames) > 0, core.SrcPos{})
				}
				if err != nil {
					return nil, stampAt(err, curDebug, pc, r)
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
				// activation installed pop here, like any frame exit.
				vc.unwindDynBinds(dynBase)
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
func stampAt(err error, debug []core.SrcPos, pc int, r *core.Registry) error {
	ae, ok := err.(*core.BoruError)
	if !ok || pc < 0 || pc >= len(debug) {
		return err
	}
	if ae.Row == 0 {
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
		// here — see design/PARAM-GUARD-SKIP-MISCOMPILE.0.md; this guard catches the
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
	if fn.RetReplay && fn.Reg != nil && fn.Reg != r {
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
				"dynamic frame replay %s: result count %d differs from the declared %d; deferring to the interpreter",
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
	return core.MakeBoruErrorAt("internal_error",
		fmt.Sprintf("bytecode: internal: %s (pc=%d, src %d:%d)", msg, pc, pos.Row, pos.Col),
		"", "", "", pos)
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
