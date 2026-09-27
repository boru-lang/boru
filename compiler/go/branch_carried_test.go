package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// branch_carried_test.go pins the recorder half of the BRANCH-CARRIED def
// (branch_carried.go) directly: which joins carryBranchJoin seats, with what
// seed, and the storability rule the arms are held to. lang's
// TestBranchCarriedDefParity and TestBranchCarriedSplitBindParity pin the
// same rules end to end against the interpreter.

// bcArmDef is an arm fragment holding one literal value def of name.
func bcArmDef(name string, v core.Value) *EmitFragment {
	return &EmitFragment{events: []EmitEvent{{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1, val: v, residentTwin: -1}}}}
}

// bcCarrier is a payload-less Integer carrier with a fixed identity.
func bcCarrier(id string) core.Value {
	v := core.NewCarrier(core.TInteger)
	v.ID = id
	return v
}

// bcSeat runs carryBranchJoins for one join over a two-arm branch event whose
// then arm is then.
func bcSeat(es *EmitState, then *EmitFragment, j core.BranchJoin) *EmitEvent {
	ev := &EmitEvent{kind: evBranch, br: &emitBranch{then: then, els: &EmitFragment{}, hasElse: true}}
	es.carryBranchJoins(ev, core.BranchRecord{HasElse: true, Joins: []core.BranchJoin{j}})
	return ev
}

// bcSeated reports the slot the joined identity was aliased to, and whether
// the arm's def stores into it.
func bcSeated(es *EmitState, ev *EmitEvent, joinedID string) (int, bool) {
	slot, ok := es.units[len(es.units)-1].localByID[joinedID]
	if !ok {
		return -1, false
	}
	d := ev.br.then.events[0].dyn
	return slot, d.armCarried && d.armSlot == slot && ev.br.carriedNames[d.name]
}

func TestCarryBranchJoinSeatingRules(t *testing.T) {
	// A CONSTANT-condition join over a pre binding: only the taken arm was
	// analysed and it always runs, so the name is seated with no seed and no
	// bound check — the arm's store is the binding past the merge.
	es := NewEmitState()
	ev := bcSeat(es, bcArmDef("x", core.NewInteger(9)), core.BranchJoin{Name: "x", Joined: bcCarrier("J1"),
		ThenBinds: true, Taken: true, HasPre: true, Pre: bcCarrier("P1")})
	slot, stores := bcSeated(es, ev, "J1")
	if slot < 0 || !stores || es.units[0].nameSlots["x"] != slot {
		t.Fatalf("a taken arm over a pre binding must be seated and store into the name's cell (slot %d, stores %v)", slot, stores)
	}
	if len(ev.br.carried) != 0 || es.units[0].boundLocals["J1"] != "" {
		t.Fatalf("a taken arm always runs: no seed and no bound check, got carried=%v bound=%v", ev.br.carried, es.units[0].boundLocals)
	}

	// A taken arm with NO pre binding pushed its own value, whose home
	// stands: nothing to seat.
	es = NewEmitState()
	ev = bcSeat(es, bcArmDef("y", core.NewInteger(9)), core.BranchJoin{Name: "y", Joined: bcCarrier("J2"), ThenBinds: true, Taken: true})
	if _, seated := es.units[0].localByID["J2"]; seated || ev.br.then.events[0].dyn.armCarried || len(es.units[0].nameSlots) != 0 {
		t.Fatal("a taken arm with no pre binding must be left to its own value's home")
	}

	// A join that names no binding arm is never seated: a slot no arm
	// stores would be read as the merge's binding.
	es = NewEmitState()
	ev = bcSeat(es, bcArmDef("z", core.NewInteger(9)), core.BranchJoin{Name: "z", Joined: bcCarrier("J3"), HasPre: true, Pre: core.NewInteger(1)})
	if _, seated := es.units[0].localByID["J3"]; seated || ev.br.then.events[0].dyn.armCarried || len(es.units[0].nameSlots) != 0 {
		t.Fatal("a join with no binding arm must not be seated")
	}
}

// TestCarryBranchJoinUnplaceablePre pins the seed rule for a pre binding with
// no compiled home. Such a join is left unseated — its read after the merge
// declines — except for a top-level SPLIT bind, whose read resolves live
// through the registry and so must not be left to it: that join is seeded with
// the registry read of the name.
func TestCarryBranchJoinUnplaceablePre(t *testing.T) {
	split := func(t *testing.T) *EmitState {
		es := NewEmitState()
		es.reg = newTestRegistry(t)
		es.loopSplitBinds = map[string]bool{"x": true}
		return es
	}
	oneArm := func(pre core.Value) core.BranchJoin {
		return core.BranchJoin{Name: "x", Joined: bcCarrier("J"), ThenBinds: true, HasPre: true, Pre: pre}
	}

	// A pre binding that resolves to a dynamic-scope read (a split bind read
	// before the branch, NoteDefRead's name): the seed is refused, the join is
	// left unseated, and a commitment the resolution made is undone.
	es := split(t)
	es.defReads = map[string]string{"P": "x"}
	ev := bcSeat(es, bcArmDef("x", core.NewInteger(9)), oneArm(bcCarrier("P")))
	if _, seated := es.units[0].localByID["J"]; seated || ev.br.then.events[0].dyn.armCarried {
		t.Fatal("a pre binding resolving to a dynamic-scope read must not seed the slot")
	}
	if es.dynScopeNames["x"] {
		t.Fatal("the resolution's dynamic-scope commitment must be undone when the name held none")
	}
	// ...and a commitment the name already held stands.
	es = split(t)
	es.defReads = map[string]string{"P": "x"}
	es.dynScopeNames = map[string]bool{"x": true}
	bcSeat(es, bcArmDef("x", core.NewInteger(9)), oneArm(bcCarrier("P")))
	if !es.dynScopeNames["x"] {
		t.Fatal("an earlier commitment of the name must survive the refused seed")
	}

	// A top-level split bind's pre binding with no home at all (the read that
	// would name it has not happened yet): seated, seeded by the registry
	// read of the name, and NO dynamic-scope commitment.
	es = split(t)
	ev = bcSeat(es, bcArmDef("x", core.NewInteger(9)), oneArm(bcCarrier("P")))
	slot, stores := bcSeated(es, ev, "J")
	if slot < 0 || !stores {
		t.Fatalf("a split-bound name's join must be seated (slot %d, stores %v)", slot, stores)
	}
	if len(ev.br.carried) != 1 || ev.br.carried[0].slot != slot || ev.br.carried[0].init.kind != opDynScope {
		t.Fatalf("the seed must be the registry read of the name, got %+v", ev.br.carried)
	}
	if name, err := core.AsString(es.consts[ev.br.carried[0].init.idx]); err != nil || name != "x" {
		t.Fatalf("the seed must look up `x`, got %v (%v)", es.consts[ev.br.carried[0].init.idx], err)
	}
	if es.dynScopeNames["x"] {
		t.Fatal("the split seed must not commit the name to dynamic scope")
	}

	// The same pre for a name that is NOT split-bound declines as before.
	es = split(t)
	es.loopSplitBinds = nil
	ev = bcSeat(es, bcArmDef("x", core.NewInteger(9)), oneArm(bcCarrier("P")))
	if _, seated := es.units[0].localByID["J"]; seated || len(ev.br.carried) != 0 {
		t.Fatal("an unplaceable pre binding of an ordinary name must leave the join unseated")
	}

	// Inside a fn unit the name is not the root's split bind: unseated.
	es = split(t)
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	ev = bcSeat(es, bcArmDef("x", core.NewInteger(9)), oneArm(bcCarrier("P")))
	if _, seated := es.units[1].localByID["J"]; seated || len(ev.br.carried) != 0 {
		t.Fatal("a unit's join over an unplaceable pre binding must stay unseated")
	}
}

// TestFragCanCarrySkipsUnrecordedBranch: a branch event with no branch record
// (a nil payload — no recorder builds one, and every other kind walk skips it
// the same way) neither carries nor blocks the name; the arm's own def does.
func TestFragCanCarrySkipsUnrecordedBranch(t *testing.T) {
	es := NewEmitState()
	def := bcArmDef("x", core.NewInteger(9)).events[0]
	if !fragCanCarry(es, &EmitFragment{events: []EmitEvent{{kind: evBranch}, def}}, "x") {
		t.Fatal("a record-less branch event must be skipped, leaving the arm's def to carry the name")
	}
	if fragCanCarry(es, &EmitFragment{events: []EmitEvent{{kind: evBranch}}}, "x") {
		t.Fatal("a record-less branch event carries nothing on its own")
	}
}

// TestDynBindStorable pins the storability rule a branch-carried def is held
// to at the join: a re-pushable home — a non-variadic computed source, a
// frame local, a const, an inert literal — and nothing else.
func TestDynBindStorable(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[4] = eventFlags{variadicResult: true}
	for _, c := range []struct {
		what string
		d    emitDynBind
		want bool
	}{
		{"a single-value computed source", emitDynBind{srcSeq: 3}, true},
		{"a variadic computed source (a loop's run)", emitDynBind{srcSeq: 4}, false},
		{"a frame local", emitDynBind{srcSeq: -1, src: localOperand(2)}, true},
		{"a const", emitDynBind{srcSeq: -1, src: ConstOperand(0)}, true},
		{"an inert literal", emitDynBind{srcSeq: -1, val: core.NewInteger(9)}, true},
		{"a payload-less carrier", emitDynBind{srcSeq: -1, val: core.NewCarrier(core.TInteger)}, false},
		{"a dynamic-scope read", emitDynBind{srcSeq: -1, src: dynScopeOperand(0)}, false},
	} {
		if got := es.dynBindStorable(&c.d); got != c.want {
			t.Errorf("%s: storable=%v, want %v", c.what, got, c.want)
		}
	}
}

// TestRootArmInstall pins which defs lower a registry install beside their
// branch-carried slot store: a ROOT arm's store, of a split-bound name
// (main's NUR226) or any other (NUR232), unless the def already installs
// through its own write-back.
func TestRootArmInstall(t *testing.T) {
	es := NewEmitState()
	es.loopSplitBinds = map[string]bool{"x": true}
	for _, name := range []string{"x", "y"} {
		arm := emitDynBind{name: name, srcSeq: -1, val: core.NewInteger(9), root: true, armCarried: true}
		if !es.rootArmInstall(&arm) {
			t.Fatalf("a root arm's literal store of %s installs", name)
		}
	}
	for what, d := range map[string]emitDynBind{
		"not branch-carried":   {name: "x", srcSeq: -1, val: core.NewInteger(9), root: true},
		"not at the root":      {name: "x", srcSeq: -1, val: core.NewInteger(9), armCarried: true},
		"written back already": {name: "x", srcSeq: -1, val: core.NewCarrier(core.TInteger), root: true, armCarried: true},
	} {
		if es.rootArmInstall(&d) {
			t.Errorf("%s: must not add an install", what)
		}
	}
}
