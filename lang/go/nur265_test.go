package lang

import (
	"strings"
	"testing"
)

// TestNUR265CheckPassCompletes pins NUR265: a filter over a declared-Any
// result with an argument list the check pass auto-evaluates (`[gt 1]`, a
// recovered `gt` over one operand) panicked inside the pass's const fold —
// index out of range, recovered as an internal engine error. The pass
// completes now. Over a List the two lanes agree; over 5 the compiled lane
// still raises gt's error where the interpreter raises filter's no-match,
// which is NUR264's static trap, recorded there.
func TestNUR265CheckPassCompletes(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][Any][5]] end filter (mk) [gt 1]`,
		`def mk fn [[][Any][[1 2]]] end filter (mk) [gt 1]`,
	} {
		if _, err := mustNew(t).Run(src); err == nil || strings.Contains(err.Error(), "internal engine error") || strings.Contains(err.Error(), "internal_error") {
			t.Errorf("%s: the check pass completes and the run raises a boru error, got %v", src, err)
		}
	}
	requireEngineParity(t, `def mk fn [[][Any][[1 2]]] end filter (mk) [gt 1]`, true)
	requireEngineParity(t, `def mk fn [[][Any][[1 2]]] end filter [gt 1] (mk)`, true)
}
