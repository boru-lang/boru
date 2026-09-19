package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmitRaiseArmDivergence: a fn whose `if`-chain bottoms out in `raise`
// returns a single, unconditional value on every NON-raising path — the raise
// arm diverges (never reaches the merge), so the result is NOT variadic. The
// emitter's variadic accounting (branchVariadicResult / eventDivergesDeep) must
// recognise a CompileDiverges word (raise) as a diverging arm, exactly as the
// shallow fragDiverges already does. Without that, such a fn is flagged
// variadic-returning and every site that consumes its result as a fixed-arity
// operand refuses ("consumes loop results"), forcing a whole-program fallback.
// This is the decision client's `apply-op` shape (an op-dispatch if-chain whose
// default branch raises, then `Assert.equal`/`print` consumes the Boolean).
//
// The negative half pins that a GENUINE 0-or-1 variadic — both arms reach the
// merge with mismatched counts (`if c [n] []`) — still refuses fixed-arity
// consumption and falls back, so the relaxation is scoped to diverging arms.
func TestEmitRaiseArmDivergence(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// Positive: raise-terminated if-chain, result consumed as a fixed arg.
	compiles := []struct{ src, want string }{
		// Two String results (the non-raising arms) concatenated by `add`.
		{`def classify fn [[n:Integer] [String] [if (n gt 0) ["+"] [if (n lt 0) ["-"] [raise bad_input "z"]]]] end (classify 7) (classify -3) add`, "[+-]"},
		// then-arm raises, else-arm returns; the else value is unconditional.
		{`def pos fn [[n:Integer] [Integer] [if (n lt 0) [raise bad_input "neg"] [n]]] end (pos 20) (pos 22) add`, "[42]"},
	}
	for _, c := range compiles {
		a, _ := New()
		prog, _, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (raise arm diverges, result is fixed-arity), but refused", c.src)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native, not island:\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s", c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// Negative: a genuine variadic (both arms reach the merge, counts differ)
	// consumed as a fixed arg must still refuse and fall back to the interpreter.
	variadic := `def maybe fn [[n:Integer] [Integer] [if (n gt 0) [n] []]] end (maybe 5) (maybe 3) add`
	a, _ := New()
	if prog, _, _, _ := a.CompileCheck(variadic); prog != nil {
		t.Errorf("%q: a genuine 0-or-1 variadic consumed as a fixed arg must refuse, but compiled:\n%s", variadic, prog.Disassemble())
	}
	// It must still RUN correctly via the interpreter fallback.
	ar, _ := New()
	gotC, _, errC := ar.RunCompiled(variadic)
	if errC != nil || fmt.Sprint(gotC) != "[8]" {
		t.Errorf("%q: fallback parity broke: gotC=%v errC=%v want=[8]", variadic, gotC, errC)
	}
}
