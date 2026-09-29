package compiler

import (
	"slices"

	core "github.com/boru-lang/boru/core/go"
)

// THE FIRED SPLICE RESULT (NUR348). A `do` over a LITERAL body that reads a
// `word` value by value — `def w word [1 2] end do [w/v]` — hands back the
// splice marker itself; the interpreter splices a native's results back at
// its word and steps them, so the marker fires there: `1 2`. The check pass
// fires the very marker it modelled (stepLiteral) and records the
// expansion's own events after the call, but the marker was the call's
// recorded result, which the compiled `do` pushed and the VM's result screen
// rejected as a tape-coupled token: `do [w/v]` was an internal_error
// compiled.
//
// The marker's firing is now modelled as what it is: the marker leaves the
// stack where it stood among the run's results. Core tells the recorder when
// a marker fires (EmitRecorder.NoteSpliceFired), and a marker a `do` call
// produced is taken out of that call's results there (spliceFired) — the
// results after it move down, as the tape's do. The call's SigRef carries the
// results the pass stepped (SpliceOuts, only when every splice among them
// fired), and the VM seats the run without its splices when each renders as
// the pass's at its position (vm_splice_outs.go): the run's binding was the
// model's, so the recorded expansion is the interpreter's. Any other run
// keeps the screen's loud defer.

// noteSpliceOuts records the results outs of the `do` call seq when one of
// them is a splice over a concrete payload (concreteSplice).
func (es *EmitState) noteSpliceOuts(seq int, outs []core.Value) {
	if !slices.ContainsFunc(outs, concreteSplice) {
		return
	}
	if es.spliceOuts == nil {
		es.spliceOuts = map[int][]core.Value{}
	}
	es.spliceOuts[seq] = append([]core.Value(nil), outs...)
}

// concreteSplice reports whether v is a splice marker over a concrete
// payload — one the pass stepped token for token (a computed payload's
// spread is RecordSpliceDyn's, and renders as no run's marker).
func concreteSplice(v core.Value) bool {
	info, err := core.AsSplice(v)
	return err == nil && core.IsConcrete(info.Data)
}

// NoteSpliceFired takes a fired splice marker v out of the results of the
// noted `do` call that produced it (noteSpliceOuts), when that call is the
// current frame's: the marker's producer is forgotten, the results after it
// move down one place, and the call leaves one value fewer. Nothing for any
// other marker, or when inactive.
func (es *EmitState) NoteSpliceFired(v core.Value, _ core.SrcPos) {
	if !es.Active() || v.ID == "" || !concreteSplice(v) {
		return
	}
	pr, produced := es.producedBy[v.ID]
	outs := es.spliceOuts[pr.seq]
	if !produced || outs == nil {
		return
	}
	ev := es.eventBySeq(pr.seq)
	if ev == nil || ev.kind != evCall || ev.call.nout <= 0 {
		return
	}
	ev.call.nout--
	delete(es.producedBy, v.ID)
	for _, o := range outs {
		if p, ok := es.producedBy[o.ID]; ok && p.seq == pr.seq && p.idx > pr.idx {
			es.producedBy[o.ID] = producer{seq: p.seq, idx: p.idx - 1}
		}
	}
	if es.firedSplices == nil {
		es.firedSplices = map[int]map[string]bool{}
	}
	if es.firedSplices[pr.seq] == nil {
		es.firedSplices[pr.seq] = map[string]bool{}
	}
	es.firedSplices[pr.seq][v.ID] = true
}

// spliceOutsAt is the SigRef.SpliceOuts of the call seq: the results the
// pass stepped, when every splice among them fired (a marker the pass took
// as data is the screen's, as before); nil otherwise.
func (lw *lowerer) spliceOutsAt(seq int) []core.Value {
	if lw.es == nil {
		return nil
	}
	outs := lw.es.spliceOuts[seq]
	for _, v := range outs {
		if core.IsSplice(v) && !lw.es.firedSplices[seq][v.ID] {
			return nil
		}
	}
	return outs
}
