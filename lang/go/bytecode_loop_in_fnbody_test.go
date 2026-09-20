package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Off-corpus regressions for loop-in-fn-body lowering — a SIDE-EFFECT `for` loop
// (body nets 0 per iteration) inside a fn/closure body. Before this feature such
// a loop declined ("body leaves extra values" / "result is a variadic loop
// value") because RecordLoop unconditionally marked every loop's result variadic
// and only the program residual could absorb a variadic. A 0-output loop in fact
// leaves ZERO runtime values, so it is a zeroOut event the unit drops and RETs
// cleanly. The langspec differential is blind to these fn-body-loop shapes, so
// they are pinned here as RunCompiled==Run (compile MUST equal interpret).

// loopSound asserts compile == interpret (value AND error agreement) for src.
func loopSound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, gerr) {
		return
	}
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

// A side-effect loop as a non-final statement inside a value-returning fn body:
// it mutates a captured Array and the fn returns a later value. Compiles to a
// native lowering (no FALLBACK island) and runs == interpret.
func TestLoopInFnBody_SideEffectStatement(t *testing.T) {
	const src = `def sum-to fn [[n:Integer] [Integer] [
		def acc (flex [0])
		for n [ def _ (acc set 0 ((acc get 0) add 1)) end ]
		acc get 0
	]]
	sum-to 5`
	loopSound(t, src)
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("did not compile: reason=%q err=%v", reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected a native lowering of the fn-body loop, got an island")
	}
}

// A side-effect loop as the ENTIRE body of a 0-return fn (the `describe`/`test`
// grouping-body shape): the unit RETs nothing; the loop runs for effects.
func TestLoopInFnBody_ZeroReturnWholeBody(t *testing.T) {
	const src = `def each-print fn [[n:Integer] [] [ for n [ print 0 ] ]]
	each-print 3`
	loopSound(t, src)
}

// Nested side-effect loops inside a fn body.
func TestLoopInFnBody_Nested(t *testing.T) {
	const src = `def grid fn [[n:Integer] [Integer] [
		def acc (flex [0])
		for n [ for n [ def _ (acc set 0 ((acc get 0) add 1)) end ] ]
		acc get 0
	]]
	grid 4`
	loopSound(t, src)
}

// SOUNDNESS SCOPING (the divergence guard): a side-effect loop whose ZERO-value
// result is CONSUMED — bound by `def x (for …)` — must NOT be miscompiled. The
// interpreter's `def` forward-collection over the empty loop grabs the next token
// (`def x (for n [print 0]) n` → `def x n`, so the body nets 0 and the fn raises
// "expected 1 return value, got 0"); the compiled 0-value loop cannot replicate
// that, so the lowering declines and falls back. Pinned: compile == interpret
// (both raise the identical error). A regression that compiled this would diverge
// (compiled returns n, interpret errors) — invisible to the differential.
func TestLoopInFnBody_ConsumedResultStaysSound(t *testing.T) {
	cases := []string{
		// bound to a value-def
		`def f fn [[n:Integer] [Integer] [ def x (for n [print 0]) n ]] f 3`,
		// fed as an operand to another word
		`def g fn [[n:Integer] [Integer] [ (for n [print 0]) add 1 ]] g 2`,
	}
	for i, src := range cases {
		t.Run(fmt.Sprintf("case%d", i), func(t *testing.T) { loopSound(t, src) })
	}
}

// A VALUE-producing loop (body nets one value per iteration) inside a fn body is
// genuinely variadic and is not covered by the side-effect lowering — it must
// stay sound (decline → fall back, or whatever the engines agree on). Pinned so a
// future change to the variadic path keeps compile == interpret.
func TestLoopInFnBody_ValueProducingStaysSound(t *testing.T) {
	const src = `def collect fn [[n:Integer] [List] [ for n [ 7 ] ]]
	collect 3`
	loopSound(t, src)
}
