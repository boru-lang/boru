package eng

import (
	"errors"
	"sync/atomic"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Finding C — a VM-internal soundness violation surfaces as a structured
// internal_error BoruError (taxonomy for tooling + the RunCompiled
// fall-back-to-interpreter signal), not a raw Go error string.
func TestVMInternalErrorIsBoruError(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	// SWAP on an empty stack is an impossible-by-construction state the
	// lowerer never emits; reaching it is the internal-error class.
	p := &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpSwap}}, Debug: []core.SrcPos{{Row: 1, Col: 1}}}
	_, runErr := RunProgram(p, r)
	if runErr == nil {
		t.Fatal("SWAP underflow returned nil error")
	}
	var ae *core.BoruError
	if !errors.As(runErr, &ae) || ae.Code != "internal_error" {
		t.Fatalf("internal error = %v (%T), want code internal_error", runErr, runErr)
	}
}

// Finding C — a panicking compiled-reachable handler is caught by the VM's
// last-resort recover and converted to a clean internal_error, mirroring the
// interpreter's top-level guard, instead of crashing with a stack trace.
func TestVMRecoversHandlerPanic(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "boom",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				panic("kaboom")
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	// Reach the panicking handler via OpCallNativePoly, which looks the word up
	// and dispatches its matched signature — no hand-built SigRef needed.
	p := &compiler.Program{
		Code:     []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNativePoly, Arg: 0}},
		Debug:    []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
		Consts:   []core.Value{core.NewInteger(1)},
		PolyRefs: []compiler.PolyRef{{Word: "boom", Arity: 1}},
	}
	_, runErr := RunProgram(p, r)
	if runErr == nil {
		t.Fatal("panicking handler returned nil error (not recovered)")
	}
	var ae *core.BoruError
	if !errors.As(runErr, &ae) || ae.Code != "internal_error" {
		t.Fatalf("recovered panic = %v (%T), want code internal_error", runErr, runErr)
	}
}

// Finding F — two OVERLAPPING runs on one registry are rejected with a clear
// concurrency_error instead of racing on the shared Invoker / scopes; nested
// SEQUENTIAL reuse of the same registry stays fine.
func TestVMConcurrencyGuard(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerAdd(r)
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	p := &compiler.Program{
		Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}},
		Debug:  []core.SrcPos{{Row: 1, Col: 1}},
		Consts: []core.Value{core.NewInteger(42)},
	}

	// Sequential reuse: two back-to-back runs on the same registry both work
	// (the flag resets on exit).
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("second sequential run: %v", err)
	}

	// Overlap: simulate an in-flight run by latching the flag, then a second
	// run must decline rather than race.
	atomic.StoreInt32(&r.VmRunning, 1)
	_, runErr := RunProgram(p, r)
	atomic.StoreInt32(&r.VmRunning, 0)
	if runErr == nil {
		t.Fatal("overlapping run on one registry returned nil error")
	}
	var ae *core.BoruError
	if !errors.As(runErr, &ae) || ae.Code != "concurrency_error" {
		t.Fatalf("overlap = %v (%T), want code concurrency_error", runErr, runErr)
	}
	// The guard must have left the flag as it found it (true here, we reset
	// it above); a fresh run now succeeds.
	if _, err := RunProgram(p, r); err != nil {
		t.Fatalf("run after rejected overlap: %v", err)
	}
}
