package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// RunProgram rolls the registry's bindings back to the Program's ReplayBase
// before executing — §6.5's rollback, carried by the program so the low-level
// compile-then-run flow inherits it (Codex P1 on #426: without it the twin
// replay stacked a second install on the check pass's kept one — `def X
// (refine Integer)` at depth 2, a later `undef X` leaving one live). Pinned
// with a hand-built program: the base is captured, "the check pass" installs,
// the run's twin replays — depth 1, not 2 — and a second run of the same
// program lands at depth 1 again (the restore is from a clone). The
// negatives: a program whose base was captured from ANOTHER registry
// restores nothing here (the caller owns a foreign registry), and a base-less
// hand-built program restores nothing either — both stack to depth 2.
func TestRunProgramRollsBackToReplayBase(t *testing.T) {
	newReg := func() *core.Registry {
		r, err := core.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		r.InitRootContext()
		return r
	}
	build := func(base core.BindingSandbox, reg *core.Registry) *compiler.Program {
		return &compiler.Program{
			Code:            []compiler.Instr{{Op: compiler.OpBindTwin, Arg: 0}},
			Debug:           []core.SrcPos{{Row: 1, Col: 1}},
			BindTwins:       []core.BindTransition{{Kind: core.BindDef, Name: "X", Depth: 1}},
			BindTwinEntries: []core.DefEntry{{Body: core.NewInteger(5)}},
			ReplayBase:      base,
			ReplayReg:       reg,
		}
	}

	r := newReg()
	base := r.SnapshotBindings()
	r.Defs.Push("X", core.NewInteger(5)) // the check pass's install
	p := build(base, r)
	if _, err := RunProgram(p, r); err != nil {
		t.Fatal(err)
	}
	if d := r.Defs.Depth("X"); d != 1 {
		t.Fatalf("X depth after compile-then-run = %d, want 1 (the pass's install rolled back, the twin's replay in its place)", d)
	}
	if _, err := RunProgram(p, r); err != nil {
		t.Fatal(err)
	}
	if d := r.Defs.Depth("X"); d != 1 {
		t.Fatalf("a second run must roll back to the same base: depth %d, want 1", d)
	}

	// A base captured from another registry: nothing restored here.
	r2 := newReg()
	r2.Defs.Push("X", core.NewInteger(5))
	if _, err := RunProgram(build(base, r), r2); err != nil {
		t.Fatal(err)
	}
	if d := r2.Defs.Depth("X"); d != 2 {
		t.Fatalf("a foreign base must not be restored: depth %d, want 2 (the caller owns its registry)", d)
	}

	// No base at all — a hand-built program.
	r3 := newReg()
	r3.Defs.Push("X", core.NewInteger(5))
	if _, err := RunProgram(build(core.BindingSandbox{}, nil), r3); err != nil {
		t.Fatal(err)
	}
	if d := r3.Defs.Depth("X"); d != 2 {
		t.Fatalf("a base-less program must restore nothing: depth %d, want 2", d)
	}

	// A program that recorded NO transition restores nothing either: its
	// registry already stands at the base (table == ledger), and the clone
	// would be paid for nothing on every run. The caller's own later install
	// is therefore untouched by the run.
	r4 := newReg()
	base4 := r4.SnapshotBindings()
	r4.Defs.Push("Y", core.NewInteger(9)) // not the pass's: the caller's, after the base
	p4 := &compiler.Program{
		Code:       []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}},
		Debug:      []core.SrcPos{{Row: 1, Col: 1}},
		Consts:     []core.Value{core.NewInteger(1)},
		ReplayBase: base4,
		ReplayReg:  r4,
	}
	if _, err := RunProgram(p4, r4); err != nil {
		t.Fatal(err)
	}
	if d := r4.Defs.Depth("Y"); d != 1 {
		t.Fatalf("a twin-less program must not roll the registry back: depth %d, want 1", d)
	}

	// A program that reads a stored handler's deps live (LiveLeadNames,
	// LiveReadNames — the seventy-first increment) but recorded no
	// transition restores nothing either: a live read implies nothing to
	// replay (review of #467, where the sets had joined the predicate and a
	// later request's rebind was undone by a re-run).
	r5 := newReg()
	base5 := r5.SnapshotBindings()
	r5.Defs.Push("Y", core.NewInteger(9))
	p5 := &compiler.Program{
		Code:          []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}},
		Debug:         []core.SrcPos{{Row: 1, Col: 1}},
		Consts:        []core.Value{core.NewInteger(1)},
		ReplayBase:    base5,
		ReplayReg:     r5,
		LiveLeadNames: map[string]bool{"helper": true},
		LiveReadNames: map[string]bool{"k": true},
	}
	if _, err := RunProgram(p5, r5); err != nil {
		t.Fatal(err)
	}
	if d := r5.Defs.Depth("Y"); d != 1 {
		t.Fatalf("a live-read program with no transition must not roll the registry back: depth %d, want 1", d)
	}
}
