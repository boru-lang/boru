package lang

import (
	"fmt"
	"strings"
	"testing"
)

// COMPILE FAILURE-CLOSURE §9.2b (landed 2026-07-17) — a splice marker re-stepped
// over a COMPUTED payload compiles to OpSpliceDyn: a DATA payload spreads
// its values verbatim at run time (spliceExpand — the interpreter's own
// marker semantics), and a CODE-BEARING or fn-valued payload DEFERS to the
// interpreter (the re-step dispatches against the live stack, which only
// the interpreter owns) — byte-identical either way. The spread's count is
// runtime-variable, so the event is variadic: only the program residual
// absorbs it, and fixed-arity consumers keep the standard compile failures.
func TestSpliceDynComputedPayloadCompiles(t *testing.T) {
	// The §9.2 fixture: a computed list payload spreads.
	mustCompileWithParity(t, `def xs (range 1 3) def d word xs d`, "[1 2]")
	// A fn-built payload.
	mustCompileWithParity(t, `def mk fn [[] [List] [[7 8]]] def xs (mk) def d word xs d`, "[7 8]")
	// A non-list computed payload contributes ITSELF (identity).
	mustCompileWithParity(t, `def n (1 add 2) def d word n d`, "[3]")
	// The spread feeding the residual under later statements.
	mustCompileWithParity(t, `def xs (range 1 3) def d word xs d 9`, "[1 2 9]")

	// A CODE-BEARING computed payload (a Forth-style macro) defers to the
	// interpreter at run time — full parity through the fallback.
	deferSrc := `def mkc fn [[] [List] [(quote [1 add 2])]] def xs (mkc) def d word xs d`
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(deferSrc)
	if cerr != nil || prog == nil {
		t.Fatalf("code-bearing payload must COMPILE (the defer is runtime): reason=%q err=%v", reason, cerr)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(deferSrc)
	if noteCompileDefect(t, deferSrc, gotC, errC) {
		return
	}
	c, _ := New()
	gotI, errI := c.RunInterp(deferSrc)
	if compiled {
		t.Errorf("code-bearing payload must DEFER to the interpreter, reported a compiled run")
	}
	if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[3]" {
		t.Errorf("defer parity: compiled=%v (%v) interp=%v (%v), want [3] both", gotC, errC, gotI, errI)
	}

	// The BARE `word xs` form is the SAME site (an __SP marker fires the
	// instant it is stepped standalone) and graduates with it.
	mustCompileWithParity(t, `def mk fn [[] [List] [[7 8]]]  def xs (mk)  word xs`, "[7 8]")

	// A RE-READ of the payload def after the spread keeps the compile failure:
	// the splice reassigns the payload's provenance to the spread event, so
	// the trailing `xs` would resolve to the variadic spread instead of the
	// original list (PR #279 review: compiled [1 2 2] vs interp [1 2 [1 2]]).
	mustFailToCompileWithParity(t, `def xs (range 1 3) def d word xs d xs`,
		"splice payload re-read after the spread")
	_ = strings.Contains
}
