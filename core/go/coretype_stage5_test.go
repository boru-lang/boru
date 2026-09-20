package core

import (
	"errors"
	"strings"
	"testing"
)

// Stage-5 coverage for core_type.go: TypeOf's degenerate-root arm,
// IsRecordShape's guards, IsValueOfType's typed-container / map-as-type /
// metatype / Unify arms, validateTypeName's part-conflict wrap, and
// InstallType's Micron / class-with-parent / schema-kind / surface /
// predicate-stamping branches plus the SubtypeNamer decline arms.

// wt5NamerBehavior wraps DefaultBehavior with a SubtypeNamer capability
// that declines (or admits) every subtype name.
type wt5NamerBehavior struct {
	TypeBehavior
	err error
}

func (b wt5NamerBehavior) ValidateSubtypeName(string) error { return b.err }

// wt5MinterBehavior wraps DefaultBehavior with the MicronSubtypeMinter
// capability, recording that the mint hook ran.
type wt5MinterBehavior struct {
	TypeBehavior
	minted *bool
}

func (b wt5MinterBehavior) MintSubtypeBehavior(def *Type, info *MicronTypeInfo) TypeBehavior {
	*b.minted = true
	return DefaultBehavior
}

// --- TypeOf / IsRecordShape ------------------------------------------------

func TestWt5TypeOfDegenerateRoot(t *testing.T) {
	// Any is the top of the main hierarchy — typeof saturates.
	anyLit := NewTypeLiteral(TAny)
	got := TypeOf(anyLit)
	if !ValuesEqual(got, anyLit) {
		t.Fatalf("TypeOf(Any) = %v, want Any", got)
	}
}

func TestWt5IsRecordShapeArms(t *testing.T) {
	// The empty map is a concrete value, not a shape.
	if IsRecordShape(NewMap(NewOrderedMap())) {
		t.Fatal("empty map must not be a record shape")
	}
	// Nested record shapes are shapes.
	inner := NewOrderedMap()
	inner.Set("b", NewTypeLiteral(TInteger))
	outer := NewOrderedMap()
	outer.Set("a", NewMap(inner))
	if !IsRecordShape(NewMap(outer)) {
		t.Fatal("nested record shape must qualify")
	}
}

// --- IsValueOfType ---------------------------------------------------------

func TestWt5IsValueOfTypeTypedContainers(t *testing.T) {
	intList := NewTypedList(NewTypeLiteral(TInteger))
	if IsValueOfType(NewList([]Value{NewInteger(1), NewString("s")}), intList) {
		t.Fatal("a non-conforming element must decline the typed list")
	}

	intMap := NewTypedMap(NewTypeLiteral(TInteger))
	if !IsValueOfType(mapOf("a", NewInteger(1)), intMap) {
		t.Fatal("a conforming map must satisfy the typed map")
	}
	if IsValueOfType(mapOf("a", NewString("s")), intMap) {
		t.Fatal("a non-conforming value must decline the typed map")
	}
}

func TestWt5IsValueOfTypeMapAsType(t *testing.T) {
	shape := mapOf("a", NewTypeLiteral(TInteger))
	if IsValueOfType(NewInteger(1), shape) {
		t.Fatal("a non-map value must decline a record-shape type")
	}
	if IsValueOfType(mapOf("b", NewInteger(1)), shape) {
		t.Fatal("a missing declared key must decline")
	}
	if IsValueOfType(mapOf("a", NewString("s")), shape) {
		t.Fatal("a mismatching field type must decline")
	}
	if !IsValueOfType(mapOf("a", NewInteger(1)), shape) {
		t.Fatal("a conforming map must satisfy the record shape")
	}
}

func TestWt5IsValueOfTypeMetatypeAndUnifyFallback(t *testing.T) {
	typeLit := NewTypeLiteral(TType)
	if !IsValueOfType(NewTypeLiteral(TInteger), typeLit) {
		t.Fatal("a bare type literal IS a Type")
	}
	if IsValueOfType(NewInteger(5), typeLit) {
		t.Fatal("a concrete scalar is not a Type")
	}
	// A concrete scalar t takes the Unify fallback (singleton types).
	if !IsValueOfType(NewInteger(1), NewInteger(1)) {
		t.Fatal("a value satisfies its own singleton")
	}
	if IsValueOfType(NewInteger(2), NewInteger(1)) {
		t.Fatal("a different value must decline the singleton")
	}
}

// --- validateTypeName ------------------------------------------------------

func TestWt5InstallTypePartConflict(t *testing.T) {
	r := newTestRegistry(t)
	err := InstallType(r, "Integer", NewTypeLiteral(TString))
	if err == nil || !strings.Contains(err.Error(), "conflicts with an existing type name") {
		t.Fatalf("reusing a builtin part must be declined, got %v", err)
	}
	var be *BoruError
	if !errors.As(err, &be) || be.Code != "type_error" {
		t.Fatalf("the conflict must be wrapped in the type_error taxonomy, got %v", err)
	}
}

// --- InstallType branches --------------------------------------------------

func TestWt5InstallTypeMicronBranch(t *testing.T) {
	r := newTestRegistry(t)
	minted := false
	fam := r.Types.MintTypeWithBehavior("Wt5MicFam", TScalar,
		wt5MinterBehavior{TypeBehavior: DefaultBehavior, minted: &minted})
	body := NewValueRaw(fam, MicronTypeInfo{Fields: NewOrderedMap()})
	if err := InstallType(r, "Wt5Mic", body); err != nil {
		t.Fatalf("InstallType Micron body: %v", err)
	}
	if !minted {
		t.Fatal("the family root's MintSubtypeBehavior hook must run")
	}
	if !r.Defs.IsType("Wt5Mic") {
		t.Fatal("Micron subtype must be bound as a type")
	}
	def := r.LookupTypeName("Wt5Mic")
	if def == nil || !def.Parent.Equal(fam) {
		t.Fatalf("Micron subtype must mint under the family root, got %v", def)
	}

	// A family whose SubtypeNamer declines the name aborts the install.
	fam2 := r.Types.MintTypeWithBehavior("Wt5MicFam2", TScalar,
		wt5NamerBehavior{TypeBehavior: DefaultBehavior, err: errors.New("wt5 name declined")})
	body2 := NewValueRaw(fam2, MicronTypeInfo{Fields: NewOrderedMap()})
	if err := InstallType(r, "Wt5MicBad", body2); err == nil ||
		!strings.Contains(err.Error(), "wt5 name declined") {
		t.Fatalf("SubtypeNamer compile failure must abort the Micron install, got %v", err)
	}
}

func TestWt5InstallTypeClassWithParent(t *testing.T) {
	r := newTestRegistry(t)
	parentType := r.Types.MintType("Wt5ParCls", TClass)
	parentInfo := &ClassTypeInfo{Name: "Class/Wt5ParCls", Type: parentType, Fields: NewOrderedMap()}
	body := NewClassType(TClass, ClassTypeInfo{Fields: NewOrderedMap(), Parent: parentInfo})
	if err := InstallType(r, "Wt5KidCls", body); err != nil {
		t.Fatalf("InstallType class body: %v", err)
	}
	tb, ok := r.TopTypeBody("Wt5KidCls")
	if !ok {
		t.Fatal("class type must be bound")
	}
	info, err := AsClassType(tb)
	if err != nil || info.Name != "Class/Wt5ParCls/Wt5KidCls" {
		t.Fatalf("class name = %q (%v), want parent-prefixed", info.Name, err)
	}
	def := r.LookupTypeName("Wt5KidCls")
	if def == nil || !def.Parent.Equal(parentType) {
		t.Fatalf("class subtype must mint under the parent class node, got %v", def)
	}
}

func TestWt5InstallTypeSchemaKinds(t *testing.T) {
	r := newTestRegistry(t)

	// A class schema mints under Ideal/Class.
	clsInfo := &TypeSchemaInfo{
		Params: []GenParam{{Name: "T"}},
		Body:   NewClassType(TClass, ClassTypeInfo{Fields: NewOrderedMap()}),
		Kind:   SchemaClass,
	}
	if err := InstallType(r, "Wt5SchC", NewTypeSchema(TType, clsInfo)); err != nil {
		t.Fatalf("InstallType class schema: %v", err)
	}
	if def := r.LookupTypeName("Wt5SchC"); def == nil || !def.Parent.Equal(TClass) {
		t.Fatalf("class schema must mint under Class, got %v", def)
	}
	if clsInfo.Name != "Class/Wt5SchC" {
		t.Fatalf("class schema name = %q", clsInfo.Name)
	}

	// An fn schema mints under FunctionSignature.
	fnInfo := &TypeSchemaInfo{
		Params: []GenParam{{Name: "T"}},
		Body:   NewInteger(1),
		Kind:   SchemaFn,
	}
	if err := InstallType(r, "Wt5SchF", NewTypeSchema(TType, fnInfo)); err != nil {
		t.Fatalf("InstallType fn schema: %v", err)
	}
	if def := r.LookupTypeName("Wt5SchF"); def == nil || !def.Parent.Equal(TFnUndef) {
		t.Fatalf("fn schema must mint under FunctionSignature, got %v", def)
	}
}

func TestWt5InstallTypeSurface(t *testing.T) {
	r := newTestRegistry(t)
	info := &SurfaceInfo{Required: NewOrderedMap(), Conform: map[string]bool{}}
	if err := InstallType(r, "Wt5Surf", NewSurfaceType(TSurface, info)); err != nil {
		t.Fatalf("InstallType surface body: %v", err)
	}
	if info.Name != "Surface/Wt5Surf" {
		t.Fatalf("surface name = %q", info.Name)
	}
	def := r.LookupTypeName("Wt5Surf")
	if def == nil || !def.Parent.Equal(TSurface) {
		t.Fatalf("surface must mint under Ideal/Surface, got %v", def)
	}
	if info.Type != def {
		t.Fatal("the shared payload must carry the minted node")
	}
}

func TestWt5InstallTypePredicateStamping(t *testing.T) {
	r := newTestRegistry(t)
	// Arm detached-unit stamping so the predicate-install stamp arm runs
	// (the default compiledRuntime seam is the no-op implementation).
	r.EnableRuntimeStamping()
	defer r.DisableRuntimeStamping()

	body := NewFunction(FnDefInfo{
		Registry: r,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Impl:       Boru([]Value{NewBoolean(true)}),
			BarrierPos: 0,
		}},
	})
	if err := InstallType(r, "Wt5PredT", body); err != nil {
		t.Fatalf("InstallType predicate body: %v", err)
	}
	def := r.LookupTypeName("Wt5PredT")
	if def == nil || !def.Parent.Equal(TInteger) {
		t.Fatalf("predicate type must mint at its declared input type, got %v", def)
	}
	// The attached unifier runs the predicate at Unify call sites.
	if !NewInteger(5).Is(def) {
		t.Fatal("predicate type must admit a passing candidate")
	}
}

func TestWt5InstallTypeSubtypeNamerRejections(t *testing.T) {
	r := newTestRegistry(t)
	rejBase := r.Types.MintTypeWithBehavior("Wt5RejBase", TInteger,
		wt5NamerBehavior{TypeBehavior: DefaultBehavior, err: errors.New("wt5 subtype declined")})

	// Bare-refine prefab: the rename-and-bind path consults the namer.
	prefab := r.Types.MintRefinePrefab(rejBase)
	prefabLit := NewTypeLiteral(prefab)
	if !IsRefinePrefab(prefabLit) {
		t.Fatal("MintRefinePrefab must produce a prefab literal")
	}
	if err := InstallType(r, "Wt5RefBad", prefabLit); err == nil ||
		!strings.Contains(err.Error(), "wt5 subtype declined") {
		t.Fatalf("prefab rename must consult the namer, got %v", err)
	}

	// DepScalar under a named family: the mint consults the namer.
	ds := NewDepScalar(DepGT, NewInteger(10))
	ds.Parent = rejBase
	if err := InstallType(r, "Wt5DepBad", ds); err == nil ||
		!strings.Contains(err.Error(), "wt5 subtype declined") {
		t.Fatalf("DepScalar mint must consult the namer, got %v", err)
	}

	// Alias of a namer-carrying type: the alias mint consults it too.
	if err := InstallType(r, "Wt5AliasBad", NewTypeLiteral(rejBase)); err == nil ||
		!strings.Contains(err.Error(), "wt5 subtype declined") {
		t.Fatalf("alias mint must consult the namer, got %v", err)
	}
}
