package lang

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// The lang-level forwarders for the eng observability seams
// (eng interp_entry.go): the frontier suite arms them through (*Boru).

// A plain interpreted Run reports Engine.Run entries through the forwarder;
// disarm stops recording.
func TestArmInterpEntryHookForwarder(t *testing.T) {
	a := mustNew(t)
	var entries []InterpEntry
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) { entries = append(entries, e) })

	if _, err := a.RunInterp(`1 add 2`); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sawRun bool
	for _, e := range entries {
		if e.Seam == "Engine.Run" {
			sawRun = true
		}
	}
	if !sawRun {
		t.Fatalf("forwarded hook missed Engine.Run: %+v", entries)
	}

	before := len(entries)
	disarm()
	if _, err := a.RunInterp(`2 add 3`); err != nil {
		t.Fatalf("post-disarm Run: %v", err)
	}
	if len(entries) != before {
		t.Fatalf("disarmed forwarded hook still recorded: %d -> %d", before, len(entries))
	}
}

// The refined-return rematch MATCH (TestDispatchRematchMatchDefers' shape) is
// a real compiled program whose OpDispatchRematch defers at run time: the
// forwarded bail hook must see exactly the vm:rematch-matched site, and the
// run must still resolve to the interpreter's correct result via the
// effect-free silent fallback. (The zz-inst shape-claim violation that used
// to sit here was reclassified 2026-07-15: a host handler violating its own
// registered signature is the host-contract internal_error class, not a
// designed model-miss bail, so it no longer feeds the census.)
// The module-load C4 seam: a module import's preamble/body run reports its
// interpreter entries ATTRIBUTED as "module-load" (the p6/check-prop
// graduation's residual class), and an import-driving compiled program has
// zero unattributed entries. The negative: the attribution never leaks past
// the load — a post-import unattributed interp entry still reports bare.
func TestModuleLoadEntriesAttributed(t *testing.T) {
	a := mustNew(t)
	var (
		mu      sync.Mutex
		entries []InterpEntry
	)
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	defer disarm()
	if _, _, err := a.RunCompiled(`import "boru:test" 1 add 2`); err != nil {
		t.Fatalf("import run: %v", err)
	}
	mu.Lock()
	loads, unattributed := 0, 0
	for _, e := range entries {
		switch e.Attribution {
		case "module-load":
			loads++
		case "":
			unattributed++
		}
	}
	mu.Unlock()
	if loads == 0 {
		t.Errorf("expected module-load-attributed entries from the boru:test preamble, got none")
	}
	if unattributed != 0 {
		t.Errorf("import-driving compiled program: %d unattributed entries", unattributed)
	}
	// Post-import, an explicit interpreter run reports its entries BARE —
	// the load bracket restored the attribution.
	entries = entries[:0]
	if _, err := a.RunInterp(`3 add 4`); err != nil {
		t.Fatalf("post-import interp: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	bare := 0
	for _, e := range entries {
		if e.Attribution == "" {
			bare++
		}
	}
	if bare == 0 {
		t.Errorf("post-import RunInterp must report bare entries (the seam must not leak), got %+v", entries)
	}
}

func TestArmRuntimeBailHookForwarder(t *testing.T) {
	a := mustNew(t)
	var bails []BailEvent
	defer a.ArmRuntimeBailHook(func(e BailEvent) { bails = append(bails, e) })()

	const src = `def Pos (refine Integer) def mk fn [[n:Integer][Integer][def y:Pos n y]] def g fn [[p:Pos][Integer][99]] g (mk 5)`
	got, compiled, err := a.RunCompiled(src)
	if noteCompileDefect(t, src, got, err) {
		return
	}
	if err != nil || compiled {
		t.Fatalf("rematch-match run: compiled=%v err=%v", compiled, err)
	}
	if fmt.Sprint(got) != "[99]" {
		t.Fatalf("rematch-match fallback result = %v, want [99]", got)
	}
	if len(bails) != 1 || bails[0].Site != "vm:rematch-matched" {
		t.Fatalf("bail events = %+v, want exactly one vm:rematch-matched", bails)
	}
	if !strings.Contains(bails[0].Reason, "matched at run time") {
		t.Fatalf("bail reason = %q, want the rematch-matched message", bails[0].Reason)
	}
}
