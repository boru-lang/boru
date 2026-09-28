package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// --- Name-confusion guards between `type` and `def` ---
//
// *Type names live in r.Types; def names live in DefStacks (and are
// registered as callables when the body is a fn). The two namespaces
// must NOT share a name, otherwise a single source token has two
// meanings depending on context. Both directions of the clash are
// rejected with a clear error before either binding happens.

func expectError(t *testing.T, src string, wantSubstr string) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(src)
	if err == nil {
		t.Fatalf("expected error matching %q for source:\n%s", wantSubstr, src)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error containing %q, got: %v", wantSubstr, err)
	}
}

func TestNameConfusion_TypeThenDef_CaseRule(t *testing.T) {
	// Post TYPE-UNIFORM Phase 2: `def Foo …` with a capitalised name
	// is a TYPE binding (def is the universal binder), equivalent to
	// `def Foo …`. So `def Foo` after `def Foo` shadows the type,
	// exactly as a second `def Foo …` would — no clash, no error.
	got := runOne(t, `def Foo Integer
def Foo String
"hello" is Foo`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("def Foo (capitalised) should shadow the type; got %v, want [true]", got)
	}
}

// Re-defining the same TYPE is also rejected (no shadow stack — type
// Re-defining the same TYPE is allowed — type bindings stack like
// def, and `undef Foo` reverts to the previous binding. The
// shadow-and-revert tests live in lang/go/test/type_shadow_test.go;
// here we only pin that the second `def Foo …` does NOT error.
func TestNameConfusion_TypeRedefinitionShadows(t *testing.T) {
	got := runOne(t, `def Foo Integer
def Foo String
"hello" is Foo`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("after shadow, \"hello\" is Foo = %v, want [\"true\"] (Foo now String)", got)
	}
}

// --- `is` evaluates predicate types ---
//
// `v is Bbd` runs the Bbd predicate against v. The predicate returns
// None on fail / unified value on success; `is` collapses that to
// Boolean: true iff non-None. Symmetric with the typed-def handler.

const isBbdSource = `def Bbd fnpred x:Any [if ((x is String) and (x gte "b") and (x lte "d")) [x] [None]]
`

// `is` carries BarrierPos=1 (mirroring `or`): only its first arg can
// be forward, so a value-then-`is`-then-type expression doesn't eat
// the next line's first token. Tests below rely on that — no parens
// needed.
func TestIsPredicate_True(t *testing.T) {
	got := runOne(t, isBbdSource+`"c" is Bbd`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("\"c\" is Bbd = %v, want [\"true\"]", got)
	}
}

func TestIsPredicate_FalseOutOfRange(t *testing.T) {
	got := runOne(t, isBbdSource+`"e" is Bbd`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("\"e\" is Bbd = %v, want [\"false\"]", got)
	}
}

func TestIsPredicate_FalseWrongType(t *testing.T) {
	got := runOne(t, isBbdSource+`99 is Bbd`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("99 is Bbd = %v, want [\"false\"]", got)
	}
}

func TestIsPredicate_TransformingPredicate(t *testing.T) {
	// `is` only checks the success/failure flag; the transformed value
	// is discarded by `is` (the Boolean answer is what matters).
	got := runOne(t, `def Up fnpred x:Any [if (x is String) [x upper] [None]]
"hello" is Up`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("\"hello\" is Up = %v, want [\"true\"]", got)
	}
}

// `is` over a non-predicate type still routes through Unify (existing
// behaviour) — guard that the predicate path doesn't break it.
func TestIsPredicate_DepScalarStillWorks(t *testing.T) {
	got := runOne(t, `def G10 (Integer gt 10)
15 is G10
5 is G10`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("15 is G10 = %v, want \"true\"", got[0])
	}
	if got[1] != "false" {
		t.Errorf("5 is G10 = %v, want \"false\"", got[1])
	}
}

// `is` with structural fn-shape types (FnUndef from `def Foo fn
// [[input] [output]]`). The value side is a `(quote name)` whose
// name resolves through DefStacks to a FnDef; `is` then runs the
// FnUndef↔FnDef structural matcher under the hood.

func TestIsFnShape_True(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
def double fn [[Integer] [Integer] [1 add]]
(quote double) is Mapper`)
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("(quote double) is Mapper = %v, want [\"true\"]", got)
	}
}

func TestIsFnShape_FalseWrongInputType(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
def stringy fn [[String] [Integer] [length]]
(quote stringy) is Mapper`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(quote stringy) is Mapper = %v, want [\"false\"]", got)
	}
}

func TestIsFnShape_FalseWrongReturnType(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
def stringer fn [[Integer] [String] [convert String]]
(quote stringer) is Mapper`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("(quote stringer) is Mapper = %v, want [\"false\"]", got)
	}
}

func TestIsFnShape_FalseNonFunction(t *testing.T) {
	got := runOne(t, `def Mapper fnsig [[Integer] [Integer]]
42 is Mapper`)
	if len(got) != 1 || got[0] != "false" {
		t.Errorf("42 is Mapper = %v, want [\"false\"]", got)
	}
}

// Regression for the BarrierPos: the next-line token must NOT be
// pulled into `is` as its second forward arg.
func TestIsBarrierPos_DoesNotEatNextToken(t *testing.T) {
	got := runOne(t, `def G10 (Integer gt 10)
15 is G10
42`)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 results", got)
	}
	if got[0] != "true" {
		t.Errorf("first result = %v, want \"true\" (15 is G10)", got[0])
	}
	if got[1] != int64(42) {
		t.Errorf("second result = %v, want 42 (untouched by is)", got[1])
	}
}
