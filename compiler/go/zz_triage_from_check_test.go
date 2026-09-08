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

func inactiveEmitState() *EmitState {
	es := NewEmitState()
	es.Compilable = false
	return es
}

// NoteShapedRead self-gates on an inactive recorder.
func TestNoteShapedReadInactive(t *testing.T) {
	es := &EmitState{}
	es.NoteShapedRead("some-id")
	if es.shapedReads != nil {
		t.Error("inactive NoteShapedRead must record nothing")
	}
	if es.shapedReadOut([]core.Value{core.NewDynamicCarrier(core.TAny)}) {
		t.Error("shapedReadOut over an empty table must be false")
	}
}

func TestS5bECodeEffectBareTypeResidualDeclines(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s5benoop",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	// Body: a no-op word plus a bare type literal. The analysis is clean
	// and leaves the literal as the residual, which the []*Type domain
	// cannot represent — the carrier conversion must decline.
	body := core.NewList([]core.Value{core.NewWord("s5benoop"), core.NewTypeLiteral(core.TInteger)})
	body.Quoted = true
	if _, ok := AnalyseCodeEffectCarrier(r, body); ok {
		t.Error("expected AnalyseCodeEffectCarrier to decline a bare-type-literal residual")
	}
}

func TestRegisterLocalReuseAndNil(t *testing.T) {
	var nilES *EmitState
	if nilES.RegisterLocal("x") != -1 {
		t.Fatal("nil RegisterLocal should be -1")
	}
	es := NewEmitState()
	s1 := es.RegisterLocal("x")
	if s2 := es.RegisterLocal("x"); s2 != s1 {
		t.Fatalf("re-registering should reuse slot: %d vs %d", s1, s2)
	}
}

func TestBeginEndLoopCarriedGuards(t *testing.T) {
	var nilES *EmitState
	nilES.BeginLoopCarried() // nil guard
	es := NewEmitState()
	es.EndLoopCarried() // empty stack → no-op
}

func TestFinalizeNilReceiver(t *testing.T) {
	var es *EmitState
	if p, why, ok := es.Finalize(nil); p != nil || ok || why == "" {
		t.Fatalf("nil Finalize = %v %q %v", p, why, ok)
	}
}

// TestTrailingApplyArityRoundTrip covers the non-nil happy path: after
// RegisterTrailingApply the arity reads back, and an unregistered id is 0.
func TestTrailingApplyArityRoundTrip(t *testing.T) {
	es := NewEmitState()
	if es.TrailingApplyArity("id") != 0 {
		t.Fatal("unregistered arity should be 0")
	}
	es.RegisterTrailingApply("id", 3)
	if got := es.TrailingApplyArity("id"); got != 3 {
		t.Fatalf("arity = %d, want 3", got)
	}
	// arity < 1 and empty id are rejected.
	es.RegisterTrailingApply("", 2)
	es.RegisterTrailingApply("id2", 0)
	if es.TrailingApplyArity("id2") != 0 {
		t.Fatal("arity<1 should not register")
	}
}

func TestMaterialiseRebuild(t *testing.T) {
	es := NewEmitState()
	// List whose member is a stripped carrier recoverable to a concrete int.
	member := core.NewCarrier(core.TInteger)
	orig := core.NewInteger(7)
	es.origByID[member.ID] = orig
	lst := core.NewList([]core.Value{member})
	got, ok := es.Materialise(lst)
	if !ok {
		t.Fatal("list with a recoverable member should materialise")
	}
	lp, isList := got.Data.(core.ListPayload)
	if !isList || len(lp.Elems) != 1 || lp.Elems[0].Carrier {
		t.Fatalf("rebuilt list member not recovered: %v", got)
	}

	// Map with nil backing → not materialisable.
	nilMap := core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}
	if _, ok := es.Materialise(nilMap); ok {
		t.Fatal("nil-map should not materialise")
	}

	// Map with a recoverable member gets rebuilt.
	m2 := core.NewCarrier(core.TInteger)
	es.origByID[m2.ID] = core.NewInteger(11)
	om := core.NewOrderedMap()
	om.Set("k", m2)
	mp := core.NewMap(om)
	got2, ok2 := es.Materialise(mp)
	if !ok2 {
		t.Fatal("map with a recoverable member should materialise")
	}
	mpOut, isMap := got2.Data.(core.MapPayload)
	if !isMap || mpOut.M == nil {
		t.Fatalf("rebuilt map missing backing: %v", got2)
	}
	if rv, _ := mpOut.M.Get("k"); rv.Carrier {
		t.Fatal("rebuilt map member not recovered")
	}
}

func TestRecordUserPolyCallGuards(t *testing.T) {
	// Inactive → no-op.
	inactiveEmitState().RecordUserPolyCall("w", nil, nil, nil, nil, nil, nil, nil, core.SrcPos{})
	// Active with an unresolvable operand → uncompilable.
	es := NewEmitState()
	es.RecordUserPolyCall("w", nil, nil, nil, nil, nil, []core.Value{carrierVal(core.TInteger)}, []core.Value{core.NewInteger(1)}, core.SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable poly operand should refuse")
	}
}

func TestRecordDynApplyDeclines(t *testing.T) {
	// Inactive → false.
	if _, ok := inactiveEmitState().RecordDynApply(nil, core.NewCarrier(core.TFunction), core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("inactive RecordDynApply should decline")
	}
	// Non-fn callee → false.
	es := NewEmitState()
	if _, ok := es.RecordDynApply(nil, core.NewInteger(5), core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("non-fn callee should decline")
	}
	// Fn-typed carrier but unresolvable → false.
	if _, ok := es.RecordDynApply(nil, core.NewCarrier(core.TFunction), core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("unresolvable fn callee should decline")
	}
	// Fn resolves (a dynamic fn carrier resolves to an event operand), but an
	// ARG is itself a fn value → false.
	fn := core.NewDynamicCarrier(core.TFunction)
	seedProduced(es, fn, 1)
	if _, ok := es.RecordDynApply([]core.Value{core.NewCarrier(core.TFunction)}, fn, core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("fn-valued arg should decline")
	}
	// Under the `apply` WORD (a pending entry for the lead) a fn-valued arg
	// is DATA on the resolved stack (the thirty-second increment): recorded.
	// (A fresh state: the decline above is the event-lead refusal, which
	// marks the state uncompilable.)
	esW := NewEmitState()
	seedProduced(esW, fn, 1)
	argFn := core.NewDynamicCarrier(core.TFunction)
	seedProduced(esW, argFn, 2)
	esW.units[len(esW.units)-1].pendingApply = append(esW.units[len(esW.units)-1].pendingApply, pendingApply{id: fn.ID})
	if n, ok := esW.RecordDynApply([]core.Value{argFn}, fn, core.NewInteger(0), core.SrcPos{}); !ok || n != 1 || !esW.Compilable {
		t.Fatal("a fn-valued arg beneath the apply word records")
	}
	if len(esW.units[len(esW.units)-1].pendingApply) != 0 {
		t.Fatal("the pending entry is consumed")
	}
	// Fn resolves, an ARG is unresolvable → false.
	//
	// The callee here must have a PROVABLE arity. Since the window trim moved
	// the arity decision ahead of the operand build, an event lead with an
	// unprovable arity refuses before any arg is resolved — so a carrier
	// callee would exercise that refusal instead of this one.
	single := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
	}}})
	single.Dynamic = true
	esArg := NewEmitState()
	seedProduced(esArg, single, 1)
	if _, ok := esArg.RecordDynApply([]core.Value{carrierVal(core.TInteger)}, single, core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("unresolvable arg should decline")
	}

	// An OVERLOADED concrete callee has no static arity — which arm runs is a
	// runtime question — so applyWindowArity declines and the window is left
	// untrimmed. Paired with an event lead that means the standing refusal.
	over := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{
		{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1},
		{Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1},
	}})
	over.Dynamic = true
	esOver := NewEmitState()
	seedProduced(esOver, over, 1)
	if _, ok := esOver.RecordDynApply([]core.Value{core.NewInteger(1)}, over, core.NewCarrier(core.TInteger), core.SrcPos{}); ok {
		t.Fatal("an overloaded callee has no provable arity — the event lead must refuse")
	}
	if esOver.Compilable {
		t.Error("the overloaded event lead must mark the program uncompilable")
	}
}

func TestRecordLoopRefusals(t *testing.T) {
	// body == nil.
	es := NewEmitState()
	es.RecordLoop(core.NewInteger(0), core.NewInteger(5), core.NewInteger(1), nil, nil, "it", core.NewInteger(0), 0, core.SrcPos{})
	if es.Compilable {
		t.Fatal("nil loop body should refuse")
	}
	// range of unknown provenance (start unresolvable).
	es = NewEmitState()
	es.RecordLoop(carrierVal(core.TInteger), core.NewInteger(5), core.NewInteger(1), &EmitFragment{}, nil, "it", core.NewInteger(0), 0, core.SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable range should refuse")
	}
	// computed start/step (start resolves to an event, not a const).
	es = NewEmitState()
	start := core.NewCarrier(core.TInteger)
	seedProduced(es, start, 1)
	es.RecordLoop(start, core.NewInteger(5), core.NewInteger(1), &EmitFragment{}, nil, "it", core.NewInteger(0), 0, core.SrcPos{})
	if es.Compilable {
		t.Fatal("computed loop start should refuse")
	}
	// iterator slot not registered (all-const range, empty body).
	es = NewEmitState()
	es.RecordLoop(core.NewInteger(0), core.NewInteger(5), core.NewInteger(1), &EmitFragment{}, nil, "it", core.NewInteger(0), 0, core.SrcPos{})
	if es.Compilable {
		t.Fatal("unregistered iterator should refuse")
	}
}

func TestRecordFallbackGuards(t *testing.T) {
	if inactiveEmitState().RecordFallback(core.FallbackSpan{}, nil, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("inactive RecordFallback should decline")
	}
	es := NewEmitState()
	if es.RecordFallback(core.FallbackSpan{}, []core.Value{carrierVal(core.TInteger)}, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("unresolvable fallback input should decline")
	}
}

func TestRecordTypedBindDeclines(t *testing.T) {
	es := NewEmitState()
	// Concrete body → declines (returns out unchanged, false).
	in := core.NewInteger(5)
	if out, ok := es.RecordTypedBind(core.TypedBindSpec{}, in, in, core.SrcPos{}); ok || out.ID != in.ID {
		t.Fatal("concrete typed-bind body should decline")
	}
	// Non-concrete but unresolvable body → declines.
	c := core.NewCarrier(core.TInteger)
	if _, ok := es.RecordTypedBind(core.TypedBindSpec{}, c, core.NewInteger(0), core.SrcPos{}); ok {
		t.Fatal("unresolvable typed-bind body should decline")
	}
}

func TestRecordPolyMarks(t *testing.T) {
	es := NewEmitState()
	es.RecordPoly("someword")
	if es.Compilable {
		t.Fatal("RecordPoly should mark uncompilable")
	}
	if es.SiteCounts[SitePoly] != 1 {
		t.Fatal("RecordPoly should tally a poly site")
	}
}
