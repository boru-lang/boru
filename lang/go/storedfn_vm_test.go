package lang

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
)

// syncBuf is a goroutine-safe writer for capturing a spawned process's async
// output (the process body runs on its own goroutine).
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// A spawn body is compiled to a 0-param unit when it is reducible, and left as a
// raw list (interpreter fallback) when it is empty or declines.
func TestSpawnBodyCompilation(t *testing.T) {
	cases := []struct {
		src      string
		wantUnit bool
	}{
		{`spawn [print 42]`, true}, // reducible → compiled to a unit + carrier
		{`spawn []`, false},        // empty body → no unit
		{`spawn [dup]`, false},     // stack-shuffle declines → no unit, falls back
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, err := a.CompileCheck(c.src)
		if err != nil || prog == nil {
			t.Fatalf("%s: compile reason=%q err=%v", c.src, reason, err)
		}
		if got := len(prog.Fns) > 0; got != c.wantUnit {
			t.Errorf("%s: body compiled=%v, want %v", c.src, got, c.wantUnit)
		}
	}
}

// A compiled spawn runs its body on the VM (RunUnit on the process fork): the
// spawned `print 42` output appears.
func TestSpawnCompiledRunsBodyOnVM(t *testing.T) {
	a, _ := New()
	buf := &syncBuf{}
	a.NativeRegistry().Output = buf
	if _, err := a.RunCompiledStrict(`spawn [print 42]`); err != nil {
		t.Fatalf("run: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "42") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("spawned body output = %q, want it to contain 42", buf.String())
}

// A CompileStoresFn word stashes a handler to invoke LATER (like Net.serve-raw's
// connection handler). The compiler compiles a capture-free handler body to its
// own unit and stamps a durable CompiledFnRef on the baked const, so the word
// can run the handler on the VM via RunUnit / InvokeCallback instead of the
// interpreter. These tests drive that edge deterministically with a synthetic
// store-fn word (no network), and pin the fallback for a non-compiling body.

// registerStash adds a CompileStoresFn word `stash {opts} handler` that returns
// its opts map and treats the handler as inert data (the store-fn contract).
func registerStash(t *testing.T) *Boru {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.Register("stash", Signature{
		Args: []*Type{TMap, TAny},
		Impl: core.Go(func(args []Value, _ map[string]Value, _ []Value, _ *core.Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}),
		Returns:       []*Type{TMap},
		BarrierPos:    -1,
		CompileEffect: core.CompileStoresFn,
	})
	return a
}

// firstStampedHandler returns the first FnDefInfo const in prog whose own sig
// carries a compiled-unit ref, or nil.
func firstStampedHandler(prog *compiler.Program) *core.Signature {
	for _, c := range prog.Consts {
		fd, ok := c.Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if compiler.CompiledRef(&fd.Signatures[i]) != nil {
				return &fd.Signatures[i]
			}
		}
	}
	return nil
}

// A capture-free store-fn handler with a compilable body is compiled to a unit,
// stamped with a CompiledFnRef, and Finalize back-stamps the *Program; RunUnit
// then executes it standalone.
func TestStoredFnCompilesAndRunsOnVM(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile failed: reason=%q err=%v", reason, err)
	}
	sig := firstStampedHandler(prog)
	if sig == nil {
		t.Fatal("handler was not compiled + stamped with a CompiledFnRef")
	}
	ref := compiler.CompiledRef(sig)
	if ref.Prog != prog {
		t.Fatalf("Finalize did not back-stamp ref.Prog (got %p, want %p)", ref.Prog, prog)
	}
	// Run the compiled unit standalone: x=5 → 6.
	out, err := eng.RunUnit(ref, a.NativeRegistry(), []Value{NewInteger(5)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("unit result = %v, want 6", out[0])
	}
}

// InvokeCallback runs a stamped handler on the VM when the registry is idle, and
// its result matches CallBoru (the interpreter) on the same body — the fail-safe
// equivalence. A busy registry (or a nil ref) falls back to the interpreter.
func TestInvokeCallbackVMAndFallbackAgree(t *testing.T) {
	a := registerStash(t)
	prog, _, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: %v", err)
	}
	sig := firstStampedHandler(prog)
	if sig == nil {
		t.Fatal("no stamped handler")
	}
	reg := a.NativeRegistry()

	// Idle registry → VM path (RunUnit).
	vmOut, err := core.InvokeCallback(reg, sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("InvokeCallback (VM): %v", err)
	}
	if n, _ := vmOut[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("VM path result = %v, want 6", vmOut[0])
	}

	// Same sig, interpreter fallback (CallBoru directly): byte-identical.
	interpOut, err := reg.CallBoru(sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("CallBoru: %v", err)
	}
	if n, _ := interpOut[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("interpreter result = %v, want 6", interpOut[0])
	}
}

// firstHandlerSig returns the first FnDefInfo const's first own sig, stamped or
// not — used to reach an UN-stamped handler for the fallback path.
func firstHandlerSig(prog *compiler.Program) *core.Signature {
	for _, c := range prog.Consts {
		fd, ok := c.Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if !fd.Signatures[i].Fallback {
				return &fd.Signatures[i]
			}
		}
	}
	return nil
}

// A handler whose body does NOT compile (uses the context-dependent `args` word)
// is left un-stamped: no ref, so InvokeCallback falls back to the interpreter,
// which runs the body and returns the same result.
func TestStoredFnNonCompilingBodyFallsBack(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [args drop x]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	if sig := firstStampedHandler(prog); sig != nil {
		t.Fatal("a non-compiling handler body must NOT be stamped")
	}
	// The un-stamped sig routes through InvokeCallback's interpreter fallback.
	sig := firstHandlerSig(prog)
	if sig == nil {
		t.Fatal("no handler sig found")
	}
	out, err := core.InvokeCallback(a.NativeRegistry(), sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("InvokeCallback fallback: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Fatalf("fallback result = %v, want 5", out[0])
	}
}

// A service handler invoked synchronously DURING a compiled run exercises the
// nested-VM path (runUnitNested): the handler is stamped (service `add` is a
// CompileStoresFn slot), and `call` runs it on the VM nested in the enclosing
// run (canHostVM is false mid-run, so RunUnit can't be used). Compiled and
// interpreted results must match.
func TestServiceCallNestedVMDifferential(t *testing.T) {
	src := `def svc (service {n: 0})
add {cmd: "inc"} ([req:Any state:Any] => [42]) svc
call {cmd: "inc"} svc`
	// The handler is stamped at compile time.
	a, _ := New()
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil || prog == nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	if firstStampedHandler(prog) == nil {
		t.Fatal("service handler was not compiled + stamped")
	}
	// Byte-identical compiled (nested-VM) vs interpreted.
	ai, _ := New()
	interp, ierr := ai.RunInterp(src)
	ac, _ := New()
	comp, cerr := ac.RunCompiledStrict(src)
	if ierr != nil || cerr != nil {
		t.Fatalf("run errors: interp=%v compiled=%v", ierr, cerr)
	}
	if fmt.Sprint(interp) != fmt.Sprint(comp) {
		t.Fatalf("compiled %v != interpreted %v", comp, interp)
	}
	if fmt.Sprint(comp) != "[42]" {
		t.Fatalf("result = %v, want [42]", comp)
	}
}

// A multi-overload handler stamps EVERY own sig to its own unit
// (COMPILE FAILURE-CLOSURE §7b): the invoke seam dispatches through MatchFnSig, so
// the matched sig's own Impl ref is the sig table — each overload runs its
// unit on the VM, and a declining sibling would interpret independently.
func TestStoredFnMultiOverloadStampsPerSig(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(
		`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1] [s:String] [String] [s]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	stamped := 0
	for _, c := range prog.Consts {
		fd, ok := c.Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if compiler.CompiledRef(&fd.Signatures[i]) != nil {
				stamped++
			}
		}
	}
	if stamped != 2 {
		t.Fatalf("both overloads must carry their own refs (§7b), got %d stamped", stamped)
	}

	// A handler where ONE sig declines (a flow sentinel) still stamps the
	// other — per-sig, fail-safe.
	prog2, reason2, _, err2 := a.CompileCheck(
		`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1] [s:String] [String] [break s]])`)
	if err2 != nil || prog2 == nil {
		t.Fatalf("compile: reason=%q err=%v", reason2, err2)
	}
	stamped2 := 0
	for _, c := range prog2.Consts {
		fd, ok := c.Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if compiler.CompiledRef(&fd.Signatures[i]) != nil {
				stamped2++
			}
		}
	}
	if stamped2 != 1 {
		t.Fatalf("exactly the sentinel-free overload must stamp, got %d", stamped2)
	}

	// The SAME def-bound handler stored twice compiles its units once —
	// the second store site sees the stamped impl and skips (first stamp
	// wins), so the shared value never carries duplicate refs.
	prog3, reason3, _, err3 := a.CompileCheck(
		`def f (fn [[x:Integer] [Integer] [x add 1]]) (stash {a: 1} f/v) (stash {b: 2} f/v)`)
	if err3 != nil || prog3 == nil {
		t.Fatalf("compile: reason=%q err=%v", reason3, err3)
	}
	refs := map[*compiler.CompiledFnRef]bool{}
	for _, c := range prog3.Consts {
		fd, ok := c.Data.(core.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if ref := compiler.CompiledRef(&fd.Signatures[i]); ref != nil {
				refs[ref] = true
			}
		}
	}
	if len(refs) != 1 {
		t.Fatalf("a twice-stored handler must carry exactly ONE ref, got %d", len(refs))
	}
}
