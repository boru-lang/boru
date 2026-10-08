package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordBindTwin's contract (§6.5's inert-emission stage): unconditional
// append — no Active()/Suspend gate, because NoteBindTransition already
// applied every ledger suppression and a second filter would be a second
// source of truth — nil-receiver safe like every EmitState method, and the
// finalized Program carries a COPY, so a later append on the same EmitState
// cannot mutate a delivered Program.
func TestRecordBindTwinAppendsUnconditionally(t *testing.T) {
	var nilES *EmitState
	nilES.RecordBindTwin(core.BindTransition{Name: "x"}, core.DefEntry{}) // must not panic

	es := NewEmitState()
	resume := es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1},
		core.DefEntry{Body: core.NewInteger(7)})
	resume()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindUndef, Name: "a", Depth: 0}, core.DefEntry{})
	if len(es.bindTwins) != 2 || len(es.bindTwinEntries) != 2 {
		t.Fatalf("twin table = %d/%d entries, want 2/2 (suspension must not filter)",
			len(es.bindTwins), len(es.bindTwinEntries))
	}

	// The suspended-recording twin has no placed op, and since the flip
	// that DECLINES: an unplaced twin is a transition the rollback would lose
	// (the full-placement gate). The table half is still unconditional —
	// both entries are there for the gate to find.
	if prog, reason, ok := es.Finalize(nil); ok || prog != nil ||
		!strings.Contains(reason, "no stream placement") {
		t.Fatalf("a table-only twin must decline at the placement gate, got ok=%v prog=%v reason=%q", ok, prog, reason)
	}
	if es.bindTwins[0].Name != "a" || es.bindTwins[0].Kind != core.BindDef || es.bindTwins[1].Kind != core.BindUndef {
		t.Fatalf("twin table = %v, want the recorded entries verbatim", es.bindTwins)
	}
}

// countBindTwinOps mirrors Finalize's placement scan for the assertions below.
func countBindTwinOps(p *Program) int {
	placed := 0
	for _, in := range p.Code {
		if in.Op == OpBindTwin {
			placed++
		}
	}
	for i := range p.Fns {
		for _, in := range p.Fns[i].Code {
			if in.Op == OpBindTwin {
				placed++
			}
		}
	}
	return placed
}

// Finalize's placement contract (§6.5's rollback-and-replay, the only
// regime since the flip): a program declines unless EVERY twin-table entry
// has a placed op — a table-only twin (suspended recording) is a
// transition the rollback would lose.
func TestFinalizePlacementGate(t *testing.T) {
	// Fully-placed twins finalize.
	es := NewEmitState()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "a", Depth: 1},
		core.DefEntry{Body: core.NewInteger(7)})
	prog, reason, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("fully-placed regime finalize declined: %s", reason)
	}
	if got := countBindTwinOps(prog); got != len(prog.BindTwins) || got != 1 {
		t.Fatalf("placed %d twin ops for a %d-entry table, want full placement", got, len(prog.BindTwins))
	}

	// A table-only twin declines under the regime.
	es = NewEmitState()
	resume := es.Suspend()
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "b", Depth: 1},
		core.DefEntry{Body: core.NewInteger(9)})
	resume()
	if _, reason, ok := es.Finalize(nil); ok || reason == "" {
		t.Fatalf("a table-only twin must decline the regime program (ok=%v reason=%q)", ok, reason)
	}
}

// tokAt builds a body token carrying a source position, for the adoption
// tests.
func tokAt(row, col int) core.Value {
	v := core.NewInteger(1)
	v.SetPos(core.SrcPos{Row: row, Col: col})
	return v
}

// The do-body adoption account (§10's recovery of the do-body class,
// §6.5's replay-never-re-execution): a table-only twin noted by the
// dispatch's own keep-defs body run (KeepDefsBodyGuard's bracket) at a
// token site inside the body's tree is PLACED by AdoptBodyTwins after
// the closure record, so the regime finalizes and the lowered op replays
// the captured entry. Every fence declines instead of placing: a twin at
// no body site, a twin noted OUTSIDE the bracket (an earlier multi-run
// driver of aliased tokens — Codex P1), a twin noted under a nested
// non-keep sub-run (the tainted each-inside-do class — Codex P1), an
// adoption attempted off the root stream (an open unit — Codex P1), and
// a Rollback that discards the adopting round un-marks the flag with the
// truncated event.
func TestFinalizeBodyAdoption(t *testing.T) {
	// A real registry at FnBodyDepth 0: the keep bracket PUBLISHES only for
	// a run that could have noted a twin, so a direct-drive test has to
	// stand where production stands (the sibling multi-run guard's tests
	// already do).
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Check.Begin()()

	noteTwin := func(es *EmitState, row, col int) {
		es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "big", Depth: 1,
			Pos: core.SrcPos{Row: row, Col: col}},
			core.DefEntry{Body: core.NewInteger(7)})
	}
	// A twin noted during the dispatch's own keep-defs body run — the
	// production shape (`do`'s check body run suspends via the keep
	// guard, and the note fires inside it).
	keepRunTwin := func(es *EmitState, row, col int) {
		end := es.KeepDefsBodyGuard(reg, "")
		noteTwin(es, row, col)
		end()
	}
	// The do's code body: a nested list tree whose leaf sits at (1,6),
	// pinning the recursive site walk.
	body := func() core.Value {
		b := core.NewList([]core.Value{tokAt(1, 4), core.NewList([]core.Value{tokAt(1, 6)})})
		b.SetPos(core.SrcPos{Row: 1, Col: 3})
		return b
	}

	// Nil and inactive receivers are no-ops, like every EmitState method.
	var nilES *EmitState
	nilES.AdoptBodyTwins(body())
	nilES.KeepDefsBodyGuard(nil, "")()

	// Bracketed, at a body site: adopted (a real placed op), finalizes. A
	// second adoption pass is a no-op — the twin is already placed.
	es := NewEmitState()
	keepRunTwin(es, 1, 6)
	es.AdoptBodyTwins(body())
	es.AdoptBodyTwins(body())
	prog, reason, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("an adopted twin must finalize under the regime: %s", reason)
	}
	if got := countBindTwinOps(prog); got != 1 {
		t.Fatalf("adoption placed %d twin ops, want 1", got)
	}

	// At no body site — and a positionless twin at ANY site — still
	// unplaced, declines (the each-body class; a synthesized transition).
	es = NewEmitState()
	es.KeepDefsBodyGuard(reg, "")()
	keepRunTwin(es, 2, 1)
	keepRunTwin(es, 0, 0)
	es.AdoptBodyTwins(body())
	if _, _, ok := es.Finalize(nil); ok {
		t.Fatal("a twin at no body site must still decline the regime program")
	}
	resume := es.Suspend()
	es.AdoptBodyTwins(body()) // suspended recorder: adopt must decline
	resume()

	// OUTSIDE the bracket: a twin noted before the dispatch's own keep
	// run — an earlier multi-run driver of the same (aliased) tokens —
	// must not be adopted even at a matching site (`def q quote [def x 5]
	// ([1 2] each q) q do` under-replays the each's installs otherwise).
	es = NewEmitState()
	resume = es.Suspend()
	noteTwin(es, 1, 6) // the each-analysis note: suspended, no keep bracket
	resume()
	es.KeepDefsBodyGuard(reg, "")() // the do's own body run notes nothing
	es.AdoptBodyTwins(body())
	if _, _, ok := es.Finalize(nil); ok {
		t.Fatal("a twin noted outside the dispatch's own keep-defs run must not be adopted")
	}

	// TAINTED: a twin noted under a nested NON-keep sub-run inside the
	// bracket (`do [[1 2] each [def x 5]]`) is multi-run at runtime —
	// one replay is wrong in count, so it must stay unplaced. A nested
	// KEEP sub-run (do inside do) still adopts.
	es = NewEmitState()
	end := es.KeepDefsBodyGuard(reg, "")
	sub := es.BodyAnalysisGuard() // the nested each's body analysis
	noteTwin(es, 1, 6)
	sub()
	end()
	es.AdoptBodyTwins(body())
	if _, _, ok := es.Finalize(nil); ok {
		t.Fatal("a twin noted under a nested non-keep sub-run must not be adopted")
	}
	es = NewEmitState()
	end = es.KeepDefsBodyGuard(reg, "")
	inner := es.KeepDefsBodyGuard(reg, "") // do inside do: once-run composes
	noteTwin(es, 1, 6)
	inner()
	end()
	es.AdoptBodyTwins(body())
	if _, _, ok := es.Finalize(nil); !ok {
		t.Fatal("a twin noted under a nested KEEP sub-run must still adopt")
	}

	// ROOT FENCE: no adoption while a unit recording is open — a do
	// nested in a callback's compiled body would place its twin inside
	// the per-invocation unit.
	es = NewEmitState()
	keepRunTwin(es, 1, 6)
	es.openUnitRecs = []int{0}
	es.AdoptBodyTwins(body())
	es.openUnitRecs = nil
	if _, _, ok := es.Finalize(nil); ok {
		t.Fatal("adoption inside an open unit recording must decline (the compile failure stands)")
	}

	// Rollback un-marks adopted flags recorded past its checkpoint, so a
	// re-recorded round could adopt again. (Production Rollback brackets
	// loop-analysis rounds whose events ride fragments — where the root
	// fence declines adoption anyway — so this is the safety net for the
	// flag/op agreement, asserted directly on the mechanics; Finalize's
	// op-scan truth is what makes a stale flag merely conservative.)
	es = NewEmitState()
	keepRunTwin(es, 1, 6)
	cp := es.Checkpoint()
	es.AdoptBodyTwins(body())
	if !es.twinPlaced[0] || len(es.twinAdoptions) != 1 {
		t.Fatal("the bracketed adoption must mark the flag and log the adoption")
	}
	es.Rollback(cp)
	if es.twinPlaced[0] || len(es.twinAdoptions) != 0 {
		t.Fatal("Rollback must un-mark the adopted flag so a re-recorded round can adopt again")
	}
}

// The placement scan counts twin ops in FN-UNIT code too. No twin lands
// there today — FnBodyDepth suppresses fn-body transitions at the ledger, so
// only root code carries them — but the arm-resident increment will place
// twins inside compiled units, and a scan blind to unit code would then
// decline programs whose tables ARE fully placed. Driven directly because no
// compilable program can produce the shape yet.
func TestTwinsFullyPlacedCountsFnUnitOps(t *testing.T) {
	p := &Program{
		BindTwins: []core.BindTransition{{Kind: core.BindDef, Name: "a", Depth: 1}},
		Fns:       []CompiledFn{{Name: "f", Code: []Instr{{Op: OpBindTwin}}}},
	}
	if !twinsFullyPlaced(p, nil) {
		t.Fatal("a twin op inside a fn unit must count toward placement")
	}
	p.BindTwins = append(p.BindTwins, core.BindTransition{Kind: core.BindUndef, Name: "a"})
	if twinsFullyPlaced(p, nil) {
		t.Fatal("a table wider than the placed ops must fail the scan")
	}
	// A twin the TRAP truncation dropped is an unreachable transition, not a
	// lost one: the interpreter raises at the trap and never performs it, so
	// the rollback leaving it uninstalled is the matching runtime state and
	// the scan must not demand an op for it.
	if !twinsFullyPlaced(p, map[int]bool{1: true}) {
		t.Fatal("a trap-dropped twin must satisfy the scan without an op")
	}
	if twinsFullyPlaced(p, map[int]bool{0: true}) {
		t.Fatal("the exemption is per index — an unrelated index must not excuse twin 1")
	}
}
