package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The NUR336 remainders' planner pieces: where a statement island starts
// (statementStart, toldAfter, defsBefore), how it writes a paren apply's
// lead (rawRead, rawReachLead, parenLead, leadUnrun), where the apply stands
// (parenLeadToken, stopPos) and what a told stack seats (seatStack).

func n336Tok(v core.Value, row, col int) core.Value {
	v.SetPos(core.SrcPos{Row: row, Col: col})
	return v
}

// statementStart orders a statement by its first token's position, or — a
// token the parser mints without one — by the point just past the `end`
// before it, or the program's start.
func TestStatementStart(t *testing.T) {
	end := n336Tok(core.NewEnd(), 1, 3)
	pair := core.NewInteger(1)
	body := []core.Value{pair, end, pair, restartTok(1, 9)}
	if got := statementStart(body, 3); got != (core.SrcPos{Row: 1, Col: 9}) {
		t.Errorf("a positioned token is its own start: %v", got)
	}
	if got := statementStart(body, 2); got != (core.SrcPos{Row: 1, Col: 4}) {
		t.Errorf("past the end before it: %v", got)
	}
	if got := statementStart(body, 0); got != (core.SrcPos{Row: 1}) {
		t.Errorf("the program's start: %v", got)
	}
	if got := statementStart([]core.Value{restartTok(1, 1), pair}, 1); got.Row != 0 {
		t.Errorf("no end before it, no start: %v", got)
	}
}

// toldAfter takes the last token up to the stop whose stack a completed def
// told; stackAtStart reads it there, or through the `end` before the
// statement.
func TestToldAfterAndStackAtStart(t *testing.T) {
	es := NewEmitState()
	end := n336Tok(core.NewEnd(), 1, 3)
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7), restartTok(1, 9), restartTok(1, 11)}
	stop := core.SrcPos{Row: 1, Col: 9}
	if got := es.toldAfter(2, stop); got != 2 {
		t.Errorf("nothing told: the statement's start, got %d", got)
	}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{
		{Row: 1, Col: 7}:  {n335Val("a", 1, 1)},
		{Row: 1, Col: 11}: nil,
		end.Pos():         nil,
	}
	if got := es.toldAfter(2, stop); got != 3 {
		t.Errorf("the told token before the stop, not one after it: got %d", got)
	}
	if got, told := es.stackAtStart(3); !told || len(got) != 1 {
		t.Errorf("the stack told at the token: %v %v", got, told)
	}
	if _, told := es.stackAtStart(2); !told {
		t.Error("the statement's first token reads the end's")
	}
	if _, told := es.stackAtStart(4); !told {
		t.Error("a later token nothing told there reads the end before its statement")
	}
	if _, told := es.stackAtStart(-1); told {
		t.Error("no token, no stack")
	}
	if _, told := es.stackAtStart(0); told {
		t.Error("the program's first statement follows no end")
	}
}

// defsBefore skips a unit statement's opening defs whose value the def took
// whole from one token: a literal, or a paren or reach an event inside which
// produced it; a def of a word's call, or one the pass did not record,
// stops it.
func TestDefsBefore(t *testing.T) {
	def := core.NewWord("def")
	paren := n336Tok(core.NewParenExpr([]core.Value{restartTok(1, 12)}), 1, 11)
	body := []core.Value{
		n336Tok(def, 1, 1), restartTok(1, 5), n336Tok(core.NewInteger(3), 1, 7),
		n336Tok(def, 1, 9), restartTok(1, 10), paren,
		n336Tok(def, 1, 15), restartTok(1, 16), n336Tok(core.NewWord("fn"), 1, 18),
		restartTok(1, 30),
	}
	tree := map[int]treeEvent{
		1: {ev: &EmitEvent{kind: evDynBind, seq: 1, dyn: &emitDynBind{pos: core.SrcPos{Row: 1, Col: 5}, srcSeq: -1}}},
		2: n335Call(2, 1, 12),
		3: {ev: &EmitEvent{kind: evDynBind, seq: 3, dyn: &emitDynBind{pos: core.SrcPos{Row: 1, Col: 10}, srcSeq: 2}}},
		4: {ev: &EmitEvent{kind: evDynBind, seq: 4, dyn: &emitDynBind{pos: core.SrcPos{Row: 1, Col: 16}, srcSeq: -1}}},
	}
	if got := defsBefore(tree, body, 0, core.SrcPos{Row: 1, Col: 30}); got != 6 {
		t.Errorf("past the literal and the paren, not the word's call: got %d", got)
	}
	if got := defsBefore(map[int]treeEvent{}, body, 0, core.SrcPos{Row: 1, Col: 30}); got != 0 {
		t.Errorf("a def the pass did not record: got %d", got)
	}
	if tookWhole(tree, &emitDynBind{srcSeq: 9}, body, 5) {
		t.Error("a producer outside the tree")
	}
}

// rawRead is a member read nothing changed; rawReachLead one the evaluated
// reach token at the path read; noteLeadRead marks the paren applies whose
// lead the island steps itself.
func TestRawReadAndLeadReads(t *testing.T) {
	es := NewEmitState()
	dot := n335Call(1, 1, 2)
	dot.ev.call.word = "dot"
	tree := map[int]treeEvent{1: dot, 2: n335Call(2, 1, 4)}
	if !es.rawRead(tree, 1) || es.rawRead(tree, 2) || es.rawRead(tree, 7) {
		t.Error("a dot read, not another call or no event")
	}
	reach := core.NewReach(core.ReachInfo{Receiver: []core.Value{core.NewWord("m")}, Eval: true})
	body := []core.Value{n336Tok(core.NewParenExpr([]core.Value{reach, core.NewInteger(7)}), 1, 1)}
	if !es.rawReachLead(tree, 1, body, []int{0, 0}) {
		t.Error("the evaluated reach token that read it")
	}
	quoted := reach
	quoted.Quoted = true
	if es.rawReachLead(tree, 1, []core.Value{core.NewParenExpr([]core.Value{quoted})}, []int{0, 0}) || es.rawReachLead(tree, 2, body, []int{0, 0}) {
		t.Error("a quoted reach, or no raw read")
	}
	es.landingAfter = map[int]core.SrcPos{1: {Row: 1, Col: 3}}
	if es.rawRead(tree, 1) {
		t.Error("a landing may fire the read")
	}
	apply := func(seq int, lead EmitOperand, name string) *EmitEvent {
		return &EmitEvent{kind: evCall, seq: seq, call: emitCall{word: parenApplyWord, ops: []EmitOperand{lead},
			dynMethod: &DynMethodSpec{Paren: true, LeadName: name}}}
	}
	es.noteLeadRead(tree, apply(10, EventOperand(1, 0), ""))
	es.noteLeadRead(tree, apply(11, localOperand(0), ""))
	es.noteLeadRead(tree, apply(12, EventOperand(1, 0), "g"))
	es.noteLeadRead(tree, &EmitEvent{kind: evCall, seq: 13, call: emitCall{word: "w"}})
	if es.leadReads[10] || !es.leadReads[11] || !es.leadReads[12] || es.leadReads[13] {
		t.Errorf("a no-event lead or a def-bound word's, not a landed read's: %v", es.leadReads)
	}
}

// parenLead plans a def-bound word's lead as the name's call writes it,
// though no event produced its value.
func TestParenLeadNamed(t *testing.T) {
	head := restartTok(1, 2)
	paren := n336Tok(core.NewParenExpr([]core.Value{head, restartTok(1, 4)}), 1, 1)
	ev := EmitEvent{kind: evCall, call: emitCall{word: parenApplyWord, pos: core.SrcPos{Row: 9, Col: 9}, leadAt: head,
		ops: []EmitOperand{localOperand(0)}, dynMethod: &DynMethodSpec{Paren: true, LeadName: "t"}}}
	got := NewEmitState().parenLead(map[int]treeEvent{}, &ev, []core.Value{paren}, 0)
	if len(got) != 1 || !got[0].named || got[0].reach || len(got[0].path) != 2 {
		t.Errorf("the named lead's plan: %+v", got)
	}
}

// parenLeadToken finds a paren apply's lead token by its argument when the
// lead's value carries another position; stopPos is where the apply stands.
func TestParenLeadTokenAndStopPos(t *testing.T) {
	es := NewEmitState()
	g := restartTok(2, 2)
	seven := n336Tok(core.NewInteger(7), 2, 4)
	es.rootBody = []core.Value{restartTok(1, 1), n336Tok(core.NewParenExpr([]core.Value{g, seven}), 2, 1)}
	away := core.SrcPos{Row: 1, Col: 1}
	if lead, moved := es.parenLeadToken(parenApplyWord, away, []core.Value{core.NewInteger(1), seven}); !moved || lead.Pos() != g.Pos() {
		t.Errorf("the lead token by the argument: %v %v", lead, moved)
	}
	if _, moved := es.parenLeadToken(parenApplyWord, g.Pos(), []core.Value{seven}); moved {
		t.Error("the lead's own position")
	}
	if _, moved := es.parenLeadToken("w", away, []core.Value{seven}); moved {
		t.Error("another apply")
	}
	if _, moved := es.parenLeadToken(parenApplyWord, away, []core.Value{n336Tok(core.NewInteger(9), 1, 1)}); moved {
		t.Error("an argument no paren holds")
	}
	es.fnRecs = []*fnUnitRec{{body: []core.Value{n336Tok(core.NewParenExpr([]core.Value{seven}), 2, 1)}}}
	es.openUnitRecs = []int{0}
	if _, moved := es.parenLeadToken(parenApplyWord, away, []core.Value{seven}); moved {
		t.Error("the open unit's body, where the argument leads its paren")
	}
	ev := EmitEvent{kind: evCall, call: emitCall{pos: away}}
	if stopPos(&ev) != away {
		t.Error("its own position")
	}
	ev.call.leadAt = g
	if stopPos(&ev) != g.Pos() {
		t.Error("the lead token's")
	}
}

// leadUnrun: the island steps the lead as the paren does — written as its
// reach or as the name's call, or not written at all — never a value it
// holds placed.
func TestLeadUnrun(t *testing.T) {
	guard := RestartSrc{Kind: RestartGuard}
	for _, c := range []struct {
		substs []RestartSubst
		want   bool
	}{
		{nil, true},
		{[]RestartSubst{{Src: RestartSrc{Kind: RestartLocal}}}, true},
		{[]RestartSubst{{Src: guard, Reach: true}}, true},
		{[]RestartSubst{{Src: guard, Named: true}}, true},
		{[]RestartSubst{{Src: guard, Placed: true}}, false},
	} {
		if got := leadUnrun(c.substs); got != c.want {
			t.Errorf("%+v: %v, want %v", c.substs, got, c.want)
		}
	}
}

// planRematchRestart plans nothing for a program with no rematch trap, a trap
// no top-level token holds, a statement whose stack the pass did not tell, or
// one whose earlier run no island may repeat.
func TestPlanRematchRestartDeclines(t *testing.T) {
	trap := func(seq int, pos core.SrcPos) EmitEvent {
		return EmitEvent{kind: evTrap, seq: seq, trap: EmitTrap{rematchWord: "add", pos: pos}}
	}
	plan := func(es *EmitState) bool {
		lw := &lowerer{es: es, promoted: map[int]int{}}
		es.planRematchRestart(lw, nil)
		return lw.landingRestarts[es.trapAt] != nil
	}
	end := n336Tok(core.NewEnd(), 1, 3)
	body := []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7)}
	es := NewEmitState()
	es.rootBody = body
	if plan(es) {
		t.Error("no trap")
	}
	es.frames[0] = []EmitEvent{trap(2, core.SrcPos{Row: 1, Col: 7})}
	es.trapAt = 2
	if plan(es) {
		t.Error("no told stack")
	}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): nil}
	es.frames[0] = []EmitEvent{{kind: evLoop, seq: 1, loop: &emitLoop{pos: core.SrcPos{Row: 1, Col: 5}}}, trap(2, core.SrcPos{Row: 1, Col: 7})}
	if plan(es) {
		t.Error("a loop ran in the statement before the trap")
	}
	es.frames[0] = []EmitEvent{trap(2, core.SrcPos{Row: 0, Col: 0})}
	if plan(es) {
		t.Error("a trap no token holds")
	}
	es.frames[0] = []EmitEvent{trap(2, core.SrcPos{Row: 1, Col: 7})}
	carrier := n335Val("c", 1, 1)
	carrier.Carrier = true
	es.rootStmtStacks[end.Pos()] = []core.Value{carrier}
	if plan(es) {
		t.Error("a told stack holding a carrier cannot be seated")
	}
	es.rootStmtStacks[end.Pos()] = nil
	if !plan(es) {
		t.Error("the statement after the told end")
	}
}

// seatStack seats a type node the run resolves by its ID.
func TestSeatStackType(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, promoted: map[int]int{}}
	integer := core.NewTypeLiteral(core.TInteger)
	srcs, _, _, ok := es.seatStack(lw, []core.Value{integer})
	if !ok || len(srcs) != 1 || srcs[0].Kind != RestartType || srcs[0].Val.ID != integer.ID {
		t.Errorf("the type node by its ID: %+v %v", srcs, ok)
	}
	dyn := integer
	dyn.Dynamic = true
	if _, _, _, ok := es.seatStack(lw, []core.Value{dyn}); ok {
		t.Error("a dynamic value cannot be seated")
	}
}
