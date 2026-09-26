package core

import "testing"

// TestNoteSlotBoundReadAndRescue pins NUR064's check-state seam from core's
// own suite: a slot-bound read is noted only in check mode, with a name; and
// the end-of-pass rescue drops exactly the undefined_word diagnostic at the
// noted token — the same name at another token is still reported.
func TestNoteSlotBoundReadAndRescue(t *testing.T) {
	var none *CheckState
	none.NoteSlotBoundRead("x", SrcPos{Row: 1, Col: 5}) // nil state: no-op

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	at := SrcPos{Row: 1, Col: 5}
	r.Check.NoteSlotBoundRead("x", at) // not in check mode: no-op
	if len(r.Check.SlotBoundReads) != 0 {
		t.Fatalf("a read outside check mode is not noted: %v", r.Check.SlotBoundReads)
	}
	r.Check.Mode = true
	r.Check.NoteSlotBoundRead("", at) // no name: no-op
	if len(r.Check.SlotBoundReads) != 0 {
		t.Fatalf("a nameless read is not noted: %v", r.Check.SlotBoundReads)
	}
	r.Check.NoteSlotBoundRead("x", at)
	r.Check.NoteSlotBoundRead("x", at) // into the map the first note made
	if len(r.Check.SlotBoundReads) != 1 || !r.Check.SlotBoundReads[SlotRead{Name: "x", Row: 1, Col: 5}] {
		t.Fatalf("the slot-bound read is noted once, by name and position: %v", r.Check.SlotBoundReads)
	}

	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Word: "x", FnBody: true, Row: 1, Col: 5})
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Word: "x", FnBody: true, Row: 2, Col: 1})
	r.RescueForwardRefDiagnostics()
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Row != 2 {
		t.Fatalf("only the noted token is rescued: %+v", r.Check.Diagnostics)
	}
}
