package core

// Stage-5 unify-family coverage: unify.go predicate-reference arms,
// Node-literal folds and degenerate-root narrowing, unify_options.go
// field rules, unify_list.go concrete/typed arms, unify_predicate.go
// gates, unify_dep.go canonicalisation, membership.go contracts, and
// the generics_unify.go child-resolution cascade.

import (
	"testing"
)

// x5reg builds a fresh registry with the root context initialised.
func x5reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// x5PredFn builds a runnable predicate fn value: one Integer param,
// body a bare `true` literal (self-evaluating, so RunPredicate always
// admits an Integer candidate).
func x5PredFn() Value {
	sig := Signature{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TBoolean},
		Impl:    Boru([]Value{NewBoolean(true)}),
	}
	return NewValueRaw(TFunction, FnDefInfo{Signatures: []Signature{sig}})
}

// x5PredType installs the predicate fn as type name and returns the
// minted node plus the exact body value installed.
func x5PredType(t *testing.T, r *Registry, name string) (*Type, Value) {
	t.Helper()
	fn := x5PredFn()
	if err := InstallType(r, name, fn); err != nil {
		t.Fatalf("InstallType(%s): %v", name, err)
	}
	def := r.LookupTypeName(name)
	if def == nil {
		t.Fatalf("LookupTypeName(%s) = nil after install", name)
	}
	return def, fn
}

// --- unify.go -------------------------------------------------------------

func TestX5UnifyRWrapper(t *testing.T) {
	r := x5reg(t)
	got, ok := UnifyR(NewTypeLiteral(TInteger), NewInteger(4), r)
	if !ok {
		t.Fatal("Integer literal vs 4 must unify")
	}
	if n, _ := AsInteger(got); n != 4 {
		t.Fatalf("expected 4, got %v", got)
	}
}

func TestX5IsPredicateFnValueNilParent(t *testing.T) {
	if isPredicateFnValue(Value{}) {
		t.Fatal("zero Value has no Parent and is not a predicate fn")
	}
}

func TestX5ResolvePredicateRefAtomAndFnBody(t *testing.T) {
	r := x5reg(t)
	def, fn := x5PredType(t, r, "X5PosA")

	// Atom naming the predicate type resolves to the minted node.
	got, ok := resolvePredicateRef(NewAtom("X5PosA"), r)
	if !ok || !got.Equal(def) {
		t.Fatalf("atom ref should resolve to the predicate node, got %v ok=%v", got, ok)
	}

	// Anonymous fn body (Name empty) resolves via the shared-construction
	// reverse lookup over the def table.
	got, ok = resolvePredicateRef(fn, r)
	if !ok || !got.Equal(def) {
		t.Fatalf("fn-body ref should resolve to the predicate node, got %v ok=%v", got, ok)
	}

	// A name that resolves to a NON-predicate type returns false.
	if err := InstallType(r, "X5AliasA", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("InstallType alias: %v", err)
	}
	if _, ok := resolvePredicateRef(NewAtom("X5AliasA"), r); ok {
		t.Fatal("alias type is not a predicate type")
	}

	// A def-table name with no type body is skipped by the reverse
	// lookup; with no matching construction the ref resolves to nothing.
	r2 := x5reg(t)
	r2.Defs.Push("x5plain", NewInteger(1))
	if _, ok := resolvePredicateRef(x5PredFn(), r2); ok {
		t.Fatal("an uninstalled predicate fn resolves to no type")
	}
}

func TestX5DisjunctHasPredicateFalse(t *testing.T) {
	disj := DisjunctInfo{Alternatives: []Value{NewInteger(1), NewTypeLiteral(TString)}}
	if disjunctHasPredicate(disj) {
		t.Fatal("no predicate alternative present")
	}
}

func TestX5UnifyInnerPredicateBothSides(t *testing.T) {
	r := x5reg(t)
	def, _ := x5PredType(t, r, "X5PosB")

	// Predicate on the a side admits the concrete b side.
	got, err := UnifyExplainR(NewTypeLiteral(def), NewInteger(5), r)
	if err != nil {
		t.Fatalf("predicate literal vs 5: %v", err)
	}
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("expected 5, got %v", got)
	}

	// Predicate on the b side.
	got, err = UnifyExplainR(NewInteger(6), NewTypeLiteral(def), r)
	if err != nil {
		t.Fatalf("6 vs predicate literal: %v", err)
	}
	if n, _ := AsInteger(got); n != 6 {
		t.Fatalf("expected 6, got %v", got)
	}
}

func TestX5UnifyInnerDisjunctPredicateBSide(t *testing.T) {
	r := x5reg(t)
	_, fn := x5PredType(t, r, "X5PosC")

	// The disjunct with a predicate alternative sits on the b side.
	disj := NewDisjunct([]Value{fn})
	got, err := UnifyExplainR(NewInteger(7), disj, r)
	if err != nil {
		t.Fatalf("7 vs predicate disjunct: %v", err)
	}
	if n, _ := AsInteger(got); n != 7 {
		t.Fatalf("expected 7, got %v", got)
	}
}

func TestX5UnifyDisjunctRNoMatch(t *testing.T) {
	r := x5reg(t)
	_, fn := x5PredType(t, r, "X5PosD")
	disj := DisjunctInfo{Alternatives: []Value{fn}}
	if _, err := unifyDisjunctR(disj, NewString("nope"), r); err == nil {
		t.Fatal("a string does not satisfy an Integer predicate disjunct")
	}
}

func TestX5UnifyNodeLiteralFamilies(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	m := NewMap(om)

	// b-side Node literal against a concrete map routes to the map family.
	got, ok := Unify(m, NewTypeLiteral(TNode))
	if !ok {
		t.Fatal("map vs Node literal must unify")
	}
	if got.Parent == nil || !got.Parent.Equal(TMap) {
		t.Fatalf("expected a Map result, got %v", got)
	}

	// a-side Node literal against a concrete list routes to the list family.
	lst := NewList([]Value{NewInteger(1)})
	got, ok = Unify(NewTypeLiteral(TNode), lst)
	if !ok {
		t.Fatal("Node literal vs list must unify")
	}
	if got.Parent == nil || !got.Parent.Equal(TList) {
		t.Fatalf("expected a List result, got %v", got)
	}

	// A non-node-family value rejects.
	if _, ok := Unify(NewTypeLiteral(TNode), NewInteger(5)); ok {
		t.Fatal("Node literal vs an integer must fail")
	}
}

func TestX5UnifyFnUndefRoute(t *testing.T) {
	// The FnUndef shape routes to unifyFnUndefShape, which rejects a
	// non-function other side.
	if _, ok := Unify(NewFnUndef(FnUndefInfo{}), NewInteger(1)); ok {
		t.Fatal("FnUndef vs integer must fail")
	}
}

func TestX5DegenRootConcreteWins(t *testing.T) {
	// None type literal (ruling) vs the concrete `none` value: the
	// concrete inhabitant wins.
	got, ok := Unify(NewTypeLiteral(TNone), NewNone())
	if !ok {
		t.Fatal("None literal vs none must unify")
	}
	if !IsNone(got) {
		t.Fatalf("expected the concrete none value, got %v", got)
	}
}

func TestX5UnifyObjectTypeInstance(t *testing.T) {
	node := &Type{tmeta: &typeMeta{Name: "X5Cls"}, ID: "X5Cls", Parent: TClass}
	// NewClassType stamps info.Type with its first argument, so mint the
	// class-type value with the instance node itself as the minted node.
	ot := NewClassType(node, ClassTypeInfo{ID: "X5Cls", Name: "Class/X5Cls", Fields: NewOrderedMap()})
	inst := NewValueRaw(node, IntPayload{N: 1})
	got, err := unifyObjectType(ot, inst, nil)
	if err != nil {
		t.Fatalf("instance vs its object type: %v", err)
	}
	if !got.Parent.Equal(node) {
		t.Fatalf("expected the instance back, got %v", got)
	}
}

func TestX5UnifySameOrSubtypeLiteralOnRight(t *testing.T) {
	got, err := unifySameOrSubtype(NewInteger(9), NewTypeLiteral(TInteger))
	if err != nil {
		t.Fatalf("9 vs Integer literal: %v", err)
	}
	if n, _ := AsInteger(got); n != 9 {
		t.Fatalf("expected 9, got %v", got)
	}
}

// --- unify_predicate.go ---------------------------------------------------

func TestX5PredicateUnifierMatchGates(t *testing.T) {
	r := x5reg(t)
	def, _ := x5PredType(t, r, "X5PosE")
	pu, ok := def.Behavior().(*PredicateUnifier)
	if !ok {
		t.Fatalf("predicate type must carry a PredicateUnifier, got %T", def.Behavior())
	}
	pu.ContentMembership()

	// Gate 1 rejects an input-type mismatch without running the body.
	if pu.Match(NewString("s"), def) {
		t.Fatal("a String is rejected at the predicate input gate")
	}
	// Gate 2 runs the body (always true here).
	if !pu.Match(NewInteger(5), def) {
		t.Fatal("an Integer must pass the always-true predicate")
	}
	// A bare type literal defers to the lattice walk (matchMembership).
	_ = pu.Match(NewTypeLiteral(TInteger), def)
}

func TestX5PredicateUnifierUnifyDirect(t *testing.T) {
	r := x5reg(t)
	def, _ := x5PredType(t, r, "X5PosF")
	pu := def.Behavior().(*PredicateUnifier)

	// Abstract a / concrete c: the candidate is the c side.
	got, uerr := pu.Unify(NewTypeLiteral(def), NewInteger(3), nil)
	if uerr != nil {
		t.Fatalf("predicate Unify literal-vs-3: %v", uerr)
	}
	if n, _ := AsInteger(got); n != 3 {
		t.Fatalf("expected 3, got %v", got)
	}

	// Rejected candidate fails definitively.
	if _, uerr := pu.Unify(NewString("x"), NewTypeLiteral(def), nil); uerr == nil {
		t.Fatal("a String candidate must be rejected")
	}
}

func TestX5InstallPredicateUnifierDirect(t *testing.T) {
	r := x5reg(t)
	node := &Type{tmeta: &typeMeta{Name: "X5IP"}, ID: "X5IP", Parent: TInteger}
	installPredicateUnifier(node, x5PredFn(), r, "X5IP")
	if _, ok := node.Behavior().(*PredicateUnifier); !ok {
		t.Fatalf("expected a PredicateUnifier, got %T", node.Behavior())
	}
}

// --- membership.go --------------------------------------------------------

// x5StubBehavior is a minimal non-Comparer TypeBehavior.
type x5StubBehavior struct{}

func (x5StubBehavior) Match(v Value, t *Type) bool { return true }
func (x5StubBehavior) Format(v Value) string       { return "x5stub" }
func (x5StubBehavior) Equal(a, b Value) bool       { return true }

func TestX5BaseCompareNoComparer(t *testing.T) {
	if _, err := baseCompare(x5StubBehavior{}, NewInteger(1), NewInteger(2)); err != ErrNoComparer {
		t.Fatalf("expected ErrNoComparer, got %v", err)
	}
}

func TestX5MatchMembershipTypeLiteralDefers(t *testing.T) {
	called := false
	got := matchMembership(NewTypeLiteral(TInteger), TInteger, nil, func(Value) bool {
		called = true
		return false
	})
	if called {
		t.Fatal("a bare type literal must not be put to the member fn")
	}
	if !got {
		t.Fatal("default lattice walk should admit Integer literal vs TInteger")
	}
}

func TestX5UnifyMembershipArms(t *testing.T) {
	// Concrete candidate on the c side.
	got, uerr := unifyMembership(NewTypeLiteral(TInteger), NewInteger(8), "X5M",
		func(v Value) (Value, bool, error) { return v, true, nil })
	if uerr != nil {
		t.Fatalf("admitted candidate: %v", uerr)
	}
	if n, _ := AsInteger(got); n != 8 {
		t.Fatalf("expected 8, got %v", got)
	}
	// Rejected candidate fails definitively.
	if _, uerr := unifyMembership(NewInteger(8), NewTypeLiteral(TInteger), "X5M",
		func(v Value) (Value, bool, error) { return Value{}, false, nil }); uerr == nil {
		t.Fatal("rejected candidate must fail")
	}
}

// --- unify_dep.go / unify_disjunct.go / unify_negation.go / weak_flex.go --

func TestX5UnifyDepScalarBSideAbstract(t *testing.T) {
	dep := NewDepScalar(DepGT, NewInteger(10))
	// Dep on the b side, abstract other side: falls through to the
	// subtype narrowing.
	got, uerr := unifyDepScalar(NewTypeLiteral(TInteger), ShapeTypeLiteral, dep, ShapeDepScalar)
	if uerr != nil {
		t.Fatalf("dep vs Integer literal: %v", uerr)
	}
	if !got.IsDepScalar() {
		t.Fatalf("expected the narrower dep side, got %v", got)
	}
}

func TestX5MembershipMarkers(t *testing.T) {
	(&DepScalarUnifier{}).ContentMembership()
	(&DisjunctUnifier{}).ContentMembership()
	(&NegationUnifier{}).ContentMembership()
	(&WeakFlexMapData{}).payloadMarker()
	(&WeakFlexListData{}).payloadMarker()
	(&WeakFlexXmlData{}).payloadMarker()
}

func TestX5DisjunctUnifierMatchBareLiteral(t *testing.T) {
	d := &DisjunctUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		Alternatives:    []Value{NewTypeLiteral(TInteger)},
		typeName:        "X5D",
	}
	_ = d.Match(NewTypeLiteral(TString), TString)
}

// --- unify_options.go -----------------------------------------------------

func x5Options(fields map[string]Value, order []string) Value {
	om := NewOrderedMap()
	for _, k := range order {
		om.Set(k, fields[k])
	}
	return NewOptionsType(om)
}

func x5Map(fields map[string]Value, order []string) Value {
	om := NewOrderedMap()
	for _, k := range order {
		om.Set(k, fields[k])
	}
	return NewMap(om)
}

func TestX5OptionsOnRightSide(t *testing.T) {
	opts := x5Options(map[string]Value{"a": NewInteger(1)}, []string{"a"})
	conc := x5Map(map[string]Value{"a": NewInteger(2)}, []string{"a"})
	// Options on the b side exercises the else-canonicalisation.
	got, uerr := unifyOptionsFamily(conc, ShapeMap, opts, ShapeOptions, nil)
	if uerr != nil {
		t.Fatalf("map vs options: %v", uerr)
	}
	gm, _ := AsMap(got)
	v, _ := gm.Get("a")
	if n, _ := AsInteger(v); n != 2 {
		t.Fatalf("caller value wins, got %v", v)
	}
}

func TestX5OptionsUnknownKey(t *testing.T) {
	opts := x5Options(map[string]Value{"a": NewInteger(1)}, []string{"a"})
	conc := x5Map(map[string]Value{"z": NewInteger(2)}, []string{"z"})
	if _, uerr := unifyOptionsFamily(opts, ShapeOptions, conc, ShapeMap, nil); uerr == nil {
		t.Fatal("unknown key must fail")
	}
}

func TestX5OptionsDefaultsAndRequired(t *testing.T) {
	// Absent key with a concrete default fills in.
	opts := x5Options(map[string]Value{"a": NewInteger(9)}, []string{"a"})
	conc := x5Map(nil, nil)
	got, uerr := unifyOptionsFamily(opts, ShapeOptions, conc, ShapeMap, nil)
	if uerr != nil {
		t.Fatalf("default fill: %v", uerr)
	}
	gm, _ := AsMap(got)
	v, _ := gm.Get("a")
	if n, _ := AsInteger(v); n != 9 {
		t.Fatalf("expected default 9, got %v", v)
	}

	// Absent key with a bare type-literal schema (no default) fails.
	req := x5Options(map[string]Value{"a": NewTypeLiteral(TInteger)}, []string{"a"})
	if _, uerr := unifyOptionsFamily(req, ShapeOptions, conc, ShapeMap, nil); uerr == nil {
		t.Fatal("missing required key must fail")
	}
}

func TestX5OptionsFieldRules(t *testing.T) {
	// Concrete schema value, matching base type: caller value wins.
	opts := x5Options(map[string]Value{"a": NewInteger(1)}, []string{"a"})
	conc := x5Map(map[string]Value{"a": NewInteger(5)}, []string{"a"})
	got, uerr := unifyOptionsFamily(opts, ShapeOptions, conc, ShapeMap, nil)
	if uerr != nil {
		t.Fatalf("same-base field: %v", uerr)
	}
	gm, _ := AsMap(got)
	v, _ := gm.Get("a")
	if n, _ := AsInteger(v); n != 5 {
		t.Fatalf("expected 5, got %v", v)
	}

	// Base-type mismatch fails with the field path.
	bad := x5Map(map[string]Value{"a": NewString("x")}, []string{"a"})
	if _, uerr := unifyOptionsFamily(opts, ShapeOptions, bad, ShapeMap, nil); uerr == nil {
		t.Fatal("base-type mismatch must fail")
	}

	// Disjunct schema: an alternative admits the value.
	dopts := x5Options(map[string]Value{
		"a": NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}),
	}, []string{"a"})
	if _, uerr = unifyOptionsFamily(dopts, ShapeOptions, conc, ShapeMap, nil); uerr != nil {
		t.Fatalf("disjunct field admit: %v", uerr)
	}
	// No alternative admits a boolean.
	bbad := x5Map(map[string]Value{"a": NewBoolean(true)}, []string{"a"})
	if _, uerr := unifyOptionsFamily(dopts, ShapeOptions, bbad, ShapeMap, nil); uerr == nil {
		t.Fatal("no disjunct alternative matches a boolean")
	}
}

// --- unify_list.go --------------------------------------------------------

func TestX5UnifyListConcretePairs(t *testing.T) {
	a := NewList([]Value{NewInteger(1), NewInteger(2)})
	b := NewList([]Value{NewInteger(1), NewInteger(2)})
	got, uerr := unifyListFamily(a, ShapeList, b, ShapeList, nil)
	if uerr != nil {
		t.Fatalf("equal lists: %v", uerr)
	}
	lst, _ := AsList(got)
	if lst.Len() != 2 {
		t.Fatalf("expected 2 elements, got %d", lst.Len())
	}

	// Length mismatch.
	c := NewList([]Value{NewInteger(1)})
	if _, uerr := unifyListFamily(a, ShapeList, c, ShapeList, nil); uerr == nil {
		t.Fatal("length mismatch must fail")
	}

	// Element mismatch.
	d := NewList([]Value{NewInteger(1), NewString("x")})
	if _, uerr := unifyListFamily(a, ShapeList, d, ShapeList, nil); uerr == nil {
		t.Fatal("element mismatch must fail")
	}
}

func TestX5UnifyListTypedOnRight(t *testing.T) {
	conc := NewList([]Value{NewInteger(1)})
	typed := NewTypedList(NewTypeLiteral(TInteger))
	got, uerr := unifyListFamily(conc, ShapeList, typed, ShapeTypedList, nil)
	if uerr != nil {
		t.Fatalf("concrete vs typed: %v", uerr)
	}
	lst, _ := AsList(got)
	if lst.Len() != 1 {
		t.Fatalf("expected 1 element, got %d", lst.Len())
	}
}

func TestX5ReparentSwappedElem(t *testing.T) {
	src := NewInteger(5)
	lit := NewTypeLiteral(TInteger)

	// No registry on the chain: reparent to the literal's type directly.
	got := reparentSwappedElem(src, lit, nil)
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("expected the source payload back, got %v", got)
	}

	// An armed chain routes the type through CanonicalType.
	r := x5reg(t)
	got = reparentSwappedElem(src, lit, r)
	if n, _ := AsInteger(got); n != 5 {
		t.Fatalf("expected the source payload back, got %v", got)
	}

	// Non-swap results pass through untouched.
	same := reparentSwappedElem(src, NewInteger(7), nil)
	if n, _ := AsInteger(same); n != 7 {
		t.Fatalf("non-swap must pass through, got %v", same)
	}
}

// --- generics_unify.go ----------------------------------------------------

func TestX5ResolveSigChildParamNested(t *testing.T) {
	r := x5reg(t)

	// Nested typed list whose inner child resolves (builtin word) —
	// rebuilds the outer container.
	inner := NewTypedList(NewWord("Integer"))
	outer := NewTypedList(inner)
	got := ResolveSigChildParam(r, outer)
	ci, err := AsChildType(got)
	if err != nil {
		t.Fatalf("resolved outer is a typed list: %v", err)
	}
	ici, err := AsChildType(ci.Child)
	if err != nil {
		t.Fatalf("resolved inner is a typed list: %v", err)
	}
	if !IsBareTypeNode(ici.Child) {
		t.Fatalf("inner child should resolve to the Integer literal, got %v", ici.Child)
	}

	// Nested typed list whose inner child does NOT resolve — the outer
	// value passes through unchanged (ExactEqual arm).
	inert := NewTypedList(NewTypedList(NewInteger(1)))
	if got := ResolveSigChildParam(r, inert); !ExactEqual(got, inert) {
		t.Fatalf("unresolvable nested child must pass through, got %v", got)
	}
}

func TestX5ResolveSigChildParamUserType(t *testing.T) {
	r := x5reg(t)
	if err := InstallType(r, "X5UT", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	v := NewTypedList(NewWord("X5UT"))
	got := ResolveSigChildParam(r, v)
	ci, err := AsChildType(got)
	if err != nil {
		t.Fatalf("resolved value is a typed list: %v", err)
	}
	if !IsBareTypeNode(ci.Child) {
		t.Fatalf("user-type child should resolve to its body, got %v", ci.Child)
	}
}

func TestX5ResolveChildTypeExprSugarAndBadParen(t *testing.T) {
	r := x5reg(t)
	r.BindSugarWord(SugarGenApply, "of")

	// A type-bound sugar child lowers to its paren form; the sub-run
	// fails (no `of` word is registered) but the sugar arm is exercised.
	sug := NewSugar(SugarInfo{Kind: SugarTypeBound, Items: []Value{NewTypeLiteral(TMap)}})
	_, _ = ResolveChildTypeExpr(r, NewTypedList(sug))

	// A paren-parented child with a non-paren payload returns v unchanged.
	badParen := NewValueRaw(TParenExpr, IntPayload{N: 1})
	v := NewTypedList(badParen)
	got, err := ResolveChildTypeExpr(r, v)
	if err != nil {
		t.Fatalf("bad paren payload returns unchanged: %v", err)
	}
	if _, cerr := AsChildType(got); cerr != nil {
		t.Fatalf("expected the typed list back, got %v", got)
	}
}

func TestX5SchemaFieldsClassArm(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("value", NewTypeLiteral(TInteger))
	body := NewClassType(TClass, ClassTypeInfo{ID: "X5SC", Name: "Class/X5SC", Fields: fields})
	got := schemaFields(&TypeSchemaInfo{Body: body})
	if got == nil || got.Len() != 1 {
		t.Fatalf("class schema fields should surface, got %v", got)
	}
}

func TestX5InferAndInstantiateSchemaDefaultBreak(t *testing.T) {
	r := x5reg(t)
	node := &Type{tmeta: &typeMeta{Name: "X5Box"}, ID: "X5Box", Parent: TMap}
	info := &TypeSchemaInfo{
		Name: "X5Box",
		Params: []GenParam{
			{Name: "T", HasDefault: true, Default: NewTypeLiteral(TInteger)},
		},
		Body: NewRecordType(NewOrderedMap()),
		Kind: SchemaRecord,
		Type: node,
	}
	schema := NewTypeSchema(node, info)
	om := NewOrderedMap()
	om.Set("value", NewInteger(1))
	// The parameter is uninferable but defaulted: the loop breaks and
	// instantiation proceeds on defaults.
	_, _ = InferAndInstantiateSchema(r, schema, NewMap(om))
}

func TestX5InstallGenBindingMapNonTypeArg(t *testing.T) {
	r := x5reg(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "X5GP"}, {Name: "X5GQ"}}}
	installed := InstallGenBindingMap(r, spec, map[string]Value{"X5GP": NewInteger(3)})
	if len(installed) != 1 || installed[0] != "X5GP" {
		t.Fatalf("expected [X5GP] installed, got %v", installed)
	}
}
