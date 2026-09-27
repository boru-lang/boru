package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur222_count_test.go pins the do count island (landing_restart.go,
// NUR222's consumer half): which do a plan takes, the runs its island
// writes, and how the lowering seats one. The lang suite drives it end to
// end (TestNUR222ConsumedPhantomCountIsland).

// countBody is `w end 5 do [(mk)] drop`: the statement begins at token 2,
// the do word is token 3 (column 7) and its body the list after it.
func countBody() []core.Value {
	body := gtok(core.NewEvalList([]core.Value{gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 11)}), 10)}), 9)
	return []core.Value{
		gtok(core.NewWord("w"), 1), gtok(core.NewEnd(), 3), gtok(core.NewInteger(5), 5), gtok(core.NewWord("do"), 7),
		body, gtok(core.NewWord("drop"), 20),
	}
}

// countDo is the do call at column col, seq 10, over its body's closure.
func countDo(col int) EmitEvent {
	return EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "do", pos: gpos(col), ops: []EmitOperand{{kind: opClosure}}}}
}

// countDrop is the call at seq 12 that takes the do's phantom.
func countDrop() EmitEvent {
	return EmitEvent{kind: evCall, seq: 12, call: emitCall{word: "drop", pos: gpos(20), ops: []EmitOperand{{kind: opEvent, idx: 10}}}}
}

func TestCountPoint(t *testing.T) {
	es := NewEmitState()
	do := countDo(7)
	tree := map[int]treeEvent{10: {ev: &do}}
	tok, substs, ok := es.countPoint(tree, 10, countBody())
	if !ok || tok != 2 || len(substs) != 1 || !substs[0].results || substs[0].span != 2 || substs[0].path[0] != 3 {
		t.Errorf("the do word and its body are the run the island writes: %d %+v %v", tok, substs, ok)
	}
	other := EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "print", pos: gpos(7), ops: []EmitOperand{{kind: opClosure}}}}
	if _, _, ok := es.countPoint(map[int]treeEvent{10: {ev: &other}}, 10, countBody()); ok {
		t.Error("only a do over its body's closure has a count island")
	}
	lead := countBody()
	lead[2] = gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 5)}), 5)
	if _, _, ok := es.countPoint(tree, 10, lead); ok {
		t.Error("a paren before the do on its level is a collection barrier the island cannot write inert")
	}
	bare := countBody()
	bare[4] = gtok(core.NewInteger(9), 9)
	if _, _, ok := es.countPoint(tree, 10, bare); ok {
		t.Error("a do with no literal body written after it has no run to write")
	}
	// A do inside a list literal, a paren before the list: the paren is the
	// statement's pending event, written as its value.
	inner := countBody()
	list := gtok(core.NewEvalList([]core.Value{inner[2], inner[3], inner[4], inner[5]}), 4)
	paren := gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 2)}), 2)
	nested := []core.Value{paren, list}
	mk := EmitEvent{kind: evCallUser, seq: 8, uc: emitUserCall{pos: gpos(2), nout: 1}}
	withPending := map[int]treeEvent{8: {ev: &mk}, 10: {ev: &do}}
	tok, substs, ok = es.countPoint(withPending, 10, nested)
	if !ok || tok != 0 || len(substs) != 2 || substs[0].seq != 8 || substs[1].path[0] != 1 || substs[1].path[1] != 1 {
		t.Errorf("the pending paren's value, then the nested do's run: %d %+v %v", tok, substs, ok)
	}
	effect := EmitEvent{kind: evCall, seq: 8, call: emitCall{word: "print", pos: gpos(2)}}
	flat := []core.Value{gtok(core.NewWord("print"), 2), list}
	if _, _, ok := es.countPoint(map[int]treeEvent{8: {ev: &effect}, 10: {ev: &do}}, 10, flat); ok {
		t.Error("an effect before the do in its statement plans no island")
	}
}

func TestPlanCountRestarts(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es}
	es.planCountRestarts(lw, nil)
	if lw.countRestarts != nil {
		t.Error("no program body, no plan")
	}
	es.rootBody = countBody()
	es.eventInfo[10] = eventFlags{catchPhantom: true}
	es.frames[0] = []EmitEvent{countDo(7), countDrop()}
	es.planCountRestarts(lw, nil)
	if r := lw.countRestarts[10]; r == nil || r.token != 2 || r.depth != -1 || len(r.substs) != 1 || !es.phantomConsumed[10] {
		t.Errorf("the root plans the consumed do's island: %+v", r)
	}
	lw = &lowerer{es: es}
	es.planCountRestarts(lw, []core.Value{core.NewInteger(3)})
	if lw.countRestarts != nil {
		t.Error("a leading residual literal with no position is no placeable prefix")
	}
	es.rootBody[2] = gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 5)}), 5)
	es.planCountRestarts(lw, nil)
	if lw.countRestarts != nil {
		t.Error("a do the count island cannot take plans none")
	}
	es.rootBody = countBody()
	es.trapAt = 1
	es.planCountRestarts(lw, nil)
	if lw.countRestarts != nil {
		t.Error("a trapped program plans none: its trap runs before the statement")
	}
}

func TestPlanUnitCountRestarts(t *testing.T) {
	es := NewEmitState()
	es.eventInfo[10] = eventFlags{catchPhantom: true}
	rec := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{countDo(7), countDrop()}}, body: countBody()}
	es.planUnitRestarts(es.units[0], rec)
	if len(rec.deopts) != 1 || !rec.deopts[0].count || rec.deopts[0].token != 2 || !es.phantomConsumed[10] {
		t.Fatalf("the unit plans its consumed do's island as it closes: %+v", rec.deopts)
	}
	flw := &lowerer{es: es}
	seatDeoptPoint(flw, rec, rec.deopts[0])
	if r := flw.countRestarts[10]; r == nil || r.token != 2 || r.depth != -1 || len(r.substs) != 1 {
		t.Errorf("seated as the do's count island: %+v", r)
	}
	lead := &fnUnitRec{frag: &EmitFragment{events: []EmitEvent{countDo(7), countDrop()}}, body: countBody()}
	lead.body[2] = gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 5)}), 5)
	es.planUnitRestarts(es.units[0], lead)
	if len(lead.deopts) != 0 {
		t.Errorf("a do the count island cannot take plans none: %+v", lead.deopts)
	}
}

func TestCountIsland(t *testing.T) {
	lw := &lowerer{es: NewEmitState(), landingBody: countBody(), landingRoot: true}
	if lw.countIsland(10) != nil {
		t.Error("no plan, no island")
	}
	lw.countRestarts = map[int]*landingRestart{10: {token: 2, depth: -1, held: -1, substs: []substPlan{{path: []int{3}, span: 2, seq: 10, results: true}}}}
	if lw.countIsland(10) != nil {
		t.Error("an island the walk never seated is none")
	}
	lw.countRestarts[10].depth = 0
	is := lw.countIsland(10)
	if is == nil || len(is.Island) != 4 || len(is.Substs) != 1 || is.Substs[0].Src.Kind != RestartResults || is.Substs[0].Span != 2 || is.Substs[0].Path[0] != 1 || is.RetPC != -1 {
		t.Fatalf("the statement from its first token, the do's run written last: %+v", is)
	}
	lw.countRestarts[10].substs = append([]substPlan{{path: []int{2}, span: 1, seq: 8}}, lw.countRestarts[10].substs...)
	if lw.countIsland(10) != nil {
		t.Error("a paren's value the stop does not hold is no island")
	}
	s := SigRef{Count: is}
	stampSigRestart(&s, 9)
	if is.RetPC != 9 {
		t.Errorf("the count island continues at the stamped pc: %d", is.RetPC)
	}
}

// TestDoBodyAfterAndCountSeat pins the count island's reach past a caught
// body (NUR282's single seat): a do's body written right after it — its
// literal list, or a computed body's one argument site — and the seats that
// check a run's count.
func TestDoBodyAfterAndCountSeat(t *testing.T) {
	es := NewEmitState()
	es.argSites = map[int][]argSite{}
	toks := []core.Value{gtok(core.NewWord("do"), 1), gtok(core.NewWord("b"), 4), gtok(core.NewParenExpr([]core.Value{gtok(core.NewWord("mk"), 7)}), 6)}
	computed := EmitEvent{kind: evCall, seq: 10, call: emitCall{word: "do", pos: gpos(1), ops: []EmitOperand{{kind: opLocal}}}}
	mk := EmitEvent{kind: evCallUser, seq: 8, uc: emitUserCall{pos: gpos(7), nout: 1}}
	tree := map[int]treeEvent{10: {ev: &computed}, 8: {ev: &mk}}
	if es.doBodyAfter(tree, 10, toks, 0) {
		t.Error("a computed body with no argument site is none the island can place")
	}
	es.argSites[10] = []argSite{{pos: gpos(4), seq: -1}}
	if !es.doBodyAfter(tree, 10, toks, 0) {
		t.Error("a read written right after the do is its body")
	}
	es.argSites[10] = []argSite{{pos: gpos(9), seq: 8}}
	if es.doBodyAfter(tree, 10, toks, 0) {
		t.Error("a paren past the token after the do is not its body")
	}
	es.argSites[10] = []argSite{{seq: 3}}
	if es.doBodyAfter(tree, 10, toks, 0) {
		t.Error("an event the tree does not hold places nothing")
	}
	es.argSites[10] = []argSite{{pos: gpos(4), seq: -1}, {pos: gpos(4), seq: -1}}
	if es.doBodyAfter(tree, 10, toks, 0) {
		t.Error("a do takes one body")
	}
	es.argSites[10] = []argSite{{seq: 8}}
	if !es.doBodyAfter(tree, 10, []core.Value{toks[0], toks[2]}, 0) {
		t.Error("a paren right after the do, by its event")
	}
	es.eventInfo[1] = eventFlags{dynBodyOne: true}
	es.eventInfo[2] = eventFlags{dynBodyResult: true, variadicRegion: true, regionMayBeFn: true}
	es.phantomConsumed = map[int]bool{3: true}
	for seq, want := range map[int]bool{1: true, 2: true, 3: true, 4: false} {
		if es.countSeat(seq) != want {
			t.Errorf("seq %d: countSeat = %v", seq, !want)
		}
	}
}
