package native

import (
	"strings"
	"testing"
)

// Tests for the fn 3-arg single-triple form (`fn input output body`,
// sig [(tnot List) Any List]) and its handler branches. The dispatch-
// level behaviour (form equivalence, the tnot-List input rule, mirror
// forms, spec-list regression pins) lives in lang/spec/fn-triple.tsv;
// these tests pin the handler-level contracts covergate needs.

// tripleArgs builds the three handler args for `fn x:Integer [Integer]
// [x mul 2]` as dispatch would deliver them (sig order: input, output,
// body).
func tripleArgs() []Value {
	input := NewTypeLiteral(TInteger)
	output := NewList([]Value{NewWord("Integer")})
	body := NewList([]Value{NewInteger(10), NewWord("add")})
	return []Value{input, output, body}
}

func TestFnTripleHandlerBuildsFunction(t *testing.T) {
	r := seam5Reg(t)
	vals, err := fnTripleHandler(tripleArgs(), nil, nil, r)
	if err != nil {
		t.Fatalf("fn triple: %v", err)
	}
	if len(vals) != 1 || !vals[0].Parent.ConformsTo(TFunction) {
		t.Fatalf("fn triple: want one Function value, got %v", vals)
	}
	fnDef, ok := vals[0].Data.(FnDefInfo)
	if !ok {
		t.Fatalf("fn triple: payload is %T, want FnDefInfo", vals[0].Data)
	}
	if len(fnDef.Signatures) != 1 {
		t.Fatalf("fn triple: want exactly one FnSig, got %d", len(fnDef.Signatures))
	}
	if fnDef.Anonymous {
		t.Fatal("fn triple: must not set the Anonymous (afn) flag")
	}
}

func TestFnTripleHandlerRejectsListInput(t *testing.T) {
	// The sig's tnot-List Pattern keeps dispatch away from this branch;
	// the handler guard is its defensive twin (check-mode fallbacks and
	// direct calls can still land here).
	r := seam5Reg(t)
	args := tripleArgs()
	args[0] = NewList([]Value{NewTypeLiteral(TInteger)})
	_, err := fnTripleHandler(args, nil, nil, r)
	if err == nil {
		t.Fatal("fn triple: a list input must error (spec-list form territory)")
	}
	if !strings.Contains(err.Error(), "non-list input") {
		t.Fatalf("fn triple: list-input error must point at the form rule, got: %v", err)
	}
}

func TestFnTripleHandlerBadInputSig(t *testing.T) {
	// An unresolvable input param routes parseFnDef's error through
	// fnConstruct's failGen tail (genSpec nil here).
	r := seam5Reg(t)
	args := tripleArgs()
	args[0] = NewWord("ZZUnknownType")
	if _, err := fnTripleHandler(args, nil, nil, r); err == nil {
		t.Fatal("fn triple: an unresolvable input sig must error")
	}
}

func TestFnTripleRunFormEquivalence(t *testing.T) {
	// The triple form and the wrapped spec-list form build the same fn.
	r := seam5Reg(t)
	got, err := seam5Run(r, `def t fn x:Integer [Integer] [x mul 2]  t 4`)
	if err != nil {
		t.Fatalf("triple form: %v", err)
	}
	if len(got) != 1 || tripleInt(t, got[0]) != 8 {
		t.Fatalf("triple form: want [8], got %v", got)
	}
	r2 := seam5Reg(t)
	got2, err := seam5Run(r2, `def t fn [[x:Integer] [Integer] [x mul 2]]  t 4`)
	if err != nil {
		t.Fatalf("spec-list form: %v", err)
	}
	if len(got2) != 1 || tripleInt(t, got2[0]) != 8 {
		t.Fatalf("spec-list form: want [8], got %v", got2)
	}
}

func TestFnTripleGenericPopsBindings(t *testing.T) {
	// `gen [T] fn x:T [T] [x]` — the triple form must attach the gen
	// spec and pop the placeholder bindings (T unbound afterwards).
	r := seam5Reg(t)
	got, err := seam5Run(r, `def ident gen [T] fn x:T [T] [x]  ident 42`)
	if err != nil {
		t.Fatalf("generic triple: %v", err)
	}
	if len(got) != 1 || tripleInt(t, got[0]) != 42 {
		t.Fatalf("generic triple: want [42], got %v", got)
	}
	if r.Defs.Has("T") {
		t.Fatal("generic triple: placeholder T must not stay bound after construction")
	}
}

func tripleInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := v.AsConcreteInteger()
	if err != nil {
		t.Fatalf("expected integer, got %v (%v)", v, err)
	}
	return n
}

// --- describe rendering of pattern-constrained sig slots ---

func TestSigArgDisplayNegationPattern(t *testing.T) {
	neg := NewNegation(NewTypeLiteral(TList))
	sig := Signature{
		Args:     []*Type{TAny, TAny},
		Patterns: map[int]Value{0: neg},
	}
	if got := sigArgDisplay(&sig, 0, TAny); got != "(tnot List)" {
		t.Fatalf("negation pattern slot: want (tnot List), got %q", got)
	}
	if got := sigArgDisplay(&sig, 1, TAny); got != TAny.String() {
		t.Fatalf("pattern-free slot: want %q, got %q", TAny.String(), got)
	}
}

func TestSigArgDisplayNonNegationPatternKeepsType(t *testing.T) {
	sig := Signature{
		Args:     []*Type{TInteger},
		Patterns: map[int]Value{0: NewInteger(0)},
	}
	if got := sigArgDisplay(&sig, 0, TInteger); got != TInteger.String() {
		t.Fatalf("literal pattern keeps the historical type rendering: want %q, got %q", TInteger.String(), got)
	}
}

func TestSigArgDisplayNegationOfNonBareInner(t *testing.T) {
	// A negation of a concrete value renders through the value's own
	// String (no bare lattice leaf to name).
	sig := Signature{
		Args:     []*Type{TAny},
		Patterns: map[int]Value{0: NewNegation(NewInteger(5))},
	}
	if got := sigArgDisplay(&sig, 0, TAny); got != "(tnot 5)" {
		t.Fatalf("negation of concrete inner: want (tnot 5), got %q", got)
	}
}

func TestSigDisplayPatternFromParams(t *testing.T) {
	// After normalizeSig, Params carry the Pattern; the display helper
	// must read that store too (not just the positional mirror).
	neg := NewNegation(NewTypeLiteral(TList))
	sig := Signature{
		Params: []FnParam{{Type: TAny, Pattern: &neg}, {Type: TAny}},
	}
	if got := sigArgDisplay(&sig, 0, TAny); got != "(tnot List)" {
		t.Fatalf("params-stored pattern: want (tnot List), got %q", got)
	}
	if _, ok := sigDisplayPattern(&sig, 1); ok {
		t.Fatal("params-stored: position 1 declares no pattern")
	}
}

func TestDescribeFnShowsTripleSig(t *testing.T) {
	r := seam5Reg(t)
	info := BuildFuncInfo(r, "fn")
	if info == nil {
		t.Fatal("no FuncInfo for fn")
	}
	if len(info.Sigs) != 3 {
		t.Fatalf("fn: want 3 sigs in describe data, got %d", len(info.Sigs))
	}
	// Match order: the 3-arg triple sig sorts first (arity), the
	// spec-list sig second, and the 0-argument refusal last (NUR091: a
	// bare `fn` raises through a declared signature of its own).
	if len(info.Sigs[0].Args) != 3 || info.Sigs[0].Args[0] != "(tnot List)" {
		t.Fatalf("fn sig 0: want triple form with (tnot List) input, got %v", info.Sigs[0].Args)
	}
	if info.Sigs[0].Args[2] != TList.String() {
		t.Fatalf("fn sig 0: want a List-typed body slot, got %v", info.Sigs[0].Args)
	}
	if len(info.Sigs[1].Args) != 1 {
		t.Fatalf("fn sig 1: want the 1-arg spec-list form, got %v", info.Sigs[1].Args)
	}
	if len(info.Sigs[2].Args) != 0 {
		t.Fatalf("fn sig 2: want the 0-argument refusal, got %v", info.Sigs[2].Args)
	}
}

func TestMatchFnSigSkipsCarrierPattern(t *testing.T) {
	// A carrier Pattern (a check-mode placeholder for a computed pattern
	// value) is never enforced — MatchFnSig must match on the param TYPE
	// alone, mirroring patternsOk / MatchSignature.
	carrier := NewCarrier(TInteger)
	fnVal := NewFunction(FnDefInfo{Signatures: []FnSig{{
		Params: []FnParam{{Type: TInteger, Pattern: &carrier}},
	}}})
	if MatchFnSig(fnVal, []Value{NewInteger(9)}) == nil {
		t.Fatal("carrier pattern must not be enforced: 9 must match the Integer param")
	}
	// A concrete literal pattern still rejects a mismatch on this path.
	zero := NewInteger(0)
	strictVal := NewFunction(FnDefInfo{Signatures: []FnSig{{
		Params: []FnParam{{Type: TInteger, Pattern: &zero}},
	}}})
	if MatchFnSig(strictVal, []Value{NewInteger(9)}) != nil {
		t.Fatal("a concrete literal pattern must still reject a mismatch")
	}
}

func TestFnTripleHandlerNoPanicOnExoticInputs(t *testing.T) {
	// Repo discipline: every handler must survive type literals and
	// carriers without panicking (lang/go/CLAUDE.md, Panic Prevention).
	// Errors are fine; panics are not.
	exotic := []Value{
		NewTypeLiteral(TMap),
		NewTypeLiteral(TNone),
		NewCarrier(TInteger),
		NewCarrier(TList),
		NewList([]Value{NewInteger(1)}),
		NewMap(NewOrderedMap()),
		NewWord("zz-not-a-type"),
	}
	for _, in := range exotic {
		for _, out := range exotic {
			for _, body := range exotic {
				r := seam5Reg(t)
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Fatalf("fn triple panicked on (%v, %v, %v): %v", in, out, body, p)
						}
					}()
					_, _ = fnTripleHandler([]Value{in, out, body}, nil, nil, r)
				}()
			}
		}
	}
}
