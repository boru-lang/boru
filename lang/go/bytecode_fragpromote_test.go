package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestFragmentValueDefPromotion pins Leaf-3: a def-chain nested in an `if` arm
// must get value-def promotion just like a top-level chain, so its computed
// producers seat as frame locals instead of interleaving on the closed
// fragment's simulated stack — which declined "operands of <op> not adjacent on
// top" (the bloom-count `def md … def xd … (md sub xd div md)` shape, where the
// chain lives in an `if` else-arm). Asserts native compile (no FALLBACK island)
// AND RunCompiledStrict==Run: a parity test alone is insufficient because
// --compile used to fall back silently, so a parity test would pass on the
// interpreter and hide the coverage gap.
func TestFragmentValueDefPromotion(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"if else-arm def-chain (bloom-count shape)",
			`def f fn [[m:Integer x:Integer] [Float] [
			  if (m lt 1) [(0 convert Float)] [
			    def md (m convert Float)
			    def xd (x convert Float)
			    def ratio (md sub xd div md)
			    def lnr (ratio mul md)
			    lnr
			  ]
			]] (10 3 f)`,
		},
		{
			// a dead def (kd unused) nested in the arm — promoted then dropped;
			// must still compile and match (it previously declined "not adjacent").
			"if else-arm with an unused def",
			`def f fn [[m:Integer x:Integer] [Float] [
			  if (m lt 1) [(0 convert Float)] [
			    def md (m convert Float)
			    def xd (x convert Float)
			    def kd ((m add x) convert Float)
			    def ratio (md sub xd div md)
			    ratio
			  ]
			]] (10 3 f)`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile (fragment value-def promotion), declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("must compile native — no FALLBACK island:\n%s", prog.Disassemble())
			}
			b, _ := New()
			got, err := b.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict declined: %v", err)
			}
			d, _ := New()
			want, werr := d.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter error: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}
