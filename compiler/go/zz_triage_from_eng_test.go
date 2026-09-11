package compiler

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// inClosureUnit — the closure-body args-projection gate's classifier.
// White-box: the nil/empty/inconsistent shapes must all report false
// (only a live, well-formed closure unit may decline the projection),
// and a closure-flagged innermost rec reports true.
func TestInClosureUnit(t *testing.T) {
	// nil receiver and empty open-unit stack: not inside any fn unit.
	var nilES *EmitState
	if nilES.InClosureUnit() {
		t.Errorf("nil EmitState: want false")
	}
	es := NewEmitState()
	if es.InClosureUnit() {
		t.Errorf("empty openUnitRecs: want false")
	}

	// Defensive: an out-of-range rec index (never produced by
	// StartFnCompile's paired push, but the guard keeps a future
	// misalignment from panicking) reports false rather than indexing.
	es.openUnitRecs = []int{5}
	if es.InClosureUnit() {
		t.Errorf("out-of-range rec index: want false")
	}
	es.openUnitRecs = []int{-1}
	if es.InClosureUnit() {
		t.Errorf("negative rec index: want false")
	}

	// A real fn unit (closure=false) does not decline; a closure unit does.
	es = NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "f"})
	es.openUnitRecs = []int{0}
	if es.InClosureUnit() {
		t.Errorf("plain fn unit: want false")
	}
	es.fnRecs[0].closure = true
	if !es.InClosureUnit() {
		t.Errorf("closure unit: want true")
	}

	// Innermost wins: a fn unit nested inside a closure unit is NOT a
	// closure context (its args frame is its own), and vice versa.
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "inner"})
	es.openUnitRecs = []int{0, 1}
	if es.InClosureUnit() {
		t.Errorf("fn unit nested in closure: want false (innermost wins)")
	}
}

// closureResidualExact / stripResidualShapeOK — the whole-residual and
// strip-input closure screens' classifier branches. White-box: defensive
// shapes report false (never compile a unit whose runtime count could
// diverge); the admitted shapes report true.
func TestClosureResidualScreens(t *testing.T) {
	// Defensive: nil state / out-of-range unit.
	if closureResidualExact(nil, 0, 1) {
		t.Errorf("nil EmitState: want false")
	}
	if stripResidualShapeOK(nil, 0, 1) {
		t.Errorf("nil EmitState: want false")
	}
	es := NewEmitState()
	if closureResidualExact(es, 3, 1) || stripResidualShapeOK(es, 3, 1) {
		t.Errorf("out-of-range unit: want false")
	}

	// Exactness: count match required; variadic / dynamic tails decline.
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "u", outOps: []EmitOperand{{kind: opConst}, {kind: opConst}}})
	if !closureResidualExact(es, 0, 2) {
		t.Errorf("2-op residual, want=2: want true")
	}
	if closureResidualExact(es, 0, 3) {
		t.Errorf("2-op residual, want=3: want false (count mismatch)")
	}
	es.fnRecs[0].variadic = true
	if closureResidualExact(es, 0, 2) {
		t.Errorf("variadic residual: want false")
	}

	// Strip shapes: 1 op admits; 2 ops admit only with the input (param
	// local 0) at the bottom; deeper/variadic/dyn shapes decline.
	rec := &fnUnitRec{name: "s", outOps: []EmitOperand{{kind: opConst}}}
	es.fnRecs = append(es.fnRecs, rec)
	if !stripResidualShapeOK(es, 1, 1) {
		t.Errorf("1-op residual: want true")
	}
	if stripResidualShapeOK(es, 1, 0) {
		t.Errorf("1-op residual with want=0: want false (count disagreement)")
	}
	rec.outOps = []EmitOperand{{kind: opLocal, idx: 0}, {kind: opConst}}
	if !stripResidualShapeOK(es, 1, 1) {
		t.Errorf("2-op residual with input bottom: want true")
	}
	rec.outOps = []EmitOperand{{kind: opConst}, {kind: opConst}}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("2-op residual with non-input bottom: want false")
	}
	rec.outOps = []EmitOperand{{kind: opLocal, idx: 0}, {kind: opConst}, {kind: opConst}}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("3-op residual: want false")
	}
	rec.outOps = nil
	if !stripResidualShapeOK(es, 1, 0) {
		t.Errorf("empty residual with want=0: want true (the zero-netting handler)")
	}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("empty residual with want=1: want false")
	}
	rec.outOps = []EmitOperand{{kind: opConst}}
	rec.dynTrailArity = 2
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("dynTrail residual: want false")
	}
	rec.dynTrailArity = 0
	rec.variadic = true
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("variadic strip residual: want false")
	}
}

// The catch-variadic latch (L-DO part 1): set by the fallible do body's
// ReturnsFn, consumed exactly once by the dispatch's record path, keyed to
// the CompileFallbackBody sig so it can never mark an unrelated word's
// event.
func TestCatchVariadicLatch(t *testing.T) {
	es := NewEmitState()
	catchSig := &core.Signature{CompileEffect: core.CompileFallbackBody}
	plainSig := &core.Signature{}

	if es.catchVariadicFor(catchSig) {
		t.Fatal("unlatched: must be false")
	}
	es.SetCatchVariadic(true)
	if es.catchVariadicFor(plainSig) {
		t.Fatal("a non-catch sig must not consume the latch")
	}
	if es.catchVariadicFor(nil) {
		t.Fatal("a nil sig must not consume the latch")
	}
	if !es.catchVariadicFor(catchSig) {
		t.Fatal("the catch sig must consume the latch")
	}
	if es.catchVariadicFor(catchSig) {
		t.Fatal("consumed: a second read must be false")
	}
	es.SetCatchVariadic(true)
	es.SetCatchVariadic(false)
	if es.catchVariadicFor(catchSig) {
		t.Fatal("an explicit clear must stick")
	}

	// Nil-receiver discipline (the recorder methods are nil-safe).
	var nilES *EmitState
	nilES.SetCatchVariadic(true)
	if nilES.catchVariadicFor(catchSig) {
		t.Fatal("nil receiver must be inert")
	}
	// The inactive recorder accepts the call as a no-op.
	core.TheInactiveEmit.SetCatchVariadic(true)
	if core.TheInactiveEmit.RecordInterpXml(core.XmlTmpl{}, nil, core.Value{}, core.SrcPos{}) {
		t.Fatal("the inactive recorder must decline RecordInterpXml")
	}
}

// The GENERIC RecordCall fall-through consumes the latch too (the closure
// probe can decline a fallible do shape; its dispatch then records through
// RecordCall and must still be variadic — the caught path nets 1 where the
// static seat expects N).
func TestCatchVariadicGenericRecordCall(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{CompileEffect: core.CompileFallbackBody}
	a, b := core.NewInteger(1), core.NewInteger(2)
	outA, outB := core.NewInteger(10), core.NewInteger(20)
	outA.ID, outB.ID = "outA", "outB"

	es.SetCatchVariadic(true)
	es.RecordCall("do", sig, []core.Value{a, b}, []core.Value{outA, outB}, core.SrcPos{}, false, false)
	if es.catchVariadicPending {
		t.Fatal("the latch must be consumed by the generic path")
	}
	marked := false
	for _, f := range es.eventInfo {
		if f.variadicResult {
			marked = true
		}
	}
	if !marked {
		t.Fatal("the generic path must mark the latched dispatch variadic")
	}
}

func TestAnyDynamicTailHelpers(t *testing.T) {
	dyn := core.NewDynamicCarrier(core.TAny)
	static := core.NewInteger(1)
	if anyDynamicTail([]core.Value{dyn, static}) {
		t.Error("static tail read as dynamic")
	}
	if !anyDynamicTail([]core.Value{static, dyn}) {
		t.Error("dynamic tail missed")
	}
	fnv := core.NewFunction(core.FnDefInfo{Name: "f"})
	if !anyFnOrDynamicTail([]core.Value{static, fnv}) {
		t.Error("fn tail missed")
	}
	if anyFnOrDynamicTail([]core.Value{fnv, static}) {
		t.Error("leading fn read as tail")
	}
}

// TestRecordPolyCallTypeNameOperand pins RecordPolyCall's type-name
// cascade: a raw un-stepped Word in the claimed window that NAMES a live
// type is stepped to its canonical literal (mirroring stepWord) and bakes
// as an OpPushType operand instead of declining the whole poly window.
// The bare-name arm (`Bytes`, `String`) is driven end-to-end by
// lang/go/test/stamp_filterlambda_test.go; the SLASHED-path arm is only
// reachable from a programmatically built claim (the parser resolves
// slashed type paths to literals at parse time — see parseWord), so it is
// pinned here directly, the same seam stepWord's twin arm uses
// (engine_seam7_run_test.go::TestS7RunBareTypePathWord).
func TestRecordPolyCallTypeNameOperand(t *testing.T) {
	newState := func() *EmitState {
		r, err := core.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		es := NewEmitState()
		es.BindRegistry(r)
		return es
	}
	lastOps := func(es *EmitState) []EmitOperand {
		frame := es.frames[len(es.frames)-1]
		return frame[len(frame)-1].call.ops
	}

	// Slashed type path (the ResolveTypePath arm).
	es := newState()
	out := core.NewDynamicCarrier(core.TInteger)
	if !es.RecordPolyCall("convert", []core.Value{core.NewWord("Scalar/Number/Integer")}, []core.Value{out}, core.SrcPos{}, es.reg, nil) {
		t.Fatal("a slashed type-path word operand should record")
	}
	if ops := lastOps(es); len(ops) != 1 || ops[0].kind != opType {
		t.Fatalf("slashed-path operand = %+v, want a type operand", lastOps(es))
	}

	// Bare kernel type name (the typeNames arm).
	es = newState()
	if !es.RecordPolyCall("convert", []core.Value{core.NewWord("Integer")}, []core.Value{out}, core.SrcPos{}, es.reg, nil) {
		t.Fatal("a bare type-name word operand should record")
	}
	if ops := lastOps(es); len(ops) != 1 || ops[0].kind != opType {
		t.Fatalf("bare-name operand = %+v, want a type operand", lastOps(es))
	}

	// Negatives — anything but a PLAIN word naming a live type keeps the
	// unresolvable-operand decline:
	// a MODIFIED word (the /s form changes collection semantics);
	es = newState()
	if es.RecordPolyCall("convert", []core.Value{core.NewWordModified("Integer", -1, true, false)}, []core.Value{out}, core.SrcPos{}, es.reg, nil) {
		t.Fatal("a modified type-name word must decline")
	}
	// a word that names no type at all;
	es = newState()
	if es.RecordPolyCall("convert", []core.Value{core.NewWord("no-such-name")}, []core.Value{out}, core.SrcPos{}, es.reg, nil) {
		t.Fatal("a non-type word must decline")
	}
	// a slashed path that resolves to no builtin.
	es = newState()
	if es.RecordPolyCall("convert", []core.Value{core.NewWord("No/Such/Type")}, []core.Value{out}, core.SrcPos{}, es.reg, nil) {
		t.Fatal("an unknown slashed path must decline")
	}
}

// TestRecordTrapFirstWins covers the "a trap is already recorded" arm of
// RecordTrap: the first top-level trap wins (execution can reach only one),
// and a second RecordTrap reports ownership without overwriting it.
func TestRecordTrapFirstWins(t *testing.T) {
	es := NewEmitState() // fresh state is active with one frame and one unit
	if !es.RecordTrap("first", "d1", "w1", "h1", core.SrcPos{}) {
		t.Fatal("first RecordTrap should be owned here")
	}
	firstAt := es.trapAt
	if firstAt == 0 {
		t.Fatal("first RecordTrap should have recorded a trap event")
	}
	if !es.RecordTrap("second", "d2", "w2", "h2", core.SrcPos{}) {
		t.Fatal("second RecordTrap should still report ownership")
	}
	if es.trapAt != firstAt {
		t.Fatalf("second RecordTrap overwrote the trap: trapAt %d != %d", es.trapAt, firstAt)
	}
}

// TestRecordTrapErrFirstWins is the RecordTrapErr mirror of
// TestRecordTrapFirstWins: once a top-level trap is recorded, a second
// RecordTrapErr reports ownership (the trapAt-already-set arm) without
// overwriting the first trap event. Two statically-definite unmatched
// dispatches in one compiled unit reach this at the integration level; the
// white-box call pins the guard directly.
func TestRecordTrapErrFirstWins(t *testing.T) {
	es := NewEmitState() // fresh state is active with one frame and one unit
	if !es.RecordTrapErr(&core.BoruError{Code: "first", Detail: "d1"}, core.SrcPos{}) {
		t.Fatal("first RecordTrapErr should be owned here")
	}
	firstAt := es.trapAt
	if firstAt == 0 {
		t.Fatal("first RecordTrapErr should have recorded a trap event")
	}
	if !es.RecordTrapErr(&core.BoruError{Code: "second", Detail: "d2"}, core.SrcPos{}) {
		t.Fatal("second RecordTrapErr should still report ownership")
	}
	if es.trapAt != firstAt {
		t.Fatalf("second RecordTrapErr overwrote the trap: trapAt %d != %d", es.trapAt, firstAt)
	}
}

func TestFoldFullStackGates(t *testing.T) {
	one := core.NewInteger(1)

	// Inactive / suspended / nested-frame / nested-unit / open-mark states
	// all decline.
	var nilES *EmitState
	if _, ok := nilES.FoldFullStack("depth", nil, nil); ok {
		t.Error("a nil recorder must decline")
	}
	es := NewEmitState()
	es.Compilable = false
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("an uncompilable state must decline")
	}
	es = NewEmitState()
	es.suspended = 1
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a suspended state must decline")
	}
	es = NewEmitState()
	es.frames = append(es.frames, nil)
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a nested frame must decline")
	}
	// A nested UNIT is no longer a decline by itself (the fifty-second
	// increment): a compiled body unit has its own stack discipline exactly
	// as the top unit does, and the exactness condition is per-unit. What
	// declines is being somewhere the unit's own residual has not been
	// reconciled yet — an open FRAGMENT above the unit's root frame.
	es = NewEmitState()
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	es.fnRecs = append(es.fnRecs, &fnUnitRec{rootFrame: 0})
	es.openUnitRecs = append(es.openUnitRecs, 0)
	es.frames = append(es.frames, nil) // a fragment open ABOVE the unit's root
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("a fragment open above the unit's root frame must decline")
	}
	// …and the same unit AT its root frame is admitted, which is what makes
	// the row above a statement about the fragment and not about the unit.
	es = NewEmitState()
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	es.fnRecs = append(es.fnRecs, &fnUnitRec{rootFrame: 0})
	es.openUnitRecs = append(es.openUnitRecs, 0)
	if _, ok := es.FoldFullStack("depth", nil, nil); !ok {
		t.Error("a body unit at its own root frame must fold")
	}
	es = NewEmitState()
	es.markWindowSeq = 3
	if _, ok := es.FoldFullStack("depth", nil, nil); ok {
		t.Error("an open mark window must decline")
	}

	// An identity-less stack entry declines; an unknown-provenance carrier
	// declines; a variadic producer declines.
	es = NewEmitState()
	if _, ok := es.FoldFullStack("depth", nil, []core.Value{one}); ok {
		t.Error("an ID-less entry must decline")
	}
	es = NewEmitState()
	dyn := core.NewCarrier(core.TAny)
	dyn.ID = "u1"
	if _, ok := es.FoldFullStack("depth", nil, []core.Value{dyn}); ok {
		t.Error("an unknown-provenance carrier must decline")
	}
	es = NewEmitState()
	vres := core.NewCarrier(core.TInteger)
	vres.ID = "v1"
	es.producedBy["v1"] = producer{seq: 7}
	es.eventInfo[7] = eventFlags{variadicResult: true}
	if _, ok := es.FoldFullStack("depth", nil, []core.Value{vres}); ok {
		t.Error("a variadic producer must decline — its runtime count is not its model count")
	}

	// An unknown full-stack word declines.
	es = NewEmitState()
	if _, ok := es.FoldFullStack("mystery", nil, []core.Value{ffsConst("c1", 1)}); ok {
		t.Error("an unknown word must decline")
	}
}

func TestFoldFullStackWords(t *testing.T) {
	stack := []core.Value{ffsConst("c1", 10), ffsConst("c2", 20)}

	// depth: a fresh CONCRETE count with the stack preserved. A frame-LOCAL
	// entry (a loop iterator's slot) is a known operand home and counts.
	es := NewEmitState()
	es.units[0].localByID["L1"] = 0
	local := core.NewCarrier(core.TInteger)
	local.ID = "L1"
	out, ok := es.FoldFullStack("depth", nil, append([]core.Value{local}, stack...))
	if !ok || len(out) != 4 {
		t.Fatalf("depth fold = %d values (ok=%v), want 4", len(out), ok)
	}
	if n, err := core.AsInteger(out[3]); err != nil || n != 3 || out[3].ID == "" {
		t.Errorf("depth top = %v, want concrete 3 with an ID", out[3])
	}

	// pick: a copy of the picked entry (same ID); an EVENT target is
	// promoted to a value-def local for the double reference.
	es = NewEmitState()
	ev := core.NewCarrier(core.TInteger)
	ev.ID = "e1"
	es.producedBy["e1"] = producer{seq: 4}
	es.eventInfo[4] = eventFlags{}
	st := []core.Value{ev, ffsConst("c3", 30)}
	out, ok = es.FoldFullStack("pick", []core.Value{core.NewInteger(1)}, st)
	if !ok || len(out) != 3 || out[2].ID != "e1" {
		t.Fatalf("pick fold = %v (ok=%v), want the e1 copy on top", out, ok)
	}
	if !es.eventInfo[4].valueDef {
		t.Error("an event-produced pick target must be promoted to a value-def local")
	}

	// pick declines: a non-concrete n, and an out-of-range n (the runtime
	// raises there — the fallback keeps the raise).
	es = NewEmitState()
	carrierN := core.NewCarrier(core.TInteger)
	carrierN.ID = "n1"
	if _, ok := es.FoldFullStack("pick", []core.Value{carrierN}, stack); ok {
		t.Error("a non-concrete n must decline")
	}
	if _, ok := es.FoldFullStack("pick", []core.Value{core.NewInteger(9)}, stack); ok {
		t.Error("an out-of-range n must decline")
	}
	if _, ok := es.FoldFullStack("pick", nil, stack); ok {
		t.Error("a missing n must decline")
	}

	// roll: the true permutation, every ID preserved.
	es = NewEmitState()
	st3 := []core.Value{ffsConst("c1", 1), ffsConst("c2", 2), ffsConst("c3", 3)}
	out, ok = es.FoldFullStack("roll", []core.Value{core.NewInteger(1)}, st3)
	if !ok || len(out) != 3 || out[0].ID != "c1" || out[1].ID != "c3" || out[2].ID != "c2" {
		t.Fatalf("roll fold = %v (ok=%v), want [c1 c3 c2]", out, ok)
	}
}

func TestEmitCheckpointRollback(t *testing.T) {
	es := NewEmitState()
	cp := es.Checkpoint().(emitCheckpoint)
	// Grow the const/type/interned pools after the checkpoint...
	es.intern(core.NewInteger(424242))
	es.internType(core.NewTypeLiteral(core.TInteger))
	if len(es.consts) != cp.consts+1 || len(es.types) != cp.types+1 {
		t.Fatalf("pools did not grow: consts=%d types=%d", len(es.consts), len(es.types))
	}
	// ...then roll back: the pools trim to the checkpoint.
	es.Rollback(cp)
	if len(es.consts) != cp.consts || len(es.types) != cp.types || es.seq != cp.seq {
		t.Errorf("rollback did not restore: consts=%d types=%d seq=%d", len(es.consts), len(es.types), es.seq)
	}
	// Nil-safety on both.
	var nilES *EmitState
	_ = nilES.Checkpoint()
	nilES.Rollback(cp)
}

// singleOutputCall gates which dyn-bound source promoteLateDynBind can seat as
// a lone frame local. The recursion-blind arms (a nil event from a non-recursed
// multi-value arm, a native evCall, a multi-output producer, a non-call event)
// are off the voxgig corpus path, so pin them directly.
func TestSingleOutputCall(t *testing.T) {
	if singleOutputCall(nil) {
		t.Error("a nil event is not a single-output call")
	}
	if !singleOutputCall(&EmitEvent{kind: evCall, call: emitCall{nout: 1}}) {
		t.Error("a single-result native call IS a single-output call")
	}
	if singleOutputCall(&EmitEvent{kind: evCall, call: emitCall{nout: 2}}) {
		t.Error("a multi-result native call is not single-output")
	}
	if !singleOutputCall(&EmitEvent{kind: evCallUser, uc: emitUserCall{nout: 1}}) {
		t.Error("a single-result user call IS a single-output call")
	}
	if singleOutputCall(&EmitEvent{kind: evCallUser, uc: emitUserCall{nout: 0}}) {
		t.Error("a zero-result user call is not single-output")
	}
	if singleOutputCall(&EmitEvent{kind: evBranch}) {
		t.Error("a branch value is not a call")
	}
}

// promoteLateDynBind's promotion tail became corpus-invisible once plan-time
// value-def promotion (planValueDefLocals) started seating every dyn-bound
// source the corpus produces BEFORE Finalize — every recorded evDynBind
// arrives with rec.promoted already carrying its srcSeq, so the late pass's
// own seat/rewrite arms no longer fire from any spec row. The pass is still
// the sound backstop for a unit finished before a later tryRecordDynBody
// arms DynEnv, so pin its contract directly: the seat + rewrite, each skip
// gate (fragment result, variadic producer, non-single-output, already
// promoted), and the disarmed no-ops.
func TestPromoteLateDynBind(t *testing.T) {
	// producer(0) — a single-output call whose value def 1 dyn-binds; a
	// consumer call (2) and the unit residual both reference event 0 and must
	// be rewritten to the seated local.
	mkRec := func() *fnUnitRec {
		return &fnUnitRec{
			numLoc: 3,
			frag: &EmitFragment{events: []EmitEvent{
				{seq: 0, kind: evCall, call: emitCall{nout: 1}},
				{seq: 1, kind: evDynBind, dyn: &emitDynBind{name: "v", srcSeq: 0}},
				{seq: 2, kind: evCall, call: emitCall{nout: 1, ops: []EmitOperand{EventOperand(0, 0)}}},
			}},
			outOps: []EmitOperand{EventOperand(0, 0)},
		}
	}
	es := &EmitState{dynEnv: true, eventInfo: map[int]eventFlags{}}

	rec := mkRec()
	es.promoteLateDynBind(rec)
	if got, ok := rec.promoted[0]; !ok || got != 3 {
		t.Fatalf("promoted[0] = %d, %v — want seat at pre-pass numLoc 3", got, ok)
	}
	if rec.numLoc != 4 {
		t.Errorf("numLoc = %d, want 4 (one seat allocated)", rec.numLoc)
	}
	if op := rec.frag.events[2].call.ops[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("consumer operand not rewritten to the seat: %+v", op)
	}
	if op := rec.outOps[0]; op.kind != opLocal || op.idx != 3 {
		t.Errorf("residual operand not rewritten to the seat: %+v", op)
	}

	// Skip gates — each leaves the rec unpromoted and unchanged.
	skip := func(name string, prep func(*fnUnitRec, *EmitState)) {
		r, s := mkRec(), &EmitState{dynEnv: true, eventInfo: map[int]eventFlags{}}
		prep(r, s)
		before := r.numLoc
		s.promoteLateDynBind(r)
		if r.numLoc != before {
			t.Errorf("%s: promoted anyway (numLoc %d -> %d)", name, before, r.numLoc)
		}
		if op := r.outOps[0]; op.kind != opEvent {
			t.Errorf("%s: residual operand rewritten: %+v", name, op)
		}
	}
	skip("fragment result", func(r *fnUnitRec, _ *EmitState) {
		// The producer is a branch arm's recorded out — it must stay on its sim.
		r.frag.events = append(r.frag.events, EmitEvent{seq: 3, kind: evBranch,
			br: &emitBranch{thenOut: EventOperand(0, 0)}})
	})
	skip("variadic producer", func(_ *fnUnitRec, s *EmitState) {
		s.eventInfo[0] = eventFlags{variadicResult: true}
	})
	skip("multi-output producer", func(r *fnUnitRec, _ *EmitState) {
		r.frag.events[0].call.nout = 2
	})
	skip("already promoted at plan time", func(r *fnUnitRec, _ *EmitState) {
		r.promoted = map[int]int{0: 1}
	})

	// Disarmed / degenerate no-ops.
	off := &EmitState{eventInfo: map[int]eventFlags{}}
	offRec := mkRec()
	off.promoteLateDynBind(offRec)
	if offRec.promoted != nil {
		t.Error("a non-DynEnv program must be byte-for-byte unchanged")
	}
	es.promoteLateDynBind(nil)
	es.promoteLateDynBind(&fnUnitRec{})
}

// TestEmitOperandZeroValueIsNone pins the invariant the kind-enum refactor
// establishes (eng/go/CLAUDE.md "No Zero-Value Overload"): a freshly zeroed
// EmitOperand is opNone — an invalid operand — and is NEVER mistaken for a
// valid "const index 0". The negative half is the point: before the refactor
// the zero value had constIdx == 0, indistinguishable from Consts[0].
func TestEmitOperandZeroValueIsNone(t *testing.T) {
	var zero EmitOperand
	if zero.kind != opNone {
		t.Fatalf("zero EmitOperand.kind = %d, want opNone (%d)", zero.kind, opNone)
	}
	// A zero operand must not read as any indexed kind.
	for _, k := range []operandKind{opConst, opEvent, opLocal, opType, opClosure} {
		if zero.kind == k {
			t.Fatalf("zero EmitOperand.kind unexpectedly == %d", k)
		}
	}
}

// TestOperandConstructors checks each indexed constructor pairs the right kind
// with its idx, and (negative) that a const operand is not read as an event,
// local, or type — the discriminations every lowerer switch relies on.
func TestOperandConstructors(t *testing.T) {
	cases := []struct {
		name string
		op   EmitOperand
		kind operandKind
		idx  int
	}{
		{"const", ConstOperand(7), opConst, 7},
		{"event", EventOperand(3, 0), opEvent, 3},
		{"local", localOperand(2), opLocal, 2},
		{"type", typeOperand(5), opType, 5},
	}
	for _, c := range cases {
		if c.op.kind != c.kind {
			t.Errorf("%s: kind = %d, want %d", c.name, c.op.kind, c.kind)
		}
		if c.op.idx != c.idx {
			t.Errorf("%s: idx = %d, want %d", c.name, c.op.idx, c.idx)
		}
	}
	// Negative: a const operand must not satisfy the event/local/type tests.
	co := ConstOperand(0)
	if co.kind == opEvent || co.kind == opLocal || co.kind == opType {
		t.Fatalf("const operand misclassified as event/local/type")
	}
}

// TestQuoteOperandInertOKFlag pins the CompileQuoteInert contract (review
// cluster 6): a word that DECLARES the flag bakes its INERT quoted operand —
// the quote / codequote / raise / timeout family — via the same bypass that
// exempts get/getr/set, so it compiles as a plain CALL_NATIVE instead of
// refusing. A QuoteArgs word WITHOUT the flag (a meta word like usurp / ref
// whose quoted operand drives a re-stepping result the VM cannot reproduce)
// stays refused, and even a flagged word refuses a NON-inert (carrier) operand.
func TestQuoteOperandInertOKFlag(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name:          "probe-qi",
		CompileEffect: core.CompileQuoteInert,
		Signatures:    []core.Signature{{Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name:       "probe-plainq",
		Signatures: []core.Signature{{Args: []*core.Type{core.TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}},
	})

	ownSig := func(name string) *core.Signature {
		fn := r.Lookup(name)
		if fn == nil {
			t.Fatalf("%s did not register", name)
		}
		for i := range fn.Signatures {
			if !fn.Signatures[i].Fallback {
				return &fn.Signatures[i]
			}
		}
		t.Fatalf("%s has no own signature", name)
		return nil
	}

	inertAtom := core.Value{Parent: core.TAtom, Data: core.AtomPayload{Name: "k"}}
	carrierArg := core.NewCarrier(core.TAtom) // not inert — no concrete payload to bake

	// Flagged + inert operand → bypass the quoted-operand refusal (bakeable).
	if !quoteOperandInertOK(r, "probe-qi", ownSig("probe-qi"), []core.Value{inertAtom}) {
		t.Error("flagged word + inert atom operand: want bakeable, got refused")
	}
	// Flagged + NON-inert (carrier) operand → still refuses (nothing to bake).
	if quoteOperandInertOK(r, "probe-qi", ownSig("probe-qi"), []core.Value{carrierArg}) {
		t.Error("flagged word + carrier operand: want refused, got bakeable")
	}
	// NOT flagged → refuses even with an inert operand (and it is not a module
	// inner native, so the other exemption branch does not apply either).
	if quoteOperandInertOK(r, "probe-plainq", ownSig("probe-plainq"), []core.Value{inertAtom}) {
		t.Error("unflagged QuoteArgs word: want refused, got bakeable")
	}
}

func TestLambdaHookQuoteScreen(t *testing.T) {
	body := core.Boru([]core.Value{core.NewWord("x")})
	r := newTestRegistry(t)

	// An Atom-typed param — the /q quote-capture class — declines even
	// though the input carrier satisfies the type: the runtime leaves the
	// lambda as data, so compiling an applying callback would diverge.
	atomFd := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "k", Type: core.TAtom}}, Impl: body,
	}}}
	if _, ok := lambdaHookCompatible(r, atomFd, []core.Value{core.NewCarrier(core.TAtom)}, ClosureInValue, true, false); ok {
		t.Error("an Atom-typed lambda param must decline the callback admission")
	}
	// The explicit Quote flag declines identically, whatever the type.
	quoteFd := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "k", Type: core.TInteger, Quote: true}}, Impl: body,
	}}}
	core.NormalizeSig(&quoteFd.Signatures[0])
	if _, ok := lambdaHookCompatible(r, quoteFd, []core.Value{core.NewCarrier(core.TInteger)}, ClosureInValue, true, false); ok {
		t.Error("a /q lambda param must decline the callback admission")
	}
	// Control: a plain Integer param keeps admitting.
	intFd := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "n", Type: core.TInteger}}, Impl: body,
	}}}
	if _, ok := lambdaHookCompatible(r, intFd, []core.Value{core.NewCarrier(core.TInteger)}, ClosureInValue, true, false); !ok {
		t.Error("a value-typed lambda param must keep admitting")
	}
}

// TestStoredHandlerDepsBranches exercises the module-level dep capture behind the
// PR #243 comment-#2 fix: storedHandlerDeps records exactly the body words bound
// as a user def at the store site (so a later NotifyNameRebound can poison a stale
// ref). It walks every guard — nil / reg-less receiver, an empty-name word, a
// repeat, and a word that is NOT a user def (a kernel native). (The per-fd
// storedFnDeps wrapper retired with the §7b per-sig stamps — every caller now
// passes the stamped sig's own body directly.)
func TestStoredHandlerDepsBranches(t *testing.T) {
	var nilES *EmitState
	if nilES.storedHandlerDeps([]core.Value{core.NewWord("x")}) != nil {
		t.Fatal("nil EmitState must return nil deps")
	}
	if (&EmitState{}).storedHandlerDeps([]core.Value{core.NewWord("x")}) != nil {
		t.Fatal("reg-less EmitState must return nil deps")
	}

	r := runUnitReg(t)
	r.Defs.Push("helper", core.NewInteger(1)) // a user def the body reads
	es := NewEmitState()
	es.reg = r
	// helper → added; NewInteger → not a word (skipped); zzznotdef → not a def
	// (ok=false, skipped); helper → already in deps (repeat branch); "" → empty
	// name (skipped).
	body := []core.Value{core.NewWord("helper"), core.NewInteger(5), core.NewWord("zzznotdef"), core.NewWord("helper"), core.NewWord("")}
	deps := es.storedHandlerDeps(body)
	if deps == nil || !deps["helper"] || len(deps) != 1 {
		t.Fatalf("deps = %v, want exactly {helper}", deps)
	}
	// A body reading no user def yields nil.
	if d := es.storedHandlerDeps([]core.Value{core.NewWord("zzznotdef")}); d != nil {
		t.Fatalf("deps = %v, want nil (no user-def reads)", d)
	}
}

// TestNotifyNameReboundBranches pins the poison propagation: a rebind of a name an
// already-created stored ref reads flips that ref's poisoned flag (→ Finalize skips
// the stamp → interpreter fallback), while a ref that does NOT read the name is
// left alone. The nil / inactive receiver guards are no-ops (a plain check pass
// must not poison anything).
func TestNotifyNameReboundBranches(t *testing.T) {
	var nilES *EmitState
	nilES.NotifyNameRebound("x") // nil receiver → no panic, no-op

	inactive := NewEmitState()
	inactive.Compilable = false // !active()
	inactive.storedFnRefs = []*CompiledFnRef{{depNames: map[string]bool{"x": true}}}
	inactive.NotifyNameRebound("x")
	if inactive.storedFnRefs[0].poisoned {
		t.Error("an inactive recorder must not poison")
	}

	es := NewEmitState()
	reads := &CompiledFnRef{depNames: map[string]bool{"helper": true}}
	other := &CompiledFnRef{depNames: map[string]bool{"kv": true}}
	es.storedFnRefs = []*CompiledFnRef{reads, other}
	es.NotifyNameRebound("helper")
	if !reads.poisoned {
		t.Error("a ref reading the rebound name must be poisoned")
	}
	if other.poisoned {
		t.Error("a ref not reading the rebound name must stay unpoisoned")
	}
	if es.Compilable {
		t.Error("a module-scope rebind of a STORE-SITE ref's dep must refuse the program")
	}

	// An OPTIONAL ref (stampFnConst's optimisation stamp) poisons the same way
	// but does NOT escalate: poisoning already restored the apply island, which
	// is the exact behaviour the program had before anything stamped it.
	// Refusing the whole program because an optimisation could not survive a
	// rebind is strictly worse than declining the optimisation.
	opt := NewEmitState()
	optRef := &CompiledFnRef{depNames: map[string]bool{"files": true}, optional: true}
	opt.storedFnRefs = []*CompiledFnRef{optRef}
	opt.NotifyNameRebound("files")
	if !optRef.poisoned {
		t.Error("an optional ref reading the rebound name must still be poisoned")
	}
	if !opt.Compilable {
		t.Error("an optional ref must NOT escalate a rebind to a program refusal")
	}
}

func TestNoteLoopCarriedArms(t *testing.T) {
	// loopCarried empty → no-op (guard).
	NewEmitState().NoteLoopCarried("n", core.NewInteger(1), core.NewInteger(2))

	// scope unit-depth mismatch → returns without registering.
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 99, slots: map[string]int{}}}
	es.NoteLoopCarried("n", core.NewInteger(1), core.NewInteger(2))

	// fresh name whose pre value is a fn value → left unregistered.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{}}}
	es.NoteLoopCarried("n", core.NewInteger(1), core.NewCarrier(core.TFunction))
	if _, seen := es.loopCarried[0].slots["n"]; seen {
		t.Fatal("fn-valued pre should not register a slot")
	}

	// outer scope at a different unit depth breaks the reuse walk; a plain
	// pre value then registers a fresh slot with an init.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{
		{unitDepth: 2, slots: map[string]int{}},
		{unitDepth: 1, slots: map[string]int{}},
	}
	es.NoteLoopCarried("n", core.NewInteger(1), core.NewInteger(2))
	if _, seen := es.loopCarried[1].slots["n"]; !seen {
		t.Fatal("a fresh carried name should register a slot")
	}

	// re-resolve path (seen name) whose pre value no longer resolves → refuse.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{
		unitDepth: 1,
		slots:     map[string]int{"n": 0},
		inits:     []carriedInit{{slot: 0}},
	}}
	es.NoteLoopCarried("n", core.NewInteger(1), core.NewCarrier(core.TInteger))
	if es.Compilable {
		t.Fatal("an unresolvable re-resolved pre value should refuse")
	}
}

// TestFinalizeSkipsPoisonedStoredRef pins the OTHER half of the poison
// contract. TestNotifyNameReboundBranches above proves the flag is set on
// the right refs; this proves Finalize ACTS on it — a poisoned ref is left
// unstamped (Prog nil), so InvokeCallback falls back to CallBoru and
// resolves the rebound name live, while a clean ref is back-stamped with
// the built program.
//
// Without this, the `if ref.poisoned { continue }` arm in Finalize was the
// single uncovered statement in the whole tree: every existing test either
// set the flag without finalizing, or finalized without a poisoned ref.
func TestFinalizeSkipsPoisonedStoredRef(t *testing.T) {
	es := NewEmitState()
	// The zeroOut-phantom residual is the minimal shape that finalizes
	// successfully (TestFinalizeResidualArms uses the same setup).
	phantom := core.NewInteger(1)
	es.producedBy[phantom.ID] = producer{seq: 1}
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f

	poisoned := &CompiledFnRef{depNames: map[string]bool{"helper": true}}
	clean := &CompiledFnRef{depNames: map[string]bool{"kv": true}}
	es.storedFnRefs = []*CompiledFnRef{poisoned, clean}

	// The rebind must happen with a unit OPEN. A module-scope rebind
	// (openUnitRecs empty) additionally marks the whole program
	// uncompilable — see NotifyNameRebound's F1 note — so Finalize refuses
	// and never reaches the skip arm. Only a rebind inside another unit's
	// analysis (a body-local def, which shadows independently) leaves a
	// poisoned ref on a program that still finalizes. That asymmetry is why
	// this arm went uncovered.
	es.openUnitRecs = []int{0}
	es.NotifyNameRebound("helper")
	es.openUnitRecs = nil

	prog, why, ok := es.Finalize([]core.Value{phantom})
	if !ok || prog == nil {
		t.Fatalf("Finalize must succeed on an empty program: ok=%v prog=%v why=%q", ok, prog, why)
	}
	if poisoned.Prog != nil {
		t.Error("a poisoned ref must be left UNSTAMPED so InvokeCallback falls back to CallBoru")
	}
	if clean.Prog != prog {
		t.Errorf("a clean ref must be back-stamped with the built program: got %v", clean.Prog)
	}
}
