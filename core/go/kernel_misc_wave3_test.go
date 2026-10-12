package core

import (
	"strings"
	"testing"
)

// --- typetable ---------------------------------------------------------------

func TestOriginKindString(t *testing.T) {
	if OriginBuiltin.String() != "builtin" {
		t.Errorf("OriginBuiltin = %q", OriginBuiltin.String())
	}
	if OriginUserDef.String() != "userdef" {
		t.Errorf("OriginUserDef = %q", OriginUserDef.String())
	}
	if OriginKind(42).String() != "unknown" {
		t.Errorf("OriginKind(42) = %q", OriginKind(42).String())
	}
}

func TestTypeIsNative(t *testing.T) {
	if !TInteger.IsNative() {
		t.Error("TInteger not native")
	}
	r := newTestRegistry(t)
	minted := r.Types.MintType("W3Native", TInteger)
	if minted.IsNative() {
		t.Error("minted type claims native")
	}
	var nilT *Type
	if nilT.IsNative() {
		t.Error("nil type claims native")
	}
}

func TestTypeTablePathAndPartLookups(t *testing.T) {
	if got := Builtin.Lookup(TInteger.Path()); got != TInteger {
		t.Errorf("Lookup(%q) = %v", TInteger.Path(), got)
	}
	if Builtin.Lookup("No/Such/Path") != nil {
		t.Error("bogus path resolved")
	}
	var nilTT *TypeTable
	if nilTT.Lookup("Scalar") != nil || nilTT.IsRoot("Scalar") || nilTT.KnownPart("Scalar") {
		t.Error("nil TypeTable lookups not nil-safe")
	}
	// Any is the universal lattice root (parent nil); Scalar chains to it.
	if !Builtin.IsRoot("Any") {
		t.Error("Any not a root")
	}
	if Builtin.IsRoot("Integer") {
		t.Error("Integer misread as a root")
	}
	if !Builtin.KnownPart("Integer") {
		t.Error("Integer not a known part")
	}
	if Builtin.KnownPart("Zorble") {
		t.Error("Zorble known")
	}
}

func TestTypeTableAdoptAndRetire(t *testing.T) {
	r := newTestRegistry(t)
	minted := r.Types.MintType("W3Adopt", TInteger)

	dyn := NewDynamicTypeTable()
	dyn.Adopt(minted)
	if dyn.LookupByID(IDOf(minted)) != minted {
		t.Error("adopted type not resolvable by ID")
	}
	// Idempotent: a same-ID re-adoption keeps the first pointer.
	clone := *minted
	dyn.Adopt(&clone)
	if dyn.LookupByID(IDOf(minted)) != minted {
		t.Error("re-adoption replaced the canonical pointer")
	}
	// Nil / empty-ID no-ops.
	dyn.Adopt(nil)
	dyn.Adopt(&Type{})
	var nilTT *TypeTable
	nilTT.Adopt(minted)

	// Retire removes the identity.
	dyn.Retire(minted)
	if dyn.LookupByID(IDOf(minted)) != nil {
		t.Error("retired type still resolvable")
	}
	dyn.Retire(nil)
	nilTT.Retire(minted)
}

func TestBuiltinIDForPath(t *testing.T) {
	if got := BuiltinIDForPath(TInteger.Path()); got != IDOf(TInteger) {
		t.Errorf("BuiltinIDForPath(Integer) = %q, want %q", got, IDOf(TInteger))
	}
	if BuiltinIDForPath("No/Such/Path") != "" {
		t.Error("bogus path yielded an ID")
	}
}

func TestBuiltinInitErrorPlumbing(t *testing.T) {
	// The healthy build has no init errors — pin that first.
	if err := BuiltinInitError(); err != nil {
		t.Fatalf("builtin table unhealthy: %v", err)
	}
	saved := builtinInitErrs
	defer func() { builtinInitErrs = saved }()
	recordBuiltinInitErr(errFake("first"))
	recordBuiltinInitErr(errFake("second"))
	err := BuiltinInitError()
	if err == nil || err.Error() != "first" {
		t.Errorf("BuiltinInitError = %v, want the first recorded error", err)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

// --- value.go ------------------------------------------------------------------

func TestResourceInstanceGetField(t *testing.T) {
	f := NewOrderedMap()
	f.Set("name", NewString("solar"))
	ri := ResourceInstanceInfo{Fields: f}
	v, ok := ri.GetField("name")
	if !ok {
		t.Fatal("field lost")
	}
	if s, _ := AsString(v); s != "solar" {
		t.Errorf("field = %v", v)
	}
	if _, ok := ri.GetField("nope"); ok {
		t.Error("missing field found")
	}
}

func TestSetIDSeedDeterminism(t *testing.T) {
	SetIDSeed(12345)
	a := GenerateID("t_")
	SetIDSeed(12345)
	b := GenerateID("t_")
	if a != b {
		t.Errorf("same seed produced %q and %q", a, b)
	}
	if !strings.HasPrefix(a, "t_") || len(a) != 2+12 {
		t.Errorf("ID shape wrong: %q", a)
	}
	SetIDSeed(54321)
	if c := GenerateID("t_"); c == a {
		t.Errorf("different seed reproduced %q", c)
	}
}

func TestXmlInterpAccessors(t *testing.T) {
	tmpl := XmlTmpl{Tag: "div"}
	v := NewXmlInterp(tmpl)
	if !IsXmlInterp(&v) {
		t.Fatal("NewXmlInterp not recognised")
	}
	got, err := AsXmlInterp(v)
	if err != nil || got.Tag != "div" {
		t.Errorf("AsXmlInterp = %+v, %v", got, err)
	}
	if _, err := AsXmlInterp(NewInteger(1)); err == nil {
		t.Error("AsXmlInterp of integer succeeded")
	}
}

func TestNewReachFromKeys(t *testing.T) {
	m := mapOf("a", NewInteger(1))
	rv := NewReachFromKeys(m, []Value{NewString("a")})
	if !IsReach(&rv) {
		t.Fatal("not a reach value")
	}
	info, err := AsReach(&rv)
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if info.Eval {
		t.Error("programmatic reach must be inert (Eval=false)")
	}
	if len(info.Receiver) != 1 || len(info.Segments) != 1 {
		t.Errorf("reach shape: %+v", info)
	}
	if k, _ := AsString(info.Segments[0].KeyLit); k != "a" {
		t.Errorf("segment key = %v", info.Segments[0].KeyLit)
	}
}

func TestIsBooleanPredicate(t *testing.T) {
	if !IsBoolean(addrOf(NewBoolean(true))) {
		t.Error("boolean not boolean")
	}
	if IsBoolean(addrOf(NewInteger(1))) {
		t.Error("integer boolean")
	}
	// A bare Boolean TYPE LITERAL is not a boolean VALUE: its Parent is
	// the literal's supertype, not TBoolean.
	if IsBoolean(addrOf(NewTypeLiteral(TBoolean))) {
		t.Error("Boolean literal misread as a boolean value")
	}
}

func TestFlatInstanceHelpers(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("n", NewInteger(1))
	ci := NewClassInstance(TClass, ClassInstanceInfo{Fields: fields})
	if !IsFlatInstance(ci) {
		t.Error("class instance not flat")
	}
	ri := NewValueRaw(TResource, ResourceInstanceInfo{Fields: fields})
	if !IsFlatInstance(ri) {
		t.Error("resource instance not flat")
	}
	if IsFlatInstance(NewInteger(1)) {
		t.Error("integer flat")
	}
	got, ok := FlatInstanceFields(ri)
	if !ok || got.Len() != 1 {
		t.Fatalf("FlatInstanceFields = %v, %v", got, ok)
	}
	// The returned map is a fresh copy.
	got.Set("extra", NewInteger(2))
	if fields.Len() != 1 {
		t.Error("FlatInstanceFields returned the live map")
	}
	if _, ok := FlatInstanceFields(NewInteger(1)); ok {
		t.Error("integer has instance fields")
	}
}

func TestAsFlexListAndXmlElement(t *testing.T) {
	fl := NewFlexList([]Value{NewInteger(1)})
	fd, err := AsFlexList(fl)
	if err != nil || len(fd.Elems) != 1 {
		t.Errorf("AsFlexList = %v, %v", fd, err)
	}
	if _, err := AsFlexList(NewTypeLiteral(TList)); err == nil {
		t.Error("AsFlexList of type literal succeeded")
	}
	if _, err := AsFlexList(NewList([]Value{NewInteger(1)})); err == nil {
		t.Error("AsFlexList of plain list succeeded")
	}

	x := NewXmlElement("p", nil, []Value{NewString("hi")})
	xp, err := AsXmlElement(x)
	if err != nil || xp.Tag != "p" || len(xp.Cren) != 1 {
		t.Errorf("AsXmlElement = %+v, %v", xp, err)
	}
	if _, err := AsXmlElement(NewInteger(1)); err == nil {
		t.Error("AsXmlElement of integer succeeded")
	}
}

// --- core_helpers ---------------------------------------------------------------

func TestUninstallFnSigs(t *testing.T) {
	r := covRegistry(t, nil)
	sigOf := func(ret *Type) Signature {
		return Signature{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{ret},
			Impl:       Boru([]Value{NewWord("n")}),
			BarrierPos: BarrierAllForward,
		}
	}
	InstallFnDef(r, "w3fn", FnDefInfo{Signatures: []Signature{sigOf(TInteger)}})
	InstallFnDef(r, "w3fn", FnDefInfo{Signatures: []Signature{{
		Params:     []FnParam{{Name: "s", Type: TString}},
		Returns:    []*Type{TString},
		Impl:       Boru([]Value{NewWord("s")}),
		BarrierPos: BarrierAllForward,
	}}})
	if len(r.Defs.Stack("w3fn")) != 2 {
		t.Fatalf("stack depth = %d", len(r.Defs.Stack("w3fn")))
	}
	// Remove the String overload by spec (params AND returns must match).
	UninstallFnSigs(r, "w3fn", FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Type: TString}},
		Returns: []*Type{TString},
	}}})
	stack := r.Defs.Stack("w3fn")
	if len(stack) != 1 {
		t.Fatalf("after undef: stack depth = %d", len(stack))
	}
	fd, _ := FnDefOf(stack[0].Data)
	if len(fd.OwnSigs()) != 1 || !fd.OwnSigs()[0].Params[0].Type.Equal(TInteger) {
		t.Errorf("wrong entry removed: %+v", *fd)
	}
	// A spec matching nothing is a no-op; an unbound name too.
	UninstallFnSigs(r, "w3fn", FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Type: TBoolean}},
		Returns: []*Type{TBoolean},
	}}})
	if len(r.Defs.Stack("w3fn")) != 1 {
		t.Error("no-match spec removed an entry")
	}
	UninstallFnSigs(r, "zz_unbound", FnUndefInfo{Sigs: []FnSigSpec{{}}})
}

func TestCowSetPropagatesThroughStores(t *testing.T) {
	r := newTestRegistry(t)
	// The root context store exists after InitRootContext.
	root := r.Contexts.Top()
	if root == nil {
		t.Fatal("no root context store")
	}
	CowSet(root, "greeting", NewString("hi"), r)
	got, ok := r.Contexts.Top().Get("greeting")
	if !ok {
		t.Fatal("COW set not visible via the context stack")
	}
	if s, _ := AsString(got); s != "hi" {
		t.Errorf("greeting = %v", got)
	}
	// The original layer is untouched — the write landed in a new layer.
	if _, ok := root.Data["greeting"]; ok {
		t.Error("COW mutated the original layer in place")
	}

	// A nested store propagates the write to its parent chain.
	child := &StoreInstanceInfo{TypeName: root.TypeName, Data: map[string]Value{}}
	CowSet(r.Contexts.Top(), "child", NewStoreValue(nil, child), r)
	liveChild, _ := r.Contexts.Top().Get("child")
	cs, ok := liveChild.Data.(*StoreInstanceInfo)
	if !ok {
		t.Fatalf("child not a store: %v", liveChild)
	}
	CowSet(cs, "inner", NewInteger(42), r)
	reread, _ := r.Contexts.Top().Get("child")
	rs, _ := reread.Data.(*StoreInstanceInfo)
	if rs == nil {
		t.Fatal("child store lost after nested COW")
	}
	iv, ok := rs.Get("inner")
	if !ok {
		t.Fatal("nested COW write lost")
	}
	if n, _ := AsInteger(iv); n != 42 {
		t.Errorf("inner = %v", iv)
	}
}

func TestFnDefsOverlap(t *testing.T) {
	mk := func(types ...*Type) FnDefInfo {
		ps := make([]FnParam, len(types))
		for i, tt := range types {
			ps[i] = FnParam{Name: "p", Type: tt}
		}
		return FnDefInfo{Signatures: []Signature{{Params: ps}}}
	}
	if !FnDefsOverlap(mk(TInteger, TString), mk(TInteger, TString)) {
		t.Error("identical param types do not overlap")
	}
	if FnDefsOverlap(mk(TInteger), mk(TString)) {
		t.Error("different param types overlap")
	}
	if FnDefsOverlap(mk(TInteger), mk(TInteger, TInteger)) {
		t.Error("different arities overlap")
	}
}

// --- generics_instantiate ---------------------------------------------------------

func TestSubstituteParamNode(t *testing.T) {
	r := newTestRegistry(t)
	// nil in, nil out.
	if got, err := substituteParamNode(r, nil, nil, nil); got != nil || err != nil {
		t.Errorf("nil: %v %v", got, err)
	}
	// Self resolves to the provided node.
	self := r.Types.MintType("W3Self", TInteger)
	if got, err := substituteParamNode(r, TSelf, nil, self); err != nil || got != self {
		t.Errorf("Self: %v %v", got, err)
	}
	// A plain (non-placeholder) type passes through.
	if got, err := substituteParamNode(r, TInteger, nil, nil); err != nil || got != TInteger {
		t.Errorf("plain: %v %v", got, err)
	}
	// A bound placeholder resolves to the binding's canonical node.
	p := MintTypeParam(r, GenParam{Name: "T"})
	got, err := substituteParamNode(r, p, map[string]Value{"T": NewTypeLiteral(TString)}, nil)
	if err != nil || got == nil || !got.Equal(TString) {
		t.Errorf("bound placeholder: %v %v", got, err)
	}
	// Unbound placeholder errors.
	if _, err := substituteParamNode(r, p, map[string]Value{}, nil); err == nil ||
		!strings.Contains(err.Error(), "unbound type parameter") {
		t.Errorf("unbound: %v", err)
	}
	// A structural (non-node) binding errors.
	if _, err := substituteParamNode(r, p, map[string]Value{"T": NewInteger(3)}, nil); err == nil ||
		!strings.Contains(err.Error(), "NAMED type argument") {
		t.Errorf("structural binding: %v", err)
	}
}

// --- carrier.go helper zoo -----------------------------------------------------------
