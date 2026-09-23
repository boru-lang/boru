package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPlacedValReadAndForwardLeftoverGuards pins the nil-safe edges of the
// two collapse-mark readers NUR184 / NUR185 added: a nil state, an ID-less
// value, a value never `/v`-read, one under a pending apply, one also read
// bare, and a registry with no check state all answer false; the marked
// cases answer true.
func TestPlacedValReadAndForwardLeftoverGuards(t *testing.T) {
	var nilES *EmitState
	if nilES.placedValRead("x") || nilES.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("a nil state answers false")
	}
	es := NewEmitState()
	if es.placedValRead("") || es.placedValRead("x") {
		t.Error("an ID-less or never-read value is not a placed /v read")
	}
	es.valReadNoted = map[string]bool{"x": true, "y": true}
	if !es.placedValRead("x") {
		t.Error("a noted /v read with no registry is placed")
	}
	if es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("no registry: no leftover mark")
	}
	es.reg = &core.Registry{}
	if !es.placedValRead("x") || es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("a registry with no check state: placed, no leftover mark")
	}
	es.reg.Check = &core.CheckState{
		WordReadFnIDs:        map[string]bool{"y": true},
		ForwardLeftoverFnIDs: map[string]bool{"z": true},
	}
	if !es.placedValRead("x") {
		t.Error("x is /v-read only: placed")
	}
	if es.placedValRead("y") {
		t.Error("y is also read bare: the word dispatch wins")
	}
	if !es.forwardLeftoverFn(core.Value{ID: "z"}) || es.forwardLeftoverFn(core.Value{ID: ""}) || es.forwardLeftoverFn(core.Value{ID: "x"}) {
		t.Error("the leftover mark is keyed by value ID")
	}
}
