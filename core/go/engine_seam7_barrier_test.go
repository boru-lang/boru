package core

// Wave-7 coverage, part 6: commitBarrierForward's probe/guard arms,
// reached by constructing the exact pending-forward tape state the
// statement-boundary commit scans. See design/TEST-SEAMS.10.md.

import "testing"

func fwdMarker(name string, funcIdx, collected, stack int) Value {
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	return NewForward(ForwardInfo{
		FuncName: name, Sig: sig, FuncIndex: funcIdx,
		CollectedArgs: collected, StackArgs: stack,
	})
}

func TestS7CommitBarrierNoPendingForward(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1)}, 1)
	if e.commitBarrierForward() {
		t.Error("no pending forward → false")
	}
}

func TestS7CommitBarrierNothingClaimed(t *testing.T) {
	// A forward that has claimed nothing yet → false.
	e := engWithTape(t, []Value{fwdMarker("cadd", 0, 0, 0), NewInteger(1)}, 2)
	if e.commitBarrierForward() {
		t.Error("claimed==0 → false")
	}
}

func TestS7CommitBarrierFuncIdxOutOfRange(t *testing.T) {
	// funcIdx points past the tape → guard returns false.
	e := engWithTape(t, []Value{NewInteger(1), fwdMarker("cadd", 99, 1, 0)}, 2)
	if e.commitBarrierForward() {
		t.Error("out-of-range funcIdx → false")
	}
}

func TestS7CommitBarrierUnregisteredWord(t *testing.T) {
	// funcIdx names a word with no registered fn → false.
	e := engWithTape(t, []Value{NewWord("nope"), fwdMarker("nope", 0, 1, 0)}, 2)
	if e.commitBarrierForward() {
		t.Error("unregistered word → false")
	}
}

func TestS7CommitBarrierStartNegative(t *testing.T) {
	// start = funcIdx - claimed < 0 → false.
	e := engWithTape(t, []Value{NewWord("cadd"), fwdMarker("cadd", 0, 1, 0)}, 2)
	if e.commitBarrierForward() {
		t.Error("start<0 → false")
	}
}

func TestS7CommitBarrierAllMarkersEmptyResolved(t *testing.T) {
	// The claimed region is entirely markers → resolved empty → false.
	e := engWithTape(t, []Value{NewMark("m"), NewWord("cadd"), fwdMarker("cadd", 1, 1, 0)}, 3)
	if e.commitBarrierForward() {
		t.Error("all-marker claimed region → false")
	}
}

func TestS7CommitBarrierSkipsMarkerInResolved(t *testing.T) {
	// A marker inside the claimed region is skipped; the remaining single
	// value can't fill cadd's 2-arg sig → no commit, but the skip arm runs.
	e := engWithTape(t, []Value{NewMark("m"), NewInteger(5), NewWord("cadd"), fwdMarker("cadd", 2, 2, 0)}, 4)
	if e.commitBarrierForward() {
		t.Error("under-filled sig → false (but marker-skip arm exercised)")
	}
}

func TestS7CommitBarrierSuccessfulCommit(t *testing.T) {
	// A forward BEFORE its func word, with exactly two claimed Integer
	// args matching cadd's 2-arg overload → committed (funcIdx-- arm).
	e := engWithTape(t, []Value{
		fwdMarker("cadd", 3, 2, 0), NewInteger(3), NewInteger(4), NewWord("cadd"),
	}, 4)
	if !e.commitBarrierForward() {
		t.Fatal("a fully-claimed 2-arg forward should commit")
	}
	// The forward marker was removed and the word force-stacked.
	for i := 0; i < e.Tape.Len(); i++ {
		if IsForward(e.Tape.Ref(i)) {
			t.Error("forward marker should have been removed on commit")
		}
	}
}
