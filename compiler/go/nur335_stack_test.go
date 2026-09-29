package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur335_stack_test.go pins the second round of NUR335/NUR336: the island
// seats the stack the pass stepped into its statement with (the `end`
// before it tells it — NoteStatementStack), checks the compiled stack still
// holds what it seats from it at the stop (heldIntact), writes a paren
// apply's own lead as the value the apply found (parenLead), and a root
// paren apply the program only pushes after places a data lead itself
// (markTailPlacements, DynMethodSpec.Place). The lang suite drives them end
// to end (TestNUR335IslandSeatsTheStackAtTheStatement,
// TestNUR336PlacedParenApply).

// NoteStatementStack keeps the stack a ROOT boundary leaves, by its
// position, and nothing else: no recording pass, no position, an open unit.
func TestNoteStatementStack(t *testing.T) {
	pos := core.SrcPos{Row: 1, Col: 9}
	stack := []core.Value{n335Val("a", 1, 1)}
	var inactive *EmitState
	inactive.NoteStatementStack(pos, stack)
	es := NewEmitState()
	es.Compilable = true
	es.NoteStatementStack(core.SrcPos{}, stack)
	es.openUnitRecs = []int{0}
	es.NoteStatementStack(pos, stack)
	if len(es.rootStmtStacks) != 0 {
		t.Fatalf("no position, or an open unit: nothing kept, got %v", es.rootStmtStacks)
	}
	es.openUnitRecs = nil
	es.NoteStatementStack(pos, stack)
	stack[0] = n335Val("b", 1, 1)
	if got := es.rootStmtStacks[pos]; len(got) != 1 || got[0].ID != "a" {
		t.Errorf("the root boundary's stack, copied: %v", got)
	}
	// stackAtStart reads it through the `end` before the statement.
	end := core.NewEnd()
	end.SetPos(pos)
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 11)}
	if got, told := es.stackAtStart(2); !told || len(got) != 1 {
		t.Errorf("the statement after the end: %v %v", got, told)
	}
	if _, told := es.stackAtStart(0); told {
		t.Error("the program's first statement follows no end")
	}
}

// seatStack places each value of the stack at the statement's start where
// the root keeps it: a promoted result in its slot, a held one on the
// compiled stack (its producer noted, bottom first), a literal known to the
// bottom as a constant; a carrier cannot be seated. rootPreStart takes it
// over the residual's inference wherever the pass told the stack.
func TestSeatStack(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, promoted: map[int]int{4: 7}}
	es.producedBy["slot"] = producer{seq: 4, idx: 1}
	es.producedBy["held"] = producer{seq: 3}
	srcs, held, slots, ok := es.seatStack(lw, []core.Value{n335Val("slot", 0, 0), n335Val("held", 0, 0), n335Val("lit", 1, 1)})
	if !ok || held != 1 || len(srcs) != 3 || srcs[0].Kind != RestartLocal || srcs[0].Idx != 8 ||
		srcs[1].Kind != RestartStack || srcs[2].Kind != RestartConst || len(slots) != 1 || slots[0] != (vmSlot{seq: 3}) {
		t.Errorf("each value where the root keeps it: %+v held=%d slots=%v ok=%v", srcs, held, slots, ok)
	}
	carrier := n335Val("c", 1, 1)
	carrier.Carrier = true
	if _, _, _, ok := es.seatStack(lw, []core.Value{carrier}); ok {
		t.Error("a carrier has no value to seat")
	}
	end := core.NewEnd()
	end.SetPos(core.SrcPos{Row: 1, Col: 3})
	es.rootBody = []core.Value{restartTok(1, 1), end, restartTok(1, 5)}
	es.rootStmtStacks = map[core.SrcPos][]core.Value{end.Pos(): {n335Val("lit", 1, 1)}}
	if srcs, _, slots, ok := es.rootPreStart(lw, nil, nil, 2, core.SrcPos{Row: 1, Col: 5}, 0); !ok || len(srcs) != 1 || slots == nil {
		t.Errorf("a told stack is seated as told: %+v %v %v", srcs, slots, ok)
	}
}

// heldIntact: the values the island seats from the compiled stack must
// still stand at its bottom at the stop — the told producers where the pass
// told the stack, else the stack the statement began over.
func TestHeldIntact(t *testing.T) {
	lw := &lowerer{vm: []vmSlot{{seq: 3}, {seq: 5}}}
	for _, c := range []struct {
		name string
		r    landingRestart
		want bool
	}{
		{"nothing held", landingRestart{held: 0}, true},
		{"the told producer still there", landingRestart{held: 1, heldAt: []vmSlot{{seq: 3}}}, true},
		{"another value in its slot", landingRestart{held: 1, heldAt: []vmSlot{{seq: 9}}}, false},
		{"the stack it began over", landingRestart{held: 2, beneath: []vmSlot{{seq: 3}, {seq: 5}}}, true},
		{"more held than the stack holds", landingRestart{held: 3, beneath: []vmSlot{{seq: 3}, {seq: 5}, {seq: 6}}}, false},
	} {
		if got := lw.heldIntact(&c.r); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// parenLead plans the island's write of a paren apply's own lead, placed:
// none for another stop, a lead no event produced, or a lead outside the
// statement's paren.
func TestParenLead(t *testing.T) {
	head, arg := restartTok(1, 2), restartTok(1, 4)
	paren := core.NewParenExpr([]core.Value{head, arg})
	paren.SetPos(core.SrcPos{Row: 1, Col: 1})
	body := []core.Value{paren}
	apply := EmitEvent{kind: evCall, call: emitCall{word: parenApplyWord, nout: 1, pos: head.Pos(),
		ops: []EmitOperand{EventOperand(1, 0), EventOperand(2, 0)}, dynMethod: &DynMethodSpec{NArgs: 1, NOut: 1, Paren: true}}}
	es := &EmitState{}
	tree := map[int]treeEvent{}
	if got := es.parenLead(tree, &apply, body, 0); len(got) != 1 || got[0].seq != 1 || !got[0].run || len(got[0].path) != 2 {
		t.Errorf("the lead's own plan: %+v", got)
	}
	if got := es.parenLead(tree, &apply, body, 1); got != nil {
		t.Errorf("a lead before the statement: %+v", got)
	}
	local := apply
	local.call.ops = []EmitOperand{{kind: opLocal}, EventOperand(2, 0)}
	shaped := apply
	shaped.call.dynMethod = &DynMethodSpec{NArgs: 1, NOut: 1}
	bare := apply
	bare.call.pos = core.SrcPos{Row: 1, Col: 1}
	for _, ev := range []*EmitEvent{&local, &shaped, {kind: evBranch}, &bare} {
		if got := es.parenLead(tree, ev, body, 0); got != nil {
			t.Errorf("no lead plan: %+v", got)
		}
	}
}

// markTailPlacements marks a root paren apply the program only pushes
// after, and no other apply.
func TestMarkTailPlacements(t *testing.T) {
	p := &Program{
		DynMethods: []DynMethodSpec{{Paren: true}, {Paren: true}, {}},
		Code: []Instr{
			{Op: OpCallDynMethod, Arg: 0}, {Op: OpCallNative},
			{Op: OpCallDynMethod, Arg: 1}, {Op: OpPushConst}, {Op: OpPushConstFresh}, {Op: OpPushLocal}, {Op: OpPushType},
		},
	}
	// Every instruction written after the one before it (pushesWrittenAfter).
	for i := range p.Code {
		p.Debug = append(p.Debug, core.SrcPos{Row: 1, Col: i + 1})
	}
	markTailPlacements(p)
	if p.DynMethods[0].Place || !p.DynMethods[1].Place {
		t.Errorf("only the apply the program only pushes after: %+v", p.DynMethods)
	}
	p.Code = append(p.Code, Instr{Op: OpCallDynMethod, Arg: 2})
	p.Debug = append(p.Debug, core.SrcPos{Row: 1, Col: len(p.Code)})
	markTailPlacements(p)
	if p.DynMethods[2].Place {
		t.Error("a shaped method apply is no paren's")
	}
}

// seatLandingSkip reads a stash of the word's result — stored and pushed
// straight back for the apply's statement island — as the stack unchanged.
func TestSeatLandingSkipOverAStash(t *testing.T) {
	apply := &emitCall{dynMethod: &DynMethodSpec{NOut: 1}, ops: []EmitOperand{EventOperand(7, 0), ConstOperand(0)}}
	seat := func(pushed int32) LandingWord {
		code := []Instr{{Op: OpReStepLanding}, {Op: OpCallUser}, {Op: OpStoreLocal, Arg: 2}, {Op: OpPushLocal, Arg: pushed}, {Op: OpSwap}}
		words := map[int]LandingWord{0: {Name: "y"}}
		lw := &lowerer{code: &code, landingWords: &words, landingSkips: map[int]int{7: 0}}
		lw.seatLandingSkip(apply)
		return words[0]
	}
	if w := seat(2); w.SkipTo != 6 {
		t.Errorf("the skip past the apply over a stashed result: %+v", w)
	}
	if w := seat(3); w.SkipTo != 0 {
		t.Errorf("a store of another slot is no stash: %+v", w)
	}
}
