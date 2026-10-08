package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The stored-handler lookup half's seats (the seventy-first increment),
// driven at the recorder: which open unit counts as a stored-ref unit,
// what a unit notes as read live, which bare reads the stored-dep arm
// seats, which leads it marks live, and what a transition of a live-lead
// name compiles or declines.
func TestStoredLiveSeats(t *testing.T) {
	var nilES *EmitState
	if nilES.storedUnitOpen() != nil || nilES.unitLiveNames(0) != nil || nilES.markLiveLead("f") || nilES.storedDepRead("k", core.NewInteger(1)) {
		t.Fatal("a nil recorder has no stored unit, no live names, no live lead")
	}
	nilES.noteUnitLive("k")
	nilES.noteUnitBaked("k")
	nilES.noteLiveNameTransition("f")

	es, reg, done := beginRegionPass(t)
	defer done()
	if es.storedUnitOpen() != nil || es.unitLiveNames(-1) != nil || es.unitLiveNames(99) != nil {
		t.Fatal("no open unit: no stored unit, no live names out of range")
	}
	es.noteUnitLive("k") // no open unit: nothing to note
	es.noteUnitBaked("k")
	if es.storedDepRead("k", core.NewInteger(1)) || es.markLiveLead("f") {
		t.Fatal("outside a stored unit nothing is seated or marked")
	}

	// An ordinary open unit is not a stored one; a stored one is.
	reg.Defs.Push("k", core.NewInteger(6))
	fnv := core.NewFunction(core.FnDefInfo{Name: "helper", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)}), Decl: core.DeclSite{Pos: core.SrcPos{Row: 1, Col: 5}, File: "h.boru"}}}})
	reg.Defs.Push("helper", fnv)
	lam := core.NewFunction(core.FnDefInfo{Name: "lam", Anonymous: true, Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	reg.Defs.Push("lam", lam)
	reg.PushFnBaseline(reg.Defs.Snapshot())
	defer reg.PopFnBaseline()
	unit, _, ok := es.StartFnCompile("u", "u", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	if es.storedUnitOpen() != nil || es.storedDepRead("k", core.NewInteger(6)) || es.markLiveLead("helper") {
		t.Fatal("a plain unit is not a stored-ref unit")
	}
	es.fnRecs[unit].storedRefUnit = true
	if es.storedUnitOpen() != es.fnRecs[unit] {
		t.Fatal("the open stored unit")
	}
	es.noteUnitLive("")
	es.noteUnitBaked("")
	es.openUnitRecs = append(es.openUnitRecs, 99) // an index outside the table notes nothing
	es.noteUnitLive("k")
	es.noteUnitBaked("k")
	es.openUnitRecs = es.openUnitRecs[:len(es.openUnitRecs)-1]
	if es.unitLiveNames(unit)["k"] {
		t.Fatal("a bad open index noted nothing")
	}
	es.noteUnitLive("k")
	if !es.unitLiveNames(unit)["k"] || es.unitLiveNames(unit)[""] {
		t.Fatalf("noted live on the unit: %v", es.unitLiveNames(unit))
	}
	// NoteFrozenRead over an open index whose record is nil notes nothing
	// (the guard ahead of the stored arm).
	es.fnRecs = append(es.fnRecs, nil)
	es.openUnitRecs = append(es.openUnitRecs, len(es.fnRecs)-1)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	es.openUnitRecs = es.openUnitRecs[:len(es.openUnitRecs)-1]
	es.fnRecs = es.fnRecs[:len(es.fnRecs)-1]
	if es.fnRecs[unit].storedBakes["k"] != 0 {
		t.Fatal("a nil record takes no bake note")
	}
	// Seats against bakes: a name read both ways — one seat, two bake
	// notes (a `/v` beside the seated read) — is not live for the ref.
	es.noteUnitBaked("k")
	if !es.unitLiveNames(unit)["k"] {
		t.Fatal("one bake against one seat: the seat took it")
	}
	es.noteUnitBaked("k")
	if es.unitLiveNames(unit)["k"] {
		t.Fatal("a second bake with no seat: the latch's")
	}
	es.noteUnitLive("k")
	// The stored-dep arm: a module-scope value seats; an unbound name, a
	// fn-local name, a fn or class value, an active token do not; a
	// dynamic-tagged read is noted live and seated nowhere new.
	if !es.storedDepRead("k", core.NewInteger(6)) {
		t.Fatal("a module-scope value read seats")
	}
	if es.storedDepRead("nope", core.NewInteger(6)) {
		t.Fatal("an unbound name does not seat")
	}
	reg.Defs.Push("loc", core.NewInteger(1)) // bound INSIDE the unit: fn-local
	if es.storedDepRead("loc", core.NewInteger(1)) {
		t.Fatal("a fn-local binding does not seat")
	}
	if es.storedDepRead("helper", fnv) || es.storedDepRead("k", core.NewWord("k")) {
		t.Fatal("a dispatching binding and an active token do not seat")
	}
	cls := core.Value{Parent: core.TAny, Data: &core.ClassTypeInfo{}}
	if es.storedDepRead("k", cls) {
		t.Fatal("a class binding does not seat")
	}
	dyn := core.NewInteger(6)
	dyn.Dynamic = true
	delete(es.fnRecs[unit].liveNames, "k")
	if es.storedDepRead("k", dyn) || !es.unitLiveNames(unit)["k"] {
		t.Fatal("a dynamic read is live already: noted, not seated")
	}
	// The live lead: a declared module-scope fn marks; a lambda, an
	// unbound name, a data value do not.
	if !es.markLiveLead("helper") || !es.liveLeadNames["helper"] || es.liveLeadNames["k"] {
		t.Fatal("a declared module-scope fn is a live lead")
	}
	if es.markLiveLead("lam") || es.markLiveLead("nope") || es.markLiveLead("k") || es.markLiveLead("") {
		t.Fatal("a lambda, an unbound name, a data value, no name: not a live lead")
	}
	// A transition of a live-lead name: not live → nothing; unbound → the
	// miss raises at run time, nothing; declared → its units; a lambda or
	// a data value → declined; suspended → declined.
	es.noteLiveNameTransition("nope")
	reg.Defs.Pop("helper")
	es.noteLiveNameTransition("helper") // unbound: nothing
	if !es.Compilable {
		t.Fatalf("an unbound live lead compiles nothing and declines nothing: %q", es.Reason)
	}
	reg.Defs.Push("helper", fnv)
	es.noteLiveNameTransition("helper")
	if !es.Compilable {
		t.Fatalf("a declared binding compiles its units: %q", es.Reason)
	}
	resume := es.Suspend()
	es.noteLiveNameTransition("helper")
	resume()
	if es.Compilable || !strings.Contains(es.Reason, "module binding helper rebound to a value with no declared signature after a stored handler dispatched it live") {
		t.Fatalf("a transition while suspended declines: %v %q", es.Compilable, es.Reason)
	}
	es2, reg2, done2 := beginRegionPass(t)
	defer done2()
	es2.liveLeadNames = map[string]bool{"helper": true}
	reg2.Defs.Push("helper", lam)
	es2.noteLiveNameTransition("helper")
	if es2.Compilable || !strings.Contains(es2.Reason, "no declared signature") {
		t.Fatalf("a lambda rebind of a live lead declines: %v %q", es2.Compilable, es2.Reason)
	}
	es2.noteLiveNameTransition("helper") // already uncompilable: nothing more
	// A live READ's transition to a dispatching value declines (review of
	// #467); to a value it does not.
	es3, reg3, done3 := beginRegionPass(t)
	defer done3()
	es3.liveReadNames = map[string]bool{"k": true}
	reg3.Defs.Push("k", core.NewInteger(11))
	es3.noteLiveNameTransition("k")
	if !es3.Compilable {
		t.Fatalf("a value rebind of a live read is the lookup's: %q", es3.Reason)
	}
	reg3.Defs.Push("k", fnv)
	es3.noteLiveNameTransition("k")
	if es3.Compilable || !strings.Contains(es3.Reason, "module binding k rebound to a dispatching value after a stored handler read it live") {
		t.Fatalf("a fn rebind of a live read declines: %v %q", es3.Compilable, es3.Reason)
	}
	if !dispatchingBinding(core.NewWord("w")) || dispatchingBinding(core.NewInteger(1)) {
		t.Fatal("dispatchingBinding: an active token is, a value is not")
	}
}
