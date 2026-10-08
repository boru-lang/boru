package lang

import (
	"fmt"
	"testing"
)

// Regression for the P1 soundness bug Codex flagged on PR #208: an
// INFERRED-return user fn (declared `[]`) whose body leaves a residual of
// UNKNOWN PROVENANCE must stay uncompilable (decline → compile failure), NOT
// silently RET zero values. A short-lived residual-drop (b70bb884's #4) made
// such a unit succeed with an empty stack where the interpreter raised — e.g. a
// deferred map expression that reads a param after its scope is gone. The
// boru:test cascade that motivated #4 no longer needs it (the genArgs-keyed
// recursion fix superseded it), so #4 was removed; these pin that compile ==
// interpret for the shapes it would have miscompiled. The langspec differential
// does not cover these exact shapes.

func inferSound(t *testing.T, src string) {
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

// Codex's exact case: a deferred map expression reads the param `a` after the
// param scope is gone — the interpreter raises undefined_word, so the compiled
// path must NOT succeed with an empty stack.
func TestInferredReturn_DeferredMapResidualStaysSound(t *testing.T) {
	inferSound(t, `def f fn [[a:Integer] [] [ {x:(a 1 add)} ]] (f 5)`)
}

// A well-typed inferred-return fn that DOES leave a resolvable value still runs
// identically (the residual is a plain concrete map).
func TestInferredReturn_ResolvableResidualSound(t *testing.T) {
	inferSound(t, `def f fn [[a:Integer] [] [ {x:a} ]] (f 5)`)
}

// An inferred-return fn whose body genuinely nets zero (a side-effect print)
// returns nothing in both engines.
func TestInferredReturn_ZeroNetBodySound(t *testing.T) {
	inferSound(t, `def f fn [[a:Integer] [] [ print a ]] f 5`)
}
