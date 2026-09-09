package core

// CompiledRuntime is the core→eng inversion seam (Stage 1 of the
// four-piece split, design/ENG-FOUR-PIECE.0.md seam S4): everything the
// pure interpreter needs FROM the bytecode runtime goes through this
// interface, generalizing the Registry.Invoker precedent. The core
// default declines every operation, so a core-only build simply takes
// the interpreter path; the VM piece installs the real runtime at init.
type CompiledRuntime interface {
	// InvokeCompiled attempts the stamped-unit fast path for a matched
	// signature (ref freshness, JIT re-stamp, the effect fence, and the
	// internal-error fallback classification are the runtime's own
	// business). ran=false → the caller owns the interpreter path.
	//
	// On ran=false the error is NOT the callee's answer — it is a report
	// about the attempt, and it distinguishes the two declines the caller
	// cannot otherwise tell apart. Nil: the unit was never hosted (no ref,
	// nothing able to host it), so the interpreter path that follows is a
	// plain island. Non-nil: the unit RAN and DEFERRED, and the runtime
	// swallowed that internal error for the caller to resolve by
	// interpreting — a designed bail, already in the bail ledger, so the
	// replay is attributed rather than counted a second time
	// (bailReplayAttribution).
	InvokeCompiled(r *Registry, sig *Signature, args []Value) (res []Value, err error, ran bool)
	// StampDetached compiles and stamps a detached fn at install time
	// (InstallType's runtime-stamping route). A decline is silent: the
	// binding stays interpreter-dispatched.
	StampDetached(r *Registry, fd FnDefInfo, pos SrcPos)
	// ClosureAsFnDef bridges a compiled closure VALUE (a ClosurePayload the
	// interpreter meets on the tape — an island's sub-engine re-stepping a
	// shuffled `each` element, NUR124's payload axis) to the FnDefInfo the
	// interpreter would have minted for the same source: one dispatchable
	// signature over the unit's declared param contract, applying the
	// closure through the registry's body-closure invoker. ok=false leaves
	// the value as data — outside a VM run, or for a unit the bridge cannot
	// describe.
	ClosureAsFnDef(r *Registry, v Value) (fnv Value, ok bool)
}

// noCompiledRuntime is the interpreter-only default: every operation
// declines, mirroring a build with no VM linked. It declines with a NIL error
// throughout — nothing ran, so there is no defer to attribute.
type noCompiledRuntime struct{}

func (noCompiledRuntime) InvokeCompiled(*Registry, *Signature, []Value) ([]Value, error, bool) {
	return nil, nil, false
}
func (noCompiledRuntime) StampDetached(*Registry, FnDefInfo, SrcPos) {}
func (noCompiledRuntime) ClosureAsFnDef(_ *Registry, v Value) (Value, bool) {
	return v, false
}

var compiledRuntime CompiledRuntime = noCompiledRuntime{}

// InstallCompiledRuntime replaces the process-wide compiled runtime —
// called once from the VM piece's init. Exposed for the piece cut; a
// second install (tests) must restore the previous value.
func InstallCompiledRuntime(rt CompiledRuntime) (prev CompiledRuntime) {
	prev = compiledRuntime
	compiledRuntime = rt
	return prev
}
