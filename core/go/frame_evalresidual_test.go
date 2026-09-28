package core

import (
	"strings"
	"testing"
)

// pendingList / pendingMap build parser-shaped pending containers
// (Eval=true, unquoted) — the form a source-literal list/map token has
// when it parks as a frame residual.
func pendingList(elems ...Value) Value {
	v := NewList(elems)
	v.Eval = true
	return v
}

func pendingMap(k string, val Value) Value {
	m := NewOrderedMap()
	m.Set(k, val)
	v := NewMap(m)
	v.Eval = true
	return v
}

// TestStepDefCleanupResidualArms drives the marker's residual scan
// directly (the in-package seam): the EvalResidual gate, the
// single-literal off-state, and both container error arms.
func TestStepDefCleanupResidualArms(t *testing.T) {
	r := runReg(t)

	failList := pendingList(NewWord("cfail"), NewInteger(1))
	failMap := pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}}))
	okMap := pendingMap("a", NewInteger(3))

	mk := func(evalResidual bool, residual ...Value) (*Engine, Value, int) {
		e := New(r)
		toks := append([]Value{NewOpenParen()}, residual...)
		marker := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: evalResidual})
		toks = append(toks, marker)
		e.Tape = NewTape(toks, StackHeadroom)
		return e, marker, len(toks) - 1
	}

	// Off-state: a pending container is left untouched (the transparency).
	e, marker, idx := mk(false, okMap)
	if err := e.stepDefCleanup(marker, idx); err != nil {
		t.Fatalf("off-state cleanup: %v", err)
	}
	if !e.Tape.At(1).Eval {
		t.Fatal("EvalResidual=false must leave the residual pending")
	}

	// On-state: the pending map evaluates in place.
	e, marker, idx = mk(true, okMap)
	if err := e.stepDefCleanup(marker, idx); err != nil {
		t.Fatalf("on-state cleanup: %v", err)
	}
	if e.Tape.At(1).Eval {
		t.Fatal("EvalResidual=true must evaluate the residual")
	}

	// List error arm: the container's evaluation failure propagates.
	e, marker, idx = mk(true, failList)
	if err := e.stepDefCleanup(marker, idx); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("list error arm = %v, want boom", err)
	}

	// Map error arm.
	e, marker, idx = mk(true, failMap)
	if err := e.stepDefCleanup(marker, idx); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("map error arm = %v, want boom", err)
	}
}

// TestResidualEvalSweepAndTeardownArms drives the two SECOND-ORDER marker
// processors directly (the in-package seam): stepCloseParen's surviving-
// marker sweep and the TCO eager teardown must propagate a residual-eval
// failure exactly as the main loop does. In census programs the marker
// reaching either site has already been stepped (its containers are no
// longer pending), so these arms need the hand-built first-time-step
// tape.
func TestResidualEvalSweepAndTeardownArms(t *testing.T) {
	r := runReg(t)
	failMap := pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}}))

	// Sweep arm: a never-stepped EvalResidual marker inside a closing
	// group with a pending failing container below it.
	e := New(r)
	e.Tape = NewTape([]Value{
		NewOpenParen(),
		failMap,
		NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true}),
		NewCloseParen(),
	}, StackHeadroom)
	e.Pointer = 3
	if err := e.stepCloseParen(true, false); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("sweep arm = %v, want boom", err)
	}

	// TCO teardown arm: the eager replay of the same marker.
	e2 := New(r)
	e2.Tape = NewTape([]Value{
		NewOpenParen(),
		pendingMap("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("cfail"), NewInteger(1)}})),
		NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true}),
	}, StackHeadroom)
	if err := e2.teardownFrameState(frameTailScan{TailStart: 2, RCIdx: -1}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("teardown arm = %v, want boom", err)
	}
}

// TestBodyEvalsResidualGate pins the ONE residual-timing predicate every
// frame-tail builder and the compile-side recording admission share: a
// multi-token body computes (true), a single paren-expression token
// computes (true — grouping cannot flip the timing, PR #260 review), and
// a single bare literal keeps the consumer-scope no-closures
// transparency (false — def-node-binding.tsv §3).
func TestBodyEvalsResidualGate(t *testing.T) {
	pe := NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("def"), NewWord("z")}})
	cases := []struct {
		name string
		body []Value
		want bool
	}{
		{"nil body", nil, false},
		{"single literal map", []Value{pendingMap("a", NewInteger(1))}, false},
		{"single literal list", []Value{pendingList(NewInteger(1))}, false},
		{"single scalar", []Value{NewInteger(1)}, false},
		{"single paren expression", []Value{pe}, true},
		{"multi token", []Value{NewWord("def"), NewWord("z"), NewInteger(0)}, true},
	}
	for _, c := range cases {
		if got := BodyEvalsResidual(c.body); got != c.want {
			t.Errorf("BodyEvalsResidual(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPendingResidualContainerArms pins the pending-residual predicate
// the marker's scan and probeTailCall's below-the-call classification
// share: only a plain, unevaluated, unquoted list/map counts — every
// typed/schema container and every non-container stays out.
func TestPendingResidualContainerArms(t *testing.T) {
	pend := func(v Value) Value { v.Eval = true; return v }
	quoted := pendingMap("a", NewInteger(1))
	quoted.Quoted = true
	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"plain pending map", pendingMap("a", NewInteger(1)), true},
		{"plain pending list", pendingList(NewInteger(1)), true},
		{"already evaluated map", NewMap(NewOrderedMap()), false},
		{"quoted map", quoted, false},
		{"scalar", NewInteger(3), false},
		// A PENDING (Eval) scalar clears the Eval/Quoted/Parent guard but is
		// neither TMap nor TList, so it falls through to the terminal `return
		// false` — the non-container fall-through path.
		{"pending scalar", pend(NewInteger(3)), false},
		{"typed map", pend(NewValueRaw(TMap, ChildTypeInfo{})), false},
		{"record type", pend(NewValueRaw(TMap, RecordTypeInfo{})), false},
		{"options type", pend(NewValueRaw(TMap, OptionsTypeInfo{})), false},
		{"typed list", pend(NewValueRaw(TList, ChildTypeInfo{})), false},
		{"table", pend(NewValueRaw(TList, TableTypeInfo{})), false},
		{"map type literal", NewTypeLiteral(TMap), false},
		{"pending non-container", pend(NewInteger(3)), false},
	}
	for _, c := range cases {
		if got := isPendingResidualContainer(c.v); got != c.want {
			t.Errorf("isPendingResidualContainer(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestResidualBottomUpAndBoundaryArms pins the scan's evaluation order
// housekeeping: BOTH pending containers evaluate (bottom-up, matching
// the end-of-run sweep — the order itself is pinned end-to-end in
// lang/go/test/residual_review_test.go), and a marker with no OpenParen
// below (a bare synthetic tape) scans from the tape start.
func TestResidualBottomUpAndBoundaryArms(t *testing.T) {
	r := runReg(t)
	mkA := pendingMap("a", NewInteger(1))
	mkB := pendingMap("b", NewInteger(2))

	e := New(r)
	marker := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true})
	e.Tape = NewTape([]Value{NewOpenParen(), mkA, mkB, marker}, StackHeadroom)
	if err := e.stepDefCleanup(marker, 3); err != nil {
		t.Fatalf("two-residual cleanup: %v", err)
	}
	if e.Tape.At(1).Eval || e.Tape.At(2).Eval {
		t.Fatal("both pending residuals must evaluate")
	}

	// No OpenParen below the marker: the scan bottoms out at the tape
	// start and still evaluates the residual.
	e2 := New(r)
	marker2 := NewDefCleanup(DefCleanupInfo{Registry: r, SkipCleanup: true, EvalResidual: true})
	e2.Tape = NewTape([]Value{pendingMap("a", NewInteger(3)), marker2}, StackHeadroom)
	if err := e2.stepDefCleanup(marker2, 1); err != nil {
		t.Fatalf("boundary cleanup: %v", err)
	}
	if e2.Tape.At(0).Eval {
		t.Fatal("the tape-start residual must evaluate")
	}
}

// The residual-eval ERROR arm leaves the frame's parked tail to the run's
// fault return: the frame is still open on the tape when the marker's
// evaluation raises, so Engine.faultReturn replays its tail once —
// truncation, the __pa Args/baseline pop, the undef pairs — as it does
// for every other error raised inside a live frame (NUR201). The pins are
// TestRunErrorUnwindsFrameOnceAfterResidualError and
// TestRunErrorUnwindsLiveFrame (fn_frame_unwind_test.go); the end-to-end
// twin lives in lang/go/test/residual_review_test.go.
