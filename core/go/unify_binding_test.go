package core

import (
	"strings"
	"testing"
)

// --- InstallType's catch-all split: alias adopt vs structural mint ---------

// An alias binding (`def Foo Integer`) ADOPTS the canonical aliased node
// (NUR093): the entry's TypeDef IS the aliased *Type, and Minted stays
// false so `undef` never retires an identity the binding did not create.
func TestBindingAliasAdoptsCanonicalNode(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "Ub0Alias", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("alias install: %v", err)
	}
	entry, ok := r.Defs.TopEntry("Ub0Alias")
	if !ok || entry.TypeDef == nil {
		t.Fatal("alias must bind a type entry")
	}
	if !entry.TypeDef.Equal(TInteger) {
		t.Errorf("alias must adopt the canonical Integer node, got %s", entry.TypeDef.Name())
	}
	if entry.Minted {
		t.Error("an adopted entry must not claim to have minted its node")
	}
	// A raw base value dispatches into the alias — the transparent
	// reading `42 is Foo` always had.
	if !NewInteger(42).Is(entry.TypeDef) {
		t.Error("42 must satisfy the adopted Integer node")
	}
}

// Aliasing a USER-minted node adopts that node — and the entry again
// records no mint, so popping the alias cannot retire the original
// binding's identity.
func TestBindingAliasAdoptsUserNode(t *testing.T) {
	r := newTestRegistry(t)
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "Ub0Nom", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("refine install: %v", err)
	}
	nomEntry, _ := r.Defs.TopEntry("Ub0Nom")
	if !nomEntry.Minted {
		t.Fatal("a refine binding mints — Minted must be true")
	}
	if err := InstallType(r, "Ub0NomAlias", NewTypeLiteral(nomEntry.TypeDef)); err != nil {
		t.Fatalf("alias-of-refine install: %v", err)
	}
	aliasEntry, _ := r.Defs.TopEntry("Ub0NomAlias")
	if aliasEntry.Minted {
		t.Error("alias of a user node must not claim the mint")
	}
	if aliasEntry.TypeDef.ID != nomEntry.TypeDef.ID {
		t.Error("alias must adopt the refine node's identity")
	}
}

// The aliased family's SubtypeNamer rule still applies on the alias arm.
type ub0Namer struct{ TypeBehavior }

func (ub0Namer) ValidateSubtypeName(name string) error {
	if !strings.HasSuffix(name, "on") {
		return &BoruError{Code: "type_error", Detail: "name " + name + " must end in -on"}
	}
	return nil
}

func TestBindingAliasNamingRuleStillApplies(t *testing.T) {
	r := newTestRegistry(t)
	fam := r.Types.MintTypeWithBehavior("Ub0Famon", TScalar, ub0Namer{TypeBehavior: DefaultBehavior})
	if err := InstallType(r, "Ub0Bad", NewTypeLiteral(fam)); err == nil ||
		!strings.Contains(err.Error(), "must end in -on") {
		t.Fatalf("alias must still pass the family naming rule, got %v", err)
	}
	if err := InstallType(r, "Ub0Goodon", NewTypeLiteral(fam)); err != nil {
		t.Fatalf("conforming alias name must install: %v", err)
	}
}

// A structural (host-type) body under a naming family still validates
// on the mint arm — the alias arm no longer exercises this return, so
// the structural side pins it itself.
type ub0HostBody struct{}

func (ub0HostBody) hostTypeBody() {}

func TestBindingStructuralNamingRuleStillApplies(t *testing.T) {
	r := newTestRegistry(t)
	fam := r.Types.MintTypeWithBehavior("Ub0Exton", TScalar, ub0Namer{TypeBehavior: DefaultBehavior})
	body := NewExtension(fam, ub0HostBody{})
	if !IsHostTypeBody(body) {
		t.Fatal("fixture must register as a host type body")
	}
	if err := InstallType(r, "Ub0BadHost", body); err == nil ||
		!strings.Contains(err.Error(), "must end in -on") {
		t.Fatalf("structural mint must pass the family naming rule, got %v", err)
	}
}

// A nil DefTable ignores an adopted push, like every other mutator.
func TestDefTablePushTypeAdoptedNil(t *testing.T) {
	var dt *DefTable
	dt.PushTypeAdopted("X", TInteger, NewTypeLiteral(TInteger))
	if dt.Has("X") {
		t.Error("nil table must ignore the push")
	}
}

// Gate 1 of DepScalarUnifier.Unify — a candidate outside the base
// family fails before the bounds check runs. Called directly: the
// dispatchUnifier LCA walk only reaches the unifier when the pair's
// lattice meet passes through the minted node, so a cross-family pair
// exercises this arm through the capability itself.
func TestDepScalarUnifyGateOne(t *testing.T) {
	u := &DepScalarUnifier{
		behaviorWrapper: behaviorWrapper{prev: DefaultBehavior},
		baseType:        TInteger,
		depInfo:         DepScalarInfo{},
		typeName:        "Ub0Gate",
	}
	if _, uerr := u.Unify(NewString("hi"), NewTypeLiteral(TInteger), nil); uerr == nil {
		t.Error("a non-base candidate must fail gate 1")
	}
}

// --- BindingBodyUnifier: singletons ----------------------------------------

func TestBindingBodySingleton(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "Ub0One", NewInteger(1)); err != nil {
		t.Fatalf("singleton install: %v", err)
	}
	node := r.LookupTypeName("Ub0One")
	u, ok := node.Behavior().(*BindingBodyUnifier)
	if !ok {
		t.Fatalf("singleton node must carry a BindingBodyUnifier, got %T", node.Behavior())
	}
	if got, _ := AsInteger(u.Body()); got != 1 {
		t.Errorf("Body() must recover the declared singleton, got %v", u.Body())
	}
	// Membership: the inhabitant passes, everything else refuses.
	if !NewInteger(1).Is(node) {
		t.Error("1 must inhabit One")
	}
	if NewInteger(2).Is(node) {
		t.Error("2 must not inhabit One")
	}
	// A bare type literal is not an inhabitant — it takes the lattice walk.
	if NewTypeLiteral(TInteger).Is(node) {
		t.Error("the Integer literal is not an inhabitant of One")
	}
	// Carrier over-approximation: dynamic, tagged-at-node, and
	// body-family carriers stay admissible; a foreign family refuses.
	if !NewDynamicCarrier(TAny).Is(node) {
		t.Error("a dynamic carrier must stay admissible")
	}
	if !NewCarrier(node).Is(node) {
		t.Error("a carrier tagged at the node must stay admissible")
	}
	if !NewCarrier(TInteger).Is(node) {
		t.Error("a base-family carrier must stay admissible")
	}
	if NewCarrier(TString).Is(node) {
		t.Error("a foreign-family carrier must refuse")
	}
}

// --- BindingBodyUnifier: record shapes -------------------------------------

func TestBindingBodyRecordShape(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	shape := NewImplicitMap(fields)
	if err := InstallType(r, "Ub0Rec", shape); err != nil {
		t.Fatalf("record-shape install: %v", err)
	}
	node := r.LookupTypeName("Ub0Rec")
	if _, ok := node.Behavior().(*BindingBodyUnifier); !ok {
		t.Fatalf("record-shape node must carry a BindingBodyUnifier, got %T", node.Behavior())
	}
	member := NewOrderedMap()
	member.Set("x", NewInteger(1))
	if !NewMap(member).Is(node) {
		t.Error("{x:1} must inhabit the record shape")
	}
	wrong := NewOrderedMap()
	wrong.Set("y", NewInteger(9))
	if NewMap(wrong).Is(node) {
		t.Error("{y:9} must not inhabit the record shape")
	}
}

// --- BindingBodyUnifier: the Unify capability ------------------------------

// Unify against the bare node admits the CANDIDATE (never the node
// literal — the typed-def swap hazard), fails definitively on a
// concrete non-member, and defers a type-level pair to the structural
// rule.
func TestBindingBodyUnifyCapability(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "Ub0Two", NewInteger(2)); err != nil {
		t.Fatalf("install: %v", err)
	}
	node := r.LookupTypeName("Ub0Two")
	lit := NewTypeLiteral(node)
	out, ok := Unify(NewInteger(2), lit)
	if !ok {
		t.Fatal("2 must unify with the Two node")
	}
	if got, _ := AsInteger(out); got != 2 {
		t.Errorf("unify must yield the candidate, got %v", out)
	}
	if _, ok := Unify(NewInteger(3), lit); ok {
		t.Error("3 must fail against the Two node definitively")
	}
	// Type-level pair: settles by the structural rule.
	if _, ok := Unify(lit, lit); !ok {
		t.Error("the node must unify with itself")
	}
}

// --- DepScalarUnifier.Unify ------------------------------------------------

func TestDepScalarUnifyCapability(t *testing.T) {
	r := newTestRegistry(t)
	body := NewDepScalar(DepGT, NewInteger(10))
	if err := InstallType(r, "Ub0Big", body); err != nil {
		t.Fatalf("depscalar install: %v", err)
	}
	node := r.LookupTypeName("Ub0Big")
	if !HasConstraintUnify(node) {
		t.Fatal("a DepScalar node must carry a constraint Unify")
	}
	lit := NewTypeLiteral(node)
	out, ok := Unify(NewInteger(100), lit)
	if !ok {
		t.Fatal("100 must unify with Big")
	}
	if got, _ := AsInteger(out); got != 100 {
		t.Errorf("unify must yield the candidate, got %v", out)
	}
	if _, ok := Unify(NewInteger(5), lit); ok {
		t.Error("5 must fail the bounds check definitively")
	}
	if _, ok := Unify(NewString("hi"), lit); ok {
		t.Error("a non-base candidate must fail gate 1")
	}
	if _, ok := Unify(lit, lit); !ok {
		t.Error("the node must unify with itself (type-level pair)")
	}
}

// --- HasConstraintUnify ----------------------------------------------------

func TestHasConstraintUnify(t *testing.T) {
	if HasConstraintUnify(nil) {
		t.Error("nil type has no constraint unify")
	}
	r := newTestRegistry(t)
	// A nominal refine node: bareRefineUnifier is neither content nor a
	// Unifier — no constraint.
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "Ub0Ref", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("refine install: %v", err)
	}
	if HasConstraintUnify(r.LookupTypeName("Ub0Ref")) {
		t.Error("a nominal refine carries no constraint unify")
	}
	// A binding-body node is content + Unifier.
	if err := InstallType(r, "Ub0Cst", NewInteger(3)); err != nil {
		t.Fatalf("singleton install: %v", err)
	}
	if !HasConstraintUnify(r.LookupTypeName("Ub0Cst")) {
		t.Error("a binding-body node carries a constraint unify")
	}
	// A Match-delegating wrapper (the `behave` shape) over a constraint
	// is still found.
	inner := r.LookupTypeName("Ub0Cst").Behavior()
	wrapped := r.Types.MintTypeWithBehavior("Ub0Wrap", TInteger,
		delegatingBehavior{inner: inner})
	if !HasConstraintUnify(wrapped) {
		t.Error("a delegating wrapper over a constraint must be found")
	}
	// A behaviorWrapper chain over a constraint is walked via Prev.
	chain := r.Types.MintTypeWithBehavior("Ub0Chain", TInteger,
		&bareRefineUnifier{behaviorWrapper: behaviorWrapper{prev: inner}})
	if !HasConstraintUnify(chain) {
		t.Error("a Prev chain over a constraint must be found")
	}
	// A plain default behavior ends the walk with no constraint.
	plain := r.Types.MintType("Ub0Plain", TInteger)
	if HasConstraintUnify(plain) {
		t.Error("a DefaultBehavior node has no constraint unify")
	}
}

// --- DefTable: adopted entries ---------------------------------------------

func TestDefTablePushTypeAdopted(t *testing.T) {
	dt := NewDefTable()
	dt.PushType("Minted", TInteger, NewTypeLiteral(TInteger))
	if e, _ := dt.TopEntry("Minted"); !e.Minted {
		t.Error("PushType must record the mint")
	}
	dt.PushTypeAdopted("Adopted", TInteger, NewTypeLiteral(TInteger))
	e, ok := dt.TopEntry("Adopted")
	if !ok || e.TypeDef == nil {
		t.Fatal("adopted entry must be a type binding")
	}
	if e.Minted {
		t.Error("PushTypeAdopted must not record a mint")
	}
	if !dt.IsType("Adopted") {
		t.Error("an adopted entry is still a type binding")
	}
	if got, _ := dt.PopEntry("Adopted"); got.Minted {
		t.Error("the popped adopted entry must keep Minted=false")
	}
}
