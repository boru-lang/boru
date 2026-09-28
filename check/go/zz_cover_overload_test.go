package check

// zz_cover_overload_test.go — white-box coverage for the overload /
// carrier-shape analysis helpers in carrier.go: the typed-list length
// refinement constructor, the combo diagnostic renderer, the
// fn-predicate overload hazard, dynamic overload reachability
// counting, the non-concrete / disjunct / imprecise operand probes,
// per-slot polymorphism detection, the check-mode literal-word
// adjacency window, and the mixed-conform disjunct split test.
//
// Every test drives the DOCUMENTED behaviour from the function's doc
// comment; fixture words follow the corpus convention of a `q` suffix.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// ovImpl is a trivial native implementation for fixture signatures —
// check-mode analysis never invokes it.
func ovImpl() *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return args, nil
	})
}

// --- NewCarrierTypedListLen ------------------------------------------------

func TestNewCarrierTypedListLenRefinement(t *testing.T) {
	v := NewCarrierTypedListLen(core.TInteger, 3)
	if !v.Carrier {
		t.Fatal("result must be a carrier")
	}
	if !v.Parent.ConformsTo(core.TList) {
		t.Fatalf("result parent = %s, want a List", v.Parent.Leaf())
	}
	ct, ok := v.Data.(core.ChildTypeInfo)
	if !ok {
		t.Fatalf("Data = %T, want core.ChildTypeInfo", v.Data)
	}
	if ct.Child.Parent == nil || !ct.Child.Parent.Equal(core.TInteger) {
		t.Error("element carrier must carry the given element type (Integer)")
	}
	if ct.Len == nil || *ct.Len != 3 {
		t.Fatalf("ChildTypeInfo.Len = %v, want a pointer to 3", ct.Len)
	}
	// The documented consumer: the static index checker recovers the
	// statically-known length from the refinement.
	if n, ok := StaticListLen(v); !ok || n != 3 {
		t.Errorf("StaticListLen = (%d, %v), want (3, true)", n, ok)
	}
}

// --- comboTypeNames --------------------------------------------------------

func TestComboTypeNamesRendering(t *testing.T) {
	// The doc's example rendering: "Integer, String". A concrete value and
	// a carrier both render through their Parent's leaf name.
	got := comboTypeNames([]core.Value{core.NewInteger(1), core.NewCarrier(core.TString)})
	if got != "Integer, String" {
		t.Errorf("comboTypeNames = %q, want %q", got, "Integer, String")
	}
	if got := comboTypeNames([]core.Value{core.NewCarrier(core.TBoolean)}); got != "Boolean" {
		t.Errorf("single-carrier combo = %q, want %q", got, "Boolean")
	}
	if got := comboTypeNames(nil); got != "" {
		t.Errorf("empty combo = %q, want empty", got)
	}
}

// --- FnPredicateOverloadHazard ---------------------------------------------

// covPredicateType mints a predicate type over Integer (the boru
// fn-body membership shape, `fn [[n:Integer] [Boolean] [...]]`) and
// returns its lattice node, whose Behavior is a *core.PredicateUnifier.
func covPredicateType(t *testing.T, r *core.Registry, name string) *core.Type {
	t.Helper()
	pred := core.MarkPredicateFn(core.NewValueRaw(core.TFunction, core.FnDefInfo{Signatures: []core.Signature{{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TBoolean},
		Impl:    core.Boru([]core.Value{core.NewBoolean(true)}),
	}}}))
	if err := core.InstallType(r, name, pred); err != nil {
		t.Fatalf("InstallType(%s): %v", name, err)
	}
	def := r.LookupTypeName(name)
	if def == nil {
		t.Fatalf("LookupTypeName(%s) = nil after install", name)
	}
	if _, ok := def.Behavior().(*core.PredicateUnifier); !ok {
		t.Fatalf("%s behavior = %T, want *core.PredicateUnifier", name, def.Behavior())
	}
	return def
}

func TestFnPredicateOverloadHazardArms(t *testing.T) {
	r := newTestRegistry(t)
	posT := covPredicateType(t, r, "HazPosQ")
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "hazq",
		Signatures: []core.Signature{
			{Args: []*core.Type{posT}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
			// Arity filler: same word, different arity — never reachable
			// for a one-arg call shape.
			{Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 2, Impl: ovImpl()},
		},
	})

	// (a) a predicate-typed slot exists AND (b) more than one arm stays
	// reachable: a dynamic Integer optimistically matches the lenient
	// predicate arm and strictly matches the Integer arm — the hazard.
	if !FnPredicateOverloadHazard(r, "hazq", []core.Value{core.NewDynamicCarrier(core.TInteger)}) {
		t.Error("dynamic Integer over predicate+Integer one-arg arms must be a hazard")
	}
	// A String carrier reaches NEITHER one-arg arm: no hazard even
	// though a predicate slot exists in the overload set.
	if FnPredicateOverloadHazard(r, "hazq", []core.Value{core.NewCarrier(core.TString)}) {
		t.Error("zero reachable arms is not a hazard")
	}
	// Two reachable arms but NO predicate slot: no hazard.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "plainq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
			{Args: []*core.Type{core.TNumber}, Returns: []*core.Type{core.TNumber}, BarrierPos: 1, Impl: ovImpl()},
		},
	})
	if FnPredicateOverloadHazard(r, "plainq", []core.Value{core.NewDynamicCarrier(core.TInteger)}) {
		t.Error("an overload set without a predicate slot carries no hazard")
	}
	// Unknown word and single-signature word: never a hazard.
	if FnPredicateOverloadHazard(r, "no-such-q", []core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("an unknown word is never a hazard")
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "monopredq",
		Signatures: []core.Signature{
			{Args: []*core.Type{posT}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
		},
	})
	if FnPredicateOverloadHazard(r, "monopredq", []core.Value{core.NewDynamicCarrier(core.TInteger)}) {
		t.Error("a single-signature word is never a hazard")
	}
}

// --- DynamicReachableOverloadCount -----------------------------------------

func TestDynamicReachableOverloadCountArms(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "dovq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
			{Args: []*core.Type{core.TString}, Returns: []*core.Type{core.TString}, BarrierPos: 1, Impl: ovImpl()},
			// Arity filler: skipped for every one-arg call shape.
			{Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 2, Impl: ovImpl()},
		},
	})

	// A strict Any carrier could hold ANY runtime value, so it reaches
	// EVERY same-arity arm — the genuinely runtime-dynamic dispatch.
	if n := DynamicReachableOverloadCount(r, "dovq", []core.Value{core.NewCarrier(core.TAny)}); n != 2 {
		t.Errorf("strict Any carrier: reachable = %d, want 2 (every same-arity arm)", n)
	}
	// A strict Integer carrier is exact: only the Integer arm.
	if n := DynamicReachableOverloadCount(r, "dovq", []core.Value{core.NewCarrier(core.TInteger)}); n != 1 {
		t.Errorf("strict Integer carrier: reachable = %d, want 1", n)
	}
	// A generic placeholder slot admits by per-call instantiation, so a
	// dynamic arg counts it reachable even though its bound-disjointness
	// probe cannot see the instantiation.
	tp := core.MintTypeParam(r, core.GenParam{Name: "TQ"})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "genq",
		Signatures: []core.Signature{
			{Args: []*core.Type{tp}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
		},
	})
	if n := DynamicReachableOverloadCount(r, "genq", []core.Value{core.NewDynamicCarrier(core.TInteger)}); n != 2 {
		t.Errorf("dynamic Integer vs generic+Integer arms: reachable = %d, want 2", n)
	}
	// Unknown word and single-signature word: 0 — no ambiguity question.
	if n := DynamicReachableOverloadCount(r, "no-such-q", []core.Value{core.NewCarrier(core.TAny)}); n != 0 {
		t.Errorf("unknown word: reachable = %d, want 0", n)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "monosq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
		},
	})
	if n := DynamicReachableOverloadCount(r, "monosq", []core.Value{core.NewCarrier(core.TAny)}); n != 0 {
		t.Errorf("single-signature word: reachable = %d, want 0", n)
	}
}

// --- AnyNonConcreteOperand -------------------------------------------------

func TestAnyNonConcreteOperandShapes(t *testing.T) {
	if AnyNonConcreteOperand(nil) {
		t.Error("no operands: nothing is non-concrete")
	}
	if AnyNonConcreteOperand([]core.Value{core.NewInteger(1), core.NewString("x")}) {
		t.Error("all payload-bearing values are concrete")
	}
	if !AnyNonConcreteOperand([]core.Value{core.NewInteger(1), core.NewCarrier(core.TInteger)}) {
		t.Error("a typed carrier is a non-concrete operand")
	}
	if !AnyNonConcreteOperand([]core.Value{core.NewTypeLiteral(core.TInteger)}) {
		t.Error("a bare type literal is a non-concrete operand")
	}
}

// --- anyDisjunctCarrier ----------------------------------------------------

func TestAnyDisjunctCarrierShapes(t *testing.T) {
	union := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TMap),
		core.NewTypeLiteral(core.TAny),
	})
	unionCarrier := union
	unionCarrier.Carrier = true

	if !anyDisjunctCarrier([]core.Value{core.NewInteger(1), unionCarrier}) {
		t.Error("a union carrier among the operands must be detected")
	}
	// The doc's bare-disjunct-carrier note: the Parent IS the union type,
	// with no DisjunctInfo payload required.
	if !anyDisjunctCarrier([]core.Value{core.NewCarrier(core.TDisjunct)}) {
		t.Error("a bare Disjunct-typed carrier is a union carrier")
	}
	if anyDisjunctCarrier([]core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a single-type carrier is not a union carrier")
	}
	// A disjunct VALUE without the Carrier flag is not a carrier at all.
	if anyDisjunctCarrier([]core.Value{union}) {
		t.Error("a non-carrier disjunct value must not be detected")
	}
	if anyDisjunctCarrier(nil) {
		t.Error("no operands: no union carrier")
	}
}

// --- anyImpreciseCarrier ---------------------------------------------------

func TestAnyImpreciseCarrierShapes(t *testing.T) {
	if !anyImpreciseCarrier([]core.Value{core.NewCarrier(core.TInteger)}) {
		t.Error("a concrete-but-imprecise typed carrier must be detected")
	}
	// Bare type nodes are excluded: a type-literal operand of is/typeof
	// is not a runtime value to re-match.
	if anyImpreciseCarrier([]core.Value{core.NewTypeLiteral(core.TInteger)}) {
		t.Error("a bare type literal is not an imprecise carrier")
	}
	if anyImpreciseCarrier([]core.Value{core.NewInteger(5)}) {
		t.Error("a concrete value is not an imprecise carrier")
	}
	if anyImpreciseCarrier(nil) {
		t.Error("no operands: no imprecise carrier")
	}
}

// --- slotIsPolymorphic -----------------------------------------------------

func TestSlotIsPolymorphicOverloads(t *testing.T) {
	r := newTestRegistry(t)
	// The slice shape from the doc comment: the data slot is String in
	// one overload and List in an equally-reachable sibling.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "slcq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{core.TString}, BarrierPos: 2, Impl: ovImpl()},
			{Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: ovImpl()},
			// Arity filler: skipped for every two-arg call shape.
			{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: ovImpl()},
		},
	})

	dynAny := core.NewDynamicCarrier(core.TAny)
	intC := core.NewCarrier(core.TInteger)

	// A dynamic value at the data slot with the sibling's other position
	// matching: the slot is polymorphic (String vs List).
	if !slotIsPolymorphic(r, "slcq", []core.Value{dynAny, intC}, 0, core.TString) {
		t.Error("slice data slot must be polymorphic: an equally-reachable List sibling exists")
	}
	// Every overload constrains position 1 to the SAME type — narrowing
	// the count slot to Integer is sound, not polymorphic.
	if slotIsPolymorphic(r, "slcq", []core.Value{dynAny, intC}, 1, core.TInteger) {
		t.Error("a slot every sibling agrees on is not polymorphic")
	}
	// The dynamic value's bound is provably DISJOINT from the sibling's
	// slot type (Integer vs List): the sibling is unreachable.
	if slotIsPolymorphic(r, "slcq", []core.Value{core.NewDynamicCarrier(core.TInteger), intC}, 0, core.TString) {
		t.Error("a provably-disjoint bound must not make the slot polymorphic")
	}
	// The sibling's OTHER position rejects the actual arg: unreachable.
	if slotIsPolymorphic(r, "slcq", []core.Value{dynAny, core.NewCarrier(core.TString)}, 0, core.TString) {
		t.Error("a sibling whose other position rejects the args is not reachable")
	}
	// Fallback signatures never count as siblings.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "fbq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{core.TString}, BarrierPos: 2, Impl: ovImpl()},
			{Fallback: true, Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: ovImpl()},
		},
	})
	if slotIsPolymorphic(r, "fbq", []core.Value{dynAny, intC}, 0, core.TString) {
		t.Error("a fallback signature is not an equally-reachable sibling")
	}
	// Unknown word / single-signature word: never polymorphic.
	if slotIsPolymorphic(r, "no-such-q", []core.Value{dynAny, intC}, 0, core.TString) {
		t.Error("an unknown word has no overloads")
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "monoslq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{core.TString}, BarrierPos: 2, Impl: ovImpl()},
		},
	})
	if slotIsPolymorphic(r, "monoslq", []core.Value{dynAny, intC}, 0, core.TString) {
		t.Error("a single-signature word has no second overload")
	}
}

// --- adjacentToLiteralWord / isLiteralWord ---------------------------------

func TestIsLiteralWordSet(t *testing.T) {
	for _, name := range []string{"import", "module", "unpack"} {
		if !isLiteralWord(core.NewWord(name)) {
			t.Errorf("%q must be a check-mode literal word", name)
		}
	}
	if isLiteralWord(core.NewWord("addq")) {
		t.Error("an ordinary word is not a literal word")
	}
	if isLiteralWord(core.NewString("import")) {
		t.Error("a non-word value is never a literal word")
	}
}

func TestAdjacentToLiteralWordWindow(t *testing.T) {
	imp := core.NewWord("import")
	str := core.NewString("boru:mod")

	// Forward form `import "x"`: the string's immediate back-neighbour.
	if !adjacentToLiteralWord([]core.Value{imp, str}, 1) {
		t.Error("forward import form: string at distance 1 behind the word")
	}
	// Stack form `"x" import`: the string's immediate forward-neighbour.
	if !adjacentToLiteralWord([]core.Value{str, imp}, 0) {
		t.Error("stack import form: string at distance 1 before the word")
	}
	// Export-name form `unpack ExportName "boru:mod"`: the module name is
	// TWO tokens after the literal word — the wider window catches it.
	if !adjacentToLiteralWord([]core.Value{core.NewWord("unpack"), core.NewWord("ExportName"), str}, 2) {
		t.Error("unpack export-name form: string at distance 2 behind the word")
	}
	// And the mirrored stack order at distance two.
	if !adjacentToLiteralWord([]core.Value{str, core.NewWord("ExportName"), core.NewWord("unpack")}, 0) {
		t.Error("string at distance 2 before the literal word must be adjacent")
	}
	// No literal word inside the two-token window.
	if adjacentToLiteralWord([]core.Value{core.NewWord("addq"), str}, 1) {
		t.Error("an ordinary neighbour word is not a literal word")
	}
	if adjacentToLiteralWord([]core.Value{str}, 0) {
		t.Error("a lone string has no neighbours")
	}
	// StripToCarriers' documented exception rides on this window: the
	// import-adjacent string is kept concrete so the import handler can
	// resolve and analyse the target.
	out := StripToCarriers([]core.Value{imp, str})
	if !core.IsConcrete(out[1]) {
		t.Error("the import-adjacent string must survive carrier-stripping")
	}
}

// --- carrierMixedConform ---------------------------------------------------

func TestCarrierMixedConformSplit(t *testing.T) {
	mixed := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger),
		core.NewTypeLiteral(core.TString),
	})
	mixed.Carrier = true

	// Integer|String vs Integer: one alternative conforms, one rejects —
	// the forward/stack split is ambiguous.
	if !carrierMixedConform(mixed, core.TInteger) {
		t.Error("Integer|String vs Integer must be mixed")
	}
	// Every alternative conforms (Integer|Float vs Number): a PURE
	// carrier, the split is consistent.
	pure := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger),
		core.NewTypeLiteral(core.TFloat),
	})
	pure.Carrier = true
	if carrierMixedConform(pure, core.TNumber) {
		t.Error("all-conforming alternatives are pure, not mixed")
	}
	// No alternative conforms: equally pure.
	if carrierMixedConform(mixed, core.TBoolean) {
		t.Error("all-rejecting alternatives are pure, not mixed")
	}
	// Guards: nil type, concrete value, non-carrier disjunct, dynamic
	// carrier, and a single-type carrier (fewer than two alternatives).
	if carrierMixedConform(mixed, nil) {
		t.Error("a nil type is never mixed")
	}
	if carrierMixedConform(core.NewInteger(1), core.TInteger) {
		t.Error("a concrete value is never mixed")
	}
	nonCarrier := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger),
		core.NewTypeLiteral(core.TString),
	})
	if carrierMixedConform(nonCarrier, core.TInteger) {
		t.Error("a non-carrier disjunct value is never mixed")
	}
	dyn := mixed
	dyn.Dynamic = true
	if carrierMixedConform(dyn, core.TInteger) {
		t.Error("a dynamic carrier is handled elsewhere, never mixed")
	}
	if carrierMixedConform(core.NewCarrier(core.TInteger), core.TInteger) {
		t.Error("a single-alternative carrier is never mixed")
	}
}
