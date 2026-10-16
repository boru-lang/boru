package core

import (
	"strings"
	"testing"
)

// typed_bind_seam7_test.go drives RunTypedBind's internal-error and
// no-builtin-ancestor arms — the compiled mirror of defTypedHandler's
// refinement branches (OpBindTyped). The happy paths and the
// unify-fail / not-matched arms are exercised by the lang-layer typed-def
// suite; these guards defend against a malformed TypedBindSpec (a compiler
// bug) and are reached here by handing RunTypedBind exactly such a spec.

func tbErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

func TestRunTypedBindPredicateNilConstraint(t *testing.T) {
	r := seam7Reg(t)
	_, err := RunTypedBind(r, &TypedBindSpec{Kind: TypedBindPredicate, Name: "x"}, NewInteger(1))
	tbErr(t, err, "typed bind x has no predicate constraint")
}

func TestRunTypedBindPredicateRunError(t *testing.T) {
	r := seam7Reg(t)
	// A non-fn constraint value makes RunPredicate error; RunTypedBind wraps it.
	bogus := NewInteger(7)
	spec := &TypedBindSpec{Kind: TypedBindPredicate, Name: "x", Describe: "P", Cons: &bogus}
	_, err := RunTypedBind(r, spec, NewInteger(1))
	tbErr(t, err, "def x: predicate type P")
}

func TestRunTypedBindRefineNilTarget(t *testing.T) {
	r := seam7Reg(t)
	_, err := RunTypedBind(r, &TypedBindSpec{Kind: TypedBindRefine, Name: "x"}, NewInteger(1))
	tbErr(t, err, "typed bind x has no refine target")
}

func TestRunTypedBindRefineNoBuiltinAncestor(t *testing.T) {
	r := seam7Reg(t)
	// A refine target whose whole parent chain is OriginUserDef and terminates
	// in nil has no builtin ancestor to unify against.
	inner := &Type{d: &Datum{Origin: OriginUserDef, tmeta: &typeMeta{Behavior: DefaultBehavior}}}
	def := &Type{d: &Datum{Origin: OriginUserDef, Parent: inner, tmeta: &typeMeta{Behavior: DefaultBehavior}}}
	spec := &TypedBindSpec{Kind: TypedBindRefine, Name: "x", Describe: "Sub", Def: def}
	_, err := RunTypedBind(r, spec, NewInteger(1))
	tbErr(t, err, "refine subtype Sub has no builtin ancestor")
}

func TestRunTypedBindDepScalarNilConstraint(t *testing.T) {
	r := seam7Reg(t)
	_, err := RunTypedBind(r, &TypedBindSpec{Kind: TypedBindDepScalar, Name: "x"}, NewInteger(1))
	tbErr(t, err, "typed bind x has no DepScalar constraint")
}

func TestRunTypedBindInvalidKind(t *testing.T) {
	r := seam7Reg(t)
	_, err := RunTypedBind(r, &TypedBindSpec{Kind: TypedBindKind(99), Name: "x"}, NewInteger(1))
	tbErr(t, err, "typed bind x has invalid kind 99")
}
