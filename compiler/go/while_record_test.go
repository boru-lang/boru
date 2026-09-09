package compiler

import (
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// while_record_test.go pins the thirty-seventh increment's recorder half:
// RecordWhile's arms and the loop traversals that visit a condition loop's
// condition beside its body (lowerLoop's own lowering is proved by the
// parity rows in lang/go/while_compile_test.go).

// whileFixture returns an active state with a registered scratch iterator,
// a resolvable condition value (produced at seq 1) and two empty fragments.
func whileFixture() (es *EmitState, cond, body *EmitFragment, cv, out core.Value) {
	es = NewEmitState()
	es.RegisterLocal("it")
	cv = carrierVal(core.TBoolean)
	seedProduced(es, cv, 1)
	return es, &EmitFragment{}, &EmitFragment{}, cv, core.NewCarrier(core.TList)
}

func TestRecordWhileRecordsOnTheCountedFrame(t *testing.T) {
	es, cond, body, cv, out := whileFixture()
	es.RecordWhile(cond, body, []core.Value{cv}, nil, "it", out, core.SrcPos{})
	if !es.Compilable {
		t.Fatalf("a one-value condition over a captured body must record, refused %q", es.Reason)
	}
	var lp *emitLoop
	for _, ev := range es.frames[0] {
		if ev.kind == evLoop {
			lp = ev.loop
		}
	}
	if lp == nil {
		t.Fatal("no loop event recorded")
	}
	if lp.cond != cond || lp.body != body {
		t.Error("the loop event must carry the condition and body fragments")
	}
	if lp.condOut.kind != opEvent || lp.condOut.idx != 1 {
		t.Errorf("condOut must be the condition's producing event, got %+v", lp.condOut)
	}
	if lp.hasBodyOut {
		t.Error("an empty body nets no value per iteration")
	}
	constInt := func(op EmitOperand) (int64, bool) {
		if op.kind != opConst || op.idx < 0 || op.idx >= len(es.consts) {
			return 0, false
		}
		v, err := core.AsInteger(es.consts[op.idx])
		return v, err == nil
	}
	if v, ok := constInt(lp.start); !ok || v != 0 {
		t.Errorf("start must be the const 0, got %+v", lp.start)
	}
	if v, ok := constInt(lp.step); !ok || v != 1 {
		t.Errorf("step must be the const 1, got %+v", lp.step)
	}
	if v, ok := constInt(lp.end); !ok || v != math.MaxInt64 {
		t.Errorf("end must be the const MaxInt64, got %+v", lp.end)
	}
	if lp.iterSlot < 0 {
		t.Error("the scratch iterator must own a frame slot")
	}
}

func TestRecordWhileRefusals(t *testing.T) {
	cases := []struct {
		name   string
		record func(es *EmitState, cond, body *EmitFragment, cv, out core.Value)
		want   string
	}{
		{"no condition fragment", func(es *EmitState, _, body *EmitFragment, cv, out core.Value) {
			es.RecordWhile(nil, body, []core.Value{cv}, nil, "it", out, core.SrcPos{})
		}, "while: body not captured"},
		{"no body fragment", func(es *EmitState, cond, _ *EmitFragment, cv, out core.Value) {
			es.RecordWhile(cond, nil, []core.Value{cv}, nil, "it", out, core.SrcPos{})
		}, "while: body not captured"},
		{"empty condition", func(es *EmitState, cond, body *EmitFragment, _, out core.Value) {
			es.RecordWhile(cond, body, nil, nil, "it", out, core.SrcPos{})
		}, "while: condition nets 0 values, not one"},
		{"two-value condition", func(es *EmitState, cond, body *EmitFragment, cv, out core.Value) {
			es.RecordWhile(cond, body, []core.Value{core.NewInteger(1), cv}, nil, "it", out, core.SrcPos{})
		}, "while: condition nets 2 values, not one"},
		{"condition of unknown provenance", func(es *EmitState, cond, body *EmitFragment, _, out core.Value) {
			es.RecordWhile(cond, body, []core.Value{carrierVal(core.TBoolean)}, nil, "it", out, core.SrcPos{})
		}, "while: condition of unknown provenance"},
	}
	for _, c := range cases {
		es, cond, body, cv, out := whileFixture()
		c.record(es, cond, body, cv, out)
		if es.Compilable {
			t.Errorf("%s: must refuse", c.name)
			continue
		}
		if !strings.Contains(es.Reason, c.want) {
			t.Errorf("%s: refused %q, want %q", c.name, es.Reason, c.want)
		}
	}
	// Inactive: nothing is recorded and nothing panics.
	es := inactiveEmitState()
	es.RecordWhile(&EmitFragment{}, &EmitFragment{}, []core.Value{core.NewBoolean(true)}, nil, "it", core.NewCarrier(core.TList), core.SrcPos{})
	if len(es.frames[0]) != 0 {
		t.Error("an inactive recorder must record nothing")
	}
}

// TestLoopTraversalsVisitTheCondition pins that every loop-event traversal
// sees a condition loop's condition beside its body, and a counted loop's
// absent condition as absent.
func TestLoopTraversalsVisitTheCondition(t *testing.T) {
	cond, body := &EmitFragment{}, &EmitFragment{}
	whileEv := &EmitEvent{seq: 3, kind: evLoop, loop: &emitLoop{
		cond: cond, body: body, condOut: EventOperand(1, 0), bodyOut: EventOperand(2, 0), hasBodyOut: true,
	}}
	forEv := &EmitEvent{seq: 3, kind: evLoop, loop: &emitLoop{body: body, bodyOut: EventOperand(2, 0)}}

	if got := childFragments(whileEv); len(got) != 2 || got[0] != cond || got[1] != body {
		t.Errorf("childFragments of a while must be [cond, body], got %v", got)
	}
	if got := childFragments(forEv); len(got) != 2 || got[0] != nil || got[1] != body {
		t.Errorf("childFragments of a for must be [nil, body], got %v", got)
	}
	if outs := fragmentOuts(whileEv); len(outs) != 2 || outs[0] == nil || outs[0].idx != 1 || outs[1] == nil || outs[1].idx != 2 {
		t.Errorf("fragmentOuts of a while must be [condOut, bodyOut], got %v", outs)
	}
	if outs := fragmentOuts(forEv); len(outs) != 2 || outs[0] != nil || outs[1] != nil {
		t.Errorf("fragmentOuts of a side-effect for must be [nil, nil], got %v", outs)
	}
	seen := map[int]bool{}
	forEachFragmentOperand(whileEv, func(op EmitOperand) {
		if op.kind == opEvent {
			seen[op.idx] = true
		}
	})
	if !seen[1] || !seen[2] {
		t.Errorf("forEachFragmentOperand must visit condOut and bodyOut, saw %v", seen)
	}
	seen = map[int]bool{}
	forEachFragmentOperand(forEv, func(op EmitOperand) {
		if op.kind == opEvent {
			seen[op.idx] = true
		}
	})
	if seen[1] || !seen[2] {
		t.Errorf("forEachFragmentOperand over a counted loop must visit bodyOut only, saw %v", seen)
	}
	if res := fragmentResultSeqs([]*EmitEvent{whileEv}); !res[1] || !res[2] {
		t.Errorf("fragmentResultSeqs must mark the condition's and body's results, got %v", res)
	}
	// The dyn-bind scan walks the condition fragment.
	names := map[string]bool{"dsn": true}
	condBind := EmitEvent{kind: evLoop, loop: &emitLoop{
		cond: &EmitFragment{events: []EmitEvent{dsBindEvent("dsn")}},
		body: &EmitFragment{},
	}}
	if !eventsBindDynScope([]EmitEvent{condBind}, names) {
		t.Error("a bind inside a while condition fragment must be detected")
	}
}
