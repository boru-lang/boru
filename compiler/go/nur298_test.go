package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur298_test.go pins the COLLECTING landing (NUR298): inside a def's
// operand group the interpreter's re-step applies a landed fn over the literal
// written after it, before the def takes the group's first value. The note
// (core.LandingNextCollect) gives its op LandingCollects and routes it
// through the root guard and the statement island.

func TestLandingCollects(t *testing.T) {
	var none *EmitState
	if none.landingCollects(1) || none.landingArg(1, false) != 0 {
		t.Error("no recorder, no collecting landing")
	}
	es, v := nfState()
	es.NoteLandingNext(v, core.LandingNextCollect, true, core.Value{})
	if !es.landingCollects(1) || es.landingArg(1, false) != LandingCollects {
		t.Errorf("a collecting note makes a collecting landing whatever sits beneath: %v", es.landingArg(1, false))
	}
	for _, c := range []struct {
		a, b, want core.LandingNext
	}{
		{core.LandingNextCollect, core.LandingNextEnd, core.LandingNextCollect},
		{core.LandingNextBoundary, core.LandingNextCollect, core.LandingNextCollect},
		{core.LandingNextCollect, core.LandingNextWord, core.LandingNextWord},
	} {
		if got := mergeLandingNext(c.a, c.b); got != c.want {
			t.Errorf("merge(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	lw := &lowerer{es: es, landingRoot: true}
	es.landingOwn = map[int]landingStep{1: {}}
	lw.noteRootBeneathLanding(4, 1)
	if len(lw.rootBeneathLandings) != 1 {
		t.Errorf("a collecting landing goes to the guard with nothing beneath: %v", lw.rootBeneathLandings)
	}
}

// TestWordlessCollectingRestart pins the collecting landing's statement
// island: seated like a landing over values beneath (seatWordlessRestart).
func TestWordlessCollectingRestart(t *testing.T) {
	es := NewEmitState()
	es.landingOwn = map[int]landingStep{4: {}}
	es.landingNext = map[int]core.LandingNext{4: core.LandingNextCollect}
	var code []Instr
	words := map[int]LandingWord(nil)
	lw := &lowerer{es: es, code: &code, landingWords: &words, landingBody: doProgram(), landingRoot: true}
	lw.landingRestarts = map[int]*landingRestart{4: {token: 0, depth: 0, held: 0}}
	lw.seatLandingWord(LandingWord{}, 4)
	if w, ok := words[0]; !ok || !w.Restart {
		t.Errorf("a collecting landing seats its statement island: %+v", words)
	}
}

// TestCrossesStatementEndFirstEntry pins the def-read crossing (NUR266) to
// the entry right after the lead: `def j (5 do [(mk)] 7) end j` leaves the
// lambda, the 7 and j's read, and the apply takes the 7 (NUR298).
func TestCrossesStatementEndFirstEntry(t *testing.T) {
	es, v := nfState()
	es.stmtEnds = []core.SrcPos{{Row: 1, Col: 20}}
	es.defReads = map[string]string{"r": "j"}
	es.defReadPos = map[string][]core.SrcPos{"r": {{Row: 1, Col: 30}}}
	read := core.Value{ID: "r"}
	seven := core.WithPosAt(core.NewInteger(7), core.SrcPos{Row: 1, Col: 10})
	if !es.crossesStatementEnd(v, []core.Value{read}) {
		t.Error("a read right after the lead, past the end, crosses")
	}
	if es.crossesStatementEnd(v, []core.Value{seven, read}) {
		t.Error("a read further on is taken only by an arity that reaches it")
	}
}

// TestStampDeoptRetStampsLandings pins the unit stamp: a unit with deopt
// points stamps its landings' islands too (`[5] each [m get "f" drop]`).
func TestStampDeoptRetStampsLandings(t *testing.T) {
	cf := CompiledFn{Deopts: []DeoptSpec{{}}, LandingWords: map[int]LandingWord{3: {Restart: true, RetPC: -1}}}
	stampDeoptRet(&cf, 7)
	if cf.Deopts[0].RetPC != 7 || cf.LandingWords[3].RetPC != 7 || !cf.RetReplay {
		t.Errorf("both stamps run: %+v", cf)
	}
}
