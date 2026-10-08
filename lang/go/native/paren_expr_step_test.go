package native

import "testing"

// Step 2 of design/legacy/PAREN-REPRESENTATION.9.ignore: a word-context ParenExpr
// evaluated when stepped on the main stack. The parser does not yet emit
// these in word context (Step 1), so these tests construct them directly
// and pin the four contracts — results flow out, defs leak, errors
// propagate (unlike `do`), and the inner expression starts with a fresh
// operand view (the OpenParen stack barrier).

func TestParenExprStepSingleResult(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// (1 add 2) → 3
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runBoru(t, r, []Value{pe})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %v", result)
	}
	if n, _ := AsInteger(result[0]); n != 3 {
		t.Fatalf("expected 3, got %v", result[0])
	}
}

func TestParenExprStepMultipleResultsFlowOut(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// (1 2 3) → 1 2 3 on the stack (results flow out)
	pe := NewParenExpr([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	result := runBoru(t, r, []Value{pe})
	if len(result) != 3 {
		t.Fatalf("expected 3 results to flow out, got %v", result)
	}
}

func TestParenExprStepResultsThenMore(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// (1 2 3) 99 → 1 2 3 99 (results splice in before following tokens)
	pe := NewParenExpr([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	result := runBoru(t, r, []Value{pe, NewInteger(99)})
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %v", result)
	}
	if n, _ := AsInteger(result[3]); n != 99 {
		t.Fatalf("expected trailing 99, got %v", result[3])
	}
}

func TestParenExprStepDefLeaks(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// (def x 1) x → 1  (defs leak to the enclosing scope)
	pe := NewParenExpr([]Value{NewWord("def"), NewWord("x"), NewInteger(1)})
	result := runBoru(t, r, []Value{pe, NewWord("x")})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %v", result)
	}
	if n, _ := AsInteger(result[0]); n != 1 {
		t.Fatalf("expected leaked x=1, got %v", result[0])
	}
}

// Step 3: a ParenExpr in a word's forward window is pre-evaluated so the
// outer word dispatches on its result (preEvalParens ParenExpr branch).

func TestParenExprForwardArgTrailing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// add 10 (1 add 2) → 13
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runBoru(t, r, []Value{NewWord("add"), NewInteger(10), pe})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %v", result)
	}
	if n, _ := AsInteger(result[0]); n != 13 {
		t.Fatalf("expected 13, got %v", result[0])
	}
}

func TestParenExprForwardArgLeading(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// add (1 add 2) 10 → 13 (paren first in the forward window)
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	result := runBoru(t, r, []Value{NewWord("add"), pe, NewInteger(10)})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %v", result)
	}
	if n, _ := AsInteger(result[0]); n != 13 {
		t.Fatalf("expected 13, got %v", result[0])
	}
}

func TestParenExprStepErrorPropagates(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// (1 div 0) 99 → error propagates (NOT caught and reified like `do`)
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("div"), NewInteger(0)})
	if e := runBoruError(t, r, []Value{pe, NewInteger(99)}); e == nil {
		t.Fatal("expected division-by-zero error to propagate from paren")
	}
}
