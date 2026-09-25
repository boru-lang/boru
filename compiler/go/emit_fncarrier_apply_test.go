package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestResolveDynamicApplyReadSubstitutedLeadFailsToCompile pins the Stage 1
// leading-apply guard: a READ-substituted fn-carrier lead (a def-read of a
// name bound to a computed fn — defReads carries its ID) with no
// statically-known closure shape declines, because the interpreter
// word-dispatches such a read while OpCallDynamic's island runs
// anonymous-VALUE semantics — for a named Go-impl fn value the two diverge
// (`def k (FnUtil.const 7)  (k 99)`: 7 interpreted, 99 from the island).
// The same lead WITHOUT the def-read tag (an event-provenance apply, the
// `((FnUtil.const 7) 99)` spelling) keeps today's lowering — the island
// mirrors the interpreter's value semantics exactly there.
func TestResolveDynamicApplyReadSubstitutedLeadFailsToCompile(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}}
	lead := core.NewCarrier(core.TFunction)
	residual := []core.Value{lead, core.NewCarrier(core.TInteger)}
	_, op, reason := es.resolveDynamicApply(lw, residual)
	if op != OpCallDynamic || reason != "" {
		t.Fatalf("an event-provenance carrier lead must keep the lowering: op=%v reason=%q", op, reason)
	}
	es.defReads = map[string]string{lead.ID: "k"}
	_, op, reason = es.resolveDynamicApply(lw, residual)
	if op != 0 || !strings.Contains(reason, "def-bound computed fn apply") {
		t.Fatalf("a read-substituted lead with unknown closure shape must decline: op=%v reason=%q", op, reason)
	}
}

// TestRecordDispatchRematchDeclinesReadSubstitutedCarrier pins the rematch
// window guard: a READ-substituted payload-less fn-carrier window value
// poisons the window (the interpreter word-dispatches that read before the
// word ever collects — `def q (if c [fn-arm] [fn-arm])  add 1 (q 5)`
// re-matched add over [1, fn, 5] and raised where the interpreter computes
// 7), so the record declines and the caller's compile failure stands.
func TestRecordDispatchRematchDeclinesReadSubstitutedCarrier(t *testing.T) {
	es := NewEmitState()
	carrier := core.NewCarrier(core.TFunction)
	es.defReads = map[string]string{carrier.ID: "q"}
	if es.RecordDispatchRematchValues("add", []core.Value{core.NewInteger(1), carrier}, []int{0}, core.SrcPos{}) {
		t.Error("a read-substituted fn-carrier window value must decline the rematch")
	}
}

// TestRecordDynApplyNameArms pins the §4.3 name-route apply's decline arms
// and its success path over a recorded def-site bind.
func TestRecordDynApplyNameArms(t *testing.T) {
	es := NewEmitState()
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny}, BarrierPos: -1,
	}}})
	out := core.NewCarrier(core.TAny)
	arg := core.NewInteger(5)

	if es.RecordDynApplyName("", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("an empty name must decline")
	}
	if es.RecordDynApplyName("h", []core.Value{arg}, core.NewInteger(1), out, core.SrcPos{}) {
		t.Error("a non-fn value must decline")
	}
	qfn := fn
	qfn.Quoted = true
	if es.RecordDynApplyName("h", []core.Value{arg}, qfn, out, core.SrcPos{}) {
		t.Error("a quoted fn must decline")
	}
	multi := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TAny, core.TAny}, BarrierPos: -1,
	}}})
	if es.RecordDynApplyName("h", []core.Value{arg}, multi, out, core.SrcPos{}) {
		t.Error("a multi-return fn must decline")
	}
	if es.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a name with no recorded def-site bind must decline")
	}

	// A recorded def-site bind with an event operand: the apply records.
	bound := core.NewCarrier(core.TFunction)
	es.frames[0] = append(es.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: 4}})
	seqBefore := len(es.frames[0])
	if !es.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Fatal("a bound name with a resolvable arg must record")
	}
	if len(es.frames[0]) != seqBefore+1 {
		t.Fatalf("expected one appended apply event, frames grew %d", len(es.frames[0])-seqBefore)
	}
	// A def site with NO operand home declines.
	es2 := NewEmitState()
	es2.frames[0] = append(es2.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: -1}})
	if es2.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a def site with no operand home must decline")
	}
	// An fn-valued ARG declines.
	es3 := NewEmitState()
	es3.frames[0] = append(es3.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", srcSeq: 4}})
	if es3.RecordDynApplyName("h", []core.Value{fn}, fn, out, core.SrcPos{}) {
		t.Error("an fn-valued arg must decline")
	}
	// An UNRESOLVABLE arg (a provenance-less carrier) declines.
	if es3.RecordDynApplyName("h", []core.Value{core.NewCarrier(core.TInteger)}, fn, out, core.SrcPos{}) {
		t.Error("an unresolvable arg must decline")
	}
	// A def site recorded with a SRC operand (a capture/local slot) is the
	// other resolution arm.
	es4 := NewEmitState()
	es4.frames[0] = append(es4.frames[0], EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "h", src: localOperand(0), srcSeq: -1}})
	if !es4.RecordDynApplyName("h", []core.Value{arg}, fn, out, core.SrcPos{}) {
		t.Error("a src-operand def site must record")
	}
	_ = bound
}

// TestRecordDynApplyEventLeadKeepQ pins the §9c event-lead gate's compile failure
// arm: an event-provenance fn CARRIER with neither a compiled-factory
// producer nor a concrete single-sig arity proof marks the program
// uncompilable (the quote-state compile failure). The PROVEN path — a
// producer-arity match recording the KeepQ apply — is pinned end-to-end by
// frontier-hof-audit.tsv §9c's `(2 (mk 4))` row, and the quoted runtime
// arm by the eng-side TestCallDynTrailKeepQQuotedStaysData.
func TestRecordDynApplyEventLeadKeepQ(t *testing.T) {
	es := NewEmitState()
	carrier := core.NewCarrier(core.TFunction)
	// Dynamic: the operand resolver's type-body screen exempts the
	// checker's gradual stand-ins, which is what a real event out is.
	carrier.Dynamic = true
	es.frames[0] = append(es.frames[0], EmitEvent{kind: evCall})
	es.producedBy[carrier.ID] = producer{seq: 0}
	out := core.NewCarrier(core.TInteger)
	if _, ok := es.RecordDynApply([]core.Value{core.NewInteger(5)}, carrier, out, core.SrcPos{}); ok {
		t.Fatal("an unprovable event-provenance carrier lead must decline")
	}
	if es.Compilable || es.Reason == "" {
		t.Errorf("the compile failure must mark the program (compilable=%v reason=%q)", es.Compilable, es.Reason)
	}

	// The concrete single-sig arity proof: a capture-bearing (unbakeable)
	// anonymous fn resolved through its producing event records the KeepQ
	// apply, consuming exactly its declared arity out of the window.
	// Dynamic exempts the operand resolver's type-body screen, as a real
	// event out is.
	//
	// A WIDER window used to decline here. It no longer does, and the change
	// is measured rather than relaxed: the interpreter UNDER-APPLIES —
	// `(1 2 (mk 4))` with a 1-arg adder nets [1, 6], the deeper 1 surviving
	// — so the faithful lowering consumes the top `arity` values and leaves
	// the rest, which is what the returned `consumed` tells the collapse
	// site to remove. The NARROWER window is the shape that still declines,
	// asserted below.
	mkFn := func() core.Value {
		v := core.NewFunction(core.FnDefInfo{Anonymous: true,
			Captured: []core.CapturedBinding{{Name: "c", Value: core.NewCarrier(core.TInteger)}},
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}}})
		v.Dynamic = true
		return v
	}
	es2 := NewEmitState()
	fn := mkFn()
	es2.frames[0] = append(es2.frames[0], EmitEvent{kind: evCall})
	es2.producedBy[fn.ID] = producer{seq: 0}
	if _, ok := es2.RecordDynApply([]core.Value{core.NewInteger(5)}, fn, core.NewCarrier(core.TInteger), core.SrcPos{}); !ok {
		t.Fatalf("a sig-proven event lead must record (reason %q)", es2.Reason)
	}
	rec := es2.frames[0][len(es2.frames[0])-1]
	if rec.kind != evCall || !rec.call.dynApplyKeepQuote {
		t.Error("the sig-proven event-lead apply must carry dynApplyKeepQuote")
	}
	// WIDER window: records, consuming only the callee's own arity.
	es3 := NewEmitState()
	fn3 := mkFn()
	es3.frames[0] = append(es3.frames[0], EmitEvent{kind: evCall})
	es3.producedBy[fn3.ID] = producer{seq: 0}
	consumed, ok := es3.RecordDynApply([]core.Value{core.NewInteger(1), core.NewInteger(2)}, fn3, core.NewCarrier(core.TInteger), core.SrcPos{})
	if !ok {
		t.Fatalf("a wider window must UNDER-APPLY, not decline (reason %q)", es3.Reason)
	}
	if consumed != 1 {
		t.Errorf("consumed = %d, want 1 — the deeper window value must survive", consumed)
	}
	if !es3.Compilable {
		t.Errorf("under-application must not mark the program uncompilable (reason %q)", es3.Reason)
	}

	// NARROWER window: the interpreter leaves the fn UNAPPLIED in the
	// residual and nothing here models that, so it declines — and the compile failure
	// must MARK, not merely decline. A bare decline lets the collapse site's
	// RegisterTrailingApply fallback lower the window anyway and answer a
	// silent wrong value (measured: `(5 (mk2 10))` answered 15 against the
	// interpreter's [5, fn]).
	two := core.NewFunction(core.FnDefInfo{Anonymous: true,
		Captured: []core.CapturedBinding{{Name: "c", Value: core.NewCarrier(core.TInteger)}},
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}}})
	two.Dynamic = true
	es4 := NewEmitState()
	es4.frames[0] = append(es4.frames[0], EmitEvent{kind: evCall})
	es4.producedBy[two.ID] = producer{seq: 0}
	if _, ok := es4.RecordDynApply([]core.Value{core.NewInteger(5)}, two, core.NewCarrier(core.TInteger), core.SrcPos{}); ok {
		t.Fatal("a window NARROWER than the callee's arity must decline")
	}
	if es4.Compilable {
		t.Error("the narrower window must MARK the program uncompilable, not silently decline")
	}
}
