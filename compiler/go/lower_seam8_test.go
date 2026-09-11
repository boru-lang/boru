package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// lower_seam8_test.go covers the bytecode lowerer's Stage-2/Stage-3 REFUSAL
// arms (lower.go) — the defensive layouts that make Finalize fall back to the
// interpreter rather than emit an unsound instruction stream. A well-formed
// recorded trace never produces these layouts, and the eng/lang program
// suites don't reach them, so they are driven by direct calls to the in-
// package lowerer with hand-built EmitEvent traces (the direct-call seam the
// fleet brief authorises, matching vm_seam7/emit_seam7). Each refusal returns
// a non-empty reason string; the lowerer never mutates program state past the
// refusal, so a bare *lowerer with a hand-seeded simulated stack is enough.

// w8lw builds a fresh lowerer over an empty program, ready for direct
// method calls. The returned *[]Instr is the emission target (rarely read;
// the refusal arms emit nothing).
func w8lw() *lowerer {
	p := &Program{}
	code := []Instr{}
	debug := []core.SrcPos{}
	return &lowerer{
		es: NewEmitState(), p: p, code: &code, debug: &debug,
		sigIdx:   map[*core.Signature]int{},
		variadic: map[int]bool{},
		promoted: map[int]int{},
		dead:     map[int]bool{},
	}
}

// --- orderedCarried: event-init partition + descending insertion sort ----

func TestW8OrderedCarried(t *testing.T) {
	// Two EVENT inits given in ascending idx order force a swap (the sort
	// orders event inits DESCENDING by producing-seq), and a const init
	// lands after them in registration order.
	carried := []carriedInit{
		{slot: 0, init: EventOperand(1, 0)},
		{slot: 1, init: EventOperand(2, 0)},
		{slot: 2, init: ConstOperand(5)},
	}
	got := orderedCarried(carried)
	if len(got) != 3 {
		t.Fatalf("orderedCarried len = %d, want 3", len(got))
	}
	if got[0].init.idx != 2 || got[1].init.idx != 1 {
		t.Fatalf("event inits not sorted descending: %d then %d", got[0].init.idx, got[1].init.idx)
	}
	if got[2].init.kind != opConst {
		t.Fatalf("const init did not land last: %v", got[2].init.kind)
	}
}

// --- planVariadicClaims: missing producer for a computed-else claim ------

func TestW8PlanVariadicClaimsMissingProducer(t *testing.T) {
	// A computed-else branch whose else operand references a producer seq that
	// is NOT in the event list: byseq returns nil and the claim is skipped.
	ev := EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		elsComputed: true, elsVal: EventOperand(99, 0),
	}}
	mb, ve := planVariadicClaims([]EmitEvent{ev})
	if mb != nil || ve != nil {
		t.Fatalf("missing producer should yield no claim maps: mb=%v ve=%v", mb, ve)
	}
}

// --- spillSeat: underflow + non-operand-interleaved refusals -------------

func TestW8SpillSeatUnderflow(t *testing.T) {
	lw := w8lw() // empty sim stack
	if reason := lw.spillSeat([]EmitOperand{ConstOperand(0)}, []int{0}, 1, core.SrcPos{}, "SPILLFAIL"); reason != "SPILLFAIL" {
		t.Fatalf("spillSeat underflow reason = %q, want SPILLFAIL", reason)
	}
}

func TestW8SpillSeatNonOperandInterleaved(t *testing.T) {
	lw := w8lw()
	lw.vm = []vmSlot{{seq: 99, idx: 0}} // a slot that is NOT one of the call's operands
	reason := lw.spillSeat([]EmitOperand{EventOperand(1, 0)}, []int{0}, 1, core.SrcPos{}, "SPILLFAIL")
	if reason != "SPILLFAIL" {
		t.Fatalf("non-operand interleave reason = %q, want SPILLFAIL", reason)
	}
}

// --- lowerLoop: variadic bound + count-not-on-top refusals ----------------

func TestW8LowerLoopVariadicBound(t *testing.T) {
	lw := w8lw()
	lw.variadic = map[int]bool{5: true}
	ev := &EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{end: EventOperand(5, 0)}}
	if reason := lw.lowerLoop(ev); !strings.Contains(reason, "loop results as a loop bound") {
		t.Fatalf("variadic loop bound reason = %q", reason)
	}
}

func TestW8LowerLoopCountNotOnTop(t *testing.T) {
	lw := w8lw() // empty sim: the event-count is not on top
	ev := &EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{end: EventOperand(5, 0)}}
	if reason := lw.lowerLoop(ev); !strings.Contains(reason, "count is not on top") {
		t.Fatalf("count-not-on-top reason = %q", reason)
	}
}

// --- lowerLoop: carried-init refusals (past a const-bound FOR_SETUP) -------

func w8constBoundLoop(carried []carriedInit) *EmitEvent {
	return &EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{
		start: ConstOperand(0), end: ConstOperand(3), step: ConstOperand(1),
		iterSlot: 0, carried: carried,
	}}
}

func TestW8LowerLoopCarriedVariadicInit(t *testing.T) {
	lw := w8lw()
	lw.variadic = map[int]bool{7: true}
	ev := w8constBoundLoop([]carriedInit{{slot: 0, init: EventOperand(7, 0)}})
	if reason := lw.lowerLoop(ev); !strings.Contains(reason, "carried init is a variadic result") {
		t.Fatalf("carried variadic init reason = %q", reason)
	}
}

func TestW8LowerLoopCarriedInitNotOnTop(t *testing.T) {
	lw := w8lw() // after FOR_SETUP the sim is empty, so the event init is not on top
	ev := w8constBoundLoop([]carriedInit{{slot: 0, init: EventOperand(7, 0)}})
	if reason := lw.lowerLoop(ev); !strings.Contains(reason, "carried init is not on top") {
		t.Fatalf("carried-init-not-on-top reason = %q", reason)
	}
}

// --- lowerStore: variadic + not-on-top refusals ---------------------------

func TestW8LowerStoreVariadic(t *testing.T) {
	lw := w8lw()
	lw.variadic = map[int]bool{5: true}
	ev := &EmitEvent{seq: 1, kind: evStore, store: &emitStore{src: EventOperand(5, 0), slot: 0}}
	if reason := lw.lowerStore(ev); !strings.Contains(reason, "store of a variadic result") {
		t.Fatalf("variadic store reason = %q", reason)
	}
}

func TestW8LowerStoreNotOnTop(t *testing.T) {
	lw := w8lw() // empty sim: the event source is not on top
	ev := &EmitEvent{seq: 1, kind: evStore, store: &emitStore{src: EventOperand(5, 0), slot: 0}}
	if reason := lw.lowerStore(ev); !strings.Contains(reason, "store source is not on top") {
		t.Fatalf("store-not-on-top reason = %q", reason)
	}
}

// --- lowerEvents: unknown event kind default ------------------------------

func TestW8LowerEventsUnknownKind(t *testing.T) {
	lw := w8lw()
	// An event whose kind is outside the recorded set trips the defensive
	// default (a trace built by any path other than the recorder).
	if reason := lw.lowerEvents([]EmitEvent{{seq: 1, kind: 9999}}, -1); !strings.Contains(reason, "unknown event kind") {
		t.Fatalf("unknown-event-kind reason = %q", reason)
	}
}

func TestW8LowerEventsPromotedMergeNotSingle(t *testing.T) {
	lw := w8lw()
	// A PROMOTED branch value-def whose merge is variadic (not a clean single
	// value on top) refuses the promoted store (lower.go:432). The const-cond
	// branch lowers cleanly, then the post-branch store check trips because the
	// merge slot was marked variadic.
	lw.promoted[1] = 0
	lw.variadic[1] = true
	tru := true
	ev := EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		constCond: &tru, thenIsVal: true, thenVal: ConstOperand(0), hasThenOut: true,
	}}
	if reason := lw.lowerEvents([]EmitEvent{ev}, -1); !strings.Contains(reason, "promoted merge is not a single value on top") {
		t.Fatalf("promoted-merge-not-single reason = %q", reason)
	}
}

// --- lowerLoop: carried event init that IS on top (the store + continue) ---

func TestW8LowerLoopCarriedInitOnTop(t *testing.T) {
	lw := w8lw()
	// Pre-seed the sim with the carried init's producing event so that, after
	// the const-bound FOR_SETUP consumes its three operands, the carried
	// init's slot is on top and the event-sourced store path runs.
	lw.vm = []vmSlot{{seq: 7, idx: 0}}
	ev := &EmitEvent{seq: 1, kind: evLoop, loop: &emitLoop{
		start: ConstOperand(0), end: ConstOperand(3), step: ConstOperand(1),
		iterSlot: 0, carried: []carriedInit{{slot: 0, init: EventOperand(7, 0)}},
		body: &EmitFragment{}, hasBodyOut: false,
	}}
	if reason := lw.lowerLoop(ev); reason != "" {
		t.Fatalf("carried event init on top should lower cleanly, got %q", reason)
	}
}

// --- layoutOperands: reverse-fast-path miss + N-event spill default -------

func TestW8LayoutOperandsReverseMissSpills(t *testing.T) {
	lw := w8lw()
	// Three event operands whose sim slots match neither the in-order nor the
	// reverse layout: the reverse fast-path bails (slotIs miss) and the
	// switch default spills — which, with no matching sim, refuses.
	lw.vm = []vmSlot{{seq: 9, idx: 0}, {seq: 9, idx: 0}, {seq: 9, idx: 0}}
	ops := []EmitOperand{EventOperand(1, 0), EventOperand(2, 0), EventOperand(3, 0)}
	msg := layoutMsgs{shapeBeyond: "SHAPEBEYOND"}
	if reason := lw.layoutOperands(ops, core.SrcPos{}, msg); reason != "SHAPEBEYOND" {
		t.Fatalf("3-event non-matching layout reason = %q, want SHAPEBEYOND", reason)
	}
}

func TestW8LayoutOperandsResultNotTop(t *testing.T) {
	lw := w8lw() // empty sim: the lone event operand is not on top
	ops := []EmitOperand{EventOperand(1, 0), ConstOperand(0)}
	msg := layoutMsgs{resultNotTop: "RESULTNOTTOP"}
	if reason := lw.layoutOperands(ops, core.SrcPos{}, msg); reason != "RESULTNOTTOP" {
		t.Fatalf("single-result-not-on-top reason = %q, want RESULTNOTTOP", reason)
	}
}

// --- seatResults: aboveLiteral / reordered / unconsumed refusals ----------

func w8seatMsgs() seatMsgs {
	return seatMsgs{variadic: "VAR", aboveLiteral: "ABOVELIT", reordered: "REORDERED", unconsumed: "UNCONSUMED"}
}

func TestW8SeatResultsAboveLiteral(t *testing.T) {
	lw := w8lw()
	lw.vm = []vmSlot{{seq: 1, idx: 0}}
	// An inert operand pushed as tail, then an event operand ABOVE it.
	ops := []EmitOperand{ConstOperand(0), EventOperand(1, 0)}
	if reason := lw.seatResults(ops, false, false, w8seatMsgs(), core.SrcPos{}); reason != "ABOVELIT" {
		t.Fatalf("event-above-literal reason = %q, want ABOVELIT", reason)
	}
}

func TestW8SeatResultsReordered(t *testing.T) {
	lw := w8lw() // empty sim: the event is not the next slot
	ops := []EmitOperand{EventOperand(1, 0)}
	if reason := lw.seatResults(ops, false, false, w8seatMsgs(), core.SrcPos{}); reason != "REORDERED" {
		t.Fatalf("event-reordered reason = %q, want REORDERED", reason)
	}
}

func TestW8SeatResultsUnconsumed(t *testing.T) {
	lw := w8lw()
	lw.vm = []vmSlot{{seq: 1, idx: 0}} // a sim slot no operand consumes
	if reason := lw.seatResults(nil, false, false, w8seatMsgs(), core.SrcPos{}); reason != "UNCONSUMED" {
		t.Fatalf("unconsumed-sim reason = %q, want UNCONSUMED", reason)
	}
}

// --- fragmentOuts: a branch event with a nil branch ----------------------

func TestW8FragmentOutsNilBranch(t *testing.T) {
	if outs := fragmentOuts(&EmitEvent{kind: evBranch, br: nil}); outs != nil {
		t.Fatalf("nil-branch fragmentOuts = %v, want nil", outs)
	}
}

// --- lowerFragment refusal / drop arms -----------------------------------

// w8pushEvent is a minimal make-list call event that pushes nout result
// slots without operands (OpMakeList over 0 operands), so a fragment's
// simulated residual can be shaped for lowerFragment's post-event checks.
func w8pushEvent(seq, nout int) EmitEvent {
	return EmitEvent{seq: seq, kind: evCall, call: emitCall{makeList: true, nout: nout}}
}

func TestW8LowerFragmentDepthLimit(t *testing.T) {
	lw := w8lw()
	lw.depth = maxLowerDepth // the next lowerFragment++ overflows the bound
	if reason := lw.lowerFragment(&EmitFragment{}, nil, false, core.SrcPos{}); !strings.Contains(reason, "beyond the compile depth limit") {
		t.Fatalf("depth-limit reason = %q", reason)
	}
}

func TestW8LowerFragmentApplyDiverges(t *testing.T) {
	lw := w8lw()
	// A loop-body apply fragment whose last event diverges (a raise-like
	// CompileDiverges call): the apply is skipped and the scope restored.
	frag := &EmitFragment{
		events:    []EmitEvent{{seq: 5, kind: evCall, call: emitCall{makeList: true, nout: 0, diverges: true}}},
		applyArgs: []EmitOperand{ConstOperand(0)},
	}
	if reason := lw.lowerFragment(frag, nil, false, core.SrcPos{}); reason != "" {
		t.Fatalf("diverging apply fragment should lower cleanly, got %q", reason)
	}
}

func TestW8LowerFragmentApplyNotSole(t *testing.T) {
	lw := w8lw()
	// applyArgs present but the body left no leading fn value (empty residual).
	frag := &EmitFragment{applyArgs: []EmitOperand{ConstOperand(0)}}
	if reason := lw.lowerFragment(frag, nil, false, core.SrcPos{}); !strings.Contains(reason, "leading fn value not the sole residual") {
		t.Fatalf("apply-not-sole reason = %q", reason)
	}
}

func TestW8LowerFragmentOutNilExtra(t *testing.T) {
	lw := w8lw()
	// out is nil (a net-0 body) but the events left a value on the sim.
	frag := &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}}
	if reason := lw.lowerFragment(frag, nil, false, core.SrcPos{}); !strings.Contains(reason, "body leaves extra values") {
		t.Fatalf("out-nil-extra reason = %q", reason)
	}
}

func TestW8LowerFragmentMultiValueNoVariadic(t *testing.T) {
	lw := w8lw()
	// A multi-value arm (residualN>1) presented to a position that cannot
	// absorb a variadic merge (allowVariadic false) refuses.
	out := EventOperand(1, 0)
	frag := &EmitFragment{residualN: 2}
	if reason := lw.lowerFragment(frag, &out, false, core.SrcPos{}); !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("multi-value-no-variadic reason = %q", reason)
	}
}

func TestW8LowerFragmentEventOutMismatch(t *testing.T) {
	lw := w8lw()
	// Single-value event out, but the sim slot does not match it.
	out := EventOperand(9, 0)
	frag := &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}}
	if reason := lw.lowerFragment(frag, &out, false, core.SrcPos{}); !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("event-out-mismatch reason = %q", reason)
	}
}

func TestW8LowerFragmentLeftoverRefuse(t *testing.T) {
	lw := w8lw()
	// A const out with a leftover side-effect event on the sim, at a position
	// that may NOT trim (allowVariadic false) refuses.
	out := ConstOperand(0)
	frag := &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}}
	if reason := lw.lowerFragment(frag, &out, false, core.SrcPos{}); !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("leftover-refuse reason = %q", reason)
	}
}

func TestW8LowerFragmentLeftoverDrop(t *testing.T) {
	lw := w8lw()
	// Same shape but at an allowVariadic branch arm: the leftover side-effect
	// result is DROPped and the const result pushed (the drop-and-seat arm).
	out := ConstOperand(0)
	frag := &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}}
	if reason := lw.lowerFragment(frag, &out, true, core.SrcPos{}); reason != "" {
		t.Fatalf("leftover-drop should lower cleanly, got %q", reason)
	}
}

// w8failFrag is a fragment whose lowerFragment refuses: one event leaves a
// value the (mismatching) event out cannot claim.
func w8failFrag() (*EmitFragment, EmitOperand) {
	return &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}}, EventOperand(9, 0)
}

// --- lowerContinue: outside a compiled loop -------------------------------

func TestW8LowerContinueOutsideLoop(t *testing.T) {
	lw := w8lw() // no loop context, not a fn unit
	if reason := lw.lowerContinue(&EmitEvent{kind: evContinue}); !strings.Contains(reason, "continue outside a compiled loop") {
		t.Fatalf("continue-outside-loop reason = %q", reason)
	}
}

// --- lowerUserCall / lowerUserPolyCall ------------------------------------

func TestW8LowerUserCallLayoutRefusal(t *testing.T) {
	lw := w8lw()
	lw.es.fnRecs = []*fnUnitRec{{name: "f"}} // referenced by the refusal message
	ev := &EmitEvent{seq: 1, kind: evCallUser, uc: emitUserCall{unit: 0, ops: []EmitOperand{EventOperand(1, 0)}}}
	if reason := lw.lowerUserCall(ev); !strings.Contains(reason, "result is not on top") {
		t.Fatalf("user-call layout reason = %q", reason)
	}
}

func TestW8LowerUserPolyCallLayoutRefusal(t *testing.T) {
	lw := w8lw()
	ev := &EmitEvent{seq: 1, kind: evCallUser, uc: emitUserCall{
		unit: -1, ops: []EmitOperand{EventOperand(1, 0)}, poly: &emitUserPolySpec{word: "p"},
	}}
	if reason := lw.lowerUserPolyCall(ev); !strings.Contains(reason, "result is not on top") {
		t.Fatalf("user-poly layout reason = %q", reason)
	}
}

func TestW8LowerUserPolyCallPromotedAndDead(t *testing.T) {
	// Promoted: the poly result is stored to a frame slot.
	lw := w8lw()
	lw.promoted[7] = 0
	ev := &EmitEvent{seq: 7, kind: evCallUser, uc: emitUserCall{unit: -1, poly: &emitUserPolySpec{word: "p"}}}
	if reason := lw.lowerUserPolyCall(ev); reason != "" {
		t.Fatalf("promoted poly call should lower cleanly, got %q", reason)
	}
	// Dead: the poly result is dropped.
	lw2 := w8lw()
	lw2.dead[7] = true
	ev2 := &EmitEvent{seq: 7, kind: evCallUser, uc: emitUserCall{unit: -1, poly: &emitUserPolySpec{word: "p"}}}
	if reason := lw2.lowerUserPolyCall(ev2); reason != "" {
		t.Fatalf("dead poly call should lower cleanly, got %q", reason)
	}
}

// --- lowerFallback: threaded-input refusals -------------------------------

func TestW8LowerFallbackArms(t *testing.T) {
	// Variadic threaded input.
	lw := w8lw()
	lw.variadic = map[int]bool{5: true}
	ev := &EmitEvent{seq: 1, kind: evFallback, fb: emitFallback{ins: []EmitOperand{EventOperand(5, 0)}}}
	if reason := lw.lowerFallback(ev); !strings.Contains(reason, "threads a loop result") {
		t.Fatalf("fallback variadic input reason = %q", reason)
	}
	// Event input not on top.
	lw2 := w8lw()
	ev2 := &EmitEvent{seq: 1, kind: evFallback, fb: emitFallback{ins: []EmitOperand{EventOperand(5, 0)}}}
	if reason := lw2.lowerFallback(ev2); !strings.Contains(reason, "fallback input is not on top") {
		t.Fatalf("fallback not-on-top reason = %q", reason)
	}
	// Multiple threaded inputs.
	lw3 := w8lw()
	ev3 := &EmitEvent{seq: 1, kind: evFallback, fb: emitFallback{ins: []EmitOperand{ConstOperand(0), ConstOperand(1)}}}
	if reason := lw3.lowerFallback(ev3); !strings.Contains(reason, "multiple threaded inputs") {
		t.Fatalf("fallback multi-input reason = %q", reason)
	}
}

// --- tailCompatibleReturns / markTailCalls --------------------------------

func TestW8TailCompatibleReturns(t *testing.T) {
	es := NewEmitState()
	// callee unit out of range.
	if es.tailCompatibleReturns(-1, []*core.Type{core.TInteger}) {
		t.Fatal("out-of-range callee unit should be incompatible")
	}
	// callee return does not conform to the caller's declared return.
	es.fnRecs = []*fnUnitRec{{returns: []*core.Type{core.TString}}}
	if es.tailCompatibleReturns(0, []*core.Type{core.TInteger}) {
		t.Fatal("String callee return should not satisfy an Integer caller return")
	}
}

func TestW8MarkTailCallsBranchSeqMismatch(t *testing.T) {
	es := NewEmitState()
	out := EventOperand(1, 0)
	// A branch as the last event whose seq is NOT the fragment's out: not tail.
	frag := &EmitFragment{events: []EmitEvent{{seq: 2, kind: evBranch, br: &emitBranch{}}}}
	if got := es.markTailCalls(frag, &out, true, nil); got != true {
		t.Fatalf("seq-mismatch branch should leave hasOut unchanged, got %v", got)
	}
}

// --- lowerBranch: condition / computed-arm refusals -----------------------

func w8condEventBranch(cond EmitOperand) *EmitEvent {
	return &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{cond: cond}}
}

func TestW8LowerBranchCondVariadic(t *testing.T) {
	lw := w8lw()
	lw.variadic = map[int]bool{5: true}
	if reason := lw.lowerBranch(w8condEventBranch(EventOperand(5, 0))); !strings.Contains(reason, "loop results as a condition") {
		t.Fatalf("variadic cond reason = %q", reason)
	}
}

func TestW8LowerBranchCondNotOnTop(t *testing.T) {
	lw := w8lw() // empty sim: the cond event is not on top
	if reason := lw.lowerBranch(w8condEventBranch(EventOperand(5, 0))); !strings.Contains(reason, "condition is not on top") {
		t.Fatalf("cond-not-on-top reason = %q", reason)
	}
}

func TestW8LowerBranchCondFragRefusal(t *testing.T) {
	lw := w8lw()
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{condFrag: frag, condOut: badOut}}
	if reason := lw.lowerBranch(ev); reason == "" {
		t.Fatal("failing cond-frag branch should refuse")
	}
}

func TestW8LowerBranchVariadicElseLayout(t *testing.T) {
	lw := w8lw()
	lw.variadicElse = map[int]bool{1: true}
	// A non-event cond in the variadic-else claim trips the stack-layout refusal.
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{cond: ConstOperand(0)}}
	if reason := lw.lowerBranch(ev); !strings.Contains(reason, "variadic-else claim stack layout") {
		t.Fatalf("variadic-else layout reason = %q", reason)
	}
}

func TestW8LowerBranchComputedNonEagerNetsNoValue(t *testing.T) {
	lw := w8lw()
	// A computed-else arm whose non-eager (then) arm produces no value AND
	// does not diverge: it reaches the merge having pushed nothing, so the
	// single merge slot would be a lie. (A DIVERGING arm never reaches the
	// merge and is admitted — the fifty-first increment; its own coverage is
	// the whole-program parity in lang/go/diverging_arm_lowering_test.go.)
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		elsComputed: true, elsVal: EventOperand(5, 0), hasThenOut: false,
	}}
	if reason := lw.lowerBranch(ev); !strings.Contains(reason, "non-eager arm nets no value") {
		t.Fatalf("computed non-eager reason = %q", reason)
	}
}

func TestW8LowerBranchComputedEagerNotOnTop(t *testing.T) {
	lw := w8lw() // empty sim: the eager value is not on top
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		elsComputed: true, elsVal: EventOperand(5, 0), hasThenOut: true,
	}}
	if reason := lw.lowerBranch(ev); !strings.Contains(reason, "eager value not on top") {
		t.Fatalf("computed eager-not-on-top reason = %q", reason)
	}
}

// --- lowerBothComputed ----------------------------------------------------

func TestW8LowerBothComputedLayout(t *testing.T) {
	lw := w8lw() // empty sim can't hold the [cond, then, else] triple
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		cond: EventOperand(3, 0), thenVal: EventOperand(2, 0), elsVal: EventOperand(1, 0),
	}}
	if reason := lw.lowerBothComputed(ev); !strings.Contains(reason, "both-computed stack layout") {
		t.Fatalf("both-computed layout reason = %q", reason)
	}
}

func TestW8LowerBothComputedVariadicArm(t *testing.T) {
	lw := w8lw()
	// Sim holds [cond, then, else] (else on top) matching the operands, but a
	// then arm is a variadic loop value.
	lw.vm = []vmSlot{{seq: 3, idx: 0}, {seq: 2, idx: 0}, {seq: 1, idx: 0}}
	lw.variadic = map[int]bool{2: true}
	ev := &EmitEvent{seq: 9, kind: evBranch, br: &emitBranch{
		cond: EventOperand(3, 0), thenVal: EventOperand(2, 0), elsVal: EventOperand(1, 0),
	}}
	if reason := lw.lowerBothComputed(ev); !strings.Contains(reason, "both-computed arm is a variadic") {
		t.Fatalf("both-computed variadic reason = %q", reason)
	}
}

// --- lowerComputedCond ----------------------------------------------------

func TestW8LowerComputedCondNotBelowEager(t *testing.T) {
	lw := w8lw() // empty sim: no cond below the eager value
	_, reason := lw.lowerComputedCond(&emitBranch{cond: EventOperand(5, 0)}, false)
	if !strings.Contains(reason, "condition not below the eager value") {
		t.Fatalf("computed-cond not-below reason = %q", reason)
	}
}

func TestW8LowerComputedCondFragRefusal(t *testing.T) {
	lw := w8lw()
	frag, badOut := w8failFrag()
	_, reason := lw.lowerComputedCond(&emitBranch{condFrag: frag, condOut: badOut}, false)
	if reason == "" {
		t.Fatal("failing computed cond-frag should refuse")
	}
}

// --- lowerCall: dynApply through the apply word --------------------------

func TestW8LowerCallDynApplyUnquote(t *testing.T) {
	lw := w8lw()
	// A trailing fn-value apply that came through the `apply` word lowers to
	// OpCallDynApplyTop (the unquote-then-apply opcode).
	ev := &EmitEvent{seq: 1, kind: evCall, call: emitCall{dynApply: 1, dynApplyUnquote: true, nout: 1}}
	if reason := lw.lowerCall(ev); reason != "" {
		t.Fatalf("dynApply-unquote should lower cleanly, got %q", reason)
	}
}

// --- RewritePromotedRefs: the evFallback operand walk --------------------

func TestW8RewritePromotedRefsFallback(t *testing.T) {
	// A fallback event with a threaded input exercises the evFallback operand
	// walk (no promoted entries → the operands are visited but unchanged).
	ev := &EmitEvent{kind: evFallback, fb: emitFallback{ins: []EmitOperand{EventOperand(1, 0)}}}
	RewritePromotedRefs(ev, map[int]int{})
	if ev.fb.ins[0].kind != opEvent {
		t.Fatalf("unpromoted fallback input should be unchanged, got %v", ev.fb.ins[0].kind)
	}
}

// --- lowerBranch const-cond / variadic-else arm failures -----------------

func TestW8LowerBranchConstCondArmRefusal(t *testing.T) {
	lw := w8lw()
	tru := true
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		constCond: &tru, then: frag, thenOut: badOut, hasThenOut: true,
	}}
	if reason := lw.lowerBranch(ev); reason == "" {
		t.Fatal("const-cond branch with a failing taken arm should refuse")
	}
}

func TestW8LowerBranchConstCondVariadicMerge(t *testing.T) {
	lw := w8lw()
	tru := true
	// A const-cond taken arm that lowers cleanly but whose result event is a
	// variadic loop value marks the merge variadic (emit.go:1920).
	lw.variadic = map[int]bool{5: true}
	ev := &EmitEvent{seq: 9, kind: evBranch, br: &emitBranch{
		constCond: &tru, then: &EmitFragment{events: []EmitEvent{w8pushEvent(5, 1)}},
		thenOut: EventOperand(5, 0), hasThenOut: true,
	}}
	if reason := lw.lowerBranch(ev); reason != "" {
		t.Fatalf("const-cond variadic-merge should lower cleanly, got %q", reason)
	}
	if !lw.variadic[9] {
		t.Fatal("the merge should have been marked variadic")
	}
}

func TestW8LowerBranchVariadicElseArmRefusal(t *testing.T) {
	lw := w8lw()
	lw.variadicElse = map[int]bool{1: true}
	lw.vm = []vmSlot{{seq: 2, idx: 0}, {seq: 3, idx: 0}} // [elsVal, cond]
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		cond: EventOperand(3, 0), elsVal: EventOperand(2, 0),
		then: frag, thenOut: badOut, hasThenOut: true,
	}}
	if reason := lw.lowerBranch(ev); reason == "" {
		t.Fatal("variadic-else branch with a failing then arm should refuse")
	}
}

func TestW8LowerBranchComputedCondRefusal(t *testing.T) {
	lw := w8lw()
	lw.vm = []vmSlot{{seq: 5, idx: 0}} // eager value on top
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		elsComputed: true, elsVal: EventOperand(5, 0), hasThenOut: true,
		condFrag: frag, condOut: badOut,
	}}
	if reason := lw.lowerBranch(ev); reason == "" {
		t.Fatal("computed branch with a failing condition should refuse")
	}
}

// --- lowerComputedBranch arm failures -------------------------------------

func TestW8LowerComputedBranchThenComputedElseRefusal(t *testing.T) {
	lw := w8lw()
	lw.emit(OpJmpIfFalse, 0, core.SrcPos{}) // pre-emit so jf=0 is a valid patch target
	lw.vm = []vmSlot{nonEventSlot}          // the eager then value on top
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		thenComputed: true, hasElse: true,
		els: frag, elsOut: badOut, hasElsOut: true,
	}}
	if reason := lw.lowerComputedBranch(ev, 0); reason == "" {
		t.Fatal("computed-then branch with a failing else arm should refuse")
	}
}

func TestW8LowerComputedBranchElseThenRefusal(t *testing.T) {
	lw := w8lw()
	lw.emit(OpJmpIfFalse, 0, core.SrcPos{})
	lw.vm = []vmSlot{nonEventSlot} // the eager else value on top
	frag, badOut := w8failFrag()
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		thenComputed: false,
		then:         frag, thenOut: badOut, hasThenOut: true,
	}}
	if reason := lw.lowerComputedBranch(ev, 0); reason == "" {
		t.Fatal("computed-else branch with a failing then arm should refuse")
	}
}

// --- markTailCalls: both branch arms tail-diverge -------------------------

func TestW8MarkTailCallsBothArmsTail(t *testing.T) {
	es := NewEmitState()
	armEvent := func(seq int) EmitEvent {
		return EmitEvent{seq: seq, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}}
	}
	outer := &EmitFragment{events: []EmitEvent{{
		seq: 1, kind: evBranch, br: &emitBranch{
			hasElse: true,
			then:    &EmitFragment{events: []EmitEvent{armEvent(10)}}, thenOut: EventOperand(10, 0), hasThenOut: true,
			els: &EmitFragment{events: []EmitEvent{armEvent(11)}}, elsOut: EventOperand(11, 0), hasElsOut: true,
		},
	}}}
	out := EventOperand(1, 0)
	// Both arms tail-call (nil caller returns → tail-compatible), so the whole
	// branch diverges and markTailCalls reports hasOut=false.
	if es.markTailCalls(outer, &out, true, nil) {
		t.Fatal("a branch whose arms both tail-call should report no residual out")
	}
}

// --- lowerBothComputedMatCond (S9.4 both-computed non-event cond) ----------

func TestW8LowerBothComputedMatCondLayout(t *testing.T) {
	lw := w8lw() // empty sim can't hold the [then, else] pair
	ev := &EmitEvent{seq: 1, kind: evBranch, br: &emitBranch{
		cond: localOperand(0), thenVal: EventOperand(2, 0), elsVal: EventOperand(1, 0),
	}}
	if reason := lw.lowerBothComputedMatCond(ev); !strings.Contains(reason, "both-computed stack layout") {
		t.Fatalf("mat-cond layout reason = %q", reason)
	}
}

func TestW8LowerBothComputedMatCondVariadicArm(t *testing.T) {
	lw := w8lw()
	// Sim holds [then, else] (else on top); a then arm is a variadic loop value.
	lw.vm = []vmSlot{{seq: 2, idx: 0}, {seq: 1, idx: 0}}
	lw.variadic = map[int]bool{2: true}
	ev := &EmitEvent{seq: 9, kind: evBranch, br: &emitBranch{
		cond: localOperand(0), thenVal: EventOperand(2, 0), elsVal: EventOperand(1, 0),
	}}
	if reason := lw.lowerBothComputedMatCond(ev); !strings.Contains(reason, "both-computed arm is a variadic") {
		t.Fatalf("mat-cond variadic reason = %q", reason)
	}
}
