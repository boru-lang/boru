package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPlacedValReadAndForwardLeftoverGuards pins the nil-safe edges of the
// two collapse-mark readers NUR184 / NUR185 added: a nil state, an ID-less
// value, a value never `/v`-read, one under a pending apply, one also read
// bare, and a registry with no check state all answer false; the marked
// cases answer true.
func TestPlacedValReadAndForwardLeftoverGuards(t *testing.T) {
	var nilES *EmitState
	if nilES.placedValRead("x") || nilES.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("a nil state answers false")
	}
	es := NewEmitState()
	if es.placedValRead("") || es.placedValRead("x") {
		t.Error("an ID-less or never-read value is not a placed /v read")
	}
	es.valReadNoted = map[string]bool{"x": true, "y": true}
	if !es.placedValRead("x") {
		t.Error("a noted /v read with no registry is placed")
	}
	if es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("no registry: no leftover mark")
	}
	es.reg = &core.Registry{}
	if !es.placedValRead("x") || es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("a registry with no check state: placed, no leftover mark")
	}
	es.reg.Check = &core.CheckState{
		WordReadFnIDs:        map[string]bool{"y": true},
		ForwardLeftoverFnIDs: map[string]bool{"z": true},
	}
	if !es.placedValRead("x") {
		t.Error("x is /v-read only: placed")
	}
	if es.placedValRead("y") {
		t.Error("y is also read bare: the word dispatch wins")
	}
	if !es.forwardLeftoverFn(core.Value{ID: "z"}) || es.forwardLeftoverFn(core.Value{ID: ""}) || es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("the leftover mark is keyed by value ID")
	}
}

// TestPolyCallDeclineReasonReSteppedOperand: a native poly record that
// collected a re-step-marked fn carrier as its operand declines with
// NUR184's reason; a quoted copy, an unmarked carrier, one the `apply`
// word owns, and a plain data operand record.
func TestPolyCallDeclineReasonReSteppedOperand(t *testing.T) {
	es := NewEmitState()
	es.reg = &core.Registry{Check: &core.CheckState{ParenReSteppedFnIDs: map[string]bool{"fnc-r": true}}}
	marked := core.NewCarrier(core.TFunction)
	marked.ID = "fnc-r"
	unmarked := core.NewCarrier(core.TFunction)
	unmarked.ID = "fnc-u"
	quoted := marked
	quoted.Quoted = true
	if got := es.polyCallDeclineReason("mul", []core.Value{marked, core.NewInteger(10)}, nil, core.SrcPos{}); got != "a dispatch collected a fn value the paren's rewind re-steps first (NUR184)" {
		t.Errorf("a re-step-marked carrier operand declines: %q", got)
	}
	for name, args := range map[string][]core.Value{
		"unmarked carrier": {unmarked, core.NewInteger(10)},
		"quoted copy":      {quoted, core.NewInteger(10)},
		"plain data":       {core.NewInteger(2), core.NewInteger(10)},
	} {
		if got := es.polyCallDeclineReason("mul", args, nil, core.SrcPos{}); got != "" {
			t.Errorf("%s records: %q", name, got)
		}
	}
	// A lead the apply word owns is the apply's: no decline here.
	es.units = append(es.units, &emitUnit{pendingApply: []pendingApply{{id: "fnc-r"}}})
	if got := es.polyCallDeclineReason("mul", []core.Value{marked, core.NewInteger(10)}, nil, core.SrcPos{}); got != "" {
		t.Errorf("an apply-owned lead records: %q", got)
	}
}

// TestProducedConstLambdaGuards pins producedConstLambda's edges: an id no
// event produced, a producer whose returned out-op is not a const, a const
// index out of range and a non-fn const all answer false; a produced fn
// const answers true.
func TestProducedConstLambdaGuards(t *testing.T) {
	es := NewEmitState()
	if es.producedConstLambda("nothing") {
		t.Error("an unproduced id is not a const lambda")
	}
	fnc := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, BarrierPos: 1}}, Anonymous: true})
	es.consts = []core.Value{core.NewInteger(1), fnc}
	unit := len(es.fnRecs)
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "mk"})
	seed := func(op EmitOperand) string {
		id := "res"
		es.fnRecs[unit].outOps = []EmitOperand{op}
		es.frames = [][]EmitEvent{{{kind: evCallUser, uc: emitUserCall{unit: unit}, seq: 1}}}
		es.producedBy = map[string]producer{id: {seq: 1, idx: 0}}
		return id
	}
	if es.producedConstLambda(seed(EmitOperand{kind: opLocal, idx: 0})) {
		t.Error("a returned local is not a const")
	}
	if es.producedConstLambda(seed(EmitOperand{kind: opConst, idx: 7})) {
		t.Error("a const index out of range")
	}
	if es.producedConstLambda(seed(EmitOperand{kind: opConst, idx: 0})) {
		t.Error("an Integer const is not a fn")
	}
	if !es.producedConstLambda(seed(EmitOperand{kind: opConst, idx: 1})) {
		t.Error("a returned baked fn const is the apply arm's")
	}
}
