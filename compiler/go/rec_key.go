package compiler

import (
	"sync/atomic"

	core "github.com/boru-lang/boru/core/go"
)

// Recorder keys (design/VALUE-NO-COPY.0.md §5; exceptions D4). A recorded
// value's producer is the (event, result index) this pass stamped on the
// holder the event's results flowed on (core.SetRecKey), read back from
// whichever copy of that holder reaches a later event (core.RecKeyOf). It
// is not looked up through the value's identity token, so two holders of
// one value produced by two events — a word that hands its argument back,
// a deterministic read made twice — answer each for its own event, and an
// argument the event read keeps the producer it had.
//
// A stamp is valid while the event it names stands in this pass: the pass
// that stamped it (Pass) and the generation of the event number (Gen). A
// rolled-back round's numbers, a retracted event's and a finished unit's
// own events' are retired (their generation moves on), and only a rollback
// reissues numbers — after retiring them — so no stamp outlives the event
// it names.

// recPasses numbers recording passes for their recorder keys.
var recPasses atomic.Uint32

// passNo is this pass's number, assigned on first use.
func (es *EmitState) passNo() uint32 {
	if es.recPass == 0 {
		es.recPass = recPasses.Add(1)
	}
	return es.recPass
}

// retireSeqs makes every stamp of the events numbered from..to (inclusive)
// resolve to nothing: a rolled-back round's (whose numbers the next round
// reissues under the new generation), a retracted event's, a finished
// unit's own.
func (es *EmitState) retireSeqs(from, to int) {
	for s := max(from, 0); s <= to; s++ {
		for len(es.seqGen) <= s {
			es.seqGen = append(es.seqGen, 0)
		}
		es.seqGen[s]++
	}
}

// genOf is the current generation of event number seq (0 until it is first
// retired).
func (es *EmitState) genOf(seq int) uint32 {
	if seq < 0 || seq >= len(es.seqGen) {
		return 0
	}
	return es.seqGen[seq]
}

// stampProduced stamps the holder out as result idx of event seq.
func (es *EmitState) stampProduced(out *core.Value, seq, idx int) {
	core.SetRecKey(out, core.RecKey{Pass: es.passNo(), Gen: es.genOf(seq), Seq: int32(seq), Idx: int32(idx)})
}

// producerOf is the event result v is in this pass: the recorder key its
// holder carries, while the event it names stands. A result a fired splice
// took out of its event (NoteSpliceFired) has none, and the results after
// it answer one place lower.
func (es *EmitState) producerOf(v *core.Value) (producer, bool) {
	return es.producerOfKey(core.RecKeyOf(v))
}

// producerOfKey is producerOf over the recorder key a holder carries — the
// form the recorder seam's queries take (core.EmitRecorder), so a caller's
// holder never crosses the interface.
func (es *EmitState) producerOfKey(k core.RecKey) (producer, bool) {
	if es == nil {
		return producer{}, false
	}
	if k.Pass == 0 || k.Pass != es.recPass {
		return producer{}, false
	}
	seq, idx := int(k.Seq), int(k.Idx)
	if seq < 0 || es.genOf(seq) != k.Gen {
		return producer{}, false
	}
	for _, f := range es.firedIdx[seq] {
		switch {
		case f == idx:
			return producer{}, false
		case f < idx:
			idx--
		}
	}
	return producer{seq: seq, idx: idx}, true
}

// producerSeq is the event v is a result of, 0 (no event) for a value no
// event of this pass produced.
func (es *EmitState) producerSeq(v *core.Value) int {
	pr, _ := es.producerOf(v)
	return pr.seq
}

// heldAs stands in for a value a note kept only the identity and recorder
// key of (a landing's own value): a query over it reads both, as it would
// have read them on the holder.
func heldAs(id string, k core.RecKey) core.Value {
	v := core.IdentityOnly(id)
	core.SetRecKey(&v, k)
	return v
}
