package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordDynBind's name gate (the parity oracle's founding fix): a
// `_`/`$`-prefixed ROOT def records its dyn-bind event — the seat of the
// OpBindGlobal push that re-installs the runtime value after the
// rollback; without it ApplyBindTwin's carrier-class skip consumes the
// twin, nothing installs, and the binding is silently lost cross-request.
// The historical skip stands for capitalised names (a type install
// replays through its own twin arms) and inside fn-body recording
// (fn-body defs are frame-locals the ledger never notes). One predicate
// answers for this gate and the loop-region split — recordsFilteredDynBind
// — so the two cannot drift apart again (NUR116 was that drift).
func TestRecordDynBindNameGate(t *testing.T) {
	carrier := core.NewCarrier(core.TList)
	events := func(es *EmitState) int { return len(es.frames[0]) }

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Root: `_` records its dyn-bind event (the push partner's seat).
	es := NewEmitState()
	es.BindRegistry(r)
	es.RecordDynBind("_", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 1 {
		t.Fatal("a `_` root def must record — the push partner's seat")
	}

	// Capitalised: skipped (a type install replays through its own twin
	// arms; no partner is needed or wanted).
	es = NewEmitState()
	es.RecordDynBind("Foo", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 0 {
		t.Fatal("a capitalised name must stay skipped")
	}

	// Fn body: the historical skip stands (fn-body defs are frame-locals;
	// the ledger never notes them, so no twin needs a partner there).
	es = NewEmitState()
	es.BindRegistry(r)
	es.reg.Check.FnBodyDepth = 1
	defer func() { es.reg.Check.FnBodyDepth = 0 }()
	es.RecordDynBind("_", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 0 {
		t.Fatal("a `_` fn-body def must stay skipped")
	}
}

// The two region-split gates share RecordDynBind's name premise through the
// same predicate, and decline an empty or capitalised name outright before
// consulting it — pinned directly, with the producer/event setup each split
// needs to reach its gate. The nil receiver answers "records nothing".
func TestRegionSplitNameGates(t *testing.T) {
	var nilES *EmitState
	if nilES.recordsFilteredDynBind() {
		t.Fatal("a nil recorder records no filtered dyn-bind")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	v := core.NewInteger(3)
	v.ID = "v-split"

	// The loop-region split: a variadic loop result produced at idx 0.
	es := NewEmitState()
	es.BindRegistry(r)
	es.producedBy[v.ID] = producer{seq: 7, idx: 0}
	if es.eventInfo == nil {
		es.eventInfo = map[int]eventFlags{}
	}
	es.eventInfo[7] = eventFlags{variadicResult: true, regionN: 1, firstElemType: core.TInteger}
	if _, ok := es.SplitLoopRegionBind("", v); ok {
		t.Fatal("an empty name must not split a loop region")
	}
	if _, ok := es.SplitLoopRegionBind("Big", v); ok {
		t.Fatal("a capitalised name must not split a loop region")
	}

	// The event-region split reaches its name gate straight after the root
	// guard.
	es = NewEmitState()
	es.BindRegistry(r)
	if _, ok := es.SplitEventRegionBind("", v); ok {
		t.Fatal("an empty name must not split an event region")
	}
	if _, ok := es.SplitEventRegionBind("Big", v); ok {
		t.Fatal("a capitalised name must not split an event region")
	}
}
