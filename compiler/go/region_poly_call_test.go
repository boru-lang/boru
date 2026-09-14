package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The two POLY seats (the sixty-first increment): a runtime-re-matched
// user-fn call and a runtime-re-matched native call each claim the Phase-A
// capture offered for their dispatch, exactly as the mono seats do, and the
// completed descriptor rides their event. Driven at the seam over a
// hand-built window, as region_user_call_test.go drives the user-call seat;
// the wiring from a real program is lang/go/region_capture_e2e_test.go's.

func TestRecordUserPolyCallClaimsItsRegion(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, b := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "f", a, b)
	// word/pos (the VM's re-match name and the blame position) are NOT the
	// claim's key; callWord/wordPos are — the point of the widened signature.
	es.RecordUserPolyCall("f", reg, nil, nil, nil, nil, []core.Value{a, b}, nil,
		core.SrcPos{Row: 1, Col: 3}, "f", pos)
	if es.PendingRegionCount() != 0 {
		t.Fatalf("the offer must be CLAIMED by the record, %d still pending", es.PendingRegionCount())
	}
	frame := es.frames[len(es.frames)-1]
	if len(frame) == 0 {
		t.Fatal("RecordUserPolyCall recorded no event")
	}
	ev := frame[len(frame)-1]
	if ev.kind != evCallUser || ev.uc.poly == nil {
		t.Fatalf("last event kind = %v poly = %v, want a poly evCallUser", ev.kind, ev.uc.poly)
	}
	d := ev.uc.region
	if d == nil {
		t.Fatal("the poly user-call event carries no descriptor — the seat did not claim the capture")
	}
	if d.Word != "f" || d.NFwd != 2 || len(d.Slots) != 2 {
		t.Fatalf("descriptor word=%q NFwd=%d slots=%d, want f/2/2", d.Word, d.NFwd, len(d.Slots))
	}
	if len(ev.uc.ops) != 2 {
		t.Errorf("ops = %d, want 2 — the claim must not disturb the operand list", len(ev.uc.ops))
	}
}

// A poly user call keyed by the BLAME position alone would miss: the offer
// is under the word's position, and a miss records the event with no
// descriptor. Pinned so the defect the increment fixed cannot come back
// silently through this seat.
func TestRecordUserPolyCallKeyedByBlamePosMisses(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	capture(t, es, reg, "f", a)
	blame := core.SrcPos{Row: 1, Col: 3}
	es.RecordUserPolyCall("f", reg, nil, nil, nil, nil, []core.Value{a}, nil, blame, "f", blame)
	if es.PendingRegionCount() != 1 {
		t.Fatalf("a claim keyed by args[0]'s position must miss, leaving the offer pending; %d pending", es.PendingRegionCount())
	}
	frame := es.frames[len(es.frames)-1]
	if ev := frame[len(frame)-1]; ev.kind != evCallUser || ev.uc.region != nil {
		t.Errorf("event kind=%v region=%v, want a poly evCallUser with no descriptor", ev.kind, ev.uc.region)
	}
}

func TestRecordPolyCallClaimsItsRegion(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a, b := core.NewInteger(1), core.NewInteger(2)
	pos := capture(t, es, reg, "f", a, b)
	// The native poly's pos IS the word's at every call site, so the mono
	// seat's key is this seat's key: no extra parameter.
	if !es.RecordPolyCall("f", []core.Value{a, b}, []core.Value{carrierVal(core.TInteger)}, pos, reg, nil) {
		t.Fatal("RecordPolyCall declined a resolvable window")
	}
	if es.PendingRegionCount() != 0 {
		t.Fatalf("the offer must be CLAIMED by the record, %d still pending", es.PendingRegionCount())
	}
	frame := es.frames[len(es.frames)-1]
	if len(frame) == 0 {
		t.Fatal("RecordPolyCall recorded no event")
	}
	ev := frame[len(frame)-1]
	if ev.kind != evCall || !ev.call.poly {
		t.Fatalf("last event kind = %v poly = %v, want a poly evCall", ev.kind, ev.call.poly)
	}
	d := ev.call.region
	if d == nil {
		t.Fatal("the poly native-call event carries no descriptor — the seat did not claim the capture")
	}
	if d.Word != "f" || d.NFwd != 2 || len(d.Slots) != 2 {
		t.Fatalf("descriptor word=%q NFwd=%d slots=%d, want f/2/2", d.Word, d.NFwd, len(d.Slots))
	}
	for i := range d.Slots {
		if d.Slots[i].Source != SlotConst {
			t.Errorf("slot %d source = %v, want SlotConst", i, d.Slots[i].Source)
		}
	}
}

// A native poly call with no capture to claim records its event with no
// descriptor — a miss is ordinary, never an error, on this seat as on the
// others.
func TestRecordPolyCallWithoutACaptureCarriesNoRegion(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()

	a := core.NewInteger(1)
	if !es.RecordPolyCall("g", []core.Value{a}, []core.Value{carrierVal(core.TInteger)}, core.SrcPos{Row: 1, Col: 1}, reg, nil) {
		t.Fatal("RecordPolyCall declined a resolvable window")
	}
	frame := es.frames[len(es.frames)-1]
	if ev := frame[len(frame)-1]; ev.kind != evCall || !ev.call.poly || ev.call.region != nil {
		t.Errorf("event kind=%v poly=%v region=%v, want a poly evCall with no descriptor", ev.kind, ev.call.poly, ev.call.region)
	}
}
