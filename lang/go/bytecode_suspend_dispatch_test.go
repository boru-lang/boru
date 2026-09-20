package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNestedDynamicDispatchUnderSuspend pins Stage-D 2/4: a no-signature dynamic
// dispatch reached UNDER a suspended carrierResults re-dispatch (an enclosing
// recovery that suspended recording to read a result type) must NOT prematurely
// MarkUncompilable the whole program — its real compile decision happens on the
// non-suspended recording pass, or it is subsumed by the enclosing poly's
// runtime re-match (callPoly / OpFallback, both interpreter-faithful). The
// nested get-of-get shapes below compile to an OpCallNativePoly that re-matches
// at run time, so the downstream consumer sees the runtime value. Soundness of
// the suspend-skip was verified by an adversarial sweep (~413 programs across
// nested-get / mutating-nested / user-fn-nested / result-consumed families, plus
// a structural argument: under suspend ZERO ops are recorded, so a suspend-only
// dispatch can never bake a wrong op).
//
// These pin compiled==interpreter AND native compile (no FALLBACK island) for
// the get-over-dynamic family whose results flow into compiled downstream code.
func TestNestedDynamicDispatchUnderSuspend(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"get-of-get result feeds arithmetic",
			`def f fn [[nd:Any] [Integer] [ ((nd "a" get) "b" get) add 100 ]] ({a:{b:5}} f)`,
			"[105]",
		},
		{
			"get-of-get result feeds a comparison",
			`def f fn [[nd:Any] [Boolean] [ ((nd "a" get) "b" get) gt 3 ]] ({a:{b:5}} f)`,
			"[true]",
		},
		{
			"triple-nested get-of-get-of-get",
			`def f fn [[nd:Any] [Any] [ (((nd "a" get) "b" get) "c" get) ]] ({a:{b:{c:42}}} f)`,
			"[42]",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile (poly re-match), declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("must compile native (no island):\n%s", prog.Disassemble())
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
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
