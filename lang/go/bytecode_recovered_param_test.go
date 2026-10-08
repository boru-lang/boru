package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Off-corpus regressions for compiling a RECOVERED user-fn call: an Any-typed
// value (e.g. from an `Options`/`Map` `get` with no field schema) flows into a
// fn with a CONCRETELY-typed param, so matchSignature cannot statically commit
// and dispatch recovers. The body unit must then be compiled against the
// DECLARED param type (the contract the VM's CALL_USER guard enforces), not
// strict-Any — otherwise a multi-overload word inside the body (notably
// `convert`) sees strict-Any, matches no overload, and declines
// ("unmatched dispatch recovered at convert"). This is the voxgig bloom-filter
// `Bloom.make` → derive-m leaf (`n MathUtil.negate convert Float`). The langspec
// differential is blind to this shape, so it is pinned as RunCompiled==Run.

func recovSound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

// An Any value (Options get) into a Float/Integer-typed fn whose body does
// `convert Float` — compiles to a native lowering (no FALLBACK) and matches.
func TestRecoveredParam_ConvertOverDeclaredType(t *testing.T) {
	const src = `import "boru:math-util" end
def derive fn [[p:Float n:Integer] [Float] [ n MathUtil.negate convert Float mul p ]]
def mk fn [[opts:Options] [Float] [
  def n-val (opts "n" get)
  def p-val (opts "p" get)
  derive p-val n-val
]]
{n:5 p:2.0} mk`
	recovSound(t, src)
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected a native lowering of the recovered derive call, got an island")
	}
}

// NEGATIVE: a runtime arg that MISSES the declared param contract must raise the
// same error in both engines (the VM's CALL_USER guard == the interpreter's
// no_signature) — never silently run the body compiled against the declared type.
func TestRecoveredParam_ContractMissStaysSound(t *testing.T) {
	const src = `def f fn [[n:Integer] [Integer] [ n add 1 ]]
def g fn [[m:Map] [Integer] [ def x (m get "x") f x ]]
{x:"not-an-int"} g`
	recovSound(t, src)
}

// The well-typed case still runs identically (the Map carries an Integer).
func TestRecoveredParam_ContractHitSound(t *testing.T) {
	const src = `def f fn [[n:Integer] [Integer] [ n mul 10 ]]
def g fn [[m:Map] [Integer] [ def x (m get "x") f x ]]
{x:7} g`
	recovSound(t, src)
}
