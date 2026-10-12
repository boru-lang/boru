package check

// Gate test for the compaction arm of spliceFnCheckTail
// (check_recovery.go:477 — `if !skipSet[i] {`).
//
// spliceFnCheckTail is the shared stack discipline behind the two check-mode
// fn-VALUE dispatches (spliceAnonCheckResult and SpliceFnValueCheckResult): it
// removes the consumed args plus the fn-value literal from the tape and splices
// the residual carriers in their place.
//
// Its first branch handles the normal case — all nArgs args were resolved
// before the pointer. It walks the whole tape window [firstArgIdx, valIdx] and
// keeps only what is NOT consumed:
//
//	skipSet = {the nArgs resolved arg indices} ∪ {valIdx}
//
// Reaching the loop is easy; ENTERING the body requires the window to contain a
// HOLE — an index that is neither a resolved arg nor the fn-value literal. That
// can only happen when a non-value token sits BETWEEN the first arg and the fn
// value, because ResolvedIndicesBefore (core/go/engine.go:3646) walks down from
// the pointer and `continue`s past Forward / Mark / Move tokens without
// collecting their indices. So the arm's precondition is:
//
//   - len(indices) == nArgs && nArgs > 0 — the first branch is taken, and
//   - at least one Forward/Mark/Move token is interleaved among the args,
//     i.e. valIdx - firstArgIdx + 1 > nArgs + 1.
//
// With a contiguous arg run (the common shape) the window is exactly the args
// plus the fn value, every index is in skipSet, and the body never runs — which
// is why the arm went uncovered when the interleaved-marker tests moved out of
// this module during the split. The body exists so those interleaved control
// tokens are COMPACTED DOWN to firstArgIdx and survive the splice rather than
// being swallowed with the args.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestGaterecovery477InterleavedMarkerIsCompacted drives the arm: a Mark token
// sits between the two args, so ResolvedIndicesBefore returns the
// NON-CONTIGUOUS pair [0, 2] and index 1 is a hole in skipSet. The arm must
// slide the Mark down to firstArgIdx so it outlives the splice.
func TestGaterecovery477InterleavedMarkerIsCompacted(t *testing.T) {
	// tape: [ arg0=1 | MARK "keep" | arg1=2 | fn-value literal ]
	//         idx 0     idx 1        idx 2    idx 3 = valIdx
	e := engWithTape(t, []core.Value{
		core.NewInteger(1),
		core.NewMark("keep"),
		core.NewInteger(2),
		core.NewString("<fn value>"),
	}, 3)

	// Precondition: the resolved args must straddle the Mark, or the window
	// has no hole and the arm is skipped entirely.
	indices := e.ResolvedIndicesBefore(2)
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Fatalf("precondition: resolved arg indices = %v, want [0 2] (Mark at 1 not collected)", indices)
	}

	result := core.NewCarrier(core.TInteger)
	spliceFnCheckTail(e, 3, 2, []core.Value{result})

	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0 (firstArgIdx)", e.Pointer)
	}
	if got := e.Tape.Len(); got != 2 {
		t.Fatalf("tape len = %d, want 2 (the compacted Mark + the residual carrier)", got)
	}
	if !core.IsMark(e.Tape.Ref(0)) {
		t.Errorf("tape[0] = %#v, want the interleaved Mark compacted to firstArgIdx", e.Tape.At(0))
	}
	if !e.Tape.At(1).Carrier {
		t.Errorf("tape[1] = %#v, want the spliced residual carrier", e.Tape.At(1))
	}
}

// TestGaterecovery477ContiguousArgsSkipEveryIndex pins the guard's false edge
// with the SAME branch and the SAME arg count: with no interleaved token the
// window is exactly {args} ∪ {valIdx}, every index is in skipSet, the loop body
// never runs and dst stays at firstArgIdx — so args and fn value are all
// consumed and only the residual remains.
func TestGaterecovery477ContiguousArgsSkipEveryIndex(t *testing.T) {
	// tape: [ arg0=1 | arg1=2 | fn-value literal ]
	e := engWithTape(t, []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
		core.NewString("<fn value>"),
	}, 2)

	indices := e.ResolvedIndicesBefore(2)
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 {
		t.Fatalf("precondition: resolved arg indices = %v, want [0 1] (contiguous)", indices)
	}

	spliceFnCheckTail(e, 2, 2, []core.Value{core.NewCarrier(core.TInteger)})

	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0", e.Pointer)
	}
	if got := e.Tape.Len(); got != 1 {
		t.Fatalf("tape len = %d, want 1 (nothing survives compaction)", got)
	}
	if !e.Tape.At(0).Carrier {
		t.Errorf("tape[0] = %#v, want the residual carrier alone", e.Tape.At(0))
	}
}
