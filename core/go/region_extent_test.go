package core

import "testing"

// The one-forward-per-scope invariant, and the `end` that enforces it.
//
// design/FULL-COMPILATION.0.md §6.2 bounds a compiled REGION by hard
// delimiters (`end`, `)`, EOF) and then relies on the region descriptor
// carrying the state that picks the stranded-forward raise text. Those two
// do not compose for free: strandedForwardError selects its Forward by
// scanning BACKWARD from the pointer and stops at an OpenParen — `end` is
// not a stop condition — so a Forward parked in an EARLIER region would be
// reachable from inside this one, and a region-bounded descriptor could not
// carry it.
//
// What closes the gap is that stepEnd runs the SAME backward scan and
// removes what it finds, so nothing survives the delimiter to be found.
// These pin that, because Stage 4's OpCollect has to reproduce the text
// SELECTION and not merely the raise.

// A parked forward strands at a barrier word — the first of the two texts.
func TestRegionParkedForwardStrandsBeforeEnd(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, []Value{fwdMarker("g", 0, 1, 0)}, 1)
	if err := e.strandedForwardError("def"); err == nil {
		t.Fatal("a parked forward must strand when a function word begins its dispatch")
	}
}

// …and stepEnd drains it, so the same probe inside the NEXT region finds
// nothing and the ordinary no-match raise fires instead. This is the whole
// reason an `end`-bounded descriptor is sufficient.
func TestRegionEndDrainsForwardSoNextRegionFindsNone(t *testing.T) {
	setStrict(t, true)
	// [ forward(g) , end ] with the pointer on the end.
	e := engWithTape(t, []Value{fwdMarker("g", 0, 0, 0), NewWord("end")}, 1)
	if err := e.stepEnd(); err != nil {
		t.Fatalf("stepEnd: %v", err)
	}
	for i := 0; i < e.Tape.Len(); i++ {
		if IsForward(e.Tape.Ref(i)) {
			t.Fatalf("stepEnd must drain the parked forward; found one at %d", i)
		}
	}
	e.Pointer = e.Tape.Len()
	if err := e.strandedForwardError("def"); err != nil {
		t.Fatalf("after the end drained it there is nothing to strand, got %v", err)
	}
}

// The scan stops at an OpenParen, so a forward parked OUTSIDE the current
// paren scope is already invisible — the property the region bound leans on
// for the paren case, as `end` does for the statement case.
func TestRegionParenScopeHidesOuterForward(t *testing.T) {
	setStrict(t, true)
	e := engWithTape(t, []Value{fwdMarker("g", 0, 1, 0), NewOpenParen()}, 2)
	if err := e.strandedForwardError("def"); err != nil {
		t.Fatalf("a forward below an OpenParen is out of scope, got %v", err)
	}
}

// --- RegionEnd: the syntactic bound a descriptor spans -------------------
