package core

import (
	"io"
	"sync/atomic"
)

// The compiled-mode effect fence (design/RUNTIME-INDEPENDENCE-COMPLETION-
// PLAN.0.md, contract C1 — the L-DUP class).
//
// RunCompiled resolves a genuine refusal or a runtime internal_error by
// silently re-running the WHOLE source on the interpreter. That re-run is
// sound only while the compiled request has emitted NO observable effect:
// SnapshotForCompile/RestoreForCompile roll back registry scopes, but nothing
// can un-print already-written output or un-send a network payload, so a
// re-run after an effect DUPLICATES it (design/legacy/VOXGIG-COMPILE-LEAVES.2.ignore
// §L-DUP — the full trie smoke suite printed twice). The pure-value
// differential corpus never exercises emit-then-fall-back, which is exactly
// why the class ships latent; the ledger makes the fallback arms prove
// "nothing escaped yet" before re-running.
//
// The ledger is a monotonic generation counter, not an effect log: fence
// sites snapshot Count() before the check pass and compare afterwards. Writes
// to the registry's output writers mark it via ArmEffectFence's wrapper —
// which forks (ForkConcurrent's shallow copy) and module sub-registries
// (RunModuleBody copies the parent's writers) inherit by value — and
// non-writer effect seams call Registry.NoteEffect directly.

// EffectLedger counts observable side effects emitted during a compiled-mode
// request. Concurrent branches share the parent's ledger pointer and may mark
// it simultaneously, so the counter is atomic.
type EffectLedger struct{ n atomic.Uint64 }

// Note marks one observable effect. Nil-safe: a registry assembled without
// NewRegistry has no ledger, and counting nothing there keeps the fallback
// behaviour it had before the fence existed.
func (l *EffectLedger) Note() {
	if l == nil {
		return
	}
	l.n.Add(1)
}

// Count returns the number of effects marked so far (0 for a nil ledger).
func (l *EffectLedger) Count() uint64 {
	if l == nil {
		return 0
	}
	return l.n.Load()
}

// NoteEffect marks one observable effect on the registry's ledger — the seam
// non-writer effects (file writes, network sends) call so the fallback fence
// sees them alongside output writes. Production callers: the write word
// (fileio.go doWrite), folder (natives.go doFolder), HTTP fetch/direct
// (fetch.go doFetch), and the model build FS (modules/model.go fileOpsFS).
// Nil-safe on both receiver and ledger: direct handler-call tests pass a nil
// registry, and counting nothing there matches the nil-ledger contract.
func (r *Registry) NoteEffect() {
	if r == nil {
		return
	}
	r.Effects.Note()
}

// noteEffectWriter marks the ledger on every write the underlying writer
// ACCEPTED (n > 0 — partial writes escaped too). Delegating first is what
// keeps the fence honest in both directions: a writer that rejects the bytes
// outright (a closed pipe returning 0, err) provably emitted nothing, so
// counting it would block a still-safe interpreter fallback. Wrapping the
// registry's writers once (ArmEffectFence) covers the whole
// print/stdout/stderr effect class without instrumenting each printing word:
// forks and module sub-registries copy the wrapped writer VALUE, so their
// writes count too.
type noteEffectWriter struct {
	w io.Writer
	l *EffectLedger
}

func (nw noteEffectWriter) Write(p []byte) (int, error) {
	n, err := nw.w.Write(p)
	if n > 0 {
		nw.l.Note()
	}
	return n, err
}

// Unwrap returns the writer this fence wraps, so a caller that must ask the
// OPERATING SYSTEM about the destination — `IO.is-tty`, and the CLI's own
// auto-colour decision, both of which stat the endpoint — reaches the real
// *os.File instead of this instrument. Without it the fence silently changed an
// observable answer: compiled mode arms the fence, so `IO.is-tty (IO.stdout)`
// reported false on a real terminal while the interpreter reported true. Same
// contract, and same reason, as SyncWriter.Unwrap (fork.go).
func (nw noteEffectWriter) Unwrap() io.Writer { return nw.w }

// ArmEffectFence wraps the registry's Output/ErrOutput writers so every write
// marks the effect ledger, and returns a restore func reinstating the
// original writers. Compiled-mode entry points arm the fence BEFORE the check
// pass: the check pass executes module imports, so an import-time effect (a
// module body printing at load) must count against the refusal fallback arm
// too, not only the runtime-bail arm.
func (r *Registry) ArmEffectFence() func() {
	savedOut, savedErr := r.Output, r.ErrOutput
	if r.Output != nil {
		r.Output = noteEffectWriter{w: r.Output, l: r.Effects}
	}
	if r.ErrOutput != nil {
		r.ErrOutput = noteEffectWriter{w: r.ErrOutput, l: r.Effects}
	}
	return func() {
		r.Output, r.ErrOutput = savedOut, savedErr
	}
}
