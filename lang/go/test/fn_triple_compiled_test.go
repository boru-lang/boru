package test

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

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
// word, which the compiler declines (Stage 3, soundness), and the spec
// corpus holds failureCeiling = 0.
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
			got, err := runReference(t, a, target+c.spell+`  deq (canon a/v) (canon b/v)`)
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
