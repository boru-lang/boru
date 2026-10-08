package eng

import (
	"bytes"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The effect ledger (effects.go). It was the C1 fence: the count let each
// compiled-mode fallback arm prove "nothing escaped yet" before silently
// re-running a program on the interpreter, because a re-run after an effect
// duplicates it (the L-DUP class). Nothing re-runs now, so the fence is gone
// and the ledger is an observability seam — these tests pin its primitives,
// and the callback-seam contract that replaced the retry fence: a bail inside
// a stamped unit propagates, and the body runs exactly once because there is
// no second run to make. The whole-program half is pinned on the lang side
// (bytecode_effectfence_test.go).

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

// The callback seam's bail contract, which is what the C1 fence used to buy
// and now holds by construction. A stamped unit that emits an observable
// effect and THEN raises internal_error (CALL_DYNAMIC underflow) PROPAGATES
// the internal_error: it is a compiler defect, and there is no CallBoru retry
// left to re-run the body behind it. The effect therefore fires exactly once,
// which the fence could only achieve by proving nothing had escaped first.
//
// Two paths, because they escape differently. Here the callback WRITES through
// the registry's Output — the real print path, not a ledger note — and the
// assertion is on the bytes a peer would have seen.
// TestInvokeCallbackBailAfterEffectPropagates is the ledger twin: the
// CALL_NATIVE notes the ledger, standing in for a write to a peer the test
// cannot observe directly.
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
		t.Fatal("the attempt must leave the registry's writer as it found it")
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
	// is that nothing runs it, because a bail is a defect and not a request
	// for a second attempt.
	sig := &core.Signature{Impl: core.NewBoruImplCompiled([]core.Value{core.NewInteger(42)}, ref)}
	out, err := core.InvokeCallback(r, sig, nil, nil)
	if !core.IsInternalErr(err) {
		t.Fatalf("fenced callback bail: err = %v (out=%v), want the propagated internal_error", err, out)
	}
	if got := r.Effects.Count(); got != 1 {
		t.Fatalf("effect count = %d, want exactly 1 (nothing ran the body again)", got)
	}
	if !strings.Contains(err.Error(), "internal") {
		t.Fatalf("propagated error should carry the internal_error taxonomy: %v", err)
	}
}
