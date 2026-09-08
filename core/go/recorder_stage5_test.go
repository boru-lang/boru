package core

import "testing"

// TestInactiveEmitMethodArms pins every method arm of the inactive no-op
// recorder (the EmitRecorder a compile-free pass runs against): each
// answer is exactly the zero/none/decline the corresponding nil-receiver
// *EmitState method returns, so swapping the inactive recorder in for
// the historical nil field stays behaviour-identical. Style follows the
// dispatch-slots / analysis-hooks pin tests.
func TestInactiveEmitMethodArms(t *testing.T) {
	e := TheInactiveEmit

	// --- activity / lifecycle.
	if e.Armed() {
		t.Fatal("inactive Armed must be false")
	}
	e.Suspend()() // resume closure is a no-op
	if e.SuspendedNow() {
		t.Fatal("inactive SuspendedNow must be false")
	}
	e.BodyAnalysisGuard()()
	e.KeepDefsBodyGuard(nil, "")()
	e.MultiRunBodyGuard(nil, "b")()
	e.RecordDynUndef("x", SrcPos{})
	e.FnBodyGuard()()

	// --- refusal + site accounting.
	e.MarkUncompilable("reason")
	e.SetCatchVariadic(true)
	if e.Sites() != nil {
		t.Fatal("inactive Sites must be nil")
	}

	// --- inline context-boundary regions (NUR054): no-ops.
	e.PushInlineCtxBoundary()
	e.PopInlineCtxBoundary()

	// --- Stage-0b probe promotions.
	if e.InClosureUnit() {
		t.Fatal("inactive InClosureUnit must be false")
	}
	if e.StoredGradualActive() {
		t.Fatal("inactive StoredGradualActive must be false")
	}
	if e.RecordSpliceDyn(Value{}, SrcPos{}) {
		t.Fatal("inactive RecordSpliceDyn must decline")
	}
	e.NoteShapedRead("id")
	if _, ok := e.MemberFnReadValue("id"); ok {
		t.Fatal("inactive MemberFnReadValue must decline")
	}
	if _, ok := e.DefReadName("id"); ok {
		t.Fatal("inactive DefReadName must decline")
	}

	// --- dispatch / value recording.
	e.RecordCall("w", nil, nil, nil, SrcPos{}, false, false)
	e.RecordPoly("w")
	if e.RecordPolyCall("w", nil, nil, SrcPos{}, nil, nil) {
		t.Fatal("inactive RecordPolyCall must decline")
	}
	e.RecordUserCall(0, nil, nil, SrcPos{})
	e.RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, nil, SrcPos{})
	if n, ok := e.RecordDynApply(nil, Value{}, Value{}, SrcPos{}); ok || n != 0 {
		t.Fatal("inactive RecordDynApply must decline with no consumed args")
	}
	if e.RecordDynApplyName("h", nil, Value{}, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordDynApplyName must decline")
	}
	if e.DynApplyLeadEligible(Value{}) {
		t.Fatal("inactive DynApplyLeadEligible must be false")
	}
	if e.RecordDynMethod(Value{}, nil, nil, "w", SrcPos{}) {
		t.Fatal("inactive RecordDynMethod must decline")
	}
	if e.RecordFallback(FallbackSpan{}, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordFallback must decline")
	}
	if e.RecordTrap("code", "detail", "w", "hint", SrcPos{}) {
		t.Fatal("inactive RecordTrap must decline")
	}
	if e.RecordTrapErr(nil, SrcPos{}) {
		t.Fatal("inactive RecordTrapErr must decline")
	}
	if e.RecordDispatchRematchValues("w", nil, 0, 0, SrcPos{}) {
		t.Fatal("inactive RecordDispatchRematchValues must decline")
	}
	out := NewInteger(7)
	if got, ok := e.RecordTypedBind(TypedBindSpec{}, Value{}, out, SrcPos{}); ok || !ValuesEqual(got, out) {
		t.Fatal("inactive RecordTypedBind must pass out through and decline")
	}
	if e.RecordMakeList(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeList must decline")
	}
	if e.RecordMakeListInner(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeListInner must decline")
	}
	if e.RecordMakeMap(nil, nil, nil, false, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordMakeMap must decline")
	}
	if e.RecordInterp(nil, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordInterp must decline")
	}
	e.RegisterTrailingApply("id", 1)
	if e.ApplyPending("id") {
		t.Fatal("inactive ApplyPending must be false")
	}
	if _, ok := e.PendingClosureApply(nil); ok {
		t.Fatal("inactive PendingClosureApply must miss")
	}
	if got := e.ArmTailApply([]Value{NewInteger(1)}); len(got) != 1 {
		t.Fatal("inactive ArmTailApply must pass the residual through")
	}
	if _, ok := e.UnitTailApply(0); ok {
		t.Fatal("inactive UnitTailApply must miss")
	}
	e.NoteMemberFnRead("id", Value{})
	if e.MemberFnRead("id") {
		t.Fatal("inactive MemberFnRead must be false")
	}
	if e.DynInputsProven(nil, nil) {
		t.Fatal("inactive DynInputsProven must be false")
	}
	v := NewInteger(3)
	if got, ok := e.Materialise(v); ok || !ValuesEqual(got, v) {
		t.Fatal("inactive Materialise must pass through and decline")
	}
	if e.ZeroOutProduced("id") {
		t.Fatal("inactive ZeroOutProduced must be false")
	}
	if e.AlreadyProduced("id") {
		t.Fatal("inactive AlreadyProduced must be false")
	}

	// --- defs / locals.
	e.RecordBindTwin(BindTransition{}, DefEntry{})
	e.MarkValueDef(Value{})
	e.RecordDefRebind("n", Value{}, SrcPos{})
	e.RecordDynBind("n", Value{}, SrcPos{})
	e.NoteDefRead("id", "n")
	e.NoteWordRead(Value{}, "n", SrcPos{})
	e.NoteValRead("id", "n")
	e.NoteFrozenRead("n", FrozenBakeValue, 0)
	e.NotifyNameRebound("n")
	if got := e.RegisterLocal("id"); got != -1 {
		t.Fatalf("inactive RegisterLocal must be -1, got %d", got)
	}

	// --- branches / loops.
	e.ArmBranchCapture()
	if e.PeekCaptureArm() {
		t.Fatal("inactive PeekCaptureArm must be false")
	}
	e.ArmLoopCapture()
	if e.ConsumeLoopArm() {
		t.Fatal("inactive ConsumeLoopArm must be false")
	}
	if _, ok := e.SplitLoopRegionBind("n", Value{}); ok {
		t.Fatal("inactive SplitLoopRegionBind must decline")
	}
	if _, ok := e.SplitEventRegionBind("n", Value{}); ok {
		t.Fatal("inactive SplitEventRegionBind must decline")
	}
	// TakeFragment / RecordBranch / RecordLoop are the group that lets a word
	// library outside compiler (basic's if / case / for) record control flow
	// without naming *compiler.EmitState. A nil fragment is the honest inactive
	// answer — there is nothing to hand back — and the two recorders drop their
	// records, which is what "no compiler linked, runs interpreted" means.
	if frag := e.TakeFragment(); frag != nil {
		t.Fatalf("inactive TakeFragment must be nil, got %v", frag)
	}
	e.RecordBranch(BranchRecord{})
	e.RecordLoop(Value{}, Value{}, Value{}, nil, nil, "iter", Value{}, 0, SrcPos{})
	if e.RecordInterpXml(XmlTmpl{}, nil, Value{}, SrcPos{}) {
		t.Fatal("inactive RecordInterpXml must decline")
	}
	e.BeginLoopCarried()
	e.EndLoopCarried()
	e.NoteLoopCarried("n", Value{}, Value{})
	if e.Checkpoint() != nil {
		t.Fatal("inactive Checkpoint must be nil")
	}
	e.Rollback(nil)
	if e.CanSeatAcrossFragment(Value{}) {
		t.Fatal("inactive CanSeatAcrossFragment must be false")
	}

	// --- fn-unit compilation.
	unit, finish, ok := e.StartFnCompile("k", "n", nil, nil, nil, nil, nil, false, SrcPos{})
	if unit != -1 || finish != nil || ok {
		t.Fatal("inactive StartFnCompile must decline with unit -1")
	}
	e.SetUnitParamTypes(0, nil, nil)
	e.SetUnitReturnPatterns(0, nil)
	e.SetUnitDecl(0, DeclSite{})
	if e.UnitVariadic(0) {
		t.Fatal("inactive UnitVariadic must be false")
	}
	if e.UnitNetsZero(0) {
		t.Fatal("inactive UnitNetsZero must be false")
	}

	// The embeddable checkpoint marker: its isEmitCheckpoint tag is the
	// whole contract.
	EmitCheckpointBase{}.isEmitCheckpoint()
}
