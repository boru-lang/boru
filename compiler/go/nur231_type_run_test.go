package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPlacedTypeTwin pins the twin lookup the run-time type install pairs
// with (NUR231's type half): the LATEST type-install twin of the name, and
// only when it has a stream placement — an unplaced one, or none, is -1.
func TestPlacedTypeTwin(t *testing.T) {
	es := NewEmitState()
	es.bindTwins = []core.BindTransition{
		{Kind: core.BindTypeInstall, Name: "T"},
		{Kind: core.BindDef, Name: "x"},
		{Kind: core.BindTypeInstall, Name: "U"},
		{Kind: core.BindTypeInstall, Name: "T"},
	}
	es.twinPlaced = []bool{true, true, false, true}
	for name, want := range map[string]int{"T": 3, "U": -1, "V": -1, "x": -1} {
		if got := es.placedTypeTwin(name); got != want {
			t.Errorf("placedTypeTwin(%s) = %d, want %d", name, got, want)
		}
	}
}

// TestRecordTypeRun pins the record of a root type def the run installs:
// the body operand and the install event, the def's twin written back so
// its replay installs nothing. Outside the root, over a body with no
// compiled home, or with no placed twin it records nothing — the def then
// declines as the compile-time word it is (RecordRuntimeDispatch).
func TestRecordTypeRun(t *testing.T) {
	body := core.NewDepScalar(core.DepGT, core.NewInteger(3))
	node := &core.Value{}
	mk := func() *EmitState {
		es := NewEmitState()
		es.bindTwins = []core.BindTransition{{Kind: core.BindTypeInstall, Name: "T"}}
		es.twinPlaced = []bool{true}
		return es
	}

	es := mk()
	if !es.recordTypeRun(&pendingTypeRun{name: "T", node: node, body: body}, core.SrcPos{}) {
		t.Fatal("a root install over a const body records")
	}
	evs := es.frames[0]
	if len(evs) == 0 || evs[len(evs)-1].call.typeRun == nil || evs[len(evs)-1].call.typeRun.Name != "T" ||
		evs[len(evs)-1].call.typeRun.Node != node || !es.bindTwins[0].WrittenBack {
		t.Fatalf("the install event carries the spec and the twin is written back: %+v %+v", evs, es.bindTwins)
	}

	es = mk()
	if es.recordTypeRun(&pendingTypeRun{name: "U", node: node, body: body}, core.SrcPos{}) {
		t.Error("a def with no placed twin records nothing")
	}
	if es.recordTypeRun(&pendingTypeRun{name: "T", node: node, body: core.NewCarrier(core.TInteger)}, core.SrcPos{}) {
		t.Error("a body with no compiled home records nothing")
	}
	es.frames = append(es.frames, nil)
	if es.recordTypeRun(&pendingTypeRun{name: "T", node: node, body: body}, core.SrcPos{}) {
		t.Error("an install inside a fragment records nothing")
	}
	if es.bindTwins[0].WrittenBack {
		t.Error("a declined install leaves the twin to replay")
	}
}

// TestRuntimeLatchesNeedALiveRecorder pins the two NUR231 latches: a
// suspended recorder sets neither, so no later dispatch consumes a latch the
// pass left while it was not recording.
func TestRuntimeLatchesNeedALiveRecorder(t *testing.T) {
	es := NewEmitState()
	resume := es.Suspend()
	es.NoteRuntimeTypeInstall("T", nil, core.Value{})
	es.NoteRuntimeDependent()
	resume()
	if es.pendingTypeRun != nil || es.pendingRuntimeDependent {
		t.Fatal("a suspended recorder latches nothing")
	}
	es.NoteRuntimeTypeInstall("T", nil, core.Value{})
	es.NoteRuntimeDependent()
	if es.pendingTypeRun == nil || !es.pendingRuntimeDependent {
		t.Fatal("a live recorder latches both")
	}
}

// TestRecordTypedBindRun pins the run-time membership bind's record: the
// value operand on top and, for an inline constraint, the constraint the run
// computed beneath it; a concrete value records too (the pass's verdict over
// an unknown bound is no verdict). Either operand without a compiled home,
// or a suspended recorder, records nothing.
func TestRecordTypedBindRun(t *testing.T) {
	cons := core.NewDepScalar(core.DepGT, core.NewInteger(3))
	in := core.NewInteger(5)
	spec := core.TypedBindSpec{Kind: core.TypedBindRunMembership, Name: "x", ConsOperand: true}

	es := NewEmitState()
	out, ok := es.RecordTypedBindRun(spec, cons, in, in, core.SrcPos{})
	if !ok || out.ID == in.ID {
		t.Fatalf("a concrete value over an inline constraint records, under a fresh ID: %v %v", out, ok)
	}
	evs := es.frames[0]
	if c := evs[len(evs)-1].call; len(c.ops) != 2 || c.typedBind == nil || !c.typedBind.ConsOperand {
		t.Fatalf("value then constraint: %+v", c)
	}
	named := core.TypedBindSpec{Kind: core.TypedBindRunMembership, Name: "y", Cons: &cons}
	if _, ok := es.RecordTypedBindRun(named, cons, in, in, core.SrcPos{}); !ok || len(es.frames[0][len(es.frames[0])-1].call.ops) != 1 {
		t.Error("a named constraint rides the spec: one operand")
	}
	if _, ok := es.RecordTypedBindRun(spec, core.NewCarrier(core.TInteger), in, in, core.SrcPos{}); ok {
		t.Error("a constraint with no compiled home records nothing")
	}
	if _, ok := es.RecordTypedBindRun(spec, cons, core.NewCarrier(core.TInteger), in, core.SrcPos{}); ok {
		t.Error("a value with no compiled home records nothing")
	}
	resume := es.Suspend()
	defer resume()
	if _, ok := es.RecordTypedBindRun(spec, cons, in, in, core.SrcPos{}); ok {
		t.Error("a suspended recorder records nothing")
	}
}

// TestRecordSigForwards pins the record of an inline signature type's
// run-time forwards (NUR231): one unnamed install event per anonymous node,
// over the refinement operand; all or nothing — a refinement with no
// compiled home, or a signature built inside a fragment, records none. A
// suspended recorder latches nothing.
func TestRecordSigForwards(t *testing.T) {
	body := core.NewDepScalar(core.DepGT, core.NewInteger(3))
	a, b := &core.Value{}, &core.Value{}
	es := NewEmitState()
	if !es.recordSigForwards([]pendingTypeRun{{node: a, body: body}, {node: b, body: body}}, core.SrcPos{}) {
		t.Fatal("two forwards over const refinements record")
	}
	evs := es.frames[0]
	if n := len(evs); n < 2 || evs[n-2].call.typeRun == nil || evs[n-2].call.typeRun.Node != a ||
		evs[n-1].call.typeRun.Node != b || evs[n-1].call.typeRun.Name != "" {
		t.Fatalf("one unnamed install per node, in order: %+v", evs)
	}
	if es.recordSigForwards([]pendingTypeRun{{node: a, body: body}, {node: b, body: core.NewCarrier(core.TInteger)}}, core.SrcPos{}) {
		t.Error("a refinement with no compiled home records nothing")
	}
	es.frames = append(es.frames, nil)
	if es.recordSigForwards([]pendingTypeRun{{node: a, body: body}}, core.SrcPos{}) {
		t.Error("a signature built inside a fragment records nothing")
	}
	sus := NewEmitState()
	resume := sus.Suspend()
	sus.NoteRuntimeSigForward(a, body)
	resume()
	if len(sus.pendingSigForwards) != 0 {
		t.Error("a suspended recorder latches nothing")
	}
	sus.NoteRuntimeSigForward(a, body)
	if len(sus.pendingSigForwards) != 1 {
		t.Error("a live recorder latches the forward")
	}
}
