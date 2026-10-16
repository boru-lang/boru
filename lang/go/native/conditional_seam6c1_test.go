package native

import (
	"strings"
	"testing"
)

// Wave-6 coverage for conditional.go: caseClauses' predicate-error and
// unbound-word-match arms, caseReturnsFn's nil-backing clause list, and
// caseBranchJoin's no-clause fallback.

func TestCaseClausesPredicateBodyError(t *testing.T) {
	r := seam5Reg(t)
	// The predicate body [no-such-word-xyz] raises undefined_word when it
	// runs against the case value; caseClauses must propagate the error.
	_, err := seam5Run(r, `case 5 [[no-such-word-xyz] 1 2]`)
	if err == nil {
		t.Fatal("expected predicate-body error to propagate")
	}
	if !strings.Contains(err.Error(), "no-such-word-xyz") {
		t.Errorf("error should name the failing word, got: %v", err)
	}
	// Positive pair: a working predicate clause still matches.
	out, err := seam5Run(r, `case 5 [[gt 3] 1 2]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "1" {
		t.Fatalf("case [gt 3] should pick 1, got %v", out)
	}
}

func TestCaseClausesUnboundWordMatchFallsToAtom(t *testing.T) {
	r := seam5Reg(t)
	// The bare-word match `blue` is unbound: it resolves through
	// ResolveWordValue to Atom(blue). Against integer 5 it fails to
	// unify, so the default fires.
	out, err := seam5Run(r, `case 5 [blue 1 2]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "2" {
		t.Fatalf("unmatched atom clause should fall to default 2, got %v", out)
	}
	// Positive pair: the same unbound-word match unifies with an atom value.
	out, err = seam5Run(r, `case blue/q [blue 1 2]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "1" {
		t.Fatalf("atom-vs-atom clause should pick 1, got %v", out)
	}
}

func TestCaseReturnsFnNilBackedClauses(t *testing.T) {
	r := seam5Reg(t)
	defer r.Check.Begin()()
	// NewList(nil) is a concrete code-body list whose ReadList has no
	// backing slice — caseReturnsFn must return the dynamic-Any carrier
	// instead of walking elements.
	out := caseReturnsFn([]Value{NewInteger(5), NewList(nil)}, r)
	if len(out) != 1 || !ParentOf(&out[0]).Equal(TAny) {
		t.Fatalf("nil-backed clauses should yield a dynamic Any carrier, got %v", out)
	}
}

func TestCaseBranchJoinNoClauses(t *testing.T) {
	r := seam5Reg(t)
	defer r.Check.Begin()()
	// With no clause elements there is nothing to join: the helper's
	// contract is a dynamic-Any fallback.
	out := caseBranchJoin(r, NewInteger(1), nil)
	if len(out) != 1 || !ParentOf(&out[0]).Equal(TAny) {
		t.Fatalf("empty clause join should yield dynamic Any, got %v", out)
	}
	// Positive pair: a single clause pair joins to the block's type.
	out = caseBranchJoin(r, NewInteger(1), []Value{NewInteger(1), NewString("x")})
	if len(out) != 1 || !ParentOf(&out[0]).ConformsTo(TString) {
		t.Fatalf("single-clause join should carry String, got %v", out)
	}
}
