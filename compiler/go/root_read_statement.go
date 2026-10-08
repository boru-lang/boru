package compiler

import core "github.com/boru-lang/boru/core/go"

// rootReadStatementPoint is the statement-start island of a root read of a
// gradual def-bound value (NUR207) that a root event consumes, where the
// read's own placement fails (seatRootConsumedRead): a literal written
// before the read in its statement, which the compiled root lays out only
// at the program's end — `1 x 7 add` over `def x m.f`, an opaque Map's fn
// member (NUR352). The interpreter dispatches x as a word there, its
// forward phase taking the 7 (8), and `add` then takes the 1 (9); the model
// took x as data, and the guard it fell back to deferred loudly. Tested
// before the statement's first op, the point hands the whole statement to
// the interpreter from its first token when the read holds a fn: nothing of
// the statement has run (writtenBeforeTest), every event from the test to
// the consumer is placed (unplacedBeforeRead), and no operand the island
// takes over is deferred past the start (deoptDeferred). The island's
// prefix is the program residual's entries before the statement, seated
// where the compiled root keeps each (rootPreStart, DeoptSpec.Seat): an
// earlier statement's literal the root lays out only at the program's end
// (`5 end 1 x 7 add`, NUR352) is a constant the island seats. The read is
// made once, and its value lives in the frame slot the test reads.
func (es *EmitState) rootReadStatementPoint(lw *lowerer, rec *fnUnitRec, r rootWordRead, seq, ci int, alsoResidual bool, residual []core.Value) (deoptPoint, bool) {
	if len(r.reads) != 1 || alsoResidual {
		return deoptPoint{}, false
	}
	if _, promoted := lw.promoted[seq]; !promoted {
		return deoptPoint{}, false
	}
	tok := statementToken(es.rootBody, r.reads[0])
	if tok < 0 || es.rootBody[tok].Pos().Row == 0 {
		return deoptPoint{}, false
	}
	d := deoptPoint{seq: seq, slot: -1, name: r.name, pos: r.reads[0], start: es.rootBody[tok].Pos(), token: tok}
	events := es.frames[0]
	k := testIndex(events, d.start)
	if ci < 0 || ci >= len(events) || k > ci || writtenBeforeTest(events, k, d.start) || unplacedBeforeRead(events, k, events[ci].seq) ||
		es.deoptDeferred(es.units[0], rec, &d, ci) {
		return deoptPoint{}, false
	}
	// The program residual's entries before the statement are the island's
	// prefix, seated where the compiled root keeps each — an earlier
	// statement's literal it lays out only at the program's end included
	// (`5 end 1 x 7 add`, NUR352) — as a statement island's are.
	tree := rootTreeEvents(events, false)
	srcs, held, _, ok := es.rootPreStart(lw, tree, residual, tok, d.start, statementFirstSeq(tree, events[ci].seq, d.start))
	if !ok {
		return deoptPoint{}, false
	}
	d.seat, d.seatHeld, d.seated = srcs, held, true
	return d, true
}
