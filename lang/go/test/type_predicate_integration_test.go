package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Comprehensive coverage for arbitrary dependent types (predicate
// types) — user-defined types whose membership is decided by a
// predicate function. Address every gap identified in the
// predicate-type coverage audit (see thread): signature dispatch,
// the `unify` word, `make` constraint, typed list/map, disjunct
// alternatives, behave-capability composition, optional fields,
// inheritance, and DepScalar interaction.
//
// Test convention for predicate bodies:
//   def Pos fn [[n:Integer] [Boolean] [n gt 0]]
// Boolean false signals "no match" (RunPredicate honors this in
// addition to None); Boolean true is "match, candidate flows
// through unchanged".

// runPred is a focused helper that fails the test on parse/run errors
// — most predicate cases want a successful run with specific output.
func runPred(t *testing.T, src string) []any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	out, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

// runPredExpectErr expects an error and returns its message.
func runPredExpectErr(t *testing.T, src string) string {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	_, err = a.Run(src)
	if err == nil {
		t.Fatalf("expected error from %q", src)
	}
	return err.Error()
}

// =========================================================
// Tier 1: signature dispatch with a predicate-typed param
// =========================================================

func TestPredicate_SigDispatch_Accepts(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def f fn [[x:Pos] [Integer] [x add 1]]
f 5`)
	if len(got) != 1 || got[0] != int64(6) {
		t.Errorf("f 5 = %v, want [6]", got)
	}
}

func TestPredicate_SigDispatch_RejectsFailingPredicate(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def f fn [[x:Pos] [Integer] [x add 1]]
f -3`)
	if !strings.Contains(msg, "no signature matches") {
		t.Errorf("got %q, want 'no signature matches'", msg)
	}
}

func TestPredicate_SigDispatch_RejectsWrongType(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def f fn [[x:Pos] [Any] [x]]
f "hello"`)
	if !strings.Contains(msg, "no signature matches") {
		t.Errorf("got %q, want 'no signature matches'", msg)
	}
}

// =========================================================
// Tier 1: `is` with a predicate type
// =========================================================

func TestPredicate_Is_Accepts(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
5 is Pos`)
	if got[0] != "true" {
		t.Errorf("5 is Pos = %v, want true", got)
	}
}

func TestPredicate_Is_RejectsByPredicate(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
-3 is Pos`)
	if got[0] != "false" {
		t.Errorf("-3 is Pos = %v, want false (fails predicate)", got)
	}
}

func TestPredicate_Is_RejectsByInputType(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
"hello" is Pos`)
	if got[0] != "false" {
		t.Errorf("\"hello\" is Pos = %v, want false (fails input-type gate)", got)
	}
}

// =========================================================
// Tier 1: `unify` word with a predicate type
// =========================================================

func TestPredicate_UnifyWord_Accepts(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
Pos unify 5`)
	if len(got) != 2 || got[1] != "true" {
		t.Errorf("Pos unify 5 = %v, want [5, true]", got)
	}
	if got[0] != int64(5) {
		t.Errorf("unified value = %v, want 5", got[0])
	}
}

func TestPredicate_UnifyWord_RejectsPredicate(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
Pos unify -3`)
	if len(got) != 2 || got[1] != "false" {
		t.Errorf("Pos unify -3 = %v, want [~unify-fail, false]", got)
	}
}

func TestPredicate_UnifyWord_RejectsInputType(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
Pos unify "hello"`)
	if len(got) != 2 || got[1] != "false" {
		t.Errorf("Pos unify \"hello\" = %v, want [~unify-fail, false]", got)
	}
}

// Commutativity: unify is symmetric.
func TestPredicate_UnifyWord_Commutative(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
(5 unify Pos) drop`)
	if got[0] != int64(5) {
		t.Errorf("5 unify Pos = %v, want [5]", got)
	}
}

// =========================================================
// Tier 1: `make` constraint enforcement
// =========================================================

func TestPredicate_MakeRecord_Accepts(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def Rec refine Record [x:Pos]
make Rec [5]`)
	if got[0] != "{x:5}" {
		t.Errorf("got %v, want {x:5}", got)
	}
}

func TestPredicate_MakeRecord_RejectsPredicate(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def Rec refine Record [x:Pos]
make Rec [-3]`)
	if !strings.Contains(msg, "is not a member of predicate") {
		t.Errorf("got %q, want 'is not a member of predicate'", msg)
	}
}

// =========================================================
// Tier 1: typeof on predicate-typed value
// =========================================================

func TestPredicate_Typeof_ReportsPredicateType(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def x:Pos 5
typeof x`)
	if got[0] != "Pos" {
		t.Errorf("typeof x = %v, want Pos", got)
	}
}

// =========================================================
// Tier 1: typed list / typed map of predicate
// =========================================================

func TestPredicate_TypedList_AllSatisfy(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
[5,10] is [:Pos]`)
	if got[0] != "true" {
		t.Errorf("[5,10] is [:Pos] = %v, want true", got)
	}
}

func TestPredicate_TypedList_RejectsFailing(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
[5,-3] is [:Pos]`)
	if got[0] != "false" {
		t.Errorf("[5,-3] is [:Pos] = %v, want false", got)
	}
}

func TestPredicate_TypedMap_AllSatisfy(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
{a:5,b:10} is {:Pos}`)
	if got[0] != "true" {
		t.Errorf("{a:5,b:10} is {:Pos} = %v, want true", got)
	}
}

func TestPredicate_TypedMap_RejectsFailing(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
{a:5,b:-3} is {:Pos}`)
	if got[0] != "false" {
		t.Errorf("{a:5,b:-3} is {:Pos} = %v, want false", got)
	}
}

// =========================================================
// Tier 2: predicate type in a disjunct
// =========================================================

func TestPredicate_InDisjunct_AcceptsPredicateBranch(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def D (Pos tor String)
5 is D`)
	if got[0] != "true" {
		t.Errorf("5 is (Pos tor String) = %v, want true (5 satisfies Pos)", got)
	}
}

func TestPredicate_InDisjunct_AcceptsOtherBranch(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def D (Pos tor String)
"hi" is D`)
	if got[0] != "true" {
		t.Errorf("\"hi\" is (Pos tor String) = %v, want true (matches String)", got)
	}
}

func TestPredicate_InDisjunct_RejectsNeither(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def D (Pos tor String)
-3 is D`)
	if got[0] != "false" {
		t.Errorf("-3 is (Pos tor String) = %v, want false", got)
	}
}

// =========================================================
// Tier 2: predicate type as Record field constraint
// =========================================================

func TestPredicate_RecordField_AcceptsValid(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def Point refine Record [x:Pos y:Pos]
make Point [3 4]`)
	if got[0] != "{x:3 y:4}" {
		t.Errorf("got %v, want {x:3 y:4}", got)
	}
}

func TestPredicate_RecordField_RejectsOneFailing(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def Point refine Record [x:Pos y:Pos]
make Point [3 -4]`)
	if !strings.Contains(msg, "y") || !strings.Contains(msg, "is not a member of predicate") {
		t.Errorf("got %q, want failure naming field y", msg)
	}
}

// =========================================================
// Tier 2: behave compare/q on a predicate type
// =========================================================

func TestPredicate_BehaveCompare(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
behave compare/q (fn [[a:Pos b:Pos] [Integer] [42]])
def x:Pos 5
def y:Pos 10
x cmp y`)
	if len(got) != 1 {
		t.Fatalf("got %v, want 1 result", got)
	}
	// Custom Comparer returns 42, cmp normalizes to +1 or keeps 42.
	if got[0] != int64(42) && got[0] != int64(1) && got[0] != "1" {
		t.Errorf("user Comparer should fire for Pos-typed values, got %v", got[0])
	}
}

// =========================================================
// Tier 3: optional predicate fields {x?:Pos}
// =========================================================

func TestPredicate_OptionalField_AbsentOk(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
({a:1} unify {a:Integer,b?:Pos}) drop`)
	if got[0] != "{a:1}" {
		t.Errorf("got %v, want {a:1} (absent optional omitted)", got)
	}
}

func TestPredicate_OptionalField_NoneOk(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
({a:1,b:None} unify {a:Integer,b?:Pos}) drop`)
	if got[0] != "{a:1 b:None}" {
		t.Errorf("got %v, want {a:1 b:None}", got)
	}
}

// =========================================================
// Tier 3: typed-def then is checks
// =========================================================

func TestPredicate_TypedDefIsAncestor(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def x:Pos 5
x is Integer`)
	if got[0] != "true" {
		t.Errorf("x is Integer = %v, want true (Pos's base is Integer)", got)
	}
}

func TestPredicate_TypedDefIsSelf(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def x:Pos 5
x is Pos`)
	if got[0] != "true" {
		t.Errorf("x is Pos = %v, want true", got)
	}
}

// =========================================================
// Tier 3: predicate type rejects via typed-def
// =========================================================

func TestPredicate_TypedDefRejects(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def x:Pos -5`)
	if !strings.Contains(msg, "Pos") && !strings.Contains(msg, "predicate") {
		t.Errorf("got %q, want failure naming Pos/predicate", msg)
	}
}

// =========================================================
// Tier 3: predicate using `guard` (value-transforming idiom)
// =========================================================

func TestPredicate_GuardIdiom_Accepts(t *testing.T) {
	got := runPred(t, `def Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d") guard x]]
"c" is Bbd`)
	if got[0] != "true" {
		t.Errorf(`"c" is Bbd = %v, want true`, got)
	}
}

func TestPredicate_GuardIdiom_Rejects(t *testing.T) {
	got := runPred(t, `def Bbd fn [x:Any Any [(x is String) and (x gte "b") and (x lte "d") guard x]]
"z" is Bbd`)
	if got[0] != "false" {
		t.Errorf(`"z" is Bbd = %v, want false`, got)
	}
}

// =========================================================
// Tier 3: combining predicate types in a Record with optional fields
// =========================================================

func TestPredicate_CombinedRecordOptional(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def Cfg refine Record [width:Pos height:Pos]
make Cfg [10 20]`)
	if got[0] != "{width:10 height:20}" {
		t.Errorf("got %v, want {width:10 height:20}", got)
	}
}

// =========================================================
// Predicate-typed value flowing through higher-order word
// =========================================================

func TestPredicate_HigherOrderEach(t *testing.T) {
	// `each` calls the body with each element. The body invokes f
	// which has signature [x:Pos]; if any element fails the predicate
	// the whole pipeline errors. With [1,2,3] all positive, all pass.
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def f fn [[x:Pos] [Integer] [x add 1]]
[1,2,3] each [f]`)
	if got[0] != "[2 3 4]" {
		t.Errorf("got %v, want [2 3 4]", got)
	}
}

// Higher-order with a failing element: each errors at the failing
// position because f's signature can't accept -2.
func TestPredicate_HigherOrderEach_OneFails(t *testing.T) {
	msg := runPredExpectErr(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def f fn [[x:Pos] [Integer] [x add 1]]
[1,-2,3] each [f]`)
	if !strings.Contains(msg, "no signature matches") {
		t.Errorf("got %q, want 'no signature matches' for -2", msg)
	}
}

// =========================================================
// Predicate type as fn-arg with extra dispatch overloads
// =========================================================

// A fn with two overloads, one Pos-typed and one fallback. The
// dispatcher picks the more-specific Pos sig for positive Integers,
// the fallback for everything else.
func TestPredicate_SigDispatch_MultipleOverloads(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def classify fn [
  [x:Pos] [String] ["positive"]
  [x:Any] [String] ["other"]
]
classify 5
classify -3
classify "hi"`)
	want := []any{"positive", "other", "other"}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("result[%d] = %v, want %v", i, got[i], w)
		}
	}
}

// =========================================================
// Boolean-domain predicates — body return is a VALUE, not a verdict
// =========================================================

// Regression for review-flagged P1: a predicate whose input type is
// Boolean must accept `false` as a valid value (the body returns
// `b`, not a verdict). Without the input-domain check in RunPredicate,
// any predicate body returning Boolean false would be rejected.

func TestPredicate_BooleanDomainAcceptsFalse(t *testing.T) {
	got := runPred(t, `def Flag fn [[b:Boolean] [Boolean] [b]]
false is Flag`)
	if got[0] != "true" {
		t.Errorf("false is Flag = %v, want true (Boolean is the value domain)", got)
	}
}

func TestPredicate_BooleanDomainAcceptsTrue(t *testing.T) {
	got := runPred(t, `def Flag fn [[b:Boolean] [Boolean] [b]]
true is Flag`)
	if got[0] != "true" {
		t.Errorf("true is Flag = %v, want true", got)
	}
}

// Counterpart: a predicate whose input type is NOT Boolean (Integer
// here) still uses Boolean returns as verdicts — `n gt 0` returning
// false means "no match".
func TestPredicate_NonBooleanDomainTreatsFalseAsVerdict(t *testing.T) {
	got := runPred(t, `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
-3 is Pos`)
	if got[0] != "false" {
		t.Errorf("-3 is Pos = %v, want false (Boolean is verdict for Integer-domain predicate)", got)
	}
}

// Predicate whose input is Any also treats Boolean returns as values
// (Any accepts Boolean).
func TestPredicate_AnyDomainAcceptsFalse(t *testing.T) {
	got := runPred(t, `def Always fn [[x:Any] [Any] [x]]
false is Always`)
	if got[0] != "true" {
		t.Errorf("false is Always = %v, want true (Any domain accepts Boolean values)", got)
	}
}

// =========================================================
// behave unify/q — runtime output-type validation
// =========================================================

// Regression for review-flagged P2: `validateUnifySig` permits Any in
// the fn return slot (because ParseFnReturns runs without a Registry
// and degrades user-type returns to Any). The runtime check on the
// body's output type is the actual enforcement point — a body that
// returns a value of an unrelated type must be rejected.

func TestBehaveUnify_RejectsWrongOutputType(t *testing.T) {
	got := runPred(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] ["bogus-string"]])
def x:Item 3
def y:Item 4
x unify y`)
	if len(got) != 2 || got[1] != "false" {
		t.Errorf("expected unify failure for bogus String output, got %v", got)
	}
}

// Accepting body — produces a valid Item (Integer-derived) value, so
// the type check passes.
func TestBehaveUnify_AcceptsCompatibleOutput(t *testing.T) {
	got := runPred(t, `def Item refine Integer
behave unify/q (fn [[a:Item b:Item] [Any] [a b add]])
def x:Item 3
def y:Item 4
x unify y`)
	if len(got) != 2 || got[1] != "true" {
		t.Fatalf("expected success, got %v", got)
	}
	if got[0] != int64(7) {
		t.Errorf("got %v, want 7", got[0])
	}
}
