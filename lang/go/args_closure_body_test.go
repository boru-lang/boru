package lang

// `args` inside a CLOSURE body: the closure analysis's args frame holds
// the CallableSpec inputs, while at run time the body executes through
// InvokeBody in the ENCLOSING call context — the interpreter's `args`
// reads the enclosing fn's per-call list. Projecting the closure frame
// const-baked the wrong (often empty) list: `do [args]` inside a fn
// compiled to PUSH_CONST [] against the interpreter's [7]. The
// projection now declines inside closure units (specialWordResults →
// EmitState.inClosureUnit) and args/__pa are screened out of island
// bodies (bodyFreeForFallback); for `do` specifically the dyn-body
// backstop (CompileDynBody) then COMPILES the dispatch — the program's
// DynEnv args bracket makes the runtime sub-run's `args` read identical
// to the interpreter's. Non-CompileDynBody words (each) keep the
// compile failure with fallback parity. Fn-UNIT args projection (args.N folding
// to PUSH_LOCAL N) is untouched.

import (
	"fmt"
	"strings"
	"testing"
)

// argsBothEngines runs src via RunCompiled and interpreted (reusing the
// shared runBothEngines helper), failing the test on either engine
// erroring, and returns the rendered results plus whether the compiled
// path actually compiled.
func argsBothEngines(t *testing.T, src string) (interp, compiled string, wasCompiled bool) {
	t.Helper()
	gotC, was, errC, gotI, errI := runBothEngines(t, src)
	if errC != nil {
		t.Fatalf("RunCompiled(%q): %v", src, errC)
	}
	if errI != nil {
		t.Fatalf("Run(%q): %v", src, errI)
	}
	return fmt.Sprint(gotI), fmt.Sprint(gotC), was
}

// TestArgsInDoBodyCompilesWithParity — a `do` body reading the enclosing
// fn's args COMPILES via the dyn-body backstop (CompileDynBody): the
// program's DynEnv mode brackets every CALL_USER frame with an args-stack
// push, so the body's runtime sub-run reads `args` exactly as the
// interpreter's per-call push provides — [7], byte-identical. (The
// original miscompile const-baked the closure analysis frame's [].)
func TestArgsInDoBodyCompilesWithParity(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	src := `def g fn [[n:Integer] [Any] [do [args]]]  g 7`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: the dyn-body backstop must compile this; reason=%q err=%v", src, reason, cerr)
	}
	interp, compiled, was := argsBothEngines(t, src)
	if !was {
		t.Errorf("%q: expected the compiled path to run", src)
	}
	if interp != compiled || interp != "[[7]]" {
		t.Errorf("%q: want [[7]] on both engines; interpreted=%q compiled=%q", src, interp, compiled)
	}
	// Since S1a (2026-09-19, design/FULL-COMPILATION-REPLAN.0.md) each
	// declares CompileDynBody too, so an each body reading the enclosing
	// fn's args takes the same backstop: the body's runtime sub-run reads
	// `args` under the DynEnv bracket and answers 7 per element —
	// byte-identical to the interpreter. (Before S1a the shape DECLINED —
	// its closure input never satisfied the args projection — and fell
	// back with parity.)
	srcEach := "def g fn [[n:Integer] [List] [[10 20] each [drop args.0]]]  g 7"
	b, _ := New()
	eProg, eReason, _, ecerr := b.CompileCheck(srcEach)
	if ecerr != nil || eProg == nil {
		t.Errorf("%q: the dyn-body backstop must compile this; reason=%q err=%v", srcEach, eReason, ecerr)
	}
	interp, compiled, was = argsBothEngines(t, srcEach)
	if !was || interp != compiled || interp != "[[7 7]]" {
		t.Errorf("%q: want [[7 7]] compiled with parity: was=%v interpreted=%q compiled=%q", srcEach, was, interp, compiled)
	}
}

// TestArgsAtTopLevelDoParity — top-level `do [args]`: args errors
// ("not inside a function") and `do` traps it into an Error value in
// BOTH engines; the dyn-body backstop compiles it and the runtime
// sub-run raises the identical trapped error.
func TestArgsAtTopLevelDoParity(t *testing.T) {
	src := `do [args]`
	interp, compiled, was := argsBothEngines(t, src)
	if !was {
		t.Errorf("%q: expected the dyn-body backstop to compile this", src)
	}
	if interp != compiled {
		t.Errorf("%q: engine divergence: interpreted=%q compiled=%q", src, interp, compiled)
	}
	if !strings.Contains(interp, "args") {
		t.Errorf("%q: expected the trapped args_error value, got %q", src, interp)
	}
}

// TestArgsFnUnitFoldStillCompiles — the positive pair: args.N inside a
// plain FN body keeps folding to the frame local (PUSH_LOCAL N) and the
// program compiles. Guards the fn-unit projection against over-broad
// closure gating.
func TestArgsFnUnitFoldStillCompiles(t *testing.T) {
	src := `def f fn [[a:Integer b:Integer] [Integer] [args.1]]  f 1 2`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected the fn-unit args.N fold to compile; reason=%q err=%v", src, reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "PUSH_LOCAL") {
		t.Errorf("%q: expected args.1 to fold to a PUSH_LOCAL frame read; got:\n%s", src, prog.Disassemble())
	}
	interp, compiled, was := argsBothEngines(t, src)
	if !was {
		t.Errorf("%q: expected the compiled path to run", src)
	}
	if interp != compiled || interp != "[2]" {
		t.Errorf("%q: want [2] on both engines; interpreted=%q compiled=%q", src, interp, compiled)
	}
}
