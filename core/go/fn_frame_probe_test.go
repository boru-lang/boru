package core

import "testing"

// Unit battery for the tail-call detection probe on synthetic tapes.
// Each accepting shape is paired with the rejecting shapes that differ
// from it by one token class — the probe is default-deny, so the
// negative cases are the contract (design/legacy/TCO-STAGED.10.ignore Stage 2).

type probeFixture struct {
	r    *Registry
	meta *FnFrameMeta
	dc   Value
	rc   Value
	und  Value // force-forward undef word, as AppendFrameTail emits
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
		dc:   NewDefCleanup(DefCleanupInfo{Snapshot: map[string]int{}, Registry: r}),
		rc:   NewReturnCheck(ReturnCheckInfo{FuncName: "f", Returns: []*Type{TInteger}}),
		und:  NewWordModified("undef", -1, false, true),
	}
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
	// (ₘ 1 f __DC __pa undef n __RC )
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
		f.dc, NewWord("__pa"), f.und, NewWord("n"), f.rc, NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 2, []int{1}, 1)
	if !ok {
		t.Fatal("canonical tail shape not detected")
	}
	if scan.Meta != f.meta {
		t.Error("scan did not surface the frame's meta pointer")
	}
	if scan.FrameOpen != 0 || scan.TailStart != 3 || scan.RCIdx != 7 || scan.CloseIdx != 8 {
		t.Errorf("scan extent wrong: %+v", scan)
	}
	if len(scan.UndefNames) != 1 || scan.UndefNames[0] != "n" {
		t.Errorf("UndefNames = %v, want [n]", scan.UndefNames)
	}
	if scan.ValuesBelow {
		t.Error("ValuesBelow set with nothing below the call")
	}
}

func TestProbeTailCallThroughGroupCloser(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ ( 1 f ) __DC __pa )   — the if-branch shape: the call sits in
	// a group whose closer precedes the frame tail.
	tokens := []Value{
		NewFrameOpen(f.meta), NewOpenParen(), NewInteger(1), NewWord("f"),
		NewCloseParen(), f.dc, NewWord("__pa"), NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 3, []int{2}, 1)
	if !ok {
		t.Fatal("tail call through a group closer not detected")
	}
	if scan.RCIdx != -1 {
		t.Errorf("RCIdx = %d, want -1 (no declared returns)", scan.RCIdx)
	}
	if scan.CloseIdx != 7 || scan.FrameOpen != 0 {
		t.Errorf("scan extent wrong: %+v", scan)
	}
}

func TestProbeTailCallValuesBelowShellAccept(t *testing.T) {
	f := newProbeFixture(t)
	// (ₘ 9 1 f __DC __pa )   — a value parked below the call is inert
	// for a shell teardown; the scan accepts and flags it.
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(9), NewInteger(1), NewWord("f"),
		f.dc, NewWord("__pa"), NewCloseParen(),
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
	pa := NewWord("__pa")

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
				NewCloseParen(), f.dc, pa, NewCloseParen(),
			},
			pointer: 4, indices: []int{3}, n: 1,
		},
		{
			name: "pending token between call and tail",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"), NewInteger(9),
				f.dc, pa, NewCloseParen(),
			},
			pointer: 1, n: 0,
		},
		{
			name: "no enclosing frame (plain group only)",
			tokens: []Value{
				NewOpenParen(), NewInteger(1), NewWord("f"),
				f.dc, pa, NewCloseParen(),
			},
			pointer: 2, indices: []int{1}, n: 1,
		},
		{
			name: "top level (tape start, no frame)",
			tokens: []Value{
				NewInteger(1), NewWord("f"), f.dc, pa, NewCloseParen(),
			},
			pointer: 1, indices: []int{0}, n: 1,
		},
		{
			name: "paren imbalance between the halves",
			tokens: []Value{
				NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
				NewCloseParen(), f.dc, pa, NewCloseParen(),
			},
			pointer: 2, indices: []int{1}, n: 1,
		},
		{
			name: "malformed undef pair",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"),
				f.dc, pa, f.und, f.rc, NewCloseParen(),
			},
			pointer: 1, n: 0,
		},
		{
			name: "mark below",
			tokens: []Value{
				NewFrameOpen(f.meta), NewMark("m1"), NewWord("f"),
				f.dc, pa, NewCloseParen(),
			},
			pointer: 2, n: 0,
		},
		{
			name: "carrier below (check-mode shape)",
			tokens: []Value{
				NewFrameOpen(f.meta), NewCarrier(TInteger), NewWord("f"),
				f.dc, pa, NewCloseParen(),
			},
			pointer: 2, n: 0,
		},
		{
			name: "non-contiguous call region",
			tokens: []Value{
				NewFrameOpen(f.meta), NewInteger(1), NewInteger(2), NewWord("f"),
				f.dc, pa, NewCloseParen(),
			},
			pointer: 3, indices: []int{1}, n: 1, // arg at 1, but 2 intervenes
		},
		{
			name: "no cleanup tail ahead (mid-body call)",
			tokens: []Value{
				NewFrameOpen(f.meta), NewWord("f"), NewWord("add"),
				f.dc, pa, NewCloseParen(),
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
