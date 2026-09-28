package lang

import (
	"strings"
	"testing"
)

// TestAssumedSigFoldArityGuard pins the scalar fold's arity guard
// (compiler tryFoldScalarConst) end to end. The check pass's no-match
// recovery (checkModeAssumeSig) records its best-fit overload over the args
// the real match rejected: `n div 0 gt 0` in a fn body assumed gt's two-slot
// DepScalar constructor over a one-value window, and the fold called that
// handler, which indexes its second param unguarded. The panic left the
// deferred fn-body check (RunPendingFnBodyChecks, outside the engine's
// recover) and reached RunCompiled's caller: a Go panic on a valid program,
// present on main at cd188a2. The guard declines the fold for a window that
// is not the sig's arity, and the programs run on both lanes alike.
func TestAssumedSigFoldArityGuard(t *testing.T) {
	for _, src := range []string{
		// The predicate type's body divides by zero: both lanes raise its
		// arith_error at the typed def (a capitalised predicate is declared
		// with fnpred — NUR099).
		`def Boom fnpred [[n:Integer] [n div 0 gt 0]] end def x:Boom 5 end x`,
		// Defined and never called: it compiles (the recovery used to fail
		// the check pass with an internal_error).
		`def f fn [[n:Integer] [Boolean] [n div 0 gt 0]] end 1`,
	} {
		requireEngineParity(t, src, true)
	}
	// Called directly, the recovery's assumed dispatch keeps its loud
	// decline — the interpreter raises the division's arith_error.
	src := `def f fn [[n:Integer] [Boolean] [n div 0 gt 0]] end f 5`
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil || err != nil || !strings.Contains(reason, "unmatched dispatch recovered at gt") {
		t.Errorf("%q: want the recovered-dispatch decline, got prog=%v reason=%q err=%v", src, prog != nil, reason, err)
	}
	if _, errI := mustNew(t).RunInterp(src); errI == nil || !strings.Contains(errI.Error(), "arith_error") {
		t.Errorf("%q: the interpreter raises the division's arith_error, got %v", src, errI)
	}
}
