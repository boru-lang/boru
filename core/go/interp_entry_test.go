package core

import (
	"strings"
	"sync"
	"testing"
)

// The observability seams (interp_entry.go): the interpreter-entry hook and
// the runtime-bail hook the frontier suite drives its "ran compiled" /
// "zero runtime bails" assertions through. Inert unless armed; pointer-shared
// into forks; nil-safe on registries assembled without NewRegistry.

// entryCollector gathers InterpEntry events thread-safely (forks may emit
// concurrently under the -race lanes).
type entryCollector struct {
	mu      sync.Mutex
	entries []InterpEntry
}

func (c *entryCollector) add(e InterpEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *entryCollector) seams() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.entries))
	for i, e := range c.entries {
		out[i] = e.Seam
	}
	return out
}

// A plain pooled sub-run enters BOTH runPooledSub and Engine.Run; the armed
// hook sees them with empty attribution (not check mode), and disarm stops
// further recording.
func TestInterpEntryHookSeamsAndDisarm(t *testing.T) {
	r := runUnitReg(t)
	var c entryCollector
	disarm := r.ArmInterpEntryHook(c.add)

	if _, err := RunPooledSub(r, []Value{NewInteger(7)}, false); err != nil {
		t.Fatalf("runPooledSub: %v", err)
	}
	seams := strings.Join(c.seams(), " ")
	if !strings.Contains(seams, "runPooledSub") || !strings.Contains(seams, "Engine.Run") {
		t.Fatalf("armed hook missed seams: got %q, want runPooledSub and Engine.Run", seams)
	}
	for _, e := range c.entries {
		if e.Attribution != "" || e.CheckMode {
			t.Fatalf("plain run entry mis-attributed: %+v", e)
		}
	}

	before := len(c.seams())
	disarm()
	if _, err := RunPooledSub(r, []Value{NewInteger(8)}, false); err != nil {
		t.Fatalf("post-disarm runPooledSub: %v", err)
	}
	if got := len(c.seams()); got != before {
		t.Fatalf("disarmed hook still recorded: %d -> %d entries", before, got)
	}
}

// Check-mode entries carry the "check-mode" attribution — the compiler
// front-end is a declared C4 carve-out, never an unattributed entry.
func TestInterpEntryHookCheckModeAttribution(t *testing.T) {
	r := runUnitReg(t)
	var c entryCollector
	defer r.ArmInterpEntryHook(c.add)()

	r.Check.Mode = true
	if _, err := RunPooledSub(r, []Value{NewInteger(1)}, false); err != nil {
		t.Fatalf("check-mode runPooledSub: %v", err)
	}
	r.Check.Mode = false

	if len(c.entries) == 0 {
		t.Fatal("check-mode run recorded no entries")
	}
	for _, e := range c.entries {
		if e.Attribution != "check-mode" || !e.CheckMode {
			t.Fatalf("check-mode entry mis-attributed: %+v", e)
		}
	}
}

// RunResolved and CallBoru are their own seams; InvokeCallback's interpreter
// fallback reports its dedicated seam name BEFORE the CallBoru it delegates to.
func TestInterpEntryHookResolvedAndCallbackSeams(t *testing.T) {
	r := runUnitReg(t)
	var c entryCollector
	defer r.ArmInterpEntryHook(c.add)()

	if _, err := RunResolved(r, nil, []Value{NewInteger(3)}); err != nil {
		t.Fatalf("RunResolved: %v", err)
	}
	if seams := strings.Join(c.seams(), " "); !strings.Contains(seams, "RunResolved") {
		t.Fatalf("RunResolved seam missing: %q", seams)
	}

	c.entries = nil
	// An unstamped callback sig falls to CallBoru through the callback seam.
	sig := &Signature{Impl: &BoruImpl{Body: []Value{NewInteger(42)}}}
	if _, err := InvokeCallback(r, sig, nil, nil); err != nil {
		t.Fatalf("InvokeCallback: %v", err)
	}
	seams := strings.Join(c.seams(), " ")
	if !strings.Contains(seams, "InvokeCallback:callboru") || !strings.Contains(seams, "CallBoru") {
		t.Fatalf("callback seams missing: %q", seams)
	}
}

// A ForkConcurrent copy shares the parent's hook holder, so a fork's
// interpreter entries reach the observer the parent armed.
func TestInterpEntryHookForkPropagation(t *testing.T) {
	r := runUnitReg(t)
	var c entryCollector
	defer r.ArmInterpEntryHook(c.add)()

	fork := r.ForkConcurrent()
	if _, err := RunPooledSub(fork, []Value{NewInteger(5)}, false); err != nil {
		t.Fatalf("fork runPooledSub: %v", err)
	}
	if len(c.entries) == 0 {
		t.Fatal("fork entries did not reach the parent-armed hook")
	}
}

// A registry assembled without NewRegistry has no holders: arming is a no-op
// (with a safe disarm), the note paths are inert, and a nil registry never
// panics — the pre-seam behaviour, exactly.
func TestObserveHooksNilSafe(t *testing.T) {
	bare := &Registry{}
	disarmI := bare.ArmInterpEntryHook(func(InterpEntry) { t.Fatal("armed on nil holder") })
	disarmB := bare.ArmRuntimeBailHook(func(BailEvent) { t.Fatal("armed on nil holder") })
	bare.noteInterp("Engine.Run")
	bare.NoteBail("vm:test", "x")
	disarmI()
	disarmB()

	var nilReg *Registry
	nilReg.noteInterp("Engine.Run") // must not panic
	nilReg.NoteBail("vm:test", "x") // must not panic
}

// NoteInterp is the exported emit point the VM's island seams use (eng/go
// cannot reach the unexported noteInterp). It records the caller's seam
// verbatim and leaves Attribution EMPTY — an island is unattributed
// interpreter execution by definition, and the frontier's
// unattributed-entry assertions depend on it staying that way. Nil-safe
// like its unexported twin.
func TestNoteInterpExportedSeam(t *testing.T) {
	r := runUnitReg(t)
	var c entryCollector
	disarm := r.ArmInterpEntryHook(c.add)
	defer disarm()

	r.NoteInterp("vm:island")
	r.NoteInterp("vm:island-resolved")

	if got := strings.Join(c.seams(), " "); got != "vm:island vm:island-resolved" {
		t.Fatalf("island seams: got %q, want %q", got, "vm:island vm:island-resolved")
	}
	for _, e := range c.entries {
		if e.Attribution != "" || e.CheckMode {
			t.Fatalf("island entry must stay unattributed: %+v", e)
		}
	}

	var nilReg *Registry
	nilReg.NoteInterp("vm:island") // must not panic
}

// The region-oracle seam, same holder discipline as the two above: unarmed
// emits drop, an armed observer sees every event until disarmed, and a
// registry without a holder arms nothing.
func TestRegionOracleHook(t *testing.T) {
	r := newTestRegistry(t)
	r.NoteRegionOracle(RegionOracleEvent{Word: "add", Outcome: "unarmed"})
	var got []RegionOracleEvent
	disarm := r.ArmRegionOracleHook(func(ev RegionOracleEvent) { got = append(got, ev) })
	r.NoteRegionOracle(RegionOracleEvent{Word: "add", NFwd: 2, Outcome: "reproduced"})
	disarm()
	r.NoteRegionOracle(RegionOracleEvent{Word: "add", Outcome: "after-disarm"})
	if len(got) != 1 || got[0].Word != "add" || got[0].NFwd != 2 || got[0].Outcome != "reproduced" {
		t.Errorf("region-oracle events = %+v", got)
	}

	// A module sub-registry reports to the SAME observer the request armed.
	child := newTestRegistry(t)
	child.InheritObserveHooks(r)
	disarm = r.ArmRegionOracleHook(func(ev RegionOracleEvent) { got = append(got, ev) })
	child.NoteRegionOracle(RegionOracleEvent{Word: "m", Outcome: "reproduced"})
	disarm()
	if len(got) != 2 || got[1].Word != "m" {
		t.Errorf("a child registry must report through the inherited holder: %+v", got)
	}

	bare := &Registry{}
	bare.ArmRegionOracleHook(func(RegionOracleEvent) { t.Fatal("armed on nil holder") })()
	bare.NoteRegionOracle(RegionOracleEvent{})
	var nilReg *Registry
	nilReg.NoteRegionOracle(RegionOracleEvent{}) // must not panic
}
