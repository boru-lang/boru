package compiler

import core "github.com/boru-lang/boru/core/go"

// Region capture, PHASE A — the compiler's seat on core's RegionRecorder seam
// (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// A region descriptor is assembled from two places, because its two halves are
// known at different times and neither site can see the other's:
//
//   - PHASE A, here, at collection time: the region's EXTENT and its
//     written-order Tokens. Both are properties of the tape, and the tape is
//     only in scope during collection. No operands exist yet.
//   - PHASE B, at RecordCall: SlotDesc.Source — which values are interned
//     consts, frame locals, prior events, compiled fragments. That is the
//     recorder's own operand model, and no tape index is in scope there.
//
// They are joined on (word, SrcPos), which both sides already hold — the
// dispatching word's own position. That was measured across the corpus with a
// two-phase probe rather than assumed, and the same measurement produced the
// two asymmetries this file must not get wrong:
//
//   - NOT EVERY RECORDED CALL HAS A REGION. `concat "a" "b"` never reaches
//     forward collection at all, so Phase B will find no capture for it. That
//     is correct: a region IS a forward-collecting dispatch, and a word that
//     took its operands from the stack has none.
//   - NOT EVERY REGION BECOMES A RECORDED CALL. `def` captures here and is
//     never recorded as a call, being a check-mode word. Its capture is simply
//     never claimed.
//
// So the map is a pool of OFFERS, not a ledger that must balance. Code that
// assumed a 1:1 pairing would be wrong in both directions.
func init() {
	core.RegionRecorder = tryRecordRegion
}

// regionKey is the (word, LOCATION) join between the two phases.
//
// The location is Row and Col ONLY — deliberately not a whole core.SrcPos.
// SrcPos carries a third field, Src, the source TEXT of the token, and
// including it would make the key depend on how a position was constructed
// rather than on where it points. Two positions naming the same place but
// built by different paths would then miss each other.
//
// That failure would have been near-invisible, which is why this is spelled
// out: a missed join looks exactly like "this call had no region", and that
// is a documented, EXPECTED outcome (a stack-only dispatch has no region).
// The bug would have hidden inside the asymmetry rather than surfacing as
// one. Caught by an end-to-end test asserting a specific capture was
// reachable, which a count-only assertion would have passed.
type regionKey struct {
	word string
	row  int
	col  int
}

// pendingRegion is one Phase-A offer: the capture, plus the REGISTRY the
// dispatch was collecting against.
//
// The registry is carried here and nowhere else. Completion has to resolve a
// word slot against the def table the DISPATCH read, and es.reg is the last
// registry BindRegistry saw — after a call into a boru-implemented module it
// is that module's sub-registry, so a later main-registry dispatch would look
// its words up in the wrong table (measured: `M.m 5 end add x 2` recorded
// `add` with NFwd 0 because `x` was sought in M's registry). It must NOT
// reach RegionDesc: a Program is shared and a run may be handed a DIFFERENT
// registry — ForkConcurrent gives each concurrent execution its own fork —
// which is why Finalize declines to stamp one onto ordinary fn units.
type pendingRegion struct {
	desc *RegionDesc
	reg  *core.Registry
}

func keyOf(word string, pos core.SrcPos) regionKey {
	return regionKey{word: word, row: pos.Row, col: pos.Col}
}

// tryRecordRegion captures the region a forward-collecting dispatch is about
// to walk. It READS the window and never evaluates — deciding what a dispatch
// claims is the collection walk's job, and later OpCollect's; recording what
// is THERE is this one's.
func tryRecordRegion(win core.CollectWindow, reg *core.Registry, w core.WordInfo, at int) {
	if reg == nil || win == nil || at < 0 || at >= win.Len() {
		return
	}
	es, _ := reg.Check.Recorder().(*EmitState)
	if es == nil || !es.Active() || es.SuspendedNow() {
		return
	}
	// Slots begin AFTER the dispatching word — the same start the collection
	// walk uses (resolveForwardArgs passes e.Pointer+1), so the extent
	// recorded is the extent walked.
	slots := CaptureRegionSlots(win, at+1)
	if slots == nil {
		return
	}
	if es.pendingRegions == nil {
		es.pendingRegions = map[regionKey]pendingRegion{}
	}
	pos := win.At(at).Pos()
	es.pendingRegions[keyOf(w.Name, pos)] = pendingRegion{
		desc: &RegionDesc{
			Lead:  LeadWord,
			Word:  w.Name,
			Slots: slots,
			Pos:   pos,
		},
		reg: reg,
	}
}

// PendingRegionCount reports how many Phase-A captures are held. It exists for
// tests: the captures are not observable any other way until Phase B completes
// them, and a capture step nothing can see is a capture step nothing can pin.
func (es *EmitState) PendingRegionCount() int {
	if es == nil {
		return 0
	}
	return len(es.pendingRegions)
}

// heldRegion is a Phase-A offer taken out of the pool EARLY — at the entry
// of a user-fn ReturnsFn, before the callee's body is analysed — and parked
// until that call's record completes it.
//
// It exists because the pool is keyed by (word, row, col) and NOT by source:
// SrcPos.Src is the token's TEXT, so two sources cannot be told apart by a
// position at all. A user call's record runs at the END of its ReturnsFn,
// after the callee's body has been analysed, and that body can dispatch the
// same word at the same row and column of ANOTHER source — a module file's
// recursive `f 0` at 2:1 under a main program's `Ns.f 1` at 2:1, pinned in
// lang/go/region_capture_e2e_test.go. With the claim deferred to the record,
// the inner capture overwrote the outer's offer and the inner record
// consumed it; the outer call then had no descriptor, which is exactly the
// near-invisible miss regionKey's doc warns about. Holding the offer at
// entry closes the window: nothing analysed inside the callee can reach an
// offer that is no longer in the pool.
//
// The holds are a STACK released in defer order, so nesting is exact: an
// inner call's hold sits above the outer's and is popped before the outer's
// record runs. A hold that found NO offer is still pushed, and it BLOCKS the
// record from falling back to the pool: by record time the pool may carry an
// inner call's re-offer under the same key, and a stack-fed dispatch must
// not claim a capture it never walked.
type heldRegion struct {
	key      regionKey
	off      pendingRegion
	ok       bool // an offer was in the pool at hold time
	consumed bool // the record has completed it
}

// HoldRegion takes the Phase-A offer for the dispatching word token out of
// the pool now and parks it for this call's record (core.EmitRecorder's
// contract). The release truncates the stack back to this hold's depth and
// must be deferred by the caller. A nil or inactive state returns a no-op.
func (es *EmitState) HoldRegion(word string, pos core.SrcPos) func() {
	if es == nil || !es.Active() {
		return func() {}
	}
	off, ok := es.takePendingRegion(word, pos)
	depth := len(es.heldRegions)
	es.heldRegions = append(es.heldRegions, heldRegion{key: keyOf(word, pos), off: off, ok: ok})
	return func() {
		if len(es.heldRegions) > depth {
			es.heldRegions = es.heldRegions[:depth]
		}
	}
}

// HeldRegionCount reports how many offers are parked under a hold. Tests
// only, for the same reason as PendingRegionCount.
func (es *EmitState) HeldRegionCount() int {
	if es == nil {
		return 0
	}
	return len(es.heldRegions)
}

// claimRegion hands completion the offer for (word, pos): the HELD offer when
// the record runs under a hold for that key — the innermost open hold is the
// top of the stack, and a user call's record runs inside its own ReturnsFn
// after every nested hold has released — else the pool's. A hold that found
// nothing yields nothing, deliberately (heldRegion); a hold completes once.
func (es *EmitState) claimRegion(word string, pos core.SrcPos) (pendingRegion, bool) {
	if n := len(es.heldRegions); n > 0 {
		h := &es.heldRegions[n-1]
		if h.key == keyOf(word, pos) {
			if h.consumed || !h.ok {
				return pendingRegion{}, false
			}
			h.consumed = true
			return h.off, true
		}
	}
	return es.takePendingRegion(word, pos)
}

// TakePendingRegion claims the Phase-A capture for (word, pos), reporting
// whether one existed. Phase B calls it to complete a descriptor; a miss is
// ordinary, not an error, per the asymmetries above.
//
// It REMOVES the capture, and that is a correctness property rather than
// tidiness. Phase A is offered before every collection, so a live dispatch
// always re-captures before it records — but the recorder can be SUSPENDED
// over a capture while the record still fires, and a non-removing take would
// then hand that record an offer from an EARLIER execution. A miss is safe
// (the dispatch simply carries no descriptor); a stale hit is a descriptor
// describing a tape the dispatch did not walk.
func (es *EmitState) TakePendingRegion(word string, pos core.SrcPos) (*RegionDesc, bool) {
	p, ok := es.takePendingRegion(word, pos)
	return p.desc, ok
}

// takePendingRegion is the same claim with the offer's registry attached —
// what completion needs and what the emitted descriptor must never carry.
func (es *EmitState) takePendingRegion(word string, pos core.SrcPos) (pendingRegion, bool) {
	if es == nil || es.pendingRegions == nil {
		return pendingRegion{}, false
	}
	k := keyOf(word, pos)
	p, ok := es.pendingRegions[k]
	if ok {
		delete(es.pendingRegions, k)
	}
	return p, ok
}
