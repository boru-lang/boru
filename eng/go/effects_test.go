package eng

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The C1 effect fence (effects.go, design/RUNTIME-INDEPENDENCE-COMPLETION-
// PLAN.0.md): the ledger counts observable effects so the compiled-mode
// fallback arms can prove "nothing escaped yet" before silently re-running a
// program on the interpreter — a re-run after an effect duplicates it (the
// L-DUP class). These tests pin the ledger primitives, the writer fence, and
// the InvokeCallback retry fence; the whole-program arms are pinned on the
// lang side (bytecode_effectfence_test.go).

// A nil ledger counts nothing and never panics — the pre-fence behaviour for
// registries assembled without NewRegistry (zero-value test fixtures).
func TestEffectLedgerNilSafe(t *testing.T) {
	var l *core.EffectLedger
	l.Note() // must not panic
	if got := l.Count(); got != 0 {
		t.Fatalf("nil ledger Count = %d, want 0", got)
	}
}

// The positive twin: a real ledger counts each Note.
func TestEffectLedgerCounts(t *testing.T) {
	l := &core.EffectLedger{}
	if got := l.Count(); got != 0 {
		t.Fatalf("fresh ledger Count = %d, want 0", got)
	}
	l.Note()
	l.Note()
	if got := l.Count(); got != 2 {
		t.Fatalf("Count after two notes = %d, want 2", got)
	}
}

// ArmEffectFence wraps both writers: a non-empty write marks the ledger and
// still reaches the underlying writer; an EMPTY write does not count (no
// observable effect); restore reinstates the original writers so later writes
// stop counting.
func TestArmEffectFenceCountsWrites(t *testing.T) {
	r := runUnitReg(t)
	var out, errOut bytes.Buffer
	r.Output, r.ErrOutput = &out, &errOut

	restore := r.ArmEffectFence()
	if _, err := r.Output.Write(nil); err != nil {
		t.Fatalf("empty write: %v", err)
	}
	if got := r.Effects.Count(); got != 0 {
		t.Fatalf("empty write counted: Count = %d, want 0", got)
	}
	if _, err := r.Output.Write([]byte("x")); err != nil {
		t.Fatalf("stdout write: %v", err)
	}
	if _, err := r.ErrOutput.Write([]byte("y")); err != nil {
		t.Fatalf("stderr write: %v", err)
	}
	if got := r.Effects.Count(); got != 2 {
		t.Fatalf("Count after one write per channel = %d, want 2", got)
	}
	if out.String() != "x" || errOut.String() != "y" {
		t.Fatalf("writes did not reach the underlying writers: out=%q err=%q", out.String(), errOut.String())
	}

	restore()
	if r.Output != &out || r.ErrOutput != &errOut {
		t.Fatal("restore did not reinstate the original writers")
	}
	if _, err := r.Output.Write([]byte("z")); err != nil {
		t.Fatalf("post-restore write: %v", err)
	}
	if got := r.Effects.Count(); got != 2 {
		t.Fatalf("post-restore write counted: Count = %d, want 2", got)
	}
}

// failWriter rejects every write outright (0, err) — a closed pipe. Nothing
// escaped, so the fence must NOT count it: counting would block a still-safe
// interpreter fallback. partialWriter accepts one byte before failing — that
// byte escaped, so it MUST count.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errClosedPipe }

type partialWriter struct{}

func (partialWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 1, errClosedPipe
}

var errClosedPipe = errors.New("closed pipe")

// A rejected write (0, err) provably emitted nothing and stays uncounted; a
// PARTIAL write (n>0 with err) escaped and counts.
func TestArmEffectFenceFailedWriteDoesNotCount(t *testing.T) {
	r := runUnitReg(t)
	r.Output = failWriter{}
	restore := r.ArmEffectFence()
	if _, err := r.Output.Write([]byte("x")); err == nil {
		t.Fatal("failWriter must reject the write")
	}
	if got := r.Effects.Count(); got != 0 {
		t.Fatalf("rejected write counted: Count = %d, want 0", got)
	}
	restore()

	r2 := runUnitReg(t)
	r2.Output = partialWriter{}
	restore2 := r2.ArmEffectFence()
	if _, err := r2.Output.Write([]byte("xy")); err == nil {
		t.Fatal("partialWriter must error")
	}
	if got := r2.Effects.Count(); got != 1 {
		t.Fatalf("partial write not counted: Count = %d, want 1", got)
	}
	restore2()
}

// The negative twin: nil writers are left nil (arming a writerless registry
// must not manufacture a writer), and restore is still safe.
func TestArmEffectFenceNilWriters(t *testing.T) {
	r := runUnitReg(t)
	r.Output, r.ErrOutput = nil, nil
	restore := r.ArmEffectFence()
	if r.Output != nil || r.ErrOutput != nil {
		t.Fatal("arming a nil writer must leave it nil")
	}
	restore()
	if r.Output != nil || r.ErrOutput != nil {
		t.Fatal("restore over nil writers must leave them nil")
	}
}

// noteEffectSig builds a 0-arg, 0-result native signature whose handler marks
// the registry's effect ledger — the unit-test stand-in for a printing word.
func noteEffectSig() *core.Signature {
	return &core.Signature{
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
			reg.NoteEffect()
			return nil, nil
		}),
	}
}

// TestInvokeCallbackBailAfterEffectPropagates pins the C1 fence on the
// callback seam: a stamped unit that emits an observable effect (the
// CALL_NATIVE notes the ledger, standing in for a write to the peer) and THEN
// raises internal_error (CALL_DYNAMIC underflow) must PROPAGATE the
// internal_error — the CallBoru retry would re-run the body and duplicate the
// effect. TestInvokeCallbackInternalErrorFallsBack is the positive twin: the
// same bail with NO prior effect still retries on the interpreter.
// TestInvokeCallbackBailAfterWriterEffectPropagates is the WRITER-path twin:
// the callback prints through the registry's Output (the actual print path,
// not a direct ledger note) and then bails. A detached callback fires after
// the enclosing compiled run disarmed its fence, so InvokeCallback must arm
// the writer fence around its own attempt — without it this write is
// invisible to the ledger and the CallBoru retry runs the body again,
// duplicating the peer-visible output.
func TestInvokeCallbackBailAfterWriterEffectPropagates(t *testing.T) {
	writeSig := &core.Signature{
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
			_, _ = reg.Output.Write([]byte("x"))
			return nil, nil
		}),
	}
	p := &compiler.Program{
		Sigs: []compiler.SigRef{{Word: "zz-write", Sig: writeSig}},
		Fns: []compiler.CompiledFn{{
			Name: "write-then-boom", NParams: 0, NLocals: 0,
			Code: []compiler.Instr{
				{Op: compiler.OpCallNative, Arg: 0},
				{Op: compiler.OpCallDynamic, Arg: 0},
				{Op: compiler.OpRet, Arg: 0},
			},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0}
	r := runUnitReg(t)
	var out bytes.Buffer
	r.Output = &out
	sig := &core.Signature{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(42)}, ref)}
	res, err := core.InvokeCallback(r, sig, nil, nil)
	if !core.IsInternalErr(err) {
		t.Fatalf("fenced writer bail: err = %v (res=%v), want the propagated internal_error", err, res)
	}
	if out.String() != "x" {
		t.Fatalf("output = %q, want exactly one %q (no retry re-ran the body)", out.String(), "x")
	}
	if r.Output != &out {
		t.Fatal("the attempt must restore the original writers")
	}
}

func TestInvokeCallbackBailAfterEffectPropagates(t *testing.T) {
	p := &compiler.Program{
		Sigs: []compiler.SigRef{{Word: "zz-note", Sig: noteEffectSig()}},
		Fns: []compiler.CompiledFn{{
			Name: "emit-then-boom", NParams: 0, NLocals: 0,
			Code: []compiler.Instr{
				{Op: compiler.OpCallNative, Arg: 0},
				{Op: compiler.OpCallDynamic, Arg: 0},
				{Op: compiler.OpRet, Arg: 0},
			},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	ref := &compiler.CompiledFnRef{Prog: p, Unit: 0}
	r := runUnitReg(t)
	// The sig carries a boru body the interpreter COULD run to 42 — the test
	// is that the fence refuses to, because the effect already escaped.
	sig := &core.Signature{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(42)}, ref)}
	out, err := core.InvokeCallback(r, sig, nil, nil)
	if !core.IsInternalErr(err) {
		t.Fatalf("fenced callback bail: err = %v (out=%v), want the propagated internal_error", err, out)
	}
	if got := r.Effects.Count(); got != 1 {
		t.Fatalf("effect count = %d, want exactly 1 (no retry ran the body again)", got)
	}
	if !strings.Contains(err.Error(), "internal") {
		t.Fatalf("propagated error should carry the internal_error taxonomy: %v", err)
	}
}
