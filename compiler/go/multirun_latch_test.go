package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// MultiRunBodyGuard's latch contract (the arm-residency bridge's seat):
// the guard suspends like BodyAnalysisGuard — the keep-bracket taint
// included, so the #421 fences hold unchanged — and on close publishes
// {bodyID, the twin range this run noted, the noting registry}, with
// overwrite semantics. A nil receiver is a no-op like every EmitState
// method.
func TestMultiRunBodyGuardLatch(t *testing.T) {
	var nilES *EmitState
	nilES.MultiRunBodyGuard(nil, "b")()

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	note := func(es *EmitState, name string) {
		es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: name, Depth: 1,
			Pos: core.SrcPos{Row: 1, Col: 7}}, core.DefEntry{Body: core.NewInteger(1)})
	}

	// The latch captures exactly the run's noted range, suspended.
	es := NewEmitState()
	end := es.MultiRunBodyGuard(r, "body-1")
	if es.Active() {
		t.Fatal("the guard must suspend recording for the body run")
	}
	note(es, "x")
	note(es, "y")
	end()
	if l := es.lastMultiRun; l.bodyID != "body-1" || l.from != 0 || l.to != 2 || l.reg != r {
		t.Fatalf("latch = %+v, want {body-1 0 2 r}", l)
	}

	// Overwrite semantics: a later run replaces the latch — the bridge's
	// bodyID comparison is what detects a nested body's overwrite.
	end = es.MultiRunBodyGuard(r, "body-2")
	note(es, "z")
	end()
	if l := es.lastMultiRun; l.bodyID != "body-2" || l.from != 2 || l.to != 3 {
		t.Fatalf("latch after second run = %+v, want {body-2 2 3}", l)
	}

	// Inside a keep bracket the delegated taint still records — the #421
	// do-adoption fence must not weaken when the guard is the multi-run
	// flavor (`do [[1 2] each [def x 5]]`).
	es = NewEmitState()
	keepEnd := es.KeepDefsBodyGuard(r, "")
	end = es.MultiRunBodyGuard(r, "body-3")
	note(es, "leak")
	end()
	keepEnd()
	if len(es.lastKeepTaints) != 1 || es.lastKeepTaints[0] != [2]int{0, 1} {
		t.Fatalf("keep taints = %v, want the multi-run sub-range [0,1)", es.lastKeepTaints)
	}
}

// The superseding rule's IDENTITY, in both directions. A fixed point
// (fold's accumulator widening) re-runs one body back-to-back with
// recording suspended, so no event lands between the rounds and the
// earlier round's twins are artifacts the placement gate must exempt.
// Two separate DISPATCHES can share a body value too — a memoized unit,
// a body read from a binding — and there both runs install at runtime,
// so superseding the first would silently drop its installs. An unmoved
// event counter is what tells the two apart; body-ID adjacency alone
// cannot, which is the hole this pins shut.
func TestMultiRunSupersedeNeedsOneFixedPoint(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	note := func(es *EmitState, name string) {
		es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: name, Depth: 1},
			core.DefEntry{Body: core.NewInteger(1)})
	}

	// Consecutive rounds of ONE fixed point: nothing recorded in between,
	// so the first round's twins are superseded.
	es := NewEmitState()
	end := es.MultiRunBodyGuard(r, "body")
	note(es, "x")
	end()
	end = es.MultiRunBodyGuard(r, "body")
	note(es, "x")
	end()
	if !es.supersededTwins[0] {
		t.Fatal("a re-run of the same body with no event in between is a fixed-point round — twin 0 must be exempt")
	}
	if es.supersededTwins[1] {
		t.Fatal("the SURVIVING round must never be exempt — its op is the one that installs")
	}

	// Two DISPATCHES sharing a body value: the second dispatch's own call
	// event lands between the runs, so neither round may be superseded.
	es = NewEmitState()
	end = es.MultiRunBodyGuard(r, "body")
	note(es, "x")
	end()
	es.appendEvent(EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "sep", srcSeq: -1, residentTwin: -1}})
	end = es.MultiRunBodyGuard(r, "body")
	note(es, "x")
	end()
	if len(es.supersededTwins) != 0 {
		t.Fatalf("a second DISPATCH of the same body installs at runtime too — nothing may be exempt, got %v",
			es.supersededTwins)
	}
}
