package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// W9 callable_words.go coverage for the directly-callable helpers of the
// check-mode closure machinery: compileClosureBody's inactive-recorder
// decline, mutableInstanceRef, lambdaHookCompatible's shape gates,
// emptyContainerConst, and bodyToksHaveSentinel.

func TestW9CompileClosureBodyInactive(t *testing.T) {
	r := newTestRegistry(t)
	// A fresh registry has the inactive (no-op) recorder, so es is a typed-nil
	// *EmitState and StartFnCompile declines.
	unit, ok := compileClosureBody(r, "w", 1, false,
		[]core.Value{core.NewWord("x")}, []core.Value{core.NewInteger(1)}, nil, nil, nil, ClosureInValue, core.SrcPos{})
	if ok || unit != -1 {
		t.Errorf("inactive recorder should decline: unit=%d ok=%v", unit, ok)
	}
}

func TestW9MutableInstanceRef(t *testing.T) {
	// A flex node is a mutable instance.
	if !mutableInstanceRef(core.NewFlexList(nil)) {
		t.Error("a flex list is a mutable instance ref")
	}
	// A value with a nil Parent but a concrete payload is not (and is not a
	// bare type node, so it reaches the p==nil guard).
	if mutableInstanceRef(core.Value{Data: core.IntPayload{N: 1}}) {
		t.Error("a nil-Parent concrete value is not a mutable instance ref")
	}
	// A bare type node is excluded.
	if mutableInstanceRef(core.NewTypeLiteral(core.TInteger)) {
		t.Error("a bare type node is not a mutable instance ref")
	}
}

func TestW9LambdaHookCompatible(t *testing.T) {
	body := core.Boru([]core.Value{core.NewWord("x")})
	r := newTestRegistry(t)

	// No own sig (empty) → decline.
	if _, ok := lambdaHookCompatible(r, &core.FnDefInfo{}, nil, ClosureInValue, false, false); ok {
		t.Error("a sig-less fn should decline")
	}
	// An own sig with an empty body → decline.
	if _, ok := lambdaHookCompatible(r, &core.FnDefInfo{Signatures: []core.Signature{{}}}, nil, ClosureInValue, false, false); ok {
		t.Error("an empty-body sig should decline")
	}
	// More than one own sig → decline.
	two := &core.FnDefInfo{Signatures: []core.Signature{
		{Params: []core.FnParam{{Name: "a"}}, Impl: body},
		{Params: []core.FnParam{{Name: "b"}}, Impl: body},
	}}
	if _, ok := lambdaHookCompatible(r, two, []core.Value{core.NewInteger(1)}, ClosureInValue, false, false); ok {
		t.Error("a multi-overload fn should decline")
	}
	// A body carrying a flow-control sentinel → decline.
	sentinel := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "a"}}, Impl: core.Boru([]core.Value{core.NewWord("break")}),
	}}}
	if _, ok := lambdaHookCompatible(r, sentinel, []core.Value{core.NewInteger(1)}, ClosureInValue, false, false); ok {
		t.Error("a sentinel body should decline")
	}
	// Param / input arity mismatch → decline.
	arity := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "a"}, {Name: "b"}}, Impl: body,
	}}}
	if _, ok := lambdaHookCompatible(r, arity, []core.Value{core.NewInteger(1)}, ClosureInValue, false, false); ok {
		t.Error("an arity mismatch should decline")
	}
	// A nil-typed param is skipped; the compatible lambda is returned.
	okFd := &core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "a"}}, Impl: body, // Type nil
	}}}
	if _, ok := lambdaHookCompatible(r, okFd, []core.Value{core.NewInteger(1)}, ClosureInValue, false, false); !ok {
		t.Error("a nil-typed param should be skipped and the lambda accepted")
	}
	// The capture gate is caller-selected: the extras/hook path (false)
	// refuses a capturing lambda; the body-lambda path (true) admits it.
	capFd := &core.FnDefInfo{
		Signatures: []core.Signature{{Params: []core.FnParam{{Name: "a"}}, Impl: body}},
		Captured:   []core.CapturedBinding{{Name: "kv", Value: core.NewInteger(9)}},
	}
	if _, ok := lambdaHookCompatible(r, capFd, []core.Value{core.NewInteger(1)}, ClosureInValue, false, false); ok {
		t.Error("the hook path must refuse a capturing lambda")
	}
	if _, ok := lambdaHookCompatible(r, capFd, []core.Value{core.NewInteger(1)}, ClosureInValue, true, false); !ok {
		t.Error("the body path must admit a capturing lambda")
	}
}

func TestW9EmptyContainerConst(t *testing.T) {
	// Non-concrete → false.
	if emptyContainerConst(core.NewCarrier(core.TList)) {
		t.Error("a carrier is not an empty container const")
	}
	// A non-nil empty list / empty map → true.
	if !emptyContainerConst(core.NewList([]core.Value{})) {
		t.Error("an empty list is an empty container const")
	}
	if !emptyContainerConst(core.NewMap(core.NewOrderedMap())) {
		t.Error("an empty map is an empty container const")
	}
	// A concrete non-container → false.
	if emptyContainerConst(core.NewInteger(1)) {
		t.Error("an integer is not an empty container const")
	}
	// A non-empty list → false.
	if emptyContainerConst(core.NewList([]core.Value{core.NewInteger(1)})) {
		t.Error("a non-empty list is not an empty container const")
	}
}

func TestW9RecordClosureDispatchInactive(t *testing.T) {
	r := newTestRegistry(t)
	// The inactive recorder is not a *EmitState → the whole path declines.
	if recordClosureDispatch(r, "w", core.CallableSpec{}, nil, nil, nil, nil, nil, nil, ClosureInValue, nil, nil, nil, nil, core.SrcPos{}, nil) {
		t.Error("an inactive recorder should decline the closure dispatch")
	}
}

func TestW9LambdaCallbackInputs(t *testing.T) {
	r := newTestRegistry(t)

	// LambdaSharesTokenShape with a nil Inputs func → decline.
	if _, _, ok := lambdaCallbackInputs(r, "w", core.CallableSpec{LambdaSharesTokenShape: true}, nil); ok {
		t.Error("a nil Inputs func should decline")
	}
	// LambdaSharesTokenShape with an Inputs func that returns nil → decline.
	nilIns := core.CallableSpec{LambdaSharesTokenShape: true, Inputs: func(_ []core.Value) []core.Value { return nil }}
	if _, _, ok := lambdaCallbackInputs(r, "w", nilIns, nil); ok {
		t.Error("an Inputs func returning nil should decline")
	}
	// LambdaSharesTokenShape with a non-nil Inputs → accepted.
	okIns := core.CallableSpec{LambdaSharesTokenShape: true, Inputs: func(_ []core.Value) []core.Value { return []core.Value{core.NewCarrier(core.TInteger)} }}
	if _, _, ok := lambdaCallbackInputs(r, "w", okIns, nil); !ok {
		t.Error("a non-nil Inputs func should be accepted")
	}
	// Non-shared shape whose data operand is past the end of args → decline.
	if _, _, ok := lambdaCallbackInputs(r, "w", core.CallableSpec{BodyPos: 5}, []core.Value{core.NewInteger(1)}); ok {
		t.Error("a body position past the operand end should decline")
	}
}

func TestW9BodyToksHaveSentinel(t *testing.T) {
	if !bodyToksHaveSentinel([]core.Value{core.NewWord("break")}) {
		t.Error("a break token is a sentinel")
	}
	if bodyToksHaveSentinel([]core.Value{core.NewWord("plain")}) {
		t.Error("a plain word is not a sentinel")
	}
}

func TestW9KeyValCarrierRegistered(t *testing.T) {
	// The KeyVal type is kernel-declared (keyval.go), so keyValCarrier tags
	// the carrier with TKeyVal directly — the former registered-or-plain-Map
	// fallback probe is gone (ADR-012 stage 2). The v field carries the
	// element type; a nil registry is fine (the parameter is unused).
	out := keyValCarrier(nil, core.TInteger)
	if !out.Parent.Equal(core.TKeyVal) {
		t.Errorf("keyValCarrier should tag with TKeyVal, got %v", out.Parent)
	}
	m, err := core.AsMap(out)
	if err != nil {
		t.Fatalf("keyValCarrier payload should be a readable map, got err=%v", err)
	}
	v, ok := m.Get(core.KeyValV)
	if !ok || !v.Parent.Equal(core.TInteger) {
		t.Errorf("the v field should carry the element type, got ok=%v %v", ok, v.Parent)
	}
}

func TestW9EmptyFlexHookOperand(t *testing.T) {
	newES := func() (*EmitState, core.Value) {
		es := NewEmitState()
		v := core.NewCarrier(core.TFlexList)
		v.ID = "w9flex"
		es.producedBy[v.ID] = producer{seq: es.seq, idx: 0}
		return es, v
	}

	// Empty top-level frame → no producing event → decline.
	es, v := newES()
	if es.emptyFlexHookOperand(v) {
		t.Error("an empty frame should decline")
	}

	// A last event that is NOT the flex construction → decline.
	es, v = newES()
	es.frames[0] = []EmitEvent{{seq: es.seq, kind: evCall, call: emitCall{word: "notflex"}}}
	if es.emptyFlexHookOperand(v) {
		t.Error("a non-flex last event should decline")
	}

	// A matching flex event but a nil registry → decline (cannot verify sig).
	es, v = newES()
	es.frames[0] = []EmitEvent{{seq: es.seq, kind: evCall, call: emitCall{word: "flex"}}}
	es.reg = nil
	if es.emptyFlexHookOperand(v) {
		t.Error("a nil registry should decline")
	}

	// A matching flex event but the registry has no `flex` word → decline.
	es, v = newES()
	es.frames[0] = []EmitEvent{{seq: es.seq, kind: evCall, call: emitCall{word: "flex"}}}
	es.reg = newTestRegistry(t)
	if es.emptyFlexHookOperand(v) {
		t.Error("a registry with no flex word should decline")
	}
}

// A fn value DEFINED in another module must never lower to a closure unit:
// the unit would be compiled against the CALLING registry, so the body's free
// words would resolve there instead of where the function was written
// (design/FUNCTION-VALUE-SCOPE.0.md §12.3). Declining hands the body to the
// runtime callback seam, which runs it on its own registry — the same place the
// interpreter runs it — so the two engines agree.
//
// The guard lives in lambdaHookCompatible rather than at its two call sites so
// that BOTH the body-lambda path and the extras/hook path (walk's ascend slot)
// inherit it from one branch.
//
// UPDATED 2026-08-27 (Stage 3): the decline is now CONDITIONAL on foreignOK.
// The body-lambda call site passes true, because it compiles the body against
// the defining registry with the caller's CheckState shared onto it; the
// extras/hook site passes false, because it has no per-hook capture layout to
// share and keeps the island. Both polarities are asserted below.
func TestW9LambdaHookDeclinesForeignRegistry(t *testing.T) {
	r := newTestRegistry(t)
	other := newTestRegistry(t)
	body := core.Boru([]core.Value{core.NewWord("x")})
	sig := core.Signature{Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Impl: body}

	// Same registry (Registry nil — a fn defined in the running scope): admitted.
	local := &core.FnDefInfo{Signatures: []core.Signature{sig}}
	if _, ok := lambdaHookCompatible(r, local, []core.Value{core.NewInteger(1)}, ClosureInValue, true, false); !ok {
		t.Error("a locally-defined lambda must still be admitted")
	}
	// Foreign defining registry, foreignOK false (the extras/hook site): declined.
	foreign := &core.FnDefInfo{Signatures: []core.Signature{sig}, Registry: other}
	if _, ok := lambdaHookCompatible(r, foreign, []core.Value{core.NewInteger(1)}, ClosureInValue, true, false); ok {
		t.Error("without foreignOK a fn value from another module must decline the closure lowering")
	}
	// …and ADMITTED with foreignOK, which is the body-lambda site: that caller
	// shares the caller's CheckState onto the defining registry before
	// compiling, so the body resolves its free words at home and the unit
	// still lands in the caller's program.
	if _, ok := lambdaHookCompatible(r, foreign, []core.Value{core.NewInteger(1)}, ClosureInValue, true, true); !ok {
		t.Error("with foreignOK a fn value from another module must be admitted")
	}
	// A fn whose defining registry IS the compiling one is not foreign.
	same := &core.FnDefInfo{Signatures: []core.Signature{sig}, Registry: r}
	if _, ok := lambdaHookCompatible(r, same, []core.Value{core.NewInteger(1)}, ClosureInValue, true, false); !ok {
		t.Error("Registry == r is the same module, not a foreign one")
	}
}
