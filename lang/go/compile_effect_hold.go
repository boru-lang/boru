package lang

import (
	"io"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// A compile pass is the program's own execution of everything the check pass
// runs for real — a RunInCheckMode word, an imported module's body (the VM
// never re-imports) — so its effects are the program's effects, emitted once.
// CompileCheck's specialisation retry re-runs the pass, and a re-run repeats
// every one of them: the duplicate-effect class (L-DUP) that removing the
// interpreter re-run closed by construction (effects.go's history).
//
// checkPassHold keeps the retry from reopening it. The first pass's output —
// both writers, in the order written — is HELD rather than written: a pass
// that is kept flushes it, a pass that is retried discards it (the retry
// writes its own). What cannot be held — a file write, a network send (the
// effect ledger counts both), a stdin read (native.StdinReads) — makes the
// first pass unrepeatable, and CompileCheck then does not retry.

// heldChunk is one held write: the bytes and the writer they were bound for.
type heldChunk struct {
	w io.Writer
	b []byte
}

// checkPassHold is one pass's held output and the unrepeatable-effect marks
// it started from.
type checkPassHold struct {
	r           *native.Registry
	out, errOut io.Writer
	effects     uint64
	stdinReads  uint64

	mu       sync.Mutex
	chunks   []heldChunk
	released bool
}

// heldWriter is the writer a held pass sees in place of dst. After the hold
// is released it writes straight through: a module sub-registry the pass
// built keeps the writer it copied (RunModuleBody), and its fns print at run
// time.
type heldWriter struct {
	h   *checkPassHold
	dst io.Writer
}

func (w heldWriter) Write(p []byte) (int, error) {
	w.h.mu.Lock()
	if w.h.released {
		w.h.mu.Unlock()
		return w.dst.Write(p)
	}
	w.h.chunks = append(w.h.chunks, heldChunk{w: w.dst, b: append([]byte(nil), p...)})
	w.h.mu.Unlock()
	return len(p), nil
}

// Unwrap names the destination, so a stream probe (IO.is-tty) inspects what
// the program is talking to, not the hold.
func (w heldWriter) Unwrap() io.Writer { return w.dst }

// holdCheckPass starts holding r's output and marks its unrepeatable-effect
// counts. An absent writer stays absent: there is nothing to hold, and a
// word that tests for one must still find none.
func holdCheckPass(r *native.Registry) *checkPassHold {
	h := &checkPassHold{r: r, out: r.Output, errOut: r.ErrOutput, effects: r.Effects.Count(), stdinReads: native.StdinReads(r)}
	r.Output = h.wrap(h.out)
	r.ErrOutput = h.wrap(h.errOut)
	return h
}

func (h *checkPassHold) wrap(dst io.Writer) io.Writer {
	if dst == nil {
		return nil
	}
	return heldWriter{h: h, dst: dst}
}

// unrepeatable reports whether the held pass did something a re-run would
// repeat and the hold could not keep: a counted effect or a stdin read.
func (h *checkPassHold) unrepeatable() bool {
	return h.r.Effects.Count() != h.effects || native.StdinReads(h.r) != h.stdinReads
}

// release restores the writers and, when keep, writes the held output to
// them in the order it was written. A discarded pass's output is dropped:
// the pass that replaces it writes its own.
func (h *checkPassHold) release(keep bool) {
	h.mu.Lock()
	chunks := h.chunks
	h.chunks = nil
	h.released = true
	h.mu.Unlock()
	h.r.Output, h.r.ErrOutput = h.out, h.errOut
	if !keep {
		return
	}
	for _, c := range chunks {
		_, _ = c.w.Write(c.b)
	}
}
