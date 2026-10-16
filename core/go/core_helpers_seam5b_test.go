package core

// Seam-5b wave E: unit tests that execute previously-unreached blocks in
// boru_error.go, code_effect.go, core_helpers.go, core_ref.go, core_xml.go,
// fork.go, storage_helpers.go, surface.go, and util.go. All calls are
// in-package and direct where possible (guard / error arms), per
// design/TEST-SEAMS.10.md. Every test uses a fresh registry so no global
// state needs restoring.

import (
	"bytes"
	"strings"
	"testing"
)

// --- boru_error.go ---------------------------------------------------------

func TestS5bEBoruErrorColDefaultsToOne(t *testing.T) {
	e := &BoruError{Code: "syntax_error", Detail: "boom", Row: 2, Col: 0}
	msg := e.Error()
	if !strings.Contains(msg, "--> 2:1") {
		t.Errorf("expected '--> 2:1' in %q", msg)
	}
}

func TestS5bEBoruErrSiteClampsRowCol(t *testing.T) {
	// row < 1 and col < 1 clamp to 1.
	out := boruErrSite("one\ntwo\nthree", "one", "msg", 0, 0)
	if out == "" {
		t.Fatal("expected a non-empty site extract")
	}
	// row beyond the line count clamps to the last line.
	out = boruErrSite("one\ntwo", "two", "msg", 99, 1)
	if out == "" {
		t.Fatal("expected a non-empty site extract for an out-of-range row")
	}
}

func TestS5bEDescribeStackTypesEdges(t *testing.T) {
	if got := describeStackTypes(NewTape(nil, 0), 0); got != "stack is empty" {
		t.Errorf("empty tape: got %q", got)
	}
	dep := NewDepScalar(DepGT, NewInteger(10))
	if !dep.IsDepScalar() {
		t.Fatal("NewDepScalar did not build a DepScalar")
	}
	long := NewString(strings.Repeat("a", 30))
	tape := NewTape([]Value{dep, long}, 0)
	got := describeStackTypes(tape, 0)
	if !strings.Contains(got, "...") {
		t.Errorf("expected the >20-char string to be truncated with ...: %q", got)
	}
	if !strings.Contains(got, "gt") {
		t.Errorf("expected the DepScalar constraint rendering in %q", got)
	}

	// A Word, an Atom, and a Float each get their own label form. The
	// pointer index is placed off this window so none is wrapped in the
	// >>> <<< pointer marker.
	wtape := NewTape([]Value{NewWord("foo"), NewAtom("bar"), NewFloat(3.5)}, 3)
	wgot := describeStackTypes(wtape, 3)
	for _, want := range []string{"word(foo)", "atom(bar)", "3.5"} {
		if !strings.Contains(wgot, want) {
			t.Errorf("expected %q in %q", want, wgot)
		}
	}
}

func TestS5bEDescribeAllSigsNil(t *testing.T) {
	if got := describeAllSigs(nil); got != "" {
		t.Errorf("describeAllSigs(nil) = %q, want empty", got)
	}
	if got := describeAllSigs(&FnDefInfo{}); got != "" {
		t.Errorf("describeAllSigs(no sigs) = %q, want empty", got)
	}
}

// --- code_effect.go -------------------------------------------------------

// --- core_helpers.go ------------------------------------------------------

func TestS5bEInstallDefFnUndefBadPayload(t *testing.T) {
	r := newTestRegistry(t)
	// Parent says FnUndef but the payload is not FnUndefInfo: InstallDef
	// must return without binding anything.
	bad := Value{d: &Datum{Parent: TFnUndef, Data: ListPayload{}}}
	InstallDef(r, "s5beundef", bad)
	if _, ok := r.Defs.Top("s5beundef"); ok {
		t.Error("bad FnUndef payload must not install a binding")
	}
}

func TestS5bEInstallDefClassTypeNaming(t *testing.T) {
	r := newTestRegistry(t)

	// Child class type: full name is Parent/Name.
	parent := &ClassTypeInfo{Name: "Ideal/S5beP"}
	child := NewClassType(TClass, ClassTypeInfo{Parent: parent, Fields: NewOrderedMap()})
	InstallDef(r, "S5beC", child)
	got, ok := r.Defs.Top("S5beC")
	if !ok {
		t.Fatal("class def not installed")
	}
	info, err := AsClassType(got)
	if err != nil {
		t.Fatalf("AsClassType: %v", err)
	}
	if info.Name != "Ideal/S5beP/S5beC" {
		t.Errorf("child class name = %q, want Ideal/S5beP/S5beC", info.Name)
	}

	// Root class type with a nil Parent *Type: def falls back to TClass.
	root := NewClassType(nil, ClassTypeInfo{Fields: NewOrderedMap()})
	InstallDef(r, "S5beR", root)
	got, ok = r.Defs.Top("S5beR")
	if !ok {
		t.Fatal("root class def not installed")
	}
	info, err = AsClassType(got)
	if err != nil {
		t.Fatalf("AsClassType: %v", err)
	}
	if info.Name != "Ideal/S5beR" {
		t.Errorf("root class name = %q, want Ideal/S5beR", info.Name)
	}
	if got.Parent() == nil || !got.Parent().Equal(TClass) {
		t.Errorf("root class def parent = %v, want TClass", got.Parent())
	}
}

func TestS5bEBuildFnBodyHandlerQuotesListParam(t *testing.T) {
	r := newTestRegistry(t)
	s := FnSig{
		Params: []FnParam{{Name: "s5bex", Type: TList}},
		Impl:   Boru([]Value{NewWord("s5bex")}),
	}
	meta := &FnFrameMeta{Name: "s5bef"}
	h := buildFnBodyHandler(r, "s5bef", s, FnDefInfo{}, meta)
	arg := NewList([]Value{NewInteger(1)})
	if IsQuoted(&arg) {
		t.Fatal("test premise: arg starts unquoted")
	}
	out, err := h([]Value{arg}, nil, nil, r)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected spliced tokens")
	}
	// The installed frame binding must carry the quoted copy.
	bound, ok := r.Defs.Top("s5bex")
	if !ok || !IsQuoted(&bound) {
		t.Errorf("list param must be installed quoted (ok=%v quoted=%v)", ok, IsQuoted(&bound))
	}
}

func TestS5bEBuildFnBodyHandlerForkDispatch(t *testing.T) {
	r := newTestRegistry(t)
	s := FnSig{
		Params:  []FnParam{{Name: "s5bev", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewWord("s5bev")}),
	}
	h := buildFnBodyHandler(r, "s5bevf", s, FnDefInfo{}, &FnFrameMeta{Name: "s5bevf"})
	// A registry FORK shares the install registry's AnalysisScopeID, so
	// the handler must run the body in the fork (branch-local bindings
	// resolve there), via CallBoru.
	fork := r.ForkConcurrent()
	out, err := h([]Value{NewInteger(42)}, nil, nil, fork)
	if err != nil {
		t.Fatalf("fork dispatch: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("fork dispatch result = %v, want [42]", out)
	}
	if n, err := AsInteger(out[0]); err != nil || n != 42 {
		t.Errorf("fork dispatch result = %v (%v), want 42", out[0], err)
	}
}

func TestS5bEBuildFnBodyHandlerArgsPushError(t *testing.T) {
	r := newTestRegistry(t)
	r.Args = nil // nil args stack: Push fails, the handler must unwind the baseline
	s := FnSig{
		Params: []FnParam{{Name: "s5bey", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("s5bey")}),
	}
	h := buildFnBodyHandler(r, "s5beg", s, FnDefInfo{}, &FnFrameMeta{Name: "s5beg"})
	if _, err := h([]Value{NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("expected an error from the nil args stack")
	}
}

func TestS5bEInstallFnDefStackOnly(t *testing.T) {
	r := newTestRegistry(t)
	InstallFnDef(r, "s5beso", FnDefInfo{
		Signatures: []Signature{{
			Params: []FnParam{{Name: "n", Type: TInteger}},
			Impl:   Boru([]Value{NewWord("n")}),
		}},
	}, true)
	fn := r.Lookup("s5beso")
	if fn == nil {
		t.Fatal("stack-only fn not installed")
	}
	found := false
	for _, s := range fn.Signatures {
		if len(s.Params) == 1 && s.BarrierPos == 0 {
			found = true
		}
	}
	if !found {
		t.Error("stack-only install must pin BarrierPos to 0")
	}
}

func TestS5bEUninstallFnSigsSkipsNonFnEntries(t *testing.T) {
	r := newTestRegistry(t)
	InstallFnDef(r, "s5bemix", FnDefInfo{
		Signatures: []Signature{{
			Params: []FnParam{{Name: "a", Type: TInteger}},
			Impl:   Boru([]Value{NewWord("a")}),
		}},
	})
	// A plain value entry ABOVE the fn: the top-down spec scan must skip
	// it (not an FnDefInfo) and still find the fn entry below.
	r.Defs.Push("s5bemix", NewInteger(7))
	UninstallFnSigs(r, "s5bemix", FnUndefInfo{
		Sigs: []FnSigSpec{{Params: []FnParam{{Name: "a", Type: TInteger}}}},
	})
	// The plain value binding must survive the targeted undef.
	top, ok := r.Defs.Top("s5bemix")
	if !ok {
		t.Fatal("value binding should remain after targeted undef")
	}
	if n, err := AsInteger(top); err != nil || n != 7 {
		t.Errorf("expected the plain value 7 to remain, got %v (%v)", top, err)
	}
}

func TestS5bECoerceBooleanAbstractList(t *testing.T) {
	if CoerceBoolean(NewCarrier(TList)) {
		t.Error("a non-concrete list carrier must be falsey")
	}
}

func TestS5bEPredicateInputTypeTwoParams(t *testing.T) {
	fn := NewFunction(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
	}}})
	if got := PredicateInputType(fn); got != nil {
		t.Errorf("two-param fn must not be a predicate type, got %v", got)
	}
}

func TestS5bEIsLiteralTypeBodyNone(t *testing.T) {
	if !IsLiteralTypeBody(NewNone()) {
		t.Error("the none value is a literal type body")
	}
}

func TestS5bESimplifyDisjunctConcreteSubsumedByLiteral(t *testing.T) {
	out := SimplifyDisjunctAlts([]Value{NewInteger(5), NewTypeLiteral(TNumber)})
	if len(out) != 1 {
		t.Fatalf("expected the concrete 5 to be subsumed by Number, got %v", out)
	}
	if !IsBareTypeNode(&out[0]) {
		t.Errorf("survivor should be the Number literal, got %v", out[0])
	}
}

func TestS5bEExpandOptionalSigsUnnamedAndBarrierClamp(t *testing.T) {
	sigs := ExpandOptionalSigs("s5beopt", []FnSig{{
		Params: []FnParam{
			{Name: "", Type: TInteger},
			{Name: "o", Type: TInteger, Optional: true},
		},
		BarrierPos: 2,
		Impl:       Boru([]Value{NewInteger(1)}),
	}})
	if len(sigs) != 2 {
		t.Fatalf("expected original + 1 expansion, got %d", len(sigs))
	}
	exp := sigs[1]
	if len(exp.Params) != 1 {
		t.Fatalf("expansion should drop the optional param, got %d params", len(exp.Params))
	}
	if exp.BarrierPos != 1 {
		t.Errorf("expanded barrier = %d, want clamp to 1", exp.BarrierPos)
	}
	// The unnamed present param is spliced via (args N dot).
	sawArgs := false
	for _, tok := range exp.Body() {
		if IsWord(&tok) {
			if w, _ := AsWord(&tok); w.Name == "args" {
				sawArgs = true
			}
		}
	}
	if !sawArgs {
		t.Error("expected the unnamed-param expansion to reference args")
	}
}

// --- core_ref.go ----------------------------------------------------------

func TestS5bEResolveRefNilAndTypeBinding(t *testing.T) {
	if _, ok := ResolveRef(nil, "x"); ok {
		t.Error("ResolveRef(nil) must fail")
	}
	r := newTestRegistry(t)
	r.Defs.PushType("S5beTy", TInteger, NewTypeLiteral(TInteger))
	v, ok := ResolveRef(r, "S5beTy")
	if !ok {
		t.Fatal("type binding should resolve")
	}
	if !IsBareTypeNode(&v) {
		t.Errorf("expected the stored type body, got %v", v)
	}
}

func s5beFnDef(barrier int) FnDefInfo {
	return FnDefInfo{
		Name: "s5beorig",
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "a", Type: TInteger}},
			BarrierPos: barrier,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return args, nil
			}),
		}},
	}
}

func TestS5bEUsurpAndRebarrierSentinelClamp(t *testing.T) {
	fn := NewFunction(s5beFnDef(-1))
	if _, ok := UsurpFunction(fn); !ok {
		t.Error("UsurpFunction should wrap a function value")
	}
	if _, ok := ForceStackFunction(fn); !ok {
		t.Error("ForceStackFunction should wrap a function value")
	}
	if _, ok := ForceArityFunction(fn, 1); !ok {
		t.Error("ForceArityFunction should wrap a function value")
	}
}

func TestS5bEFuncSigBarrier(t *testing.T) {
	if got := funcSigBarrier(NewInteger(3), 2); got != 2 {
		t.Errorf("non-fn orig: got %d, want n", got)
	}
	fn := NewFunction(s5beFnDef(-1))
	if got := funcSigBarrier(fn, 1); got != 1 {
		t.Errorf("sentinel barrier should clamp to n: got %d", got)
	}
	if got := funcSigBarrier(fn, 9); got != 9 {
		t.Errorf("no matching arity: got %d, want n", got)
	}
}

func TestS5bEDispatchHandlersClampBarrier(t *testing.T) {
	orig := NewFunction(s5beFnDef(0))
	args := []Value{NewInteger(1)}

	rh := rebarrierDispatchHandler(orig, -1)
	out, err := rh(args, nil, nil, nil)
	if err != nil || len(out) != 4 {
		t.Errorf("rebarrier out = %v (%v), want ( orig 1 )", out, err)
	}

	uh := usurpDispatchHandler(orig, 5)
	out, err = uh(args, nil, nil, nil)
	if err != nil || len(out) != 4 {
		t.Errorf("usurp out = %v (%v), want ( orig 1 )", out, err)
	}
}

// --- core_xml.go ----------------------------------------------------------

func TestS5bEXmlBehaviorEdges(t *testing.T) {
	b := xmlBehavior{}
	lit := NewTypeLiteral(TXml)
	if got := b.Format(lit); got != "Xml" {
		t.Errorf("Format(bare literal) = %q, want Xml", got)
	}
	el := NewXmlElement("a", nil, nil)
	if b.Equal(el, lit) {
		t.Error("element vs bare literal must not be equal")
	}
	if !b.Equal(lit, NewTypeLiteral(TXml)) {
		t.Error("two bare Xml literals are equal")
	}

	// Attribute-count mismatch.
	attr := NewOrderedMap()
	attr.Set("k", NewString("v"))
	withAttr := NewXmlElement("a", attr, nil)
	if b.Equal(el, withAttr) {
		t.Error("attr-count mismatch must not be equal")
	}

	// Child kind mismatch: element vs text.
	elChild := NewXmlElement("a", nil, []Value{NewXmlElement("b", nil, nil)})
	textChild := NewXmlElement("a", nil, []Value{NewString("b")})
	if b.Equal(elChild, textChild) {
		t.Error("element child vs text child must not be equal")
	}

	// Nested elements unequal.
	elChild2 := NewXmlElement("a", nil, []Value{NewXmlElement("c", nil, nil)})
	if b.Equal(elChild, elChild2) {
		t.Error("different nested elements must not be equal")
	}
	if !b.Equal(elChild, NewXmlElement("a", nil, []Value{NewXmlElement("b", nil, nil)})) {
		t.Error("identical nested elements are equal")
	}

	// Text children equal / unequal.
	t1 := NewXmlElement("a", nil, []Value{NewString("x")})
	t2 := NewXmlElement("a", nil, []Value{NewString("x")})
	t3 := NewXmlElement("a", nil, []Value{NewString("y")})
	if !b.Equal(t1, t2) {
		t.Error("same text children are equal")
	}
	if b.Equal(t1, t3) {
		t.Error("different text children are not equal")
	}
}

func TestS5bEXmlChildTextNonString(t *testing.T) {
	if got := xmlChildText(NewInteger(7)); got != "7" {
		t.Errorf("xmlChildText(7) = %q", got)
	}
}

func TestS5bEOrderedMapLenNil(t *testing.T) {
	// NewXmlElement normalizes a nil attr map, so the nil arm is only
	// reachable by direct call.
	if got := orderedMapLen(nil); got != 0 {
		t.Errorf("orderedMapLen(nil) = %d, want 0", got)
	}
}

func TestS5bEIsValidXmlName(t *testing.T) {
	if IsValidXmlName("") {
		t.Error("empty name is invalid")
	}
	if IsValidXmlName("1abc") {
		t.Error("digit-led name is invalid")
	}
	if !IsValidXmlName("a-b.c:d") {
		t.Error("a-b.c:d is a valid xml name")
	}
}

// --- fork.go ----------------------------------------------------------------

func TestS5bESyncWriterWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewSyncWriter(&buf)
	n, err := w.Write([]byte("s5be"))
	if err != nil || n != 4 || buf.String() != "s5be" {
		t.Errorf("SyncWriter.Write: n=%d err=%v buf=%q", n, err, buf.String())
	}
}

// --- storage_helpers.go -----------------------------------------------------

func TestS5bEGetKeyKinds(t *testing.T) {
	if got := GetKey(NewWord("wkey")); got != "wkey" {
		t.Errorf("word key = %q", got)
	}
	if got := GetKey(NewFloat(1.5)); got != "1.5" {
		t.Errorf("float key = %q", got)
	}
	if got := GetKey(NewBoolean(true)); got != "true" {
		t.Errorf("true key = %q", got)
	}
	if got := GetKey(NewBoolean(false)); got != "false" {
		t.Errorf("false key = %q", got)
	}
}

// --- surface.go --------------------------------------------------------------

func TestS5bESubstituteSelfReturns(t *testing.T) {
	spec := FnSigSpec{
		Params:  []FnParam{{Name: "a", Type: TSelf}},
		Returns: []*Type{TSelf, nil},
	}
	out := SubstituteSelf(spec, TInteger)
	if out.Returns[0] == nil || IDOf(out.Returns[0]) != IDOf(TInteger) {
		t.Errorf("Self return not substituted: %v", out.Returns[0])
	}
	if out.Returns[1] != nil {
		t.Error("nil return slot must stay nil")
	}
}

func TestS5bEUnifySurfaceSwappedAndUnminted(t *testing.T) {
	surf := NewValueRaw(TSurface, &SurfaceInfo{Name: "S5beS", Required: NewOrderedMap()})
	// a is NOT the surface → the operands swap; the surface has no minted
	// node → unify fails with the declare-it hint.
	if _, err := unifySurface(NewInteger(1), surf); err == nil {
		t.Error("expected a unify failure for an unminted surface")
	}
}

func TestS5bEUnifySurfaceClassTypeOperand(t *testing.T) {
	info := &SurfaceInfo{
		Name:     "S5beS2",
		Required: NewOrderedMap(),
		Conform:  map[string]bool{IDOf(TClass): true},
		Type:     TSurface,
	}
	surf := NewValueRaw(TSurface, info)
	other := NewClassType(TClass, ClassTypeInfo{Name: "Ideal/S5beK", Fields: NewOrderedMap()})
	other.SetCarrier(true) // abstract: routes to the type-containment switch
	got, err := unifySurface(surf, other)
	if err != nil {
		t.Fatalf("class type conforming via Conform set should unify: %v", err)
	}
	if !IsClassType(&got) {
		t.Errorf("unify result should be the class-type side, got %v", got)
	}
}

func TestS5bESurfaceUnifierFallbacks(t *testing.T) {
	su := &surfaceUnifier{info: &SurfaceInfo{Required: NewOrderedMap(), Conform: map[string]bool{}}}
	// A bare type literal passes through to the default walk.
	if su.Match(NewTypeLiteral(TInteger), TSurface) {
		t.Error("an Integer literal does not inhabit the surface")
	}
	// Format of a non-surface value delegates to the base behavior.
	if got := su.Format(NewInteger(3)); got != "3" {
		t.Errorf("Format fallback = %q, want 3", got)
	}
}

// --- util.go ------------------------------------------------------------------

func TestS5bEMapFieldPayloadMismatch(t *testing.T) {
	m := NewOrderedMap()
	m.Set("s", Value{d: &Datum{Parent: TString, Data: IntPayload{N: 1}}})
	m.Set("i", Value{d: &Datum{Parent: TInteger, Data: StrPayload{S: "x"}}})
	m.Set("b", Value{d: &Datum{Parent: TBoolean, Data: StrPayload{S: "x"}}})
	m.Set("f", Value{d: &Datum{Parent: TFloat, Data: StrPayload{S: "x"}}})

	if _, ok := MapFieldString(m, "s"); ok {
		t.Error("MapFieldString must fail on a non-string payload")
	}
	if _, ok := MapFieldInteger(m, "i"); ok {
		t.Error("MapFieldInteger must fail on a non-int payload")
	}
	if _, ok := MapFieldBoolean(m, "b"); ok {
		t.Error("MapFieldBoolean must fail on a non-bool payload")
	}
	if _, ok := MapFieldFloat(m, "f"); ok {
		t.Error("MapFieldFloat must fail on a non-float payload")
	}
}

func TestS5bEPredicateSandboxNilAndCheckRestore(t *testing.T) {
	if got := snapshotPredicateState(nil); got.types != nil || got.check != nil {
		t.Error("nil registry snapshot must be zero")
	}
	restorePredicateState(nil, predicateSandbox{}) // must not panic

	r := newTestRegistry(t)
	s := snapshotPredicateState(r)
	s.check = nil
	restorePredicateState(r, s)
	if r.Check != nil {
		t.Error("restoring a nil check snapshot must clear r.Check")
	}
}
