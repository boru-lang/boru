package stackform

import (
	core "github.com/boru-lang/boru/core/go"
)

// Compile runs `tokens` through an Engine with a StackForm-recording
// Recorder installed and returns both the resulting top-level stack
// and the recorded StackForm.
//
// The recording side effect is exactly the architecture proposed by
// design/legacy/boru-bytecode-report.0.ignore §1.2 ("the compiler is the
// checker with a recording side effect") — except we record on the
// normal-execution path, not the carrier-only check path, so the
// values stored in PushLit ops are the actual data the engine saw.
//
// Determinism: callers that need reproducible Compile output (e.g.
// the PBT reducer shrinking a generator program) should use a
// seeded rand instance via `rand.with-seed N` rather than the
// clock-seeded top-level. The Recorder simply observes what the
// engine does; if the program is non-deterministic, so is the
// resulting StackForm.
func Compile(reg *core.Registry, tokens []core.Value) (result []core.Value, form *StackForm, err error) {
	form = &StackForm{}
	rec := &recorder{form: form}
	e := core.NewTop(reg)
	e.SetRecorder(rec)
	result, err = e.Run(tokens)
	return result, form, err
}

// recorder is the engine-side Recorder implementation that appends
// Ops to a StackForm. Implements core.Recorder.
//
// skipPushes tracks handler-result re-pushes the engine emits as the
// main loop scans past spliced results. Without this, every result
// value would be double-counted: once as Call (already implicit) and
// once as PushLit (the engine re-encountering it at the pointer).
type recorder struct {
	form       *StackForm
	skipPushes int
}

func (r *recorder) OnPushLit(v core.Value) {
	if r.skipPushes > 0 {
		r.skipPushes--
		return
	}
	r.form.Append(PushLit{V: v})
}

func (r *recorder) OnCall(name string, arity, returns int) {
	r.form.Append(Call{Name: name, Arity: arity})
	r.skipPushes += returns
}

// Skip implements core.RecorderSkipper. The engine calls this when
// it's about to re-encounter N stack values that have already been
// emitted (paren-close rewind, etc.).
func (r *recorder) Skip(n int) {
	r.skipPushes += n
}
