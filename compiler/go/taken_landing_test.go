package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// takenFixture records one call event producing a fresh carrier and returns
// the recorder, the carrier and the event's seq.
func takenFixture(word string, nout int) (*EmitState, core.Value, int) {
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: word, nout: nout}})
	v := core.NewDynamicCarrier(core.TAny)
	es.producedBy[v.ID] = producer{seq: seq}
	return es, v, seq
}

// TestNoteTakenLandingGates pins NoteTakenLanding (NUR349): a single-result
// member read a dispatch took owes a COLLECTING landing, marked taken; an
// inactive recorder, a value with no producer, a def-bound read (its name's
// guard owns it, NUR123), any other producer and a multi-result read note
// nothing.
func TestNoteTakenLandingGates(t *testing.T) {
	// The member read: landed, collecting, taken.
	es, v, seq := takenFixture("dot", 1)
	es.NoteTakenLanding(v)
	if _, landed := es.landingAfter[seq]; !landed || !es.landingCollects(seq) || !es.landingTaken[seq] {
		t.Errorf("a taken member read must owe a collecting landing: after=%v next=%v taken=%v", es.landingAfter, es.landingNext, es.landingTaken)
	}
	// A second note on another read keeps the first.
	v2 := core.NewDynamicCarrier(core.TAny)
	seq2 := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "get", nout: 1}})
	es.producedBy[v2.ID] = producer{seq: seq2}
	es.NoteTakenLanding(v2)
	if !es.landingTaken[seq] || !es.landingTaken[seq2] {
		t.Errorf("both reads are taken: %v", es.landingTaken)
	}

	for _, c := range []struct {
		why   string
		setup func() (*EmitState, core.Value)
	}{
		{"an inactive recorder", func() (*EmitState, core.Value) {
			return &EmitState{}, core.NewDynamicCarrier(core.TAny)
		}},
		{"no producing event", func() (*EmitState, core.Value) {
			return NewEmitState(), core.NewDynamicCarrier(core.TAny)
		}},
		{"a producer seq with no event", func() (*EmitState, core.Value) {
			es := NewEmitState()
			v := core.NewDynamicCarrier(core.TAny)
			es.producedBy[v.ID] = producer{seq: 99}
			return es, v
		}},
		{"a def-bound read", func() (*EmitState, core.Value) {
			es, v, _ := takenFixture("dot", 1)
			es.defReads = map[string]string{v.ID: "x"}
			return es, v
		}},
		{"a native's result that is no member read", func() (*EmitState, core.Value) {
			es, v, _ := takenFixture("add", 1)
			return es, v
		}},
		{"a multi-result read", func() (*EmitState, core.Value) {
			es, v, _ := takenFixture("get", 2)
			return es, v
		}},
		{"a branch's join", func() (*EmitState, core.Value) {
			es := NewEmitState()
			seq := es.appendEvent(EmitEvent{kind: evBranch, br: &emitBranch{}})
			v := core.NewDynamicCarrier(core.TAny)
			es.producedBy[v.ID] = producer{seq: seq}
			return es, v
		}},
	} {
		es, v := c.setup()
		es.NoteTakenLanding(v)
		if len(es.landingAfter) != 0 || len(es.landingTaken) != 0 {
			t.Errorf("%s: must note nothing, got after=%v taken=%v", c.why, es.landingAfter, es.landingTaken)
		}
	}
}

// TestTakenLandingFail pins the lowering's verdict on a TAKEN landing
// (NUR349): guardable at a body's own depth — the root's or a fn unit's —
// and a decline inside a fragment, where the landing keeps today's arms.
func TestTakenLandingFail(t *testing.T) {
	es := NewEmitState()
	es.landingTaken = map[int]bool{3: true}
	for _, c := range []struct {
		why  string
		lw   *lowerer
		seq  int
		fail bool
	}{
		{"an untaken landing", &lowerer{es: es, depth: 2}, 4, false},
		{"the root's own depth", &lowerer{es: es, landingRoot: true}, 3, false},
		{"a fn unit's own depth", &lowerer{es: es, isFnUnit: true}, 3, false},
		{"inside a fragment", &lowerer{es: es, landingRoot: true, depth: 1}, 3, true},
		{"neither the root nor a unit", &lowerer{es: es}, 3, true},
	} {
		if got := c.lw.takenLandingFail(c.seq); (got != "") != c.fail {
			t.Errorf("%s: takenLandingFail = %q, want fail=%v", c.why, got, c.fail)
		}
	}
	// emitLandingAfter returns the verdict, spending the note and emitting
	// nothing.
	es.landingAfter = map[int]core.SrcPos{3: {}}
	var code []Instr
	var debug []core.SrcPos
	lw := &lowerer{es: es, code: &code, debug: &debug, depth: 1}
	if reason := lw.emitLandingAfter(&EmitEvent{seq: 3}, &emitCall{nout: 1}); !strings.Contains(reason, "NUR349") || len(code) != 0 {
		t.Errorf("a fragment's taken landing must decline: %q %v", reason, code)
	}
}

// TestUnitTakenLandingFail pins the unit twin (NUR349): a unit whose
// residual is a tail apply chain or a trailing apply keeps today's arms and
// guards no landing (guardUnitLandings), so a taken one there declines; a
// unit the guard serves does not.
func TestUnitTakenLandingFail(t *testing.T) {
	es := NewEmitState()
	es.landingTaken = map[int]bool{5: true}
	for _, c := range []struct {
		why  string
		unit *fnUnitRec
		seq  int
		fail bool
	}{
		{"a unit the guard serves", &fnUnitRec{name: "g"}, 5, false},
		{"a trailing-apply unit", &fnUnitRec{name: "g", dynTrailArity: 1}, 5, true},
		{"an apply-chain unit", &fnUnitRec{name: "g", applyChain: []applyStep{{}}}, 5, true},
		{"an untaken landing", &fnUnitRec{name: "g", dynTrailArity: 1}, 6, false},
	} {
		lw := &lowerer{es: es, isFnUnit: true, rec: c.unit}
		if got := lw.takenLandingFail(c.seq); (got != "") != c.fail {
			t.Errorf("%s: takenLandingFail = %q, want fail=%v", c.why, got, c.fail)
		}
	}
}

// TestRerunBranch pins the branch a statement island may run again whole
// (NUR343): every event its condition and arms hold is a read or such a
// branch itself. An arm holding any other event — an effect, a binding —
// is not one.
func TestRerunBranch(t *testing.T) {
	read := EmitEvent{kind: evCall, call: emitCall{word: "dot", nout: 1}}
	effect := EmitEvent{kind: evCall, call: emitCall{word: "print"}}
	bind := EmitEvent{kind: evDynBind}
	frag := func(evs ...EmitEvent) *EmitFragment { return &EmitFragment{events: evs} }
	nested := EmitEvent{kind: evBranch, br: &emitBranch{then: frag(read)}}
	badNested := EmitEvent{kind: evBranch, br: &emitBranch{els: frag(effect)}}
	for _, c := range []struct {
		why  string
		ev   EmitEvent
		want bool
	}{
		{"a call is no branch", read, false},
		{"constant arms", EmitEvent{kind: evBranch, br: &emitBranch{}}, true},
		{"reads in the arms and the condition", EmitEvent{kind: evBranch, br: &emitBranch{condFrag: frag(read), then: frag(read), els: frag(read)}}, true},
		{"a nested re-runnable branch", EmitEvent{kind: evBranch, br: &emitBranch{then: frag(nested)}}, true},
		{"an effect in an arm", EmitEvent{kind: evBranch, br: &emitBranch{then: frag(read, effect)}}, false},
		{"a binding in an arm", EmitEvent{kind: evBranch, br: &emitBranch{els: frag(bind)}}, false},
		{"a nested branch with an effect", EmitEvent{kind: evBranch, br: &emitBranch{then: frag(badNested)}}, false},
	} {
		if got := rerunBranch(&c.ev); got != c.want {
			t.Errorf("%s: rerunBranch = %v, want %v", c.why, got, c.want)
		}
	}
}
