package core

// Stage-5 coverage for canon.go (behavior walk, big numbers, unnamed
// nodes, reach tokens, fn canon), surface.go (unifySurface arms, the
// surfaceUnifier), equal.go (default-equality arms), compare.go
// (classified-compare error paths, container identity), and sugar.go
// (marker expansion + stepSugar).

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// --- canon.go -------------------------------------------------------------

// x5FmtBehavior is a non-default Behavior with its own Format.
type x5FmtBehavior struct{ x5StubBehavior }

func (x5FmtBehavior) Format(Value) string { return "X5FMT" }

// x5DelegatingBehavior delegates Format via the exported marker.
type x5DelegatingBehavior struct{ x5StubBehavior }

func (x5DelegatingBehavior) FormatDelegate() {}

func x5UserType(name string, parent *Type, b TypeBehavior) *Type {
	t := addrOf(withID(Type{d: &Datum{tmeta: &typeMeta{Name: name, Behavior: b}, Parent: parent}}, name))
	t.SetOrigin(OriginUserDef)
	return t
}

func TestX5CanonValueBehaviorWalk(t *testing.T) {
	// User-origin type with a nil Behavior: skipped, falls to the switch.
	plain := x5UserType("X5CanA", TInteger, nil)
	if got := CanonValue(NewValueRaw(plain, IntPayload{N: 7})); got != "7" {
		t.Fatalf("nil-behavior user type canons by family, got %q", got)
	}
	// User-origin type whose Behavior delegates Format: skipped too.
	deleg := x5UserType("X5CanB", TInteger, x5DelegatingBehavior{})
	if got := CanonValue(NewValueRaw(deleg, IntPayload{N: 8})); got != "8" {
		t.Fatalf("format-delegating behavior canons by family, got %q", got)
	}
	// User-origin type with a real Format: routed through it.
	fmted := x5UserType("X5CanC", TInteger, x5FmtBehavior{})
	if got := CanonValue(NewValueRaw(fmted, IntPayload{N: 9})); got != "X5FMT" {
		t.Fatalf("custom behavior owns the canon, got %q", got)
	}
}

func TestX5CanonValueUnnamedNodeAndBigNums(t *testing.T) {
	// A bare node with no registered name canons as its leaf.
	anon := x5UserType("X5Leafy", TInteger, nil)
	if got := CanonValue(NewTypeLiteral(anon)); got != "X5Leafy" {
		t.Fatalf("unregistered node canons as its leaf, got %q", got)
	}
	// Big integer and big decimal arms.
	if got := CanonValue(NewBigInteger(big.NewInt(42))); got == "" {
		t.Fatal("big integer canon must render")
	}
	d, _, err := apd.NewFromString("1.5")
	if err != nil {
		t.Fatalf("apd decimal: %v", err)
	}
	if got := CanonValue(NewBigDecimal(d)); got == "" {
		t.Fatal("big decimal canon must render")
	}
}

func TestX5CanonValueMapBadPayload(t *testing.T) {
	// A Map-parented value whose payload is not map-readable falls back
	// to Value.String.
	bad := NewValueRaw(TMap, IntPayload{N: 3})
	_ = CanonValue(bad)
}

func TestX5CanonChildDisjunctGrouping(t *testing.T) {
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	got := canonChild(d)
	if !strings.HasPrefix(got, "(") || !strings.HasSuffix(got, ")") {
		t.Fatalf("a disjunct child canon groups in parens, got %q", got)
	}
}

func TestX5CanonReachTokenNested(t *testing.T) {
	inner := NewReach(ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{{KeyLit: NewWord("a")}},
	})
	if got := canonReachToken(inner); got != "m.a" {
		t.Fatalf("nested reach token canons dotted, got %q", got)
	}
}

func TestX5CanonReachTokenEndMarker(t *testing.T) {
	// The end marker renders as `;` INSIDE a reach, never as `end`
	// (NUR066). A bare `end` in reach-token position is a Word — a field
	// name in a segment — so spelling the marker `end` here would
	// re-parse as that field and silently change the program. Pinned in
	// both positions the token appears in.
	if got := canonReachToken(NewEnd()); got != ";" {
		t.Fatalf("the end marker canons as `;` inside a reach, got %q", got)
	}
	stray := NewReach(ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{{KeyLit: NewEnd()}},
	})
	if got := canonReach(stray); got != "m.;" {
		t.Fatalf("a marker SEGMENT canons as m.;, got %q", got)
	}
	// The contrast that makes the arm load-bearing: the WORD `end` is a
	// field name and keeps its own spelling, so the two do not collide.
	named := NewReach(ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{{KeyLit: NewWord("end")}},
	})
	if got := canonReach(named); got != "m.end" {
		t.Fatalf("a Word segment named `end` canons as m.end, got %q", got)
	}
	// Top-level canon is UNAFFECTED — outside a reach `end` is still the
	// marker's own spelling (CanonValue's own IsEnd arm).
	if got := CanonValue(NewEnd()); got != "end" {
		t.Fatalf("top-level end still canons as `end`, got %q", got)
	}
}

func TestX5CanonFnDefMultiSigMultiParam(t *testing.T) {
	sigA := Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TString}},
		Returns: []*Type{TBoolean},
		Impl:    Boru([]Value{NewBoolean(true)}),
	}
	sigB := Signature{
		Params: []FnParam{{Name: "c"}},
		Impl:   Boru([]Value{NewInteger(1)}),
	}
	got := canonFnDef(FnDefInfo{Name: "x5fn", Signatures: []Signature{sigA, sigB}})
	if !strings.Contains(got, "a:") || !strings.Contains(got, "b:") || !strings.Contains(got, "c") {
		t.Fatalf("fn canon must render both sigs and every param, got %q", got)
	}
}

// --- surface.go -----------------------------------------------------------

func x5Surface(name string, conform map[string]bool) (*Type, Value, *SurfaceInfo) {
	node := x5UserType(name, TSurface, nil)
	info := &SurfaceInfo{Name: "Surface/" + name, Required: NewOrderedMap(), Conform: conform}
	sv := NewSurfaceType(node, info)
	return node, sv, info
}

func TestX5UnifySurfaceSurfacePairs(t *testing.T) {
	_, sv1, _ := x5Surface("X5Shp1", map[string]bool{})
	_, sv2, _ := x5Surface("X5Shp2", map[string]bool{})

	// Shared payload unifies to itself.
	got, uerr := unifySurface(sv1, sv1)
	if uerr != nil {
		t.Fatalf("same surface must unify: %v", uerr)
	}
	if !IsSurfaceType(got) {
		t.Fatalf("expected the surface back, got %v", got)
	}
	// Distinct payloads reject.
	if _, uerr := unifySurface(sv1, sv2); uerr == nil {
		t.Fatal("distinct surfaces must not unify")
	}
}

func TestX5UnifySurfaceConcreteMembership(t *testing.T) {
	node, sv, info := x5Surface("X5Shp3", map[string]bool{IDOf(TInteger): true})
	installSurfaceUnifier(node, info, "X5Shp3")

	// A concrete integer conforms via the surfaceUnifier walk.
	got, uerr := unifySurface(sv, NewInteger(5))
	if uerr != nil {
		t.Fatalf("conforming concrete value: %v", uerr)
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("expected 5, got %v", got)
	}
	// A non-conforming concrete value rejects.
	if _, uerr := unifySurface(sv, NewString("x")); uerr == nil {
		t.Fatal("a non-conforming value must reject")
	}
}

func TestX5UnifySurfaceAbstractArms(t *testing.T) {
	_, sv, _ := x5Surface("X5Shp4", map[string]bool{IDOf(TInteger): true})

	// Bare type node in the conformance set: containment.
	got, uerr := unifySurface(sv, NewTypeLiteral(TInteger))
	if uerr != nil {
		t.Fatalf("conforming bare node: %v", uerr)
	}
	if !IsBareTypeNode(&got) {
		t.Fatalf("expected the narrower type back, got %v", got)
	}
	// Carrier of a conforming type.
	got, uerr = unifySurface(sv, NewCarrier(TInteger))
	if uerr != nil {
		t.Fatalf("conforming carrier: %v", uerr)
	}
	if !got.Carrier() {
		t.Fatalf("expected the carrier back, got %v", got)
	}
	// Non-conforming bare node walks off the chain and rejects.
	if _, uerr := unifySurface(sv, NewTypeLiteral(TString)); uerr == nil {
		t.Fatal("a non-conforming type must reject")
	}
}

func TestX5SurfaceUnifierMatchFormatAndInfoOf(t *testing.T) {
	node, sv, info := x5Surface("X5Shp5", map[string]bool{IDOf(TInteger): true})
	info.Required.Set("area", NewFnUndef(FnUndefInfo{}))
	installSurfaceUnifier(node, info, "X5Shp5")
	su := node.Behavior().(*surfaceUnifier)
	su.ContentMembership()

	if !su.Match(NewInteger(1), node) {
		t.Fatal("an Integer is in the conformance set")
	}
	if su.Match(NewString("x"), node) {
		t.Fatal("a String is not in the conformance set")
	}
	if got := su.Format(sv); !strings.Contains(got, "area") {
		t.Fatalf("surface Format renders the operation set, got %q", got)
	}
	if got, ok := SurfaceInfoOf(node); !ok || got != info {
		t.Fatalf("SurfaceInfoOf should find the payload, got %v ok=%v", got, ok)
	}
	if _, ok := SurfaceInfoOf(TInteger); ok {
		t.Fatal("TInteger is not a surface node")
	}
}

// --- equal.go -------------------------------------------------------------

func TestX5ValuesEqualDefaultScalarArms(t *testing.T) {
	if !valuesEqualDefault(NewBoolean(true), NewBoolean(true)) {
		t.Fatal("equal booleans")
	}
	if valuesEqualDefault(NewBoolean(true), NewBoolean(false)) {
		t.Fatal("unequal booleans")
	}
	if !valuesEqualDefault(NewAtom("a"), NewAtom("a")) {
		t.Fatal("equal atoms")
	}
	if valuesEqualDefault(NewAtom("a"), NewAtom("b")) {
		t.Fatal("unequal atoms")
	}
}

func TestX5ValuesEqualDefaultStructuralArms(t *testing.T) {
	// Two tables with the same record fields.
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TInteger))
	f2 := NewOrderedMap()
	f2.Set("x", NewTypeLiteral(TInteger))
	if !valuesEqualDefault(NewTableType(RecordTypeInfo{Fields: f1}), NewTableType(RecordTypeInfo{Fields: f2})) {
		t.Fatal("same-schema tables are equal")
	}

	// Two typed maps with the same child.
	if !valuesEqualDefault(NewTypedMap(NewTypeLiteral(TInteger)), NewTypedMap(NewTypeLiteral(TInteger))) {
		t.Fatal("same-child typed maps are equal")
	}

	// Two concrete maps compare structurally.
	m1 := NewOrderedMap()
	m1.Set("k", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("k", NewInteger(1))
	if !valuesEqualDefault(NewMap(m1), NewMap(m2)) {
		t.Fatal("same-content maps are equal")
	}
}

func TestX5MapsEqualMismatches(t *testing.T) {
	a := NewOrderedMap()
	a.Set("k", NewInteger(1))
	b := NewOrderedMap()
	if mapsEqual(a, b) {
		t.Fatal("length mismatch")
	}
	c := NewOrderedMap()
	c.Set("z", NewInteger(1))
	if mapsEqual(a, c) {
		t.Fatal("missing key")
	}
}

// --- compare.go -----------------------------------------------------------

// x5ErrComparer is a Comparer whose Compare fails.
type x5ErrComparer struct{ x5StubBehavior }

func (x5ErrComparer) Compare(a, b Value) (int, error) { return 0, errors.New("x5 boom") }

// x5ZeroComparer always reports equality.
type x5ZeroComparer struct{ x5StubBehavior }

func (x5ZeroComparer) Compare(a, b Value) (int, error) { return 0, nil }

func TestX5OrderedCompareErrorPath(t *testing.T) {
	bad := x5UserType("X5Cmp1", TInteger, x5ErrComparer{})
	a := NewValueRaw(bad, IntPayload{N: 1})
	b := NewValueRaw(bad, IntPayload{N: 2})
	if _, err := orderedCompare("lt", a, b); err == nil {
		t.Fatal("a failing comparer must surface its error")
	}
}

func TestX5ScalarSemanticEqualArms(t *testing.T) {
	// Same-parent scalar with a non-default Behavior delegates Equal.
	same := x5UserType("X5Cmp2", TInteger, x5StubBehavior{})
	a := NewValueRaw(same, IntPayload{N: 1})
	b := NewValueRaw(same, IntPayload{N: 2})
	eq, handled := scalarSemanticEqual(a, b)
	if !handled || !eq {
		t.Fatalf("behavior-owned equality, got eq=%v handled=%v", eq, handled)
	}

	// Cross-parent pair whose LCA is one of the two parents and carries
	// a Comparer: equality by Compare()==0.
	base := x5UserType("X5Cmp3", TInteger, x5ZeroComparer{})
	child := x5UserType("X5Cmp4", base, nil)
	av := NewValueRaw(child, IntPayload{N: 1})
	bv := NewValueRaw(base, IntPayload{N: 9})
	eq, handled = scalarSemanticEqual(av, bv)
	if !handled || !eq {
		t.Fatalf("LCA comparer equality, got eq=%v handled=%v", eq, handled)
	}
}

func TestX5ExactEqualContainerIdentity(t *testing.T) {
	// FlexList identity: same store pointer.
	fl := NewFlexList([]Value{NewInteger(1)})
	if !ExactEqual(fl, fl) {
		t.Fatal("a flex list is eq to itself")
	}
	if ExactEqual(fl, NewFlexList([]Value{NewInteger(1)})) {
		t.Fatal("a fresh flex list is a different container")
	}

	// XML identity: shared attr map + children slice.
	attr := NewOrderedMap()
	x1 := NewXmlElement("t", attr, nil)
	x2 := NewXmlElement("t", attr, nil)
	if !ExactEqual(x1, x2) {
		t.Fatal("same-attr empty-children xml elements alias")
	}
	other := NewXmlElement("u", attr, nil)
	if ExactEqual(x1, other) {
		t.Fatal("different tags are different elements")
	}
}

func TestX5OpaqueIdealExactEqualDefault(t *testing.T) {
	eq, handled := opaqueIdealExactEqual(NewInteger(1), NewInteger(1))
	if eq || handled {
		t.Fatal("an integer payload is not an opaque ideal")
	}
}

func TestX5RelationalHandlerError(t *testing.T) {
	h := relationalHandler("lt", func(c int) bool { return c < 0 })
	if _, err := h([]Value{NewInteger(1), NewString("x")}, nil, nil, nil); err == nil {
		t.Fatal("a cross-family pair is incomparable for lt")
	}
}

// --- sugar.go -------------------------------------------------------------

func TestX5SugarExpansionKinds(t *testing.T) {
	r := x5reg(t)
	r.BindSugarWord(SugarForceArity, "x5arity")
	r.BindSugarWord(SugarMini, "x5mini")
	r.BindSugarWord(SugarGenApply, "x5of")
	r.BindSugarWord(SugarGenHead, "x5gen")
	src := NewInteger(0)

	got, err := SugarExpansion(r, SugarInfo{Kind: SugarForceArity, N: 2}, src, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("force-arity expands to word+N, got %v err=%v", got, err)
	}
	got, err = SugarExpansion(r, SugarInfo{Kind: SugarMini, Name: "re", Src: "a+"}, src, false)
	if err != nil || len(got) != 3 {
		t.Fatalf("mini expands to word+kind+src, got %v err=%v", got, err)
	}
	got, err = SugarExpansion(r, SugarInfo{Kind: SugarTypeBound, Items: []Value{NewTypeLiteral(TMap)}}, src, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("type-bound expands to one paren, got %v err=%v", got, err)
	}

	// Angle head form with a head error surfaces it.
	if _, err := SugarExpansion(r, SugarInfo{Kind: SugarAngle, Name: "Box", HeadErr: "broken"}, src, true); err == nil {
		t.Fatal("a head-form error must surface")
	}
	// Angle head form success.
	got, err = SugarExpansion(r, SugarInfo{Kind: SugarAngle, Name: "Box", Head: NewList(nil)}, src, true)
	if err != nil || len(got) != 3 {
		t.Fatalf("angle head form expands to recv+gen+head, got %v err=%v", got, err)
	}
	// Angle use-site paren.
	got, err = SugarExpansion(r, SugarInfo{Kind: SugarAngle, Name: "Box", Items: []Value{NewTypeLiteral(TInteger)}}, src, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("angle use-site expands to one paren, got %v err=%v", got, err)
	}
}

func TestX5StepSugarSplices(t *testing.T) {
	r := x5reg(t)
	r.BindSugarWord(SugarUsurp, "x5usurp")
	e := New(r)
	e.Tape = NewTape([]Value{NewSugar(SugarInfo{Kind: SugarUsurp})}, 8)
	if err := e.stepSugar(0); err != nil {
		t.Fatalf("stepSugar: %v", err)
	}
	if w, err := AsWord(e.Tape.Ref(0)); err != nil || w.Name != "x5usurp" {
		t.Fatalf("marker must splice to its bound word, got %v", e.Tape.At(0))
	}
}
