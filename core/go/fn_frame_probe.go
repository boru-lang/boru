package core

// Tail-call detection (design/legacy/TCO-STAGED.10.ignore Stage 2). The probe is a
// read-only structural scan of the tape around a dispatching fn-body
// call; it answers "is this call in tail position of an enclosing fn
// frame whose tail the engine could run eagerly?". Detection only —
// the counter on Registry.TCO is the entire effect. Frame replacement
// acts on the scan in a later stage, gated by Registry.TCO.Disable.
//
// A call is a tail call iff, at dispatch time:
//
//   forward:  everything between the call region and the enclosing
//             frame's close paren is close-parens of groups opened
//             inside the frame, followed by exactly the frame's own
//             cleanup tail (__DC __pa undef-pairs [__RC]);
//   backward: everything between the frame's marked open paren and
//             the call region is inert — open parens of those same
//             groups and concrete values. Anything PENDING below
//             (a parked Forward, a Mark/Move, a word) means caller
//             code still runs after the callee returns, so the frame
//             must nest. The classic false positive the backward half
//             exists to reject: `n add (f …)` — the forward half sees
//             a perfect tail pattern but the parked add still consumes
//             the frame's result (and a parked def would re-install a
//             binding the eager teardown just removed).
//
// The two halves must also agree structurally: the close-parens
// accepted ahead must equal the open parens passed below, or the scan
// is looking at a shape it doesn't understand and declines.

// TCOState is the registry's tail-call-elimination surface.
type TCOState struct {
	// Disable turns frame elision/replacement off. DIAGNOSTIC ONLY
	// since Stage 6: tail-call elimination is a documented language
	// guarantee (REFERENCE.md "Recursion and tail calls"), so running
	// with Disable set changes the resource semantics programs are
	// entitled to rely on — deep tail chains will exhaust the tape.
	// Use it to bisect a suspected elision bug, never in production.
	// The detection probe ignores it, so Detected stays meaningful
	// either way.
	Disable bool
	// Detected counts fn-body dispatches found in tail position since
	// the registry was built. Detection-only telemetry: cheap,
	// read-only, always on (Disable does not affect it).
	Detected int64
	// Elided counts detected tail calls handled by the SHELL variant
	// (frame tail torn down eagerly, shell kept — the values-below
	// case). Replaced counts those handled by FULL frame replacement
	// (clean frame interior; zero residue). Elided+Replaced ≤ Detected;
	// the gap is the calls the eligibility gate declined.
	Elided   int64
	Replaced int64
}

// frameTailScan reports the innermost enclosing frame as found by
// probeTailCall: its identity, extent, and tail layout.
type frameTailScan struct {
	Meta      *FnFrameMeta // enclosing frame's overload identity
	FrameOpen int          // index of the frame's marked open paren
	TailStart int          // index of the frame's __DC marker
	RCIdx     int          // index of the frame's ReturnCheck; -1 when none
	CloseIdx  int          // index of the frame's close paren
	// UndefNames are the tail's undef-pair names in tail (reverse-
	// install) order — the captures+params an eager teardown undefs.
	UndefNames []string
	// ValuesBelow is true when concrete values sit between the frame
	// open and the call region. Inert for a teardown that leaves the
	// frame's shell in place; a full frame replacement must nest
	// instead (the values would be dropped).
	ValuesBelow bool
	// PendingEvalBelow is true when one of those values is a pending
	// (unevaluated) plain list/map — the shape an EvalResidual frame's
	// eager teardown would evaluate BEFORE the callee runs, where the
	// parked marker evaluates it after the callee returns. Such a
	// frame must nest (tcoEligible declines) so elision cannot reorder
	// the residual's side effects around the tail callee.
	PendingEvalBelow bool
}

// probeTailCall scans around the dispatch at e.pointer (an fn-body
// call whose matched arg positions are sortedIndices, n = arg count).
// It returns ok=false for any shape it does not positively recognise —
// default-deny throughout.
func (e *Engine) probeTailCall(sortedIndices []int, n int) (frameTailScan, bool) {
	scan := frameTailScan{RCIdx: -1}

	// The call region must be contiguous and end at the word: matched
	// args at pointer-n..pointer-1. Interleaved survivors (internal
	// forwards preserved by the splice compaction) mean tokens between
	// the args would outlive the call — decline rather than reason
	// about them.
	start := e.Pointer
	if n > 0 {
		if len(sortedIndices) != n {
			return scan, false
		}
		for k, idx := range sortedIndices {
			if idx != e.Pointer-n+k {
				return scan, false
			}
		}
		start = sortedIndices[0]
	}
	if start < 0 {
		return scan, false
	}

	// Forward half: )* __DC __pa (undef name)* [__RC] ) — anything
	// else, at any point, is not a tail.
	i := e.Pointer + 1
	closersAhead := 0
	for i < e.Tape.Len() && IsCloseParen(e.Tape.At(i)) {
		closersAhead++
		i++
	}
	if i >= e.Tape.Len() || !IsDefCleanup(e.Tape.At(i)) {
		return scan, false
	}
	scan.TailStart = i
	i++
	if i >= e.Tape.Len() {
		return scan, false
	}
	if w, err := AsWord(e.Tape.At(i)); err != nil || w.Name != "__pa" {
		return scan, false
	}
	i++
	for i+1 < e.Tape.Len() {
		u, err := AsWord(e.Tape.At(i))
		if err != nil || u.Name != "undef" || !u.ForceForward {
			break
		}
		nm, err := AsWord(e.Tape.At(i + 1))
		if err != nil {
			return scan, false
		}
		scan.UndefNames = append(scan.UndefNames, nm.Name)
		i += 2
	}
	if i < e.Tape.Len() && IsReturnCheck(e.Tape.At(i)) {
		scan.RCIdx = i
		i++
	}
	if i >= e.Tape.Len() || !IsCloseParen(e.Tape.At(i)) {
		return scan, false
	}
	scan.CloseIdx = i

	// Backward half: from below the call region down to the frame's
	// marked open paren, accepting only the open parens of enclosed
	// groups and inert concrete values. Everything pending — Forwards,
	// Marks, Moves, words, markers, carriers — declines.
	opensBelow := 0
	for j := start - 1; j >= 0; j-- {
		v := e.Tape.At(j)
		switch {
		case IsFrameOpen(v):
			if opensBelow != closersAhead {
				return scan, false
			}
			info, err := AsFrameOpen(v)
			if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
				return scan, false
			}
			scan.FrameOpen = j
			scan.Meta = info.Meta
			return scan, true
		case IsOpenParen(v):
			opensBelow++
		case IsForward(v), IsWord(v), IsMark(v), IsMove(v),
			IsCloseParen(v), IsEnd(v), IsDefCleanup(v),
			IsReturnCheck(v), IsSplice(v):
			return scan, false
		case IsConcrete(v):
			scan.ValuesBelow = true
			if isPendingResidualContainer(v) {
				scan.PendingEvalBelow = true
			}
		default:
			// Type literals, carriers, and anything not positively
			// classified: decline.
			return scan, false
		}
	}
	// Reached the tape start without a frame open: top-level call, no
	// enclosing frame.
	return scan, false
}
