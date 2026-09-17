package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// codequote is quote's code-capturing sibling (paren-nesting Step 4,
// design/legacy/PAREN-REPRESENTATION.9.ignore): it captures a forward paren RAW as a
// ParenExpr value instead of evaluating it. `quote (expr)` keeps its
// evaluate-then-quote semantics; codequote is the structural-quotability
// form the macro layer wants.

func TestCodequoteCapturesParenAsCode(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`codequote (1 add 2)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("codequote left %d values, want 1", len(res))
	}
	if !core.IsParenExpr(res[0]) {
		t.Errorf("codequote (1 add 2) top is %s, want ParenExpr (raw code)", res[0].Parent.String())
	}
}

// codequote captures the paren WITHOUT evaluating, so an undefined word
// inside is preserved as code rather than raising undefined_word.
func TestCodequoteDoesNotEvaluateContents(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`codequote (nosuchword)`})
	if err != nil {
		t.Fatalf("codequote should not evaluate its paren (got error): %v", err)
	}
	if len(res) != 1 || !core.IsParenExpr(res[0]) {
		t.Fatalf("want a single ParenExpr, got %v", res)
	}
}

// codequote on a word/list behaves exactly like quote.
func TestCodequoteWordAndList(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`codequote foo`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 || !res[0].Parent.Equal(core.TAtom) {
		t.Fatalf("codequote foo top is %v, want Atom", res)
	}
	res2, err := runNativeSteps(t, nil, []string{`codequote [1 add 2]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res2) != 1 || !res2[0].Parent.Equal(core.TList) {
		t.Fatalf("codequote [1 add 2] top is %v, want raw List", res2)
	}
}

// Guard: quote (paren) MUST keep its evaluate-then-quote semantics —
// codequote is a NEW word, not a change to quote (apply/usurp inert-value
// idioms rely on this).
func TestQuoteParenUnchangedByCodequote(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`quote (1 add 2)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 3 {
		t.Errorf("quote (1 add 2) = %v, want 3 (quote semantics unchanged)", res[0])
	}
}
