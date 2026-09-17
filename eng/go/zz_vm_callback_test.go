package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// VM-executing callback tests: they assert the COMPILED unit runs, so
// they live where the VM does.

// InvokeCallback consults depsFresh: a stale runtime-stamped ref must take
// the interpreter (CallBoru over the boru body), not the frozen unit.
func TestInvokeCallbackStaleDepFallsBack(t *testing.T) {
	r := stampReg(t)
	r.Defs.Push("dep", core.NewInteger(1))

	// A unit that would return 42 if the VM path ran.
	p := &compiler.Program{
		Consts: []core.Value{core.NewInteger(42)},
		Fns: []compiler.CompiledFn{{
			Name:  "const42",
			Code:  []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		}},
	}
	// The boru body the interpreter runs instead: a single literal 7.
	sig := &core.Signature{Impl: &core.BoruImpl{
		Body: []core.Value{core.NewInteger(7)},
		Compiled: &compiler.CompiledFnRef{Prog: p, Unit: 0, DepSnap: map[string]compiler.DepSnapEntry{
			"dep": {Depth: 99, Gen: -1}, // never matches → stale
		}},
	}}
	out, err := core.InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 7 {
		t.Fatalf("stale ref must run the interpreter body (7), got %v", out[0])
	}

	// The positive twin: a FRESH snapshot takes the VM unit (42).
	sig.Impl.(*core.BoruImpl).Compiled.(*compiler.CompiledFnRef).DepSnap = map[string]compiler.DepSnapEntry{
		"dep": {Depth: r.Defs.Depth("dep"), Gen: r.Defs.Gen("dep")},
	}
	out, err = core.InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("InvokeCallback (fresh): %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 42 {
		t.Fatalf("fresh ref must run the VM unit (42), got %v", out[0])
	}
}

// The §7c JIT re-stamp (REFUSAL-CLOSURE.0): a detached ref whose dep was
// REBOUND after the stamp re-compiles against the live binding at invoke
// time — the fresh twin runs on the VM with the NEW value (parity with the
// interpreter's live resolution) — instead of degrading permanently to
// CallBoru. A second invoke under the same binding REUSES the twin (no
// second compile); the try budget bounds a hot rebinding loop; and a
// declined re-stamp (stamping disarmed) takes the interpreter.
func TestInvokeCallbackJITRestamp(t *testing.T) {
	r := stampReg(t)
	r.EnableRuntimeStamping()
	r.Defs.Push("dep", core.NewInteger(1))

	fd := BoruBodyFd(core.NewWord("dep"))
	fd.Name = "reader"
	ref, ok := compiler.StampDetachedFn(r, fd, core.SrcPos{Row: 1, Col: 1})
	if !ok {
		t.Fatalf("initial stamp declined: %+v", r.StampEvents())
	}
	sig := &core.Signature{Impl: &core.BoruImpl{Body: []core.Value{core.NewWord("dep")}, Compiled: ref}}

	// Fresh: the frozen unit returns the stamp-time binding.
	out, err := core.InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("fresh invoke: %v", err)
	}
	if n, _ := core.AsInteger(out[0]); n != 1 {
		t.Fatalf("fresh unit must return the stamp-time value 1, got %v", out[0])
	}

	// Rebind dep → the old snapshot is stale → the seam re-stamps and the
	// twin returns the LIVE value, exactly as the interpreter resolves it.
	r.Defs.Pop("dep")
	r.Defs.Push("dep", core.NewInteger(2))
	stampsBefore := len(r.StampEvents())
	out, err = core.InvokeCallback(r, sig, nil, nil)
	if err != nil {
		t.Fatalf("restamped invoke: %v", err)
	}
	if n, _ := core.AsInteger(out[0]); n != 2 {
		t.Fatalf("re-stamped unit must return the live value 2, got %v", out[0])
	}
	if got := len(r.StampEvents()); got != stampsBefore+1 {
		t.Fatalf("the re-stamp must record one stamp event, got %d new", got-stampsBefore)
	}

	// Same binding, second invoke: the twin is fresh — REUSED, no compile.
	stampsBefore = len(r.StampEvents())
	if out, err = core.InvokeCallback(r, sig, nil, nil); err != nil {
		t.Fatalf("reuse invoke: %v", err)
	}
	if n, _ := core.AsInteger(out[0]); n != 2 {
		t.Fatalf("twin reuse must return 2, got %v", out[0])
	}
	if got := len(r.StampEvents()); got != stampsBefore {
		t.Fatalf("twin reuse must not re-compile, got %d new events", got-stampsBefore)
	}

	// The try budget: each further rebind pays one re-stamp until the budget
	// (RestampMaxTries, one already spent) exhausts; after that the seam
	// stays on CallBoru — which STILL resolves the live binding, so values
	// keep matching the interpreter — containment, not a fix.
	for i := 3; i <= 6; i++ {
		r.Defs.Pop("dep")
		r.Defs.Push("dep", core.NewInteger(int64(i)))
		if out, err = core.InvokeCallback(r, sig, nil, nil); err != nil {
			t.Fatalf("rebind %d invoke: %v", i, err)
		}
		if n, _ := core.AsInteger(out[0]); n != int64(i) {
			t.Fatalf("rebind %d: got %v, want the live value (VM twin or CallBoru alike)", i, out[0])
		}
	}
	if tries := ref.Restamp.Tries; tries != compiler.RestampMaxTries {
		t.Fatalf("try budget must exhaust at %d, got %d", compiler.RestampMaxTries, tries)
	}

	// A declined re-stamp (stamping disarmed mid-life) takes the interpreter:
	// reset the budget to isolate the decline arm from the exhausted arm.
	ref.Restamp.Tries = 0
	ref.Restamp.Cur = nil
	r.DisableRuntimeStamping()
	r.Defs.Pop("dep")
	r.Defs.Push("dep", core.NewInteger(9))
	if out, err = core.InvokeCallback(r, sig, nil, nil); err != nil {
		t.Fatalf("disarmed invoke: %v", err)
	}
	if n, _ := core.AsInteger(out[0]); n != 9 {
		t.Fatalf("disarmed re-stamp must fall to CallBoru's live resolution (9), got %v", out[0])
	}
}

func stampReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func BoruBodyFd(body ...core.Value) core.FnDefInfo {
	return core.FnDefInfo{Signatures: []core.Signature{{Impl: core.Boru(body)}}}
}
