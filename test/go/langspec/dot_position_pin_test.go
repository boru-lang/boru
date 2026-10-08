package langspec

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// TestDotAccessKeepsSourcePosition pins NUR113's compiled half.
//
// A dot-sugar access lowers through synthesized `dot`/`dotr` words, and a
// module-qualified one resolves to a Function that moduleNSGetReturns hands
// back verbatim — built at module-construction time, carrying no position.
// That value is the token a later VALUE dispatch reads its own position from,
// so every opcode downstream used to record 0:0 and the compiled lane rendered
// "source position unknown" where the interpreter has a caret.
//
// The compile-or-fallback gate cannot catch this on its own: it compares the
// two engines, and before the fix they agreed by BOTH losing the position.
// That symmetry is exactly why the hole survived so long, so the pin here
// asserts the position is PRESENT rather than merely equal.
func TestDotAccessKeepsSourcePosition(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{"module-qualified call", `import "boru:test" Assert.not-equal 3 3`},
		{"plain map dot access", `def m {s:1} end m.s`},
		{"dot access inside a paren operand", `def m {a:1} end 10 div (m.a sub 1)`},
		// The /s modifier over a dot-access: the subject is a dynamic(Any)
		// carrier, not a Function, because getNodeReturns deliberately does
		// not narrow a dispatch-bearing field. recordGradualWrap takes the
		// recorded position from that argument, so the whole sugar chain
		// downstream recorded 0:0 until the stamp covered dynamic carriers.
		{"the /s modifier over a dot access", `def m {d:div/v} end 10 0 m.d/s`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatal(err)
			}
			p, _, _, cerr := a.CompileCheck(c.src)
			if cerr != nil || p == nil {
				t.Fatalf("expected a compiled program, got %v", cerr)
			}
			for pc, in := range p.Code {
				if in.Op != compiler.OpCallNative && in.Op != compiler.OpCallNativePoly {
					continue
				}
				if pc >= len(p.Debug) || p.Debug[pc].Row == 0 {
					t.Errorf("%s at pc %d has no source position — a dot-sugar "+
						"access lost its caret on the compiled lane (NUR113)", in.Op.String(), pc)
				}
			}
		})
	}
}
