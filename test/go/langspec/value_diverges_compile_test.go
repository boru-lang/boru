package langspec

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestValueDivergesCompilesNative pins the CompileValueDiverges mechanism that
// cleared the last interpreter island (error.tsv:25): a static-zero integer
// div/mod raises value-dependently, so a closure body ending in it compiles as
// a divergent terminal (no RET, the catching `do` wraps the raised error)
// instead of islanding. islandGate=0 guards the corpus; this documents the
// specific shape and the runtime parity.
func TestValueDivergesCompilesNative(t *testing.T) {
	t.Parallel()
	// Positive: a static-zero div/mod inside a `do` body compiles FULLY NATIVE.
	native := []string{
		`def e (do [1 div 0]) convert Map e`, // the cleared island (error.tsv:25)
		`do [1 div 0]`,
		`do [1 mod 0]`,
	}
	for _, src := range native {
		prog, reason, _, err := compileRow(t, src)
		if err != nil {
			t.Fatalf("%q: check error: %v", src, err)
		}
		if prog == nil {
			t.Fatalf("%q: declined (reason %q); expected native compile", src, reason)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: compiled with an interpreter island; expected fully native:\n%s", src, prog.Disassemble())
		}
	}

	// Correctness: the divergence is a RUNTIME raise, not a compile-time fold —
	// compiled output must equal interpreted for both the raising and the
	// normal path (the differential gates cover the corpus; these pin the
	// mechanism directly, including a non-zero divisor that must NOT diverge).
	parity := []struct {
		src  string
		want string
	}{
		{`def e (do [1 div 0]) convert Map e`, "{code:arith_error message:'division by zero'}"},
		{`def e (do [7 mod 0]) convert Map e`, "{code:arith_error message:'modulo by zero'}"},
		{`10 div 2`, "5"},
		{`10 mod 3`, "1"},
	}
	for _, c := range parity {
		a, _ := lang.New()
		a.SetClock(specClock)
		gotC, compiled, errC := a.RunCompiled(c.src)
		b, _ := lang.New()
		b.SetClock(specClock)
		gotI, errI := b.RunInterp(c.src)
		if errC != nil || errI != nil {
			t.Errorf("%q: run error compiled=%v interp=%v", c.src, errC, errI)
			continue
		}
		if !compiled {
			t.Errorf("%q: did not run compiled", c.src)
		}
		if gc, gi := renderAny(gotC), renderAny(gotI); gc != gi || gi != c.want {
			t.Errorf("%q: compiled=%s interp=%s want %s", c.src, gc, gi, c.want)
		}
	}
}
