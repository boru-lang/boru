package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached default-deny arms in the tail-call machinery
// (fn_frame_probe.go, fn_frame_elide.go, fn_frame.go). Each is driven by
// a hand-built Engine with a crafted tape and pointer — the probe/elide
// helpers are pure structural scans over that state. Per
// design/TEST-SEAMS.10.md.

import "testing"

func w8dc(r *Registry) Value { return NewDefCleanup(DefCleanupInfo{Registry: r, PopFrame: true}) }

// --- fn_frame_probe.go ------------------------------------------------------

func TestW8ProbeTailCallArgCountMismatch(t *testing.T) {
	e := New(w8reg(t))
	if _, ok := e.probeTailCall([]int{}, 2); ok {
		t.Fatal("mismatched sortedIndices length vs n must decline")
	}
}

func TestW8ProbeTailCallNegativeStart(t *testing.T) {
	e := New(w8reg(t))
	e.Pointer = -1
	if _, ok := e.probeTailCall(nil, 0); ok {
		t.Fatal("a negative start index must decline")
	}
}

func TestW8ProbeTailCallForwardArms(t *testing.T) {
	r := w8reg(t)
	cases := []struct {
		name string
		tape []Value
	}{
		{"dc-at-tape-end", []Value{NewWord("f"), w8dc(r)}},
		{"truncation-only-marker", []Value{NewWord("f"), NewDefCleanup(DefCleanupInfo{Registry: r}), NewCloseParen()}},
		{"no-close-after-tail", []Value{NewWord("f"), w8dc(r), NewInteger(1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := New(r)
			e.Tape = NewTape(c.tape, 4)
			e.Pointer = 0
			if _, ok := e.probeTailCall(nil, 0); ok {
				t.Fatalf("%s: expected the probe to decline", c.name)
			}
		})
	}
}

// --- fn_frame_elide.go ------------------------------------------------------

func TestW8TcoEligibleNilMeta(t *testing.T) {
	e := New(w8reg(t))
	if e.tcoEligible(frameTailScan{Meta: nil}, &Signature{}, 0) {
		t.Fatal("a scan with no frame meta must decline")
	}
}

func TestW8TcoEligibleNonDefCleanupTail(t *testing.T) {
	r := w8reg(t)
	e := New(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, 4)
	sig := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{}}}
	scan := frameTailScan{Meta: &FnFrameMeta{}, TailStart: 0, RCIdx: -1}
	if e.tcoEligible(scan, sig, r.Defs.Mutations()) {
		t.Fatal("a tail whose start is not a DefCleanup must decline")
	}
}

func TestW8ReturnsConformBadReturnCheck(t *testing.T) {
	r := w8reg(t)
	e := New(r)
	e.Tape = NewTape([]Value{NewInteger(1)}, 4)
	scan := frameTailScan{RCIdx: 0}
	if e.returnsConform(scan, &Signature{Returns: []*Type{TInteger}}) {
		t.Fatal("a non-ReturnCheck at RCIdx must decline")
	}
}

func TestW8ElideTailFramePopError(t *testing.T) {
	r := w8reg(t)
	r.Args = nil // make PopFrameArgs (in teardownFrameState) error
	e := New(r)
	e.Tape = NewTape([]Value{w8dc(r)}, 4)
	scan := frameTailScan{TailStart: 0, RCIdx: -1, CloseIdx: 0}
	if err := e.elideTailFrame(scan); err == nil {
		t.Fatal("a frame teardown that fails to pop args must surface the error")
	}
}

// --- fn_frame.go ------------------------------------------------------------

func TestW8PopFrameArgsNilStack(t *testing.T) {
	r := w8reg(t)
	r.Args = nil
	if err := PopFrameArgs(r); err == nil {
		t.Fatal("popping a nil args stack must error")
	}
}
