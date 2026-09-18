package core

import (
	"strings"
	"testing"
)

// Stage 2/3 node-membership seams, driven by core's OWN suite
// (cover-gate-core): every kind's Unify capability, the node-content
// recovery helpers, and the chain-walking recognizers are exercised
// here directly so the standalone floor holds without leaning on the
// lang corpus (which drives the same arms end-to-end).

// --- helpers ---------------------------------------------------------------

// s3DepNode installs `name (Integer gt 10)` and returns its minted node.
func s3DepNode(t *testing.T, r *Registry, name string) *Type {
	t.Helper()
	if err := InstallType(r, name, NewDepScalar(DepGT, NewInteger(10))); err != nil {
		t.Fatalf("install %s: %v", name, err)
	}
	e, _ := r.Defs.TopEntry(name)
	return e.TypeDef
}

// --- DepScalar: named-refinement content + the intersection arm ------------

func TestS3DepScalarContentShapes(t *testing.T) {
	r := newTestRegistry(t)
	body := NewDepScalar(DepGT, NewInteger(10))
	if got, ok := depScalarContent(body); !ok || !got.IsDepScalar() {
		t.Error("a raw DepScalar body is its own content")
	}
	node := s3DepNode(t, r, "S3Big")
	if got, ok := depScalarContent(NewTypeLiteral(node)); !ok || !got.IsDepScalar() {
		t.Error("a named refinement's node recovers its recorded bounds")
	}
	if _, ok := depScalarContent(NewTypeLiteral(TInteger)); ok {
		t.Error("a bare builtin node records no DepScalar content")
	}
	if _, ok := depScalarContent(NewInteger(5)); ok {
		t.Error("a concrete scalar is not DepScalar content")
	}
}

func TestS3NamedDepScalarSiblingsIntersect(t *testing.T) {
	r := newTestRegistry(t)
	a := s3DepNode(t, r, "S3A")
	if err := InstallType(r, "S3B", NewDepScalar(DepLT, NewInteger(20))); err != nil {
		t.Fatalf("install S3B: %v", err)
	}
	b, _ := r.Defs.TopEntry("S3B")
	out, ok := UnifyR(NewTypeLiteral(a), NewTypeLiteral(b.TypeDef), r)
	if !ok || !out.IsDepScalar() {
		t.Fatalf("named siblings must meet to the interval intersection, got %v (ok=%v)", out, ok)
	}
}

// --- disjunct: the node's Unify arms + the recognizer ----------------------

func TestS3DisjunctUnifierNodeArms(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "S3May", NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNone)})); err != nil {
		t.Fatalf("install disjunct: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3May")
	node := NewTypeLiteral(e.TypeDef)
	u, ok := e.TypeDef.Behavior().(*DisjunctUnifier)
	if !ok {
		t.Fatalf("disjunct node must carry a DisjunctUnifier, got %T", e.TypeDef.Behavior())
	}
	// Same-node pair settles structurally.
	if _, err := u.Unify(node, node, nil); err != nil {
		t.Errorf("node vs node must settle structurally: %v", err)
	}
	// Candidate on either side of the node.
	if out, err := u.Unify(NewInteger(5), node, nil); err != nil || !IsConcrete(out) {
		t.Errorf("5 must unify through the Integer alternative (candidate left): %v", err)
	}
	if out, err := u.Unify(node, NewInteger(5), nil); err != nil || !IsConcrete(out) {
		t.Errorf("5 must unify through the Integer alternative (candidate right): %v", err)
	}
	if _, err := u.Unify(NewString("x"), node, nil); err == nil {
		t.Error("a non-member must fail definitively")
	}
}

func TestS3IsDisjunctTypeNodeShapes(t *testing.T) {
	r := newTestRegistry(t)
	if IsDisjunctTypeNode(NewInteger(5)) {
		t.Error("a concrete value is not a disjunct node")
	}
	if IsDisjunctTypeNode(NewTypeLiteral(TInteger)) {
		t.Error("a builtin node carries no Unifier at all")
	}
	dep := s3DepNode(t, r, "S3Dep")
	if IsDisjunctTypeNode(NewTypeLiteral(dep)) {
		t.Error("a DepScalar node's Unifier is not a DisjunctUnifier")
	}
	if err := InstallType(r, "S3May2", NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNone)})); err != nil {
		t.Fatalf("install disjunct: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3May2")
	if !IsDisjunctTypeNode(NewTypeLiteral(e.TypeDef)) {
		t.Error("a named disjunct's node carries its DisjunctUnifier")
	}
}

// --- negation: the node's Unify arms ---------------------------------------

func TestS3NegationUnifierNodeArms(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "S3Not", NewNegation(NewTypeLiteral(TString))); err != nil {
		t.Fatalf("install negation: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3Not")
	node := NewTypeLiteral(e.TypeDef)
	u, ok := e.TypeDef.Behavior().(*NegationUnifier)
	if !ok {
		t.Fatalf("negation node must carry a NegationUnifier, got %T", e.TypeDef.Behavior())
	}
	if _, err := u.Unify(node, node, nil); err != nil {
		t.Errorf("node vs node must settle structurally: %v", err)
	}
	if out, err := u.Unify(node, NewInteger(5), nil); err != nil || !IsConcrete(out) {
		t.Errorf("a non-String is a member of the complement (candidate right): %v", err)
	}
	if _, err := u.Unify(NewString("x"), node, nil); err == nil {
		t.Error("a String must be refused by tnot String (candidate left)")
	}
}

// --- fn shape: the node's Unify arms ---------------------------------------

func TestS3FnUndefUnifierNodeArms(t *testing.T) {
	r := newTestRegistry(t)
	shape := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TString},
	}}})
	if err := InstallType(r, "S3Sig", shape); err != nil {
		t.Fatalf("install fn shape: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3Sig")
	node := NewTypeLiteral(e.TypeDef)
	u, ok := e.TypeDef.Behavior().(*FnUndefUnifier)
	if !ok {
		t.Fatalf("fn-shape node must carry an FnUndefUnifier, got %T", e.TypeDef.Behavior())
	}
	if _, err := u.Unify(node, node, nil); err != nil {
		t.Errorf("node vs node must settle structurally: %v", err)
	}
	// Carrier over-approximation: a Function carrier is admissible, a
	// String carrier provably is not.
	if out, err := u.Unify(node, NewCarrier(TFunction), nil); err != nil || !out.Carrier {
		t.Errorf("a Function carrier is admissible against the shape node: %v", err)
	}
	if _, err := u.Unify(NewCarrier(TString), node, nil); err == nil {
		t.Error("a String carrier can never be a function")
	}
	// Concrete candidates through the structural signature check.
	match := NewFunction(FnDefInfo{Name: "s3ok", Signatures: []Signature{{
		Args:    []*Type{TInteger},
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TString},
	}}})
	if _, err := u.Unify(match, node, nil); err != nil {
		t.Errorf("a shape-conforming fn must be admitted: %v", err)
	}
	mismatch := NewFunction(FnDefInfo{Name: "s3no", Signatures: []Signature{{
		Args:    []*Type{TBoolean},
		Params:  []FnParam{{Name: "x", Type: TBoolean}},
		Returns: []*Type{TBoolean},
	}}})
	if _, err := u.Unify(node, mismatch, nil); err == nil {
		t.Error("a non-conforming fn must be refused")
	}
}

// --- surface: the node's Unify arms ----------------------------------------

func TestS3SurfaceUnifierNodeArms(t *testing.T) {
	r := newTestRegistry(t)
	info := &SurfaceInfo{Required: NewOrderedMap(), Conform: map[string]bool{}}
	if err := InstallType(r, "S3Shape", NewSurfaceType(TSurface, info)); err != nil {
		t.Fatalf("install surface: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3Shape")
	node := NewTypeLiteral(e.TypeDef)
	u, ok := e.TypeDef.Behavior().(*surfaceUnifier)
	if !ok {
		t.Fatalf("surface node must carry a surfaceUnifier, got %T", e.TypeDef.Behavior())
	}
	exposer := r.Types.MintType("Class/S3Circle", TClass)
	info.Conform[exposer.ID] = true

	if _, err := u.Unify(node, node, nil); err != nil {
		t.Errorf("node vs node must settle structurally: %v", err)
	}
	// Type-level containment: the conformance-set walk, both orderings.
	if _, err := u.Unify(node, NewTypeLiteral(exposer), nil); err != nil {
		t.Errorf("an exposer's node satisfies the bound: %v", err)
	}
	if _, err := u.Unify(NewTypeLiteral(exposer), node, nil); err != nil {
		t.Errorf("an exposer's node satisfies the bound (swapped): %v", err)
	}
	if _, err := u.Unify(node, NewTypeLiteral(TString), nil); err == nil {
		t.Error("a non-exposer node must fail the containment walk")
	}
	// Value-level candidates route through Match's parent-chain walk.
	if _, err := u.Unify(node, NewCarrier(exposer), nil); err != nil {
		t.Errorf("a carrier tagged at an exposer conforms: %v", err)
	}
	if _, err := u.Unify(node, NewCarrier(TString), nil); err == nil {
		t.Error("a carrier outside the conformance set must be refused")
	}
}

// --- predicate: the chain-walking recognizer -------------------------------

// s3PrevWrap layers over a Behavior exposing only Prev (the kernel
// wrapper chain); s3DelegateWrap exposes only DelegatesMatchTo (the
// `behave` install shape). Both must stay transparent to the
// chain-walking recognizers and unifierOf.
type s3PrevWrap struct{ behaviorWrapper }

func (w s3PrevWrap) Match(v Value, t *Type) bool { return baseBehavior(w.prev).Match(v, t) }

type s3DelegateWrap struct {
	to TypeBehavior
}

func (d s3DelegateWrap) Match(v Value, t *Type) bool    { return baseBehavior(d.to).Match(v, t) }
func (d s3DelegateWrap) Format(v Value) string          { return baseBehavior(d.to).Format(v) }
func (d s3DelegateWrap) Equal(a, b Value) bool          { return baseBehavior(d.to).Equal(a, b) }
func (d s3DelegateWrap) DelegatesMatchTo() TypeBehavior { return d.to }

func TestS3IsPredicateTypeNodeShapes(t *testing.T) {
	r := newTestRegistry(t)
	pred := NewFunction(FnDefInfo{Name: "s3p", Signatures: []Signature{{
		Args:    []*Type{TInteger},
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TBoolean},
	}}})
	if err := InstallType(r, "S3Pos", pred); err != nil {
		t.Fatalf("install predicate: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3Pos")
	node := NewTypeLiteral(e.TypeDef)
	if !IsPredicateTypeNode(node) {
		t.Fatalf("a predicate type's node must be recognized (behavior %T)", e.TypeDef.Behavior())
	}
	if IsPredicateTypeNode(NewInteger(5)) {
		t.Error("a concrete value is not a predicate node")
	}
	if IsPredicateTypeNode(NewTypeLiteral(TInteger)) {
		t.Error("a plain builtin node is not a predicate node")
	}
	// The recognizer walks Prev chains and behave-style delegation.
	inner := e.TypeDef.Behavior()
	wrapped := r.Types.MintType("S3PosW", TInteger)
	wrapped.SetBehavior(s3PrevWrap{behaviorWrapper{prev: inner}})
	if !IsPredicateTypeNode(NewTypeLiteral(wrapped)) {
		t.Error("a kernel wrapper over the PredicateUnifier stays recognizable (Prev hop)")
	}
	delegated := r.Types.MintType("S3PosD", TInteger)
	delegated.SetBehavior(s3DelegateWrap{to: inner})
	if !IsPredicateTypeNode(NewTypeLiteral(delegated)) {
		t.Error("a behave-style delegator over the PredicateUnifier stays recognizable (delegate hop)")
	}
}

// The Any-input predicate mints under Function itself — a dispatch
// category with no concrete base (design/legacy/TYPE-REPRESENTATION.1.ignore §N3).
func TestS3AnyInputPredicateMintsUnderFunction(t *testing.T) {
	r := newTestRegistry(t)
	pred := NewFunction(FnDefInfo{Name: "s3any", Signatures: []Signature{{
		Args:    []*Type{TAny},
		Params:  []FnParam{{Name: "x", Type: TAny}},
		Returns: []*Type{TBoolean},
	}}})
	if err := InstallType(r, "S3Gate", pred); err != nil {
		t.Fatalf("install any-input predicate: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3Gate")
	if e.TypeDef == nil || !e.TypeDef.Parent.Equal(TFunction) {
		t.Fatalf("an Any-input predicate mints under Function, got %v", e.TypeDef)
	}
}

// --- unifierOf: the chain-walk arms ----------------------------------------

func TestS3UnifierOfChainWalk(t *testing.T) {
	r := newTestRegistry(t)
	dep := s3DepNode(t, r, "S3UBig")
	inner := dep.Behavior()
	if _, ok := unifierOf(inner); !ok {
		t.Fatal("the DepScalarUnifier is a Unifier")
	}
	if u, ok := unifierOf(s3PrevWrap{behaviorWrapper{prev: inner}}); !ok || u == nil {
		t.Error("unifierOf must walk a Prev chain")
	}
	if u, ok := unifierOf(s3DelegateWrap{to: inner}); !ok || u == nil {
		t.Error("unifierOf must walk a behave-style delegation")
	}
	if _, ok := unifierOf(DefaultBehavior); ok {
		t.Error("DefaultBehavior carries no Unifier")
	}
}

// --- the node-content recovery arms in the shared consumers ----------------

func TestS3NodeContentConsumerArms(t *testing.T) {
	r := newTestRegistry(t)
	big := s3DepNode(t, r, "S3CBig")
	bigLit := NewTypeLiteral(big)

	// resolveTypeOperand (via NegateType): a named refinement's operand
	// resolves to its bounds, so the complement is the interval form.
	if neg := NegateType(bigLit); IsNegation(neg) {
		t.Errorf("tnot over a named refinement must compute the interval complement, got %v", neg)
	}

	// canonTypeArg: the named argument canonicalises by its content.
	if got := canonTypeArg(bigLit); !strings.Contains(got, "gt") {
		t.Errorf("canonTypeArg must canonicalise the node by its bounds, got %q", got)
	}

	// ResolveTypeLiteralDef: the node resolves to its recorded body.
	if got := ResolveTypeLiteralDef(bigLit, r); !got.IsDepScalar() {
		t.Errorf("ResolveTypeLiteralDef must recover the recorded body, got %v", got)
	}

	// rejectsTypeLiteral: a content-bearing node is admissible exactly
	// where its declared body was; a pure nominal literal keeps the
	// rejection.
	if rejectsTypeLiteral(bigLit, TInteger) {
		t.Error("a content-bearing node must be admissible at a value slot")
	}
	if !rejectsTypeLiteral(NewTypeLiteral(TInteger), TInteger) {
		t.Error("a pure nominal literal keeps the rejection")
	}

	// MakeFieldValueR: a constraint node with a Unify capability runs
	// its rule — admitting a member, refusing a non-member.
	if out, err := MakeFieldValueR(NewInteger(50), bigLit, r); err != nil || !IsConcrete(out) {
		t.Errorf("50 satisfies the refinement field constraint: %v", err)
	}
	if _, err := MakeFieldValueR(NewInteger(5), bigLit, r); err == nil {
		t.Error("5 must be refused by the refinement field constraint")
	}

	// MakeClassFieldValue: a class-content node resolves to its schema
	// for the nested-class probe.
	cdef := r.Types.MintType("Class/S3CC", TClass)
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	cbody := NewClassType(cdef, ClassTypeInfo{Fields: fields, ID: "S3CC", Name: "Class/S3CC"})
	cdef.SetTypeBody(cbody)
	src := NewOrderedMap()
	src.Set("a", NewInteger(1))
	// The nested-class rule wants an INSTANCE, not a raw map — the
	// refusal naming the class proves the node resolved to its schema.
	if _, err := MakeClassFieldValue(NewMap(src), NewTypeLiteral(cdef), r); err == nil ||
		!strings.Contains(err.Error(), "S3CC") {
		t.Errorf("the class-content node must resolve to its schema for the nested-class probe, got %v", err)
	}
}

// IsValueOfType's node arm: catch-all content resolves (keeping the
// record-shape subset semantics), constraint-Unifier kinds keep the
// node and answer through the Behavior.
func TestS3IsValueOfTypeNodeArms(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	if err := InstallType(r, "S3VR", NewImplicitMap(fields)); err != nil {
		t.Fatalf("install record shape: %v", err)
	}
	e, _ := r.Defs.TopEntry("S3VR")
	m := NewOrderedMap()
	m.Set("x", NewInteger(1))
	m.Set("extra", NewInteger(9))
	if !IsValueOfType(NewMap(m), NewTypeLiteral(e.TypeDef)) {
		t.Error("the record-shape node keeps the subset semantics ({x:1 extra:9} conforms)")
	}
	big := s3DepNode(t, r, "S3VBig")
	if IsValueOfType(NewInteger(5), NewTypeLiteral(big)) {
		t.Error("a refinement node keeps its bounds (5 is out)")
	}
	if !IsValueOfType(NewInteger(50), NewTypeLiteral(big)) {
		t.Error("a refinement node keeps its bounds (50 is in)")
	}
	if !IsValueOfType(NewInteger(5), NewTypeLiteral(TInteger)) {
		t.Error("a plain builtin node is untouched by the resolution")
	}
}
