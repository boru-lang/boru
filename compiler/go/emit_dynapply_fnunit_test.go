package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// emit_dynapply_fnunit_test.go pins the fn-unit dynamic-apply classifier arms
// the corpus rows don't reach: dynFrameWindow's all-prefix decline,
// noteDynFrameReplay's decline path, noteApplyLoopReplay's shape declines, and
// setLoopBodyApply's source-order gate.

func daUnit(ids ...string) (*emitUnit, *fnUnitRec) {
	u := &emitUnit{localByID: map[string]int{}}
	for i, id := range ids {
		u.localByID[id] = i
	}
	rec := &fnUnitRec{nParams: len(ids), nUnnamed: len(ids), returns: []*core.Type{core.TAny}}
	return u, rec
}

func daFnCarrier(id string) core.Value {
	v := core.NewCarrier(core.TFunction)
	v.ID = id
	return v
}

func TestDynFrameWindowArms(t *testing.T) {
	u, rec := daUnit("p0", "p1")
	// All entries are the unnamed-param re-pushes: no token region — decline.
	if _, ok := dynFrameWindow(u, rec, []core.Value{daFnCarrier("p0"), daFnCarrier("p1")}); ok {
		t.Error("an all-prefix residual has no token region and must decline")
	}
	// A read beyond the prefix forms the token region.
	if w, ok := dynFrameWindow(u, rec, []core.Value{daFnCarrier("p0"), daFnCarrier("p1"), daFnCarrier("p0")}); !ok || w != 1 {
		t.Errorf("window = %d/%v, want 1/true", w, ok)
	}
	// A NON-param-local at the bottom stops the prefix immediately: the whole
	// residual is the token region.
	if w, ok := dynFrameWindow(u, rec, []core.Value{daFnCarrier("q9"), daFnCarrier("p0")}); !ok || w != 2 {
		t.Errorf("non-local bottom window = %d/%v, want 2/true", w, ok)
	}
}

func TestNoteDynFrameReplayArms(t *testing.T) {
	es := NewEmitState()
	u, rec := daUnit("p0", "p1")
	// A fn value beyond the exempt window whose residual is ALL prefix:
	// dynFrameWindow declines, so the scan keeps the compile failure (false).
	if es.noteDynFrameReplay(u, rec, []core.Value{daFnCarrier("p0"), daFnCarrier("p1")}, 1) {
		t.Error("an undecomposable fn residual must keep the compile failure")
	}
	if rec.retReplay {
		t.Error("a declined replay must not mark the rec")
	}
	// No fn value at all: nothing to replay, no compile failure.
	if !es.noteDynFrameReplay(u, rec, []core.Value{core.NewInteger(1), core.NewInteger(2)}, 1) {
		t.Error("a fn-free residual never declines here")
	}
	// A fn value with a token region seats the replay.
	u2, rec2 := daUnit("p0", "p1")
	if !es.noteDynFrameReplay(u2, rec2, []core.Value{daFnCarrier("p0"), daFnCarrier("p1"), daFnCarrier("p0")}, 2) {
		t.Error("a decomposable fn residual must seat the replay")
	}
	if rec2.dynFrameW != 1 || !rec2.retReplay {
		t.Errorf("replay window = %d retReplay=%v, want 1/true", rec2.dynFrameW, rec2.retReplay)
	}
	// A DYNAMIC value (a bounded gradual carrier — the map-get-over-Any
	// stylesheet idiom) arms the same replay: callable or not is decided by
	// the runtime rule the replay re-steps under.
	u3, rec3 := daUnit("p0", "p1")
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	dyn.ID = "d0"
	if !es.noteDynFrameReplay(u3, rec3, []core.Value{daFnCarrier("p0"), daFnCarrier("p1"), dyn}, 2) {
		t.Error("a dynamic residual value must seat the replay")
	}
	if rec3.dynFrameW != 1 || !rec3.retReplay {
		t.Errorf("dynamic replay window = %d retReplay=%v, want 1/true", rec3.dynFrameW, rec3.retReplay)
	}
	// TWO applicable values in the window — the chained forward apply
	// `f (g x)` over NAMED params (no unnamed prefix, so the window is the
	// whole residual): the flat re-push lost the inner group's collapse, the
	// re-step is unprovable — decline, keep the compile failure.
	u4 := &emitUnit{localByID: map[string]int{"f": 0, "g": 1, "x": 2}}
	rec4 := &fnUnitRec{nParams: 3, nUnnamed: 0, returns: []*core.Type{core.TAny}}
	xv := core.NewCarrier(core.TInteger)
	xv.ID = "x"
	if es.noteDynFrameReplay(u4, rec4, []core.Value{daFnCarrier("f"), daFnCarrier("g"), xv}, 2) {
		t.Error("a window with two applicable fn values must keep the compile failure")
	}
	if rec4.retReplay {
		t.Error("a declined multi-applicable replay must not mark the rec")
	}
	// The mixed shape — a Function value AND a Dynamic maybe-callable in one
	// window — declines the same way.
	u5 := &emitUnit{localByID: map[string]int{"f": 0}}
	rec5 := &fnUnitRec{nParams: 1, nUnnamed: 0, returns: []*core.Type{core.TAny}}
	dyn2 := core.NewCarrier(core.TAny)
	dyn2.Dynamic = true
	dyn2.ID = "d1"
	if es.noteDynFrameReplay(u5, rec5, []core.Value{daFnCarrier("f"), dyn2}, 1) {
		t.Error("a Function + Dynamic window must keep the compile failure")
	}
}

func TestNoteApplyLoopReplayArms(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[7] = eventFlags{variadicResult: true, applyLoop: true}
	es.eventInfo[8] = eventFlags{}
	loopOp := EventOperand(7, 0)
	rec := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*core.Type{core.TAny}}

	// No apply-loop event: unchanged.
	ops := []EmitOperand{EventOperand(8, 0)}
	if got := es.noteApplyLoopReplay(rec, ops); len(got) != 1 || rec.retReplay {
		t.Error("a residual without an apply-loop event is untouched")
	}
	// The loop sits deeper than the unnamed window: decline.
	rec0 := &fnUnitRec{nParams: 1, nUnnamed: 0, returns: []*core.Type{core.TAny}}
	ops = []EmitOperand{localOperand(0), loopOp}
	if got := es.noteApplyLoopReplay(rec0, ops); rec0.retReplay || len(got) != 2 {
		t.Error("a prefix beyond the unnamed window must decline")
	}
	// A non-param-local prefix entry: decline.
	rec1 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*core.Type{core.TAny}}
	ops = []EmitOperand{ConstOperand(0), loopOp}
	if got := es.noteApplyLoopReplay(rec1, ops); rec1.retReplay || len(got) != 2 {
		t.Error("a non-param prefix must decline")
	}
	// An EVENT above the loop region: decline (only inert tails seat).
	rec2 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*core.Type{core.TAny}}
	ops = []EmitOperand{loopOp, EventOperand(8, 0)}
	if got := es.noteApplyLoopReplay(rec2, ops); rec2.retReplay || len(got) != 2 {
		t.Error("an event above the loop region must decline")
	}
	// The corpus shape: [param re-push, loop, inert tail] splits the prefix.
	rec3 := &fnUnitRec{nParams: 1, nUnnamed: 1, returns: []*core.Type{core.TAny}}
	ops = []EmitOperand{localOperand(0), loopOp, localOperand(2)}
	got := es.noteApplyLoopReplay(rec3, ops)
	if !rec3.retReplay || len(rec3.retPrefix) != 1 || len(got) != 2 {
		t.Errorf("replay split = prefix %d + ops %d (retReplay=%v), want 1+2/true",
			len(rec3.retPrefix), len(got), rec3.retReplay)
	}
}

func TestSetLoopBodyApplySourceOrderGate(t *testing.T) {
	newES := func() (*EmitState, core.Value) {
		es := NewEmitState()
		es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
		fn := daFnCarrier("fnv")
		es.units[len(es.units)-1].localByID["fnv"] = 0
		return es, fn
	}
	arg := func(row, col int) core.Value {
		v := core.NewInteger(1)
		v.SetPos(core.SrcPos{Row: row, Col: col})
		return v
	}
	evAt := func(row, col int) EmitEvent {
		return EmitEvent{kind: evCall, call: emitCall{pos: core.SrcPos{Row: row, Col: col}}}
	}

	// No source anchor anywhere: decline.
	es, fn := newES()
	body := &EmitFragment{}
	if es.setLoopBodyApply(body, []core.Value{fn, core.NewInteger(1)}) {
		t.Error("an apply with no source anchor cannot be ordered — decline")
	}
	// Events strictly BEFORE the apply: the apply-last hoist.
	es, fn = newES()
	body = &EmitFragment{events: []EmitEvent{evAt(1, 5), {kind: evCall, call: emitCall{}}}}
	if !es.setLoopBodyApply(body, []core.Value{fn, arg(1, 40)}) || body.applyFirst {
		t.Errorf("events before the apply must seat apply-LAST (applyFirst=%v)", body.applyFirst)
	}
	// Events strictly AFTER: apply-FIRST (a zero-pos event counts as neither).
	es, fn = newES()
	body = &EmitFragment{events: []EmitEvent{evAt(1, 60)}}
	if !es.setLoopBodyApply(body, []core.Value{fn, arg(1, 10)}) || !body.applyFirst {
		t.Errorf("events after the apply must seat apply-FIRST (applyFirst=%v)", body.applyFirst)
	}
	// Events on BOTH sides: a mid-body apply — decline.
	es, fn = newES()
	body = &EmitFragment{events: []EmitEvent{evAt(1, 5), evAt(1, 60)}}
	if es.setLoopBodyApply(body, []core.Value{fn, arg(1, 30)}) {
		t.Error("a mid-body apply cannot seat at either end — decline")
	}
	// A fn operand that is neither a frame local nor an event (a baked inert
	// fn CONST recovered via its original) cannot seat the per-iteration
	// apply — decline.
	es, _ = newES()
	cfn := core.NewCarrier(core.TFunction)
	cfn.ID = "cfn"
	es.origByID["cfn"] = core.NewFunction(core.FnDefInfo{Name: "", Signatures: []core.Signature{{Returns: []*core.Type{core.TAny}, BarrierPos: -1}}})
	body = &EmitFragment{}
	if es.setLoopBodyApply(body, []core.Value{cfn, arg(1, 10)}) {
		t.Error("a const fn operand must decline the loop apply")
	}
}

func TestReplayForceOrderArms(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[5] = eventFlags{variadicResult: true}

	// In-order [event bottom, inert top]: no event above a non-event — nil.
	if got := es.replayForceOrder([]EmitOperand{EventOperand(0, 0), localOperand(1)}); got != nil {
		t.Errorf("in-order residual must not force-order, got %v", got)
	}
	// Out-of-order [inert bottom, event top] — the stylesheet-apply shape:
	// promote the event so the RET re-pushes in token order.
	got := es.replayForceOrder([]EmitOperand{localOperand(1), EventOperand(3, 0)})
	if got == nil || !got[3] || len(got) != 1 {
		t.Errorf("out-of-order residual must force-order {3}, got %v", got)
	}
	// A MULTI-RESULT event operand (resIdx != 0) cannot be re-pushed as one
	// slot — decline (nil), keep the seating compile failure.
	if got := es.replayForceOrder([]EmitOperand{localOperand(1), EventOperand(3, 1)}); got != nil {
		t.Errorf("a multi-result operand must decline force-order, got %v", got)
	}
	// A VARIADIC event operand (runtime-variable count) cannot store to one
	// slot — decline (nil).
	if got := es.replayForceOrder([]EmitOperand{localOperand(1), EventOperand(5, 0)}); got != nil {
		t.Errorf("a variadic operand must decline force-order, got %v", got)
	}
}

func TestReplayIsBodyTailArms(t *testing.T) {
	es := NewEmitState()
	at := func(row, col int) core.SrcPos { return core.SrcPos{Row: row, Col: col} }
	win := func(pos core.SrcPos) []core.Value {
		v := core.NewInteger(1)
		v.SetPos(pos)
		return []core.Value{daFnCarrier("f"), v}
	}
	evAt := func(p core.SrcPos) EmitEvent { return EmitEvent{kind: evCall, call: emitCall{pos: p}} }

	// An event-free body is trivially a tail.
	if !es.replayIsBodyTail(&EmitFragment{}, win(at(1, 30))) {
		t.Error("an event-free body is a tail")
	}
	// Events all BEFORE the window anchor: still a tail.
	frag := &EmitFragment{events: []EmitEvent{evAt(at(1, 5))}}
	if !es.replayIsBodyTail(frag, win(at(1, 30))) {
		t.Error("events before the apply keep the replay")
	}
	// An event AFTER the anchor: its effects would reorder — decline.
	frag = &EmitFragment{events: []EmitEvent{evAt(at(1, 60))}}
	if es.replayIsBodyTail(frag, win(at(1, 30))) {
		t.Error("an event after the apply must decline the replay")
	}
	// Events but NO positioned window value: order unprovable — decline.
	frag = &EmitFragment{events: []EmitEvent{evAt(at(1, 5))}}
	if es.replayIsBodyTail(frag, win(core.SrcPos{})) {
		t.Error("an unanchored window with events must decline")
	}
}

func TestReplayIsBodyTailSeqAnchor(t *testing.T) {
	// A window holding an EVENT RESULT orders by the recorded trace (seq),
	// not source columns: the nested-paren argument of the stylesheet apply
	// `(rules get (Fmt.kind nd))` sits textually after its consumer but ran
	// before it, and the get result carrier carries no Pos of its own.
	evSeq := func(seq int) EmitEvent {
		return EmitEvent{seq: seq, kind: evCall, call: emitCall{pos: core.SrcPos{Row: 1, Col: 99}}}
	}
	dynAt := func(id string) core.Value {
		v := core.NewCarrier(core.TAny)
		v.Dynamic = true
		v.ID = id
		return v
	}

	// The window's producer is the trace's LAST event: a tail — arm.
	es := NewEmitState()
	res := dynAt("r1")
	es.producedBy[res.ID] = producer{seq: 7}
	frag := &EmitFragment{events: []EmitEvent{evSeq(3), evSeq(7)}}
	if !es.replayIsBodyTail(frag, []core.Value{core.NewInteger(1), res}) {
		t.Error("a window produced by the trace's last event is the body tail")
	}
	// An event recorded AFTER the window's producer (`[(m get "k") print
	// "x"]`): the replay would reorder its effects behind the apply — decline.
	es2 := NewEmitState()
	res2 := dynAt("r2")
	es2.producedBy[res2.ID] = producer{seq: 3}
	frag2 := &EmitFragment{events: []EmitEvent{evSeq(3), evSeq(7)}}
	if es2.replayIsBodyTail(frag2, []core.Value{core.NewInteger(1), res2}) {
		t.Error("an event recorded after the window's producer must decline")
	}
}
