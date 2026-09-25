package eng

import (
	"fmt"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vmInternalError is the ONE text both VM panic guards surface — runVMEntry's
// top-level recover and runForeignUnit's per-callback one. RunCompiled and
// InvokeCompiled both key their degrade-to-interpreter decision on the
// internal_error CLASS, so the two guards must not drift apart.
func vmInternalError(rec any, src string) error {
	e := core.MakeBoruError("internal_error",
		fmt.Sprintf("internal bytecode VM error: %v", rec), "", src, "")
	e.VMDefer = true
	return e
}

// Foreign (detached) unit hosting — the half of InvokeCallback's contract that
// was missing.
//
// InvokeCallback promises to run a stamped callback body "on the VM when it
// compiled to a unit (nested in a live run, or fresh on an idle registry)".
// The idle half was real; the NESTED half was not. runUnitNested compared
// ref.Prog against the running program and declined every mismatch, and a
// DETACHED ref (StampDetachedFn) is a mismatch BY CONSTRUCTION — it is compiled
// on an isolated fork into its own standalone one-unit Program. So every
// runtime-stamped callback reached from inside a live compiled run — the
// predicate types being the whole population of them in the corpus — stamped
// successfully, reported Stamped:true in the stamp ledger, and then ran on the
// interpreter anyway. The ledger said compiled; the runtime interpreted. That
// divergence is what the interp-entry census measures and what T2 forbids.
//
// Hosting a foreign program's unit is not the same as starting a run: the
// concurrency guard is already held by the enclosing run, and re-taking it
// (RunUnit) is exactly what would fail. What the foreign unit needs is a
// vmContext of its OWN — bound to ITS program, so unit indices, closure units
// and constants resolve against the right artifact — nested inside the live
// one.
//
// Four pieces of state are deliberately SHARED with the enclosing run rather
// than restarted, because each is a runaway guard that a per-callback reset
// would silently defeat:
//
//   - steps      — copied in and handed back, so a hot callback cannot mint
//     itself a fresh step budget on every invoke.
//   - frameDepth — seeded from the enclosing depth, so recursion THROUGH a
//     detached callback (a predicate whose body triggers another typed bind)
//     is bounded by the same ceiling as recursion inside one program, and
//     fails as tape_exhausted instead of overflowing the Go stack.
//   - ceiling / stepLimit — the registry's, unchanged.
//
// Everything else is per-context and must NOT be shared: dynBinds (the foreign
// run unwinds its own trail), islandEng (the enclosing run's island engine may
// be mid-flight — this is precisely the re-entrancy its contract forbids), and
// foreignInvokers (cleared here exactly as runVMEntry clears its own).

// runForeignUnit runs ref's unit — belonging to a program OTHER than the one
// vc is running — nested inside vc's live run, and reports whether a VM path
// took it. handled=false leaves the callback to the interpreter, byte-
// identically to the behaviour before foreign hosting existed.
//
// The panic guard is local rather than borrowed from the enclosing
// runVMEntry's: a soundness bailout inside ONE callback must degrade THAT
// callback (InvokeCompiled reports it), not abort the whole enclosing program.
func (vc *vmContext) runForeignUnit(ref *compiler.CompiledFnRef, args []core.Value, named bool) (res []core.Value, handled bool, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			res, handled, err = nil, true, vmInternalError(rec, vc.r.Source)
		}
	}()
	// The fn-VALUE seam's foreign arm: the hosted root RET takes the CallBoru
	// discipline, as enterCallbackUnit's does for an in-program ref.
	prev, prevNamed := vc.rootRetTrim, vc.rootRetNamed
	vc.rootRetTrim, vc.rootRetNamed = !named, named
	defer func() { vc.rootRetTrim, vc.rootRetNamed = prev, prevNamed }()
	// The value's own frame (see pushRootArgs): its call args ride in from
	// the seam, for the DynEnv unit that reads them.
	defer pushRootArgs(vc.r, ref.Prog, args)()
	res, err = vc.hostForeign(ref.Prog, vc.r, ref.Unit, args, ref.Captures, false)
	return res, true, err
}

// hostForeign is the nested-hosting body both foreign entries share:
// runForeignUnit for a detached REF, and invokeClosureOn for a closure VALUE
// whose program is not the running one. It builds p's own vmContext, points
// the registry's body seams at it for the duration, and restores everything on
// the way out.
//
// reg is the registry the body runs against — vc.r for a detached ref, the
// CALLING registry for a closure (invokeClosureOn's contract: a module
// sub-registry or a per-connection fork resolves names as its own dispatch
// would).
func (vc *vmContext) hostForeign(p *compiler.Program, reg *core.Registry, unit int, inputs, captures []core.Value, flowEscapes bool) ([]core.Value, error) {
	r := vc.r
	sub := &vmContext{
		p:           p,
		r:           r,
		flowEscapes: flowEscapes,
		ceiling:     vc.ceiling,
		// The seam the host was entered through decides the hosted root RET's
		// return discipline (runForeignUnit: the fn-VALUE seam; invokeClosureOn:
		// the token seam).
		rootRetTrim:  vc.rootRetTrim,
		rootRetNamed: vc.rootRetNamed,
		stepLimit:    vc.stepLimit,
		steps:        vc.steps,
		argsFloor:    r.Args.Depth(),
		frameDepth:   vc.frameDepth,
	}
	// Registered first so it runs last of this function's defers: the budget is
	// handed back on every path, a bailed body included.
	defer func() { vc.steps = sub.steps }()
	if p.DynEnv {
		// The DynEnv args bracket, per runVMEntry: an error unwind returns
		// straight out of sub.run, so rebalance to the entry depth on every
		// path rather than only the clean one.
		defer r.Args.Truncate(sub.argsFloor)
	}
	// The foreign program owns body execution for the duration: a closure it
	// pushes, or a further detached ref it reaches, indexes ITS Fns table, so
	// both seams must point at sub while it runs. Restored on exit so the
	// enclosing run resumes with its own.
	prevInvoker := r.Invoker
	prevNested := r.NestedRunner
	r.Invoker = sub.invokeClosureOn
	r.NestedRunner = sub.runUnitNested
	defer func() {
		r.Invoker = prevInvoker
		r.NestedRunner = prevNested
		for _, fr := range sub.foreignInvokers {
			fr.Invoker = nil
		}
	}()
	res, err := sub.enterBodyUnit(reg, unit, bindUnitLocals(reg, &p.Fns[unit], inputs, captures))
	// A KEEP-DEFS unit hosted here (a run-time token body, StampTokenBody:
	// NUR202) leaves the installs its run made on the sub-context's trail,
	// for "the enclosing frame" to pop — and that frame is THIS context's
	// current activation (the native that drove the body runs inside it),
	// so the entries move to this trail, where the activation's RET, the
	// run's end at root, or an error unwind truncates them exactly as its
	// own. Every other unit unwound its trail at its own RET (nothing to
	// move); a kept unit's raise keeps them too, the interpreter's
	// leak-then-raise.
	if len(sub.dynBinds) > 0 {
		vc.dynBinds = append(vc.dynBinds, sub.dynBinds...)
	}
	return res, err
}

// closureProgram answers whether cl was minted by a program OTHER than the one
// vc is running, and returns it when so.
//
// A ClosurePayload's Unit is an index into its OWN program's Fns table. For as
// long as a closure could only be invoked under the program that pushed it,
// that was a distinction without a difference and the payload carried no
// program at all. Nested foreign hosting ends that: while a detached unit runs,
// the registry's Invoker points at the FOREIGN program's context, so a closure
// belonging to the enclosing one would index the wrong table — out of range
// (caught, degraded) or, worse, a valid index naming a different body, which is
// a silent wrong answer. This is the check that makes that impossible rather
// than merely unlikely.
//
// A payload with no program recorded reads as the running program, which is
// exactly what it did before the field existed. Note the p != nil arm: a
// hand-built NewClosure(nil, …) boxes a TYPED nil, so the type assertion
// SUCCEEDS and only the pointer test separates "no identity" from "a real
// foreign program" — the same typed-nil trap the S3 plan installers document.
func (vc *vmContext) closureProgram(cl core.ClosurePayload) (*compiler.Program, bool) {
	p, ok := cl.Prog.(*compiler.Program)
	return p, ok && p != nil && p != vc.p
}

// shapeInputs applies a closure's declared input convention to the inputs a
// higher-order handler hands it, returning the slice the param slots bind from.
//
// Only ClosureInStackPair does anything: the handler pushed in STACK order
// (deeper first) and the body's params were declared in the top-down order the
// interpreter's MatchSignature produces, so binding reverses them. Every other
// shape is already positional — the handler's order IS the slot order — and is
// returned untouched. The reverse is on a COPY: the caller's slice is a
// handler's own buffer, reused across iterations of a fold.
//
// The reversal is UNCONDITIONAL on the shape, with no input-count guard. A
// `len(inputs) < 2` early return would be a pure allocation saving — reversing
// nothing or one thing is the identity — but it would read as an arity-keyed
// branch in a kernel whose one argument rule holds at EVERY arity, and a
// branch that reads like an exception eventually gets treated as one.
func shapeInputs(cl core.ClosurePayload, inputs []core.Value) []core.Value {
	if cl.InShape != compiler.ClosureInStackPair {
		return inputs
	}
	out := make([]core.Value, len(inputs))
	for i, v := range inputs {
		out[len(inputs)-1-i] = v
	}
	return out
}
