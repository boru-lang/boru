package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// captureArmResidual (the branch-arm twin of RecordLoop's all-inert capture):
// a multi-value arm residual is captured whole for the lowering's re-push —
// EVENT-produced entries included, since planValueDefLocals force-promotes
// each to a frame slot before the re-push reads it. A parked Function, a value
// with no resolvable provenance, and a single-value arm all decline
// (residualOps stays nil — the arm keeps its refusal or its single-out path).
func TestCaptureArmResidualArms(t *testing.T) {
	r := seam7Reg(t)
	es := NewEmitState()
	es.BindRegistry(r)

	frag := &EmitFragment{}
	es.captureArmResidual(frag, []core.Value{core.NewInteger(1), core.NewInteger(2)})
	if len(frag.residualOps) != 2 {
		t.Fatalf("all-inert capture = %d ops, want 2", len(frag.residualOps))
	}

	es.captureArmResidual(nil, []core.Value{core.NewInteger(1), core.NewInteger(2)}) // nil frag: no panic

	single := &EmitFragment{}
	es.captureArmResidual(single, []core.Value{core.NewInteger(1)})
	if single.residualOps != nil {
		t.Error("a single-value arm must not capture")
	}

	fnArm := &EmitFragment{}
	es.captureArmResidual(fnArm, []core.Value{core.NewInteger(1), core.NewCarrier(core.TFunction)})
	if fnArm.residualOps != nil {
		t.Error("a parked-fn arm must not capture (the auto-apply hazard)")
	}

	// An entry with no PROVENANCE — no producing event, no frame local, and
	// nothing the const gate will bake — declines the WHOLE capture. A
	// partial list is the dangerous outcome, not a harmless one: drop the
	// decline and the loop appends a zero EmitOperand (const slot 0) in the
	// unresolvable entry's place, and the list is then full-length, so
	// armRepushableResidual accepts it and the re-push seats const 0 where
	// the value belonged.
	//
	// The NON-FINAL position is the one that needs a test: RecordBranch's
	// resolveArm resolves only the arm's LAST value, so every entry beneath
	// it is first looked at here. The program shape is an unresolvable value
	// under a resolvable one — e.g. `import "boru:type-util"
	// if true [tpartial 7] [3 4]`, whose undefined word leaves a carrier with
	// no producer under the trailing 7.
	noProv := core.NewCarrier(core.TInteger)
	for _, c := range []struct {
		name string
		stk  []core.Value
	}{
		{"unresolvable BELOW the arm result (resolveArm never looks here)",
			[]core.Value{noProv, core.NewInteger(7)}},
		{"unresolvable AS the arm result",
			[]core.Value{core.NewInteger(7), noProv}},
	} {
		arm := &EmitFragment{}
		es.captureArmResidual(arm, c.stk)
		if arm.residualOps != nil {
			t.Errorf("%s: captured %v, want the decline (residualOps stays nil)", c.name, arm.residualOps)
		}
	}

	// A DYNAMIC Any entry declines too — but at the parked-fn screen ABOVE,
	// not for want of provenance: Any conforms to Function, so a gradual
	// value is treated as possibly-callable and carries the auto-apply hazard
	// on its own. Pinned as its own case, with the screen it takes named, so
	// the two declines are never read as one.
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	if !core.SigTypeMatches(dyn, core.TFunction) {
		t.Error("a gradual Any entry must read as possibly-callable — the parked-fn screen is what refuses it")
	}
	dynArm := &EmitFragment{}
	es.captureArmResidual(dynArm, []core.Value{core.NewInteger(1), dyn})
	if dynArm.residualOps != nil {
		t.Error("a dynamic Any entry must not capture")
	}

	// An EVENT-produced entry is captured (the loop side's restriction lifted
	// for arms): the operand names its producing seq, which RewritePromotedRefs
	// rewrites to the frame slot planValueDefLocals allocated.
	produced := core.NewCarrier(core.TInteger)
	es.setProduced(produced, 4)
	evArm := &EmitFragment{}
	es.captureArmResidual(evArm, []core.Value{core.NewInteger(1), produced})
	if len(evArm.residualOps) != 2 || evArm.residualOps[1].kind != opEvent || evArm.residualOps[1].idx != 4 {
		t.Fatalf("event-bearing capture = %+v, want the producing seq as entry 1", evArm.residualOps)
	}
}

// The variadic-region rematch seat: a trap whose leading operand is a
// variadic branch merge requires that merge slot on the sim TOP (the region
// tops the runtime stack); a mismatched sim declines and the whole-program
// refusal stands.
func TestLowerTrapVariadicRegionNotOnTop(t *testing.T) {
	es := NewEmitState()
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{7: true}, promoted: map[int]int{}}
	ev := EmitEvent{kind: evTrap, trap: EmitTrap{
		rematchWord:       "w",
		rematchOps:        []EmitOperand{EventOperand(7, 0), ConstOperand(0)},
		rematchWrittenOff: 1,
		rematchNWritten:   1,
	}}
	if reason := lw.lowerTrap(&ev); reason == "" ||
		reason != "stack discipline: variadic rematch region is not on top" {
		t.Fatalf("empty-sim variadic rematch reason = %q, want the region-not-on-top decline", reason)
	}
}

// A multi-value arm that is neither all-inert-captured nor event-seated is
// irreconstructible: lowerFragment's default keeps the whole-program refusal
// (the sound fallback) — the arm the each row covered before its all-inert
// capture graduated it.
func TestLowerFragmentIrreconstructibleMultiArm(t *testing.T) {
	es := NewEmitState()
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	frag := &EmitFragment{residualN: 2}
	out := ConstOperand(0)
	reason := lw.lowerFragment(frag, &out, true, core.SrcPos{})
	if !strings.Contains(reason, "branch leaves extra values") {
		t.Fatalf("irreconstructible multi-arm reason = %q, want the branch-residual refusal", reason)
	}
}

// RecordTypedBind's inactive-recorder decline (the guard split when concrete
// PREDICATE operands were admitted — the fn-predicate bind is a runtime
// evaluation, so concreteness no longer short-circuits that kind).
func TestRecordTypedBindInactiveDeclines(t *testing.T) {
	es := NewEmitState()
	resume := es.Suspend()
	defer resume()
	if _, ok := es.RecordTypedBind(core.TypedBindSpec{Kind: core.TypedBindPredicate, Name: "x"},
		core.NewInteger(1), core.NewInteger(1), core.SrcPos{}); ok {
		t.Fatal("a suspended recorder must decline the typed-bind record")
	}
}

// armRepushableResidual admits ONLY a branch ARM (childFragments slot 1/2)
// whose multi-value residual was captured whole; armResidualForceSeqs then
// names that residual's EVENT entries — the seqs planValueDefLocals must move
// to frame slots so lowerFragment's re-push arm finds an empty sim.
func TestArmRepushableResidual(t *testing.T) {
	whole := &EmitFragment{residualN: 2, residualOps: []EmitOperand{ConstOperand(0), EventOperand(3, 0)}}
	br := &EmitEvent{seq: 9, kind: evBranch, br: &emitBranch{condFrag: whole, then: whole, els: nil}}
	cases := []struct {
		name string
		ev   *EmitEvent
		fi   int
		frag *EmitFragment
		want bool
	}{
		{"a whole-captured then arm", br, 1, whole, true},
		{"the CONDITION fragment (slot 0) is not an arm", br, 0, whole, false},
		{"a nil fragment", br, 2, nil, false},
		{"a single-value arm", br, 1, &EmitFragment{residualN: 1}, false},
		{"a partial capture", br, 1, &EmitFragment{residualN: 2}, false},
		{"a LOOP body", &EmitEvent{kind: evLoop, loop: &emitLoop{}}, 0, whole, false},
	}
	for _, c := range cases {
		if got := armRepushableResidual(c.ev, c.fi, c.frag); got != c.want {
			t.Errorf("%s: armRepushableResidual = %v, want %v", c.name, got, c.want)
		}
	}

	force := armResidualForceSeqs([]*EmitEvent{br, {seq: 1, kind: evCall, call: emitCall{}}})
	if len(force) != 1 || !force[3] {
		t.Fatalf("armResidualForceSeqs = %v, want only the event entry seq 3", force)
	}
	// A branch whose arms decline the capture contributes nothing.
	plain := &EmitEvent{seq: 8, kind: evBranch, br: &emitBranch{then: &EmitFragment{residualN: 2}}}
	if got := armResidualForceSeqs([]*EmitEvent{plain}); len(got) != 0 {
		t.Errorf("a partial-capture arm forced %v, want none", got)
	}
}
