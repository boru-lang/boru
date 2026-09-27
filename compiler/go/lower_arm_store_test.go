package compiler

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// lower_arm_store_test.go drives the lowering arms around the branch-carried
// store and its neighbours directly (w8lw's bare lowerer): the store's
// layouts and declines (storeArmBind, through lowerDynBind), the split-bound
// name's registry install beside the store, and the declines a planner that
// could not promote a seed or a range operand leaves to the lowering.

func lasOps(code []Instr) string { return fmt.Sprint(lfsOps(code)) }

func lasWant(ops ...Opcode) string { return fmt.Sprint(ops) }

// TestStoreArmBindArms pins every layout a branch-carried def's store lowers
// from, and the three it declines.
func TestStoreArmBindArms(t *testing.T) {
	arm := func(d emitDynBind) *emitDynBind {
		d.name, d.armCarried, d.armSlot, d.residentTwin = "x", true, 1, -1
		return &d
	}

	// A promoted source re-pushes from its local.
	lw := w8lw()
	lw.promoted[5] = 3
	if reason := lw.storeArmBind(arm(emitDynBind{srcSeq: 5})); reason != "" {
		t.Fatalf("promoted source: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpPushLocal, OpStoreLocal) || (*lw.code)[0].Arg != 3 || (*lw.code)[1].Arg != 1 || len(lw.vm) != 0 {
		t.Fatalf("promoted source: code %s %v, vm %v", got, *lw.code, lw.vm)
	}

	// A LIVE source on the sim top is stored and pushed back for its
	// other consumers.
	lw = w8lw()
	lw.vm = []vmSlot{{seq: 5}}
	if reason := lw.storeArmBind(arm(emitDynBind{srcSeq: 5})); reason != "" {
		t.Fatalf("live sim-top source: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpStoreLocal, OpPushLocal) || len(lw.vm) != 1 {
		t.Fatalf("live sim-top source must be copied: code %s, vm %v", got, lw.vm)
	}

	// A DEAD source kept only for this bind (bindConsumes) is consumed.
	lw = w8lw()
	lw.vm = []vmSlot{{seq: 5}}
	lw.dead[5], lw.bindConsumes = true, map[int]bool{5: true}
	if reason := lw.storeArmBind(arm(emitDynBind{srcSeq: 5})); reason != "" {
		t.Fatalf("dead sim-top source: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpStoreLocal) || len(lw.vm) != 0 {
		t.Fatalf("dead sim-top source must be consumed: code %s, vm %v", got, lw.vm)
	}

	// An inert literal bakes; a frame local re-pushes.
	lw = w8lw()
	if reason := lw.storeArmBind(arm(emitDynBind{srcSeq: -1, val: core.NewInteger(9)})); reason != "" {
		t.Fatalf("inert literal: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpPushConst, OpStoreLocal) {
		t.Fatalf("inert literal: code %s", got)
	}
	lw = w8lw()
	if reason := lw.storeArmBind(arm(emitDynBind{srcSeq: -1, src: localOperand(4)})); reason != "" {
		t.Fatalf("frame local: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpPushLocal, OpStoreLocal) || (*lw.code)[0].Arg != 4 {
		t.Fatalf("frame local: code %s %v", got, *lw.code)
	}

	// The declines: each names its own reason and emits nothing.
	for _, c := range []struct {
		what, reason string
		setup        func(*lowerer)
		d            emitDynBind
	}{
		{"a variadic source (a loop's run)", "binds loop results (Stage 2)",
			func(lw *lowerer) { lw.variadic[5] = true }, emitDynBind{srcSeq: 5}},
		{"a computed source off the sim top", "source is not on top of the stack",
			func(lw *lowerer) { lw.vm = []vmSlot{{seq: 6}} }, emitDynBind{srcSeq: 5}},
		{"a computed source with no sim entry", "source is not on top of the stack",
			func(*lowerer) {}, emitDynBind{srcSeq: 5}},
		{"the source's second result", "source is not on top of the stack",
			func(lw *lowerer) { lw.vm = []vmSlot{{seq: 5, idx: 1}} }, emitDynBind{srcSeq: 5}},
		{"a literal with no inert value", "of unknown provenance",
			func(*lowerer) {}, emitDynBind{srcSeq: -1, val: core.NewCarrier(core.TInteger)}},
	} {
		lw := w8lw()
		c.setup(lw)
		reason := lw.storeArmBind(arm(c.d))
		if !strings.Contains(reason, c.reason) || !strings.Contains(reason, "branch-carried def `x`") || len(*lw.code) != 0 {
			t.Errorf("%s: reason %q, code %v — want a decline naming %q and nothing emitted", c.what, reason, *lw.code, c.reason)
		}
	}
}

// TestLowerDynBindArmStore: lowerDynBind lowers a branch-carried def's store
// first, propagates its decline unchanged, and adds the registry install for
// a root arm's store of a top-level split-bound name.
func TestLowerDynBindArmStore(t *testing.T) {
	store := func(d *emitDynBind) *EmitEvent { return &EmitEvent{kind: evDynBind, dyn: d} }

	// The store's decline is the def's decline: nothing after it lowers.
	lw := w8lw()
	reason := lw.lowerDynBind(store(&emitDynBind{name: "x", srcSeq: 5, armCarried: true, armSlot: 1, residentTwin: -1}))
	if !strings.Contains(reason, "branch-carried def `x` source is not on top of the stack") || len(*lw.code) != 0 {
		t.Fatalf("the arm store's decline must propagate: reason %q, code %v", reason, *lw.code)
	}

	// A plain root arm store: the slot store alone.
	lw = w8lw()
	if reason := lw.lowerDynBind(store(&emitDynBind{name: "x", srcSeq: -1, val: core.NewInteger(9), root: true,
		armCarried: true, armSlot: 1, residentTwin: -1})); reason != "" {
		t.Fatalf("root arm store: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpPushConst, OpStoreLocal) {
		t.Fatalf("a root arm store of an ordinary name is the slot store alone, got %s", got)
	}

	// The same store of a split-bound name also installs the binding.
	lw = w8lw()
	lw.es.loopSplitBinds = map[string]bool{"x": true}
	if reason := lw.lowerDynBind(store(&emitDynBind{name: "x", srcSeq: -1, val: core.NewInteger(9), root: true,
		armCarried: true, armSlot: 1, residentTwin: -1})); reason != "" {
		t.Fatalf("split-bound arm store: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpPushConst, OpStoreLocal, OpPushConst, OpBindDynScope) || len(lw.vm) != 0 {
		t.Fatalf("a root arm store of a split-bound name must also install it, got %s (vm %v)", got, lw.vm)
	}
	if name, err := core.AsString(lw.es.consts[(*lw.code)[3].Arg]); err != nil || name != "x" {
		t.Fatalf("the install must bind `x`, got %v (%v)", lw.es.consts[(*lw.code)[3].Arg], err)
	}
}

// TestForEachOperandArmSourceOuter: a branch-carried store whose computed
// source was produced OUTSIDE its arm is a cross-floor reference, counted so
// the planner promotes the source to a frame local; a source produced inside
// the arm is not (its dead-ness stays the ordinary refcount's).
func TestForEachOperandArmSourceOuter(t *testing.T) {
	visited := func(d *emitDynBind) []EmitOperand {
		var ops []EmitOperand
		forEachOperand(&EmitEvent{kind: evDynBind, dyn: d}, func(op EmitOperand) { ops = append(ops, op) })
		return ops
	}
	outer := visited(&emitDynBind{name: "x", srcSeq: 5, armCarried: true, armSrcOuter: true})
	if len(outer) != 2 || outer[1].kind != opEvent || outer[1].idx != 5 || outer[1].resIdx != 0 {
		t.Fatalf("an outer source must be visited as its event's result, got %v", outer)
	}
	if inner := visited(&emitDynBind{name: "x", srcSeq: 5, armCarried: true}); len(inner) != 1 || inner[0].kind != opNone {
		t.Fatalf("an arm-internal source is not an operand, got %v", inner)
	}
}

// TestLowerLoopEventRangeDeclines: a counted loop's range start or step that
// is still an event at the lowering — one the planner could not promote to a
// frame local (collectLoopRangeSources) — is not re-pushable under the end,
// and declines before FOR_SETUP.
func TestLowerLoopEventRangeDeclines(t *testing.T) {
	for what, lp := range map[string]*emitLoop{
		"an event start": {start: EventOperand(3, 0), end: ConstOperand(1), step: ConstOperand(0)},
		"an event step":  {start: ConstOperand(0), end: ConstOperand(1), step: EventOperand(3, 0)},
	} {
		lw := w8lw()
		reason := lw.lowerLoop(&EmitEvent{kind: evLoop, seq: 9, loop: lp})
		if reason != "for: computed range start/step (Stage 2 follow-on)" {
			t.Errorf("%s: reason %q", what, reason)
		}
		for _, in := range *lw.code {
			if in.Op == OpForSetup {
				t.Errorf("%s: FOR_SETUP emitted past the decline", what)
			}
		}
	}
}

// TestLowerBranchEventSeedDeclines: a branch-carried seed that is still an
// event at the lowering (a producer the planner could not promote — its
// collectBranchCarriedSources request unmet) declines before any seed is
// stored.
func TestLowerBranchEventSeedDeclines(t *testing.T) {
	lw := w8lw()
	ev := &EmitEvent{kind: evBranch, seq: 9, br: &emitBranch{hasElse: true,
		carried: []carriedInit{{slot: 0, init: ConstOperand(0)}, {slot: 1, init: EventOperand(4, 0)}}}}
	if reason := lw.lowerBranch(ev); reason != "if: carried def seed is not a re-pushable value (Stage 2)" {
		t.Fatalf("an event seed must decline, got %q", reason)
	}
	if len(*lw.code) != 0 {
		t.Fatalf("no seed may be stored before the decline, got %v", *lw.code)
	}
}

// TestMergeBindConsumes pins the nil-safe union of the root and arm
// bind-consumes sets.
func TestMergeBindConsumes(t *testing.T) {
	if got := mergeBindConsumes(nil, map[int]bool{3: true}); len(got) != 1 || !got[3] {
		t.Errorf("merging into nil: got %v, want {3}", got)
	}
	a := map[int]bool{1: true}
	if got := mergeBindConsumes(a, nil); len(got) != 1 || !got[1] {
		t.Errorf("merging nothing: got %v, want {1}", got)
	}
	if got := mergeBindConsumes(a, map[int]bool{2: true}); len(got) != 2 || !got[1] || !got[2] {
		t.Errorf("merging: got %v, want {1 2}", got)
	}
}

// TestLowerUserPolyCallResultDispositions pins what happens to a
// runtime-dispatched user call's result: a DEAD result a root write-back
// consumes stays on the stack for the bind to pop (no DROP), a dead result
// nothing consumes is dropped, and a live one takes one sim slot per result.
func TestLowerUserPolyCallResultDispositions(t *testing.T) {
	call := func() *EmitEvent {
		return &EmitEvent{kind: evCallUser, seq: 6, uc: emitUserCall{unit: -1, nout: 1,
			poly: &emitUserPolySpec{word: "pf", units: []int{0, 1}}}}
	}
	lw := w8lw()
	lw.dead[6], lw.bindConsumes = true, map[int]bool{6: true}
	if reason := lw.lowerUserPolyCall(call()); reason != "" {
		t.Fatalf("consumed dead result: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpCallUserPoly) || len(lw.vm) != 1 || lw.vm[0] != (vmSlot{seq: 6}) {
		t.Fatalf("a dead result the write-back consumes must stay for the bind: code %s, vm %v", got, lw.vm)
	}
	if len(lw.p.UserPolys) != 1 || lw.p.UserPolys[0].Word != "pf" {
		t.Fatalf("the call's dispatch table: %+v", lw.p.UserPolys)
	}

	lw = w8lw()
	lw.dead[6] = true
	if reason := lw.lowerUserPolyCall(call()); reason != "" {
		t.Fatalf("dead result: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpCallUserPoly, OpDrop) || len(lw.vm) != 0 {
		t.Fatalf("an unconsumed dead result is dropped: code %s, vm %v", got, lw.vm)
	}

	lw = w8lw()
	if reason := lw.lowerUserPolyCall(call()); reason != "" {
		t.Fatalf("live result: %s", reason)
	}
	if got := lasOps(*lw.code); got != lasWant(OpCallUserPoly) || len(lw.vm) != 1 {
		t.Fatalf("a live result takes its sim slot: code %s, vm %v", got, lw.vm)
	}
}
