package test

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// fn's 3-arg triple form and afn both construct Function values whose
// operands can be COMPUTED (`fn (2 add 3) [String] ['five']`, `(2 add 3)
// afn [body]`). Under static analysis a computed operand resolves to a
// carrier, and the check-time construction the compiler would intern as
// a constant carries that carrier where the live value belongs (an
// Integer carrier as the input pattern instead of 5) — baking it made
// `d 5` refuse dispatch in the compiled engine while the interpreter,
// re-constructing with the live value, matched. fnTripleHandler and
// afnHandler now mark such constructions uncompilable so the program
// falls back to the interpreter, and a carrier PATTERN on a signature
// is never enforced at match time (patternsOk / MatchSignature /
// MatchFnSig skip it) so the CHECK pass has no false positive either.
// This test pins compiled == interpreted for the operand shapes. The
// cases live here rather than in lang/spec because these constructions
// are deliberately interpreter-only and the spec corpus enforces
// refusalCeiling = 0 (every spec value row must compile).
func TestFnConstructionCompiledParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	cases := []struct {
		src  string
		want string // the (shared) result both engines must produce
	}{
		// Computed triple-form input — the miscompile case: the input
		// pattern must be the LIVE 5, not the check-time carrier.
		{`def d fn (2 add 3) [String] ["five"]  d 5`, `[five]`},
		// Computed afn input — same class, pre-existing before the
		// triple form; fixed by the same guard.
		{`def d ((2 add 3) afn ["got5"])  d 5`, `[got5]`},
		// Fully-literal constructions still compile natively and agree.
		{`def d fn 5 [String] ["five"]  d 5`, `[five]`},
		{`def d fn x:Integer [Integer] [x mul 2]  d 4`, `[8]`},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			ai, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			interp, ierr := ai.RunInterp(c.src)
			if ierr != nil {
				t.Fatalf("interp: %v", ierr)
			}
			ac, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			comp, _, cerr := ac.RunCompiled(c.src)
			if cerr != nil {
				t.Fatalf("compiled: %v", cerr)
			}
			is, cs := fmt.Sprint(interp), fmt.Sprint(comp)
			if is != c.want {
				t.Errorf("interpreted = %s, want %s", is, c.want)
			}
			if cs != is {
				t.Errorf("compiled = %s diverges from interpreted = %s (miscompile)", cs, is)
			}
		})
	}
}

// The negative side of the parity contract: a computed input pattern is
// still ENFORCED at runtime (the guard only defers construction to the
// interpreter, it does not loosen the constructed fn), and the check
// pass stays CLEAN on the deferred construction (the carrier pattern is
// not a static false positive).
func TestFnComputedPatternStillEnforced(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := a.Run(`def d fn (2 add 3) [String] ["five"]  d 9`); err == nil {
		t.Fatal("d 9 must not match the computed input pattern 5")
	}
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := ac.Check(`def d fn (2 add 3) [String] ["five"]  d 5`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		t.Errorf("check must be clean on a computed-input construction, got: %v", d)
	}
}

// A single-parameter, single-return signature has SIX valid spellings,
// differing only in how many square-bracket pairs they use. STYLE-GUIDE.md
// §S1 tells authors to write the one with the fewest, and this test is why
// that is a STYLE rule rather than a semantic one: all six build a
// canon-identical value, so the choice among them carries no meaning.
//
// The value half (each spelling computing 42) is pinned in lang/spec/
// fn-triple.tsv §2b, which is where a spec row belongs. The identity half
// lives here instead, for the reason that file's header gives for computed
// operands: `deq (canon a/v) (canon b/v)` passes a function VALUE to a
// word, which the compiler refuses (Stage 3, soundness), and the spec
// corpus holds refusalCeiling = 0.
//
// `boru fmt` collapses only the last of the six (NUR088), which is what
// makes the rule worth stating in a guide at all.
func TestFnSignatureSpellingsAreOneValue(t *testing.T) {
	const target = `def a fn x:Integer Integer [mul 2 x]  ` // 1 pair — fewest

	for _, c := range []struct {
		pairs int
		spell string
	}{
		{2, `def b fn x:Integer [Integer] [mul 2 x]`},
		{2, `def b fn [x:Integer Integer [mul 2 x]]`},
		{3, `def b fn [[x:Integer] Integer [mul 2 x]]`},
		{3, `def b fn [x:Integer [Integer] [mul 2 x]]`},
		{4, `def b fn [[x:Integer] [Integer] [mul 2 x]]`},
	} {
		t.Run(fmt.Sprintf("pairs%d/%s", c.pairs, c.spell), func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			got, err := a.Run(target + c.spell + `  deq (canon a/v) (canon b/v)`)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if s := fmt.Sprint(got); s != "[true]" {
				t.Errorf("canon %s != canon of the 1-pair form: got %s", c.spell, s)
			}
		})
	}
}

// The seventh spelling is NOT a longer way of writing the first: a LIST in
// the input position always selects fn's spec-list signature, so a lone
// bracketed input strands the output and body as separate arguments. This
// is the one shape STYLE-GUIDE.md §S1 calls out as invalid rather than
// merely verbose.
func TestFnBracketedInputAloneIsNotATripleForm(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := a.Run(`def f fn [x:Integer] [Integer] [mul 2 x]  f 21`); err == nil {
		t.Fatal("a bracketed input alone must not parse as the 3-arg triple form")
	}
}
