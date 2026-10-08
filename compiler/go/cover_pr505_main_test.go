package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cover_pr505_main_test.go pins two branch-carried helpers on the inputs
// their contracts name but the in-tree producers never hand them: a join
// record naming no binding arm (carryBranchJoin), and a def whose value
// source is none of the kinds a slot store can re-push (dynBindStorable).

// TestCoverPR505MainJoinWithoutBindingArmSeatsNothing: a join is seated only
// through the arms that rebind its name. A record that is not taken and names
// neither arm as binding (core.InstallJoinedDefs never produces one — every
// join it returns binds in at least one arm — but RecordBranch takes the
// record from any caller) carries nothing out: no slot is allocated, the
// joined identity is not aliased, and the branch carries no name. The
// well-formed twin — the then arm binds the name to an inert literal — is
// seated.
func TestCoverPR505MainJoinWithoutBindingArmSeatsNothing(t *testing.T) {
	joined := core.NewCarrier(core.TInteger)
	joined.ID = "joined-x"
	armDef := EmitEvent{kind: evDynBind, dyn: &emitDynBind{name: "x", srcSeq: -1, val: core.NewInteger(1), residentTwin: -1}}
	branch := func() *EmitEvent {
		return &EmitEvent{kind: evBranch, br: &emitBranch{
			then: &EmitFragment{events: []EmitEvent{armDef}},
			els:  &EmitFragment{},
		}}
	}

	es := NewEmitState()
	ev := branch()
	es.carryBranchJoins(ev, core.BranchRecord{HasElse: true, Joins: []core.BranchJoin{{Name: "x", Joined: joined}}})
	u := es.units[0]
	if len(u.nameSlots) != 0 || u.numLocals != 0 {
		t.Errorf("a join naming no binding arm must allocate no slot: slots=%v locals=%d", u.nameSlots, u.numLocals)
	}
	if _, aliased := u.localByID[joined.ID]; aliased || ev.br.carriedNames["x"] || len(ev.br.carried) != 0 {
		t.Errorf("a join naming no binding arm must seat nothing: aliased=%v names=%v carried=%v", aliased, ev.br.carriedNames, ev.br.carried)
	}

	es = NewEmitState()
	ev = branch()
	es.carryBranchJoins(ev, core.BranchRecord{HasElse: true, Joins: []core.BranchJoin{{Name: "x", Joined: joined, ThenBinds: true}}})
	slot, ok := es.units[0].nameSlots["x"]
	if !ok || !ev.br.carriedNames["x"] || es.units[0].localByID[joined.ID] != slot {
		t.Errorf("a then-binding join is seated in the name's slot: slot=%d ok=%v names=%v", slot, ok, ev.br.carriedNames)
	}
	if d := ev.br.then.events[0].dyn; !d.armCarried || d.armSlot != slot {
		t.Errorf("the binding arm's def stores into the slot: armCarried=%v armSlot=%d, want slot %d", d.armCarried, d.armSlot, slot)
	}
}

// TestCoverPR505MainDynBindStorableKinds: a def's value can be re-pushed for
// a frame-slot store only from a non-variadic event, a local, a const, or an
// inert literal. Any other operand kind — a dynamic-scope or read-as-data
// lookup, a type, a closure — is not probed and is not storable, so the name
// stays unseated rather than committing to a program-wide dynamic read.
func TestCoverPR505MainDynBindStorableKinds(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[3] = eventFlags{variadicResult: true}
	for _, tc := range []struct {
		name string
		d    *emitDynBind
		want bool
	}{
		{"a fixed-count event", &emitDynBind{srcSeq: 2}, true},
		{"a variadic event", &emitDynBind{srcSeq: 3}, false},
		{"a local", &emitDynBind{srcSeq: -1, src: localOperand(0)}, true},
		{"a const", &emitDynBind{srcSeq: -1, src: ConstOperand(0)}, true},
		{"an inert literal", &emitDynBind{srcSeq: -1, val: core.NewInteger(1)}, true},
		{"a payload-less carrier", &emitDynBind{srcSeq: -1, val: core.NewCarrier(core.TInteger)}, false},
		{"a dynamic-scope read", &emitDynBind{srcSeq: -1, src: dynScopeOperand(0)}, false},
		{"a read-as-data lookup", &emitDynBind{srcSeq: -1, src: dataScopeOperand(0)}, false},
		{"a type", &emitDynBind{srcSeq: -1, src: typeOperand(0)}, false},
		{"a closure", &emitDynBind{srcSeq: -1, src: EmitOperand{kind: opClosure}}, false},
	} {
		if got := es.dynBindStorable(tc.d); got != tc.want {
			t.Errorf("%s: dynBindStorable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
