package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur334_val_read_test.go dials NoteValReadLive (kept_defs.go, NUR334): the
// `/v` read's share of the kept-defs discipline a bare read takes through
// NoteDefRead and the tag hook. The whole-program pins are lang's
// TestNUR334*.

// A `/v` read of a name a computed keep-defs body leaked, while the latch is
// armed, is seated live: a fresh identity, a carrier of its type (so nothing
// downstream bakes the model's pre-body value), and a one-result live event
// at the read — the name joins the live-read names, so a miss raises.
func TestNoteValReadLiveSeatsALeakedName(t *testing.T) {
	es := NewEmitState()
	es.latchKeptDefs("do")
	es.keepLeakNames = map[string]bool{"t": true}
	es.dynLeakNames = map[string]bool{"t": true}
	v := core.NewInteger(0)
	v.ID = "nur334-t"
	at := core.SrcPos{Row: 1, Col: 43}
	es.NoteValReadLive(&v, "t", at)
	if v.ID == "nur334-t" || !v.Carrier || !v.Dynamic || !v.Parent.Equal(core.TInteger) {
		t.Fatalf("the read is seated live as a dynamic(Integer) carrier with its own identity — the computed body may have bound any type — got %+v", v)
	}
	if es.armReadCompileFailure != "" || es.pendingKeptRead != "" {
		t.Fatalf("a seated read observes nothing stale: poison=%q pending=%q", es.armReadCompileFailure, es.pendingKeptRead)
	}
	if !es.liveReadNames["t"] || !es.liveReadIDs[v.ID] || es.defReads[v.ID] != "t" {
		t.Errorf("the live read is named: names=%v ids=%v reads=%v", es.liveReadNames, es.liveReadIDs, es.defReads)
	}
	ev := es.frames[0][len(es.frames[0])-1]
	if ev.kind != evCall || !ev.call.live || !ev.call.liveRef || ev.call.pos != at {
		t.Errorf("a live `/v` event at the read's token, got %+v", ev)
	}
	if pr, ok := es.producedBy[v.ID]; !ok || pr.seq != ev.seq {
		t.Errorf("the read's producer is the live event: %v %v", pr, ok)
	}
	// It lowers to the value spelling's lookup (OpLookupDynScopeRef), where
	// a bare read's live event lowers to OpLookupDynScope.
	cf := &CompiledFn{}
	lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
		sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{}, promoted: map[int]int{}}
	if reason := lw.lowerCall(&ev); reason != "" {
		t.Fatalf("the live /v event lowers: %s", reason)
	}
	if len(cf.Code) != 1 || cf.Code[0].Op != OpLookupDynScopeRef || cf.Debug[0] != at {
		t.Fatalf("LOOKUP_DYN_SCOPE_REF at the read token: %v %v", cf.Code, cf.Debug)
	}
	bare := ev
	bare.call.liveRef = false
	if reason := lw.lowerCall(&bare); reason != "" || cf.Code[1].Op != OpLookupDynScope {
		t.Fatalf("a bare live read keeps OpLookupDynScope: %v %q", cf.Code, reason)
	}
}

// Where the bare read is an OBSERVER the `/v` read is one too: a name no
// keep-defs body leaked, read while the latch is armed, poisons the latch
// (NUR210); so does a leaked name whose value may be a fn at run time — the
// Any node, a Function carrier (keptReadMayBeFn) — which the live lookup
// cannot push as the `/v` read's data.
func TestNoteValReadLiveObservers(t *testing.T) {
	for label, c := range map[string]struct {
		v      core.Value
		leaked bool
	}{
		"an unleaked name":   {core.NewInteger(0), false},
		"the Any node":       {core.NewTypeLiteral(core.TAny), true},
		"a Function carrier": {core.NewCarrier(core.TFunction), true},
	} {
		es := NewEmitState()
		es.latchKeptDefs("do")
		if c.leaked {
			es.keepLeakNames = map[string]bool{"t": true}
			es.dynLeakNames = map[string]bool{"t": true}
		}
		v := c.v
		v.ID = "nur334-obs"
		es.NoteValReadLive(&v, "t", core.SrcPos{Row: 1, Col: 1})
		if es.pendingKeptRead != "" && c.leaked {
			// A leaked name's read waits for its seat; the next event
			// settles it (keptDefsEvent's flush).
			t.Errorf("%s: the unseatable read is settled at once", label)
		}
		es.flushKeptRead()
		if !strings.Contains(es.armReadCompileFailure, "the read of `t`") || !strings.Contains(es.armReadCompileFailure, "(NUR210)") {
			t.Errorf("%s: the read is an observer and declines, got %q", label, es.armReadCompileFailure)
		}
		if v.ID != "nur334-obs" || es.liveReadNames["t"] {
			t.Errorf("%s: an observer is not seated, got %+v", label, v)
		}
	}
}

// Nothing for a read the discipline does not concern: an inactive recorder,
// a value with no identity, no name, and — with no latch and no leak — a
// plain read, which keeps its home.
func TestNoteValReadLiveNoOps(t *testing.T) {
	es := NewEmitState()
	v := core.NewInteger(3)
	v.ID = "nur334-plain"
	es.NoteValReadLive(nil, "t", core.SrcPos{})
	es.NoteValReadLive(&v, "", core.SrcPos{})
	noID := core.NewInteger(4)
	es.NoteValReadLive(&noID, "t", core.SrcPos{})
	es.NoteValReadLive(&v, "t", core.SrcPos{})
	if v.ID != "nur334-plain" || v.Carrier || es.armReadCompileFailure != "" || len(es.liveReadNames) != 0 {
		t.Errorf("an unlatched, unleaked read is untouched: %+v poison=%q", v, es.armReadCompileFailure)
	}
	es.latchKeptDefs("do")
	es.keepLeakNames = map[string]bool{"t": true}
	resume := es.Suspend()
	es.NoteValReadLive(&v, "t", core.SrcPos{})
	resume()
	if v.ID != "nur334-plain" || es.armReadCompileFailure != "" {
		t.Errorf("a suspended recorder notes nothing: %+v poison=%q", v, es.armReadCompileFailure)
	}
}

// A LITERAL keep-defs body's leak (NoteKeepDefsLeak: the pass ran the
// tokens, so the type is exact) seats its `/v` read live with the static
// carrier; only a computed body's leak goes gradual (computedLeakGradual).
func TestNoteValReadLiveLiteralLeakKeepsItsType(t *testing.T) {
	es := NewEmitState()
	es.keepLeakNames = map[string]bool{"x": true}
	v := core.NewCarrier(core.TInteger)
	v.ID = "nur334-x"
	es.NoteValReadLive(&v, "x", core.SrcPos{Row: 1, Col: 1})
	if v.ID == "nur334-x" || !v.Carrier || v.Dynamic || !es.liveReadNames["x"] {
		t.Fatalf("a literal leak's /v read is seated live, static: %+v", v)
	}
	// A computed-body leak of a concrete value the latch did not settle
	// stays concrete: only a carrier is widened.
	es = NewEmitState()
	es.keepLeakNames = map[string]bool{"y": true}
	es.dynLeakNames = map[string]bool{"y": true}
	w := core.NewInteger(4)
	w.ID = "nur334-y"
	es.NoteValReadLive(&w, "y", core.SrcPos{Row: 1, Col: 1})
	if w.Carrier || w.Dynamic || !es.liveReadNames["y"] {
		t.Fatalf("an unlatched concrete read is seated as it stands: %+v", w)
	}
}

// keepInstallable's type-node arm (NUR333): a keep-defs def of a lowercase
// name to a type node that existed before this pass installs it (the kept
// install pushes it by OpPushType), and one to a node this pass minted keeps
// the skip — the body's type twin replays that node after the unit. An
// adopted alias mints nothing, so its node stays installable.
func TestKeepInstallableTypeNodes(t *testing.T) {
	es := NewEmitState()
	if !es.keepInstallable(EmitOperand{}, -1, core.NewTypeLiteral(core.TInteger)) {
		t.Fatal("a builtin type node is installable")
	}
	minted := core.NewTypeLiteral(core.TInteger)
	minted.ID = "nur333-P"
	if !es.keepInstallable(EmitOperand{}, -1, minted) {
		t.Fatal("a node minted before this pass is installable")
	}
	es.RecordTypeInstall("Q", core.DefEntry{TypeDef: core.TString}, core.SrcPos{})
	if !es.keepInstallable(EmitOperand{}, -1, core.NewTypeLiteral(core.TString)) {
		t.Error("an adopted alias's node stays installable")
	}
	es.RecordTypeInstall("P", core.DefEntry{TypeDef: &minted, Minted: true}, core.SrcPos{})
	if es.keepInstallable(EmitOperand{}, -1, minted) {
		t.Error("a node this pass minted keeps the skip")
	}
	anon := core.NewTypeLiteral(core.TInteger)
	anon.ID = ""
	if es.keepInstallable(EmitOperand{}, -1, anon) {
		t.Error("an identity-less node has no type operand to push")
	}
	// A nil state notes nothing, and does not panic.
	var nilES *EmitState
	nilES.RecordTypeInstall("P", core.DefEntry{TypeDef: &minted, Minted: true}, core.SrcPos{})
}

// A run-time stamp whose keep-defs unit skipped an install declines through
// the arm-read seam (stampKeepSkipped): the first skip names the def, a
// later one leaves it, and after a terminal trap nothing is poisoned.
func TestStampKeepSkippedDeclines(t *testing.T) {
	es := NewEmitState()
	es.stampKeepSkipped("y")
	es.stampKeepSkipped("z")
	if !strings.Contains(es.armReadCompileFailure, "def `y`") || !strings.Contains(es.armReadCompileFailure, "no bind twin") {
		t.Fatalf("the first skipped def declines the stamp, got %q", es.armReadCompileFailure)
	}
	es = NewEmitState()
	es.trapAt = 1
	es.stampKeepSkipped("y")
	if es.armReadCompileFailure != "" {
		t.Errorf("after a terminal trap the def is unreachable: %q", es.armReadCompileFailure)
	}
}
