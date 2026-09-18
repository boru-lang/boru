package lang

import (
	"fmt"
	"testing"
)

// bt wraps s in backticks (a boru interpolated-string literal) — Go raw strings
// cannot contain a backtick, so build the source by concatenation.
func bt(s string) string { return "`" + s + "`" }

// TestCheckPropInterpStringModuleScope pins Leaf-6: a property-harness body
// (test-check-prop is Callable==nil — baked as data and re-interpreted by its Go
// handler in a sub-engine) containing an interpolated string compiles AT MODULE
// SCOPE, where every ${name} resolves against the registry exactly as the
// interpreter does. Before the fix isInertConst had no InterpString case, so the
// whole body was "not inert" and the call refused "code-body word
// test-check-prop (Stage 2)".
func TestCheckPropInterpStringModuleScope(t *testing.T) {
	src := `import "boru:test" end
def res (Test.check-prop "x" [r.int 1 9] [ var [[k] (` + bt("v${k}") + `) eq ` + bt("v${k}") + ` ] ] 5 1 0)
res get "ok"`
	a, _ := New()
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("module-scope check-prop with interp string must compile, refused: %v", err)
	}
	b, _ := New()
	want, werr := b.RunInterp(src)
	if werr != nil {
		t.Fatalf("interpreter error: %v", werr)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("compiled %v != interpreter %v", got, want)
	}
	if fmt.Sprint(got) != "[true]" {
		t.Errorf("got %v, want [true]", got)
	}
}

// TestCheckPropInterpStringFnScopeRefuses is the MISCOMPILE GUARD: a check-prop
// NESTED in a compiled fn whose body interpolates a FRAME LOCAL (${pfx}) must
// NOT bake — re-interpreting the baked body in a sub-engine resolves ${pfx}
// against the registry (absent) instead of the VM frame slot, diverging
// (compiled false vs interpreter true). So the fn-scoped case must REFUSE under
// force-compile and fall back to the interpreter, yielding the interpreter's
// answer. This is the exact off-corpus shape the langspec differential is blind
// to (the leaf's central soundness boundary).
func TestCheckPropInterpStringFnScopeRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	src := `import "boru:test" end
def run-props fn [[pfx:Integer] [Boolean] [
  def res (Test.check-prop "x" [r.int 1 9] [ var [[k] (` + bt("${pfx}-${k}") + `) eq ` + bt("${pfx}-${k}") + ` ] ] 5 1 0)
  res get "ok"
]]
(run-props 100)`
	// Strict compile MUST refuse — the fn-frame-local interp is unbakeable.
	a, _ := New()
	if _, err := a.RunCompiledStrict(src); err == nil {
		t.Fatal("fn-nested check-prop interpolating a frame local must REFUSE strict compile (else it silently miscompiles)")
	}
	// Non-strict: falls back to the interpreter and yields the CORRECT answer.
	b, _ := New()
	got, compiled, err := b.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled error: %v", err)
	}
	if compiled {
		t.Error("fn-nested check-prop must fall back, not compile (frame-local bake hazard)")
	}
	c, _ := New()
	want, _ := c.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("fallback result %v != interpreter %v", got, want)
	}
	if fmt.Sprint(got) != "[true]" {
		t.Errorf("interpreter answer %v, want [true] (the property holds)", got)
	}
}
