package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vmCompiledRuntime is the bytecode runner's CompiledRuntime — the real
// implementation of core's S4 seam (design/ENG-FOUR-PIECE.0.md). It
// owns the stamped-ref freshness/JIT-re-stamp dance, the C1 effect
// fence around the attempt, and the internal-error degrade decision;
// core's InvokeCallback sees only ran/declined.
type vmCompiledRuntime struct{}

func init() { core.InstallCompiledRuntime(vmCompiledRuntime{}) }

func (vmCompiledRuntime) InvokeCompiled(r *core.Registry, sig *core.Signature, args []core.Value) ([]core.Value, error, bool) {
	ref := compiler.CompiledRef(sig)
	if ref != nil && ref.Prog != nil && !ref.DepsFresh(r) {
		ref = ref.JitRestamp(r)
	}
	if ref == nil || ref.Prog == nil {
		return nil, nil, false
	}
	// The writer fence is armed around the attempt because a DETACHED
	// callback fires after the enclosing compiled run disarmed its own
	// fence: without the wrap, a callback that PRINTS and then bails
	// would leave the ledger untouched and the interpreter retry would
	// emit the output a second time. Nested invocations are already
	// armed; the second wrap only double-counts, and the fence reads
	// deltas, not magnitudes.
	disarm := r.ArmEffectFence()
	effectsAt := r.Effects.Count()
	res, err, ran := invokeCompiledUnit(r, ref, args)
	disarm()
	if !ran {
		return nil, nil, false
	}
	if !core.IsInternalErr(err) {
		return res, err, true
	}
	// C1 effect fence (effects.go): the interpreter retry re-runs the
	// whole body, so it is sound only while the failed unit emitted NO
	// observable effect — a callback that wrote to the peer and THEN
	// bailed must surface the internal_error rather than double its
	// output.
	if r.Effects.Count() != effectsAt {
		return nil, err, true
	}
	// The swallowed internal error rides back WITH ran=false: it is the only
	// thing that tells the caller its interpreter path is a designed defer's
	// replay rather than an island (CompiledRuntime.InvokeCompiled's contract).
	// The caller must not surface it — vmDefer already recorded the bail, and
	// the interpreter is about to produce the canonical answer.
	return nil, err, false
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
	invoke := r.Invoker
	fnv, ok := closureFnDef(&prog.Fns[cl.Unit], func(args []core.Value) ([]core.Value, error) {
		return invoke(r, v, args)
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
