package core

import (
	"strings"
	"testing"
)

// zz_lane_removal_cover_test.go — the kernel paths whose core-side tests
// left with the compiler lane (they drove them through recorder fakes) and
// that the interpreter and the check pass still reach: pinned again here,
// recorder-free, so core/go's own suite keeps every reachable statement
// (ADR-008, `make cover-gate-core`).

func lrFreshID(v Value) Value {
	SetID(&v, GenerateID("lr"))
	return v
}

// lrReg is a registry with one word, lrdub (an Integer doubled), for the
// body-running seams.
func lrReg(t *testing.T) *Registry {
	t.Helper()
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(NativeFunc{Name: "lrdub", Signatures: []Signature{{
		Args: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			n, _ := args[0].AsConcreteInteger()
			return []Value{NewInteger(n * 2)}, nil
		}),
		Returns: []*Type{TInteger}, BarrierPos: -1,
	}}})
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLaneRemovalEnterSpecArm(t *testing.T) {
	r := covRegistry(t, nil)
	r.EnterSpecArm(true)()
	if r.Check.SpecArmDepth != 0 {
		t.Fatalf("a known arm takes no bracket: depth %d", r.Check.SpecArmDepth)
	}
	leave := r.EnterSpecArm(false)
	if r.Check.SpecArmDepth != 1 {
		t.Fatalf("an undecidable arm raises the depth: %d", r.Check.SpecArmDepth)
	}
	leave()
	if r.Check.SpecArmDepth != 0 {
		t.Fatalf("leaving lowers it: %d", r.Check.SpecArmDepth)
	}
}

func TestLaneRemovalFnMemberReads(t *testing.T) {
	r := newTestRegistry(t)
	c := r.Check
	fnv := NewFunction(anonFn(at(NewWord("k"), 1, 1)))
	om := NewOrderedMap()
	om.Set("c", fnv)
	om.Set("n", NewInteger(1))
	mp, lst := NewMap(om), NewList([]Value{NewInteger(0), fnv})
	out := func() []Value { return []Value{lrFreshID(NewCarrier(TAny))} }
	for _, tc := range []struct {
		name string
		word string
		args []Value
		want bool
	}{
		{"a map's fn member by atom", "dot", []Value{NewAtom("c"), mp}, true},
		{"a map's fn member by string", "getr", []Value{NewString("c"), mp}, true},
		{"a list's fn element", "get", []Value{NewInteger(1), lst}, true},
		{"a list index out of range", "get", []Value{NewInteger(5), lst}, false},
		{"a list's data element", "get", []Value{NewInteger(0), lst}, false},
		{"a map's data member", "dot", []Value{NewAtom("n"), mp}, false},
		{"a carrier key", "dot", []Value{NewCarrier(TAtom), mp}, false},
		{"a carrier container", "dot", []Value{NewAtom("c"), NewCarrier(TMap)}, false},
		{"a nil map", "dot", []Value{NewAtom("c"), {d: &Datum{Parent: TMap, Data: MapPayload{}}}}, false},
		{"an Integer key into a map", "dot", []Value{NewInteger(1), mp}, false},
		{"another word", "first", []Value{NewAtom("c"), mp}, false},
	} {
		c.FnMemberReads = nil
		res := out()
		c.NoteFnMemberRead(tc.word, tc.args, res)
		if _, ok := c.FnMemberRead(IDOf(&res[0])); ok != tc.want {
			t.Errorf("%s: tagged=%v, want %v", tc.name, ok, tc.want)
		}
	}
	c.NoteFnMemberRead("dot", []Value{NewAtom("c"), mp}, nil)
	c.NoteFnMemberRead("dot", []Value{NewAtom("c"), mp}, []Value{{d: &Datum{Parent: TAny, Carrier: true}}})
	if len(c.FnMemberReads) != 0 {
		t.Error("no result, or a result with no identity, is not tagged")
	}
}

func TestLaneRemovalJoinedDefsNarrowingAndTakenArms(t *testing.T) {
	r := w8reg(t)
	r.Check.Mode = true
	pre := NewDynamicCarrier(TAny)
	r.Defs.Push("njq", pre)
	narrowed := NewDynamicCarrierValue(NewCarrier(TList))
	SetID(&narrowed, IDOf(&pre))
	// then-only, else-only, and both-arms narrowing: no push.
	InstallJoinedDefs(r, map[string]Value{"njq": narrowed}, nil)
	InstallJoinedDefs(r, nil, map[string]Value{"njq": narrowed})
	InstallJoinedDefs(r, map[string]Value{"njq": narrowed}, map[string]Value{"njq": narrowed})
	if d := r.Defs.Depth("njq"); d != 1 {
		t.Fatalf("a narrowing-only add must not be re-pushed at the join: depth = %d, want 1", d)
	}
	// A genuine both-arms binding pushes.
	InstallJoinedDefs(r, map[string]Value{"bjq": NewCarrier(TInteger)}, map[string]Value{"bjq": NewCarrier(TFloat)})
	if d := r.Defs.Depth("bjq"); d != 1 {
		t.Fatalf("a genuine both-arms join must push: depth = %d", d)
	}
	// A constant-condition branch: the taken arm's own value, marked Taken.
	tv := NewInteger(1)
	joins := InstallTakenArmDefs(r, map[string]Value{"tka": tv}, nil)
	if len(joins) != 1 || !joins[0].Taken || !joins[0].ThenBinds || joins[0].ElseBinds || joins[0].Name != "tka" {
		t.Fatalf("a then-arm taken binding reports one Taken then-join: %+v", joins)
	}
	if got, ok := r.Defs.Top("tka"); !ok || IDOf(&got) != IDOf(&tv) {
		t.Errorf("the taken arm's binding is the arm's own value: %v/%v", got, ok)
	}
	ev := NewString("e")
	joins = InstallTakenArmDefs(r, nil, map[string]Value{"tkb": ev})
	if len(joins) != 1 || !joins[0].Taken || joins[0].ThenBinds || !joins[0].ElseBinds {
		t.Fatalf("an else-arm taken binding reports one Taken else-join: %+v", joins)
	}
}

func TestLaneRemovalCheckFnCarrierBoundName(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	if _, hit := CheckFnCarrierBoundName(r, ""); hit {
		t.Error("an empty id must miss")
	}
	if _, hit := CheckFnCarrierBoundName(r, "v1"); hit {
		t.Error("an empty table must miss")
	}
	carrier := NewCarrier(TFunction)
	SetID(&carrier, "v1")
	NoteCheckFnCarrierBind(r, "f1", carrier)
	name, hit := CheckFnCarrierBoundName(r, "v1")
	if !hit || name != "f1" {
		t.Errorf("want the bound name f1, got %q hit=%v", name, hit)
	}
	if _, hit := CheckFnCarrierBoundName(r, "v2"); hit {
		t.Error("an unbound id must miss")
	}
	// DefTop resolves the table-bound carrier under analysis.
	r.Defs.Push("d", NewInteger(7))
	e := NewTop(r)
	if got, ok := e.DefTop("f1"); !ok || !got.Carrier() || !got.Parent().ConformsTo(TFunction) {
		t.Errorf("under analysis the seat must resolve the table-bound carrier: %v %v", got, ok)
	}
	if got, ok := e.DefTop("d"); !ok || !IsConcrete(got) {
		t.Errorf("a Defs binding must resolve as itself: %v %v", got, ok)
	}
	if _, ok := e.DefTop("zz"); ok {
		t.Error("an unbound name must miss")
	}
}

func TestLaneRemovalStepWordCarrierFnIsABarrier(t *testing.T) {
	setStrict(t, true)
	// Stranded: the uncommittable parked forward below the read.
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	NoteCheckFnCarrierBind(r, "h", NewCarrier(TFunction))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0), NewWord("h")}, StackHeadroom)
	e.Pointer = 3
	err := e.stepWord(e.Tape.Ref(3))
	if err == nil || !strings.Contains(err.Error(), "strict rule") {
		t.Fatalf("a fn word read under a pending forward strands it, got %v", err)
	}
	// A gradual carrier is no barrier: it is substituted as before.
	NoteCheckFnCarrierBind(r, "g", NewDynamicCarrier(TAny))
	e = NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), NewWord("waiting"), fwdMarker("waiting", 1, 1, 0), NewWord("g")}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepWord(e.Tape.Ref(3)); err != nil {
		t.Errorf("a gradual carrier's read is not a barrier, got %v", err)
	}
	// A plain read of the bound name substitutes the carrier.
	e = NewTop(r)
	e.Tape = NewTape([]Value{NewWord("h")}, StackHeadroom)
	if err := e.stepWord(e.Tape.Ref(0)); err != nil {
		t.Fatalf("plain-check stepWord errored: %v", err)
	}
	if got := e.Tape.At(0); IsUndefined(&got) || !IsFnTypedCarrier(&got) {
		t.Errorf("the check must read the bound fn carrier, got %v", got)
	}
}

func TestLaneRemovalQuotedFnCarrierReadUnderAnalysis(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	cv := NewCarrier(TFunction)
	SetID(&cv, "lrq-1")
	SetQuoted(&cv, true)
	r.Defs.Push("lrq", cv)
	e := NewTop(r)
	e.Tape = NewTape([]Value{at(NewWord("lrq"), 1, 1)}, StackHeadroom)
	if err := e.stepWord(e.Tape.Ref(0)); err != nil {
		t.Fatalf("a quoted fn carrier's read: %v", err)
	}
	if got := e.Tape.At(0); IsQuoted(&got) {
		t.Error("a bare read of a binding that may hold a fn is the word dispatch: the quote is dropped")
	}
	if len(e.defReads["lrq-1"]) != 1 {
		t.Errorf("the read's position is noted for the def-read ledger: %v", e.defReads)
	}
}

func TestLaneRemovalBareCallContext(t *testing.T) {
	r := covRegistry(t, nil)
	w := NewWord("cadd")
	tape := func(vs ...Value) *Tape { return NewTape(vs, StackHeadroom) }
	for _, c := range []struct {
		name    string
		vs      []Value
		ptr     int
		ops     []int
		callEnd int
		want    bool
	}{
		{"tape start, tape end", []Value{NewInteger(1), w}, 1, []int{0}, 1, true},
		{"open paren beneath, close paren after", []Value{NewOpenParen(), NewInteger(1), w, NewCloseParen()}, 2, []int{1}, 2, true},
		{"open paren beneath, statement end after", []Value{NewOpenParen(), NewInteger(1), w, NewEnd()}, 2, []int{1}, 2, true},
		{"forward operand under an open paren", []Value{NewOpenParen(), w, NewInteger(1), NewCloseParen()}, 1, []int{2}, 2, true},
		{"value beneath the first operand", []Value{NewInteger(9), NewInteger(1), w}, 2, []int{1}, 2, false},
		{"value after the call", []Value{NewOpenParen(), NewInteger(1), w, NewInteger(5)}, 2, []int{1}, 2, false},
	} {
		e := NewTop(r)
		e.Tape = tape(c.vs...)
		e.Pointer = c.ptr
		if got := e.bareCallContext(c.ops, c.callEnd); got != c.want {
			t.Errorf("%s: bareCallContext = %v, want %v", c.name, got, c.want)
		}
	}
	e := NewTop(r)
	e.Tape = tape(NewInteger(1), w)
	e.Pointer = 1
	e.IsTop = false
	if e.bareCallContext([]int{0}, 1) {
		t.Error("off the top engine the shape is not bare")
	}
}

func TestLaneRemovalInlineRegionsUnderAnalysis(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	saved := AnalysisImpl.ValueCarriesCarrier
	AnalysisImpl.ValueCarriesCarrier = func(v Value) bool { return v.Carrier() }
	defer func() { AnalysisImpl.ValueCarriesCarrier = saved }()
	e := NewTop(r)
	out, err := e.runInlineCtxRegion([]Value{NewInteger(1)}, "interpolation hole", false, true)
	if err != nil || len(out) != 1 {
		t.Fatalf("a grouped inline region under analysis runs as a container sub: %v %v", out, err)
	}
	got, err := e.evalInterpString(NewInterpString([]InterpPart{{Expr: []Value{NewCarrier(TInteger)}}}))
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if !got.Carrier() || !got.Parent().Equal(TString) {
		t.Errorf("dynamic interp = %v, want String carrier", got)
	}
	child := XmlTmpl{Tag: "q", Cren: []XmlCren{
		{Kind: XmlCrenExpr, Expr: []Value{NewCarrier(TInteger)}},
		{Kind: XmlCrenExpr, Expr: []Value{NewInteger(1), NewInteger(2)}},
	}}
	tmpl := XmlTmpl{
		Tag:  "p",
		Attr: []XmlAttrTmpl{{Name: "a", Parts: []InterpPart{{Expr: []Value{NewCarrier(TInteger)}}}}},
		Cren: []XmlCren{{Kind: XmlCrenChild, Child: &child}},
	}
	xv, err := e.EvalXmlInterp(NewXmlInterp(tmpl))
	if err != nil {
		t.Fatalf("EvalXmlInterp: %v", err)
	}
	if !xv.Carrier() && !IsXmlValue(xv) {
		t.Errorf("dynamic xml interp = %v, want an Xml value or carrier", xv)
	}
}

func TestLaneRemovalStepLiteralBraidArms(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	prev := CheckBraid
	defer func() { CheckBraid = prev }()
	for _, arm := range []string{"shaped", "dynamic"} {
		CheckBraid = prev
		taken := false
		hook := func(e *Engine, _ int) bool { taken = true; e.Pointer++; return true }
		if arm == "shaped" {
			CheckBraid.TryShapedMethodDispatch = hook
		} else {
			CheckBraid.TryDynamicFnValueDispatch = hook
		}
		e := NewTop(r)
		e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
		if err := e.stepLiteral(); err != nil || !taken {
			t.Errorf("%s: the braid's %s arm takes the literal under analysis: taken=%v err=%v", arm, arm, taken, err)
		}
	}
}

func TestLaneRemovalMarkApplied(t *testing.T) {
	n := NewInteger(1)
	if m := MarkApplied(n); IsApplied(&m) || m.Data().(IntPayload).N != 1 {
		t.Error("a value that is no fn is untouched")
	}
	named := NewFunction(FnDefInfo{Name: "f", Signatures: []Signature{{}}})
	if m := MarkApplied(named); IsApplied(&m) {
		t.Error("a named fn is not marked")
	}
	anon := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{}}})
	if m := MarkApplied(anon); !IsApplied(&m) {
		t.Error("an anonymous fn whose only signatures take no argument is marked applied")
	}
}

func TestLaneRemovalRecoveryRaw(t *testing.T) {
	e := NewTop(covRegistry(t, nil))
	e.NoteRecoveryRaw(1, NewInteger(2))
	e.NoteRecoveryRaw(3, NewInteger(4))
	if len(e.recoveryRaw) != 2 {
		t.Fatalf("recovery literals noted: %v", e.recoveryRaw)
	}
	e.ClearRecoveryRaw()
	if e.recoveryRaw != nil {
		t.Error("ClearRecoveryRaw must forget the literals")
	}
}

func TestLaneRemovalInvokeBodyAndCallRegion(t *testing.T) {
	r := lrReg(t)
	out, err := InvokeBody(r, NewList([]Value{NewWord("lrdub")}), []Value{NewInteger(2)})
	if err != nil || len(out) != 1 || out[0].String() != "4" {
		t.Fatalf("InvokeBody = %v / %v", out, err)
	}
	r.RegisterNativeFunc(NativeFunc{Name: "lrcall", Signatures: []Signature{{
		Args: []*Type{TList, TAny}, NoEvalArgs: map[int]bool{0: true},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			return CallRegion(reg, "lrcall", "lrcall body", []Value{args[1]}, args[0])
		}),
		Returns: []*Type{TAny}, BarrierPos: -1,
	}}})
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
	var notes []string
	e.SetTrace(func(_, _ int, _ []Value, note string) {
		if note != "" {
			notes = append(notes, note)
		}
	})
	out, err = e.Run([]Value{NewWord("lrcall"), NewList([]Value{NewWord("lrdub")}), NewInteger(5)})
	if err != nil || len(out) != 1 || out[0].String() != "10" {
		t.Fatalf("a native's tail body runs as a call region: %v / %v", out, err)
	}
	if joined := strings.Join(notes, "\n"); !strings.Contains(joined, "lrcall body") {
		t.Errorf("the trace names the region:\n%s", joined)
	}
}

func TestLaneRemovalFnNamesAndDefCopy(t *testing.T) {
	raw := Value{d: &Datum{Parent: TFunction, Data: &FnDefInfo{Name: "f"}}}
	if HolderFnName(&raw) != "" || FnNameOf(&raw) != "f" {
		t.Errorf("an unnamed holder: holder %q, name %q", HolderFnName(&raw), FnNameOf(&raw))
	}
	held := raw
	SetFnName(&held, "g")
	SetFnName(&held, "g") // the same name again changes nothing
	if FnNameOf(&held) != "g" || HolderFnName(&held) != "g" {
		t.Errorf("a named holder: %q / %q", FnNameOf(&held), HolderFnName(&held))
	}
	if FnNameOf(&raw) != "f" || held.Data().(*FnDefInfo).Name != "f" {
		t.Error("naming one holder reached the other holder or the payload")
	}
	n := NewInteger(1)
	if FnNameOf(&n) != "" {
		t.Error("a value that is no fn has no name")
	}
	if _, ok := FnDefCopy(IntPayload{N: 1}); ok {
		t.Error("FnDefCopy over a payload that is no fn definition")
	}
	stamped := NewFunction(FnDefInfo{Anonymous: true, Signatures: []Signature{{Anonymous: true}, {}}})
	for i, sig := range stamped.Data().(*FnDefInfo).Signatures {
		if !sig.Anonymous {
			t.Errorf("signature %d of an anonymous fn is stamped anonymous", i)
		}
	}
}

func TestLaneRemovalNilReceivers(t *testing.T) {
	var c *dispatchCacheState
	if fn, ok := c.get("x", 0); fn != nil || ok {
		t.Fatalf("nil get = (%v, %v), want (nil, false)", fn, ok)
	}
	c.put("x", 0, nil)
	c.reset()
	zero := &Registry{Check: &CheckState{StepBudget: -1}}
	if zero.Lookup("anything") != nil {
		t.Fatal("zero-registry Lookup = non-nil")
	}
	var r *Registry
	if r.AnalysisScopeID() != 0 {
		t.Error("a nil registry's scope id is 0")
	}
	var dt *DefTable
	if dt.Stack("x") != nil {
		t.Error("a nil table stacks nothing")
	}
	idle := &CheckState{}
	idle.RecordDef("k", SrcPos{Row: 1, Col: 1})
	if idle.DefsDone != 0 || idle.DefsInstalled != nil {
		t.Errorf("RecordDef outside a pass records nothing: %d %v", idle.DefsDone, idle.DefsInstalled)
	}
}

func TestLaneRemovalRootDefSiteUnderAnalysis(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	InstallDef(r, "lrroot", at(NewInteger(1), 3, 5))
	if _, ok := r.Defs.Top("lrroot"); !ok {
		t.Fatal("the def is installed")
	}
}

func TestLaneRemovalHasUnknownRefinementWalks(t *testing.T) {
	unknown := NewDepScalar(DepGT, NewCarrier(TInteger))
	known := NewDepScalar(DepGT, NewInteger(1))
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		v    Value
		want bool
	}{
		{"unknown", unknown, true},
		{"known", known, false},
		{"union", NewDisjunct([]Value{NewTypeLiteral(TString), unknown}), true},
		{"known union", NewDisjunct([]Value{NewTypeLiteral(TString), known}), false},
		{"negation", NewNegation(unknown), true},
		{"typed list", NewValueRaw(TList, ChildTypeInfo{Child: unknown}), true},
		{"named", NewTypeLiteral(passNode(r, "Hn")), true},
		{"builtin", NewTypeLiteral(TInteger), false},
		{"scalar", NewInteger(3), false},
	} {
		if got := HasUnknownRefinement(tc.v); got != tc.want {
			t.Errorf("%s: HasUnknownRefinement = %v, want %v", tc.name, got, tc.want)
		}
	}
	deep := unknown
	for i := 0; i < 40; i++ {
		deep = NewNegation(deep)
	}
	if HasUnknownRefinement(deep) {
		t.Error("a runaway nesting stops the walk")
	}
}

func TestLaneRemovalUninstallType(t *testing.T) {
	if UninstallType(nil, "x") {
		t.Error("a nil registry uninstalls nothing")
	}
	r := covRegistry(t, nil)
	if UninstallType(r, "LrNotBound") {
		t.Error("an unbound name uninstalls nothing")
	}
	def := r.Types.MintType("LrT", TInteger)
	r.Defs.PushType("LrT", def, NewTypeLiteral(def))
	if !UninstallType(r, "LrT") {
		t.Fatal("a bound type is uninstalled")
	}
	if _, ok := r.Defs.Top("LrT"); ok {
		t.Error("the binding is gone")
	}
}

func TestLaneRemovalKernelFormatArms(t *testing.T) {
	ci := NewClassInstance(TClass, ClassInstanceInfo{
		TypeRef: &ClassTypeInfo{ID: "T_lrcid", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(ci); !strings.HasPrefix(got, "Ideal/Class/T_lrcid{") {
		t.Errorf("class instance = %q", got)
	}
	named := NewClassInstance(TClass, ClassInstanceInfo{
		TypeRef: &ClassTypeInfo{Name: "Ideal/Class/LrN", ID: "T_lrn", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(named); !strings.HasPrefix(got, "Ideal/Class/LrN{") {
		t.Errorf("named class instance = %q", got)
	}
	ri := NewResourceInstance(TResourceEntity, ResourceInstanceInfo{
		TypeRef: &ResourceTypeInfo{Name: "Ideal/Resource/Entity/LrR", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(ri); !strings.HasPrefix(got, "Ideal/Resource/Entity/LrR{") {
		t.Errorf("resource instance = %q", got)
	}
	ct := NewClassType(TClass, ClassTypeInfo{ID: "T_lrct", Fields: NewOrderedMap()})
	if got := kernelFormatDefault(ct); !strings.HasPrefix(got, "object<Ideal/Class/T_lrct>") {
		t.Errorf("class type = %q", got)
	}
	dis := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if got := kernelFormatDefault(NewNegation(dis)); got != "tnot (Integer tor String)" {
		t.Errorf("negation compound = %q", got)
	}
}

func TestLaneRemovalSigTypeAndFnBodiedType(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	neg := NegateType(NewTypeLiteral(TInteger))
	neg.SetCarrier(true)
	r.Defs.Push("lrneg", neg)
	got, err := EvalSigTypeExpr(r, NewParenExpr([]Value{NewWord("lrneg")}), "x")
	if err != nil || got.Carrier() || !IsNegation(got) {
		t.Fatalf("a negation the pass flagged a carrier is the type itself: %v %v", got, err)
	}
	if PredicateInputType(NewInteger(1)) != nil {
		t.Error("a value that is no fn has no predicate input type")
	}
	if isFnBodiedValue(NewTypeLiteral(TFunction)) {
		t.Error("a type literal under Function is not a fn value")
	}
	fn := NewFunction(FnDefInfo{Name: "lrk", Signatures: []Signature{{Args: []*Type{TAny}}}})
	err = InstallType(r, "LrK", fn)
	if be, ok := err.(*BoruError); !ok || be.Code != "def_error" {
		t.Fatalf("a capitalised name over a plain fn is refused at the declaration: %v", err)
	}
}

func TestLaneRemovalStepWordValReadsTheBoundCarrier(t *testing.T) {
	r := covRegistry(t, nil)
	defer r.Check.Begin()()
	NoteCheckFnCarrierBind(r, "hv", NewCarrier(TFunction))
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewWord("hv")}, StackHeadroom)
	if err := e.stepWordVal(e.Tape.At(0), WordInfo{Name: "hv", ArgCount: -1, ForceVal: true}); err != nil {
		t.Fatalf("a /v read of a table-bound name under analysis: %v", err)
	}
	if got := e.Tape.At(0); !IsFnTypedCarrier(&got) {
		t.Errorf("the read delivers the bound fn carrier as data, got %v", got)
	}
}

func TestLaneRemovalCallDriverFromGo(t *testing.T) {
	d := &callDriver{inputs: []Value{NewInteger(1)}, body: NewList([]Value{NewWord("lrdub")}), kind: "lr body"}
	in, body, more, err := d.Next(0)
	if err != nil || !more || len(in) != 1 || !body.Parent().ConformsTo(TList) {
		t.Fatalf("the driver's one iteration hands over its inputs and body: %v %v %v %v", in, body, more, err)
	}
	if _, _, more, _ := d.Next(1); more {
		t.Error("a call region is one iteration")
	}
	if d.Describe() != "lr body" {
		t.Errorf("Describe = %q", d.Describe())
	}
}
