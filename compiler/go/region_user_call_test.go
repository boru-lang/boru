package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The user-call seat: RecordUserCall claims the Phase-A capture offered for
// its dispatch, exactly as RecordCall does, and the completed descriptor
// rides the evCallUser event. Driven at the seam, over a hand-built window,
// as the native seat's tests are; the wiring from a real program is
// lang/go/region_capture_e2e_test.go's.
func TestRecordUserCallClaimsItsRegion(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, b := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "f", a, b)
	unit, _, ok := es.StartFnCompile("k", "f", nil, []core.Value{a, b},
		[]*core.Type{core.TInteger, core.TInteger}, []string{"x", "y"}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	// The event's own pos is the first argument's (the blame position); the
	// claim is keyed by the WORD's, passed separately — the point of the
	// widened signature.
	es.RecordUserCall(unit, "f", []core.Value{a, b}, nil, core.SrcPos{Row: 1, Col: 3}, pos)
	if es.PendingRegionCount() != 0 {
		t.Fatalf("the offer must be CLAIMED by the record, %d still pending", es.PendingRegionCount())
	}
	frame := es.frames[len(es.frames)-1]
	if len(frame) == 0 {
		t.Fatal("RecordUserCall recorded no event")
	}
	ev := frame[len(frame)-1]
	if ev.kind != evCallUser {
		t.Fatalf("last event kind = %v, want evCallUser", ev.kind)
	}
	d := ev.uc.region
	if d == nil {
		t.Fatal("the user-call event carries no descriptor — the seat did not claim the capture")
	}
	if d.Word != "f" || d.NFwd != 2 || len(d.Slots) != 2 {
		t.Fatalf("descriptor word=%q NFwd=%d slots=%d, want f/2/2", d.Word, d.NFwd, len(d.Slots))
	}
	for i := range d.Slots {
		if d.Slots[i].Source == SlotNone {
			t.Errorf("slot %d was claimed and left SlotNone", i)
		}
	}
	if len(ev.uc.ops) != 2 {
		t.Errorf("ops = %d, want 2 — the claim must not disturb the operand list", len(ev.uc.ops))
	}
}

// A user call with no capture to claim (a stack-only dispatch never reaches
// forward collection, so Phase A offers nothing) records its event with no
// descriptor — a miss is ordinary, never an error.
func TestRecordUserCallWithoutACaptureCarriesNoRegion(t *testing.T) {
	es, _, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	unit, _, ok := es.StartFnCompile("k", "g", nil, []core.Value{a},
		[]*core.Type{core.TInteger}, []string{"x"}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	es.RecordUserCall(unit, "g", []core.Value{a}, nil, core.SrcPos{Row: 1, Col: 3}, core.SrcPos{Row: 1, Col: 1})
	frame := es.frames[len(es.frames)-1]
	if len(frame) == 0 {
		t.Fatal("RecordUserCall recorded no event")
	}
	if ev := frame[len(frame)-1]; ev.kind != evCallUser || ev.uc.region != nil {
		t.Errorf("event kind=%v region=%v, want evCallUser with no descriptor", ev.kind, ev.uc.region)
	}
}
