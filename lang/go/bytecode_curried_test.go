package lang

import (
	"fmt"
	"strings"
	"testing"
)

// REFUSAL-CLOSURE §9.2d (landed 2026-07-17) — a factory body RETURNING a
// nameless verbose-`fn` construction compiles exactly like the lambda form:
// tryReturnedClosure's model now admits any NAMELESS fn value (`fn [...]`
// in a body position constructs Anonymous=false but carries no name; a
// NAMED value keeps the decline — registry dispatch/recursion semantics).
// The returned closure compiles to its own unit with the factory's param
// riding as a capture (§7a's machinery), and the outer apply runs it
// VM-native.
func TestCurriedFactoryCompiles(t *testing.T) {
	// The §9.2 fixture (verbose form) and its lambda twin.
	//
	// These are the WRAPPED spelling, and the wrap is the whole difference
	// (NUR101, design/PAREN-RESTEP-RULE.0.md): the outer paren leaves two
	// survivors, so the park declines and the rewind re-steps the carrier into
	// a call. The UNWRAPPED `(mk 1) 2` places instead, and compiling both as
	// applies is how `(mk2 5) 10` came to answer 11 against the interpreter's
	// `fn (Integer) 10`. The residual lowering now tells them apart by the
	// re-step record ParenReSteppedFnIDs, taken at the collapse.
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] ((mk 1) 2)`, "[3]")
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]] ((mk 1) 2)`, "[3]")
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] (((mk 1)) 2)`, "[3]")

	// The unwrapped twin of the first row, pinned beside it so the pair reads
	// as one rule rather than two behaviours: no enclosing rewind, so the
	// carrier is PLACED and the 2 lands beside it.
	//
	// GRADUATED 2026-08-27 from a refusal to a parity row (Stage 3): the
	// residual-layout loops now skip a placed, unread carrier instead of
	// refusing it on a closure-render fear that measurement did not support.
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] (mk 1) 2`,
		"[fn (Integer) 2]")

	// Decline fences, each parity-faithful:
	// THREE-level currying (a capture threading through two constructions)
	// keeps the refusal.
	{
		src := `def mk3 fn [[a:Integer] [Function] [(fn [[b:Integer] [Function] [(fn [[c:Integer] [Integer] [a add b add c]])]])]] (((mk3 1) 2) 3)`
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(src)
		if prog != nil {
			t.Errorf("three-level currying compiled — graduate this fence to a parity row")
		}
		b, _ := New()
		_, _, errC := b.RunCompiled(src)
		if !strings.Contains(fmt.Sprint(errC), "compile_refused") {
			t.Errorf("three-level currying: err=%v, want compile_refused", errC)
		}
		c, _ := New()
		if out, err := c.RunInterp(src); err != nil || fmt.Sprint(out) != "[6]" {
			t.Errorf("interp = %v (%v), want [6]", out, err)
		}
	}
	// A def-bound returned closure applies with parity through the fallback
	// (the def consumer is a different seam; the value is what matters).
	{
		src := `def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] def f (mk 10) (f 5)`
		b, _ := New()
		gotC, _, errC := b.RunCompiled(src)
		c, _ := New()
		gotI, errI := c.RunInterp(src)
		if errC != nil || errI != nil || fmt.Sprint(gotC) != "[15]" || fmt.Sprint(gotI) != "[15]" {
			t.Errorf("def-bound closure: compiled=%v (%v) interp=%v (%v), want [15]", gotC, errC, gotI, errI)
		}
	}
	// A QUOTED returned fn value stays INERT: lowering it to a closure would
	// drop the Quoted flag and the VM would auto-apply a value the
	// interpreter keeps as data (PR #279 review: compiled [3] vs interp
	// [fn (Integer) 2]). The nameless gate excludes quoted values, so it
	// falls back with parity.
	mustCompileWithParity(t,
		`def mk fn [[] [Function] [quote (fn [[b:Integer] [Integer] [b add 1]])]] ((mk) 2)`,
		"[fn (Integer) 2]")
}
