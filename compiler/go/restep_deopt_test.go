package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// restep_deopt_test.go pins the RE-STEP point (NUR124): the recorder's note,
// the planner's placement and declines, and the lowering — an OpDeoptIfFn
// over the results the event left on top, with the unit's unpushed unnamed
// params seated as the island's prefix — or the refusal a fn-typed note no
// point serves.

// reStepUnit opens a closure-shaped unit over one UNNAMED fn-typed input
// whose body is `5 swap drop 9`: swap (seq 1) re-produces the input on top,
// drop (seq 2) consumes it. The note lands on swap's fn-typed result with
// resume at drop's token.
func reStepUnit(t *testing.T, body []core.Value, resume core.SrcPos, outOps ...EmitOperand) (*EmitState, *emitUnit, *fnUnitRec) {
	t.Helper()
	es := NewEmitState()
	in := core.NewCarrier(core.TFunction)
	unit, _, ok := es.StartFnCompile("k", "each$body", nil, []core.Value{in}, nil, []string{""}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	u := es.units[len(es.units)-1]
	rec := es.fnRecs[unit]
	five := es.internUnpooled(core.NewInteger(5))
	out0, out1 := core.NewCarrier(core.TInteger), core.NewCarrier(core.TFunction)
	es.producedBy[out0.ID] = producer{seq: 1}
	es.producedBy[out1.ID] = producer{seq: 1, idx: 1}
	rec.frag = &EmitFragment{events: []EmitEvent{
		{seq: 1, kind: evCall, call: emitCall{word: "swap", nout: 2, pos: deoptAt(3), ops: []EmitOperand{localOperand(0), ConstOperand(five)}}},
		{seq: 2, kind: evCall, call: emitCall{word: "drop", nout: 0, pos: deoptAt(8), ops: []EmitOperand{EventOperand(1, 1)}}},
	}}
	rec.outOps = outOps
	es.SetUnitBody(unit, body)
	es.NoteFnResultReStep(out1, resume)
	return es, u, rec
}

func TestNoteFnResultReStepArms(t *testing.T) {
	es := NewEmitState()
	fn := core.NewCarrier(core.TFunction)
	// No id, or a result no event produced (a fold's, a const's): nothing
	// to test after, nothing noted.
	es.NoteFnResultReStep(core.Value{}, deoptAt(1))
	es.NoteFnResultReStep(fn, deoptAt(1))
	if len(es.reStepNotes) != 0 {
		t.Fatalf("an unproduced result notes nothing: %v", es.reStepNotes)
	}
	es.producedBy[fn.ID] = producer{seq: 4}
	dyn := core.NewDynamicCarrier(core.TAny)
	es.producedBy[dyn.ID] = producer{seq: 4, idx: 1}
	es.NoteFnResultReStep(dyn, deoptAt(1))
	if n := es.reStepNotes[4]; n.strict || n.resume != deoptAt(1) {
		t.Errorf("a gradual result notes non-strict at its resume: %+v", n)
	}
	es.NoteFnResultReStep(fn, deoptAt(2))
	if n := es.reStepNotes[4]; !n.strict || n.resume != deoptAt(2) {
		t.Errorf("a fn-typed result marks the event's note strict: %+v", n)
	}
	es.NoteFnResultReStep(dyn, deoptAt(3))
	if n := es.reStepNotes[4]; !n.strict {
		t.Errorf("strict is sticky: %+v", n)
	}
	// An inactive recorder notes nothing.
	es.Compilable = false
	es.NoteFnResultReStep(fn, deoptAt(9))
	if es.reStepNotes[4].resume != deoptAt(3) {
		t.Error("an inactive recorder must not note")
	}
}

func TestPlanReStepDeopts(t *testing.T) {
	body := []core.Value{deoptLit(core.NewInteger(5), 1), deoptTok("swap", 3), deoptTok("drop", 8), deoptLit(core.NewInteger(9), 13)}
	// The point sits after swap, resuming at drop's token (2); the unit
	// carries the deopt env.
	es, u, rec := reStepUnit(t, body, deoptAt(8))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 1 || !rec.deopts[0].restep || rec.deopts[0].seq != 1 || rec.deopts[0].token != 2 || rec.deopts[0].start != deoptAt(8) || !rec.deoptEnv || !u.deoptEnv {
		t.Errorf("a re-step point after swap, resuming at drop: %+v env=%v", rec.deopts, rec.deoptEnv)
	}
	// A resume position that is no token of this body declines.
	es, u, rec = reStepUnit(t, body, deoptAt(99))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 || rec.deoptEnv {
		t.Errorf("a resume outside the body declines: %+v", rec.deopts)
	}
	// A residual literal written BEFORE the resume that the unit pushes only
	// at its RET is on the interpreter's stack at the point and not on the
	// compiled one: the deferred-operand accounting declines.
	es, u, rec = reStepUnit(t, []core.Value{deoptLit(core.NewInteger(7), 1), deoptLit(core.NewInteger(5), 3), deoptTok("swap", 5), deoptTok("drop", 10)}, deoptAt(10))
	rec.outOps = []EmitOperand{ConstOperand(es.internUnpooled(core.NewInteger(7)))}
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("a deferred residual literal declines: %+v", rec.deopts)
	}
	// No notes at all: nothing planned, the env untouched.
	es = NewEmitState()
	es.planReStepDeopts(nil, &fnUnitRec{})
	// A unit with no body plans nothing (planDeopts' first gate).
	es, u, rec = reStepUnit(t, nil, deoptAt(8))
	es.planDeopts(u, rec)
	if len(rec.deopts) != 0 {
		t.Errorf("no body, no point: %+v", rec.deopts)
	}
}

func TestEmitReStepAfter(t *testing.T) {
	es := NewEmitState()
	es.reStepNotes = map[int]reStepNote{1: {resume: deoptAt(8), strict: true}, 3: {resume: deoptAt(20)}}
	swap := &EmitEvent{seq: 1, kind: evCall, call: emitCall{word: "swap", nout: 2, pos: deoptAt(3)}}
	fresh := func(vm []vmSlot) (*lowerer, *[]Instr, *[]DeoptSpec) {
		var code []Instr
		var dbg []core.SrcPos
		var tbl []DeoptSpec
		lw := &lowerer{es: es, code: &code, debug: &dbg, deoptTable: &tbl, vm: vm, unnamedParams: []int{0},
			deoptAfterSeq: map[int]deoptPoint{1: {seq: 1, token: 2, start: deoptAt(8), pos: deoptAt(3), restep: true}}}
		return lw, &code, &tbl
	}
	// The planned point, the event's two results on top: one op over both
	// results, the unpushed unnamed input seated as the prefix.
	lw, code, tbl := fresh([]vmSlot{{seq: 1, idx: 0}, {seq: 1, idx: 1}})
	if reason := lw.emitReStepAfter(swap); reason != "" {
		t.Fatalf("planned: %s", reason)
	}
	if len(*code) != 1 || (*code)[0].Op != OpDeoptIfFn || len(*tbl) != 1 || (*tbl)[0].Results != 2 || (*tbl)[0].Token != 2 || len((*tbl)[0].Prefix) != 1 || (*tbl)[0].Prefix[0] != 0 {
		t.Errorf("one DEOPT_IF_FN over 2 results with prefix [0]: %v %+v", *code, *tbl)
	}
	if _, still := lw.deoptAfterSeq[1]; still {
		t.Error("a lowered point is spent")
	}
	// The input already pushed at the root: no prefix.
	lw, _, tbl = fresh([]vmSlot{{seq: 1, idx: 0}, {seq: 1, idx: 1}})
	lw.emit(OpPushLocal, 0, deoptAt(1))
	*lw.code = (*lw.code)[:0]
	if reason := lw.emitReStepAfter(swap); reason != "" || len((*tbl)[0].Prefix) != 0 {
		t.Errorf("a pushed input is on the stack region already: %s %+v", reason, *tbl)
	}
	// The input pushed inside a nested fragment: the point cannot know
	// whether the interpreter still holds it, and a strict note refuses.
	lw, _, _ = fresh([]vmSlot{{seq: 1, idx: 0}, {seq: 1, idx: 1}})
	lw.depth = 1
	lw.emit(OpPushLocal, 0, deoptAt(1))
	lw.depth = 0
	if reason := lw.emitReStepAfter(swap); !strings.Contains(reason, "`swap`") || !strings.Contains(reason, "NUR124") {
		t.Errorf("a nested push declines the point and the strict note refuses: %q", reason)
	}
	// The results not on top (promoted to a slot): a strict note refuses.
	lw, _, _ = fresh(nil)
	if reason := lw.emitReStepAfter(swap); !strings.Contains(reason, "NUR124") {
		t.Errorf("no results on top: %q", reason)
	}
	// A gradual note with no point keeps the optimistic model; a fallback
	// event's refusal names no word.
	lw, _, _ = fresh(nil)
	if reason := lw.emitReStepAfter(&EmitEvent{seq: 3, kind: evFallback, fb: emitFallback{}}); reason != "" {
		t.Errorf("gradual, unplanned: %q", reason)
	}
	es.reStepNotes[3] = reStepNote{resume: deoptAt(20), strict: true}
	if reason := lw.emitReStepAfter(&EmitEvent{seq: 3, kind: evFallback, fb: emitFallback{}}); !strings.HasPrefix(reason, "a call:") {
		t.Errorf("a strict note on a non-call event: %q", reason)
	}
	// An event with no note, and a lowerer with no state: nothing.
	if reason := lw.emitReStepAfter(&EmitEvent{seq: 7, kind: evCall}); reason != "" {
		t.Errorf("unnoted: %q", reason)
	}
	if reason := (&lowerer{}).emitReStepAfter(swap); reason != "" {
		t.Errorf("stateless: %q", reason)
	}
}

// TestDeoptPrefixOnStatementPoints pins the prefix on NUR123's statement
// points: an unpushed unnamed input rides beneath the region, and a point
// after a NESTED push of one (an arm that may or may not have run) declines
// and is dropped rather than kept for a later statement.
func TestDeoptPrefixOnStatementPoints(t *testing.T) {
	es := NewEmitState()
	fresh := func() (*lowerer, *[]Instr, *[]DeoptSpec) {
		var code []Instr
		var dbg []core.SrcPos
		var tbl []DeoptSpec
		lw := &lowerer{es: es, code: &code, debug: &dbg, deoptTable: &tbl, unnamedParams: []int{0, 1},
			deopts: []deoptPoint{{seq: -1, slot: 2, name: "j", token: 1, pos: deoptAt(5), start: deoptAt(5)}}}
		return lw, &code, &tbl
	}
	// Slot 1 pushed at the root, slot 0 not: the prefix seats slot 0 alone.
	lw, code, tbl := fresh()
	lw.emit(OpPushLocal, 1, deoptAt(1))
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(*code) != 2 || (*code)[1].Op != OpDeoptIfFn || len(*tbl) != 1 || len((*tbl)[0].Prefix) != 1 || (*tbl)[0].Prefix[0] != 0 || (*tbl)[0].Slot != 2 {
		t.Errorf("the unpushed input is the prefix: %v %+v", *code, *tbl)
	}
	// Slot 0 pushed inside a nested fragment before the point: declined and
	// dropped.
	lw, code, tbl = fresh()
	lw.depth = 1
	lw.emit(OpPushLocal, 0, deoptAt(1))
	lw.depth = 0
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(*code) != 1 || len(*tbl) != 0 || len(lw.deopts) != 0 {
		t.Errorf("a nested push of an unnamed input declines the point: %v %+v %+v", *code, *tbl, lw.deopts)
	}
}
