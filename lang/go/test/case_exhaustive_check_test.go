package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Static exhaustiveness for `case` (design/case-exhaustiveness.0.md): a
// default-less case whose clauses do not provably cover the scrutinee's
// static type is an error-severity check finding (case_not_exhaustive),
// and the trailing default is not required when the type disjunction is
// met. These tests pin both directions for every domain shape — declared
// unions, enums, Boolean, optionals, newtypes, concrete values — plus the
// opt-outs (dynamic scrutinees, per-call-shape concrete narrowing) and
// the two advisory duals.

func checkDiag(t *testing.T, src string) lang.CheckResult {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(src)
	if err != nil {
		t.Fatalf("check %q: %v", src, err)
	}
	return res
}

// diagWithCode returns the first diagnostic with the given code, if any.
func diagWithCode(res lang.CheckResult, code string) (lang.CheckDiagnostic, bool) {
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return d, true
		}
	}
	return lang.CheckDiagnostic{}, false
}

// wantCase asserts the presence (or absence) of one case-coverage code and,
// when present, that its detail mentions every fragment.
func wantCase(t *testing.T, src, code string, want bool, fragments ...string) {
	t.Helper()
	res := checkDiag(t, src)
	d, got := diagWithCode(res, code)
	if got != want {
		t.Fatalf("%q: %s present=%v, want %v (diags: %v)", src, code, got, want, diagCodes(res))
	}
	if !got {
		return
	}
	for _, f := range fragments {
		if !strings.Contains(d.Detail, f) {
			t.Errorf("%q: %s detail %q missing %q", src, code, d.Detail, f)
		}
	}
}

func TestCaseExhaustiveDeclaredUnion(t *testing.T) {
	// A declared union scrutinee with a missing alternative is an error
	// naming the uncovered alternative…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// …full type-clause coverage needs no default…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s"]]] f 5`,
		"case_not_exhaustive", false)
	// …a trailing default always suffices…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1 "d"]]] f 5`,
		"case_not_exhaustive", false)
	// …and the union type itself in match position covers all its members.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [IS "either"]]] f 5`,
		"case_not_exhaustive", false)
	// A nested union expands through its member unions.
	wantCase(t,
		`def AB (Integer tor String) def ABC (AB tor Boolean) `+
			`def f fn [[x:ABC][Integer][case x [Integer 1 String 2 true 3 false 4]]] f 5`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveBoolean(t *testing.T) {
	// true+false decompose the opaque Boolean leaf…
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [true 1 false 0]]] f true`,
		"case_not_exhaustive", false)
	// …a missing arm is an error naming the uncovered value…
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [true 1]]] f true`,
		"case_not_exhaustive", true, "uncovered", "false")
	// …and the Boolean type literal covers both at once.
	wantCase(t,
		`def f fn [[b:Boolean][Integer][case b [Boolean 1]]] f true`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveEnum(t *testing.T) {
	// Every member listed → no default needed…
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][String][case c [red/q "r" green/q "g" blue/q "b"]]] f red/q`,
		"case_not_exhaustive", false)
	// …a missing member is an error naming it…
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][Integer][case c [red/q 1 green/q 2]]] f red/q`,
		"case_not_exhaustive", true, "uncovered", "blue")
	// …and a covering type literal (Atom) covers every member.
	wantCase(t,
		`def Color (red/q tor green/q tor blue/q) `+
			`def f fn [[c:Color][String][case c [Atom "atom"]]] f red/q`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveOptionalNone(t *testing.T) {
	wantCase(t,
		`def MaybeInt (Integer tor none) def f fn [[x:MaybeInt][Integer][case x [Integer 1 none 0]]] f 5`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def MaybeInt (Integer tor none) def f fn [[x:MaybeInt][Integer][case x [Integer 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered")
}

func TestCaseExhaustiveConcrete(t *testing.T) {
	// A top-level concrete scrutinee is value-precise: a provably-matching
	// clause list is exhaustive…
	wantCase(t, `case 2 [1 "one" 2 "two"]`, "case_not_exhaustive", false)
	// …and a provably-unmatched value is an error naming it.
	wantCase(t, `case 9 [1 "one" 2 "two"]`, "case_not_exhaustive", true, "uncovered", "9")
	// The stack-value form is checked identically.
	wantCase(t, `9 case [1 "one" 2 "two"]`, "case_not_exhaustive", true, "uncovered", "9")
	wantCase(t, `2 case [1 "one" 2 "two"]`, "case_not_exhaustive", false)
}

func TestCaseExhaustivePlainTypeParam(t *testing.T) {
	// Value clauses can never cover an infinite plain type…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [1 "a" 2 "b"]]] f 1`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …a covering type clause or a default resolves it.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Integer "i"]]] f 1`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def f fn [[x:Integer][String][case x [1 "a" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// A supertype clause covers the subtype scrutinee.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Number "n"]]] f 1`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveNewtype(t *testing.T) {
	// The newtype boundary: a base-type clause does NOT cover a
	// user-minted newtype alternative…
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Pos][String][case x [Integer "i"]]] f (def y:Pos 5 y)`,
		"case_not_exhaustive", true, "uncovered", "Pos")
	// …the newtype itself, an [is …] predicate, or Any (the written
	// catch-all) cover it…
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Pos][String][case x [Pos "p"]]] f (def y:Pos 5 y)`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Pos][String][case x [[is Pos] "p"]]] f (def y:Pos 5 y)`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Pos][String][case x [Any "a"]]] f (def y:Pos 5 y)`,
		"case_not_exhaustive", false)
	// …and in the other direction a newtype clause does not statically
	// cover its base scrutinee either.
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Integer][String][case x [Pos "p"]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
}

func TestCaseExhaustivePredicates(t *testing.T) {
	// Comparison predicates prove coverage via interval union: a total
	// pair covers the whole Integer domain…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1 [lte 3] 2]]] f 5`,
		"case_not_exhaustive", false)
	// …ℤ-adjacency bridges integer bounds ([gte 4] joins [lte 3])…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gte 4] 1 [lte 3] 2]]] f 5`,
		"case_not_exhaustive", false)
	// …a genuine gap is caught ([gt 3]/[lt 3] misses 3)…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1 [lt 3] 2]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …and an [eq …] point bridges it.
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1 [lt 3] 2 [eq 3] 3]]] f 5`,
		"case_not_exhaustive", false)
	// Float is NEVER interval-total: nan is a Float inhabitant that no
	// ordered comparison matches, so even a bound-sharing pair leaves
	// the domain uncovered.
	wantCase(t,
		`def f fn [[x:Float][Integer][case x [[gt 3.0] 1 [lt 3.0] 2]]] f 5.0`,
		"case_not_exhaustive", true, "uncovered", "Float")
	wantCase(t,
		`def f fn [[x:Float][Integer][case x [[gte 3.0] 1 [lt 3.0] 2]]] f 5.0`,
		"case_not_exhaustive", true, "uncovered", "Float")
	// A lone half-line still demands a default…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …a DepScalar refinement match contributes its bounds, so its
	// complement predicate completes the domain…
	wantCase(t,
		`def Big (Integer gt 10) def f fn [[x:Integer][String][case x [Big "big" [lte 10] "small"]]] f 5`,
		"case_not_exhaustive", false)
	// …and predicates compose with type clauses over a union.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [[gt 3] 0 Integer 1 String 2]]] f 5`,
		"case_not_exhaustive", false)
	// An unrecognized predicate shape stays opaque (no credit).
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3 1] 1 [lte 3] 2]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// A NON-NUMERIC named refinement in [is T] has no representable
	// interval and stays opaque too — the name resolves to its node,
	// the recorded String-based bounds contribute nothing.
	wantCase(t,
		`def S (String gte "a") def f fn [[x:Integer][String][case x [[is S] "yes" [gt 0] "pos"]]] f 1`,
		"case_not_exhaustive", true, "uncovered", "Integer")
}

func TestCaseExhaustiveDynamic(t *testing.T) {
	// A dynamic (untyped) scrutinee has no static type to prove coverage
	// against, so it REQUIRES a trailing default…
	wantCase(t,
		`def f fn [[x:Any][][case x [1 "one" 2 "two"]]] f 9`,
		"case_not_exhaustive", true, "gradual")
	wantCase(t,
		`def f fn [[x:Any][String][case x [1 "one" "other"]]] f 9`,
		"case_not_exhaustive", false)
	// …or an Any clause, the written catch-all.
	wantCase(t,
		`def f fn [[x:Any][String][case x [1 "one" Any "other"]]] f 9`,
		"case_not_exhaustive", false)
	// A code-body scrutinee computes its value — dynamic, same rule.
	wantCase(t, `case [1 add 1] [5 "five"]`, "case_not_exhaustive", true, "gradual")
	wantCase(t, `case [1 add 1] [2 "two" "other"]`, "case_not_exhaustive", false)
	wantCase(t, `case [1 add 1] [1 "one" Any "other"]`, "case_not_exhaustive", false)
}

func TestCaseExhaustiveEdgeShapes(t *testing.T) {
	// A lone default clause is trivially exhaustive.
	wantCase(t,
		`def f fn [[x:Integer][String][case x ["only"]]] f 5`,
		"case_not_exhaustive", false)
	// An EMPTY clause list over a typed scrutinee can match nothing.
	wantCase(t,
		`def f fn [[x:Integer][][case x []]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
}

func TestCaseExhaustiveMatchShapes(t *testing.T) {
	// An unresolvable word in match position is opaque: no coverage
	// credit (a typed default-less case still errors)…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [zzz "a"]]] f 1`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …and no false finding when a default is present.
	wantCase(t, `case 1 [zzz "a" "d"]`, "case_not_exhaustive", false)
	// A union match that covers NONE of the scrutinee's alternatives
	// leaves everything uncovered.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[b:Boolean][String][case b [IS "no"]]] f true`,
		"case_not_exhaustive", true, "uncovered")
	// A declared-union fn RETURN feeding case directly is a checked domain.
	wantCase(t,
		`def IS (Integer tor String) def g fn [[][IS][5]] def f fn [[][String][case (g) [Integer "i"]]] f`,
		"case_not_exhaustive", true, "uncovered", "String")
	wantCase(t,
		`def IS (Integer tor String) def g fn [[][IS][5]] def f fn [[][String][case (g) [Integer "i" String "s"]]] f`,
		"case_not_exhaustive", false)
	// A bare type node used as the scrutinee VALUE is not a checkable
	// domain — skipped, never a finding.
	wantCase(t, `case Integer [Integer "i"]`, "case_not_exhaustive", false)
	// A concrete none scrutinee normalizes onto the None node and is
	// covered by a `none` clause.
	wantCase(t, `case none [none 0]`, "case_not_exhaustive", false)
}

func TestCaseUnreachableClauseAdvisory(t *testing.T) {
	// A clause fully covered by an earlier clause is dead…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [Number 1 Integer 2 3]]] f 1`,
		"case_unreachable_clause", true, "Integer")
	// …including a value clause behind its covering type clause…
	wantCase(t,
		`def f fn [[x:Integer][String][case x [Integer "i" 5 "five" "d"]]] f 1`,
		"case_unreachable_clause", true)
	// …and an exact duplicate value clause.
	wantCase(t, `case 1 [1 "a" 1 "b" "d"]`, "case_unreachable_clause", true)
	// Distinct live clauses are never flagged.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s"]]] f 5`,
		"case_unreachable_clause", false)
	wantCase(t, `case 2 [1 "a" 2 "b" "d"]`, "case_unreachable_clause", false)
}

func TestCaseRedundantDefaultAdvisory(t *testing.T) {
	// Full coverage of a DECLARED union makes the trailing default dead…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s" "d"]]] f 5`,
		"case_redundant_default", true)
	// …but partial coverage keeps it live…
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" "d"]]] f 5`,
		"case_redundant_default", false)
	// …and a non-declared domain (a concrete scrutinee) never draws the
	// advisory, even when provably covered: only the author-declared
	// union domain is stable across call shapes.
	wantCase(t, `case 2 [2 "two" "d"]`, "case_redundant_default", false)
}

// errorCodes returns the codes of the error-severity diagnostics only.
func errorCodes(res lang.CheckResult) []string {
	var out []string
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			out = append(out, d.Code)
		}
	}
	return out
}

// TestCaseExhaustiveShorthandFnForm pins the SHORTHAND spelling
// (`fn x:IS Out [body]`) against the bracket spelling every other test in
// this file uses. The two forms parse to the same annotation — the map
// `{x: word(IS)}` either way — so any divergence is post-parse, and there
// was one: `fn`'s triple sig carried NoEvalArgs (list-only) but not
// NoEvalMapArgs, so the shorthand's map slot was auto-evaluated and the
// type NAME was replaced by its BODY. A union/enum body is a payload-
// carrying Disjunct value rather than a lattice node, so the declared
// type was erased before ParseFnParams ran and the scrutinee arrived
// here as dynamic(Any) — the case checker was being fed a lie.
//
// The bracket form was immune only because its slot is a List, which
// NoEvalArgs does suppress. Every row below is asserted in BOTH forms:
// the point is equivalence, so a future change that fixes one spelling
// and not the other fails here.
func TestCaseExhaustiveShorthandFnForm(t *testing.T) {
	both := func(shorthand, bracket, code string, want bool, fragments ...string) {
		t.Helper()
		wantCase(t, shorthand, code, want, fragments...)
		wantCase(t, bracket, code, want, fragments...)
	}

	// A declared union proves exhaustive in both forms…
	both(`def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s"]] f 5`,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s"]]] f 5`,
		"case_not_exhaustive", false)
	// …and a MISSING alternative errors with the same precise message in
	// both. Naming the uncovered alternative is the load-bearing half: the
	// erased-type bug still produced an error here, but the WRONG one
	// ("the scrutinee is dynamic"), which denies the annotation the user
	// wrote and sends them to add a default they do not need.
	both(`def IS (Integer tor String) def f fn x:IS Integer [case x [Integer 1]] f 5`,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// An ENUM-shaped union behaves identically.
	both(`def Color (red/q tor green/q tor blue/q) def f fn c:Color String [case c [red/q "r" green/q "g" blue/q "b"]] f red/q`,
		`def Color (red/q tor green/q tor blue/q) def f fn [[c:Color][String][case c [red/q "r" green/q "g" blue/q "b"]]] f red/q`,
		"case_not_exhaustive", false)
	both(`def Color (red/q tor green/q tor blue/q) def f fn c:Color Integer [case c [red/q 1 green/q 2]] f red/q`,
		`def Color (red/q tor green/q tor blue/q) def f fn [[c:Color][Integer][case c [red/q 1 green/q 2]]] f red/q`,
		"case_not_exhaustive", true, "uncovered", "blue")
	// The redundant-default advisory survives the shorthand: it is emitted
	// only for an author-DECLARED union domain, so erasing the declaration
	// silently dropped it (0 emitted, no error anywhere — the quietest of
	// the consequences).
	both(`def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s" "d"]] f 5`,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s" "d"]]] f 5`,
		"case_redundant_default", true)
	// Partial coverage keeps the default live in both forms — the negative
	// that makes the row above mean something.
	both(`def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" "d"]] f 5`,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" "d"]]] f 5`,
		"case_redundant_default", false)
	// A genuinely dynamic shorthand param is still dynamic: the fix must
	// not manufacture a static type where none was declared.
	wantCase(t, `def f fn x:Any String [case x [1 "one" 2 "two"]] f 9`,
		"case_not_exhaustive", true, "gradual")
}

// TestShorthandFnLambdaFormUnionParam is the `=>` twin of the test above.
// `afn`'s input sig sits at slot 1 (its canonical call is the swap
// `input afn body`), so it needed the same map-eval suppression `fn`'s
// slot 0 does.
func TestShorthandFnLambdaFormUnionParam(t *testing.T) {
	wantCase(t,
		`def IS (Integer tor String) def f (x:IS => [case x [Integer "i" String "s"]]) f 5`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def IS (Integer tor String) def f (x:IS => [case x [Integer 1]]) f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// An unannotated lambda param stays dynamic.
	wantCase(t, `def f (x:Any => [case x [1 "one"]]) f 9`,
		"case_not_exhaustive", true, "gradual")
}

// TestShorthandFnPreservesArity is the sharpest consequence of the erased
// annotation and the reason this defect outranks the case-coverage
// symptom it was reported as. When the union body reached ParseFnParams
// as a VALUE it hit the None-stripping branch meant for INLINE disjuncts,
// which synthesised a 0-arg overload: a declared one-parameter function
// became callable with no arguments, silently, returning its body.
func TestShorthandFnPreservesArity(t *testing.T) {
	src := `def IN (Integer tor None) def f fn x:IN Integer [0] f`
	res := checkDiag(t, src)
	if _, ok := diagWithCode(res, "no_signature"); !ok {
		t.Fatalf("calling a 1-param shorthand fn with no argument must not "+
			"dispatch — want no_signature, got: %v", diagCodes(res))
	}
	// The bracket form has always rejected it; equivalence is the contract.
	res = checkDiag(t, `def IN (Integer tor None) def f fn [[x:IN][Integer][0]] f`)
	if _, ok := diagWithCode(res, "no_signature"); !ok {
		t.Fatalf("bracket form: want no_signature, got: %v", diagCodes(res))
	}
	// Passing the argument still works in both forms.
	for _, ok := range []string{
		`def IN (Integer tor None) def f fn x:IN Integer [0] f 5`,
		`def IN (Integer tor None) def f fn [[x:IN][Integer][0]] f 5`,
	} {
		if res := checkDiag(t, ok); len(errorCodes(res)) != 0 {
			t.Errorf("%q: want clean, got %v", ok, diagCodes(res))
		}
	}
}

// TestShorthandFnUnionReturnType pins the OUTPUT slot. A Disjunct in the
// return position was read as a concrete return-by-value, which spliced
// the type onto the body stack and produced a bogus arity complaint —
// `IsSigTypeValue` recognised a type NAME (the Word) but not the
// evaluated union VALUE, so suppressing the map eval alone was not
// enough.
func TestShorthandFnUnionReturnType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out, err := runReference(t, a, `def IS (Integer tor String) def f fn x:Integer IS [x] 1 f`)
	if err != nil {
		t.Fatalf("union return type in shorthand fn: %v", err)
	}
	if len(out) != 1 || fmt.Sprint(out[0]) != "1" {
		t.Fatalf("want [1], got %v", out)
	}
	// BOTH forms must now REJECT a Boolean body under a declared union
	// return. The shorthand used to accept it silently: the output slot's
	// bare Word is resolved by forward collection (no NoEval flag gates
	// that), so ResolveSigType saw a Disjunct VALUE and answered
	// `(TAny, &pattern)` — and `Any` accepts everything. The pattern had
	// nowhere to live, because FnSig.Returns is []*Type.
	//
	// FnSig.ReturnPatterns is that missing channel, and the enforcement
	// runs BEFORE the `exp == TAny` skip, because a declared union
	// degrades its type to Any precisely when the pattern is the only
	// contract left.
	//
	// Asserted on ALL THREE engines, and that is the point rather than
	// thoroughness for its own sake. This test was written check-only, and
	// check-only is exactly what let the fix ship half-done: the pattern was
	// threaded onto ReturnCheckInfo (interpreter) and into
	// checkBodyReturnConformance (check pass) but NOT onto CompiledFn, so
	// `boru check` and `RunInterp` rejected the Boolean body while the
	// DEFAULT compiled run accepted it and returned `true`. A declared union
	// return was a comment on the one path most programs take. Whenever a
	// contract is enforced in more than one engine, assert it in every one —
	// a green check-mode test is not evidence about the VM.
	for _, form := range []struct{ name, src string }{
		{"bracket", `def IS (Integer tor String) def f fn [[x:Integer][IS][true]] 1 f`},
		{"shorthand", `def IS (Integer tor String) def f fn x:Integer IS [true] 1 f`},
	} {
		got := checkDiag(t, form.src)
		if len(errorCodes(got)) == 0 {
			t.Errorf("%s form must reject a Boolean body under a declared union "+
				"return, got clean: %v", form.name, diagCodes(got))
		}
		for _, mode := range []string{"compiled", "interpreted"} {
			e, _ := lang.New()
			var err error
			if mode == "compiled" {
				_, err = e.Run(form.src)
			} else {
				_, err = e.RunInterp(form.src)
			}
			if err == nil {
				t.Errorf("%s form, %s: a Boolean body under a declared union return "+
					"must raise; it was ACCEPTED", form.name, mode)
			} else if !strings.Contains(err.Error(), "return value 1") {
				t.Errorf("%s form, %s: want a return-value type_error, got %v",
					form.name, mode, err)
			}
		}
	}
	// The negative half, and the one that keeps the rule honest: a body
	// returning EITHER alternative of the union must still be accepted. A
	// pattern check that rejected everything would pass the two assertions
	// above and be useless.
	for _, ok := range []struct{ name, src string }{
		{"Integer alternative", `def IS (Integer tor String) def f fn x:Integer IS [x] 1 f`},
		{"String alternative", `def IS (Integer tor String) def f fn x:Integer IS ["s"] 1 f`},
	} {
		got := checkDiag(t, ok.src)
		if codes := errorCodes(got); len(codes) != 0 {
			t.Errorf("%s: a body returning a valid member of the declared union "+
				"must be accepted, got %v", ok.name, codes)
		}
		for _, mode := range []string{"compiled", "interpreted"} {
			e, _ := lang.New()
			var err error
			if mode == "compiled" {
				_, err = e.Run(ok.src)
			} else {
				_, err = e.RunInterp(ok.src)
			}
			if err != nil {
				t.Errorf("%s, %s: a valid member of the declared union must run: %v",
					ok.name, mode, err)
			}
		}
	}
}

// TestListOutputSigStructuralReturn covers the OTHER route into
// FnSig.ReturnPatterns: a LIST output signature (`fn [[…] [[:Integer]] […]]`)
// whose element resolves to a structural pattern. The union cases above all
// take ParseFnReturns' single-value branch; this one takes its per-element
// loop, and the two allocate the pattern slice differently.
//
// It is also the direction that makes the feature coherent. A typed container
// is already enforced on the PARAM side — `f ["a"]` against `x:[:Integer]` is
// `signature_error` — and before ReturnPatterns the same declaration in the
// RETURN slot admitted anything list-shaped, because ParseFnReturns kept only
// the `*Type` (TList) and dropped the element constraint. Params and returns
// are the same contract read in two directions; they must refuse the same
// values.
func TestListOutputSigStructuralReturn(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		bad  bool
	}{
		{"typed list, wrong element", `def f fn [[x:Integer] [[:Integer]] [["a"]]] 1 f`, true},
		{"typed list, right element", `def f fn [[x:Integer] [[:Integer]] [[1 2]]] 1 f`, false},
		{"typed map, wrong value", `def f fn [[x:Integer] [{:String}] [{a:1}]] 1 f`, true},
		{"typed map, right value", `def f fn [[x:Integer] [{:String}] [{a:"z"}]] 1 f`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, mode := range []string{"compiled", "interpreted"} {
				e, _ := lang.New()
				var err error
				if mode == "compiled" {
					_, err = e.Run(c.src)
				} else {
					_, err = e.RunInterp(c.src)
				}
				switch {
				case c.bad && err == nil:
					t.Errorf("%s: a declared structural return must refuse a "+
						"non-conforming body; it was ACCEPTED", mode)
				case c.bad && !strings.Contains(err.Error(), "return value 1"):
					t.Errorf("%s: want a return-value type_error, got %v", mode, err)
				case !c.bad && err != nil:
					t.Errorf("%s: a conforming body must run: %v", mode, err)
				}
			}
		})
	}
}

// TestCaseExhaustiveInlineUnionParam covers the OTHER route into the
// erased-union defect, which the NoEvalMapArgs fix does not touch: an
// INLINE union annotation `x:(Integer tor String)`. ParseFnParams evaluates
// a paren annotation itself, so ResolveSigType gets a Disjunct value with no
// minted lattice node to name and answers (TAny, &pattern) — and
// paramBodyCarrier read only p.Type, binding the body a dynamic(Any).
//
// Both spellings denote the same domain, so both must analyse the same. This
// failed in the BRACKET form too, which is what marks it a separate route
// rather than another symptom of the shorthand bug.
func TestCaseExhaustiveInlineUnionParam(t *testing.T) {
	for _, form := range []struct{ name, src string }{
		{"shorthand", `def f fn x:(Integer tor String) String [case x [Integer "i" String "s"]] f 5`},
		{"bracket", `def f fn [[x:(Integer tor String)][String][case x [Integer "i" String "s"]]] f 5`},
	} {
		wantCase(t, form.src, "case_not_exhaustive", false)
	}
	// A missing alternative names it, exactly as the NAMED union does — the
	// inline form is not merely accepted, it is analysed.
	wantCase(t,
		`def f fn x:(Integer tor String) String [case x [Integer "i"]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	wantCase(t,
		`def f fn [[x:(Integer tor String)][String][case x [Integer "i"]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// The redundant-default advisory follows the domain, so it fires for the
	// inline union too.
	wantCase(t,
		`def f fn x:(Integer tor String) String [case x [Integer "i" String "s" "d"]] f 5`,
		"case_redundant_default", true)
	// NEGATIVE: widening the annotation must NOT manufacture a domain.
	wantCase(t, `def f fn x:Any String [case x [Integer "i" String "s"]] f 5`,
		"case_not_exhaustive", true, "gradual")
	// NEGATIVE: dispatch still enforces the inline union at the call.
	res := checkDiag(t, `def f fn x:(Integer tor String) Integer [0] f true`)
	if _, ok := diagWithCode(res, "no_signature"); !ok {
		t.Errorf("an inline union param must still reject a Boolean argument, "+
			"got: %v", diagCodes(res))
	}
}

// TestCaseDynamicDiagnosticDoesNotDenyTheAnnotation pins the wording fix.
// `paramBodyCarrier` marks a typed-CONTAINER param (`{:Integer}` / `[:Integer]`)
// dynamic — correctly, the element type does not fix the container value's
// type here — so the diagnostic is reachable for a parameter the author DID
// annotate. Saying "the scrutinee is dynamic" reads as "you did not type
// this" and sends them to re-annotate something already annotated; the text
// now describes the POSITION ("gradual here") instead of denying the
// declaration.
func TestCaseDynamicDiagnosticDoesNotDenyTheAnnotation(t *testing.T) {
	for _, src := range []string{
		`def f fn m:{:Integer} String [case m [Map "m"]] f {a:1}`,
		`def f fn l:[:Integer] String [case l [List "l"]] f [1]`,
		`def f fn x:Any String [case x [1 "one"]] f 9`,
	} {
		res := checkDiag(t, src)
		d, ok := diagWithCode(res, "case_not_exhaustive")
		if !ok {
			t.Fatalf("%q: want case_not_exhaustive, got %v", src, diagCodes(res))
		}
		if strings.Contains(d.Detail, "the scrutinee is dynamic") {
			t.Errorf("%q: the diagnostic must not assert the scrutinee is "+
				"untyped — an annotation may be present and still be gradual "+
				"at this position: %q", src, d.Detail)
		}
		if !strings.Contains(d.Detail, "gradual") {
			t.Errorf("%q: detail %q should say the position is gradual", src, d.Detail)
		}
	}
}

// TestDescribeCaseShorthandExamplesAreTrue executes the two hand-authored
// `describe case` examples that shipped FALSE claims. TestHelpExamplesCorrect
// skips hand-authored examples by construction (they are prose, not the
// `;# <exact-stack>` shape its matcher validates), which is exactly why these
// two could be wrong for as long as they were. Nothing here generalises that
// gap away — it pins these two, because these two are documented promises
// about the feature this file tests.
func TestDescribeCaseShorthandExamplesAreTrue(t *testing.T) {
	// "exhaustive over IS — no default needed"
	wantCase(t,
		`def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s"]]`,
		"case_not_exhaustive", false)
	// "check ERROR case_not_exhaustive — uncovered: String"
	wantCase(t,
		`def IS (Integer tor String) def f fn x:IS Integer [case x [Integer 1]]`,
		"case_not_exhaustive", true, "uncovered", "String")
	// The four neighbours in the same block, so a future edit to any of them
	// is checked rather than merely plausible.
	wantCase(t, `def f fn b:Boolean Integer [case b [true 1 false 0]]`,
		"case_not_exhaustive", false)
	wantCase(t, `def f fn x:Integer Integer [case x [[gt 3] 1 [lte 3] 2]]`,
		"case_not_exhaustive", false)
	wantCase(t, `def Pos refine Integer def f fn x:Pos String [case x [Pos "p"]]`,
		"case_not_exhaustive", false)
	wantCase(t, `def f fn x:Any String [case x [1 "one" "other"]]`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustiveSeverities(t *testing.T) {
	res := checkDiag(t, `case 9 [1 "one" 2 "two"]`)
	d, ok := diagWithCode(res, "case_not_exhaustive")
	if !ok {
		t.Fatalf("case_not_exhaustive missing: %v", diagCodes(res))
	}
	if d.Severity != lang.SeverityError {
		t.Errorf("case_not_exhaustive severity = %s, want error", d.Severity)
	}
	if d.RuntimeMirror {
		t.Errorf("case_not_exhaustive must NOT be a RuntimeMirror — no-match is not a runtime error, and the compile pipeline must refuse on it")
	}
	if d.Word != "case" {
		t.Errorf("case_not_exhaustive word = %q, want case", d.Word)
	}

	res = checkDiag(t, `def f fn [[x:Integer][Integer][case x [Number 1 Integer 2 3]]] f 1`)
	if d, ok := diagWithCode(res, "case_unreachable_clause"); !ok || d.Severity != lang.SeverityInfo {
		t.Errorf("case_unreachable_clause must be info severity, got %+v (ok=%v)", d, ok)
	}
	res = checkDiag(t,
		`def IS (Integer tor String) def f fn [[x:IS][String][case x [Integer "i" String "s" "d"]]] f 5`)
	if d, ok := diagWithCode(res, "case_redundant_default"); !ok || d.Severity != lang.SeverityInfo {
		t.Errorf("case_redundant_default must be info severity, got %+v (ok=%v)", d, ok)
	}
}

func TestCaseExhaustiveDedupeAcrossCallShapes(t *testing.T) {
	// The fn body is analysed at construction and once per call shape;
	// the finding must appear exactly once per case site.
	res := checkDiag(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [Integer 1]]] f 5 f "s"`)
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == "case_not_exhaustive" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly 1 case_not_exhaustive across call shapes, got %d: %v", n, diagCodes(res))
	}
}

// TestCaseRuntimeNoMatchProducesNothing pins the ENGINE-level no-match
// semantics now that no check-clean program can express it (the
// dynamic-scrutinee rule requires a default): an unchecked run of a
// default-less case that matches nothing produces NO value, like `if`
// without an else. The library Run path does not gate on the checker,
// so the runtime contract stays pinned here.
func TestCaseRuntimeNoMatchProducesNothing(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	out, err := runReference(t, a, `def f fn [[x:Any][][case x [1 "one" 2 "two"]]] f 9`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("no-match/no-default case must produce nothing, got %v", out)
	}
}

func TestCaseExhaustiveRunRefusal(t *testing.T) {
	// `boru check` gating is severity-based: the finding is an error, so a
	// non-exhaustive program FAILS check while the covered twin passes.
	res := checkDiag(t, `case 9 [1 "one" 2 "two"]`)
	if res.Summary.Errors == 0 {
		t.Fatalf("non-exhaustive case must produce an error-severity finding: %+v", res.Summary)
	}
	res = checkDiag(t, `case 9 [1 "one" 2 "two" "many"]`)
	if res.Summary.Errors != 0 {
		t.Fatalf("defaulted case must check clean, got %d errors: %v", res.Summary.Errors, diagCodes(res))
	}
}

func TestCaseExhaustiveIntervalMachinery(t *testing.T) {
	// [is Big] over a DepScalar refinement contributes its bounds.
	wantCase(t,
		`def Big (Integer gt 10) def f fn [[x:Integer][String][case x [[is Big] "b" [lte 10] "s"]]] f 5`,
		"case_not_exhaustive", false)
	// Half-lines merge through an unbounded middle span.
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[lte 3] 1 [gte 0] 2 [gte 10] 3]]] f 1`,
		"case_not_exhaustive", false)
	// A Float scrutinee is never interval-total (nan matches no
	// comparison) — even a full-looking span errs…
	wantCase(t,
		`def f fn [[x:Float][Integer][case x [[lt 3.0] 1 [lte 3.0] 2 [gt 3.0] 3]]] f 1.0`,
		"case_not_exhaustive", true, "uncovered", "Float")
	wantCase(t,
		`def f fn [[x:Float][Integer][case x [[gt 3.0] 1]]] f 5.0`,
		"case_not_exhaustive", true)
	// …but a concrete non-nan float IS point-checked against intervals…
	wantCase(t, `case 5.0 [[gt 3.0] "big"]`, "case_not_exhaustive", false)
	wantCase(t, `case inf [[gt 0.0] "up"]`, "case_not_exhaustive", false)
	// …while nan is admitted by no interval (P1 review finding).
	wantCase(t, `case nan [[lte 0.0] "x"]`, "case_not_exhaustive", true, "uncovered")
	wantCase(t, `case nan [[lte 0.0] "x" "d"]`, "case_not_exhaustive", false)
	// An integer-empty point predicate ([eq 3.5] admits no integer) is
	// simply no coverage — the default keeps the case legal.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[eq 3.5] "x" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// Intervals admit a CONCRETE scrutinee's exact value…
	wantCase(t, `case 5 [[gt 3] "big" [lte 3] "small"]`, "case_not_exhaustive", false)
	// …and boundary exclusion is exact: [gt 3]/[lt 3] misses 3 itself.
	wantCase(t, `case 3 [[gt 3] "a" [lt 3] "b"]`, "case_not_exhaustive", true, "uncovered", "3")
	// Intervals prove nothing about a non-numeric union alternative.
	wantCase(t,
		`def IS (Integer tor String) def f fn [[x:IS][Integer][case x [[gt 3] 1 [lte 3] 2]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "String")
	// Integer bounds are EXACT int64 — float64 would collapse values
	// above 2^53 and wrongly accept this (P2 review finding)…
	wantCase(t,
		`case 9007199254740992 [[lte 9007199254740991] "lo" [gte 9007199254740993] "hi"]`,
		"case_not_exhaustive", true, "uncovered", "9007199254740992")
	// …while exact adjacency at the same magnitude still proves total…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[lte 9007199254740991] 1 [gte 9007199254740992] 2]]] f 5`,
		"case_not_exhaustive", false)
	// …and a single interval reaching MinInt64 covers the whole int64 domain.
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gte -9223372036854775808] 1]]] f 5`,
		"case_not_exhaustive", false)
	// A DepScalar interval applies only within its BASE family: Big
	// (Integer gt 10) admits no Float at runtime (P2 review finding).
	wantCase(t,
		`def Big (Integer gt 10) def f fn [[x:Float][String][case x [Big "big" [lte 10.0] "small"]]] f 11.0`,
		"case_not_exhaustive", true, "uncovered", "Float")
	// A hi-bounded refinement contributes its Hi bound.
	wantCase(t,
		`def Small (Integer lte 10) def f fn [[x:Integer][String][case x [Small "s" [gt 10] "b"]]] f 5`,
		"case_not_exhaustive", false)
	// A String-family refinement match carries no interval (it still
	// unify-covers concrete values; the default keeps this legal).
	wantCase(t,
		`def S (String gt "m") def f fn [[x:String][String][case x [S "hi" "lo"]]] f "z"`,
		"case_not_exhaustive", false)
}

func TestCaseExhaustivePredicateShapes(t *testing.T) {
	// [is Boolean] covers the decomposed true/false alternatives.
	wantCase(t,
		`def f fn [[b:Boolean][String][case b [[is Boolean] "b"]]] f true`,
		"case_not_exhaustive", false)
	// An [is …] with an unresolvable target is opaque (default keeps it legal).
	wantCase(t, `case 1 [[is zzz] "a" "d"]`, "case_not_exhaustive", false)
	// A 2-element list that opens with a non-word is not a predicate shape.
	wantCase(t, `case 1 [[3 5] "a" "d"]`, "case_not_exhaustive", false)
	// An unrecognized comparison op ([neq …]) earns no interval.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[neq 3] "a" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// A non-literal predicate bound is not recognized.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[gt foo] "a" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// A generic-instantiation paren match evaluates at runtime — opaque.
	wantCase(t,
		`def Box gen [T] class {value:T} def b (make (Box of [Integer]) {value:7}) end case b [(Box of [Integer]) "int-box" "other"]`,
		"case_not_exhaustive", false)
	// A code-body scrutinee with a non-list clause arg is the runtime's
	// case_error territory, not a coverage finding.
	wantCase(t, `case [1 add 1] Integer`, "case_not_exhaustive", false)
	// An empty even-length clause list over a computed scrutinee still
	// demands the catch-all…
	wantCase(t, `case [1 add 1] []`, "case_not_exhaustive", true, "gradual")
	// …and predicate clauses alone cannot satisfy a dynamic scrutinee.
	wantCase(t,
		`def f fn [[x:Any][String][case x [[gt 3] "hi" 1 "one"]]] f 9`,
		"case_not_exhaustive", true, "gradual")
}

func TestCaseUnreachableIntervalAdvisories(t *testing.T) {
	// Interval containment: [gt 5] after [gt 3] is dead…
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3] 1 [gt 5] 2 3]]] f 1`,
		"case_unreachable_clause", true)
	// …but a merely-overlapping pair is live.
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[lt 5] 1 [gt 3] 2 3]]] f 1`,
		"case_unreachable_clause", false)
	// Float bounds over an INTEGER scrutinee floor/ceil exactly, in
	// both inclusivity directions.
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[lt 3.5] 1 [gte 3.5] 2]]] f 1`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def f fn [[x:Integer][Integer][case x [[gt 3.5] 1 [lte 3.5] 2]]] f 1`,
		"case_not_exhaustive", false)
	// Saturation at the int64 rim: strict bounds beyond the domain edge
	// admit no Integer, and out-of-range float bounds empty out — the
	// defaults keep these legal, the checker just earns no coverage.
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[gt 9223372036854775807] "x" [lte 9223372036854775806] "y"]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[lt -9223372036854775808] "x" "d"]]] f 1`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[gte 10000000000000000000.0] "x" "d"]]] f 1`,
		"case_not_exhaustive", false)
	wantCase(t,
		`def f fn [[x:Integer][String][case x [[lte -10000000000000000000.0] "x" "d"]]] f 1`,
		"case_not_exhaustive", false)
	// A float point below the interval's low bound is not admitted.
	wantCase(t, `case 2.0 [[gt 3.0] "big" "d"]`, "case_not_exhaustive", false)
	// The base-family filter applies on both interval passes: a
	// Float-based refinement earns nothing toward an Integer domain…
	wantCase(t,
		`def BigF (Float gt 10.0) def f fn [[x:Integer][String][case x [BigF "f" [lte 10] "lo"]]] f 5`,
		"case_not_exhaustive", true, "uncovered", "Integer")
	// …and an Integer-based refinement never admits a concrete float.
	wantCase(t,
		`def Big (Integer gt 10) case 11.0 [Big "big" "d"]`,
		"case_not_exhaustive", false)
	// [is T] containment: [is Pos] after [is Integer] is dead.
	wantCase(t,
		`def Pos refine Integer def f fn [[x:Integer][Integer][case x [[is Integer] 1 [is Pos] 2 3]]] f 1`,
		"case_unreachable_clause", true)
}
