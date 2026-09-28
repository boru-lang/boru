package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSeatStoredLiveReads pins NUR284's recorder half: a stored-ref unit's
// live read of a name some fn that binds it reaches — the check's binder
// model, over the value's own reader identity (NUR257) — joins
// dynScopeNames, so every binder installs the name where the unit's lookup
// finds it. A read no binder reaches, and a pass with no check state, add
// nothing.
func TestSeatStoredLiveReads(t *testing.T) {
	es := NewEmitState()
	es.storedLiveReads = []storedLiveRead{{name: "k", pos: core.SrcPos{Row: 1, Col: 25}}}
	es.seatStoredLiveReads()
	if es.dynScopeNames["k"] {
		t.Error("no check state: nothing to reach")
	}
	body := core.SrcPos{Row: 1, Col: 24}
	es.reg = &core.Registry{Check: &core.CheckState{
		FnBinders:    map[string]map[string]bool{"k": {"h": true}, "q": {"g": true}},
		FnCallGraph:  map[string]map[string]bool{"h": {core.AnonFnReader(body): true}},
		AnonFnBodies: map[core.SrcPos]core.SrcPos{body: {Row: 1, Col: 30}},
	}}
	es.storedLiveReads = append(es.storedLiveReads,
		storedLiveRead{name: "q", pos: core.SrcPos{Row: 1, Col: 26}},
		storedLiveRead{name: "k", pos: core.SrcPos{Row: 2, Col: 1}})
	es.seatStoredLiveReads()
	if !es.dynScopeNames["k"] {
		t.Error("h binds k and reaches the value the read is in: k joins dynScopeNames")
	}
	if es.dynScopeNames["q"] {
		t.Error("g binds q but never reaches the value: q stays out")
	}
}
