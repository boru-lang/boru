package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// tape builds a CollectWindow the seam can read.
func regionTape(vals ...core.Value) core.CollectWindow { return core.NewTape(vals, 0) }

// TestTryRecordRegionCaptures pins Phase A: a forward-collecting dispatch's
// extent and written order are captured, keyed by the (word, pos) join Phase B
// will use.
func TestTryRecordRegionCaptures(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	defer done()
	// AFTER Begin: the check run installs its own recorder, so assigning
	// before it would be overwritten and the seam would silently decline.
	es := NewEmitState()
	reg.Check.Emit = es

	word := core.WithPosAt(core.NewWord("pair"), core.SrcPos{Row: 1, Col: 1})
	win := regionTape(word, core.NewInteger(1), core.NewInteger(2))

	tryRecordRegion(win, reg, core.WordInfo{Name: "pair", ArgCount: -1}, 0)

	if got := es.PendingRegionCount(); got != 1 {
		t.Fatalf("PendingRegionCount = %d, want 1", got)
	}
	d, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 1, Col: 1})
	if !ok {
		t.Fatal("the capture is not reachable under the (word, pos) join Phase B uses")
	}
	if d.Lead != LeadWord || d.Word != "pair" {
		t.Errorf("lead = %v/%q, want LeadWord/pair", d.Lead, d.Word)
	}
	// Slots begin AFTER the dispatching word — the same start the collection
	// walk uses, so the extent recorded is the extent walked.
	if len(d.Slots) != 2 {
		t.Fatalf("captured %d slots, want 2 (the two literals after the word)", len(d.Slots))
	}
	// Phase A leaves Source unset ON PURPOSE: it is not knowable here, and
	// SlotNone is the invalid zero that stops it being read as Consts[0].
	for i, s := range d.Slots {
		if s.Source != SlotNone {
			t.Errorf("slot %d source = %v, want SlotNone until Phase B fills it", i, s.Source)
		}
		// Compared by CanonValue, not DeepEqual: an unevaluated
		// ParenExprPayload is not DeepEqual to ITSELF, which is the trap
		// that once got a per-slot content check deleted rather than fixed.
		if core.CanonValue(s.Token) != core.CanonValue(win.At(i+1)) {
			t.Errorf("slot %d token %q does not match the window token %q it stood for",
				i, core.CanonValue(s.Token), core.CanonValue(win.At(i+1)))
		}
	}
}

// TestTryRecordRegionDeclines pins the guards. Each of these is a real state
// the seam is offered, and capturing in any of them would either crash or
// record a region for a pass that is not recording.
func TestTryRecordRegionDeclines(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	defer done()
	// AFTER Begin: the check run installs its own recorder, so assigning
	// before it would be overwritten and the seam would silently decline.
	es := NewEmitState()
	reg.Check.Emit = es

	w := core.WordInfo{Name: "pair", ArgCount: -1}
	full := regionTape(core.NewWord("pair"), core.NewInteger(1))

	cases := []struct {
		name string
		run  func()
	}{
		{"nil registry", func() { tryRecordRegion(full, nil, w, 0) }},
		{"nil window", func() { tryRecordRegion(nil, reg, w, 0) }},
		{"index past the end", func() { tryRecordRegion(full, reg, w, 9) }},
		{"negative index", func() { tryRecordRegion(full, reg, w, -1) }},
		{"word with nothing after it", func() {
			tryRecordRegion(regionTape(core.NewWord("pair")), reg, w, 0)
		}},
		// A fn VALUE at the lead (a lambda applied in a paren group) is a
		// nameless word: a LeadWord descriptor could not resolve it live.
		{"nameless lead", func() { tryRecordRegion(full, reg, core.WordInfo{ArgCount: -1}, 0) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.run()
			if n := es.PendingRegionCount(); n != 0 {
				t.Fatalf("captured %d regions, want 0", n)
			}
		})
	}
}

// TestTakePendingRegionMissIsOrdinary pins the FIRST measured asymmetry: not
// every recorded call has a region. `concat "a" "b"` never reaches forward
// collection, so Phase B finds no capture — and that is correct, not an error,
// because a region IS a forward-collecting dispatch.
func TestTakePendingRegionMissIsOrdinary(t *testing.T) {
	es := NewEmitState()
	if d, ok := es.TakePendingRegion("concat", core.SrcPos{Row: 1, Col: 1}); ok || d != nil {
		t.Fatal("a word that never forward-collected must report no capture, not a zero descriptor")
	}
	if d, ok := (*EmitState)(nil).TakePendingRegion("x", core.SrcPos{}); ok || d != nil {
		t.Fatal("a nil state must report no capture rather than panicking")
	}
}

// TestRecaptureIsIdempotent pins the loop case: the same source position
// dispatches on every iteration. RegionDesc is STATIC — "true of every
// execution of the region" — so the extent does not vary and overwriting is
// correct rather than merely tolerated.
func TestRecaptureIsIdempotent(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	defer done()
	// AFTER Begin: the check run installs its own recorder, so assigning
	// before it would be overwritten and the seam would silently decline.
	es := NewEmitState()
	reg.Check.Emit = es

	word := core.WithPosAt(core.NewWord("pair"), core.SrcPos{Row: 2, Col: 5})
	win := regionTape(word, core.NewInteger(1), core.NewInteger(2))
	w := core.WordInfo{Name: "pair", ArgCount: -1}

	tryRecordRegion(win, reg, w, 0)
	first, _ := es.TakePendingRegion("pair", core.SrcPos{Row: 2, Col: 5})
	firstLen := len(first.Slots)

	tryRecordRegion(win, reg, w, 0)
	if n := es.PendingRegionCount(); n != 1 {
		t.Fatalf("re-capture at one position made %d entries, want 1", n)
	}
	again, _ := es.TakePendingRegion("pair", core.SrcPos{Row: 2, Col: 5})
	if len(again.Slots) != firstLen {
		t.Fatalf("re-capture changed the extent: %d -> %d slots", firstLen, len(again.Slots))
	}
}

// TestJoinKeyIgnoresSourceText pins the flaw that a count-only assertion
// missed. core.SrcPos carries a third field beyond Row and Col — Src, the
// token's source TEXT — so keying the join on a whole SrcPos makes it depend
// on how a position was BUILT rather than on where it points.
//
// The failure mode is what makes this worth a test rather than a comment: a
// missed join is indistinguishable from "this call had no region", which is a
// documented and EXPECTED outcome for any stack-only dispatch. The bug would
// have hidden inside a known-good asymmetry instead of surfacing as one.
func TestJoinKeyIgnoresSourceText(t *testing.T) {
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	defer done()
	es := NewEmitState()
	reg.Check.Emit = es

	// Capture with a position that carries source text, as a real tape token
	// does.
	withText := core.SrcPos{Row: 4, Col: 9, Src: "pair"}
	word := core.WithPosAt(core.NewWord("pair"), withText)
	// A take CLAIMS the offer, so each probe of the key gets its own capture —
	// the same state a real re-execution leaves, and the reason the subject
	// here stays the key rather than the lifetime.
	recapture := func() {
		tryRecordRegion(regionTape(word, core.NewInteger(1)), reg,
			core.WordInfo{Name: "pair", ArgCount: -1}, 0)
	}

	// Look it up with the SAME LOCATION but no source text — the shape a
	// caller that reconstructed a position would have.
	recapture()
	if _, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 4, Col: 9}); !ok {
		t.Fatal("the join must key on location alone; Src must not participate")
	}
	// And with different text at the same location.
	recapture()
	if _, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 4, Col: 9, Src: "other"}); !ok {
		t.Fatal("differing Src at one location must still join")
	}
	// A genuinely different location must still miss.
	recapture()
	if _, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 4, Col: 10}); ok {
		t.Fatal("a different column is a different region and must not join")
	}
	// The claim is what makes a miss safe: after taking, the offer is gone, so
	// a record with no capture of its own cannot inherit an earlier one.
	if _, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 4, Col: 9}); !ok {
		t.Fatal("the last recapture must still be claimable")
	}
	if _, ok := es.TakePendingRegion("pair", core.SrcPos{Row: 4, Col: 9}); ok {
		t.Fatal("a claimed capture must not be offered twice")
	}
}

// TestPendingRegionCountOnNil covers the nil receiver. It is not ceremony:
// the count is read from test helpers and from Phase B's eventual drain, and
// a nil EmitState is the ordinary state on any path where recording never
// armed — a compiler-less build, a suspended pass. Returning 0 rather than
// panicking is the contract.
func TestPendingRegionCountOnNil(t *testing.T) {
	if n := (*EmitState)(nil).PendingRegionCount(); n != 0 {
		t.Fatalf("nil EmitState reported %d pending regions, want 0", n)
	}
	if n := NewEmitState().PendingRegionCount(); n != 0 {
		t.Fatalf("a fresh EmitState reported %d pending regions, want 0", n)
	}
}
