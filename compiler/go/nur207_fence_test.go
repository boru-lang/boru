package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur207_fence_test.go pins the fold's escape fence (NUR207, 2026-09-26):
// a container member read tryFoldParkedMemberFn folded to its parking lambda
// may reach a def, a branch arm, `apply`, `typeof` and the residual; every
// other consumer poisons the placement gate (foldedEscape), so a program the
// member's 0-arg landing declined on main still declines. It also pins the
// gradual-claim read accounting (pendingGradualRead / flushGradualRead): a
// def-bound read of a gradual claim the def-read model never saw declines
// (NUR216's `def k j`).

// n207Folded is a fresh recorder over a registry with one folded member.
func n207Folded(t *testing.T) (*EmitState, *core.Registry, core.Value, func()) {
	t.Helper()
	es, reg, done := beginRegionPass(t)
	lam := n207Lambda()
	lam.ID = "folded"
	es.noteFolded(lam)
	return es, reg, lam, done
}

func n207WantEscape(t *testing.T, es *EmitState, name, where string) {
	t.Helper()
	if !strings.Contains(es.armReadCompileFailure, "reaches "+where) || !strings.Contains(es.armReadCompileFailure, "NUR207") {
		t.Errorf("%s: want the fold's escape at %s, got %q", name, where, es.armReadCompileFailure)
	}
}

func TestFoldedEscapeFence(t *testing.T) {
	es := NewEmitState()
	lam := n207Lambda()
	lam.ID = "folded"
	other := n207Lambda()
	if es.foldedInside(core.NewList([]core.Value{lam})) {
		t.Error("nothing folded: nothing inside")
	}
	es.noteFolded(lam)
	// Every copy of the member fn shares its body: a renamed install whose
	// identity differs is still the fold's.
	copied := lam
	copied.ID = "renamed"
	carrier := core.NewDynamicCarrier(core.TAny)
	carrier.ID = lam.ID
	for name, c := range map[string]struct {
		v            core.Value
		folded, carr bool
	}{
		"the folded value":      {lam, true, false},
		"a copy sharing a body": {copied, true, false},
		"a carrier of its id":   {carrier, true, true},
		"another lambda":        {other, false, false},
		"data":                  {core.NewInteger(1), false, false},
		"a bodiless fn":         {core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{}}}), false, false},
	} {
		if got := es.isFolded(c.v); got != c.folded {
			t.Errorf("%s: isFolded = %v, want %v", name, got, c.folded)
		}
		if got := es.anyFoldedCarrier([]core.Value{c.v}); got != c.carr {
			t.Errorf("%s: anyFoldedCarrier = %v, want %v", name, got, c.carr)
		}
	}
	if es.anyFolded([]core.Value{other, core.NewInteger(1)}) || !es.anyFolded([]core.Value{other, lam}) {
		t.Error("anyFolded is any of the values")
	}
	om := core.NewOrderedMap()
	om.Set("k", core.NewInteger(1))
	om.Set("f", lam)
	plain := core.NewOrderedMap()
	plain.Set("k", core.NewInteger(1))
	for name, c := range map[string]struct {
		v    core.Value
		want bool
	}{
		"the value itself is its consumer's operand": {lam, false},
		"a list element":           {core.NewList([]core.Value{core.NewInteger(1), lam}), true},
		"a nested list element":    {core.NewList([]core.Value{core.NewList([]core.Value{lam})}), true},
		"a map member":             {core.NewMap(om), true},
		"a map without it":         {core.NewMap(plain), false},
		"a map with no payload":    {core.NewValueRaw(core.TMap, core.MapPayload{}), false},
		"a list of data":           {core.NewList([]core.Value{core.NewInteger(1)}), false},
		"a list of another lambda": {core.NewList([]core.Value{other}), false},
	} {
		if got := es.foldedInside(c.v); got != c.want {
			t.Errorf("%s: foldedInside = %v, want %v", name, got, c.want)
		}
	}
	es.foldedEscape("somewhere")
	n207WantEscape(t, es, "first escape", "somewhere")
	es.foldedEscape("elsewhere")
	n207WantEscape(t, es, "the first poison stands", "somewhere")
	es.armReadCompileFailure = ""
	es.trapAt = 3
	es.foldedEscape("past a trap")
	if es.armReadCompileFailure != "" {
		t.Error("past a terminal trap the escape is unreachable and poisons nothing")
	}
}

func TestFoldedEscapeSites(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 1}
	for _, c := range []struct {
		name, where string
		run         func(es *EmitState, reg *core.Registry, lam core.Value)
	}{
		{"a native word's operand", "`size`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			recordDispatchOutcome(reg, "size", nil, []core.Value{lam}, nil, pos, nil)
		}},
		{"apply over a folded carrier", "`apply`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			c := core.NewDynamicCarrier(core.TAny)
			c.ID = lam.ID
			recordDispatchOutcome(reg, "apply", nil, []core.Value{c}, nil, pos, nil)
		}},
		{"a list literal", "a list literal", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.RecordMakeList(reg, []core.Value{lam}, core.NewList([]core.Value{lam}), pos)
		}},
		{"a map literal", "a map literal", func(es *EmitState, reg *core.Registry, lam core.Value) {
			om := core.NewOrderedMap()
			om.Set("f", lam)
			es.RecordMakeMap(reg, []string{"f"}, []core.Value{lam}, false, core.NewMap(om), pos)
		}},
		{"a baked container operand", "a container literal", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.resolveOperand(core.NewList([]core.Value{lam}))
		}},
		{"a def of a container over it", "a container literal", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.RecordDynBind("k", core.NewList([]core.Value{lam}), pos)
		}},
		{"a /v read", "a `/v` read of `k`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.NoteValRead(lam.ID, "k")
		}},
		{"an if condition", "an `if` condition", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.carryFoldedTaint(core.BranchRecord{Cond: lam})
		}},
		{"an if condition's stack", "an `if` condition", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.carryFoldedTaint(core.BranchRecord{CondStk: []core.Value{lam}})
		}},
		{"a user fn's argument", "a call of `g`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			unit, _, ok := es.StartFnCompile("g", "g", nil, []core.Value{lam}, []*core.Type{core.TAny}, []string{"h"}, nil, false, core.SrcPos{})
			if !ok {
				t.Fatal("StartFnCompile declined")
			}
			es.RecordUserCall(unit, "g", []core.Value{lam}, nil, pos, pos)
		}},
		{"a user poly call's argument", "a call of `g`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.Compilable = false
			es.RecordUserPolyCall("g", reg, nil, nil, nil, nil, []core.Value{lam}, nil, pos, "g", pos)
		}},
		{"a user poly call of a unit returning it", "a call of `g`", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.Compilable = false
			es.fnRecs = []*fnUnitRec{{}, {returnsFolded: true}}
			es.RecordUserPolyCall("g", reg, nil, []int{-1, 0, 5, 1}, nil, nil, nil, nil, pos, "g", pos)
		}},
		{"a code body's result", "a code body's or a lambda's result", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.fnResidualReplayReason(&emitUnit{localByID: map[string]int{}}, &fnUnitRec{closure: true}, []core.Value{lam}, nil, 0)
		}},
		{"a pending folded fire the model stood aside for", "a def-bound read the model stood aside for", func(es *EmitState, reg *core.Registry, lam core.Value) {
			es.pendingFoldedFire = lam.ID
			es.flushGradualRead()
		}},
	} {
		es, reg, lam, done := n207Folded(t)
		c.run(es, reg, lam)
		n207WantEscape(t, es, c.name, c.where)
		done()
	}
	// The consumers the fold is measured against poison nothing.
	for _, word := range []string{"if", "typeof", "apply"} {
		es, reg, lam, done := n207Folded(t)
		recordDispatchOutcome(reg, word, nil, []core.Value{lam}, nil, pos, nil)
		if strings.Contains(es.armReadCompileFailure, "NUR207") {
			t.Errorf("`%s` of the folded member itself: no escape, got %q", word, es.armReadCompileFailure)
		}
		done()
	}
}

func TestFoldedTaintFollowsBranchAndCall(t *testing.T) {
	es, _, lam, done := n207Folded(t)
	defer done()
	out := core.NewDynamicCarrier(core.TAny)
	two := core.NewInteger(2)
	es.carryFoldedTaint(core.BranchRecord{ThenValue: &lam, ElsValue: &two, Out: out})
	if !es.foldedMembers[out.ID] || es.armReadCompileFailure != "" {
		t.Errorf("an arm value makes the merge folded, poisoning nothing (%q)", es.armReadCompileFailure)
	}
	body := core.NewDynamicCarrier(core.TAny)
	es.carryFoldedTaint(core.BranchRecord{ThenStk: []core.Value{two}, ElsStk: []core.Value{lam}, Out: body})
	if !es.foldedMembers[body.ID] {
		t.Error("an arm body leaving it makes the merge folded")
	}
	plain := core.NewDynamicCarrier(core.TAny)
	es.carryFoldedTaint(core.BranchRecord{ThenValue: &two, Out: plain})
	es.carryFoldedTaint(core.BranchRecord{ThenValue: &lam})
	if es.foldedMembers[plain.ID] || es.foldedMembers[""] {
		t.Error("data arms, or no merge value, taint nothing")
	}
	// RecordBranch flags the arm and carries the taint to its merge.
	rb, _, rbDone := beginRegionPass(t)
	defer rbDone()
	rb.noteFolded(lam)
	merged := core.NewDynamicCarrier(core.TAny)
	rb.RecordBranch(core.BranchRecord{Cond: core.NewInteger(1), HasElse: true, ThenValue: &lam, ElsValue: &two, Out: merged})
	if !rb.Compilable || !rb.foldedMembers[merged.ID] || !rb.eventInfo[rb.producedBy[merged.ID].seq].armLeavesFn {
		t.Errorf("a branch over the folded arm records, flags and taints its merge (%q)", rb.Reason)
	}
	// A named fn unit returning it: its call results are folded.
	rec := &fnUnitRec{}
	es.fnResidualReplayReason(&emitUnit{localByID: map[string]int{}}, rec, []core.Value{lam}, nil, 0)
	if !rec.returnsFolded || es.armReadCompileFailure != "" {
		t.Fatalf("a named unit's folded return is carried, not declined (%q)", es.armReadCompileFailure)
	}
	unit, _, ok := es.StartFnCompile("g", "g", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok {
		t.Fatal("StartFnCompile declined")
	}
	es.fnRecs[unit].returnsFolded = true
	res := core.NewDynamicCarrier(core.TAny)
	es.RecordUserCall(unit, "g", nil, []core.Value{{}, res}, core.SrcPos{}, core.SrcPos{})
	if !es.foldedMembers[res.ID] || es.foldedMembers[""] {
		t.Error("the call result of a unit returning the folded member is folded")
	}
	// A def of such a carrier, unclaimed, is read as data while it holds a fn.
	es.noteFnLeavingBind(res)
	if !es.fnLeavingBinds[res.ID] {
		t.Error("an unclaimed folded carrier's def is marked")
	}
	claimed := core.NewDynamicCarrier(core.TAny)
	es.noteFolded(claimed)
	es.gradualClaims = map[string]bool{claimed.ID: true}
	es.noteFnLeavingBind(claimed)
	if es.fnLeavingBinds[claimed.ID] {
		t.Error("a claimed folded carrier is the def-read model's")
	}
	// RecordDynMethod clears the pending fire of the read it dispatches.
	es.pendingFoldedFire = lam.ID
	es.RecordDynMethod(lam, nil, nil, "f", core.SrcPos{})
	if es.pendingFoldedFire != "" {
		t.Error("the model's dispatch of the folded read settles it")
	}
	es.pendingFoldedFire = "other"
	es.RecordDynMethod(lam, nil, nil, "f", core.SrcPos{})
	if es.pendingFoldedFire != "other" {
		t.Error("another fn's dispatch settles nothing")
	}
}

func TestGradualReadAccounting(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()
	es.gradualClaims = map[string]bool{"j1": true, "j2": true}
	reg.Check.NoteFnShape(core.Value{ID: "j1"}, core.FnShape{Arity: 0})
	if _, ok := es.producerReturnedClosureArity("j1"); ok {
		t.Error("a gradual claim is the def-read model's alone: no other shape reader sees it")
	}
	// The read the model is stepping answers once; a re-step does not.
	es.NoteDefRead("j1", "j")
	if es.pendingGradualRead != "j1" {
		t.Fatalf("a gradual claim's read is pending, got %q", es.pendingGradualRead)
	}
	if name, ok := es.DefReadName("j1"); !ok || name != "j" {
		t.Errorf("the pending read answers, got %q/%v", name, ok)
	}
	if name, ok := es.DefReadName("j1"); ok || name != "" {
		t.Errorf("a value carrying the read's identity is data, got %q/%v", name, ok)
	}
	es.flushGradualRead()
	if es.armReadCompileFailure != "" {
		t.Fatalf("a read the model saw poisons nothing, got %q", es.armReadCompileFailure)
	}
	// A read the model never sees — `def k j` — declines at the next event.
	es.NoteDefRead("j1", "j")
	es.NoteDefRead("j2", "j")
	if !strings.Contains(es.armReadCompileFailure, "read of `j`") || !strings.Contains(es.armReadCompileFailure, "NUR216") {
		t.Errorf("an unseen gradual read declines, got %q", es.armReadCompileFailure)
	}
	if es.pendingGradualRead != "j2" {
		t.Errorf("the next read is pending, got %q", es.pendingGradualRead)
	}
	es.flushGradualRead()
	if es.pendingGradualRead != "" {
		t.Error("a flush past an existing poison still clears the pending read")
	}
	es.armReadCompileFailure = ""
	es.trapAt = 2
	es.NoteDefRead("j1", "j")
	es.flushGradualRead()
	if es.armReadCompileFailure != "" {
		t.Error("past a terminal trap the read is unreachable and poisons nothing")
	}
	es.trapAt = 0
	// A folded carrier's claimed read is the model's to fire: pending until
	// its dispatch, and inside a unit it declines.
	es.noteFolded(core.Value{ID: "j1"})
	es.NoteDefRead("j1", "j")
	es.DefReadName("j1")
	if es.pendingFoldedFire != "j1" || es.armReadCompileFailure != "" {
		t.Errorf("a top-level folded read awaits its fire, got %q/%q", es.pendingFoldedFire, es.armReadCompileFailure)
	}
	es.pendingFoldedFire = ""
	// An open unit carries its record, as every real one does (the kept-defs
	// observer walks the open records on a read).
	es.fnRecs = []*fnUnitRec{{}}
	es.openUnitRecs = []int{0}
	es.NoteDefRead("j1", "j")
	es.DefReadName("j1")
	n207WantEscape(t, es, "a folded read in a unit", "a def-bound read inside a fn or code body")
}
