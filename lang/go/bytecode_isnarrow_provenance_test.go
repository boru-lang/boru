package lang

import (
	"fmt"
	"testing"
)

// TestIsNarrowOperandProvenance pins Leaf-2 (STATS): an `is`-narrowed binding
// must keep its source value ID so resolveOperand finds its provenance (param
// slot / producing event). is-narrowing is a static-only refinement — at
// runtime the binding is UNCHANGED — so the narrowed operand resolves to the
// original slot, which already holds the right value (no value-passing half).
//
// Before the fix, ApplyGuardNarrowing / ApplyComplementNarrowing minted a FRESH
// carrier ID, so a narrowed value feeding a user call or a builtin declined
// "fn call operand of unknown provenance" / "...at <w>" (the stats as-summary
// `if (x is List) [build-summary x] [x]` shape). BOTH the then (guard) and else
// (complement) branches must carry the true runtime value — pinned with
// RunCompiledStrict==Run, since the langspec differential is blind to this
// union-param→is-narrow shape.
func TestIsNarrowOperandProvenance(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// then-branch narrowed value feeds a builtin (size); else returns the int.
		{"builtin on narrowed list (then)",
			`def f fn [[x:(List tor Integer)] [Integer] [if (x is List) [x size] [x]]] (f [1 2 3])`, "[3]"},
		{"narrowed int passthrough (else)",
			`def f fn [[x:(List tor Integer)] [Integer] [if (x is List) [x size] [x]]] (f 9)`, "[9]"},
		// then-branch narrowed value feeds a USER call (the build-summary shape).
		{"user call on narrowed list (then)",
			`def g fn [[xs:List] [Integer] [xs size]] ` +
				`def f fn [[x:(List tor Integer)] [Integer] [if (x is List) [g x] [x]]] (f [1 2 3 4])`, "[4]"},
		{"user call else int (else)",
			`def g fn [[xs:List] [Integer] [xs size]] ` +
				`def f fn [[x:(List tor Integer)] [Integer] [if (x is List) [g x] [x]]] (f 7)`, "[7]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict declined (want native compile): %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
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
