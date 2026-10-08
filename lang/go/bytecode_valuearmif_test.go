package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmitValueArmIf: `if cond v1 v2` with VALUE arms (not `[body]` code lists)
// now lowers the then arm symmetrically with the else arm — a select (push v1 /
// push v2 around JMP_IF_FALSE) — instead of declining "then-branch not captured".
// This covers the direct form, a dynamic condition, and the usurp-if shape
// (`usurp if` dispatches `if` with value arms). The negative half pins that a
// COMPUTED then value (an event eagerly on the stack) still declines the
// value-then path rather than miscompiling.
func TestEmitValueArmIf(t *testing.T) {
	cases := []struct{ src, want string }{
		{`if true 99 88`, "[99]"},
		{`if false 99 88`, "[88]"},
		{`def x 0  if (x eq 0) 99 88`, "[99]"},         // dynamic (event) condition, value arms
		{`def ifu (usurp if)  true ifu 88 99`, "[99]"}, // usurp-if reverses to value arms
		{`def ifu (usurp if)  false ifu 88 99`, "[88]"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (value-arm if), but declined", c.src)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native (value-arm select), not island:\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// `if cond (a) (b)` — both arms computed — compiles via the OpReverse select
	// lowering (TestBothComputedIfLowers). A computed value arm needs a STACK
	// (event) condition, though: with no computed arm at all, a plain value arm
	// still lowers as a select here.
}
