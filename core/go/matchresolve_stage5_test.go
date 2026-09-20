package core

import (
	"strings"
	"testing"
)

// Stage-5 coverage for match.go (patternsOk / OpenUnifyMap), resolve.go
// (the scalar-word cascade and typed-container deep resolution),
// typed_bind.go (RunTypedBind's happy and decline arms), typetable.go
// (lookup guards, RegisterType path validation, Clone, MintTestType),
// and types.go (NewType expansion, ResolveTypePath, the lattice
// predicates, ValidateTypeNameParts, refreshTypeNames).

// wt5KindMap builds the concrete one-key map {kind:<v>} used by the
// structural-map-pattern arms.
func wt5KindMap(kind string) Value {
	m := NewOrderedMap()
	m.Set("kind", NewString(kind))
	return NewMap(m)
}

// wt5Predicate builds a single-arg predicate fn whose body is the given
// token run, declared over the given input type.
func wt5Predicate(r *Registry, input *Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name:     "Wt5Pred",
		Registry: r,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: input}},
			Impl:       Boru(body),
			BarrierPos: 0,
		}},
	})
}

// --- match.go: patternsOk -------------------------------------------------

func TestWt5PatternsOkCarrierPattern(t *testing.T) {
	// A CARRIER pattern is a check-mode placeholder — never enforced.
	sig := &Signature{Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewCarrier(TInteger)}}
	tape := NewTape([]Value{NewInteger(99)}, 0)
	if !patternsOk(sig, []int{0}, tape, 0, nil) {
		t.Fatal("carrier pattern must be skipped, not enforced")
	}
}

func TestWt5PatternsOkForwardWordResolution(t *testing.T) {
	// A forward position may hold the unresolved Word for a def-bound
	// value; patternsOk resolves it through r.Defs before unifying.
	r := newTestRegistry(t)
	r.Defs.Push("wt5bound", NewInteger(5))
	defer r.Defs.Pop("wt5bound")

	sig := &Signature{Args: []*Type{TInteger}, Patterns: map[int]Value{0: NewInteger(5)}}
	tape := NewTape([]Value{NewWord("wt5bound")}, 0)
	if !patternsOk(sig, []int{0}, tape, 1, r) {
		t.Fatal("forward word bound to the pattern value must match")
	}
	// The same word bound to a mismatching value must fail.
	r.Defs.Push("wt5bound2", NewInteger(7))
	defer r.Defs.Pop("wt5bound2")
	tape2 := NewTape([]Value{NewWord("wt5bound2")}, 0)
	if patternsOk(sig, []int{0}, tape2, 1, r) {
		t.Fatal("forward word bound to a mismatching value must fail")
	}
}

func TestWt5PatternsOkStructuralMapArms(t *testing.T) {
	sig := &Signature{Args: []*Type{TMap}, Patterns: map[int]Value{0: wt5KindMap("api")}}

	// Forward position: structural map patterns are stack-only — skip.
	tape := NewTape([]Value{wt5KindMap("db")}, 0)
	if !patternsOk(sig, []int{0}, tape, 1, nil) {
		t.Fatal("structural map pattern must be skipped on forward positions")
	}
	// Stack position, mismatching map: OpenUnifyMap rejects.
	if patternsOk(sig, []int{0}, tape, 0, nil) {
		t.Fatal("stack-matched structural map pattern must reject a mismatch")
	}
	// Stack position, matching map: continue.
	tapeOk := NewTape([]Value{wt5KindMap("api")}, 0)
	if !patternsOk(sig, []int{0}, tapeOk, 0, nil) {
		t.Fatal("stack-matched structural map pattern must accept a subset match")
	}
}

func TestWt5PatternsOkNegationArms(t *testing.T) {
	notList := NewNegation(NewTypeLiteral(TList))
	sig := &Signature{Args: []*Type{TAny}, Patterns: map[int]Value{0: notList}}

	// A list operand violates tnot List.
	tapeList := NewTape([]Value{NewList([]Value{NewInteger(1)})}, 0)
	if patternsOk(sig, []int{0}, tapeList, 0, nil) {
		t.Fatal("tnot List vs a concrete list must reject")
	}
	// A non-list operand passes the negation and continues.
	tapeInt := NewTape([]Value{NewInteger(7)}, 0)
	if !patternsOk(sig, []int{0}, tapeInt, 0, nil) {
		t.Fatal("tnot List vs an integer must accept")
	}
}

func TestWt5OpenUnifyMapNonConcreteAndAbsent(t *testing.T) {
	// A typed-map pattern has no OrderedMap to walk — routed through
	// the full unifier.
	typed := NewTypedMap(NewTypeLiteral(TInteger))
	conc := NewOrderedMap()
	conc.Set("a", NewInteger(1))
	if !OpenUnifyMap(typed, NewMap(conc)) {
		t.Fatal("typed-map pattern vs conforming concrete map must unify")
	}

	// A pattern key absent on the candidate unifies against Absent;
	// an Absent-admitting pattern value accepts and continues.
	pat := NewOrderedMap()
	pat.Set("opt", NewTypeLiteral(TAbsent))
	if !OpenUnifyMap(NewMap(pat), NewMap(NewOrderedMap())) {
		t.Fatal("Absent-admitting pattern value must accept a missing key")
	}
}

// --- resolve.go -----------------------------------------------------------

func TestWt5ResolveWordsDeepScalarArms(t *testing.T) {
	// true / false words.
	if v := ResolveWordsDeep(NewWord("true")); !ValuesEqual(v, NewBoolean(true)) {
		t.Fatalf("true word = %v", v)
	}
	if v := ResolveWordsDeep(NewWord("false")); !ValuesEqual(v, NewBoolean(false)) {
		t.Fatalf("false word = %v", v)
	}
	// Builtin type name resolves to its literal.
	if v := ResolveWordsDeep(NewWord("Integer")); !IsBareTypeNode(v) || !(&v).Equal(TInteger) {
		t.Fatalf("Integer word = %v", v)
	}
	// Unknown names degrade to Atoms.
	if v := ResolveWordsDeep(NewWord("wt5-nope")); !IsAtom(v) {
		t.Fatalf("unknown word = %v", v)
	}
	// The registry arm resolves a user type binding.
	r := newTestRegistry(t)
	def := r.Types.MintType("Wt5RWD", TInteger)
	body := NewTypeLiteral(def)
	r.Defs.PushType("Wt5RWD", def, body)
	defer r.Defs.Pop("Wt5RWD")
	if v := ResolveWordsDeepR(NewWord("Wt5RWD"), r); !ValuesEqual(v, body) {
		t.Fatalf("registry-armed word = %v", v)
	}
}

func TestWt5ResolveWordsDeepTypedContainers(t *testing.T) {
	// Typed list with inline elements: child and elements resolve deep.
	tl := NewTypedListWithElements(NewTypeLiteral(TBoolean), []Value{NewWord("true")})
	out := ResolveWordsDeep(tl)
	if !IsTypedList(out) {
		t.Fatalf("typed list did not survive resolution: %v", out)
	}
	ci, err := AsChildType(out)
	if err != nil || len(ci.Elements) != 1 || !ValuesEqual(ci.Elements[0], NewBoolean(true)) {
		t.Fatalf("typed-list elements not resolved: %v %v", ci, err)
	}

	// Typed map with inline entries.
	tm := NewTypedMapWithEntries(NewTypeLiteral(TBoolean), []ChildEntry{{Key: "a", Value: NewWord("false")}})
	out = ResolveWordsDeep(tm)
	if !IsTypedMap(out) {
		t.Fatalf("typed map did not survive resolution: %v", out)
	}
	ci, err = AsChildType(out)
	if err != nil || len(ci.Entries) != 1 || !ValuesEqual(ci.Entries[0].Value, NewBoolean(false)) {
		t.Fatalf("typed-map entries not resolved: %v %v", ci, err)
	}
}

// --- typed_bind.go --------------------------------------------------------

func TestWt5RunTypedBindPredicateArms(t *testing.T) {
	r := newTestRegistry(t)

	// A false verdict declines the binding.
	predFalse := wt5Predicate(r, TInteger, []Value{NewBoolean(false)})
	spec := &TypedBindSpec{Kind: TypedBindPredicate, Name: "x", Describe: "Wt5P", Cons: &predFalse}
	if _, err := RunTypedBind(r, spec, NewInteger(3)); err == nil ||
		!strings.Contains(err.Error(), "does not satisfy predicate type") {
		t.Fatalf("false verdict must decline, got %v", err)
	}

	// A true verdict admits; a recorded Def reparents to the named type.
	predTrue := wt5Predicate(r, TInteger, []Value{NewBoolean(true)})
	def := r.Types.MintType("Wt5PredBind", TInteger)
	spec = &TypedBindSpec{Kind: TypedBindPredicate, Name: "x", Describe: "Wt5PredBind", Cons: &predTrue, Def: def}
	out, err := RunTypedBind(r, spec, NewInteger(3))
	if err != nil {
		t.Fatalf("true verdict must admit: %v", err)
	}
	if !out.Parent.Equal(def) {
		t.Fatalf("predicate bind must reparent to the named type, got %v", out.Parent)
	}
	if n, _ := AsInteger(out); n != 3 {
		t.Fatalf("payload must survive the reparent, got %v", out)
	}
}

func TestWt5RunTypedBindRefineArms(t *testing.T) {
	r := newTestRegistry(t)
	def := r.Types.MintType("Wt5RefBind", TInteger)
	spec := &TypedBindSpec{Kind: TypedBindRefine, Name: "x", Describe: "Wt5RefBind", Def: def}

	// A value outside the builtin base is declined.
	if _, err := RunTypedBind(r, spec, NewString("s")); err == nil ||
		!strings.Contains(err.Error(), "does not unify with declared type") {
		t.Fatalf("non-conforming value must decline, got %v", err)
	}
	// A conforming value reparents to the newtype.
	out, err := RunTypedBind(r, spec, NewInteger(4))
	if err != nil {
		t.Fatalf("conforming value must bind: %v", err)
	}
	if !out.Parent.Equal(def) {
		t.Fatalf("refine bind must reparent, got %v", out.Parent)
	}
}

func TestWt5RunTypedBindDepScalarArms(t *testing.T) {
	r := newTestRegistry(t)
	cons := NewDepScalar(DepGT, NewInteger(10))
	spec := &TypedBindSpec{Kind: TypedBindDepScalar, Name: "x", Describe: "(Integer gt 10)", Cons: &cons}

	// Outside the subset: declined.
	if _, err := RunTypedBind(r, spec, NewInteger(5)); err == nil ||
		!strings.Contains(err.Error(), "does not unify with declared type") {
		t.Fatalf("out-of-range value must decline, got %v", err)
	}
	// Inside the subset: admitted with the base tag kept.
	out, err := RunTypedBind(r, spec, NewInteger(20))
	if err != nil {
		t.Fatalf("in-range value must bind: %v", err)
	}
	if !out.Parent.Equal(TInteger) {
		t.Fatalf("DepScalar bind must keep the base tag, got %v", out.Parent)
	}
}

// --- typetable.go ---------------------------------------------------------

func TestWt5TypeTableLookupAndRegisterGuards(t *testing.T) {
	var nilTT *TypeTable
	if nilTT.LookupByID("x") != nil {
		t.Fatal("nil table LookupByID must be nil")
	}
	if Builtin.LookupByID("") != nil {
		t.Fatal("empty-ID lookup must be nil")
	}

	tt := NewDynamicTypeTable()
	if _, err := tt.RegisterType("", 971001, "wt5:test", nil); err == nil ||
		!strings.Contains(err.Error(), "empty path") {
		t.Fatalf("empty path must be declined, got %v", err)
	}
	if _, err := tt.RegisterType("A//B", 971002, "wt5:test", nil); err == nil ||
		!strings.Contains(err.Error(), "empty part") {
		t.Fatalf("empty part must be declined, got %v", err)
	}
}

func TestWt5TypeTableCloneCopiesParts(t *testing.T) {
	cl := Builtin.Clone()
	if cl == nil || !cl.parts["Scalar"] {
		t.Fatal("Clone must copy the parts index")
	}
}

func TestWt5MintTestTypeExpansion(t *testing.T) {
	// A short-name first part expands via the Builtin leaf index; a
	// fully-expanded builtin path returns the builtin node itself.
	if got := MintTestType("Number/Float"); got != TFloat {
		t.Fatalf("MintTestType(Number/Float) = %v, want the builtin Float", got)
	}
	// A non-builtin leaf under a builtin parent attaches there.
	got := MintTestType("Number/Wt5MintCustom")
	if got == nil || got.Parent != TNumber {
		t.Fatalf("MintTestType(Number/Wt5MintCustom).Parent = %v, want Number", got)
	}
}

func TestWt5BuiltinRegisterRefreshesTypeNames(t *testing.T) {
	// Registering into the package-level Builtin table refreshes the
	// parser's bare-name snapshot (refreshTypeNames).
	def, err := Builtin.RegisterType("Ideal/Wt5RefreshProbe", 977501, "wt5:test", nil)
	if err != nil {
		t.Fatalf("RegisterType on Builtin: %v", err)
	}
	if TypeNames["Wt5RefreshProbe"] != def {
		t.Fatal("TypeNames must include the freshly-registered builtin")
	}
	if got, ok := ResolveBuiltinTypeName("Wt5RefreshProbe"); !ok || got != def {
		t.Fatalf("ResolveBuiltinTypeName = %v %v", got, ok)
	}
}

// --- types.go -------------------------------------------------------------

func TestWt5NewTypeShortNameExpansion(t *testing.T) {
	// "String/ProperString" expands to Scalar/String/ProperString.
	got, err := NewType("String/ProperString")
	if err != nil || got != TStringProper {
		t.Fatalf("NewType(String/ProperString) = %v %v", got, err)
	}
}

func TestWt5ResolveTypePathInvalid(t *testing.T) {
	if _, ok := ResolveTypePath("Wt5Zz/Nope"); ok {
		t.Fatal("unknown path must not resolve")
	}
}

func TestWt5LatticePredicates(t *testing.T) {
	if !TInteger.ConformsTo(nil) {
		t.Fatal("nil pattern must be a vacuous match")
	}
	if !TInteger.PathSubtype(TNumber) || TNumber.PathSubtype(TInteger) {
		t.Fatal("PathSubtype must follow the ancestry walk")
	}
	if got := TInteger.Specificity(); got != 3 {
		t.Fatalf("Integer specificity = %d, want 3", got)
	}
	if got := TAny.Specificity(); got != 1 {
		t.Fatalf("Any specificity = %d, want 1", got)
	}
	var nilT *Type
	if nilT.Leaf() != "" {
		t.Fatal("nil Leaf must be empty")
	}
}

func TestWt5ValidateTypeNamePartsConflict(t *testing.T) {
	err := ValidateTypeNameParts("Wt5A/Wt5B", func(p string) bool { return p == "Wt5B" })
	if err == nil || !strings.Contains(err.Error(), "conflicts with an existing type name") {
		t.Fatalf("conflicting part must be declined, got %v", err)
	}
	if err := ValidateTypeNameParts("Wt5A", func(string) bool { return false }); err != nil {
		t.Fatalf("clean name must pass, got %v", err)
	}
}
