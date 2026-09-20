package core

import (
	"sync/atomic"
)

// The effect ledger — what is left of the compiled-mode effect fence
// (design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore, contract C1, the L-DUP
// class), now an OBSERVABILITY SEAM and nothing more.
//
// The fence existed because RunCompiled used to resolve a compile failure or a
// runtime internal_error by silently re-running the WHOLE source on the
// interpreter, and that re-run was sound only while nothing observable had
// escaped: SnapshotForCompile/RestoreForCompile roll back registry scopes, but
// nothing can un-print already-written output or un-send a network payload, so
// a re-run after an effect DUPLICATED it (the full trie smoke suite printed
// twice). The fence made each fallback arm prove "nothing escaped yet" before
// re-running.
//
// Nothing re-runs the source any more (design/COMPILABLE-SUBSET.md §1: a
// program compiles and its bytecode runs, or it does not compile and that is an
// error), so there is no arm left to gate and the duplicate-effect class is
// gone by construction. The writer-wrapping half went with it: ArmEffectFence
// wrapped Output/ErrOutput so a print counted, and with no arm to block it
// wrapped the writers for nobody.
//
// What survives is the counter itself, because the effect seams are a useful
// PROBE: a test asserts that a denied request never reached the network, or
// that a bailed callback body ran exactly once, by reading Count() across the
// operation. It gates nothing at run time — treat it like the interp-entry and
// runtime-bail hooks (interp_entry.go), a test seam rather than API.

// EffectLedger counts observable side effects emitted during a request.
// Concurrent branches share the parent's ledger pointer and may mark it
// simultaneously, so the counter is atomic.
type EffectLedger struct{ n atomic.Uint64 }

// Note marks one observable effect. Nil-safe: a registry assembled without
// NewRegistry has no ledger, and counting nothing there is harmless now that
// no decision reads the count.
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
// the effecting words call so a test can see them: the write word (fileio.go
// doWrite), folder (natives.go doFolder), HTTP fetch/direct (fetch.go doFetch),
// the IO handle/binary/lock/temp/mmap/fs words, and the model build FS
// (modules/model.go fileOpsFS). Nil-safe on both receiver and ledger: direct
// handler-call tests pass a nil registry, and counting nothing there matches
// the nil-ledger contract.
func (r *Registry) NoteEffect() {
	if r == nil {
		return
	}
	r.Effects.Note()
}
