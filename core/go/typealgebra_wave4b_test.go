package core

import (
	"strings"
	"testing"
)

// Wave 4b coverage: the type-level connectives (core_boolean.go —
// tor/tand/tnot handlers and TandValues), the DepScalar algebra
// (depscalar.go — bounds, intersection, complement, between), bounded
// Type bodies (core_boundedtype.go), and the negation / DepScalar /
// disjunct unifier folds (unify_negation.go, unify_dep.go,
// unify_disjunct.go) reached through Unify and installed type
// behaviors.

// --- tor ---------------------------------------------------------------------

func TestTorHandlerBuildsDisjunct(t *testing.T) {
	out, err := TorHandler([]Value{NewTypeLiteral(TString), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TorHandler: %v", err)
	}
	di, aerr := AsDisjunct(out[0])
	if aerr != nil {
		t.Fatalf("tor result not a disjunct: %v", aerr)
	}
	if len(di.Alternatives) != 2 {
		t.Errorf("alternatives = %d, want 2", len(di.Alternatives))
	}
}

func TestTorHandlerSimplifies(t *testing.T) {
	// Duplicate alternatives dedupe to the lone alternative.
	out, err := TorHandler([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TorHandler dup: %v", err)
	}
	if IsDisjunct(&out[0]) {
		t.Error("duplicate union stayed a disjunct")
	}
	// Never is the identity.
	out, err = TorHandler([]Value{NewTypeLiteral(TNever), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TorHandler never: %v", err)
	}
	if IsDisjunct(&out[0]) {
		t.Error("Never did not drop out of the union")
	}
	// Never tor Never collapses to Never.
	out, err = TorHandler([]Value{NewTypeLiteral(TNever), NewTypeLiteral(TNever)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TorHandler never/never: %v", err)
	}
	if !IsNeverShape(out[0]) {
		t.Errorf("Never tor Never = %v, want Never", out[0])
	}
}

func TestTorHandlerDeMorgan(t *testing.T) {
	na := NewNegation(NewTypeLiteral(TInteger))
	nb := NewNegation(NewTypeLiteral(TString))
	out, err := TorHandler([]Value{nb, na}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TorHandler negations: %v", err)
	}
	// (tnot A) tor (tnot B) = tnot (A tand B); Integer tand String is
	// Never, so the complement is Any.
	if !isAnyShape(out[0]) {
		t.Errorf("(tnot Integer) tor (tnot String) = %v, want Any", out[0])
	}
}

func TestTorReturnsFn(t *testing.T) {
	out := TorReturnsFn([]Value{NewTypeLiteral(TString), NewTypeLiteral(TInteger)}, nil)
	if !IsDisjunct(&out[0]) {
		t.Errorf("TorReturnsFn = %v, want a disjunct", out[0])
	}
	// Wrong arity degrades to an Any carrier.
	out = TorReturnsFn([]Value{NewTypeLiteral(TString)}, nil)
	if !IsCarrier(&out[0]) || !ParentOf(&out[0]).Equal(TAny) {
		t.Errorf("TorReturnsFn bad arity = %v, want Any carrier", out[0])
	}
	// The De Morgan fold applies in carrier mode too.
	out = TorReturnsFn([]Value{
		NewNegation(NewTypeLiteral(TString)), NewNegation(NewTypeLiteral(TInteger)),
	}, nil)
	if !isAnyShape(out[0]) {
		t.Errorf("carrier-mode De Morgan = %v, want Any", out[0])
	}
}

// --- tand --------------------------------------------------------------------

func TestTandHandlerAndValues(t *testing.T) {
	// Integer tand Number narrows to Integer.
	out, err := TandHandler([]Value{NewTypeLiteral(TNumber), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TandHandler: %v", err)
	}
	if !(&out[0]).Equal(TInteger) {
		t.Errorf("Integer tand Number = %v, want Integer", out[0])
	}
	// Disjoint scalars collapse to Never.
	nv := TandValues(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	if !IsNeverShape(nv) {
		t.Errorf("Integer tand String = %v, want Never", nv)
	}
	// Never annihilates.
	if !IsNeverShape(TandValues(NewTypeLiteral(TNever), NewTypeLiteral(TInteger))) {
		t.Error("Never tand Integer != Never")
	}
}

func TestTandValuesDistributesOverDisjuncts(t *testing.T) {
	union := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	// (Integer tor String) tand Integer = Integer.
	got := TandValues(union, NewTypeLiteral(TInteger))
	if IsDisjunct(&got) || !(&got).Equal(TInteger) {
		t.Errorf("distribution = %v, want Integer", got)
	}
	// (Integer tor String) tand Boolean = Never.
	if !IsNeverShape(TandValues(union, NewTypeLiteral(TBoolean))) {
		t.Error("disjoint distribution not Never")
	}
	// Disjunct-vs-disjunct cross product keeps both compatible arms.
	other := NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TInteger)})
	both := TandValues(union, other)
	if !IsDisjunct(&both) {
		t.Errorf("union tand union = %v, want a 2-alt disjunct", both)
	}
}

func TestTandValuesDeMorgan(t *testing.T) {
	a := NewNegation(NewTypeLiteral(TInteger))
	b := NewNegation(NewTypeLiteral(TString))
	got := TandValues(a, b)
	ni, err := AsNegation(got)
	if err != nil {
		t.Fatalf("(tnot A) tand (tnot B) = %v, want a negation", got)
	}
	if !IsDisjunct(&ni.Inner) {
		t.Errorf("negation inner = %v, want (Integer tor String)", ni.Inner)
	}
}

func TestTandValuesMergesConcreteMaps(t *testing.T) {
	got := TandValues(mapOf("a", NewInteger(1)), mapOf("b", NewInteger(2)))
	m, err := AsMap(got)
	if err != nil {
		t.Fatalf("map tand map = %v: %v", got, err)
	}
	if m.Len() != 2 {
		t.Errorf("merged map has %d keys, want 2", m.Len())
	}
	// Overlapping keys intersect; incompatible values collapse to Never.
	bad := TandValues(mapOf("a", NewTypeLiteral(TInteger)), mapOf("a", NewTypeLiteral(TString)))
	if !IsNeverShape(bad) {
		t.Errorf("incompatible field intersection = %v, want Never", bad)
	}
	// Compatible overlap keeps the narrower type.
	ok := TandValues(mapOf("a", NewTypeLiteral(TNumber)), mapOf("a", NewTypeLiteral(TInteger)))
	om, err := AsMap(ok)
	if err != nil {
		t.Fatalf("compatible overlap: %v", err)
	}
	av, _ := om.Get("a")
	if !(&av).Equal(TInteger) {
		t.Errorf("overlap value = %v, want Integer", av)
	}
}

func TestTandReturnsFn(t *testing.T) {
	out := TandReturnsFn([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNumber)}, nil)
	if !(&out[0]).Equal(TInteger) {
		t.Errorf("TandReturnsFn = %v, want Integer", out[0])
	}
	// A non-type operand degrades to the declared Any.
	out = TandReturnsFn([]Value{NewInteger(1), NewTypeLiteral(TInteger)}, nil)
	if !IsCarrier(&out[0]) || !ParentOf(&out[0]).Equal(TAny) {
		t.Errorf("TandReturnsFn concrete operand = %v, want Any carrier", out[0])
	}
	out = TandReturnsFn([]Value{NewTypeLiteral(TInteger)}, nil)
	if !IsCarrier(&out[0]) || !ParentOf(&out[0]).Equal(TAny) {
		t.Errorf("TandReturnsFn bad arity = %v, want Any carrier", out[0])
	}
}

// --- tnot --------------------------------------------------------------------

func TestNegateTypeIdentities(t *testing.T) {
	if !isAnyShape(NegateType(NewTypeLiteral(TNever))) {
		t.Error("tnot Never != Any")
	}
	if !IsNeverShape(NegateType(NewTypeLiteral(TAny))) {
		t.Error("tnot Any != Never")
	}
	// Double negation elimination.
	inner := NewTypeLiteral(TString)
	back := NegateType(NewNegation(inner))
	if !(&back).Equal(TString) {
		t.Errorf("tnot tnot String = %v, want String", back)
	}
	// Ordinary type wraps.
	n := NegateType(NewTypeLiteral(TBoolean))
	if !IsNegation(n) {
		t.Errorf("tnot Boolean = %v, want a negation", n)
	}
}

func TestNegateTypeDepScalarComplement(t *testing.T) {
	// tnot (Integer gt 0) = (Integer lte 0) tor (tnot Integer)
	dep := NewDepScalar(DepGT, NewInteger(0))
	got := NegateType(dep)
	di, err := AsDisjunct(got)
	if err != nil {
		t.Fatalf("complement = %v, want a disjunct: %v", got, err)
	}
	var sawRay, sawNeg bool
	for _, alt := range di.Alternatives {
		if alt.IsDepScalar() {
			info, _ := alt.AsDepScalar()
			if info.Hi != nil && info.Hi.Inclusive {
				sawRay = true
			}
		}
		if IsNegation(alt) {
			sawNeg = true
		}
	}
	if !sawRay || !sawNeg {
		t.Errorf("complement alternatives = %v, want (Integer lte 0) and (tnot Integer)", di.Alternatives)
	}
}

func TestTnotHandlerAndReturnsFn(t *testing.T) {
	out, err := TnotHandler([]Value{NewTypeLiteral(TString)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("TnotHandler: %v", err)
	}
	if !IsNegation(out[0]) {
		t.Errorf("TnotHandler = %v, want a negation", out[0])
	}
	cout := TnotReturnsFn([]Value{NewTypeLiteral(TString)}, nil)
	if !IsNegation(cout[0]) || !IsCarrier(&cout[0]) {
		t.Errorf("TnotReturnsFn = %v, want a negation carrier", cout[0])
	}
	cout = TnotReturnsFn(nil, nil)
	if !IsCarrier(&cout[0]) || !ParentOf(&cout[0]).Equal(TAny) {
		t.Errorf("TnotReturnsFn bad arity = %v, want Any carrier", cout[0])
	}
}

func TestShapePredicates(t *testing.T) {
	if !IsNeverShape(NewTypeLiteral(TNever)) || IsNeverShape(NewTypeLiteral(TInteger)) {
		t.Error("isNeverShape broken")
	}
	if !isAnyShape(NewTypeLiteral(TAny)) || isAnyShape(NewTypeLiteral(TInteger)) {
		t.Error("isAnyShape broken")
	}
	if !isAnyShape(NewCarrier(TAny)) {
		t.Error("Any carrier not any-shaped")
	}
	if !isPlainConcreteMap(mapOf("a", NewInteger(1))) {
		t.Error("plain map not plain")
	}
	if isPlainConcreteMap(NewTypeLiteral(TMap)) {
		t.Error("Map literal claims plain concrete")
	}
	if isPlainConcreteMap(NewRecordType(NewOrderedMap())) {
		t.Error("record type claims plain concrete map")
	}
	if isPlainConcreteMap(NewInteger(1)) {
		t.Error("integer claims plain concrete map")
	}
}

// --- depscalar.go -------------------------------------------------------------

func TestDepKindString(t *testing.T) {
	cases := map[DepKind]string{
		DepGT:           "gt",
		DepGTE:          "gte",
		DepLT:           "lt",
		DepLTE:          "lte",
		DepGTE | DepLTE: "gte|lte",
		0:               "?",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("DepKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestBoundToKind(t *testing.T) {
	if BoundToKind(nil, true) != 0 {
		t.Error("nil bound kind != 0")
	}
	inc := &DepBound{Inclusive: true, Value: NewInteger(1)}
	str := &DepBound{Inclusive: false, Value: NewInteger(1)}
	if BoundToKind(inc, true) != DepGTE || BoundToKind(str, true) != DepGT {
		t.Error("lower bound kinds wrong")
	}
	if BoundToKind(inc, false) != DepLTE || BoundToKind(str, false) != DepLT {
		t.Error("upper bound kinds wrong")
	}
}

func TestNewDepScalarValidation(t *testing.T) {
	// Unsupported base → None-parented zero value.
	v := NewDepScalar(DepGT, NewList(nil))
	if !ParentOf(&v).Equal(TNone) {
		t.Errorf("list-bound DepScalar parent = %v, want None", ParentOf(&v))
	}
	// Multi-bit kind does not decompose.
	v = NewDepScalar(DepGT|DepLT, NewInteger(1))
	if !ParentOf(&v).Equal(TNone) {
		t.Errorf("multi-bit DepScalar parent = %v, want None", ParentOf(&v))
	}
	// String base works.
	sv := NewDepScalar(DepLT, NewString("m"))
	if !ParentOf(&sv).Equal(TString) || !sv.IsDepScalar() {
		t.Errorf("string DepScalar = %v", sv)
	}
	// AsDepScalar on a non-DepScalar errors.
	if _, err := addrOf(NewInteger(1)).AsDepScalar(); err == nil {
		t.Error("AsDepScalar on plain integer succeeded")
	}
}

func TestDepScalarRendering(t *testing.T) {
	lo := NewDepScalar(DepGTE, NewInteger(10))
	if got := lo.String(); !strings.Contains(got, "gte") || !strings.Contains(got, "10") {
		t.Errorf("render = %q", got)
	}
	hi := NewDepScalar(DepLT, NewInteger(5))
	if got := hi.String(); !strings.Contains(got, "lt") || !strings.Contains(got, "5") {
		t.Errorf("render = %q", got)
	}
	interval := NewValueRaw(TInteger, DepScalarInfo{
		Lo: &DepBound{Inclusive: true, Value: NewInteger(1)},
		Hi: &DepBound{Inclusive: true, Value: NewInteger(9)},
	})
	if got := interval.String(); !strings.Contains(got, "gte 1") || !strings.Contains(got, "lte 9") {
		t.Errorf("interval render = %q", got)
	}
	empty := NewValueRaw(TInteger, DepScalarInfo{})
	if got := empty.String(); got != "(Integer)" {
		t.Errorf("empty-info render = %q", got)
	}
	if renderDepScalar(NewInteger(1)) != "" {
		t.Error("renderDepScalar accepted a plain integer")
	}
}

func TestDepScalarCheckThroughUnify(t *testing.T) {
	dep := NewDepScalar(DepGTE, NewInteger(10))
	// Satisfying concrete value: unify returns the plain scalar.
	got, ok := Unify(dep, NewInteger(15))
	if !ok {
		t.Fatal("15 does not unify with (Integer gte 10)")
	}
	if n, _ := AsInteger(got); n != 15 {
		t.Errorf("unified = %v, want 15", got)
	}
	// Boundary value passes an inclusive bound.
	if _, ok := Unify(dep, NewInteger(10)); !ok {
		t.Error("10 fails gte 10")
	}
	// Violating value fails.
	if _, ok := Unify(dep, NewInteger(9)); ok {
		t.Error("9 passes gte 10")
	}
	// Cross-base value fails cleanly.
	if _, ok := Unify(dep, NewString("x")); ok {
		t.Error("string passes an Integer bound")
	}
	// Strict bound rejects the boundary.
	strict := NewDepScalar(DepGT, NewInteger(10))
	if _, ok := Unify(strict, NewInteger(10)); ok {
		t.Error("10 passes gt 10")
	}
	// Upper bounds.
	upper := NewDepScalar(DepLTE, NewInteger(3))
	if _, ok := Unify(upper, NewInteger(3)); !ok {
		t.Error("3 fails lte 3")
	}
	if _, ok := Unify(upper, NewInteger(4)); ok {
		t.Error("4 passes lte 3")
	}
}

func TestUnifyDepScalarIntersection(t *testing.T) {
	// Opposite sides form an interval.
	lo := NewDepScalar(DepGT, NewInteger(5))
	hi := NewDepScalar(DepLT, NewInteger(10))
	got, ok := Unify(lo, hi)
	if !ok {
		t.Fatal("(gt 5) tand (lt 10) failed")
	}
	info, err := got.AsDepScalar()
	if err != nil || info.Lo == nil || info.Hi == nil {
		t.Fatalf("interval = %v (%v)", got, err)
	}
	// Same side tightens to the more restrictive bound.
	a := NewDepScalar(DepGTE, NewInteger(10))
	b := NewDepScalar(DepGTE, NewInteger(5))
	got, ok = Unify(a, b)
	if !ok {
		t.Fatal("same-side tighten failed")
	}
	info, _ = got.AsDepScalar()
	if n, _ := AsInteger(info.Lo.Value); n != 10 {
		t.Errorf("tightened bound = %v, want 10", info.Lo.Value)
	}
	// Equal bounds prefer the strict form.
	got, ok = Unify(NewDepScalar(DepGT, NewInteger(7)), NewDepScalar(DepGTE, NewInteger(7)))
	if !ok {
		t.Fatal("equal-bound tighten failed")
	}
	info, _ = got.AsDepScalar()
	if info.Lo.Inclusive {
		t.Error("equal bounds did not prefer strict")
	}
	// Empty intersection fails.
	if _, ok := Unify(NewDepScalar(DepGT, NewInteger(10)), NewDepScalar(DepLT, NewInteger(5))); ok {
		t.Error("(gt 10) tand (lt 5) unified")
	}
	// Equal bounds with a strict side: empty.
	if _, ok := Unify(NewDepScalar(DepGT, NewInteger(5)), NewDepScalar(DepLT, NewInteger(5))); ok {
		t.Error("(gt 5) tand (lt 5) unified")
	}
	// Equal inclusive bounds: the single point survives.
	got, ok = Unify(NewDepScalar(DepGTE, NewInteger(5)), NewDepScalar(DepLTE, NewInteger(5)))
	if !ok {
		t.Error("[5,5] interval rejected")
	} else if !got.IsDepScalar() {
		t.Errorf("[5,5] = %v", got)
	}
	// Different bases fail.
	if _, ok := Unify(NewDepScalar(DepGT, NewInteger(1)), NewDepScalar(DepLT, NewString("z"))); ok {
		t.Error("cross-base DepScalars unified")
	}
}

func TestTandValuesDepScalars(t *testing.T) {
	// TandValues routes DepScalar pairs through Unify.
	got := TandValues(NewDepScalar(DepGTE, NewInteger(10)), NewDepScalar(DepGTE, NewInteger(5)))
	if !got.IsDepScalar() {
		t.Fatalf("tand of DepScalars = %v", got)
	}
	// Empty intersection collapses to Never.
	if !IsNeverShape(TandValues(NewDepScalar(DepGT, NewInteger(10)), NewDepScalar(DepLT, NewInteger(5)))) {
		t.Error("empty DepScalar intersection not Never")
	}
}

func TestMakeDepScalarSig(t *testing.T) {
	sig := MakeDepScalarSig("gte", DepGTE)
	if len(sig.Args) != 2 || sig.BarrierPos != -1 {
		t.Fatalf("sig shape = %+v", sig)
	}
	h := sig.DispatchHandler()
	// args[0] = bound, args[1] = type literal.
	out, err := h([]Value{NewInteger(10), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("gte handler: %v", err)
	}
	if !out[0].IsDepScalar() || !ParentOf(&out[0]).Equal(TInteger) {
		t.Errorf("gte result = %v", out[0])
	}
	// Concrete arg where the type literal belongs.
	if _, err := h([]Value{NewInteger(10), NewInteger(3)}, nil, nil, nil); err == nil {
		t.Error("concrete type-slot accepted")
	}
	// Unsupported base type.
	if _, err := h([]Value{NewInteger(10), NewTypeLiteral(TList)}, nil, nil, nil); err == nil {
		t.Error("List base accepted")
	}
	// Bound/base mismatch.
	if _, err := h([]Value{NewString("x"), NewTypeLiteral(TInteger)}, nil, nil, nil); err == nil {
		t.Error("string bound against Integer base accepted")
	}
	// With a registry the original is remembered without error.
	r := newTestRegistry(t)
	if _, err := h([]Value{NewInteger(1), NewTypeLiteral(TInteger)}, nil, nil, r); err != nil {
		t.Errorf("handler with registry: %v", err)
	}
}

func TestBetweenHandler(t *testing.T) {
	// low, high, type — args top-first: args[0]=low, args[1]=high, args[2]=type.
	out, err := BetweenHandler([]Value{NewInteger(1), NewInteger(9), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("BetweenHandler: %v", err)
	}
	info, aerr := out[0].AsDepScalar()
	if aerr != nil || info.Lo == nil || info.Hi == nil {
		t.Fatalf("between result = %v (%v)", out[0], aerr)
	}
	if !info.Lo.Inclusive || !info.Hi.Inclusive {
		t.Error("between bounds not inclusive")
	}
	// Inverted bounds collapse to Never.
	out, err = BetweenHandler([]Value{NewInteger(9), NewInteger(1), NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("inverted between: %v", err)
	}
	if !IsNeverShape(out[0]) {
		t.Errorf("inverted between = %v, want Never", out[0])
	}
	// Concrete type arg rejected.
	if _, err := BetweenHandler([]Value{NewInteger(1), NewInteger(9), NewInteger(5)}, nil, nil, nil); err == nil {
		t.Error("concrete type arg accepted")
	}
	// Unsupported base rejected.
	if _, err := BetweenHandler([]Value{NewInteger(1), NewInteger(9), NewTypeLiteral(TMap)}, nil, nil, nil); err == nil {
		t.Error("Map base accepted")
	}
	// Low bound of the wrong family rejected.
	if _, err := BetweenHandler([]Value{NewString("a"), NewInteger(9), NewTypeLiteral(TInteger)}, nil, nil, nil); err == nil {
		t.Error("string low bound accepted")
	}
	// High bound of the wrong family rejected.
	if _, err := BetweenHandler([]Value{NewInteger(1), NewString("z"), NewTypeLiteral(TInteger)}, nil, nil, nil); err == nil {
		t.Error("string high bound accepted")
	}
}

// --- named type installs (unify_dep / unify_disjunct / unify_negation) --------

func TestInstalledDepScalarType(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "W4bBig", NewDepScalar(DepGT, NewInteger(10))); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	node := r.LookupTypeName("W4bBig")
	if node == nil {
		t.Fatal("W4bBig not bound")
	}
	if !addrOf(NewInteger(100)).Is(node) {
		t.Error("100 is not Big")
	}
	if addrOf(NewInteger(5)).Is(node) {
		t.Error("5 is Big")
	}
	if addrOf(NewString("hi")).Is(node) {
		t.Error("'hi' is Big")
	}
	// A carrier already tagged as the predicate type passes nominally.
	c := NewCarrier(node)
	if !c.Is(node) {
		t.Error("Big carrier rejected by Big")
	}
	// The bare literal passes through the default walk (not an inhabitant).
	if addrOf(NewTypeLiteral(node)).Is(node) != true {
		t.Log("bare literal routed through the default walk")
	}
}

func TestInstalledDisjunctType(t *testing.T) {
	r := newTestRegistry(t)
	body := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if err := InstallType(r, "W4bIntOrStr", body); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	node := r.LookupTypeName("W4bIntOrStr")
	if node == nil {
		t.Fatal("W4bIntOrStr not bound")
	}
	if !addrOf(NewInteger(1)).Is(node) || !addrOf(NewString("s")).Is(node) {
		t.Error("disjunct alternatives rejected")
	}
	if addrOf(NewBoolean(true)).Is(node) {
		t.Error("boolean admitted by (Integer tor String)")
	}
}

func TestInstalledNegationType(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "W4bNotStr", NewNegation(NewTypeLiteral(TString))); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	node := r.LookupTypeName("W4bNotStr")
	if node == nil {
		t.Fatal("W4bNotStr not bound")
	}
	if !addrOf(NewInteger(5)).Is(node) {
		t.Error("5 rejected by (tnot String)")
	}
	if addrOf(NewString("s")).Is(node) {
		t.Error("'s' admitted by (tnot String)")
	}
}

func TestUnifyNegationDirect(t *testing.T) {
	neg := NewNegation(NewTypeLiteral(TString))
	// Concrete non-member admitted.
	got, ok := Unify(neg, NewInteger(5))
	if !ok {
		t.Fatal("5 rejected by tnot String")
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Errorf("unified = %v", got)
	}
	// Concrete member rejected.
	if _, ok := Unify(neg, NewString("x")); ok {
		t.Error("'x' admitted by tnot String")
	}
	// Any narrows to the negation itself.
	got, ok = Unify(neg, NewTypeLiteral(TAny))
	if !ok || !IsNegation(got) {
		t.Errorf("Any tand tnot String = %v, %v", got, ok)
	}
	// A type wholly contained in the inner type is empty.
	if _, ok := Unify(NewNegation(NewTypeLiteral(TNumber)), NewTypeLiteral(TInteger)); ok {
		t.Error("Integer tand (tnot Number) unified — Integer ⊆ Number")
	}
	// A supertype is NOT wholly contained: admitted.
	if _, ok := Unify(NewNegation(NewTypeLiteral(TInteger)), NewTypeLiteral(TNumber)); !ok {
		t.Error("Number tand (tnot Integer) wrongly proved empty")
	}
	// Compound inner (DepScalar): abstract operand admitted.
	if _, ok := Unify(NewNegation(NewDepScalar(DepGT, NewInteger(0))), NewTypeLiteral(TInteger)); !ok {
		t.Error("Integer tand (tnot (Integer gt 0)) wrongly proved empty")
	}
	// isPlainTypeRef gates.
	if isPlainTypeRef(NewDepScalar(DepGT, NewInteger(0))) {
		t.Error("DepScalar counted as a plain type ref")
	}
	if !isPlainTypeRef(NewTypeLiteral(TInteger)) {
		t.Error("plain literal not a plain type ref")
	}
}

func TestUnifyDisjunctDirect(t *testing.T) {
	disj := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if got, ok := Unify(disj, NewInteger(4)); !ok || !IsConcrete(&got) {
		t.Errorf("4 vs (Integer tor String) = %v, %v", got, ok)
	}
	if _, ok := Unify(disj, NewBoolean(true)); ok {
		t.Error("true admitted by (Integer tor String)")
	}
	// Any preserves the disjunct.
	got, ok := Unify(disj, NewTypeLiteral(TAny))
	if !ok || !IsDisjunct(&got) {
		t.Errorf("Any vs disjunct = %v, %v", got, ok)
	}
	// Map alternative uses open (subset) matching.
	pat := mapOf("kind", NewString("a"))
	md := NewDisjunct([]Value{pat})
	cand := mapOf("kind", NewString("a"), "extra", NewInteger(1))
	if _, ok := Unify(md, cand); !ok {
		t.Error("open map alternative rejected a superset map")
	}
	if _, ok := Unify(md, mapOf("kind", NewString("b"))); ok {
		t.Error("open map alternative admitted a mismatch")
	}
}

// --- bounded Type (core_boundedtype.go) ----------------------------------------

func TestBoundedTypeBasics(t *testing.T) {
	bt := NewBoundedType(TMap)
	if !IsBoundedType(bt) {
		t.Fatal("NewBoundedType not a bounded type")
	}
	if IsBoundedType(NewInteger(1)) || IsBoundedType(NewTypeLiteral(TType)) {
		t.Error("IsBoundedType overclaims")
	}
	n, err := AsBoundedType(bt)
	if err != nil || !n.Equal(TMap) {
		t.Errorf("AsBoundedType = %v, %v", n, err)
	}
	if _, err := AsBoundedType(NewInteger(1)); err == nil {
		t.Error("AsBoundedType accepted an integer")
	}
}

func TestTypeMembershipAndDenotedNode(t *testing.T) {
	if !TypeMembership(NewTypeLiteral(TMap)) {
		t.Error("bare literal not a type")
	}
	if !TypeMembership(NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})) {
		t.Error("disjunct body not a type")
	}
	if TypeMembership(NewInteger(1)) {
		t.Error("concrete integer is a type")
	}
	if TypeMembership(NewCarrier(TInteger)) {
		t.Error("carrier is a type")
	}
	if n := DenotedTypeNode(NewTypeLiteral(TList)); n == nil || !n.Equal(TList) {
		t.Errorf("DenotedTypeNode = %v", n)
	}
	if DenotedTypeNode(NewInteger(1)) != nil {
		t.Error("concrete value denotes a node")
	}
}

func TestUnifyBoundedType(t *testing.T) {
	mapT := NewBoundedType(TMap)
	// A type literal within the bound is admitted.
	got, ok := Unify(mapT, NewTypeLiteral(TMap))
	if !ok {
		t.Fatal("Map literal rejected by Type of [Map]")
	}
	if !IsBareTypeNode(&got) {
		t.Errorf("unified = %v", got)
	}
	// Outside the bound: rejected.
	if _, ok := Unify(mapT, NewTypeLiteral(TInteger)); ok {
		t.Error("Integer literal admitted by Type of [Map]")
	}
	// A concrete value is not a type at all.
	if _, ok := Unify(mapT, NewInteger(1)); ok {
		t.Error("1 admitted by Type of [Map]")
	}
	// Bounded vs bounded: the narrower bound wins.
	numT := NewBoundedType(TNumber)
	intT := NewBoundedType(TInteger)
	got, ok = Unify(numT, intT)
	if !ok {
		t.Fatal("Integer/t vs Number/t failed")
	}
	n, _ := AsBoundedType(got)
	if !n.Equal(TInteger) {
		t.Errorf("narrower bound = %v, want Integer", n)
	}
	// Disjoint bounds fail.
	if _, ok := Unify(NewBoundedType(TMap), NewBoundedType(TInteger)); ok {
		t.Error("disjoint bounded Types unified")
	}
}
