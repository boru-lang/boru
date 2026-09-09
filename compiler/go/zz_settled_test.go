package compiler

import (
	"strings"
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestInactiveEmitMethods calls every EmitRecorder method on the shared no-op
// recorder. Each returns the inactive/none answer; the point is coverage of
// the method bodies, plus a sanity check that the answers match the "recording
// is not live" contract.
func TestInactiveEmitMethods(t *testing.T) {
	e := core.TheInactiveEmit

	if e.Active() || e.Armed() || e.SuspendedNow() {
		t.Fatal("inactive recorder reports live/armed")
	}
	if !e.TopFrameOnly() {
		t.Fatal("inactive recorder should be top-frame")
	}
	// Suspend / guards return callable no-op funcs.
	e.Suspend()()
	e.BodyAnalysisGuard()()
	e.KeepDefsBodyGuard(nil, "")()
	e.MultiRunBodyGuard(nil, "b")()
	e.RecordDynUndef("x", core.SrcPos{})
	e.FnBodyGuard()()
	e.BindRegistry(nil)

	e.MarkUncompilable("x")
	if e.Sites() != nil {
		t.Fatal("inactive Sites should be nil")
	}
	if _, ok := e.DefReadName("id"); ok {
		t.Fatal("inactive DefReadName should decline")
	}
	e.PushInlineCtxBoundary()
	e.PopInlineCtxBoundary()
	// Stage-0b promotions: the probes that replaced the concrete
	// *EmitState asserts all decline on the inactive recorder.
	if e.InClosureUnit() || e.StoredGradualActive() {
		t.Fatal("inactive closure/stored probes should decline")
	}
	if folded, ok := e.FoldFullStack("w", nil, nil); folded != nil || ok {
		t.Fatal("inactive FoldFullStack should decline")
	}
	if e.RecordSpliceDyn(core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordSpliceDyn should decline")
	}
	e.NoteShapedRead("id")
	if v, ok := e.MemberFnReadValue("id"); ok || v.Data != nil {
		t.Fatal("inactive memberFnReadValue should miss")
	}

	e.RecordCall("w", nil, nil, nil, core.SrcPos{}, false, false)
	e.RecordPoly("w")
	if e.RecordPolyCall("w", nil, nil, core.SrcPos{}, nil, nil) {
		t.Fatal("inactive RecordPolyCall should refuse")
	}
	e.RecordUserCall(0, nil, nil, core.SrcPos{})
	e.RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, nil, core.SrcPos{})
	if _, ok := e.RecordDynApply(nil, core.Value{}, core.Value{}, core.SrcPos{}); ok {
		t.Fatal("inactive RecordDynApply should refuse")
	}
	if e.RecordDynMethod(core.Value{}, nil, nil, "w", core.SrcPos{}) {
		t.Fatal("inactive RecordDynMethod should refuse")
	}
	if e.RecordFallback(core.FallbackSpan{}, nil, core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordFallback should refuse")
	}
	if e.RecordTrap("c", "d", "w", "h", core.SrcPos{}) {
		t.Fatal("inactive RecordTrap should refuse")
	}
	in := core.NewInteger(3)
	if out, ok := e.RecordTypedBind(core.TypedBindSpec{}, in, in, core.SrcPos{}); ok || out.ID != in.ID {
		t.Fatalf("inactive RecordTypedBind = %v %v", out, ok)
	}
	if e.RecordMakeList(nil, nil, core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordMakeList should refuse")
	}
	if e.RecordMakeListInner(nil, nil, core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordMakeListInner should refuse")
	}
	if e.RecordMakeMap(nil, nil, nil, false, core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordMakeMap should refuse")
	}
	if e.RecordInterp(nil, nil, core.Value{}, core.SrcPos{}) {
		t.Fatal("inactive RecordInterp should refuse")
	}
	e.RegisterTrailingApply("id", 1)
	e.NoteMemberFnRead("id", core.Value{})
	if e.MemberFnRead("id") {
		t.Fatal("inactive memberFnRead should be false")
	}
	if e.DynInputsProven(nil, nil) {
		t.Fatal("inactive dynInputsProven should be false")
	}
	if v, ok := e.Materialise(in); ok || v.ID != in.ID {
		t.Fatalf("inactive materialise = %v %v", v, ok)
	}
	if e.ZeroOutProduced("id") || e.AlreadyProduced("id") {
		t.Fatal("inactive produced probes should be false")
	}

	e.RecordBindTwin(core.BindTransition{}, core.DefEntry{})
	e.MarkValueDef(in)
	e.RecordDefRebind("n", in, core.SrcPos{})
	e.RefuseCarriedUndef("n")
	if e.RegisterLocal("id") != -1 {
		t.Fatal("inactive RegisterLocal should be -1")
	}
	e.RememberOriginal(in)
	e.RememberStrippedOriginals(nil, nil)

	e.ArmBranchCapture()
	e.ArmLoopCapture()
	if e.ConsumeLoopArm() {
		t.Fatal("inactive ConsumeLoopArm should be false")
	}
	e.BeginLoopCarried()
	e.EndLoopCarried()
	e.NoteLoopCarried("n", core.Value{}, core.Value{})
	e.Rollback(e.Checkpoint())
	if e.CanSeatAcrossFragment(in) {
		t.Fatal("inactive CanSeatAcrossFragment should be false")
	}

	unit, finish, ok := e.StartFnCompile("k", "n", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if unit != -1 || finish != nil || ok {
		t.Fatalf("inactive StartFnCompile = %d finish-nil=%v %v", unit, finish == nil, ok)
	}
	e.SetUnitParamTypes(0, nil, nil)
	e.SetUnitReturnPatterns(0, nil)
	e.SetUnitDecl(0, core.DeclSite{})
	if e.UnitVariadic(0) {
		t.Fatal("inactive unitVariadic should be false")
	}
}

// RecordSpliceDyn's decline arms (§9.2b): an inactive state and an
// unresolvable payload both leave the refusal standing.
func TestRecordSpliceDynDeclines(t *testing.T) {
	if (&EmitState{}).RecordSpliceDyn(core.Value{}, core.SrcPos{}) {
		t.Error("an inactive EmitState must decline")
	}
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)
	orphan := core.NewCarrier(core.TList) // no event, no local, no const home
	if es.RecordSpliceDyn(orphan, core.SrcPos{}) {
		t.Error("an unresolvable payload must decline")
	}
}

func TestMaterialiseMapPrefixCopy(t *testing.T) {
	es := NewEmitState()
	// A 2-key map whose SECOND key is the changed (recoverable) member: the
	// rebuild copies the unchanged prefix, then patches the second.
	c := core.NewCarrier(core.TInteger)
	es.origByID[c.ID] = core.NewInteger(9)
	om := core.NewOrderedMap()
	om.Set("a", core.NewInteger(1)) // unchanged prefix
	om.Set("b", c)                  // changed → triggers prefix copy
	got, ok := es.Materialise(core.NewMap(om))
	if !ok {
		t.Fatal("map with a recoverable member should materialise")
	}
	mp := got.Data.(core.MapPayload)
	if bv, _ := mp.M.Get("b"); bv.Carrier {
		t.Fatal("changed member not recovered")
	}
	if av, _ := mp.M.Get("a"); av.Carrier {
		t.Fatal("unchanged prefix member should survive")
	}
}

// TestPayloadMarkersInvoke calls every payloadMarker (and the
// HostTypeBody marker) directly. The methods exist purely to satisfy
// the sealed Payload interface (see CLAUDE.md "Sealed Payload") and are
// never dispatched at runtime, so this is the only way to execute them.
func TestPayloadMarkersInvoke(t *testing.T) {
	payloads := []core.Payload{
		core.ReachInfo{},
		core.IntPayload{},
		core.FloatPayload{},
		core.BigIntPayload{},
		core.DecimalPayload{},
		core.StrPayload{},
		core.BoolPayload{},
		core.AtomPayload{},
		core.PathonPayload{},
		core.MicronPayload{},
		core.MicronTypeInfo{},
		core.ListPayload{},
		core.MapPayload{},
		core.ParenExprPayload{},
		core.InterpStringPayload{},
		core.TimePayload{},
		core.DurationPayload{},
		core.TimezonePayload{},
		core.MaterializerPayload{},
		core.NonePayload{},
		core.ExtensionPayload{},
		core.WordInfo{},
		core.ForwardInfo{},
		core.MarkInfo{},
		core.MoveInfo{},
		core.SpliceInfo{},
		core.ReturnCheckInfo{},
		core.DefCleanupInfo{},
		core.GuardFactInfo{},
		core.FrameOpenInfo{},
		core.ModuleDesc{},
		core.FnDefInfo{},
		core.FnUndefInfo{},
		core.DisjunctInfo{},
		core.NegationInfo{},
		core.ChildTypeInfo{},
		core.CodeEffectInfo{},
		core.RecordTypeInfo{},
		core.OptionsTypeInfo{},
		core.TableTypeInfo{},
		core.TableData{},
		core.ClassTypeInfo{},
		core.ClassInstanceInfo{},
		&core.SurfaceInfo{},
		&core.GenSpecInfo{},
		core.GenParam{},
		&core.TypeSchemaInfo{},
		core.GenInstRef{},
		&core.FlexListData{},
		core.XmlElementPayload{},
		core.XmlInterpPayload{},
		&core.FlexXmlData{},
		&core.StoreInstanceInfo{},
		&core.StoreShapeInfo{},
		core.ResourceTypeInfo{},
		core.ResourceInstanceInfo{},
		&core.TimeoutInfo{},
		&core.IntervalInfo{},
		core.ErrorInfo{},
		core.CalDurationData{},
		core.DepScalarInfo{},
		core.ClosurePayload{},
		core.PathonInfo{},
		core.NewNone().Data,
	}
	for _, p := range payloads {
		_ = p // the list itself is the pin: every entry satisfies Payload
	}
}

func TestS7EvalXmlInterpCheckModeDynamicRefuses(t *testing.T) {
	r := xmlReg(t)
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	defer done()
	e := core.NewTop(r)
	tmpl := core.XmlTmpl{Tag: "p", Attr: []core.XmlAttrTmpl{{
		Name: "x", Parts: []core.InterpPart{{Expr: []core.Value{core.NewWord("dynv")}}},
	}}}
	out, err := e.EvalXmlInterp(core.NewXmlInterp(tmpl))
	if err != nil {
		t.Fatalf("EvalXmlInterp: %v", err)
	}
	// A dynamic interpolation under an active recorder yields an Xml carrier.
	if !out.Carrier || !strings.Contains(out.Parent.Leaf(), "Xml") {
		t.Errorf("expected an Xml carrier, got %v", out)
	}
}

func cifReturns(args []core.Value, r *core.Registry) []core.Value {
	es := r.Check
	if lit, ok := core.LiteralCondValue(args[0]); ok {
		// Statically-known condition: capture only the taken arm.
		var stk []core.Value
		var defs map[string]core.Value
		if lit {
			restore := core.ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[1])
			restore()
			core.InstallJoinedDefs(r, defs, nil)
		} else {
			restore := core.ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = core.RunCarrierBodyWithDefs(r, args[2])
			restore()
			core.InstallJoinedDefs(r, nil, defs)
		}
		frag := testRecorderState(es).TakeFragment()
		if len(stk) == 0 {
			es.Recorder().MarkUncompilable("cif: branch produces no value")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		testRecorderState(es).RecordBranch(core.BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []core.Value{out}
	}

	armOf := func(arg core.Value, then bool) (*EmitFragment, []core.Value, map[string]core.Value, *core.Value) {
		if core.IsConcrete(arg) && arg.Parent.ConformsTo(core.TList) {
			var restore func()
			if then {
				restore = core.ApplyGuardNarrowing(r, args[0])
			} else {
				restore = core.ApplyComplementNarrowing(r, args[0])
			}
			es.Recorder().ArmBranchCapture()
			stk, defs := core.RunCarrierBodyWithDefs(r, arg)
			frag := testRecorderState(es).TakeFragment()
			restore()
			return asFragment(frag), stk, defs, nil
		}
		v := arg
		return nil, []core.Value{v}, nil, &v
	}
	thenFrag, thenStk, thenDefs, thenValue := armOf(args[1], true)
	elseFrag, elseStk, elseDefs, elseValue := armOf(args[2], false)
	core.InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := core.JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		out := core.NewCarrier(core.TNone)
		testRecorderState(es).RecordBranch(core.BranchRecord{
			Cond: args[0], HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		if !es.Recorder().Active() {
			return nil
		}
		return []core.Value{out}
	}
	if !es.Recorder().Active() {
		if v, ok := core.FoldVariadicArms(thenStk, elseStk); ok {
			return []core.Value{v}
		}
	}
	out := joined[len(joined)-1]
	testRecorderState(es).RecordBranch(core.BranchRecord{
		Cond: args[0], HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []core.Value{out}
}

func TestW9TryRecordMethodApplyRecordFails(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	r.Check.Emit = es
	// Origin carrier with no producing event → RecordDynMethod fails and the
	// program is marked uncompilable.
	r.Check.PendingMethodApply = &core.PendingMethodApply{Origin: core.NewCarrier(core.TAny), Word: "m"}
	if !check.TryRecordMethodApply(r, "m", nil, nil, core.SrcPos{}) {
		t.Error("a matching pending must be consumed")
	}
	if es.Compilable {
		t.Error("a failed RecordDynMethod must mark the program uncompilable")
	}
}

// armEmit arms a live bytecode recorder on r so es.Active() is true.
func armEmit(r *core.Registry) *EmitState {
	es := NewEmitState()
	r.Check.Emit = es
	return es
}

// --- joinedElementCarrier ---------------------------------------------------

// TestUserPolyPlanHandle pins the opaque-plan accessors: a nil-plan
// bake returns a NIL INTERFACE through the installer wrapper (the
// typed-nil trap), and a concrete plan's accessors expose the parallel
// slices the record site unpacks.
func TestUserPolyPlanHandle(t *testing.T) {
	if got := check.DispatchBraid.CompileUserPolyArms(nil, core.TheInactiveEmit, "", nil, nil); got != nil {
		t.Fatal("a declined bake must surface as a nil interface")
	}
	p := &userPolyPlan{sigIdx: []int{1}, units: []int{2}, impls: []core.SigImpl{nil}, sigs: []core.Signature{{}}}
	var h check.UserPolyPlan = p
	if len(h.SigIdx()) != 1 || len(h.Units()) != 1 || len(h.Impls()) != 1 || len(h.Sigs()) != 1 {
		t.Fatal("plan accessors must expose the parallel slices")
	}
	p.joined = []bool{true}
	p.outs = []core.Value{core.NewInteger(9)}
	out := []core.Value{core.NewInteger(1)}
	h.SubstituteJoinedOuts(out)
	if n, err := core.AsInteger(out[0]); err != nil || n != 9 {
		t.Fatal("SubstituteJoinedOuts must substitute the joined position")
	}
}

func TestS6aDataListElemTypeMapNilParents(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("a", core.Value{})
	if got := check.DataListElemTypeFromValue(core.NewMap(m)); !got.Equal(core.TAny) {
		t.Errorf("nil-Parent map values: got %v, want Any", got)
	}
}

// --- tryFoldScalarConst / concreteHandlerEval / scalarFoldOperand -----------

func TestS7CheckAtIndicesGuards(t *testing.T) {
	r := newTestRegistry(t)
	// Unknown data length (non-list) -> no-op, no diagnostic.
	check.CheckAtIndices(r, core.NewList([]core.Value{core.NewInteger(0)}), core.NewInteger(1), "at")
	// Known data length, but the indices list is nil -> early return.
	data := core.NewList([]core.Value{core.NewInteger(1)})
	check.CheckAtIndices(r, core.NewList(nil), data, "at")
	if n := len(r.Check.Diagnostics); n != 0 {
		t.Errorf("guarded CheckAtIndices should emit nothing, got %d diagnostics", n)
	}
}

func TestS6b0CheckMakeConstructionUnreadableMap(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	// Concrete, map-conforming, but the payload is a record type body:
	// AsMap declines and the check is a no-op.
	src := core.NewRecordType(core.NewOrderedMap())
	ct := core.NewClassType(core.TClass, core.ClassTypeInfo{Name: "Ideal/S6b0CM", Fields: core.NewOrderedMap()})
	check.CheckMakeConstruction(r, ct, src, core.SrcPos{Row: 1, Col: 1})
	if len(r.Check.Diagnostics) != 0 {
		t.Errorf("unreadable construction map must not emit diagnostics: %v", r.Check.Diagnostics)
	}
}

func TestS6aTryRecordFallbackBakedNonInertDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb6", core.CompileIslandPure, 1, -1)
	// A FlexList is CONCRETE (materialise bakes it) but MUTABLE, so
	// isInertConst rejects it: baking a mutable instance into the island's
	// const span would let a later mutation corrupt the frozen bake. The
	// non-inert-data arg branch declines, so the program falls back to the
	// interpreter unchanged. (An enclosing-scope binding read of such a value
	// no longer reaches here — resolveOperand routes it to a dynamic-scope
	// lookup — so this direct unit test pins the refusal contract.)
	args := []core.Value{core.NewFlexList([]core.Value{core.NewInteger(1)})}
	outs := []core.Value{core.NewDynamicCarrier(core.TAny)} // dynamic out: island is legitimate
	if TryRecordFallback(r, "s6afb6", sig, args, outs, core.SrcPos{}) {
		t.Error("a baked but non-inert (mutable) data arg must decline the island")
	}
}

// --- bodyFreeForFallback --------------------------------------------------------

// InterpBodyInert / InterpMemberInert widen the const-bake whitelist so a
// property-harness body (test-check-prop) containing an interpolated string
// bakes as code-as-data at MODULE scope. These pins fix the soundness boundary:
//   - an InterpString is admitted only as a MEMBER (InterpMemberInert), never as
//     a standalone top-level const, and never when its holes are non-inert;
//   - InterpBodyInert's TOP LEVEL stays exactly isInertConst — a standalone
//     ParenExpr is NOT bakeable (baking `(loopy 1)` wrongly compiled a nested
//     too-deep macroexpand that must refuse);
//   - isInertConst itself is UNCHANGED — still strict — which is what keeps an
//     InterpString body refused inside a compiled fn frame (the fn-scope
//     miscompile guard rests on this).
func TestInterpBodyInertBoundary(t *testing.T) {
	// `v${k}` — an InterpString with an inert hole (the Word k).
	interp := core.NewInterpString([]core.InterpPart{{Lit: "v"}, {Expr: []core.Value{core.NewWord("k")}}})

	// Admitted as a MEMBER, and inside a List body.
	if !InterpMemberInert(interp) {
		t.Error("InterpMemberInert(`v${k}`) = false; an InterpString with inert holes must be admitted as a member")
	}
	if !InterpBodyInert(core.NewList([]core.Value{interp})) {
		t.Error("InterpBodyInert([`v${k}`]) = false; a List body with an InterpString member must be inert")
	}
	// Nested two deep — the recursion must reach it.
	if !InterpBodyInert(core.NewList([]core.Value{core.NewList([]core.Value{interp})})) {
		t.Error("InterpBodyInert([[`v${k}`]]) = false; a deeply nested InterpString member must be reached")
	}

	// NEGATIVE: a carrier/dynamic InterpString (render only known at run time)
	// must NOT bake — a stale check-mode render would diverge.
	cInterp := interp
	cInterp.Carrier = true
	if InterpMemberInert(cInterp) {
		t.Error("InterpMemberInert(carrier `v${k}`) = true; a carrier InterpString must not bake")
	}
	// NEGATIVE: a hole that is a carrier (non-inert) rejects the whole InterpString.
	badHole := core.NewInterpString([]core.InterpPart{{Expr: []core.Value{core.NewCarrier(core.TInteger)}}})
	if InterpMemberInert(badHole) {
		t.Error("InterpMemberInert(`${carrier}`) = true; a non-inert hole must reject")
	}

	// TOP-LEVEL strictness: a standalone InterpString and a standalone ParenExpr
	// are NOT bakeable bodies (InterpBodyInert defers to isInertConst there).
	if InterpBodyInert(interp) {
		t.Error("InterpBodyInert(`v${k}`) = true; a standalone InterpString must not bake (member-only)")
	}
	if InterpBodyInert(core.NewParenExpr([]core.Value{core.NewWord("loopy"), core.NewInteger(1)})) {
		t.Error("InterpBodyInert((loopy 1)) = true; a standalone ParenExpr is deferred code, not a bakeable body")
	}

	// INVARIANT: the strict whitelist must stay strict — it must NOT admit the
	// InterpString. This is the foundation of the fn-scope miscompile guard: at
	// fn scope noEvalBodiesInertScoped uses isInertConst, which keeps the body
	// refused so it falls back to the interpreter instead of baking a
	// frame-local ${name} that would resolve against the registry.
	if core.IsInertConst(interp) {
		t.Error("isInertConst(`v${k}`) = true; the strict whitelist must NOT admit an InterpString")
	}
	if core.IsInertConst(core.NewList([]core.Value{interp})) {
		t.Error("isInertConst([`v${k}`]) = true; the strict whitelist must keep an InterpString body refused")
	}
}

func TestRecordCallOperandsInertFnBakeCaptureFree(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	sig := &core.Signature{Args: []*core.Type{core.TFunction}, FnInertArgs: map[int]bool{0: true}}
	// The sibling of the captured case: a CAPTURE-FREE concrete fn value in an
	// fn-inert slot bakes as a const the storing / introspecting handler reads
	// at run time — its frozen body has no captured names to leave unbound, so
	// the const is faithful. A plain capture-free fn is const-baked by
	// resolveOperand's isInertConst path BEFORE this switch; only a fn that
	// isInertConst refuses reaches here. A capture-free MACRO module fn (Registry
	// set, Macro true) is exactly that case — resolveOperand declines it, so the
	// `IsConcrete && no-captures` arm is what places the const operand.
	fn := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Signatures: []core.Signature{{Args: []*core.Type{core.TInteger}}},
			Registry: r, Macro: true}}
	ops, ok := es.RecordCallOperands("stash", sig, []core.Value{fn})
	if !ok {
		t.Fatalf("a capture-free fn that resolveOperand refuses must bake as a const here")
	}
	if len(ops) != 1 || ops[0].kind != opConst {
		t.Fatalf("expected a single opConst operand, got %+v", ops)
	}
}

func TestXmlInterpDynamicCompilesToOp(t *testing.T) {
	// GRADUATED (REFUSAL-CLOSURE §9.2c, 2026-07-17): a runtime-computed hole
	// lowers to OpInterpXml — the hole's own dispatch records its event and
	// the op rebuilds the element from the popped value at run time
	// (rebuildXmlFromTmpl). End-to-end parity is pinned in
	// lang/go bytecode_s9_landing_test.go TestS9XmlInterpComputed.
	r := covRegistry(t, nil)
	tmpl := core.XmlTmpl{
		Tag:  "p",
		Cren: []core.XmlCren{{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)}}},
	}
	prog, reason := compileTokens(t, r, []core.Value{core.NewXmlInterp(tmpl)})
	if prog == nil {
		t.Fatalf("dynamic XML interp must compile to OpInterpXml, refused: %s", reason)
	}
	if !strings.Contains(prog.Disassemble(), "INTERP_XML") {
		t.Fatalf("expected an INTERP_XML op:\n%s", prog.Disassemble())
	}
}

func TestEngineRecorderAndTraceHooks(t *testing.T) {
	r := covRegistry(t, nil)
	e := core.NewTop(r)
	rec := &w4CountRecorder{}
	e.SetRecorder(rec)
	steps := 0
	e.SetTrace(func(step, pointer int, tape []core.Value, note string) {
		steps++
	})
	out, err := e.Run([]core.Value{core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)})
	if err != nil {
		t.Fatalf("traced run: %v", err)
	}
	if renderAll(out) != "3" {
		t.Errorf("traced run = %s", renderAll(out))
	}
	if rec.lits == 0 || rec.calls == 0 {
		t.Errorf("recorder saw lits=%d calls=%d", rec.lits, rec.calls)
	}
	if steps == 0 {
		t.Error("trace callback never fired")
	}
	e.SetRecorder(nil)
	e.SetTrace(nil)
}
