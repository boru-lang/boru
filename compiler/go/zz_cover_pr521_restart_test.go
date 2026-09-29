package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// planLandingRestarts' three declining arms, each paired with the plan the
// same program makes once the arm's condition is lifted, and parenOf's
// descent past a nested container that holds no token at the event.

// pr521Plan runs planLandingRestarts over es and reports whether it planned
// an island for seq.
func pr521Plan(es *EmitState, seq int) bool {
	lw := &lowerer{es: es, promoted: map[int]int{}}
	es.planLandingRestarts(lw, nil)
	return lw.landingRestarts[seq] != nil
}

// pr521Shaped is a shaped method apply (a dynMethod call) at pos.
func pr521Shaped(seq int, pos core.SrcPos) EmitEvent {
	return EmitEvent{kind: evCall, seq: seq, call: emitCall{word: "(apply)", nout: 1, pos: pos, dynMethod: &DynMethodSpec{NArgs: 0, NOut: 1}}}
}

// A shaped apply no top-level token holds has no statement to take over.
func TestPlanLandingRestartsNoStatementToken(t *testing.T) {
	end := n336Tok(core.NewEnd(), 1, 3)
	es := NewEmitState()
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7)}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): nil}
	es.frames[0] = []EmitEvent{pr521Shaped(2, core.SrcPos{})}
	if pr521Plan(es, 2) {
		t.Error("an apply no token holds plans no island")
	}
	es.frames[0] = []EmitEvent{pr521Shaped(2, core.SrcPos{Row: 1, Col: 7})}
	if !pr521Plan(es, 2) {
		t.Error("the apply in the statement after the told end plans its island")
	}
}

// A statement with no known start, or one whose stack the pass did not tell
// where the program ends in a trap, plans none.
func TestPlanLandingRestartsUnseatedStart(t *testing.T) {
	end := n336Tok(core.NewEnd(), 1, 3)
	es := NewEmitState()
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7)}
	es.frames[0] = []EmitEvent{pr521Shaped(2, core.SrcPos{Row: 1, Col: 7})}
	es.trapAt = 5
	if pr521Plan(es, 2) {
		t.Error("an untold stack before a terminal trap plans no island")
	}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): nil}
	if !pr521Plan(es, 2) {
		t.Error("a told stack before a terminal trap plans the apply's island")
	}

	// A position-less `end` and first token: the statement orders by no
	// position at all.
	bare := core.NewEnd()
	pair := core.NewInteger(1)
	anon := NewEmitState()
	anon.rootBody = []core.Value{restartTok(1, 1), bare, pair, restartTok(1, 7)}
	anon.rootStmtStacks = map[core.SrcPos][]core.Value{{}: nil}
	anon.frames[0] = []EmitEvent{pr521Shaped(2, core.SrcPos{Row: 1, Col: 7})}
	if statementStart(anon.rootBody, 2).Row != 0 {
		t.Fatal("fixture: the statement must have no start")
	}
	if pr521Plan(anon, 2) {
		t.Error("a statement with no start plans no island")
	}
}

// A told stack the island cannot seat (a carrier) plans none.
func TestPlanLandingRestartsUnseatableStack(t *testing.T) {
	end := n336Tok(core.NewEnd(), 1, 3)
	es := NewEmitState()
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 5), restartTok(1, 7)}
	es.frames[0] = []EmitEvent{pr521Shaped(2, core.SrcPos{Row: 1, Col: 7})}
	carrier := n335Val("c", 1, 1)
	carrier.Carrier = true
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): {carrier}}
	if pr521Plan(es, 2) {
		t.Error("a told stack holding a carrier seats no island")
	}
	es.rootStmtStacks[end.Pos()] = []core.Value{n335Val("", 1, 1)}
	if !pr521Plan(es, 2) {
		t.Error("a told stack of a constant seats the island")
	}
}

// parenOf declines a call standing at a paren's own position whose tokens
// begin later: the path ends at the paren, the loop descends into it and
// finds no token there. The same call at the paren's first token is the
// paren's value.
func TestParenOfPastNestedContainer(t *testing.T) {
	head := restartTok(1, 12)
	paren := core.NewParenExpr([]core.Value{head})
	paren.SetPos(core.SrcPos{Row: 1, Col: 10})
	body := []core.Value{paren}
	at := func(col int) *EmitEvent {
		return &EmitEvent{kind: evCallUser, uc: emitUserCall{pos: core.SrcPos{Row: 1, Col: col}, nout: 1}}
	}
	if p := tokenPath(body, core.SrcPos{Row: 1, Col: 10}); len(p) != 1 || p[0] != 0 {
		t.Fatalf("fixture: the path must end at the paren: %v", p)
	}
	if path, ok := parenOf(at(10), body, 0); ok || path != nil {
		t.Errorf("a call at the paren's own position is no paren's: %v %v", path, ok)
	}
	if path, ok := parenOf(at(12), body, 0); !ok || len(path) != 1 || path[0] != 0 {
		t.Errorf("a call at the paren's first token is the paren's value: %v %v", path, ok)
	}
	// An empty paren likewise: nothing inside to hold the call.
	empty := core.NewParenExpr(nil)
	empty.SetPos(core.SrcPos{Row: 1, Col: 10})
	if path, ok := parenOf(at(10), []core.Value{empty}, 0); ok || path != nil {
		t.Errorf("an empty paren holds no call: %v %v", path, ok)
	}
}
