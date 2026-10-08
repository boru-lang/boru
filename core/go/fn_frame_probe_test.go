package core

import "testing"

// Unit battery for the tail-call detection probe on synthetic tapes.
// Each accepting shape is paired with the rejecting shapes that differ
// from it by one token class — the probe is default-deny, so the
// negative cases are the contract (design/legacy/TCO-STAGED.10.ignore Stage 2).

type probeFixture struct {
	r    *Registry
	meta *FnFrameMeta
	dc   Value // a frame marker tearing down the param `n`, as AppendFrameTail emits
	rc   Value
}

func newProbeFixture(t *testing.T) probeFixture {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return probeFixture{
		r:    r,
		meta: &FnFrameMeta{Name: "f"},
		dc:   frameMarker(r, map[string]int{}, "n"),
		rc:   NewReturnCheck(ReturnCheckInfo{FuncName: "f", Returns: []*Type{TInteger}}),
	}
}

// frameMarker builds a frame's DefCleanup marker as AppendFrameTail does:
// the snapshot, the frame pop, and the captures+params to tear down.
func frameMarker(r *Registry, snapshot map[string]int, names ...string) Value {
	return NewDefCleanup(DefCleanupInfo{Snapshot: snapshot, Registry: r, PopFrame: true, Names: names})
}

func (f probeFixture) probe(t *testing.T, tokens []Value, pointer int, indices []int, n int) (frameTailScan, bool) {
	t.Helper()
	e := NewTop(f.r)
	e.Tape = NewTape(tokens, 8)
	e.Pointer = pointer
	return e.probeTailCall(indices, n)
}

func TestProbeTailCallCanonical(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ 1 f __DC __RC ) — the marker tears down the param n
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
		f.dc, f.rc, NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 2, []int{1}, 1)
	if !ok {
		t.Fatal("canonical tail shape not detected")
	}
	if scan.Meta != f.meta {
		t.Error("scan did not surface the frame's meta pointer")
	}
	if scan.FrameOpen != 0 || scan.TailStart != 3 || scan.RCIdx != 4 || scan.CloseIdx != 5 {
		t.Errorf("scan extent wrong: %+v", scan)
	}
	if len(scan.Names) != 1 || scan.Names[0] != "n" {
		t.Errorf("Names = %v, want [n]", scan.Names)
	}
	if scan.ValuesBelow {
		t.Error("ValuesBelow set with nothing below the call")
	}
}

func TestProbeTailCallThroughGroupCloser(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ ( 1 f ) __DC )   — the if-branch shape: the call sits in
	// a group whose closer precedes the frame tail.
	tokens := []Value{
		NewFrameOpen(f.meta), NewOpenParen(), NewInteger(1), NewWord("f"),
		NewCloseParen(), f.dc, NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 3, []int{2}, 1)
	if !ok {
		t.Fatal("tail call through a group closer not detected")
	}
	if scan.RCIdx != -1 {
		t.Errorf("RCIdx = %d, want -1 (no declared returns)", scan.RCIdx)
	}
	if scan.CloseIdx != 6 || scan.FrameOpen != 0 {
		t.Errorf("scan extent wrong: %+v", scan)
	}
}

func TestProbeTailCallValuesBelowShellAccept(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ 9 1 f __DC )   — a value parked below the call is inert
	// for a shell teardown; the scan accepts and flags it.
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(9), NewInteger(1), NewWord("f"),
		f.dc, NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 3, []int{2}, 1)
	if !ok {
		t.Fatal("value-below shape not detected")
	}
	if !scan.ValuesBelow {
		t.Error("ValuesBelow not flagged")
	}
}

func TestProbeTailCallRejects(t *testing.T) {
	f := newProbeFixture(t)
	// A truncation-only marker (TruncateFrameDefs' shape): not a frame tail.
	bareDC := NewDefCleanup(DefCleanupInfo{Snapshot: map[string]int{}, Registry: f.r})

	cases := []struct {
		name    string
		tokens  []Value
		pointer int
		indices []int
		n       int
	}{
		{
			// The classic false positive: `n add (f …)` parks an add
			// Forward below the group; the forward half alone would
			// match.
			name: "forward parked below",
			tokens: []Value{
				NewFrameOpen(f.meta), NewForward(ForwardInfo{FuncName: "add", ExpectedArgs: 2}),
				NewOpenParen(), NewInteger(1), NewWord("f"),
				NewCloseParen(), f.dc, NewCloseParen(),
			},
			pointer: 4, indices: []int{3}, n: 1,
		},
		{
			name: "pending token between call and tail",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"), NewInteger(9),
				f.dc, NewCloseParen(),
			},
			pointer: 1, n: 0,
		},
		{
			name: "no enclosing frame (plain group only)",
			tokens: []Value{
				NewOpenParen(), NewInteger(1), NewWord("f"),
				f.dc, NewCloseParen(),
			},
			pointer: 2, indices: []int{1}, n: 1,
		},
		{
			name: "top level (tape start, no frame)",
			tokens: []Value{
				NewInteger(1), NewWord("f"), f.dc, NewCloseParen(),
			},
			pointer: 1, indices: []int{0}, n: 1,
		},
		{
			name: "paren imbalance between the halves",
			tokens: []Value{
				NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
				NewCloseParen(), f.dc, NewCloseParen(),
			},
			pointer: 2, indices: []int{1}, n: 1,
		},
		{
			name: "truncation-only marker (closes no frame)",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"),
				bareDC, f.rc, NewCloseParen(),
			},
			pointer: 1, n: 0,
		},
		{
			name: "mark below",
			tokens: []Value{
				NewFrameOpen(f.meta), NewMark("m1"), NewWord("f"),
				f.dc, NewCloseParen(),
			},
			pointer: 2, n: 0,
		},
		{
			name: "carrier below (check-mode shape)",
			tokens: []Value{
				NewFrameOpen(f.meta), NewCarrier(TInteger), NewWord("f"),
				f.dc, NewCloseParen(),
			},
			pointer: 2, n: 0,
		},
		{
			name: "non-contiguous call region",
			tokens: []Value{
				NewFrameOpen(f.meta), NewInteger(1), NewInteger(2), NewWord("f"),
				f.dc, NewCloseParen(),
			},
			pointer: 3, indices: []int{1}, n: 1, // arg at 1, but 2 intervenes
		},
		{
			name: "no cleanup tail ahead (mid-body call)",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"), NewWord("add"),
				f.dc, NewCloseParen(),
			},
			pointer: 1, n: 0,
		},
	}
	for _, tc := range cases {
		if _, ok := f.probe(t, tc.tokens, tc.pointer, tc.indices, tc.n); ok {
			t.Errorf("%s: probe accepted, must decline", tc.name)
		}
	}
}
