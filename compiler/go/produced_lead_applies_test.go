package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// chainState is a hand-built recording of a curried factory chain:
//
//	unit 0  mk3/1        → PUSH_CLOSURE unit 1        (returns [Any]: a fn value's count contract)
//	unit 1  fnval$body   [b:Integer] → PUSH_CLOSURE unit 2
//	unit 2  fnval$body   [c:Integer] → an add event   (returns [Integer])
//	seq 1   evCallUser unit 0             → "c1"  (the factory's closure)
//	seq 2   wordDynApply over seq 1's out → "o1"  (the chain's next closure)
//	seq 3   wordDynApply over seq 2's out → "o2"  (the chain's value, an Integer)
func chainState() *EmitState {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{
		{name: "mk3", nParams: 1, paramTypes: []*core.Type{core.TInteger}, paramPatterns: []*core.Value{nil}, returns: []*core.Type{core.TFunction}, outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}},
		{name: "fnval$body", nParams: 1, paramTypes: []*core.Type{core.TInteger}, paramPatterns: []*core.Value{nil}, returns: []*core.Type{core.TAny}, outOps: []EmitOperand{{kind: opClosure, closureUnit: 2}}},
		{name: "fnval$body", nParams: 1, paramTypes: []*core.Type{core.TInteger}, paramPatterns: []*core.Value{nil}, returns: []*core.Type{core.TInteger}, outOps: []EmitOperand{EventOperand(9, 0)}},
	}
	es.frames = [][]EmitEvent{{
		{seq: 1, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}},
		{seq: 2, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{EventOperand(1, 0), {kind: opConst}}}},
		{seq: 3, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{EventOperand(2, 0), {kind: opConst}}}},
	}}
	es.producedBy = map[string]producer{"c1": {seq: 1}, "o1": {seq: 2}, "o2": {seq: 3}}
	return es
}

func intArgs(ns ...int64) []core.Value {
	out := make([]core.Value, len(ns))
	for i, n := range ns {
		out[i] = core.NewInteger(n)
	}
	return out
}

// TestProducedLeadApplies pins the recorder seam the curried-chain arm asks
// (core's parenProducedLeadApplyIdx): a produced closure's declared contract
// against the window, level by level down the chain.
func TestProducedLeadApplies(t *testing.T) {
	es := chainState()
	ret, ok := es.ProducedLeadApplies("c1", intArgs(2))
	if !ok || ret == nil || !ret.Equal(core.TFunction) {
		t.Errorf("the factory's closure over one Integer applies and nets a Function (its declared return): (%v, %v)", ret, ok)
	}
	ret, ok = es.ProducedLeadApplies("o1", intArgs(3))
	if !ok || ret == nil || !ret.Equal(core.TInteger) {
		t.Errorf("the first apply's result is unit 2's closure (unit 1's out-op), declared Integer: got (%v, %v)", ret, ok)
	}
	if _, ok = es.ProducedLeadApplies("o2", intArgs(4)); ok {
		t.Error("the second apply's result is unit 2's add event — no closure to apply")
	}
	// Declines.
	for name, c := range map[string]struct {
		id   string
		args []core.Value
	}{
		"unknown id":       {"nope", intArgs(2)},
		"arity mismatch":   {"c1", intArgs(2, 3)},
		"type mismatch":    {"c1", []core.Value{core.NewString("s")}},
		"a Number carrier": {"c1", []core.Value{core.NewCarrier(core.TNumber)}},
		"empty id":         {"", intArgs(2)},
	} {
		if _, ok := es.ProducedLeadApplies(c.id, c.args); ok {
			t.Errorf("%s: must decline", name)
		}
	}
	es.defReads = map[string]string{"c1": "h"}
	if _, ok := es.ProducedLeadApplies("c1", intArgs(2)); ok {
		t.Error("a def-read binding is the read model's, never the paren's")
	}
	delete(es.defReads, "c1")
	es.fnRecs[1].paramPatterns = []*core.Value{{}}
	if _, ok := es.ProducedLeadApplies("c1", intArgs(2)); ok {
		t.Error("a param pattern is not a static contract")
	}
	es.fnRecs[1].paramTypes = nil
	if _, ok := es.ProducedLeadApplies("c1", intArgs(2)); ok {
		t.Error("a unit with no recorded params has no contract")
	}
	// An unnamed-param frame keeps the whole-frame replay (NUR180).
	es = chainState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "each$body", nParams: 1, nUnnamed: 1})
	es.openUnitRecs = append(es.openUnitRecs, 3)
	if _, ok := es.ProducedLeadApplies("c1", intArgs(2)); ok {
		t.Error("inside an unnamed-param frame the arm stands aside")
	}
	// Inactive.
	es = chainState()
	es.Compilable = false
	if _, ok := es.ProducedLeadApplies("c1", intArgs(2)); ok {
		t.Error("an inactive recorder answers nothing")
	}
}

// TestEventProducedFnOpArms pins the out-op walk's declines: an apply whose
// fn operand is a local, a second result, a non-closure applied operand, a
// closure unit with no single out-op, an unknown seq, a call event that is
// not an apply, and the depth bound over a self-referencing apply.
func TestEventProducedFnOpArms(t *testing.T) {
	es := chainState()
	if op, ok := es.eventProducedFnOp(1, 0); !ok || op.kind != opClosure || op.closureUnit != 1 {
		t.Errorf("a user call's single out-op: (%+v, %v)", op, ok)
	}
	if _, ok := es.eventProducedFnOp(42, 0); ok {
		t.Error("an unknown seq has no fn value")
	}
	if _, ok := es.eventProducedFnOp(1, 9); ok {
		t.Error("the depth bound declines")
	}
	es.frames[0] = append(es.frames[0],
		EmitEvent{seq: 10, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{{kind: opLocal}}}},
		EmitEvent{seq: 11, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{EventOperand(1, 1)}}},
		EmitEvent{seq: 12, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{{kind: opConst}}}},
		EmitEvent{seq: 13, kind: evCall, call: emitCall{word: "add", nout: 1, ops: []EmitOperand{EventOperand(1, 0)}}},
		EmitEvent{seq: 14, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{EventOperand(14, 0)}}},
		EmitEvent{seq: 15, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{{kind: opClosure, closureUnit: 7}}}},
		EmitEvent{seq: 16, kind: evCall, call: emitCall{word: wordDynApply, nout: 1, ops: []EmitOperand{EventOperand(2, 0)}}},
		EmitEvent{seq: 17, kind: evCallUser, uc: emitUserCall{unit: 7, nout: 1}},
	)
	for name, seq := range map[string]int{
		"a local fn operand":            10,
		"a second result":               11,
		"a const applied":               12,
		"not an apply":                  13,
		"a self-referencing apply":      14,
		"a unit out of range":           15,
		"a user call unit out of range": 17,
	} {
		if _, ok := es.eventProducedFnOp(seq, 0); ok {
			t.Errorf("%s: must decline", name)
		}
	}
	// The applied closure's unit has no single out-op.
	es.fnRecs[2].outOps = nil
	if _, ok := es.eventProducedFnOp(16, 0); ok {
		t.Error("a closure unit netting no single value has no fn value to hand on")
	}
	if _, ok := es.producerReturnedOutOpSeq(3); ok {
		t.Error("an apply event is not a user call")
	}
	if s, ok := es.eventProducedFnShape(1, 0); !ok || s.Arity != 1 || s.Result == nil || s.Result.Arity != 1 {
		t.Errorf("the shape walk rides the same out-op: %+v %v", s, ok)
	}
	if _, ok := es.eventProducedFnShape(42, 0); ok {
		t.Error("no out-op, no shape")
	}
}

// TestFnOpContract pins the contract read off an out-op: a closure unit's
// declared params / patterns / return (the out-op's construction when the
// return is the count contract's Any), a const lambda's own signature, and
// the declines.
func TestFnOpContract(t *testing.T) {
	es := chainState()
	c, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 2})
	if !ok || len(c.params) != 1 || !c.params[0].Equal(core.TInteger) || c.ret == nil || !c.ret.Equal(core.TInteger) {
		t.Errorf("unit 2: declared Integer param and return: %+v %v", c, ok)
	}
	c, ok = es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1})
	if !ok || c.ret == nil || !c.ret.Equal(core.TFunction) {
		t.Errorf("unit 1: an Any return whose out-op pushes a closure is a Function: %+v %v", c, ok)
	}
	// An Any return over a const-lambda out-op is a Function too; over
	// anything else it stays Any.
	lam := core.NewValueRaw(core.TFunction, core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}}, Returns: []*core.Type{core.TInteger},
		Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}},
	}}})
	es.consts = []core.Value{lam, core.NewInteger(7)}
	es.fnRecs[1].outOps = []EmitOperand{{kind: opConst, idx: 0}}
	if c, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1}); !ok || !c.ret.Equal(core.TFunction) {
		t.Errorf("a const lambda out-op is a Function: %+v %v", c, ok)
	}
	es.fnRecs[1].outOps = []EmitOperand{{kind: opConst, idx: 1}}
	if c, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1}); !ok || !c.ret.Equal(core.TAny) {
		t.Errorf("a const 7 out-op says nothing: %+v %v", c, ok)
	}
	es.fnRecs[1].outOps = []EmitOperand{EventOperand(1, 0)}
	if c, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1}); !ok || !c.ret.Equal(core.TAny) {
		t.Errorf("an event out-op says nothing: %+v %v", c, ok)
	}
	es.fnRecs[1].outOps = nil
	es.fnRecs[1].returns = nil
	if _, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1}); ok {
		t.Error("no declared return and no single out-op: nothing vouches for one value")
	}
	es.fnRecs[1].returns = []*core.Type{core.TInteger}
	if c, ok := es.fnOpContract(EmitOperand{kind: opClosure, closureUnit: 1}); !ok || !c.ret.Equal(core.TInteger) {
		t.Errorf("a declared return alone vouches: %+v %v", c, ok)
	}
	// The const-lambda arm.
	c, ok = es.fnOpContract(EmitOperand{kind: opConst, idx: 0})
	if !ok || len(c.params) != 1 || !c.params[0].Equal(core.TInteger) || !c.ret.Equal(core.TInteger) {
		t.Errorf("a const lambda's own signature: %+v %v", c, ok)
	}
	if _, ok := es.fnOpContract(EmitOperand{kind: opConst, idx: 1}); ok {
		t.Error("a const 7 is no lambda")
	}
	undeclared := core.NewValueRaw(core.TFunction, core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger, Optional: true}},
		Impl:   &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}},
	}}})
	es.consts = append(es.consts, undeclared)
	if _, ok := es.fnOpContract(EmitOperand{kind: opConst, idx: 2}); ok {
		t.Error("an optional param is not a static contract")
	}
	quoteP := core.NewValueRaw(core.TFunction, core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TAtom, Quote: true}},
		Impl:   &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}},
	}}})
	es.consts = append(es.consts, quoteP)
	if _, ok := es.fnOpContract(EmitOperand{kind: opConst, idx: 3}); ok {
		t.Error("a /q param is not a static contract")
	}
	plain := core.NewValueRaw(core.TFunction, core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}},
		Impl:   &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}},
	}}})
	es.consts = append(es.consts, plain)
	if c, ok := es.fnOpContract(EmitOperand{kind: opConst, idx: 4}); !ok || !c.ret.Equal(core.TAny) {
		t.Errorf("an undeclared const-lambda return is Any: %+v %v", c, ok)
	}
	for name, op := range map[string]EmitOperand{
		"a local":             {kind: opLocal},
		"an event":            EventOperand(1, 0),
		"a unit out of range": {kind: opClosure, closureUnit: 99},
		"a negative unit":     {kind: opClosure, closureUnit: -1},
	} {
		if _, ok := es.fnOpContract(op); ok {
			t.Errorf("%s: no contract", name)
		}
	}
}

// TestArgConformsStatically pins the argument gate: an Any (or absent) param
// takes any data value; otherwise the argument's own concrete type must
// conform, and a disjunct or untyped argument never does.
func TestArgConformsStatically(t *testing.T) {
	two := core.NewInteger(2)
	if !argConformsStatically(two, nil) || !argConformsStatically(two, core.TAny) {
		t.Error("an Any param takes any data value")
	}
	if !argConformsStatically(two, core.TInteger) || !argConformsStatically(two, core.TNumber) {
		t.Error("an Integer conforms to Integer and Number")
	}
	if argConformsStatically(core.NewString("s"), core.TInteger) {
		t.Error("a String is no Integer")
	}
	if argConformsStatically(core.NewCarrier(core.TNumber), core.TInteger) {
		t.Error("a Number carrier may not be an Integer")
	}
	if argConformsStatically(core.Value{}, core.TInteger) {
		t.Error("an untyped value conforms to nothing")
	}
	dj := core.NewDisjunct([]core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)})
	if argConformsStatically(dj, core.TInteger) {
		t.Error("a disjunct is not one type")
	}
}
