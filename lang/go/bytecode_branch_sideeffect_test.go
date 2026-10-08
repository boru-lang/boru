package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestBranchArmSideEffectLeftover pins the fix for a branch arm that leaves an IGNORED
// side-effect call's result BELOW its single trailing result. A 0-return swap helper
// applied in an `each`-driven `if` arm — `[ arr i j swap-at end 0 ]` — leaves a value
// on the sim stack the fragment analysis already netted out (residualN==1, the trailing
// `0`), so the arm declined "branch leaves extra values". The lowerer now DROPS the
// ignored side-effect results (they already ran). This is the pancake/tim sort-chain
// leaf; reproduced here as an inline module (the swap helper is module-private, like the
// real sort lib). compile == interpret MUST hold — compiles natively, RunCompiledStrict
// == Run. want is the program residual (single wrapped).
func TestBranchArmSideEffectLeftover(t *testing.T) {
	const swapMod = `import module [
  def sw fn [[j:Integer i:Integer a:FlexList] [] [ def t (a get i) a set i (a get j) end a set j t end ]]
  def srt fn [[xs:List] [List] [ def arr (flex xs) def _ (iota 3 each [ var [[i] if ((arr get i) gt (arr get (i add 1))) [ arr (i add 1) i sw end 0 ] [0] ] ]) (node arr) ]]
  export "M" {srt: srt/v}
] end `
	strict := []struct{ name, src, want string }{
		// one bubble pass: the swap-arm's ignored helper result is dropped, the arm nets
		// its trailing 0, and the swaps mutate the array in place.
		{"each-if arm: 0-return swap helper then a const",
			swapMod + `([3 1 4 2] M.srt)`, "[[1 3 2 4]]"},
		{"no swaps needed (arm never taken)",
			swapMod + `([1 2 3 4] M.srt)`, "[[1 2 3 4]]"},
		// MULTI-VALUE arm (residualN>1): a COUNTED computed value below a trailing const.
		// The each leaves the whole arm residual and trims to the top per iteration, so
		// the arm compiles as a variadic multi-value (fragMulti) — the heap/intro leaf.
		{"multi-value arm: one computed value then a const",
			`(iota 3 each [ var [[i] if (i lt 5) [ (i add 100) end 0 ] [7] ] ])`, "[[0 0 0]]"},
		{"multi-value arm: two computed values then a const",
			`(iota 3 each [ var [[i] if (i lt 1) [ (i add 100) end (i add 200) end 0 ] [7] ] ])`, "[[0 7 7]]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
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
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
