package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestRecordCodeBodyClosureReadArms pins the §9f guard: a code BODY
// argument that READS a name dyn-bound to a compiled closure refuses,
// because the body's native re-runs its tokens through the interpreter
// and a ClosurePayload is invokable only through the VM's re-entrant
// runner. The scope is the READ, not the bind — §9b's factory family
// applies exactly such closures from compiled code and must stay
// compiling, which a blanket refusal at the bind would have cost.
func TestRecordCodeBodyClosureReadArms(t *testing.T) {
	es := NewEmitState()
	body := core.NewList([]core.Value{core.NewWord("h"), core.NewInteger(1)})

	// No noted closures: nothing to refuse.
	if es.recordCodeBodyClosureRead([]core.Value{body}) {
		t.Error("an empty closure set must not refuse")
	}

	es.dynBoundClosures = map[string]bool{"h": true}

	// A non-list argument is not a body.
	if es.recordCodeBodyClosureRead([]core.Value{core.NewInteger(3)}) {
		t.Error("a non-list argument must not refuse")
	}
	// A list carrier carries no readable tokens.
	if es.recordCodeBodyClosureRead([]core.Value{core.NewCarrier(core.TList)}) {
		t.Error("a list carrier must not refuse")
	}
	// A body naming an UNRELATED word is untouched.
	other := core.NewList([]core.Value{core.NewWord("g"), core.NewInteger(1)})
	if es.recordCodeBodyClosureRead([]core.Value{other}) {
		t.Error("a body reading an unrelated name must not refuse")
	}
	// The shape itself: the body reads the dyn-bound closure.
	if !es.recordCodeBodyClosureRead([]core.Value{other, body}) {
		t.Fatal("a body reading a dyn-bound compiled closure must refuse")
	}
	if es.Compilable || !strings.Contains(es.Reason, "compiled closure") {
		t.Errorf("the refusal must mark the program (compilable=%v reason=%q)", es.Compilable, es.Reason)
	}
}

// TestArgIsProducedClosureArms pins the §9g guard: a compiled closure
// this pass produced (a call result the recorder knows returned an
// OpPushClosure) reaching a word's argument slot means the paren that
// should have APPLIED it did not collapse, so the word would receive the
// FUNCTION where the interpreter receives the applied result.
func TestArgIsProducedClosureArms(t *testing.T) {
	es := NewEmitState()
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
	}}})

	// A plain value is never the shape.
	if es.argIsProducedClosure("w", nil, []core.Value{core.NewInteger(5)}) {
		t.Error("a non-fn argument must not refuse")
	}
	// An fn value the pass did NOT produce as a closure is left alone —
	// a word with a genuine Function slot keeps working.
	if es.argIsProducedClosure("w", nil, []core.Value{fn}) {
		t.Error("an fn value with no produced-closure provenance must not refuse")
	}
	// The shape: the value came back from a user call whose unit returns
	// exactly one compiled closure — the factory pattern
	// producerReturnedClosureArity recognises.
	es.fnRecs = append(es.fnRecs,
		&fnUnitRec{outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}},
		&fnUnitRec{nParams: 1})
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 0, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	es.producedBy[fn.ID] = producer{seq: 0}
	// The `apply` word's one-arg overload over a CONCRETE closure value is
	// the pending-apply path (the twenty-eighth increment), never this
	// refusal; its two-arg Reach overload holds the closure as data and
	// keeps it, and so does a fn-typed CARRIER the check cannot re-step.
	if es.argIsProducedClosure("apply", nil, []core.Value{fn}) {
		t.Fatal("apply's one-arg overload over a produced closure must not refuse here")
	}
	if !es.Compilable {
		t.Fatal("the apply exemption must leave the program compilable")
	}
	if !es.argIsProducedClosure("apply", nil, []core.Value{core.NewInteger(1), fn}) {
		t.Fatal("apply's two-arg overload over a produced closure must refuse")
	}
	es.Compilable, es.Reason = true, ""
	carrier := core.NewCarrier(core.TFunction)
	es.producedBy[carrier.ID] = producer{seq: 0}
	if !es.argIsProducedClosure("apply", nil, []core.Value{carrier}) {
		t.Fatal("apply over a produced fn-typed carrier must keep the refusal")
	}
	es.Compilable, es.Reason = true, ""
	// A slot DECLARED Function binds the closure as data (the thirtieth
	// increment): a named param, a positional one; an Any slot and a nil
	// signature keep the refusal.
	fnSlot := &core.Signature{Params: []core.FnParam{{Type: core.TInteger}, {Type: core.TFunction}}}
	if es.argIsProducedClosure("w", fnSlot, []core.Value{core.NewInteger(1), fn}) {
		t.Fatal("a produced closure at a declared Function param must not refuse")
	}
	posSlot := &core.Signature{Args: []*core.Type{core.TInteger, core.TFunction}}
	if es.argIsProducedClosure("w", posSlot, []core.Value{core.NewInteger(1), fn}) {
		t.Fatal("a produced closure at a positional Function slot must not refuse")
	}
	anySlot := &core.Signature{Params: []core.FnParam{{Type: core.TInteger}, {Type: core.TAny}}}
	if !es.argIsProducedClosure("w", anySlot, []core.Value{core.NewInteger(1), fn}) {
		t.Fatal("a produced closure at an Any slot keeps the refusal")
	}
	es.Compilable, es.Reason = true, ""
	// The apply word's own Function slot is not the exception: a produced
	// fn-typed CARRIER under apply keeps the refusal whatever apply declares.
	applySig := &core.Signature{Args: []*core.Type{core.TFunction}}
	if !es.argIsProducedClosure("apply", applySig, []core.Value{carrier}) {
		t.Fatal("apply's declared Function slot does not lift the carrier refusal")
	}
	es.Compilable, es.Reason = true, ""
	if !es.argIsProducedClosure("w", nil, []core.Value{core.NewInteger(1), fn}) {
		t.Fatal("a produced compiled closure at an argument slot must refuse")
	}
	if es.Compilable || !strings.Contains(es.Reason, "argument slot") {
		t.Errorf("the refusal must mark the program (compilable=%v reason=%q)", es.Compilable, es.Reason)
	}
}
