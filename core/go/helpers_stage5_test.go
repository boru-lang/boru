package core

// Stage-5 coverage tests for core_helpers.go: the installDef fn/undef
// paths (module-wrapper rebinding, overlap removal, targeted undef),
// buildFnBodyHandler's capture/generic arms, the typed-container retag
// family (RetagTypedContainerValue / RetagFlexElem /
// retagFlexChildrenInPlace / RetagTypedContainerArgs), CowDel,
// CoerceBoolean's container arms, the IsTypeBody / IsLiteralTypeBody /
// PredicateInputType / BaseValue(-ForConstraint) arms, and
// ExpandOptionalSigs' invalid-default handling. Registries are fresh per
// test; no package globals are modified.

import (
	"strings"
	"testing"
)

// --- installDef: fn bodies ------------------------------------------------

func TestStage5InstallDefFnBadPayload(t *testing.T) {
	r := newTestRegistry(t)
	// Parent says Function but the payload is not FnDefInfo: nothing binds.
	bad := Value{Parent: TFunction, Data: ListPayload{}}
	InstallDef(r, "st5badfn", bad)
	if _, ok := r.Defs.Top("st5badfn"); ok {
		t.Error("a Function-parented non-FnDefInfo body must install nothing")
	}
}

func stage5IntFn(paramType *Type) FnDefInfo {
	return FnDefInfo{
		Signatures: []Signature{{
			Params: []FnParam{{Name: "a", Type: paramType}},
			Impl:   Boru([]Value{NewWord("a")}),
		}},
	}
}

func TestStage5InstallDefOverlapRemoval(t *testing.T) {
	r := newTestRegistry(t)

	InstallDef(r, "st5re", NewFunction(stage5IntFn(TInteger)))
	InstallDef(r, "st5re", NewFunction(stage5IntFn(TString)))
	if got := len(r.Defs.Stack("st5re")); got != 2 {
		t.Fatalf("premise: two stacked entries, got %d", got)
	}

	// Redefine the Integer overload from inside a conditional body: the
	// overlapping entry is dropped (the String one is kept) and the
	// analysis marks the program uncompilable (a no-op recorder here).
	r.Check.CondBodyDepth = 1
	InstallDef(r, "st5re", NewFunction(stage5IntFn(TInteger)))
	r.Check.CondBodyDepth = 0

	stack := r.Defs.Stack("st5re")
	if len(stack) != 2 {
		t.Fatalf("post-redefinition stack depth = %d, want 2 (String kept + new Integer)", len(stack))
	}
	oldest, ok := stack[0].Data.(FnDefInfo)
	if !ok {
		t.Fatal("oldest entry is not an fn def")
	}
	sigs := oldest.OwnSigs()
	if len(sigs) != 1 || !sigs[0].Params[0].Type.Equal(TString) {
		t.Error("the non-overlapping String overload must survive the redefinition")
	}
}

func TestStage5InstallDefTargetedUndef(t *testing.T) {
	r := newTestRegistry(t)
	InstallDef(r, "st5fu", NewFunction(stage5IntFn(TInteger)))
	if _, ok := r.Defs.Top("st5fu"); !ok {
		t.Fatal("premise: fn installed")
	}
	// The FnUndef body routes InstallDef to UninstallFnSigs.
	InstallDef(r, "st5fu", NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{{Name: "a", Type: TInteger}},
	}}}))
	if _, ok := r.Defs.Top("st5fu"); ok {
		t.Error("the targeted undef must remove the matching fn entry")
	}
}

func TestStage5InstallDefModuleWrapperRebind(t *testing.T) {
	r := newTestRegistry(t)
	sub, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	sub.Register("st5inner", Signature{
		Params: []FnParam{{Name: "a", Type: TInteger}},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		}),
	})

	hooked := ""
	r.MarkReady()
	r.OnRegisterHook = func(name string) { hooked = name }

	// A trivial-delegation wrapper (all-unnamed params, body = one word,
	// carrying the sub-registry): rebinding installs the INNER native's
	// locked signatures verbatim under the new name.
	wrapper := FnDefInfo{
		Registry: sub,
		Signatures: []Signature{{
			Params: []FnParam{{Name: "", Type: TInteger}},
			Impl:   Boru([]Value{NewWord("st5inner")}),
		}},
	}
	InstallDef(r, "st5wrap", NewFunction(wrapper))

	top, ok := r.Defs.Top("st5wrap")
	if !ok {
		t.Fatal("wrapper rebinding installed nothing")
	}
	fd, ok := top.Data.(FnDefInfo)
	if !ok {
		t.Fatal("rebound entry is not an fn def")
	}
	if len(fd.Signatures) == 0 || !fd.Signatures[0].Locked {
		t.Error("the rebound entry must carry the inner native's LOCKED sigs")
	}
	if fd.Registry != sub {
		t.Error("the rebound entry must keep the sub-registry")
	}
	if hooked != "st5wrap" {
		t.Errorf("OnRegisterHook fired for %q, want st5wrap", hooked)
	}

	// A multi-sig wrapper delegating to DIFFERENT inner words is NOT
	// trivial: it takes the ordinary install path (own unlocked sigs).
	mixed := FnDefInfo{
		Registry: sub,
		Signatures: []Signature{
			{Params: []FnParam{{Name: "", Type: TInteger}}, Impl: Boru([]Value{NewWord("st5inner")})},
			{Params: []FnParam{{Name: "", Type: TString}}, Impl: Boru([]Value{NewWord("st5other")})},
		},
	}
	InstallDef(r, "st5mixed", NewFunction(mixed))
	top, ok = r.Defs.Top("st5mixed")
	if !ok {
		t.Fatal("mixed wrapper installed nothing")
	}
	fd, ok = top.Data.(FnDefInfo)
	if !ok || len(fd.Signatures) == 0 {
		t.Fatal("mixed wrapper entry is not an fn def")
	}
	if fd.Signatures[0].Locked {
		t.Error("a non-trivial wrapper must NOT adopt locked inner sigs")
	}

	// All-trivial but the inner name is absent from the sub-registry:
	// falls through to the ordinary install path.
	missing := FnDefInfo{
		Registry: sub,
		Signatures: []Signature{{
			Params: []FnParam{{Name: "", Type: TInteger}},
			Impl:   Boru([]Value{NewWord("st5missing")}),
		}},
	}
	InstallDef(r, "st5nolookup", NewFunction(missing))
	if _, ok := r.Defs.Top("st5nolookup"); !ok {
		t.Error("a wrapper whose inner is unknown still installs its own sigs")
	}
}

func TestStage5InstallFnDefRegisterHook(t *testing.T) {
	r := newTestRegistry(t)
	hooked := ""
	r.MarkReady()
	r.OnRegisterHook = func(name string) { hooked = name }
	InstallFnDef(r, "st5hook", stage5IntFn(TInteger))
	if hooked != "st5hook" {
		t.Errorf("OnRegisterHook fired for %q, want st5hook", hooked)
	}
}

func TestStage5UninstallFnSigsSkipsLocked(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("st5locked", Signature{
		Params: []FnParam{{Name: "a", Type: TInteger}},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return args, nil
		}),
	})
	before := len(r.Defs.Stack("st5locked"))
	if before == 0 {
		t.Fatal("premise: native registered")
	}
	UninstallFnSigs(r, "st5locked", FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{{Name: "a", Type: TInteger}},
	}}})
	if got := len(r.Defs.Stack("st5locked")); got != before {
		t.Errorf("locked entry count changed %d -> %d; locked sigs are never removed", before, got)
	}
}

// --- buildFnBodyHandler ---------------------------------------------------

func TestStage5BuildFnBodyHandlerLeafCaptures(t *testing.T) {
	r := newTestRegistry(t)
	s := FnSig{
		Params: []FnParam{{Name: "st5lx", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("st5lx")}),
	}
	fnDef := FnDefInfo{Captured: []CapturedBinding{{Name: "st5lc", Value: NewInteger(9)}}}
	h := buildFnBodyHandler(r, "st5leaf", s, fnDef, &FnFrameMeta{Name: "st5leaf"})
	out, err := h([]Value{NewInteger(5)}, nil, nil, r)
	if err != nil {
		t.Fatalf("leaf handler: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("leaf handler must splice tokens")
	}
	cv, ok := r.Defs.Top("st5lc")
	if !ok {
		t.Fatal("the lexical capture must be installed for the call")
	}
	if n, err := AsInteger(cv); err != nil || n != 9 {
		t.Errorf("capture = %v (%v), want 9", cv, err)
	}
	pv, ok := r.Defs.Top("st5lx")
	if !ok {
		t.Fatal("the named param must be installed")
	}
	if n, err := AsInteger(pv); err != nil || n != 5 {
		t.Errorf("param = %v (%v), want 5", pv, err)
	}
}

func TestStage5BuildFnBodyHandlerGenAndCaptures(t *testing.T) {
	r := newTestRegistry(t)
	s := FnSig{
		Params: []FnParam{{Name: "st5nx", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("st5nx")}),
	}
	// A generic fn always needs frame state: the non-leaf path installs
	// captures, snapshots the def table, and installs the (empty here)
	// inferred type-param bindings.
	fnDef := FnDefInfo{
		Gen:      &GenSpecInfo{},
		Captured: []CapturedBinding{{Name: "st5nc", Value: NewInteger(3)}},
	}
	h := buildFnBodyHandler(r, "st5gen", s, fnDef, &FnFrameMeta{Name: "st5gen", HasGen: true})
	out, err := h([]Value{NewInteger(1)}, nil, nil, r)
	if err != nil {
		t.Fatalf("gen handler: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("gen handler must splice tokens")
	}
	cv, ok := r.Defs.Top("st5nc")
	if !ok {
		t.Fatal("the capture must be installed on the frame-state path")
	}
	if n, err := AsInteger(cv); err != nil || n != 3 {
		t.Errorf("capture = %v (%v), want 3", cv, err)
	}
}

// --- typed-container retagging -------------------------------------------

func TestStage5RetagTypedContainerValueArms(t *testing.T) {
	intLit := NewTypeLiteral(TInteger)

	// Non-typed-container pattern: pass-through.
	plainPat := NewTypeLiteral(TMap)
	arg := NewInteger(1)
	if got := RetagTypedContainerValue(plainPat, arg); got.ID != arg.ID {
		t.Error("a non-container pattern must pass the arg through")
	}

	// Flex map arg: header retag in place, store shared.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	flex := NewFlexMap(om)
	pat := NewTypedMap(intLit)
	got := RetagTypedContainerValue(pat, flex)
	if ec, ok := got.ElemConstraint(); !ok || !ValueType(ec).Equal(TInteger) {
		t.Errorf("flex retag lost the element constraint: %v %v", ec, ok)
	}
	if m, err := AsMutableMap(got); err != nil || m != om {
		t.Error("flex retag must keep the SHARED backing store")
	}

	// Plain (immutable) map arg: re-unified, quote preserved.
	pom := NewOrderedMap()
	pom.Set("a", NewInteger(2))
	plain := NewMap(pom)
	plain.Quoted = true
	got = RetagTypedContainerValue(pat, plain)
	if !got.Quoted {
		t.Error("the list-arg quote must survive the unify retag")
	}
	if !got.Parent.Equal(TMap) {
		t.Errorf("unify retag parent = %v", got.Parent)
	}
}

func TestStage5RetagFlexElemNested(t *testing.T) {
	intLit := NewTypeLiteral(TInteger)

	// Nested {:{:Integer}}: the outer flex's EXISTING children are
	// retagged in place through the shared store.
	innerOm := NewOrderedMap()
	innerOm.Set("v", NewInteger(1))
	inner := NewFlexMap(innerOm)
	outerOm := NewOrderedMap()
	outerOm.Set("c1", inner)
	outer := NewFlexMap(outerOm)

	nestedPat := NewTypedMap(NewTypedMap(intLit))
	got := RetagTypedContainerValue(nestedPat, outer)
	if _, ok := got.ElemConstraint(); !ok {
		t.Fatal("outer flex lost its element constraint")
	}
	child, ok := outerOm.Get("c1")
	if !ok {
		t.Fatal("child vanished")
	}
	if ec, ok := child.ElemConstraint(); !ok || !ValueType(ec).Equal(TInteger) {
		t.Errorf("nested child not retagged in place: %v %v", ec, ok)
	}

	// Nested [:[:Integer]]: the flex-list arm.
	innerL := NewFlexList([]Value{NewInteger(1)})
	outerL := NewFlexList([]Value{innerL})
	nestedListPat := NewTypedList(NewTypedList(intLit))
	got = RetagTypedContainerValue(nestedListPat, outerL)
	if _, ok := got.ElemConstraint(); !ok {
		t.Fatal("outer flex list lost its element constraint")
	}
	fd, err := AsFlexList(outerL)
	if err != nil {
		t.Fatalf("AsFlexList: %v", err)
	}
	if ec, ok := fd.Elems[0].ElemConstraint(); !ok || !ValueType(ec).Equal(TInteger) {
		t.Errorf("nested list child not retagged in place: %v %v", ec, ok)
	}
}

func TestStage5RetagTypedContainerArgs(t *testing.T) {
	pat := NewTypedMap(NewTypeLiteral(TInteger))
	params := []FnParam{
		{Name: "m", Type: TMap, Pattern: &pat},
		{Name: "n", Type: TInteger},
	}
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	args := []Value{NewMap(om), NewInteger(2)}

	out := RetagTypedContainerArgs(params, args)
	if len(out) != 2 {
		t.Fatalf("out len = %d", len(out))
	}
	if _, ok := out[0].ElemConstraint(); !ok {
		t.Error("the typed-container arg must be retagged in the returned slice")
	}
	if _, ok := args[0].ElemConstraint(); ok {
		t.Error("copy-on-write: the caller's slice must stay untouched")
	}

	// isTypedContainerParam's pattern-bearing tail, both outcomes.
	if !isTypedContainerParam(FnParam{Pattern: &pat}) {
		t.Error("a {:T} pattern is a typed-container param")
	}
	lit := NewTypeLiteral(TInteger)
	if isTypedContainerParam(FnParam{Pattern: &lit}) {
		t.Error("a bare literal pattern is not a typed-container param")
	}
}

// --- CowDel ---------------------------------------------------------------

func TestStage5CowDelRootAndNested(t *testing.T) {
	r := newTestRegistry(t)
	root := r.Contexts.Top()
	if root == nil {
		t.Fatal("no root context store")
	}
	CowSet(root, "st5k", NewString("v"), r)
	if _, ok := r.Contexts.Top().Get("st5k"); !ok {
		t.Fatal("premise: COW set visible")
	}

	CowDel(r.Contexts.Top(), "st5k", r)
	if _, ok := r.Contexts.Top().Get("st5k"); ok {
		t.Error("a tombstoned key must read as absent")
	}
	// Deleting a never-present key is an idempotent no-op layer.
	CowDel(r.Contexts.Top(), "st5never", r)
	if _, ok := r.Contexts.Top().Get("st5never"); ok {
		t.Error("deleting an absent key must stay absent")
	}

	// Nested store: the delete propagates a fresh layer up the parent
	// chain, and the context re-binds to the propagated root.
	child := &StoreInstanceInfo{TypeName: "Object/Store", Data: map[string]Value{}}
	CowSet(r.Contexts.Top(), "st5child", NewStoreValue(nil, child), r)
	live, _ := r.Contexts.Top().Get("st5child")
	cs, ok := live.Data.(*StoreInstanceInfo)
	if !ok {
		t.Fatalf("child not a store: %v", live)
	}
	CowSet(cs, "inner", NewInteger(42), r)
	live, _ = r.Contexts.Top().Get("st5child")
	cs, _ = live.Data.(*StoreInstanceInfo)
	if cs == nil {
		t.Fatal("child lost after nested set")
	}
	CowDel(cs, "inner", r)
	reread, ok := r.Contexts.Top().Get("st5child")
	if !ok {
		t.Fatal("child binding lost after nested delete")
	}
	rs, _ := reread.Data.(*StoreInstanceInfo)
	if rs == nil {
		t.Fatal("child no longer a store after nested delete")
	}
	if _, ok := rs.Get("inner"); ok {
		t.Error("the nested delete must hide the inner key")
	}
}

// --- CoerceBoolean container arms ----------------------------------------

func TestStage5CoerceBooleanContainerArms(t *testing.T) {
	// A list-family value with a non-[]Value backing (a table type) is truthy.
	if !CoerceBoolean(NewTableType(RecordTypeInfo{Fields: NewOrderedMap()})) {
		t.Error("a non-slice list backing is truthy")
	}
	// A non-concrete map carrier is falsey.
	if CoerceBoolean(NewCarrier(TMap)) {
		t.Error("an abstract map carrier is falsey")
	}
	// A map-family value with a non-OrderedMap backing (a typed map) is truthy.
	if !CoerceBoolean(NewTypedMap(NewTypeLiteral(TInteger))) {
		t.Error("a non-OrderedMap map backing is truthy")
	}
	// An unresolved word rendering "false" keeps its boolean reading.
	if CoerceBoolean(NewWord("false")) {
		t.Error("the bare word false is falsey")
	}
}

// --- type-body predicates -------------------------------------------------

// stage5HostBody marks an ExtensionPayload as a host-Ideal type body.
type stage5HostBody struct{}

func (stage5HostBody) hostTypeBody() {}

func TestStage5IsTypeBodyArms(t *testing.T) {
	implicit := NewOrderedMap()
	implicit.Implicit = true

	cases := []struct {
		name string
		v    Value
	}{
		{"implicit map", NewMap(implicit)},
		{"options type", NewOptionsType(NewOrderedMap())},
		{"table type", NewTableType(RecordTypeInfo{Fields: NewOrderedMap()})},
		{"typed list", NewTypedList(NewTypeLiteral(TInteger))},
		{"typed map", NewTypedMap(NewTypeLiteral(TInteger))},
		{"bounded type", NewBoundedType(TInteger)},
		{"surface type", NewSurfaceType(TSurface, &SurfaceInfo{Name: "St5Surf", Required: NewOrderedMap()})},
		{"fnsig type", NewFnUndef(FnUndefInfo{})},
		{"micron type", NewValueRaw(TMicron, MicronTypeInfo{Name: "St5M"})},
		{"host type body", NewValueRaw(TAny, ExtensionPayload{Body: stage5HostBody{}})},
	}
	for _, c := range cases {
		if !IsTypeBody(c.v) {
			t.Errorf("IsTypeBody(%s) = false, want true", c.name)
		}
	}
}

func TestStage5PredicateInputTypeArms(t *testing.T) {
	if got := PredicateInputType(Value{}); got != nil {
		t.Errorf("nil-parent value: got %v", got)
	}
	if got := PredicateInputType(Value{Parent: TFunction, Data: ListPayload{}}); got != nil {
		t.Errorf("non-FnDefInfo payload: got %v", got)
	}
	undeclared := func(pt *Type) Value {
		return NewFunction(FnDefInfo{Signatures: []Signature{{
			Params: []FnParam{{Name: "x", Type: pt}},
		}}})
	}
	mk := func(pt *Type) Value { return MarkPredicateFn(undeclared(pt)) }
	if got := PredicateInputType(mk(TInteger)); got == nil || !got.Equal(TInteger) {
		t.Errorf("Integer predicate input = %v, want Integer", got)
	}
	// A one-parameter fn the author did not declare a predicate has no
	// input type: the parameter-count route is gone (NUR099).
	if got := PredicateInputType(undeclared(TInteger)); got != nil {
		t.Errorf("undeclared one-parameter fn: got %v", got)
	}
	if got := PredicateInputType(mk(nil)); got != nil {
		t.Errorf("nil param type: got %v", got)
	}
	if got := PredicateInputType(mk(TAny)); got != nil {
		t.Errorf("Any param type: got %v", got)
	}
}

func TestStage5IsLiteralTypeBodyArms(t *testing.T) {
	if !IsLiteralTypeBody(NewInteger(3)) {
		t.Error("a concrete scalar is a literal type body")
	}
	if IsLiteralTypeBody(NewTypeLiteral(TInteger)) {
		t.Error("a bare scalar literal (no payload) is not")
	}
	if !IsLiteralTypeBody(NewList([]Value{NewInteger(1)})) {
		t.Error("a concrete list is a literal type body")
	}
	if !IsLiteralTypeBody(NewMap(NewOrderedMap())) {
		t.Error("a concrete map is a literal type body")
	}
}

func TestStage5SimplifyDisjunctBareSubtype(t *testing.T) {
	out := SimplifyDisjunctAlts([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNumber)})
	if len(out) != 1 {
		t.Fatalf("Integer|Number must simplify to one alt, got %v", out)
	}
	if !ValueType(out[0]).Equal(TNumber) {
		t.Errorf("survivor = %v, want Number", out[0])
	}
}

// --- BaseValue / defaults -------------------------------------------------

func TestStage5BaseValueArms(t *testing.T) {
	cases := []struct {
		t    *Type
		want *Type
	}{
		{TFloat, TFloat},
		{TNumber, TInteger}, // Number bases to integer zero
		{TBoolean, TBoolean},
		{TList, TList},
		{TMap, TMap},
		{TNone, TNone},
		{TAtom, TAtom},
	}
	for _, c := range cases {
		v, err := BaseValue(c.t)
		if err != nil {
			t.Errorf("BaseValue(%s): %v", c.t.String(), err)
			continue
		}
		if !ValueType(v).Equal(c.want) {
			t.Errorf("BaseValue(%s) = %v, want a %s", c.t.String(), v, c.want.String())
		}
	}
	if _, err := BaseValue(TFunction); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("BaseValue(Function) error = %v, want unsupported", err)
	}
}

func TestStage5BaseValueForConstraintDisjunct(t *testing.T) {
	// The first type-literal alternative supplies the base.
	d := NewDisjunct([]Value{NewInteger(5), NewTypeLiteral(TString)})
	v, err := BaseValueForConstraint(d)
	if err != nil {
		t.Fatalf("disjunct base: %v", err)
	}
	if s, err := AsString(v); err != nil || s != "" {
		t.Errorf("disjunct base = %v (%v), want the empty string", v, err)
	}

	// No type-literal alternative: none.
	d = NewDisjunct([]Value{NewInteger(5)})
	v, err = BaseValueForConstraint(d)
	if err != nil {
		t.Fatalf("literal-free disjunct base: %v", err)
	}
	if !ValueType(v).Equal(TNone) {
		t.Errorf("literal-free disjunct base = %v, want None", v)
	}
}

func TestStage5OmittedDefaultOptions(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("retries", NewInteger(3))        // concrete default: kept
	fields.Set("mode", NewTypeLiteral(TString)) // type body: skipped
	opts := NewOptionsType(fields)
	p := FnParam{Name: "o", Type: TMap, Pattern: &opts}

	v, err := omittedDefaultValue(p)
	if err != nil {
		t.Fatalf("omittedDefaultValue: %v", err)
	}
	m, err := AsMutableMap(v)
	if err != nil {
		t.Fatalf("options default is not a map: %v", err)
	}
	if got, ok := m.Get("retries"); !ok {
		t.Error("concrete field default missing")
	} else if n, err := AsInteger(got); err != nil || n != 3 {
		t.Errorf("retries = %v (%v), want 3", got, err)
	}
	if _, ok := m.Get("mode"); ok {
		t.Error("a type-body field carries no default and must be skipped")
	}
}

// --- ExpandOptionalSigs invalid arms -------------------------------------

func TestStage5ExpandOptionalSigsUnsatisfiableDefault(t *testing.T) {
	// The omitted optional's type has no base value (Function): the
	// omission combination is skipped entirely; the present NAMED param
	// path is still walked before the failure latches.
	sigs := ExpandOptionalSigs("st5opt", []FnSig{{
		Params: []FnParam{
			{Name: "a", Type: TInteger},
			{Name: "o", Type: TFunction, Optional: true},
		},
		Impl: Boru([]Value{NewWord("a")}),
	}})
	if len(sigs) != 1 {
		t.Fatalf("an unsatisfiable omission must add no overload, got %d sigs", len(sigs))
	}
}

func TestStage5ExpandOptionalSigsDefaultFailsPattern(t *testing.T) {
	// The synthesized base (0) fails the param's own pattern (String):
	// the omission overload is skipped.
	strLit := NewTypeLiteral(TString)
	sigs := ExpandOptionalSigs("st5pat", []FnSig{{
		Params: []FnParam{
			{Name: "o", Type: TInteger, Optional: true, Pattern: &strLit},
		},
		Impl: Boru([]Value{NewInteger(1)}),
	}})
	if len(sigs) != 1 {
		t.Fatalf("a default failing its pattern must add no overload, got %d sigs", len(sigs))
	}
}

func TestStage5ExpandOptionalSigsNamedPresentParam(t *testing.T) {
	// A valid omission with a NAMED present param: the expansion body
	// references the present param by word.
	sigs := ExpandOptionalSigs("st5named", []FnSig{{
		Params: []FnParam{
			{Name: "a", Type: TInteger},
			{Name: "o", Type: TInteger, Optional: true},
		},
		Impl: Boru([]Value{NewWord("a")}),
	}})
	if len(sigs) != 2 {
		t.Fatalf("expected original + 1 expansion, got %d", len(sigs))
	}
	sawA := false
	for _, tok := range sigs[1].Body() {
		if IsWord(tok) {
			if w, err := AsWord(tok); err == nil && w.Name == "a" {
				sawA = true
			}
		}
	}
	if !sawA {
		t.Error("the expansion body must reference the named present param")
	}
}
