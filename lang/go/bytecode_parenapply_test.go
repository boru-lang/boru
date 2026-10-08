package lang

import (
	"fmt"
	"testing"
)

// COMPILE FAILURE-CLOSURE §9.2e (landed 2026-07-17) — a leading-dynamic fn-value
// application bounded by a paren (`((m get "f") x)`, `(m.g 3) add 1`)
// compiles to a guarded OpCallDynMethod (the §3 arrival chassis at the paren
// boundary): the runtime member value applies to the args exactly as the
// interpreter's paren auto-dispatch, and the window COLLAPSES to one carrier
// BEFORE any trailing op — so a downstream consumer seats the apply's RESULT,
// not the old paren-unaware OpCallDynamic reorder (which lowered `(m.g 3) add
// 1` as `m.g(3 add 1)=8`). The provenance gate keeps a NON-member leading
// dynamic declining.
func TestParenLeadingApplyCompiles(t *testing.T) {
	// The stamp-suite shape, inside an fn body.
	interpOnlyWithCompileFailure(t,
		`def m {f: ([y:Integer] => [y add 1])} def h fn [[x:Integer] [Integer] [ add 1 (x (m get "f") apply) ]] h 7`, "[9]")
	// Method-field reads, the former miscompile guard's fixtures.
	mustCompileWithParity(t,
		`def m {g: (fn [[x:Integer][Integer][x mul 2]])} ((m.g 3) add 1)`, "[7]")
	mustCompileWithParity(t,
		`def m {g: (fn [[x:Integer][Integer][x mul 2]])} (m.g 3) add 1`, "[7]")

	// Decline fence: a NON-member leading dynamic (a branch-merged fn value,
	// no member-read provenance) keeps the compile failure — parity via
	// fallback. The service-capturing-handler in a body is declined too (the
	// factory-body miscompile), pinned in the frontier ledger.
	{
		src := `def q (if true [([y:Integer] => [y add 1])] [([y:Integer] => [y sub 1])]) add 1 (q 5)`
		a, _ := New()
		gotC, _, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			return
		}
		b, _ := New()
		gotI, errI := b.RunInterp(src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || errC != nil && errI == nil {
			t.Errorf("non-member fence: compiled=%v (%v) interp=%v (%v)", gotC, errC, gotI, errI)
		}
	}
}
