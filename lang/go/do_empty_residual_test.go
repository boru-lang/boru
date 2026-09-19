package lang_test

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestDoEmptyResidualAgreesAcrossEngines pins the arity model for a `do` body
// that leaves NOTHING — the shapes that agree, and the one that did not.
//
// CLOSED 2026-09-05: the each-over-a-net-zero-do row agrees on both lanes
// since the compile-time top trim of callback bodies was retired (NUR120 —
// the each body's residual is kept whole, and the seam's own discipline
// decides what the handler sees). The history below is kept because the
// five model-layer attempts it records are still the constraints any change
// to the `do` arity model has to satisfy; the row now pins the agreement.
//
// A non-empty body with an empty residual has two runtime shapes:
//
//   - it RAISED (`do [raise "x"]`) — `do` catches it and nets ONE Error value;
//   - it COMPLETED with a net-zero stack (`do [9 drop]`) — it nets NOTHING.
//
// `doListReturnsFn` models both as `[Error]`, distinguishing them only by
// token count — which separates `do []` from the rest but says nothing about
// raising. For the second shape that over-reports the count by one, and the
// value a consumer was supposed to keep goes missing:
//
//	[1 2 3] each [do [9 drop]]
//	  compiled    → [boru/each_error]: element 0: body produced no result
//	  interpreted → [1 2 3]        (the identity map — the element survives)
//
// Default mode, no fallback warning, silently different answers. The
// interpreter is canonical: the element survives a body that touched nothing.
//
// THE DIVERGENT ROW IS PINNED AS DIVERGENT, deliberately, because five fixes
// were tried at the model layer and each traded this defect for a worse one.
// Recording which, so the next attempt starts past them:
//
//  1. Model it empty in the compile pass. Fixes `each`, and makes every
//     net-zero `do` UNLOWERABLE — a `do` dispatch with zero modelled outputs
//     cannot be seated, so `def b (quote [break]) for 5 [do b i]` stops
//     compiling (TestEdgeFindingQuotedDoBodyFlowEscapesLoop).
//  2. Model it empty everywhere. Same, plus it breaks the raise-then-read
//     idiom: `do [raise "x"] dot code` leaves `dot` no receiver, and the
//     program compiles and raises signature_error instead of falling back.
//  3. MarkUncompilable on the shape. Refuses the whole `do […] error […]`
//     family — ten tests that pin those compile paths by name.
//  4. SetCatchVariadic — the mechanism built for exactly a runtime-variable
//     count. The latch IS consumed (verified by instrumenting
//     catchVariadicFor: pending, and a CompileFallbackBody sig takes it), but
//     marking the `do` event's result variadic does not stop the ENCLOSING
//     each-body closure from being seated at the over-reported count.
//  5. A gradual `dynamic(Any)` result instead of a strict Error carrier —
//     one output, so still lowerable, and variable so the consumer should
//     decline. It does not decline.
//
// Discrimination is not the blocker either: `doBodyMayRaise` is deliberately
// conservative (any registered word is fallible, so `drop` qualifies), a
// diagnostic-delta around the body analysis is zero for BOTH shapes
// (measured), and a `CompileDiverges`-keyed token scan does separate them —
// but every model it selects still runs into 1-5 above.
//
// So the fix is not in the model. The each-body unit is analysed with the
// residual [element, Error] and emits a unit that returns NEITHER; the
// mismatch between an analysed residual and what the unit actually RETs is a
// closure-body seating question in the emitter, and that is where the next
// attempt belongs.
func TestDoEmptyResidualAgreesAcrossEngines(t *testing.T) {
	cases := []struct {
		name, src, want string
		// wantDiverge marks a shape the engines do NOT yet agree on. want is
		// then the INTERPRETER's answer, which is canonical here.
		wantDiverge bool
	}{
		// The formerly open divergence (closed 2026-09-05, see the header).
		{name: "each over a net-zero do body",
			src: `[1 2 3] each [ do [ 9 drop ] ]`, want: "[[1 2 3]]"},

		// The shapes that DO agree, and that any fix must keep agreeing. These
		// are not filler: 1-5 above each broke at least one of them, so they
		// are the constraints the next attempt has to satisfy.
		{name: "each over a net-zero body, no nested do",
			src: `[1 2 3] each [ 9 drop ]`, want: "[[1 2 3]]"},
		{name: "each over an empty body",
			src: `[1 2 3] each [ ]`, want: "[[1 2 3]]"},
		{name: "net-zero do at the top level",
			src: `do [ 9 drop ]`, want: "[]"},
		// The raise half: a body that raises nets one caught Error, and a field
		// read on it must resolve. Approach 2 broke exactly this.
		{name: "raise, then read the caught error's code",
			src: `do [ raise "x" ] dot code`, want: "[user_error]"},
		{name: "static div-by-zero, then read the code",
			src: `do [ 1 0 div ] dot code`, want: "[arith_error]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := lang.New()
			compiled, cErr := a.Run(c.src)
			b, _ := lang.New()
			interp, iErr := b.RunInterp(c.src)

			if iErr != nil {
				t.Fatalf("the interpreter is canonical here and must run clean: %v", iErr)
			}
			if is := fmt.Sprint(interp); is != c.want {
				t.Errorf("interpreted %s, want %s — a change to the canonical answer "+
					"is a semantic change, not a compile detail", is, c.want)
			}

			if lang.NoteCompileDefect(t, c.src, compiled, cErr) {
				// No compiled answer to agree with: the failure is booked.
				return
			}
			agree := cErr == nil && fmt.Sprint(compiled) == fmt.Sprint(interp)
			switch {
			case c.wantDiverge && agree:
				t.Errorf("this shape is recorded as an OPEN divergence but the "+
					"engines now agree (%v). That is progress — drop wantDiverge "+
					"from this row and delete the corresponding note from the "+
					"function comment.", compiled)
			case !c.wantDiverge && !agree:
				t.Errorf("compiled %v (err %v) != interpreted %v — the `do` arity "+
					"model has regressed; see the five rejected approaches in this "+
					"test's comment before changing doListReturnsFn.",
					compiled, cErr, interp)
			}
		})
	}
}

// TestEachBodyThatConsumesItsElementStillErrors is the negative half at the
// consumer: `each_error` must still fire when the body genuinely eats its
// element. Any fix for the divergence above has to keep this — the two cases
// are indistinguishable from the handler (`len(res) == 0`) and are told apart
// only by whether the element survived the frame, so a fix that simply
// silences the error would break this instead.
func TestEachBodyThatConsumesItsElementStillErrors(t *testing.T) {
	const src = `[1 2 3] each [ drop ]`
	for _, mode := range []string{"compiled", "interpreted"} {
		a, _ := lang.New()
		var err error
		if mode == "compiled" {
			_, err = a.Run(src)
		} else {
			_, err = a.RunInterp(src)
		}
		if err == nil {
			t.Errorf("%s: a body that consumes its element leaves each nothing to "+
				"collect and must raise", mode)
		}
	}
}
