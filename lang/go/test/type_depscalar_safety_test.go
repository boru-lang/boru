package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- DepScalar safety: don't fall through scalar dispatch ---
//
// `*Type.ConformsTo` reports `DepInteger.ConformsTo(TInteger)` as
// true (used by sig matching). The risk is any code that does
// `if v.Parent.ConformsTo(TString) { _, _ := v.engine.AsString() }` — on a
// DepScalar payload, AsString errors, the underscore swallows the
// error, and the caller gets a zero value. These tests pin the
// DepScalar-specific branches in the four most-traveled equality /
// formatting paths.

func runOK(t *testing.T, src string) []any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	got, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return got
}

// --- valuesEqual: two equal DepScalars compare equal ---

func TestDepScalar_EqEqual(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer gt 10)
a eq b`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Int gt 10) eq (Int gt 10) = %v, want [true]", got)
	}
}

// Two different DepScalars (different bound) compare unequal.
func TestDepScalar_EqDifferentBound(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer gt 20)
a eq b`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Int gt 10) eq (Int gt 20) = %v, want [false]", got)
	}
}

// Different-side DepScalars compare unequal.
func TestDepScalar_EqDifferentKind(t *testing.T) {
	got := runOK(t, `def a (Integer gt 10)
def b (Integer lt 10)
a eq b`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(Int gt 10) eq (Int lt 10) = %v, want [false]", got)
	}
}

// Interval DepScalars compare equal only when both bounds match.
// Run as separate program invocations because `def x EXPR` (without
// extra parens) stores EXPR as deferred code, and re-evaluating it
// in sequence interacts with stack state in non-obvious ways.
func TestDepScalar_EqInterval(t *testing.T) {
	got := runOK(t, `def a (Integer between 10 20)
def b (Integer between 10 20)
a eq b`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("[10,20] eq [10,20] = %v, want [\"true\"]", got)
	}

	got = runOK(t, `def a (Integer between 10 20)
def c (Integer between 10 25)
a eq c`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("[10,20] eq [10,25] = %v, want [\"false\"]", got)
	}
}

// --- compareValues: DepScalars take part in the total order ---

func TestDepScalar_LtTotalOrder(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	got, err := runReference(t, a, `(Integer gt 10) lt (Integer gt 20)`)
	if err != nil {
		t.Fatalf("comparing DepScalars with lt errored: %v", err)
	}
	// The order is total, so DepScalars compare without error. They
	// carry no Comparer and no structure to recurse into, so the tie
	// breaks on the canonical form: "(Integer gt 10)" < "(Integer gt 20)".
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(Int gt 10) lt (Int gt 20) = %v, want [true]", got)
	}
}

// --- formatValueJSON / boru_error: render the constraint, no panic ---

// `print` formats via formatForPrint — already guarded but keep the
// regression test so any future reorder doesn't lose it.
func TestDepScalar_PrintRendersConstraint(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("print panicked: %v", r)
		}
	}()
	_, err = a.Run(`(Integer gt 10) print`)
	if err != nil {
		t.Fatalf("print errored: %v", err)
	}
}

// --- Recover-based panic guard: a DepScalar flowing through any
// surface that uses ConformsTo(scalar)→AsX must not panic. Exercises
// eq, lt (caught), and print together.
func TestDepScalar_NoPanicOnHotPaths(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("hot-path panicked on DepScalar: %v", r)
		}
	}()
	// Each line exercises a different path. Errors from compareValues
	// are expected and tolerated; a panic would fail the test.
	_, _ = a.Run(`def x (Integer gte 10)
def y (Integer gte 10)
x eq y
x print`)
}

// --- DepScalar in sig positions across all scalar bases ---
//
// `ResolveSigType` historically had a dispatch ordering bug: String
// and Atom DepScalars matched the IsWord/String/Atom name-resolution
// branch (because their Parent.ConformsTo(TString)/TAtom), where
// AsString silently failed and ResolveTypeName errored on the empty
// name. The fix guards that branch with !v.IsDepScalar() so all
// scalar-base DepScalars fall into the scalar-pattern branch (kind =
// base type, pattern = the DepScalar Value). These tests pin each
// base type so a future re-shuffle doesn't reintroduce the asymmetry.

func TestDepScalar_InSig_Integer(t *testing.T) {
	got := runOK(t, `def f fn [[n:(Integer gt 10)] [Integer] [n]]
20 f`)
	if len(got) != 1 || got[0] != int64(20) {
		t.Errorf("Integer DepScalar in sig: got %v, want [20]", got)
	}
}

func TestDepScalar_InSig_Float(t *testing.T) {
	got := runOK(t, `def f fn [[n:(Float gte 1.5)] [Float] [n]]
2.5 f`)
	if len(got) != 1 || got[0] != "2.5" {
		t.Errorf("Float DepScalar in sig: got %v, want [2.5]", got)
	}
}

func TestDepScalar_InSig_String(t *testing.T) {
	got := runOK(t, `def f fn [[s:(String lt "z")] [Boolean] [true]]
"a" f`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("String DepScalar in sig: got %v, want [true]", got)
	}
}

func TestDepScalar_InSig_Atom(t *testing.T) {
	got := runOK(t, `def f fn [[a:(Atom gt foo/q)] [Boolean] [true]]
zoo/q f`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("Atom DepScalar in sig: got %v, want [true]", got)
	}
}

// Constraint-violating args reject via the sig matcher, not at
// sig-parse time. These are the negative complements to InSig_*.

func TestDepScalar_InSig_String_RejectsOutOfBound(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def f fn [[s:(String lt "z")] [Boolean] [true]]
"z" f`)
	if err == nil {
		t.Errorf("expected sig mismatch for \"z\" lt \"z\", got nil")
	}
}

func TestDepScalar_InSig_Atom_RejectsOutOfBound(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(`def f fn [[a:(Atom gt foo/q)] [Boolean] [true]]
foo/q f`)
	if err == nil {
		t.Errorf("expected sig mismatch for foo gt foo, got nil")
	}
}

// --- DepScalar bound to a refine subtype ---
//
// `def x:Pos (Integer gt 10)` exercises the refine-bare reparent
// branch with a DepScalar body (not the usual Integer scalar). The
// reparent must preserve the DepScalarInfo payload while updating
// Parent to Pos, so `IsDepScalar(x)` stays true, `typeof x` reports
// Pos, and the rendered form reads `(Pos gt 10)` (leaf = Parent.Name).

func TestDepScalar_BoundToRefineSubtype(t *testing.T) {
	got := runOK(t, `def Pos refine Integer
def x:Pos (Integer gt 10)
typeof x`)
	if len(got) != 1 || got[0] != "Pos" {
		t.Errorf("typeof reparented DepScalar: got %v, want [Pos]", got)
	}
}

func TestDepScalar_BoundToRefineSubtype_IsPos(t *testing.T) {
	got := runOK(t, `def Pos refine Integer
def x:Pos (Integer gt 10)
x is Pos`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("reparented DepScalar is Pos: got %v, want [true]", got)
	}
}

func TestDepScalar_BoundToRefineSubtype_IsAncestor(t *testing.T) {
	got := runOK(t, `def Pos refine Integer
def x:Pos (Integer gt 10)
x is Integer`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("reparented DepScalar is Integer (ancestor): got %v, want [true]", got)
	}
}

// `inspect x` reports the constraint with the leaf name flipped to
// the subtype — proves the payload survived reparenting and the
// renderer reads `v.Parent.Name` (= "Pos") rather than caching the
// pre-reparent base name.
func TestDepScalar_BoundToRefineSubtype_InspectShowsSubtype(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	got, err := a.Run(`def Pos refine Integer
def x:Pos (Integer gt 10)
inspect x`)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("inspect: got %d results, want 1: %v", len(got), got)
	}
	s := fmt.Sprintf("%v", got[0])
	for _, want := range []string{"dependent_scalar", "leaf:'Pos'", "kind:'gt'"} {
		if !strings.Contains(s, want) {
			t.Errorf("inspect output missing %q: %s", want, s)
		}
	}
}

// `behave compare/q` on a refine subtype must dispatch for DepScalar
// values reparented to that subtype, not just for concrete Integer
// inhabitants. The LCA walk for `x cmp y` lands on Pos regardless of
// whether the operands are concrete (Parent=Pos, Data=IntPayload) or
// constraint values (Parent=Pos, Data=DepScalarInfo) — the user
// Comparer owns the result either way. The test pins this with a
// constant-returning Comparer so the body doesn't depend on the
// payload shape.
func TestDepScalar_BehaveCmpOnRefineSubtype(t *testing.T) {
	got := runOK(t, `def Pos refine Integer
behave compare/q (fn [[a:Pos b:Pos] [Integer] [42]])
def x:Pos (Integer gt 10)
def y:Pos (Integer gt 20)
x cmp y`)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(got), got)
	}
	// User Comparer returns 42, normalised to +1 by the cmp word.
	// We accept any of {42, 1, "1"} so the test is robust against
	// whichever normalisation convention the cmp surface uses.
	if got[0] != int64(42) && got[0] != int64(1) && got[0] != "1" {
		t.Errorf("user Comparer should have fired for DepScalar-Pos: got %v", got[0])
	}
}

// Mixed operands: one reparented DepScalar, one concrete Pos. The
// LCA is still Pos, so the user Comparer fires.
func TestDepScalar_BehaveCmpMixedDepConcreteOnRefine(t *testing.T) {
	got := runOK(t, `def Pos refine Integer
behave compare/q (fn [[a:Pos b:Pos] [Integer] [42]])
def x:Pos (Integer gt 10)
def y:Pos 7
x cmp y`)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(got), got)
	}
	if got[0] != int64(42) && got[0] != int64(1) && got[0] != "1" {
		t.Errorf("user Comparer should have fired for mixed Dep/concrete Pos: got %v", got[0])
	}
}
