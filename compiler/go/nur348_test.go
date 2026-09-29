package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur348_test.go dials NUR348's two mechanisms directly: a live read's
// point under a terminal trap (kept_live_deopt.go trapHeldBeneath, the
// lowering's stack check) and a `do`'s fired splice result
// (splice_outs.go). The whole-program pins are lang's TestNUR348*.

// trapHeldBeneath serves a trap program's live point only in the trap's own
// statement, at its first token, over a told stack every value of which an
// event left on the compiled stack — and keeps those results off the dead
// list, where the interpreter's stack holds them.
func TestTrapHeldBeneath(t *testing.T) {
	// `a end x b end z`: the trap stands at b, in x's statement.
	body := []core.Value{deoptTok("a", 1), deoptLit(core.NewEnd(), 3), deoptTok("x", 7), deoptTok("b", 9), deoptLit(core.NewEnd(), 11), deoptTok("z", 15)}
	trap := EmitEvent{seq: 8, kind: evTrap, trap: EmitTrap{pos: deoptAt(9)}}
	tree := rootTreeEvents([]EmitEvent{trap}, false)
	held, slot, lit, orphan := core.NewCarrier(core.TAny), core.NewCarrier(core.TAny), core.NewInteger(4), core.NewCarrier(core.TAny)
	held.ID, slot.ID, orphan.ID = "held", "slot", "orphan"
	fresh := func(stack ...core.Value) (*EmitState, *lowerer) {
		es := NewEmitState()
		es.rootBody, es.trapAt = body, 8
		es.producedBy[held.ID] = producer{seq: 3}
		es.producedBy[slot.ID] = producer{seq: 4}
		if stack != nil {
			es.rootStmtStacks = map[core.SrcPos][]core.Value{deoptAt(3): stack}
		}
		return es, &lowerer{es: es, promoted: map[int]int{4: 7}, dead: map[int]bool{3: true}}
	}
	for _, c := range []struct {
		name  string
		start core.SrcPos
		tree  map[int]treeEvent
		stack []core.Value
	}{
		{"a start at no token", deoptAt(8), tree, []core.Value{held}},
		{"no trap in the tree", deoptAt(7), map[int]treeEvent{}, []core.Value{held}},
		{"a start inside its statement", deoptAt(9), tree, []core.Value{held}},
		{"another statement than the trap's", deoptAt(15), tree, []core.Value{held}},
		{"an untold stack", deoptAt(7), tree, nil},
		{"a literal the stack holds", deoptAt(7), tree, []core.Value{lit}},
		{"a result promoted to a slot", deoptAt(7), tree, []core.Value{slot}},
		{"a value no event produced", deoptAt(7), tree, []core.Value{orphan}},
	} {
		es, lw := fresh(c.stack...)
		if _, ok := es.trapHeldBeneath(lw, c.tree, c.start); ok {
			t.Errorf("%s: served", c.name)
		}
		if !lw.dead[3] {
			t.Errorf("%s: a declined point keeps the dead list", c.name)
		}
	}
	es, lw := fresh(held)
	got, ok := es.trapHeldBeneath(lw, tree, deoptAt(7))
	if !ok || len(got) != 1 || got[0] != (vmSlot{seq: 3}) || lw.dead[3] {
		t.Errorf("an event's result on the stack is held and kept: %v %v dead=%v", got, ok, lw.dead)
	}
	es, lw = fresh([]core.Value{}...)
	es.rootStmtStacks = map[core.SrcPos][]core.Value{deoptAt(3): {}}
	if got, ok := es.trapHeldBeneath(lw, tree, deoptAt(7)); !ok || got == nil || len(got) != 0 {
		t.Errorf("an empty told stack is held, as an empty stack: %v %v", got, ok)
	}
}

// A trap program's live point is emitted only where the compiled stack at
// its test is the one it holds (deoptPoint.trapHeld); elsewhere it is
// dropped, and the trap keeps its own defer.
func TestEmitDeoptsBeforeTrapHeld(t *testing.T) {
	point := deoptPoint{seq: 5, slot: -1, name: "x", start: deoptAt(7), token: 2, live: &keptLiveRead{name: "x"}, trapHeld: []vmSlot{{seq: 3}}}
	cf := &CompiledFn{}
	lw := &lowerer{es: NewEmitState(), p: &Program{}, code: &cf.Code, debug: &cf.Debug, deoptTable: &cf.Deopts,
		vm: []vmSlot{{seq: 3}, {seq: 4}}, deopts: []deoptPoint{point}}
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(cf.Deopts) != 0 || len(cf.Code) != 0 || len(lw.deopts) != 0 {
		t.Errorf("a stack other than the held one emits no point: %+v %+v", cf.Deopts, cf.Code)
	}
	lw.vm, lw.deopts = []vmSlot{{seq: 3}}, []deoptPoint{point}
	lw.emitDeoptsBefore(core.SrcPos{})
	if len(cf.Deopts) != 1 || !cf.Deopts[0].Live || len(cf.Code) != 1 || cf.Code[0].Op != OpDeoptIfFn {
		t.Errorf("the held stack emits the live point: %+v %+v", cf.Deopts, cf.Code)
	}
}

// NoteSpliceFired takes a fired marker out of the noted `do` call that
// produced it, the results after it moving down; any other marker, or an
// inactive state, is left alone.
func TestNoteSpliceFired(t *testing.T) {
	marker := func() core.Value {
		return core.NewSplice(core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)}))
	}
	one, fired, three := core.NewInteger(1), marker(), core.NewInteger(3)
	one.ID, fired.ID, three.ID = "one", "fired", "three"
	fresh := func() *EmitState {
		es := NewEmitState()
		es.Compilable = true
		seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 3}})
		outs := []core.Value{one, fired, three}
		for i, o := range outs {
			es.setProducedAt(o, seq, i)
		}
		es.noteSpliceOuts(seq, outs)
		return es
	}
	es := fresh()
	seq := es.producedBy[fired.ID].seq
	es.NoteSpliceFired(fired, core.SrcPos{})
	ev := es.eventBySeq(seq)
	if ev.call.nout != 2 || es.producedBy[three.ID] != (producer{seq: seq, idx: 1}) || es.producedBy[one.ID] != (producer{seq: seq}) {
		t.Errorf("the marker leaves the call's results: nout=%d three=%+v one=%+v", ev.call.nout, es.producedBy[three.ID], es.producedBy[one.ID])
	}
	if _, still := es.producedBy[fired.ID]; still || !es.firedSplices[seq][fired.ID] {
		t.Error("the fired marker is forgotten and noted fired")
	}
	lw := &lowerer{es: es}
	if outs := lw.spliceOutsAt(seq); len(outs) != 3 || !core.IsSplice(outs[1]) {
		t.Errorf("every splice fired: the call carries the results the pass stepped: %v", outs)
	}

	// The markers left alone.
	computed := core.NewSplice(core.NewCarrier(core.TList))
	computed.ID = "computed-marker"
	unnoted := marker()
	unnoted.ID = "unnoted"
	for _, c := range []struct {
		name string
		es   func() *EmitState
		v    core.Value
	}{
		{"an inactive state", func() *EmitState { es := fresh(); es.Compilable = false; return es }, fired},
		{"an identity-less marker", fresh, core.NewSplice(core.NewInteger(1))},
		{"a computed payload's marker", fresh, computed},
		{"a marker no event produced", fresh, unnoted},
		{"a result that is no splice", fresh, one},
	} {
		es := c.es()
		es.NoteSpliceFired(c.v, core.SrcPos{})
		if ev := es.eventBySeq(seq); ev.call.nout != 3 || len(es.firedSplices) != 0 {
			t.Errorf("%s: the call's results change: nout=%d fired=%v", c.name, ev.call.nout, es.firedSplices)
		}
	}
	// A noted call that is not the current frame's, or leaves no value.
	es = fresh()
	es.frames = append(es.frames, nil)
	es.NoteSpliceFired(fired, core.SrcPos{})
	if len(es.firedSplices) != 0 {
		t.Error("a call outside the current frame keeps its results")
	}
	es = fresh()
	es.eventBySeq(seq).call.nout = 0
	es.NoteSpliceFired(fired, core.SrcPos{})
	if len(es.firedSplices) != 0 {
		t.Error("a call that leaves no value keeps its results")
	}
	// A marker the pass took as data never fired: no SpliceOuts, the
	// screen's defer.
	es = fresh()
	if outs := (&lowerer{es: es}).spliceOutsAt(seq); outs != nil {
		t.Errorf("an unfired marker carries no results: %v", outs)
	}
	if outs := (&lowerer{}).spliceOutsAt(seq); outs != nil {
		t.Errorf("no emit state carries no results: %v", outs)
	}
	// Results with no concrete marker are not noted.
	es = NewEmitState()
	es.noteSpliceOuts(1, []core.Value{one, computed})
	if es.spliceOuts != nil {
		t.Errorf("no concrete marker among the results: %v", es.spliceOuts)
	}
}

// A loop-analysis round the pass discards (Rollback) takes its splice notes
// with it: its seqs and value IDs come back in the stabilised round, where a
// marker the discarded round fired but the final round took as data must
// not stamp the call's SpliceOuts (the review of #522) — the screen's defer
// is owed there.
func TestSpliceNotesRollBack(t *testing.T) {
	marker, three := core.NewSplice(core.NewList([]core.Value{core.NewInteger(1)})), core.NewInteger(3)
	marker.ID, three.ID = "marker", "three"
	es := NewEmitState()
	record := func() int {
		seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 2}})
		outs := []core.Value{marker, three}
		for i, o := range outs {
			es.setProducedAt(o, seq, i)
		}
		es.noteSpliceOuts(seq, outs)
		return seq
	}
	cp := es.Checkpoint()
	discarded := record()
	es.NoteSpliceFired(marker, core.SrcPos{})
	if (&lowerer{es: es}).spliceOutsAt(discarded) == nil {
		t.Fatal("the discarded round's fired marker is noted")
	}
	es.Rollback(cp)
	es.frames[0] = nil // the caller's own frame truncation (check's carrier.go)
	if len(es.spliceOuts) != 0 || len(es.firedSplices) != 0 {
		t.Errorf("the rollback keeps the discarded round's splice notes: %v %v", es.spliceOuts, es.firedSplices)
	}
	final := record()
	if final != discarded {
		t.Fatalf("the final round reuses the seq: %d / %d", final, discarded)
	}
	if outs := (&lowerer{es: es}).spliceOutsAt(final); outs != nil {
		t.Errorf("a marker only the discarded round fired stamps the final call: %v", outs)
	}
}
