package core

// Standalone-suite coverage for guard_narrow.go — the flow-sensitive
// half of branch analysis, moved down from the check piece at the
// 2026-08-08 ADR-013 amendment. The behaviour under test is what makes
// `if (x is T) [then] [else]` mean something different about x in each
// arm.

import "testing"

// guardCond builds an `if` condition body: a list of raw tokens, which
// is the shape both `if [x is T]` and the paren form reduce to.
func guardCond(toks ...Value) Value { return NewList(toks) }

func TestBoolWord(t *testing.T) {
	if BoolWord(true) != "true" || BoolWord(false) != "false" {
		t.Error("BoolWord must render the source literals")
	}
}

func TestLiteralCondValue(t *testing.T) {
	// Not a list / no payload at all.
	if _, ok := LiteralCondValue(Value{}); ok {
		t.Error("a payload-less value is not a decided condition")
	}
	if _, ok := LiteralCondValue(NewInteger(1)); ok {
		t.Error("a non-list condition is not decided")
	}
	// A list of the wrong length is not a single literal.
	if _, ok := LiteralCondValue(guardCond(NewWord("true"), NewWord("false"))); ok {
		t.Error("a two-token condition is not a single literal")
	}
	if _, ok := LiteralCondValue(NewList(nil)); ok {
		t.Error("an empty condition is not decided")
	}

	// Bare true/false WORDS — in check mode the words stay unresolved
	// until the branch runs, so this is the common shape.
	if v, ok := LiteralCondValue(guardCond(NewWord("true"))); !ok || !v {
		t.Errorf("[true] = %v/%v, want true", v, ok)
	}
	if v, ok := LiteralCondValue(guardCond(NewWord("false"))); !ok || v {
		t.Errorf("[false] = %v/%v, want false", v, ok)
	}
	// Some other word is not a literal.
	if _, ok := LiteralCondValue(guardCond(NewWord("maybe"))); ok {
		t.Error("an arbitrary word is not a decided condition")
	}

	// A concrete Boolean value (the post-runtime path).
	if v, ok := LiteralCondValue(guardCond(NewBoolean(true))); !ok || !v {
		t.Errorf("[true-value] = %v/%v, want true", v, ok)
	}

	// A pre-evaluated paren cond arrives wrapped in a GuardFactInfo
	// carrier; the decided Boolean must be read straight through the
	// wrapper or the unreachable-branch analysis goes silent.
	wrapped := NewValueRaw(TBoolean, GuardFactInfo{Prev: BoolPayload{B: true}})
	wrapped.Carrier = true
	if v, ok := LiteralCondValue(guardCond(wrapped)); !ok || !v {
		t.Errorf("wrapped cond = %v/%v, want true", v, ok)
	}
	// A wrapper with no decided Prev falls through to the other arms.
	undecided := NewValueRaw(TBoolean, GuardFactInfo{Toks: []Value{NewWord("x")}})
	undecided.Carrier = true
	if _, ok := LiteralCondValue(guardCond(undecided)); ok {
		t.Error("an undecided wrapped cond must not report a literal")
	}
}

func TestExtractGuardClausesDeclines(t *testing.T) {
	r := w8reg(t)
	isW := NewWord("is")

	// Nil registry / payload-less condition.
	if got := extractGuardClauses(nil, guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger))); got != nil {
		t.Error("a nil registry must decline")
	}
	if got := extractGuardClauses(r, Value{}); got != nil {
		t.Error("a payload-less condition must decline")
	}
	// Too short to hold a triplet.
	if got := extractGuardClauses(r, guardCond(NewWord("x"))); got != nil {
		t.Error("a one-token condition must decline")
	}

	// A COMPOUND boolean condition licenses no per-clause narrowing:
	// under a disjunction either clause could be the true one, and a
	// negation inverts the arm. Bail entirely.
	for _, op := range []string{"or", "nor", "xor", "xnor", "not"} {
		cond := guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger), NewWord(op))
		if got := extractGuardClauses(r, cond); got != nil {
			t.Errorf("compound %q must decline, got %+v", op, got)
		}
	}

	// A middle token that is not `is`, and a non-word head, are not guards.
	if got := extractGuardClauses(r, guardCond(NewWord("x"), NewWord("eq"), NewTypeLiteral(TInteger))); got != nil {
		t.Errorf("`x eq T` is not a guard, got %+v", got)
	}
	if got := extractGuardClauses(r, guardCond(NewInteger(1), isW, NewTypeLiteral(TInteger))); got != nil {
		t.Errorf("a non-word subject is not a guard, got %+v", got)
	}
	// A payload-bearing non-type target is not a guard.
	if got := extractGuardClauses(r, guardCond(NewWord("x"), isW, NewInteger(3))); got != nil {
		t.Errorf("`x is 3` is not a type guard, got %+v", got)
	}
}

func TestExtractGuardClausesPositive(t *testing.T) {
	r := w8reg(t)
	isW := NewWord("is")

	// The plain triplet over a bare type literal.
	got := extractGuardClauses(r, guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger)))
	if len(got) != 1 || got[0].Name != "x" || !got[0].Type.Equal(TInteger) {
		t.Fatalf("clauses = %+v, want one x:Integer clause", got)
	}
	if got[0].ThenOnly {
		t.Error("a direct `x is T` clause narrows both arms")
	}

	// A GuardFactInfo carrier condition extracts from the preserved
	// tokens exactly as from a list body (the canonical paren form).
	wrapped := NewValueRaw(TBoolean, GuardFactInfo{
		Toks: []Value{NewWord("y"), isW, NewTypeLiteral(TString)},
	})
	wrapped.Carrier = true
	got = extractGuardClauses(r, wrapped)
	if len(got) != 1 || got[0].Name != "y" || !got[0].Type.Equal(TString) {
		t.Fatalf("paren-form clauses = %+v, want one y:String clause", got)
	}

	// A type-WORD target resolves through the def table.
	probe := MintTestType("Ideal/GuardProbeType")
	r.Defs.Push("GuardProbeType", NewTypeLiteral(probe))
	got = extractGuardClauses(r, guardCond(NewWord("z"), isW, NewWord("GuardProbeType")))
	if len(got) != 1 || !got[0].Type.Equal(probe) {
		t.Fatalf("word-target clauses = %+v, want the minted type", got)
	}

	// A BUILTIN type name arrives as a Word post-opacity and resolves
	// through the builtin index rather than the def table.
	got = extractGuardClauses(r, guardCond(NewWord("b"), isW, NewWord("Integer")))
	if len(got) != 1 || !got[0].Type.Equal(TInteger) {
		t.Fatalf("builtin-word clauses = %+v, want Integer", got)
	}
}

func TestApplyGuardNarrowing(t *testing.T) {
	isW := NewWord("is")
	cond := guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger))

	// Analysis OFF: a no-op restore, nothing pushed.
	r := w8reg(t)
	r.Defs.Push("x", NewCarrier(TAny))
	before := r.Defs.Depth("x")
	ApplyGuardNarrowing(r, cond)()
	if r.Defs.Depth("x") != before {
		t.Error("with analysis off, narrowing must not touch the def stack")
	}

	// No clauses in the condition: also a no-op.
	r = w8reg(t)
	r.Check.Mode = true
	ApplyGuardNarrowing(r, guardCond(NewWord("x")))()

	// The real path: an abstract binding MEETS the guard type, keeping
	// the source's value ID because is-narrowing is static-only — the
	// runtime binding is unchanged, so a fresh ID would strand every
	// later reference.
	r = w8reg(t)
	r.Check.Mode = true
	src := NewDynamicCarrier(TAny)
	r.Defs.Push("x", src)
	restore := ApplyGuardNarrowing(r, cond)
	narrowed, ok := r.Defs.Top("x")
	if !ok {
		t.Fatal("narrowing must push a binding")
	}
	if IDOf(&narrowed) != IDOf(&src) {
		t.Errorf("narrowed ID = %q, want the source's %q", IDOf(&narrowed), IDOf(&src))
	}
	if narrowed.Parent == nil || !narrowed.Parent.ConformsTo(TInteger) {
		t.Errorf("narrowed binding = %v, want Integer-conforming", narrowed)
	}
	if !IsDynamic(&narrowed) {
		t.Error("a dynamic binding must stay dynamic through the meet")
	}
	restore()
	if v, _ := r.Defs.Top("x"); IsDynamic(&v) != IsDynamic(&src) || IDOf(&v) != IDOf(&src) {
		t.Error("restore must pop the narrowing")
	}

	// An UNBOUND name still pushes the plain guard carrier (the `cur`
	// lookup declines, so there is nothing to meet with).
	r = w8reg(t)
	r.Check.Mode = true
	ApplyGuardNarrowing(r, cond)
	if v, ok := r.Defs.Top("x"); !ok || !v.Parent.Equal(TInteger) {
		t.Errorf("unbound narrowing = %v/%v, want a plain Integer carrier", v, ok)
	}
}

func TestApplyGuardNarrowingRedundantAndDead(t *testing.T) {
	isW := NewWord("is")
	cond := guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger))

	// REDUNDANT GUARD advisory: the binding's static type already
	// entails the guard, so the check cannot fail. Strict, non-concrete
	// carriers only.
	r := w8reg(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewCarrier(TInteger))
	ApplyGuardNarrowing(r, cond)()
	found := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("redundant_guard emitted %d times, want 1", found)
	}
	// Re-running the same guard dedupes rather than piling up (fn bodies
	// re-analyse per shape and fixpoint round).
	ApplyGuardNarrowing(r, cond)()
	found = 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("redundant_guard deduped to %d, want 1", found)
	}

	// DEAD ARM: the guard can never hold for this binding's type, so the
	// meet collapses to Never. The then-arm's unreachable dispatch errors
	// are suppressed for its duration.
	r = w8reg(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewCarrier(TString))
	restore := ApplyGuardNarrowing(r, cond)
	if r.Check.SuppressBodyErrors != 1 {
		t.Errorf("SuppressBodyErrors = %d inside a dead arm, want 1", r.Check.SuppressBodyErrors)
	}
	restore()
	if r.Check.SuppressBodyErrors != 0 {
		t.Errorf("SuppressBodyErrors = %d after restore, want 0", r.Check.SuppressBodyErrors)
	}
}

func TestApplyComplementNarrowing(t *testing.T) {
	isW := NewWord("is")
	cond := guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger))

	// Analysis off: no-op.
	r := w8reg(t)
	r.Defs.Push("x", NewCarrier(TAny))
	ApplyComplementNarrowing(r, cond)()

	// Zero clauses: no-op.
	r = w8reg(t)
	r.Check.Mode = true
	ApplyComplementNarrowing(r, guardCond(NewWord("x")))()

	// MORE THAN ONE clause: the else path of `A and B` is `notA or notB`,
	// so neither clause can be subtracted. Restricted to the single-guard
	// case by construction.
	r = w8reg(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewCarrier(TAny))
	r.Defs.Push("y", NewCarrier(TAny))
	two := guardCond(NewWord("x"), isW, NewTypeLiteral(TInteger),
		NewWord("y"), isW, NewTypeLiteral(TString))
	depth := r.Defs.Depth("x")
	ApplyComplementNarrowing(r, two)()
	if r.Defs.Depth("x") != depth {
		t.Error("a two-clause condition must narrow no else-arm binding")
	}

	// An UNBOUND name has nothing to subtract from.
	r = w8reg(t)
	r.Check.Mode = true
	ApplyComplementNarrowing(r, cond)()

	// A DYNAMIC binding stays gradual: the guard failed at run time, but
	// the static type is Any, so subtracting T would wrongly forbid every
	// later use.
	r = w8reg(t)
	r.Check.Mode = true
	dyn := NewDynamicCarrier(TAny)
	r.Defs.Push("x", dyn)
	ApplyComplementNarrowing(r, cond)()
	if v, _ := r.Defs.Top("x"); !IsDynamic(&v) {
		t.Error("a dynamic binding's else path must stay gradual")
	}

	// The real refinement: a DISJUNCT binding loses the guarded
	// alternative on the else path.
	r = w8reg(t)
	r.Check.Mode = true
	union := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	union.Carrier = true
	src := union
	r.Defs.Push("x", src)
	restore := ApplyComplementNarrowing(r, cond)
	narrowed, _ := r.Defs.Top("x")
	if IDOf(&narrowed) != IDOf(&src) {
		t.Errorf("complement ID = %q, want the source's %q", IDOf(&narrowed), IDOf(&src))
	}
	if narrowed.Parent.ConformsTo(TInteger) {
		t.Errorf("complement = %v, still admits Integer", narrowed)
	}
	restore()

	// NO REFINEMENT: T is disjoint from the binding's type, so the
	// complement equals the original and nothing is pushed.
	r = w8reg(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewCarrier(TString))
	depth = r.Defs.Depth("x")
	ApplyComplementNarrowing(r, cond)()
	if r.Defs.Depth("x") != depth {
		t.Error("a non-refining complement must push nothing")
	}

	// DEAD ELSE ARM: the guard held for every value of x's type, so the
	// else path is unreachable and its errors are suppressed.
	r = w8reg(t)
	r.Check.Mode = true
	r.Defs.Push("x", NewCarrier(TInteger))
	restore = ApplyComplementNarrowing(r, cond)
	if r.Check.SuppressBodyErrors != 1 {
		t.Errorf("SuppressBodyErrors = %d inside a dead else arm, want 1", r.Check.SuppressBodyErrors)
	}
	restore()
	if r.Check.SuppressBodyErrors != 0 {
		t.Errorf("SuppressBodyErrors = %d after restore, want 0", r.Check.SuppressBodyErrors)
	}
}

// TestGuardClauseDepScalarTarget drives the PREDICATE-REFINEMENT arm of
// the guard-type switch: a `refine` newtype whose body is a DepScalar
// narrows to its MINTED lattice node, not to the base family the body's
// Parent records — that node's depScalarUnifier is what admits an
// abstract carrier tagged with it nominally.
func TestGuardClauseDepScalarTarget(t *testing.T) {
	r := w8reg(t)
	big := MintTestType("Number/Integer/GuardBig")
	body := NewDepScalar(DepGT, NewInteger(10))
	if !body.IsDepScalar() {
		t.Fatalf("fixture is not a DepScalar: %v", body)
	}
	r.Defs.PushType("GuardBig", big, body)

	got := extractGuardClauses(r, guardCond(NewWord("n"), NewWord("is"), NewWord("GuardBig")))
	if len(got) != 1 {
		t.Fatalf("clauses = %+v, want one", got)
	}
	if !got[0].Type.Equal(big) {
		t.Errorf("guard type = %v, want the minted node %v", got[0].Type, big)
	}
}

// TestGuardClausePredicateImplied drives the 2-token predicate guard —
// `if (is-list xs) …` — and the ThenOnly contract that comes with it: a
// TRUE predicate implies the type, a FALSE one says nothing about which
// non-T shape the value has, so ApplyComplementNarrowing must skip the
// clause entirely.
func TestGuardClausePredicateImplied(t *testing.T) {
	r := w8reg(t)
	installPred(r, "islist", []Signature{boruPredSig(
		[]FnParam{{Name: "x", Type: TAny}},
		[]*Type{TBoolean},
		[]Value{NewWord("x"), NewWord("is"), NewWord("List")})})

	// A 2-token guard only ever reaches the analysis as a PAREN group —
	// `if (is-list xs) …` — which arrives as a Boolean carrier whose
	// GuardFactInfo preserves the original tokens. The list branch
	// requires three tokens, so the list spelling never gets here.
	predCond := func(toks ...Value) Value {
		v := NewValueRaw(TBoolean, GuardFactInfo{Toks: toks})
		v.Carrier = true
		return v
	}
	cond := predCond(NewWord("islist"), NewWord("xs"))

	// The subject must be a LIVE binding; without one there is no fact.
	if got := extractGuardClauses(r, cond); got != nil {
		t.Errorf("unbound subject = %+v, want no clause", got)
	}

	r.Defs.Push("xs", NewCarrier(TAny))
	got := extractGuardClauses(r, cond)
	if len(got) != 1 {
		t.Fatalf("clauses = %+v, want one predicate-implied clause", got)
	}
	if got[0].Name != "xs" || !got[0].Type.Equal(TList) {
		t.Errorf("clause = %+v, want xs:List", got[0])
	}
	if !got[0].ThenOnly {
		t.Error("a predicate-derived fact must be ThenOnly")
	}

	// A word that is not a recognisable is-T predicate yields nothing.
	if got := extractGuardClauses(r, predCond(NewWord("nosuchpred"), NewWord("xs"))); got != nil {
		t.Errorf("unknown predicate = %+v, want no clause", got)
	}

	// The THEN arm narrows on the fact…
	r.Check.Mode = true
	restore := ApplyGuardNarrowing(r, cond)
	if v, _ := r.Defs.Top("xs"); v.Parent == nil || !v.Parent.ConformsTo(TList) {
		t.Errorf("then-arm binding = %v, want List-conforming", v)
	}
	restore()

	// …and the ELSE arm does not: a false predicate carries no
	// information, so the clause is skipped and the binding is untouched.
	depth := r.Defs.Depth("xs")
	ApplyComplementNarrowing(r, cond)()
	if r.Defs.Depth("xs") != depth {
		t.Error("a ThenOnly clause must leave the else arm untouched")
	}
}
