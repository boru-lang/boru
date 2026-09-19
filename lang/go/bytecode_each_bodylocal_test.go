package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEachBodyLocalValueDef pins voxgig leaf L0: an each/closure body that binds
// its OWN local via a value-def (`def j …`) used to refuse force-compile. The
// cause: an analysis sub-engine run of the body left `j` bound in the registry
// ABOVE the enclosing-fn baseline, so ComputeCaptures wrongly grabbed `j` as a
// capture; its resolveOperand then failed (j is not an enclosing binding), the
// closure declined, and the each fell back to the interpreter. The fix excludes
// names a body `def`s for ITSELF (collectBodyLocalDefs) from capture — they are
// the closure's frame-locals, promoted by the body compile, not captures.
//
// Discriminator: `each [def j 5 j]` (const) always compiled; `each [def j (5 add
// 1) j]` (computed) refused — the computed value's extra def-depth pushed `j`
// above the baseline. Both must compile now, and a body that captures a GENUINE
// enclosing binding (`cur`) alongside its own local (`j`) must capture only cur.
func TestEachBodyLocalValueDef(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// Cases that MUST compile natively (RunCompiledStrict) — no island fallback.
	strict := []struct{ name, src, want string }{
		{"computed body-local in each", `def g fn [[][Integer][def _ ([1] each [def j (5 add 1) j]) 5]] (g)`, "[5]"},
		{"const body-local in each (regression)", `def g fn [[][Integer][def _ ([1] each [def j 5 j]) 5]] (g)`, "[5]"},
		{"body-local + genuine capture (cur)", `def f fn [[n:Integer][Integer][ def cur (flex [9]) def _ (iota n each [def j (cur get 0) j]) (cur get 0)]] (f 3)`, "[9]"},
		{"var-block loop yielding a result (sort idiom)", `def f fn [[][Integer][ def cur (flex [5]) def _ ([1] each [var [[t] def j (cur get 0) (cur set 0 (j add 1)) j]]) (cur get 0)]] (f)`, "[6]"},
		{"multiple body-local defs", `def g fn [[][List][ [1 2] each [def a (5 add 1) def b (a mul 2) (a add b)] ]] (g)`, "[[18 18]]"},
		{"genuine enclosing capture (not a body-local)", `def f fn [[][List][ def acc 100 ([1 2] each [acc add 1]) ]] (f)`, "[[101 101]]"},
		{"body-local AND capture together", `def f fn [[][List][ def acc 10 ([1 2] each [def j (acc add 1) (j mul 2)]) ]] (f)`, "[[22 22]]"},
	}
	for _, c := range strict {
		t.Run("strict/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile, refused: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// SOUNDNESS: compile==interpret must hold (fallback allowed) for shapes that
	// stress capture correctness — a body-local must NOT shadow/corrupt an
	// enclosing binding of the same name, and a genuine capture must keep its
	// value. These pin that excluding body-locals from capture did not drop a real
	// capture nor confuse same-named scopes.
	sound := []struct{ name, src string }{
		// `j` is BOTH an enclosing def (999) and a body-local in the each. The each
		// body's `def j` shadows locally → each yields [6]; the trailing `j` is the
		// enclosing 999. Excluding the body-local from capture must keep both right.
		{"same-name enclosing + body-local", `def j 999 def _ ([1] each [def j (5 add 1) j]) j`},
	}
	for _, c := range sound {
		t.Run("sound/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, got, err) {
				return
			}
			if err != nil {
				t.Fatalf("RunCompiled error: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter error: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}
