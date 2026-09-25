package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vmCompiledRuntime is the bytecode runner's CompiledRuntime — the real
// implementation of core's S4 seam (design/legacy/ENG-FOUR-PIECE.0.ignore). It
// owns the stamped-ref freshness/JIT-re-stamp dance, the C1 effect
// fence around the attempt, and the internal-error degrade decision;
// core's InvokeCallback sees only ran/declined.
type vmCompiledRuntime struct{}

func init() { core.InstallCompiledRuntime(vmCompiledRuntime{}) }

func (vmCompiledRuntime) InvokeCompiled(r *core.Registry, sig *core.Signature, args []core.Value) ([]core.Value, error, bool) {
	return invokeCompiled(r, sig, args, false)
}

// InvokeCompiledStrict is InvokeCompiled for a NAMED fn call: the unit's
// root RET takes the frame's return contract (NUR191, invokeCompiledUnit's
// named entry).
func (vmCompiledRuntime) InvokeCompiledStrict(r *core.Registry, sig *core.Signature, args []core.Value) ([]core.Value, error, bool) {
	return invokeCompiled(r, sig, args, true)
}

func invokeCompiled(r *core.Registry, sig *core.Signature, args []core.Value, named bool) ([]core.Value, error, bool) {
	ref := compiler.CompiledRef(sig)
	if ref != nil && ref.Prog != nil && !ref.DepsFresh(r) {
		ref = ref.JitRestamp(r)
	}
	if ref == nil || ref.Prog == nil {
		return nil, nil, false
	}
	res, err, ran := invokeCompiledUnit(r, ref, args, named)
	if !ran {
		return nil, nil, false
	}
	// A soundness bail inside the callback's unit used to ride back with
	// ran=false so the caller retried the whole body on the interpreter,
	// guarded by the effect fence so a callback that had written to the peer
	// and THEN bailed did not write twice. Nothing retries now: the bail is a
	// compiler defect and it surfaces, effect or no effect.
	return res, err, true
}

// ClosureAsFnDef is the VALUE-path twin of closureAsWord (NUR124's payload
// axis): an island's sub-engine re-stepping a compiled closure it finds on
// the tape — a shuffled `each` element, a native's fn result — asks for the
// FnDefInfo the interpreter would have minted there. The running VM's
// invoker (Registry.Invoker, the body-closure seam RunProgram installs)
// hosts the apply, so a closure met outside any VM run stays data, as does
// one whose program or unit the payload cannot name.
func (vmCompiledRuntime) ClosureAsFnDef(r *core.Registry, v core.Value) (core.Value, bool) {
	cl, ok := v.Data.(core.ClosurePayload)
	if !ok || r == nil || r.Invoker == nil {
		return v, false
	}
	prog, ok := cl.Prog.(*compiler.Program)
	if !ok || prog == nil || cl.Unit < 0 || cl.Unit >= len(prog.Fns) {
		return v, false
	}
	// The bridged value lives for ONE dispatch: the interpreter keeps the
	// closure itself on the tape (fnDefAtPointer) and uses the bridge only
	// to decide and run that dispatch, so the run's invoker captured here
	// never outlives the run — a parked closure escapes as the payload, not
	// as a handler bound to this run's context (Codex P2 on PR #444).
	invoke := r.Invoker
	// Applied after the interpreter's dispatch matched the bridged
	// signature: SigMatched, so the invoker applies the unit positionally
	// (ClosurePayload.SigMatched).
	matched := core.ClosureSigMatched(v)
	fnv, ok := closureFnDef(&prog.Fns[cl.Unit], cl.Ident, func(args []core.Value) ([]core.Value, error) {
		return invoke(r, matched, args)
	})
	if !ok {
		return v, false
	}
	return fnv, true
}

func (vmCompiledRuntime) StampDetached(r *core.Registry, fd core.FnDefInfo, pos core.SrcPos) {
	// fd arrives with the stamp-event name applied by the caller; the
	// ref lands on the shared *BoruImpl pointer (stampCompiledRef), so
	// the same copy serves both calls.
	if ref, stampOK := compiler.StampDetachedFn(r, fd, pos); stampOK {
		compiler.StampCompiledRef(fd, ref)
	}
}

// LazyStamp is the compiled runtime's first-application stamp — the
// detached stamp made universal (compiler.LazyStampFnSig): it stamps, or
// finds the earlier stamp of, the sig a fn VALUE's application matched, and
// says whether the sig now carries a unit for InvokeCompiled to run.
func (vmCompiledRuntime) LazyStamp(r *core.Registry, fd core.FnDefInfo, sig *core.Signature, pos core.SrcPos) bool {
	if sig == nil {
		return false
	}
	return compiler.LazyStampFnSig(r, fd, sig, pos) != nil
}
