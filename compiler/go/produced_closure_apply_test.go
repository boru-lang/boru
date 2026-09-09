package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// produced_closure_apply_test.go pins the twenty-eighth increment's recorder
// pieces: the pending-apply lookup by sig body (PendingClosureApply), the
// produced-fn-value predicate the `apply` word's registration reads
// (producedFnValue), and Finalize's refusal of a pending application no
// dispatch consumed.

func TestPendingClosureApply(t *testing.T) {
	es := NewEmitState()
	body := []core.Value{core.NewInteger(1)}
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: body}}}})
	if _, ok := es.PendingClosureApply(body); ok {
		t.Fatal("no pending entry: must miss")
	}
	u := es.units[len(es.units)-1]
	u.pendingApply = []pendingApply{{id: "carrier"}, {id: fn.ID, fn: fn}}
	if _, ok := es.PendingClosureApply(nil); ok {
		t.Fatal("an empty body must miss")
	}
	got, ok := es.PendingClosureApply(body)
	if !ok || got.ID != fn.ID {
		t.Fatal("the produced-closure entry must match its own sig body; a carrier entry is skipped")
	}
	if _, ok := es.PendingClosureApply([]core.Value{core.NewInteger(1)}); ok {
		t.Fatal("an equal body in another array is another construction: must miss")
	}
	if _, ok := (*EmitState)(nil).PendingClosureApply(body); ok {
		t.Fatal("a nil recorder must miss")
	}
	if _, ok := (&EmitState{}).PendingClosureApply(body); ok {
		t.Fatal("a recorder with no unit must miss")
	}
}

func TestProducedFnValue(t *testing.T) {
	es := NewEmitState()
	if es.producedFnValue("nope") {
		t.Fatal("an unknown id is not a produced fn value")
	}
	// A closure a user call's unit returns.
	es.fnRecs = append(es.fnRecs,
		&fnUnitRec{outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}},
		&fnUnitRec{nParams: 1})
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 0, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	es.producedBy["closure"] = producer{seq: 0}
	if !es.producedFnValue("closure") {
		t.Fatal("a closure a unit returns is a produced fn value")
	}
	// The result of a compiled fn-value apply (the staged combinators' inner
	// apply nets the next closure).
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 1, kind: evCall, call: emitCall{word: wordDynApply}})
	es.producedBy["applied"] = producer{seq: 1}
	if !es.producedFnValue("applied") {
		t.Fatal("a fn-value apply's result is a produced fn value")
	}
	// A plain native call's result is neither.
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 2, kind: evCall, call: emitCall{word: "add"}})
	es.producedBy["sum"] = producer{seq: 2}
	if es.producedFnValue("sum") {
		t.Fatal("a native call's result is not a produced fn value")
	}
}

func TestFinalizeLeftoverPendingApply(t *testing.T) {
	es := NewEmitState()
	es.Compilable = true
	es.units[0].pendingApply = []pendingApply{{id: "x"}}
	if p, reason, ok := es.Finalize(nil); ok || p != nil || !strings.Contains(reason, "never dispatched") {
		t.Fatalf("a pending apply on the program unit must refuse: ok=%v reason=%q", ok, reason)
	}
}
